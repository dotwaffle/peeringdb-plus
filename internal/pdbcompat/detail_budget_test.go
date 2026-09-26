package pdbcompat

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil/seed"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// seedOrgWithNetworks creates one org (id 1) plus n "ok" child networks,
// one "deleted" and one "pending" network that the estimate must NOT bill
// (depth expansion keeps only live children in its sets).
func seedOrgWithNetworks(t *testing.T, client *ent.Client, n int) {
	t.Helper()
	ctx := t.Context()
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	if _, err := client.Organization.Create().
		SetID(1).SetName("HubOrg").SetNameFold(unifold.Fold("HubOrg")).
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		Save(ctx); err != nil {
		t.Fatalf("seed org: %v", err)
	}
	for i := 1; i <= n; i++ {
		if _, err := client.Network.Create().
			SetID(i).SetOrgID(1).SetName("HubNet").SetNameFold(unifold.Fold("HubNet")).
			SetAsn(64500 + i).
			SetCreated(now).SetUpdated(now).SetStatus("ok").
			Save(ctx); err != nil {
			t.Fatalf("seed network %d: %v", i, err)
		}
	}
	if _, err := client.Network.Create().
		SetID(n + 1).SetOrgID(1).SetName("GoneNet").SetNameFold(unifold.Fold("GoneNet")).
		SetAsn(64500 + n + 1).
		SetCreated(now).SetUpdated(now).SetStatus("deleted").
		Save(ctx); err != nil {
		t.Fatalf("seed deleted network: %v", err)
	}
	if _, err := client.Network.Create().
		SetID(n + 2).SetOrgID(1).SetName("PendingNet").SetNameFold(unifold.Fold("PendingNet")).
		SetAsn(64500 + n + 2).
		SetCreated(now).SetUpdated(now).SetStatus("pending").
		Save(ctx); err != nil {
		t.Fatalf("seed pending network: %v", err)
	}
}

// TestDetailInflightEstimate_CountsChildren verifies the depth>=2 estimate
// is the flat Depth2 figure plus child COUNT(*) × child Depth0 per
// embedded set, that deleted and pending children are excluded (mirroring
// the StatusIn filter the depth expansion applies), and that depth<2 requests
// fall back to the flat TypicalRowBytes figure without any count queries.
func TestDetailInflightEstimate_CountsChildren(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := t.Context()

	const nets = 7
	seedOrgWithNetworks(t, client, nets)

	// depth=2: flat org Depth2 + 7 ok networks × net Depth0. The deleted
	// and pending networks and the empty fac/ix/carrier/campus sets add
	// nothing.
	want := int64(TypicalRowBytes(peeringdb.TypeOrg, 2)) +
		int64(nets)*int64(TypicalRowBytes(peeringdb.TypeNet, 0))
	if got := detailInflightEstimate(ctx, client, peeringdb.TypeOrg, 1, 2); got != want {
		t.Errorf("depth=2 estimate = %d, want %d", got, want)
	}

	// depth=1 renders sets as ID lists — flat expanded figure only.
	if got, want := detailInflightEstimate(ctx, client, peeringdb.TypeOrg, 1, 1), int64(TypicalRowBytes(peeringdb.TypeOrg, 1)); got != want {
		t.Errorf("depth=1 estimate = %d, want %d (flat, no child counts)", got, want)
	}

	// depth=0 is the bare row.
	if got, want := detailInflightEstimate(ctx, client, peeringdb.TypeOrg, 1, 0), int64(TypicalRowBytes(peeringdb.TypeOrg, 0)); got != want {
		t.Errorf("depth=0 estimate = %d, want %d (bare row)", got, want)
	}

	// A leaf type with no child-set entry prices flat at any depth.
	if got, want := detailInflightEstimate(ctx, client, peeringdb.TypePoc, 1, 2), int64(TypicalRowBytes(peeringdb.TypePoc, 2)); got != want {
		t.Errorf("leaf-type estimate = %d, want %d (flat Depth2)", got, want)
	}
}

