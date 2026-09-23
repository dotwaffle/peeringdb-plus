package parity

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// TestParity_Status locks the upstream 2.83.0 rest.py:719-750 status ×
// since matrix against future regression. Each subtest covers one cell
// of the matrix:
//
//	{no since, since=N} × {list, pk-lookup} × {campus, netixlan, other}
//
// The matrix is built from the type's live statuses (models.py:109-122):
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
			t.Errorf("list w/o since: got ids %v, want [1] (only status=ok)", ids)
		}
	})

	t.Run("pk_lookup_admits_pending", func(t *testing.T) {
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
			t.Errorf("pk lookup on pending: got %d, want 200; body=%s",
				status, string(body))
		}
	})

	t.Run("pk_lookup_deleted_returns_404", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:702-710 (pk-lookup branch excludes deleted)
		// upstream: pdb_api_test.py:1247 (deleted row pk lookup → 404)
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
		// upstream: rest.py:712-715 (since>0 admits ok+deleted, excludes
		// pending — except for campus carve-out)
		// upstream: pdb_api_test.py:1317 (since-window list assertion)
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
		// upstream: rest.py:712-715 (campus carve-out: pending admitted
		// on since>0 list — the IXP onboarding workflow needs pending
		// campuses to sync within the cycle window)
		// upstream: pdb_api_test.py:3965 (campus list with mixed statuses)
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
		// upstream: rest.py:696 (`if since > 0` — since=0 never
		// activates the matrix; the plain status='ok' list serves).
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

	t.Run("since_boundary_is_strictly_greater", func(t *testing.T) {
		t.Parallel()
		// upstream: django-handleref manager.py since() filters
		// Q(updated__gt=timestamp) — strictly greater, so a poller
		// re-using the max updated it has already seen does NOT
		// re-receive the boundary row.
		c := testutil.SetupClient(t)
		seedNet(t, c, 1, 64501, "ok", t0)                // exactly at the boundary
		seedNet(t, c, 2, 64502, "ok", t0.Add(time.Hour)) // inside the window

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, fmt.Sprintf("/api/net?since=%d", t0.Unix()))
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		ids := extractIDs(t, body)
		want := []int{2}
		if !equalIntSlice(ids, want) {
			t.Errorf("since boundary must be exclusive (updated > since): got %v, want %v", ids, want)
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
		// Default order is updated descending.
		want := []int{2, 1}
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
			{"?status__in=ok,not-operational", []int{2, 1}},
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

	t.Run("DIVERGENCE_poc_tombstone_outlives_upstream_retention", func(t *testing.T) {
		t.Parallel()
		// DIVERGENCE: upstream hard deletes a soft-deleted poc when its
		// updated value is POC_DELETION_PERIOD (default 30 days) old. It
		// is the only upstream hard delete. After that, a ?since= window
		// that covers the deletion no longer returns the tombstone. The
		// mirror keeps every tombstone, so the window still returns it.
		// See docs/API.md § Known Divergences.
		// This test ASSERTS the divergence (it is NOT a parity match).
		// upstream: 2.83.0 management/commands/pdb_delete_pocs.py:34-38,58
		// + mainsite/settings/__init__.py:684 (30 days)
		// + docs/api/obj_poc.md:14-17
		c := testutil.SetupClient(t)
		ctx := t.Context()
		deletedAt := time.Now().UTC().Add(-90 * 24 * time.Hour).Truncate(time.Second)
		seedNet(t, c, 1, 64501, "ok", t0)
		// Upstream blanks name, phone, email and url on soft delete
		// (docs/api/obj_poc.md:12-13); the ent defaults are "".
		if _, err := c.Poc.Create().
			SetID(10).SetNetID(1).SetRole("NOC").SetVisible("Public").
			SetStatus("deleted").SetCreated(t0).SetUpdated(deletedAt).
			Save(ctx); err != nil {
			t.Fatalf("seed poc tombstone: %v", err)
		}

		srv := newTestServer(t, c)
		path := fmt.Sprintf("/api/poc?since=%d", deletedAt.Add(-24*time.Hour).Unix())
		status, body := httpGet(t, srv, path)
		if status != http.StatusOK {
			t.Fatalf("GET %s: status = %d; body=%s", path, status, string(body))
		}
		// Upstream returns [] here: the tombstone is 90 days old.
		if ids := extractIDs(t, body); !equalIntSlice(ids, []int{10}) {
			t.Errorf("GET %s: got %v, want [10] (retained tombstone; divergence canary)", path, ids)
		}
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
