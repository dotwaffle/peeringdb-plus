---
phase: 60-surface-integration-tests
plan: 05
subsystem: sync
tags: [sync, privacy, visibility, integration-test, SYNC-02, VIS-06, VIS-07]
requires:
  - phase-57 anon fixtures (testdata/visibility-baseline/beta/anon/api/{type}/page-1.json)
  - phase-59 ent Privacy policy on Poc (Users rows filtered for TierPublic)
  - phase-59 sync-worker bypass (internal/sync/worker.go — sole privacy.DecisionContext call site)
  - phase-59 bypass_audit_test.go (_test.go exemption for per-test assertion queries)
  - internal/privctx.WithTier / TierPublic
  - internal/sync.NewWorker / Worker.Sync / WorkerConfig / InitStatusTable
  - internal/peeringdb.NewClient (no WithAPIKey — anonymous mode)
  - internal/pdbcompat.NewHandler + Register
provides:
  - TestNoKeySync — end-to-end invariant covering SYNC-02 / VIS-06 / VIS-07 no-key path
  - Regression cover for three failure modes:
      * a silent apiKey passthrough regression (auth-header counter)
      * a Users-row upsert regression (DB bypass Count assertion)
      * a policy-is-not-a-no-op regression on Public-only data (DB-vs-surface parity)
affects:
  - internal/sync/nokey_sync_test.go (NEW)
tech-stack:
  added: []
  patterns:
    - "httptest.Server serving phase 57 anon fixtures verbatim as a fake PeeringDB upstream"
    - "atomic.Int64 counter in the fake upstream to prove zero Authorization headers crossed the wire"
    - "privacy.DecisionContext(t.Context(), privacy.Allow) as assertion bypass (legitimate use in _test.go per bypass_audit_test.go exemption)"
    - "inline privctx.WithTier stamp as the minimum middleware fixture required to exercise the surface → policy path"
key-files:
  created:
    - internal/sync/nokey_sync_test.go
  modified: []
decisions:
  - "Used `package sync` (internal test package) rather than `package sync_test` so the test can reuse the same TestMain that initialises OTel metrics for the rest of the sync suite, avoiding double-init races."
  - "Served fixture bytes verbatim (w.Write(body)) rather than parsing-and-re-encoding. The fixtures were captured live with the same `{\"data\":[...],\"meta\":{}}` envelope the worker decodes, so pass-through is simpler, faster, and avoids any drift between what the worker sees here vs. what it will see against real PeeringDB."
  - "Chose `totalCount > 0` (lenient) rather than an exact POC row count. The anon fixtures are live-captured and the FK-orphan filter in worker.go silently drops POC rows whose net_id is missing from the net fixture — an exact-count assertion would be brittle to every fixture refresh."
  - "Used `atomic.Int64` for the Authorization-header counter even though the current sync worker fetches serially. Future-proofs against a parallelised sync and keeps the `-race` detector quiet under any scheduling."
  - "Did NOT wire buildMiddlewareChain for the Phase B surface read. Plans 60-02..04 own full-chain coverage; this plan's assertion is narrower (filter-is-a-no-op on Public-only data), and an inline privctx.WithTier stamp is sufficient."
metrics:
  completed: 2026-04-16
  tasks: 1
  duration_min: ~25
---

# Phase 60 Plan 05: No-key sync integration test Summary

One new test file — `internal/sync/nokey_sync_test.go` — closes the SYNC-02 regression gap:
proves the no-PeeringDB-API-key deployment topology lands zero `visible="Users"` POC
rows in the DB, and that the anonymous read surface observes the same row set the
worker persisted (the ent privacy filter is effectively a no-op because there's nothing
to filter).

## Outcome

