---
phase: quick-260324-lc5
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/sync/worker.go
  - internal/sync/worker_test.go
  - internal/otel/metrics.go
  - cmd/peeringdb-plus/main.go
autonomous: true
requirements: [DYN-01, DYN-02, DYN-03, DYN-04]

must_haves:
  truths:
    - "Scheduler runs on ALL instances, not just primary"
    - "Scheduler skips sync when node is not primary"
    - "Newly promoted node checks last sync time and syncs immediately if overdue"
    - "Demotion during active sync cancels the sync context"
    - "POST /sync handler detects primary status live on each request"
    - "Role transitions are logged at INFO level; steady-state replica skips at DEBUG"
  artifacts:
    - path: "internal/sync/worker.go"
      provides: "Dynamic primary detection in scheduler + demotion monitor"
      contains: "IsPrimary func() bool"
    - path: "internal/sync/worker_test.go"
      provides: "Tests for DYN-01 through DYN-03"
      contains: "TestStartScheduler_SkipsOnReplica"
    - path: "internal/otel/metrics.go"
      provides: "Role transition counter metric"
      contains: "pdbplus.role.transitions"
    - path: "cmd/peeringdb-plus/main.go"
      provides: "Always-start scheduler + live isPrimary in POST /sync"
      contains: "go syncWorker.StartScheduler"
  key_links:
    - from: "cmd/peeringdb-plus/main.go"
      to: "internal/sync/worker.go"
      via: "IsPrimary func() bool injected into WorkerConfig"
      pattern: "IsPrimary.*litefs\\.IsPrimaryWithFallback"
    - from: "internal/sync/worker.go (runSyncCycle)"
      to: "internal/sync/worker.go (SyncWithRetry)"
      via: "per-cycle derived context with demotion monitor goroutine"
      pattern: "context\\.WithCancel"
    - from: "cmd/peeringdb-plus/main.go (POST /sync)"
      to: "internal/litefs/primary.go"
      via: "live call to isPrimaryFn() on each request"
      pattern: "isPrimaryFn\\(\\)"
---

<objective>
Convert static startup-time primary detection into dynamic per-tick detection inside the sync scheduler.

Purpose: Allow the sync process to detect promotion/demotion at runtime without requiring a restart when another LiteFS instance is promoted. This is critical for high-availability edge deployment on Fly.io where Consul lease changes can move the primary role between nodes.

Output: Modified scheduler that runs on all instances, gates sync on live primary check, aborts mid-sync on demotion, and a POST /sync handler that uses live primary detection.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/STATE.md
@.planning/quick/260324-lc5-dynamic-primary-detection-on-sync-cycle-/260324-lc5-CONTEXT.md
@.planning/quick/260324-lc5-dynamic-primary-detection-on-sync-cycle-/260324-lc5-RESEARCH.md

@internal/sync/worker.go
@internal/sync/worker_test.go
@internal/sync/status.go
@internal/otel/metrics.go
@internal/litefs/primary.go
@cmd/peeringdb-plus/main.go

<interfaces>
<!-- Key types and contracts the executor needs. -->

From internal/sync/worker.go:
```go
// WorkerConfig holds configuration for the sync worker.
type WorkerConfig struct {
    IncludeDeleted bool
    IsPrimary      bool          // DEAD FIELD — replace with func() bool
    SyncMode       config.SyncMode
}

// Worker orchestrates PeeringDB data synchronization.
type Worker struct {
    pdbClient      *peeringdb.Client
    entClient      *ent.Client
    db             *sql.DB
    config         WorkerConfig
    running        atomic.Bool
    synced         atomic.Bool
    logger         *slog.Logger
    retryBackoffs  []time.Duration
}

func NewWorker(pdbClient *peeringdb.Client, entClient *ent.Client, db *sql.DB, cfg WorkerConfig, logger *slog.Logger) *Worker
func (w *Worker) StartScheduler(ctx context.Context, interval time.Duration)
func (w *Worker) SyncWithRetry(ctx context.Context, mode config.SyncMode) error
func (w *Worker) Sync(ctx context.Context, mode config.SyncMode) error
func (w *Worker) HasCompletedSync() bool
func GetLastSuccessfulSyncTime(ctx context.Context, db *sql.DB) (time.Time, error)
```

