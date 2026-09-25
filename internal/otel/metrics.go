package otel

import (
	"context"
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"

	"github.com/dotwaffle/peeringdb-plus/internal/litefs"
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
var ResponseHeapDeltaBytes metric.Int64Histogram

// SyncOperations counts sync operations by status (success/failed).
var SyncOperations metric.Int64Counter

// SyncTypeObjects counts objects synced per type. The sync worker adds
// the counts of a cycle only after its transaction commits
// (internal/sync/worker.go recordObjectCounts).
var SyncTypeObjects metric.Int64Counter

// SyncTypeDeleted counts the rows that sync itself marks deleted, per type.
// Only the netixlan cascade of a deleted network emits it
// (internal/sync/netixlan_cascade.go), after its transaction commits.
// Tombstones that upstream sends are counted in SyncTypeObjects.
var SyncTypeDeleted metric.Int64Counter

// SyncTypeFetchErrors counts the failed sync fetch steps per type. The
// cause can be a cursor read, a PeeringDB request or a scratch write
// (internal/sync/worker.go failFetchStep).
var SyncTypeFetchErrors metric.Int64Counter

// SyncTypeUpsertErrors counts database upsert errors per type.
var SyncTypeUpsertErrors metric.Int64Counter

// SyncTypeFallback counts incremental-to-full fallback events per type.
var SyncTypeFallback metric.Int64Counter

// SyncTypeOrphans counts FK-orphan rows observed per sync cycle, broken
// down by {type, parent_type, field, action} where action is "drop"
// (row excluded entirely from upsert) or "null" (FK column nulled but
// row kept). Provides the per-cycle aggregate that replaces the per-row
// WARN logs which previously blew Tempo's 7.5 MB per-trace budget.
var SyncTypeOrphans metric.Int64Counter

// SyncFKBackfill counts live FK-backfill attempts during sync, broken down
// by {type, parent_type, result} where result ∈ {hit, miss, ratelimited,
// error}. When fkCheckParent finds a missing
// parent, sync attempts one upstream fetch via ?since=1&id__in=N before
// declaring the child an orphan. Cap-hit (per-cycle budget exhausted),
// HTTP failures, and "parent truly absent upstream" each get their own
// result label so dashboards can split successful recoveries from
// rate-limit pressure.
var SyncFKBackfill metric.Int64Counter

// SyncLockRetries counts the retries of short primary writes after a
// transient SQLite lock error (SQLITE_BUSY or SQLITE_PROTOCOL), by op:
// the sync_status INSERT and UPDATE and the startup poc scrub and
// netixlan cascade transactions (internal/sync/lockretry.go). The sync
// transaction and the sync_status prune are not retried. Cardinality: 5
// op values.
var SyncLockRetries metric.Int64Counter

// SyncHistoryRequests counts the upstream requests of the history sweep
// (internal/sync/history_sweep.go), by type and result: ok, rate_limited
// (429 or WAF block) or error. Cardinality: 12 type values x 3 results.
var SyncHistoryRequests metric.Int64Counter

// PeeringDBRequests counts outbound HTTP requests to the PeeringDB API by
// status_class ∈ {2xx, 3xx, 4xx, 5xx, network_error}.
// The sync-level fk_backfill counter only sees post-decision events;
// this counter sees every request the transport makes (including retries).
// Cardinality: 5 values × 1 metric = 5 series (well under any concern).
var PeeringDBRequests metric.Int64Counter

// PeeringDBRetries counts in-transport retries broken down by cause ∈
// {429, 5xx, network_error}. The 429 axis catches
// upstream rate-limit pressure that the limiter under-provisioned for;
// 5xx catches upstream instability; network_error catches conn/DNS
// failures. The application-level 5xx ladder in doWithRetry also bumps
// the 5xx counter — both are intentional (every retry is a retry).
var PeeringDBRetries metric.Int64Counter

// PeeringDBRateLimitWaitMS is a histogram of per-request rate-limiter
// wait durations in milliseconds. Replaces the
// span-event-only signal with a metric so operators can see p50/p95/p99
// without enabling sampled tracing. Bucket boundaries cover 0 (always-
// available bursts) up to 5s (the biggest gap a 1/3s limiter can impose
// on a single Wait call).
var PeeringDBRateLimitWaitMS metric.Float64Histogram

// RoleTransitions counts LiteFS role transition events (promoted/demoted).
var RoleTransitions metric.Int64Counter

func init() {
	BindInstruments()
}

// BindInstruments (re)creates every package-level instrument on the
// current global MeterProvider. It runs automatically at package init,
// so the instruments are never nil and callers need no nil-guards or
// init-ordering discipline: instruments created before the first
// otel.SetMeterProvider are delegating shims that bind to the real
// provider when main wires it.
//
// That delegation happens exactly ONCE per process (otel's global
// delegateMeterOnce). Production sets one provider at startup and never
// needs this function. Tests that install a fresh MeterProvider after
// another provider was already set in the same process MUST call
// BindInstruments afterwards to rebind the instruments to the new
// provider — otherwise recorded values flow to the previously-bound
// provider and the test's reader collects nothing.
func BindInstruments() {
	SyncDuration = mustFloat64Histogram("pdbplus.sync.duration",
		metric.WithDescription("Duration of sync operations in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(1, 5, 10, 30, 60, 120, 300),
	)
	SyncOperations = mustInt64Counter("pdbplus.sync.operations",
		metric.WithDescription("Count of sync operations by status"),
		metric.WithUnit("{operation}"),
	)
	SyncTypeObjects = mustInt64Counter("pdbplus.sync.type.objects",
		metric.WithDescription("Number of objects synced per type"),
		metric.WithUnit("{object}"),
	)
	SyncTypeDeleted = mustInt64Counter("pdbplus.sync.type.deleted",
		metric.WithDescription("Rows that sync marked deleted, per type (netixlan cascade)"),
		metric.WithUnit("{object}"),
	)
	SyncTypeFetchErrors = mustInt64Counter("pdbplus.sync.type.fetch_errors",
		metric.WithDescription("Sync fetch-step errors per type (cursor read, PeeringDB request, scratch write)"),
		metric.WithUnit("{error}"),
	)
	SyncTypeUpsertErrors = mustInt64Counter("pdbplus.sync.type.upsert_errors",
		metric.WithDescription("Database upsert errors per type"),
		metric.WithUnit("{error}"),
	)
	SyncTypeFallback = mustInt64Counter("pdbplus.sync.type.fallback",
		metric.WithDescription("Incremental-to-full sync fallback events per type"),
		metric.WithUnit("{event}"),
	)
	SyncTypeOrphans = mustInt64Counter("pdbplus.sync.type.orphans",
		metric.WithDescription("FK-orphan rows observed per sync cycle, by type/parent_type/field/action"),
		metric.WithUnit("{row}"),
	)
	SyncFKBackfill = mustInt64Counter("pdbplus.sync.fk_backfill",
		metric.WithDescription("Live FK-backfill attempts during sync, by type/parent_type/result"),
		metric.WithUnit("{attempt}"),
	)
	SyncLockRetries = mustInt64Counter("pdbplus.sync.lock_retries",
		metric.WithDescription("Retries of short primary writes after a transient SQLite lock error, by op"),
		metric.WithUnit("{retry}"),
	)
	SyncHistoryRequests = mustInt64Counter("pdbplus.sync.history.requests",
		metric.WithDescription("Upstream requests of the history sweep, by type/result"),
		metric.WithUnit("{request}"),
	)
	PeeringDBRequests = mustInt64Counter("pdbplus.peeringdb.requests",
		metric.WithDescription("Outbound HTTP requests to PeeringDB API, by status_class"),
		metric.WithUnit("{request}"),
	)
	PeeringDBRetries = mustInt64Counter("pdbplus.peeringdb.retries",
		metric.WithDescription("In-transport PeeringDB request retries, by cause"),
		metric.WithUnit("{retry}"),
	)
	PeeringDBRateLimitWaitMS = mustFloat64Histogram("pdbplus.peeringdb.rate_limit_wait_ms",
		metric.WithDescription("Per-request PeeringDB rate-limiter wait duration in milliseconds"),
		metric.WithUnit("ms"),
		metric.WithExplicitBucketBoundaries(0, 1, 10, 50, 100, 250, 500, 1000, 2500, 5000),
	)
	RoleTransitions = mustInt64Counter("pdbplus.role.transitions",
		metric.WithDescription("Role transition events (promoted/demoted)"),
		metric.WithUnit("{event}"),
	)
	ResponseHeapDeltaBytes = mustInt64Histogram("pdbplus.response.heap_delta",
		metric.WithDescription("Per-request Go heap HeapInuse delta on pdbcompat list handlers, in bytes"),
		metric.WithUnit("By"),
		// Buckets span 512 B to 512 MiB: the low end catches
		// near-zero-delta responses, the high end the budget-breach
		// neighbourhood (PDBPLUS_RESPONSE_MEMORY_LIMIT default
		// 128 MiB; buckets step past that so outliers still count).
		metric.WithExplicitBucketBoundaries(
			512, 1024, 4096, 16384, 65536, 262144, 1048576, 4194304,
			16777216, 67108864, 268435456, 536870912,
		),
	)
}

// The must* helpers panic on registration failure. The global
// (delegating) meter never fails; an SDK meter fails only on an invalid
// instrument name or unit — a programming error caught by any test run.

func mustInt64Counter(name string, opts ...metric.Int64CounterOption) metric.Int64Counter {
	c, err := otel.Meter("peeringdb-plus").Int64Counter(name, opts...)
	if err != nil {
		panic(fmt.Sprintf("otel: registering %s counter: %v", name, err))
	}
	return c
}

func mustInt64Histogram(name string, opts ...metric.Int64HistogramOption) metric.Int64Histogram {
	h, err := otel.Meter("peeringdb-plus").Int64Histogram(name, opts...)
	if err != nil {
		panic(fmt.Sprintf("otel: registering %s histogram: %v", name, err))
	}
	return h
}

func mustFloat64Histogram(name string, opts ...metric.Float64HistogramOption) metric.Float64Histogram {
	h, err := otel.Meter("peeringdb-plus").Float64Histogram(name, opts...)
	if err != nil {
		panic(fmt.Sprintf("otel: registering %s histogram: %v", name, err))
	}
	return h
}

// InitFreshnessGauge registers the sync freshness observable gauge.
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
// end-of-sync-cycle peak heap and RSS (bytes) for the sync-memory dashboard
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

// InitScratchFreeGauge registers the pdbplus.scratch.free gauge: the free
// space of the file system of dir (PDBPLUS_SCRATCH_DIR), in bytes, as
// free reports it. On Fly.io the dir of the primary is on its LiteFS
// volume. Each collection calls free once. An error, or a value above
// math.MaxInt64 (free space not known), observes nothing.
func InitScratchFreeGauge(dir string, free func(dir string) (uint64, error)) error {
	meter := otel.Meter("peeringdb-plus")
	_, err := meter.Int64ObservableGauge("pdbplus.scratch.free",
		metric.WithDescription("Free space of the file system of PDBPLUS_SCRATCH_DIR, in bytes"),
		metric.WithUnit("By"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			v, err := free(dir)
			if err != nil || v > math.MaxInt64 {
				return nil
			}
			o.Observe(int64(v))
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("registering pdbplus.scratch.free gauge: %w", err)
	}
	return nil
}

// InitBuildInfoGauge registers the pdbplus.build.info gauge: the value 1
// with the attribute service.version set to version. The metric resource
// has no service.version (buildMetricResource), so this one series per
// machine carries the version on the metrics path, and a deploy does not
// start a new copy of every other series.
// Must be called after OTel Setup().
func InitBuildInfoGauge(version string) error {
	meter := otel.Meter("peeringdb-plus")
	attrs := metric.WithAttributeSet(attribute.NewSet(semconv.ServiceVersion(version)))
	// No unit: the Prometheus translation would add a suffix to the
	// name (unit "1" on a gauge gives pdbplus_build_info_ratio).
	_, err := meter.Int64ObservableGauge("pdbplus.build.info",
		metric.WithDescription("Build of the running process; the value is always 1"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(1, attrs)
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("registering pdbplus.build.info gauge: %w", err)
	}
	return nil
}

// InitObjectCountGauges registers an observable Int64Gauge that reports the
// number of objects stored per PeeringDB type. Reads from a cache function
// that returns pre-computed counts updated at sync completion time.
// A collection observes nothing while isPrimary returns false: only the
// primary runs the sync worker that updates the cache, so the counts of a
// replica stay at their values from process start.
// Must be called after OTel Setup().
func InitObjectCountGauges(countsFn func() map[string]int64, isPrimary func() bool) error {
	meter := otel.Meter("peeringdb-plus")
	_, err := meter.Int64ObservableGauge("pdbplus.data.type.count",
		metric.WithDescription("Number of objects stored per PeeringDB type"),
		metric.WithUnit("{object}"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			if !isPrimary() {
				return nil
			}
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

// InitLiteFSGauges registers instruments that report the LiteFS metrics
// of the local node. One collection calls scrape once and observes the
// instruments from its result. An instrument whose LiteFS series is
// absent (a nil field of litefs.Metrics) gets no value. When scrape
// fails, the collection observes no LiteFS values, so a dashboard shows
// a gap instead of zeros. The caller logs scrape failures.
//
// Register the instruments only when the LiteFS metrics endpoint is
// configured. LiteFS runs only in the Fly.io deployment.
func InitLiteFSGauges(scrape func(ctx context.Context) (litefs.Metrics, error)) error {
	meter := otel.Meter("peeringdb-plus")
	txid, err := meter.Int64ObservableGauge("pdbplus.litefs.txid",
		metric.WithDescription("Current LiteFS transaction ID of the database"),
		metric.WithUnit("{transaction}"),
	)
	if err != nil {
		return fmt.Errorf("registering pdbplus.litefs.txid gauge: %w", err)
	}
	commits, err := meter.Int64ObservableCounter("pdbplus.litefs.commits",
		metric.WithDescription("Database commits on this node since LiteFS started; a replica does not commit"),
		metric.WithUnit("{commit}"),
	)
	if err != nil {
		return fmt.Errorf("registering pdbplus.litefs.commits counter: %w", err)
	}
	ltxSize, err := meter.Int64ObservableGauge("pdbplus.litefs.ltx.size",
		metric.WithDescription("LiteFS litefs_db_ltx_bytes: size of the newest LTX file after a commit, size of the retained LTX files after each retention pass"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return fmt.Errorf("registering pdbplus.litefs.ltx.size gauge: %w", err)
	}
	ltxFiles, err := meter.Int64ObservableGauge("pdbplus.litefs.ltx.files",
		metric.WithDescription("Number of LTX files that LiteFS keeps on disk for the database"),
		metric.WithUnit("{file}"),
	)
	if err != nil {
		return fmt.Errorf("registering pdbplus.litefs.ltx.files gauge: %w", err)
	}
	ltxLag, err := meter.Float64ObservableGauge("pdbplus.litefs.ltx.lag",
		metric.WithDescription("Time from the creation of the last LTX file applied on this node to its apply"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return fmt.Errorf("registering pdbplus.litefs.ltx.lag gauge: %w", err)
	}
	lag, err := meter.Float64ObservableGauge("pdbplus.litefs.lag",
		metric.WithDescription("Time since this node last received a frame from the LiteFS primary; 0 on the primary"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return fmt.Errorf("registering pdbplus.litefs.lag gauge: %w", err)
	}
	subscribers, err := meter.Int64ObservableGauge("pdbplus.litefs.subscribers",
		metric.WithDescription("Replicas connected to this LiteFS node"),
		metric.WithUnit("{subscriber}"),
	)
	if err != nil {
		return fmt.Errorf("registering pdbplus.litefs.subscribers gauge: %w", err)
	}
	_, err = meter.RegisterCallback(func(ctx context.Context, o metric.Observer) error {
		m, err := scrape(ctx)
		if err != nil {
			return nil //nolint:nilerr // scrape logs its failures; returning the error would log it again on every collection.
		}
		o.ObserveInt64(txid, m.TXID)
		o.ObserveInt64(commits, m.Commits)
		// A nil value is a series that LiteFS has not created yet. It
		// leaves a gap, not a zero.
		if m.LTXBytes != nil {
			o.ObserveInt64(ltxSize, *m.LTXBytes)
		}
		if m.LTXFiles != nil {
			o.ObserveInt64(ltxFiles, *m.LTXFiles)
		}
		if m.LTXLagSeconds != nil {
			o.ObserveFloat64(ltxLag, *m.LTXLagSeconds)
		}
		o.ObserveFloat64(lag, m.LagSeconds)
		o.ObserveInt64(subscribers, m.Subscribers)
		return nil
	}, txid, commits, ltxSize, ltxFiles, ltxLag, lag, subscribers)
	if err != nil {
		return fmt.Errorf("registering litefs metrics callback: %w", err)
	}
	return nil
}
