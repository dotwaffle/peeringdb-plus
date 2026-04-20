---
phase: 68-status-since-matrix
plan: 03
subsystem: pdbcompat
tags: [status-matrix, since, limit, pk-lookup, soft-delete, rest-compat]

# Dependency graph
requires:
  - phase: 68-status-since-matrix (plan 01)
    provides: PDBPLUS_INCLUDE_DELETED gate removed; upsert path unconditionally persists status=deleted rows
  - phase: 68-status-since-matrix (plan 02)
    provides: 13 markStaleDeleted* soft-delete closures producing real tombstones with updated=cycleStart (STATUS-03 data prereq)
provides:
  - applyStatusMatrix(isCampus, sinceSet bool) helper in filter.go (rest.py:694-727 status predicate)
  - 13 list closures in registry_funcs.go emit applyStatusMatrix + conditional .Limit(opts.Limit) via `if opts.Limit > 0 { q2 = q2.Limit(...) }`
  - 13 Fields maps in registry.go with `"status": FieldString` removed (D-07 silent-ignore semantic)
  - 26 StatusIn("ok","pending") predicate inserts across the 13 getXWithDepth functions in depth.go (13 depth>=2 Where extensions + 13 .Get→.Query().Where().Only() flips)
  - ParsePaginationParams accepts limit=0 as unlimited sentinel + MaxLimit clamp gated on limit>0 (LIMIT-01)
  - serveList LIMIT-02 depth guardrail (silently ignores ?depth= on list endpoints with a debug slog)
  - TestEntLimitZeroProbe locks the empirical ent Limit(0) behaviour
  - TestStatusMatrix table-driven E2E with 9 subtests covering STATUS-01/02/04 + LIMIT-01/02
  - Pre-existing handler_test assertions reconciled to the new D-07 semantic
affects: [68-04-changelog, 69-unicode-operators-in, 70-traversal, 71-memory, 72-parity]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Research-assumption probe test as first task in a plan: TestEntLimitZeroProbe proved Assumption A1 WRONG (ent typed builder treats Limit(0) as unlimited via sqlgraph graph.go:1086 `if q.Limit != 0`, NOT as `LIMIT 0`). The probe-first structure caught the wrong assumption before touching any of the 13 closures, preserving the plan's defensive `if opts.Limit > 0` gate as correct-under-either-behaviour."
    - "Silent-filter removal for D-07-style intentional overrides: removing a field from the Fields map is how ParseFilters produces the silent-ignore semantic without touching buildPredicate. No new reservedParams entry, no new handler branch — the absence IS the semantic."
    - "Defensive conditional-limit gate: `q2 := q.Offset(opts.Skip); if opts.Limit > 0 { q2 = q2.Limit(opts.Limit) }; q2.All(ctx)` survives whether ent Limit(0)=unlimited (current) or Limit(0)=LIMIT 0 (hypothesised). Zero risk under either ent behaviour."
    - "Pre-existing test assertions that seeded deleted/pending rows need either fixture flips (handler_test.go: 3rd Network status=deleted → status=ok) or assertion flips (TestExactFilter's ?status=ok assertion replaced with ?asn=13335 — status field is no longer filterable)."

key-files:
  created:
    - internal/pdbcompat/limit_probe_test.go
    - internal/pdbcompat/status_matrix_test.go
    - .planning/phases/68-status-since-matrix/68-03-SUMMARY.md
  modified:
    - internal/pdbcompat/filter.go
    - internal/pdbcompat/registry.go
    - internal/pdbcompat/registry_funcs.go
    - internal/pdbcompat/response.go
    - internal/pdbcompat/handler.go
    - internal/pdbcompat/depth.go
    - internal/pdbcompat/handler_test.go

