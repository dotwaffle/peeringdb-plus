package parity

import (
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// TestParity_Ordering locks the list order of the pdbcompat /api
// surface.
//
// A list without ?since is ordered by id ascending. Upstream adds no
// ORDER BY to a plain list: get_queryset only filters on the live
// statuses (2.83.0 rest.py:747-748). None of the 13 models declares
// Meta.ordering. The django-peeringdb abstract bases declare their own
// `class Meta` without subclassing the django-handleref Meta, so the
// handleref ordering is not inherited, and the upstream migrations
// record no "ordering" option for these models (only the API-key models
// in migrations/0064_api_keys.py have one). MySQL then serves the rows
// in primary-key order. A live capture on 2026-09-23 of
// /api/<type>?limit=2 returned the two lowest ids on every type except
// netixlan (see DIVERGENCE_netixlan_list_order_is_id_asc).
//
// A ?since= list is ordered by updated ascending (rest.py:744). The
// subtests in status_test.go lock that order. The id tiebreak on rows
// with the same updated value is the mirror's own rule.
func TestParity_Ordering(t *testing.T) {
	t.Parallel()

	// Base timestamp; spread updated by 1h so SQLite time precision is
	// never a factor.
	t0 := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)

	seedOrg := func(t *testing.T, c *ent.Client, id int, updated time.Time) {
		t.Helper()
		name := fmt.Sprintf("OrderOrg%d", id)
		if _, err := c.Organization.Create().
			SetID(id).SetName(name).SetNameFold(unifold.Fold(name)).
			SetStatus("ok").SetCreated(updated).SetUpdated(updated).
			Save(t.Context()); err != nil {
			t.Fatalf("seed org %d: %v", id, err)
		}
	}

	t.Run("default_list_order_id_asc", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:747-748 (a plain list filters on
		// the live statuses and adds no order_by)
		// upstream: 2.83.0 migrations/0001_initial.py (no "ordering"
		// option on the 13 models)
		c := testutil.SetupClient(t)
		ctx := t.Context()
		seedOrg(t, c, 1, t0)
		// The timestamps neither rise nor fall with the id: id ASC is
		// [5 10 20], updated or created DESC is [10 5 20], and ASC is
		// [20 5 10]. A timestamp sort key in either direction fails.
		for _, r := range []struct {
			id      int
			updated time.Time
		}{
			{20, t0},
			{5, t0.Add(1 * time.Hour)},
			{10, t0.Add(2 * time.Hour)},
		} {
			if _, err := c.Network.Create().
				SetID(r.id).SetName("Net").SetNameFold(unifold.Fold("Net")).
				SetAsn(64500 + r.id).SetStatus("ok").
				SetOrgID(1).
				SetCreated(r.updated).SetUpdated(r.updated).
				Save(ctx); err != nil {
				t.Fatalf("seed net %d: %v", r.id, err)
			}
		}

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/net")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", status, string(body))
		}
		want := []int{5, 10, 20}
		if got := extractIDs(t, body); !slices.Equal(got, want) {
			t.Errorf("default order: got %v, want %v", got, want)
		}
	})

	t.Run("default_list_order_id_asc_org", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:747-748; live capture 2026-09-23
		// /api/org?limit=2 returned ids 2 (updated 2023) and 7
		// (updated 2021), in id order and against updated order.
		// As in the capture, org 2 is newer than org 7. Org 3 is the
		// newest, so updated DESC is [3 2 7] and updated ASC is
		// [7 2 3], and only id order gives [2 3 7].
		c := testutil.SetupClient(t)
		seedOrg(t, c, 2, t0.Add(1*time.Hour))
		seedOrg(t, c, 7, t0)
		seedOrg(t, c, 3, t0.Add(2*time.Hour))

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/org")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", status, string(body))
		}
		want := []int{2, 3, 7}
		if got := extractIDs(t, body); !slices.Equal(got, want) {
			t.Errorf("default org order: got %v, want %v", got, want)
		}
	})

	t.Run("limit_skip_pages_follow_id_asc", func(t *testing.T) {
		t.Parallel()
		// upstream: pdb_api_test.py:4013-4019
		// (test_guest_005_list_pagination compares each skip/limit
		// page with Organization.objects.filter(status="ok"), a
		// queryset without an ORDER BY)
		c := testutil.SetupClient(t)
		// ids 1..7 with updated values in a shuffled order.
		for i, id := range []int{4, 1, 7, 2, 6, 3, 5} {
			seedOrg(t, c, id, t0.Add(time.Duration(i)*time.Hour))
		}

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/org?skip=3&limit=3")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", status, string(body))
		}
		want := []int{4, 5, 6}
		if got := extractIDs(t, body); !slices.Equal(got, want) {
			t.Errorf("skip=3&limit=3 page: got %v, want %v", got, want)
		}
	})

	t.Run("since_same_updated_tiebreak_id_asc", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:744 (a since list is ordered by
		// updated only)
		// synthesised: upstream leaves the order of rows with the same
		// updated value to MySQL. The mirror sorts them by id so that
		// a paged since window is stable.
		c := testutil.SetupClient(t)
		seedOrg(t, c, 9, t0.Add(time.Hour))
		seedOrg(t, c, 4, t0.Add(time.Hour))
		seedOrg(t, c, 6, t0)

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/org?since=1")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", status, string(body))
		}
		want := []int{6, 4, 9}
		if got := extractIDs(t, body); !slices.Equal(got, want) {
			t.Errorf("since order: got %v, want %v", got, want)
		}
	})

	t.Run("distance_list_order_nearest_first_id_tiebreak", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 serializers.py:1897 (order_by("distance")).
		// Fac 10 and 15 are at the same point. Upstream leaves rows at
		// the same distance to MySQL; the id tiebreak is the mirror's
		// choice, which keeps skip/limit pages stable.
		srv := newTestServer(t, seedDistanceRows(t, t0))
		const search = "/api/fac?latitude=50.110900&longitude=8.682100&distance=500"
		assertIDsInOrder(t, srv, []silentIgnoreCase{
			{path: search, want: []int{10, 15, 11, 12}},
			{path: search + "&limit=2", want: []int{10, 15}},
			{path: search + "&skip=2&limit=2", want: []int{11, 12}},
		})
	})

	t.Run("distance_since_keeps_updated_order", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:738-745 orders a ?since= list by
		// updated after prepare_query, and Django order_by replaces the
		// distance order (django/db/models/query.py:1721-1728). The
		// distance filter still applies, and the status matrix admits
		// the deleted fac 16 (rest.py:719-735). Updated: 12 t0, 11
		// t0+1h, 16 t0+2h, 10 and 15 t0+3h (id tiebreak).
		srv := newTestServer(t, seedDistanceRows(t, t0))
		assertIDsInOrder(t, srv, []silentIgnoreCase{
			{path: "/api/fac?since=1&latitude=50.110900&longitude=8.682100&distance=500", want: []int{12, 11, 16, 10, 15}},
		})
	})

	t.Run("DIVERGENCE_netixlan_list_order_is_id_asc", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:747-748 (no order_by) +
		// models.py:6111 (index netixlan_status). MySQL can read the
		// status IN ('ok', 'not-operational') filter through the
		// status index, which returns the not-operational rows first.
		// A live capture on 2026-09-23 of /api/netixlan?limit=2
		// returned ids 1145 and 1218, both not-operational, ahead of
		// ok rows with lower ids. The mirror always returns id order.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		seedOrg(t, c, 1, t0)
		c.Network.Create().
			SetID(1).SetName("OrderNet").SetNameFold(unifold.Fold("OrderNet")).
			SetAsn(64500).SetOrgID(1).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		c.InternetExchange.Create().
			SetID(1).SetName("OrderIX").SetNameFold(unifold.Fold("OrderIX")).
			SetOrgID(1).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		c.IxLan.Create().
			SetID(1).SetIxID(1).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		for _, r := range []struct {
			id     int
			status string
		}{
			{5, "ok"},
			{9, "not-operational"},
		} {
			c.NetworkIxLan.Create().
				SetID(r.id).SetNetID(1).SetIxlanID(1).SetIxID(1).
				SetAsn(64500).SetSpeed(10000).SetName("OrderIX").
				SetOperational(r.status == "ok").SetStatus(r.status).
				SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		}

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/netixlan")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", status, string(body))
		}
		want := []int{5, 9}
		if got := extractIDs(t, body); !slices.Equal(got, want) {
			t.Errorf("netixlan order: got %v, want %v (id order, not grouped by status)", got, want)
		}
	})
}
