package graph

import (
	"context"
	"testing"

	"github.com/chetansachan/socialflow/api-gateway/graph/dataloader"
	"github.com/chetansachan/socialflow/api-gateway/graph/model"
	socialflowpb "github.com/chetansachan/socialflow/proto"
	"google.golang.org/grpc"
)

type fakeCommentClient struct {
	socialflowpb.CommentServiceClient

	createRequest  *socialflowpb.CreateCommentRequest
	updateRequest  *socialflowpb.UpdateCommentRequest
	deleteRequest  *socialflowpb.DeleteCommentRequest
	likeRequest    *socialflowpb.LikeCommentRequest
	listRequest    *socialflowpb.ListCommentsRequest
	getRequest     *socialflowpb.GetCommentRequest
	likeStateReq   *socialflowpb.GetCommentLikeStatesRequest
	comment        *socialflowpb.Comment
	comments       []*socialflowpb.Comment
	nextCursor     string
	hasMore        bool
	isLiked        bool
	likeStates     map[string]bool
	message        string
	updateResponse *socialflowpb.Comment
}

func (c *fakeCommentClient) CreateComment(ctx context.Context, req *socialflowpb.CreateCommentRequest, _ ...grpc.CallOption) (*socialflowpb.CreateCommentResponse, error) {
	c.createRequest = req
	return &socialflowpb.CreateCommentResponse{Comment: c.comment}, nil
}

func (c *fakeCommentClient) UpdateComment(ctx context.Context, req *socialflowpb.UpdateCommentRequest, _ ...grpc.CallOption) (*socialflowpb.UpdateCommentResponse, error) {
	c.updateRequest = req
	return &socialflowpb.UpdateCommentResponse{Comment: c.updateResponse}, nil
}

func (c *fakeCommentClient) DeleteComment(ctx context.Context, req *socialflowpb.DeleteCommentRequest, _ ...grpc.CallOption) (*socialflowpb.DeleteCommentResponse, error) {
	c.deleteRequest = req
	return &socialflowpb.DeleteCommentResponse{Message: "comment deleted successfully"}, nil
}

func (c *fakeCommentClient) LikeComment(ctx context.Context, req *socialflowpb.LikeCommentRequest, _ ...grpc.CallOption) (*socialflowpb.LikeCommentResponse, error) {
	c.likeRequest = req
	return &socialflowpb.LikeCommentResponse{IsLiked: c.isLiked, Message: c.message}, nil
}

func (c *fakeCommentClient) GetComment(ctx context.Context, req *socialflowpb.GetCommentRequest, _ ...grpc.CallOption) (*socialflowpb.GetCommentResponse, error) {
	c.getRequest = req
	return &socialflowpb.GetCommentResponse{Comment: c.comment}, nil
}

func (c *fakeCommentClient) ListComments(ctx context.Context, req *socialflowpb.ListCommentsRequest, _ ...grpc.CallOption) (*socialflowpb.ListCommentsResponse, error) {
	c.listRequest = req
	return &socialflowpb.ListCommentsResponse{
		Comments:   c.comments,
		NextCursor: c.nextCursor,
		HasMore:    c.hasMore,
	}, nil
}

func (c *fakeCommentClient) GetCommentLikeStates(ctx context.Context, req *socialflowpb.GetCommentLikeStatesRequest, _ ...grpc.CallOption) (*socialflowpb.GetCommentLikeStatesResponse, error) {
	c.likeStateReq = req
	return &socialflowpb.GetCommentLikeStatesResponse{Liked: c.likeStates}, nil
}

func TestCreateCommentScopesAuthorToPrincipal(t *testing.T) {
	client := &fakeCommentClient{comment: &socialflowpb.Comment{
		Id:       "c1",
		PostId:   "p1",
		AuthorId: "u1",
		Content:  "hi",
	}}
	resolver := &mutationResolver{&Resolver{CommentClient: client}}

	parentID := "c0"
	res, err := resolver.CreateComment(viewerContext("u1"), model.CreateCommentInput{
		PostID:   "p1",
		ParentID: &parentID,
		Content:  "hi",
	})
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}

	if client.createRequest.GetAuthorId() != "u1" {
		t.Fatalf("author = %q, want the authenticated caller", client.createRequest.GetAuthorId())
	}
	if client.createRequest.GetParentId() != "c0" {
		t.Fatalf("parent = %q, want c0", client.createRequest.GetParentId())
	}
	if res.ID != "c1" || res.AuthorID != "u1" {
		t.Fatalf("comment = %+v", res)
	}
}

