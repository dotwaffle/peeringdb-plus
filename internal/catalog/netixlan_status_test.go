package catalog

import (
	"slices"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// seedNetIXLanStatuses seeds two networks at two exchanges with one ok,
// two not-operational and one deleted netixlan:
//
//   - 200: AS65001 at IX 20, ok, 10G
//   - 201: AS65002 at IX 20, not-operational, 100G, planned removal and
//     RFC 8950 in meta
//   - 202: AS65001 at IX 21, not-operational, 1G
//   - 203: AS65002 at IX 21, deleted, 400G
func seedNetIXLanStatuses(t *testing.T, c *ent.Client) {
	t.Helper()
	ctx := t.Context()
	now := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)

	org := c.Organization.Create().SetID(1).SetName("Status Org").
		SetCreated(now).SetUpdated(now).SaveX(ctx)
	nets := map[int]*ent.Network{
		65001: c.Network.Create().SetID(10).SetName("Live Net").SetAsn(65001).
			SetOrganization(org).SetCreated(now).SetUpdated(now).SaveX(ctx),
		65002: c.Network.Create().SetID(11).SetName("Dark Net").SetAsn(65002).
			SetOrganization(org).SetCreated(now).SetUpdated(now).SaveX(ctx),
	}
	lans := map[int]*ent.IxLan{}
	for id, name := range map[int]string{20: "Shared IX", 21: "Solo IX"} {
		ix := c.InternetExchange.Create().SetID(id).SetName(name).
			SetOrganization(org).SetCreated(now).SetUpdated(now).SaveX(ctx)
		lans[id] = c.IxLan.Create().SetID(id * 5).SetInternetExchange(ix).
			SetCreated(now).SetUpdated(now).SaveX(ctx)
	}

	plan := map[string]any{
		"planned_status_change": map[string]any{"status": "deleted", "date": "2026-12-31"},
		"rfc8950":               true,
	}
	for _, row := range []struct {
		id, asn, ixID, speed int
		status               string
		meta                 map[string]any
	}{
		{200, 65001, 20, 10_000, "ok", map[string]any{}},
		{201, 65002, 20, 100_000, "not-operational", plan},
		{202, 65001, 21, 1_000, "not-operational", nil},
		{203, 65002, 21, 400_000, "deleted", nil},
	} {
		c.NetworkIxLan.Create().SetID(row.id).
			SetNetwork(nets[row.asn]).SetIxLan(lans[row.ixID]).SetIxID(row.ixID).
			SetAsn(row.asn).SetSpeed(row.speed).
			SetStatus(row.status).SetOperational(row.status == "ok").
			SetMeta(row.meta).
			SetCreated(now).SetUpdated(now).SaveX(ctx)
	}
}

