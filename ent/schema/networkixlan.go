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
			Annotations(entproto.Field(1)).
			Comment("PeeringDB networkixlan ID"),
		field.Int("ix_side_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ|entrest.FilterNEQ|entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE|entrest.FilterIn|entrest.FilterNotIn), entproto.Field(2)).
			Comment("IX-side facility FK"),
		field.Int("ixlan_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ|entrest.FilterNEQ|entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE|entrest.FilterIn|entrest.FilterNotIn), entproto.Field(3)).
			Comment("FK to IX LAN"),
		field.Int("net_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ|entrest.FilterNEQ|entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE|entrest.FilterIn|entrest.FilterNotIn), entproto.Field(4)).
			Comment("FK to network"),
		field.Int("net_side_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ|entrest.FilterNEQ|entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE|entrest.FilterIn|entrest.FilterNotIn), entproto.Field(5)).
			Comment("Net-side facility FK"),
		field.Int("asn").
			Positive().
			Annotations(entrest.WithFilter(entrest.FilterEQ|entrest.FilterNEQ|entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE|entrest.FilterIn|entrest.FilterNotIn), entproto.Field(6)).
			Comment("Autonomous System Number"),
		field.Bool("bfd_support").
			Default(false).
			Annotations(entproto.Field(7)).
			Comment("BFD support"),
		field.String("ipaddr4").
			Optional().
			Nillable().
			Annotations(entproto.Field(8)).
			Comment("IPv4 address"),
		field.String("ipaddr6").
			Optional().
			Nillable().
			Annotations(entproto.Field(9)).
			Comment("IPv6 address"),
		field.Bool("is_rs_peer").
			Default(false).
			Annotations(entproto.Field(10)).
			Comment("Route server peer"),
		field.String("notes").
			Optional().
			Default("").
			Annotations(entproto.Field(11)).
			Comment("Notes"),
		field.Bool("operational").
			Default(true).
			Annotations(entproto.Field(12)).
			Comment("Operational status"),
		field.Int("speed").
			Annotations(entproto.Field(13)).
			Comment("Port speed in Mbps"),

		// Computed fields (from serializer, stored per D-40)
		field.Int("ix_id").
			Optional().
			Annotations(entproto.Field(14)).
			Comment("Internet exchange ID (computed)"),
		field.String("name").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(15)).
			Comment("Name (computed)"),

		// HandleRefModel common fields
		field.Time("created").
			Immutable().
			Annotations(entrest.WithFilter(entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE), entproto.Field(16)).
			Comment("PeeringDB creation timestamp"),
		field.Time("updated").
			Annotations(entrest.WithFilter(entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE), entproto.Field(17)).
			Comment("PeeringDB last update timestamp"),
		field.String("status").
			Default("ok").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(18)).
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
			Annotations(entrest.WithEagerLoad(true), entproto.Skip()),
		edge.From("network", Network.Type).
			Ref("network_ix_lans").
			Field("net_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true), entproto.Skip()),
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
	}
}

// Annotations of the NetworkIxLan.
func (NetworkIxLan) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
		entproto.Message(entproto.PackageName("peeringdb.v1")),
	}
}

// Hooks returns NetworkIxLan mutation hooks for OTel tracing per D-46.
func (NetworkIxLan) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("NetworkIxLan"),
	}
}
