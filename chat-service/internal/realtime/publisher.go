package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Event types published on a member's channel. The gateway relays these frames
// to whichever socket holds that user's session.
const (
	EventMessageCreated = "message.created"
	EventMessageDeleted = "message.deleted"
	EventTyping         = "typing"
	EventRead           = "read"
)

// Envelope mirrors the WebSocket frame the gateway already speaks for
// notifications, so clients handle one shape for both streams.
type Envelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// Publisher fans chat frames out to the Redis Pub/Sub channel each member
// listens on, and mirrors them onto a stream so a reconnecting client can catch
// up on what it missed rather than polling for it.
type Publisher struct {
	client      *redis.Client
	eventStream string
}

func NewPublisher(client *redis.Client, eventStream string) *Publisher {
	if strings.TrimSpace(eventStream) == "" {
		eventStream = "social:events"
	}
	return &Publisher{client: client, eventStream: eventStream}
}

// userChannel returns the per-member channel the gateway subscribes to.
func userChannel(userID uuid.UUID) string {
	return "chat:" + userID.String()
}

// PublishToUsers sends one envelope to every listed user.
func (p *Publisher) PublishToUsers(ctx context.Context, userIDs []uuid.UUID, eventType string, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to encode %s payload: %w", eventType, err)
	}

	frame := Envelope{Type: eventType, Data: encoded}
	wire, err := json.Marshal(frame)
	if err != nil {
		return fmt.Errorf("failed to encode %s frame: %w", eventType, err)
	}

	for _, userID := range userIDs {
		if err := p.client.Publish(ctx, userChannel(userID), wire).Err(); err != nil {
			return fmt.Errorf("failed to publish %s to %s: %w", eventType, userID, err)
		}
	}

	// The stream is the durable copy. It carries the same frame plus a per-user
	// channel field so a catch-up reader can filter to one member.
	return p.client.XAdd(ctx, &redis.XAddArgs{
		Stream: p.eventStream,
		Values: []any{
			"type", eventType,
			"frame", string(wire),
		},
	}).Err()
}

// PublishTyping notifies a thread's members that somebody is or is not typing.
func (p *Publisher) PublishTyping(ctx context.Context, members []uuid.UUID, conversationID, userID uuid.UUID, isTyping bool) error {
	return p.PublishToUsers(ctx, members, EventTyping, map[string]any{
		"conversation_id": conversationID.String(),
		"user_id":         userID.String(),
		"is_typing":       isTyping,
	})
}

// PublishMessageCreated fans a new message out to the thread's members.
func (p *Publisher) PublishMessageCreated(ctx context.Context, members []uuid.UUID, payload map[string]any) error {
	return p.PublishToUsers(ctx, members, EventMessageCreated, payload)
}

// PublishMessageDeleted tells the other members a message became a tombstone.
// The frame names only the id so a client can drop it from its own page.
func (p *Publisher) PublishMessageDeleted(ctx context.Context, members []uuid.UUID, conversationID, messageID uuid.UUID) error {
	return p.PublishToUsers(ctx, members, EventMessageDeleted, map[string]any{
		"conversation_id": conversationID.String(),
		"message_id":      messageID.String(),
	})
}

// PublishRead tells the other members that their messages were seen, so the
// sender can stop showing a "waiting" state.
func (p *Publisher) PublishRead(ctx context.Context, members []uuid.UUID, conversationID, userID uuid.UUID) error {
	return p.PublishToUsers(ctx, members, EventRead, map[string]any{
		"conversation_id": conversationID.String(),
		"user_id":         userID.String(),
	})
}
