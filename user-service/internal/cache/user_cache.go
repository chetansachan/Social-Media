package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/chetansachan/socialflow/user-service/internal/models"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// ErrMiss reports that a key was not present. Callers should treat any other
// error the same way: a cache outage degrades to the database, never to a
// failed request.
var ErrMiss = errors.New("cache miss")

// UserCache stores public profile data. Passwords are never written: the User
// type marks Password as json:"-", so encoding the model cannot leak a hash.
type UserCache interface {
	Get(ctx context.Context, id uuid.UUID) (*models.User, error)
	GetMany(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]*models.User, error)
	Set(ctx context.Context, user *models.User, ttl time.Duration) error
	SetMany(ctx context.Context, users []models.User, ttl time.Duration) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// RedisUserCache is the shared profile cache. Keys are namespaced by version so
// a future encoding change can roll out without deleting the old keys.
type RedisUserCache struct {
	client *redis.Client
}

// NewRedisUserCache returns a cache backed by client.
func NewRedisUserCache(client *redis.Client) *RedisUserCache {
	return &RedisUserCache{client: client}
}

func userKey(id uuid.UUID) string {
	return "user:profile:v1:" + id.String()
}

// Get returns the cached profile or ErrMiss.
func (c *RedisUserCache) Get(ctx context.Context, id uuid.UUID) (*models.User, error) {
	encoded, err := c.client.Get(ctx, userKey(id)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrMiss
		}
		return nil, fmt.Errorf("read profile cache: %w", err)
	}

	var user models.User
	if err := json.Unmarshal(encoded, &user); err != nil {
		// A corrupt entry must not poison the key forever; drop it and miss.
		_ = c.client.Del(ctx, userKey(id)).Err()
		return nil, ErrMiss
	}
	user.Password = ""
	return &user, nil
}

// GetMany fetches every id in one round trip. Missing and corrupt entries are
// omitted from the result rather than returned as errors.
func (c *RedisUserCache) GetMany(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]*models.User, error) {
	found := make(map[uuid.UUID]*models.User, len(ids))
	if len(ids) == 0 {
		return found, nil
	}

	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = userKey(id)
	}

	values, err := c.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("read profile cache batch: %w", err)
	}

	for i, value := range values {
		encoded, ok := value.(string)
		if !ok || encoded == "" {
			continue
		}
		var user models.User
		if err := json.Unmarshal([]byte(encoded), &user); err != nil {
			_ = c.client.Del(ctx, keys[i]).Err()
			continue
		}
		user.Password = ""
		found[ids[i]] = &user
	}

	return found, nil
}

// Set stores one profile for ttl.
func (c *RedisUserCache) Set(ctx context.Context, user *models.User, ttl time.Duration) error {
	if user == nil || ttl <= 0 {
		return nil
	}
	encoded, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("encode profile cache: %w", err)
	}
	if err := c.client.Set(ctx, userKey(user.ID), encoded, ttl).Err(); err != nil {
		return fmt.Errorf("write profile cache: %w", err)
	}
	return nil
}

// SetMany stores a batch of profiles in a single pipeline.
func (c *RedisUserCache) SetMany(ctx context.Context, users []models.User, ttl time.Duration) error {
	if len(users) == 0 || ttl <= 0 {
		return nil
	}

	pipe := c.client.Pipeline()
	for i := range users {
		encoded, err := json.Marshal(&users[i])
		if err != nil {
			continue
		}
		pipe.Set(ctx, userKey(users[i].ID), encoded, ttl)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("write profile cache batch: %w", err)
	}
	return nil
}

// Delete removes one profile. It is called after every write that changes the
// profile so the next read repopulates from the primary-backed repository.
func (c *RedisUserCache) Delete(ctx context.Context, id uuid.UUID) error {
	if err := c.client.Del(ctx, userKey(id)).Err(); err != nil {
		return fmt.Errorf("delete profile cache: %w", err)
	}
	return nil
}
