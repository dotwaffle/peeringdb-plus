package schema_test

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent"
	entschema "entgo.io/ent/schema"

	"github.com/dotwaffle/peeringdb-plus/ent/schema"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// testTimestamp provides a consistent timestamp for all tests.
var testTimestamp = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// TestAllSchemasCRUD verifies that all 13 PeeringDB object types can be created
// and queried back with correct field values. Table-driven per T-1.
func TestAllSchemasCRUD(t *testing.T) {
	t.Parallel()

	client := testutil.SetupClient(t)
	ctx := context.Background()

	// Create Organization (parent for many types)
	org, err := client.Organization.Create().
		SetID(100).
		SetName("Test Org").
		SetCreated(testTimestamp).
		SetUpdated(testTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating organization: %v", err)
	}
	if org.Name != "Test Org" {
		t.Errorf("org.Name = %q, want %q", org.Name, "Test Org")
	}

	t.Run("Organization", func(t *testing.T) {
		t.Parallel()
		got, err := client.Organization.Get(ctx, 100)
		if err != nil {
			t.Fatalf("querying organization: %v", err)
		}
		if got.Name != "Test Org" {
			t.Errorf("name = %q, want %q", got.Name, "Test Org")
		}
		if got.Status != "ok" {
			t.Errorf("status = %q, want %q", got.Status, "ok")
		}
	})

	t.Run("Network", func(t *testing.T) {
		t.Parallel()
		net, err := client.Network.Create().
			SetID(200).
			SetName("Test Network").
			SetAsn(65000).
			SetOrgID(100).
			SetOrganization(org).
			SetCreated(testTimestamp).
			SetUpdated(testTimestamp).
			Save(ctx)
		if err != nil {
			t.Fatalf("creating network: %v", err)
		}
		got, err := client.Network.Get(ctx, net.ID)
		if err != nil {
			t.Fatalf("querying network: %v", err)
		}
		if got.Name != "Test Network" {
			t.Errorf("name = %q, want %q", got.Name, "Test Network")
		}
		if got.Asn != 65000 {
			t.Errorf("asn = %d, want %d", got.Asn, 65000)
		}
		if got.OrgID == nil || *got.OrgID != 100 {
			t.Errorf("org_id = %v, want 100", got.OrgID)
		}
	})

	t.Run("Facility", func(t *testing.T) {
		t.Parallel()
		fac, err := client.Facility.Create().
			SetID(300).
			SetName("Test Facility").
			SetOrgID(100).
			SetOrganization(org).
			SetClli("SFCACA01").
			SetCreated(testTimestamp).
			SetUpdated(testTimestamp).
			Save(ctx)
		if err != nil {
			t.Fatalf("creating facility: %v", err)
		}
		got, err := client.Facility.Get(ctx, fac.ID)
		if err != nil {
			t.Fatalf("querying facility: %v", err)
		}
		if got.Name != "Test Facility" {
			t.Errorf("name = %q, want %q", got.Name, "Test Facility")
		}
		if got.Clli != "SFCACA01" {
			t.Errorf("clli = %q, want %q", got.Clli, "SFCACA01")
		}
	})

	t.Run("InternetExchange", func(t *testing.T) {
		t.Parallel()
		ix, err := client.InternetExchange.Create().
			SetID(400).
			SetName("Test IX").
			SetOrgID(100).
			SetOrganization(org).
			SetCity("Amsterdam").
			SetCountry("NL").
			SetRegionContinent("Europe").
			SetMedia("Ethernet").
			SetCreated(testTimestamp).
			SetUpdated(testTimestamp).
			Save(ctx)
		if err != nil {
			t.Fatalf("creating internet exchange: %v", err)
		}
		got, err := client.InternetExchange.Get(ctx, ix.ID)
		if err != nil {
			t.Fatalf("querying internet exchange: %v", err)
		}
		if got.Name != "Test IX" {
			t.Errorf("name = %q, want %q", got.Name, "Test IX")
		}
		if got.Media != "Ethernet" {
			t.Errorf("media = %q, want %q", got.Media, "Ethernet")
		}
	})

	t.Run("Campus", func(t *testing.T) {
		t.Parallel()
		campus, err := client.Campus.Create().
			SetID(500).
			SetName("Test Campus").
			SetOrgID(100).
			SetOrganization(org).
			SetCountry("US").
			SetCity("Ashburn").
			SetCreated(testTimestamp).
			SetUpdated(testTimestamp).
			Save(ctx)
		if err != nil {
			t.Fatalf("creating campus: %v", err)
		}
		got, err := client.Campus.Get(ctx, campus.ID)
		if err != nil {
			t.Fatalf("querying campus: %v", err)
		}
		if got.Name != "Test Campus" {
			t.Errorf("name = %q, want %q", got.Name, "Test Campus")
		}
		if got.OrgID == nil || *got.OrgID != 100 {
			t.Errorf("org_id = %v, want 100", got.OrgID)
		}
	})

	t.Run("Carrier", func(t *testing.T) {
		t.Parallel()
		carrier, err := client.Carrier.Create().
			SetID(600).
			SetName("Test Carrier").
			SetOrgID(100).
			SetOrganization(org).
			SetCreated(testTimestamp).
			SetUpdated(testTimestamp).
			Save(ctx)
		if err != nil {
			t.Fatalf("creating carrier: %v", err)
		}
		got, err := client.Carrier.Get(ctx, carrier.ID)
		if err != nil {
			t.Fatalf("querying carrier: %v", err)
		}
		if got.Name != "Test Carrier" {
			t.Errorf("name = %q, want %q", got.Name, "Test Carrier")
		}
	})

	t.Run("IxLan", func(t *testing.T) {
		t.Parallel()
		// Create IX first for FK
		ix, err := client.InternetExchange.Create().
			SetID(401).
			SetName("IXLan Test IX").
			SetCreated(testTimestamp).
			SetUpdated(testTimestamp).
			Save(ctx)
		if err != nil {
			t.Fatalf("creating IX for IxLan: %v", err)
		}
		ixlan, err := client.IxLan.Create().
			SetID(700).
			SetIxID(ix.ID).
			SetInternetExchange(ix).
			SetMtu(9000).
			SetCreated(testTimestamp).
			SetUpdated(testTimestamp).
			Save(ctx)
		if err != nil {
			t.Fatalf("creating ixlan: %v", err)
		}
		got, err := client.IxLan.Get(ctx, ixlan.ID)
		if err != nil {
			t.Fatalf("querying ixlan: %v", err)
		}
		if got.Mtu != 9000 {
			t.Errorf("mtu = %d, want %d", got.Mtu, 9000)
		}
	})

	t.Run("IxPrefix", func(t *testing.T) {
		t.Parallel()
		// Create IX and IxLan first for FK chain
		ix2, err := client.InternetExchange.Create().
			SetID(402).
			SetName("IxPrefix Test IX").
			SetCreated(testTimestamp).
			SetUpdated(testTimestamp).
			Save(ctx)
		if err != nil {
			t.Fatalf("creating IX for IxPrefix: %v", err)
		}
		ixlan2, err := client.IxLan.Create().
			SetID(701).
			SetInternetExchange(ix2).
			SetCreated(testTimestamp).
			SetUpdated(testTimestamp).
			Save(ctx)
		if err != nil {
			t.Fatalf("creating IxLan for IxPrefix: %v", err)
		}
		pfx, err := client.IxPrefix.Create().
			SetID(800).
			SetIxlanID(ixlan2.ID).
			SetIxLan(ixlan2).
			SetProtocol("IPv4").
			SetPrefix("10.0.0.0/24").
			SetCreated(testTimestamp).
			SetUpdated(testTimestamp).
			Save(ctx)
		if err != nil {
			t.Fatalf("creating ixprefix: %v", err)
		}
		got, err := client.IxPrefix.Get(ctx, pfx.ID)
		if err != nil {
			t.Fatalf("querying ixprefix: %v", err)
		}
		if got.Prefix != "10.0.0.0/24" {
			t.Errorf("prefix = %q, want %q", got.Prefix, "10.0.0.0/24")
		}
		if got.Protocol != "IPv4" {
			t.Errorf("protocol = %q, want %q", got.Protocol, "IPv4")
		}
	})

	t.Run("Poc", func(t *testing.T) {
		t.Parallel()
		// Create Network first for FK
		net2, err := client.Network.Create().
			SetID(201).
			SetName("POC Test Network").
			SetAsn(65001).
			SetCreated(testTimestamp).
			SetUpdated(testTimestamp).
			Save(ctx)
		if err != nil {
			t.Fatalf("creating network for Poc: %v", err)
		}
		poc, err := client.Poc.Create().
			SetID(900).
			SetNetID(net2.ID).
			SetNetwork(net2).
			SetRole("Technical").
			SetVisible("Public").
			SetName("John Doe").
			SetEmail("john@example.com").
			SetCreated(testTimestamp).
			SetUpdated(testTimestamp).
			Save(ctx)
		if err != nil {
			t.Fatalf("creating poc: %v", err)
		}
		got, err := client.Poc.Get(ctx, poc.ID)
		if err != nil {
			t.Fatalf("querying poc: %v", err)
		}
		if got.Role != "Technical" {
			t.Errorf("role = %q, want %q", got.Role, "Technical")
		}
	})

	t.Run("NetworkFacility", func(t *testing.T) {
		t.Parallel()
		net3, err := client.Network.Create().
			SetID(202).
			SetName("NetFac Test Network").
			SetAsn(65002).
			SetCreated(testTimestamp).
			SetUpdated(testTimestamp).
			Save(ctx)
		if err != nil {
			t.Fatalf("creating network for NetworkFacility: %v", err)
		}
		fac3, err := client.Facility.Create().
			SetID(301).
			SetName("NetFac Test Facility").
			SetCreated(testTimestamp).
			SetUpdated(testTimestamp).
			Save(ctx)
		if err != nil {
			t.Fatalf("creating facility for NetworkFacility: %v", err)
		}
		nf, err := client.NetworkFacility.Create().
			SetID(1000).
			SetNetID(net3.ID).
			SetNetwork(net3).
			SetFacID(fac3.ID).
			SetFacility(fac3).
			SetLocalAsn(65002).
			SetCreated(testTimestamp).
			SetUpdated(testTimestamp).
			Save(ctx)
		if err != nil {
			t.Fatalf("creating network facility: %v", err)
		}
		got, err := client.NetworkFacility.Get(ctx, nf.ID)
		if err != nil {
			t.Fatalf("querying network facility: %v", err)
		}
		if got.LocalAsn != 65002 {
			t.Errorf("local_asn = %d, want %d", got.LocalAsn, 65002)
		}
	})

	t.Run("NetworkIxLan", func(t *testing.T) {
		t.Parallel()
		net4, err := client.Network.Create().
			SetID(203).
			SetName("NetIxLan Test Network").
			SetAsn(65003).
			SetCreated(testTimestamp).
			SetUpdated(testTimestamp).
			Save(ctx)
		if err != nil {
			t.Fatalf("creating network for NetworkIxLan: %v", err)
		}
		ix4, err := client.InternetExchange.Create().
			SetID(403).
			SetName("NetIxLan Test IX").
			SetCreated(testTimestamp).
			SetUpdated(testTimestamp).
			Save(ctx)
		if err != nil {
			t.Fatalf("creating IX for NetworkIxLan: %v", err)
		}
		ixlan4, err := client.IxLan.Create().
			SetID(702).
			SetInternetExchange(ix4).
			SetCreated(testTimestamp).
			SetUpdated(testTimestamp).
			Save(ctx)
		if err != nil {
			t.Fatalf("creating IxLan for NetworkIxLan: %v", err)
		}
		nixl, err := client.NetworkIxLan.Create().
			SetID(1100).
			SetNetID(net4.ID).
			SetNetwork(net4).
			SetIxlanID(ixlan4.ID).
			SetIxLan(ixlan4).
			SetSpeed(10000).
			SetAsn(65003).
			SetIpaddr4("192.0.2.1").
			SetIpaddr6("2001:db8::1").
			SetCreated(testTimestamp).
			SetUpdated(testTimestamp).
			Save(ctx)
		if err != nil {
			t.Fatalf("creating network ixlan: %v", err)
		}
		got, err := client.NetworkIxLan.Get(ctx, nixl.ID)
		if err != nil {
			t.Fatalf("querying network ixlan: %v", err)
		}
		if got.Ipaddr4 == nil || *got.Ipaddr4 != "192.0.2.1" {
			t.Errorf("ipaddr4 = %v, want %q", got.Ipaddr4, "192.0.2.1")
		}
		if got.Ipaddr6 == nil || *got.Ipaddr6 != "2001:db8::1" {
			t.Errorf("ipaddr6 = %v, want %q", got.Ipaddr6, "2001:db8::1")
		}
		if got.Speed != 10000 {
			t.Errorf("speed = %d, want %d", got.Speed, 10000)
		}
	})

	t.Run("IxFacility", func(t *testing.T) {
		t.Parallel()
		ix5, err := client.InternetExchange.Create().
			SetID(404).
			SetName("IxFac Test IX").
			SetCreated(testTimestamp).
			SetUpdated(testTimestamp).
			Save(ctx)
		if err != nil {
			t.Fatalf("creating IX for IxFacility: %v", err)
		}
		fac5, err := client.Facility.Create().
			SetID(302).
			SetName("IxFac Test Facility").
			SetCreated(testTimestamp).
			SetUpdated(testTimestamp).
			Save(ctx)
		if err != nil {
			t.Fatalf("creating facility for IxFacility: %v", err)
		}
		ixf, err := client.IxFacility.Create().
			SetID(1200).
			SetIxID(ix5.ID).
			SetInternetExchange(ix5).
			SetFacID(fac5.ID).
			SetFacility(fac5).
			SetCreated(testTimestamp).
			SetUpdated(testTimestamp).
			Save(ctx)
		if err != nil {
			t.Fatalf("creating ix facility: %v", err)
		}
		got, err := client.IxFacility.Get(ctx, ixf.ID)
		if err != nil {
			t.Fatalf("querying ix facility: %v", err)
		}
		if got.IxID == nil || *got.IxID != 404 {
			t.Errorf("ix_id = %v, want 404", got.IxID)
		}
		if got.FacID == nil || *got.FacID != 302 {
			t.Errorf("fac_id = %v, want 302", got.FacID)
		}
	})

	t.Run("CarrierFacility", func(t *testing.T) {
		t.Parallel()
		carrier6, err := client.Carrier.Create().
			SetID(601).
			SetName("CarrierFac Test Carrier").
			SetCreated(testTimestamp).
			SetUpdated(testTimestamp).
			Save(ctx)
		if err != nil {
			t.Fatalf("creating carrier for CarrierFacility: %v", err)
		}
		fac6, err := client.Facility.Create().
			SetID(303).
			SetName("CarrierFac Test Facility").
			SetCreated(testTimestamp).
			SetUpdated(testTimestamp).
			Save(ctx)
		if err != nil {
			t.Fatalf("creating facility for CarrierFacility: %v", err)
		}
		cf, err := client.CarrierFacility.Create().
			SetID(1300).
			SetCarrierID(carrier6.ID).
			SetCarrier(carrier6).
			SetFacID(fac6.ID).
			SetFacility(fac6).
			SetCreated(testTimestamp).
			SetUpdated(testTimestamp).
			Save(ctx)
		if err != nil {
			t.Fatalf("creating carrier facility: %v", err)
		}
		got, err := client.CarrierFacility.Get(ctx, cf.ID)
		if err != nil {
			t.Fatalf("querying carrier facility: %v", err)
		}
		if got.CarrierID == nil || *got.CarrierID != 601 {
			t.Errorf("carrier_id = %v, want 601", got.CarrierID)
		}
		if got.FacID == nil || *got.FacID != 303 {
			t.Errorf("fac_id = %v, want 303", got.FacID)
		}
	})
}

