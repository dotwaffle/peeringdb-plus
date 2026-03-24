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
	IsPrimary      bool
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
func NewWorker(pdbClient *peeringdb.Client, entClient *ent.Client, db *sql.DB, cfg WorkerConfig, logger *slog.Logger) *Worker {
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

// syncStep defines a single step in the sync process.
type syncStep struct {
	name          string
	fn            func(ctx context.Context, tx *ent.Tx) (count int, deleted int, err error)
	incrementalFn func(ctx context.Context, tx *ent.Tx, since time.Time) (count int, generated time.Time, err error)
}

// syncSteps returns the ordered list of sync steps in FK dependency order per D-06.
func (w *Worker) syncSteps() []syncStep {
	return []syncStep{
		{"org", w.syncOrganizations, w.syncOrganizationsIncremental},
		{"campus", w.syncCampuses, w.syncCampusesIncremental},
		{"fac", w.syncFacilities, w.syncFacilitiesIncremental},
		{"carrier", w.syncCarriers, w.syncCarriersIncremental},
		{"carrierfac", w.syncCarrierFacilities, w.syncCarrierFacilitiesIncremental},
		{"ix", w.syncInternetExchanges, w.syncInternetExchangesIncremental},
		{"ixlan", w.syncIxLans, w.syncIxLansIncremental},
		{"ixpfx", w.syncIxPrefixes, w.syncIxPrefixesIncremental},
		{"ixfac", w.syncIxFacilities, w.syncIxFacilitiesIncremental},
		{"net", w.syncNetworks, w.syncNetworksIncremental},
		{"poc", w.syncPocs, w.syncPocsIncremental},
		{"netfac", w.syncNetworkFacilities, w.syncNetworkFacilitiesIncremental},
		{"netixlan", w.syncNetworkIxLans, w.syncNetworkIxLansIncremental},
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
	// references (e.g., facilities referencing deleted campuses) and the
	// delete-stale step can remove referenced rows before dependents update.
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

	for _, step := range w.syncSteps() {
		w.logger.LogAttrs(ctx, slog.LevelInfo, "syncing",
			slog.String("type", step.name),
			slog.String("mode", string(mode)),
		)

		stepStart := time.Now()
		_, stepSpan := otel.Tracer("sync").Start(ctx, "sync-"+step.name)

		var count, deleted int
		var stepErr error

		cursor, cursorErr := GetCursor(ctx, w.db, step.name)
		if cursorErr != nil {
			w.logger.LogAttrs(ctx, slog.LevelWarn, "failed to get cursor, using full sync",
				slog.String("type", step.name),
				slog.String("error", cursorErr.Error()),
			)
		}

		if mode == config.SyncModeIncremental && !cursor.IsZero() {
			// Incremental: try with since cursor.
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
				count, deleted, stepErr = step.fn(ctx, tx)
				if stepErr == nil {
					cursorUpdates[step.name] = start
				}
			} else {
				deleted = 0
				cursorUpdates[step.name] = generated
			}
		} else {
			// Full sync (default, first sync, or no cursor).
			count, deleted, stepErr = step.fn(ctx, tx)
			if stepErr == nil {
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

		// Record per-type success metrics per D-07, D-08.
		pdbotel.SyncTypeDuration.Record(ctx, time.Since(stepStart).Seconds(), typeAttr)
		pdbotel.SyncTypeObjects.Add(ctx, int64(count), typeAttr)
		pdbotel.SyncTypeDeleted.Add(ctx, int64(deleted), typeAttr)

		objectCounts[step.name] = count
		totalCount += count

		w.logger.LogAttrs(ctx, slog.LevelInfo, "synced",
			slog.String("type", step.name),
			slog.Int("count", count),
			slog.Int("deleted", deleted),
		)
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

// StartScheduler runs periodic sync via time.Ticker per D-22.
// It runs an initial sync immediately, then syncs on each tick.
// The scheduler uses the SyncMode from WorkerConfig.
// The scheduler stops when ctx is cancelled per CC-2.
func (w *Worker) StartScheduler(ctx context.Context, interval time.Duration) {
	mode := w.config.SyncMode
	if mode == "" {
		mode = config.SyncModeFull
	}

	// Run initial sync immediately using retry wrapper per D-21.
	if err := w.SyncWithRetry(ctx, mode); err != nil {
		w.logger.LogAttrs(ctx, slog.LevelError, "initial sync failed",
			slog.String("error", err.Error()),
		)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.SyncWithRetry(ctx, mode); err != nil {
				w.logger.LogAttrs(ctx, slog.LevelError, "scheduled sync failed",
					slog.String("error", err.Error()),
				)
			}
		}
	}
}

// Per-type sync methods. Each fetches from PeeringDB, filters deleted,
// upserts, and deletes stale rows.

func (w *Worker) syncOrganizations(ctx context.Context, tx *ent.Tx) (int, int, error) {
	items, err := peeringdb.FetchType[peeringdb.Organization](ctx, w.pdbClient, peeringdb.TypeOrg)
	if err != nil {
		return 0, 0, fmt.Errorf("fetch organizations: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterOrgsByStatus(items)
	}
	ids, err := upsertOrganizations(ctx, tx, items)
	if err != nil {
		return 0, 0, err
	}
	deleted, err := deleteStaleOrganizations(ctx, tx, ids)
	if err != nil {
		return 0, 0, err
	}
	return len(items), deleted, nil
}

func (w *Worker) syncCampuses(ctx context.Context, tx *ent.Tx) (int, int, error) {
	items, err := peeringdb.FetchType[peeringdb.Campus](ctx, w.pdbClient, peeringdb.TypeCampus)
	if err != nil {
		return 0, 0, fmt.Errorf("fetch campuses: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterCampusesByStatus(items)
	}
	ids, err := upsertCampuses(ctx, tx, items)
	if err != nil {
		return 0, 0, err
	}
	deleted, err := deleteStaleCampuses(ctx, tx, ids)
	if err != nil {
		return 0, 0, err
	}
	return len(items), deleted, nil
}

func (w *Worker) syncFacilities(ctx context.Context, tx *ent.Tx) (int, int, error) {
	items, err := peeringdb.FetchType[peeringdb.Facility](ctx, w.pdbClient, peeringdb.TypeFac)
	if err != nil {
		return 0, 0, fmt.Errorf("fetch facilities: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterFacilitiesByStatus(items)
	}
	ids, err := upsertFacilities(ctx, tx, items)
	if err != nil {
		return 0, 0, err
	}
	deleted, err := deleteStaleFacilities(ctx, tx, ids)
	if err != nil {
		return 0, 0, err
	}
	return len(items), deleted, nil
}

func (w *Worker) syncCarriers(ctx context.Context, tx *ent.Tx) (int, int, error) {
	items, err := peeringdb.FetchType[peeringdb.Carrier](ctx, w.pdbClient, peeringdb.TypeCarrier)
	if err != nil {
		return 0, 0, fmt.Errorf("fetch carriers: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterCarriersByStatus(items)
	}
	ids, err := upsertCarriers(ctx, tx, items)
	if err != nil {
		return 0, 0, err
	}
	deleted, err := deleteStaleCarriers(ctx, tx, ids)
	if err != nil {
		return 0, 0, err
	}
	return len(items), deleted, nil
}

func (w *Worker) syncCarrierFacilities(ctx context.Context, tx *ent.Tx) (int, int, error) {
	items, err := peeringdb.FetchType[peeringdb.CarrierFacility](ctx, w.pdbClient, peeringdb.TypeCarrierFac)
	if err != nil {
		return 0, 0, fmt.Errorf("fetch carrier facilities: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterCarrierFacilitiesByStatus(items)
	}
	ids, err := upsertCarrierFacilities(ctx, tx, items)
	if err != nil {
		return 0, 0, err
	}
	deleted, err := deleteStaleCarrierFacilities(ctx, tx, ids)
	if err != nil {
		return 0, 0, err
	}
	return len(items), deleted, nil
}

func (w *Worker) syncInternetExchanges(ctx context.Context, tx *ent.Tx) (int, int, error) {
	items, err := peeringdb.FetchType[peeringdb.InternetExchange](ctx, w.pdbClient, peeringdb.TypeIX)
	if err != nil {
		return 0, 0, fmt.Errorf("fetch internet exchanges: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterInternetExchangesByStatus(items)
	}
	ids, err := upsertInternetExchanges(ctx, tx, items)
	if err != nil {
		return 0, 0, err
	}
	deleted, err := deleteStaleInternetExchanges(ctx, tx, ids)
	if err != nil {
		return 0, 0, err
	}
	return len(items), deleted, nil
}

func (w *Worker) syncIxLans(ctx context.Context, tx *ent.Tx) (int, int, error) {
	items, err := peeringdb.FetchType[peeringdb.IxLan](ctx, w.pdbClient, peeringdb.TypeIXLan)
	if err != nil {
		return 0, 0, fmt.Errorf("fetch ix lans: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterIxLansByStatus(items)
	}
	ids, err := upsertIxLans(ctx, tx, items)
	if err != nil {
		return 0, 0, err
	}
	deleted, err := deleteStaleIxLans(ctx, tx, ids)
	if err != nil {
		return 0, 0, err
	}
	return len(items), deleted, nil
}

func (w *Worker) syncIxPrefixes(ctx context.Context, tx *ent.Tx) (int, int, error) {
	items, err := peeringdb.FetchType[peeringdb.IxPrefix](ctx, w.pdbClient, peeringdb.TypeIXPfx)
	if err != nil {
		return 0, 0, fmt.Errorf("fetch ix prefixes: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterIxPrefixesByStatus(items)
	}
	ids, err := upsertIxPrefixes(ctx, tx, items)
	if err != nil {
		return 0, 0, err
	}
	deleted, err := deleteStaleIxPrefixes(ctx, tx, ids)
	if err != nil {
		return 0, 0, err
	}
	return len(items), deleted, nil
}

func (w *Worker) syncIxFacilities(ctx context.Context, tx *ent.Tx) (int, int, error) {
	items, err := peeringdb.FetchType[peeringdb.IxFacility](ctx, w.pdbClient, peeringdb.TypeIXFac)
	if err != nil {
		return 0, 0, fmt.Errorf("fetch ix facilities: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterIxFacilitiesByStatus(items)
	}
	ids, err := upsertIxFacilities(ctx, tx, items)
	if err != nil {
		return 0, 0, err
	}
	deleted, err := deleteStaleIxFacilities(ctx, tx, ids)
	if err != nil {
		return 0, 0, err
	}
	return len(items), deleted, nil
}

func (w *Worker) syncNetworks(ctx context.Context, tx *ent.Tx) (int, int, error) {
	items, err := peeringdb.FetchType[peeringdb.Network](ctx, w.pdbClient, peeringdb.TypeNet)
	if err != nil {
		return 0, 0, fmt.Errorf("fetch networks: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterNetworksByStatus(items)
	}
	ids, err := upsertNetworks(ctx, tx, items)
	if err != nil {
		return 0, 0, err
	}
	deleted, err := deleteStaleNetworks(ctx, tx, ids)
	if err != nil {
		return 0, 0, err
	}
	return len(items), deleted, nil
}

func (w *Worker) syncPocs(ctx context.Context, tx *ent.Tx) (int, int, error) {
	items, err := peeringdb.FetchType[peeringdb.Poc](ctx, w.pdbClient, peeringdb.TypePoc)
	if err != nil {
		return 0, 0, fmt.Errorf("fetch pocs: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterPocsByStatus(items)
	}
	ids, err := upsertPocs(ctx, tx, items)
	if err != nil {
		return 0, 0, err
	}
	deleted, err := deleteStalePocs(ctx, tx, ids)
	if err != nil {
		return 0, 0, err
	}
	return len(items), deleted, nil
}

func (w *Worker) syncNetworkFacilities(ctx context.Context, tx *ent.Tx) (int, int, error) {
	items, err := peeringdb.FetchType[peeringdb.NetworkFacility](ctx, w.pdbClient, peeringdb.TypeNetFac)
	if err != nil {
		return 0, 0, fmt.Errorf("fetch network facilities: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterNetworkFacilitiesByStatus(items)
	}
	ids, err := upsertNetworkFacilities(ctx, tx, items)
	if err != nil {
		return 0, 0, err
	}
	deleted, err := deleteStaleNetworkFacilities(ctx, tx, ids)
	if err != nil {
		return 0, 0, err
	}
	return len(items), deleted, nil
}

func (w *Worker) syncNetworkIxLans(ctx context.Context, tx *ent.Tx) (int, int, error) {
	items, err := peeringdb.FetchType[peeringdb.NetworkIxLan](ctx, w.pdbClient, peeringdb.TypeNetIXLan)
	if err != nil {
		return 0, 0, fmt.Errorf("fetch network ix lans: %w", err)
	}
	if !w.config.IncludeDeleted {
		items = filterNetworkIxLansByStatus(items)
	}
	ids, err := upsertNetworkIxLans(ctx, tx, items)
	if err != nil {
		return 0, 0, err
	}
	deleted, err := deleteStaleNetworkIxLans(ctx, tx, ids)
	if err != nil {
		return 0, 0, err
	}
	return len(items), deleted, nil
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
