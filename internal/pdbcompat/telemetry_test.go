package pdbcompat

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	pdbotel "github.com/dotwaffle/peeringdb-plus/internal/otel"
)

// installManualMetricReader installs a manual-reader MeterProvider as the
// global provider for the duration of the test and returns the reader so
// the test can collect observations synchronously.
func installManualMetricReader(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })
	return reader
}

// installInMemorySpanExporter installs a synchronous in-memory span
// exporter as the global TracerProvider so captured spans are available
// immediately on End.
func installInMemorySpanExporter(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	return exporter
}

// TestMemStatsHeapInuseKiB_Positive sanity-checks that the sampler
// returns a positive KiB value on a live process. Not a rigorous
// monotonicity test — GC between calls can legitimately shrink
// HeapInuse.
func TestMemStatsHeapInuseKiB_Positive(t *testing.T) {
	t.Parallel()

	got := memStatsHeapInuseKiB()
	if got <= 0 {
		t.Errorf("memStatsHeapInuseKiB() = %d, want > 0 (test process has a non-empty heap)", got)
	}
}

// TestRecordResponseHeapDelta_SetsSpanAttribute verifies
// recordResponseHeapDelta stamps pdbplus.response.heap_delta_kib on the
// active span (D-06 wire-level contract).
func TestRecordResponseHeapDelta_SetsSpanAttribute(t *testing.T) {
	// Not parallel: installs global TracerProvider.
	exporter := installInMemorySpanExporter(t)

	ctx, span := otel.Tracer("test").Start(context.Background(), "handler")
	recordResponseHeapDelta(ctx, "/api/net", "net", 0)
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	var found bool
	for _, a := range spans[0].Attributes {
		if string(a.Key) == "pdbplus.response.heap_delta_kib" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("span missing attribute pdbplus.response.heap_delta_kib; attrs=%+v", spans[0].Attributes)
	}
}

// TestRecordResponseHeapDelta_RecordsHistogram verifies
// recordResponseHeapDelta records a histogram observation on the
// pdbplus.response.heap_delta_kib instrument with endpoint + entity
// attributes.
func TestRecordResponseHeapDelta_RecordsHistogram(t *testing.T) {
	// Not parallel: installs global MeterProvider.
	reader := installManualMetricReader(t)

	if err := pdbotel.InitResponseHeapHistogram(); err != nil {
		t.Fatalf("InitResponseHeapHistogram: %v", err)
	}

	recordResponseHeapDelta(context.Background(), "/api/net", "net", 0)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	var found *metricdata.Metrics
	for _, sm := range rm.ScopeMetrics {
		for i := range sm.Metrics {
			if sm.Metrics[i].Name == "pdbplus.response.heap_delta_kib" {
				found = &sm.Metrics[i]
			}
		}
	}
	if found == nil {
		t.Fatal("pdbplus.response.heap_delta_kib metric not found in collected ResourceMetrics")
	}
	hist, ok := found.Data.(metricdata.Histogram[int64])
	if !ok {
		t.Fatalf("expected Histogram[int64], got %T", found.Data)
	}
	if len(hist.DataPoints) == 0 {
		t.Fatal("expected >= 1 histogram data point")
	}
	var gotEndpoint, gotEntity string
	for _, a := range hist.DataPoints[0].Attributes.ToSlice() {
		switch a.Key {
		case "endpoint":
			gotEndpoint = a.Value.AsString()
		case "entity":
			gotEntity = a.Value.AsString()
		}
	}
	if gotEndpoint != "/api/net" {
		t.Errorf("endpoint = %q, want /api/net", gotEndpoint)
	}
	if gotEntity != "net" {
		t.Errorf("entity = %q, want net", gotEntity)
	}
}

// TestRecordResponseHeapDelta_FiresOnce verifies that wrapping a handler
// closure with `defer recordResponseHeapDelta(...)` fires EXACTLY ONE
// span attr and ONE histogram observation — guarding against a future
// refactor accidentally double-sampling (e.g. calling both inline AND via
// defer).
func TestRecordResponseHeapDelta_FiresOnce(t *testing.T) {
	// Not parallel: installs global providers.
	exporter := installInMemorySpanExporter(t)
	reader := installManualMetricReader(t)

	if err := pdbotel.InitResponseHeapHistogram(); err != nil {
		t.Fatalf("InitResponseHeapHistogram: %v", err)
	}

	handler := func(ctx context.Context) {
		startKiB := memStatsHeapInuseKiB()
		defer recordResponseHeapDelta(ctx, "/api/net", "net", startKiB)
		// Simulated handler body — no telemetry calls.
	}

	ctx, span := otel.Tracer("test").Start(context.Background(), "handler")
	handler(ctx)
	span.End()

	// Exactly one span, exactly one heap_delta_kib attribute.
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	var attrCount int
	for _, a := range spans[0].Attributes {
		if string(a.Key) == "pdbplus.response.heap_delta_kib" {
			attrCount++
		}
	}
	if attrCount != 1 {
		t.Errorf("pdbplus.response.heap_delta_kib attribute count = %d, want 1", attrCount)
	}

	// Exactly one histogram data point.
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	var found *metricdata.Metrics
	for _, sm := range rm.ScopeMetrics {
		for i := range sm.Metrics {
			if sm.Metrics[i].Name == "pdbplus.response.heap_delta_kib" {
				found = &sm.Metrics[i]
			}
		}
	}
	if found == nil {
		t.Fatal("pdbplus.response.heap_delta_kib metric not found")
	}
	hist, ok := found.Data.(metricdata.Histogram[int64])
	if !ok {
		t.Fatalf("expected Histogram[int64], got %T", found.Data)
	}
	if len(hist.DataPoints) != 1 {
		t.Errorf("histogram data point count = %d, want 1", len(hist.DataPoints))
	}
	// Count should be 1 observation on that point.
	if len(hist.DataPoints) == 1 && hist.DataPoints[0].Count != 1 {
		t.Errorf("histogram data point Count = %d, want 1", hist.DataPoints[0].Count)
	}
}

// TestRecordResponseHeapDelta_NilHistogramSafe verifies that if the
// global histogram pointer is nil (e.g. InitResponseHeapHistogram was
// never called), the sampler does not panic — telemetry is best-effort.
func TestRecordResponseHeapDelta_NilHistogramSafe(t *testing.T) {
	// Not parallel: mutates package-level pdbotel.ResponseHeapDeltaKiB.
	prev := pdbotel.ResponseHeapDeltaKiB
	pdbotel.ResponseHeapDeltaKiB = nil
	t.Cleanup(func() { pdbotel.ResponseHeapDeltaKiB = prev })

	// Must not panic.
	recordResponseHeapDelta(context.Background(), "/api/net", "net", 0)
}
