---
phase: 70-cross-entity-traversal
plan: 01
subsystem: api
tags: [pdbcompat, ent, schema-annotations, codegen, path-a, path-b]

# Dependency graph
requires:
  - phase: 69-phase69-filter-fold
    provides: internal/pdbcompat/filter.go parseFieldOp — future extension point for __ segment splitting (D-06)
provides:
  - pdbcompat.PrepareQueryAllowAnnotation (ent schema.Annotation, Name="PrepareQueryAllow")
  - pdbcompat.FilterExcludeFromTraversalAnnotation (ent schema.Annotation, Name="FilterExcludeFromTraversal")
  - pdbcompat.WithPrepareQueryAllow(fields ...string) constructor
  - pdbcompat.WithFilterExcludeFromTraversal() constructor
  - pdbcompat.AllowlistEntry{Direct []string, Via map[string][]string} value type
affects: [70-02, 70-03, 70-04, 70-05]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "ent schema.Annotation Name() as stable grep-able string for codegen lookup"
    - "Annotation constructor copies caller slice (no aliasing) — mitigates T-70-01-01"

key-files:
  created:
    - internal/pdbcompat/annotations.go
    - internal/pdbcompat/annotations_test.go
  modified: []

key-decisions:
  - "Annotation types go in a dedicated annotations.go — not colocated with registry.go — so codegen tool (Plan 70-02) can consume the pdbcompat package without pulling in registry runtime surface"
  - "No package doc comment in annotations.go; registry.go already owns the package doc — avoid duplication"

patterns-established:
  - "Pattern: ent schema.Annotation types live in internal/pdbcompat/annotations.go with stable Name() strings locked by table-driven tests"
  - "Pattern: Constructor copies variadic slice inputs to avoid caller aliasing (applies to future WithFoo(fields ...string) additions)"

requirements-completed: [TRAVERSAL-01, TRAVERSAL-02]

# Metrics
duration: ~5min
completed: 2026-04-19
---

# Phase 70 Plan 01: pdbcompat annotation types + AllowlistEntry value type Summary

**Two exported ent schema.Annotation types (WithPrepareQueryAllow + WithFilterExcludeFromTraversal) plus AllowlistEntry value type — the Go foundation Plan 70-02's codegen tool consumes.**

## Performance

- **Duration:** ~5 min
- **Started:** 2026-04-19T (plan execution)
- **Completed:** 2026-04-19
- **Tasks:** 2 (both atomic)
- **Files modified:** 2 (both new)

## Accomplishments

- `PrepareQueryAllowAnnotation` implementing `ent/schema.Annotation` — Path A allowlist foundation (CONTEXT.md D-01)
- `FilterExcludeFromTraversalAnnotation` implementing `ent/schema.Annotation` — FILTER_EXCLUDE edge marker (CONTEXT.md D-03)
- `AllowlistEntry{Direct, Via}` value type locked — codegen-to-runtime contract for Plan 70-02 → Plan 70-05
- Round-trip tests lock the Name() strings ("PrepareQueryAllow", "FilterExcludeFromTraversal") so a silent rename can't break codegen lookup
- Aliasing test (T-70-01-01 mitigation) proves the constructor copies the caller's slice

## Task Commits

Each task was committed atomically:

1. **Task 1: Create pdbcompat annotation types** — `268346b` (feat)
2. **Task 2: Annotation round-trip test** — `41f2ceb` (test)

## Files Created/Modified

- `internal/pdbcompat/annotations.go` (92 lines) — Three exported types (`PrepareQueryAllowAnnotation`, `FilterExcludeFromTraversalAnnotation`, `AllowlistEntry`) + two constructors (`WithPrepareQueryAllow`, `WithFilterExcludeFromTraversal`). Package doc intentionally omitted (already on `registry.go`).
- `internal/pdbcompat/annotations_test.go` (68 lines) — 4 tests locking the contract: table-driven `TestWithPrepareQueryAllow` (3 sub-cases), `TestWithPrepareQueryAllow_NoAliasing`, `TestWithFilterExcludeFromTraversal`, `TestAllowlistEntry_Shape`.

## Decisions Made

None — plan executed exactly as written. Both tasks matched the plan verbatim. No scope additions, no corrective fixes, no CLAUDE.md-driven adjustments. The plan's code block was copy-paste-ready; annotations file went in 1:1. Only superficial formatting differences (godoc example uses tab indentation rather than markdown 4-space) because go fmt enforces it.

## Deviations from Plan

None — plan executed exactly as written.

## Issues Encountered

None.

## Verification

- `go build ./internal/pdbcompat/...` — pass
- `go vet ./...` — pass (clean)
- `go build ./...` — pass (full module builds; no schema import yet, so no knock-on effects)
- `go test -race ./internal/pdbcompat/ -run 'TestWithPrepareQueryAllow|TestWithFilterExcludeFromTraversal|TestAllowlistEntry_Shape' -v` — 4 tests PASS (7 with sub-cases)
- `go test -race ./internal/pdbcompat/...` — all pre-existing pdbcompat tests still pass (6.492s)
- `golangci-lint run ./internal/pdbcompat/...` — 0 issues
- `grep -c "PrepareQueryAllowAnnotation|FilterExcludeFromTraversalAnnotation|AllowlistEntry" internal/pdbcompat/annotations.go` → 13 (spec requires >=3)
- `grep -c "Name() string" internal/pdbcompat/annotations.go` → 2 (spec requires ==2)

## User Setup Required

None — pure Go internal API surface. No env vars, no external service config.

## Next Phase Readiness

- **Plan 70-02 (codegen tool):** unblocked. `cmd/pdb-compat-allowlist/` can now import `github.com/dotwaffle/peeringdb-plus/internal/pdbcompat` and reference `PrepareQueryAllowAnnotation{}.Name()` / `FilterExcludeFromTraversalAnnotation{}.Name()` to look up annotations in `gen.Graph.Nodes[i].Annotations` / `Edges[j].Annotations`. `AllowlistEntry` is the exact shape the tool should emit into `allowlist_gen.go`.
- **Plan 70-03 (13 schema annotations):** unblocked. Schemas can import `internal/pdbcompat` and call `pdbcompat.WithPrepareQueryAllow("org__name", ...)` in their `Annotations()` slice without breaking ent codegen.
- **No blockers.** All downstream plans' prerequisites on this plan are met.

## Self-Check: PASSED

- FOUND: internal/pdbcompat/annotations.go
- FOUND: internal/pdbcompat/annotations_test.go
- FOUND commit: 268346b (feat task 1)
- FOUND commit: 41f2ceb (test task 2)

---
*Phase: 70-cross-entity-traversal*
*Completed: 2026-04-19*
