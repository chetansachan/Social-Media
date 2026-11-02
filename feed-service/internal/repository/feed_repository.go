package repository

import (
	"context"

	"github.com/chetansachan/socialflow/feed-service/internal/models"
	followpb "github.com/chetansachan/socialflow/proto"
	postpb "github.com/chetansachan/socialflow/proto"
	"github.com/redis/go-redis/v9"
)

// FeedRepository provides unified data access across the timeline cache, follow service, and post service.
type FeedRepository interface {
	TimelineCache() TimelineCache
	FollowClient() FollowClient
	PostClient() PostClient

	GetFollowingUserIDs(ctx context.Context, userID string) ([]string, error)
	GetFollowerUserIDs(ctx context.Context, userID string) ([]string, error)
	GetFollowCounts(ctx context.Context, userID string) (int32, int32, error)
	GetPostsByAuthor(ctx context.Context, authorID string) ([]models.FeedPost, error)
	GetPostsByAuthorIDs(ctx context.Context, authorIDs []string) ([]models.FeedPost, error)
	BatchGetPosts(ctx context.Context, postIDs []string) ([]models.FeedPost, error)
}

type feedRepository struct {
	timelineCache TimelineCache
	followClient  FollowClient
	postClient    PostClient
}

// NewFeedRepository creates a unified repository instance.
func NewFeedRepository(followClient followpb.FollowServiceClient, postClient postpb.PostServiceClient, rdb *redis.Client) FeedRepository {
	return &feedRepository{
		timelineCache: NewRedisTimelineCache(rdb),
		followClient:  NewFollowClient(followClient),
		postClient:    NewPostClient(postClient),
	}
}

// NewFeedRepositoryWithClients allows injection of custom or mocked client adapters.
func NewFeedRepositoryWithClients(tc TimelineCache, fc FollowClient, pc PostClient) FeedRepository {
	if tc == nil {
		tc = &noopTimelineCache{}
	}
	return &feedRepository{
		timelineCache: tc,
		followClient:  fc,
		postClient:    pc,
	}
}

func (r *feedRepository) TimelineCache() TimelineCache {
	return r.timelineCache
}

func (r *feedRepository) FollowClient() FollowClient {
	return r.followClient
}

func (r *feedRepository) PostClient() PostClient {
	return r.postClient
}

func (r *feedRepository) GetFollowingUserIDs(ctx context.Context, userID string) ([]string, error) {
	return r.followClient.GetFollowingUserIDs(ctx, userID)
}

func (r *feedRepository) GetFollowerUserIDs(ctx context.Context, userID string) ([]string, error) {
	return r.followClient.GetFollowerUserIDs(ctx, userID)
}

func (r *feedRepository) GetFollowCounts(ctx context.Context, userID string) (int32, int32, error) {
	return r.followClient.GetFollowCounts(ctx, userID)
}

func (r *feedRepository) GetPostsByAuthor(ctx context.Context, authorID string) ([]models.FeedPost, error) {
	return r.postClient.GetPostsByAuthor(ctx, authorID)
}

func (r *feedRepository) GetPostsByAuthorIDs(ctx context.Context, authorIDs []string) ([]models.FeedPost, error) {
	return r.postClient.GetPostsByAuthorIDs(ctx, authorIDs)
}

func (r *feedRepository) BatchGetPosts(ctx context.Context, postIDs []string) ([]models.FeedPost, error) {
	return r.postClient.BatchGetPosts(ctx, postIDs)
}