key-decisions:
  - "Research Assumption A1 rebutted by Task 1 probe: ent v0.14.6 typed builder treats Limit(0) as 'unlimited' (sqlgraph graph.go:1086 `if q.Limit != 0` gate), NOT 'LIMIT 0'. The plan's `if opts.Limit > 0` conditional is still correct — it works under either ent behaviour and documents intent. The probe test now locks the ACTUAL empirical behaviour with a comment flagging the sqlgraph line; any future ent change that reinstates Limit(0)=LIMIT-0 semantics will RED-trip the probe before a silent regression escapes."
  - "handler_test.go fixture flip (3rd seed Network status=deleted → status=ok) chosen over a TestListEndpoint assertion rewrite: the pre-existing tests (TestListEndpoint, TestSearch, TestResultsSortedByDefaultOrder, TestPagination, etc.) are orthogonal to status-matrix behaviour — they test list shape, search, sort, and pagination. Keeping their 3-row fixture and 3-row expectations is cleaner than editing 5 unrelated test bodies. TestExactFilter is the one legitimately-status-aware pre-existing test; it was rewritten to use `?asn=13335` because `?status=ok` is now silently-dropped by ParseFilters per D-07."
  - "26 inline `StatusIn(\"ok\", \"pending\")` literals rather than a package-level `var pkAllowedStatuses = []string{\"ok\", \"pending\"}`: research recommendation for grep-ability — 26 identical lines are easier to spot in code review than a shared variable that could accidentally gain a third status. CLAUDE.md future-phase addenda (a hypothetical STATUS-08 admitting a new status on pk) would want to search for 'pending' in depth.go anyway."
  - "handler.go serveList LIMIT-02 guardrail as a debug-log-only no-op rather than a 400 error: research Open Question 1 recommendation (b) — upstream rest.py behaves identically (silently ignores unsupported request shapes). A 400 would break callers who happen to pass `?depth=` on list URLs out of habit. The Phase 71 list+depth work will make the param functional; until then the silent ignore is the least-surprising behaviour. opts.Depth never leaks into list closures (grep-verified) so there's no actual bug to fix — just a slog.Debug paper trail for operators."
  - "Campus-only applyStatusMatrix(true) inside wireCampusFuncs: the other 12 wire functions pass false. This is the ONLY asymmetry across the 13 closures — matches upstream rest.py:721 campus special case for the pending-on-since rule (D-05). A grep acceptance check (`applyStatusMatrix(true` = 1; `applyStatusMatrix(false` = 12) locks this invariant."

patterns-established:
  - "Probe-first-task pattern for research-assumption plans: when a plan has a flagged assumption (A1, A2, ...), make the first task an empirical probe test that locks the assumption before any implementation edits depend on it. If the probe RED-trips, STOP and re-plan (this plan's probe trip was benign — the plan's `if opts.Limit > 0` gate was already defensive enough to survive either ent behaviour, so execution continued with the corrected probe assertion + a documentation comment)."
  - "Fields-map entry removal = silent-ignore semantic in pdbcompat: a clean way to express 'this param is intentionally ignored'. Used here for `?status=<anything>`. Future use case: any query param the upstream API silently swallows without documenting."
  - "Conditional .Limit gate pattern for ent queries where unlimited is a first-class sentinel: `if opts.Limit > 0 { q = q.Limit(opts.Limit) }` makes the unlimited case explicit and guards against ent's internal `Limit(0)=unlimited` behaviour being changed in a future version."

requirements-completed:
  - STATUS-01
  - STATUS-02
  - STATUS-03
  - STATUS-04
  - LIMIT-01
  - LIMIT-02

# Metrics
duration: ~40min
completed: 2026-04-19
---

# Phase 68 Plan 03: pdbcompat Status × Since Matrix + limit=0 Semantics Summary

**Wired the pdbcompat request path to upstream rest.py:494-727's status × since matrix + limit=0 unlimited semantics across 13 list closures, 13 pk-lookup functions (26 StatusIn inserts), and the Fields-map + pagination-params layer; closed STATUS-01/02/03/04 + LIMIT-01/02 in one coordinated edit locked down by an 8-subtest table-driven E2E.**

## Performance

