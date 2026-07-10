package pdbcompat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestDepth_DeletedRowsExcludedFromSets locks the 2026-06-10 audit fix:
// the depth>=2 eager-loaded _set collections must exclude tombstoned
// (status='deleted') rows, matching upstream's nested prefetch filter
// (peeringdb_server/serializers.py:928 filters status="ok"; we admit
// "pending" too, mirroring the depth-1 ID-list builders that were
// live-validated in v1.20.5). Before the fix the DEFAULT detail shape
// (depth=2) re-published deleted rows — including deleted POC contact
// data — indefinitely.
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
