package pdbcompat

import (
	"context"
	"fmt"

	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

func init() {
	wireOrgFuncs()
	wireNetFuncs()
	wireFacFuncs()
	wireIXFuncs()
	wirePocFuncs()
	wireIXLanFuncs()
	wireIXPfxFuncs()
	wireNetIXLanFuncs()
	wireNetFacFuncs()
	wireIXFacFuncs()
	wireCarrierFuncs()
	wireCarrierFacFuncs()
	wireCampusFuncs()

	// Phase 71 WR-01 invariant: every entity that exposes a List closure
	// MUST also expose a Count closure. serveList's pre-flight budget
	// check in handler.go is gated on `tc.Count != nil` — a missing Count
	// would silently bypass the 413 guardrail and put the process back at
	// OOM risk on an unbounded list. Failing fast at startup is cheap;
	// silent DoS exposure is not. See CONTEXT.md D-02.
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
		panic(fmt.Sprintf("pdbcompat: Registry entries have List without CountFunc (Phase 71 WR-01): %v", missing))
	}
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

// applySince adds an updated >= since filter if Since is set in opts.
func applySince(opts QueryOptions) func(*sql.Selector) {
	if opts.Since == nil {
		return nil
	}
	return sql.FieldGTE("updated", *opts.Since)
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

func wireOrgFuncs() {
	orgPredicates := func(opts QueryOptions) []predicate.Organization {
		preds := castPredicates[predicate.Organization](opts.Filters)
		if s := applySince(opts); s != nil {
			preds = append(preds, predicate.Organization(s))
		}
		// Phase 68 D-05/D-07: upstream rest.py:694-727 status matrix.
		preds = append(preds, predicate.Organization(applyStatusMatrix(false /*isCampus*/, opts.Since != nil)))
		return preds
	}
	setFuncs(peeringdb.TypeOrg,
		func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error) {
			// Phase 69 IN-02: empty __in returns empty set per D-06.
			if opts.EmptyResult {
				return []any{}, 0, nil
			}
			preds := orgPredicates(opts)
			q := client.Organization.Query().Where(preds...).Order(ent.Desc("updated"), ent.Desc("created"), ent.Desc("id"))
			total, err := q.Count(ctx)
			if err != nil {
				return nil, 0, fmt.Errorf("count organizations: %w", err)
			}
			q2 := q.Offset(opts.Skip)
			if opts.Limit > 0 {
				q2 = q2.Limit(opts.Limit)
			}
			orgs, err := q2.All(ctx)
			if err != nil {
				return nil, 0, fmt.Errorf("list organizations: %w", err)
			}
			result := organizationsFromEnt(orgs)
			out := make([]any, len(result))
			for i, v := range result {
				out[i] = v
			}
			return out, total, nil
		},
		func(ctx context.Context, client *ent.Client, opts QueryOptions) (int, error) {
			// Phase 69 IN-02: empty __in returns empty set per D-06.
			if opts.EmptyResult {
				return 0, nil
			}
			preds := orgPredicates(opts)
			total, err := client.Organization.Query().Where(preds...).Count(ctx)
			if err != nil {
				return 0, fmt.Errorf("count organizations: %w", err)
			}
			return servedRowCount(total, opts), nil
		},
		getOrgWithDepth,
	)
}

func wireNetFuncs() {
	netPredicates := func(opts QueryOptions) []predicate.Network {
		preds := castPredicates[predicate.Network](opts.Filters)
		if s := applySince(opts); s != nil {
			preds = append(preds, predicate.Network(s))
		}
		// Phase 68 D-05/D-07: upstream rest.py:694-727 status matrix.
		preds = append(preds, predicate.Network(applyStatusMatrix(false /*isCampus*/, opts.Since != nil)))
		return preds
	}
	setFuncs(peeringdb.TypeNet,
		func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error) {
			// Phase 69 IN-02: empty __in returns empty set per D-06.
			if opts.EmptyResult {
				return []any{}, 0, nil
			}
			preds := netPredicates(opts)
			q := client.Network.Query().Where(preds...).Order(ent.Desc("updated"), ent.Desc("created"), ent.Desc("id"))
			total, err := q.Count(ctx)
			if err != nil {
				return nil, 0, fmt.Errorf("count networks: %w", err)
			}
			q2 := q.Offset(opts.Skip)
			if opts.Limit > 0 {
				q2 = q2.Limit(opts.Limit)
			}
			nets, err := q2.All(ctx)
			if err != nil {
				return nil, 0, fmt.Errorf("list networks: %w", err)
			}
			result := networksFromEnt(nets)
			out := make([]any, len(result))
			for i, v := range result {
				out[i] = v
			}
			return out, total, nil
		},
		func(ctx context.Context, client *ent.Client, opts QueryOptions) (int, error) {
			// Phase 69 IN-02: empty __in returns empty set per D-06.
			if opts.EmptyResult {
				return 0, nil
			}
			preds := netPredicates(opts)
			total, err := client.Network.Query().Where(preds...).Count(ctx)
			if err != nil {
				return 0, fmt.Errorf("count networks: %w", err)
			}
			return servedRowCount(total, opts), nil
		},
		getNetWithDepth,
	)
}

