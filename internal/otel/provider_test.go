package otel

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestSetup_ReturnsNonNilShutdown(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_METRICS_EXPORTER", "none")
	t.Setenv("OTEL_LOGS_EXPORTER", "none")

	ctx := t.Context()
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

	ctx := t.Context()
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

	ctx := t.Context()
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

	ctx := t.Context()
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

	ctx := t.Context()
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
	ctx := t.Context()
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

// findAttr returns the string value of an attribute key on a resource's
// attribute set, plus a bool indicating presence. Centralises the
// scan-the-attribute-set loop used across the resource tests.
func findAttr(res interface{ Attributes() []attribute.KeyValue }, key string) (string, bool) {
	for _, attr := range res.Attributes() {
		if string(attr.Key) == key {
			return attr.Value.AsString(), true
		}
	}
	return "", false
}

func TestBuildResource_WithCloudRegion(t *testing.T) {
	t.Setenv("FLY_REGION", "iad")

	ctx := t.Context()
	res := buildResource(ctx, "test-service")

	got, ok := findAttr(res, "cloud.region")
	if !ok || got != "iad" {
		t.Errorf("resource attributes %v do not contain cloud.region=iad", res.Attributes())
	}
}

func TestBuildResource_IncludesServiceVersion(t *testing.T) {
	ctx := t.Context()
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

// TestBuildMetricResource_OmitsServiceInstanceID locks in the metric
// resource's per-VM cardinality reduction: service.instance.id must NOT
// appear on metric resource attributes so the backend aggregates across
// VMs rather than fanning out per series. service.namespace, cloud.region,
// cloud.provider and cloud.platform stay on the metric resource because
// they answer the operator's actual breakdown questions (process group +
// region) at low cardinality.
func TestBuildMetricResource_OmitsServiceInstanceID(t *testing.T) {
	t.Setenv("FLY_MACHINE_ID", "abc123")
	t.Setenv("FLY_PROCESS_GROUP", "primary")
	t.Setenv("FLY_REGION", "iad")

	ctx := t.Context()
	res := buildMetricResource(ctx, "test-service")

	if got, ok := findAttr(res, "service.instance.id"); ok {
		t.Errorf("metric resource must not contain service.instance.id; found %q", got)
	}

	// Sanity: service.name must still be present so the metric is attributed.
	if got, ok := findAttr(res, "service.name"); !ok || got != "test-service" {
		t.Errorf("metric resource missing service.name=test-service; got %v", res.Attributes())
	}
	// GC-allowlisted attrs the dashboard depends on must remain on metrics.
	for _, want := range []struct{ key, val string }{
		{"service.namespace", "primary"},
		{"cloud.region", "iad"},
		{"cloud.provider", "fly_io"},
		{"cloud.platform", "fly_io_apps"},
	} {
		if got, ok := findAttr(res, want.key); !ok || got != want.val {
			t.Errorf("metric resource missing %s=%s; got %v", want.key, want.val, res.Attributes())
		}
	}
}

// TestBuildResource_IncludesServiceInstanceID locks in the trace/log
// resource's per-VM debuggability: service.instance.id MUST remain on
// buildResource so a future refactor cannot accidentally strip it from
// traces and logs.
func TestBuildResource_IncludesServiceInstanceID(t *testing.T) {
	t.Setenv("FLY_MACHINE_ID", "abc123")

	ctx := t.Context()
	res := buildResource(ctx, "test-service")

	if got, ok := findAttr(res, "service.instance.id"); !ok || got != "abc123" {
		t.Errorf("trace/log resource must contain service.instance.id=abc123; got %v", res.Attributes())
	}
}

// TestBuildResource_IncludesServiceNamespace asserts FLY_PROCESS_GROUP is
// emitted as semconv service.namespace on the trace/log resource.
func TestBuildResource_IncludesServiceNamespace(t *testing.T) {
	t.Setenv("FLY_PROCESS_GROUP", "primary")

	ctx := t.Context()
	res := buildResource(ctx, "test-service")

	if got, ok := findAttr(res, "service.namespace"); !ok || got != "primary" {
		t.Errorf("resource must contain service.namespace=primary; got %v", res.Attributes())
	}
}

// TestBuildResource_CloudProviderConstant asserts cloud.provider /
// cloud.platform are emitted unconditionally (independent of any FLY_*
// env vars). These are 1-cardinality semconv keys that GC allowlists for
// free.
func TestBuildResource_CloudProviderConstant(t *testing.T) {
	// Explicitly clear FLY_* env vars so we prove the unconditional path.
	t.Setenv("FLY_REGION", "")
	t.Setenv("FLY_MACHINE_ID", "")
	t.Setenv("FLY_APP_NAME", "")
	t.Setenv("FLY_PROCESS_GROUP", "")

	ctx := t.Context()
	res := buildResource(ctx, "test-service")

	if got, ok := findAttr(res, "cloud.provider"); !ok || got != "fly_io" {
		t.Errorf("resource must contain cloud.provider=fly_io; got %v", res.Attributes())
	}
	if got, ok := findAttr(res, "cloud.platform"); !ok || got != "fly_io_apps" {
		t.Errorf("resource must contain cloud.platform=fly_io_apps; got %v", res.Attributes())
	}
}

// TestBuildResource_EmptyEnvOmitsAttr asserts that an unset env var does
// NOT leak as an empty-string attribute (existing behaviour preserved).
func TestBuildResource_EmptyEnvOmitsAttr(t *testing.T) {
	t.Setenv("FLY_REGION", "")
	t.Setenv("FLY_PROCESS_GROUP", "")
	t.Setenv("FLY_APP_NAME", "")
	t.Setenv("FLY_MACHINE_ID", "")

	ctx := t.Context()
	res := buildResource(ctx, "test-service")

	for _, key := range []string{"cloud.region", "service.namespace", "fly.app_name", "service.instance.id"} {
		if _, ok := findAttr(res, key); ok {
			t.Errorf("resource must omit %s when its source env var is empty; got %v", key, res.Attributes())
		}
	}
}

// TestBuildMetricResource_IncludesServiceNamespace asserts the metric
// resource still carries service.namespace (process group is a desired
// 2-cardinality dimension on metrics, distinct from the per-VM
// service.instance.id strip).
func TestBuildMetricResource_IncludesServiceNamespace(t *testing.T) {
	t.Setenv("FLY_PROCESS_GROUP", "replica")

	ctx := t.Context()
	res := buildMetricResource(ctx, "test-service")

	if got, ok := findAttr(res, "service.namespace"); !ok || got != "replica" {
		t.Errorf("metric resource must contain service.namespace=replica; got %v", res.Attributes())
	}
}

// TestBuildMetricResource_IncludesCloudRegion asserts the metric resource
// keeps cloud.region (8-cardinality fly region is a desired dimension on
// metrics, distinct from per-VM strip).
func TestBuildMetricResource_IncludesCloudRegion(t *testing.T) {
	t.Setenv("FLY_REGION", "lhr")

	ctx := t.Context()
	res := buildMetricResource(ctx, "test-service")

	if got, ok := findAttr(res, "cloud.region"); !ok || got != "lhr" {
		t.Errorf("metric resource must contain cloud.region=lhr; got %v", res.Attributes())
	}
}

func TestSetup_RuntimeMetrics(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_METRICS_EXPORTER", "none")
	t.Setenv("OTEL_LOGS_EXPORTER", "none")

	ctx := t.Context()
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

func TestSetup_InvalidSpanExporter(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "invalid_exporter_that_does_not_exist")
	t.Setenv("OTEL_METRICS_EXPORTER", "none")
	t.Setenv("OTEL_LOGS_EXPORTER", "none")

	ctx := t.Context()
	_, err := Setup(ctx, SetupInput{ServiceName: "test", SampleRate: 1.0})
	if err == nil {
		t.Fatal("expected error for invalid span exporter")
	}
	if !strings.Contains(err.Error(), "creating span exporter") {
		t.Errorf("error = %q, want substring %q", err, "creating span exporter")
	}
}

func TestSetup_InvalidMetricReader(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_METRICS_EXPORTER", "invalid_exporter_that_does_not_exist")
	t.Setenv("OTEL_LOGS_EXPORTER", "none")

	ctx := t.Context()
	_, err := Setup(ctx, SetupInput{ServiceName: "test", SampleRate: 1.0})
	if err == nil {
		t.Fatal("expected error for invalid metric reader")
	}
	if !strings.Contains(err.Error(), "creating metric reader") {
		t.Errorf("error = %q, want substring %q", err, "creating metric reader")
	}
}

func TestSetup_InvalidLogExporter(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_METRICS_EXPORTER", "none")
	t.Setenv("OTEL_LOGS_EXPORTER", "invalid_exporter_that_does_not_exist")

	ctx := t.Context()
	_, err := Setup(ctx, SetupInput{ServiceName: "test", SampleRate: 1.0})
	if err == nil {
		t.Fatal("expected error for invalid log exporter")
	}
	if !strings.Contains(err.Error(), "creating log exporter") {
		t.Errorf("error = %q, want substring %q", err, "creating log exporter")
	}
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

	ctx := t.Context()
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
