package repository

import (
	"context"
	"fmt"

	"github.com/chetansachan/socialflow/notification-service/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DeviceTokenRepository owns APNs/FCM token registration and revocation.
type DeviceTokenRepository interface {
	Register(ctx context.Context, userID uuid.UUID, token, platform, deviceName, appVersion string) error
	Revoke(ctx context.Context, userID uuid.UUID, token string) error
	ListActive(ctx context.Context, userID uuid.UUID) ([]models.DeviceToken, error)
	MarkInactive(ctx context.Context, token string) error
}

type deviceTokenRepository struct {
	db *pgxpool.Pool
}

func NewDeviceTokenRepository(db *pgxpool.Pool) DeviceTokenRepository {
	return &deviceTokenRepository{db: db}
}

func (r *deviceTokenRepository) Register(ctx context.Context, userID uuid.UUID, token, platform, deviceName, appVersion string) error {
	query := `
		INSERT INTO user_device_tokens (user_id, token, platform, device_name, app_version, is_active, last_seen_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), TRUE, NOW())
		ON CONFLICT (token) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			platform = EXCLUDED.platform,
			device_name = COALESCE(EXCLUDED.device_name, user_device_tokens.device_name),
			app_version = COALESCE(EXCLUDED.app_version, user_device_tokens.app_version),
			is_active = TRUE,
			last_seen_at = NOW()
	`
	if _, err := r.db.Exec(ctx, query, userID, token, platform, deviceName, appVersion); err != nil {
		return fmt.Errorf("failed to register device token: %w", err)
	}
	return nil
}

func (r *deviceTokenRepository) Revoke(ctx context.Context, userID uuid.UUID, token string) error {
	query := `
		UPDATE user_device_tokens
		SET is_active = FALSE
		WHERE user_id = $1 AND token = $2
	`
	if _, err := r.db.Exec(ctx, query, userID, token); err != nil {
		return fmt.Errorf("failed to revoke device token: %w", err)
	}
	return nil
}

func (r *deviceTokenRepository) ListActive(ctx context.Context, userID uuid.UUID) ([]models.DeviceToken, error) {
	query := `
		SELECT id, user_id, token, platform, COALESCE(device_name, ''), COALESCE(app_version, ''),
		       is_active, last_seen_at, created_at
		FROM user_device_tokens
		WHERE user_id = $1 AND is_active = TRUE
		ORDER BY last_seen_at DESC
	`
	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list device tokens: %w", err)
	}
	defer rows.Close()

	tokens := make([]models.DeviceToken, 0, 4)
	for rows.Next() {
		var token models.DeviceToken
		if err := rows.Scan(
			&token.ID,
			&token.UserID,
			&token.Token,
			&token.Platform,
			&token.DeviceName,
			&token.AppVersion,
			&token.IsActive,
			&token.LastSeenAt,
			&token.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan device token: %w", err)
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("device token iteration failed: %w", err)
	}
	return tokens, nil
}

func (r *deviceTokenRepository) MarkInactive(ctx context.Context, token string) error {
	query := `UPDATE user_device_tokens SET is_active = FALSE WHERE token = $1`
	if _, err := r.db.Exec(ctx, query, token); err != nil {
		return fmt.Errorf("failed to deactivate device token: %w", err)
	}
	return nil
}
