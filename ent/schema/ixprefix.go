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
			Comment("PeeringDB ixprefix ID"),
		field.Int("ixlan_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("FK to IX LAN"),
		field.Bool("in_dfz").
			Default(false).
			Comment("In default-free zone"),
		field.String("notes").
			Optional().
			Default("").
			Comment("Notes"),
		field.String("prefix").
			NotEmpty().
			Unique().
			Comment("IP prefix"),
		field.String("protocol").
			Optional().
			Default("").
			Comment("Protocol (IPv4/IPv6)"),

		// HandleRefModel common fields
		field.Time("created").
			Immutable().
			Annotations(entrest.WithFilter(entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE)).
			Comment("PeeringDB creation timestamp"),
		field.Time("updated").
			Annotations(entrest.WithFilter(entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE)).
			Comment("PeeringDB last update timestamp"),
		field.String("status").
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
		index.Fields("ixlan_id"),
		index.Fields("prefix"),
		index.Fields("status"),
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
