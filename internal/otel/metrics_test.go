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

func TestInitMetrics_NoError(t *testing.T) {
	t.Setenv("OTEL_METRICS_EXPORTER", "none")

	if err := InitMetrics(); err != nil {
		t.Fatalf("InitMetrics returned error: %v", err)
	}
}

func TestInitMetrics_SyncDurationNotNil(t *testing.T) {
	t.Setenv("OTEL_METRICS_EXPORTER", "none")

	if err := InitMetrics(); err != nil {
		t.Fatalf("InitMetrics returned error: %v", err)
	}
	if SyncDuration == nil {
		t.Fatal("SyncDuration is nil after InitMetrics")
	}
}

func TestInitMetrics_SyncOperationsNotNil(t *testing.T) {
	t.Setenv("OTEL_METRICS_EXPORTER", "none")

	if err := InitMetrics(); err != nil {
		t.Fatalf("InitMetrics returned error: %v", err)
	}
	if SyncOperations == nil {
		t.Fatal("SyncOperations is nil after InitMetrics")
	}
}

func TestSyncDuration_RecordDoesNotPanic(t *testing.T) {
	t.Setenv("OTEL_METRICS_EXPORTER", "none")

	if err := InitMetrics(); err != nil {
		t.Fatalf("InitMetrics returned error: %v", err)
	}

	// Recording a value should not panic.
	SyncDuration.Record(context.Background(), 5.0,
		metric.WithAttributes(attribute.String("type", "full")),
	)
}

func TestSyncOperations_AddDoesNotPanic(t *testing.T) {
	t.Setenv("OTEL_METRICS_EXPORTER", "none")

	if err := InitMetrics(); err != nil {
		t.Fatalf("InitMetrics returned error: %v", err)
	}

	// Adding a value should not panic.
	SyncOperations.Add(context.Background(), 1,
		metric.WithAttributes(attribute.String("status", "success")),
	)
}

func TestInitMetrics_PerTypeInstruments(t *testing.T) {
	t.Setenv("OTEL_METRICS_EXPORTER", "none")

	if err := InitMetrics(); err != nil {
		t.Fatalf("InitMetrics returned error: %v", err)
	}

	tests := []struct {
		name  string
		value any
	}{
		{"SyncTypeDuration", SyncTypeDuration},
		{"SyncTypeObjects", SyncTypeObjects},
		{"SyncTypeDeleted", SyncTypeDeleted},
		{"SyncTypeFetchErrors", SyncTypeFetchErrors},
		{"SyncTypeUpsertErrors", SyncTypeUpsertErrors},
	}
	for _, tc := range tests {
		if tc.value == nil {
			t.Errorf("%s is nil after InitMetrics", tc.name)
		}
	}
}

func TestInitMetrics_PerTypeRecordDoesNotPanic(t *testing.T) {
	t.Setenv("OTEL_METRICS_EXPORTER", "none")

	if err := InitMetrics(); err != nil {
		t.Fatalf("InitMetrics returned error: %v", err)
	}

	ctx := context.Background()
	typeAttr := metric.WithAttributes(attribute.String("type", "org"))

	SyncTypeDuration.Record(ctx, 1.5, typeAttr)
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

func TestInitMetrics_RecordsValues(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	if err := InitMetrics(); err != nil {
		t.Fatalf("InitMetrics: %v", err)
	}

	ctx := context.Background()
	typeAttr := metric.WithAttributes(attribute.String("type", "org"))

	SyncTypeDuration.Record(ctx, 2.5, typeAttr)
	SyncTypeObjects.Add(ctx, 42, typeAttr)
	SyncTypeDeleted.Add(ctx, 3, typeAttr)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	found := findMetric(rm, "pdbplus.sync.type.duration")
	if found == nil {
		t.Fatal("expected pdbplus.sync.type.duration metric, not found")
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
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	lastSync := time.Now().Add(-5 * time.Minute)
	err := InitFreshnessGauge(func(_ context.Context) (time.Time, bool) {
		return lastSync, true
	})
	if err != nil {
		t.Fatalf("InitFreshnessGauge: %v", err)
	}

	ctx := context.Background()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	found := findMetric(rm, "pdbplus.sync.freshness")
	if found == nil {
		t.Fatal("expected pdbplus.sync.freshness metric, not found")
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
