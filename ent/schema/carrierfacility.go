package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CarrierFacility holds the schema definition for the CarrierFacility entity.
// Maps to the PeeringDB "carrierfac" object type.
type CarrierFacility struct {
	ent.Schema
}

// Fields of the CarrierFacility.
func (CarrierFacility) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			Positive().
			Immutable().
			Comment("PeeringDB carrier-facility ID"),
		field.Int("carrier_id").
			Optional().
			Nillable().
			Comment("FK to carrier"),
		field.Int("fac_id").
			Optional().
			Nillable().
			Comment("FK to facility"),

		// Computed fields (from serializer, stored per D-40)
		field.String("name").
			Optional().
			Default("").
			Comment("Facility name (computed)"),

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

// Edges of the CarrierFacility.
func (CarrierFacility) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("carrier", Carrier.Type).
			Ref("carrier_facilities").
			Field("carrier_id").
			Unique(),
		edge.From("facility", Facility.Type).
			Ref("carrier_facilities").
			Field("fac_id").
			Unique(),
	}
}

// Indexes of the CarrierFacility.
func (CarrierFacility) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("carrier_id"),
		index.Fields("fac_id"),
	}
}

// Annotations of the CarrierFacility.
func (CarrierFacility) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
	}
}

// Hooks returns CarrierFacility mutation hooks for OTel tracing per D-46.
func (CarrierFacility) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("CarrierFacility"),
	}
}
