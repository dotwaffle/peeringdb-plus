# Requirements: PeeringDB Plus — v1.16

**Defined:** 2026-04-18
**Core Value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.
**Theme:** Django-compat correctness — align `pdbcompat` with upstream PeeringDB Django semantics based on validated analysis of `peeringdb/peeringdb@99e92c72` and `django-peeringdb`, while respecting the 256 MB replica memory ceiling established in v1.15 Phase 65.

## Source-of-truth references

All requirements below cite upstream code by path and line. The canonical references are:

- `src/peeringdb_server/rest.py` — ModelViewSet.get_queryset (filter/paginate/depth/since)
- `src/peeringdb_server/serializers.py` — per-entity `prepare_query`, `queryable_relations`, `FILTER_EXCLUDE`
- `src/peeringdb_server/models.py` — `related_to_*` helpers
- `src/peeringdb_server/management/commands/pdb_api_test.py` — behavioural ground truth
- `django-peeringdb/src/django_peeringdb/models/abstract.py` — field types/defaults
- `django-handleref/src/django_handleref/models.py` — base `Meta.ordering` and `status`

## v1.16 Requirements

### Default Ordering (ORDER)

- [x] **ORDER-01**: `pdbcompat` list endpoints (`/api/<type>`) return rows ordered by `(-updated, -created)` matching `django-handleref` base `Meta.ordering`
- [x] **ORDER-02**: `grpcserver` List and Stream RPCs return rows ordered by `(-updated, -created)`; cursor pagination remains stable under this order
- [x] **ORDER-03**: `entrest` (`/rest/v1/*`) default list ordering matches upstream; explicit `?sort=` overrides still honoured

### Status × Since Matrix (STATUS)

- [x] **STATUS-01**: `pdbcompat` list requests without `?since` return only rows where `status=ok` (matches `rest.py:694-727`) **(delivered by Phase 68 Plan 03 — applyStatusMatrix helper + 13 list-closure edits + Fields-map `status` removals per D-07.)**
- [x] **STATUS-02**: `pdbcompat` single-object requests (by pk) include rows where `status IN (ok, pending)` **(delivered by Phase 68 Plan 03 — 26 `StatusIn("ok","pending")` inserts across 13 `getXWithDepth` functions in depth.go per D-06.)**
- [x] **STATUS-03**: `pdbcompat` list requests with `?since>0` include rows where `status IN (ok, deleted)`, with `pending` additionally included for the `campus` type **(data prereq delivered by Phase 68 Plan 02 soft-delete flip; pdbcompat surface delivered by Phase 68 Plan 03 — applyStatusMatrix(isCampus, sinceSet=true) emits the allowed status slice with pending appended only for campus per D-05.)**
- [x] **STATUS-04**: Explicit `?status=deleted` without `?since` returns an empty set (upstream applies the final `status=ok` filter unconditionally on list requests) **(delivered by Phase 68 Plan 03 — Fields-map removal causes ParseFilters to silently drop `?status=<anything>`; applyStatusMatrix still emits `status=ok` unconditionally when `sinceSet=false` per D-07.)**
- [x] **STATUS-05**: The `PDBPLUS_INCLUDE_DELETED` config gate is re-scoped: it controls *what sync persists*, not *what pdbcompat serves* — status-matrix filtering becomes pdbcompat's responsibility **(delivered by Phase 68 Plan 01 via D-01 removal — the gate is gone entirely; sync unconditionally persists deleted rows starting with Plan 68-02's soft-delete flip.)**

### Limit Semantics (LIMIT)

- [x] **LIMIT-01**: `pdbcompat` `?limit=0` returns all matching rows with no upper bound (matches `rest.py:494-497`); count-only semantics are NOT implemented (pdbfe claim confirmed wrong) **(delivered by Phase 68 Plan 03 — ParsePaginationParams treats `limit=0` as unlimited sentinel with MaxLimit clamp gated on `limit > 0`; 13 list closures apply `.Limit(opts.Limit)` only when `opts.Limit > 0`. Empirical probe test locks ent v0.14.6 `.Limit(0)` = unlimited behaviour via sqlgraph `graph.go:1086` `if q.Limit != 0` gate.)**
- [x] **LIMIT-02**: Depth>0 list responses continue to respect the implicit `API_DEPTH_ROW_LIMIT=250` cap per `rest.py:744-748` **(delivered by Phase 68 Plan 03 as a serveList depth-on-list guardrail — `?depth=` on list endpoints is silently ignored with a `slog.DebugContext` paper trail; opts.Depth never leaks into list closures. Phase 71 will own the memory-safe functional list+depth implementation.)**

