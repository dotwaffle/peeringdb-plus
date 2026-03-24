# Quick Task 260324-lc5: Dynamic primary detection on sync cycle start - Context

**Gathered:** 2026-03-24
**Status:** Ready for planning

<domain>
## Task Boundary

On the start of every sync cycle, the sync process should check whether it is currently the primary or not. This will allow it to change modes without needing a restart when another instance is promoted.

Currently, `isPrimary` is checked once at startup in `main.go:90` via `litefs.IsPrimaryWithFallback()`. The scheduler is conditionally started (`if isPrimary { go syncWorker.StartScheduler(...) }`), and the POST /sync handler captures the static `isPrimary` in a closure. Neither adapts to runtime primary changes.

</domain>

<decisions>
## Implementation Decisions

### Check Location
- Check `litefs.IsPrimary()` at the top of each tick in `StartScheduler` loop — skip sync if not primary, run if primary.

### Promotion Behavior (Replica → Primary)
- On first detection as primary, check last sync time. Sync immediately only if overdue (last sync + interval < now). Otherwise wait for next tick.
- The scheduler must always be running on ALL instances (not just primary), so it can detect promotion and act.

### Demotion Behavior (Primary → Replica)
- Abort immediately. Cancel the sync context to stop ASAP when no longer primary.
- This means `SyncWithRetry` / `RunOnce` should check primary status and bail out if demoted mid-sync.

### POST /sync Handler
- Replace the static `isPrimary` closure capture with a live `litefs.IsPrimary()` call on each request.
- This makes POST /sync immediately aware of role changes without restart.

### Claude's Discretion
- How to propagate the primary check function into the worker (dependency injection vs. direct call)
- Whether to add an OTel metric for primary/replica role transitions
- Logging level and frequency for role change detection

</decisions>

<specifics>
## Specific Ideas

- The scheduler currently only starts on primary (`main.go:138`). It needs to start on ALL instances now, with the primary check inside the loop.
- `litefs.IsPrimaryWithFallback()` already exists and handles LiteFS + env var fallback — reuse it.
- The abort-on-demotion requires a cancellable context per sync cycle, checked against primary status.
- `WorkerConfig.IsPrimary` dead field can be removed as part of this change (it's already planned for Phase 18).

</specifics>

<canonical_refs>
## Canonical References

- `internal/litefs/primary.go` — `IsPrimary()`, `IsPrimaryWithFallback()` functions
- `internal/sync/worker.go:394` — `StartScheduler()` loop
- `cmd/peeringdb-plus/main.go:90` — static `isPrimary` detection at startup
- `cmd/peeringdb-plus/main.go:138` — conditional scheduler start
- `cmd/peeringdb-plus/main.go:154` — static `isPrimary` in POST /sync handler

</canonical_refs>
