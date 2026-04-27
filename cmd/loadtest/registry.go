package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// restPlurals maps the 13 PeeringDB type constants to their entrest
// /rest/v1 plural path segments. Values verified 2026-04-27 against
// ent/rest/openapi.json.
var restPlurals = map[string]string{
	peeringdb.TypeOrg:        "organizations",
	peeringdb.TypeNet:        "networks",
	peeringdb.TypeFac:        "facilities",
	peeringdb.TypeIX:         "internet-exchanges",
	peeringdb.TypePoc:        "pocs",
	peeringdb.TypeIXLan:      "ix-lans",
	peeringdb.TypeIXPfx:      "ix-prefixes",
	peeringdb.TypeNetIXLan:   "network-ix-lans",
	peeringdb.TypeNetFac:     "network-facilities",
	peeringdb.TypeIXFac:      "ix-facilities",
	peeringdb.TypeCarrier:    "carriers",
	peeringdb.TypeCarrierFac: "carrier-facilities",
	peeringdb.TypeCampus:     "campuses",
}

// rpcServiceNames maps each entity short name to its ConnectRPC
// service name from proto/peeringdb/v1/services.proto.
var rpcServiceNames = map[string]string{
	peeringdb.TypeOrg:        "OrganizationService",
	peeringdb.TypeNet:        "NetworkService",
	peeringdb.TypeFac:        "FacilityService",
	peeringdb.TypeIX:         "InternetExchangeService",
	peeringdb.TypePoc:        "PocService",
	peeringdb.TypeIXLan:      "IxLanService",
	peeringdb.TypeIXPfx:      "IxPrefixService",
	peeringdb.TypeNetIXLan:   "NetworkIxLanService",
	peeringdb.TypeNetFac:     "NetworkFacilityService",
	peeringdb.TypeIXFac:      "IxFacilityService",
	peeringdb.TypeCarrier:    "CarrierService",
	peeringdb.TypeCarrierFac: "CarrierFacilityService",
	peeringdb.TypeCampus:     "CampusService",
}

// rpcMethodNames maps each entity short name to its ConnectRPC PascalCase
// singular noun (used for Get<Noun> methods).
var rpcMethodNames = map[string]string{
	peeringdb.TypeOrg:        "Organization",
	peeringdb.TypeNet:        "Network",
	peeringdb.TypeFac:        "Facility",
	peeringdb.TypeIX:         "InternetExchange",
	peeringdb.TypePoc:        "Poc",
	peeringdb.TypeIXLan:      "IxLan",
	peeringdb.TypeIXPfx:      "IxPrefix",
	peeringdb.TypeNetIXLan:   "NetworkIxLan",
	peeringdb.TypeNetFac:     "NetworkFacility",
	peeringdb.TypeIXFac:      "IxFacility",
	peeringdb.TypeCarrier:    "Carrier",
	peeringdb.TypeCarrierFac: "CarrierFacility",
	peeringdb.TypeCampus:     "Campus",
}

// rpcListMethods maps each entity short name to its full ConnectRPC
// List method name. Naive `+s` pluralisation produces invalid names
// for several types (Campuses, Facilities, IxPrefixes, etc.) — this
// map is the source of truth, mirroring proto/peeringdb/v1/services.proto.
var rpcListMethods = map[string]string{
	peeringdb.TypeOrg:        "ListOrganizations",
	peeringdb.TypeNet:        "ListNetworks",
	peeringdb.TypeFac:        "ListFacilities",
	peeringdb.TypeIX:         "ListInternetExchanges",
	peeringdb.TypePoc:        "ListPocs",
	peeringdb.TypeIXLan:      "ListIxLans",
	peeringdb.TypeIXPfx:      "ListIxPrefixes",
	peeringdb.TypeNetIXLan:   "ListNetworkIxLans",
	peeringdb.TypeNetFac:     "ListNetworkFacilities",
	peeringdb.TypeIXFac:      "ListIxFacilities",
	peeringdb.TypeCarrier:    "ListCarriers",
	peeringdb.TypeCarrierFac: "ListCarrierFacilities",
	peeringdb.TypeCampus:     "ListCampuses",
}

// foldedEntities is the set of 6 PeeringDB types whose name/aka/city
// columns have shadow `_fold` companions per Phase 69. Used to
// generate `?<field>__contains=...` filter shapes against the
// case-insensitive routing.
var foldedEntities = map[string]bool{
	peeringdb.TypeOrg:     true,
	peeringdb.TypeNet:     true,
	peeringdb.TypeFac:     true,
	peeringdb.TypeIX:      true,
	peeringdb.TypeCarrier: true,
	peeringdb.TypeCampus:  true,
}

