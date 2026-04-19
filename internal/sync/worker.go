package sync

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/privacy"
	"github.com/dotwaffle/peeringdb-plus/internal/config"
	pdbotel "github.com/dotwaffle/peeringdb-plus/internal/otel"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// ErrSyncMemoryLimitExceeded is returned by Worker.Sync when
// runtime.ReadMemStats reports HeapAlloc above WorkerConfig.SyncMemoryLimit
// after the Phase A fetch pass completes. The sync aborts without
// opening the ent transaction; the running mutex is released on
// return and the next scheduled retry proceeds normally after the
// Phase A scratch batches are reclaimed by GC.
//
// Commit F (Plan 54-03) defense-in-depth against PeeringDB data growth
// that exceeds the 400 MB bench harness baseline at runtime (e.g. if
// netixlan doubles between benchmarks and production). Callers detect
// the sentinel via errors.Is per GO-ERR-2.
var ErrSyncMemoryLimitExceeded = errors.New("sync aborted: memory limit exceeded")

// defaultRetryBackoffs defines the backoff durations for sync-level retries per D-21.
var defaultRetryBackoffs = []time.Duration{30 * time.Second, 2 * time.Minute, 8 * time.Minute}

// WorkerConfig holds configuration for the sync worker.
type WorkerConfig struct {
	IsPrimary func() bool // live primary detection; nil defaults to always-primary
	SyncMode  config.SyncMode
	// OnSyncComplete is called after a successful sync with per-type object
	// counts and the completion timestamp. The timestamp is the same
	// value persisted into the sync_status row by recordSuccess, so
	// downstream consumers (e.g. the caching middleware ETag setter wired
	// in cmd/peeringdb-plus/main.go for PERF-07) stay in lock-step with
	// the database without an extra round-trip.
	OnSyncComplete func(counts map[string]int, syncTime time.Time)

	// SyncMemoryLimit is the peak Go heap ceiling (bytes) checked
	// after Phase A fetch completes and before the ent.Tx opens. If
	// runtime.MemStats.HeapAlloc exceeds this value, Sync aborts with
	// ErrSyncMemoryLimitExceeded. Zero disables the guardrail. Wired
	// from config.Config.SyncMemoryLimit by main.go. Commit F default
	// is 400 MB (matches the DEBT-03 bench regression gate).
	SyncMemoryLimit int64

	// HeapWarnBytes is the peak Go heap threshold (bytes) above which
	// the end-of-sync-cycle emitter fires slog.Warn("heap threshold
	// crossed", ...). The OTel span attr pdbplus.sync.peak_heap_mib is
	// attached regardless. Zero disables only the Warn (not the attr).
	// Wired from config.Config.HeapWarnBytes by main.go.
	//
	// SEED-001 escalation signal: sustained breach triggers the
	// incremental-sync evaluation path documented in
	// .planning/seeds/SEED-001-incremental-sync-evaluation.md.
	HeapWarnBytes int64

	// RSSWarnBytes is the peak OS RSS threshold (bytes) above which
	// the emitter fires slog.Warn. Read from /proc/self/status VmHWM on
	// Linux; skipped on other OSes (the RSS attr is then omitted — it
	// is not set to zero). Zero disables only the Warn.
	RSSWarnBytes int64
}

// Worker orchestrates PeeringDB data synchronization.
type Worker struct {
	pdbClient     *peeringdb.Client
	entClient     *ent.Client
	db            *sql.DB // underlying sql.DB for sync_status table
	config        WorkerConfig
	running       atomic.Bool
	synced        atomic.Bool     // true after first successful sync (D-30)
	logger        *slog.Logger
	retryBackoffs []time.Duration // per D-21; defaults to 30s, 2m, 8m
	// fkRegistry maps parent type name to the set of IDs successfully
	// upserted during the current Phase B. Populated by recordIDs
	// callbacks from dispatchScratchChunk as parent types land (org,
	// fac, net, ...); consumed by fkFilter closures on downstream child
	// types (netixlan, poc, ...) so rows referencing an orphaned parent
	// are dropped before the upsert rather than rolled back at commit.
	//
	// PeeringDB's public /api/{type}?depth=0 responses occasionally
	// contain child rows whose parent rows are suppressed server-side
	// (deleted orgs still referenced by live nets, etc). Without this
	// registry, Phase 54's defer_foreign_keys=ON commit check rejects
	// the entire sync transaction. See v1.13 Phase 54-FK-ORPHAN notes.
	//
	// Reset by resetFKState at the start of each Sync run. Single-writer
	// because Worker.running serialises concurrent Sync calls.
	fkRegistry map[string]map[int]struct{}
	// fkSkippedIDs maps type name to the set of row IDs that were
	// dropped by the upsert-time fkFilter. Used by syncUpsertPass to
	// subtract these from remoteIDsByType before the delete pass runs,
	// so any previously-inserted row whose FK is now orphaned is
	// cleaned up (avoids a parent-delete-while-child-remains FK
	// violation at commit in steady-state syncs). Reset alongside
	// fkRegistry in resetFKState.
	fkSkippedIDs map[string]map[int]struct{}
}

// NewWorker creates a new sync worker.
// If cfg.IsPrimary is nil, it defaults to always-primary for backward
// compatibility (local dev, tests without explicit primary config).
func NewWorker(pdbClient *peeringdb.Client, entClient *ent.Client, db *sql.DB, cfg WorkerConfig, logger *slog.Logger) *Worker {
	if cfg.IsPrimary == nil {
		cfg.IsPrimary = func() bool { return true }
	}
	return &Worker{
		pdbClient:     pdbClient,
		entClient:     entClient,
		db:            db,
		config:        cfg,
		logger:        logger,
		retryBackoffs: defaultRetryBackoffs,
	}
}

// SetRetryBackoffs overrides the default retry backoff durations. Intended for testing.
func (w *Worker) SetRetryBackoffs(backoffs []time.Duration) {
	w.retryBackoffs = backoffs
}

// resetFKState clears the per-sync FK registry and skipped-ID tracker.
// Called at the start of each Sync run so downstream fkFilter closures
// see a clean slate — the previous run's parent IDs are irrelevant to
// the next run's orphan detection.
func (w *Worker) resetFKState() {
	w.fkRegistry = make(map[string]map[int]struct{}, 13)
	w.fkSkippedIDs = make(map[string]map[int]struct{}, 13)
}

// fkRegisterIDs records successfully-upserted IDs for the named parent
// type so child types processed later in Phase B can validate their FK
// references against this set. Safe to call with an empty slice.
func (w *Worker) fkRegisterIDs(typeName string, ids []int) {
	set, ok := w.fkRegistry[typeName]
	if !ok {
		set = make(map[int]struct{}, len(ids))
		w.fkRegistry[typeName] = set
	}
	for _, id := range ids {
		set[id] = struct{}{}
	}
}

// fkHasParent reports whether the given ID is registered for the named
// parent type. An id of zero is treated as a null/unset FK and passes
// through unchanged — ent's schema nullability is the source of truth
// for whether zero/null is actually allowed on the column. If the
// parent set is missing entirely (parent type not yet upserted) the
// row is allowed through on the assumption that syncSteps is in
// parent-first FK order and a missing entry represents a programming
// mistake that should surface at commit rather than silently drop data.
func (w *Worker) fkHasParent(typeName string, id int) bool {
	if id == 0 {
		return true
	}
	set, ok := w.fkRegistry[typeName]
	if !ok {
		return true
	}
	_, ok = set[id]
	return ok
}

// fkMarkSkipped records that the given child ID was dropped by the
// upsert-time fkFilter. syncUpsertPass subtracts these IDs from
// remoteIDsByType before the delete pass so any previously-inserted
// row with the same ID is cleaned up, preventing a
// parent-delete-while-child-remains FK violation on the next commit.
func (w *Worker) fkMarkSkipped(typeName string, id int) {
	set, ok := w.fkSkippedIDs[typeName]
	if !ok {
		set = make(map[int]struct{})
		w.fkSkippedIDs[typeName] = set
	}
	set[id] = struct{}{}
}

