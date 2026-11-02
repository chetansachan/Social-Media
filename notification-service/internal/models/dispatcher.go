package models

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// RealtimePayload is the WebSocket frame delivered to an active client. The
// shape matches the client protocol documented in SYSTEM_DESIGN.md.
type RealtimePayload struct {
	Event string       `json:"event"`
	Data  RealtimeData `json:"data"`
}

// RealtimeData carries just enough information to render a toast and update the
// bell badge without another round trip.
type RealtimeData struct {
	ID               string           `json:"id"`
	Type             NotificationType `json:"type"`
	Title            string           `json:"title"`
	ContentPreview   string           `json:"content_preview,omitempty"`
	ActionURL        string           `json:"action_url"`
	Actors           []Actor          `json:"actors,omitempty"`
	TotalActorsCount int              `json:"total_actors_count"`
	UnreadCount      int64            `json:"unread_count"`
	CreatedAt        string           `json:"created_at"`
}

// NewRealtimePayload builds the standard frame for a freshly created or
// aggregated notification.
func NewRealtimePayload(notification *Notification, unreadCount int64) RealtimePayload {
	return RealtimePayload{
		Event: "notification_received",
		Data: RealtimeData{
			ID:               notification.ID.String(),
			Type:             notification.Type,
			Title:            notification.Title,
			ContentPreview:   notification.ContentPreview,
			ActionURL:        notification.ActionURL,
			Actors:           notification.Actors,
			TotalActorsCount: notification.TotalActorsCount,
			UnreadCount:      unreadCount,
			CreatedAt:        notification.CreatedAt.UTC().Format(time.RFC3339),
		},
	}
}

// PushMessage is queued for the FCM/APNs worker pool.
type PushMessage struct {
	RecipientID     uuid.UUID
	NotificationID  string
	Title           string
	Body            string
	ActionURL       string
	UnreadCount     int64
	IsSecurityAlert bool
}

// EmailMessage is queued for the transactional email worker.
type EmailMessage struct {
	RecipientID    uuid.UUID
	RecipientEmail string
	Subject        string
	Body           string
	NotificationID string
}

// ValidateEmailMessage ensures the SMTP provider has a destination.
func (m EmailMessage) Validate() error {
	if m.RecipientID == uuid.Nil {
		return fmt.Errorf("%w: email recipient is required", ErrInvalidInput)
	}
	if m.RecipientEmail == "" {
		return fmt.Errorf("%w: email address is required", ErrInvalidInput)
	}
	if m.Subject == "" {
		return fmt.Errorf("%w: email subject is required", ErrInvalidInput)
	}
	return nil
}
