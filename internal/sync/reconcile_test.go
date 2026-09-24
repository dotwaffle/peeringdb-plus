package sync_test

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/dotwaffle/peeringdb-plus/ent/migrate"
	"github.com/dotwaffle/peeringdb-plus/internal/config"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/sync"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestSync_FullModeReconcilesLocallyDivergedRows locks the 2026-06-10
// audit fix: the upsert skip gate (excluded.updated > updated) skipped
// rows whose local copy diverged from upstream WITHOUT an updated bump —
// e.g. FKs nulled by the orphan filter — on every cycle INCLUDING the
// daily forced-full whose documented purpose is complete reconciliation.
// Full-mode cycles now carry the reconcile-all marker that relaxes the
// gate to `>=`, so the snapshot rewrites the row; incremental cycles keep
// the optimization (and its documented bounded same-second-drift). A
// full-mode snapshot never rewrites a row whose stored updated is newer:
// upstream serves that snapshot from a cache that can be stale.
func TestSync_FullModeReconcilesLocallyDivergedRows(t *testing.T) {
	t.Parallel()

	seeded := time.Date(2026, 4, 1, 6, 0, 0, 0, time.UTC)
	upstreamName := "Upstream Truth"

	orgPayload := fmt.Sprintf(`{"meta":{},"data":[{"id":1,"name":%q,"aka":"","name_long":"","website":"","social_media":[],"notes":"","address1":"","address2":"","city":"","state":"","country":"US","zipcode":"","suite":"","floor":"","created":"2026-04-01T06:00:00Z","updated":%q,"status":"ok"}]}`,
		upstreamName, seeded.Format(time.RFC3339))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		typeName := strings.TrimPrefix(r.URL.Path, "/api/")
		skip := r.URL.Query().Get("skip")
		if typeName != "org" || (skip != "" && skip != "0") || r.URL.Query().Get("since") != "" {
			_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
			return
		}
		_, _ = w.Write([]byte(orgPayload))
	}))
	// t.Cleanup, not defer: the sub-tests are parallel, so the parent
	// function returns before they run — a defer would close the stub
	// server out from under them.
	t.Cleanup(server.Close)

	run := func(t *testing.T, mode config.SyncMode, localName string, localUpdated time.Time, wantName string) {
		t.Helper()
		client, db := testutil.SetupClientWithDB(t)
		ctx := t.Context()
		if _, err := client.Organization.Create().
			SetID(1).SetName(localName).
			SetCreated(seeded).SetUpdated(localUpdated).SetStatus("ok").
			Save(ctx); err != nil {
			t.Fatalf("seed org: %v", err)
		}

		pdbClient := peeringdb.NewClient(server.URL, slog.Default())
		pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
		pdbClient.SetRetryBaseDelay(0)
		if err := sync.InitStatusTable(ctx, db); err != nil {
			t.Fatalf("init status table: %v", err)
		}
		w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{}, slog.Default())
		if err := w.Sync(ctx, mode); err != nil {
			t.Fatalf("sync: %v", err)
		}

		org, err := client.Organization.Get(ctx, 1)
		if err != nil {
			t.Fatalf("get org: %v", err)
		}
		if org.Name != wantName {
			t.Errorf("mode=%s: org name = %q, want %q", mode, org.Name, wantName)
		}
		if wantName == localName && !org.Updated.Equal(localUpdated) {
			t.Errorf("mode=%s: org updated = %v, want the stored %v", mode, org.Updated, localUpdated)
		}
	}

	// The diverged cases store the SAME updated value as upstream (the
	// orphan-filter FK-null shape: local mutation, no bump).
	t.Run("full reconciles", func(t *testing.T) {
		t.Parallel()
		run(t, config.SyncModeFull, "Locally Diverged", seeded, upstreamName)
	})
	t.Run("incremental keeps skip gate", func(t *testing.T) {
		t.Parallel()
		// Incremental with an equal updated value skips the rewrite —
		// the deliberate optimization (strict >, not >=).
		run(t, config.SyncModeIncremental, "Locally Diverged", seeded, "Locally Diverged")
	})
	t.Run("full keeps newer stored row", func(t *testing.T) {
		t.Parallel()
		// The stored row is newer than the snapshot's version, for
		// example from an incremental cycle after upstream built its
		// API cache. The window fetch here returns nothing, so only
		// the gate keeps the row from rolling back.
		run(t, config.SyncModeFull, "Locally Newer", seeded.Add(time.Hour), "Locally Newer")
	})
}

