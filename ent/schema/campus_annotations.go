package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/mixin"
)

// campusTableAnnotationMixin pins the SQL table name for the Campus entity to
// "campuses". This works around go-openapi/inflect's mis-singularisation of
// "campus" → "campu" on the codegen-tool path used by
// cmd/pdb-compat-allowlist (see ent/entc.go fixCampusInflection for the
// equivalent ent-runtime patch — that one is load-bearing for ent's own
// codegen path and is NOT redundant with this annotation).
//
// Phase 73 BUG-01 (DEFER-70-06-01) fix per CONTEXT.md D-01 — the schema-level
// annotation is the single source of truth for every entc.LoadGraph consumer:
// cmd/pdb-compat-allowlist today, any future codegen tool tomorrow.
//
// Lives in this sibling file rather than inside Annotations() on the
// generated ent/schema/campus.go because cmd/pdb-schema-generate
// regenerates campus.go on every `go generate ./...` and would strip
// hand-edits. See ent/schema/poc_policy.go for the original sibling-file
// precedent and CLAUDE.md § Schema & Visibility for the convention.
type campusTableAnnotationMixin struct {
	mixin.Schema
}

// Annotations is additive to (Campus).Annotations() — ent merges mixin
// annotations with the schema's own. Therefore the entgql/entrest
// annotations on the generated campus.go and the entsql.Annotation here
// coexist without conflict.
func (campusTableAnnotationMixin) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "campuses"},
	}
}

// Compile-time assertion that the mixin satisfies ent.Mixin so adding it
// to (Campus).Mixin()'s slice is type-safe.
var _ ent.Mixin = campusTableAnnotationMixin{}
