---
phase: 68-status-since-matrix
plan: 01
subsystem: sync
tags: [config, sync, deprecation, env-var, soft-delete-prep]

# Dependency graph
requires:
  - phase: 67-default-ordering-flip
    provides: pdbcompat/sync baseline with compound ordering; unchanged for 68-01 but sets the commit-clean baseline
provides:
  - PDBPLUS_INCLUDE_DELETED removed from Config struct with slog.Warn grace-period shim
  - WorkerConfig.IncludeDeleted field gone; syncIncremental[E] no longer carries an includeDeleted parameter
  - filterByStatus[E] generic helper + 244-line filter_test.go deleted
  - 13 dispatchScratchChunk case branches now call syncIncremental without the trailing includeDeleted argument
  - Unified sync upsert path unconditionally persists status=deleted rows; hard-delete pass still runs (Plan 68-02 flips it)
  - Docs table entry removed from README.md / CLAUDE.md / docs/CONFIGURATION.md with an explicit "Removed in v1.16" migration note
affects: [68-02-soft-delete-flip, 68-03-status-matrix-pdbcompat, 68-04-changelog]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Deprecation WARN pattern for removed env vars: os.Getenv check + slog.Warn with slog.String('value', v) attribute, gated by non-empty check, gosec G706 suppression documented via threat register entry"
    - "Test pattern: slog.NewTextHandler(&buf, ...) + slog.SetDefault swap + t.Cleanup restore for sequential WARN-log capture (non-parallel subtests because slog.SetDefault is process-global)"

key-files:
  created: []
  modified:
    - internal/config/config.go
    - internal/config/config_test.go
    - internal/sync/worker.go
    - cmd/peeringdb-plus/main.go
    - internal/sync/integration_test.go
    - internal/sync/worker_test.go
    - internal/sync/nokey_sync_test.go
    - internal/sync/replay_snapshot_test.go
    - internal/sync/worker_bench_test.go
    - internal/sync/testdata/refactor_parity.golden.json
    - docs/CONFIGURATION.md
    - docs/TESTING.md
    - README.md
    - CLAUDE.md

key-decisions:
  - "Test fixture assertions updated from 2 -> 3 orgs in TestFullSyncWithFixtures / TestSyncDeletesStaleRecords: the upsert path now persists status=deleted upstream rows (the plan's 'assertion set unchanged' note was literally impossible — filter removal changes the observed row count). The hard-delete pass still runs, so subsequent syncs that drop a row from the fixture still cleanly reduce the count (e.g. TestSyncDeletesStaleRecords second sync still ends at 1 org)."
  - "Deleted TestSyncFilterDeletedObjects entirely — both subtests (exclude_deleted / include_deleted) assert the now-removed filter's semantics. Intent is preserved by TestSyncPersistsDeletedRowsUnconditional (the renamed TestSyncIncludeDeleted)."
  - "newTestWorker and newTestWorkerWithMode helpers had their includeDeleted bool parameter removed outright (not just the field initializer) — leaving an unused param would have tripped unparam/unused linters, and 23 call sites collapsing from 'newTestWorker(t, f, false)' to 'newTestWorker(t, f)' is mechanical."
  - "Regenerated internal/sync/testdata/refactor_parity.golden.json to add the newly-persisted org 3 tombstone row. Golden file regeneration via 'go test ./internal/sync -update -run TestSync_RefactorParity' — flag is -update (not --update) per the test harness."
  - "Added //nolint:gosec G706 suppression on the slog.Warn call with rationale tied to threat register T-68-01-03: PDBPLUS_INCLUDE_DELETED is a boolean flag with no PII/credential content, value is attached as a structured slog attribute (not interpolated into the message), and slog.String quotes the value on output."

patterns-established:
  - "Env-var removal grace-period: log-and-ignore in vN, fatal startup error in vN+1 via one-line slog.Warn → return nil, errors.New(...) swap. Documented in the Config Load() comment at the removal site so the v1.17 follow-up is a grep-able pin."
  - "Docs CONFIGURATION.md convention: 'Removed in vX.Y' table replaces the retired variable row with columns (Variable, Removed in, Replacement, Migration). Keeps the migration instruction in the operator-facing doc rather than burying it in CHANGELOG."

requirements-completed:
  - STATUS-05

