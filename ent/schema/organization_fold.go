package schema

import "entgo.io/ent"

// Mixin wires the diacritic-fold shadow columns onto Organization.
// Sibling-file pattern — see network_fold.go for the rationale.
func (Organization) Mixin() []ent.Mixin {
	return []ent.Mixin{
		foldMixin{fields: []string{"name", "aka", "city"}},
	}
}