// TestNullableFK verifies that nullable FK fields accept nil values per D-20.
// PeeringDB has referential integrity violations that must not break our schema.
func TestNullableFK(t *testing.T) {
	t.Parallel()

	client := testutil.SetupClient(t)
	ctx := context.Background()

	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "NetworkFacility with nil FKs",
			fn: func() error {
				_, err := client.NetworkFacility.Create().
					SetID(2000).
					SetLocalAsn(65000).
					SetCreated(testTimestamp).
					SetUpdated(testTimestamp).
					Save(ctx)
				return err
			},
		},
		{
			name: "NetworkIxLan with nil FKs",
			fn: func() error {
				_, err := client.NetworkIxLan.Create().
					SetID(2001).
					SetSpeed(10000).
					SetAsn(65000).
					SetCreated(testTimestamp).
					SetUpdated(testTimestamp).
					Save(ctx)
				return err
			},
		},
		{
			name: "IxFacility with nil FKs",
			fn: func() error {
				_, err := client.IxFacility.Create().
					SetID(2002).
					SetCreated(testTimestamp).
					SetUpdated(testTimestamp).
					Save(ctx)
				return err
			},
		},
		{
			name: "CarrierFacility with nil FKs",
			fn: func() error {
				_, err := client.CarrierFacility.Create().
					SetID(2003).
					SetCreated(testTimestamp).
					SetUpdated(testTimestamp).
					Save(ctx)
				return err
			},
		},
		{
			name: "Poc with nil net_id",
			fn: func() error {
				_, err := client.Poc.Create().
					SetID(2004).
					SetRole("Technical").
					SetCreated(testTimestamp).
					SetUpdated(testTimestamp).
					Save(ctx)
				return err
			},
		},
		{
			name: "Network with nil org_id",
			fn: func() error {
				_, err := client.Network.Create().
					SetID(2005).
					SetName("Orphan Network").
					SetAsn(64500).
					SetCreated(testTimestamp).
					SetUpdated(testTimestamp).
					Save(ctx)
				return err
			},
		},
		{
			name: "Facility with nil org_id and nil campus_id",
			fn: func() error {
				_, err := client.Facility.Create().
					SetID(2006).
					SetName("Orphan Facility").
					SetCreated(testTimestamp).
					SetUpdated(testTimestamp).
					Save(ctx)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := tt.fn(); err != nil {
				t.Fatalf("creating with nil FK: %v", err)
			}
		})
	}
}

