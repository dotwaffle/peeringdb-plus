---
phase: 34-query-optimization-architecture
verified: 2026-03-26T08:30:00Z
status: gaps_found
score: 4/5 must-haves verified
re_verification: false
gaps:
  - truth: "termrender dispatch_test.go compiles and passes"
    status: failed
    reason: "dispatch_test.go line 55 references TotalCount field removed in plan 01; plan 03 used stale field name"
    artifacts:
      - path: "internal/web/termrender/dispatch_test.go"
        issue: "Line 55 uses TotalCount (deleted from SearchGroup in plan 01). Should be HasMore."
    missing:
      - "Change TotalCount: 1 to HasMore: false (or HasMore: true) on line 55 of dispatch_test.go"
human_verification:
  - test: "Verify RFC 9457 error response on REST surface"
    expected: "GET /rest/v1/nonexistent returns application/problem+json with type, title, status, detail fields"
    why_human: "REST error middleware buffers responses -- need running server to test end-to-end"
  - test: "Verify EXPLAIN QUERY PLAN shows index usage on updated/created filters"
    expected: "SEARCH ... USING INDEX on updated or created"
    why_human: "Requires SQLite instance with migrated schema and data; programmatic EXPLAIN not wired into tests"
---

# Phase 34: Query Optimization & Architecture Verification Report

**Phase Goal:** Search and API queries are faster (no double-counting, proper indexes, no JSON roundtrips), errors are consistent across all surfaces, and the renderer and detail handlers are cleanly structured
**Verified:** 2026-03-26T08:30:00Z
**Status:** gaps_found
**Re-verification:** No -- initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Search issues 6 SQL queries per request (one per entity type), not 12 | VERIFIED | `internal/web/search.go`: all 6 `queryXxx` methods use `Limit(displayLimit + 1)` with hasMore truncation. No `.Count(ctx)` calls anywhere. Each type returns `([]SearchHit, bool, error)` not `([]SearchHit, int, error)`. |
| 2 | EXPLAIN QUERY PLAN on updated/created filters shows index usage | ? UNCERTAIN | All 13 schemas have `index.Fields("updated")` and `index.Fields("created")`. Migration schema (`ent/migrate/schema.go`) contains named indexes (e.g. `campus_updated`, `network_created`). Actual EXPLAIN verification requires running SQLite instance. |
| 3 | Field projection in pdbcompat does not call json.Marshal or json.Unmarshal | VERIFIED | `internal/pdbcompat/search.go`: `itemToMap` uses `reflect.ValueOf` and `getFieldMap` with `sync.Map` caching. Zero occurrences of `json.Marshal` or `json.Unmarshal` in that file. |
| 4 | Error responses are consistent across API surfaces (RFC 9457 for HTTP surfaces) | VERIFIED (scoped) | RFC 9457 applied to 3 HTTP surfaces: pdbcompat (7 WriteProblem call sites), web JSON mode (404/500), REST (restErrorMiddleware). ConnectRPC keeps standard connect.NewError (protocol-mandated format). GraphQL excluded per design. Terminal text mode has structured status/title/message output already. |
| 5 | Terminal renderer dispatches via registered function map, detail handlers under 80 lines | VERIFIED (code) / FAILED (test) | `dispatch.go` has `Register[T]` generic function and `renderers` map. `renderer.go` RenderPage uses `renderers[reflect.TypeOf(data)]` -- zero `case templates.` type-switch entries. All 6 handler bodies are 29 lines each. All 6 `queryXxx` methods extracted. BUT `dispatch_test.go` line 55 uses deleted `TotalCount` field causing build failure. |

