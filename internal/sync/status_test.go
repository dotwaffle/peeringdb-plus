package sync_test

import (
	"strings"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/sync"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

func TestInitStatusTable_CreatesCursorsTable(t *testing.T) {
	t.Parallel()
	_, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()

	if err := sync.InitStatusTable(ctx, db); err != nil {
		t.Fatalf("InitStatusTable: %v", err)
	}

	var name string
	err := db.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='table' AND name='sync_cursors'`,
	).Scan(&name)
	if err != nil {
		t.Fatalf("sync_cursors table not found: %v", err)
	}
	if name != "sync_cursors" {
		t.Errorf("expected table name sync_cursors, got %s", name)
	}
}

func TestGetCursor_NoRows(t *testing.T) {
	t.Parallel()
	_, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()

	if err := sync.InitStatusTable(ctx, db); err != nil {
		t.Fatalf("InitStatusTable: %v", err)
	}

	got, err := sync.GetCursor(ctx, db, "net")
	if err != nil {
		t.Fatalf("GetCursor: %v", err)
	}
	if !got.IsZero() {
		t.Errorf("expected zero time for missing cursor, got %v", got)
	}
}

func TestUpsertCursor_InsertAndGet(t *testing.T) {
	t.Parallel()
	_, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()

	if err := sync.InitStatusTable(ctx, db); err != nil {
		t.Fatalf("InitStatusTable: %v", err)
	}

	ts := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	if err := sync.UpsertCursor(ctx, db, "net", ts, "success"); err != nil {
		t.Fatalf("UpsertCursor: %v", err)
	}

	got, err := sync.GetCursor(ctx, db, "net")
	if err != nil {
		t.Fatalf("GetCursor: %v", err)
	}
	if !got.Equal(ts) {
		t.Errorf("expected %v, got %v", ts, got)
	}
}

func TestUpsertCursor_UpdateExisting(t *testing.T) {
	t.Parallel()
	_, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()

	if err := sync.InitStatusTable(ctx, db); err != nil {
		t.Fatalf("InitStatusTable: %v", err)
	}

	ts1 := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	ts2 := time.Date(2026, 3, 23, 13, 0, 0, 0, time.UTC)

	if err := sync.UpsertCursor(ctx, db, "ix", ts1, "success"); err != nil {
		t.Fatalf("UpsertCursor first: %v", err)
	}
	if err := sync.UpsertCursor(ctx, db, "ix", ts2, "success"); err != nil {
		t.Fatalf("UpsertCursor second: %v", err)
	}

	got, err := sync.GetCursor(ctx, db, "ix")
	if err != nil {
		t.Fatalf("GetCursor: %v", err)
	}
	if !got.Equal(ts2) {
		t.Errorf("expected updated timestamp %v, got %v", ts2, got)
	}
}

func TestGetCursor_IgnoresFailedStatus(t *testing.T) {
	t.Parallel()
	_, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()

	if err := sync.InitStatusTable(ctx, db); err != nil {
		t.Fatalf("InitStatusTable: %v", err)
	}

	ts := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	if err := sync.UpsertCursor(ctx, db, "fac", ts, "failed"); err != nil {
		t.Fatalf("UpsertCursor: %v", err)
	}

	got, err := sync.GetCursor(ctx, db, "fac")
	if err != nil {
		t.Fatalf("GetCursor: %v", err)
	}
	if !got.IsZero() {
		t.Errorf("expected zero time for failed cursor, got %v", got)
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

func TestGetCursor_DBError(t *testing.T) {
	t.Parallel()
	_, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()

	if err := sync.InitStatusTable(ctx, db); err != nil {
		t.Fatalf("InitStatusTable: %v", err)
	}

	db.Close()

	_, err := sync.GetCursor(ctx, db, "net")
	if err == nil {
		t.Fatal("expected error for closed DB, got nil")
	}
	if !strings.Contains(err.Error(), "get cursor for") {
		t.Errorf("error = %q, want substring %q", err.Error(), "get cursor for")
	}
}

func TestUpsertCursor_DBError(t *testing.T) {
	t.Parallel()
	_, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()

	if err := sync.InitStatusTable(ctx, db); err != nil {
		t.Fatalf("InitStatusTable: %v", err)
	}

	db.Close()

	err := sync.UpsertCursor(ctx, db, "net", time.Now(), "success")
	if err == nil {
		t.Fatal("expected error for closed DB, got nil")
	}
	if !strings.Contains(err.Error(), "upsert cursor for") {
		t.Errorf("error = %q, want substring %q", err.Error(), "upsert cursor for")
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

	_, err := sync.RecordSyncStart(ctx, db, time.Now())
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
	id, err := sync.RecordSyncStart(ctx, db, time.Now())
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
