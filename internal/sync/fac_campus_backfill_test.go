package sync_test

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"golang.org/x/time/rate"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/sync"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// makeMinimalCampus returns a JSON-friendly campus row. The updated
// value is older than any sync cursor, like the pending campuses that
// the mirror never saw in a ?since window.
func makeMinimalCampus(id, orgID int, status string) map[string]any {
	return map[string]any{
		"id":           id,
		"org_id":       orgID,
		"org_name":     "Org",
		"name":         fmt.Sprintf("Campus %d", id),
		"name_long":    nil,
		"aka":          nil,
		"website":      "",
		"social_media": []any{},
		"notes":        "",
		"country":      "US",
		"city":         "",
		"zipcode":      "",
		"state":        "",
		"logo":         nil,
		"created":      "2024-01-01T00:00:00Z",
		"updated":      "2024-01-01T00:00:00Z",
		"status":       status,
	}
}

// facWithCampus returns a facility row that references campusID.
func facWithCampus(id, orgID, campusID int) json.RawMessage {
	f := makeMinimalFac(id, orgID)
	f["campus_id"] = campusID
	return mustJSON(f)
}

// facCampusSyncResult is the state after syncFacCampusFixture.
type facCampusSyncResult struct {
	client *ent.Client
	rec    *batchedFetchRecorder
	log    string
}

// syncFacCampusFixture runs one full sync over a fake upstream with
// three facilities. Facs 10 and 11 reference campuses 64 and 114, which
// the bare /api/campus list does not return. Fac 12 references campus
// 5, which the bare list returns. campusRow, when not nil, answers the
// /api/campus?since=1&id__in= backfill request. When it is nil, the
// request returns no rows.
func syncFacCampusFixture(t *testing.T, fkCap int, campusRow func(int) json.RawMessage) facCampusSyncResult {
	t.Helper()
	const orgID = 1
	rowFn := map[string]func(int) json.RawMessage{}
	if campusRow != nil {
		rowFn["campus"] = campusRow
	}
	rec := newBatchedFetchRecorder()
	server := newBatchedTestServer(t, rec,
		map[string][]json.RawMessage{
			"org":    {orgJSON(orgID, "Org", "ok")},
			"campus": {mustJSON(makeMinimalCampus(5, orgID, "ok"))},
			"fac": {
				facWithCampus(10, orgID, 64),
				facWithCampus(11, orgID, 114),
				facWithCampus(12, orgID, 5),
			},
		},
		rowFn,
	)
	t.Cleanup(server.Close)

	client, log := fullSyncWithDebugLog(t, server.URL, fkCap)
	return facCampusSyncResult{client: client, rec: rec, log: log}
}

// fullSyncWithDebugLog runs one full sync against the fake upstream at
// url with the given FK backfill request cap. It returns the client and
// the DEBUG log of the cycle.
func fullSyncWithDebugLog(t *testing.T, url string, fkCap int) (*ent.Client, string) {
	t.Helper()
	client, db := testutil.SetupClientWithDB(t)
	pdbClient := peeringdb.NewClient(url, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)
	if err := sync.InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init: %v", err)
	}
	logs := &lockedWriter{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{
		FKBackfillMaxRequestsPerCycle: fkCap,
	}, logger)
	if err := w.Sync(t.Context(), "full"); err != nil {
		t.Fatalf("sync: %v", err)
	}
	return client, logs.String()
}

// facCampusOrphanLog is the per-row DEBUG line of recordOrphan for a
// fac whose campus_id was set to NULL.
func facCampusOrphanLog(facID, campusID int) string {
	return fmt.Sprintf(`msg="fk orphan" child_type=fac child_id=%d field=campus_id parent_type=campus orphan_parent_id=%d action=null`,
		facID, campusID)
}

// assertFacCampus checks the stored campus_id of a facility. want 0
// means NULL.
func assertFacCampus(t *testing.T, client *ent.Client, facID, want int) {
	t.Helper()
	fac, err := client.Facility.Get(t.Context(), facID)
	if err != nil {
		t.Fatalf("fac %d: %v (a missing campus must not drop the facility)", facID, err)
	}
	switch {
	case want == 0 && fac.CampusID != nil:
		t.Errorf("fac %d campus_id = %d, want NULL", facID, *fac.CampusID)
	case want != 0 && fac.CampusID == nil:
		t.Errorf("fac %d campus_id = NULL, want %d", facID, want)
	case want != 0 && *fac.CampusID != want:
		t.Errorf("fac %d campus_id = %d, want %d", facID, *fac.CampusID, want)
	}
}

// TestSync_BackfillsPendingFacCampus locks the backfill of a missing fac
// campus. Upstream keeps a campus pending while it has fewer than two
// facilities. A bare /api/campus list holds only ok campuses, and a
// ?since window holds a pending campus only when it changes, so the
// mirror never stored such a campus. Before the fix, the fac fkFilter
// set campus_id to NULL without a backfill, and every full cycle
// recorded the same orphans. The fac prefetchStaged pass now fetches the
// missing campuses in one batched request, and the facilities keep
// campus_id.
func TestSync_BackfillsPendingFacCampus(t *testing.T) {
	t.Parallel()

	res := syncFacCampusFixture(t, 5, func(id int) json.RawMessage {
		return mustJSON(makeMinimalCampus(id, 1, "pending"))
	})

	_, _, byType := res.rec.snapshot()
	if want := []string{"64,114"}; !slices.Equal(byType["campus"], want) {
		t.Errorf("campus backfill requests = %q, want %q (one batched request, no request for campus 5)",
			byType["campus"], want)
	}
	assertFacCampus(t, res.client, 10, 64)
	assertFacCampus(t, res.client, 11, 114)
	assertFacCampus(t, res.client, 12, 5)
	for _, id := range []int{64, 114} {
		c, err := res.client.Campus.Get(t.Context(), id)
		if err != nil {
			t.Fatalf("campus %d: %v (want it stored by backfill)", id, err)
		}
		if c.Status != "pending" {
			t.Errorf("campus %d status = %q, want pending", id, c.Status)
		}
	}
	if strings.Contains(res.log, `msg="fk orphan" child_type=fac`) {
		t.Errorf("fac orphan recorded, want none, log:\n%s", res.log)
	}
}

