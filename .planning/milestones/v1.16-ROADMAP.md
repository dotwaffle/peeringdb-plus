# Roadmap: PeeringDB Plus

## Milestones

- ✅ **v1.0 – v1.14** — shipped (see [MILESTONES.md](./MILESTONES.md))
- ✅ **v1.15 — Infrastructure Polish & Schema Hygiene** — shipped 2026-04-18 (Phases 63-66, 11 requirements)
- ✅ **v1.16 — Django-compat Correctness** — complete 2026-04-19 (Phases 67-72, 25 requirements; ready for `/gsd-complete-milestone` archival)

## Phases

**v1.16 — Django-compat Correctness (current)**

- [x] **Phase 67: Default ordering flip** — Pdbcompat, grpcserver, entrest list paths return rows in upstream `(-updated, -created)` order
- [x] **Phase 68: Status × since matrix + limit=0 semantics** — Pdbcompat applies upstream status-filter rules and treats `limit=0` as unlimited
- [ ] **Phase 69: Filter-value Unicode folding, operator coercion, `__in` robustness** — Pdbcompat filter layer matches upstream `rest.py:544-662` value handling
- [x] **Phase 70: Cross-entity `__` traversal (Path A + Path B + 2-hop)** — Pdbcompat resolves filter paths across FKs like upstream serializers
- [x] **Phase 71: Memory-safe response paths on 256 MB replicas** — Depth=2 / `limit=0` / large traversals stay within the replica memory envelope
- [x] **Phase 72: Upstream parity regression + divergence docs** — Ported `pdb_api_test.py` ground-truth tests lock the new semantics

<details>
<summary>✅ v1.15 — Infrastructure Polish & Schema Hygiene (Phases 63-66) — SHIPPED 2026-04-18</summary>

- [x] Phase 63: Schema hygiene — drop vestigial columns (1 plan)
- [x] Phase 64: Field-level privacy — ixlan.ixf_ixp_member_list_url (3 plans)
- [x] Phase 65: Asymmetric Fly fleet — process groups + ephemeral replicas (2 plans)
- [x] Phase 66: Observability + sqlite3 tooling (3 plans)

Archive: [`.planning/milestones/v1.15-ROADMAP.md`](./milestones/v1.15-ROADMAP.md)
Requirements: [`.planning/milestones/v1.15-REQUIREMENTS.md`](./milestones/v1.15-REQUIREMENTS.md)
Audit: [`.planning/milestones/v1.15-MILESTONE-AUDIT.md`](./milestones/v1.15-MILESTONE-AUDIT.md)

</details>

All shipped milestones are summarised in [MILESTONES.md](./MILESTONES.md). Per-milestone ROADMAP snapshots live at `.planning/milestones/v{X.Y}-ROADMAP.md`, and phase artifacts (plans, summaries, verification reports) at `.planning/milestones/v{X.Y}-phases/` (archived) or `.planning/phases/` (current milestone).

## Phase Details

### Phase 67: Default ordering flip
**Goal**: List endpoints across all three query surfaces return rows in upstream PeeringDB's `(-updated, -created)` order instead of the current `id ASC`, matching `django-handleref` base `Meta.ordering`.
**Depends on**: Nothing (baseline parity fix; touches list paths broadly but is behaviourally self-contained).
**Requirements**: ORDER-01, ORDER-02, ORDER-03
**Success Criteria** (what must be TRUE):
  1. `curl /api/net` (pdbcompat) returns rows sorted by `updated DESC`, tie-broken by `created DESC`, matching upstream response ordering
  2. `ListNetworks` / `StreamNetworks` and analogous ConnectRPC List/Stream RPCs for all 13 types return rows in the same `(-updated, -created)` order; cursor pagination remains stable across pages
  3. `/rest/v1/*` list endpoints default to `(-updated, -created)` while still honouring explicit `?sort=` overrides for non-default orderings
  4. Existing streaming RPC cursor-resume semantics (`since_id`, `updated_since`) continue to work under the new default order
