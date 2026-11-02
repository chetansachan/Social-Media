package dispatchers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/chetansachan/socialflow/notification-service/internal/models"
	"github.com/redis/go-redis/v9"
)

// RealtimeDispatcher publishes notification frames to the Redis Pub/Sub
// channel watched by the WebSocket gateway that holds the recipient's socket.
// This is what makes delivery work across multiple gateway pods.
type RealtimeDispatcher struct {
	client *redis.Client
	logger *log.Logger
}

func NewRealtimeDispatcher(client *redis.Client, logger *log.Logger) *RealtimeDispatcher {
	if logger == nil {
		logger = log.Default()
	}
	return &RealtimeDispatcher{client: client, logger: logger}
}

// Publish is best-effort: the durable inbox row is already committed, so a
// Pub/Sub failure must not fail the event. The client reconnects with a cursor.
func (d *RealtimeDispatcher) Publish(ctx context.Context, userID string, payload models.RealtimePayload) error {
	if d == nil || d.client == nil {
		return nil
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to encode realtime payload: %w", err)
	}

	channel := "notify:" + userID
	if err := d.client.Publish(ctx, channel, encoded).Err(); err != nil {
		return fmt.Errorf("failed to publish realtime payload: %w", err)
	}
	return nil
}