// TestEdgeTraversal verifies that edge traversal works across entity relationships.
// Creates an org -> network -> network_ix_lan chain and traverses it.
func TestEdgeTraversal(t *testing.T) {
	t.Parallel()

	client := testutil.SetupClient(t)
	ctx := context.Background()

	// Create org -> network -> network_ix_lan chain
	org, err := client.Organization.Create().
		SetID(3000).
		SetName("Traversal Org").
		SetCreated(testTimestamp).
		SetUpdated(testTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating organization: %v", err)
	}

	net, err := client.Network.Create().
		SetID(3001).
		SetName("Traversal Network").
		SetAsn(64000).
		SetOrganization(org).
		SetCreated(testTimestamp).
		SetUpdated(testTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating network: %v", err)
	}

	ix, err := client.InternetExchange.Create().
		SetID(3002).
		SetName("Traversal IX").
		SetOrganization(org).
		SetCreated(testTimestamp).
		SetUpdated(testTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating internet exchange: %v", err)
	}

	ixlan, err := client.IxLan.Create().
		SetID(3003).
		SetInternetExchange(ix).
		SetCreated(testTimestamp).
		SetUpdated(testTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating ixlan: %v", err)
	}

	_, err = client.NetworkIxLan.Create().
		SetID(3004).
		SetNetwork(net).
		SetIxLan(ixlan).
		SetSpeed(10000).
		SetAsn(64000).
		SetCreated(testTimestamp).
		SetUpdated(testTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating network ixlan: %v", err)
	}

	// Traverse: org -> networks
	t.Run("org to networks", func(t *testing.T) {
		networks, err := org.QueryNetworks().All(ctx)
		if err != nil {
			t.Fatalf("querying networks from org: %v", err)
		}
		if len(networks) != 1 {
			t.Fatalf("got %d networks, want 1", len(networks))
		}
		if networks[0].Asn != 64000 {
			t.Errorf("network asn = %d, want %d", networks[0].Asn, 64000)
		}
	})

	// Traverse: network -> network_ix_lans
	t.Run("network to network_ix_lans", func(t *testing.T) {
		nixlans, err := net.QueryNetworkIxLans().All(ctx)
		if err != nil {
			t.Fatalf("querying network_ix_lans from network: %v", err)
		}
		if len(nixlans) != 1 {
			t.Fatalf("got %d network_ix_lans, want 1", len(nixlans))
		}
		if nixlans[0].Speed != 10000 {
			t.Errorf("speed = %d, want %d", nixlans[0].Speed, 10000)
		}
	})

	// Traverse: org -> networks -> network_ix_lans (chained)
	t.Run("org to networks to network_ix_lans", func(t *testing.T) {
		nixlans, err := org.QueryNetworks().QueryNetworkIxLans().All(ctx)
		if err != nil {
			t.Fatalf("chained traversal: %v", err)
		}
		if len(nixlans) != 1 {
			t.Fatalf("got %d network_ix_lans from chain, want 1", len(nixlans))
		}
	})

	// Traverse: org -> internet_exchanges -> ix_lans
	t.Run("org to internet_exchanges to ix_lans", func(t *testing.T) {
		ixlans, err := org.QueryInternetExchanges().QueryIxLans().All(ctx)
		if err != nil {
			t.Fatalf("chained IX traversal: %v", err)
		}
		if len(ixlans) != 1 {
			t.Fatalf("got %d ix_lans from chain, want 1", len(ixlans))
		}
	})

	// Reverse traversal: network_ix_lan -> network -> organization
	t.Run("reverse traversal networkixlan to org", func(t *testing.T) {
		nixlans, err := client.NetworkIxLan.Query().All(ctx)
		if err != nil {
			t.Fatalf("querying all network_ix_lans: %v", err)
		}
		if len(nixlans) == 0 {
			t.Fatal("no network_ix_lans found")
		}
		netFromNixl, err := nixlans[0].QueryNetwork().Only(ctx)
		if err != nil {
			t.Fatalf("querying network from network_ix_lan: %v", err)
		}
		orgFromNet, err := netFromNixl.QueryOrganization().Only(ctx)
		if err != nil {
			t.Fatalf("querying org from network: %v", err)
		}
		if orgFromNet.Name != "Traversal Org" {
			t.Errorf("org name = %q, want %q", orgFromNet.Name, "Traversal Org")
		}
	})
}

