---
phase: 68-status-since-matrix
fixed_at: 2026-04-19T00:00:00Z
review_path: .planning/phases/68-status-since-matrix/68-REVIEW.md
iteration: 1
findings_in_scope: 2
fixed: 2
skipped: 0
status: all_fixed
---

# Phase 68: Code Review Fix Report

**Fixed at:** 2026-04-19T00:00:00Z
**Source review:** .planning/phases/68-status-since-matrix/68-REVIEW.md
**Iteration:** 1

**Summary:**
- Findings in scope: 2 (WR-01, WR-02; INFO items out of scope per fix_scope=critical_warning)
- Fixed: 2
- Skipped: 0

Full sanity check (`go build ./...` and `go test -race ./...`) ran clean
after both commits.

## Fixed Issues

### WR-01: Double `emitMemoryTelemetry` call on rollback path

**Files modified:** `internal/sync/worker.go`
**Commit:** a891b37
**Applied fix:** Removed the `w.emitMemoryTelemetry(ctx, ...)` call from
`rollbackAndRecord`. Every rollback path in `Worker.Sync` (L359, L366,
L370) delegates to `rollbackAndRecord`, which unconditionally calls
`recordFailure` — and `recordFailure` already emits memory telemetry at
its entry. Dropping the redundant call at `rollbackAndRecord:542`
eliminates duplicate `slog.Warn("heap threshold crossed", ...)` records
and duplicate `pdbotel.SyncPeakHeapMiB.Store` writes on threshold-breach
rollbacks. Left a code comment at the former call site explaining why
the emission was deliberately removed (so a future reader doesn't
re-introduce it for symmetry).

Chose the "remove from rollbackAndRecord" variant over the "single defer
at Sync entry" variant — it's the smaller, more local change and
preserves the existing call graph. `recordFailure` is still invoked on
all three non-rollback failure paths (fetch error, tx.Open error,
checkMemoryLimit error) where it is the sole emission site.

### WR-02: Silent no-op when `remoteIDs > maxSQLVars` in `deleteStaleChunked`

**Files modified:** `internal/sync/delete.go`
**Commit:** 898e5c0
**Applied fix:** Added `log/slog` import and emitted
`slog.WarnContext(ctx, "soft-delete skipped: remoteIDs exceed maxSQLVars
chunk limit, SEED-004 trigger candidate", ...)` with structured attrs
`type`, `remote_ids`, `max_vars` before the `return 0, nil` fallthrough.
Promoted the formerly-discarded `_ context.Context` parameter to a live
`ctx` — this incidentally addresses IN-03 (context-discard nit) which
flagged the `_` naming as a GO-CTX-1 oddity pending WR-02's resolution.

No-op behaviour itself is unchanged — the WARN is purely a signal for
operators / alert rules so the condition becomes visible ahead of SEED-004's
tombstone-GC design work. No test updates needed: the existing sync
integration tests do not exercise the >30K path, and adding a unit
test that allocates a 30001-element slice feels like overkill for a
log-emission change. The behavioural contract (return `(0, nil)`) is
preserved.

## Skipped Issues

None.

## Info Findings (Out of Scope)

Five INFO items (IN-01 through IN-05) were flagged in REVIEW.md but
are outside `fix_scope=critical_warning`. IN-03 (`_ context.Context`
discarded) was incidentally resolved by the WR-02 fix since the context
is now consumed by `slog.WarnContext`.

The remaining INFO items (IN-01 dead `QueryOptions` fields, IN-02
`applySince` signature narrowing, IN-04 `X-Powered-By` version
hardcoding, IN-05 test-name/assertion-mechanism mismatch) are left for a
future pass — none are load-bearing.

---

_Fixed: 2026-04-19T00:00:00Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
