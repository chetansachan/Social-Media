package dispatchers

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/alicebob/miniredis/v2"
	"github.com/chetansachan/socialflow/notification-service/internal/models"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// -----------------------------------------------------------------------------
// Test doubles
// -----------------------------------------------------------------------------

// fakeDispatchRecorder captures the (channel, status) pairs the dispatchers
// hand to observability.Metrics so a test can prove which branch ran without
// pulling in the platform module.
type fakeDispatchRecorder struct {
	mu    sync.Mutex
	calls []dispatchCall
}

type dispatchCall struct {
	channel string
	status  string
}

func (f *fakeDispatchRecorder) RecordDispatch(channel, status string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, dispatchCall{channel: channel, status: status})
}

func (f *fakeDispatchRecorder) snapshot() []dispatchCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]dispatchCall(nil), f.calls...)
}

// newRealtimeTestPair wires a real RealtimeDispatcher against a throwaway
// miniredis, so the Redis Pub/Sub path is exercised for real but with nothing
// leaving this process.
func newRealtimeTestPair(t *testing.T) (*RealtimeDispatcher, *redis.Client, *fakeDispatchRecorder) {
	t.Helper()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	recorder := &fakeDispatchRecorder{}
	dispatcher := NewRealtimeDispatcher(client, log.New(io.Discard, "", 0))
	dispatcher.SetMetrics(recorder)

	return dispatcher, client, recorder
}

// -----------------------------------------------------------------------------
// RealtimeDispatcher.Publish
// -----------------------------------------------------------------------------

func TestRealtimePublishDeliversFrameOnUserChannel(t *testing.T) {
	t.Parallel()

	dispatcher, client, recorder := newRealtimeTestPair(t)
	ctx := context.Background()
	userID := uuid.New()
	channel := "notify:" + userID.String()

	// The WebSocket gateway holds exactly this subscription. Receive() blocks
	// until Redis confirms the SUBSCRIBE, which is what makes the assertion
	// below deterministic instead of a race between the goroutines.
	pubsub := client.Subscribe(ctx, channel)
	t.Cleanup(func() { _ = pubsub.Close() })
	if _, err := pubsub.Receive(ctx); err != nil {
		t.Fatalf("subscribe to %s: %v", channel, err)
	}
	frames := pubsub.Channel()

	payload := models.RealtimePayload{
		Event: "notification_received",
		Data: models.RealtimeData{
			ID:               uuid.NewString(),
			Type:             models.TypePostLike,
			Title:            "Alice liked your post",
			ActionURL:        "socialflow://posts/42",
			TotalActorsCount: 1,
			UnreadCount:      7,
			CreatedAt:        "2026-01-02T03:04:05Z",
			Actors:           []models.Actor{{ID: uuid.NewString(), Username: "alice"}},
		},
	}
	if err := dispatcher.Publish(ctx, userID.String(), payload); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case frame := <-frames:
		if frame.Channel != channel {
			t.Errorf("channel = %q, want %q: the gateway keys off the recipient", frame.Channel, channel)
		}

		// The gateway parses the raw frame, so assert the wire shape as well as
		// the decoded struct.
		if !strings.Contains(frame.Payload, `"event":"notification_received"`) {
			t.Errorf("frame = %q, want the client protocol event field", frame.Payload)
		}

		var got models.RealtimePayload
		if err := json.Unmarshal([]byte(frame.Payload), &got); err != nil {
			t.Fatalf("decode frame %q: %v", frame.Payload, err)
		}
		if got.Event != payload.Event {
			t.Errorf("event = %q, want %q", got.Event, payload.Event)
		}
		if got.Data.ID != payload.Data.ID {
			t.Errorf("data.id = %q, want %q", got.Data.ID, payload.Data.ID)
		}
		if got.Data.Type != payload.Data.Type {
			t.Errorf("data.type = %q, want %q", got.Data.Type, payload.Data.Type)
		}
		if got.Data.Title != payload.Data.Title {
			t.Errorf("data.title = %q, want %q", got.Data.Title, payload.Data.Title)
		}
		if got.Data.ActionURL != payload.Data.ActionURL {
			t.Errorf("data.action_url = %q, want %q", got.Data.ActionURL, payload.Data.ActionURL)
		}
		if got.Data.UnreadCount != payload.Data.UnreadCount {
			t.Errorf("data.unread_count = %d, want %d", got.Data.UnreadCount, payload.Data.UnreadCount)
		}
		if got.Data.CreatedAt != payload.Data.CreatedAt {
			t.Errorf("data.created_at = %q, want %q", got.Data.CreatedAt, payload.Data.CreatedAt)
		}
		if len(got.Data.Actors) != 1 || got.Data.Actors[0].Username != "alice" {
			t.Errorf("data.actors = %+v, want the hydrated actor list", got.Data.Actors)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no realtime frame arrived on notify:<user_id>")
	}

	// Publish is best-effort: the inbox row is already committed, so the happy
	// path must both report success and return nil to the caller.
	if calls := recorder.snapshot(); len(calls) != 1 || calls[0] != (dispatchCall{channel: "realtime", status: "success"}) {
		t.Errorf("dispatch calls = %+v, want [{realtime success}]", calls)
	}
}