// TestDetailInflightEstimate_FacPricesFlat verifies that a facility
// detail bills only its flat Depth2 figure. getFacWithDepth renders the
// org and campus but no reverse set, so the netfac, ixfac and carrierfac
// rows at the facility add nothing.
func TestDetailInflightEstimate_FacPricesFlat(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := t.Context()
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	org := client.Organization.Create().
		SetName("Org").SetNameFold(unifold.Fold("Org")).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	fac := client.Facility.Create().
		SetName("Fac").SetNameFold(unifold.Fold("Fac")).SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	net := client.Network.Create().
		SetName("Net").SetNameFold(unifold.Fold("Net")).SetAsn(65001).
		SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	ix := client.InternetExchange.Create().
		SetName("IX").SetNameFold(unifold.Fold("IX")).SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	carrier := client.Carrier.Create().
		SetName("Carrier").SetNameFold(unifold.Fold("Carrier")).SetOrgID(org.ID).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	client.NetworkFacility.Create().
		SetNetwork(net).SetFacility(fac).SetLocalAsn(65001).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	client.IxFacility.Create().
		SetInternetExchange(ix).SetFacility(fac).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	client.CarrierFacility.Create().
		SetCarrier(carrier).SetFacility(fac).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)

	want := int64(TypicalRowBytes(peeringdb.TypeFac, 2))
	if got := detailInflightEstimate(ctx, client, peeringdb.TypeFac, fac.ID, 2); got != want {
		t.Errorf("fac depth=2 estimate = %d, want %d (flat Depth2)", got, want)
	}
}

// TestDetailInflightEstimate_CountsLiveNetixlan verifies that the two
// sets built from netixlan rows, net.netixlan_set and the through-relation
// ixlan.net_set, bill the netixlan live statuses (ok and not-operational)
// and leave pending and deleted rows out, as the depth expansion does.
func TestDetailInflightEstimate_CountsLiveNetixlan(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := t.Context()
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	org := client.Organization.Create().
		SetName("Org").SetNameFold(unifold.Fold("Org")).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	net := client.Network.Create().
		SetName("Net").SetNameFold(unifold.Fold("Net")).SetAsn(65001).
		SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	ix := client.InternetExchange.Create().
		SetName("IX").SetNameFold(unifold.Fold("IX")).SetOrganization(org).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	lan := client.IxLan.Create().
		SetInternetExchange(ix).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	for _, status := range []string{"ok", "not-operational", "pending", "deleted"} {
		client.NetworkIxLan.Create().
			SetNetwork(net).SetIxLan(lan).SetAsn(65001).SetSpeed(1000).
			SetCreated(now).SetUpdated(now).SetStatus(status).SaveX(ctx)
	}

	// net: poc_set and netfac_set are empty, netixlan_set bills 2 rows.
	wantNet := int64(TypicalRowBytes(peeringdb.TypeNet, 2)) +
		2*int64(TypicalRowBytes(peeringdb.TypeNetIXLan, 0))
	if got := detailInflightEstimate(ctx, client, peeringdb.TypeNet, net.ID, 2); got != wantNet {
		t.Errorf("net depth=2 estimate = %d, want %d (ok + not-operational netixlans)", got, wantNet)
	}

	// ixlan: ixpfx_set is empty, net_set bills one network per live
	// netixlan join row.
	wantLan := int64(TypicalRowBytes(peeringdb.TypeIXLan, 2)) +
		2*int64(TypicalRowBytes(peeringdb.TypeNet, 0))
	if got := detailInflightEstimate(ctx, client, peeringdb.TypeIXLan, lan.ID, 2); got != wantLan {
		t.Errorf("ixlan depth=2 estimate = %d, want %d (ok + not-operational join rows)", got, wantLan)
	}
}

// TestChildSets_CoverRegistryParents locks the depth.go and childSets
// alignment at the type level: exactly the 6 parent types whose depth
// expansion embeds reverse sets have an entry, the keys of a parent are
// unique, and every childType in the table has a calibrated row size (an
// unknown name prices at defaultRowSize with no error).
func TestChildSets_CoverRegistryParents(t *testing.T) {
	t.Parallel()
	wantParents := map[string]int{
		peeringdb.TypeOrg:     5, // net, fac, ix, carrier, campus
		peeringdb.TypeNet:     3, // poc, netfac, netixlan
		peeringdb.TypeIX:      2, // ixlan, fac (via ixfac)
		peeringdb.TypeIXLan:   2, // ixpfx, net (via netixlan)
		peeringdb.TypeCarrier: 1, // carrierfac
		peeringdb.TypeCampus:  1, // fac
	}
	if len(childSets) != len(wantParents) {
		t.Errorf("childSets has %d parent types, want %d", len(childSets), len(wantParents))
	}
	for parent, wantSets := range wantParents {
		sets, found := childSets[parent]
		if !found {
			t.Errorf("childSets missing parent %q", parent)
			continue
		}
		if len(sets) != wantSets {
			t.Errorf("childSets[%q] has %d sets, want %d", parent, len(sets), wantSets)
		}
		keys := map[string]bool{}
		for _, cs := range sets {
			if keys[cs.key] {
				t.Errorf("childSets[%q] has key %q twice", parent, cs.key)
			}
			keys[cs.key] = true
			if _, calibrated := typicalRowBytes[cs.childType]; !calibrated {
				t.Errorf("childSets[%q] names uncalibrated child type %q", parent, cs.childType)
			}
		}
	}
}

