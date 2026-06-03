package schema

import "entgo.io/ent"

// Mixin wires the diacritic-fold shadow columns onto Facility.
// Sibling-file pattern — see network_fold.go for the rationale.
func (Facility) Mixin() []ent.Mixin {
	return []ent.Mixin{
		foldMixin{fields: []string{"name", "aka", "city"}},
	}
}
