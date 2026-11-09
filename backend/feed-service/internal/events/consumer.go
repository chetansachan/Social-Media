package events

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/chetansachan/socialflow/feed-service/internal/models"
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
// tell "the consumer is behind" apart from "the consumer is dead" long before
// a user notices their feed has stopped moving.
const lagInterval = 15 * time.Second

// Consumer reads domain events from a Redis Stream consumer group. Redis
// Streams are used here as a local, dependency-free stand-in for Kafka or
// RabbitMQ; the domain services only see models.Event, so the transport can be
// swapped without touching business logic.
type Consumer struct {
	client   *redis.Client
	handler  Handler
	stream   string
	group    string
	consumer string
	metrics  LagRecorder
}

func NewConsumer(client *redis.Client, handler Handler, stream, group, consumer string) *Consumer {
	return &Consumer{
		client:   client,
		handler:  handler,
		stream:   stream,
		group:    group,
		consumer: consumer,
	}
}

// SetMetrics attaches the backlog gauge. It is optional: the consumer works
// without a recorder, and reporting is skipped until this is called.
func (c *Consumer) SetMetrics(recorder LagRecorder) {
	c.metrics = recorder
}

// Run blocks until the context is cancelled. Redis disconnects are retried;
// malformed messages are acknowledged to avoid a poison-message loop.
func (c *Consumer) Run(ctx context.Context) error {
	if c.client == nil {
		return nil
	}
	if err := c.ensureGroup(ctx); err != nil {
		return err
	}

	reclaimTicker := time.NewTicker(30 * time.Second)
	defer reclaimTicker.Stop()

	lagTicker := time.NewTicker(lagInterval)
	defer lagTicker.Stop()
	// Publish once at startup so a fresh pod's gauge reflects reality before
	// the first interval elapses.
	c.reportLag(ctx)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-reclaimTicker.C:
			c.reclaimPending(ctx)
		case <-lagTicker.C:
			c.reportLag(ctx)
		default:
		}

		streams, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.group,
			Consumer: c.consumer,
			Streams:  []string{c.stream, ">"},
			Count:    50,
			Block:    2 * time.Second,
		}).Result()

		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("feed event consumer read error: %v", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
				continue
			}
		}

		for _, stream := range streams {
			for _, message := range stream.Messages {
				c.handleMessage(ctx, message)
			}
		}
	}
}

func (c *Consumer) ensureGroup(ctx context.Context) error {
	err := c.client.XGroupCreateMkStream(ctx, c.stream, c.group, "0").Err()
	if err == nil || strings.Contains(err.Error(), "BUSYGROUP") {
		return nil
	}
	return fmt.Errorf("failed to create consumer group %s: %w", c.group, err)
}

// reportLag publishes the number of stream entries that this consumer group has
// not yet delivered. Redis returns -1 when it cannot determine the lag (for
// example right after XGROUP SETID), so the gauge is clamped at zero instead of
// reporting a negative backlog.
func (c *Consumer) reportLag(ctx context.Context) {
	if c.metrics == nil {
		return
	}

	groups, err := c.client.XInfoGroups(ctx, c.stream).Result()
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("feed event consumer lag lookup failed: %v", err)
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
		c.metrics.SetConsumerLag(c.stream, c.group, lag)
		return
	}
}

// reclaimPending retries messages that were delivered but never acknowledged,
// for example because a downstream service was unavailable.
func (c *Consumer) reclaimPending(ctx context.Context) {
	start := "0-0"
	for attempt := 0; attempt < 10; attempt++ {
		messages, next, err := c.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream:   c.stream,
			Group:    c.group,
			Consumer: c.consumer,
			MinIdle:  30 * time.Second,
			Start:    start,
			Count:    50,
		}).Result()
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("feed event consumer reclaim error: %v", err)
			}
			return
		}
		for _, message := range messages {
			c.handleMessage(ctx, message)
		}
		if len(messages) == 0 || next == "0-0" {
			return
		}
		start = next
	}
}

func (c *Consumer) handleMessage(ctx context.Context, message redis.XMessage) {
	event, err := models.EventFromValues(message.Values)
	if err != nil {
		// A malformed message can never succeed on retry. Acknowledge it so it
		// does not block the consumer group forever.
		log.Printf("feed event consumer dropping malformed message %s: %v", message.ID, err)
		_ = c.client.XAck(ctx, c.stream, c.group, message.ID).Err()
		return
	}

	if err := c.handler.HandleEvent(ctx, event); err != nil {
		// Leave the message pending. A retry worker (or the next deployment)
		// can reclaim it after the downstream dependency recovers.
		log.Printf("feed event consumer failed to process %s (%s): %v", message.ID, event.Type, err)
		return
	}

	if err := c.client.XAck(ctx, c.stream, c.group, message.ID).Err(); err != nil {
		log.Printf("feed event consumer failed to ack message %s: %v", message.ID, err)
	}
}
