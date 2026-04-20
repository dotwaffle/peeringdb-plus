---
phase: 67
plan: 05
subsystem: grpcserver
tags: [ordering, pagination, cursor, keyset, compound-order, atomic-commit, wave-4, tdd]
requirements:
  - ORDER-02
dependency_graph:
  requires:
    - 67-01
    - 67-02
    - 67-04
  provides:
    - 13 List*/Stream* RPCs across all entity types now emit rows in (-updated, -created, -id) order
    - StreamParams.QueryBatch flipped to streamCursor-based compound keyset pagination
    - Shared keysetCursorPredicate helper in internal/grpcserver/generic.go — single source of truth for the compound keyset predicate
  affects:
    - 67-06 — cross-surface E2E will assert grpc order matches pdbcompat/entrest
    - Future Stream* clients: keyset cursor is opaque on the wire (string page_token body shape changed inside the base64 envelope) but we have no public ConnectRPC consumers yet so no external break
tech_stack:
  added: []
  patterns:
    - Compound keyset cursor via ent's sql.OrPredicates / sql.AndPredicates canonical composers
    - Shared keysetCursorPredicate(cursor) helper returning func(*sql.Selector), usable by any entity's QueryBatch closure via predicate.X(...) conversion
    - GetUpdated callback pattern alongside GetID for cursor-key extraction from generic entity types
    - Time-spread test fixtures — updated += time.Hour * i to make compound ordering deterministic
key_files:
  created:
    - .planning/phases/67-default-ordering-flip/67-05-SUMMARY.md
  modified:
    - internal/grpcserver/generic.go
    - internal/grpcserver/campus.go
    - internal/grpcserver/carrier.go
    - internal/grpcserver/carrierfacility.go
    - internal/grpcserver/facility.go
    - internal/grpcserver/internetexchange.go
    - internal/grpcserver/ixfacility.go
    - internal/grpcserver/ixlan.go
    - internal/grpcserver/ixprefix.go
    - internal/grpcserver/network.go
    - internal/grpcserver/networkfacility.go
    - internal/grpcserver/networkixlan.go
    - internal/grpcserver/organization.go
    - internal/grpcserver/poc.go
    - internal/grpcserver/grpcserver_test.go
decisions:
  - CONTEXT.md D-01 realized — compound (updated, id) keyset cursor via ent's sql.OrPredicates/sql.AndPredicates
  - CONTEXT.md D-05 realized — SinceID/UpdatedSince remain pure predicates; they no longer seed the cursor lastID (deleted the `if params.SinceID != nil { lastID = int(*params.SinceID) }` line)
  - Added shared keysetCursorPredicate helper in generic.go rather than duplicating the OR-of-AND shape in 13 closures
  - Used q.Where(predicate.X(keysetCursorPredicate(cursor))) pattern — the `predicate.X` conversion works because ent's generated predicate types are `~func(*sql.Selector)` (generic constraint match)
  - Fixed three pre-existing tests whose "first-message" assertions embedded an implicit id-ASC assumption by spreading seed timestamps (id=1 gets updated+1h) — preserves assertion intent without rewriting semantics
metrics:
  duration: "~15min"
  completed: "2026-04-19"
  tasks_completed: "2/2"
  files_changed: 15
---

# Phase 67 Plan 05: grpcserver default ordering flip Summary

**All 26 List/Stream ORDER BY sites across 13 grpcserver entities now emit `(-updated, -created, -id)`; StreamParams.QueryBatch flipped to streamCursor-based compound keyset pagination in a single atomic commit.**

## Performance

- **Started:** ~2026-04-19T12:35:00Z
- **Completed:** 2026-04-19T12:50:08Z
- **Duration:** ~15 min
- **Tasks:** 2/2 (TDD: RED → GREEN, bundled as 2 atomic commits)
- **Files modified:** 15

## Accomplishments

- All 26 grpcserver ORDER BY sites (13 List + 13 Stream) flipped from `ent.Asc(<type>.FieldID)` to compound `ent.Desc(<type>.FieldUpdated), ent.Desc(<type>.FieldCreated), ent.Desc(<type>.FieldID)`
- `StreamParams[E,P].QueryBatch` signature flipped from `(ctx, preds, afterID int, limit int)` to `(ctx, preds, cursor streamCursor, limit int)`; new `GetUpdated func(*E) time.Time` field added for cursor emission
- All 13 per-entity QueryBatch closures wired to compound keyset predicate `(updated < cursor.Updated) OR (updated = cursor.Updated AND id < cursor.ID)` via shared `keysetCursorPredicate` helper
- Removed the Plan 04 TODO anchor (`TODO(phase-67 plan 05)`) — anchor consumed
- Rewrote the existing `TestListNetworks/"ordering by ID ascending"` subtest to `"default compound ordering"` with full (-updated, -created, -id) assertions
- Added 5 new test functions: `TestDefaultOrdering_Grpc_Network`, `TestDefaultOrdering_Grpc_Facility`, `TestDefaultOrdering_Grpc_InternetExchange`, `TestCursorResume_CompoundKeyset`, `TestStreamOrdering_ConcurrentMutation`
- Added 3 new seed helpers: `seedMultiTimestampNetworks`, `seedMultiTimestampFacilities`, `seedMultiTimestampInternetExchanges`

