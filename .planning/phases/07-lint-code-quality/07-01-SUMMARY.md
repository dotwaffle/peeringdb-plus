---
phase: 07-lint-code-quality
plan: 01
subsystem: tooling
tags: [golangci-lint, linting, dead-code, code-quality]

# Dependency graph
requires: []
provides:
  - golangci-lint v2 configuration with generated:strict exclusion
  - Clean codebase with dead code removed (globalid.go, dataloader, config.IsPrimary)
  - Baseline lint violation count for Plan 02
affects: [07-02-PLAN]

# Tech tracking
tech-stack:
  added: []
  removed: [github.com/vikstrous/dataloadgen]
  patterns:
    - "golangci-lint v2 config with generated:strict for ent/gqlgen exclusion"

key-files:
  created:
    - .golangci.yml
  modified:
    - cmd/peeringdb-plus/main.go
    - graph/resolver_test.go
    - internal/config/config.go
    - internal/sync/worker.go
    - go.mod
    - go.sum
  deleted:
    - graph/globalid.go
    - graph/dataloader/loader.go

key-decisions:
  - "generated:strict uses standard header detection only, no path-based exclusions for ent/gqlgen"
  - "Standard defaults (govet, errcheck, staticcheck, unused, gosimple, ineffassign, typecheck) plus gocritic/misspell/nolintlint/revive"

patterns-established:
  - "golangci-lint v2 config at repo root with generated:strict header detection"

requirements-completed: [LINT-01, LINT-03]

# Metrics
duration: 3min
completed: 2026-03-23
---

# Phase 7 Plan 01: Lint Config & Dead Code Summary

**golangci-lint v2 configured with generated:strict exclusion, dead code removed (globalid.go, dataloader package, config.IsPrimary), ~40 violations baselined for Plan 02**

## Performance

- **Duration:** 3 min
- **Started:** 2026-03-23T21:44:14Z
- **Completed:** 2026-03-23T21:47:53Z
- **Tasks:** 2
- **Files modified:** 8 (6 modified, 2 deleted)

## Accomplishments
- Removed all dead code: graph/globalid.go, graph/dataloader/ package, config.IsPrimary field, WorkerConfig.IsPrimary field
- Removed dataloadgen dependency from go.mod
- Created golangci-lint v2 configuration with standard defaults plus gocritic/misspell/nolintlint/revive
- Established baseline: ~40 lint violations (21 errcheck, 8 revive, 6 staticcheck, 3 unused, 1 gocritic, 1 nolintlint)
- Verified go build, go test, and go vet all pass clean

## Task Commits

Each task was committed atomically:

1. **Task 1: Remove dead code (globalid.go, dataloader, config.IsPrimary)** - `ec182e1` (refactor)
2. **Task 2: Create golangci-lint v2 configuration** - `e1d0a69` (chore)

## Files Created/Modified
- `.golangci.yml` - golangci-lint v2 configuration with standard defaults + gocritic/misspell/nolintlint/revive
- `cmd/peeringdb-plus/main.go` - Removed dataloader import/wiring and IsPrimary from WorkerConfig literal
- `graph/resolver_test.go` - Removed dataloader import/wiring, use gqlHandler directly
- `internal/config/config.go` - Removed IsPrimary field and PDBPLUS_IS_PRIMARY parsing
- `internal/sync/worker.go` - Removed IsPrimary from WorkerConfig struct
- `go.mod` / `go.sum` - Removed dataloadgen dependency
- `graph/globalid.go` - Deleted (unused MarshalGlobalID/UnmarshalGlobalID)
- `graph/dataloader/loader.go` - Deleted (unused DataLoader middleware)

## Decisions Made
- Used `generated: strict` header detection only (no path-based exclusions) -- trusts standard `// Code generated ... DO NOT EDIT.` headers in all ent and gqlgen files
- Excluded gosec from test files (test code commonly uses hardcoded values that trigger false positives)
- No gofumpt or line length limits per user decisions in 07-CONTEXT.md

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Lint Baseline for Plan 02

Current violation breakdown by linter:
- **errcheck:** 21 violations (unchecked error returns)
- **revive:** 8 violations (unused params, var-declaration, type stutter)
- **staticcheck:** 6 violations (QF1012 fmt.Fprintf suggestions)
- **unused:** 3 violations
- **gocritic:** 1 violation (exitAfterDefer)
- **nolintlint:** 1 violation (unused nolint directive)

Total: ~40 violations in hand-written code (generated code excluded by generated:strict).

## Next Phase Readiness
- golangci-lint v2 config ready for Plan 02 to fix all violations
- Baseline count (~40) is manageable in a single plan
- go vet already passes clean (LINT-03 satisfied)

---
*Phase: 07-lint-code-quality*
*Completed: 2026-03-23*
