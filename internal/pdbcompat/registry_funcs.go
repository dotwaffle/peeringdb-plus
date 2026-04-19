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
}

// setFuncs updates a Registry entry's List and Get functions.
func setFuncs(name string, list ListFunc, get GetFunc) {
	tc := Registry[name]
	tc.List = list
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

func wireOrgFuncs() {
	setFuncs(peeringdb.TypeOrg,
		func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error) {
			preds := castPredicates[predicate.Organization](opts.Filters)
			if s := applySince(opts); s != nil {
				preds = append(preds, predicate.Organization(s))
			}
			// Phase 68 D-05/D-07: upstream rest.py:694-727 status matrix.
			preds = append(preds, predicate.Organization(applyStatusMatrix(false /*isCampus*/, opts.Since != nil)))
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
		getOrgWithDepth,
	)
}

func wireNetFuncs() {
	setFuncs(peeringdb.TypeNet,
		func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error) {
			preds := castPredicates[predicate.Network](opts.Filters)
			if s := applySince(opts); s != nil {
				preds = append(preds, predicate.Network(s))
			}
			// Phase 68 D-05/D-07: upstream rest.py:694-727 status matrix.
			preds = append(preds, predicate.Network(applyStatusMatrix(false /*isCampus*/, opts.Since != nil)))
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
		getNetWithDepth,
	)
}

func wireFacFuncs() {
	setFuncs(peeringdb.TypeFac,
		func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error) {
			preds := castPredicates[predicate.Facility](opts.Filters)
			if s := applySince(opts); s != nil {
				preds = append(preds, predicate.Facility(s))
			}
			// Phase 68 D-05/D-07: upstream rest.py:694-727 status matrix.
			preds = append(preds, predicate.Facility(applyStatusMatrix(false /*isCampus*/, opts.Since != nil)))
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
		getFacWithDepth,
	)
}

func wireIXFuncs() {
	setFuncs(peeringdb.TypeIX,
		func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error) {
			preds := castPredicates[predicate.InternetExchange](opts.Filters)
			if s := applySince(opts); s != nil {
				preds = append(preds, predicate.InternetExchange(s))
			}
			// Phase 68 D-05/D-07: upstream rest.py:694-727 status matrix.
			preds = append(preds, predicate.InternetExchange(applyStatusMatrix(false /*isCampus*/, opts.Since != nil)))
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
		getIXWithDepth,
	)
}

func wirePocFuncs() {
	setFuncs(peeringdb.TypePoc,
		func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error) {
			preds := castPredicates[predicate.Poc](opts.Filters)
			if s := applySince(opts); s != nil {
				preds = append(preds, predicate.Poc(s))
			}
			// Phase 68 D-05/D-07: upstream rest.py:694-727 status matrix.
			preds = append(preds, predicate.Poc(applyStatusMatrix(false /*isCampus*/, opts.Since != nil)))
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
		getPocWithDepth,
	)
}

func wireIXLanFuncs() {
	setFuncs(peeringdb.TypeIXLan,
		func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error) {
			preds := castPredicates[predicate.IxLan](opts.Filters)
			if s := applySince(opts); s != nil {
				preds = append(preds, predicate.IxLan(s))
			}
			// Phase 68 D-05/D-07: upstream rest.py:694-727 status matrix.
			preds = append(preds, predicate.IxLan(applyStatusMatrix(false /*isCampus*/, opts.Since != nil)))
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
		getIXLanWithDepth,
	)
}

func wireIXPfxFuncs() {
	setFuncs(peeringdb.TypeIXPfx,
		func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error) {
			preds := castPredicates[predicate.IxPrefix](opts.Filters)
			if s := applySince(opts); s != nil {
				preds = append(preds, predicate.IxPrefix(s))
			}
			// Phase 68 D-05/D-07: upstream rest.py:694-727 status matrix.
			preds = append(preds, predicate.IxPrefix(applyStatusMatrix(false /*isCampus*/, opts.Since != nil)))
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
		getIXPfxWithDepth,
	)
}

func wireNetIXLanFuncs() {
	setFuncs(peeringdb.TypeNetIXLan,
		func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error) {
			preds := castPredicates[predicate.NetworkIxLan](opts.Filters)
			if s := applySince(opts); s != nil {
				preds = append(preds, predicate.NetworkIxLan(s))
			}
			// Phase 68 D-05/D-07: upstream rest.py:694-727 status matrix.
			preds = append(preds, predicate.NetworkIxLan(applyStatusMatrix(false /*isCampus*/, opts.Since != nil)))
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
		getNetIXLanWithDepth,
	)
}

func wireNetFacFuncs() {
	setFuncs(peeringdb.TypeNetFac,
		func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error) {
			preds := castPredicates[predicate.NetworkFacility](opts.Filters)
			if s := applySince(opts); s != nil {
				preds = append(preds, predicate.NetworkFacility(s))
			}
			// Phase 68 D-05/D-07: upstream rest.py:694-727 status matrix.
			preds = append(preds, predicate.NetworkFacility(applyStatusMatrix(false /*isCampus*/, opts.Since != nil)))
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
		getNetFacWithDepth,
	)
}

func wireIXFacFuncs() {
	setFuncs(peeringdb.TypeIXFac,
		func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error) {
			preds := castPredicates[predicate.IxFacility](opts.Filters)
			if s := applySince(opts); s != nil {
				preds = append(preds, predicate.IxFacility(s))
			}
			// Phase 68 D-05/D-07: upstream rest.py:694-727 status matrix.
			preds = append(preds, predicate.IxFacility(applyStatusMatrix(false /*isCampus*/, opts.Since != nil)))
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
		getIXFacWithDepth,
	)
}

func wireCarrierFuncs() {
	setFuncs(peeringdb.TypeCarrier,
		func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error) {
			preds := castPredicates[predicate.Carrier](opts.Filters)
			if s := applySince(opts); s != nil {
				preds = append(preds, predicate.Carrier(s))
			}
			// Phase 68 D-05/D-07: upstream rest.py:694-727 status matrix.
			preds = append(preds, predicate.Carrier(applyStatusMatrix(false /*isCampus*/, opts.Since != nil)))
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
		getCarrierWithDepth,
	)
}

func wireCarrierFacFuncs() {
	setFuncs(peeringdb.TypeCarrierFac,
		func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error) {
			preds := castPredicates[predicate.CarrierFacility](opts.Filters)
			if s := applySince(opts); s != nil {
				preds = append(preds, predicate.CarrierFacility(s))
			}
			// Phase 68 D-05/D-07: upstream rest.py:694-727 status matrix.
			preds = append(preds, predicate.CarrierFacility(applyStatusMatrix(false /*isCampus*/, opts.Since != nil)))
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
		getCarrierFacWithDepth,
	)
}

func wireCampusFuncs() {
	setFuncs(peeringdb.TypeCampus,
		func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error) {
			preds := castPredicates[predicate.Campus](opts.Filters)
			if s := applySince(opts); s != nil {
				preds = append(preds, predicate.Campus(s))
			}
			// Phase 68 D-05/D-07: upstream rest.py:694-727 status matrix.
			// Campus is the only type that admits status=pending on list+since (rest.py:721).
			preds = append(preds, predicate.Campus(applyStatusMatrix(true /*isCampus*/, opts.Since != nil)))
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
		getCampusWithDepth,
	)
}
