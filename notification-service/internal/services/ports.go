package services

import (
	"context"
	"time"

	"github.com/chetansachan/socialflow/notification-service/internal/models"
	"github.com/google/uuid"
)

// NotificationStore is the narrow persistence port used by the service. The
// PostgreSQL repository satisfies it, while tests can provide a small fake.
type NotificationStore interface {
	Create(ctx context.Context, input *models.CreateNotificationInput) (*models.Notification, error)
	GetByID(ctx context.Context, id uuid.UUID) (*models.Notification, error)
	List(ctx context.Context, recipientID uuid.UUID, filter models.NotificationType, cursor models.Cursor, limit int) ([]models.Notification, error)
	MarkRead(ctx context.Context, recipientID uuid.UUID, ids []string) (int, error)
	MarkAllRead(ctx context.Context, recipientID uuid.UUID) (int, error)
	Delete(ctx context.Context, recipientID uuid.UUID, id uuid.UUID) (bool, error)
	CountUnread(ctx context.Context, recipientID uuid.UUID) (int, error)
	UpdateAggregate(ctx context.Context, id uuid.UUID, actorIDs []string, total int, title, preview string, reopen bool) (bool, error)
}

// SettingsStore loads and saves per-user preferences.
type SettingsStore interface {
	Get(ctx context.Context, userID uuid.UUID) (*models.NotificationSettings, error)
	Save(ctx context.Context, settings *models.NotificationSettings) error
}

// CacheStore is the Redis-backed hot path used for unread badges, dedup
// locks, aggregation windows, and rate limiting.
type CacheStore interface {
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

// DeviceStore manages push destinations.
type DeviceStore interface {
	Register(ctx context.Context, userID uuid.UUID, token, platform, deviceName, appVersion string) error
	Revoke(ctx context.Context, userID uuid.UUID, token string) error
	ListActive(ctx context.Context, userID uuid.UUID) ([]models.DeviceToken, error)
	MarkInactive(ctx context.Context, token string) error
}

// ActorResolver hydrates user profiles for inbox rendering.
type ActorResolver interface {
	GetActors(ctx context.Context, ids []string) map[string]models.Actor
}

// RealtimePublisher pushes a frame to the WebSocket gateway via Redis Pub/Sub.
type RealtimePublisher interface {
	Publish(ctx context.Context, userID string, payload models.RealtimePayload) error
}

// PushQueue accepts mobile push work without blocking the event consumer.
type PushQueue interface {
	Enqueue(ctx context.Context, message models.PushMessage) error
}

// EmailQueue accepts transactional email work.
type EmailQueue interface {
	Enqueue(ctx context.Context, message models.EmailMessage) error
}
