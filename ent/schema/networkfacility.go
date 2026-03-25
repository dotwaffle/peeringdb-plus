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
			Annotations(entproto.Field(1)).
			Comment("PeeringDB networkfacility ID"),
		field.Int("fac_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ|entrest.FilterNEQ|entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE|entrest.FilterIn|entrest.FilterNotIn), entproto.Field(2)).
			Comment("FK to facility"),
		field.Int("net_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ|entrest.FilterNEQ|entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE|entrest.FilterIn|entrest.FilterNotIn), entproto.Field(3)).
			Comment("FK to network"),
		field.Int("local_asn").
			Annotations(entproto.Field(4)).
			Comment("Local ASN"),

		// Computed fields (from serializer, stored per D-40)
		field.String("name").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(5)).
			Comment("Name (computed)"),
		field.String("city").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(6)).
			Comment("City (computed)"),
		field.String("country").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(7)).
			Comment("Country (computed)"),

		// HandleRefModel common fields
		field.Time("created").
			Immutable().
			Annotations(entrest.WithFilter(entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE), entproto.Field(8)).
			Comment("PeeringDB creation timestamp"),
		field.Time("updated").
			Annotations(entrest.WithFilter(entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE), entproto.Field(9)).
			Comment("PeeringDB last update timestamp"),
		field.String("status").
			Default("ok").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(10)).
			Comment("Record status"),
	}
}

// Edges of the NetworkFacility.
func (NetworkFacility) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("facility", Facility.Type).
			Ref("network_facilities").
			Field("fac_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true), entproto.Skip()),
		edge.From("network", Network.Type).
			Ref("network_facilities").
			Field("net_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true), entproto.Skip()),
	}
}

// Indexes of the NetworkFacility.
func (NetworkFacility) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("fac_id"),
		index.Fields("net_id"),
		index.Fields("status"),
	}
}

// Annotations of the NetworkFacility.
func (NetworkFacility) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
		entproto.Message(entproto.PackageName("peeringdb.v1")),
	}
}

// Hooks returns NetworkFacility mutation hooks for OTel tracing per D-46.
func (NetworkFacility) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("NetworkFacility"),
	}
}
