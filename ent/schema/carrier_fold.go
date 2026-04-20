package schema

import "entgo.io/ent"

// Mixin wires Phase 69 UNICODE-01 fold shadow columns onto Carrier.
// Sibling-file pattern — see network_fold.go for the rationale.
func (Carrier) Mixin() []ent.Mixin {
	return []ent.Mixin{
		foldMixin{fields: []string{"name", "aka"}},
	}
}