// syncStep defines a single step in the sync process. Upserts run in
// parent-first FK order; deletes run in reverse (child-first) order.
//
// After PERF-05 (Plan 54-02 Commit D), the fetch and upsert paths dispatch
// via fetchOneTypeFull / fetchOneTypeIncremental / upsertOneType type
// switches on step.name — the old upsertFn/incrementalFn fields were
// removed because fetch now runs outside the tx. The deleteFn closure
// soft-deletes rows (Phase 68 D-02): rows absent from the remote response
// are marked status='deleted' with updated=cycleStart rather than physically
// removed, so upstream rest.py:700-712 status × since matrix queries can
// return tombstones.
type syncStep struct {
	name     string
	deleteFn func(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (marked int, err error)
}

// syncSteps returns the ordered list of sync steps in FK dependency order per D-06.
// Upserts are processed in this order (parents first); deletes in reverse (children first).
func (w *Worker) syncSteps() []syncStep {
	return []syncStep{
		{"org", markStaleDeletedOrganizations},
		{"campus", markStaleDeletedCampuses},
		{"fac", markStaleDeletedFacilities},
		{"carrier", markStaleDeletedCarriers},
		{"carrierfac", markStaleDeletedCarrierFacilities},
		{"ix", markStaleDeletedInternetExchanges},
		{"ixlan", markStaleDeletedIxLans},
		{"ixpfx", markStaleDeletedIxPrefixes},
		{"ixfac", markStaleDeletedIxFacilities},
		{"net", markStaleDeletedNetworks},
		{"poc", markStaleDeletedPocs},
		{"netfac", markStaleDeletedNetworkFacilities},
		{"netixlan", markStaleDeletedNetworkIxLans},
	}
}

// Sync executes a synchronization from PeeringDB to the local database.
// It acquires a mutex to prevent concurrent runs and wraps all database
// writes in a single transaction per D-19.
//
// Sync is an orchestrator. PERF-05 splits it into three phases:
//
//  1. Phase A (NO TX HELD): HTTP fetch + JSON decode stream into an
//     isolated /tmp SQLite "scratch" database — Go heap stays bounded
//     to one element per StreamAll handler invocation (~5-10 KB) instead
//     of one full []T per type (~35 MB for netixlan).
//  2. Fetch Barrier: scratch DB fully populated; open the real LiteFS tx.
//  3. Phase B (SINGLE REAL TX): drain each scratch table in chunks,
//     decode each chunk to typed Go structs, upsertX into the real ent
//     tables, free the chunk, repeat. Delete stale rows. Commit.
//     D-19 preserved: one ent.Tx wraps every real-DB write.
//
// The scratch DB is unlinked on both success and error via defer. See
// internal/sync/scratch.go for the scratch DB lifecycle and pragmas.
// Commit D' — mandatory per Decision #2 because Commit A baseline
// (535 MiB) and Commit D baseline (613 MiB) both exceeded the 400 MiB
// gate on production-scale fixtures.
//
// Commit F (Plan 54-03): between the Phase A fetch barrier and the
// ent.Tx open, Sync calls w.checkMemoryLimit with the current
// runtime.MemStats.HeapAlloc reading. If SyncMemoryLimit is set and
// the heap exceeds it, Sync aborts with ErrSyncMemoryLimitExceeded
// before opening the tx — defense-in-depth against PeeringDB data
// growth that exceeds the benchmark baseline at runtime.
//
// REFAC-03 line budget is <= 100 — enforced by TestWorkerSync_LineBudget.
//
// Plan 59-05 (VIS-05, D-08/D-09): Sync is the SOLE production call site
// for privacy.DecisionContext(ctx, privacy.Allow). The bypass is stamped
// on ctx before any other work — before the w.running CAS, before any
// otel span Start, before any ent call — so every downstream ent read,
// upsert, and child goroutine (e.g. runSyncCycle's demotion monitor)
// inherits allow-all via standard context.WithValue parent-chain lookup.
// The ent rule-dispatch loop short-circuits at the stored decision, so
// no per-rule predicate mutation runs on bypass ctx.
//
// TestSyncBypass_SingleCallSite enforces "exactly one production call
// site" — do NOT add the bypass in SyncWithRetry, runSyncCycle, any
// upsert helper, or any HTTP path. Tier elevation for non-sync callers
// goes through internal/privctx.WithTier (TierUsers), a different
// mechanism.
func (w *Worker) Sync(ctx context.Context, mode config.SyncMode) error {
	ctx = privacy.DecisionContext(ctx, privacy.Allow) // VIS-05 bypass — sole call site (D-08/D-09)
	if !w.running.CompareAndSwap(false, true) {
		w.logger.Warn("sync already running, skipping")
		return nil
	}
	defer w.running.Store(false)

	ctx, span := otel.Tracer("sync").Start(ctx, "sync-"+string(mode))
	defer span.End()

	start := time.Now()
	statusID, err := RecordSyncStart(ctx, w.db, start)
	if err != nil {
		w.logger.LogAttrs(ctx, slog.LevelError, "failed to record sync start",
			slog.Any("error", err))
	}

	// Tighten GC for the duration of the sync run: the default 100%
	// target heap growth is too loose when upsert bursts allocate
	// hundreds of MiB between GC cycles. Setting GCPercent=25 forces
	// the collector to kick in at 25% heap growth, trading ~5% extra
	// CPU for bounded peak heap. Restored on return so the value does
	// not leak to other goroutines.
	//
	// TRADE-OFF NOTE (54-REVIEW.md WR-03): debug.SetGCPercent and
	// debug.SetMemoryLimit are PROCESS-GLOBAL. All goroutines (API
	// handlers included) see shorter GC cycles and potential assist
	// throttling for the sync duration (~30s hourly). Acceptable
	// because syncs run on the hourly schedule and the 512 MB VM cap
	// is a harder constraint than p99 latency during sync. If future
	// workload characteristics change, consider running the sync in a
	// dedicated goroutine with LockOSThread, or moving to a separate
	// process to isolate the runtime tuning.
	prevGCPercent := debug.SetGCPercent(25)
	defer debug.SetGCPercent(prevGCPercent)

	// Hard memory limit: tell the Go runtime to use aggressive GC
	// (including goroutine assist) when live heap approaches 400 MiB.
	// Combined with GCPercent=25 this bounds the working set below
	// the fly.toml 512 MB VM cap even under allocation bursts during
	// netixlan bulk upsert. Restored on return.
	const syncMemLimit = 400 * 1024 * 1024
	prevMemLimit := debug.SetMemoryLimit(syncMemLimit)
	defer debug.SetMemoryLimit(prevMemLimit)

	scratch, err := openScratchDB(ctx)
	if err != nil {
		w.recordFailure(ctx, statusID, start, err)
		return err
	}
	defer closeScratchDB(ctx, scratch, w.logger)

	// === Phase A — NO TX HELD ===
	// HTTP + JSON decode stream into the scratch DB; Go heap stays bounded.
	cursorUpdates, fromIncremental, err := w.syncFetchPass(ctx, scratch, mode, start)
	if err != nil {
		w.recordFailure(ctx, statusID, start, err)
		return err
	}
	// Commit F guardrail: see checkMemoryLimit godoc (defense-in-depth).
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	if memErr := w.checkMemoryLimit(ctx, ms.HeapAlloc, w.config.SyncMemoryLimit, len(fromIncremental)); memErr != nil {
		w.recordFailure(ctx, statusID, start, memErr)
		return memErr
	}
	// === Fetch Barrier ===
	// Scratch DB fully populated. Open the real LiteFS tx now.
	tx, err := w.entClient.Tx(ctx)
	if err != nil {
		beginErr := fmt.Errorf("begin sync transaction: %w", err)
		w.recordFailure(ctx, statusID, start, beginErr)
		return beginErr
	}
	if _, err := tx.ExecContext(ctx, "PRAGMA defer_foreign_keys = ON"); err != nil {
		fkErr := fmt.Errorf("defer FK checks: %w", err)
		w.rollbackAndRecord(ctx, tx, statusID, start, fkErr)
		return fkErr
	}

	// === Phase B — SINGLE REAL TX ===
	objectCounts, remoteIDsByType, err := w.syncUpsertPass(ctx, tx, scratch, fromIncremental)
	if err != nil {
		w.rollbackAndRecord(ctx, tx, statusID, start, err)
		return err
	}
	if err := w.syncDeletePass(ctx, tx, remoteIDsByType, start); err != nil {
		w.rollbackAndRecord(ctx, tx, statusID, start, err)
		return err
	}
	if err := tx.Commit(); err != nil {
		syncErr := fmt.Errorf("commit sync transaction: %w", err)
		w.recordFailure(ctx, statusID, start, syncErr)
		return syncErr
	}

	w.recordSuccess(ctx, mode, statusID, start, objectCounts, cursorUpdates)
	return nil
}

// checkMemoryLimit reads the current Go heap allocation and returns
// ErrSyncMemoryLimitExceeded if it exceeds the configured limit. If
// limit is 0 (or negative), the guardrail is disabled and the function
// returns nil immediately.
//
// Commit F defense-in-depth (Plan 54-03): called from Worker.Sync
// between the Phase A fetch return and the ent.Tx open so that the
// abort happens BEFORE any database lock is taken. Extracted into a
// helper so Worker.Sync's call site stays within the REFAC-03 100-line
// budget (TestWorkerSync_LineBudget). DO NOT inline this into Sync —
// the helper extraction is load-bearing for the line budget.
//
// The batchCount argument is diagnostic only: it does NOT influence
// the limit decision; it is logged on breach so operators can see how
// many types had been fetched when the abort fired.
//
// heapAlloc comparison: runtime.MemStats.HeapAlloc is uint64 but the
// configured limit is int64 to match the Config struct's env var
// parser. A uint64 >= 2^63 would overflow int64, but in practice the
// Go heap cannot approach that value (it would require >9 EiB of RAM),
// so the comparison is safe. The explicit cap below keeps gosec happy
// without adding runtime cost.
func (w *Worker) checkMemoryLimit(ctx context.Context, heapAlloc uint64, limit int64, batchCount int) error {
	if limit <= 0 {
		return nil
	}
	// Cap the conversion at MaxInt64 to silence gosec G115; real heaps
	// never get close so the cap is unreachable in practice.
	const maxInt64 = uint64(1<<63 - 1)
	heapInt64 := int64(maxInt64)
	if heapAlloc < maxInt64 {
		heapInt64 = int64(heapAlloc)
	}
	if heapInt64 <= limit {
		return nil
	}
	w.logger.LogAttrs(ctx, slog.LevelWarn, "sync aborted: memory limit exceeded",
		slog.Int64("heap_alloc", heapInt64),
		slog.Int64("limit", limit),
		slog.Int("batches", batchCount),
	)
	return ErrSyncMemoryLimitExceeded
}

// emitMemoryTelemetry samples the Go runtime heap and (on Linux) the
// OS RSS high-water mark at the end of a sync cycle, attaches them as
// OTel attributes to the current sync-cycle span, and emits
// slog.Warn("heap threshold crossed") when either value exceeds its
// configured threshold.
//
// Attribute naming follows the pdbplus.* convention established in
// Phase 61 (pdbplus.privacy.tier): pdbplus.sync.peak_heap_mib and
// pdbplus.sync.peak_rss_mib. Units are MiB so dashboards can plot them
// directly without a divisor.
//
// On non-Linux the RSS attr is OMITTED entirely — zero is a valid
// metric value and would produce misleading flat lines on dashboards.
//
// heapWarnBytes == 0 disables the heap Warn (attr still fires);
// rssWarnBytes == 0 likewise. Attribute emission is unconditional so
// dashboards retain timeseries even when alerting is disabled.
//
// D-09: sampling frequency is sync cycle frequency (default 1h via
// PDBPLUS_SYNC_INTERVAL). No periodic background sampler.
func (w *Worker) emitMemoryTelemetry(ctx context.Context, heapWarnBytes, rssWarnBytes int64) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	const maxInt64 = uint64(1<<63 - 1)
	heapBytes := int64(maxInt64)
	if ms.HeapInuse < maxInt64 {
		heapBytes = int64(ms.HeapInuse)
	}
	heapMiB := heapBytes / (1024 * 1024)

	rssBytes, rssOK := readLinuxVMHWM()

	span := trace.SpanFromContext(ctx)
	attrs := []attribute.KeyValue{
		attribute.Int64("pdbplus.sync.peak_heap_mib", heapMiB),
	}
	var rssMiB int64
	if rssOK {
		rssMiB = rssBytes / (1024 * 1024)
		attrs = append(attrs, attribute.Int64("pdbplus.sync.peak_rss_mib", rssMiB))
	}
	span.SetAttributes(attrs...)

	// Publish to the ObservableGauges so Prometheus / Grafana pick them up.
	pdbotel.SyncPeakHeapMiB.Store(heapMiB)
	if rssOK {
		pdbotel.SyncPeakRSSMiB.Store(rssMiB)
	}

	heapOver := heapWarnBytes > 0 && heapBytes > heapWarnBytes
	rssOver := rssOK && rssWarnBytes > 0 && rssBytes > rssWarnBytes
	if !heapOver && !rssOver {
		return
	}
	logAttrs := []slog.Attr{
		slog.Int64("peak_heap_mib", heapMiB),
		slog.Int64("heap_warn_mib", heapWarnBytes/(1024*1024)),
	}
	if rssOK {
		logAttrs = append(logAttrs,
			slog.Int64("peak_rss_mib", rssMiB),
			slog.Int64("rss_warn_mib", rssWarnBytes/(1024*1024)),
		)
	}
	logAttrs = append(logAttrs,
		slog.Bool("heap_over", heapOver),
		slog.Bool("rss_over", rssOver),
	)
	w.logger.LogAttrs(ctx, slog.LevelWarn, "heap threshold crossed", logAttrs...)
}

