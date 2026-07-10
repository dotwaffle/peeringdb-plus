package pdbcompat

import (
	"context"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/campus"
	"github.com/dotwaffle/peeringdb-plus/ent/carrier"
	"github.com/dotwaffle/peeringdb-plus/ent/carrierfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/facility"
	"github.com/dotwaffle/peeringdb-plus/ent/internetexchange"
	"github.com/dotwaffle/peeringdb-plus/ent/ixfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/ixlan"
	"github.com/dotwaffle/peeringdb-plus/ent/ixprefix"
	"github.com/dotwaffle/peeringdb-plus/ent/network"
	"github.com/dotwaffle/peeringdb-plus/ent/networkfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/networkixlan"
	"github.com/dotwaffle/peeringdb-plus/ent/poc"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// detailChildSet describes one reverse `_set` collection that a depth>=2
// detail response fully embeds: the child entity whose Depth0 row size
// prices each element, and a COUNT(*) closure mirroring the status filter
// the depth expansion applies when it loads the set (StatusIn ok/pending,
// same as the With* eager-loads in depth.go).
type detailChildSet struct {
	childType string
	count     func(ctx context.Context, client *ent.Client, id int) (int, error)
}

// detailChildSets maps each parent type to the `_set` collections its
// depth>=2 detail response embeds as full child objects. The entries
// mirror the get<Type>WithDepth eager-loads in depth.go — a set added or
// removed there must be reflected here or the in-flight pool estimate
// drifts from what the response actually materializes. Through-relation
// sets price the resolved entity, not the join row: ix.fac_set renders
// facilities (one per ixfac join row) and ixlan.net_set renders networks
// (one per netixlan join row), so their counts run over the join table
// while childType names the rendered entity. Leaf types (poc, ixpfx,
// netixlan, netfac, ixfac, carrierfac) embed only bounded parent FK
// objects at depth>=2 — the flat Depth2 figure already covers them, so
// they carry no entry.
var detailChildSets = map[string][]detailChildSet{
	peeringdb.TypeOrg: {
		{peeringdb.TypeNet, func(ctx context.Context, client *ent.Client, id int) (int, error) {
			return client.Network.Query().Where(network.OrgID(id), network.StatusIn("ok", "pending")).Count(ctx)
		}},
		{peeringdb.TypeFac, func(ctx context.Context, client *ent.Client, id int) (int, error) {
			return client.Facility.Query().Where(facility.OrgID(id), facility.StatusIn("ok", "pending")).Count(ctx)
		}},
		{peeringdb.TypeIX, func(ctx context.Context, client *ent.Client, id int) (int, error) {
			return client.InternetExchange.Query().Where(internetexchange.OrgID(id), internetexchange.StatusIn("ok", "pending")).Count(ctx)
		}},
		{peeringdb.TypeCarrier, func(ctx context.Context, client *ent.Client, id int) (int, error) {
			return client.Carrier.Query().Where(carrier.OrgID(id), carrier.StatusIn("ok", "pending")).Count(ctx)
		}},
		{peeringdb.TypeCampus, func(ctx context.Context, client *ent.Client, id int) (int, error) {
			return client.Campus.Query().Where(campus.OrgID(id), campus.StatusIn("ok", "pending")).Count(ctx)
		}},
	},
	peeringdb.TypeNet: {
		{peeringdb.TypePoc, func(ctx context.Context, client *ent.Client, id int) (int, error) {
			return client.Poc.Query().Where(poc.NetID(id), poc.StatusIn("ok", "pending")).Count(ctx)
		}},
		{peeringdb.TypeNetFac, func(ctx context.Context, client *ent.Client, id int) (int, error) {
			return client.NetworkFacility.Query().Where(networkfacility.NetID(id), networkfacility.StatusIn("ok", "pending")).Count(ctx)
		}},
		{peeringdb.TypeNetIXLan, func(ctx context.Context, client *ent.Client, id int) (int, error) {
			return client.NetworkIxLan.Query().Where(networkixlan.NetID(id), networkixlan.StatusIn("ok", "pending")).Count(ctx)
		}},
	},
	peeringdb.TypeFac: {
		{peeringdb.TypeNetFac, func(ctx context.Context, client *ent.Client, id int) (int, error) {
			return client.NetworkFacility.Query().Where(networkfacility.FacID(id), networkfacility.StatusIn("ok", "pending")).Count(ctx)
		}},
		{peeringdb.TypeIXFac, func(ctx context.Context, client *ent.Client, id int) (int, error) {
			return client.IxFacility.Query().Where(ixfacility.FacID(id), ixfacility.StatusIn("ok", "pending")).Count(ctx)
		}},
		{peeringdb.TypeCarrierFac, func(ctx context.Context, client *ent.Client, id int) (int, error) {
			return client.CarrierFacility.Query().Where(carrierfacility.FacID(id), carrierfacility.StatusIn("ok", "pending")).Count(ctx)
		}},
	},
	peeringdb.TypeIX: {
		{peeringdb.TypeIXLan, func(ctx context.Context, client *ent.Client, id int) (int, error) {
			return client.IxLan.Query().Where(ixlan.IxID(id), ixlan.StatusIn("ok", "pending")).Count(ctx)
		}},
		// ix.fac_set resolves facilities through the ixfac join.
		{peeringdb.TypeFac, func(ctx context.Context, client *ent.Client, id int) (int, error) {
			return client.IxFacility.Query().Where(ixfacility.IxID(id), ixfacility.StatusIn("ok", "pending")).Count(ctx)
		}},
	},
	peeringdb.TypeIXLan: {
		{peeringdb.TypeIXPfx, func(ctx context.Context, client *ent.Client, id int) (int, error) {
			return client.IxPrefix.Query().Where(ixprefix.IxlanID(id), ixprefix.StatusIn("ok", "pending")).Count(ctx)
		}},
		// ixlan.net_set resolves networks through the netixlan join.
		{peeringdb.TypeNet, func(ctx context.Context, client *ent.Client, id int) (int, error) {
			return client.NetworkIxLan.Query().Where(networkixlan.IxlanID(id), networkixlan.StatusIn("ok", "pending")).Count(ctx)
		}},
	},
	peeringdb.TypeCarrier: {
		{peeringdb.TypeCarrierFac, func(ctx context.Context, client *ent.Client, id int) (int, error) {
			return client.CarrierFacility.Query().Where(carrierfacility.CarrierID(id), carrierfacility.StatusIn("ok", "pending")).Count(ctx)
		}},
	},
	peeringdb.TypeCampus: {
		{peeringdb.TypeFac, func(ctx context.Context, client *ent.Client, id int) (int, error) {
			return client.Facility.Query().Where(facility.CampusID(id), facility.StatusIn("ok", "pending")).Count(ctx)
		}},
	},
}

// detailInflightEstimate prices a detail response for the shared in-flight
// pool (Handler.inflightBytes). At depth < 2 the response is a bare row or
// flat-FK-plus-ID-lists shape, bounded by the flat TypicalRowBytes figure.
// At depth >= 2 the embedded `_set` collections are unbounded — a hub org
// expands thousands of full network objects — so the flat Depth2 figure
// is a floor, not an estimate; each embedded set adds child COUNT(*) ×
// child Depth0 bytes on top of it. A failed count degrades to the flat
// figure for that set rather than erroring: admission is a safety net and
// a count hiccup must not turn a servable detail request into a 5xx (the
// real query will surface a persistent DB error moments later anyway).
func detailInflightEstimate(ctx context.Context, client *ent.Client, typeName string, id, depth int) int64 {
	est := int64(TypicalRowBytes(typeName, depth))
	if depth < 2 {
		return est
	}
	for _, cs := range detailChildSets[typeName] {
		n, err := cs.count(ctx, client, id)
		if err != nil {
			continue
		}
		est += int64(n) * int64(TypicalRowBytes(cs.childType, 0))
	}
	return est
}
