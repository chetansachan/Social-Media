package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/chetansachan/socialflow/notification-service/internal/models"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func newTestCache(t *testing.T) (CacheRepository, *miniredis.Miniredis) {
	t.Helper()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return NewCacheRepository(client, time.Hour), server
}

func TestCacheUnreadCounter(t *testing.T) {
	t.Parallel()

	cache, _ := newTestCache(t)
	ctx := context.Background()
	userID := uuid.New()

	if _, found, err := cache.GetUnread(ctx, userID); err != nil || found {
		t.Fatalf("cold counter: found=%v err=%v", found, err)
	}

	if value, err := cache.IncrUnread(ctx, userID); err != nil || value != 1 {
		t.Fatalf("first increment = %d, %v", value, err)
	}
	if value, err := cache.IncrUnread(ctx, userID); err != nil || value != 2 {
		t.Fatalf("second increment = %d, %v", value, err)
	}

	if value, found, err := cache.GetUnread(ctx, userID); err != nil || !found || value != 2 {
		t.Fatalf("GetUnread = %d, %v, %v", value, found, err)
	}

	if value, err := cache.DecrUnread(ctx, userID, 1); err != nil || value != 1 {
		t.Fatalf("decrement = %d, %v", value, err)
	}
	if value, err := cache.DecrUnread(ctx, userID, 5); err != nil || value != 0 {
		t.Fatalf("floor decrement = %d, %v", value, err)
	}
	if value, err := cache.DecrUnread(ctx, userID, 1); err != nil || value != 0 {
		t.Fatalf("decrement below zero = %d, %v", value, err)
	}

	if err := cache.ResetUnread(ctx, userID); err != nil {
		t.Fatalf("ResetUnread: %v", err)
	}
	if value, found, err := cache.GetUnread(ctx, userID); err != nil || !found || value != 0 {
		t.Fatalf("after reset = %d, %v, %v", value, found, err)
	}
}

func TestCacheDedupLock(t *testing.T) {
	t.Parallel()

	cache, _ := newTestCache(t)
	ctx := context.Background()

	acquired, err := cache.AcquireDedup(ctx, "dedup:test", time.Minute)
	if err != nil || !acquired {
		t.Fatalf("first acquire = %v, %v", acquired, err)
	}

	acquired, err = cache.AcquireDedup(ctx, "dedup:test", time.Minute)
	if err != nil || acquired {
		t.Fatalf("second acquire = %v, %v", acquired, err)
	}

	if err := cache.ReleaseDedup(ctx, "dedup:test"); err != nil {
		t.Fatalf("release: %v", err)
	}
	acquired, err = cache.AcquireDedup(ctx, "dedup:test", time.Minute)
	if err != nil || !acquired {
		t.Fatalf("acquire after release = %v, %v", acquired, err)
	}
}

func TestCacheAggregationState(t *testing.T) {
	t.Parallel()

	cache, _ := newTestCache(t)
	ctx := context.Background()

	if _, found, err := cache.GetAggregation(ctx, "agg:test"); err != nil || found {
		t.Fatalf("cold aggregation: found=%v err=%v", found, err)
	}

	if err := cache.SetAggregation(ctx, "agg:test", "notification-1", time.Minute); err != nil {
		t.Fatalf("SetAggregation: %v", err)
	}
	value, found, err := cache.GetAggregation(ctx, "agg:test")
	if err != nil || !found || value != "notification-1" {
		t.Fatalf("GetAggregation = %q, %v, %v", value, found, err)
	}

	added, err := cache.RegisterAggregationActor(ctx, "agg:test", "actor-1", time.Minute)
	if err != nil || !added {
		t.Fatalf("first actor = %v, %v", added, err)
	}
	added, err = cache.RegisterAggregationActor(ctx, "agg:test", "actor-1", time.Minute)
	if err != nil || added {
		t.Fatalf("duplicate actor = %v, %v", added, err)
	}

	removed, err := cache.RemoveAggregationActor(ctx, "agg:test", "actor-1", time.Minute)
	if err != nil || !removed {
		t.Fatalf("remove actor = %v, %v", removed, err)
	}
	removed, err = cache.RemoveAggregationActor(ctx, "agg:test", "actor-1", time.Minute)
	if err != nil || removed {
		t.Fatalf("remove missing actor = %v, %v", removed, err)
	}

	if err := cache.ClearAggregation(ctx, "agg:test"); err != nil {
		t.Fatalf("ClearAggregation: %v", err)
	}
	if _, found, err := cache.GetAggregation(ctx, "agg:test"); err != nil || found {
		t.Fatalf("aggregation should be cleared: found=%v err=%v", found, err)
	}
}

func TestCacheThrottleAndPushBudget(t *testing.T) {
	t.Parallel()

	cache, _ := newTestCache(t)
	ctx := context.Background()
	userID := uuid.New()

	throttled, err := cache.ShouldThrottle(ctx, "agg:rt:test", time.Minute)
	if err != nil || throttled {
		t.Fatalf("first throttle check = %v, %v", throttled, err)
	}
	throttled, err = cache.ShouldThrottle(ctx, "agg:rt:test", time.Minute)
	if err != nil || !throttled {
		t.Fatalf("second throttle check = %v, %v", throttled, err)
	}

	for i := 0; i < 2; i++ {
		allowed, err := cache.AllowPush(ctx, userID, 2, time.Hour)
		if err != nil || !allowed {
			t.Fatalf("push %d should be allowed: %v, %v", i, allowed, err)
		}
	}
	allowed, err := cache.AllowPush(ctx, userID, 2, time.Hour)
	if err != nil || allowed {
		t.Fatalf("third push should be limited: %v, %v", allowed, err)
	}
}

func TestCacheUnavailable(t *testing.T) {
	t.Parallel()

	cache := NewCacheRepository(nil, time.Hour)
	ctx := context.Background()

	if _, err := cache.IncrUnread(ctx, uuid.New()); !errors.Is(err, models.ErrCacheUnavailable) {
		t.Fatalf("expected ErrCacheUnavailable, got %v", err)
	}
	if _, _, err := cache.GetUnread(ctx, uuid.New()); !errors.Is(err, models.ErrCacheUnavailable) {
		t.Fatalf("expected ErrCacheUnavailable, got %v", err)
	}
	if _, err := cache.AcquireDedup(ctx, "key", time.Minute); !errors.Is(err, models.ErrCacheUnavailable) {
		t.Fatalf("expected ErrCacheUnavailable, got %v", err)
	}
}
