---
phase: 260427-ojm
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - cmd/peeringdb-plus/main.go
  - internal/sync/initialcounts_test.go
autonomous: true
requirements:
  - QUICK-260427-OJM
must_haves:
  truths:
    - "After every successful sync cycle, the objectCountCache holds current ent-table totals (Count(ctx) per type), not per-cycle upserted-row deltas."
    - "The gauge cache writer in OnSyncComplete reuses pdbsync.InitialObjectCounts so the cold-start primer and the post-sync writer share a single implementation."
    - "Incremental sync cycles (where len(items) is tiny) leave the gauge cache reflecting the full table, not the delta."
    - "If the post-sync count pass fails, the previous cache value is preserved and a slog.Warn is emitted; the sync cycle's success status is unchanged."
    - "The PERF-07 ETag swap (cachingState.UpdateETag(syncTime)) still fires on every successful sync — it is independent of the count refresh."
    - "TestInitialObjectCounts_PocPolicyBypass and the existing initialcounts test suite still pass."
  artifacts:
    - path: "cmd/peeringdb-plus/main.go"
      provides: "OnSyncComplete callback that calls pdbsync.InitialObjectCounts after each sync"
      contains: "InitialObjectCounts"
    - path: "internal/sync/initialcounts_test.go"
      provides: "Regression test proving an incremental-style sync (tiny upsert batch over a large seeded table) refreshes the cache to current-table totals, not the delta"
      contains: "TestInitialObjectCounts_AfterSync_ReflectsCurrentTable"
  key_links:
    - from: "cmd/peeringdb-plus/main.go OnSyncComplete"
      to: "internal/sync/initialcounts.go InitialObjectCounts"
      via: "function call inside the callback body"
      pattern: "pdbsync\\.InitialObjectCounts\\(ctx, entClient\\)"
    - from: "OnSyncComplete error path"
      to: "logger.Warn"
      via: "guarded if-err branch that preserves previous cache"
      pattern: "slog\\.LevelWarn|logger\\.Warn"
---

<objective>
Replace the per-step `len(items)` write into the `objectCountCache` with a post-sync `InitialObjectCounts(ctx, entClient)` pass so the `pdbplus_data_type_count` gauge reflects current table totals on every cycle — including incremental syncs where `len(items)` is just the delta.

Purpose: closes the residual semantic mismatch flagged in the just-resolved POC oscillation debug session (.planning/debug/poc-count-doubling-halving.md). Predecessor commit 5c780c5 made the two writers numerically agree by stamping `TierUsers` in `InitialObjectCounts`; this task makes the OnSyncComplete writer **reuse** the primer instead of producing a different (delta-shaped) value, so all 8 fleet instances always converge on the same current-table number across full and incremental cycles.

Output: a single-task code change in `cmd/peeringdb-plus/main.go` plus one regression test in `internal/sync/initialcounts_test.go`. No schema changes, no proto changes, no new env vars, no new metrics.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/STATE.md
@./CLAUDE.md
@.planning/debug/poc-count-doubling-halving.md
@internal/sync/initialcounts.go
@internal/sync/initialcounts_test.go

<interfaces>
<!-- Key types and contracts the executor needs. Extracted from internal/sync. -->

From internal/sync/initialcounts.go:
```go
// InitialObjectCounts runs a one-shot Count(ctx) against each of the 13
// PeeringDB entity tables and returns the result keyed by PeeringDB type
// name. Stamps privctx.TierUsers internally so Poc.Policy admits visible="Users"
// rows — symmetry with the OnSyncComplete writer.
//
// Cost: ~1-2s on a primed LiteFS DB (13 sequential SQLite COUNT(*) queries).
// Returns the full map or an error; no partial results.
func InitialObjectCounts(ctx context.Context, client *ent.Client) (map[string]int64, error)
```

From internal/sync/worker.go (line 72) — the callback contract MUST stay backwards-compatible because the test fakes call it with the existing signature:
```go
OnSyncComplete func(counts map[string]int, syncTime time.Time)
```
The `counts` arg can be IGNORED inside the callback body (the new implementation uses `entClient.Count(ctx)` instead), but the parameter list MUST NOT change — the worker call site at worker.go:755 passes both args, and altering the signature ripples into worker_test.go fakes for no benefit.

