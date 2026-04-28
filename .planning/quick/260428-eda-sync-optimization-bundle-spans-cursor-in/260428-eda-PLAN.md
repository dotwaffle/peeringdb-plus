---
phase: 260428-eda
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/database/database.go
  - internal/sync/worker.go
  - internal/sync/upsert.go
  - internal/sync/upsert_test.go
  - internal/sync/status.go
  - internal/sync/initialcounts.go
  - internal/sync/initialcounts_test.go
  - cmd/peeringdb-plus/main.go
autonomous: true
requirements: [QUICK-260428-eda]

must_haves:
  truths:
    - "A typical incremental sync cycle in steady state (zero rows actually changed) commits the main upsert tx within seconds rather than ~60s, with cursor writes folded into that one tx."
    - "A future sync trace shows named child spans for every second between the last per-step upsert span and the root span end (sync-commit, sync-finalize, sync-cursor-updates, sync-on-complete, sync-record-status) — no >2s gap with no child span."
    - "Per-row skip-on-unchanged is enforced at the SQL level: an upsert with the same id and same `updated` timestamp does NOT touch the row; an upsert with a newer `updated` does."
    - "InitialObjectCounts returns the same map[string]int64 keys/values as before but issues exactly ONE SQL query against the underlying *sql.DB instead of 13 ent Count() roundtrips."
    - "UpsertCursor signature accepts an *ent.Tx (not *sql.DB), is called from inside syncUpsertPass before its return, and the godoc reflects the new in-tx semantic."
    - "From a user-observable perspective: a steady-state sync (no changed rows) commits its main upsert tx in <5s wall time (down from ~60s), measurable on a Tempo trace as the duration of the sync-commit span."
    - "All existing sync tests pass — TestWorkerSync_LineBudget, internal/sync/integration_test.go (CI-runnable full+incremental coverage of D-30 zero-divergence), bypass_audit_test.go, parity tests. (replay_snapshot_test.go remains as offline_replay-tagged defense-in-depth; not part of CI verify.)"
  artifacts:
    - path: "internal/database/database.go"
      provides: "DSN with synchronous=NORMAL, cache_size=-32000 (32 MB), temp_store=MEMORY"
      contains: "_pragma=synchronous(NORMAL)"
    - path: "internal/sync/worker.go"
      provides: "Wrapped tx.Commit + recordSuccess in named OTel spans; cursor-writes removed from recordSuccess; per-tx cache_spill=OFF; cursorUpdates threaded into syncUpsertPass; cursor_write_caused_rollback span attr"
      contains: "sync-commit"
    - path: "internal/sync/upsert.go"
      provides: "13 OnConflict sites use sql.UpdateWhere(...) to skip-on-unchanged based on the `updated` column; entgo bulk OnConflict path"
      contains: "sql.UpdateWhere"
    - path: "internal/sync/upsert_test.go"
      provides: "Regression test: unchanged-row upsert does NOT touch row; newer-updated does."
      contains: "TestUpsert_SkipOnUnchanged"
    - path: "internal/sync/status.go"
      provides: "UpsertCursor(ctx, tx *ent.Tx, ...) — runs inside the main sync tx; godoc updated"
      contains: "UpsertCursor(ctx context.Context, tx"
    - path: "internal/sync/initialcounts.go"
      provides: "Single UNION ALL SELECT against the underlying *sql.DB; ent.Client param replaced/augmented with *sql.DB; godoc reflects raw-SQL bypass of privacy policy with poc-count regression cross-reference"
      contains: "UNION ALL"
    - path: "internal/sync/initialcounts_test.go"
      provides: "Table-name regression test introspects sqlite_master to assert each table referenced in initialCountsQuery actually exists; fails fast on entgo re-pluralisation."
      contains: "TestInitialCountsQuery_TableNamesMatchSchema"
  key_links:
    - from: "internal/sync/worker.go (Worker.Sync at line ~617)"
      to: "tx.Commit()"
      via: "wrapped in sync-commit OTel span"
      pattern: "sync-commit"
    - from: "internal/sync/worker.go (syncUpsertPass)"
      to: "internal/sync/status.go UpsertCursor(ctx, tx, ...)"
      via: "cursorUpdates parameter threaded into syncUpsertPass; loop runs before return inside main tx"
      pattern: "UpsertCursor\\(ctx, tx"
    - from: "internal/sync/upsert.go (13 upsertX functions)"
      to: "ent CreateBulk(...).OnConflict(...).UpdateWhere(...)"
      via: "sql.UpdateWhere(sql.P(...)) referencing excluded.updated > <table>.updated, with empirically-verified zero-time guard (NULL plus the actual modernc-driver text/integer representation of time.Time{})"
      pattern: "sql\\.UpdateWhere"
    - from: "cmd/peeringdb-plus/main.go (OnSyncComplete callback + startup primer)"
      to: "internal/sync/initialcounts.go InitialObjectCounts"
      via: "signature now takes *sql.DB; both call sites updated; located via grep -n 'InitialObjectCounts' cmd/peeringdb-plus/main.go"
      pattern: "InitialObjectCounts\\("
    - from: "internal/database/database.go:27 DSN"
      to: "modernc.org/sqlite pragma parser"
      via: "&_pragma=synchronous(NORMAL)&_pragma=cache_size(-32000)&_pragma=temp_store(MEMORY)"
      pattern: "synchronous\\(NORMAL\\)"
---

<objective>
Drop typical incremental sync cycle wall time from ~117s to a target of <30s in steady state by eliminating the 23.4s post-commit tail, halving the LiteFS-replicated commit count (1 instead of 14), avoiding writes when rows are unchanged, replacing 13 sequential ent Count() calls with one UNION ALL, and tuning SQLite pragmas for the bulk-write workload. Add observability spans for the post-commit tail so any remaining time becomes attributable.

Purpose: Production trace `1c22931e65d639516a6987c848657283` showed sync-incremental took 118.03s; 23.4s of that was a black hole with no child spans, and 270k upserts ran even though zero rows actually changed. This bundle closes that gap and removes the redundant work.

Output: Faster, observable sync cycle. No behavioural change to downstream API surfaces; D-30 replay-snapshot zero-divergence preserved.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/STATE.md
@CLAUDE.md
@internal/sync/worker.go
@internal/sync/upsert.go
@internal/sync/status.go
@internal/sync/initialcounts.go
@internal/sync/integration_test.go
@internal/database/database.go

<interfaces>
<!-- Key contracts the executor needs. Extracted from the codebase. -->

From `internal/sync/status.go` (current — to be modified in Task 5):
```go
// CURRENT (D-19 atomicity violation — separate *sql.DB Exec, separate LiteFS commit):
func UpsertCursor(ctx context.Context, db *sql.DB, objType string, lastSyncAt time.Time, status string) error
```

