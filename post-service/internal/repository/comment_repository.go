package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/chetansachan/socialflow/post-service/internal/database"
	"github.com/chetansachan/socialflow/post-service/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CommentRepository owns the comment tables. Reads go to the replica; every
// mutation runs on the primary and updates the denormalised counters in the
// same transaction as the row it changes.
type CommentRepository interface {
	CreateComment(ctx context.Context, input *models.CreateCommentInput) (*models.Comment, error)
	GetComment(ctx context.Context, id uuid.UUID) (*models.Comment, error)
	ListComments(ctx context.Context, postID uuid.UUID, parentID *uuid.UUID, cursor models.CommentCursor, limit int) ([]models.Comment, error)
	UpdateComment(ctx context.Context, commentID, authorID uuid.UUID, content string) (*models.Comment, error)
	DeleteComment(ctx context.Context, commentID, authorID uuid.UUID) error
	LikeComment(ctx context.Context, commentID, userID uuid.UUID) (bool, error)
	GetCommentLikeStates(ctx context.Context, userID uuid.UUID, commentIDs []uuid.UUID) (map[string]bool, error)
}

type commentRepository struct {
	writer *pgxpool.Pool
	reader *pgxpool.Pool
}

func NewCommentRepository(db *database.DB) CommentRepository {
	return &commentRepository{
		writer: db.Writer(),
		reader: db.Reader(),
	}
}

const commentColumns = `id, post_id, author_id, parent_id, content, like_count, reply_count, depth, is_deleted, created_at, updated_at`

func scanComment(row interface{ Scan(dest ...any) error }) (*models.Comment, error) {
	comment := &models.Comment{}
	if err := row.Scan(
		&comment.ID,
		&comment.PostID,
		&comment.AuthorID,
		&comment.ParentID,
		&comment.Content,
		&comment.LikeCount,
		&comment.ReplyCount,
		&comment.Depth,
		&comment.IsDeleted,
		&comment.CreatedAt,
		&comment.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return comment, nil
}

// CreateComment inserts the comment, bumps the post and parent counters, and
// writes the notification event inside one transaction. A comment can never
// point at a parent from a different post: that would build a thread the post
// page cannot render.
func (r *commentRepository) CreateComment(ctx context.Context, input *models.CreateCommentInput) (*models.Comment, error) {
	tx, err := r.writer.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin create comment transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var postAuthorID uuid.UUID
	var postContent string
	if err := tx.QueryRow(ctx, `
		SELECT author_id, content FROM posts
		WHERE id = $1 AND deleted_at IS NULL
	`, input.PostID).Scan(&postAuthorID, &postContent); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrPostNotFound
		}
		return nil, fmt.Errorf("failed to load post for comment: %w", err)
	}

	depth := 0
	var parentAuthorID uuid.UUID
	if input.ParentID != nil {
		var parentPostID uuid.UUID
		var parentDeleted bool
		if err := tx.QueryRow(ctx, `
			SELECT author_id, post_id, depth, is_deleted
			FROM post_comments
			WHERE id = $1
			FOR UPDATE
		`, *input.ParentID).Scan(&parentAuthorID, &parentPostID, &depth, &parentDeleted); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, models.ErrCommentNotFound
			}
			return nil, fmt.Errorf("failed to load parent comment: %w", err)
		}
		if parentDeleted {
			return nil, fmt.Errorf("%w: cannot reply to a deleted comment", models.ErrInvalidInput)
		}
		if parentPostID != input.PostID {
			return nil, fmt.Errorf("%w: parent comment belongs to another post", models.ErrInvalidInput)
		}
		depth++
		if depth > maxCommentDepth {
			return nil, models.ErrCommentDepthExceeded
		}
	}

	comment, err := scanComment(tx.QueryRow(ctx, `
		INSERT INTO post_comments (post_id, author_id, parent_id, content, depth)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+commentColumns,
		input.PostID, input.AuthorID, input.ParentID, input.Content, depth,
	))
	if err != nil {
		return nil, fmt.Errorf("failed to insert comment: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE posts
		SET comment_count = comment_count + 1, updated_at = NOW()
		WHERE id = $1
	`, input.PostID); err != nil {
		return nil, fmt.Errorf("failed to increment post comment count: %w", err)
	}

	event := models.CommentCreatedEvent{
		PostID:         input.PostID.String(),
		CommentID:      comment.ID.String(),
		PostAuthorID:   postAuthorID.String(),
		ActorID:        input.AuthorID.String(),
		ContentPreview: preview(input.Content, 140),
		CreatedAt:      comment.CreatedAt.UnixNano(),
	}
	eventType := "post.commented"

	if input.ParentID != nil {
		if _, err := tx.Exec(ctx, `
			UPDATE post_comments
			SET reply_count = reply_count + 1
			WHERE id = $1
		`, *input.ParentID); err != nil {
			return nil, fmt.Errorf("failed to increment parent reply count: %w", err)
		}
		eventType = "comment.replied"
		event.ParentCommentID = input.ParentID.String()
		event.ParentAuthorID = parentAuthorID.String()
	}

	if err := enqueueOutbox(ctx, tx, eventType, comment.ID, event); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit create comment transaction: %w", err)
	}

	return comment, nil
}

