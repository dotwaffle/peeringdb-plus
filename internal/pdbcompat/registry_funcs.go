package pdbcompat

import (
	"context"
	"fmt"
	"slices"

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
	"github.com/dotwaffle/peeringdb-plus/ent/organization"
	"github.com/dotwaffle/peeringdb-plus/ent/poc"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	"github.com/dotwaffle/peeringdb-plus/internal/pdbtypes"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

func init() {
	wireEntity(entityWiring[*ent.OrganizationQuery, predicate.Organization, organization.OrderOption, *ent.Organization]{
		name:    peeringdb.TypeOrg,
		plural:  "organizations",
		query:   func(c *ent.Client) *ent.OrganizationQuery { return c.Organization.Query() },
		convert: func(_ context.Context, o *ent.Organization) any { return organizationFromEnt(o) },
		get:     getOrgWithDepth,
		id:      func(o *ent.Organization) int { return o.ID },
	})
	wireEntity(entityWiring[*ent.NetworkQuery, predicate.Network, network.OrderOption, *ent.Network]{
		name:    peeringdb.TypeNet,
		plural:  "networks",
		query:   func(c *ent.Client) *ent.NetworkQuery { return c.Network.Query() },
		convert: func(_ context.Context, n *ent.Network) any { return networkFromEnt(n) },
		get:     getNetWithDepth,
		id:      func(n *ent.Network) int { return n.ID },
	})
	wireEntity(entityWiring[*ent.FacilityQuery, predicate.Facility, facility.OrderOption, *ent.Facility]{
		name:    peeringdb.TypeFac,
		plural:  "facilities",
		query:   func(c *ent.Client) *ent.FacilityQuery { return c.Facility.Query() },
		convert: func(_ context.Context, f *ent.Facility) any { return facilityFromEnt(f) },
		get:     getFacWithDepth,
	})
	wireEntity(entityWiring[*ent.InternetExchangeQuery, predicate.InternetExchange, internetexchange.OrderOption, *ent.InternetExchange]{
		name:    peeringdb.TypeIX,
		plural:  "internet exchanges",
		query:   func(c *ent.Client) *ent.InternetExchangeQuery { return c.InternetExchange.Query() },
		convert: func(_ context.Context, ix *ent.InternetExchange) any { return internetExchangeFromEnt(ix) },
		get:     getIXWithDepth,
		id:      func(ix *ent.InternetExchange) int { return ix.ID },
	})
	wireEntity(entityWiring[*ent.PocQuery, predicate.Poc, poc.OrderOption, *ent.Poc]{
		name:    peeringdb.TypePoc,
		plural:  "pocs",
		query:   func(c *ent.Client) *ent.PocQuery { return c.Poc.Query() },
		convert: func(_ context.Context, p *ent.Poc) any { return pocFromEnt(p) },
		get:     getPocWithDepth,
	})
	wireEntity(entityWiring[*ent.IxLanQuery, predicate.IxLan, ixlan.OrderOption, *ent.IxLan]{
		name:   peeringdb.TypeIXLan,
		plural: "ixlans",
		query:  func(c *ent.Client) *ent.IxLanQuery { return c.IxLan.Query() },
		// ixLanFromEnt is the only ctx-aware serializer: it redacts
		// ixf_ixp_member_list_url per the caller's privacy tier.
		convert: func(ctx context.Context, l *ent.IxLan) any { return ixLanFromEnt(ctx, l) },
		get:     getIXLanWithDepth,
		id:      func(l *ent.IxLan) int { return l.ID },
	})
	wireEntity(entityWiring[*ent.IxPrefixQuery, predicate.IxPrefix, ixprefix.OrderOption, *ent.IxPrefix]{
		name:    peeringdb.TypeIXPfx,
		plural:  "ixprefixes",
		query:   func(c *ent.Client) *ent.IxPrefixQuery { return c.IxPrefix.Query() },
		convert: func(_ context.Context, p *ent.IxPrefix) any { return ixPrefixFromEnt(p) },
		get:     getIXPfxWithDepth,
	})
	wireEntity(entityWiring[*ent.NetworkIxLanQuery, predicate.NetworkIxLan, networkixlan.OrderOption, *ent.NetworkIxLan]{
		name:    peeringdb.TypeNetIXLan,
		plural:  "networkixlans",
		query:   func(c *ent.Client) *ent.NetworkIxLanQuery { return c.NetworkIxLan.Query() },
		convert: func(_ context.Context, n *ent.NetworkIxLan) any { return networkIxLanFromEnt(n) },
		get:     getNetIXLanWithDepth,
	})
	wireEntity(entityWiring[*ent.NetworkFacilityQuery, predicate.NetworkFacility, networkfacility.OrderOption, *ent.NetworkFacility]{
		name:    peeringdb.TypeNetFac,
		plural:  "networkfacilities",
		query:   func(c *ent.Client) *ent.NetworkFacilityQuery { return c.NetworkFacility.Query() },
		convert: func(_ context.Context, n *ent.NetworkFacility) any { return networkFacilityFromEnt(n) },
		get:     getNetFacWithDepth,
	})
	wireEntity(entityWiring[*ent.IxFacilityQuery, predicate.IxFacility, ixfacility.OrderOption, *ent.IxFacility]{
		name:    peeringdb.TypeIXFac,
		plural:  "ixfacilities",
		query:   func(c *ent.Client) *ent.IxFacilityQuery { return c.IxFacility.Query() },
		convert: func(_ context.Context, f *ent.IxFacility) any { return ixFacilityFromEnt(f) },
		get:     getIXFacWithDepth,
	})
	wireEntity(entityWiring[*ent.CarrierQuery, predicate.Carrier, carrier.OrderOption, *ent.Carrier]{
		name:    peeringdb.TypeCarrier,
		plural:  "carriers",
		query:   func(c *ent.Client) *ent.CarrierQuery { return c.Carrier.Query() },
		convert: func(_ context.Context, cr *ent.Carrier) any { return carrierFromEnt(cr) },
		get:     getCarrierWithDepth,
		id:      func(cr *ent.Carrier) int { return cr.ID },
	})
	wireEntity(entityWiring[*ent.CarrierFacilityQuery, predicate.CarrierFacility, carrierfacility.OrderOption, *ent.CarrierFacility]{
		name:    peeringdb.TypeCarrierFac,
		plural:  "carrierfacilities",
		query:   func(c *ent.Client) *ent.CarrierFacilityQuery { return c.CarrierFacility.Query() },
		convert: func(_ context.Context, cf *ent.CarrierFacility) any { return carrierFacilityFromEnt(cf) },
		get:     getCarrierFacWithDepth,
	})
	wireEntity(entityWiring[*ent.CampusQuery, predicate.Campus, campus.OrderOption, *ent.Campus]{
		name:   peeringdb.TypeCampus,
		plural: "campuses",
		// Campus is the only type that admits status=pending on
		// list+since (2.83.0 rest.py:725-735).
		isCampus: true,
		query:    func(c *ent.Client) *ent.CampusQuery { return c.Campus.Query() },
		convert:  func(_ context.Context, cp *ent.Campus) any { return campusFromEnt(cp) },
		get:      getCampusWithDepth,
		id:       func(cp *ent.Campus) int { return cp.ID },
	})

	// List/count pairing invariant: every entity that exposes a List closure
	// MUST also expose a Count closure. serveList's pre-flight budget
	// check in handler.go is gated on `tc.Count != nil` — a missing Count
	// would silently bypass the 413 guardrail and put the process back at
	// OOM risk on an unbounded list. wireEntity registers both halves from
	// one wiring entry, so this can only trip if a Registry entry is
	// mutated outside this file. Failing fast at startup is cheap;
	// silent DoS exposure is not.
	//
	// Runs as the LAST step of init() so it observes the fully-wired
	// Registry. Iteration order over a map is unspecified but the panic
	// message includes every offending name so a future contributor sees
	// the complete list on the first failure.
	var missing []string
	for name, tc := range Registry {
		if tc.List != nil && tc.Count == nil {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		panic(fmt.Sprintf("pdbcompat: Registry entries have List without CountFunc: %v", missing))
	}

	// Get/Match pairing: serveDetail applies the filters of a request
	// through Match. A Get without a Match would serve the object and
	// ignore every filter.
	missing = nil
	for name, tc := range Registry {
		if tc.Get != nil && tc.Match == nil {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		panic(fmt.Sprintf("pdbcompat: Registry entries have Get without MatchFunc: %v", missing))
	}

	// List depth: every type has ListIDs, and a type has ListDepth if
	// and only if childSets lists sets for it. A set type without
	// ListDepth would serve depth-0 rows at list depth > 0, and a
	// ListDepth without sets would render nothing extra.
	missing = nil
	for name, tc := range Registry {
		if tc.ListIDs == nil || (tc.ListDepth != nil) != (len(childSets[name]) > 0) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		panic(fmt.Sprintf("pdbcompat: Registry entries have ListIDs or ListDepth out of step with childSets: %v", missing))
	}
}

// listQuery is the builder shape every generated ent query type shares.
// Q is the concrete builder (self-referential so chained calls keep the
// concrete type), P its predicate type, O its order option, and E the
// row type returned by All.
type listQuery[Q any, P, O ~func(*sql.Selector), E any] interface {
	Where(...P) Q
	Order(...O) Q
	Offset(int) Q
	Limit(int) Q
	All(context.Context) ([]E, error)
	Count(context.Context) (int, error)
	Exist(context.Context) (bool, error)
	IDs(context.Context) ([]int, error)
}

// entityWiring declares one entity's list/count/get registration for
// wireEntity. The four type parameters name the entity's generated ent
// types explicitly at each wiring site — verbose, but it keeps every
// entry greppable by concrete type.
type entityWiring[Q listQuery[Q, P, O, E], P, O ~func(*sql.Selector), E any] struct {
	name     string
	plural   string // noun for "list <plural>" / "count <plural>" error wrapping
	isCampus bool   // campus-only status-matrix branch (2.83.0 rest.py:725-735)
	query    func(*ent.Client) Q
	convert  func(context.Context, E) any
	get      GetFunc
	// id returns the id of a row. Required for the types with reverse
	// sets (childSets), whose list at depth > 0 loads rows by id.
	id func(E) int
}

// wireEntity registers one entity's List, Count, Get and Match
// functions in the Registry. List and Count are built from a SINGLE
// shared predicate builder, so the pre-flight budget count and the
// served response can never disagree: predicate divergence (which
// would break the 413 guarantee) is unrepresentable by construction.
// Match takes the same filter predicates, so a detail request and a
// list request apply a filter with the same SQL.
func wireEntity[Q listQuery[Q, P, O, E], P, O ~func(*sql.Selector), E any](w entityWiring[Q, P, O, E]) {
	// The live status set is fixed per type (netixlan: ok and
	// not-operational; all others: ok), so resolve it once at wiring.
	live := pdbtypes.LiveStatuses(w.name)
	predicates := func(opts QueryOptions) []P {
		preds := castPredicates[P](opts.Filters)
		if s := applySince(opts); s != nil {
			preds = append(preds, P(s))
		}
		// upstream 2.83.0 rest.py:719-750 status matrix — appended LAST
		// so no client-supplied filter can widen the visible status set.
		preds = append(preds, P(applyStatusMatrix(live, w.isCampus, opts.Since != nil)))
		return preds
	}
	list := func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, error) {
		// empty __in returns empty set.
		if opts.EmptyResult {
			return []any{}, nil
		}
		q := w.query(client).Where(predicates(opts)...).Order(listOrder[O](opts)...).Offset(opts.Skip)
		if opts.Limit > 0 {
			q = q.Limit(opts.Limit)
		}
		rows, err := q.All(ctx)
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", w.plural, err)
		}
		out := make([]any, len(rows))
		for i, row := range rows {
			out[i] = w.convert(ctx, row)
		}
		return out, nil
	}
	count := func(ctx context.Context, client *ent.Client, opts QueryOptions) (int, error) {
		// empty __in returns empty set.
		if opts.EmptyResult {
			return 0, nil
		}
		total, err := w.query(client).Where(predicates(opts)...).Count(ctx)
		if err != nil {
			return 0, fmt.Errorf("count %s: %w", w.plural, err)
		}
		return servedRowCount(total, opts), nil
	}
	// match sends SELECT id ... WHERE <filters> AND id = ? LIMIT 1. It
	// adds no status matrix: the detail status set is the inline
	// StatusIn of the PK lookup (w.get). The query runs with the
	// request ctx, so the poc privacy policy applies.
	match := func(ctx context.Context, client *ent.Client, id int, filters []func(*sql.Selector)) (bool, error) {
		preds := append(castPredicates[P](filters), P(sql.FieldEQ("id", id)))
		ok, err := w.query(client).Where(preds...).Exist(ctx)
		if err != nil {
			return false, fmt.Errorf("match %s: %w", w.plural, err)
		}
		return ok, nil
	}
	// listIDs sends the list query with the same predicates, order, skip
	// and limit as list, and reads only the ids.
	listIDs := func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]int, error) {
		if opts.EmptyResult {
			return nil, nil
		}
		q := w.query(client).Where(predicates(opts)...).Order(listOrder[O](opts)...).Offset(opts.Skip)
		if opts.Limit > 0 {
			q = q.Limit(opts.Limit)
		}
		ids, err := q.IDs(ctx)
		if err != nil {
			return nil, fmt.Errorf("list %s ids: %w", w.plural, err)
		}
		return ids, nil
	}
	var listDepth ListDepthFunc
	if len(childSets[w.name]) > 0 {
		if w.id == nil {
			panic(fmt.Sprintf("pdbcompat: %s has reverse sets but no id accessor", w.name))
		}
		// listDepth loads one chunk of rows by id. The id predicate goes
		// into a copy of opts.Filters, so predicates keeps the status
		// matrix last and opts.Filters is not changed. The chunk query
		// needs no ORDER BY: the rows are put in the order of ids.
		listDepth = func(ctx context.Context, client *ent.Client, opts QueryOptions, ids []int, depth int, sets []childSet) ([]func() any, error) {
			if len(ids) == 0 {
				return nil, nil
			}
			chunk := opts
			chunk.Filters = append(slices.Clone(opts.Filters), idInJSON("id", ids))
			chunk.Skip, chunk.Limit = 0, 0
			rows, err := w.query(client).Where(predicates(chunk)...).All(ctx)
			if err != nil {
				return nil, fmt.Errorf("list %s chunk: %w", w.plural, err)
			}
			byID := make(map[int]E, len(rows))
			for _, row := range rows {
				byID[w.id(row)] = row
			}
			parents := make([]E, 0, len(rows))
			parentIDs := make([]int, 0, len(rows))
			for _, id := range ids {
				if row, ok := byID[id]; ok {
					parents = append(parents, row)
					parentIDs = append(parentIDs, id)
				}
			}
			renders, err := renderListDepthChunk(ctx, client, parents, parentIDs, depth, sets,
				func(row E) any { return w.convert(ctx, row) })
			if err != nil {
				return nil, fmt.Errorf("list %s sets: %w", w.plural, err)
			}
			return renders, nil
		}
	}
	setFuncs(w.name, entityFuncs{
		list: list, count: count, get: w.get, match: match,
		listIDs: listIDs, listDepth: listDepth,
	})
}

