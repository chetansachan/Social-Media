package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const testSecret = "test-secret"

func signToken(t *testing.T, secret string, expires time.Time) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		UserID: "11111111-1111-1111-1111-111111111111",
		Email:  "alice@example.com",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expires),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func TestParseToken_Valid(t *testing.T) {
	raw := signToken(t, testSecret, time.Now().Add(time.Minute))

	principal, err := ParseToken(raw, testSecret)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if principal.UserID != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("user id = %q", principal.UserID)
	}
	if principal.Role != "user" {
		t.Errorf("role = %q, want user default", principal.Role)
	}
	if principal.Token != raw {
		t.Error("raw token was not retained")
	}
}

func TestParseToken_RejectsExpiredAndForged(t *testing.T) {
	expired := signToken(t, testSecret, time.Now().Add(-time.Minute))
	if _, err := ParseToken(expired, testSecret); err == nil {
		t.Fatal("expected expired token to be rejected")
	}

	forged := signToken(t, "different-secret", time.Now().Add(time.Minute))
	if _, err := ParseToken(forged, testSecret); err == nil {
		t.Fatal("expected token with wrong signature to be rejected")
	}
}

func TestMiddleware_AttachesPrincipalAndRejectsInvalidToken(t *testing.T) {
	raw := signToken(t, testSecret, time.Now().Add(time.Minute))

	var seen Principal
	handler := Middleware(testSecret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, _ = FromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodPost, "/query", nil)
	request.Header.Set("Authorization", "Bearer "+raw)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
	if seen.UserID == "" {
		t.Fatal("principal was not attached")
	}

	badRequest := httptest.NewRequest(http.MethodPost, "/query", nil)
	badRequest.Header.Set("Authorization", "Bearer not-a-token")
	badRecorder := httptest.NewRecorder()
	handler.ServeHTTP(badRecorder, badRequest)
	if badRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("invalid token status = %d, want 401", badRecorder.Code)
	}
}

func TestMiddleware_AllowsAnonymousPublicRequests(t *testing.T) {
	handler := Middleware(testSecret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/query", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
}

func TestRequire_ReturnsUnauthenticatedCode(t *testing.T) {
	_, err := Require(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}

	var payload struct {
		Extensions map[string]any `json:"extensions"`
	}
	encoded, _ := json.Marshal(err)
	_ = json.Unmarshal(encoded, &payload)
	if payload.Extensions["code"] != "UNAUTHENTICATED" {
		t.Fatalf("extensions = %v", payload.Extensions)
	}
}

func TestWebsocketInit_ParsesConnectionParams(t *testing.T) {
	raw := signToken(t, testSecret, time.Now().Add(time.Minute))
	init := WebsocketInit(testSecret)

	ctx, _, err := init(context.Background(), map[string]any{
		"Authorization": "Bearer " + raw,
	})
	if err != nil {
		t.Fatalf("WebsocketInit: %v", err)
	}
	principal, ok := FromContext(ctx)
	if !ok || principal.UserID == "" {
		t.Fatal("principal was not attached to the websocket context")
	}

	if _, _, err := init(context.Background(), map[string]any{
		"Authorization": "Bearer forged",
	}); err == nil {
		t.Fatal("expected invalid websocket token to be rejected")
	}
}

func TestUnaryClientInterceptor_ForwardsIdentity(t *testing.T) {
	var captured metadata.MD
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		captured, _ = metadata.FromOutgoingContext(ctx)
		return nil
	}

	ctx := WithPrincipal(context.Background(), Principal{
		UserID: "user-1", Email: "a@example.com", Role: "user", Token: "raw-token",
	})
	interceptor := UnaryClientInterceptor()
	if err := interceptor(ctx, "/test", nil, nil, nil, invoker); err != nil {
		t.Fatalf("interceptor: %v", err)
	}

	if got := captured.Get("x-user-id"); len(got) != 1 || got[0] != "user-1" {
		t.Fatalf("x-user-id = %v", got)
	}
	if got := captured.Get("authorization"); len(got) != 1 || got[0] != "Bearer raw-token" {
		t.Fatalf("authorization = %v", got)
	}
}