// TestSync_FullModeWritesOnlyChangedRows runs two full cycles over the
// same fixtures and counts the rows that the second cycle updates.
// Full mode passes rows with an equal updated through the gate, but it
// writes only a row where a column differs: rewriting an unchanged row
// writes the index pages of its table, and each replica applies them with
// the WAL locks held. A row that changed without an updated bump is still
// written.
func TestSync_FullModeWritesOnlyChangedRows(t *testing.T) {
	t.Parallel()
	fs := newFixtureServer(t)
	client, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()

	pdbClient := peeringdb.NewClient(fs.server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)
	if err := sync.InitStatusTable(ctx, db); err != nil {
		t.Fatalf("init status table: %v", err)
	}
	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{}, slog.Default())
	if err := w.Sync(ctx, config.SyncModeFull); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Log each row update of the entity tables.
	if _, err := db.ExecContext(ctx, `CREATE TABLE update_log (tbl TEXT NOT NULL)`); err != nil {
		t.Fatalf("create update_log: %v", err)
	}
	for _, tb := range migrate.Tables {
		q := fmt.Sprintf(`CREATE TRIGGER log_update_%[1]s AFTER UPDATE ON %[1]s
			BEGIN INSERT INTO update_log (tbl) VALUES ('%[1]s'); END`, tb.Name)
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatalf("create trigger on %s: %v", tb.Name, err)
		}
	}
	updates := func(t *testing.T) map[string]int {
		t.Helper()
		rows, err := db.QueryContext(ctx, `SELECT tbl, COUNT(*) FROM update_log GROUP BY tbl`)
		if err != nil {
			t.Fatalf("read update_log: %v", err)
		}
		defer func() { _ = rows.Close() }()
		got := map[string]int{}
		for rows.Next() {
			var tbl string
			var n int
			if err := rows.Scan(&tbl, &n); err != nil {
				t.Fatalf("scan update_log: %v", err)
			}
			got[tbl] = n
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("read update_log: %v", err)
		}
		if _, err := db.ExecContext(ctx, `DELETE FROM update_log`); err != nil {
			t.Fatalf("clear update_log: %v", err)
		}
		return got
	}

	if err := w.Sync(ctx, config.SyncModeFull); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if got := updates(t); len(got) != 0 {
		t.Errorf("unchanged full cycle updated rows: %v, want none", got)
	}

	// Rename org 1 upstream without an updated bump.
	var orgs []map[string]any
	if err := json.Unmarshal(fs.fixtures["org"], &orgs); err != nil {
		t.Fatalf("decode org fixture: %v", err)
	}
	orgs[0]["name"] = "Renamed Organization"
	renamed, err := json.Marshal(orgs)
	if err != nil {
		t.Fatalf("encode org fixture: %v", err)
	}
	fs.setFixtureData("org", renamed)
	if err := w.Sync(ctx, config.SyncModeFull); err != nil {
		t.Fatalf("third sync: %v", err)
	}
	if got, want := updates(t), map[string]int{"organizations": 1}; !maps.Equal(got, want) {
		t.Errorf("full cycle after a rename updated %v, want %v", got, want)
	}
	if name := client.Organization.GetX(ctx, 1).Name; name != "Renamed Organization" {
		t.Errorf("org 1 name = %q, want the renamed value", name)
	}
}