**Plans:** 6/6 plans executed
- [x] 67-01-PLAN.md — Add `index.Fields("updated")` + `entrest.WithDefaultSort/Order` + `WithSortable(true)` to all 13 ent schemas + lock-step generator template (D-02, D-08)
- [x] 67-02-PLAN.md — `entc.TemplateDir` override for compound `(-updated, -created, -id)` entrest default (D-07) — embeds exact upstream sorting.tmpl variables
- [x] 67-03-PLAN.md — Flip 13 pdbcompat `.Order()` calls to compound + regenerate 39 goldens + new TestDefaultOrdering_Pdbcompat
- [x] 67-04-PLAN.md — Add `streamCursor` type + encode/decode helpers in grpcserver/pagination.go (Plan 05 dep)
- [x] 67-05-PLAN.md — Flip 26 grpcserver order sites + StreamParams signature + 13 QueryBatch keyset closures + tests
- [x] 67-06-PLAN.md — Cross-surface E2E test (pdbcompat + entrest + grpcserver parity + nested _set per D-04 clarification) + docs/ARCHITECTURE.md § Ordering

### Phase 68: Status × since matrix + limit=0 semantics
**Goal**: Pdbcompat response filtering matches the upstream `rest.py:494-727` rules for `?status`, `?since`, and `?limit`, and the `PDBPLUS_INCLUDE_DELETED` gate is re-scoped as a sync-side knob only.
**Depends on**: Phase 67 (shared pdbcompat list-path code — order flip lands first so status matrix builds on the new baseline).
**Requirements**: STATUS-01, STATUS-02, STATUS-03, STATUS-04, STATUS-05, LIMIT-01, LIMIT-02
**Success Criteria** (what must be TRUE):
  1. `GET /api/<type>` (no `since`) returns only `status=ok` rows for every entity type; `GET /api/<type>/{pk}` additionally admits `status=pending`
  2. `GET /api/<type>?since={ts}` returns rows with `status IN (ok, deleted)` for all types, and additionally `pending` for `/api/campus`
  3. `GET /api/<type>?status=deleted` (no `since`) returns an empty set — the final `status=ok` list filter is applied unconditionally
  4. `GET /api/<type>?limit=0` returns every matching row with no upper bound (NOT a count envelope); `depth>0` responses continue to cap at the upstream `API_DEPTH_ROW_LIMIT=250`
  5. `PDBPLUS_INCLUDE_DELETED` controls whether sync persists deleted rows locally; pdbcompat status-matrix filtering is independent of this gate and matches upstream regardless of its value
**Plans:** 4/4 plans executed
- [x] 68-01-PLAN.md — Remove PDBPLUS_INCLUDE_DELETED from Config + WorkerConfig + syncIncremental; delete filterByStatus helper; strip test fixtures
- [x] 68-02-PLAN.md — Soft-delete flip: rename 13 deleteStale* → markStaleDeleted*; plumb cycleStart through syncStep.deleteFn; add TestSync_SoftDeleteMarksRows
- [x] 68-03-PLAN.md — pdbcompat status matrix helper + 13 list-closure edits + 26 depth.go StatusIn predicates + limit=0 gate fix + list-depth guardrail + status_matrix_test.go
- [x] 68-04-PLAN.md — CHANGELOG.md bootstrap + docs/API.md Known Divergences + CLAUDE.md soft-delete hygiene note + final REQ-ID coverage audit

