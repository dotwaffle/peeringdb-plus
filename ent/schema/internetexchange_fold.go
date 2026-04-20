package schema

import "entgo.io/ent"

// Mixin wires Phase 69 UNICODE-01 fold shadow columns onto
// InternetExchange. Sibling-file pattern — see network_fold.go for the
// rationale.
func (InternetExchange) Mixin() []ent.Mixin {
	return []ent.Mixin{
		foldMixin{fields: []string{"name", "aka", "name_long", "city"}},
	}
}
