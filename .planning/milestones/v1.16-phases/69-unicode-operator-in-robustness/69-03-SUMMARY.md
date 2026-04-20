---
phase: 69
plan: 03
subsystem: sync
tags:
  - sync
  - upsert
  - unicode
  - shadow-columns
dependency-graph:
  requires:
    - 69-01 (internal/unifold package — provides Fold() called by the 6 upsert funcs)
    - 69-02 (ent schemas — provides SetNameFold/SetAkaFold/SetCityFold/SetNameLongFold setters)
  provides:
    - 16 `unifold.Fold()` calls across 6 upsert funcs in internal/sync/upsert.go
    - golang.org/x/text promoted from `// indirect` to direct dependency
    - End-to-end round-trip test anchoring the contract: `Name: "Zürich GmbH"` → DB → `NameFold: "zurich gmbh"`
  affects:
    - Plan 69-04 (pdbcompat filter will route WHERE clauses to *_fold columns populated by this plan)
    - First post-deploy sync cycle (within 1h of v1.16 deploy) backfills all rows per D-03
tech-stack:
  added:
    - golang.org/x/text v0.36.0 (promoted from indirect; already present in module tree via transitive dep)
  patterns:
    - "TDD RED→GREEN cycle: test at plan's own package level (internal/sync) rather than black-box from caller"
    - "Builder-chain insertion order: put _fold setters at the end of the chain immediately before the trailing nillable-field setters to keep grep-able 4-line blocks per entity"
    - "OnConflictColumns().UpdateNewValues() implicitly covers the new columns — no OnConflict clause changes needed (confirmed in all 6 funcs)"
key-files:
  created:
    - internal/sync/upsert_test.go (TestUpsertPopulatesFoldColumns — 118 lines)
    - .planning/phases/69-unicode-operator-in-robustness/69-03-SUMMARY.md
  modified:
    - internal/sync/upsert.go (+1 import, +16 .SetXxxFold() lines across 6 funcs)
    - go.mod (golang.org/x/text moved from indirect to direct require block)
    - go.sum (updated by `go mod tidy`)
    - internal/sync/testdata/refactor_parity.golden.json (regenerated — _fold columns now populated in post-sync DB snapshot, +362 bytes)
decisions:
  - "Regenerated refactor_parity.golden.json as a legitimate expected diff — the plan's verification path `go test -race ./internal/sync/...` flagged this immediately, and the golden update is the documented regeneration path (`-update` flag). Alternative of patching golden manually would bypass the worker's actual output and defeat the test's purpose."
  - "Builder-chain insertion placed the _fold setters immediately after SetStatus(...) rather than after SetCity(...) (as the plan's rationale section suggested). This keeps all 16 SetXxxFold lines grep-able as a trailing block per entity and preserves the existing b.SetNillableLogo(...)/b.SetNillableLatitude(...)/b.SetNillableLongitude(...) pattern that sits on its own line group after the method-chain return."
  - "Idempotency sub-test in TestUpsertPopulatesFoldColumns proves the OnConflict().UpdateNewValues() path rewrites _fold columns on re-sync (not just initial create) — explicitly covers must_haves truth #4."
requirements-completed:
  - UNICODE-01
metrics:
  duration: 3m47s
  completed: 2026-04-19
---

# Phase 69 Plan 03: Populate _fold columns in 6 sync upserts

**Extended the 6 upsert functions for entities with shadow `*_fold` columns (network, facility, internetexchange, organization, campus, carrier) to call `unifold.Fold()` on 16 searchable text fields at sync time, closing the UNICODE-01 data-population path and enabling Plan 69-04 to route pdbcompat `__contains`/`__startswith` WHERE clauses to the pre-folded columns.**

## Performance

- **Duration:** 3m47s
- **Started:** 2026-04-19T16:06:20Z
- **Completed:** 2026-04-19T16:10:07Z
- **Tasks:** 2 (RED test, GREEN impl)
- **Files modified:** 4 (1 created, 3 modified) + 1 regenerated golden

## Accomplishments

- 16 `unifold.Fold(x.Field)` calls inserted into 6 upsert builder closures
  (exact counts: organization 3, network 3, facility 3, internetexchange 4,
  campus 1, carrier 2).
- `TestUpsertPopulatesFoldColumns` anchors the contract with a round-trip
  assertion (`Zürich` → `zurich`, `Straße` → `strasse`, `Köln` → `koln`)
  plus an idempotency sub-test that re-upserts the same row with ASCII
  variants and asserts the fold values are re-computed identically.
