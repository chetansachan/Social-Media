package ratelimit

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/chetansachan/socialflow/api-gateway/auth"
	"github.com/vektah/gqlparser/v2/ast"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func operationContext(operationName, field string) *graphql.OperationContext {
	return &graphql.OperationContext{
		OperationName: operationName,
		Operation: &ast.OperationDefinition{
			Operation: ast.Mutation,
			SelectionSet: ast.SelectionSet{
				&ast.Field{Name: field},
			},
		},
	}
}

func countingHandler(executed *int) graphql.OperationHandler {
	return func(ctx context.Context) graphql.ResponseHandler {
		return func(context.Context) *graphql.Response {
			*executed++
			return &graphql.Response{Data: []byte(`{"ok":true}`)}
		}
	}
}

func TestGuardRejectsWhenBudgetIsExhausted(t *testing.T) {
	limiter, _ := newTestLimiter(t)
	guard := NewGuard(limiter,
		map[string]Policy{"CreatePost": {Limit: 2, Window: time.Minute}},
		Policy{Limit: 100, Window: time.Minute},
		Policy{Limit: 100, Window: time.Minute},
		discardLogger(),
	)

	ctx := graphql.WithOperationContext(context.Background(), operationContext("MyMutation", "CreatePost"))
	ctx = auth.WithPrincipal(ctx, auth.Principal{UserID: "u1"})

	executed := 0
	next := countingHandler(&executed)

	for i := 0; i < 2; i++ {
		response := guard.InterceptOperation(ctx, next)(ctx)
		if len(response.Errors) != 0 {
			t.Fatalf("request %d was rejected: %v", i, response.Errors)
		}
	}

	response := guard.InterceptOperation(ctx, next)(ctx)
	if len(response.Errors) != 1 {
		t.Fatalf("over-budget response errors = %d, want 1", len(response.Errors))
	}
	if code := response.Errors[0].Extensions["code"]; code != "RATE_LIMITED" {
		t.Fatalf("error code = %v, want RATE_LIMITED", code)
	}
	if executed != 2 {
		t.Fatalf("resolver ran %d times, want 2: a denied request executed the operation", executed)
	}
}

func TestGuardKeysOnRootFieldNotOperationName(t *testing.T) {
	limiter, _ := newTestLimiter(t)
	guard := NewGuard(limiter,
		map[string]Policy{"CreatePost": {Limit: 1, Window: time.Minute}},
		Policy{Limit: 100, Window: time.Minute},
		Policy{Limit: 100, Window: time.Minute},
		discardLogger(),
	)

	// A client names a write operation after a cheap query. The guard must
	// still spend the CreatePost budget.
	ctx := graphql.WithOperationContext(context.Background(), operationContext("GetFeed", "CreatePost"))
	ctx = auth.WithPrincipal(ctx, auth.Principal{UserID: "u1"})

	executed := 0
	next := countingHandler(&executed)

	if response := guard.InterceptOperation(ctx, next)(ctx); len(response.Errors) != 0 {
		t.Fatalf("first request was rejected: %v", response.Errors)
	}
	response := guard.InterceptOperation(ctx, next)(ctx)
	if len(response.Errors) != 1 || response.Errors[0].Extensions["code"] != "RATE_LIMITED" {
		t.Fatalf("second request errors = %v, want RATE_LIMITED", response.Errors)
	}
}

func TestGuardLimitsAnonymousTrafficByClientIP(t *testing.T) {
	limiter, _ := newTestLimiter(t)
	guard := NewGuard(limiter,
		map[string]Policy{"Login": {Limit: 1, Window: time.Minute}},
		Policy{Limit: 100, Window: time.Minute},
		Policy{Limit: 1, Window: time.Minute},
		discardLogger(),
	)

	executed := 0
	next := countingHandler(&executed)

	one := graphql.WithOperationContext(context.Background(), operationContext("Login", "Login"))
	one = context.WithValue(one, clientIPKey{}, "203.0.113.9")

	if response := guard.InterceptOperation(one, next)(one); len(response.Errors) != 0 {
		t.Fatalf("first request was rejected: %v", response.Errors)
	}
	second := guard.InterceptOperation(one, next)(one)
	if len(second.Errors) != 1 {
		t.Fatalf("second request from the same IP had %d errors, want 1", len(second.Errors))
	}

	// A different address gets its own budget.
	other := graphql.WithOperationContext(context.Background(), operationContext("Login", "Login"))
	other = context.WithValue(other, clientIPKey{}, "198.51.100.7")
	if response := guard.InterceptOperation(other, next)(other); len(response.Errors) != 0 {
		t.Fatalf("request from a different IP was rejected: %v", response.Errors)
	}
}

func TestGuardFailsOpenWhenRedisIsUnavailable(t *testing.T) {
	limiter, server := newTestLimiter(t)
	server.Close()

	guard := NewGuard(limiter,
		map[string]Policy{"Login": {Limit: 1, Window: time.Minute}},
		Policy{Limit: 100, Window: time.Minute},
		Policy{Limit: 1, Window: time.Minute},
		discardLogger(),
	)

	ctx := graphql.WithOperationContext(context.Background(), operationContext("Login", "Login"))
	ctx = context.WithValue(ctx, clientIPKey{}, "203.0.113.9")

	executed := 0
	response := guard.InterceptOperation(ctx, countingHandler(&executed))(ctx)
	if len(response.Errors) != 0 {
		t.Fatalf("rate limiter outage rejected the request: %v", response.Errors)
	}
	if executed != 1 {
		t.Fatalf("resolver ran %d times, want 1", executed)
	}
}
