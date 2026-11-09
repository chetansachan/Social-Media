package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chetansachan/socialflow/post-service/internal/models"
	commentpb "github.com/chetansachan/socialflow/proto"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeCommentService struct {
	comment *models.Comment
	page    *models.CommentPage
	liked   map[string]bool
	isLiked bool

	err error

	lastCreate models.CreateCommentInput
	lastList   struct {
		postID   string
		parentID string
		limit    int32
		cursor   string
	}
}

func (s *fakeCommentService) CreateComment(ctx context.Context, input models.CreateCommentInput) (*models.Comment, error) {
	s.lastCreate = input
	return s.comment, s.err
}

func (s *fakeCommentService) GetComment(ctx context.Context, id string) (*models.Comment, error) {
	return s.comment, s.err
}

func (s *fakeCommentService) ListComments(ctx context.Context, postID, parentID string, limit int32, cursor string) (*models.CommentPage, error) {
	s.lastList.postID = postID
	s.lastList.parentID = parentID
	s.lastList.limit = limit
	s.lastList.cursor = cursor
	return s.page, s.err
}

func (s *fakeCommentService) UpdateComment(ctx context.Context, commentID, authorID, content string) (*models.Comment, error) {
	return s.comment, s.err
}

func (s *fakeCommentService) DeleteComment(ctx context.Context, commentID, authorID string) error {
	return s.err
}

func (s *fakeCommentService) LikeComment(ctx context.Context, commentID, userID string) (bool, error) {
	return s.isLiked, s.err
}

func (s *fakeCommentService) GetCommentLikeStates(ctx context.Context, userID string, commentIDs []string) (map[string]bool, error) {
	return s.liked, s.err
}

