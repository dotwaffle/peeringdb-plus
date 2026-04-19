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

// InternetExchange holds the schema definition for the InternetExchange entity.
// Maps to the PeeringDB "ix" object type.
type InternetExchange struct {
	ent.Schema
}

// Fields of the InternetExchange.
func (InternetExchange) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			Positive().
			Immutable().
			Comment("PeeringDB internetexchange ID"),
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
		field.Time("ixf_last_import").
			Optional().
			Nillable().
			Comment("IXF last import timestamp"),
		field.Int("ixf_net_count").
			Optional().
			Default(0).
			Comment("IXF net count"),
		field.String("logo").
			Optional().
			Nillable().
			Comment("Logo URL"),
		field.String("media").
			Optional().
			Default("Ethernet").
			Comment("Exchange media type"),
		field.String("name").
			NotEmpty().
			Annotations(
				entgql.OrderField("NAME"),
				entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray),
			).
			Comment("Internet exchange name (not unique — PeeringDB permits duplicates)"),
		field.String("name_long").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Long name"),
		field.String("notes").
			Optional().
			Default("").
			Comment("Notes"),
		field.String("policy_email").
			Optional().
			Default("").
			Comment("Policy email"),
		field.String("policy_phone").
			Optional().
			Default("").
			Comment("Policy phone"),
		field.Bool("proto_ipv6").
			Default(false).
			Comment("Supports IPv6"),
		field.Bool("proto_multicast").
			Default(false).
			Comment("Supports multicast"),
		field.Bool("proto_unicast").
			Default(false).
			Comment("Supports unicast"),
		field.String("region_continent").
			Optional().
			Default("").
			Comment("Region/continent"),
		field.String("sales_email").
			Optional().
			Default("").
			Comment("Sales email"),
		field.String("sales_phone").
			Optional().
			Default("").
			Comment("Sales phone"),
		field.String("service_level").
			Optional().
			Default("").
			Comment("Service level"),
		field.JSON("social_media", []schematypes.SocialMedia{}).
			Optional().
			Annotations(entrest.WithSchema(socialMediaSchema())).
			Comment("Social media links"),
		field.String("status_dashboard").
			Optional().
			Nillable().
			Comment("Status dashboard URL"),
		field.String("tech_email").
			Optional().
			Default("").
			Comment("Technical email"),
		field.String("tech_phone").
			Optional().
			Default("").
			Comment("Technical phone"),
		field.String("terms").
			Optional().
			Default("").
			Comment("Terms"),
		field.String("url_stats").
			Optional().
			Default("").
			Comment("Statistics URL"),
		field.String("website").
			Optional().
			Default("").
			Comment("IX website URL"),

		// Computed fields (from serializer, stored per D-40)
		field.Int("net_count").
			Optional().
			Default(0).
			Comment("Net Count (computed)"),
		field.Int("fac_count").
			Optional().
			Default(0).
			Comment("Fac Count (computed)"),
		field.String("ixf_import_request").
			Optional().
			Nillable().
			Comment("Ixf Import Request (computed)"),
		field.String("ixf_import_request_status").
			Optional().
			Default("").
			Comment("Ixf Import Request Status (computed)"),

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
		field.String("name_long_fold").
			Optional().
			Default("").
			Annotations(entgql.Skip(entgql.SkipAll), entrest.WithSkip(true)).
			Comment("Unicode-folded form of name_long for pdbcompat diacritic-insensitive matching (Phase 69 UNICODE-01; populated by internal/sync.upsert via internal/unifold.Fold)"),
		field.String("city_fold").
			Optional().
			Default("").
			Annotations(entgql.Skip(entgql.SkipAll), entrest.WithSkip(true)).
			Comment("Unicode-folded form of city for pdbcompat diacritic-insensitive matching (Phase 69 UNICODE-01; populated by internal/sync.upsert via internal/unifold.Fold)"),
	}
}

// Edges of the InternetExchange.
func (InternetExchange) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("ix_facilities", IxFacility.Type).
			Annotations(entrest.WithEagerLoad(true)),
		edge.To("ix_lans", IxLan.Type).
			Annotations(entrest.WithEagerLoad(true)),
		edge.From("organization", Organization.Type).
			Ref("internet_exchanges").
			Field("org_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true)),
	}
}

// Indexes of the InternetExchange.
func (InternetExchange) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name"),
		index.Fields("org_id"),
		index.Fields("status"),
		index.Fields("updated"),
	}
}

// Annotations of the InternetExchange.
func (InternetExchange) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		// Phase 70 TRAVERSAL-01: Path A allowlist mirrored from upstream
		// peeringdb_server/serializers.py:3622 InternetExchangeSerializer.prepare_query.
		// get_relation_filters seeds ["ixlan", "ixfac", "fac", "net", ...] plus
		// org__* derived from select_related("org"). ixpfx__prefix exposed as
		// PDB-surface alias resolving through ix_lans.ix_prefixes (1-hop
		// via reverse FK).
		// DROP: capacity — special aggregator filter (Model.filter_capacity
		// line 3665), not a relation field.
		schemaannot.WithPrepareQueryAllow(
			"org__name",
			"ixlan__name",
			"ixpfx__prefix",
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

// Hooks returns InternetExchange mutation hooks for OTel tracing per D-46.
func (InternetExchange) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("InternetExchange"),
	}
}
