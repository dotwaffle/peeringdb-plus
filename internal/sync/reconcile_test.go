package sync_test

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"

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
// Full-mode cycles now carry the reconcile-all marker that disables the
// gate, so the snapshot rewrites the row; incremental cycles keep the
// optimization (and its documented bounded same-second-drift).
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

	run := func(t *testing.T, mode config.SyncMode, wantName string) {
		t.Helper()
		client, db := testutil.SetupClientWithDB(t)
		ctx := t.Context()
		// Local copy diverged from upstream with the SAME updated value
		// (the orphan-filter FK-null shape: local mutation, no bump).
		if _, err := client.Organization.Create().
			SetID(1).SetName("Locally Diverged").
			SetCreated(seeded).SetUpdated(seeded).SetStatus("ok").
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
	}

	t.Run("full reconciles", func(t *testing.T) {
		t.Parallel()
		run(t, config.SyncModeFull, upstreamName)
	})
	t.Run("incremental keeps skip gate", func(t *testing.T) {
		t.Parallel()
		// Incremental with an equal updated value skips the rewrite —
		// the deliberate optimization (strict >, not >=).
		run(t, config.SyncModeIncremental, "Locally Diverged")
	})
}