# Metrics
duration: 32min
completed: 2026-04-19
---

# Phase 68 Plan 01: Remove PDBPLUS_INCLUDE_DELETED wiring Summary

**PDBPLUS_INCLUDE_DELETED removed from Config/WorkerConfig/syncIncremental with slog.Warn-and-ignore grace period; filterByStatus helper + its 244-line test file deleted; all test fixtures and docs updated for the unconditional-upsert baseline that Plan 68-02 will soft-delete-flip.**

## Performance

- **Duration:** ~32 min
- **Started:** 2026-04-19T13:36Z (approx)
- **Completed:** 2026-04-19T14:08Z (approx)
- **Tasks:** 3
- **Files modified:** 14 (plus 1 file deletion: `internal/sync/filter_test.go`)

## Accomplishments
- `Config.IncludeDeleted` field removed; the `parseBool` block in `Load()` is replaced with a slog.Warn shim that logs and ignores the legacy env var during the v1.16 → v1.17 grace period
- `WorkerConfig.IncludeDeleted` field removed; `syncIncremental[E]` signature loses its trailing `includeDeleted bool` parameter; `filterByStatus[E]` generic helper deleted alongside its 244-line `filter_test.go`
- 13 `dispatchScratchChunk` case branches updated to drop the leading `includeDeleted := w.config.IncludeDeleted` read and the trailing `includeDeleted` argument on every `syncIncremental(...)` call
- Test fixtures across `integration_test.go`, `worker_test.go`, `nokey_sync_test.go`, `replay_snapshot_test.go`, `worker_bench_test.go` stripped of `IncludeDeleted: true/false` field initializers; the two test helpers (`newTestWorker`, `newTestWorkerWithMode`) lost their `includeDeleted bool` parameter outright at all 23 call sites
- `TestSyncIncludeDeleted` renamed to `TestSyncPersistsDeletedRowsUnconditional` with an updated assertion comment (Plan 68-02 will rewrite this entirely as `TestSync_SoftDeleteMarksRows`)
- `docs/CONFIGURATION.md` gained a `### Removed in v1.16` block with migration instructions; `README.md`, `CLAUDE.md`, `docs/TESTING.md` no longer list the env var in active-config tables

## Task Commits

Each task was committed atomically:

1. **Task 1: Remove IncludeDeleted from Config + emit deprecation WARN** — `d6c8922` (refactor)
2. **Task 2: Remove IncludeDeleted wiring from sync worker + delete filterByStatus helper** — `055f606` (refactor)
3. **Task 3: Strip IncludeDeleted fixtures from sync test files + intermediate rename + docs** — `f8a1205` (refactor)

## Files Created/Modified

- `internal/config/config.go` — dropped `Config.IncludeDeleted`; added deprecation slog.Warn block with gosec G706 suppression + comment tying it to threat register T-68-01-03; added `log/slog` import
- `internal/config/config_test.go` — replaced `TestLoad_IncludeDeleted` with `TestLoad_IncludeDeleted_Deprecated` (slog.SetDefault capture + two non-parallel subtests); added `bytes` + `log/slog` imports
- `internal/sync/worker.go` — removed `WorkerConfig.IncludeDeleted`, `filterByStatus[E]` helper, and the `includeDeleted` parameter + filter branch from `syncIncremental[E]`; stripped the trailing `, includeDeleted` argument from 13 `dispatchScratchChunk` call sites; refreshed stale "unless IncludeDeleted is set" comments in `syncUpsertPass` and `drainAndUpsertType`
- `internal/sync/filter_test.go` — **deleted** (244 lines); the file's self-assertion at line 244 (`t.Fatalf("filter.go must not exist ...")`) is satisfied by its own removal
- `cmd/peeringdb-plus/main.go` — removed `IncludeDeleted: cfg.IncludeDeleted,` from the `sync.NewWorker(..., sync.WorkerConfig{...})` call site
- `internal/sync/integration_test.go` — stripped `IncludeDeleted: false/true` initializers from 5 `sync.WorkerConfig{}` literals; renamed `TestSyncIncludeDeleted` to `TestSyncPersistsDeletedRowsUnconditional` with comment/assertion updates; bumped `TestFullSyncWithFixtures` organizations-count from 2 to 3 and updated the preamble comment; bumped `TestSyncDeletesStaleRecords` first-sync assertion from 2 to 3
- `internal/sync/worker_test.go` — removed `includeDeleted bool` parameter from `newTestWorker` + `newTestWorkerWithMode`; collapsed 23 call sites; stripped `IncludeDeleted: false` from 3 direct `WorkerConfig` literals; deleted `TestSyncFilterDeletedObjects` entirely (tested the removed filter)
- `internal/sync/nokey_sync_test.go`, `internal/sync/replay_snapshot_test.go`, `internal/sync/worker_bench_test.go` — each dropped the single `IncludeDeleted: ...` line from their `WorkerConfig{}` literal
- `internal/sync/testdata/refactor_parity.golden.json` — regenerated via `go test ./internal/sync -update -run TestSync_RefactorParity`; adds the newly-persisted org 3 (`Defunct Telecom`, `status=deleted`) tombstone row
- `docs/CONFIGURATION.md` — removed the `PDBPLUS_INCLUDE_DELETED` row from the Sync Worker env var table; added a new `### Removed in v1.16` subsection with a migration table
- `docs/TESTING.md` — removed the `IncludeDeleted: false,` line from the `sync.WorkerConfig` code example
- `README.md` — removed the `PDBPLUS_INCLUDE_DELETED` row from the environment variables table
- `CLAUDE.md` — removed the `PDBPLUS_INCLUDE_DELETED` row from the environment variables table

