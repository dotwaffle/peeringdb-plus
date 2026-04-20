---
phase: 71
plan: 04
subsystem: pdbcompat
tags: [memory-budget, streaming, 413, rfc9457, handler-wiring]
requires: [71-01, 71-02, 71-03]
provides:
  - pdbcompat.Handler.responseMemoryLimit field + NewHandler(client, budget) signature
  - pdbcompat.Registry[*].Count CountFunc sibling across 13 entity types
  - pdbcompat.iterFromSlice(rows) RowsIter adapter for materialised→streamed handoff
  - serveList pre-flight SELECT COUNT(*) → CheckBudget → 413 gate
  - serveList StreamListResponse wiring (replaces legacy WriteResponse on list path)
  - cmd/peeringdb-plus/main.go plumbs cfg.ResponseMemoryLimit into NewHandler
affects:
  - Plan 71-05 will attach per-request heap-delta telemetry to the same serveList path
  - Plan 71-06 will document the Response Memory Envelope using the wired handler
  - Phase 72 parity tests depend on the 413 body shape landed here
tech-stack:
  added: []
  patterns:
    - "Shared <x>Predicates local closure drift-proofs List/Count predicate parity across 13 entities"
    - "iterFromSlice as an incremental half-step toward cursor-based streaming — flip tc.List later without touching serveList"
    - "Budget=0 as a documented disable sentinel for local dev / tests"
key-files:
  created:
    - internal/pdbcompat/stream_integration_test.go
  modified:
    - internal/pdbcompat/registry.go
    - internal/pdbcompat/registry_funcs.go
    - internal/pdbcompat/handler.go
    - internal/pdbcompat/stream.go
    - internal/pdbcompat/stream_test.go
    - internal/pdbcompat/handler_test.go
    - internal/pdbcompat/anon_parity_test.go
    - internal/pdbcompat/bench_traversal_test.go
    - internal/pdbcompat/depth_test.go
    - internal/pdbcompat/golden_test.go
    - internal/pdbcompat/phase69_filter_test.go
    - internal/pdbcompat/registry_funcs_ordering_test.go
    - internal/pdbcompat/traversal_e2e_test.go
    - internal/pdbcompat/testdata/golden/{13 entity types}/list.json
    - cmd/peeringdb-plus/main.go
    - cmd/peeringdb-plus/e2e_privacy_test.go
    - cmd/peeringdb-plus/ordering_cross_surface_e2e_test.go
    - cmd/peeringdb-plus/privacy_surfaces_test.go
    - internal/sync/nokey_sync_test.go
decisions:
  - "D-71-04-01 (executor): iterFromSlice promoted from stream_test.go helper to stream.go production code. Eliminates duplicate declarations; sets up the future flip to pull-iterator tc.List without touching handler or tests."
  - "D-71-04-02 (executor): servedRowCount helper in registry_funcs.go centralises the post-Offset/Limit arithmetic so every CountFunc computes the same served-row count the List closure will actually emit. Matches CheckBudget's math (count × typicalRowBytes)."
  - "D-71-04-03 (executor): NewHandler constructor signature flipped from (client) to (client, budget). Chose constructor over setter (plan gave the choice) for fail-fast semantics — every test-helper caller explicitly opts into a budget or explicitly passes 0 to disable. All 10 call sites (7 internal + 3 cmd) updated in the same commit."
  - "D-71-04-04 (executor): Golden list fixtures for all 13 entity types regenerated to drop the trailing newline — that is the intentional D-07 byte divergence between WriteResponse (json.NewEncoder appends \\n) and StreamListResponse (closes with ]}). Detail fixtures are unchanged because serveDetail still uses WriteResponse per D-07 list-only scope."
  - "D-71-04-05 (executor): Budget check skipped entirely when emptyResult=true (Phase 69 IN-02 short-circuit). The result is known-empty; counting 0 rows and running CheckBudget(0, ...) would always pass but wastes a SELECT COUNT(*) round-trip. TestServeList_EmptyResultShortCircuitsBeforeBudget pins this behaviour."
metrics:
  duration_seconds: 423
  completed_date: "2026-04-19"
  tasks_completed: 2
  files_touched: 30
  commits: 2
---

# Phase 71 Plan 04: Pre-flight Budget + Streaming Wired into serveList Summary

Wired the Plan 01 streaming writer and Plan 03 budget check into the pdbcompat list handler. Over-budget list requests now 413 with the RFC 9457 problem-detail body BEFORE any rows are fetched; under-budget requests stream through the token writer instead of materialising the full envelope. 128 MiB default budget loads from Config in `cmd/peeringdb-plus/main.go`; tests opt into or out of the budget explicitly via the new `NewHandler(client, budget)` signature.

## What was built

### Task 1 — Registry refactor (commit 27ba127)

- `registry.go`: new `CountFunc` type + `TypeConfig.Count` field. Signatures:
  ```go
  type CountFunc func(ctx context.Context, client *ent.Client, opts QueryOptions) (int, error)
  ```