func TestCommentMutationsRequireAuthentication(t *testing.T) {
	resolver := &mutationResolver{&Resolver{CommentClient: &fakeCommentClient{}}}

	if _, err := resolver.CreateComment(context.Background(), model.CreateCommentInput{PostID: "p1", Content: "hi"}); err == nil {
		t.Fatal("anonymous create should fail")
	}
	if _, err := resolver.UpdateComment(context.Background(), model.UpdateCommentInput{ID: "c1", Content: "hi"}); err == nil {
		t.Fatal("anonymous update should fail")
	}
	if _, err := resolver.DeleteComment(context.Background(), "c1"); err == nil {
		t.Fatal("anonymous delete should fail")
	}
	if _, err := resolver.LikeComment(context.Background(), model.LikeCommentInput{CommentID: "c1"}); err == nil {
		t.Fatal("anonymous like should fail")
	}
}

func TestUpdateAndDeleteCommentPassPrincipalAuthor(t *testing.T) {
	client := &fakeCommentClient{updateResponse: &socialflowpb.Comment{
		Id:       "c1",
		AuthorId: "u1",
		Content:  "edited",
	}}
	resolver := &mutationResolver{&Resolver{CommentClient: client}}

	if _, err := resolver.UpdateComment(viewerContext("u1"), model.UpdateCommentInput{ID: "c1", Content: "edited"}); err != nil {
		t.Fatalf("UpdateComment: %v", err)
	}
	if client.updateRequest.GetAuthorId() != "u1" || client.updateRequest.GetCommentId() != "c1" {
		t.Fatalf("update request = %+v", client.updateRequest)
	}

	if _, err := resolver.DeleteComment(viewerContext("u1"), "c1"); err != nil {
		t.Fatalf("DeleteComment: %v", err)
	}
	if client.deleteRequest.GetAuthorId() != "u1" || client.deleteRequest.GetCommentId() != "c1" {
		t.Fatalf("delete request = %+v", client.deleteRequest)
	}
}

func TestLikeCommentScopesUserToPrincipal(t *testing.T) {
	client := &fakeCommentClient{isLiked: true, message: "comment liked successfully"}
	resolver := &mutationResolver{&Resolver{CommentClient: client}}

	res, err := resolver.LikeComment(viewerContext("u1"), model.LikeCommentInput{CommentID: "c1"})
	if err != nil {
		t.Fatalf("LikeComment: %v", err)
	}

	if client.likeRequest.GetUserId() != "u1" || client.likeRequest.GetCommentId() != "c1" {
		t.Fatalf("like request = %+v", client.likeRequest)
	}
	if !res.IsLiked || res.Message != "comment liked successfully" {
		t.Fatalf("response = %+v", res)
	}
}

func TestGetCommentsClampsPageSizeAndMapsPage(t *testing.T) {
	parentID := "c0"
	client := &fakeCommentClient{
		comments: []*socialflowpb.Comment{
			{Id: "c1", PostId: "p1", AuthorId: "u1", Content: "one", ParentId: parentID},
			{Id: "c2", PostId: "p1", AuthorId: "u2", Content: "two"},
		},
		nextCursor: "cursor-2",
		hasMore:    true,
	}
	resolver := &queryResolver{&Resolver{CommentClient: client}}

	limit := int32(500)
	res, err := resolver.GetComments(context.Background(), "p1", &parentID, &limit, nil)
	if err != nil {
		t.Fatalf("GetComments: %v", err)
	}

	if client.listRequest.GetLimit() != maxCommentPageSize {
		t.Fatalf("limit = %d, want %d", client.listRequest.GetLimit(), maxCommentPageSize)
	}
	if client.listRequest.GetParentId() != parentID {
		t.Fatalf("parent = %q, want %q", client.listRequest.GetParentId(), parentID)
	}
	if len(res.Comments) != 2 {
		t.Fatalf("comments = %d, want 2", len(res.Comments))
	}
	if res.Comments[0].ParentID == nil || *res.Comments[0].ParentID != parentID {
		t.Fatalf("first parent = %v", res.Comments[0].ParentID)
	}
	if res.Comments[1].ParentID != nil {
		t.Fatalf("second parent = %v, want nil", res.Comments[1].ParentID)
	}
	if res.NextCursor == nil || *res.NextCursor != "cursor-2" || !res.HasMore {
		t.Fatalf("page metadata = %+v", res)
	}
}

