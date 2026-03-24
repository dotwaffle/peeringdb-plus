package otel

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestSetup_ReturnsNonNilShutdown(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_METRICS_EXPORTER", "none")
	t.Setenv("OTEL_LOGS_EXPORTER", "none")

	ctx := context.Background()
	out, err := Setup(ctx, SetupInput{
		ServiceName: "test-service",
		SampleRate:  1.0,
	})
	if err != nil {
		t.Fatalf("Setup returned error: %v", err)
	}
	if out == nil {
		t.Fatal("Setup returned nil output")
	}
	if out.Shutdown == nil {
		t.Fatal("Setup returned nil shutdown function")
	}
	t.Cleanup(func() { _ = out.Shutdown(ctx) })
}

func TestSetup_SetsGlobalTracerProvider(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_METRICS_EXPORTER", "none")
	t.Setenv("OTEL_LOGS_EXPORTER", "none")

	ctx := context.Background()
	out, err := Setup(ctx, SetupInput{
		ServiceName: "test-service",
		SampleRate:  1.0,
	})
	if err != nil {
		t.Fatalf("Setup returned error: %v", err)
	}
	t.Cleanup(func() { _ = out.Shutdown(ctx) })

	tp := otel.GetTracerProvider()
	if _, ok := tp.(*sdktrace.TracerProvider); !ok {
		t.Errorf("global TracerProvider is %T, want *sdktrace.TracerProvider", tp)
	}
}

func TestSetup_SetsGlobalMeterProvider(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_METRICS_EXPORTER", "none")
	t.Setenv("OTEL_LOGS_EXPORTER", "none")

	ctx := context.Background()
	out, err := Setup(ctx, SetupInput{
		ServiceName: "test-service",
		SampleRate:  1.0,
	})
	if err != nil {
		t.Fatalf("Setup returned error: %v", err)
	}
	t.Cleanup(func() { _ = out.Shutdown(ctx) })

	mp := otel.GetMeterProvider()
	if _, ok := mp.(*sdkmetric.MeterProvider); !ok {
		t.Errorf("global MeterProvider is %T, want *sdkmetric.MeterProvider", mp)
	}
}

func TestSetup_DisabledTraces(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_METRICS_EXPORTER", "none")
	t.Setenv("OTEL_LOGS_EXPORTER", "none")

	ctx := context.Background()
	out, err := Setup(ctx, SetupInput{
		ServiceName: "test-disabled-traces",
		SampleRate:  0.0,
	})
	if err != nil {
		t.Fatalf("Setup with disabled traces returned error: %v", err)
	}
	t.Cleanup(func() { _ = out.Shutdown(ctx) })
}

func TestSetup_ShutdownFlushesWithoutError(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_METRICS_EXPORTER", "none")
	t.Setenv("OTEL_LOGS_EXPORTER", "none")

	ctx := context.Background()
	out, err := Setup(ctx, SetupInput{
		ServiceName: "test-shutdown",
		SampleRate:  1.0,
	})
	if err != nil {
		t.Fatalf("Setup returned error: %v", err)
	}

	if err := out.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown returned error: %v", err)
	}
}

func TestBuildResource_IncludesServiceName(t *testing.T) {
	ctx := context.Background()
	res := buildResource(ctx, "my-test-service")

	found := false
	for _, attr := range res.Attributes() {
		if string(attr.Key) == "service.name" && attr.Value.AsString() == "my-test-service" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("resource attributes %v do not contain service.name=my-test-service", res.Attributes())
	}
}

func TestBuildResource_WithFlyRegion(t *testing.T) {
	t.Setenv("FLY_REGION", "iad")

	ctx := context.Background()
	res := buildResource(ctx, "test-service")

	found := false
	for _, attr := range res.Attributes() {
		if string(attr.Key) == "fly.region" && attr.Value.AsString() == "iad" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("resource attributes %v do not contain fly.region=iad", res.Attributes())
	}
}

func TestBuildResource_IncludesServiceVersion(t *testing.T) {
	ctx := context.Background()
	res := buildResource(ctx, "test-service")

	found := false
	for _, attr := range res.Attributes() {
		if string(attr.Key) == "service.version" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("resource attributes %v do not contain service.version", res.Attributes())
	}
}

func TestSetup_RuntimeMetrics(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_METRICS_EXPORTER", "none")
	t.Setenv("OTEL_LOGS_EXPORTER", "none")

	ctx := context.Background()
	out, err := Setup(ctx, SetupInput{
		ServiceName: "test-runtime",
		SampleRate:  1.0,
	})
	if err != nil {
		t.Fatalf("Setup returned error (runtime.Start should succeed): %v", err)
	}
	t.Cleanup(func() { _ = out.Shutdown(ctx) })
	// If we get here without error, runtime.Start was called successfully.
}

func TestSetup_PrometheusExporter(t *testing.T) {
	// Find an available port to avoid conflicts with parallel tests.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("finding free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_METRICS_EXPORTER", "prometheus")
	t.Setenv("OTEL_EXPORTER_PROMETHEUS_PORT", fmt.Sprintf("%d", port))
	t.Setenv("OTEL_EXPORTER_PROMETHEUS_HOST", "127.0.0.1")
	t.Setenv("OTEL_LOGS_EXPORTER", "none")

	ctx := context.Background()
	out, err := Setup(ctx, SetupInput{
		ServiceName: "test-prometheus",
		SampleRate:  1.0,
	})
	if err != nil {
		t.Fatalf("Setup with prometheus exporter returned error: %v", err)
	}
	t.Cleanup(func() { _ = out.Shutdown(ctx) })

	// Wait briefly for the HTTP server to start.
	time.Sleep(100 * time.Millisecond)

	// Verify the /metrics endpoint responds with Prometheus text format.
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/metrics", port))
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics status = %d, want 200", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading /metrics body: %v", err)
	}

	// Prometheus text format should contain at least the target_info metric
	// or HELP/TYPE declarations.
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "# HELP") && !strings.Contains(bodyStr, "# TYPE") {
		t.Errorf("/metrics response does not contain Prometheus text format markers; got:\n%s", bodyStr[:min(len(bodyStr), 500)])
	}
}
