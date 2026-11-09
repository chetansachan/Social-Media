package observability

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Health tracks whether the process can serve traffic. Liveness stays green
// while the process is alive; readiness flips red as soon as shutdown starts or
// a dependency check fails, so Kubernetes stops sending new requests before the
// listener closes.
type Health struct {
	ready atomic.Bool

	mu     sync.RWMutex
	checks map[string]func(context.Context) error
}

// NewHealth returns a ready health tracker with no dependency checks.
func NewHealth() *Health {
	health := &Health{checks: make(map[string]func(context.Context) error)}
	health.ready.Store(true)
	return health
}

// SetReady flips readiness. Call it with false on SIGTERM before graceful stop.
func (h *Health) SetReady(ready bool) {
	h.ready.Store(ready)
}

// AddCheck registers a dependency probe for /readyz. Checks should be cheap
// and bounded; the handler imposes a two-second timeout on each one.
func (h *Health) AddCheck(name string, check func(context.Context) error) {
	if check == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checks[name] = check
}

// AdminMux serves /metrics, /healthz, and /readyz on the internal admin port.
func (h *Health) AdminMux(metrics *Metrics) http.Handler {
	mux := http.NewServeMux()
	if metrics != nil {
		mux.Handle("/metrics", metrics.Handler())
	}
	mux.HandleFunc("/healthz", h.handleLive)
	mux.HandleFunc("/readyz", h.handleReady)
	return mux
}

func (h *Health) handleLive(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Health) handleReady(w http.ResponseWriter, r *http.Request) {
	if !h.ready.Load() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status": "draining",
			"checks": map[string]string{},
		})
		return
	}

	h.mu.RLock()
	checks := make(map[string]func(context.Context) error, len(h.checks))
	for name, check := range h.checks {
		checks[name] = check
	}
	h.mu.RUnlock()

	results := make(map[string]string, len(checks))
	healthy := true
	for name, check := range checks {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		err := check(ctx)
		cancel()
		if err != nil {
			results[name] = err.Error()
			healthy = false
			continue
		}
		results[name] = "ok"
	}

	status := http.StatusOK
	label := "ok"
	if !healthy {
		status = http.StatusServiceUnavailable
		label = "unavailable"
	}
	writeJSON(w, status, map[string]any{"status": label, "checks": results})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
