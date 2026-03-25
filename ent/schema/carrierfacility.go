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
			Annotations(entproto.Field(1)).
			Comment("PeeringDB carrierfacility ID"),
		field.Int("carrier_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ|entrest.FilterNEQ|entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE|entrest.FilterIn|entrest.FilterNotIn), entproto.Field(2)).
			Comment("FK to carrier"),
		field.Int("fac_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ|entrest.FilterNEQ|entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE|entrest.FilterIn|entrest.FilterNotIn), entproto.Field(3)).
			Comment("FK to facility"),

		// Computed fields (from serializer, stored per D-40)
		field.String("name").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(4)).
			Comment("Name (computed)"),

		// HandleRefModel common fields
		field.Time("created").
			Immutable().
			Annotations(entrest.WithFilter(entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE), entproto.Field(5)).
			Comment("PeeringDB creation timestamp"),
		field.Time("updated").
			Annotations(entrest.WithFilter(entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE), entproto.Field(6)).
			Comment("PeeringDB last update timestamp"),
		field.String("status").
			Default("ok").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(7)).
			Comment("Record status"),
	}
}

// Edges of the CarrierFacility.
func (CarrierFacility) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("carrier", Carrier.Type).
			Ref("carrier_facilities").
			Field("carrier_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true), entproto.Skip()),
		edge.From("facility", Facility.Type).
			Ref("carrier_facilities").
			Field("fac_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true), entproto.Skip()),
	}
}

// Indexes of the CarrierFacility.
func (CarrierFacility) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("carrier_id"),
		index.Fields("fac_id"),
		index.Fields("status"),
	}
}

// Annotations of the CarrierFacility.
func (CarrierFacility) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
		entproto.Message(entproto.PackageName("peeringdb.v1")),
	}
}

// Hooks returns CarrierFacility mutation hooks for OTel tracing per D-46.
func (CarrierFacility) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("CarrierFacility"),
	}
}