// TestOtelMutationHook_ErrorPath verifies that the otelMutationHook records
// errors on the span when a mutation fails (hooks.go line 28). Uses an
// in-memory span exporter to capture and inspect span events.
func TestOtelMutationHook_ErrorPath(t *testing.T) {
	// Set up in-memory span exporter to capture spans.
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(prev)
	})

	client := testutil.SetupClient(t)
	ctx := context.Background()

	// Create an Organization to establish ID=1.
	_, err := client.Organization.Create().
		SetID(1).
		SetName("First").
		SetCreated(testTimestamp).
		SetUpdated(testTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	// Attempt duplicate ID -- triggers mutation error through hook.
	_, err = client.Organization.Create().
		SetID(1).
		SetName("Dupe").
		SetCreated(testTimestamp).
		SetUpdated(testTimestamp).
		Save(ctx)
	if err == nil {
		t.Fatal("expected error on duplicate ID")
	}

	// Flush the tracer provider so spans are exported.
	if err := tp.ForceFlush(ctx); err != nil {
		t.Fatalf("force flush: %v", err)
	}

	// Verify the hook recorded the error on the span.
	// The hook names spans "ent.{Type}.{Op}" where Op.String() returns "OpCreate".
	spans := exporter.GetSpans()
	var found bool
	for _, s := range spans {
		if s.Name == "ent.Organization.OpCreate" {
			for _, evt := range s.Events {
				if evt.Name == "exception" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("expected span with RecordError event for ent.Organization.OpCreate")
		for _, s := range spans {
			t.Logf("  span: %s (events: %d)", s.Name, len(s.Events))
		}
	}
}

// TestFKConstraintViolation verifies that FK constraint violations are caught
// when foreign_keys pragma is enabled. Table-driven per T-1.
func TestFKConstraintViolation(t *testing.T) {
	t.Parallel()

	client := testutil.SetupClient(t)
	ctx := context.Background()

	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "Network with non-existent org_id",
			fn: func() error {
				_, err := client.Network.Create().
					SetID(5000).
					SetName("FK Test Network").
					SetAsn(65000).
					SetOrgID(99999). // Does not exist
					SetCreated(testTimestamp).
					SetUpdated(testTimestamp).
					Save(ctx)
				return err
			},
		},
		{
			name: "IxLan with non-existent ix_id",
			fn: func() error {
				_, err := client.IxLan.Create().
					SetID(5001).
					SetIxID(99999). // Does not exist
					SetCreated(testTimestamp).
					SetUpdated(testTimestamp).
					Save(ctx)
				return err
			},
		},
		{
			name: "Poc with non-existent net_id",
			fn: func() error {
				_, err := client.Poc.Create().
					SetID(5002).
					SetNetID(99999). // Does not exist
					SetRole("Technical").
					SetCreated(testTimestamp).
					SetUpdated(testTimestamp).
					Save(ctx)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.fn()
			if err == nil {
				t.Error("expected FK constraint error, got nil")
			}
		})
	}
}

