package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	OutboxStatusPending    = "PENDING"
	OutboxStatusPublishing = "PUBLISHING"
	OutboxStatusPublished  = "PUBLISHED"
	OutboxStatusFailed     = "FAILED"
)

// OutboxEvent is a committed follow-graph event waiting for the relay.
type OutboxEvent struct {
	ID          uuid.UUID       `json:"id"`
	EventType   string          `json:"event_type"`
	AggregateID uuid.UUID       `json:"aggregate_id"`
	Payload     json.RawMessage `json:"payload"`
	Status      string          `json:"status"`
	Attempts    int             `json:"attempts"`
	AvailableAt time.Time       `json:"available_at"`
	CreatedAt   time.Time       `json:"created_at"`
}

// Envelope is the wire format written to Redis Streams.
type Envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
	EventID string          `json:"event_id,omitempty"`
}

func (e OutboxEvent) MarshalEnvelope() ([]byte, error) {
	return json.Marshal(Envelope{
		Type:    e.EventType,
		Payload: e.Payload,
		EventID: e.ID.String(),
	})
}

// UserFollowedEvent is consumed by feed fanout and notifications.
type UserFollowedEvent struct {
	FollowerID  string `json:"follower_id"`
	FollowingID string `json:"following_id"`
}

// UserUnfollowedEvent asks consumers to stop fanning out and is retained for
// analytics; historical follow notifications are not retracted.
type UserUnfollowedEvent struct {
	FollowerID  string `json:"follower_id"`
	FollowingID string `json:"following_id"`
}
