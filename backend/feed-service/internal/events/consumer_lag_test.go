package events

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type lagSample struct {
	stream string
	group  string
	lag    int64
}

type recordingLag struct {
	samples []lagSample
}

func (r *recordingLag) SetConsumerLag(stream, group string, lag int64) {
	r.samples = append(r.samples, lagSample{stream: stream, group: group, lag: lag})
}

func newTestConsumer(t *testing.T) (*Consumer, *redis.Client) {
	t.Helper()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return NewConsumer(client, nil, "social:events", "feed-service", "feed-1"), client
}

func TestReportLagPublishesGroupBacklog(t *testing.T) {
	consumer, client := newTestConsumer(t)
	ctx := context.Background()

	if err := consumer.ensureGroup(ctx); err != nil {
		t.Fatalf("ensure group: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := client.XAdd(ctx, &redis.XAddArgs{
			Stream: consumer.stream,
			Values: map[string]any{"type": "post.created"},
		}).Err(); err != nil {
			t.Fatalf("add event %d: %v", i, err)
		}
	}

	recorder := &recordingLag{}
	consumer.SetMetrics(recorder)
	consumer.reportLag(ctx)

	if len(recorder.samples) != 1 {
		t.Fatalf("samples = %d, want 1", len(recorder.samples))
	}
	got := recorder.samples[0]
	if got.stream != "social:events" || got.group != "feed-service" {
		t.Fatalf("labels = %s/%s, want social:events/feed-service", got.stream, got.group)
	}
	// The group was created at 0-0 and nothing has been read, so all three
	// entries are undelivered.
	if got.lag != 3 {
		t.Fatalf("lag = %d, want 3", got.lag)
	}
}

func TestReportLagSkipsForeignGroupsAndMissingRecorder(t *testing.T) {
	consumer, client := newTestConsumer(t)
	ctx := context.Background()

	if err := consumer.ensureGroup(ctx); err != nil {
		t.Fatalf("ensure group: %v", err)
	}
	if err := client.XAdd(ctx, &redis.XAddArgs{
		Stream: consumer.stream,
		Values: map[string]any{"type": "post.created"},
	}).Err(); err != nil {
		t.Fatalf("add event: %v", err)
	}

	// Without a recorder the lookup is skipped entirely and must not panic.
	consumer.reportLag(ctx)

	recorder := &recordingLag{}
	consumer.SetMetrics(recorder)

	// Every service has its own group on the shared stream. A consumer must
	// publish only its own backlog, never another service's.
	consumer.group = "notification-service"
	consumer.reportLag(ctx)
	if len(recorder.samples) != 0 {
		t.Fatalf("samples = %v, want none for a foreign group", recorder.samples)
	}
}
