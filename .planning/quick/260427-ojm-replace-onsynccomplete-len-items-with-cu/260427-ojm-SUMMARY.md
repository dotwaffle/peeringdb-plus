---
quick_task: 260427-ojm
title: Replace OnSyncComplete len(items) with InitialObjectCounts
type: bugfix
tags: [observability, sync, privacy, gauge-cache]
completed: 2026-04-27
duration_minutes: 30
commits:
  - hash: fed800b
    type: test
    msg: "add failing tests for live-count OnSyncComplete"
  - hash: 363b7f0
    type: fix
    msg: "refresh gauge cache via InitialObjectCounts on sync"
tech-stack:
  added: []
  patterns: [tier-elevation-via-privctx, fail-soft-cache-refresh]
key-files:
  created:
    - cmd/peeringdb-plus/onsynccomplete_test.go
  modified:
    - internal/sync/initialcounts.go
    - internal/sync/initialcounts_test.go
    - internal/sync/worker.go
    - cmd/peeringdb-plus/main.go
decisions:
  - 'Drop counts arg from OnSyncComplete (was always the wrong value to feed into the gauge cache).'
  - 'Elevate to TierUsers inside InitialObjectCounts (not at every caller) — defense in depth.'
  - 'Fail-soft on cache refresh error: keep last-known-good total, do NOT blank to zeros.'
metrics:
  task_count: 3
  file_count: 4
---

# Quick task 260427-ojm: replace OnSyncComplete len(items) with live row counts

**One-liner:** Fix the `pdbplus_data_type_count` gauge "doubling-halving" by making OnSyncComplete refresh from a privacy-elevated `Count(ctx)` per type instead of consuming per-cycle upsert deltas.

## Problem

The `pdbplus_data_type_count` gauge cache (atomic.Pointer in `cmd/peeringdb-plus/main.go`) was primed by two paths that disagreed on what the value meant:

1. **Startup** — `pdbsync.InitialObjectCounts(ctx, entClient)` ran a one-shot `Count(ctx)` per entity table. This used a bare ctx, which the Poc privacy policy treats as `TierPublic` — so rows with `visible="Users"` were filtered out. Poc came back undercounted by however many private contacts existed.
2. **Per sync cycle** — `OnSyncComplete(counts map[string]int, syncTime time.Time)` received a map of "rows upserted this cycle" (`len(items)` per type) and stuffed it into the same cache. For incremental syncs this was a delta, not a total. For full syncs the upsert path was a raw count (no privacy filter), so Poc came back at full count.

Net effect: the gauge value flipped between filtered (after startup) and raw (after each full sync), and dropped to a delta after each incremental sync. From the dashboard this looked like Poc count "doubling and halving" on every sync cycle.

## Fix

Three changes:

1. **`internal/sync/initialcounts.go`** — wrap the input ctx with `privctx.WithTier(ctx, privctx.TierUsers)` at the top of `InitialObjectCounts` so every `Count(...)` call sees the elevated tier. Bypass-audit invariant preserved (uses the supported `privctx.WithTier` channel, not `privacy.DecisionContext(..., privacy.Allow)` which is restricted to `worker.go`).

2. **`internal/sync/worker.go`** — change `WorkerConfig.OnSyncComplete` from `func(counts map[string]int, syncTime time.Time)` to `func(ctx context.Context, syncTime time.Time)`. The per-cycle upsert-count map was always the wrong value to feed into the gauge cache; it stays alive inside `recordSuccess` for sync-status persistence and structured logging (`sumCounts`).

3. **`cmd/peeringdb-plus/main.go`** — rewrite the callback to:
   - Call `pdbsync.InitialObjectCounts(ctx, entClient)` against the worker's ctx (privacy elevation is now baked into the helper).
   - On error: log a Warn and SKIP the cache update — keeps the last-known-good total visible on the dashboard rather than blanking it to zeros that would trigger ops alerts.
   - On success: store into the atomic cache.
   - Always: `cachingState.UpdateETag(syncTime)` (PERF-07 — decoupled from gauge cache freshness).

## TDD gate sequence

Plan was `tdd="true"`. Two commits in the canonical order:

- **RED** (`fed800b` — `test(260427-ojm)`): assertion that `InitialObjectCounts` returns `counts["poc"] == 3` (1 Public + 2 Users via `seed.Full`) and `reflect`-based signature lock for `WorkerConfig.OnSyncComplete`. Both fail at HEAD: `counts[poc]=1` and field type is `func(map[string]int, time.Time)`.
- **GREEN** (`363b7f0` — `fix(260427-ojm)`): implementation. Both tests pass.

No REFACTOR commit needed — the implementation lands clean.

## Verification gates

All four gates from the plan constraints, all green:

| Gate | Result |
|---|---|
| `go build ./...` | clean |
| `go vet ./...` | clean |
| `golangci-lint run ./...` | 0 issues |
| `go test -race ./internal/sync/... ./internal/otel/... ./cmd/peeringdb-plus/...` | all pass |

`TestSyncBypass_SingleCallSite` (the production-code bypass-audit invariant) passes unchanged — the fix used `privctx.WithTier`, not a new `privacy.DecisionContext(..., privacy.Allow)` call site.

## Deviations from plan

None. Two trivial mid-execution adjustments:

1. The signature-lock test originally used named `(ctx, syncTime)` parameters in the `reflect.TypeOf(func(...) {})` literal; revive flagged them as unused. Renamed to `_, _` per the standard idiom for type-only function literals. Cosmetic only — same `reflect.Type` produced.
2. Plan task 1 prompt referenced `PreparseQueryAllow` constraint phrasing ("the existing InitialObjectCounts already uses privctx.TierUsers"); at HEAD it did not. Treated as aspirational and made it true by adding the `privctx.WithTier` line in the GREEN phase — that was the intent of the constraint as evidenced by the bypass-audit invariant.

## Auth gates

None.

## Known stubs

None.

## Threat flags

None — no new network endpoints, no auth/file/schema surface changes. Privacy elevation lives within an internal helper that only counts rows; it does not return row data.

## Self-Check: PASSED

Files claimed:

- `cmd/peeringdb-plus/onsynccomplete_test.go` — created (verified `[ -f ]`).
- `internal/sync/initialcounts.go`, `internal/sync/initialcounts_test.go`, `internal/sync/worker.go`, `cmd/peeringdb-plus/main.go` — modified (verified via `git diff --name-only fed800b~1..HEAD`).

Commits claimed:

- `fed800b` — present (verified via `git log --oneline -5`).
- `363b7f0` — present (verified via `git log --oneline -5`).
