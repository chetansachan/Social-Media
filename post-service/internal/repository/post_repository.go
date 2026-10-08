package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/chetansachan/socialflow/post-service/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostRepostiory interface {
	CreatePost(ctx context.Context, input *models.CreatePostInput) (*models.Post, error)
	GetPostById(ctx context.Context, id string) (*models.Post, error)
}

type postRepostiory struct {
	db *pgxpool.Pool
}

func NewPostRepository(db *pgxpool.Pool) PostRepostiory {
	return &postRepostiory{db: db}
}

func (r *postRepostiory) GetPostById(ctx context.Context, id string) (*models.Post, error) {
	query := `
		SELECT id, author_id, content, media_url, is_public, like_count, created_at, updated_at
		FROM posts
		WHERE id = $1
	`
	Post := &models.Post{}
	if err := r.db.QueryRow(ctx, query, id).Scan(
		&Post.ID,
		&Post.AuthorID,
		&Post.Content,
		&Post.MediaURL,
		&Post.IsPublic,
		&Post.LikeCount,
		&Post.CreatedAt,
		&Post.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrPostNotFound
		}
		return nil, fmt.Errorf("failed to get user by id: %w", err)
	}

	return Post, nil
}

func (r *postRepostiory) CreatePost(ctx context.Context, input *models.CreatePostInput) (*models.Post, error) {
	query := `
		INSERT INTO author_id, content, media_url, is_public
		VALUES ($1, $2, $3, $4)
		RETURNING id, author_id, content, media_url, is_public, like_count, created_at, updated_at
	`

	post := &models.Post{}
	if err := r.db.QueryRow(ctx, query,
		input.AuthorID,
		input.Content,
		input.MediaURL,
		input.IsPublic,
	).Scan(
		&post.ID,
		&post.AuthorID,
		&post.Content,
		&post.MediaURL,
		&post.IsPublic,
		&post.LikeCount,
		&post.CreatedAt,
		&post.UpdatedAt,
		&post.DeletedAt,
	); err != nil {
		return nil, fmt.Errorf("failed to insert post: %w", err)
	}

	return post, nil
}
