package models

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Conversation types stored in conversations.type.
const (
	ConversationDirect = "DIRECT"
	ConversationGroup  = "GROUP"
)

type Conversation struct {
	ID            uuid.UUID `json:"id"`
	Type          string    `json:"type"`
	Title         string    `json:"title"`
	CreatedBy     uuid.UUID `json:"created_by"`
	LastMessageAt time.Time `json:"last_message_at"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Participant is a membership row plus that member's read watermark.
type Participant struct {
	UserID     uuid.UUID  `json:"user_id"`
	LastReadAt *time.Time `json:"last_read_at,omitempty"`
}

type Message struct {
	ID             uuid.UUID  `json:"id"`
	ConversationID uuid.UUID  `json:"conversation_id"`
	SenderID       uuid.UUID  `json:"sender_id"`
	Content        string     `json:"content"`
	CreatedAt      time.Time  `json:"created_at"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty"`
}

// ConversationCursor pages the inbox by last activity. LastMessageAt falls back
// to CreatedAt when a thread is still empty.
type ConversationCursor struct {
	LastMessageAt time.Time
	ID            uuid.UUID
}

func (c ConversationCursor) IsZero() bool {
	return c.ID == uuid.Nil && c.LastMessageAt.IsZero()
}

func EncodeConversationCursor(at time.Time, id uuid.UUID) string {
	raw := at.UTC().Format(time.RFC3339Nano) + "|" + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func DecodeConversationCursor(encoded string) (ConversationCursor, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return ConversationCursor{}, nil
	}

	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return ConversationCursor{}, fmt.Errorf("%w: malformed conversation cursor", ErrInvalidInput)
	}

	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 {
		return ConversationCursor{}, fmt.Errorf("%w: conversation cursor has no position", ErrInvalidInput)
	}

	at, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return ConversationCursor{}, fmt.Errorf("%w: conversation cursor has an invalid timestamp", ErrInvalidInput)
	}

	id, err := uuid.Parse(parts[1])
	if err != nil {
		return ConversationCursor{}, fmt.Errorf("%w: conversation cursor has an invalid id", ErrInvalidInput)
	}

	return ConversationCursor{LastMessageAt: at, ID: id}, nil
}

// MessageCursor pages a thread newest first, oldest-of-the-page last.
type MessageCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

func (c MessageCursor) IsZero() bool {
	return c.ID == uuid.Nil && c.CreatedAt.IsZero()
}

func EncodeMessageCursor(at time.Time, id uuid.UUID) string {
	raw := at.UTC().Format(time.RFC3339Nano) + "|" + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func DecodeMessageCursor(encoded string) (MessageCursor, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return MessageCursor{}, nil
	}

	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return MessageCursor{}, fmt.Errorf("%w: malformed message cursor", ErrInvalidInput)
	}

	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 {
		return MessageCursor{}, fmt.Errorf("%w: message cursor has no position", ErrInvalidInput)
	}

	at, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return MessageCursor{}, fmt.Errorf("%w: message cursor has an invalid timestamp", ErrInvalidInput)
	}

	id, err := uuid.Parse(parts[1])
	if err != nil {
		return MessageCursor{}, fmt.Errorf("%w: message cursor has an invalid id", ErrInvalidInput)
	}

	return MessageCursor{CreatedAt: at, ID: id}, nil
}

// ConversationSummary is a thread joined with its members, last message, and
// unread count for one viewer. It lives next to Conversation so the inbox and
// detail reads share one row shape without importing the repository package.
type ConversationSummary struct {
	Conversation Conversation
	Participants []Participant
	LastMessage  *Message
	UnreadCount  int
}

// MessagePage is one newest-first page of a thread.
type MessagePage struct {
	Messages   []Message
	NextCursor string
	HasMore    bool
}

// InboxPage is one page of a viewer's threads plus the badge across all of them,
// so a client renders an unread count without summing the page client-side.
type InboxPage struct {
	Conversations []ConversationSummary
	NextCursor    string
	HasMore       bool
	TotalUnread   int
}
