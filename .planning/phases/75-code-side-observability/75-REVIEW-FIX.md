---
phase: 75-code-side-observability
fixed_at: 2026-04-26T23:59:00Z
review_path: .planning/phases/75-code-side-observability/75-REVIEW.md
iteration: 1
findings_in_scope: 4
fixed: 4
skipped: 0
deferred: 3
status: applied
---

# Phase 75: Code Review Fix Report

**Fixed at:** 2026-04-26T23:59:00Z
**Source review:** `.planning/phases/75-code-side-observability/75-REVIEW.md`
**Iteration:** 1
**Scope:** Critical + Warning (`--auto`)

**Summary:**

- Findings in scope: 4 (4 warnings; 0 blockers)
- Fixed: 4
- Skipped: 0
- Deferred to a future `--all` pass: 3 (INFO findings)

## Fixed Issues

### WR-01: TestPrewarmCounters_NoError missing OTEL_METRICS_EXPORTER=none setup

**Files modified:** `internal/otel/prewarm_test.go`
**Commit:** `c7d6657`
**Status:** FIXED
**Applied fix:** Added `t.Setenv("OTEL_METRICS_EXPORTER", "none")` at the top of `TestPrewarmCounters_NoError` (before `InitMetrics()` is invoked) so the test does not attempt to dial a real OTLP endpoint via autoexport. Matches the established package convention (10 occurrences in `internal/otel/metrics_test.go`).
**Verification:** `go test -race -count=1 ./internal/otel/...` — PASS.

### WR-02: InitialObjectCounts does not check ctx between per-type queries

**Files modified:** `internal/sync/initialcounts.go`
**Commit:** `0d6f01c`
**Status:** FIXED
**Applied fix:** Added a defensive `if err := ctx.Err(); err != nil { return nil, fmt.Errorf("count %s: %w", q.name, err) }` check at the top of each loop iteration. Preserves the existing per-table error-wrap pattern; ensures a SIGTERM mid-boot unwinds promptly even when the SQLite/FUSE driver is briefly insensitive to ctx cancellation during LiteFS hydration.
**Verification:** `go test -race -count=1 ./internal/sync/...` — PASS.

### WR-03: PrewarmCounters has no defensive nil-check on counter vars

**Files modified:** `internal/otel/prewarm.go`, `internal/otel/prewarm_test.go`
**Commit:** `b430973`
**Status:** FIXED
**Applied fix:** Implemented the reviewer's preferred option (1):

1. Added `"fmt"` and `"go.opentelemetry.io/otel"` imports.
2. Added a 5-counter named-table guard at the top of `PrewarmCounters` that fires `otel.Handle(fmt.Errorf("prewarm: counter %q is nil — InitMetrics() must run first", name))` once per nil counter, surfacing the misordering as a startup WARN without panicking.
3. Wrapped each `.Add(ctx, 0, ...)` call in a per-counter nil-check so the function safely no-ops on the missing counter.
4. Updated the `PrewarmCounters` doc comment — the previous text claimed "calling on nil counters will panic"; the new text describes the otel.Handle surfacing path and cites REVIEW WR-03.
5. Updated `TestPrewarmCounters_NoError` docstring to match the new contract (no longer asserts panic; asserts no-error happy path).

**Verification:** `go test -race -count=1 ./internal/otel/... ./cmd/peeringdb-plus/...` — PASS. `golangci-lint run ./...` — 0 issues.

### WR-04: TestPeeringDBEntityTypes_ParityNote provides zero enforcement

**Files modified:** `internal/otel/prewarm_test.go`
**Commit:** `9210bf2`
**Status:** FIXED
**Applied fix:** Replaced `TestPeeringDBEntityTypes_ParityNote` (a `t.Log`-only test) with `TestPeeringDBEntityTypes_Parity`, an order-agnostic set-equality test that compares `PeeringDBEntityTypes` against a canonical 13-entity golden whose source of truth is `internal/sync/initialcounts.go`'s `queries` slice. Uses `slices.Sorted(slices.Values(...))` + `slices.Equal` for an order-agnostic comparison; emits a `t.Errorf` with both sorted slices on drift, plus a "Update internal/otel/prewarm.go AND internal/sync/initialcounts.go together" remediation hint. Cites Phase 75 D-02 in the test docstring.

**Drift mode now caught:** same-cardinality renames (e.g. the deferred DEFER-70-06-01 `"campus"` → `"campuses"` rename). The sibling `_Cardinality` count check would silently pass on this drift; the new parity test catches it.

**Verification:**
- `go test -race -count=1 ./internal/otel/...` — PASS.
- Deliberate-mutation roundtrip: temporarily renamed `"campus"` → `"campuses"` in `internal/otel/prewarm.go`; the new test fired with the expected diff, then revert + green re-run confirmed.

## Deferred Issues (out of scope for `--auto`)

These INFO findings from `75-REVIEW.md` are recorded for a future `--all` pass. None block the v1.18.0 phase ship.

### IN-01: 14-line closure-table for what could be a 13-line slice of {name,query} pairs

**File:** `internal/sync/initialcounts.go:58-76`
**Status:** DEFERRED (style preference; reviewer explicitly classified as "not a defect").
**Reason:** Phase 75 SUMMARY records the closure-table as a conscious choice. Optional refactor only.

### IN-02: Doc-comment line-number references will rot

**Files:**
- `cmd/peeringdb-plus/main.go:286-287`, `:910-913`
- `internal/otel/prewarm.go:40-49`

**Status:** DEFERRED (cosmetic; reviewer explicitly notes `internal/sync/initialcounts.go` is fine).
**Reason:** The line-number anchors are a project-wide hygiene issue (the SUMMARY itself flags the pattern); a sweep across all affected files belongs in a dedicated docs phase rather than this code-review-fix pass.

### IN-03: TestPrewarmCounters_NoError lacks t.Parallel()

**File:** `internal/otel/prewarm_test.go:8-20`
**Status:** DEFERRED (mild inconsistency only; reviewer flagged INFO not WARNING).
**Reason:** The reviewer's own analysis notes "either mark all three as parallel ... or accept the inconsistency." Out of scope for the `--auto` pass; revisit on `--all` or in a parallelisation hygiene sweep.

## Post-fix verification

- `go test -race -count=1 ./internal/otel/... ./internal/sync/... ./cmd/peeringdb-plus/...` — PASS.
- `golangci-lint run ./...` — 0 issues.

---

_Fixed: 2026-04-26T23:59:00Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