## Decisions Made

- **Test assertion counts updated (plan said "unchanged"):** `TestFullSyncWithFixtures` and `TestSyncDeletesStaleRecords` first-sync assertions for organizations moved from 2 to 3. The plan's `<important_constraints>` claimed "the assertion set is unchanged from pre-phase except the renamed test" — this was literally impossible: removing the `filterByStatus` branch means org 3 (`status=deleted`) now enters the DB via upsert, and the hard-delete pass keeps it because its ID is still in `remoteIDs`. `TestSyncDeletesStaleRecords` second-sync assertion (1 org after stale delete) remains unchanged because the fixture replacement removes org 3 from the remote, so the hard-delete pass reconciles.
- **`TestSyncFilterDeletedObjects` deleted:** The `exclude_deleted` subtest asserted 1 org with the filter active; the `include_deleted` subtest asserted 2 orgs with the filter bypassed. Both subtests now produce 2 orgs (no filter exists), making the "exclude_deleted" assertion stale and the whole test semantically obsolete. Intent is preserved by `TestSyncPersistsDeletedRowsUnconditional`.
- **Helper parameter removed, not just field initializer:** `newTestWorker(t, f, includeDeleted bool)` and `newTestWorkerWithMode(t, baseURL, mode, includeDeleted bool)` lost their trailing bool parameter outright. An unused parameter would have tripped `unparam`/`unused` linters on the next CI run, and 23 mechanical call-site collapses are preferable to lint suppressions.
- **Golden file regeneration:** `internal/sync/testdata/refactor_parity.golden.json` grew by ~24 lines to include the org 3 tombstone. Regenerated via `go test ./internal/sync -update -run TestSync_RefactorParity` (single-hyphen `-update`, not the double-dash flag the failing test message suggests).
- **gosec G706 suppression with threat-register reference:** The `slog.Warn` call annotates with `//nolint:gosec // G706: boolean flag, structured attr, threat register T-68-01-03`. PDBPLUS_INCLUDE_DELETED is a boolean flag (`true`/`false`) with no PII; the value is attached as a structured `slog.String("value", v)` attribute (not interpolated into the format string); `slog.String` quotes the output. Matches the plan's threat register T-68-01-03 disposition.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Updated fixture-count assertions from 2 to 3 in two integration tests**
- **Found during:** Task 3 (`go test -race ./internal/sync` after stripping `IncludeDeleted` initializers)
- **Issue:** The plan claimed "`go test -race ./...` passes (full suite green; ... assertion set is unchanged from pre-phase except the renamed test)". Not true: removing the `filterByStatus` call means `status=deleted` rows now enter the DB via upsert, bumping `TestFullSyncWithFixtures`' org count from 2 to 3 and `TestSyncDeletesStaleRecords`' first-sync org count from 2 to 3.
- **Fix:** Updated both assertions + their leading comments to state the new unconditional-upsert behavior; kept `TestSyncDeletesStaleRecords`' second-sync assertion at 1 (correct: the fixture-replacement drops org 3 from remoteIDs and the hard-delete pass reconciles).
- **Files modified:** `internal/sync/integration_test.go`
- **Verification:** Full sync test suite passes (`go test ./internal/sync -race -count=1` → `ok`).
- **Committed in:** `f8a1205` (Task 3 commit)

