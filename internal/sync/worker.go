package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
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
	IsPrimary      func() bool // live primary detection; nil defaults to always-primary
	SyncMode       config.SyncMode
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
type syncStep struct {
	name          string
	upsertFn      func(ctx context.Context, tx *ent.Tx) (count int, remoteIDs []int, err error)
	deleteFn      func(ctx context.Context, tx *ent.Tx, remoteIDs []int) (deleted int, err error)
	incrementalFn func(ctx context.Context, tx *ent.Tx, since time.Time) (count int, generated time.Time, err error)
}

// syncSteps returns the ordered list of sync steps in FK dependency order per D-06.
// Upserts are processed in this order (parents first); deletes in reverse (children first).
func (w *Worker) syncSteps() []syncStep {
	return []syncStep{
		{"org", w.fetchAndUpsertOrganizations, deleteStaleOrganizations, w.syncOrganizationsIncremental},
		{"campus", w.fetchAndUpsertCampuses, deleteStaleCampuses, w.syncCampusesIncremental},
		{"fac", w.fetchAndUpsertFacilities, deleteStaleFacilities, w.syncFacilitiesIncremental},
		{"carrier", w.fetchAndUpsertCarriers, deleteStaleCarriers, w.syncCarriersIncremental},
		{"carrierfac", w.fetchAndUpsertCarrierFacilities, deleteStaleCarrierFacilities, w.syncCarrierFacilitiesIncremental},
		{"ix", w.fetchAndUpsertInternetExchanges, deleteStaleInternetExchanges, w.syncInternetExchangesIncremental},
		{"ixlan", w.fetchAndUpsertIxLans, deleteStaleIxLans, w.syncIxLansIncremental},
		{"ixpfx", w.fetchAndUpsertIxPrefixes, deleteStaleIxPrefixes, w.syncIxPrefixesIncremental},
		{"ixfac", w.fetchAndUpsertIxFacilities, deleteStaleIxFacilities, w.syncIxFacilitiesIncremental},
		{"net", w.fetchAndUpsertNetworks, deleteStaleNetworks, w.syncNetworksIncremental},
		{"poc", w.fetchAndUpsertPocs, deleteStalePocs, w.syncPocsIncremental},
		{"netfac", w.fetchAndUpsertNetworkFacilities, deleteStaleNetworkFacilities, w.syncNetworkFacilitiesIncremental},
		{"netixlan", w.fetchAndUpsertNetworkIxLans, deleteStaleNetworkIxLans, w.syncNetworkIxLansIncremental},
	}
}