From `internal/sync/worker.go`:
```go
func (w *Worker) Sync(ctx context.Context, mode config.SyncMode) error  // line 530
// Calls flow at lines 599..624:
//   tx, _ := w.entClient.Tx(ctx)
//   tx.ExecContext(ctx, "PRAGMA defer_foreign_keys = ON")     // line 605 — CHANGE 5 inserts cache_spill OFF here
//   objectCounts, _ := w.syncUpsertPass(ctx, tx, scratch, fromIncremental)
//   tx.Commit()                                               // line 617 — CHANGE 1 wraps this
//   w.recordSuccess(ctx, mode, statusID, start, objectCounts, cursorUpdates)  // line 623 — CHANGE 1 wraps body

func (w *Worker) syncUpsertPass(ctx context.Context, tx *ent.Tx, scratch *scratchDB, _ map[string]bool) (
    map[string]int, error,
)  // line 1016 — CHANGE 2: thread cursorUpdates as new param, write cursors before return

func (w *Worker) recordSuccess(
    ctx context.Context, mode config.SyncMode, statusID int64, start time.Time,
    objectCounts map[string]int, cursorUpdates map[string]time.Time,
)  // line 802 — CHANGE 1 wraps body in sync-finalize span; CHANGE 2 deletes the cursor-write loop at 813-818
```

From `internal/sync/upsert.go` (13 OnConflict sites at lines 108, 144, 204, 238, 262, 319, 350, 375, 401, 463, 491, 518, 648):
```go
// CURRENT pattern (always-update — wastes 270k row writes on no-op cycles):
return tx.Organization.CreateBulk(batch...).
    OnConflictColumns(organization.FieldID).
    UpdateNewValues().
    Exec(ctx)
```

From `entgo.io/ent@v0.14.6/dialect/sql/builder.go`:
```go
// VERIFIED PATH for CHANGE 3: entgo bulk OnConflict supports UpdateWhere as a ConflictOption.
//
// sql.ResolveWithNewValues()  // emits DO UPDATE SET col = excluded.col for every column
// sql.UpdateWhere(p *Predicate)  // appends WHERE p to the conflict-update branch
//
// CreateBulk works because <Entity>CreateBulk.OnConflict(...) signature accepts ...sql.ConflictOption.
// Verified at ent/organization_create.go:1813 + dialect/sql/builder.go:267.
```

From `internal/sync/initialcounts.go:80` (current — to be replaced in Task 3):
```go
func InitialObjectCounts(ctx context.Context, client *ent.Client) (map[string]int64, error)
// Currently: 13 separate client.X.Query().Count(ctx) under TierUsers context.
// CHANGE 6: replace with single UNION ALL via *sql.DB. Caller is cmd/peeringdb-plus/main.go (multiple call sites — grep).
```

From `ent/migrate/schema.go` — verified table names for the UNION ALL query:
```
organizations, campuses, facilities, carriers, carrier_facilities,
internet_exchanges, ix_lans, ix_prefixes, ix_facilities,
networks, pocs, network_facilities, network_ix_lans
```
</interfaces>

<load_bearing_invariants>
<!-- Existing invariants the executor MUST preserve. Search-grep for these by token. -->

