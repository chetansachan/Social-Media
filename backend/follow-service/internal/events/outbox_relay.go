package events

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/chetansachan/socialflow/follow-service/internal/models"
	"github.com/chetansachan/socialflow/follow-service/internal/repository"
	"github.com/redis/go-redis/v9"
)

// Publisher sends one outbox event to the message broker.
type Publisher interface {
	Publish(ctx context.Context, event models.OutboxEvent) error
}

// RedisPublisher writes the canonical event envelope to a Redis Stream.
type RedisPublisher struct {
	client *redis.Client
	stream string
}

func NewRedisPublisher(client *redis.Client, stream string) *RedisPublisher {
	return &RedisPublisher{client: client, stream: stream}
}

func (p *RedisPublisher) Publish(ctx context.Context, event models.OutboxEvent) error {
	envelope, err := event.MarshalEnvelope()
	if err != nil {
		return fmt.Errorf("failed to encode outbox event %s: %w", event.ID, err)
	}
	return p.client.XAdd(ctx, &redis.XAddArgs{
		Stream: p.stream,
		Values: map[string]any{"event": string(envelope)},
	}).Err()
}

// RelayOptions controls the outbox relay loop.
type RelayOptions struct {
	PollInterval time.Duration
	BatchSize    int
	MaxAttempts  int
	StaleAfter   time.Duration
	PruneAfter   time.Duration
	ReclaimEvery time.Duration
	BaseBackoff  time.Duration
	MaxBackoff   time.Duration
	Logger       *log.Logger
}

func (o *RelayOptions) defaults() {
	if o.PollInterval <= 0 {
		o.PollInterval = 500 * time.Millisecond
	}
	if o.BatchSize <= 0 {
		o.BatchSize = 100
	}
	if o.MaxAttempts <= 0 {
		o.MaxAttempts = 10
	}
	if o.StaleAfter <= 0 {
		o.StaleAfter = 2 * time.Minute
	}
	if o.PruneAfter <= 0 {
		o.PruneAfter = 7 * 24 * time.Hour
	}
	if o.ReclaimEvery <= 0 {
		o.ReclaimEvery = time.Minute
	}
	if o.BaseBackoff <= 0 {
		o.BaseBackoff = 2 * time.Second
	}
	if o.MaxBackoff <= 0 {
		o.MaxBackoff = 5 * time.Minute
	}
	if o.Logger == nil {
		o.Logger = log.Default()
	}
}

// OutboxRelay drains outbox_events into the broker. FOR UPDATE SKIP LOCKED in
// the repository makes it safe to run several instances.
type OutboxRelay struct {
	repo      repository.OutboxRepository
	publisher Publisher
	options   RelayOptions
}

func NewOutboxRelay(repo repository.OutboxRepository, publisher Publisher, options RelayOptions) *OutboxRelay {
	options.defaults()
	return &OutboxRelay{repo: repo, publisher: publisher, options: options}
}

func (r *OutboxRelay) Run(ctx context.Context) {
	r.options.Logger.Printf("outbox relay started (batch=%d, poll=%s)", r.options.BatchSize, r.options.PollInterval)

	ticker := time.NewTicker(r.options.PollInterval)
	defer ticker.Stop()
	maintenance := time.NewTicker(r.options.ReclaimEvery)
	defer maintenance.Stop()

	for {
		select {
		case <-ctx.Done():
			r.options.Logger.Printf("outbox relay stopped")
			return
		case <-maintenance.C:
			r.maintain(ctx)
		case <-ticker.C:
			if _, err := r.Drain(ctx); err != nil && ctx.Err() == nil {
				r.options.Logger.Printf("outbox relay drain error: %v", err)
			}
		}
	}
}

// Drain publishes at most one batch and returns the number of events settled.
func (r *OutboxRelay) Drain(ctx context.Context) (int, error) {
	events, err := r.repo.FetchPending(ctx, r.options.BatchSize)
	if err != nil {
		return 0, err
	}

	published := 0
	for _, event := range events {
		if err := ctx.Err(); err != nil {
			return published, err
		}

		if err := r.publisher.Publish(ctx, event); err != nil {
			backoff := r.backoff(event.Attempts)
			if markErr := r.repo.MarkFailed(ctx, event.ID, event.Attempts, err.Error(), backoff, r.options.MaxAttempts); markErr != nil {
				r.options.Logger.Printf("outbox relay failed to reschedule %s: %v", event.ID, markErr)
			}
			r.options.Logger.Printf("outbox relay publish %s (%s) failed: %v; retrying in %s",
				event.ID, event.EventType, err, backoff)
			continue
		}

		if err := r.repo.MarkPublished(ctx, event.ID); err != nil {
			r.options.Logger.Printf("outbox relay failed to mark %s published: %v", event.ID, err)
			continue
		}
		published++
	}

	return published, nil
}

func (r *OutboxRelay) maintain(ctx context.Context) {
	reclaimed, err := r.repo.ReclaimStale(ctx, r.options.StaleAfter)
	if err != nil {
		r.options.Logger.Printf("outbox relay stale reclaim failed: %v", err)
	} else if reclaimed > 0 {
		r.options.Logger.Printf("outbox relay reclaimed %d stale events", reclaimed)
	}

	pruned, err := r.repo.PrunePublished(ctx, r.options.PruneAfter)
	if err != nil {
		r.options.Logger.Printf("outbox relay prune failed: %v", err)
	} else if pruned > 0 {
		r.options.Logger.Printf("outbox relay pruned %d published events", pruned)
	}
}

func (r *OutboxRelay) backoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	factor := math.Pow(2, float64(attempts-1))
	delay := time.Duration(float64(r.options.BaseBackoff) * factor)
	if delay > r.options.MaxBackoff {
		return r.options.MaxBackoff
	}
	return delay
}
