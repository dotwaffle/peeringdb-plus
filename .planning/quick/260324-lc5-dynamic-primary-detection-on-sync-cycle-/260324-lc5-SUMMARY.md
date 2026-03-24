---
phase: quick-260324-lc5
plan: 01
subsystem: sync
tags: [litefs, primary-detection, scheduler, otel, context-cancellation]

# Dependency graph
requires: []
provides:
  - Dynamic per-tick primary detection in sync scheduler
  - Demotion monitor with context cancellation during active sync
  - RoleTransitions OTel counter metric (pdbplus.role.transitions)
  - Live isPrimaryFn in POST /sync handler
affects: [sync, observability, deployment]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Inject func() bool for runtime role detection instead of static bool"
    - "Per-cycle context with demotion monitor goroutine (1s poll interval)"
    - "primarySwitch test helper with atomic.Bool for controllable IsPrimary"

key-files:
  created: []
  modified:
    - internal/sync/worker.go
    - internal/sync/worker_test.go
    - internal/otel/metrics.go
    - cmd/peeringdb-plus/main.go

key-decisions:
  - "IsPrimary changed from bool to func() bool in WorkerConfig (removes dead field DEBT-01)"
  - "Demotion monitor polls every 1s (matches LiteFS failover timing)"
  - "Nil IsPrimary defaults to always-primary for backward compatibility"

patterns-established:
  - "Dynamic role detection via injected func() bool: testable, decoupled from litefs package"
  - "Demotion monitor goroutine tied to cycleCtx per CC-2 with done channel for clean exit"

requirements-completed: [DYN-01, DYN-02, DYN-03, DYN-04]

# Metrics
duration: 4min
completed: 2026-03-24
---

# Quick Task 260324-lc5: Dynamic Primary Detection on Sync Cycle Summary

**Dynamic per-tick primary detection in sync scheduler with demotion abort, role transition metrics, and live POST /sync handler**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-24T15:36:20Z
- **Completed:** 2026-03-24T15:40:45Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- Scheduler now runs on ALL instances, gates sync on live IsPrimary() per tick
- Demotion mid-sync cancels the cycle context via monitor goroutine (1s poll)
- Promotion detection checks last sync time and syncs immediately if overdue
- POST /sync handler detects primary status live per request (no restart needed)
- RoleTransitions OTel counter emits direction=promoted/demoted attributes
- All existing tests pass unchanged (nil IsPrimary defaults to always-true)

## Task Commits

Each task was committed atomically:

1. **Task 1 (RED): Add failing tests for dynamic primary detection** - `6f18daf` (test)
2. **Task 1 (GREEN): Dynamic primary detection in sync scheduler** - `46180f0` (feat)
3. **Task 2: Wire dynamic primary detection in main.go** - `11f46d3` (feat)

## Files Created/Modified
- `internal/sync/worker.go` - WorkerConfig.IsPrimary changed to func() bool, added runSyncCycle with demotion monitor, rewrote StartScheduler for all-instance execution
- `internal/sync/worker_test.go` - Added primarySwitch helper, TestStartScheduler_SkipsOnReplica (DYN-01), TestStartScheduler_PromotionSync (DYN-02), TestRunSyncCycle_DemotionAbort (DYN-03)
- `internal/otel/metrics.go` - Added RoleTransitions Int64Counter (pdbplus.role.transitions)
- `cmd/peeringdb-plus/main.go` - Created isPrimaryFn closure, passed to WorkerConfig, unconditional scheduler start, live isPrimaryFn() in POST /sync

## Decisions Made
- IsPrimary changed from bool to func() bool in WorkerConfig, satisfying both the dynamic detection requirement and removing dead field DEBT-01
- Demotion monitor polls at 1s intervals (matching LiteFS failover timing: TTL 10s + lock-delay 1s)
- nil IsPrimary in WorkerConfig defaults to always-primary for backward compatibility
- Static isPrimary bool retained in main.go only for one-time schema migration and InitStatusTable gates

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None.

## Known Stubs
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Dynamic primary detection is complete and tested
- Ready for v1.5 observability phases (role transition metric available for dashboards)
- No blockers

## Self-Check: PASSED

All 4 files verified present. All 3 commits verified in git log.

---
*Quick Task: 260324-lc5*
*Completed: 2026-03-24*