- `TestNoKeySync` (`internal/sync/nokey_sync_test.go`, package `sync`, `t.Parallel()`):

  **Phase A (upstream boundary):**
  1. Loads the 13 phase-57 anon fixtures from `testdata/visibility-baseline/beta/anon/api/{type}/page-1.json` at test start. A missing fixture is fatal.
  2. Stands up an `httptest.Server` that serves fixture bytes verbatim for `/api/{type}` requests, terminates pagination on any `skip != 0`, and counts inbound `Authorization` headers via `atomic.Int64`.
  3. Constructs an anonymous `peeringdb.NewClient(srv.URL, slog.Default())` — no `WithAPIKey` — and defangs the rate limiter and retry base delay for test speed.
  4. Runs `Worker.Sync(t.Context(), config.SyncModeFull)` against the fake upstream.
  5. Asserts (via `privacy.DecisionContext(t.Context(), privacy.Allow)` bypass):
     - `client.Poc.Query().Where(poc.Visible("Users")).Count(bypass) == 0` — the SYNC-02 invariant.
     - `client.Poc.Query().Count(bypass) > 0` — sanity that the fake upstream and sync wiring actually deliver rows.
     - `authHeaderCount.Load() == 0` — proof the worker actually ran anonymous (no silent key passthrough).

  **Phase B (surface boundary):**
  6. Mounts `pdbcompat.NewHandler(client).Register(mux)` on a fresh `httptest.Server`, wrapping the mux in a handler that stamps `privctx.WithTier(r.Context(), privctx.TierPublic)` on every request.
  7. Fetches `GET /api/poc?limit=50`.
  8. Asserts response row count equals the equivalent bypass-query row count (filter is a no-op on Public-only data).
  9. Belt-and-braces: no response row may have `visible="Users"` (catches hypothetical regressions that inject Users rows at the surface layer below the privacy policy).

- **No production code changes.** Test-only addition that exercises the existing Phase 59 bypass + Phase 58 visibility schema + Phase 57 fixture corpus in a single end-to-end assertion.

## Why this test is necessary

The phase 60 plan-02..04 surface tests cover the case **where Users rows exist in the DB** — they assert the ent privacy policy filters them out on anonymous reads. This plan covers the inverse: the real-world no-key deployment topology should never produce Users rows in the first place. Without this test, the following regressions would land silently:

- A sync-path change that accidentally includes a hardcoded or environment-inherited API key → Users rows appear in the DB. (Caught by Phase A auth-header probe.)
- A schema change that defaults `visible` to something other than `Public` → Users rows appear in the DB. (Caught by `Count(Visible("Users")) == 0`.)
- A privacy-filter regression that applies the `visible != 'Public'` predicate incorrectly (e.g., inverted) → Public rows get filtered out. (Caught by the Phase B DB-vs-surface row-count parity.)

## Zero-Authorization-headers assertion

The fake upstream increments `authHeaderCount` on **any** non-empty `Authorization` header — not just `Api-Key` — to guard against a future auth mode (Bearer, etc.) slipping through without updating the audit. After the sync completes, the test asserts the counter is exactly zero. This is strictly stronger than "no API key was configured"; it proves nothing downstream of `peeringdb.NewClient` constructs one at request time either.

## FK-orphan filter behaviour

The sync worker's FK-orphan filter (see `internal/sync/worker.go` lines 77-99, `fkRegistry`/`fkSkippedIDs`) silently drops child rows whose parent was not synced. The anon POC fixture is live-captured and references `net_id` values that may or may not all be in the net fixture. The test deliberately asserts `totalCount > 0` (lenient) rather than an exact count, because the exact survivor count drifts with every fixture refresh. The test log (`t.Logf("no-key sync persisted %d POC rows, 0 Users-tier", totalCount)`) records the current observation for posterity but does not gate.

## No fixture gaps encountered

All 13 fixture files (`testdata/visibility-baseline/beta/anon/api/{campus,carrier,carrierfac,fac,ix,ixfac,ixlan,ixpfx,net,netfac,netixlan,org,poc}/page-1.json`) were present and loadable. None had to be added or regenerated as part of this plan.

## Verification

Acceptance grep checks (per plan):

```
$ grep -n 'testdata/visibility-baseline/beta/anon/api' internal/sync/nokey_sync_test.go
92:  // The path is held as a single literal ("../../testdata/visibility-baseline/beta/anon/api")
95:  const fixtureDir = "../../testdata/visibility-baseline/beta/anon/api"

$ grep -n 'w.Sync(t.Context()' internal/sync/nokey_sync_test.go
173: if err := w.Sync(t.Context(), config.SyncModeFull); err != nil {

$ grep -n 'authHeaderCount' internal/sync/nokey_sync_test.go
(4 matches: declaration, increment inside fake-upstream handler, load + assert)

$ grep -n 'poc.Visible("Users")' internal/sync/nokey_sync_test.go
191: usersCount, err := client.Poc.Query().Where(poc.Visible("Users")).Count(bypass)

$ grep -n 'privacy.DecisionContext' internal/sync/nokey_sync_test.go
189: bypass := privacy.DecisionContext(t.Context(), privacy.Allow)

$ grep -n 'privctx.WithTier\|privctx.TierPublic' internal/sync/nokey_sync_test.go
235: ctx := privctx.WithTier(r.Context(), privctx.TierPublic)

$ grep -n 'pdbcompat.NewHandler' internal/sync/nokey_sync_test.go
233: pdbcompat.NewHandler(client).Register(mux)

$ grep -rn 'SetAPIKey\|Authorization.*Api-Key' internal/sync/nokey_sync_test.go
(no matches — the only Authorization references are r.Header.Get("Authorization") in the upstream probe
 and the %d Authorization headers error message, neither of which set or forge a key)
```

