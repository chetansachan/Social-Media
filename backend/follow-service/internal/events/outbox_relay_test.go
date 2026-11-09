package events

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"
	"time"

	"github.com/chetansachan/socialflow/follow-service/internal/models"
	"github.com/google/uuid"
)

type fakeOutboxRepo struct {
	events    []models.OutboxEvent
	published []uuid.UUID
	failed    []uuid.UUID
	fetchErr  error
}

func (f *fakeOutboxRepo) FetchPending(context.Context, int) ([]models.OutboxEvent, error) {
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	events := f.events
	f.events = nil
	return events, nil
}

func (f *fakeOutboxRepo) MarkPublished(_ context.Context, id uuid.UUID) error {
	f.published = append(f.published, id)
	return nil
}

func (f *fakeOutboxRepo) MarkFailed(_ context.Context, id uuid.UUID, _ int, _ string, _ time.Duration, _ int) error {
	f.failed = append(f.failed, id)
	return nil
}

func (f *fakeOutboxRepo) ReclaimStale(context.Context, time.Duration) (int64, error) {
	return 0, nil
}

func (f *fakeOutboxRepo) PrunePublished(context.Context, time.Duration) (int64, error) {
	return 0, nil
}

type fakePublisher struct {
	failIDs map[uuid.UUID]bool
	sent    []uuid.UUID
}

func (f *fakePublisher) Publish(_ context.Context, event models.OutboxEvent) error {
	if f.failIDs[event.ID] {
		return errors.New("redis unavailable")
	}
	f.sent = append(f.sent, event.ID)
	return nil
}

func newTestRelay(repo *fakeOutboxRepo, publisher Publisher) *OutboxRelay {
	return NewOutboxRelay(repo, publisher, RelayOptions{
		BatchSize:   10,
		BaseBackoff: time.Second,
		MaxBackoff:  10 * time.Second,
		Logger:      log.New(io.Discard, "", 0),
	})
}

func TestOutboxRelay_DrainPublishesAndMarksEvents(t *testing.T) {
	first, second := uuid.New(), uuid.New()
	repo := &fakeOutboxRepo{events: []models.OutboxEvent{
		{ID: first, EventType: "user.followed"},
		{ID: second, EventType: "user.unfollowed"},
	}}
	publisher := &fakePublisher{failIDs: map[uuid.UUID]bool{second: true}}
	relay := newTestRelay(repo, publisher)

	count, err := relay.Drain(context.Background())
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if count != 1 {
		t.Fatalf("published count = %d, want 1", count)
	}
	if len(repo.published) != 1 || repo.published[0] != first {
		t.Fatalf("published = %v, want [%s]", repo.published, first)
	}
	if len(repo.failed) != 1 || repo.failed[0] != second {
		t.Fatalf("failed = %v, want [%s]", repo.failed, second)
	}
}

func TestOutboxRelay_BackoffCapsAtMaximum(t *testing.T) {
	relay := newTestRelay(&fakeOutboxRepo{}, &fakePublisher{})
	if got := relay.backoff(1); got != time.Second {
		t.Fatalf("backoff(1) = %s, want 1s", got)
	}
	if got := relay.backoff(10); got != 10*time.Second {
		t.Fatalf("backoff(10) = %s, want 10s", got)
	}
}
