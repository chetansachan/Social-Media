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

// MarshalEnvelope writes the wire envelope. The event id is omitted when the row
// has none, because omitempty does not cover a string and the nil UUID would
// otherwise reach consumers as a real id.
func (e OutboxEvent) MarshalEnvelope() ([]byte, error) {
	envelope := Envelope{
		Type:    e.EventType,
		Payload: e.Payload,
	}
	if e.ID != uuid.Nil {
		envelope.EventID = e.ID.String()
	}
	return json.Marshal(envelope)
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
