package web

import (
	"context"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// seedCompareTestData creates two networks with overlapping and unique presences
// for comparison tests. Returns the ent client for direct assertions.
//
// Entities created:
//   - Org (ID=1, Name="TestOrg")
//   - Network A (ID=10, ASN=13335, Name="Cloudflare")
//   - Network B (ID=11, ASN=15169, Name="Google")
//   - IX "DE-CIX Frankfurt" (ID=20) with IxLan (ID=100) -- both networks present
//   - IX "AMS-IX" (ID=21) with IxLan (ID=101) -- only network A present
//   - Facility "Equinix FR5" (ID=30) in Frankfurt, DE -- both networks present
//   - Facility "Equinix AM5" (ID=31) in Amsterdam, NL -- only network B present
//   - Campus "Frankfurt Campus" (ID=40) owning facility 30
//   - NetworkIxLan for A at DE-CIX (ID=200, speed=10000, ipaddr4="80.81.193.100", is_rs_peer=true)
//   - NetworkIxLan for B at DE-CIX (ID=201, speed=100000, ipaddr4="80.81.193.200", ipaddr6="2001:7f8::3b41:0:1")
//   - NetworkIxLan for A at AMS-IX (ID=202, speed=100000)
//   - NetworkFacility for A at Equinix FR5 (ID=300, local_asn=13335)
//   - NetworkFacility for B at Equinix FR5 (ID=301, local_asn=15169)
//   - NetworkFacility for B at Equinix AM5 (ID=302, local_asn=15169)
func seedCompareTestData(t *testing.T, client *ent.Client) {
	t.Helper()
	ctx := context.Background()

	org, err := client.Organization.Create().
		SetID(1).SetName("TestOrg").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating organization: %v", err)
	}

	netA, err := client.Network.Create().
		SetID(10).SetName("Cloudflare").SetAsn(13335).
		SetOrgID(1).SetOrganization(org).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating network A: %v", err)
	}

	netB, err := client.Network.Create().
		SetID(11).SetName("Google").SetAsn(15169).
		SetOrgID(1).SetOrganization(org).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating network B: %v", err)
	}

	// IX "DE-CIX Frankfurt" -- both networks present.
	ix1, err := client.InternetExchange.Create().
		SetID(20).SetName("DE-CIX Frankfurt").
		SetOrgID(1).SetOrganization(org).
		SetCity("Frankfurt").SetCountry("DE").
		SetRegionContinent("Europe").SetMedia("Ethernet").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating ix DE-CIX: %v", err)
	}

	ixlan1, err := client.IxLan.Create().
		SetID(100).SetIxID(ix1.ID).SetInternetExchange(ix1).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating ixlan for DE-CIX: %v", err)
	}

	// IX "AMS-IX" -- only network A present.
	ix2, err := client.InternetExchange.Create().
		SetID(21).SetName("AMS-IX").
		SetOrgID(1).SetOrganization(org).
		SetCity("Amsterdam").SetCountry("NL").
		SetRegionContinent("Europe").SetMedia("Ethernet").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating ix AMS-IX: %v", err)
	}

	ixlan2, err := client.IxLan.Create().
		SetID(101).SetIxID(ix2.ID).SetInternetExchange(ix2).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating ixlan for AMS-IX: %v", err)
	}

	// Facility "Equinix FR5" -- both networks present, part of campus.
	fac1, err := client.Facility.Create().
		SetID(30).SetName("Equinix FR5").
		SetOrgID(1).SetOrganization(org).
		SetCity("Frankfurt").SetCountry("DE").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating facility Equinix FR5: %v", err)
	}

	// Facility "Equinix AM5" -- only network B present.
	fac2, err := client.Facility.Create().
		SetID(31).SetName("Equinix AM5").
		SetOrgID(1).SetOrganization(org).
		SetCity("Amsterdam").SetCountry("NL").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating facility Equinix AM5: %v", err)
	}

	// Campus "Frankfurt Campus" owning Equinix FR5.
	campusEntity, err := client.Campus.Create().
		SetID(40).SetName("Frankfurt Campus").
		SetOrgID(1).SetOrganization(org).
		SetCity("Frankfurt").SetCountry("DE").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating campus: %v", err)
	}

	// Assign facility 30 to campus 40.
	_, err = fac1.Update().SetCampusID(campusEntity.ID).SetCampus(campusEntity).Save(ctx)
	if err != nil {
		t.Fatalf("assigning facility to campus: %v", err)
	}

	// NetworkIxLan for A at DE-CIX.
	ipv4A := "80.81.193.100"
	_, err = client.NetworkIxLan.Create().
		SetID(200).
		SetNetID(netA.ID).SetNetwork(netA).
		SetIxlanID(100).SetIxLan(ixlan1).
		SetAsn(13335).SetSpeed(10000).
		SetNillableIpaddr4(&ipv4A).
		SetIsRsPeer(true).SetOperational(true).
		SetName("DE-CIX Frankfurt").SetIxID(20).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating networkixlan A at DE-CIX: %v", err)
	}

	// NetworkIxLan for B at DE-CIX.
	ipv4B := "80.81.193.200"
	ipv6B := "2001:7f8::3b41:0:1"
	_, err = client.NetworkIxLan.Create().
		SetID(201).
		SetNetID(netB.ID).SetNetwork(netB).
		SetIxlanID(100).SetIxLan(ixlan1).
		SetAsn(15169).SetSpeed(100000).
		SetNillableIpaddr4(&ipv4B).
		SetNillableIpaddr6(&ipv6B).
		SetOperational(true).
		SetName("DE-CIX Frankfurt").SetIxID(20).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating networkixlan B at DE-CIX: %v", err)
	}

	// NetworkIxLan for A at AMS-IX (only A).
	_, err = client.NetworkIxLan.Create().
		SetID(202).
		SetNetID(netA.ID).SetNetwork(netA).
		SetIxlanID(101).SetIxLan(ixlan2).
		SetAsn(13335).SetSpeed(100000).
		SetOperational(true).
		SetName("AMS-IX").SetIxID(21).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating networkixlan A at AMS-IX: %v", err)
	}

	// NetworkFacility for A at Equinix FR5.
	_, err = client.NetworkFacility.Create().
		SetID(300).
		SetNetID(netA.ID).SetNetwork(netA).
		SetFacID(fac1.ID).SetFacility(fac1).
		SetLocalAsn(13335).SetName("Equinix FR5").
		SetCity("Frankfurt").SetCountry("DE").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating networkfacility A at FR5: %v", err)
	}

	// NetworkFacility for B at Equinix FR5.
	_, err = client.NetworkFacility.Create().
		SetID(301).
		SetNetID(netB.ID).SetNetwork(netB).
		SetFacID(fac1.ID).SetFacility(fac1).
		SetLocalAsn(15169).SetName("Equinix FR5").
		SetCity("Frankfurt").SetCountry("DE").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating networkfacility B at FR5: %v", err)
	}

	// NetworkFacility for B at Equinix AM5 (only B).
	_, err = client.NetworkFacility.Create().
		SetID(302).
		SetNetID(netB.ID).SetNetwork(netB).
		SetFacID(fac2.ID).SetFacility(fac2).
		SetLocalAsn(15169).SetName("Equinix AM5").
		SetCity("Amsterdam").SetCountry("NL").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating networkfacility B at AM5: %v", err)
	}
}

