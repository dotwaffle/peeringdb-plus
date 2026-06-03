package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/lrstanley/entrest"
)

// foldMixin contributes `<fieldname>_fold` shadow columns for the Phase 69
// UNICODE-01 diacritic-insensitive matching pipeline. Each fold column is
// internal plumbing only and must stay off every wire surface. Three guards
// achieve that, one per serializer style:
//   - entgql.Skip(SkipAll) drops the field from the GraphQL schema (gqlgen
//     builds its own types, so the field is simply not queryable).
//   - StructTag(`json:"-"`) drops it from REST: entrest serialises the raw
//     ent model with encoding/json (ent/rest/server.go Encode), so the
//     model's json tag — not entrest.WithSkip — governs the wire bytes.
//     WithSkip only removes the field from the OpenAPI/filter surface, so
//     without json:"-" the columns leaked on /rest/v1/* for all 6 folded types.
//   - entrest.WithSkip(true) keeps the field out of REST filters/sort/spec.
//
// pdbcompat is unaffected either way — it serialises hand-built DTOs, never
// the ent model. Populated by internal/sync.upsert via internal/unifold.Fold;
// consumed by the pdbcompat filter layer which rewrites bare
// `<field>__contains` filters onto the shadow column.
//
// Living in this sibling file (rather than inside the per-entity generated
// ent/schema/{type}.go) keeps the shadow-column declarations safe from
// cmd/pdb-schema-generate, which regenerates {type}.go from
// schema/peeringdb.json and would otherwise strip these hand-authored
// fields on every full-tree `go generate ./...`. ent's codegen still
// merges the mixin's fields with the generated fields via the
// {Entity}.Mixin() hook (declared in {type}_fold.go siblings).
//
// See ent/schema/poc_policy.go for the original sibling-file precedent.
type foldMixin struct {
	ent.Schema
	fields []string
}

// Fields implements ent.Mixin. Returns one `<fieldname>_fold` field per
// entry in m.fields, each carrying the entgql/entrest skip annotations
// required by Phase 69 to prevent the shadow column from leaking onto
// any wire surface.
func (m foldMixin) Fields() []ent.Field {
	out := make([]ent.Field, 0, len(m.fields))
	for _, name := range m.fields {
		out = append(out,
			field.String(name+"_fold").
				Optional().
				Default("").
				StructTag(`json:"-"`).
				Annotations(
					entgql.Skip(entgql.SkipAll),
					entrest.WithSkip(true),
				).
				Comment("Unicode-folded form of "+name+" for pdbcompat diacritic-insensitive matching (Phase 69 UNICODE-01; populated by internal/sync.upsert via internal/unifold.Fold)"),
		)
	}
	return out
}
