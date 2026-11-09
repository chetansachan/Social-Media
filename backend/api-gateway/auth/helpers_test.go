package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vektah/gqlparser/v2/gqlerror"
	"google.golang.org/grpc/metadata"
)

func TestBearerTokenAcceptsWithAndWithoutTheScheme(t *testing.T) {
	cases := map[string]struct {
		header string
		want   string
	}{
		"lowercase scheme":  {header: "bearer abc", want: "abc"},
		"mixed case scheme": {header: "BeArEr abc", want: "abc"},
		"no scheme":         {header: "abc", want: "abc"},
		"extra whitespace":  {header: "  bearer   abc  ", want: "abc"},
		"empty header":      {header: "", want: ""},
		"spaces only":       {header: "   ", want: ""},
		// A scheme we do not recognise is passed through whole: dropping the
		// first word would silently mangle a token that contains a space.
		"unknown scheme": {header: "Basic dXNlcjpwYXNz", want: "Basic dXNlcjpwYXNz"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := bearerToken(tc.header); got != tc.want {
				t.Fatalf("bearerToken(%q) = %q, want %q", tc.header, got, tc.want)
			}
		})
	}
}

func TestAuthDirectiveRejectsAnonymousCallers(t *testing.T) {
	served := false
	next := func(ctx context.Context) (any, error) {
		served = true
		return "payload", nil
	}

	if _, err := AuthDirective(context.Background(), nil, next); err == nil {
		t.Fatal("the @auth directive must reject an anonymous caller")
	} else {
		gqlErr, ok := err.(*gqlerror.Error)
		if !ok || gqlErr.Extensions["code"] != "UNAUTHENTICATED" {
			t.Fatalf("error = %#v, want an UNAUTHENTICATED GraphQL error", err)
		}
	}
	if served {
		t.Fatal("the field resolver should not run for an anonymous caller")
	}

	ctx := WithPrincipal(context.Background(), Principal{UserID: "u1"})
	payload, err := AuthDirective(ctx, nil, next)
	if err != nil {
		t.Fatalf("authenticated call: %v", err)
	}
	if payload != "payload" || !served {
		t.Fatalf("payload = %v, served = %v", payload, served)
	}
}

func TestFirstStoreSkipsMissingStores(t *testing.T) {
	one := &fakeStore{}
	two := &fakeStore{}

	if got := firstStore([]RevocationStore{nil, nil, one, two}); got != RevocationStore(one) {
		t.Fatalf("firstStore = %v, want the first non-nil store", got)
	}
	if got := firstStore(nil); got != nil {
		t.Fatal("an empty list has no store")
	}
	if got := firstStore([]RevocationStore{nil}); got != nil {
		t.Fatal("a list of nils has no store")
	}
}

type fakeStore struct{ RevocationStore }

func TestTokenFromMetadataReadsTheBearerHeader(t *testing.T) {
	if got := TokenFromMetadata(context.Background()); got != "" {
		t.Fatalf("TokenFromMetadata = %q, want empty without metadata", got)
	}

	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer abc"))
	if got := TokenFromMetadata(ctx); got != "abc" {
		t.Fatalf("TokenFromMetadata = %q, want abc", got)
	}

	ctx = metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "raw"))
	if got := TokenFromMetadata(ctx); got != "raw" {
		t.Fatalf("TokenFromMetadata = %q, want the raw header", got)
	}
}

func TestRevocationKeyIsNamespacedPerToken(t *testing.T) {
	if got := revocationKey("jti-1"); got != "token:blacklist:jti-1" {
		t.Fatalf("revocationKey = %q", got)
	}
	if revocationKey("a") == revocationKey("b") {
		t.Fatal("different tokens must not share a key")
	}
}

func TestWriteAuthErrorEmitsGraphQLShapedJSON(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeAuthError(recorder, http.StatusUnauthorized, "TOKEN_REVOKED", "session revoked")

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("content type = %q", contentType)
	}

	var payload struct {
		Errors []struct {
			Message    string         `json:"message"`
			Extensions map[string]any `json:"extensions"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("body %q is not valid JSON: %v", recorder.Body.String(), err)
	}
	if len(payload.Errors) != 1 {
		t.Fatalf("errors = %d, want 1", len(payload.Errors))
	}
	if payload.Errors[0].Message != "session revoked" {
		t.Fatalf("message = %q", payload.Errors[0].Message)
	}
	if payload.Errors[0].Extensions["code"] != "TOKEN_REVOKED" {
		t.Fatalf("extensions = %v", payload.Errors[0].Extensions)
	}
}
