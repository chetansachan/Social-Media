package events

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/chetansachan/socialflow/notification-service/internal/models"
	"github.com/redis/go-redis/v9"
)

// Handler processes one decoded domain event.
type Handler interface {
	HandleEvent(ctx context.Context, event models.Event) error
}

// LagRecorder receives the backlog of a consumer group. The observability
// package's Metrics value implements it; keeping the dependency this narrow
// lets tests assert reporting without a Prometheus registry.
type LagRecorder interface {
	SetConsumerLag(stream, group string, lag int64)
}

// lagInterval bounds how stale the backlog gauge can be. Alerting uses it to
// tell "the consumer is behind" apart from "the consumer is dead".
const lagInterval = 15 * time.Second

// Options configures the consumer group and poison-message behaviour.
type Options struct {
	Streams       []string
	Group         string
	Consumer      string
	DLQStream     string
	ClaimInterval time.Duration
	MaxAttempts   int
	Logger        *log.Logger
}

// Consumer reads domain events from one or more Redis Streams consumer groups.
// Redis Streams are the local, dependency-free stand-in for Kafka; the handler
// only sees models.Event, so the broker can be swapped later.
type Consumer struct {
	client   *redis.Client
	handler  Handler
	streams  []string
	group    string
	consumer string
	dlq      string
	claim    time.Duration
	maxTries int
	logger   *log.Logger
	metrics  LagRecorder
}

func NewConsumer(client *redis.Client, handler Handler, options Options) *Consumer {
	if options.ClaimInterval <= 0 {
		options.ClaimInterval = 30 * time.Second
	}
	if options.MaxAttempts <= 0 {
		options.MaxAttempts = 5
	}
	if options.DLQStream == "" {
		options.DLQStream = "stream:notifications:dlq"
	}
	if options.Group == "" {
		options.Group = "notification-service"
	}
	if options.Consumer == "" {
		options.Consumer = "notification-service-1"
	}
	if options.Logger == nil {
		options.Logger = log.Default()
	}
	if len(options.Streams) == 0 {
		options.Streams = []string{"social:events"}
	}

	return &Consumer{
		client:   client,
		handler:  handler,
		streams:  options.Streams,
		group:    options.Group,
		consumer: options.Consumer,
		dlq:      options.DLQStream,
		claim:    options.ClaimInterval,
		maxTries: options.MaxAttempts,
		logger:   options.Logger,
	}
}

// SetMetrics attaches the backlog gauge. It is optional: the consumer works
// without a recorder, and reporting is skipped until this is called.
func (c *Consumer) SetMetrics(recorder LagRecorder) {
	c.metrics = recorder
}

// Run blocks until the context is cancelled. Each stream gets its own reader
// goroutine so a slow stream cannot starve the others.
func (c *Consumer) Run(ctx context.Context) error {
	if c.client == nil {
		return nil
	}

	for _, stream := range c.streams {
		if err := c.ensureGroup(ctx, stream); err != nil {
			return err
		}
	}

	var wg sync.WaitGroup
	for _, stream := range c.streams {
		wg.Add(1)
		go func(stream string) {
			defer wg.Done()
			c.loop(ctx, stream)
		}(stream)
	}

	wg.Wait()
	return nil
}

func (c *Consumer) ensureGroup(ctx context.Context, stream string) error {
	err := c.client.XGroupCreateMkStream(ctx, stream, c.group, "0").Err()
	if err == nil || strings.Contains(err.Error(), "BUSYGROUP") {
		return nil
	}
	return fmt.Errorf("failed to create consumer group %s on %s: %w", c.group, stream, err)
}

func (c *Consumer) loop(ctx context.Context, stream string) {
	ticker := time.NewTicker(c.claim)
	defer ticker.Stop()

	lagTicker := time.NewTicker(lagInterval)
	defer lagTicker.Stop()
	// Publish once at startup so a fresh pod's gauge reflects reality before
	// the first interval elapses.
	c.reportLag(ctx, stream)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.reclaimPending(ctx, stream)
		case <-lagTicker.C:
			c.reportLag(ctx, stream)
		default:
		}

		result, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.group,
			Consumer: c.consumer,
			Streams:  []string{stream, ">"},
			Count:    50,
			Block:    2 * time.Second,
		}).Result()

		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			c.logger.Printf("notification consumer read error on %s: %v", stream, err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
				continue
			}
		}

		for _, batch := range result {
			for _, message := range batch.Messages {
				c.handleMessage(ctx, stream, message)
			}
		}
	}
}

