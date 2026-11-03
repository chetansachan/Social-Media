package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/chetansachan/socialflow/post-service/internal/database"
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
	LikePost(ctx context.Context, postId, userId uuid.UUID) (bool, error)
	GetAllPostLikers(ctx context.Context, postId uuid.UUID) ([]string, error)
	GetAllLikedPosts(ctx context.Context, userId uuid.UUID) ([]models.Post, error)
	GetLikeStates(ctx context.Context, userId uuid.UUID, postIds []uuid.UUID) (map[string]bool, error)
	SaveFiles(ctx context.Context, name string, ext string, mimeType string) error
}

type postRepository struct {
	writer *pgxpool.Pool
	reader *pgxpool.Pool
}

// NewPostRepository builds a repository over the primary and read pools.
// Mutations use the primary; list and detail reads use the replica. Ownership
// can be checked from a replica because a post's author never changes, and the
// mutating statement is still scoped by author_id on the primary.
func NewPostRepository(db *database.DB) PostRepository {
	return &postRepository{
		writer: db.Writer(),
		reader: db.Reader(),
	}
}

func (r *postRepository) GetPostById(ctx context.Context, id uuid.UUID) (*models.Post, error) {
	query := `
		SELECT id, author_id, content, media_url, is_public, like_count, created_at, updated_at, deleted_at
		FROM posts
		WHERE id = $1 AND deleted_at IS NULL 
	`
	Post := &models.Post{}
	if err := r.reader.QueryRow(ctx, query, id).Scan(
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
	rows, err := r.reader.Query(ctx, query, authorId)
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
	tx, err := r.writer.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin create post transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `
		INSERT INTO posts(author_id, content, media_url, is_public)
		VALUES ($1, $2, $3, $4)
		RETURNING id, author_id, content, media_url, is_public, like_count, created_at, updated_at, deleted_at
	`

	post := &models.Post{}
	if err := tx.QueryRow(ctx, query,
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

	// The event and the post commit together. If the relay cannot reach Redis
	// yet, the row stays PENDING and is retried after the outage.
	if err := enqueueOutbox(ctx, tx, "post.created", post.ID, models.PostCreatedEvent{
		PostID:      post.ID.String(),
		AuthorID:    post.AuthorID.String(),
		CreatedAt:   post.CreatedAt.UnixNano(),
		IsPublic:    post.IsPublic,
		PostPreview: preview(post.Content, 140),
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit create post transaction: %w", err)
	}

	return post, nil
}

// preview trims user content to a short, rune-safe snippet for notifications
// and push payloads.
func preview(content string, limit int) string {
	trimmed := strings.TrimSpace(content)
	if limit <= 0 {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) <= limit {
		return trimmed
	}
	return string(runes[:limit]) + "…"
}

func (r *postRepository) UpdatePost(ctx context.Context, input *models.UpdatePostInput, postId uuid.UUID, authorId uuid.UUID) (*models.Post, error) {
	query := `
		UPDATE posts
		SET content = $1, is_public = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $3 AND author_id = $4
		RETURNING id, author_id, content, media_url, is_public, like_count, created_at, updated_at, deleted_at
	`

	post := &models.Post{}
	if err := r.writer.QueryRow(ctx, query,
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
	tx, err := r.writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin delete post transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	const query = `
		DELETE FROM posts
		WHERE id = $1 AND author_id = $2
		RETURNING author_id
	`

	var deletedAuthorID uuid.UUID
	if err := tx.QueryRow(ctx, query, postId, authorId).Scan(&deletedAuthorID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.ErrPostNotFound
		}
		return fmt.Errorf("error deleting post: %w", err)
	}

	if err := enqueueOutbox(ctx, tx, "post.deleted", postId, models.PostDeletedEvent{
		PostID:   postId.String(),
		AuthorID: deletedAuthorID.String(),
	}); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit delete post transaction: %w", err)
	}
	return nil
}

func (r *postRepository) MovePostToArchive(ctx context.Context, postId uuid.UUID, authorId uuid.UUID) error {
	tx, err := r.writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin archive transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	const query = `
		UPDATE posts
		SET deleted_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND author_id = $2 AND deleted_at IS NULL
		RETURNING author_id
	`

	var archivedAuthorID uuid.UUID
	if err := tx.QueryRow(ctx, query, postId, authorId).Scan(&archivedAuthorID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.ErrPostNotFound
		}
		return fmt.Errorf("error archiving post: %w", err)
	}

	if err := enqueueOutbox(ctx, tx, "post.deleted", postId, models.PostDeletedEvent{
		PostID:   postId.String(),
		AuthorID: archivedAuthorID.String(),
	}); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit archive transaction: %w", err)
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
	if err := r.reader.QueryRow(ctx, query, id).Scan(
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
	rows, err := r.reader.Query(ctx, query, authorId)
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

func (r *postRepository) LikePost(ctx context.Context, postId, userId uuid.UUID) (bool, error) {
	tx, err := r.writer.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead,
	})
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	// Read the author and a short preview inside the same transaction so the
	// emitted event always describes the post state the like was applied to.
	var postAuthorID uuid.UUID
	var postContent string
	if err := tx.QueryRow(ctx, `
		SELECT author_id, content FROM posts
		WHERE id = $1 AND deleted_at IS NULL
	`, postId).Scan(&postAuthorID, &postContent); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, models.ErrPostNotFound
		}
		return false, fmt.Errorf("failed to load post for like: %w", err)
	}

	toggleQuery := `
		INSERT INTO post_likes (post_id, user_id, is_liked, liked_at, unliked_at)
		VALUES ($1, $2, true, NOW(), NOW())
		ON CONFLICT (post_id, user_id) 
		DO UPDATE SET 
			is_liked = NOT post_likes.is_liked,
			liked_at = CASE WHEN NOT post_likes.is_liked = true THEN NOW() ELSE post_likes.liked_at END,
			unliked_at = CASE WHEN NOT post_likes.is_liked = false THEN NOW() ELSE post_likes.unliked_at END
		RETURNING is_liked;
	`

	var isCurrentlyLiked bool
	err = tx.QueryRow(ctx, toggleQuery, postId, userId).Scan(&isCurrentlyLiked)
	if err != nil {
		return false, fmt.Errorf("failed to toggle like status: %w", err)
	}

	var counterChange int
	if isCurrentlyLiked {
		counterChange = 1
	} else {
		counterChange = -1
	}

	// Update the post's total count. GREATEST(0, ...) prevents negative values.
	updateCounterQuery := `
		UPDATE posts 
		SET like_count = GREATEST(0, like_count + $1), updated_at = NOW() 
		WHERE id = $2 AND deleted_at IS NULL;
	`
	_, err = tx.Exec(ctx, updateCounterQuery, counterChange, postId)
	if err != nil {
		return false, fmt.Errorf("failed to update post like counter: %w", err)
	}

	// Self-likes never notify; suppressing them at the producer keeps the
	// notification stream clean and saves consumer work.
	if userId != postAuthorID {
		eventType := "post.unliked"
		if isCurrentlyLiked {
			eventType = "post.liked"
		}
		if err := enqueueOutbox(ctx, tx, eventType, postId, models.PostReactionEvent{
			PostID:       postId.String(),
			PostAuthorID: postAuthorID.String(),
			ActorID:      userId.String(),
			PostPreview:  preview(postContent, 140),
		}); err != nil {
			return false, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("failed to commit tx: %w", err)
	}

	return isCurrentlyLiked, nil
}

