---
phase: 67
plan: 06
subsystem: cross-surface-verification
tags: [ordering, e2e, cross-surface, docs, verification, wave-5]
completed_at: 2026-04-19
milestone: v1.16
requires: [67-01, 67-02, 67-03, 67-04, 67-05]
provides:
  - TestOrdering_CrossSurface — 3-surface compound (-updated, -created, -id) parity proof (Network representative)
  - TestEntrestDefaultOrder — entrest ORDER-03 override contract (default + explicit id asc/desc)
  - TestEntrestNestedSetOrder — D-04 clarification lock (edges.network_ix_lans DESC by updated)
  - docs/ARCHITECTURE.md § Ordering — operator-facing documentation of the compound default
affects:
  - cmd/peeringdb-plus/ordering_cross_surface_e2e_test.go (new, 574 lines)
  - docs/ARCHITECTURE.md (new § Ordering between API surfaces and Privacy layer)
tech-stack:
  added: []
  patterns:
    - multi-surface-httptest-fixture (pdbcompat + entrest + ConnectRPC on one mux)
    - per-subtest atomic DB counter for parallel-safe in-memory SQLite
    - envelope-agnostic id extractor (fetchEnvelopeIDs for data vs content keys)
key-files:
  created:
    - cmd/peeringdb-plus/ordering_cross_surface_e2e_test.go
  modified:
    - docs/ARCHITECTURE.md
decisions:
  - Scoped the cross-surface test to pdbcompat + entrest + ConnectRPC NetworkService only. GraphQL (Relay cursor) and Web UI are out-of-scope for ORDER-01..03 per CONTEXT.md. GraphQL and Web UI are documented as out-of-scope in the new ARCHITECTURE.md § Ordering paragraph, not asserted.
  - Skipped buildMiddlewareChain wrap. The ordering contract lives below middleware (in ent query builders / entrest templates / grpcserver QueryBatch closures), so a bare mux + httptest.NewServer isolates the assertion. This avoided pulling in readiness/CSP/MaxBytesBody/logging surfaces irrelevant to ordering.
  - Nested _set test uses Network → NetworkIxLan hierarchy (not a made-up pair). The Network schema declares `edge.To("network_ix_lans", ...).Annotations(entrest.WithEagerLoad(true))` at `ent/schema/network.go:202`, so /rest/v1/networks auto-eagerloads that edge unconditionally. There is no `?depth=N` query param — entrest's auto-eagerload is schema-declarative. Plan's phrasing "depth=2" is a codename for "depth >= 1 eager-loaded edge"; asserted against `content[0].edges.network_ix_lans[]`.
  - Timestamp monotonicity check added alongside the id-order check on the nested _set test. The real contract is "DESC by updated"; ids only correlate because of how we seeded. The extra loop guards against a future seed refactor breaking that correlation silently.
metrics:
  duration: ~25m
  tasks: 2
  files: 2
  commits: [3b40536, cdbb8e4]
requirements:
  - ORDER-01
  - ORDER-02
  - ORDER-03
---

# Phase 67 Plan 06: Cross-surface ordering parity E2E + docs Summary

## One-liner

Closed Phase 67 by landing a 574-line cross-surface integration test that asserts pdbcompat `/api/net`, entrest `/rest/v1/networks`, and ConnectRPC `ListNetworks` all return identical row order under the new compound `(-updated, -created, -id)` default (including both tie-break fallbacks and the D-04-clarification nested `edges.network_ix_lans` case), plus a new `## Ordering` section in `docs/ARCHITECTURE.md` documenting the contract for operators.

## What shipped

Two atomic commits, plan task order honoured (tests → docs):

1. **`3b40536` — Task 1: cross-surface E2E test** — `cmd/peeringdb-plus/ordering_cross_surface_e2e_test.go` (new, 574 lines). Three top-level `Test*` functions covering all 7 sub-tests the plan's must_haves enumerate.
2. **`cdbb8e4` — Task 2: operator docs** — `docs/ARCHITECTURE.md` (+33 lines, single `## Ordering` section inserted between the existing `## API surfaces` and `## Privacy layer` sections).

