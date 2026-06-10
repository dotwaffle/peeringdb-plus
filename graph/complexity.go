package graph

import (
	"entgo.io/contrib/entgql"

	"github.com/dotwaffle/peeringdb-plus/ent"
)

// Fan-out-aware GraphQL complexity weights (2026-06-10 audit).
//
// gqlgen's default costing charges 1 per field regardless of list fan-out,
// so extension.FixedComplexityLimit bounded the number of FIELDS in the
// query text, not the number of ROWS the query materializes. A depth-15
// query nesting unpaginated edge lists (internetExchanges → ixLans →
// networkIxLans → network → networkIxLans → …) cost ~15 units under the
// old limit of 500 yet multiplied row counts per hop into the millions —
// an OOM-kill on 256 MB replicas.
//
// ComplexityLimits assigns each list field a multiplier so the computed
// complexity approximates "fields-of-rows materialized":
//
//   - Relay connections and offset/limit flat lists multiply by the
//     requested page size (first/last/limit, with the same defaults the
//     resolvers apply).
//   - Unpaginated entity edge lists multiply by a weight approximating
//     the live corpus's average per-parent cardinality (rounded up;
//     heavy-tailed edges round up harder). These only need to be the
//     right order of magnitude for the budget to bind.
//
// ComplexityLimit is the budget those units are spent against, enforced
// by extension.FixedComplexityLimit in internal/graphql. Calibration:
// a full first:1000 page of networks with all scalar fields costs ~50k;
// one internet exchange's full member list (ixLans → networkIxLans, all
// fields) costs ~30k; first:1000 exchanges each expanding their full
// member list costs ~30M and is rejected — that shape materializes the
// entire netixlan table plus duplicates and is exactly the replica-OOM
// case. Clients needing bulk data should paginate the top-level
// connection instead of fanning out nested lists.
const ComplexityLimit = 1_000_000

// Average per-parent cardinality weights for unpaginated edge lists.
// Order-of-magnitude estimates from the live corpus (≈30k networks,
// ≈120k netixlans, ≈3k ixlans, ≈17k facilities, ≈230k netfacs).
const (
	weightFew             = 4  // org.*, ix.ixLans, ixlan.ixPrefixes, net.pocs, campus.facilities, fac.{ix,carrier}Facilities
	weightModerate        = 16 // net.{networkIxLans,networkFacilities}, ix.ixFacilities, carrier.carrierFacilities, fac.networkFacilities
	weightIxlanMemberList = 64 // ixlan.networkIxLans — heavy tail (large IXes have 1000+ members)
)

// pageCost returns the row multiplier for a Relay connection, mirroring
// defaultFirst: explicit first/last pass through, absent both defaults
// to DefaultLimit.
func pageCost(first, last *int) int {
	switch {
	case first != nil:
		return max(*first, 1)
	case last != nil:
		return max(*last, 1)
	default:
		return DefaultLimit
	}
}

// limitCost returns the row multiplier for an offset/limit flat list,
// mirroring ValidateOffsetLimit's default.
func limitCost(limit *int) int {
	if limit != nil {
		return max(*limit, 1)
	}
	return DefaultLimit
}

