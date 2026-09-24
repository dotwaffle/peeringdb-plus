package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/dotwaffle/peeringdb-plus/internal/config"
)

// Status represents the result of a sync operation.
type Status struct {
	LastSyncAt   time.Time
	Duration     time.Duration
	ObjectCounts map[string]int // type -> count
	Status       string         // "success", "failed", "running"
	ErrorMessage string         // empty on success
}

// InitStatusTable creates the sync_status and sync_history_sweep tables
// if they do not exist.
// These are not ent-managed entities; they store operational metadata via raw SQL.
//
// A `mode TEXT NOT NULL DEFAULT 'incremental'` column is added
// to sync_status. Fresh databases get the column via a future CREATE TABLE
// adjustment (kept off the schema literal here for rollback simplicity);
// existing databases get it via an idempotent ALTER TABLE probed against
// pragma_table_info. GetLastSuccessfulFullSyncTime reads the column to
// implement the PDBPLUS_FULL_SYNC_INTERVAL escape hatch.
//
// The table is bounded: each sync cycle deletes old rows (see
// pruneSyncStatus).
func InitStatusTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS sync_status (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			started_at DATETIME NOT NULL,
			completed_at DATETIME,
			duration_ms INTEGER,
			object_counts TEXT,
			status TEXT NOT NULL DEFAULT 'running',
			error_message TEXT DEFAULT ''
		)
	`)
	if err != nil {
		return fmt.Errorf("create sync_status table: %w", err)
	}

	// Idempotent migration — add the `mode` column if it's
	// missing. SQLite does not support `IF NOT EXISTS` on `ALTER TABLE
	// ADD COLUMN`, so we probe via pragma_table_info first. Existing
	// primary instances upgrade their table in-place; fresh databases
	// also get the column the same way (the CREATE TABLE schema literal
	// above is intentionally NOT updated — keeping the existing schema
	// stable simplifies rollback to a pre-mu0 binary, which would still
	// run against a column it doesn't know about because the column has
	// a non-NULL DEFAULT).
	hasMode, err := columnExists(ctx, db, "sync_status", "mode")
	if err != nil {
		return fmt.Errorf("probe sync_status.mode column: %w", err)
	}
	if !hasMode {
		if _, err := db.ExecContext(ctx,
			`ALTER TABLE sync_status ADD COLUMN mode TEXT NOT NULL DEFAULT 'incremental'`,
		); err != nil {
			return fmt.Errorf("add sync_status.mode column: %w", err)
		}
	}

	// Progress of the history sweep (see history_sweep.go).
	if _, err := db.ExecContext(ctx, historySweepTableSQL); err != nil {
		return fmt.Errorf("create sync_history_sweep table: %w", err)
	}

	return nil
}

// ReapStaleRunningRows transitions any sync_status rows stuck in "running"
// state to "failed" with an explanatory error message. Call from startup
// on the primary machine — a row can get stuck in "running" if a previous
// process was killed mid-sync (e.g. rolling deploy terminated the primary
// before the sync commit/rollback path ran). The Worker.running atomic
// is reset on process start, so no future sync is blocked by these rows;
// the transition is purely cosmetic so /ui/about and /readyz queries
// stop seeing phantom "running" syncs that will never complete.
//
// Safe to call concurrently with live sync workers because the Consul
// lease guarantees only one primary at a time. A legitimate in-flight
// "running" row would be replaced by its own RecordSyncComplete call
// (latest write wins); in practice the reap runs BEFORE the first
// sync worker tick so there's no real overlap window.
//
// A transient SQLite lock error retries the UPDATE under retry, and each
// retry logs a WARN to logger (see retryOnLock). The UPDATE is safe to
// run again: a row that an earlier attempt changed is no longer
// "running".
//
// Returns the number of rows transitioned.
func ReapStaleRunningRows(ctx context.Context, db *sql.DB, logger *slog.Logger, retry LockRetry) (int, error) {
	return retryOnLock(ctx, logger, retry, opReapStaleRunningRows, func(ctx context.Context) (int, error) {
		return reapStaleRunningRows(ctx, db)
	})
}

