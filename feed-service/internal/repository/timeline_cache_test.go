package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chetansachan/socialflow/feed-service/internal/models"
)

func TestNoopTimelineCache(t *testing.T) {
	cache := NewRedisTimelineCache(nil)
	ctx := context.Background()

	if err := cache.Ping(ctx); !errors.Is(err, models.ErrCacheUnavailable) {
		t.Fatalf("expected ErrCacheUnavailable from Ping, got %v", err)
	}
	if _, err := cache.GetTimeline(ctx, "user", 0, 10); !errors.Is(err, models.ErrCacheUnavailable) {
		t.Fatalf("expected cache miss signal, got %v", err)
	}
	if err := cache.PushToTimeline(ctx, "user", "post", 1, 800, time.Hour); !errors.Is(err, models.ErrCacheUnavailable) {
		t.Fatalf("expected fanout write to fail loudly, got %v", err)
	}
	if err := cache.InvalidateTimeline(ctx, "user"); err != nil {
		t.Fatalf("invalidation should be a safe no-op, got %v", err)
	}
}

func TestKeyFormatters(t *testing.T) {
	userID := "11111111-2222-3333-4444-555555555555"
	if got := timelineKey(userID); got != "timeline:"+userID {
		t.Fatalf("timelineKey = %s", got)
	}
	if got := celebrityRecentKey(userID); got != "celebrity:recent:"+userID {
		t.Fatalf("celebrityRecentKey = %s", got)
	}
	if got := activeUserKey(userID); got != "user:active:"+userID {
		t.Fatalf("activeUserKey = %s", got)
	}
}
