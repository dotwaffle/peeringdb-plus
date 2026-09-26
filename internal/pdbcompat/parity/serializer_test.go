package parity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/pdbcompat"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
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

	// seedIxfURLRows seeds one ix with seven ixlans that differ only in
	// the IX-F member list URL and its visibility, one ixpfx on the empty
	// Public ixlan and one ixpfx on the NULL Public ixlan. A nil url
	// leaves the setter out, which stores NULL.
	const (
		publicEmpty  = 1
		usersEmpty   = 2
		privateEmpty = 3
		publicURL    = 4
		publicNull   = 5
		usersNull    = 6
		privateNull  = 7
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
			url     *string
		}{
			{publicEmpty, "Public", new("")},
			{usersEmpty, "Users", new("")},
			{privateEmpty, "Private", new("")},
			{publicURL, "Public", new(urlValue)},
			{publicNull, "Public", nil},
			{usersNull, "Users", nil},
			{privateNull, "Private", nil},
		} {
			c.IxLan.Create().
				SetID(r.id).SetIxID(1).
				SetIxfIxpMemberListURLVisible(r.visible).
				SetNillableIxfIxpMemberListURL(r.url).
				SetStatus("ok").SetCreated(t0).SetUpdated(t0).
				SaveX(ctx)
		}
		mustIxPfx(ctx, t, c, 1, "192.0.2.0/24", publicEmpty, t0)
		mustIxPfx(ctx, t, c, 2, "198.51.100.0/24", publicNull, t0)
		return c
	}

	// keyAbsent in a want map means "the key is not in the object". A
	// nil want means the key is present with JSON null.
	type keyAbsent struct{}
	checkIxfURL := func(t *testing.T, where string, want map[int]any, row map[string]any, needVisible bool) {
		t.Helper()
		id := int(row["id"].(float64))
		w, tracked := want[id]
		if !tracked {
			t.Errorf("%s: unexpected ixlan id %d", where, id)
			return
		}
		if _, ok := row["ixf_ixp_member_list_url_visible"]; needVisible && !ok {
			t.Errorf("%s ixlan %d: _visible key missing", where, id)
		}
		got, present := row["ixf_ixp_member_list_url"]
		_, wantAbsent := w.(keyAbsent)
		switch {
		case wantAbsent && present:
			t.Errorf("%s ixlan %d: url key present (%#v), want absent", where, id, got)
		case !wantAbsent && !present:
			t.Errorf("%s ixlan %d: url key absent, want %#v", where, id, w)
		case !wantAbsent && got != w:
			t.Errorf("%s ixlan %d: url = %#v, want %#v", where, id, got, w)
		}
	}

	t.Run("ixlan_ixf_url_key_follows_permission", func(t *testing.T) {
		t.Parallel()
		// upstream: permissions.py:344-353 (key deleted only without
		// permission); DRF serializers.py:548-550 (None renders null).
		// The captured beta anon response has a Public ixlan with
		// "ixf_ixp_member_list_url": "".
		c := seedIxfURLRows(t)
		srv := newTestServer(t, c)

		want := map[int]any{
			publicEmpty:  "",
			usersEmpty:   keyAbsent{},
			privateEmpty: keyAbsent{},
			publicURL:    urlValue,
			publicNull:   nil,
			usersNull:    keyAbsent{},
			privateNull:  keyAbsent{},
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
			checkIxfURL(t, "list", want, row, true)
		}

		for id := range want {
			status, body := httpGet(t, srv, fmt.Sprintf("/api/ixlan/%d?depth=0", id))
			if status != http.StatusOK {
				t.Fatalf("detail %d status = %d; body=%s", id, status, body)
			}
			checkIxfURL(t, "detail", want, decodeDataArray(t, body)[0], true)
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
			checkIxfURL(t, "ix.ixlan_set", want, e.(map[string]any), true)
		}

		for _, pfx := range []int{1, 2} {
			path := fmt.Sprintf("/api/ixpfx/%d?depth=2", pfx)
			status, body = httpGet(t, srv, path)
			if status != http.StatusOK {
				t.Fatalf("%s: status = %d; body=%s", path, status, body)
			}
			lan, ok := decodeDataArray(t, body)[0]["ixlan"].(map[string]any)
			if !ok {
				t.Fatalf("%s: no ixlan object; body=%s", path, body)
			}
			checkIxfURL(t, "ixpfx.ixlan", want, lan, true)
		}

		// ?fields= keeps the URL key and its value. Only the URL key is
		// checked: upstream also re-adds _visible here (row N5).
		status, body = httpGet(t, srv, "/api/ixlan?fields=id,ixf_ixp_member_list_url")
		if status != http.StatusOK {
			t.Fatalf("fields list status = %d; body=%s", status, body)
		}
		for _, row := range decodeDataArray(t, body) {
			checkIxfURL(t, "fields list", want, row, false)
		}
	})

	t.Run("ixf_url_users_tier_renders_stored_value", func(t *testing.T) {
		t.Parallel()
		// upstream: pdb_api_test.py:1442-1449 (a user sees Public and
		// Users URLs); permissions.py:344-353. The stored value renders
		// as is: "" stays "" and NULL renders null.
		c := seedIxfURLRows(t)
		srv := newTierTestServer(t, c, privctx.TierUsers)

		want := map[int]any{
			publicEmpty:  "",
			usersEmpty:   "",
			privateEmpty: keyAbsent{},
			publicURL:    urlValue,
			publicNull:   nil,
			usersNull:    nil,
			privateNull:  keyAbsent{},
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
			checkIxfURL(t, "list", want, row, true)
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
			checkIxfURL(t, "ix.ixlan_set", want, e.(map[string]any), true)
		}
	})

	t.Run("DIVERGENCE_ixf_url_users_row_null_after_anonymous_sync", func(t *testing.T) {
		t.Parallel()
		// upstream: an authenticated caller gets the stored value
		// (permissions.py:344-353); an anonymous sync never receives it,
		// so the mirror renders null. Registered in docs/API.md § Known
		// Divergences.
		// This test ASSERTS the divergence (it is NOT a parity match).
		c := testutil.SetupClient(t)
		ctx := t.Context()
		mustOrg(ctx, t, c, 1, "IXF Org", t0)
		mustIX(ctx, t, c, 1, "IXF IX", 1, t0)

		// An anonymous sync receives a Users row without the URL key.
		raw := `{"id":10,"ix_id":1,"name":"","descr":"","mtu":1500,` +
			`"ixf_ixp_member_list_url_visible":"Users",` +
			`"created":"2026-09-23T12:00:00Z","updated":"2026-09-23T12:00:00Z","status":"ok"}`
		var il peeringdb.IxLan
		if err := json.Unmarshal([]byte(raw), &il); err != nil {
			t.Fatalf("decode: %v", err)
		}
		c.IxLan.Create().
			SetID(il.ID).SetIxID(il.IXID).
			SetIxfIxpMemberListURLVisible(il.IXFIXPMemberListURLVisible).
			SetNillableIxfIxpMemberListURL(il.IXFIXPMemberListURL).
			SetStatus(il.Status).SetCreated(il.Created).SetUpdated(il.Updated).
			SaveX(ctx)

		srv := newTierTestServer(t, c, privctx.TierUsers)
		row := listDepthRow(t, srv, "/api/ixlan/10")
		got, present := row["ixf_ixp_member_list_url"]
		if !present || got != nil {
			t.Errorf("Users row after an anonymous sync: url = %#v (present=%v), want null (divergence canary)", got, present)
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

	t.Run("list_depth1_sets_as_id_lists", func(t *testing.T) {
		t.Parallel()
		// upstream: pdb_api_test.py:4122-4129 at 2.83.0 (a list at depth
		// 1 renders each _set as an id list) and serializers.py:1286-1290
		// (list_exclude: no forward FK object on a list row). The
		// facility-link sets are in facility order and ixlan.net_set
		// keeps the netixlan order with duplicates, as on a detail
		// response (serializers.py:1140-1148, :1678-1681).
		srv := newTestServer(t, seedListDepthShapes(t, t0))
		org := listDepthRow(t, srv, "/api/org?id=1&depth=1")
		for set, want := range map[string][]int{
			"net_set":     {1, 2, 3},
			"fac_set":     {7, 64},
			"ix_set":      {1},
			"carrier_set": {1},
			"campus_set":  {1}, // campus 2 is pending
		} {
			if got := setIDs(t, org, set); !slices.Equal(got, want) {
				t.Errorf("org %s = %v, want %v", set, got, want)
			}
		}
		for _, tc := range []struct {
			path, fk, set string
			want          []int
		}{
			{"/api/net?id=1&depth=1", "org", "netfac_set", []int{300, 200}},
			{"/api/net?id=1&depth=1", "org", "netixlan_set", []int{11, 14}},
			{"/api/ix?id=1&depth=1", "org", "fac_set", []int{7, 64}},
			{"/api/ix?id=1&depth=1", "org", "ixlan_set", []int{1}},
			{"/api/carrier?id=1&depth=1", "org", "carrierfac_set", []int{300, 200}},
			{"/api/campus?id=1&depth=1", "org", "fac_set", []int{64}},
			{"/api/ixlan?id=1&depth=1", "ix", "net_set", []int{2, 1, 2, 4, 1}},
		} {
			row := listDepthRow(t, srv, tc.path)
			if _, ok := row[tc.fk]; ok {
				t.Errorf("%s: row has the %q key", tc.path, tc.fk)
			}
			if got := setIDs(t, row, tc.set); !slices.Equal(got, tc.want) {
				t.Errorf("%s %s = %v, want %v", tc.path, tc.set, got, tc.want)
			}
		}
	})

	t.Run("list_depth2_sets_as_objects", func(t *testing.T) {
		t.Parallel()
		// upstream: pdb_api_test.py:4131-4137 at 2.83.0 (a list at depth
		// 2 renders each _set as objects), serializers.py:1240-1309 (a
		// nested object drops the FK back to its parent and has no _set
		// of its own at depth 2), :1140-1148 (netixlan_set holds the
		// not-operational rows) and :1678-1681 (a through set filters the
		// join row only).
		srv := newTestServer(t, seedListDepthShapes(t, t0))
		net := setObjects(t, listDepthRow(t, srv, "/api/org?id=1&depth=2"), "net_set")[0]
		if net["id"] != float64(1) || net["asn"] == nil {
			t.Errorf("org net_set[0] = %v, want the full net 1", net)
		}
		for _, k := range []string{"org_id", "poc_set", "netfac_set", "netixlan_set"} {
			if _, ok := net[k]; ok {
				t.Errorf("org net_set[0] has the %q key", k)
			}
		}
		statuses := map[any]bool{}
		for _, nix := range setObjects(t, listDepthRow(t, srv, "/api/net?id=1&depth=2"), "netixlan_set") {
			if _, ok := nix["net_id"]; ok {
				t.Errorf("net netixlan_set element has net_id: %v", nix)
			}
			statuses[nix["status"]] = true
		}
		if !statuses["not-operational"] {
			t.Errorf("net netixlan_set statuses = %v, want a not-operational row", statuses)
		}
		fac := setObjects(t, listDepthRow(t, srv, "/api/campus?id=1&depth=2"), "fac_set")[0]
		if _, ok := fac["org_id"]; ok || fac["campus_id"] != float64(1) {
			t.Errorf("campus fac_set[0] = %v, want campus_id 1 and no org_id", fac)
		}
		cf := setObjects(t, listDepthRow(t, srv, "/api/carrier?id=1&depth=2"), "carrierfac_set")[0]
		if cf["carrier_id"] != float64(1) {
			t.Errorf("carrier carrierfac_set[0] = %v, want carrier_id 1", cf)
		}
		found := false
		for _, n := range setObjects(t, listDepthRow(t, srv, "/api/ixlan?id=1&depth=2"), "net_set") {
			if n["id"] == float64(4) && n["status"] == "deleted" {
				found = true
			}
		}
		if !found {
			t.Error("ixlan net_set has no deleted net 4 (a live netixlan points to it)")
		}
	})

	t.Run("list_depth_matches_detail_without_fk_objects", func(t *testing.T) {
		t.Parallel()
		// upstream: serializers.py:1286-1290 at 2.83.0: a list row is
		// the detail row without the forward FK objects. Locks the list
		// set loaders to the detail getters.
		srv := newTestServer(t, seedListDepthShapes(t, t0))
		for _, typ := range []string{"org", "net", "ix", "ixlan", "carrier", "campus"} {
			for _, d := range []string{"1", "2"} {
				list := listDepthRow(t, srv, "/api/"+typ+"?id=1&depth="+d)
				detail := listDepthRow(t, srv, "/api/"+typ+"/1?depth="+d)
				for _, k := range []string{"org", "campus", "ix"} {
					delete(detail, k)
				}
				if !reflect.DeepEqual(list, detail) {
					t.Errorf("%s depth=%s: list row differs from the detail row:\n  list:   %v\n  detail: %v", typ, d, list, detail)
				}
			}
		}
	})

	t.Run("list_depth_since_deleted_parent_expands_live_children", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:719-750 at 2.83.0 (a ?since list holds the
		// deleted rows) and serializers.py:1140-1148 (the nested sets
		// keep their own live filter): a deleted org lists its live net.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		c.Organization.Create().SetID(5).SetName("Gone Org").SetNameFold(unifold.Fold("Gone Org")).
			SetStatus("deleted").SetCreated(t0).SetUpdated(t0.Add(time.Hour)).SaveX(ctx)
		mustNet(ctx, t, c, 50, "Live Net", 64550, 5, t0)
		srv := newTestServer(t, c)
		path := fmt.Sprintf("/api/org?since=%d&depth=1", t0.Add(30*time.Minute).Unix())
		row := listDepthRow(t, srv, path)
		if row["id"] != float64(5) || row["status"] != "deleted" {
			t.Fatalf("%s: row = %v, want the deleted org 5", path, row)
		}
		if got := setIDs(t, row, "net_set"); !slices.Equal(got, []int{50}) {
			t.Errorf("%s: net_set = %v, want [50]", path, got)
		}
	})

	t.Run("list_depth_fields_selects_sets", func(t *testing.T) {
		t.Parallel()
		// upstream: serializers.py:942-950 at 2.83.0: ?fields= drops the
		// _set fields that it does not name. The mirror keeps id and a
		// plain field whose name ends in _set (registered row
		// DIVERGENCE_fields_keeps_id_and_detail_sets).
		srv := newTestServer(t, seedListDepthShapes(t, t0))
		for _, tc := range []struct {
			path string
			want []string
		}{
			{"/api/org?id=1&depth=2&fields=name", []string{"id", "name"}},
			{"/api/org?id=1&depth=2&fields=name,net_set", []string{"id", "name", "net_set"}},
			{"/api/net?id=1&depth=2&fields=name", []string{"id", "irr_as_set", "name"}},
		} {
			if got := slices.Sorted(maps.Keys(listDepthRow(t, srv, tc.path))); !slices.Equal(got, tc.want) {
				t.Errorf("%s: keys = %v, want %v", tc.path, got, tc.want)
			}
		}
	})

	t.Run("DIVERGENCE_list_depth3_renders_depth2_shape", func(t *testing.T) {
		t.Parallel()
		// DIVERGENCE: upstream pdb_api_test.py:4140-4152 at 2.83.0: a
		// list at depth 3 gives each set element its own _set fields as
		// id lists (serializers.py:1240-1309). The mirror renders the
		// depth-2 shape. See docs/API.md § Known Divergences.
		// This test ASSERTS the divergence (it is NOT a parity match).
		srv := newTestServer(t, seedListDepthShapes(t, t0))
		net := setObjects(t, listDepthRow(t, srv, "/api/org?id=1&depth=3"), "net_set")[0]
		if _, ok := net["netfac_set"]; ok {
			t.Errorf("depth=3 org net_set[0] has netfac_set (divergence canary): %v", net)
		}
		_, two := httpGet(t, srv, "/api/org?id=1&depth=2")
		_, three := httpGet(t, srv, "/api/org?id=1&depth=3")
		if !bytes.Equal(two, three) {
			t.Errorf("depth=3 body differs from depth=2:\n  2: %s\n  3: %s", two, three)
		}
	})

	t.Run("DIVERGENCE_list_depth_ctf_ignored", func(t *testing.T) {
		t.Parallel()
		// DIVERGENCE: upstream rest.py:654-655 at 2.83.0: with _ctf, the
		// last date filter with an operator is kept in request._ctf, and
		// serializers.py:998-1002 applies it to every nested set, so
		// upstream lists only net 1. The mirror ignores _ctf (an unknown
		// key). See docs/API.md § Known Divergences.
		// This test ASSERTS the divergence (it is NOT a parity match).
		c := testutil.SetupClient(t)
		ctx := t.Context()
		day := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
		mustOrg(ctx, t, c, 1, "CTF Org", day)
		mustNet(ctx, t, c, 1, "CTF Old", 64501, 1, day)
		mustNet(ctx, t, c, 2, "CTF New", 64502, 1, day.AddDate(0, 0, 5))
		srv := newTestServer(t, c)
		row := listDepthRow(t, srv, "/api/org?id=1&depth=1&updated__lte=2026-09-22&_ctf=1")
		if got := setIDs(t, row, "net_set"); !slices.Equal(got, []int{1, 2}) {
			t.Errorf("net_set = %v, want [1 2] (divergence canary)", got)
		}
	})

	t.Run("DIVERGENCE_fields_keeps_id_and_detail_sets", func(t *testing.T) {
		t.Parallel()
		// DIVERGENCE: upstream serializers.py:942-950 at 2.83.0 drops
		// every field that ?fields= does not name, id and the _set
		// fields too, and the API cache path drops every key not named
		// (api_cache.py:170-177). The mirror keeps id, every key that
		// ends in _set and every nested object on a detail, and a plain
		// field that ends in _set (net irr_as_set) on a list. See
		// docs/API.md § Known Divergences.
		// For an ixlan, upstream IXLanSerializer.to_representation adds
		// ixf_ixp_member_list_url_visible back whenever the URL is in the
		// output (serializers.py:4319-4337), and handle_ixlan removes it
		// again only when the URL and _visible are the only keys left
		// (permissions.py:355-372). The mirror never adds _visible back.
		// This test ASSERTS the divergence (it is NOT a parity match).
		c := seedListDepthShapes(t, t0)
		ctx := t.Context()
		c.IxLan.Create().SetID(20).SetIxID(1).
			SetIxfIxpMemberListURLVisible("Public").
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		c.IxLan.Create().SetID(21).SetIxID(1).
			SetIxfIxpMemberListURLVisible("Users").
			SetIxfIxpMemberListURL("https://ixf.example.test/21.json").
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		srv := newTestServer(t, c)
		for _, tc := range []struct {
			path string
			want []string
		}{
			// Upstream: [name].
			{"/api/org?id=1&fields=name", []string{"id", "name"}},
			{"/api/net?id=1&fields=name", []string{"id", "irr_as_set", "name"}},
			// Upstream: [name]. Detail defaults to depth 2.
			{"/api/net/1?fields=name", []string{"id", "irr_as_set", "name", "netfac_set", "netixlan_set", "org", "poc_set"}},
			// Upstream: [ixf_ixp_member_list_url].
			{"/api/ixlan?id=20&fields=ixf_ixp_member_list_url", []string{"id", "ixf_ixp_member_list_url"}},
			// Upstream: [id, ixf_ixp_member_list_url, ixf_ixp_member_list_url_visible].
			{"/api/ixlan?id=20&fields=id,ixf_ixp_member_list_url", []string{"id", "ixf_ixp_member_list_url"}},
			// Anonymous caller, Users row. Upstream:
			// [ixf_ixp_member_list_url_visible].
			{"/api/ixlan?id=21&fields=ixf_ixp_member_list_url", []string{"id"}},
		} {
			if got := slices.Sorted(maps.Keys(listDepthRow(t, srv, tc.path))); !slices.Equal(got, tc.want) {
				t.Errorf("%s: keys = %v, want %v (divergence canary)", tc.path, got, tc.want)
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

	t.Run("as_set_list_maps_ok_networks_with_a_set", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:1405-1412, models.py:5699-5705,
		// renderers.py:123-130 at 2.83.0. The list maps the ASN of each
		// ok network with a set that is not empty. The mirror sorts by
		// asn; upstream sends database order, and JSON object order has
		// no meaning.
		c := testutil.SetupClient(t)
		seedASSetNets(t, c, t0)
		srv := newTestServer(t, c)

		status, body := httpGet(t, srv, "/api/as_set")
		if status != http.StatusOK {
			t.Fatalf("GET /api/as_set: status = %d; body=%s", status, body)
		}
		want := map[string]string{"9": "RIPE::AS-NINE", "64500": "AS-ONE", "64504": "AS-ÜNI"}
		if got := decodeASSetMap(t, body); !maps.Equal(got, want) {
			t.Errorf("GET /api/as_set: data = %v, want %v", got, want)
		}
		assertTopLevelKeys(t, body, "data", "meta")
		assertMetaKeys(t, body)
		// Lock the mirror order: asn ascending, not string order.
		i9 := bytes.Index(body, []byte(`"9":`))
		i64500 := bytes.Index(body, []byte(`"64500":`))
		i64504 := bytes.Index(body, []byte(`"64504":`))
		if i9 < 0 || i64500 < i9 || i64504 < i64500 {
			t.Errorf("GET /api/as_set: key order is not 9, 64500, 64504: %s", body)
		}
	})

	t.Run("as_set_list_empty_is_empty_data_array", func(t *testing.T) {
		t.Parallel()
		// upstream: renderers.py:131-132 at 2.83.0. An empty dict is
		// falsy, so the renderer writes "data": [].
		c := testutil.SetupClient(t)
		seedASSetNet(t, c, 2, 64501, "ok", "", t0)
		seedASSetNet(t, c, 3, 64502, "deleted", "AS-GONE", t0)
		srv := newTestServer(t, c)

		status, body := httpGet(t, srv, "/api/as_set")
		if status != http.StatusOK {
			t.Fatalf("GET /api/as_set: status = %d; body=%s", status, body)
		}
		if got := decodeDataArray(t, body); len(got) != 0 {
			t.Errorf("GET /api/as_set: data = %v, want []", got)
		}
	})

	t.Run("as_set_list_ignores_query_parameters", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:1411-1412 at 2.83.0. list() does not call
		// filter_queryset or the pagination, so it reads no parameter.
		c := testutil.SetupClient(t)
		seedASSetNets(t, c, t0)
		srv := newTestServer(t, c)

		_, bare := httpGet(t, srv, "/api/as_set")
		path := "/api/as_set?limit=1&skip=5&depth=2&fields=x&since=1&asn=64500&status=deleted&limit=abc&skip=-1&q=zzz"
		status, body := httpGet(t, srv, path)
		if status != http.StatusOK {
			t.Fatalf("GET %s: status = %d; body=%s", path, status, body)
		}
		if !bytes.Equal(body, bare) {
			t.Errorf("GET %s: body = %s, want the bare list %s", path, body, bare)
		}
	})

	t.Run("as_set_detail_returns_one_pair", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:1414-1423 at 2.83.0.
		c := testutil.SetupClient(t)
		seedASSetNets(t, c, t0)
		srv := newTestServer(t, c)

		for path, want := range map[string]map[string]string{
			"/api/as_set/64500": {"64500": "AS-ONE"},
			"/api/as_set/64501": {"64501": ""},
		} {
			status, body := httpGet(t, srv, path)
			if status != http.StatusOK {
				t.Errorf("GET %s: status = %d; body=%s", path, status, body)
				continue
			}
			if got := decodeASSetMap(t, body); !maps.Equal(got, want) {
				t.Errorf("GET %s: data = %v, want %v", path, got, want)
			}
		}
	})

	t.Run("as_set_detail_parses_like_python_int", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:1416 at 2.83.0 (int(asn)).
		// synthesised: CPython int() grammar.
		c := testutil.SetupClient(t)
		seedASSetNets(t, c, t0)
		srv := newTestServer(t, c)

		want := map[string]string{"64500": "AS-ONE"}
		for _, v := range []string{
			"+64500",
			"064500",
			"64_500",
			"%2064500%20",
			"%0964500",
			url.PathEscape("٦٤٥٠٠"), // Arabic-Indic digits
			url.PathEscape("６４５００"), // fullwidth digits
		} {
			path := "/api/as_set/" + v
			status, body := httpGet(t, srv, path)
			if status != http.StatusOK {
				t.Errorf("GET %s: status = %d, want 200; body=%s", path, status, body)
				continue
			}
			if got := decodeASSetMap(t, body); !maps.Equal(got, want) {
				t.Errorf("GET %s: data = %v, want %v", path, got, want)
			}
		}
	})

	t.Run("as_set_detail_invalid_asn_400", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:1417-1420 at 2.83.0. int() raises
		// ValueError, and the renderer moves detail to meta.error
		// (renderers.py:134-140).
		c := testutil.SetupClient(t)
		seedASSetNets(t, c, t0)
		srv := newTestServer(t, c)

		for _, v := range []string{
			"abc", "AS64500", "64__500", "_64500", "64500_", "%20", "-%2064500",
			strings.Repeat("1", 4301),
		} {
			path := "/api/as_set/" + v
			status, body := httpGet(t, srv, path)
			if status != http.StatusBadRequest {
				t.Errorf("GET %s: status = %d, want 400; body=%s", path, status, body)
				continue
			}
			assertTopLevelKeys(t, body, "meta")
			if got := mustDecodeMetaError(t, body).Error; got != "Invalid ASN" {
				t.Errorf("GET %s: meta.error = %q, want %q", path, got, "Invalid ASN")
			}
		}
	})

	t.Run("as_set_detail_missing_404_empty_body", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:1421-1422, renderers.py:106-107 at 2.83.0,
		// drf response.py:82-83. Response(status=404) has no data, so
		// the body is empty and DRF drops Content-Type. A negative or
		// out-of-range ASN finds no row (Django IntegerFieldExact raises
		// EmptyResultSet).
		c := testutil.SetupClient(t)
		seedASSetNets(t, c, t0)
		srv := newTestServer(t, c)

		for _, v := range []string{"1", "-64500", "0", "-0", "99999999999999999999999"} {
			path := "/api/as_set/" + v
			status, hdr, body := httpDo(t, srv, http.MethodGet, path, nil)
			if status != http.StatusNotFound {
				t.Errorf("GET %s: status = %d, want 404; body=%s", path, status, body)
				continue
			}
			if len(body) != 0 {
				t.Errorf("GET %s: body = %q, want empty", path, body)
			}
			if ct := hdr.Get("Content-Type"); ct != "" {
				t.Errorf("GET %s: Content-Type = %q, want none", path, ct)
			}
		}
	})
}

