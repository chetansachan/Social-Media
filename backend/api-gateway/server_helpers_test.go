package main

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"google.golang.org/grpc"
)

func TestEnvOrTrimsAndFallsBack(t *testing.T) {
	t.Setenv("TEST_ONE", "  value  ")
	if got := envOr("TEST_ONE", "fallback"); got != "value" {
		t.Fatalf("envOr = %q, want the trimmed value", got)
	}

	t.Setenv("TEST_EMPTY", "   ")
	if got := envOr("TEST_EMPTY", "fallback"); got != "fallback" {
		t.Fatalf("envOr = %q, want the fallback for a blank value", got)
	}

	if got := envOr("TEST_MISSING", "fallback"); got != "fallback" {
		t.Fatalf("envOr = %q, want the fallback for an unset var", got)
	}
}

func TestEnvParsersFallBackOnBadInput(t *testing.T) {
	t.Setenv("TEST_INT", "42")
	if got := envInt("TEST_INT", 7); got != 42 {
		t.Fatalf("envInt = %d, want 42", got)
	}
	t.Setenv("TEST_BAD_INT", "not-a-number")
	if got := envInt("TEST_BAD_INT", 7); got != 7 {
		t.Fatalf("envInt = %d, want the fallback on a bad value", got)
	}
	if got := envInt("TEST_MISSING_INT", 7); got != 7 {
		t.Fatalf("envInt = %d, want the fallback when unset", got)
	}

	if got := envBool("TEST_BOOL", true); got != true {
		t.Fatal("envBool should fall back when unset")
	}
	t.Setenv("TEST_BOOL", "false")
	if got := envBool("TEST_BOOL", true); got != false {
		t.Fatalf("envBool = %v, want false", got)
	}
	t.Setenv("TEST_BOOL", "yes")
	if got := envBool("TEST_BOOL", true); got != true {
		t.Fatalf("envBool = %v, want the fallback on an unparseable value", got)
	}

	if got := envDuration("TEST_MISSING_DURATION", 0); got != 0 {
		t.Fatalf("envDuration = %v, want the zero fallback", got)
	}
	t.Setenv("TEST_DURATION", "3s")
	if got := envDuration("TEST_DURATION", time.Second); got != 3*time.Second {
		t.Fatalf("envDuration = %v, want 3s", got)
	}
	t.Setenv("TEST_DURATION", "0s")
	if got := envDuration("TEST_DURATION", time.Second); got != time.Second {
		t.Fatalf("envDuration = %v, want the fallback on a non-positive value", got)
	}
	t.Setenv("TEST_DURATION", "banana")
	if got := envDuration("TEST_DURATION", time.Second); got != time.Second {
		t.Fatalf("envDuration = %v, want the fallback on a bad value", got)
	}
}

func TestJWTSecretPrefersTheDedicatedVariable(t *testing.T) {
	t.Setenv("JWT_SECRET", "primary")
	t.Setenv("MYSECRETKEY", "legacy")
	secret, err := jwtSecret()
	if err != nil {
		t.Fatalf("jwtSecret: %v", err)
	}
	if secret != "primary" {
		t.Fatalf("secret = %q, want JWT_SECRET to win", secret)
	}

	t.Setenv("JWT_SECRET", "")
	secret, err = jwtSecret()
	if err != nil {
		t.Fatalf("jwtSecret: %v", err)
	}
	if secret != "legacy" {
		t.Fatalf("secret = %q, want MYSECRETKEY as the fallback", secret)
	}

	t.Setenv("MYSECRETKEY", "   ")
	if _, err := jwtSecret(); err == nil {
		t.Fatal("a blank secret pair should fail closed rather than sign with an empty key")
	}
}

func TestDialServicesBuildsOnePoolPerTarget(t *testing.T) {
	t.Setenv("USER_SERVICE_ADDR", "localhost:15051")
	t.Setenv("POST_SERVICE_ADDR", "localhost:15052")
	t.Setenv("FOLLOW_SERVICE_ADDR", "localhost:15053")
	t.Setenv("FEED_SERVICE_ADDR", "localhost:15054")
	t.Setenv("NOTIFICATION_SERVICE_ADDR", "localhost:15055")
	t.Setenv("MODERATION_SERVICE_ADDR", "localhost:15056")
	t.Setenv("CHAT_SERVICE_ADDR", "localhost:15057")
	t.Setenv("GRPC_SUBCHANNELS", "2")

	connections, err := dialServices(slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("dialServices: %v", err)
	}
	defer func() {
		for _, conn := range connections {
			_ = conn.Close()
		}
	}()

	want := []string{
		"user-service", "post-service", "follow-service",
		"feed-service", "notification-service",
		"moderation-service", "chat-service",
	}
	if len(connections) != len(want) {
		t.Fatalf("connections = %d, want one per target", len(connections))
	}
	for _, name := range want {
		pool, ok := connections[name]
		if !ok || pool == nil {
			t.Fatalf("missing pool for %s", name)
		}
		if len(pool.conns) != 2 {
			t.Fatalf("%s subchannels = %d, want 2", name, len(pool.conns))
		}
	}
}

