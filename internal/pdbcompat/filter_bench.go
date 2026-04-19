//go:build bench
// +build bench

// Package pdbcompat: bench-only helpers for Phase 69 Plan 05 index-decision
// benchmarks. See bench_test.go. NEVER compiled in production builds;
// guarded by //go:build bench so non-bench `go build ./...` never sees
// these symbols (GO-CC-3: no package-level mutable production state).
//
// Rationale: an earlier draft of Plan 05 proposed a package-level
// `disableShadowRoute bool` inside production filter.go to flip the
// shadow-routing path at bench time. That would have required editing
// filter.go (locked by Plan 69-04) AND added mutable state to the
// production package. The build-tag-gated shim moves both the toggle and
// the benchmark out of the default build entirely. `go test -tags=bench`
// is the only code path that ever sees these two helpers.
package pdbcompat

import (
	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// directContainsPredicate reproduces the Phase 68 non-shadow path
// (sql.FieldContainsFold on the raw column with the raw query value) for
// benchmark comparison against the Phase 69 shadow path. Used ONLY by
// bench_test.go under -tags=bench — there is no production caller.
func directContainsPredicate(field, value string) func(*sql.Selector) {
	return sql.FieldContainsFold(field, value)
}

// shadowContainsPredicate reproduces the Phase 69 shadow path
// (sql.FieldContainsFold on the <field>_fold sibling column with
// unifold.Fold(value) as the RHS). Used ONLY by bench_test.go under
// -tags=bench — there is no production caller. Production code reaches
// the same shape via buildContains(field, value, ft, folded=true) inside
// internal/pdbcompat/filter.go.
func shadowContainsPredicate(field, value string) func(*sql.Selector) {
	return sql.FieldContainsFold(field+"_fold", unifold.Fold(value))
}
