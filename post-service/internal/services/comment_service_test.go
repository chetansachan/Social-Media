package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/chetansachan/socialflow/post-service/internal/models"
	"github.com/google/uuid"
)

type fakeCommentRepo struct {
	comments map[uuid.UUID]*models.Comment
	likes    map[likeKey]bool

	createErr error
	listErr   error
	updateErr error
	deleteErr error
	likeErr   error

	lastCreate  *models.CreateCommentInput
	lastPostID  uuid.UUID
	lastParent  *uuid.UUID
	lastLimit   int
	lastCursor  models.CommentCursor
	lastDeleted uuid.UUID
	likeStates  []uuid.UUID
}

func newFakeCommentRepo() *fakeCommentRepo {
	return &fakeCommentRepo{
		comments: make(map[uuid.UUID]*models.Comment),
		likes:    make(map[likeKey]bool),
	}
}

func (r *fakeCommentRepo) CreateComment(ctx context.Context, input *models.CreateCommentInput) (*models.Comment, error) {
	if r.createErr != nil {
		return nil, r.createErr
	}
	r.lastCreate = input

	comment := &models.Comment{
		ID:        uuid.New(),
		PostID:    input.PostID,
		AuthorID:  input.AuthorID,
		ParentID:  input.ParentID,
		Content:   input.Content,
		Depth:     0,
		CreatedAt: time.Now().UTC(),
	}
	r.comments[comment.ID] = comment
	return comment, nil
}

func (r *fakeCommentRepo) GetComment(ctx context.Context, id uuid.UUID) (*models.Comment, error) {
	comment, ok := r.comments[id]
	if !ok {
		return nil, models.ErrCommentNotFound
	}
	copy := *comment
	return &copy, nil
}

func (r *fakeCommentRepo) ListComments(
	ctx context.Context,
	postID uuid.UUID,
	parentID *uuid.UUID,
	cursor models.CommentCursor,
	limit int,
) ([]models.Comment, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	r.lastPostID = postID
	r.lastParent = parentID
	r.lastLimit = limit
	r.lastCursor = cursor

	// Return limit+1 rows so the service can prove has_more without a COUNT.
	comments := make([]models.Comment, 0, limit+1)
	base := time.Now().UTC()
	for i := 0; i <= limit; i++ {
		comments = append(comments, models.Comment{
			ID:        uuid.New(),
			PostID:    postID,
			Content:   "comment",
			CreatedAt: base.Add(time.Duration(-i) * time.Second),
		})
	}
	return comments, nil
}

func (r *fakeCommentRepo) UpdateComment(ctx context.Context, commentID, authorID uuid.UUID, content string) (*models.Comment, error) {
	if r.updateErr != nil {
		return nil, r.updateErr
	}
	comment, ok := r.comments[commentID]
	if !ok {
		return nil, models.ErrCommentNotFound
	}
	copy := *comment
	copy.Content = content
	return &copy, nil
}

func (r *fakeCommentRepo) DeleteComment(ctx context.Context, commentID, authorID uuid.UUID) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	r.lastDeleted = commentID
	return nil
}

func (r *fakeCommentRepo) LikeComment(ctx context.Context, commentID, userID uuid.UUID) (bool, error) {
	if r.likeErr != nil {
		return false, r.likeErr
	}
	key := likeKey{postID: commentID, userID: userID}
	r.likes[key] = !r.likes[key]
	return r.likes[key], nil
}

func (r *fakeCommentRepo) GetCommentLikeStates(ctx context.Context, userID uuid.UUID, commentIDs []uuid.UUID) (map[string]bool, error) {
	r.likeStates = commentIDs
	states := make(map[string]bool, len(commentIDs))
	for _, id := range commentIDs {
		if r.likes[likeKey{postID: id, userID: userID}] {
			states[id.String()] = true
		}
	}
	return states, nil
}

func TestCreateCommentValidatesAndTrimsInput(t *testing.T) {
	repo := newFakeCommentRepo()
	service := NewCommentService(repo)

	postID, authorID := uuid.New(), uuid.New()
	comment, err := service.CreateComment(context.Background(), models.CreateCommentInput{
		PostID:   postID,
		AuthorID: authorID,
		Content:  "  hello world  ",
	})
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}
	if comment.Content != "hello world" {
		t.Fatalf("content = %q, want trimmed value", comment.Content)
	}
	if repo.lastCreate == nil || repo.lastCreate.Content != "hello world" {
		t.Fatalf("repository received %+v, want trimmed content", repo.lastCreate)
	}
}