// reportLag publishes the number of stream entries this consumer group has not
// yet delivered. Redis returns -1 when the lag cannot be determined (for
// example right after XGROUP SETID), so the gauge is clamped at zero instead of
// reporting a negative backlog.
func (c *Consumer) reportLag(ctx context.Context, stream string) {
	if c.metrics == nil {
		return
	}

	groups, err := c.client.XInfoGroups(ctx, stream).Result()
	if err != nil {
		if ctx.Err() == nil {
			c.logger.Printf("notification consumer lag lookup failed on %s: %v", stream, err)
		}
		return
	}

	for _, group := range groups {
		if group.Name != c.group {
			continue
		}
		lag := group.Lag
		if lag < 0 {
			lag = 0
		}
		c.metrics.SetConsumerLag(stream, c.group, lag)
		return
	}
}

// reclaimPending retries messages delivered to a consumer that died before
// acknowledging them. XAutoClaim transfers ownership back to this consumer.
func (c *Consumer) reclaimPending(ctx context.Context, stream string) {
	start := "0-0"
	for attempt := 0; attempt < 10; attempt++ {
		messages, next, err := c.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream:   stream,
			Group:    c.group,
			Consumer: c.consumer,
			MinIdle:  c.claim,
			Start:    start,
			Count:    50,
		}).Result()
		if err != nil {
			if ctx.Err() == nil {
				c.logger.Printf("notification consumer reclaim error on %s: %v", stream, err)
			}
			return
		}

		for _, message := range messages {
			c.handleMessage(ctx, stream, message)
		}
		if len(messages) == 0 || next == "0-0" {
			return
		}
		start = next
	}
}

func (c *Consumer) handleMessage(ctx context.Context, stream string, message redis.XMessage) {
	attempts := c.recordAttempt(ctx, stream, message.ID)

	event, err := models.EventFromValues(message.Values)
	if err != nil {
		// Malformed events can never succeed. Route them to the DLQ and ack so
		// they do not pin the consumer group forever.
		c.deadLetter(ctx, stream, message, fmt.Errorf("decode failed: %w", err))
		c.ack(ctx, stream, message.ID)
		return
	}

	err = c.handler.HandleEvent(ctx, event)
	switch {
	case err == nil:
		c.ack(ctx, stream, message.ID)
	case errors.Is(err, models.ErrPermanentEvent):
		c.deadLetter(ctx, stream, message, err)
		c.ack(ctx, stream, message.ID)
	case attempts >= int64(c.maxTries):
		c.deadLetter(ctx, stream, message, fmt.Errorf("retry budget exhausted: %w", err))
		c.ack(ctx, stream, message.ID)
	default:
		// Leave the message pending. The reclaim loop retries it after the
		// downstream dependency recovers.
		c.logger.Printf("notification consumer failed %s/%s (attempt %d/%d): %v",
			stream, message.ID, attempts, c.maxTries, err)
	}
}

func (c *Consumer) recordAttempt(ctx context.Context, stream, messageID string) int64 {
	key := fmt.Sprintf("notif:attempts:%s:%s", stream, messageID)
	count, err := c.client.Incr(ctx, key).Result()
	if err != nil {
		return 1
	}
	_ = c.client.Expire(ctx, key, time.Hour).Err()
	return count
}

func (c *Consumer) ack(ctx context.Context, stream, messageID string) {
	if err := c.client.XAck(ctx, stream, c.group, messageID).Err(); err != nil {
		c.logger.Printf("notification consumer failed to ack %s/%s: %v", stream, messageID, err)
		return
	}
	_ = c.client.Del(ctx, fmt.Sprintf("notif:attempts:%s:%s", stream, messageID)).Err()
}

func (c *Consumer) deadLetter(ctx context.Context, stream string, message redis.XMessage, cause error) {
	values := map[string]any{
		"source_stream": stream,
		"source_id":     message.ID,
		"error":         cause.Error(),
		"failed_at":     time.Now().UTC().Format(time.RFC3339),
	}
	for key, value := range message.Values {
		values["event_"+key] = value
	}

	if err := c.client.XAdd(ctx, &redis.XAddArgs{
		Stream: c.dlq,
		Values: values,
	}).Err(); err != nil {
		c.logger.Printf("notification consumer failed to write DLQ entry for %s/%s: %v", stream, message.ID, err)
		return
	}
	c.logger.Printf("notification consumer moved %s/%s to DLQ: %v", stream, message.ID, cause)
}
