---
phase: 42-test-quality-audit-coverage-hygiene
plan: 05
subsystem: testing
tags: [graphql, gqlgen, where-input, error-path, coverage]

requires:
  - phase: 42-03
    provides: "Error path coverage baseline and verification identifying 80% list resolver coverage gap"
provides:
  - "All 13 list resolver where.P() error branches tested via empty not clause technique"
  - "custom.resolvers.go per-resolver coverage moved from 80% to 90%"
affects: []

tech-stack:
  added: []
  patterns: ["Empty where not clause to trigger ErrEmptyXxxWhereInput for error path testing"]

key-files:
  created: []
  modified:
    - graph/resolver_test.go

key-decisions:
  - "Empty not clause (where: { not: {} }) reliably triggers where.P() error through GraphQL input layer without gqlgen rejection"

patterns-established:
  - "WhereInput error testing: send empty nested clause to exercise P() error paths in list resolvers"

requirements-completed: [QUAL-02]

duration: 3min
completed: 2026-03-26
---

# Phase 42 Plan 05: Graph Resolver Where Filter Error Path Coverage Summary

**Table-driven test covering all 13 list resolver where.P() error branches using empty not clause technique, closing QUAL-02 gap for graph/ package**

## Performance

- **Duration:** 3 min
- **Started:** 2026-03-26T13:44:58Z
- **Completed:** 2026-03-26T13:48:38Z
- **Tasks:** 1
- **Files modified:** 1

## Accomplishments
- All 13 list resolver where.P() error paths in custom.resolvers.go are now exercised by TestGraphQLAPI_WhereFilterError
- Per-resolver function coverage moved from 80% to 90% for all 13 list resolvers (OrganizationsList through CampusesList)
- The empty `not: {}` clause technique reliably triggers ErrEmptyXxxWhereInput through the standard GraphQL input layer

## Task Commits

Each task was committed atomically:

1. **Task 1: Graph resolver where.P() error path tests** - `07781b9` (test)

## Files Created/Modified
- `graph/resolver_test.go` - Added TestGraphQLAPI_WhereFilterError with 13 table-driven subtests exercising where.P() error branches

## Decisions Made
- Used `where: { not: {} }` approach to trigger errors -- gqlgen accepts empty nested objects and passes them through to the resolver, where `.P()` on the inner empty WhereInput returns ErrEmptyXxxWhereInput
- Used `id` field for ixLansList, ixFacilitiesList, and carrierFacilitiesList queries since those types lack a `name` field in the response or have limited scalar fields

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Known Stubs
None.

## Next Phase Readiness
- QUAL-02 graph resolver error path gap is now closed
- All 13 list resolvers at 90%+ function coverage in custom.resolvers.go
- The remaining 10% per function is the ValidateOffsetLimit error return, which is already covered by TestValidateOffsetLimit unit test

---
*Phase: 42-test-quality-audit-coverage-hygiene*
*Completed: 2026-03-26*

## Self-Check: PASSED
- graph/resolver_test.go: FOUND
- 42-05-SUMMARY.md: FOUND
- Commit 07781b9: FOUND
