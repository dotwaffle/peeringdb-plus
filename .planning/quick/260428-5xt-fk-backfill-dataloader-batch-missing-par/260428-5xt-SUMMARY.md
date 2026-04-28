---
phase: 260428-5xt
plan: 01
status: complete
completed: 2026-04-28
duration: ~30min
files_modified:
  - internal/peeringdb/client.go
  - internal/peeringdb/client_test.go
  - internal/sync/fk_backfill.go
  - internal/sync/fk_backfill_test.go
  - internal/sync/worker.go
tags: [sync, fk-backfill, dataloader, batching, peeringdb-client]
requirements_satisfied:
  - 5XT-01-batched-fetch
  - 5XT-02-batch-backfill
  - 5XT-03-chunk-prepass
  - 5XT-04-tests
---

# Quick Task 260428-5xt: FK Backfill Dataloader Summary

## One-liner

Replaced the per-row FK backfill HTTP pattern with a dataloader-style chunk pre-pass + batched `?since=1&id__in=<csv>` fetcher, collapsing N per-row HTTP requests into ⌈N/100⌉ per parent type per chunk. Eliminates the upstream `API_THROTTLE_REPEATED_REQUEST` exposure that bricked v1.18.2 during truncate-and-resync recovery.

## Atomic commits

| # | Hash      | Type     | Subject |
|---|-----------|----------|---------|
| 1 | `e018ba7` | feat     | `feat(peeringdb): add FetchByIDs for batched id__in fetches` |
| 2 | `e532079` | refactor | `refactor(sync): introduce fkBackfillBatch as backfill core; preserve fkBackfillParent wrapper` |
| 3 | `22d5be1` | feat     | `feat(sync): chunk pre-pass batches missing parent FK backfill per type` |
| 4 | `c54d62c` | test     | `test(sync): add batched FK backfill tests (one-request, chunks-at-100, recursive, cap, deadline)` |

Plus the pre-dispatch plan commit at the worktree base (`20f24536`).

## What changed

### `internal/peeringdb/client.go` — new entry point

`FetchByIDs(ctx, objectType, ids []int) ([]json.RawMessage, error)`:
- Empty/nil short-circuits with zero HTTP.
- Splits `ids` into chunks of `fetchByIDsChunk = 100` to keep URLs well under 8 KiB.
- Each chunk consumes ONE limiter token (250 IDs → 3 sequential requests through the limiter).
- Builds CSV via `strconv.Itoa` + `strings.Join` (no `fmt.Sprintf` allocations on the hot path).
- All-or-nothing per call (no partial returns on chunk error).
- Routes through existing `FetchRaw` → `doWithRetry` → rate-limited transport — inherits 429/WAF/5xx semantics for free.

### `internal/sync/fk_backfill.go` — new core, preserved wrapper

- New `fkBackfillBatch(ctx, tx, parentType, ids, childType) []int` is the dataloader entry point.
- Existing `fkBackfillParent(ctx, tx, childType, parentType, parentID) bool` becomes a 2-line wrapper: `fkBackfillBatch(..., []int{parentID}, childType)` then `dbHasRecord(...)`. Production callers (`worker.go:fkCheckParent`, `worker.go:nullSideFK`) compile and behave unchanged.
- `fetchSingleByID` removed (replaced by `FetchByIDs` for both single-ID and batched paths — same wire shape).
- New `prefetchMissingParentsForChunk(ctx, tx, chunkType, rows)` walks every row's required-parent FKs (per `parentFKSpec`), groups missing IDs per parent type, and issues ONE batched call per parent type. Sequential across parent types (concurrent fetches would fight the rate limiter).
- Recursive grandparent batching: each fetched row's `parentFKsOf` is walked, missing grandparents grouped by parent type and recursively batched. Bounded by per-cycle dedup cache.

### `internal/sync/worker.go` — single-line wire

```go
func (w *Worker) dispatchScratchChunk(ctx, tx, name, rows) (int, error) {
    // Quick task 260428-5xt: ...
    w.prefetchMissingParentsForChunk(ctx, tx, name, rows)
    switch name {
    ...
}
```

