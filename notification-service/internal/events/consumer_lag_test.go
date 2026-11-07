package events

import (
	"context"
	"log"
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

func newLagTestConsumer(t *testing.T) (*Consumer, *redis.Client) {
	t.Helper()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	consumer := NewConsumer(client, nil, Options{
		Streams:  []string{"social:events", "social:notifications"},
		Group:    "notification-service",
		Consumer: "notification-1",
		Logger:   log.New(testWriter{t}, "", 0),
	})
	return consumer, client
}

// testWriter routes consumer warnings into the test log so a failed lookup is
// visible in the test output instead of being swallowed.
type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Logf("%s", p)
	return len(p), nil
}

func TestReportLagPublishesEachStream(t *testing.T) {
	consumer, client := newLagTestConsumer(t)
	ctx := context.Background()

	for _, stream := range consumer.streams {
		if err := consumer.ensureGroup(ctx, stream); err != nil {
			t.Fatalf("ensure group on %s: %v", stream, err)
		}
		for i := 0; i < 2; i++ {
			if err := client.XAdd(ctx, &redis.XAddArgs{
				Stream: stream,
				Values: map[string]any{"type": "post.created"},
			}).Err(); err != nil {
				t.Fatalf("add event %d to %s: %v", i, stream, err)
			}
		}
	}

	recorder := &recordingLag{}
	consumer.SetMetrics(recorder)
	for _, stream := range consumer.streams {
		consumer.reportLag(ctx, stream)
	}

	if len(recorder.samples) != 2 {
		t.Fatalf("samples = %d, want one per stream", len(recorder.samples))
	}
	seen := make(map[string]int64, len(recorder.samples))
	for _, sample := range recorder.samples {
		if sample.group != "notification-service" {
			t.Fatalf("group = %q, want notification-service", sample.group)
		}
		seen[sample.stream] = sample.lag
	}
	for _, stream := range consumer.streams {
		// The group was created at 0-0 and nothing has been read, so both
		// entries are undelivered.
		if lag, ok := seen[stream]; !ok || lag != 2 {
			t.Fatalf("lag[%s] = %d (present=%v), want 2", stream, lag, ok)
		}
	}
}

func TestReportLagSkipsForeignGroupsAndMissingRecorder(t *testing.T) {
	consumer, client := newLagTestConsumer(t)
	ctx := context.Background()

	if err := consumer.ensureGroup(ctx, "social:events"); err != nil {
		t.Fatalf("ensure group: %v", err)
	}
	if err := client.XAdd(ctx, &redis.XAddArgs{
		Stream: "social:events",
		Values: map[string]any{"type": "post.created"},
	}).Err(); err != nil {
		t.Fatalf("add event: %v", err)
	}

	// Without a recorder the lookup is skipped entirely and must not panic.
	consumer.reportLag(ctx, "social:events")

	recorder := &recordingLag{}
	consumer.SetMetrics(recorder)

	// A consumer must never publish another service's backlog.
	consumer.group = "feed-service"
	consumer.reportLag(ctx, "social:events")
	if len(recorder.samples) != 0 {
		t.Fatalf("samples = %v, want none for a foreign group", recorder.samples)
	}
}
