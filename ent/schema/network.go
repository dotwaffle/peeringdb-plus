package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/lrstanley/entrest"

	"github.com/dotwaffle/peeringdb-plus/ent/schematypes"
	"github.com/dotwaffle/peeringdb-plus/internal/pdbcompat/schemaannot"
)

// Network holds the schema definition for the Network entity.
// Maps to the PeeringDB "net" object type.
type Network struct {
	ent.Schema
}

// Fields of the Network.
func (Network) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			Positive().
			Immutable().
			Comment("PeeringDB network ID"),
		field.Int("org_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("FK to organization"),
		field.String("aka").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Also known as"),
		field.Bool("allow_ixp_update").
			Default(false).
			Comment("Allow IXP update"),
		field.Int("asn").
			Unique().
			Positive().
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("Autonomous System Number"),
		field.Bool("info_ipv6").
			Default(false).
			Comment("Supports IPv6"),
		field.Bool("info_multicast").
			Default(false).
			Comment("Supports multicast"),
		field.Bool("info_never_via_route_servers").
			Default(false).
			Comment("Never via route servers"),
		field.Int("info_prefixes4").
			Optional().
			Nillable().
			Comment("IPv4 prefix count"),
		field.Int("info_prefixes6").
			Optional().
			Nillable().
			Comment("IPv6 prefix count"),
		field.String("info_ratio").
			Optional().
			Default("").
			Comment("Traffic ratio"),
		field.String("info_scope").
			Optional().
			Default("").
			Comment("Geographic scope"),
		field.String("info_traffic").
			Optional().
			Default("").
			Comment("Traffic level"),
		field.String("info_type").
			Optional().
			Default("").
			Comment("Network type"),
		field.JSON("info_types", []string{}).
			Optional().
			Comment("Network types (multi-choice)"),
		field.Bool("info_unicast").
			Default(false).
			Comment("Supports unicast"),
		field.String("irr_as_set").
			Optional().
			Default("").
			Comment("IRR AS-SET"),
		field.String("logo").
			Optional().
			Nillable().
			Comment("Logo URL"),
		field.String("looking_glass").
			Optional().
			Default("").
			Comment("Looking glass URL"),
		field.String("name").
			NotEmpty().
			Annotations(
				entgql.OrderField("NAME"),
				entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray),
			).
			Comment("Network name (not unique — PeeringDB permits duplicates)"),
		field.String("name_long").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Long name"),
		field.String("notes").
			Optional().
			Default("").
			Comment("Notes"),
		field.String("policy_contracts").
			Optional().
			Default("").
			Comment("Peering policy contracts"),
		field.String("policy_general").
			Optional().
			Default("").
			Comment("General peering policy"),
		field.String("policy_locations").
			Optional().
			Default("").
			Comment("Peering policy locations"),
		field.Bool("policy_ratio").
			Default(false).
			Comment("Peering policy ratio requirement"),
		field.String("policy_url").
			Optional().
			Default("").
			Comment("Peering policy URL"),
		field.String("rir_status").
			Optional().
			Nillable().
			Comment("RIR status"),
		field.Time("rir_status_updated").
			Optional().
			Nillable().
			Comment("RIR status last updated"),
		field.String("route_server").
			Optional().
			Default("").
			Comment("Route server URL"),
		field.JSON("social_media", []schematypes.SocialMedia{}).
			Optional().
			Annotations(entrest.WithSchema(socialMediaSchema())).
			Comment("Social media links"),
		field.String("status_dashboard").
			Optional().
			Nillable().
			Comment("Status dashboard URL"),
		field.String("website").
			Optional().
			Default("").
			Comment("Network website URL"),

		// Computed fields (from serializer, stored per D-40)
		field.Int("ix_count").
			Optional().
			Default(0).
			Comment("Ix Count (computed)"),
		field.Int("fac_count").
			Optional().
			Default(0).
			Comment("Fac Count (computed)"),
		field.Time("netixlan_updated").
			Optional().
			Nillable().
			Comment("Netixlan Updated (computed)"),
		field.Time("netfac_updated").
			Optional().
			Nillable().
			Comment("Netfac Updated (computed)"),
		field.Time("poc_updated").
			Optional().
			Nillable().
			Comment("Poc Updated (computed)"),

		// HandleRefModel common fields
		field.Time("created").
			Immutable().
			Annotations(entrest.WithFilter(entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE)).
			Comment("PeeringDB creation timestamp"),
		field.Time("updated").
			Annotations(
				entrest.WithFilter(entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE),
				entrest.WithSortable(true),
			).
			Comment("PeeringDB last update timestamp"),
		field.String("status").
			Default("ok").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Record status"),

		// Phase 69 UNICODE-01 shadow columns — internal plumbing for pdbcompat
		// diacritic-insensitive matching; populated by internal/sync.upsert via
		// internal/unifold.Fold. Skipped from entrest + entgql so they stay
		// server-side and do not leak to any wire surface (proto is already
		// frozen via entproto.SkipGenFile in ent/entc.go).
		field.String("name_fold").
			Optional().
			Default("").
			Annotations(entgql.Skip(entgql.SkipAll), entrest.WithSkip(true)).
			Comment("Unicode-folded form of name for pdbcompat diacritic-insensitive matching (Phase 69 UNICODE-01; populated by internal/sync.upsert via internal/unifold.Fold)"),
		field.String("aka_fold").
			Optional().
			Default("").
			Annotations(entgql.Skip(entgql.SkipAll), entrest.WithSkip(true)).
			Comment("Unicode-folded form of aka for pdbcompat diacritic-insensitive matching (Phase 69 UNICODE-01; populated by internal/sync.upsert via internal/unifold.Fold)"),
		field.String("name_long_fold").
			Optional().
			Default("").
			Annotations(entgql.Skip(entgql.SkipAll), entrest.WithSkip(true)).
			Comment("Unicode-folded form of name_long for pdbcompat diacritic-insensitive matching (Phase 69 UNICODE-01; populated by internal/sync.upsert via internal/unifold.Fold)"),
	}
}