### __in Robustness (IN)

- [x] **IN-01**: `pdbcompat` `?<field>__in=` accepts arbitrarily-large comma-separated lists without hitting SQLite's 999-variable limit (implementation path: rewrite to `WHERE <field> IN (SELECT value FROM json_each(?))` using a single bind parameter)
- [x] **IN-02**: Empty `?<field>__in=` returns an empty result set (matches Django ORM semantics)

### Unicode Folding + Operator Coercion (UNICODE)

- [x] **UNICODE-01**: Filter values passed to `__contains` / `__startswith` / `__iexact` are Unicode-folded before reaching SQL, matching `rest.py:576` (`unidecode.unidecode(v)`); `?name__contains=Zürich` matches rows containing `zurich`
- [x] **UNICODE-02**: `__contains` operator coerced to `__icontains` behaviour at the query layer; `__startswith` coerced to `__istartswith` (matches `rest.py:638-641`)
- [x] **UNICODE-03**: `ParseFilters` fuzz corpus extended to cover non-ASCII inputs (diacritics, CJK, combining marks, zero-width joiners); zero panics under fuzz

### Cross-Entity `__` Traversal (TRAVERSAL)

- [ ] **TRAVERSAL-01**: `pdbcompat` implements per-serializer `prepare_query` allowlists for all 13 entity types, mirroring upstream `serializers.py` (11 distinct allowlist shapes verified in the validation audit)
- [ ] **TRAVERSAL-02**: `pdbcompat` implements automatic `<fk>__<field>` traversal via ent relationship introspection minus a documented `FILTER_EXCLUDE` list — matches upstream `queryable_relations()` (Path B)
- [ ] **TRAVERSAL-03**: 2-hop traversal is supported (e.g. `/api/fac?ixlan__ix__fac_count__gt=0` resolves through netixlan → ix → fac_count as in upstream `pdb_api_test.py:2340,2348`)
- [ ] **TRAVERSAL-04**: Unknown filter fields are silently ignored rather than rejected with HTTP 400 (matches upstream `rest.py:544-662` behaviour; breaking strictness would regress existing integrations)

### Memory-Safe Response Paths (MEMORY)

- [ ] **MEMORY-01**: Depth=2 and `limit=0` responses stream JSON encoding through the response writer with bounded intermediate allocations; no full-result materialisation to a single slice before flush
- [ ] **MEMORY-02**: A configurable per-response memory ceiling (`PDBPLUS_RESPONSE_MEMORY_LIMIT`, default matches the 256 MB replica budget minus operating headroom) triggers graceful truncation with an RFC 9457 problem-detail 413 response before Fly OOM-kills the machine
- [ ] **MEMORY-03**: Response-path peak heap / RSS telemetry is recorded per-request (OTel span attributes + optional Prometheus gauge) reusing the v1.15 Phase 66 `runtime.MemStats` harness, so replicas that approach the ceiling are observable in Grafana
- [ ] **MEMORY-04**: The per-endpoint memory envelope (worst-case depth/limit combinations) is documented in `docs/ARCHITECTURE.md` with operator-actionable knobs

### Upstream Parity Regression (PARITY)

- [ ] **PARITY-01**: Behavioural regression tests port the ground-truth assertions from upstream `pdb_api_test.py` covering: default ordering (added via ORDER), status × since matrix (STATUS), `limit=0` semantics (LIMIT), `__in` behaviour (IN), and the six 1-hop + 2-hop traversal cases cited in the validation audit (TRAVERSAL)
- [ ] **PARITY-02**: A docs update records the NEW divergences we deliberately accept from upstream (if any emerge during implementation) in `docs/API.md`, so future conformance audits can distinguish intentional-divergence from regression

## Future Requirements

Deferred from v1.16 scope:

- [ ] OAuth identity integration for Users-tier admittance — VIS-08 substrate shipped v1.15 Phase 64
- [ ] SEED-001 incremental sync implementation — dormant; sync-path trigger hasn't fired
- [ ] SEED-003 primary HA hot-standby — dormant; LHR-outage trigger hasn't fired
- [ ] RPKI / BGP / IRR feature integrations — larger milestone, needs separate design pass

## Out of Scope

