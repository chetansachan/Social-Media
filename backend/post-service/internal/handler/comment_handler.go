package handler

import (
	"context"
	"errors"
	"time"

	"github.com/chetansachan/socialflow/post-service/internal/models"
	"github.com/chetansachan/socialflow/post-service/internal/services"
	commentpb "github.com/chetansachan/socialflow/proto"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CommentHandler exposes threaded comments over gRPC. It lives in post-service
// beside PostHandler because a comment write and the post's denormalised
// comment_count change in the same transaction.
type CommentHandler struct {
	commentpb.UnimplementedCommentServiceServer
	commentService services.CommentService
}

func NewCommentHandler(commentService services.CommentService) *CommentHandler {
	return &CommentHandler{commentService: commentService}
}

// commentStatus maps domain errors to the gRPC codes the gateway keys on.
// Order matters: the sentinel errors are independent, but modelling them in one
// place stops the seven RPC methods from drifting apart.
func commentStatus(err error) error {
	switch {
	case errors.Is(err, models.ErrCommentNotFound), errors.Is(err, models.ErrPostNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, models.ErrCommentForbidden):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, models.ErrInvalidInput), errors.Is(err, models.ErrCommentDepthExceeded):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

func toCommentPB(comment *models.Comment) *commentpb.Comment {
	if comment == nil {
		return nil
	}

	parentID := ""
	if comment.ParentID != nil {
		parentID = comment.ParentID.String()
	}

	return &commentpb.Comment{
		Id:         comment.ID.String(),
		PostId:     comment.PostID.String(),
		AuthorId:   comment.AuthorID.String(),
		ParentId:   parentID,
		Content:    comment.Content,
		LikeCount:  int32(comment.LikeCount),
		ReplyCount: int32(comment.ReplyCount),
		Depth:      int32(comment.Depth),
		IsDeleted:  comment.IsDeleted,
		CreatedAt:  comment.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  comment.UpdatedAt.Format(time.RFC3339),
	}
}

func (h *CommentHandler) CreateComment(ctx context.Context, req *commentpb.CreateCommentRequest) (*commentpb.CreateCommentResponse, error) {
	postID, err := uuid.Parse(req.GetPostId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid post id: %v", err)
	}
	authorID, err := uuid.Parse(req.GetAuthorId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid author id: %v", err)
	}

	var parentID *uuid.UUID
	if req.GetParentId() != "" {
		parsed, err := uuid.Parse(req.GetParentId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid parent comment id: %v", err)
		}
		parentID = &parsed
	}

	comment, err := h.commentService.CreateComment(ctx, models.CreateCommentInput{
		PostID:   postID,
		AuthorID: authorID,
		ParentID: parentID,
		Content:  req.GetContent(),
	})
	if err != nil {
		return nil, commentStatus(err)
	}

	return &commentpb.CreateCommentResponse{Comment: toCommentPB(comment)}, nil
}

func (h *CommentHandler) GetComment(ctx context.Context, req *commentpb.GetCommentRequest) (*commentpb.GetCommentResponse, error) {
	comment, err := h.commentService.GetComment(ctx, req.GetId())
	if err != nil {
		return nil, commentStatus(err)
	}

	return &commentpb.GetCommentResponse{Comment: toCommentPB(comment)}, nil
}

func (h *CommentHandler) ListComments(ctx context.Context, req *commentpb.ListCommentsRequest) (*commentpb.ListCommentsResponse, error) {
	page, err := h.commentService.ListComments(
		ctx,
		req.GetPostId(),
		req.GetParentId(),
		req.GetLimit(),
		req.GetCursor(),
	)
	if err != nil {
		return nil, commentStatus(err)
	}

	comments := make([]*commentpb.Comment, 0, len(page.Comments))
	for i := range page.Comments {
		comments = append(comments, toCommentPB(&page.Comments[i]))
	}

	return &commentpb.ListCommentsResponse{
		Comments:   comments,
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore,
	}, nil
}

func (h *CommentHandler) UpdateComment(ctx context.Context, req *commentpb.UpdateCommentRequest) (*commentpb.UpdateCommentResponse, error) {
	comment, err := h.commentService.UpdateComment(
		ctx,
		req.GetCommentId(),
		req.GetAuthorId(),
		req.GetContent(),
	)
	if err != nil {
		return nil, commentStatus(err)
	}

	return &commentpb.UpdateCommentResponse{Comment: toCommentPB(comment)}, nil
}

func (h *CommentHandler) DeleteComment(ctx context.Context, req *commentpb.DeleteCommentRequest) (*commentpb.DeleteCommentResponse, error) {
	if err := h.commentService.DeleteComment(ctx, req.GetCommentId(), req.GetAuthorId()); err != nil {
		return nil, commentStatus(err)
	}

	return &commentpb.DeleteCommentResponse{Message: "comment deleted successfully"}, nil
}

func (h *CommentHandler) LikeComment(ctx context.Context, req *commentpb.LikeCommentRequest) (*commentpb.LikeCommentResponse, error) {
	isLiked, err := h.commentService.LikeComment(ctx, req.GetCommentId(), req.GetUserId())
	if err != nil {
		return nil, commentStatus(err)
	}

	message := "comment unliked successfully"
	if isLiked {
		message = "comment liked successfully"
	}

	return &commentpb.LikeCommentResponse{IsLiked: isLiked, Message: message}, nil
}

func (h *CommentHandler) GetCommentLikeStates(ctx context.Context, req *commentpb.GetCommentLikeStatesRequest) (*commentpb.GetCommentLikeStatesResponse, error) {
	states, err := h.commentService.GetCommentLikeStates(ctx, req.GetUserId(), req.GetCommentIds())
	if err != nil {
		return nil, commentStatus(err)
	}

	return &commentpb.GetCommentLikeStatesResponse{Liked: states}, nil
}
