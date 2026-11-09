package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/chetansachan/socialflow/notification-service/internal/models"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// CacheRepository is the Redis hot path: unread badges, idempotency locks,
// aggregation windows, realtime throttles, and push rate limits. All methods
// return models.ErrCacheUnavailable when Redis was not configured so callers
// can degrade instead of failing the request.
type CacheRepository interface {
	IncrUnread(ctx context.Context, userID uuid.UUID) (int64, error)
	GetUnread(ctx context.Context, userID uuid.UUID) (int64, bool, error)
	SetUnread(ctx context.Context, userID uuid.UUID, value int64) error
	DecrUnread(ctx context.Context, userID uuid.UUID, by int) (int64, error)
	ResetUnread(ctx context.Context, userID uuid.UUID) error

	AcquireDedup(ctx context.Context, key string, ttl time.Duration) (bool, error)
	ReleaseDedup(ctx context.Context, key string) error

	GetAggregation(ctx context.Context, key string) (string, bool, error)
	SetAggregation(ctx context.Context, key, notificationID string, ttl time.Duration) error
	ClearAggregation(ctx context.Context, key string) error
	RegisterAggregationActor(ctx context.Context, key, actorID string, ttl time.Duration) (bool, error)
	RemoveAggregationActor(ctx context.Context, key, actorID string, ttl time.Duration) (bool, error)

	ShouldThrottle(ctx context.Context, key string, window time.Duration) (bool, error)
	AllowPush(ctx context.Context, userID uuid.UUID, limit int, window time.Duration) (bool, error)
}

type cacheRepository struct {
	client    *redis.Client
	unreadTTL time.Duration
}

func NewCacheRepository(client *redis.Client, unreadTTL time.Duration) CacheRepository {
	if unreadTTL <= 0 {
		unreadTTL = 7 * 24 * time.Hour
	}
	return &cacheRepository{client: client, unreadTTL: unreadTTL}
}

func (r *cacheRepository) available() error {
	if r.client == nil {
		return models.ErrCacheUnavailable
	}
	return nil
}

func unreadKey(userID uuid.UUID) string {
	return "unread:" + userID.String()
}

func aggregationActorsKey(key string) string {
	return "agg:actors:" + key
}

// decrFloorScript atomically decrements and clamps at zero so double mark-read
// requests or retention pruning can never produce a negative badge.
var decrFloorScript = redis.NewScript(`
	local value = redis.call('DECRBY', KEYS[1], ARGV[1])
	if value < 0 then
		redis.call('SET', KEYS[1], 0)
		value = 0
	end
	redis.call('EXPIRE', KEYS[1], ARGV[2])
	return value
`)

func (r *cacheRepository) IncrUnread(ctx context.Context, userID uuid.UUID) (int64, error) {
	if err := r.available(); err != nil {
		return 0, err
	}
	pipe := r.client.TxPipeline()
	incr := pipe.Incr(ctx, unreadKey(userID))
	pipe.Expire(ctx, unreadKey(userID), r.unreadTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("failed to increment unread counter: %w", err)
	}
	return incr.Val(), nil
}

func (r *cacheRepository) GetUnread(ctx context.Context, userID uuid.UUID) (int64, bool, error) {
	if err := r.available(); err != nil {
		return 0, false, err
	}
	value, err := r.client.Get(ctx, unreadKey(userID)).Result()
	if errors.Is(err, redis.Nil) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("failed to read unread counter: %w", err)
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("unread counter is not an integer: %w", err)
	}
	return parsed, true, nil
}

func (r *cacheRepository) SetUnread(ctx context.Context, userID uuid.UUID, value int64) error {
	if err := r.available(); err != nil {
		return err
	}
	if value < 0 {
		value = 0
	}
	if err := r.client.Set(ctx, unreadKey(userID), value, r.unreadTTL).Err(); err != nil {
		return fmt.Errorf("failed to seed unread counter: %w", err)
	}
	return nil
}

func (r *cacheRepository) DecrUnread(ctx context.Context, userID uuid.UUID, by int) (int64, error) {
	if err := r.available(); err != nil {
		return 0, err
	}
	if by <= 0 {
		return r.getUnreadValue(ctx, userID)
	}
	value, err := decrFloorScript.Run(ctx, r.client, []string{unreadKey(userID)}, by, int(r.unreadTTL.Seconds())).Int64()
	if err != nil {
		return 0, fmt.Errorf("failed to decrement unread counter: %w", err)
	}
	return value, nil
}

func (r *cacheRepository) getUnreadValue(ctx context.Context, userID uuid.UUID) (int64, error) {
	value, _, err := r.GetUnread(ctx, userID)
	return value, err
}