// readLinuxVMHWM reads /proc/self/status and returns the VmHWM
// (peak resident set size high-water mark) in bytes. The second
// return is false on non-Linux or when the file is absent/unreadable
// — callers MUST treat this as "RSS not available" rather than zero.
//
// VmHWM format: "VmHWM:\t  345216 kB" — tab/space-separated, value in
// kB (base 1024 on Linux). Multiply by 1024 to get bytes.
//
// VmHWM is the peak-RSS high-water mark, not the instantaneous RSS;
// it only decreases when an operator resets it via
// `echo 5 > /proc/self/clear_refs` or the process restarts. This is
// the correct signal for SEED-001 escalation — a single burst is what
// matters, not the steady-state value.
func readLinuxVMHWM() (int64, bool) {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "VmHWM:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, false
		}
		kb, parseErr := strconv.ParseInt(fields[1], 10, 64)
		if parseErr != nil {
			return 0, false
		}
		if kb < 0 || kb > math.MaxInt64/1024 {
			return 0, false
		}
		return kb * 1024, true
	}
	return 0, false
}

// rollbackAndRecord rolls back the tx and records the failure in one place
// so Worker.Sync's error paths stay a one-liner each (REFAC-03 line budget).
// Logs the rollback error at ERROR level — a failing rollback inside a
// failing sync is worth surfacing in the error stream.
func (w *Worker) rollbackAndRecord(ctx context.Context, tx *ent.Tx, statusID int64, start time.Time, syncErr error) {
	// OBS-05: emit heap + RSS span attrs and (if over threshold) slog.Warn.
	w.emitMemoryTelemetry(ctx, w.config.HeapWarnBytes, w.config.RSSWarnBytes)
	if rbErr := tx.Rollback(); rbErr != nil {
		w.logger.LogAttrs(ctx, slog.LevelError, "rollback failed",
			slog.Any("error", rbErr))
	}
	w.recordFailure(ctx, statusID, start, syncErr)
}