func TestGetCommentsDefaultsPageSize(t *testing.T) {
	client := &fakeCommentClient{}
	resolver := &queryResolver{&Resolver{CommentClient: client}}

	if _, err := resolver.GetComments(context.Background(), "p1", nil, nil, nil); err != nil {
		t.Fatalf("GetComments: %v", err)
	}
	if client.listRequest.GetLimit() != defaultCommentPageSize {
		t.Fatalf("limit = %d, want %d", client.listRequest.GetLimit(), defaultCommentPageSize)
	}
	if client.listRequest.GetParentId() != "" {
		t.Fatalf("parent = %q, want empty", client.listRequest.GetParentId())
	}
}

func TestGetCommentMapsFields(t *testing.T) {
	client := &fakeCommentClient{comment: &socialflowpb.Comment{
		Id:         "c1",
		PostId:     "p1",
		AuthorId:   "u1",
		ParentId:   "c0",
		Content:    "hello",
		LikeCount:  2,
		ReplyCount: 3,
		Depth:      1,
		IsDeleted:  true,
		CreatedAt:  "2026-01-01T00:00:00Z",
		UpdatedAt:  "2026-01-01T00:01:00Z",
	}}
	resolver := &queryResolver{&Resolver{CommentClient: client}}

	res, err := resolver.GetComment(context.Background(), "c1")
	if err != nil {
		t.Fatalf("GetComment: %v", err)
	}
	if client.getRequest.GetId() != "c1" {
		t.Fatalf("requested id = %q", client.getRequest.GetId())
	}
	if res.Content != "hello" || res.LikeCount != 2 || res.ReplyCount != 3 || res.Depth != 1 || !res.IsDeleted {
		t.Fatalf("comment = %+v", res)
	}
	if res.ParentID == nil || *res.ParentID != "c0" {
		t.Fatalf("parent = %v", res.ParentID)
	}
}

func TestCommentHasLikedIsAnonymousFriendly(t *testing.T) {
	r := &commentResolver{&Resolver{}}

	liked, err := r.HasLiked(context.Background(), &model.Comment{ID: "c1"})
	if err != nil {
		t.Fatalf("anonymous HasLiked: %v", err)
	}
	if liked {
		t.Fatal("anonymous viewer should not be marked as having liked")
	}
}

func TestCommentHasLikedUsesLoader(t *testing.T) {
	client := &fakeCommentClient{likeStates: map[string]bool{"c1": true}}
	loaders := dataloader.New(nil, nil, client)

	ctx := dataloader.WithLoaders(viewerContext("u1"), loaders)
	r := &commentResolver{&Resolver{}}

	liked, err := r.HasLiked(ctx, &model.Comment{ID: "c1"})
	if err != nil {
		t.Fatalf("HasLiked: %v", err)
	}
	if !liked {
		t.Fatal("expected the loader to report the comment as liked")
	}
	if client.likeStateReq.GetUserId() != "u1" || len(client.likeStateReq.GetCommentIds()) != 1 {
		t.Fatalf("like state request = %+v", client.likeStateReq)
	}
}

type batchUserClient struct {
	socialflowpb.UserServiceClient

	requested []string
	users     map[string]*socialflowpb.User
}

func (c *batchUserClient) BatchGetUsers(ctx context.Context, req *socialflowpb.BatchGetUsersRequest, _ ...grpc.CallOption) (*socialflowpb.BatchGetUsersResponse, error) {
	c.requested = append(c.requested, req.GetIds()...)
	users := make([]*socialflowpb.User, 0, len(req.GetIds()))
	for _, id := range req.GetIds() {
		if user, ok := c.users[id]; ok {
			users = append(users, user)
		}
	}
	return &socialflowpb.BatchGetUsersResponse{Users: users}, nil
}

func TestCommentAuthorUsesUserLoader(t *testing.T) {
	users := &batchUserClient{users: map[string]*socialflowpb.User{
		"u1": {Id: "u1", Username: "alice", Name: "Alice"},
	}}
	loaders := dataloader.New(users, nil, nil)
	ctx := dataloader.WithLoaders(context.Background(), loaders)
	r := &commentResolver{&Resolver{}}

	author, err := r.Author(ctx, &model.Comment{AuthorID: "u1"})
	if err != nil {
		t.Fatalf("Author: %v", err)
	}
	if author.ID != "u1" || author.Username != "alice" {
		t.Fatalf("author = %+v", author)
	}
	if len(users.requested) != 1 || users.requested[0] != "u1" {
		t.Fatalf("requested users = %v", users.requested)
	}
}