**Score:** 4/5 truths verified (1 blocked by test compilation error)

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/web/search.go` | Limit+1 search with HasMore | VERIFIED | `displayLimit` const, `HasMore bool` in TypeResult, all 6 query methods use `Limit(displayLimit + 1)` |
| `internal/web/templates/searchtypes.go` | SearchGroup with HasMore bool | VERIFIED | `HasMore bool` field, `hasMoreSuffix` helper |
| `internal/pdbcompat/search.go` | Reflect-based field projection | VERIFIED | `reflect.ValueOf`, `getFieldMap`, `fieldAccessor`, `sync.Map` caching |
| `ent/schema/network.go` | updated and created indexes | VERIFIED | `index.Fields("updated")`, `index.Fields("created")` present |
| All 13 schemas | updated and created indexes | VERIFIED | 13 schemas each have 1 updated + 1 created index (grep counts confirm) |
| `internal/httperr/problem.go` | RFC 9457 ProblemDetail + WriteProblem | VERIFIED | ProblemDetail struct, WriteProblem, NewProblemDetail, WriteProblemInput, `application/problem+json` |
| `internal/httperr/problem_test.go` | Tests for problem detail | VERIFIED | TestWriteProblem exists |
| `internal/web/termrender/dispatch.go` | Generic Register function + dispatch map | VERIFIED | `Register[T any]`, `renderers` map, `init()` registers 8 types |
| `internal/web/termrender/dispatch_test.go` | Tests for dispatch | FAILED (build error) | Line 55 uses `TotalCount: 1` -- field was renamed to `HasMore` in plan 01 |
| `internal/web/detail.go` | Refactored handlers with queryXxx methods | VERIFIED | 6 queryXxx methods, 6 handlers at 29 lines each |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `internal/web/search.go` | `internal/web/handler.go` | `TypeResult.HasMore` field | WIRED | handler.go line 185: `HasMore: r.HasMore` |
| `internal/web/handler.go` | `internal/web/templates/searchtypes.go` | `convertToSearchGroups HasMore mapping` | WIRED | `HasMore: r.HasMore` maps TypeResult.HasMore to SearchGroup.HasMore |
| `internal/pdbcompat/handler.go` | `internal/httperr/problem.go` | `httperr.WriteProblem` | WIRED | 7 call sites in handler.go use `WriteProblem(w, httperr.WriteProblemInput{...})` via pdbcompat wrapper |
| `internal/web/render.go` | `internal/httperr/problem.go` | `httperr.NewProblemDetail` | WIRED | Lines 101, 106 use `httperr.NewProblemDetail` for 404 and 500 JSON responses |
| `cmd/peeringdb-plus/main.go` | `internal/httperr/problem.go` | `restErrorMiddleware` | WIRED | Line 211: `restErrorMiddleware(restSrv.Handler())`, line 412: `httperr.WriteProblem` |
| `internal/web/termrender/dispatch.go` | `internal/web/termrender/renderer.go` | `renderers[reflect.TypeOf(data)]` | WIRED | renderer.go line 74 looks up in dispatch map |
| `internal/web/detail.go` | `internal/web/templates` | `queryXxx returns templates.XxxDetail` | WIRED | All 6 queryXxx methods have return type `templates.XxxDetail` |

### Data-Flow Trace (Level 4)

Not applicable for this phase -- artifacts are query optimization, error formatting, and architectural refactoring, not data-rendering components.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Project builds | `go build ./...` | Clean (exit 0) | PASS |
| web package tests | `go test -race ./internal/web/` | ok (5.887s) | PASS |
| pdbcompat tests | `go test -race ./internal/pdbcompat/` | ok (2.756s) | PASS |
| httperr tests | `go test -race ./internal/httperr/` | ok (1.017s) | PASS |
| termrender tests | `go test -race ./internal/web/termrender/` | build failed | FAIL |
| go vet (all phase pkgs) | `go vet ./internal/web/ ./internal/web/termrender/ ./internal/pdbcompat/ ./internal/httperr/` | termrender dispatch_test.go:55 unknown field TotalCount | FAIL |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| PERF-01 | 34-01 | Single query per entity type (no separate count) | SATISFIED | 6 query methods use Limit(displayLimit+1), no Count() calls |
| PERF-03 | 34-01 | Database indexes on updated and created fields | SATISFIED | 13 schemas have both indexes; ent migration regenerated with named indexes |
| PERF-05 | 34-01 | Field projection avoids JSON roundtrip | SATISFIED | reflect.ValueOf-based itemToMap, zero json.Marshal/Unmarshal in search.go |
| ARCH-01 | 34-02 | Unified error format across API surfaces | SATISFIED (scoped) | RFC 9457 on pdbcompat, web JSON, REST; ConnectRPC/GraphQL kept per-protocol formats per CONTEXT.md decisions |
| ARCH-04 | 34-03 | Interface-based terminal renderer dispatch | SATISFIED (code, not test) | Register[T] generic dispatch map replaces type-switch; test has build error |
| QUAL-04 | 34-03 | Web detail handlers under 80 lines each | SATISFIED | All 6 handlers are 29 lines; queryXxx methods extracted |

No orphaned requirements -- all 6 requirement IDs from REQUIREMENTS.md mapped to Phase 34 appear in plan frontmatter.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `internal/web/termrender/dispatch_test.go` | 55 | Stale field reference `TotalCount` (deleted in plan 01, used in plan 03) | BLOCKER | Breaks test compilation for entire termrender package |

### Human Verification Required

### 1. REST Error Middleware End-to-End

**Test:** Send `GET /rest/v1/nonexistent_resource` to the running server
**Expected:** Response has Content-Type `application/problem+json` with body containing `type`, `title`, `status`, `detail` fields
**Why human:** REST error middleware uses response capture pattern that requires a running server to test end-to-end

### 2. EXPLAIN QUERY PLAN on Updated/Created Indexes

**Test:** Run `EXPLAIN QUERY PLAN SELECT * FROM networks WHERE updated > ?` on a migrated SQLite database
**Expected:** Output shows `SEARCH networks USING INDEX network_updated`
**Why human:** Requires a SQLite instance with the ent migration applied; no in-repo test currently exercises EXPLAIN

### Gaps Summary

**One gap found:** The `dispatch_test.go` file created by plan 03 references `TotalCount` on line 55, a field that was renamed to `HasMore` by plan 01. This is a cross-plan coordination issue -- plan 03 was executed in a worktree and used stale struct definitions. The fix is a one-line change: replace `TotalCount: 1` with `HasMore: false` (or `HasMore: true`).

All other truths, artifacts, and key links are verified. The build compiles cleanly; only the test file has a compilation error. The three other test suites (web, pdbcompat, httperr) pass with race detector.

**Note on ARCH-01 scope:** The ROADMAP success criterion mentions "6 API surfaces" with "code, message, details" fields, while the implementation uses RFC 9457 (`type`, `title`, `status`, `detail`) on 3 HTTP surfaces (pdbcompat, web JSON, REST). ConnectRPC and GraphQL were excluded by explicit design decisions in CONTEXT.md and the plans. The field name difference (RFC 9457 vs generic code/message) was a deliberate architectural choice documented in the phase decisions. This is a scoping refinement, not a gap.

---

_Verified: 2026-03-26T08:30:00Z_
_Verifier: Claude (gsd-verifier)_
