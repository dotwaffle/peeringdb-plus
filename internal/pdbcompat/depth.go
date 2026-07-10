package pdbcompat

import (
	"context"
	"fmt"
	"reflect"
	"sort"

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
)

// toMap converts a serializer struct to map[string]any so depth responses can
// add `_set` fields and strip back-reference keys dynamically. It walks the
// cached reflect field maps from search.go (one pass, no intermediate JSON
// encoding — the former json.Marshal/Unmarshal round-trip cost 2N+1 full JSON
// passes per detail request). Field values are stored as their Go values; the
// final WriteResponse marshal renders them identically to the round-trip.
//
// `omitempty` parity is load-bearing: peeringdb.IxLan declares
// `ixf_ixp_member_list_url,omitempty` so a privfield-redacted (zero) value
// must drop the KEY, exactly as json.Marshal would — emitting an empty string
// would leak the field's presence to anonymous callers.
// TestToMap_MatchesJSONRoundTrip locks the equivalence.
func toMap(v any) map[string]any {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return map[string]any{}
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return map[string]any{}
	}
	fm := getFieldMap(rv.Type())
	m := make(map[string]any, len(fm))
	for name, acc := range fm {
		fv := rv.Field(acc.index)
		if acc.omitEmpty && isEmptyJSONValue(fv) {
			continue
		}
		m[name] = fv.Interface()
	}
	return m
}

// isEmptyJSONValue mirrors encoding/json's isEmptyValue: the values that the
// `omitempty` tag option suppresses.
func isEmptyJSONValue(v reflect.Value) bool {
	switch v.Kind() { //nolint:exhaustive // default arm mirrors encoding/json's isEmptyValue
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		return v.IsZero()
	case reflect.Interface, reflect.Pointer:
		return v.IsNil()
	default:
		// Structs (e.g. time.Time), complex, chan, func, etc.: never
		// "empty" for omitempty purposes — matches encoding/json.
		return false
	}
}

// orEmptySlice converts a typed slice to []any, returning an empty []any{}
// for nil or zero-length slices. This ensures JSON serialization produces []
// instead of null for _set fields.
func orEmptySlice[T any](v []T) []any {
	if len(v) == 0 {
		return []any{}
	}
	out := make([]any, len(v))
	for i, item := range v {
		out[i] = item
	}
	return out
}

// setWithout serializes each element of an embedded reverse-relation set to a
// map and drops the named keys. It is used to strip the back-reference parent
// FK (e.g. net_id inside a Network's netixlan_set) from nested elements, which
// upstream PeeringDB omits when the relation is reached through its parent — a
// netixlan embedded under /api/net/<id>?depth=2 carries no net_id. Empty input
// yields []any{} so the field serializes as [] rather than null.
func setWithout[T any](items []T, dropKeys ...string) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		m := toMap(item)
		for _, k := range dropKeys {
			delete(m, k)
		}
		out = append(out, m)
	}
	return out
}

// sortedIDsOrEmpty normalises an ent IDs() result into an ascending, non-nil
// slice so empty sets serialize as [] (not null) and ordering is deterministic,
// matching upstream's ascending-id nested ID lists.
func sortedIDsOrEmpty(ids []int, err error) ([]int, error) {
	if err != nil {
		return nil, err
	}
	if ids == nil {
		ids = []int{}
	}
	sort.Ints(ids)
	return ids, nil
}

// intsOrEmpty normalises an ent Ints() result into a non-nil slice WITHOUT
// sorting. Upstream renders through-relation ID lists (an ixlan's net_set
// reached via the netixlan join, an ix's fac_set via the ixfac join) in join
// order with duplicates preserved — unlike the direct reverse-set ID lists
// which come back ascending. Empty input yields []int{} so the field
// serializes as [] rather than null.
func intsOrEmpty(ids []int, err error) ([]int, error) {
	if err != nil {
		return nil, err
	}
	if ids == nil {
		ids = []int{}
	}
	return ids, nil
}

