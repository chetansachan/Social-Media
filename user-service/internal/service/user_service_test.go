package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/chetansachan/socialflow/user-service/internal/models"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type fakeUserRepo struct {
	createErr  error
	updateErr  error
	deleteErr  error
	byEmail    map[string]*models.User
	byID       map[uuid.UUID]*models.User
	tokens     []*models.RefreshTkn
	rotatedOld string
	rotatedNew string
	updated    *models.UpdateUserInput
	updatedID  uuid.UUID
	deleted    uuid.UUID
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{
		byEmail: make(map[string]*models.User),
		byID:    make(map[uuid.UUID]*models.User),
	}
}

func (r *fakeUserRepo) Create(ctx context.Context, user *models.User) (*models.User, error) {
	if r.createErr != nil {
		return nil, r.createErr
	}
	stored := *user
	stored.ID = uuid.New()
	stored.CreatedAt = time.Now().UTC()
	r.byEmail[stored.Email] = &stored
	r.byID[stored.ID] = &stored

	// Mirror pgx: the caller receives its own struct, so masking the password
	// on the returned value cannot mutate the persisted row.
	result := stored
	return &result, nil
}

func (r *fakeUserRepo) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	user, ok := r.byEmail[email]
	if !ok {
		return nil, models.ErrUserNotFound
	}
	copy := *user
	return &copy, nil
}

func (r *fakeUserRepo) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	for _, user := range r.byID {
		if user.Username == username {
			copy := *user
			return &copy, nil
		}
	}
	return nil, models.ErrUserNotFound
}

func (r *fakeUserRepo) GetById(ctx context.Context, id uuid.UUID) (*models.User, error) {
	user, ok := r.byID[id]
	if !ok {
		return nil, models.ErrUserNotFound
	}
	copy := *user
	return &copy, nil
}

func (r *fakeUserRepo) GetByIds(ctx context.Context, ids []uuid.UUID) ([]models.User, error) {
	users := make([]models.User, 0, len(ids))
	for _, id := range ids {
		if user, ok := r.byID[id]; ok {
			users = append(users, *user)
		}
	}
	return users, nil
}

func (r *fakeUserRepo) SaveToken(ctx context.Context, data *models.RefreshTkn) error {
	stored := *data
	r.tokens = append(r.tokens, &stored)
	return nil
}

func (r *fakeUserRepo) GetRefreshTk(ctx context.Context, tkn string) (*models.RefreshTkn, error) {
	for _, token := range r.tokens {
		if token.Token == tkn {
			copy := *token
			return &copy, nil
		}
	}
	return nil, models.ErrUserNotFound
}

func (r *fakeUserRepo) RotateToken(ctx context.Context, oldtkn string, newtkn string, data *models.RefreshTkn) error {
	r.rotatedOld = oldtkn
	r.rotatedNew = newtkn
	for _, token := range r.tokens {
		if token.Token == oldtkn {
			token.IsRevoked = true
		}
	}
	stored := *data
	r.tokens = append(r.tokens, &stored)
	return nil
}

func (r *fakeUserRepo) UpdateUser(ctx context.Context, input *models.UpdateUserInput, id uuid.UUID) (*models.User, error) {
	if r.updateErr != nil {
		return nil, r.updateErr
	}
	r.updated = input
	r.updatedID = id

	user, ok := r.byID[id]
	if !ok {
		return nil, models.ErrUserNotFound
	}
	if input.Username != nil {
		user.Username = *input.Username
	}
	if input.Name != nil {
		user.Name = *input.Name
	}
	if input.Email != nil {
		user.Email = *input.Email
	}
	if input.Password != nil {
		user.Password = *input.Password
	}
	if input.IsPublic != nil {
		user.IsPublic = *input.IsPublic
	}
	copy := *user
	return &copy, nil
}

func (r *fakeUserRepo) DeleteUser(ctx context.Context, id uuid.UUID) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	r.deleted = id
	delete(r.byID, id)
	return nil
}

func (r *fakeUserRepo) seed(t *testing.T, user *models.User) {
	t.Helper()
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	r.byEmail[user.Email] = user
	r.byID[user.ID] = user
}

