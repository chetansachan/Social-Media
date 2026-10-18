package models

import (
	"time"

	"github.com/google/uuid"
)

type Follow struct {
	FollowerID  uuid.UUID `json:"follower_id"`
	FollowingID uuid.UUID `json:"following_id"`
	CreatedAt   time.Time `json:"created_at"`
}

type FollowCounts struct {
	FollowersCount int `json:"followers_count"`
	FollowingCount int `json:"following_count"`
}
