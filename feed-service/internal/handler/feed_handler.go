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

	query := models.GetFeedQuery{
		UserID:   req.GetUserId(),
		PageSize: req.GetPageSize(),
		Cursor:   req.GetCursor(),
	}

	result, err := h.feedService.GetFeed(ctx, query)
	if err != nil {
		if errors.Is(err, models.ErrInvalidInput) {
			return nil, status.Errorf(codes.InvalidArgument, "%v", err)
		}
		return nil, status.Errorf(codes.Internal, "failed to get feed: %v", err)
	}

	feedPosts := make([]*feedpb.FeedPost, 0, len(result.Posts))
	for _, p := range result.Posts {
		var updatedAtStr string
		if p.UpdatedAt != nil {
			updatedAtStr = p.UpdatedAt.Format(time.RFC3339)
		}

		feedPosts = append(feedPosts, &feedpb.FeedPost{
			Id:        p.ID,
			AuthorId:  p.AuthorID,
			Content:   p.Content,
			MediaUrl:  p.MediaURL,
			IsPublic:  p.IsPublic,
			LikeCount: p.LikeCount,
			CreatedAt: p.CreatedAt.Format(time.RFC3339),
			UpdatedAt: updatedAtStr,
		})
	}

	return &feedpb.GetFeedResponse{
		Posts:      feedPosts,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}, nil
}

func (h *FeedHandler) InvalidateUserFeed(ctx context.Context, req *feedpb.InvalidateUserFeedRequest) (*feedpb.InvalidateUserFeedResponse, error) {
	if req.GetUserId() == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	if err := h.feedService.InvalidateUserFeed(ctx, req.GetUserId()); err != nil {
		if errors.Is(err, models.ErrInvalidInput) {
			return nil, status.Errorf(codes.InvalidArgument, "%v", err)
		}
		return nil, status.Errorf(codes.Internal, "failed to invalidate feed: %v", err)
	}

	return &feedpb.InvalidateUserFeedResponse{
		Success: true,
	}, nil
}
