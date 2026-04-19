---
phase: 71
plan: 03
subsystem: pdbcompat
tags: [memory-budget, rfc9457, 413, config]
requires: [71-02]
provides:
  - pdbcompat.CheckBudget(count, entity, depth, budgetBytes) → (BudgetExceeded, bool)
  - pdbcompat.WriteBudgetProblem(w, instance, info) RFC 9457 413 writer
  - pdbcompat.ResponseTooLargeType constant (https://peeringdb-plus.fly.dev/errors/response-too-large)
  - pdbcompat.BudgetExceeded struct (MaxRows, BudgetBytes, EstimatedBytes, Count, Entity, Depth)
  - Config.ResponseMemoryLimit int64 loaded from PDBPLUS_RESPONSE_MEMORY_LIMIT (default 128 MiB)
affects:
  - Plan 71-04 will wire serveList() to call CheckBudget + WriteBudgetProblem on list paths
  - Plan 71-05 will attach BudgetExceeded fields to OTel span + Prometheus counter on 413 emission
tech-stack:
  added: []
  patterns:
    - "Reuse existing parseByteSize helper for PDBPLUS_RESPONSE_MEMORY_LIMIT (mandatory unit suffix; zero disables)"
    - "Hand-rolled budget problem body to carry max_rows / budget_bytes extensions without polluting httperr.ProblemDetail"
key-files:
  created:
    - internal/pdbcompat/budget.go
    - internal/pdbcompat/budget_test.go
  modified:
    - internal/config/config.go
    - internal/config/config_test.go
    - docs/CONFIGURATION.md
decisions:
  - "D-71-03-01 (executor): Hand-roll budgetProblemBody struct instead of extending httperr.ProblemDetail. Rationale: ProblemDetail hardcodes Type='about:blank' and lacks max_rows / budget_bytes extensions; keeping the custom shape in pdbcompat confines the extension to the surface that uses it. Plan 71-03 explicitly allowed this tradeoff."
  - "D-71-03-02 (executor): BudgetExceeded struct carries the raw Count / EstimatedBytes / Entity / Depth as json:\"-\" fields so Plan 04 (handler logging) and Plan 05 (OTel span attrs) can read them without a second TypicalRowBytes lookup."
metrics:
  duration_seconds: 560
  completed_date: "2026-04-19"
  tasks_completed: 2
  files_touched: 5
  commits: 3
---

# Phase 71 Plan 03: budget.go + Config.ResponseMemoryLimit + RFC 9457 413 Summary

Landed the pure-math + config + 413-writer layer for Phase 71's memory budget. No handler wiring in this plan — that is 71-04. Everything here is additive, fully unit-testable, and does not touch any existing list / streaming code path.

## What was built

### Task 1 — Config layer (commit 7debbcd)

- `Config.ResponseMemoryLimit int64` field in `internal/config/config.go` next to `SyncMemoryLimit`, `HeapWarnBytes`, `RSSWarnBytes`.
- Loaded from `PDBPLUS_RESPONSE_MEMORY_LIMIT` via the existing `parseByteSize` helper. Mandatory unit suffix (`KB`/`MB`/`GB`/`TB`, base 1024; `K`/`M`/`G`/`T` as aliases). Default `128 MiB` (134217728 bytes) per Phase 71 D-05. Literal `"0"` disables the check.
- `validate()` rejects negative values with `PDBPLUS_RESPONSE_MEMORY_LIMIT must be non-negative (0 = disabled)`.
- `internal/config/config_test.go` gained two tests:
  - `TestLoad_ResponseMemoryLimit_Default` — asserts the 128 MiB default.
  - `TestLoad_ResponseMemoryLimit_Parse` — 11 subtests covering valid units, short aliases, lowercase, explicit-zero disable, bare-number rejection, unknown unit, negative, missing prefix, non-numeric prefix.
- `docs/CONFIGURATION.md` gained a new env-var row in the HTTP Server section plus a validation-rules row.

### Task 2 — Budget math + 413 writer (commits 3740f27 RED, dc6aec2 GREEN)

- `ResponseTooLargeType` constant = `"https://peeringdb-plus.fly.dev/errors/response-too-large"` (D-04).
- `BudgetExceeded` struct with `MaxRows`, `BudgetBytes`, `EstimatedBytes`, `Count`, `Entity`, `Depth`. Only `MaxRows` and `BudgetBytes` are wire-serialized; the others carry diagnostic context for Plan 04/05 downstream consumers.
- `CheckBudget(count, entity, depth, budgetBytes) (BudgetExceeded, bool)`:
  - `budgetBytes <= 0` → `(zero, true)` (disabled).
  - Otherwise computes `estimated = count × TypicalRowBytes(entity, depth)`; returns `(zero, true)` if `estimated <= budget`, else returns a populated struct with `MaxRows = budget / perRow` (integer floor) and `false`.
  - Defensive `perRow <= 0` fallback to `defaultRowSize` — unreachable under the current map but guards against future regressions.
- `WriteBudgetProblem(w, instance, info)` emits the D-04 body verbatim:
  - Status `413 Request Entity Too Large`
  - `Content-Type: application/problem+json`
  - `X-Powered-By: PeeringDB-Plus/1.1` (matching existing response helpers)
  - No `Retry-After` — request-shape, not transient.
  - JSON body with `type`, `title`, `status`, `detail`, `instance` (omitempty), `max_rows`, `budget_bytes`.
  - `detail` format: `"Request would return ~%d rows totaling ~%d bytes; limit is %d bytes"`.
- `internal/pdbcompat/budget_test.go` has 7 subtests covering under-budget, over-budget, zero-budget disable, MaxRows computation, unknown-entity fallback, 413 body shape, and the human-readable detail string.

## Default values

| Variable | Default | Notes |
|---|---|---|
| `PDBPLUS_RESPONSE_MEMORY_LIMIT` | `128MB` (134217728 B) | D-05; Fly replica 256 MB − 80 MB Go runtime − 48 MB slack |

## Commit SHAs

| Phase | Commit | Description |
|---|---|---|
| Task 1 (config) | 7debbcd | `feat(71-03): add Config.ResponseMemoryLimit + env-var loading + docs row` |
| Task 2 (RED) | 3740f27 | `test(71-03): add failing tests for pdbcompat.CheckBudget + WriteBudgetProblem` |
| Task 2 (GREEN) | dc6aec2 | `feat(71-03): pdbcompat memory budget + RFC 9457 413` |

## Deviations from plan

### Deliberate choices documented in CONTEXT.md

- **Hand-rolled `budgetProblemBody`**: Plan 71-03 already flagged this as the expected approach — existing `httperr.ProblemDetail` hardcodes `Type="about:blank"` and lacks the `max_rows` / `budget_bytes` extensions. The plan explicitly allowed keeping the extension local to pdbcompat rather than extending the shared httperr types. No scope creep.

### Minor additions beyond the plan text

- **Validation-rules row in `docs/CONFIGURATION.md`**: The plan specified only the env-var table row in the HTTP Server section. I also added a matching row in the "Validation rules" table further down so operator error messages are documented alongside `PDBPLUS_SYNC_MEMORY_LIMIT`'s existing row. One-line addition, keeps the two memory-budget variables parallel in the docs. Rule 2 (auto-add missing documentation) — documenting only the knob without its rejection messages would have been asymmetric with `PDBPLUS_SYNC_MEMORY_LIMIT`.

- **CLAUDE.md NOT updated**: The plan text said "Plan 06 owns CLAUDE.md edits but this row can be added now to avoid churn, same shape as docs/CONFIGURATION.md. Defer to Plan 06 if CLAUDE.md has unrelated edits pending; if not, land here." Decision: defer to Plan 71-06 so the CLAUDE.md env-var table gets a single coherent update covering `PDBPLUS_RESPONSE_MEMORY_LIMIT` (71-03) alongside any other docs-close ripples. No pending CLAUDE.md churn right now, but the alternative — landing it here and then touching CLAUDE.md again in 71-06 — produces two commits for one paragraph.

### Auto-fixes applied

None. The plan executed exactly as written.

### Threat flags

None. Plan 03 does not introduce new network surface, auth paths, or schema changes at trust boundaries — it adds pure-math + config + a 413 writer that does not embed caller-controlled data in its body other than the `Count` / `EstimatedBytes` integers which are themselves computed server-side from `SELECT COUNT(*)` + a hardcoded table.

## Verification acceptance

All 7 verification items from the plan:

1. `TMPDIR=/tmp/claude-1000 go test -race ./internal/config -run TestLoad_ResponseMemoryLimit` — **PASS** (12 subtests: 1 default + 11 parse).
2. `TMPDIR=/tmp/claude-1000 go test -race ./internal/pdbcompat -run "TestCheckBudget|TestWriteBudgetProblem"` — **PASS** (7 subtests).
3. `TMPDIR=/tmp/claude-1000 go vet ./internal/config ./internal/pdbcompat` — **PASS** (clean).
4. `grep -c 'ResponseMemoryLimit' internal/config/config.go` = **4** (field declaration + godoc anchor + loader + validator; spec required ≥ 3).
5. `grep -c 'PDBPLUS_RESPONSE_MEMORY_LIMIT' docs/CONFIGURATION.md` = **2** (env-var table row + validation-rules row; spec required ≥ 1).
6. `grep -c -E 'ResponseTooLargeType|CheckBudget|WriteBudgetProblem' internal/pdbcompat/budget.go` = **8** (constant + function signatures + internal references; spec required ≥ 5).
7. No changes to `handler.go`, `registry_funcs.go`, `response.go`, `stream.go`, or `rowsize.go` — confirmed via `git show --stat dc6aec2 3740f27 7debbcd`. **PASS**.

Additional gates run beyond the plan spec:
- `TMPDIR=/tmp/claude-1000 go build ./...` — clean.
- `TMPDIR=/tmp/claude-1000 go test -race ./...` — full repo green.
- `TMPDIR=/tmp/claude-1000 golangci-lint run` — 0 issues.

## What Plan 04 needs from this one

- `pdbcompat.CheckBudget(count, entity, depth, cfg.ResponseMemoryLimit)` at the top of `serveList` after `SELECT COUNT(*)` returns `count`.
- `pdbcompat.WriteBudgetProblem(w, r.URL.Path, info)` on the `ok=false` branch.
- `cfg.ResponseMemoryLimit` must be threaded into the handler (either via package-level variable set at main or via a new field on a handler struct — Plan 04's choice).

## Self-Check: PASSED

- `internal/pdbcompat/budget.go` — FOUND (new file, 124 LOC).
- `internal/pdbcompat/budget_test.go` — FOUND (new file, 203 LOC).
- `internal/config/config.go` — FOUND (modified; +15 LOC).
- `internal/config/config_test.go` — FOUND (modified; +65 LOC).
- `docs/CONFIGURATION.md` — FOUND (modified; +2 rows).
- Commit `7debbcd` — FOUND in git log.
- Commit `3740f27` — FOUND in git log.
- Commit `dc6aec2` — FOUND in git log.
