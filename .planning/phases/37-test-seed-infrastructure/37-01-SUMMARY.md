---
phase: 37-test-seed-infrastructure
plan: 01
subsystem: testing
tags: [seed, testutil, ent, sqlite, tdd]

requires:
  - phase: none
    provides: standalone package, no phase dependencies
provides:
  - "seed.Full() creates all 13 PeeringDB types with deterministic IDs"
  - "seed.Minimal() creates 4 core types for basic relationship tests"
  - "seed.Networks() creates n networks with unique ASNs for scalability tests"
  - "Result struct with typed references to all created entities"
affects: [38-graph-test-coverage, 39-grpcserver-test-coverage, 40-web-test-coverage, 41-pdbcompat-test-coverage, 42-sync-test-coverage]

tech-stack:
  added: []
  patterns: ["deterministic test seeding with fixed IDs", "dual FK int + edge object setter pattern", "topological creation order for FK constraints"]

key-files:
  created:
    - internal/testutil/seed/seed.go
    - internal/testutil/seed/seed_test.go
  modified: []

key-decisions:
  - "Fixed IDs matching legacy seedAllTestData for backward compatibility"
  - "testing.TB parameter instead of *testing.T for benchmark reuse"

patterns-established:
  - "Seed pattern: always use both SetFKID(id) and SetFKEdge(entity) on every entity creation"
  - "Seed IDs: Org=1, Net=10, IX=20, Fac=30, Campus=40, Carrier=50, IxLan=100, NetIxLan=200, NetFac=300, IxFac=400, Poc=500, CarrierFac=600, IxPrefix=700"

requirements-completed: [INFRA-01]

duration: 4min
completed: 2026-03-26
---

# Phase 37 Plan 01: Test Seed Infrastructure Summary

**Deterministic test seed package (Full/Minimal/Networks) creating all 13 PeeringDB entity types with fixed IDs and validated FK relationships**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-26T10:37:52Z
- **Completed:** 2026-03-26T10:41:28Z
- **Tasks:** 2
- **Files created:** 2

## Accomplishments
- Created reusable seed package eliminating ~840 lines of duplicated entity creation across 4 packages
- Full() seeds all 13 PeeringDB types plus Network2 and Facility2 with deterministic IDs
- Minimal() seeds 4 core types (Org, Network, IX, Facility) for lightweight tests
- Networks() seeds Org + n Networks with unique ASNs (65001+i) for scalability testing
- All tests pass with -race, all 3 consumer packages (graph, grpcserver, web) build without import cycles

## Task Commits

Each task was committed atomically:

1. **Task 1 RED: Failing tests for seed package** - `d327687` (test)
2. **Task 1 GREEN: Implement seed package** - `15faf05` (feat)
3. **Task 2: Import-cycle validation** - verification only, no code changes

## Files Created/Modified
- `internal/testutil/seed/seed.go` - Full, Minimal, Networks seed functions + Result struct (316 lines)
- `internal/testutil/seed/seed_test.go` - 6 test functions covering all seed functions, entity counts, and FK relationships (359 lines)

## Decisions Made
- Used testing.TB (not *testing.T) so seed functions work in benchmarks too
- Preserved exact IDs from legacy seedAllTestData in internal/web/detail_test.go for backward compatibility
- Networks() uses private-use ASN range starting at 65001 to avoid collisions with real ASNs in Full()

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Known Stubs

None - all functions fully implemented with realistic field values.

## Next Phase Readiness
- Seed package ready for import by graph, grpcserver, web, pdbcompat, and sync test files
- Future phases (38-42) can replace bespoke entity creation with seed.Full(t, client) or seed.Minimal(t, client)

---
*Phase: 37-test-seed-infrastructure*
*Completed: 2026-03-26*
