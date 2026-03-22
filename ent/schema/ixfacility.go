package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// IxFacility holds the schema definition for the IxFacility entity.
// Maps to the PeeringDB "ixfac" object type.
type IxFacility struct {
	ent.Schema
}

// Fields of the IxFacility.
func (IxFacility) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			Positive().
			Immutable().
			Comment("PeeringDB IX-Facility ID"),
		field.Int("ix_id").
			Optional().
			Nillable().
			Comment("FK to internet exchange"),
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

// Edges of the IxFacility.
func (IxFacility) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("internet_exchange", InternetExchange.Type).
			Ref("ix_facilities").
			Field("ix_id").
			Unique(),
		edge.From("facility", Facility.Type).
			Ref("ix_facilities").
			Field("fac_id").
			Unique(),
	}
}

// Indexes of the IxFacility.
func (IxFacility) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("ix_id"),
		index.Fields("fac_id"),
	}
}

// Annotations of the IxFacility.
func (IxFacility) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
	}
}

// Hooks returns IxFacility mutation hooks for OTel tracing per D-46.
func (IxFacility) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("IxFacility"),
	}
}
