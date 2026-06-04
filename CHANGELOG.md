# Changelog

All notable changes to this project are documented in this file.

The format follows [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Historical release notes prior to v1.16 are preserved in the project's
Git history (tags `v1.0.0` through `v1.15.0`).

## [Unreleased]

## [1.19.3] — 2026-06-04

### Fixed

- **`/api/<type>/<id>?depth=2` nested serialization now matches upstream
  PeeringDB.** A conformance comparison against live `www.peeringdb.com`
  surfaced three divergences in single-object depth-2 expansion, now
  corrected: nested reverse-set elements drop the parent back-reference
  FK (a netixlan embedded under a net no longer carries `net_id`); an
  embedded `org` carries its own reverse relations as bare ID lists
  (`net_set`/`fac_set`/…) instead of being flattened away; and the
  Facility serializer no longer embeds `netfac_set`/`ixfac_set`/
  `carrierfac_set` at depth=2 (a ~28x payload reduction for dense
  facilities), expanding only its `org` and `campus` FK objects. Flat
  (depth-0) list/detail responses were already byte-faithful across all
  13 types. The one remaining bounded gap — second-level nested FK
  objects stay flat — is recorded under § Known Divergences in the API
  reference.

## [1.19.2] — 2026-06-04

### Removed

- **Ported parity-fixture pipeline (`internal/testutil/parity` +
  `cmd/pdb-fixture-port`).** The ~55k lines of generated fixture data
  carried unseedable Python-source artefacts and were consumed by no
  behavioural test, and the `--check` drift command was wired into
  neither CI nor the `go generate` drift gate. The
  `internal/pdbcompat/parity` regression suite is unaffected: each test
  already seeds clean rows inline via the ent client and cites the
  upstream `pdb_api_test.py` source line in a comment.

### Added

- **Per-entity status×since matrix regression test** covering all 13
  PeeringDB types (previously only `net` + `campus` had behavioural
  coverage), guarding against a per-entity wiring omission that would
  leak `deleted`/`pending` rows onto the anonymous `/api` list surface.
- **`StreamIxLans` field-level redaction coverage** at both privacy
  tiers, closing the one load-bearing redaction surface that had no
  test for the gated `ixf_ixp_member_list_url` field.

### Changed

- **CI collapsed from four parallel Go jobs to one.** The separate
  lint / test / build / govulncheck jobs were folded into a single
  cached `ci` job that runs them in order (generated-code drift check →
  `go build` → race tests → `golangci-lint` → `govulncheck`) against one
  warmed module/build cache; `docker-build` stays a separate parallel
  job. `govulncheck` is now advisory (`continue-on-error`) — a flagged
  vulnerability warns but does not block the merge.
- **Five overview-dashboard panels reworked for honest rendering.**
  Sync Duration (p95) draws as points (sparse per-sync data, not a
  holey line); Sync Operations shows `round(increase())` bars (cycle
  counts, not a meaningless sub-0.01 ops/s rate); Request Rate by Route
  labels the empty-route series `(unrouted)` instead of "Value"; Latency
  adds p50 and excludes health/readiness probes (~92% of requests,
  which masked the real API tail); Objects Synced per Type rounds the
  edge-extrapolated counts.

### Fixed

- **`<field>_fold` shadow columns no longer leak on the REST surface.**
  The diacritic-folding shadow columns are server-side plumbing and are
  skipped on the GraphQL and proto wire surfaces, but the entrest path
  still emitted them; they are now stripped from `/rest/v1/` responses.
- **Sync-freshness gauge reported replica uptime, not data age.** The
  `pdbplus.sync.freshness` value was cached and refreshed only by the
  sync worker's `OnSyncComplete`, which never fires on replicas (they
  do not run the worker), so the gauge climbed with replica uptime — a
  node up 22h reported 22h of staleness while serving fresh data,
  tripping the >2h alert fleet-wide. It now reads `sync_status` live on
  each metric collection (a cheap single-row local lookup), so replicas
  report true data age and real LiteFS replication lag.

### Internal

- **`go generate ./...` now converges in a single pass.** The
  `pdb-schema-generate` step is sequenced ahead of `entc` inside
  `ent/generate.go`, so the schema producer always runs before its
  consumer and no second pass is needed.
- **Treewide removal of internal planning and process vocabulary** from
  comments, test identifiers, documentation, log/panic strings, the
  Grafana dashboards, and the code-generation sources. Behaviour-
  preserving; genuine external references (upstream source citations,
  versions, dates) are kept.

## [1.19.1] — 2026-05-31

### Changed

- **Vendored htmx bumped to 2.0.10.**

## [1.19.0] — 2026-05-31

