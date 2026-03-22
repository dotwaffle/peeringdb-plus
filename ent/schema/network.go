package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
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
			Comment("FK to organization"),
		field.String("name").
			MaxLen(255).
			NotEmpty().
			Unique().
			Annotations(
				entgql.OrderField("NAME"),
			).
			Comment("Network name"),
		field.String("aka").
			Optional().
			MaxLen(255).
			Default("").
			Comment("Also known as"),
		field.String("name_long").
			Optional().
			MaxLen(255).
			Default("").
			Comment("Long name"),
		field.String("website").
			Optional().
			Default("").
			Comment("Network website URL"),
		field.JSON("social_media", []SocialMedia{}).
			Optional().
			Comment("Social media links"),
		field.Int("asn").
			Unique().
			Positive().
			Comment("Autonomous System Number"),
		field.String("looking_glass").
			Optional().
			Default("").
			Comment("Looking glass URL"),
		field.String("route_server").
			Optional().
			Default("").
			Comment("Route server URL"),
		field.String("irr_as_set").
			Optional().
			MaxLen(255).
			Default("").
			Comment("IRR AS-SET"),
		field.String("info_type").
			Optional().
			MaxLen(60).
			Default("").
			Comment("Network type"),
		field.JSON("info_types", []string{}).
			Optional().
			Comment("Network types (multi-choice)"),
		field.Int("info_prefixes4").
			Optional().
			Nillable().
			Comment("IPv4 prefix count"),
		field.Int("info_prefixes6").
			Optional().
			Nillable().
			Comment("IPv6 prefix count"),
		field.String("info_traffic").
			Optional().
			MaxLen(39).
			Default("").
			Comment("Traffic level"),
		field.String("info_ratio").
			Optional().
			MaxLen(45).
			Default("").
			Comment("Traffic ratio"),
		field.String("info_scope").
			Optional().
			MaxLen(39).
			Default("").
			Comment("Geographic scope"),
		field.Bool("info_unicast").
			Default(false).
			Comment("Supports unicast"),
		field.Bool("info_multicast").
			Default(false).
			Comment("Supports multicast"),
		field.Bool("info_ipv6").
			Default(false).
			Comment("Supports IPv6"),
		field.Bool("info_never_via_route_servers").
			Default(false).
			Comment("Never via route servers"),
		field.String("notes").
			Optional().
			Default("").
			Comment("Notes"),
		field.String("policy_url").
			Optional().
			Default("").
			Comment("Peering policy URL"),
		field.String("policy_general").
			Optional().
			MaxLen(72).
			Default("").
			Comment("General peering policy"),
		field.String("policy_locations").
			Optional().
			MaxLen(72).
			Default("").
			Comment("Peering policy locations"),
		field.Bool("policy_ratio").
			Default(false).
			Comment("Peering policy ratio requirement"),
		field.String("policy_contracts").
			Optional().
			MaxLen(36).
			Default("").
			Comment("Peering policy contracts"),
		field.Bool("allow_ixp_update").
			Default(false).
			Comment("Allow IXP update"),
		field.String("status_dashboard").
			Optional().
			Nillable().
			Comment("Status dashboard URL"),
		field.String("rir_status").
			Optional().
			Nillable().
			MaxLen(255).
			Comment("RIR status"),
		field.Time("rir_status_updated").
			Optional().
			Nillable().
			Comment("RIR status last updated"),
		field.String("logo").
			Optional().
			Nillable().
			Comment("Logo URL"),

		// Computed fields (from serializer, stored per D-40)
		field.Int("ix_count").
			Optional().
			Default(0).
			Comment("Internet exchange count (computed)"),
		field.Int("fac_count").
			Optional().
			Default(0).
			Comment("Facility count (computed)"),
		field.Time("netixlan_updated").
			Optional().
			Nillable().
			Comment("Last netixlan update (computed)"),
		field.Time("netfac_updated").
			Optional().
			Nillable().
			Comment("Last netfac update (computed)"),
		field.Time("poc_updated").
			Optional().
			Nillable().
			Comment("Last POC update (computed)"),

		// HandleRefModel common fields
		field.Time("created").
			Immutable().
			Comment("PeeringDB creation timestamp"),
		field.Time("updated").
			Comment("PeeringDB last update timestamp"),
		field.String("status").
			MaxLen(255).
			Default("ok").
			Comment("Record status"),
	}
}

// Edges of the Network.
func (Network) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("organization", Organization.Type).
			Ref("networks").
			Field("org_id").
			Unique(),
		edge.To("pocs", Poc.Type),
		edge.To("network_facilities", NetworkFacility.Type),
		edge.To("network_ix_lans", NetworkIxLan.Type),
	}
}

// Indexes of the Network.
func (Network) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name"),
		index.Fields("status"),
		index.Fields("asn"),
		index.Fields("org_id"),
	}
}

// Annotations of the Network.
func (Network) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
	}
}

// Hooks returns Network mutation hooks for OTel tracing per D-46.
func (Network) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("Network"),
	}
}