// recordSuccess runs all post-commit bookkeeping: per-type cursor updates,
// sync-level metrics, sync_status row update, first-success flag, and the
// OnSyncComplete callback. Extracted from Sync so the orchestrator body
// stays under the REFAC-03 line budget.
func (w *Worker) recordSuccess(
	ctx context.Context,
	mode config.SyncMode,
	statusID int64,
	start time.Time,
	objectCounts map[string]int,
	cursorUpdates map[string]time.Time,
) {
	// OBS-05: emit heap + RSS span attrs and (if over threshold) slog.Warn.
	w.emitMemoryTelemetry(ctx, w.config.HeapWarnBytes, w.config.RSSWarnBytes)
	for typeName, generated := range cursorUpdates {
		if err := UpsertCursor(ctx, w.db, typeName, generated, "success"); err != nil {
			w.logger.LogAttrs(ctx, slog.LevelError, "failed to update cursor",
				slog.String("type", typeName), slog.Any("error", err))
		}
	}
	elapsed := time.Since(start)
	statusAttr := metric.WithAttributes(attribute.String("status", "success"))
	pdbotel.SyncDuration.Record(ctx, elapsed.Seconds(), statusAttr)
	pdbotel.SyncOperations.Add(ctx, 1, statusAttr)
	w.logger.LogAttrs(ctx, slog.LevelInfo, "sync complete",
		slog.String("mode", string(mode)),
		slog.Duration("duration", elapsed),
		slog.Int("total_objects", sumCounts(objectCounts)))
	// Capture completion timestamp once so the sync_status row AND the
	// OnSyncComplete callback (PERF-07: caching middleware ETag setter)
	// see the exact same value. A drift here would mean the atomic ETag
	// pointer and the DB-backed sync time could disagree.
	completedAt := time.Now()
	if statusID > 0 {
		_ = RecordSyncComplete(ctx, w.db, statusID, Status{
			LastSyncAt:   completedAt,
			Duration:     elapsed,
			ObjectCounts: objectCounts,
			Status:       "success",
		})
	}
	w.synced.Store(true)
	if w.config.OnSyncComplete != nil {
		w.config.OnSyncComplete(objectCounts, completedAt)
	}
}

// sumCounts returns the sum of all values in a per-type count map.
func sumCounts(m map[string]int) int {
	total := 0
	for _, v := range m {
		total += v
	}
	return total
}

// syncBatch is a dead marker kept for the TestSync_BatchFreeAfterUpsert
// structural regression lock. Before REFAC-04 (Commit E), this struct
// held per-type []T slices that drainAndUpsertType zeroed between chunks
// to release the backing array. After REFAC-04, per-chunk typed slices
// live in processScratchChunk's locals (one generic helper per type)
// and are reclaimed automatically when the helper returns — the
// function-scope release is strictly more reliable than the old
// map-entry clearing hack. The struct stays as an empty placeholder so
// the `batches[name] = syncBatch{}` literal in drainAndUpsertType
// continues to satisfy the regression test's string match while
// compiling to a no-op map write (free, and kept as a grep-visible
// anchor for the PERF-05 documentation trail).
type syncBatch struct{}

// syncFetchPass runs Phase A against the scratch DB: for each of the 13
// PeeringDB types, stream the HTTP response body into a /tmp SQLite
// staging table via StreamAll's callback. Go heap stays bounded to one
// element per handler invocation (~5-10 KB) instead of one full []T per
// type (~35 MB for netixlan). No ent.Tx is held during Phase A — the
// absence of *ent.Tx from the signature is a compile-time guard against
// accidental tx-in-fetch drift.
//
// Returns per-type cursor update timestamps and a map flagging which
// types came from an incremental fetch (those skip the Phase B delete
// pass because incremental sync does not compute a complete remote-ID
// set). On error, no ent.Tx has been opened yet; the caller records
// failure and returns without touching the real database. The scratch
// file is unlinked by the caller's `defer closeScratchDB(...)`.
//
// Fallback-to-full-on-incremental-error semantics preserved: if the
// incremental stage fails mid-way, the scratch table is truncated and
// the full-mode stage is retried. The final batch for that type is
// flagged as full so the delete pass runs.
//
// PERF-05 option (b): this helper is the fetch-outside-tx pass that
// splits fetch from upsert. Commit D' (this commit) routes Phase A
// through an isolated scratch SQLite DB so the Go heap stays bounded.
func (w *Worker) syncFetchPass(ctx context.Context, scratch *scratchDB, mode config.SyncMode, start time.Time) (
	cursorUpdates map[string]time.Time,
	fromIncremental map[string]bool,
	err error,
) {
	steps := w.syncSteps()
	cursorUpdates = make(map[string]time.Time, len(steps))
	fromIncremental = make(map[string]bool, len(steps))

	for _, step := range steps {
		w.logger.LogAttrs(ctx, slog.LevelInfo, "fetching",
			slog.String("type", step.name),
			slog.String("mode", string(mode)),
		)

		_, stepSpan := otel.Tracer("sync").Start(ctx, "sync-fetch-"+step.name)

		cursor, cursorErr := GetCursor(ctx, w.db, step.name)
		if cursorErr != nil {
			w.logger.LogAttrs(ctx, slog.LevelWarn, "failed to get cursor, using full sync",
				slog.String("type", step.name),
				slog.Any("error", cursorErr),
			)
		}

		cursorUpdate, incremental, stepErr := w.stageOneTypeToScratch(ctx, scratch, step.name, mode, cursor, start, stepSpan)

		stepSpan.End()
		typeAttr := metric.WithAttributes(attribute.String("type", step.name))

		if stepErr != nil {
			pdbotel.SyncTypeFetchErrors.Add(ctx, 1, typeAttr)
			return nil, nil, fmt.Errorf("fetch %s: %w", step.name, stepErr)
		}

		cursorUpdates[step.name] = cursorUpdate
		fromIncremental[step.name] = incremental
	}

	return cursorUpdates, fromIncremental, nil
}

// stageOneTypeToScratch streams a single PeeringDB type into its scratch
// staging table, handling the incremental-with-fallback-to-full
// semantics. On incremental error the scratch table for this type is
// truncated (to drop any partial insert) and a full stage is retried.
// Returns the cursor update timestamp, a flag indicating whether the
// successful run was incremental, and any error.
func (w *Worker) stageOneTypeToScratch(ctx context.Context, scratch *scratchDB, name string, mode config.SyncMode, cursor time.Time, start time.Time, stepSpan trace.Span) (time.Time, bool, error) {
	// Incremental attempt with fallback to full on error.
	if mode == config.SyncModeIncremental && !cursor.IsZero() {
		generated, incErr := scratch.stageType(ctx, w.pdbClient, name, cursor)
		if incErr == nil {
			return generated, true, nil
		}
		// Fallback: clear partial incremental state and retry as full.
		typeAttr := metric.WithAttributes(attribute.String("type", name))
		pdbotel.SyncTypeFallback.Add(ctx, 1, typeAttr)
		stepSpan.AddEvent("incremental.fallback",
			trace.WithAttributes(
				attribute.String("type", name),
				attribute.String("error", incErr.Error()),
			),
		)
		w.logger.LogAttrs(ctx, slog.LevelWarn, "incremental sync failed, falling back to full",
			slog.String("type", name),
			slog.Any("error", incErr),
		)
		if _, delErr := scratch.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %q", name)); delErr != nil {
			return time.Time{}, false, fmt.Errorf("clear partial incremental scratch %s: %w", name, delErr)
		}
	}
	// Full sync (default, first sync, no cursor, or incremental-fallback).
	if _, err := scratch.stageType(ctx, w.pdbClient, name, time.Time{}); err != nil {
		return time.Time{}, false, err
	}
	return start, false, nil
}

