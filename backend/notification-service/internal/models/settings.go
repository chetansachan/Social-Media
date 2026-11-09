package models

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// NotificationSettings mirrors user_notification_settings. Consumers consult
// it before persisting or dispatching any channel.
type NotificationSettings struct {
	UserID uuid.UUID `json:"user_id"`

	InAppLikes    bool `json:"inapp_likes"`
	InAppComments bool `json:"inapp_comments"`
	InAppFollows  bool `json:"inapp_follows"`
	InAppMentions bool `json:"inapp_mentions"`

	PushEnabled  bool `json:"push_enabled"`
	PushLikes    bool `json:"push_likes"`
	PushComments bool `json:"push_comments"`
	PushFollows  bool `json:"push_follows"`
	PushMentions bool `json:"push_mentions"`

	EmailEnabled        bool `json:"email_enabled"`
	EmailWeeklyDigest   bool `json:"email_weekly_digest"`
	EmailSecurityAlerts bool `json:"email_security_alerts"`

	QuietHoursEnabled bool   `json:"quiet_hours_enabled"`
	QuietHoursStart   string `json:"quiet_hours_start"`
	QuietHoursEnd     string `json:"quiet_hours_end"`
	Timezone          string `json:"timezone"`

	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// DefaultSettings is used when a user has never opened the settings screen.
// It intentionally mirrors the database defaults.
func DefaultSettings(userID uuid.UUID) *NotificationSettings {
	return &NotificationSettings{
		UserID:              userID,
		InAppLikes:          true,
		InAppComments:       true,
		InAppFollows:        true,
		InAppMentions:       true,
		PushEnabled:         true,
		PushLikes:           false,
		PushComments:        true,
		PushFollows:         true,
		PushMentions:        true,
		EmailEnabled:        true,
		EmailWeeklyDigest:   true,
		EmailSecurityAlerts: true,
		QuietHoursEnabled:   false,
		Timezone:            "UTC",
	}
}

// InAppEnabled resolves the in-app category toggle. System alerts are always
// delivered in-app because they carry account/security information.
func (s *NotificationSettings) InAppEnabled(notificationType NotificationType) bool {
	switch notificationType.Category() {
	case "likes":
		return s.InAppLikes
	case "comments":
		return s.InAppComments
	case "follows":
		return s.InAppFollows
	case "mentions":
		return s.InAppMentions
	default:
		return true
	}
}

// PushCategoryEnabled resolves the push category toggle. push_enabled is a
// master switch that must also be on.
func (s *NotificationSettings) PushCategoryEnabled(notificationType NotificationType) bool {
	if !s.PushEnabled {
		return false
	}
	switch notificationType.Category() {
	case "likes":
		return s.PushLikes
	case "comments":
		return s.PushComments
	case "follows":
		return s.PushFollows
	case "mentions":
		return s.PushMentions
	default:
		return true
	}
}

// EmailCategoryEnabled resolves email opt-ins. Security alerts have their own
// mandatory toggle; everything else currently falls under the master switch
// (digest batching is handled by the email provider).
func (s *NotificationSettings) EmailCategoryEnabled(notificationType NotificationType) bool {
	if notificationType == TypeSystemAlert {
		return s.EmailSecurityAlerts
	}
	return s.EmailEnabled
}

// IsQuietHours reports whether the user's local clock is inside their quiet
// window. Windows may cross midnight (22:00 - 07:00). An equal start and end
// with quiet hours enabled means "all day".
func (s *NotificationSettings) IsQuietHours(now time.Time) bool {
	if !s.QuietHoursEnabled {
		return false
	}

	start, err := parseClock(s.QuietHoursStart)
	if err != nil {
		return false
	}
	end, err := parseClock(s.QuietHoursEnd)
	if err != nil {
		return false
	}
	if start == end {
		return true
	}

	location, err := time.LoadLocation(s.Timezone)
	if err != nil || location == nil {
		location = time.UTC
	}
	local := now.In(location)
	current := local.Hour()*60 + local.Minute()

	if start < end {
		return current >= start && current < end
	}
	return current >= start || current < end
}

// Validate checks enum-like fields and quiet-hours consistency before saving.
func (s *NotificationSettings) Validate() error {
	if s.UserID == uuid.Nil {
		return fmt.Errorf("%w: user_id is required", ErrInvalidInput)
	}
	if s.Timezone == "" {
		s.Timezone = "UTC"
	}
	if _, err := time.LoadLocation(s.Timezone); err != nil {
		return fmt.Errorf("%w: unknown timezone %q", ErrInvalidInput, s.Timezone)
	}
	if s.QuietHoursEnabled {
		if s.QuietHoursStart == "" || s.QuietHoursEnd == "" {
			return fmt.Errorf("%w: quiet hours require both a start and an end", ErrInvalidInput)
		}
		if _, err := parseClock(s.QuietHoursStart); err != nil {
			return fmt.Errorf("%w: quiet_hours_start must be HH:MM", ErrInvalidInput)
		}
		if _, err := parseClock(s.QuietHoursEnd); err != nil {
			return fmt.Errorf("%w: quiet_hours_end must be HH:MM", ErrInvalidInput)
		}
	} else if s.QuietHoursStart != "" || s.QuietHoursEnd != "" {
		if _, err := parseClock(s.QuietHoursStart); s.QuietHoursStart != "" && err != nil {
			return fmt.Errorf("%w: quiet_hours_start must be HH:MM", ErrInvalidInput)
		}
		if _, err := parseClock(s.QuietHoursEnd); s.QuietHoursEnd != "" && err != nil {
			return fmt.Errorf("%w: quiet_hours_end must be HH:MM", ErrInvalidInput)
		}
	}
	return nil
}

func parseClock(value string) (int, error) {
	value = strings.TrimSpace(value)
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid clock value %q", value)
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, fmt.Errorf("clock value out of range %q", value)
	}
	return hour*60 + minute, nil
}