// Edges of the Network.
func (Network) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("network_facilities", NetworkFacility.Type).
			Annotations(entrest.WithEagerLoad(true)),
		edge.To("network_ix_lans", NetworkIxLan.Type).
			Annotations(entrest.WithEagerLoad(true)),
		edge.From("organization", Organization.Type).
			Ref("networks").
			Field("org_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true)),
		edge.To("pocs", Poc.Type).
			Annotations(entrest.WithEagerLoad(true)),
	}
}

// Indexes of the Network.
func (Network) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("asn"),
		index.Fields("name"),
		index.Fields("org_id"),
		index.Fields("status"),
		index.Fields("updated"),
	}
}

// Annotations of the Network.
func (Network) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		// Phase 70 TRAVERSAL-01: Path A allowlist mirrored from upstream
		// peeringdb_server/serializers.py:2947 NetworkSerializer.prepare_query
		// (secondary cite: serializers.py:2995
		// NetworkSerializer.finalize_query_params — legacy info_type → info_types
		// rewrite). get_relation_filters seeds ["ixlan", "ix", "netixlan",
		// "netfac", "fac", ...] plus org__* derived from select_related("org").
		//
		// Keys use TraversalKey tokens (equivalent to PeeringDB type names
		// like "netfac", "netixlan") — NOT the ent edge Go name
		// ("network_facilities", "network_ix_lans"). The parser resolves
		// allowlist entries via LookupEdge, which indexes Edges[] by
		// TraversalKey; a Go-name key is silently ignored and behaves
		// identically to an unconfigured filter (Phase 70 REVIEW WR-01
		// regression — upstream spelling "network_facilities__facility__name"
		// renamed to "netfac__fac__name" here to match the runtime lookup
		// convention already used elsewhere in this schema).
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
		schemaannot.WithPrepareQueryAllow(
			"org__name",
			"org__id",
			"ix__name",    // TRAVERSAL-gap: junction via netixlan, >2 hops
			"ixlan__name", // TRAVERSAL-gap: junction via netixlan, >2 hops
			"fac__name",   // TRAVERSAL-gap: junction via netfac, >2 hops
			"netfac__fac__name",
		),
		entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
		entrest.WithDefaultSort("updated"),
		entrest.WithDefaultOrder(entrest.OrderDesc),
	}
}

// Hooks returns Network mutation hooks for OTel tracing per D-46.
func (Network) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("Network"),
	}
}
