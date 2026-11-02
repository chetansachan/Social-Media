package repository

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/chetansachan/socialflow/feed-service/internal/models"
	"github.com/redis/go-redis/v9"
)

const timelineChunkSize = 500

// TimelineCache encapsulates every Redis operation used by the Feed Service.
// Only post IDs and timestamps are stored, never full post payloads.
type TimelineCache interface {
	GetTimeline(ctx context.Context, userID string, maxScore int64, limit int64) ([]models.TimelineItem, error)
	PushToTimeline(ctx context.Context, userID, postID string, score int64, maxSize int64, ttl time.Duration) error
	PushBatch(ctx context.Context, userIDs []string, postID string, score int64, maxSize int64, ttl time.Duration) error
	BackfillTimeline(ctx context.Context, userID string, items []models.TimelineItem, maxSize int64, ttl time.Duration) error
	RemovePost(ctx context.Context, userID, postID string) error
	RemovePostsBatch(ctx context.Context, userID string, postIDs []string) error
	InvalidateTimeline(ctx context.Context, userID string) error

	StoreCelebrityPost(ctx context.Context, authorID, postID string, score int64, maxItems int64, ttl time.Duration) error
	GetCelebrityRecentPostsBatch(ctx context.Context, authorIDs []string, limit int64) (map[string][]models.TimelineItem, error)
	RemoveCelebrityPost(ctx context.Context, authorID, postID string) error

	MarkUserActive(ctx context.Context, userID string, ttl time.Duration) error
	IsUserActiveBatch(ctx context.Context, userIDs []string) (map[string]bool, error)

	Ping(ctx context.Context) error
}

type redisTimelineCache struct {
	client *redis.Client
}

// NewRedisTimelineCache returns a Redis-backed cache. A nil client produces a
// no-op implementation so the service can still boot and use pull fallback.
func NewRedisTimelineCache(client *redis.Client) TimelineCache {
	if client == nil {
		return &noopTimelineCache{}
	}
	return &redisTimelineCache{client: client}
}

func timelineKey(userID string) string {
	return "timeline:" + userID
}

func celebrityRecentKey(authorID string) string {
	return "celebrity:recent:" + authorID
}

func activeUserKey(userID string) string {
	return "user:active:" + userID
}

func toRedisScore(score int64) float64 {
	return float64(score)
}

func fromRedisScore(score float64) int64 {
	return int64(math.Round(score))
}

