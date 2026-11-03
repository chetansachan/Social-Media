// Package dataloader batches the per-field lookups that GraphQL would
// otherwise issue once per object. A feed page of 20 posts resolves every
// author with a single BatchGetUsers call and every like state with a single
// GetLikeStates call.
package dataloader

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	gopherdl "github.com/graph-gophers/dataloader/v7"

	socialflowpb "github.com/chetansachan/socialflow/proto"
)

const (
	userBatchCapacity = 200
	likeBatchCapacity = 500
)

// LikeKey identifies one viewer/post pair. The viewer is part of the key so a
// single request can never observe another user's like state.
type LikeKey struct {
	ViewerID string
	PostID   string
}

// Loaders is the request-scoped set of batch loaders. Each field resolver asks
// its loader for one key; the loader coalesces every key requested during the
// same resolution tick into one RPC.
type Loaders struct {
	UserByID  gopherdl.Interface[string, *socialflowpb.User]
	LikeState gopherdl.Interface[LikeKey, bool]
}

// New builds loaders bound to the supplied gRPC clients.
func New(userClient socialflowpb.UserServiceClient, postClient socialflowpb.PostServiceClient) *Loaders {
	return &Loaders{
		UserByID:  newUserLoader(userClient),
		LikeState: newLikeLoader(postClient),
	}
}

func newUserLoader(client socialflowpb.UserServiceClient) *gopherdl.Loader[string, *socialflowpb.User] {
	batch := func(ctx context.Context, ids []string) []*gopherdl.Result[*socialflowpb.User] {
		results := make([]*gopherdl.Result[*socialflowpb.User], len(ids))
		if client == nil {
			return fillResults(results, fmt.Errorf("user client is not configured"))
		}

		resp, err := client.BatchGetUsers(ctx, &socialflowpb.BatchGetUsersRequest{Ids: ids})
		if err != nil {
			return fillResults(results, fmt.Errorf("batch get users: %w", err))
		}

		byID := make(map[string]*socialflowpb.User, len(resp.GetUsers()))
		for _, user := range resp.GetUsers() {
			byID[user.GetId()] = user
		}
		for i, id := range ids {
			if user, ok := byID[id]; ok {
				results[i] = &gopherdl.Result[*socialflowpb.User]{Data: user}
				continue
			}
			results[i] = &gopherdl.Result[*socialflowpb.User]{
				Error: fmt.Errorf("user %s not found", id),
			}
		}
		return results
	}

	return gopherdl.NewBatchedLoader(batch, gopherdl.WithBatchCapacity[string, *socialflowpb.User](userBatchCapacity))
}

func newLikeLoader(client socialflowpb.PostServiceClient) *gopherdl.Loader[LikeKey, bool] {
	batch := func(ctx context.Context, keys []LikeKey) []*gopherdl.Result[bool] {
		results := make([]*gopherdl.Result[bool], len(keys))
		if client == nil {
			return fillBoolResults(results, fmt.Errorf("post client is not configured"))
		}

		// Feed pages normally contain one viewer, but grouping keeps the loader
		// correct if a single query ever spans several principals.
		byViewer := make(map[string][]string)
		for _, key := range keys {
			byViewer[key.ViewerID] = append(byViewer[key.ViewerID], key.PostID)
		}

		liked := make(map[LikeKey]bool, len(keys))
		for viewer, postIDs := range byViewer {
			resp, err := client.GetLikeStates(ctx, &socialflowpb.GetLikeStatesRequest{
				UserId:  viewer,
				PostIds: postIDs,
			})
			if err != nil {
				return fillBoolResults(results, fmt.Errorf("get like states for %s: %w", viewer, err))
			}
			for postID, isLiked := range resp.GetLiked() {
				liked[LikeKey{ViewerID: viewer, PostID: postID}] = isLiked
			}
		}

		for i, key := range keys {
			results[i] = &gopherdl.Result[bool]{Data: liked[key]}
		}
		return results
	}

	return gopherdl.NewBatchedLoader(batch, gopherdl.WithBatchCapacity[LikeKey, bool](likeBatchCapacity))
}

func fillResults(results []*gopherdl.Result[*socialflowpb.User], err error) []*gopherdl.Result[*socialflowpb.User] {
	for i := range results {
		results[i] = &gopherdl.Result[*socialflowpb.User]{Error: err}
	}
	return results
}

func fillBoolResults(results []*gopherdl.Result[bool], err error) []*gopherdl.Result[bool] {
	for i := range results {
		results[i] = &gopherdl.Result[bool]{Error: err}
	}
	return results
}

type contextKey struct{}

// WithLoaders attaches request-scoped loaders to the context.
func WithLoaders(ctx context.Context, loaders *Loaders) context.Context {
	return context.WithValue(ctx, contextKey{}, loaders)
}

// FromContext returns the loaders attached to this request. It returns nil for
// contexts that never passed through Middleware (tests, background jobs).
func FromContext(ctx context.Context) *Loaders {
	loaders, _ := ctx.Value(contextKey{}).(*Loaders)
	return loaders
}

// Middleware installs a fresh loader set on every HTTP request. Sharing one
// loader across requests would mix users' cached like states.
func Middleware(factory func() *Loaders) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if factory != nil {
				r = r.WithContext(WithLoaders(r.Context(), factory()))
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireUser loads a user and returns a descriptive error when the id is
// absent from user-service.
func (l *Loaders) RequireUser(ctx context.Context, id string) (*socialflowpb.User, error) {
	if l == nil || l.UserByID == nil {
		return nil, fmt.Errorf("user loader is not configured")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("user id is empty")
	}
	return l.UserByID.Load(ctx, id)()
}