- `golang.org/x/text` promoted from `// indirect` to the direct require
  block — reflects the fact that the runtime sync hot-path now depends on
  it through `internal/unifold`.
- 7 unaffected upsert funcs (poc, ixlan, ixprefix, ixfacility,
  networkfacility, networkixlan, carrierfacility) byte-identical to
  pre-plan — confirmed via `git diff` inspection; the only additions in
  the diff are `SetXxxFold(unifold.Fold(...))` calls in the 6 expected
  functions.

## Task Commits

Each task was committed atomically per TDD discipline:

1. **Task 1: Write failing test (RED)** — `8ce16ab` (test)
2. **Task 2: Extend 6 upsert functions (GREEN)** — `cdad023` (feat)

_Plan metadata commit for this SUMMARY + STATE + ROADMAP updates lands
after self-check._

## Files Created/Modified

- `internal/sync/upsert_test.go` — NEW. `TestUpsertPopulatesFoldColumns`
  exercises `upsertOrganizations` end-to-end: builds 1 organization with
  German-language `Name/Aka/City`, opens a tx, upserts, commits, reads
  back via `client.Organization.Get(ctx, 1)`, asserts the 3 fold columns
  were populated. Then re-upserts the same ID with ASCII variants and
  asserts idempotency.
- `internal/sync/upsert.go` — MODIFIED. `+1` import line
  (`internal/unifold`), `+16` `.SetXxxFold(unifold.Fold(x.Field))` lines
  across `upsertOrganizations`, `upsertCampuses`, `upsertFacilities`,
  `upsertCarriers`, `upsertInternetExchanges`, `upsertNetworks`. The
  other 7 upsert funcs are byte-identical to their pre-plan form.
- `go.mod` — MODIFIED. `golang.org/x/text v0.36.0` moved from the
  indirect block (line ~194) into the direct require block (now line 39),
  next to the other `golang.org/x/*` dependencies.
- `go.sum` — MODIFIED by `go mod tidy`. No hash changes, only attribution
  changes.
- `internal/sync/testdata/refactor_parity.golden.json` — REGENERATED via
  `go test -update -run TestSync_RefactorParity`. The post-sync DB snapshot
  now carries non-empty `_fold` values for all rows that have non-empty
  `Name/Aka/City/NameLong` in the fixture set, yielding +362 bytes vs
  pre-plan baseline.

## Decisions Made

**D-03-01 — Builder-chain insertion point: trailing.** The plan's
rationale section suggested inserting `_fold` setters AFTER `SetCity(...)`
and BEFORE `SetStatus(...)`. I placed them immediately after
`SetStatus(...)`, at the trailing end of the main method chain, for two
reasons: (1) groups the 16 new lines as grep-able trailing blocks per
entity, and (2) preserves the pre-existing `b.SetNillableLogo(...)` /
`b.SetNillableLatitude(...)` / `b.SetNillableLongitude(...)` pattern that
intentionally sits on separate lines after the method-chain `return b`
flow. Functionally equivalent — the order of `.SetXxx()` calls on an ent
builder is commutative with respect to the persisted row.

**D-03-02 — No OnConflict changes needed.** All 6 modified functions
already call `.OnConflictColumns(xxx.FieldID).UpdateNewValues()`, which
ent documents as "on conflict, update every settable field from the
incoming values." This implicitly covers the new `_fold` columns —
verified empirically by the `TestUpsertPopulatesFoldColumns` idempotency
sub-test (re-upsert with a different-case `Name` rewrites `NameFold` to
match). No hand-rolled `.Update(...)` predicate needed.

**D-03-03 — Golden regeneration as Rule 1 bug fix.** `TestSync_RefactorParity`
failed after Task 2 because the post-sync DB snapshot now carries populated
`_fold` columns that were empty in the pre-plan baseline. This is the
CORRECT new behavior — the golden was encoding a bug (empty folds) that
Plan 69-03 exists to fix. Ran `go test -race ./internal/sync/... -update
-run TestSync_RefactorParity` per the test's own error-message instructions,
which re-wrote the golden. Cleaner than cherry-editing JSON by hand; the
flag exists precisely for this case.

## Deviations from Plan

**1. [Rule 1 - Bug] Regenerated `internal/sync/testdata/refactor_parity.golden.json`**

- **Found during:** Task 2 verification (`go test -race ./internal/sync/...`).
- **Issue:** `TestSync_RefactorParity` failed: `parity drift: got 16302
  bytes, want 15940 bytes` — the golden encoded empty `_fold` columns
  from Plan 69-02's auto-migrate default, but Task 2's implementation
  (correctly) now populates them.
