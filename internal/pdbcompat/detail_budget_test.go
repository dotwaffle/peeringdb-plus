package pdbcompat

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// seedOrgWithNetworks creates one org (id 1) plus n "ok" child networks
// and one "deleted" network that the estimate must NOT bill (depth
// expansion filters sets to StatusIn ok/pending).
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
}

// TestDetailInflightEstimate_CountsChildren verifies the depth>=2 estimate
// is the flat Depth2 figure plus child COUNT(*) × child Depth0 per
// embedded set, that tombstoned children are excluded (mirroring the
// StatusIn filter the depth expansion applies), and that depth<2 requests
// fall back to the flat TypicalRowBytes figure without any count queries.
func TestDetailInflightEstimate_CountsChildren(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := t.Context()

	const nets = 7
	seedOrgWithNetworks(t, client, nets)

	// depth=2: flat org Depth2 + 7 ok networks × net Depth0. The deleted
	// network and the empty fac/ix/carrier/campus sets add nothing.
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

// TestDetailChildSets_CoverRegistryParents locks the depth.go ↔
// detailChildSets alignment at the type level: exactly the 7 parent types
// whose depth>=2 expansion embeds full child objects carry an entry, and
// every childType named in the table has a calibrated row size (an
// unknown name would silently price at defaultRowSize).
func TestDetailChildSets_CoverRegistryParents(t *testing.T) {
	t.Parallel()
	wantParents := map[string]int{
		peeringdb.TypeOrg:     5, // net, fac, ix, carrier, campus
		peeringdb.TypeNet:     3, // poc, netfac, netixlan
		peeringdb.TypeFac:     3, // netfac, ixfac, carrierfac
		peeringdb.TypeIX:      2, // ixlan, fac (via ixfac)
		peeringdb.TypeIXLan:   2, // ixpfx, net (via netixlan)
		peeringdb.TypeCarrier: 1, // carrierfac
		peeringdb.TypeCampus:  1, // fac
	}
	if len(detailChildSets) != len(wantParents) {
		t.Errorf("detailChildSets has %d parent types, want %d", len(detailChildSets), len(wantParents))
	}
	for parent, wantSets := range wantParents {
		sets, found := detailChildSets[parent]
		if !found {
			t.Errorf("detailChildSets missing parent %q", parent)
			continue
		}
		if len(sets) != wantSets {
			t.Errorf("detailChildSets[%q] has %d sets, want %d", parent, len(sets), wantSets)
		}
		for _, cs := range sets {
			if _, calibrated := typicalRowBytes[cs.childType]; !calibrated {
				t.Errorf("detailChildSets[%q] names uncalibrated child type %q", parent, cs.childType)
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