// nestedOrgMap renders an organization as it appears when embedded in a parent
// object at depth=2 (e.g. the `org` field of a Network). Upstream expands the
// org's own scalar fields and represents its reverse relations as bare ID
// lists (the depth budget is exhausted one level down), so callers get
// net_set/fac_set/ix_set/carrier_set/campus_set as []int rather than full
// objects. The org entity must already be loaded by the caller.
func nestedOrgMap(ctx context.Context, o *ent.Organization) (map[string]any, error) {
	m := toMap(organizationFromEnt(o))
	var err error
	if m["net_set"], err = sortedIDsOrEmpty(o.QueryNetworks().Where(network.StatusIn("ok", "pending")).IDs(ctx)); err != nil {
		return nil, fmt.Errorf("nested org %d net_set: %w", o.ID, err)
	}
	if m["fac_set"], err = sortedIDsOrEmpty(o.QueryFacilities().Where(facility.StatusIn("ok", "pending")).IDs(ctx)); err != nil {
		return nil, fmt.Errorf("nested org %d fac_set: %w", o.ID, err)
	}
	if m["ix_set"], err = sortedIDsOrEmpty(o.QueryInternetExchanges().Where(internetexchange.StatusIn("ok", "pending")).IDs(ctx)); err != nil {
		return nil, fmt.Errorf("nested org %d ix_set: %w", o.ID, err)
	}
	if m["carrier_set"], err = sortedIDsOrEmpty(o.QueryCarriers().Where(carrier.StatusIn("ok", "pending")).IDs(ctx)); err != nil {
		return nil, fmt.Errorf("nested org %d carrier_set: %w", o.ID, err)
	}
	if m["campus_set"], err = sortedIDsOrEmpty(o.QueryCampuses().Where(campus.StatusIn("ok", "pending")).IDs(ctx)); err != nil {
		return nil, fmt.Errorf("nested org %d campus_set: %w", o.ID, err)
	}
	return m, nil
}

// nestedCampusMap renders a campus embedded one level down (its `org` is then
// at the depth boundary, so it expands as a flat object with no further sets;
// its fac_set is a bare ID list). The campus must be loaded with its
// organization edge (WithCampus(q.WithOrganization())).
func nestedCampusMap(ctx context.Context, c *ent.Campus) (map[string]any, error) {
	m := toMap(campusFromEnt(c))
	ids, err := sortedIDsOrEmpty(c.QueryFacilities().Where(facility.StatusIn("ok", "pending")).IDs(ctx))
	if err != nil {
		return nil, fmt.Errorf("nested campus %d fac_set: %w", c.ID, err)
	}
	m["fac_set"] = ids
	if c.Edges.Organization != nil {
		m["org"] = organizationFromEnt(c.Edges.Organization)
	}
	return m, nil
}

// The nested*Map builders below render a singular FK object embedded one level
// down at depth=2 (e.g. the `net` under a poc/netfac/netixlan). They mirror
// nestedOrgMap's contract: a full object whose own reverse relations are bare
// ID lists and whose own forward FK objects are flat (their sets popped). This
// is exactly upstream's recursive depth budget — verified against live
// www.peeringdb.com payloads (2026-06-08). The same builders render a top-level
// object at ?depth=1 (see the depth==1 getter branches).

// nestedNetMap renders a Network as a singular FK at depth=2 (or as a top-level
// object at depth=1): the network's poc_set/netfac_set/netixlan_set as ascending
// ID lists and its org as a flat object.
func nestedNetMap(ctx context.Context, n *ent.Network) (map[string]any, error) {
	m := toMap(networkFromEnt(n))
	if org, err := n.QueryOrganization().Only(ctx); err == nil {
		m["org"] = organizationFromEnt(org)
	} else if !ent.IsNotFound(err) {
		return nil, fmt.Errorf("nested net %d org: %w", n.ID, err)
	}
	var err error
	if m["poc_set"], err = sortedIDsOrEmpty(n.QueryPocs().Where(poc.StatusIn("ok", "pending")).IDs(ctx)); err != nil {
		return nil, fmt.Errorf("nested net %d poc_set: %w", n.ID, err)
	}
	if m["netfac_set"], err = sortedIDsOrEmpty(n.QueryNetworkFacilities().Where(networkfacility.StatusIn("ok", "pending")).IDs(ctx)); err != nil {
		return nil, fmt.Errorf("nested net %d netfac_set: %w", n.ID, err)
	}
	if m["netixlan_set"], err = sortedIDsOrEmpty(n.QueryNetworkIxLans().Where(networkixlan.StatusIn("ok", "pending")).IDs(ctx)); err != nil {
		return nil, fmt.Errorf("nested net %d netixlan_set: %w", n.ID, err)
	}
	return m, nil
}

// nestedFacMap renders a Facility as a singular FK at depth=2 (or top-level at
// depth=1): the FacilitySerializer embeds NO reverse sets (netfac/ixfac/
// carrierfac are omitted), only its org and campus FK objects, both flat.
func nestedFacMap(ctx context.Context, f *ent.Facility) (map[string]any, error) {
	m := toMap(facilityFromEnt(f))
	if org, err := f.QueryOrganization().Only(ctx); err == nil {
		m["org"] = organizationFromEnt(org)
	} else if !ent.IsNotFound(err) {
		return nil, fmt.Errorf("nested fac %d org: %w", f.ID, err)
	}
	if cmp, err := f.QueryCampus().Only(ctx); err == nil {
		m["campus"] = campusFromEnt(cmp)
	} else if ent.IsNotFound(err) {
		// Upstream FacilitySerializer.campus is a related field emitted at
		// detail depth as null for a campus-less facility, not omitted.
		m["campus"] = nil
	} else {
		return nil, fmt.Errorf("nested fac %d campus: %w", f.ID, err)
	}
	return m, nil
}

