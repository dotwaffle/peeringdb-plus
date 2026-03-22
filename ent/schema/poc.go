package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Poc holds the schema definition for the Poc (Point of Contact) entity.
// Maps to the PeeringDB "poc" object type.
type Poc struct {
	ent.Schema
}

// Fields of the Poc.
func (Poc) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			Positive().
			Immutable().
			Comment("PeeringDB POC ID"),
		field.Int("net_id").
			Optional().
			Nillable().
			Comment("FK to network"),
		field.String("role").
			MaxLen(27).
			Comment("Contact role"),
		field.String("visible").
			MaxLen(64).
			Default("Public").
			Comment("Visibility level"),
		field.String("name").
			Optional().
			MaxLen(254).
			Default("").
			Comment("Contact name"),
		field.String("phone").
			Optional().
			MaxLen(100).
			Default("").
			Comment("Contact phone"),
		field.String("email").
			Optional().
			MaxLen(254).
			Default("").
			Comment("Contact email"),
		field.String("url").
			Optional().
			Default("").
			Comment("Contact URL"),

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

// Edges of the Poc.
func (Poc) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("network", Network.Type).
			Ref("pocs").
			Field("net_id").
			Unique(),
	}
}

// Indexes of the Poc.
func (Poc) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("net_id"),
		index.Fields("role"),
	}
}

// Annotations of the Poc.
func (Poc) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
	}
}

// Hooks returns Poc mutation hooks for OTel tracing per D-46.
func (Poc) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("Poc"),
	}
}
