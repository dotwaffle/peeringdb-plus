package pdbcompat

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent/ixlan"
	"github.com/dotwaffle/peeringdb-plus/ent/network"
	"github.com/dotwaffle/peeringdb-plus/ent/networkfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/networkixlan"
	"github.com/dotwaffle/peeringdb-plus/internal/pdbtypes"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
)

// This file ports the presence keys that an upstream serializer handles
// in prepare_query (PeeringDB 2.83.0): net not_ix and not_fac, and fac
// and ix not_net, all_net, asn_overlap, org_present and org_not_present.
// Each key keeps or excludes the listed rows by their links to the
// networks, exchanges, facilities or organizations whose ids (for
// asn_overlap: ASNs) the value gives.
// Only the exact key is a presence key: upstream tests `"not_ix" in
// kwargs`, so not_ix__in is an unknown key.

// presenceKey describes one presence key of a type.
type presenceKey struct {
	// list is true when upstream splits the value at commas. Otherwise
	// the value is one id.
	list bool
	// parse, when set, replaces parseItems. It gets the items of the
	// value and returns the ids for build, or none = true when no row
	// can match.
	parse func(items []string) (ids []int, none bool, err error)
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
	// FacilitySerializer.prepare_query (serializers.py:2126-2201).
	peeringdb.TypeFac: {
		"asn_overlap":     {list: true, parse: parseASNOverlap, build: asnOverlapFac},
		"org_present":     {list: true, build: orgPresence(false, facOrgPaths)},
		"org_not_present": {list: true, build: orgPresence(true, facOrgPaths)},
		"all_net":         {list: true, build: allFacNets},
		"not_net":         {list: true, build: notRelated("net")},
	},
	// InternetExchangeSerializer.prepare_query (serializers.py:4554-4629).
	peeringdb.TypeIX: {
		"asn_overlap":     {list: true, parse: parseASNOverlap, build: asnOverlapIX},
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
// A list value splits at every comma, as Python str.split(",") does: an
// empty item stays. The parse hook of pk, or parseItems, reads the
// items. When no row can match, the predicate is a constant false, not
// an empty result, so that the other keys of the request are still
// parsed and their errors still return 400.
func buildPresencePredicate(tc TypeConfig, pk presenceKey, value string, tier privctx.Tier) (func(*sql.Selector), error) {
	items := []string{value}
	if pk.list {
		items = strings.Split(value, ",")
	}
	parse := pk.parse
	if parse == nil {
		parse = parseItems
	}
	ids, none, err := parse(items)
	if err != nil {
		return nil, err
	}
	if none {
		return matchNone, nil
	}
	return pk.build(tc, ids, tier)
}

// parseItems converts every item to an integer. Upstream converts the
// items with int() or passes them to an integer lookup, and the
// ValueError of any other item is a 400 (2.83.0 rest.py:488-500). pyInt
// accepts the forms that int() accepts, for example "1_00" and Unicode
// digits. An empty item is not an integer either.
func parseItems(items []string) ([]int, bool, error) {
	ids := make([]int, len(items))
	for i, item := range items {
		id, _, err := pyInt(item)
		if err != nil {
			return nil, false, fmt.Errorf("%q is not an integer", item)
		}
		ids[i] = id
	}
	return ids, false, nil
}

// matchNone is the predicate of a presence key that no row can match.
func matchNone(s *sql.Selector) { s.Where(sql.False()) }

// maxASNOverlap is the largest ASN list that overlapping_asns accepts
// (2.83.0 models.py:2460-2461, :2870-2871).
const maxASNOverlap = 25

// The errors of overlapping_asns (2.83.0 models.py:2457-2461,
// :2867-2871).
var (
	errASNOverlapTooFew  = errors.New("Need to specify at least two asns")     //nolint:staticcheck // exact upstream message text
	errASNOverlapTooMany = errors.New("Can only compare a maximum of 25 asns") //nolint:staticcheck // exact upstream message text
)

// parseASNOverlap parses the items of asn_overlap as overlapping_asns
// does (2.83.0 models.py:2436-2483, :2846-2893). The item count is
// checked before any item is converted, so asn_overlap=abc is the
// too-few error. Upstream keys the ASNs of each row by the raw item and
// compares the key count with the item count, so an item that occurs
// two times matches no row. Two different items for the same ASN (for
// example "64500" and " 64500") count as one ASN. An ASN above the
// integer range saturates and matches no network, as the Django
// IntegerFieldOverflow lookup matches no row upstream.
func parseASNOverlap(items []string) ([]int, bool, error) {
	switch {
	case len(items) == 1:
		return nil, false, errASNOverlapTooFew
	case len(items) > maxASNOverlap:
		return nil, false, errASNOverlapTooMany
	}
	asns, _, err := parseItems(items)
	if err != nil {
		return nil, false, err
	}
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if _, dup := seen[item]; dup {
			return nil, true, nil
		}
		seen[item] = struct{}{}
	}
	return asns, false, nil
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
	return facsWithEveryNet(inNets, likelyOK, distinctCount(ids)), nil
}

// allIXNets keeps the exchanges that have a netixlan with status "ok"
// for every listed network, through the ixlan of the netixlan
// (models.py:2777-2811). The ixlan row has no status check.
func allIXNets(_ TypeConfig, ids []int, _ privctx.Tier) (func(*sql.Selector), error) {
	inNets, err := buildPredicate(networkixlan.FieldNetID, "in", joinIDs(ids), FieldInt, false)
	if err != nil {
		return nil, err
	}
	return ixsWithEveryNet(inNets, likelyOK, distinctCount(ids)), nil
}

// asnOverlapFac keeps the facilities that have a netfac with status
// "ok" for the network of every listed ASN (models.py:2464-2471).
func asnOverlapFac(_ TypeConfig, asns []int, _ privctx.Tier) (func(*sql.Selector), error) {
	inNets, err := netsWithASN(asns)
	if err != nil {
		return nil, err
	}
	return facsWithEveryNet(inNets, likelyOK, distinctCount(asns)), nil
}

// asnOverlapIX keeps the exchanges that have a netixlan with a live
// status, "ok" or "not-operational", for the network of every listed
// ASN, through the ixlan of the netixlan (models.py:2875-2881,
// live_statuses :109-122).
func asnOverlapIX(_ TypeConfig, asns []int, _ privctx.Tier) (func(*sql.Selector), error) {
	inNets, err := netsWithASN(asns)
	if err != nil {
		return nil, err
	}
	live := likelyStatusIn(pdbtypes.LiveStatuses(peeringdb.TypeNetIXLan))
	return ixsWithEveryNet(inNets, live, distinctCount(asns)), nil
}

// linkNetIDColumn is the network FK column of netfac and netixlan.
const linkNetIDColumn = "net_id"

// netsWithASN selects the link rows whose network has one of asns:
// net_id IN (SELECT id FROM networks WHERE asn IN json_each(?)).
// Upstream matches network__asn, not the netfac local_asn or the
// netixlan asn, and checks no status on the network. asn is unique, so
// each ASN names at most one network, and COUNT(DISTINCT net_id) counts
// the matched ASNs.
func netsWithASN(asns []int) (func(*sql.Selector), error) {
	asnIn, err := buildPredicate(network.FieldAsn, "in", joinIDs(asns), FieldInt, false)
	if err != nil {
		return nil, err
	}
	return func(s *sql.Selector) {
		n := sql.Table(network.Table)
		nets := sql.Select(n.C(network.FieldID)).From(n)
		asnIn(nets)
		s.Where(sql.In(s.C(linkNetIDColumn), nets))
	}, nil
}

// facsWithEveryNet keeps the facilities that have a netfac, with a
// status that status admits, for each of the want networks that inNets
// selects. The status test must use likely(), or SQLite reads every
// such netfac through a status index (TestPresencePlan_KeepsNetIndex).
func facsWithEveryNet(inNets, status func(*sql.Selector), want int) func(*sql.Selector) {
	return func(s *sql.Selector) {
		nf := sql.Table(networkfacility.Table)
		sub := sql.Select(nf.C(networkfacility.FieldFacID)).From(nf)
		inNets(sub)
		status(sub)
		sub.GroupBy(nf.C(networkfacility.FieldFacID)).
			Having(sql.ExprP("COUNT(DISTINCT "+nf.C(networkfacility.FieldNetID)+") = ?", want))
		s.Where(sql.In(s.C(parentPKColumn), sub))
	}
}

// ixsWithEveryNet is facsWithEveryNet for exchanges, through the ixlan
// of each netixlan. The ixlan row has no status check.
func ixsWithEveryNet(inNets, status func(*sql.Selector), want int) func(*sql.Selector) {
	return func(s *sql.Selector) {
		nixl := sql.Table(networkixlan.Table)
		lan := sql.Table(ixlan.Table)
		sub := sql.Select().From(nixl)
		// Join gives lan an alias, so lan.C must run after it.
		sub.Join(lan).On(nixl.C(networkixlan.FieldIxlanID), lan.C(ixlan.FieldID))
		sub.Select(lan.C(ixlan.FieldIxID))
		inNets(sub)
		status(sub)
		sub.GroupBy(lan.C(ixlan.FieldIxID)).
			Having(sql.ExprP("COUNT(DISTINCT "+nixl.C(networkixlan.FieldNetID)+") = ?", want))
		s.Where(sql.In(s.C(parentPKColumn), sub))
	}
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
