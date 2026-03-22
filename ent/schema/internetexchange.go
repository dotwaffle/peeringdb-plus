package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// InternetExchange holds the schema definition for the InternetExchange entity.
// Maps to the PeeringDB "ix" object type.
type InternetExchange struct {
	ent.Schema
}

// Fields of the InternetExchange.
func (InternetExchange) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			Positive().
			Immutable().
			Comment("PeeringDB internet exchange ID"),
		field.Int("org_id").
			Optional().
			Nillable().
			Comment("FK to organization"),
		field.String("name").
			MaxLen(64).
			NotEmpty().
			Unique().
			Annotations(
				entgql.OrderField("NAME"),
			).
			Comment("Internet exchange name"),
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
		field.String("city").
			MaxLen(192).
			Default("").
			Comment("City"),
		field.String("country").
			Default("").
			Comment("Country code"),
		field.String("region_continent").
			MaxLen(255).
			Default("").
			Comment("Region / continent"),
		field.String("media").
			MaxLen(128).
			Default("Ethernet").
			Comment("Media type"),
		field.String("notes").
			Optional().
			Default("").
			Comment("Notes"),
		field.Bool("proto_unicast").
			Default(false).
			Comment("Supports unicast"),
		field.Bool("proto_multicast").
			Default(false).
			Comment("Supports multicast"),
		field.Bool("proto_ipv6").
			Default(false).
			Comment("Supports IPv6"),
		field.String("website").
			Optional().
			Default("").
			Comment("Internet exchange website URL"),
		field.JSON("social_media", []SocialMedia{}).
			Optional().
			Comment("Social media links"),
		field.String("url_stats").
			Optional().
			Default("").
			Comment("Statistics URL"),
		field.String("tech_email").
			Optional().
			MaxLen(254).
			Default("").
			Comment("Technical contact email"),
		field.String("tech_phone").
			Optional().
			MaxLen(192).
			Default("").
			Comment("Technical contact phone"),
		field.String("policy_email").
			Optional().
			MaxLen(254).
			Default("").
			Comment("Policy contact email"),
		field.String("policy_phone").
			Optional().
			MaxLen(192).
			Default("").
			Comment("Policy contact phone"),
		field.String("sales_email").
			Optional().
			MaxLen(254).
			Default("").
			Comment("Sales contact email"),
		field.String("sales_phone").
			Optional().
			MaxLen(192).
			Default("").
			Comment("Sales contact phone"),

		// Computed fields (from serializer, stored per D-40)
		field.Int("net_count").
			Optional().
			Default(0).
			Comment("Network count (computed)"),
		field.Int("fac_count").
			Optional().
			Default(0).
			Comment("Facility count (computed)"),

		field.Int("ixf_net_count").
			Default(0).
			Comment("IXF net count"),
		field.Time("ixf_last_import").
			Optional().
			Nillable().
			Comment("Last IXF import timestamp"),
		field.String("ixf_import_request").
			Optional().
			Nillable().
			Comment("IXF import request"),
		field.String("ixf_import_request_status").
			Optional().
			Default("").
			Comment("IXF import request status"),
		field.String("service_level").
			Optional().
			MaxLen(60).
			Default("").
			Comment("Service level"),
		field.String("terms").
			Optional().
			MaxLen(60).
			Default("").
			Comment("Terms"),
		field.String("status_dashboard").
			Optional().
			Nillable().
			Comment("Status dashboard URL"),
		field.String("logo").
			Optional().
			Nillable().
			Comment("Logo URL"),

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

// Edges of the InternetExchange.
func (InternetExchange) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("organization", Organization.Type).
			Ref("internet_exchanges").
			Field("org_id").
			Unique(),
		edge.To("ix_lans", IxLan.Type),
		edge.To("ix_facilities", IxFacility.Type),
	}
}

// Indexes of the InternetExchange.
func (InternetExchange) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name"),
		index.Fields("status"),
		index.Fields("org_id"),
	}
}

// Annotations of the InternetExchange.
func (InternetExchange) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
	}
}

// Hooks returns InternetExchange mutation hooks for OTel tracing per D-46.
func (InternetExchange) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("InternetExchange"),
	}
}