// TestChildSets_CountOverIDs checks each countByParent query against the
// depth-2 detail render. Over all parent ids of the type plus one missing
// id, the result must equal the results of the single-id queries, must
// have no entry for a parent with no element or for an id outside ids,
// and the count of each live parent must equal the number of elements in
// the rendered set with the same key. The extra rows add a second org with
// a live and a deleted network, netixlans in each status (two live rows
// of one network make a duplicate in ixlan.net_set), and one row that is
// not live in each other child type. The seed has a Users poc, which the
// unstamped (Public) context must not count.
func TestChildSets_CountOverIDs(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := t.Context()
	r := seed.Full(t, client)
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	org2 := client.Organization.Create().
		SetID(2).SetName("SecondOrg").SetNameFold(unifold.Fold("SecondOrg")).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	for id, status := range map[int]string{9100: "ok", 9101: "deleted"} {
		client.Network.Create().
			SetID(id).SetOrganization(org2).SetAsn(64000 + id).
			SetName("Net" + strconv.Itoa(id)).SetNameFold(unifold.Fold("Net" + strconv.Itoa(id))).
			SetCreated(now).SetUpdated(now).SetStatus(status).SaveX(ctx)
	}
	for id, status := range map[int]string{9200: "ok", 9201: "not-operational", 9202: "deleted", 9203: "pending"} {
		client.NetworkIxLan.Create().
			SetID(id).SetNetID(9100).SetIxLan(r.IxLan).SetAsn(73100).SetSpeed(1000).
			SetCreated(now).SetUpdated(now).SetStatus(status).SaveX(ctx)
	}
	// One row that is not live in each other child type, under a live
	// parent, so that the count-versus-render check covers the status
	// filter of every set.
	client.Facility.Create().SetID(9300).SetName("DeletedFac").SetNameFold("deletedfac").
		SetOrganization(r.Org).SetCampus(r.Campus).
		SetCreated(now).SetUpdated(now).SetStatus("deleted").SaveX(ctx)
	client.Campus.Create().SetID(9301).SetName("PendingCampus").SetNameFold("pendingcampus").
		SetOrganization(r.Org).
		SetCreated(now).SetUpdated(now).SetStatus("pending").SaveX(ctx)
	client.InternetExchange.Create().SetID(9302).SetName("DeletedIX").SetNameFold("deletedix").
		SetOrganization(r.Org).SetCity("Frankfurt").SetCountry("DE").
		SetRegionContinent("Europe").SetMedia("Ethernet").
		SetCreated(now).SetUpdated(now).SetStatus("deleted").SaveX(ctx)
	client.Carrier.Create().SetID(9303).SetName("DeletedCarrier").SetNameFold("deletedcarrier").
		SetOrganization(r.Org).
		SetCreated(now).SetUpdated(now).SetStatus("deleted").SaveX(ctx)
	client.Poc.Create().SetID(9304).SetNetwork(r.Network).SetName("DeletedPoc").SetRole("NOC").
		SetVisible("Public").
		SetCreated(now).SetUpdated(now).SetStatus("deleted").SaveX(ctx)
	client.NetworkFacility.Create().SetID(9305).SetNetwork(r.Network2).SetFacility(r.Facility).
		SetLocalAsn(6939).
		SetCreated(now).SetUpdated(now).SetStatus("deleted").SaveX(ctx)
	client.IxFacility.Create().SetID(9306).SetInternetExchange(r.IX).SetFacility(r.Facility2).
		SetCreated(now).SetUpdated(now).SetStatus("deleted").SaveX(ctx)
	client.IxLan.Create().SetID(9307).SetInternetExchange(r.IX).
		SetCreated(now).SetUpdated(now).SetStatus("deleted").SaveX(ctx)
	client.IxPrefix.Create().SetID(9308).SetIxLan(r.IxLan).
		SetPrefix("80.81.200.0/22").SetProtocol("IPv4").
		SetCreated(now).SetUpdated(now).SetStatus("deleted").SaveX(ctx)
	client.CarrierFacility.Create().SetID(9309).SetCarrier(r.Carrier).SetFacility(r.Facility2).
		SetCreated(now).SetUpdated(now).SetStatus("deleted").SaveX(ctx)

	parentIDs := map[string]func() ([]int, error){
		peeringdb.TypeOrg:     func() ([]int, error) { return client.Organization.Query().IDs(ctx) },
		peeringdb.TypeNet:     func() ([]int, error) { return client.Network.Query().IDs(ctx) },
		peeringdb.TypeIX:      func() ([]int, error) { return client.InternetExchange.Query().IDs(ctx) },
		peeringdb.TypeIXLan:   func() ([]int, error) { return client.IxLan.Query().IDs(ctx) },
		peeringdb.TypeCarrier: func() ([]int, error) { return client.Carrier.Query().IDs(ctx) },
		peeringdb.TypeCampus:  func() ([]int, error) { return client.Campus.Query().IDs(ctx) },
	}
	const missingID = 999999
	for typ, sets := range childSets {
		ids, err := parentIDs[typ]()
		if err != nil {
			t.Fatalf("%s ids: %v", typ, err)
		}
		query := append(slices.Clone(ids), missingID)
		for _, cs := range sets {
			name := typ + "." + cs.key
			all, err := cs.countByParent(ctx, client, query)
			if err != nil {
				t.Fatalf("%s over %v: %v", name, query, err)
			}
			single := map[int]int{}
			for _, id := range query {
				one, err := cs.countByParent(ctx, client, []int{id})
				if err != nil {
					t.Fatalf("%s over [%d]: %v", name, id, err)
				}
				for k, n := range one {
					if k != id {
						t.Errorf("%s over [%d] has an entry for id %d", name, id, k)
					}
					single[k] = n
				}
			}
			if !maps.Equal(all, single) {
				t.Errorf("%s over %v = %v, single-id queries give %v", name, query, all, single)
			}
			for k, n := range all {
				if n <= 0 || !slices.Contains(ids, k) {
					t.Errorf("%s has entry %d: %d, want only parents with elements", name, k, n)
				}
			}
		}
		for _, id := range ids {
			got, err := Registry[typ].Get(ctx, client, id, 2)
			if ent.IsNotFound(err) {
				continue // a deleted parent has no detail response
			}
			if err != nil {
				t.Fatalf("%s/%d depth 2: %v", typ, id, err)
			}
			m, ok := got.(map[string]any)
			if !ok {
				t.Fatalf("%s/%d depth 2 renders %T, want map[string]any", typ, id, got)
			}
			for _, cs := range sets {
				set, found := m[cs.key]
				if !found {
					t.Errorf("%s/%d depth 2 has no key %q", typ, id, cs.key)
					continue
				}
				counts, err := cs.countByParent(ctx, client, []int{id})
				if err != nil {
					t.Fatalf("%s.%s over [%d]: %v", typ, cs.key, id, err)
				}
				if n := reflect.ValueOf(set).Len(); n != counts[id] {
					t.Errorf("%s/%d %s renders %d elements, countByParent gives %d", typ, id, cs.key, n, counts[id])
				}
			}
		}
	}
}

