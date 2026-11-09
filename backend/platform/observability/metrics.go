package observability

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// Metrics owns a private Prometheus registry for one service. A private
// registry keeps test servers and multiple Metrics instances from colliding on
// the process-global default registerer.
type Metrics struct {
	service  string
	podID    string
	registry *prometheus.Registry

	grpcHandling   *prometheus.HistogramVec
	httpRequests   *prometheus.CounterVec
	httpDuration   *prometheus.HistogramVec
	consumerLag    *prometheus.GaugeVec
	websockets     *prometheus.GaugeVec
	dispatches     *prometheus.CounterVec
	registeredPool map[string]struct{}
}

// NewMetrics builds the metric set every service exposes, including Go runtime
// and process collectors.
func NewMetrics(service string) *Metrics {
	podID := os.Getenv("HOSTNAME")
	if podID == "" {
		podID = "local"
	}

	metrics := &Metrics{
		service:        service,
		podID:          podID,
		registry:       prometheus.NewRegistry(),
		registeredPool: make(map[string]struct{}),

		grpcHandling: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "grpc_server_handling_seconds",
			Help:    "Latency distribution of gRPC handlers.",
			Buckets: prometheus.DefBuckets,
		}, []string{"service", "grpc_method", "code"}),

		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total incoming HTTP requests.",
		}, []string{"service", "handler", "status"}),

		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Latency distribution of HTTP handlers.",
			Buckets: prometheus.DefBuckets,
		}, []string{"service", "handler"}),

		consumerLag: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "redis_stream_consumer_lag",
			Help: "Unprocessed events in a Redis Stream consumer group.",
		}, []string{"stream", "group"}),

		websockets: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "websocket_active_connections",
			Help: "Concurrent live WebSocket connections.",
		}, []string{"pod_id"}),

		dispatches: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "notification_dispatch_total",
			Help: "Notification dispatches by channel and outcome.",
		}, []string{"channel", "status"}),
	}

	metrics.registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		metrics.grpcHandling,
		metrics.httpRequests,
		metrics.httpDuration,
		metrics.consumerLag,
		metrics.websockets,
		metrics.dispatches,
	)

	return metrics
}

// Handler serves the Prometheus exposition format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// RegisterDBPool exposes the number of checked-out connections for a named
// pool. acquired is typically a closure over pgxpool.Stat().AcquiredConns.
func (m *Metrics) RegisterDBPool(pool string, acquired func() int32) {
	if m == nil || acquired == nil {
		return
	}
	if _, exists := m.registeredPool[pool]; exists {
		return
	}
	m.registeredPool[pool] = struct{}{}

	gauge := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "db_pool_connections_active",
		Help: "Physical database connections currently in use.",
		ConstLabels: prometheus.Labels{
			"service": m.service,
			"pool":    pool,
		},
	}, func() float64 {
		return float64(acquired())
	})

	if err := m.registry.Register(gauge); err != nil {
		// A duplicate here means two pools share a name; keep the first and
		// surface the mistake in logs rather than panicking the process.
		fmt.Printf("observability: register db pool %q: %v\n", pool, err)
	}
}

// UnaryServerInterceptor records gRPC latency, method, and status code.
func (m *Metrics) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		response, err := handler(ctx, req)
		m.grpcHandling.WithLabelValues(
			m.service,
			info.FullMethod,
			status.Code(err).String(),
		).Observe(time.Since(start).Seconds())
		return response, err
	}
}

// StreamServerInterceptor records gRPC streaming latency and outcome.
func (m *Metrics) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		err := handler(srv, stream)
		m.grpcHandling.WithLabelValues(
			m.service,
			info.FullMethod,
			status.Code(err).String(),
		).Observe(time.Since(start).Seconds())
		return err
	}
}

// HTTPMiddleware records request count and latency. It preserves Hijacker and
// Flusher so a WebSocket upgrade still works through the wrapper.
func (m *Metrics) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)

		handler := r.Pattern
		if handler == "" {
			handler = r.URL.Path
		}
		m.httpRequests.WithLabelValues(m.service, handler, strconv.Itoa(recorder.status)).Inc()

		// An upgraded connection lives for as long as the client keeps it
		// open. Recording that as HTTP latency would swamp the histogram with
		// multi-minute samples and hide real request slowdowns.
		if recorder.hijacked {
			return
		}
		m.httpDuration.WithLabelValues(m.service, handler).Observe(time.Since(start).Seconds())
	})
}

// SetConsumerLag publishes the backlog of a Redis Stream consumer group.
func (m *Metrics) SetConsumerLag(stream, group string, lag int64) {
	if m == nil {
		return
	}
	m.consumerLag.WithLabelValues(stream, group).Set(float64(lag))
}

// WebsocketConnected increments the live-connection gauge.
func (m *Metrics) WebsocketConnected() {
	if m == nil {
		return
	}
	m.websockets.WithLabelValues(m.podID).Inc()
}

// WebsocketDisconnected decrements the live-connection gauge.
func (m *Metrics) WebsocketDisconnected() {
	if m == nil {
		return
	}
	m.websockets.WithLabelValues(m.podID).Dec()
}

// RecordDispatch counts a notification dispatch by channel and status.
func (m *Metrics) RecordDispatch(channel, status string) {
	if m == nil {
		return
	}
	m.dispatches.WithLabelValues(channel, status).Inc()
}

// statusRecorder captures the status code while remaining compatible with
// handlers that upgrade to WebSockets.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	hijacked    bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}
	r.status = status
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(body []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(body)
}

func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not support hijacking")
	}
	conn, buffer, err := hijacker.Hijack()
	if err == nil {
		r.hijacked = true
		r.status = http.StatusSwitchingProtocols
	}
	return conn, buffer, err
}

func (r *statusRecorder) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := r.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}
