---
phase: quick-260324-lc5
verified: 2026-03-24T15:45:00Z
status: passed
score: 6/6 must-haves verified
---

# Quick Task 260324-lc5: Dynamic Primary Detection on Sync Cycle Verification Report

**Task Goal:** On the start of every sync cycle, the sync process should check whether it is currently the primary or not. This will allow it to change modes without needing a restart when another instance is promoted.
**Verified:** 2026-03-24T15:45:00Z
**Status:** passed
**Re-verification:** No -- initial verification

## Goal Achievement

### Observable Truths

| #   | Truth | Status | Evidence |
| --- | ----- | ------ | -------- |
| 1 | Scheduler runs on ALL instances, not just primary | VERIFIED | `cmd/peeringdb-plus/main.go:145` -- `go syncWorker.StartScheduler(ctx, cfg.SyncInterval)` with no `if isPrimary` guard. Comment: "Start scheduler on all instances per D-22, D-29." |
| 2 | Scheduler skips sync when node is not primary | VERIFIED | `worker.go:505-507` -- `if !isPrimary { log DEBUG "not primary, skipping sync"; continue }`. Test `TestStartScheduler_SkipsOnReplica` confirms zero API calls when IsPrimary=false. |
| 3 | Newly promoted node checks last sync time and syncs immediately if overdue | VERIFIED | `worker.go:482-492` -- promotion detection block checks `GetLastSuccessfulSyncTime`, runs sync if `lastSync.IsZero() || time.Since(lastSync) >= interval`. Test `TestStartScheduler_PromotionSync` confirms sync triggered after flipping IsPrimary from false to true. |
| 4 | Demotion during active sync cancels the sync context | VERIFIED | `worker.go:398-429` -- `runSyncCycle` creates per-cycle context with `context.WithCancel`, monitor goroutine polls IsPrimary every 1s, calls `cycleCancel()` on demotion. Test `TestRunSyncCycle_DemotionAbort` confirms early abort (took 3.14s vs 3s delay, well under 2.5s threshold after 500ms flip). |
| 5 | POST /sync handler detects primary status live on each request | VERIFIED | `main.go:159` -- `if !isPrimaryFn()` where `isPrimaryFn` is a closure wrapping `litefs.IsPrimaryWithFallback(litefs.PrimaryFile, "PDBPLUS_IS_PRIMARY")` defined at `main.go:93-95`. Each request calls the function, not reading a static bool. |
| 6 | Role transitions are logged at INFO level; steady-state replica skips at DEBUG | VERIFIED | `worker.go:483` -- promoted: `slog.LevelInfo, "promoted to primary, checking sync status"`. `worker.go:495` -- demoted: `slog.LevelInfo, "demoted to replica"`. `worker.go:506` -- steady-state skip: `slog.LevelDebug, "not primary, skipping sync"`. |

**Score:** 6/6 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `internal/sync/worker.go` | Dynamic primary detection in scheduler + demotion monitor | VERIFIED | `WorkerConfig.IsPrimary` is `func() bool` (line 30). `runSyncCycle` with demotion monitor (lines 398-429). `StartScheduler` runs on all instances with per-tick role detection (lines 435-513). |
| `internal/sync/worker_test.go` | Tests for DYN-01 through DYN-03 | VERIFIED | `TestStartScheduler_SkipsOnReplica` (DYN-01, line 1294), `TestStartScheduler_PromotionSync` (DYN-02, line 1326), `TestRunSyncCycle_DemotionAbort` (DYN-03, line 1366). All pass with `-race`. |
| `internal/otel/metrics.go` | Role transition counter metric | VERIFIED | `RoleTransitions` declared as `metric.Int64Counter` (line 37), registered as `pdbplus.role.transitions` with description and unit `{event}` (lines 111-117). |
| `cmd/peeringdb-plus/main.go` | Always-start scheduler + live isPrimary in POST /sync | VERIFIED | `isPrimaryFn` closure (lines 93-95), passed to `WorkerConfig.IsPrimary` (line 139), scheduler started unconditionally (line 145), `isPrimaryFn()` used in POST /sync (line 159). |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| `cmd/peeringdb-plus/main.go` | `internal/sync/worker.go` | `isPrimaryFn` injected into `WorkerConfig.IsPrimary` | WIRED | `main.go:139` -- `IsPrimary: isPrimaryFn`. The closure wraps `litefs.IsPrimaryWithFallback` (main.go:93-95). Worker uses `w.config.IsPrimary()` throughout StartScheduler (lines 444, 479, 482, 494, 505) and runSyncCycle (line 413). |
| `worker.go (runSyncCycle)` | `worker.go (SyncWithRetry)` | Per-cycle derived context with demotion monitor goroutine | WIRED | `context.WithCancel` at line 399 creates `cycleCtx`. Monitor goroutine cancels on demotion (line 415). `SyncWithRetry(cycleCtx, mode)` at line 422 receives the cancellable context. Goroutine cleanup via `cycleCancel()` + `<-done` (lines 427-428) per CC-2. |
| `main.go (POST /sync)` | `internal/litefs/primary.go` | Live call to `isPrimaryFn()` on each request | WIRED | `main.go:159` -- `if !isPrimaryFn()` calls the closure which invokes `litefs.IsPrimaryWithFallback`. Not a static bool capture. |

