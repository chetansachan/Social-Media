package events

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/chetansachan/socialflow/feed-service/internal/models"
	"github.com/redis/go-redis/v9"
)

// recordingHandler is a Handler double. failFor makes the first n calls fail,
// which is how the tests simulate a downstream service being unavailable.
type recordingHandler struct {
	events  []models.Event
	failFor int
}

func (h *recordingHandler) HandleEvent(ctx context.Context, event models.Event) error {
	h.events = append(h.events, event)
	if len(h.events) <= h.failFor {
		return errors.New("downstream fanout unavailable")
	}
	return nil
}

// seen reports every decoded event handed to the handler, in order.
func (h *recordingHandler) seen() []models.Event {
	return h.events
}

// newHandlerConsumer wires a handler double into a consumer group backed by a
// throwaway miniredis instance. The server is returned so a test can move the
// clock instead of sleeping for the reclaim interval.
func newHandlerConsumer(t *testing.T, handler Handler) (*Consumer, *redis.Client, *miniredis.Miniredis) {
	t.Helper()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return NewConsumer(client, handler, "social:events", "feed-service", "feed-1"), client, server
}

// addPendingMessage writes one entry and then reads it as the group's consumer,
// returning a literal XMessage for the delivered ID. Delivering it first is what
// puts the entry in the Pending Entries List, which is how the tests observe
// acknowledgement.
func addPendingMessage(t *testing.T, client *redis.Client, stream, group, consumer string, values map[string]interface{}) redis.XMessage {
	t.Helper()

	ctx := context.Background()
	id, err := client.XAdd(ctx, &redis.XAddArgs{Stream: stream, Values: values}).Result()
	if err != nil {
		t.Fatalf("add event: %v", err)
	}
	if _, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: consumer,
		Streams:  []string{stream, ">"},
		Count:    10,
	}).Result(); err != nil {
		t.Fatalf("read group: %v", err)
	}

	return redis.XMessage{ID: id, Values: values}
}

// pendingCount is the length of the group's Pending Entries List: the entries
// that were delivered but never acknowledged. miniredis implements the XPENDING
// summary form, so an unacked message is directly observable.
func pendingCount(t *testing.T, client *redis.Client, stream, group string) int64 {
	t.Helper()

	info, err := client.XPending(context.Background(), stream, group).Result()
	if err != nil {
		t.Fatalf("pending list: %v", err)
	}
	return info.Count
}

// quietConsumerLog stops the package-level logger from spraying test output for
// the warnings these tests deliberately provoke.
func quietConsumerLog(t *testing.T) {
	t.Helper()

	restore := log.Writer()
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(restore) })
}

func TestHandleMessageDeliversDecodedEvent(t *testing.T) {
	quietConsumerLog(t)

	handler := &recordingHandler{}
	consumer, client, _ := newHandlerConsumer(t, handler)
	ctx := context.Background()

	if err := consumer.ensureGroup(ctx); err != nil {
		t.Fatalf("ensure group: %v", err)
	}
	message := addPendingMessage(t, client, consumer.stream, consumer.group, consumer.consumer, map[string]interface{}{
		"type":    "post.created",
		"payload": `{"post_id":"post-1","author_id":"author-1"}`,
	})
	if got := pendingCount(t, client, consumer.stream, consumer.group); got != 1 {
		t.Fatalf("pending = %d, want 1 before the consumer handles anything", got)
	}

	consumer.handleMessage(ctx, message)

	seen := handler.seen()
	if len(seen) != 1 {
		t.Fatalf("handler calls = %d, want 1", len(seen))
	}
	// The handler only ever sees the domain envelope, never the Redis message.
	if seen[0].Type != models.EventPostCreated {
		t.Errorf("type = %q, want %q", seen[0].Type, models.EventPostCreated)
	}
	if string(seen[0].Payload) != `{"post_id":"post-1","author_id":"author-1"}` {
		t.Errorf("payload = %s", seen[0].Payload)
	}
	// A handled event is acknowledged, so the Pending Entries List drains
	// instead of growing one entry per message forever.
	if got := pendingCount(t, client, consumer.stream, consumer.group); got != 0 {
		t.Fatalf("pending = %d after a successful handle, want 0", got)
	}
}

func TestHandleMessageAcceptsNestedEnvelope(t *testing.T) {
	quietConsumerLog(t)

	handler := &recordingHandler{}
	consumer, client, _ := newHandlerConsumer(t, handler)
	ctx := context.Background()

	if err := consumer.ensureGroup(ctx); err != nil {
		t.Fatalf("ensure group: %v", err)
	}
	// Producers that XADD a single JSON envelope instead of flat fields must
	// reach the same handler path.
	message := addPendingMessage(t, client, consumer.stream, consumer.group, consumer.consumer, map[string]interface{}{
		"event": `{"type":"post.deleted","payload":{"post_id":"post-1"}}`,
	})

	consumer.handleMessage(ctx, message)

	seen := handler.seen()
	if len(seen) != 1 {
		t.Fatalf("handler calls = %d, want 1", len(seen))
	}
	if seen[0].Type != models.EventPostDeleted {
		t.Errorf("type = %q, want %q", seen[0].Type, models.EventPostDeleted)
	}
	if got := pendingCount(t, client, consumer.stream, consumer.group); got != 0 {
		t.Fatalf("pending = %d after a successful handle, want 0", got)
	}
}