The pre-pass runs ONCE per chunk before the per-type fkFilter closures fire — so the existing per-row `fkCheckParent → fkBackfillParent` path becomes a no-op (dedup cache short-circuit) for any parent already loaded by the pre-pass.

## Semantic shift

`fkBackfillCount` is now bumped per-row (`len(idsToFetch)`), not per-HTTP-request. The cap (`PDBPLUS_FK_BACKFILL_MAX_PER_CYCLE`, default 200) now meaningfully limits total parent rows fetched per cycle regardless of how they're batched.

The `fk_backfill{result=hit}` metric interpretation does NOT change — both old and new code emit one hit per inserted row. Steady-state ~16 hits/cycle (production) implies the same number of row inserts; what changes is that 16 hits no longer implies 16 HTTP requests.

Documented in `fkBackfillBatch` godoc.

## Test coverage

### Existing tests (preserved unchanged)

All 7 `TestFKCheckParent_Backfill*` tests pass against the refactored `fkBackfillParent` thin wrapper:
- `TestFKCheckParent_BackfillIntegration` — single net → 1 backfill
- `TestFKCheckParent_BackfillDedup` — 2 nets sharing 1 missing parent → 1 backfill
- `TestFKCheckParent_BackfillCapZeroDisablesBackfill` — cap=0 → 0 backfills
- `TestFKCheckParent_BackfillCapHitRecordsRatelimited` — cap=1, 2 misses → 1 backfill, 1 ratelimited
- `TestFKCheckParent_BackfillFetchErrorRecordsError` — 5xx → result=error, child dropped
- `TestFKCheckParent_BackfillRecursesIntoGrandparent` — carrier→org chain → 3 dedup-bounded backfills
- `TestFKCheckParent_BackfillDeadlineFallsBackToDrop` — 1ns deadline → 0 backfills, deadline_exceeded

### New tests (5)

- `TestFKBackfill_BatchedFetch_OneRequest` — 50 nets, 50 distinct missing orgs → 1 batched HTTP, sorted ascending id__in
- `TestFKBackfill_BatchedFetch_ChunksAt100` — 250 nets across 3 chunks → 3 batched HTTP with id__in cardinalities [100,100,50]
- `TestFKBackfill_BatchedFetch_RecursiveGrandparents` — 50 carrierfacs → 50 carriers → 50 distinct orgs → exactly 2 batched HTTP (carriers + orgs)
- `TestFKBackfill_BatchedFetch_RespectsCap` — cap=10, 50 missing → 1 batched HTTP with 10 IDs, 40 ratelimited
- `TestFKBackfill_BatchedFetch_RespectsDeadline` — 1ns deadline → 0 HTTP, all 50 deadline_exceeded

## Quality gates

| Gate | Status |
|------|--------|
| `go build ./...` | clean |
| `go vet ./...` | clean |
| `golangci-lint run` | 0 issues |
| `go test -race ./...` | all packages pass (full repo) |
| `go generate ./... && git diff` | zero codegen drift |
| `govulncheck ./...` | no vulnerabilities |
| `gofmt -s -l` | clean across all 5 modified files |

## Critical invariants verified

- `fkBackfillParent` thin wrapper preserves single-row callers at `worker.go:1441` (carrier→org check via `fkCheckParent`) and `worker.go:1477` (NetworkIxLan side-FK via `nullSideFK`) — both compile unchanged with no signature change.
- Recursive grandparent backfill remains bounded by the per-cycle dedup cache (each `(type, id)` fires exactly once).
- Pre-pass + per-row fallback do NOT double-count toward `fkBackfillCap` — the dedup cache short-circuits any parent already loaded by the pre-pass.
- `pdbcompat` invariants (Phase 64 `applyStatusMatrix`, Phase 64 `StatusIn("ok","pending")` PK lookups, Phase 64 `privfield.Redact` at all 5 surfaces, Phase 71 response memory budget) — UNTOUCHED. Diff scope: `internal/peeringdb/{client.go,client_test.go}` + `internal/sync/{fk_backfill.go,fk_backfill_test.go,worker.go}` only.
- Production-validated golden path (carrier 403 / org 18985 backfill) preserved via `TestFKCheckParent_BackfillRecursesIntoGrandparent`.

