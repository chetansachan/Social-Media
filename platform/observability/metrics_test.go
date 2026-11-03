package observability

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func gatherMetric(t *testing.T, registry *prometheus.Registry, name string) *dto.MetricFamily {
	t.Helper()

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, family := range families {
		if family.GetName() == name {
			return family
		}
	}
	t.Fatalf("metric %q was not registered", name)
	return nil
}

func TestMetricsHandlerExposesRuntimeCollectors(t *testing.T) {
	metrics := NewMetrics("api-gateway")
	// Histograms only appear once they have a sample.
	metrics.grpcHandling.WithLabelValues("api-gateway", "/test", "OK").Observe(0.01)

	recorder := httptest.NewRecorder()

	metrics.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "go_goroutines") {
		t.Fatal("Go runtime metrics were not exposed")
	}
	if !strings.Contains(body, "grpc_server_handling_seconds") {
		t.Fatal("gRPC latency metric was not exposed")
	}
}

func TestUnaryInterceptorRecordsCodeAndLatency(t *testing.T) {
	metrics := NewMetrics("post-service")
	interceptor := metrics.UnaryServerInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/post.PostService/GetPost"}

	if _, err := interceptor(context.Background(), nil, info, func(context.Context, any) (any, error) {
		return "ok", nil
	}); err != nil {
		t.Fatalf("success handler: %v", err)
	}
	if _, err := interceptor(context.Background(), nil, info, func(context.Context, any) (any, error) {
		return nil, status.Error(codes.NotFound, "missing")
	}); err == nil {
		t.Fatal("expected the handler error to be returned")
	}

	family := gatherMetric(t, metrics.registry, "grpc_server_handling_seconds")
	var samples uint64
	for _, metric := range family.GetMetric() {
		samples += metric.GetHistogram().GetSampleCount()
	}
	if samples != 2 {
		t.Fatalf("sample count = %d, want 2", samples)
	}
}

func TestHTTPMiddlewareUsesRouteAndStatus(t *testing.T) {
	metrics := NewMetrics("api-gateway")
	handler := metrics.HTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/query", nil))

	family := gatherMetric(t, metrics.registry, "http_requests_total")
	labels := map[string]string{}
	for _, label := range family.GetMetric()[0].GetLabel() {
		labels[label.GetName()] = label.GetValue()
	}
	if labels["status"] != "404" {
		t.Fatalf("status label = %q", labels["status"])
	}
	if labels["handler"] != "/query" {
		t.Fatalf("handler label = %q", labels["handler"])
	}
	if labels["service"] != "api-gateway" {
		t.Fatalf("service label = %q", labels["service"])
	}
}

func TestDBPoolGaugeUsesClosure(t *testing.T) {
	metrics := NewMetrics("post-service")
	metrics.RegisterDBPool("writer", func() int32 { return 7 })
	// Registering the same pool twice must be a no-op, not a panic.
	metrics.RegisterDBPool("writer", func() int32 { return 99 })

	family := gatherMetric(t, metrics.registry, "db_pool_connections_active")
	if got := family.GetMetric()[0].GetGauge().GetValue(); got != 7 {
		t.Fatalf("pool gauge = %v, want 7", got)
	}
}

type hijackableWriter struct {
	*httptest.ResponseRecorder
}

func (w hijackableWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, errors.New("not used")
}

func TestStatusRecorderPreservesHijacking(t *testing.T) {
	recorder := &statusRecorder{ResponseWriter: hijackableWriter{ResponseRecorder: httptest.NewRecorder()}}

	if _, _, err := recorder.Hijack(); err == nil {
		t.Fatal("expected the delegated hijacker to be called")
	}
}

func TestStatusRecorderRejectsHijackingWhenUnsupported(t *testing.T) {
	recorder := &statusRecorder{ResponseWriter: httptest.NewRecorder()}

	if _, _, err := recorder.Hijack(); err == nil {
		t.Fatal("expected an unsupported writer to fail")
	}
}
