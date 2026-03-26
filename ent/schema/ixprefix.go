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
			Annotations(entproto.Field(1)).
			Comment("PeeringDB ixprefix ID"),
		field.Int("ixlan_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ|entrest.FilterNEQ|entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE|entrest.FilterIn|entrest.FilterNotIn), entproto.Field(2)).
			Comment("FK to IX LAN"),
		field.Bool("in_dfz").
			Default(false).
			Annotations(entproto.Field(3)).
			Comment("In default-free zone"),
		field.String("notes").
			Optional().
			Default("").
			Annotations(entproto.Field(4)).
			Comment("Notes"),
		field.String("prefix").
			NotEmpty().
			Unique().
			Annotations(entproto.Field(5)).
			Comment("IP prefix"),
		field.String("protocol").
			Optional().
			Default("").
			Annotations(entproto.Field(6)).
			Comment("Protocol (IPv4/IPv6)"),

		// HandleRefModel common fields
		field.Time("created").
			Immutable().
			Annotations(entrest.WithFilter(entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE), entproto.Field(7)).
			Comment("PeeringDB creation timestamp"),
		field.Time("updated").
			Annotations(entrest.WithFilter(entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE), entproto.Field(8)).
			Comment("PeeringDB last update timestamp"),
		field.String("status").
			Default("ok").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(9)).
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
			Annotations(entrest.WithEagerLoad(true), entproto.Skip()),
	}
}

// Indexes of the IxPrefix.
func (IxPrefix) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("ixlan_id"),
		index.Fields("prefix"),
		index.Fields("status"),
		index.Fields("updated"),
		index.Fields("created"),
	}
}

// Annotations of the IxPrefix.
func (IxPrefix) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
		entproto.Message(entproto.PackageName("peeringdb.v1")),
	}
}

// Hooks returns IxPrefix mutation hooks for OTel tracing per D-46.
func (IxPrefix) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("IxPrefix"),
	}
}
