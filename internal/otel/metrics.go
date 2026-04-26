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

// SyncPeakHeapBytes holds the most-recent end-of-sync-cycle Go runtime
// HeapInuse in bytes. Exposed via pdbplus.sync.peak_heap ObservableGauge
// (Prom name: pdbplus_sync_peak_heap_bytes) for Grafana / Prometheus
// dashboards to plot over time. Zero means "no sync has completed yet"
// — the gauge suppresses observation until the first store. Bytes is
// the canonical Prom unit; dashboards format MiB / GiB at render time.
var SyncPeakHeapBytes atomic.Int64

// SyncPeakRSSBytes holds the most-recent end-of-sync-cycle
// /proc/self/status VmHWM in bytes (Linux only). Zero means "not Linux"
// or "no sync yet" — the gauge suppresses observation when zero.
var SyncPeakRSSBytes atomic.Int64

// SyncDuration records the duration of sync operations in seconds.
var SyncDuration metric.Float64Histogram

// ResponseHeapDeltaBytes records the per-request Go heap HeapInuse delta
// (exit - entry) for pdbcompat list handlers, in bytes. Populated by
// internal/pdbcompat.recordResponseHeapDelta via defer at the top of
// serveList.
//
// Attributes: endpoint (e.g. "/api/net"), entity (e.g. "net"). Low-cardinality
// by construction — 1 endpoint per type × 13 types = 13 label combinations.
//
// Registered by InitResponseHeapHistogram (Phase 71 Plan 05, D-06).
var ResponseHeapDeltaBytes metric.Int64Histogram

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

// SyncTypeOrphans counts FK-orphan rows observed per sync cycle, broken
// down by {type, parent_type, field, action} where action is "drop"
// (row excluded entirely from upsert) or "null" (FK column nulled but
// row kept). Provides the per-cycle aggregate that replaces the per-row
// WARN logs which previously blew Tempo's 7.5 MB per-trace budget.
var SyncTypeOrphans metric.Int64Counter

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

	SyncTypeOrphans, err = meter.Int64Counter("pdbplus.sync.type.orphans",
		metric.WithDescription("FK-orphan rows observed per sync cycle, by type/parent_type/field/action"),
		metric.WithUnit("{row}"),
	)
	if err != nil {
		return fmt.Errorf("registering pdbplus.sync.type.orphans counter: %w", err)
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
// end-of-sync-cycle peak heap and RSS (bytes) for SEED-001 dashboard
// watch. Values are updated by internal/sync.(*Worker).emitMemoryTelemetry
// via the SyncPeakHeapBytes / SyncPeakRSSBytes atomics. A zero value
// suppresses the observation (no sync yet, or non-Linux for RSS) so
// dashboards don't plot misleading zeros.
//
// Bytes is the canonical Prom unit (post-2026-04-26 audit); dashboards
// format MiB / GiB at render time via Grafana's "bytes" field unit.
func InitMemoryGauges() error {
	meter := otel.Meter("peeringdb-plus")
	_, err := meter.Int64ObservableGauge("pdbplus.sync.peak_heap",
		metric.WithDescription("Peak Go heap (HeapInuse) at end of last sync cycle, in bytes"),
		metric.WithUnit("By"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			if v := SyncPeakHeapBytes.Load(); v > 0 {
				o.Observe(v)
			}
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("registering pdbplus.sync.peak_heap gauge: %w", err)
	}
	_, err = meter.Int64ObservableGauge("pdbplus.sync.peak_rss",
		metric.WithDescription("Peak OS RSS (/proc/self/status VmHWM) at end of last sync cycle, in bytes"),
		metric.WithUnit("By"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			if v := SyncPeakRSSBytes.Load(); v > 0 {
				o.Observe(v)
			}
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("registering pdbplus.sync.peak_rss gauge: %w", err)
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
// Bucket boundaries span 512 B to 512 MiB (in bytes per the post-2026-04-26
// audit unit canonicalisation): the low end catches near-zero-delta
// responses (cached small payloads), the high end the budget-breach
// neighbourhood (PDBPLUS_RESPONSE_MEMORY_LIMIT default 128 MiB; buckets
// step past that into 256 / 512 MiB territory so outliers still get
// counted).
func InitResponseHeapHistogram() error {
	meter := otel.Meter("peeringdb-plus")
	h, err := meter.Int64Histogram("pdbplus.response.heap_delta",
		metric.WithDescription("Per-request Go heap HeapInuse delta on pdbcompat list handlers, in bytes"),
		metric.WithUnit("By"),
		metric.WithExplicitBucketBoundaries(
			512, 1024, 4096, 16384, 65536, 262144, 1048576, 4194304,
			16777216, 67108864, 268435456, 536870912,
		),
	)
	if err != nil {
		return fmt.Errorf("registering pdbplus.response.heap_delta histogram: %w", err)
	}
	ResponseHeapDeltaBytes = h
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
