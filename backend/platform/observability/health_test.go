package observability

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthLiveIsAlwaysOK(t *testing.T) {
	health := NewHealth()
	health.SetReady(false)

	recorder := httptest.NewRecorder()
	health.AdminMux(nil).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 while draining", recorder.Code)
	}
}

func TestHealthReadyPassesChecks(t *testing.T) {
	health := NewHealth()
	health.AddCheck("postgres", func(context.Context) error { return nil })

	recorder := httptest.NewRecorder()
	health.AdminMux(nil).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
}

func TestHealthReadyFailsOnDependency(t *testing.T) {
	health := NewHealth()
	health.AddCheck("postgres", func(context.Context) error { return errors.New("connection refused") })

	recorder := httptest.NewRecorder()
	health.AdminMux(nil).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}

func TestHealthReadyFailsWhileDraining(t *testing.T) {
	health := NewHealth()
	health.SetReady(false)

	recorder := httptest.NewRecorder()
	health.AdminMux(nil).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}
