package realtime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func newTestPublisher(t *testing.T) (*Publisher, *redis.Client) {
	t.Helper()
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(server.Close)

	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return NewPublisher(client, ""), client
}

// streamLength reads the durable copy through the client rather than the fake's
// internals, so the assertion uses the same command path the publisher does.
func streamLength(t *testing.T, client *redis.Client, stream string) int64 {
	t.Helper()
	length, err := client.XLen(context.Background(), stream).Result()
	if err != nil {
		t.Fatalf("XLen %s: %v", stream, err)
	}
	return length
}

// subscribe drains one channel until the first message arrives.
func subscribe(t *testing.T, client *redis.Client, channel string) *redis.PubSub {
	t.Helper()
	pubsub := client.Subscribe(context.Background(), channel)
	if _, err := pubsub.Receive(context.Background()); err != nil {
		t.Fatalf("subscribe to %s: %v", channel, err)
	}
	t.Cleanup(func() { _ = pubsub.Close() })
	return pubsub
}

func TestUserChannelIsPerMember(t *testing.T) {
	first := userChannel(uuid.New())
	second := userChannel(uuid.New())

	if first == second {
		t.Fatal("two members must not share a channel")
	}
	if first[:5] != "chat:" {
		t.Fatalf("channel = %q, want the chat: prefix the gateway subscribes to", first)
	}
}

func TestPublishMessageCreatedReachesEveryMemberAndTheStream(t *testing.T) {
	publisher, client := newTestPublisher(t)

	alice, bob := uuid.New(), uuid.New()
	aliceChannel := subscribe(t, client, userChannel(alice))
	bobChannel := subscribe(t, client, userChannel(bob))

	if err := publisher.PublishMessageCreated(context.Background(), []uuid.UUID{alice, bob}, map[string]any{
		"conversation_id": uuid.New().String(),
		"message_id":      uuid.New().String(),
		"sender_id":       alice.String(),
		"content":         "hello",
	}); err != nil {
		t.Fatalf("PublishMessageCreated: %v", err)
	}

	for name, pubsub := range map[string]*redis.PubSub{"alice": aliceChannel, "bob": bobChannel} {
		message, err := receiveFrame(pubsub)
		if err != nil {
			t.Fatalf("%s did not receive the frame: %v", name, err)
		}
		if message.Type != EventMessageCreated {
			t.Fatalf("%s frame type = %q, want %q", name, message.Type, EventMessageCreated)
		}

		var payload struct {
			Content string `json:"content"`
		}
		if err := json.Unmarshal(message.Data, &payload); err != nil {
			t.Fatalf("%s payload is not valid JSON: %v", name, err)
		}
		if payload.Content != "hello" {
			t.Fatalf("%s content = %q, want the inlined body", name, payload.Content)
		}
	}

	// The stream copy is what lets a reconnecting client catch up.
	if length := streamLength(t, client, "social:events"); length != 1 {
		t.Fatalf("stream length = %d, want one mirrored frame", length)
	}
}

func TestPublishTypingAndReadCarryTheirType(t *testing.T) {
	publisher, client := newTestPublisher(t)
	alice, bob := uuid.New(), uuid.New()
	conversation := uuid.New()

	bobChannel := subscribe(t, client, userChannel(bob))

	if err := publisher.PublishTyping(context.Background(), []uuid.UUID{bob}, conversation, alice, true); err != nil {
		t.Fatalf("PublishTyping: %v", err)
	}
	frame, err := receiveFrame(bobChannel)
	if err != nil {
		t.Fatalf("receive typing frame: %v", err)
	}
	if frame.Type != EventTyping {
		t.Fatalf("frame type = %q, want %q", frame.Type, EventTyping)
	}
	var typing struct {
		IsTyping bool `json:"is_typing"`
	}
	if err := json.Unmarshal(frame.Data, &typing); err != nil {
		t.Fatalf("typing payload: %v", err)
	}
	if !typing.IsTyping {
		t.Fatal("is_typing should survive the round trip")
	}

	if err := publisher.PublishRead(context.Background(), []uuid.UUID{bob}, conversation, alice); err != nil {
		t.Fatalf("PublishRead: %v", err)
	}
	frame, err = receiveFrame(bobChannel)
	if err != nil {
		t.Fatalf("receive read frame: %v", err)
	}
	if frame.Type != EventRead {
		t.Fatalf("frame type = %q, want %q", frame.Type, EventRead)
	}
}

func TestPublishMessageDeletedOnlyNamesTheID(t *testing.T) {
	publisher, _ := newTestPublisher(t)
	alice, bob := uuid.New(), uuid.New()
	conversation := uuid.New()
	message := uuid.New()

	bobChannel := subscribe(t, publisher.client, userChannel(bob))

	if err := publisher.PublishMessageDeleted(context.Background(), []uuid.UUID{bob}, conversation, message); err != nil {
		t.Fatalf("PublishMessageDeleted: %v", err)
	}

	frame, err := receiveFrame(bobChannel)
	if err != nil {
		t.Fatalf("receive deletion frame: %v", err)
	}
	var payload struct {
		MessageID string `json:"message_id"`
		Content   string `json:"content"`
	}
	if err := json.Unmarshal(frame.Data, &payload); err != nil {
		t.Fatalf("deletion payload: %v", err)
	}
	if payload.MessageID != message.String() {
		t.Fatalf("message_id = %q, want the deleted id", payload.MessageID)
	}
	if payload.Content != "" {
		t.Fatal("a deletion frame must not carry the retracted content")
	}
	_ = alice
}

func TestPublishSurfacesEncodingFailures(t *testing.T) {
	publisher, _ := newTestPublisher(t)

	// A channel is not JSON-encodable, so the payload marshal fails before any
	// publish happens rather than half-sending a frame.
	if err := publisher.PublishToUsers(context.Background(), []uuid.UUID{uuid.New()}, EventTyping, map[string]any{
		"bad": make(chan int),
	}); err == nil {
		t.Fatal("expected an encoding failure")
	}
}

func TestPublishUsesTheConfiguredStream(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(server.Close)

	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	publisher := NewPublisher(client, "social:events:chat")

	if err := publisher.PublishRead(context.Background(), []uuid.UUID{uuid.New()}, uuid.New(), uuid.New()); err != nil {
		t.Fatalf("PublishRead: %v", err)
	}
	if length := streamLength(t, client, "social:events:chat"); length != 1 {
		t.Fatalf("stream length = %d, want one frame on the configured stream", length)
	}

	// An unset stream name must fall back to the shared bus rather than publish
	// nowhere.
	fallback := NewPublisher(client, "   ")
	if err := fallback.PublishRead(context.Background(), []uuid.UUID{uuid.New()}, uuid.New(), uuid.New()); err != nil {
		t.Fatalf("PublishRead on the fallback: %v", err)
	}
	if length := streamLength(t, client, "social:events"); length != 1 {
		t.Fatalf("stream length = %d, want one frame on the shared bus", length)
	}
}

func receiveFrame(pubsub *redis.PubSub) (Envelope, error) {
	message, err := pubsub.ReceiveMessage(context.Background())
	if err != nil {
		return Envelope{}, err
	}

	var frame Envelope
	if err := json.Unmarshal([]byte(message.Payload), &frame); err != nil {
		return Envelope{}, err
	}
	return frame, nil
}
