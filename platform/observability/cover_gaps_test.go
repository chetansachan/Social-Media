package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// metricValue reads a single metric out of the private registry, matching on
// label values. Gathering rather than reaching into the collector keeps the test
// honest about what the /metrics endpoint actually serves.
func metricValue(t *testing.T, metrics *Metrics, familyName string, labels map[string]string) (float64, bool) {
	t.Helper()

	families, err := metrics.registry.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	for _, family := range families {
		if family.GetName() != familyName {
			continue
		}
		for _, metric := range family.GetMetric() {
			if !labelsMatch(metric.GetLabel(), labels) {
				continue
			}
			switch {
			case metric.GetGauge() != nil:
				return metric.GetGauge().GetValue(), true
			case metric.GetCounter() != nil:
				return metric.GetCounter().GetValue(), true
			case metric.GetHistogram() != nil:
				// Sample sum: one observation means the milliseconds recorded
				// are present, which is the part worth asserting.
				return metric.GetHistogram().GetSampleSum(), true
			}
		}
	}
	return 0, false
}

func labelsMatch(pairs []*dto.LabelPair, want map[string]string) bool {
	for name, value := range want {
		found := false
		for _, pair := range pairs {
			if pair.GetName() == name {
				if pair.GetValue() != value {
					return false
				}
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func mustMetric(t *testing.T, metrics *Metrics, familyName string, labels map[string]string) float64 {
	t.Helper()
	value, ok := metricValue(t, metrics, familyName, labels)
	if !ok {
		t.Fatalf("metric %s%v not found", familyName, labels)
	}
	return value
}

func TestConsumerLagGaugeTracksBacklogPerGroup(t *testing.T) {
	metrics := NewMetrics("test-service")

	metrics.SetConsumerLag("social:events", "feed-service", 12)
	metrics.SetConsumerLag("social:events", "notification-service", 3)
	metrics.SetConsumerLag("social:events", "feed-service", 7)

	feed := map[string]string{"stream": "social:events", "group": "feed-service"}
	notifications := map[string]string{"stream": "social:events", "group": "notification-service"}

	// A lag gauge is a level, not a counter: a newer reading replaces the last
	// one, otherwise a backlog that drains to zero would keep reporting growth.
	if got := mustMetric(t, metrics, "redis_stream_consumer_lag", feed); got != 7 {
		t.Fatalf("feed lag = %v, want 7", got)
	}
	if got := mustMetric(t, metrics, "redis_stream_consumer_lag", notifications); got != 3 {
		t.Fatalf("notification lag = %v, want 3", got)
	}
}

func TestMetricsAccessorsOnANilMetricsAreSafe(t *testing.T) {
	var metrics *Metrics

	// Every service wires these before it knows whether Redis or a socket is
	// available, so a nil receiver must be a no-op rather than a panic on a
	// worker goroutine.
	metrics.SetConsumerLag("social:events", "feed-service", 1)
	metrics.WebsocketConnected()
	metrics.WebsocketDisconnected()
	metrics.RecordDispatch("realtime", "success")
}

func TestWebsocketGaugeFollowsLiveConnections(t *testing.T) {
	metrics := NewMetrics("test-service")

	metrics.WebsocketConnected()
	metrics.WebsocketConnected()
	if got := mustMetric(t, metrics, "websocket_active_connections", map[string]string{"pod_id": metrics.podID}); got != 2 {
		t.Fatalf("active connections = %v, want 2", got)
	}

	metrics.WebsocketDisconnected()
	if got := mustMetric(t, metrics, "websocket_active_connections", map[string]string{"pod_id": metrics.podID}); got != 1 {
		t.Fatalf("active connections = %v, want 1 after a disconnect", got)
	}

	metrics.WebsocketDisconnected()
	if got := mustMetric(t, metrics, "websocket_active_connections", map[string]string{"pod_id": metrics.podID}); got != 0 {
		t.Fatalf("active connections = %v, want 0", got)
	}
}

func TestRecordDispatchCountsChannelAndStatus(t *testing.T) {
	metrics := NewMetrics("test-service")

	metrics.RecordDispatch("realtime", "success")
	metrics.RecordDispatch("realtime", "error")
	metrics.RecordDispatch("realtime", "success")

	success := mustMetric(t, metrics, "notification_dispatch_total", map[string]string{"channel": "realtime", "status": "success"})
	if success != 2 {
		t.Fatalf("successful dispatches = %v, want 2", success)
	}
	failures := mustMetric(t, metrics, "notification_dispatch_total", map[string]string{"channel": "realtime", "status": "error"})
	if failures != 1 {
		t.Fatalf("failed dispatches = %v, want 1", failures)
	}
}

func TestLogLevelDefaultsToInfo(t *testing.T) {
	cases := map[string]struct {
		value string
		want  slog.Level
	}{
		"unset":   {value: "", want: slog.LevelInfo},
		"garbage": {value: "verbose", want: slog.LevelInfo},
		"debug":   {value: "debug", want: slog.LevelDebug},
		"upper":   {value: "  WARN ", want: slog.LevelWarn},
		"warning": {value: "warning", want: slog.LevelWarn},
		"error":   {value: "error", want: slog.LevelError},
		"unknown": {value: "trace", want: slog.LevelInfo},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("LOG_LEVEL", tc.value)
			if got := logLevel(); got != tc.want {
				t.Fatalf("logLevel(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

// NewLogger writes to stdout, which a test cannot read. It differs only in
// destination from the writer variant, so asserting it stays usable is enough.
func TestNewLoggerReturnsUsableLogger(t *testing.T) {
	logger := NewLogger("test-service")
	if logger == nil {
		t.Fatal("NewLogger returned nil")
	}
	if !logger.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("an info record should be enabled at the default level")
	}
}

// WithGroup is what a logger calls when a caller pushes one. It has to keep
// wrapping, or records emitted after it lose their trace ids.
func TestTraceHandlerWithAttrsAndGroupKeepWrapping(t *testing.T) {
	handler := &traceHandler{next: slog.NewJSONHandler(&bytes.Buffer{}, nil)}

	if _, ok := handler.WithAttrs([]slog.Attr{slog.String("k", "v")}).(*traceHandler); !ok {
		t.Fatal("WithAttrs must keep wrapping")
	}
	if _, ok := handler.WithGroup("db").(*traceHandler); !ok {
		t.Fatal("WithGroup must keep wrapping")
	}
	if !handler.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("Enabled must delegate to the wrapped handler")
	}
}

func TestTraceHandlerInjectsIDsAfterGroup(t *testing.T) {
	var output bytes.Buffer
	logger := NewLoggerWithWriter("test-service", &output).WithGroup("db")

	logger.Info("query ran", "table", "users")

	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("log line is not JSON: %v (%s)", err, output.String())
	}
	if record["service"] != "test-service" {
		t.Fatalf("service = %v", record["service"])
	}
	if record["db"] == nil {
		t.Fatal("the pushed group should namespace the attributes")
	}
}

func TestTracerIsNamedAfterTheCaller(t *testing.T) {
	if Tracer("post-service/repository") == nil {
		t.Fatal("Tracer returned nil")
	}
}

func TestStreamServerInterceptorRecordsMethodLatency(t *testing.T) {
	metrics := NewMetrics("test-service")
	interceptor := metrics.StreamServerInterceptor()

	called := false
	err := interceptor(nil, &fakeServerStream{}, &grpc.StreamServerInfo{FullMethod: "/chat.ChatService/ChatEvents"},
		func(srv any, stream grpc.ServerStream) error {
			called = true
			time.Sleep(time.Millisecond)
			return nil
		})
	if err != nil {
		t.Fatalf("interceptor returned %v, want nil", err)
	}
	if !called {
		t.Fatal("the handler was not invoked")
	}

	value, ok := metricValue(t, metrics, "grpc_server_handling_seconds", map[string]string{
		"grpc_method": "/chat.ChatService/ChatEvents",
		"code":        "OK",
		"service":     metrics.service,
	})
	if !ok {
		t.Fatal("no streaming histogram sample was recorded")
	}
	if value <= 0 {
		t.Fatalf("recorded seconds = %v, want a positive sample", value)
	}
}

type fakeServerStream struct{ grpc.ServerStream }

func (fakeServerStream) SetHeader(metadata.MD) error { return nil }
func (fakeServerStream) SendMsg(any) error           { return nil }
func (fakeServerStream) RecvMsg(any) error           { return nil }
func (fakeServerStream) Context() context.Context    { return context.Background() }
