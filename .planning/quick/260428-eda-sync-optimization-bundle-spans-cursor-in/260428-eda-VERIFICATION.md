---
phase: 260428-eda-sync-optimization-bundle
verified: 2026-04-28T11:30:00Z
status: passed
score: 11/11 must-haves verified
overrides_applied: 0
---

# Quick Task 260428-eda: Sync Optimization Bundle Verification Report

**Task Goal:** Drop typical sync wall-time from ~117s to <30s in steady state, AND close the 23s post-commit observability gap. Six discrete changes (DSN pragmas, in-tx cache_spill, UNION ALL counts, observability spans, cursor-in-tx, skip-on-unchanged) bundled to compound their effects.

**Verified:** 2026-04-28
**Status:** passed
**Re-verification:** No — initial verification

**Performance scope reminder:** the wall-time speedup from 117s to <30s requires a production sync trace to validate empirically. The verifier validated **code correctness, test coverage, and invariant preservation only** — not performance numbers. Operator will run `fly deploy` and compare against trace `1c22931e65d639516a6987c848657283` to confirm the speedup.

## Goal Achievement

### Observable Truths

| #  | Truth                                                                                                                                                                  | Status     | Evidence                                                                                                                          |
| -- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------- | --------------------------------------------------------------------------------------------------------------------------------- |
| 1  | DSN pragmas `synchronous(NORMAL)`, `cache_size(-32000)`, `temp_store(MEMORY)` present in DSN                                                                           | VERIFIED   | `internal/database/database.go:44` — all three appended; rationale documented in `Open` godoc lines 27-40                          |
| 2  | PRAGMA `cache_spill = OFF` set inside the main upsert tx                                                                                                                | VERIFIED   | `internal/sync/worker.go:809` inside `prepareTxPragmas` helper; called from `Worker.Sync:605`                                      |
| 3  | 13 OnConflict sites in upsert.go all use `sql.UpdateWhere(predicate)` with strict `>` semantics                                                                         | VERIFIED   | `grep -c sql.UpdateWhere internal/sync/upsert.go = 13`; predicate at lines 74-85 emits `excluded.updated > ` (strict `>`)         |
| 4  | `InitialObjectCounts` returns identical map shape via single UNION ALL query                                                                                            | VERIFIED   | `internal/sync/initialcounts.go:91` takes `*sql.DB`, single `db.QueryContext(ctx, initialCountsQuery)` at line 101; query const at lines 32-46 |
| 5  | Spans `sync-commit`, `sync-finalize`, `sync-cursor-updates`, `sync-on-complete`, `sync-record-status` exist in worker.go                                                | VERIFIED   | `sync-commit:785`, `sync-finalize:854`, `sync-cursor-updates:1140`, `sync-record-status:885`, `sync-on-complete:902`              |
| 6  | UpsertCursor signature now takes `*ent.Tx` (not `*sql.DB`); recordSuccess no longer has the cursor write loop                                                            | VERIFIED   | `internal/sync/status.go:102` `func UpsertCursor(ctx context.Context, tx *ent.Tx, ...)`; `recordSuccess` body (839-906) contains no cursor loop, only the migration comment at 850 |
| 7  | TestWorkerSync_LineBudget passes (Sync() ≤ 100 lines)                                                                                                                  | VERIFIED   | `--- PASS: TestWorkerSync_LineBudget (0.00s)` (Sync ends at line 626 starting from 530 — body within budget)                       |
| 8  | integration_test.go passes — exercises the full sync path including new spans + skip-unchanged                                                                         | VERIFIED   | `--- PASS` for TestFullSyncWithFixtures, TestSyncTombstonesExplicitDeletedRecords, TestSync_TombstonePersistedFromExplicitPayload, TestSyncFKIntegrity_AfterTombstoneCycle, TestSyncIdempotent |
| 9  | VIS-05 sole privacy bypass call site at worker.go:531 unchanged                                                                                                         | VERIFIED   | `worker.go:531: ctx = privacy.DecisionContext(ctx, privacy.Allow)`; TestSyncBypass_SingleCallSite passes                            |
| 10 | Phase 75 OBS-01 D-01 history paragraph preserved in InitialObjectCounts godoc with poc-count-doubling-halving cross-reference                                            | VERIFIED   | `initialcounts.go:73-86` retains "Phase 75 OBS-01 D-01 history" paragraph and cross-reference to `.planning/debug/poc-count-doubling-halving.md` at line 85 |
| 11 | `pdbplus.sync.cursor_write_caused_rollback` attribute set on root span when cursor write causes rollback                                                                | VERIFIED   | `worker.go:1145` `rootSpan.SetAttributes(attribute.Bool("pdbplus.sync.cursor_write_caused_rollback", true))` inside cursor write failure branch |

