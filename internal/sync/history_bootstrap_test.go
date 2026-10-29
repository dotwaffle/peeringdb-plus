package sync

import (
	"database/sql"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/config"
)

func TestHistoryBootstrapAllStatuses(t *testing.T) {
	t.Parallel()
	hs := newHistoryServer(t)
	// The live cache is empty. The uncached feed includes an older live
	// row and a newer tombstone. A deleted-only fetch would lose row 1.
	hs.deleted["org"] = []map[string]any{
		bumpUpdated(makeOrg(1, "Live", "ok"), "2024-01-01T00:00:00Z"),
		bumpUpdated(makeOrg(9000, "Deleted", "deleted"), "2025-01-01T00:00:00Z"),
	}
	w, db, _ := newHistoryWorker(t, hs, 1)
	mustSync(t, w, config.SyncModeIncremental)
	assertOrg(t, w, 1, "ok", "2024-01-01T00:00:00Z")
	assertOrg(t, w, 9000, "deleted", "2025-01-01T00:00:00Z")
	assertWatermark(t, db, "org", "2025-01-01T00:00:00Z")
	if got, want := hs.sentWindows(), []string{"org?depth=0&id__gte=1&since=1"}; !slices.Equal(got, want) {
		t.Fatalf("windows %v, want %v", got, want)
	}
	if got := historyRows(t, db); !slices.Equal(got, []string{"org:1:1"}) {
		t.Fatalf("progress %v", got)
	}
}

// campusBootstrapWorker seeds only the live parent. Every other type is
// already checked, leaving the empty campus type as the next sweep step.
func campusBootstrapWorker(t *testing.T) (*historyServer, *Worker, *sql.DB) {
	t.Helper()
	hs := newHistoryServer(t)
	hs.setLive("org", makeOrg(1, "Parent", "ok"))
	w, db, _ := newHistoryWorker(t, hs, 0)
	mustSync(t, w, config.SyncModeIncremental)
	for _, name := range historySweepTypes {
		if name == "campus" {
			continue
		}
		if _, err := db.ExecContext(t.Context(), `INSERT INTO sync_history_sweep (type,next_id,done,updated_at) VALUES (?,1,1,?)`, name, time.Now().UTC().Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
	}
	w.config.HistoryMaxRequestsPerCycle = 1
	return hs, w, db
}

func TestHistoryBootstrapPendingCampus(t *testing.T) {
	t.Parallel()
	hs, w, db := campusBootstrapWorker(t)
	hs.deleted["campus"] = []map[string]any{
		bumpUpdated(bootstrapCampus(1, 1, "pending"), "2023-01-01T00:00:00Z"),
		bumpUpdated(bootstrapCampus(2, 1, "deleted"), "2025-01-01T00:00:00Z"),
	}
	mustSync(t, w, config.SyncModeIncremental)
	for _, id := range []int{1, 2} {
		if _, err := w.entClient.Campus.Get(t.Context(), id); err != nil {
			t.Fatalf("campus %d: %v", id, err)
		}
	}
	assertWatermark(t, db, "campus", "2025-01-01T00:00:00Z")
	if got := hs.sentWindows(); !slices.Equal(got, []string{"campus?depth=0&id__gte=1&since=1"}) {
		t.Fatalf("windows %v", got)
	}
	// A new worker resumes committed progress without resending the URL.
	resumed := NewWorker(newFastPDBClient(t, hs.srv.URL), w.entClient, db, w.config, w.logger)
	mustSync(t, resumed, config.SyncModeIncremental)
	if got := hs.sentWindows(); len(got) != 1 {
		t.Fatalf("repeated completed bootstrap: %v", got)
	}
}

func TestHistoryBootstrapRetry(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"partial response", "transaction rollback"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			hs, w, db := campusBootstrapWorker(t)
			hs.deleted["campus"] = []map[string]any{bootstrapCampus(1, 1, "pending")}
			if failure == "partial response" {
				hs.mu.Lock()
				hs.truncate = "campus"
				hs.mu.Unlock()
			} else {
				_, err := db.ExecContext(t.Context(), `CREATE TRIGGER history_abort BEFORE INSERT ON sync_history_sweep WHEN NEW.type='campus' BEGIN SELECT RAISE(ABORT,'bootstrap rollback'); END`)
				if err != nil {
					t.Fatal(err)
				}
			}
			err := w.Sync(t.Context(), config.SyncModeIncremental)
			if failure == "transaction rollback" && (err == nil || !strings.Contains(err.Error(), "bootstrap rollback")) {
				t.Fatalf("sync error %v", err)
			}
			if failure == "partial response" && err != nil {
				t.Fatal(err)
			}
			if got := hs.sentWindows(); len(got) != 1 {
				t.Fatalf("bootstrap not attempted: %v", got)
			}
			if n, err := w.entClient.Campus.Query().Count(t.Context()); err != nil || n != 0 {
				t.Fatalf("campuses %d, error %v", n, err)
			}
			assertWatermark(t, db, "campus", "")
			for _, row := range historyRows(t, db) {
				if strings.HasPrefix(row, "campus:") {
					t.Fatalf("failed window committed progress %s", row)
				}
			}
			if failure == "transaction rollback" {
				if _, err := db.ExecContext(t.Context(), `DROP TRIGGER history_abort`); err != nil {
					t.Fatal(err)
				}
			}
			hs.mu.Lock()
			hs.truncate = ""
			hs.mu.Unlock()
			mustSync(t, w, config.SyncModeIncremental)
			if got := hs.sentWindows(); len(got) != 1 {
				t.Fatalf("repeat protection failed: %v", got)
			}
			for key := range w.historyMemo {
				w.historyMemo[key] = time.Now().Add(-historyMemoTTL)
			}
			mustSync(t, w, config.SyncModeIncremental)
			if _, err := w.entClient.Campus.Get(t.Context(), 1); err != nil {
				t.Fatal(err)
			}
			assertWatermark(t, db, "campus", "2024-01-01T00:00:00Z")
		})
	}
}

