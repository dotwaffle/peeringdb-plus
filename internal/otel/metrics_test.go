package otel

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
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