func (r *commentRepository) GetComment(ctx context.Context, id uuid.UUID) (*models.Comment, error) {
	comment, err := scanComment(r.reader.QueryRow(ctx, `
		SELECT `+commentColumns+` FROM post_comments WHERE id = $1
	`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrCommentNotFound
		}
		return nil, fmt.Errorf("failed to get comment: %w", err)
	}
	return comment, nil
}

// ListComments returns one page of a post's top-level comments (newest first)
// or of one comment's replies (oldest first). A deleted comment is kept in the
// result only while it still has replies, so a conversation never loses its
// structure but a deleted leaf disappears.
func (r *commentRepository) ListComments(
	ctx context.Context,
	postID uuid.UUID,
	parentID *uuid.UUID,
	cursor models.CommentCursor,
	limit int,
) ([]models.Comment, error) {
	if limit <= 0 {
		limit = 20
	}
	// Fetch one extra row to decide has_more without a COUNT query.
	fetch := limit + 1

	var (
		rows pgx.Rows
		err  error
	)

	if parentID == nil {
		rows, err = r.reader.Query(ctx, `
			SELECT `+commentColumns+`
			FROM post_comments
			WHERE post_id = $1
			  AND parent_id IS NULL
			  AND (is_deleted = FALSE OR reply_count > 0)
			  AND ($2::timestamptz IS NULL OR (created_at, id) < ($2, $3))
			ORDER BY created_at DESC, id DESC
			LIMIT $4
		`, postID, nullableTime(cursor), cursor.ID, fetch)
	} else {
		rows, err = r.reader.Query(ctx, `
			SELECT `+commentColumns+`
			FROM post_comments
			WHERE post_id = $1
			  AND parent_id = $2
			  AND (is_deleted = FALSE OR reply_count > 0)
			  AND ($3::timestamptz IS NULL OR (created_at, id) > ($3, $4))
			ORDER BY created_at ASC, id ASC
			LIMIT $5
		`, postID, *parentID, nullableTime(cursor), cursor.ID, fetch)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query comments: %w", err)
	}
	defer rows.Close()

	comments := make([]models.Comment, 0, fetch)
	for rows.Next() {
		comment, err := scanComment(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan comment: %w", err)
		}
		comments = append(comments, *comment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating comments: %w", err)
	}

	return comments, nil
}

// nullableTime turns an unset cursor into SQL NULL so the first page needs no
// special query.
func nullableTime(cursor models.CommentCursor) *time.Time {
	if cursor.IsZero() {
		return nil
	}
	at := cursor.CreatedAt
	return &at
}

func (r *commentRepository) UpdateComment(
	ctx context.Context,
	commentID, authorID uuid.UUID,
	content string,
) (*models.Comment, error) {
	comment, err := scanComment(r.writer.QueryRow(ctx, `
		UPDATE post_comments
		SET content = $1, updated_at = NOW()
		WHERE id = $2 AND author_id = $3 AND is_deleted = FALSE
		RETURNING `+commentColumns,
		content, commentID, authorID,
	))
	if err == nil {
		return comment, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("failed to update comment: %w", err)
	}

	// No row updated: distinguish "gone" from "not yours" so the handler can
	// return the right gRPC code. Read the primary: the replica may not have
	// the row yet, which would turn a permission error into a spurious 404.
	var (
		ownerID   uuid.UUID
		isDeleted bool
	)
	if lookupErr := r.writer.QueryRow(ctx, `
		SELECT author_id, is_deleted FROM post_comments WHERE id = $1
	`, commentID).Scan(&ownerID, &isDeleted); lookupErr != nil {
		if errors.Is(lookupErr, pgx.ErrNoRows) {
			return nil, models.ErrCommentNotFound
		}
		return nil, fmt.Errorf("failed to look up comment after update: %w", lookupErr)
	}
	if isDeleted {
		return nil, models.ErrCommentNotFound
	}
	if ownerID != authorID {
		return nil, models.ErrCommentForbidden
	}
	return nil, models.ErrCommentNotFound
}

// DeleteComment soft-deletes the row so replies keep their parent, then
// decrements the post and parent counters. It is idempotent: deleting an
// already-deleted comment succeeds without changing a counter twice.
func (r *commentRepository) DeleteComment(ctx context.Context, commentID, authorID uuid.UUID) error {
	tx, err := r.writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin delete comment transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var (
		postID    uuid.UUID
		parentID  *uuid.UUID
		ownerID   uuid.UUID
		isDeleted bool
	)
	if err := tx.QueryRow(ctx, `
		SELECT post_id, parent_id, author_id, is_deleted
		FROM post_comments
		WHERE id = $1
		FOR UPDATE
	`, commentID).Scan(&postID, &parentID, &ownerID, &isDeleted); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.ErrCommentNotFound
		}
		return fmt.Errorf("failed to load comment for delete: %w", err)
	}

	if ownerID != authorID {
		return models.ErrCommentForbidden
	}
	if isDeleted {
		return nil
	}

	if _, err := tx.Exec(ctx, `
		UPDATE post_comments
		SET is_deleted = TRUE, content = '', updated_at = NOW()
		WHERE id = $1
	`, commentID); err != nil {
		return fmt.Errorf("failed to soft-delete comment: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE posts
		SET comment_count = GREATEST(0, comment_count - 1), updated_at = NOW()
		WHERE id = $1
	`, postID); err != nil {
		return fmt.Errorf("failed to decrement post comment count: %w", err)
	}

	if parentID != nil {
		if _, err := tx.Exec(ctx, `
			UPDATE post_comments
			SET reply_count = GREATEST(0, reply_count - 1)
			WHERE id = $1
		`, *parentID); err != nil {
			return fmt.Errorf("failed to decrement parent reply count: %w", err)
		}
	}

	if err := enqueueOutbox(ctx, tx, "comment.deleted", commentID, models.CommentDeletedEvent{
		PostID:    postID.String(),
		CommentID: commentID.String(),
		AuthorID:  authorID.String(),
	}); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit delete comment transaction: %w", err)
	}
	return nil
}

func (r *commentRepository) LikeComment(ctx context.Context, commentID, userID uuid.UUID) (bool, error) {
	tx, err := r.writer.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return false, fmt.Errorf("failed to begin like comment transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var (
		commentAuthorID uuid.UUID
		postID          uuid.UUID
		content         string
		isDeleted       bool
	)
	if err := tx.QueryRow(ctx, `
		SELECT author_id, post_id, content, is_deleted
		FROM post_comments
		WHERE id = $1
		FOR UPDATE
	`, commentID).Scan(&commentAuthorID, &postID, &content, &isDeleted); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, models.ErrCommentNotFound
		}
		return false, fmt.Errorf("failed to load comment for like: %w", err)
	}
	if isDeleted {
		return false, models.ErrCommentNotFound
	}

	toggleQuery := `
		INSERT INTO comment_likes (comment_id, user_id, is_liked, liked_at, unliked_at)
		VALUES ($1, $2, true, NOW(), NOW())
		ON CONFLICT (comment_id, user_id)
		DO UPDATE SET
			is_liked = NOT comment_likes.is_liked,
			liked_at = CASE WHEN NOT comment_likes.is_liked = true THEN NOW() ELSE comment_likes.liked_at END,
			unliked_at = CASE WHEN NOT comment_likes.is_liked = false THEN NOW() ELSE comment_likes.unliked_at END
		RETURNING is_liked
	`

	var isCurrentlyLiked bool
	if err := tx.QueryRow(ctx, toggleQuery, commentID, userID).Scan(&isCurrentlyLiked); err != nil {
		return false, fmt.Errorf("failed to toggle comment like: %w", err)
	}

	counterChange := -1
	if isCurrentlyLiked {
		counterChange = 1
	}
	if _, err := tx.Exec(ctx, `
		UPDATE post_comments
		SET like_count = GREATEST(0, like_count + $1), updated_at = NOW()
		WHERE id = $2
	`, counterChange, commentID); err != nil {
		return false, fmt.Errorf("failed to update comment like counter: %w", err)
	}

	// Self-likes never notify, matching the post-like rule.
	if userID != commentAuthorID {
		eventType := "comment.unliked"
		if isCurrentlyLiked {
			eventType = "comment.liked"
		}
		if err := enqueueOutbox(ctx, tx, eventType, commentID, models.CommentReactionEvent{
			PostID:          postID.String(),
			CommentID:       commentID.String(),
			CommentAuthorID: commentAuthorID.String(),
			ActorID:         userID.String(),
			ContentPreview:  preview(content, 140),
		}); err != nil {
			return false, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("failed to commit like comment transaction: %w", err)
	}
	return isCurrentlyLiked, nil
}

// GetCommentLikeStates returns only the comments the user currently likes.
// Callers treat a missing key as false.
func (r *commentRepository) GetCommentLikeStates(
	ctx context.Context,
	userID uuid.UUID,
	commentIDs []uuid.UUID,
) (map[string]bool, error) {
	states := make(map[string]bool, len(commentIDs))
	if len(commentIDs) == 0 {
		return states, nil
	}

	rows, err := r.reader.Query(ctx, `
		SELECT comment_id
		FROM comment_likes
		WHERE user_id = $1 AND is_liked = TRUE AND comment_id = ANY($2)
	`, userID, commentIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to query comment like states: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan comment like state: %w", err)
		}
		states[id.String()] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating comment like states: %w", err)
	}
	return states, nil
}

// maxCommentDepth caps nesting. The cap keeps a thread readable and bounds the
// recursive rendering work a single post can trigger.
const maxCommentDepth = 5