_v1.17 and v1.18.x shipped as incremental patch work that was not
catalogued here individually; see the Git tags for those releases. The
entries below are the 2026-05-30 audit-hardening batch (merged via
PR #13)._

### Breaking

- **`PDBPLUS_INCLUDE_DELETED` is now a fatal startup error.** The v1.16
  deprecation logged a WARN and ignored the variable, promising a hard
  error in v1.17; that is now enforced. Remove it from your environment —
  sync always persists deleted rows as tombstones.

### Added

- **`__in` filtering on bool, float and time fields.**
  `?info_unicast__in=true,false`, `?latitude__in=…`, and
  `?created__in=<epoch>,<epoch>` now filter instead of returning `400`,
  matching upstream Django coercion. Values bind through ent's type
  converter (not the string `json_each` path), so time comparisons match
  the stored representation exactly.

### Changed

- **Repeated query parameters take the last value** (`?asn=1&asn=2` → `2`),
  matching Django's `QueryDict` and upstream PeeringDB (was first-value).
- **Filter type errors name the type** (`field type bool`) instead of the
  internal enum integer (`field type 2`).
- **`PDBPLUS_SYNC_STALE_THRESHOLD` is validated at startup** — a
  non-positive value is rejected at boot rather than pinning `/readyz` at
  503 for the process lifetime.
- **Detail endpoints (`/api/<type>/<id>`) are gated by the response memory
  budget**, like list endpoints — an over-budget `depth=2` expansion now
  returns `413` instead of being served unbounded.

### Security

- **`/api` 500 responses no longer echo raw ent/SQL error strings.** The
  driver error is logged server-side; the client receives a generic
  detail.
- **`Cache-Control: private` on Users-tier deployments.** When
  `PDBPLUS_PUBLIC_TIER=users`, responses carry private-audience data and
  are no longer marked `public`, so shared/CDN caches will not store them.
  Public deployments are unchanged (`public`).

### Performance

- List pages that serve no rows (e.g. `?skip=` past the end of the result
  set) short-circuit before the `ORDER BY … OFFSET` sort.
- The sync-freshness gauge reads a cached value instead of a live
  `sync_status` query on every Prometheus scrape.
- FK validation memoises confirmed-present parents per sync cycle,
  collapsing the per-child `Exist()` queries when many children share one
  untouched parent.
- pdbcompat list serialization builds its output in a single pass,
  dropping a redundant intermediate slice.
- The `/rest/v1/ix-lans` redaction path skips re-marshalling the body when
  no field was gated out.

### Fixed

- Corrected three documentation claims against the pinned upstream source:
  the detail `?depth=` default is `2` (not `0`); there is no top-level
  `meta.count` (the empty `__in` example is `{"data":[],"meta":{}}`); and
  `org_flags` is not an upstream filter parameter (recorded as a
  Validation Note, not a divergence).

### Internal

- Removed dead sync code (the write-only FK skipped-ID tracker and the
  unread `getStatus` filter) and the unused `litefs.IsPrimary` wrapper.
- Clarified stale doc comments: the per-request heap-delta metric is
  process-global, the FK-backfill cap default is 20, stream cursors are
  session-local, and the REST writers' `http.Flusher` contract (the
  pass-through writer delegates `Flush`; the buffering redact writer must
  not).

## [1.16.0] — 2026-04-19

v1.16 is a coordinated release. The cross-surface default ordering,
status×since matrix, `?limit=0` semantics, Unicode folding, cross-entity
traversal, and memory-safe response paths ship together in a single deploy
window; the upstream parity regression suite ships independently as a
code-only test lock-in. Do not deploy the `?limit=0` change in isolation —
pdbcompat `?limit=0` now returns all matching rows, and the memory-safe
response paths that bound that behaviour ship alongside it.

> **Coordinated release window:** the v1.16 behavioural changes are now
> complete and ready to deploy as a bundle. The `limit=0` unbounded
> semantics are safe in prod only with the memory budget in place — do
> NOT ship the unbounded-limit change without the memory budget. The
> parity regression test lock-in ships independently as a follow-up —
> no production deploy required; it is a CI regression gate only.
>
> **v1.16 complete (2026-04-19):** all behavioural changes shipped.
> Default ordering, status, limit, `__in`, Unicode, traversal, memory,
> and parity coverage are traced and complete.

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
  remains `250`. `?depth=` on list endpoints is silently ignored at this
  stage; list+depth support with the `API_DEPTH_ROW_LIMIT=250` cap
  follows later.

- **pdbcompat cross-surface default ordering** flipped to
  `(-updated, -created, -id)` (shipped earlier in v1.16).
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
  (`unidecode.unidecode(v)`).

