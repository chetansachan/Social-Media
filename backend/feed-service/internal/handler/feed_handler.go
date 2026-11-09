package handler

import (
	"context"
	"errors"
	"time"

	"github.com/chetansachan/socialflow/feed-service/internal/models"
	"github.com/chetansachan/socialflow/feed-service/internal/services"
	feedpb "github.com/chetansachan/socialflow/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// FeedHandler implements the generated gRPC FeedServiceServer contract.
type FeedHandler struct {
	feedpb.UnimplementedFeedServiceServer
	feedService services.FeedService
}

func NewFeedHandler(feedService services.FeedService) *FeedHandler {
	return &FeedHandler{feedService: feedService}
}

func (h *FeedHandler) GetFeed(ctx context.Context, req *feedpb.GetFeedRequest) (*feedpb.GetFeedResponse, error) {
	if req.GetUserId() == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	result, err := h.feedService.GetFeed(ctx, models.GetFeedQuery{
		UserID:   req.GetUserId(),
		PageSize: req.GetPageSize(),
		Cursor:   req.GetCursor(),
	})
	if err != nil {
		return nil, mapFeedError(err, "failed to get feed")
	}

	posts := make([]*feedpb.FeedPost, 0, len(result.Posts))
	for _, post := range result.Posts {
		updatedAt := ""
		if post.UpdatedAt != nil {
			updatedAt = post.UpdatedAt.Format(time.RFC3339)
		}

		posts = append(posts, &feedpb.FeedPost{
			Id:        post.ID,
			AuthorId:  post.AuthorID,
			Content:   post.Content,
			MediaUrl:  post.MediaURL,
			IsPublic:  post.IsPublic,
			LikeCount: post.LikeCount,
			CreatedAt: post.CreatedAt.Format(time.RFC3339),
			UpdatedAt: updatedAt,
		})
	}

	return &feedpb.GetFeedResponse{
		Posts:      posts,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}, nil
}

func (h *FeedHandler) InvalidateUserFeed(ctx context.Context, req *feedpb.InvalidateUserFeedRequest) (*feedpb.InvalidateUserFeedResponse, error) {
	if req.GetUserId() == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	if err := h.feedService.InvalidateUserFeed(ctx, req.GetUserId()); err != nil {
		return nil, mapFeedError(err, "failed to invalidate feed")
	}

	return &feedpb.InvalidateUserFeedResponse{Success: true}, nil
}

func mapFeedError(err error, fallback string) error {
	switch {
	case errors.Is(err, models.ErrInvalidInput), errors.Is(err, models.ErrInvalidCursor):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, models.ErrFeedUnavailable):
		return status.Error(codes.Unavailable, err.Error())
	default:
		return status.Errorf(codes.Internal, "%s: %v", fallback, err)
	}
}
