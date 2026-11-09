package main

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/sony/gobreaker/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// timeoutInterceptor bounds every backend RPC. gRPC forwards the deadline as
// grpc-timeout, so a slow dependency cancels its own work instead of holding a
// gateway goroutine and a downstream transaction open indefinitely.
func timeoutInterceptor(timeout time.Duration) grpc.UnaryClientInterceptor {
	if timeout <= 0 {
		return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
	}

	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		// Never replace a tighter deadline the caller already set.
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= timeout {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// breakerSet keeps one circuit breaker per gRPC method. A per-method breaker
// isolates a failing endpoint (for example GetFeed) from healthy ones on the
// same connection, and a trip stops new calls for the open timeout instead of
// queueing them behind a dependency that is already saturated.
type breakerSet struct {
	mu       sync.Mutex
	breakers map[string]*gobreaker.CircuitBreaker[struct{}]
	timeout  time.Duration
}

func newBreakerSet(openTimeout time.Duration) *breakerSet {
	if openTimeout <= 0 {
		openTimeout = 10 * time.Second
	}
	return &breakerSet{
		breakers: make(map[string]*gobreaker.CircuitBreaker[struct{}]),
		timeout:  openTimeout,
	}
}

func (b *breakerSet) breaker(method string) *gobreaker.CircuitBreaker[struct{}] {
	b.mu.Lock()
	defer b.mu.Unlock()

	if existing, ok := b.breakers[method]; ok {
		return existing
	}

	breaker := gobreaker.NewCircuitBreaker[struct{}](gobreaker.Settings{
		Name:        method,
		MaxRequests: 1, // one half-open probe at a time
		Interval:    0, // counts reset only on state change
		Timeout:     b.timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// Five consecutive failures is enough evidence for a low-traffic
			// method; busier methods also trip at a 50% failure rate.
			if counts.Requests < 10 {
				return counts.ConsecutiveFailures >= 5
			}
			return float64(counts.TotalFailures)/float64(counts.Requests) > 0.5
		},
		IsSuccessful: isSuccessfulRPC,
		OnStateChange: func(name string, from, to gobreaker.State) {
			level := slog.LevelInfo
			if to == gobreaker.StateOpen {
				level = slog.LevelWarn
			}
			slog.Log(context.Background(), level, "gRPC circuit breaker state changed",
				"method", name, "from", from.String(), "to", to.String())
		},
	})
	b.breakers[method] = breaker
	return breaker
}

// Intercept rejects calls while the method's breaker is open and records the
// outcome of every allowed call.
func (b *breakerSet) Intercept(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	_, err := b.breaker(method).Execute(func() (struct{}, error) {
		return struct{}{}, invoker(ctx, method, req, reply, cc, opts...)
	})
	if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
		return status.Errorf(codes.Unavailable, "circuit breaker open for %s", method)
	}
	return err
}

// isSuccessfulRPC separates dependency failures from application responses. A
// NotFound or PermissionDenied means the service answered, so it must not count
// toward tripping the breaker; Unavailable, DeadlineExceeded, and Internal mean
// the dependency itself is unhealthy.
func isSuccessfulRPC(err error) bool {
	if err == nil {
		return true
	}
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted, codes.Internal, codes.Unknown:
		return false
	default:
		return true
	}
}
