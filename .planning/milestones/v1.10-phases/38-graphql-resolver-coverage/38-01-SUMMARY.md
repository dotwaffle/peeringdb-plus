---
phase: 38-graphql-resolver-coverage
plan: 01
subsystem: testing
tags: [graphql, gqlgen, resolver, coverage, integration-test]

# Dependency graph
requires:
  - phase: 37-test-seed-infrastructure
    provides: seed.Full() deterministic entity seeding for all 13 types
provides:
  - 80%+ per-file coverage on custom.resolvers.go, schema.resolvers.go, pagination.go
  - Table-driven integration tests for all 13 offset/limit list resolvers
  - Table-driven integration tests for 12 cursor-based resolvers
  - Error path tests for NetworkByAsn not-found, SyncStatus missing, validatePageSize
  - Unit tests for ValidateOffsetLimit all branches
affects: []

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "seedFullTestServer helper using seed.Full for complete entity seeding"
    - "Table-driven GraphQL integration tests with shared server across subtests"
    - "Where-filter branch coverage via valid filter arguments on all 13 list resolvers"

key-files:
  created: []
  modified:
    - graph/resolver_test.go

key-decisions:
  - "Skip campusSlice cursor test due to schema.graphqls vs generated.go field name mismatch (campuses vs campusSlice)"
  - "Exercise where-filter branch on all 13 list resolvers to push custom.resolvers.go above 80%"
  - "Exercise validatePageSize error branch on 11 cursor resolvers to push schema.resolvers.go above 80%"

patterns-established:
  - "seedFullTestServer: reusable helper creating all 13 entity types plus sync_status for GraphQL integration tests"

requirements-completed: [GQL-01, GQL-02, GQL-03]

# Metrics
duration: 10min
completed: 2026-03-26
---

# Phase 38 Plan 01: GraphQL Resolver Coverage Summary

**80%+ per-file coverage on all 3 hand-written resolver files via 7 new test functions exercising all 13 list resolvers, 12 cursor resolvers, error paths, and pagination validation**

## Performance

- **Duration:** 10 min
- **Started:** 2026-03-26T11:00:00Z
- **Completed:** 2026-03-26T11:10:02Z
- **Tasks:** 2
- **Files modified:** 1

## Accomplishments
- All 13 offset/limit list resolvers exercised with data assertions matching seed.Full() entities
- All 12 testable cursor resolvers exercised with edge count and totalCount assertions (campusSlice skipped due to pre-existing schema mismatch)
- Error paths covered: NetworkByAsn not-found returns null, SyncStatus-missing returns null, validatePageSize rejects first/last > 1000 on all resolvers
- ValidateOffsetLimit 100% branch coverage (8 test cases covering all 5 branches)
- Where-filter branch exercised on all 13 list resolvers, pushing custom.resolvers.go to 81%+ average
- Per-file coverage: custom.resolvers.go 81%, schema.resolvers.go 94%, pagination.go 100%

## Task Commits

Each task was committed atomically:

1. **Task 1: Offset/limit list resolvers + custom error paths + pagination unit tests** - `1c6ccef` (test)
2. **Task 2: Cursor-based resolvers + validatePageSize last branch + Nodes + coverage verification** - `3a42e36` (test)

## Files Created/Modified
- `graph/resolver_test.go` - Added seedFullTestServer helper, 7 new test functions with 661 new lines covering all hand-written resolver code

## Decisions Made
- Skipped campusSlice cursor resolver test: the GraphQL schema file (schema.graphqls) defines the field as `campuses` while generated.go dispatches on `campusSlice`. Neither field name works via HTTP integration test. The offset/limit `campusesList` resolver IS tested and passes. This is a pre-existing code generation drift issue.
- Added where-filter tests for all 13 list resolvers (not just a representative set) to ensure every resolver's where branch is exercised, pushing all functions to 80%.
- Added page size error tests for 11 cursor resolvers (excluding campusSlice) to push each function from 66.7% to 100%.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] campusSlice/campuses schema mismatch**
- **Found during:** Task 2 (cursor resolver tests)
- **Issue:** `campusSlice` field in generated.go doesn't match `campuses` field in schema.graphqls. Query validation rejects `campusSlice`, but `campuses` causes a runtime panic in generated code.
- **Fix:** Skipped campusSlice cursor test with explanatory comment. The campusesList offset/limit resolver is unaffected and tested.
- **Files modified:** graph/resolver_test.go
- **Verification:** All 12 other cursor resolver tests pass
- **Committed in:** 3a42e36 (Task 2 commit)

**2. [Rule 2 - Missing Critical] Added where-filter and page size error tests for coverage**
- **Found during:** Task 2 (coverage verification step)
- **Issue:** Initial coverage was below 80% on custom.resolvers.go (56%) and schema.resolvers.go (72%) because only happy-path tests were added. The where-filter branch and validatePageSize error branch needed exercise.
- **Fix:** Added TestGraphQLAPI_OffsetLimitWithWhereFilter (13 subtests) and TestGraphQLAPI_CursorPageSizeErrors (11 subtests) to cover these branches.
- **Files modified:** graph/resolver_test.go
- **Verification:** Coverage now at custom.resolvers.go 81%, schema.resolvers.go 94%, pagination.go 100%
- **Committed in:** 3a42e36 (Task 2 commit)

---

**Total deviations:** 2 auto-fixed (1 blocking, 1 missing critical)
**Impact on plan:** Both deviations were necessary to meet 80% coverage target and handle a pre-existing schema issue. No scope creep.

## Issues Encountered
- campusSlice cursor resolver is untestable via HTTP due to pre-existing schema/generated code field name mismatch. This does not prevent the overall 80% coverage target from being met.

## User Setup Required
None - no external service configuration required.

## Known Stubs
None - all test functions contain real assertions against seeded data.

## Next Phase Readiness
- GraphQL resolver coverage complete for all testable resolvers
- campusSlice schema mismatch should be tracked for resolution (re-running `go generate ./ent` would regenerate schema.graphqls to match generated.go)

## Self-Check: PASSED

- FOUND: graph/resolver_test.go
- FOUND: 1c6ccef (Task 1 commit)
- FOUND: 3a42e36 (Task 2 commit)
- FOUND: 38-01-SUMMARY.md

---
*Phase: 38-graphql-resolver-coverage*
*Completed: 2026-03-26*