## Task Commits

1. **Task 1 (RED): default ordering + cursor resume tests** — `331077b` (test) — 5 new test functions + 3 seed helpers + existing subtest rewrite; tests fail on build-pass+runtime-assert-fail as required.
2. **Task 2 (GREEN): atomic grpcserver flip** — `4096c10` (feat) — generic.go signature change + shared `keysetCursorPredicate` helper + 13 handler file updates + 3 pre-existing-test seed-timestamp-spread fixes, all in one atomic commit.

## Files Created/Modified

### `internal/grpcserver/generic.go` (core signature change)

- `StreamParams[E,P].QueryBatch` type: `afterID int, limit int` → `cursor streamCursor, limit int`
- Added `GetUpdated func(*E) time.Time` for cursor-key extraction after each batch
- `StreamEntities` main loop: replaced `lastID int` tracker with `cursor streamCursor`
- Removed `lastID = int(*params.SinceID)` seed — SinceID is now purely a predicate (D-05)
- Removed the Plan 04 TODO comment
- Added shared `keysetCursorPredicate(cursor streamCursor) func(*sql.Selector)` helper — single source of truth for the compound keyset predicate shape

### 13 entity handler files (same mechanical pattern)

Each `List<Entity>` Query callback's `Order(ent.Asc(<pkg>.FieldID))` became `Order(ent.Desc(<pkg>.FieldUpdated), ent.Desc(<pkg>.FieldCreated), ent.Desc(<pkg>.FieldID))`.

Each `Stream<Entity>` QueryBatch closure:
- Signature updated to `cursor streamCursor`
- Replaced `Where(<pkg>.IDGT(afterID))` with conditional `q.Where(predicate.<Type>(keysetCursorPredicate(cursor)))` guarded by `!cursor.empty()`
- Same compound Order() call as the List handler
- Added `GetUpdated: func(x *ent.<Type>) time.Time { return x.Updated }`

Files: `campus.go`, `carrier.go`, `carrierfacility.go`, `facility.go`, `internetexchange.go`, `ixfacility.go`, `ixlan.go`, `ixprefix.go`, `network.go`, `networkfacility.go`, `networkixlan.go`, `organization.go`, `poc.go`.

### `internal/grpcserver/grpcserver_test.go` (+391 lines)

- Added `reflect` import.
- Rewrote `TestListNetworks/"ordering by ID ascending"` → `"default compound ordering"` with full tuple-order invariant check plus deterministic `[3,2,1]` ID sequence assertion.
- Spread updated timestamps (1h per row) in `TestListNetworks` seed loop and `seedStreamNetworks` helper so the new order is observable. All existing assertions still pass because:
  - `TestStreamNetworksUpdatedSince`'s windows (2026-01-14..2026-01-16) still envelope the spread.
  - `TestStreamNetworksSinceId`'s count assertions don't depend on order.
- Added 3 seed helpers: `seedMultiTimestamp{Networks,Facilities,InternetExchanges}`.
- Added `TestDefaultOrdering_Grpc_Network/Facility/InternetExchange` — 3-row compound-ordering assertions.
- Added `TestCursorResume_CompoundKeyset` — 10-row paged-fetch equals unbounded-fetch invariant.
- Added `TestStreamOrdering_ConcurrentMutation` — RESEARCH §G-05 mid-stream-mutation smoke test.
- Fixed three pre-existing tests (`TestStreamCarrierFacilities`, `TestStreamNetworkIxLans`, `TestStreamPocs`) whose first-message assertions embedded an implicit id-ASC assumption by setting id=1's updated+=1h so it still sorts first under the new default order.

## Verification

### Grep counts (per plan acceptance criteria)

