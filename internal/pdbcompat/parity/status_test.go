package parity

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
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
// A single-object GET applies the same filters, and ANDs them with the
// PK status set (rest.py:849-855). The detail_* subtests lock this.
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
	// unique list query with an empty result: {"data": [], "meta":
	// {"error": "Entity not found"}} (2.83.0 rest.py:809-815).
	assertEntityNotFound := func(t *testing.T, srv *httptest.Server, path string) {
		t.Helper()
		status, body := httpGet(t, srv, path)
		if status != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404; body=%s", path, status, string(body))
			return
		}
		if got := mustDecodeMetaError(t, body).Error; got != "Entity not found" {
			t.Errorf("GET %s: meta.error = %q, want %q", path, got, "Entity not found")
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

	t.Run("unique_key_non_integer_returns_404", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:670-683 (a plain key on a field that
		// is not a relation, a date or a boolean is an __iexact filter),
		// :809-815 (unique-query 404); Django lookups.py:430-438
		// (IExact.prepare_rhs=False: the value is not converted, so
		// MySQL compares the integer as decimal text);
		// pdb_api_test.py:3904-3910. rest.py:597 folds the value with
		// unidecode first, so full-width digits match.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		seedNet(t, c, 1, 64501, "ok", t0)
		mustOrg(ctx, t, c, 1, "IntKeyOrg", t0)
		mustFac(ctx, t, c, 1, "IntKeyFac", 1, t0)
		// A contact that the anonymous tier cannot read: a value that is
		// not an integer must not tell it apart from a missing id.
		if _, err := c.Poc.Create().
			SetID(1).SetNetID(1).SetRole("NOC").SetVisible("Users").
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).
			Save(ctx); err != nil {
			t.Fatalf("seed poc: %v", err)
		}
		srv := newTestServer(t, c)
		for _, path := range []string{
			"/api/net?id=abc",
			"/api/net?id=",
			"/api/net?id=01",
			"/api/net?asn=abc",
			"/api/net?asn=",
			"/api/net?asn=064501",
			"/api/net?asn=%2B64501",
			"/api/net?asn=%2064501",
			"/api/net?asn=64_501",
			"/api/fac?id=abc",
			"/api/ixlan?id=abc",
			"/api/poc?id=abc",
			"/api/poc?id=999",
		} {
			assertEntityNotFound(t, srv, path)
		}
		for _, path := range []string{
			"/api/net?asn=64501",
			"/api/net?id=1",
			"/api/net?asn=" + url.QueryEscape("\uff16\uff14\uff15\uff10\uff11"),
		} {
			status, body := httpGet(t, srv, path)
			if got := extractIDs(t, body); status != http.StatusOK || !equalIntSlice(got, []int{1}) {
				t.Errorf("GET %s: status %d ids %v, want 200 [1]", path, status, got)
			}
		}
	})

	t.Run("plain_int_key_non_integer_matches_nothing", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:670-683 (__iexact on an integer model
		// field, also through queryable_relations for org__id and
		// net__asn); Django lookups.py:430-438. The value is not
		// converted, so it matches no row and raises no error. These
		// keys are not unique queries, so the list is empty.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		mustOrg(ctx, t, c, 1, "PlainIntOrg", t0)
		mustNet(ctx, t, c, 10, "PlainIntNet", 64510, 1, t0)
		mustIX(ctx, t, c, 20, "PlainIntIX", 1, t0)
		mustIxLan(ctx, t, c, 20, "PlainIntLan", 20, t0)
		c.NetworkIxLan.Create().
			SetID(30).SetNetID(10).SetIxlanID(20).SetIxID(20).
			SetAsn(64510).SetSpeed(1000).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		srv := newTestServer(t, c)
		for _, path := range []string{
			"/api/netixlan?asn=abc",
			"/api/netixlan?asn=064510",
			"/api/netixlan?speed=1_000",
			"/api/net?info_prefixes4=abc",
			"/api/net?org__id=abc",
			"/api/net?org__id=01",
			"/api/netixlan?net__asn=abc",
		} {
			assertEmptyList(t, srv, path)
		}
		// Control: the decimal text matches.
		assertKeysResolve(t, srv, []silentIgnoreCase{
			{path: "/api/netixlan?asn=64510", want: []int{30}},
			{path: "/api/netixlan?speed=1000", want: []int{30}},
			{path: "/api/net?org__id=1", want: []int{10}},
			{path: "/api/netixlan?net__asn=64510", want: []int{30}},
		})
	})

	t.Run("int_key_with_operator_or_fk_stays_400", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:676-677 (a FK key is an exact lookup
		// on <fk>_id, which converts the value), :693-703 and
		// :824-831 (the ValueError of an operator lookup is a 400);
		// serializers.py:2119-2124, :3743-3748, :4531-4543 (the count
		// seeds are exact lookups in prepare_query, rest.py:494-495).
		c := testutil.SetupClient(t)
		seedNet(t, c, 1, 64501, "ok", t0)
		srv := newTestServer(t, c)
		for _, path := range []string{
			"/api/net?asn__lt=abc",
			"/api/net?asn__in=a",
			"/api/net?asn__in=1,x",
			"/api/net?org=abc",
			"/api/net?org=",
			"/api/net?org_id=abc",
			"/api/fac?net_count=abc",
			"/api/fac?net_count__gt=abc",
			"/api/ix?fac_count=abc",
			"/api/ix?net_count=abc",
			"/api/net?fac_count=abc",
		} {
			status, body := httpGet(t, srv, path)
			if status != http.StatusBadRequest {
				t.Errorf("GET %s: status = %d, want 400; body=%s", path, status, string(body))
				continue
			}
			if got := mustDecodeMetaError(t, body).Error; !strings.Contains(got, "to int") {
				t.Errorf("GET %s: meta.error = %q, want an int conversion error", path, got)
			}
		}
	})

	t.Run("strict_int_values_use_python_int", func(t *testing.T) {
		t.Parallel()
		// upstream: Django fields/__init__.py:2123-2131
		// (IntegerField.get_prep_value is int(value)), rest.py:597
		// (unidecode on the filter loop values); serializers.py:618-619
		// (get_relation_filters passes the raw value), :2188-2190 (fac
		// all_net: int() on each item), :4560-4562 (ix all_net),
		// :3750-3753 (net not_ix). Python int() accepts white space at
		// the ends, a sign, underscores between digits and Unicode Nd
		// digits. See seedPresenceKeys for the rows.
		c := seedPresenceKeys(t, t0)
		c.Facility.UpdateOneID(400).SetNetCount(2).SaveX(t.Context())
		srv := newTestServer(t, c)
		for _, tc := range []struct{ path, canonical string }{
			{"/api/net?org_id=%201", "/api/net?org_id=1"},
			{"/api/net?org=01", "/api/net?org=1"},
			{"/api/net?asn__in=%2064500,064501", "/api/net?asn__in=64500,64501"},
			{"/api/net?asn__lt=64_502", "/api/net?asn__lt=64502"},
			{"/api/net?asn__gte=%2B64501", "/api/net?asn__gte=64501"},
			{"/api/fac?net=%EF%BC%91%EF%BC%90%EF%BC%90", "/api/fac?net=100"},
			{"/api/fac?net_count=%202", "/api/fac?net_count=2"},
			{"/api/org?asn=%2064500", "/api/org?asn=64500"},
			{"/api/net?not_ix=%2020", "/api/net?not_ix=20"},
			{"/api/fac?all_net=1_00", "/api/fac?all_net=100"},
			{"/api/ix?all_net=%D9%A1%D9%A0%D9%A0", "/api/ix?all_net=100"},
			{"/api/fac?org_present=%203", "/api/fac?org_present=3"},
			// asn_overlap compares network__asn, an integer lookup
			// (models.py:2464-2471, :2875-2881). The canonical requests
			// keep fac 400 and ix 20.
			{"/api/fac?asn_overlap=64500,64_501", "/api/fac?asn_overlap=64500,64501"},
			{"/api/ix?asn_overlap=%D9%A6%D9%A4%D9%A5%D9%A0%D9%A0,%2064501", "/api/ix?asn_overlap=64500,64501"},
		} {
			wantStatus, wantBody := httpGet(t, srv, tc.canonical)
			want := extractIDs(t, wantBody)
			if wantStatus != http.StatusOK || len(want) == 0 {
				t.Fatalf("GET %s: status %d ids %v, want 200 and a row", tc.canonical, wantStatus, want)
			}
			assertKeysResolve(t, srv, []silentIgnoreCase{{path: tc.path, want: want}})
		}
		// A count seed of a prepare_query uses the first value of a
		// repeated key (serializers.py:618-619).
		assertKeysResolve(t, srv, []silentIgnoreCase{
			{path: "/api/fac?net_count=2&net_count=0", want: []int{400}},
		})
		// The ix capacity key converts its value with int() too
		// (serializers.py:4545-4546, models.py:2927-2931). See
		// seedCapacity for the rows.
		capSrv := newTestServer(t, seedCapacity(t, t0))
		for _, tc := range []struct {
			path string
			want []int
		}{
			{"/api/ix?capacity__gte=1_000", []int{20}},
			{"/api/ix?capacity__gte=1000", []int{20}},
			{"/api/ix?capacity=%D9%A5%D9%A0%D9%A0", []int{21}},
			{"/api/ix?capacity=500", []int{21}},
			{"/api/ix?capacity__in=%D9%A5%D9%A0%D9%A0,0_0", []int{21, 22}},
		} {
			assertKeysResolve(t, capSrv, []silentIgnoreCase{{path: tc.path, want: tc.want}})
		}
	})

	t.Run("unique_key_non_integer_with_bad_operator_is_400", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:693-703 runs every filter in one
		// qset.filter() call, so the ValueError of asn__lt is a 400
		// even though id=abc matches no row. The mirror parses the keys
		// in map order, so run the request many times.
		c := testutil.SetupClient(t)
		seedNet(t, c, 1, 64501, "ok", t0)
		srv := newTestServer(t, c)
		for range 20 {
			status, body := httpGet(t, srv, "/api/net?id=abc&asn__lt=x")
			if status != http.StatusBadRequest {
				t.Fatalf("GET /api/net?id=abc&asn__lt=x: status = %d, want 400; body=%s", status, string(body))
			}
		}
	})

	t.Run("DIVERGENCE_unknown_path_json_404", func(t *testing.T) {
		t.Parallel()
		// DIVERGENCE: upstream has no route for an unknown type, so
		// Django serves the 404 HTML page of the web site (2.83.0
		// mainsite/urls.py:111, views.py:336-340). The mirror sends a
		// JSON 404 in the /api/ error form, with the same status. See
		// docs/API.md § Known Divergences.
		srv := newTestServer(t, testutil.SetupClient(t))
		status, hdr, body := httpDo(t, srv, http.MethodGet, "/api/foo", nil)
		if status != http.StatusNotFound {
			t.Fatalf("GET /api/foo: status = %d, want 404; body=%s", status, string(body))
		}
		if ct := hdr.Get("Content-Type"); ct != "application/json" {
			t.Errorf("GET /api/foo: Content-Type = %q, want application/json", ct)
		}
		if got := mustDecodeMetaError(t, body).Error; got == "" {
			t.Errorf("GET /api/foo: meta.error is empty")
		}
	})

	t.Run("DIVERGENCE_non_get_method_405_read_only", func(t *testing.T) {
		t.Parallel()
		// DIVERGENCE: upstream admits an anonymous write through its
		// permission classes (2.83.0 rest.py:396-405, :438-451,
		// permissions.py:282-302) and runs the handler: POST with no
		// body is a 400 (rest.py:867-924), PATCH a 403 (:970-974),
		// DELETE a 204, 403 or 400 (:978-1020), and OPTIONS a 200 with
		// DRF metadata (DRF views.py:531-538). Every upstream response
		// lists the methods of the route in Allow (DRF views.py:157-164).
		// The as_set lookup (rest.py:1396-1399) maps only GET, so
		// upstream sends Allow: GET there, and an anonymous write fails
		// the permission check first: 401 (drf views.py:174-180,
		// permissions.py:191-199; tests/test_api_cache_keys.py:222-245).
		// The mirror is read-only: every method other than GET and HEAD
		// gets 405, with Allow: GET, HEAD, or Allow: GET on as_set. See
		// docs/API.md § Known Divergences.
		c := testutil.SetupClient(t)
		seedNet(t, c, 1, 64501, "ok", t0)
		srv := newTestServer(t, c)
		for _, tc := range []struct{ method, path, wantAllow string }{
			{http.MethodPost, "/api/net", "GET, HEAD"},
			{http.MethodPatch, "/api/net/1", "GET, HEAD"},
			{http.MethodDelete, "/api/net/1", "GET, HEAD"},
			{http.MethodOptions, "/api/net", "GET, HEAD"},
			{http.MethodPost, "/api/as_set", "GET"},
		} {
			status, hdr, body := httpDo(t, srv, tc.method, tc.path, nil)
			if status != http.StatusMethodNotAllowed {
				t.Errorf("%s %s: status = %d, want 405; body=%s", tc.method, tc.path, status, string(body))
				continue
			}
			if got := hdr.Get("Allow"); got != tc.wantAllow {
				t.Errorf("%s %s: Allow = %q, want %q", tc.method, tc.path, got, tc.wantAllow)
			}
			want := "Method \"" + tc.method + "\" not allowed."
			if got := mustDecodeMetaError(t, body).Error; got != want {
				t.Errorf("%s %s: meta.error = %q, want %q", tc.method, tc.path, got, want)
			}
		}
	})

	t.Run("as_set_head_and_options_405_allow_get", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:1399 at 2.83.0 (http_method_names =
		// ["get"]). DRF compares the method with that list after the
		// permission check, which a read method passes, so HEAD and
		// OPTIONS get 405 (drf views.py:513-521, :167-172) with
		// Allow: GET (views.py:158-164, :448-449). net/http sends no
		// body for HEAD.
		c := testutil.SetupClient(t)
		seedNet(t, c, 1, 64501, "ok", t0)
		srv := newTestServer(t, c)
		for _, tc := range []struct{ method, path string }{
			{http.MethodHead, "/api/as_set"},
			{http.MethodHead, "/api/as_set/64501"},
			{http.MethodOptions, "/api/as_set"},
		} {
			status, hdr, body := httpDo(t, srv, tc.method, tc.path, nil)
			if status != http.StatusMethodNotAllowed {
				t.Errorf("%s %s: status = %d, want 405; body=%s", tc.method, tc.path, status, string(body))
				continue
			}
			if got := hdr.Get("Allow"); got != "GET" {
				t.Errorf("%s %s: Allow = %q, want GET", tc.method, tc.path, got)
			}
			if tc.method == http.MethodHead {
				continue
			}
			want := "Method \"" + tc.method + "\" not allowed."
			if got := mustDecodeMetaError(t, body).Error; got != want {
				t.Errorf("%s %s: meta.error = %q, want %q", tc.method, tc.path, got, want)
			}
		}
	})

	t.Run("as_set_detail_ignores_status", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:1405-1406, :1416 at 2.83.0, handleref
		// models.py:92-93. retrieve() uses Network.objects with no status
		// filter, so a deleted or pending network returns its set. The
		// list takes only status ok.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		for _, n := range []struct {
			id, asn          int
			status, irrAsSet string
		}{
			{1, 64500, "ok", "AS-ONE"},
			{3, 64502, "deleted", "AS-GONE"},
			{4, 64503, "pending", "AS-PEND"},
		} {
			if _, err := c.Network.Create().
				SetID(n.id).SetName("StatusNet").SetNameFold(unifold.Fold("StatusNet")).
				SetAsn(n.asn).SetIrrAsSet(n.irrAsSet).SetStatus(n.status).
				SetCreated(t0).SetUpdated(t0).
				Save(ctx); err != nil {
				t.Fatalf("seed net id=%d: %v", n.id, err)
			}
		}
		srv := newTestServer(t, c)

		for path, want := range map[string]string{
			"/api/as_set/64502": `{"meta":{},"data":[{"64502":"AS-GONE"}]}`,
			"/api/as_set/64503": `{"meta":{},"data":[{"64503":"AS-PEND"}]}`,
			"/api/as_set":       `{"meta":{},"data":[{"64500":"AS-ONE"}]}`,
		} {
			status, body := httpGet(t, srv, path)
			if status != http.StatusOK {
				t.Errorf("GET %s: status = %d, want 200; body=%s", path, status, body)
				continue
			}
			if string(body) != want {
				t.Errorf("GET %s: body = %s, want %s", path, body, want)
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

	t.Run("detail_filter_on_hidden_poc_no_oracle", func(t *testing.T) {
		t.Parallel()
		// A filter on a detail request for a contact that the caller's
		// tier cannot read gives the same 404 as no filter, whether or
		// not the filter matches the hidden row. Upstream runs the
		// filters before the permission check and answers 404 for a
		// miss and 403 for a match, which tells a guest whether a
		// guessed value is right. The mirror applies the poc privacy
		// policy in the filter query too.
		// synthesised: mirror privacy invariant; upstream
		// rest.py:855-865 answers 403/404.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		seedNet(t, c, 1, 64501, "ok", t0)
		for id, row := range map[int]struct{ visible, name string }{
			10: {"Users", "Hidden"},
			11: {"Public", "Shown"},
		} {
			if _, err := c.Poc.Create().
				SetID(id).SetNetID(1).SetRole("NOC").SetVisible(row.visible).
				SetName(row.name).SetEmail("poc@example.invalid").
				SetStatus("ok").SetCreated(t0).SetUpdated(t0).
				Save(ctx); err != nil {
				t.Fatalf("seed poc id=%d: %v", id, err)
			}
		}

		public := newTestServerWithTier(t, c, privctx.TierPublic)
		_, bare := httpGet(t, public, "/api/poc/10")
		for _, path := range []string{"/api/poc/10?name=Hidden", "/api/poc/10?name=Other"} {
			status, body := httpGet(t, public, path)
			if status != http.StatusNotFound {
				t.Errorf("public GET %s: status = %d, want 404; body=%s", path, status, string(body))
			}
			if string(body) != string(bare) {
				t.Errorf("public GET %s: body %s differs from /api/poc/10 %s", path, body, bare)
			}
		}
		if status, body := httpGet(t, public, "/api/poc/11?name=Shown"); status != http.StatusOK {
			t.Errorf("public GET /api/poc/11?name=Shown: status = %d, want 200; body=%s", status, string(body))
		}

		users := newTestServerWithTier(t, c, privctx.TierUsers)
		for path, want := range map[string]int{
			"/api/poc/10?name=Hidden": http.StatusOK,
			"/api/poc/10?name=Other":  http.StatusNotFound,
		} {
			if status, body := httpGet(t, users, path); status != want {
				t.Errorf("users GET %s: status = %d, want %d; body=%s", path, status, want, string(body))
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

		// On an int field, __iexact matches the decimal text of the
		// value, as a key without an operator does, so a value that is
		// not an integer matches no row. On a key that names a FK,
		// __iexact is an exact id match. __icontains is a 400 (the int
		// column has no substring match). Upstream ignores these keys
		// and returns every net [100 101 102].
		fk := newTestServer(t, seedFKKeys(t, t0))
		assertKeysResolve(t, fk, []silentIgnoreCase{
			{path: "/api/net?org__iexact=1", want: []int{100}},
			{path: "/api/net?asn__iexact=64501", want: []int{101}},
			{path: "/api/net?asn__iexact=abc", want: []int{}},
			{path: "/api/net?asn__iexact=064501", want: []int{}},
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

	t.Run("detail_applies_list_filters", func(t *testing.T) {
		t.Parallel()
		// A single-object GET applies the filters of a list. retrieve
		// calls DRF get_object, which filters get_queryset() and then
		// looks up the pk. A filter that excludes the object gives the
		// same 404 as an id that does not exist. The detail status set
		// (live + pending) ANDs with a ?status= filter, and ?since= does
		// not widen it. ?q= is ignored.
		// upstream: 2.83.0 rest.py:849-855 (retrieve -> get_object),
		// :477-703 (filters), :718-750 (detail status set), :566 (q);
		// drf generics.py:79-105; django/shortcuts.py:90-93
		c := testutil.SetupClient(t)
		seedNetIXLanMix(t, c)
		if err := c.NetworkIxLan.UpdateOneID(1).SetIpaddr6("2001:7f8::1").Exec(t.Context()); err != nil {
			t.Fatalf("set netixlan 1 ipaddr6: %v", err)
		}
		mustIxPfx(t.Context(), t, c, 1, "10.0.0.0/24", 1, t0)
		// Fac 1 has a netfac of net 1 (asn 64500).
		mustFac(t.Context(), t, c, 1, "StatusFac", 1, t0)
		c.NetworkFacility.Create().
			SetID(1).SetNetID(1).SetFacID(1).SetLocalAsn(64500).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(t.Context())
		srv := newTestServer(t, c)
		// The PK miss of each path, sent to a server with no rows.
		empty := newTestServer(t, testutil.SetupClient(t))

		cases := []struct {
			path    string
			want    int
			wantIDs []int
			wantErr string
		}{
			{path: "/api/net/1?name=nomatch", want: http.StatusNotFound, wantErr: "No Network matches the given query."},
			{path: "/api/net/1?name=NetIXLanNet", want: http.StatusOK, wantIDs: []int{1}},
			{path: "/api/net/1?name__contains=ixlan", want: http.StatusOK, wantIDs: []int{1}},
			{path: "/api/net/1?asn=64500", want: http.StatusOK, wantIDs: []int{1}},
			// netixlan 2 is not-operational, 3 pending, 4 deleted.
			{path: "/api/netixlan/2?status=ok", want: http.StatusNotFound, wantErr: "No NetworkIXLan matches the given query."},
			{path: "/api/netixlan/2?status=not-operational", want: http.StatusOK, wantIDs: []int{2}},
			{path: "/api/netixlan/3?status=pending", want: http.StatusOK, wantIDs: []int{3}},
			{path: "/api/netixlan/4?status=deleted", want: http.StatusNotFound, wantErr: "No NetworkIXLan matches the given query."},
			// since is checked, then ignored: the detail status set
			// never admits deleted (rest.py:718).
			{path: "/api/netixlan/1?since=1", want: http.StatusOK, wantIDs: []int{1}},
			{path: "/api/netixlan/4?since=1", want: http.StatusNotFound, wantErr: "No NetworkIXLan matches the given query."},
			{path: "/api/net/1?q=nomatch", want: http.StatusOK, wantIDs: []int{1}},
			{path: "/api/net/1?id=2", want: http.StatusNotFound, wantErr: "No Network matches the given query."},
			{path: "/api/net/1?id=1", want: http.StatusOK, wantIDs: []int{1}},
			// A plain key on an integer field is __iexact upstream: the
			// value is not converted and matches nothing
			// (rest.py:670-683, django/db/models/lookups.py:430-432).
			{path: "/api/net/1?id=abc", want: http.StatusNotFound, wantErr: "No Network matches the given query."},
			// name IN ('') matches only an empty name.
			{path: "/api/net/1?name__in=", want: http.StatusNotFound, wantErr: "No Network matches the given query."},
			{path: "/api/net/1?depth=0&name=nomatch", want: http.StatusNotFound, wantErr: "No Network matches the given query."},
			{path: "/api/net/1?fields=id&name=NetIXLanNet", want: http.StatusOK, wantIDs: []int{1}},
			// The bare ipaddr6 value is canonicalized before it filters
			// (rest.py:605-606, util.py:61-73).
			{path: "/api/netixlan/1?ipaddr6=2001:7F8:0:0::1", want: http.StatusOK, wantIDs: []int{1}},
			{path: "/api/netixlan/1?ipaddr6=2001:7f8:0:0::2", want: http.StatusNotFound, wantErr: "No NetworkIXLan matches the given query."},
			// ipblock is a text prefix match on the ixpfx prefix of the
			// exchange (serializers.py:4548-4552, models.py:2830-2843).
			{path: "/api/ix/1?ipblock=10.0.0.0", want: http.StatusOK, wantIDs: []int{1}},
			{path: "/api/ix/1?ipblock=10.0.0.5", want: http.StatusNotFound, wantErr: "No InternetExchange matches the given query."},
			// whereis keeps the prefixes that contain the address
			// (serializers.py:4154-4168, models.py:5179-5197).
			{path: "/api/ixpfx/1?whereis=10.0.0.5", want: http.StatusOK, wantIDs: []int{1}},
			{path: "/api/ixpfx/1?whereis=10.1.0.5", want: http.StatusNotFound, wantErr: "No IXLanPrefix matches the given query."},
			// capacity is the sum of the speed of the netixlans that
			// are not deleted: 1, 2 and 3 on ixlan 1, 10000 each
			// (serializers.py:4545-4546, models.py:2895-2942).
			{path: "/api/ix/1?capacity=30000", want: http.StatusOK, wantIDs: []int{1}},
			{path: "/api/ix/1?capacity__gt=30000", want: http.StatusNotFound, wantErr: "No InternetExchange matches the given query."},
			// asn_overlap keeps the facilities that the network of
			// every listed ASN reaches (serializers.py:2126-2129,
			// models.py:2436-2483). Two items for one ASN count as one.
			{path: "/api/fac/1?asn_overlap=64500,%2064500", want: http.StatusOK, wantIDs: []int{1}},
			{path: "/api/fac/1?asn_overlap=64500,64501", want: http.StatusNotFound, wantErr: "No Facility matches the given query."},
		}
		for _, tc := range cases {
			status, body := httpGet(t, srv, tc.path)
			if status != tc.want {
				t.Errorf("GET %s: status = %d, want %d; body=%s", tc.path, status, tc.want, string(body))
				continue
			}
			if tc.want == http.StatusOK {
				if ids := extractIDs(t, body); !equalIntSlice(ids, tc.wantIDs) {
					t.Errorf("GET %s: got %v, want %v", tc.path, ids, tc.wantIDs)
				}
				continue
			}
			if got := mustDecodeMetaError(t, body).Error; got != tc.wantErr {
				t.Errorf("GET %s: meta.error = %q, want %q", tc.path, got, tc.wantErr)
			}
			pk, _, _ := strings.Cut(tc.path, "?")
			if missStatus, missBody := httpGet(t, empty, pk); missStatus != http.StatusNotFound || string(missBody) != string(body) {
				t.Errorf("GET %s: body %s differs from the PK miss %s (status %d)", tc.path, body, missBody, missStatus)
			}
		}
		// A prepare_query error is a 400 on a single-object GET too
		// (rest.py:493-500): get_object filters get_queryset().
		for _, tc := range []struct{ path, wantErr string }{
			{"/api/ixpfx/1?whereis=abc", "does not appear to be an IPv4 or IPv6 address"},
			{"/api/ix/1?capacity=abc", "is not an integer"},
			// One ASN (models.py:2457-2458).
			{"/api/fac/1?asn_overlap=64500", "Need to specify at least two asns"},
		} {
			status, body := httpGet(t, srv, tc.path)
			if status != http.StatusBadRequest {
				t.Errorf("GET %s: status = %d, want 400; body=%s", tc.path, status, string(body))
				continue
			}
			if msg := mustDecodeMetaError(t, body).Error; !strings.Contains(msg, tc.wantErr) {
				t.Errorf("GET %s: meta.error = %q, want it to contain %q", tc.path, msg, tc.wantErr)
			}
		}
	})

	t.Run("detail_validates_pagination_and_since", func(t *testing.T) {
		t.Parallel()
		// A single-object GET parses since, skip, limit and depth as a
		// list does (rest.py:505-523). The query is sliced before get()
		// (:755-760), and Django cannot filter a sliced query, so a
		// limit or skip above 0 is a 404 with the DRF default text,
		// whether or not the object exists. A negative limit does not
		// slice (limit > 0 is false).
		// upstream: 2.83.0 rest.py:505-523, :755-760; drf
		// generics.py:13-21, exceptions.py:188-191;
		// django/db/models/query.py:1505-1507
		c := testutil.SetupClient(t)
		seedNet(t, c, 1, 64500, "ok", t0)
		srv := newTestServer(t, c)

		cases := []struct {
			path    string
			want    int
			wantErr string
		}{
			{"/api/net/1?since=abc", http.StatusBadRequest, "'since' needs to be a unix timestamp (epoch seconds)"},
			{"/api/net/1?since=", http.StatusBadRequest, "'since' needs to be a unix timestamp (epoch seconds)"},
			{"/api/net/1?limit=abc", http.StatusBadRequest, "'limit' needs to be a number"},
			{"/api/net/1?limit=", http.StatusBadRequest, "'limit' needs to be a number"},
			{"/api/net/1?skip=abc", http.StatusBadRequest, "'skip' needs to be a number"},
			{"/api/net/1?skip=", http.StatusBadRequest, "'skip' needs to be a number"},
			// The list order: skip before since.
			{"/api/net/1?since=abc&skip=abc", http.StatusBadRequest, "'skip' needs to be a number"},
			{"/api/net/1?limit=1", http.StatusNotFound, "Not found."},
			{"/api/net/1?skip=1", http.StatusNotFound, "Not found."},
			{"/api/net/999?limit=1", http.StatusNotFound, "Not found."},
			{"/api/net/1?limit=-1&skip=1", http.StatusNotFound, "Not found."},
			{"/api/net/1?limit=0&skip=0", http.StatusOK, ""},
			{"/api/net/1?limit=-1", http.StatusOK, ""},
			// depth is parsed after since and before the filters
			// (rest.py:520-523). The serializer clamps a number.
			{"/api/net/1?depth=abc", http.StatusBadRequest, "'depth' needs to be a number"},
			{"/api/net/1?depth=", http.StatusBadRequest, "'depth' needs to be a number"},
			{"/api/net/1?depth=abc&since=abc", http.StatusBadRequest, "'since' needs to be a unix timestamp (epoch seconds)"},
			{"/api/net/1?depth=abc&name=nomatch", http.StatusBadRequest, "'depth' needs to be a number"},
			{"/api/net/1?depth=abc&depth=0", http.StatusOK, ""},
			{"/api/net/1?depth=%D9%A1", http.StatusOK, ""},
			{"/api/net/1?depth=99", http.StatusOK, ""},
		}
		for _, tc := range cases {
			status, body := httpGet(t, srv, tc.path)
			if status != tc.want {
				t.Errorf("GET %s: status = %d, want %d; body=%s", tc.path, status, tc.want, string(body))
				continue
			}
			if tc.want == http.StatusOK {
				if ids := extractIDs(t, body); !equalIntSlice(ids, []int{1}) {
					t.Errorf("GET %s: got %v, want [1]", tc.path, ids)
				}
				continue
			}
			if got := mustDecodeMetaError(t, body).Error; got != tc.wantErr {
				t.Errorf("GET %s: meta.error = %q, want %q", tc.path, got, tc.wantErr)
			}
		}
	})

	t.Run("detail_non_integer_id_404", func(t *testing.T) {
		t.Parallel()
		// Upstream converts the pk with int() in get_object_or_404,
		// after get_queryset has checked the parameters and filters. A
		// ValueError becomes a bare Http404 with the DRF default text.
		// An id with a "." takes the format-suffix route, and the
		// renderer negotiation raises Http404 in initial(), before any
		// parameter check. A value that int() accepts is the pk, and a
		// value out of the integer range is an empty result.
		// upstream: drf generics.py:13-21, :87-100, routers.py:143,
		// urlpatterns.py:109, negotiation.py:80-88, views.py:408-411,
		// exceptions.py:188-191; django/db/models/fields/__init__.py:2123-2131,
		// django/db/models/lookups.py:461-494; 2.83.0 rest.py:505-523
		c := testutil.SetupClient(t)
		seedNet(t, c, 1, 64500, "ok", t0)
		srv := newTestServer(t, c)

		cases := []struct {
			path    string
			want    int
			wantErr string
		}{
			{"/api/net/abc", http.StatusNotFound, "Not found."},
			{"/api/net/1.5", http.StatusNotFound, "Not found."},
			{"/api/net/1_", http.StatusNotFound, "Not found."},
			// Upstream has no route for this path (HTML 404, see the
			// unknown-path row); only the status is parity.
			{"/api/net/1/extra", http.StatusNotFound, "Not found."},
			// A parameter error wins over an id without a ".".
			{"/api/net/abc?since=abc", http.StatusBadRequest, "'since' needs to be a unix timestamp (epoch seconds)"},
			{"/api/net/abc?depth=abc", http.StatusBadRequest, "'depth' needs to be a number"},
			// The format-suffix route wins over a parameter error.
			{"/api/net/1.5?since=abc", http.StatusNotFound, "Not found."},
			// The id wins over a filter that empties the result and
			// over a filter that excludes the object.
			{"/api/net/abc?name__in=", http.StatusNotFound, "Not found."},
			{"/api/net/abc?name=nomatch", http.StatusNotFound, "Not found."},
			{"/api/net/%D9%A1", http.StatusOK, ""},
			{"/api/net/+1", http.StatusOK, ""},
			{"/api/net/%201%20", http.StatusOK, ""},
			{"/api/net/0_1", http.StatusOK, ""},
			{"/api/net/-1", http.StatusNotFound, "No Network matches the given query."},
			{"/api/net/99999999999999999999", http.StatusNotFound, "No Network matches the given query."},
		}
		for _, tc := range cases {
			status, body := httpGet(t, srv, tc.path)
			if status != tc.want {
				t.Errorf("GET %s: status = %d, want %d; body=%s", tc.path, status, tc.want, string(body))
				continue
			}
			if tc.want == http.StatusOK {
				if ids := extractIDs(t, body); !equalIntSlice(ids, []int{1}) {
					t.Errorf("GET %s: got %v, want [1]", tc.path, ids)
				}
				continue
			}
			assertTopLevelKeys(t, body, "meta")
			if got := mustDecodeMetaError(t, body).Error; got != tc.wantErr {
				t.Errorf("GET %s: meta.error = %q, want %q", tc.path, got, tc.wantErr)
			}
		}
	})

	t.Run("DIVERGENCE_detail_upstream_server_errors", func(t *testing.T) {
		t.Parallel()
		// DIVERGENCE: upstream answers these single-object GETs with a
		// server error (500). retrieve has no error handler (2.83.0
		// rest.py:849-865), and DRF runs get_queryset outside its 404
		// wrapper (drf generics.py:87, :100). The mirror answers as a
		// list request does.
		// See docs/API.md § Known Divergences.
		// This test ASSERTS the divergence (it is NOT a parity match).
		c := testutil.SetupClient(t)
		ctx := t.Context()
		seedNet(t, c, 1, 64500, "ok", t0)
		seedCampus(t, c, 50, "ok", t0)
		for _, id := range []int{400, 401} {
			mustFac(ctx, t, c, id, fmt.Sprintf("ServerErrFac%d", id), 1, t0)
			c.Facility.UpdateOneID(id).SetCampusID(50).ExecX(ctx)
		}
		srv := newTestServer(t, c)

		cases := []struct {
			path    string
			want    int
			wantIDs []int
			wantErr string
		}{
			// Upstream: the negative slice raises ValueError in
			// get_queryset (rest.py:755-760,
			// django/db/models/query.py:410-417).
			{path: "/api/net/1?skip=-1", want: http.StatusBadRequest, wantErr: "Negative indexing is not supported."},
			// Upstream: the fac_set join returns the campus once for
			// each facility and no distinct() runs, so get() raises
			// MultipleObjectsReturned (rest.py:711-716,
			// django/db/models/query.py:638-641).
			{path: "/api/campus/50?facility__in=400,401", want: http.StatusOK, wantIDs: []int{50}},
			// Upstream: the filter error handler reads inst[0], which
			// raises TypeError (rest.py:693-701).
			{path: "/api/net/1?asn__lt=abc", want: http.StatusBadRequest, wantErr: "filter error: "},
			// Upstream: the same in the date branch (rest.py:647-651).
			{path: "/api/net/1?created__lt=x", want: http.StatusBadRequest, wantErr: "filter error: "},
			// Upstream: the filter runs before the slice, so the same.
			{path: "/api/net/1?limit=1&asn__lt=abc", want: http.StatusBadRequest, wantErr: "filter error: "},
			// Upstream: int("") raises ValueError, then inst[0] raises
			// TypeError (rest.py:665-666, :697).
			{path: "/api/net/1?asn__in=", want: http.StatusNotFound, wantErr: "No Network matches the given query."},
		}
		for _, tc := range cases {
			status, body := httpGet(t, srv, tc.path)
			if status != tc.want {
				t.Errorf("GET %s: status = %d, want %d (divergence canary); body=%s", tc.path, status, tc.want, string(body))
				continue
			}
			if tc.want == http.StatusOK {
				if ids := extractIDs(t, body); !equalIntSlice(ids, tc.wantIDs) {
					t.Errorf("GET %s: got %v, want %v", tc.path, ids, tc.wantIDs)
				}
				continue
			}
			if got := mustDecodeMetaError(t, body).Error; !strings.HasPrefix(got, tc.wantErr) {
				t.Errorf("GET %s: meta.error = %q, want prefix %q", tc.path, got, tc.wantErr)
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
