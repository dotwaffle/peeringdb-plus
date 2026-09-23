package schema

import "github.com/dotwaffle/peeringdb-plus/internal/pdbcompat/schemaannot"

// PrepareQueryAllows is the hand-written source-of-truth for the
// Path A traversal allowlists. Each entry is derived from the upstream
// peeringdb_server/serializers.py prepare_query of the type (its
// get_relation_filters seed list), or from related_fields /
// queryable_relations when the serializer has no such list. The keys
// are the mirror's choice of PDB-surface aliases, not a copy of an
// upstream list: some resolve keys that upstream ignores, and some
// cannot resolve here (see the per-entry notes and docs/API.md
// § Known Divergences).
//
// Line numbers cite PeeringDB 2.83.0 (465931c0).
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
// When adding a new allowlist entry, cite the upstream serializers.py
// line it derives from — the citations are load-bearing for future
// audits against upstream revisions.
var PrepareQueryAllows = map[string]schemaannot.PrepareQueryAllowAnnotation{
	// Path A allowlist mirrored from upstream
	// peeringdb_server/serializers.py:4970 OrganizationSerializer.prepare_query.
	// Upstream does NOT call get_relation_filters; it only special-cases
	// the asn kwarg as net_set__asn=X (lines 4980-4983). Relation-filter
	// surface derives from queryable_relations auto-introspection (Path B).
	// We enumerate the commonly-used reverse-FK aliases.
	// DROP: distance — spatial search (convert_to_spatial_search, lines
	// 4985-4990), out of scope for this traversal surface.
	"org": {
		Fields: []string{
			"net__name",
			"net__asn",
			"ix__name",
			"fac__name",
			"fac__country",
		},
	},

	// Path A allowlist mirrored from upstream
	// peeringdb_server/serializers.py:3708 NetworkSerializer.prepare_query
	// (secondary cite: serializers.py:3765
	// NetworkSerializer.finalize_query_params — legacy info_type → info_types
	// rewrite). get_relation_filters seeds ["ixlan", "ix", "netixlan",
	// "netfac", "fac", ...] plus org__* derived from select_related("org").
	//
	// Junction-only gap: ix__name, ixlan__name, and fac__name on net are
	// listed here for upstream-parity readability only — NONE resolve
	// at runtime. Network has no direct edges to ix / ixlan / fac in
	// our ent schema; those targets are reachable only through the
	// junction entities (netixlan, netfac), which would require a
	// 3-hop traversal (net→netixlan→ixlan→ix or net→netfac→fac) and
	// exceeds the 2-hop cap. The parser silent-ignores these
	// keys (TestTraversal_E2E_Matrix.net_ix_name_contains_ignored
	// locks the behaviour). Upstream resolves them in prepare_query, a
	// registered divergence. Kept in the list as a comment-like marker
	// so upstream-parity readers see the mapping; removing them would
	// hide the upstream shape without changing behaviour.
	"net": {
		Fields: []string{
			"org__name",
			"org__id",
			"ix__name",    // junction-only: via netixlan, >2 hops
			"ixlan__name", // junction-only: via netixlan, >2 hops
			"fac__name",   // junction-only: via netfac, >2 hops
			"netfac__fac__name",
		},
	},

	// Path A allowlist mirrored from upstream
	// peeringdb_server/serializers.py:2092 FacilitySerializer.prepare_query.
	// Concrete <fk>__<field> keys derived from the get_relation_filters
	// seed list ("net", "ix", "org_name", ...). The net__* and ix__* keys
	// do not resolve here: fac has no net or ix edge (registered
	// divergence). ixlan__ix__fac_count is not an upstream fac filter
	// either (fac has no ixlan relation); both sides ignore it.
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

	// Path A allowlist mirrored from upstream
	// peeringdb_server/serializers.py:4503 InternetExchangeSerializer.prepare_query.
	// get_relation_filters seeds ["ixlan", "ixfac", "fac", "net", ...] plus
	// org__* derived from select_related("org"). ixpfx__prefix exposed as
	// PDB-surface alias resolving through ix_lans.ix_prefixes (1-hop
	// via reverse FK).
	// DROP: capacity — special aggregator filter (Model.filter_capacity
	// line 4546), not a relation field.
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

	// Path A allowlist mirrored from upstream
	// peeringdb_server/serializers.py:4854 CampusSerializer.prepare_query.
	// get_relation_filters seed ["facility"] is rewritten to "fac_set__..."
	// at line 4865 (Django reverse-accessor). Translated to PDB-surface
	// alias fac__* which resolves through our forward edge
	// campus.facilities.* at parse time. org__name derived
	// from select_related("org").
	"campus": {
		Fields: []string{
			"org__name",
			"fac__name",
			"fac__country",
		},
	},

	// Path A allowlist mirrored from upstream
	// peeringdb_server/serializers.py:2712 CarrierSerializer.prepare_query.
	// Upstream seed is the reverse-accessor "carrierfac_set__facility_id";
	// we translate to PDB-surface aliases fac__name / fac__country that
	// resolve through the local forward edges
	// carrier.carrier_facilities.facility at parse time.
	"carrier": {
		Fields: []string{
			"org__name",
			"fac__name",
			"fac__country",
		},
	},

	// Path A allowlist mirrored from upstream
	// peeringdb_server/serializers.py:4298 IXLanSerializer.prepare_query.
	// Upstream returns (qset.select_related("ix", "ix__org"), {}) with no
	// get_relation_filters call; client-facing ix__* filters derive from
	// the ix FK through queryable_relations (serializers.py:970-996;
	// Meta.related_fields at :4291). ixpfx__prefix is
	// a reverse-FK filter commonly used to locate IxLans by the prefix
	// they contain (upstream queryable_relations auto-exposure equivalent).
	"ixlan": {
		Fields: []string{
			"ix__name",
			"ix__id",
			"ixpfx__prefix",
		},
	},

	// Path A allowlist mirrored from upstream
	// peeringdb_server/serializers.py:4154 IXLanPrefixSerializer.prepare_query.
	// get_relation_filters seed ["ix_id", "ix", "whereis"]; we expose the
	// 2-hop ixlan__ix__{name,id} paths implied by the eager-load chain
	// select_related("ixlan", "ixlan__ix", "ixlan__ix__org") at line 4155.
	// DROP: whereis — not a relation filter (IP-in-prefix search via
	// Model.whereis_ip, lines 4165-4166); out of scope for this traversal
	// surface.
	"ixpfx": {
		Fields: []string{
			"ixlan__name",
			"ixlan__ix__name",
			"ixlan__ix__id",
		},
	},

	// Path A allowlist mirrored from upstream
	// peeringdb_server/serializers.py:2837
	// InternetExchangeFacilitySerializer.prepare_query. Upstream seed
	// ["name", "country", "city"] is rewritten to facility__<field> at
	// line 2845; we expose the PDB-surface aliases directly.
	"ixfac": {
		Fields: []string{
			"fac__name",
			"fac__country",
			"fac__city",
			"ix__name",
		},
	},

	// Path A allowlist mirrored from upstream
	// peeringdb_server/serializers.py:3414
	// NetworkFacilitySerializer.prepare_query. get_relation_filters seed
	// ["name", "country", "city"] is rewritten to facility__<field> at
	// line 3422; net__* filters derive from the eager-load chain
	// select_related("network", "network__org").
	"netfac": {
		Fields: []string{
			"net__name",
			"net__asn",
			"fac__name",
			"fac__country",
		},
	},

	// Path A allowlist mirrored from upstream
	// peeringdb_server/serializers.py:3152 NetworkIXLanSerializer.prepare_query.
	// get_relation_filters seed ["ix_id", "ix", "name"]; upstream rewrites
	// "name" to "ix__name" at lines 3166-3167. net__* filters derive from the
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

	// Path A allowlist derived from upstream
	// peeringdb_server/serializers.py:2576 CarrierFacilitySerializer (no
	// prepare_query classmethod — inherits ModelSerializer default plus
	// queryable_relations auto-introspection). Paired upstream anchor:
	// serializers.py:2712 CarrierSerializer.prepare_query — same FK
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

	// Path A allowlist mirrored from upstream
	// peeringdb_server/serializers.py:2906
	// NetworkContactSerializer.prepare_query. Upstream returns (qset, {})
	// (no get_relation_filters); client-facing net__* filters derive from
	// Meta.related_fields = ["net"] (serializers.py:2899) and
	// queryable_relations auto-introspection. Row-level visibility still
	// governed by ent Privacy policy in ent/schema/poc_policy.go.
	"poc": {
		Fields: []string{
			"net__name",
			"net__asn",
		},
	},
}
