// Package seed provides deterministic test data seeding for PeeringDB entity types.
// It creates well-known entities with fixed IDs so that any test package can
// populate a database with realistic data via a single function call.
package seed

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
)

// Timestamp is the deterministic timestamp used for all seed entity
// created/updated fields.
var Timestamp = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// Result holds typed references to all entities created by seed functions.
type Result struct {
	Org             *ent.Organization
	Network         *ent.Network
	Network2        *ent.Network          // second network, only in Full
	IX              *ent.InternetExchange
	Facility        *ent.Facility
	Facility2       *ent.Facility         // campus-assigned facility, only in Full
	Campus          *ent.Campus
	Carrier         *ent.Carrier
	IxLan           *ent.IxLan
	IxPrefix        *ent.IxPrefix
	NetworkIxLan    *ent.NetworkIxLan
	NetworkFacility *ent.NetworkFacility
	IxFacility      *ent.IxFacility
	CarrierFacility *ent.CarrierFacility
	Poc             *ent.Poc
	AllNetworks     []*ent.Network // all created networks (for Networks())
}

// Full creates one entity of each of the 13 PeeringDB types plus a second
// Network and a campus-assigned Facility. It uses deterministic IDs matching
// the legacy seedAllTestData pattern for backward compatibility.
func Full(tb testing.TB, client *ent.Client) *Result {
	tb.Helper()
	ctx := context.Background()
	r := &Result{}

	var err error

	// Organization (root entity, no FK dependencies).
	r.Org, err = client.Organization.Create().
		SetID(1).SetName("Test Organization").
		SetCity("Frankfurt").SetCountry("DE").
		SetCreated(Timestamp).SetUpdated(Timestamp).
		Save(ctx)
	if err != nil {
		tb.Fatalf("seed: create Organization: %v", err)
	}

	// Campus (depends on Org).
	r.Campus, err = client.Campus.Create().
		SetID(40).SetName("Test Campus").
		SetOrgID(r.Org.ID).SetOrganization(r.Org).
		SetCity("Berlin").SetCountry("DE").
		SetCreated(Timestamp).SetUpdated(Timestamp).
		Save(ctx)
	if err != nil {
		tb.Fatalf("seed: create Campus: %v", err)
	}

	// Carrier (depends on Org).
	r.Carrier, err = client.Carrier.Create().
		SetID(50).SetName("Test Carrier").
		SetOrgID(r.Org.ID).SetOrganization(r.Org).
		SetFacCount(1).
		SetCreated(Timestamp).SetUpdated(Timestamp).
		Save(ctx)
	if err != nil {
		tb.Fatalf("seed: create Carrier: %v", err)
	}

	// Facility (depends on Org).
	r.Facility, err = client.Facility.Create().
		SetID(30).SetName("Equinix FR5").
		SetOrgID(r.Org.ID).SetOrganization(r.Org).
		SetCity("Frankfurt").SetCountry("DE").
		SetNetCount(1).SetIxCount(1).SetCarrierCount(1).
		SetCreated(Timestamp).SetUpdated(Timestamp).
		Save(ctx)
	if err != nil {
		tb.Fatalf("seed: create Facility: %v", err)
	}

	// Facility2 (campus-assigned facility, depends on Org + Campus).
	r.Facility2, err = client.Facility.Create().
		SetID(31).SetName("Campus Facility").
		SetOrgID(r.Org.ID).SetOrganization(r.Org).
		SetCampusID(r.Campus.ID).SetCampus(r.Campus).
		SetCity("Berlin").SetCountry("DE").
		SetCreated(Timestamp).SetUpdated(Timestamp).
		Save(ctx)
	if err != nil {
		tb.Fatalf("seed: create Facility2: %v", err)
	}

	// InternetExchange (depends on Org).
	r.IX, err = client.InternetExchange.Create().
		SetID(20).SetName("DE-CIX Frankfurt").
		SetOrgID(r.Org.ID).SetOrganization(r.Org).
		SetCity("Frankfurt").SetCountry("DE").
		SetRegionContinent("Europe").SetMedia("Ethernet").
		SetNetCount(1).SetFacCount(1).
		SetCreated(Timestamp).SetUpdated(Timestamp).
		Save(ctx)
	if err != nil {
		tb.Fatalf("seed: create InternetExchange: %v", err)
	}

	// IxLan (depends on IX).
	r.IxLan, err = client.IxLan.Create().
		SetID(100).
		SetIxID(r.IX.ID).SetInternetExchange(r.IX).
		SetCreated(Timestamp).SetUpdated(Timestamp).
		Save(ctx)
	if err != nil {
		tb.Fatalf("seed: create IxLan: %v", err)
	}

	// Network (depends on Org).
	r.Network, err = client.Network.Create().
		SetID(10).SetName("Cloudflare").SetAsn(13335).
		SetOrgID(r.Org.ID).SetOrganization(r.Org).
		SetIxCount(1).SetFacCount(1).
		SetCreated(Timestamp).SetUpdated(Timestamp).
		Save(ctx)
	if err != nil {
		tb.Fatalf("seed: create Network: %v", err)
	}

	// Network2 (depends on Org).
	r.Network2, err = client.Network.Create().
		SetID(11).SetName("Hurricane Electric").SetAsn(6939).
		SetOrgID(r.Org.ID).SetOrganization(r.Org).
		SetCreated(Timestamp).SetUpdated(Timestamp).
		Save(ctx)
	if err != nil {
		tb.Fatalf("seed: create Network2: %v", err)
	}

	r.AllNetworks = []*ent.Network{r.Network, r.Network2}

	// IxPrefix (depends on IxLan).
	r.IxPrefix, err = client.IxPrefix.Create().
		SetID(700).
		SetIxlanID(r.IxLan.ID).SetIxLan(r.IxLan).
		SetPrefix("80.81.192.0/22").SetProtocol("IPv4").SetInDfz(true).
		SetCreated(Timestamp).SetUpdated(Timestamp).
		Save(ctx)
	if err != nil {
		tb.Fatalf("seed: create IxPrefix: %v", err)
	}

	// NetworkIxLan (depends on Network + IxLan).
	r.NetworkIxLan, err = client.NetworkIxLan.Create().
		SetID(200).
		SetNetID(r.Network.ID).SetNetwork(r.Network).
		SetIxlanID(r.IxLan.ID).SetIxLan(r.IxLan).
		SetAsn(13335).SetSpeed(10000).
		SetName("DE-CIX Frankfurt").SetIxID(r.IX.ID).
		SetCreated(Timestamp).SetUpdated(Timestamp).
		Save(ctx)
	if err != nil {
		tb.Fatalf("seed: create NetworkIxLan: %v", err)
	}

	// NetworkFacility (depends on Network + Facility).
	r.NetworkFacility, err = client.NetworkFacility.Create().
		SetID(300).
		SetNetID(r.Network.ID).SetNetwork(r.Network).
		SetFacID(r.Facility.ID).SetFacility(r.Facility).
		SetLocalAsn(13335).
		SetName("Equinix FR5").
		SetCreated(Timestamp).SetUpdated(Timestamp).
		Save(ctx)
	if err != nil {
		tb.Fatalf("seed: create NetworkFacility: %v", err)
	}

	// IxFacility (depends on IX + Facility).
	r.IxFacility, err = client.IxFacility.Create().
		SetID(400).
		SetFacID(r.Facility.ID).SetFacility(r.Facility).
		SetIxID(r.IX.ID).SetInternetExchange(r.IX).
		SetName("DE-CIX Frankfurt").
		SetCity("Frankfurt").SetCountry("DE").
		SetCreated(Timestamp).SetUpdated(Timestamp).
		Save(ctx)
	if err != nil {
		tb.Fatalf("seed: create IxFacility: %v", err)
	}

	// CarrierFacility (depends on Carrier + Facility).
	r.CarrierFacility, err = client.CarrierFacility.Create().
		SetID(600).
		SetCarrierID(r.Carrier.ID).SetCarrier(r.Carrier).
		SetFacID(r.Facility.ID).SetFacility(r.Facility).
		SetName("Equinix FR5").
		SetCreated(Timestamp).SetUpdated(Timestamp).
		Save(ctx)
	if err != nil {
		tb.Fatalf("seed: create CarrierFacility: %v", err)
	}

	// Poc (depends on Network).
	r.Poc, err = client.Poc.Create().
		SetID(500).
		SetNetID(r.Network.ID).SetNetwork(r.Network).
		SetName("NOC Contact").SetRole("NOC").
		SetCreated(Timestamp).SetUpdated(Timestamp).
		Save(ctx)
	if err != nil {
		tb.Fatalf("seed: create Poc: %v", err)
	}

	return r
}

