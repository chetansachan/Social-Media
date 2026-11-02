package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func signToken(t *testing.T, secret string, claims map[string]any, algorithm string) string {
	t.Helper()

	header, err := json.Marshal(map[string]string{"alg": algorithm, "typ": "JWT"})
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}

	encode := base64.RawURLEncoding.EncodeToString
	signingInput := encode(header) + "." + encode(payload)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	signature := encode(mac.Sum(nil))

	return signingInput + "." + signature
}

func TestVerifyHS256(t *testing.T) {
	t.Parallel()

	userID := uuid.NewString()
	token := signToken(t, "secret", map[string]any{
		"sub": userID,
		"exp": time.Now().Add(time.Hour).Unix(),
	}, "HS256")

	claims, err := verifyHS256(token, "secret")
	if err != nil {
		t.Fatalf("verifyHS256: %v", err)
	}
	if claims["sub"] != userID {
		t.Errorf("sub = %v", claims["sub"])
	}
}

func TestVerifyHS256RejectsBadTokens(t *testing.T) {
	t.Parallel()

	userID := uuid.NewString()

	if _, err := verifyHS256("not-a-token", "secret"); err == nil {
		t.Error("malformed token should fail")
	}

	valid := signToken(t, "secret", map[string]any{"sub": userID}, "HS256")
	if _, err := verifyHS256(valid, "different-secret"); err == nil {
		t.Error("wrong secret should fail")
	}

	expired := signToken(t, "secret", map[string]any{
		"sub": userID,
		"exp": time.Now().Add(-time.Hour).Unix(),
	}, "HS256")
	if _, err := verifyHS256(expired, "secret"); err == nil {
		t.Error("expired token should fail")
	}

	wrongAlg := signToken(t, "secret", map[string]any{"sub": userID}, "HS512")
	if _, err := verifyHS256(wrongAlg, "secret"); err == nil {
		t.Error("non-HS256 token should fail")
	}
}

func TestWSAuthenticateWithJWT(t *testing.T) {
	t.Parallel()

	userID := uuid.NewString()
	token := signToken(t, "secret", map[string]any{
		"user_id": userID,
		"exp":     time.Now().Add(time.Hour).Unix(),
	}, "HS256")

	handler := NewWSHandler(nil, "secret", false, []string{"*"}, nil)

	request := httptest.NewRequest("GET", "/ws?token="+token, nil)
	got, err := handler.authenticate(request)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if got != userID {
		t.Errorf("user id = %q, want %q", got, userID)
	}
}

func TestWSAuthenticateDevMode(t *testing.T) {
	t.Parallel()

	userID := uuid.NewString()
	handler := NewWSHandler(nil, "", true, []string{"*"}, nil)

	request := httptest.NewRequest("GET", "/ws?user_id="+userID, nil)
	got, err := handler.authenticate(request)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if got != userID {
		t.Errorf("user id = %q, want %q", got, userID)
	}
}

func TestWSAuthenticateRejectsMissingConfig(t *testing.T) {
	t.Parallel()

	handler := NewWSHandler(nil, "", false, []string{"*"}, nil)
	request := httptest.NewRequest("GET", "/ws?user_id="+uuid.NewString(), nil)

	if _, err := handler.authenticate(request); err == nil {
		t.Fatal("dev auth should be disabled when allowDevUserID is false")
	}
}

func TestWSAuthenticateRejectsInvalidUserID(t *testing.T) {
	t.Parallel()

	token := signToken(t, "secret", map[string]any{
		"sub": "not-a-uuid",
		"exp": time.Now().Add(time.Hour).Unix(),
	}, "HS256")

	handler := NewWSHandler(nil, "secret", false, []string{"*"}, nil)
	request := httptest.NewRequest("GET", "/ws?token="+token, nil)

	if _, err := handler.authenticate(request); err == nil {
		t.Fatal("invalid UUID claim should be rejected")
	}
}
