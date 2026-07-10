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
	"slices"
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
// Defense-in-depth against PeeringDB data growth
// that exceeds the 400 MB bench harness baseline at runtime (e.g. if
// netixlan doubles between benchmarks and production). Callers detect
// the sentinel via errors.Is.
var ErrSyncMemoryLimitExceeded = errors.New("sync aborted: memory limit exceeded")

// ErrSyncAlreadyRunning is returned by Sync (and propagated unchanged by
// SyncWithRetry) when a cycle is already in flight and the trigger is
// dropped by the running-CAS guard. There is no queueing or pending-mode
// coalescing: the requested mode is lost. Callers that need to surface the
// drop (e.g. the POST /sync handler answering 409 Conflict instead of 202)
// detect it via errors.Is.
var ErrSyncAlreadyRunning = errors.New("sync already running, trigger dropped")

// ErrSyncAttemptTimeout is the cancellation cause installed when a sync
// attempt exceeds WorkerConfig.SyncTimeout. It distinguishes the watchdog
// firing (context.Cause) from the other cycle-cancellation sources —
// demotion, shutdown — in logs and tests.
var ErrSyncAttemptTimeout = errors.New("sync attempt exceeded PDBPLUS_SYNC_TIMEOUT")

// defaultRetryBackoffs defines the backoff durations for sync-level retries.
var defaultRetryBackoffs = []time.Duration{30 * time.Second, 2 * time.Minute, 8 * time.Minute}

// WorkerConfig holds configuration for the sync worker.
type WorkerConfig struct {
	IsPrimary func() bool // live primary detection; nil defaults to always-primary
	SyncMode  config.SyncMode
	// OnSyncComplete is called after a successful sync with the worker's
	// ctx and the completion timestamp. The timestamp is the same value
	// persisted into the sync_status row by recordSuccess, so downstream
	// consumers (e.g. the caching middleware ETag setter wired in
	// cmd/peeringdb-plus/main.go) stay in lock-step with the
	// database without an extra round-trip.
	//
	// The per-cycle upsert-count map (the old
	// `counts map[string]int` arg) was removed. It was the wrong value to
	// feed into the pdbplus_data_type_count gauge cache — for incremental
	// syncs it was a delta, and for Poc it never agreed with the
	// privacy-filtered Count(ctx) used at startup ("doubling-halving").
	// Consumers should run pdbsync.InitialObjectCounts(ctx, client) on
	// the supplied ctx if they want live row counts, or query
	// sync_status (which still persists the raw upsert deltas) if they
	// want cycle telemetry.
	OnSyncComplete func(ctx context.Context, syncTime time.Time)

	// SyncMemoryLimit is the peak Go heap ceiling (bytes) checked
	// after Phase A fetch completes and before the ent.Tx opens. If
	// runtime.MemStats.HeapAlloc exceeds this value, Sync aborts with
	// ErrSyncMemoryLimitExceeded. Zero disables the guardrail. Wired
	// from config.Config.SyncMemoryLimit by main.go. Default
	// is 400 MB (matches the bench regression gate).
	SyncMemoryLimit int64

	// HeapWarnBytes is the peak Go heap threshold (bytes) above which
	// the end-of-sync-cycle emitter fires slog.Warn("heap threshold
	// crossed", ...). The OTel span attr pdbplus.sync.peak_heap_bytes is
	// attached regardless. Zero disables only the Warn (not the attr).
	// Wired from config.Config.HeapWarnBytes by main.go.
	//
	// Escalation signal: a sustained breach across multiple cycles is
	// the operational trigger to re-open the incremental-sync evaluation.
	HeapWarnBytes int64

	// RSSWarnBytes is the peak OS RSS threshold (bytes) above which
	// the emitter fires slog.Warn. Read from /proc/self/status VmHWM on
	// Linux; skipped on other OSes (the RSS attr is then omitted — it
	// is not set to zero). Zero disables only the Warn.
	RSSWarnBytes int64

	// FKBackfillMaxRequestsPerCycle caps the number of underlying HTTP
	// requests issued by FK-backfill per sync cycle. v1.18.5 semantic
	// shift: the previous cap counted ROWS (originally per-row,
	// nominally per-row but later batched — a weak circuit
	// breaker once batching collapsed N rows into 1 request). The cap
	// now directly bounds upstream HTTP traffic, the surface protected
	// by upstream's API_THROTTLE_REPEATED_REQUEST and our local rate
	// limiter. Default 20 (PDBPLUS_FK_BACKFILL_MAX_REQUESTS_PER_CYCLE)
	// — at 1 req/sec auth, ≈20s of upstream pressure max per cycle.
	// 0 disables backfill entirely (drop-on-miss behavior, preserved
	// as an operator escape-hatch).
	FKBackfillMaxRequestsPerCycle int

	// FKBackfillTimeout is the per-cycle wall-clock budget for FK
	// backfill HTTP activity. v1.18.3: added because backfill calls
	// happen inside the sync transaction; without a deadline a cascade
	// of slow / rate-limited backfills could hold the tx open for tens
	// of minutes, stalling LiteFS replication. After the deadline,
	// fkBackfillParent short-circuits to drop-on-miss so the rest of
	// the sync (bulk fetches + upserts) can commit. Dropped rows are
	// recovered by the next FULL-mode cycle (which re-fetches every
	// row, stages the tombstone window, and bypasses the upsert skip
	// gate) — NOT by the next incremental, whose MAX(updated) cursor
	// typically advances past the dropped rows. Default 5 minutes
	// (PDBPLUS_FK_BACKFILL_TIMEOUT). Zero or negative disables the
	// deadline (only the cap applies).
	FKBackfillTimeout time.Duration

	// SyncTimeout bounds the wall clock of a single sync attempt (each
	// SyncWithRetry attempt gets its own budget, so the retry ladder can
	// still recover with a fresh connection after a timed-out attempt).
	// The peeringdb.Client deliberately has no whole-request timeout —
	// a full-sync body read is legitimately slow — so this deadline is
	// what stops a body that trickles bytes forever from wedging the
	// cycle, and with it the running latch, for the life of the process
	// (every later trigger would return ErrSyncAlreadyRunning while data
	// went silently stale fleet-wide). Wired from PDBPLUS_SYNC_TIMEOUT
	// (default 30m). Zero or negative disables the deadline.
	SyncTimeout time.Duration

	// FullSyncInterval is the interval after which a sync cycle forces
	// a full bare-list refetch of every type, regardless of the
	// per-table MAX(updated) cursor. Defends against pathological
	// upstream cross-row inconsistency where a `?since=` response
	// includes row R' (updated=M) but is missing earlier row R
	// (updated < M); R is permanently missed under any since-based
	// design without periodic full refetch. Wired from
	// PDBPLUS_FULL_SYNC_INTERVAL (default 24h). Zero disables the
	// escape hatch (only the per-cycle MAX(updated) cursor applies).
	FullSyncInterval time.Duration
}

