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

// Carrier holds the schema definition for the Carrier entity.
// Maps to the PeeringDB "carrier" object type.
type Carrier struct {
	ent.Schema
}

// Fields of the Carrier.
func (Carrier) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			Positive().
			Immutable().
			Annotations(entproto.Field(1)).
			Comment("PeeringDB carrier ID"),
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
		field.String("logo").
			Optional().
			Nillable().
			Annotations(entproto.Field(4)).
			Comment("Logo URL"),
		field.String("name").
			NotEmpty().
			Unique().
			Annotations(
				entgql.OrderField("NAME"),
				entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray),
				entproto.Field(5),
			).
			Comment("Carrier name"),
		field.String("name_long").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(6)).
			Comment("Long name"),
		field.String("notes").
			Optional().
			Default("").
			Annotations(entproto.Field(7)).
			Comment("Notes"),
		field.JSON("social_media", []SocialMedia{}).
			Optional().
			Annotations(entrest.WithSchema(socialMediaSchema()), entproto.Skip()).
			Comment("Social media links"),
		field.String("website").
			Optional().
			Default("").
			Annotations(entproto.Field(8)).
			Comment("Carrier website URL"),

		// Computed fields (from serializer, stored per D-40)
		field.String("org_name").
			Optional().
			Default("").
			Annotations(entproto.Field(9)).
			Comment("Org Name (computed)"),
		field.Int("fac_count").
			Optional().
			Default(0).
			Annotations(entproto.Field(10)).
			Comment("Fac Count (computed)"),

		// HandleRefModel common fields
		field.Time("created").
			Immutable().
			Annotations(entrest.WithFilter(entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE), entproto.Field(11)).
			Comment("PeeringDB creation timestamp"),
		field.Time("updated").
			Annotations(entrest.WithFilter(entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE), entproto.Field(12)).
			Comment("PeeringDB last update timestamp"),
		field.String("status").
			Default("ok").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(13)).
			Comment("Record status"),
	}
}

// Edges of the Carrier.
func (Carrier) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("carrier_facilities", CarrierFacility.Type).
			Annotations(entrest.WithEagerLoad(true), entproto.Skip()),
		edge.From("organization", Organization.Type).
			Ref("carriers").
			Field("org_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true), entproto.Skip()),
	}
}

// Indexes of the Carrier.
func (Carrier) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name"),
		index.Fields("org_id"),
		index.Fields("status"),
	}
}

// Annotations of the Carrier.
func (Carrier) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
		entproto.Message(entproto.PackageName("peeringdb.v1")),
	}
}

// Hooks returns Carrier mutation hooks for OTel tracing per D-46.
func (Carrier) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("Carrier"),
	}
}