## Adjacent cleanups

- `gofmt -s` applied to pre-existing whitespace drift in `internal/sync/worker.go` (`NewWorker` struct alignment + `nullSideFK` godoc list indentation) and `internal/sync/fk_backfill_test.go` (Net JSON literal alignment). No semantic change.
- `revive` rename: `cap → capLimit` constant in `TestFKBackfill_BatchedFetch_RespectsCap` (avoid shadowing builtin).
- `modernize` rewrites: single-line `min`/`max` calls in `peeringdb.FetchByIDs` chunk loop and `sync.fkBackfillBatch` cap-budget calc.

## Post-deploy verification

After deploy of any commit including this work:

1. Sync metrics dashboard:
   - `pdbplus_sync_fk_backfill_total{result="hit"}` should remain at ~16 hits/cycle steady state (per-row semantic preserved — what's inserted, not how many HTTP).
   - `pdbplus_peeringdb_requests_total` per cycle should DROP for any cycle that has multiple distinct orphans of the same parent type (chunk pre-pass collapses them into 1 request).
2. Production data sanity:
   - `carrier_count` stays at 278 (production-validated golden — `fly ssh console -a peeringdb-plus -C 'sqlite3 /litefs/peeringdb-plus.db "SELECT COUNT(*) FROM carriers;"'`).
   - `./scripts/compare-upstream-parity.sh` shows no parity gaps vs upstream.
3. Catch-up / recovery scenario (truncate then re-sync):
   - Logs should show one `fk backfill: parent inserted` per inserted row (per-row semantic preserved) — useful to grep for unexpected per-row HTTP behavior.
   - HTTP request counter per cycle (Prom: `pdbplus_peeringdb_requests_total` rate over the cycle window) should stay well under the previous "N orphans → N+~14 requests" peak — should now be `N_distinct_parent_types × ⌈avg_misses/100⌉ + ~14 bulk pages`.

## Deferred follow-ups

- **Nullable FK batching.** `Facility.campus_id`, `NetworkIxLan.{net_side_id, ix_side_id}` are intentionally NOT batched (still per-row via `fkFilter` null-on-miss). If dashboards show these as a hot per-row HTTP source after deploy, add a `nullableFKSpec` and a second pre-pass that nulls-on-miss (vs drops-on-miss). No production evidence today.
- **Batch-size histogram.** Could add `pdbplus.sync.fk_backfill_batch{parent_type}` histogram to surface batch-size distribution post-deploy. Not strictly needed — `pdbplus_peeringdb_requests_total` rate vs `pdbplus_sync_fk_backfill_total{result="hit"}` rate is enough for the catch-up signal.
- **Pre-existing gofmt drift in test files.** Some other `*_test.go` files in `internal/sync/` may still have similar Net-JSON alignment drift. Out of scope for this task.

## Self-Check: PASSED

- All 4 task commits exist in `git log` (e018ba7, e532079, 22d5be1, c54d62c) ✓
- 5 modified files match the plan's `files_modified` declaration ✓
- All 7 existing `TestFKCheckParent_Backfill*` tests pass without modification ✓
- All 5 new `TestFKBackfill_BatchedFetch_*` tests pass ✓
- `go test -race ./...` (full repo) passes ✓
- `golangci-lint run` reports 0 issues ✓
- `go generate ./... && git diff` produces no codegen drift ✓
- `govulncheck ./...` finds no vulnerabilities ✓
- `prefetchMissingParentsForChunk` called exactly once in `worker.go`, immediately before `switch name {` ✓
- Single `FetchByIDs` definition in `internal/peeringdb/client.go` ✓
- No `id__in` produced anywhere in `internal/sync/*.go` (non-test) source — only via `FetchByIDs` ✓
