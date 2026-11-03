package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Outbox status values persisted in outbox_events.status.
const (
	OutboxStatusPending    = "PENDING"
	OutboxStatusPublishing = "PUBLISHING"
	OutboxStatusPublished  = "PUBLISHED"
	OutboxStatusFailed     = "FAILED"
)

// OutboxEvent is one committed domain event waiting to be relayed to the
// message broker. It is written inside the same PostgreSQL transaction as the
// business mutation that produced it.
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

// Envelope is the wire format written to Redis Streams. Consumers accept both
// the single "event" JSON field and separate "type"/"payload" fields; this
// shape also carries the outbox id for consumer-side idempotency.
type Envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
	EventID string          `json:"event_id,omitempty"`
}

// MarshalEnvelope renders the event in its broker envelope.
func (e OutboxEvent) MarshalEnvelope() ([]byte, error) {
	return json.Marshal(Envelope{
		Type:    e.EventType,
		Payload: e.Payload,
		EventID: e.ID.String(),
	})
}

// PostCreatedEvent tells the feed and notification pipelines that a post now
// exists. created_at is a Unix timestamp in nanoseconds so consumers can
// normalise it to their own score precision.
type PostCreatedEvent struct {
	PostID      string `json:"post_id"`
	AuthorID    string `json:"author_id"`
	CreatedAt   int64  `json:"created_at"`
	IsPublic    bool   `json:"is_public"`
	PostPreview string `json:"post_preview,omitempty"`
}

// PostDeletedEvent asks consumers to remove a post from timelines and pending
// digests. It is emitted for both hard deletes and archives.
type PostDeletedEvent struct {
	PostID   string `json:"post_id"`
	AuthorID string `json:"author_id"`
}

// PostReactionEvent covers post.liked and post.unliked. The actor is the user
// who toggled the like; the recipient is the post author.
type PostReactionEvent struct {
	PostID       string `json:"post_id"`
	PostAuthorID string `json:"post_author_id"`
	ActorID      string `json:"actor_id"`
	PostPreview  string `json:"post_preview,omitempty"`
}
