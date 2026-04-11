package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
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
//  1. Phase A (NO TX HELD): HTTP fetch + JSON decode + filter happen
//     against in-memory batches. No database lock is held — PeeringDB I/O
//     and decode cost is ~30-60s of the sync.
//  2. Fetch Barrier: all 13 per-type batches resident in memory. Open the
//     real LiteFS tx now.
//  3. Phase B (SINGLE REAL TX): upsert each batch (freeing memory as each
//     type finishes) then delete stale rows. Commit. D-19 preserved: one
//     ent.Tx wraps every write.
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

	// === Phase A — NO TX HELD ===
	// HTTP + JSON decode + filter happen against in-memory batches only.
	batches, cursorUpdates, err := w.syncFetchPass(ctx, mode, start)
	if err != nil {
		w.recordFailure(ctx, statusID, start, err)
		return err
	}
	// === Fetch Barrier ===
	// All 13 batches resident in memory. Open the real LiteFS tx now.
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
	objectCounts, remoteIDsByType, err := w.syncUpsertPass(ctx, tx, batches)
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
// Phase A populates this; Phase B drains and nils each entry after the
// per-type upsert completes to release memory before the next batch begins.
//
// fromIncremental distinguishes incremental fetches (which skip the delete
// pass — deletes only apply to full syncs) from full fetches. generated
// carries the PeeringDB meta.generated timestamp for cursor advancement on
// incremental success.
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

	// fromIncremental is true when the batch came from an incremental fetch;
	// syncUpsertPass skips the delete pass for such batches (incremental
	// sync does not compute a full remote-ID set, only a delta).
	fromIncremental bool
	// generated is the PeeringDB meta.generated timestamp, used to advance
	// the per-type cursor on successful incremental commit.
	generated time.Time
}

// syncFetchPass runs Phase A: fetch all 13 types from PeeringDB, decode
// into typed slices, filter deleted rows, and return batches keyed by
// step.name. No ent.Tx is held during Phase A — HTTP and JSON decode
// happen against the live PeeringDB API with zero database locks.
//
// Returns the batches map and per-type cursor update timestamps. On error,
// no tx has been opened yet; the caller records failure and returns
// without touching the database. The absence of *ent.Tx from the
// signature is a compile-time guard against accidental tx-in-fetch drift.
//
// PERF-05 option (b): this helper is the fetch-outside-tx pass that splits
// fetch from upsert. Do NOT rewrite to decode-into-tx — the Commit D' scratch
// SQLite fallback depends on the fetch being materialised before the tx opens.
func (w *Worker) syncFetchPass(ctx context.Context, mode config.SyncMode, start time.Time) (
	batches map[string]syncBatch,
	cursorUpdates map[string]time.Time,
	err error,
) {
	steps := w.syncSteps()
	batches = make(map[string]syncBatch, len(steps))
	cursorUpdates = make(map[string]time.Time, len(steps))

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

		batch, cursorUpdate, stepErr := w.fetchOneType(ctx, step.name, mode, cursor, start, stepSpan)

		stepSpan.End()
		typeAttr := metric.WithAttributes(attribute.String("type", step.name))
		pdbotel.SyncTypeDuration.Record(ctx, time.Since(stepStart).Seconds(), typeAttr)

		if stepErr != nil {
			pdbotel.SyncTypeFetchErrors.Add(ctx, 1, typeAttr)
			return nil, nil, fmt.Errorf("fetch %s: %w", step.name, stepErr)
		}

		batches[step.name] = batch
		cursorUpdates[step.name] = cursorUpdate
	}

	return batches, cursorUpdates, nil
}

