package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/chetansachan/socialflow/notification-service/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// SettingsRepository stores per-user channel/category preferences. Reads are
// served from a Redis JSON cache when available; PostgreSQL is authoritative.
type SettingsRepository interface {
	Get(ctx context.Context, userID uuid.UUID) (*models.NotificationSettings, error)
	Save(ctx context.Context, settings *models.NotificationSettings) error
}

type settingsRepository struct {
	db       *pgxpool.Pool
	cache    *redis.Client
	cacheTTL time.Duration
}

func NewSettingsRepository(db *pgxpool.Pool, cache *redis.Client, cacheTTL time.Duration) SettingsRepository {
	if cacheTTL <= 0 {
		cacheTTL = 24 * time.Hour
	}
	return &settingsRepository{db: db, cache: cache, cacheTTL: cacheTTL}
}

func settingsCacheKey(userID uuid.UUID) string {
	return "prefs:" + userID.String()
}

func (r *settingsRepository) Get(ctx context.Context, userID uuid.UUID) (*models.NotificationSettings, error) {
	if r.cache != nil {
		cached, err := r.cache.Get(ctx, settingsCacheKey(userID)).Bytes()
		if err == nil {
			var settings models.NotificationSettings
			if json.Unmarshal(cached, &settings) == nil {
				return &settings, nil
			}
		} else if !errors.Is(err, redis.Nil) {
			// Cache failures must never take the inbox down; fall through to SQL.
			_ = err
		}
	}

	query := `
		SELECT
			user_id, inapp_likes, inapp_comments, inapp_follows, inapp_mentions,
			push_enabled, push_likes, push_comments, push_follows, push_mentions,
			email_enabled, email_weekly_digest, email_security_alerts,
			quiet_hours_enabled,
			COALESCE(to_char(quiet_hours_start, 'HH24:MI'), ''),
			COALESCE(to_char(quiet_hours_end, 'HH24:MI'), ''),
			timezone, updated_at
		FROM user_notification_settings
		WHERE user_id = $1
	`

	settings := &models.NotificationSettings{}
	err := r.db.QueryRow(ctx, query, userID).Scan(
		&settings.UserID,
		&settings.InAppLikes,
		&settings.InAppComments,
		&settings.InAppFollows,
		&settings.InAppMentions,
		&settings.PushEnabled,
		&settings.PushLikes,
		&settings.PushComments,
		&settings.PushFollows,
		&settings.PushMentions,
		&settings.EmailEnabled,
		&settings.EmailWeeklyDigest,
		&settings.EmailSecurityAlerts,
		&settings.QuietHoursEnabled,
		&settings.QuietHoursStart,
		&settings.QuietHoursEnd,
		&settings.Timezone,
		&settings.UpdatedAt,
	)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("failed to load notification settings: %w", err)
		}
		settings = models.DefaultSettings(userID)
	}

	r.cacheSettings(ctx, settings)
	return settings, nil
}

func (r *settingsRepository) Save(ctx context.Context, settings *models.NotificationSettings) error {
	query := `
		INSERT INTO user_notification_settings (
			user_id,
			inapp_likes, inapp_comments, inapp_follows, inapp_mentions,
			push_enabled, push_likes, push_comments, push_follows, push_mentions,
			email_enabled, email_weekly_digest, email_security_alerts,
			quiet_hours_enabled, quiet_hours_start, quiet_hours_end, timezone
		)
		VALUES (
			$1,
			$2, $3, $4, $5,
			$6, $7, $8, $9, $10,
			$11, $12, $13,
			$14, NULLIF($15, '')::time, NULLIF($16, '')::time, $17
		)
		ON CONFLICT (user_id) DO UPDATE SET
			inapp_likes = EXCLUDED.inapp_likes,
			inapp_comments = EXCLUDED.inapp_comments,
			inapp_follows = EXCLUDED.inapp_follows,
			inapp_mentions = EXCLUDED.inapp_mentions,
			push_enabled = EXCLUDED.push_enabled,
			push_likes = EXCLUDED.push_likes,
			push_comments = EXCLUDED.push_comments,
			push_follows = EXCLUDED.push_follows,
			push_mentions = EXCLUDED.push_mentions,
			email_enabled = EXCLUDED.email_enabled,
			email_weekly_digest = EXCLUDED.email_weekly_digest,
			email_security_alerts = EXCLUDED.email_security_alerts,
			quiet_hours_enabled = EXCLUDED.quiet_hours_enabled,
			quiet_hours_start = EXCLUDED.quiet_hours_start,
			quiet_hours_end = EXCLUDED.quiet_hours_end,
			timezone = EXCLUDED.timezone,
			updated_at = NOW()
		RETURNING updated_at
	`

	if err := r.db.QueryRow(ctx, query,
		settings.UserID,
		settings.InAppLikes,
		settings.InAppComments,
		settings.InAppFollows,
		settings.InAppMentions,
		settings.PushEnabled,
		settings.PushLikes,
		settings.PushComments,
		settings.PushFollows,
		settings.PushMentions,
		settings.EmailEnabled,
		settings.EmailWeeklyDigest,
		settings.EmailSecurityAlerts,
		settings.QuietHoursEnabled,
		settings.QuietHoursStart,
		settings.QuietHoursEnd,
		settings.Timezone,
	).Scan(&settings.UpdatedAt); err != nil {
		return fmt.Errorf("failed to save notification settings: %w", err)
	}

	r.cacheSettings(ctx, settings)
	return nil
}

func (r *settingsRepository) cacheSettings(ctx context.Context, settings *models.NotificationSettings) {
	if r.cache == nil {
		return
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		return
	}
	_ = r.cache.Set(ctx, settingsCacheKey(settings.UserID), encoded, r.cacheTTL).Err()
}
