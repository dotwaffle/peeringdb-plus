package parity

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/pdbtypes"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// TestParity_Status locks the upstream 2.83.0 rest.py:719-750 status ×
// since matrix against future regression. Each subtest covers one cell
// of the matrix:
//
//	{no since, since=N} × {list, pk-lookup} × {campus, netixlan, other}
//
// The matrix is built from the type's live statuses (2.83.0
// models.py:109-122):
// "ok" on every type, plus "not-operational" on netixlan. A list without
// since admits the live statuses (rest.py:748), a since list admits live
// + deleted (:723), and a PK lookup admits live + pending (:750). The
// netixlan_* subtests lock the second live status. The depth_sets_*
// subtest locks the nested _set rule: live statuses only, so a pending
// child is left out (serializers.py:1140-1148).
//
// The campus row carries the rest.py:725-735 carve-out where
// status="pending" is admitted on `since>0` list queries (the IXP
// onboarding workflow expects pending campuses to surface to syncing
// clients within the cycle window).
//
// A caller ?status= is an ordinary model-field filter upstream: it
// becomes status__iexact (2.83.0 rest.py:683) or status__in
// (:665-666), and the matrix filter applied after it ANDs with it
// (:745-750). It can only narrow the admitted set. The explicit_status_*
// subtests lock this.
//
// upstream: 2.83.0 peeringdb_server/rest.py:719-750 (status × since matrix)
// upstream: 2.83.0 pdb_api_test.py:4022-4044 (the two since tests;
// the other admission rules are implicit in fixture-mix expectations
// across the test corpus).
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

	// seedNetIXLan seeds one netixlan under a shared org/net/ix/ixlan
	// chain, creating the parents on first use. operational is set the
	// way upstream derives it on save: true only for status "ok"
	// (2.83.0 models.py:6512).
	seedNetIXLan := func(t *testing.T, c *ent.Client, id int, status string, updated time.Time) {
		t.Helper()
		ctx := t.Context()
		if n, _ := c.IxLan.Query().Count(ctx); n == 0 {
			c.Organization.Create().
				SetID(1).SetName("NetIXLanParent").SetNameFold(unifold.Fold("NetIXLanParent")).
				SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
			c.Network.Create().
				SetID(1).SetName("NetIXLanNet").SetNameFold(unifold.Fold("NetIXLanNet")).
				SetAsn(64500).SetOrgID(1).
				SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
			c.InternetExchange.Create().
				SetID(1).SetName("NetIXLanIX").SetNameFold(unifold.Fold("NetIXLanIX")).
				SetOrgID(1).
				SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
			c.IxLan.Create().
				SetID(1).SetIxID(1).
				SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		}
		if _, err := c.NetworkIxLan.Create().
			SetID(id).SetNetID(1).SetIxlanID(1).SetIxID(1).
			SetAsn(64500).SetSpeed(10000).SetName("NetIXLanIX").
			SetOperational(status == "ok").SetStatus(status).
			SetCreated(t0).SetUpdated(updated).
			Save(ctx); err != nil {
			t.Fatalf("seed netixlan id=%d: %v", id, err)
		}
	}
	// seedNetIXLanMix seeds one netixlan per status, with updated
	// ascending in this order: 1 ok, 2 not-operational, 3 pending,
	// 4 deleted.
	seedNetIXLanMix := func(t *testing.T, c *ent.Client) {
		t.Helper()
		for i, status := range []string{"ok", "not-operational", "pending", "deleted"} {
			seedNetIXLan(t, c, i+1, status, t0.Add(time.Duration(i)*time.Hour))
		}
	}

	t.Run("list_no_since_status_ok_only", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:747-748 (default branch — a list
		// without ?since filters to the live statuses, ok on net)
		// synthesised: no single upstream test asserts the default mix;
		// every guest list expectation relies on it.
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
			t.Errorf("list w/o since: got ids %v, want [1] (only status=ok)", ids)
		}
	})

	t.Run("pk_lookup_admits_pending", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:749-750 (pk-lookup branch admits
		// the live statuses plus pending, so a pending row returns 200
		// by direct ID even though lists hide it)
		// synthesised: no upstream test fetches a pending row by ID.
		c := testutil.SetupClient(t)
		seedNet(t, c, 20, 64520, "pending", t0)

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/net/20")
		if status != http.StatusOK {
			t.Errorf("pk lookup on pending: got %d, want 200; body=%s",
				status, string(body))
		}
	})

	t.Run("pk_lookup_deleted_returns_404", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:749-750 (pk-lookup branch excludes
		// deleted, so the lookup returns 404)
		// synthesised: no upstream test fetches a deleted row by ID.
		c := testutil.SetupClient(t)
		seedNet(t, c, 30, 64530, "deleted", t0)

		srv := newTestServer(t, c)
		status, _ := httpGet(t, srv, "/api/net/30")
		if status != http.StatusNotFound {
			t.Errorf("pk lookup on deleted: got %d, want 404", status)
		}
	})

	t.Run("list_since_admits_deleted_excludes_pending_noncampus", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:719-746 (since>0 admits the live
		// statuses plus deleted at :723, and excludes pending except
		// for the campus carve-out at :725-735)
		// upstream: 2.83.0 pdb_api_test.py:4022-4028
		// (test_guest_005_list_since returns deleted rows)
		c := testutil.SetupClient(t)
		seedNet(t, c, 1, 64501, "ok", t0)
		seedNet(t, c, 2, 64502, "pending", t0.Add(1*time.Hour))
		seedNet(t, c, 3, 64503, "deleted", t0.Add(2*time.Hour))

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/net?since=1")
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		ids := extractIDs(t, body)
		// since lists order updated-ASCENDING (upstream django-handleref
		// since() ordering, 2026-06-10 audit): id=1 (ok, t0) before
		// id=3 (deleted, t0+2h). id=2 (pending) excluded.
		want := []int{1, 3}
		if !equalIntSlice(ids, want) {
			t.Errorf("since admits ok+deleted: got %v, want %v", ids, want)
		}
	})

	t.Run("depth_sets_exclude_pending_campus", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 serializers.py:1140-1148 (the nested _set
		// prefetch admits the live statuses only; 2.82.0 :935 filtered
		// status="ok") + rest.py:774-777 (a detail response uses the
		// same prefetch). A pending campus is left out of its org's
		// campus_set, as an ID at depth=1 and as an object at depth=2,
		// but a PK lookup still returns it (rest.py:750).
		c := testutil.SetupClient(t)
		seedCampus(t, c, 1, "ok", t0)
		seedCampus(t, c, 2, "pending", t0.Add(time.Hour))

		srv := newTestServer(t, c)
		for _, path := range []string{"/api/org/1?depth=1", "/api/org/1?depth=2", "/api/org/1"} {
			status, body := httpGet(t, srv, path)
			if status != http.StatusOK {
				t.Fatalf("GET %s: status = %d; body=%s", path, status, string(body))
			}
			rows := decodeDataArray(t, body)
			if len(rows) != 1 {
				t.Fatalf("GET %s: %d rows, want 1", path, len(rows))
			}
			set, ok := rows[0]["campus_set"].([]any)
			if !ok {
				t.Fatalf("GET %s: campus_set is %T, want array", path, rows[0]["campus_set"])
			}
			var ids []int
			for _, el := range set {
				switch v := el.(type) {
				case float64: // depth=1 ID list
					ids = append(ids, int(v))
				case map[string]any: // depth=2 object
					id, _ := v["id"].(float64)
					ids = append(ids, int(id))
				}
			}
			if want := []int{1}; !equalIntSlice(ids, want) {
				t.Errorf("GET %s: campus_set = %v, want %v (pending campus left out)", path, ids, want)
			}
		}
		if status, body := httpGet(t, srv, "/api/campus/2"); status != http.StatusOK {
			t.Errorf("pk lookup on pending campus: got %d, want 200; body=%s", status, string(body))
		}
	})

	t.Run("list_since_campus_admits_pending", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:725-735 (campus carve-out: pending
		// admitted on since>0 list — the IXP onboarding workflow needs
		// pending campuses to sync within the cycle window)
		// upstream: 2.83.0 pdb_api_test.py:4032-4044
		// (test_guest_005_list_campus_since returns pending campuses)
		c := testutil.SetupClient(t)
		seedCampus(t, c, 1, "ok", t0)
		seedCampus(t, c, 2, "pending", t0.Add(1*time.Hour))
		seedCampus(t, c, 3, "deleted", t0.Add(2*time.Hour))

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/campus?since=1")
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		ids := extractIDs(t, body)
		// All 3 admitted on since>0 for campus, ordered updated-ascending.
		want := []int{1, 2, 3}
		if !equalIntSlice(ids, want) {
			t.Errorf("campus since admits all 3: got %v, want %v", ids, want)
		}
	})

	t.Run("since_zero_is_inert_like_bare_list", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:719 (`if since > 0` — since=0 never
		// activates the matrix; the plain live-status list serves).
		c := testutil.SetupClient(t)
		seedNet(t, c, 1, 64501, "ok", t0)
		seedNet(t, c, 2, 64502, "deleted", t0.Add(time.Hour))

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/net?since=0")
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		ids := extractIDs(t, body)
		want := []int{1}
		if len(ids) != 1 || ids[0] != 1 {
			t.Errorf("since=0 must behave like a bare list (ok only): got %v, want %v", ids, want)
		}
	})

	t.Run("since_boundary_includes_same_second", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:736-744 (since() compares updated
		// with updated__gt against datetime.fromtimestamp(N), which is
		// N.000000) + serializers.py:1920-1924 (updated is shown
		// truncated to the second). Upstream stores microseconds, so a
		// row shown as updated=N is returned for since=N. The mirror
		// stores only the shown second, so the boundary is inclusive.
		c := testutil.SetupClient(t)
		seedNet(t, c, 1, 64501, "ok", t0.Add(-time.Second)) // one second before
		seedNet(t, c, 2, 64502, "ok", t0)                   // same second
		seedNet(t, c, 3, 64503, "ok", t0.Add(time.Hour))    // inside the window

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, fmt.Sprintf("/api/net?since=%d", t0.Unix()))
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		ids := extractIDs(t, body)
		want := []int{2, 3}
		if !equalIntSlice(ids, want) {
			t.Errorf("since boundary must include the same second (updated >= since): got %v, want %v", ids, want)
		}
	})

	t.Run("since_orders_updated_ascending", func(t *testing.T) {
		t.Parallel()
		// upstream: django-handleref since() returns rows ordered by
		// updated ascending so incremental pollers resume from the
		// last row's updated value.
		c := testutil.SetupClient(t)
		seedNet(t, c, 1, 64501, "ok", t0.Add(3*time.Hour))
		seedNet(t, c, 2, 64502, "ok", t0.Add(1*time.Hour))
		seedNet(t, c, 3, 64503, "ok", t0.Add(2*time.Hour))

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/net?since=1")
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		ids := extractIDs(t, body)
		want := []int{2, 3, 1}
		if !equalIntSlice(ids, want) {
			t.Errorf("since list must order updated ascending: got %v, want %v", ids, want)
		}
	})

	t.Run("explicit_status_deleted_no_since_is_empty", func(t *testing.T) {
		t.Parallel()
		// synthesised: 2.83.0 rest.py:683 turns ?status= into
		// status__iexact, and :748 ANDs the no-since matrix (ok only)
		// after it. The two filters cannot both match, so the list is
		// empty. The ok row proves the key is not dropped: a dropped
		// key would return it.
		c := testutil.SetupClient(t)
		seedNet(t, c, 1, 64501, "deleted", t0)
		seedNet(t, c, 2, 64502, "ok", t0.Add(time.Hour))

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/net?status=deleted")
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		ids := extractIDs(t, body)
		if len(ids) != 0 {
			t.Errorf("?status=deleted w/o since: got %v, want []", ids)
		}
	})

	t.Run("explicit_status_pending_no_since_is_empty", func(t *testing.T) {
		t.Parallel()
		// synthesised: 2.83.0 rest.py:683 + :748 (same AND as above;
		// pending rows are PK-visible but never on a no-since list).
		c := testutil.SetupClient(t)
		seedNet(t, c, 1, 64501, "ok", t0)
		seedNet(t, c, 2, 64502, "pending", t0.Add(time.Hour))

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/net?status=pending")
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		ids := extractIDs(t, body)
		if len(ids) != 0 {
			t.Errorf("?status=pending w/o since: got %v, want []", ids)
		}
	})

	t.Run("explicit_status_deleted_since_returns_tombstones_only", func(t *testing.T) {
		t.Parallel()
		// upstream: pdb_api_test.py:4022-4028 (test_guest_005_list_since:
		// net?since=N&status=deleted returns exactly the deleted nets,
		// not ok+deleted)
		c := testutil.SetupClient(t)
		seedNet(t, c, 1, 64501, "ok", t0)
		seedNet(t, c, 2, 64502, "deleted", t0.Add(1*time.Hour))
		seedNet(t, c, 3, 64503, "deleted", t0.Add(2*time.Hour))

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/net?since=1&status=deleted")
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		ids := extractIDs(t, body)
		want := []int{2, 3}
		if !equalIntSlice(ids, want) {
			t.Errorf("since+status=deleted: got %v, want %v (tombstones only)", ids, want)
		}
	})

	t.Run("explicit_status_pending_campus_since_returns_pending_only", func(t *testing.T) {
		t.Parallel()
		// upstream: pdb_api_test.py:4032-4044
		// (test_guest_005_list_campus_since: campus?since=N&status=pending
		// returns only pending rows)
		c := testutil.SetupClient(t)
		seedCampus(t, c, 1, "ok", t0)
		seedCampus(t, c, 2, "pending", t0.Add(1*time.Hour))
		seedCampus(t, c, 3, "deleted", t0.Add(2*time.Hour))

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/campus?since=1&status=pending")
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		ids := extractIDs(t, body)
		want := []int{2}
		if !equalIntSlice(ids, want) {
			t.Errorf("campus since+status=pending: got %v, want %v (pending only)", ids, want)
		}
	})

	t.Run("explicit_status_is_case_insensitive", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:683 (an exact match on a CharField
		// becomes __iexact, so ?status=OK matches "ok")
		c := testutil.SetupClient(t)
		seedNet(t, c, 1, 64501, "ok", t0)

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/net?status=OK")
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		ids := extractIDs(t, body)
		want := []int{1}
		if !equalIntSlice(ids, want) {
			t.Errorf("?status=OK: got %v, want %v", ids, want)
		}
	})

	t.Run("explicit_status_in_ands_with_since_matrix", func(t *testing.T) {
		t.Parallel()
		// synthesised: 2.83.0 rest.py:665-666 splits ?status__in= into
		// a list, and :745 ANDs the since matrix (ok+deleted for net).
		// pending is in the caller list but outside the matrix.
		c := testutil.SetupClient(t)
		seedNet(t, c, 1, 64501, "ok", t0)
		seedNet(t, c, 2, 64502, "pending", t0.Add(1*time.Hour))
		seedNet(t, c, 3, 64503, "deleted", t0.Add(2*time.Hour))

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/net?since=1&status__in=pending,deleted")
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		ids := extractIDs(t, body)
		want := []int{3}
		if !equalIntSlice(ids, want) {
			t.Errorf("since+status__in=pending,deleted: got %v, want %v", ids, want)
		}
	})

	t.Run("netixlan_not_operational_is_live_on_bare_list", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 models.py:109-122 (live_statuses: netixlan
		// is live as ok or not-operational) + rest.py:748 (a no-since
		// list admits the live statuses)
		c := testutil.SetupClient(t)
		seedNetIXLanMix(t, c)

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/netixlan")
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		// A plain list is ordered by id ascending.
		want := []int{1, 2}
		if ids := extractIDs(t, body); !equalIntSlice(ids, want) {
			t.Errorf("netixlan bare list: got %v, want %v (ok + not-operational)", ids, want)
		}
	})

	t.Run("netixlan_not_operational_in_since_window", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:723 (since admits the live statuses
		// plus deleted; pending stays hidden on netixlan)
		c := testutil.SetupClient(t)
		seedNetIXLanMix(t, c)

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/netixlan?since=1")
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		want := []int{1, 2, 4}
		if ids := extractIDs(t, body); !equalIntSlice(ids, want) {
			t.Errorf("netixlan since: got %v, want %v (ok + not-operational + deleted)", ids, want)
		}
	})

	t.Run("netixlan_not_operational_pk_lookup_returns_200", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:750 (a PK lookup admits the live
		// statuses plus pending)
		c := testutil.SetupClient(t)
		seedNetIXLanMix(t, c)

		srv := newTestServer(t, c)
		for _, path := range []string{"/api/netixlan/2", "/api/netixlan/2?depth=0"} {
			if status, body := httpGet(t, srv, path); status != http.StatusOK {
				t.Errorf("GET %s: got %d, want 200; body=%s", path, status, string(body))
			}
		}
	})

	t.Run("netixlan_operational_false_returns_not_operational", func(t *testing.T) {
		t.Parallel()
		// upstream: pdb_api_test.py:5070-5090
		// (test_guest_005_list_filter_netixlan_operational: a
		// not-operational netixlan is returned by operational=0)
		c := testutil.SetupClient(t)
		seedNetIXLanMix(t, c)

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/netixlan?operational=0")
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		want := []int{2}
		if ids := extractIDs(t, body); !equalIntSlice(ids, want) {
			t.Errorf("netixlan operational=0: got %v, want %v", ids, want)
		}
	})

	t.Run("netixlan_status_filter_selects_live_subset", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 docs/api/obj_netixlan.md:134-137 (status=ok
		// no longer returns every live connection; use
		// status__in=ok,not-operational) + rest.py:683 (status__iexact)
		c := testutil.SetupClient(t)
		seedNetIXLanMix(t, c)

		srv := newTestServer(t, c)
		cases := []struct {
			query string
			want  []int
		}{
			{"?status=ok", []int{1}},
			{"?status=not-operational", []int{2}},
			{"?status__in=ok,not-operational", []int{1, 2}},
		}
		for _, tc := range cases {
			status, body := httpGet(t, srv, "/api/netixlan"+tc.query)
			if status != http.StatusOK {
				t.Fatalf("GET /api/netixlan%s: status = %d; body=%s", tc.query, status, string(body))
			}
			if ids := extractIDs(t, body); !equalIntSlice(ids, tc.want) {
				t.Errorf("GET /api/netixlan%s: got %v, want %v", tc.query, ids, tc.want)
			}
		}
	})

	// assertEntityNotFound checks the 404 that upstream returns for a
	// unique list query with an empty result. The body is problem+json
	// under the registered error-envelope divergence.
	assertEntityNotFound := func(t *testing.T, srv *httptest.Server, path string) {
		t.Helper()
		status, body := httpGet(t, srv, path)
		if status != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404; body=%s", path, status, string(body))
			return
		}
		if p := mustDecodeProblem(t, body); p.Detail != "Entity not found" {
			t.Errorf("GET %s: detail = %q, want %q", path, p.Detail, "Entity not found")
		}
	}
	// assertEmptyList checks a 200 with an empty data array.
	assertEmptyList := func(t *testing.T, srv *httptest.Server, path string) {
		t.Helper()
		status, body := httpGet(t, srv, path)
		if status != http.StatusOK {
			t.Errorf("GET %s: status = %d, want 200; body=%s", path, status, string(body))
			return
		}
		if ids := extractIDs(t, body); len(ids) != 0 {
			t.Errorf("GET %s: got ids %v, want []", path, ids)
		}
	}

	t.Run("unique_id_miss_404_all_types", func(t *testing.T) {
		t.Parallel()
		// upstream: pdb_api_test.py:3904-3910 (test_guest_001_GET_list_404:
		// every reftag with id=99999999 raises NotFoundException) +
		// 2.83.0 rest.py:809-815 + serializers.py:962-967
		c := testutil.SetupClient(t)
		srv := newTestServer(t, c)
		for _, typ := range pdbtypes.Names() {
			assertEntityNotFound(t, srv, "/api/"+typ+"?limit=1&id=99999999")
		}
	})

	t.Run("unique_net_asn_miss_404", func(t *testing.T) {
		t.Parallel()
		// upstream: pdb_api_test.py:3908-3910 (net with
		// asn=99999999999 raises NotFoundException) +
		// serializers.py:3815-3820 (NetworkSerializer adds "asn")
		c := testutil.SetupClient(t)
		seedNet(t, c, 1, 64501, "ok", t0)
		srv := newTestServer(t, c)
		assertEntityNotFound(t, srv, "/api/net?limit=1&asn=99999999999")
	})

	t.Run("unique_id_hit_200", func(t *testing.T) {
		t.Parallel()
		// upstream: pdb_api_test.py:3912-3918 (an id that exists
		// returns exactly one row)
		c := testutil.SetupClient(t)
		seedNet(t, c, 1, 64501, "ok", t0)
		seedNet(t, c, 2, 64502, "ok", t0)
		srv := newTestServer(t, c)
		for _, path := range []string{"/api/net?id=1", "/api/net?asn=64501"} {
			status, body := httpGet(t, srv, path)
			if status != http.StatusOK {
				t.Fatalf("GET %s: status = %d; body=%s", path, status, string(body))
			}
			if ids := extractIDs(t, body); !equalIntSlice(ids, []int{1}) {
				t.Errorf("GET %s: got %v, want [1]", path, ids)
			}
		}
	})

	t.Run("unique_id_with_excluding_filter_404", func(t *testing.T) {
		t.Parallel()
		// upstream: pdb_api_test.py:1031-1033 (net with its own id and
		// a non-matching filter raises NotFoundException): the check
		// runs on the final result, whatever filtered it out.
		c := testutil.SetupClient(t)
		seedNet(t, c, 1, 64501, "ok", t0)
		seedNet(t, c, 2, 64502, "deleted", t0)
		srv := newTestServer(t, c)
		assertEntityNotFound(t, srv, "/api/net?id=1&name=nomatch")
		// A tombstone is outside the no-since status matrix.
		assertEntityNotFound(t, srv, "/api/net?id=2")
		assertEntityNotFound(t, srv, "/api/net?asn=64501&name=nomatch")
		assertEntityNotFound(t, srv, fmt.Sprintf("/api/net?id=1&since=%d", t0.Add(time.Hour).Unix()))
	})

	t.Run("unique_id_skip_past_end_404", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:757-760 (skip slices the result)
		// + :809-815 (the check runs on the sliced list). With a
		// budget the mirror answers this from COUNT(*) and never
		// runs the list query.
		c := testutil.SetupClient(t)
		seedNet(t, c, 1, 64501, "ok", t0)
		srv := newTestServerWithBudget(t, c, 1<<30)
		assertEntityNotFound(t, srv, "/api/net?id=1&skip=5")
		assertEmptyList(t, srv, "/api/net?name=nomatch&skip=5")
	})

	t.Run("non_unique_keys_miss_200_empty", func(t *testing.T) {
		t.Parallel()
		// upstream: serializers.py:962-967 checks the literal key
		// "id" only, so id__in is not unique; "asn" is unique on net
		// only (:3815-3820), so netixlan?asn= is a plain filter.
		c := testutil.SetupClient(t)
		seedNet(t, c, 1, 64501, "ok", t0)
		srv := newTestServer(t, c)
		for _, path := range []string{
			"/api/org?id__in=999",
			"/api/net?asn__in=99999",
			"/api/netixlan?asn=99999",
			"/api/net?name=nomatch",
		} {
			assertEmptyList(t, srv, path)
		}
	})

	t.Run("unique_id_with_page_200_empty", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:799-815 + pagination.py:35-50: with
		// ?page= the response data is the pagination object, which is
		// never empty, so the 404 check does not fire.
		c := testutil.SetupClient(t)
		srv := newTestServer(t, c)
		assertEmptyList(t, srv, "/api/net?id=99999999&page=1")
	})

	t.Run("DIVERGENCE_poc_hidden_id_returns_404", func(t *testing.T) {
		t.Parallel()
		// DIVERGENCE: upstream runs the unique-query 404 check before
		// APIPermissionsApplicator removes the contacts that the caller
		// may not read (2.83.0 rest.py:809-821), so an anonymous
		// /api/poc?id=<Users contact>, or a /api/poc?id=<Private
		// contact> from a user who is not a member of the owning
		// organization, gets 200 {"data": []}, and a missing id gets
		// 404. The mirror hides the contact in the query (the
		// poc.visible privacy policy) and returns 404 for both, so the
		// status does not show that the contact exists.
		// See docs/API.md § Known Divergences.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		seedNet(t, c, 1, 64501, "ok", t0)
		for id, visible := range map[int]string{101: "Users", 102: "Private"} {
			if _, err := c.Poc.Create().
				SetID(id).SetNetID(1).SetRole("NOC").SetVisible(visible).
				SetStatus("ok").SetCreated(t0).SetUpdated(t0).
				Save(ctx); err != nil {
				t.Fatalf("seed poc id=%d: %v", id, err)
			}
		}
		anon := newTestServer(t, c)
		assertEntityNotFound(t, anon, "/api/poc?id=101")
		assertEntityNotFound(t, anon, "/api/poc?id=102")
		assertEntityNotFound(t, anon, "/api/poc?id=999")

		users := newTestServerWithTier(t, c, privctx.TierUsers)
		assertEntityNotFound(t, users, "/api/poc?id=102")
		assertEntityNotFound(t, users, "/api/poc?id=999")
		// Control: the users tier reads the Users contact.
		status, body := httpGet(t, users, "/api/poc?id=101")
		if got := extractIDs(t, body); status != http.StatusOK || len(got) != 1 || got[0] != 101 {
			t.Errorf("users tier /api/poc?id=101: status %d ids %v, want 200 [101]", status, got)
		}
	})

	t.Run("DIVERGENCE_unique_key_non_integer_returns_400", func(t *testing.T) {
		t.Parallel()
		// DIVERGENCE: upstream turns a plain id or asn key into an
		// __iexact filter (2.83.0 rest.py:670-683). Django does not
		// convert an iexact value to an integer, so a non-integer or
		// empty value matches no row, and the unique query then gets
		// the 404 (rest.py:809-815). The mirror rejects the value with
		// a 400, as for every other integer filter. See docs/API.md
		// § Known Divergences.
		c := testutil.SetupClient(t)
		seedNet(t, c, 1, 64501, "ok", t0)
		srv := newTestServer(t, c)
		for _, path := range []string{
			"/api/net?id=abc",
			"/api/net?id=",
			"/api/net?asn=abc",
			"/api/net?asn=",
			"/api/fac?id=abc",
		} {
			status, body := httpGet(t, srv, path)
			if status != http.StatusBadRequest {
				t.Errorf("GET %s: status = %d, want 400; body=%s", path, status, string(body))
				continue
			}
			if p := mustDecodeProblem(t, body); !strings.Contains(p.Detail, "to int") {
				t.Errorf("GET %s: detail = %q, want an int conversion error", path, p.Detail)
			}
		}
	})

	t.Run("deleted_poc_blanks_contact_fields", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 serializers.py:2941-2954 (#569)
		// upstream: 2.83.0 pdb_api_test.py:3120-3127
		// Upstream blanks name, phone, email and url when it renders a
		// deleted contact with its status, whatever the database holds. The
		// tombstone here holds contact data, as the rows that the removed
		// inference-by-absence sync code (v1.16.0 to v1.18.1) marked
		// deleted do. A live contact keeps its data.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		seedNet(t, c, 1, 64501, "ok", t0)
		for _, p := range []struct {
			id     int
			status string
		}{{10, "deleted"}, {11, "ok"}} {
			if _, err := c.Poc.Create().
				SetID(p.id).SetNetID(1).SetRole("NOC").SetVisible("Public").
				SetName("Jane Doe").SetPhone("+1 555 0100").
				SetEmail("jane@example.invalid").SetURL("https://example.invalid/jane").
				SetStatus(p.status).SetCreated(t0).SetUpdated(t0.Add(time.Hour)).
				Save(ctx); err != nil {
				t.Fatalf("seed poc id=%d: %v", p.id, err)
			}
		}

		srv := newTestServer(t, c)
		path := fmt.Sprintf("/api/poc?since=%d", t0.Unix())
		status, body := httpGet(t, srv, path)
		if status != http.StatusOK {
			t.Fatalf("GET %s: status = %d; body=%s", path, status, string(body))
		}
		byID := make(map[int]map[string]any)
		for _, row := range decodeDataArray(t, body) {
			if id, ok := row["id"].(float64); ok {
				byID[int(id)] = row
			}
		}
		if len(byID) != 2 {
			t.Fatalf("GET %s: got %d rows, want 2 (ids 10 and 11); body=%s", path, len(byID), string(body))
		}

		tomb := byID[10]
		for _, key := range []string{"name", "phone", "email", "url"} {
			if got := tomb[key]; got != "" {
				t.Errorf("deleted poc %s = %v, want \"\"", key, got)
			}
		}
		for key, want := range map[string]any{
			"role": "NOC", "visible": "Public", "net_id": float64(1), "status": "deleted",
		} {
			if got := tomb[key]; got != want {
				t.Errorf("deleted poc %s = %v, want %v", key, got, want)
			}
		}

		live := byID[11]
		for key, want := range map[string]string{
			"name": "Jane Doe", "phone": "+1 555 0100",
			"email": "jane@example.invalid", "url": "https://example.invalid/jane",
		} {
			if got := live[key]; got != want {
				t.Errorf("live poc %s = %v, want %q", key, got, want)
			}
		}
	})

	t.Run("DIVERGENCE_deleted_poc_blanked_without_status_field", func(t *testing.T) {
		t.Parallel()
		// DIVERGENCE: upstream blanks the contact fields of a deleted
		// contact only when status is among the rendered fields. The
		// serializer removes the fields that ?fields= does not name, and
		// to_representation blanks only when the rendered status is
		// "deleted". A soft delete keeps the stored values, so upstream
		// returns them for ?fields=id,name,email. The mirror blanks the
		// fields before it applies ?fields=, so it returns "".
		// See docs/API.md § Known Divergences.
		// This test ASSERTS the divergence (it is NOT a parity match).
		// upstream: 2.83.0 serializers.py:942-950 (?fields= filter)
		// + serializers.py:2941-2954 (status check)
		c := testutil.SetupClient(t)
		ctx := t.Context()
		seedNet(t, c, 1, 64501, "ok", t0)
		if _, err := c.Poc.Create().
			SetID(10).SetNetID(1).SetRole("NOC").SetVisible("Public").
			SetName("Jane Doe").SetPhone("+1 555 0100").
			SetEmail("jane@example.invalid").SetURL("https://example.invalid/jane").
			SetStatus("deleted").SetCreated(t0).SetUpdated(t0.Add(time.Hour)).
			Save(ctx); err != nil {
			t.Fatalf("seed poc tombstone: %v", err)
		}

		srv := newTestServer(t, c)
		path := fmt.Sprintf("/api/poc?since=%d&fields=id,name,email", t0.Unix())
		status, body := httpGet(t, srv, path)
		if status != http.StatusOK {
			t.Fatalf("GET %s: status = %d; body=%s", path, status, string(body))
		}
		rows := decodeDataArray(t, body)
		if len(rows) != 1 || rows[0]["id"] != float64(10) {
			t.Fatalf("GET %s: got %v, want one row with id 10", path, rows)
		}
		if _, ok := rows[0]["status"]; ok {
			t.Fatalf("GET %s: row has status, want it projected away: %v", path, rows[0])
		}
		// Upstream returns "Jane Doe" and "jane@example.invalid" here.
		for _, key := range []string{"name", "email"} {
			if got := rows[0][key]; got != "" {
				t.Errorf("GET %s: %s = %v, want \"\" (divergence canary)", path, key, got)
			}
		}
	})

	t.Run("DIVERGENCE_deleted_net_netixlan_tombstone_in_since_window", func(t *testing.T) {
		t.Parallel()
		// DIVERGENCE: when the RIR reclaims the ASN of a network,
		// upstream pdb_rir_status removes the live connections (ok,
		// not-operational) of the network with an SQL delete, then
		// soft-deletes the network. The connections get no tombstone, so
		// a ?since= window returns nothing for them, and ?id=<id>&since=N
		// answers 404 Entity not found. Sync marks such a connection
		// deleted with operational false and keeps its updated value, so
		// the window returns the tombstone and the id query answers 200.
		// The seed is the state that the sync cascade leaves.
		// See docs/API.md § Known Divergences.
		// This test ASSERTS the divergence (it is NOT a parity match).
		// upstream: 2.83.0 management/commands/pdb_rir_status.py:440-443
		// + models.py:5720-5725 (netixlan_set_active)
		// + models.py:109-122 (live_statuses)
		// + rest.py:809-815 (unique-query 404)
		c := testutil.SetupClient(t)
		ctx := t.Context()
		last := t0.Add(time.Hour)
		mustOrg(ctx, t, c, 1, "ReclaimOrg", t0)
		mustIX(ctx, t, c, 20, "ReclaimIX", 1, t0)
		mustIxLan(ctx, t, c, 200, "ReclaimLAN", 20, t0)
		if _, err := c.Network.Create().
			SetID(101).SetName("ReclaimNet").SetNameFold(unifold.Fold("ReclaimNet")).
			SetAsn(64511).SetOrgID(1).
			SetStatus("deleted").SetCreated(t0).SetUpdated(t0.Add(48 * time.Hour)).
			Save(ctx); err != nil {
			t.Fatalf("seed net tombstone: %v", err)
		}
		if _, err := c.NetworkIxLan.Create().
			SetID(1001).SetNetID(101).SetIxlanID(200).SetIxID(20).
			SetAsn(64511).SetSpeed(10000).SetName("ReclaimIX").
			SetOperational(false).SetStatus("deleted").
			SetCreated(t0).SetUpdated(last).
			Save(ctx); err != nil {
			t.Fatalf("seed cascaded netixlan: %v", err)
		}

		srv := newTestServer(t, c)
		for _, path := range []string{
			fmt.Sprintf("/api/netixlan?since=%d", last.Unix()),
			// Upstream answers 404 Entity not found here.
			fmt.Sprintf("/api/netixlan?id=1001&since=%d", last.Unix()),
		} {
			status, body := httpGet(t, srv, path)
			if status != http.StatusOK {
				t.Errorf("GET %s: status = %d, want 200 (divergence canary); body=%s", path, status, string(body))
				continue
			}
			// Upstream returns no row for the connection.
			rows := decodeDataArray(t, body)
			if len(rows) != 1 || rows[0]["id"] != float64(1001) {
				t.Errorf("GET %s: got %v, want one row with id 1001 (divergence canary)", path, rows)
				continue
			}
			for key, want := range map[string]any{
				"status":      "deleted",
				"operational": false,
				"net_id":      float64(101),
				"updated":     last.Format(time.RFC3339),
			} {
				if got := rows[0][key]; got != want {
					t.Errorf("GET %s: %s = %v, want %v", path, key, got, want)
				}
			}
		}
	})

	t.Run("DIVERGENCE_hidden_poc_detail_404", func(t *testing.T) {
		t.Parallel()
		// DIVERGENCE: upstream answers a GET for a contact that the
		// caller may not read with 403. retrieve returns
		// HTTP_403_FORBIDDEN when the permission applicator denies the
		// whole object. The upstream tests expect this for a Users
		// contact and a guest, and for a Private contact and a user who
		// is not a member of the owning organization. The mirror answers
		// 404, as for an id that does not exist, so the status code does
		// not show that the hidden contact exists.
		// See docs/API.md § Known Divergences.
		// This test ASSERTS the divergence (it is NOT a parity match).
		// upstream: 2.83.0 rest.py:849-865 (retrieve)
		// + pdb_api_test.py:575-577 (assert_get_forbidden)
		// + pdb_api_test.py:3855-3856 (guest, Users contact)
		// + pdb_api_test.py:1413-1414 (user, Private contact)
		c := testutil.SetupClient(t)
		ctx := t.Context()
		seedNet(t, c, 1, 64501, "ok", t0)
		for id, visible := range map[int]string{10: "Users", 11: "Private"} {
			if _, err := c.Poc.Create().
				SetID(id).SetNetID(1).SetRole("NOC").SetVisible(visible).
				SetName("Hidden Contact").SetEmail("hidden@example.invalid").
				SetStatus("ok").SetCreated(t0).SetUpdated(t0).
				Save(ctx); err != nil {
				t.Fatalf("seed poc id=%d: %v", id, err)
			}
		}

		cases := []struct {
			tier privctx.Tier
			id   int
			want int
		}{
			// Upstream: 403 for each hidden contact.
			{privctx.TierPublic, 10, http.StatusNotFound},
			{privctx.TierPublic, 11, http.StatusNotFound},
			{privctx.TierUsers, 11, http.StatusNotFound},
			// Control: the Users tier reads the Users contact.
			{privctx.TierUsers, 10, http.StatusOK},
		}
		for _, tc := range cases {
			srv := newTestServerWithTier(t, c, tc.tier)
			path := fmt.Sprintf("/api/poc/%d", tc.id)
			if status, body := httpGet(t, srv, path); status != tc.want {
				t.Errorf("tier %d GET %s: status = %d, want %d (divergence canary); body=%s", tc.tier, path, status, tc.want, string(body))
			}
		}
	})

	t.Run("DIVERGENCE_i_operator_suffixes_filter", func(t *testing.T) {
		t.Parallel()
		// DIVERGENCE: upstream's operator regex (2.83.0 rest.py:616)
		// knows only lt, lte, gt, gte, contains, startswith and in. A
		// key ending in __iexact, __icontains or __istartswith is not a
		// filter key (:628-630, :670), so upstream ignores it and
		// returns every live row. The mirror applies these suffixes,
		// on status as on any other field.
		// See docs/API.md § Known Divergences.
		// This test ASSERTS the divergence (it is NOT a parity match).
		c := testutil.SetupClient(t)
		seedNetIXLanMix(t, c)

		srv := newTestServer(t, c)
		// Upstream returns the live rows [2 1] for each request.
		cases := []struct {
			query string
			want  []int
		}{
			{"status__iexact=OK", []int{1}},
			{"status__icontains=OPER", []int{2}},
			{"status__istartswith=NOT", []int{2}},
		}
		for _, tc := range cases {
			status, body := httpGet(t, srv, "/api/netixlan?"+tc.query)
			if status != http.StatusOK {
				t.Fatalf("?%s: status = %d; body=%s", tc.query, status, string(body))
			}
			if ids := extractIDs(t, body); !equalIntSlice(ids, tc.want) {
				t.Errorf("?%s: got %v, want %v (divergence canary)", tc.query, ids, tc.want)
			}
		}

		// On an int field, and on a key that names a FK, __iexact is an
		// exact id match and __icontains is a 400 (the int column has no
		// substring match). Upstream ignores both keys and returns every
		// net.
		fk := newTestServer(t, seedFKKeys(t, t0))
		assertKeysResolve(t, fk, []silentIgnoreCase{
			{path: "/api/net?org__iexact=1", want: []int{100}},
		})
		if status, body := httpGet(t, fk, "/api/net?org__icontains=1"); status != http.StatusBadRequest {
			t.Errorf("?org__icontains=1: status = %d, want 400; body=%s", status, string(body))
		}

		// On a relation key of a prepare_query, get_relation_filters
		// does not parse the suffix (serializers.py:643-654). A
		// 3-segment key drops it and filters the field on the value, so
		// upstream runs an exact match. On the status of the pinned
		// row, make_relation_filter then replaces the value with ok
		// (models.py:221-234). A 2-segment key keeps the suffix as a
		// Django lookup on the relation, which raises FieldError, so
		// upstream returns 400 (rest.py:488-500).
		rel := newTestServer(t, seedRelationSeedKeys(t, t0))
		assertKeysResolve(t, rel, []silentIgnoreCase{
			// Upstream: [].
			{path: "/api/net?ix__name__icontains=seedix20", want: []int{100}},
			// Upstream: [20].
			{path: "/api/ix?ixlan__status__iexact=pending", want: []int{}},
			// Upstream: 400.
			{path: "/api/net?ixlan__iexact=200", want: []int{100}},
		})
	})

	t.Run("relation_keys_pin_row_status_under_since", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0. Some related_to_<x> methods run
		// make_relation_filter on the listed rows, so status="ok"
		// applies to the listed row itself (models.py:221-234):
		//   - ixpfx related_to_ix filters ixlan__<field> on the prefix
		//     (models.py:5167-5177). The ixlan and the exchange are not
		//     checked.
		//   - netfac and ixfac related_to_{name,country,city} filter
		//     facility__<field> on the row (models.py:6002-6037,
		//     :3239-3274). The facility is not checked.
		//   - campus related_to_facility filters fac_set on the campus
		//     (models.py:2101-2111).
		// A since list admits deleted rows, and pending campuses
		// (rest.py:719-750), but these keys still drop them.
		// pdb_api_test.py:5107-5127 and :5141-5161 test the netfac and
		// ixfac keys.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		mustOrg(ctx, t, c, 1, "PinOrg", t0)
		mustIX(ctx, t, c, 20, "PinIX20", 1, t0)
		mustIX(ctx, t, c, 21, "PinIX21", 1, t0)
		mustIxLan(ctx, t, c, 200, "PinLanA", 20, t0)
		c.IxLan.Create().
			SetID(210).SetIxID(21).SetName("PinLanP").
			SetStatus("pending").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		mustIxPfx(ctx, t, c, 1000, "10.0.0.0/24", 200, t0)
		c.IxPrefix.Create().
			SetID(1001).SetPrefix("10.0.1.0/24").SetProtocol("IPv4").SetIxlanID(200).
			SetStatus("deleted").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		mustIxPfx(ctx, t, c, 1002, "10.1.0.0/24", 210, t0)
		mustNet(ctx, t, c, 100, "PinNet", 64500, 1, t0)
		c.Campus.Create().
			SetID(50).SetName("PinCampusA").SetNameFold(unifold.Fold("PinCampusA")).SetOrgID(1).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		c.Campus.Create().
			SetID(51).SetName("PinCampusP").SetNameFold(unifold.Fold("PinCampusP")).SetOrgID(1).
			SetStatus("pending").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		for _, f := range []struct {
			id, campus int
			name       string
		}{
			{400, 0, "PinFacA"},
			{402, 50, "PinFacC"},
			{403, 51, "PinFacD"},
		} {
			fc := c.Facility.Create().
				SetID(f.id).SetName(f.name).SetNameFold(unifold.Fold(f.name)).
				SetOrgID(1).SetCity("TestCity").SetCityFold(unifold.Fold("TestCity")).
				SetCountry("DE").
				SetStatus("ok").SetCreated(t0).SetUpdated(t0)
			if f.campus != 0 {
				fc.SetCampusID(f.campus)
			}
			fc.SaveX(ctx)
		}
		for id, st := range map[int]string{600: "ok", 601: "deleted"} {
			c.NetworkFacility.Create().
				SetID(id).SetNetID(100).SetFacID(400).SetLocalAsn(64500).
				SetStatus(st).SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		}
		for id, st := range map[int]string{700: "ok", 701: "deleted"} {
			c.IxFacility.Create().
				SetID(id).SetIxID(id - 680).SetFacID(400).
				SetStatus(st).SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		}

		srv := newTestServer(t, c)
		assertKeysResolve(t, srv, []silentIgnoreCase{
			{path: "/api/ixpfx?ix_id=20&since=1", want: []int{1000}},
			{path: "/api/ixpfx?ix__name=PinIX20&since=1", want: []int{1000}},
			{path: "/api/ixpfx?ix_id=21", want: []int{1002}},
			{path: "/api/netfac?name=PinFacA&since=1", want: []int{600}},
			{path: "/api/netfac?city=TestCity&since=1", want: []int{600}},
			{path: "/api/ixfac?name=PinFacA&since=1", want: []int{700}},
			{path: "/api/ixfac?country=DE&since=1", want: []int{700}},
			{path: "/api/campus?facility=402&since=1", want: []int{50}},
			{path: "/api/campus?facility=403&since=1", want: []int{}},
			{path: "/api/campus?facility__name=PinFacC&since=1", want: []int{50}},
			// Controls: a forward FK key checks no status.
			{path: "/api/ixpfx?ixlan_id=200&since=1", want: []int{1000, 1001}},
			{path: "/api/netfac?fac_id=400&since=1", want: []int{600, 601}},
			{path: "/api/campus?since=1", want: []int{50, 51}},
		})
	})

	t.Run("DIVERGENCE_detail_ignores_filters", func(t *testing.T) {
		t.Parallel()
		// DIVERGENCE: upstream applies the query-parameter filters to a
		// single-object GET too. retrieve (2.83.0 rest.py:849-855) calls
		// DRF get_object, which filters get_queryset(): the same filters
		// as a list (:565-703) plus the live-or-pending PK status set
		// (:750). A filter that excludes the object returns 404. The
		// mirror reads only ?depth= and ?fields= on a detail request.
		// See docs/API.md § Known Divergences.
		// This test ASSERTS the divergence (it is NOT a parity match).
		c := testutil.SetupClient(t)
		seedNetIXLanMix(t, c)

		srv := newTestServer(t, c)
		cases := []struct {
			path string
			want []int
		}{
			// netixlan 2 is not-operational. Upstream: 404.
			{"/api/netixlan/2?status=ok", []int{2}},
			// net 1 is named NetIXLanNet. Upstream: 404.
			{"/api/net/1?name=nomatch", []int{1}},
		}
		for _, tc := range cases {
			status, body := httpGet(t, srv, tc.path)
			if status != http.StatusOK {
				t.Fatalf("GET %s: status = %d, want 200 (divergence canary); body=%s", tc.path, status, string(body))
			}
			if ids := extractIDs(t, body); !equalIntSlice(ids, tc.want) {
				t.Errorf("GET %s: got %v, want %v", tc.path, ids, tc.want)
			}
		}
	})
}

// equalIntSlice reports whether a and b hold the same ids in the same
// positions. The status assertions compare order too: a plain list is
// ordered by id ascending, and a ?since= list by updated ascending.
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
