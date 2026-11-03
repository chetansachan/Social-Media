package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/chetansachan/socialflow/user-service/internal/models"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func newTestCache(t *testing.T) (*miniredis.Miniredis, *RedisUserCache) {
	t.Helper()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return server, NewRedisUserCache(client)
}

func testUser() *models.User {
	updated := time.Now().UTC().Add(-time.Hour)
	return &models.User{
		ID:        uuid.New(),
		Username:  "chetan",
		Name:      "Chetan Sachan",
		Email:     "chetan@example.com",
		Password:  "$2a$12$not-a-real-hash",
		IsPublic:  true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: &updated,
	}
}

func TestRedisUserCacheRoundTrip(t *testing.T) {
	_, cache := newTestCache(t)
	ctx := context.Background()
	user := testUser()

	if err := cache.Set(ctx, user, time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}

	cached, err := cache.Get(ctx, user.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cached.ID != user.ID || cached.Username != user.Username || cached.Email != user.Email {
		t.Fatalf("cached profile does not match: %+v", cached)
	}
	if cached.Password != "" {
		t.Fatal("password was stored in or served from the cache")
	}
	if cached.UpdatedAt == nil || !cached.UpdatedAt.Equal(*user.UpdatedAt) {
		t.Fatal("updated_at did not survive the round trip")
	}
}

func TestRedisUserCacheMiss(t *testing.T) {
	_, cache := newTestCache(t)

	if _, err := cache.Get(context.Background(), uuid.New()); !errors.Is(err, ErrMiss) {
		t.Fatalf("Get = %v, want ErrMiss", err)
	}
}

func TestRedisUserCacheExpires(t *testing.T) {
	server, cache := newTestCache(t)
	ctx := context.Background()
	user := testUser()

	if err := cache.Set(ctx, user, time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	server.FastForward(2 * time.Minute)

	if _, err := cache.Get(ctx, user.ID); !errors.Is(err, ErrMiss) {
		t.Fatalf("Get after expiry = %v, want ErrMiss", err)
	}
}

func TestRedisUserCacheBatch(t *testing.T) {
	_, cache := newTestCache(t)
	ctx := context.Background()

	first := testUser()
	second := testUser()
	second.Username = "second"

	if err := cache.SetMany(ctx, []models.User{*first, *second}, time.Minute); err != nil {
		t.Fatalf("SetMany: %v", err)
	}

	missing := uuid.New()
	found, err := cache.GetMany(ctx, []uuid.UUID{first.ID, second.ID, missing})
	if err != nil {
		t.Fatalf("GetMany: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("found %d profiles, want 2", len(found))
	}
	if found[first.ID] == nil || found[second.ID] == nil {
		t.Fatal("a populated id was dropped")
	}
	if _, ok := found[missing]; ok {
		t.Fatal("a missing id was reported present")
	}
}

func TestRedisUserCacheDelete(t *testing.T) {
	_, cache := newTestCache(t)
	ctx := context.Background()
	user := testUser()

	if err := cache.Set(ctx, user, time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := cache.Delete(ctx, user.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := cache.Get(ctx, user.ID); !errors.Is(err, ErrMiss) {
		t.Fatalf("Get after Delete = %v, want ErrMiss", err)
	}
}

func TestRedisUserCacheDropsCorruptEntry(t *testing.T) {
	server, cache := newTestCache(t)
	ctx := context.Background()
	user := testUser()

	// Simulate a stale writer or a manual edit that produced invalid JSON.
	server.Set(userKey(user.ID), "{not-json")

	if _, err := cache.Get(ctx, user.ID); !errors.Is(err, ErrMiss) {
		t.Fatalf("Get = %v, want ErrMiss", err)
	}
	if server.Exists(userKey(user.ID)) {
		t.Fatal("a corrupt entry was left in place")
	}
}
