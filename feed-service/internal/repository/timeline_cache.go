package repository

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/chetansachan/socialflow/feed-service/internal/models"
	"github.com/redis/go-redis/v9"
)

// TimelineCache defines the interface for interacting with timeline in-memory caches.
type TimelineCache interface {
	GetTimeline(ctx context.Context, userID string, maxScore float64, limit int64) ([]models.TimelineItem, error)
	PushToTimeline(ctx context.Context, userID string, postID string, score float64, maxSize int64, ttl time.Duration) error
	PushBatch(ctx context.Context, userIDs []string, postID string, score float64, maxSize int64, ttl time.Duration) error
	RemovePost(ctx context.Context, userID string, postID string) error
	RemovePostBatch(ctx context.Context, userIDs []string, postID string) error
	InvalidateTimeline(ctx context.Context, userID string) error
	BackfillTimeline(ctx context.Context, userID string, items []models.TimelineItem, maxSize int64, ttl time.Duration) error
	StoreCelebrityPost(ctx context.Context, authorID string, postID string, score float64, maxItems int64, ttl time.Duration) error
	GetCelebrityRecentPosts(ctx context.Context, authorID string, limit int64) ([]models.TimelineItem, error)
	MarkUserActive(ctx context.Context, userID string, ttl time.Duration) error
	IsUserActive(ctx context.Context, userID string) (bool, error)
	Ping(ctx context.Context) error
}

type redisTimelineCache struct {
	client *redis.Client
}

// NewRedisTimelineCache creates a new Redis-backed timeline cache.
func NewRedisTimelineCache(client *redis.Client) TimelineCache {
	if client == nil {
		return &noopTimelineCache{}
	}
	return &redisTimelineCache{client: client}
}

func timelineKey(userID string) string {
	return fmt.Sprintf("timeline:%s", userID)
}

func celebrityRecentKey(authorID string) string {
	return fmt.Sprintf("celebrity:recent:%s", authorID)
}

func activeUserKey(userID string) string {
	return fmt.Sprintf("user:active:%s", userID)
}

// GetTimeline retrieves a slice of post IDs and timestamps from the user's Redis sorted set.
func (c *redisTimelineCache) GetTimeline(ctx context.Context, userID string, maxScore float64, limit int64) ([]models.TimelineItem, error) {
	key := timelineKey(userID)

	maxStr := "+inf"
	if maxScore > 0 {
		// Use '(' for exclusive upper bound when cursor is given
		maxStr = fmt.Sprintf("(%f", maxScore)
	}

	opt := &redis.ZRangeBy{
		Min:    "-inf",
		Max:    maxStr,
		Offset: 0,
		Count:  limit,
	}

	results, err := c.client.ZRevRangeByScoreWithScores(ctx, key, opt).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to query timeline from redis: %w", err)
	}

	items := make([]models.TimelineItem, 0, len(results))
	for _, z := range results {
		postID, ok := z.Member.(string)
		if !ok {
			postID = fmt.Sprintf("%v", z.Member)
		}
		items = append(items, models.TimelineItem{
			PostID: postID,
			Score:  z.Score,
		})
	}

	return items, nil
}

// PushToTimeline pushes a post ID into a single user's timeline and trims it to maxSize.
func (c *redisTimelineCache) PushToTimeline(ctx context.Context, userID string, postID string, score float64, maxSize int64, ttl time.Duration) error {
	key := timelineKey(userID)
	pipe := c.client.Pipeline()

	pipe.ZAdd(ctx, key, redis.Z{
		Score:  score,
		Member: postID,
	})

	if maxSize > 0 {
		// Keep top maxSize items; remove items below top maxSize
		pipe.ZRemRangeByRank(ctx, key, 0, -(maxSize + 1))
	}

	if ttl > 0 {
		pipe.Expire(ctx, key, ttl)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to push post to timeline %s: %w", userID, err)
	}

	return nil
}

// PushBatch uses pipelining to batch push a post into multiple followers' timelines.
func (c *redisTimelineCache) PushBatch(ctx context.Context, userIDs []string, postID string, score float64, maxSize int64, ttl time.Duration) error {
	if len(userIDs) == 0 {
		return nil
	}

	// Chunk large follower batches to prevent excessive buffer allocation
	const chunkSize = 500
	for i := 0; i < len(userIDs); i += chunkSize {
		end := i + chunkSize
		if end > len(userIDs) {
			end = len(userIDs)
		}
		chunk := userIDs[i:end]

		pipe := c.client.Pipeline()
		for _, uid := range chunk {
			key := timelineKey(uid)
			pipe.ZAdd(ctx, key, redis.Z{
				Score:  score,
				Member: postID,
			})
			if maxSize > 0 {
				pipe.ZRemRangeByRank(ctx, key, 0, -(maxSize + 1))
			}
			if ttl > 0 {
				pipe.Expire(ctx, key, ttl)
			}
		}

		if _, err := pipe.Exec(ctx); err != nil {
			return fmt.Errorf("failed executing batch timeline push: %w", err)
		}
	}

	return nil
}

// RemovePost removes a specific post from a user's timeline cache.
func (c *redisTimelineCache) RemovePost(ctx context.Context, userID string, postID string) error {
	key := timelineKey(userID)
	if err := c.client.ZRem(ctx, key, postID).Err(); err != nil {
		return fmt.Errorf("failed to remove post %s from timeline %s: %w", postID, userID, err)
	}
	return nil
}

