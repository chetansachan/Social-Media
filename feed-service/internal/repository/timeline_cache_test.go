package repository

import (
	"context"
	"testing"
	"time"

	"github.com/chetansachan/socialflow/feed-service/internal/models"
)

func TestNoopTimelineCache(t *testing.T) {
	cache := NewRedisTimelineCache(nil)
	ctx := context.Background()

	// Verify Ping returns ErrCacheUnavailable
	if err := cache.Ping(ctx); err != models.ErrCacheUnavailable {
		t.Errorf("expected ErrCacheUnavailable on ping, got %v", err)
	}

	// Verify GetTimeline returns ErrCacheUnavailable
	_, err := cache.GetTimeline(ctx, "user-123", 0, 10)
	if err != models.ErrCacheUnavailable {
		t.Errorf("expected ErrCacheUnavailable on GetTimeline, got %v", err)
	}

	// Verify Push operations complete without error
	if err := cache.PushToTimeline(ctx, "user-123", "post-456", 12345, 800, time.Hour); err != nil {
		t.Errorf("expected nil error on PushToTimeline, got %v", err)
	}

	if err := cache.PushBatch(ctx, []string{"u1", "u2"}, "post-1", 100, 800, time.Hour); err != nil {
		t.Errorf("expected nil error on PushBatch, got %v", err)
	}

	// Verify IsUserActive defaults to true on fallback
	active, err := cache.IsUserActive(ctx, "user-123")
	if err != nil || !active {
		t.Errorf("expected active=true, got %v (err: %v)", active, err)
	}
}

func TestKeyFormatters(t *testing.T) {
	uid := "11111111-2222-3333-4444-555555555555"
	if key := timelineKey(uid); key != "timeline:"+uid {
		t.Errorf("unexpected timelineKey: %s", key)
	}
	if key := celebrityRecentKey(uid); key != "celebrity:recent:"+uid {
		t.Errorf("unexpected celebrityRecentKey: %s", key)
	}
	if key := activeUserKey(uid); key != "user:active:"+uid {
		t.Errorf("unexpected activeUserKey: %s", key)
	}
}
