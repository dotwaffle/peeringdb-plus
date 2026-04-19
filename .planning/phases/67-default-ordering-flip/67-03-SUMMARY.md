---
phase: 67
plan: 03
subsystem: pdbcompat
tags: [pdbcompat, ordering, tdd, goldens, order-by, compound-default]
completed_at: 2026-04-19
milestone: v1.16
requires: [67-01, 67-02]
provides: [pdbcompat_compound_default_orderby]
affects: [internal/pdbcompat/registry_funcs.go, internal/pdbcompat/handler_test.go, internal/pdbcompat/anon_parity_test.go, internal/pdbcompat/testdata/golden/]
tech-stack:
  added: []
  patterns: [compound-default-ordering, id-desc-tertiary-tiebreak, rule1-collateral-test-update]
key-files:
  created:
    - internal/pdbcompat/registry_funcs_ordering_test.go
  modified:
    - internal/pdbcompat/registry_funcs.go
    - internal/pdbcompat/handler_test.go
    - internal/pdbcompat/anon_parity_test.go
decisions:
  - D-02/D-07 compound ORDER BY delivered at the pdbcompat query-construction layer (not via ent schema annotation — that's the entrest-only D-07 path)
  - D-03 tautological golden regen — 39 files re-flushed under `-args -update`, diff empty, zero structural changes (expected under 1-row fixtures)
  - Rule 1 (bug/contract flip) TestResultsSortedByID renamed to TestResultsSortedByDefaultOrder and re-asserted against the new DESC contract
  - Rule 1 (bug/order-sensitivity) anon_parity_test ixlan sub-test pinned to id=100 via registered filter surface — keeps shape parity against the Private-shaped fixture under any order
metrics:
  duration: ~25m
  tasks: 2
  files: 4
  commits: [adf77f5, fb02653]
requirements:
  - ORDER-01
---

# Phase 67 Plan 03: pdbcompat default-ordering flip Summary

## One-liner

All 13 pdbcompat `/api/<type>` list closures now emit compound `ORDER BY updated DESC, created DESC, id DESC`, matching upstream `django-handleref` `Meta.ordering = ("-updated", "-created")` with a deterministic `id DESC` tiebreak; new `TestDefaultOrdering_Pdbcompat` proves the contract with multi-row seed + two tie-break sub-tests.

## What shipped

Two atomic commits in the plan-specified RED → GREEN order:

1. **`adf77f5` — RED phase** — new `internal/pdbcompat/registry_funcs_ordering_test.go` (339 lines) with `TestDefaultOrdering_Pdbcompat` covering:
   - `Network`, `Facility`, `InternetExchange` sub-tests — each seeds 3 rows with distinct `updated` timestamps (t0, t0+1h, t0+2h) and identical `created`; asserts the response body's `data[].id` slice is in the expected DESC order.
   - `TieBreakCreated` sub-test — two Network rows with identical `updated` but distinct `created`; asserts fallback to `created DESC`.
   - `TieBreakID` sub-test — two Network rows with identical `updated` AND `created` but distinct `id`; asserts fallback to `id DESC`.
   - Used `package pdbcompat` (matching neighbouring tests), fresh `testutil.SetupClient(t)` per sub-test (in-memory SQLite), hand-rolled parent-org + entity seeds (NO modification to `internal/testutil/seed` per plan `<read_first>` rule).
   - Expected-state-after-Task-1: the test FAILS with 5/5 sub-tests failing the contract — confirmed by `go test -race -run TestDefaultOrdering_Pdbcompat` exiting non-zero with messages like `Network ordering mismatch: got [5 10 20], want [20 10 5]`.

2. **`fb02653` — GREEN phase** — three file edits + a golden-regen no-op:
   - `internal/pdbcompat/registry_funcs.go`: `replace_all` flip of `.Order(ent.Asc("id"))` → `.Order(ent.Desc("updated"), ent.Desc("created"), ent.Desc("id"))` — exactly 13 sites, across all 13 registered entity wirings (`wireOrgFuncs`, `wireNetFuncs`, `wireFacFuncs`, `wireIXFuncs`, `wirePocFuncs`, `wireIXLanFuncs`, `wireIXPfxFuncs`, `wireNetIXLanFuncs`, `wireNetFacFuncs`, `wireIXFacFuncs`, `wireCarrierFuncs`, `wireCarrierFacFuncs`, `wireCampusFuncs`).
   - `internal/pdbcompat/handler_test.go`: renamed `TestResultsSortedByID` → `TestResultsSortedByDefaultOrder`; body rewritten to assert the compound-DESC contract against the 3-row past/now/future seed (`ids == [3,2,1]`). Removed the unused `"sort"` import.
   - `internal/pdbcompat/anon_parity_test.go`: added a per-type URL suffix so the `ixlan` sub-test pins to `?id=100` (`seed.IxLanGatedID`) via the already-registered `ixf_ixp_member_list_url_visible` filter. This makes the shape-parity comparison deterministic against the fixture's `Private` `data[0]` shape regardless of order resolution on the tied (100 gated / 101 Public) ixlan seed pair. Added `"strconv"` import.
   - `internal/pdbcompat/testdata/golden/`: regenerated via `go test -run TestGoldenFiles ./internal/pdbcompat/ -args -update` — **diff is empty across all 39 files**. Single-row fixtures tautologically produce no reorder under any stable ORDER BY; CONTEXT.md D-03 2026-04-19 note anticipates exactly this and defers multi-row coverage to Phase 72.

## Task record

### Task 1 — RED (commit `adf77f5`)

Wrote the new test file and confirmed it failed the contract. Sub-tests ran under `t.Parallel()` inside each `t.Run`; each sub-test owns an isolated in-memory SQLite via `testutil.SetupClient(t)`. A small `orderingTestCtx` helper carried the shared base timestamp + client so the three representative seeds (Network/Facility/IX) could stay declarative and avoid duplication.

**Verify output (RED) — excerpt:**

```
--- FAIL: TestDefaultOrdering_Pdbcompat (0.00s)
    --- FAIL: TestDefaultOrdering_Pdbcompat/Network (0.22s)
        registry_funcs_ordering_test.go:55: Network ordering mismatch: got [5 10 20], want [20 10 5]
    --- FAIL: TestDefaultOrdering_Pdbcompat/Facility (0.22s)
        registry_funcs_ordering_test.go:55: Facility ordering mismatch: got [30 40 50], want [50 40 30]
    --- FAIL: TestDefaultOrdering_Pdbcompat/InternetExchange (0.22s)
        registry_funcs_ordering_test.go:55: InternetExchange ordering mismatch: got [100 200 300], want [300 200 100]
    --- FAIL: TestDefaultOrdering_Pdbcompat/TieBreakCreated (0.22s)
        registry_funcs_ordering_test.go:106: tie-break by created DESC failed: got [10 11], want [11 10] ...
    --- FAIL: TestDefaultOrdering_Pdbcompat/TieBreakID (0.22s)
        registry_funcs_ordering_test.go:152: tie-break by id DESC failed: got [10 99], want [99 10]
```

Exactly the failures the plan anticipated — code still emitted `ent.Asc("id")`, so all five assertions surfaced the ASC id order against the expected DESC compound order.

Acceptance (Task 1):
- File `internal/pdbcompat/registry_funcs_ordering_test.go` exists — 339 lines (well above the `min_lines: 100` truth).
- `TestDefaultOrdering_Pdbcompat` contains the required 3 sub-tests plus 2 tie-break sub-tests — 5 sub-tests total, all failing pre-Task-2.

### Task 2 — GREEN + goldens + collateral test updates (commit `fb02653`)

Three-step execution:

1. **Flip** — single `Edit(replace_all=true)` on `registry_funcs.go` replaced all 13 `.Order(ent.Asc("id"))` → `.Order(ent.Desc("updated"), ent.Desc("created"), ent.Desc("id"))`. Post-flip grep counts: 13 compound / 0 ASC (matches plan acceptance).
2. **New test passes** — `go test -race -run TestDefaultOrdering_Pdbcompat ./internal/pdbcompat/...` green in 1.297s.
3. **Full suite** — `go test -race ./internal/pdbcompat/...` initially surfaced two collateral failures; both fixed inline per Rule 1 (direct consequence of the intentional ordering contract change) and committed together.

**Goldens regen** — executed `go test -run TestGoldenFiles ./internal/pdbcompat/ -args -count=1 -update` (the bare `-update ./...` form the plan shorthand suggested hit `./...` with no Go files in repo root; moved to explicit package + `-args -update` per Go test flag semantics). `git diff --stat internal/pdbcompat/testdata/golden/` → empty. 39 goldens were rewritten with byte-identical content — reorder is a no-op under 1-row fixtures (CONTEXT.md D-03 2026-04-19 note).

**Test suite health after all edits:**

- `go vet ./...` — clean.
- `go build ./...` — clean.
- `go test -race ./internal/pdbcompat/...` — `ok` 2.544s (full pdbcompat suite).
- `go test -race ./internal/conformance/... ./cmd/peeringdb-plus/...` — both `ok`.
- `go test -race ./...` — all 21 test-bearing packages pass (grpcserver included even though it's concurrently being edited by Plan 67-04 in the same working tree — 67-04 tests stay green with both plans' edits co-resident).

Acceptance (Task 2):
- `grep -c 'ent.Desc("updated"), ent.Desc("created"), ent.Desc("id")' internal/pdbcompat/registry_funcs.go` = **13**. ✅
- `grep -c 'ent.Asc("id")' internal/pdbcompat/registry_funcs.go` = **0**. ✅
- `go test -race -run TestDefaultOrdering_Pdbcompat ./internal/pdbcompat/...` passes. ✅
- `go test -race -run TestGoldenFiles ./internal/pdbcompat/...` passes. ✅
- `go test -race ./internal/pdbcompat/...` passes. ✅

## Files changed

| File | Change | Kind |
|------|--------|------|
| `internal/pdbcompat/registry_funcs_ordering_test.go` | **new** (339 lines) | Task 1 RED test |
| `internal/pdbcompat/registry_funcs.go` | 13 `.Order(...)` sites flipped | Task 2 GREEN flip |
| `internal/pdbcompat/handler_test.go` | Renamed `TestResultsSortedByID` → `TestResultsSortedByDefaultOrder`; rewrote body for compound-DESC contract; dropped unused `sort` import | Rule 1 collateral |
| `internal/pdbcompat/anon_parity_test.go` | ixlan sub-test pinned to `?id=100` via registered filter; `strconv` import added | Rule 1 collateral |
| `internal/pdbcompat/testdata/golden/` (39 files) | Regenerated — byte-identical | D-03 tautological flush |

**Not touched (respecting Plan 67-04's scope):** `internal/grpcserver/pagination.go`, `internal/grpcserver/pagination_test.go`, `internal/grpcserver/generic.go`, any other `internal/grpcserver/*` file. Plan 67-04's concurrent edits in the working tree were visible during my run (noted in `git status`) but were staged and committed under 67-04's own commits (`568c965`, `c854fa9`) — zero file overlap with this plan.

## Deviations from plan

Two Rule 1 collateral fixes needed beyond the plan's explicit step list — both direct consequences of the intentional ordering flip, and both kept in the single GREEN commit per the plan's "single commit contains: code flip, new test file, regenerated goldens" constraint.

### 1. [Rule 1 — Obsolete contract test] `TestResultsSortedByID` → `TestResultsSortedByDefaultOrder`

- **Found during:** Task 2 verify (`go test -race ./internal/pdbcompat/...`).
- **Issue:** The old test asserted `sort.IntsAreSorted(ids)` — the very id-ASC contract this plan intentionally replaces. Output: `handler_test.go:441: results not sorted by ID: [3 2 1]`.
- **Fix:** Renamed the test (signalling intent), replaced body with explicit `slices.Equal(ids, []int{3,2,1})` assertion tied to the `setupTestHandler` 3-row past/now/future seed, removed unused `sort` import.
- **Files modified:** `internal/pdbcompat/handler_test.go`.
- **Commit:** `fb02653` (bundled with GREEN per plan constraint).

### 2. [Rule 1 — Ordering-sensitive fixture match] `anon_parity_test.go` ixlan sub-test

- **Found during:** Task 2 verify.
- **Issue:** `seed.Full` seeds TWO ixlan rows with identical `updated` + `created`: id=100 (`visible="Users"` → URL redacted for anon) and id=101 (`visible="Public"` → URL visible). Under compound DESC the updated+created tie resolves to `id DESC`, so id=101 wins under `?limit=1`. id=101 surfaces `ixf_ixp_member_list_url` (Public visibility path), but the committed upstream anon fixture's `data[0]` is a `Private` shape (no URL field) — producing `extra_field` mismatch: `path="data[0].ixf_ixp_member_list_url" kind="extra_field"`.
- **Fix considered & rejected:** (a) edit `seed.Full` to stagger ixlan timestamps — wide blast radius across many tests and Phase 72 dependencies; (b) add to `knownDivergences` — the plan explicitly warns "The intent of this test is to catch shape drift, not launder it".
- **Fix applied:** Added a per-type URL suffix so the ixlan sub-test queries `/api/ixlan?limit=1&id=100` via the already-registered `ixf_ixp_member_list_url_visible` filter infrastructure (confirmed registered at `internal/pdbcompat/registry.go:253`). This pins the sub-test to the gated/`Users`-visibility row regardless of ordering, exercising the same redaction path the test was written to cover. No production code touched; the filter surface is a pre-existing behaviour.
- **Files modified:** `internal/pdbcompat/anon_parity_test.go` (+17 lines comment + logic, +1 `strconv` import).
- **Commit:** `fb02653`.

### Golden regen command shape

Plan text suggested `go test -update ./internal/pdbcompat -run TestGoldenFiles`. Executed as written, Go rejects this with `no Go files in /home/dotwaffle/Code/pdb/peeringdb-plus` — `-update` is a local test flag defined in `golden_test.go:20`, not a `go test` framework flag, so it must go after `-args`. Used the canonical form `go test -run TestGoldenFiles ./internal/pdbcompat/ -args -update`. Zero semantic deviation — same result (39 files re-flushed, byte-identical).

## Signals for the phase verifier

```bash
# 1. 13 compound ORDER BY in pdbcompat
grep -c 'ent\.Desc("updated"), ent\.Desc("created"), ent\.Desc("id")' internal/pdbcompat/registry_funcs.go
# → 13

# 2. Zero id-ASC remaining in pdbcompat List closures
grep -c 'ent\.Asc("id")' internal/pdbcompat/registry_funcs.go
# → 0

# 3. New multi-row ordering test present + passing
go test -race -run TestDefaultOrdering_Pdbcompat ./internal/pdbcompat/... -count=1
# → ok

# 4. Goldens still match (any accidental structural divergence would fail)
go test -race -run TestGoldenFiles ./internal/pdbcompat/... -count=1
# → ok

# 5. Full pdbcompat suite green (includes anon-parity + contract-flip test)
go test -race ./internal/pdbcompat/... -count=1
# → ok

# 6. Full repo green
go test -race ./... -count=1
# → ok
```

## Self-Check

Files:
- `internal/pdbcompat/registry_funcs.go` — FOUND, 13 compound-DESC, 0 ASC.
- `internal/pdbcompat/registry_funcs_ordering_test.go` — FOUND, 339 lines, 5 sub-tests.
- `internal/pdbcompat/handler_test.go` — FOUND, contains `TestResultsSortedByDefaultOrder`, no `"sort"` import.
- `internal/pdbcompat/anon_parity_test.go` — FOUND, contains `strconv.Itoa(seed.IxLanGatedID)`.
- `internal/pdbcompat/testdata/golden/` — FOUND (13 dirs, 39 files), all present.

Commits:
- `adf77f5` — FOUND in `git log` (RED — test file committed with expected failure).
- `fb02653` — FOUND in `git log` (GREEN — flip + collateral test updates + golden re-flush).

## Self-Check: PASSED

## PLAN 67-03 COMPLETE

**Commits:**
- `adf77f5` — `test(67-03): add TestDefaultOrdering_Pdbcompat with multi-row seed (RED)`
- `fb02653` — `feat(67-03): flip pdbcompat default ordering to (-updated, -created, -id)`
