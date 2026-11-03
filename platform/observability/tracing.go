package observability

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// TracingConfig describes where spans should be sent.
type TracingConfig struct {
	// ServiceName becomes the service.name resource attribute.
	ServiceName string
	// Endpoint is the OTLP gRPC collector address, for example
	// "otel-collector:4317". Empty disables export but still enables
	// trace-context propagation between services.
	Endpoint string
	// Insecure skips TLS; use it for a collector on a private network or a
	// local Jaeger/Tempo all-in-one container.
	Insecure bool
	// SampleRatio is the fraction of root traces to record. 1 records every
	// trace; 0 still honors a parent's sampling decision.
	SampleRatio float64
}

// SetupTracing installs a global tracer provider and the W3C trace-context
// propagator. The returned shutdown function flushes buffered spans and must be
// called during graceful shutdown.
func SetupTracing(ctx context.Context, cfg TracingConfig) (func(context.Context) error, error) {
	// Always install propagation, even without a collector: a downstream
	// service can still continue the trace.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	if cfg.Endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}

	options := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(cfg.Endpoint)}
	if cfg.Insecure {
		options = append(options, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptracegrpc.New(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
	}

	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))
	if cfg.SampleRatio >= 1 {
		sampler = sdktrace.ParentBased(sdktrace.AlwaysSample())
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sampler),
		sdktrace.WithResource(newResource(cfg.ServiceName)),
	)
	otel.SetTracerProvider(provider)

	return provider.Shutdown, nil
}

// TracingConfigFromEnv reads the standard OTLP variables used by every
// service:
//
//	OTEL_EXPORTER_OTLP_ENDPOINT   collector host:port (scheme is stripped)
//	OTEL_EXPORTER_OTLP_INSECURE   "false" enables TLS (default true)
//	OTEL_TRACES_SAMPLE_RATIO      0..1, default 0.1
func TracingConfigFromEnv(serviceName string) TracingConfig {
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	endpoint = strings.TrimPrefix(endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimSuffix(endpoint, "/")

	insecure := true
	if raw := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_INSECURE")); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			insecure = parsed
		}
	}

	ratio := 0.1
	if raw := strings.TrimSpace(os.Getenv("OTEL_TRACES_SAMPLE_RATIO")); raw != "" {
		if parsed, err := strconv.ParseFloat(raw, 64); err == nil && parsed >= 0 && parsed <= 1 {
			ratio = parsed
		}
	}

	return TracingConfig{
		ServiceName: serviceName,
		Endpoint:    endpoint,
		Insecure:    insecure,
		SampleRatio: ratio,
	}
}

// Tracer returns a tracer scoped to this module. Services may call it directly
// or rely on otel.Tracer.
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}

func newResource(serviceName string) *resource.Resource {
	attributes := []attribute.KeyValue{
		attribute.String("service.name", serviceName),
		attribute.String("service.namespace", "socialflow"),
	}
	return resource.NewSchemaless(attributes...)
}
