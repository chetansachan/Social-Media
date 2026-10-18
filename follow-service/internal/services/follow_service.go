package services

import (
	"context"

	"fmt"

	"github.com/chetansachan/socialflow/follow-service/internal/models"
	"github.com/chetansachan/socialflow/follow-service/internal/repository"
	"github.com/google/uuid"
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

func (s *followService) FollowUser(ctx context.Context, followerID, followingID string) error {
	if followerID == "" || followingID == "" {
		return models.ErrInvalidID
	}
	if followerID == followingID {
		return models.ErrCannotFollowSelf
	}

	followerUUID, err := uuid.Parse(followerID)
	if err != nil {
		return fmt.Errorf("%w: invalid follower id", models.ErrInvalidID)
	}

	followingUUID, err := uuid.Parse(followingID)
	if err != nil {
		return fmt.Errorf("%w: invalid following id", models.ErrInvalidID)
	}

	return s.followRepo.FollowUser(ctx, followerUUID, followingUUID)
}

func (s *followService) UnfollowUser(ctx context.Context, followerID, followingID string) error {
	if followerID == "" || followingID == "" {
		return models.ErrInvalidID
	}

	followerUUID, err := uuid.Parse(followerID)
	if err != nil {
		return fmt.Errorf("%w: invalid follower id", models.ErrInvalidID)
	}

	followingUUID, err := uuid.Parse(followingID)
	if err != nil {
		return fmt.Errorf("%w: invalid following id", models.ErrInvalidID)
	}

	return s.followRepo.UnfollowUser(ctx, followerUUID, followingUUID)
}

func (s *followService) GetFollowers(ctx context.Context, userID string) ([]string, error) {
	if userID == "" {
		return nil, models.ErrInvalidID
	}

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid user id", models.ErrInvalidID)
	}

	followerUUIDs, err := s.followRepo.GetFollowers(ctx, userUUID)
	if err != nil {
		return nil, err
	}

	followers := make([]string, len(followerUUIDs))
	for i, id := range followerUUIDs {
		followers[i] = id.String()
	}

	return followers, nil
}