// seedASSetNet creates one network for the as_set sub-tests. A network
// needs no org.
func seedASSetNet(t *testing.T, c *ent.Client, id, asn int, status, irrAsSet string, ts time.Time) {
	t.Helper()
	if _, err := c.Network.Create().
		SetID(id).SetName("ASSetNet").SetNameFold(unifold.Fold("ASSetNet")).
		SetAsn(asn).SetIrrAsSet(irrAsSet).SetStatus(status).
		SetCreated(ts).SetUpdated(ts).
		Save(t.Context()); err != nil {
		t.Fatalf("seed net id=%d: %v", id, err)
	}
}

// seedASSetNets seeds the common as_set rows: three listable networks
// (asn 9, 64500, 64504), an ok network with an empty set (64501), a
// deleted one (64502) and a pending one (64503).
func seedASSetNets(t *testing.T, c *ent.Client, ts time.Time) {
	t.Helper()
	for _, n := range []struct {
		id, asn          int
		status, irrAsSet string
	}{
		{1, 64500, "ok", "AS-ONE"},
		{2, 64501, "ok", ""},
		{3, 64502, "deleted", "AS-GONE"},
		{4, 64503, "pending", "AS-PEND"},
		{5, 9, "ok", "RIPE::AS-NINE"},
		{6, 64504, "ok", "AS-ÜNI"},
	} {
		seedASSetNet(t, c, n.id, n.asn, n.status, n.irrAsSet, ts)
	}
}

