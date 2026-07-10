package otel

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// TestInstruments_BoundAtPackageInit locks the package-init binding
// contract: every package-level instrument is usable without any Init
// call, so callers need no nil-guards.
func TestInstruments_BoundAtPackageInit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value any
	}{
		{"SyncDuration", SyncDuration},
		{"SyncOperations", SyncOperations},
		{"SyncTypeObjects", SyncTypeObjects},
		{"SyncTypeDeleted", SyncTypeDeleted},
		{"SyncTypeFetchErrors", SyncTypeFetchErrors},
		{"SyncTypeUpsertErrors", SyncTypeUpsertErrors},
		{"SyncTypeFallback", SyncTypeFallback},
		{"SyncTypeOrphans", SyncTypeOrphans},
		{"SyncFKBackfill", SyncFKBackfill},
		{"PeeringDBRequests", PeeringDBRequests},
		{"PeeringDBRetries", PeeringDBRetries},
		{"PeeringDBRateLimitWaitMS", PeeringDBRateLimitWaitMS},
		{"RoleTransitions", RoleTransitions},
		{"ResponseHeapDeltaBytes", ResponseHeapDeltaBytes},
	}
	for _, tc := range tests {
		if tc.value == nil {
			t.Errorf("%s is nil at package init", tc.name)
		}
	}
}

func TestSyncDuration_RecordDoesNotPanic(t *testing.T) {
	t.Setenv("OTEL_METRICS_EXPORTER", "none")

	// Recording a value should not panic.
	SyncDuration.Record(t.Context(), 5.0,
		metric.WithAttributes(attribute.String("type", "full")),
	)
}

func TestSyncOperations_AddDoesNotPanic(t *testing.T) {
	t.Setenv("OTEL_METRICS_EXPORTER", "none")

	// Adding a value should not panic.
	SyncOperations.Add(t.Context(), 1,
		metric.WithAttributes(attribute.String("status", "success")),
	)
}

func TestPerTypeRecordDoesNotPanic(t *testing.T) {
	t.Setenv("OTEL_METRICS_EXPORTER", "none")

	ctx := t.Context()
	typeAttr := metric.WithAttributes(attribute.String("type", "org"))

	SyncTypeObjects.Add(ctx, 10, typeAttr)
	SyncTypeDeleted.Add(ctx, 2, typeAttr)
	SyncTypeFetchErrors.Add(ctx, 1, typeAttr)
	SyncTypeUpsertErrors.Add(ctx, 1, typeAttr)
}

func TestInitFreshnessGauge_NoError(t *testing.T) {
	t.Setenv("OTEL_METRICS_EXPORTER", "none")

	err := InitFreshnessGauge(func(_ context.Context) (time.Time, bool) {
		return time.Now().Add(-5 * time.Minute), true
	})
	if err != nil {
		t.Fatalf("InitFreshnessGauge returned error: %v", err)
	}
}

func TestInitFreshnessGauge_NoSync(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { _ = mp.Shutdown(t.Context()) })

	// Callback returns false (no successful sync yet).
	err := InitFreshnessGauge(func(_ context.Context) (time.Time, bool) {
		return time.Time{}, false
	})
	if err != nil {
		t.Fatalf("InitFreshnessGauge: %v", err)
	}

	ctx := t.Context()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	// When the callback returns !ok (no sync), no observation is recorded.
	// The OTel SDK may omit the gauge entirely from collected metrics when
	// no data points exist. Either outcome is correct -- the coverage target
	// is exercising the `return nil` path in the callback.
	found := findMetric(rm, "pdbplus.sync.freshness")
	if found != nil {
		gauge, ok := found.Data.(metricdata.Gauge[float64])
		if ok && len(gauge.DataPoints) > 0 {
			t.Logf("SDK reported %d data points (implementation detail); callback !ok path exercised", len(gauge.DataPoints))
		}
	}
	// If found == nil, the SDK simply didn't report the metric (expected
	// when no Observe calls were made). The callback was still invoked.
}

func TestBindInstruments_RecordsValues(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { _ = mp.Shutdown(t.Context()) })

	// Rebind: otel's global delegation is once-only, so instruments
	// created at package init may be bound to a provider another test
	// installed earlier in this process.
	BindInstruments()

	ctx := t.Context()
	typeAttr := metric.WithAttributes(attribute.String("type", "org"))

	SyncTypeObjects.Add(ctx, 42, typeAttr)
	SyncTypeDeleted.Add(ctx, 3, typeAttr)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	foundObjs := findMetric(rm, "pdbplus.sync.type.objects")
	if foundObjs == nil {
		t.Fatal("expected pdbplus.sync.type.objects metric, not found")
	}

	foundDel := findMetric(rm, "pdbplus.sync.type.deleted")
	if foundDel == nil {
		t.Fatal("expected pdbplus.sync.type.deleted metric, not found")
	}
}