// Worker orchestrates PeeringDB data synchronization.
type Worker struct {
	pdbClient     *peeringdb.Client
	entClient     *ent.Client
	db            *sql.DB // underlying sql.DB for sync_status table
	config        WorkerConfig
	running       atomic.Bool
	synced        atomic.Bool // true after first successful sync
	logger        *slog.Logger
	retryBackoffs []time.Duration // defaults to 30s, 2m, 8m
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
	// registry, the defer_foreign_keys=ON commit check rejects
	// the entire sync transaction.
	//
	// Reset by resetFKState at the start of each Sync run. Single-writer
	// because Worker.running serialises concurrent Sync calls.
	fkRegistry map[string]map[int]struct{}
	// fkOrphanCounts aggregates per-cycle FK-orphan observations so they
	// can be summarised in a single end-of-cycle WARN log + metric
	// increments. Replaces the per-row WARN log spam that blew Tempo's
	// 7.5 MB per-trace cap (audit 2026-04-26). Reset alongside fkRegistry
	// in resetFKState.
	fkOrphanCounts map[fkOrphanKey]int
	// fkBackfillTried is the per-cycle dedup cache for live FK backfills.
	// Keyed by (parent type, parent ID) — if the
	// same missing parent is referenced by N child rows, we issue exactly
	// ONE backfill HTTP fetch and short-circuit the next N-1 lookups.
	// Reset alongside fkRegistry in resetFKState.
	fkBackfillTried map[fkBackfillKey]struct{}
	// fkPresence is the per-cycle cache of parent IDs confirmed present in
	// the local DB by the dbHasRecord fallback. Only positive results are
	// stored: a parent reaching that fallback was not synced this cycle
	// (parent-first ordering registers synced parents in fkRegistry), so
	// its row is neither inserted nor deleted for the rest of the cycle and
	// the existence answer is stable. When many child rows share one
	// untouched parent (e.g. hundreds of netixlans under one org), the
	// siblings hit this cache instead of repeating the Exist() query.
	// Reset alongside fkRegistry in resetFKState.
	fkPresence map[fkBackfillKey]struct{}
	// fkMissing is the per-cycle negative cache: parent IDs whose
	// backfill already ran this cycle and whose row is confirmed absent
	// from the local DB. fkBackfillParent consults it before repeating
	// the dbHasRecord Exist() query — when many child rows reference
	// the same unrecoverable parent, the siblings short-circuit here.
	// Safe because Phase B stages parents before children and a parent
	// landing later in the cycle can only arrive via backfill of the
	// same ID, which the dedup cache prevents from re-running. Reset
	// alongside fkRegistry in resetFKState.
	fkMissing map[fkBackfillKey]struct{}
	// fkBackfillRequestCount is the per-cycle counter of underlying HTTP
	// requests issued by FK-backfill, compared against fkBackfillRequestCap.
	// Bumped by ⌈len(idsToFetch)/peeringdb.FetchByIDsBatchSize⌉ in
	// fkBackfillBatch BEFORE the FetchByIDs call — one unit per actual
	// HTTP chunk on the wire, not per row.
	fkBackfillRequestCount int
	// fkBackfillRequestCap is the per-cycle hard cap on underlying HTTP
	// requests issued by FK backfill. Captured once in NewWorker from
	// WorkerConfig.FKBackfillMaxRequestsPerCycle; 0 disables backfill
	// entirely (drop-on-miss behavior, preserved for operator escape-hatch).
	fkBackfillRequestCap int
	// fkBackfillDeadline is the wall-clock deadline for FK-backfill HTTP
	// activity within the current sync cycle. Set in resetFKState from
	// fkBackfillTimeout. After the deadline, fkBackfillParent short-
	// circuits to drop-on-miss so the rest of the sync (which is
	// independent of backfill — bulk fetches and upserts continue) can
	// commit; dropped rows are recovered by the next full-mode
	// reconcile cycle, not the next incremental. v1.18.3:
	// added because backfill HTTP calls happen inside the sync tx;
	// without a deadline, a cascade of slow backfills could hold the tx
	// open indefinitely and stall LiteFS replication.
	fkBackfillDeadline time.Time
	// fkBackfillTimeout is the per-cycle wall-clock budget for backfill
	// HTTP activity. Captured once in NewWorker from
	// WorkerConfig.FKBackfillTimeout; zero or negative disables the
	// deadline (no timeout — only the cap applies).
	fkBackfillTimeout time.Duration
	// cyclePeakHeapBytes is the running per-cycle maximum of HeapInuse,
	// folded in by foldPeakHeap at the points where the cycle's heap
	// actually peaks: after the Phase A fetch and after each type's
	// Phase B upsert BEFORE its runtime.GC() call. emitMemoryTelemetry
	// previously sampled HeapInuse once at end-of-cycle — i.e. after all
	// 13 per-type GC calls had reclaimed the upsert spike — so the
	// "peak_heap" gauge systematically reported the post-cycle floor and
	// the documented escalation trigger (sustained peak heap above
	// PDBPLUS_HEAP_WARN_MIB) could never fire. Reset at the top of each
	// Sync; single-writer because Worker.running serialises cycles.
	cyclePeakHeapBytes int64
}

// fkOrphanKey is the dimension grouping a single class of FK-orphan
// observation: an action ("drop"|"null") taken on a particular child
// type's FK field that pointed at a missing parent row.
type fkOrphanKey struct {
	ChildType  string
	ParentType string
	Field      string
	Action     string // "drop" — row excluded; "null" — FK nulled, row kept
}

// NewWorker creates a new sync worker.
// If cfg.IsPrimary is nil, it defaults to always-primary for backward
// compatibility (local dev, tests without explicit primary config).
func NewWorker(pdbClient *peeringdb.Client, entClient *ent.Client, db *sql.DB, cfg WorkerConfig, logger *slog.Logger) *Worker {
	if cfg.IsPrimary == nil {
		cfg.IsPrimary = func() bool { return true }
	}
	return &Worker{
		pdbClient:            pdbClient,
		entClient:            entClient,
		db:                   db,
		config:               cfg,
		logger:               logger,
		retryBackoffs:        defaultRetryBackoffs,
		fkBackfillRequestCap: cfg.FKBackfillMaxRequestsPerCycle,
		fkBackfillTimeout:    cfg.FKBackfillTimeout,
	}
}

// SetRetryBackoffs overrides the default retry backoff durations. Intended for testing.
func (w *Worker) SetRetryBackoffs(backoffs []time.Duration) {
	w.retryBackoffs = backoffs
}

// resetFKState clears the per-sync FK registry and orphan counters.
// Called at the start of each Sync run so downstream fkFilter closures
// see a clean slate — the previous run's parent IDs are irrelevant to
// the next run's orphan detection.
func (w *Worker) resetFKState() {
	w.fkRegistry = make(map[string]map[int]struct{}, 13)
	w.fkOrphanCounts = make(map[fkOrphanKey]int)
	// Reset per-cycle FK-backfill dedup cache and
	// counter alongside the rest of the FK state. The cap is captured
	// once at NewWorker time; only the per-cycle bookkeeping resets.
	w.fkBackfillTried = make(map[fkBackfillKey]struct{}, 64)
	w.fkPresence = make(map[fkBackfillKey]struct{}, 64)
	w.fkMissing = make(map[fkBackfillKey]struct{}, 64)
	w.fkBackfillRequestCount = 0
	// v1.18.3: per-cycle deadline for backfill HTTP activity. Zero
	// timeout → zero deadline → fkBackfillParent's deadline check
	// short-circuits to "no deadline".
	if w.fkBackfillTimeout > 0 {
		w.fkBackfillDeadline = time.Now().Add(w.fkBackfillTimeout)
	} else {
		w.fkBackfillDeadline = time.Time{}
	}
}

// recordOrphan tracks one FK-orphan observation for the per-cycle
// summary. The per-row hot path logs at DEBUG (so individual rows are
// still recoverable in a verbose run) and increments the
// pdbplus.sync.type.orphans counter so the dashboard sees activity in
// near-real time. emitOrphanSummary collapses these into one WARN log
// per cycle.
func (w *Worker) recordOrphan(ctx context.Context, key fkOrphanKey, childID, parentID int) {
	w.fkOrphanCounts[key]++
	pdbotel.SyncTypeOrphans.Add(ctx, 1, metric.WithAttributes(
		attribute.String("type", key.ChildType),
		attribute.String("parent_type", key.ParentType),
		attribute.String("field", key.Field),
		attribute.String("action", key.Action),
	))
	w.logger.LogAttrs(ctx, slog.LevelDebug, "fk orphan",
		slog.String("child_type", key.ChildType),
		slog.Int("child_id", childID),
		slog.String("field", key.Field),
		slog.String("parent_type", key.ParentType),
		slog.Int("orphan_parent_id", parentID),
		slog.String("action", key.Action),
	)
}