func TestHistoryBootstrapStillEmpty(t *testing.T) {
	t.Parallel()
	hs, w, db := campusBootstrapWorker(t)
	mustSync(t, w, config.SyncModeIncremental)
	if got := hs.sentWindows(); len(got) != 1 {
		t.Fatalf("empty bootstrap not checked: %v", got)
	}
	resumed := NewWorker(newFastPDBClient(t, hs.srv.URL), w.entClient, db, w.config, w.logger)
	mustSync(t, resumed, config.SyncModeIncremental)
	if got := hs.sentWindows(); len(got) != 1 {
		t.Fatalf("restart repeated checked URL: %v", got)
	}
	hs.deleted["campus"] = []map[string]any{bootstrapCampus(1, 1, "pending")}
	for key := range resumed.historyMemo {
		resumed.historyMemo[key] = time.Now().Add(-historyMemoTTL)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE sync_history_sweep SET updated_at=?`, time.Now().Add(-historyMemoTTL).UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	mustSync(t, resumed, config.SyncModeIncremental)
	if _, err := resumed.entClient.Campus.Get(t.Context(), 1); err != nil {
		t.Fatalf("late campus not recovered: %v", err)
	}
	if got := hs.sentWindows(); len(got) != 2 {
		t.Fatalf("budget or retry violation: %v", got)
	}
}

func bootstrapCampus(id, orgID int, status string) map[string]any {
	return map[string]any{"id": id, "org_id": orgID, "name": "Campus", "status": status, "created": "2023-01-01T00:00:00Z", "updated": "2024-01-01T00:00:00Z"}
}

func TestHistoryBootstrapFairScheduling(t *testing.T) {
	t.Parallel()
	hs := newHistoryServer(t)
	w, db, _ := newHistoryWorker(t, hs, 1)
	expire := func() {
		t.Helper()
		old := time.Now().Add(-historyMemoTTL).UTC()
		for key := range w.historyMemo {
			w.historyMemo[key] = old
		}
		if _, err := db.ExecContext(t.Context(), `UPDATE sync_history_sweep SET updated_at=?`, old.Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
	}
	// Even when earlier empty checks have expired, each unfinished type
	// must get its first request within the one-request cycle budget.
	for i, name := range historySweepTypes {
		expire()
		mustSync(t, w, config.SyncModeIncremental)
		got := hs.sentWindows()
		if len(got) != i+1 || !strings.HasPrefix(got[i], name+"?") {
			t.Fatalf("cycle %d: windows %v, want next type %s", i, got, name)
		}
	}
	// After completion, oldest checks get priority. Recently refreshed
	// early types must not prevent later empty types from being checked.
	expire()
	for i, name := range historySweepTypes {
		mustSync(t, w, config.SyncModeIncremental)
		got := hs.sentWindows()
		at := len(historySweepTypes) + i
		if len(got) != at+1 || !strings.HasPrefix(got[at], name+"?") {
			t.Fatalf("refresh %d: windows %v, want next type %s", i, got, name)
		}
	}
}
