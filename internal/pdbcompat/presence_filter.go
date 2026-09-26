package pdbcompat

import (
	"fmt"
	"strconv"
	"strings"

	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent/ixlan"
	"github.com/dotwaffle/peeringdb-plus/ent/networkfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/networkixlan"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
)

// This file ports the presence keys that an upstream serializer handles
// in prepare_query (PeeringDB 2.83.0): net not_ix and not_fac, and fac
// and ix not_net, all_net, org_present and org_not_present. Each key
// keeps or excludes the listed rows by their links to the networks,
// exchanges, facilities or organizations whose ids the value gives.
// Only the exact key is a presence key: upstream tests `"not_ix" in
// kwargs`, so not_ix__in is an unknown key.

// presenceKey describes one presence key of a type.
type presenceKey struct {
	// list is true when upstream splits the value at commas. Otherwise
	// the value is one id.
	list bool
	// build returns the predicate on the listed row for the ids of the
	// value.
	build func(tc TypeConfig, ids []int, tier privctx.Tier) (func(*sql.Selector), error)
}

// presenceKeys maps a type to its presence keys.
var presenceKeys = map[string]map[string]presenceKey{
	// NetworkSerializer.prepare_query (serializers.py:3750-3760):
	// not_related_to_ix and not_related_to_fac (models.py:5603-5616,
	// :5683-5697) exclude the networks that net?ix= and net?fac= keep.
	peeringdb.TypeNet: {
		"not_ix":  {build: notRelated("ix")},
		"not_fac": {build: notRelated("fac")},
	},
	// FacilitySerializer.prepare_query (serializers.py:2131-2201).
	peeringdb.TypeFac: {
		"org_present":     {list: true, build: orgPresence(false, facOrgPaths)},
		"org_not_present": {list: true, build: orgPresence(true, facOrgPaths)},
		"all_net":         {list: true, build: allFacNets},
		"not_net":         {list: true, build: notRelated("net")},
	},
	// InternetExchangeSerializer.prepare_query (serializers.py:4559-4629).
	peeringdb.TypeIX: {
		"all_net":         {list: true, build: allIXNets},
		"not_net":         {list: true, build: notRelated("net")},
		"org_present":     {list: true, build: orgPresence(false, ixOrgPaths)},
		"org_not_present": {list: true, build: orgPresence(true, ixOrgPaths)},
	},
}

// lookupPresenceKey reports whether key is a presence key of typ.
func lookupPresenceKey(typ, key string) (presenceKey, bool) {
	pk, ok := presenceKeys[typ][key]
	return pk, ok
}

// buildPresencePredicate parses value and builds the predicate of pk.
// Every item must be an integer: upstream converts the items with int()
// or passes them to an integer lookup, and the ValueError of any other
// item is a 400 (2.83.0 rest.py:488-500). pyInt accepts the forms that
// int() accepts, for example "1_00" and Unicode digits. An empty value
// is not an integer either.
func buildPresencePredicate(tc TypeConfig, pk presenceKey, value string, tier privctx.Tier) (func(*sql.Selector), error) {
	items := []string{value}
	if pk.list {
		items = strings.Split(value, ",")
	}
	ids := make([]int, len(items))
	for i, item := range items {
		id, _, err := pyInt(item)
		if err != nil {
			return nil, fmt.Errorf("%q is not an integer", item)
		}
		ids[i] = id
	}
	return pk.build(tc, ids, tier)
}

// joinIDs returns ids as a comma-separated list, the value form of an
// __in filter.
func joinIDs(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.Itoa(id)
	}
	return strings.Join(parts, ",")
}

// notRelated returns the builder of a key that excludes the rows that
// the relation seed of the type keeps for any of the ids. The seed pins
// the link row to status "ok", as make_relation_filter does in the
// not_related_to_<x> methods (models.py:2349-2363, :2419-2433,
// :2814-2828).
func notRelated(seed string) func(TypeConfig, []int, privctx.Tier) (func(*sql.Selector), error) {
	return func(tc TypeConfig, ids []int, tier privctx.Tier) (func(*sql.Selector), error) {
		sd, ok := relationSeeds[tc.Name][seed]
		if !ok {
			return nil, fmt.Errorf("no relation seed %q on %s", seed, tc.Name)
		}
		p, ok, _, err := buildRelationSeedPredicate(tc, sd, []string{"in"}, joinIDs(ids), tier)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("relation seed %q on %s does not resolve", seed, tc.Name)
		}
		return sql.NotPredicates(p), nil
	}
}

