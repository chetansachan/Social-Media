package events

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"
	"time"

	"github.com/chetansachan/socialflow/post-service/internal/models"
	"github.com/google/uuid"
)

type fakeOutboxRepo struct {
	events    []models.OutboxEvent
	published []uuid.UUID
	failed    []uuid.UUID
	backoffs  []time.Duration
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

func (f *fakeOutboxRepo) MarkFailed(_ context.Context, id uuid.UUID, _ int, _ string, backoff time.Duration, _ int) error {
	f.failed = append(f.failed, id)
	f.backoffs = append(f.backoffs, backoff)
	return nil
}

func (f *fakeOutboxRepo) ReclaimStale(context.Context, time.Duration) (int64, error) {
	return 0, nil
}

func (f *fakeOutboxRepo) PrunePublished(context.Context, time.Duration) (int64, error) {
	return 0, nil
}

func (f *fakeOutboxRepo) Stats(context.Context) (int64, int64, error) {
	return 0, 0, nil
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
		{ID: first, EventType: "post.created"},
		{ID: second, EventType: "post.liked"},
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
	if got := repo.backoffs[0]; got != time.Second {
		t.Fatalf("first retry backoff = %s, want 1s", got)
	}
}

func TestOutboxRelay_BackoffGrowsAndCaps(t *testing.T) {
	relay := newTestRelay(&fakeOutboxRepo{}, &fakePublisher{})

	cases := []struct {
		attempts int
		want     time.Duration
	}{
		{attempts: 1, want: time.Second},
		{attempts: 2, want: 2 * time.Second},
		{attempts: 3, want: 4 * time.Second},
		{attempts: 4, want: 8 * time.Second},
		{attempts: 5, want: 10 * time.Second},
		{attempts: 20, want: 10 * time.Second},
	}
	for _, tc := range cases {
		if got := relay.backoff(tc.attempts); got != tc.want {
			t.Errorf("backoff(%d) = %s, want %s", tc.attempts, got, tc.want)
		}
	}
}

func TestOutboxRelay_DrainReturnsFetchError(t *testing.T) {
	sentinel := errors.New("postgres down")
	relay := newTestRelay(&fakeOutboxRepo{fetchErr: sentinel}, &fakePublisher{})

	if _, err := relay.Drain(context.Background()); !errors.Is(err, sentinel) {
		t.Fatalf("Drain error = %v, want %v", err, sentinel)
	}
}
