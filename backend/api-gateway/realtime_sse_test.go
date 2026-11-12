package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/chetansachan/socialflow/api-gateway/auth"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

/* ---- The realtime stream -------------------------------------------------
   The handler is the only part of the SSE path that runs in Go. The Next.js
   proxy in front of it needs a running server, so this test proves the half
   that is Go's to get wrong: that a frame published for a user reaches that
   user's stream, in the shape the browser consumes, and nobody else's. */

// streamCapture is a ResponseWriter that buffers writes and parses SSE events,
// so a streaming handler can be read while it runs rather than after it returns
// -- which it never does, because it serves until the client disconnects.
type streamCapture struct {
	header http.Header
	status int

	mu      sync.Mutex
	buffer  strings.Builder
	written chan struct{}
}

func newStreamCapture() *streamCapture {
	return &streamCapture{
		header:  make(http.Header),
		written: make(chan struct{}, 64),
	}
}

func (c *streamCapture) Header() http.Header { return c.header }

func (c *streamCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	c.buffer.Write(p)
	c.mu.Unlock()
	// A signal, not the payload: the payload stays in the buffer so nothing is
	// consumed by whoever happens to read first.
	select {
	case c.written <- struct{}{}:
	default:
	}
	return len(p), nil
}

func (c *streamCapture) WriteHeader(status int) { c.status = status }

func (c *streamCapture) Flush() {}

// buffered returns everything written so far.
func (c *streamCapture) buffered() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buffer.String()
}

/**
 * nextBlock returns the next blank-line-terminated block, comment or not.
 *
 * This is the readiness barrier: the handler's opening comment proves its Redis
 * subscription is live, so anything published after this returns is received.
 */
func (c *streamCapture) nextBlock(t *testing.T, within time.Duration) string {
	t.Helper()
	deadline := time.After(within)

	for {
		if block := c.takeBlock(); block != "" {
			return block
		}
		select {
		case <-c.written:
		case <-deadline:
			t.Fatalf("no block within %s; got %q", within, c.buffered())
		}
	}
}

/**
 * nextEvent returns the next SSE event, skipping comments.
 *
 * SSE events are terminated by a blank line. A line beginning with ':' is a
 * comment -- the keep-alive this handler sends every 15 seconds -- which a real
 * EventSource ignores, so this reader must too, or a test would see the
 * keep-alive and mistake it for a frame.
 */
func (c *streamCapture) nextEvent(t *testing.T, within time.Duration) string {
	t.Helper()
	deadline := time.After(within)

	for {
		if event := c.takeEvent(); event != "" {
			return event
		}
		select {
		case <-c.written:
		case <-deadline:
			t.Fatalf("no event within %s; got %q", within, c.buffered())
		}
	}
}

// takeBlock pops the first complete block and consumes it, whatever it holds.
func (c *streamCapture) takeBlock() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	block, rest, ok := cutEvent(c.buffer.String())
	if !ok {
		return ""
	}
	c.buffer.Reset()
	c.buffer.WriteString(rest)
	return block
}

// takeEvent pops the first complete block that carries a field, discarding
// comments on the way as a browser would.
func (c *streamCapture) takeEvent() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	text := c.buffer.String()
	for {
		block, rest, ok := cutEvent(text)
		if !ok {
			return ""
		}
		text = rest
		if hasField(block) {
			c.buffer.Reset()
			c.buffer.WriteString(text)
			return block
		}
	}
}

// cutEvent splits off the first blank-line-terminated block.
func cutEvent(text string) (block, rest string, ok bool) {
	for _, terminator := range []string{"\r\n\r\n", "\n\n", "\r\r"} {
		if i := strings.Index(text, terminator); i != -1 {
			return text[:i], text[i+len(terminator):], true
		}
	}
	return "", text, false
}

// hasField reports whether a block declares an event or data field, which is
// what separates a real frame from a comment.
func hasField(block string) bool {
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") || strings.HasPrefix(line, "data:") {
			return true
		}
	}
	return false
}

// streamToken issues a token the gateway's own middleware will accept. Signing
// it here rather than reaching for a fixture keeps this test honest about the
// token shape the middleware requires: HS256, a non-empty user_id, and exp.
func streamToken(t *testing.T, userID string) string {
	t.Helper()
	claims := &auth.Claims{
		UserID: userID,
		Email:  userID + "@example.test",
		Role:   "user",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(streamTestSecret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

const streamTestSecret = "test-secret"

/**
 * openStream starts the authenticated handler and waits for its opening
 * comment.
 *
 * Waiting matters: the handler subscribes to Redis after writing that comment,
 * and a frame published before the subscription is confirmed is dropped by
 * Redis rather than queued. Returning before it lands would make every test
 * below a coin flip.
 */
func openStream(
	t *testing.T,
	redisClient *redis.Client,
	userID string,
) (*streamCapture, context.CancelFunc) {
	t.Helper()

	handler := auth.Middleware(streamTestSecret, realtimeStreamHandler(redisClient))
	request := httptest.NewRequest(http.MethodGet, "/stream", nil)
	request.Header.Set("Authorization", "Bearer "+streamToken(t, userID))

	ctx, cancel := context.WithCancel(request.Context())
	capture := newStreamCapture()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(capture, request.WithContext(ctx))
	}()

	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("stream handler did not return after the client disconnected")
		}
	})

	// The opening comment proves the Redis subscription is confirmed, so
	// anything published after this returns is received rather than dropped.
	capture.nextBlock(t, 3*time.Second)
	return capture, cancel
}