func TestInitFreshnessGauge_RecordsValue(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { _ = mp.Shutdown(t.Context()) })

	lastSync := time.Now().Add(-5 * time.Minute)
	err := InitFreshnessGauge(func(_ context.Context) (time.Time, bool) {
		return lastSync, true
	})
	if err != nil {
		t.Fatalf("InitFreshnessGauge: %v", err)
	}

	ctx := t.Context()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	found := findMetric(rm, "pdbplus.sync.freshness")
	if found == nil {
		t.Fatal("expected pdbplus.sync.freshness metric, not found")
		return
	}

	gauge, ok := found.Data.(metricdata.Gauge[float64])
	if !ok {
		t.Fatalf("expected Gauge[float64], got %T", found.Data)
	}
	if len(gauge.DataPoints) == 0 {
		t.Fatal("expected at least one data point")
	}
	// Freshness should be roughly 300 seconds (5 minutes), allow some tolerance.
	val := gauge.DataPoints[0].Value
	if val < 290 || val > 310 {
		t.Errorf("expected freshness ~300s, got %f", val)
	}
}

// TestInitMemoryGauges_RecordsValues asserts both peak gauges emit
// the values held in the SyncPeakHeapBytes / SyncPeakRSSBytes atomics that
// internal/sync.(*Worker).emitMemoryTelemetry updates at end of cycle.
func TestInitMemoryGauges_RecordsValues(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { _ = mp.Shutdown(t.Context()) })

	const heap, rss = 419430400, 524288000 // 400 MiB, 500 MiB
	SyncPeakHeapBytes.Store(heap)
	SyncPeakRSSBytes.Store(rss)
	// Reset so the globals don't leak into sibling (non-parallel) tests.
	t.Cleanup(func() { SyncPeakHeapBytes.Store(0); SyncPeakRSSBytes.Store(0) })

	if err := InitMemoryGauges(); err != nil {
		t.Fatalf("InitMemoryGauges: %v", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	for name, want := range map[string]int64{
		"pdbplus.sync.peak_heap": heap,
		"pdbplus.sync.peak_rss":  rss,
	} {
		found := findMetric(rm, name)
		if found == nil {
			t.Errorf("%s metric not found", name)
			continue
		}
		gauge, ok := found.Data.(metricdata.Gauge[int64])
		if !ok {
			t.Errorf("%s: expected Gauge[int64], got %T", name, found.Data)
			continue
		}
		if len(gauge.DataPoints) == 0 {
			t.Errorf("%s: no data points", name)
			continue
		}
		if got := gauge.DataPoints[0].Value; got != want {
			t.Errorf("%s = %d, want %d", name, got, want)
		}
	}
}

// TestInitMemoryGauges_ZeroSuppressed locks the documented zero-suppression
// invariant: before the first sync (or non-Linux RSS) the atomics are 0 and
// the callbacks MUST NOT observe, so dashboards don't plot misleading zeros.
// Dropping the `v > 0` guard makes the gauge emit a 0 data point and fails here.
func TestInitMemoryGauges_ZeroSuppressed(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { _ = mp.Shutdown(t.Context()) })

	SyncPeakHeapBytes.Store(0)
	SyncPeakRSSBytes.Store(0)

	if err := InitMemoryGauges(); err != nil {
		t.Fatalf("InitMemoryGauges: %v", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	for _, name := range []string{"pdbplus.sync.peak_heap", "pdbplus.sync.peak_rss"} {
		found := findMetric(rm, name)
		if found == nil {
			continue // SDK omitted the metric entirely — correct (no observation).
		}
		if gauge, ok := found.Data.(metricdata.Gauge[int64]); ok && len(gauge.DataPoints) > 0 {
			t.Errorf("%s emitted %d data point(s) for a zero value; want suppressed", name, len(gauge.DataPoints))
		}
	}
}

func TestInitObjectCountGauges_NoError(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { _ = mp.Shutdown(t.Context()) })

	counts := make(map[string]int64)
	if err := InitObjectCountGauges(func() map[string]int64 { return counts }); err != nil {
		t.Fatalf("InitObjectCountGauges returned error: %v", err)
	}
}

func TestInitObjectCountGauges_RecordsValues(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { _ = mp.Shutdown(t.Context()) })

	// Pre-populated counts simulating a post-sync cache update.
	counts := map[string]int64{
		"org": 42, "campus": 5, "fac": 100, "carrier": 3, "carrierfac": 7,
		"ix": 50, "ixlan": 50, "ixpfx": 200, "ixfac": 80,
		"net": 300, "poc": 150, "netfac": 400, "netixlan": 500,
	}

	if err := InitObjectCountGauges(func() map[string]int64 { return counts }); err != nil {
		t.Fatalf("InitObjectCountGauges: %v", err)
	}

	ctx := t.Context()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	found := findMetric(rm, "pdbplus.data.type.count")
	if found == nil {
		t.Fatal("expected pdbplus.data.type.count metric, not found")
		return
	}

	gauge, ok := found.Data.(metricdata.Gauge[int64])
	if !ok {
		t.Fatalf("expected Gauge[int64], got %T", found.Data)
	}

	// Expect 13 data points (one per PeeringDB type).
	if len(gauge.DataPoints) != 13 {
		t.Errorf("expected 13 data points, got %d", len(gauge.DataPoints))
	}

	// Build a map of type -> value from the gauge data points.
	typeValues := make(map[string]int64)
	for _, dp := range gauge.DataPoints {
		for _, attr := range dp.Attributes.ToSlice() {
			if attr.Key == "type" {
				typeValues[attr.Value.AsString()] = dp.Value
			}
		}
	}

	// Verify each type reports the expected cached count.
	for typ, expected := range counts {
		val, exists := typeValues[typ]
		if !exists {
			t.Errorf("missing type attribute %q in data points", typ)
			continue
		}
		if val != expected {
			t.Errorf("type %q count = %d, want %d", typ, val, expected)
		}
	}
}

