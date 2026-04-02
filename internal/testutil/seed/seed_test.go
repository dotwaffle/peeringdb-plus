package seed_test

import (
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil/seed"
)

func TestFull(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	r := seed.Full(t, client)

	// All 15 fields must be non-nil (13 types + Network2 + Facility2).
	checks := []struct {
		name string
		ok   bool
	}{
		{"Org", r.Org != nil},
		{"Network", r.Network != nil},
		{"Network2", r.Network2 != nil},
		{"IX", r.IX != nil},
		{"Facility", r.Facility != nil},
		{"Facility2", r.Facility2 != nil},
		{"Campus", r.Campus != nil},
		{"Carrier", r.Carrier != nil},
		{"IxLan", r.IxLan != nil},
		{"IxPrefix", r.IxPrefix != nil},
		{"NetworkIxLan", r.NetworkIxLan != nil},
		{"NetworkFacility", r.NetworkFacility != nil},
		{"IxFacility", r.IxFacility != nil},
		{"CarrierFacility", r.CarrierFacility != nil},
		{"Poc", r.Poc != nil},
	}
	for _, c := range checks {
		if !c.ok {
			t.Errorf("Result.%s is nil", c.name)
		}
	}

	// Deterministic IDs.
	ids := []struct {
		name string
		got  int
		want int
	}{
		{"Org", r.Org.ID, 1},
		{"Network", r.Network.ID, 10},
		{"Network2", r.Network2.ID, 11},
		{"IX", r.IX.ID, 20},
		{"Facility", r.Facility.ID, 30},
		{"Facility2", r.Facility2.ID, 31},
		{"Campus", r.Campus.ID, 40},
		{"Carrier", r.Carrier.ID, 50},
		{"IxLan", r.IxLan.ID, 100},
		{"IxPrefix", r.IxPrefix.ID, 700},
		{"NetworkIxLan", r.NetworkIxLan.ID, 200},
		{"NetworkFacility", r.NetworkFacility.ID, 300},
		{"IxFacility", r.IxFacility.ID, 400},
		{"CarrierFacility", r.CarrierFacility.ID, 600},
		{"Poc", r.Poc.ID, 500},
	}
	for _, id := range ids {
		if id.got != id.want {
			t.Errorf("Result.%s.ID = %d, want %d", id.name, id.got, id.want)
		}
	}

	// Realistic names.
	names := []struct {
		name string
		val  string
	}{
		{"Org", r.Org.Name},
		{"Network", r.Network.Name},
		{"IX", r.IX.Name},
		{"Facility", r.Facility.Name},
	}
	for _, n := range names {
		if n.val == "" {
			t.Errorf("Result.%s.Name is empty", n.name)
		}
	}

	// AllNetworks must contain both networks.
	if len(r.AllNetworks) != 2 {
		t.Errorf("AllNetworks length = %d, want 2", len(r.AllNetworks))
	}
}

