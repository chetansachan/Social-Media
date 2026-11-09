package main

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestTimeoutInterceptorAddsDeadline(t *testing.T) {
	interceptor := timeoutInterceptor(3 * time.Second)

	err := interceptor(
		context.Background(),
		"/test.Service/Method",
		nil,
		nil,
		nil,
		func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Fatal("invoker saw no deadline")
			}
			remaining := time.Until(deadline)
			if remaining <= 2*time.Second || remaining > 3*time.Second {
				t.Fatalf("remaining deadline = %s, want just under 3s", remaining)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("intercept: %v", err)
	}
}

func TestTimeoutInterceptorKeepsTighterCallerDeadline(t *testing.T) {
	interceptor := timeoutInterceptor(3 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := interceptor(
		ctx,
		"/test.Service/Method",
		nil,
		nil,
		nil,
		func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Fatal("invoker saw no deadline")
			}
			if remaining := time.Until(deadline); remaining > 150*time.Millisecond {
				t.Fatalf("remaining deadline = %s, want the caller's 100ms bound", remaining)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("intercept: %v", err)
	}
}

func TestBreakerOpensAfterConsecutiveFailures(t *testing.T) {
	breakers := newBreakerSet(time.Minute)
	calls := 0
	failing := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		calls++
		return status.Error(codes.Unavailable, "backend down")
	}

	for i := 0; i < 5; i++ {
		err := breakers.Intercept(context.Background(), "/test.Service/Method", nil, nil, nil, failing)
		if status.Code(err) != codes.Unavailable {
			t.Fatalf("call %d code = %s, want Unavailable", i, status.Code(err))
		}
	}
	if calls != 5 {
		t.Fatalf("invoker calls = %d, want 5", calls)
	}

	// The sixth call must be rejected without touching the backend.
	err := breakers.Intercept(context.Background(), "/test.Service/Method", nil, nil, nil, failing)
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("open breaker code = %s, want Unavailable", status.Code(err))
	}
	if calls != 5 {
		t.Fatalf("invoker calls = %d, want 5: the breaker did not fail fast", calls)
	}
}

func TestBreakerDoesNotTripOnApplicationErrors(t *testing.T) {
	breakers := newBreakerSet(time.Minute)
	calls := 0
	notFound := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		calls++
		return status.Error(codes.NotFound, "missing")
	}

	for i := 0; i < 20; i++ {
		err := breakers.Intercept(context.Background(), "/test.Service/Method", nil, nil, nil, notFound)
		if status.Code(err) != codes.NotFound {
			t.Fatalf("call %d code = %s, want NotFound", i, status.Code(err))
		}
	}
	if calls != 20 {
		t.Fatalf("invoker calls = %d, want 20: application errors must not trip the breaker", calls)
	}
}

func TestIsSuccessfulRPCTreatsTransportFailuresAsFailures(t *testing.T) {
	failures := []codes.Code{codes.Unavailable, codes.DeadlineExceeded, codes.Internal, codes.ResourceExhausted}
	for _, code := range failures {
		if isSuccessfulRPC(status.Error(code, "failure")) {
			t.Fatalf("code %s was counted as a success", code)
		}
	}

	successes := []codes.Code{codes.NotFound, codes.PermissionDenied, codes.InvalidArgument, codes.OK}
	for _, code := range successes {
		if !isSuccessfulRPC(status.Error(code, "application response")) {
			t.Fatalf("code %s was counted as a dependency failure", code)
		}
	}
}