- **GraphQL or ConnectRPC semantic changes beyond the ordering flip.** v1.16 is pdbcompat-first. Filter-traversal and status-matrix work target `pdbcompat` only; grpcserver/entrest keep their existing shapes.
- **Fixing upstream bugs.** Where pdbfe's claims turn out to be wrong *because upstream is wrong* (e.g. `net?country=NL` isn't a valid upstream filter), we don't pave over that — we match upstream, including its rough edges.
- **Web UI / terminal renderer changes.** The compat fixes live below the surface-rendering layer.
- **Strictness hooks.** Unknown filter fields must continue to be silently ignored (upstream behaviour); rejecting them with HTTP 400 is an explicit non-goal.
- **Write paths.** Read-only mirror invariant remains.

## Traceability

Each REQ-ID maps to exactly one phase. 25/25 requirements mapped — 100% coverage.

| REQ-ID | Phase | Status |
|--------|-------|--------|
| ORDER-01 | 67 | Complete |
| ORDER-02 | 67 | Complete |
| ORDER-03 | 67 | Complete |
| STATUS-01 | 68 | complete (68-03) |
| STATUS-02 | 68 | complete (68-03) |
| STATUS-03 | 68 | complete (68-02 data prereq + 68-03 pdbcompat surface) |
| STATUS-04 | 68 | complete (68-03) |
| STATUS-05 | 68 | complete (68-01) |
| LIMIT-01 | 68 | complete (68-03) |
| LIMIT-02 | 68 | complete (68-03 guardrail; functional list+depth deferred to Phase 71) |
| IN-01 | 69 | complete (69-04; json_each(?) single-bind in internal/pdbcompat/filter.go:264, EXPLAIN QUERY PLAN test in phase69_filter_test.go) |
| IN-02 | 69 | complete (69-04; errEmptyIn sentinel + QueryOptions.EmptyResult flag + 13 closure guards) |
| UNICODE-01 | 69 | complete (69-04; 16 fields across 6 TypeConfigs route via <field>_fold with unifold.Fold on RHS) |
| UNICODE-02 | 69 | complete (69-04; coerceToCaseInsensitive in filter.go maps __contains → __icontains, __startswith → __istartswith) |
| UNICODE-03 | 69 | complete (69-05; FuzzFilterParser corpus extended with 21 D-07 cases — diacritics/CJK/RTL/RLO/ZWJ/combining/null/>64KB + IN edges; local 60s fuzz on Ryzen 5 3600 = 469197 execs / 65 new interesting / zero panics) |
| TRAVERSAL-01 | 70 | complete (70-03; 13 `pdbcompat.WithPrepareQueryAllow` annotations in ent/schema/*.go + populated entries in `internal/pdbcompat/allowlist_gen.go` Allowlists map; TestParseFilters_AllThirteenEntitiesCoverPathA in filter_test.go) |
| TRAVERSAL-02 | 70 | complete (70-04; `Edges` map emitted by cmd/pdb-compat-allowlist + `LookupEdge`/`ResolveEdges`/`TargetFields` API in internal/pdbcompat/introspect.go; TestLookupEdge_AllThirteenEntitiesCovered + TestLookupEdge_KnownHops in introspect_test.go) |
| TRAVERSAL-03 | 70 | complete (70-05/06/07; BenchmarkTraversal_2Hop_UpstreamParity + BenchmarkTraversal_2Hop_WithLimitAndSkip + TestBenchTraversal_D07_Ceiling in bench_traversal_test.go lock <50ms/op @ 10k rows; E2E case ixlan__ix__fac_count__gt=0 + ixlan__ix__id in traversal_e2e_test.go) |
| TRAVERSAL-04 | 70 | complete (70-05/06; ParseFiltersCtx silent-ignore + WithUnknownFields ctx accumulator in filter.go; slog.DebugContext + OTel attr `pdbplus.filter.unknown_fields` in handler.go; TestParseFilters_UnknownFieldsAppendToCtx in filter_test.go; E2E cases unknown_local_field / unknown_fk_segment / unknown_target_field / too_deep_three_hop / too_deep_four_hop in traversal_e2e_test.go) |
| MEMORY-01 | 71 | pending |
| MEMORY-02 | 71 | pending |
| MEMORY-03 | 71 | pending |
| MEMORY-04 | 71 | pending |
| PARITY-01 | 72 | pending |
| PARITY-02 | 72 | pending |
