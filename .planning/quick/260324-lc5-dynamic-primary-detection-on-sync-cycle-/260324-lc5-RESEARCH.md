# Quick Task 260324-lc5: Dynamic Primary Detection on Sync Cycle Start - Research

**Researched:** 2026-03-24
**Domain:** LiteFS primary/replica role detection, Go context cancellation, sync lifecycle
**Confidence:** HIGH

## Summary

This task converts the static startup-time primary detection (`isPrimary` bool captured once in `main.go:90`) into dynamic per-tick detection inside the sync scheduler. The existing `litefs.IsPrimaryWithFallback()` function is already suitable for repeated calls -- it performs a cheap `os.Stat()` on `/litefs/.primary` which is a FUSE-mounted file managed by LiteFS. The primary change is architectural: the scheduler must run on ALL instances (not just primary), check primary status each tick, and use per-cycle context cancellation to abort syncs on demotion.

LiteFS uses Consul-based lease management with default TTL of 10 seconds and 1 second lock-delay. The `.primary` file is managed by LiteFS's FUSE layer and appears/disappears as the Consul lease changes hands. On clean shutdown, failover is immediate (lease destroyed instantly). On crash, failover waits for TTL expiry (~10s default). The `.primary` file exists only on replicas connected to the primary -- its absence means the local node IS the primary.

**Primary recommendation:** Inject a `func() bool` primary-check function into the Worker, call it at the top of each scheduler tick to gate sync execution, and wrap each sync cycle in a derived context with a background goroutine that cancels on demotion.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- Check `litefs.IsPrimary()` at the top of each tick in `StartScheduler` loop -- skip sync if not primary, run if primary.
- On first detection as primary, check last sync time. Sync immediately only if overdue (last sync + interval < now). Otherwise wait for next tick.
- The scheduler must always be running on ALL instances (not just primary), so it can detect promotion and act.
- Abort immediately. Cancel the sync context to stop ASAP when no longer primary.
- `SyncWithRetry` / `RunOnce` should check primary status and bail out if demoted mid-sync.
- Replace the static `isPrimary` closure capture with a live `litefs.IsPrimary()` call on each request for POST /sync handler.

### Claude's Discretion
- How to propagate the primary check function into the worker (dependency injection vs. direct call)
- Whether to add an OTel metric for primary/replica role transitions
- Logging level and frequency for role change detection

### Deferred Ideas (OUT OF SCOPE)
- None specified
</user_constraints>

## Architecture Patterns

### Pattern 1: Injecting Primary Check Function into Worker

**What:** Add a `func() bool` field to `Worker` (or `WorkerConfig`) that the scheduler calls to check primary status. This follows the project's existing patterns (functional options, dependency injection via constructor).

**Why this over direct import:** The `litefs` package is already imported in `main.go`. Injecting a function keeps the `sync` package decoupled from `litefs` -- testable with a simple closure that returns a controlled value.

**Recommended approach:** Add an `IsPrimary func() bool` field to the `Worker` struct, set via a new field in `WorkerConfig` or via a setter method. The constructor in `main.go` would wire it as:

```go
// In WorkerConfig (replaces the dead IsPrimary bool field):
type WorkerConfig struct {
    IncludeDeleted bool
    SyncMode       config.SyncMode
    IsPrimary      func() bool // live primary detection; nil means always-primary
}
```

This repurposes the existing dead `IsPrimary bool` field (already flagged for removal in DEBT-01) by changing its type. The planner can remove the old bool and add the func in the same change, satisfying the tech debt item simultaneously.

**Nil safety:** If `IsPrimary` is nil, default to `func() bool { return true }` in `NewWorker()` for backward compatibility (local dev, tests).

### Pattern 2: Per-Cycle Context with Demotion Monitor

**What:** Each sync cycle gets its own derived context. A background goroutine polls primary status during the sync and cancels the context on demotion.

**Structure:**

```go
// Inside scheduler tick (when primary):
cycleCtx, cycleCancel := context.WithCancel(ctx)

// Monitor goroutine for demotion.
done := make(chan struct{})
go func() {
    defer close(done)
    ticker := time.NewTicker(1 * time.Second) // poll every 1s
    defer ticker.Stop()
    for {
        select {
        case <-cycleCtx.Done():
            return
        case <-ticker.C:
            if !w.config.IsPrimary() {
                w.logger.LogAttrs(cycleCtx, slog.LevelWarn, "demoted during sync, aborting")
                cycleCancel()
                return
            }
        }
    }
}()

err := w.SyncWithRetry(cycleCtx, mode)
cycleCancel() // ensure monitor goroutine exits
<-done        // wait for clean exit per CC-2
```

