package handler

import (
	"context"
	"errors"

	"github.com/chetansachan/socialflow/follow-service/internal/models"
	"github.com/chetansachan/socialflow/follow-service/internal/services"
	followpb "github.com/chetansachan/socialflow/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type FollowHandler struct {
	followpb.UnimplementedFollowServiceServer
	followService services.FollowService
}

func NewFollowHandler(followService services.FollowService) *FollowHandler {
	return &FollowHandler{followService: followService}
}

func (h *FollowHandler) FollowUser(ctx context.Context, req *followpb.FollowUserRequest) (*followpb.FollowUserResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	err := h.followService.FollowUser(ctx, req.FollowerId, req.FollowingId)
	if err != nil {
		if errors.Is(err, models.ErrCannotFollowSelf) {
			return nil, status.Error(codes.InvalidArgument, "cannot follow yourself")
		}
		if errors.Is(err, models.ErrAlreadyFollowing) {
			return nil, status.Error(codes.AlreadyExists, "already following this user")
		}
		if errors.Is(err, models.ErrInvalidID) {
			return nil, status.Errorf(codes.InvalidArgument, "%v", err)
		}
		return nil, status.Errorf(codes.Internal, "failed to follow user: %v", err)
	}

	return &followpb.FollowUserResponse{
		Message: "Successfully followed user",
	}, nil
}

func (h *FollowHandler) UnfollowUser(ctx context.Context, req *followpb.UnfollowUserRequest) (*followpb.UnfollowUserResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	err := h.followService.UnfollowUser(ctx, req.FollowerId, req.FollowingId)
	if err != nil {
		if errors.Is(err, models.ErrRelationNotFound) {
			return nil, status.Error(codes.NotFound, "follow relationship not found")
		}
		if errors.Is(err, models.ErrInvalidID) {
			return nil, status.Errorf(codes.InvalidArgument, "%v", err)
		}
		return nil, status.Errorf(codes.Internal, "failed to unfollow user: %v", err)
	}

	return &followpb.UnfollowUserResponse{
		Message: "Successfully unfollowed user",
	}, nil
}