func TestHandleMessageAcksMalformedPayload(t *testing.T) {
	quietConsumerLog(t)

	handler := &recordingHandler{}
	consumer, client, _ := newHandlerConsumer(t, handler)
	ctx := context.Background()

	if err := consumer.ensureGroup(ctx); err != nil {
		t.Fatalf("ensure group: %v", err)
	}
	// A producer dropped the payload field, so this entry can never be decoded
	// into an event, no matter how often it is retried.
	message := addPendingMessage(t, client, consumer.stream, consumer.group, consumer.consumer, map[string]interface{}{
		"type": "post.created",
	})

	consumer.handleMessage(ctx, message)

	if len(handler.seen()) != 0 {
		t.Fatalf("handler calls = %d, want 0 for an undecodable message", len(handler.seen()))
	}
	// A malformed payload must be acked so a poison pill cannot block the
	// stream forever.
	if got := pendingCount(t, client, consumer.stream, consumer.group); got != 0 {
		t.Fatalf("pending = %d after a malformed message, want 0", got)
	}
}

func TestHandleMessageAcksMalformedEnvelope(t *testing.T) {
	quietConsumerLog(t)

	handler := &recordingHandler{}
	consumer, client, _ := newHandlerConsumer(t, handler)
	ctx := context.Background()

	if err := consumer.ensureGroup(ctx); err != nil {
		t.Fatalf("ensure group: %v", err)
	}
	// A truncated envelope is written by a crashing producer.
	message := addPendingMessage(t, client, consumer.stream, consumer.group, consumer.consumer, map[string]interface{}{
		"event": `{"type":"post.created"`,
	})

	consumer.handleMessage(ctx, message)

	if len(handler.seen()) != 0 {
		t.Fatalf("handler calls = %d, want 0 for a truncated envelope", len(handler.seen()))
	}
	if got := pendingCount(t, client, consumer.stream, consumer.group); got != 0 {
		t.Fatalf("pending = %d after a truncated envelope, want 0", got)
	}
}

func TestHandleMessageLeavesRejectedEventPending(t *testing.T) {
	quietConsumerLog(t)

	handler := &recordingHandler{failFor: 1}
	consumer, client, _ := newHandlerConsumer(t, handler)
	ctx := context.Background()

	if err := consumer.ensureGroup(ctx); err != nil {
		t.Fatalf("ensure group: %v", err)
	}
	message := addPendingMessage(t, client, consumer.stream, consumer.group, consumer.consumer, map[string]interface{}{
		"type":    "post.created",
		"payload": `{"post_id":"post-1"}`,
	})

	consumer.handleMessage(ctx, message)

	if len(handler.seen()) != 1 {
		t.Fatalf("handler calls = %d, want 1", len(handler.seen()))
	}
	// A rejected event stays pending so the reclaim loop can retry it once the
	// downstream dependency recovers. Acking it here would drop the post.
	if got := pendingCount(t, client, consumer.stream, consumer.group); got != 1 {
		t.Fatalf("pending = %d after a failed handle, want 1", got)
	}
}

func TestRunStopsWhenContextIsCancelled(t *testing.T) {
	quietConsumerLog(t)

	handler := &recordingHandler{}
	consumer, client, _ := newHandlerConsumer(t, handler)

	ctx := context.Background()
	if err := client.XAdd(ctx, &redis.XAddArgs{
		Stream: consumer.stream,
		Values: map[string]any{"type": "post.created", "payload": `{"post_id":"post-1"}`},
	}).Err(); err != nil {
		t.Fatalf("add event: %v", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- consumer.Run(runCtx) }()

	select {
	case err := <-done:
		// A cancelled context is a clean shutdown, not an error.
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after the context was cancelled")
	}

	// The one entry in the stream is delivered before the context expires.
	if len(handler.seen()) != 1 {
		t.Fatalf("handler calls = %d, want 1", len(handler.seen()))
	}
	if got := pendingCount(t, client, consumer.stream, consumer.group); got != 0 {
		t.Fatalf("pending = %d after Run, want 0", got)
	}
}

func TestReclaimPendingRedeliversIdleMessage(t *testing.T) {
	quietConsumerLog(t)

	handler := &recordingHandler{failFor: 1}
	consumer, client, server := newHandlerConsumer(t, handler)
	ctx := context.Background()

	// Pin miniredis to the wall clock so pending-entry idle time is measurable.
	server.SetTime(time.Now())

	if err := consumer.ensureGroup(ctx); err != nil {
		t.Fatalf("ensure group: %v", err)
	}
	message := addPendingMessage(t, client, consumer.stream, consumer.group, consumer.consumer, map[string]interface{}{
		"type":    "post.created",
		"payload": `{"post_id":"post-1"}`,
	})

	// The first delivery fails, so the entry stays pending and idle.
	consumer.handleMessage(ctx, message)
	if got := pendingCount(t, client, consumer.stream, consumer.group); got != 1 {
		t.Fatalf("pending = %d, want 1 while the entry is unacknowledged", got)
	}

	// Move the clock past the 30s reclaim interval instead of sleeping.
	server.SetTime(time.Now().Add(31 * time.Second))

	consumer.reclaimPending(ctx)

	// The idle entry is claimed by this consumer again and redelivered to the
	// handler, which now succeeds, so the group drains.
	seen := handler.seen()
	if len(seen) != 2 {
		t.Fatalf("handler calls = %d, want 2 (one delivery plus one reclaim)", len(seen))
	}
	if seen[1].Type != models.EventPostCreated {
		t.Errorf("redelivered type = %q, want %q", seen[1].Type, models.EventPostCreated)
	}
	if got := pendingCount(t, client, consumer.stream, consumer.group); got != 0 {
		t.Fatalf("pending = %d after the reclaim, want 0", got)
	}
}
