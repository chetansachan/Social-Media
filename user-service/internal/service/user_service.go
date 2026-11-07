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
	"unicode"

	"github.com/chetansachan/socialflow/user-service/internal/cache"
	"github.com/chetansachan/socialflow/user-service/internal/config"
	"github.com/chetansachan/socialflow/user-service/internal/models"
	"github.com/chetansachan/socialflow/user-service/internal/repository"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/sync/singleflight"
)

type UserService interface {
	Register(ctx context.Context, input models.CreateUserInput) (*models.User, error)
	Login(ctx context.Context, input models.LoginRequest) (*models.LoginResponse, error)
	GetById(ctx context.Context, id string) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	GetByUsername(ctx context.Context, username string) (*models.User, error)
	GetByIds(ctx context.Context, ids []string) ([]models.User, error)
	Refresh(ctx context.Context, tkn string) (*models.RotationResponse, error)
	UpdateUser(ctx context.Context, input models.UpdateUserInput, id string) (*models.User, error)
	DeleteUser(ctx context.Context, id string) (*models.DeleteUserRespose, error)
	SearchUsers(ctx context.Context, query string, limit int32) ([]models.User, error)
}

type userService struct {
	userRepo repository.UserRepository

	// cache is optional. Without Redis the database is still protected by the
	// singleflight group below.
	cache    cache.UserCache
	cacheTTL time.Duration

	// profileGroup collapses concurrent misses for the same user into one
	// database query. This is what keeps a cold cache from stampeding
	// PostgreSQL when a popular profile is requested by thousands of clients.
	profileGroup singleflight.Group
}

// Option customizes the service without breaking existing callers.
type Option func(*userService)

// WithUserCache enables the shared profile cache and sets its TTL.
func WithUserCache(userCache cache.UserCache, ttl time.Duration) Option {
	return func(s *userService) {
		if ttl <= 0 {
			ttl = 10 * time.Minute
		}
		s.cache = userCache
		s.cacheTTL = ttl
	}
}

// bcryptCost is above the library default so a leaked password table stays
// expensive to brute-force; bcrypt records the cost inside each hash, so
// previously issued hashes keep verifying.
const bcryptCost = 12

// minPasswordLength is the shortest password accepted at registration and
// during a password change.
const minPasswordLength = 8

// Search result sets stay small: autocomplete wants the handful of best
// matches, and each row is hydrated into a profile card by the caller.
const (
	defaultSearchPageSize = 20
	maxSearchPageSize     = 50
)

