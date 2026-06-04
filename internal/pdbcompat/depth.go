package pdbcompat

import (
	"context"
	"encoding/json"
	"fmt"
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

// toMap converts a struct to map[string]any using JSON marshal/unmarshal.
// Used only for depth=2 responses where _set fields must be added dynamically.
func toMap(v any) map[string]any {
	b, err := json.Marshal(v)
	if err != nil {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return map[string]any{}
	}
	return m
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
			WithNetworks().
			WithFacilities().
			WithInternetExchanges().
			WithCarriers().
			WithCampuses().
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
	return organizationFromEnt(o), nil
}

// getNetWithDepth fetches a network by ID with optional depth expansion.
// At depth >= 2, expands org object and adds poc_set, netfac_set, netixlan_set.
func getNetWithDepth(ctx context.Context, client *ent.Client, id, depth int) (any, error) {
	if depth >= 2 {
		n, err := client.Network.Query().
			Where(network.ID(id), network.StatusIn("ok", "pending")).
			WithOrganization().
			WithPocs().
			WithNetworkFacilities().
			WithNetworkIxLans().
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
		}
		return m, nil
	}

	f, err := client.Facility.Query().
		Where(facility.ID(id), facility.StatusIn("ok", "pending")).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("get facility %d: %w", id, err)
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
			WithIxLans().
			WithIxFacilities(func(q *ent.IxFacilityQuery) {
				q.WithFacility()
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
	return internetExchangeFromEnt(ix), nil
}

// getIXLanWithDepth fetches an IXLan by ID with optional depth expansion.
// At depth >= 2, expands ix object and adds ixpfx_set, netixlan_set.
func getIXLanWithDepth(ctx context.Context, client *ent.Client, id, depth int) (any, error) {
	if depth >= 2 {
		l, err := client.IxLan.Query().
			Where(ixlan.ID(id), ixlan.StatusIn("ok", "pending")).
			WithInternetExchange().
			WithIxPrefixes().
			WithNetworkIxLans().
			Only(ctx)
		if err != nil {
			return nil, fmt.Errorf("get ixlan %d: %w", id, err)
		}
		base := ixLanFromEnt(ctx, l)
		m := toMap(base)
		if l.Edges.InternetExchange != nil {
			// ix is expanded flat here; its own reverse sets as ID-lists
			// (a depth-3 detail) are not reproduced — see depth parity notes.
			m["ix"] = internetExchangeFromEnt(l.Edges.InternetExchange)
		}
		m["ixpfx_set"] = setWithout(ixPrefixesFromEnt(l.Edges.IxPrefixes), "ixlan_id")
		m["netixlan_set"] = setWithout(networkIxLansFromEnt(l.Edges.NetworkIxLans), "ixlan_id")
		return m, nil
	}

	l, err := client.IxLan.Query().
		Where(ixlan.ID(id), ixlan.StatusIn("ok", "pending")).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("get ixlan %d: %w", id, err)
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
			WithCarrierFacilities().
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
		m["carrierfac_set"] = setWithout(carrierFacilitiesFromEnt(c.Edges.CarrierFacilities), "carrier_id")
		return m, nil
	}

	c, err := client.Carrier.Query().
		Where(carrier.ID(id), carrier.StatusIn("ok", "pending")).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("get carrier %d: %w", id, err)
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
			WithFacilities().
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
		m["fac_set"] = setWithout(facilitiesFromEnt(c.Edges.Facilities), "campus_id")
		return m, nil
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
	if depth >= 2 {
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
			m["net"] = networkFromEnt(nf.Edges.Network)
		}
		if nf.Edges.Facility != nil {
			m["fac"] = facilityFromEnt(nf.Edges.Facility)
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
	if depth >= 2 {
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
			m["net"] = networkFromEnt(nixl.Edges.Network)
		}
		if nixl.Edges.IxLan != nil {
			m["ixlan"] = ixLanFromEnt(ctx, nixl.Edges.IxLan)
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
	if depth >= 2 {
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
			m["ix"] = internetExchangeFromEnt(ixf.Edges.InternetExchange)
		}
		if ixf.Edges.Facility != nil {
			m["fac"] = facilityFromEnt(ixf.Edges.Facility)
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
	if depth >= 2 {
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
			m["carrier"] = carrierFromEnt(cf.Edges.Carrier)
		}
		if cf.Edges.Facility != nil {
			m["fac"] = facilityFromEnt(cf.Edges.Facility)
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
	if depth >= 2 {
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
			m["net"] = networkFromEnt(p.Edges.Network)
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
	if depth >= 2 {
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
			m["ixlan"] = ixLanFromEnt(ctx, p.Edges.IxLan)
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
