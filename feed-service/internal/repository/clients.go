package repository

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/chetansachan/socialflow/feed-service/internal/models"
	followpb "github.com/chetansachan/socialflow/proto"
	postpb "github.com/chetansachan/socialflow/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// FollowClient defines the interface for interacting with Follow Service.
type FollowClient interface {
	GetFollowingUserIDs(ctx context.Context, userID string) ([]string, error)
	GetFollowerUserIDs(ctx context.Context, userID string) ([]string, error)
	GetFollowCounts(ctx context.Context, userID string) (int32, int32, error)
}

// PostClient defines the interface for interacting with Post Service.
type PostClient interface {
	GetPostByID(ctx context.Context, postID string) (*models.FeedPost, error)
	GetPostsByAuthor(ctx context.Context, authorID string) ([]models.FeedPost, error)
	GetPostsByAuthorIDs(ctx context.Context, authorIDs []string) ([]models.FeedPost, error)
	BatchGetPosts(ctx context.Context, postIDs []string) ([]models.FeedPost, error)
}

type grpcFollowClient struct {
	client followpb.FollowServiceClient
}

// NewFollowClient creates a new gRPC Follow client adapter.
func NewFollowClient(client followpb.FollowServiceClient) FollowClient {
	return &grpcFollowClient{client: client}
}

func (c *grpcFollowClient) GetFollowingUserIDs(ctx context.Context, userID string) ([]string, error) {
	resp, err := c.client.GetFollowing(ctx, &followpb.GetFollowingRequest{
		UserId: userID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch following users: %w", err)
	}
	return resp.GetFollowingIds(), nil
}

func (c *grpcFollowClient) GetFollowerUserIDs(ctx context.Context, userID string) ([]string, error) {
	resp, err := c.client.GetFollowers(ctx, &followpb.GetFollowersRequest{
		UserId: userID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch followers: %w", err)
	}
	return resp.GetFollowerIds(), nil
}

func (c *grpcFollowClient) GetFollowCounts(ctx context.Context, userID string) (int32, int32, error) {
	resp, err := c.client.GetFollowCounts(ctx, &followpb.GetFollowCountsRequest{
		UserId: userID,
	})
	if err != nil {
		return 0, 0, fmt.Errorf("failed to fetch follow counts: %w", err)
	}
	return resp.GetFollowersCount(), resp.GetFollowingCount(), nil
}

type grpcPostClient struct {
	client postpb.PostServiceClient
}

// NewPostClient creates a new gRPC Post client adapter.
func NewPostClient(client postpb.PostServiceClient) PostClient {
	return &grpcPostClient{client: client}
}

func (c *grpcPostClient) GetPostByID(ctx context.Context, postID string) (*models.FeedPost, error) {
	resp, err := c.client.GetPostById(ctx, &postpb.PostByIdRequest{
		Id: postID,
	})
	if err != nil {
		st, ok := status.FromError(err)
		if ok && st.Code() == codes.NotFound {
			return nil, nil // Post was deleted or not found
		}
		return nil, fmt.Errorf("failed to get post %s: %w", postID, err)
	}

	p := resp.GetPost()
	if p == nil {
		return nil, nil
	}

	return toFeedPost(p), nil
}

func (c *grpcPostClient) GetPostsByAuthor(ctx context.Context, authorID string) ([]models.FeedPost, error) {
	resp, err := c.client.GetAllPosts(ctx, &postpb.AllPostRequest{
		AuthorId: authorID,
	})
	if err != nil {
		st, ok := status.FromError(err)
		if ok && st.Code() == codes.NotFound {
			return []models.FeedPost{}, nil
		}
		return nil, fmt.Errorf("failed to fetch posts for author %s: %w", authorID, err)
	}

	posts := make([]models.FeedPost, 0, len(resp.GetPost()))
	for _, p := range resp.GetPost() {
		posts = append(posts, *toFeedPost(p))
	}

	return posts, nil
}

func (c *grpcPostClient) GetPostsByAuthorIDs(ctx context.Context, authorIDs []string) ([]models.FeedPost, error) {
	if len(authorIDs) == 0 {
		return []models.FeedPost{}, nil
	}

	var (
		wg         sync.WaitGroup
		mu         sync.Mutex
		allPosts   []models.FeedPost
		maxWorkers = 10
		sem        = make(chan struct{}, maxWorkers)
	)

	for _, authorID := range authorIDs {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(aid string) {
			defer wg.Done()
			defer func() { <-sem }()

			posts, err := c.GetPostsByAuthor(ctx, aid)
			if err != nil {
				return
			}

			mu.Lock()
			allPosts = append(allPosts, posts...)
			mu.Unlock()
		}(authorID)
	}

	wg.Wait()
	return allPosts, nil
}

// BatchGetPosts retrieves multiple posts concurrently, eliminating the N+1 problem.
func (c *grpcPostClient) BatchGetPosts(ctx context.Context, postIDs []string) ([]models.FeedPost, error) {
	if len(postIDs) == 0 {
		return []models.FeedPost{}, nil
	}

	var (
		wg         sync.WaitGroup
		mu         sync.Mutex
		postMap    = make(map[string]*models.FeedPost, len(postIDs))
		maxWorkers = 15
		sem        = make(chan struct{}, maxWorkers)
	)

	for _, pid := range postIDs {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			defer func() { <-sem }()

			post, err := c.GetPostByID(ctx, id)
			if err != nil || post == nil {
				return
			}

			mu.Lock()
			postMap[id] = post
			mu.Unlock()
		}(pid)
	}

	wg.Wait()

	// Maintain original postIDs slice order
	results := make([]models.FeedPost, 0, len(postMap))
	for _, pid := range postIDs {
		if post, exists := postMap[pid]; exists && post != nil {
			results = append(results, *post)
		}
	}

	return results, nil
}

func toFeedPost(p *postpb.Post) *models.FeedPost {
	createdAt, err := time.Parse(time.RFC3339, p.GetCreatedAt())
	if err != nil {
		createdAt = time.Now()
	}

	var updatedAt *time.Time
	if p.GetUpdatedAt() != "" {
		if parsedUpdated, err := time.Parse(time.RFC3339, p.GetUpdatedAt()); err == nil {
			updatedAt = &parsedUpdated
		}
	}

	return &models.FeedPost{
		ID:        p.GetId(),
		AuthorID:  p.GetAuthorId(),
		Content:   p.GetContent(),
		MediaURL:  p.GetMediaUrl(),
		IsPublic:  p.GetIsPublic(),
		LikeCount: p.GetLikeCount(),
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
}