- **Duration:** ~40 minutes
- **Started:** 2026-04-19T14:35Z (approx)
- **Completed:** 2026-04-19T15:15Z (approx)
- **Tasks:** 4
- **Files created:** 3 (2 test files + this SUMMARY)
- **Files modified:** 7 (filter.go, registry.go, registry_funcs.go, response.go, handler.go, depth.go, handler_test.go)

## Accomplishments

- **Task 1 (empirical probe):** `TestEntLimitZeroProbe` RED-tripped on initial run against Research Assumption A1 (hypothesised `.Limit(0)` would emit `LIMIT 0`). Empirically confirmed ent v0.14.6's typed builder treats `Limit(0)` as unlimited via sqlgraph graph.go:1086 `if q.Limit != 0` gate. Probe test rewritten to lock the actual behaviour with a sqlgraph line-ref in the doc comment. The plan's `if opts.Limit > 0` gate pattern still applies — it's defensive under both ent behaviours. `applyStatusMatrix` helper also landed in filter.go (rest.py:694-727 status predicate).
- **Task 2 (list path):** Removed 13 `"status": FieldString` entries from registry.go Fields maps (D-07 silent-ignore); applied `applyStatusMatrix(isCampus, opts.Since != nil)` predicate append + conditional `.Limit(opts.Limit)` gate across all 13 `wireXFuncs` closures in registry_funcs.go (grep acceptance: `applyStatusMatrix` = 13, `applyStatusMatrix(true` = 1, `applyStatusMatrix(false` = 12, `if opts.Limit > 0` = 13). Fixed `ParsePaginationParams` in response.go to accept `limit=0` as the unlimited sentinel + gate MaxLimit clamp. Added LIMIT-02 depth-on-list guardrail in serveList (debug slog-only no-op).
- **Task 3 (pk lookup):** Added 26 `StatusIn("ok", "pending")` predicate inserts in depth.go — 13 Where extensions on depth>=2 branches + 13 `.Get(ctx, id)` → `.Query().Where(X.ID(id), X.StatusIn("ok","pending")).Only(ctx)` flips on default-depth branches. Grep acceptance: `.Get(ctx, id)` = 0, `StatusIn("ok", "pending")` = 26. `ent.IsNotFound` semantics flow through `.Only()` unchanged so handler.go:218's 404 response carries over to status=deleted rows.
- **Task 4 (test coverage):** Created `status_matrix_test.go` with 9 t.Run subtests covering list_no_since_returns_only_ok, list_with_since_non_campus_returns_ok_and_deleted, list_with_since_campus_includes_pending, pk_ok_returns_200, pk_pending_returns_200, pk_deleted_returns_404, status_deleted_no_since_is_empty, limit_zero_returns_all_rows, and depth_on_list_is_silently_ignored. All subtests pass on first run against the end-to-end HTTP handler.

## Task Commits

| # | Task | Commit | Type |
|---|------|--------|------|
| 1 | Probe ent Limit(0) + add applyStatusMatrix helper | `f2cefdf` | test |
| 2 | Apply status matrix + limit=0 semantics across 13 list closures | `1aae237` | feat |
| 3 | Add StatusIn(ok, pending) predicates to 13 pk-lookup functions | `ee28d12` | feat |
| 4 | Add table-driven status × since × limit matrix E2E | `23892a0` | test |

## Files Created/Modified

