package models

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCommentCursorRoundTrip(t *testing.T) {
	createdAt := time.Now().UTC().Round(0)
	id := uuid.New()

	decoded, err := DecodeCommentCursor(EncodeCommentCursor(createdAt, id))
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	if !decoded.CreatedAt.Equal(createdAt) {
		t.Fatalf("created_at = %s, want %s", decoded.CreatedAt, createdAt)
	}
	if decoded.ID != id {
		t.Fatalf("id = %s, want %s", decoded.ID, id)
	}
	if decoded.IsZero() {
		t.Fatal("decoded cursor should not be zero")
	}
}

func TestDecodeCommentCursorEmptyIsZero(t *testing.T) {
	for _, encoded := range []string{"", "   "} {
		cursor, err := DecodeCommentCursor(encoded)
		if err != nil {
			t.Fatalf("decode %q: %v", encoded, err)
		}
		if !cursor.IsZero() {
			t.Fatalf("cursor = %+v, want zero", cursor)
		}
	}
}

func TestDecodeCommentCursorRejectsMalformedInput(t *testing.T) {
	encode := func(raw string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(raw))
	}
	cases := []string{
		"not-base64!!",
		encode("no-separator"),
		encode("not-a-time|" + uuid.Nil.String()),
		encode("2024-01-01T00:00:00Z|not-a-uuid"),
	}
	for _, encoded := range cases {
		if _, err := DecodeCommentCursor(encoded); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("decode %q: err = %v, want ErrInvalidInput", encoded, err)
		}
	}
}