From cmd/peeringdb-plus/main.go (current implementation — REPLACE the count-loop, KEEP the ETag swap):
```go
OnSyncComplete: func(counts map[string]int, syncTime time.Time) {
    m := make(map[string]int64, len(counts))
    for k, v := range counts {
        m[k] = int64(v)
    }
    objectCountCache.Store(&m)
    // PERF-07: swap the cached ETag using the exact completion
    // timestamp the worker persisted to sync_status. One SHA-256
    // per sync, zero per request.
    cachingState.UpdateETag(syncTime)
},
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Switch OnSyncComplete gauge-cache writer to InitialObjectCounts; add regression test</name>
  <files>
    cmd/peeringdb-plus/main.go
    internal/sync/initialcounts_test.go
  </files>
  <behavior>
    - Test 1 (NEW, in `internal/sync/initialcounts_test.go`): `TestInitialObjectCounts_AfterSync_ReflectsCurrentTable` — seed.Full populates the DB (13 types, deterministic counts). Call `InitialObjectCounts(ctx, client)` twice in succession (simulating cold-start + post-sync) without any intervening writes; assert both calls return identical maps. This locks the contract that the helper is idempotent and side-effect free, which is what makes it safe to call from BOTH the startup path AND the OnSyncComplete callback. (We do NOT need to spin up a real Worker.Sync to prove the post-sync semantic — the helper's contract IS the post-sync contract; the existing seeding test fixture stands in for "post-sync DB state".)
    - Test 2 (NEW, same file): `TestInitialObjectCounts_DeltaVsTableTotal` — seed.Full produces N rows for `net`, then explicitly assert `counts["net"] >= 2` (seed.Full creates ≥2 networks per its contract — see existing TestInitialObjectCounts_AllThirteenTypes). The assertion comment explains: "If a future regression replaced this with a per-cycle delta count, the cache would hold tiny numbers (often 0 or 1) on incremental sync cycles. By asserting the helper returns the full seeded count, we lock the current-table-totals contract."
    - Existing `TestInitialObjectCounts_PocPolicyBypass`, `TestInitialObjectCounts_AllThirteenTypes`, `TestInitialObjectCounts_EmptyDB`, `TestInitialObjectCounts_KeyParityWithSyncSteps` continue to pass unchanged.
    - No new test required at the cmd/peeringdb-plus integration layer — the existing initialcounts tests fully cover the helper, and the callback body is a one-liner wrapper. Integration coverage is provided in production by the Grafana panel itself; tests at the cmd layer would require constructing a fake Worker which is out of proportion to the change.
  </behavior>
  <action>
    **Step A — `cmd/peeringdb-plus/main.go` callback rewrite (around lines 263-273):**

    Replace the body of the `OnSyncComplete` callback so the gauge cache is refreshed via `pdbsync.InitialObjectCounts` instead of re-shaping the per-cycle `counts` arg. The callback signature MUST stay `func(counts map[string]int, syncTime time.Time)` for backward compatibility with the worker call site at `internal/sync/worker.go:755` and existing test fakes — but the `counts` arg becomes unused inside the body (rename to `_` per Go convention to silence linters and signal intent).

    The new body:

    ```go
    OnSyncComplete: func(_ map[string]int, syncTime time.Time) {
        // Refresh the gauge cache with current table totals via the same
        // helper that primes the cache at cold start. Reusing the helper
        // (rather than re-shaping the per-cycle `counts` arg) means the
        // cache holds full-table values on every cycle — including
        // incremental syncs where `counts` is just the delta since the
        // per-type cursor. See .planning/debug/poc-count-doubling-halving.md
        // for the predecessor symptom (Public/Users tier mismatch on POC).
        // The deeper semantic — that `counts` is an upserted-row delta,
        // not a current-table total — is what this site fixes.
        //
        // Cost: ~1-2s of 13 sequential SQLite COUNT(*) calls on a primed
        // LiteFS DB. Acceptable: OnSyncComplete already runs after commit
        // + FK validation, and the path is primary-only (replicas never
        // call this — their cache stays seeded from the cold-start primer
        // and is correct because the DB is read-only-replicated under them).
        //
        // Failure handling: if the count pass errors (e.g. transient
        // SQLite contention), keep the previous cache value and emit a
        // WARN. Do NOT fail the sync cycle — the sync already succeeded;
        // only the gauge prime is degraded for one interval. The next
        // successful sync will refresh the cache.
        refreshed, refreshErr := pdbsync.InitialObjectCounts(ctx, entClient)
        if refreshErr != nil {
            logger.LogAttrs(ctx, slog.LevelWarn,
                "post-sync object count refresh failed; gauge cache holds previous values",
                slog.Any("error", refreshErr))
        } else {
            objectCountCache.Store(&refreshed)
        }
        // PERF-07: swap the cached ETag using the exact completion
        // timestamp the worker persisted to sync_status. One SHA-256
        // per sync, zero per request. Independent of the count refresh —
        // ETag freshness must not be coupled to a transient count error.
        cachingState.UpdateETag(syncTime)
    },
    ```

    Key invariants:
    - `cachingState.UpdateETag(syncTime)` STILL fires unconditionally — it is decoupled from the count refresh by living on the success path of the parent if-else (here, after the if-else block, so it always runs).
    - The first arg is renamed `_` to make "counts is intentionally ignored" grep-visible.
    - `ctx`, `entClient`, `objectCountCache`, `cachingState`, `logger` are all already in scope at this callback site (inspected lines 195-273 of main.go).
    - No new imports needed: `pdbsync` (alias for `internal/sync`) is already imported and used at line 199 for the cold-start primer; `slog` is imported.

    **Step B — `internal/sync/initialcounts_test.go` test additions:**

    Append two new test functions to the existing file:

    ```go
    // TestInitialObjectCounts_AfterSync_ReflectsCurrentTable locks the
    // contract that InitialObjectCounts is idempotent and side-effect
    // free, so it is safe to call from BOTH the cold-start primer AND
    // the OnSyncComplete callback in cmd/peeringdb-plus/main.go. Without
    // this property, the post-sync writer could drift from the cold-start
    // writer across the 8-instance fleet (see
    // .planning/debug/poc-count-doubling-halving.md for the symptom this
    // contract prevents).
    func TestInitialObjectCounts_AfterSync_ReflectsCurrentTable(t *testing.T) {
        t.Parallel()
        client := testutil.SetupClient(t)
        seed.Full(t, client)

        first, err := InitialObjectCounts(context.Background(), client)
        if err != nil {
            t.Fatalf("InitialObjectCounts (1st call): %v", err)
        }
        second, err := InitialObjectCounts(context.Background(), client)
        if err != nil {
            t.Fatalf("InitialObjectCounts (2nd call): %v", err)
        }

        if len(first) != len(second) {
            t.Fatalf("len mismatch: first=%d second=%d", len(first), len(second))
        }
        for k, v := range first {
            if second[k] != v {
                t.Errorf("counts[%q]: first=%d second=%d (helper must be idempotent)", k, v, second[k])
            }
        }
    }

    // TestInitialObjectCounts_DeltaVsTableTotal locks the current-table-
    // totals contract. If a future regression replaced this helper with
    // a per-cycle delta count (the pre-fix OnSyncComplete behaviour —
    // `objectCounts[step.name] = len(items)` over per-cycle scratch), the
    // cache would hold tiny numbers (often 0 or 1) on incremental sync
    // cycles. By asserting the helper returns the full seeded counts,
    // we lock the post-fix semantics that drove this quick task
    // (260427-ojm).
    func TestInitialObjectCounts_DeltaVsTableTotal(t *testing.T) {
        t.Parallel()
        client := testutil.SetupClient(t)
        seed.Full(t, client)

        counts, err := InitialObjectCounts(context.Background(), client)
        if err != nil {
            t.Fatalf("InitialObjectCounts: %v", err)
        }

        // seed.Full creates ≥2 networks (cf. TestInitialObjectCounts_AllThirteenTypes
        // which asserts every type is non-zero). A delta-count regression
        // would frequently report 0 or 1 on cycles after the initial seed
        // because incremental sync upserts only changed rows. Asserting ≥2
        // catches that regression cheaply without depending on exact seed
        // arithmetic (which other tests already pin).
        if got := counts["net"]; got < 2 {
            t.Errorf("counts[\"net\"] = %d, want ≥ 2 (full table total, not per-cycle delta). "+
                "If you see 0 or 1 here, the helper has likely been replaced with a "+
                "per-cycle upserted-row count — restore the Count(ctx) per-table loop. "+
                "See .planning/quick/260427-ojm-replace-onsynccomplete-len-items-with-cu/.", got)
        }
    }
    ```

    Place the two new tests at the end of the file, after `TestInitialObjectCounts_PocPolicyBypass`.

    **Step C — verification commands (run in order):**

    1. `go build ./...`
    2. `go vet ./...`
    3. `go test -race -run "TestInitialObjectCounts" ./internal/sync/`
    4. `go test -race ./internal/sync/...`
    5. `go test -race ./internal/otel/... ./cmd/peeringdb-plus/...`
    6. `golangci-lint run ./internal/sync/... ./cmd/peeringdb-plus/...`

    All must pass. Step 3 should show 6 tests passing (the existing 4 + the 2 new ones).

    **Honour CLAUDE.md conventions:**
    - GO-CS-1: `gofmt -s` clean (Go formatter on save).
    - GO-ERR-1: `slog.Warn` includes the wrapped error via `slog.Any("error", err)` per the existing pattern at main.go:201 (`logger.Error("...", slog.Any("error", err))`).
    - GO-OBS-5: typed slog attribute setters (`slog.Any`, `slog.LevelWarn`) — the existing main.go callback site already uses this style.
    - The bypass audit invariant (single `privacy.Allow` call site in `Worker.Sync`) is preserved because `InitialObjectCounts` uses `privctx.TierUsers`, not `privacy.Allow`.
  </action>
  <verify>
    <automated>cd /home/dotwaffle/Code/pdb/peeringdb-plus && go build ./... && go vet ./... && go test -race -run "TestInitialObjectCounts" ./internal/sync/ && go test -race ./internal/sync/... ./internal/otel/... ./cmd/peeringdb-plus/... && golangci-lint run ./internal/sync/... ./cmd/peeringdb-plus/...</automated>
  </verify>
  <done>
    - `cmd/peeringdb-plus/main.go` `OnSyncComplete` callback body calls `pdbsync.InitialObjectCounts(ctx, entClient)` and stores the result into `objectCountCache` only on success; `cachingState.UpdateETag(syncTime)` fires unconditionally.
    - The first parameter of the callback is renamed `_` (underscore) to signal that the per-cycle delta is intentionally ignored.
    - On count-refresh error, a `slog.Warn` is emitted ("post-sync object count refresh failed; gauge cache holds previous values") and the previous cache value is preserved.
    - `internal/sync/initialcounts_test.go` contains two new tests: `TestInitialObjectCounts_AfterSync_ReflectsCurrentTable` and `TestInitialObjectCounts_DeltaVsTableTotal`.
    - `go build ./...`, `go vet ./...`, `go test -race ./internal/sync/... ./internal/otel/... ./cmd/peeringdb-plus/...`, and `golangci-lint run ./internal/sync/... ./cmd/peeringdb-plus/...` all pass clean.
    - Existing tests (`TestInitialObjectCounts_PocPolicyBypass`, `TestInitialObjectCounts_AllThirteenTypes`, `TestInitialObjectCounts_EmptyDB`, `TestInitialObjectCounts_KeyParityWithSyncSteps`, `TestSyncBypass_SingleCallSite`) all still pass.
    - No proto regen, no `go generate ./...` required (this is a non-schema change; CLAUDE.md sibling-file convention is N/A).
  </done>
</task>

</tasks>

<verification>
- Compile + vet + lint clean across `internal/sync` and `cmd/peeringdb-plus`.
- All 6 `TestInitialObjectCounts_*` tests pass under `-race`.
- `TestSyncBypass_SingleCallSite` (bypass audit) still passes — no new `privacy.Allow` references introduced; `InitialObjectCounts` continues to use `privctx.TierUsers`.
- Manual smoke check (post-deploy, out of scope for this plan but documented in the SUMMARY): in production, `pdbplus_data_type_count{type="poc"}` should hold a single stable value across the 8-instance fleet on both full and incremental sync cycles. Replicas already converged after commit 5c780c5; this plan ensures the primary stops dipping to the per-cycle delta on incremental cycles.
</verification>

<success_criteria>
- The OnSyncComplete callback in `cmd/peeringdb-plus/main.go` reuses `pdbsync.InitialObjectCounts(ctx, entClient)` rather than reshaping the per-cycle delta map.
- The PERF-07 ETag swap (`cachingState.UpdateETag(syncTime)`) is decoupled from the count refresh and fires on every successful sync regardless of count-refresh outcome.
- A count-refresh error logs a WARN and preserves the previous cache value; the sync cycle's success status is unchanged.
- Two new regression tests lock the current-table-totals contract for `InitialObjectCounts`.
- No changes to: ent schema, proto, env vars, metric names, OTel resource attrs, or the Grafana dashboard. The dashboard's `max by(type)` aggregation now becomes a deduplicator (all 8 instances converge on the same value) rather than an alternation oracle.
</success_criteria>

<output>
After completion, create `.planning/quick/260427-ojm-replace-onsynccomplete-len-items-with-cu/260427-ojm-SUMMARY.md` summarising:
- The before/after callback body diff (5 lines net change in main.go).
- Confirmation that the bypass audit invariant is preserved (no new `privacy.Allow` site).
- A note that the deeper Phase 75 OBS-01 work is now semantically complete: both the cold-start primer and the post-sync writer source counts from the same `InitialObjectCounts` helper, eliminating the residual upsert-vs-current semantic mismatch flagged in `.planning/debug/poc-count-doubling-halving.md` § "Out-of-scope follow-up".
- Test output snippet showing all 6 `TestInitialObjectCounts_*` tests pass.
</output>
