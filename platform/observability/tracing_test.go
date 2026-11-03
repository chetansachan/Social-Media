package observability

import "testing"

func TestTracingConfigFromEnvDefaults(t *testing.T) {
	cfg := TracingConfigFromEnv("api-gateway")

	if cfg.ServiceName != "api-gateway" {
		t.Fatalf("service = %q", cfg.ServiceName)
	}
	if cfg.Endpoint != "" {
		t.Fatalf("endpoint = %q, want empty", cfg.Endpoint)
	}
	if !cfg.Insecure {
		t.Fatal("a local collector should default to insecure")
	}
	if cfg.SampleRatio != 0.1 {
		t.Fatalf("sample ratio = %v, want 0.1", cfg.SampleRatio)
	}
}

func TestTracingConfigFromEnvOverrides(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "https://collector:4317/")
	t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "false")
	t.Setenv("OTEL_TRACES_SAMPLE_RATIO", "0.25")

	cfg := TracingConfigFromEnv("post-service")

	if cfg.Endpoint != "collector:4317" {
		t.Fatalf("endpoint = %q, want the scheme stripped", cfg.Endpoint)
	}
	if cfg.Insecure {
		t.Fatal("OTEL_EXPORTER_OTLP_INSECURE=false was ignored")
	}
	if cfg.SampleRatio != 0.25 {
		t.Fatalf("sample ratio = %v, want 0.25", cfg.SampleRatio)
	}
}

func TestTracingConfigFromEnvIgnoresInvalidRatio(t *testing.T) {
	t.Setenv("OTEL_TRACES_SAMPLE_RATIO", "7")

	cfg := TracingConfigFromEnv("feed-service")

	if cfg.SampleRatio != 0.1 {
		t.Fatalf("sample ratio = %v, want the default", cfg.SampleRatio)
	}
}

func TestSetupTracingWithoutEndpointIsNoop(t *testing.T) {
	shutdown, err := SetupTracing(t.Context(), TracingConfig{ServiceName: "api-gateway"})
	if err != nil {
		t.Fatalf("SetupTracing: %v", err)
	}
	if err := shutdown(t.Context()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}