// allFacNets keeps the facilities that have a netfac with status "ok"
// for every listed network. related_to_multiple_networks intersects one
// related_to_net query per id (models.py:2366-2399). The mirror counts
// the distinct networks of each facility in one query, so a long list
// adds no SQL term per id.
func allFacNets(_ TypeConfig, ids []int, _ privctx.Tier) (func(*sql.Selector), error) {
	inNets, err := buildPredicate(networkfacility.FieldNetID, "in", joinIDs(ids), FieldInt, false)
	if err != nil {
		return nil, err
	}
	want := distinctCount(ids)
	return func(s *sql.Selector) {
		nf := sql.Table(networkfacility.Table)
		sub := sql.Select(nf.C(networkfacility.FieldFacID)).From(nf)
		inNets(sub)
		likelyOK(sub)
		sub.GroupBy(nf.C(networkfacility.FieldFacID)).
			Having(sql.ExprP("COUNT(DISTINCT "+nf.C(networkfacility.FieldNetID)+") = ?", want))
		s.Where(sql.In(s.C(parentPKColumn), sub))
	}, nil
}

// allIXNets keeps the exchanges that have a netixlan with status "ok"
// for every listed network, through the ixlan of the netixlan
// (models.py:2777-2811). The ixlan row has no status check.
func allIXNets(_ TypeConfig, ids []int, _ privctx.Tier) (func(*sql.Selector), error) {
	inNets, err := buildPredicate(networkixlan.FieldNetID, "in", joinIDs(ids), FieldInt, false)
	if err != nil {
		return nil, err
	}
	want := distinctCount(ids)
	return func(s *sql.Selector) {
		nixl := sql.Table(networkixlan.Table)
		lan := sql.Table(ixlan.Table)
		sub := sql.Select().From(nixl)
		// Join gives lan an alias, so lan.C must run after it.
		sub.Join(lan).On(nixl.C(networkixlan.FieldIxlanID), lan.C(ixlan.FieldID))
		sub.Select(lan.C(ixlan.FieldIxID))
		inNets(sub)
		likelyOK(sub)
		sub.GroupBy(lan.C(ixlan.FieldIxID)).
			Having(sql.ExprP("COUNT(DISTINCT "+nixl.C(networkixlan.FieldNetID)+") = ?", want))
		s.Where(sql.In(s.C(parentPKColumn), sub))
	}, nil
}

// distinctCount returns the number of distinct values in ids.
func distinctCount(ids []int) int {
	seen := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		seen[id] = struct{}{}
	}
	return len(seen)
}

// orgPath is one way from the listed row to the rows whose org_id an
// org_present key compares: a list of traversal hops, each an edge of
// Edges, from a start type.
type orgPath struct {
	from string
	hops []string
}

// facOrgPaths are the links of org_present on fac: the networks of the
// netfacs and the exchanges of the ixfacs (serializers.py:2131-2157).
var facOrgPaths = []orgPath{
	{from: peeringdb.TypeFac, hops: []string{"netfac", "net"}},
	{from: peeringdb.TypeFac, hops: []string{"ixfac", "ix"}},
}

// ixOrgPaths are the links of org_present on ix: the networks of the
// netixlans and the facilities of the ixfacs (serializers.py:4575-4601).
// Upstream takes the ixlan_id of each netixlan as the exchange id, so
// the first path starts at the ixlan type: its netixlan edge compares
// the id of the listed exchange with netixlan.ixlan_id. Every stored
// ixlan has the id of its exchange, so this is also the path through
// the ixlan.
var ixOrgPaths = []orgPath{
	{from: peeringdb.TypeIXLan, hops: []string{"netixlan", "net"}},
	{from: peeringdb.TypeIX, hops: []string{"ixfac", "fac"}},
}

// orgPresence returns the builder of org_present (negate false) or
// org_not_present (negate true). The key keeps, or excludes, the rows
// that a path links to a row whose org_id is in the list. Upstream reads
// the links with the objects manager (serializers.py:2131-2185,
// :4575-4629), so no row of a path has a status check: deleted links and
// deleted networks, exchanges and facilities count.
func orgPresence(negate bool, paths []orgPath) func(TypeConfig, []int, privctx.Tier) (func(*sql.Selector), error) {
	return func(_ TypeConfig, ids []int, tier privctx.Tier) (func(*sql.Selector), error) {
		preds := make([]func(*sql.Selector), len(paths))
		for i, path := range paths {
			edges, err := lookupHops(path.from, path.hops)
			if err != nil {
				return nil, err
			}
			leaf, err := buildPredicate("org_id", "in", joinIDs(ids), FieldInt, false)
			if err != nil {
				return nil, err
			}
			preds[i] = relationPathPredicate(edges, noPin, leaf, tier)
		}
		p := sql.OrPredicates(preds...)
		if negate {
			return sql.NotPredicates(p), nil
		}
		return p, nil
	}
}

// lookupHops returns the edges of hops from type from.
func lookupHops(from string, hops []string) ([]EdgeMetadata, error) {
	edges := make([]EdgeMetadata, len(hops))
	rowType := from
	for i, hop := range hops {
		e, ok := LookupEdge(rowType, hop)
		if !ok {
			return nil, fmt.Errorf("no edge %q on %s", hop, rowType)
		}
		edges[i] = e
		rowType = e.TargetType
	}
	return edges, nil
}
