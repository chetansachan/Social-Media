package models

// PostCreatedEvent represents an event emitted by Post Service when a new post is published.
type PostCreatedEvent struct {
	PostID    string `json:"post_id"`
	AuthorID  string `json:"author_id"`
	CreatedAt int64  `json:"created_at"` // Unix timestamp (nanoseconds or milliseconds)
	IsPublic  bool   `json:"is_public"`
}

// PostDeletedEvent represents an event emitted by Post Service when a post is removed.
type PostDeletedEvent struct {
	PostID   string `json:"post_id"`
	AuthorID string `json:"author_id"`
}

// UserFollowedEvent represents an event emitted when user A follows user B.
type UserFollowedEvent struct {
	FollowerID  string `json:"follower_id"`
	FollowingID string `json:"following_id"`
}

// UserUnfollowedEvent represents an event emitted when user A unfollows user B.
type UserUnfollowedEvent struct {
	FollowerID  string `json:"follower_id"`
	FollowingID string `json:"following_id"`
}
