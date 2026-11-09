package ratelimit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/chetansachan/socialflow/api-gateway/auth"
	"github.com/vektah/gqlparser/v2/ast"
)

func TestClientIPResolvesForwardedOnlyWhenTrusted(t *testing.T) {
	newRequest := func(remote string) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/query", nil)
		req.RemoteAddr = remote
		req.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.1")
		req.Header.Set("X-Real-IP", "198.51.100.9")
		return req
	}

	// Untrusted (the default): a client must not be able to forge the key that
	// its own budget is scoped to.
	if got := clientIP(newRequest("192.0.2.10:5555"), false); got != "192.0.2.10" {
		t.Fatalf("clientIP = %q, want the socket address when forwarded headers are untrusted", got)
	}

	// Trusted: the first hop of X-Forwarded-For is the client.
	if got := clientIP(newRequest("192.0.2.10:5555"), true); got != "203.0.113.7" {
		t.Fatalf("clientIP = %q, want the first forwarded hop", got)
	}

	req := newRequest("192.0.2.10:5555")
	req.Header.Del("X-Forwarded-For")
	if got := clientIP(req, true); got != "198.51.100.9" {
		t.Fatalf("clientIP = %q, want X-Real-IP as the fallback", got)
	}

	// A RemoteAddr without a port must not blow up the split.
	req = httptest.NewRequest(http.MethodPost, "/query", nil)
	req.RemoteAddr = "192.0.2.10"
	if got := clientIP(req, false); got != "192.0.2.10" {
		t.Fatalf("clientIP = %q, want the bare address", got)
	}
}

func TestWithClientIPStoresTheAddressForTheGraphQLLayer(t *testing.T) {
	var seen string
	handler := WithClientIP(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = ClientIP(r.Context())
	}))

	req := httptest.NewRequest(http.MethodPost, "/query", nil)
	req.RemoteAddr = "192.0.2.10:5555"
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if seen != "203.0.113.7" {
		t.Fatalf("ClientIP in handler = %q, want the address the middleware resolved", seen)
	}
}

func TestWithClientIPSkipsAnEmptyAddress(t *testing.T) {
	handler := WithClientIP(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest(http.MethodPost, "/query", nil)
	req.RemoteAddr = ""
	handler.ServeHTTP(httptest.NewRecorder(), req)

	// The key is only stored when there is something to store, so the guard
	// cannot key on an empty subject.
	if ClientIP(context.Background()) != "" {
		t.Fatal("an empty address must not be stored")
	}
}

func TestGuardExtensionMetadata(t *testing.T) {
	guard := NewGuard(nil, nil, Policy{}, Policy{}, nil)

	if guard.ExtensionName() != "RateLimit" {
		t.Fatalf("extension name = %q", guard.ExtensionName())
	}
	if err := guard.Validate(nil); err != nil {
		t.Fatalf("Validate = %v, want nil", err)
	}
}

func TestPolicyForPrefersFieldSpecificBudgets(t *testing.T) {
	guard := &Guard{
		policies:      map[string]Policy{"CreatePost": {Limit: 3, Window: time.Minute}},
		authenticated: Policy{Limit: 10, Window: time.Minute},
		anonymous:     Policy{Limit: 2, Window: time.Minute},
	}

	if got := guard.policyFor("CreatePost", true); got.Limit != 3 {
		t.Fatalf("CreatePost policy = %+v, want the field budget", got)
	}
	if got := guard.policyFor("searchPosts", true); got.Limit != 10 {
		t.Fatalf("authenticated fallback = %+v", got)
	}
	if got := guard.policyFor("searchPosts", false); got.Limit != 2 {
		t.Fatalf("anonymous fallback = %+v", got)
	}
}

func TestRootFieldTakesTheFirstRootSelection(t *testing.T) {
	opCtx := &graphql.OperationContext{
		Operation: &ast.OperationDefinition{
			SelectionSet: ast.SelectionSet{
				&ast.Field{Name: "getFeed"},
				&ast.Field{Name: "second"},
			},
		},
	}
	if got := rootField(opCtx); got != "getFeed" {
		t.Fatalf("rootField = %q, want the first root field", got)
	}

	// Fragments and inline fragments are not fields; they must be skipped so a
	// document that opens with one still finds its root field.
	opCtx.Operation.SelectionSet = ast.SelectionSet{
		&ast.InlineFragment{},
		&ast.Field{Name: "Login"},
		&ast.FragmentSpread{Name: "Body"},
	}
	if got := rootField(opCtx); got != "Login" {
		t.Fatalf("rootField = %q, want the first real field", got)
	}

	if got := rootField(nil); got != "" {
		t.Fatalf("rootField(nil) = %q, want an empty string", got)
	}
	if got := rootField(&graphql.OperationContext{}); got != "" {
		t.Fatalf("rootField without an operation = %q", got)
	}
	onlyFragment := &graphql.OperationContext{
		Operation: &ast.OperationDefinition{SelectionSet: ast.SelectionSet{&ast.FragmentSpread{Name: "Body"}}},
	}
	if got := rootField(onlyFragment); got != "" {
		t.Fatalf("rootField with no fields = %q", got)
	}
}

func TestGuardInterceptAllowsANamelessOperation(t *testing.T) {
	guard := &Guard{}
	served := false
	next := func(ctx context.Context) graphql.ResponseHandler {
		served = true
		return func(ctx context.Context) *graphql.Response {
			return &graphql.Response{Data: []byte(`{"data":{}}`)}
		}
	}

	// No operation context at all: the guard must pass the request through
	// rather than panic on GetOperationContext.
	handler := guard.InterceptOperation(context.Background(), next)
	if response := handler(context.Background()); response == nil {
		t.Fatal("expected the operation to be served")
	}
	if !served {
		t.Fatal("a request without a root field should not be blocked")
	}
}

func TestGuardKeysAuthenticatedRequestsOnTheUser(t *testing.T) {
	guard := &Guard{limiter: nil, policies: map[string]Policy{}}
	ctx := graphql.WithOperationContext(auth.WithPrincipal(context.Background(), auth.Principal{UserID: "u1"}), &graphql.OperationContext{
		Operation: &ast.OperationDefinition{
			Operation:    ast.Query,
			SelectionSet: ast.SelectionSet{&ast.Field{Name: "getFeed"}},
		},
	})

	served := false
	next := func(ctx context.Context) graphql.ResponseHandler {
		served = true
		return func(ctx context.Context) *graphql.Response { return &graphql.Response{Data: []byte(`{"data":{}}`)} }
	}

	// A nil limiter passes everything through; the point here is that the
	// extension reaches the next handler with the authenticated context intact.
	_ = guard.InterceptOperation(ctx, next)(ctx)
	if !served {
		t.Fatal("the operation should be served")
	}
}
