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
			Comment("Obsolete. PeeringDB always reports `false`, and the value does not describe 802.1Q VLAN tagging support"),
		field.Bool("ixf_ixp_import_enabled").
			Default(false).
			Comment("IXF import enabled"),
		field.String("ixf_ixp_member_list_url").
			Optional().
			Nillable().
			Annotations(entgql.Skip(entgql.SkipWhereInput)).
			Comment("IX-F member list URL. Hidden unless ixf_ixp_member_list_url_visible lets the caller's tier see it."),
		field.String("ixf_ixp_member_list_url_visible").
			Optional().
			Default("Private").
			Comment("IXF member list URL visibility"),
		field.Int("mtu").
			Optional().
			Default(1500).
			Comment("Maximum transmission unit offered on this LAN, in bytes"),
		field.String("name").
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

// Annotations of the IxLan.
func (IxLan) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
		entrest.WithDefaultSort("updated"),
		entrest.WithDefaultOrder(entrest.OrderDesc),
	}
}

// Hooks returns IxLan mutation hooks. Removed 2026-04-28
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
func (IxLan) Hooks() []ent.Hook {
	return nil
}
