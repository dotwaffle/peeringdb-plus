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

// NetworkIxLan holds the schema definition for the NetworkIxLan entity.
// Maps to the PeeringDB "netixlan" object type.
type NetworkIxLan struct {
	ent.Schema
}

// Fields of the NetworkIxLan.
func (NetworkIxLan) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			Positive().
			Immutable().
			Comment("PeeringDB network-IXLan ID"),
		field.Int("net_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("FK to network"),
		field.Int("ix_id").
			Optional().
			Default(0).
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("Internet exchange ID (computed, not an edge)"),
		field.Int("ixlan_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("FK to IXLan"),

		// Computed fields (from serializer, stored per D-40)
		field.String("name").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Exchange name (computed)"),

		field.String("notes").
			Optional().
			MaxLen(255).
			Default("").
			Comment("Notes"),
		field.Int("speed").
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("Port speed in Mbps"),
		field.Int("asn").
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("Autonomous System Number"),
		field.String("ipaddr4").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("IPv4 address"),
		field.String("ipaddr6").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("IPv6 address"),
		field.Bool("is_rs_peer").
			Default(false).
			Annotations(entrest.WithFilter(entrest.FilterEQ)).
			Comment("Is route server peer"),
		field.Bool("bfd_support").
			Default(false).
			Comment("BFD support"),
		field.Bool("operational").
			Default(true).
			Annotations(entrest.WithFilter(entrest.FilterEQ)).
			Comment("Operational status"),
		field.Int("net_side_id").
			Optional().
			Nillable().
			Comment("Network-side facility ID"),
		field.Int("ix_side_id").
			Optional().
			Nillable().
			Comment("IX-side facility ID"),

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

// Edges of the NetworkIxLan.
func (NetworkIxLan) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("network", Network.Type).
			Ref("network_ix_lans").
			Field("net_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true)),
		edge.From("ix_lan", IxLan.Type).
			Ref("network_ix_lans").
			Field("ixlan_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true)),
	}
}

// Indexes of the NetworkIxLan.
func (NetworkIxLan) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("net_id"),
		index.Fields("ixlan_id"),
		index.Fields("asn"),
		index.Fields("ipaddr4"),
		index.Fields("ipaddr6"),
	}
}

// Annotations of the NetworkIxLan.
func (NetworkIxLan) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
	}
}

// Hooks returns NetworkIxLan mutation hooks for OTel tracing per D-46.
func (NetworkIxLan) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("NetworkIxLan"),
	}
}