// entityFuncs holds the query functions of one Registry entry.
type entityFuncs struct {
	list      ListFunc
	count     CountFunc
	get       GetFunc
	match     MatchFunc
	listIDs   ListIDsFunc
	listDepth ListDepthFunc
}

// setFuncs updates the query functions of a Registry entry.
func setFuncs(name string, f entityFuncs) {
	tc := Registry[name]
	tc.List = f.list
	tc.Count = f.count
	tc.Get = f.get
	tc.Match = f.match
	tc.ListIDs = f.listIDs
	tc.ListDepth = f.listDepth
	Registry[name] = tc
}

// castPredicates converts generic sql.Selector functions to typed predicates
// via the shared underlying function signature.
func castPredicates[T ~func(*sql.Selector)](filters []func(*sql.Selector)) []T {
	out := make([]T, len(filters))
	for i, f := range filters {
		out[i] = T(f)
	}
	return out
}

// applySince adds an updated >= since filter if Since is set in opts.
//
// Upstream filters with django-handleref since(): created__gt or
// updated__gt against datetime.fromtimestamp(since), which is
// since.000000 (2.83.0 rest.py:736-744). updated__gt alone covers
// created__gt because created <= updated on every row. Upstream stores
// updated with microseconds but serializes it truncated to the second
// (serializers.py:1920-1924), and the mirror stores only that second.
// A row shown as updated=N almost always has a non-zero fraction
// upstream, so upstream returns it for since=N. The boundary is
// therefore inclusive on the stored second. With a strict > filter, a
// client that polls with since=<max updated seen> never gets a row
// that the mirror syncs later with the same second. The cost is that
// the client gets the boundary rows again, which is idempotent.
func applySince(opts QueryOptions) func(*sql.Selector) {
	if opts.Since == nil {
		return nil
	}
	return sql.FieldGTE("updated", *opts.Since)
}

