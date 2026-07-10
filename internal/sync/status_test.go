package sync_test

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/sync"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

func TestInitStatusTable_CreatesStatusTable(t *testing.T) {
	t.Parallel()
	_, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()

	if err := sync.InitStatusTable(ctx, db); err != nil {
		t.Fatalf("InitStatusTable: %v", err)
	}

	var name string
	err := db.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='table' AND name='sync_status'`,
	).Scan(&name)
	if err != nil {
		t.Fatalf("sync_status table not found: %v", err)
	}

	// The idempotent mode-column migration must have run.
	var one int
	err = db.QueryRowContext(ctx,
		`SELECT 1 FROM pragma_table_info('sync_status') WHERE name = 'mode'`,
	).Scan(&one)
	if err != nil {
		t.Fatalf("sync_status.mode column not found: %v", err)
	}
}

func TestInitStatusTable_DBError(t *testing.T) {
	t.Parallel()
	_, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()

	db.Close()

	err := sync.InitStatusTable(ctx, db)
	if err == nil {
		t.Fatal("expected error for closed DB, got nil")
	}
	if !strings.Contains(err.Error(), "create sync_status table") {
		t.Errorf("error = %q, want substring %q", err.Error(), "create sync_status table")
	}
}

func TestRecordSyncStart_DBError(t *testing.T) {
	t.Parallel()
	_, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()

	if err := sync.InitStatusTable(ctx, db); err != nil {
		t.Fatalf("InitStatusTable: %v", err)
	}

	db.Close()

	_, err := sync.RecordSyncStart(ctx, db, time.Now(), "incremental")
	if err == nil {
		t.Fatal("expected error for closed DB, got nil")
	}
	if !strings.Contains(err.Error(), "record sync start") {
		t.Errorf("error = %q, want substring %q", err.Error(), "record sync start")
	}
}

func TestRecordSyncComplete_DBError(t *testing.T) {
	t.Parallel()
	_, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()

	if err := sync.InitStatusTable(ctx, db); err != nil {
		t.Fatalf("InitStatusTable: %v", err)
	}

	// Record a start row so we have a valid ID.
	id, err := sync.RecordSyncStart(ctx, db, time.Now(), "full")
	if err != nil {
		t.Fatalf("RecordSyncStart: %v", err)
	}

	db.Close()

	err = sync.RecordSyncComplete(ctx, db, id, sync.Status{
		LastSyncAt:   time.Now(),
		Duration:     time.Second,
		ObjectCounts: map[string]int{"net": 10},
		Status:       "success",
	})
	if err == nil {
		t.Fatal("expected error for closed DB, got nil")
	}
	if !strings.Contains(err.Error(), "record sync complete") {
		t.Errorf("error = %q, want substring %q", err.Error(), "record sync complete")
	}
}

func TestGetLastSuccessfulSyncTime_DBError(t *testing.T) {
	t.Parallel()
	_, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()

	if err := sync.InitStatusTable(ctx, db); err != nil {
		t.Fatalf("InitStatusTable: %v", err)
	}

	db.Close()

	_, err := sync.GetLastSuccessfulSyncTime(ctx, db)
	if err == nil {
		t.Fatal("expected error for closed DB, got nil")
	}
	if !strings.Contains(err.Error(), "get last successful sync time") {
		t.Errorf("error = %q, want substring %q", err.Error(), "get last successful sync time")
	}
}

func TestGetLastStatus_DBError(t *testing.T) {
	t.Parallel()
	_, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()

	if err := sync.InitStatusTable(ctx, db); err != nil {
		t.Fatalf("InitStatusTable: %v", err)
	}

	db.Close()

	_, err := sync.GetLastStatus(ctx, db)
	if err == nil {
		t.Fatal("expected error for closed DB, got nil")
	}
	if !strings.Contains(err.Error(), "get last sync status") {
		t.Errorf("error = %q, want substring %q", err.Error(), "get last sync status")
	}
}

func TestReapStaleRunningRows(t *testing.T) {
	t.Parallel()
	_, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()

	if err := sync.InitStatusTable(ctx, db); err != nil {
		t.Fatalf("InitStatusTable: %v", err)
	}

	// Seed three rows: one running (stale), one success, one failed.
	// Only the running row should be transitioned by the reap.
	now := time.Now()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO sync_status (started_at, status) VALUES (?, 'running')`,
		now.Add(-2*time.Hour),
	); err != nil {
		t.Fatalf("seed running row: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO sync_status (started_at, completed_at, duration_ms, status) VALUES (?, ?, ?, 'success')`,
		now.Add(-1*time.Hour), now.Add(-59*time.Minute), 60000,
	); err != nil {
		t.Fatalf("seed success row: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO sync_status (started_at, completed_at, duration_ms, status, error_message) VALUES (?, ?, ?, 'failed', ?)`,
		now.Add(-30*time.Minute), now.Add(-29*time.Minute), 60000, "boom",
	); err != nil {
		t.Fatalf("seed failed row: %v", err)
	}

	reaped, err := sync.ReapStaleRunningRows(ctx, db)
	if err != nil {
		t.Fatalf("ReapStaleRunningRows: %v", err)
	}
	if reaped != 1 {
		t.Errorf("reaped = %d, want 1", reaped)
	}

	// Verify: zero running rows left, one newly-failed row with the reap message.
	var runningCount int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sync_status WHERE status = 'running'`,
	).Scan(&runningCount); err != nil {
		t.Fatalf("count running: %v", err)
	}
	if runningCount != 0 {
		t.Errorf("running rows after reap = %d, want 0", runningCount)
	}

	var reapMsgCount int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sync_status WHERE status = 'failed' AND error_message LIKE 'startup reap%'`,
	).Scan(&reapMsgCount); err != nil {
		t.Fatalf("count reap-marked: %v", err)
	}
	if reapMsgCount != 1 {
		t.Errorf("reap-marked rows = %d, want 1", reapMsgCount)
	}

	// Verify: pre-existing "success" and "failed" rows were NOT touched.
	var successCount, originalFailedCount int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sync_status WHERE status = 'success'`,
	).Scan(&successCount); err != nil {
		t.Fatalf("count success: %v", err)
	}
	if successCount != 1 {
		t.Errorf("success rows after reap = %d, want 1 (unchanged)", successCount)
	}
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sync_status WHERE status = 'failed' AND error_message = 'boom'`,
	).Scan(&originalFailedCount); err != nil {
		t.Fatalf("count original failed: %v", err)
	}
	if originalFailedCount != 1 {
		t.Errorf("original failed rows = %d, want 1 (unchanged)", originalFailedCount)
	}
}