func wireFacFuncs() {
	facPredicates := func(opts QueryOptions) []predicate.Facility {
		preds := castPredicates[predicate.Facility](opts.Filters)
		if s := applySince(opts); s != nil {
			preds = append(preds, predicate.Facility(s))
		}
		// Phase 68 D-05/D-07: upstream rest.py:694-727 status matrix.
		preds = append(preds, predicate.Facility(applyStatusMatrix(false /*isCampus*/, opts.Since != nil)))
		return preds
	}
	setFuncs(peeringdb.TypeFac,
		func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error) {
			// Phase 69 IN-02: empty __in returns empty set per D-06.
			if opts.EmptyResult {
				return []any{}, 0, nil
			}
			preds := facPredicates(opts)
			q := client.Facility.Query().Where(preds...).Order(ent.Desc("updated"), ent.Desc("created"), ent.Desc("id"))
			total, err := q.Count(ctx)
			if err != nil {
				return nil, 0, fmt.Errorf("count facilities: %w", err)
			}
			q2 := q.Offset(opts.Skip)
			if opts.Limit > 0 {
				q2 = q2.Limit(opts.Limit)
			}
			facs, err := q2.All(ctx)
			if err != nil {
				return nil, 0, fmt.Errorf("list facilities: %w", err)
			}
			result := facilitiesFromEnt(facs)
			out := make([]any, len(result))
			for i, v := range result {
				out[i] = v
			}
			return out, total, nil
		},
		func(ctx context.Context, client *ent.Client, opts QueryOptions) (int, error) {
			// Phase 69 IN-02: empty __in returns empty set per D-06.
			if opts.EmptyResult {
				return 0, nil
			}
			preds := facPredicates(opts)
			total, err := client.Facility.Query().Where(preds...).Count(ctx)
			if err != nil {
				return 0, fmt.Errorf("count facilities: %w", err)
			}
			return servedRowCount(total, opts), nil
		},
		getFacWithDepth,
	)
}

