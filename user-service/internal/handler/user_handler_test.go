package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	userpb "github.com/chetansachan/socialflow/proto"
	"github.com/chetansachan/socialflow/user-service/internal/models"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeUserService struct {
	user    *models.User
	users   []models.User
	session *models.LoginResponse
	rotated *models.RotationResponse
	err     error

	lastRegister models.CreateUserInput
	lastLogin    models.LoginRequest
	lastUpdate   models.UpdateUserInput
	lastUpdateID string
	lastDeleteID string
	lastQuery    string
	lastLimit    int32
	lastByEmail  string
	lastRefresh  string
}

func (s *fakeUserService) Register(ctx context.Context, input models.CreateUserInput) (*models.User, error) {
	s.lastRegister = input
	if s.err != nil {
		return nil, s.err
	}
	result := models.User{
		ID:        uuid.New(),
		Username:  input.Username,
		Name:      input.Name,
		Email:     input.Email,
		IsPublic:  true,
		CreatedAt: time.Now().UTC(),
	}
	return &result, nil
}

func (s *fakeUserService) Login(ctx context.Context, input models.LoginRequest) (*models.LoginResponse, error) {
	s.lastLogin = input
	return s.session, s.err
}

func (s *fakeUserService) GetById(ctx context.Context, id string) (*models.User, error) {
	if s.err != nil {
		return nil, s.err
	}
	user := models.User{ID: uuid.New(), Username: "alice", Name: "Alice", Email: "alice@example.com", IsPublic: true}
	return &user, nil
}

func (s *fakeUserService) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	s.lastByEmail = email
	if s.err != nil {
		return nil, s.err
	}
	return &models.User{ID: uuid.New(), Username: "found", Email: email}, nil
}

func (s *fakeUserService) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	return s.GetById(ctx, username)
}

func (s *fakeUserService) GetByIds(ctx context.Context, ids []string) ([]models.User, error) {
	return s.users, s.err
}

func (s *fakeUserService) Refresh(ctx context.Context, tkn string) (*models.RotationResponse, error) {
	s.lastRefresh = tkn
	return s.rotated, s.err
}

func (s *fakeUserService) UpdateUser(ctx context.Context, input models.UpdateUserInput, id string) (*models.User, error) {
	s.lastUpdate = input
	s.lastUpdateID = id
	if s.err != nil {
		return nil, s.err
	}
	return &models.User{ID: uuid.New(), Username: "updated", Email: "updated@example.com"}, nil
}

func (s *fakeUserService) DeleteUser(ctx context.Context, id string) (*models.DeleteUserRespose, error) {
	s.lastDeleteID = id
	if s.err != nil {
		return nil, s.err
	}
	return &models.DeleteUserRespose{Message: "User Deleted Successfully"}, nil
}

func (s *fakeUserService) SearchUsers(ctx context.Context, query string, limit int32) ([]models.User, error) {
	s.lastQuery = query
	s.lastLimit = limit
	return s.users, s.err
}

func sampleUser() *models.User {
	updated := time.Now().UTC().Add(-time.Hour).Round(0)
	return &models.User{
		ID:        uuid.New(),
		Username:  "alice",
		Name:      "Alice",
		Email:     "alice@example.com",
		IsPublic:  true,
		CreatedAt: time.Now().UTC().Add(-time.Hour).Round(0),
		UpdatedAt: &updated,
	}
}