- **pdbcompat operator coercion**: `__contains` is now equivalent to
  `__icontains` (case-insensitive) and `__startswith` is equivalent
  to `__istartswith`, per upstream `rest.py:638-641`. All other
  operators (`__exact`, `__iexact`, `__gt`, `__lt`, `__gte`, `__lte`,
  `__in`) are unchanged.

- **pdbcompat `__in` large-list support**: `?<field>__in=` now accepts
  arbitrarily-large comma-separated lists via a SQLite `json_each`
  single-bind rewrite, bypassing the 999-variable parameter limit.
  Empty `__in` (e.g. `?asn__in=`) returns `{"data":[],"meta":{}}`
  with no SQL executed, matching Django ORM `Model.objects.filter(id__in=[])`
  semantics.

- **pdbcompat fuzz corpus** extended with 21 non-ASCII and `__in`
  edge-case seeds (diacritics, CJK, RTL, RLO/LRO overrides, ZWJ,
  combining marks, null bytes, 70 KB literals, 1201-element `__in`,
  empty `__in`, all-empty `__in` parts). Local 60s run on a Ryzen 5
  3600 logged 469k executions / 65 new interesting / zero panics.

- **Cross-entity `__` traversal in pdbcompat.** The
  `/api/<type>?<fk>__<field>=` and
  `/api/<type>?<fk>__<fk>__<field>=` query shapes now resolve across
  foreign-key edges, mirroring upstream PeeringDB's `prepare_query`
  allowlists (Path A) and `queryable_relations()` auto-introspection
  (Path B). Hard-capped at 2 hops. Every 13-entity
  allowlist was translated 1:1 from `peeringdb_server/serializers.py`
  (SHA `99e92c72`); each annotation carries a `serializers.py:<line>`
  source comment for audit. A new codegen tool `cmd/pdb-compat-allowlist`
  reads ent schema annotations and emits
  `internal/pdbcompat/allowlist_gen.go` (Path A allowlists + Path B
  `Edges` map) wired into `go generate ./...` after ent codegen and
  before buf codegen. Example 2-hop case working:
  `GET /api/fac?ixlan__ix__fac_count__gt=0`.

- **Unknown filter fields silently ignored.**
  `GET /api/net?totally_unknown_field=x` returns HTTP 200 with a
  silently-unfiltered result rather than 400, matching upstream
  `rest.py:544-662`. Operators gain DEBUG-level visibility via
  `slog.DebugContext("pdbcompat: unknown filter fields silently ignored", ...)`
  and OTel span attribute
  `pdbplus.filter.unknown_fields` (CSV of all unknowns per request).
  The same diagnostic fires for typos, deprecated field names, and
  filter keys with >2 `__`-separated relation segments (the 2-hop cap).

- **2-hop cost ceiling (<50ms/op @ 10k rows).** New
  `BenchmarkTraversal_*` in `internal/pdbcompat/bench_traversal_test.go`
  plus a go-test-time `TestBenchTraversal_TwoHopCeiling` gate guard the
  2-hop query cost ceiling. A nightly CI workflow
  (`.github/workflows/bench.yml`) regression-gates via benchstat —
  prevents a future Cartesian-join regression from landing silently.

- **Memory-safe response paths on 256 MB replicas.**
  - **Streaming JSON emission** for pdbcompat list responses —
    `internal/pdbcompat/stream.go` `StreamListResponse` writes
    `{"meta":…,"data":[…]}` token-by-token via per-row `json.Marshal`
    and `http.Flusher.Flush()` every 100 rows. Replaces the legacy
    full-slice `json.NewEncoder` materialisation on the `serveList`
    path.
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
    doubled for headroom (13 types × 2 depths in
    `internal/pdbcompat/rowsize.go`).
  - **Per-request heap-delta telemetry.** New OTel span attribute
    `pdbplus.response.heap_delta_kib` (sampled once at handler entry
    and once at exit via `defer`; STW ~µs, NEVER per row) plus
    Prometheus histogram
    `pdbplus_response_heap_delta_kib{endpoint,entity}`. Registered
    via `pdbotel.InitResponseHeapHistogram()`. Grafana gains a
    "Response Heap Delta (KiB) — p50/p95/p99 by endpoint" panel (id
    36) at the bottom of the sync-memory watch row in
    `deploy/grafana/dashboards/pdbplus-overview.json`.
  - **`docs/ARCHITECTURE.md` § Response Memory Envelope** documents
    the envelope derivation, the three moving parts (stream / rowsize
    / budget), a per-entity worst-case sizing table with computed
    `max_rows` at the 128 MiB default, the request lifecycle, and the
    telemetry wire-up. `CLAUDE.md` gains a sibling § Response memory
    envelope convention with the maintainer checklist for
    adding new entity types.

