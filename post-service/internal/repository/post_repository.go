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

type PostRepository interface {
	CreatePost(ctx context.Context, input *models.CreatePostInput) (*models.Post, error)
	GetPostById(ctx context.Context, id uuid.UUID) (*models.Post, error)
	UpdatePost(ctx context.Context, input *models.UpdatePostInput, postId uuid.UUID, authorId uuid.UUID) (*models.Post, error)
	DeletePost(ctx context.Context, postId uuid.UUID, authorId uuid.UUID) error
	GetAllArchivedPost(ctx context.Context, authorId uuid.UUID) ([]models.Post, error)
	GetArchivedPostById(ctx context.Context, id uuid.UUID) (*models.Post, error)
	MovePostToArchive(ctx context.Context, postId uuid.UUID, authorId uuid.UUID) error
	GetAllPost(ctx context.Context, authorId uuid.UUID) ([]models.Post, error)
}

type postRepository struct {
	db *pgxpool.Pool
}

func NewPostRepository(db *pgxpool.Pool) PostRepository {
	return &postRepository{db: db}
}

func (r *postRepository) GetPostById(ctx context.Context, id uuid.UUID) (*models.Post, error) {
	query := `
		SELECT id, author_id, content, media_url, is_public, like_count, created_at, updated_at, deleted_at
		FROM posts
		WHERE id = $1 AND deleted_at IS NULL 
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
		&Post.DeletedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrPostNotFound
		}
		return nil, fmt.Errorf("failed to get post by id: %w", err)
	}

	return Post, nil
}

func (r *postRepository) GetAllPost(ctx context.Context, authorId uuid.UUID) ([]models.Post, error) {
	query := `
		SELECT id, author_id, content, media_url, is_public, like_count, created_at, updated_at, deleted_at
		FROM posts
		WHERE author_id = $1 AND deleted_at IS NULL 
	`
	rows, err := r.db.Query(ctx, query, authorId)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrPostNotFound
		}
		return nil, fmt.Errorf("failed to get post by id: %w", err)
	}
	defer rows.Close()

	var posts []models.Post

	for rows.Next() {
		var p models.Post
		err := rows.Scan(
			&p.ID,
			&p.AuthorID,
			&p.Content,
			&p.MediaURL,
			&p.IsPublic,
			&p.LikeCount,
			&p.CreatedAt,
			&p.UpdatedAt,
			&p.DeletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("Row scan failed: %w", err)
		}
		posts = append(posts, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("Error during row iteration: %w", err)
	}

	return posts, nil
}

func (r *postRepository) CreatePost(ctx context.Context, input *models.CreatePostInput) (*models.Post, error) {
	query := `
		INSERT INTO posts(author_id, content, media_url, is_public)
		VALUES ($1, $2, $3, $4)
		RETURNING id, author_id, content, media_url, is_public, like_count, created_at, updated_at, deleted_at
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

func (r *postRepository) UpdatePost(ctx context.Context, input *models.UpdatePostInput, postId uuid.UUID, authorId uuid.UUID) (*models.Post, error) {
	query := `
		UPDATE posts
		SET content = $1, is_public = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $3 AND author_id = $4
		RETURNING id, author_id, content, media_url, is_public, like_count, created_at, updated_at, deleted_at
	`

	post := &models.Post{}
	if err := r.db.QueryRow(ctx, query,
		input.Content,
		input.IsPublic,
		postId,
		authorId,
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
		return nil, fmt.Errorf("failed to update post: %w", err)
	}

	return post, nil
}

func (r *postRepository) DeletePost(ctx context.Context, postId uuid.UUID, authorId uuid.UUID) error {
	query := `
		DELETE FROM posts
		WHERE id = $1 AND author_id = $2
	`

	tag, err := r.db.Exec(ctx, query, postId, authorId)
	if err != nil {
		return fmt.Errorf("Error deleting Post: %v", err)
	}
	if tag.RowsAffected() == 0 {
		return models.ErrPostNotFound
	}

	return nil
}

func (r *postRepository) MovePostToArchive(ctx context.Context, postId uuid.UUID, authorId uuid.UUID) error {
	query := `
		UPDATE posts
		SET deleted_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND author_id = $2 AND deleted_at IS NULL
	`

	tag, err := r.db.Exec(ctx, query, postId, authorId)
	if err != nil {
		return fmt.Errorf("Error deleting Post: %v", err)
	}
	if tag.RowsAffected() == 0 {
		return models.ErrPostNotFound
	}

	return nil
}

func (r *postRepository) GetArchivedPostById(ctx context.Context, id uuid.UUID) (*models.Post, error) {
	query := `
		SELECT id, author_id, content, media_url, is_public, like_count, created_at, updated_at, deleted_at
		FROM posts
		WHERE id = $1 AND deleted_at IS NOT NULL 
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
		&Post.DeletedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrPostNotFound
		}
		return nil, fmt.Errorf("failed to get archived post by id: %w", err)
	}

	return Post, nil
}

func (r *postRepository) GetAllArchivedPost(ctx context.Context, authorId uuid.UUID) ([]models.Post, error) {
	query := `
		SELECT id, author_id, content, media_url, is_public, like_count, created_at, updated_at, deleted_at
		FROM posts
		WHERE author_id = $1 AND deleted_at IS NOT NULL 
	`
	rows, err := r.db.Query(ctx, query, authorId)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrPostNotFound
		}
		return nil, fmt.Errorf("failed to get archived post by id: %w", err)
	}
	defer rows.Close()

	var posts []models.Post

	for rows.Next() {
		var p models.Post
		err := rows.Scan(
			&p.ID,
			&p.AuthorID,
			&p.Content,
			&p.MediaURL,
			&p.IsPublic,
			&p.LikeCount,
			&p.CreatedAt,
			&p.UpdatedAt,
			&p.DeletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("Row scan failed: %w", err)
		}
		posts = append(posts, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("Error during row iteration: %w", err)
	}

	return posts, nil
}
