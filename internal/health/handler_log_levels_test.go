package health_test

import (
	"database/sql"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/health"
	_ "modernc.org/sqlite"
)

// Phase 77 OBS-06 — log-level lock for the post-AUDIT.md slog levels in
// the health handler.
//
// AUDIT.md rows demoted in `internal/health/handler.go`:
//
//   - L123 "readyz no sync completed" (default branch)  WARN → DEBUG
//   - L148 "readyz no sync completed" (running branch)  WARN → DEBUG
//
// Rationale: Fly hits /readyz every ~15s × 8 machines during the
// pre-first-sync window (5–15 min cold start). The 503 response already
// drives Fly proxy failover — the WARN log is non-actionable noise that
// masked real WARNs in operator-grep windows.
//
// Security-signal rows explicitly KEPT at WARN/ERROR:
//
//   - L90, L114, L140 "readyz db probe failed" / "readyz sync lookup failed" — ERROR
//   - L157 "readyz sync marked failed" — WARN
//   - L166 "readyz unknown sync status" — WARN
//   - L181 "readyz sync stale" — WARN

// runReadyz invokes the readiness handler against the supplied DB and
// returns the HTTP status and the captured slog records.
func runReadyz(t *testing.T, db *sql.DB) (int, []slog.Record) {
	t.Helper()

	logCap := &testLogHandler{}
	handler := health.ReadinessHandler(health.ReadinessInput{
		DB:             db,
		StaleThreshold: 24 * time.Hour,
		Logger:         slog.New(logCap),
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	handler.ServeHTTP(rec, req)
	return rec.Code, logCap.snapshot()
}

// TestHealth_NoSyncCompletedIsDebug locks the AUDIT.md L123 demotion:
// when GetLastStatus returns nil (no sync row yet, pre-first-sync window),
// the readyz handler MUST log at DEBUG, not WARN.
func TestHealth_NoSyncCompletedIsDebug(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	initSyncTable(t, db)
	// No sync rows inserted — GetLastStatus returns (nil, nil).

	status, records := runReadyz(t, db)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", status, http.StatusServiceUnavailable)
	}

	// Locate the "readyz no sync completed" record.
	var found bool
	for _, r := range records {
		if r.Message != "readyz no sync completed" {
			continue
		}
		found = true
		if r.Level == slog.LevelWarn {
			t.Errorf("AUDIT.md L123: 'readyz no sync completed' must be DEBUG, found WARN")
		}
		if r.Level != slog.LevelDebug {
			t.Errorf("AUDIT.md L123: 'readyz no sync completed' must be DEBUG, got %s", r.Level)
		}
	}
	if !found {
		t.Errorf("expected 'readyz no sync completed' record; got %d records", len(records))
	}
}

// TestHealth_NoSyncCompletedIsDebug_RunningBranch locks the AUDIT.md L148
// demotion: when the latest sync_status row is "running" but no completed
// row exists, the readyz handler MUST log at DEBUG, not WARN.
func TestHealth_NoSyncCompletedIsDebug_RunningBranch(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	initSyncTable(t, db)
	insertRunningSync(t, db, time.Now())

	status, records := runReadyz(t, db)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", status, http.StatusServiceUnavailable)
	}

	var found bool
	for _, r := range records {
		if r.Message != "readyz no sync completed" {
			continue
		}
		found = true
		if r.Level == slog.LevelWarn {
			t.Errorf("AUDIT.md L148: 'readyz no sync completed' (running branch) must be DEBUG, found WARN")
		}
		if r.Level != slog.LevelDebug {
			t.Errorf("AUDIT.md L148: 'readyz no sync completed' (running branch) must be DEBUG, got %s", r.Level)
		}
	}
	if !found {
		t.Errorf("expected 'readyz no sync completed' record on running branch; got %d records", len(records))
	}
}

// insertRunningSync inserts a sync_status row in 'running' state with no
// completion timestamp. Used by the L148 running-branch test.
func insertRunningSync(t *testing.T, db *sql.DB, startedAt time.Time) {
	t.Helper()
	_, err := db.ExecContext(t.Context(),
		`INSERT INTO sync_status (started_at, completed_at, duration_ms, object_counts, status, error_message) VALUES (?, NULL, NULL, ?, ?, ?)`,
		startedAt, `{}`, "running", "",
	)
	if err != nil {
		t.Fatalf("inserting running sync status: %v", err)
	}
}
