---
phase: 07-lint-code-quality
plan: 02
subsystem: tooling
tags: [golangci-lint, linting, code-quality, errcheck, revive, staticcheck]

# Dependency graph
requires:
  - phase: 07-01
    provides: "golangci-lint v2 configuration with generated:strict exclusion and ~40 violation baseline"
provides:
  - Zero lint violations across all hand-written code
  - Clean go vet and go test -race across entire codebase
  - Renamed sync.SyncStatus to sync.Status to eliminate type stutter
affects: []

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Explicit error discards with _ = for best-effort writes in HTTP handlers"
    - "fmt.Fprintf(&b, ...) instead of b.WriteString(fmt.Sprintf(...)) for builder patterns"

key-files:
  created: []
  modified:
    - cmd/pdb-schema-extract/main.go
    - cmd/pdb-schema-generate/main.go
    - cmd/peeringdb-plus/main.go
    - graph/custom.resolvers.go
    - graph/resolver_test.go
    - internal/graphql/handler.go
    - internal/health/handler.go
    - internal/otel/logger_test.go
    - internal/otel/metrics_test.go
    - internal/otel/provider.go
    - internal/otel/provider_test.go
    - internal/pdbcompat/depth_test.go
    - internal/pdbcompat/handler.go
    - internal/pdbcompat/handler_test.go
    - internal/pdbcompat/serializer.go
    - internal/peeringdb/client.go
    - internal/peeringdb/client_test.go
    - internal/sync/integration_test.go
    - internal/sync/status.go
    - internal/sync/worker.go
    - internal/sync/worker_test.go
    - schema/generate.go

key-decisions:
  - "Renamed sync.SyncStatus to sync.Status to resolve revive type stutter (sync.SyncStatus reads as sync.Sync...)"
  - "Used nolint:gocritic for exitAfterDefer in main.go rather than extracting run() function -- trivial cancel() defer at early init"
  - "Used nolint:staticcheck for deprecated gqlgen NewDefaultServer -- no replacement API available"

patterns-established:
  - "Unused HTTP handler params renamed to _ (not removed) to match http.HandlerFunc signature"

requirements-completed: [LINT-02, LINT-03]

# Metrics
duration: 16min
completed: 2026-03-23
---

# Phase 7 Plan 02: Fix Lint Violations Summary

**All 40 golangci-lint violations fixed across 22 hand-written files: 21 errcheck, 8 revive, 6 staticcheck, 3 unused, 1 gocritic, 1 nolintlint**

## Performance

- **Duration:** 16 min
- **Started:** 2026-03-23T21:50:45Z
- **Completed:** 2026-03-23T22:07:32Z
- **Tasks:** 2
- **Files modified:** 22

## Accomplishments
- Fixed all 40 lint violations identified in Plan 01's baseline, zero violations remaining
- Renamed sync.SyncStatus to sync.Status to eliminate package stutter, updated all 8 consuming files
- Removed 3 unused functions (derefString, removeFixtureData, newIntegrationWorker)
- All tests pass with -race detector, go vet clean, go build -trimpath clean
- No generated code modified (ent/, graph/generated.go, graph/model/models.go all untouched)

## Task Commits

Each task was committed atomically:

1. **Task 1: Fix all golangci-lint violations in hand-written code** - `0b4ff06` (fix)
2. **Task 2: Final verification of clean lint and vet** - verification only, no commit needed

## Files Created/Modified
- `cmd/pdb-schema-extract/main.go` - Added explicit error discard on fmt.Sscanf
- `cmd/pdb-schema-generate/main.go` - Replaced WriteString(Sprintf) with Fprintf, renamed unused apiPath param
- `cmd/peeringdb-plus/main.go` - Discarded memlimit return, removed var type annotation, nolint exitAfterDefer, renamed GetLastSyncStatus to GetLastStatus
- `graph/custom.resolvers.go` - Renamed unused ctx param, updated GetLastStatus call
- `graph/resolver_test.go` - Updated pdbsync.Status reference
- `internal/graphql/handler.go` - Added nolint:staticcheck for deprecated gqlgen API
- `internal/health/handler.go` - Added explicit error discards on json.Encode, renamed unused r param, updated sync.Status references
- `internal/otel/logger_test.go` - Wrapped defer Shutdown in closure with error discard
- `internal/otel/metrics_test.go` - Added error discards on cleanup Shutdown calls
- `internal/otel/provider.go` - Renamed unused ctx param in buildResource
- `internal/otel/provider_test.go` - Added error discards on cleanup Shutdown calls
- `internal/pdbcompat/depth_test.go` - Added error discards on json.Unmarshal calls
- `internal/pdbcompat/handler.go` - Removed stale nolint:errcheck directive
- `internal/pdbcompat/handler_test.go` - Added error discards on json.Unmarshal calls
- `internal/pdbcompat/serializer.go` - Removed unused derefString function
- `internal/peeringdb/client.go` - Added error discard on io.Copy drain
- `internal/peeringdb/client_test.go` - Added error discards on w.Write, renamed unused r params, used tagged switch
- `internal/sync/integration_test.go` - Removed unused removeFixtureData and newIntegrationWorker, added w.Write error discards, updated GetLastStatus call
- `internal/sync/status.go` - Renamed SyncStatus to Status, GetLastSyncStatus to GetLastStatus
- `internal/sync/worker.go` - Updated Status reference
- `internal/sync/worker_test.go` - Added error discards on json.Encode and Shutdown, updated GetLastStatus calls
- `schema/generate.go` - Reworded comment to avoid ineffectual go:generate directive

## Decisions Made
- Renamed sync.SyncStatus to sync.Status per revive stutter rule -- propagated across 8 files including health handler, resolvers, tests, and main.go
- Used nolint:gocritic for exitAfterDefer rather than extracting a run() function -- the deferred cancel() is trivial at the point of os.Exit during early init
- Used nolint:staticcheck for deprecated gqlgen handler.NewDefaultServer -- this is the only gqlgen server factory and has no replacement
- Fixed all w.Write and json.Encode unchecked returns with `_, _ =` or `_ =` rather than adding error handling, because these are best-effort writes to HTTP response bodies where the client may have disconnected

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Additional violations surfaced iteratively**
- **Found during:** Task 1 (lint fix iteration)
- **Issue:** Initial golangci-lint run showed 40 violations, but fixing some revealed additional violations in the same files (unchecked w.Write, unused r params) that were masked by earlier errors
- **Fix:** Fixed all violations iteratively until golangci-lint reported 0 issues
- **Files modified:** internal/peeringdb/client_test.go, internal/sync/integration_test.go, internal/pdbcompat/handler_test.go
- **Verification:** golangci-lint run ./... exits 0
- **Committed in:** 0b4ff06

---

**Total deviations:** 1 auto-fixed (additional violations surfaced during iteration)
**Impact on plan:** Minor scope expansion to fix all violations, not just the initial 40. No architectural changes.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Known Stubs

None - no stubs or placeholder data in modified files.

## Next Phase Readiness
- Phase 7 complete: golangci-lint, go vet, go test -race, and go build -trimpath all pass clean
- LINT-01 (config), LINT-02 (violations fixed), LINT-03 (go vet clean) all satisfied
- Codebase ready for CI pipeline enforcement in Phase 9

---
*Phase: 07-lint-code-quality*
*Completed: 2026-03-23*