func wireIXFuncs() {
	ixPredicates := func(opts QueryOptions) []predicate.InternetExchange {
		preds := castPredicates[predicate.InternetExchange](opts.Filters)
		if s := applySince(opts); s != nil {
			preds = append(preds, predicate.InternetExchange(s))
		}
		// Phase 68 D-05/D-07: upstream rest.py:694-727 status matrix.
		preds = append(preds, predicate.InternetExchange(applyStatusMatrix(false /*isCampus*/, opts.Since != nil)))
		return preds
	}
	setFuncs(peeringdb.TypeIX,
		func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error) {
			// Phase 69 IN-02: empty __in returns empty set per D-06.
			if opts.EmptyResult {
				return []any{}, 0, nil
			}
			preds := ixPredicates(opts)
			q := client.InternetExchange.Query().Where(preds...).Order(ent.Desc("updated"), ent.Desc("created"), ent.Desc("id"))
			total, err := q.Count(ctx)
			if err != nil {
				return nil, 0, fmt.Errorf("count internet exchanges: %w", err)
			}
			q2 := q.Offset(opts.Skip)
			if opts.Limit > 0 {
				q2 = q2.Limit(opts.Limit)
			}
			ixes, err := q2.All(ctx)
			if err != nil {
				return nil, 0, fmt.Errorf("list internet exchanges: %w", err)
			}
			result := internetExchangesFromEnt(ixes)
			out := make([]any, len(result))
			for i, v := range result {
				out[i] = v
			}
			return out, total, nil
		},
		func(ctx context.Context, client *ent.Client, opts QueryOptions) (int, error) {
			// Phase 69 IN-02: empty __in returns empty set per D-06.
			if opts.EmptyResult {
				return 0, nil
			}
			preds := ixPredicates(opts)
			total, err := client.InternetExchange.Query().Where(preds...).Count(ctx)
			if err != nil {
				return 0, fmt.Errorf("count internet exchanges: %w", err)
			}
			return servedRowCount(total, opts), nil
		},
		getIXWithDepth,
	)
}

func wirePocFuncs() {
	pocPredicates := func(opts QueryOptions) []predicate.Poc {
		preds := castPredicates[predicate.Poc](opts.Filters)
		if s := applySince(opts); s != nil {
			preds = append(preds, predicate.Poc(s))
		}
		// Phase 68 D-05/D-07: upstream rest.py:694-727 status matrix.
		preds = append(preds, predicate.Poc(applyStatusMatrix(false /*isCampus*/, opts.Since != nil)))
		return preds
	}
	setFuncs(peeringdb.TypePoc,
		func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error) {
			// Phase 69 IN-02: empty __in returns empty set per D-06.
			if opts.EmptyResult {
				return []any{}, 0, nil
			}
			preds := pocPredicates(opts)
			q := client.Poc.Query().Where(preds...).Order(ent.Desc("updated"), ent.Desc("created"), ent.Desc("id"))
			total, err := q.Count(ctx)
			if err != nil {
				return nil, 0, fmt.Errorf("count pocs: %w", err)
			}
			q2 := q.Offset(opts.Skip)
			if opts.Limit > 0 {
				q2 = q2.Limit(opts.Limit)
			}
			pocs, err := q2.All(ctx)
			if err != nil {
				return nil, 0, fmt.Errorf("list pocs: %w", err)
			}
			result := pocsFromEnt(pocs)
			out := make([]any, len(result))
			for i, v := range result {
				out[i] = v
			}
			return out, total, nil
		},
		func(ctx context.Context, client *ent.Client, opts QueryOptions) (int, error) {
			// Phase 69 IN-02: empty __in returns empty set per D-06.
			if opts.EmptyResult {
				return 0, nil
			}
			preds := pocPredicates(opts)
			total, err := client.Poc.Query().Where(preds...).Count(ctx)
			if err != nil {
				return 0, fmt.Errorf("count pocs: %w", err)
			}
			return servedRowCount(total, opts), nil
		},
		getPocWithDepth,
	)
}