// decodeASSetMap decodes the one object in the data array of an as_set
// response.
func decodeASSetMap(t *testing.T, body []byte) map[string]string {
	t.Helper()
	var env struct {
		Data []map[string]string `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode as_set body: %v; body=%s", err, body)
	}
	if len(env.Data) != 1 {
		t.Fatalf("as_set data has %d objects, want 1; body=%s", len(env.Data), body)
	}
	return env.Data[0]
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

// seedListDepthShapes seeds one row of each type with reverse sets
// (id 1) and children that exercise the set filters and orders:
//   - org 1: nets 3, 1, 2 (inserted out of order) and a deleted net 4,
//     facs 7 and 64, ix 1, carrier 1, campus 1 and a pending campus 2.
//   - fac 64 is in campus 1.
//   - netfac, ixfac and carrierfac 300 (fac 7) and 200 (fac 64), on net
//     1, ix 1 and carrier 1: link ids run opposite to the fac ids.
//   - ixlan 1 on ix 1 with netixlans 10 (net 2), 11 (net 1), 12 (net
//     2), 13 (deleted net 4) and 14 (net 1, not-operational).
//   - net 1 has irr_as_set "AS-SHAPE".
func seedListDepthShapes(t *testing.T, t0 time.Time) *ent.Client {
	t.Helper()
	c := testutil.SetupClient(t)
	ctx := t.Context()
	mustOrg(ctx, t, c, 1, "Shape Org", t0)
	for _, id := range []int{3, 1, 2} {
		mustNet(ctx, t, c, id, fmt.Sprintf("Shape Net %d", id), 64500+id, 1, t0)
	}
	c.Network.UpdateOneID(1).SetIrrAsSet("AS-SHAPE").ExecX(ctx)
	c.Network.Create().SetID(4).SetName("Shape Gone").SetNameFold(unifold.Fold("Shape Gone")).
		SetAsn(64504).SetOrgID(1).SetStatus("deleted").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
	mustCampus(ctx, t, c, 1, "Shape Campus", 1, t0)
	c.Campus.Create().SetID(2).SetName("Shape Pending").SetNameFold(unifold.Fold("Shape Pending")).
		SetOrgID(1).SetStatus("pending").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
	mustFac(ctx, t, c, 7, "Shape Fac 7", 1, t0)
	mustFac(ctx, t, c, 64, "Shape Fac 64", 1, t0)
	c.Facility.UpdateOneID(64).SetCampusID(1).ExecX(ctx)
	mustIX(ctx, t, c, 1, "Shape IX", 1, t0)
	mustIxLan(ctx, t, c, 1, "Shape LAN", 1, t0)
	c.Carrier.Create().SetID(1).SetName("Shape Carrier").SetOrgID(1).
		SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
	for _, l := range []struct{ id, fac int }{{300, 7}, {200, 64}} {
		c.NetworkFacility.Create().SetID(l.id).SetNetID(1).SetFacID(l.fac).SetLocalAsn(64501).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		c.IxFacility.Create().SetID(l.id).SetIxID(1).SetFacID(l.fac).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		c.CarrierFacility.Create().SetID(l.id).SetCarrierID(1).SetFacID(l.fac).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
	}
	for _, n := range []struct {
		id, net int
		status  string
	}{{10, 2, "ok"}, {11, 1, "ok"}, {12, 2, "ok"}, {13, 4, "ok"}, {14, 1, "not-operational"}} {
		c.NetworkIxLan.Create().
			SetID(n.id).SetNetID(n.net).SetIxlanID(1).SetIxID(1).SetName("Shape IX").
			SetAsn(64500 + n.net).SetSpeed(1000).SetOperational(n.status == "ok").
			SetStatus(n.status).SetCreated(t0).SetUpdated(t0).SaveX(ctx)
	}
	return c
}

// listDepthRow GETs a list or detail that must return 200 with one row
// and returns the row.
func listDepthRow(t *testing.T, srv *httptest.Server, path string) map[string]any {
	t.Helper()
	status, body := httpGet(t, srv, path)
	if status != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200; body=%s", path, status, headBody(body, 300))
	}
	rows := decodeDataArray(t, body)
	if len(rows) != 1 {
		t.Fatalf("GET %s: %d rows, want 1", path, len(rows))
	}
	return rows[0]
}

// setIDs returns the ids of a depth-1 _set field (an id list).
func setIDs(t *testing.T, row map[string]any, key string) []int {
	t.Helper()
	raw, ok := row[key].([]any)
	if !ok {
		t.Fatalf("%s = %T(%v), want an array", key, row[key], row[key])
	}
	ids := make([]int, 0, len(raw))
	for _, v := range raw {
		f, ok := v.(float64)
		if !ok {
			t.Fatalf("%s element = %T(%v), want an id", key, v, v)
		}
		ids = append(ids, int(f))
	}
	return ids
}

// setObjects returns the elements of a depth-2 _set field (objects).
func setObjects(t *testing.T, row map[string]any, key string) []map[string]any {
	t.Helper()
	raw, ok := row[key].([]any)
	if !ok || len(raw) == 0 {
		t.Fatalf("%s = %T(%v), want a non-empty array", key, row[key], row[key])
	}
	out := make([]map[string]any, 0, len(raw))
	for _, v := range raw {
		m, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("%s element = %T(%v), want an object", key, v, v)
		}
		out = append(out, m)
	}
	return out
}
