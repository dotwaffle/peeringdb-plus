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

// IxLan holds the schema definition for the IxLan entity.
// Maps to the PeeringDB "ixlan" object type.
type IxLan struct {
	ent.Schema
}

// Fields of the IxLan.
func (IxLan) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			Positive().
			Immutable().
			Comment("PeeringDB ixlan ID"),
		field.Int("ix_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("FK to internet exchange"),
		field.String("arp_sponge").
			Optional().
			Nillable().
			Comment("ARP sponge MAC address"),
		field.String("descr").
			Optional().
			Default("").
			Comment("Description"),
		field.Bool("dot1q_support").
			Default(false).
			Comment("802.1Q support"),
		field.Bool("ixf_ixp_import_enabled").
			Default(false).
			Comment("IXF import enabled"),
		field.String("ixf_ixp_member_list_url_visible").
			MaxLen(64).
			Optional().
			Default("Private").
			Comment("IXF member list URL visibility"),
		field.Int("mtu").
			Optional().
			Default(1500).
			Comment("MTU size"),
		field.String("name").
			MaxLen(255).
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("LAN name"),
		field.Int("rs_asn").
			Optional().
			Nillable().
			Default(0).
			Comment("Route server ASN"),

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

// Edges of the IxLan.
func (IxLan) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("internet_exchange", InternetExchange.Type).
			Ref("ix_lans").
			Field("ix_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true)),
		edge.To("ix_prefixes", IxPrefix.Type).
			Annotations(entrest.WithEagerLoad(true)),
		edge.To("network_ix_lans", NetworkIxLan.Type).
			Annotations(entrest.WithEagerLoad(true)),
	}
}

// Indexes of the IxLan.
func (IxLan) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("ix_id"),
		index.Fields("name"),
		index.Fields("status"),
	}
}

// Annotations of the IxLan.
func (IxLan) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
	}
}

// Hooks returns IxLan mutation hooks for OTel tracing per D-46.
func (IxLan) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("IxLan"),
	}
}
