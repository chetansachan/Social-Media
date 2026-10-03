package handler

import (
	"context"
	"errors"
	"time"

	userpb "github.com/chetansachan/socialflow/proto"
	"github.com/chetansachan/socialflow/user-service/internal/models"
	"github.com/chetansachan/socialflow/user-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type UserHandler struct {
	userpb.UnimplementedUserServiceServer
	userService service.UserService
}

func NewUserHandler(userService service.UserService) *UserHandler {
	return &UserHandler{userService: userService}
}

func (h *UserHandler) Register(ctx context.Context, req *userpb.RegisterUserRequest) (*userpb.RegisterUserResponse, error) {
	input := models.CreateUserInput{
		Username: req.GetUsername(),
		Name:     req.GetName(),
		Email:    req.GetEmail(),
		Password: req.GetPassword(),
		IsPublic: req.IsPublic,
	}

	user, err := h.userService.Register(ctx, input)
	if err != nil {
		switch {
		case errors.Is(err, models.ErrUserAlreadyExists):
			return nil, status.Errorf(codes.AlreadyExists, "%v", err)
		case errors.Is(err, models.ErrInvalidInput):
			return nil, status.Errorf(codes.InvalidArgument, "%v", err)
		default:
			return nil, status.Errorf(codes.Internal, "failed to register user: %v", err)
		}
	}

	return &userpb.RegisterUserResponse{
		User: &userpb.User{
			Id:        user.ID.String(),
			Username:  user.Username,
			Name:      user.Name,
			Email:     user.Email,
			IsPublic:  user.IsPublic,
			CreatedAt: user.CreatedAt.Format(time.RFC3339),
			UpdatedAt: user.UpdatedAt.Format(time.RFC3339),
		},
	}, nil
}