Expected state — all 7 sub-tests PASS on first run, because Waves 1-4 (Plans 01-05) already landed the contract on each individual surface. This plan is a goal-backward verification, not a change to behaviour.

## Task record

### Task 1 — Cross-surface E2E test (commit `3b40536`)

Created a single test file with three top-level tests plus a shared fixture:

- `buildOrderingFixture(t)` — isolated in-memory SQLite via `enttest.Open` + atomic DB counter (parallel-safe); wires pdbcompat, entrest, and ConnectRPC `NetworkService` onto a fresh `http.ServeMux` and wraps in `httptest.NewServer`. Skips the production middleware chain — ordering is below it.
- `seedNetworksWithUpdated(t, client, n, base)` — creates n networks with ids 1..n, each with `updated := base + (i-1)*1h`, shared `created = base`. Returns the expected DESC id order (`[n, n-1, ..., 1]`).
- Three fetch helpers: `fetchPdbcompatNetworkIDs` (decodes `{"data":[]}` envelope), `fetchEntrestNetworkIDs` (decodes `{"content":[]}` envelope with optional query params), `fetchGrpcNetworkIDs` (uses the generated `peeringdbv1connect.NetworkServiceClient`).
- `fetchEnvelopeIDs` — envelope-key-agnostic inner helper factoring out the duplicated JSON decoding.

**`TestOrdering_CrossSurface`** — three sub-tests, all `t.Parallel()`:

| Sub-test | Seed | Expected order on ALL 3 surfaces | Pass |
| --- | --- | --- | --- |
| `Network` | 3 networks, `updated` spread 1h | `[3, 2, 1]` | ✓ |
| `TieBreakCreated` | 2 networks, same `updated`, different `created` | `[11, 10]` (later created wins) | ✓ |
| `TieBreakID` | 2 networks, same `updated` + `created`, different `id` | `[99, 10]` (higher id wins) | ✓ |

Each sub-test also asserts pairwise equality of the three slices — the cross-surface parity contract is stronger than each surface-independent assertion.

**`TestEntrestDefaultOrder`** — three sub-tests covering ORDER-03 override contract:

| Sub-test | URL | Expected order |
| --- | --- | --- |
| `default` | `/rest/v1/networks` (no query) | `[3, 2, 1]` — compound default |
| `explicit_id_asc` | `/rest/v1/networks?sort=id&order=asc` | `[1, 2, 3]` — override honoured |
| `explicit_id_desc` | `/rest/v1/networks?sort=id&order=desc` | `[3, 2, 1]` |

Confirms Plan 02's template override only activates on the default path (`?sort=` absent or matching the declared default field); explicit sorts are untouched.

**`TestEntrestNestedSetOrder/depth2`** — locks in the CONTEXT.md D-04 clarification. Seeds `Org → Network(1) → 3 NetworkIxLan(10,20,30)` with spread `updated` timestamps. GET `/rest/v1/networks` and asserts `content[0].edges.network_ix_lans[].id == [30, 20, 10]`. Also adds a timestamp-monotonicity loop so a future seed refactor cannot silently break the contract.

**Note on plan's "depth=2" phrasing.** Entrest does NOT accept a `?depth=N` query parameter. Nested edges are eager-loaded per schema annotation (`entrest.WithEagerLoad(true)` on the `Network.network_ix_lans` edge at `ent/schema/network.go:202`). So the response shape is always `{"content":[{"id":..., "edges":{"network_ix_lans":[{...}]}}]}` for `/rest/v1/networks`, with no extra flag. The plan's "depth=2" is a codename for "depth ≥ 1 eager-loaded edge" — confirmed by inspecting `ent/rest/eagerload.go:146-160` (the `EagerLoadNetwork` func that calls `WithNetworkIxLans(func(e) { applySortingNetworkIxLan(e, "updated", "desc"); ... })`).

**Verification (Task 1 `<verify>`):**