// minimal creates only the 4 core entity types needed for basic relationship
// traversal: Organization, Network, InternetExchange, and Facility.
// Junction types are not created; their Result fields remain nil.
func minimal(tb testing.TB, client *ent.Client) *Result {
	tb.Helper()
	ctx := context.Background()
	r := &Result{}

	var err error

	r.Org, err = client.Organization.Create().
		SetID(1).SetName("Test Organization").
		SetCity("Frankfurt").SetCountry("DE").
		SetCreated(Timestamp).SetUpdated(Timestamp).
		Save(ctx)
	if err != nil {
		tb.Fatalf("seed: create Organization: %v", err)
	}

	r.IX, err = client.InternetExchange.Create().
		SetID(20).SetName("DE-CIX Frankfurt").
		SetOrgID(r.Org.ID).SetOrganization(r.Org).
		SetCity("Frankfurt").SetCountry("DE").
		SetRegionContinent("Europe").SetMedia("Ethernet").
		SetCreated(Timestamp).SetUpdated(Timestamp).
		Save(ctx)
	if err != nil {
		tb.Fatalf("seed: create InternetExchange: %v", err)
	}

	r.Facility, err = client.Facility.Create().
		SetID(30).SetName("Equinix FR5").
		SetOrgID(r.Org.ID).SetOrganization(r.Org).
		SetCity("Frankfurt").SetCountry("DE").
		SetCreated(Timestamp).SetUpdated(Timestamp).
		Save(ctx)
	if err != nil {
		tb.Fatalf("seed: create Facility: %v", err)
	}

	r.Network, err = client.Network.Create().
		SetID(10).SetName("Cloudflare").SetAsn(13335).
		SetOrgID(r.Org.ID).SetOrganization(r.Org).
		SetCreated(Timestamp).SetUpdated(Timestamp).
		Save(ctx)
	if err != nil {
		tb.Fatalf("seed: create Network: %v", err)
	}

	r.AllNetworks = []*ent.Network{r.Network}

	return r
}

// networks creates one Organization and n Networks, each with a unique ASN
// starting at 65001. Result.Network is set to the first network created.
func networks(tb testing.TB, client *ent.Client, n int) *Result {
	tb.Helper()
	ctx := context.Background()
	r := &Result{}

	var err error

	r.Org, err = client.Organization.Create().
		SetID(1).SetName("Test Organization").
		SetCity("Frankfurt").SetCountry("DE").
		SetCreated(Timestamp).SetUpdated(Timestamp).
		Save(ctx)
	if err != nil {
		tb.Fatalf("seed: create Organization: %v", err)
	}

	r.AllNetworks = make([]*ent.Network, n)
	for i := range n {
		net, nerr := client.Network.Create().
			SetID(10 + i).
			SetName(fmt.Sprintf("Network %d", i+1)).
			SetAsn(65001 + i).
			SetOrgID(r.Org.ID).SetOrganization(r.Org).
			SetCreated(Timestamp).SetUpdated(Timestamp).
			Save(ctx)
		if nerr != nil {
			tb.Fatalf("seed: create Network %d: %v", i+1, nerr)
		}
		r.AllNetworks[i] = net
	}
	r.Network = r.AllNetworks[0]

	return r
}
