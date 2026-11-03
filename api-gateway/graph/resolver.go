package graph

import (
	"github.com/chetansachan/socialflow/api-gateway/graph/dataloader"
	socialflowpb "github.com/chetansachan/socialflow/proto"
	"github.com/redis/go-redis/v9"
)

type Resolver struct {
	UserClient         socialflowpb.UserServiceClient
	PostClient         socialflowpb.PostServiceClient
	FollowClient       socialflowpb.FollowServiceClient
	FeedClient         socialflowpb.FeedServiceClient
	NotificationClient socialflowpb.NotificationServiceClient

	// Redis carries real-time notification frames from notification-service to
	// GraphQL subscription clients through Pub/Sub channels.
	Redis *redis.Client

	// NewLoaders builds request-scoped batch loaders for author and like-state
	// lookups. Tests can supply fakes here.
	NewLoaders func() *dataloader.Loaders
}