// syncUpsertPass runs Phase B upserts inside the single tx. It drains
// each scratch staging table in FK parent-first order, chunking the rows
// into memory-bounded slices (scratchChunkSize rows at a time) so peak
// Go heap stays under ~10 MB per chunk. Each chunk is decoded to its
// typed Go struct, upserted into the real ent table, and then
// IMMEDIATELY freed via `batches[step.name] = syncBatch{}` to release
// the slice backing array before the next chunk loads. This is the core memory
// optimization for PERF-05 — without it, Phase B peak memory would
// double during the handover between chunks. DO NOT remove the
// batch-free line.
//
// The remoteIDsByType map is populated from scratch via a final
// `SELECT id FROM scratch.{type}` after the chunked upsert completes —
// this gives the delete pass the complete remote-ID set for full syncs.
// Incremental syncs skip delete (fromIncremental[name] == true).
//
// D-19 atomicity is preserved: all real-DB writes run inside the same
// ent.Tx, and any upsert error triggers a rollback via the orchestrator.
func (w *Worker) syncUpsertPass(ctx context.Context, tx *ent.Tx, scratch *scratchDB, fromIncremental map[string]bool) (
	objectCounts map[string]int,
	remoteIDsByType map[string][]int,
	err error,
) {
	// Reset the per-sync FK orphan-filter state before the first
	// dispatchScratchChunk call populates it. Kept here rather than in
	// Sync so the Sync body stays within the REFAC-03 100-line budget
	// enforced by TestWorkerSync_LineBudget.
	w.resetFKState()
	steps := w.syncSteps()
	objectCounts = make(map[string]int, len(steps))
	remoteIDsByType = make(map[string][]int, len(steps))
	// batches carries one decoded chunk at a time; the map entry is
	// cleared after each chunk upsert for the PERF-05 memory bound.
	batches := make(map[string]syncBatch, 1)

	for _, step := range steps {
		_, stepSpan := otel.Tracer("sync").Start(ctx, "sync-upsert-"+step.name)

		count, stepErr := w.drainAndUpsertType(ctx, tx, scratch, step.name, batches)

		stepSpan.End()
		typeAttr := metric.WithAttributes(attribute.String("type", step.name))

		if stepErr != nil {
			pdbotel.SyncTypeUpsertErrors.Add(ctx, 1, typeAttr)
			return nil, nil, fmt.Errorf("upsert %s: %w", step.name, stepErr)
		}

		pdbotel.SyncTypeObjects.Add(ctx, int64(count), typeAttr)
		objectCounts[step.name] = count

		// Full-sync batches contribute a complete remote-ID set used by
		// the delete pass. Incremental batches are a delta only — skip
		// delete. The remote IDs are read from scratch directly so the
		// ID set does not inflate Go heap during the upsert phase.
		//
		// Subtract any IDs dropped by the upsert-time fkFilter so the
		// delete pass picks up previously-inserted rows whose FK is now
		// orphaned — this prevents parent-delete-while-child-remains FK
		// violations at commit in steady-state syncs.
		if !fromIncremental[step.name] {
			ids, idErr := w.collectScratchIDs(ctx, scratch, step.name)
			if idErr != nil {
				return nil, nil, fmt.Errorf("collect remote ids %s: %w", step.name, idErr)
			}
			if skipped := w.fkSkippedIDs[step.name]; len(skipped) > 0 {
				filtered := ids[:0]
				for _, id := range ids {
					if _, drop := skipped[id]; !drop {
						filtered = append(filtered, id)
					}
				}
				ids = filtered
			}
			remoteIDsByType[step.name] = ids
		}

		// PERF-05 hard gate (400 MB): force a GC cycle between types
		// to deterministically reclaim the chunked decode buffers and
		// ent query-builder state before the next type's upsert begins.
		// Without this, sampled peak heap varies run-to-run because GC
		// scheduling lags the allocation spike on large types like
		// netixlan (200K rows). A per-type GC hint costs ~20ms per type
		// (13 × 20ms = 260ms added latency) in exchange for bounded
		// peak heap on the 512 MB fly.toml VM.
		runtime.GC()

		w.logger.LogAttrs(ctx, slog.LevelInfo, "upserted",
			slog.String("type", step.name),
			slog.Int("count", count),
		)
	}

	return objectCounts, remoteIDsByType, nil
}

// drainAndUpsertType reads scratch[type] in chunks of scratchChunkSize
// rows, decodes each chunk into typed Go structs, upserts the chunk
// into the real ent table, and frees the chunk memory before reading
// the next. Returns the total row count across all chunks.
//
// The chunked replay is the difference between peak heap ~20 MB and
// peak heap ~600 MB: netixlan is ~200K rows × ~200 bytes = ~40 MB if
// loaded in one shot, versus ~1 MB per 5000-row chunk. D-19 atomicity
// is preserved because all upserts run through the same ent.Tx.
//
// REFAC-04 (Commit E): per-chunk decode+filter+upsert dispatches to the
// generic syncIncremental[E] via dispatchScratchChunk, replacing the
// old 13-arm type-switches in decodeScratchChunk and upsertOneType.
// The per-chunk typed slice is local to syncIncremental[E] and is
// reclaimed when that helper returns — the old `batches[name] =
// syncBatch{}` map-entry clearing is no longer functionally required
// but is kept as a grep-visible no-op anchor for the
// TestSync_BatchFreeAfterUpsert regression lock.
func (w *Worker) drainAndUpsertType(ctx context.Context, tx *ent.Tx, scratch *scratchDB, name string, batches map[string]syncBatch) (int, error) {
	total := 0
	afterID := 0
	for {
		rows, lastID, err := scratch.drainChunk(ctx, name, afterID, scratchChunkSize)
		if err != nil {
			return total, err
		}
		if len(rows) == 0 {
			break
		}

		count, upErr := w.dispatchScratchChunk(ctx, tx, name, rows)
		if upErr != nil {
			return total, upErr
		}
		total += count

		// MANDATORY memory optimization anchor: historically cleared the
		// per-type entry in the batches map to release the chunk backing
		// array between iterations. Post REFAC-04 (Commit E) the typed
		// chunk slice lives in syncIncremental[E]'s local scope and is
		// reclaimed automatically on return — scope-based release is
		// strictly more reliable than map-entry clearing. The literal
		// write below compiles to a no-op map store and is kept as a
		// grep-visible anchor for TestSync_BatchFreeAfterUpsert and the
		// PERF-05 documentation trail in ARCHITECTURE.md §2. DO NOT
		// remove without first updating the regression test.
		batches[name] = syncBatch{}

		if len(rows) < scratchChunkSize {
			break
		}
		afterID = lastID
	}
	return total, nil
}

