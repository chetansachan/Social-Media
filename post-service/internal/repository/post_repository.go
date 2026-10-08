package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/chetansachan/socialflow/post-service/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostRepostiory interface {
	CreatePost(ctx context.Context, input *models.CreatePostInput) (*models.Post, error)
	GetPostById(ctx context.Context, id uuid.UUID) (*models.Post, error)
	UpdatePost(ctx context.Context, input *models.UpdatePostInput, postId uuid.UUID, authorId uuid.UUID) (*models.Post, error)
}

type postRepostiory struct {
	db *pgxpool.Pool
}

func NewPostRepository(db *pgxpool.Pool) PostRepostiory {
	return &postRepostiory{db: db}
}

func (r *postRepostiory) GetPostById(ctx context.Context, id uuid.UUID) (*models.Post, error) {
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
		INSERT INTO posts(author_id, content, media_url, is_public)
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

func (r *postRepostiory) UpdatePost(ctx context.Context, input *models.UpdatePostInput, postId uuid.UUID, authorId uuid.UUID) (*models.Post, error) {
	query := `
		UPDATE posts
		SET content = $1, is_public = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $3 AND author_id = $4
		RETURNING id, author_id, content, media_url, is_public, like_count, created_at, updated_at
	`

	post := &models.Post{}
	if err := r.db.QueryRow(ctx, query,
		input.Content,
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
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrPostNotFound
		}
		return nil, fmt.Errorf("failed to insert post: %w", err)
	}

	return post, nil
}
