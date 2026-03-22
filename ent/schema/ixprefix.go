package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
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
			Comment("FK to IXLan"),
		field.String("protocol").
			MaxLen(64).
			Comment("Protocol (IPv4 or IPv6)"),
		field.String("prefix").
			Unique().
			Comment("IP prefix"),
		field.Bool("in_dfz").
			Default(false).
			Comment("In default-free zone"),
		field.String("notes").
			Optional().
			MaxLen(255).
			Default("").
			Comment("Notes"),

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

// Edges of the IxPrefix.
func (IxPrefix) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("ix_lan", IxLan.Type).
			Ref("ix_prefixes").
			Field("ixlan_id").
			Unique(),
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
	}
}

// Hooks returns IxPrefix mutation hooks for OTel tracing per D-46.
func (IxPrefix) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("IxPrefix"),
	}
}
