package models

import "time"

type FeedPost struct {
	ID        string     `json:"id"`
	AuthorID  string     `json:"author_id"`
	Content   string     `json:"content"`
	MediaURL  string     `json:"media_url"`
	IsPublic  bool       `json:"is_public"`
	LikeCount int32      `json:"like_count"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

type GetFeedQuery struct {
	UserID   string `json:"user_id"`
	PageSize int32  `json:"page_size"`
	Cursor   string `json:"cursor"`
}

type FeedResult struct {
	Posts      []FeedPost `json:"posts"`
	NextCursor string     `json:"next_cursor"`
	HasMore    bool       `json:"has_more"`
}