// GetTimeline reads up to limit members in reverse chronological order.
// maxScore is inclusive; the service layer removes the cursor item itself and
// resolves same-timestamp ties with the post ID.
func (c *redisTimelineCache) GetTimeline(ctx context.Context, userID string, maxScore int64, limit int64) ([]models.TimelineItem, error) {
	if limit <= 0 {
		return []models.TimelineItem{}, nil
	}

	max := "+inf"
	if maxScore > 0 {
		max = strconv.FormatInt(maxScore, 10)
	}

	results, err := c.client.ZRevRangeByScoreWithScores(ctx, timelineKey(userID), &redis.ZRangeBy{
		Min:    "-inf",
		Max:    max,
		Offset: 0,
		Count:  limit,
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to query timeline for %s: %w", userID, err)
	}

	items := make([]models.TimelineItem, 0, len(results))
	for _, z := range results {
		items = append(items, models.TimelineItem{
			PostID: fmt.Sprintf("%v", z.Member),
			Score:  fromRedisScore(z.Score),
		})
	}

	return items, nil
}

// PushToTimeline inserts one post into a user's timeline and trims it to the
// configured maximum size. Timeline TTL is refreshed on every write.
func (c *redisTimelineCache) PushToTimeline(ctx context.Context, userID, postID string, score int64, maxSize int64, ttl time.Duration) error {
	key := timelineKey(userID)
	pipe := c.client.Pipeline()

	pipe.ZAdd(ctx, key, redis.Z{Score: toRedisScore(score), Member: postID})
	if maxSize > 0 {
		pipe.ZRemRangeByRank(ctx, key, 0, -(maxSize + 1))
	}
	if ttl > 0 {
		pipe.Expire(ctx, key, ttl)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to push post %s to timeline %s: %w", postID, userID, err)
	}
	return nil
}

// PushBatch fans one post out to many followers using chunked pipelines. This
// is the write-amplification hot path, so network round-trips are amortised.
func (c *redisTimelineCache) PushBatch(ctx context.Context, userIDs []string, postID string, score int64, maxSize int64, ttl time.Duration) error {
	if len(userIDs) == 0 {
		return nil
	}

	for start := 0; start < len(userIDs); start += timelineChunkSize {
		end := start + timelineChunkSize
		if end > len(userIDs) {
			end = len(userIDs)
		}

		pipe := c.client.Pipeline()
		for _, userID := range userIDs[start:end] {
			key := timelineKey(userID)
			pipe.ZAdd(ctx, key, redis.Z{Score: toRedisScore(score), Member: postID})
			if maxSize > 0 {
				pipe.ZRemRangeByRank(ctx, key, 0, -(maxSize + 1))
			}
			if ttl > 0 {
				pipe.Expire(ctx, key, ttl)
			}
		}

		if _, err := pipe.Exec(ctx); err != nil {
			return fmt.Errorf("failed batch push of post %s: %w", postID, err)
		}
	}

	return nil
}

// BackfillTimeline injects a batch of historical items into a timeline. It is
// used on follow events and cold-start cache warming.
func (c *redisTimelineCache) BackfillTimeline(ctx context.Context, userID string, items []models.TimelineItem, maxSize int64, ttl time.Duration) error {
	if len(items) == 0 {
		return nil
	}

	key := timelineKey(userID)
	pipe := c.client.Pipeline()

	members := make([]redis.Z, len(items))
	for i, item := range items {
		members[i] = redis.Z{Score: toRedisScore(item.Score), Member: item.PostID}
	}

	pipe.ZAdd(ctx, key, members...)
	if maxSize > 0 {
		pipe.ZRemRangeByRank(ctx, key, 0, -(maxSize + 1))
	}
	if ttl > 0 {
		pipe.Expire(ctx, key, ttl)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to backfill timeline for %s: %w", userID, err)
	}
	return nil
}

// RemovePost removes a single ghost/deleted post reference.
func (c *redisTimelineCache) RemovePost(ctx context.Context, userID, postID string) error {
	if err := c.client.ZRem(ctx, timelineKey(userID), postID).Err(); err != nil {
		return fmt.Errorf("failed to remove post %s from timeline %s: %w", postID, userID, err)
	}
	return nil
}

// RemovePostsBatch removes many post references from one timeline in a single
// pipeline. Used when a user unfollows an author.
func (c *redisTimelineCache) RemovePostsBatch(ctx context.Context, userID string, postIDs []string) error {
	if len(postIDs) == 0 {
		return nil
	}

	key := timelineKey(userID)
	pipe := c.client.Pipeline()
	for _, postID := range postIDs {
		pipe.ZRem(ctx, key, postID)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to remove posts from timeline %s: %w", userID, err)
	}
	return nil
}

// InvalidateTimeline deletes the entire precomputed feed for a user.
func (c *redisTimelineCache) InvalidateTimeline(ctx context.Context, userID string) error {
	if err := c.client.Del(ctx, timelineKey(userID)).Err(); err != nil {
		return fmt.Errorf("failed to invalidate timeline for %s: %w", userID, err)
	}
	return nil
}

// StoreCelebrityPost keeps the latest posts of a high-follower author in a
// shared per-author cache. Followers pull from this key at read time.
func (c *redisTimelineCache) StoreCelebrityPost(ctx context.Context, authorID, postID string, score int64, maxItems int64, ttl time.Duration) error {
	key := celebrityRecentKey(authorID)
	pipe := c.client.Pipeline()

	pipe.ZAdd(ctx, key, redis.Z{Score: toRedisScore(score), Member: postID})
	if maxItems > 0 {
		pipe.ZRemRangeByRank(ctx, key, 0, -(maxItems + 1))
	}
	if ttl > 0 {
		pipe.Expire(ctx, key, ttl)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to store celebrity post for %s: %w", authorID, err)
	}
	return nil
}

// GetCelebrityRecentPostsBatch fetches recent celebrity posts for many authors
// in one pipelined round-trip. Authors without a key simply return no items.
func (c *redisTimelineCache) GetCelebrityRecentPostsBatch(ctx context.Context, authorIDs []string, limit int64) (map[string][]models.TimelineItem, error) {
	result := make(map[string][]models.TimelineItem, len(authorIDs))
	if len(authorIDs) == 0 || limit <= 0 {
		return result, nil
	}

	const chunkSize = 200
	for start := 0; start < len(authorIDs); start += chunkSize {
		end := start + chunkSize
		if end > len(authorIDs) {
			end = len(authorIDs)
		}
		chunk := authorIDs[start:end]

		pipe := c.client.Pipeline()
		commands := make([]*redis.ZSliceCmd, len(chunk))
		for i, authorID := range chunk {
			commands[i] = pipe.ZRevRangeWithScores(ctx, celebrityRecentKey(authorID), 0, limit-1)
		}

		if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
			return nil, fmt.Errorf("failed to fetch celebrity timelines: %w", err)
		}

		for i, command := range commands {
			items := make([]models.TimelineItem, 0, len(command.Val()))
			for _, z := range command.Val() {
				items = append(items, models.TimelineItem{
					PostID: fmt.Sprintf("%v", z.Member),
					Score:  fromRedisScore(z.Score),
				})
			}
			result[chunk[i]] = items
		}
	}

	return result, nil
}

