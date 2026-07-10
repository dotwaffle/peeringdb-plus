// The per-type registry. Every switch, map, and list that used to
// fan out over the 13 PeeringDB entity types by hand (dispatch switch,
// upsertSingleRaw switch, dbHasRecord switch, entityTables map,
// scratchTypes list, initial-counts UNION ALL) now derives from the
// single ordered typeRegistry slice below. Adding a 14th type means
// adding ONE descriptor entry — the compiler and the lockstep tests
// catch anything else.

package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
	"github.com/dotwaffle/peeringdb-plus/ent/organization"
	"github.com/dotwaffle/peeringdb-plus/ent/poc"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// typeDescriptor bundles every per-type wiring point for one PeeringDB
// entity type. The closures carry the concrete Go type so the rest of
// the sync code can stay type-erased:
//
//   - chunkUpsert: the Phase B scratch-chunk replay (decode + FK filter
//   - bulk upsert) — the body each dispatchScratchChunk arm used to
//     hold. FK-orphan policy lives in the fkFilter closures here.
//   - singleUpsert: decode ONE raw JSON object and land it via the same
//     bulk upsert helper — the FK-backfill path.
//   - exists: ID existence probe against the real DB inside the sync tx.
type typeDescriptor struct {
	name  string // PeeringDB type name, e.g. "org"
	table string // ent SQL table name, e.g. "organizations"

	chunkUpsert  func(w *Worker, ctx context.Context, tx *ent.Tx, rows []scratchRow) (int, error)
	singleUpsert func(ctx context.Context, tx *ent.Tx, raw json.RawMessage) (int, error)
	exists       func(ctx context.Context, tx *ent.Tx, id int) (bool, error)
}

