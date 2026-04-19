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

// Poc holds the schema definition for the Poc entity.
// Maps to the PeeringDB "poc" object type.
type Poc struct {
	ent.Schema
}

// Fields of the Poc.
func (Poc) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			Positive().
			Immutable().
			Comment("PeeringDB poc ID"),
		field.Int("net_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("FK to network"),
		field.String("email").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Email address"),
		field.String("name").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Contact name"),
		field.String("phone").
			Optional().
			Default("").
			Comment("Phone number"),
		field.String("role").
			NotEmpty().
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Contact role"),
		field.String("url").
			Optional().
			Default("").
			Comment("URL"),
		field.String("visible").
			Optional().
			Default("Public").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Visibility level"),

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

// Edges of the Poc.
func (Poc) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("network", Network.Type).
			Ref("pocs").
			Field("net_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true)),
	}
}

// Indexes of the Poc.
func (Poc) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name"),
		index.Fields("net_id"),
		index.Fields("role"),
		index.Fields("status"),
		index.Fields("updated"),
	}
}

// Annotations of the Poc.
func (Poc) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		// Phase 70 TRAVERSAL-01: Path A allowlist mirrored from upstream
		// peeringdb_server/serializers.py:2423
		// NetworkContactSerializer.prepare_query. Upstream returns (qset, {})
		// (no get_relation_filters); client-facing net__* filters derive from
		// Meta.related_fields = ["net"] (serializers.py:2416) and
		// queryable_relations auto-introspection. Row-level visibility still
		// governed by ent Privacy policy in ent/schema/poc_policy.go.
		schemaannot.WithPrepareQueryAllow(
			"net__name",
			"net__asn",
		),
		entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
		entrest.WithDefaultSort("updated"),
		entrest.WithDefaultOrder(entrest.OrderDesc),
	}
}

// Hooks returns Poc mutation hooks for OTel tracing per D-46.
func (Poc) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("Poc"),
	}
}
