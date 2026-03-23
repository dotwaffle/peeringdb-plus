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

// IxPrefix holds the schema definition for the IxPrefix entity.
// Maps to the PeeringDB "ixpfx" object type.
type IxPrefix struct {
	ent.Schema
}

// Fields of the IxPrefix.
func (IxPrefix) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			Positive().
			Immutable().
			Comment("PeeringDB IX prefix ID"),
		field.Int("ixlan_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("FK to IXLan"),
		field.String("protocol").
			MaxLen(64).
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Protocol (IPv4 or IPv6)"),
		field.String("prefix").
			Unique().
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("IP prefix"),
		field.Bool("in_dfz").
			Default(false).
			Annotations(entrest.WithFilter(entrest.FilterEQ)).
			Comment("In default-free zone"),
		field.String("notes").
			Optional().
			MaxLen(255).
			Default("").
			Comment("Notes"),

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

// Edges of the IxPrefix.
func (IxPrefix) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("ix_lan", IxLan.Type).
			Ref("ix_prefixes").
			Field("ixlan_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true)),
	}
}

// Indexes of the IxPrefix.
func (IxPrefix) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("ixlan_id"),
		index.Fields("protocol"),
	}
}

// Annotations of the IxPrefix.
func (IxPrefix) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
	}
}

// Hooks returns IxPrefix mutation hooks for OTel tracing per D-46.
func (IxPrefix) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("IxPrefix"),
	}
}
