package parity

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/pdbcompat"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestParity_Serializer locks the field values and keys that the /api
// serializers render, where they differ from the stored row or depend on
// the caller.
//
// upstream: serializers.py and permissions.py at 2.83.0 (cited per
// sub-test).
func TestParity_Serializer(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)

	// seedIxfURLRows seeds one ix with four ixlans that differ only in
	// the IX-F member list URL and its visibility, and one ixpfx on the
	// empty Public ixlan.
	const (
		publicEmpty  = 1
		usersEmpty   = 2
		privateEmpty = 3
		publicURL    = 4
		urlValue     = "https://ixf.example.test/members.json"
	)
	seedIxfURLRows := func(t *testing.T) *ent.Client {
		t.Helper()
		c := testutil.SetupClient(t)
		ctx := t.Context()
		mustOrg(ctx, t, c, 1, "IXF Org", t0)
		mustIX(ctx, t, c, 1, "IXF IX", 1, t0)
		for _, r := range []struct {
			id      int
			visible string
			url     string
		}{
			{publicEmpty, "Public", ""},
			{usersEmpty, "Users", ""},
			{privateEmpty, "Private", ""},
			{publicURL, "Public", urlValue},
		} {
			c.IxLan.Create().
				SetID(r.id).SetIxID(1).
				SetIxfIxpMemberListURLVisible(r.visible).
				SetIxfIxpMemberListURL(r.url).
				SetStatus("ok").SetCreated(t0).SetUpdated(t0).
				SaveX(ctx)
		}
		mustIxPfx(ctx, t, c, 1, "192.0.2.0/24", publicEmpty, t0)
		return c
	}

	t.Run("ixlan_ixf_url_key_follows_permission", func(t *testing.T) {
		t.Parallel()
		// upstream: permissions.py:344-353 at 2.83.0 (handle_ixlan deletes
		// ixf_ixp_member_list_url only when the caller does not have the
		// permission for its visibility). The captured beta anon response
		// has a Public ixlan with "ixf_ixp_member_list_url": "".
		c := seedIxfURLRows(t)
		srv := newTestServer(t, c)

		// want: ixlan id -> the URL value, or nil for "key absent".
		want := map[int]any{
			publicEmpty:  "",
			usersEmpty:   nil,
			privateEmpty: nil,
			publicURL:    urlValue,
		}
		check := func(t *testing.T, where string, row map[string]any) {
			t.Helper()
			id := int(row["id"].(float64))
			w, tracked := want[id]
			if !tracked {
				t.Errorf("%s: unexpected ixlan id %d", where, id)
				return
			}
			if _, ok := row["ixf_ixp_member_list_url_visible"]; !ok {
				t.Errorf("%s ixlan %d: _visible key missing", where, id)
			}
			got, present := row["ixf_ixp_member_list_url"]
			switch {
			case w == nil && present:
				t.Errorf("%s ixlan %d: url key present (%#v), want absent", where, id, got)
			case w != nil && !present:
				t.Errorf("%s ixlan %d: url key absent, want %#v", where, id, w)
			case w != nil && got != w:
				t.Errorf("%s ixlan %d: url = %#v, want %#v", where, id, got, w)
			}
		}

		status, body := httpGet(t, srv, "/api/ixlan")
		if status != http.StatusOK {
			t.Fatalf("list status = %d, want 200; body=%s", status, body)
		}
		rows := decodeDataArray(t, body)
		if len(rows) != len(want) {
			t.Fatalf("list returned %d rows, want %d", len(rows), len(want))
		}
		for _, row := range rows {
			check(t, "list", row)
		}

		for id := range want {
			status, body := httpGet(t, srv, fmt.Sprintf("/api/ixlan/%d?depth=0", id))
			if status != http.StatusOK {
				t.Fatalf("detail %d status = %d; body=%s", id, status, body)
			}
			check(t, "detail", decodeDataArray(t, body)[0])
		}

		status, body = httpGet(t, srv, "/api/ix/1?depth=2")
		if status != http.StatusOK {
			t.Fatalf("ix detail status = %d; body=%s", status, body)
		}
		set, _ := decodeDataArray(t, body)[0]["ixlan_set"].([]any)
		if len(set) != len(want) {
			t.Fatalf("ix.ixlan_set has %d rows, want %d", len(set), len(want))
		}
		for _, e := range set {
			check(t, "ix.ixlan_set", e.(map[string]any))
		}

		status, body = httpGet(t, srv, "/api/ixpfx/1?depth=2")
		if status != http.StatusOK {
			t.Fatalf("ixpfx detail status = %d; body=%s", status, body)
		}
		lan, ok := decodeDataArray(t, body)[0]["ixlan"].(map[string]any)
		if !ok {
			t.Fatalf("ixpfx detail has no ixlan object; body=%s", body)
		}
		check(t, "ixpfx.ixlan", lan)
	})

	t.Run("DIVERGENCE_ixf_url_empty_value", func(t *testing.T) {
		t.Parallel()
		// upstream: permissions.py:344-353 at 2.83.0 keeps the key for a
		// caller with the permission. The model column is nullable
		// (django-peeringdb abstract.py:819-824), so an empty URL renders
		// as null or "" as stored. The mirror stores both as "": a Public
		// row renders "", and an empty Users row omits the key at every
		// tier, because an anonymous sync stores "" for every Users row.
		c := seedIxfURLRows(t)
		srv := newTierTestServer(t, c, privctx.TierUsers)

		status, body := httpGet(t, srv, "/api/ixlan")
		if status != http.StatusOK {
			t.Fatalf("list status = %d, want 200; body=%s", status, body)
		}
		for _, row := range decodeDataArray(t, body) {
			id := int(row["id"].(float64))
			got, present := row["ixf_ixp_member_list_url"]
			switch id {
			case publicEmpty:
				if got != "" {
					t.Errorf("Public empty row: url = %#v (present=%v), want \"\"", got, present)
				}
			case usersEmpty:
				if present {
					t.Errorf("Users empty row at TierUsers: url key present (%#v), want absent", got)
				}
			}
		}
	})

	t.Run("ix_media_and_ixlan_dot1q_are_constants", func(t *testing.T) {
		t.Parallel()
		// upstream: serializers.py:4497-4500 at 2.83.0 (get_media returns
		// "Ethernet", #1555) and serializers.py:4304-4307 (get_dot1q_support
		// returns False, #903). Both fields are deprecated, and the stored
		// value is not rendered.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		mustOrg(ctx, t, c, 1, "Const Org", t0)
		c.InternetExchange.Create().
			SetID(1).SetName("Const IX").SetOrgID(1).SetMedia("Fiber").
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		c.IxLan.Create().
			SetID(1).SetIxID(1).SetDot1qSupport(true).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		mustIxPfx(ctx, t, c, 1, "192.0.2.0/24", 1, t0)
		srv := newTestServer(t, c)

		// get fetches path and returns the first data row.
		get := func(path string) map[string]any {
			t.Helper()
			status, body := httpGet(t, srv, path)
			if status != http.StatusOK {
				t.Fatalf("GET %s: status = %d; body=%s", path, status, body)
			}
			return decodeDataArray(t, body)[0]
		}
		checkIX := func(where string, ix map[string]any) {
			t.Helper()
			if ix["media"] != "Ethernet" {
				t.Errorf("%s: media = %#v, want \"Ethernet\"", where, ix["media"])
			}
		}
		checkLan := func(where string, lan map[string]any) {
			t.Helper()
			if lan["dot1q_support"] != false {
				t.Errorf("%s: dot1q_support = %#v, want false", where, lan["dot1q_support"])
			}
		}

		checkIX("ix list", get("/api/ix"))
		checkIX("ix depth=0", get("/api/ix/1?depth=0"))
		checkIX("ix depth=1", get("/api/ix/1?depth=1"))
		ix2 := get("/api/ix/1?depth=2")
		checkIX("ix depth=2", ix2)
		set, _ := ix2["ixlan_set"].([]any)
		if len(set) != 1 {
			t.Fatalf("ix depth=2 ixlan_set has %d rows, want 1", len(set))
		}
		checkLan("ix.ixlan_set", set[0].(map[string]any))

		checkLan("ixlan list", get("/api/ixlan"))
		checkLan("ixlan depth=0", get("/api/ixlan/1?depth=0"))
		lan2 := get("/api/ixlan/1?depth=2")
		checkLan("ixlan depth=2", lan2)
		nestedIX, _ := lan2["ix"].(map[string]any)
		checkIX("ixlan.ix", nestedIX)

		pfx := get("/api/ixpfx/1?depth=2")
		nestedLan, _ := pfx["ixlan"].(map[string]any)
		checkLan("ixpfx.ixlan", nestedLan)
	})

	t.Run("ixpfx_tombstone_null_prefix", func(t *testing.T) {
		t.Parallel()
		// synthesised: upstream /api/ixpfx?since=1&status=deleted returns
		// tombstone 4185 with "prefix": null (live, 2026-09-25). Sync
		// decodes the null to "" and stores it. /api renders a stored ""
		// as null, with and without ?fields= projection. A live prefix is
		// unchanged.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		mustOrg(ctx, t, c, 1, "Pfx Org", t0)
		mustIX(ctx, t, c, 1, "Pfx IX", 1, t0)
		mustIxLan(ctx, t, c, 1, "Pfx Lan", 1, t0)
		mustIxPfx(ctx, t, c, 1, "192.0.2.0/24", 1, t0)
		c.IxPrefix.Create().
			SetID(2).SetPrefix("").SetProtocol("IPv4").SetIxlanID(1).
			SetStatus("deleted").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		srv := newTestServer(t, c)

		for _, path := range []string{
			fmt.Sprintf("/api/ixpfx?since=%d", t0.Unix()),
			fmt.Sprintf("/api/ixpfx?since=%d&fields=id,prefix", t0.Unix()),
		} {
			status, body := httpGet(t, srv, path)
			if status != http.StatusOK {
				t.Fatalf("GET %s: status = %d; body=%s", path, status, body)
			}
			byID := make(map[int]map[string]any)
			for _, row := range decodeDataArray(t, body) {
				if id, ok := row["id"].(float64); ok {
					byID[int(id)] = row
				}
			}
			if len(byID) != 2 {
				t.Fatalf("GET %s: got %d rows, want 2; body=%s", path, len(byID), body)
			}
			if got, ok := byID[2]["prefix"]; !ok || got != nil {
				t.Errorf("GET %s: tombstone prefix = %#v (present %v), want null", path, got, ok)
			}
			if got := byID[1]["prefix"]; got != "192.0.2.0/24" {
				t.Errorf("GET %s: live prefix = %#v, want \"192.0.2.0/24\"", path, got)
			}
		}

		// Upstream returns tombstone 4185 for each of these filters with an
		// empty value (live, 2026-09-25), so the stored "" must match them.
		for _, filter := range []string{
			"prefix=", "prefix__contains=", "prefix__startswith=", "prefix__in=,192.0.2.0/24",
		} {
			path := fmt.Sprintf("/api/ixpfx?since=%d&%s", t0.Unix(), filter)
			status, body := httpGet(t, srv, path)
			if status != http.StatusOK {
				t.Fatalf("GET %s: status = %d; body=%s", path, status, body)
			}
			if ids := extractIDs(t, body); !slices.Contains(ids, 2) {
				t.Errorf("GET %s: ids = %v, want the tombstone (2) included", path, ids)
			}
		}
	})

	t.Run("facility_link_sets_sort_by_facility_id", func(t *testing.T) {
		t.Parallel()
		// upstream: serializers.py:1140-1148 at 2.83.0 (the nested
		// prefetch has no ORDER BY) and models.py:3284, :5998, :6603 (the
		// unique (ix|network|carrier, facility) index that MySQL reads the
		// set through). Live /api/net/20?depth=2 and /api/ix/26?depth=2
		// return these sets in facility-id order. The link ids below run
		// opposite to their facility ids.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		mustOrg(ctx, t, c, 1, "Link Org", t0)
		mustNet(ctx, t, c, 1, "Link Net", 64501, 1, t0)
		mustIX(ctx, t, c, 1, "Link IX", 1, t0)
		c.Carrier.Create().SetID(1).SetName("Link Carrier").SetOrgID(1).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		facIDs := []int{7, 64, 440}
		linkIDs := []int{300, 200, 100}
		for i, facID := range facIDs {
			mustFac(ctx, t, c, facID, fmt.Sprintf("Link Fac %d", facID), 1, t0)
			c.NetworkFacility.Create().SetID(linkIDs[i]).SetNetID(1).SetFacID(facID).SetLocalAsn(64501).
				SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
			c.IxFacility.Create().SetID(linkIDs[i]).SetIxID(1).SetFacID(facID).
				SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
			c.CarrierFacility.Create().SetID(linkIDs[i]).SetCarrierID(1).SetFacID(facID).
				SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		}
		srv := newTestServer(t, c)

		for _, tc := range []struct {
			path, set string
			want      []int
		}{
			{"/api/net/1?depth=1", "netfac_set", linkIDs},
			{"/api/net/1?depth=2", "netfac_set", linkIDs},
			{"/api/ix/1?depth=1", "fac_set", facIDs},
			{"/api/ix/1?depth=2", "fac_set", facIDs},
			{"/api/carrier/1?depth=1", "carrierfac_set", linkIDs},
			{"/api/carrier/1?depth=2", "carrierfac_set", linkIDs},
		} {
			status, body := httpGet(t, srv, tc.path)
			if status != http.StatusOK {
				t.Fatalf("GET %s: status = %d; body=%s", tc.path, status, body)
			}
			raw, _ := decodeDataArray(t, body)[0][tc.set].([]any)
			got := make([]int, 0, len(raw))
			for _, e := range raw {
				switch v := e.(type) {
				case float64:
					got = append(got, int(v))
				case map[string]any:
					got = append(got, int(v["id"].(float64)))
				}
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("%s %s = %v, want %v", tc.path, tc.set, got, tc.want)
			}
		}
	})

	t.Run("net_info_types_is_a_list", func(t *testing.T) {
		t.Parallel()
		// upstream: serializers.py:3947-3960 at 2.83.0 renders info_types
		// as a list, [] when empty. The column is non-null with blank=True
		// (django-peeringdb abstract.py:471-477). The beta anon capture has
		// no null info_types.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		mustOrg(ctx, t, c, 1, "Types Org", t0)
		mustNet(ctx, t, c, 1, "Types Net", 64501, 1, t0) // no info_types stored
		srv := newTestServer(t, c)

		for _, path := range []string{"/api/net", "/api/net/1?depth=0", "/api/net/1?depth=2", "/api/org/1?depth=2"} {
			status, body := httpGet(t, srv, path)
			if status != http.StatusOK {
				t.Fatalf("GET %s: status = %d; body=%s", path, status, body)
			}
			row := decodeDataArray(t, body)[0]
			if path == "/api/org/1?depth=2" {
				set, _ := row["net_set"].([]any)
				if len(set) != 1 {
					t.Fatalf("org net_set has %d rows, want 1", len(set))
				}
				row = set[0].(map[string]any)
			}
			got, ok := row["info_types"].([]any)
			if !ok || len(got) != 0 {
				t.Errorf("GET %s: info_types = %#v, want []", path, row["info_types"])
			}
		}
	})
}

// newTierTestServer is newTestServer with the privacy tier stamped on
// each request context, as middleware.PrivacyTier does in production.
func newTierTestServer(t testing.TB, c *ent.Client, tier privctx.Tier) *httptest.Server {
	t.Helper()
	h := pdbcompat.NewHandler(c, 0)
	mux := http.NewServeMux()
	h.Register(mux)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(privctx.WithTier(r.Context(), tier)))
	}))
	t.Cleanup(srv.Close)
	return srv
}
