package otel

import (
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// SyncDuration records the duration of sync operations in seconds.
var SyncDuration metric.Float64Histogram

// SyncOperations counts sync operations by status (success/failed).
var SyncOperations metric.Int64Counter

// InitMetrics registers custom metric instruments for sync operations per D-05.
// HTTP metrics are handled automatically by otelhttp middleware (Plan 03).
func InitMetrics() error {
	meter := otel.Meter("peeringdb-plus")

	var err error
	SyncDuration, err = meter.Float64Histogram("pdbplus.sync.duration",
		metric.WithDescription("Duration of sync operations in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(1, 5, 10, 30, 60, 120, 300),
	)
	if err != nil {
		return fmt.Errorf("registering pdbplus.sync.duration histogram: %w", err)
	}

	SyncOperations, err = meter.Int64Counter("pdbplus.sync.operations",
		metric.WithDescription("Count of sync operations by status"),
		metric.WithUnit("{operation}"),
	)
	if err != nil {
		return fmt.Errorf("registering pdbplus.sync.operations counter: %w", err)
	}

	return nil
}