func TestCreateCommentRejectsInvalidInput(t *testing.T) {
	service := NewCommentService(newFakeCommentRepo())
	postID, authorID := uuid.New(), uuid.New()

	cases := map[string]models.CreateCommentInput{
		"missing post":    {AuthorID: authorID, Content: "hi"},
		"missing author":  {PostID: postID, Content: "hi"},
		"empty":           {PostID: postID, AuthorID: authorID, Content: "   "},
		"too long":        {PostID: postID, AuthorID: authorID, Content: strings.Repeat("a", maxCommentLength+1)},
		"too long runes":  {PostID: postID, AuthorID: authorID, Content: strings.Repeat("é", maxCommentLength+1)},
		"exactly max len": {PostID: postID, AuthorID: authorID, Content: strings.Repeat("a", maxCommentLength)},
	}

	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := service.CreateComment(context.Background(), input)
			if name == "exactly max len" {
				if err != nil {
					t.Fatalf("expected max length to be accepted, got %v", err)
				}
				return
			}
			if !errors.Is(err, models.ErrInvalidInput) {
				t.Fatalf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestCreateCommentNormalisesNilParent(t *testing.T) {
	repo := newFakeCommentRepo()
	service := NewCommentService(repo)

	nilParent := uuid.Nil
	if _, err := service.CreateComment(context.Background(), models.CreateCommentInput{
		PostID:   uuid.New(),
		AuthorID: uuid.New(),
		ParentID: &nilParent,
		Content:  "top level",
	}); err != nil {
		t.Fatalf("create comment: %v", err)
	}
	if repo.lastCreate.ParentID != nil {
		t.Fatalf("parent = %v, want nil for a zero uuid", repo.lastCreate.ParentID)
	}
}

func TestCreateCommentPreservesDomainErrors(t *testing.T) {
	repo := newFakeCommentRepo()
	service := NewCommentService(repo)
	input := models.CreateCommentInput{PostID: uuid.New(), AuthorID: uuid.New(), Content: "hi"}

	for _, want := range []error{
		models.ErrPostNotFound,
		models.ErrCommentNotFound,
		models.ErrCommentDepthExceeded,
		models.ErrInvalidInput,
	} {
		repo.createErr = want
		if _, err := service.CreateComment(context.Background(), input); !errors.Is(err, want) {
			t.Fatalf("err = %v, want %v", err, want)
		}
	}

	repo.createErr = errors.New("connection reset")
	if _, err := service.CreateComment(context.Background(), input); errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("infrastructure error was misclassified as invalid input: %v", err)
	}
}

func TestListCommentsDefaultsAndClampsPageSize(t *testing.T) {
	repo := newFakeCommentRepo()
	service := NewCommentService(repo)
	postID := uuid.New()

	for _, tc := range []struct {
		name      string
		requested int32
		want      int
	}{
		{name: "default", requested: 0, want: defaultPageSize},
		{name: "negative", requested: -5, want: defaultPageSize},
		{name: "explicit", requested: 5, want: 5},
		{name: "clamped", requested: 1000, want: maxPageSize},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, err := service.ListComments(context.Background(), postID.String(), "", tc.requested, "")
			if err != nil {
				t.Fatalf("list comments: %v", err)
			}
			if repo.lastLimit != tc.want {
				t.Fatalf("limit = %d, want %d", repo.lastLimit, tc.want)
			}
			if len(page.Comments) != tc.want {
				t.Fatalf("comments = %d, want %d", len(page.Comments), tc.want)
			}
			if !page.HasMore || page.NextCursor == "" {
				t.Fatalf("page should report more results: %+v", page)
			}
		})
	}
}

func TestListCommentsRejectsBadCursorAndIDs(t *testing.T) {
	service := NewCommentService(newFakeCommentRepo())

	if _, err := service.ListComments(context.Background(), "nope", "", 0, ""); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("bad post id: err = %v, want ErrInvalidInput", err)
	}
	if _, err := service.ListComments(context.Background(), uuid.New().String(), "nope", 0, ""); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("bad parent id: err = %v, want ErrInvalidInput", err)
	}
	if _, err := service.ListComments(context.Background(), uuid.New().String(), "", 0, "not-a-cursor"); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("bad cursor: err = %v, want ErrInvalidInput", err)
	}
}

