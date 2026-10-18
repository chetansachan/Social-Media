package repository

import (
	"context"
	"fmt"

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

func (r *followRepository) FollowUser(ctx context.Context, followerID, followingID uuid.UUID) error {
	query := `
		INSERT INTO follows (follower_id, following_id)
		VALUES ($1, $2)
		ON CONFLICT (follower_id, following_id) DO NOTHING
	`

	tag, err := r.db.Exec(ctx, query, followerID, followingID)
	if err != nil {
		return fmt.Errorf("failed to follow user: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return models.ErrAlreadyFollowing
	}

	return nil
}