func TestCompareService_SharedIXPs(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	seedCompareTestData(t, client)

	svc := NewCompareService(client)
	data, err := svc.Compare(context.Background(), CompareInput{
		ASN1:     13335,
		ASN2:     15169,
		ViewMode: "shared",
	})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}

	if len(data.SharedIXPs) != 1 {
		t.Fatalf("SharedIXPs length = %d, want 1", len(data.SharedIXPs))
	}

	ix := data.SharedIXPs[0]
	if ix.IXID != 20 {
		t.Errorf("IXID = %d, want 20", ix.IXID)
	}
	if ix.IXName != "DE-CIX Frankfurt" {
		t.Errorf("IXName = %q, want %q", ix.IXName, "DE-CIX Frankfurt")
	}
	if !ix.Shared {
		t.Error("Shared should be true")
	}
	if ix.NetA == nil {
		t.Fatal("NetA should not be nil")
	}
	if ix.NetA.Speed != 10000 {
		t.Errorf("NetA.Speed = %d, want 10000", ix.NetA.Speed)
	}
	if ix.NetA.IPAddr4 != "80.81.193.100" {
		t.Errorf("NetA.IPAddr4 = %q, want %q", ix.NetA.IPAddr4, "80.81.193.100")
	}
	if !ix.NetA.IsRSPeer {
		t.Error("NetA.IsRSPeer should be true")
	}
	if ix.NetB == nil {
		t.Fatal("NetB should not be nil")
	}
	if ix.NetB.Speed != 100000 {
		t.Errorf("NetB.Speed = %d, want 100000", ix.NetB.Speed)
	}
	if ix.NetB.IPAddr4 != "80.81.193.200" {
		t.Errorf("NetB.IPAddr4 = %q, want %q", ix.NetB.IPAddr4, "80.81.193.200")
	}
	if ix.NetB.IPAddr6 != "2001:7f8::3b41:0:1" {
		t.Errorf("NetB.IPAddr6 = %q, want %q", ix.NetB.IPAddr6, "2001:7f8::3b41:0:1")
	}
}

