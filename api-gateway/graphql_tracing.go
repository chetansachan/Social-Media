package main

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// graphqlTracing opens one span per GraphQL operation. The HTTP span already
// covers the transport; this span adds the operation name and type, which is
// what makes a trace readable when a single document fans out to several gRPC
// backends.
type graphqlTracing struct{}

func (graphqlTracing) ExtensionName() string { return "OpenTelemetry" }

func (graphqlTracing) Validate(graphql.ExecutableSchema) error { return nil }

func (graphqlTracing) InterceptOperation(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
	operationType := ""
	operationName := ""
	// HasOperationContext is the non-panicking guard. GetOperationContext
	// panics by design, and it is reachable without one: the transport
	// handlers that build this context run after some extension hooks.
	if graphql.HasOperationContext(ctx) {
		opCtx := graphql.GetOperationContext(ctx)
		operationName = opCtx.OperationName
		if opCtx.Operation != nil {
			operationType = string(opCtx.Operation.Operation)
		}
	}

	// The operation name is user-supplied, so it is bounded by the query
	// string, not by a metric label. The span name follows the OpenTelemetry
	// GraphQL convention: graphql.<name> or graphql.<type>.
	spanName := "graphql." + operationType
	if operationName != "" {
		spanName = "graphql." + operationName
	}

	ctx, span := otel.Tracer("api-gateway/graphql").Start(ctx, spanName,
		trace.WithAttributes(
			attribute.String("graphql.operation.type", operationType),
			attribute.String("graphql.operation.name", operationName),
		),
	)
	handler := next(ctx)

	return func(ctx context.Context) *graphql.Response {
		response := handler(ctx)
		if response == nil {
			span.End()
			return nil
		}
		if len(response.Errors) > 0 {
			for _, graphErr := range response.Errors {
				span.RecordError(graphErr)
			}
			span.SetStatus(codes.Error, response.Errors.Error())
		}
		return response
	}
}
