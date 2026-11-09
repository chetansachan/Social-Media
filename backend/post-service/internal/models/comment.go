package models

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Comment is one node in a post's discussion tree.
type Comment struct {
	ID         uuid.UUID  `json:"id"`
	PostID     uuid.UUID  `json:"post_id"`
	AuthorID   uuid.UUID  `json:"author_id"`
	ParentID   *uuid.UUID `json:"parent_id,omitempty"`
	Content    string     `json:"content"`
	LikeCount  int        `json:"like_count"`
	ReplyCount int        `json:"reply_count"`
	Depth      int        `json:"depth"`
	IsDeleted  bool       `json:"is_deleted"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// CreateCommentInput is the validated service input. ParentID is nil for a
// top-level comment.
type CreateCommentInput struct {
	PostID   uuid.UUID
	AuthorID uuid.UUID
	ParentID *uuid.UUID
	Content  string
}

// CommentPage is a keyset-paginated slice of comments plus the cursor for the
// next page.
type CommentPage struct {
	Comments   []Comment
	NextCursor string
	HasMore    bool
}

// CommentCursor is the keyset position of the last comment in a page. Top-level
// comments page newest first; replies page oldest first. The UUID tie-breaker
// keeps the order stable when many comments share a timestamp.
type CommentCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// IsZero reports whether the cursor is unset (first page).
func (c CommentCursor) IsZero() bool {
	return c.ID == uuid.Nil && c.CreatedAt.IsZero()
}

// EncodeCommentCursor serialises a keyset position as a URL-safe opaque token.
func EncodeCommentCursor(createdAt time.Time, id uuid.UUID) string {
	raw := createdAt.UTC().Format(time.RFC3339Nano) + "|" + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeCommentCursor parses a cursor produced by EncodeCommentCursor.
func DecodeCommentCursor(encoded string) (CommentCursor, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return CommentCursor{}, nil
	}

	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return CommentCursor{}, fmt.Errorf("%w: malformed comment cursor", ErrInvalidInput)
	}

	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 {
		return CommentCursor{}, fmt.Errorf("%w: comment cursor has no position", ErrInvalidInput)
	}

	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return CommentCursor{}, fmt.Errorf("%w: comment cursor has an invalid timestamp", ErrInvalidInput)
	}

	id, err := uuid.Parse(parts[1])
	if err != nil {
		return CommentCursor{}, fmt.Errorf("%w: comment cursor has an invalid id", ErrInvalidInput)
	}

	return CommentCursor{CreatedAt: createdAt, ID: id}, nil
}
