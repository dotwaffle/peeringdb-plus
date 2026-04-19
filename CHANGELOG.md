# Changelog

All notable changes to this project are documented in this file.

The format follows [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Historical release notes prior to v1.16 live in
[`.planning/MILESTONES.md`](./.planning/MILESTONES.md).

## [Unreleased] — v1.16

v1.16 is a coordinated milestone release. Phases 67, 68, 69, 70, and 71 ship
together in a single deploy window; Phase 72 (upstream parity regression)
ships independently as a code-only test lock-in. Do not deploy any individual
Phase 68 commit in isolation — pdbcompat `?limit=0` now returns all matching
rows, and the memory-safe response paths that bound that behaviour land in
Phase 71.

> **Coordinated release window:** v1.16 phases 67-71 are now complete
> and ready to deploy as a bundle. `limit=0` unbounded semantics
> (Phase 68) are safe in prod only with the Phase 71 memory budget in
> place — do NOT ship 67-70 without 71. Phase 72 (parity regression
> test lock-in) ships independently as a follow-up — no production
> deploy required; it is a CI regression gate only.
>
> **v1.16 milestone complete (2026-04-19):** All 6 phases (67, 68, 69,
> 70, 71, 72) shipped. 25/25 requirements across 8 categories (ORDER,
> STATUS, LIMIT, IN, UNICODE, TRAVERSAL, MEMORY, PARITY) traced and
> complete. Next action: `/gsd-complete-milestone` to archive v1.16
> into `.planning/milestones/v1.16-*`.

### Breaking

- **Removed `PDBPLUS_INCLUDE_DELETED` environment variable.** Sync now
  always persists deleted rows as tombstones (soft-delete via
  `UPDATE ... SET status='deleted'`). During the v1.16 → v1.17 grace
  period, setting this variable triggers a startup WARN and is ignored;
  v1.17 upgrades this to a fatal startup error. Remove it from your
  environment. See
  [`docs/CONFIGURATION.md` § Removed in v1.16](./docs/CONFIGURATION.md#removed-in-v116).

  **One-time gap:** Rows hard-deleted by sync cycles BEFORE the v1.16
  upgrade are gone forever. `?status=deleted` and `?since=N` queries
  populate going forward from the first post-upgrade sync cycle. See
  [`docs/API.md` § Known Divergences](./docs/API.md#known-divergences).

### Added

- **pdbcompat status × since matrix** matching upstream
  `peeringdb_server/rest.py:694-727`. List requests without `?since`
  return only `status=ok`. List requests with `?since=N` admit
  `(ok, deleted)`, plus `pending` for campus. Single-object GETs
  (`/api/<type>/<id>`) admit `(ok, pending)` for all 13 entity types.
  Explicit `?status=deleted` on a list request without `?since`
  silently returns an empty set, matching the upstream unconditional
  `filter(status='ok')` on `rest.py:725`.

- **pdbcompat `?limit=0` semantics** match upstream `rest.py:734-737`:
  an explicit `limit=0` returns all matching rows. The default-when-unset
  remains `250`. `?depth=` on list endpoints is silently ignored in
  Phase 68; Phase 71 will add list+depth support with the
  `API_DEPTH_ROW_LIMIT=250` cap.

- **pdbcompat cross-surface default ordering** flipped to
  `(-updated, -created, -id)` (Phase 67, shipped earlier in v1.16).
  Applies to pdbcompat `/api/`, entrest `/rest/v1/`, ConnectRPC list
  RPCs, and GraphQL list queries. Single-object lookups and nested
  `_set` fields are unchanged.

- **pdbcompat Unicode folding** for diacritic-insensitive matching on
  searchable text fields. `?name__contains=Zurich` now matches a DB
  row where `name="Zürich"`. Implementation uses shadow columns
  (`<field>_fold`) populated at sync time by a new `internal/unifold`
  package (NFKD decomposition + a small hand-rolled ligature map for
  `ß`/`æ`/`ø`/`ł`/`þ`/`đ`). 16 shadow columns across 6 entity types
  (network, facility, internetexchange, organization, campus,
  carrier). Matches upstream `peeringdb_server/rest.py:576`
  (`unidecode.unidecode(v)`). Closes UNICODE-01.

- **pdbcompat operator coercion**: `__contains` is now equivalent to
  `__icontains` (case-insensitive) and `__startswith` is equivalent
  to `__istartswith`, per upstream `rest.py:638-641`. All other
  operators (`__exact`, `__iexact`, `__gt`, `__lt`, `__gte`, `__lte`,
  `__in`) are unchanged. Closes UNICODE-02.

- **pdbcompat `__in` large-list support**: `?<field>__in=` now accepts
  arbitrarily-large comma-separated lists via a SQLite `json_each`
  single-bind rewrite, bypassing the 999-variable parameter limit.
  Empty `__in` (e.g. `?asn__in=`) returns `{"data":[],"meta":{"count":0}}`
  with no SQL executed, matching Django ORM `Model.objects.filter(id__in=[])`
  semantics. Closes IN-01 and IN-02.

- **pdbcompat fuzz corpus** extended with 21 non-ASCII and `__in`
  edge-case seeds (diacritics, CJK, RTL, RLO/LRO overrides, ZWJ,
  combining marks, null bytes, 70 KB literals, 1201-element `__in`,
  empty `__in`, all-empty `__in` parts). Local 60s run on a Ryzen 5
  3600 logged 469k executions / 65 new interesting / zero panics.
  Closes UNICODE-03.

- **Cross-entity `__` traversal in pdbcompat (Phase 70).** The
  `/api/<type>?<fk>__<field>=` and
  `/api/<type>?<fk>__<fk>__<field>=` query shapes now resolve across
  foreign-key edges, mirroring upstream PeeringDB's `prepare_query`
  allowlists (Path A) and `queryable_relations()` auto-introspection
  (Path B). Hard-capped at 2 hops (cite: Phase 70 D-04). Every 13-entity
  allowlist was translated 1:1 from `peeringdb_server/serializers.py`
  (SHA `99e92c72`); each annotation carries a `serializers.py:<line>`
  source comment for audit. A new codegen tool `cmd/pdb-compat-allowlist`
  reads ent schema annotations and emits
  `internal/pdbcompat/allowlist_gen.go` (Path A allowlists + Path B
  `Edges` map) wired into `go generate ./...` after ent codegen and
  before buf codegen. Example 2-hop case working:
  `GET /api/fac?ixlan__ix__fac_count__gt=0`. Closes TRAVERSAL-01,
  TRAVERSAL-02, and TRAVERSAL-03.

- **Unknown filter fields silently ignored (TRAVERSAL-04).**
  `GET /api/net?totally_unknown_field=x` returns HTTP 200 with a
  silently-unfiltered result rather than 400, matching upstream
  `rest.py:544-662`. Operators gain DEBUG-level visibility via
  `slog.DebugContext("pdbcompat: unknown filter fields silently ignored
  (Phase 70 TRAVERSAL-04)", ...)` and OTel span attribute
  `pdbplus.filter.unknown_fields` (CSV of all unknowns per request).
  The same diagnostic fires for typos, deprecated field names, and
  filter keys with >2 `__`-separated relation segments (the 2-hop cap
  per Phase 70 D-04). Closes TRAVERSAL-04.

- **2-hop cost ceiling (<50ms/op @ 10k rows).** New
  `BenchmarkTraversal_*` in `internal/pdbcompat/bench_traversal_test.go`
  plus a go-test-time `TestBenchTraversal_D07_Ceiling` gate guard the
  2-hop query cost ceiling. A nightly CI workflow
  (`.github/workflows/bench.yml`) regression-gates via benchstat —
  prevents a future Cartesian-join regression from landing silently
  (Phase 70 D-07).

- **Phase 71 (MEMORY-01..04): Memory-safe response paths on 256 MB replicas.**
  - **Streaming JSON emission** for pdbcompat list responses —
    `internal/pdbcompat/stream.go` `StreamListResponse` writes
    `{"meta":…,"data":[…]}` token-by-token via per-row `json.Marshal`
    and `http.Flusher.Flush()` every 100 rows. Replaces the legacy
    full-slice `json.NewEncoder` materialisation on the `serveList`
    path. Closes MEMORY-01.
  - **`PDBPLUS_RESPONSE_MEMORY_LIMIT` env var** (default 128 MiB =
    256 MB replica − 80 MB Go runtime baseline − 48 MB slack). Gates
    response size via a pre-flight `SELECT COUNT(*) × typical_row_bytes`
    heuristic in `internal/pdbcompat/budget.go` `CheckBudget`.
    Over-budget requests receive RFC 9457
    `application/problem+json` 413 with `max_rows`, `budget_bytes`,
    and a human-readable `detail` string BEFORE any row data is
    fetched. Unit suffix required (`KB`/`MB`/`GB`/`TB`); the literal
    `0` disables the check for local development. No `Retry-After` —
    413 is request-shape, not transient. Per-entity
    `typical_row_bytes` calibrated via `BenchmarkRowSize_*` and
    doubled per D-03 (13 types × 2 depths in
    `internal/pdbcompat/rowsize.go`). Closes MEMORY-02.
  - **Per-request heap-delta telemetry.** New OTel span attribute
    `pdbplus.response.heap_delta_kib` (sampled once at handler entry
    and once at exit via `defer`; STW ~µs, NEVER per row) plus
    Prometheus histogram
    `pdbplus_response_heap_delta_kib{endpoint,entity}`. Registered
    via `pdbotel.InitResponseHeapHistogram()`. Grafana gains a
    "Response Heap Delta (KiB) — p50/p95/p99 by endpoint" panel (id
    36) at the bottom of the SEED-001 watch row in
    `deploy/grafana/dashboards/pdbplus-overview.json`. Closes
    MEMORY-03.
  - **`docs/ARCHITECTURE.md` § Response Memory Envelope** documents
    the envelope derivation, the three moving parts (stream / rowsize
    / budget), a per-entity worst-case sizing table with computed
    `max_rows` at the 128 MiB default, the request lifecycle, and the
    telemetry wire-up. `CLAUDE.md` gains a sibling § Response memory
    envelope (Phase 71) convention with the maintainer checklist for
    adding new entity types. Closes MEMORY-04.

- **Phase 72 (PARITY-01, PARITY-02): Upstream parity regression lock-in.**
  - **`internal/pdbcompat/parity/` category-split regression suite** —
    6 test files (`ordering_test.go`, `status_test.go`,
    `limit_test.go`, `unicode_test.go`, `in_test.go`,
    `traversal_test.go`) + a shared `harness_helpers_test.go`. 31
    hard-pass tests total: 27 v1.16-semantic sub-tests covering
    ORDER-01..03 / STATUS-01..06 / LIMIT-01/01b/02 / UNICODE-01/02 /
    IN-01/02/03 / TRAVERSAL-01..04 plus 4 harness probes. 2 explicit
    `DIVERGENCE_` sub-tests lock the v1.16 silent-ignore semantic for
    DEFER-70-verifier-01 (`fac?ixlan__ix__fac_count__gt=0`) and the
    HTTP 500 outcome for DEFER-70-06-01 (`fac?campus__name=X`). 15
    `pdb_api_test.py`-or-synthesised citation hits, 36 `t.Parallel()`
    call sites, 4 `DIVERGENCE` markers per CONTEXT.md grep invariants.
    Suite wall time 15.4s under `-race`. Closes PARITY-01.
  - **`cmd/pdb-fixture-port/` fixture-porting tool** reads upstream
    `src/peeringdb_server/management/commands/pdb_api_test.py` and
    emits Go fixture literals into
    `internal/testutil/parity/fixtures.go`. 5560 ported rows across 6
    category vars pinned to `peeringdb/peeringdb@99e92c72` (full SHA
    `99e92c726172ead7d224ce34c344eff0bccb3e63`) with
    `sha256:75c7a6fab734db7…` source-file hash recorded in the
    generated header. `--upstream-commit <sha>` override preserves the
    pinned SHA during snapshot-replay regeneration;
    `--check` flag compares current upstream against the pinned SHA
    and reports drift (advisory only per CONTEXT.md D-03, not
    blocking).
  - **`internal/pdbcompat/parity/bench_test.go` performance lock-in** —
    3 `b.Loop()`-style benchmarks per GO-TOOL-1:
    `BenchmarkParity_TwoHopTraversal` (`ixpfx?ixlan__ix__id=20`,
    ~580μs/op on Ryzen 5 3600),
    `BenchmarkParity_LimitZeroStreaming` (5000-row seeded end-to-end
    Phase 71 `stream.go` path, ~82.7ms/op),
    `BenchmarkParity_InFiveThousandElements` (5001-id IN via Phase 69
    `json_each` rewrite, ~98.6ms/op). Benchstat-on-main is out of
    scope per CONTEXT.md D-06 — benchmarks are local-run only, gated
    by the standard `go test -race ./...` tier. `testing.TB` widening
    plumbed through `testutil.SetupClient` + 9 parity harness helpers
    (type-only change, every `*testing.T` call site still satisfies
    the interface).
  - **`docs/API.md § Known Divergences` extended** with 3 new rows
    (pre-Phase-68 hard-delete gap parity cross-ref; pdbfe `limit=0`
    count-only invalid claim; depth-on-list silent-drop LIMIT-02
    guardrail) plus `TestParity_*` cross-refs appended to the Since
    columns of the existing DEFER-70-06-01 + DEFER-70-verifier-01
    rows. Closes PARITY-02.
  - **`docs/API.md § Validation Notes` NEW sub-section** documenting
    5 invalid third-party claims about upstream behaviour with pinned
    `peeringdb/peeringdb@99e92c72…` SHA refs: (1) `net?country=NL` is
    not a valid filter (country lives on `org`), (2) `?limit=0` is
    unlimited not count-only, (3) default ordering is
    `(-updated, -created)` not `id ASC`, (4) Unicode folding is Python
    `unidecode` not MySQL collation, (5) filter surface is
    `prepare_query` + `queryable_relations` not a DRF `filterset_class`.
    Each row cites the specific upstream file:line and cross-
    references the parity sub-test locking the corrected behaviour.
    Future conformance audits against pdbfe's gotchas doc don't re-
    research the same invalid claims. Closes PARITY-02.

### Changed

- **Sync now soft-deletes** instead of hard-deleting. The 13
  `deleteStale*` functions in `internal/sync/delete.go` were renamed
  to `markStaleDeleted*`; they run
  `UPDATE ... SET status='deleted', updated=<cycle_start>` per sync
  cycle. One `cycleStart` timestamp is stamped on every tombstone
  within a cycle so `?since=N` windows stay atomic. Tombstone
  garbage-collection policy is deferred to SEED-004 (planted
  2026-04-19).

- **`parseFieldOp` signature extended** in
  `internal/pdbcompat/filter.go`. Return tuple expanded from
  `(field, op string)` to `(relationSegments []string, finalField,
  op string)` so the parser can detect `<fk>__<field>` patterns before
  consulting Path A / Path B and enforce the 2-hop cap (Phase 70 D-06).
  Internal-only — no callers exist outside `internal/pdbcompat`.

- **`ParseFilters` gains a context-aware sibling.** New
  `ParseFiltersCtx(ctx, params, tc)` threads an unknown-field
  accumulator via `context.Value` so the handler emits one aggregated
  `slog.DebugContext` call and a single OTel span attribute per
  request, rather than one per unknown field. Legacy `ParseFilters`
  kept as a shim for existing call sites.

### Deprecated

- `PDBPLUS_INCLUDE_DELETED` (see Breaking above; removal completes
  with fatal startup error in v1.17).

### Fixed

- `?limit=0` on pdbcompat list endpoints previously fell back to
  `DefaultLimit=250`. Now returns all rows up to any other filter,
  matching upstream behaviour (`rest.py:734-737`).

### Known issues

- **One-time ASCII-only window for diacritic-insensitive matching.**
  Between v1.16 deploy and the first post-deploy sync cycle (≤1h with
  the default `PDBPLUS_SYNC_INTERVAL=1h`), rows synced before the
  upgrade have `<field>_fold = ''` and return no match for non-ASCII
  queries against `__contains` / `__startswith` on searchable text
  fields. ASCII queries continue to work via the existing non-folded
  columns throughout the window. No manual backfill is required — the
  next standard sync cycle rewrites every affected row via the
  `OnConflict().UpdateNewValues()` path. See
  [`docs/API.md` § Known Divergences](./docs/API.md#known-divergences).

- **Unknown filter fields silently ignored is a feature, not a bug
  (Phase 70 TRAVERSAL-04).** Typos (`?nmae=x`), deprecated field names,
  and filter keys with >2 `__`-separated relation segments do not
  return HTTP 400 — the filter is silently dropped and the response
  contains the full unfiltered result set. This matches upstream
  PeeringDB (`rest.py:544-662`) and preserves existing client
  integrations that probe field names. Clients that want strict
  validation should inspect the OTel span attribute
  `pdbplus.filter.unknown_fields` or enable DEBUG-level logging to
  surface the dropped keys.

- **`campus` edge table-name codegen bug (DEFER-70-06-01).**
  `cmd/pdb-compat-allowlist` emits `TargetTable: "campus"` instead
  of the correct `"campuses"` for all edges targeting the Campus
  entity, because `entc.LoadGraph` does not apply the
  `fixCampusInflection` patch used by the ent runtime codegen. Affected
  queries (e.g. `GET /api/fac?campus__name=X`) return
  `500 SQL logic error: no such table: campus (1)`. The outgoing
  edges FROM Campus are correct. Documented one-time gap; fix
  scheduled as a follow-up (preferred approach: add
  `entsql.Annotation{Table: "campuses"}` to `ent/schema/campus.go`).
  See `.planning/phases/70-cross-entity-traversal/deferred-items.md`.

- **`fac?ixlan__ix__fac_count__gt=0` (`pdb_api_test.py:2340`) is
  silent-ignored** — requires 3-hop traversal via `ixfac` which
  exceeds the documented 2-hop cap; Phase 72 will lock this as
  documented divergence. Tracked as DEFER-70-verifier-01 in
  `.planning/phases/70-cross-entity-traversal/deferred-items.md`. The
  generic 2-hop mechanism works for entity pairs with direct edges
  (e.g. `ixpfx?ixlan__ix__id=20`).

---

Historical release notes (v1.0 through v1.15) are archived in
[`.planning/MILESTONES.md`](./.planning/MILESTONES.md).