func TestRealtimePublishRecordsErrorWhenRedisUnreachable(t *testing.T) {
	t.Parallel()

	server := miniredis.RunT(t)
	// Retries are disabled here: the point is that the metrics branch records
	// the failure, and the default backoff would only make the test slow.
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })

	// Take the server down before publishing so the failure is a plain
	// connection error rather than anything an operator has to reproduce.
	server.Close()

	recorder := &fakeDispatchRecorder{}
	dispatcher := NewRealtimeDispatcher(client, log.New(io.Discard, "", 0))
	dispatcher.SetMetrics(recorder)

	err := dispatcher.Publish(context.Background(), uuid.NewString(), models.RealtimePayload{Event: "notification_received"})
	if err == nil {
		t.Fatal("publish to a stopped Redis should report the failure")
	}
	if calls := recorder.snapshot(); len(calls) != 1 || calls[0] != (dispatchCall{channel: "realtime", status: "error"}) {
		t.Errorf("dispatch calls = %+v, want [{realtime error}]", calls)
	}
}

// -----------------------------------------------------------------------------
// RealtimeDispatcher.SetMetrics
// -----------------------------------------------------------------------------

func TestRealtimeSetMetricsNilRecorderDoesNotPanic(t *testing.T) {
	t.Parallel()

	dispatcher, _, _ := newRealtimeTestPair(t)
	ctx := context.Background()

	// SetMetrics(nil) documents how metrics get switched off. Publish must then
	// take the disabled branch instead of dereferencing the nil recorder.
	dispatcher.SetMetrics(nil)

	if err := dispatcher.Publish(ctx, uuid.NewString(), models.RealtimePayload{Event: "notification_received"}); err != nil {
		t.Fatalf("publish with metrics disabled: %v", err)
	}
}

func TestRealtimeSetMetricsOnNilDispatcher(t *testing.T) {
	t.Parallel()

	// The dispatcher is used as a value receiver guard in Publish as well, so
	// SetMetrics has to tolerate being called on a nil dispatcher.
	var dispatcher *RealtimeDispatcher
	dispatcher.SetMetrics(&fakeDispatchRecorder{})
}

