// replay_test.go is a one-shot offline replay harness: it serves the
// files in /tmp/claude-1000/pdb-snapshot/*.json as a fake PeeringDB API
// and runs Worker.Sync against a fresh in-memory database.
//
// Goal: prove that the v1.13 upsert-time FK orphan filter lets sync
// commit cleanly against real api.peeringdb.com data that contains 23
// structural orphans (nets/carriers/facilities/carrierfacs/ixprefixes
// pointing at server-side-suppressed parents).
//
// Usage:
//
//	TMPDIR=/tmp/claude-1000 go test -run TestReplaySnapshot \
//	    -tags offline_replay ./internal/sync/
//
// The test is behind a build tag so it never runs in CI and never
// touches api.peeringdb.com. It only reads the captured files.

//go:build offline_replay

package sync

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/time/rate"

	"github.com/dotwaffle/peeringdb-plus/internal/config"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestReplaySnapshot runs a full sync against the captured real-world
// PeeringDB snapshot and asserts commit-time FK integrity holds.
func TestReplaySnapshot(t *testing.T) {
	snapshotDir := "/tmp/claude-1000/pdb-snapshot"
	if _, err := os.Stat(snapshotDir); err != nil {
		t.Skipf("snapshot dir %s missing: %v", snapshotDir, err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// URL shape: /api/{type}?depth=0
		path := strings.TrimPrefix(r.URL.Path, "/api/")
		objType := strings.Split(path, "?")[0]
		file := filepath.Join(snapshotDir, objType+".json")
		data, err := os.ReadFile(file)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)

	client, db := testutil.SetupClientWithDB(t)
	if err := InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	pdbClient := peeringdb.NewClient(srv.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)

	worker := NewWorker(pdbClient, client, db, WorkerConfig{
		IncludeDeleted: true,
		SyncMode:       config.SyncModeFull,
		IsPrimary:      func() bool { return true },
	}, slog.Default())

	if err := worker.Sync(t.Context(), config.SyncModeFull); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// Spot check: row counts should match expectation within orphan drift.
	ctx := t.Context()
	orgs, _ := client.Organization.Query().Count(ctx)
	nets, _ := client.Network.Query().Count(ctx)
	carriers, _ := client.Carrier.Query().Count(ctx)
	facs, _ := client.Facility.Query().Count(ctx)
	ixpfxs, _ := client.IxPrefix.Query().Count(ctx)
	carrierfacs, _ := client.CarrierFacility.Query().Count(ctx)

	t.Logf("row counts: orgs=%d nets=%d carriers=%d facs=%d ixpfxs=%d carrierfacs=%d",
		orgs, nets, carriers, facs, ixpfxs, carrierfacs)

	// Expected orphans dropped from snapshot:
	//   net.org_id → org        : 2 orphans (ids 27698, 27764)
	//   carrier.org_id → org    : 1 orphan (id 403 NTT America)
	//   ixpfx.ixlan_id → ixlan  : 1 orphan
	//   carrierfac.fac_id → fac : 10 orphans
	//   fac.campus_id → campus  : 9 orphans (but campus_id is nullable,
	//                              facility row is kept with null)
	//
	// Total kept rows should equal fetched-total minus the ones above
	// (except fac: all kept because campus_id is nullable).
	//
	// If the counts don't match, we'll figure out why.
}
