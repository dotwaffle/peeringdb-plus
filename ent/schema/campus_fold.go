package schema

import "entgo.io/ent"

// Mixin wires Phase 69 UNICODE-01 fold shadow columns onto Campus, plus the
// Phase 73 BUG-01 entsql.Annotation{Table: "campuses"} table-name pin
// (campusTableAnnotationMixin lives in campus_annotations.go).
// Sibling-file pattern — see network_fold.go for the rationale.
func (Campus) Mixin() []ent.Mixin {
	return []ent.Mixin{
		foldMixin{fields: []string{"name"}},
		campusTableAnnotationMixin{},
	}
}