func TestRealtimeSetMetricsReplacesRecorder(t *testing.T) {
	t.Parallel()

	dispatcher, _, _ := newRealtimeTestPair(t)
	ctx := context.Background()

	wired := &fakeDispatchRecorder{}
	dispatcher.SetMetrics(wired)

	if err := dispatcher.Publish(ctx, uuid.NewString(), models.RealtimePayload{Event: "notification_received"}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if calls := wired.snapshot(); len(calls) != 1 || calls[0] != (dispatchCall{channel: "realtime", status: "success"}) {
		t.Errorf("dispatch calls = %+v, want [{realtime success}] on the newly wired recorder", calls)
	}
}

// -----------------------------------------------------------------------------
// recordDispatch
// -----------------------------------------------------------------------------

func TestRecordDispatchForwardsChannelAndStatus(t *testing.T) {
	t.Parallel()

	recorder := &fakeDispatchRecorder{}
	recordDispatch(recorder, "push", "success")
	recordDispatch(recorder, "email", "queue_full")

	calls := recorder.snapshot()
	want := []dispatchCall{{channel: "push", status: "success"}, {channel: "email", status: "queue_full"}}
	if len(calls) != len(want) {
		t.Fatalf("calls = %+v, want %+v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Errorf("call %d = %+v, want %+v", i, calls[i], want[i])
		}
	}
}

func TestRecordDispatchIgnoresNilRecorder(t *testing.T) {
	t.Parallel()

	// "metrics disabled" is expressed as a nil recorder, so the nil branch is
	// what keeps a deployment without Prometheus working at all.
	recordDispatch(nil, "realtime", "success")
}

// -----------------------------------------------------------------------------
// retry
// -----------------------------------------------------------------------------

func TestRetryRetriesUntilProviderRecovers(t *testing.T) {
	t.Parallel()

	calls := 0
	err := retry(context.Background(), 3, func() error {
		calls++
		if calls < 3 {
			return errors.New("provider temporarily unavailable")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("retry = %v, want the transient failure to be absorbed", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestRetryStopsAtMaxAttempts(t *testing.T) {
	t.Parallel()

	const maxAttempts = 4
	calls := 0
	sentinel := errors.New("provider still failing")

	err := retry(context.Background(), maxAttempts, func() error {
		calls++
		return sentinel
	})

	if !errors.Is(err, sentinel) {
		t.Errorf("retry = %v, want the last provider error surfaced to the caller", err)
	}
	if calls != maxAttempts {
		t.Errorf("calls = %d, want %d: the bound is what stops an unbounded retry loop", calls, maxAttempts)
	}
}

func TestRetryDoesNotRetryUnregisteredToken(t *testing.T) {
	t.Parallel()

	// A provider saying "this token is gone" will say so on every attempt, so
	// retrying just burns three round trips before deactivating the token.
	calls := 0
	err := retry(context.Background(), 5, func() error {
		calls++
		return models.ErrTokenUnregistered
	})

	if !errors.Is(err, models.ErrTokenUnregistered) {
		t.Errorf("retry = %v, want ErrTokenUnregistered", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestRetryClampsAttemptsToAtLeastOne(t *testing.T) {
	t.Parallel()

	calls := 0
	err := retry(context.Background(), 0, func() error {
		calls++
		return errors.New("boom")
	})

	if err == nil {
		t.Fatal("retry = nil, want the operation error")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1: a zero/negative budget must still run once", calls)
	}
}

// -----------------------------------------------------------------------------
// truncate
// -----------------------------------------------------------------------------

func TestTruncateIsRuneAware(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value string
		limit int
		want  string
	}{
		{
			name:  "shorter than the limit is untouched",
			value: "Alice liked your post",
			limit: 120,
			want:  "Alice liked your post",
		},
		{
			// The guard is "less than or equal", so a body that exactly fills
			// the column must not gain a spurious ellipsis.
			name:  "exactly at the limit is untouched",
			value: "12345",
			limit: 5,
			want:  "12345",
		},
		{
			name:  "overflow keeps limit-3 runes",
			value: "encyclopedia",
			limit: 10,
			want:  "encyclo...",
		},
		{
			// Slicing by bytes here would emit a broken half-rune that renders
			// as a replacement character in the client toast.
			name:  "multi-byte runes are counted once",
			value: strings.Repeat("é", 11),
			limit: 10,
			want:  strings.Repeat("é", 7) + "...",
		},
		{
			name:  "cjk runes are counted once",
			value: "日本語です",
			limit: 4,
			want:  "日...",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := truncate(tc.value, tc.limit)
			if got != tc.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tc.value, tc.limit, got, tc.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("truncate(%q, %d) = %q is not valid UTF-8", tc.value, tc.limit, got)
			}
			if utf8.RuneCountInString(got) > tc.limit {
				t.Errorf("truncate(%q, %d) = %q has %d runes, want at most %d",
					tc.value, tc.limit, got, utf8.RuneCountInString(got), tc.limit)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// providerError
// -----------------------------------------------------------------------------

func TestProviderErrorIncludesStatusAndBody(t *testing.T) {
	t.Parallel()

	body := []byte(`{"error":{"status":"INVALID_ARGUMENT","message":"bad token"}}`)

	err := providerError("FCM", http.StatusBadRequest, body)
	if err == nil {
		t.Fatal("providerError = nil, want an error")
	}

	// The upstream body is the only clue an on-call engineer has, so the
	// message has to carry the provider, the status, and the body.
	message := err.Error()
	for _, want := range []string{"FCM", "400", string(body)} {
		if !strings.Contains(message, want) {
			t.Errorf("message = %q, want it to contain %q", message, want)
		}
	}
}

func TestProviderErrorTruncatesLongBody(t *testing.T) {
	t.Parallel()

	// Upstreams happily return a multi-kilobyte error page; without the cap it
	// would end up in the log line and in the returned error.
	err := providerError("APNs", http.StatusInternalServerError, []byte(strings.Repeat("a", 4096)))

	message := err.Error()
	if !strings.HasSuffix(message, "...") {
		t.Errorf("message = %q, want the body truncated with an ellipsis", message)
	}
	const prefix = "APNs returned status 500: "
	if body := strings.TrimPrefix(message, prefix); len(body) != 300 {
		t.Errorf("body length = %d, want the 300 rune cap", len(body))
	}
}

// -----------------------------------------------------------------------------
// fcmTokenError / apnsTokenError
// -----------------------------------------------------------------------------

func TestFCMTokenError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
		want bool
	}{
		{
			// FCM reports a stale token as a status string on a 400 response,
			// which the dispatcher must treat as "deactivate this token"
			// instead of a retryable failure.
			name: "unregistered token",
			body: `{"error":{"status":"UNREGISTERED","message":"Requested entity was not found"}}`,
			want: true,
		},
		{
			name: "invalid registration token",
			body: `{"error":{"status":"INVALID_ARGUMENT","message":"The registration token is not a valid FCM registration token"}}`,
			want: true,
		},
		{
			// The helper uppercases the body because FCM mirrors the producer's
			// casing; a lowercase spelling must still be recognised.
			name: "lowercase status is still recognised",
			body: `{"error":{"status":"unregistered"}}`,
			want: true,
		},
		{
			name: "unrelated server failure",
			body: `{"error":{"status":"INTERNAL","message":"backend error"}}`,
			want: false,
		},
		{
			name: "empty body",
			body: ``,
			want: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := fcmTokenError([]byte(tc.body)); got != tc.want {
				t.Errorf("fcmTokenError(%s) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}

func TestAPNsTokenError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "bad device token",
			body: `{"reason":"BadDeviceToken"}`,
			want: true,
		},
		{
			name: "unregistered",
			body: `{"reason":"Unregistered"}`,
			want: true,
		},
		{
			name: "token issued for another topic",
			body: `{"reason":"DeviceTokenNotForTopic"}`,
			want: true,
		},
		{
			// A real APNs failure that must keep being retried: deactivating a
			// perfectly good token would silently drop push for that device.
			name: "unrelated reason",
			body: `{"reason":"BadCollapseId"}`,
			want: false,
		},
		{
			// Only the structured reason field is trusted, so a raw body that
			// merely contains the words must not deactivate a token.
			name: "unstructured body",
			body: `token unregistered`,
			want: false,
		},
		{
			name: "empty body",
			body: ``,
			want: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := apnsTokenError([]byte(tc.body)); got != tc.want {
				t.Errorf("apnsTokenError(%s) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// maskToken
// -----------------------------------------------------------------------------

func TestMaskTokenKeepsHeadAndTail(t *testing.T) {
	t.Parallel()

	token := "fcm-token-abcdef0123456789"
	got := maskToken(token)

	if got == token {
		t.Errorf("maskToken(%q) returned the token in the clear", token)
	}
	// The head and tail survive so an operator can correlate a device in
	// provider logs, while the middle never reaches a log line.
	if got != "fcm-to...6789" {
		t.Errorf("maskToken(%q) = %q, want %q", token, got, "fcm-to...6789")
	}
}

func TestMaskTokenDropsShortTokens(t *testing.T) {
	t.Parallel()

	// Masking by offset would leak almost the whole short token, so anything
	// short enough to be leaked in full is replaced with asterisks instead.
	if got := maskToken("abc123"); got != "***" {
		t.Errorf("maskToken(short) = %q, want ***", got)
	}
	// The boundary itself: twelve bytes is the largest value that is dropped.
	if got := maskToken("abcdefghijkl"); got != "***" {
		t.Errorf("maskToken(12 bytes) = %q, want ***", got)
	}
	if got := maskToken("abcdefghijklm"); got != "abcdef...jklm" {
		t.Errorf("maskToken(13 bytes) = %q, want abcdef...jklm", got)
	}
}

// -----------------------------------------------------------------------------
// sanitizeHeader
// -----------------------------------------------------------------------------

func TestSanitizeHeaderBlocksHeaderInjection(t *testing.T) {
	t.Parallel()

	// A subject is written straight into an SMTP DATA block. A CRLF that
	// survived would terminate the Subject header and let a caller inject
	// arbitrary SMTP commands (BCC, a second recipient, a second body).
	attacks := []string{
		"Welcome\r\nBCC: victim@example.com",
		"Welcome\nBCC: victim@example.com",
		"Welcome\r\n\r\nInjected body follows",
		"subject\r\nRCPT TO:<attacker@example.com>",
	}

	for _, subject := range attacks {
		got := sanitizeHeader(subject)
		if strings.ContainsAny(got, "\r\n") {
			t.Errorf("sanitizeHeader(%q) = %q, want no CR or LF", subject, got)
		}
	}
}

func TestSanitizeHeaderReplacesBreaksWithSpaces(t *testing.T) {
	t.Parallel()

	if got := sanitizeHeader("line one\r\nline two"); got != "line one  line two" {
		t.Errorf("sanitizeHeader(crlf) = %q, want line one  line two", got)
	}
	if got := sanitizeHeader("plain subject"); got != "plain subject" {
		t.Errorf("sanitizeHeader(plain) = %q, want it unchanged", got)
	}

	// A tab cannot terminate a header line, so it is legal folding whitespace
	// and is preserved; only CR/LF can break out of the header block.
	if got := sanitizeHeader("folded\theader"); got != "folded\theader" {
		t.Errorf("sanitizeHeader(tab) = %q, want the folding tab preserved", got)
	}
}

// -----------------------------------------------------------------------------
// Log providers
// -----------------------------------------------------------------------------

func TestLogPushProviderMasksDeviceToken(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	provider := NewLogPushProvider(log.New(&out, "", 0))

	token := "fcm-token-abcdef0123456789"
	err := provider.Send(context.Background(),
		models.DeviceToken{Token: token, Platform: models.PlatformAndroid},
		models.PushMessage{
			RecipientID: uuid.New(),
			Title:       "Alice liked your post",
			Body:        "Your post is getting attention",
		})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	logged := out.String()
	// A device token is a bearer credential for the push provider, and the log
	// provider exists precisely so local runs never write one out in full.
	if strings.Contains(logged, token) {
		t.Errorf("log line = %q, want the device token masked", logged)
	}
	if !strings.Contains(logged, maskToken(token)) {
		t.Errorf("log line = %q, want the masked token %q", logged, maskToken(token))
	}
	// The line still has to be useful for debugging a local run.
	if !strings.Contains(logged, "platform=android") {
		t.Errorf("log line = %q, want the device platform", logged)
	}
	if !strings.Contains(logged, "Alice liked your post") {
		t.Errorf("log line = %q, want the title", logged)
	}
}

func TestLogEmailProviderTruncatesBody(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	provider := NewLogEmailProvider(log.New(&out, "", 0))

	body := strings.Repeat("x", 500)
	message := models.EmailMessage{
		RecipientID:    uuid.New(),
		RecipientEmail: "user@example.com",
		Subject:        "Welcome to SocialFlow",
		Body:           body,
	}
	if err := provider.Send(context.Background(), message); err != nil {
		t.Fatalf("send: %v", err)
	}

	logged := out.String()
	// A broken template can hand the provider an unbounded body; the log
	// provider must keep it inside the log window instead of flooding stdout.
	if strings.Contains(logged, body) {
		t.Error("log line contains the full body, want it truncated to the log window")
	}
	// The address stays verbatim because this provider never leaves the local
	// process; the subject is what an operator scans for.
	if !strings.Contains(logged, "Welcome to SocialFlow") {
		t.Errorf("log line = %q, want the subject", logged)
	}
}

// -----------------------------------------------------------------------------
// NewSMTPEmailProvider
// -----------------------------------------------------------------------------

func TestNewSMTPEmailProviderValidatesHostAndFrom(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		host string
		from string
	}{
		{"missing host", "", "from@example.com"},
		// Whitespace is trimmed by the checker, so a stray space must not
		// produce a relay address that cannot be dialled.
		{"blank host", "   ", "from@example.com"},
		{"missing from", "smtp.example.com", ""},
		{"blank from", "smtp.example.com", "   "},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := NewSMTPEmailProvider(tc.host, 0, "user", "pass", tc.from); err == nil {
				t.Errorf("NewSMTPEmailProvider(host=%q, from=%q) = nil error, want a validation failure", tc.host, tc.from)
			}
		})
	}
}

func TestNewSMTPEmailProviderDefaultsPort(t *testing.T) {
	t.Parallel()

	// 587 is the submission port with STARTTLS; a caller passing a zero-value
	// config must still reach the relay rather than dialling port 0.
	for _, port := range []int{0, -1} {
		provider, err := NewSMTPEmailProvider("smtp.example.com", port, "user", "pass", "from@example.com")
		if err != nil {
			t.Fatalf("NewSMTPEmailProvider(port=%d): %v", port, err)
		}
		if provider.port != 587 {
			t.Errorf("port = %d for input %d, want the 587 default", provider.port, port)
		}
	}

	provider, err := NewSMTPEmailProvider("smtp.example.com", 2525, "", "", "from@example.com")
	if err != nil {
		t.Fatalf("NewSMTPEmailProvider: %v", err)
	}
	if provider.port != 2525 {
		t.Errorf("port = %d, want the explicit 2525 to be kept", provider.port)
	}
	if provider.from != "from@example.com" {
		t.Errorf("from = %q, want from@example.com", provider.from)
	}
}

// -----------------------------------------------------------------------------
// JWT / key helpers (the rest are covered in push_providers_test.go)
// -----------------------------------------------------------------------------

func TestEncodeJWTPartEmitsUnpaddedBase64URL(t *testing.T) {
	t.Parallel()

	part, err := encodeJWTPart(map[string]string{"alg": "ES256", "typ": "JWT"})
	if err != nil {
		t.Fatalf("encodeJWTPart: %v", err)
	}

	// JWT segments are base64url *without* padding: "=", "+" and "/" are all
	// illegal inside the Authorization header the gateway echoes back.
	if strings.ContainsAny(part, "=+/") {
		t.Errorf("part = %q, want unpadded raw base64url", part)
	}

	decoded, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		t.Fatalf("decode %q: %v", part, err)
	}
	var header map[string]string
	if err := json.Unmarshal(decoded, &header); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	if header["alg"] != "ES256" || header["typ"] != "JWT" {
		t.Errorf("header = %v, want alg=ES256 and typ=JWT", header)
	}
}

func TestEncodeJWTPartRejectsUnmarshalableValue(t *testing.T) {
	t.Parallel()

	// Claims come from a map the caller controls; a value JSON cannot encode
	// must surface as an error instead of a silently blank JWT segment.
	if _, err := encodeJWTPart(map[string]any{"unsupported": make(chan int)}); err == nil {
		t.Error("encodeJWTPart with a channel value should fail")
	}
}

func TestParseECPrivateKey(t *testing.T) {
	t.Parallel()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	sec1, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal SEC1: %v", err)
	}
	pemSEC1 := string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: sec1}))

	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal PKCS8: %v", err)
	}
	pemPKCS8 := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}))

	// Service-account and .p8 downloads are frequently pasted through JSON,
	// which escapes every newline. Both spellings must resolve to the same key.
	escaped := strings.ReplaceAll(pemSEC1, "\n", `\n`)

	cases := []struct {
		name    string
		pemData string
	}{
		{"SEC1 EC PRIVATE KEY", pemSEC1},
		{"PKCS8 PRIVATE KEY", pemPKCS8},
		{"escaped newlines", escaped},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			parsed, err := parseECPrivateKey(tc.pemData)
			if err != nil {
				t.Fatalf("parseECPrivateKey: %v", err)
			}
			if !parsed.Equal(key) {
				t.Error("parsed key does not match the generated key")
			}
		})
	}
}

func TestParseECPrivateKeyRejectsUnsupportedFormats(t *testing.T) {
	t.Parallel()

	if _, err := parseECPrivateKey("not pem"); err == nil {
		t.Error("garbage input should fail to decode as PEM")
	}

	// An RSA key parses cleanly as PKCS8 but is not an APNs signing key: it
	// must be rejected rather than silently handed to ecdsa.Sign.
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	rsaPKCS8, err := x509.MarshalPKCS8PrivateKey(rsaKey)
	if err != nil {
		t.Fatalf("marshal RSA key: %v", err)
	}
	rsaPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: rsaPKCS8}))

	if _, err := parseECPrivateKey(rsaPEM); err == nil {
		t.Error("an RSA key should be rejected as an EC private key")
	}
}

func TestParsedECKeySignsVerifiableJWT(t *testing.T) {
	t.Parallel()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	sec1, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	pemData := string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: sec1}))

	parsed, err := parseECPrivateKey(pemData)
	if err != nil {
		t.Fatalf("parseECPrivateKey: %v", err)
	}

	// A key that decodes but silently loses the scalar would fail at the
	// provider, so round trip it through the real signing path.
	token, err := signES256(parsed, "KEY123", map[string]any{"iss": "TEAM123"})
	if err != nil {
		t.Fatalf("signES256: %v", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token parts = %d, want 3", len(parts))
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}

	byteLen := (parsed.Curve.Params().BitSize + 7) / 8
	if len(signature) != byteLen*2 {
		t.Fatalf("signature length = %d, want %d", len(signature), byteLen*2)
	}
	r := new(big.Int).SetBytes(signature[:byteLen])
	s := new(big.Int).SetBytes(signature[byteLen:])

	hash := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(&parsed.PublicKey, hash[:], r, s) {
		t.Error("ES256 token signed with a PEM-parsed key did not verify")
	}
}
