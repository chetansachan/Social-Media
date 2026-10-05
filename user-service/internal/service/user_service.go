package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/chetansachan/socialflow/user-service/internal/config"
	"github.com/chetansachan/socialflow/user-service/internal/models"
	"github.com/chetansachan/socialflow/user-service/internal/repository"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type UserService interface {
	Register(ctx context.Context, input models.CreateUserInput) (*models.User, error)
	Login(ctx context.Context, input models.LoginRequest) (*models.LoginResponse, error)
	GetById(ctx context.Context, id string) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	GetByUsername(ctx context.Context, username string) (*models.User, error)
	Refresh(ctx context.Context, tkn string) (*models.RotationResponse, error)
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

func (s *userService) Login(ctx context.Context, input models.LoginRequest) (*models.LoginResponse, error) {
	email := strings.TrimSpace(strings.ToLower(input.Email))
	password := input.Password

	if email == "" {
		return nil, fmt.Errorf("%w: email is required", models.ErrInvalidInput)
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, fmt.Errorf("%w: invalid email address format", models.ErrInvalidInput)
	}

	if password == "" {
		return nil, fmt.Errorf("%w: password is required", models.ErrInvalidInput)
	}

	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, models.ErrUserNotFound) {
			return nil, fmt.Errorf("User not found %w", err)
		}
		return nil, fmt.Errorf("Internal Server error %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, fmt.Errorf("Invalid Password %w", err)
	}

	cfg := config.Load()
	mySecretKey := cfg.SecretKey

	access := jwt.NewWithClaims(jwt.SigningMethodHS256, models.MyCustomClaims{
		UserId: user.ID,
		Email:  user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
	refresh := jwt.NewWithClaims(jwt.SigningMethodHS256, models.MyCustomClaims{
		UserId: user.ID,
		Email:  user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * 7 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
	accessToken, err := access.SignedString([]byte(mySecretKey))
	if err != nil {
		return nil, fmt.Errorf("Error gernating tkn %w", err)
	}
	refreshToken, err := refresh.SignedString([]byte(mySecretKey))
	if err != nil {
		return nil, fmt.Errorf("Error gernating tkn %w", err)
	}

	hash := sha256.Sum256([]byte(refreshToken))
	refreshtkn := hex.EncodeToString(hash[:])

	data := &models.RefreshTkn{
		UserId:    user.ID,
		Token:     refreshtkn,
		ExpiresAt: time.Now().Add(24 * 7 * time.Hour),
	}

	if err := s.userRepo.SaveToken(ctx, data); err != nil {
		return nil, fmt.Errorf("Error saving token %w", err)
	}
	return &models.LoginResponse{
		AccessTkn:  accessToken,
		RefreshTkn: refreshToken,
		Message:    "Login Successfully",
	}, nil
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

func (s *userService) Refresh(ctx context.Context, tkn string) (*models.RotationResponse, error) {
	hash := sha256.Sum256([]byte(tkn))
	oldhashtkn := hex.EncodeToString(hash[:])

	data, err := s.userRepo.GetRefreshTk(ctx, string(oldhashtkn))
	if err != nil {
		return nil, fmt.Errorf("token not found: %w", err)
	}

	if data.IsRevoked || time.Now().After(data.ExpiresAt) {
		return nil, errors.New("unauthorized: token is revoked or expired")
	}

	cfg := config.Load()

	token, err := jwt.ParseWithClaims(tkn, &models.MyCustomClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(cfg.SecretKey), nil
	})
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*models.MyCustomClaims)
	if !ok {
		return nil, errors.New("unknown claims type, cannot proceed")
	}

	access := jwt.NewWithClaims(jwt.SigningMethodHS256, models.MyCustomClaims{
		UserId: claims.UserId,
		Email:  claims.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})

	refresh := jwt.NewWithClaims(jwt.SigningMethodHS256, models.MyCustomClaims{
		UserId: claims.UserId,
		Email:  claims.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * 7 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})

	accessToken, err := access.SignedString([]byte(cfg.SecretKey))
	if err != nil {
		return nil, fmt.Errorf("Error gernating tkn %w", err)
	}
	refreshToken, err := refresh.SignedString([]byte(cfg.SecretKey))
	if err != nil {
		return nil, fmt.Errorf("Error gernating tkn %w", err)
	}

	newHash := sha256.Sum256([]byte(refreshToken))
	refreshtkn := hex.EncodeToString(newHash[:])
	now := time.Now()

	newData := &models.RefreshTkn{
		UserId:    claims.UserId,
		Token:     refreshtkn,
		ExpiresAt: time.Now().Add(24 * 7 * time.Hour),
		UpdatedAt: &now,
	}

	if err := s.userRepo.RotateToken(ctx, oldhashtkn, refreshtkn, newData); err != nil {
		return nil, fmt.Errorf("failed to save token: %w", err)
	}

	return &models.RotationResponse{
		AccessTkn:  accessToken,
		RefreshTkn: refreshToken,
	}, nil
}
