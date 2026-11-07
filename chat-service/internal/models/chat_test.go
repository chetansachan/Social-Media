package models

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestConversationCursorRoundTrip(t *testing.T) {
	at := time.Now().UTC().Round(0)
	id := uuid.New()

	cursor, err := DecodeConversationCursor(EncodeConversationCursor(at, id))
	if err != nil {
		t.Fatalf("decode conversation cursor: %v", err)
	}
	if !cursor.LastMessageAt.Equal(at) || cursor.ID != id {
		t.Fatalf("cursor = %+v, want %s/%s", cursor, at, id)
	}
	if cursor.IsZero() {
		t.Fatal("decoded cursor should not be zero")
	}
}

func TestMessageCursorRoundTrip(t *testing.T) {
	at := time.Now().UTC().Round(0)
	id := uuid.New()

	cursor, err := DecodeMessageCursor(EncodeMessageCursor(at, id))
	if err != nil {
		t.Fatalf("decode message cursor: %v", err)
	}
	if !cursor.CreatedAt.Equal(at) || cursor.ID != id {
		t.Fatalf("cursor = %+v, want %s/%s", cursor, at, id)
	}
	if cursor.IsZero() {
		t.Fatal("decoded cursor should not be zero")
	}
}

func TestCursorsRejectMalformedInput(t *testing.T) {
	encode := func(raw string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(raw))
	}

	for _, encoded := range []string{
		"not-base64!!",
		encode("no-separator"),
		encode("not-a-time|" + uuid.Nil.String()),
		encode("2024-01-01T00:00:00Z|not-a-uuid"),
	} {
		if _, err := DecodeConversationCursor(encoded); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("conversation cursor %q: err = %v, want ErrInvalidInput", encoded, err)
		}
		if _, err := DecodeMessageCursor(encoded); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("message cursor %q: err = %v, want ErrInvalidInput", encoded, err)
		}
	}
}

func TestCursorsTreatBlankAsFirstPage(t *testing.T) {
	for _, encoded := range []string{"", "   "} {
		conversation, err := DecodeConversationCursor(encoded)
		if err != nil || !conversation.IsZero() {
			t.Fatalf("conversation cursor = %+v, %v", conversation, err)
		}
		message, err := DecodeMessageCursor(encoded)
		if err != nil || !message.IsZero() {
			t.Fatalf("message cursor = %+v, %v", message, err)
		}
	}
}

func TestIsConversationType(t *testing.T) {
	if !IsConversationType(ConversationDirect) || !IsConversationType(ConversationGroup) {
		t.Fatal("supported conversation types were rejected")
	}
	if IsConversationType("direct") || IsConversationType("") {
		t.Fatal("unsupported conversation type was accepted")
	}
}
