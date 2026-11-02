package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	PlatformIOS     = "ios"
	PlatformAndroid = "android"
	PlatformWeb     = "web"
)

// DeviceToken is a mobile/web push destination registered by a client.
type DeviceToken struct {
	ID         uuid.UUID `json:"id"`
	UserID     uuid.UUID `json:"user_id"`
	Token      string    `json:"token"`
	Platform   string    `json:"platform"`
	DeviceName string    `json:"device_name,omitempty"`
	AppVersion string    `json:"app_version,omitempty"`
	IsActive   bool      `json:"is_active"`
	LastSeenAt time.Time `json:"last_seen_at"`
	CreatedAt  time.Time `json:"created_at"`
}

// IsValidPlatform reports whether a platform can be stored in the CHECK
// constraint on user_device_tokens.
func IsValidPlatform(platform string) bool {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case PlatformIOS, PlatformAndroid, PlatformWeb:
		return true
	default:
		return false
	}
}

// ValidateDeviceToken validates registration input.
func ValidateDeviceToken(userID uuid.UUID, token, platform string) error {
	if userID == uuid.Nil {
		return fmt.Errorf("%w: user_id is required", ErrInvalidInput)
	}
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("%w: token is required", ErrInvalidInput)
	}
	if len(token) > 512 {
		return fmt.Errorf("%w: token is too long", ErrInvalidInput)
	}
	if !IsValidPlatform(platform) {
		return fmt.Errorf("%w: platform must be ios, android, or web", ErrInvalidInput)
	}
	return nil
}
