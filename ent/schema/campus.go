package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Campus holds the schema definition for the Campus entity.
// Maps to the PeeringDB "campus" object type.
type Campus struct {
	ent.Schema
}

// Fields of the Campus.
func (Campus) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			Positive().
			Immutable().
			Comment("PeeringDB campus ID"),
		field.Int("org_id").
			Optional().
			Nillable().
			Comment("FK to organization"),
		field.String("org_name").
			Optional().
			Comment("Organization name (computed)"),
		field.String("name").
			MaxLen(255).
			NotEmpty().
			Unique().
			Annotations(
				entgql.OrderField("NAME"),
			).
			Comment("Campus name"),
		field.String("name_long").
			Optional().
			Nillable().
			MaxLen(255).
			Comment("Long name"),
		field.String("aka").
			Optional().
			Nillable().
			MaxLen(255).
			Comment("Also known as"),
		field.String("website").
			Optional().
			Default("").
			Comment("Campus website URL"),
		field.JSON("social_media", []SocialMedia{}).
			Optional().
			Comment("Social media links"),
		field.String("notes").
			Optional().
			Default("").
			Comment("Notes"),
		field.String("country").
			Optional().
			Default("").
			Comment("Country code"),
		field.String("city").
			Optional().
			Default("").
			Comment("City"),
		field.String("zipcode").
			Optional().
			Default("").
			Comment("Postal / ZIP code"),
		field.String("state").
			Optional().
			Default("").
			Comment("State or province"),
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

// Edges of the Campus.
func (Campus) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("organization", Organization.Type).
			Ref("campuses").
			Field("org_id").
			Unique(),
		edge.To("facilities", Facility.Type),
	}
}

// Indexes of the Campus.
func (Campus) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name"),
		index.Fields("status"),
		index.Fields("org_id"),
	}
}

// Annotations of the Campus.
func (Campus) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
	}
}

// Hooks returns Campus mutation hooks for OTel tracing per D-46.
func (Campus) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("Campus"),
	}
}
