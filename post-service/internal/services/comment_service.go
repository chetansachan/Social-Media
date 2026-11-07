package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/chetansachan/socialflow/post-service/internal/models"
	"github.com/chetansachan/socialflow/post-service/internal/repository"
	"github.com/google/uuid"
)

// CommentService owns comment validation and pagination. Structural rules that
// need the parent row inside the transaction (nesting depth, cross-post
// parents) live in the repository; everything the caller can fix before a
// query happens here.
type CommentService interface {
	CreateComment(ctx context.Context, input models.CreateCommentInput) (*models.Comment, error)
	GetComment(ctx context.Context, id string) (*models.Comment, error)
	ListComments(ctx context.Context, postID, parentID string, limit int32, cursor string) (*models.CommentPage, error)
	UpdateComment(ctx context.Context, commentID, authorID, content string) (*models.Comment, error)
	DeleteComment(ctx context.Context, commentID, authorID string) error
	LikeComment(ctx context.Context, commentID, userID string) (bool, error)
	GetCommentLikeStates(ctx context.Context, userID string, commentIDs []string) (map[string]bool, error)
}

const (
	maxCommentLength = 1000
	defaultPageSize  = 20
	maxPageSize      = 100
)

type commentService struct {
	repo repository.CommentRepository
}

func NewCommentService(repo repository.CommentRepository) CommentService {
	return &commentService{repo: repo}
}

func (s *commentService) CreateComment(ctx context.Context, input models.CreateCommentInput) (*models.Comment, error) {
	if input.PostID == uuid.Nil || input.AuthorID == uuid.Nil {
		return nil, fmt.Errorf("%w: post and author are required", models.ErrInvalidInput)
	}

	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, fmt.Errorf("%w: comment cannot be empty", models.ErrInvalidInput)
	}
	if utf8.RuneCountInString(content) > maxCommentLength {
		return nil, fmt.Errorf("%w: comment must be at most %d characters", models.ErrInvalidInput, maxCommentLength)
	}
	if input.ParentID != nil && *input.ParentID == uuid.Nil {
		input.ParentID = nil
	}

	input.Content = content
	comment, err := s.repo.CreateComment(ctx, &input)
	switch {
	case err == nil:
		return comment, nil
	case errors.Is(err, models.ErrPostNotFound),
		errors.Is(err, models.ErrCommentNotFound),
		errors.Is(err, models.ErrInvalidInput),
		errors.Is(err, models.ErrCommentDepthExceeded):
		return nil, err
	default:
		return nil, fmt.Errorf("failed to create comment: %w", err)
	}
}

func (s *commentService) GetComment(ctx context.Context, id string) (*models.Comment, error) {
	commentID, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid comment id", models.ErrInvalidInput)
	}

	comment, err := s.repo.GetComment(ctx, commentID)
	if err != nil {
		if errors.Is(err, models.ErrCommentNotFound) {
			return nil, models.ErrCommentNotFound
		}
		return nil, fmt.Errorf("failed to load comment: %w", err)
	}
	return comment, nil
}

func (s *commentService) ListComments(
	ctx context.Context,
	postID, parentID string,
	limit int32,
	cursor string,
) (*models.CommentPage, error) {
	postUUID, err := uuid.Parse(strings.TrimSpace(postID))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid post id", models.ErrInvalidInput)
	}

	var parentUUID *uuid.UUID
	if strings.TrimSpace(parentID) != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(parentID))
		if err != nil {
			return nil, fmt.Errorf("%w: invalid parent comment id", models.ErrInvalidInput)
		}
		parentUUID = &parsed
	}

	pageSize := int(limit)
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	position, err := models.DecodeCommentCursor(cursor)
	if err != nil {
		return nil, err
	}

	comments, err := s.repo.ListComments(ctx, postUUID, parentUUID, position, pageSize)
	if err != nil {
		return nil, fmt.Errorf("failed to list comments: %w", err)
	}

	page := &models.CommentPage{Comments: comments}
	if len(comments) > pageSize {
		page.Comments = comments[:pageSize]
		page.HasMore = true
		last := page.Comments[len(page.Comments)-1]
		page.NextCursor = models.EncodeCommentCursor(last.CreatedAt, last.ID)
	}
	return page, nil
}

func (s *commentService) UpdateComment(ctx context.Context, commentID, authorID, content string) (*models.Comment, error) {
	commentUUID, err := uuid.Parse(strings.TrimSpace(commentID))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid comment id", models.ErrInvalidInput)
	}
	authorUUID, err := uuid.Parse(strings.TrimSpace(authorID))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid author id", models.ErrInvalidInput)
	}

	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil, fmt.Errorf("%w: comment cannot be empty", models.ErrInvalidInput)
	}
	if utf8.RuneCountInString(trimmed) > maxCommentLength {
		return nil, fmt.Errorf("%w: comment must be at most %d characters", models.ErrInvalidInput, maxCommentLength)
	}

	comment, err := s.repo.UpdateComment(ctx, commentUUID, authorUUID, trimmed)
	if err != nil {
		switch {
		case errors.Is(err, models.ErrCommentNotFound), errors.Is(err, models.ErrCommentForbidden):
			return nil, err
		default:
			return nil, fmt.Errorf("failed to update comment: %w", err)
		}
	}
	return comment, nil
}

func (s *commentService) DeleteComment(ctx context.Context, commentID, authorID string) error {
	commentUUID, err := uuid.Parse(strings.TrimSpace(commentID))
	if err != nil {
		return fmt.Errorf("%w: invalid comment id", models.ErrInvalidInput)
	}
	authorUUID, err := uuid.Parse(strings.TrimSpace(authorID))
	if err != nil {
		return fmt.Errorf("%w: invalid author id", models.ErrInvalidInput)
	}

	if err := s.repo.DeleteComment(ctx, commentUUID, authorUUID); err != nil {
		switch {
		case errors.Is(err, models.ErrCommentNotFound), errors.Is(err, models.ErrCommentForbidden):
			return err
		default:
			return fmt.Errorf("failed to delete comment: %w", err)
		}
	}
	return nil
}

func (s *commentService) LikeComment(ctx context.Context, commentID, userID string) (bool, error) {
	commentUUID, err := uuid.Parse(strings.TrimSpace(commentID))
	if err != nil {
		return false, fmt.Errorf("%w: invalid comment id", models.ErrInvalidInput)
	}
	userUUID, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return false, fmt.Errorf("%w: invalid user id", models.ErrInvalidInput)
	}

	liked, err := s.repo.LikeComment(ctx, commentUUID, userUUID)
	if err != nil {
		if errors.Is(err, models.ErrCommentNotFound) {
			return false, models.ErrCommentNotFound
		}
		return false, fmt.Errorf("failed to like comment: %w", err)
	}
	return liked, nil
}

func (s *commentService) GetCommentLikeStates(ctx context.Context, userID string, commentIDs []string) (map[string]bool, error) {
	userUUID, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid user id", models.ErrInvalidInput)
	}

	parsed := make([]uuid.UUID, 0, len(commentIDs))
	seen := make(map[uuid.UUID]struct{}, len(commentIDs))
	for _, raw := range commentIDs {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		parsed = append(parsed, id)
	}

	return s.repo.GetCommentLikeStates(ctx, userUUID, parsed)
}
