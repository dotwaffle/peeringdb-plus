package pdbcompat

import (
	"context"
	"encoding/json"
	"fmt"

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
// carrier_set, and campus_set arrays per D-19.
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
		m["net_set"] = orEmptySlice(networksFromEnt(o.Edges.Networks))
		m["fac_set"] = orEmptySlice(facilitiesFromEnt(o.Edges.Facilities))
		m["ix_set"] = orEmptySlice(internetExchangesFromEnt(o.Edges.InternetExchanges))
		m["carrier_set"] = orEmptySlice(carriersFromEnt(o.Edges.Carriers))
		m["campus_set"] = orEmptySlice(campusesFromEnt(o.Edges.Campuses))
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
			m["org"] = organizationFromEnt(n.Edges.Organization)
		}
		m["poc_set"] = orEmptySlice(pocsFromEnt(n.Edges.Pocs))
		m["netfac_set"] = orEmptySlice(networkFacilitiesFromEnt(n.Edges.NetworkFacilities))
		m["netixlan_set"] = orEmptySlice(networkIxLansFromEnt(n.Edges.NetworkIxLans))
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
		f, err := client.Facility.Query().
			Where(facility.ID(id), facility.StatusIn("ok", "pending")).
			WithOrganization().
			WithCampus().
			WithNetworkFacilities().
			WithIxFacilities().
			WithCarrierFacilities().
			Only(ctx)
		if err != nil {
			return nil, fmt.Errorf("get facility %d: %w", id, err)
		}
		base := facilityFromEnt(f)
		m := toMap(base)
		if f.Edges.Organization != nil {
			m["org"] = organizationFromEnt(f.Edges.Organization)
		}
		if f.Edges.Campus != nil {
			m["campus"] = campusFromEnt(f.Edges.Campus)
		}
		m["netfac_set"] = orEmptySlice(networkFacilitiesFromEnt(f.Edges.NetworkFacilities))
		m["ixfac_set"] = orEmptySlice(ixFacilitiesFromEnt(f.Edges.IxFacilities))
		m["carrierfac_set"] = orEmptySlice(carrierFacilitiesFromEnt(f.Edges.CarrierFacilities))
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
			m["org"] = organizationFromEnt(ix.Edges.Organization)
		}
		m["ixlan_set"] = orEmptySlice(ixLansFromEnt(ctx, ix.Edges.IxLans))
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
			m["ix"] = internetExchangeFromEnt(l.Edges.InternetExchange)
		}
		m["ixpfx_set"] = orEmptySlice(ixPrefixesFromEnt(l.Edges.IxPrefixes))
		m["netixlan_set"] = orEmptySlice(networkIxLansFromEnt(l.Edges.NetworkIxLans))
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
			m["org"] = organizationFromEnt(c.Edges.Organization)
		}
		m["carrierfac_set"] = orEmptySlice(carrierFacilitiesFromEnt(c.Edges.CarrierFacilities))
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
			m["org"] = organizationFromEnt(c.Edges.Organization)
		}
		m["fac_set"] = orEmptySlice(facilitiesFromEnt(c.Edges.Facilities))
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
