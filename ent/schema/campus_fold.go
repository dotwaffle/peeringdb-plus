package schema

import "entgo.io/ent"

// Mixin wires the diacritic-fold shadow columns onto Campus, plus the
// entsql.Annotation{Table: "campuses"} table-name pin
// (campusTableAnnotationMixin lives in campus_annotations.go).
// Sibling-file pattern — see network_fold.go for the rationale.
func (Campus) Mixin() []ent.Mixin {
	return []ent.Mixin{
		foldMixin{fields: []string{"name"}},
		campusTableAnnotationMixin{},
	}
}
