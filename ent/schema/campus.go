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
			Annotations(entproto.Field(1)).
			Comment("PeeringDB campus ID"),
		field.Int("org_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ|entrest.FilterNEQ|entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE|entrest.FilterIn|entrest.FilterNotIn), entproto.Field(2)).
			Comment("FK to organization"),
		field.String("aka").
			Optional().
			Nillable().
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
		field.String("logo").
			Optional().
			Nillable().
			Annotations(entproto.Field(6)).
			Comment("Logo URL"),
		field.String("name").
			NotEmpty().
			Unique().
			Annotations(
				entgql.OrderField("NAME"),
				entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray),
				entproto.Field(7),
			).
			Comment("Campus name"),
		field.String("name_long").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(8)).
			Comment("Long name"),
		field.String("notes").
			Optional().
			Default("").
			Annotations(entproto.Field(9)).
			Comment("Notes"),
		field.JSON("social_media", []SocialMedia{}).
			Optional().
			Annotations(entrest.WithSchema(socialMediaSchema()), entproto.Skip()).
			Comment("Social media links"),
		field.String("state").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(10)).
			Comment("State or province"),
		field.String("website").
			Optional().
			Default("").
			Annotations(entproto.Field(11)).
			Comment("Campus website URL"),
		field.String("zipcode").
			Optional().
			Default("").
			Annotations(entproto.Field(12)).
			Comment("Postal / ZIP code"),

		// Computed fields (from serializer, stored per D-40)
		field.String("org_name").
			Optional().
			Default("").
			Annotations(entproto.Field(13)).
			Comment("Org Name (computed)"),

		// HandleRefModel common fields
		field.Time("created").
			Immutable().
			Annotations(entrest.WithFilter(entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE), entproto.Field(14)).
			Comment("PeeringDB creation timestamp"),
		field.Time("updated").
			Annotations(entrest.WithFilter(entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE), entproto.Field(15)).
			Comment("PeeringDB last update timestamp"),
		field.String("status").
			Default("ok").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(16)).
			Comment("Record status"),
	}
}

// Edges of the Campus.
func (Campus) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("facilities", Facility.Type).
			Annotations(entrest.WithEagerLoad(true), entproto.Skip()),
		edge.From("organization", Organization.Type).
			Ref("campuses").
			Field("org_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true), entproto.Skip()),
	}
}

// Indexes of the Campus.
func (Campus) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name"),
		index.Fields("org_id"),
		index.Fields("status"),
		index.Fields("updated"),
		index.Fields("created"),
	}
}

// Annotations of the Campus.
func (Campus) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
		entproto.Message(entproto.PackageName("peeringdb.v1")),
	}
}

// Hooks returns Campus mutation hooks for OTel tracing per D-46.
func (Campus) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("Campus"),
	}
}
