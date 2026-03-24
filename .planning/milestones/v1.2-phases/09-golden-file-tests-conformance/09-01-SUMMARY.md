---
phase: 09-golden-file-tests-conformance
plan: 01
subsystem: testing
tags: [golden-files, go-cmp, pdbcompat, deterministic-tests, httptest]

# Dependency graph
requires:
  - phase: 06-peeringdb-compat-api
    provides: PeeringDB compat handler, serializers, depth expansion, response envelope
provides:
  - Golden file test infrastructure with -update flag for all 13 PeeringDB types
  - 39 committed golden files (list, detail, depth) locking down JSON output
  - Deterministic test data setup with explicit IDs and fixed timestamps
affects: [09-02, pdbcompat, serializer-changes]

# Tech tracking
tech-stack:
  added: [go-cmp v0.7.0 (direct)]
  patterns: [golden-file-testing, flag-based-update, deterministic-test-data]

key-files:
  created:
    - internal/pdbcompat/golden_test.go
    - internal/pdbcompat/testdata/golden/{type}/{scenario}.json (39 files)
  modified:
    - go.mod

key-decisions:
  - "Promoted go-cmp v0.7.0 from indirect to direct dependency for human-readable golden file diffs"
  - "Explicit entity IDs (100-1300) and fixed timestamps for fully deterministic golden output"
  - "Compact JSON format matching actual API response output (not pretty-printed)"

patterns-established:
  - "Golden file pattern: flag.Bool update, compareOrUpdate helper, testdata/golden/{type}/{scenario}.json"
  - "Deterministic test data: setupGoldenTestData with SetID() and fixed goldenTime"

requirements-completed: [GOLD-01, GOLD-02, GOLD-03, GOLD-04]

# Metrics
duration: 4min
completed: 2026-03-23
---

# Phase 9 Plan 1: Golden File Infrastructure Summary

**Golden file test infrastructure with 39 committed reference files locking down PeeringDB compat JSON output for all 13 types across list, detail, and depth-expanded scenarios**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-23T23:32:16Z
- **Completed:** 2026-03-23T23:36:30Z
- **Tasks:** 2
- **Files modified:** 41

## Accomplishments
- Golden file test infrastructure with `-update` flag, `compareOrUpdate` helper, and `cmp.Diff` for readable diffs
- Deterministic test data setup creating all 13 entity types with explicit IDs (100-1300) and fixed timestamps
- 39 golden files generated and committed covering list, detail, and depth-expanded scenarios for all 13 PeeringDB types
- Tests verified deterministic across 5 consecutive runs, passing with -race and clean lint

## Task Commits

Each task was committed atomically:

1. **Task 1: Create golden file test infrastructure and deterministic test data** - `7fd3957b` (feat)
2. **Task 2: Generate golden files and verify round-trip consistency** - `ed78298` (feat)

## Files Created/Modified
- `internal/pdbcompat/golden_test.go` - Golden file test infrastructure with TestGoldenFiles, setupGoldenTestData, compareOrUpdate
- `internal/pdbcompat/testdata/golden/org/{list,detail,depth}.json` - Organization golden files
- `internal/pdbcompat/testdata/golden/net/{list,detail,depth}.json` - Network golden files with poc_set, netfac_set, netixlan_set in depth
- `internal/pdbcompat/testdata/golden/fac/{list,detail,depth}.json` - Facility golden files
- `internal/pdbcompat/testdata/golden/ix/{list,detail,depth}.json` - Internet exchange golden files
- `internal/pdbcompat/testdata/golden/poc/{list,detail,depth}.json` - POC golden files with expanded net in depth
- `internal/pdbcompat/testdata/golden/ixlan/{list,detail,depth}.json` - IXLan golden files
- `internal/pdbcompat/testdata/golden/ixpfx/{list,detail,depth}.json` - IX prefix golden files
- `internal/pdbcompat/testdata/golden/netixlan/{list,detail,depth}.json` - Network IX LAN golden files
- `internal/pdbcompat/testdata/golden/netfac/{list,detail,depth}.json` - Network facility golden files
- `internal/pdbcompat/testdata/golden/ixfac/{list,detail,depth}.json` - IX facility golden files
- `internal/pdbcompat/testdata/golden/carrier/{list,detail,depth}.json` - Carrier golden files
- `internal/pdbcompat/testdata/golden/carrierfac/{list,detail,depth}.json` - Carrier facility golden files
- `internal/pdbcompat/testdata/golden/campus/{list,detail,depth}.json` - Campus golden files
- `go.mod` - go-cmp v0.7.0 promoted to direct dependency

## Decisions Made
- Promoted go-cmp v0.7.0 from indirect to direct dependency for human-readable golden file diffs
- Used explicit entity IDs (100-1300 in increments of 100) for deterministic output
- Fixed timestamp of 2025-01-01T00:00:00Z for all entities
- Compact JSON format (not pretty-printed) matching actual API response output

## Deviations from Plan

None - plan executed exactly as written.

## Known Stubs

None - all golden files contain real serialized data from the compat layer.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Golden file infrastructure ready for conformance CLI tool (plan 09-02)
- Any future changes to serializers, depth expansion, or response formatting will be caught by golden file test failures
- Run `go test ./internal/pdbcompat/... -run TestGolden -update` to regenerate after intentional changes

## Self-Check: PASSED

- golden_test.go: FOUND
- org/list.json: FOUND
- net/depth.json: FOUND
- campus/depth.json: FOUND
- Commit 7fd3957b: FOUND
- Commit ed78298: FOUND
- Golden files: 39/39

---
*Phase: 09-golden-file-tests-conformance*
*Completed: 2026-03-23*