func TestReapStaleRunningRows_NoOp(t *testing.T) {
	t.Parallel()
	_, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()

	if err := sync.InitStatusTable(ctx, db); err != nil {
		t.Fatalf("InitStatusTable: %v", err)
	}

	reaped, err := sync.ReapStaleRunningRows(ctx, db)
	if err != nil {
		t.Fatalf("ReapStaleRunningRows: %v", err)
	}
	if reaped != 0 {
		t.Errorf("reaped = %d, want 0 for empty table", reaped)
	}
}

// TestStatusMigration_ModeColumnIdempotent locks the schema
// migration: an existing primary instance with a sync_status table that
// LACKS the `mode` column gains it via idempotent ALTER TABLE on the
// next InitStatusTable call. A second InitStatusTable call is a no-op
// (no duplicate column, no error).
//
// Simulates the production upgrade path by manually creating the
// earlier schema (sync_status WITHOUT mode), then calling
// InitStatusTable, then introspecting via pragma_table_info.
func TestStatusMigration_ModeColumnIdempotent(t *testing.T) {
	t.Parallel()
	_, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()

	// Drop the auto-created sync_status table (testutil.SetupClientWithDB
	// + ent migrate doesn't create it, but be defensive). Then create
	// the pre-mu0 schema literal to simulate a primary that hasn't been
	// upgraded yet.
	if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS sync_status`); err != nil {
		t.Fatalf("drop sync_status: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE sync_status (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			started_at DATETIME NOT NULL,
			completed_at DATETIME,
			duration_ms INTEGER,
			object_counts TEXT,
			status TEXT NOT NULL DEFAULT 'running',
			error_message TEXT DEFAULT ''
		)
	`); err != nil {
		t.Fatalf("create pre-mu0 sync_status: %v", err)
	}

	// First call: should add the `mode` column.
	if err := sync.InitStatusTable(ctx, db); err != nil {
		t.Fatalf("InitStatusTable (first call): %v", err)
	}
	if !columnExistsTest(t, db, "sync_status", "mode") {
		t.Fatal("expected sync_status.mode to exist after first InitStatusTable call")
	}

	// Second call: idempotent — no duplicate column, no error.
	if err := sync.InitStatusTable(ctx, db); err != nil {
		t.Fatalf("InitStatusTable (second call): %v", err)
	}
	if !columnExistsTest(t, db, "sync_status", "mode") {
		t.Fatal("expected sync_status.mode to exist after second InitStatusTable call")
	}

	// Insert a row WITHOUT specifying mode — defaults to 'incremental'.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO sync_status (started_at, status) VALUES (?, 'success')`,
		time.Now(),
	); err != nil {
		t.Fatalf("insert default-mode row: %v", err)
	}
	var mode string
	if err := db.QueryRowContext(ctx,
		`SELECT mode FROM sync_status ORDER BY id DESC LIMIT 1`,
	).Scan(&mode); err != nil {
		t.Fatalf("read mode: %v", err)
	}
	if mode != "incremental" {
		t.Errorf("expected default mode 'incremental', got %q", mode)
	}
}

