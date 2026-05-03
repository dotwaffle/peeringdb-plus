// Tests for the SDK Views configured in provider.go that trim otelhttp /
// otelconnect cardinality. These are unit tests against a freshly-built
// MeterProvider wired to a ManualReader — Setup() itself is not called
// because it pulls in autoexport (env-driven exporter selection) and we
// don't need a network exporter to verify View behaviour.
package otel

import (
	"context"
	"slices"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// trimViews mirrors the Views block in Setup(). Keep this list in sync
// when you add or remove a View in provider.go — there is no SoT extraction
// because the Views are inlined inside Setup(), and the cost of mirroring
// is one short edit per change.
func trimViews() []sdkmetric.View {
	dropped := []string{
		"http.server.request.body.size",
		"http.server.response.body.size",
		"rpc.server.duration",
		"rpc.server.request.size",
		"rpc.server.response.size",
		"rpc.server.requests_per_rpc",
		"rpc.server.responses_per_rpc",
	}
	views := make([]sdkmetric.View, 0, len(dropped)+1)
	for _, name := range dropped {
		views = append(views, sdkmetric.NewView(
			sdkmetric.Instrument{Name: name},
			sdkmetric.Stream{Aggregation: sdkmetric.AggregationDrop{}},
		))
	}
	views = append(views, sdkmetric.NewView(
		sdkmetric.Instrument{Name: "http.server.request.duration"},
		sdkmetric.Stream{
			Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
				Boundaries: []float64{0.01, 0.05, 0.25, 1, 5},
				NoMinMax:   false,
			},
			AttributeFilter: attribute.NewAllowKeysFilter(
				"http.route",
				"http.response.status_code",
				"network.protocol.version",
			),
		},
	))
	return views
}

// newTestProvider wires a MeterProvider with the trim Views into a
// ManualReader so tests can synchronously Collect().
func newTestProvider(t *testing.T) (*sdkmetric.MeterProvider, *sdkmetric.ManualReader) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	opts := []sdkmetric.Option{sdkmetric.WithReader(reader)}
	for _, v := range trimViews() {
		opts = append(opts, sdkmetric.WithView(v))
	}
	mp := sdkmetric.NewMeterProvider(opts...)
	t.Cleanup(func() {
		_ = mp.Shutdown(context.Background())
	})
	return mp, reader
}

func collect(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("manual reader Collect: %v", err)
	}
	return rm
}

func metricNames(rm metricdata.ResourceMetrics) []string {
	var names []string
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			names = append(names, m.Name)
		}
	}
	return names
}

