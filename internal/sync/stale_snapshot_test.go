package sync_test

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	stdsync "sync"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/config"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/sync"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// staleSnapshotServer stubs an upstream that serves the bare /api/org list
// from a stale API cache (snapshot) and ?since=N from the live rows with
// updated >= N. It records the since value of every first-page org
// request, "" for the bare list.
type staleSnapshotServer struct {
	*httptest.Server

	mu     stdsync.Mutex
	sinces []string
}

func newStaleSnapshotServer(t *testing.T, generated time.Time, snapshot, live []map[string]any, failSince bool) *staleSnapshotServer {
	t.Helper()
	s := &staleSnapshotServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		rows := []map[string]any{}
		if strings.TrimPrefix(r.URL.Path, "/api/") == "org" && (q.Get("skip") == "" || q.Get("skip") == "0") {
			since := q.Get("since")
			s.mu.Lock()
			s.sinces = append(s.sinces, since)
			s.mu.Unlock()
			switch {
			case since == "":
				rows = snapshot
			case failSince:
				http.Error(w, "boom", http.StatusInternalServerError)
				return
			default:
				n, err := strconv.ParseInt(since, 10, 64)
				if err != nil {
					http.Error(w, "bad since", http.StatusBadRequest)
					return
				}
				for _, row := range live {
					updated, _ := time.Parse(time.RFC3339, row["updated"].(string))
					if updated.Unix() >= n {
						rows = append(rows, row)
					}
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta": map[string]any{"generated": generated.Unix()},
			"data": rows,
		})
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *staleSnapshotServer) sinceValues() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.sinces)
}

// staleTestOrg returns an upstream org row named name.
func staleTestOrg(id int, name, status string, updated time.Time) map[string]any {
	row := tombstoneTestOrgRow(id, status, updated)
	row["name"] = name
	return row
}

// runStaleSnapshotSync runs one sync cycle in mode against server.
func runStaleSnapshotSync(t *testing.T, client *ent.Client, db *sql.DB, server *staleSnapshotServer, mode config.SyncMode) error {
	t.Helper()
	pdbClient := peeringdb.NewClient(server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)
	if err := sync.InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}
	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{}, slog.Default())
	return w.Sync(t.Context(), mode)
}

// Timeline shared by the stale-snapshot tests. Upstream built its API cache
// at cacheBuilt; the newest row in that snapshot is org 2 at snapshotMax.
// Org 1 changed upstream at changed, after the cache build, and org 3 was
// created at cursorAt, which is also the mirror's pre-cycle cursor.
var (
	orgOldAt    = time.Date(2026, 9, 22, 20, 0, 0, 0, time.UTC)
	snapshotMax = time.Date(2026, 9, 22, 22, 40, 0, 0, time.UTC)
	cacheBuilt  = time.Date(2026, 9, 22, 23, 14, 11, 0, time.UTC)
	changed     = time.Date(2026, 9, 23, 4, 5, 0, 0, time.UTC)
	cursorAt    = time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
)

// TestSync_FullModeRepairsFromStaleSnapshot reproduces the 2026-09-23 prod
// state. A full cycle over a stale upstream cache had rolled org 1 back to
// its pre-change version, including updated, and the cursor was already
// past the change, so a window from the cursor never returned the row. The
// window must start at the snapshot's newest updated so that the next full
// cycle repairs the row.
func TestSync_FullModeRepairsFromStaleSnapshot(t *testing.T) {
	t.Parallel()

	snapshot := []map[string]any{
		staleTestOrg(1, "Org1", "ok", orgOldAt),
		staleTestOrg(2, "Org2", "ok", snapshotMax),
	}
	live := []map[string]any{
		staleTestOrg(1, "Org1 New", "ok", changed),
		staleTestOrg(2, "Org2", "ok", snapshotMax),
		staleTestOrg(3, "Org3", "ok", cursorAt),
	}
	server := newStaleSnapshotServer(t, cacheBuilt, snapshot, live, false)

	client, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()
	for _, row := range []struct {
		id      int
		name    string
		updated time.Time
	}{{1, "Org1", orgOldAt}, {2, "Org2", snapshotMax}, {3, "Org3", cursorAt}} {
		if _, err := client.Organization.Create().
			SetID(row.id).SetName(row.name).
			SetCreated(orgOldAt).SetUpdated(row.updated).SetStatus("ok").
			Save(ctx); err != nil {
			t.Fatalf("seed org %d: %v", row.id, err)
		}
	}

	if err := runStaleSnapshotSync(t, client, db, server, config.SyncModeFull); err != nil {
		t.Fatalf("sync: %v", err)
	}

	want := []string{"", strconv.FormatInt(snapshotMax.Unix(), 10)}
	if got := server.sinceValues(); !slices.Equal(got, want) {
		t.Errorf("org since values = %q, want %q (bare list, then a window from the snapshot's newest updated)", got, want)
	}
	assertOrg1Current(t, client)
}

// assertOrg1Current fails t unless org 1 holds its post-cache version.
func assertOrg1Current(t *testing.T, client *ent.Client) {
	t.Helper()
	org1, err := client.Organization.Get(t.Context(), 1)
	if err != nil {
		t.Fatalf("get org 1: %v", err)
	}
	if org1.Name != "Org1 New" || !org1.Updated.Equal(changed) {
		t.Errorf("org 1 = (%q, %v), want (%q, %v)", org1.Name, org1.Updated, "Org1 New", changed)
	}
}