**Poll interval:** 1 second is a reasonable balance. The `os.Stat()` call is essentially free (FUSE inode lookup). Faster polling (100ms) adds no value since LiteFS failover itself takes seconds.

### Pattern 3: Scheduler Loop Restructure

**What:** `StartScheduler` always runs on all instances. The ticker fires on all nodes, but sync only executes when primary.

**Current structure** (simplified):
```
if isPrimary { go syncWorker.StartScheduler(ctx, interval) }
```

**New structure:**
```
go syncWorker.StartScheduler(ctx, interval)  // always start
```

Inside `StartScheduler`, the loop becomes:
1. Check `w.config.IsPrimary()` at top of each tick
2. If not primary: log at DEBUG level, skip, continue
3. If newly promoted: check last sync time, sync immediately if overdue
4. If primary: run sync with demotion monitor

**Tracking previous role:** Use a local `wasPrimary bool` variable to detect transitions and log them at INFO level (not every tick's "still replica" at DEBUG).

### Anti-Patterns to Avoid

- **Polling too frequently in the monitor goroutine:** More than 1/second wastes CPU for no gain since LiteFS failover is measured in seconds.
- **Checking primary status inside the Sync() function itself:** This would break the clean separation. Sync() takes a context and respects cancellation -- it does not need to know WHY the context was cancelled. The demotion monitor is the caller's responsibility.
- **Using a channel or signal for role changes:** LiteFS does not provide a notification mechanism. Polling the `.primary` file is the documented approach.
- **Removing the `running` atomic guard:** The mutex (`w.running.CompareAndSwap`) must stay. Even with per-cycle context, concurrent triggers (scheduler tick + POST /sync) could overlap.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| File watch for `.primary` | fsnotify/inotify watcher | Simple `os.Stat()` polling | FUSE filesystems have unreliable inotify support. LiteFS docs recommend reading the file directly. The poll cost is negligible. |
| Distributed lock for role | Custom Consul integration | LiteFS's built-in lease management | LiteFS already manages Consul leases. The `.primary` file is the application-facing interface. |

## Common Pitfalls

### Pitfall 1: Race Between Primary Check and Write

**What goes wrong:** Primary check returns true, but between the check and the first database write, the node is demoted. SQLite returns `SQLITE_READONLY` or disk I/O error.

**Why it happens:** The `.primary` file is eventually consistent with Consul lease state. There is an inherent TOCTOU gap.

**How to avoid:** This is unavoidable by design. The existing `Sync()` function already handles errors from database operations (transaction begin, upserts, commit). If a write fails because the node was demoted, the transaction is rolled back and `recordFailure()` is called. The error propagates normally.

**Key insight:** The demotion monitor goroutine is a best-effort optimization to abort EARLY (saving wasted PeeringDB API calls), not a correctness guarantee. The actual safety net is SQLite + LiteFS rejecting writes on replicas.

### Pitfall 2: Initial Sync on Promotion Missing Data

**What goes wrong:** A replica is promoted but the `sync_status` table has no data (replicas don't run schema migration or init).

**Why it happens:** Currently, `InitStatusTable()` and `Schema.Create()` only run if `isPrimary` is true at startup. A promoted replica never ran these.

**How to avoid:** Two options:
1. Run `Schema.Create()` and `InitStatusTable()` on every startup regardless of role (ent's `Schema.Create` is idempotent with `IfNotExists`). This is the simplest fix.
2. Check and run migrations on first promotion detection. More complex, less value.

**Recommendation:** Option 1. Schema migration on replicas is a no-op if the table already exists (replicated from primary). On promotion, the tables already exist because LiteFS replicated them. The only edge case is a brand new cluster where both nodes start simultaneously -- but Consul ensures only one gets the lease.

### Pitfall 3: Logging Spam from Replica Ticks

**What goes wrong:** Every tick (default: 1 hour) on every replica logs "not primary, skipping sync." With N replicas this is N log lines per interval.

**Why it happens:** Logging unconditionally on skip.

**How to avoid:** Only log role transitions at INFO level. Use a `wasPrimary` tracking variable:
- Transition to primary: log INFO "promoted to primary"
- Transition to replica: log INFO "demoted to replica"
- Steady state replica: log DEBUG "skipping sync, not primary" (or skip entirely)

### Pitfall 4: POST /sync Forwarding on Demotion

**What goes wrong:** POST /sync handler checks `IsPrimary()`, sees true, starts sync. Before sync begins writing, node is demoted. The `go syncWorker.SyncWithRetry(ctx, mode)` fails mid-sync.

**How to avoid:** This is the same TOCTOU as Pitfall 1. The existing Fly-Replay header mechanism (`w.Header().Set("Fly-Replay", "leader")`) handles the non-primary case. For the race window, the sync failure path handles the rest. No additional safeguard needed beyond the existing error handling.

### Pitfall 5: Context Leak in Demotion Monitor

**What goes wrong:** The monitor goroutine is started but never stopped because `cycleCancel()` is not called or `<-done` is not awaited.

**Why it happens:** Early return or panic between goroutine start and cleanup.

**How to avoid:** Use `defer cycleCancel()` immediately after creating the context. Use `defer func() { <-done }()` or explicit cleanup after the sync call. Per CC-2, goroutine lifetime MUST be tied to context.

## LiteFS Failover Timing

| Scenario | Failover Time | .primary File Update |
|----------|---------------|---------------------|
| Clean shutdown (SIGTERM) | Immediate (lease destroyed) | Milliseconds |
| Crash / network partition | TTL expiry (default 10s) + lock-delay (default 1s) = ~11s | After new primary elected |
| Manual promotion (`litefs promote`) | Immediate | Milliseconds |

**Source:** [LiteFS Architecture](https://github.com/superfly/litefs/blob/main/docs/ARCHITECTURE.md), [LiteFS Config](https://fly.io/docs/litefs/config/)

**This project's litefs.yml:** Uses default TTL (10s) and lock-delay (1s). Consul-based lease with `promote: true`. The `candidate` field is `${FLY_REGION == PRIMARY_REGION}` which means only nodes in the primary region are candidates.

**Implication for demotion monitor:** A 1-second poll interval in the demotion monitor is appropriate. Failover takes seconds, so sub-second polling would not provide meaningful earlier detection.

## Code Examples

### Modified StartScheduler (sketch)

```go
func (w *Worker) StartScheduler(ctx context.Context, interval time.Duration) {
    mode := w.config.SyncMode
    if mode == "" {
        mode = config.SyncModeFull
    }

    wasPrimary := false

    // Initial check: if we're primary and data exists, mark ready.
    if w.config.IsPrimary() {
        wasPrimary = true
        lastSync, err := GetLastSuccessfulSyncTime(ctx, w.db)
        if err != nil {
            w.logger.LogAttrs(ctx, slog.LevelWarn, "failed to get last sync time",
                slog.String("error", err.Error()),
            )
        }
        if lastSync.IsZero() {
            w.runSyncCycle(ctx, config.SyncModeFull)
        } else {
            w.synced.Store(true)
            if time.Since(lastSync) >= interval {
                w.runSyncCycle(ctx, mode)
            }
        }
    } else {
        // Replica: check if data exists from replication.
        lastSync, _ := GetLastSuccessfulSyncTime(ctx, w.db)
        if !lastSync.IsZero() {
            w.synced.Store(true)
        }
    }

    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            isPrimary := w.config.IsPrimary()
            // Log role transitions.
            if isPrimary && !wasPrimary {
                w.logger.LogAttrs(ctx, slog.LevelInfo, "promoted to primary, checking sync status")
                lastSync, _ := GetLastSuccessfulSyncTime(ctx, w.db)
                if lastSync.IsZero() || time.Since(lastSync) >= interval {
                    w.runSyncCycle(ctx, mode)
                }
            } else if !isPrimary && wasPrimary {
                w.logger.LogAttrs(ctx, slog.LevelInfo, "demoted to replica")
            }
            wasPrimary = isPrimary
            if !isPrimary {
                continue
            }
            w.runSyncCycle(ctx, mode)
        }
    }
}
```

### Modified POST /sync Handler (sketch)

```go
// In main.go: capture the primary check function, not a bool.
isPrimaryFn := func() bool {
    return litefs.IsPrimaryWithFallback(litefs.PrimaryFile, "PDBPLUS_IS_PRIMARY")
}

mux.HandleFunc("POST /sync", func(w http.ResponseWriter, r *http.Request) {
    if !isPrimaryFn() {
        w.Header().Set("Fly-Replay", "leader")
        w.WriteHeader(http.StatusTemporaryRedirect)
        return
    }
    // ... rest unchanged
})
```

### Demotion Monitor Helper (sketch)

```go
// runSyncCycle wraps SyncWithRetry with a demotion monitor.
func (w *Worker) runSyncCycle(ctx context.Context, mode config.SyncMode) {
    cycleCtx, cycleCancel := context.WithCancel(ctx)
    defer cycleCancel()

    done := make(chan struct{})
    go func() {
        defer close(done)
        ticker := time.NewTicker(1 * time.Second)
        defer ticker.Stop()
        for {
            select {
            case <-cycleCtx.Done():
                return
            case <-ticker.C:
                if !w.config.IsPrimary() {
                    w.logger.LogAttrs(cycleCtx, slog.LevelWarn, "demoted during sync, aborting cycle")
                    cycleCancel()
                    return
                }
            }
        }
    }()

    if err := w.SyncWithRetry(cycleCtx, mode); err != nil {
        w.logger.LogAttrs(ctx, slog.LevelError, "sync cycle failed",
            slog.String("error", err.Error()),
        )
    }
    cycleCancel()
    <-done // wait for monitor goroutine to exit per CC-2
}
```

## Discretion Recommendations

### Primary Check Injection: `func() bool` in WorkerConfig

**Recommendation:** Add `IsPrimary func() bool` to `WorkerConfig`. This replaces the dead `IsPrimary bool` field (satisfying DEBT-01) and keeps the sync package testable.

In tests, inject `func() bool { return true }` or a controllable `atomic.Bool` wrapper. In `main.go`, inject the closure wrapping `litefs.IsPrimaryWithFallback()`.

### OTel Metric for Role Transitions

**Recommendation:** Yes, add a counter `pdbplus.role.transitions` with attribute `direction=promoted|demoted`. This is cheap (one counter increment per transition event, not per tick) and valuable for operational visibility in Grafana dashboards (which are planned for v1.5).

### Logging Level and Frequency

**Recommendation:**
- Role transitions (promoted/demoted): `slog.LevelInfo` -- these are operationally significant
- Skipping sync (steady-state replica): `slog.LevelDebug` -- suppressed in production
- Demotion mid-sync abort: `slog.LevelWarn` -- unusual and worth investigating
- Primary tick starting sync: existing INFO logging in `SyncWithRetry` is sufficient

## Project Constraints (from CLAUDE.md)

Key constraints that apply to this task:

- **CC-2 (MUST):** Tie goroutine lifetime to context.Context -- the demotion monitor goroutine must be properly tied to cycleCtx
- **CC-3 (MUST):** Protect shared state -- `w.running` atomic bool is the existing mutex, must be preserved
- **ERR-1 (MUST):** Wrap errors with %w and context
- **OBS-1 (MUST):** Structured logging with slog and consistent fields
- **OBS-5 (SHOULD):** Use attribute setters like slog.String() (already done in existing code)
- **CFG-2 (MUST):** Config immutable after init -- the `IsPrimary` function is set once at construction time
- **CS-5 (MUST):** Use input structs for >2 args -- WorkerConfig already satisfies this
- **API-1 (MUST):** Document exported items
- **T-1 (MUST):** Table-driven tests, deterministic and hermetic

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) |
| Config file | None (stdlib) |
| Quick run command | `go test -race ./internal/sync/ ./internal/litefs/ -count=1` |
| Full suite command | `go test -race ./... -count=1` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| DYN-01 | Scheduler skips sync when not primary | unit | `go test -race ./internal/sync/ -run TestStartScheduler_SkipsOnReplica -count=1` | Wave 0 |
| DYN-02 | Scheduler detects promotion and syncs if overdue | unit | `go test -race ./internal/sync/ -run TestStartScheduler_PromotionSync -count=1` | Wave 0 |
| DYN-03 | Demotion mid-sync cancels context | unit | `go test -race ./internal/sync/ -run TestRunSyncCycle_DemotionAbort -count=1` | Wave 0 |
| DYN-04 | POST /sync uses live primary check | unit | `go test -race ./cmd/peeringdb-plus/ -run TestPostSync_LivePrimary -count=1` | Wave 0 |

### Wave 0 Gaps
- [ ] New test cases for dynamic primary behavior in `internal/sync/worker_test.go`
- [ ] Test helper: controllable `IsPrimary` function (e.g., `atomic.Bool` wrapper)

## Sources

### Primary (HIGH confidence)
- [LiteFS Architecture](https://github.com/superfly/litefs/blob/main/docs/ARCHITECTURE.md) - Lease TTL, failover timing
- [LiteFS Primary Detection](https://fly.io/docs/litefs/primary/) - `.primary` file semantics
- [LiteFS Config](https://fly.io/docs/litefs/config/) - Default TTL (10s), lock-delay (1s)
- Project codebase: `internal/litefs/primary.go`, `internal/sync/worker.go`, `cmd/peeringdb-plus/main.go`

### Secondary (MEDIUM confidence)
- [LiteFS FAQ](https://fly.io/docs/litefs/faq/) - General failover behavior
- [LiteFS How It Works](https://fly.io/docs/litefs/how-it-works/) - Lease system overview

## Metadata

**Confidence breakdown:**
- Architecture patterns: HIGH -- based on direct codebase analysis and Go stdlib patterns
- LiteFS failover timing: MEDIUM -- official docs confirm TTL/lock-delay defaults but .primary file update latency is not explicitly documented
- Pitfalls: HIGH -- derived from codebase analysis and LiteFS documented behavior

**Research date:** 2026-03-24
**Valid until:** 2026-04-24 (stable domain, no fast-moving dependencies)
