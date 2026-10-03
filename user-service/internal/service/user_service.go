package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"github.com/chetansachan/socialflow/user-service/internal/models"
	"github.com/chetansachan/socialflow/user-service/internal/repository"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type UserService interface {
	Register(ctx context.Context, input models.CreateUserInput) (*models.User, error)
	GetById(ctx context.Context, id string) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	GetByUsername(ctx context.Context, username string) (*models.User, error)
}

type userService struct {
	userRepo repository.UserRepository
}

func NewUserService(userRepo repository.UserRepository) UserService {
	return &userService{userRepo: userRepo}
}

func (s *userService) Register(ctx context.Context, input models.CreateUserInput) (*models.User, error) {
	// Support both username and name
	username := strings.TrimSpace(input.Username)
	name := strings.TrimSpace(input.Name)
	email := strings.TrimSpace(strings.ToLower(input.Email))
	password := input.Password

	// Validate inputs
	if username == "" {
		return nil, fmt.Errorf("%w: name/username is required", models.ErrInvalidInput)
	}
	if len(username) < 3 || len(username) > 50 {
		return nil, fmt.Errorf("%w: username must be between 3 and 50 characters", models.ErrInvalidInput)
	}

	if name == "" {
		return nil, fmt.Errorf(
			"%w: name is required",
			models.ErrInvalidInput,
		)
	}

	if len(name) > 100 {
		return nil, fmt.Errorf(
			"%w: name must be at most 100 characters",
			models.ErrInvalidInput,
		)
	}

	if email == "" {
		return nil, fmt.Errorf("%w: email is required", models.ErrInvalidInput)
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, fmt.Errorf("%w: invalid email address format", models.ErrInvalidInput)
	}

	if len(password) < 6 {
		return nil, fmt.Errorf("%w: password must be at least 6 characters long", models.ErrInvalidInput)
	}

	// Parse / hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Default is_public to true if not specified
	isPublic := true
	if input.IsPublic != nil {
		isPublic = *input.IsPublic
	}

	userToCreate := &models.User{
		Username: username,
		Name:     name,
		Email:    email,
		Password: string(hashedPassword),
		IsPublic: isPublic,
	}

	// Save to DB via repository
	createdUser, err := s.userRepo.Create(ctx, userToCreate)
	if err != nil {
		return nil, err
	}

	// Mask the password before returning to caller
	createdUser.Password = ""

	return createdUser, nil
}

func (s *userService) GetById(ctx context.Context, id string) (*models.User, error) {
	userid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("Error parsing id %w", err)
	}

	user, err := s.userRepo.GetById(ctx, userid)
	if err != nil {
		if errors.Is(err, models.ErrUserNotFound) {
			return nil, models.ErrUserNotFound
		}
		return nil, fmt.Errorf("Internal Server error %w", err)
	}

	user.Password = ""

	return user, nil
}

func (s *userService) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	userEmail := strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return nil, fmt.Errorf("%w: email is required", models.ErrInvalidInput)
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, fmt.Errorf("%w: invalid email address format", models.ErrInvalidInput)
	}

	user, err := s.userRepo.GetByEmail(ctx, userEmail)
	if err != nil {
		if errors.Is(err, models.ErrUserNotFound) {
			return nil, fmt.Errorf("User not found %w", err)
		}
		return nil, fmt.Errorf("Internal Server error %w", err)
	}

	user.Password = ""

	return user, nil
}

func (s *userService) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	Username := strings.TrimSpace(username)
	if username == "" {
		return nil, fmt.Errorf("%w: name/username is required", models.ErrInvalidInput)
	}
	if len(username) < 3 || len(username) > 50 {
		return nil, fmt.Errorf("%w: username must be between 3 and 50 characters", models.ErrInvalidInput)
	}
	user, err := s.userRepo.GetByUsername(ctx, Username)
	if err != nil {
		if errors.Is(err, models.ErrUserNotFound) {
			return nil, fmt.Errorf("User not found %w", err)
		}
		return nil, fmt.Errorf("Internal Server error %w", err)
	}

	user.Password = ""

	return user, nil
}