func (r *postRepository) GetAllPostLikers(ctx context.Context, postId uuid.UUID) ([]string, error) {
	query := `
		SELECT user_id
		FROM post_likes
		WHERE post_id = $1 AND is_liked = TRUE
		ORDER BY liked_at DESC
	`

	rows, err := r.reader.Query(ctx, query, postId)
	if err != nil {
		return nil, fmt.Errorf("failed to query post likers: %w", err)
	}
	defer rows.Close()

	likers := make([]string, 0)
	for rows.Next() {
		var userID uuid.UUID
		if err := rows.Scan(&userID); err != nil {
			return nil, fmt.Errorf("failed to scan user id: %w", err)
		}
		likers = append(likers, userID.String())
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during row iteration: %w", err)
	}

	return likers, nil
}

func (r *postRepository) GetAllLikedPosts(ctx context.Context, userId uuid.UUID) ([]models.Post, error) {
	query := `
		SELECT p.id, p.author_id, p.content, p.media_url, p.is_public, p.like_count, p.created_at, p.updated_at, p.deleted_at
		FROM posts p
		JOIN post_likes pl ON p.id = pl.post_id
		WHERE pl.user_id = $1 AND pl.is_liked = TRUE AND p.deleted_at IS NULL
		ORDER BY pl.liked_at DESC
	`

	rows, err := r.reader.Query(ctx, query, userId)
	if err != nil {
		return nil, fmt.Errorf("failed to query liked posts: %w", err)
	}
	defer rows.Close()

	posts := make([]models.Post, 0)
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
			return nil, fmt.Errorf("row scan failed: %w", err)
		}
		posts = append(posts, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during row iteration: %w", err)
	}

	return posts, nil
}

// GetLikeStates returns only the posts the user currently likes. Callers should
// treat a missing key as false.
func (r *postRepository) GetLikeStates(ctx context.Context, userId uuid.UUID, postIds []uuid.UUID) (map[string]bool, error) {
	states := make(map[string]bool, len(postIds))
	if len(postIds) == 0 {
		return states, nil
	}

	const query = `
		SELECT post_id
		FROM post_likes
		WHERE user_id = $1 AND is_liked = TRUE AND post_id = ANY($2)
	`
	rows, err := r.reader.Query(ctx, query, userId, postIds)
	if err != nil {
		return nil, fmt.Errorf("failed to query like states: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var postID uuid.UUID
		if err := rows.Scan(&postID); err != nil {
			return nil, fmt.Errorf("failed to scan like state: %w", err)
		}
		states[postID.String()] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating like states: %w", err)
	}
	return states, nil
}

func (r *postRepository) SaveFiles(ctx context.Context, name string, ext string, mimeType string) error {
	if len(name) > 180 {
		name = name[:180]
	}
	if len(ext) > 80 {
		ext = ext[:80]
	}
	if len(mimeType) > 100 {
		mimeType = mimeType[:100]
	}
	query := `
		INSERT INTO files (file_name, file_ext, file_mimetype)
		VALUES ($1, $2, $3)
	`

	if _, err := r.writer.Exec(ctx, query, name, ext, mimeType); err != nil {
		return fmt.Errorf("database error: %w", err)
	}

	return nil
}