```
$ go test -race -run 'TestOrdering_CrossSurface|TestEntrestDefaultOrder|TestEntrestNestedSetOrder' ./cmd/peeringdb-plus/... -count=1 -v
--- PASS: TestEntrestNestedSetOrder (0.00s)
    --- PASS: TestEntrestNestedSetOrder/depth2 (0.31s)
--- PASS: TestEntrestDefaultOrder (0.00s)
    --- PASS: TestEntrestDefaultOrder/explicit_id_desc (0.31s)
    --- PASS: TestEntrestDefaultOrder/explicit_id_asc (0.31s)
    --- PASS: TestEntrestDefaultOrder/default (0.31s)
--- PASS: TestOrdering_CrossSurface (0.00s)
    --- PASS: TestOrdering_CrossSurface/TieBreakID (0.32s)
    --- PASS: TestOrdering_CrossSurface/TieBreakCreated (0.32s)
    --- PASS: TestOrdering_CrossSurface/Network (0.33s)
PASS
ok  	github.com/dotwaffle/peeringdb-plus/cmd/peeringdb-plus	1.450s
```

### Task 2 — docs/ARCHITECTURE.md § Ordering (commit `cdbb8e4`)

Added a new `## Ordering` section (33 lines, between `## API surfaces` and `## Privacy layer`) covering:

1. Compound default definition + upstream alignment (`django-handleref` `Meta.ordering = ("-updated", "-created")` plus `id DESC` tiebreak).
2. Per-surface implementation split (pdbcompat + grpcserver direct ent `.Order()`; entrest template override; grpcserver keyset cursor).
3. Nested `_set` arrays at depth ≥ 1 behaviour (D-04 clarification).
4. GraphQL/Web UI explicitly out-of-scope.
5. Performance note: `updated` index on all 13 entities + operator verification command (`sqlite3 ... '.schema'`).

**Verification (Task 2 `<verify>`):**

```
$ grep -n "^## Ordering\|updated, -created, -id\|depth" docs/ARCHITECTURE.md | head -5
260:## Ordering
262:All list endpoints return rows in compound `(-updated, -created, -id)` order by default, matching
277:- **Nested `_set` arrays at depth ≥ 1** (entrest only): entrest's eager-load template calls
```

All three acceptance grep checks pass.

## Files changed

| File | Change | Kind |
|---|---|---|
| `cmd/peeringdb-plus/ordering_cross_surface_e2e_test.go` | new, 574 lines | Task 1 cross-surface test |
| `docs/ARCHITECTURE.md` | +33 lines (new § Ordering) | Task 2 operator docs |

Total: **2 files changed, 607 insertions(+)** — split across commits `3b40536` + `cdbb8e4`.

## Acceptance criteria

| ID | Criterion | Status | Evidence |
|---|---|---|---|
| T1-1 | File exists ≥200 lines | PASS | 574 lines |
| T1-2 | All 7 sub-tests pass | PASS | `go test -run 'TestOrdering_CrossSurface\|TestEntrestDefaultOrder\|TestEntrestNestedSetOrder'` → ok |
| T1-3 | `go test -race ./...` passes full suite | PASS | All 23 test-bearing packages ok |
| T2-1 | grep `Ordering` heading matches | PASS | `^## Ordering` at line 260 |
| T2-2 | Section mentions compound default, cross-surface parity, entrest template override, nested _set, keyset cursor, updated index | PASS | All six items present |
| T2-3 | Section ≥5 lines | PASS | 33 lines |

**Success criteria (plan-level):**

| ID | Criterion | Status |
|---|---|---|
| S-1 | Cross-surface parity test passes | PASS |
| S-2 | entrest override contract test passes | PASS |
| S-3 | Nested _set order test passes (D-04 lock) | PASS |
| S-4 | Documentation updated | PASS |
| S-5 | ORDER-01/02/03 verified end-to-end | PASS — Plans 03/04/05 assert per-surface; this plan asserts parity across all three |

## Deviations from plan

