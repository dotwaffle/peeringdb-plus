package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/contrib/entproto"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/lrstanley/entrest"
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
			Annotations(entproto.Field(1)).
			Comment("PeeringDB network ID"),
		field.Int("org_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ|entrest.FilterNEQ|entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE|entrest.FilterIn|entrest.FilterNotIn), entproto.Field(2)).
			Comment("FK to organization"),
		field.String("aka").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(3)).
			Comment("Also known as"),
		field.Bool("allow_ixp_update").
			Default(false).
			Annotations(entproto.Field(4)).
			Comment("Allow IXP update"),
		field.Int("asn").
			Unique().
			Positive().
			Annotations(entrest.WithFilter(entrest.FilterEQ|entrest.FilterNEQ|entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE|entrest.FilterIn|entrest.FilterNotIn), entproto.Field(5)).
			Comment("Autonomous System Number"),
		field.Bool("info_ipv6").
			Default(false).
			Annotations(entproto.Field(6)).
			Comment("Supports IPv6"),
		field.Bool("info_multicast").
			Default(false).
			Annotations(entproto.Field(7)).
			Comment("Supports multicast"),
		field.Bool("info_never_via_route_servers").
			Default(false).
			Annotations(entproto.Field(8)).
			Comment("Never via route servers"),
		field.Int("info_prefixes4").
			Optional().
			Nillable().
			Annotations(entproto.Field(9)).
			Comment("IPv4 prefix count"),
		field.Int("info_prefixes6").
			Optional().
			Nillable().
			Annotations(entproto.Field(10)).
			Comment("IPv6 prefix count"),
		field.String("info_ratio").
			Optional().
			Default("").
			Annotations(entproto.Field(11)).
			Comment("Traffic ratio"),
		field.String("info_scope").
			Optional().
			Default("").
			Annotations(entproto.Field(12)).
			Comment("Geographic scope"),
		field.String("info_traffic").
			Optional().
			Default("").
			Annotations(entproto.Field(13)).
			Comment("Traffic level"),
		field.String("info_type").
			Optional().
			Default("").
			Annotations(entproto.Field(14)).
			Comment("Network type"),
		field.JSON("info_types", []string{}).
			Optional().
			Annotations(entproto.Field(15)).
			Comment("Network types (multi-choice)"),
		field.Bool("info_unicast").
			Default(false).
			Annotations(entproto.Field(16)).
			Comment("Supports unicast"),
		field.String("irr_as_set").
			Optional().
			Default("").
			Annotations(entproto.Field(17)).
			Comment("IRR AS-SET"),
		field.String("logo").
			Optional().
			Nillable().
			Annotations(entproto.Field(18)).
			Comment("Logo URL"),
		field.String("looking_glass").
			Optional().
			Default("").
			Annotations(entproto.Field(19)).
			Comment("Looking glass URL"),
		field.String("name").
			NotEmpty().
			Unique().
			Annotations(
				entgql.OrderField("NAME"),
				entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray),
				entproto.Field(20),
			).
			Comment("Network name"),
		field.String("name_long").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(21)).
			Comment("Long name"),
		field.String("notes").
			Optional().
			Default("").
			Annotations(entproto.Field(22)).
			Comment("Notes"),
		field.String("policy_contracts").
			Optional().
			Default("").
			Annotations(entproto.Field(23)).
			Comment("Peering policy contracts"),
		field.String("policy_general").
			Optional().
			Default("").
			Annotations(entproto.Field(24)).
			Comment("General peering policy"),
		field.String("policy_locations").
			Optional().
			Default("").
			Annotations(entproto.Field(25)).
			Comment("Peering policy locations"),
		field.Bool("policy_ratio").
			Default(false).
			Annotations(entproto.Field(26)).
			Comment("Peering policy ratio requirement"),
		field.String("policy_url").
			Optional().
			Default("").
			Annotations(entproto.Field(27)).
			Comment("Peering policy URL"),
		field.String("rir_status").
			Optional().
			Nillable().
			Annotations(entproto.Field(28)).
			Comment("RIR status"),
		field.Time("rir_status_updated").
			Optional().
			Nillable().
			Annotations(entproto.Field(29)).
			Comment("RIR status last updated"),
		field.String("route_server").
			Optional().
			Default("").
			Annotations(entproto.Field(30)).
			Comment("Route server URL"),
		field.JSON("social_media", []SocialMedia{}).
			Optional().
			Annotations(entrest.WithSchema(socialMediaSchema()), entproto.Skip()).
			Comment("Social media links"),
		field.String("status_dashboard").
			Optional().
			Nillable().
			Annotations(entproto.Field(31)).
			Comment("Status dashboard URL"),
		field.String("website").
			Optional().
			Default("").
			Annotations(entproto.Field(32)).
			Comment("Network website URL"),

		// Computed fields (from serializer, stored per D-40)
		field.Int("ix_count").
			Optional().
			Default(0).
			Annotations(entproto.Field(33)).
			Comment("Ix Count (computed)"),
		field.Int("fac_count").
			Optional().
			Default(0).
			Annotations(entproto.Field(34)).
			Comment("Fac Count (computed)"),
		field.Time("netixlan_updated").
			Optional().
			Nillable().
			Annotations(entproto.Field(35)).
			Comment("Netixlan Updated (computed)"),
		field.Time("netfac_updated").
			Optional().
			Nillable().
			Annotations(entproto.Field(36)).
			Comment("Netfac Updated (computed)"),
		field.Time("poc_updated").
			Optional().
			Nillable().
			Annotations(entproto.Field(37)).
			Comment("Poc Updated (computed)"),

		// HandleRefModel common fields
		field.Time("created").
			Immutable().
			Annotations(entrest.WithFilter(entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE), entproto.Field(38)).
			Comment("PeeringDB creation timestamp"),
		field.Time("updated").
			Annotations(entrest.WithFilter(entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE), entproto.Field(39)).
			Comment("PeeringDB last update timestamp"),
		field.String("status").
			Default("ok").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(40)).
			Comment("Record status"),
	}
}

// Edges of the Network.
func (Network) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("network_facilities", NetworkFacility.Type).
			Annotations(entrest.WithEagerLoad(true), entproto.Skip()),
		edge.To("network_ix_lans", NetworkIxLan.Type).
			Annotations(entrest.WithEagerLoad(true), entproto.Skip()),
		edge.From("organization", Organization.Type).
			Ref("networks").
			Field("org_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true), entproto.Skip()),
		edge.To("pocs", Poc.Type).
			Annotations(entrest.WithEagerLoad(true), entproto.Skip()),
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
		index.Fields("created"),
	}
}

// Annotations of the Network.
func (Network) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
		entproto.Message(entproto.PackageName("peeringdb.v1")),
	}
}

// Hooks returns Network mutation hooks for OTel tracing per D-46.
func (Network) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("Network"),
	}
}
