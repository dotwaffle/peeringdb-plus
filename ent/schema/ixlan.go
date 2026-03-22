package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
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
			Comment("PeeringDB IXLan ID"),
		field.Int("ix_id").
			Optional().
			Nillable().
			Comment("FK to internet exchange"),
		field.String("name").
			Optional().
			MaxLen(255).
			Default("").
			Comment("IXLan name"),
		field.String("descr").
			Optional().
			Default("").
			Comment("Description"),
		field.Int("mtu").
			Default(1500).
			Comment("MTU"),
		field.Bool("dot1q_support").
			Default(false).
			Comment("802.1Q support"),
		field.Int("rs_asn").
			Optional().
			Nillable().
			Default(0).
			Comment("Route server ASN"),
		field.String("arp_sponge").
			Optional().
			Nillable().
			Comment("ARP sponge MAC address"),
		field.String("ixf_ixp_member_list_url_visible").
			MaxLen(64).
			Default("Private").
			Comment("IXF member list URL visibility"),
		field.Bool("ixf_ixp_import_enabled").
			Default(false).
			Comment("IXF import enabled"),

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

// Edges of the IxLan.
func (IxLan) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("internet_exchange", InternetExchange.Type).
			Ref("ix_lans").
			Field("ix_id").
			Unique(),
		edge.To("ix_prefixes", IxPrefix.Type),
		edge.To("network_ix_lans", NetworkIxLan.Type),
	}
}

// Indexes of the IxLan.
func (IxLan) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("ix_id"),
	}
}

// Annotations of the IxLan.
func (IxLan) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
	}
}

// Hooks returns IxLan mutation hooks for OTel tracing per D-46.
func (IxLan) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("IxLan"),
	}
}
