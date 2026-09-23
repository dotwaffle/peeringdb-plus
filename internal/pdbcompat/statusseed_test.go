package pdbcompat

import (
	"fmt"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// statusSeedT0 is the fixed timestamp for all rows seeded by the
// per-entity helpers below. A constant epoch keeps `?since=1` windows
// and default ordering deterministic.
var statusSeedT0 = time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)

// seedStatusParentsFor creates the (status=ok) FK ancestors needed by tag,
// using fixed IDs distinct from the child rows' 9xx band. The entity under
// test is never its own parent, so the parents (a different type) never
// pollute the type's own list/detail endpoint.
//
// Shared by TestStatusMatrix_AllEntities (list matrix) and the depth PK
// status-matrix breadth test, so both seed an identical FK graph.
func seedStatusParentsFor(tb testing.TB, c *ent.Client, tag string) {
	tb.Helper()
	ctx := tb.Context()
	t0 := statusSeedT0
	org := func() {
		c.Organization.Create().SetID(1).SetName("ParentOrg").SetStatus("ok").
			SetCreated(t0).SetUpdated(t0).SaveX(ctx)
	}
	net := func() {
		c.Network.Create().SetID(10).SetName("ParentNet").SetAsn(13335).SetOrgID(1).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
	}
	ix := func() {
		c.InternetExchange.Create().SetID(20).SetName("ParentIX").SetOrgID(1).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
	}
	fac := func() {
		c.Facility.Create().SetID(30).SetName("ParentFac").SetOrgID(1).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
	}
	carrier := func() {
		c.Carrier.Create().SetID(50).SetName("ParentCarrier").SetOrgID(1).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
	}
	ixlan := func() {
		c.IxLan.Create().SetID(70).SetIxID(20).SetStatus("ok").
			SetCreated(t0).SetUpdated(t0).SaveX(ctx)
	}
	switch tag {
	case peeringdb.TypeOrg:
		// root entity, no FK parents
	case peeringdb.TypeNet, peeringdb.TypeFac, peeringdb.TypeIX,
		peeringdb.TypeCarrier, peeringdb.TypeCampus:
		org()
	case peeringdb.TypePoc:
		org()
		net()
	case peeringdb.TypeIXLan:
		org()
		ix()
	case peeringdb.TypeIXPfx:
		org()
		ix()
		ixlan()
	case peeringdb.TypeNetIXLan:
		org()
		net()
		ix()
		ixlan()
	case peeringdb.TypeNetFac:
		org()
		net()
		fac()
	case peeringdb.TypeIXFac:
		org()
		ix()
		fac()
	case peeringdb.TypeCarrierFac:
		org()
		carrier()
		fac()
	default:
		tb.Fatalf("seedStatusParentsFor: unknown tag %q", tag)
	}
}