**Score:** 11/11 truths verified

### Required Artifacts

| Artifact                                  | Expected                                                                                                                                                      | Status     | Details                                                                                                                                |
| ----------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| `internal/database/database.go`           | DSN with synchronous=NORMAL, cache_size=-32000 (32 MB), temp_store=MEMORY                                                                                     | VERIFIED   | Line 44; full DSN combines existing 3 pragmas + new 3                                                                                  |
| `internal/sync/worker.go`                 | Wrapped tx.Commit + recordSuccess in named OTel spans; cursor-writes removed from recordSuccess; per-tx cache_spill=OFF; cursorUpdates threaded into syncUpsertPass | VERIFIED   | `commitWithSpan` helper:784, `prepareTxPragmas` helper:805, `syncUpsertPass(ctx, tx, scratch, _, cursorUpdates)`:1083; cursor-write loop migrated to syncUpsertPass:1141-1149 |
| `internal/sync/upsert.go`                 | 13 OnConflict sites use sql.UpdateWhere(...) to skip-on-unchanged based on the `updated` column                                                               | VERIFIED   | 13/13 sites use the `OnConflict(ConflictColumns(...), ResolveWithNewValues(), UpdateWhere(skipUnchangedPredicate(...)))` form          |
| `internal/sync/upsert_test.go`            | Regression test: TestUpsert_SkipOnUnchanged                                                                                                                   | VERIFIED   | 4 sub-tests: skip / update_on_newer / update_on_zero / insert_when_absent — all PASS                                                    |
| `internal/sync/status.go`                 | `UpsertCursor(ctx, tx *ent.Tx, ...)` with godoc updated                                                                                                       | VERIFIED   | Line 102; godoc 80-101 documents the in-tx semantic, D-19 atomicity, failure-mode shift, cursor_write_caused_rollback attr          |
| `internal/sync/initialcounts.go`          | Single UNION ALL SELECT against the underlying *sql.DB                                                                                                        | VERIFIED   | `const initialCountsQuery` at line 32 (UNION ALL across 13 tables), `db.QueryContext` at line 101                                    |
| `internal/sync/initialcounts_test.go`     | TestInitialCountsQuery_TableNamesMatchSchema                                                                                                                  | VERIFIED   | Line 137; introspects sqlite_master for all 13 expected table names                                                                    |

### Key Link Verification

| From                                                            | To                                                              | Via                                                                                                       | Status   | Details                                                                                          |
| --------------------------------------------------------------- | --------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- | -------- | ------------------------------------------------------------------------------------------------ |
| Worker.Sync (worker.go:618)                                     | tx.Commit()                                                     | wrapped in sync-commit OTel span                                                                          | WIRED    | `commitWithSpan(ctx, tx)` at 618 → `commitWithSpan` opens `sync-commit` span at 785 then `tx.Commit()` |
| syncUpsertPass (worker.go:1140)                                 | UpsertCursor(ctx, tx, ...) inside main tx                       | cursorUpdates parameter threaded through Worker.Sync→syncUpsertPass                                       | WIRED    | Worker.Sync:613 passes `cursorUpdates` to syncUpsertPass; loop at 1141-1149 runs before return    |
| upsert.go (13 functions)                                        | ent CreateBulk(...).OnConflict(...).UpdateWhere(...)            | sql.UpdateWhere(skipUnchangedPredicate(<table>))                                                          | WIRED    | All 13 sites verified by grep; predicate at upsert.go:74-85 emits zero-time guard + strict `>`    |
| cmd/peeringdb-plus/main.go (startup primer + OnSyncComplete)    | InitialObjectCounts                                             | both call sites pass *sql.DB (`db`, not `entClient`)                                                      | WIRED    | main.go:199 (startup primer) and main.go:289 (OnSyncComplete callback) both pass `db`             |
| internal/database/database.go:44 DSN                            | modernc.org/sqlite pragma parser                                | &_pragma=synchronous(NORMAL)&_pragma=cache_size(-32000)&_pragma=temp_store(MEMORY)                        | WIRED    | DSN string includes all three new pragmas; existing tests pass against modernc                    |

### Behavioral Spot-Checks

