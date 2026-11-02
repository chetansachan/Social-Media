package repository

import (
	"context"

	"github.com/chetansachan/socialflow/feed-service/internal/models"
	followpb "github.com/chetansachan/socialflow/proto"
	postpb "github.com/chetansachan/socialflow/proto"
	"github.com/redis/go-redis/v9"
)

// FeedRepository is the single data-access facade used by the domain services.
// It combines the Redis timeline store with the Follow and Post gRPC clients.
type FeedRepository interface {
	TimelineCache() TimelineCache

	GetFollowingUserIDs(ctx context.Context, userID string) ([]string, error)
	GetFollowerUserIDs(ctx context.Context, userID string) ([]string, error)
	GetFollowCounts(ctx context.Context, userID string) (int32, int32, error)
	GetPostsByAuthor(ctx context.Context, authorID string) ([]models.FeedPost, error)
	GetPostsByAuthorIDs(ctx context.Context, authorIDs []string) ([]models.FeedPost, error)
	HydratePosts(ctx context.Context, postIDs []string) ([]models.FeedPost, []string, error)
}

type feedRepository struct {
	timelineCache TimelineCache
	followClient  FollowClient
	postClient    PostClient
}

// NewFeedRepository wires the production Redis cache and gRPC adapters.
func NewFeedRepository(followClient followpb.FollowServiceClient, postClient postpb.PostServiceClient, client *redis.Client) FeedRepository {
	return &feedRepository{
		timelineCache: NewRedisTimelineCache(client),
		followClient:  NewFollowClient(followClient),
		postClient:    NewPostClient(postClient),
	}
}

// NewFeedRepositoryWithClients exists for tests and alternate transports.
func NewFeedRepositoryWithClients(cache TimelineCache, followClient FollowClient, postClient PostClient) FeedRepository {
	if cache == nil {
		cache = &noopTimelineCache{}
	}
	return &feedRepository{
		timelineCache: cache,
		followClient:  followClient,
		postClient:    postClient,
	}
}

func (r *feedRepository) TimelineCache() TimelineCache {
	return r.timelineCache
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

func (r *feedRepository) HydratePosts(ctx context.Context, postIDs []string) ([]models.FeedPost, []string, error) {
	return r.postClient.HydratePosts(ctx, postIDs)
}
