package models

import (
	"time"

	"github.com/google/uuid"
)

type Post struct {
	ID        uuid.UUID  `json:"id"`
	AuthorID  uuid.UUID  `json:"author_id"`
	Content   string     `json:"content"`
	MediaURL  string     `json:"media_url"`
	IsPublic  bool       `json:"is_public"`
	LikeCount int        `json:"like_count"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

type CreatePostInput struct {
	AuthorID uuid.UUID `json:"author_id"`
	Content  string    `json:"content"`
	MediaURL string    `json:"media_url"`
	IsPublic *bool      `json:"is_public,omitempty"`
}

type UpdatePostInput struct {
	Content  *string    `json:"content,omitempty"`
	IsPublic *bool      `json:"is_public,omitempty"`
}