// ComplexityLimits returns the ComplexityRoot wired into the gqlgen
// server config so FixedComplexityLimit(ComplexityLimit) bounds rows
// materialized rather than fields mentioned.
func ComplexityLimits() ComplexityRoot {
	scale := func(n int) func(int) int {
		return func(childComplexity int) int { return n * childComplexity }
	}

	var c ComplexityRoot

	// Unpaginated entity edge lists — the multiplicative fan-out hops.
	c.Organization.Networks = scale(weightFew)
	c.Organization.Facilities = scale(weightFew)
	c.Organization.InternetExchanges = scale(weightFew)
	c.Organization.Campuses = scale(weightFew)
	c.Organization.Carriers = scale(weightFew)
	c.Campus.Facilities = scale(weightFew)
	c.Carrier.CarrierFacilities = scale(weightModerate)
	c.Facility.NetworkFacilities = scale(weightModerate)
	c.Facility.IxFacilities = scale(weightFew)
	c.Facility.CarrierFacilities = scale(weightFew)
	c.InternetExchange.IxLans = scale(weightFew)
	c.InternetExchange.IxFacilities = scale(weightModerate)
	c.IxLan.NetworkIxLans = scale(weightIxlanMemberList)
	c.IxLan.IxPrefixes = scale(weightFew)
	c.Network.NetworkIxLans = scale(weightModerate)
	c.Network.NetworkFacilities = scale(weightModerate)
	c.Network.Pocs = scale(weightFew)

	// Relay connections: multiply by requested page size.
	c.Query.Campuses = func(childComplexity int, _ *entgql.Cursor[int], first *int, _ *entgql.Cursor[int], last *int, _ *ent.CampusOrder, _ *ent.CampusWhereInput) int {
		return pageCost(first, last) * childComplexity
	}
	c.Query.Carriers = func(childComplexity int, _ *entgql.Cursor[int], first *int, _ *entgql.Cursor[int], last *int, _ *ent.CarrierOrder, _ *ent.CarrierWhereInput) int {
		return pageCost(first, last) * childComplexity
	}
	c.Query.CarrierFacilities = func(childComplexity int, _ *entgql.Cursor[int], first *int, _ *entgql.Cursor[int], last *int, _ *ent.CarrierFacilityWhereInput) int {
		return pageCost(first, last) * childComplexity
	}
	c.Query.Facilities = func(childComplexity int, _ *entgql.Cursor[int], first *int, _ *entgql.Cursor[int], last *int, _ *ent.FacilityOrder, _ *ent.FacilityWhereInput) int {
		return pageCost(first, last) * childComplexity
	}
	c.Query.InternetExchanges = func(childComplexity int, _ *entgql.Cursor[int], first *int, _ *entgql.Cursor[int], last *int, _ *ent.InternetExchangeOrder, _ *ent.InternetExchangeWhereInput) int {
		return pageCost(first, last) * childComplexity
	}
	c.Query.IxFacilities = func(childComplexity int, _ *entgql.Cursor[int], first *int, _ *entgql.Cursor[int], last *int, _ *ent.IxFacilityWhereInput) int {
		return pageCost(first, last) * childComplexity
	}
	c.Query.IxLans = func(childComplexity int, _ *entgql.Cursor[int], first *int, _ *entgql.Cursor[int], last *int, _ *ent.IxLanWhereInput) int {
		return pageCost(first, last) * childComplexity
	}
	c.Query.IxPrefixes = func(childComplexity int, _ *entgql.Cursor[int], first *int, _ *entgql.Cursor[int], last *int, _ *ent.IxPrefixWhereInput) int {
		return pageCost(first, last) * childComplexity
	}
	c.Query.NetworkFacilities = func(childComplexity int, _ *entgql.Cursor[int], first *int, _ *entgql.Cursor[int], last *int, _ *ent.NetworkFacilityWhereInput) int {
		return pageCost(first, last) * childComplexity
	}
	c.Query.NetworkIxLans = func(childComplexity int, _ *entgql.Cursor[int], first *int, _ *entgql.Cursor[int], last *int, _ *ent.NetworkIxLanWhereInput) int {
		return pageCost(first, last) * childComplexity
	}
	c.Query.Networks = func(childComplexity int, _ *entgql.Cursor[int], first *int, _ *entgql.Cursor[int], last *int, _ *ent.NetworkOrder, _ *ent.NetworkWhereInput) int {
		return pageCost(first, last) * childComplexity
	}
	c.Query.Organizations = func(childComplexity int, _ *entgql.Cursor[int], first *int, _ *entgql.Cursor[int], last *int, _ *ent.OrganizationOrder, _ *ent.OrganizationWhereInput) int {
		return pageCost(first, last) * childComplexity
	}
	c.Query.Pocs = func(childComplexity int, _ *entgql.Cursor[int], first *int, _ *entgql.Cursor[int], last *int, _ *ent.PocWhereInput) int {
		return pageCost(first, last) * childComplexity
	}

	// Offset/limit flat lists: multiply by requested limit.
	c.Query.CampusesList = func(childComplexity int, _ *int, limit *int, _ *ent.CampusWhereInput) int {
		return limitCost(limit) * childComplexity
	}
	c.Query.CarriersList = func(childComplexity int, _ *int, limit *int, _ *ent.CarrierWhereInput) int {
		return limitCost(limit) * childComplexity
	}
	c.Query.CarrierFacilitiesList = func(childComplexity int, _ *int, limit *int, _ *ent.CarrierFacilityWhereInput) int {
		return limitCost(limit) * childComplexity
	}
	c.Query.FacilitiesList = func(childComplexity int, _ *int, limit *int, _ *ent.FacilityWhereInput) int {
		return limitCost(limit) * childComplexity
	}
	c.Query.InternetExchangesList = func(childComplexity int, _ *int, limit *int, _ *ent.InternetExchangeWhereInput) int {
		return limitCost(limit) * childComplexity
	}
	c.Query.IxFacilitiesList = func(childComplexity int, _ *int, limit *int, _ *ent.IxFacilityWhereInput) int {
		return limitCost(limit) * childComplexity
	}
	c.Query.IxLansList = func(childComplexity int, _ *int, limit *int, _ *ent.IxLanWhereInput) int {
		return limitCost(limit) * childComplexity
	}
	c.Query.IxPrefixesList = func(childComplexity int, _ *int, limit *int, _ *ent.IxPrefixWhereInput) int {
		return limitCost(limit) * childComplexity
	}
	c.Query.NetworkFacilitiesList = func(childComplexity int, _ *int, limit *int, _ *ent.NetworkFacilityWhereInput) int {
		return limitCost(limit) * childComplexity
	}
	c.Query.NetworkIxLansList = func(childComplexity int, _ *int, limit *int, _ *ent.NetworkIxLanWhereInput) int {
		return limitCost(limit) * childComplexity
	}
	c.Query.NetworksList = func(childComplexity int, _ *int, limit *int, _ *ent.NetworkWhereInput) int {
		return limitCost(limit) * childComplexity
	}
	c.Query.OrganizationsList = func(childComplexity int, _ *int, limit *int, _ *ent.OrganizationWhereInput) int {
		return limitCost(limit) * childComplexity
	}
	c.Query.PocsList = func(childComplexity int, _ *int, limit *int, _ *ent.PocWhereInput) int {
		return limitCost(limit) * childComplexity
	}

	// Batched node lookup: one row per requested id.
	c.Query.Nodes = func(childComplexity int, ids []int) int {
		return max(len(ids), 1) * childComplexity
	}

	return c
}