func wireIXLanFuncs() {
	ixLanPredicates := func(opts QueryOptions) []predicate.IxLan {
		preds := castPredicates[predicate.IxLan](opts.Filters)
		if s := applySince(opts); s != nil {
			preds = append(preds, predicate.IxLan(s))
		}
		// Phase 68 D-05/D-07: upstream rest.py:694-727 status matrix.
		preds = append(preds, predicate.IxLan(applyStatusMatrix(false /*isCampus*/, opts.Since != nil)))
		return preds
	}
	setFuncs(peeringdb.TypeIXLan,
		func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error) {
			// Phase 69 IN-02: empty __in returns empty set per D-06.
			if opts.EmptyResult {
				return []any{}, 0, nil
			}
			preds := ixLanPredicates(opts)
			q := client.IxLan.Query().Where(preds...).Order(ent.Desc("updated"), ent.Desc("created"), ent.Desc("id"))
			total, err := q.Count(ctx)
			if err != nil {
				return nil, 0, fmt.Errorf("count ixlans: %w", err)
			}
			q2 := q.Offset(opts.Skip)
			if opts.Limit > 0 {
				q2 = q2.Limit(opts.Limit)
			}
			lans, err := q2.All(ctx)
			if err != nil {
				return nil, 0, fmt.Errorf("list ixlans: %w", err)
			}
			result := ixLansFromEnt(ctx, lans)
			out := make([]any, len(result))
			for i, v := range result {
				out[i] = v
			}
			return out, total, nil
		},
		func(ctx context.Context, client *ent.Client, opts QueryOptions) (int, error) {
			// Phase 69 IN-02: empty __in returns empty set per D-06.
			if opts.EmptyResult {
				return 0, nil
			}
			preds := ixLanPredicates(opts)
			total, err := client.IxLan.Query().Where(preds...).Count(ctx)
			if err != nil {
				return 0, fmt.Errorf("count ixlans: %w", err)
			}
			return servedRowCount(total, opts), nil
		},
		getIXLanWithDepth,
	)
}

func wireIXPfxFuncs() {
	ixPfxPredicates := func(opts QueryOptions) []predicate.IxPrefix {
		preds := castPredicates[predicate.IxPrefix](opts.Filters)
		if s := applySince(opts); s != nil {
			preds = append(preds, predicate.IxPrefix(s))
		}
		// Phase 68 D-05/D-07: upstream rest.py:694-727 status matrix.
		preds = append(preds, predicate.IxPrefix(applyStatusMatrix(false /*isCampus*/, opts.Since != nil)))
		return preds
	}
	setFuncs(peeringdb.TypeIXPfx,
		func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error) {
			// Phase 69 IN-02: empty __in returns empty set per D-06.
			if opts.EmptyResult {
				return []any{}, 0, nil
			}
			preds := ixPfxPredicates(opts)
			q := client.IxPrefix.Query().Where(preds...).Order(ent.Desc("updated"), ent.Desc("created"), ent.Desc("id"))
			total, err := q.Count(ctx)
			if err != nil {
				return nil, 0, fmt.Errorf("count ixprefixes: %w", err)
			}
			q2 := q.Offset(opts.Skip)
			if opts.Limit > 0 {
				q2 = q2.Limit(opts.Limit)
			}
			pfxs, err := q2.All(ctx)
			if err != nil {
				return nil, 0, fmt.Errorf("list ixprefixes: %w", err)
			}
			result := ixPrefixesFromEnt(pfxs)
			out := make([]any, len(result))
			for i, v := range result {
				out[i] = v
			}
			return out, total, nil
		},
		func(ctx context.Context, client *ent.Client, opts QueryOptions) (int, error) {
			// Phase 69 IN-02: empty __in returns empty set per D-06.
			if opts.EmptyResult {
				return 0, nil
			}
			preds := ixPfxPredicates(opts)
			total, err := client.IxPrefix.Query().Where(preds...).Count(ctx)
			if err != nil {
				return 0, fmt.Errorf("count ixprefixes: %w", err)
			}
			return servedRowCount(total, opts), nil
		},
		getIXPfxWithDepth,
	)
}

