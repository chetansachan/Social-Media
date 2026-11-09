// Package observability holds the cross-cutting production concerns shared by
// every SocialFlow service: structured logging, OpenTelemetry tracing,
// Prometheus metrics, and health/readiness endpoints.
//
// It is a separate module so each service can depend on one implementation
// instead of copying instrumentation code.
package observability