// TestCatalog_ListsNotOperationalConnections locks that the network, IX
// and compare queries treat a not-operational netixlan as live. PeeringDB
// 2.83.0 lists these connections in its network and exchange views
// (docs/api/obj_netixlan.md:50-54) and counts their speed in its IX
// statistics (stats.py:105-134). Deleted rows stay out.
func TestCatalog_ListsNotOperationalConnections(t *testing.T) {
	t.Parallel()
	c := testutil.SetupClient(t)
	seedNetIXLanStatuses(t, c)
	ctx := t.Context()

	t.Run("network IX presences", func(t *testing.T) {
		t.Parallel()
		net, err := NewService(c).Network(ctx, 65001)
		if err != nil {
			t.Fatalf("Network: %v", err)
		}
		var ixIDs []int
		for _, row := range net.IXPresences {
			ixIDs = append(ixIDs, row.IXID)
		}
		slices.Sort(ixIDs)
		if !slices.Equal(ixIDs, []int{20, 21}) {
			t.Errorf("IX presence IDs = %v, want [20 21]", ixIDs)
		}
		if net.AggregateBW != 11_000 {
			t.Errorf("AggregateBW = %d, want 11000", net.AggregateBW)
		}
	})

	t.Run("IX participants", func(t *testing.T) {
		t.Parallel()
		ix, err := NewService(c).IX(ctx, 20)
		if err != nil {
			t.Fatalf("IX: %v", err)
		}
		var asns []int
		for _, row := range ix.Participants {
			asns = append(asns, row.ASN)
		}
		if !slices.Equal(asns, []int{65001, 65002}) {
			t.Errorf("participant ASNs = %v, want [65001 65002]", asns)
		}
		if ix.AggregateBW != 110_000 {
			t.Errorf("AggregateBW = %d, want 110000", ix.AggregateBW)
		}
	})

	t.Run("compare shared IXPs", func(t *testing.T) {
		t.Parallel()
		data, err := NewCompareService(c).Compare(ctx, CompareInput{ASN1: 65001, ASN2: 65002})
		if err != nil {
			t.Fatalf("Compare: %v", err)
		}
		if len(data.SharedIXPs) != 1 || data.SharedIXPs[0].IXID != 20 {
			t.Fatalf("SharedIXPs = %+v, want only IX 20", data.SharedIXPs)
		}
		if netB := data.SharedIXPs[0].NetB; netB == nil || netB.Speed != 100_000 {
			t.Errorf("SharedIXPs[0].NetB = %+v, want the 100G not-operational presence", netB)
		}
	})

	t.Run("compare with the not-operational side as A", func(t *testing.T) {
		t.Parallel()
		// Swap the sides so that the A-side query must admit the
		// not-operational presence of AS65002 at IX 20.
		data, err := NewCompareService(c).Compare(ctx, CompareInput{ASN1: 65002, ASN2: 65001})
		if err != nil {
			t.Fatalf("Compare: %v", err)
		}
		if len(data.SharedIXPs) != 1 || data.SharedIXPs[0].IXID != 20 {
			t.Fatalf("SharedIXPs = %+v, want only IX 20", data.SharedIXPs)
		}
		if netA := data.SharedIXPs[0].NetA; netA == nil || netA.Speed != 100_000 {
			t.Errorf("SharedIXPs[0].NetA = %+v, want the 100G not-operational presence", netA)
		}
	})
}

// TestCatalog_ConnectionMarkers locks that the network, IX and compare
// rows carry the markers of each connection, and that an ok connection
// without meta keys carries none.
func TestCatalog_ConnectionMarkers(t *testing.T) {
	t.Parallel()
	c := testutil.SetupClient(t)
	seedNetIXLanStatuses(t, c)
	ctx := t.Context()

	marked := ConnectionMarkers{
		NotOperational: true, PlannedStatus: "deleted", PlannedDate: "2026-12-31", RFC8950: true,
	}

	net, err := NewService(c).Network(ctx, 65002)
	if err != nil {
		t.Fatalf("Network: %v", err)
	}
	if len(net.IXPresences) != 1 || net.IXPresences[0].Markers != marked {
		t.Errorf("IXPresences = %+v, want one row with markers %+v", net.IXPresences, marked)
	}

	ix, err := NewService(c).IX(ctx, 20)
	if err != nil {
		t.Fatalf("IX: %v", err)
	}
	got := map[int]ConnectionMarkers{}
	for _, row := range ix.Participants {
		got[row.ASN] = row.Markers
	}
	if got[65001] != (ConnectionMarkers{}) || got[65002] != marked {
		t.Errorf("participant markers = %+v, want none for AS65001 and %+v for AS65002", got, marked)
	}

	data, err := NewCompareService(c).Compare(ctx, CompareInput{ASN1: 65001, ASN2: 65002})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if len(data.SharedIXPs) != 1 {
		t.Fatalf("SharedIXPs = %+v, want one", data.SharedIXPs)
	}
	shared := data.SharedIXPs[0]
	if shared.NetA.Markers != (ConnectionMarkers{}) || shared.NetB.Markers != marked {
		t.Errorf("compare markers = %+v / %+v, want none / %+v", shared.NetA.Markers, shared.NetB.Markers, marked)
	}
}