// TestSync_FacCampusNulledWhenBackfillOff locks the cap=0 escape hatch
// for the fac campus: no campus request, campus_id set to NULL, the
// orphan recorded with action null, and the facility kept.
func TestSync_FacCampusNulledWhenBackfillOff(t *testing.T) {
	t.Parallel()

	res := syncFacCampusFixture(t, 0, func(id int) json.RawMessage {
		return mustJSON(makeMinimalCampus(id, 1, "pending"))
	})

	if calls, idIns, _ := res.rec.snapshot(); calls != 0 {
		t.Errorf("backfill requests = %q, want none (cap 0 disables backfill)", idIns)
	}
	assertFacCampus(t, res.client, 10, 0)
	assertFacCampus(t, res.client, 11, 0)
	assertFacCampus(t, res.client, 12, 5)
	for _, line := range []string{facCampusOrphanLog(10, 64), facCampusOrphanLog(11, 114)} {
		if !strings.Contains(res.log, line) {
			t.Errorf("log has no %q, log:\n%s", line, res.log)
		}
	}
	if n, _ := res.client.Campus.Query().Count(t.Context()); n != 1 {
		t.Errorf("campusCount = %d, want 1 (only campus 5)", n)
	}
}

// TestSync_FacCampusNulledWhenAbsentUpstream locks the miss path: the
// prefetchStaged pass sends one batched campus request, upstream returns
// no row, and the fkFilter sets campus_id to NULL and records the
// orphan. The per-row path sends no second request for an id that the
// pass already tried.
func TestSync_FacCampusNulledWhenAbsentUpstream(t *testing.T) {
	t.Parallel()

	res := syncFacCampusFixture(t, 5, nil)

	_, _, byType := res.rec.snapshot()
	if want := []string{"64,114"}; !slices.Equal(byType["campus"], want) {
		t.Errorf("campus backfill requests = %q, want %q (the per-row path must not repeat it)",
			byType["campus"], want)
	}
	assertFacCampus(t, res.client, 10, 0)
	assertFacCampus(t, res.client, 11, 0)
	assertFacCampus(t, res.client, 12, 5)
	for _, line := range []string{facCampusOrphanLog(10, 64), facCampusOrphanLog(11, 114)} {
		if !strings.Contains(res.log, line) {
			t.Errorf("log has no %q, log:\n%s", line, res.log)
		}
	}
}

// TestSync_FacCampusBackfillSparesRequestCap locks the one-pass campus
// backfill. The missing campuses are in three fac chunks (scratch
// replays 100 rows per chunk), and a carrierfac references a missing
// carrier. With a cap of 2 requests, the campuses use one request and
// the carrier the other. A campus fetch for each chunk used 3 requests,
// so the cap stopped the carrier request and the carrierfac was dropped.
func TestSync_FacCampusBackfillSparesRequestCap(t *testing.T) {
	t.Parallel()

	const orgID = 1
	facCampus := map[int]int{1: 64, 150: 114, 250: 143}
	facs := make([]json.RawMessage, 0, 250)
	for id := 1; id <= 250; id++ {
		if campusID, ok := facCampus[id]; ok {
			facs = append(facs, facWithCampus(id, orgID, campusID))
			continue
		}
		facs = append(facs, mustJSON(makeMinimalFac(id, orgID)))
	}
	rec := newBatchedFetchRecorder()
	server := newBatchedTestServer(t, rec,
		map[string][]json.RawMessage{
			"org":        {orgJSON(orgID, "Org", "ok")},
			"fac":        facs,
			"carrierfac": {mustJSON(makeMinimalCarrierFac(1, 77, 1))},
		},
		map[string]func(int) json.RawMessage{
			"campus": func(id int) json.RawMessage {
				return mustJSON(makeMinimalCampus(id, orgID, "pending"))
			},
			"carrier": func(id int) json.RawMessage {
				return mustJSON(makeMinimalCarrier(id, orgID))
			},
		},
	)
	t.Cleanup(server.Close)

	client, log := fullSyncWithDebugLog(t, server.URL, 2)

	_, _, byType := rec.snapshot()
	if want := []string{"64,114,143"}; !slices.Equal(byType["campus"], want) {
		t.Errorf("campus backfill requests = %q, want %q (one request for all fac chunks)",
			byType["campus"], want)
	}
	if want := []string{"77"}; !slices.Equal(byType["carrier"], want) {
		t.Errorf("carrier backfill requests = %q, want %q", byType["carrier"], want)
	}
	for facID, campusID := range facCampus {
		assertFacCampus(t, client, facID, campusID)
	}
	if _, err := client.CarrierFacility.Get(t.Context(), 1); err != nil {
		t.Errorf("carrierfac 1: %v (want it stored after the carrier backfill)", err)
	}
	if strings.Contains(log, "fk backfill cap reached") {
		t.Errorf("request cap reached, want the campuses in one request, log:\n%s", log)
	}
}