```bash
$ grep -c 'ent\.Desc(\w\+\.FieldUpdated)' internal/grpcserver/*.go | awk -F: '{sum+=$2} END {print sum}'
26   # 13 List + 13 Stream ✓
$ grep -c 'ent\.Asc(\w\+\.FieldID)' internal/grpcserver/*.go | awk -F: '{sum+=$2} END {print sum}'
0    # all id-ascending sites removed ✓
$ grep -c 'TODO(phase-67 plan 05)' internal/grpcserver/*.go | awk -F: '{sum+=$2} END {print sum}'
0    # Plan 04 TODO anchor consumed ✓
$ grep -c 'cursor streamCursor' internal/grpcserver/generic.go
3    # struct field + StreamEntities local + keysetCursorPredicate param ✓
```

### Tests

- `go build ./...` — passes ✓
- `go vet ./...` — clean ✓
- `golangci-lint run ./internal/grpcserver/...` — 0 issues ✓
- `go test -race ./internal/grpcserver/... -count=1` — passes (8.858s) ✓
- `go test -race ./... -count=1` — full suite passes ✓

### Acceptance criteria — plan spec

| Criterion | Status |
|---|---|
| ≥26 `ent.Desc(\w+.FieldUpdated)` hits across `internal/grpcserver/*.go` | PASS — exactly 26 |
| 0 `ent.Asc(\w+.FieldID)` hits | PASS — 0 |
| `cursor streamCursor` present in generic.go | PASS — 3 hits |
| 0 `TODO(phase-67 plan 05)` hits | PASS — 0 |
| `go test -race ./internal/grpcserver/...` passes | PASS |
| `go test -race ./...` passes whole repo | PASS |
| `go build ./...` passes | PASS |
| `TestDefaultOrdering_Grpc_Network/Facility/InternetExchange` pass | PASS |
| `TestCursorResume_CompoundKeyset` passes | PASS |
| `TestListNetworks/"default compound ordering"` (rewritten) passes | PASS |
| `TestStreamNetworks*` (with seed-timestamp spread) still pass | PASS |

## Decisions Made

### D-01 (execution): shared `keysetCursorPredicate` helper in `generic.go`

Rather than duplicate the nested `sql.OrPredicates(sql.FieldLT, sql.AndPredicates(sql.FieldEQ, sql.FieldLT))` shape across 13 QueryBatch closures, added a single `keysetCursorPredicate(cursor) func(*sql.Selector)` helper beside `castPredicates` in `generic.go`. Each closure uses `predicate.<Type>(keysetCursorPredicate(cursor))` to downcast — legal because ent's generated predicate types are `~func(*sql.Selector)` and ent's variadic composers are declared with that constraint.

Benefit: one place to audit, one place to unit-test (indirectly via `TestCursorResume_CompoundKeyset`), zero duplication.

### D-02 (execution): SinceID no longer seeds `lastID` tracker

Old code had:
```go
lastID := 0
if params.SinceID != nil {
    lastID = int(*params.SinceID)
}
```

Under compound keyset this is incorrect — `SinceID` is purely a predicate (D-05), already injected into the predicates slice at line `sql.FieldGT("id", int(*params.SinceID))`. Seeding the cursor would skip rows that have `updated > start-of-stream` under the new DESC order.

The new code starts with `var cursor streamCursor` (zero value = empty predicate), and SinceID does its work as a pure predicate applied BEFORE the ordering.

### D-03 (execution): pre-existing-test seed-timestamp spread vs assertion rewrite

Three existing tests (`TestStreamCarrierFacilities`, `TestStreamNetworkIxLans`, `TestStreamPocs`) had assertions of the form "first message's field X == id=1's value". Under the old id-ASC order this was tautologically true. Under the new (-updated, -created, -id) order it breaks when all rows share the same `updated` and the `(-id)` tiebreaker makes the highest-id row first.

Two fix options were considered:

1. Rewrite the assertions to match the new first-row's field values.
2. Spread seed timestamps so id=1 is genuinely the most-recently-updated row.

Chose option (2) because it:
- preserves the existing assertion's intent (the authors wrote them to check "id=1 is present and has the right values" — the id-ASC coincidence was incidental);
- adds minimal code churn (1 line per test — `SetUpdated(now.Add(time.Hour))` on id=1);
- matches the pattern already established by the Task 1 changes to `seedStreamNetworks` and `TestListNetworks` seed loops.

## Deviations from Plan

None — plan executed as written. Two minor on-plan adjustments worth noting:

1. **Task 1 test set was expanded.** Plan required only `TestDefaultOrdering_Grpc_{Network,Facility,InternetExchange}` + `TestCursorResume_CompoundKeyset`. Added `TestStreamOrdering_ConcurrentMutation` per the plan's "optional but recommended per RESEARCH §G-05" note.

