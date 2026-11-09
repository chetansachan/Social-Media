package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/chetansachan/socialflow/follow-service/internal/database"
	"github.com/chetansachan/socialflow/follow-service/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OutboxRepository claims and settles events for the relay worker.
type OutboxRepository interface {
	FetchPending(ctx context.Context, limit int) ([]models.OutboxEvent, error)
	MarkPublished(ctx context.Context, id uuid.UUID) error
	MarkFailed(ctx context.Context, id uuid.UUID, attempts int, reason string, backoff time.Duration, maxAttempts int) error
	ReclaimStale(ctx context.Context, olderThan time.Duration) (int64, error)
	PrunePublished(ctx context.Context, olderThan time.Duration) (int64, error)
}

type outboxRepository struct {
	db *pgxpool.Pool
}

// NewOutboxRepository always uses the primary: the relay must see the rows it
// just claimed and settled.
func NewOutboxRepository(db *database.DB) OutboxRepository {
	return &outboxRepository{db: db.Writer()}
}

// enqueueOutbox writes the event inside the caller's transaction.
func enqueueOutbox(ctx context.Context, tx pgx.Tx, eventType string, aggregateID uuid.UUID, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to encode %s outbox payload: %w", eventType, err)
	}

	const query = `
		INSERT INTO outbox_events (event_type, aggregate_id, payload)
		VALUES ($1, $2, $3)
	`
	if _, err := tx.Exec(ctx, query, eventType, aggregateID, encoded); err != nil {
		return fmt.Errorf("failed to enqueue %s event: %w", eventType, err)
	}
	return nil
}

func (r *outboxRepository) FetchPending(ctx context.Context, limit int) ([]models.OutboxEvent, error) {
	if limit <= 0 {
		limit = 100
	}

	const query = `
		WITH claimable AS (
			SELECT id
			FROM outbox_events
			WHERE status = 'PENDING' AND available_at <= NOW()
			ORDER BY created_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE outbox_events AS events
		SET status = 'PUBLISHING',
			locked_at = NOW(),
			attempts = events.attempts + 1
		FROM claimable
		WHERE events.id = claimable.id
		RETURNING events.id, events.event_type, events.aggregate_id, events.payload,
			events.status, events.attempts, events.available_at, events.created_at
	`

	rows, err := r.db.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to claim outbox events: %w", err)
	}
	defer rows.Close()

	events := make([]models.OutboxEvent, 0, limit)
	for rows.Next() {
		var event models.OutboxEvent
		if err := rows.Scan(
			&event.ID,
			&event.EventType,
			&event.AggregateID,
			&event.Payload,
			&event.Status,
			&event.Attempts,
			&event.AvailableAt,
			&event.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan outbox event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating outbox events: %w", err)
	}
	return events, nil
}

func (r *outboxRepository) MarkPublished(ctx context.Context, id uuid.UUID) error {
	const query = `
		UPDATE outbox_events
		SET status = 'PUBLISHED', published_at = NOW(), last_error = NULL
		WHERE id = $1
	`
	if _, err := r.db.Exec(ctx, query, id); err != nil {
		return fmt.Errorf("failed to mark outbox event %s published: %w", id, err)
	}
	return nil
}

func (r *outboxRepository) MarkFailed(
	ctx context.Context,
	id uuid.UUID,
	attempts int,
	reason string,
	backoff time.Duration,
	maxAttempts int,
) error {
	status := models.OutboxStatusPending
	if maxAttempts > 0 && attempts >= maxAttempts {
		status = models.OutboxStatusFailed
	}

	const query = `
		UPDATE outbox_events
		SET status = $2,
			last_error = $3,
			available_at = NOW() + $4::interval,
			locked_at = NULL
		WHERE id = $1
	`
	if _, err := r.db.Exec(ctx, query, id, status, reason, backoff.String()); err != nil {
		return fmt.Errorf("failed to reschedule outbox event %s: %w", id, err)
	}
	return nil
}

func (r *outboxRepository) ReclaimStale(ctx context.Context, olderThan time.Duration) (int64, error) {
	const query = `
		UPDATE outbox_events
		SET status = 'PENDING', locked_at = NULL
		WHERE status = 'PUBLISHING' AND locked_at < NOW() - $1::interval
	`
	tag, err := r.db.Exec(ctx, query, olderThan.String())
	if err != nil {
		return 0, fmt.Errorf("failed to reclaim stale outbox events: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (r *outboxRepository) PrunePublished(ctx context.Context, olderThan time.Duration) (int64, error) {
	const query = `
		DELETE FROM outbox_events
		WHERE status = 'PUBLISHED' AND published_at < NOW() - $1::interval
	`
	tag, err := r.db.Exec(ctx, query, olderThan.String())
	if err != nil {
		return 0, fmt.Errorf("failed to prune published outbox events: %w", err)
	}
	return tag.RowsAffected(), nil
}