// RemovePostBatch removes a post from multiple users' timeline caches.
func (c *redisTimelineCache) RemovePostBatch(ctx context.Context, userIDs []string, postID string) error {
	if len(userIDs) == 0 {
		return nil
	}

	pipe := c.client.Pipeline()
	for _, uid := range userIDs {
		pipe.ZRem(ctx, timelineKey(uid), postID)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed executing batch post removal: %w", err)
	}
	return nil
}

// InvalidateTimeline deletes the user's timeline key completely.
func (c *redisTimelineCache) InvalidateTimeline(ctx context.Context, userID string) error {
	key := timelineKey(userID)
	if err := c.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to invalidate timeline for user %s: %w", userID, err)
	}
	return nil
}

// BackfillTimeline injects multiple items into a user's timeline cache.
func (c *redisTimelineCache) BackfillTimeline(ctx context.Context, userID string, items []models.TimelineItem, maxSize int64, ttl time.Duration) error {
	if len(items) == 0 {
		return nil
	}

	key := timelineKey(userID)
	pipe := c.client.Pipeline()

	zMembers := make([]redis.Z, len(items))
	for i, item := range items {
		zMembers[i] = redis.Z{
			Score:  item.Score,
			Member: item.PostID,
		}
	}

	pipe.ZAdd(ctx, key, zMembers...)
	if maxSize > 0 {
		pipe.ZRemRangeByRank(ctx, key, 0, -(maxSize + 1))
	}
	if ttl > 0 {
		pipe.Expire(ctx, key, ttl)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to backfill timeline for user %s: %w", userID, err)
	}

	return nil
}

// StoreCelebrityPost stores a celebrity's recent post in their dedicated public recent feed.
func (c *redisTimelineCache) StoreCelebrityPost(ctx context.Context, authorID string, postID string, score float64, maxItems int64, ttl time.Duration) error {
	key := celebrityRecentKey(authorID)
	pipe := c.client.Pipeline()

	pipe.ZAdd(ctx, key, redis.Z{
		Score:  score,
		Member: postID,
	})
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

// GetCelebrityRecentPosts returns recent posts for a celebrity account.
func (c *redisTimelineCache) GetCelebrityRecentPosts(ctx context.Context, authorID string, limit int64) ([]models.TimelineItem, error) {
	key := celebrityRecentKey(authorID)
	results, err := c.client.ZRevRangeWithScores(ctx, key, 0, limit-1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get celebrity recent posts: %w", err)
	}

	items := make([]models.TimelineItem, 0, len(results))
	for _, z := range results {
		postID, ok := z.Member.(string)
		if !ok {
			postID = fmt.Sprintf("%v", z.Member)
		}
		items = append(items, models.TimelineItem{
			PostID: postID,
			Score:  z.Score,
		})
	}

	return items, nil
}

// MarkUserActive records that a user has been active on the platform.
func (c *redisTimelineCache) MarkUserActive(ctx context.Context, userID string, ttl time.Duration) error {
	key := activeUserKey(userID)
	now := time.Now().Unix()
	return c.client.Set(ctx, key, strconv.FormatInt(now, 10), ttl).Err()
}

// IsUserActive checks whether a user has been active recently.
func (c *redisTimelineCache) IsUserActive(ctx context.Context, userID string) (bool, error) {
	key := activeUserKey(userID)
	exists, err := c.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}

// Ping checks Redis connectivity.
func (c *redisTimelineCache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// noopTimelineCache is a fallback implementation when Redis is not available.
type noopTimelineCache struct{}

func (n *noopTimelineCache) GetTimeline(ctx context.Context, userID string, maxScore float64, limit int64) ([]models.TimelineItem, error) {
	return nil, models.ErrCacheUnavailable
}
func (n *noopTimelineCache) PushToTimeline(ctx context.Context, userID string, postID string, score float64, maxSize int64, ttl time.Duration) error {
	return nil
}
func (n *noopTimelineCache) PushBatch(ctx context.Context, userIDs []string, postID string, score float64, maxSize int64, ttl time.Duration) error {
	return nil
}
func (n *noopTimelineCache) RemovePost(ctx context.Context, userID string, postID string) error {
	return nil
}
func (n *noopTimelineCache) RemovePostBatch(ctx context.Context, userIDs []string, postID string) error {
	return nil
}
func (n *noopTimelineCache) InvalidateTimeline(ctx context.Context, userID string) error {
	return nil
}
func (n *noopTimelineCache) BackfillTimeline(ctx context.Context, userID string, items []models.TimelineItem, maxSize int64, ttl time.Duration) error {
	return nil
}
func (n *noopTimelineCache) StoreCelebrityPost(ctx context.Context, authorID string, postID string, score float64, maxItems int64, ttl time.Duration) error {
	return nil
}
func (n *noopTimelineCache) GetCelebrityRecentPosts(ctx context.Context, authorID string, limit int64) ([]models.TimelineItem, error) {
	return nil, models.ErrCacheUnavailable
}
func (n *noopTimelineCache) MarkUserActive(ctx context.Context, userID string, ttl time.Duration) error {
	return nil
}
func (n *noopTimelineCache) IsUserActive(ctx context.Context, userID string) (bool, error) {
	return true, nil // default to active when cache is unavailable
}
func (n *noopTimelineCache) Ping(ctx context.Context) error {
	return models.ErrCacheUnavailable
}