// TestSchemaEdges exercises the Edges() method on all 13 schema types,
// covering 13 previously-uncovered configuration methods.
func TestSchemaEdges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		edgesFn func() []ent.Edge
		minLen  int
	}{
		{"Organization", schema.Organization{}.Edges, 5},
		{"Network", schema.Network{}.Edges, 3},
		{"Facility", schema.Facility{}.Edges, 3},
		{"InternetExchange", schema.InternetExchange{}.Edges, 3},
		{"Campus", schema.Campus{}.Edges, 1},
		{"Carrier", schema.Carrier{}.Edges, 1},
		{"CarrierFacility", schema.CarrierFacility{}.Edges, 2},
		{"IxFacility", schema.IxFacility{}.Edges, 2},
		{"IxLan", schema.IxLan{}.Edges, 2},
		{"IxPrefix", schema.IxPrefix{}.Edges, 1},
		{"NetworkFacility", schema.NetworkFacility{}.Edges, 2},
		{"NetworkIxLan", schema.NetworkIxLan{}.Edges, 2},
		{"Poc", schema.Poc{}.Edges, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			edges := tt.edgesFn()
			if len(edges) < tt.minLen {
				t.Errorf("len(Edges()) = %d, want >= %d", len(edges), tt.minLen)
			}
		})
	}
}

