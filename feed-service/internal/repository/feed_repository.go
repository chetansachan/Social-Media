package repository

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/chetansachan/socialflow/feed-service/internal/models"
	followpb "github.com/chetansachan/socialflow/proto"
	postpb "github.com/chetansachan/socialflow/proto"
)

type FeedRepository interface {
	GetFollowingUserIDs(ctx context.Context, userID string) ([]string, error)
	GetPostsByAuthor(ctx context.Context, authorID string) ([]models.FeedPost, error)
	GetPostsByAuthorIDs(ctx context.Context, authorIDs []string) ([]models.FeedPost, error)
}

type feedRepository struct {
	followClient followpb.FollowServiceClient
	postClient   postpb.PostServiceClient
}

func NewFeedRepository(followClient followpb.FollowServiceClient, postClient postpb.PostServiceClient) FeedRepository {
	return &feedRepository{
		followClient: followClient,
		postClient:   postClient,
	}
}

func (r *feedRepository) GetFollowingUserIDs(ctx context.Context, userID string) ([]string, error) {
	resp, err := r.followClient.GetFollowing(ctx, &followpb.GetFollowingRequest{
		UserId: userID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch following users: %w", err)
	}

	return resp.GetFollowingIds(), nil
}

func (r *feedRepository) GetPostsByAuthor(ctx context.Context, authorID string) ([]models.FeedPost, error) {
	resp, err := r.postClient.GetAllPosts(ctx, &postpb.AllPostRequest{
		AuthorId: authorID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch posts for author %s: %w", authorID, err)
	}

	posts := make([]models.FeedPost, 0, len(resp.GetPost()))
	for _, p := range resp.GetPost() {
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

		posts = append(posts, models.FeedPost{
			ID:        p.GetId(),
			AuthorID:  p.GetAuthorId(),
			Content:   p.GetContent(),
			MediaURL:  p.GetMediaUrl(),
			IsPublic:  p.GetIsPublic(),
			LikeCount: p.GetLikeCount(),
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		})
	}

	return posts, nil
}

func (r *feedRepository) GetPostsByAuthorIDs(ctx context.Context, authorIDs []string) ([]models.FeedPost, error) {
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

			posts, err := r.GetPostsByAuthor(ctx, aid)
			if err != nil {
				// Resilient fallback: log or skip author if their posts cannot be fetched
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
