package pdbcompat

import (
	"context"
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
	"github.com/dotwaffle/peeringdb-plus/ent/poc"
)

// List depth: a list request with ?depth= above 0 adds the reverse `_set`
// fields of each row, as upstream (2.83.0 rest.py:520-523, :774-777,
// serializers.py:1240-1309). At depth 1 a set is a list of ids, and at
// depth 2 or more it is a list of flat objects. A list row never carries
// the forward FK object (org, campus, ix and so on): upstream removes
// Meta.list_exclude from every row under a list root
// (serializers.py:1286-1290) and prefetches no forward FK on a list
// (:1166-1167). Depth 3 renders the depth-2 shape (see docs/API.md
// § Known Divergences).
//
// The loaders below send ONE query per set for all the parents of a chunk,
// with the parent ids bound as one JSON array (idInJSON), and group the
// rows by the FK in Go. They do not use the ent With* eager loads: those
// bind one parameter per id. The status filter of each set is the filter
// of its countByParent in childSets, so the list estimate and the
// rendered sets agree (TestListDepth_CountsMatchRender). The ORDER BY
// starts with the FK, so the query reads the FK index, and the rest of
// the order is the order of the detail sets (see the comment on the
// facility-link sets in depth.go).

// defaultListDepthChunk is the number of parent rows that a list at
// depth > 0 loads and renders at a time. Handler.listDepthChunk holds the
// value that the handler uses.
const defaultListDepthChunk = 250

// setRender returns the value of one `_set` field for one parent of a
// chunk: a []int at depth 1, a []any of objects at depth 2.
type setRender func(parentID int) any

// setLoader loads one `_set` collection for the parents of a chunk at
// list depth 1 or 2 and returns the render function of the set.
type setLoader func(ctx context.Context, client *ent.Client, parentIDs []int, depth int) (setRender, error)

// linkRow is one row of a depth-1 set query or of a through-relation
// join: the parent FK column and the id that the set shows. ent scans the
// selected columns into the fields by name, so the struct has a field for
// each column that a set query selects. A query sets only the fields of
// its own columns.
type linkRow struct {
	ID        *int `json:"id"`
	OrgID     *int `json:"org_id"`
	NetID     *int `json:"net_id"`
	IxID      *int `json:"ix_id"`
	IxlanID   *int `json:"ixlan_id"`
	CarrierID *int `json:"carrier_id"`
	CampusID  *int `json:"campus_id"`
	FacID     *int `json:"fac_id"`
}

func linkID(r linkRow) *int         { return r.ID }
func linkOrg(r linkRow) *int        { return r.OrgID }
func linkNet(r linkRow) *int        { return r.NetID }
func linkIX(r linkRow) *int         { return r.IxID }
func linkIXLan(r linkRow) *int      { return r.IxlanID }
func linkCarrier(r linkRow) *int    { return r.CarrierID }
func linkCampus(r linkRow) *int     { return r.CampusID }
func linkFacility(r linkRow) *int   { return r.FacID }
func rowNetOrg(n *ent.Network) *int { return n.OrgID }

// linkScanner is the Scan method of an ent <Type>Select builder.
type linkScanner interface {
	Scan(ctx context.Context, v any) error
}

