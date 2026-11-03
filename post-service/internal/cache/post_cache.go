package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/chetansachan/socialflow/post-service/internal/models"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// ErrMiss reports that a key was not present. Any other error is treated the
// same way by the service: the cache is an optimization, never a dependency.
var ErrMiss = errors.New("cache miss")

// PostCache stores post detail reads. Keys are versioned so the encoding can
// change without colliding with entries written by an older build.
type PostCache interface {
	Get(ctx context.Context, id uuid.UUID) (*models.Post, error)
	Set(ctx context.Context, post *models.Post, ttl time.Duration) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// RedisPostCache is the shared post detail cache.
type RedisPostCache struct {
	client *redis.Client
}

// NewRedisPostCache returns a cache backed by client.
func NewRedisPostCache(client *redis.Client) *RedisPostCache {
	return &RedisPostCache{client: client}
}

func postKey(id uuid.UUID) string {
	return "post:detail:v1:" + id.String()
}

// Get returns the cached post or ErrMiss.
func (c *RedisPostCache) Get(ctx context.Context, id uuid.UUID) (*models.Post, error) {
	encoded, err := c.client.Get(ctx, postKey(id)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrMiss
		}
		return nil, fmt.Errorf("read post cache: %w", err)
	}

	var post models.Post
	if err := json.Unmarshal(encoded, &post); err != nil {
		// Do not let a corrupt value pin the key; drop it and report a miss.
		_ = c.client.Del(ctx, postKey(id)).Err()
		return nil, ErrMiss
	}
	return &post, nil
}

// Set stores one post for ttl.
func (c *RedisPostCache) Set(ctx context.Context, post *models.Post, ttl time.Duration) error {
	if post == nil || ttl <= 0 {
		return nil
	}
	encoded, err := json.Marshal(post)
	if err != nil {
		return fmt.Errorf("encode post cache: %w", err)
	}
	if err := c.client.Set(ctx, postKey(post.ID), encoded, ttl).Err(); err != nil {
		return fmt.Errorf("write post cache: %w", err)
	}
	return nil
}

// Delete drops a post after any mutation. Like counts, edits, archive state,
// and deletion all change what a detail read would return.
func (c *RedisPostCache) Delete(ctx context.Context, id uuid.UUID) error {
	if err := c.client.Del(ctx, postKey(id)).Err(); err != nil {
		return fmt.Errorf("delete post cache: %w", err)
	}
	return nil
}