// fetchOneType fetches a single PeeringDB type, applies the per-type
// deleted-status filter (if configured), and returns a populated syncBatch
// with its cursor update timestamp. Handles incremental mode with
// fallback-to-full-for-this-type on incremental error (matching the
// pre-PERF-05 behavior that the refactor parity golden file locks).
//
// The type switch over step.name mirrors syncSteps() — adding a new
// PeeringDB type requires updating both this switch and the upsertOneType
// companion below. REFAC-04 (Plan 54-03) will collapse this into a generic
// helper; until then, keep the 13 arms explicit so the diff reads cleanly.
func (w *Worker) fetchOneType(ctx context.Context, name string, mode config.SyncMode, cursor time.Time, start time.Time, stepSpan trace.Span) (syncBatch, time.Time, error) {
	// Incremental attempt with fallback to full on error.
	if mode == config.SyncModeIncremental && !cursor.IsZero() {
		batch, generated, incErr := w.fetchOneTypeIncremental(ctx, name, cursor)
		if incErr == nil {
			batch.fromIncremental = true
			batch.generated = generated
			return batch, generated, nil
		}
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
	}
	// Full sync (default, first sync, no cursor, or incremental-fallback).
	batch, err := w.fetchOneTypeFull(ctx, name)
	if err != nil {
		return syncBatch{}, time.Time{}, err
	}
	return batch, start, nil
}

// fetchOneTypeFull runs the full-sync fetch path for a single type: pull
// the entire dataset via FetchType[T] (which now streams), then apply the
// deleted-status filter unless IncludeDeleted is set. Caller fills in
// fromIncremental = false (the zero value) so the delete pass runs.
func (w *Worker) fetchOneTypeFull(ctx context.Context, name string) (syncBatch, error) {
	switch name {
	case "org":
		items, err := peeringdb.FetchType[peeringdb.Organization](ctx, w.pdbClient, peeringdb.TypeOrg)
		if err != nil {
			return syncBatch{}, err
		}
		if !w.config.IncludeDeleted {
			items = filterOrgsByStatus(items)
		}
		return syncBatch{orgs: items}, nil
	case "campus":
		items, err := peeringdb.FetchType[peeringdb.Campus](ctx, w.pdbClient, peeringdb.TypeCampus)
		if err != nil {
			return syncBatch{}, err
		}
		if !w.config.IncludeDeleted {
			items = filterCampusesByStatus(items)
		}
		return syncBatch{campuses: items}, nil
	case "fac":
		items, err := peeringdb.FetchType[peeringdb.Facility](ctx, w.pdbClient, peeringdb.TypeFac)
		if err != nil {
			return syncBatch{}, err
		}
		if !w.config.IncludeDeleted {
			items = filterFacilitiesByStatus(items)
		}
		return syncBatch{facilities: items}, nil
	case "carrier":
		items, err := peeringdb.FetchType[peeringdb.Carrier](ctx, w.pdbClient, peeringdb.TypeCarrier)
		if err != nil {
			return syncBatch{}, err
		}
		if !w.config.IncludeDeleted {
			items = filterCarriersByStatus(items)
		}
		return syncBatch{carriers: items}, nil
	case "carrierfac":
		items, err := peeringdb.FetchType[peeringdb.CarrierFacility](ctx, w.pdbClient, peeringdb.TypeCarrierFac)
		if err != nil {
			return syncBatch{}, err
		}
		if !w.config.IncludeDeleted {
			items = filterCarrierFacilitiesByStatus(items)
		}
		return syncBatch{carrierFacs: items}, nil
	case "ix":
		items, err := peeringdb.FetchType[peeringdb.InternetExchange](ctx, w.pdbClient, peeringdb.TypeIX)
		if err != nil {
			return syncBatch{}, err
		}
		if !w.config.IncludeDeleted {
			items = filterInternetExchangesByStatus(items)
		}
		return syncBatch{ixes: items}, nil
	case "ixlan":
		items, err := peeringdb.FetchType[peeringdb.IxLan](ctx, w.pdbClient, peeringdb.TypeIXLan)
		if err != nil {
			return syncBatch{}, err
		}
		if !w.config.IncludeDeleted {
			items = filterIxLansByStatus(items)
		}
		return syncBatch{ixlans: items}, nil
	case "ixpfx":
		items, err := peeringdb.FetchType[peeringdb.IxPrefix](ctx, w.pdbClient, peeringdb.TypeIXPfx)
		if err != nil {
			return syncBatch{}, err
		}
		if !w.config.IncludeDeleted {
			items = filterIxPrefixesByStatus(items)
		}
		return syncBatch{ixpfxs: items}, nil
	case "ixfac":
		items, err := peeringdb.FetchType[peeringdb.IxFacility](ctx, w.pdbClient, peeringdb.TypeIXFac)
		if err != nil {
			return syncBatch{}, err
		}
		if !w.config.IncludeDeleted {
			items = filterIxFacilitiesByStatus(items)
		}
		return syncBatch{ixfacs: items}, nil
	case "net":
		items, err := peeringdb.FetchType[peeringdb.Network](ctx, w.pdbClient, peeringdb.TypeNet)
		if err != nil {
			return syncBatch{}, err
		}
		if !w.config.IncludeDeleted {
			items = filterNetworksByStatus(items)
		}
		return syncBatch{networks: items}, nil
	case "poc":
		items, err := peeringdb.FetchType[peeringdb.Poc](ctx, w.pdbClient, peeringdb.TypePoc)
		if err != nil {
			return syncBatch{}, err
		}
		if !w.config.IncludeDeleted {
			items = filterPocsByStatus(items)
		}
		return syncBatch{pocs: items}, nil
	case "netfac":
		items, err := peeringdb.FetchType[peeringdb.NetworkFacility](ctx, w.pdbClient, peeringdb.TypeNetFac)
		if err != nil {
			return syncBatch{}, err
		}
		if !w.config.IncludeDeleted {
			items = filterNetworkFacilitiesByStatus(items)
		}
		return syncBatch{netfacs: items}, nil
	case "netixlan":
		items, err := peeringdb.FetchType[peeringdb.NetworkIxLan](ctx, w.pdbClient, peeringdb.TypeNetIXLan)
		if err != nil {
			return syncBatch{}, err
		}
		if !w.config.IncludeDeleted {
			items = filterNetworkIxLansByStatus(items)
		}
		return syncBatch{netixlans: items}, nil
	}
	return syncBatch{}, fmt.Errorf("unknown sync type: %s", name)
}

