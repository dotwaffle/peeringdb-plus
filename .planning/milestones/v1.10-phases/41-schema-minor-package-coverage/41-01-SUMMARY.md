---
phase: 41-schema-minor-package-coverage
plan: 01
subsystem: testing
tags: [ent, schema, otel, hooks, coverage, fk-constraints]

# Dependency graph
requires: []
provides:
  - "ent/schema 100% coverage on hand-written code (up from 47.4%)"
  - "otelMutationHook error path test proving span.RecordError works"
  - "FK constraint violation tests validating foreign_keys pragma enforcement"
affects: []

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "In-memory OTel span exporter for hook error path verification"
    - "Table-driven FK constraint tests with non-existent references"
    - "Direct-call schema configuration method tests (Edges/Indexes/Annotations)"

key-files:
  created: []
  modified:
    - "ent/schema/schema_test.go"

key-decisions:
  - "Span name uses OpCreate (not Create) matching ent Op.String() output"

patterns-established:
  - "OTel hook testing: duplicate primary key triggers mutation error, in-memory exporter captures span events"

requirements-completed: [SCHEMA-01, SCHEMA-02, SCHEMA-03]

# Metrics
duration: 4min
completed: 2026-03-26
---

# Phase 41 Plan 01: Schema Coverage Summary

**otelMutationHook error path, FK constraint violations, and all 39 Edges/Indexes/Annotations methods tested -- coverage 47.4% to 100.0%**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-26T12:40:45Z
- **Completed:** 2026-03-26T12:45:05Z
- **Tasks:** 2
- **Files modified:** 1

## Accomplishments
- otelMutationHook error branch (hooks.go:28) covered by TestOtelMutationHook_ErrorPath -- verifies span.RecordError via in-memory OTel exporter
- FK constraint violations tested for Network, IxLan, and Poc with non-existent FK references (foreign_keys pragma enabled)
- All 13 schema types have Edges(), Indexes(), and Annotations() exercised in dedicated table-driven tests
- ent/schema hand-written code coverage raised from 47.4% to 100.0% (target was 65%)

## Task Commits

Each task was committed atomically:

1. **Task 1: Schema hook error path and FK constraint tests** - `25c4c5c` (test)
2. **Task 2: Schema Edges/Indexes/Annotations coverage for all 13 types** - `ce8282d` (test)

## Files Created/Modified
- `ent/schema/schema_test.go` - Added TestOtelMutationHook_ErrorPath, TestFKConstraintViolation, TestSchemaEdges, TestSchemaIndexes, TestSchemaAnnotations

## Decisions Made
- Span name is "ent.Organization.OpCreate" (not "ent.Organization.Create") because ent's Op.String() returns "OpCreate" -- discovered during test execution, fixed inline

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed span name assertion**
- **Found during:** Task 1 (TestOtelMutationHook_ErrorPath)
- **Issue:** Plan assumed span name "ent.Organization.Create" but ent Op.String() returns "OpCreate"
- **Fix:** Updated assertion to match "ent.Organization.OpCreate"
- **Files modified:** ent/schema/schema_test.go
- **Verification:** Test passes, span with RecordError event found
- **Committed in:** 25c4c5c (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** Minor span name correction. No scope creep.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- ent/schema at 100% coverage, all requirements (SCHEMA-01, SCHEMA-02, SCHEMA-03) satisfied
- Ready for 41-02 plan targeting internal/otel, internal/health, internal/peeringdb coverage

## Self-Check: PASSED

- [x] ent/schema/schema_test.go exists
- [x] Commit 25c4c5c exists (Task 1)
- [x] Commit ce8282d exists (Task 2)

---
*Phase: 41-schema-minor-package-coverage*
*Completed: 2026-03-26*
