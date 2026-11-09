// Package auth verifies the access tokens issued by user-service and carries
// the resulting identity through HTTP, GraphQL, and gRPC boundaries.
package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/golang-jwt/jwt/v5"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// Claims mirrors the token issued by user-service. Role is optional so older
// tokens remain valid; authorization decisions treat an empty role as "user".
type Claims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role,omitempty"`
	jwt.RegisteredClaims
}

// Principal is the authenticated caller attached to a request context.
type Principal struct {
	UserID string
	Email  string
	Role   string
	Token  string

	// TokenID is the token's jti claim. It is the handle used to revoke this
	// exact access token on logout, and ExpiresAt is when it would otherwise
	// stop being accepted, which bounds how long the revocation must live.
	TokenID   string
	ExpiresAt time.Time
}

type principalKey struct{}

// WithPrincipal stores the authenticated caller in the context.
func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, principal)
}

// FromContext returns the authenticated caller, if the request carried a valid
// token.
func FromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalKey{}).(Principal)
	return principal, ok && principal.UserID != ""
}

// ParseToken validates an HS256 access token and returns its principal.
func ParseToken(raw, secret string) (Principal, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Principal{}, errors.New("missing token")
	}

	claims := &Claims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	}, jwt.WithExpirationRequired())
	if err != nil {
		return Principal{}, fmt.Errorf("invalid token: %w", err)
	}
	if !token.Valid {
		return Principal{}, errors.New("invalid token")
	}
	if strings.TrimSpace(claims.UserID) == "" {
		return Principal{}, errors.New("token has no user_id claim")
	}

	role := strings.TrimSpace(claims.Role)
	if role == "" {
		role = "user"
	}

	principal := Principal{
		UserID:  claims.UserID,
		Email:   claims.Email,
		Role:    role,
		Token:   raw,
		TokenID: strings.TrimSpace(claims.ID),
	}
	if claims.ExpiresAt != nil {
		principal.ExpiresAt = claims.ExpiresAt.Time
	}

	return principal, nil
}

// Middleware extracts a bearer token from the Authorization header (or the
// token query parameter used by WebSocket clients) and attaches the principal.
// Requests without a token continue as anonymous; the @auth GraphQL directive
// rejects them where authentication is required.
func Middleware(secret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r.Header.Get("Authorization"))
		if token == "" {
			token = strings.TrimSpace(r.URL.Query().Get("token"))
		}

		if token != "" {
			principal, err := ParseToken(token, secret)
			if err != nil {
				writeAuthError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "unauthorized")
				return
			}
			r = r.WithContext(WithPrincipal(r.Context(), principal))
		}

		next.ServeHTTP(w, r)
	})
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
		return strings.TrimSpace(parts[1])
	}
	return header
}

// Require returns the authenticated caller or a GraphQL UNAUTHENTICATED error.
func Require(ctx context.Context) (Principal, error) {
	principal, ok := FromContext(ctx)
	if !ok {
		return Principal{}, &gqlerror.Error{
			Message:    "authentication required",
			Extensions: map[string]any{"code": "UNAUTHENTICATED"},
		}
	}
	return principal, nil
}

// AuthDirective implements the schema's @auth field middleware.
func AuthDirective(ctx context.Context, _ any, next graphql.Resolver) (any, error) {
	if _, ok := FromContext(ctx); !ok {
		return nil, &gqlerror.Error{
			Message:    "authentication required",
			Extensions: map[string]any{"code": "UNAUTHENTICATED"},
		}
	}
	return next(ctx)
}

// UnaryClientInterceptor forwards identity and the raw bearer token to
// downstream microservices through gRPC metadata.
func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		if principal, ok := FromContext(ctx); ok {
			ctx = metadata.AppendToOutgoingContext(ctx,
				"authorization", "Bearer "+principal.Token,
				"x-user-id", principal.UserID,
				"x-user-email", principal.Email,
				"x-user-role", principal.Role,
			)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// WebsocketInit parses the connection_init payload used by GraphQL
// subscriptions. Missing tokens are allowed here so the @auth directive can
// produce a protocol-level UNAUTHENTICATED error on the subscription field.
// When a revocation store is supplied, a logged-out token is refused during the
// handshake instead of opening a stream.
func WebsocketInit(secret string, revocations ...RevocationStore) transport.WebsocketInitFunc {
	store := firstStore(revocations)

	return func(ctx context.Context, payload transport.InitPayload) (context.Context, *transport.InitPayload, error) {
		raw := payload.Authorization()
		if raw == "" {
			raw = payload.GetString("token")
		}
		raw = bearerToken(raw)
		if raw == "" {
			return ctx, nil, nil
		}

		principal, err := ParseToken(raw, secret)
		if err != nil {
			return ctx, nil, err
		}

		if store != nil {
			revoked, err := store.IsRevoked(ctx, principal.TokenID)
			if err != nil {
				return ctx, nil, fmt.Errorf("could not verify the session: %w", err)
			}
			if revoked {
				return ctx, nil, errors.New("session has been revoked")
			}
		}

		return WithPrincipal(ctx, principal), nil, nil
	}
}

func firstStore(stores []RevocationStore) RevocationStore {
	for _, store := range stores {
		if store != nil {
			return store
		}
	}
	return nil
}

// TokenFromMetadata extracts a bearer token forwarded by an upstream caller.
func TokenFromMetadata(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get("authorization")
	if len(values) == 0 {
		return ""
	}
	return bearerToken(values[0])
}