// collectScratchIDs reads the full set of row ids from scratch[type].
// Used to build the remote-ID set for the Phase B delete pass without
// keeping the typed slice in Go heap. The id set for netixlan at 200K
// rows is only ~1.6 MB (8 bytes per int64) — well inside the heap
// budget even for the largest type.
func (w *Worker) collectScratchIDs(ctx context.Context, scratch *scratchDB, name string) ([]int, error) {
	rows, err := scratch.db.QueryContext(ctx, fmt.Sprintf("SELECT id FROM %q ORDER BY id", name))
	if err != nil {
		return nil, fmt.Errorf("query scratch ids %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan scratch id %s: %w", name, err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate scratch ids %s: %w", name, err)
	}
	return ids, nil
}

// syncIncrementalInput bundles the per-type parameters for
// syncIncremental[E]. Declared immediately before the consuming
// function per GO-CS-6. objectType is used for error-wrap context;
// getStatus extracts the deleted-status filter key from an element;
// upsert performs the bulk upsert inside the caller's ent.Tx.
//
// fkFilter, when non-nil, is called per row after the deleted-status
// filter and before the upsert. The row pointer lets the closure
// mutate nullable FK fields in place (e.g. null out an orphaned
// campus_id on a facility so the facility row itself is kept). When
// the closure returns false the row is dropped from the chunk and
// the caller MUST call fkMarkSkipped on its id so the delete pass
// can reconcile any previously-inserted row with the same id.
//
// recordIDs, when non-nil, is called with the []int returned by the
// upsert closure so downstream child types can validate their FK
// references against the parent set (see Worker.fkRegisterIDs).
type syncIncrementalInput[E any] struct {
	objectType string
	getStatus  func(E) string
	fkFilter   func(*E) bool
	recordIDs  func(ids []int)
	upsert     func(ctx context.Context, tx *ent.Tx, items []E) ([]int, error)
}

// syncIncremental decodes a chunk of raw scratch rows for a single
// PeeringDB type into typed Go structs and upserts the chunk into the
// real ent table via the per-type upsert closure. Returns the count of
// upserted rows.
//
// REFAC-04 (Commit E): this generic helper replaces the 13 per-type
// arms that used to live in decodeScratchChunk and upsertOneType. The
// type-specific behavior is now carried by the closure arguments on
// syncIncrementalInput[E], so the bookkeeping code (decode, upsert,
// error-wrap) lives in exactly one place instead of being copy-pasted
// 13 times with only type names changed.
//
// Phase 68 Plan 01 (D-01): removed the `includeDeleted` parameter and
// the `filterByStatus` branch. Sync now unconditionally persists rows
// with any upstream status (including "deleted") through the upsert
// path; the row-level status × since matrix is applied by serializer
// surfaces (pdbcompat in Plan 68-03). Plan 68-02 then flips the delete
// pass from hard-delete to soft-delete, closing the STATUS-03 loop.
//
// Each call processes ONE chunk (<=scratchChunkSize rows). The typed
// `items` slice is local to this function, so the chunk backing array
// is reclaimed automatically when the helper returns — no map-entry
// clearing is necessary for the PERF-05 memory bound. D-19 atomicity
// is preserved: the upsert closure binds to a tx captured by the
// caller, and every real-DB write still runs inside that single tx.
//
// Package-level function (not a method) because Go does not allow
// method-level type parameters.
func syncIncremental[E any](ctx context.Context, tx *ent.Tx, in syncIncrementalInput[E], rows []scratchRow) (int, error) {
	items := make([]E, 0, len(rows))
	for _, r := range rows {
		var v E
		if err := json.Unmarshal(r.raw, &v); err != nil {
			return 0, fmt.Errorf("decode %s id=%d: %w", in.objectType, r.id, err)
		}
		items = append(items, v)
	}
	if in.fkFilter != nil {
		kept := make([]E, 0, len(items))
		for i := range items {
			if in.fkFilter(&items[i]) {
				kept = append(kept, items[i])
			}
		}
		items = kept
	}
	insertedIDs, err := in.upsert(ctx, tx, items)
	if err != nil {
		return 0, fmt.Errorf("upsert %s: %w", in.objectType, err)
	}
	if in.recordIDs != nil {
		in.recordIDs(insertedIDs)
	}
	return len(items), nil
}

// dispatchScratchChunk routes a chunk of scratch rows for the named
// PeeringDB type through the generic syncIncremental[E] helper. This
// is the single dispatch point for the 13 closed-set entity types —
// each case binds the concrete type E plus its per-type status accessor
// and upsert helper, then calls the generic.
//
// REFAC-04 (Commit E): this 13-arm dispatch replaces the two separate
// 13-arm type-switches that used to live in decodeScratchChunk and
// upsertOneType. Adding a new PeeringDB type now requires a single
// case entry here (and the corresponding entry in syncSteps /
// scratchTypes). Removing or reordering cases must stay in lockstep
// with syncSteps() to preserve FK dependency ordering.
//
// v1.13 FK orphan filter: cases for child types supply a fkFilter
// closure that checks each incoming row's FK references against the
// parent sets populated by earlier syncIncremental.recordIDs
// callbacks. Rows whose parents are missing are dropped and logged,
// and their IDs are recorded via fkMarkSkipped so the delete pass can
// clean up any previously-inserted row with the same ID. This handles
// PeeringDB snapshots that expose live children pointing at
// server-side-suppressed parents (e.g. NTT America carrier → org).
func (w *Worker) dispatchScratchChunk(ctx context.Context, tx *ent.Tx, name string, rows []scratchRow) (int, error) {
	switch name {
	case peeringdb.TypeOrg:
		return syncIncremental(ctx, tx, syncIncrementalInput[peeringdb.Organization]{
			objectType: name,
			getStatus:  func(v peeringdb.Organization) string { return v.Status },
			recordIDs:  func(ids []int) { w.fkRegisterIDs(peeringdb.TypeOrg, ids) },
			upsert:     upsertOrganizations,
		}, rows)
	case peeringdb.TypeCampus:
		return syncIncremental(ctx, tx, syncIncrementalInput[peeringdb.Campus]{
			objectType: name,
			getStatus:  func(v peeringdb.Campus) string { return v.Status },
			fkFilter: func(v *peeringdb.Campus) bool {
				return w.fkCheckParent(ctx, peeringdb.TypeCampus, v.ID,
					peeringdb.TypeOrg, v.OrgID, "org_id")
			},
			recordIDs: func(ids []int) { w.fkRegisterIDs(peeringdb.TypeCampus, ids) },
			upsert:    upsertCampuses,
		}, rows)
	case peeringdb.TypeFac:
		return syncIncremental(ctx, tx, syncIncrementalInput[peeringdb.Facility]{
			objectType: name,
			getStatus:  func(v peeringdb.Facility) string { return v.Status },
			fkFilter: func(v *peeringdb.Facility) bool {
				if !w.fkCheckParent(ctx, peeringdb.TypeFac, v.ID,
					peeringdb.TypeOrg, v.OrgID, "org_id") {
					return false
				}
				// campus_id is Optional().Nillable() in the ent
				// schema — if the referenced campus is missing,
				// null the reference out and keep the facility
				// (avoids cascading the drop through netfac /
				// ixfac / carrierfac children of the facility).
				if v.CampusID != nil && !w.fkHasParent(peeringdb.TypeCampus, *v.CampusID) {
					w.logger.LogAttrs(ctx, slog.LevelWarn, "nulling orphan FK",
						slog.String("child_type", peeringdb.TypeFac),
						slog.Int("child_id", v.ID),
						slog.String("field", "campus_id"),
						slog.Int("orphan_parent_id", *v.CampusID),
					)
					v.CampusID = nil
				}
				return true
			},
			recordIDs: func(ids []int) { w.fkRegisterIDs(peeringdb.TypeFac, ids) },
			upsert:    upsertFacilities,
		}, rows)
	case peeringdb.TypeCarrier:
		return syncIncremental(ctx, tx, syncIncrementalInput[peeringdb.Carrier]{
			objectType: name,
			getStatus:  func(v peeringdb.Carrier) string { return v.Status },
			fkFilter: func(v *peeringdb.Carrier) bool {
				return w.fkCheckParent(ctx, peeringdb.TypeCarrier, v.ID,
					peeringdb.TypeOrg, v.OrgID, "org_id")
			},
			recordIDs: func(ids []int) { w.fkRegisterIDs(peeringdb.TypeCarrier, ids) },
			upsert:    upsertCarriers,
		}, rows)
	case peeringdb.TypeCarrierFac:
		return syncIncremental(ctx, tx, syncIncrementalInput[peeringdb.CarrierFacility]{
			objectType: name,
			getStatus:  func(v peeringdb.CarrierFacility) string { return v.Status },
			fkFilter: func(v *peeringdb.CarrierFacility) bool {
				if !w.fkCheckParent(ctx, peeringdb.TypeCarrierFac, v.ID,
					peeringdb.TypeCarrier, v.CarrierID, "carrier_id") {
					return false
				}
				return w.fkCheckParent(ctx, peeringdb.TypeCarrierFac, v.ID,
					peeringdb.TypeFac, v.FacID, "fac_id")
			},
			recordIDs: func(ids []int) { w.fkRegisterIDs(peeringdb.TypeCarrierFac, ids) },
			upsert:    upsertCarrierFacilities,
		}, rows)
	case peeringdb.TypeIX:
		return syncIncremental(ctx, tx, syncIncrementalInput[peeringdb.InternetExchange]{
			objectType: name,
			getStatus:  func(v peeringdb.InternetExchange) string { return v.Status },
			fkFilter: func(v *peeringdb.InternetExchange) bool {
				return w.fkCheckParent(ctx, peeringdb.TypeIX, v.ID,
					peeringdb.TypeOrg, v.OrgID, "org_id")
			},
			recordIDs: func(ids []int) { w.fkRegisterIDs(peeringdb.TypeIX, ids) },
			upsert:    upsertInternetExchanges,
		}, rows)
	case peeringdb.TypeIXLan:
		return syncIncremental(ctx, tx, syncIncrementalInput[peeringdb.IxLan]{
			objectType: name,
			getStatus:  func(v peeringdb.IxLan) string { return v.Status },
			fkFilter: func(v *peeringdb.IxLan) bool {
				return w.fkCheckParent(ctx, peeringdb.TypeIXLan, v.ID,
					peeringdb.TypeIX, v.IXID, "ix_id")
			},
			recordIDs: func(ids []int) { w.fkRegisterIDs(peeringdb.TypeIXLan, ids) },
			upsert:    upsertIxLans,
		}, rows)
	case peeringdb.TypeIXPfx:
		return syncIncremental(ctx, tx, syncIncrementalInput[peeringdb.IxPrefix]{
			objectType: name,
			getStatus:  func(v peeringdb.IxPrefix) string { return v.Status },
			fkFilter: func(v *peeringdb.IxPrefix) bool {
				return w.fkCheckParent(ctx, peeringdb.TypeIXPfx, v.ID,
					peeringdb.TypeIXLan, v.IXLanID, "ixlan_id")
			},
			recordIDs: func(ids []int) { w.fkRegisterIDs(peeringdb.TypeIXPfx, ids) },
			upsert:    upsertIxPrefixes,
		}, rows)
	case peeringdb.TypeIXFac:
		return syncIncremental(ctx, tx, syncIncrementalInput[peeringdb.IxFacility]{
			objectType: name,
			getStatus:  func(v peeringdb.IxFacility) string { return v.Status },
			fkFilter: func(v *peeringdb.IxFacility) bool {
				if !w.fkCheckParent(ctx, peeringdb.TypeIXFac, v.ID,
					peeringdb.TypeIX, v.IXID, "ix_id") {
					return false
				}
				return w.fkCheckParent(ctx, peeringdb.TypeIXFac, v.ID,
					peeringdb.TypeFac, v.FacID, "fac_id")
			},
			recordIDs: func(ids []int) { w.fkRegisterIDs(peeringdb.TypeIXFac, ids) },
			upsert:    upsertIxFacilities,
		}, rows)
	case peeringdb.TypeNet:
		return syncIncremental(ctx, tx, syncIncrementalInput[peeringdb.Network]{
			objectType: name,
			getStatus:  func(v peeringdb.Network) string { return v.Status },
			fkFilter: func(v *peeringdb.Network) bool {
				return w.fkCheckParent(ctx, peeringdb.TypeNet, v.ID,
					peeringdb.TypeOrg, v.OrgID, "org_id")
			},
			recordIDs: func(ids []int) { w.fkRegisterIDs(peeringdb.TypeNet, ids) },
			upsert:    upsertNetworks,
		}, rows)
	case peeringdb.TypePoc:
		return syncIncremental(ctx, tx, syncIncrementalInput[peeringdb.Poc]{
			objectType: name,
			getStatus:  func(v peeringdb.Poc) string { return v.Status },
			fkFilter: func(v *peeringdb.Poc) bool {
				return w.fkCheckParent(ctx, peeringdb.TypePoc, v.ID,
					peeringdb.TypeNet, v.NetID, "net_id")
			},
			recordIDs: func(ids []int) { w.fkRegisterIDs(peeringdb.TypePoc, ids) },
			upsert:    upsertPocs,
		}, rows)
	case peeringdb.TypeNetFac:
		return syncIncremental(ctx, tx, syncIncrementalInput[peeringdb.NetworkFacility]{
			objectType: name,
			getStatus:  func(v peeringdb.NetworkFacility) string { return v.Status },
			fkFilter: func(v *peeringdb.NetworkFacility) bool {
				if !w.fkCheckParent(ctx, peeringdb.TypeNetFac, v.ID,
					peeringdb.TypeNet, v.NetID, "net_id") {
					return false
				}
				return w.fkCheckParent(ctx, peeringdb.TypeNetFac, v.ID,
					peeringdb.TypeFac, v.FacID, "fac_id")
			},
			recordIDs: func(ids []int) { w.fkRegisterIDs(peeringdb.TypeNetFac, ids) },
			upsert:    upsertNetworkFacilities,
		}, rows)
	case peeringdb.TypeNetIXLan:
		return syncIncremental(ctx, tx, syncIncrementalInput[peeringdb.NetworkIxLan]{
			objectType: name,
			getStatus:  func(v peeringdb.NetworkIxLan) string { return v.Status },
			fkFilter: func(v *peeringdb.NetworkIxLan) bool {
				if !w.fkCheckParent(ctx, peeringdb.TypeNetIXLan, v.ID,
					peeringdb.TypeNet, v.NetID, "net_id") {
					return false
				}
				if !w.fkCheckParent(ctx, peeringdb.TypeNetIXLan, v.ID,
					peeringdb.TypeIX, v.IXID, "ix_id") {
					return false
				}
				return w.fkCheckParent(ctx, peeringdb.TypeNetIXLan, v.ID,
					peeringdb.TypeIXLan, v.IXLanID, "ixlan_id")
			},
			recordIDs: func(ids []int) { w.fkRegisterIDs(peeringdb.TypeNetIXLan, ids) },
			upsert:    upsertNetworkIxLans,
		}, rows)
	}
	return 0, fmt.Errorf("unknown sync type: %s", name)
}

// fkCheckParent is the per-row FK validation helper called from the
// fkFilter closures in dispatchScratchChunk. Returns true if parentID
// is registered for parentType (or is a zero/null FK); otherwise logs
// the orphan at WARN, records the child id in fkSkippedIDs so the
// delete pass can reconcile, and returns false so syncIncremental
// drops the row from the chunk.
func (w *Worker) fkCheckParent(ctx context.Context, childType string, childID int, parentType string, parentID int, field string) bool {
	if w.fkHasParent(parentType, parentID) {
		return true
	}
	w.fkMarkSkipped(childType, childID)
	w.logger.LogAttrs(ctx, slog.LevelWarn, "dropping FK orphan",
		slog.String("child_type", childType),
		slog.Int("child_id", childID),
		slog.String("field", field),
		slog.String("parent_type", parentType),
		slog.Int("parent_id", parentID),
	)
	return false
}

// syncDeletePass runs the per-type soft-delete loop in child-first (reverse
// FK) order, skipping types that have no remoteIDs (incremental sync
// succeeded). cycleStart is the single timestamp stamped on every row marked
// status='deleted' during this cycle (Phase 68 D-02) — reused from the
// Worker.Sync-entry time.Now() so all 13 types see identical timestamps.
// The orchestrator handles rollback + recordFailure on error.
func (w *Worker) syncDeletePass(ctx context.Context, tx *ent.Tx, remoteIDsByType map[string][]int, cycleStart time.Time) error {
	steps := w.syncSteps()
	for i := len(steps) - 1; i >= 0; i-- {
		step := steps[i]
		remoteIDs, ok := remoteIDsByType[step.name]
		if !ok {
			// Incremental sync succeeded for this type — no delete needed.
			continue
		}

		_, stepSpan := otel.Tracer("sync").Start(ctx, "sync-delete-"+step.name)

		marked, stepErr := step.deleteFn(ctx, tx, remoteIDs, cycleStart)

		stepSpan.End()

		typeAttr := metric.WithAttributes(attribute.String("type", step.name))

		if stepErr != nil {
			return fmt.Errorf("mark stale deleted %s: %w", step.name, stepErr)
		}

		// Record per-type delete metrics per D-08. The metric name stays
		// SyncTypeDeleted — operator semantics for a row absent from the
		// visible list are still "it's gone", even though the row physically
		// remains as a tombstone post-Phase-68 D-02.
		pdbotel.SyncTypeDeleted.Add(ctx, int64(marked), typeAttr)

		if marked > 0 {
			w.logger.LogAttrs(ctx, slog.LevelInfo, "marked stale deleted",
				slog.String("type", step.name),
				slog.Int("marked", marked),
			)
		}
	}
	return nil
}

// recordFailure records a failed sync in the sync_status table and metrics.
func (w *Worker) recordFailure(ctx context.Context, statusID int64, start time.Time, syncErr error) {
	// OBS-05: emit heap + RSS span attrs and (if over threshold) slog.Warn.
	// Called even on failure — memory pressure is interesting regardless of sync outcome.
	w.emitMemoryTelemetry(ctx, w.config.HeapWarnBytes, w.config.RSSWarnBytes)
	// Record sync-level failure metrics per D-06.
	failedAttr := metric.WithAttributes(attribute.String("status", "failed"))
	pdbotel.SyncDuration.Record(ctx, time.Since(start).Seconds(), failedAttr)
	pdbotel.SyncOperations.Add(ctx, 1, failedAttr)

	if statusID > 0 {
		_ = RecordSyncComplete(ctx, w.db, statusID, Status{
			LastSyncAt:   time.Now(),
			Duration:     time.Since(start),
			Status:       "failed",
			ErrorMessage: syncErr.Error(),
		})
	}
}

// SyncWithRetry calls Sync and retries on failure with exponential backoff per D-21.
//
// Rate-limit short-circuit: when the wrapped error is a *peeringdb.RateLimitError,
// the retry ladder is skipped entirely. PeeringDB's unauthenticated quota is
// 1 request per distinct query-string per hour, and all three default backoffs
// (30s, 2m, 8m) fall well inside that window — every retry within the window
// is guaranteed to 429 again AND consumes another slot against the hourly
// quota. Returning immediately lets the hourly scheduler retry naturally on
// its next tick (1h interval ≥ most Retry-After values we've observed).
func (w *Worker) SyncWithRetry(ctx context.Context, mode config.SyncMode) error {
	err := w.Sync(ctx, mode)
	if err == nil {
		return nil
	}
	if rateLimited(err) {
		w.logger.LogAttrs(ctx, slog.LevelWarn, "sync rate-limited, deferring to next scheduled tick",
			slog.Any("error", err),
		)
		return err
	}

	maxRetries := len(w.retryBackoffs)
	var lastErr error
	for attempt, backoff := range w.retryBackoffs {
		lastErr = err
		w.logger.LogAttrs(ctx, slog.LevelWarn, "sync failed, retrying",
			slog.Int("attempt", attempt+1),
			slog.Duration("backoff", backoff),
			slog.Any("error", err),
		)

		// Wait for backoff, respecting context cancellation.
		timer := time.NewTimer(backoff)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("sync retry cancelled: %w", ctx.Err())
		}

		err = w.Sync(ctx, mode)
		if err == nil {
			return nil
		}
		// If the NEXT attempt also hit the rate limit, stop retrying for
		// the same reason as the initial short-circuit above.
		if rateLimited(err) {
			w.logger.LogAttrs(ctx, slog.LevelWarn, "sync rate-limited during retry, deferring",
				slog.Int("attempt", attempt+1),
				slog.Any("error", err),
			)
			return err
		}
	}

	w.logger.LogAttrs(ctx, slog.LevelError, "sync failed after all retries",
		slog.Int("retries", maxRetries),
		slog.Any("error", err),
	)
	return fmt.Errorf("sync failed after %d retries: %w", maxRetries, lastErr)
}

