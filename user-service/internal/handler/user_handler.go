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

func (h *UserHandler) GetUserById(ctx context.Context, req *userpb.GetUserByIdRequest) (*userpb.GetUserByIdResponse, error) {
	user, err := h.userService.GetById(ctx, req.Id)
	if err != nil {
		if errors.Is(err, models.ErrUserNotFound) {
			return nil, models.ErrUserNotFound
		}
		return nil, status.Errorf(codes.Internal, "failed to find user %v", err)
	}

	return &userpb.GetUserByIdResponse{
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

func (h *UserHandler) GetUserByEmail(ctx context.Context, req *userpb.GetUserByEmailRequest) (*userpb.GetUserByEmailResponse, error) {
	user, err := h.userService.GetByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, models.ErrUserNotFound) {
			return nil, models.ErrUserNotFound
		}
		return nil, status.Errorf(codes.Internal, "failed to find user %v", err)
	}

	return &userpb.GetUserByEmailResponse{
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

func (h *UserHandler) GetUserByUsername(ctx context.Context, req *userpb.GetUserByUsernameRequest) (*userpb.GetUserByUsernameResponse, error) {
	user, err := h.userService.GetByUsername(ctx, req.Username)
	if err != nil {
		if errors.Is(err, models.ErrUserNotFound) {
			return nil, models.ErrUserNotFound
		}
		return nil, status.Errorf(codes.Internal, "failed to find user %v", err)
	}

	return &userpb.GetUserByUsernameResponse{
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
