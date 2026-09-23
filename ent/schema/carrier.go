package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/lrstanley/entrest"

	"github.com/dotwaffle/peeringdb-plus/ent/schematypes"
)

// Carrier holds the schema definition for the Carrier entity.
// Maps to the PeeringDB "carrier" object type.
type Carrier struct {
	ent.Schema
}

// Fields of the Carrier.
func (Carrier) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			Positive().
			Immutable().
			Comment("PeeringDB carrier ID"),
		field.Int("org_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("FK to organization"),
		field.String("aka").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Also known as"),
		field.String("logo").
			Optional().
			Nillable().
			Comment("Logo URL"),
		field.String("name").
			Annotations(
				entgql.OrderField("NAME"),
				entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray),
			).
			Comment("Carrier name (not unique — PeeringDB permits duplicates)"),
		field.String("name_long").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Long name"),
		field.String("notes").
			Optional().
			Default("").
			Comment("Notes"),
		field.JSON("social_media", []schematypes.SocialMedia{}).
			Optional().
			Annotations(entrest.WithSchema(socialMediaSchema())).
			Comment("Social media links"),
		field.String("website").
			Optional().
			Default("").
			Comment("Carrier website URL"),

		// Computed fields (from serializer)
		field.String("org_name").
			Optional().
			Default("").
			Comment("Name of the organization this record belongs to (computed)"),
		field.Int("fac_count").
			Optional().
			Default(0).
			Comment("Number of facilities for this carrier (computed)"),

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

// Edges of the Carrier.
func (Carrier) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("carrier_facilities", CarrierFacility.Type).
			Annotations(entrest.WithEagerLoad(true)),
		edge.From("organization", Organization.Type).
			Ref("carriers").
			Field("org_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true)),
	}
}

// Indexes of the Carrier.
func (Carrier) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name"),
		index.Fields("org_id"),
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

// Annotations of the Carrier.
func (Carrier) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
		entrest.WithDefaultSort("updated"),
		entrest.WithDefaultOrder(entrest.OrderDesc),
	}
}

// Hooks returns Carrier mutation hooks. Removed 2026-04-28
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
func (Carrier) Hooks() []ent.Hook {
	return nil
}