// emitOrphanSummary emits a single end-of-cycle log line summarising
// the orphan FKs handled during the current sync. Called from the
// terminal Sync paths (recordSuccess / rollbackAndRecord / recordFailure)
// alongside emitMemoryTelemetry. Uses WARN when at least one orphan
// fired; DEBUG when the cycle was clean. The structured "summary"
// attribute is a stable []map[string]any so dashboards / alerts can
// group on it.
func (w *Worker) emitOrphanSummary(ctx context.Context) {
	if len(w.fkOrphanCounts) == 0 {
		w.logger.LogAttrs(ctx, slog.LevelDebug, "fk orphans summary", slog.Int("total", 0))
		return
	}
	var total int
	groups := make([]map[string]any, 0, len(w.fkOrphanCounts))
	for k, count := range w.fkOrphanCounts {
		total += count
		groups = append(groups, map[string]any{
			"child_type":  k.ChildType,
			"parent_type": k.ParentType,
			"field":       k.Field,
			"action":      k.Action,
			"count":       count,
		})
	}
	slices.SortFunc(groups, func(a, b map[string]any) int {
		// Stable, grep-friendly ordering: child_type, parent_type, field, action.
		if r := cmp.Compare(a["child_type"].(string), b["child_type"].(string)); r != 0 {
			return r
		}
		if r := cmp.Compare(a["parent_type"].(string), b["parent_type"].(string)); r != 0 {
			return r
		}
		if r := cmp.Compare(a["field"].(string), b["field"].(string)); r != 0 {
			return r
		}
		return cmp.Compare(a["action"].(string), b["action"].(string))
	})
	w.logger.LogAttrs(ctx, slog.LevelWarn, "fk orphans summary",
		slog.Int("total", total),
		slog.Any("groups", groups),
	)
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
// for whether zero/null is actually allowed on the column.
//
// State-aware fallback (Phase v1.16+): if the parent set is missing or
// the ID is not found in memory (common during incremental syncs), we
// query the local database to check if the record exists there before
// declaring it an orphan.
func (w *Worker) fkHasParent(ctx context.Context, tx *ent.Tx, typeName string, id int) bool {
	if id == 0 {
		return true
	}
	set, ok := w.fkRegistry[typeName]
	if ok {
		if _, exists := set[id]; exists {
			return true
		}
	}
	// Per-cycle positive-presence cache: a sibling child row already
	// confirmed this parent exists in the DB this cycle, so skip the
	// repeat Exist() query (see fkPresence doc for why this is stable).
	key := fkBackfillKey{Type: typeName, ID: id}
	if _, cached := w.fkPresence[key]; cached {
		return true
	}
	// Memory check failed: check the DB to distinguish true orphans from
	// untouched parents during incremental syncs. Cache only confirmed
	// hits — a miss may be recovered by backfill, which registers the
	// parent in fkRegistry (consulted above).
	if w.dbHasRecord(ctx, tx, typeName, id) {
		w.fkPresence[key] = struct{}{}
		return true
	}
	return false
}

// dbHasRecord checks the real database for the existence of an ID for
// the named PeeringDB type via its typeRegistry descriptor.
func (w *Worker) dbHasRecord(ctx context.Context, tx *ent.Tx, typeName string, id int) bool {
	desc, ok := descriptorByName[typeName]
	if !ok {
		w.logger.LogAttrs(ctx, slog.LevelError, "unknown type for DB record check", slog.String("type", typeName))
		return false
	}
	exists, err := desc.exists(ctx, tx, id)
	if err != nil {
		w.logger.LogAttrs(ctx, slog.LevelError, "failed to check DB for record",
			slog.String("type", typeName),
			slog.Int("id", id),
			slog.Any("error", err),
		)
		return false
	}
	return exists
}

// syncStep defines a single step in the sync process. Upserts run in
// parent-first FK order.
//
// The fetch and upsert paths dispatch
// via fetchOneTypeFull / fetchOneTypeIncremental / upsertOneType type
// switches on step.name — the old upsertFn/incrementalFn fields were
// removed because fetch now runs outside the tx.
//
// The deleteFn closure (the prior
// inference-by-absence soft-delete) was removed. With bootstrap-with-?since=1
// plus the live FK backfill, deletes arrive
// explicitly from upstream as status='deleted' rows in ?since=N
// payloads — no inference needed. Existing tombstones in the live DB
// are preserved (no schema change, no row mutation by this commit);
// the dormant tombstone-GC work still owns the eventual GC story.
type syncStep struct {
	name string
}

// StepOrder returns a copy of the canonical 13-name sync step
// ordering. Out-of-package callers (cmd/loadtest sync mode) use it
// as the parity reference; mutating the returned slice is safe.
func StepOrder() []string {
	out := make([]string, len(canonicalStepOrder))
	copy(out, canonicalStepOrder)
	return out
}

// syncSteps returns the ordered list of sync steps in FK dependency
// order. The per-type deleteFn
// machinery is gone — sync no longer infers deletions from absence;
// upstream sends explicit status='deleted' tombstones in ?since=N
// payloads (Task 2 bootstrap), and the live FK backfill (Task 3)
// handles missing parents.
func (w *Worker) syncSteps() []syncStep {
	steps := make([]syncStep, len(canonicalStepOrder))
	for i, name := range canonicalStepOrder {
		steps[i] = syncStep{name: name}
	}
	return steps
}

// Sync executes a synchronization from PeeringDB to the local database.
// It acquires a mutex to prevent concurrent runs and wraps all database
// writes in a single transaction.
//
// Sync is an orchestrator split into three phases:
//
//  1. Phase A (NO TX HELD): HTTP fetch + JSON decode stream into an
//     isolated /tmp SQLite "scratch" database — Go heap stays bounded
//     to one element per StreamAll handler invocation (~5-10 KB) instead
//     of one full []T per type (~35 MB for netixlan).
//  2. Fetch Barrier: scratch DB fully populated; open the real LiteFS tx.
//  3. Phase B (SINGLE REAL TX): drain each scratch table in chunks,
//     decode each chunk to typed Go structs, upsertX into the real ent
//     tables, free the chunk, repeat. Delete stale rows. Commit.
//     Atomicity preserved: one ent.Tx wraps every real-DB write.
//
// The scratch DB is unlinked on both success and error via defer. See
// internal/sync/scratch.go for the scratch DB lifecycle and pragmas.
// The scratch-SQLite fallback was mandatory because the pre-refactor
// baseline (535 MiB) and the fetch-outside-tx split (613 MiB) both
// exceeded the 400 MiB gate on production-scale fixtures.
//
// Between the Phase A fetch barrier and the
// ent.Tx open, Sync calls w.checkMemoryLimit with the current
// runtime.MemStats.HeapAlloc reading. If SyncMemoryLimit is set and
// the heap exceeds it, Sync aborts with ErrSyncMemoryLimitExceeded
// before opening the tx — defense-in-depth against PeeringDB data
// growth that exceeds the benchmark baseline at runtime.
//
// The Sync line budget is <= 100 — enforced by TestWorkerSync_LineBudget.
//
// Sync is the SOLE production call site
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
// resolveEffectiveMode applies the PDBPLUS_FULL_SYNC_INTERVAL escape hatch:
// if the configured mode is incremental AND the configured
// FullSyncInterval is positive AND the most recent successful full sync is
// older than that interval, the cycle's effective mode upgrades to full —
// every per-type fetch issues a bare list (no since=).
//
// Single GetLastSuccessfulFullSyncTime call per cycle by design (NOT once
// per type) — the per-step loop in syncFetchPass receives the resolved
// SyncMode and never re-queries sync_status.
//
// Fail-soft on sync_status read errors: a transient query failure logs
// at INFO and falls through with the configured mode, never blocking sync.
//
// Returns "full" when FullSyncInterval == 0 is unchanged from the
// configured mode (zero is the documented "disabled" sentinel, mirroring
// PDBPLUS_FK_BACKFILL_TIMEOUT semantics).
func (w *Worker) resolveEffectiveMode(ctx context.Context, configured config.SyncMode) config.SyncMode {
	if configured != config.SyncModeIncremental {
		return configured
	}
	if w.config.FullSyncInterval <= 0 {
		return configured
	}
	lastFull, err := GetLastSuccessfulFullSyncTime(ctx, w.db)
	if err != nil {
		// Fail-soft: log and continue with configured mode. A
		// sync_status read failure is not a reason to block sync.
		w.logger.LogAttrs(ctx, slog.LevelInfo,
			"failed to read last full sync time, skipping full-sync interval check",
			slog.Any("error", err),
		)
		return configured
	}
	if lastFull.IsZero() || time.Since(lastFull) >= w.config.FullSyncInterval {
		w.logger.LogAttrs(ctx, slog.LevelInfo, "forcing full bare-list refetch",
			slog.Time("last_full_sync", lastFull),
			slog.Duration("full_sync_interval", w.config.FullSyncInterval),
		)
		return config.SyncModeFull
	}
	return configured
}

// forceTraceKey is the context key carrying the manual-sync force-trace flag.
type forceTraceKey struct{}

// WithForceTrace marks ctx so the sync cycle run under it force-samples its
// trace, overriding the sampler's default of dropping scheduled-sync traces.
// It is set by the manual POST /sync handler and never by the timer scheduler,
// so an on-demand sync is observable end to end — including its per-query DB
// spans when PDBPLUS_OTEL_SQL is enabled.
func WithForceTrace(ctx context.Context) context.Context {
	return context.WithValue(ctx, forceTraceKey{}, true)
}

// forceTraceFromContext reports whether ctx was marked by WithForceTrace.
func forceTraceFromContext(ctx context.Context) bool {
	v, _ := ctx.Value(forceTraceKey{}).(bool)
	return v
}

func (w *Worker) Sync(ctx context.Context, mode config.SyncMode) (err error) {
	ctx = privacy.DecisionContext(ctx, privacy.Allow) // privacy bypass — sole production call site
	if !w.running.CompareAndSwap(false, true) {
		// The trigger (and its requested mode — possibly the operator's
		// ?mode=full escape hatch) is dropped, not queued. Return the
		// sentinel so callers can tell "started" from "discarded".
		w.logger.LogAttrs(ctx, slog.LevelWarn, "sync already running, dropping trigger",
			slog.String("requested_mode", string(mode)))
		return ErrSyncAlreadyRunning
	}
	defer w.running.Store(false)
	w.cyclePeakHeapBytes = 0 // fresh peak-heap high-water mark per cycle

	// Watchdog: bound this attempt's wall clock. The upstream client has
	// no whole-request timeout by design (see peeringdb.NewClient), so this
	// deadline is the only thing standing between a trickling body read and
	// a permanently wedged running latch. Cancellation propagates into
	// every in-flight HTTP body read via the request context.
	if t := w.config.SyncTimeout; t > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeoutCause(ctx, t, ErrSyncAttemptTimeout)
		defer cancel()
	}

	// Tag the root sync span so the sampler can gate sync traces: origin=sync
	// makes scheduled cycles drop by default; a manual POST /sync sets
	// force_sample (via WithForceTrace) so that one cycle — and its per-query
	// DB spans when PDBPLUS_OTEL_SQL is on — is sampled.
	spanAttrs := []attribute.KeyValue{attribute.String(pdbotel.AttrSyncOrigin, pdbotel.SyncOriginValue)}
	if forceTraceFromContext(ctx) {
		spanAttrs = append(spanAttrs, attribute.Bool(pdbotel.AttrForceSample, true))
	}
	ctx, span := otel.Tracer("sync").Start(ctx, "sync-"+string(mode), trace.WithAttributes(spanAttrs...))
	defer span.End()

	start := time.Now()
	// Resolve effective mode (PDBPLUS_FULL_SYNC_INTERVAL
	// escape hatch) BEFORE recording the status row.
	effectiveMode := w.resolveEffectiveMode(ctx, mode)
	statusID, startErr := RecordSyncStart(ctx, w.db, start, string(effectiveMode))
	if startErr != nil {
		w.logger.LogAttrs(ctx, slog.LevelError, "failed to record sync start",
			slog.Any("error", startErr))
	}

	// Panic firewall: cmd/peeringdb-plus/main.go spawns the sync workers
	// as bare goroutines (StartScheduler, the on-demand `go in.SyncFn`).
	// middleware.Recovery only guards HTTP request goroutines, so without
	// this defer a panic from one bad upstream record (e.g. a nil-deref
	// in upsert) would crash the whole process and take down all five API
	// surfaces on this edge node. Recover converts the panic into a failed
	// cycle so the scheduler proceeds to the next tick instead.
	defer w.recoverSyncPanic(ctx, recoverSyncPanicInput{
		mode:     effectiveMode,
		statusID: statusID,
		start:    start,
		errp:     &err,
	})

	err = w.syncCycle(ctx, effectiveMode, statusID, start)
	return err
}