- **`internal/pdbcompat/limit_probe_test.go` (new, 67 LOC):** `TestEntLimitZeroProbe` locks the empirical ent `.Limit(0)` = unlimited behaviour via two subtests (Limit_0_returns_all_rows, no_Limit_returns_all_rows). Doc comment flags `sqlgraph/graph.go:1086 `if q.Limit != 0`` as the upstream mechanism; if that ever flips, this probe RED-trips.
- **`internal/pdbcompat/filter.go` (+18 LOC):** New `applyStatusMatrix(isCampus, sinceSet bool)` helper immediately after `applySince`. Returns `sql.FieldEQ("status","ok")` when `sinceSet=false`, `sql.FieldIn("status","ok","deleted")` when `sinceSet=true && !isCampus`, `sql.FieldIn("status","ok","deleted","pending")` when `sinceSet=true && isCampus`. Doc references rest.py:694-727.
- **`internal/pdbcompat/registry.go` (-13 LOC):** Deleted 13 `"status": FieldString` entries from the 13 Fields maps. No other changes.
- **`internal/pdbcompat/registry_funcs.go` (+78 LOC):** 13 closures each gained a `preds = append(preds, predicate.X(applyStatusMatrix(isCampus, opts.Since != nil)))` line after the applySince block + a `q2 := q.Offset(opts.Skip); if opts.Limit > 0 { q2 = q2.Limit(opts.Limit) }; rows, err := q2.All(ctx)` rewrite of the `.Limit(opts.Limit).Offset(opts.Skip).All(ctx)` tail. Only `wireCampusFuncs` passes `true` for isCampus.
- **`internal/pdbcompat/response.go` (+8/-3 LOC):** `ParsePaginationParams` now accepts `parsed >= 0` (was `> 0`) and gates `MaxLimit` clamp on `limit > 0 && limit > MaxLimit` (was just `limit > MaxLimit`). Doc comment expanded with LIMIT-01 rationale + rest.py:734-737 reference + Phase 71 memory-budget coordination note.
- **`internal/pdbcompat/handler.go` (+14 LOC):** Import `log/slog`. `serveList` gets a `if params.Get("depth") != ""` check that emits `slog.DebugContext(..., "pdbcompat list: ignoring unsupported ?depth= param ...")` before parsing pagination. opts.Depth is never populated for list requests so this is documentation-only; Phase 71 will make the param functional.
- **`internal/pdbcompat/depth.go` (+52/-26 LOC):** 13 `.Where(X.ID(id))` calls in depth>=2 branches extended to `.Where(X.ID(id), X.StatusIn("ok", "pending"))`. 13 `.Get(ctx, id)` calls in depth<2 branches replaced with `.Query().Where(X.ID(id), X.StatusIn("ok", "pending")).Only(ctx)`.
- **`internal/pdbcompat/handler_test.go` (+20/-7 LOC):** setupTestHandler's 3rd seed Network flipped from `status="deleted"` to `status="ok"` (5 pre-existing tests depend on 3-row shape and are orthogonal to status matrix semantics). TestExactFilter rewritten to use `?asn=13335` since `?status=ok` now silently dropped.
- **`internal/pdbcompat/status_matrix_test.go` (new, 268 LOC):** `TestStatusMatrix` with 9 t.Run subtests + two local helpers (`fetchDataLength`, `fetchStatusCode`) that complement the existing `registry_funcs_ordering_test.go` helpers (`newMuxForOrdering`, `fetchIDOrder`).

## Decisions Made

- **Research Assumption A1 rebutted + probe test updated:** Probe-first pattern paid off — Assumption A1 was factually wrong but the plan's `if opts.Limit > 0` gate still worked. Probe test now documents the actual ent mechanism (sqlgraph `if q.Limit != 0`) so any future regression catches itself.
- **Fixture-flip over test-rewrite for handler_test.go:** 5 pre-existing tests (TestListEndpoint, TestSearch, TestResultsSortedByDefaultOrder, TestPagination, TestSearchEmpty) assert list shape/search/sort/pagination behaviour orthogonal to status filtering. Flipping the 3rd seed network's `status="deleted"` → `status="ok"` preserved all 5 tests unchanged. TestExactFilter is the only genuinely-status-aware pre-existing test; it got a surgical rewrite to use `?asn=13335` since `?status=` is no longer in the Fields map.
- **Inline StatusIn literals (26 copies) over shared package var:** Research recommendation. Grep-ability trumps DRY for security-relevant allowlists — 26 identical `StatusIn("ok", "pending")` lines are easier to audit in code review than a shared `var pkAllowedStatuses = []string{"ok", "pending"}` that could gain a third status in a future PR.
- **LIMIT-02 guardrail as debug-slog no-op (not a 400):** Upstream rest.py silently ignores unsupported request shapes. A 400 would break callers who pass `?depth=` on list URLs out of habit. Phase 71 will make the param functional; until then the silent ignore is the least-surprising behaviour.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Research Assumption A1 was wrong — ent Limit(0) is already unlimited, not `LIMIT 0`**

