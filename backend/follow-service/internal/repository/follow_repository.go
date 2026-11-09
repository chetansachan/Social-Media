package repository

import (
	"context"
	"fmt"

	"github.com/chetansachan/socialflow/follow-service/internal/database"
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
	writer *pgxpool.Pool
	reader *pgxpool.Pool
}

// NewFollowRepository builds a repository over the primary and read pools.
// Follow status reads the primary because it drives the follow button and must
// reflect the user's own last action; heavier list and count reads use the
// replica.
func NewFollowRepository(db *database.DB) FollowRepository {
	return &followRepository{
		writer: db.Writer(),
		reader: db.Reader(),
	}
}

func (r *followRepository) FollowUser(ctx context.Context, followerID, followingID uuid.UUID) error {
	tx, err := r.writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin follow transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `
		INSERT INTO follows (follower_id, following_id)
		VALUES ($1, $2)
		ON CONFLICT (follower_id, following_id) DO NOTHING
	`

	tag, err := tx.Exec(ctx, query, followerID, followingID)
	if err != nil {
		return fmt.Errorf("failed to follow user: %w", err)
	}

	// Following is idempotent: a repeated request is a no-op and emits no
	// duplicate event.
	if tag.RowsAffected() == 0 {
		return tx.Commit(ctx)
	}

	if err := enqueueOutbox(ctx, tx, "user.followed", followingID, models.UserFollowedEvent{
		FollowerID:  followerID.String(),
		FollowingID: followingID.String(),
	}); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit follow transaction: %w", err)
	}
	return nil
}

func (r *followRepository) UnfollowUser(ctx context.Context, followerID, followingID uuid.UUID) error {
	tx, err := r.writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin unfollow transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `
		DELETE FROM follows
		WHERE follower_id = $1 AND following_id = $2
	`

	tag, err := tx.Exec(ctx, query, followerID, followingID)
	if err != nil {
		return fmt.Errorf("failed to unfollow user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Unfollowing twice is a no-op, not an error.
		return tx.Commit(ctx)
	}

	if err := enqueueOutbox(ctx, tx, "user.unfollowed", followingID, models.UserUnfollowedEvent{
		FollowerID:  followerID.String(),
		FollowingID: followingID.String(),
	}); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit unfollow transaction: %w", err)
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

	rows, err := r.reader.Query(ctx, query, userID)
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

func (r *followRepository) GetFollowing(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	query := `
		SELECT following_id
		FROM follows
		WHERE follower_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.reader.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get following: %w", err)
	}
	defer rows.Close()

	following := make([]uuid.UUID, 0)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan following id: %w", err)
		}
		following = append(following, id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating following: %w", err)
	}

	return following, nil
}

func (r *followRepository) GetFollowCounts(ctx context.Context, userID uuid.UUID) (*models.FollowCounts, error) {
	query := `
		SELECT 
			(SELECT COUNT(*) FROM follows WHERE following_id = $1) AS followers_count,
			(SELECT COUNT(*) FROM follows WHERE follower_id = $1) AS following_count
	`

	counts := &models.FollowCounts{}
	err := r.reader.QueryRow(ctx, query, userID).Scan(&counts.FollowersCount, &counts.FollowingCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get follow counts: %w", err)
	}

	return counts, nil
}

func (r *followRepository) GetFollowStatus(ctx context.Context, followerID, followingID uuid.UUID) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM follows
			WHERE follower_id = $1 AND following_id = $2
		)
	`

	var isFollowing bool
	err := r.writer.QueryRow(ctx, query, followerID, followingID).Scan(&isFollowing)
	if err != nil {
		return false, fmt.Errorf("failed to check follow status: %w", err)
	}

	return isFollowing, nil
}

func (r *followRepository) RemoveFollower(ctx context.Context, userID, followerID uuid.UUID) error {
	tx, err := r.writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin remove follower transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `
		DELETE FROM follows
		WHERE following_id = $1 AND follower_id = $2
	`

	tag, err := tx.Exec(ctx, query, userID, followerID)
	if err != nil {
		return fmt.Errorf("failed to remove follower: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return tx.Commit(ctx)
	}

	if err := enqueueOutbox(ctx, tx, "user.unfollowed", userID, models.UserUnfollowedEvent{
		FollowerID:  followerID.String(),
		FollowingID: userID.String(),
	}); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit remove follower transaction: %w", err)
	}
	return nil
}