None — all 7 sub-tests passed on first run against the post-Wave-4 codebase, confirming the predecessor plans landed the contract correctly. Two minor on-plan adjustments worth flagging:

### 1. Removed the unused `connect.NewRequest` wrapping

Initial draft followed the field_privacy_e2e_test.go pattern of `cl.X(ctx, connect.NewRequest(&...))`. The generated `networkServiceClient.ListNetworks` signature is `(ctx, req *pb.ListNetworksRequest) (*pb.ListNetworksResponse, error)` — the client wraps/unwraps connect.Request/Response internally (`gen/peeringdb/v1/peeringdbv1connect/services.connect.go:1322`). The wrapping imports `connectrpc.com/connect` as an unused package. Dropped the import + the `connect.NewRequest(...)` call. Single-line change; same semantics.

### 2. Plan's "depth=2" interpreted as "depth ≥ 1 eager-loaded edge"

Entrest does NOT have a `?depth=N` query param. The plan's phrasing was inherited from PeeringDB's pdbcompat `?depth=N` convention — but the clarification in CONTEXT.md D-04 explicitly refers to entrest's automatic eager-loading of edges with `entrest.WithEagerLoad(true)`. The test asserts against `content[0].edges.network_ix_lans[]`, which entrest produces unconditionally on `/rest/v1/networks`. Documented inline in the test's godoc + in this summary.

## Authentication gates

None.

## Known Stubs

None.

## Threat Flags

None — no new network endpoints, auth paths, file access, or schema changes. Test-only + docs-only changes; threat model T-67-06 accepts this per the plan.

## Signals for the phase verifier

```bash
# 1. Test file exists, ≥200 lines
wc -l cmd/peeringdb-plus/ordering_cross_surface_e2e_test.go
# → 574 cmd/peeringdb-plus/ordering_cross_surface_e2e_test.go

# 2. All 3 top-level tests defined
grep -c '^func Test' cmd/peeringdb-plus/ordering_cross_surface_e2e_test.go
# → 3

# 3. All 7 sub-tests pass
go test -race -run 'TestOrdering_CrossSurface|TestEntrestDefaultOrder|TestEntrestNestedSetOrder' \
  ./cmd/peeringdb-plus/... -count=1 -v 2>&1 | grep -c '^    --- PASS'
# → 7

# 4. Full suite green
go test -race ./... -count=1
# → ok (all 23 test-bearing packages)

# 5. ARCHITECTURE.md § Ordering present + substantive
grep -A 30 '^## Ordering' docs/ARCHITECTURE.md | wc -l
# → 31 (heading + 30 lines of content)
grep -c '^## Ordering\|updated, -created, -id\|depth' docs/ARCHITECTURE.md
# → ≥3

# 6. Lint clean
golangci-lint run ./cmd/peeringdb-plus/...
# → 0 issues.

# 7. Commits on main
git log --oneline -3
# → cdbb8e4 docs(67-06): document compound default ordering + cross-surface parity
# → 3b40536 test(67-06): add cross-surface ordering parity + entrest override + nested _set E2E
# → 93b61b8 docs(67-05): complete grpcserver default-ordering flip plan
```

## Self-Check

Files:

- `cmd/peeringdb-plus/ordering_cross_surface_e2e_test.go` — FOUND, 574 lines, contains `TestOrdering_CrossSurface`, `TestEntrestDefaultOrder`, `TestEntrestNestedSetOrder`.
- `docs/ARCHITECTURE.md` — FOUND, contains `## Ordering` at line 260.

Commits:

- `3b40536` — FOUND in git log (`test(67-06): add cross-surface ordering parity + entrest override + nested _set E2E`).
- `cdbb8e4` — FOUND in git log (`docs(67-06): document compound default ordering + cross-surface parity`).

## Self-Check: PASSED

## PLAN 67-06 COMPLETE

**Commits:**
- `3b40536` — `test(67-06): add cross-surface ordering parity + entrest override + nested _set E2E`
- `cdbb8e4` — `docs(67-06): document compound default ordering + cross-surface parity`
