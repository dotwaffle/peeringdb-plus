package pdbcompat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestDepth_DeletedRowsExcludedFromSets locks the 2026-06-10 audit fix:
// the depth>=2 eager-loaded _set collections must exclude tombstoned
// (status='deleted') rows, matching upstream's nested prefetch filter,
// which admits only the child's live statuses (2.83.0
// serializers.py:1140-1148). Before the fix the DEFAULT detail shape
// (depth=2) re-published deleted rows — including deleted POC contact
// data — indefinitely. TestDepth_PendingChildrenExcludedFromSets covers
// pending children.
func TestDepth_DeletedRowsExcludedFromSets(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := t.Context()
	now := time.Now().Truncate(time.Second).UTC()

	org := client.Organization.Create().
		SetName("Tombstone Org").SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	// Sibling pairs: one live, one tombstoned, per reverse set.
	netOK := client.Network.Create().
		SetName("Live Net").SetAsn(65001).SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	netDel := client.Network.Create().
		SetName("Deleted Net").SetAsn(65002).SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("deleted").SaveX(ctx)

	facOK := client.Facility.Create().
		SetName("Live Fac").SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	facDel := client.Facility.Create().
		SetName("Deleted Fac").SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("deleted").SaveX(ctx)

	ixOK := client.InternetExchange.Create().
		SetName("Live IX").SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	ixDel := client.InternetExchange.Create().
		SetName("Deleted IX").SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("deleted").SaveX(ctx)

	carOK := client.Carrier.Create().
		SetName("Live Carrier").SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	carDel := client.Carrier.Create().
		SetName("Deleted Carrier").SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("deleted").SaveX(ctx)

	campOK := client.Campus.Create().
		SetName("Live Campus").SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	campDel := client.Campus.Create().
		SetName("Deleted Campus").SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("deleted").SaveX(ctx)

	pocOK := client.Poc.Create().
		SetName("Live POC").SetRole("Abuse").SetVisible("Public").SetNetwork(netOK).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	pocDel := client.Poc.Create().
		SetName("Deleted POC").SetRole("Abuse").SetVisible("Public").SetNetwork(netOK).
		SetCreated(now).SetUpdated(now).SetStatus("deleted").SaveX(ctx)

	netfacOK := client.NetworkFacility.Create().
		SetNetwork(netOK).SetFacility(facOK).SetLocalAsn(65001).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	netfacDel := client.NetworkFacility.Create().
		SetNetwork(netOK).SetFacility(facDel).SetLocalAsn(65001).
		SetCreated(now).SetUpdated(now).SetStatus("deleted").SaveX(ctx)

	ixlanOK := client.IxLan.Create().
		SetInternetExchange(ixOK).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	ixlanDel := client.IxLan.Create().
		SetInternetExchange(ixOK).
		SetCreated(now).SetUpdated(now).SetStatus("deleted").SaveX(ctx)

	nilOK := client.NetworkIxLan.Create().
		SetNetwork(netOK).SetIxLan(ixlanOK).SetAsn(65001).SetSpeed(1000).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	nilDel := client.NetworkIxLan.Create().
		SetNetwork(netOK).SetIxLan(ixlanDel).SetAsn(65001).SetSpeed(1000).
		SetCreated(now).SetUpdated(now).SetStatus("deleted").SaveX(ctx)

	ixpfxOK := client.IxPrefix.Create().
		SetIxLan(ixlanOK).SetPrefix("10.0.0.0/24").SetProtocol("IPv4").
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	ixpfxDel := client.IxPrefix.Create().
		SetIxLan(ixlanOK).SetPrefix("10.0.1.0/24").SetProtocol("IPv4").
		SetCreated(now).SetUpdated(now).SetStatus("deleted").SaveX(ctx)

	// A tombstoned netixlan join row on the live ixlan: its (live) network
	// must not surface in the ixlan's net_set.
	netOK2 := client.Network.Create().
		SetName("Live Net 2").SetAsn(65003).SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	client.NetworkIxLan.Create().
		SetNetwork(netOK2).SetIxLan(ixlanOK).SetAsn(65003).SetSpeed(1000).
		SetCreated(now).SetUpdated(now).SetStatus("deleted").SaveX(ctx)

	ixfacOK := client.IxFacility.Create().
		SetInternetExchange(ixOK).SetFacility(facOK).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	ixfacDel := client.IxFacility.Create().
		SetInternetExchange(ixOK).SetFacility(facDel).
		SetCreated(now).SetUpdated(now).SetStatus("deleted").SaveX(ctx)
	_, _ = ixfacOK, ixfacDel

	cfOK := client.CarrierFacility.Create().
		SetCarrier(carOK).SetFacility(facOK).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	cfDel := client.CarrierFacility.Create().
		SetCarrier(carOK).SetFacility(facDel).
		SetCreated(now).SetUpdated(now).SetStatus("deleted").SaveX(ctx)

	campFacOK := client.Facility.Create().
		SetName("Live Campus Fac").SetOrganization(org).SetCampus(campOK).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	campFacDel := client.Facility.Create().
		SetName("Deleted Campus Fac").SetOrganization(org).SetCampus(campOK).
		SetCreated(now).SetUpdated(now).SetStatus("deleted").SaveX(ctx)

	h := NewHandler(client, 0)
	mux := http.NewServeMux()
	h.Register(mux)

	// fetchSets GETs the default-depth (2) detail and returns the named
	// _set arrays' embedded ids.
	fetchSetIDs := func(t *testing.T, path, setKey string) map[int]bool {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: %d: %s", path, rec.Code, rec.Body.String())
		}
		var env struct {
			Data []map[string]any `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		if len(env.Data) != 1 {
			t.Fatalf("GET %s: %d rows, want 1", path, len(env.Data))
		}
		raw, ok := env.Data[0][setKey].([]any)
		if !ok {
			t.Fatalf("GET %s: %s missing or not an array: %T", path, setKey, env.Data[0][setKey])
		}
		ids := make(map[int]bool, len(raw))
		for _, el := range raw {
			obj, ok := el.(map[string]any)
			if !ok {
				t.Fatalf("GET %s: %s element is %T, want object", path, setKey, el)
			}
			if id, ok := obj["id"].(float64); ok {
				ids[int(id)] = true
			}
		}
		return ids
	}

	cases := []struct {
		path, setKey   string
		wantID, dropID int
	}{
		{fmt.Sprintf("/api/org/%d", org.ID), "net_set", netOK.ID, netDel.ID},
		{fmt.Sprintf("/api/org/%d", org.ID), "ix_set", ixOK.ID, ixDel.ID},
		{fmt.Sprintf("/api/org/%d", org.ID), "carrier_set", carOK.ID, carDel.ID},
		{fmt.Sprintf("/api/org/%d", org.ID), "campus_set", campOK.ID, campDel.ID},
		{fmt.Sprintf("/api/org/%d", org.ID), "fac_set", facOK.ID, facDel.ID},
		{fmt.Sprintf("/api/net/%d", netOK.ID), "poc_set", pocOK.ID, pocDel.ID},
		{fmt.Sprintf("/api/net/%d", netOK.ID), "netfac_set", netfacOK.ID, netfacDel.ID},
		{fmt.Sprintf("/api/net/%d", netOK.ID), "netixlan_set", nilOK.ID, nilDel.ID},
		{fmt.Sprintf("/api/ix/%d", ixOK.ID), "ixlan_set", ixlanOK.ID, ixlanDel.ID},
		{fmt.Sprintf("/api/ix/%d", ixOK.ID), "fac_set", facOK.ID, facDel.ID},
		{fmt.Sprintf("/api/ixlan/%d", ixlanOK.ID), "ixpfx_set", ixpfxOK.ID, ixpfxDel.ID},
		{fmt.Sprintf("/api/ixlan/%d", ixlanOK.ID), "net_set", netOK.ID, netOK2.ID},
		{fmt.Sprintf("/api/carrier/%d", carOK.ID), "carrierfac_set", cfOK.ID, cfDel.ID},
		{fmt.Sprintf("/api/campus/%d", campOK.ID), "fac_set", campFacOK.ID, campFacDel.ID},
	}
	for _, tc := range cases {
		t.Run(tc.path+"/"+tc.setKey, func(t *testing.T) {
			t.Parallel()
			ids := fetchSetIDs(t, tc.path, tc.setKey)
			if !ids[tc.wantID] {
				t.Errorf("%s %s: live id %d missing (got %v)", tc.path, tc.setKey, tc.wantID, ids)
			}
			if ids[tc.dropID] {
				t.Errorf("%s %s: tombstoned id %d leaked into the set (got %v)", tc.path, tc.setKey, tc.dropID, ids)
			}
		})
	}
	_ = ent.Client{}
}

// depthSetIDs GETs a detail path and returns the ids in the named _set,
// sorted: the bare numbers of a depth=1 ID list, or the "id" of each
// object at depth=2.
func depthSetIDs(t *testing.T, h http.Handler, path, key string) []int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: %d: %s", path, rec.Code, rec.Body.String())
	}
	var env struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil || len(env.Data) != 1 {
		t.Fatalf("GET %s: decode: %v (rows=%d)", path, err, len(env.Data))
	}
	raw, ok := env.Data[0][key].([]any)
	if !ok {
		t.Fatalf("GET %s: %s is %T, want array", path, key, env.Data[0][key])
	}
	ids := make([]int, 0, len(raw))
	for _, el := range raw {
		switch v := el.(type) {
		case float64:
			ids = append(ids, int(v))
		case map[string]any:
			id, ok := v["id"].(float64)
			if !ok {
				t.Fatalf("GET %s: %s element has no numeric id: %v", path, key, v)
			}
			ids = append(ids, int(id))
		default:
			t.Fatalf("GET %s: %s element is %T", path, key, el)
		}
	}
	slices.Sort(ids)
	return ids
}

// TestDepth_PendingChildrenExcludedFromSets locks the live-only rule for
// nested sets. Upstream's nested prefetch admits only the child's live
// statuses (2.83.0 serializers.py:1140-1148; 2.82.0 filtered status="ok"
// at :935), and detail responses use the same prefetch (rest.py:774-777).
// So a pending child is left out of every _set: from the ID list at
// depth=1 and from the objects at depth=2. A through-relation set drops
// the pending join row. In practice only campus rows reach our DB as
// pending, because the since matrix admits pending for campus alone, but
// every set follows the same rule.
func TestDepth_PendingChildrenExcludedFromSets(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := t.Context()
	now := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)

	org := client.Organization.Create().
		SetName("Pending Org").SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	// Sibling pairs: one live, one pending, per reverse set.
	netOK := client.Network.Create().
		SetName("Live Net").SetAsn(65001).SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	netPend := client.Network.Create().
		SetName("Pending Net").SetAsn(65002).SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("pending").SaveX(ctx)
	// netOther is live, but it reaches the ixlan only through a pending
	// netixlan.
	netOther := client.Network.Create().
		SetName("Other Net").SetAsn(65003).SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)

	facOK := client.Facility.Create().
		SetName("Live Fac").SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	facPend := client.Facility.Create().
		SetName("Pending Fac").SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("pending").SaveX(ctx)
	// facOther is live, but it reaches the ix only through a pending ixfac.
	facOther := client.Facility.Create().
		SetName("Other Fac").SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)

	ixOK := client.InternetExchange.Create().
		SetName("Live IX").SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	ixPend := client.InternetExchange.Create().
		SetName("Pending IX").SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("pending").SaveX(ctx)

	carOK := client.Carrier.Create().
		SetName("Live Carrier").SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	carPend := client.Carrier.Create().
		SetName("Pending Carrier").SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("pending").SaveX(ctx)

	campOK := client.Campus.Create().
		SetName("Live Campus").SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	campPend := client.Campus.Create().
		SetName("Pending Campus").SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("pending").SaveX(ctx)

	pocOK := client.Poc.Create().
		SetName("Live POC").SetRole("Abuse").SetVisible("Public").SetNetwork(netOK).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	pocPend := client.Poc.Create().
		SetName("Pending POC").SetRole("Abuse").SetVisible("Public").SetNetwork(netOK).
		SetCreated(now).SetUpdated(now).SetStatus("pending").SaveX(ctx)

	netfacOK := client.NetworkFacility.Create().
		SetNetwork(netOK).SetFacility(facOK).SetLocalAsn(65001).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	netfacPend := client.NetworkFacility.Create().
		SetNetwork(netOK).SetFacility(facOther).SetLocalAsn(65001).
		SetCreated(now).SetUpdated(now).SetStatus("pending").SaveX(ctx)

	ixlanOK := client.IxLan.Create().
		SetInternetExchange(ixOK).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	ixlanPend := client.IxLan.Create().
		SetInternetExchange(ixOK).
		SetCreated(now).SetUpdated(now).SetStatus("pending").SaveX(ctx)

	nilOK := client.NetworkIxLan.Create().
		SetNetwork(netOK).SetIxLan(ixlanOK).SetAsn(65001).SetSpeed(1000).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	nilPend := client.NetworkIxLan.Create().
		SetNetwork(netOK).SetIxLan(ixlanOK).SetAsn(65001).SetSpeed(1000).
		SetCreated(now).SetUpdated(now).SetStatus("pending").SaveX(ctx)
	client.NetworkIxLan.Create().
		SetNetwork(netOther).SetIxLan(ixlanOK).SetAsn(65003).SetSpeed(1000).
		SetCreated(now).SetUpdated(now).SetStatus("pending").SaveX(ctx)

	ixpfxOK := client.IxPrefix.Create().
		SetIxLan(ixlanOK).SetPrefix("10.0.0.0/24").SetProtocol("IPv4").
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	ixpfxPend := client.IxPrefix.Create().
		SetIxLan(ixlanOK).SetPrefix("10.0.1.0/24").SetProtocol("IPv4").
		SetCreated(now).SetUpdated(now).SetStatus("pending").SaveX(ctx)

	client.IxFacility.Create().
		SetInternetExchange(ixOK).SetFacility(facOK).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	client.IxFacility.Create().
		SetInternetExchange(ixOK).SetFacility(facOther).
		SetCreated(now).SetUpdated(now).SetStatus("pending").SaveX(ctx)

	cfOK := client.CarrierFacility.Create().
		SetCarrier(carOK).SetFacility(facOK).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	cfPend := client.CarrierFacility.Create().
		SetCarrier(carOK).SetFacility(facOther).
		SetCreated(now).SetUpdated(now).SetStatus("pending").SaveX(ctx)

	campFacOK := client.Facility.Create().
		SetName("Live Campus Fac").SetOrganization(org).SetCampus(campOK).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	campFacPend := client.Facility.Create().
		SetName("Pending Campus Fac").SetOrganization(org).SetCampus(campOK).
		SetCreated(now).SetUpdated(now).SetStatus("pending").SaveX(ctx)

	mux := newMuxForOrdering(client)

	cases := []struct {
		path, setKey   string
		wantID, dropID int
	}{
		{fmt.Sprintf("/api/org/%d", org.ID), "net_set", netOK.ID, netPend.ID},
		{fmt.Sprintf("/api/org/%d", org.ID), "fac_set", facOK.ID, facPend.ID},
		{fmt.Sprintf("/api/org/%d", org.ID), "ix_set", ixOK.ID, ixPend.ID},
		{fmt.Sprintf("/api/org/%d", org.ID), "carrier_set", carOK.ID, carPend.ID},
		{fmt.Sprintf("/api/org/%d", org.ID), "campus_set", campOK.ID, campPend.ID},
		{fmt.Sprintf("/api/net/%d", netOK.ID), "poc_set", pocOK.ID, pocPend.ID},
		{fmt.Sprintf("/api/net/%d", netOK.ID), "netfac_set", netfacOK.ID, netfacPend.ID},
		{fmt.Sprintf("/api/net/%d", netOK.ID), "netixlan_set", nilOK.ID, nilPend.ID},
		{fmt.Sprintf("/api/ix/%d", ixOK.ID), "ixlan_set", ixlanOK.ID, ixlanPend.ID},
		{fmt.Sprintf("/api/ix/%d", ixOK.ID), "fac_set", facOK.ID, facOther.ID},
		{fmt.Sprintf("/api/ixlan/%d", ixlanOK.ID), "ixpfx_set", ixpfxOK.ID, ixpfxPend.ID},
		{fmt.Sprintf("/api/ixlan/%d", ixlanOK.ID), "net_set", netOK.ID, netOther.ID},
		{fmt.Sprintf("/api/carrier/%d", carOK.ID), "carrierfac_set", cfOK.ID, cfPend.ID},
		{fmt.Sprintf("/api/campus/%d", campOK.ID), "fac_set", campFacOK.ID, campFacPend.ID},
	}
	for _, tc := range cases {
		for _, depth := range []string{"1", "2"} {
			path := tc.path + "?depth=" + depth
			t.Run(path+"/"+tc.setKey, func(t *testing.T) {
				t.Parallel()
				ids := depthSetIDs(t, mux, path, tc.setKey)
				if !slices.Contains(ids, tc.wantID) {
					t.Errorf("%s %s: live id %d missing (got %v)", path, tc.setKey, tc.wantID, ids)
				}
				if slices.Contains(ids, tc.dropID) {
					t.Errorf("%s %s: pending id %d leaked into the set (got %v)", path, tc.setKey, tc.dropID, ids)
				}
			})
		}
	}

	// A PK lookup still admits pending (rest.py:750): the children left
	// out of the sets stay fetchable by ID.
	for _, path := range []string{
		fmt.Sprintf("/api/campus/%d", campPend.ID),
		fmt.Sprintf("/api/net/%d", netPend.ID),
		fmt.Sprintf("/api/netixlan/%d", nilPend.ID),
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s: got %d, want 200 (PK lookup admits pending)", path, rec.Code)
		}
	}
}

// TestDepth_ThroughSetsFilterJoinRowOnly locks the through-relation rule:
// ix.fac_set and ixlan.net_set filter the ixfac or netixlan join row by
// status, and render the facility or network the row points to without a
// status filter of their own (2.83.0 serializers.py:1678-1681, nested()
// extract via getattr(item, getter)). A live join row to a deleted
// facility or network still lists it, at depth=1 and at depth=2.
func TestDepth_ThroughSetsFilterJoinRowOnly(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := t.Context()
	now := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)

	org := client.Organization.Create().
		SetName("Through Org").SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	netGone := client.Network.Create().
		SetName("Deleted Net").SetAsn(65001).SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("deleted").SaveX(ctx)
	facGone := client.Facility.Create().
		SetName("Deleted Fac").SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("deleted").SaveX(ctx)
	ix := client.InternetExchange.Create().
		SetName("Through IX").SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	lan := client.IxLan.Create().
		SetInternetExchange(ix).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	client.NetworkIxLan.Create().
		SetNetwork(netGone).SetIxLan(lan).SetAsn(65001).SetSpeed(1000).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	client.IxFacility.Create().
		SetInternetExchange(ix).SetFacility(facGone).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)

	mux := newMuxForOrdering(client)
	cases := []struct {
		path, key string
		want      []int
	}{
		{fmt.Sprintf("/api/ixlan/%d?depth=1", lan.ID), "net_set", []int{netGone.ID}},
		{fmt.Sprintf("/api/ixlan/%d?depth=2", lan.ID), "net_set", []int{netGone.ID}},
		{fmt.Sprintf("/api/ix/%d?depth=1", ix.ID), "fac_set", []int{facGone.ID}},
		{fmt.Sprintf("/api/ix/%d?depth=2", ix.ID), "fac_set", []int{facGone.ID}},
	}
	for _, tc := range cases {
		t.Run(tc.path+"/"+tc.key, func(t *testing.T) {
			t.Parallel()
			if got := depthSetIDs(t, mux, tc.path, tc.key); !slices.Equal(got, tc.want) {
				t.Errorf("%s %s = %v, want %v", tc.path, tc.key, got, tc.want)
			}
		})
	}
}