func NewUserService(userRepo repository.UserRepository, opts ...Option) UserService {
	service := &userService{userRepo: userRepo}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service
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

	if len(password) < minPasswordLength {
		return nil, fmt.Errorf("%w: password must be at least %d characters long", models.ErrInvalidInput, minPasswordLength)
	}
	if err := validatePasswordComplexity(password); err != nil {
		return nil, err
	}

	// Parse / hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
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
			return nil, models.ErrInvalidCredentials
		}
		return nil, fmt.Errorf("internal server error: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, models.ErrInvalidCredentials
	}

	cfg := config.Load()
	mySecretKey := cfg.SecretKey

	access := jwt.NewWithClaims(jwt.SigningMethodHS256, models.MyCustomClaims{
		UserId: user.ID,
		Email:  user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			// A unique jti makes every rotation observably distinct even when
			// two tokens are minted inside the same second, and gives the
			// gateway a handle for revocation.
			ID:        uuid.NewString(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
	refresh := jwt.NewWithClaims(jwt.SigningMethodHS256, models.MyCustomClaims{
		UserId: user.ID,
		Email:  user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
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

// GetById returns a public profile. Redis serves repeated reads; concurrent
// misses for the same id are collapsed into one repository query so a cold
// cache cannot stampede PostgreSQL.
func (s *userService) GetById(ctx context.Context, id string) (*models.User, error) {
	userid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("Error parsing id %w", err)
	}

	user, err := s.loadProfile(ctx, userid)
	if err != nil {
		if errors.Is(err, models.ErrUserNotFound) {
			return nil, models.ErrUserNotFound
		}
		return nil, fmt.Errorf("Internal Server error %w", err)
	}

	return user, nil
}

func (s *userService) loadProfile(ctx context.Context, id uuid.UUID) (*models.User, error) {
	if s.cache != nil {
		if user, err := s.cache.Get(ctx, id); err == nil {
			return cloneUser(user), nil
		}
	}

	value, err, _ := s.profileGroup.Do(id.String(), func() (any, error) {
		// The load outlives the request that happened to win the race: if
		// that client disconnects, the remaining waiters still get their
		// result and the cache still gets filled.
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()

		// A previous winner may have populated the cache between this
		// caller's miss and its entry into the group.
		if s.cache != nil {
			if user, err := s.cache.Get(loadCtx, id); err == nil {
				return cloneUser(user), nil
			}
		}

		user, err := s.userRepo.GetById(loadCtx, id)
		if err != nil {
			return nil, err
		}
		user.Password = ""
		if s.cache != nil {
			_ = s.cache.Set(loadCtx, user, s.cacheTTL)
		}
		return user, nil
	})
	if err != nil {
		return nil, err
	}

	user, ok := value.(*models.User)
	if !ok || user == nil {
		return nil, models.ErrUserNotFound
	}
	// singleflight hands the same pointer to every waiter; return a copy so
	// one caller cannot mutate another caller's response.
	return cloneUser(user), nil
}

func cloneUser(user *models.User) *models.User {
	if user == nil {
		return nil
	}
	copied := *user
	copied.Password = ""
	return &copied
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

// GetByIds validates and de-duplicates ids before issuing a single query.
// Unknown or malformed ids are skipped rather than failing the whole batch:
// a deleted user must not break a feed page.
//
// The cache absorbs the steady state (every feed page hydrates the same
// authors). Only ids that missed are fetched, and those are written back as a
// batch so the next page is a pure cache read.
func (s *userService) GetByIds(ctx context.Context, ids []string) ([]models.User, error) {
	unique := make([]uuid.UUID, 0, len(ids))
	seen := make(map[uuid.UUID]struct{}, len(ids))
	for _, raw := range ids {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}

	if len(unique) == 0 {
		return []models.User{}, nil
	}

	cached := make(map[uuid.UUID]*models.User, len(unique))
	if s.cache != nil {
		if found, err := s.cache.GetMany(ctx, unique); err == nil {
			cached = found
		}
	}

	missing := make([]uuid.UUID, 0, len(unique))
	for _, id := range unique {
		if _, ok := cached[id]; !ok {
			missing = append(missing, id)
		}
	}

	if len(missing) > 0 {
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()

		loaded, err := s.userRepo.GetByIds(loadCtx, missing)
		if err != nil {
			return nil, err
		}
		for i := range loaded {
			loaded[i].Password = ""
			cached[loaded[i].ID] = &loaded[i]
		}
		if s.cache != nil && len(loaded) > 0 {
			_ = s.cache.SetMany(loadCtx, loaded, s.cacheTTL)
		}
	}

	// Preserve the caller's id order; the repository does not guarantee one.
	users := make([]models.User, 0, len(unique))
	for _, id := range unique {
		if user, ok := cached[id]; ok && user != nil {
			users = append(users, *cloneUser(user))
		}
	}
	return users, nil
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
			ID:        uuid.NewString(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})

	refresh := jwt.NewWithClaims(jwt.SigningMethodHS256, models.MyCustomClaims{
		UserId: claims.UserId,
		Email:  claims.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
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

func (s *userService) UpdateUser(ctx context.Context, input models.UpdateUserInput, id string) (*models.User, error) {
	userid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid user id format: %v", models.ErrInvalidInput, err)
	}
	user, err := s.userRepo.GetById(ctx, userid)
	if err != nil {
		if errors.Is(err, models.ErrUserNotFound) {
			return nil, models.ErrUserNotFound
		}
		return nil, fmt.Errorf("internal server error: %w", err)
	}

	username := user.Username
	if input.Username != nil {
		trimmed := strings.TrimSpace(*input.Username)
		if trimmed == "" || len(trimmed) < 3 || len(trimmed) > 50 {
			return nil, fmt.Errorf("%w: username must be between 3 and 50 characters", models.ErrInvalidInput)
		}
		username = trimmed
	}

	name := user.Name
	if input.Name != nil {
		trimmed := strings.TrimSpace(*input.Name)
		if trimmed == "" {
			return nil, fmt.Errorf("%w: name cannot be empty", models.ErrInvalidInput)
		}
		if len(trimmed) > 100 {
			return nil, fmt.Errorf("%w: name must be at most 100 characters", models.ErrInvalidInput)
		}
		name = trimmed
	}

	email := user.Email
	if input.Email != nil {
		trimmed := strings.TrimSpace(strings.ToLower(*input.Email))
		if trimmed == "" {
			return nil, fmt.Errorf("%w: email cannot be empty", models.ErrInvalidInput)
		}
		if _, err := mail.ParseAddress(trimmed); err != nil {
			return nil, fmt.Errorf("%w: invalid email address format", models.ErrInvalidInput)
		}
		email = trimmed
	}

	password := user.Password
	if input.Password != nil && *input.Password != "" {
		if len(*input.Password) < minPasswordLength {
			return nil, fmt.Errorf("%w: password must be at least %d characters long", models.ErrInvalidInput, minPasswordLength)
		}
		if err := validatePasswordComplexity(*input.Password); err != nil {
			return nil, err
		}
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(*input.Password), bcryptCost)
		if err != nil {
			return nil, fmt.Errorf("failed to hash password: %w", err)
		}
		password = string(hashedPassword)
	}

	isPublic := user.IsPublic
	if input.IsPublic != nil {
		isPublic = *input.IsPublic
	}

	updatedUserCreds := models.UpdateUserInput{
		Username: &username,
		Name:     &name,
		Email:    &email,
		Password: &password,
		IsPublic: &isPublic,
	}

	updatedUser, err := s.userRepo.UpdateUser(ctx, &updatedUserCreds, userid)
	if err != nil {
		if errors.Is(err, models.ErrUserNotFound) || errors.Is(err, models.ErrUserAlreadyExists) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	s.invalidateProfile(ctx, userid)

	updatedUser.Password = ""

	return updatedUser, nil
}

// SearchUsers validates and clamps an autocomplete query, then masks the
// password column the repository selects alongside the profile fields. The
// hash never crosses the service boundary.
func (s *userService) SearchUsers(ctx context.Context, query string, limit int32) ([]models.User, error) {
	normalized, ok := models.NormalizeSearchQuery(query)
	if !ok {
		return nil, fmt.Errorf("%w: search query must be 1-%d characters", models.ErrInvalidInput, models.MaxSearchQueryLength)
	}

	pageSize := int(limit)
	if pageSize <= 0 {
		pageSize = defaultSearchPageSize
	}
	if pageSize > maxSearchPageSize {
		pageSize = maxSearchPageSize
	}

	users, err := s.userRepo.SearchUsers(ctx, normalized, pageSize)
	if err != nil {
		return nil, fmt.Errorf("failed to search users: %w", err)
	}

	for i := range users {
		users[i].Password = ""
	}
	if users == nil {
		users = []models.User{}
	}
	return users, nil
}

func (s *userService) DeleteUser(ctx context.Context, id string) (*models.DeleteUserRespose, error) {
	userId, err := uuid.Parse(id)
	if err != nil {
		return &models.DeleteUserRespose{}, fmt.Errorf("Unable to parse the id: %v", err)
	}

	if err := s.userRepo.DeleteUser(ctx, userId); err != nil {
		return &models.DeleteUserRespose{}, fmt.Errorf("Error Deleting User: %v", err)
	}

	s.invalidateProfile(ctx, userId)

	return &models.DeleteUserRespose{
		Message: "User Deleted Successfuly",
	}, nil
}

// invalidateProfile drops a cached profile after a write. It uses a bounded
// context detached from the request so a client disconnecting mid-response
// cannot leave a stale entry behind.
func (s *userService) invalidateProfile(ctx context.Context, id uuid.UUID) {
	if s.cache == nil {
		return
	}
	cacheCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	_ = s.cache.Delete(cacheCtx, id)
}

// validatePasswordComplexity requires both a letter and a digit. The rule is
// intentionally predictable rather than blocking symbols, and it mirrors the
// client-side hint in the auth screens.
func validatePasswordComplexity(password string) error {
	var hasLetter, hasDigit bool
	for _, r := range password {
		switch {
		case unicode.IsLetter(r):
			hasLetter = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return fmt.Errorf("%w: password must contain at least one letter and one number", models.ErrInvalidInput)
	}
	return nil
}