func (r *cacheRepository) ResetUnread(ctx context.Context, userID uuid.UUID) error {
	if err := r.available(); err != nil {
		return err
	}
	if err := r.client.Set(ctx, unreadKey(userID), 0, r.unreadTTL).Err(); err != nil {
		return fmt.Errorf("failed to reset unread counter: %w", err)
	}
	return nil
}

func (r *cacheRepository) AcquireDedup(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if err := r.available(); err != nil {
		return false, err
	}
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	acquired, err := r.client.SetNX(ctx, key, "1", ttl).Result()
	if err != nil {
		return false, fmt.Errorf("failed to acquire dedup lock: %w", err)
	}
	return acquired, nil
}

// ReleaseDedup removes an idempotency lock when processing failed after the
// lock was acquired, so the broker retry is not silently suppressed.
func (r *cacheRepository) ReleaseDedup(ctx context.Context, key string) error {
	if err := r.available(); err != nil {
		return err
	}
	if err := r.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to release dedup lock: %w", err)
	}
	return nil
}

func (r *cacheRepository) GetAggregation(ctx context.Context, key string) (string, bool, error) {
	if err := r.available(); err != nil {
		return "", false, err
	}
	value, err := r.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("failed to read aggregation key: %w", err)
	}
	return value, true, nil
}

func (r *cacheRepository) SetAggregation(ctx context.Context, key, notificationID string, ttl time.Duration) error {
	if err := r.available(); err != nil {
		return err
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	if err := r.client.Set(ctx, key, notificationID, ttl).Err(); err != nil {
		return fmt.Errorf("failed to set aggregation key: %w", err)
	}
	return nil
}

func (r *cacheRepository) ClearAggregation(ctx context.Context, key string) error {
	if err := r.available(); err != nil {
		return err
	}
	if err := r.client.Del(ctx, key, aggregationActorsKey(key)).Err(); err != nil {
		return fmt.Errorf("failed to clear aggregation state: %w", err)
	}
	return nil
}

// RegisterAggregationActor adds an actor to the set backing one aggregation
// window. The boolean is false when the actor was already present, which
// prevents the same like being counted twice after the short dedup TTL expires.
func (r *cacheRepository) RegisterAggregationActor(ctx context.Context, key, actorID string, ttl time.Duration) (bool, error) {
	if err := r.available(); err != nil {
		return false, err
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	actorKey := aggregationActorsKey(key)
	added, err := r.client.SAdd(ctx, actorKey, actorID).Result()
	if err != nil {
		return false, fmt.Errorf("failed to register aggregation actor: %w", err)
	}
	_ = r.client.Expire(ctx, actorKey, ttl).Err()
	return added > 0, nil
}

func (r *cacheRepository) RemoveAggregationActor(ctx context.Context, key, actorID string, ttl time.Duration) (bool, error) {
	if err := r.available(); err != nil {
		return false, err
	}
	actorKey := aggregationActorsKey(key)
	removed, err := r.client.SRem(ctx, actorKey, actorID).Result()
	if err != nil {
		return false, fmt.Errorf("failed to remove aggregation actor: %w", err)
	}
	if ttl > 0 {
		_ = r.client.Expire(ctx, actorKey, ttl).Err()
	}
	return removed > 0, nil
}

// ShouldThrottle returns true when the caller should skip a realtime update
// because the throttle key already exists.
func (r *cacheRepository) ShouldThrottle(ctx context.Context, key string, window time.Duration) (bool, error) {
	if err := r.available(); err != nil {
		return false, err
	}
	if window <= 0 {
		window = 30 * time.Second
	}
	acquired, err := r.client.SetNX(ctx, key, "1", window).Result()
	if err != nil {
		return false, fmt.Errorf("failed to evaluate realtime throttle: %w", err)
	}
	return !acquired, nil
}

// AllowPush implements a fixed-window per-user push budget. Security alerts
// bypass this in the policy service.
func (r *cacheRepository) AllowPush(ctx context.Context, userID uuid.UUID, limit int, window time.Duration) (bool, error) {
	if err := r.available(); err != nil {
		return false, err
	}
	if limit <= 0 {
		return true, nil
	}
	if window <= 0 {
		window = time.Hour
	}

	bucket := time.Now().UTC().Unix() / int64(window.Seconds())
	key := fmt.Sprintf("push:rate:%s:%d", userID.String(), bucket)

	pipe := r.client.TxPipeline()
	count := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, window)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, fmt.Errorf("failed to evaluate push rate limit: %w", err)
	}
	return count.Val() <= int64(limit), nil
}