// nestedIxMap renders an InternetExchange as a singular FK at depth=2 (or
// top-level at depth=1): ixlan_set as an ascending ID list, fac_set as the
// facility IDs reached through the ixfac join (join order, no dedup), and org
// as a flat object.
func nestedIxMap(ctx context.Context, ix *ent.InternetExchange) (map[string]any, error) {
	m := toMap(internetExchangeFromEnt(ix))
	if org, err := ix.QueryOrganization().Only(ctx); err == nil {
		m["org"] = organizationFromEnt(org)
	} else if !ent.IsNotFound(err) {
		return nil, fmt.Errorf("nested ix %d org: %w", ix.ID, err)
	}
	var err error
	if m["ixlan_set"], err = sortedIDsOrEmpty(ix.QueryIxLans().Where(ixlan.StatusIn("ok", "pending")).IDs(ctx)); err != nil {
		return nil, fmt.Errorf("nested ix %d ixlan_set: %w", ix.ID, err)
	}
	if m["fac_set"], err = intsOrEmpty(ix.QueryIxFacilities().Where(ixfacility.StatusIn("ok", "pending")).Select(ixfacility.FieldFacID).Ints(ctx)); err != nil {
		return nil, fmt.Errorf("nested ix %d fac_set: %w", ix.ID, err)
	}
	return m, nil
}

// nestedIxLanMap renders an IXLan as a singular FK at depth=2 (or top-level at
// depth=1): ixpfx_set as an ascending ID list, net_set as the network IDs
// reached through the netixlan join (join order, no dedup), and ix flat.
func nestedIxLanMap(ctx context.Context, l *ent.IxLan) (map[string]any, error) {
	m := toMap(ixLanFromEnt(ctx, l))
	if ix, err := l.QueryInternetExchange().Only(ctx); err == nil {
		m["ix"] = internetExchangeFromEnt(ix)
	} else if !ent.IsNotFound(err) {
		return nil, fmt.Errorf("nested ixlan %d ix: %w", l.ID, err)
	}
	var err error
	if m["ixpfx_set"], err = sortedIDsOrEmpty(l.QueryIxPrefixes().Where(ixprefix.StatusIn("ok", "pending")).IDs(ctx)); err != nil {
		return nil, fmt.Errorf("nested ixlan %d ixpfx_set: %w", l.ID, err)
	}
	if m["net_set"], err = intsOrEmpty(l.QueryNetworkIxLans().Where(networkixlan.StatusIn("ok", "pending")).Select(networkixlan.FieldNetID).Ints(ctx)); err != nil {
		return nil, fmt.Errorf("nested ixlan %d net_set: %w", l.ID, err)
	}
	return m, nil
}

// nestedCarrierMap renders a Carrier as a singular FK at depth=2 (or top-level
// at depth=1): carrierfac_set as an ascending ID list and org flat.
func nestedCarrierMap(ctx context.Context, c *ent.Carrier) (map[string]any, error) {
	m := toMap(carrierFromEnt(c))
	if org, err := c.QueryOrganization().Only(ctx); err == nil {
		m["org"] = organizationFromEnt(org)
	} else if !ent.IsNotFound(err) {
		return nil, fmt.Errorf("nested carrier %d org: %w", c.ID, err)
	}
	var err error
	if m["carrierfac_set"], err = sortedIDsOrEmpty(c.QueryCarrierFacilities().Where(carrierfacility.StatusIn("ok", "pending")).IDs(ctx)); err != nil {
		return nil, fmt.Errorf("nested carrier %d carrierfac_set: %w", c.ID, err)
	}
	return m, nil
}

// resolveFacilitiesFromIxFacilities walks an IX → IxFacility through-relation
// and returns the embedded Facility records. Mirrors upstream PeeringDB's
// `nested(FacilitySerializer, through="ixfac_set", getter="facility")` shape:
// the IX `fac_set` field is a list of expanded Facility objects, NOT raw
// IxFacility join rows. Callers must have eager-loaded `WithIxFacilities(func
// (q *ent.IxFacilityQuery) { q.WithFacility() })` so each ixfac.Edges.Facility
// is populated. Nil-skipping defends against a deleted Facility paired with a
// stale IxFacility row.
func resolveFacilitiesFromIxFacilities(ixfacs []*ent.IxFacility) []*ent.Facility {
	out := make([]*ent.Facility, 0, len(ixfacs))
	for _, ixf := range ixfacs {
		if ixf == nil || ixf.Edges.Facility == nil {
			continue
		}
		out = append(out, ixf.Edges.Facility)
	}
	return out
}