func TestDialRespectsSubchannelCount(t *testing.T) {
	t.Setenv("GRPC_SUBCHANNELS", "3")

	pool, err := dial("localhost:15099", 0, newBreakerSet(0))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = pool.Close() }()

	if len(pool.conns) != 3 {
		t.Fatalf("subchannels = %d, want 3", len(pool.conns))
	}
}

func TestConnPoolCloseJoinsFailuresAndClosesEveryConn(t *testing.T) {
	pool := newConnPool(closeErrConn{}, closeErrConn{})

	err := pool.Close()
	if err == nil {
		t.Fatal("Close should report a failure")
	}
	if !errors.Is(err, errClose) {
		t.Fatalf("Close = %v, want both failures joined", err)
	}
}

var errClose = errors.New("close failed")

type closeErrConn struct{}

func (closeErrConn) Invoke(ctx context.Context, method string, args, reply any, opts ...grpc.CallOption) error {
	return nil
}

func (closeErrConn) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, nil
}

func (closeErrConn) Close() error { return errClose }

func TestConnPoolNewStreamUsesThePool(t *testing.T) {
	var mu sync.Mutex
	var calls []int
	pool := newConnPool(recordingConn{id: 0, mu: &mu, calls: &calls})

	if _, err := pool.NewStream(context.Background(), &grpc.StreamDesc{}, "/test.Service/Stream"); err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("streams = %v, want one call through the pool", calls)
	}
}

func TestGraphqlTracingExtensionMetadata(t *testing.T) {
	extension := graphqlTracing{}

	if extension.ExtensionName() != "OpenTelemetry" {
		t.Fatalf("extension name = %q", extension.ExtensionName())
	}
	if err := extension.Validate(nil); err != nil {
		t.Fatalf("Validate = %v, want nil", err)
	}
}

func TestGraphqlTracingRecordsErrors(t *testing.T) {
	ctx := graphql.WithOperationContext(context.Background(), &graphql.OperationContext{
		OperationName: "GetFeed",
		Operation: &ast.OperationDefinition{
			Operation: ast.Query,
		},
	})

	handler := graphqlTracing{}.InterceptOperation(ctx, func(ctx context.Context) graphql.ResponseHandler {
		return func(ctx context.Context) *graphql.Response {
			return &graphql.Response{Errors: []*gqlerror.Error{gqlerror.Errorf("boom")}}
		}
	})

	response := handler(ctx)
	if response == nil {
		t.Fatal("the response should pass through untouched")
	}
	if len(response.Errors) != 1 {
		t.Fatalf("errors = %d, want 1", len(response.Errors))
	}
}

// A request that fails before a document is parsed reaches this extension
// without an operation context. GetOperationContext panics in that state, so the
// extension has to tolerate it rather than turn a 400 into a 500.
func TestGraphqlTracingToleratesMissingOperationContext(t *testing.T) {
	handler := graphqlTracing{}.InterceptOperation(context.Background(), func(ctx context.Context) graphql.ResponseHandler {
		return func(ctx context.Context) *graphql.Response {
			return &graphql.Response{Errors: []*gqlerror.Error{gqlerror.Errorf("boom")}}
		}
	})

	response := handler(context.Background())
	if response == nil || len(response.Errors) != 1 {
		t.Fatalf("response = %+v, want the errors to survive", response)
	}
}

func TestGraphqlTracingHandlesNilResponse(t *testing.T) {
	handler := graphqlTracing{}.InterceptOperation(context.Background(), func(ctx context.Context) graphql.ResponseHandler {
		return func(ctx context.Context) *graphql.Response { return nil }
	})

	if response := handler(context.Background()); response != nil {
		t.Fatalf("response = %+v, want nil", response)
	}
}