func wireNetIXLanFuncs() {
	netIXLanPredicates := func(opts QueryOptions) []predicate.NetworkIxLan {
		preds := castPredicates[predicate.NetworkIxLan](opts.Filters)
		if s := applySince(opts); s != nil {
			preds = append(preds, predicate.NetworkIxLan(s))
		}
		// Phase 68 D-05/D-07: upstream rest.py:694-727 status matrix.
		preds = append(preds, predicate.NetworkIxLan(applyStatusMatrix(false /*isCampus*/, opts.Since != nil)))
		return preds
	}
	setFuncs(peeringdb.TypeNetIXLan,
		func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error) {
			// Phase 69 IN-02: empty __in returns empty set per D-06.
			if opts.EmptyResult {
				return []any{}, 0, nil
			}
			preds := netIXLanPredicates(opts)
			q := client.NetworkIxLan.Query().Where(preds...).Order(ent.Desc("updated"), ent.Desc("created"), ent.Desc("id"))
			total, err := q.Count(ctx)
			if err != nil {
				return nil, 0, fmt.Errorf("count networkixlans: %w", err)
			}
			q2 := q.Offset(opts.Skip)
			if opts.Limit > 0 {
				q2 = q2.Limit(opts.Limit)
			}
			nixls, err := q2.All(ctx)
			if err != nil {
				return nil, 0, fmt.Errorf("list networkixlans: %w", err)
			}
			result := networkIxLansFromEnt(nixls)
			out := make([]any, len(result))
			for i, v := range result {
				out[i] = v
			}
			return out, total, nil
		},
		func(ctx context.Context, client *ent.Client, opts QueryOptions) (int, error) {
			// Phase 69 IN-02: empty __in returns empty set per D-06.
			if opts.EmptyResult {
				return 0, nil
			}
			preds := netIXLanPredicates(opts)
			total, err := client.NetworkIxLan.Query().Where(preds...).Count(ctx)
			if err != nil {
				return 0, fmt.Errorf("count networkixlans: %w", err)
			}
			return servedRowCount(total, opts), nil
		},
		getNetIXLanWithDepth,
	)
}

func wireNetFacFuncs() {
	netFacPredicates := func(opts QueryOptions) []predicate.NetworkFacility {
		preds := castPredicates[predicate.NetworkFacility](opts.Filters)
		if s := applySince(opts); s != nil {
			preds = append(preds, predicate.NetworkFacility(s))
		}
		// Phase 68 D-05/D-07: upstream rest.py:694-727 status matrix.
		preds = append(preds, predicate.NetworkFacility(applyStatusMatrix(false /*isCampus*/, opts.Since != nil)))
		return preds
	}
	setFuncs(peeringdb.TypeNetFac,
		func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error) {
			// Phase 69 IN-02: empty __in returns empty set per D-06.
			if opts.EmptyResult {
				return []any{}, 0, nil
			}
			preds := netFacPredicates(opts)
			q := client.NetworkFacility.Query().Where(preds...).Order(ent.Desc("updated"), ent.Desc("created"), ent.Desc("id"))
			total, err := q.Count(ctx)
			if err != nil {
				return nil, 0, fmt.Errorf("count networkfacilities: %w", err)
			}
			q2 := q.Offset(opts.Skip)
			if opts.Limit > 0 {
				q2 = q2.Limit(opts.Limit)
			}
			nfacs, err := q2.All(ctx)
			if err != nil {
				return nil, 0, fmt.Errorf("list networkfacilities: %w", err)
			}
			result := networkFacilitiesFromEnt(nfacs)
			out := make([]any, len(result))
			for i, v := range result {
				out[i] = v
			}
			return out, total, nil
		},
		func(ctx context.Context, client *ent.Client, opts QueryOptions) (int, error) {
			// Phase 69 IN-02: empty __in returns empty set per D-06.
			if opts.EmptyResult {
				return 0, nil
			}
			preds := netFacPredicates(opts)
			total, err := client.NetworkFacility.Query().Where(preds...).Count(ctx)
			if err != nil {
				return 0, fmt.Errorf("count networkfacilities: %w", err)
			}
			return servedRowCount(total, opts), nil
		},
		getNetFacWithDepth,
	)
}