// TestSchemaIndexes exercises the Indexes() method on all 13 schema types,
// covering 13 previously-uncovered configuration methods.
func TestSchemaIndexes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		indexesFn func() []ent.Index
		minLen    int
	}{
		{"Organization", schema.Organization{}.Indexes, 1},
		{"Network", schema.Network{}.Indexes, 1},
		{"Facility", schema.Facility{}.Indexes, 1},
		{"InternetExchange", schema.InternetExchange{}.Indexes, 1},
		{"Campus", schema.Campus{}.Indexes, 1},
		{"Carrier", schema.Carrier{}.Indexes, 1},
		{"CarrierFacility", schema.CarrierFacility{}.Indexes, 1},
		{"IxFacility", schema.IxFacility{}.Indexes, 1},
		{"IxLan", schema.IxLan{}.Indexes, 1},
		{"IxPrefix", schema.IxPrefix{}.Indexes, 1},
		{"NetworkFacility", schema.NetworkFacility{}.Indexes, 1},
		{"NetworkIxLan", schema.NetworkIxLan{}.Indexes, 1},
		{"Poc", schema.Poc{}.Indexes, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			indexes := tt.indexesFn()
			if len(indexes) < tt.minLen {
				t.Errorf("len(Indexes()) = %d, want >= %d", len(indexes), tt.minLen)
			}
		})
	}
}

