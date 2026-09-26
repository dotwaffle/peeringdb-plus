package pdbcompat

import (
	"context"
	"fmt"
	"strconv"

	"entgo.io/ent/dialect/sql"

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

// childSet describes one reverse `_set` collection of a parent type: the
// JSON key of the set, the child entity whose Depth0 row size prices each
// element, and a count of the elements per parent.
//
// countByParent returns, for each id in ids that has at least one element
// in the set, the element count. A parent with no element has no entry.
// It sends one GROUP BY query, with ids bound as one JSON array
// (idInJSON). Its status filter is the filter that the depth expansion
// applies when it loads the set: likelyOK for a child type with the one
// live status "ok", likelyNetIXLanSet for the two sets of netixlan rows.
// The poc set counts through PocQuery, so the poc privacy policy applies.
type childSet struct {
	key           string
	childType     string
	countByParent func(ctx context.Context, client *ent.Client, ids []int) (map[int]int, error)
	// load loads the set for the parents of one chunk of a list at
	// depth > 0 (list_depth.go). It applies the status filter of
	// countByParent.
	load setLoader
}

// childSets maps each parent type to the `_set` collections of its depth
// expansion, in the order that get<Type>WithDepth in depth.go renders
// them. The entries mirror the get<Type>WithDepth eager-loads. If a set is added there or
// removed from there, change this table too, or the in-flight pool
// estimate does not agree with the response.
//
// A through-relation set prices the resolved entity, not the join row:
// ix.fac_set renders facilities (one per ixfac join row) and ixlan.net_set
// renders networks (one per netixlan join row). So their counts run over
// the join table, and childType names the rendered entity.
//
// The leaf types (poc, ixpfx, netixlan, netfac, ixfac, carrierfac) and fac
// embed only bounded parent FK objects at depth>=2. The flat Depth2 figure
// covers them, so they have no entry. fac renders its org and campus but
// no reverse set, because the upstream FacilitySerializer has none.
var childSets = map[string][]childSet{
	peeringdb.TypeOrg: {
		{"net_set", peeringdb.TypeNet, func(ctx context.Context, client *ent.Client, ids []int) (map[int]int, error) {
			return countGroups(ctx, client.Network.Query().
				Where(idInJSON(network.FieldOrgID, ids), likelyOK).
				GroupBy(network.FieldOrgID).Aggregate(ent.Count()), parentOrg)
		}, loadOrgNetSet},
		{"fac_set", peeringdb.TypeFac, func(ctx context.Context, client *ent.Client, ids []int) (map[int]int, error) {
			return countGroups(ctx, client.Facility.Query().
				Where(idInJSON(facility.FieldOrgID, ids), likelyOK).
				GroupBy(facility.FieldOrgID).Aggregate(ent.Count()), parentOrg)
		}, loadOrgFacSet},
		{"ix_set", peeringdb.TypeIX, func(ctx context.Context, client *ent.Client, ids []int) (map[int]int, error) {
			return countGroups(ctx, client.InternetExchange.Query().
				Where(idInJSON(internetexchange.FieldOrgID, ids), likelyOK).
				GroupBy(internetexchange.FieldOrgID).Aggregate(ent.Count()), parentOrg)
		}, loadOrgIXSet},
		{"carrier_set", peeringdb.TypeCarrier, func(ctx context.Context, client *ent.Client, ids []int) (map[int]int, error) {
			return countGroups(ctx, client.Carrier.Query().
				Where(idInJSON(carrier.FieldOrgID, ids), likelyOK).
				GroupBy(carrier.FieldOrgID).Aggregate(ent.Count()), parentOrg)
		}, loadOrgCarrierSet},
		{"campus_set", peeringdb.TypeCampus, func(ctx context.Context, client *ent.Client, ids []int) (map[int]int, error) {
			return countGroups(ctx, client.Campus.Query().
				Where(idInJSON(campus.FieldOrgID, ids), likelyOK).
				GroupBy(campus.FieldOrgID).Aggregate(ent.Count()), parentOrg)
		}, loadOrgCampusSet},
	},
	peeringdb.TypeNet: {
		{"poc_set", peeringdb.TypePoc, func(ctx context.Context, client *ent.Client, ids []int) (map[int]int, error) {
			return countGroups(ctx, client.Poc.Query().
				Where(idInJSON(poc.FieldNetID, ids), likelyOK).
				GroupBy(poc.FieldNetID).Aggregate(ent.Count()), parentNet)
		}, loadNetPocSet},
		{"netfac_set", peeringdb.TypeNetFac, func(ctx context.Context, client *ent.Client, ids []int) (map[int]int, error) {
			return countGroups(ctx, client.NetworkFacility.Query().
				Where(idInJSON(networkfacility.FieldNetID, ids), likelyOK).
				GroupBy(networkfacility.FieldNetID).Aggregate(ent.Count()), parentNet)
		}, loadNetNetFacSet},
		{"netixlan_set", peeringdb.TypeNetIXLan, func(ctx context.Context, client *ent.Client, ids []int) (map[int]int, error) {
			return countGroups(ctx, client.NetworkIxLan.Query().
				Where(idInJSON(networkixlan.FieldNetID, ids), likelyNetIXLanSet).
				GroupBy(networkixlan.FieldNetID).Aggregate(ent.Count()), parentNet)
		}, loadNetNetIXLanSet},
	},
	peeringdb.TypeIX: {
		{"ixlan_set", peeringdb.TypeIXLan, func(ctx context.Context, client *ent.Client, ids []int) (map[int]int, error) {
			return countGroups(ctx, client.IxLan.Query().
				Where(idInJSON(ixlan.FieldIxID, ids), likelyOK).
				GroupBy(ixlan.FieldIxID).Aggregate(ent.Count()), parentIX)
		}, loadIXIXLanSet},
		// ix.fac_set resolves facilities through the ixfac join.
		{"fac_set", peeringdb.TypeFac, func(ctx context.Context, client *ent.Client, ids []int) (map[int]int, error) {
			return countGroups(ctx, client.IxFacility.Query().
				Where(idInJSON(ixfacility.FieldIxID, ids), likelyOK).
				GroupBy(ixfacility.FieldIxID).Aggregate(ent.Count()), parentIX)
		}, loadIXFacSet},
	},
	peeringdb.TypeIXLan: {
		{"ixpfx_set", peeringdb.TypeIXPfx, func(ctx context.Context, client *ent.Client, ids []int) (map[int]int, error) {
			return countGroups(ctx, client.IxPrefix.Query().
				Where(idInJSON(ixprefix.FieldIxlanID, ids), likelyOK).
				GroupBy(ixprefix.FieldIxlanID).Aggregate(ent.Count()), parentIXLan)
		}, loadIXLanIXPfxSet},
		// ixlan.net_set resolves networks through the netixlan join.
		{"net_set", peeringdb.TypeNet, func(ctx context.Context, client *ent.Client, ids []int) (map[int]int, error) {
			return countGroups(ctx, client.NetworkIxLan.Query().
				Where(idInJSON(networkixlan.FieldIxlanID, ids), likelyNetIXLanSet).
				GroupBy(networkixlan.FieldIxlanID).Aggregate(ent.Count()), parentIXLan)
		}, loadIXLanNetSet},
	},
	peeringdb.TypeCarrier: {
		{"carrierfac_set", peeringdb.TypeCarrierFac, func(ctx context.Context, client *ent.Client, ids []int) (map[int]int, error) {
			return countGroups(ctx, client.CarrierFacility.Query().
				Where(idInJSON(carrierfacility.FieldCarrierID, ids), likelyOK).
				GroupBy(carrierfacility.FieldCarrierID).Aggregate(ent.Count()), parentCarrier)
		}, loadCarrierCarrierFacSet},
	},
	peeringdb.TypeCampus: {
		{"fac_set", peeringdb.TypeFac, func(ctx context.Context, client *ent.Client, ids []int) (map[int]int, error) {
			return countGroups(ctx, client.Facility.Query().
				Where(idInJSON(facility.FieldCampusID, ids), likelyOK).
				GroupBy(facility.FieldCampusID).Aggregate(ent.Count()), parentCampus)
		}, loadCampusFacSet},
	},
}

// parentCount is one row of a countByParent query. ent scans a GROUP BY
// row into the struct fields by column name, and the group column is the
// FK column of the set. So the struct has one field for each FK column
// that childSets groups by. A query sets only the field of its own FK
// column, and the parent function of the entry reads that field. Count
// takes the COUNT(*) column, which ent scans as "count".
type parentCount struct {
	OrgID     int `json:"org_id"`
	NetID     int `json:"net_id"`
	IxID      int `json:"ix_id"`
	IxlanID   int `json:"ixlan_id"`
	CarrierID int `json:"carrier_id"`
	CampusID  int `json:"campus_id"`
	Count     int `json:"count"`
}

func parentOrg(r parentCount) int     { return r.OrgID }
func parentNet(r parentCount) int     { return r.NetID }
func parentIX(r parentCount) int      { return r.IxID }
func parentIXLan(r parentCount) int   { return r.IxlanID }
func parentCarrier(r parentCount) int { return r.CarrierID }
func parentCampus(r parentCount) int  { return r.CampusID }

// groupScanner is the Scan method of an ent <Type>GroupBy builder.
type groupScanner interface {
	Scan(ctx context.Context, v any) error
}

// countGroups runs the GROUP BY query g and returns the count of each
// parent, keyed by the parent id that parent reads from the row.
func countGroups(ctx context.Context, g groupScanner, parent func(parentCount) int) (map[int]int, error) {
	var rows []parentCount
	if err := g.Scan(ctx, &rows); err != nil {
		return nil, err
	}
	counts := make(map[int]int, len(rows))
	for _, r := range rows {
		counts[parent(r)] = r.Count
	}
	return counts, nil
}

// idInJSON returns `col IN (SELECT value FROM json_each(?))` with ids bound
// as one JSON array, the same form as buildIn. A query over many ids then
// binds one parameter, not one parameter per id.
func idInJSON(col string, ids []int) func(*sql.Selector) {
	b := make([]byte, 0, 2+8*len(ids))
	b = append(b, '[')
	for i, id := range ids {
		if i > 0 {
			b = append(b, ',')
		}
		b = strconv.AppendInt(b, int64(id), 10)
	}
	list := string(append(b, ']'))
	return func(s *sql.Selector) {
		s.Where(sql.ExprP(s.C(col)+" IN (SELECT value FROM json_each(?))", list))
	}
}

// detailInflightEstimate prices a detail response for the shared in-flight
// pool (Handler.inflightBytes). At depth < 2 the response is a bare row or
// the flat-FK-plus-ID-lists shape, and the flat TypicalRowBytes figure
// bounds it. At depth >= 2 the embedded `_set` collections have no bound:
// a hub org expands thousands of full network objects. So the flat Depth2
// figure is a floor, and each embedded set adds its element count × child
// Depth0 bytes. A failed count uses the flat figure for that set and
// returns no error. Admission is a safety net, and a count error must not
// turn a servable detail request into a 5xx (the real query shows a
// persistent DB error moments later).
func detailInflightEstimate(ctx context.Context, client *ent.Client, typeName string, id, depth int) int64 {
	est := int64(TypicalRowBytes(typeName, depth))
	if depth < 2 {
		return est
	}
	for _, cs := range childSets[typeName] {
		counts, err := cs.countByParent(ctx, client, []int{id})
		if err != nil {
			continue
		}
		est += int64(counts[id]) * int64(TypicalRowBytes(cs.childType, 0))
	}
	return est
}

// Constants of the list-depth estimate (listDepthEstimate).
const (
	// listIDBytes prices one element of a depth-1 set: one id in the
	// loaded rows and in the rendered []int, plus its JSON digits.
	listIDBytes = 16
	// listDepthIDBytes prices one id of the served id slice, with slack.
	listDepthIDBytes = 16
	// listDepthRenderFactor prices the rendered maps of one row and its
	// marshaled JSON as a multiple of the row estimate. The allocation of
	// toMap plus json.Marshal is at most 3.78 × Depth0 (campus), measured
	// on seed rows (TestListDepthRenderFactor).
	listDepthRenderFactor = 4
)

// listDepthCost is the estimate of a list at depth > 0.
type listDepthCost struct {
	// bytes is the 413 figure and the in-flight pool charge.
	bytes int64
	// chunkRows and chunkBytes describe the most expensive chunk.
	chunkRows  int
	chunkBytes int64
}

// exceeded returns the 413 diagnostics of the estimate. max_rows is the
// row count that fits the budget at the average row cost of the most
// expensive chunk, so a retry with limit=max_rows makes a chunk that
// fits. One row above the budget gives 0.
func (c listDepthCost) exceeded(count int, budget int64, entity string, depth int) BudgetExceeded {
	return BudgetExceeded{
		MaxRows:        int(budget * int64(c.chunkRows) / max(1, c.chunkBytes)),
		BudgetBytes:    budget,
		EstimatedBytes: c.bytes,
		Count:          count,
		Entity:         entity,
		Depth:          depth,
	}
}

// listDepthEstimate prices a list at depth 1 or 2 by its most expensive
// chunk of chunk served ids, as only one chunk is loaded at a time. Per
// parent p:
//
//	rowBytes(p) = Depth0(parent) + Σ_sets n(p, s) × elem(s)
//
// where elem(s) is Depth0(child) at depth 2 and listIDBytes at depth 1,
// and n comes from the countByParent of the set (the poc count applies
// the privacy policy). Per chunk c:
//
//	chunkBytes(c) = Σ_{p∈c} rowBytes(p) + listDepthRenderFactor × max_{p∈c} rowBytes(p)
//
// The result is listDepthIDBytes × len(ids) + max_c chunkBytes(c). Each
// set sends one GROUP BY query over all ids. A failed count is an error,
// not a fallback: a list can be cut with limit or a filter, so the
// handler returns 500 and does not admit a fan-out that it cannot price.
func listDepthEstimate(ctx context.Context, client *ent.Client, typeName string, ids []int, depth int, sets []childSet, chunk int) (listDepthCost, error) {
	chunk = max(chunk, 1)
	rowBytes := make([]int64, len(ids))
	base := int64(TypicalRowBytes(typeName, 0))
	for i := range rowBytes {
		rowBytes[i] = base
	}
	if len(sets) > 0 {
		pos := make(map[int]int, len(ids))
		for i, id := range ids {
			pos[id] = i
		}
		for _, cs := range sets {
			counts, err := cs.countByParent(ctx, client, ids)
			if err != nil {
				return listDepthCost{}, fmt.Errorf("count %s %s: %w", typeName, cs.key, err)
			}
			elem := int64(listIDBytes)
			if depth >= 2 {
				elem = int64(TypicalRowBytes(cs.childType, 0))
			}
			for id, n := range counts {
				if i, ok := pos[id]; ok {
					rowBytes[i] += int64(n) * elem
				}
			}
		}
	}
	var cost listDepthCost
	for start := 0; start < len(ids); start += chunk {
		end := min(start+chunk, len(ids))
		var sum, top int64
		for _, b := range rowBytes[start:end] {
			sum += b
			top = max(top, b)
		}
		if cb := sum + listDepthRenderFactor*top; cb > cost.chunkBytes {
			cost.chunkBytes = cb
			cost.chunkRows = end - start
		}
	}
	cost.bytes = listDepthIDBytes*int64(len(ids)) + cost.chunkBytes
	return cost, nil
}
