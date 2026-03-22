package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// NetworkFacility holds the schema definition for the NetworkFacility entity.
// Maps to the PeeringDB "netfac" object type.
type NetworkFacility struct {
	ent.Schema
}

// Fields of the NetworkFacility.
func (NetworkFacility) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			Positive().
			Immutable().
			Comment("PeeringDB network-facility ID"),
		field.Int("net_id").
			Optional().
			Nillable().
			Comment("FK to network"),
		field.Int("fac_id").
			Optional().
			Nillable().
			Comment("FK to facility"),

		// Computed fields (from serializer, stored per D-40)
		field.String("name").
			Optional().
			Default("").
			Comment("Facility name (computed)"),
		field.String("city").
			Optional().
			Default("").
			Comment("City (computed)"),
		field.String("country").
			Optional().
			Default("").
			Comment("Country (computed)"),

		field.Int("local_asn").
			Comment("Local ASN at this facility"),

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

// Edges of the NetworkFacility.
func (NetworkFacility) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("network", Network.Type).
			Ref("network_facilities").
			Field("net_id").
			Unique(),
		edge.From("facility", Facility.Type).
			Ref("network_facilities").
			Field("fac_id").
			Unique(),
	}
}

// Indexes of the NetworkFacility.
func (NetworkFacility) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("net_id"),
		index.Fields("fac_id"),
		index.Fields("local_asn"),
	}
}

// Annotations of the NetworkFacility.
func (NetworkFacility) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
	}
}

// Hooks returns NetworkFacility mutation hooks for OTel tracing per D-46.
func (NetworkFacility) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("NetworkFacility"),
	}
}