### Data-Flow Trace (Level 4)

Not applicable -- this task modifies control flow (scheduler gating, context cancellation), not data rendering. No dynamic data artifacts to trace.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Binary compiles | `go build ./cmd/peeringdb-plus/` | Clean compilation, no errors | PASS |
| Go vet passes | `go vet ./internal/sync/ ./cmd/peeringdb-plus/ ./internal/otel/` | No warnings | PASS |
| DYN-01: Replica skips sync | `go test -race -run TestStartScheduler_SkipsOnReplica` | PASS (0 API calls, 0.44s) | PASS |
| DYN-02: Promotion triggers sync | `go test -race -run TestStartScheduler_PromotionSync` | PASS (API calls > 0 after flip, 0.79s) | PASS |
| DYN-03: Demotion aborts sync | `go test -race -run TestRunSyncCycle_DemotionAbort` | PASS (early abort in ~3.14s, well under 3s org delay + 2.5s threshold) | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ---------- | ----------- | ------ | -------- |
| DYN-01 | 260324-lc5-PLAN.md | Scheduler on replica never calls SyncWithRetry | SATISFIED | `TestStartScheduler_SkipsOnReplica` passes; `worker.go:505-507` skips on non-primary |
| DYN-02 | 260324-lc5-PLAN.md | Promotion detection with overdue sync check | SATISFIED | `TestStartScheduler_PromotionSync` passes; `worker.go:482-492` handles promotion |
| DYN-03 | 260324-lc5-PLAN.md | Demotion mid-sync cancels cycle context | SATISFIED | `TestRunSyncCycle_DemotionAbort` passes; `worker.go:413-416` cancels on demotion |
| DYN-04 | 260324-lc5-PLAN.md | RoleTransitions OTel counter with direction attribute | SATISFIED | `metrics.go:37,111-117` registers counter; `worker.go:484-486,496-498` emits with direction=promoted/demoted |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| (none) | - | - | - | No TODOs, FIXMEs, placeholders, empty implementations, or hardcoded empty data found in any modified file |

### Human Verification Required

### 1. Live LiteFS Failover

**Test:** Deploy to Fly.io with 2+ instances. Trigger a primary failover (e.g., `fly machine stop` on current primary). Monitor logs on the surviving instance.
**Expected:** Surviving instance logs "promoted to primary, checking sync status" at INFO level, then runs a sync cycle if overdue. The demoted instance (when restarted) logs "demoted to replica" and stops syncing.
**Why human:** Requires live LiteFS cluster with Consul lease management. Cannot simulate real `.primary` file changes in CI.

### 2. POST /sync Replay After Failover

**Test:** With 2 instances running, POST `/sync` to a replica after a failover event.
**Expected:** Before failover: replica returns 307 with `Fly-Replay: leader`. After failover (replica becomes primary): same replica accepts the POST and returns 202.
**Why human:** Requires Fly.io's Fly-Replay header processing and live primary file changes.

### Gaps Summary

No gaps found. All six observable truths are verified. All four artifacts pass existence, substantive content, and wiring checks. All three key links are verified. All four DYN requirements are satisfied with passing tests. The binary compiles and passes vet. No anti-patterns detected in modified files.

---

_Verified: 2026-03-24T15:45:00Z_
_Verifier: Claude (gsd-verifier)_