- **Found during:** Task 1 (TestEntLimitZeroProbe initially asserted `len(rows) == 0` per research hypothesis; test RED-tripped returning 3 rows)
- **Issue:** The plan and research both asserted that ent's typed-builder `.Limit(0)` emits SQL `LIMIT 0` (returns zero rows). Empirically false: ent v0.14.6's sqlgraph layer at `dialect/sql/sqlgraph/graph.go:1086` guards `if q.Limit != 0 { selector.Limit(q.Limit) }`. `Limit(0)` is treated identically to not calling `.Limit()` at all.
- **Fix:** Updated `TestEntLimitZeroProbe` to lock the actual behaviour with a sqlgraph file:line reference in the doc comment. The plan's `if opts.Limit > 0 { q2 = q2.Limit(opts.Limit) }` gate is still the right implementation choice — it's defensive and grep-visible. Also now correct under both behaviours.
- **Files modified:** `internal/pdbcompat/limit_probe_test.go`
- **Verification:** `go test ./internal/pdbcompat -race -count=1 -run 'TestEntLimitZeroProbe'` → `ok`.
- **Committed in:** `f2cefdf` (Task 1 commit)

**2. [Rule 1 - Bug] Pre-existing handler_test.go assertions stale under new D-07 semantic**

- **Found during:** Task 2 (6 tests in handler_test.go FAILed after removing `"status":FieldString` + applying status matrix)
- **Issue:** `setupTestHandler` seeds 3 Networks (one each: ok/ok/deleted). Under the new D-07 semantic, list responses without `?since` silently filter to `status=ok` regardless of any `?status=` param, so the seed's deleted row is no longer visible. Five tests (TestListEndpoint, TestSearch, TestResultsSortedByDefaultOrder, TestPagination, TestSearchEmpty) expected 3-row responses. TestExactFilter's `?status=ok` assertion is doubly-broken: the Fields map no longer contains `status` so the filter is silently dropped.
- **Fix:** Flipped the 3rd seed Network's status from `"deleted"` to `"ok"` — preserves 5 tests unchanged with comments explaining why. Rewrote TestExactFilter to use `?asn=13335` (a still-filterable field) since the original `?status=ok` assertion is now meaningless under D-07.
- **Files modified:** `internal/pdbcompat/handler_test.go`
- **Verification:** `go test ./internal/pdbcompat -race -count=1` → `ok` (all 34 tests pass).
- **Committed in:** `1aae237` (Task 2 commit — rolled in with the closure edits)

---

**Total deviations:** 2 auto-fixed Rule 1 bugs. Both were expected consequences of the plan semantics:
- Probe test assertion needed flipping because research was wrong (the probe-first pattern caught this exactly as designed).
- Pre-existing test assertions/fixtures needed flipping because D-07 intentionally changes list visibility semantics for non-ok rows.

**Impact on plan:** No scope creep. All auto-fixes are in-scope test maintenance for the STATUS-01/D-07 flip. No new code paths introduced beyond what the plan specified.

## Known Stubs

None. All changes wire real behaviour; no placeholder returns, no mock data sources.

## Threat Flags

None. No new network surface, auth paths, or schema changes — all changes are within the existing pdbcompat request path.

## Issues Encountered

- Research Assumption A1 rebuttal (documented above).
- Pre-existing test assertion flips (documented above).

Both predictable under the probe-first + D-07-semantic flip pattern.

## Scope Compliance

