---
phase: 03-production-readiness
plan: 02
subsystem: infra
tags: [health-check, readiness, liveness, litefs, primary-detection, sqlite]

# Dependency graph
requires:
  - phase: 01-data-foundation
    provides: "sync_status table and GetLastSyncStatus function"
provides:
  - "HTTP /healthz liveness endpoint (always 200)"
  - "HTTP /readyz readiness endpoint (db + sync freshness checks)"
  - "LiteFS primary/replica detection via .primary file"
  - "IsPrimaryWithFallback for local dev without LiteFS"
affects: [03-production-readiness, deployment, fly-io]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Inverted .primary file semantics: file exists = replica, absent = primary"
    - "ReadinessInput struct for dependency injection into handler (CS-5)"
    - "Component-level health response JSON with status + message"
    - "2-second timeout context for db ping in readiness check"

key-files:
  created:
    - "internal/litefs/primary.go"
    - "internal/litefs/primary_test.go"
    - "internal/health/handler.go"
    - "internal/health/handler_test.go"
  modified: []

key-decisions:
  - "Separate /healthz (liveness) and /readyz (readiness) endpoints rather than combined /health"
  - "getLastCompletedSync queries for non-running rows when latest sync is still running"
  - "IsPrimaryWithFallback checks parent directory existence to detect LiteFS mount"
  - "Failed sync status always sets not_ready regardless of age (degraded component)"

patterns-established:
  - "Health handler uses ReadinessInput struct for dependency injection per CS-5"
  - "LiteFS detection uses filepath.Dir to check mount directory existence"
  - "Tests use in-memory SQLite with sync.InitStatusTable for isolated DB state"

requirements-completed: [OPS-04, OPS-05, STOR-02]

# Metrics
duration: 4min
completed: 2026-03-22
---

# Phase 3 Plan 2: Health Endpoints & LiteFS Primary Detection Summary

**Liveness/readiness HTTP handlers with sync freshness checking and LiteFS primary/replica detection via .primary file semantics**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-22T17:49:06Z
- **Completed:** 2026-03-22T17:53:27Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- LiteFS primary detection with inverted .primary file semantics (absent = primary, present = replica)
- IsPrimaryWithFallback for environments without LiteFS using env var fallback
- /healthz liveness endpoint returns 200 always with JSON status
- /readyz readiness endpoint checks database connectivity (2s timeout) and sync data freshness against configurable threshold
- Per-component status reporting in JSON response (db, sync) with RFC3339 timestamps and age duration
- Handles edge case of currently-running sync by checking previous completed sync result

## Task Commits

Each task was committed atomically:

1. **Task 1: Create LiteFS primary detection package** - `e8c942e` (test), `a0a0916` (feat)
2. **Task 2: Create health and readiness HTTP handlers** - `ad500e3` (test), `273ae58` (feat)

_Note: TDD tasks have separate test and feat commits (RED -> GREEN)_

## Files Created/Modified
- `internal/litefs/primary.go` - LiteFS primary/replica detection via .primary file and env var fallback
- `internal/litefs/primary_test.go` - 8 test cases covering file exists/absent, LiteFS dir, env var branches
- `internal/health/handler.go` - HTTP handlers for /healthz (liveness) and /readyz (readiness) endpoints
- `internal/health/handler_test.go` - 8 test scenarios covering healthy, stale, no-sync, db-down, running, failed states

## Decisions Made
- Separate /healthz and /readyz endpoints (not combined /health) for Kubernetes/Fly.io compatibility
- getLastCompletedSync as a private helper in the health package to handle "running" sync edge case rather than adding to the sync package
- IsPrimaryWithFallback checks filepath.Dir(path) existence to detect whether LiteFS mount is present
- Failed sync always results in not_ready status regardless of age -- a failed sync is always degraded

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
- t.Setenv incompatible with t.Parallel in Go 1.26 -- removed t.Parallel from IsPrimaryWithFallback tests that use t.Setenv (Go enforces this at runtime)

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Health endpoints ready for wiring into main HTTP server and Fly.io health check configuration
- LiteFS primary detection ready for use by sync worker to gate writes to primary node
- Plan 03 (Fly.io deployment) can reference these endpoints in fly.toml health checks

## Self-Check: PASSED

All 4 files verified present. All 4 commits verified in git log.

---
*Phase: 03-production-readiness*
*Completed: 2026-03-22*
