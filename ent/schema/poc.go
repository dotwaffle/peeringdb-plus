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
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("FK to network"),
		field.String("role").
			MaxLen(27).
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Contact role"),
		field.String("visible").
			MaxLen(64).
			Default("Public").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Visibility level"),
		field.String("name").
			Optional().
			MaxLen(254).
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
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
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Contact email"),
		field.String("url").
			Optional().
			Default("").
			Comment("Contact URL"),

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

// Edges of the Poc.
func (Poc) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("network", Network.Type).
			Ref("pocs").
			Field("net_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true)),
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
		entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
	}
}

// Hooks returns Poc mutation hooks for OTel tracing per D-46.
func (Poc) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("Poc"),
	}
}
