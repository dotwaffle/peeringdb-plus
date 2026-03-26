---
phase: 42-test-quality-audit-coverage-hygiene
plan: 01
subsystem: testing
tags: [fuzz-testing, coverage, ci, octocov, coverpkg]

requires:
  - phase: none
    provides: existing ParseFilters implementation and CI pipeline
provides:
  - Fuzz test for filter parser (SEC-4 compliance)
  - CI coverage scoped to hand-written packages only
affects: [ci, coverage-reporting, pdbcompat]

tech-stack:
  added: []
  patterns: [fuzz-test-for-untrusted-input-parsers, coverpkg-scoped-coverage]

key-files:
  created:
    - internal/pdbcompat/fuzz_test.go
  modified:
    - .octocov.yml
    - .github/workflows/ci.yml

key-decisions:
  - "Two-pronged coverage exclusion: -coverpkg at measurement level (excludes 23 generated packages) plus octocov exclude patterns at reporting level (handles file-level exclusions like graph/generated.go and *_templ.go)"
  - "Fuzz test uses same-package access (package pdbcompat) to directly test ParseFilters with all 5 FieldType values"

patterns-established:
  - "Fuzz tests for untrusted input parsers: seed corpus covers all type variants plus edge cases, fuzz body calls parser and asserts no-panic"
  - "Coverage scoping: grep -vE pattern on go list excludes generated package trees from -coverpkg denominator"

requirements-completed: [QUAL-03, INFRA-02]

duration: 4min
completed: 2026-03-26
---

# Phase 42 Plan 01: Fuzz Test & CI Coverage Hygiene Summary

**Fuzz test for ParseFilters (275K executions, zero panics) plus CI coverage scoped to 25 hand-written packages excluding 23 generated packages**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-26T13:13:21Z
- **Completed:** 2026-03-26T13:17:42Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- FuzzFilterParser with 11-entry seed corpus covering all 5 field types and 8 operators, ran 275K+ executions in 30s with zero panics
- CI coverage denominator now excludes 23 generated packages (ent/, gen/) via -coverpkg flag
- Octocov report excludes ent/**, gen/**, graph/generated.go, and *_templ.go file patterns

## Task Commits

Each task was committed atomically:

1. **Task 1: Create fuzz test for ParseFilters** - `e93e2f6` (test)
2. **Task 2: Configure CI coverage exclusion** - `0643550` (chore)

## Files Created/Modified
- `internal/pdbcompat/fuzz_test.go` - Fuzz test exercising ParseFilters with random key/value pairs
- `.octocov.yml` - Added coverage.exclude patterns for 4 generated code categories
- `.github/workflows/ci.yml` - Added -coverpkg flag scoping coverage to hand-written packages

## Decisions Made
- Two-pronged coverage exclusion: -coverpkg at go test level plus octocov exclude at reporting level ensures both measurement and display are accurate
- graph/schema.resolvers.go and graph/custom.resolvers.go NOT excluded (hand-written resolver code that gqlgen preserves during regeneration)
- Fuzz test uses 2-argument seeds (key, value) to maximize corpus diversity

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Known Stubs
None

## Next Phase Readiness
- Fuzz test infrastructure established, can be extended for other parsers
- Coverage exclusion patterns in place for accurate reporting going forward

---
*Phase: 42-test-quality-audit-coverage-hygiene*
*Completed: 2026-03-26*
