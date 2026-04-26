---
phase: 75-code-side-observability
plan: 01
subsystem: observability
tags: [otel, prometheus, metrics, cold-start, ent, sqlite, grafana]

# Dependency graph
requires:
  - phase: 19
    provides: pdbplus_data_type_count gauge + InitObjectCountGauges + objectCountCache atomic.Pointer (PERF-02 cached-counts pattern)
  - phase: 65
    provides: asymmetric Fly fleet (1 primary + 7 ephemeral replicas with 5-45s LiteFS cold-sync window — context for the +1-2s startup-cost acceptance)
provides:
  - InitialObjectCounts(ctx, *ent.Client) (map[string]int64, error) — public helper in package sync that runs one-shot Count(ctx) per the 13 PeeringDB entity tables
  - Cold-start primer wired into cmd/peeringdb-plus/main.go between database.Open and pdbotel.InitObjectCountGauges so the OTel ObservableGauge callback reads real values from its first observation
  - 3 unit tests locking the contract (all-13 non-zero on seed, all-13 zero on empty DB, key-parity with worker.syncSteps())
affects:
  - phase: 75-02 (OBS-02 — zero-rate counter pre-warm; same cold-start phase, sibling plan)
  - phase: 75-03 (OBS-04 — http.route middleware; sibling plan)
  - phase: 76 (OBS-03 dashboard hardening — visual confirmation of correctly-populated gauge)
  - phase: 77 (OBS-06 telemetry audit — sync-cycle log entries unchanged, but the new "seeded initial object counts" startup log adds one INFO-level entry per process boot for the auditor to classify)

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Two-primer atomic-cache pattern: startup synchronous Count(ctx) seeds the floor, OnSyncComplete callback refreshes after every cycle. Either path can populate the map; the OTel ObservableGauge callback is agnostic to which primer ran."
    - "Closure-table dispatch over the 13 entity types (counter struct with name + run func) — uniform per-type error wrapping (`count %s: %w`) so failures are grep-able by type name."

key-files:
  created:
    - internal/sync/initialcounts.go (86 lines)
    - internal/sync/initialcounts_test.go (90 lines, 3 sub-tests)
  modified:
    - cmd/peeringdb-plus/main.go (replaced 5-line empty-map prime with 22-line synchronous-seed block; line range ~183-205)

key-decisions:
  - "Counts include ALL rows regardless of status (no `WHERE status IN (ok, pending)` filter), matching the existing OnSyncComplete cache contract — the dashboard wants Phase 68 tombstones (status='deleted') visible in 'Total Objects' until tombstone GC ships (SEED-004 dormant). CONTEXT.md D-01 mentioned the WHERE clause as Django-parity colour, but the cache contract is upstream of that and takes precedence."
  - "Helper exports from package sync (not from internal/otel) so production code has zero test deps — the seed.Full + testutil.SetupClient imports live only in initialcounts_test.go. Avoided extending InitObjectCountGauges signature to take a callback because that would couple the otel package to ent/sqlite for what is logically a sync-package concern (counts of synced entities)."
  - "Fail-fast (slog.Error + os.Exit(1)) on seed error rather than slog.Warn + continue — a Count(ctx) failure here means the DB is in an unexpected state (e.g. LiteFS not yet mounted on a replica boot race) and silently swallowing it would mask a real problem. GO-CFG-1 alignment."

patterns-established:
  - "Per-type Count(ctx) closure-table: when adding a 14th entity type post-Phase 75, add one row to BOTH internal/sync/initialcounts.go's queries slice AND internal/sync/worker.go's syncSteps() — TestInitialObjectCounts_KeyParityWithSyncSteps's len()==13 assertion catches drift on the initial-counts side; TestSyncSteps_AllThirteen (worker_test.go) catches it on the syncSteps side."
  - "Plan-time `os.Exit(1)` count assertions are stale within ~1-2 milestones — the file grows. Future plans should drop count-equality acceptance criteria in favour of structural assertions (e.g. 'new branch is fail-fast, not warn-and-continue')."

requirements-completed: [OBS-01]

# Metrics
duration: 6min
completed: 2026-04-26
---

# Phase 75 Plan 01: Cold-start Gauge Population Summary

**Synchronous one-shot Count(ctx) per entity table at process startup eliminates the ~15-min "no data" window on the pdbplus_data_type_count gauge after every fresh deploy.**

## Performance

- **Duration:** ~6 min
- **Started:** 2026-04-26T23:18:03Z
- **Completed:** 2026-04-26T23:24:03Z
- **Tasks:** 2 (TDD: RED + GREEN for Task 1, plus Task 2 wire-up)
- **Files modified:** 3 (2 created, 1 modified)

## Accomplishments

- New `InitialObjectCounts(ctx, *ent.Client) (map[string]int64, error)` helper in `internal/sync/initialcounts.go` runs 13 sequential `client.X.Query().Count(ctx)` calls and returns a map keyed by PeeringDB type name.
- Wire-up at `cmd/peeringdb-plus/main.go` between `database.Open` (line ~116) and `pdbotel.InitObjectCountGauges` (line ~209) seeds `objectCountCache` so the OTel ObservableGauge callback reads real values from its FIRST observation.
- 3 table-driven unit tests under `-race` lock the contract: all-13-non-zero after `seed.Full`, all-13-explicit-zero on empty DB, key-parity with `worker.syncSteps()`.
- `OnSyncComplete` callback at `cmd/peeringdb-plus/main.go:244-254` is byte-unchanged — sync-completion still primes the cache after every successful cycle. The seed is a FLOOR for the pre-first-sync window, not a replacement.

