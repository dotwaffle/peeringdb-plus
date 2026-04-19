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
			Comment("PeeringDB networkixlan ID"),
		field.Int("ix_side_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("IX-side facility FK"),
		field.Int("ixlan_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("FK to IX LAN"),
		field.Int("net_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("FK to network"),
		field.Int("net_side_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("Net-side facility FK"),
		field.Int("asn").
			Positive().
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("Autonomous System Number"),
		field.Bool("bfd_support").
			Default(false).
			Comment("BFD support"),
		field.String("ipaddr4").
			Optional().
			Nillable().
			Comment("IPv4 address"),
		field.String("ipaddr6").
			Optional().
			Nillable().
			Comment("IPv6 address"),
		field.Bool("is_rs_peer").
			Default(false).
			Comment("Route server peer"),
		field.String("notes").
			Optional().
			Default("").
			Comment("Notes"),
		field.Bool("operational").
			Default(true).
			Comment("Operational status"),
		field.Int("speed").
			Comment("Port speed in Mbps"),

		// Computed fields (from serializer, stored per D-40)
		field.Int("ix_id").
			Optional().
			Comment("Internet exchange ID (computed)"),
		field.String("name").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Name (computed)"),

		// HandleRefModel common fields
		field.Time("created").
			Immutable().
			Annotations(entrest.WithFilter(entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE)).
			Comment("PeeringDB creation timestamp"),
		field.Time("updated").
			Annotations(
				entrest.WithFilter(entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE),
				entrest.WithSortable(true),
			).
			Comment("PeeringDB last update timestamp"),
		field.String("status").
			Default("ok").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Record status"),
	}
}

// Edges of the NetworkIxLan.
func (NetworkIxLan) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("ix_lan", IxLan.Type).
			Ref("network_ix_lans").
			Field("ixlan_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true)),
		edge.From("network", Network.Type).
			Ref("network_ix_lans").
			Field("net_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true)),
	}
}

// Indexes of the NetworkIxLan.
func (NetworkIxLan) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("ix_side_id"),
		index.Fields("ixlan_id"),
		index.Fields("net_id"),
		index.Fields("net_side_id"),
		index.Fields("status"),
		index.Fields("updated"),
	}
}

// Annotations of the NetworkIxLan.
func (NetworkIxLan) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
		entrest.WithDefaultSort("updated"),
		entrest.WithDefaultOrder(entrest.OrderDesc),
	}
}

// Hooks returns NetworkIxLan mutation hooks for OTel tracing per D-46.
func (NetworkIxLan) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("NetworkIxLan"),
	}
}