func sampleComment() *models.Comment {
	parentID := uuid.New()
	now := time.Now().UTC().Round(0)
	return &models.Comment{
		ID:         uuid.New(),
		PostID:     uuid.New(),
		AuthorID:   uuid.New(),
		ParentID:   &parentID,
		Content:    "hello",
		LikeCount:  3,
		ReplyCount: 1,
		Depth:      1,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

func TestToCommentPBMapsOptionalParent(t *testing.T) {
	comment := sampleComment()
	pb := toCommentPB(comment)

	if pb.ParentId != comment.ParentID.String() {
		t.Fatalf("parent = %q, want %q", pb.ParentId, comment.ParentID)
	}
	if pb.Content != comment.Content || pb.LikeCount != 3 || pb.ReplyCount != 1 || pb.Depth != 1 {
		t.Fatalf("mapped comment = %+v", pb)
	}
	if pb.CreatedAt != comment.CreatedAt.Format(time.RFC3339) {
		t.Fatalf("created_at = %q", pb.CreatedAt)
	}

	comment.ParentID = nil
	if got := toCommentPB(comment).GetParentId(); got != "" {
		t.Fatalf("nil parent mapped to %q, want empty", got)
	}
	if toCommentPB(nil) != nil {
		t.Fatal("nil comment should map to nil")
	}
}

func TestCommentHandlerCreateParsesIDs(t *testing.T) {
	service := &fakeCommentService{comment: sampleComment()}
	h := NewCommentHandler(service)

	if _, err := h.CreateComment(context.Background(), &commentpb.CreateCommentRequest{
		PostId:   "not-a-uuid",
		AuthorId: uuid.New().String(),
		Content:  "hi",
	}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("bad post id: code = %v, want InvalidArgument", status.Code(err))
	}

	if _, err := h.CreateComment(context.Background(), &commentpb.CreateCommentRequest{
		PostId:   uuid.New().String(),
		AuthorId: uuid.New().String(),
		ParentId: "not-a-uuid",
		Content:  "hi",
	}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("bad parent id: code = %v, want InvalidArgument", status.Code(err))
	}

	postID, authorID := uuid.New(), uuid.New()
	resp, err := h.CreateComment(context.Background(), &commentpb.CreateCommentRequest{
		PostId:   postID.String(),
		AuthorId: authorID.String(),
		Content:  "  hi  ",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if resp.GetComment() == nil {
		t.Fatal("response has no comment")
	}
	if service.lastCreate.PostID != postID || service.lastCreate.AuthorID != authorID {
		t.Fatalf("service received %+v", service.lastCreate)
	}
	if service.lastCreate.ParentID != nil {
		t.Fatalf("parent = %v, want nil", service.lastCreate.ParentID)
	}
}

func TestCommentHandlerListForwardsPagination(t *testing.T) {
	service := &fakeCommentService{page: &models.CommentPage{
		Comments:   []models.Comment{*sampleComment(), *sampleComment()},
		NextCursor: "next",
		HasMore:    true,
	}}
	h := NewCommentHandler(service)

	postID := uuid.New().String()
	resp, err := h.ListComments(context.Background(), &commentpb.ListCommentsRequest{
		PostId:   postID,
		ParentId: uuid.New().String(),
		Limit:    25,
		Cursor:   "opaque",
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(resp.GetComments()) != 2 || resp.GetNextCursor() != "next" || !resp.GetHasMore() {
		t.Fatalf("response = %+v", resp)
	}
	if service.lastList.postID != postID || service.lastList.limit != 25 || service.lastList.cursor != "opaque" {
		t.Fatalf("service received %+v", service.lastList)
	}
}

func TestCommentHandlerMapsDomainErrors(t *testing.T) {
	cases := []struct {
		err  error
		want codes.Code
	}{
		{models.ErrCommentNotFound, codes.NotFound},
		{models.ErrPostNotFound, codes.NotFound},
		{models.ErrCommentForbidden, codes.PermissionDenied},
		{models.ErrInvalidInput, codes.InvalidArgument},
		{models.ErrCommentDepthExceeded, codes.InvalidArgument},
		{errors.New("connection reset"), codes.Internal},
	}
	for _, tc := range cases {
		h := NewCommentHandler(&fakeCommentService{err: tc.err})

		_, err := h.GetComment(context.Background(), &commentpb.GetCommentRequest{Id: uuid.New().String()})
		if status.Code(err) != tc.want {
			t.Fatalf("%v: code = %v, want %v", tc.err, status.Code(err), tc.want)
		}
	}
}

func TestCommentHandlerLikeMessageTracksState(t *testing.T) {
	for _, liked := range []bool{true, false} {
		h := NewCommentHandler(&fakeCommentService{isLiked: liked})

		resp, err := h.LikeComment(context.Background(), &commentpb.LikeCommentRequest{
			CommentId: uuid.New().String(),
			UserId:    uuid.New().String(),
		})
		if err != nil {
			t.Fatalf("like: %v", err)
		}
		if resp.GetIsLiked() != liked {
			t.Fatalf("is_liked = %v, want %v", resp.GetIsLiked(), liked)
		}
		if liked && resp.GetMessage() != "comment liked successfully" {
			t.Fatalf("message = %q", resp.GetMessage())
		}
		if !liked && resp.GetMessage() != "comment unliked successfully" {
			t.Fatalf("message = %q", resp.GetMessage())
		}
	}
}

func TestCommentHandlerDeleteAndLikeStates(t *testing.T) {
	service := &fakeCommentService{liked: map[string]bool{"a": true}}
	h := NewCommentHandler(service)

	if _, err := h.DeleteComment(context.Background(), &commentpb.DeleteCommentRequest{
		CommentId: uuid.New().String(),
		AuthorId:  uuid.New().String(),
	}); err != nil {
		t.Fatalf("delete: %v", err)
	}

	resp, err := h.GetCommentLikeStates(context.Background(), &commentpb.GetCommentLikeStatesRequest{
		UserId:     uuid.New().String(),
		CommentIds: []string{"a", "b"},
	})
	if err != nil {
		t.Fatalf("like states: %v", err)
	}
	if !resp.GetLiked()["a"] || resp.GetLiked()["b"] {
		t.Fatalf("liked = %v", resp.GetLiked())
	}
}
