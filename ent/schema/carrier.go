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
			Comment("PeeringDB carrier ID"),
		field.Int("org_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("FK to organization"),
		field.String("org_name").
			Optional().
			Default("").
			Comment("Organization name (computed)"),
		field.String("name").
			MaxLen(255).
			NotEmpty().
			Unique().
			Annotations(
				entgql.OrderField("NAME"),
				entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray),
			).
			Comment("Carrier name"),
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
		field.String("website").
			Optional().
			Default("").
			Comment("Carrier website URL"),
		field.JSON("social_media", []SocialMedia{}).
			Optional().
			Annotations(entrest.WithSchema(socialMediaSchema())).
			Comment("Social media links"),
		field.String("notes").
			Optional().
			Default("").
			Comment("Notes"),

		// Computed fields (from serializer, stored per D-40)
		field.Int("fac_count").
			Optional().
			Default(0).
			Comment("Facility count (computed)"),

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

// Edges of the Carrier.
func (Carrier) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("organization", Organization.Type).
			Ref("carriers").
			Field("org_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true)),
		edge.To("carrier_facilities", CarrierFacility.Type).
			Annotations(entrest.WithEagerLoad(true)),
	}
}

// Indexes of the Carrier.
func (Carrier) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name"),
		index.Fields("status"),
		index.Fields("org_id"),
	}
}

// Annotations of the Carrier.
func (Carrier) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
	}
}

// Hooks returns Carrier mutation hooks for OTel tracing per D-46.
func (Carrier) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("Carrier"),
	}
}