// scanLinks runs a select of two columns and returns its rows.
func scanLinks(ctx context.Context, s linkScanner) ([]linkRow, error) {
	var rows []linkRow
	if err := s.Scan(ctx, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// groupLinks returns the values of the rows, grouped by parent, in row
// order. A row with a NULL parent or value is left out.
func groupLinks(rows []linkRow, parent, value func(linkRow) *int) map[int][]int {
	out := make(map[int][]int)
	for _, r := range rows {
		p, v := parent(r), value(r)
		if p == nil || v == nil {
			continue
		}
		out[*p] = append(out[*p], *v)
	}
	return out
}

// idListRender returns the depth-1 render of a set: the ids of the
// parent in query order, [] when the parent has none.
func idListRender(rows []linkRow, parent, value func(linkRow) *int) setRender {
	groups := groupLinks(rows, parent, value)
	return func(p int) any {
		if ids := groups[p]; ids != nil {
			return ids
		}
		return []int{}
	}
}

// objectRender returns the depth-2 render of a set whose elements are
// the child rows: the rows of the parent in query order, rendered by
// render. A row with a NULL parent is left out.
func objectRender[E any](rows []E, parent func(E) *int, render func([]E) []any) setRender {
	groups := make(map[int][]E)
	for _, r := range rows {
		if p := parent(r); p != nil {
			groups[*p] = append(groups[*p], r)
		}
	}
	return func(p int) any { return render(groups[p]) }
}

// throughRender returns the depth-2 render of a through-relation set: one
// element per join row, in join order, resolved to its target by id.
// Duplicates stay. A join row whose target is missing is left out, as on
// the detail path.
func throughRender[T any](rows []linkRow, parent, target func(linkRow) *int, byID map[int]T, render func([]T) []any) setRender {
	groups := groupLinks(rows, parent, target)
	return func(p int) any {
		ids := groups[p]
		items := make([]T, 0, len(ids))
		for _, id := range ids {
			if t, ok := byID[id]; ok {
				items = append(items, t)
			}
		}
		return render(items)
	}
}

// distinctTargets returns the distinct non-NULL target ids of the rows.
func distinctTargets(rows []linkRow, target func(linkRow) *int) []int {
	seen := make(map[int]bool, len(rows))
	out := make([]int, 0, len(rows))
	for _, r := range rows {
		if v := target(r); v != nil && !seen[*v] {
			seen[*v] = true
			out = append(out, *v)
		}
	}
	return out
}

// Org sets: net_set, fac_set, ix_set, carrier_set and campus_set. Each
// element drops org_id (upstream nested(..., exclude=["org_id", "org"]),
// 2.83.0 serializers.py:4905-4933).

func loadOrgNetSet(ctx context.Context, client *ent.Client, ids []int, depth int) (setRender, error) {
	q := client.Network.Query().Where(idInJSON(network.FieldOrgID, ids), likelyOK).
		Order(network.ByOrgID(), network.ByID())
	if depth < 2 {
		rows, err := scanLinks(ctx, q.Select(network.FieldOrgID, network.FieldID))
		return idListRender(rows, linkOrg, linkID), err
	}
	rows, err := q.All(ctx)
	return objectRender(rows, rowNetOrg, func(g []*ent.Network) []any {
		return setWithout(networksFromEnt(g), "org_id")
	}), err
}

func loadOrgFacSet(ctx context.Context, client *ent.Client, ids []int, depth int) (setRender, error) {
	q := client.Facility.Query().Where(idInJSON(facility.FieldOrgID, ids), likelyOK).
		Order(facility.ByOrgID(), facility.ByID())
	if depth < 2 {
		rows, err := scanLinks(ctx, q.Select(facility.FieldOrgID, facility.FieldID))
		return idListRender(rows, linkOrg, linkID), err
	}
	rows, err := q.All(ctx)
	return objectRender(rows, func(f *ent.Facility) *int { return f.OrgID }, func(g []*ent.Facility) []any {
		return setWithout(facilitiesFromEnt(g), "org_id")
	}), err
}

func loadOrgIXSet(ctx context.Context, client *ent.Client, ids []int, depth int) (setRender, error) {
	q := client.InternetExchange.Query().Where(idInJSON(internetexchange.FieldOrgID, ids), likelyOK).
		Order(internetexchange.ByOrgID(), internetexchange.ByID())
	if depth < 2 {
		rows, err := scanLinks(ctx, q.Select(internetexchange.FieldOrgID, internetexchange.FieldID))
		return idListRender(rows, linkOrg, linkID), err
	}
	rows, err := q.All(ctx)
	return objectRender(rows, func(x *ent.InternetExchange) *int { return x.OrgID }, func(g []*ent.InternetExchange) []any {
		return setWithout(internetExchangesFromEnt(g), "org_id")
	}), err
}

func loadOrgCarrierSet(ctx context.Context, client *ent.Client, ids []int, depth int) (setRender, error) {
	q := client.Carrier.Query().Where(idInJSON(carrier.FieldOrgID, ids), likelyOK).
		Order(carrier.ByOrgID(), carrier.ByID())
	if depth < 2 {
		rows, err := scanLinks(ctx, q.Select(carrier.FieldOrgID, carrier.FieldID))
		return idListRender(rows, linkOrg, linkID), err
	}
	rows, err := q.All(ctx)
	return objectRender(rows, func(c *ent.Carrier) *int { return c.OrgID }, func(g []*ent.Carrier) []any {
		return setWithout(carriersFromEnt(g), "org_id")
	}), err
}

func loadOrgCampusSet(ctx context.Context, client *ent.Client, ids []int, depth int) (setRender, error) {
	q := client.Campus.Query().Where(idInJSON(campus.FieldOrgID, ids), likelyOK).
		Order(campus.ByOrgID(), campus.ByID())
	if depth < 2 {
		rows, err := scanLinks(ctx, q.Select(campus.FieldOrgID, campus.FieldID))
		return idListRender(rows, linkOrg, linkID), err
	}
	rows, err := q.All(ctx)
	return objectRender(rows, func(c *ent.Campus) *int { return c.OrgID }, func(g []*ent.Campus) []any {
		return setWithout(campusesFromEnt(g), "org_id")
	}), err
}

// Net sets: poc_set, netfac_set and netixlan_set. Each element drops
// net_id (2.83.0 serializers.py:3491-3507). The poc query runs through
// PocQuery, so the poc privacy policy removes the contacts that the
// caller's tier cannot read, from the ids and from the objects.

func loadNetPocSet(ctx context.Context, client *ent.Client, ids []int, depth int) (setRender, error) {
	q := client.Poc.Query().Where(idInJSON(poc.FieldNetID, ids), likelyOK).
		Order(poc.ByNetID(), poc.ByID())
	if depth < 2 {
		rows, err := scanLinks(ctx, q.Select(poc.FieldNetID, poc.FieldID))
		return idListRender(rows, linkNet, linkID), err
	}
	rows, err := q.All(ctx)
	return objectRender(rows, func(p *ent.Poc) *int { return p.NetID }, func(g []*ent.Poc) []any {
		return setWithout(pocsFromEnt(g), "net_id")
	}), err
}

func loadNetNetFacSet(ctx context.Context, client *ent.Client, ids []int, depth int) (setRender, error) {
	q := client.NetworkFacility.Query().Where(idInJSON(networkfacility.FieldNetID, ids), likelyOK).
		Order(networkfacility.ByNetID(), networkfacility.ByFacID(), networkfacility.ByID())
	if depth < 2 {
		rows, err := scanLinks(ctx, q.Select(networkfacility.FieldNetID, networkfacility.FieldID))
		return idListRender(rows, linkNet, linkID), err
	}
	rows, err := q.All(ctx)
	return objectRender(rows, func(n *ent.NetworkFacility) *int { return n.NetID }, func(g []*ent.NetworkFacility) []any {
		return setWithout(networkFacilitiesFromEnt(g), "net_id")
	}), err
}

func loadNetNetIXLanSet(ctx context.Context, client *ent.Client, ids []int, depth int) (setRender, error) {
	q := client.NetworkIxLan.Query().Where(idInJSON(networkixlan.FieldNetID, ids), likelyNetIXLanSet).
		Order(networkixlan.ByNetID(), networkixlan.ByID())
	if depth < 2 {
		rows, err := scanLinks(ctx, q.Select(networkixlan.FieldNetID, networkixlan.FieldID))
		return idListRender(rows, linkNet, linkID), err
	}
	rows, err := q.All(ctx)
	return objectRender(rows, func(n *ent.NetworkIxLan) *int { return n.NetID }, func(g []*ent.NetworkIxLan) []any {
		return setWithout(networkIxLansFromEnt(g), "net_id")
	}), err
}

// IX sets: ixlan_set (drops ix_id) and fac_set, the facilities reached
// through the ixfac join (2.83.0 serializers.py:4362-4370). The ixlan
// objects go through ixLansFromEnt, which redacts
// ixf_ixp_member_list_url for the caller's tier.

func loadIXIXLanSet(ctx context.Context, client *ent.Client, ids []int, depth int) (setRender, error) {
	q := client.IxLan.Query().Where(idInJSON(ixlan.FieldIxID, ids), likelyOK).
		Order(ixlan.ByIxID(), ixlan.ByID())
	if depth < 2 {
		rows, err := scanLinks(ctx, q.Select(ixlan.FieldIxID, ixlan.FieldID))
		return idListRender(rows, linkIX, linkID), err
	}
	rows, err := q.All(ctx)
	return objectRender(rows, func(l *ent.IxLan) *int { return l.IxID }, func(g []*ent.IxLan) []any {
		return setWithout(ixLansFromEnt(ctx, g), "ix_id")
	}), err
}

func loadIXFacSet(ctx context.Context, client *ent.Client, ids []int, depth int) (setRender, error) {
	rows, err := scanLinks(ctx, client.IxFacility.Query().
		Where(idInJSON(ixfacility.FieldIxID, ids), likelyOK).
		Order(ixfacility.ByIxID(), ixfacility.ByFacID(), ixfacility.ByID()).
		Select(ixfacility.FieldIxID, ixfacility.FieldFacID))
	if err != nil {
		return nil, err
	}
	if depth < 2 {
		return idListRender(rows, linkIX, linkFacility), nil
	}
	// The join row decides membership. The facility is not filtered
	// (2.83.0 serializers.py:1678-1681).
	facs, err := client.Facility.Query().
		Where(idInJSON(facility.FieldID, distinctTargets(rows, linkFacility))).All(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[int]*ent.Facility, len(facs))
	for _, f := range facs {
		byID[f.ID] = f
	}
	return throughRender(rows, linkIX, linkFacility, byID, func(g []*ent.Facility) []any {
		return orEmptySlice(facilitiesFromEnt(g))
	}), nil
}

// IXLan sets: ixpfx_set (drops ixlan_id) and net_set, the networks
// reached through the netixlan join, one per join row with duplicates
// (2.83.0 serializers.py:4252-4262).

func loadIXLanIXPfxSet(ctx context.Context, client *ent.Client, ids []int, depth int) (setRender, error) {
	q := client.IxPrefix.Query().Where(idInJSON(ixprefix.FieldIxlanID, ids), likelyOK).
		Order(ixprefix.ByIxlanID(), ixprefix.ByID())
	if depth < 2 {
		rows, err := scanLinks(ctx, q.Select(ixprefix.FieldIxlanID, ixprefix.FieldID))
		return idListRender(rows, linkIXLan, linkID), err
	}
	rows, err := q.All(ctx)
	return objectRender(rows, func(p *ent.IxPrefix) *int { return p.IxlanID }, func(g []*ent.IxPrefix) []any {
		return setWithout(ixPrefixesFromEnt(g), "ixlan_id")
	}), err
}

func loadIXLanNetSet(ctx context.Context, client *ent.Client, ids []int, depth int) (setRender, error) {
	rows, err := scanLinks(ctx, client.NetworkIxLan.Query().
		Where(idInJSON(networkixlan.FieldIxlanID, ids), likelyNetIXLanSet).
		Order(networkixlan.ByIxlanID(), networkixlan.ByID()).
		Select(networkixlan.FieldIxlanID, networkixlan.FieldNetID))
	if err != nil {
		return nil, err
	}
	if depth < 2 {
		return idListRender(rows, linkIXLan, linkNet), nil
	}
	// The join row decides membership. The network is not filtered
	// (2.83.0 serializers.py:1678-1681).
	nets, err := client.Network.Query().
		Where(idInJSON(network.FieldID, distinctTargets(rows, linkNet))).All(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[int]*ent.Network, len(nets))
	for _, n := range nets {
		byID[n.ID] = n
	}
	return throughRender(rows, linkIXLan, linkNet, byID, func(g []*ent.Network) []any {
		return orEmptySlice(networksFromEnt(g))
	}), nil
}

// loadCarrierCarrierFacSet loads carrier.carrierfac_set in facility
// order. The elements keep carrier_id (2.83.0 serializers.py:2658-2662
// excludes only the fac object).
func loadCarrierCarrierFacSet(ctx context.Context, client *ent.Client, ids []int, depth int) (setRender, error) {
	q := client.CarrierFacility.Query().Where(idInJSON(carrierfacility.FieldCarrierID, ids), likelyOK).
		Order(carrierfacility.ByCarrierID(), carrierfacility.ByFacID(), carrierfacility.ByID())
	if depth < 2 {
		rows, err := scanLinks(ctx, q.Select(carrierfacility.FieldCarrierID, carrierfacility.FieldID))
		return idListRender(rows, linkCarrier, linkID), err
	}
	rows, err := q.All(ctx)
	return objectRender(rows, func(c *ent.CarrierFacility) *int { return c.CarrierID }, func(g []*ent.CarrierFacility) []any {
		return setWithout(carrierFacilitiesFromEnt(g))
	}), err
}

// loadCampusFacSet loads campus.fac_set. The elements keep campus_id and
// drop org_id (2.83.0 serializers.py:4784-4788).
func loadCampusFacSet(ctx context.Context, client *ent.Client, ids []int, depth int) (setRender, error) {
	q := client.Facility.Query().Where(idInJSON(facility.FieldCampusID, ids), likelyOK).
		Order(facility.ByCampusID(), facility.ByID())
	if depth < 2 {
		rows, err := scanLinks(ctx, q.Select(facility.FieldCampusID, facility.FieldID))
		return idListRender(rows, linkCampus, linkID), err
	}
	rows, err := q.All(ctx)
	return objectRender(rows, func(f *ent.Facility) *int { return f.CampusID }, func(g []*ent.Facility) []any {
		return setWithout(facilitiesFromEnt(g), "org_id")
	}), err
}

// selectSets returns the sets of typeName that a list at depth > 0 loads:
// every set without ?fields=, else only the sets that fields names (the
// names are trimmed as in applyFieldProjection). Upstream removes every
// field that fields does not name (2.83.0 serializers.py:942-950).
func selectSets(typeName string, fields []string) []childSet {
	all := childSets[typeName]
	if len(fields) == 0 {
		return all
	}
	want := make(map[string]bool, len(fields))
	for _, f := range fields {
		want[strings.TrimSpace(f)] = true
	}
	out := make([]childSet, 0, len(all))
	for _, cs := range all {
		if want[cs.key] {
			out = append(out, cs)
		}
	}
	return out
}

// renderListDepthChunk loads the selected sets of one chunk of parents
// and returns one render closure per parent, in the order of parentIDs.
// convert renders the parent row. A closure builds the row map when the
// stream pulls it.
func renderListDepthChunk[E any](ctx context.Context, client *ent.Client, parents []E, parentIDs []int, depth int, sets []childSet, convert func(E) any) ([]func() any, error) {
	renders := make([]setRender, len(sets))
	for i, cs := range sets {
		r, err := cs.load(ctx, client, parentIDs, depth)
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", cs.key, err)
		}
		renders[i] = r
	}
	out := make([]func() any, len(parents))
	for i, p := range parents {
		id := parentIDs[i]
		out[i] = func() any {
			m := toMap(convert(p))
			for j, cs := range sets {
				m[cs.key] = renders[j](id)
			}
			return m
		}
	}
	return out, nil
}
