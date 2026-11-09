package models

import (
	"encoding/json"
	"testing"
)

func TestEventFromValuesEnvelope(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"event": `{"type":"post.liked","payload":{"post_id":"p1","actor_id":"a1"}}`,
	}
	event, err := EventFromValues(payload)
	if err != nil {
		t.Fatalf("EventFromValues() error: %v", err)
	}
	if event.Type != EventPostLiked {
		t.Errorf("type = %q, want %q", event.Type, EventPostLiked)
	}

	var decoded map[string]string
	if err := event.DecodePayload(&decoded); err != nil {
		t.Fatalf("DecodePayload() error: %v", err)
	}
	if decoded["post_id"] != "p1" {
		t.Errorf("post_id = %q", decoded["post_id"])
	}
}

func TestEventFromValuesTypeAndPayload(t *testing.T) {
	t.Parallel()

	values := map[string]any{
		"type":     "comment-replied",
		"event_id": "evt-1",
		"payload":  `{"comment_id":"c1"}`,
	}
	event, err := EventFromValues(values)
	if err != nil {
		t.Fatalf("EventFromValues() error: %v", err)
	}
	if event.Type != EventCommentReplied {
		t.Errorf("type = %q, want %q", event.Type, EventCommentReplied)
	}
	if event.EventID != "evt-1" {
		t.Errorf("event_id = %q", event.EventID)
	}
}

func TestEventFromValuesFlat(t *testing.T) {
	t.Parallel()

	values := map[string]any{
		"type":       "user.mentioned",
		"actor_id":   "a1",
		"post_id":    "p1",
		"comment_id": "c1",
	}
	event, err := EventFromValues(values)
	if err != nil {
		t.Fatalf("EventFromValues() error: %v", err)
	}

	var decoded map[string]string
	if err := event.DecodePayload(&decoded); err != nil {
		t.Fatalf("DecodePayload() error: %v", err)
	}
	if decoded["actor_id"] != "a1" || decoded["post_id"] != "p1" {
		t.Errorf("flat payload not preserved: %#v", decoded)
	}
}

func TestEventFromValuesRejectsMalformed(t *testing.T) {
	t.Parallel()

	if _, err := EventFromValues(map[string]any{}); err == nil {
		t.Fatal("expected error for empty message")
	}
	if _, err := EventFromValues(map[string]any{"event": "{not-json"}); err == nil {
		t.Fatal("expected error for malformed envelope")
	}
	if _, err := EventFromValues(map[string]any{"type": "post.liked", "payload": 42}); err == nil {
		t.Fatal("expected error for non-string payload")
	}
}

func TestNormalizeEventType(t *testing.T) {
	t.Parallel()

	cases := map[string]EventType{
		"post.liked":             EventPostLiked,
		"post_liked":             EventPostLiked,
		"PostLiked":              EventPostLiked,
		"like.created":           EventPostLiked,
		"user.followed":          EventUserFollowed,
		"user-followed":          EventUserFollowed,
		"comment.created":        EventPostCommented,
		"creator.post":           EventCreatorPost,
		"account.security.alert": EventSecurityAlert,
	}
	for input, want := range cases {
		if got := NormalizeEventType(input); got != want {
			t.Errorf("NormalizeEventType(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestEventPayloadRecipientResolution(t *testing.T) {
	t.Parallel()

	like := PostReactionEvent{PostAuthorID: "author", UserID: "user"}
	if got := like.Recipient(); got != "author" {
		t.Errorf("like recipient = %q", got)
	}

	like = PostReactionEvent{UserID: "user"}
	if got := like.Recipient(); got != "user" {
		t.Errorf("fallback recipient = %q", got)
	}

	mention := UserMentionedEvent{MentionedUserID: "mentioned", RecipientID: "other"}
	if got := mention.Recipient(); got != "mentioned" {
		t.Errorf("mention recipient = %q", got)
	}
}

func TestEventDecodePayloadInvalid(t *testing.T) {
	t.Parallel()

	event := Event{Type: EventPostLiked}
	if err := event.DecodePayload(&struct{}{}); err == nil {
		t.Fatal("empty payload should fail")
	}

	event.Payload = json.RawMessage(`{"post_id":`)
	if err := event.DecodePayload(&PostReactionEvent{}); err == nil {
		t.Fatal("malformed JSON should fail")
	}
}