func TestRegisterMapsUserAndForwardsEveryField(t *testing.T) {
	service := &fakeUserService{}
	h := NewUserHandler(service)

	isPublic := false
	res, err := h.Register(context.Background(), &userpb.RegisterUserRequest{
		Username: "alice",
		Name:     "Alice",
		Email:    "alice@example.com",
		Password: "Passw0rd!",
		IsPublic: &isPublic,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	// The handler must not trim or default anything: the service layer owns
	// validation, and a second opinion here would let the two disagree.
	if service.lastRegister.Username != "alice" {
		t.Fatalf("username = %q, want the input forwarded untouched", service.lastRegister.Username)
	}
	if service.lastRegister.IsPublic == nil || *service.lastRegister.IsPublic {
		t.Fatal("IsPublic must pass through as-is so a nil default is not invented")
	}
	if res.GetUser().GetUsername() != "alice" || res.GetUser().GetEmail() != "alice@example.com" {
		t.Fatalf("user = %+v", res.GetUser())
	}
	if res.GetUser().GetCreatedAt() == "" {
		t.Fatal("created_at should be formatted for the client")
	}
}

func TestRegisterMapsDuplicateAndInvalidInput(t *testing.T) {
	cases := []struct {
		err  error
		want codes.Code
	}{
		{models.ErrUserAlreadyExists, codes.AlreadyExists},
		{models.ErrInvalidInput, codes.InvalidArgument},
		{errors.New("connection reset"), codes.Internal},
	}
	for _, tc := range cases {
		h := NewUserHandler(&fakeUserService{err: tc.err})
		_, err := h.Register(context.Background(), &userpb.RegisterUserRequest{Username: "a", Email: "a@b.c", Password: "Passw0rd!"})
		if status.Code(err) != tc.want {
			t.Fatalf("%v: code = %v, want %v", tc.err, status.Code(err), tc.want)
		}
	}
}

func TestLoginMapsTokensAndBadCredentials(t *testing.T) {
	service := &fakeUserService{session: &models.LoginResponse{AccessTkn: "a", RefreshTkn: "r", Message: "ok"}}
	h := NewUserHandler(service)

	res, err := h.Login(context.Background(), &userpb.LoginRequest{Email: "alice@example.com", Password: "Passw0rd!"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if service.lastLogin.Email != "alice@example.com" {
		t.Fatalf("login request = %+v", service.lastLogin)
	}
	if res.GetAccessTkn() != "a" || res.GetRefreshTkn() != "r" || res.GetMessage() != "ok" {
		t.Fatalf("session = %+v", res)
	}

	// A wrong password and an unknown account must be indistinguishable, so an
	// attacker cannot enumerate emails. Anything unexpected is a 500 rather than
	// a NotFound for the same reason.
	wrongPassword := &fakeUserService{err: models.ErrInvalidCredentials}
	if _, got := NewUserHandler(wrongPassword).Login(context.Background(), &userpb.LoginRequest{Email: "nope@example.com", Password: "x"}); status.Code(got) != codes.Unauthenticated {
		t.Fatalf("wrong password: code = %v, want Unauthenticated", status.Code(got))
	}
	unknownAccount := &fakeUserService{err: models.ErrUserNotFound}
	if _, got := NewUserHandler(unknownAccount).Login(context.Background(), &userpb.LoginRequest{Email: "nope@example.com", Password: "x"}); status.Code(got) == codes.NotFound {
		t.Fatal("a NotFound on login would reveal that the account does not exist")
	}
}

func TestSearchUsersMapsResultsAndValidation(t *testing.T) {
	service := &fakeUserService{users: []models.User{*sampleUser()}}
	h := NewUserHandler(service)

	res, err := h.SearchUsers(context.Background(), &userpb.SearchUsersRequest{Query: "  ali ", Limit: 25})
	if err != nil {
		t.Fatalf("SearchUsers: %v", err)
	}
	if service.lastQuery != "  ali " {
		t.Fatalf("query = %q, want the input forwarded untouched", service.lastQuery)
	}
	if service.lastLimit != 25 {
		t.Fatalf("limit = %d", service.lastLimit)
	}
	if len(res.GetUsers()) != 1 || res.GetUsers()[0].GetUsername() != "alice" {
		t.Fatalf("users = %+v", res.GetUsers())
	}

	h = NewUserHandler(&fakeUserService{err: models.ErrInvalidInput})
	if _, err := h.SearchUsers(context.Background(), &userpb.SearchUsersRequest{Query: ""}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("blank query: code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestUpdateUserMapsEveryDomainError(t *testing.T) {
	cases := []struct {
		err  error
		want codes.Code
	}{
		{models.ErrInvalidInput, codes.InvalidArgument},
		{models.ErrUserNotFound, codes.NotFound},
		{models.ErrUserAlreadyExists, codes.AlreadyExists},
		{errors.New("connection reset"), codes.Internal},
	}
	for _, tc := range cases {
		h := NewUserHandler(&fakeUserService{err: tc.err})
		_, err := h.UpdateUser(context.Background(), &userpb.UpdateUserRequest{Id: uuid.New().String()})
		if status.Code(err) != tc.want {
			t.Fatalf("%v: code = %v, want %v", tc.err, status.Code(err), tc.want)
		}
	}
}

func TestBatchGetUsersMapsListAndKeepsOrder(t *testing.T) {
	first, second := sampleUser(), sampleUser()
	service := &fakeUserService{users: []models.User{*first, *second}}
	h := NewUserHandler(service)

	res, err := h.BatchGetUsers(context.Background(), &userpb.BatchGetUsersRequest{Ids: []string{first.ID.String(), second.ID.String()}})
	if err != nil {
		t.Fatalf("BatchGetUsers: %v", err)
	}
	if len(res.GetUsers()) != 2 {
		t.Fatalf("users = %d, want 2", len(res.GetUsers()))
	}
	if res.GetUsers()[0].GetId() != first.ID.String() {
		t.Fatal("batch order must be preserved: dataloader keys line up positionally")
	}
	if res.GetUsers()[1].GetId() != second.ID.String() {
		t.Fatalf("second user = %s", res.GetUsers()[1].GetId())
	}
}

func TestBatchGetUsersReturnsEmptyListForNoMatches(t *testing.T) {
	h := NewUserHandler(&fakeUserService{})

	res, err := h.BatchGetUsers(context.Background(), &userpb.BatchGetUsersRequest{Ids: []string{uuid.New().String()}})
	if err != nil {
		t.Fatalf("BatchGetUsers: %v", err)
	}
	if res.GetUsers() == nil {
		t.Fatal("an empty batch must serialise as [], not null")
	}
}

func TestGetUserByEmailMapsNotFound(t *testing.T) {
	h := NewUserHandler(&fakeUserService{err: models.ErrUserNotFound})

	if _, err := h.GetUserByEmail(context.Background(), &userpb.GetUserByEmailRequest{Email: "nope@example.com"}); status.Code(err) != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", status.Code(err))
	}

	h = NewUserHandler(&fakeUserService{})
	res, err := h.GetUserByEmail(context.Background(), &userpb.GetUserByEmailRequest{Email: "alice@example.com"})
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if res.GetUser().GetEmail() != "alice@example.com" {
		t.Fatalf("user = %+v", res.GetUser())
	}
}

func TestRefreshAndDeleteMapThrough(t *testing.T) {
	service := &fakeUserService{rotated: &models.RotationResponse{AccessTkn: "a", RefreshTkn: "r"}}
	h := NewUserHandler(service)

	res, err := h.Refresh(context.Background(), &userpb.RefreshTknRequest{Refreshtkn: "old"})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if service.lastRefresh != "old" || res.GetNewAccessTkn() != "a" {
		t.Fatalf("rotated = %+v", res)
	}

	deleted, err := h.DeleteUser(context.Background(), &userpb.DeleteUserRequest{Id: "u1"})
	if err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if service.lastDeleteID != "u1" || deleted.GetMessage() == "" {
		t.Fatalf("deleted %q, message %q", service.lastDeleteID, deleted.GetMessage())
	}
}

func TestToUserPBHandlesNilAndOptionalUpdatedAt(t *testing.T) {
	if toUserPB(nil) != nil {
		t.Fatal("nil should map to nil")
	}

	user := sampleUser()
	mapped := toUserPB(user)
	if mapped.GetUpdatedAt() == "" {
		t.Fatal("a set updated_at should be formatted")
	}

	user.UpdatedAt = nil
	if got := toUserPB(user).GetUpdatedAt(); got != "" {
		t.Fatalf("nil updated_at mapped to %q, want empty string", got)
	}
}
