// Package ratelimit enforces per-user and per-IP GraphQL budgets in Redis.
package ratelimit

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// Policy is a sliding-window budget: at most Limit requests per Window.
type Policy struct {
	Limit  int
	Window time.Duration
}

// Limiter stores one sorted set per subject. Trimming, counting, and inserting
// happen inside a single Lua script, so concurrent requests from every gateway
// pod share one atomic view of the budget.
type Limiter struct {
	client *redis.Client
	seq    atomic.Uint64
}

func New(client *redis.Client) *Limiter {
	return &Limiter{client: client}
}

var slidingWindowScript = redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local member = ARGV[4]

-- Drop everything that fell out of the window before counting.
redis.call('ZREMRANGEBYSCORE', key, '-inf', now - window)
local used = redis.call('ZCARD', key)
if used >= limit then
  return 0
end

redis.call('ZADD', key, now, member)
redis.call('PEXPIRE', key, window)
return 1
`)

// Allow reports whether subject may perform one more request under policy. The
// window is anchored to now, so a caller that already fell behind cannot earn
// extra budget. A missing subject or a disabled policy allows the request.
func (l *Limiter) Allow(ctx context.Context, field, subject string, policy Policy, now time.Time) (bool, error) {
	if l == nil || l.client == nil || policy.Limit <= 0 || policy.Window <= 0 {
		return true, nil
	}
	if strings.TrimSpace(subject) == "" {
		return true, nil
	}

	// The window is part of the key so changing a limit never mixes old and
	// new budgets in the same sorted set.
	key := fmt.Sprintf("ratelimit:%s:%s:%d", field, subject, int(policy.Window/time.Second))
	member := fmt.Sprintf("%d:%d", now.UnixNano(), l.seq.Add(1))

	allowed, err := slidingWindowScript.Run(ctx, l.client, []string{key},
		now.UnixMilli(),
		policy.Window.Milliseconds(),
		policy.Limit,
		member,
	).Int()
	if err != nil {
		return false, err
	}
	return allowed == 1, nil
}

type clientIPKey struct{}

// WithClientIP copies the caller's address into the request context so the
// GraphQL layer can limit anonymous traffic. Forwarded headers are trusted only
// when the gateway sits behind a proxy that overwrites them.
func WithClientIP(trustForwarded bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r, trustForwarded)
			if ip != "" {
				r = r.WithContext(context.WithValue(r.Context(), clientIPKey{}, ip))
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request, trustForwarded bool) string {
	if trustForwarded {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			first := strings.TrimSpace(strings.Split(forwarded, ",")[0])
			if first != "" {
				return first
			}
		}
		if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
			return realIP
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}

// ClientIP returns the address stored by WithClientIP, or an empty string.
func ClientIP(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPKey{}).(string)
	return ip
}