// getOrgWithDepth fetches an organization by ID with optional depth expansion.
// At depth >= 2, eagerly loads and serializes net_set, fac_set, ix_set,
// carrier_set, and campus_set arrays.
func getOrgWithDepth(ctx context.Context, client *ent.Client, id, depth int) (any, error) {
	if depth >= 2 {
		o, err := client.Organization.Query().
			Where(organization.ID(id), organization.StatusIn("ok", "pending")).
			WithNetworks(func(q *ent.NetworkQuery) { q.Where(network.StatusIn("ok", "pending")) }).
			WithFacilities(func(q *ent.FacilityQuery) { q.Where(facility.StatusIn("ok", "pending")) }).
			WithInternetExchanges(func(q *ent.InternetExchangeQuery) { q.Where(internetexchange.StatusIn("ok", "pending")) }).
			WithCarriers(func(q *ent.CarrierQuery) { q.Where(carrier.StatusIn("ok", "pending")) }).
			WithCampuses(func(q *ent.CampusQuery) { q.Where(campus.StatusIn("ok", "pending")) }).
			Only(ctx)
		if err != nil {
			return nil, fmt.Errorf("get organization %d: %w", id, err)
		}
		base := organizationFromEnt(o)
		m := toMap(base)
		m["net_set"] = setWithout(networksFromEnt(o.Edges.Networks), "org_id")
		m["fac_set"] = setWithout(facilitiesFromEnt(o.Edges.Facilities), "org_id")
		m["ix_set"] = setWithout(internetExchangesFromEnt(o.Edges.InternetExchanges), "org_id")
		m["carrier_set"] = setWithout(carriersFromEnt(o.Edges.Carriers), "org_id")
		m["campus_set"] = setWithout(campusesFromEnt(o.Edges.Campuses), "org_id")
		return m, nil
	}

	o, err := client.Organization.Query().
		Where(organization.ID(id), organization.StatusIn("ok", "pending")).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("get organization %d: %w", id, err)
	}
	if depth == 1 {
		// depth=1: org's reverse sets as ID lists (org has no forward FK).
		return nestedOrgMap(ctx, o)
	}
	return organizationFromEnt(o), nil
}

// getNetWithDepth fetches a network by ID with optional depth expansion.
// At depth >= 2, expands org object and adds poc_set, netfac_set, netixlan_set.
func getNetWithDepth(ctx context.Context, client *ent.Client, id, depth int) (any, error) {
	if depth >= 2 {
		n, err := client.Network.Query().
			Where(network.ID(id), network.StatusIn("ok", "pending")).
			WithOrganization().
			WithPocs(func(q *ent.PocQuery) { q.Where(poc.StatusIn("ok", "pending")) }).
			WithNetworkFacilities(func(q *ent.NetworkFacilityQuery) { q.Where(networkfacility.StatusIn("ok", "pending")) }).
			WithNetworkIxLans(func(q *ent.NetworkIxLanQuery) { q.Where(networkixlan.StatusIn("ok", "pending")) }).
			Only(ctx)
		if err != nil {
			return nil, fmt.Errorf("get network %d: %w", id, err)
		}
		base := networkFromEnt(n)
		m := toMap(base)
		if n.Edges.Organization != nil {
			if m["org"], err = nestedOrgMap(ctx, n.Edges.Organization); err != nil {
				return nil, err
			}
		}
		m["poc_set"] = setWithout(pocsFromEnt(n.Edges.Pocs), "net_id")
		m["netfac_set"] = setWithout(networkFacilitiesFromEnt(n.Edges.NetworkFacilities), "net_id")
		m["netixlan_set"] = setWithout(networkIxLansFromEnt(n.Edges.NetworkIxLans), "net_id")
		return m, nil
	}

	n, err := client.Network.Query().
		Where(network.ID(id), network.StatusIn("ok", "pending")).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("get network %d: %w", id, err)
	}
	if depth == 1 {
		// depth=1: flat org FK + poc_set/netfac_set/netixlan_set as ID lists.
		return nestedNetMap(ctx, n)
	}
	return networkFromEnt(n), nil
}

