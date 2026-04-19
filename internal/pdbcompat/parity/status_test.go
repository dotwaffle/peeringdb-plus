package parity

import (
	"net/http"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// TestParity_Status locks the upstream rest.py:694-727 status × since
// matrix (Phase 68 D-05/D-07) against future regression. Each subtest
// covers one cell of the matrix:
//
//	{no since, since=N} × {list, pk-lookup} × {campus, non-campus}
//
// The campus row carries the rest.py:712-715 carve-out where
// status="pending" is admitted on `since>0` list queries (the IXP
// onboarding workflow expects pending campuses to surface to syncing
// clients within the cycle window).
//
// upstream: peeringdb_server/rest.py:694-727 (status × since matrix)
// upstream: pdb_api_test.py (multiple sites; admission rules are
// implicit in fixture-mix expectations across the test corpus).
func TestParity_Status(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)

	// seedNet: distinct id/asn/status/updated. Each subtest owns its
	// own ent client so seeding helpers can share an ID space without
	// conflict.
	seedNet := func(t *testing.T, c *ent.Client, id, asn int, status string, updated time.Time) {
		t.Helper()
		if _, err := c.Network.Create().
			SetID(id).SetName("StatusNet").SetNameFold(unifold.Fold("StatusNet")).
			SetAsn(asn).SetStatus(status).
			SetCreated(t0).SetUpdated(updated).
			Save(t.Context()); err != nil {
			t.Fatalf("seed net id=%d: %v", id, err)
		}
	}
	seedCampus := func(t *testing.T, c *ent.Client, id int, status string, updated time.Time) {
		t.Helper()
		ctx := t.Context()
		if n, _ := c.Organization.Query().Count(ctx); n == 0 {
			if _, err := c.Organization.Create().
				SetID(1).SetName("CampusParent").SetNameFold(unifold.Fold("CampusParent")).
				SetStatus("ok").SetCreated(t0).SetUpdated(t0).
				Save(ctx); err != nil {
				t.Fatalf("seed org: %v", err)
			}
		}
		if _, err := c.Campus.Create().
			SetID(id).SetName("StatusCampus").SetNameFold(unifold.Fold("StatusCampus")).
			SetOrgID(1).SetCity("Berlin").SetCountry("DE").
			SetStatus(status).
			SetCreated(t0).SetUpdated(updated).
			Save(ctx); err != nil {
			t.Fatalf("seed campus id=%d: %v", id, err)
		}
	}

	t.Run("STATUS-01_list_no_since_status_ok_only", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:694-700 (default branch — list filters to
		// status=ok unconditionally without ?since)
		// upstream: pdb_api_test.py:5081 (list endpoint default-mix
		// expectations)
		c := testutil.SetupClient(t)
		seedNet(t, c, 1, 64501, "ok", t0)
		seedNet(t, c, 2, 64502, "pending", t0.Add(1*time.Hour))
		seedNet(t, c, 3, 64503, "deleted", t0.Add(2*time.Hour))

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/net")
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		ids := extractIDs(t, body)
		if len(ids) != 1 || ids[0] != 1 {
			t.Errorf("STATUS-01 list w/o since: got ids %v, want [1] (only status=ok)", ids)
		}
	})

	t.Run("STATUS-02_pk_lookup_admits_pending", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:702-710 (pk-lookup branch admits pending)
		// upstream: pdb_api_test.py:1242 (pk lookup on a pending row
		// returns 200 — the row is visible by direct ID even if hidden
		// from list responses).
		c := testutil.SetupClient(t)
		seedNet(t, c, 20, 64520, "pending", t0)

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/net/20")
		if status != http.StatusOK {
			t.Errorf("STATUS-02 pk lookup on pending: got %d, want 200; body=%s",
				status, string(body))
		}
	})

	t.Run("STATUS-03_pk_lookup_deleted_returns_404", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:702-710 (pk-lookup branch excludes deleted)
		// upstream: pdb_api_test.py:1247 (deleted row pk lookup → 404)
		c := testutil.SetupClient(t)
		seedNet(t, c, 30, 64530, "deleted", t0)

		srv := newTestServer(t, c)
		status, _ := httpGet(t, srv, "/api/net/30")
		if status != http.StatusNotFound {
			t.Errorf("STATUS-03 pk lookup on deleted: got %d, want 404", status)
		}
	})

	t.Run("STATUS-04_list_since_admits_deleted_excludes_pending_noncampus", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:712-715 (since>0 admits ok+deleted, excludes
		// pending — except for campus carve-out)
		// upstream: pdb_api_test.py:1317 (since-window list assertion)
		c := testutil.SetupClient(t)
		seedNet(t, c, 1, 64501, "ok", t0)
		seedNet(t, c, 2, 64502, "pending", t0.Add(1*time.Hour))
		seedNet(t, c, 3, 64503, "deleted", t0.Add(2*time.Hour))

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/net?since=0")
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		ids := extractIDs(t, body)
		// (-updated, -created): id=3 (deleted, t0+2h) > id=1 (ok, t0)
		// id=2 (pending) excluded.
		want := []int{3, 1}
		if !equalIntSlice(ids, want) {
			t.Errorf("STATUS-04 since admits ok+deleted: got %v, want %v", ids, want)
		}
	})

	t.Run("STATUS-05_list_since_campus_admits_pending", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:712-715 (campus carve-out: pending admitted
		// on since>0 list — the IXP onboarding workflow needs pending
		// campuses to sync within the cycle window)
		// upstream: pdb_api_test.py:3965 (campus list with mixed statuses)
		c := testutil.SetupClient(t)
		seedCampus(t, c, 1, "ok", t0)
		seedCampus(t, c, 2, "pending", t0.Add(1*time.Hour))
		seedCampus(t, c, 3, "deleted", t0.Add(2*time.Hour))

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/campus?since=0")
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		ids := extractIDs(t, body)
		// All 3 admitted on since>0 for campus.
		want := []int{3, 2, 1}
		if !equalIntSlice(ids, want) {
			t.Errorf("STATUS-05 campus since admits all 3: got %v, want %v", ids, want)
		}
	})

	t.Run("STATUS-06_explicit_status_deleted_no_since_is_empty", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:694-700 (default branch overrides explicit
		// ?status= when ?since is absent — the implicit ok-filter wins)
		// upstream: pdb_api_test.py:1341 (explicit ?status=deleted
		// without ?since returns empty)
		c := testutil.SetupClient(t)
		seedNet(t, c, 1, 64501, "deleted", t0)

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/net?status=deleted")
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		ids := extractIDs(t, body)
		if len(ids) != 0 {
			t.Errorf("STATUS-06 ?status=deleted w/o since: got %v, want []", ids)
		}
	})
}

// equalIntSlice is a local helper because slices.Equal requires
// matching element counts AND positions (which is what we want — all
// status assertions are order-sensitive due to the (-updated,
// -created) default).
func equalIntSlice(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