// reapStaleRunningRows is one attempt of ReapStaleRunningRows.
func reapStaleRunningRows(ctx context.Context, db *sql.DB) (int, error) {
	result, err := db.ExecContext(ctx,
		`UPDATE sync_status
		 SET status = 'failed',
		     completed_at = ?,
		     error_message = 'startup reap: process restarted before sync completed'
		 WHERE status = 'running'`,
		time.Now(),
	)
	if err != nil {
		return 0, fmt.Errorf("reap stale running rows: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("reap stale running rows affected count: %w", err)
	}
	return int(affected), nil
}

// recordSyncStart runs RecordSyncStart on w.db, and retries it under
// w.lockRetry on a transient SQLite lock error (see retryOnLock). A
// failed INSERT adds no row, so a retry cannot add a second one.
func (w *Worker) recordSyncStart(ctx context.Context, startedAt time.Time, mode config.SyncMode) (int64, error) {
	return retryOnLock(ctx, w.logger, w.lockRetry, opRecordSyncStart, func(ctx context.Context) (int64, error) {
		return RecordSyncStart(ctx, w.db, startedAt, string(mode))
	})
}

// recordSyncComplete runs RecordSyncComplete on w.db, and retries it
// under w.lockRetry on a transient SQLite lock error (see retryOnLock).
// Each attempt writes the same values.
func (w *Worker) recordSyncComplete(ctx context.Context, id int64, status Status) error {
	_, err := retryOnLock(ctx, w.logger, w.lockRetry, opRecordSyncComplete, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, RecordSyncComplete(ctx, w.db, id, status)
	})
	return err
}

// The sync_status retention limits of Worker.Sync (see pruneSyncStatus).
const (
	// syncStatusKeepRows is the number of newest sync_status rows that
	// the prune keeps. That is about 31 days at the 15m sync interval.
	// When each tick fails and retries 3 times, it is about 7.8 days. A
	// reader that lists cycles sees at most this number of rows, plus the
	// two anchor rows.
	syncStatusKeepRows = 3000
	// syncStatusPruneBatch is the maximum number of rows that one prune
	// deletes. When the first prunes delete an old backlog, the batch
	// keeps each commit small, so each LTX file that LiteFS sends to the
	// replicas is small too.
	syncStatusPruneBatch = 1000
)