From internal/litefs/primary.go:
```go
const PrimaryFile = "/litefs/.primary"
func IsPrimary() bool
func IsPrimaryAt(path string) bool
func IsPrimaryWithFallback(path string, envKey string) bool
```

From internal/otel/metrics.go:
```go
// All existing metrics are package-level vars registered via InitMetrics().
// Pattern: declare var, register in InitMetrics(), use elsewhere via pdbotel.VarName.
var SyncDuration metric.Float64Histogram
var SyncOperations metric.Int64Counter
// ... etc
func InitMetrics() error
```

From cmd/peeringdb-plus/main.go:
```go
// Line 90: static detection (to be replaced with live function)
isPrimary := litefs.IsPrimaryWithFallback(litefs.PrimaryFile, "PDBPLUS_IS_PRIMARY")

// Line 132-135: WorkerConfig construction (IsPrimary bool currently unused)
syncWorker := pdbsync.NewWorker(pdbClient, entClient, db, pdbsync.WorkerConfig{
    IncludeDeleted: cfg.IncludeDeleted,
    SyncMode:       cfg.SyncMode,
}, logger)

// Line 138-140: conditional scheduler start (to become unconditional)
if isPrimary {
    go syncWorker.StartScheduler(ctx, cfg.SyncInterval)
}

// Line 153-158: static isPrimary in POST /sync closure (to become live call)
mux.HandleFunc("POST /sync", func(w http.ResponseWriter, r *http.Request) {
    if !isPrimary {
        w.Header().Set("Fly-Replay", "leader")
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Refactor Worker for dynamic primary detection with demotion monitor</name>
  <files>internal/sync/worker.go, internal/sync/worker_test.go, internal/otel/metrics.go</files>
  <behavior>
    - DYN-01: Worker with IsPrimary returning false never calls SyncWithRetry in StartScheduler. Verify by running scheduler for 2 ticks with IsPrimary=false, assert zero sync calls.
    - DYN-02: Worker detects promotion (IsPrimary flips true), checks last sync time, syncs immediately if overdue (last sync + interval < now). Verify by setting IsPrimary=false initially, flipping to true after 1 tick, assert sync is triggered.
    - DYN-02b: Worker detects promotion but last sync is recent (not overdue), does NOT sync immediately — waits for next tick.
    - DYN-03: Demotion mid-sync cancels the cycle context. Verify by starting a sync with IsPrimary=true, flipping to false during sync execution, assert context is cancelled (SyncWithRetry returns context cancellation error).
    - Role transition logging: promoted->INFO, demoted->INFO, steady-state replica tick->DEBUG.
    - Nil IsPrimary func in WorkerConfig defaults to always-primary (backward compat for tests).
    - RoleTransitions OTel counter incremented on promotion and demotion with direction attribute.
  </behavior>
  <action>
**1. Add role transition metric to `internal/otel/metrics.go`:**
- Add `var RoleTransitions metric.Int64Counter` package-level variable.
- Register in `InitMetrics()` as `pdbplus.role.transitions` with description "Role transition events (promoted/demoted)" and unit `{event}`.
- Follow the exact same pattern as existing counters (e.g., SyncTypeFallback).

**2. Refactor `WorkerConfig.IsPrimary` in `internal/sync/worker.go`:**
- Change `IsPrimary bool` to `IsPrimary func() bool` in `WorkerConfig`. This removes the dead bool field (tech debt DEBT-01).
- In `NewWorker()`, if `cfg.IsPrimary == nil`, default to `func() bool { return true }` for backward compatibility.

**3. Add `runSyncCycle` method to Worker:**
- Create `func (w *Worker) runSyncCycle(ctx context.Context, mode config.SyncMode)`.
- Derive `cycleCtx, cycleCancel := context.WithCancel(ctx)`.
- `defer cycleCancel()` immediately.
- Start a monitor goroutine that polls `w.config.IsPrimary()` every 1 second. If false, log at WARN "demoted during sync, aborting cycle", call `cycleCancel()`, and return. Exit on `cycleCtx.Done()`.
- Use a `done := make(chan struct{})` closed by the goroutine's defer. After `SyncWithRetry(cycleCtx, mode)` returns, call `cycleCancel()` then `<-done` to ensure the goroutine exits cleanly per CC-2.
- On sync error, log at ERROR.

**4. Rewrite `StartScheduler` to run on all instances:**
- Remove the assumption that StartScheduler only runs on primary.
- Add a `wasPrimary bool` local variable initialized to false.
- **Initial check block:** If `w.config.IsPrimary()` returns true, set `wasPrimary = true`, get last sync time. If `lastSync.IsZero()`, run `w.runSyncCycle(ctx, config.SyncModeFull)`. If not zero, mark `w.synced.Store(true)` and if overdue (`time.Since(lastSync) >= interval`), run `w.runSyncCycle(ctx, mode)`. If not primary, check if data exists from replication (`GetLastSuccessfulSyncTime`), if so mark `w.synced.Store(true)`.
- **Ticker loop:** On each tick, check `isPrimary := w.config.IsPrimary()`. Detect transitions:
  - `isPrimary && !wasPrimary`: log INFO "promoted to primary, checking sync status", increment `pdbotel.RoleTransitions` with attribute `direction=promoted`. Check last sync time; if overdue or zero, run sync cycle immediately.
  - `!isPrimary && wasPrimary`: log INFO "demoted to replica", increment `pdbotel.RoleTransitions` with attribute `direction=demoted`.
  - `!isPrimary` (steady state): log DEBUG "not primary, skipping sync", continue.
  - `isPrimary` (steady state): run `w.runSyncCycle(ctx, mode)`.
  - Update `wasPrimary = isPrimary` after transition handling.
- Remove the old pre-tick delay logic (the timer-based wait for `lastSync.Add(interval)`). The new approach runs an immediate sync cycle if overdue, and otherwise waits for the next tick. This is simpler and handles both initial boot and promotion cases uniformly.

**5. Write tests in `internal/sync/worker_test.go`:**
- Create a test helper: `type primarySwitch struct { v atomic.Bool }` with `func (p *primarySwitch) IsPrimary() bool { return p.v.Load() }`. This is controllable in tests.
- **TestStartScheduler_SkipsOnReplica (DYN-01):** Create Worker with IsPrimary=false, short interval (50ms), mock PeeringDB server returning empty data. Run StartScheduler in a goroutine, wait 150ms, cancel ctx. Assert zero API calls (fixture.callCount == 0).
- **TestStartScheduler_PromotionSync (DYN-02):** Create Worker with primarySwitch starting false. After 1 tick, flip to true. Assert sync is triggered (callCount > 0 or check synced flag).
- **TestRunSyncCycle_DemotionAbort (DYN-03):** Create Worker with primarySwitch starting true. Start runSyncCycle. Use a mock PeeringDB server with a slow response (delay handler). Flip primarySwitch to false during the sync. Assert that the cycle context is cancelled (sync returns early with context error).
- Use table-driven tests per T-1. Run with `-race` per T-2. Mark safe tests with `t.Parallel()` per T-3.
- Call `ensureMetrics(t)` at the start of each test (existing pattern).
- For tests that need a DB for `GetLastSuccessfulSyncTime`, use `testutil` to create a temp SQLite DB with `InitStatusTable`.
  </action>
  <verify>
    <automated>cd /home/dotwaffle/Code/pdb/peeringdb-plus && go test -race ./internal/sync/ -run "TestStartScheduler_SkipsOnReplica|TestStartScheduler_PromotionSync|TestRunSyncCycle_DemotionAbort" -count=1 -v</automated>
  </verify>
  <done>
    - WorkerConfig.IsPrimary is `func() bool` (dead bool removed)
    - NewWorker defaults nil IsPrimary to always-true
    - StartScheduler runs on all instances, gates sync on IsPrimary() per tick
    - runSyncCycle wraps SyncWithRetry with demotion monitor goroutine (1s poll)
    - Role transitions logged at INFO, steady-state replica at DEBUG
    - RoleTransitions counter registered in OTel metrics
    - All three new tests pass with -race
  </done>
</task>

<task type="auto">
  <name>Task 2: Wire dynamic primary detection in main.go</name>
  <files>cmd/peeringdb-plus/main.go</files>
  <action>
**1. Create `isPrimaryFn` closure:**
- After the existing `isPrimary` bool assignment (line 90), create a function closure:
  ```go
  isPrimaryFn := func() bool {
      return litefs.IsPrimaryWithFallback(litefs.PrimaryFile, "PDBPLUS_IS_PRIMARY")
  }
  ```
- Keep the static `isPrimary` bool for the one-time schema migration and InitStatusTable gates (lines 93-106). These MUST still run only when the node starts as primary. The research (Pitfall 2) confirms replicas already have tables replicated from primary, so migration on promotion is unnecessary.

**2. Pass `isPrimaryFn` into WorkerConfig:**
- Update the `NewWorker` call (line 132) to include `IsPrimary: isPrimaryFn` in the WorkerConfig struct literal.

**3. Make scheduler start unconditional:**
- Remove the `if isPrimary {` guard around `go syncWorker.StartScheduler(ctx, cfg.SyncInterval)` (lines 138-140).
- Change to just: `go syncWorker.StartScheduler(ctx, cfg.SyncInterval)` -- always start the scheduler on all instances. The scheduler itself now gates sync on `w.config.IsPrimary()`.

**4. Replace static isPrimary in POST /sync handler:**
- In the `POST /sync` handler (line 153-158), replace `if !isPrimary {` with `if !isPrimaryFn() {`. This makes the handler detect role changes live without restart.

**5. Update startup log:**
- The `slog.Bool("is_primary", isPrimary)` in the server start log (line 258) should remain as-is -- it logs the initial state at boot, which is still useful.
  </action>
  <verify>
    <automated>cd /home/dotwaffle/Code/pdb/peeringdb-plus && go build ./cmd/peeringdb-plus/ && go vet ./cmd/peeringdb-plus/ && go test -race ./... -count=1</automated>
  </verify>
  <done>
    - isPrimaryFn closure wraps litefs.IsPrimaryWithFallback for live detection
    - WorkerConfig.IsPrimary field set to isPrimaryFn
    - Scheduler starts unconditionally on all instances (no isPrimary guard)
    - POST /sync uses isPrimaryFn() for live role check
    - Static isPrimary retained only for one-time schema migration gates
    - Full test suite passes (no regressions), binary builds cleanly
  </done>
</task>

</tasks>

<verification>
1. `go build ./cmd/peeringdb-plus/` -- binary compiles without errors
2. `go vet ./...` -- no vet warnings
3. `go test -race ./internal/sync/ -count=1 -v` -- all sync tests pass including new DYN tests
4. `go test -race ./... -count=1` -- full suite passes, no regressions
5. Verify `WorkerConfig.IsPrimary` type is `func() bool` (not `bool`): `grep "IsPrimary.*func" internal/sync/worker.go`
6. Verify scheduler is always started: `grep -A1 "go syncWorker.StartScheduler" cmd/peeringdb-plus/main.go` shows no `if isPrimary` guard
7. Verify POST /sync uses live check: `grep "isPrimaryFn" cmd/peeringdb-plus/main.go`
</verification>

<success_criteria>
- The scheduler runs on ALL instances regardless of initial primary status
- Primary detection happens dynamically at the top of each scheduler tick
- A sync in progress is aborted (context cancelled) when the node is demoted
- A newly promoted node checks whether sync is overdue and acts accordingly
- POST /sync detects primary status live per request (no restart needed)
- Role transitions emit OTel counter metrics with direction attribute
- All existing tests continue to pass (IsPrimary nil defaults to always-true)
- No goroutine leaks: demotion monitor always exits via deferred cleanup
</success_criteria>

<output>
After completion, create `.planning/quick/260324-lc5-dynamic-primary-detection-on-sync-cycle-/260324-lc5-SUMMARY.md`
</output>