- `registry_funcs.go`: 13 shared `<x>Predicates` local closures extracted from the existing List bodies and reused by BOTH the List closure and the new Count closure. Flipped `setFuncs` to a 3-arg signature `(name, list, count, get)`.
- `servedRowCount(total, opts)` helper centralises the post-Offset/Limit math so the pre-flight estimate matches the actual served row count (not the raw total).
- Grep invariants held:
  - `applyStatusMatrix` appears 13x (one per predicate helper — down from 13x inline in the old List bodies).
  - `opts.EmptyResult` appears 26x (13 List + 13 Count short-circuits).

### Task 2 — Handler wiring + tests + main.go plumbing (commit 28408e1)

- `handler.go`:
  - `Handler` struct gains `responseMemoryLimit int64`.
  - `NewHandler(client, responseMemoryLimit)` — constructor now takes the budget (0 disables).
  - `serveList` runs `tc.Count(ctx, client, opts)` → `CheckBudget(count, tc.Name, 0, h.responseMemoryLimit)`. On breach: `slog.WarnContext` with typed attrs (endpoint / type / count / estimated_bytes / budget_bytes / max_rows) then `WriteBudgetProblem` + return (413; no rows fetched). Under budget: existing `tc.List` + optional field projection, then `StreamListResponse(ctx, w, struct{}{}, iterFromSlice(results))` replaces the legacy `WriteResponse`.
  - `emptyResult` (Phase 69 IN-02) bypasses both the Count query and the CheckBudget gate — result is known-empty so counting is wasted work.
  - `serveDetail` is unchanged (D-07 scope limits streaming to list paths only). Detail still uses `WriteResponse`.
- `stream.go`: `iterFromSlice(rows []any) RowsIter` helper — produces a pull-iterator from an already-materialised slice. Production code now (was previously a test-only helper in `stream_test.go`; the test-file declaration was removed to avoid a redeclaration compile error).
- `cmd/peeringdb-plus/main.go`: `pdbcompat.NewHandler(entClient, cfg.ResponseMemoryLimit)`.
- Test-helper churn: 10 call sites to `pdbcompat.NewHandler(client)` flipped to `pdbcompat.NewHandler(client, 0)` to preserve their original scope (not memory-guardrail tests). Each edit carries a one-line comment documenting why the budget is disabled.
- `testdata/golden/*/list.json` regenerated (13 entity list fixtures) — the trailing newline dropped because `StreamListResponse` closes with `]}` while the old `json.NewEncoder.Encode` appended `\n`. Detail fixtures unchanged (serveDetail still uses WriteResponse per D-07).

### Integration tests (new: `internal/pdbcompat/stream_integration_test.go`)

Four tests exercising the wired path via `httptest.NewServer(mux)`:

1. **TestServeList_OverBudget413** — seed.Full + 1-byte budget; GET `/api/net` returns 413, Content-Type `application/problem+json`, body.type = `ResponseTooLargeType`, body.status = 413, body.budget_bytes = 1, body.max_rows >= 0, body.instance = `/api/net`. Asserts the envelope (`"data":[`) does NOT appear in the body — proof no rows leaked from a half-started stream.
2. **TestServeList_UnderBudgetStreams** — seed.Full + 10 MiB budget; GET `/api/net` returns 200, Content-Type `application/json`, envelope decodes with non-empty data array where every row decodes as a JSON object (catches separator-logic regressions).
3. **TestServeList_ByteExactParityWithLegacy** — seed.Full + budget=0 (disabled); GET `/api/org`. Asserts the streamed body decodes to the same JSON value as the legacy WriteResponse envelope would have produced, and that the byte-level shape is exactly the legacy form minus the trailing `\n` (the intentional D-07 one-byte divergence). If any future change appends a newline in the streaming path, this test fails loudly.
4. **TestServeList_EmptyResultShortCircuitsBeforeBudget** — seed.Full + 1-byte budget; GET `/api/net?asn__in=` returns 200 with body `{"meta":{},"data":[]}`. Proves the Phase 69 IN-02 empty-result path bypasses the budget check entirely (would otherwise 413 on a 1-byte budget).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Redeclared iterFromSlice removed from stream_test.go**
- **Found during:** Task 2 first test run — compile error "iterFromSlice redeclared in this block" between stream.go:119 and stream_test.go:16.
- **Issue:** Plan 01 (71-01) had created `iterFromSlice` as a private test-only helper in `stream_test.go`. Plan 71-04 Task 2 instructed adding the same helper to `stream.go` as production code (called from serveList). Both declarations collided.
- **Fix:** Removed the stream_test.go declaration (left a short comment pointing at stream.go). The existing stream_test.go tests continue to consume the new production iterFromSlice symbol — no test logic changed.
- **Files modified:** `internal/pdbcompat/stream_test.go`
- **Commit:** 28408e1 (folded into Task 2)

