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

// Organization holds the schema definition for the Organization entity.
// Maps to the PeeringDB "org" object type.
type Organization struct {
	ent.Schema
}

// Fields of the Organization.
func (Organization) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			Positive().
			Immutable().
			Comment("PeeringDB organization ID"),
		field.String("address1").
			Optional().
			Default("").
			Comment("Address line 1"),
		field.String("address2").
			Optional().
			Default("").
			Comment("Address line 2"),
		field.String("aka").
			Optional().
			Default("").
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
		field.String("floor").
			Optional().
			Default("").
			Comment("Floor"),
		field.Float("latitude").
			Optional().
			Nillable().
			Comment("Latitude"),
		field.String("logo").
			Optional().
			Nillable().
			Comment("Logo URL"),
		field.Float("longitude").
			Optional().
			Nillable().
			Comment("Longitude"),
		field.String("name").
			NotEmpty().
			Annotations(
				entgql.OrderField("NAME"),
				entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray),
			).
			Comment("Organization name (not unique — PeeringDB permits duplicates; observed 2026-04-04 when upstream began serving duplicate display names, breaking every sync with UNIQUE constraint failed)"),
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
		field.String("state").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("State or province"),
		field.String("suite").
			Optional().
			Default("").
			Comment("Suite number"),
		field.String("website").
			Optional().
			Default("").
			Comment("Organization website URL"),
		field.String("zipcode").
			Optional().
			Default("").
			Comment("Postal / ZIP code"),

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
		field.String("aka_fold").
			Optional().
			Default("").
			Annotations(entgql.Skip(entgql.SkipAll), entrest.WithSkip(true)).
			Comment("Unicode-folded form of aka for pdbcompat diacritic-insensitive matching (Phase 69 UNICODE-01; populated by internal/sync.upsert via internal/unifold.Fold)"),
		field.String("city_fold").
			Optional().
			Default("").
			Annotations(entgql.Skip(entgql.SkipAll), entrest.WithSkip(true)).
			Comment("Unicode-folded form of city for pdbcompat diacritic-insensitive matching (Phase 69 UNICODE-01; populated by internal/sync.upsert via internal/unifold.Fold)"),
	}
}

// Edges of the Organization.
func (Organization) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("campuses", Campus.Type).
			Annotations(entrest.WithEagerLoad(true)),
		edge.To("carriers", Carrier.Type).
			Annotations(entrest.WithEagerLoad(true)),
		edge.To("facilities", Facility.Type).
			Annotations(entrest.WithEagerLoad(true)),
		edge.To("internet_exchanges", InternetExchange.Type).
			Annotations(entrest.WithEagerLoad(true)),
		edge.To("networks", Network.Type).
			Annotations(entrest.WithEagerLoad(true)),
	}
}

// Indexes of the Organization.
func (Organization) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name"),
		index.Fields("status"),
		index.Fields("updated"),
	}
}

// Annotations of the Organization.
func (Organization) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		// Phase 70 TRAVERSAL-01: Path A allowlist mirrored from upstream
		// peeringdb_server/serializers.py:4041 OrganizationSerializer.prepare_query.
		// Upstream does NOT call get_relation_filters; it only special-cases
		// the asn kwarg as net_set__asn=X (line 4053). Relation-filter surface
		// derives from queryable_relations auto-introspection (Path B). We
		// enumerate the commonly-used reverse-FK aliases.
		// DROP: distance — spatial search (convert_to_spatial_search line 4056),
		// out of Phase 70 scope.
		schemaannot.WithPrepareQueryAllow(
			"net__name",
			"net__asn",
			"ix__name",
			"fac__name",
			"fac__country",
		),
		entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
		entrest.WithDefaultSort("updated"),
		entrest.WithDefaultOrder(entrest.OrderDesc),
	}
}

// Hooks returns Organization mutation hooks for OTel tracing per D-46.
func (Organization) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("Organization"),
	}
}
