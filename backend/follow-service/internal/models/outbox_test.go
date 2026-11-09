package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMarshalEnvelopeWrapsTheOutboxRow(t *testing.T) {
	eventID := uuid.New()
	aggregateID := uuid.New()
	now := time.Now().UTC().Round(0)
	payload := json.RawMessage(`{"follower_id":"u1","following_id":"u2"}`)

	encoded, err := OutboxEvent{
		ID:          eventID,
		EventType:   "user.followed",
		AggregateID: aggregateID,
		Payload:     payload,
		Status:      OutboxStatusPending,
		AvailableAt: now,
		CreatedAt:   now,
	}.MarshalEnvelope()
	if err != nil {
		t.Fatalf("MarshalEnvelope: %v", err)
	}

	var envelope Envelope
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatalf("envelope is not valid JSON: %v (%s)", err, encoded)
	}

	// The event id travels with the frame so a consumer that sees the same event
	// twice (a relay retry, or a stream redelivery) can drop the second copy.
	if envelope.EventID != eventID.String() {
		t.Fatalf("event_id = %q, want %q", envelope.EventID, eventID)
	}
	if envelope.Type != "user.followed" {
		t.Fatalf("type = %q", envelope.Type)
	}

	var decoded UserFollowedEvent
	if err := json.Unmarshal(envelope.Payload, &decoded); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}
	if decoded.FollowerID != "u1" || decoded.FollowingID != "u2" {
		t.Fatalf("payload = %+v", decoded)
	}
}

func TestMarshalEnvelopeOmitsTheIDWhenUnset(t *testing.T) {
	encoded, err := OutboxEvent{
		EventType: "user.unfollowed",
		Payload:   json.RawMessage(`{"follower_id":"u1","following_id":"u2"}`),
	}.MarshalEnvelope()
	if err != nil {
		t.Fatalf("MarshalEnvelope: %v", err)
	}

	// A zero id must be absent rather than serialised as the nil UUID, which a
	// consumer would otherwise treat as a real id.
	if containsKey(encoded, "event_id") {
		t.Fatalf("envelope = %s, want no event_id", encoded)
	}
}

func TestMarshalEnvelopePassesThePayloadThroughSemantically(t *testing.T) {
	payload := json.RawMessage(`{"  oddly":  "  spaced  "}`)

	encoded, err := OutboxEvent{EventType: "user.followed", Payload: payload}.MarshalEnvelope()
	if err != nil {
		t.Fatalf("MarshalEnvelope: %v", err)
	}

	var envelope Envelope
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// json.RawMessage compacts on the way out, so compare the decoded value
	// rather than the bytes: identical JSON is what matters, not the spacing.
	var want, got map[string]string
	if err := json.Unmarshal(payload, &want); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}
	if err := json.Unmarshal(envelope.Payload, &got); err != nil {
		t.Fatalf("envelope payload is not valid JSON: %v", err)
	}
	if got["  oddly"] != want["  oddly"] {
		t.Fatalf("payload = %v, want %v", got, want)
	}
}

func TestOutboxStatusesAreDistinct(t *testing.T) {
	statuses := []string{
		OutboxStatusPending,
		OutboxStatusPublishing,
		OutboxStatusPublished,
		OutboxStatusFailed,
	}
	seen := make(map[string]struct{}, len(statuses))
	for _, status := range statuses {
		if _, duplicate := seen[status]; duplicate {
			t.Fatalf("duplicate outbox status %q", status)
		}
		seen[status] = struct{}{}
	}
}

func containsKey(encoded []byte, key string) bool {
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &generic); err != nil {
		return false
	}
	_, ok := generic[key]
	return ok
}