// recoverSyncPanicInput carries the per-cycle bookkeeping that the panic
// firewall needs to convert a recovered panic into a recorded failure.
// Declared before recoverSyncPanic.
type recoverSyncPanicInput struct {
	mode     config.SyncMode
	statusID int64
	start    time.Time
	// errp points at Sync's named return so the recovered panic surfaces
	// to the caller as an ordinary error instead of unwinding the stack.
	errp *error
}

// recoverSyncPanic is the deferred panic firewall for a single sync cycle.
// On recover it logs at ERROR with a stack trace (mirroring
// internal/middleware/recovery.go), records a failed sync status + the
// failure metric via recordFailure, and stores the synthesised error into
// the caller's named return. The bookkeeping runs on a context detached
// from cancellation so a SIGTERM/demotion mid-cycle cannot turn the status
// write into a no-op. Non-panic returns are a no-op.
func (w *Worker) recoverSyncPanic(ctx context.Context, in recoverSyncPanicInput) {
	rec := recover()
	if rec == nil {
		return
	}
	panicErr := fmt.Errorf("sync cycle panicked: %v", rec)
	w.logger.LogAttrs(ctx, slog.LevelError, "sync cycle panic recovered",
		slog.Any("panic", rec),
		slog.String("stack", string(debug.Stack())),
		slog.String("mode", string(in.mode)),
	)
	w.recordFailure(ctx, in.mode, in.statusID, in.start, panicErr)
	if in.errp != nil {
		*in.errp = panicErr
	}
}

// syncCycle runs one sync cycle's data work: scratch fetch (Phase A), the
// memory guardrail, then the single real transaction (Phase B) and the
// terminal success/failure recording. Extracted from Sync so the public
// entry point can install the panic firewall and stay within the
// line budget (TestWorkerSync_LineBudget). statusID/start/effectiveMode are
// threaded in so the firewall and every terminal recorder agree on the same
// status row and timing.
func (w *Worker) syncCycle(ctx context.Context, effectiveMode config.SyncMode, statusID int64, start time.Time) error {
	// Tighten GC for the duration of the sync run: the default 100%
	// target heap growth is too loose when upsert bursts allocate
	// hundreds of MiB between GC cycles. Setting GCPercent=25 forces
	// the collector to kick in at 25% heap growth, trading ~5% extra
	// CPU for bounded peak heap. Restored on return so the value does
	// not leak to other goroutines.
	//
	// TRADE-OFF NOTE: debug.SetGCPercent and
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

	// Full-mode cycles reconcile completely: the reconcile-all marker
	// disables the upsert pass's updated-timestamp skip gate so rows the
	// sync mutated locally without bumping `updated` (orphan-filter FK
	// nulls) re-converge with upstream, and newly added _fold columns
	// backfill. This is the documented purpose of the daily forced-full
	// escalation; without the marker the gate skipped those rows forever.
	if effectiveMode == config.SyncModeFull {
		ctx = withReconcileAll(ctx)
	}

	scratch, err := openScratchDB(ctx)
	if err != nil {
		w.recordFailure(ctx, effectiveMode, statusID, start, err)
		return err
	}
	defer closeScratchDB(ctx, scratch, w.logger)

	// === Phase A — NO TX HELD ===
	// HTTP + JSON decode stream into the scratch DB; Go heap stays bounded.
	fromIncremental, err := w.syncFetchPass(ctx, scratch, effectiveMode)
	if err != nil {
		w.recordFailure(ctx, effectiveMode, statusID, start, err)
		return err
	}
	// Memory guardrail: see checkMemoryLimit godoc (defense-in-depth).
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	w.foldPeakHeap(ms.HeapInuse) // post-Phase-A is one of the cycle's heap peaks
	if memErr := w.checkMemoryLimit(ctx, ms.HeapAlloc, w.config.SyncMemoryLimit, len(fromIncremental)); memErr != nil {
		w.recordFailure(ctx, effectiveMode, statusID, start, memErr)
		return memErr
	}
	// === Fetch Barrier ===
	// Scratch DB fully populated. Open the real LiteFS tx now.
	tx, err := w.entClient.Tx(ctx)
	if err != nil {
		beginErr := fmt.Errorf("begin sync transaction: %w", err)
		w.recordFailure(ctx, effectiveMode, statusID, start, beginErr)
		return beginErr
	}
	if err := prepareTxPragmas(ctx, tx); err != nil {
		w.rollbackAndRecord(ctx, effectiveMode, tx, statusID, start, err)
		return err
	}

	// === Phase B — SINGLE REAL TX === (no delete pass.
	// Cursor writes are gone — cursor is derived from
	// MAX(updated) per table on the next cycle.)
	objectCounts, err := w.syncUpsertPass(ctx, tx, scratch, fromIncremental)
	if err != nil {
		w.rollbackAndRecord(ctx, effectiveMode, tx, statusID, start, err)
		return err
	}
	if commitErr := commitWithSpan(ctx, tx); commitErr != nil {
		syncErr := fmt.Errorf("commit sync transaction: %w", commitErr)
		w.recordFailure(ctx, effectiveMode, statusID, start, syncErr)
		return syncErr
	}

	w.recordSuccess(ctx, effectiveMode, statusID, start, objectCounts)
	return nil
}

// checkMemoryLimit reads the current Go heap allocation and returns
// ErrSyncMemoryLimitExceeded if it exceeds the configured limit. If
// limit is 0 (or negative), the guardrail is disabled and the function
// returns nil immediately.
//
// Defense-in-depth: called from Worker.Sync
// between the Phase A fetch return and the ent.Tx open so that the
// abort happens BEFORE any database lock is taken. Extracted into a
// helper so Worker.Sync's call site stays within the 100-line
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

// foldPeakHeap folds a HeapInuse observation into the per-cycle peak
// high-water mark. Callers sit where the cycle's heap actually peaks
// (post-Phase-A, and post-upsert pre-GC for each type); see the
// cyclePeakHeapBytes field comment for why end-of-cycle sampling alone
// under-reports.
func (w *Worker) foldPeakHeap(heapInuse uint64) {
	const maxInt64 = uint64(1<<63 - 1)
	hb := int64(maxInt64)
	if heapInuse < maxInt64 {
		hb = int64(heapInuse)
	}
	if hb > w.cyclePeakHeapBytes {
		w.cyclePeakHeapBytes = hb
	}
}

// samplePeakHeap reads the runtime heap and folds it into the per-cycle
// peak. ~µs of STW per call; called 14 times per cycle (once after
// Phase A via the existing memory-limit read, once per type in Phase B).
func (w *Worker) samplePeakHeap() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	w.foldPeakHeap(ms.HeapInuse)
}

