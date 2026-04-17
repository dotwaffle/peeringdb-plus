package schema

import (
	"context"

	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/lrstanley/entrest"

	pdbent "github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/poc"
	"github.com/dotwaffle/peeringdb-plus/ent/privacy"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
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
			Annotations(entrest.WithFilter(entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE)).
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
	}
}

// Annotations of the Poc.
func (Poc) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
	}
}

// Hooks returns Poc mutation hooks for OTel tracing per D-46.
func (Poc) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("Poc"),
	}
}

// Policy returns the privacy policy for the Poc entity (Phase 59 VIS-04).
//
// Query rule: rows with visible != "Public" (or NULL — see Pitfall 2
// in 59-RESEARCH.md) are filtered out for any ctx whose tier is
// TierPublic. TierUsers callers (env-override PDBPLUS_PUBLIC_TIER=users,
// or a future OAuth session) see every row. Sync workers bypass the
// policy via privacy.DecisionContext(ctx, privacy.Allow) set at worker
// entry (internal/sync/worker.go — D-08/D-09), so ingest is unaffected.
//
// No Mutation rule (D-03): sync writes travel the bypass; no other
// writers exist on this read-only mirror.
//
// NULL-safety (Pitfall 2): SQL three-valued logic makes
// `visible = 'Public'` FALSE for NULL, so we OR with VisibleIsNil for
// defence-in-depth against any future migration that leaves rows with
// unset visibility. The schema's Default("Public") also backstops this
// on inserts — two independent safeguards, per threat T-59-04.
func (Poc) Policy() ent.Policy {
	return privacy.Policy{
		Query: privacy.QueryPolicy{
			privacy.PocQueryRuleFunc(func(ctx context.Context, q *pdbent.PocQuery) error {
				if privctx.TierFrom(ctx) == privctx.TierUsers {
					return privacy.Skip
				}
				q.Where(poc.Or(
					poc.VisibleEQ("Public"),
					poc.VisibleIsNil(),
				))
				return privacy.Skip
			}),
		},
	}
}
