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
			Comment("PeeringDB internet exchange ID"),
		field.Int("org_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("FK to organization"),
		field.String("name").
			MaxLen(64).
			NotEmpty().
			Unique().
			Annotations(
				entgql.OrderField("NAME"),
				entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray),
			).
			Comment("Internet exchange name"),
		field.String("aka").
			Optional().
			MaxLen(255).
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Also known as"),
		field.String("name_long").
			Optional().
			MaxLen(255).
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Long name"),
		field.String("city").
			MaxLen(192).
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("City"),
		field.String("country").
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Country code"),
		field.String("region_continent").
			MaxLen(255).
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Region / continent"),
		field.String("media").
			MaxLen(128).
			Default("Ethernet").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Media type"),
		field.String("notes").
			Optional().
			Default("").
			Comment("Notes"),
		field.Bool("proto_unicast").
			Default(false).
			Annotations(entrest.WithFilter(entrest.FilterEQ)).
			Comment("Supports unicast"),
		field.Bool("proto_multicast").
			Default(false).
			Comment("Supports multicast"),
		field.Bool("proto_ipv6").
			Default(false).
			Annotations(entrest.WithFilter(entrest.FilterEQ)).
			Comment("Supports IPv6"),
		field.String("website").
			Optional().
			Default("").
			Comment("Internet exchange website URL"),
		field.JSON("social_media", []SocialMedia{}).
			Optional().
			Annotations(entrest.WithSchema(socialMediaSchema())).
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
		edge.From("organization", Organization.Type).
			Ref("internet_exchanges").
			Field("org_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true)),
		edge.To("ix_lans", IxLan.Type).
			Annotations(entrest.WithEagerLoad(true)),
		edge.To("ix_facilities", IxFacility.Type).
			Annotations(entrest.WithEagerLoad(true)),
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
		entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
	}
}

// Hooks returns InternetExchange mutation hooks for OTel tracing per D-46.
func (InternetExchange) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("InternetExchange"),
	}
}
