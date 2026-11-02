package models

import "time"

// FeedPost represents an enriched post item in a user's feed.
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

// GetFeedQuery holds the query arguments for fetching a user's feed.
type GetFeedQuery struct {
	UserID   string `json:"user_id"`
	PageSize int32  `json:"page_size"`
	Cursor   string `json:"cursor"`
}

// FeedResult represents the paginated response containing feed posts.
type FeedResult struct {
	Posts      []FeedPost `json:"posts"`
	NextCursor string     `json:"next_cursor"`
	HasMore    bool       `json:"has_more"`
}

// TimelineItem represents a lightweight entry in the in-memory Redis sorted set timeline.
type TimelineItem struct {
	PostID string  `json:"post_id"`
	Score  float64 `json:"score"` // Unix timestamp in nanoseconds or milliseconds
}
