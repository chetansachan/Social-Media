package ratelimit

import (
	"context"
	"log/slog"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/chetansachan/socialflow/api-gateway/auth"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// Guard is a gqlgen operation extension that spends one unit of a per-field
// budget before the operation executes. It keys on the root schema field, not
// on the client-supplied operation name, so renaming a mutation cannot borrow
// another field's allowance.
type Guard struct {
	limiter       *Limiter
	policies      map[string]Policy
	authenticated Policy
	anonymous     Policy
	logger        *slog.Logger
}

func NewGuard(limiter *Limiter, policies map[string]Policy, authenticated, anonymous Policy, logger *slog.Logger) *Guard {
	if logger == nil {
		logger = slog.Default()
	}
	return &Guard{
		limiter:       limiter,
		policies:      policies,
		authenticated: authenticated,
		anonymous:     anonymous,
		logger:        logger,
	}
}

func (g *Guard) ExtensionName() string { return "RateLimit" }

func (g *Guard) Validate(graphql.ExecutableSchema) error { return nil }

func (g *Guard) InterceptOperation(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
	if g == nil || g.limiter == nil {
		return next(ctx)
	}

	// HasOperationContext rather than a nil check: the getter panics by design,
	// and a request that fails before parsing never carries one. rootField
	// already treats a missing context as "no root field".
	var field string
	if graphql.HasOperationContext(ctx) {
		field = rootField(graphql.GetOperationContext(ctx))
	}
	if field == "" {
		return next(ctx)
	}

	principal, authenticated := auth.FromContext(ctx)
	subject := ClientIP(ctx)
	if authenticated && principal.UserID != "" {
		subject = "user:" + principal.UserID
	} else if subject != "" {
		subject = "ip:" + subject
	}

	policy := g.policyFor(field, authenticated)
	allowed, err := g.limiter.Allow(ctx, field, subject, policy, time.Now())
	if err != nil {
		// Rate limiting protects capacity; it must not become an outage of its
		// own when Redis hiccups. Session revocation still fails closed.
		g.logger.Warn("rate limiter unavailable; allowing request", "field", field, "error", err)
		return next(ctx)
	}
	if allowed {
		return next(ctx)
	}

	g.logger.Info("rate limit exceeded", "field", field, "subject", subject, "limit", policy.Limit, "window", policy.Window)
	return func(context.Context) *graphql.Response {
		return &graphql.Response{
			Errors: gqlerror.List{{
				Message: "rate limit exceeded; retry later",
				Extensions: map[string]any{
					"code":  "RATE_LIMITED",
					"field": field,
				},
			}},
		}
	}
}

func (g *Guard) policyFor(field string, authenticated bool) Policy {
	if policy, ok := g.policies[field]; ok {
		return policy
	}
	if authenticated {
		return g.authenticated
	}
	return g.anonymous
}

func rootField(opCtx *graphql.OperationContext) string {
	if opCtx == nil || opCtx.Operation == nil {
		return ""
	}
	for _, selection := range opCtx.Operation.SelectionSet {
		if field, ok := selection.(*ast.Field); ok {
			return field.Name
		}
	}
	return ""
}