func TestCompareService_SharedFacilities(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	seedCompareTestData(t, client)

	svc := NewCompareService(client)
	data, err := svc.Compare(context.Background(), CompareInput{
		ASN1:     13335,
		ASN2:     15169,
		ViewMode: "shared",
	})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}

	if len(data.SharedFacilities) != 1 {
		t.Fatalf("SharedFacilities length = %d, want 1", len(data.SharedFacilities))
	}

	fac := data.SharedFacilities[0]
	if fac.FacID != 30 {
		t.Errorf("FacID = %d, want 30", fac.FacID)
	}
	if fac.FacName != "Equinix FR5" {
		t.Errorf("FacName = %q, want %q", fac.FacName, "Equinix FR5")
	}
	if !fac.Shared {
		t.Error("Shared should be true")
	}
	if fac.NetA == nil {
		t.Fatal("NetA should not be nil")
	}
	if fac.NetA.LocalASN != 13335 {
		t.Errorf("NetA.LocalASN = %d, want 13335", fac.NetA.LocalASN)
	}
	if fac.NetB == nil {
		t.Fatal("NetB should not be nil")
	}
	if fac.NetB.LocalASN != 15169 {
		t.Errorf("NetB.LocalASN = %d, want 15169", fac.NetB.LocalASN)
	}
}

func TestCompareService_SharedCampuses(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	seedCompareTestData(t, client)

	svc := NewCompareService(client)
	data, err := svc.Compare(context.Background(), CompareInput{
		ASN1:     13335,
		ASN2:     15169,
		ViewMode: "shared",
	})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}

	if len(data.SharedCampuses) != 1 {
		t.Fatalf("SharedCampuses length = %d, want 1", len(data.SharedCampuses))
	}

	camp := data.SharedCampuses[0]
	if camp.CampusID != 40 {
		t.Errorf("CampusID = %d, want 40", camp.CampusID)
	}
	if camp.CampusName != "Frankfurt Campus" {
		t.Errorf("CampusName = %q, want %q", camp.CampusName, "Frankfurt Campus")
	}
	if len(camp.SharedFacilities) != 1 {
		t.Fatalf("SharedFacilities length = %d, want 1", len(camp.SharedFacilities))
	}
	if camp.SharedFacilities[0].FacID != 30 {
		t.Errorf("SharedFacilities[0].FacID = %d, want 30", camp.SharedFacilities[0].FacID)
	}
}

func TestCompareService_NoOverlap(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := context.Background()

	org, err := client.Organization.Create().
		SetID(1).SetName("TestOrg").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating organization: %v", err)
	}

	netC, err := client.Network.Create().
		SetID(100).SetName("NetC").SetAsn(64496).
		SetOrgID(1).SetOrganization(org).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating network C: %v", err)
	}

	netD, err := client.Network.Create().
		SetID(101).SetName("NetD").SetAsn(64497).
		SetOrgID(1).SetOrganization(org).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating network D: %v", err)
	}

	// IX only for C.
	ixC, err := client.InternetExchange.Create().
		SetID(50).SetName("IX-C").
		SetOrgID(1).SetOrganization(org).
		SetRegionContinent("Europe").SetMedia("Ethernet").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating ix C: %v", err)
	}

	ixlanC, err := client.IxLan.Create().
		SetID(500).SetIxID(ixC.ID).SetInternetExchange(ixC).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating ixlan C: %v", err)
	}

	_, err = client.NetworkIxLan.Create().
		SetID(5000).
		SetNetID(netC.ID).SetNetwork(netC).
		SetIxlanID(ixlanC.ID).SetIxLan(ixlanC).
		SetAsn(64496).SetSpeed(1000).
		SetName("IX-C").SetIxID(50).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating networkixlan C: %v", err)
	}

	// Facility only for D.
	facD, err := client.Facility.Create().
		SetID(60).SetName("Fac-D").
		SetOrgID(1).SetOrganization(org).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating facility D: %v", err)
	}

	_, err = client.NetworkFacility.Create().
		SetID(6000).
		SetNetID(netD.ID).SetNetwork(netD).
		SetFacID(facD.ID).SetFacility(facD).
		SetLocalAsn(64497).SetName("Fac-D").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating networkfacility D: %v", err)
	}

	svc := NewCompareService(client)
	data, err := svc.Compare(ctx, CompareInput{
		ASN1:     64496,
		ASN2:     64497,
		ViewMode: "shared",
	})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}

	if len(data.SharedIXPs) != 0 {
		t.Errorf("SharedIXPs length = %d, want 0", len(data.SharedIXPs))
	}
	if len(data.SharedFacilities) != 0 {
		t.Errorf("SharedFacilities length = %d, want 0", len(data.SharedFacilities))
	}
	if len(data.SharedCampuses) != 0 {
		t.Errorf("SharedCampuses length = %d, want 0", len(data.SharedCampuses))
	}
}