func wireIXFacFuncs() {
	ixFacPredicates := func(opts QueryOptions) []predicate.IxFacility {
		preds := castPredicates[predicate.IxFacility](opts.Filters)
		if s := applySince(opts); s != nil {
			preds = append(preds, predicate.IxFacility(s))
		}
		// Phase 68 D-05/D-07: upstream rest.py:694-727 status matrix.
		preds = append(preds, predicate.IxFacility(applyStatusMatrix(false /*isCampus*/, opts.Since != nil)))
		return preds
	}
	setFuncs(peeringdb.TypeIXFac,
		func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error) {
			// Phase 69 IN-02: empty __in returns empty set per D-06.
			if opts.EmptyResult {
				return []any{}, 0, nil
			}
			preds := ixFacPredicates(opts)
			q := client.IxFacility.Query().Where(preds...).Order(ent.Desc("updated"), ent.Desc("created"), ent.Desc("id"))
			total, err := q.Count(ctx)
			if err != nil {
				return nil, 0, fmt.Errorf("count ixfacilities: %w", err)
			}
			q2 := q.Offset(opts.Skip)
			if opts.Limit > 0 {
				q2 = q2.Limit(opts.Limit)
			}
			ixfacs, err := q2.All(ctx)
			if err != nil {
				return nil, 0, fmt.Errorf("list ixfacilities: %w", err)
			}
			result := ixFacilitiesFromEnt(ixfacs)
			out := make([]any, len(result))
			for i, v := range result {
				out[i] = v
			}
			return out, total, nil
		},
		func(ctx context.Context, client *ent.Client, opts QueryOptions) (int, error) {
			// Phase 69 IN-02: empty __in returns empty set per D-06.
			if opts.EmptyResult {
				return 0, nil
			}
			preds := ixFacPredicates(opts)
			total, err := client.IxFacility.Query().Where(preds...).Count(ctx)
			if err != nil {
				return 0, fmt.Errorf("count ixfacilities: %w", err)
			}
			return servedRowCount(total, opts), nil
		},
		getIXFacWithDepth,
	)
}

func wireCarrierFuncs() {
	carrierPredicates := func(opts QueryOptions) []predicate.Carrier {
		preds := castPredicates[predicate.Carrier](opts.Filters)
		if s := applySince(opts); s != nil {
			preds = append(preds, predicate.Carrier(s))
		}
		// Phase 68 D-05/D-07: upstream rest.py:694-727 status matrix.
		preds = append(preds, predicate.Carrier(applyStatusMatrix(false /*isCampus*/, opts.Since != nil)))
		return preds
	}
	setFuncs(peeringdb.TypeCarrier,
		func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error) {
			// Phase 69 IN-02: empty __in returns empty set per D-06.
			if opts.EmptyResult {
				return []any{}, 0, nil
			}
			preds := carrierPredicates(opts)
			q := client.Carrier.Query().Where(preds...).Order(ent.Desc("updated"), ent.Desc("created"), ent.Desc("id"))
			total, err := q.Count(ctx)
			if err != nil {
				return nil, 0, fmt.Errorf("count carriers: %w", err)
			}
			q2 := q.Offset(opts.Skip)
			if opts.Limit > 0 {
				q2 = q2.Limit(opts.Limit)
			}
			carriers, err := q2.All(ctx)
			if err != nil {
				return nil, 0, fmt.Errorf("list carriers: %w", err)
			}
			result := carriersFromEnt(carriers)
			out := make([]any, len(result))
			for i, v := range result {
				out[i] = v
			}
			return out, total, nil
		},
		func(ctx context.Context, client *ent.Client, opts QueryOptions) (int, error) {
			// Phase 69 IN-02: empty __in returns empty set per D-06.
			if opts.EmptyResult {
				return 0, nil
			}
			preds := carrierPredicates(opts)
			total, err := client.Carrier.Query().Where(preds...).Count(ctx)
			if err != nil {
				return 0, fmt.Errorf("count carriers: %w", err)
			}
			return servedRowCount(total, opts), nil
		},
		getCarrierWithDepth,
	)
}