// allEntityTypes lists all 13 type constants in a stable order — same
// as syncOrder for parity, although registry order has no semantic
// importance.
var allEntityTypes = []string{
	peeringdb.TypeOrg,
	peeringdb.TypeCampus,
	peeringdb.TypeFac,
	peeringdb.TypeCarrier,
	peeringdb.TypeCarrierFac,
	peeringdb.TypeIX,
	peeringdb.TypeIXLan,
	peeringdb.TypeIXPfx,
	peeringdb.TypeIXFac,
	peeringdb.TypeNet,
	peeringdb.TypePoc,
	peeringdb.TypeNetFac,
	peeringdb.TypeNetIXLan,
}

// jsonHeader is the canonical Content-Type for GraphQL POST and
// ConnectRPC unary calls. Reused across endpoint builders.
func jsonHeader() http.Header {
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	return h
}

// registryAll builds the full inventory of endpoints for sweep mode.
// It is also the source pool for soak mode random selection.
//
// `ids` supplies a per-type id to use for get-by-id / Get<Type> shapes
// — a real id discovered at startup via `discoverIDs`. When nil or
// missing for a given type, defaults to 1 (sufficient for tests +
// local dev with seed data; production runs should always pass a
// discovered map because lower IDs are sparse on the live mirror).
//
// Per-entity shapes:
//   - pdbcompat: list-default, list-filtered, get-by-id (3)
//   - entrest:   list-default, get-by-id (2)
//   - graphql:   list (1)
//   - connectrpc: rpc-get, rpc-list (2)
//
// 13 entities × 8 shapes = 104. Plus 3 web-ui surface-wide routes,
// plus an extra pdbcompat startswith filter for each of 6 folded
// entities (6), plus a 2-hop traversal shape for net (1). Total
// ~114, well above the ≥100 pin in the test.
func registryAll(ids map[string]int) []Endpoint {
	eps := make([]Endpoint, 0, 128)
	sinceUnix := time.Now().Add(-time.Hour).Unix()

	idFor := func(t string) int {
		if v, ok := ids[t]; ok && v > 0 {
			return v
		}
		return 1
	}

	for _, t := range allEntityTypes {
		id := idFor(t)
		eps = append(eps, pdbCompatEndpoints(t, sinceUnix, id)...)
		eps = append(eps, entrestEndpoints(t, id)...)
		eps = append(eps, graphqlEndpoints(t)...)
		eps = append(eps, connectRPCEndpoints(t, id)...)
	}

	// Surface-wide Web UI endpoints — content-negotiation requires a
	// browser-flavoured UA so we get HTML rather than ANSI text. Hit
	// applies the UA based on Surface; no per-endpoint header needed.
	eps = append(eps,
		Endpoint{Surface: SurfaceWebUI, Shape: "ui-home", Method: "GET", Path: "/ui/"},
		Endpoint{Surface: SurfaceWebUI, Shape: "ui-about", Method: "GET", Path: "/ui/about"},
		Endpoint{Surface: SurfaceWebUI, Shape: "ui-asn", Method: "GET", Path: "/ui/asn/15169"},
	)

	return eps
}

// pdbCompatEndpoints generates the pdbcompat (/api/<short>...)
// shapes: list-default, list-filtered (per-type), get-by-id, plus
// optional folded-prefix and 2-hop traversal shapes.
func pdbCompatEndpoints(t string, sinceUnix int64, id int) []Endpoint {
	out := []Endpoint{
		{
			Surface:    SurfacePdbCompat,
			EntityType: t,
			Shape:      "list-default",
			Method:     "GET",
			Path:       fmt.Sprintf("/api/%s?limit=10", t),
		},
		{
			Surface:    SurfacePdbCompat,
			EntityType: t,
			Shape:      "get-by-id",
			Method:     "GET",
			Path:       fmt.Sprintf("/api/%s/%d", t, id),
		},
	}

	// Per-type filter shape.
	out = append(out, Endpoint{
		Surface:    SurfacePdbCompat,
		EntityType: t,
		Shape:      "list-filtered",
		Method:     "GET",
		Path:       pdbCompatFilterPath(t, sinceUnix),
	})

	// Phase 69 folded routing — exercise the `_fold` column path on
	// the 6 entities that have shadow columns.
	if foldedEntities[t] {
		out = append(out, Endpoint{
			Surface:    SurfacePdbCompat,
			EntityType: t,
			Shape:      "list-folded-startswith",
			Method:     "GET",
			Path:       fmt.Sprintf("/api/%s?name__startswith=a&limit=10", t),
		})
	}

	// Phase 70 cross-entity traversal smoke — net only.
	if t == peeringdb.TypeNet {
		out = append(out, Endpoint{
			Surface:    SurfacePdbCompat,
			EntityType: t,
			Shape:      "list-traversal",
			Method:     "GET",
			Path:       "/api/net?ix__name__contains=de-cix&limit=10",
		})
	}

	return out
}