// fetchOneTypeIncremental runs the incremental-sync fetch path for a
// single type. Uses fetchIncremental[T] + per-type status filter. Returns
// the populated batch and the meta.generated timestamp from the response.
func (w *Worker) fetchOneTypeIncremental(ctx context.Context, name string, cursor time.Time) (syncBatch, time.Time, error) {
	switch name {
	case "org":
		items, generated, err := fetchIncremental[peeringdb.Organization](ctx, w.pdbClient, peeringdb.TypeOrg, cursor)
		if err != nil {
			return syncBatch{}, time.Time{}, err
		}
		if !w.config.IncludeDeleted {
			items = filterOrgsByStatus(items)
		}
		return syncBatch{orgs: items}, generated, nil
	case "campus":
		items, generated, err := fetchIncremental[peeringdb.Campus](ctx, w.pdbClient, peeringdb.TypeCampus, cursor)
		if err != nil {
			return syncBatch{}, time.Time{}, err
		}
		if !w.config.IncludeDeleted {
			items = filterCampusesByStatus(items)
		}
		return syncBatch{campuses: items}, generated, nil
	case "fac":
		items, generated, err := fetchIncremental[peeringdb.Facility](ctx, w.pdbClient, peeringdb.TypeFac, cursor)
		if err != nil {
			return syncBatch{}, time.Time{}, err
		}
		if !w.config.IncludeDeleted {
			items = filterFacilitiesByStatus(items)
		}
		return syncBatch{facilities: items}, generated, nil
	case "carrier":
		items, generated, err := fetchIncremental[peeringdb.Carrier](ctx, w.pdbClient, peeringdb.TypeCarrier, cursor)
		if err != nil {
			return syncBatch{}, time.Time{}, err
		}
		if !w.config.IncludeDeleted {
			items = filterCarriersByStatus(items)
		}
		return syncBatch{carriers: items}, generated, nil
	case "carrierfac":
		items, generated, err := fetchIncremental[peeringdb.CarrierFacility](ctx, w.pdbClient, peeringdb.TypeCarrierFac, cursor)
		if err != nil {
			return syncBatch{}, time.Time{}, err
		}
		if !w.config.IncludeDeleted {
			items = filterCarrierFacilitiesByStatus(items)
		}
		return syncBatch{carrierFacs: items}, generated, nil
	case "ix":
		items, generated, err := fetchIncremental[peeringdb.InternetExchange](ctx, w.pdbClient, peeringdb.TypeIX, cursor)
		if err != nil {
			return syncBatch{}, time.Time{}, err
		}
		if !w.config.IncludeDeleted {
			items = filterInternetExchangesByStatus(items)
		}
		return syncBatch{ixes: items}, generated, nil
	case "ixlan":
		items, generated, err := fetchIncremental[peeringdb.IxLan](ctx, w.pdbClient, peeringdb.TypeIXLan, cursor)
		if err != nil {
			return syncBatch{}, time.Time{}, err
		}
		if !w.config.IncludeDeleted {
			items = filterIxLansByStatus(items)
		}
		return syncBatch{ixlans: items}, generated, nil
	case "ixpfx":
		items, generated, err := fetchIncremental[peeringdb.IxPrefix](ctx, w.pdbClient, peeringdb.TypeIXPfx, cursor)
		if err != nil {
			return syncBatch{}, time.Time{}, err
		}
		if !w.config.IncludeDeleted {
			items = filterIxPrefixesByStatus(items)
		}
		return syncBatch{ixpfxs: items}, generated, nil
	case "ixfac":
		items, generated, err := fetchIncremental[peeringdb.IxFacility](ctx, w.pdbClient, peeringdb.TypeIXFac, cursor)
		if err != nil {
			return syncBatch{}, time.Time{}, err
		}
		if !w.config.IncludeDeleted {
			items = filterIxFacilitiesByStatus(items)
		}
		return syncBatch{ixfacs: items}, generated, nil
	case "net":
		items, generated, err := fetchIncremental[peeringdb.Network](ctx, w.pdbClient, peeringdb.TypeNet, cursor)
		if err != nil {
			return syncBatch{}, time.Time{}, err
		}
		if !w.config.IncludeDeleted {
			items = filterNetworksByStatus(items)
		}
		return syncBatch{networks: items}, generated, nil
	case "poc":
		items, generated, err := fetchIncremental[peeringdb.Poc](ctx, w.pdbClient, peeringdb.TypePoc, cursor)
		if err != nil {
			return syncBatch{}, time.Time{}, err
		}
		if !w.config.IncludeDeleted {
			items = filterPocsByStatus(items)
		}
		return syncBatch{pocs: items}, generated, nil
	case "netfac":
		items, generated, err := fetchIncremental[peeringdb.NetworkFacility](ctx, w.pdbClient, peeringdb.TypeNetFac, cursor)
		if err != nil {
			return syncBatch{}, time.Time{}, err
		}
		if !w.config.IncludeDeleted {
			items = filterNetworkFacilitiesByStatus(items)
		}
		return syncBatch{netfacs: items}, generated, nil
	case "netixlan":
		items, generated, err := fetchIncremental[peeringdb.NetworkIxLan](ctx, w.pdbClient, peeringdb.TypeNetIXLan, cursor)
		if err != nil {
			return syncBatch{}, time.Time{}, err
		}
		if !w.config.IncludeDeleted {
			items = filterNetworkIxLansByStatus(items)
		}
		return syncBatch{netixlans: items}, generated, nil
	}
	return syncBatch{}, time.Time{}, fmt.Errorf("unknown sync type: %s", name)
}