// getFacWithDepth fetches a facility by ID with optional depth expansion.
// At depth >= 2, expands org and campus objects, adds netfac_set, ixfac_set,
// carrierfac_set.
func getFacWithDepth(ctx context.Context, client *ent.Client, id, depth int) (any, error) {
	if depth >= 2 {
		// Upstream's FacilitySerializer does NOT nest netfac/ixfac/carrierfac
		// reverse sets at depth=2 (verified against /api/fac/1?depth=2: only
		// the inline social_media / available_voltage_services lists appear).
		// It expands the org and campus FK objects only.
		f, err := client.Facility.Query().
			Where(facility.ID(id), facility.StatusIn("ok", "pending")).
			WithOrganization().
			WithCampus(func(q *ent.CampusQuery) { q.WithOrganization() }).
			Only(ctx)
		if err != nil {
			return nil, fmt.Errorf("get facility %d: %w", id, err)
		}
		base := facilityFromEnt(f)
		m := toMap(base)
		if f.Edges.Organization != nil {
			if m["org"], err = nestedOrgMap(ctx, f.Edges.Organization); err != nil {
				return nil, err
			}
		}
		if f.Edges.Campus != nil {
			if m["campus"], err = nestedCampusMap(ctx, f.Edges.Campus); err != nil {
				return nil, err
			}
		} else {
			// Upstream emits campus:null for a campus-less facility at detail
			// depth rather than omitting the key.
			m["campus"] = nil
		}
		return m, nil
	}

	f, err := client.Facility.Query().
		Where(facility.ID(id), facility.StatusIn("ok", "pending")).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("get facility %d: %w", id, err)
	}
	if depth == 1 {
		// depth=1: flat org + campus FK objects, no reverse sets (upstream's
		// FacilitySerializer omits netfac/ixfac/carrierfac).
		return nestedFacMap(ctx, f)
	}
	return facilityFromEnt(f), nil
}

// getIXWithDepth fetches an internet exchange by ID with optional depth
// expansion. At depth >= 2, expands org object and adds ixlan_set plus fac_set.
// fac_set mirrors upstream PeeringDB's InternetExchangeSerializer
// (peeringdb_server/serializers.py:3514): a list of expanded Facility objects
// resolved through the IxFacility many-to-many via getter="facility". The raw
// IxFacility join records are NOT exposed (upstream omits ixfac_set on the IX
// surface entirely; ixfac_set only appears on the facility-side serializer).
func getIXWithDepth(ctx context.Context, client *ent.Client, id, depth int) (any, error) {
	if depth >= 2 {
		ix, err := client.InternetExchange.Query().
			Where(internetexchange.ID(id), internetexchange.StatusIn("ok", "pending")).
			WithOrganization().
			WithIxLans(func(q *ent.IxLanQuery) { q.Where(ixlan.StatusIn("ok", "pending")) }).
			WithIxFacilities(func(q *ent.IxFacilityQuery) {
				q.Where(ixfacility.StatusIn("ok", "pending")).WithFacility()
			}).
			Only(ctx)
		if err != nil {
			return nil, fmt.Errorf("get internet exchange %d: %w", id, err)
		}
		base := internetExchangeFromEnt(ix)
		m := toMap(base)
		if ix.Edges.Organization != nil {
			if m["org"], err = nestedOrgMap(ctx, ix.Edges.Organization); err != nil {
				return nil, err
			}
		}
		m["ixlan_set"] = setWithout(ixLansFromEnt(ctx, ix.Edges.IxLans), "ix_id")
		// fac_set reaches Facility through the IxFacility join, so its elements
		// carry no ix back-reference to strip (verified: elem keys match upstream).
		m["fac_set"] = orEmptySlice(facilitiesFromEnt(resolveFacilitiesFromIxFacilities(ix.Edges.IxFacilities)))
		return m, nil
	}

	ix, err := client.InternetExchange.Query().
		Where(internetexchange.ID(id), internetexchange.StatusIn("ok", "pending")).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("get internet exchange %d: %w", id, err)
	}
	if depth == 1 {
		// depth=1: flat org FK + ixlan_set/fac_set as ID lists.
		return nestedIxMap(ctx, ix)
	}
	return internetExchangeFromEnt(ix), nil
}

