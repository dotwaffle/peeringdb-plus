---
phase: 49-refactoring-tech-debt
plan: 03
subsystem: testing
tags: [graphql, sqlite, gqlgen, ent, database, integration-test, pragma]

# Dependency graph
requires:
  - phase: 48-response-hardening-internal-quality
    provides: classifyError with ent type checks (errors.As), database pool config
provides:
  - GraphQL handler integration tests (error presenter, complexity limit, depth limit)
  - database.Open pragma verification tests (WAL, foreign_keys, busy_timeout, pool config)
affects: [49-refactoring-tech-debt]

# Tech tracking
tech-stack:
  added: []
  patterns: [httptest+gqlgen integration testing, pragma verification via raw SQL QueryRow]

key-files:
  created:
    - internal/database/database_test.go
  modified:
    - internal/graphql/handler_test.go

key-decisions:
  - "100 aliased queries to exceed gqlgen default complexity limit (1 per field, no pagination multiplier)"
  - "17-level depth via org->networks->org traversal to exceed depth limit of 15"

patterns-established:
  - "postGQL helper: httptest.NewRecorder + json.Marshal for in-process GraphQL handler testing"
  - "Table-driven pragma tests: QueryRow PRAGMA to verify SQLite configuration"

requirements-completed: [QUAL-01, QUAL-02]

# Metrics
duration: 9min
completed: 2026-04-02
---

# Phase 49 Plan 03: GraphQL Handler & Database Test Coverage Summary

**Integration tests for GraphQL error presenter, complexity/depth limits, and SQLite pragma verification via database.Open**

## Performance

- **Duration:** 9 min
- **Started:** 2026-04-02T05:04:21Z
- **Completed:** 2026-04-02T05:14:19Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments
- GraphQL handler tested for error presenter extensions.code, complexity limit rejection (500), and depth limit rejection (15)
- database.Open tested for WAL journal mode, foreign key enforcement, busy timeout (5000ms), and connection pool MaxOpenConnections (10)
- All tests pass with -race detector

## Task Commits

Each task was committed atomically:

1. **Task 1: Add GraphQL handler tests for error presenter and query limits** - `3a85e1d` (test)
2. **Task 2: Add database.Open tests for pragma verification and error paths** - `ace7a4f` (test)

## Files Created/Modified
- `internal/graphql/handler_test.go` - Added TestErrorPresenter_SetsCodeExtension, TestComplexityLimit_RejectsComplex, TestDepthLimit_RejectsDeep
- `internal/database/database_test.go` - Created with TestOpen_Success, TestOpen_Pragmas (table-driven), TestOpen_PoolConfig

## Decisions Made
- gqlgen's default FixedComplexityLimit counts 1 per field selection without pagination multiplier -- used 100 aliased queries to produce complexity 600 (> 500 limit)
- Depth test uses org->networks->org traversal chain to reach 17 levels (exceeding limit of 15), using correct schema field types (direct arrays, not Relay connections)
- Database tests use t.TempDir() for file-backed SQLite (not in-memory) to test actual pragma application via DSN parameters

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Fixed GraphQL query syntax for complexity and depth tests**
- **Found during:** Task 1 (GraphQL handler tests)
- **Issue:** Plan suggested using `first` args on nested entity fields (e.g., `networks(first:100)`) and Relay `edges/node` syntax on non-connection fields. Organization.networks returns `[Network!]` directly, not a connection type.
- **Fix:** Used correct schema: direct array access for nested fields, 100 aliased top-level queries for complexity, org->networks->org traversal for depth
- **Files modified:** internal/graphql/handler_test.go
- **Verification:** All 4 tests pass with -race
- **Committed in:** 3a85e1d (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Query syntax correction necessary for tests to validate actual handler behavior. No scope creep.

## Issues Encountered
None beyond the schema syntax deviation above.

## Known Stubs
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- GraphQL and database packages now have integration test coverage
- Ready for remaining phase 49 plans (detail.go split, upsert deduplication, about terminal rendering, seed consolidation)

## Self-Check: PASSED

- internal/graphql/handler_test.go: FOUND
- internal/database/database_test.go: FOUND
- 49-03-SUMMARY.md: FOUND
- Commit 3a85e1d: FOUND
- Commit ace7a4f: FOUND

---
*Phase: 49-refactoring-tech-debt*
*Completed: 2026-04-02*