### Phase 69: Filter-value Unicode folding, operator coercion, `__in` robustness
**Goal**: The pdbcompat filter layer reproduces upstream value-handling: `unidecode`-equivalent folding before SQL, `__contains`/`__startswith` coerced to case-insensitive variants, and arbitrarily-large `__in` lists without SQLite's 999-variable limit.
**Depends on**: Phase 67 (shares `internal/pdbcompat/filter.go`; sequencing avoids merge pain with ordering work).
**Requirements**: IN-01, IN-02, UNICODE-01, UNICODE-02, UNICODE-03
**Success Criteria** (what must be TRUE):
  1. `GET /api/net?name__contains=Zürich` matches rows whose name contains `zurich` (diacritic-insensitive); `?name__startswith=Zü` matches rows starting with `zu` — both operators behave case-insensitively
  2. `GET /api/net?id__in={csv of 5000 ids}` returns the expected rows without tripping SQLite's 999-parameter ceiling (single-bind `json_each` rewrite verified in the query plan)
  3. `GET /api/net?id__in=` (empty list) returns an empty result set, matching Django ORM semantics
  4. The `ParseFilters` fuzz corpus includes non-ASCII inputs (diacritics, CJK, combining marks, ZWJ); a fresh fuzz run produces zero panics
  5. Behaviour for ASCII-only inputs is unchanged — existing golden files and conformance tests pass without regeneration
**Plans:** 6 plans
- [x] 69-01-PLAN.md — internal/unifold package (Fold + NFKD + hand-rolled fold map + unit tests)
- [x] 69-02-PLAN.md — 16 field.String("*_fold") across 6 ent schemas + scoped ent regen (entgql.Skip+entrest.WithSkip annotations keep _fold off all wire surfaces; commit 9e408de)
- [x] 69-03-PLAN.md — 6 upsertX funcs populate _fold columns via unifold.Fold (16 calls wired); 7 unaffected upserts byte-identical; golang.org/x/text promoted to direct dep; commits 8ce16ab+cdad023
- [x] 69-04-PLAN.md — filter.go: coerceToCaseInsensitive + shadow-column routing + json_each __in + empty-__in sentinel; 16 folded fields across 6 TypeConfigs; 13 EmptyResult guards in list closures; commits 9839273 (RED) + 9aa661d (GREEN)
- [x] 69-05-PLAN.md — fuzz corpus extension (469k local execs, 0 panics) + build-tag-gated bench shim + shadow-index decision DEFERRED (benchstat n=6: shadow within 1% of direct at 10k rows, p=0.065); commit 298033d
- [x] 69-06-PLAN.md — CHANGELOG + docs/API.md § Known Divergences row + CLAUDE.md § Shadow-column folding (Phase 69) convention + REQ-ID audit (5/5 grep-verified); Phase 69 CLOSED; commit 0b1e6a1

### Phase 70: Cross-entity `__` traversal (Path A + Path B + 2-hop)
**Goal**: Pdbcompat resolves `<fk>__<field>` and `<fk>__<fk>__<field>` filter paths the way upstream does — per-serializer `prepare_query` allowlists (Path A) AND automatic relation introspection minus a `FILTER_EXCLUDE` list (Path B), with 2-hop support.
**Depends on**: Phase 69 (shared filter.go surface; traversal builds on the Unicode/operator-coercion layer).
**Requirements**: TRAVERSAL-01, TRAVERSAL-02, TRAVERSAL-03, TRAVERSAL-04
**Success Criteria** (what must be TRUE):
  1. All 13 entity types have per-serializer `prepare_query` allowlists covering the 11 distinct shapes identified in the validation audit; each allowed 1-hop filter (e.g. `net?org__name=X`) returns the correct rows
  2. `queryable_relations()`-equivalent introspection walks ent FK edges and exposes every relation not in the documented `FILTER_EXCLUDE` list, enabling Path B traversal without per-entity boilerplate
  3. 2-hop traversal works: `GET /api/fac?ixlan__ix__fac_count__gt=0` resolves through netixlan → ix → fac_count and returns the same rows as the upstream `pdb_api_test.py:2340,2348` assertion
  4. Unknown filter fields (typos, deprecated names) are silently ignored with a 200 response rather than a 400 — matches upstream `rest.py:544-662` and avoids breaking existing integrations
  5. The documented `FILTER_EXCLUDE` list is recorded in `docs/API.md` so operators can predict which relations are intentionally un-traversable