func wireCarrierFacFuncs() {
	carrierFacPredicates := func(opts QueryOptions) []predicate.CarrierFacility {
		preds := castPredicates[predicate.CarrierFacility](opts.Filters)
		if s := applySince(opts); s != nil {
			preds = append(preds, predicate.CarrierFacility(s))
		}
		// Phase 68 D-05/D-07: upstream rest.py:694-727 status matrix.
		preds = append(preds, predicate.CarrierFacility(applyStatusMatrix(false /*isCampus*/, opts.Since != nil)))
		return preds
	}
	setFuncs(peeringdb.TypeCarrierFac,
		func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error) {
			// Phase 69 IN-02: empty __in returns empty set per D-06.
			if opts.EmptyResult {
				return []any{}, 0, nil
			}
			preds := carrierFacPredicates(opts)
			q := client.CarrierFacility.Query().Where(preds...).Order(ent.Desc("updated"), ent.Desc("created"), ent.Desc("id"))
			total, err := q.Count(ctx)
			if err != nil {
				return nil, 0, fmt.Errorf("count carrierfacilities: %w", err)
			}
			q2 := q.Offset(opts.Skip)
			if opts.Limit > 0 {
				q2 = q2.Limit(opts.Limit)
			}
			cfs, err := q2.All(ctx)
			if err != nil {
				return nil, 0, fmt.Errorf("list carrierfacilities: %w", err)
			}
			result := carrierFacilitiesFromEnt(cfs)
			out := make([]any, len(result))
			for i, v := range result {
				out[i] = v
			}
			return out, total, nil
		},
		func(ctx context.Context, client *ent.Client, opts QueryOptions) (int, error) {
			// Phase 69 IN-02: empty __in returns empty set per D-06.
			if opts.EmptyResult {
				return 0, nil
			}
			preds := carrierFacPredicates(opts)
			total, err := client.CarrierFacility.Query().Where(preds...).Count(ctx)
			if err != nil {
				return 0, fmt.Errorf("count carrierfacilities: %w", err)
			}
			return servedRowCount(total, opts), nil
		},
		getCarrierFacWithDepth,
	)
}

func wireCampusFuncs() {
	campusPredicates := func(opts QueryOptions) []predicate.Campus {
		preds := castPredicates[predicate.Campus](opts.Filters)
		if s := applySince(opts); s != nil {
			preds = append(preds, predicate.Campus(s))
		}
		// Phase 68 D-05/D-07: upstream rest.py:694-727 status matrix.
		// Campus is the only type that admits status=pending on list+since (rest.py:721).
		preds = append(preds, predicate.Campus(applyStatusMatrix(true /*isCampus*/, opts.Since != nil)))
		return preds
	}
	setFuncs(peeringdb.TypeCampus,
		func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error) {
			// Phase 69 IN-02: empty __in returns empty set per D-06.
			if opts.EmptyResult {
				return []any{}, 0, nil
			}
			preds := campusPredicates(opts)
			q := client.Campus.Query().Where(preds...).Order(ent.Desc("updated"), ent.Desc("created"), ent.Desc("id"))
			total, err := q.Count(ctx)
			if err != nil {
				return nil, 0, fmt.Errorf("count campuses: %w", err)
			}
			q2 := q.Offset(opts.Skip)
			if opts.Limit > 0 {
				q2 = q2.Limit(opts.Limit)
			}
			campuses, err := q2.All(ctx)
			if err != nil {
				return nil, 0, fmt.Errorf("list campuses: %w", err)
			}
			result := campusesFromEnt(campuses)
			out := make([]any, len(result))
			for i, v := range result {
				out[i] = v
			}
			return out, total, nil
		},
		func(ctx context.Context, client *ent.Client, opts QueryOptions) (int, error) {
			// Phase 69 IN-02: empty __in returns empty set per D-06.
			if opts.EmptyResult {
				return 0, nil
			}
			preds := campusPredicates(opts)
			total, err := client.Campus.Query().Where(preds...).Count(ctx)
			if err != nil {
				return 0, fmt.Errorf("count campuses: %w", err)
			}
			return servedRowCount(total, opts), nil
		},
		getCampusWithDepth,
	)
}
