package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// RevocationStore remembers access tokens that were explicitly logged out
// before their natural expiry.
type RevocationStore interface {
	// Revoke blacklists a token id for at most ttl.
	Revoke(ctx context.Context, tokenID string, ttl time.Duration) error
	// IsRevoked reports whether a token id was blacklisted.
	IsRevoked(ctx context.Context, tokenID string) (bool, error)
}

// ErrAlreadyRevoked is returned by a store that expects the token to be live.
var ErrAlreadyRevoked = errors.New("token already revoked")

// RedisRevocations stores revocations in Redis. Each key lives exactly as long
// as the token would otherwise be valid, so the blacklist prunes itself and
// never grows without bound.
type RedisRevocations struct {
	client *redis.Client
}

// NewRedisRevocations returns a Redis-backed revocation store.
func NewRedisRevocations(client *redis.Client) *RedisRevocations {
	return &RedisRevocations{client: client}
}

func revocationKey(tokenID string) string {
	return "token:blacklist:" + tokenID
}

// Revoke blacklists tokenID for ttl. A non-positive ttl means the token already
// expired, so nothing needs to be stored.
func (r *RedisRevocations) Revoke(ctx context.Context, tokenID string, ttl time.Duration) error {
	tokenID = strings.TrimSpace(tokenID)
	if tokenID == "" {
		return errors.New("cannot revoke a token without an id")
	}
	if ttl <= 0 {
		return nil
	}
	if err := r.client.Set(ctx, revocationKey(tokenID), "1", ttl).Err(); err != nil {
		return fmt.Errorf("store token revocation: %w", err)
	}
	return nil
}

// IsRevoked reports whether tokenID is blacklisted. An empty id is never
// revoked: tokens minted before jti support cannot be tracked, and they expire
// on their own within the access-token lifetime.
func (r *RedisRevocations) IsRevoked(ctx context.Context, tokenID string) (bool, error) {
	tokenID = strings.TrimSpace(tokenID)
	if tokenID == "" {
		return false, nil
	}
	exists, err := r.client.Exists(ctx, revocationKey(tokenID)).Result()
	if err != nil {
		return false, fmt.Errorf("check token revocation: %w", err)
	}
	return exists > 0, nil
}

// RejectRevoked refuses a request whose access token was logged out. It runs
// after Middleware, so it only has to inspect an already verified principal.
//
// A store failure fails closed: serving a logged-out token because Redis is
// briefly unavailable is worse than asking the client to retry.
func RejectRevoked(store RevocationStore, next http.Handler) http.Handler {
	if store == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := FromContext(r.Context())
		if !ok {
			next.ServeHTTP(w, r)
			return
		}

		revoked, err := store.IsRevoked(r.Context(), principal.TokenID)
		if err != nil {
			writeAuthError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "could not verify the session")
			return
		}
		if revoked {
			writeAuthError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "session has been revoked")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func writeAuthError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"errors":[{"message":%q,"extensions":{"code":%q}}]}`, message, code)
}
