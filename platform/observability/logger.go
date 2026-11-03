package observability

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

// NewLogger returns a JSON logger writing to stdout with a service attribute.
// Every record logged with a context that carries a span also gets trace_id
// and span_id, which is what lets Loki or Elasticsearch join logs to traces.
func NewLogger(service string) *slog.Logger {
	return NewLoggerWithWriter(service, os.Stdout)
}

// NewLoggerWithWriter is NewLogger with an explicit destination, used by tests
// and by commands that need to capture output.
func NewLoggerWithWriter(service string, output io.Writer) *slog.Logger {
	handler := slog.NewJSONHandler(output, &slog.HandlerOptions{
		Level: logLevel(),
	})
	return slog.New(&traceHandler{next: handler}).With(slog.String("service", service))
}

func logLevel() slog.Level {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL"))) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// traceHandler injects the active trace identifiers into each record. It wraps
// another handler so JSON formatting and level filtering stay in one place.
type traceHandler struct {
	next slog.Handler
}

func (h *traceHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *traceHandler) Handle(ctx context.Context, record slog.Record) error {
	if span := trace.SpanContextFromContext(ctx); span.IsValid() {
		record.AddAttrs(
			slog.String("trace_id", span.TraceID().String()),
			slog.String("span_id", span.SpanID().String()),
		)
	}
	return h.next.Handle(ctx, record)
}

func (h *traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceHandler{next: h.next.WithAttrs(attrs)}
}

func (h *traceHandler) WithGroup(name string) slog.Handler {
	return &traceHandler{next: h.next.WithGroup(name)}
}