- **Fix:** Re-ran the worker with `-update` flag as documented in the
  test's own error message, which wrote the new 16302-byte golden
  reflecting the correct post-implementation DB state.
- **Files modified:** `internal/sync/testdata/refactor_parity.golden.json`.
- **Verification:** Re-ran `go test -race ./internal/sync/...` — all
  pass. No other tests touched this file.
- **Committed in:** `cdad023` (part of Task 2's atomic commit alongside
  the upsert.go change that required it).

The plan's `<verification>` section did not anticipate this golden
regeneration because it pre-dates Plan 69-02's landed auto-migrate.
Recording as a Rule 1 auto-fix for the record; the change is the
smallest and most local resolution — alternatives (patching golden
manually, excluding `_fold` columns from the parity snapshot) would
either bypass the worker's actual behavior or weaken the parity check.

---

**Total deviations:** 1 auto-fixed (Rule 1 — test golden drift).

**Impact on plan:** Minimal. The deviation is a test-support artifact, not
a logic change. No scope creep, no architectural decisions touched.

## Issues Encountered

None — the TDD cycle hit RED → GREEN cleanly on first attempt. All 16
`SetXxxFold` methods from Plan 69-02 were callable as expected; `go mod
tidy` network access worked without sandbox-bypass flags; `go vet` and
`golangci-lint run ./internal/sync/... ./internal/unifold/...` were clean
on first run.

## User Setup Required

None — this is a server-side data-population change that activates
automatically on the next sync cycle after deploy. Per D-03 from
CONTEXT.md, the first post-v1.16-deploy sync cycle (within 1h) backfills
all `_fold` columns for all existing rows via the standard
`OnConflict().UpdateNewValues()` path exercised by the idempotency
sub-test. No manual intervention, no backfill script.

## Next Phase Readiness

- **Plan 69-04 (pdbcompat filter routing):** UNBLOCKED. The `_fold`
  columns are now actually populated at sync time, so Plan 69-04 can
  safely route `name__contains` → `name_fold LIKE ?` queries without
  worrying about empty columns silently dropping matches.
- **Brief divergence window:** For v1.16 deploys, there is a ≤1h window
  between rollout completion and first-sync completion where `_fold`
  columns contain their auto-migrate default (`""`). During that window,
  pdbcompat `__contains` queries (when Plan 69-04 lands) will return
  empty results for non-ASCII search terms — ASCII queries keep working
  because they match the original (non-folded) column. CHANGELOG entry
  will document the window.

## Field-Count Note (16 vs "~18")

CONTEXT.md D-01 described "~18 shadow columns across 6 entities." Plan
69-02's SUMMARY locked the tight count at 16 after per-entity
enumeration. This plan's 16 `.SetXxxFold(unifold.Fold(...))` calls
correspond 1:1 to those 16 columns:

| Entity            | Fields folded                        | Count |
| ----------------- | ------------------------------------ | ----- |
| organization      | Name, Aka, City                      | 3     |
| network           | Name, Aka, NameLong                  | 3     |
| facility          | Name, Aka, City                      | 3     |
| internet exchange | Name, Aka, NameLong, City            | 4     |
| campus            | Name                                 | 1     |
| carrier           | Name, Aka                            | 2     |
| **Total**         |                                      | **16** |

Grep confirmation: `grep -c 'unifold\.Fold' internal/sync/upsert.go` →
16; `grep -cE 'SetNameFold|SetAkaFold|SetCityFold|SetNameLongFold'
internal/sync/upsert.go` → 16.

## Self-Check: PASSED

- `internal/sync/upsert_test.go`: FOUND (committed in `8ce16ab`)
- `internal/sync/upsert.go`: FOUND (modified in `cdad023`)
- `go.mod`: FOUND (modified in `cdad023`, x/text now direct)
- `go.sum`: FOUND (modified in `cdad023`)
- `internal/sync/testdata/refactor_parity.golden.json`: FOUND (regenerated in `cdad023`)
- Commits `8ce16ab` and `cdad023`: BOTH FOUND via `git log --oneline`
- `go test -race ./internal/sync/...`: PASS
- `go vet ./...`: PASS
- `golangci-lint run ./internal/sync/... ./internal/unifold/...`: 0 issues

---
*Phase: 69-unicode-operator-in-robustness*
*Completed: 2026-04-19*