// TestViews_DropsBodySizeAndRPCFamily records data for every instrument the
// Views are supposed to drop, then asserts none of them survive the Collect.
func TestViews_DropsBodySizeAndRPCFamily(t *testing.T) {
	t.Parallel()
	mp, reader := newTestProvider(t)
	meter := mp.Meter("test")

	dropped := []string{
		"http.server.request.body.size",
		"http.server.response.body.size",
		"rpc.server.duration",
		"rpc.server.request.size",
		"rpc.server.response.size",
		"rpc.server.requests_per_rpc",
		"rpc.server.responses_per_rpc",
	}
	for _, name := range dropped {
		// Histograms are the natural shape for duration/size — Drop
		// applies regardless of instrument kind, so a single shape is
		// enough for the assertion.
		h, err := meter.Int64Histogram(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		h.Record(context.Background(), 42)
	}

	rm := collect(t, reader)
	got := metricNames(rm)
	for _, name := range dropped {
		if slices.Contains(got, name) {
			t.Errorf("expected %s to be dropped by View, got %v", name, got)
		}
	}
}

// TestViews_HTTPDurationCoarsensBuckets records a single duration sample
// and asserts the histogram exposes our 5-boundary set rather than
// otelhttp's default.
func TestViews_HTTPDurationCoarsensBuckets(t *testing.T) {
	t.Parallel()
	mp, reader := newTestProvider(t)
	meter := mp.Meter("test")

	// Match the exact instrument name otelhttp v0.68 uses on the metric
	// pipeline (the trace pipeline uses a different name).
	h, err := meter.Float64Histogram("http.server.request.duration")
	if err != nil {
		t.Fatalf("create histogram: %v", err)
	}
	h.Record(context.Background(), 0.123,
		metric.WithAttributes(
			attribute.String("http.route", "/api/net"),
			attribute.Int("http.response.status_code", 200),
			attribute.String("network.protocol.version", "1.1"),
		),
	)

	rm := collect(t, reader)
	hist := findFloat64Histogram(t, rm, "http.server.request.duration")
	if len(hist.DataPoints) != 1 {
		t.Fatalf("expected 1 data point, got %d", len(hist.DataPoints))
	}
	got := hist.DataPoints[0].Bounds
	want := []float64{0.01, 0.05, 0.25, 1, 5}
	if !slices.Equal(got, want) {
		t.Errorf("histogram bounds = %v, want %v", got, want)
	}
}

// TestViews_HTTPDurationDropsMethodAttribute records two samples that differ
// only by http.request.method (and a few other denied keys); after the
// AttributeFilter strips those, the two samples must collapse into one
// data point — proving method is no longer an axis.
func TestViews_HTTPDurationDropsMethodAttribute(t *testing.T) {
	t.Parallel()
	mp, reader := newTestProvider(t)
	meter := mp.Meter("test")

	h, err := meter.Float64Histogram("http.server.request.duration")
	if err != nil {
		t.Fatalf("create histogram: %v", err)
	}

	commonAllowed := []attribute.KeyValue{
		attribute.String("http.route", "/api/net"),
		attribute.Int("http.response.status_code", 200),
		attribute.String("network.protocol.version", "1.1"),
	}
	// First sample: method=GET, plus a couple of other denied keys.
	h.Record(context.Background(), 0.1, metric.WithAttributes(append(slices.Clone(commonAllowed),
		attribute.String("http.request.method", "GET"),
		attribute.String("server.address", "primary.lhr"),
		attribute.String("url.scheme", "https"),
	)...))
	// Second sample: same allowed keys, different denied keys. Should
	// merge into the first data point.
	h.Record(context.Background(), 0.2, metric.WithAttributes(append(slices.Clone(commonAllowed),
		attribute.String("http.request.method", "POST"),
		attribute.String("server.address", "replica.iad"),
		attribute.String("url.scheme", "http"),
	)...))

	rm := collect(t, reader)
	hist := findFloat64Histogram(t, rm, "http.server.request.duration")
	if len(hist.DataPoints) != 1 {
		t.Fatalf("expected 1 data point after filter, got %d (method/address/scheme axes leaked)", len(hist.DataPoints))
	}
	dp := hist.DataPoints[0]
	if dp.Count != 2 {
		t.Errorf("expected count=2 after merge, got %d", dp.Count)
	}
	// Verify the surviving attribute set matches the allow-list and
	// nothing more.
	wantKeys := map[attribute.Key]bool{
		"http.route":                true,
		"http.response.status_code": true,
		"network.protocol.version":  true,
	}
	for _, kv := range dp.Attributes.ToSlice() {
		if !wantKeys[kv.Key] {
			t.Errorf("unexpected attribute %q on filtered data point", kv.Key)
		}
		delete(wantKeys, kv.Key)
	}
	for k := range wantKeys {
		t.Errorf("allow-listed attribute %q missing from data point", k)
	}
}

func findFloat64Histogram(t *testing.T, rm metricdata.ResourceMetrics, name string) metricdata.Histogram[float64] {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			h, ok := m.Data.(metricdata.Histogram[float64])
			if !ok {
				t.Fatalf("metric %s has data type %T, want Histogram[float64]", name, m.Data)
			}
			return h
		}
	}
	t.Fatalf("metric %s not found in ResourceMetrics; got %v", name, metricNames(rm))
	return metricdata.Histogram[float64]{}
}
