package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime"
	"runtime/debug"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/config"
	pdbotel "github.com/dotwaffle/peeringdb-plus/internal/otel"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// defaultRetryBackoffs defines the backoff durations for sync-level retries per D-21.
var defaultRetryBackoffs = []time.Duration{30 * time.Second, 2 * time.Minute, 8 * time.Minute}

// WorkerConfig holds configuration for the sync worker.
type WorkerConfig struct {
	IncludeDeleted bool
	IsPrimary      func() bool              // live primary detection; nil defaults to always-primary
	SyncMode       config.SyncMode
	OnSyncComplete func(counts map[string]int) // called after successful sync with per-type object counts
}

// Worker orchestrates PeeringDB data synchronization.
type Worker struct {
	pdbClient      *peeringdb.Client
	entClient      *ent.Client
	db             *sql.DB // underlying sql.DB for sync_status table
	config         WorkerConfig
	running        atomic.Bool
	synced         atomic.Bool    // true after first successful sync (D-30)
	logger         *slog.Logger
	retryBackoffs  []time.Duration // per D-21; defaults to 30s, 2m, 8m
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

// syncStep defines a single step in the sync process. Upserts run in
// parent-first FK order; deletes run in reverse (child-first) order.
//
// After PERF-05 (Plan 54-02 Commit D), the fetch and upsert paths dispatch
// via fetchOneTypeFull / fetchOneTypeIncremental / upsertOneType type
// switches on step.name — the old upsertFn/incrementalFn fields were
// removed because fetch now runs outside the tx. Only deleteFn survives
// because deletes still go through per-type methods inside the tx.
type syncStep struct {
	name     string
	deleteFn func(ctx context.Context, tx *ent.Tx, remoteIDs []int) (deleted int, err error)
}

// syncSteps returns the ordered list of sync steps in FK dependency order per D-06.
// Upserts are processed in this order (parents first); deletes in reverse (children first).
func (w *Worker) syncSteps() []syncStep {
	return []syncStep{
		{"org", deleteStaleOrganizations},
		{"campus", deleteStaleCampuses},
		{"fac", deleteStaleFacilities},
		{"carrier", deleteStaleCarriers},
		{"carrierfac", deleteStaleCarrierFacilities},
		{"ix", deleteStaleInternetExchanges},
		{"ixlan", deleteStaleIxLans},
		{"ixpfx", deleteStaleIxPrefixes},
		{"ixfac", deleteStaleIxFacilities},
		{"net", deleteStaleNetworks},
		{"poc", deleteStalePocs},
		{"netfac", deleteStaleNetworkFacilities},
		{"netixlan", deleteStaleNetworkIxLans},
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
// REFAC-03 line budget is <= 100 — enforced by TestWorkerSync_LineBudget.
func (w *Worker) Sync(ctx context.Context, mode config.SyncMode) error {
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
	// === Fetch Barrier ===
	// Scratch DB fully populated. Open the real LiteFS tx now.
	tx, err := w.entClient.Tx(ctx)
	if err != nil {
		w.recordFailure(ctx, statusID, start, fmt.Errorf("begin sync transaction: %w", err))
		return fmt.Errorf("begin sync transaction: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "PRAGMA defer_foreign_keys = ON"); err != nil {
		w.rollbackAndRecord(ctx, tx, statusID, start, fmt.Errorf("defer FK checks: %w", err))
		return fmt.Errorf("defer FK checks: %w", err)
	}

	// === Phase B — SINGLE REAL TX ===
	objectCounts, remoteIDsByType, err := w.syncUpsertPass(ctx, tx, scratch, fromIncremental)
	if err != nil {
		w.rollbackAndRecord(ctx, tx, statusID, start, err)
		return err
	}
	if err := w.syncDeletePass(ctx, tx, remoteIDsByType); err != nil {
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

// rollbackAndRecord rolls back the tx and records the failure in one place
// so Worker.Sync's error paths stay a one-liner each (REFAC-03 line budget).
// Logs the rollback error at ERROR level — a failing rollback inside a
// failing sync is worth surfacing in the error stream.
func (w *Worker) rollbackAndRecord(ctx context.Context, tx *ent.Tx, statusID int64, start time.Time, syncErr error) {
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
	if statusID > 0 {
		_ = RecordSyncComplete(ctx, w.db, statusID, Status{
			LastSyncAt:   time.Now(),
			Duration:     elapsed,
			ObjectCounts: objectCounts,
			Status:       "success",
		})
	}
	w.synced.Store(true)
	if w.config.OnSyncComplete != nil {
		w.config.OnSyncComplete(objectCounts)
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

// syncBatch holds the fetched + filtered data for a single PeeringDB type
// during Phase A of a sync. The concrete type varies per entity, so the
// struct has one optional slice per type; exactly one is non-nil, matching
// the step.name. A type switch in syncUpsertPass dispatches to the correct
// upsert function — the 13 entity types are closed-set and compile-time
// known, so a type switch is simpler and more reviewable than a generic
// interface.
//
// Phase B's drainAndUpsertType populates one syncBatch per chunk drained
// from the scratch DB, calls upsertOneType, then resets the map entry to
// the zero value before draining the next chunk. This bounds peak Go
// heap to one scratchChunkSize-worth of decoded rows per type.
//
// fromIncremental distinction is tracked in a separate map on the fetch
// pass result (not a field here) so the syncBatch remains purely a
// memory-bound replay buffer.
type syncBatch struct {
	// Exactly one of these is non-nil, matching the step.name.
	orgs        []peeringdb.Organization
	campuses    []peeringdb.Campus
	facilities  []peeringdb.Facility
	carriers    []peeringdb.Carrier
	carrierFacs []peeringdb.CarrierFacility
	ixes        []peeringdb.InternetExchange
	ixlans      []peeringdb.IxLan
	ixpfxs      []peeringdb.IxPrefix
	ixfacs      []peeringdb.IxFacility
	networks    []peeringdb.Network
	pocs        []peeringdb.Poc
	netfacs     []peeringdb.NetworkFacility
	netixlans   []peeringdb.NetworkIxLan
}

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

		stepStart := time.Now()
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
		pdbotel.SyncTypeDuration.Record(ctx, time.Since(stepStart).Seconds(), typeAttr)

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
// typed Go struct, filtered by deleted-status (unless IncludeDeleted is
// set), upserted into the real ent table, and then IMMEDIATELY freed
// via `batches[step.name] = syncBatch{}` to release the slice backing
// array before the next chunk loads. This is the core memory
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
	steps := w.syncSteps()
	objectCounts = make(map[string]int, len(steps))
	remoteIDsByType = make(map[string][]int, len(steps))
	// batches carries one decoded chunk at a time; the map entry is
	// cleared after each chunk upsert for the PERF-05 memory bound.
	batches := make(map[string]syncBatch, 1)

	for _, step := range steps {
		stepStart := time.Now()
		_, stepSpan := otel.Tracer("sync").Start(ctx, "sync-upsert-"+step.name)

		count, stepErr := w.drainAndUpsertType(ctx, tx, scratch, step.name, batches)

		stepSpan.End()
		typeAttr := metric.WithAttributes(attribute.String("type", step.name))
		pdbotel.SyncTypeDuration.Record(ctx, time.Since(stepStart).Seconds(), typeAttr)

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
		if !fromIncremental[step.name] {
			ids, idErr := w.collectScratchIDs(ctx, scratch, step.name)
			if idErr != nil {
				return nil, nil, fmt.Errorf("collect remote ids %s: %w", step.name, idErr)
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
// rows, decodes each chunk into typed Go structs, filters deleted rows
// (unless IncludeDeleted is set), upserts the chunk into the real ent
// table via upsertOneType, and frees the chunk memory before reading
// the next. Returns the total row count across all chunks.
//
// The chunked replay is the difference between peak heap ~20 MB and
// peak heap ~600 MB: netixlan is ~200K rows × ~200 bytes = ~40 MB if
// loaded in one shot, versus ~1 MB per 5000-row chunk. D-19 atomicity
// is preserved because all upserts run through the same ent.Tx.
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

		batch, decErr := decodeScratchChunk(name, rows, w.config.IncludeDeleted)
		if decErr != nil {
			return total, decErr
		}

		count, _, upErr := w.upsertOneType(ctx, tx, name, batch)
		if upErr != nil {
			return total, upErr
		}
		total += count

		// MANDATORY memory optimization: free the chunk backing array
		// before the next drainChunk call. Drops Phase B peak memory
		// from ~O(type_size) to ~O(chunk_size). DO NOT remove — this
		// is the PERF-05 batch-free line documented in ARCHITECTURE.md
		// §2 and regression-locked by TestSync_BatchFreeAfterUpsert.
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

// decodeScratchChunk decodes a chunk of raw JSON scratch rows for the
// given PeeringDB type into a typed syncBatch, applying the deleted-status
// filter unless includeDeleted is true. The type switch mirrors
// upsertOneType's layout — adding a new PeeringDB type requires updating
// both in lockstep.
func decodeScratchChunk(name string, rows []scratchRow, includeDeleted bool) (syncBatch, error) {
	switch name {
	case "org":
		items := make([]peeringdb.Organization, 0, len(rows))
		for _, r := range rows {
			var v peeringdb.Organization
			if err := json.Unmarshal(r.raw, &v); err != nil {
				return syncBatch{}, fmt.Errorf("decode %s id=%d: %w", name, r.id, err)
			}
			items = append(items, v)
		}
		if !includeDeleted {
			items = filterOrgsByStatus(items)
		}
		return syncBatch{orgs: items}, nil
	case "campus":
		items := make([]peeringdb.Campus, 0, len(rows))
		for _, r := range rows {
			var v peeringdb.Campus
			if err := json.Unmarshal(r.raw, &v); err != nil {
				return syncBatch{}, fmt.Errorf("decode %s id=%d: %w", name, r.id, err)
			}
			items = append(items, v)
		}
		if !includeDeleted {
			items = filterCampusesByStatus(items)
		}
		return syncBatch{campuses: items}, nil
	case "fac":
		items := make([]peeringdb.Facility, 0, len(rows))
		for _, r := range rows {
			var v peeringdb.Facility
			if err := json.Unmarshal(r.raw, &v); err != nil {
				return syncBatch{}, fmt.Errorf("decode %s id=%d: %w", name, r.id, err)
			}
			items = append(items, v)
		}
		if !includeDeleted {
			items = filterFacilitiesByStatus(items)
		}
		return syncBatch{facilities: items}, nil
	case "carrier":
		items := make([]peeringdb.Carrier, 0, len(rows))
		for _, r := range rows {
			var v peeringdb.Carrier
			if err := json.Unmarshal(r.raw, &v); err != nil {
				return syncBatch{}, fmt.Errorf("decode %s id=%d: %w", name, r.id, err)
			}
			items = append(items, v)
		}
		if !includeDeleted {
			items = filterCarriersByStatus(items)
		}
		return syncBatch{carriers: items}, nil
	case "carrierfac":
		items := make([]peeringdb.CarrierFacility, 0, len(rows))
		for _, r := range rows {
			var v peeringdb.CarrierFacility
			if err := json.Unmarshal(r.raw, &v); err != nil {
				return syncBatch{}, fmt.Errorf("decode %s id=%d: %w", name, r.id, err)
			}
			items = append(items, v)
		}
		if !includeDeleted {
			items = filterCarrierFacilitiesByStatus(items)
		}
		return syncBatch{carrierFacs: items}, nil
	case "ix":
		items := make([]peeringdb.InternetExchange, 0, len(rows))
		for _, r := range rows {
			var v peeringdb.InternetExchange
			if err := json.Unmarshal(r.raw, &v); err != nil {
				return syncBatch{}, fmt.Errorf("decode %s id=%d: %w", name, r.id, err)
			}
			items = append(items, v)
		}
		if !includeDeleted {
			items = filterInternetExchangesByStatus(items)
		}
		return syncBatch{ixes: items}, nil
	case "ixlan":
		items := make([]peeringdb.IxLan, 0, len(rows))
		for _, r := range rows {
			var v peeringdb.IxLan
			if err := json.Unmarshal(r.raw, &v); err != nil {
				return syncBatch{}, fmt.Errorf("decode %s id=%d: %w", name, r.id, err)
			}
			items = append(items, v)
		}
		if !includeDeleted {
			items = filterIxLansByStatus(items)
		}
		return syncBatch{ixlans: items}, nil
	case "ixpfx":
		items := make([]peeringdb.IxPrefix, 0, len(rows))
		for _, r := range rows {
			var v peeringdb.IxPrefix
			if err := json.Unmarshal(r.raw, &v); err != nil {
				return syncBatch{}, fmt.Errorf("decode %s id=%d: %w", name, r.id, err)
			}
			items = append(items, v)
		}
		if !includeDeleted {
			items = filterIxPrefixesByStatus(items)
		}
		return syncBatch{ixpfxs: items}, nil
	case "ixfac":
		items := make([]peeringdb.IxFacility, 0, len(rows))
		for _, r := range rows {
			var v peeringdb.IxFacility
			if err := json.Unmarshal(r.raw, &v); err != nil {
				return syncBatch{}, fmt.Errorf("decode %s id=%d: %w", name, r.id, err)
			}
			items = append(items, v)
		}
		if !includeDeleted {
			items = filterIxFacilitiesByStatus(items)
		}
		return syncBatch{ixfacs: items}, nil
	case "net":
		items := make([]peeringdb.Network, 0, len(rows))
		for _, r := range rows {
			var v peeringdb.Network
			if err := json.Unmarshal(r.raw, &v); err != nil {
				return syncBatch{}, fmt.Errorf("decode %s id=%d: %w", name, r.id, err)
			}
			items = append(items, v)
		}
		if !includeDeleted {
			items = filterNetworksByStatus(items)
		}
		return syncBatch{networks: items}, nil
	case "poc":
		items := make([]peeringdb.Poc, 0, len(rows))
		for _, r := range rows {
			var v peeringdb.Poc
			if err := json.Unmarshal(r.raw, &v); err != nil {
				return syncBatch{}, fmt.Errorf("decode %s id=%d: %w", name, r.id, err)
			}
			items = append(items, v)
		}
		if !includeDeleted {
			items = filterPocsByStatus(items)
		}
		return syncBatch{pocs: items}, nil
	case "netfac":
		items := make([]peeringdb.NetworkFacility, 0, len(rows))
		for _, r := range rows {
			var v peeringdb.NetworkFacility
			if err := json.Unmarshal(r.raw, &v); err != nil {
				return syncBatch{}, fmt.Errorf("decode %s id=%d: %w", name, r.id, err)
			}
			items = append(items, v)
		}
		if !includeDeleted {
			items = filterNetworkFacilitiesByStatus(items)
		}
		return syncBatch{netfacs: items}, nil
	case "netixlan":
		items := make([]peeringdb.NetworkIxLan, 0, len(rows))
		for _, r := range rows {
			var v peeringdb.NetworkIxLan
			if err := json.Unmarshal(r.raw, &v); err != nil {
				return syncBatch{}, fmt.Errorf("decode %s id=%d: %w", name, r.id, err)
			}
			items = append(items, v)
		}
		if !includeDeleted {
			items = filterNetworkIxLansByStatus(items)
		}
		return syncBatch{netixlans: items}, nil
	}
	return syncBatch{}, fmt.Errorf("unknown sync type: %s", name)
}

// upsertOneType dispatches the batch to the correct per-type upsert
// function. Mirrors syncSteps()'s closed-set layout and fetchOneTypeFull.
// Returns (count, remoteIDs, err). Adding a new PeeringDB type requires
// updating this switch in lockstep with fetchOneTypeFull / fetchOneTypeIncremental.
func (w *Worker) upsertOneType(ctx context.Context, tx *ent.Tx, name string, batch syncBatch) (int, []int, error) {
	switch name {
	case "org":
		ids, err := upsertOrganizations(ctx, tx, batch.orgs)
		if err != nil {
			return 0, nil, err
		}
		return len(batch.orgs), ids, nil
	case "campus":
		ids, err := upsertCampuses(ctx, tx, batch.campuses)
		if err != nil {
			return 0, nil, err
		}
		return len(batch.campuses), ids, nil
	case "fac":
		ids, err := upsertFacilities(ctx, tx, batch.facilities)
		if err != nil {
			return 0, nil, err
		}
		return len(batch.facilities), ids, nil
	case "carrier":
		ids, err := upsertCarriers(ctx, tx, batch.carriers)
		if err != nil {
			return 0, nil, err
		}
		return len(batch.carriers), ids, nil
	case "carrierfac":
		ids, err := upsertCarrierFacilities(ctx, tx, batch.carrierFacs)
		if err != nil {
			return 0, nil, err
		}
		return len(batch.carrierFacs), ids, nil
	case "ix":
		ids, err := upsertInternetExchanges(ctx, tx, batch.ixes)
		if err != nil {
			return 0, nil, err
		}
		return len(batch.ixes), ids, nil
	case "ixlan":
		ids, err := upsertIxLans(ctx, tx, batch.ixlans)
		if err != nil {
			return 0, nil, err
		}
		return len(batch.ixlans), ids, nil
	case "ixpfx":
		ids, err := upsertIxPrefixes(ctx, tx, batch.ixpfxs)
		if err != nil {
			return 0, nil, err
		}
		return len(batch.ixpfxs), ids, nil
	case "ixfac":
		ids, err := upsertIxFacilities(ctx, tx, batch.ixfacs)
		if err != nil {
			return 0, nil, err
		}
		return len(batch.ixfacs), ids, nil
	case "net":
		ids, err := upsertNetworks(ctx, tx, batch.networks)
		if err != nil {
			return 0, nil, err
		}
		return len(batch.networks), ids, nil
	case "poc":
		ids, err := upsertPocs(ctx, tx, batch.pocs)
		if err != nil {
			return 0, nil, err
		}
		return len(batch.pocs), ids, nil
	case "netfac":
		ids, err := upsertNetworkFacilities(ctx, tx, batch.netfacs)
		if err != nil {
			return 0, nil, err
		}
		return len(batch.netfacs), ids, nil
	case "netixlan":
		ids, err := upsertNetworkIxLans(ctx, tx, batch.netixlans)
		if err != nil {
			return 0, nil, err
		}
		return len(batch.netixlans), ids, nil
	}
	return 0, nil, fmt.Errorf("unknown sync type: %s", name)
}

// syncDeletePass runs the per-type delete loop in child-first (reverse FK)
// order, skipping types that have no remoteIDs (incremental sync succeeded).
// The orchestrator handles rollback + recordFailure on error.
func (w *Worker) syncDeletePass(ctx context.Context, tx *ent.Tx, remoteIDsByType map[string][]int) error {
	steps := w.syncSteps()
	for i := len(steps) - 1; i >= 0; i-- {
		step := steps[i]
		remoteIDs, ok := remoteIDsByType[step.name]
		if !ok {
			// Incremental sync succeeded for this type — no delete needed.
			continue
		}

		stepStart := time.Now()
		_, stepSpan := otel.Tracer("sync").Start(ctx, "sync-delete-"+step.name)

		deleted, stepErr := step.deleteFn(ctx, tx, remoteIDs)

		stepSpan.End()

		typeAttr := metric.WithAttributes(attribute.String("type", step.name))

		if stepErr != nil {
			pdbotel.SyncTypeDuration.Record(ctx, time.Since(stepStart).Seconds(), typeAttr)
			return fmt.Errorf("delete stale %s: %w", step.name, stepErr)
		}

		// Record per-type delete metrics per D-08.
		pdbotel.SyncTypeDeleted.Add(ctx, int64(deleted), typeAttr)

		if deleted > 0 {
			w.logger.LogAttrs(ctx, slog.LevelInfo, "deleted stale",
				slog.String("type", step.name),
				slog.Int("deleted", deleted),
			)
		}
	}
	return nil
}

// recordFailure records a failed sync in the sync_status table and metrics.
func (w *Worker) recordFailure(ctx context.Context, statusID int64, start time.Time, syncErr error) {
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
func (w *Worker) SyncWithRetry(ctx context.Context, mode config.SyncMode) error {
	err := w.Sync(ctx, mode)
	if err == nil {
		return nil
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
	}

	w.logger.LogAttrs(ctx, slog.LevelError, "sync failed after all retries",
		slog.Int("retries", maxRetries),
		slog.Any("error", err),
	)
	return fmt.Errorf("sync failed after %d retries: %w", maxRetries, lastErr)
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
// Role changes are detected dynamically each tick via w.config.IsPrimary().
// The scheduler stops when ctx is cancelled per CC-2.
func (w *Worker) StartScheduler(ctx context.Context, interval time.Duration) {
	mode := w.config.SyncMode
	if mode == "" {
		mode = config.SyncModeFull
	}

	wasPrimary := false

	// Initial check: determine role and act accordingly.
	if w.config.IsPrimary() {
		wasPrimary = true
		lastSync, err := GetLastSuccessfulSyncTime(ctx, w.db)
		if err != nil {
			w.logger.LogAttrs(ctx, slog.LevelWarn, "failed to get last sync time",
				slog.Any("error", err),
			)
		}
		if lastSync.IsZero() {
			// No prior successful sync -- database is empty, do a full sync now.
			w.runSyncCycle(ctx, config.SyncModeFull)
		} else {
			// Data exists from a prior sync -- serve requests immediately.
			w.synced.Store(true)
			if time.Since(lastSync) >= interval {
				w.runSyncCycle(ctx, mode)
			}
		}
	} else {
		// Replica: check if data exists from replication.
		lastSync, _ := GetLastSuccessfulSyncTime(ctx, w.db)
		if !lastSync.IsZero() {
			w.synced.Store(true)
		}
		w.logger.LogAttrs(ctx, slog.LevelDebug, "starting scheduler as replica")
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			isPrimary := w.config.IsPrimary()

			// Detect role transitions.
			if isPrimary && !wasPrimary {
				w.logger.LogAttrs(ctx, slog.LevelInfo, "promoted to primary, checking sync status")
				pdbotel.RoleTransitions.Add(ctx, 1,
					metric.WithAttributes(attribute.String("direction", "promoted")),
				)
				lastSync, _ := GetLastSuccessfulSyncTime(ctx, w.db)
				wasPrimary = true
				if lastSync.IsZero() || time.Since(lastSync) >= interval {
					w.runSyncCycle(ctx, mode)
				}
				continue
			}
			if !isPrimary && wasPrimary {
				w.logger.LogAttrs(ctx, slog.LevelInfo, "demoted to replica")
				pdbotel.RoleTransitions.Add(ctx, 1,
					metric.WithAttributes(attribute.String("direction", "demoted")),
				)
				wasPrimary = false
				continue
			}

			wasPrimary = isPrimary

			if !isPrimary {
				w.logger.LogAttrs(ctx, slog.LevelDebug, "not primary, skipping sync")
				continue
			}

			w.runSyncCycle(ctx, mode)
		}
	}
}