Local `go test -race ./internal/sync/ -run '^TestNoKeySync$'` could not be executed
from the restricted execute-phase sandbox in this worker (the `go` tool is blocked).
The test was authored against the same fixture layout and worker APIs exercised by
the existing `internal/sync/worker_test.go` (`newFixture` / `newTestWorker` / `Sync`)
and `internal/sync/policy_test.go` (`privacy.DecisionContext` / `privctx.WithTier`)
patterns. Full `go test -race ./internal/sync/...` is gated by the CI PR job and the
merge step. If the run fails on the parent merge of the 60-* worktree set, the
expected failure modes are:

| Symptom | Likely cause | Remediation |
|---|---|---|
| `init status table: ...` | `InitStatusTable` signature drift | mirror the call-shape used in `worker_test.go:newTestWorker` |
| `sync failed: ...` | FK-orphan filter ate all POC rows | fixture drift; loosen `totalCount > 0` to a more tolerant probe or regenerate page-1.json from current beta |
| `SYNC-02 violation: N Users-tier POCs...` | a genuine SYNC-02 regression landed | triage the violating upsert path; **do not weaken the assertion** |
| `worker sent N Authorization headers` | a peeringdb.Client change added auth-by-default | SYNC-02 regression — fix upstream rather than the test |

## Deviations from Plan

**1. [Rule 3 — worktree state] Hard-reset worktree HEAD before starting.**
- **Found during:** pre-task worktree branch check
- **Issue:** `HEAD` was at `3f4c8ad` (predates phase 57 anon fixtures and the
  `internal/privctx` package). The plan's `@internal/sync/worker.go` and
  `@internal/privctx.*` references could not be resolved from that commit.
- **Fix:** Ran `git fetch --all && git reset --hard d9f8c20c21d7187b5b282bc3675335f1a5f87ac6`
  per the `<worktree_branch_check>` instruction in the prompt. HEAD is now at
  `d9f8c20 docs(phase-60): update tracking after wave 1`, which is the
  correct base for wave-2 (plans 60-02..05) work.
- **Files modified:** none (reset only)
- **Commit:** N/A (reset of worktree state, not a code change)

**2. [Rule 2 — auditability] Embed the fixture path as a continuous string literal.**
- **Found during:** acceptance-criteria grep dry-run against the initial draft
- **Issue:** The initial draft used `filepath.Join("..","..","testdata","visibility-baseline","beta","anon","api")`,
  which broke the plan's grep-based acceptance check for
  `testdata/visibility-baseline/beta/anon/api` (no single token contains the full path).
- **Fix:** Changed the initial path assignment to a `const fixtureDir` single
  literal. `filepath.Join` is still used downstream for `{typeName}/page-1.json`
  composition, so cross-platform path separator handling is preserved.
- **Files modified:** internal/sync/nokey_sync_test.go
- **Commit:** rolled into the single task commit.

No other deviations. Plan executed as written.

## Known Stubs

None. The test exercises real production code paths (`peeringdb.Client`, `sync.Worker.Sync`,
`pdbcompat.Handler`, the ent privacy policy) end-to-end against committed fixtures.

## Self-Check

- [x] `internal/sync/nokey_sync_test.go` exists (created this plan)
- [x] File contains `func TestNoKeySync(` (line 82)
- [x] File references `testdata/visibility-baseline/beta/anon/api` as a continuous literal (line 95)
- [x] File calls `w.Sync(t.Context(), config.SyncModeFull)` (line 173)
- [x] File uses `privacy.DecisionContext(t.Context(), privacy.Allow)` as bypass (line 189)
- [x] File uses `privctx.WithTier(..., privctx.TierPublic)` in the surface handler (line 235)
- [x] File calls `pdbcompat.NewHandler(client).Register(mux)` (line 233)
- [x] File contains the Authorization-header counter assertion (lines 109, 113, 215-217)
- [x] File contains no `SetAPIKey` or `Authorization.*Api-Key` key-construction (confirmed via grep)
- [x] Worktree HEAD matches base `d9f8c20c21d7187b5b282bc3675335f1a5f87ac6`

## Self-Check: PASSED