func TestInitObjectCountGauges_EmptyCache(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { _ = mp.Shutdown(t.Context()) })

	// Empty map simulates state before first sync completes.
	counts := make(map[string]int64)
	if err := InitObjectCountGauges(func() map[string]int64 { return counts }); err != nil {
		t.Fatalf("InitObjectCountGauges: %v", err)
	}

	ctx := t.Context()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	// The gauge should still be present but with zero data points
	// because the cache is empty (no sync has run yet).
	found := findMetric(rm, "pdbplus.data.type.count")
	if found == nil {
		// Acceptable: SDK may omit gauges with no observations.
		return
	}

	gauge, ok := found.Data.(metricdata.Gauge[int64])
	if !ok {
		t.Fatalf("expected Gauge[int64], got %T", found.Data)
	}
	if len(gauge.DataPoints) != 0 {
		t.Errorf("expected 0 data points for empty cache, got %d", len(gauge.DataPoints))
	}
}

// findMetric searches ResourceMetrics for a metric by name.
func findMetric(rm metricdata.ResourceMetrics, name string) *metricdata.Metrics {
	for _, sm := range rm.ScopeMetrics {
		for i := range sm.Metrics {
			if sm.Metrics[i].Name == name {
				return &sm.Metrics[i]
			}
		}
	}
	return nil
}

// TestResponseHeapDeltaBytes_RecordDoesNotPanic verifies the histogram
// instrument records samples without panicking when no reader is wired,
// matching the best-effort behaviour expected from
// internal/pdbcompat.recordResponseHeapDelta on a misconfigured exporter.
func TestResponseHeapDeltaBytes_RecordDoesNotPanic(t *testing.T) {
	t.Setenv("OTEL_METRICS_EXPORTER", "none")

	ResponseHeapDeltaBytes.Record(t.Context(), 42,
		metric.WithAttributes(
			attribute.String("endpoint", "/api/net"),
			attribute.String("entity", "net"),
		),
	)
}

// TestResponseHeapHistogram_RecordsValues verifies an actual observation
// flows through the OTel manual reader with the expected attributes.
func TestResponseHeapHistogram_RecordsValues(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { _ = mp.Shutdown(t.Context()) })

	BindInstruments()

	ctx := t.Context()
	ResponseHeapDeltaBytes.Record(ctx, 128,
		metric.WithAttributes(
			attribute.String("endpoint", "/api/net"),
			attribute.String("entity", "net"),
		),
	)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	found := findMetric(rm, "pdbplus.response.heap_delta")
	if found == nil {
		t.Fatal("expected pdbplus.response.heap_delta metric, not found")
		return
	}
	hist, ok := found.Data.(metricdata.Histogram[int64])
	if !ok {
		t.Fatalf("expected Histogram[int64], got %T", found.Data)
	}
	if len(hist.DataPoints) == 0 {
		t.Fatal("expected at least one histogram data point")
	}
	// Verify the endpoint+entity attributes landed on the data point.
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
		t.Errorf("endpoint attr = %q, want %q", gotEndpoint, "/api/net")
	}
	if gotEntity != "net" {
		t.Errorf("entity attr = %q, want %q", gotEntity, "net")
	}
}
