package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/lrstanley/entrest"
)

// foldMixin contributes `<fieldname>_fold` shadow columns for the Phase 69
// UNICODE-01 diacritic-insensitive matching pipeline. Each fold column is
// emitted with entgql.Skip + entrest.WithSkip so it stays off every wire
// surface — internal plumbing only. Populated by internal/sync.upsert via
// internal/unifold.Fold; consumed by the pdbcompat filter layer which
// rewrites bare `<field>__contains` filters onto the shadow column.
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
				Annotations(
					entgql.Skip(entgql.SkipAll),
					entrest.WithSkip(true),
				).
				Comment("Unicode-folded form of "+name+" for pdbcompat diacritic-insensitive matching (Phase 69 UNICODE-01; populated by internal/sync.upsert via internal/unifold.Fold)"),
		)
	}
	return out
}
