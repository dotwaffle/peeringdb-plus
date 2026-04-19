package otel

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// SyncPeakHeapMiB holds the most-recent end-of-sync-cycle Go runtime
// HeapInuse in MiB. Exposed via pdbplus.sync.peak_heap_mib ObservableGauge
// for Grafana / Prometheus dashboards to plot over time. Zero means
// "no sync has completed yet" — the gauge suppresses observation until
// the first store.
var SyncPeakHeapMiB atomic.Int64

// SyncPeakRSSMiB holds the most-recent end-of-sync-cycle /proc/self/status
// VmHWM in MiB (Linux only). Zero means "not Linux" or "no sync yet" —
// the gauge suppresses observation when zero.
var SyncPeakRSSMiB atomic.Int64

// SyncDuration records the duration of sync operations in seconds.
var SyncDuration metric.Float64Histogram

// ResponseHeapDeltaKiB records the per-request Go heap HeapInuse delta
// (exit - entry) for pdbcompat list handlers, in KiB. Populated by
// internal/pdbcompat.recordResponseHeapDelta via defer at the top of
// serveList. Unit is KiB (vs sync's MiB) because per-request deltas
// are typically 10-1000× smaller than per-cycle peaks.
//
// Attributes: endpoint (e.g. "/api/net"), entity (e.g. "net"). Low-cardinality
// by construction — 1 endpoint per type × 13 types = 13 label combinations.
//
// Registered by InitResponseHeapHistogram (Phase 71 Plan 05, D-06).
var ResponseHeapDeltaKiB metric.Int64Histogram

// SyncOperations counts sync operations by status (success/failed).
var SyncOperations metric.Int64Counter

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

// InitMemoryGauges registers observable gauges that report the most-recent
// end-of-sync-cycle peak heap and RSS (MiB) for SEED-001 dashboard watch.
// Values are updated by internal/sync.(*Worker).emitMemoryTelemetry via the
// SyncPeakHeapMiB / SyncPeakRSSMiB atomics. A zero value suppresses the
// observation (no sync yet, or non-Linux for RSS) so dashboards don't plot
// misleading zeros.
func InitMemoryGauges() error {
	meter := otel.Meter("peeringdb-plus")
	_, err := meter.Int64ObservableGauge("pdbplus.sync.peak_heap_mib",
		metric.WithDescription("Peak Go heap (HeapInuse) at end of last sync cycle, in MiB"),
		metric.WithUnit("MiB"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			if v := SyncPeakHeapMiB.Load(); v > 0 {
				o.Observe(v)
			}
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("registering pdbplus.sync.peak_heap_mib gauge: %w", err)
	}
	_, err = meter.Int64ObservableGauge("pdbplus.sync.peak_rss_mib",
		metric.WithDescription("Peak OS RSS (/proc/self/status VmHWM) at end of last sync cycle, in MiB"),
		metric.WithUnit("MiB"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			if v := SyncPeakRSSMiB.Load(); v > 0 {
				o.Observe(v)
			}
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("registering pdbplus.sync.peak_rss_mib gauge: %w", err)
	}
	return nil
}

// InitResponseHeapHistogram registers the per-request heap-delta histogram
// for pdbcompat response paths (Phase 71 D-06, MEMORY-03). Called from
// main.go at startup after the OTel SDK is ready. Analogous to
// InitMemoryGauges but records a histogram (distribution over p50/p95/p99)
// rather than a point-in-time gauge, because per-request deltas are the
// thing operators want a distribution over — not a last-write-wins value.
//
// Bucket boundaries span 0.5 KiB to 512 MiB: the low end catches
// near-zero-delta responses (cached small payloads), the high end the
// budget-breach neighbourhood (PDBPLUS_RESPONSE_MEMORY_LIMIT default 128
// MiB in KiB = 131072; buckets step past that into 256/512 MiB territory
// so outliers still get counted).
func InitResponseHeapHistogram() error {
	meter := otel.Meter("peeringdb-plus")
	h, err := meter.Int64Histogram("pdbplus.response.heap_delta_kib",
		metric.WithDescription("Per-request Go heap HeapInuse delta on pdbcompat list handlers, in KiB"),
		metric.WithUnit("KiB"),
		metric.WithExplicitBucketBoundaries(0.5, 1, 4, 16, 64, 256, 1024, 4096, 16384, 65536, 262144, 524288),
	)
	if err != nil {
		return fmt.Errorf("registering pdbplus.response.heap_delta_kib histogram: %w", err)
	}
	ResponseHeapDeltaKiB = h
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