// getIXLanWithDepth fetches an IXLan by ID with optional depth expansion.
// At depth >= 2, expands ix object and adds ixpfx_set plus net_set (the
// networks reached through the netixlan join, matching upstream's
// IXLanSerializer which exposes net_set rather than the raw netixlan rows).
func getIXLanWithDepth(ctx context.Context, client *ent.Client, id, depth int) (any, error) {
	if depth >= 2 {
		l, err := client.IxLan.Query().
			Where(ixlan.ID(id), ixlan.StatusIn("ok", "pending")).
			WithInternetExchange().
			WithIxPrefixes(func(q *ent.IxPrefixQuery) {
				q.Where(ixprefix.StatusIn("ok", "pending"))
			}).
			WithNetworkIxLans(func(q *ent.NetworkIxLanQuery) {
				q.Where(networkixlan.StatusIn("ok", "pending")).
					WithNetwork(func(nq *ent.NetworkQuery) {
						nq.Where(network.StatusIn("ok", "pending"))
					})
			}).
			Only(ctx)
		if err != nil {
			return nil, fmt.Errorf("get ixlan %d: %w", id, err)
		}
		base := ixLanFromEnt(ctx, l)
		m := toMap(base)
		if l.Edges.InternetExchange != nil {
			// ix is a singular FK one level down: full object carrying its own
			// ixlan_set/fac_set as ID lists (nestedIxMap), matching upstream.
			if m["ix"], err = nestedIxMap(ctx, l.Edges.InternetExchange); err != nil {
				return nil, err
			}
		}
		m["ixpfx_set"] = setWithout(ixPrefixesFromEnt(l.Edges.IxPrefixes), "ixlan_id")
		// Upstream IXLanSerializer exposes net_set, NOT netixlan_set:
		// nested(NetworkSerializer, through="netixlan_set", getter="network")
		// (serializers.py:3407) yields one flat Network per active netixlan
		// join row — no dedup, join order. We resolve each join row to its
		// Network and emit the flat NetworkSerializer shape.
		netSet := make([]any, 0, len(l.Edges.NetworkIxLans))
		for _, nixl := range l.Edges.NetworkIxLans {
			if nixl != nil && nixl.Edges.Network != nil {
				netSet = append(netSet, networkFromEnt(nixl.Edges.Network))
			}
		}
		m["net_set"] = netSet
		return m, nil
	}

	l, err := client.IxLan.Query().
		Where(ixlan.ID(id), ixlan.StatusIn("ok", "pending")).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("get ixlan %d: %w", id, err)
	}
	if depth == 1 {
		// depth=1: flat ix FK + net_set/ixpfx_set as ID lists (the same shape
		// nestedIxLanMap renders when an ixlan is embedded one level down).
		return nestedIxLanMap(ctx, l)
	}
	return ixLanFromEnt(ctx, l), nil
}

// getCarrierWithDepth fetches a carrier by ID with optional depth expansion.
// At depth >= 2, expands org object and adds carrierfac_set.
func getCarrierWithDepth(ctx context.Context, client *ent.Client, id, depth int) (any, error) {
	if depth >= 2 {
		c, err := client.Carrier.Query().
			Where(carrier.ID(id), carrier.StatusIn("ok", "pending")).
			WithOrganization().
			WithCarrierFacilities(func(q *ent.CarrierFacilityQuery) { q.Where(carrierfacility.StatusIn("ok", "pending")) }).
			Only(ctx)
		if err != nil {
			return nil, fmt.Errorf("get carrier %d: %w", id, err)
		}
		base := carrierFromEnt(c)
		m := toMap(base)
		if c.Edges.Organization != nil {
			if m["org"], err = nestedOrgMap(ctx, c.Edges.Organization); err != nil {
				return nil, err
			}
		}
		// Upstream CarrierSerializer.carrierfac_set (serializers.py:2196)
		// excludes only ["fac"] (the nested Facility object, which our flat
		// CarrierFacility serializer never emits) — carrier_id and fac_id both
		// stay on each element.
		m["carrierfac_set"] = setWithout(carrierFacilitiesFromEnt(c.Edges.CarrierFacilities))
		return m, nil
	}

	c, err := client.Carrier.Query().
		Where(carrier.ID(id), carrier.StatusIn("ok", "pending")).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("get carrier %d: %w", id, err)
	}
	if depth == 1 {
		// depth=1: flat org FK + carrierfac_set as ID list.
		return nestedCarrierMap(ctx, c)
	}
	return carrierFromEnt(c), nil
}

// getCampusWithDepth fetches a campus by ID with optional depth expansion.
// At depth >= 2, expands org object and adds fac_set.
func getCampusWithDepth(ctx context.Context, client *ent.Client, id, depth int) (any, error) {
	if depth >= 2 {
		c, err := client.Campus.Query().
			Where(campus.ID(id), campus.StatusIn("ok", "pending")).
			WithOrganization().
			WithFacilities(func(q *ent.FacilityQuery) { q.Where(facility.StatusIn("ok", "pending")) }).
			Only(ctx)
		if err != nil {
			return nil, fmt.Errorf("get campus %d: %w", id, err)
		}
		base := campusFromEnt(c)
		m := toMap(base)
		if c.Edges.Organization != nil {
			if m["org"], err = nestedOrgMap(ctx, c.Edges.Organization); err != nil {
				return nil, err
			}
		}
		// Upstream CampusSerializer.fac_set (serializers.py:3917) excludes
		// ["org_id","org"], so a facility nested under a campus keeps campus_id
		// (the parent back-ref) and drops org_id — the inverse of the org case.
		m["fac_set"] = setWithout(facilitiesFromEnt(c.Edges.Facilities), "org_id")
		return m, nil
	}

	if depth == 1 {
		c, err := client.Campus.Query().
			Where(campus.ID(id), campus.StatusIn("ok", "pending")).
			WithOrganization().
			Only(ctx)
		if err != nil {
			return nil, fmt.Errorf("get campus %d: %w", id, err)
		}
		// depth=1: flat org FK + fac_set as ID list.
		return nestedCampusMap(ctx, c)
	}

	c, err := client.Campus.Query().
		Where(campus.ID(id), campus.StatusIn("ok", "pending")).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("get campus %d: %w", id, err)
	}
	return campusFromEnt(c), nil
}

