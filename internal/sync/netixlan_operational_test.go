package sync_test

import (
	"log/slog"
	"testing"

	"golang.org/x/time/rate"

	"github.com/dotwaffle/peeringdb-plus/internal/config"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/sync"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestSync_NetixlanOperational locks the handling of the netixlan
// operational flag. PeeringDB 2.83.0 derives it from status on save
// (models.py:6512) and keeps it only for a deprecation window. A sent
// value is stored as sent; when upstream omits the key, sync derives
// operational = (status == "ok").
func TestSync_NetixlanOperational(t *testing.T) {
	t.Parallel()

	row := func(id int, ip, status string, operational any, sendKey bool) map[string]any {
		r := metaTestNetIxLan("2026-04-01T00:00:00Z", nil)
		r["id"] = id
		r["ipaddr4"] = ip
		r["status"] = status
		delete(r, "meta")
		delete(r, "operational")
		if sendKey {
			r["operational"] = operational
		}
		return r
	}
	rows := metaTestRows()
	rows["net"] = []any{metaTestNet(1, 65001, "2026-04-01T00:00:00Z", nil)}
	rows["netixlan"] = []any{
		row(10, "192.0.2.10", "ok", true, true),
		row(11, "192.0.2.11", "ok", false, true),
		row(12, "192.0.2.12", "ok", nil, false),
		row(13, "192.0.2.13", "not-operational", nil, false),
	}
	srv := newMetaTestServer(t, rows)

	client, db := testutil.SetupClientWithDB(t)
	pdbClient := peeringdb.NewClient(srv.server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)
	if err := sync.InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}
	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{}, slog.Default())
	ctx := t.Context()
	if err := w.Sync(ctx, config.SyncModeIncremental); err != nil {
		t.Fatalf("sync: %v", err)
	}

	for id, want := range map[int]bool{
		10: true,  // sent true
		11: false, // sent false, stored as sent
		12: true,  // omitted, status ok
		13: false, // omitted, status not-operational
	} {
		nixl, err := client.NetworkIxLan.Get(ctx, id)
		if err != nil {
			t.Fatalf("get netixlan %d: %v", id, err)
		}
		if nixl.Operational != want {
			t.Errorf("netixlan %d operational = %v, want %v", id, nixl.Operational, want)
		}
	}
}