- `TestWorkerSync_LineBudget` — Worker.Sync body capped at 100 lines. CHANGE 1 + CHANGE 2 + CHANGE 5 will push it over without helper extractions, so the helper extractions in T2 (`prepareTxPragmas`) and T4 (`commitWithSpan`) are MANDATORY, not conditional.
- D-19 atomicity — sync_status, sync_cursors, ent upserts must commit atomically (this plan moves cursor writes INTO the main tx; sync_status row update remains a separate raw-SQL Exec, which is fine — it's the "outcome" record, not a data write).
- D-30 replay snapshot — `replay_snapshot_test.go`: incremental cycles MUST produce zero divergence vs full. CHANGE 3's skip-on-unchanged MUST behave identically: if no row changed, nothing to write means nothing to diverge. **Note:** this test is `//go:build offline_replay` and is NOT exercised by `go test` without the tag. The CI-runnable equivalent is `internal/sync/integration_test.go` which exercises full+incremental against hermetic fixtures — that's what verifies D-30 in the standard test command.
- VIS-05 — bypass_audit_test.go restricts privacy.Allow to one production callsite. CHANGE 6's switch from ent Count() to raw SQL means the TierUsers elevation in initialcounts.go can be removed (raw SQL bypasses ent privacy entirely). The new godoc MUST preserve the historical "POC count doubling/halving" cross-reference so future maintainers searching for that regression find the explanation. The bypass audit invariant is unchanged.
- pdbcompat status matrix (Phase 68) and `_fold` shadow columns (Phase 69) — neither is touched by this plan. The skip-on-unchanged predicate operates on the `updated` column only.
- entproto frozen since v1.6 — we are not touching ent schemas, only call patterns. No proto regen needed.
</load_bearing_invariants>
</context>

<tasks>

<task type="auto">
  <name>Task 1: CHANGE 4 — DSN pragmas (synchronous=NORMAL, cache_size=-32000, temp_store=MEMORY)</name>
  <files>internal/database/database.go</files>
  <action>
Append three pragmas to the DSN format string in `Open` at line 26-29:

```go
dsn := fmt.Sprintf(
    "file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"+
        "&_pragma=synchronous(NORMAL)&_pragma=cache_size(-32000)&_pragma=temp_store(MEMORY)",
    dbPath,
)
```

Rationale embedded in the package godoc and/or a comment immediately above the DSN string:
- `synchronous(NORMAL)` is safe under LiteFS-replicated WAL — LiteFS provides durability via streaming replication; per-commit fsync to local disk is redundant overhead. The Fly primary's local WAL is replayed on replicas regardless of the local fsync mode.
- `cache_size(-32000)` = 32 MB page cache (negative value = KiB). Default is 2000 pages = ~2 MB. The bulk-upsert workload reuses recently-read pages heavily during the 60s upsert burst; 32 MB fits comfortably under the Fly 512 MB primary VM cap (sync peak heap ~37 MB, peak RSS ~232 MB observed in production).
- `temp_store(MEMORY)` keeps sorter and temp tables in RAM. modernc.org/sqlite's default is FILE which on Fly hits the rootfs overlay (NOT tmpfs — verified via /proc/mounts).

Verify modernc.org/sqlite pragma syntax by reading `$GOPATH/pkg/mod/modernc.org/sqlite*/sqlite.go` if uncertain — the `_pragma=name(value)` form is what its connector parser accepts. The three new pragmas are stdlib SQLite pragmas, no driver-specific behaviour.

Do NOT change `db.SetMaxOpenConns/SetMaxIdleConns/SetConnMaxLifetime` — out of scope.
  </action>
  <verify>
    <automated>go build ./... && go test -race ./internal/database/... ./internal/sync/... -count=1 -timeout=120s && golangci-lint run ./internal/database/... ./internal/sync/...</automated>
  </verify>
  <done>
    - DSN string contains all six pragmas (existing 3 + new 3).
    - `go test -race ./internal/sync/...` still passes.
    - No new lint findings: `golangci-lint run ./internal/database/...` clean.
    - Atomic commit: `chore(sync): tune SQLite pragmas for bulk-write workload`.
  </done>
</task>

<task type="auto">
  <name>Task 2: CHANGE 5 — Per-tx PRAGMA cache_spill = OFF (helper extraction MANDATORY)</name>
  <files>internal/sync/worker.go</files>
  <action>
**Helper extraction is MANDATORY in this task** (W1 line-budget guard). Without it, T2 (+5) plus T4's commit-span change (+3) push Worker.Sync past the 100-line `TestWorkerSync_LineBudget` cap. The pattern below adds exactly +1 line to Sync.

**1. Extract a helper.** New file-local function in worker.go (place near the other tx helpers):

```go
// prepareTxPragmas runs the per-tx PRAGMA setup that the bulk-upsert
// transaction depends on. It runs:
//   - PRAGMA defer_foreign_keys = ON    (existing — defers FK constraint
//     checking to commit so we can upsert in any order)
//   - PRAGMA cache_spill = OFF           (260428-eda CHANGE 5 — keeps
//     dirty pages in the connection's page cache instead of spilling to
//     the WAL between writes; bounded by cache_size from the DSN)
//
// cache_spill is per-tx (not via the DSN) because it's a connection-
// scoped pragma whose effect we only want during the bulk-write tx.
// Setting it via the DSN would apply it to read-path connections too.
func prepareTxPragmas(ctx context.Context, tx *ent.Tx) error {
    if _, err := tx.ExecContext(ctx, "PRAGMA defer_foreign_keys = ON"); err != nil {
        return fmt.Errorf("defer foreign keys: %w", err)
    }
    if _, err := tx.ExecContext(ctx, "PRAGMA cache_spill = OFF"); err != nil {
        return fmt.Errorf("disable cache_spill: %w", err)
    }
    return nil
}
```

**2. Replace the inline PRAGMA at Worker.Sync line ~605.** The existing block is:

```go
if _, err := tx.ExecContext(ctx, "PRAGMA defer_foreign_keys = ON"); err != nil {
    deferErr := fmt.Errorf("defer foreign keys: %w", err)
    w.rollbackAndRecord(ctx, mode, tx, statusID, start, deferErr)
    return deferErr
}
```

Replace with:

```go
if err := prepareTxPragmas(ctx, tx); err != nil {
    w.rollbackAndRecord(ctx, mode, tx, statusID, start, err)
    return err
}
```

Net change to Worker.Sync: -1 line (replaces 5 lines with 4). This buys headroom for T4.

**3. Line budget.** `TestWorkerSync_LineBudget` measures Worker.Sync only. The new helper lives outside Sync. With this extraction Sync is comfortably under 100 lines and T4's later additions stay under too.
  </action>
  <verify>
    <automated>go build ./... && go test -race ./internal/sync/... -count=1 -timeout=120s -run "TestWorkerSync_LineBudget|TestWorker" && golangci-lint run ./internal/sync/...</automated>
  </verify>
  <done>
    - `prepareTxPragmas` helper exists and runs both PRAGMAs in order.
    - Worker.Sync calls `prepareTxPragmas(ctx, tx)` once; no inline PRAGMA exec remains in Sync.
    - Sync error path covered: a failing PRAGMA results in rollback + recordFailure (mirrors the prior defer_foreign_keys path via the helper return).
    - `TestWorkerSync_LineBudget` still passes (Sync line count went DOWN, not up).
    - Atomic commit: `perf(sync): hold dirty pages in cache during bulk-upsert tx`.
  </done>
</task>

<task type="auto">
  <name>Task 3: CHANGE 6 — Replace InitialObjectCounts with single UNION ALL</name>
  <files>internal/sync/initialcounts.go, internal/sync/initialcounts_test.go, cmd/peeringdb-plus/main.go</files>
  <action>
Rewrite `InitialObjectCounts` to issue exactly one SQL query against the underlying *sql.DB.

**1. Update the function signature.** The current signature is `(ctx, *ent.Client) (map[string]int64, error)`. Replace `*ent.Client` with `*sql.DB`:

```go
func InitialObjectCounts(ctx context.Context, db *sql.DB) (map[string]int64, error)
```

The ent.Client parameter is removed — raw SQL bypasses ent entirely.

**2. Implement.** Single `db.QueryContext` against:

```sql
SELECT 'org' AS t, COUNT(*) AS c FROM organizations
UNION ALL SELECT 'campus', COUNT(*) FROM campuses
UNION ALL SELECT 'fac', COUNT(*) FROM facilities
UNION ALL SELECT 'carrier', COUNT(*) FROM carriers
UNION ALL SELECT 'carrierfac', COUNT(*) FROM carrier_facilities
UNION ALL SELECT 'ix', COUNT(*) FROM internet_exchanges
UNION ALL SELECT 'ixlan', COUNT(*) FROM ix_lans
UNION ALL SELECT 'ixpfx', COUNT(*) FROM ix_prefixes
UNION ALL SELECT 'ixfac', COUNT(*) FROM ix_facilities
UNION ALL SELECT 'net', COUNT(*) FROM networks
UNION ALL SELECT 'poc', COUNT(*) FROM pocs
UNION ALL SELECT 'netfac', COUNT(*) FROM network_facilities
UNION ALL SELECT 'netixlan', COUNT(*) FROM network_ix_lans
```

Hold the SQL string as a package-level `const initialCountsQuery = "..."` for grep-ability. Iterate `rows.Next()`, scan `(name string, count int64)`, populate the map. Defer `rows.Close()`. Honour `ctx.Err()` once before the query.

**3. Drop the TierUsers elevation.** Raw SQL bypasses ent's privacy policy entirely (no Privacy Hook fires on raw `db.QueryContext`). Remove the `privctx.WithTier(ctx, privctx.TierUsers)` line.

**4. Godoc rewrite — REQUIRED structure (B4 preserves the POC regression cross-reference).** Replace the existing godoc with the following four-paragraph structure:

```go
// InitialObjectCounts runs a one-shot UNION ALL COUNT(*) against each of
// the 13 PeeringDB entity tables and returns the result keyed by
// PeeringDB type name. The keys match those produced by syncSteps() so
// the same atomic cache can be primed by either the startup path (this
// helper) or the OnSyncComplete callback.
//
// Implements OBS-01 D-01: synchronous startup population so the
// pdbplus_data_type_count gauge reports correct values within 30s of
// process start instead of holding zeros until the first sync cycle
// completes (~15 min default, ~1h on unauthenticated instances).
//
// Cost: a single SQL UNION ALL across 13 tables; ~1ms on a primed
// LiteFS DB. Replaces the prior 13 sequential ent Count() calls
// (~15-20ms in aggregate). Counts include all rows regardless of status
// (matching the existing OnSyncComplete cache contract — "raw upserted-
// row count from the latest sync cycle"). Phase 68 tombstones
// (status="deleted") are rows the dashboard wants to see in "Total
// Objects" until tombstone GC ships (SEED-004 dormant). If a future
// requirement wants live-only counts, that's a separate metric.
//
// Privacy: raw SQL bypasses ent's Privacy policy entirely (no Privacy
// Hook fires on db.QueryContext). The COUNT(*) sees every physical row
// regardless of privacy tier — symmetric with the OnSyncComplete writer
// (which runs under privacy.DecisionContext(ctx, privacy.Allow)).
//
// Phase 75 OBS-01 D-01 history: this function previously elevated ctx
// to TierUsers via privctx.WithTier to keep Poc.Policy from filtering
// visible!="Public" rows. Without it, the cross-writer disagreement on
// POC counts caused the pdbplus_data_type_count{type="poc"} 2x/0.5x
// oscillation visible on the Grafana "Object Counts Over Time" panel:
// replicas (which only ever ran InitialObjectCounts) held the public-
// only count P while the primary's cache flipped between T ≈ 2P (just
// after a full sync) and tiny incremental deltas, and max by(type)
// across the 8-instance fleet alternated between T and P accordingly.
// 260428-eda CHANGE 6 retires the tier elevation entirely: raw SQL
// achieves the same row-set without going through ent privacy at all
// (a COUNT bypass is intentional and safe). See
// .planning/debug/poc-count-doubling-halving.md for the full incident
// analysis.
//
// Errors are returned wrapped with the type name so an operator can
// see which table failed; partial results are NOT returned — a single
// failure aborts the whole call to keep the contract simple.
```

The "Phase 75 OBS-01 D-01 history" paragraph and the cross-reference to `.planning/debug/poc-count-doubling-halving.md` are MANDATORY. Future maintainers grepping for "TierUsers" or "poc.*doubl" in this file MUST find this explanation.

Remove the unused `privctx` and `ent` imports; add `database/sql` if not already there.

**5. Update the call sites in cmd/peeringdb-plus/main.go.** The existing godoc says startup-primer + OnSyncComplete callback; verify by grep — do NOT assume:

```bash
grep -n "InitialObjectCounts" cmd/peeringdb-plus/main.go
```

For each match: update the call to pass the *sql.DB (returned alongside *ent.Client by `database.Open`) instead of the *ent.Client. If a closure captures the wrong handle, fix the capture too. The OnSyncComplete callback at line ~286 and the startup primer (separately) both need updating.

**6. Test updates in internal/sync/initialcounts_test.go.**

- Existing tests that pass `*ent.Client` need updating to pass the underlying `*sql.DB`. Use `internal/testutil.SetupClientWithDB(t)` which returns both (verified — the file uses `SetupClientWithDB` already).
- Keep the test that verifies all 13 keys are present.
- Keep the test that verifies POC counts include `visible="Users"` rows.
- **Add a table-name regression test (W5).** Introspect ent's schema via `sqlite_master` and assert every table name referenced in `initialCountsQuery` exists in the live DB. Fails fast on an entgo bump that re-pluralises a table:

```go
func TestInitialCountsQuery_TableNamesMatchSchema(t *testing.T) {
    t.Parallel()
    _, db := testutil.SetupClientWithDB(t)
    ctx := t.Context()

    // Tables we expect — extracted statically from initialCountsQuery.
    expected := []string{
        "organizations", "campuses", "facilities", "carriers",
        "carrier_facilities", "internet_exchanges", "ix_lans",
        "ix_prefixes", "ix_facilities", "networks", "pocs",
        "network_facilities", "network_ix_lans",
    }
    for _, name := range expected {
        var got string
        err := db.QueryRowContext(ctx,
            `SELECT name FROM sqlite_master WHERE type='table' AND name=?`,
            name,
        ).Scan(&got)
        if err != nil {
            t.Errorf("table %q referenced by initialCountsQuery not found in schema: %v", name, err)
        }
    }
}
```

If any entry fails, the entgo schema has drifted from the hard-coded UNION ALL — fix initialcounts.go's table list before merging.

**Crucial guardrail:** the table names above are NOT guesses. They were grepped from `ent/migrate/schema.go` (the generator's own truth source). Do NOT renumber/rename without re-grep.
  </action>
  <verify>
    <automated>go build ./... && go test -race ./internal/sync/... -count=1 -timeout=120s -run "InitialObjectCounts|TestPocCount|TestInitialCountsQuery" && golangci-lint run ./internal/sync/... ./cmd/peeringdb-plus/...</automated>
  </verify>
  <done>
    - `InitialObjectCounts` issues exactly one SQL query (verifiable by reading the function — single `db.QueryContext` call).
    - All 13 keys present in returned map; values match seeded row counts in tests.
    - Both call sites in cmd/peeringdb-plus/main.go updated to pass *sql.DB; `grep -n "InitialObjectCounts" cmd/peeringdb-plus/main.go` enumerates only updated lines.
    - `privctx`/`ent.Client` imports removed if no longer referenced.
    - Godoc preserves the POC count doubling/halving cross-reference and points to `.planning/debug/poc-count-doubling-halving.md`.
    - `TestInitialCountsQuery_TableNamesMatchSchema` passes — every table in the SQL exists in the schema.
    - `go test -race ./internal/sync/... -count=1` passes; `golangci-lint run` clean.
    - Atomic commit: `perf(sync): collapse 13 Count() calls into one UNION ALL`.
  </done>
</task>

<task type="auto">
  <name>Task 4: CHANGE 1 — Observability spans for the post-commit tail (helper extraction MANDATORY)</name>
  <files>internal/sync/worker.go</files>
  <action>
Wrap the post-commit work in named OTel spans so a future trace shows where the 23.4s tail goes.

**1. Wrap `tx.Commit()` in a `sync-commit` span via a MANDATORY helper extraction (W1 + W2).** At worker.go:617 (current tx.Commit call), introduce a file-local helper:

```go
// commitWithSpan commits tx inside a named OTel span so the LiteFS-
// replicated commit duration is visible in Tempo. Pattern matches
// existing per-step spans elsewhere in worker.go (e.g. line ~917).
//
// 260428-eda CHANGE 1.
func commitWithSpan(ctx context.Context, tx *ent.Tx) error {
    _, commitSpan := otel.Tracer("sync").Start(ctx, "sync-commit")
    defer commitSpan.End()
    return tx.Commit()
}
```

Replace the existing `tx.Commit()` call site in Sync with:

```go
if commitErr := commitWithSpan(ctx, tx); commitErr != nil {
    syncErr := fmt.Errorf("commit sync transaction: %w", commitErr)
    w.recordFailure(ctx, mode, statusID, start, syncErr)
    return syncErr
}
```

W2: do NOT introduce an unused `commitCtx` variable. The `_, commitSpan := otel.Tracer("sync").Start(...)` form (matching the existing pattern at worker.go:917) is canonical.

Net change to Worker.Sync: roughly +1 line vs the prior tx.Commit block. Combined with T2's -1, line budget stays comfortable.

**2. Wrap the body of `recordSuccess` in a `sync-finalize` span.** At worker.go:802, immediately after the `func recordSuccess(...)` `{`, add:

```go
ctx, finalizeSpan := otel.Tracer("sync").Start(ctx, "sync-finalize")
defer finalizeSpan.End()
```

Reassign ctx so the sub-spans (next step) parent under `sync-finalize` rather than the root.

**3. Add three sub-spans inside recordSuccess:**

- `sync-cursor-updates` — wrap the cursor write loop at lines 813-818. **Order-of-operations note:** Task 4 lands BEFORE Task 5. So in this task, the loop is still present and gets a span. When Task 5 removes the loop, also remove this span — Task 5 will move the cursor-write span into syncUpsertPass.
- `sync-record-status` — wrap the `RecordSyncComplete(ctx, w.db, ...)` call at line ~840.
- `sync-on-complete` — wrap the `w.config.OnSyncComplete(ctx, completedAt)` call at line ~852.

Each sub-span pattern (W2-aligned, no unused ctx):
```go
_, span := otel.Tracer("sync").Start(ctx, "sync-cursor-updates")
// ... existing code ...
span.End()
```

**4. Imports.** `otel` import already present in worker.go (used by syncFetchPass step spans). No new imports needed.

**5. Line budget.** This task adds ~12 lines to recordSuccess (4 spans × ~3 lines each) and ~1 line net to Sync (the `commitWithSpan` call replaces the tx.Commit block). recordSuccess has no line-budget test (TestWorkerSync_LineBudget covers Sync only). Sync stays under 100 thanks to T2's helper extraction.

**6. No behavioural change.** Spans are pure observability; no return paths change.
  </action>
  <verify>
    <automated>go build ./... && go test -race ./internal/sync/... -count=1 -timeout=120s -run "TestWorkerSync_LineBudget|TestWorker" && golangci-lint run ./internal/sync/...</automated>
  </verify>
  <done>
    - `commitWithSpan` helper exists and uses `_, commitSpan := otel.Tracer("sync").Start(...)` (no unused ctx variable).
    - `tx.Commit()` is wrapped in a span named `sync-commit` via the helper.
    - `recordSuccess` body is wrapped in a span named `sync-finalize`.
    - Three sub-spans present: `sync-cursor-updates`, `sync-record-status`, `sync-on-complete`.
    - `TestWorkerSync_LineBudget` passes; `golangci-lint run` clean (no unused-variable findings).
    - All sync tests pass; behavioural expectations unchanged.
    - Atomic commit: `feat(sync): instrument post-commit tail with named spans`.
  </done>
</task>

<task type="auto">
  <name>Task 5: CHANGE 2 — Embed cursor writes in main upsert tx</name>
  <files>internal/sync/worker.go, internal/sync/status.go, internal/sync/status_test.go</files>
  <action>
Move cursor writes from recordSuccess (separate *sql.DB Exec post-commit, ~13 LiteFS-replicated commits) into syncUpsertPass (inside the single main upsert tx, 0 extra commits).

**1. Convert UpsertCursor signature.** In `internal/sync/status.go:80`:

```go
// BEFORE:
func UpsertCursor(ctx context.Context, db *sql.DB, objType string, lastSyncAt time.Time, status string) error

// AFTER:
func UpsertCursor(ctx context.Context, tx *ent.Tx, objType string, lastSyncAt time.Time, status string) error {
    _, err := tx.ExecContext(ctx,
        `INSERT INTO sync_cursors (type, last_sync_at, last_status)
         VALUES (?, ?, ?)
         ON CONFLICT(type) DO UPDATE SET last_sync_at = excluded.last_sync_at, last_status = excluded.last_status`,
        objType, lastSyncAt, status,
    )
    if err != nil {
        return fmt.Errorf("upsert cursor for %s: %w", objType, err)
    }
    return nil
}
```

The SQL is byte-identical; only the connection target changes. Add the `ent` import to status.go.

**2. Update the godoc** at status.go:78-79:
```go
// UpsertCursor updates or inserts the sync cursor for a type.
//
// Called WITHIN the main sync transaction (via *ent.Tx) so cursor
// writes commit atomically with their corresponding ent upserts. This
// closes the prior gap where cursor writes were 13 separate post-commit
// *sql.DB Exec calls — each one a LiteFS-replicated commit — and
// removes the failure window where ent upserts were durable but the
// cursor advance was not (resulting in re-fetching already-applied
// rows on the next cycle).
//
// Per quick task 260428-eda. See the prior comment about D-19 atomicity:
// sync_status (the outcome record) remains a separate raw-SQL Exec
// because it must reflect the OUTCOME of the tx (success/failure/error
// message) — that's correct. Cursors describe DATA STATE and belong
// inside the data tx.
//
// Failure-mode shift: a cursor-write failure now rolls back the entire
// upsert tx (including any FK-backfill HTTP work that already happened
// inside it). This is the CORRECT semantic — cursor IS data state and
// a divergence between upserts-committed and cursor-not-advanced is
// the very bug being fixed. The OTel span attribute
// pdbplus.sync.cursor_write_caused_rollback is set true on the sync
// root span when a rollback was caused by cursor write failure (B3).
// SyncWithRetry handles transient failures by re-running the cycle.
```

**3. Thread cursorUpdates into syncUpsertPass.** Worker.Sync at line ~612:

```go
// BEFORE:
objectCounts, err := w.syncUpsertPass(ctx, tx, scratch, fromIncremental)

// AFTER:
objectCounts, err := w.syncUpsertPass(ctx, tx, scratch, fromIncremental, cursorUpdates)
```

In syncUpsertPass (worker.go:1016) — extend the signature:

```go
func (w *Worker) syncUpsertPass(
    ctx context.Context, tx *ent.Tx, scratch *scratchDB,
    _ map[string]bool,
    cursorUpdates map[string]time.Time,
) (map[string]int, error)
```

Immediately before syncUpsertPass returns successfully (at the bottom, before the final `return objectCounts, nil`), add:

```go
// Cursor writes commit atomically with the upserts (260428-eda CHANGE 2).
_, cursorSpan := otel.Tracer("sync").Start(ctx, "sync-cursor-updates")
for typeName, generated := range cursorUpdates {
    if err := UpsertCursor(ctx, tx, typeName, generated, "success"); err != nil {
        cursorSpan.End()
        // B3: stamp the sync root span with the rollback cause so a
        // future Tempo trace makes the failure mode visible.
        if rootSpan := trace.SpanFromContext(ctx); rootSpan.IsRecording() {
            rootSpan.SetAttributes(attribute.Bool("pdbplus.sync.cursor_write_caused_rollback", true))
        }
        return nil, fmt.Errorf("write cursor for %s: %w", typeName, err)
    }
}
cursorSpan.End()
```

The `sync-cursor-updates` span (added in CHANGE 1 / Task 4 inside recordSuccess) MOVES here — it's now inside the main tx, parented under whatever sync-cycle span is active. Add the imports `go.opentelemetry.io/otel/attribute` and `go.opentelemetry.io/otel/trace` if not already present.

**Add a docstring paragraph on Worker.Sync** near the cursor-write site documenting the failure-mode shift (B3):

```go
// NOTE (260428-eda CHANGE 2): cursor writes happen INSIDE the main
// upsert tx via syncUpsertPass. A cursor-write failure rolls back the
// entire tx — including any FK-backfill HTTP work that already
// happened in it. This inversion is intentional: the previous
// post-commit-cursor design left a window where upserts were durable
// but the cursor was not, causing re-fetch of already-applied rows on
// the next cycle. Rollback after wasted FK backfills is strictly
// better than that divergence. SyncWithRetry handles transient
// failures by re-running the whole cycle. The OTel span attr
// pdbplus.sync.cursor_write_caused_rollback flags this failure mode
// for postmortem in Tempo.
```

**4. Remove the now-dead loop from recordSuccess.** At worker.go:813-818, delete:

```go
for typeName, generated := range cursorUpdates {
    if err := UpsertCursor(ctx, w.db, typeName, generated, "success"); err != nil {
        w.logger.LogAttrs(ctx, slog.LevelError, "failed to update cursor",
            slog.String("type", typeName), slog.Any("error", err))
    }
}
```

Also remove the `sync-cursor-updates` sub-span that Task 4 placed around this loop — it has moved to syncUpsertPass.

The `cursorUpdates` parameter to recordSuccess can stay or be dropped:
- **Prefer dropping it.** recordSuccess no longer uses cursorUpdates after this task. Removing the parameter requires updating the recordSuccess signature and the call site at Worker.Sync line ~623. Cleaner.

**5. No retry inside the cursor loop (B3).** The worker's outer SyncWithRetry path already handles transient failures by re-running the whole cycle. Do NOT add an in-loop retry — that would mask the very failure mode the rollback attribute is meant to surface.

**6. Test impact (W6) — ENUMERATED.** From `internal/sync/status_test.go`, the *sql.DB-flavoured UpsertCursor callers are:

- `TestUpsertCursor_InsertAndGet` (line 51) — rewrite to open `tx, err := client.Tx(ctx)` from `testutil.SetupClient(t)`, call `UpsertCursor(ctx, tx, ...)`, then `tx.Commit()`, then `GetCursor` reads via the original *sql.DB. Round-trip semantic preserved.
- `TestUpsertCursor_UpdateExisting` (line 74) — same rewrite pattern; two UpsertCursor calls each in their own committed tx.
- `TestUpsertCursor_DBError` (line 166) — DO NOT delete; preserve fault-injection coverage. Rewrite as: open `tx`, call `tx.Rollback()`, then call `UpsertCursor(ctx, tx, ...)` — modernc surfaces use-after-rollback as `sql: transaction has already been committed or rolled back`. Assert error contains "upsert cursor for".

Other UpsertCursor call sites: any in `internal/sync/integration_test.go` or worker tests need the same rewrite. Grep with:

```bash
grep -n "UpsertCursor(ctx, db" internal/sync/...
grep -rn "sync\.UpsertCursor" .
```

If a test was relying on the post-commit-not-blocking behaviour, update its expectation: cursor failures now abort the cycle.

**7. Line budget.** Worker.Sync gains 0 net lines (just one extra arg). syncUpsertPass gains ~14 lines (loop + rollback-attr stamping). recordSuccess loses ~6. TestWorkerSync_LineBudget unchanged.
  </action>
  <verify>
    <automated>go build ./... && go test -race ./internal/sync/... -count=1 -timeout=180s && golangci-lint run ./internal/sync/...</automated>
  </verify>
  <done>
    - UpsertCursor takes *ent.Tx; godoc reflects the new in-tx semantic AND documents the failure-mode shift + cursor_write_caused_rollback attribute.
    - Cursor-write loop runs inside syncUpsertPass before its return.
    - The dead cursor-write loop and its span are gone from recordSuccess.
    - On a successful sync, exactly one tx.Commit happens (covering upserts + cursors).
    - `pdbplus.sync.cursor_write_caused_rollback` attr is set on the root span when a cursor write fails.
    - All three enumerated status tests (TestUpsertCursor_InsertAndGet, TestUpsertCursor_UpdateExisting, TestUpsertCursor_DBError) rewritten and passing — DB-error coverage preserved via tx.Rollback() + use-after-rollback.
    - Worker.Sync carries a docstring paragraph documenting the failure-mode shift.
    - All sync tests pass, including TestWorkerSync_LineBudget and the CI-runnable integration_test.go (D-30 zero-divergence equivalent). `golangci-lint run` clean.
    - Atomic commit: `perf(sync): fold cursor writes into main upsert tx (D-19 atomicity)`.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 6: CHANGE 3 — Per-row skip-on-unchanged via UpdateWhere predicate</name>
  <files>internal/sync/upsert.go, internal/sync/upsert_test.go</files>
  <behavior>
    - Test 1 (skip-on-unchanged): seed organization id=1 with updated=T, name="A". Call upsertOrganizations with the same id=1, updated=T, name="B". Assert the row's name is STILL "A" (the row was not updated). Use a sentinel: read back via tx.Organization.Get(ctx, 1) and compare Name.
    - Test 2 (update-on-newer): seed organization id=1 with updated=T, name="A". Call upsertOrganizations with id=1, updated=T+1s, name="B". Assert the row's name is now "B".
    - Test 3 (update-on-zero-updated): seed organization id=1 with updated=zeroTime (time.Time{}), name="A". Call upsertOrganizations with id=1, updated=zeroTime, name="B". Assert the row's name is now "B" (zero-updated rows must always update — otherwise rows with no `updated` value would be permanently frozen).
    - Test 4 (insert-when-absent): no row id=1. Call upsertOrganizations with id=1. Assert row exists.
    - Test 5 (D-30 zero-divergence smoke, CI-runnable): `internal/sync/integration_test.go` MUST still pass — drives the bigger guarantee that incremental cycles produce the same result as full. (replay_snapshot_test.go is build-tagged offline_replay and not exercised by CI; it remains as offline defense-in-depth.)
  </behavior>
  <action>
For each of the 13 OnConflict sites in `internal/sync/upsert.go` (lines 108, 144, 204, 238, 262, 319, 350, 375, 401, 463, 491, 518, 648), replace `UpdateNewValues()` with the explicit `OnConflict(...)` form carrying both `sql.ResolveWithNewValues()` and `sql.UpdateWhere(...)`.

**Pattern (organization shown; apply to all 13).** Currently:

```go
return tx.Organization.CreateBulk(batch...).
    OnConflictColumns(organization.FieldID).
    UpdateNewValues().
    Exec(ctx)
```

Replace with:

```go
return tx.Organization.CreateBulk(batch...).
    OnConflict(
        sql.ConflictColumns(organization.FieldID),
        sql.ResolveWithNewValues(),
        sql.UpdateWhere(skipUnchangedPredicate("organizations")),
    ).
    Exec(ctx)
```

**STEP 0 — Empirically verify the modernc zero-time storage representation BEFORE finalizing the predicate (B2).** The original plan assumed `<table>.updated = 0` works because SQLite stores zero time as integer 0. That's wrong: modernc.org/sqlite serializes Go `time.Time{}` via the driver's value converter, which can produce text `'0001-01-01T00:00:00Z'` rather than integer 0 depending on the column affinity ent declared. Before writing the predicate, run a one-line probe in a scratch test (or a manual `go run` snippet) — for example:

```go
// Probe: insert a row with updated=time.Time{}, then SELECT updated.
client, db := testutil.SetupClientWithDB(t)
_ = client.Organization.Create().SetID(99999).SetName("zero-probe").
    SetUpdated(time.Time{}).SaveX(ctx)
var raw any
_ = db.QueryRowContext(ctx, "SELECT updated FROM organizations WHERE id=99999").Scan(&raw)
t.Logf("zero-time raw representation: %T %v", raw, raw)
```

Use the observed representation to write the predicate. The safest universal form covers both possibilities:

```go
//   excluded.updated > <table>.updated
//   OR <table>.updated IS NULL
//   OR <table>.updated <= '1900-01-01'   -- catches text '0001-01-01...'
//   OR <table>.updated = 0               -- catches integer-affinity zero (paranoia)
```

If the probe shows TEXT storage (Go `string` scan target receiving `"0001-01-01 00:00:00+00:00"` or similar), use the `<= '1900-01-01'` guard. If it shows INTEGER (Go `int64` 0), use `= 0`. If you can't reproduce a zero-update row from upstream and the corpus has none, the OR-IS-NULL clause alone may suffice — but include both guards out of caution. **Document the observed representation in a code comment above the predicate** so future maintainers don't repeat the probe.

Where `skipUnchangedPredicate(table string) *sql.Predicate` is a single new helper at the top of `internal/sync/upsert.go`:

```go
// skipUnchangedPredicate emits the WHERE clause for ON CONFLICT DO UPDATE
// that gates writes on the upstream `updated` timestamp. It returns a
// predicate equivalent to:
//
//   excluded.updated > <table>.updated
//   OR <table>.updated IS NULL
//   OR <table>.updated <= '1900-01-01'   // see STEP 0 probe — modernc text affinity
//
// The OR-IS-NULL/OR-pre-1900 guards exist because PeeringDB rows
// occasionally land with zero `updated` (legacy rows pre-Phase-X
// migration); we must always allow them through, otherwise a row with
// `updated` permanently at zero would become unwriteable. The literal
// '1900-01-01' is the lower-bound sentinel: any real PeeringDB
// timestamp is post-2000, and modernc.org/sqlite stores Go
// time.Time{} as text '0001-01-01...' under the default value
// converter (verified empirically — see STEP 0).
//
// SAME-SECOND DRIFT (260428-eda CHANGE 3 known limitation): a row
// edited at upstream within the same second as the prior cursor
// advance will skip on the next incremental cycle. Mitigations:
// (a) the next upstream change will bump `updated` past the cursor
// and reconcile naturally; (b) full-mode runs
// (PDBPLUS_SYNC_MODE=full) ignore this predicate and reconcile
// completely. We deliberately use strict `>` rather than `>=`:
// `>=` would defeat the optimization entirely, since PeeringDB's
// ?since=N is inclusive and every refetch produces excluded.updated
// >= existing.updated. The bounded same-second-drift risk is the
// trade.
//
// 260428-eda CHANGE 3.
func skipUnchangedPredicate(table string) *sql.Predicate {
    return sql.P(func(b *sql.Builder) {
        b.WriteString("excluded.updated > ").Ident(table).WriteString(".updated")
        b.WriteString(" OR ").Ident(table).WriteString(".updated IS NULL")
        b.WriteString(" OR ").Ident(table).WriteString(".updated <= '1900-01-01'")
    })
}
```

(Adjust the third clause to match what the STEP 0 probe revealed for the actual modernc storage representation. If both text and int affinities appear, OR them together — it costs ~nothing.)

Verify the `sql.Predicate` surface in `entgo.io/ent@v0.14.6/dialect/sql/builder.go` — the `sql.P(func(*sql.Builder))` constructor and `sql.UpdateWhere(*Predicate) ConflictOption` are both present.

**Add the import** `entgo.io/ent/dialect/sql` to upsert.go if not already present.

**Table-name mapping** (must match the same names verified in CHANGE 6):
- organization → "organizations"
- campus → "campuses"
- facility → "facilities"
- carrier → "carriers"
- carrierfacility → "carrier_facilities"
- internetexchange → "internet_exchanges"
- ixlan → "ix_lans"
- ixprefix → "ix_prefixes"
- ixfacility → "ix_facilities"
- network → "networks"
- poc → "pocs"
- networkfacility → "network_facilities"
- networkixlan → "network_ix_lans"

A typo here is silent (the predicate references the wrong identifier; SQLite will fail with `no such column`). The integration test surfaces this.

**ResolveWithNewValues** is the equivalent of the prior `UpdateNewValues()` (it sets every column to `excluded.<col>`). The two changes — explicit `OnConflict(...)` form and `UpdateWhere` predicate — are independent; the resolve behaviour is preserved.

**Test file: internal/sync/upsert_test.go.** Add `TestUpsert_SkipOnUnchanged` with sub-tests matching the behaviour block above. Use `internal/testutil.SetupClient(t)` for an isolated in-memory DB. Open a tx via `client.Tx(context.Background())`, seed initial state via raw `client.Organization.Create()...Save(ctx)`, then call `upsertOrganizations(ctx, tx, []peeringdb.Organization{...})` and assert via `tx.Organization.Get(ctx, 1)`.

The SENTINEL trick: pick a column that's set in the upsert payload but distinguishable between two write attempts (Name is fine — it's set by SetName on every Create). After the second upsert, read back: if `row.Name` reflects the second call's value, the predicate didn't fire (row was updated); if it reflects the first call's value, the predicate fired (row was NOT updated, which is what we want when updated is unchanged).

**Same-second drift convergence escape-hatch.** This optimization carries a known same-second-drift risk: a row edited within the same upstream second as the prior cursor advance will skip on the next incremental. Mitigated by full-mode runs. **Add a follow-up seed note**: create `.planning/seeds/SEED-XXX-periodic-full-sync-schedule.md` (executor allocates next free SEED number) sketching a periodic full-sync schedule (e.g., weekly) for convergence — out of scope for this plan, but flagged.

**D-30 preservation.** The CI-runnable equivalent is `internal/sync/integration_test.go`. After CHANGE 3, an unchanged-row incremental will skip writes that would otherwise be no-ops (excluded col == row col for every col); the row is byte-identical either way, so divergence stays zero. `replay_snapshot_test.go` is `//go:build offline_replay` and is NOT exercised by `go test`; it stays as defense-in-depth that the operator can run offline if needed (no tag changes). If the integration test fails, the predicate is wrong (likely a column-rename or table-rename typo) — debug by logging the generated SQL via SQLite trace.
  </action>
  <verify>
    <automated>go build ./... && go test -race ./internal/sync/... -count=1 -timeout=300s && golangci-lint run ./internal/sync/...</automated>
  </verify>
  <done>
    - All 13 OnConflict sites use `sql.UpdateWhere(skipUnchangedPredicate(<table>))`.
    - `skipUnchangedPredicate` helper exists once at the top of upsert.go with documented zero-handling AND same-second-drift caveat.
    - The zero-time guard matches the modernc driver's actual storage representation (verified via the STEP 0 probe; observed form documented in a code comment).
    - `TestUpsert_SkipOnUnchanged` passes all 4 sub-tests (skip / update-on-newer / update-on-zero / insert-when-absent).
    - `internal/sync/integration_test.go` passes (D-30 zero-divergence equivalent; CI-runnable).
    - SEED note created at `.planning/seeds/SEED-XXX-periodic-full-sync-schedule.md` documenting the full-mode convergence escape-hatch.
    - `go test -race ./internal/sync/... -count=1` clean.
    - `golangci-lint run ./internal/sync/...` clean.
    - Atomic commit: `perf(sync): skip ON CONFLICT updates when row is unchanged`.
  </done>
</task>

</tasks>

<verification>
After all 6 tasks land, verify the full chain:

1. `go build ./...` — clean build.
2. `go test -race ./... -count=1 -timeout=600s` — all tests pass, no regressions.
3. `golangci-lint run` — clean.
4. `go generate ./...` — drift gate clean (this plan does not touch ent schemas, codegen should be no-op).
5. Hand-grep for the load-bearing tokens declared in must_haves.key_links:
   - `grep -n "sync-commit" internal/sync/worker.go`
   - `grep -n "sync-finalize\|sync-cursor-updates\|sync-on-complete\|sync-record-status" internal/sync/worker.go`
   - `grep -n "UpsertCursor(ctx, tx" internal/sync/worker.go`
   - `grep -c "sql.UpdateWhere" internal/sync/upsert.go` should equal 13
   - `grep -n "UNION ALL" internal/sync/initialcounts.go`
   - `grep -n "synchronous(NORMAL)" internal/database/database.go`
   - `grep -n "PRAGMA cache_spill" internal/sync/worker.go`
   - `grep -n "cursor_write_caused_rollback" internal/sync/worker.go`
   - `grep -n "poc-count-doubling-halving" internal/sync/initialcounts.go`
   - `grep -n "InitialObjectCounts" cmd/peeringdb-plus/main.go` — both call sites pass *sql.DB
6. Atomic commit log shows 6 commits (one per task), each with a focused subject line.

NO production deploy from this plan — operator (user) deploys manually with `fly deploy` after merge to main and observes the next sync trace in Tempo to confirm:
- post-commit tail < 5s (down from 23.4s)
- sync-commit span has duration <5s
- sync-cursor-updates span (now inside main tx) has duration <100ms
- skip-on-unchanged: pdbplus.sync.type.objects gauges show same row counts but tx commit time drops dramatically
</verification>

<success_criteria>
- All 6 atomic commits land cleanly on a feature branch.
- `go test -race ./...` passes.
- `golangci-lint run` clean across all touched directories.
- `go generate ./...` produces zero drift (codegen pipeline untouched).
- TestWorkerSync_LineBudget green (Sync still under 100 lines — helper extractions in T2 + T4 keep it there).
- `internal/sync/integration_test.go` green (D-30 zero-divergence equivalent, CI-runnable).
- bypass_audit_test.go green (privacy.Allow audit invariant preserved; raw SQL in initialcounts is not a bypass-audit candidate — it doesn't go through ent privacy at all).
- All 13 OnConflict upsert sites use UpdateWhere with the same predicate helper; no copy-paste of the predicate text.
- DSN carries 6 pragmas; per-tx PRAGMA cache_spill present via `prepareTxPragmas` helper.
- `pdbplus.sync.cursor_write_caused_rollback` span attr is set when cursor write causes rollback.
- InitialObjectCounts godoc preserves the POC count doubling/halving cross-reference.
- Future sync trace will show named child spans for the post-commit phase.
</success_criteria>

<output>
After completion, create `.planning/quick/260428-eda-sync-optimization-bundle-spans-cursor-in/260428-eda-SUMMARY.md` with:

- One section per task: what changed, key file diffs, test results.
- A "Production trace expectations" section listing what the operator should see in the next sync trace after deploy (named spans, commit time, atomicity, cursor_write_caused_rollback attr semantics).
- A "Rollback recipe" section: each commit reverts cleanly in reverse order; CHANGE 3 (skip-on-unchanged) is the riskiest and a `git revert` of just that commit cleanly disables the optimisation while leaving the others in place.
- A "Same-second drift" callout pointing to the new SEED note (periodic full-sync schedule).
- Reference back to `.planning/STATE.md` quick-tasks-completed table — append a new row.
</output>
