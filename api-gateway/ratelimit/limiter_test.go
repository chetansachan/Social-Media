package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestLimiter(t *testing.T) (*Limiter, *miniredis.Miniredis) {
	t.Helper()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return New(client), server
}

func TestLimiterAllowsExactlyTheBudget(t *testing.T) {
	limiter, _ := newTestLimiter(t)
	now := time.Unix(1_700_000_000, 0)
	policy := Policy{Limit: 3, Window: time.Minute}

	for i := 0; i < 3; i++ {
		allowed, err := limiter.Allow(context.Background(), "Login", "ip:1.2.3.4", policy, now)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		if !allowed {
			t.Fatalf("request %d was rejected before the budget ran out", i)
		}
	}

	allowed, err := limiter.Allow(context.Background(), "Login", "ip:1.2.3.4", policy, now)
	if err != nil {
		t.Fatalf("over-budget request: %v", err)
	}
	if allowed {
		t.Fatal("fourth request was allowed with a limit of three")
	}
}

func TestLimiterSeparatesSubjectsAndFields(t *testing.T) {
	limiter, _ := newTestLimiter(t)
	now := time.Unix(1_700_000_000, 0)
	policy := Policy{Limit: 1, Window: time.Minute}

	if allowed, _ := limiter.Allow(context.Background(), "Login", "ip:1.2.3.4", policy, now); !allowed {
		t.Fatal("first subject was rejected")
	}
	if allowed, _ := limiter.Allow(context.Background(), "Login", "ip:1.2.3.4", policy, now); allowed {
		t.Fatal("the same subject exceeded its budget")
	}
	if allowed, _ := limiter.Allow(context.Background(), "Login", "ip:5.6.7.8", policy, now); !allowed {
		t.Fatal("a different subject inherited the first subject's budget")
	}
	if allowed, _ := limiter.Allow(context.Background(), "CreatePost", "ip:1.2.3.4", policy, now); !allowed {
		t.Fatal("a different field inherited the first field's budget")
	}
}

func TestLimiterRefillsAfterTheWindow(t *testing.T) {
	limiter, _ := newTestLimiter(t)
	now := time.Unix(1_700_000_000, 0)
	policy := Policy{Limit: 1, Window: time.Minute}

	if allowed, _ := limiter.Allow(context.Background(), "getFeed", "user:u1", policy, now); !allowed {
		t.Fatal("first request was rejected")
	}
	if allowed, _ := limiter.Allow(context.Background(), "getFeed", "user:u1", policy, now.Add(30*time.Second)); allowed {
		t.Fatal("request inside the window was allowed")
	}
	if allowed, _ := limiter.Allow(context.Background(), "getFeed", "user:u1", policy, now.Add(61*time.Second)); !allowed {
		t.Fatal("request after the window was still rejected")
	}
}

func TestLimiterDisabledPolicyAllows(t *testing.T) {
	limiter, _ := newTestLimiter(t)
	allowed, err := limiter.Allow(context.Background(), "Login", "ip:1.2.3.4", Policy{}, time.Now())
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if !allowed {
		t.Fatal("a zero policy should not limit")
	}
}
