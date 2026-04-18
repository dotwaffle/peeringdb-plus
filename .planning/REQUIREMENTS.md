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

- [ ] **ORDER-01**: `pdbcompat` list endpoints (`/api/<type>`) return rows ordered by `(-updated, -created)` matching `django-handleref` base `Meta.ordering`
- [ ] **ORDER-02**: `grpcserver` List and Stream RPCs return rows ordered by `(-updated, -created)`; cursor pagination remains stable under this order
- [ ] **ORDER-03**: `entrest` (`/rest/v1/*`) default list ordering matches upstream; explicit `?sort=` overrides still honoured

### Status × Since Matrix (STATUS)

- [ ] **STATUS-01**: `pdbcompat` list requests without `?since` return only rows where `status=ok` (matches `rest.py:694-727`)
- [ ] **STATUS-02**: `pdbcompat` single-object requests (by pk) include rows where `status IN (ok, pending)`
- [ ] **STATUS-03**: `pdbcompat` list requests with `?since>0` include rows where `status IN (ok, deleted)`, with `pending` additionally included for the `campus` type
- [ ] **STATUS-04**: Explicit `?status=deleted` without `?since` returns an empty set (upstream applies the final `status=ok` filter unconditionally on list requests)
- [ ] **STATUS-05**: The `PDBPLUS_INCLUDE_DELETED` config gate is re-scoped: it controls *what sync persists*, not *what pdbcompat serves* — status-matrix filtering becomes pdbcompat's responsibility

### Limit Semantics (LIMIT)

- [ ] **LIMIT-01**: `pdbcompat` `?limit=0` returns all matching rows with no upper bound (matches `rest.py:494-497`); count-only semantics are NOT implemented (pdbfe claim confirmed wrong)
- [ ] **LIMIT-02**: Depth>0 list responses continue to respect the implicit `API_DEPTH_ROW_LIMIT=250` cap per `rest.py:744-748`

### __in Robustness (IN)

- [ ] **IN-01**: `pdbcompat` `?<field>__in=` accepts arbitrarily-large comma-separated lists without hitting SQLite's 999-variable limit (implementation path: rewrite to `WHERE <field> IN (SELECT value FROM json_each(?))` using a single bind parameter)
- [ ] **IN-02**: Empty `?<field>__in=` returns an empty result set (matches Django ORM semantics)

### Unicode Folding + Operator Coercion (UNICODE)

- [ ] **UNICODE-01**: Filter values passed to `__contains` / `__startswith` / `__iexact` are Unicode-folded before reaching SQL, matching `rest.py:576` (`unidecode.unidecode(v)`); `?name__contains=Zürich` matches rows containing `zurich`
- [ ] **UNICODE-02**: `__contains` operator coerced to `__icontains` behaviour at the query layer; `__startswith` coerced to `__istartswith` (matches `rest.py:638-641`)
- [ ] **UNICODE-03**: `ParseFilters` fuzz corpus extended to cover non-ASCII inputs (diacritics, CJK, combining marks, zero-width joiners); zero panics under fuzz

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
| ORDER-01 | 67 | pending |
| ORDER-02 | 67 | pending |
| ORDER-03 | 67 | pending |
| STATUS-01 | 68 | pending |
| STATUS-02 | 68 | pending |
| STATUS-03 | 68 | pending |
| STATUS-04 | 68 | pending |
| STATUS-05 | 68 | pending |
| LIMIT-01 | 68 | pending |
| LIMIT-02 | 68 | pending |
| IN-01 | 69 | pending |
| IN-02 | 69 | pending |
| UNICODE-01 | 69 | pending |
| UNICODE-02 | 69 | pending |
| UNICODE-03 | 69 | pending |
| TRAVERSAL-01 | 70 | pending |
| TRAVERSAL-02 | 70 | pending |
| TRAVERSAL-03 | 70 | pending |
| TRAVERSAL-04 | 70 | pending |
| MEMORY-01 | 71 | pending |
| MEMORY-02 | 71 | pending |
| MEMORY-03 | 71 | pending |
| MEMORY-04 | 71 | pending |
| PARITY-01 | 72 | pending |
| PARITY-02 | 72 | pending |
