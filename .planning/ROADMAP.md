# Roadmap: PeeringDB Plus

## Milestones

- ✅ **v1.0 – v1.14** — shipped (see [MILESTONES.md](./MILESTONES.md))
- ✅ **v1.15 — Infrastructure Polish & Schema Hygiene** — shipped 2026-04-18 (Phases 63-66, 11 requirements)
- 🟡 **v1.16 — Django-compat Correctness** — defined 2026-04-18 (Phases 67-72, 25 requirements)

## Phases

**v1.16 — Django-compat Correctness (current)**

- [x] **Phase 67: Default ordering flip** — Pdbcompat, grpcserver, entrest list paths return rows in upstream `(-updated, -created)` order
- [ ] **Phase 68: Status × since matrix + limit=0 semantics** — Pdbcompat applies upstream status-filter rules and treats `limit=0` as unlimited
- [ ] **Phase 69: Filter-value Unicode folding, operator coercion, `__in` robustness** — Pdbcompat filter layer matches upstream `rest.py:544-662` value handling
- [ ] **Phase 70: Cross-entity `__` traversal (Path A + Path B + 2-hop)** — Pdbcompat resolves filter paths across FKs like upstream serializers
- [ ] **Phase 71: Memory-safe response paths on 256 MB replicas** — Depth=2 / `limit=0` / large traversals stay within the replica memory envelope
- [ ] **Phase 72: Upstream parity regression + divergence docs** — Ported `pdb_api_test.py` ground-truth tests lock the new semantics

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
**Plans**: TBD

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
**Plans**: TBD

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
**Plans**: TBD

### Phase 71: Memory-safe response paths on 256 MB replicas
**Goal**: Depth=2, `limit=0`, and wide traversal queries stream JSON encoding with bounded intermediate allocations, enforce a per-response memory ceiling that trips before Fly OOM-kills the machine, and expose per-request heap/RSS telemetry.
**Depends on**: Phase 67, 68, 69, 70 (must know the worst-case response shapes those phases enable before the ceiling and streaming harness can be sized correctly).
**Requirements**: MEMORY-01, MEMORY-02, MEMORY-03, MEMORY-04
**Success Criteria** (what must be TRUE):
  1. A `depth=2` or `limit=0` pdbcompat response streams bytes through the `http.ResponseWriter` without materialising the full result set to a single `[]byte` or `[]T` slice before the first flush
  2. A response exceeding `PDBPLUS_RESPONSE_MEMORY_LIMIT` (default sized against the 256 MB replica budget minus operating headroom) is truncated with an RFC 9457 `application/problem+json` 413 response before the process gets OOM-killed
  3. Per-request OTel span attributes and a Prometheus gauge expose response-path peak heap and RSS, reusing the v1.15 Phase 66 `runtime.MemStats` harness; Grafana's SEED-001 watch row shows response-path peaks alongside sync-path peaks
  4. `docs/ARCHITECTURE.md` documents the per-endpoint memory envelope (worst-case depth/limit combinations per type) and the operator-actionable knobs that cap it
**Plans**: TBD

### Phase 72: Upstream parity regression + divergence docs
**Goal**: Lock the v1.16 semantics in place by porting the ground-truth assertions from upstream `pdb_api_test.py` and documenting any intentional divergences so future conformance audits can distinguish them from regressions.
**Depends on**: Phase 67, 68, 69, 70, 71 (all new behaviours must be in before parity tests can assert them).
**Requirements**: PARITY-01, PARITY-02
**Success Criteria** (what must be TRUE):
  1. A regression test suite ported from `pdb_api_test.py` asserts: default ordering (Phase 67), status × since matrix (Phase 68), `limit=0` unlimited semantics (Phase 68), `__in` behaviour for both large and empty lists (Phase 69), and the six 1-hop + 2-hop traversal cases cited in the validation audit (Phase 70) — all passing in CI
  2. Every intentional divergence from upstream discovered during Phase 67-71 execution is enumerated in `docs/API.md` with a rationale, so future conformance audits can distinguish intentional-divergence from regression
  3. A follow-up conformance run against `beta.peeringdb.com` (or recorded fixtures) shows zero unexpected diffs — any new diffs surfacing are either codified in `docs/API.md` or flagged as bugs
**Plans**: TBD

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
| 68. Status × since matrix + limit=0 | 0/? | Not started | - |
| 69. Unicode + operator + __in | 0/? | Not started | - |
| 70. Cross-entity __ traversal | 0/? | Not started | - |
| 71. Memory-safe response paths | 0/? | Not started | - |
| 72. Upstream parity regression | 0/? | Not started | - |

## Backlog

_No parked 999.x phases._