// pruneSyncStatus deletes the sync_status rows that are older than the
// keep newest rows, at most batch rows per call, oldest first. It returns
// the number of deleted rows. It never deletes the two anchor rows,
// however old they are:
//
//   - The newest success row. GetLastSuccessfulSyncTime and
//     GetLastSuccessfulStatus read it for the synced latch, the warm
//     start, the ETag watcher and the freshness displays.
//   - The newest full success row. GetLastSuccessfulFullSyncTime reads
//     it for the PDBPLUS_FULL_SYNC_INTERVAL check.
//
// The anchor predicates are copies of the WHERE clauses of these
// readers, so each anchor is the row that its reader returns.
// Worker.Sync calls the prune after it inserts the running row of the
// cycle, so the window always holds the newest row (GetLastStatus). It
// also holds the newest completed row (GetLastCompletedStatus) unless
// the keep-1 rows before the running row all stayed 'running', which
// needs keep-1 failed status updates in a row. At most keep + 2 rows
// remain.
//
// The rule counts rows, not days. The DSN sets no _time_format, so
// modernc.org/sqlite stores a time.Time as the text of t.String(), which
// the SQLite date functions cannot parse. Never compare sync_status times
// in SQL. AUTOINCREMENT ids only increase, so the id order is the insert
// order.
//
// The boundary is the id of the keep-th newest row. When the table has
// fewer rows, the boundary is NULL and no row is deleted. The anchors use
// IS NOT. When no success row or no full success row exists, that anchor
// is NULL, and "id IS NOT NULL" is true. So a missing anchor does not
// stop the prune. With != (or NOT IN with a list of the two anchor
// values), a NULL anchor gives NULL for every row, and the prune deletes
// nothing.
//
// This SQLite build has no DELETE ... LIMIT, so the LIMIT is in the
// IN-subquery. SQLite completes the subquery before it deletes a row.
// SQLite reads a negative LIMIT as no limit and a negative OFFSET as 0,
// so the function reads keep and batch below 1 as 1.
func pruneSyncStatus(ctx context.Context, db *sql.DB, keep, batch int) (int64, error) {
	result, err := db.ExecContext(ctx,
		`DELETE FROM sync_status
		 WHERE id IN (
		   SELECT id FROM sync_status
		   WHERE id < (SELECT id FROM sync_status ORDER BY id DESC LIMIT 1 OFFSET ?)
		     AND id IS NOT (SELECT id FROM sync_status
		                    WHERE status = 'success' ORDER BY id DESC LIMIT 1)
		     AND id IS NOT (SELECT id FROM sync_status
		                    WHERE status = 'success' AND mode = 'full' ORDER BY id DESC LIMIT 1)
		   ORDER BY id
		   LIMIT ?
		 )`,
		max(keep, 1)-1, max(batch, 1),
	)
	if err != nil {
		return 0, fmt.Errorf("prune sync_status: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("prune sync_status affected count: %w", err)
	}
	return deleted, nil
}

// pruneStatusRows runs pruneSyncStatus on w.db with syncStatusKeepRows
// and syncStatusPruneBatch. Worker.Sync calls it once per attempt, after
// the INSERT of the running row. It runs inside the running latch, so no
// other sync_status write runs at the same time. It uses the cycle
// context, so the watchdog, a demotion and SIGTERM cancel it.
//
// The prune is optional, and the next attempt runs it again, so it makes
// one attempt and does not retry on a lock error. busy_timeout still
// limits a SQLITE_BUSY wait. SQLite rolls back a failed autocommit
// statement, so a failure deletes no row. An error logs a WARN and does
// not fail the cycle. A successful prune sets the
// pdbplus.sync.status_rows_pruned attribute on the sync span, and logs
// the count at DEBUG when it deleted rows.
func (w *Worker) pruneStatusRows(ctx context.Context) {
	deleted, err := pruneSyncStatus(ctx, w.db, syncStatusKeepRows, syncStatusPruneBatch)
	if err != nil {
		w.logger.LogAttrs(ctx, slog.LevelWarn, "failed to prune sync_status rows",
			slog.Any("error", err))
		return
	}
	trace.SpanFromContext(ctx).SetAttributes(attribute.Int64("pdbplus.sync.status_rows_pruned", deleted))
	if deleted > 0 {
		w.logger.LogAttrs(ctx, slog.LevelDebug, "pruned sync_status rows",
			slog.Int64("deleted", deleted))
	}
}

// RecordSyncStart inserts a new running sync status row and returns its ID.
//
// Mode is "full" or "incremental" — persisted in the
// sync_status.mode column so GetLastSuccessfulFullSyncTime can find the
// most recent full-sync completion (used by the
// PDBPLUS_FULL_SYNC_INTERVAL escape hatch). The mode parameter MUST
// reflect the cycle's effective behaviour, not the configured default
// — a forced bare-list refetch should be recorded as "full".
func RecordSyncStart(ctx context.Context, db *sql.DB, startedAt time.Time, mode string) (int64, error) {
	result, err := db.ExecContext(ctx,
		`INSERT INTO sync_status (started_at, status, mode) VALUES (?, 'running', ?)`,
		startedAt, mode,
	)
	if err != nil {
		return 0, fmt.Errorf("record sync start: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get sync status id: %w", err)
	}
	return id, nil
}

// GetLastSuccessfulFullSyncTime returns the completion time of the most
// recent successful FULL sync, or zero time if no full sync has been
// recorded. Used by the PDBPLUS_FULL_SYNC_INTERVAL escape hatch in
// syncFetchPass to force a periodic bare-list refetch — defends against
// pathological upstream cross-row inconsistency where a since= response
// includes row R' (updated=M) but is missing earlier row R (updated < M);
// R is permanently missed under any since-based design.
func GetLastSuccessfulFullSyncTime(ctx context.Context, db *sql.DB) (time.Time, error) {
	var completedAt time.Time
	err := db.QueryRowContext(ctx,
		`SELECT completed_at FROM sync_status
		 WHERE status = 'success' AND mode = 'full'
		 ORDER BY id DESC LIMIT 1`,
	).Scan(&completedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("get last successful full sync time: %w", err)
	}
	return completedAt, nil
}

// columnExists reports whether the named column is present on the named
// table. Implemented via pragma_table_info — the only built-in SQLite
// way to introspect a column without parsing CREATE TABLE statements.
// Used by InitStatusTable to make the sync_status.mode migration
// idempotent.
func columnExists(ctx context.Context, db *sql.DB, table, column string) (bool, error) {
	// #nosec G201 — table/column are package-internal constants, not
	// caller-supplied; SQL injection is not possible. Same justification
	// pattern as internal/sync/cursor.go GetMaxUpdated.
	query := fmt.Sprintf(`SELECT 1 FROM pragma_table_info(%q) WHERE name = ?`, table)
	rows, err := db.QueryContext(ctx, query, column)
	if err != nil {
		return false, fmt.Errorf("query pragma_table_info(%s): %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	exists := rows.Next()
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate pragma_table_info(%s): %w", table, err)
	}
	return exists, nil
}

// RecordSyncComplete updates the sync status row with results.
func RecordSyncComplete(ctx context.Context, db *sql.DB, id int64, status Status) error {
	countsJSON, err := json.Marshal(status.ObjectCounts)
	if err != nil {
		return fmt.Errorf("marshal object counts: %w", err)
	}
	_, err = db.ExecContext(ctx,
		`UPDATE sync_status SET completed_at = ?, duration_ms = ?, object_counts = ?, status = ?, error_message = ? WHERE id = ?`,
		status.LastSyncAt,
		status.Duration.Milliseconds(),
		string(countsJSON),
		status.Status,
		status.ErrorMessage,
		id,
	)
	if err != nil {
		return fmt.Errorf("record sync complete: %w", err)
	}
	return nil
}

// GetLastSuccessfulSyncTime returns the completion time of the most recent
// successful sync, or zero time if no successful sync has been recorded.
func GetLastSuccessfulSyncTime(ctx context.Context, db *sql.DB) (time.Time, error) {
	var completedAt time.Time
	err := db.QueryRowContext(ctx,
		`SELECT completed_at FROM sync_status WHERE status = 'success' ORDER BY id DESC LIMIT 1`,
	).Scan(&completedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("get last successful sync time: %w", err)
	}
	return completedAt, nil
}

// GetLastStatus returns the most recent sync status.
// Returns nil if no sync has been recorded.
func GetLastStatus(ctx context.Context, db *sql.DB) (*Status, error) {
	row := db.QueryRowContext(ctx,
		`SELECT started_at, completed_at, duration_ms, object_counts, status, error_message FROM sync_status ORDER BY id DESC LIMIT 1`,
	)
	return scanStatusRow(row, "get last sync status")
}

// GetLastCompletedStatus returns the most recent non-running sync status row
// (either "success" or "failed"). Used by /readyz to fall back past an
// in-flight "running" row so the health check reflects the most recent
// outcome — whether success or failure. Returns nil if no completed sync
// has ever been recorded.
//
// NOTE: this is the right answer for /readyz (which wants to know "what's
// the most recent outcome?" and reports unhealthy on failed) but NOT for
// UI freshness displays (which want "when was the last known-good data?").
// UI surfaces should use GetLastSuccessfulStatus instead.
func GetLastCompletedStatus(ctx context.Context, db *sql.DB) (*Status, error) {
	row := db.QueryRowContext(ctx,
		`SELECT started_at, completed_at, duration_ms, object_counts, status, error_message
		 FROM sync_status
		 WHERE status != 'running'
		 ORDER BY id DESC LIMIT 1`,
	)
	return scanStatusRow(row, "get last completed sync status")
}

// GetLastSuccessfulStatus returns the most recent successful sync status row.
// This is the right answer for UI surfaces that want to display "when was
// the last known-good data?" — it skips past any in-flight "running" rows
// AND any "failed" rows to find the most recent row with status="success".
// Returns nil if no successful sync has ever been recorded.
//
// Unlike GetLastSuccessfulSyncTime (which returns only the timestamp),
// this returns the full Status struct including object counts and
// duration for display purposes.
func GetLastSuccessfulStatus(ctx context.Context, db *sql.DB) (*Status, error) {
	row := db.QueryRowContext(ctx,
		`SELECT started_at, completed_at, duration_ms, object_counts, status, error_message
		 FROM sync_status
		 WHERE status = 'success'
		 ORDER BY id DESC LIMIT 1`,
	)
	return scanStatusRow(row, "get last successful sync status")
}

// scanStatusRow decodes a sync_status row into a *Status. Returns (nil, nil)
// when the row is empty (sql.ErrNoRows). errContext is used as the error
// message prefix so callers can tell which query failed.
func scanStatusRow(row *sql.Row, errContext string) (*Status, error) {
	var (
		startedAt    time.Time
		completedAt  sql.NullTime
		durationMs   sql.NullInt64
		countsStr    sql.NullString
		statusStr    string
		errorMessage sql.NullString
	)
	err := row.Scan(&startedAt, &completedAt, &durationMs, &countsStr, &statusStr, &errorMessage)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errContext, err)
	}

	s := &Status{
		LastSyncAt: startedAt,
		Status:     statusStr,
	}
	if completedAt.Valid {
		s.LastSyncAt = completedAt.Time
	}
	if durationMs.Valid {
		s.Duration = time.Duration(durationMs.Int64) * time.Millisecond
	}
	if countsStr.Valid && countsStr.String != "" {
		s.ObjectCounts = make(map[string]int)
		if err := json.Unmarshal([]byte(countsStr.String), &s.ObjectCounts); err != nil {
			return nil, fmt.Errorf("unmarshal object counts: %w", err)
		}
	}
	if errorMessage.Valid {
		s.ErrorMessage = errorMessage.String
	}
	return s, nil
}