**Plans:** 8/8 plans executed
- [x] 70-01-PLAN.md — pdbcompat annotation types (WithPrepareQueryAllow, WithFilterExcludeFromTraversal, AllowlistEntry); internal/pdbcompat/annotations.go + annotations_test.go land 92+68 LOC; Name() strings locked by round-trip test; commits 268346b (feat) + 41f2ceb (test)
- [x] 70-02-PLAN.md — cmd/pdb-compat-allowlist codegen tool + ent/generate.go wiring + allowlist_gen.go bootstrap; 299+99+19 LOC (main.go + main_test.go + allowlist_gen.go); entc.LoadGraph-based schema walk with deterministic sort-before-render; two-run SHA256 byte-stable (6b0857fd...); commit dd8ffcc
- [x] 70-03-PLAN.md — 13 ent schema WithPrepareQueryAllow annotations mirroring upstream serializers.py@99e92c72 (line refs 1823/2244/2361/2423/2573/2732/2947/2995/3315/3451/3622/3925/4041); populated Allowlists map regenerated into allowlist_gen.go; commit 1c1c5b4
- [x] 70-04-PLAN.md — Edges map emission + internal/pdbcompat/introspect.go (LookupEdge/ResolveEdges/TargetFields); codegen-time static emission per amended D-02 (no runtime client.Schema.Tables walk, no sync.Once, no init-order coupling); commit a1319a1
- [x] 70-05-PLAN.md — parseFieldOp 3-tuple (relationSegments, finalField, op) + ParseFiltersCtx (ctx-threaded unknown-field accumulator) + 1-hop/2-hop predicate builders + handler diagnostics (slog.DebugContext + OTel attr `pdbplus.filter.unknown_fields`); commit 7e0d3f4
- [x] 70-06-PLAN.md — Exhaustive E2E traversal matrix (17 subtests in traversal_e2e_test.go) + dynamic 13-entity Path A coverage in filter_test.go + 3 Phase 68/69 regression guards in handler_test.go; seed.Full gains 8 fixture rows at IDs 8001+; DEFER-70-06-01 logged; commits 88472a1 + a27f092 + 4d93383 + e5834da
- [x] 70-07-PLAN.md — BenchmarkTraversal_1Hop_Direct + BenchmarkTraversal_2Hop_UpstreamParity + BenchmarkTraversal_2Hop_WithLimitAndSkip + TestBenchTraversal_D07_Ceiling go-test-time gate (<50ms/op @ 10k rows) + nightly `.github/workflows/bench.yml` CI regression-gating via benchstat; commit 4a0bd45
- [x] 70-08-PLAN.md — CHANGELOG.md v1.16 [Unreleased] Phase 70 bullets + docs/API.md § Cross-entity traversal (Phase 70) + 2 Known Divergences rows + CLAUDE.md § Cross-entity __ traversal convention + REQUIREMENTS.md traceability close; Phase 70 CLOSED

### Phase 71: Memory-safe response paths on 256 MB replicas
**Goal**: Depth=2, `limit=0`, and wide traversal queries stream JSON encoding with bounded intermediate allocations, enforce a per-response memory ceiling that trips before Fly OOM-kills the machine, and expose per-request heap/RSS telemetry.
**Depends on**: Phase 67, 68, 69, 70 (must know the worst-case response shapes those phases enable before the ceiling and streaming harness can be sized correctly).
**Requirements**: MEMORY-01, MEMORY-02, MEMORY-03, MEMORY-04
**Success Criteria** (what must be TRUE):
  1. A `depth=2` or `limit=0` pdbcompat response streams bytes through the `http.ResponseWriter` without materialising the full result set to a single `[]byte` or `[]T` slice before the first flush
  2. A response exceeding `PDBPLUS_RESPONSE_MEMORY_LIMIT` (default sized against the 256 MB replica budget minus operating headroom) is truncated with an RFC 9457 `application/problem+json` 413 response before the process gets OOM-killed
  3. Per-request OTel span attributes and a Prometheus gauge expose response-path peak heap and RSS, reusing the v1.15 Phase 66 `runtime.MemStats` harness; Grafana's SEED-001 watch row shows response-path peaks alongside sync-path peaks
  4. `docs/ARCHITECTURE.md` documents the per-endpoint memory envelope (worst-case depth/limit combinations per type) and the operator-actionable knobs that cap it
