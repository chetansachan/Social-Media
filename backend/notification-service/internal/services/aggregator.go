package services

import (
	"context"
	"fmt"
	"time"

	"github.com/chetansachan/socialflow/notification-service/internal/models"
	"github.com/google/uuid"
)

// Aggregator owns the Redis state for the "like storm" roll-up. The durable
// notification row remains in PostgreSQL; Redis only remembers which row is
// currently accepting new actors for an entity.
type Aggregator struct {
	cache    CacheStore
	window   time.Duration
	throttle time.Duration
}

func NewAggregator(cache CacheStore, window, throttle time.Duration) *Aggregator {
	if window <= 0 {
		window = 15 * time.Minute
	}
	if throttle <= 0 {
		throttle = 30 * time.Second
	}
	return &Aggregator{cache: cache, window: window, throttle: throttle}
}

// Key is the canonical aggregation key: agg:<recipient>:<type>:<entity>.
func (a *Aggregator) Key(recipientID uuid.UUID, notificationType models.NotificationType, entityID string) string {
	return fmt.Sprintf("agg:%s:%s:%s", recipientID.String(), notificationType, entityID)
}

func (a *Aggregator) Existing(ctx context.Context, key string) (string, bool, error) {
	if a == nil || a.cache == nil {
		return "", false, models.ErrCacheUnavailable
	}
	return a.cache.GetAggregation(ctx, key)
}

func (a *Aggregator) Remember(ctx context.Context, key, notificationID string) error {
	if a == nil || a.cache == nil {
		return models.ErrCacheUnavailable
	}
	return a.cache.SetAggregation(ctx, key, notificationID, a.window)
}

func (a *Aggregator) Forget(ctx context.Context, key string) error {
	if a == nil || a.cache == nil {
		return models.ErrCacheUnavailable
	}
	return a.cache.ClearAggregation(ctx, key)
}

// AddActor returns false when the actor was already counted in this window.
func (a *Aggregator) AddActor(ctx context.Context, key, actorID string) (bool, error) {
	if a == nil || a.cache == nil {
		return false, models.ErrCacheUnavailable
	}
	return a.cache.RegisterAggregationActor(ctx, key, actorID, a.window)
}

func (a *Aggregator) RemoveActor(ctx context.Context, key, actorID string) (bool, error) {
	if a == nil || a.cache == nil {
		return false, models.ErrCacheUnavailable
	}
	return a.cache.RemoveAggregationActor(ctx, key, actorID, a.window)
}

// ShouldBroadcast returns true when a realtime frame is allowed. Aggregated
// updates are throttled per entity so a viral post cannot flood a socket.
func (a *Aggregator) ShouldBroadcast(ctx context.Context, key string) bool {
	if a == nil || a.cache == nil {
		return true
	}
	throttled, err := a.cache.ShouldThrottle(ctx, "agg:rt:"+key, a.throttle)
	if err != nil {
		return true
	}
	return !throttled
}