- **D-04 (no ceiling on limit=0):** Honoured — no safety cap added; Phase 71 still owns the memory budget. Explicit comment in ParsePaginationParams documents the coordination requirement.
- **D-05 (campus-only pending-on-since):** Honoured — `wireCampusFuncs` is the only call site passing `applyStatusMatrix(true /*isCampus*/, ...)`; grep acceptance locks this (true=1, false=12).
- **D-06 (all 13 pk-paths get (ok, pending)):** Honoured — 26 StatusIn inserts in depth.go (grep count exact).
- **D-07 (list without since filters to status=ok):** Achieved via the Fields-map removal approach (cleaner than adding a separate silent-intercept).
- **LIMIT-02 guardrail:** Honoured — serveList silently ignores `?depth=` with a debug slog; opts.Depth never leaks into list closures (grep-verified).
- **Out of scope untouched:** sync, config, docs, entrest, grpcserver, GraphQL — zero changes. Proto frozen since v1.6.
- **Goldens:** Not regenerated — testutil/seed.Full uses status=ok exclusively so existing 39 goldens are untouched.
- **Build/lint gates:** `go build ./...`, `go vet ./...`, `go test -race ./... -count=1`, `golangci-lint run ./...` all green.

## User Setup Required

None. Behaviour change is transparent at the API layer; existing callers that relied on `?status=deleted` list responses would have been already broken because no deleted rows existed in the DB pre-Phase-68 Plan 02. Callers who use the new `?since=N` window will now see tombstone rows for the first time.

## Next Phase Readiness

Plan 68-03 closes STATUS-01, STATUS-02, STATUS-03 (the surface-level filter), STATUS-04, LIMIT-01, and LIMIT-02 at the pdbcompat boundary. Only Plan 68-04 remains for Phase 68 — it handles:

- CHANGELOG.md bootstrap documenting the Phase 68 behavioural changes for v1.16.
- docs/API.md Known Divergences section (Phase 72 seeds).
- CLAUDE.md soft-delete hygiene note covering the sync-side changes from Plan 68-02.
- Final REQ-ID coverage audit (STATUS-01..05, LIMIT-01, LIMIT-02).

No blockers for Plan 68-04. The coordinated-ship reminder (68 + 69 + 70 + 71 as a single release) stands; `limit=0` unbounded without Phase 71's memory budget is still the OOM risk documented in STATE.md.

---
*Phase: 68-status-since-matrix*
*Completed: 2026-04-19*

## Self-Check: PASSED

- FOUND: internal/pdbcompat/limit_probe_test.go (TestEntLimitZeroProbe with Limit_0_returns_all_rows + no_Limit_returns_all_rows subtests)
- FOUND: internal/pdbcompat/filter.go (applyStatusMatrix helper at line 24, rest.py:694-727 doc reference)
- FOUND: internal/pdbcompat/registry.go (0 `"status": FieldString` entries remaining in Fields maps)
- FOUND: internal/pdbcompat/registry_funcs.go (13 applyStatusMatrix call sites; 1 true + 12 false; 13 `if opts.Limit > 0` gates)
- FOUND: internal/pdbcompat/response.go (`parsed >= 0` gate + `limit > 0 && limit > MaxLimit` clamp)
- FOUND: internal/pdbcompat/handler.go (Phase 68 LIMIT-02 guardrail comment + slog.DebugContext call)
- FOUND: internal/pdbcompat/depth.go (0 `.Get(ctx, id)` calls; 26 `StatusIn("ok", "pending")` predicates; 13 getXWithDepth functions preserved)
- FOUND: internal/pdbcompat/status_matrix_test.go (9 t.Run subtests, fetchDataLength + fetchStatusCode helpers)
- FOUND: internal/pdbcompat/handler_test.go (3rd seed Network status=ok; TestExactFilter uses ?asn=13335)
- FOUND: commit f2cefdf (Task 1 — probe + applyStatusMatrix)
- FOUND: commit 1aae237 (Task 2 — 13 closures + registry.go + response.go + handler.go)
- FOUND: commit ee28d12 (Task 3 — 26 StatusIn inserts in depth.go)
- FOUND: commit 23892a0 (Task 4 — status_matrix_test.go)
- PASS: `go build ./...`
- PASS: `go vet ./...`
- PASS: `go test -race ./... -count=1 -timeout 300s` (entire repo green)
- PASS: `golangci-lint run ./...` (0 issues)