// typeRegistry is the single source of truth for the 13 PeeringDB
// entity types, ordered parent-before-child (FK dependency order — the
// order Phase B replays scratch tables in). canonicalStepOrder,
// entityTables, scratchTypes, and the initial-counts UNION ALL all
// derive from this slice.
var typeRegistry = []typeDescriptor{
	{
		name:  peeringdb.TypeOrg,
		table: "organizations",
		chunkUpsert: func(w *Worker, ctx context.Context, tx *ent.Tx, rows []scratchRow) (int, error) {
			return syncIncremental(ctx, tx, syncIncrementalInput[peeringdb.Organization]{
				objectType: peeringdb.TypeOrg,
				recordIDs:  func(ids []int) { w.fkRegisterIDs(peeringdb.TypeOrg, ids) },
				upsert:     upsertOrganizations,
			}, rows)
		},
		singleUpsert: singleRawUpserter(peeringdb.TypeOrg,
			func(v peeringdb.Organization) int { return v.ID }, upsertOrganizations),
		exists: func(ctx context.Context, tx *ent.Tx, id int) (bool, error) {
			return tx.Organization.Query().Where(organization.ID(id)).Exist(ctx)
		},
	},
	{
		name:  peeringdb.TypeCampus,
		table: "campuses",
		chunkUpsert: func(w *Worker, ctx context.Context, tx *ent.Tx, rows []scratchRow) (int, error) {
			return syncIncremental(ctx, tx, syncIncrementalInput[peeringdb.Campus]{
				objectType: peeringdb.TypeCampus,
				fkRefs: func(v *peeringdb.Campus) []parentFKRef {
					return []parentFKRef{{FieldName: "org_id", ParentType: peeringdb.TypeOrg, ID: v.OrgID}}
				},
				prefetch: func(refs []parentFKRef) {
					w.prefetchMissingParents(ctx, tx, peeringdb.TypeCampus, refs)
				},
				fkFilter: func(v *peeringdb.Campus) bool {
					return w.fkCheckParent(ctx, tx, peeringdb.TypeCampus, v.ID,
						peeringdb.TypeOrg, v.OrgID, "org_id")
				},
				recordIDs: func(ids []int) { w.fkRegisterIDs(peeringdb.TypeCampus, ids) },
				upsert:    upsertCampuses,
			}, rows)
		},
		singleUpsert: singleRawUpserter(peeringdb.TypeCampus,
			func(v peeringdb.Campus) int { return v.ID }, upsertCampuses),
		exists: func(ctx context.Context, tx *ent.Tx, id int) (bool, error) {
			return tx.Campus.Query().Where(campus.ID(id)).Exist(ctx)
		},
	},
	{
		name:  peeringdb.TypeFac,
		table: "facilities",
		chunkUpsert: func(w *Worker, ctx context.Context, tx *ent.Tx, rows []scratchRow) (int, error) {
			return syncIncremental(ctx, tx, syncIncrementalInput[peeringdb.Facility]{
				objectType: peeringdb.TypeFac,
				fkRefs: func(v *peeringdb.Facility) []parentFKRef {
					return []parentFKRef{{FieldName: "org_id", ParentType: peeringdb.TypeOrg, ID: v.OrgID}}
				},
				prefetch: func(refs []parentFKRef) {
					w.prefetchMissingParents(ctx, tx, peeringdb.TypeFac, refs)
				},
				fkFilter: func(v *peeringdb.Facility) bool {
					if !w.fkCheckParent(ctx, tx, peeringdb.TypeFac, v.ID,
						peeringdb.TypeOrg, v.OrgID, "org_id") {
						return false
					}
					// campus_id is Optional().Nillable() in the ent
					// schema — if the referenced campus is missing,
					// null the reference out and keep the facility
					// (avoids cascading the drop through netfac /
					// ixfac / carrierfac children of the facility).
					if v.CampusID != nil && !w.fkHasParent(ctx, tx, peeringdb.TypeCampus, *v.CampusID) {
						w.recordOrphan(ctx, fkOrphanKey{
							ChildType:  peeringdb.TypeFac,
							ParentType: peeringdb.TypeCampus,
							Field:      "campus_id",
							Action:     "null",
						}, v.ID, *v.CampusID)
						v.CampusID = nil
					}
					return true
				},
				recordIDs: func(ids []int) { w.fkRegisterIDs(peeringdb.TypeFac, ids) },
				upsert:    upsertFacilities,
			}, rows)
		},
		singleUpsert: singleRawUpserter(peeringdb.TypeFac,
			func(v peeringdb.Facility) int { return v.ID }, upsertFacilities),
		exists: func(ctx context.Context, tx *ent.Tx, id int) (bool, error) {
			return tx.Facility.Query().Where(facility.ID(id)).Exist(ctx)
		},
	},
	{
		name:  peeringdb.TypeCarrier,
		table: "carriers",
		chunkUpsert: func(w *Worker, ctx context.Context, tx *ent.Tx, rows []scratchRow) (int, error) {
			return syncIncremental(ctx, tx, syncIncrementalInput[peeringdb.Carrier]{
				objectType: peeringdb.TypeCarrier,
				fkRefs: func(v *peeringdb.Carrier) []parentFKRef {
					return []parentFKRef{{FieldName: "org_id", ParentType: peeringdb.TypeOrg, ID: v.OrgID}}
				},
				prefetch: func(refs []parentFKRef) {
					w.prefetchMissingParents(ctx, tx, peeringdb.TypeCarrier, refs)
				},
				fkFilter: func(v *peeringdb.Carrier) bool {
					return w.fkCheckParent(ctx, tx, peeringdb.TypeCarrier, v.ID,
						peeringdb.TypeOrg, v.OrgID, "org_id")
				},
				recordIDs: func(ids []int) { w.fkRegisterIDs(peeringdb.TypeCarrier, ids) },
				upsert:    upsertCarriers,
			}, rows)
		},
		singleUpsert: singleRawUpserter(peeringdb.TypeCarrier,
			func(v peeringdb.Carrier) int { return v.ID }, upsertCarriers),
		exists: func(ctx context.Context, tx *ent.Tx, id int) (bool, error) {
			return tx.Carrier.Query().Where(carrier.ID(id)).Exist(ctx)
		},
	},
	{
		name:  peeringdb.TypeCarrierFac,
		table: "carrier_facilities",
		chunkUpsert: func(w *Worker, ctx context.Context, tx *ent.Tx, rows []scratchRow) (int, error) {
			return syncIncremental(ctx, tx, syncIncrementalInput[peeringdb.CarrierFacility]{
				objectType: peeringdb.TypeCarrierFac,
				fkRefs: func(v *peeringdb.CarrierFacility) []parentFKRef {
					return []parentFKRef{
						{FieldName: "carrier_id", ParentType: peeringdb.TypeCarrier, ID: v.CarrierID},
						{FieldName: "fac_id", ParentType: peeringdb.TypeFac, ID: v.FacID},
					}
				},
				prefetch: func(refs []parentFKRef) {
					w.prefetchMissingParents(ctx, tx, peeringdb.TypeCarrierFac, refs)
				},
				fkFilter: func(v *peeringdb.CarrierFacility) bool {
					if !w.fkCheckParent(ctx, tx, peeringdb.TypeCarrierFac, v.ID,
						peeringdb.TypeCarrier, v.CarrierID, "carrier_id") {
						return false
					}
					return w.fkCheckParent(ctx, tx, peeringdb.TypeCarrierFac, v.ID,
						peeringdb.TypeFac, v.FacID, "fac_id")
				},
				recordIDs: func(ids []int) { w.fkRegisterIDs(peeringdb.TypeCarrierFac, ids) },
				upsert:    upsertCarrierFacilities,
			}, rows)
		},
		singleUpsert: singleRawUpserter(peeringdb.TypeCarrierFac,
			func(v peeringdb.CarrierFacility) int { return v.ID }, upsertCarrierFacilities),
		exists: func(ctx context.Context, tx *ent.Tx, id int) (bool, error) {
			return tx.CarrierFacility.Query().Where(carrierfacility.ID(id)).Exist(ctx)
		},
	},
	{
		name:  peeringdb.TypeIX,
		table: "internet_exchanges",
		chunkUpsert: func(w *Worker, ctx context.Context, tx *ent.Tx, rows []scratchRow) (int, error) {
			return syncIncremental(ctx, tx, syncIncrementalInput[peeringdb.InternetExchange]{
				objectType: peeringdb.TypeIX,
				fkRefs: func(v *peeringdb.InternetExchange) []parentFKRef {
					return []parentFKRef{{FieldName: "org_id", ParentType: peeringdb.TypeOrg, ID: v.OrgID}}
				},
				prefetch: func(refs []parentFKRef) {
					w.prefetchMissingParents(ctx, tx, peeringdb.TypeIX, refs)
				},
				fkFilter: func(v *peeringdb.InternetExchange) bool {
					return w.fkCheckParent(ctx, tx, peeringdb.TypeIX, v.ID,
						peeringdb.TypeOrg, v.OrgID, "org_id")
				},
				recordIDs: func(ids []int) { w.fkRegisterIDs(peeringdb.TypeIX, ids) },
				upsert:    upsertInternetExchanges,
			}, rows)
		},
		singleUpsert: singleRawUpserter(peeringdb.TypeIX,
			func(v peeringdb.InternetExchange) int { return v.ID }, upsertInternetExchanges),
		exists: func(ctx context.Context, tx *ent.Tx, id int) (bool, error) {
			return tx.InternetExchange.Query().Where(internetexchange.ID(id)).Exist(ctx)
		},
	},
	{
		name:  peeringdb.TypeIXLan,
		table: "ix_lans",
		chunkUpsert: func(w *Worker, ctx context.Context, tx *ent.Tx, rows []scratchRow) (int, error) {
			return syncIncremental(ctx, tx, syncIncrementalInput[peeringdb.IxLan]{
				objectType: peeringdb.TypeIXLan,
				fkRefs: func(v *peeringdb.IxLan) []parentFKRef {
					return []parentFKRef{{FieldName: "ix_id", ParentType: peeringdb.TypeIX, ID: v.IXID}}
				},
				prefetch: func(refs []parentFKRef) {
					w.prefetchMissingParents(ctx, tx, peeringdb.TypeIXLan, refs)
				},
				fkFilter: func(v *peeringdb.IxLan) bool {
					return w.fkCheckParent(ctx, tx, peeringdb.TypeIXLan, v.ID,
						peeringdb.TypeIX, v.IXID, "ix_id")
				},
				recordIDs: func(ids []int) { w.fkRegisterIDs(peeringdb.TypeIXLan, ids) },
				upsert:    upsertIxLans,
			}, rows)
		},
		singleUpsert: singleRawUpserter(peeringdb.TypeIXLan,
			func(v peeringdb.IxLan) int { return v.ID }, upsertIxLans),
		exists: func(ctx context.Context, tx *ent.Tx, id int) (bool, error) {
			return tx.IxLan.Query().Where(ixlan.ID(id)).Exist(ctx)
		},
	},
	{
		name:  peeringdb.TypeIXPfx,
		table: "ix_prefixes",
		chunkUpsert: func(w *Worker, ctx context.Context, tx *ent.Tx, rows []scratchRow) (int, error) {
			return syncIncremental(ctx, tx, syncIncrementalInput[peeringdb.IxPrefix]{
				objectType: peeringdb.TypeIXPfx,
				fkRefs: func(v *peeringdb.IxPrefix) []parentFKRef {
					return []parentFKRef{{FieldName: "ixlan_id", ParentType: peeringdb.TypeIXLan, ID: v.IXLanID}}
				},
				prefetch: func(refs []parentFKRef) {
					w.prefetchMissingParents(ctx, tx, peeringdb.TypeIXPfx, refs)
				},
				fkFilter: func(v *peeringdb.IxPrefix) bool {
					return w.fkCheckParent(ctx, tx, peeringdb.TypeIXPfx, v.ID,
						peeringdb.TypeIXLan, v.IXLanID, "ixlan_id")
				},
				recordIDs: func(ids []int) { w.fkRegisterIDs(peeringdb.TypeIXPfx, ids) },
				upsert:    upsertIxPrefixes,
			}, rows)
		},
		singleUpsert: singleRawUpserter(peeringdb.TypeIXPfx,
			func(v peeringdb.IxPrefix) int { return v.ID }, upsertIxPrefixes),
		exists: func(ctx context.Context, tx *ent.Tx, id int) (bool, error) {
			return tx.IxPrefix.Query().Where(ixprefix.ID(id)).Exist(ctx)
		},
	},
	{
		name:  peeringdb.TypeIXFac,
		table: "ix_facilities",
		chunkUpsert: func(w *Worker, ctx context.Context, tx *ent.Tx, rows []scratchRow) (int, error) {
			return syncIncremental(ctx, tx, syncIncrementalInput[peeringdb.IxFacility]{
				objectType: peeringdb.TypeIXFac,
				fkRefs: func(v *peeringdb.IxFacility) []parentFKRef {
					return []parentFKRef{
						{FieldName: "ix_id", ParentType: peeringdb.TypeIX, ID: v.IXID},
						{FieldName: "fac_id", ParentType: peeringdb.TypeFac, ID: v.FacID},
					}
				},
				prefetch: func(refs []parentFKRef) {
					w.prefetchMissingParents(ctx, tx, peeringdb.TypeIXFac, refs)
				},
				fkFilter: func(v *peeringdb.IxFacility) bool {
					if !w.fkCheckParent(ctx, tx, peeringdb.TypeIXFac, v.ID,
						peeringdb.TypeIX, v.IXID, "ix_id") {
						return false
					}
					return w.fkCheckParent(ctx, tx, peeringdb.TypeIXFac, v.ID,
						peeringdb.TypeFac, v.FacID, "fac_id")
				},
				recordIDs: func(ids []int) { w.fkRegisterIDs(peeringdb.TypeIXFac, ids) },
				upsert:    upsertIxFacilities,
			}, rows)
		},
		singleUpsert: singleRawUpserter(peeringdb.TypeIXFac,
			func(v peeringdb.IxFacility) int { return v.ID }, upsertIxFacilities),
		exists: func(ctx context.Context, tx *ent.Tx, id int) (bool, error) {
			return tx.IxFacility.Query().Where(ixfacility.ID(id)).Exist(ctx)
		},
	},
	{
		name:  peeringdb.TypeNet,
		table: "networks",
		chunkUpsert: func(w *Worker, ctx context.Context, tx *ent.Tx, rows []scratchRow) (int, error) {
			return syncIncremental(ctx, tx, syncIncrementalInput[peeringdb.Network]{
				objectType: peeringdb.TypeNet,
				fkRefs: func(v *peeringdb.Network) []parentFKRef {
					return []parentFKRef{{FieldName: "org_id", ParentType: peeringdb.TypeOrg, ID: v.OrgID}}
				},
				prefetch: func(refs []parentFKRef) {
					w.prefetchMissingParents(ctx, tx, peeringdb.TypeNet, refs)
				},
				fkFilter: func(v *peeringdb.Network) bool {
					return w.fkCheckParent(ctx, tx, peeringdb.TypeNet, v.ID,
						peeringdb.TypeOrg, v.OrgID, "org_id")
				},
				recordIDs: func(ids []int) { w.fkRegisterIDs(peeringdb.TypeNet, ids) },
				upsert:    upsertNetworks,
			}, rows)
		},
		singleUpsert: singleRawUpserter(peeringdb.TypeNet,
			func(v peeringdb.Network) int { return v.ID }, upsertNetworks),
		exists: func(ctx context.Context, tx *ent.Tx, id int) (bool, error) {
			return tx.Network.Query().Where(network.ID(id)).Exist(ctx)
		},
	},
	{
		name:  peeringdb.TypePoc,
		table: "pocs",
		chunkUpsert: func(w *Worker, ctx context.Context, tx *ent.Tx, rows []scratchRow) (int, error) {
			return syncIncremental(ctx, tx, syncIncrementalInput[peeringdb.Poc]{
				objectType: peeringdb.TypePoc,
				fkRefs: func(v *peeringdb.Poc) []parentFKRef {
					return []parentFKRef{{FieldName: "net_id", ParentType: peeringdb.TypeNet, ID: v.NetID}}
				},
				prefetch: func(refs []parentFKRef) {
					w.prefetchMissingParents(ctx, tx, peeringdb.TypePoc, refs)
				},
				fkFilter: func(v *peeringdb.Poc) bool {
					return w.fkCheckParent(ctx, tx, peeringdb.TypePoc, v.ID,
						peeringdb.TypeNet, v.NetID, "net_id")
				},
				recordIDs: func(ids []int) { w.fkRegisterIDs(peeringdb.TypePoc, ids) },
				upsert:    upsertPocs,
			}, rows)
		},
		singleUpsert: singleRawUpserter(peeringdb.TypePoc,
			func(v peeringdb.Poc) int { return v.ID }, upsertPocs),
		exists: func(ctx context.Context, tx *ent.Tx, id int) (bool, error) {
			return tx.Poc.Query().Where(poc.ID(id)).Exist(ctx)
		},
	},
	{
		name:  peeringdb.TypeNetFac,
		table: "network_facilities",
		chunkUpsert: func(w *Worker, ctx context.Context, tx *ent.Tx, rows []scratchRow) (int, error) {
			return syncIncremental(ctx, tx, syncIncrementalInput[peeringdb.NetworkFacility]{
				objectType: peeringdb.TypeNetFac,
				fkRefs: func(v *peeringdb.NetworkFacility) []parentFKRef {
					return []parentFKRef{
						{FieldName: "net_id", ParentType: peeringdb.TypeNet, ID: v.NetID},
						{FieldName: "fac_id", ParentType: peeringdb.TypeFac, ID: v.FacID},
					}
				},
				prefetch: func(refs []parentFKRef) {
					w.prefetchMissingParents(ctx, tx, peeringdb.TypeNetFac, refs)
				},
				fkFilter: func(v *peeringdb.NetworkFacility) bool {
					if !w.fkCheckParent(ctx, tx, peeringdb.TypeNetFac, v.ID,
						peeringdb.TypeNet, v.NetID, "net_id") {
						return false
					}
					return w.fkCheckParent(ctx, tx, peeringdb.TypeNetFac, v.ID,
						peeringdb.TypeFac, v.FacID, "fac_id")
				},
				recordIDs: func(ids []int) { w.fkRegisterIDs(peeringdb.TypeNetFac, ids) },
				upsert:    upsertNetworkFacilities,
			}, rows)
		},
		singleUpsert: singleRawUpserter(peeringdb.TypeNetFac,
			func(v peeringdb.NetworkFacility) int { return v.ID }, upsertNetworkFacilities),
		exists: func(ctx context.Context, tx *ent.Tx, id int) (bool, error) {
			return tx.NetworkFacility.Query().Where(networkfacility.ID(id)).Exist(ctx)
		},
	},
	{
		name:  peeringdb.TypeNetIXLan,
		table: "network_ix_lans",
		chunkUpsert: func(w *Worker, ctx context.Context, tx *ent.Tx, rows []scratchRow) (int, error) {
			return syncIncremental(ctx, tx, syncIncrementalInput[peeringdb.NetworkIxLan]{
				objectType: peeringdb.TypeNetIXLan,
				fkRefs: func(v *peeringdb.NetworkIxLan) []parentFKRef {
					return []parentFKRef{
						{FieldName: "net_id", ParentType: peeringdb.TypeNet, ID: v.NetID},
						{FieldName: "ixlan_id", ParentType: peeringdb.TypeIXLan, ID: v.IXLanID},
					}
				},
				prefetch: func(refs []parentFKRef) {
					w.prefetchMissingParents(ctx, tx, peeringdb.TypeNetIXLan, refs)
				},
				fkFilter: func(v *peeringdb.NetworkIxLan) bool {
					// Required FKs: net_id, ixlan_id. Drop on miss after
					// backfill attempt (legacy behavior).
					//
					// NOTE: ix_id is NOT an independent FK upstream — it is
					// serializer-computed from ixlan.ix_id (peeringdb_server/
					// serializers.py NetworkIxLanSerializer). Validating
					// ixlan_id (below) is sufficient. We
					// removed the redundant ix_id check that was producing
					// false-positive orphans whenever an ix mid-sync was a
					// missing parent for an otherwise-valid netixlan row.
					if !w.fkCheckParent(ctx, tx, peeringdb.TypeNetIXLan, v.ID,
						peeringdb.TypeNet, v.NetID, "net_id") {
						return false
					}
					if !w.fkCheckParent(ctx, tx, peeringdb.TypeNetIXLan, v.ID,
						peeringdb.TypeIXLan, v.IXLanID, "ixlan_id") {
						return false
					}
					// Optional side FKs: net_side_id, ix_side_id. Upstream
					// peeringdb_server/models.py:5630-5642 declares both as
					// `null=True, on_delete=SET_NULL`. We
					// mirror that contract by null-on-miss (after backfill
					// attempt) rather than dropping the entire row.
					w.nullSideFK(ctx, tx, &v.NetSideID, "net_side_id", v.ID)
					w.nullSideFK(ctx, tx, &v.IXSideID, "ix_side_id", v.ID)
					return true
				},
				recordIDs: func(ids []int) { w.fkRegisterIDs(peeringdb.TypeNetIXLan, ids) },
				upsert:    upsertNetworkIxLans,
			}, rows)
		},
		singleUpsert: singleRawUpserter(peeringdb.TypeNetIXLan,
			func(v peeringdb.NetworkIxLan) int { return v.ID }, upsertNetworkIxLans),
		exists: func(ctx context.Context, tx *ent.Tx, id int) (bool, error) {
			return tx.NetworkIxLan.Query().Where(networkixlan.ID(id)).Exist(ctx)
		},
	},
}

