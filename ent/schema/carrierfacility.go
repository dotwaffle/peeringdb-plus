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
			Comment("PeeringDB carrierfacility ID"),
		field.Int("carrier_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("FK to carrier"),
		field.Int("fac_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("FK to facility"),

		// Computed fields (from serializer)
		field.String("name").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Name of the facility this record refers to (computed)"),

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

// Edges of the CarrierFacility.
func (CarrierFacility) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("carrier", Carrier.Type).
			Ref("carrier_facilities").
			Field("carrier_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true)),
		edge.From("facility", Facility.Type).
			Ref("carrier_facilities").
			Field("fac_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true)),
	}
}

// Indexes of the CarrierFacility.
func (CarrierFacility) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("carrier_id"),
		index.Fields("fac_id"),
		index.Fields("status"),
		index.Fields("updated"),
		// Composite (status, updated, created, id) index. The pdbcompat
		// ?since= COUNT(*) reads it as a covering index for the status set
		// plus the updated bound (verified with EXPLAIN QUERY PLAN).
		// entrest and ConnectRPC list and stream queries order by
		// (-updated, -created, -id). SQLite reads that order from this
		// index without a temp B-tree only when the client filters on one
		// status. Their default lists have no status filter: they do not
		// use this index and sort in a temp B-tree. A leading status
		// column is required for the filtered case: the planner ignores a
		// bare updated, created, id index, reads the single-column status
		// index for the filter and then sorts. pdbcompat lists do not use
		// this index: they are ordered by id, or by updated for a ?since=
		// window.
		index.Fields("status", "updated", "created", "id"),
	}
}

// Annotations of the CarrierFacility.
func (CarrierFacility) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
		entrest.WithDefaultSort("updated"),
		entrest.WithDefaultOrder(entrest.OrderDesc),
	}
}

// Hooks returns CarrierFacility mutation hooks. Removed 2026-04-28
// (post v1.18.5): the prior otelMutationHook created one OTel span per
// mutation, which exploded the parent sync trace to >7.5MB during
// large catch-up cycles (270k upserts → 270k child spans → Tempo
// rejected the trace with TRACE_TOO_LARGE). Per-type and per-cycle
// observability is already covered by:
//   - pdbplus.sync.type.objects counter (per-type cumulative)
//   - pdbplus.sync.duration histogram (per-cycle)
//   - sync-fetch-{type} / sync-upsert-{type} per-step spans
//
// Restore on a per-Op basis only if a specific debugging need surfaces.
func (CarrierFacility) Hooks() []ent.Hook {
	return nil
}
