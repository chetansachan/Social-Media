package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newRevocationStore(t *testing.T) (*miniredis.Miniredis, *RedisRevocations) {
	t.Helper()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return server, NewRedisRevocations(client)
}

func TestRedisRevocationsRoundTrip(t *testing.T) {
	_, store := newRevocationStore(t)
	ctx := context.Background()

	revoked, err := store.IsRevoked(ctx, "jti-1")
	if err != nil {
		t.Fatalf("IsRevoked: %v", err)
	}
	if revoked {
		t.Fatal("a fresh token id must not be revoked")
	}

	if err := store.Revoke(ctx, "jti-1", time.Minute); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	revoked, err = store.IsRevoked(ctx, "jti-1")
	if err != nil {
		t.Fatalf("IsRevoked: %v", err)
	}
	if !revoked {
		t.Fatal("token id was not blacklisted")
	}
}

func TestRedisRevocationsPrunesAtTokenExpiry(t *testing.T) {
	server, store := newRevocationStore(t)
	ctx := context.Background()

	if err := store.Revoke(ctx, "jti-1", time.Minute); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	// The key must not outlive the token; Redis expires it on its own.
	server.FastForward(2 * time.Minute)

	revoked, err := store.IsRevoked(ctx, "jti-1")
	if err != nil {
		t.Fatalf("IsRevoked: %v", err)
	}
	if revoked {
		t.Fatal("revocation outlived the token it protected")
	}
}

func TestRedisRevocationsIgnoresExpiredOrMissingTokens(t *testing.T) {
	_, store := newRevocationStore(t)
	ctx := context.Background()

	// An already-expired token needs no blacklist entry.
	if err := store.Revoke(ctx, "jti-1", -time.Minute); err != nil {
		t.Fatalf("Revoke with a past expiry: %v", err)
	}
	if revoked, _ := store.IsRevoked(ctx, "jti-1"); revoked {
		t.Fatal("an expired token id was stored")
	}

	// A token without a jti cannot be tracked; it must not be reported revoked.
	if revoked, err := store.IsRevoked(ctx, ""); err != nil || revoked {
		t.Fatalf("empty id = %v, %v", revoked, err)
	}
	if err := store.Revoke(ctx, "", time.Minute); err == nil {
		t.Fatal("expected revoking an empty id to fail")
	}
}

type stubRevocations struct {
	revoked   bool
	err       error
	lastID    string
	lastTTL   time.Duration
	revokeErr error
}

func (s *stubRevocations) Revoke(ctx context.Context, tokenID string, ttl time.Duration) error {
	s.lastID = tokenID
	s.lastTTL = ttl
	return s.revokeErr
}

func (s *stubRevocations) IsRevoked(ctx context.Context, tokenID string) (bool, error) {
	s.lastID = tokenID
	return s.revoked, s.err
}

func TestRejectRevokedRejectsBlacklistedSession(t *testing.T) {
	store := &stubRevocations{revoked: true}
	handler := RejectRevoked(store, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("a revoked token must not reach the handler")
	}))

	request := httptest.NewRequest(http.MethodPost, "/query", nil)
	request = request.WithContext(WithPrincipal(request.Context(), Principal{
		UserID: "u1", TokenID: "jti-1",
	}))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
	if store.lastID != "jti-1" {
		t.Fatalf("checked token id %q", store.lastID)
	}
}

func TestRejectRevokedPassesLiveSession(t *testing.T) {
	store := &stubRevocations{}
	handler := RejectRevoked(store, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodPost, "/query", nil)
	request = request.WithContext(WithPrincipal(request.Context(), Principal{
		UserID: "u1", TokenID: "jti-1",
	}))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
}

func TestRejectRevokedFailsClosedWhenRedisIsDown(t *testing.T) {
	store := &stubRevocations{err: errors.New("redis unavailable")}
	handler := RejectRevoked(store, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("a request that cannot be checked must not proceed")
	}))

	request := httptest.NewRequest(http.MethodPost, "/query", nil)
	request = request.WithContext(WithPrincipal(request.Context(), Principal{
		UserID: "u1", TokenID: "jti-1",
	}))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}

func TestRejectRevokedAllowsAnonymousRequests(t *testing.T) {
	store := &stubRevocations{err: errors.New("must not be consulted")}
	handler := RejectRevoked(store, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/query", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
}

func TestWebsocketInitRejectsRevokedToken(t *testing.T) {
	raw := signToken(t, testSecret, time.Now().Add(time.Minute))
	init := WebsocketInit(testSecret, &stubRevocations{revoked: true})

	if _, _, err := init(context.Background(), map[string]any{
		"Authorization": "Bearer " + raw,
	}); err == nil {
		t.Fatal("expected a logged-out websocket token to be rejected")
	}
}

func TestWebsocketInitAcceptsLiveTokenWithStore(t *testing.T) {
	raw := signToken(t, testSecret, time.Now().Add(time.Minute))
	init := WebsocketInit(testSecret, &stubRevocations{})

	ctx, _, err := init(context.Background(), map[string]any{
		"Authorization": "Bearer " + raw,
	})
	if err != nil {
		t.Fatalf("WebsocketInit: %v", err)
	}
	if _, ok := FromContext(ctx); !ok {
		t.Fatal("principal was not attached")
	}
}
