package repository

import (
	"context"

	"github.com/chetansachan/socialflow/follow-service/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FollowRepository interface {
	FollowUser(ctx context.Context, followerID, followingID uuid.UUID) error
	UnfollowUser(ctx context.Context, followerID, followingID uuid.UUID) error
	GetFollowers(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
	GetFollowing(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
	GetFollowCounts(ctx context.Context, userID uuid.UUID) (*models.FollowCounts, error)
	GetFollowStatus(ctx context.Context, followerID, followingID uuid.UUID) (bool, error)
	RemoveFollower(ctx context.Context, userID, followerID uuid.UUID) error
}

type followRepository struct {
	db *pgxpool.Pool
}

func NewFollowRepository(db *pgxpool.Pool) FollowRepository {
	return &followRepository{db: db}
}