func TestRealtimeStreamRelaysNotificationsAndChat(t *testing.T) {
	server, client := newTestRedis(t)
	userID := newUUID(t)
	otherID := newUUID(t)

	capture, _ := openStream(t, client, userID)

	// A frame for somebody else must never reach this stream. It is published
	// first so a leak would surface as the very first event read below.
	server.Publish(streamChannelNotification+otherID, `{"event":"notification_received","data":{"id":"nope","title":"Someone else"}}`)
	server.Publish(streamChannelChat+otherID, `{"type":"message.created","data":{"content":"not yours"}}`)

	server.Publish(streamChannelNotification+userID, `{"event":"notification_received","data":{"id":"n1","type":"post_like","title":"New like","action_url":"/posts/p1","total_actors_count":1,"unread_count":4,"created_at":"2026-01-01T00:00:00Z"}}`)
	server.Publish(streamChannelChat+userID, `{"type":"message.created","data":{"conversation_id":"c1","message_id":"m1","sender_id":"`+otherID+`","content":"hello"}}`)

	notification := capture.nextEvent(t, 3*time.Second)
	chat := capture.nextEvent(t, 3*time.Second)

	if !strings.Contains(notification, "event: notifications") {
		t.Fatalf("notification frame = %q", notification)
	}
	// The backend marshals this as `unread_count`, and the gateway relays the
	// payload verbatim. This assertion is what keeps the client from reading a
	// camelCase name that never arrives.
	if !strings.Contains(notification, `"unread_count":4`) {
		t.Fatalf("the authoritative bell count did not ride along: %q", notification)
	}

	if !strings.Contains(chat, "event: chat") {
		t.Fatalf("chat frame = %q", chat)
	}
	if !strings.Contains(chat, `"message.created"`) {
		t.Fatalf("chat frame lost its type: %q", chat)
	}
	if !strings.Contains(chat, `"content":"hello"`) {
		t.Fatalf("chat frame lost its payload: %q", chat)
	}
	if !strings.Contains(chat, `"conversation_id":"c1"`) {
		t.Fatalf("chat frame lost its snake_case keys: %q", chat)
	}

	// Nothing from the other user may have appeared: the two events read above
	// came from this user's channels, and a leak would have arrived first.
	whole := capture.buffered()
	if strings.Contains(whole, "Someone else") || strings.Contains(whole, "not yours") {
		t.Fatalf("another user's frame leaked into this stream: %q", whole)
	}

	header := capture.header
	if got := header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := header.Get("Cache-Control"); got != "no-cache, no-transform" {
		t.Errorf("Cache-Control = %q", got)
	}
	// Without this a proxy buffers the frames into bursts.
	if got := header.Get("X-Accel-Buffering"); got != "no" {
		t.Errorf("X-Accel-Buffering = %q", got)
	}
}

func TestRealtimeStreamRejectsAnonymous(t *testing.T) {
	_, client := newTestRedis(t)

	request := httptest.NewRequest(http.MethodGet, "/stream", nil)
	recorder := httptest.NewRecorder()

	handler := auth.Middleware(streamTestSecret, realtimeStreamHandler(client))
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
}

// A malformed frame must not tear the stream down: the next one carries the
// same state, so dropping the bad frame is strictly better than hanging up and
// making the client reconnect through the whole handshake again.
func TestRealtimeStreamSurvivesAMalformedFrame(t *testing.T) {
	server, client := newTestRedis(t)
	userID := newUUID(t)

	capture, _ := openStream(t, client, userID)

	server.Publish(streamChannelNotification+userID, `this is not json`)
	server.Publish(streamChannelNotification+userID, `{"event":"notification_received","data":{"id":"n2","title":"After the bad one","unread_count":1,"total_actors_count":1,"action_url":"/posts/p2","created_at":"2026-01-01T00:00:00Z"}}`)

	event := capture.nextEvent(t, 3*time.Second)
	if !strings.Contains(event, "event: notifications") {
		t.Fatalf("event = %q", event)
	}
	if !strings.Contains(event, "After the bad one") {
		t.Fatalf("the frame after the malformed one was dropped: %q", event)
	}
}

// A subject that is not a UUID is a configuration error, not a caller error,
// but it must not be subscribed to a channel nobody ever publishes to -- that
// is a silent hole the client would experience as "notifications never arrive".
func TestRealtimeStreamRejectsANonUUIDSubject(t *testing.T) {
	_, client := newTestRedis(t)

	handler := auth.Middleware(streamTestSecret, realtimeStreamHandler(client))
	request := httptest.NewRequest(http.MethodGet, "/stream", nil)
	request.Header.Set("Authorization", "Bearer "+streamToken(t, "not-a-uuid"))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
}

// newTestRedis mirrors graph.subscription_test.go's helper. It is duplicated
// rather than exported because reaching across a package boundary it has no
// business crossing would make the graph package own the root package's tests.
func newTestRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return server, client
}

func newUUID(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewRandom()
	if err != nil {
		t.Fatalf("uuid: %v", err)
	}
	return id.String()
}