**Plans:** 6/6 plans executed
- [x] 71-01-PLAN.md — internal/pdbcompat/stream.go StreamListResponse + iterator shape + 7 unit tests (Wave 1, isolated)
- [x] 71-02-PLAN.md — internal/pdbcompat/rowsize.go typical_row_bytes map + bench_row_size_test.go calibration (13 types × 2 depths, doubled per D-03; calibrated 2026-04-19 from seed.Full at benchtime=20x × count=3)
- [x] 71-03-PLAN.md — Config.ResponseMemoryLimit (default 128 MiB via parseByteSize) + internal/pdbcompat/budget.go CheckBudget + WriteBudgetProblem (D-04 RFC 9457 413); docs/CONFIGURATION.md env-var row + validation row
- [x] 71-04-PLAN.md — pdbcompat registry CountFunc sibling + serveList pre-flight budget check + StreamListResponse wiring; 2 commits (27ba127 registry refactor + 28408e1 handler wiring); 4 new integration tests; 13 list-path goldens regenerated for D-07 trailing-newline divergence; MEMORY-01 + MEMORY-02 shipped
- [x] 71-05-PLAN.md — internal/pdbcompat/telemetry.go recordResponseHeapDelta + OTel span attr + Prometheus histogram + Grafana panel (Wave 5); 2 commits (c2304ae metric registration + 292e758 sampler + handler wire + dashboard); 5 new pdbcompat tests + 3 new otel tests; panel id 36 at y=33 with p50/p95/p99 by endpoint; MEMORY-03 shipped
- [x] 71-06-PLAN.md — docs/ARCHITECTURE.md § Response Memory Envelope + CHANGELOG v1.16 Phase 71 Added bullets + coordinated-release ship-ready note + CLAUDE.md § Response memory envelope (Phase 71) convention + PDBPLUS_RESPONSE_MEMORY_LIMIT env-var row + REQ-ID audit (MEMORY-01..04 all grep-verifiable); MEMORY-04 shipped; Phase 71 CLOSED

### Phase 72: Upstream parity regression + divergence docs
**Goal**: Lock the v1.16 semantics in place by porting the ground-truth assertions from upstream `pdb_api_test.py` and documenting any intentional divergences so future conformance audits can distinguish them from regressions.
**Depends on**: Phase 67, 68, 69, 70, 71 (all new behaviours must be in before parity tests can assert them).
**Requirements**: PARITY-01, PARITY-02
**Success Criteria** (what must be TRUE):
  1. A regression test suite ported from `pdb_api_test.py` asserts: default ordering (Phase 67), status × since matrix (Phase 68), `limit=0` unlimited semantics (Phase 68), `__in` behaviour for both large and empty lists (Phase 69), and the six 1-hop + 2-hop traversal cases cited in the validation audit (Phase 70) — all passing in CI
  2. Every intentional divergence from upstream discovered during Phase 67-71 execution is enumerated in `docs/API.md` with a rationale, so future conformance audits can distinguish intentional-divergence from regression
  3. A follow-up conformance run against `beta.peeringdb.com` (or recorded fixtures) shows zero unexpected diffs — any new diffs surfacing are either codified in `docs/API.md` or flagged as bugs
**Plans**: 6/6 plans executed