// pdbCompatFilterPath returns a per-type filter shape that exercises
// a meaningful predicate path on the server side. Selection rationale:
//   - net:                 ?asn=15169 — the most common real-world filter.
//   - org/carrier:         ?name__contains=… — folded shadow column path.
//   - fac/ix/campus:       ?city__contains=london — folded shadow column path.
//   - ixlan/ixpfx:         ?limit=10&depth=1 — depth-coverage shape.
//   - else (poc/netfac/…): ?since=<unix-1h> — exercises status × since matrix.
func pdbCompatFilterPath(t string, sinceUnix int64) string {
	switch t {
	case peeringdb.TypeNet:
		return "/api/net?asn=15169"
	case peeringdb.TypeOrg:
		return "/api/org?name__contains=foo"
	case peeringdb.TypeCarrier:
		return "/api/carrier?name__contains=cogent"
	case peeringdb.TypeFac, peeringdb.TypeIX, peeringdb.TypeCampus:
		return fmt.Sprintf("/api/%s?city__contains=london", t)
	case peeringdb.TypeIXLan, peeringdb.TypeIXPfx:
		return fmt.Sprintf("/api/%s?limit=10&depth=1", t)
	default:
		return fmt.Sprintf("/api/%s?since=%d", t, sinceUnix)
	}
}

// entrestEndpoints generates the entrest (/rest/v1/<plural>...) list
// and get-by-id shapes.
func entrestEndpoints(t string, id int) []Endpoint {
	plural := restPlurals[t]
	return []Endpoint{
		{
			Surface:    SurfaceEntRest,
			EntityType: t,
			Shape:      "list-default",
			Method:     "GET",
			Path:       fmt.Sprintf("/rest/v1/%s?itemsPerPage=10", plural),
		},
		{
			Surface:    SurfaceEntRest,
			EntityType: t,
			Shape:      "get-by-id",
			Method:     "GET",
			Path:       fmt.Sprintf("/rest/v1/%s/%d", plural, id),
		},
	}
}

// graphqlEndpoints generates the GraphQL list shape per entity type.
// gqlgen / entgql conventions pluralise root list fields; the plural
// segment from restPlurals doubles as the GraphQL field name with one
// caveat — entgql lowerCamelCases hyphens; "internet-exchanges"
// becomes "internetExchanges", "ix-lans" → "ixLans", etc. Compute
// directly here.
func graphqlEndpoints(t string) []Endpoint {
	field := graphqlFieldName(t)
	body := fmt.Sprintf(`{"query":"{ %s(first: 10) { edges { node { id } } } }"}`, field)
	return []Endpoint{
		{
			Surface:    SurfaceGraphQL,
			EntityType: t,
			Shape:      "graphql-list",
			Method:     "POST",
			Path:       "/graphql",
			Body:       []byte(body),
			Header:     jsonHeader(),
		},
	}
}

// graphqlFieldName returns the lowerCamelCase GraphQL root list field
// name for the given short type. entgql pluralisation matches the REST
// plural minus hyphens.
func graphqlFieldName(t string) string {
	switch t {
	case peeringdb.TypeOrg:
		return "organizations"
	case peeringdb.TypeNet:
		return "networks"
	case peeringdb.TypeFac:
		return "facilities"
	case peeringdb.TypeIX:
		return "internetExchanges"
	case peeringdb.TypePoc:
		return "pocs"
	case peeringdb.TypeIXLan:
		return "ixLans"
	case peeringdb.TypeIXPfx:
		return "ixPrefixes"
	case peeringdb.TypeNetIXLan:
		return "networkIxLans"
	case peeringdb.TypeNetFac:
		return "networkFacilities"
	case peeringdb.TypeIXFac:
		return "ixFacilities"
	case peeringdb.TypeCarrier:
		return "carriers"
	case peeringdb.TypeCarrierFac:
		return "carrierFacilities"
	case peeringdb.TypeCampus:
		return "campuses"
	default:
		return t
	}
}

// connectRPCEndpoints generates rpc-get and rpc-list shapes per
// entity. URL form: /peeringdb.v1.<Service>/<Method>.
//
// List method names come from rpcListMethods (proto-defined plurals)
// rather than `+s` because several types have irregular plurals
// (Facilities, Campuses, IxPrefixes, etc.). Get method bodies use the
// per-type discovered id rather than a hardcoded `id:1` because the
// live mirror has sparse low-id ranges for org/poc/netfac/netixlan.
func connectRPCEndpoints(t string, id int) []Endpoint {
	svc := rpcServiceNames[t]
	method := rpcMethodNames[t]
	listMethod := rpcListMethods[t]
	return []Endpoint{
		{
			Surface:    SurfaceConnectRPC,
			EntityType: t,
			Shape:      "rpc-get",
			Method:     "POST",
			Path:       fmt.Sprintf("/peeringdb.v1.%s/Get%s", svc, method),
			Body:       fmt.Appendf(nil, `{"id":%d}`, id),
			Header:     jsonHeader(),
		},
		{
			Surface:    SurfaceConnectRPC,
			EntityType: t,
			Shape:      "rpc-list",
			Method:     "POST",
			Path:       fmt.Sprintf("/peeringdb.v1.%s/%s", svc, listMethod),
			Body:       []byte(`{"limit":10}`),
			Header:     jsonHeader(),
		},
	}
}