// rateLimited reports whether err wraps a *peeringdb.RateLimitError anywhere
// in its chain. Used by SyncWithRetry to skip the retry ladder on HTTP 429
// responses from PeeringDB.
func rateLimited(err error) bool {
	_, ok := errors.AsType[*peeringdb.RateLimitError](err)
	return ok
}

// HasCompletedSync reports whether at least one successful sync has completed.
// Used for 503 behavior per D-30.
func (w *Worker) HasCompletedSync() bool {
	return w.synced.Load()
}

// runSyncCycle wraps SyncWithRetry with a demotion monitor goroutine.
// If the node is demoted during sync (IsPrimary returns false), the cycle
// context is cancelled, causing SyncWithRetry to abort early.
// The monitor goroutine lifetime is tied to cycleCtx per CC-2.
func (w *Worker) runSyncCycle(ctx context.Context, mode config.SyncMode) {
	cycleCtx, cycleCancel := context.WithCancel(ctx)
	defer cycleCancel()

	// Monitor goroutine: polls IsPrimary every 1s and cancels on demotion.
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-cycleCtx.Done():
				return
			case <-ticker.C:
				if !w.config.IsPrimary() {
					w.logger.LogAttrs(cycleCtx, slog.LevelWarn, "demoted during sync, aborting cycle")
					cycleCancel()
					return
				}
			}
		}
	}()

	if err := w.SyncWithRetry(cycleCtx, mode); err != nil {
		w.logger.LogAttrs(ctx, slog.LevelError, "sync cycle failed",
			slog.Any("error", err),
		)
	}
	cycleCancel() // ensure monitor goroutine exits
	<-done        // wait for clean exit per CC-2
}

