package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestLoggerWritesServiceAndTraceID(t *testing.T) {
	var output bytes.Buffer
	logger := NewLoggerWithWriter("user-service", &output)

	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatalf("TraceIDFromHex: %v", err)
	}
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatalf("SpanIDFromHex: %v", err)
	}
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	}))

	logger.InfoContext(ctx, "profile loaded", "user_id", "u1", "latency_ms", 12)

	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("decode log line: %v (%s)", err, output.String())
	}
	if record["service"] != "user-service" {
		t.Fatalf("service = %v", record["service"])
	}
	if record["trace_id"] != traceID.String() {
		t.Fatalf("trace_id = %v, want %s", record["trace_id"], traceID)
	}
	if record["span_id"] != spanID.String() {
		t.Fatalf("span_id = %v, want %s", record["span_id"], spanID)
	}
	if record["user_id"] != "u1" {
		t.Fatalf("user_id = %v", record["user_id"])
	}
}

func TestLoggerWithoutSpanOmitsTraceFields(t *testing.T) {
	var output bytes.Buffer
	logger := NewLoggerWithWriter("user-service", &output)

	logger.InfoContext(context.Background(), "no span")

	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("decode log line: %v", err)
	}
	if _, ok := record["trace_id"]; ok {
		t.Fatal("trace_id was added without an active span")
	}
}