| Behavior                                                            | Command                                                                                                  | Result                                                              | Status |
| ------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------- | ------ |
| Build clean                                                         | `go build ./...`                                                                                         | (empty stdout/stderr — exit 0)                                      | PASS   |
| Targeted regression tests pass                                      | `go test -race -run TestWorkerSync_LineBudget\|TestUpsert_SkipOnUnchanged\|TestInitialObjectCounts\|TestInitialCountsQuery_TableNamesMatchSchema\|TestUpsertCursor_*\|TestSyncBypass_SingleCallSite ./internal/sync/...` | All listed sub-tests PASS                                            | PASS   |
| Full sync test suite green                                          | `go test -race ./internal/sync/... ./internal/database/...`                                              | `ok internal/sync 12.792s`, `ok internal/database 1.075s`           | PASS   |
| Full repo test suite green                                          | `go test -race ./...`                                                                                    | All packages PASS, no FAIL output                                    | PASS   |
| Lint clean                                                          | `golangci-lint run ./internal/sync/... ./internal/database/... ./cmd/peeringdb-plus/...`                 | `0 issues.`                                                          | PASS   |
| 13 sql.UpdateWhere call sites                                       | `grep -c sql.UpdateWhere internal/sync/upsert.go`                                                        | `13`                                                                | PASS   |
| Six expected commit subjects in git log                             | `git log --oneline -10`                                                                                  | dd1514f, 5646d75, eaabb00, 88e7fb8, 76bb885, 8a9b5cb above 73b7efd  | PASS   |

### Anti-Patterns Found

None. No TODO/FIXME/PLACEHOLDER markers in the touched files; no stub `return nil` or `return []` in implementation paths; no console.log-only handlers; no hardcoded empty data flowing to user-visible output.

### Invariants Preserved

- **TestWorkerSync_LineBudget** — Worker.Sync stays under 100 lines despite +1 line for `commitWithSpan` (T2's helper extraction net -1 line offsets it; T5 keeps Sync at 97 lines per SUMMARY).
- **D-19 atomicity** — sync_status row update remains a separate raw-SQL Exec (outcome record, by design); cursor writes folded INTO main tx (the bug-fix); ent upserts atomic with cursor advance now.
- **D-30 zero-divergence** — CI-runnable equivalent (integration_test.go) passes; replay_snapshot_test.go is build-tagged offline_replay and intentionally not exercised.
- **VIS-05 single privacy bypass** — `TestSyncBypass_SingleCallSite` passes; `privacy.DecisionContext(ctx, privacy.Allow)` only at worker.go:531.
- **InitialObjectCounts dropping `privctx.WithTier(ctx, TierUsers)`** — intentional and safe per CHANGE 6: raw SQL bypasses ent's Privacy policy entirely (no Privacy Hook fires on `db.QueryContext`), so the tier elevation is no longer needed; POC count parity preserved (TestInitialObjectCounts_PocPolicyBypass passes).
- **No proto/codegen drift** — only call patterns changed; ent schemas untouched.

### Human Verification Required

None for code-correctness verification scope. The wall-time performance claim (117s → <30s) requires a production sync trace post-deploy and is **explicitly outside this verifier's scope** (per task instructions). Operator action item:

- After `fly deploy`, observe the next Tempo trace and confirm:
  - `sync-commit` span duration < 5s in steady state
  - `sync-cursor-updates` span (now inside main tx) duration < 100ms
  - No >2s gap between last `sync-upsert-*` span and root span end
  - Steady-state cycles where zero rows changed: tx commit duration drops dramatically (predicate skips no-op rewrites)

This is acceptance-test / observability work, not code verification.

### Gaps Summary

No gaps. All 11 must-have truths verified against the codebase, all 7 required artifacts present and substantive, all 5 key links wired correctly, all behavioral spot-checks pass, no anti-patterns found.

The implementation matches the PLAN's must_haves frontmatter precisely — including the load-bearing details: helper extractions (`commitWithSpan`, `prepareTxPragmas`) for line-budget compliance, the strict `>` predicate semantic with documented same-second-drift trade-off and SEED-005 escape-hatch, the empirically-verified zero-time guard (`<= '1900-01-01'` matches modernc's TEXT representation of `time.Time{}`), the rollback-attr stamping for postmortem visibility in Tempo, and the godoc cross-reference preservation for the POC count doubling/halving incident.

The only deviations from the PLAN (documented in SUMMARY § Deviations) are:
1. Updating 7 worker tests + 3 integration test fixtures to bump `updated` on second-pass payloads (necessary contract change once skip-on-unchanged lands; matches real PeeringDB behaviour). Each touched site carries a `260428-eda CHANGE 3:` comment for future grep-ability.
2. The Sync orchestrator note was slimmed (3-line cross-reference) and the long failure-mode-shift documentation moved to syncUpsertPass godoc — necessary to keep Sync under the 100-line `TestWorkerSync_LineBudget` cap. The semantic content is preserved, just relocated.

Both deviations are correct and necessary; neither weakens the goal.

---

_Verified: 2026-04-28T11:30:00Z_
_Verifier: Claude (gsd-verifier)_