// Leaf entity depth-aware getters: these expand FK edges at depth=2 but have
// no _set fields.

// getNetFacWithDepth fetches a network facility by ID. At depth >= 2, expands
// the net and fac FK edges to full objects.
func getNetFacWithDepth(ctx context.Context, client *ent.Client, id, depth int) (any, error) {
	if depth >= 1 {
		nf, err := client.NetworkFacility.Query().
			Where(networkfacility.ID(id), networkfacility.StatusIn("ok", "pending")).
			WithNetwork().
			WithFacility().
			Only(ctx)
		if err != nil {
			return nil, fmt.Errorf("get networkfacility %d: %w", id, err)
		}
		base := networkFacilityFromEnt(nf)
		m := toMap(base)
		if nf.Edges.Network != nil {
			if depth >= 2 {
				if m["net"], err = nestedNetMap(ctx, nf.Edges.Network); err != nil {
					return nil, err
				}
			} else {
				m["net"] = networkFromEnt(nf.Edges.Network)
			}
		}
		if nf.Edges.Facility != nil {
			if depth >= 2 {
				if m["fac"], err = nestedFacMap(ctx, nf.Edges.Facility); err != nil {
					return nil, err
				}
			} else {
				m["fac"] = facilityFromEnt(nf.Edges.Facility)
			}
		}
		return m, nil
	}

	nf, err := client.NetworkFacility.Query().
		Where(networkfacility.ID(id), networkfacility.StatusIn("ok", "pending")).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("get networkfacility %d: %w", id, err)
	}
	return networkFacilityFromEnt(nf), nil
}

// getNetIXLanWithDepth fetches a network IX LAN by ID. At depth >= 2, expands
// the net and ixlan FK edges to full objects.
func getNetIXLanWithDepth(ctx context.Context, client *ent.Client, id, depth int) (any, error) {
	if depth >= 1 {
		nixl, err := client.NetworkIxLan.Query().
			Where(networkixlan.ID(id), networkixlan.StatusIn("ok", "pending")).
			WithNetwork().
			WithIxLan().
			Only(ctx)
		if err != nil {
			return nil, fmt.Errorf("get networkixlan %d: %w", id, err)
		}
		base := networkIxLanFromEnt(nixl)
		m := toMap(base)
		if nixl.Edges.Network != nil {
			if depth >= 2 {
				if m["net"], err = nestedNetMap(ctx, nixl.Edges.Network); err != nil {
					return nil, err
				}
			} else {
				m["net"] = networkFromEnt(nixl.Edges.Network)
			}
		}
		if nixl.Edges.IxLan != nil {
			if depth >= 2 {
				if m["ixlan"], err = nestedIxLanMap(ctx, nixl.Edges.IxLan); err != nil {
					return nil, err
				}
			} else {
				m["ixlan"] = ixLanFromEnt(ctx, nixl.Edges.IxLan)
			}
		}
		return m, nil
	}

	nixl, err := client.NetworkIxLan.Query().
		Where(networkixlan.ID(id), networkixlan.StatusIn("ok", "pending")).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("get networkixlan %d: %w", id, err)
	}
	return networkIxLanFromEnt(nixl), nil
}