// listOrder returns the ORDER BY for a list query.
//
// A plain list is ordered by id ascending. Upstream adds no ORDER BY to
// a plain list (2.83.0 rest.py:747-748), and none of the 13 models
// declares Meta.ordering (migrations/0001_initial.py has no "ordering"
// option), so MySQL serves the rows in primary-key order. SQLite reads
// the rowid table in this order and does not sort.
//
// A ?since= list is ordered by updated ascending, as upstream orders it
// (rest.py:738-745). The id tiebreak keeps the order of rows with the
// same updated value stable across pages.
//
// A plain list with opts.OrderBy (a fac or org distance search) is
// ordered by that key, then by id. Upstream orders by distance only
// (serializers.py:1897) and leaves ties to the database. A ?since= list
// keeps the updated order: upstream order_by("updated") replaces the
// distance order (rest.py:744; Django order_by clears the earlier
// ordering, django/db/models/query.py:1721-1728).
func listOrder[T ~func(*sql.Selector)](opts QueryOptions) []T {
	if opts.Since != nil {
		return []T{T(ent.Asc("updated")), T(ent.Asc("id"))}
	}
	if opts.OrderBy != nil {
		return []T{T(opts.OrderBy), T(ent.Asc("id"))}
	}
	return []T{T(ent.Asc("id"))}
}

// servedRowCount computes the post-Offset/Limit row count the handler
// will actually serve given a raw filtered total. Shared by every
// CountFunc so the pre-flight budget math stays consistent across the
// 13 entities.
func servedRowCount(total int, opts QueryOptions) int {
	served := max(total-opts.Skip, 0)
	if opts.Limit > 0 && served > opts.Limit {
		served = opts.Limit
	}
	return served
}
