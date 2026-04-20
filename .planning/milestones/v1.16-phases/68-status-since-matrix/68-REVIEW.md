---
phase: 68-status-since-matrix
reviewed: 2026-04-19T00:00:00Z
depth: standard
files_reviewed: 19
files_reviewed_list:
  - cmd/peeringdb-plus/main.go
  - internal/config/config.go
  - internal/config/config_test.go
  - internal/pdbcompat/depth.go
  - internal/pdbcompat/filter.go
  - internal/pdbcompat/handler.go
  - internal/pdbcompat/handler_test.go
  - internal/pdbcompat/limit_probe_test.go
  - internal/pdbcompat/registry_funcs.go
  - internal/pdbcompat/registry.go
  - internal/pdbcompat/response.go
  - internal/pdbcompat/status_matrix_test.go
  - internal/sync/delete.go
  - internal/sync/integration_test.go
  - internal/sync/nokey_sync_test.go
  - internal/sync/replay_snapshot_test.go
  - internal/sync/worker_bench_test.go
  - internal/sync/worker.go
  - internal/sync/worker_test.go
findings:
  critical: 0
  warning: 2
  info: 5
  total: 7
status: issues_found
---

# Phase 68: Code Review Report

**Reviewed:** 2026-04-19T00:00:00Z
**Depth:** standard
**Files Reviewed:** 19
**Status:** issues_found

## Summary

Phase 68 delivers four orthogonal changes: (1) removal of `PDBPLUS_INCLUDE_DELETED` with a grace-period WARN, (2) 13 hard-delete closures flipped to soft-delete (`markStaleDeleted*`), (3) `applyStatusMatrix` wired into all 13 list closures plus pk-lookups (inline `StatusIn("ok","pending")` in depth.go), (4) `limit=0 → all rows` semantics + list+`?depth=` guardrail.

Overall the phase is clean and well-instrumented. The `applyStatusMatrix` helper is correctly campus-gated (only `wireCampusFuncs` passes `true`). Every delete closure receives a single `cycleStart time.Time` captured at `Worker.Sync` entry via `start := time.Now()`, threaded unchanged through `syncDeletePass`, and captured by the per-type closure — no `time.Now()` leaks into closures. The `limit=0` behaviour is empirically locked by `TestEntLimitZeroProbe` which asserts ent's `Limit(0) == unlimited` semantics. The deprecation path for `PDBPLUS_INCLUDE_DELETED` is test-locked with both "env_set_warns" and "env_unset_no_warn" sub-tests.

Findings are two WARNING items (double telemetry emission on rollback path, and silent no-op on >32K soft-delete chunks) and five INFO items (dead fields, minor GO-CS-5 drift, redundant nil check).

## Warnings

### WR-01: Double `emitMemoryTelemetry` call on rollback path

**File:** `internal/sync/worker.go:542, 1252`
**Issue:** `rollbackAndRecord` calls `emitMemoryTelemetry`, then calls `recordFailure` which ALSO calls `emitMemoryTelemetry`. On the rollback path (`Worker.Sync` L359, L366, L370), telemetry fires twice: the span attrs `pdbplus.sync.peak_heap_mib` / `pdbplus.sync.peak_rss_mib` are overwritten (harmless), but `slog.Warn("heap threshold crossed", ...)` also fires twice when either threshold is breached, producing duplicate log records that alert-rule aggregation must de-dupe. The `pdbotel.SyncPeakHeapMiB.Store` atomic write is also duplicated. Predates Phase 68 (introduced in a9f509f, Phase 66-01) but `worker.go` is in Phase 68 scope and the issue is visible on every failing sync that breaches thresholds.

**Fix:** Remove the call from `recordFailure` (it was only added so direct `recordFailure` callers from `syncFetchPass` / `Sync` before `Tx.Open` emit telemetry); instead, always emit telemetry at the single top-level terminal path in `Worker.Sync` (after tx.Commit / tx.Rollback / fetch-pass error) via a single `defer` at the top of Sync. Or: remove the call from `rollbackAndRecord` since every rollback path goes through `recordFailure` anyway.