// TestSync_FullModeStaleSnapshotKeepsIncrementalRows locks the combined
// behavior on the path that caused the damage: a forced-full cycle over a
// stale cache does not undo the rows that an earlier cycle stored. The
// full cycle's window fetches org 1 again, so the upsert gate alone is not
// exercised here; TestSync_FullModeReconcilesLocallyDivergedRows covers it.
func TestSync_FullModeStaleSnapshotKeepsIncrementalRows(t *testing.T) {
	t.Parallel()

	snapshot := []map[string]any{
		staleTestOrg(1, "Org1", "ok", orgOldAt),
		staleTestOrg(2, "Org2", "ok", snapshotMax),
	}
	live := []map[string]any{
		staleTestOrg(1, "Org1 New", "ok", changed),
		staleTestOrg(2, "Org2", "ok", snapshotMax),
		staleTestOrg(3, "Org3", "ok", cursorAt),
	}
	server := newStaleSnapshotServer(t, cacheBuilt, snapshot, live, false)

	client, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()
	// First cycle: the bare list seeds the snapshot, and its window brings
	// org 1 and org 3 up to date.
	if err := runStaleSnapshotSync(t, client, db, server, config.SyncModeIncremental); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	assertOrg1Current(t, client)
	if n, err := client.Organization.Query().Count(ctx); err != nil || n != 3 {
		t.Fatalf("after first sync: org count = %d (err %v), want 3", n, err)
	}
	// Second cycle: the forced-full escalation over the same stale cache.
	if err := runStaleSnapshotSync(t, client, db, server, config.SyncModeFull); err != nil {
		t.Fatalf("full sync: %v", err)
	}

	assertOrg1Current(t, client)
	if n, err := client.Organization.Query().Count(ctx); err != nil || n != 3 {
		t.Errorf("org count = %d (err %v), want 3", n, err)
	}
}

// TestSync_ZeroCursorWindowFailureTolerated asserts that on an empty table
// a failed snapshot window does not fail the cycle. Committing the snapshot
// sets the cursor to the snapshot's newest updated, so the next cycle's
// ?since= fetch is the same window.
func TestSync_ZeroCursorWindowFailureTolerated(t *testing.T) {
	t.Parallel()

	for _, mode := range []config.SyncMode{config.SyncModeFull, config.SyncModeIncremental} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			snapshot := []map[string]any{staleTestOrg(2, "Org2", "ok", snapshotMax)}
			server := newStaleSnapshotServer(t, cacheBuilt, snapshot, nil, true)

			client, db := testutil.SetupClientWithDB(t)
			if err := runStaleSnapshotSync(t, client, db, server, mode); err != nil {
				t.Fatalf("sync failed on a zero-cursor window error: %v", err)
			}
			// The client retries the failed window request, so compact
			// the repeats.
			want := []string{"", strconv.FormatInt(snapshotMax.Unix(), 10)}
			if got := slices.Compact(server.sinceValues()); !slices.Equal(got, want) {
				t.Errorf("org since values = %q, want %q", got, want)
			}
			if _, err := client.Organization.Get(t.Context(), 2); err != nil {
				t.Errorf("snapshot row not committed: %v", err)
			}
		})
	}
}

// TestSync_FullModeRepairsRevertedRow covers an upstream change that
// moves updated backwards: the IX-F import-log rollback reverts a row
// through django-reversion, which saves the old version, old updated
// included. Incremental cycles never see the revert. A full cycle must
// repair the row, because the stored version is older than the
// snapshot's newest row and so predates the snapshot.
func TestSync_FullModeRepairsRevertedRow(t *testing.T) {
	t.Parallel()

	editedAt := orgOldAt.Add(time.Hour)
	upstream := []map[string]any{
		staleTestOrg(1, "Org1", "ok", orgOldAt), // reverted to its old version
		staleTestOrg(2, "Org2", "ok", snapshotMax),
	}
	server := newStaleSnapshotServer(t, cacheBuilt, upstream, upstream, false)

	client, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()
	for _, row := range []struct {
		id      int
		name    string
		updated time.Time
	}{{1, "Org1 Edited", editedAt}, {2, "Org2", snapshotMax}} {
		if _, err := client.Organization.Create().
			SetID(row.id).SetName(row.name).
			SetCreated(orgOldAt).SetUpdated(row.updated).SetStatus("ok").
			Save(ctx); err != nil {
			t.Fatalf("seed org %d: %v", row.id, err)
		}
	}

	if err := runStaleSnapshotSync(t, client, db, server, config.SyncModeFull); err != nil {
		t.Fatalf("sync: %v", err)
	}

	org1, err := client.Organization.Get(ctx, 1)
	if err != nil {
		t.Fatalf("get org 1: %v", err)
	}
	if org1.Name != "Org1" || !org1.Updated.Equal(orgOldAt) {
		t.Errorf("org 1 = (%q, %v), want the reverted (%q, %v)", org1.Name, org1.Updated, "Org1", orgOldAt)
	}
}
