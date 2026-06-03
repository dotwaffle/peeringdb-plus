package sync

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/config"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// Log-level lock for the reviewed slog levels.
//
// Log lines demoted in `internal/sync/worker.go`:
//
//   - "fetching"             INFO → DEBUG
//   - "upserted"              INFO → DEBUG
//   - "marked stale deleted" INFO → DEBUG
//   - "failed to get cursor, using full sync" WARN → INFO
//   - "sync rate-limited, deferring to next scheduled tick" WARN → INFO
//   - "sync rate-limited during retry, deferring" WARN → INFO
//   - "failed to get last sync time" WARN → DEBUG
//
// Security signals explicitly preserved (locked by acceptance criteria):
//
//   - "fk orphans summary" WARN (when total>0)
//   - "heap threshold crossed" WARN
//   - "incremental sync failed, falling back to full" WARN
//   - "sync failed after all retries" ERROR
//   - "demoted during sync, aborting cycle" WARN
//
// These tests drive a single full sync cycle through the existing fixture
// harness and assert level placement on the captured slog records.

// captureSyncLogs runs a single full Sync cycle with all 13 types empty and
// returns the captured slog output at the requested handler level. The
// handler level acts as a filter — passing slog.LevelDebug captures all
// records, slog.LevelInfo filters DEBUG out (matching the production
// stdout handler).
func captureSyncLogs(t *testing.T, handlerLevel slog.Level) string {
	t.Helper()
	ctx := t.Context()

	f := newFixture(t)
	// Seed only org with one row; remaining 12 types return empty data.
	// All 13 fetch+upsert closures still fire and produce "fetching" /
	// "upserted" log lines.
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}

	client, db := testutil.SetupClientWithDB(t)
	if err := InitStatusTable(ctx, db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: handlerLevel}))
	pdbClient := newFastPDBClient(t, f.server.URL)

	w := NewWorker(pdbClient, client, db, WorkerConfig{
		IsPrimary: func() bool { return true },
		SyncMode:  config.SyncModeFull,
	}, logger)

	if err := w.Sync(ctx, config.SyncModeFull); err != nil {
		t.Fatalf("sync: %v", err)
	}
	return buf.String()
}

// TestSyncLogLevels_FetchingIsDebug verifies that the per-type "fetching"
// line emits at DEBUG and is suppressed at INFO.
func TestSyncLogLevels_FetchingIsDebug(t *testing.T) {
	t.Parallel()

	out := captureSyncLogs(t, slog.LevelDebug)

	if !strings.Contains(out, `"msg":"fetching"`) {
		t.Fatalf("expected at least one fetching log; output:\n%s", out)
	}
	// Every fetching record must carry level=DEBUG.
	if strings.Contains(out, `"level":"INFO","msg":"fetching"`) {
		t.Errorf("'fetching' must be DEBUG, found INFO; output:\n%s", out)
	}
	if !strings.Contains(out, `"level":"DEBUG","msg":"fetching"`) {
		t.Errorf("'fetching' must be DEBUG; output:\n%s", out)
	}
}

// TestSyncLogLevels_UpsertedIsDebug verifies that the per-type "upserted"
// line emits at DEBUG and is suppressed at INFO.
func TestSyncLogLevels_UpsertedIsDebug(t *testing.T) {
	t.Parallel()

	out := captureSyncLogs(t, slog.LevelDebug)

	if !strings.Contains(out, `"msg":"upserted"`) {
		t.Fatalf("expected at least one upserted log; output:\n%s", out)
	}
	if strings.Contains(out, `"level":"INFO","msg":"upserted"`) {
		t.Errorf("'upserted' must be DEBUG, found INFO; output:\n%s", out)
	}
	if !strings.Contains(out, `"level":"DEBUG","msg":"upserted"`) {
		t.Errorf("'upserted' must be DEBUG; output:\n%s", out)
	}
}

// TestSyncLogLevels_DefaultINFOFiltersDebug locks the operator contract:
// at handler-level INFO (the production stdout default), the per-step
// DEBUG records are not emitted to stdout. This is the volume-reduction
// gate — if "fetching" or "upserted" leak through INFO, Loki ingestion
// is unchanged.
func TestSyncLogLevels_DefaultINFOFiltersDebug(t *testing.T) {
	t.Parallel()

	out := captureSyncLogs(t, slog.LevelInfo)

	if strings.Contains(out, `"msg":"fetching"`) {
		t.Errorf("at handler INFO, 'fetching' must be filtered out; output:\n%s", out)
	}
	if strings.Contains(out, `"msg":"upserted"`) {
		t.Errorf("at handler INFO, 'upserted' must be filtered out; output:\n%s", out)
	}
	// Sanity: the per-cycle "sync complete" INFO summary still emits at
	// INFO — it is explicitly preserved.
	if !strings.Contains(out, `"msg":"sync complete"`) {
		t.Errorf("'sync complete' must remain INFO; output:\n%s", out)
	}
}

// TestSyncLogLevels_FKOrphansSummaryStaysWarn locks the security-signal
// preservation invariant: when fk orphan rows are observed in a cycle,
// the per-cycle aggregate MUST fire at WARN. Demoting this would
// re-introduce the data-integrity blind spot the per-row →
// per-cycle refactor solved (and re-breach Tempo's 7.5 MB cap).
//
// Driven directly via recordOrphan + emitOrphanSummary because the FK
// fixture wiring through Sync is heavyweight and the security invariant
// is the level placement, not the orphan-detection codepath.
func TestSyncLogLevels_FKOrphansSummaryStaysWarn(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	w := &Worker{logger: logger}
	w.resetFKState()

	ctx := context.Background()
	w.recordOrphan(ctx, fkOrphanKey{ChildType: "carrierfac", ParentType: "fac", Field: "fac_id", Action: "drop"}, 100, 200)
	w.recordOrphan(ctx, fkOrphanKey{ChildType: "fac", ParentType: "campus", Field: "campus_id", Action: "null"}, 300, 400)

	buf.Reset()
	w.emitOrphanSummary(ctx)

	out := buf.String()
	if !strings.Contains(out, `"level":"WARN","msg":"fk orphans summary"`) {
		t.Errorf("'fk orphans summary' (total>0) must be WARN; output:\n%s", out)
	}
	if !strings.Contains(out, `"total":2`) {
		t.Errorf("expected total=2 in summary; output:\n%s", out)
	}
}