- **Upstream parity regression lock-in.**
  - **`internal/pdbcompat/parity/` category-split regression suite** —
    6 test files (`ordering_test.go`, `status_test.go`,
    `limit_test.go`, `unicode_test.go`, `in_test.go`,
    `traversal_test.go`) + a shared `harness_helpers_test.go`. 31
    hard-pass tests total: 27 v1.16-semantic sub-tests covering the
    default-ordering, status, limit, Unicode, `__in`, and traversal
    behaviours plus 4 harness probes. 2 explicit
    `DIVERGENCE_` sub-tests lock the v1.16 silent-ignore semantic for
    the 3-hop `fac?ixlan__ix__fac_count__gt=0` case and the
    HTTP 500 outcome for the `fac?campus__name=X` case. 15
    `pdb_api_test.py`-or-synthesised citation hits, 36 `t.Parallel()`
    call sites, 4 `DIVERGENCE` markers.
    Suite wall time 15.4s under `-race`.
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
    and reports drift (advisory only, not blocking).
  - **`internal/pdbcompat/parity/bench_test.go` performance lock-in** —
    3 `b.Loop()`-style benchmarks:
    `BenchmarkParity_TwoHopTraversal` (`ixpfx?ixlan__ix__id=20`,
    ~580μs/op on Ryzen 5 3600),
    `BenchmarkParity_LimitZeroStreaming` (5000-row seeded end-to-end
    `stream.go` path, ~82.7ms/op),
    `BenchmarkParity_InFiveThousandElements` (5001-id IN via the
    `json_each` rewrite, ~98.6ms/op). Benchstat-on-main is out of
    scope — benchmarks are local-run only, gated
    by the standard `go test -race ./...` tier. `testing.TB` widening
    plumbed through `testutil.SetupClient` + 9 parity harness helpers
    (type-only change, every `*testing.T` call site still satisfies
    the interface).
  - **`docs/API.md § Known Divergences` extended** with 3 new rows
    (pre-soft-delete hard-delete gap parity cross-ref; pdbfe `limit=0`
    count-only invalid claim; depth-on-list silent-drop
    guardrail) plus `TestParity_*` cross-refs appended to the Since
    columns of the existing `fac?campus__name=X` +
    `fac?ixlan__ix__fac_count__gt=0` divergence rows.
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
    research the same invalid claims.

### Changed

- **Sync now soft-deletes** instead of hard-deleting. The 13
  `deleteStale*` functions in `internal/sync/delete.go` were renamed
  to `markStaleDeleted*`; they run
  `UPDATE ... SET status='deleted', updated=<cycle_start>` per sync
  cycle. One `cycleStart` timestamp is stamped on every tombstone
  within a cycle so `?since=N` windows stay atomic. Tombstone
  garbage-collection policy is deferred as dormant work (planted
  2026-04-19).

- **`parseFieldOp` signature extended** in
  `internal/pdbcompat/filter.go`. Return tuple expanded from
  `(field, op string)` to `(relationSegments []string, finalField,
  op string)` so the parser can detect `<fk>__<field>` patterns before
  consulting Path A / Path B and enforce the 2-hop cap.
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

- **Unknown filter fields silently ignored is a feature, not a bug.**
  Typos (`?nmae=x`), deprecated field names,
  and filter keys with >2 `__`-separated relation segments do not
  return HTTP 400 — the filter is silently dropped and the response
  contains the full unfiltered result set. This matches upstream
  PeeringDB (`rest.py:544-662`) and preserves existing client
  integrations that probe field names. Clients that want strict
  validation should inspect the OTel span attribute
  `pdbplus.filter.unknown_fields` or enable DEBUG-level logging to
  surface the dropped keys.

- **`campus` edge table-name codegen bug.**
  `cmd/pdb-compat-allowlist` emits `TargetTable: "campus"` instead
  of the correct `"campuses"` for all edges targeting the Campus
  entity, because `entc.LoadGraph` does not apply the
  `fixCampusInflection` patch used by the ent runtime codegen. Affected
  queries (e.g. `GET /api/fac?campus__name=X`) return
  `500 SQL logic error: no such table: campus (1)`. The outgoing
  edges FROM Campus are correct. Documented one-time gap; fix
  scheduled as a follow-up (preferred approach: add
  `entsql.Annotation{Table: "campuses"}` to `ent/schema/campus.go`).

- **`fac?ixlan__ix__fac_count__gt=0` (`pdb_api_test.py:2340`) is
  silent-ignored** — requires 3-hop traversal via `ixfac` which
  exceeds the documented 2-hop cap; the parity suite locks this as a
  documented divergence. The
  generic 2-hop mechanism works for entity pairs with direct edges
  (e.g. `ixpfx?ixlan__ix__id=20`).

---

Historical release notes (v1.0 through v1.15) are preserved in the
project's Git history (tags `v1.0.0` through `v1.15.0`).