// Sync executes a synchronization from PeeringDB to the local database.
// The mode parameter controls whether a full or incremental sync is performed.
// It acquires a mutex to prevent concurrent runs and wraps all changes in
// a single database transaction per D-19.
func (w *Worker) Sync(ctx context.Context, mode config.SyncMode) error {
	// Mutex per D-24: if already running, skip.
	if !w.running.CompareAndSwap(false, true) {
		w.logger.Warn("sync already running, skipping")
		return nil
	}
	defer w.running.Store(false)

	ctx, span := otel.Tracer("sync").Start(ctx, "sync-"+string(mode))
	defer span.End()

	start := time.Now()

	// Record sync start in sync_status table per D-26.
	statusID, err := RecordSyncStart(ctx, w.db, start)
	if err != nil {
		w.logger.LogAttrs(ctx, slog.LevelError, "failed to record sync start",
			slog.String("error", err.Error()),
		)
		// Non-fatal: continue with sync.
	}

	// Disable FK constraints for bulk sync. PeeringDB data contains dangling
	// references (e.g., facilities referencing deleted campuses).
	if _, err := w.db.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		w.recordFailure(ctx, statusID, start, fmt.Errorf("disable FK checks: %w", err))
		return fmt.Errorf("disable FK checks: %w", err)
	}
	defer w.db.ExecContext(ctx, "PRAGMA foreign_keys = ON") //nolint:errcheck // best-effort re-enable

	// Begin transaction per D-19.
	tx, err := w.entClient.Tx(ctx)
	if err != nil {
		w.recordFailure(ctx, statusID, start, fmt.Errorf("begin sync transaction: %w", err))
		return fmt.Errorf("begin sync transaction: %w", err)
	}

	objectCounts := make(map[string]int)
	totalCount := 0
	cursorUpdates := make(map[string]time.Time) // type -> generated timestamp

	steps := w.syncSteps()

	// remoteIDsByType collects IDs from the upsert pass for the delete pass.
	remoteIDsByType := make(map[string][]int, len(steps))

	// Pass 1: Fetch and upsert in parent-first FK order.
	for _, step := range steps {
		w.logger.LogAttrs(ctx, slog.LevelInfo, "syncing",
			slog.String("type", step.name),
			slog.String("mode", string(mode)),
		)

		stepStart := time.Now()
		_, stepSpan := otel.Tracer("sync").Start(ctx, "sync-upsert-"+step.name)

		var count int
		var stepErr error

		cursor, cursorErr := GetCursor(ctx, w.db, step.name)
		if cursorErr != nil {
			w.logger.LogAttrs(ctx, slog.LevelWarn, "failed to get cursor, using full sync",
				slog.String("type", step.name),
				slog.String("error", cursorErr.Error()),
			)
		}

		if mode == config.SyncModeIncremental && !cursor.IsZero() {
			// Incremental: try with since cursor (no deletes).
			var generated time.Time
			count, generated, stepErr = step.incrementalFn(ctx, tx, cursor)
			if stepErr != nil {
				// Fallback to full for this type.
				typeAttr := metric.WithAttributes(attribute.String("type", step.name))
				pdbotel.SyncTypeFallback.Add(ctx, 1, typeAttr)
				stepSpan.AddEvent("incremental.fallback",
					trace.WithAttributes(
						attribute.String("type", step.name),
						attribute.String("error", stepErr.Error()),
					),
				)
				w.logger.LogAttrs(ctx, slog.LevelWarn, "incremental sync failed, falling back to full",
					slog.String("type", step.name),
					slog.String("error", stepErr.Error()),
				)
				var remoteIDs []int
				count, remoteIDs, stepErr = step.upsertFn(ctx, tx)
				if stepErr == nil {
					remoteIDsByType[step.name] = remoteIDs
					cursorUpdates[step.name] = start
				}
			} else {
				cursorUpdates[step.name] = generated
			}
		} else {
			// Full sync (default, first sync, or no cursor).
			var remoteIDs []int
			count, remoteIDs, stepErr = step.upsertFn(ctx, tx)
			if stepErr == nil {
				remoteIDsByType[step.name] = remoteIDs
				cursorUpdates[step.name] = start
			}
		}

		stepSpan.End()

		typeAttr := metric.WithAttributes(attribute.String("type", step.name))

		if stepErr != nil {
			// Record per-type error metric per D-10.
			if strings.HasPrefix(stepErr.Error(), "fetch ") {
				pdbotel.SyncTypeFetchErrors.Add(ctx, 1, typeAttr)
			} else {
				pdbotel.SyncTypeUpsertErrors.Add(ctx, 1, typeAttr)
			}

			// Record per-type duration even on failure.
			pdbotel.SyncTypeDuration.Record(ctx, time.Since(stepStart).Seconds(), typeAttr)

			// Rollback transaction per D-21.
			if rbErr := tx.Rollback(); rbErr != nil {
				w.logger.LogAttrs(ctx, slog.LevelError, "rollback failed",
					slog.String("error", rbErr.Error()),
				)
			}
			syncErr := fmt.Errorf("sync %s: %w", step.name, stepErr)
			w.recordFailure(ctx, statusID, start, syncErr)
			return syncErr
		}

		// Record per-type upsert metrics per D-07.
		pdbotel.SyncTypeDuration.Record(ctx, time.Since(stepStart).Seconds(), typeAttr)
		pdbotel.SyncTypeObjects.Add(ctx, int64(count), typeAttr)

		objectCounts[step.name] = count
		totalCount += count

		w.logger.LogAttrs(ctx, slog.LevelInfo, "upserted",
			slog.String("type", step.name),
			slog.Int("count", count),
		)
	}

	// Pass 2: Delete stale records in child-first (reverse FK) order.
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

			if rbErr := tx.Rollback(); rbErr != nil {
				w.logger.LogAttrs(ctx, slog.LevelError, "rollback failed",
					slog.String("error", rbErr.Error()),
				)
			}
			syncErr := fmt.Errorf("delete stale %s: %w", step.name, stepErr)
			w.recordFailure(ctx, statusID, start, syncErr)
			return syncErr
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

	// Commit transaction.
	if err := tx.Commit(); err != nil {
		syncErr := fmt.Errorf("commit sync transaction: %w", err)
		w.recordFailure(ctx, statusID, start, syncErr)
		return syncErr
	}

	// Update per-type cursors AFTER successful commit.
	for typeName, generated := range cursorUpdates {
		if err := UpsertCursor(ctx, w.db, typeName, generated, "success"); err != nil {
			w.logger.LogAttrs(ctx, slog.LevelError, "failed to update cursor",
				slog.String("type", typeName),
				slog.String("error", err.Error()),
			)
		}
	}

	elapsed := time.Since(start)

	// Record sync-level metrics per D-06.
	statusAttr := metric.WithAttributes(attribute.String("status", "success"))
	pdbotel.SyncDuration.Record(ctx, elapsed.Seconds(), statusAttr)
	pdbotel.SyncOperations.Add(ctx, 1, statusAttr)

	w.logger.LogAttrs(ctx, slog.LevelInfo, "sync complete",
		slog.String("mode", string(mode)),
		slog.Duration("duration", elapsed),
		slog.Int("total_objects", totalCount),
	)

	// Record success in sync_status table.
	if statusID > 0 {
		_ = RecordSyncComplete(ctx, w.db, statusID, Status{
			LastSyncAt:   time.Now(),
			Duration:     elapsed,
			ObjectCounts: objectCounts,
			Status:       "success",
		})
	}

	// Mark first successful sync per D-30.
	w.synced.Store(true)

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
			slog.String("error", err.Error()),
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
		slog.String("error", err.Error()),
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
			slog.String("error", err.Error()),
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
				slog.String("error", err.Error()),
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

