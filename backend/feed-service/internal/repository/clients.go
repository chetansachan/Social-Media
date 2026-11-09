package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/chetansachan/socialflow/feed-service/internal/models"
	followpb "github.com/chetansachan/socialflow/proto"
	postpb "github.com/chetansachan/socialflow/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	rpcTimeout       = 3 * time.Second
	authorWorkers    = 10
	hydrationWorkers = 15
)

// FollowClient is the Feed Service's view of the social graph.
type FollowClient interface {
	GetFollowingUserIDs(ctx context.Context, userID string) ([]string, error)
	GetFollowerUserIDs(ctx context.Context, userID string) ([]string, error)
	GetFollowCounts(ctx context.Context, userID string) (followers int32, following int32, err error)
}

// PostClient is the Feed Service's view of the post catalogue.
type PostClient interface {
	GetPostByID(ctx context.Context, postID string) (*models.FeedPost, error)
	GetPostsByAuthor(ctx context.Context, authorID string) ([]models.FeedPost, error)
	GetPostsByAuthorIDs(ctx context.Context, authorIDs []string) ([]models.FeedPost, error)
	HydratePosts(ctx context.Context, postIDs []string) (posts []models.FeedPost, missing []string, err error)
}

type grpcFollowClient struct {
	client followpb.FollowServiceClient
}

func NewFollowClient(client followpb.FollowServiceClient) FollowClient {
	return &grpcFollowClient{client: client}
}

func (c *grpcFollowClient) GetFollowingUserIDs(ctx context.Context, userID string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, rpcTimeout)
	defer cancel()

	resp, err := c.client.GetFollowing(ctx, &followpb.GetFollowingRequest{UserId: userID})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch following users: %w", err)
	}
	return resp.GetFollowingIds(), nil
}

func (c *grpcFollowClient) GetFollowerUserIDs(ctx context.Context, userID string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, rpcTimeout)
	defer cancel()

	resp, err := c.client.GetFollowers(ctx, &followpb.GetFollowersRequest{UserId: userID})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch followers: %w", err)
	}
	return resp.GetFollowerIds(), nil
}

func (c *grpcFollowClient) GetFollowCounts(ctx context.Context, userID string) (int32, int32, error) {
	ctx, cancel := context.WithTimeout(ctx, rpcTimeout)
	defer cancel()

	resp, err := c.client.GetFollowCounts(ctx, &followpb.GetFollowCountsRequest{UserId: userID})
	if err != nil {
		return 0, 0, fmt.Errorf("failed to fetch follow counts: %w", err)
	}
	return resp.GetFollowersCount(), resp.GetFollowingCount(), nil
}

type grpcPostClient struct {
	client postpb.PostServiceClient
}

func NewPostClient(client postpb.PostServiceClient) PostClient {
	return &grpcPostClient{client: client}
}

// GetPostByID returns (nil, nil) when Post Service reports NotFound. That is a
// deliberate signal: the post is deleted, archived, or no longer accessible.
func (c *grpcPostClient) GetPostByID(ctx context.Context, postID string) (*models.FeedPost, error) {
	ctx, cancel := context.WithTimeout(ctx, rpcTimeout)
	defer cancel()

	resp, err := c.client.GetPostById(ctx, &postpb.PostByIdRequest{Id: postID})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get post %s: %w", postID, err)
	}
	if resp.GetPost() == nil {
		return nil, nil
	}
	return toFeedPost(resp.GetPost()), nil
}

func (c *grpcPostClient) GetPostsByAuthor(ctx context.Context, authorID string) ([]models.FeedPost, error) {
	ctx, cancel := context.WithTimeout(ctx, rpcTimeout)
	defer cancel()

	resp, err := c.client.GetAllPosts(ctx, &postpb.AllPostRequest{AuthorId: authorID})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return []models.FeedPost{}, nil
		}
		return nil, fmt.Errorf("failed to fetch posts for author %s: %w", authorID, err)
	}

	posts := make([]models.FeedPost, 0, len(resp.GetPost()))
	for _, post := range resp.GetPost() {
		posts = append(posts, *toFeedPost(post))
	}
	return posts, nil
}

// GetPostsByAuthorIDs uses a bounded worker pool so a cold-start pull does not
// turn into thousands of sequential gRPC round-trips.
func (c *grpcPostClient) GetPostsByAuthorIDs(ctx context.Context, authorIDs []string) ([]models.FeedPost, error) {
	if len(authorIDs) == 0 {
		return []models.FeedPost{}, nil
	}

	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		allPosts []models.FeedPost
		errs     []error
		sem      = make(chan struct{}, authorWorkers)
	)

loop:
	for _, authorID := range authorIDs {
		select {
		case <-workCtx.Done():
			break loop
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			defer func() { <-sem }()

			posts, err := c.GetPostsByAuthor(workCtx, id)
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
				return
			}

			mu.Lock()
			allPosts = append(allPosts, posts...)
			mu.Unlock()
		}(authorID)
	}

	wg.Wait()

	if len(allPosts) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	if err := ctx.Err(); err != nil && len(allPosts) == 0 {
		return nil, err
	}
	return allPosts, nil
}

// HydratePosts turns a page of post IDs into rich feed cards. Post Service
// currently exposes a single-post RPC, so the adapter provides batch semantics
// with a bounded concurrent worker pool. Missing IDs are returned separately so
// the read path can prune ghost references without treating transient RPC
// failures as deletions.
func (c *grpcPostClient) HydratePosts(ctx context.Context, postIDs []string) ([]models.FeedPost, []string, error) {
	if len(postIDs) == 0 {
		return []models.FeedPost{}, nil, nil
	}

	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		byID    = make(map[string]models.FeedPost, len(postIDs))
		missing []string
		errs    []error
		sem     = make(chan struct{}, hydrationWorkers)
	)

loop:
	for _, postID := range postIDs {
		select {
		case <-workCtx.Done():
			break loop
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			defer func() { <-sem }()

			post, err := c.GetPostByID(workCtx, id)
			mu.Lock()
			defer mu.Unlock()

			switch {
			case err != nil:
				errs = append(errs, err)
			case post == nil:
				missing = append(missing, id)
			default:
				byID[id] = *post
			}
		}(postID)
	}

	wg.Wait()

	posts := make([]models.FeedPost, 0, len(byID))
	for _, postID := range postIDs {
		if post, ok := byID[postID]; ok {
			posts = append(posts, post)
		}
	}

	if len(posts) == 0 && len(errs) > 0 {
		return nil, missing, errors.Join(errs...)
	}
	if ctxErr := ctx.Err(); ctxErr != nil && len(posts) == 0 {
		return nil, missing, ctxErr
	}
	return posts, missing, nil
}

func toFeedPost(post *postpb.Post) *models.FeedPost {
	createdAt, err := time.Parse(time.RFC3339Nano, post.GetCreatedAt())
	if err != nil {
		createdAt, _ = time.Parse(time.RFC3339, post.GetCreatedAt())
	}

	var updatedAt *time.Time
	if post.GetUpdatedAt() != "" {
		parsed, err := time.Parse(time.RFC3339Nano, post.GetUpdatedAt())
		if err != nil {
			parsed, err = time.Parse(time.RFC3339, post.GetUpdatedAt())
		}
		if err == nil {
			updatedAt = &parsed
		}
	}

	return &models.FeedPost{
		ID:        post.GetId(),
		AuthorID:  post.GetAuthorId(),
		Content:   post.GetContent(),
		MediaURL:  post.GetMediaUrl(),
		IsPublic:  post.GetIsPublic(),
		LikeCount: post.GetLikeCount(),
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
}