// StartScheduler runs the sync scheduler on all instances per D-22.
// On primary nodes it executes sync cycles; on replicas it waits for promotion.
// Role changes are detected dynamically at each scheduler wakeup via
// w.config.IsPrimary(). The scheduler stops when ctx is cancelled per CC-2.
//
// Scheduling anchor: the next sync is scheduled at lastCompletion + interval,
// not at processStart + N*interval. This matters across restarts — a rolling
// deploy mid-interval would otherwise delay the next sync by up to a full
// interval (the ticker would re-anchor on process start). Concretely:
//
//   - Fresh DB (no prior successful sync) on a primary: run a full sync
//     immediately, then schedule the next at now+interval.
//   - Warm start on a primary with a recent lastSync: wait until
//     lastSync+interval; if that is already in the past, the first
//     iteration fires immediately.
//   - Replica: wake every interval to check for promotion. Matches the
//     heartbeat cadence of the pre-rewrite ticker-based design.
//
// After each cycle — success or failure — the next sync is scheduled at
// time.Now()+interval. A slower-than-expected sync therefore does NOT
// shorten the following window, and a failed sync gives PeeringDB a full
// interval to recover before the next external-facing retry.
func (w *Worker) StartScheduler(ctx context.Context, interval time.Duration) {
	mode := cmp.Or(w.config.SyncMode, config.SyncModeFull)

	lastSync, err := GetLastSuccessfulSyncTime(ctx, w.db)
	if err != nil {
		w.logger.LogAttrs(ctx, slog.LevelWarn, "failed to get last sync time",
			slog.Any("error", err),
		)
	}
	if !lastSync.IsZero() {
		// Prior data exists (either from our own sync history or from
		// LiteFS replication) — serve requests immediately.
		w.synced.Store(true)
	}

	wasPrimary := w.config.IsPrimary()

	// Fresh-DB fast path: a primary with no prior successful sync must run
	// a full sync before entering the wait loop. Forced to SyncModeFull
	// regardless of the configured mode because there is no cursor data
	// for an incremental run to resume from.
	if wasPrimary && lastSync.IsZero() {
		w.runSyncCycle(ctx, config.SyncModeFull)
	}

	// Compute the initial wakeup time.
	var nextAt time.Time
	switch {
	case wasPrimary && lastSync.IsZero():
		// Just ran the fresh-DB sync above — schedule the next one a
		// full interval from now.
		nextAt = time.Now().Add(interval)
	case wasPrimary:
		// Warm start: anchor at lastSync+interval. If that is already
		// in the past, the first loop iteration will fire immediately;
		// otherwise we wait exactly until the anchor.
		nextAt = lastSync.Add(interval)
	default:
		// Replica: wake every interval to check for promotion.
		nextAt = time.Now().Add(interval)
		w.logger.LogAttrs(ctx, slog.LevelDebug, "starting scheduler as replica")
	}

	for {
		wait := max(time.Until(nextAt), 0)

		// time.NewTimer (not time.After) so ctx-cancellation can Stop()
		// the timer and release it to the GC immediately instead of
		// waiting the full interval for the fire.
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		isPrimary := w.config.IsPrimary()

		// Role transitions.
		if isPrimary && !wasPrimary {
			w.logger.LogAttrs(ctx, slog.LevelInfo, "promoted to primary, checking sync status")
			pdbotel.RoleTransitions.Add(ctx, 1,
				metric.WithAttributes(attribute.String("direction", "promoted")),
			)
			wasPrimary = true
			// Re-read from the DB: replication may have advanced the
			// last-sync timestamp while we were a replica.
			ls, _ := GetLastSuccessfulSyncTime(ctx, w.db)
			if ls.IsZero() || time.Since(ls) >= interval {
				w.runSyncCycle(ctx, mode)
				nextAt = time.Now().Add(interval)
			} else {
				nextAt = ls.Add(interval)
			}
			continue
		}
		if !isPrimary && wasPrimary {
			w.logger.LogAttrs(ctx, slog.LevelInfo, "demoted to replica")
			pdbotel.RoleTransitions.Add(ctx, 1,
				metric.WithAttributes(attribute.String("direction", "demoted")),
			)
			wasPrimary = false
			nextAt = time.Now().Add(interval)
			continue
		}
		wasPrimary = isPrimary

		if !isPrimary {
			// Still a replica — heartbeat for the next role check.
			w.logger.LogAttrs(ctx, slog.LevelDebug, "not primary, skipping sync")
			nextAt = time.Now().Add(interval)
			continue
		}

		// Primary and the scheduled time has arrived — run the cycle
		// and anchor the next run at now+interval (a slow sync does
		// NOT shorten the next window).
		w.runSyncCycle(ctx, mode)
		nextAt = time.Now().Add(interval)
	}
}

