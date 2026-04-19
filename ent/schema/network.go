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
