package models

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestOutboxEvent_MarshalEnvelope(t *testing.T) {
	id := uuid.New()
	event := OutboxEvent{
		ID:        id,
		EventType: "post.created",
		Payload:   json.RawMessage(`{"post_id":"p1"}`),
	}

	encoded, err := event.MarshalEnvelope()
	if err != nil {
		t.Fatalf("MarshalEnvelope: %v", err)
	}

	var envelope Envelope
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if envelope.Type != "post.created" {
		t.Errorf("type = %q", envelope.Type)
	}
	if envelope.EventID != id.String() {
		t.Errorf("event_id = %q, want %q", envelope.EventID, id)
	}

	var payload map[string]string
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["post_id"] != "p1" {
		t.Errorf("payload = %v", payload)
	}
}