func TestListCommentsPassesDecodedCursor(t *testing.T) {
	repo := newFakeCommentRepo()
	service := NewCommentService(repo)

	createdAt := time.Now().UTC().Round(0)
	id := uuid.New()
	cursor := models.EncodeCommentCursor(createdAt, id)

	if _, err := service.ListComments(context.Background(), uuid.New().String(), "", 10, cursor); err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if !repo.lastCursor.CreatedAt.Equal(createdAt) || repo.lastCursor.ID != id {
		t.Fatalf("cursor = %+v, want %s/%s", repo.lastCursor, createdAt, id)
	}
}

func TestUpdateCommentValidation(t *testing.T) {
	service := NewCommentService(newFakeCommentRepo())
	commentID, authorID := uuid.New(), uuid.New()

	cases := map[string]struct {
		commentID string
		authorID  string
		content   string
		want      error
	}{
		"bad comment id": {commentID: "x", authorID: authorID.String(), content: "hi", want: models.ErrInvalidInput},
		"bad author id":  {commentID: commentID.String(), authorID: "x", content: "hi", want: models.ErrInvalidInput},
		"empty content":  {commentID: commentID.String(), authorID: authorID.String(), content: " ", want: models.ErrInvalidInput},
		"long content":   {commentID: commentID.String(), authorID: authorID.String(), content: strings.Repeat("a", maxCommentLength+1), want: models.ErrInvalidInput},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := service.UpdateComment(context.Background(), tc.commentID, tc.authorID, tc.content); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestUpdateAndDeletePreserveOwnershipErrors(t *testing.T) {
	repo := newFakeCommentRepo()
	service := NewCommentService(repo)
	commentID, authorID := uuid.New(), uuid.New()

	repo.updateErr = models.ErrCommentForbidden
	if _, err := service.UpdateComment(context.Background(), commentID.String(), authorID.String(), "hi"); !errors.Is(err, models.ErrCommentForbidden) {
		t.Fatalf("update err = %v, want ErrCommentForbidden", err)
	}

	repo.deleteErr = models.ErrCommentForbidden
	if err := service.DeleteComment(context.Background(), commentID.String(), authorID.String()); !errors.Is(err, models.ErrCommentForbidden) {
		t.Fatalf("delete err = %v, want ErrCommentForbidden", err)
	}

	repo.deleteErr = nil
	if err := service.DeleteComment(context.Background(), commentID.String(), authorID.String()); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if repo.lastDeleted != commentID {
		t.Fatalf("deleted = %s, want %s", repo.lastDeleted, commentID)
	}
}

func TestLikeCommentWrapsDomainErrors(t *testing.T) {
	repo := newFakeCommentRepo()
	service := NewCommentService(repo)

	if _, err := service.LikeComment(context.Background(), "x", uuid.New().String()); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("bad comment id: err = %v, want ErrInvalidInput", err)
	}
	if _, err := service.LikeComment(context.Background(), uuid.New().String(), "x"); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("bad user id: err = %v, want ErrInvalidInput", err)
	}

	repo.likeErr = models.ErrCommentNotFound
	if _, err := service.LikeComment(context.Background(), uuid.New().String(), uuid.New().String()); !errors.Is(err, models.ErrCommentNotFound) {
		t.Fatalf("err = %v, want ErrCommentNotFound", err)
	}
}

func TestGetCommentLikeStatesFiltersInvalidAndDuplicateIDs(t *testing.T) {
	repo := newFakeCommentRepo()
	service := NewCommentService(repo)
	userID := uuid.New()
	first, second := uuid.New(), uuid.New()

	states, err := service.GetCommentLikeStates(context.Background(), userID.String(), []string{
		first.String(),
		first.String(),
		"not-a-uuid",
		second.String(),
	})
	if err != nil {
		t.Fatalf("like states: %v", err)
	}
	if len(repo.likeStates) != 2 {
		t.Fatalf("queried ids = %v, want two unique ids", repo.likeStates)
	}
	if len(states) != 0 {
		t.Fatalf("states = %v, want empty", states)
	}
}