// TestRecordSyncStart_PersistsMode locks the RecordSyncStart
// signature: the mode parameter lands in sync_status.mode and is
// distinguishable from the default.
func TestRecordSyncStart_PersistsMode(t *testing.T) {
	t.Parallel()
	_, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()
	if err := sync.InitStatusTable(ctx, db); err != nil {
		t.Fatalf("InitStatusTable: %v", err)
	}

	t1 := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	idFull, err := sync.RecordSyncStart(ctx, db, t1, "full")
	if err != nil {
		t.Fatalf("RecordSyncStart full: %v", err)
	}
	idIncr, err := sync.RecordSyncStart(ctx, db, t1.Add(time.Hour), "incremental")
	if err != nil {
		t.Fatalf("RecordSyncStart incremental: %v", err)
	}

	for _, c := range []struct {
		id   int64
		want string
	}{
		{idFull, "full"},
		{idIncr, "incremental"},
	} {
		var got string
		if err := db.QueryRowContext(ctx,
			`SELECT mode FROM sync_status WHERE id = ?`, c.id,
		).Scan(&got); err != nil {
			t.Fatalf("read mode for id %d: %v", c.id, err)
		}
		if got != c.want {
			t.Errorf("sync_status.mode for id %d = %q, want %q", c.id, got, c.want)
		}
	}
}

