package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestScoreFromTimestampNormalisesPrecision(t *testing.T) {
	at := time.Date(2026, time.November, 2, 12, 0, 0, 0, time.UTC)
	want := at.UnixMicro()

	cases := []struct {
		name string
		in   int64
	}{
		{"seconds", at.Unix()},
		{"milliseconds", at.UnixMilli()},
		{"microseconds", at.UnixMicro()},
		{"nanoseconds", at.UnixNano()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ScoreFromTimestamp(tc.in); got != want {
				t.Fatalf("ScoreFromTimestamp(%d) = %d, want %d", tc.in, got, want)
			}
		})
	}
}

func TestTimeFromScoreRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Microsecond)
	if got := TimeFromScore(ScoreFromTime(now)); !got.Equal(now) {
		t.Fatalf("time round-trip = %v, want %v", got, now)
	}
}

func TestEventFromValues(t *testing.T) {
	payload := json.RawMessage(`{"post_id":"p1","author_id":"a1","created_at":1,"is_public":true}`)
	envelope, _ := json.Marshal(Event{Type: EventPostCreated, Payload: payload})

	event, err := EventFromValues(map[string]interface{}{"event": string(envelope)})
	if err != nil {
		t.Fatalf("envelope decode: %v", err)
	}
	if event.Type != EventPostCreated || len(event.Payload) == 0 {
		t.Fatalf("unexpected event: %+v", event)
	}

	event, err = EventFromValues(map[string]interface{}{
		"type":    string(EventPostDeleted),
		"payload": `{"post_id":"p1","author_id":"a1"}`,
	})
	if err != nil {
		t.Fatalf("field decode: %v", err)
	}
	if event.Type != EventPostDeleted {
		t.Fatalf("unexpected type: %s", event.Type)
	}

	if _, err := EventFromValues(map[string]interface{}{}); err == nil {
		t.Fatal("expected an error for an empty message")
	}
}
