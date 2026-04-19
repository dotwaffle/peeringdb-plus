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
	"github.com/dotwaffle/peeringdb-plus/internal/pdbcompat/schemaannot"
)

// Campus holds the schema definition for the Campus entity.
// Maps to the PeeringDB "campus" object type.
type Campus struct {
	ent.Schema
}

// Fields of the Campus.
func (Campus) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			Positive().
			Immutable().
			Comment("PeeringDB campus ID"),
		field.Int("org_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("FK to organization"),
		field.String("aka").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Also known as"),
		field.String("city").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("City"),
		field.String("country").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Country code"),
		field.String("logo").
			Optional().
			Nillable().
			Comment("Logo URL"),
		field.String("name").
			NotEmpty().
			Annotations(
				entgql.OrderField("NAME"),
				entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray),
			).
			Comment("Campus name (not unique — PeeringDB permits duplicates)"),
		field.String("name_long").
			Optional().
			Nillable().
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
		field.String("state").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("State or province"),
		field.String("website").
			Optional().
			Default("").
			Comment("Campus website URL"),
		field.String("zipcode").
			Optional().
			Default("").
			Comment("Postal / ZIP code"),

		// Computed fields (from serializer, stored per D-40)
		field.String("org_name").
			Optional().
			Default("").
			Comment("Org Name (computed)"),

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

		// Phase 69 UNICODE-01 shadow columns — internal plumbing for pdbcompat
		// diacritic-insensitive matching; populated by internal/sync.upsert via
		// internal/unifold.Fold. Skipped from entrest + entgql so they stay
		// server-side and do not leak to any wire surface (proto is already
		// frozen via entproto.SkipGenFile in ent/entc.go).
		field.String("name_fold").
			Optional().
			Default("").
			Annotations(entgql.Skip(entgql.SkipAll), entrest.WithSkip(true)).
			Comment("Unicode-folded form of name for pdbcompat diacritic-insensitive matching (Phase 69 UNICODE-01; populated by internal/sync.upsert via internal/unifold.Fold)"),
	}
}

// Edges of the Campus.
func (Campus) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("facilities", Facility.Type).
			Annotations(entrest.WithEagerLoad(true)),
		edge.From("organization", Organization.Type).
			Ref("campuses").
			Field("org_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true)),
	}
}

// Indexes of the Campus.
func (Campus) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name"),
		index.Fields("org_id"),
		index.Fields("status"),
		index.Fields("updated"),
	}
}

// Annotations of the Campus.
func (Campus) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		// Phase 70 TRAVERSAL-01: Path A allowlist mirrored from upstream
		// peeringdb_server/serializers.py:3925 CampusSerializer.prepare_query.
		// get_relation_filters seed ["facility"] is rewritten to "fac_set__..."
		// at line 3936 (Django reverse-accessor). Translated to PDB-surface
		// alias fac__* which resolves through our forward edge
		// campus.facilities.* at parse time (Plan 70-05). org__name derived
		// from select_related("org").
		schemaannot.WithPrepareQueryAllow(
			"org__name",
			"fac__name",
			"fac__country",
		),
		entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
		entrest.WithDefaultSort("updated"),
		entrest.WithDefaultOrder(entrest.OrderDesc),
	}
}

// Hooks returns Campus mutation hooks for OTel tracing per D-46.
func (Campus) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("Campus"),
	}
}
