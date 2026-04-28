package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
)

// Status represents the result of a sync operation.
type Status struct {
	LastSyncAt   time.Time
	Duration     time.Duration
	ObjectCounts map[string]int // type -> count
	Status       string         // "success", "failed", "running"
	ErrorMessage string         // empty on success
}

// InitStatusTable creates the sync_status and sync_cursors tables if they don't exist.
// These are not ent-managed entities; they store operational metadata via raw SQL.
//
// 260428-mu0: a `mode TEXT NOT NULL DEFAULT 'incremental'` column is added
// to sync_status. Fresh databases get the column via a future CREATE TABLE
// adjustment (kept off the schema literal here for rollback simplicity);
// existing databases get it via an idempotent ALTER TABLE probed against
// pragma_table_info. GetLastSuccessfulFullSyncTime reads the column to
// implement the PDBPLUS_FULL_SYNC_INTERVAL escape hatch.
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

	// 260428-mu0: idempotent migration — add the `mode` column if it's
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

	// DEPRECATED v1.18.10 (260428-mu0): the sync_cursors table is no
	// longer written to — cursors are now derived from MAX(updated) per
	// table by internal/sync/cursor.go GetMaxUpdated. The CREATE TABLE
	// statement is preserved so an emergency rollback to a binary that
	// reads sync_cursors continues to function (it will see whatever
	// rows the prior run last wrote, which are still valid high-water-
	// mark timestamps). Slated for removal in v1.19.
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS sync_cursors (
			type TEXT PRIMARY KEY,
			last_sync_at DATETIME NOT NULL,
			last_status TEXT NOT NULL DEFAULT 'success'
		)
	`)
	if err != nil {
		return fmt.Errorf("create sync_cursors table: %w", err)
	}

	return nil
}

// DEPRECATED v1.18.10 (260428-mu0): cursor is now derived from
// MAX(updated) per table via internal/sync/cursor.go GetMaxUpdated.
// Slated for removal in v1.19. Do not call from new code. The
// sync_cursors table CREATE TABLE in InitStatusTable is preserved for
// rollback safety; existing rows are ignored by the production sync
// path.
//
// GetCursor returns the last sync timestamp for a type. Returns zero
// time only if no cursor row exists for the type.
//
// v1.18.3: dropped the prior `AND last_status = 'success'` filter. The
// cursor is a high-water-mark timestamp; failure observability lives in
// the separate sync_status table. Coupling cursor reads to last_status
// caused subtle "all cursors zero after a failed cycle" surprises that
// could trigger expensive re-fetches (and was load-bearing in the
// v1.18.2 bootstrap regression). The last_status column is preserved
// for stored data compatibility and any future dashboard queries.
func GetCursor(ctx context.Context, db *sql.DB, objType string) (time.Time, error) {
	var lastSyncAt time.Time
	err := db.QueryRowContext(ctx,
		`SELECT last_sync_at FROM sync_cursors WHERE type = ?`,
		objType,
	).Scan(&lastSyncAt)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("get cursor for %s: %w", objType, err)
	}
	return lastSyncAt, nil
}

// DEPRECATED v1.18.10 (260428-mu0): cursor is now derived from
// MAX(updated) per table via internal/sync/cursor.go GetMaxUpdated.
// Slated for removal in v1.19. Do not call from new code. The
// sync_cursors table CREATE TABLE in InitStatusTable is preserved for
// rollback safety; existing rows are ignored by the production sync
// path.
//
// UpsertCursor updates or inserts the sync cursor for a type.
//
// 260428-eda CHANGE 2: called WITHIN the main sync transaction (via *ent.Tx)
// so cursor writes commit atomically with their corresponding ent upserts.
// This closes the prior gap where cursor writes were 13 separate post-commit
// *sql.DB Exec calls — each one a LiteFS-replicated commit — and removes the
// failure window where ent upserts were durable but the cursor advance was
// not (resulting in re-fetching already-applied rows on the next cycle).
//
// D-19 atomicity: sync_status (the outcome record) remains a separate
// raw-SQL Exec because it must reflect the OUTCOME of the tx
// (success/failure/error message) — that's correct. Cursors describe DATA
// STATE and belong inside the data tx.
//
// Failure-mode shift: a cursor-write failure now rolls back the entire
// upsert tx (including any FK-backfill HTTP work that already happened
// inside it). This is the CORRECT semantic — cursor IS data state and a
// divergence between upserts-committed and cursor-not-advanced is the very
// bug being fixed. The OTel span attribute
// pdbplus.sync.cursor_write_caused_rollback is set true on the sync root
// span when a rollback was caused by cursor write failure (B3).
// SyncWithRetry handles transient failures by re-running the cycle.
func UpsertCursor(ctx context.Context, tx *ent.Tx, objType string, lastSyncAt time.Time, status string) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO sync_cursors (type, last_sync_at, last_status)
		 VALUES (?, ?, ?)
		 ON CONFLICT(type) DO UPDATE SET last_sync_at = excluded.last_sync_at, last_status = excluded.last_status`,
		objType, lastSyncAt, status,
	)
	if err != nil {
		return fmt.Errorf("upsert cursor for %s: %w", objType, err)
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
// Returns the number of rows transitioned.
func ReapStaleRunningRows(ctx context.Context, db *sql.DB) (int, error) {
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

// RecordSyncStart inserts a new running sync status row and returns its ID.
//
// 260428-mu0: mode is "full" or "incremental" — persisted in the
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
//
// 260428-mu0.
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
// idempotent (260428-mu0).
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
// Unlike GetLastSuccessfulSyncTime (which returns only the timestamp for
// ETag seeding), this returns the full Status struct including object
// counts and duration for display purposes.
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
