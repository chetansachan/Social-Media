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

func formatTimestamp(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
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
			UpdatedAt: formatTimestamp(user.UpdatedAt),
		},
	}, nil
}

func (h *UserHandler) Login(ctx context.Context, req *userpb.LoginRequest) (*userpb.LoginResponse, error) {
	input := models.LoginRequest{
		Email:    req.GetEmail(),
		Password: req.GetPassword(),
	}

	response, err := h.userService.Login(ctx, input)
	if err != nil {
		switch {
		case errors.Is(err, models.ErrInvalidInput):
			return nil, status.Errorf(codes.InvalidArgument, "%v", err)
		case errors.Is(err, models.ErrUserNotFound):
			return nil, status.Errorf(codes.NotFound, "%v", err)
		default:
			return nil, status.Errorf(codes.Internal, "failed to register user: %v", err)
		}
	}

	return &userpb.LoginResponse{
		AccessTkn:  response.AccessTkn,
		RefreshTkn: response.RefreshTkn,
		Message:    response.Message,
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
			UpdatedAt: formatTimestamp(user.UpdatedAt),
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
			UpdatedAt: formatTimestamp(user.UpdatedAt),
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
			UpdatedAt: formatTimestamp(user.UpdatedAt),
		},
	}, nil
}

func (h *UserHandler) Refresh(ctx context.Context, req *userpb.RefreshTknRequest) (*userpb.RefreshTknResponse, error) {
	res, err := h.userService.Refresh(ctx, req.Refreshtkn)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Unauthorized %v", err)
	}

	return &userpb.RefreshTknResponse{
		NewAccessTkn:  res.AccessTkn,
		NewRefreshTkn: res.RefreshTkn,
	}, nil
}

func (h *UserHandler) UpdateUser(ctx context.Context, req *userpb.UpdateUserRequest) (*userpb.UpdateUserResponse, error) {
	input := models.UpdateUserInput{
		Username: req.Username,
		Name:     req.Name,
		Email:    req.Email,
		Password: req.Password,
		IsPublic: req.IsPublic,
	}
	res, err := h.userService.UpdateUser(ctx, input, req.Id)
	if err != nil {
		switch {
		case errors.Is(err, models.ErrInvalidInput):
			return nil, status.Errorf(codes.InvalidArgument, "%v", err)
		case errors.Is(err, models.ErrUserNotFound):
			return nil, status.Errorf(codes.NotFound, "%v", err)
		case errors.Is(err, models.ErrUserAlreadyExists):
			return nil, status.Errorf(codes.AlreadyExists, "%v", err)
		default:
			return nil, status.Errorf(codes.Internal, "failed to update user: %v", err)
		}
	}

	return &userpb.UpdateUserResponse{
		User: &userpb.User{
			Id: res.ID.String(),
			Username: res.Username,
			Name: res.Name,
			Email: res.Email,
			IsPublic: res.IsPublic,
			CreatedAt: res.CreatedAt.Format(time.RFC3339),
			UpdatedAt: formatTimestamp(res.UpdatedAt),
		},
	}, nil
}

func (h *UserHandler) DeleteUser(ctx context.Context, req *userpb.DeleteUserRequest) (*userpb.DeleteUserResponse, error) {
	_, err := h.userService.DeleteUser(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete user: %v", err)
	}

	return &userpb.DeleteUserResponse{
		Message: "User Deleted Successfully",
	}, nil
}