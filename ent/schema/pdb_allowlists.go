package schema

import "github.com/dotwaffle/peeringdb-plus/internal/pdbcompat/schemaannot"

// PrepareQueryAllows is the hand-written source-of-truth for Phase 70
// TRAVERSAL-01 Path A allowlists. Each entry mirrors an upstream
// peeringdb_server/serializers.py get_relation_filters list verbatim
// (or the equivalent related_fields / queryable_relations derivation
// when the serializer overrides or inherits default behaviour).
//
// Consumed by cmd/pdb-compat-allowlist at codegen time to emit
// internal/pdbcompat/allowlist_gen.go. Keys are PeeringDB type strings
// ("net", "fac", etc. — the Registry/URL namespace), matching the map
// keys emitted in the generated file.
//
// Keys use TraversalKey tokens (equivalent to PeeringDB type names
// like "netfac", "netixlan") — NOT the ent edge Go names
// ("network_facilities", "network_ix_lans"). The runtime parser resolves
// entries via LookupEdge, which indexes Edges[] by TraversalKey; a
// Go-name key would be silently ignored.
//
// Living in this sibling file (rather than inside Annotations() on the
// per-entity generated ent/schema/{type}.go) keeps the source-of-truth
// safe from cmd/pdb-schema-generate, which regenerates {type}.go from
// schema/peeringdb.json and would otherwise strip these hand-authored
// annotations on every full-tree `go generate ./...`. See
// ent/schema/poc_policy.go for the original sibling-file precedent.
//
// When adding a new allowlist entry, keep the `// Source: serializers.py:<line>`
// comments — they are load-bearing for future audits against upstream
// revisions.
var PrepareQueryAllows = map[string]schemaannot.PrepareQueryAllowAnnotation{
	// Phase 70 TRAVERSAL-01: Path A allowlist mirrored from upstream
	// peeringdb_server/serializers.py:4041 OrganizationSerializer.prepare_query.
	// Upstream does NOT call get_relation_filters; it only special-cases
	// the asn kwarg as net_set__asn=X (line 4053). Relation-filter surface
	// derives from queryable_relations auto-introspection (Path B). We
	// enumerate the commonly-used reverse-FK aliases.
	// DROP: distance — spatial search (convert_to_spatial_search line 4056),
	// out of Phase 70 scope.
	"org": {
		Fields: []string{
			"net__name",
			"net__asn",
			"ix__name",
			"fac__name",
			"fac__country",
		},
	},

	// Phase 70 TRAVERSAL-01: Path A allowlist mirrored from upstream
	// peeringdb_server/serializers.py:2947 NetworkSerializer.prepare_query
	// (secondary cite: serializers.py:2995
	// NetworkSerializer.finalize_query_params — legacy info_type → info_types
	// rewrite). get_relation_filters seeds ["ixlan", "ix", "netixlan",
	// "netfac", "fac", ...] plus org__* derived from select_related("org").
	//
	// TRAVERSAL-gap: ix__name, ixlan__name, and fac__name on net are
	// listed here for upstream-parity readability only — NONE resolve
	// at runtime. Network has no direct edges to ix / ixlan / fac in
	// our ent schema; those targets are reachable only through the
	// junction entities (netixlan, netfac), which would require a
	// 3-hop traversal (net→netixlan→ixlan→ix or net→netfac→fac) and
	// exceeds the D-04 2-hop cap. The parser silent-ignores these
	// keys (TestTraversal_E2E_Matrix.upstream_5081_net_ix_name_contains
	// locks the behaviour). Kept in the list as a comment-like marker
	// so upstream-parity readers see the mapping; removing them would
	// hide the upstream shape without changing behaviour.
	"net": {
		Fields: []string{
			"org__name",
			"org__id",
			"ix__name",    // TRAVERSAL-gap: junction via netixlan, >2 hops
			"ixlan__name", // TRAVERSAL-gap: junction via netixlan, >2 hops
			"fac__name",   // TRAVERSAL-gap: junction via netfac, >2 hops
			"netfac__fac__name",
		},
	},

	// Phase 70 TRAVERSAL-01: Path A allowlist mirrored from upstream
	// peeringdb_server/serializers.py:1823 FacilitySerializer.prepare_query.
	// Concrete <fk>__<field> keys derived from the get_relation_filters
	// seed list ("net", "ix", "org_name", ...) plus the ixlan__ix__fac_count
	// 2-hop test case from pdb_api_test.py:5047,5081.
	"fac": {
		Fields: []string{
			"org__name",
			"campus__name",
			"net__name",
			"net__asn",
			"ix__name",
			"ix__id",
			"ixlan__ix__fac_count",
		},
	},

	// Phase 70 TRAVERSAL-01: Path A allowlist mirrored from upstream
	// peeringdb_server/serializers.py:3622 InternetExchangeSerializer.prepare_query.
	// get_relation_filters seeds ["ixlan", "ixfac", "fac", "net", ...] plus
	// org__* derived from select_related("org"). ixpfx__prefix exposed as
	// PDB-surface alias resolving through ix_lans.ix_prefixes (1-hop
	// via reverse FK).
	// DROP: capacity — special aggregator filter (Model.filter_capacity
	// line 3665), not a relation field.
	"ix": {
		Fields: []string{
			"org__name",
			"ixlan__name",
			"ixpfx__prefix",
			"net__name",
			"net__asn",
			"fac__name",
			"fac__country",
		},
	},

	// Phase 70 TRAVERSAL-01: Path A allowlist mirrored from upstream
	// peeringdb_server/serializers.py:3925 CampusSerializer.prepare_query.
	// get_relation_filters seed ["facility"] is rewritten to "fac_set__..."
	// at line 3936 (Django reverse-accessor). Translated to PDB-surface
	// alias fac__* which resolves through our forward edge
	// campus.facilities.* at parse time (Plan 70-05). org__name derived
	// from select_related("org").
	"campus": {
		Fields: []string{
			"org__name",
			"fac__name",
			"fac__country",
		},
	},

	// Phase 70 TRAVERSAL-01: Path A allowlist mirrored from upstream
	// peeringdb_server/serializers.py:2244 CarrierSerializer.prepare_query.
	// Upstream seed is the reverse-accessor "carrierfac_set__facility_id";
	// we translate to PDB-surface aliases fac__name / fac__country that
	// resolve through the local forward edges
	// carrier.carrier_facilities.facility at parse time (Plan 70-05).
	"carrier": {
		Fields: []string{
			"org__name",
			"fac__name",
			"fac__country",
		},
	},

	// Phase 70 TRAVERSAL-01: Path A allowlist mirrored from upstream
	// peeringdb_server/serializers.py:3451 IXLanSerializer.prepare_query.
	// Upstream returns (qset.select_related("ix", "ix__org"), {}) with no
	// get_relation_filters call; client-facing ix__* filters derive from
	// Meta.related_fields = ["ix"] (serializers.py:3444). ixpfx__prefix is
	// a reverse-FK filter commonly used to locate IxLans by the prefix
	// they contain (upstream queryable_relations auto-exposure equivalent).
	"ixlan": {
		Fields: []string{
			"ix__name",
			"ix__id",
			"ixpfx__prefix",
		},
	},

	// Phase 70 TRAVERSAL-01: Path A allowlist mirrored from upstream
	// peeringdb_server/serializers.py:3315 IXLanPrefixSerializer.prepare_query.
	// get_relation_filters seed ["ix_id", "ix", "whereis"]; we expose the
	// 2-hop ixlan__ix__{name,id} paths implied by the eager-load chain
	// select_related("ixlan", "ixlan__ix", "ixlan__ix__org") at line 3316.
	// DROP: whereis — not a relation filter (IP-in-prefix spatial search
	// via Model.whereis_ip line 3327); out of Phase 70 scope.
	"ixpfx": {
		Fields: []string{
			"ixlan__name",
			"ixlan__ix__name",
			"ixlan__ix__id",
		},
	},

	// Phase 70 TRAVERSAL-01: Path A allowlist mirrored from upstream
	// peeringdb_server/serializers.py:2361
	// InternetExchangeFacilitySerializer.prepare_query. Upstream seed
	// ["name", "country", "city"] is rewritten to facility__<field> at
	// line 2366; we expose the PDB-surface aliases directly.
	"ixfac": {
		Fields: []string{
			"fac__name",
			"fac__country",
			"fac__city",
			"ix__name",
		},
	},

	// Phase 70 TRAVERSAL-01: Path A allowlist mirrored from upstream
	// peeringdb_server/serializers.py:2732
	// NetworkFacilitySerializer.prepare_query. get_relation_filters seed
	// ["name", "country", "city"] is rewritten to facility__<field> at
	// line 2737; net__* filters derive from the eager-load chain
	// select_related("network", "network__org").
	"netfac": {
		Fields: []string{
			"net__name",
			"net__asn",
			"fac__name",
			"fac__country",
		},
	},

	// Phase 70 TRAVERSAL-01: Path A allowlist mirrored from upstream
	// peeringdb_server/serializers.py:2573 NetworkIXLanSerializer.prepare_query.
	// get_relation_filters seed ["ix_id", "ix", "name"]; upstream rewrites
	// "name" to "ix__name" at line 2579. net__* filters derive from the
	// eager-load chain select_related("network", "network__org").
	"netixlan": {
		Fields: []string{
			"net__name",
			"net__asn",
			"ix__name",
			"ix__id",
			"ixlan__name",
		},
	},

	// Phase 70 TRAVERSAL-01: Path A allowlist derived from upstream
	// peeringdb_server/serializers.py:2124 CarrierFacilitySerializer (no
	// prepare_query classmethod — inherits ModelSerializer default plus
	// queryable_relations auto-introspection). Paired upstream anchor:
	// serializers.py:2244 CarrierSerializer.prepare_query — same FK
	// reach-set as Carrier ↔ Facility via this junction table.
	// Meta.related_fields = ["carrier", "facility"] (implicit, parallels
	// IxFacility and NetworkFacility junction-table conventions).
	"carrierfac": {
		Fields: []string{
			"carrier__name",
			"fac__name",
			"fac__country",
		},
	},

	// Phase 70 TRAVERSAL-01: Path A allowlist mirrored from upstream
	// peeringdb_server/serializers.py:2423
	// NetworkContactSerializer.prepare_query. Upstream returns (qset, {})
	// (no get_relation_filters); client-facing net__* filters derive from
	// Meta.related_fields = ["net"] (serializers.py:2416) and
	// queryable_relations auto-introspection. Row-level visibility still
	// governed by ent Privacy policy in ent/schema/poc_policy.go.
	"poc": {
		Fields: []string{
			"net__name",
			"net__asn",
		},
	},
}