```go
// Option A — defer-based single emission at Sync entry:
func (w *Worker) Sync(ctx context.Context, mode config.SyncMode) error {
    // ... existing prelude ...
    defer w.emitMemoryTelemetry(ctx, w.config.HeapWarnBytes, w.config.RSSWarnBytes)
    // ... remove emitMemoryTelemetry from recordSuccess / rollbackAndRecord / recordFailure ...
}
```

---

### WR-02: Silent no-op when `remoteIDs > maxSQLVars` in `deleteStaleChunked`

**File:** `internal/sync/delete.go:39-53`
**Issue:** When `len(remoteIDs) > maxSQLVars` (30000), `deleteStaleChunked` returns `(0, nil)` with no log, metric, or error — no rows are soft-deleted, and the caller cannot distinguish "nothing to delete" from "too many remote IDs to express as a NOT-IN chunk". Comment flags this for SEED-004 tombstone-GC follow-up but the silent fallthrough means that once any PeeringDB entity crosses 30K rows (netixlan is already ~200K — but it's handled via the full set compared against local rows, so the "delete NOT-IN (remoteIDs)" path would trigger precisely when the remote set is LARGE), deletions stop working without any operator signal.

Behavioural regression risk is low today because netixlan deletes are currently evaluated as "rows whose ID is not in the 200K-strong remote set" — which SQLite would refuse via `too many SQL variables`. The pre-v1.16 hard-delete path had the same silent fallthrough, so this is NOT a Phase 68 regression, but it IS now carried forward into the soft-delete path without improvement.

**Fix:** Emit a `slog.Warn` when `len(remoteIDs) > maxSQLVars` so the condition is visible in logs; leave the no-op behaviour unchanged pending SEED-004:

```go
func deleteStaleChunked(ctx context.Context, remoteIDs []int, deleteFn func([]int) (int, error), typeName string) (int, error) {
    if len(remoteIDs) <= maxSQLVars {
        n, err := deleteFn(remoteIDs)
        if err != nil {
            return 0, fmt.Errorf("mark stale deleted %s: %w", typeName, err)
        }
        return n, nil
    }
    // SEED-004 watch: chunk boundary crossed — no tombstones written this cycle.
    slog.WarnContext(ctx, "soft-delete skipped: remoteIDs exceed maxSQLVars chunk limit",
        slog.String("type", typeName),
        slog.Int("remote_ids", len(remoteIDs)),
        slog.Int("max_vars", maxSQLVars),
    )
    return 0, nil
}
```

Note: `ctx` is currently `_ context.Context` in the signature (see IN-03).

## Info

### IN-01: `QueryOptions.Search`, `.Fields`, `.Depth` are dead fields

**File:** `internal/pdbcompat/registry.go:38-41`
**Issue:** `QueryOptions` defines three fields (`Search`, `Fields`, `Depth`) that are never populated by `serveList` (the struct literal at `handler.go:188-193` sets only `Filters / Limit / Skip / Since`) and never read by any list closure. The `Depth` comment says "only used on detail", but `serveDetail` has its own separate parameter and does not go through `QueryOptions`. Phase 68 research explicitly documented this (patterns.md:321) and Phase 71 may repurpose `Depth`, but in the interim these are dead fields. Minor API-surface clutter.

**Fix:** Delete the unused fields, or add a `// Phase 71 reserved` comment banner that groups them:

```go
type QueryOptions struct {
    Filters []func(*sql.Selector)
    Limit   int
    Skip    int
    Since   *time.Time
    // Phase 71 reserved — not currently populated by serveList.
    // Search string   // ?q= parameter
    // Fields []string // ?fields= parameter
    // Depth  int      // depth parameter (only used on detail)
}
```

---

### IN-02: `applySince` takes entire `QueryOptions` to read one field

**File:** `internal/pdbcompat/registry_funcs.go:49-54`
**Issue:** `applySince(opts QueryOptions)` ignores all fields except `opts.Since`. Per GO-CS-5 / GO-CS-3 a narrower signature would be clearer. Contrasts with `applyStatusMatrix(isCampus, sinceSet bool)` which cleanly takes only the two bits it needs. The `applyStatusMatrix` call site immediately below also re-computes `opts.Since != nil` — redundant with `applySince`'s internal nil check.

**Fix:** Narrow the signature and deduplicate the nil check:

```go
func applySince(since *time.Time) func(*sql.Selector) {
    if since == nil {
        return nil
    }
    return sql.FieldGTE("updated", *since)
}

// caller:
sinceSet := opts.Since != nil
if s := applySince(opts.Since); s != nil {
    preds = append(preds, predicate.Organization(s))
}
preds = append(preds, predicate.Organization(applyStatusMatrix(false, sinceSet)))
```

---

### IN-03: `deleteStaleChunked` declares `_ context.Context` then discards it

**File:** `internal/sync/delete.go:39`
**Issue:** `func deleteStaleChunked(_ context.Context, ...)` takes a context but immediately discards it. Every caller threads `ctx` into the returned closure via a separate capture (e.g. `markStaleDeletedOrganizations` line 62-65), so the parameter exists only to satisfy an ergonomic convention. Per GO-CTX-1 this is odd — either use the context (e.g. for the diagnostic log suggested in WR-02) or remove the parameter.

**Fix:** Once WR-02 is addressed and uses `ctx` for `slog.WarnContext`, rename `_` to `ctx` and use it. If WR-02 is declined, remove the parameter entirely:

```go
func deleteStaleChunked(ctx context.Context, remoteIDs []int, deleteFn func([]int) (int, error), typeName string) (int, error) {
    // ... use ctx in the slog.WarnContext call from WR-02
```

---

### IN-04: `X-Powered-By` version string is hardcoded

**File:** `internal/pdbcompat/response.go:21`
**Issue:** `poweredByHeader = "PeeringDB-Plus/1.1"` is a string literal not tied to any build-time version stamp or `go.mod` tag. Any release bump requires a manual edit here. Not a bug but a maintenance hazard — v1.16 is shipping with a "1.1" wire string.

**Fix:** Tie to `runtime/debug.ReadBuildInfo` or inject at link time via `-ldflags -X`:

```go
// Read once at init; fall back to "dev" when module info unavailable (e.g. tests).
var poweredByHeader = func() string {
    if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
        return "PeeringDB-Plus/" + info.Main.Version
    }
    return "PeeringDB-Plus/dev"
}()
```

Note: outside the Phase 68 blast radius; flagged because `response.go` is in scope and the drift is long-standing.

---

### IN-05: `TestStatusMatrix/status_deleted_no_since_is_empty` double-checks behaviour already guaranteed by the Fields-map removal

**File:** `internal/pdbcompat/status_matrix_test.go:197-215`
**Issue:** The sub-test passes `?status=deleted` to `/api/net` and asserts zero results. The assertion is correct but the mechanism is slightly subtle: `status` is no longer a key in `registry.go`'s Fields map for any type, so `ParseFilters` silently drops `status=deleted` per D-20 (unknown-field silent-ignore). The test would pass even if `applyStatusMatrix` were broken, so it does not specifically lock the STATUS-04 invariant. The test comment acknowledges this but the name "status_deleted_no_since_is_empty" suggests it's asserting the matrix behaviour.

**Fix:** Rename to `fields_map_drops_status_param` or add a second sub-test that directly exercises `applyStatusMatrix(false, false)` returning `FieldEQ("status","ok")` via a unit test that builds a selector and inspects its predicate chain. Minor — the observable behaviour is tested correctly; this is a clarity nit.

---

_Reviewed: 2026-04-19T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