// TestSchemaAnnotations exercises the Annotations() method on all 13 schema
// types, covering 13 previously-uncovered configuration methods.
func TestSchemaAnnotations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		annotationsFn func() []entschema.Annotation
		minLen        int
	}{
		{"Organization", schema.Organization{}.Annotations, 1},
		{"Network", schema.Network{}.Annotations, 1},
		{"Facility", schema.Facility{}.Annotations, 1},
		{"InternetExchange", schema.InternetExchange{}.Annotations, 1},
		{"Campus", schema.Campus{}.Annotations, 1},
		{"Carrier", schema.Carrier{}.Annotations, 1},
		{"CarrierFacility", schema.CarrierFacility{}.Annotations, 1},
		{"IxFacility", schema.IxFacility{}.Annotations, 1},
		{"IxLan", schema.IxLan{}.Annotations, 1},
		{"IxPrefix", schema.IxPrefix{}.Annotations, 1},
		{"NetworkFacility", schema.NetworkFacility{}.Annotations, 1},
		{"NetworkIxLan", schema.NetworkIxLan{}.Annotations, 1},
		{"Poc", schema.Poc{}.Annotations, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			annotations := tt.annotationsFn()
			if len(annotations) < tt.minLen {
				t.Errorf("len(Annotations()) = %d, want >= %d", len(annotations), tt.minLen)
			}
		})
	}
}