// singleRawUpserter adapts a per-type bulk upsert helper into the
// type-erased singleUpsert closure shape via decodeAndUpsertSingle.
func singleRawUpserter[E any](
	name string,
	idFn func(E) int,
	upsert func(context.Context, *ent.Tx, []E) ([]int, error),
) func(context.Context, *ent.Tx, json.RawMessage) (int, error) {
	return func(ctx context.Context, tx *ent.Tx, raw json.RawMessage) (int, error) {
		return decodeAndUpsertSingle(ctx, tx, name, raw, idFn, upsert)
	}
}

// descriptorByName indexes typeRegistry for the dispatch paths.
// Populated in init() rather than a var initializer: the chunkUpsert
// closures reference Worker methods that read this map, and an
// initializer expression would form a static initialization cycle.
var descriptorByName map[string]*typeDescriptor

func init() {
	descriptorByName = make(map[string]*typeDescriptor, len(typeRegistry))
	for i := range typeRegistry {
		descriptorByName[typeRegistry[i].name] = &typeRegistry[i]
	}
}

// canonicalStepOrder is the sync step ordering (FK dependency order),
// derived from typeRegistry. syncSteps() zips this with the per-type
// work; StepOrder() exposes a defensive copy for out-of-package
// consumers (e.g. cmd/loadtest's sync mode parity test).
var canonicalStepOrder = func() []string {
	out := make([]string, len(typeRegistry))
	for i, d := range typeRegistry {
		out[i] = d.name
	}
	return out
}()