// TestServeDetail_ConcurrentBudgetPool mirrors
// TestServeList_ConcurrentBudgetPool for the detail path: a depth=2 hub
// object passes the flat per-request 413 check but must still charge its
// child fan-out against the shared in-flight pool, 503 when the pool is
// nearly full, and refund its charge on every path.
func TestServeDetail_ConcurrentBudgetPool(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)

	const nets = 40
	seedOrgWithNetworks(t, client, nets)

	// The count-based fan-out estimate for /api/org/1 at the default
	// depth=2. Budget admits it once with headroom, but not twice.
	estimate := int64(TypicalRowBytes(peeringdb.TypeOrg, 2)) +
		int64(nets)*int64(TypicalRowBytes(peeringdb.TypeNet, 0))
	budget := estimate + estimate/2

	h := NewHandler(client, budget)
	mux := http.NewServeMux()
	h.Register(mux)

	get := func() int {
		req := httptest.NewRequest(http.MethodGet, "/api/org/1", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}

	// Baseline: fits with an empty pool (the flat 413 check passes and
	// the fan-out charge fits under budget).
	if code := get(); code != http.StatusOK {
		t.Fatalf("baseline GET: status %d, want 200", code)
	}

	// Simulate another in-flight near-budget response holding its charge.
	h.inflightBytes.Add(estimate)
	if code := get(); code != http.StatusServiceUnavailable {
		t.Errorf("with pool nearly full: status %d, want 503", code)
	}

	// Once the other response releases, details are admitted again —
	// proving the rejection path refunded its charge.
	h.inflightBytes.Add(-estimate)
	if code := get(); code != http.StatusOK {
		t.Errorf("after release: status %d, want 200 (charge leak?)", code)
	}
	if got := h.inflightBytes.Load(); got != 0 {
		t.Errorf("in-flight pool = %d after all requests done, want 0", got)
	}
}
