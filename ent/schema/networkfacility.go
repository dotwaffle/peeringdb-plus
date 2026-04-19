package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/lrstanley/entrest"

	"github.com/dotwaffle/peeringdb-plus/internal/pdbcompat/schemaannot"
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
			Comment("PeeringDB networkfacility ID"),
		field.Int("fac_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("FK to facility"),
		field.Int("net_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("FK to network"),
		field.Int("local_asn").
			Comment("Local ASN"),

		// Computed fields (from serializer, stored per D-40)
		field.String("name").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Name (computed)"),
		field.String("city").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("City (computed)"),
		field.String("country").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Country (computed)"),

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

// Edges of the NetworkFacility.
func (NetworkFacility) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("facility", Facility.Type).
			Ref("network_facilities").
			Field("fac_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true)),
		edge.From("network", Network.Type).
			Ref("network_facilities").
			Field("net_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true)),
	}
}

// Indexes of the NetworkFacility.
func (NetworkFacility) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("fac_id"),
		index.Fields("net_id"),
		index.Fields("status"),
		index.Fields("updated"),
	}
}

// Annotations of the NetworkFacility.
func (NetworkFacility) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		// Phase 70 TRAVERSAL-01: Path A allowlist mirrored from upstream
		// peeringdb_server/serializers.py:2732
		// NetworkFacilitySerializer.prepare_query. get_relation_filters seed
		// ["name", "country", "city"] is rewritten to facility__<field> at
		// line 2737; net__* filters derive from the eager-load chain
		// select_related("network", "network__org").
		schemaannot.WithPrepareQueryAllow(
			"net__name",
			"net__asn",
			"fac__name",
			"fac__country",
		),
		entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
		entrest.WithDefaultSort("updated"),
		entrest.WithDefaultOrder(entrest.OrderDesc),
	}
}

// Hooks returns NetworkFacility mutation hooks for OTel tracing per D-46.
func (NetworkFacility) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("NetworkFacility"),
	}
}