**2. [Rule 3 - Blocking] 13 golden list fixtures regenerated for trailing-newline divergence**
- **Found during:** Task 2 first test run after wiring StreamListResponse.
- **Issue:** `TestGoldenFiles/*/list` subtests (13 failures, one per entity) compared stored fixtures against the new streamed output. The ONLY diff was a trailing `\n` in the stored file that StreamListResponse does not emit (it closes with `]}`, while the old WriteResponse path used `json.NewEncoder.Encode` which appends `\n`).
- **Fix:** Ran `go test -run TestGoldenFiles -update` to regenerate all 13 list fixtures. Verified the diff is exactly one trailing newline per file (confirmed via `git diff` on org/list.json — the only change is `\ No newline at end of file`). Detail and depth fixtures were NOT modified — they still route through `serveDetail` → `WriteResponse` per D-07 list-only scope.
- **Files modified:** `internal/pdbcompat/testdata/golden/{campus,carrier,carrierfac,fac,ix,ixfac,ixlan,ixpfx,net,netfac,netixlan,org,poc}/list.json`
- **Commit:** 28408e1 (folded into Task 2)

### Test-Helper Signature Churn

Ten existing callers of `pdbcompat.NewHandler(client)` were updated to `pdbcompat.NewHandler(client, 0)` — explicitly disabling the budget for tests whose scope is not memory guardrails. This matches the plan's preferred constructor-arg approach (fail-fast, not setter). Each edit includes a one-line comment explaining why the budget is disabled for that test's scope. None of these tests assert budget behaviour; they all continue to pass with budget=0 indistinguishable from the pre-plan handler behaviour.

Callers updated:
- `internal/pdbcompat/handler_test.go` (2 call sites)
- `internal/pdbcompat/anon_parity_test.go`
- `internal/pdbcompat/bench_traversal_test.go`
- `internal/pdbcompat/depth_test.go` (2 call sites)
- `internal/pdbcompat/golden_test.go`
- `internal/pdbcompat/phase69_filter_test.go`
- `internal/pdbcompat/registry_funcs_ordering_test.go`
- `internal/pdbcompat/traversal_e2e_test.go`
- `cmd/peeringdb-plus/e2e_privacy_test.go`
- `cmd/peeringdb-plus/ordering_cross_surface_e2e_test.go`
- `cmd/peeringdb-plus/privacy_surfaces_test.go`
- `internal/sync/nokey_sync_test.go`

## Trailing-newline quirk (D-07)

`WriteResponse` uses `json.NewEncoder(w).Encode(envelope{...})` which appends a `\n` after the top-level object. `StreamListResponse` closes with `]}` and does NOT append a newline — the bytes on the wire are exactly one byte shorter. This is documented in the `TestServeList_ByteExactParityWithLegacy` test comment (with the one-byte divergence explicitly asserted) and is the reason all 13 list golden fixtures were regenerated. Detail fixtures retain the trailing newline because `serveDetail` still uses `WriteResponse` (D-07 list-only scope).

## Verification

- `TMPDIR=/tmp/claude-1000 go build ./...` — clean
- `TMPDIR=/tmp/claude-1000 go test -race ./... -count=1` — all packages green (pdbcompat: 8.1s, cmd/peeringdb-plus: 4.3s, sync: 14.9s, etc.)
- `TMPDIR=/tmp/claude-1000 go vet ./...` — clean
- `TMPDIR=/tmp/claude-1000 golangci-lint run` — 0 issues
- `grep -c 'applyStatusMatrix' internal/pdbcompat/registry_funcs.go` = 13 (Phase 68 invariant preserved)
- `grep -c 'opts.EmptyResult' internal/pdbcompat/registry_funcs.go` = 26 (Phase 69 IN-02 guards doubled for Count closures)
- `grep -c 'CheckBudget' internal/pdbcompat/handler.go` = 4 (1 call site + 3 documentation references)
- `grep -c 'StreamListResponse' internal/pdbcompat/handler.go` = 2 (1 call site + 1 comment)
- `grep -c 'WriteBudgetProblem' internal/pdbcompat/handler.go` = 1 (call site)
- `grep -c 'WriteResponse(w' internal/pdbcompat/handler.go` = 1 (in serveDetail — intentional per D-07)
- `grep -c 'ResponseMemoryLimit' cmd/peeringdb-plus/main.go` = 1 (`cfg.ResponseMemoryLimit` passed to `NewHandler`)

## Commits

- 27ba127 — refactor(71-04): registry gains CountFunc sibling via shared predicate helper
- 28408e1 — feat(71-04): wire pre-flight budget + streaming into serveList

## Self-Check: PASSED

- Created file `internal/pdbcompat/stream_integration_test.go`: FOUND
- Commit 27ba127: FOUND in git log
- Commit 28408e1: FOUND in git log
- All 4 new integration tests pass individually and as a group
- Full `go test -race ./...` green across the module
- Grep invariants all match expected counts (13 / 26 / 1 / 1)
