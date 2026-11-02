package models

import (
	"encoding/json"
	"fmt"
	"time"
)

// EventType identifies a domain event consumed by the fanout workers.
type EventType string

const (
	EventPostCreated    EventType = "post.created"
	EventPostDeleted    EventType = "post.deleted"
	EventUserFollowed   EventType = "user.followed"
	EventUserUnfollowed EventType = "user.unfollowed"
)

// Event is the broker-neutral envelope. Payload is kept raw so the transport
// adapter does not need to know every event shape.
type Event struct {
	Type    EventType       `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// DecodePayload unmarshals the event payload into the supplied destination.
func (e Event) DecodePayload(dst any) error {
	if len(e.Payload) == 0 {
		return fmt.Errorf("%w: event payload is empty", ErrInvalidInput)
	}
	if err := json.Unmarshal(e.Payload, dst); err != nil {
		return fmt.Errorf("%w: cannot decode %s payload: %v", ErrInvalidInput, e.Type, err)
	}
	return nil
}

// EventFromValues decodes a broker message represented as string fields. It
// accepts either a single JSON envelope in "event" or separate "type" and
// "payload" fields.
func EventFromValues(values map[string]interface{}) (Event, error) {
	if raw, ok := values["event"]; ok {
		data, err := bytesFromValue(raw)
		if err != nil {
			return Event{}, err
		}
		var event Event
		if err := json.Unmarshal(data, &event); err != nil {
			return Event{}, fmt.Errorf("%w: malformed event envelope: %v", ErrInvalidInput, err)
		}
		if event.Type == "" {
			return Event{}, fmt.Errorf("%w: event type is missing", ErrInvalidInput)
		}
		return event, nil
	}

	typeValue, ok := values["type"]
	if !ok {
		return Event{}, fmt.Errorf("%w: message has no event envelope", ErrInvalidInput)
	}
	payloadValue, ok := values["payload"]
	if !ok {
		return Event{}, fmt.Errorf("%w: message has no payload", ErrInvalidInput)
	}

	eventType, err := stringFromValue(typeValue)
	if err != nil {
		return Event{}, err
	}
	payload, err := bytesFromValue(payloadValue)
	if err != nil {
		return Event{}, err
	}

	event := Event{Type: EventType(eventType), Payload: payload}
	if event.Type == "" {
		return Event{}, fmt.Errorf("%w: event type is missing", ErrInvalidInput)
	}
	return event, nil
}

func stringFromValue(value interface{}) (string, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case []byte:
		return string(typed), nil
	default:
		return "", fmt.Errorf("%w: unsupported message field type %T", ErrInvalidInput, value)
	}
}

func bytesFromValue(value interface{}) ([]byte, error) {
	switch typed := value.(type) {
	case string:
		return []byte(typed), nil
	case []byte:
		return typed, nil
	default:
		return nil, fmt.Errorf("%w: unsupported message field type %T", ErrInvalidInput, value)
	}
}

// PostCreatedEvent is emitted after Post Service persists a new post.
type PostCreatedEvent struct {
	PostID    string `json:"post_id"`
	AuthorID  string `json:"author_id"`
	CreatedAt int64  `json:"created_at"` // Unix timestamp in seconds, millis, micros, or nanos
	IsPublic  bool   `json:"is_public"`
}

// PostDeletedEvent is emitted after a post is deleted or archived.
type PostDeletedEvent struct {
	PostID   string `json:"post_id"`
	AuthorID string `json:"author_id"`
}

// UserFollowedEvent is emitted after a follow relationship is created.
type UserFollowedEvent struct {
	FollowerID  string `json:"follower_id"`
	FollowingID string `json:"following_id"`
}

// UserUnfollowedEvent is emitted after a follow relationship is removed.
type UserUnfollowedEvent struct {
	FollowerID  string `json:"follower_id"`
	FollowingID string `json:"following_id"`
}

// ScoreFromTimestamp normalises a Unix timestamp of unknown precision to the
// Unix-microsecond score used by timeline sorted sets.
func ScoreFromTimestamp(ts int64) int64 {
	if ts <= 0 {
		return time.Now().UnixMicro()
	}

	switch {
	case ts < 1e11: // seconds (current value ~1.7e9)
		return ts * 1e6
	case ts < 1e14: // milliseconds (current value ~1.7e12)
		return ts * 1e3
	case ts < 1e17: // microseconds (current value ~1.7e15)
		return ts
	default: // nanoseconds (current value ~1.7e18)
		return ts / 1e3
	}
}

// ScoreFromTime converts a wall-clock time to the timeline score unit.
func ScoreFromTime(t time.Time) int64 {
	return t.UnixMicro()
}

// TimeFromScore converts a timeline score back to a wall-clock time.
func TimeFromScore(score int64) time.Time {
	return time.UnixMicro(score)
}
