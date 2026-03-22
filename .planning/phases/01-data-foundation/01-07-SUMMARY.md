---
phase: 01-data-foundation
plan: 07
subsystem: testing
tags: [fixtures, integration-tests, sync, peeringdb, sqlite]

# Dependency graph
requires:
  - phase: 01-data-foundation/01-04
    provides: "Sync worker with upsert, delete, status tracking, and retry logic"
  - phase: 01-data-foundation/01-01
    provides: "testutil.SetupClient and SetupClientWithDB for in-memory ent clients"
  - phase: 01-data-foundation/01-03
    provides: "PeeringDB client with FetchType, Response[T], and all 13 Go struct types"
provides:
  - "13 fixture files with realistic PeeringDB API response shapes for all object types"
  - "Fixture-based integration tests for full sync pipeline end-to-end"
  - "Test coverage for stale record deletion, deleted-object filtering, edge traversal, idempotency"
affects: [02-api-surface, 01-data-foundation]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Fixture-based integration testing with httptest.Server serving JSON files"
    - "fixtureServer helper for loading/mutating fixture data at runtime"

key-files:
  created:
    - testdata/fixtures/org.json
    - testdata/fixtures/net.json
    - testdata/fixtures/fac.json
    - testdata/fixtures/ix.json
    - testdata/fixtures/poc.json
    - testdata/fixtures/ixlan.json
    - testdata/fixtures/ixpfx.json
    - testdata/fixtures/netixlan.json
    - testdata/fixtures/netfac.json
    - testdata/fixtures/ixfac.json
    - testdata/fixtures/carrier.json
    - testdata/fixtures/carrierfac.json
    - testdata/fixtures/campus.json
    - internal/sync/integration_test.go
  modified: []

key-decisions:
  - "Fixture data uses synthetic but realistic values with valid FK relationships across all 13 types"
  - "Integration test uses external _test package (sync_test) to test public API surface only"

patterns-established:
  - "Fixture loading: os.ReadFile from testdata/fixtures/ via relative path from test location"
  - "fixtureServer: httptest.Server wrapping fixture data with setFixtureData/removeFixtureData mutation methods"
  - "newIntegrationWorker: factory function wiring fixture server, in-memory ent client, and sync worker"

requirements-completed: [DATA-04]

# Metrics
duration: 6min
completed: 2026-03-22
---

# Phase 01 Plan 07: Test Fixtures and Integration Tests Summary

**13 PeeringDB fixture files and 4 integration tests verifying full sync pipeline: upsert, delete, status filtering, edge traversal, and idempotency**

## Performance

- **Duration:** 6 min
- **Started:** 2026-03-22T15:39:39Z
- **Completed:** 2026-03-22T15:45:44Z
- **Tasks:** 2
- **Files modified:** 14

## Accomplishments
- Created 13 JSON fixture files with realistic PeeringDB API response shapes including valid FK relationships
- Built 4 integration tests covering full sync pipeline end-to-end with fixture data
- Verified stale record deletion, status=deleted filtering, edge traversal, sync_status recording, and idempotency
- Org 3 with status=deleted enables testing of D-32 deleted-object filtering in both include/exclude modes

## Task Commits

Each task was committed atomically:

1. **Task 1: Create test fixtures for all 13 PeeringDB object types** - `32ee0e7` (test)
2. **Task 2: Create fixture-based integration tests for full sync pipeline** - `73a37cf` (test)

## Files Created/Modified
- `testdata/fixtures/org.json` - 3 organizations (incl. 1 status=deleted)
- `testdata/fixtures/net.json` - 2 networks with ASN 65001 and 65002
- `testdata/fixtures/fac.json` - 2 facilities with FK to org and campus
- `testdata/fixtures/ix.json` - 2 internet exchanges
- `testdata/fixtures/poc.json` - 1 point of contact for net 1
- `testdata/fixtures/ixlan.json` - 2 IX LANs with FK to ix
- `testdata/fixtures/ixpfx.json` - 2 IX prefixes (IPv4 and IPv6)
- `testdata/fixtures/netixlan.json` - 1 network-IXLan with ipaddr4 and ipaddr6
- `testdata/fixtures/netfac.json` - 1 network-facility association
- `testdata/fixtures/ixfac.json` - 1 IX-facility association
- `testdata/fixtures/carrier.json` - 1 carrier
- `testdata/fixtures/carrierfac.json` - 1 carrier-facility association
- `testdata/fixtures/campus.json` - 2 campuses with FK to org
- `internal/sync/integration_test.go` - 4 integration tests with fixture server helpers

## Decisions Made
- Used external test package (sync_test) to test only the public API surface of the sync package
- Fixture data uses synthetic but realistic values -- not real PeeringDB records
- TestSyncDeletesStaleRecords also removes dependent FK records to avoid constraint violations, which accurately reflects how PeeringDB API responses behave (child records are removed when parent is removed)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed FK constraint violation in TestSyncDeletesStaleRecords**
- **Found during:** Task 2 (integration test implementation)
- **Issue:** Removing org 2 from mock response while campus 2 (FK to org 2) still existed caused FOREIGN KEY constraint failure on re-sync
- **Fix:** Updated test to also remove all dependent records (campus, fac, ix, ixlan, ixpfx, net) when removing org 2, accurately reflecting how PeeringDB API responses behave in practice
- **Files modified:** internal/sync/integration_test.go
- **Verification:** All 4 integration tests pass with -race
- **Committed in:** 73a37cf (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** Test logic fix to properly handle FK cascading in mock data. No scope creep.

## Issues Encountered
None beyond the FK constraint fix documented above.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- All 13 PeeringDB object types have fixture data for CI testing
- Full sync pipeline is integration-tested end-to-end
- Ready for Plan 06 (main binary) and subsequent API surface development

## Self-Check: PASSED

All 14 files verified present. Both commit hashes (32ee0e7, 73a37cf) verified in git log.

---
*Phase: 01-data-foundation*
*Completed: 2026-03-22*