// TestGetLastSuccessfulFullSyncTime locks the query: filters
// to status='success' AND mode='full', returning the most recent
// completed_at. Verifies that intervening incremental successes do NOT
// shadow the most recent full sync.
func TestGetLastSuccessfulFullSyncTime(t *testing.T) {
	t.Parallel()
	_, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()
	if err := sync.InitStatusTable(ctx, db); err != nil {
		t.Fatalf("InitStatusTable: %v", err)
	}

	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	// Seed three completed rows: full @ T-2h, incremental @ T-1h, full @ T-30m.
	for _, row := range []struct {
		started time.Time
		mode    string
	}{
		{now.Add(-2 * time.Hour), "full"},
		{now.Add(-1 * time.Hour), "incremental"},
		{now.Add(-30 * time.Minute), "full"},
	} {
		id, err := sync.RecordSyncStart(ctx, db, row.started, row.mode)
		if err != nil {
			t.Fatalf("seed RecordSyncStart: %v", err)
		}
		if err := sync.RecordSyncComplete(ctx, db, id, sync.Status{
			LastSyncAt: row.started.Add(time.Minute),
			Duration:   time.Minute,
			Status:     "success",
		}); err != nil {
			t.Fatalf("seed RecordSyncComplete: %v", err)
		}
	}

	got, err := sync.GetLastSuccessfulFullSyncTime(ctx, db)
	if err != nil {
		t.Fatalf("GetLastSuccessfulFullSyncTime: %v", err)
	}
	want := now.Add(-30 * time.Minute).Add(time.Minute) // most recent full's completed_at
	if delta := got.Sub(want); delta < -time.Second || delta > time.Second {
		t.Errorf("GetLastSuccessfulFullSyncTime = %v, want %v (±1s)", got, want)
	}

	// Add an incremental success @ T — should NOT shadow the full @ T-30m.
	id, err := sync.RecordSyncStart(ctx, db, now, "incremental")
	if err != nil {
		t.Fatalf("late incremental RecordSyncStart: %v", err)
	}
	if err := sync.RecordSyncComplete(ctx, db, id, sync.Status{
		LastSyncAt: now.Add(time.Minute),
		Duration:   time.Minute,
		Status:     "success",
	}); err != nil {
		t.Fatalf("late incremental RecordSyncComplete: %v", err)
	}
	got2, err := sync.GetLastSuccessfulFullSyncTime(ctx, db)
	if err != nil {
		t.Fatalf("GetLastSuccessfulFullSyncTime (after incremental): %v", err)
	}
	if !got2.Equal(got) {
		t.Errorf("intervening incremental success changed full-sync result; got %v, want %v",
			got2, got)
	}
}

// TestGetLastSuccessfulFullSyncTime_None: empty sync_status (no full
// sync ever recorded) → zero time, no error.
func TestGetLastSuccessfulFullSyncTime_None(t *testing.T) {
	t.Parallel()
	_, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()
	if err := sync.InitStatusTable(ctx, db); err != nil {
		t.Fatalf("InitStatusTable: %v", err)
	}

	got, err := sync.GetLastSuccessfulFullSyncTime(ctx, db)
	if err != nil {
		t.Fatalf("GetLastSuccessfulFullSyncTime: %v", err)
	}
	if !got.IsZero() {
		t.Errorf("expected zero time on empty sync_status, got %v", got)
	}

	// Add an incremental-only history → still zero time (no full).
	id, err := sync.RecordSyncStart(ctx, db, time.Now(), "incremental")
	if err != nil {
		t.Fatalf("RecordSyncStart: %v", err)
	}
	if err := sync.RecordSyncComplete(ctx, db, id, sync.Status{
		LastSyncAt: time.Now(),
		Duration:   time.Second,
		Status:     "success",
	}); err != nil {
		t.Fatalf("RecordSyncComplete: %v", err)
	}
	got2, err := sync.GetLastSuccessfulFullSyncTime(ctx, db)
	if err != nil {
		t.Fatalf("GetLastSuccessfulFullSyncTime (incremental-only): %v", err)
	}
	if !got2.IsZero() {
		t.Errorf("expected zero time when only incremental syncs exist, got %v", got2)
	}
}

// columnExistsTest is a test-only helper to verify a column exists in a
// SQLite table. Cannot reach into sync.columnExists (unexported) from
// _test package; pragma_table_info is the canonical SQLite introspection.
func columnExistsTest(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := db.QueryContext(t.Context(),
		`SELECT 1 FROM pragma_table_info(?) WHERE name = ?`,
		table, column,
	)
	if err != nil {
		t.Fatalf("pragma_table_info(%s): %v", table, err)
	}
	defer func() { _ = rows.Close() }()
	return rows.Next()
}
