package otel

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/dotwaffle/peeringdb-plus/ent"
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
			lastSync, ok := lastSyncFn(context.Background())
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

// typeCounter pairs a short PeeringDB type name with a function that returns
// the count of stored objects for that type.
type typeCounter struct {
	name    string
	countFn func(ctx context.Context) (int, error)
}

// InitObjectCountGauges registers an observable Int64Gauge that reports the
// number of objects stored per PeeringDB type. The gauge callback queries the
// ent client for all 13 PeeringDB types on each scrape per D-12, D-13, D-14.
// Must be called after OTel Setup() and database initialization.
func InitObjectCountGauges(client *ent.Client) error {
	counters := []typeCounter{
		{"org", func(ctx context.Context) (int, error) { return client.Organization.Query().Count(ctx) }},
		{"campus", func(ctx context.Context) (int, error) { return client.Campus.Query().Count(ctx) }},
		{"fac", func(ctx context.Context) (int, error) { return client.Facility.Query().Count(ctx) }},
		{"carrier", func(ctx context.Context) (int, error) { return client.Carrier.Query().Count(ctx) }},
		{"carrierfac", func(ctx context.Context) (int, error) { return client.CarrierFacility.Query().Count(ctx) }},
		{"ix", func(ctx context.Context) (int, error) { return client.InternetExchange.Query().Count(ctx) }},
		{"ixlan", func(ctx context.Context) (int, error) { return client.IxLan.Query().Count(ctx) }},
		{"ixpfx", func(ctx context.Context) (int, error) { return client.IxPrefix.Query().Count(ctx) }},
		{"ixfac", func(ctx context.Context) (int, error) { return client.IxFacility.Query().Count(ctx) }},
		{"net", func(ctx context.Context) (int, error) { return client.Network.Query().Count(ctx) }},
		{"poc", func(ctx context.Context) (int, error) { return client.Poc.Query().Count(ctx) }},
		{"netfac", func(ctx context.Context) (int, error) { return client.NetworkFacility.Query().Count(ctx) }},
		{"netixlan", func(ctx context.Context) (int, error) { return client.NetworkIxLan.Query().Count(ctx) }},
	}

	meter := otel.Meter("peeringdb-plus")
	_, err := meter.Int64ObservableGauge("pdbplus.data.type.count",
		metric.WithDescription("Number of objects stored per PeeringDB type"),
		metric.WithUnit("{object}"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			ctx := context.Background()
			for _, c := range counters {
				count, err := c.countFn(ctx)
				if err != nil {
					// Skip this type on error; avoid noisy errors in
					// observable callbacks that run on every scrape.
					continue
				}
				o.Observe(int64(count), metric.WithAttributes(
					attribute.String("type", c.name),
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