// Per-type fetch+upsert methods. Each fetches from PeeringDB, filters deleted,
// upserts, and returns the remote IDs for the later delete pass.

func (w *Worker) fetchAndUpsertOrganizations(ctx context.Context, tx *ent.Tx) (int, []int, error) {
	items, err := peeringdb.FetchType[peeringdb.Organization](ctx, w.pdbClient, peeringdb.TypeOrg)
	if err != nil {
		return 0, nil, fmt.Errorf("fetch organizations: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterOrgsByStatus(items)
	}
	ids, err := upsertOrganizations(ctx, tx, items)
	if err != nil {
		return 0, nil, err
	}
	return len(items), ids, nil
}

func (w *Worker) fetchAndUpsertCampuses(ctx context.Context, tx *ent.Tx) (int, []int, error) {
	items, err := peeringdb.FetchType[peeringdb.Campus](ctx, w.pdbClient, peeringdb.TypeCampus)
	if err != nil {
		return 0, nil, fmt.Errorf("fetch campuses: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterCampusesByStatus(items)
	}
	ids, err := upsertCampuses(ctx, tx, items)
	if err != nil {
		return 0, nil, err
	}
	return len(items), ids, nil
}

func (w *Worker) fetchAndUpsertFacilities(ctx context.Context, tx *ent.Tx) (int, []int, error) {
	items, err := peeringdb.FetchType[peeringdb.Facility](ctx, w.pdbClient, peeringdb.TypeFac)
	if err != nil {
		return 0, nil, fmt.Errorf("fetch facilities: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterFacilitiesByStatus(items)
	}
	ids, err := upsertFacilities(ctx, tx, items)
	if err != nil {
		return 0, nil, err
	}
	return len(items), ids, nil
}

func (w *Worker) fetchAndUpsertCarriers(ctx context.Context, tx *ent.Tx) (int, []int, error) {
	items, err := peeringdb.FetchType[peeringdb.Carrier](ctx, w.pdbClient, peeringdb.TypeCarrier)
	if err != nil {
		return 0, nil, fmt.Errorf("fetch carriers: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterCarriersByStatus(items)
	}
	ids, err := upsertCarriers(ctx, tx, items)
	if err != nil {
		return 0, nil, err
	}
	return len(items), ids, nil
}

func (w *Worker) fetchAndUpsertCarrierFacilities(ctx context.Context, tx *ent.Tx) (int, []int, error) {
	items, err := peeringdb.FetchType[peeringdb.CarrierFacility](ctx, w.pdbClient, peeringdb.TypeCarrierFac)
	if err != nil {
		return 0, nil, fmt.Errorf("fetch carrier facilities: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterCarrierFacilitiesByStatus(items)
	}
	ids, err := upsertCarrierFacilities(ctx, tx, items)
	if err != nil {
		return 0, nil, err
	}
	return len(items), ids, nil
}

func (w *Worker) fetchAndUpsertInternetExchanges(ctx context.Context, tx *ent.Tx) (int, []int, error) {
	items, err := peeringdb.FetchType[peeringdb.InternetExchange](ctx, w.pdbClient, peeringdb.TypeIX)
	if err != nil {
		return 0, nil, fmt.Errorf("fetch internet exchanges: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterInternetExchangesByStatus(items)
	}
	ids, err := upsertInternetExchanges(ctx, tx, items)
	if err != nil {
		return 0, nil, err
	}
	return len(items), ids, nil
}

func (w *Worker) fetchAndUpsertIxLans(ctx context.Context, tx *ent.Tx) (int, []int, error) {
	items, err := peeringdb.FetchType[peeringdb.IxLan](ctx, w.pdbClient, peeringdb.TypeIXLan)
	if err != nil {
		return 0, nil, fmt.Errorf("fetch ix lans: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterIxLansByStatus(items)
	}
	ids, err := upsertIxLans(ctx, tx, items)
	if err != nil {
		return 0, nil, err
	}
	return len(items), ids, nil
}

func (w *Worker) fetchAndUpsertIxPrefixes(ctx context.Context, tx *ent.Tx) (int, []int, error) {
	items, err := peeringdb.FetchType[peeringdb.IxPrefix](ctx, w.pdbClient, peeringdb.TypeIXPfx)
	if err != nil {
		return 0, nil, fmt.Errorf("fetch ix prefixes: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterIxPrefixesByStatus(items)
	}
	ids, err := upsertIxPrefixes(ctx, tx, items)
	if err != nil {
		return 0, nil, err
	}
	return len(items), ids, nil
}

func (w *Worker) fetchAndUpsertIxFacilities(ctx context.Context, tx *ent.Tx) (int, []int, error) {
	items, err := peeringdb.FetchType[peeringdb.IxFacility](ctx, w.pdbClient, peeringdb.TypeIXFac)
	if err != nil {
		return 0, nil, fmt.Errorf("fetch ix facilities: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterIxFacilitiesByStatus(items)
	}
	ids, err := upsertIxFacilities(ctx, tx, items)
	if err != nil {
		return 0, nil, err
	}
	return len(items), ids, nil
}

func (w *Worker) fetchAndUpsertNetworks(ctx context.Context, tx *ent.Tx) (int, []int, error) {
	items, err := peeringdb.FetchType[peeringdb.Network](ctx, w.pdbClient, peeringdb.TypeNet)
	if err != nil {
		return 0, nil, fmt.Errorf("fetch networks: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterNetworksByStatus(items)
	}
	ids, err := upsertNetworks(ctx, tx, items)
	if err != nil {
		return 0, nil, err
	}
	return len(items), ids, nil
}

func (w *Worker) fetchAndUpsertPocs(ctx context.Context, tx *ent.Tx) (int, []int, error) {
	items, err := peeringdb.FetchType[peeringdb.Poc](ctx, w.pdbClient, peeringdb.TypePoc)
	if err != nil {
		return 0, nil, fmt.Errorf("fetch pocs: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterPocsByStatus(items)
	}
	ids, err := upsertPocs(ctx, tx, items)
	if err != nil {
		return 0, nil, err
	}
	return len(items), ids, nil
}

func (w *Worker) fetchAndUpsertNetworkFacilities(ctx context.Context, tx *ent.Tx) (int, []int, error) {
	items, err := peeringdb.FetchType[peeringdb.NetworkFacility](ctx, w.pdbClient, peeringdb.TypeNetFac)
	if err != nil {
		return 0, nil, fmt.Errorf("fetch network facilities: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterNetworkFacilitiesByStatus(items)
	}
	ids, err := upsertNetworkFacilities(ctx, tx, items)
	if err != nil {
		return 0, nil, err
	}
	return len(items), ids, nil
}

func (w *Worker) fetchAndUpsertNetworkIxLans(ctx context.Context, tx *ent.Tx) (int, []int, error) {
	items, err := peeringdb.FetchType[peeringdb.NetworkIxLan](ctx, w.pdbClient, peeringdb.TypeNetIXLan)
	if err != nil {
		return 0, nil, fmt.Errorf("fetch network ix lans: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterNetworkIxLansByStatus(items)
	}
	ids, err := upsertNetworkIxLans(ctx, tx, items)
	if err != nil {
		return 0, nil, err
	}
	return len(items), ids, nil
}

// fetchIncremental is a generic helper that fetches objects with a since cursor
// and returns items along with the generated timestamp from the API response.
func fetchIncremental[T any](ctx context.Context, c *peeringdb.Client, objectType string, since time.Time) ([]T, time.Time, error) {
	result, err := c.FetchAll(ctx, objectType, peeringdb.WithSince(since))
	if err != nil {
		return nil, time.Time{}, err
	}

	items := make([]T, 0, len(result.Data))
	for i, raw := range result.Data {
		var v T
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, time.Time{}, fmt.Errorf("unmarshal %s item %d: %w", objectType, i, err)
		}
		items = append(items, v)
	}

	generated := result.Meta.Generated
	if generated.IsZero() {
		generated = time.Now().Add(-5 * time.Minute)
	}

	return items, generated, nil
}

// Per-type incremental sync methods. Each fetches with WithSince, upserts
// (no delete stale), and returns count + generated timestamp.

func (w *Worker) syncOrganizationsIncremental(ctx context.Context, tx *ent.Tx, since time.Time) (int, time.Time, error) {
	items, generated, err := fetchIncremental[peeringdb.Organization](ctx, w.pdbClient, peeringdb.TypeOrg, since)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("fetch organizations: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterOrgsByStatus(items)
	}
	if _, err := upsertOrganizations(ctx, tx, items); err != nil {
		return 0, time.Time{}, err
	}
	return len(items), generated, nil
}

func (w *Worker) syncCampusesIncremental(ctx context.Context, tx *ent.Tx, since time.Time) (int, time.Time, error) {
	items, generated, err := fetchIncremental[peeringdb.Campus](ctx, w.pdbClient, peeringdb.TypeCampus, since)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("fetch campuses: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterCampusesByStatus(items)
	}
	if _, err := upsertCampuses(ctx, tx, items); err != nil {
		return 0, time.Time{}, err
	}
	return len(items), generated, nil
}

func (w *Worker) syncFacilitiesIncremental(ctx context.Context, tx *ent.Tx, since time.Time) (int, time.Time, error) {
	items, generated, err := fetchIncremental[peeringdb.Facility](ctx, w.pdbClient, peeringdb.TypeFac, since)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("fetch facilities: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterFacilitiesByStatus(items)
	}
	if _, err := upsertFacilities(ctx, tx, items); err != nil {
		return 0, time.Time{}, err
	}
	return len(items), generated, nil
}

func (w *Worker) syncCarriersIncremental(ctx context.Context, tx *ent.Tx, since time.Time) (int, time.Time, error) {
	items, generated, err := fetchIncremental[peeringdb.Carrier](ctx, w.pdbClient, peeringdb.TypeCarrier, since)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("fetch carriers: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterCarriersByStatus(items)
	}
	if _, err := upsertCarriers(ctx, tx, items); err != nil {
		return 0, time.Time{}, err
	}
	return len(items), generated, nil
}

func (w *Worker) syncCarrierFacilitiesIncremental(ctx context.Context, tx *ent.Tx, since time.Time) (int, time.Time, error) {
	items, generated, err := fetchIncremental[peeringdb.CarrierFacility](ctx, w.pdbClient, peeringdb.TypeCarrierFac, since)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("fetch carrier facilities: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterCarrierFacilitiesByStatus(items)
	}
	if _, err := upsertCarrierFacilities(ctx, tx, items); err != nil {
		return 0, time.Time{}, err
	}
	return len(items), generated, nil
}

func (w *Worker) syncInternetExchangesIncremental(ctx context.Context, tx *ent.Tx, since time.Time) (int, time.Time, error) {
	items, generated, err := fetchIncremental[peeringdb.InternetExchange](ctx, w.pdbClient, peeringdb.TypeIX, since)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("fetch internet exchanges: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterInternetExchangesByStatus(items)
	}
	if _, err := upsertInternetExchanges(ctx, tx, items); err != nil {
		return 0, time.Time{}, err
	}
	return len(items), generated, nil
}

func (w *Worker) syncIxLansIncremental(ctx context.Context, tx *ent.Tx, since time.Time) (int, time.Time, error) {
	items, generated, err := fetchIncremental[peeringdb.IxLan](ctx, w.pdbClient, peeringdb.TypeIXLan, since)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("fetch ix lans: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterIxLansByStatus(items)
	}
	if _, err := upsertIxLans(ctx, tx, items); err != nil {
		return 0, time.Time{}, err
	}
	return len(items), generated, nil
}

func (w *Worker) syncIxPrefixesIncremental(ctx context.Context, tx *ent.Tx, since time.Time) (int, time.Time, error) {
	items, generated, err := fetchIncremental[peeringdb.IxPrefix](ctx, w.pdbClient, peeringdb.TypeIXPfx, since)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("fetch ix prefixes: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterIxPrefixesByStatus(items)
	}
	if _, err := upsertIxPrefixes(ctx, tx, items); err != nil {
		return 0, time.Time{}, err
	}
	return len(items), generated, nil
}

func (w *Worker) syncIxFacilitiesIncremental(ctx context.Context, tx *ent.Tx, since time.Time) (int, time.Time, error) {
	items, generated, err := fetchIncremental[peeringdb.IxFacility](ctx, w.pdbClient, peeringdb.TypeIXFac, since)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("fetch ix facilities: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterIxFacilitiesByStatus(items)
	}
	if _, err := upsertIxFacilities(ctx, tx, items); err != nil {
		return 0, time.Time{}, err
	}
	return len(items), generated, nil
}

func (w *Worker) syncNetworksIncremental(ctx context.Context, tx *ent.Tx, since time.Time) (int, time.Time, error) {
	items, generated, err := fetchIncremental[peeringdb.Network](ctx, w.pdbClient, peeringdb.TypeNet, since)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("fetch networks: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterNetworksByStatus(items)
	}
	if _, err := upsertNetworks(ctx, tx, items); err != nil {
		return 0, time.Time{}, err
	}
	return len(items), generated, nil
}

func (w *Worker) syncPocsIncremental(ctx context.Context, tx *ent.Tx, since time.Time) (int, time.Time, error) {
	items, generated, err := fetchIncremental[peeringdb.Poc](ctx, w.pdbClient, peeringdb.TypePoc, since)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("fetch pocs: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterPocsByStatus(items)
	}
	if _, err := upsertPocs(ctx, tx, items); err != nil {
		return 0, time.Time{}, err
	}
	return len(items), generated, nil
}

func (w *Worker) syncNetworkFacilitiesIncremental(ctx context.Context, tx *ent.Tx, since time.Time) (int, time.Time, error) {
	items, generated, err := fetchIncremental[peeringdb.NetworkFacility](ctx, w.pdbClient, peeringdb.TypeNetFac, since)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("fetch network facilities: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterNetworkFacilitiesByStatus(items)
	}
	if _, err := upsertNetworkFacilities(ctx, tx, items); err != nil {
		return 0, time.Time{}, err
	}
	return len(items), generated, nil
}

func (w *Worker) syncNetworkIxLansIncremental(ctx context.Context, tx *ent.Tx, since time.Time) (int, time.Time, error) {
	items, generated, err := fetchIncremental[peeringdb.NetworkIxLan](ctx, w.pdbClient, peeringdb.TypeNetIXLan, since)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("fetch network ix lans: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterNetworkIxLansByStatus(items)
	}
	if _, err := upsertNetworkIxLans(ctx, tx, items); err != nil {
		return 0, time.Time{}, err
	}
	return len(items), generated, nil
}