// seedStatusRow creates one row of tag with the given id and status,
// mirroring the known-good required-field setters from
// internal/testutil/seed. net asn and ixpfx prefix are derived from id
// to stay unique across multiple rows of the same type.
func seedStatusRow(tb testing.TB, c *ent.Client, tag string, id int, status string) {
	tb.Helper()
	ctx := tb.Context()
	t0 := statusSeedT0
	var err error
	switch tag {
	case peeringdb.TypeOrg:
		_, err = c.Organization.Create().SetID(id).SetName("Org").SetStatus(status).
			SetCreated(t0).SetUpdated(t0).Save(ctx)
	case peeringdb.TypeNet:
		_, err = c.Network.Create().SetID(id).SetName("Net").SetAsn(900000 + id).
			SetOrgID(1).SetStatus(status).SetCreated(t0).SetUpdated(t0).Save(ctx)
	case peeringdb.TypeFac:
		_, err = c.Facility.Create().SetID(id).SetName("Fac").SetOrgID(1).
			SetStatus(status).SetCreated(t0).SetUpdated(t0).Save(ctx)
	case peeringdb.TypeIX:
		_, err = c.InternetExchange.Create().SetID(id).SetName("IX").SetOrgID(1).
			SetStatus(status).SetCreated(t0).SetUpdated(t0).Save(ctx)
	case peeringdb.TypeCarrier:
		_, err = c.Carrier.Create().SetID(id).SetName("Carrier").SetOrgID(1).
			SetStatus(status).SetCreated(t0).SetUpdated(t0).Save(ctx)
	case peeringdb.TypeCampus:
		_, err = c.Campus.Create().SetID(id).SetName("Campus").SetOrgID(1).
			SetStatus(status).SetCreated(t0).SetUpdated(t0).Save(ctx)
	case peeringdb.TypePoc:
		// visible defaults to "Public" → anon-visible, so only status gates it.
		_, err = c.Poc.Create().SetID(id).SetNetID(10).SetName("Poc").SetRole("NOC").
			SetStatus(status).SetCreated(t0).SetUpdated(t0).Save(ctx)
	case peeringdb.TypeIXLan:
		_, err = c.IxLan.Create().SetID(id).SetIxID(20).SetStatus(status).
			SetCreated(t0).SetUpdated(t0).Save(ctx)
	case peeringdb.TypeIXPfx:
		_, err = c.IxPrefix.Create().SetID(id).SetIxlanID(70).
			SetPrefix(fmt.Sprintf("10.0.%d.0/24", id-900)).SetProtocol("IPv4").
			SetStatus(status).SetCreated(t0).SetUpdated(t0).Save(ctx)
	case peeringdb.TypeNetIXLan:
		_, err = c.NetworkIxLan.Create().SetID(id).SetNetID(10).SetIxlanID(70).
			SetIxID(20).SetAsn(13335).SetSpeed(10000).SetName("NetIXLan").
			SetStatus(status).SetCreated(t0).SetUpdated(t0).Save(ctx)
	case peeringdb.TypeNetFac:
		_, err = c.NetworkFacility.Create().SetID(id).SetNetID(10).SetFacID(30).
			SetLocalAsn(13335).SetName("NetFac").SetStatus(status).
			SetCreated(t0).SetUpdated(t0).Save(ctx)
	case peeringdb.TypeIXFac:
		_, err = c.IxFacility.Create().SetID(id).SetIxID(20).SetFacID(30).
			SetName("IXFac").SetStatus(status).SetCreated(t0).SetUpdated(t0).Save(ctx)
	case peeringdb.TypeCarrierFac:
		_, err = c.CarrierFacility.Create().SetID(id).SetCarrierID(50).SetFacID(30).
			SetName("CarrierFac").SetStatus(status).SetCreated(t0).SetUpdated(t0).Save(ctx)
	default:
		tb.Fatalf("seedStatusRow: unknown tag %q", tag)
	}
	if err != nil {
		tb.Fatalf("seed %s id=%d status=%s: %v", tag, id, status, err)
	}
}

// statusMatrixEntities lists every PeeringDB list endpoint tag with its
// isCampus flag (campus admits status=pending under ?since, the others do
// not) and its live status set (upstream live_statuses(), 2.83.0
// models.py:109-122). The live sets are spelled out here rather than read
// from pdbtypes.LiveStatuses, so a wrong mapping there fails the tests.
// Shared by the status-matrix and depth PK breadth tests.
var statusMatrixEntities = []struct {
	tag      string
	isCampus bool
	live     []string
}{
	{peeringdb.TypeOrg, false, []string{"ok"}},
	{peeringdb.TypeNet, false, []string{"ok"}},
	{peeringdb.TypeFac, false, []string{"ok"}},
	{peeringdb.TypeIX, false, []string{"ok"}},
	{peeringdb.TypePoc, false, []string{"ok"}},
	{peeringdb.TypeIXLan, false, []string{"ok"}},
	{peeringdb.TypeIXPfx, false, []string{"ok"}},
	{peeringdb.TypeNetIXLan, false, []string{"ok", "not-operational"}},
	{peeringdb.TypeNetFac, false, []string{"ok"}},
	{peeringdb.TypeIXFac, false, []string{"ok"}},
	{peeringdb.TypeCarrier, false, []string{"ok"}},
	{peeringdb.TypeCarrierFac, false, []string{"ok"}},
	{peeringdb.TypeCampus, true, []string{"ok"}},
}

// seedStatusMatrixRows seeds one row of tag per status for the breadth
// tests: 901 ok, 902 deleted, 903 pending, then one row from 904 up for
// each further live status (netixlan: 904 not-operational). It returns
// the IDs of the live rows.
func seedStatusMatrixRows(tb testing.TB, c *ent.Client, tag string, live []string) []int {
	tb.Helper()
	seedStatusRow(tb, c, tag, 901, "ok")
	seedStatusRow(tb, c, tag, 902, "deleted")
	seedStatusRow(tb, c, tag, 903, "pending")
	liveIDs := []int{901}
	next := 904
	for _, status := range live {
		if status == "ok" {
			continue
		}
		seedStatusRow(tb, c, tag, next, status)
		liveIDs = append(liveIDs, next)
		next++
	}
	return liveIDs
}
