package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/chetansachan/socialflow/post-service/internal/models"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func newTestCache(t *testing.T) (*miniredis.Miniredis, *RedisPostCache) {
	t.Helper()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return server, NewRedisPostCache(client)
}

func testPost() *models.Post {
	updated := time.Now().UTC().Add(-time.Minute)
	return &models.Post{
		ID:        uuid.New(),
		AuthorID:  uuid.New(),
		Content:   "hello world",
		MediaURL:  "https://res.cloudinary.com/demo/image/upload/v1/post.jpg",
		IsPublic:  true,
		LikeCount: 42,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: &updated,
	}
}

func TestRedisPostCacheRoundTrip(t *testing.T) {
	_, cache := newTestCache(t)
	ctx := context.Background()
	post := testPost()

	if err := cache.Set(ctx, post, time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}

	cached, err := cache.Get(ctx, post.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cached.ID != post.ID || cached.AuthorID != post.AuthorID {
		t.Fatalf("cached post does not match: %+v", cached)
	}
	if cached.Content != post.Content || cached.LikeCount != post.LikeCount {
		t.Fatalf("cached fields do not match: %+v", cached)
	}
	if cached.UpdatedAt == nil || !cached.UpdatedAt.Equal(*post.UpdatedAt) {
		t.Fatal("updated_at did not survive the round trip")
	}
}

func TestRedisPostCacheMiss(t *testing.T) {
	_, cache := newTestCache(t)

	if _, err := cache.Get(context.Background(), uuid.New()); !errors.Is(err, ErrMiss) {
		t.Fatalf("Get = %v, want ErrMiss", err)
	}
}

func TestRedisPostCacheExpires(t *testing.T) {
	server, cache := newTestCache(t)
	ctx := context.Background()
	post := testPost()

	if err := cache.Set(ctx, post, time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	server.FastForward(2 * time.Minute)

	if _, err := cache.Get(ctx, post.ID); !errors.Is(err, ErrMiss) {
		t.Fatalf("Get after expiry = %v, want ErrMiss", err)
	}
}

func TestRedisPostCacheDelete(t *testing.T) {
	_, cache := newTestCache(t)
	ctx := context.Background()
	post := testPost()

	if err := cache.Set(ctx, post, time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := cache.Delete(ctx, post.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := cache.Get(ctx, post.ID); !errors.Is(err, ErrMiss) {
		t.Fatalf("Get after Delete = %v, want ErrMiss", err)
	}
}

func TestRedisPostCacheDropsCorruptEntry(t *testing.T) {
	server, cache := newTestCache(t)
	ctx := context.Background()
	post := testPost()

	server.Set(postKey(post.ID), "{not-json")

	if _, err := cache.Get(ctx, post.ID); !errors.Is(err, ErrMiss) {
		t.Fatalf("Get = %v, want ErrMiss", err)
	}
	if server.Exists(postKey(post.ID)) {
		t.Fatal("a corrupt entry was left in place")
	}
}
