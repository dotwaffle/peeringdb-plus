package otel

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
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
	t.Cleanup(func() { out.Shutdown(ctx) })
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
	t.Cleanup(func() { out.Shutdown(ctx) })

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
	t.Cleanup(func() { out.Shutdown(ctx) })

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
	t.Cleanup(func() { out.Shutdown(ctx) })
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
	t.Cleanup(func() { out.Shutdown(ctx) })
	// If we get here without error, runtime.Start was called successfully.
}