// RemoveCelebrityPost removes a deleted post from an author's celebrity cache.
func (c *redisTimelineCache) RemoveCelebrityPost(ctx context.Context, authorID, postID string) error {
	if err := c.client.ZRem(ctx, celebrityRecentKey(authorID), postID).Err(); err != nil {
		return fmt.Errorf("failed to remove celebrity post %s: %w", postID, err)
	}
	return nil
}

// MarkUserActive refreshes the activity marker used to skip fanout writes to
// dormant accounts.
func (c *redisTimelineCache) MarkUserActive(ctx context.Context, userID string, ttl time.Duration) error {
	if ttl <= 0 {
		return nil
	}
	if err := c.client.Set(ctx, activeUserKey(userID), "1", ttl).Err(); err != nil {
		return fmt.Errorf("failed to mark user %s active: %w", userID, err)
	}
	return nil
}

// IsUserActiveBatch checks many activity markers with one pipelined round-trip.
func (c *redisTimelineCache) IsUserActiveBatch(ctx context.Context, userIDs []string) (map[string]bool, error) {
	active := make(map[string]bool, len(userIDs))
	if len(userIDs) == 0 {
		return active, nil
	}

	for start := 0; start < len(userIDs); start += timelineChunkSize {
		end := start + timelineChunkSize
		if end > len(userIDs) {
			end = len(userIDs)
		}
		chunk := userIDs[start:end]

		pipe := c.client.Pipeline()
		commands := make([]*redis.IntCmd, len(chunk))
		for i, userID := range chunk {
			commands[i] = pipe.Exists(ctx, activeUserKey(userID))
		}

		if _, err := pipe.Exec(ctx); err != nil {
			return nil, fmt.Errorf("failed to check active users: %w", err)
		}
		for i, command := range commands {
			active[chunk[i]] = command.Val() > 0
		}
	}

	return active, nil
}

// Ping verifies Redis connectivity.
func (c *redisTimelineCache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// noopTimelineCache keeps the service alive when Redis is unreachable. Reads
// report cache misses so the service falls back to the pull model; writes
// report an error so event consumers do not acknowledge undelivered fanout.
type noopTimelineCache struct{}

func (n *noopTimelineCache) GetTimeline(context.Context, string, int64, int64) ([]models.TimelineItem, error) {
	return nil, models.ErrCacheUnavailable
}

func (n *noopTimelineCache) PushToTimeline(context.Context, string, string, int64, int64, time.Duration) error {
	return models.ErrCacheUnavailable
}

func (n *noopTimelineCache) PushBatch(context.Context, []string, string, int64, int64, time.Duration) error {
	return models.ErrCacheUnavailable
}

func (n *noopTimelineCache) BackfillTimeline(context.Context, string, []models.TimelineItem, int64, time.Duration) error {
	return models.ErrCacheUnavailable
}

func (n *noopTimelineCache) RemovePost(context.Context, string, string) error {
	return nil
}

func (n *noopTimelineCache) RemovePostsBatch(context.Context, string, []string) error {
	return nil
}

func (n *noopTimelineCache) InvalidateTimeline(context.Context, string) error {
	return nil
}

func (n *noopTimelineCache) StoreCelebrityPost(context.Context, string, string, int64, int64, time.Duration) error {
	return models.ErrCacheUnavailable
}

func (n *noopTimelineCache) GetCelebrityRecentPostsBatch(context.Context, []string, int64) (map[string][]models.TimelineItem, error) {
	return nil, models.ErrCacheUnavailable
}

func (n *noopTimelineCache) RemoveCelebrityPost(context.Context, string, string) error {
	return nil
}

func (n *noopTimelineCache) MarkUserActive(context.Context, string, time.Duration) error {
	return nil
}

func (n *noopTimelineCache) IsUserActiveBatch(context.Context, []string) (map[string]bool, error) {
	return map[string]bool{}, nil
}

func (n *noopTimelineCache) Ping(context.Context) error {
	return models.ErrCacheUnavailable
}