func newService(t *testing.T) (*userService, *fakeUserRepo) {
	t.Helper()
	t.Setenv("MYSECRETKEY", "test-secret")

	repo := newFakeUserRepo()
	return NewUserService(repo).(*userService), repo
}

func hashPassword(t *testing.T, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	return string(hash)
}

func TestRegisterSuccess(t *testing.T) {
	svc, repo := newService(t)

	user, err := svc.Register(context.Background(), models.CreateUserInput{
		Username: "chetan",
		Name:     "Chetan Sachan",
		Email:    "  Chetan@Example.COM ",
		Password: "str0ngpass",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if user.ID == uuid.Nil {
		t.Fatal("registered user has no id")
	}
	if user.Email != "chetan@example.com" {
		t.Fatalf("email = %q, want a normalized address", user.Email)
	}
	if user.Password != "" {
		t.Fatal("password hash leaked to the caller")
	}
	if !user.IsPublic {
		t.Fatal("is_public should default to true")
	}

	stored := repo.byEmail[user.Email]
	if stored == nil {
		t.Fatal("user was not persisted")
	}
	if stored.Password == "" || stored.Password == "str0ngpass" {
		t.Fatal("password was stored without hashing")
	}
	cost, err := bcrypt.Cost([]byte(stored.Password))
	if err != nil {
		t.Fatalf("bcrypt cost: %v", err)
	}
	if cost != bcryptCost {
		t.Fatalf("bcrypt cost = %d, want %d", cost, bcryptCost)
	}
}

func TestRegisterRejectsDuplicateUser(t *testing.T) {
	svc, repo := newService(t)
	repo.createErr = models.ErrUserAlreadyExists

	_, err := svc.Register(context.Background(), models.CreateUserInput{
		Username: "chetan",
		Name:     "Chetan",
		Email:    "chetan@example.com",
		Password: "str0ngpass",
	})
	if !errors.Is(err, models.ErrUserAlreadyExists) {
		t.Fatalf("err = %v, want ErrUserAlreadyExists", err)
	}
}

func TestRegisterRejectsInvalidEmail(t *testing.T) {
	svc, _ := newService(t)

	for _, email := range []string{"invalid-email", "@domain.com", "user@", ""} {
		_, err := svc.Register(context.Background(), models.CreateUserInput{
			Username: "chetan",
			Name:     "Chetan",
			Email:    email,
			Password: "str0ngpass",
		})
		if !errors.Is(err, models.ErrInvalidInput) {
			t.Fatalf("email %q: err = %v, want ErrInvalidInput", email, err)
		}
	}
}

func TestRegisterRejectsWeakPassword(t *testing.T) {
	svc, _ := newService(t)

	cases := map[string]string{
		"too short": "abc123",
		"no digit":  "onlyletters",
		"no letter": "12345678",
		"empty":     "",
	}
	for name, password := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := svc.Register(context.Background(), models.CreateUserInput{
				Username: "chetan",
				Name:     "Chetan",
				Email:    "chetan@example.com",
				Password: password,
			})
			if !errors.Is(err, models.ErrInvalidInput) {
				t.Fatalf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestLoginReturnsSignedTokens(t *testing.T) {
	svc, repo := newService(t)
	user := &models.User{
		Username: "chetan",
		Name:     "Chetan",
		Email:    "chetan@example.com",
		Password: hashPassword(t, "str0ngpass"),
		IsPublic: true,
	}
	repo.seed(t, user)

	res, err := svc.Login(context.Background(), models.LoginRequest{
		Email:    "CHETAN@example.com",
		Password: "str0ngpass",
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.AccessTkn == "" || res.RefreshTkn == "" {
		t.Fatal("login did not return both tokens")
	}

	claims := &models.MyCustomClaims{}
	parsed, err := jwt.ParseWithClaims(res.AccessTkn, claims, func(token *jwt.Token) (any, error) {
		return []byte("test-secret"), nil
	})
	if err != nil || !parsed.Valid {
		t.Fatalf("access token is not valid: %v", err)
	}
	if claims.UserId != user.ID || claims.Email != "chetan@example.com" {
		t.Fatalf("claims = %+v", claims)
	}
	if claims.ExpiresAt == nil || time.Until(claims.ExpiresAt.Time) > 15*time.Minute {
		t.Fatalf("access token expiry = %v", claims.ExpiresAt)
	}

	if len(repo.tokens) != 1 {
		t.Fatalf("saved refresh tokens = %d, want 1", len(repo.tokens))
	}
	hash := sha256.Sum256([]byte(res.RefreshTkn))
	if repo.tokens[0].Token != hex.EncodeToString(hash[:]) {
		t.Fatal("refresh token was not stored hashed")
	}
}

func TestLoginRejectsUnknownEmailAndWrongPasswordAlike(t *testing.T) {
	svc, repo := newService(t)
	user := &models.User{
		Username: "chetan",
		Name:     "Chetan",
		Email:    "chetan@example.com",
		Password: hashPassword(t, "str0ngpass"),
		IsPublic: true,
	}
	repo.seed(t, user)

	if _, err := svc.Login(context.Background(), models.LoginRequest{
		Email: "chetan@example.com", Password: "wrongpass1",
	}); !errors.Is(err, models.ErrInvalidCredentials) {
		t.Fatalf("wrong password err = %v, want ErrInvalidCredentials", err)
	}

	if _, err := svc.Login(context.Background(), models.LoginRequest{
		Email: "nobody@example.com", Password: "str0ngpass",
	}); !errors.Is(err, models.ErrInvalidCredentials) {
		t.Fatalf("unknown email err = %v, want ErrInvalidCredentials", err)
	}
}

func TestRefreshRotatesTokens(t *testing.T) {
	svc, repo := newService(t)
	user := &models.User{
		Username: "chetan",
		Name:     "Chetan",
		Email:    "chetan@example.com",
		Password: hashPassword(t, "str0ngpass"),
		IsPublic: true,
	}
	repo.seed(t, user)

	login, err := svc.Login(context.Background(), models.LoginRequest{
		Email: "chetan@example.com", Password: "str0ngpass",
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	res, err := svc.Refresh(context.Background(), login.RefreshTkn)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if res.AccessTkn == "" || res.RefreshTkn == "" {
		t.Fatal("refresh did not return both tokens")
	}
	if res.RefreshTkn == login.RefreshTkn {
		t.Fatal("refresh token was not rotated")
	}

	oldHash := sha256.Sum256([]byte(login.RefreshTkn))
	if repo.rotatedOld != hex.EncodeToString(oldHash[:]) {
		t.Fatal("the old refresh hash was not rotated")
	}
	if len(repo.tokens) != 2 || !repo.tokens[0].IsRevoked {
		t.Fatalf("tokens = %+v", repo.tokens)
	}
}

func TestRefreshRejectsRevokedToken(t *testing.T) {
	svc, repo := newService(t)
	user := &models.User{
		Username: "chetan",
		Name:     "Chetan",
		Email:    "chetan@example.com",
		Password: hashPassword(t, "str0ngpass"),
		IsPublic: true,
	}
	repo.seed(t, user)

	login, err := svc.Login(context.Background(), models.LoginRequest{
		Email: "chetan@example.com", Password: "str0ngpass",
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	repo.tokens[0].IsRevoked = true

	if _, err := svc.Refresh(context.Background(), login.RefreshTkn); err == nil {
		t.Fatal("expected a revoked refresh token to be rejected")
	}
}

func TestRefreshRejectsUnknownToken(t *testing.T) {
	svc, _ := newService(t)

	if _, err := svc.Refresh(context.Background(), "not-a-token"); err == nil {
		t.Fatal("expected an unknown refresh token to be rejected")
	}
}

func TestUpdateUserPartialUpdateKeepsOtherFields(t *testing.T) {
	svc, repo := newService(t)
	originalHash := hashPassword(t, "str0ngpass")
	user := &models.User{
		Username: "chetan",
		Name:     "Old Name",
		Email:    "chetan@example.com",
		Password: originalHash,
		IsPublic: true,
	}
	repo.seed(t, user)

	name := "  New Name  "
	updated, err := svc.UpdateUser(context.Background(), models.UpdateUserInput{Name: &name}, user.ID.String())
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}

	if updated.Name != "New Name" {
		t.Fatalf("name = %q", updated.Name)
	}
	if updated.Email != "chetan@example.com" {
		t.Fatalf("email = %q, want the stored address", updated.Email)
	}
	if updated.Password != "" {
		t.Fatal("password hash leaked from UpdateUser")
	}
	stored := repo.byID[user.ID]
	if stored.Password != originalHash {
		t.Fatal("password was overwritten by a profile-only update")
	}
	if repo.updated.Username == nil || *repo.updated.Username != "chetan" {
		t.Fatalf("username = %v, want the stored username", repo.updated.Username)
	}
}

func TestUpdateUserRejectsInvalidPassword(t *testing.T) {
	svc, repo := newService(t)
	user := &models.User{
		Username: "chetan",
		Name:     "Chetan",
		Email:    "chetan@example.com",
		Password: hashPassword(t, "str0ngpass"),
		IsPublic: true,
	}
	repo.seed(t, user)

	short := "abc1"
	if _, err := svc.UpdateUser(context.Background(), models.UpdateUserInput{Password: &short}, user.ID.String()); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestUpdateUserUnknownUser(t *testing.T) {
	svc, _ := newService(t)

	name := "New Name"
	_, err := svc.UpdateUser(context.Background(), models.UpdateUserInput{Name: &name}, uuid.New().String())
	if !errors.Is(err, models.ErrUserNotFound) {
		t.Fatalf("err = %v, want ErrUserNotFound", err)
	}
}

func TestGetByIdHidesPassword(t *testing.T) {
	svc, repo := newService(t)
	user := &models.User{
		Username: "chetan",
		Name:     "Chetan",
		Email:    "chetan@example.com",
		Password: hashPassword(t, "str0ngpass"),
		IsPublic: true,
	}
	repo.seed(t, user)

	found, err := svc.GetById(context.Background(), user.ID.String())
	if err != nil {
		t.Fatalf("GetById: %v", err)
	}
	if found.Password != "" {
		t.Fatal("password hash leaked from GetById")
	}
}

func TestGetByIdsSkipsMalformedAndMasksPasswords(t *testing.T) {
	svc, repo := newService(t)
	first := &models.User{
		Username: "first", Name: "First", Email: "first@example.com",
		Password: hashPassword(t, "str0ngpass"), IsPublic: true,
	}
	second := &models.User{
		Username: "second", Name: "Second", Email: "second@example.com",
		Password: hashPassword(t, "str0ngpass"), IsPublic: true,
	}
	repo.seed(t, first)
	repo.seed(t, second)

	users, err := svc.GetByIds(context.Background(), []string{
		first.ID.String(),
		"not-a-uuid",
		second.ID.String(),
		first.ID.String(),
	})
	if err != nil {
		t.Fatalf("GetByIds: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("users = %d, want 2 after de-duplication", len(users))
	}
	for _, user := range users {
		if user.Password != "" {
			t.Fatalf("password hash leaked for %s", user.Username)
		}
	}
}

func TestDeleteUser(t *testing.T) {
	svc, repo := newService(t)
	user := &models.User{
		Username: "chetan", Name: "Chetan", Email: "chetan@example.com",
		Password: hashPassword(t, "str0ngpass"), IsPublic: true,
	}
	repo.seed(t, user)

	res, err := svc.DeleteUser(context.Background(), user.ID.String())
	if err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if res.Message == "" {
		t.Fatal("expected a confirmation message")
	}
	if repo.deleted != user.ID {
		t.Fatalf("deleted = %s, want %s", repo.deleted, user.ID)
	}
}

func TestDeleteUserRejectsMalformedID(t *testing.T) {
	svc, _ := newService(t)

	if _, err := svc.DeleteUser(context.Background(), "not-a-uuid"); err == nil {
		t.Fatal("expected a malformed id to be rejected")
	}
}
