package models

import "time"

// FeedPost is a hydrated post card returned to the client. The Feed Service
// stores only post IDs; this structure exists only after Post Service hydration.
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

// GetFeedQuery contains the arguments for one feed page request.
type GetFeedQuery struct {
	UserID   string `json:"user_id"`
	PageSize int32  `json:"page_size"`
	Cursor   string `json:"cursor"`
}

// FeedResult is the paginated feed response.
type FeedResult struct {
	Posts      []FeedPost `json:"posts"`
	NextCursor string     `json:"next_cursor"`
	HasMore    bool       `json:"has_more"`
}

// TimelineItem is a lightweight entry in a Redis sorted set. Score is the
// post creation time in Unix microseconds. Microseconds fit exactly in a
// float64 until the year 2255, so cursor comparisons stay deterministic.
type TimelineItem struct {
	PostID string `json:"post_id"`
	Score  int64  `json:"score"`
}
