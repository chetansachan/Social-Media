package models

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestReportCursorRoundTrip(t *testing.T) {
	createdAt := time.Now().UTC().Round(0)
	id := uuid.New()

	cursor, err := DecodeReportCursor(EncodeReportCursor(createdAt, id))
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	if !cursor.CreatedAt.Equal(createdAt) || cursor.ID != id {
		t.Fatalf("cursor = %+v, want %s/%s", cursor, createdAt, id)
	}
	if cursor.IsZero() {
		t.Fatal("decoded cursor should not be zero")
	}
}

func TestDecodeReportCursorRejectsMalformedInput(t *testing.T) {
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
		if _, err := DecodeReportCursor(encoded); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("decode %q: err = %v, want ErrInvalidInput", encoded, err)
		}
	}

	cursor, err := DecodeReportCursor("  ")
	if err != nil || !cursor.IsZero() {
		t.Fatalf("empty cursor = %+v, %v", cursor, err)
	}
}

func TestEnumHelpers(t *testing.T) {
	if !IsTargetType(TargetPost) || IsTargetType("post") || IsTargetType("") {
		t.Fatal("target type helper accepted or rejected the wrong value")
	}
	if !IsReason(ReasonSelfHarm) || IsReason("SELFHARM") {
		t.Fatal("reason helper accepted or rejected the wrong value")
	}
	if !IsOpenStatus(StatusPending) || IsOpenStatus(StatusActioned) {
		t.Fatal("open status helper accepted or rejected the wrong value")
	}
	if !IsResolvableStatus(StatusDismissed) || IsResolvableStatus(StatusPending) {
		t.Fatal("resolvable status helper accepted or rejected the wrong value")
	}
}
