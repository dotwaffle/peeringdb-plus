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
			Annotations(entproto.Field(1)).
			Comment("PeeringDB internetexchange ID"),
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
		field.String("city").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(4)).
			Comment("City"),
		field.String("country").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(5)).
			Comment("Country code"),
		field.Time("ixf_last_import").
			Optional().
			Nillable().
			Annotations(entproto.Field(6)).
			Comment("IXF last import timestamp"),
		field.Int("ixf_net_count").
			Optional().
			Default(0).
			Annotations(entproto.Field(7)).
			Comment("IXF net count"),
		field.String("logo").
			Optional().
			Nillable().
			Annotations(entproto.Field(8)).
			Comment("Logo URL"),
		field.String("media").
			Optional().
			Default("Ethernet").
			Annotations(entproto.Field(9)).
			Comment("Exchange media type"),
		field.String("name").
			NotEmpty().
			Unique().
			Annotations(
				entgql.OrderField("NAME"),
				entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray),
				entproto.Field(10),
			).
			Comment("Internet exchange name"),
		field.String("name_long").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(11)).
			Comment("Long name"),
		field.String("notes").
			Optional().
			Default("").
			Annotations(entproto.Field(12)).
			Comment("Notes"),
		field.String("policy_email").
			Optional().
			Default("").
			Annotations(entproto.Field(13)).
			Comment("Policy email"),
		field.String("policy_phone").
			Optional().
			Default("").
			Annotations(entproto.Field(14)).
			Comment("Policy phone"),
		field.Bool("proto_ipv6").
			Default(false).
			Annotations(entproto.Field(15)).
			Comment("Supports IPv6"),
		field.Bool("proto_multicast").
			Default(false).
			Annotations(entproto.Field(16)).
			Comment("Supports multicast"),
		field.Bool("proto_unicast").
			Default(false).
			Annotations(entproto.Field(17)).
			Comment("Supports unicast"),
		field.String("region_continent").
			Optional().
			Default("").
			Annotations(entproto.Field(18)).
			Comment("Region/continent"),
		field.String("sales_email").
			Optional().
			Default("").
			Annotations(entproto.Field(19)).
			Comment("Sales email"),
		field.String("sales_phone").
			Optional().
			Default("").
			Annotations(entproto.Field(20)).
			Comment("Sales phone"),
		field.String("service_level").
			Optional().
			Default("").
			Annotations(entproto.Field(21)).
			Comment("Service level"),
		field.JSON("social_media", []SocialMedia{}).
			Optional().
			Annotations(entrest.WithSchema(socialMediaSchema()), entproto.Skip()).
			Comment("Social media links"),
		field.String("status_dashboard").
			Optional().
			Nillable().
			Annotations(entproto.Field(22)).
			Comment("Status dashboard URL"),
		field.String("tech_email").
			Optional().
			Default("").
			Annotations(entproto.Field(23)).
			Comment("Technical email"),
		field.String("tech_phone").
			Optional().
			Default("").
			Annotations(entproto.Field(24)).
			Comment("Technical phone"),
		field.String("terms").
			Optional().
			Default("").
			Annotations(entproto.Field(25)).
			Comment("Terms"),
		field.String("url_stats").
			Optional().
			Default("").
			Annotations(entproto.Field(26)).
			Comment("Statistics URL"),
		field.String("website").
			Optional().
			Default("").
			Annotations(entproto.Field(27)).
			Comment("IX website URL"),

		// Computed fields (from serializer, stored per D-40)
		field.Int("net_count").
			Optional().
			Default(0).
			Annotations(entproto.Field(28)).
			Comment("Net Count (computed)"),
		field.Int("fac_count").
			Optional().
			Default(0).
			Annotations(entproto.Field(29)).
			Comment("Fac Count (computed)"),
		field.String("ixf_import_request").
			Optional().
			Nillable().
			Annotations(entproto.Field(30)).
			Comment("Ixf Import Request (computed)"),
		field.String("ixf_import_request_status").
			Optional().
			Default("").
			Annotations(entproto.Field(31)).
			Comment("Ixf Import Request Status (computed)"),

		// HandleRefModel common fields
		field.Time("created").
			Immutable().
			Annotations(entrest.WithFilter(entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE), entproto.Field(32)).
			Comment("PeeringDB creation timestamp"),
		field.Time("updated").
			Annotations(entrest.WithFilter(entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE), entproto.Field(33)).
			Comment("PeeringDB last update timestamp"),
		field.String("status").
			Default("ok").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(34)).
			Comment("Record status"),
	}
}

// Edges of the InternetExchange.
func (InternetExchange) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("ix_facilities", IxFacility.Type).
			Annotations(entrest.WithEagerLoad(true), entproto.Skip()),
		edge.To("ix_lans", IxLan.Type).
			Annotations(entrest.WithEagerLoad(true), entproto.Skip()),
		edge.From("organization", Organization.Type).
			Ref("internet_exchanges").
			Field("org_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true), entproto.Skip()),
	}
}

// Indexes of the InternetExchange.
func (InternetExchange) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name"),
		index.Fields("org_id"),
		index.Fields("status"),
		index.Fields("updated"),
		index.Fields("created"),
	}
}

// Annotations of the InternetExchange.
func (InternetExchange) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
		entproto.Message(entproto.PackageName("peeringdb.v1")),
	}
}

// Hooks returns InternetExchange mutation hooks for OTel tracing per D-46.
func (InternetExchange) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("InternetExchange"),
	}
}
