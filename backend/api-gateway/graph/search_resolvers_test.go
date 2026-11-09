package graph

import (
	"context"
	"testing"

	socialflowpb "github.com/chetansachan/socialflow/proto"
	"google.golang.org/grpc"
)

type fakeSearchClient struct {
	socialflowpb.PostServiceClient
	socialflowpb.UserServiceClient

	postRequest *socialflowpb.SearchPostsRequest
	userRequest *socialflowpb.SearchUsersRequest

	posts []*socialflowpb.Post
	users []*socialflowpb.User
}

func (c *fakeSearchClient) SearchPosts(ctx context.Context, req *socialflowpb.SearchPostsRequest, _ ...grpc.CallOption) (*socialflowpb.SearchPostsResponse, error) {
	c.postRequest = req
	return &socialflowpb.SearchPostsResponse{Posts: c.posts}, nil
}

func (c *fakeSearchClient) SearchUsers(ctx context.Context, req *socialflowpb.SearchUsersRequest, _ ...grpc.CallOption) (*socialflowpb.SearchUsersResponse, error) {
	c.userRequest = req
	return &socialflowpb.SearchUsersResponse{Users: c.users}, nil
}

func TestSearchPostsClampsAndMapsResults(t *testing.T) {
	client := &fakeSearchClient{posts: []*socialflowpb.Post{
		{Id: "p1", AuthorId: "u1", Content: "golang tips", LikeCount: 3},
		{Id: "p2", AuthorId: "u2", Content: "golang tricks"},
	}}
	resolver := &queryResolver{&Resolver{PostClient: client}}

	results, err := resolver.SearchPosts(context.Background(), "  golang  ", nil)
	if err != nil {
		t.Fatalf("SearchPosts: %v", err)
	}

	if client.postRequest.GetQuery() != "  golang  " {
		t.Fatalf("query = %q, want the raw input forwarded", client.postRequest.GetQuery())
	}
	if client.postRequest.GetLimit() != defaultSearchPageSize {
		t.Fatalf("limit = %d, want %d", client.postRequest.GetLimit(), defaultSearchPageSize)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	if results[0].ID != "p1" || results[0].LikeCount != 3 {
		t.Fatalf("first result = %+v", results[0])
	}

	limit := int32(1000)
	results, err = resolver.SearchPosts(context.Background(), "golang", &limit)
	if err != nil {
		t.Fatalf("SearchPosts with a large page: %v", err)
	}
	if client.postRequest.GetLimit() != maxSearchPageSize {
		t.Fatalf("limit = %d, want %d", client.postRequest.GetLimit(), maxSearchPageSize)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
}

func TestSearchUsersClampsAndMapsResults(t *testing.T) {
	client := &fakeSearchClient{users: []*socialflowpb.User{
		{Id: "u1", Username: "alice", Name: "Alice"},
	}}
	resolver := &queryResolver{&Resolver{UserClient: client}}

	results, err := resolver.SearchUsers(context.Background(), "al", nil)
	if err != nil {
		t.Fatalf("SearchUsers: %v", err)
	}

	if client.userRequest.GetQuery() != "al" {
		t.Fatalf("query = %q", client.userRequest.GetQuery())
	}
	if client.userRequest.GetLimit() != defaultSearchPageSize {
		t.Fatalf("limit = %d, want %d", client.userRequest.GetLimit(), defaultSearchPageSize)
	}
	if len(results) != 1 || results[0].Username != "alice" {
		t.Fatalf("results = %+v", results)
	}

	limit := int32(-1)
	if _, err := resolver.SearchUsers(context.Background(), "al", &limit); err != nil {
		t.Fatalf("SearchUsers with a non-positive page: %v", err)
	}
	if client.userRequest.GetLimit() != defaultSearchPageSize {
		t.Fatalf("limit = %d, want %d", client.userRequest.GetLimit(), defaultSearchPageSize)
	}
}

func TestSearchPostsReturnsEmptySliceNotNil(t *testing.T) {
	resolver := &queryResolver{&Resolver{PostClient: &fakeSearchClient{}, UserClient: &fakeSearchClient{}}}

	posts, err := resolver.SearchPosts(context.Background(), "nothing", nil)
	if err != nil {
		t.Fatalf("SearchPosts: %v", err)
	}
	if posts == nil {
		t.Fatal("an empty result should still serialise as [] for clients")
	}

	users, err := resolver.SearchUsers(context.Background(), "nobody", nil)
	if err != nil {
		t.Fatalf("SearchUsers: %v", err)
	}
	if users == nil {
		t.Fatal("an empty result should still serialise as [] for clients")
	}
}
