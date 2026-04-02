package otel

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// SyncDuration records the duration of sync operations in seconds.
var SyncDuration metric.Float64Histogram

// SyncOperations counts sync operations by status (success/failed).
var SyncOperations metric.Int64Counter

// SyncTypeDuration records per-type sync step duration in seconds.
var SyncTypeDuration metric.Float64Histogram

// SyncTypeObjects counts objects synced per type.
var SyncTypeObjects metric.Int64Counter

// SyncTypeDeleted counts objects deleted per type.
var SyncTypeDeleted metric.Int64Counter

// SyncTypeFetchErrors counts PeeringDB API fetch errors per type per D-10.
var SyncTypeFetchErrors metric.Int64Counter

// SyncTypeUpsertErrors counts database upsert errors per type per D-10.
var SyncTypeUpsertErrors metric.Int64Counter

// SyncTypeFallback counts incremental-to-full fallback events per type.
var SyncTypeFallback metric.Int64Counter

// RoleTransitions counts LiteFS role transition events (promoted/demoted).
var RoleTransitions metric.Int64Counter

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

	SyncTypeDuration, err = meter.Float64Histogram("pdbplus.sync.type.duration",
		metric.WithDescription("Duration of sync per PeeringDB object type"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.5, 1, 2, 5, 10, 30, 60),
	)
	if err != nil {
		return fmt.Errorf("registering pdbplus.sync.type.duration histogram: %w", err)
	}

	SyncTypeObjects, err = meter.Int64Counter("pdbplus.sync.type.objects",
		metric.WithDescription("Number of objects synced per type"),
		metric.WithUnit("{object}"),
	)
	if err != nil {
		return fmt.Errorf("registering pdbplus.sync.type.objects counter: %w", err)
	}

	SyncTypeDeleted, err = meter.Int64Counter("pdbplus.sync.type.deleted",
		metric.WithDescription("Number of objects deleted per type"),
		metric.WithUnit("{object}"),
	)
	if err != nil {
		return fmt.Errorf("registering pdbplus.sync.type.deleted counter: %w", err)
	}

	SyncTypeFetchErrors, err = meter.Int64Counter("pdbplus.sync.type.fetch_errors",
		metric.WithDescription("PeeringDB API fetch errors per type"),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		return fmt.Errorf("registering pdbplus.sync.type.fetch_errors counter: %w", err)
	}

	SyncTypeUpsertErrors, err = meter.Int64Counter("pdbplus.sync.type.upsert_errors",
		metric.WithDescription("Database upsert errors per type"),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		return fmt.Errorf("registering pdbplus.sync.type.upsert_errors counter: %w", err)
	}

	SyncTypeFallback, err = meter.Int64Counter("pdbplus.sync.type.fallback",
		metric.WithDescription("Incremental-to-full sync fallback events per type"),
		metric.WithUnit("{event}"),
	)
	if err != nil {
		return fmt.Errorf("registering pdbplus.sync.type.fallback counter: %w", err)
	}

	RoleTransitions, err = meter.Int64Counter("pdbplus.role.transitions",
		metric.WithDescription("Role transition events (promoted/demoted)"),
		metric.WithUnit("{event}"),
	)
	if err != nil {
		return fmt.Errorf("registering pdbplus.role.transitions counter: %w", err)
	}

	return nil
}

// InitFreshnessGauge registers the sync freshness observable gauge per D-09.
// The lastSyncFn callback returns the time of the last successful sync.
// Must be called after OTel Setup() and database initialization.
func InitFreshnessGauge(lastSyncFn func(ctx context.Context) (time.Time, bool)) error {
	meter := otel.Meter("peeringdb-plus")
	_, err := meter.Float64ObservableGauge("pdbplus.sync.freshness",
		metric.WithDescription("Seconds since last successful sync"),
		metric.WithUnit("s"),
		metric.WithFloat64Callback(func(_ context.Context, o metric.Float64Observer) error {
			lastSync, ok := lastSyncFn(context.Background()) //nolint:contextcheck // observable callback receives unused context parameter; context.Background() is intentional for DB queries
			if !ok {
				return nil // No observation if no successful sync.
			}
			o.Observe(time.Since(lastSync).Seconds())
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("registering pdbplus.sync.freshness gauge: %w", err)
	}
	return nil
}

// InitObjectCountGauges registers an observable Int64Gauge that reports the
// number of objects stored per PeeringDB type. Reads from a cache function
// that returns pre-computed counts updated at sync completion time (PERF-02).
// Must be called after OTel Setup().
func InitObjectCountGauges(countsFn func() map[string]int64) error {
	meter := otel.Meter("peeringdb-plus")
	_, err := meter.Int64ObservableGauge("pdbplus.data.type.count",
		metric.WithDescription("Number of objects stored per PeeringDB type"),
		metric.WithUnit("{object}"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			counts := countsFn()
			for typeName, count := range counts {
				o.Observe(count, metric.WithAttributes(
					attribute.String("type", typeName),
				))
			}
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("registering pdbplus.data.type.count gauge: %w", err)
	}
	return nil
}
