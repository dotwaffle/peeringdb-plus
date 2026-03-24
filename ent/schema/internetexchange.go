package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/lrstanley/entrest"
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
			Comment("PeeringDB internetexchange ID"),
		field.Int("org_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("FK to organization"),
		field.String("aka").
			MaxLen(255).
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Also known as"),
		field.String("city").
			MaxLen(192).
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("City"),
		field.String("country").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Country code"),
		field.Time("ixf_last_import").
			Optional().
			Nillable().
			Comment("IXF last import timestamp"),
		field.Int("ixf_net_count").
			Optional().
			Default(0).
			Comment("IXF net count"),
		field.String("logo").
			Optional().
			Nillable().
			Comment("Logo URL"),
		field.String("media").
			MaxLen(128).
			Optional().
			Default("Ethernet").
			Comment("Exchange media type"),
		field.String("name").
			MaxLen(64).
			NotEmpty().
			Unique().
			Annotations(
				entgql.OrderField("NAME"),
				entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray),
			).
			Comment("Internet exchange name"),
		field.String("name_long").
			MaxLen(255).
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Long name"),
		field.String("notes").
			Optional().
			Default("").
			Comment("Notes"),
		field.String("policy_email").
			MaxLen(254).
			Optional().
			Default("").
			Comment("Policy email"),
		field.String("policy_phone").
			MaxLen(192).
			Optional().
			Default("").
			Comment("Policy phone"),
		field.Bool("proto_ipv6").
			Default(false).
			Comment("Supports IPv6"),
		field.Bool("proto_multicast").
			Default(false).
			Comment("Supports multicast"),
		field.Bool("proto_unicast").
			Default(false).
			Comment("Supports unicast"),
		field.String("region_continent").
			MaxLen(255).
			Optional().
			Default("").
			Comment("Region/continent"),
		field.String("sales_email").
			MaxLen(254).
			Optional().
			Default("").
			Comment("Sales email"),
		field.String("sales_phone").
			MaxLen(192).
			Optional().
			Default("").
			Comment("Sales phone"),
		field.String("service_level").
			MaxLen(60).
			Optional().
			Default("").
			Comment("Service level"),
		field.JSON("social_media", []SocialMedia{}).
			Optional().
			Annotations(entrest.WithSchema(socialMediaSchema())).
			Comment("Social media links"),
		field.String("status_dashboard").
			Optional().
			Nillable().
			Comment("Status dashboard URL"),
		field.String("tech_email").
			MaxLen(254).
			Optional().
			Default("").
			Comment("Technical email"),
		field.String("tech_phone").
			MaxLen(192).
			Optional().
			Default("").
			Comment("Technical phone"),
		field.String("terms").
			MaxLen(60).
			Optional().
			Default("").
			Comment("Terms"),
		field.String("url_stats").
			Optional().
			Default("").
			Comment("Statistics URL"),
		field.String("website").
			Optional().
			Default("").
			Comment("IX website URL"),

		// Computed fields (from serializer, stored per D-40)
		field.Int("net_count").
			Optional().
			Default(0).
			Comment("Net Count (computed)"),
		field.Int("fac_count").
			Optional().
			Default(0).
			Comment("Fac Count (computed)"),
		field.String("ixf_import_request").
			Optional().
			Nillable().
			Comment("Ixf Import Request (computed)"),
		field.String("ixf_import_request_status").
			Optional().
			Default("").
			Comment("Ixf Import Request Status (computed)"),

		// HandleRefModel common fields
		field.Time("created").
			Immutable().
			Annotations(entrest.WithFilter(entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE)).
			Comment("PeeringDB creation timestamp"),
		field.Time("updated").
			Annotations(entrest.WithFilter(entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE)).
			Comment("PeeringDB last update timestamp"),
		field.String("status").
			MaxLen(255).
			Default("ok").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Record status"),
	}
}

// Edges of the InternetExchange.
func (InternetExchange) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("ix_facilities", IxFacility.Type).
			Annotations(entrest.WithEagerLoad(true)),
		edge.To("ix_lans", IxLan.Type).
			Annotations(entrest.WithEagerLoad(true)),
		edge.From("organization", Organization.Type).
			Ref("internet_exchanges").
			Field("org_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true)),
	}
}

// Indexes of the InternetExchange.
func (InternetExchange) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name"),
		index.Fields("org_id"),
		index.Fields("status"),
	}
}

// Annotations of the InternetExchange.
func (InternetExchange) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
	}
}

// Hooks returns InternetExchange mutation hooks for OTel tracing per D-46.
func (InternetExchange) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("InternetExchange"),
	}
}