// getIXFacWithDepth fetches an IX facility by ID. At depth >= 2, expands
// the ix and fac FK edges to full objects.
func getIXFacWithDepth(ctx context.Context, client *ent.Client, id, depth int) (any, error) {
	if depth >= 1 {
		ixf, err := client.IxFacility.Query().
			Where(ixfacility.ID(id), ixfacility.StatusIn("ok", "pending")).
			WithInternetExchange().
			WithFacility().
			Only(ctx)
		if err != nil {
			return nil, fmt.Errorf("get ixfacility %d: %w", id, err)
		}
		base := ixFacilityFromEnt(ixf)
		m := toMap(base)
		if ixf.Edges.InternetExchange != nil {
			if depth >= 2 {
				if m["ix"], err = nestedIxMap(ctx, ixf.Edges.InternetExchange); err != nil {
					return nil, err
				}
			} else {
				m["ix"] = internetExchangeFromEnt(ixf.Edges.InternetExchange)
			}
		}
		if ixf.Edges.Facility != nil {
			if depth >= 2 {
				if m["fac"], err = nestedFacMap(ctx, ixf.Edges.Facility); err != nil {
					return nil, err
				}
			} else {
				m["fac"] = facilityFromEnt(ixf.Edges.Facility)
			}
		}
		return m, nil
	}

	ixf, err := client.IxFacility.Query().
		Where(ixfacility.ID(id), ixfacility.StatusIn("ok", "pending")).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("get ixfacility %d: %w", id, err)
	}
	return ixFacilityFromEnt(ixf), nil
}

// getCarrierFacWithDepth fetches a carrier facility by ID. At depth >= 2,
// expands the carrier and fac FK edges to full objects.
func getCarrierFacWithDepth(ctx context.Context, client *ent.Client, id, depth int) (any, error) {
	if depth >= 1 {
		cf, err := client.CarrierFacility.Query().
			Where(carrierfacility.ID(id), carrierfacility.StatusIn("ok", "pending")).
			WithCarrier().
			WithFacility().
			Only(ctx)
		if err != nil {
			return nil, fmt.Errorf("get carrierfacility %d: %w", id, err)
		}
		base := carrierFacilityFromEnt(cf)
		m := toMap(base)
		if cf.Edges.Carrier != nil {
			if depth >= 2 {
				if m["carrier"], err = nestedCarrierMap(ctx, cf.Edges.Carrier); err != nil {
					return nil, err
				}
			} else {
				m["carrier"] = carrierFromEnt(cf.Edges.Carrier)
			}
		}
		if cf.Edges.Facility != nil {
			if depth >= 2 {
				if m["fac"], err = nestedFacMap(ctx, cf.Edges.Facility); err != nil {
					return nil, err
				}
			} else {
				m["fac"] = facilityFromEnt(cf.Edges.Facility)
			}
		}
		return m, nil
	}

	cf, err := client.CarrierFacility.Query().
		Where(carrierfacility.ID(id), carrierfacility.StatusIn("ok", "pending")).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("get carrierfacility %d: %w", id, err)
	}
	return carrierFacilityFromEnt(cf), nil
}

// getPocWithDepth fetches a POC by ID. At depth >= 2, expands the net FK edge.
func getPocWithDepth(ctx context.Context, client *ent.Client, id, depth int) (any, error) {
	if depth >= 1 {
		p, err := client.Poc.Query().
			Where(poc.ID(id), poc.StatusIn("ok", "pending")).
			WithNetwork().
			Only(ctx)
		if err != nil {
			return nil, fmt.Errorf("get poc %d: %w", id, err)
		}
		base := pocFromEnt(p)
		m := toMap(base)
		if p.Edges.Network != nil {
			if depth >= 2 {
				if m["net"], err = nestedNetMap(ctx, p.Edges.Network); err != nil {
					return nil, err
				}
			} else {
				m["net"] = networkFromEnt(p.Edges.Network)
			}
		}
		return m, nil
	}

	p, err := client.Poc.Query().
		Where(poc.ID(id), poc.StatusIn("ok", "pending")).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("get poc %d: %w", id, err)
	}
	return pocFromEnt(p), nil
}

// getIXPfxWithDepth fetches an IX prefix by ID. At depth >= 2, expands the
// ixlan FK edge.
func getIXPfxWithDepth(ctx context.Context, client *ent.Client, id, depth int) (any, error) {
	if depth >= 1 {
		p, err := client.IxPrefix.Query().
			Where(ixprefix.ID(id), ixprefix.StatusIn("ok", "pending")).
			WithIxLan().
			Only(ctx)
		if err != nil {
			return nil, fmt.Errorf("get ixprefix %d: %w", id, err)
		}
		base := ixPrefixFromEnt(p)
		m := toMap(base)
		if p.Edges.IxLan != nil {
			if depth >= 2 {
				if m["ixlan"], err = nestedIxLanMap(ctx, p.Edges.IxLan); err != nil {
					return nil, err
				}
			} else {
				m["ixlan"] = ixLanFromEnt(ctx, p.Edges.IxLan)
			}
		}
		return m, nil
	}

	p, err := client.IxPrefix.Query().
		Where(ixprefix.ID(id), ixprefix.StatusIn("ok", "pending")).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("get ixprefix %d: %w", id, err)
	}
	return ixPrefixFromEnt(p), nil
}
