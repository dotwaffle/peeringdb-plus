package pdbcompat

import (
	"context"
	"fmt"

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
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

func init() {
	wireEntity(entityWiring[*ent.OrganizationQuery, predicate.Organization, organization.OrderOption, *ent.Organization]{
		name:    peeringdb.TypeOrg,
		plural:  "organizations",
		query:   func(c *ent.Client) *ent.OrganizationQuery { return c.Organization.Query() },
		convert: func(_ context.Context, o *ent.Organization) any { return organizationFromEnt(o) },
		get:     getOrgWithDepth,
	})
	wireEntity(entityWiring[*ent.NetworkQuery, predicate.Network, network.OrderOption, *ent.Network]{
		name:    peeringdb.TypeNet,
		plural:  "networks",
		query:   func(c *ent.Client) *ent.NetworkQuery { return c.Network.Query() },
		convert: func(_ context.Context, n *ent.Network) any { return networkFromEnt(n) },
		get:     getNetWithDepth,
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
		// list+since (rest.py:721).
		isCampus: true,
		query:    func(c *ent.Client) *ent.CampusQuery { return c.Campus.Query() },
		convert:  func(_ context.Context, cp *ent.Campus) any { return campusFromEnt(cp) },
		get:      getCampusWithDepth,
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
}

// entityWiring declares one entity's list/count/get registration for
// wireEntity. The four type parameters name the entity's generated ent
// types explicitly at each wiring site — verbose, but it keeps every
// entry greppable by concrete type.
type entityWiring[Q listQuery[Q, P, O, E], P, O ~func(*sql.Selector), E any] struct {
	name     string
	plural   string // noun for "list <plural>" / "count <plural>" error wrapping
	isCampus bool   // campus-only status-matrix branch (rest.py:721)
	query    func(*ent.Client) Q
	convert  func(context.Context, E) any
	get      GetFunc
}

// wireEntity registers one entity's List, Count, and Get functions in
// the Registry. List and Count are built from a SINGLE shared predicate
// builder, so the pre-flight budget count and the served response can
// never disagree — predicate divergence (which would break the 413
// guarantee) is unrepresentable by construction.
func wireEntity[Q listQuery[Q, P, O, E], P, O ~func(*sql.Selector), E any](w entityWiring[Q, P, O, E]) {
	predicates := func(opts QueryOptions) []P {
		preds := castPredicates[P](opts.Filters)
		if s := applySince(opts); s != nil {
			preds = append(preds, P(s))
		}
		// upstream rest.py:694-727 status matrix — appended LAST so no
		// client-supplied filter can widen the visible status set.
		preds = append(preds, P(applyStatusMatrix(w.isCampus, opts.Since != nil)))
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
	setFuncs(w.name, list, count, w.get)
}

// setFuncs updates a Registry entry's List, Count, and Get functions.
func setFuncs(name string, list ListFunc, count CountFunc, get GetFunc) {
	tc := Registry[name]
	tc.List = list
	tc.Count = count
	tc.Get = get
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

// applySince adds an updated > since filter if Since is set in opts.
// Strictly-greater mirrors upstream django-handleref's since() filter
// (Q(created__gt) | Q(updated__gt)); updated__gt alone subsumes
// created__gt because created <= updated on every row. GTE would
// re-serve every boundary row to a client polling with
// since=<max updated seen>.
func applySince(opts QueryOptions) func(*sql.Selector) {
	if opts.Since == nil {
		return nil
	}
	return sql.FieldGT("updated", *opts.Since)
}

// listOrder returns the ordering for a list query. Plain lists keep the
// stable newest-first triple; ?since= lists are ordered updated-ascending
// (id-ascending tiebreak) to mirror upstream's incremental-update
// ordering, so pollers can resume from the last row's updated value.
func listOrder[T ~func(*sql.Selector)](opts QueryOptions) []T {
	if opts.Since != nil {
		return []T{T(ent.Asc("updated")), T(ent.Asc("id"))}
	}
	return []T{T(ent.Desc("updated")), T(ent.Desc("created")), T(ent.Desc("id"))}
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