**2. [Rule 1 - Bug] Deleted semantically-obsolete TestSyncFilterDeletedObjects**
- **Found during:** Task 3 (after dropping the `includeDeleted` parameter from `newTestWorker`, the `t.Run("exclude_deleted")` subtest's 1-org assertion could never pass)
- **Issue:** Test asserts the behaviour of the now-removed `filterByStatus` helper. `exclude_deleted` wanted 1 org (deleted filtered out); `include_deleted` wanted 2 orgs (deleted included). Both now produce 2 orgs because the filter is gone.
- **Fix:** Deleted the entire test function (44 lines). Intent is preserved by the renamed `TestSyncPersistsDeletedRowsUnconditional` in `integration_test.go`.
- **Files modified:** `internal/sync/worker_test.go`
- **Verification:** `go test ./internal/sync -race -count=1 -run 'TestSyncFilterDeletedObjects'` reports "no tests to run"; the rest of the suite stays green.
- **Committed in:** `f8a1205` (Task 3 commit)

**3. [Rule 1 - Bug] Regenerated internal/sync/testdata/refactor_parity.golden.json**
- **Found during:** Task 3 (`go test ./internal/sync -race` surfaced `TestSync_RefactorParity` failure: `got 15171 bytes, want 14561 bytes`)
- **Issue:** The golden file captured the full-suite DB dump with the old filter behaviour (2 orgs). Post-plan the dump has 3 orgs + the org 3 tombstone row.
- **Fix:** Regenerated via `go test ./internal/sync -update -count=1 -run TestSync_RefactorParity`. Diff is an additive insert of the org 3 row — no other rows shifted.
- **Files modified:** `internal/sync/testdata/refactor_parity.golden.json`
- **Verification:** `go test ./internal/sync -race -count=1 -run TestSync_RefactorParity` now passes.
- **Committed in:** `f8a1205` (Task 3 commit)

**4. [Rule 2 - Missing Critical] Added gosec G706 nolint with threat-register rationale**
- **Found during:** Pre-commit lint check after Task 3 (`golangci-lint run` reported `G706: Log injection via taint analysis` on the deprecation slog.Warn)
- **Issue:** gosec flags env-var values used as slog attributes as tainted input even when attached via the structured `slog.String(...)` helper (which quotes output and is not vulnerable to log-injection the way `%s` format interpolation is).
- **Fix:** Added `//nolint:gosec // G706: boolean flag, structured attr, threat register T-68-01-03` with a multi-line rationale comment immediately above the `slog.Warn` call. Rationale matches the plan's threat register T-68-01-03 `accept` disposition verbatim.
- **Files modified:** `internal/config/config.go`
- **Verification:** `golangci-lint run --timeout 180s ./internal/config/... ./internal/sync/... ./cmd/peeringdb-plus/...` → `0 issues`.
- **Committed in:** `f8a1205` (Task 3 commit — rolled into docs/test cleanup rather than amending Task 1 per the project git protocol "never amend existing commits").

**5. [Rule 1 - Bug] Removed `includeDeleted` parameter from the two test-only factory helpers**
- **Found during:** Task 3 (keeping the field initializer approach suggested by the plan would have left the helper's bool parameter unused at all 23 call sites)
- **Issue:** The plan said "DELETE the `IncludeDeleted: true/false` line from the `sync.WorkerConfig{...}` struct literal. Do NOT edit logic, only remove the field initializer." But inside `newTestWorker`, the removed initializer was `IncludeDeleted: includeDeleted` — keeping only the parameter with no consumer would either trip `unparam`/`unused` linters on the next CI run or force a lint suppression.
- **Fix:** Removed the `includeDeleted bool` parameter from both `newTestWorker(t, f, includeDeleted bool)` and `newTestWorkerWithMode(t, baseURL, mode, includeDeleted bool)`; collapsed all 23 call sites (`newTestWorker(t, f, false)` → `newTestWorker(t, f)`, etc.) via `replace_all` edits.
- **Files modified:** `internal/sync/worker_test.go`
- **Verification:** `go vet ./internal/sync/... && go test ./internal/sync -race -count=1` → both green.
- **Committed in:** `f8a1205` (Task 3 commit)

---

**Total deviations:** 5 auto-fixed (4 Rule 1 — fixes to stale assertions/goldens/parameters, 1 Rule 2 — missing lint suppression + threat-register anchor)
**Impact on plan:** All auto-fixes are mechanical consequences of the plan's actual semantics (removing the filter changes observed row counts; removing a field initializer that was the only consumer of a parameter leaves the parameter unused). Plan text understated these as "assertion set unchanged" — the ripples were minor but required for a green suite. No scope creep beyond what Phase 68 D-01 explicitly prescribes.

## Issues Encountered

- `TestSync_RefactorParity` failure after test-file edits: resolved by regenerating the golden (see Deviation #3). Flag is `-update` (single hyphen), not `--update` — the failure message suggests `go test -update ./internal/sync/...` which `go test` interprets as "run from package path `.`" and fails with `no Go files in /home/dotwaffle/Code/pdb/peeringdb-plus`. Correct invocation: `go test ./internal/sync -update -run TestSync_RefactorParity`.

## User Setup Required

None — no external service configuration required. The deprecation WARN is purely informational; operators still running with `PDBPLUS_INCLUDE_DELETED=...` will see a single-line log at startup and the rest of Load() proceeds normally.

## Next Phase Readiness

Plan 68-01 delivers STATUS-05 (env var removed from Config; sync no longer branches on it). Pre-conditions for the next two plans:

- **Plan 68-02 (soft-delete flip):** 13 `deleteStale*` functions in `internal/sync/delete.go` get renamed to `markStaleDeleted*` and rewritten to `UPDATE ... SET status='deleted', updated=cycleStart WHERE id NOT IN (...)`. The plan-02 test `TestSync_SoftDeleteMarksRows` will replace the intermediate `TestSyncPersistsDeletedRowsUnconditional` renamed here. Because hard-delete still runs after Plan 68-01, there is a one-cycle semantic window where deleted upstream rows are upserted → immediately hard-deleted — net effect equivalent to pre-Phase-68 behaviour, so no functional regression in the interval between the two plans landing.
- **Plan 68-03 (status × since matrix in pdbcompat):** New `applyStatusMatrix(isCampus, sinceSet)` helper in `internal/pdbcompat/filter.go`; 13 closures in `registry_funcs.go` gain the predicate; pk paths in `depth.go` admit `(ok, pending)`; `limit=0` fix in `response.go`. None of these touch sync or config paths — 68-01's changes are sync-side only.
- **Plan 68-04 (CHANGELOG.md bootstrap):** First root-level CHANGELOG entry will include the `PDBPLUS_INCLUDE_DELETED` removal under `### Breaking`. That is 68-04's scope, not 68-01's.

No blockers for the rest of Phase 68.

---
*Phase: 68-status-since-matrix*
*Completed: 2026-04-19*

## Self-Check: PASSED

- FOUND: internal/config/config.go (deprecation WARN at line 183-192)
- FOUND: internal/config/config_test.go (TestLoad_IncludeDeleted_Deprecated at line 242)
- FOUND: internal/sync/worker.go (syncIncremental signature without includeDeleted param; filterByStatus gone)
- ABSENT: internal/sync/filter_test.go (deleted)
- FOUND: cmd/peeringdb-plus/main.go (no IncludeDeleted field in NewWorker call)
- FOUND: internal/sync/integration_test.go (TestSyncPersistsDeletedRowsUnconditional at line 367)
- FOUND: docs/CONFIGURATION.md (### Removed in v1.16 section at line 44)
- FOUND: commit d6c8922 (Task 1)
- FOUND: commit 055f606 (Task 2)
- FOUND: commit f8a1205 (Task 3)
- PASS: `go build ./...`
- PASS: `go vet ./...`
- PASS: `go test -race ./... -count=1` (full suite green)
- PASS: `golangci-lint run ./internal/config/... ./internal/sync/... ./cmd/peeringdb-plus/...` (0 issues)
