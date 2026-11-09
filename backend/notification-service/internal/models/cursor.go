package models

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Cursor is the keyset position of the last item in a page. Ordering is
// (created_at DESC, id DESC); the UUID tie-breaker keeps pagination stable when
// many notifications share a timestamp.
type Cursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// IsZero reports whether the cursor is unset (first page).
func (c Cursor) IsZero() bool {
	return c.ID == uuid.Nil && c.CreatedAt.IsZero()
}

// EncodeCursor serialises a keyset position as a URL-safe, opaque token.
func EncodeCursor(createdAt time.Time, id uuid.UUID) string {
	raw := createdAt.UTC().Format(time.RFC3339Nano) + "|" + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeCursor parses a cursor produced by EncodeCursor.
func DecodeCursor(encoded string) (Cursor, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return Cursor{}, nil
	}

	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return Cursor{}, fmt.Errorf("%w: malformed base64", ErrInvalidCursor)
	}

	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 {
		return Cursor{}, fmt.Errorf("%w: missing fields", ErrInvalidCursor)
	}

	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return Cursor{}, fmt.Errorf("%w: invalid timestamp", ErrInvalidCursor)
	}

	id, err := uuid.Parse(parts[1])
	if err != nil {
		return Cursor{}, fmt.Errorf("%w: invalid id", ErrInvalidCursor)
	}

	return Cursor{CreatedAt: createdAt, ID: id}, nil
}