2. **Pre-existing tests needing fixup scope.** Plan listed: "Also touch `seedStreamNetworks` (~grpcserver_test.go:1692+): spread timestamps across the 3 seeded rows" and anticipated changes to `TestStreamNetworksSinceId`. Reality: `seedStreamNetworks` needed the spread as predicted, and three OTHER `Stream*` tests (CarrierFacilities, NetworkIxLans, Pocs) with implicit-id-ASC-assumption `first-message` assertions also needed seed spread — identified and fixed during Task 2 green-verification. Still within plan scope (test compat with new default ordering), no scope creep.

**Total deviations:** 0 from plan; 2 on-plan scope expansions (+1 optional test, +3 pre-existing test fixtures).

**Impact on plan:** None — plan's single-atomic-commit discipline held. Task 1 ran green at RED phase (5/5 tests fail at assertion, build passes). Task 2 ran green at GREEN phase (all tests pass, full repo passes). No REFACTOR phase needed.

## Authentication gates

None.

## Known Stubs

None.

## Threat Flags

None — no new network endpoints, auth paths, file access, or schema changes. The threat model in the plan's `<threat_model>` block is realized as:

- **T-67-05-01** (cursor field tampering) — mitigated. Cursor fields arrive validated via `decodeStreamCursor` (Plan 04 T-67-04-01); typed ent predicates (`predicate.<Type>(keysetCursorPredicate(cursor))`) prevent SQL injection.
- **T-67-05-02** (pathological cursor DoS) — accepted. `LIMIT streamBatchSize` always applied; new ordering + index (Plan 01) bounds scan cost.
- **T-67-05-03** (header leak under new order) — accepted. D-06 preserved: grpc-total-count reflects filtered cardinality only, no order-dependent leak.

## TDD Gate Compliance

Plan explicitly required atomic single-commit bundling for Task 2 (signature change needs all callers updated). Plan ALSO called for a RED test phase in Task 1. Commits:

- `331077b` — Task 1 RED commit (`test(67-05): ...`). Tests compile, assertions fail on id-ASC data.
- `4096c10` — Task 2 GREEN commit (`feat(67-05): ...`). Signature change + 13 handlers + test fixtures in one atomic commit. All tests pass.

Gate sequence: `test(...)` RED commit exists → `feat(...)` GREEN commit exists after it. ✓

## Issues Encountered

Three pre-existing tests (`TestStreamCarrierFacilities`, `TestStreamNetworkIxLans`, `TestStreamPocs`) had weak "first-message" assertions that silently assumed id-ASC ordering. Surfaced during Task 2's first full test run (post-handler flip, pre-commit). Fixed in-task per D-03 by spreading seed timestamps. See [Decisions Made § D-03](#d-03-execution-pre-existing-test-seed-timestamp-spread-vs-assertion-rewrite).

## Next Phase Readiness

Plan 67-06 (cross-surface E2E) can now assert that grpcserver List/Stream RPCs return rows in the same (-updated, -created, -id) order as pdbcompat (Plan 67-03) and entrest (Plan 67-02). All three surfaces are now aligned on the same compound default order — the v1.16 ORDER-02 requirement is fully met for grpc.

No blockers. No open threads.

## Self-Check

- `internal/grpcserver/generic.go` — FOUND
- `internal/grpcserver/network.go` — FOUND
- `internal/grpcserver/campus.go` — FOUND
- `internal/grpcserver/carrier.go` — FOUND
- `internal/grpcserver/carrierfacility.go` — FOUND
- `internal/grpcserver/facility.go` — FOUND
- `internal/grpcserver/internetexchange.go` — FOUND
- `internal/grpcserver/ixfacility.go` — FOUND
- `internal/grpcserver/ixlan.go` — FOUND
- `internal/grpcserver/ixprefix.go` — FOUND
- `internal/grpcserver/networkfacility.go` — FOUND
- `internal/grpcserver/networkixlan.go` — FOUND
- `internal/grpcserver/organization.go` — FOUND
- `internal/grpcserver/poc.go` — FOUND
- `internal/grpcserver/grpcserver_test.go` — FOUND
- commit `331077b` — FOUND in git log (test RED)
- commit `4096c10` — FOUND in git log (feat GREEN)

## Self-Check: PASSED

## Commits

- `331077b` — test(67-05): add default ordering + cursor resume tests for grpcserver (RED)
- `4096c10` — feat(67-05): flip grpcserver default ordering to (-updated, -created, -id) with compound keyset cursor

---
*Phase: 67-default-ordering-flip*
*Plan: 05*
*Completed: 2026-04-19*

## PLAN 67-05 COMPLETE