Plans:
- [x] 72-01-PLAN.md — cmd/pdb-fixture-port/ tool scaffold + PoC ordering-category fixture emission into internal/testutil/parity/fixtures.go (SHA-pinned header)
- [x] 72-02-PLAN.md — Extend pdb-fixture-port to STATUS + LIMIT category fixtures (STATUS-01..05 matrix + LIMIT-01 unbounded + LIMIT-02 depth-on-list guardrail)
- [x] 72-03-PLAN.md — Extend pdb-fixture-port to UNICODE + IN + TRAVERSAL fixtures; `--category all` emits the full 6-category file deterministically (5560 fixtures total: 12 ord + 46 status + 270 limit byte-identical to 72-02 + 216 unicode + 5002 in + 14 traversal; same pinned SHA peeringdb/peeringdb@99e92c72; --upstream-commit override flag added for SHA-preservation during pinned-snapshot regeneration)
- [x] 72-04-PLAN.md — internal/pdbcompat/parity/ — 6 category-split regression test files + shared harness locking ORDER/STATUS/LIMIT/UNICODE/IN/TRAVERSAL semantics (31 hard-pass tests; 4 harness probes + 27 v1.16-semantic subtests; 2 explicit DIVERGENCE asserts for DEFER-70-verifier-01 + DEFER-70-06-01; 15 citation hits + 36 t.Parallel + 4 DIVERGENCE markers per CONTEXT.md grep invariants; harness file is *_test.go to satisfy internal/sync TestTestutilIsTestOnly invariant) (PARITY-01)
- [x] 72-05-PLAN.md — internal/pdbcompat/parity/bench_test.go — 3 b.Loop()-style benchmarks (2-hop traversal ~580μs/op, limit=0 streaming ~82.7ms/op, 5001-element __in ~98.6ms/op on Ryzen 5 3600) per D-07; testing.TB widening across testutil.SetupClient + 9 parity harness helpers; single atomic commit e325752; full-repo `go test -race ./...` green post-widening (PARITY-01)
- [x] 72-06-PLAN.md — docs/API.md § Known Divergences extension + NEW § Validation Notes (5 invalid-pdbfe-claims with peeringdb/peeringdb@99e92c72 upstream SHA refs) + CLAUDE.md § Upstream parity regression (Phase 72) convention + CHANGELOG.md v1.16 [Unreleased] Phase 72 Added bullets + milestone-close note + REQUIREMENTS.md PARITY-02 flipped complete + ROADMAP.md Phase 72 [x] (PARITY-02) — v1.16 milestone COMPLETE

## Phase Dependency Graph

```
67 (ordering)
 └── 68 (status+limit)  ── shares pdbcompat list-path
      └── 69 (unicode/operator/__in)  ── shares filter.go
           └── 70 (traversal)  ── shares filter.go
                └── 71 (memory envelope)  ── needs 67-70 worst-case shapes
                     └── 72 (parity regression)  ── locks 67-71
```

Notes on parallelism:

- **Phase 67** is the broadest touch (all three surfaces) but the thinnest behavioural change. Landing it first avoids merge conflicts with 68/69's pdbcompat edits and unblocks the parity tests' ordering assumptions.
- **Phases 68 and 69** both edit `internal/pdbcompat/filter.go`; serialising them (68 → 69) avoids a guaranteed merge conflict. They are NOT parallelisable without pain.
- **Phase 70** builds directly on 69's filter layer (Path A + Path B traversal is resolved inside the same parser), so 69 → 70 is strict.
- **Phase 71** is deliberately last-before-parity because it needs to know the real worst-case response shapes (depth=2 + traversal + `limit=0`) that 67-70 enable, so the memory ceiling and streaming harness are sized correctly.
- **Phase 72** closes the milestone and must land after every behaviour it asserts is in.

## Progress

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 67. Default ordering flip | 6/6 | Complete | 2026-04-19 |
| 68. Status × since matrix + limit=0 | 4/4 | Complete | 2026-04-19 |
| 69. Unicode + operator + __in | 6/6 | Complete | 2026-04-19 |
| 70. Cross-entity __ traversal | 8/8 | Complete | 2026-04-19 |
| 71. Memory-safe response paths | 6/6 | Complete | 2026-04-19 |
| 72. Upstream parity regression | 6/6 | Complete | 2026-04-19 |

## Backlog

_No parked 999.x phases._