// syncUpsertPass runs Phase B upserts inside the single tx. It drains the
// batches map in FK parent-first order, upserting each type and then
// IMMEDIATELY setting batches[step.name] = syncBatch{} to release the slice
// backing array so the next GC cycle can reclaim it. This is the core
// memory optimization for PERF-05 — without it, Phase B peak memory is
// ~295 MB; with it, peak drops to ~220 MB (ARCHITECTURE.md §2). DO NOT
// remove the batch-free line — it is the difference between "fits in 512
// MB VM" and "OOM".
//
// Returns per-type object counts and the remote-ID map for the later
// delete pass. Incremental batches are omitted from remoteIDsByType so
// the delete pass skips them (incremental sync does not compute a full
// remote-ID set). D-19 atomicity is preserved: all writes run inside the
// same ent.Tx.
func (w *Worker) syncUpsertPass(ctx context.Context, tx *ent.Tx, batches map[string]syncBatch) (
	objectCounts map[string]int,
	remoteIDsByType map[string][]int,
	err error,
) {
	steps := w.syncSteps()
	objectCounts = make(map[string]int, len(steps))
	remoteIDsByType = make(map[string][]int, len(steps))

	for _, step := range steps {
		batch, ok := batches[step.name]
		if !ok {
			continue
		}

		stepStart := time.Now()
		_, stepSpan := otel.Tracer("sync").Start(ctx, "sync-upsert-"+step.name)

		count, remoteIDs, stepErr := w.upsertOneType(ctx, tx, step.name, batch)

		stepSpan.End()
		typeAttr := metric.WithAttributes(attribute.String("type", step.name))
		pdbotel.SyncTypeDuration.Record(ctx, time.Since(stepStart).Seconds(), typeAttr)

		if stepErr != nil {
			pdbotel.SyncTypeUpsertErrors.Add(ctx, 1, typeAttr)
			return nil, nil, fmt.Errorf("upsert %s: %w", step.name, stepErr)
		}

		pdbotel.SyncTypeObjects.Add(ctx, int64(count), typeAttr)
		objectCounts[step.name] = count
		// Full-sync batches contribute a complete remote-ID set used by the
		// delete pass. Incremental batches are a delta only — skip delete.
		if !batch.fromIncremental {
			remoteIDsByType[step.name] = remoteIDs
		}

		// MANDATORY memory optimization: free the batch backing array now
		// that upsert succeeded for this type. Drops Phase B peak memory
		// from ~295 MB to ~220 MB (ARCHITECTURE.md §2). DO NOT remove this
		// line — it is the difference between "fits in 512 MB VM" and "OOM".
		batches[step.name] = syncBatch{}

		w.logger.LogAttrs(ctx, slog.LevelInfo, "upserted",
			slog.String("type", step.name),
			slog.Int("count", count),
		)
	}

	return objectCounts, remoteIDsByType, nil
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

// fetchIncremental is a generic helper that streams objects of the given
// type with a ?since= cursor and returns items plus the earliest
// meta.generated timestamp across all pages. It uses StreamAll directly
// (bypassing the FetchType wrapper) so the per-page meta is captured.
// Decoding happens inside the stream callback — the transient []byte
// buffer from json.Decoder is unmarshalled into a fresh T immediately
// and can be reused for the next element.
func fetchIncremental[T any](ctx context.Context, c *peeringdb.Client, objectType string, since time.Time) ([]T, time.Time, error) {
	var items []T
	handler := func(raw json.RawMessage) error {
		var v T
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("unmarshal %s item %d: %w", objectType, len(items), err)
		}
		items = append(items, v)
		return nil
	}
	meta, err := c.StreamAll(ctx, objectType, handler, peeringdb.WithSince(since))
	if err != nil {
		return nil, time.Time{}, err
	}

	generated := meta.Generated
	if generated.IsZero() {
		// Conservative cursor fallback when the server omits meta.generated:
		// rewind 5 minutes so the next sync doesn't miss objects modified
		// between the last successful fetch and clock drift.
		generated = time.Now().Add(-5 * time.Minute)
	}
	return items, generated, nil
}
