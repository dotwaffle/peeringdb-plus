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

// Organization holds the schema definition for the Organization entity.
// Maps to the PeeringDB "org" object type.
type Organization struct {
	ent.Schema
}

// Fields of the Organization.
func (Organization) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			Positive().
			Immutable().
			Annotations(entproto.Field(1)).
			Comment("PeeringDB organization ID"),
		field.String("address1").
			Optional().
			Default("").
			Annotations(entproto.Field(2)).
			Comment("Address line 1"),
		field.String("address2").
			Optional().
			Default("").
			Annotations(entproto.Field(3)).
			Comment("Address line 2"),
		field.String("aka").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(4)).
			Comment("Also known as"),
		field.String("city").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(5)).
			Comment("City"),
		field.String("country").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(6)).
			Comment("Country code"),
		field.String("floor").
			Optional().
			Default("").
			Annotations(entproto.Field(7)).
			Comment("Floor"),
		field.Float("latitude").
			Optional().
			Nillable().
			Annotations(entproto.Field(8)).
			Comment("Latitude"),
		field.String("logo").
			Optional().
			Nillable().
			Annotations(entproto.Field(9)).
			Comment("Logo URL"),
		field.Float("longitude").
			Optional().
			Nillable().
			Annotations(entproto.Field(10)).
			Comment("Longitude"),
		field.String("name").
			NotEmpty().
			Unique().
			Annotations(
				entgql.OrderField("NAME"),
				entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray),
				entproto.Field(11),
			).
			Comment("Organization name"),
		field.String("name_long").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(12)).
			Comment("Long name"),
		field.String("notes").
			Optional().
			Default("").
			Annotations(entproto.Field(13)).
			Comment("Notes"),
		field.JSON("social_media", []SocialMedia{}).
			Optional().
			Annotations(entrest.WithSchema(socialMediaSchema()), entproto.Skip()).
			Comment("Social media links"),
		field.String("state").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(14)).
			Comment("State or province"),
		field.String("suite").
			Optional().
			Default("").
			Annotations(entproto.Field(15)).
			Comment("Suite number"),
		field.String("website").
			Optional().
			Default("").
			Annotations(entproto.Field(16)).
			Comment("Organization website URL"),
		field.String("zipcode").
			Optional().
			Default("").
			Annotations(entproto.Field(17)).
			Comment("Postal / ZIP code"),

		// Computed fields (from serializer, stored per D-40)
		field.Int("net_count").
			Optional().
			Default(0).
			Annotations(entproto.Field(18)).
			Comment("Net Count (computed)"),
		field.Int("fac_count").
			Optional().
			Default(0).
			Annotations(entproto.Field(19)).
			Comment("Fac Count (computed)"),

		// HandleRefModel common fields
		field.Time("created").
			Immutable().
			Annotations(entrest.WithFilter(entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE), entproto.Field(20)).
			Comment("PeeringDB creation timestamp"),
		field.Time("updated").
			Annotations(entrest.WithFilter(entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE), entproto.Field(21)).
			Comment("PeeringDB last update timestamp"),
		field.String("status").
			Default("ok").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(22)).
			Comment("Record status"),
	}
}

// Edges of the Organization.
func (Organization) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("campuses", Campus.Type).
			Annotations(entrest.WithEagerLoad(true), entproto.Skip()),
		edge.To("carriers", Carrier.Type).
			Annotations(entrest.WithEagerLoad(true), entproto.Skip()),
		edge.To("facilities", Facility.Type).
			Annotations(entrest.WithEagerLoad(true), entproto.Skip()),
		edge.To("internet_exchanges", InternetExchange.Type).
			Annotations(entrest.WithEagerLoad(true), entproto.Skip()),
		edge.To("networks", Network.Type).
			Annotations(entrest.WithEagerLoad(true), entproto.Skip()),
	}
}

// Indexes of the Organization.
func (Organization) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name"),
		index.Fields("status"),
	}
}

// Annotations of the Organization.
func (Organization) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
		entproto.Message(entproto.PackageName("peeringdb.v1")),
	}
}

// Hooks returns Organization mutation hooks for OTel tracing per D-46.
func (Organization) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("Organization"),
	}
}
