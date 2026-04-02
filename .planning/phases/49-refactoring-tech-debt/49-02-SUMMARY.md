---
phase: 49-refactoring-tech-debt
plan: 02
subsystem: sync
tags: [generics, refactoring, batch-upsert, deduplication]

requires:
  - phase: none
    provides: n/a
provides:
  - Generic upsertBatch[Item, Builder] function for batch upsert deduplication
  - 13 per-type upsert functions refactored to use shared generic
affects: [sync, worker]

tech-stack:
  added: []
  patterns: [generic batch function with closure-based type specialization]

key-files:
  created: []
  modified: [internal/sync/upsert.go]

key-decisions:
  - "Closure-based generics over interface-based approach: buildFn/saveFn closures keep per-type logic inline without requiring new types"
  - "Preserved all 13 function signatures so worker.go call sites require zero changes"

patterns-established:
  - "upsertBatch[Item, Builder] pattern: generic batch loop with idFn/buildFn/saveFn closures for type-safe bulk operations"

requirements-completed: [REFAC-02]

duration: 3min
completed: 2026-04-02
---

# Phase 49 Plan 02: Upsert Deduplication Summary

**Generic upsertBatch function extracts batch-loop boilerplate from 13 copy-pasted upsert functions, reducing upsert.go from 613 to 541 lines**

## Performance

- **Duration:** 3 min
- **Started:** 2026-04-02T05:04:20Z
- **Completed:** 2026-04-02T05:07:01Z
- **Tasks:** 2
- **Files modified:** 1

## Accomplishments
- Extracted generic `upsertBatch[Item, Builder]` function encapsulating the batch loop, ID collection, and error wrapping
- Refactored all 13 per-type upsert functions to call `upsertBatch` with type-specific closures
- Preserved all function signatures -- worker.go unchanged, all 50 sync tests and benchmarks pass

## Task Commits

Each task was committed atomically:

1. **Task 1: Create generic upsertBatch and refactor per-type functions** - `d5e3f59` (refactor)
2. **Task 2: Verify all sync tests and benchmarks pass** - verification only, no code changes

## Files Created/Modified
- `internal/sync/upsert.go` - Added generic upsertBatch function, refactored 13 per-type upsert functions to use it

## Decisions Made
- Used closure-based generics (buildFn/saveFn closures) rather than interface-based approach from CONTEXT.md -- closures keep per-type field mappings inline and avoid introducing new interface types
- Preserved all 13 existing function signatures so worker.go call sites needed zero modifications

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Upsert deduplication complete
- All sync tests passing, ready for further refactoring work in this phase

---
*Phase: 49-refactoring-tech-debt*
*Completed: 2026-04-02*

## Self-Check: PASSED

- FOUND: internal/sync/upsert.go
- FOUND: .planning/phases/49-refactoring-tech-debt/49-02-SUMMARY.md
- FOUND: commit d5e3f59