func TestCompareService_FullViewIXPs(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	seedCompareTestData(t, client)

	svc := NewCompareService(client)
	data, err := svc.Compare(context.Background(), CompareInput{
		ASN1:     13335,
		ASN2:     15169,
		ViewMode: "full",
	})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}

	if len(data.AllIXPs) != 2 {
		t.Fatalf("AllIXPs length = %d, want 2", len(data.AllIXPs))
	}

	// AllIXPs sorted by name: AMS-IX, DE-CIX Frankfurt.
	amsIX := data.AllIXPs[0]
	if amsIX.IXName != "AMS-IX" {
		t.Errorf("AllIXPs[0].IXName = %q, want %q", amsIX.IXName, "AMS-IX")
	}
	if amsIX.Shared {
		t.Error("AMS-IX should not be shared")
	}
	if amsIX.NetA == nil {
		t.Error("AMS-IX should have NetA (Cloudflare)")
	}
	if amsIX.NetB != nil {
		t.Error("AMS-IX should not have NetB")
	}

	decix := data.AllIXPs[1]
	if decix.IXName != "DE-CIX Frankfurt" {
		t.Errorf("AllIXPs[1].IXName = %q, want %q", decix.IXName, "DE-CIX Frankfurt")
	}
	if !decix.Shared {
		t.Error("DE-CIX Frankfurt should be shared")
	}
	if decix.NetA == nil || decix.NetB == nil {
		t.Error("DE-CIX Frankfurt should have both NetA and NetB")
	}
}

func TestCompareService_FullViewFacilities(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	seedCompareTestData(t, client)

	svc := NewCompareService(client)
	data, err := svc.Compare(context.Background(), CompareInput{
		ASN1:     13335,
		ASN2:     15169,
		ViewMode: "full",
	})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}

	if len(data.AllFacilities) != 2 {
		t.Fatalf("AllFacilities length = %d, want 2", len(data.AllFacilities))
	}

	// AllFacilities sorted by name: Equinix AM5, Equinix FR5.
	am5 := data.AllFacilities[0]
	if am5.FacName != "Equinix AM5" {
		t.Errorf("AllFacilities[0].FacName = %q, want %q", am5.FacName, "Equinix AM5")
	}
	if am5.Shared {
		t.Error("Equinix AM5 should not be shared")
	}
	if am5.NetA != nil {
		t.Error("Equinix AM5 should not have NetA")
	}
	if am5.NetB == nil {
		t.Error("Equinix AM5 should have NetB (Google)")
	}

	fr5 := data.AllFacilities[1]
	if fr5.FacName != "Equinix FR5" {
		t.Errorf("AllFacilities[1].FacName = %q, want %q", fr5.FacName, "Equinix FR5")
	}
	if !fr5.Shared {
		t.Error("Equinix FR5 should be shared")
	}
	if fr5.NetA == nil || fr5.NetB == nil {
		t.Error("Equinix FR5 should have both NetA and NetB")
	}
}

func TestCompareService_InvalidASN(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)

	svc := NewCompareService(client)
	_, err := svc.Compare(context.Background(), CompareInput{
		ASN1:     99999,
		ASN2:     99998,
		ViewMode: "shared",
	})
	if err == nil {
		t.Fatal("expected error for non-existent ASN, got nil")
	}
}
