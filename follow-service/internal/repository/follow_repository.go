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

func (r *followRepository) UnfollowUser(ctx context.Context, followerID, followingID uuid.UUID) error {
	query := `
		DELETE FROM follows
		WHERE follower_id = $1 AND following_id = $2
	`

	tag, err := r.db.Exec(ctx, query, followerID, followingID)
	if err != nil {
		return fmt.Errorf("failed to unfollow user: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return models.ErrRelationNotFound
	}

	return nil
}

func (r *followRepository) GetFollowers(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	query := `
		SELECT follower_id
		FROM follows
		WHERE following_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get followers: %w", err)
	}
	defer rows.Close()

	followers := make([]uuid.UUID, 0)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan follower id: %w", err)
		}
		followers = append(followers, id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating followers: %w", err)
	}

	return followers, nil
}