## Task Commits

Each task was committed atomically:

1. **Task 1 (TDD RED): failing tests for InitialObjectCounts** — `0c8ca6d` (test)
2. **Task 1 (TDD GREEN): InitialObjectCounts helper** — `0eae6a1` (feat)
3. **Task 2: wire seed into main.go before InitObjectCountGauges** — `5ea3e19` (feat)

(Plan metadata + STATE.md commit follows after this SUMMARY.)

## Files Created/Modified

- `internal/sync/initialcounts.go` (CREATED, 86 lines) — public `InitialObjectCounts` helper with package-level doc explaining the OBS-01 cold-start problem and the two-primer pattern.
- `internal/sync/initialcounts_test.go` (CREATED, 90 lines) — 3 sub-tests: `_AllThirteenTypes`, `_EmptyDB`, `_KeyParityWithSyncSteps`. All three are `t.Parallel()`-safe (each test gets its own isolated in-memory SQLite client via `testutil.SetupClient(t)`).
- `cmd/peeringdb-plus/main.go` (MODIFIED, ~22-line block replacing the prior 5-line empty-map prime) — call site is `seededCounts, err := pdbsync.InitialObjectCounts(ctx, entClient)` followed by `objectCountCache.Store(&seededCounts)` and an `slog.LogAttrs` info log recording the type count.

## Decisions Made

See frontmatter `key-decisions`. Summary: (1) counts are status-agnostic to match the existing OnSyncComplete cache contract; (2) helper lives in package `sync` rather than `internal/otel` to keep production code test-dep-free; (3) fail-fast on seed error per GO-CFG-1.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Lint] Detached package comment**

- **Found during:** Task 1 GREEN phase post-implementation lint
- **Issue:** `golangci-lint run ./internal/sync/...` reported `revive: package-comments: package comment is detached` because the long doc-block above `package sync` was separated by a blank line (the planner-supplied template put the package-level rationale ABOVE `package sync`).
- **Fix:** Moved the rationale comment to AFTER the `package sync` declaration, where it now reads as a plain top-of-file comment (still discoverable, no longer linted as a malformed package doc).
- **Files modified:** `internal/sync/initialcounts.go`
- **Verification:** `golangci-lint run ./internal/sync/...` returns `0 issues.` after the fix.
- **Committed in:** `0eae6a1` (Task 1 GREEN commit — fix folded in before commit, not a separate commit)

### Acceptance-Criterion Misalignment (informational, no code change)

**A7 of Task 2** asserted `os.Exit(1)` count `<=12` ("current count is 11 + 1 for new branch"). Actual count post-fix is 14 (the file has grown 13 baseline + 1 new). The structural intent is preserved — the new branch is fail-fast, not warn-and-continue, matching GO-CFG-1. The plan's count baseline was stale; recording as a process note in the patterns-established frontmatter so future plans use structural rather than count-equality assertions.

---

**Total deviations:** 1 auto-fixed (lint) + 1 stale acceptance baseline (no code change required)
**Impact on plan:** Zero scope creep. Both items are housekeeping; the implementation matches the plan's behavioural contract exactly.

## Issues Encountered

None — clean RED → GREEN → wire-up arc. All gates green on first try after the lint fix.

## User Setup Required

None — no environment variables, dashboard changes, or external service configuration introduced by this plan. Manual deploy-time verification (recorded for Phase 75-completion verifier, not for this plan):

- **Post-deploy** (after `fly deploy`): query `count by(type)(pdbplus_data_type_count{service_name="peeringdb-plus"})` in Grafana Cloud Prometheus within 30s of new machine boot. Expect ≥13 distinct `type` labels with non-zero values for the 11+ entities that have rows in production. Some entities (`carrier`, `carrierfac`, `campus`) may legitimately have very small but non-zero counts.

## Next Phase Readiness

- Plan 75-02 (OBS-02 zero-rate counter pre-warm) is independent of this plan and can proceed immediately. They share the cold-start "right after `InitMetrics()`" code region but modify disjoint subsystems: 75-01 touches the gauge cache primer; 75-02 touches the counter pre-warm site.
- Plan 75-03 (OBS-04 http.route middleware) is independent of both other plans.
- Phase 76 OBS-03 (dashboard hardening) has a soft dependency on this plan — the visual confirmation that the gauge populates within 30s is the natural QA for OBS-03's filter-sweep verification.
- Phase 77 OBS-06 (log-level audit) will see one new INFO-level startup log per process boot ("seeded initial object counts"). The auditor should treat this as expected baseline.

## Self-Check: PASSED

- **Files exist:**
  - `internal/sync/initialcounts.go` — FOUND
  - `internal/sync/initialcounts_test.go` — FOUND
  - `.planning/phases/75-code-side-observability/75-01-SUMMARY.md` — FOUND
- **File modified:** `cmd/peeringdb-plus/main.go` — 1 file, +22/-3 (verified via `git diff --stat`)
- **Commits exist in git log:**
  - `0c8ca6d` (test RED) — FOUND
  - `0eae6a1` (feat GREEN) — FOUND
  - `5ea3e19` (feat wire-up) — FOUND

---
*Phase: 75-code-side-observability*
*Completed: 2026-04-26*