// entityTables maps a PeeringDB type name (the keys produced by
// syncSteps()) to the underlying ent table name, derived from
// typeRegistry. Consumers read raw SQLite tables outside the ent client
// (cursor derivation, initial counts). TestEntityTablesMatchSchema
// introspects sqlite_master to enforce that every value is a real table.
var entityTables = func() map[string]string {
	m := make(map[string]string, len(typeRegistry))
	for _, d := range typeRegistry {
		m[d.name] = d.table
	}
	return m
}()

// scratchTypes is the closed-set of PeeringDB types that get a scratch
// staging table, derived from typeRegistry. Order does not matter here —
// the FK parent-first order is enforced later at the Phase B replay
// loop via syncSteps().
var scratchTypes = canonicalStepOrder

// initialCountsQuery is the UNION ALL holding the 13 PeeringDB entity
// counts, derived from typeRegistry.
// TestInitialCountsQuery_TableNamesMatchSchema introspects sqlite_master
// to assert each table exists in the live ent schema.
var initialCountsQuery = func() string {
	// Type and table names come from the closed-set typeRegistry —
	// single-quoted SQL string literals and double-quoted identifiers
	// respectively; no user input is involved.
	var b strings.Builder
	for i, d := range typeRegistry {
		if i == 0 {
			fmt.Fprintf(&b, "SELECT '%s' AS t, COUNT(*) AS c FROM %q", d.name, d.table)
			continue
		}
		fmt.Fprintf(&b, "\nUNION ALL SELECT '%s', COUNT(*) FROM %q", d.name, d.table)
	}
	return b.String()
}()