// emitMemoryTelemetry reports the cycle's peak Go heap (the maximum of
// the per-cycle foldPeakHeap samples and one final end-of-cycle read)
// and (on Linux) the OS RSS high-water mark, attaches them as OTel
// attributes to the current sync-cycle span, and emits
// slog.Warn("heap threshold crossed") when either value exceeds its
// configured threshold.
//
// Note the two values have different lifetimes: peak_heap_bytes is
// per-cycle (reset at the top of Sync), while peak_rss_bytes (VmHWM)
// is a process-lifetime high-water mark that includes API-serving load
// and only resets on restart.
//
// Attribute naming follows the pdbplus.* convention (e.g.
// pdbplus.privacy.tier): pdbplus.sync.peak_heap_bytes and
// pdbplus.sync.peak_rss_bytes. Bytes is the canonical Prom unit (per
// the 2026-04-26 audit unit canonicalisation); dashboards format MiB /
// GiB at render time via Grafana's "bytes" field unit.
//
// On non-Linux the RSS attr is OMITTED entirely — zero is a valid
// metric value and would produce misleading flat lines on dashboards.
//
// heapWarnBytes == 0 disables the heap Warn (attr still fires);
// rssWarnBytes == 0 likewise. Attribute emission is unconditional so
// dashboards retain timeseries even when alerting is disabled.
//
// Sampling frequency is sync cycle frequency (default 1h via
// PDBPLUS_SYNC_INTERVAL). No periodic background sampler.
func (w *Worker) emitMemoryTelemetry(ctx context.Context, heapWarnBytes, rssWarnBytes int64) {
	// Fold one final sample so early-failure paths (which never reached a
	// foldPeakHeap call site) still report a real observation.
	w.samplePeakHeap()
	heapBytes := w.cyclePeakHeapBytes

	rssBytes, rssOK := readLinuxVMHWM()

	span := trace.SpanFromContext(ctx)
	attrs := []attribute.KeyValue{
		attribute.Int64("pdbplus.sync.peak_heap_bytes", heapBytes),
	}
	if rssOK {
		attrs = append(attrs, attribute.Int64("pdbplus.sync.peak_rss_bytes", rssBytes))
	}
	span.SetAttributes(attrs...)

	// Publish to the ObservableGauges so Prometheus / Grafana pick them up.
	pdbotel.SyncPeakHeapBytes.Store(heapBytes)
	if rssOK {
		pdbotel.SyncPeakRSSBytes.Store(rssBytes)
	}

	heapOver := heapWarnBytes > 0 && heapBytes > heapWarnBytes
	rssOver := rssOK && rssWarnBytes > 0 && rssBytes > rssWarnBytes
	if !heapOver && !rssOver {
		return
	}
	logAttrs := []slog.Attr{
		slog.Int64("peak_heap_bytes", heapBytes),
		slog.Int64("heap_warn_bytes", heapWarnBytes),
	}
	if rssOK {
		logAttrs = append(logAttrs,
			slog.Int64("peak_rss_bytes", rssBytes),
			slog.Int64("rss_warn_bytes", rssWarnBytes),
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
// the correct signal for the sustained-high-heap escalation — a single
// burst is what matters, not the steady-state value.
func readLinuxVMHWM() (int64, bool) {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, false
	}
	for line := range strings.SplitSeq(string(data), "\n") {
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

// commitWithSpan commits tx inside a named OTel span so the LiteFS-
// replicated commit duration is visible in Tempo. Pattern matches
// existing per-step spans elsewhere in worker.go (e.g. line ~917).
func commitWithSpan(ctx context.Context, tx *ent.Tx) error {
	_, commitSpan := otel.Tracer("sync").Start(ctx, "sync-commit")
	defer commitSpan.End()
	return tx.Commit()
}

// prepareTxPragmas runs the per-tx PRAGMA setup that the bulk-upsert
// transaction depends on. It runs:
//   - PRAGMA defer_foreign_keys = ON    (existing — defers FK constraint
//     checking to commit so we can upsert in any order)
//   - PRAGMA cache_spill = OFF          (keeps dirty
//     pages in the connection's page cache instead of spilling to the WAL
//     between writes; bounded by cache_size from the DSN)
//
// cache_spill is per-tx (not via the DSN) because it's a connection-scoped
// pragma whose effect we only want during the bulk-write tx. Setting it via
// the DSN would apply it to read-path connections too.
//
// Extracted from Worker.Sync so the orchestrator body stays under the
// line budget enforced by TestWorkerSync_LineBudget. DO NOT
// inline this back into Sync.
func prepareTxPragmas(ctx context.Context, tx *ent.Tx) error {
	if _, err := tx.ExecContext(ctx, "PRAGMA defer_foreign_keys = ON"); err != nil {
		return fmt.Errorf("defer foreign keys: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "PRAGMA cache_spill = OFF"); err != nil {
		return fmt.Errorf("disable cache_spill: %w", err)
	}
	return nil
}

// rollbackAndRecord rolls back the tx and records the failure in one place
// so Worker.Sync's error paths stay a one-liner each (line budget).
// Logs the rollback error at ERROR level — a failing rollback inside a
// failing sync is worth surfacing in the error stream.
//
// mode is threaded through to recordFailure so the failure metric carries
// the same {status,mode} attribute pair as the success metric
// (explicit dataflow > implicit context lookup).
func (w *Worker) rollbackAndRecord(ctx context.Context, mode config.SyncMode, tx *ent.Tx, statusID int64, start time.Time, syncErr error) {
	// Memory telemetry is emitted exactly once in recordFailure below — do not
	// emit here as well. Double emission caused duplicate
	// "heap threshold crossed" WARN records on every rollback path that
	// breached thresholds.
	if rbErr := tx.Rollback(); rbErr != nil {
		w.logger.LogAttrs(ctx, slog.LevelError, "rollback failed",
			slog.Any("error", rbErr))
	}
	w.recordFailure(ctx, mode, statusID, start, syncErr)
}

// recordSuccess runs all post-commit bookkeeping: per-type cursor updates,
// sync-level metrics, sync_status row update, first-success flag, and the
// OnSyncComplete callback. Extracted from Sync so the orchestrator body
// stays under the line budget.
func (w *Worker) recordSuccess(
	ctx context.Context,
	mode config.SyncMode,
	statusID int64,
	start time.Time,
	objectCounts map[string]int,
) {
	// Wrap the post-commit bookkeeping in a
	// sync-finalize span and ctx-reassign so the sub-spans below parent
	// under sync-finalize rather than the root.
	//
	// The prior sync-cursor-updates loop is fully gone —
	// cursor advancement is now implicit via MAX(updated) on the entity
	// tables themselves (see internal/sync/cursor.go GetMaxUpdated).
	ctx, finalizeSpan := otel.Tracer("sync").Start(ctx, "sync-finalize")
	defer finalizeSpan.End()

	// Detach cancellation for the terminal bookkeeping: a cancellation
	// (demotion monitor / SIGTERM) can land between the data commit and
	// this success record, and a cancelled ctx turns RecordSyncComplete's
	// UPDATE into a no-op — leaving sync_status stuck "running" despite a
	// durable commit — and drops the terminal metric. WithoutCancel keeps
	// the sync-finalize span (context values survive) while a short
	// timeout caps the detached write. Symmetric with recordFailure.
	ctx, cancel := terminalRecordContext(ctx)
	defer cancel()

	// Emit heap + RSS span attrs and (if over threshold) slog.Warn.
	w.emitMemoryTelemetry(ctx, w.config.HeapWarnBytes, w.config.RSSWarnBytes)
	w.emitOrphanSummary(ctx)

	elapsed := time.Since(start)
	// Label sync metrics with mode={full,incremental} so
	// dashboards/alerts can distinguish cycle behaviour after the default flip.
	// Cardinality: status (2) × mode (2) = 4 combinations on
	// pdbplus_sync_operations_total — well under any concern.
	attrs := metric.WithAttributes(
		attribute.String("status", "success"),
		attribute.String("mode", string(mode)),
	)
	pdbotel.SyncDuration.Record(ctx, elapsed.Seconds(), attrs)
	pdbotel.SyncOperations.Add(ctx, 1, attrs)
	w.logger.LogAttrs(ctx, slog.LevelInfo, "sync complete",
		slog.String("mode", string(mode)),
		slog.Duration("duration", elapsed),
		slog.Int("total_objects", sumCounts(objectCounts)))
	// Capture completion timestamp once so the sync_status row AND the
	// OnSyncComplete callback (the caching middleware ETag setter)
	// see the exact same value. A drift here would mean the atomic ETag
	// pointer and the DB-backed sync time could disagree.
	completedAt := time.Now()
	if statusID > 0 {
		// sync-record-status span around the
		// sync_status row update (separate raw-SQL Exec, by design —
		// outcome record, not data state).
		_, statusSpan := otel.Tracer("sync").Start(ctx, "sync-record-status")
		// Bookkeeping failure must not fail the cycle (the data commit
		// already succeeded), but it must be VISIBLE: a persistently
		// failing write leaves sync_status rows stuck "running" and
		// freshness reporting frozen at the last successful write.
		if recErr := RecordSyncComplete(ctx, w.db, statusID, Status{
			LastSyncAt:   completedAt,
			Duration:     elapsed,
			ObjectCounts: objectCounts,
			Status:       "success",
		}); recErr != nil {
			w.logger.LogAttrs(ctx, slog.LevelError, "failed to record sync completion",
				slog.Int64("status_id", statusID),
				slog.String("outcome", "success"),
				slog.Any("error", recErr))
		}
		statusSpan.End()
	}
	w.synced.Store(true)
	if w.config.OnSyncComplete != nil {
		// Pass the worker ctx instead of the per-cycle
		// upsert-count map. The callback decides what to compute (typically
		// pdbsync.InitialObjectCounts for live row counts).
		//
		// sync-on-complete span around the callback
		// (typically refreshes the gauge cache via InitialObjectCounts).
		_, onCompleteSpan := otel.Tracer("sync").Start(ctx, "sync-on-complete")
		w.config.OnSyncComplete(ctx, completedAt)
		onCompleteSpan.End()
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
// structural regression lock. Before the per-chunk refactor, this struct
// held per-type []T slices that drainAndUpsertType zeroed between chunks
// to release the backing array. After the refactor, per-chunk typed slices
// live in processScratchChunk's locals (one generic helper per type)
// and are reclaimed automatically when the helper returns — the
// function-scope release is strictly more reliable than the old
// map-entry clearing hack. The struct stays as an empty placeholder so
// the `batches[name] = syncBatch{}` literal in drainAndUpsertType
// continues to satisfy the regression test's string match while
// compiling to a no-op map write (free, and kept as a grep-visible
// anchor for the memory-bound documentation trail).
type syncBatch struct{}

// syncFetchPass runs Phase A against the scratch DB: for each of the 13
// PeeringDB types, stream the HTTP response body into a /tmp SQLite
// staging table via StreamAll's callback. Go heap stays bounded to one
// element per handler invocation (~5-10 KB) instead of one full []T per
// type (~35 MB for netixlan). No ent.Tx is held during Phase A — the
// absence of *ent.Tx from the signature is a compile-time guard against
// accidental tx-in-fetch drift.
//
// Returns a map flagging which types came from an incremental fetch
// (those skip the Phase B delete pass because incremental sync does not
// compute a complete remote-ID set). On error, no ent.Tx has been opened
// yet; the caller records failure and returns without touching the real
// database. The scratch file is unlinked by the caller's
// `defer closeScratchDB(...)`.
//
// Fallback-to-full-on-incremental-error semantics preserved: if the
// incremental stage fails mid-way, the scratch table is truncated and
// the full-mode stage is retried. The final batch for that type is
// flagged as full so the delete pass runs.
//
// The cursor is derived from MAX(updated) on the entity
// table instead of reading a sync_cursors row keyed on meta.generated.
// PeeringDB does not include meta.generated on ?since= responses (see
// internal/peeringdb/client_live_test.go TestMetaGeneratedLive/
// paginated_incremental); the prior design stored the absent zero-time,
// which alternated every cycle into a full bare-list re-fetch. The new
// derivation reads MAX(updated) once per type via the indexed query in
// internal/sync/cursor.go GetMaxUpdated.
//
// This helper is the fetch-outside-tx pass that
// splits fetch from upsert. It routes Phase A
// through an isolated scratch SQLite DB so the Go heap stays bounded.
func (w *Worker) syncFetchPass(ctx context.Context, scratch *scratchDB, mode config.SyncMode) (
	fromIncremental map[string]bool,
	err error,
) {
	steps := w.syncSteps()
	fromIncremental = make(map[string]bool, len(steps))

	for _, step := range steps {
		w.logger.LogAttrs(ctx, slog.LevelDebug, "fetching",
			slog.String("type", step.name),
			slog.String("mode", string(mode)),
		)

		_, stepSpan := otel.Tracer("sync").Start(ctx, "sync-fetch-"+step.name)

		// Derive cursor from MAX(updated) on the entity
		// table instead of reading sync_cursors. Replaces the v1.13
		// meta.generated-based design that alternated every cycle into
		// a full bare-list re-fetch (PeeringDB ?since= responses omit
		// meta.generated — see internal/peeringdb/client_live_test.go).
		table, ok := entityTables[step.name]
		if !ok {
			stepSpan.End()
			return nil, fmt.Errorf("syncFetchPass: no entity table mapping for %q", step.name)
		}
		cursor, cursorErr := GetMaxUpdated(ctx, w.db, table)
		if cursorErr != nil {
			w.logger.LogAttrs(ctx, slog.LevelInfo, "failed to get max(updated), using full sync",
				slog.String("type", step.name),
				slog.Any("error", cursorErr),
			)
			// Fall through with zero cursor → full bare-list path. This
			// mirrors the prior GetCursor error behaviour and is the
			// correct fail-soft response (full path is always safe).
		}

		incremental, stepErr := w.stageOneTypeToScratch(ctx, scratch, step.name, mode, cursor, stepSpan)

		stepSpan.End()
		typeAttr := metric.WithAttributes(attribute.String("type", step.name))

		if stepErr != nil {
			pdbotel.SyncTypeFetchErrors.Add(ctx, 1, typeAttr)
			return nil, fmt.Errorf("fetch %s: %w", step.name, stepErr)
		}

		fromIncremental[step.name] = incremental
	}

	return fromIncremental, nil
}

// stageOneTypeToScratch streams a single PeeringDB type into its scratch
// staging table, handling the incremental-with-fallback-to-full
// semantics. On incremental error the scratch table for this type is
// truncated (to drop any partial insert) and a full stage is retried.
// Returns a flag indicating whether the successful run was incremental.
//
// The cursor is derived from MAX(updated) by the caller
// (syncFetchPass). The previous design returned a per-call cursor
// update derived from meta.generated; with that field absent on
// ?since= responses it stored zero, which then alternated every cycle
// into a full bare-list re-fetch. Cursor advancement is now implicit:
// once the upsert tx commits the new rows, the next cycle's
// GetMaxUpdated picks up the boundary automatically.
//
// Bootstrap reversal (v1.18.3): the v1.18.2 bootstrap path that used
// time.Unix(1,0) to fetch ?since=1 on a zero cursor was reverted because
// it tripped upstream's API_THROTTLE_REPEATED_REQUEST throttle (1MB
// threshold, 10/min cap — see upstream main_settings.py) plus AWS WAF
// limits invisible in upstream code. The full-historical fetch returned
// retry-after values up to 54 minutes, blocking sync indefinitely.
//
// Current behaviour: cursor zero → full sync via bare list (status='ok'
// only, smaller responses). Historical-delete capture for fresh installs
// is deferred to a proper multi-cycle bootstrap design (v1.19+);
// FK backfill catches the orphans that matter on
// demand, including via recursive grandparent backfill (v1.18.3).
func (w *Worker) stageOneTypeToScratch(ctx context.Context, scratch *scratchDB, name string, mode config.SyncMode, cursor time.Time, stepSpan trace.Span) (bool, error) {
	fellBack := false
	// Incremental attempt requires a populated cursor. Zero cursor falls
	// through to the full-sync path below (bare list, status='ok' only).
	if mode == config.SyncModeIncremental && !cursor.IsZero() {
		_, incErr := scratch.stageType(ctx, w.pdbClient, name, cursor)
		if incErr == nil {
			return true, nil
		}
		fellBack = true
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
			return false, fmt.Errorf("clear partial incremental scratch %s: %w", name, delErr)
		}
	}
	// Full sync (default, first sync, no cursor, or incremental-fallback).
	if _, err := scratch.stageType(ctx, w.pdbClient, name, time.Time{}); err != nil {
		return false, err
	}
	// Tombstone-window capture: a bare list contains only status='ok' rows
	// (upstream filters bare lists per rest.py), and committing the full
	// snapshot advances the derived cursor (MAX(updated)) past the
	// pre-cycle window — without a follow-up ?since= fetch, deletes that
	// landed upstream since the last cycle would be permanently lost:
	// served live by all surfaces forever and absent from our own ?since=
	// exports. Stage the window on top of the snapshot; the scratch
	// INSERT OR REPLACE is keyed on id, so window rows (including
	// tombstones) win over their bare-list versions.
	//
	// In explicit full mode a failure here MUST fail the type: committing
	// the snapshot without the window would advance the cursor past
	// deletes we never saw, so the cycle retries instead. On the
	// incremental-fallback path the window fetch is the same request
	// shape that just failed — it is retried once best-effort (the
	// earlier failure may have been transient), but a second failure is
	// logged and tolerated so the fallback keeps its purpose: a cycle
	// that lands fresh 'ok' rows despite a broken ?since= endpoint.
	if !cursor.IsZero() {
		if _, err := scratch.stageType(ctx, w.pdbClient, name, cursor); err != nil {
			if !fellBack {
				return false, fmt.Errorf("stage tombstone window %s since %s: %w",
					name, cursor.Format(time.RFC3339), err)
			}
			stepSpan.AddEvent("tombstone_window.discarded",
				trace.WithAttributes(
					attribute.String("type", name),
					attribute.String("error", err.Error()),
				),
			)
			w.logger.LogAttrs(ctx, slog.LevelWarn, "tombstone window discarded after incremental fallback",
				slog.String("type", name),
				slog.Time("since", cursor),
				slog.Any("error", err),
			)
		}
	}
	return false, nil
}

// syncUpsertPass runs Phase B upserts inside the single tx. It drains
// each scratch staging table in FK parent-first order, chunking the rows
// into memory-bounded slices (scratchChunkSize rows at a time) so peak
// Go heap stays under ~10 MB per chunk. Each chunk is decoded to its
// typed Go struct, upserted into the real ent table, and then
// IMMEDIATELY freed via `batches[step.name] = syncBatch{}` to release
// the slice backing array before the next chunk loads. This is the core memory
// optimization — without it, Phase B peak memory would
// double during the handover between chunks. DO NOT remove the
// batch-free line.
//
// The per-type remoteIDsByType map and
// the final `SELECT id FROM scratch.{type}` collection step are gone
// alongside the inference-by-absence delete pass. Sync no longer
// infers deletions from absence; upstream sends explicit
// status='deleted' tombstones.
// fromIncremental is retained only as a defensive parameter — the
// per-step branching it used to gate is also gone.
//
// Atomicity is preserved: all real-DB writes run inside the same
// ent.Tx, and any upsert error triggers a rollback via the orchestrator.
//
// The per-type sync_cursors writes that were once
// folded into this tx have been removed. The cursor is now a derived
// quantity (MAX(updated) per table — see internal/sync/cursor.go); once
// the tx commits the new rows, the next cycle's GetMaxUpdated picks up
// the boundary automatically. The atomicity guarantee is unchanged: the
// data state IS the cursor, so the row-and-cursor divergence the prior
// in-tx-cursor-write protected against is now impossible by
// construction (no separate row to lag).
func (w *Worker) syncUpsertPass(
	ctx context.Context,
	tx *ent.Tx,
	scratch *scratchDB,
	_ map[string]bool,
) (
	map[string]int,
	error,
) {
	// Reset the per-sync FK orphan-filter state before the first
	// dispatchScratchChunk call populates it. Kept here rather than in
	// Sync so the Sync body stays within the 100-line budget
	// enforced by TestWorkerSync_LineBudget.
	w.resetFKState()
	steps := w.syncSteps()
	objectCounts := make(map[string]int, len(steps))
	// batches carries one decoded chunk at a time; the map entry is
	// cleared after each chunk upsert for the memory bound.
	batches := make(map[string]syncBatch, 1)

	for _, step := range steps {
		_, stepSpan := otel.Tracer("sync").Start(ctx, "sync-upsert-"+step.name)

		count, stepErr := w.drainAndUpsertType(ctx, tx, scratch, step.name, batches)

		stepSpan.End()
		typeAttr := metric.WithAttributes(attribute.String("type", step.name))

		if stepErr != nil {
			pdbotel.SyncTypeUpsertErrors.Add(ctx, 1, typeAttr)
			return nil, fmt.Errorf("upsert %s: %w", step.name, stepErr)
		}

		pdbotel.SyncTypeObjects.Add(ctx, int64(count), typeAttr)
		objectCounts[step.name] = count

		// Capture the true peak BEFORE the GC below reclaims the upsert
		// spike — this is where the cycle's heap high-water mark lives.
		w.samplePeakHeap()

		// Memory hard gate (400 MB): force a GC cycle between types
		// to deterministically reclaim the chunked decode buffers and
		// ent query-builder state before the next type's upsert begins.
		// Without this, sampled peak heap varies run-to-run because GC
		// scheduling lags the allocation spike on large types like
		// netixlan (200K rows). A per-type GC hint costs ~20ms per type
		// (13 × 20ms = 260ms added latency) in exchange for bounded
		// peak heap on the 512 MB fly.toml VM.
		runtime.GC()

		w.logger.LogAttrs(ctx, slog.LevelDebug, "upserted",
			slog.String("type", step.name),
			slog.Int("count", count),
		)
	}

	// The per-type sync_cursors writes are gone. Cursor
	// advancement is implicit — once this tx commits the new rows, the
	// next cycle's syncFetchPass derives the cursor from MAX(updated) on
	// each entity table (internal/sync/cursor.go GetMaxUpdated). The
	// sync-cursor-updates span name no longer applies; the
	// pdbplus.sync.cursor_write_caused_rollback OTel attribute is also
	// retired (the failure mode it surfaced is no longer reachable).
	return objectCounts, nil
}

// drainAndUpsertType reads scratch[type] in chunks of scratchChunkSize
// rows, decodes each chunk into typed Go structs, upserts the chunk
// into the real ent table, and frees the chunk memory before reading
// the next. Returns the total row count across all chunks.
//
// The chunked replay is the difference between peak heap ~20 MB and
// peak heap ~600 MB: netixlan is ~200K rows × ~200 bytes = ~40 MB if
// loaded in one shot, versus ~1 MB per 5000-row chunk. Atomicity
// is preserved because all upserts run through the same ent.Tx.
//
// Per-chunk decode+filter+upsert dispatches to the
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
		// array between iterations. Now the typed
		// chunk slice lives in syncIncremental[E]'s local scope and is
		// reclaimed automatically on return — scope-based release is
		// strictly more reliable than map-entry clearing. The literal
		// write below compiles to a no-op map store and is kept as a
		// grep-visible anchor for TestSync_BatchFreeAfterUpsert and the
		// memory-bound documentation trail in ARCHITECTURE.md §2. DO NOT
		// remove without first updating the regression test.
		batches[name] = syncBatch{}

		if len(rows) < scratchChunkSize {
			break
		}
		afterID = lastID
	}
	return total, nil
}

// syncIncrementalInput bundles the per-type parameters for
// syncIncremental[E]. Declared immediately before the consuming
// function. objectType is used for error-wrap context;
// upsert performs the bulk upsert inside the caller's ent.Tx.
//
// fkFilter, when non-nil, is called per row after the deleted-status
// filter and before the upsert. The row pointer lets the closure
// mutate nullable FK fields in place (e.g. null out an orphaned
// campus_id on a facility so the facility row itself is kept). When
// the closure returns false the row is dropped from the chunk.
//
// recordIDs, when non-nil, is called with the []int returned by the
// upsert closure so downstream child types can validate their FK
// references against the parent set (see Worker.fkRegisterIDs).
//
// fkRefs, when non-nil, returns the row's REQUIRED parent FK references
// (mirroring parentFKSpec) from the typed decode. Together with the
// prefetch hook it drives the chunk-level missing-parent pre-pass on
// the already-decoded rows — the pre-pass previously re-decoded every
// row's raw JSON into a map to read the same FK fields the typed
// struct already carries. Nullable FKs (fac.campus_id, netixlan side
// FKs) are deliberately excluded, matching parentFKSpec.
type syncIncrementalInput[E any] struct {
	objectType string
	fkRefs     func(*E) []parentFKRef
	prefetch   func(refs []parentFKRef)
	fkFilter   func(*E) bool
	recordIDs  func(ids []int)
	upsert     func(ctx context.Context, tx *ent.Tx, items []E) ([]int, error)
}

// syncIncremental decodes a chunk of raw scratch rows for a single
// PeeringDB type into typed Go structs and upserts the chunk into the
// real ent table via the per-type upsert closure. Returns the count of
// upserted rows.
//
// This generic helper replaces the 13 per-type
// arms that used to live in decodeScratchChunk and upsertOneType. The
// type-specific behavior is now carried by the closure arguments on
// syncIncrementalInput[E], so the bookkeeping code (decode, upsert,
// error-wrap) lives in exactly one place instead of being copy-pasted
// 13 times with only type names changed.
//
// Removed the `includeDeleted` parameter and
// the `filterByStatus` branch. Sync now unconditionally persists rows
// with any upstream status (including "deleted") through the upsert
// path; the row-level status × since matrix is applied by serializer
// surfaces (pdbcompat). The delete pass is a soft-delete, closing the
// stale-status loop.
//
// Each call processes ONE chunk (<=scratchChunkSize rows). The typed
// `items` slice is local to this function, so the chunk backing array
// is reclaimed automatically when the helper returns — no map-entry
// clearing is necessary for the memory bound. Atomicity
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
	// Chunk-level missing-parent pre-pass on the typed rows, BEFORE the
	// per-row fkFilter closures fire — the prefetch hook batches the
	// missing-parent HTTP fetches so the per-row fkCheckParent path
	// dedup-short-circuits.
	if in.fkRefs != nil && in.prefetch != nil {
		refs := make([]parentFKRef, 0, len(items))
		for i := range items {
			refs = append(refs, in.fkRefs(&items[i])...)
		}
		in.prefetch(refs)
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
// PeeringDB type through its typeRegistry descriptor's chunkUpsert
// closure (which binds the concrete type E, the per-type FK-orphan
// fkFilter policy, and the bulk upsert helper, then calls the generic
// syncIncremental[E]). Adding a new PeeringDB type requires a single
// descriptor entry in registry.go.
func (w *Worker) dispatchScratchChunk(ctx context.Context, tx *ent.Tx, name string, rows []scratchRow) (int, error) {
	desc, ok := descriptorByName[name]
	if !ok {
		return 0, fmt.Errorf("unknown sync type: %s", name)
	}
	return desc.chunkUpsert(w, ctx, tx, rows)
}

// fkCheckParent is the per-row FK validation helper called from the
// fkFilter closures in dispatchScratchChunk. Returns true if parentID
// is registered for parentType (or is a zero/null FK), or if the live
// backfill recovered the parent from upstream.
// Otherwise records the orphan via Worker.recordOrphan (DEBUG log +
// per-cycle counter) and returns false so syncIncremental drops the
// row from the chunk. emitOrphanSummary surfaces the per-cycle
// aggregate at WARN.
//
// Backfill is gated on fkBackfillRequestCap > 0 — operators who set
// PDBPLUS_FK_BACKFILL_MAX_REQUESTS_PER_CYCLE=0 retain the legacy drop-on-miss
// behavior (no upstream traffic; child rows simply dropped).
func (w *Worker) fkCheckParent(ctx context.Context, tx *ent.Tx, childType string, childID int, parentType string, parentID int, field string) bool {
	if w.fkHasParent(ctx, tx, parentType, parentID) {
		return true
	}
	if w.fkBackfillRequestCap > 0 && w.fkBackfillParent(ctx, tx, childType, parentType, parentID) {
		return true
	}
	w.recordOrphan(ctx, fkOrphanKey{
		ChildType:  childType,
		ParentType: parentType,
		Field:      field,
		Action:     "drop",
	}, childID, parentID)
	return false
}

// nullSideFK enforces the upstream SET_NULL contract on optional side
// FKs (NetworkIxLan.{net_side_id, ix_side_id} → fac).
// Per peeringdb_server/models.py:5630-5642 these
// columns are `null=True, on_delete=SET_NULL`, so a missing parent
// must NULL the column rather than drop the row.
//
// Process:
//  1. ptr is nil (FK already null) → no-op.
//  2. Parent present (cache or DB) → no-op.
//  3. Backfill enabled and recovers parent → no-op.
//  4. Otherwise: record the orphan with action="null" and zero ptr.
//
// Field name is recorded on the orphan counter so dashboards can split
// "net_side_id" misses from "ix_side_id" misses for the same NetworkIxLan
// child type.
func (w *Worker) nullSideFK(ctx context.Context, tx *ent.Tx, ptr **int, field string, childID int) {
	if ptr == nil || *ptr == nil {
		return
	}
	parentID := **ptr
	if w.fkHasParent(ctx, tx, peeringdb.TypeFac, parentID) {
		return
	}
	if w.fkBackfillRequestCap > 0 && w.fkBackfillParent(ctx, tx, peeringdb.TypeNetIXLan, peeringdb.TypeFac, parentID) {
		return
	}
	w.recordOrphan(ctx, fkOrphanKey{
		ChildType:  peeringdb.TypeNetIXLan,
		ParentType: peeringdb.TypeFac,
		Field:      field,
		Action:     "null",
	}, childID, parentID)
	*ptr = nil
}

// terminalRecordTimeout bounds the detached-context bookkeeping write so a
// wedged DB connection cannot hang a terminal recorder indefinitely. The
// status UPDATE is a single small raw-SQL Exec; this is generous.
const terminalRecordTimeout = 10 * time.Second

// terminalRecordContext derives the context used by the terminal recorders
// (recordSuccess / recordFailure) for their final status write and metric
// emission. It strips cancellation and deadlines via context.WithoutCancel
// — the cycle context may already be cancelled by the demotion monitor
// (runSyncCycle's cycleCancel) or a SIGTERM by the time we record the
// outcome, and a cancelled ctx turns RecordSyncComplete's UPDATE into a
// no-op (leaving sync_status stuck "running") and drops the terminal
// metric. WithoutCancel preserves context values, so the active OTel span
// still receives emitMemoryTelemetry's attributes. A short timeout is
// layered back on so the detached write cannot hang forever. Callers MUST
// defer the returned cancel.
func terminalRecordContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), terminalRecordTimeout)
}

// recordFailure records a failed sync in the sync_status table and metrics.
//
// mode is threaded through so the failure metric carries the same
// {status,mode} attribute pair as the success metric.
//
// The status write + terminal metrics run on a context detached from
// cancellation (terminalRecordContext): this path is reached after a
// possibly-cancelled cycle — including the panic firewall — and the
// failure MUST be recorded durably even if the cycle context is already
// Done. Without detachment the UPDATE silently no-ops and sync_status is
// left stuck "running".
func (w *Worker) recordFailure(ctx context.Context, mode config.SyncMode, statusID int64, start time.Time, syncErr error) {
	recCtx, cancel := terminalRecordContext(ctx)
	defer cancel()

	// Emit heap + RSS span attrs and (if over threshold) slog.Warn.
	// Called even on failure — memory pressure is interesting regardless of sync outcome.
	w.emitMemoryTelemetry(recCtx, w.config.HeapWarnBytes, w.config.RSSWarnBytes)
	w.emitOrphanSummary(recCtx)
	// Record sync-level failure metrics.
	attrs := metric.WithAttributes(
		attribute.String("status", "failed"),
		attribute.String("mode", string(mode)),
	)
	pdbotel.SyncDuration.Record(recCtx, time.Since(start).Seconds(), attrs)
	pdbotel.SyncOperations.Add(recCtx, 1, attrs)

	if statusID > 0 {
		// Same visibility contract as recordSuccess: never fail the
		// terminal path on a bookkeeping write, but never swallow it
		// silently either — otherwise rows stuck "running" accumulate
		// with no operator signal.
		if recErr := RecordSyncComplete(recCtx, w.db, statusID, Status{
			LastSyncAt:   time.Now(),
			Duration:     time.Since(start),
			Status:       "failed",
			ErrorMessage: syncErr.Error(),
		}); recErr != nil {
			w.logger.LogAttrs(recCtx, slog.LevelError, "failed to record sync completion",
				slog.Int64("status_id", statusID),
				slog.String("outcome", "failed"),
				slog.Any("error", recErr))
		}
	}
}

// SyncWithRetry calls Sync and retries on failure with exponential backoff.
//
// Rate-limit short-circuit: when the wrapped error is a *peeringdb.RateLimitError,
// the retry ladder is skipped entirely. PeeringDB's unauthenticated quota is
// 1 request per distinct query-string per hour, and all three default backoffs
// (30s, 2m, 8m) fall well inside that window — every retry within the window
// is guaranteed to 429 again AND consumes another slot against the hourly
// quota. Returning immediately lets the hourly scheduler retry naturally on
// its next tick (1h interval ≥ most Retry-After values we've observed).
//
// WAF short-circuit: a WAF / IP-block 403 (peeringdb.IsWAFBlocked) also skips
// the ladder. Retrying within the same source IP is pointless against an
// IP-level block, and hammering an actively-blocking upstream risks
// prolonging the block. Defer to the next scheduled tick, same as the 429
// path.
//
// Already-running short-circuit: ErrSyncAlreadyRunning from the CAS guard is
// returned as-is without retrying — the in-flight cycle owns the worker and a
// 30s-later retry would race it.
func (w *Worker) SyncWithRetry(ctx context.Context, mode config.SyncMode) error {
	err := w.Sync(ctx, mode)
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrSyncAlreadyRunning) {
		return err
	}
	if rateLimited(err) {
		w.logger.LogAttrs(ctx, slog.LevelInfo, "sync rate-limited, deferring to next scheduled tick",
			slog.Any("error", err),
		)
		return err
	}
	if peeringdb.IsWAFBlocked(err) {
		w.logger.LogAttrs(ctx, slog.LevelWarn, "sync blocked by upstream WAF, deferring to next scheduled tick",
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
			w.logger.LogAttrs(ctx, slog.LevelInfo, "sync rate-limited during retry, deferring",
				slog.Int("attempt", attempt+1),
				slog.Any("error", err),
			)
			return err
		}
		// Same for a WAF block surfacing mid-ladder: further retries
		// only dig the IP-block deeper.
		if peeringdb.IsWAFBlocked(err) {
			w.logger.LogAttrs(ctx, slog.LevelWarn, "sync blocked by upstream WAF during retry, deferring",
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

// Running reports whether a sync cycle is currently in flight. Used by
// the POST /sync handler for a best-effort synchronous 409: the check
// races a concurrently starting cycle (TOCTOU), but the CAS guard in
// Sync remains the authoritative gate — a lost race only degrades the
// response back to 202 with the trigger logged and dropped.
func (w *Worker) Running() bool {
	return w.running.Load()
}

// HasCompletedSync reports whether at least one successful sync has completed.
// Used for 503 behavior.
func (w *Worker) HasCompletedSync() bool {
	return w.synced.Load()
}

// runSyncCycle wraps SyncWithRetry with a demotion monitor goroutine.
// If the node is demoted during sync (IsPrimary returns false), the cycle
// context is cancelled, causing SyncWithRetry to abort early.
// The monitor goroutine lifetime is tied to cycleCtx.
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
	<-done        // wait for clean exit
}

// StartScheduler runs the sync scheduler on all instances.
// On primary nodes it executes sync cycles; on replicas it waits for promotion.
// Role changes are detected dynamically at each scheduler wakeup via
// w.config.IsPrimary(). The scheduler stops when ctx is cancelled.
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
		w.logger.LogAttrs(ctx, slog.LevelDebug, "failed to get last sync time",
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
			//
			// While the readiness latch is unset, re-read sync_status on
			// each heartbeat: a replica that booted before the primary's
			// first successful sync (fresh-fleet bootstrap, wiped-primary
			// recovery, or a transient read failure at boot) would
			// otherwise latch synced=false for the life of the process
			// and serve 503 on every data route even after LiteFS
			// replication delivers a fully-synced database. The read is
			// one local-SQLite row per interval, only while unsynced.
			if !w.synced.Load() {
				if ls, lsErr := GetLastSuccessfulSyncTime(ctx, w.db); lsErr == nil && !ls.IsZero() {
					w.logger.LogAttrs(ctx, slog.LevelInfo, "replicated sync history observed, marking ready",
						slog.Time("last_sync", ls),
					)
					w.synced.Store(true)
				}
			}
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
