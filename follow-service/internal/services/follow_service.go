package services

import (
	"context"

	"github.com/chetansachan/socialflow/follow-service/internal/models"
	"github.com/chetansachan/socialflow/follow-service/internal/repository"
)

type FollowService interface {
	FollowUser(ctx context.Context, followerID, followingID string) error
	UnfollowUser(ctx context.Context, followerID, followingID string) error
	GetFollowers(ctx context.Context, userID string) ([]string, error)
	GetFollowing(ctx context.Context, userID string) ([]string, error)
	GetFollowCounts(ctx context.Context, userID string) (*models.FollowCounts, error)
	GetFollowStatus(ctx context.Context, followerID, followingID string) (bool, error)
	RemoveFollower(ctx context.Context, userID, followerID string) error
}

type followService struct {
	followRepo repository.FollowRepository
}

func NewFollowService(followRepo repository.FollowRepository) FollowService {
	return &followService{followRepo: followRepo}
}