func TestFull_EntityCounts(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	seed.Full(t, client)

	ctx := t.Context()
	counts := []struct {
		name string
		got  int
		want int
	}{
		{"Organization", must(client.Organization.Query().Count(ctx)), 1},
		{"Network", must(client.Network.Query().Count(ctx)), 2},
		{"InternetExchange", must(client.InternetExchange.Query().Count(ctx)), 1},
		{"Facility", must(client.Facility.Query().Count(ctx)), 2},
		{"Campus", must(client.Campus.Query().Count(ctx)), 1},
		{"Carrier", must(client.Carrier.Query().Count(ctx)), 1},
		{"IxLan", must(client.IxLan.Query().Count(ctx)), 1},
		{"IxPrefix", must(client.IxPrefix.Query().Count(ctx)), 1},
		{"NetworkIxLan", must(client.NetworkIxLan.Query().Count(ctx)), 1},
		{"NetworkFacility", must(client.NetworkFacility.Query().Count(ctx)), 1},
		{"IxFacility", must(client.IxFacility.Query().Count(ctx)), 1},
		{"CarrierFacility", must(client.CarrierFacility.Query().Count(ctx)), 1},
		{"Poc", must(client.Poc.Query().Count(ctx)), 1},
	}
	for _, c := range counts {
		if c.got != c.want {
			t.Errorf("%s count = %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestFull_Relationships(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	r := seed.Full(t, client)

	ctx := t.Context()

	// NetworkIxLan -> IxLan
	nixl, err := client.NetworkIxLan.Get(ctx, r.NetworkIxLan.ID)
	if err != nil {
		t.Fatalf("get NetworkIxLan: %v", err)
	}
	ixlan, err := nixl.QueryIxLan().Only(ctx)
	if err != nil {
		t.Fatalf("query NetworkIxLan->IxLan: %v", err)
	}
	if ixlan.ID != r.IxLan.ID {
		t.Errorf("NetworkIxLan->IxLan.ID = %d, want %d", ixlan.ID, r.IxLan.ID)
	}

	// NetworkIxLan -> Network
	net, err := nixl.QueryNetwork().Only(ctx)
	if err != nil {
		t.Fatalf("query NetworkIxLan->Network: %v", err)
	}
	if net.ID != r.Network.ID {
		t.Errorf("NetworkIxLan->Network.ID = %d, want %d", net.ID, r.Network.ID)
	}

	// NetworkFacility -> Network
	nf, err := client.NetworkFacility.Get(ctx, r.NetworkFacility.ID)
	if err != nil {
		t.Fatalf("get NetworkFacility: %v", err)
	}
	nfNet, err := nf.QueryNetwork().Only(ctx)
	if err != nil {
		t.Fatalf("query NetworkFacility->Network: %v", err)
	}
	if nfNet.ID != r.Network.ID {
		t.Errorf("NetworkFacility->Network.ID = %d, want %d", nfNet.ID, r.Network.ID)
	}

	// NetworkFacility -> Facility
	nfFac, err := nf.QueryFacility().Only(ctx)
	if err != nil {
		t.Fatalf("query NetworkFacility->Facility: %v", err)
	}
	if nfFac.ID != r.Facility.ID {
		t.Errorf("NetworkFacility->Facility.ID = %d, want %d", nfFac.ID, r.Facility.ID)
	}

	// IxFacility -> InternetExchange
	ixf, err := client.IxFacility.Get(ctx, r.IxFacility.ID)
	if err != nil {
		t.Fatalf("get IxFacility: %v", err)
	}
	ixfIX, err := ixf.QueryInternetExchange().Only(ctx)
	if err != nil {
		t.Fatalf("query IxFacility->IX: %v", err)
	}
	if ixfIX.ID != r.IX.ID {
		t.Errorf("IxFacility->IX.ID = %d, want %d", ixfIX.ID, r.IX.ID)
	}

	// CarrierFacility -> Carrier
	cf, err := client.CarrierFacility.Get(ctx, r.CarrierFacility.ID)
	if err != nil {
		t.Fatalf("get CarrierFacility: %v", err)
	}
	cfCarrier, err := cf.QueryCarrier().Only(ctx)
	if err != nil {
		t.Fatalf("query CarrierFacility->Carrier: %v", err)
	}
	if cfCarrier.ID != r.Carrier.ID {
		t.Errorf("CarrierFacility->Carrier.ID = %d, want %d", cfCarrier.ID, r.Carrier.ID)
	}

	// Poc -> Network
	poc, err := client.Poc.Get(ctx, r.Poc.ID)
	if err != nil {
		t.Fatalf("get Poc: %v", err)
	}
	pocNet, err := poc.QueryNetwork().Only(ctx)
	if err != nil {
		t.Fatalf("query Poc->Network: %v", err)
	}
	if pocNet.ID != r.Network.ID {
		t.Errorf("Poc->Network.ID = %d, want %d", pocNet.ID, r.Network.ID)
	}

	// IxPrefix -> IxLan
	ixp, err := client.IxPrefix.Get(ctx, r.IxPrefix.ID)
	if err != nil {
		t.Fatalf("get IxPrefix: %v", err)
	}
	ixpIxlan, err := ixp.QueryIxLan().Only(ctx)
	if err != nil {
		t.Fatalf("query IxPrefix->IxLan: %v", err)
	}
	if ixpIxlan.ID != r.IxLan.ID {
		t.Errorf("IxPrefix->IxLan.ID = %d, want %d", ixpIxlan.ID, r.IxLan.ID)
	}
}

func TestMinimal(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	r := seed.Minimal(t, client)

	// Core 4 entities must be non-nil.
	if r.Org == nil {
		t.Error("Minimal: Org is nil")
	}
	if r.Network == nil {
		t.Error("Minimal: Network is nil")
	}
	if r.IX == nil {
		t.Error("Minimal: IX is nil")
	}
	if r.Facility == nil {
		t.Error("Minimal: Facility is nil")
	}

	// Junction types must be nil.
	nilChecks := []struct {
		name string
		ok   bool
	}{
		{"Network2", r.Network2 == nil},
		{"Facility2", r.Facility2 == nil},
		{"Campus", r.Campus == nil},
		{"Carrier", r.Carrier == nil},
		{"IxLan", r.IxLan == nil},
		{"IxPrefix", r.IxPrefix == nil},
		{"NetworkIxLan", r.NetworkIxLan == nil},
		{"NetworkFacility", r.NetworkFacility == nil},
		{"IxFacility", r.IxFacility == nil},
		{"CarrierFacility", r.CarrierFacility == nil},
		{"Poc", r.Poc == nil},
	}
	for _, c := range nilChecks {
		if !c.ok {
			t.Errorf("Minimal: Result.%s should be nil", c.name)
		}
	}

	// AllNetworks should contain one network.
	if len(r.AllNetworks) != 1 {
		t.Errorf("Minimal: AllNetworks length = %d, want 1", len(r.AllNetworks))
	}
}

func TestMinimal_EntityCounts(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	seed.Minimal(t, client)

	ctx := t.Context()
	counts := []struct {
		name string
		got  int
		want int
	}{
		{"Organization", must(client.Organization.Query().Count(ctx)), 1},
		{"Network", must(client.Network.Query().Count(ctx)), 1},
		{"InternetExchange", must(client.InternetExchange.Query().Count(ctx)), 1},
		{"Facility", must(client.Facility.Query().Count(ctx)), 1},
		{"Campus", must(client.Campus.Query().Count(ctx)), 0},
		{"Carrier", must(client.Carrier.Query().Count(ctx)), 0},
		{"IxLan", must(client.IxLan.Query().Count(ctx)), 0},
		{"IxPrefix", must(client.IxPrefix.Query().Count(ctx)), 0},
		{"NetworkIxLan", must(client.NetworkIxLan.Query().Count(ctx)), 0},
		{"NetworkFacility", must(client.NetworkFacility.Query().Count(ctx)), 0},
		{"IxFacility", must(client.IxFacility.Query().Count(ctx)), 0},
		{"CarrierFacility", must(client.CarrierFacility.Query().Count(ctx)), 0},
		{"Poc", must(client.Poc.Query().Count(ctx)), 0},
	}
	for _, c := range counts {
		if c.got != c.want {
			t.Errorf("%s count = %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestNetworks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		n    int
	}{
		{"one network", 1},
		{"two networks", 2},
		{"five networks", 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := testutil.SetupClient(t)
			r := seed.Networks(t, client, tt.n)

			if r.Org == nil {
				t.Fatal("Networks: Org is nil")
			}
			if r.Network == nil {
				t.Fatal("Networks: Network (first) is nil")
			}
			if len(r.AllNetworks) != tt.n {
				t.Fatalf("AllNetworks length = %d, want %d", len(r.AllNetworks), tt.n)
			}

			// Each network must have a unique ASN starting at 65001.
			seen := make(map[int]bool)
			for i, net := range r.AllNetworks {
				wantASN := 65001 + i
				if net.Asn != wantASN {
					t.Errorf("AllNetworks[%d].Asn = %d, want %d", i, net.Asn, wantASN)
				}
				if seen[net.Asn] {
					t.Errorf("duplicate ASN %d", net.Asn)
				}
				seen[net.Asn] = true
			}
		})
	}
}

// must is a test helper that returns the value or panics on error.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
