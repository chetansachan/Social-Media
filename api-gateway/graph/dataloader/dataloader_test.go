package dataloader

import (
	"context"
	"sync"
	"testing"

	socialflowpb "github.com/chetansachan/socialflow/proto"
	"google.golang.org/grpc"
)

// countingUserClient embeds the generated interface so it satisfies every
// method while only implementing the batch lookup the loader uses.
type countingUserClient struct {
	socialflowpb.UserServiceClient

	mu    sync.Mutex
	calls int
	ids   []string
	users map[string]*socialflowpb.User
}

func (c *countingUserClient) BatchGetUsers(ctx context.Context, req *socialflowpb.BatchGetUsersRequest, _ ...grpc.CallOption) (*socialflowpb.BatchGetUsersResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.calls++
	c.ids = append(c.ids, req.GetIds()...)

	users := make([]*socialflowpb.User, 0, len(req.GetIds()))
	for _, id := range req.GetIds() {
		if user, ok := c.users[id]; ok {
			users = append(users, user)
		}
	}
	return &socialflowpb.BatchGetUsersResponse{Users: users}, nil
}

func (c *countingUserClient) snapshot() (int, []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls, append([]string(nil), c.ids...)
}

type countingPostClient struct {
	socialflowpb.PostServiceClient

	mu      sync.Mutex
	calls   int
	viewers []string
	liked   map[string]map[string]bool
}

func (c *countingPostClient) GetLikeStates(ctx context.Context, req *socialflowpb.GetLikeStatesRequest, _ ...grpc.CallOption) (*socialflowpb.GetLikeStatesResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.calls++
	c.viewers = append(c.viewers, req.GetUserId())

	liked := make(map[string]bool, len(req.GetPostIds()))
	for _, postID := range req.GetPostIds() {
		if state, ok := c.liked[req.GetUserId()][postID]; ok {
			liked[postID] = state
		}
	}
	return &socialflowpb.GetLikeStatesResponse{Liked: liked}, nil
}

func (c *countingPostClient) snapshot() (int, []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls, append([]string(nil), c.viewers...)
}

func TestUserLoaderBatchesOneCallPerTick(t *testing.T) {
	client := &countingUserClient{
		users: map[string]*socialflowpb.User{
			"u1": {Id: "u1", Username: "one"},
			"u2": {Id: "u2", Username: "two"},
			"u3": {Id: "u3", Username: "three"},
		},
	}

	loaders := New(client, nil)
	ctx := context.Background()

	thunks := []func() (*socialflowpb.User, error){
		loaders.UserByID.Load(ctx, "u1"),
		loaders.UserByID.Load(ctx, "u2"),
		loaders.UserByID.Load(ctx, "u3"),
	}
	for i, thunk := range thunks {
		user, err := thunk()
		if err != nil {
			t.Fatalf("user %d: %v", i, err)
		}
		if user.GetId() == "" {
			t.Fatalf("user %d has no id", i)
		}
	}

	calls, ids := client.snapshot()
	if calls != 1 {
		t.Fatalf("BatchGetUsers calls = %d, want 1", calls)
	}
	if len(ids) != 3 {
		t.Fatalf("batched ids = %v", ids)
	}
}

func TestUserLoaderReturnsErrorForUnknownUser(t *testing.T) {
	loaders := New(&countingUserClient{}, nil)

	_, err := loaders.RequireUser(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected an error for an unknown user")
	}
}

func TestLikeLoaderGroupsByViewer(t *testing.T) {
	client := &countingPostClient{
		liked: map[string]map[string]bool{
			"viewer": {"p1": true, "p2": false, "p3": true},
		},
	}

	loaders := New(nil, client)
	ctx := context.Background()

	thunks := []func() (bool, error){
		loaders.LikeState.Load(ctx, LikeKey{ViewerID: "viewer", PostID: "p1"}),
		loaders.LikeState.Load(ctx, LikeKey{ViewerID: "viewer", PostID: "p2"}),
		loaders.LikeState.Load(ctx, LikeKey{ViewerID: "viewer", PostID: "p3"}),
	}
	want := []bool{true, false, true}
	for i, thunk := range thunks {
		got, err := thunk()
		if err != nil {
			t.Fatalf("like %d: %v", i, err)
		}
		if got != want[i] {
			t.Fatalf("like %d = %v, want %v", i, got, want[i])
		}
	}

	calls, viewers := client.snapshot()
	if calls != 1 {
		t.Fatalf("GetLikeStates calls = %d, want 1", calls)
	}
	if len(viewers) != 1 || viewers[0] != "viewer" {
		t.Fatalf("viewers = %v", viewers)
	}
}

func TestLikeLoaderNeverMixesViewers(t *testing.T) {
	client := &countingPostClient{
		liked: map[string]map[string]bool{
			"alice": {"p1": true},
			"bob":   {"p1": false},
		},
	}

	loaders := New(nil, client)
	ctx := context.Background()

	alice := loaders.LikeState.Load(ctx, LikeKey{ViewerID: "alice", PostID: "p1"})
	bob := loaders.LikeState.Load(ctx, LikeKey{ViewerID: "bob", PostID: "p1"})

	if liked, err := alice(); err != nil || !liked {
		t.Fatalf("alice = %v, %v", liked, err)
	}
	if liked, err := bob(); err != nil || liked {
		t.Fatalf("bob = %v, %v", liked, err)
	}

	if calls, _ := client.snapshot(); calls != 2 {
		t.Fatalf("GetLikeStates calls = %d, want one per viewer", calls)
	}
}
