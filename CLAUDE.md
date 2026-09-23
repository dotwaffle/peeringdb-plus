## Project

**PeeringDB Plus**

A high-performance, globally distributed, read-only mirror of PeeringDB data. It syncs PeeringDB objects incrementally by default (full re-fetch is an operator escape-hatch) on a regular schedule (default 1h, 15m when authenticated, or on-demand), stores them in SQLite on LiteFS for edge-local reads on Fly.io, and presents the data through modern API surfaces: GraphQL, gRPC, and OpenAPI-compliant REST. Built in Go using entgo as the ORM.

**Core Value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.

### Constraints

- **Language**: Go 1.27.1
- **ORM**: entgo (non-negotiable — ecosystem drives GraphQL/gRPC/REST generation)
- **Storage**: SQLite + LiteFS (enables edge distribution without a central database)
- **Platform**: Fly.io (LiteFS dependency, global edge deployment)
- **Observability**: OpenTelemetry — mandatory for tracing, metrics, and logs
- **Data fidelity**: Must handle PeeringDB's actual API responses, not their documented spec

## Documentation
- Canonical user/operator/contributor docs live in `docs/` (`ARCHITECTURE.md`, `CONFIGURATION.md`, `GETTING-STARTED.md`, `DEVELOPMENT.md`, `TESTING.md`, `API.md`, `DEPLOYMENT.md`) and `CONTRIBUTING.md` at the root. Read the relevant doc before re-deriving information from code or duplicating content into a response.
- `CLAUDE.md` is Claude's project memory, not user-facing docs. Keep it out of any docs-generation workflow; edit it directly.

## Technology Stack

Dependencies and standard build/test/lint commands are derivable from `go.mod` and `docs/DEVELOPMENT.md`.

LiteFS is in **maintenance mode** — stable but unsupported by Fly.io. No drop-in alternative exists for edge SQLite replication.

## Conventions

### Code Generation
- `go generate ./...` runs the full codegen pipeline and converges in a SINGLE pass on a clean tree (the schema producer is sequenced ahead of entc, its consumer):
  1. `ent/generate.go` — runs `cmd/pdb-schema-generate` (peeringdb.json → ent/schema/*.go) FIRST, then entc.go (ent + entgql + entrest + entproto), then `cmd/pdb-compat-allowlist`, then `buf generate` for proto Go types
  2. `graph/generate.go` — runs `gqlgen generate` for GraphQL resolvers/models. GOTCHA: gqlgen's config loader takes the package name from the alphabetically-FIRST `.go` file in `graph/` — a `package graph_test` file sorting before `custom.resolvers.go` breaks generation with "exec and model define the same import path (graph vs graph_test)". Name new test files so they sort after it (e.g. `resolver_*_test.go`).
  3. `internal/web/templates/generate.go` — runs `templ generate` for templ Go files
  - GOTCHA: `scalar Map` lives in `graph/schema.graphqls`, emitted by entgql because `Network.meta` / `NetworkIxLan.meta` use it. `graph/custom.graphql` must not redeclare it ("Cannot redeclare type Map"). entgql cannot see the custom.graphql declaration: its gqlgen schema load fails when run from `ent/`, so it always emits the builtin.
  - `schema/generate.go` carries no `go:generate` directive (package doc for the manual `pdb-schema-extract` step); the schema-regen step now lives first in `ent/generate.go`.
- `mise.toml` and `mise.lock` own Go and all contributor CLI versions; run `mise install --locked`, then invoke generators as ordinary binaries.
- Hand-edited schema methods live in `{type}_{method}.go` siblings — see "Hand-edited schema methods" below.
- Always commit `*_templ.go` alongside `.templ` changes, and generated `ent/`/`gen/`/`graph/` files alongside schema changes.
- Proto files: `proto/peeringdb/v1/v1.proto` (messages; entproto output frozen at v1.6, hand-maintained since), `services.proto` (RPCs, hand-written), `common.proto` (manual types like SocialMedia).
- `ent/entc.go` patches go-openapi/inflect to fix "campus" → "campu" mangling via `go:linkname`.

### Schema & Visibility

Two ent fields carry upstream PeeringDB visibility signals:

- `poc.visible` — row-level (`Public` / `Users` / `Private`). The ent Privacy policy admits `visible IN tier.AdmittedVisibilities() OR NULL`: TierPublic → `Public`; TierUsers → `Public`+`Users`; NO tier sees `Private` (upstream: owning-org members only; mirror has no org membership). Only entity where a whole row can be hidden.
- `privctx.Tier.AdmittedVisibilities()` is the single tier→visibility mapping; the poc policy, pdbcompat `applyVisibilityGate` (traversal subqueries) and `privfield.Redact` all use it; never hand-code a tier/visibility check.
- `ixlan.ixf_ixp_member_list_url_visible` — per-field (`Public` / `Users` / `Private`). Gates the sibling `ixf_ixp_member_list_url`; `internal/privfield.Redact` nulls/omits at the serializer layer across all 5 API surfaces. ent's built-in Privacy operates at query/row level only — field-level redaction is a serializer-layer concern.

### Field-level privacy

`internal/privfield.Redact(ctx, visible, value) (out string, omit bool)` is the single source of truth. Every API serializer calls it for each gated field; `privctx.TierFrom(ctx)` reads the tier stamped by `middleware.PrivacyTier`, and unstamped contexts fail-closed to `TierPublic`. Serializer surfaces that must call `Redact` today:

- **pdbcompat** — `internal/pdbcompat/serializer.go` `ixLanFromEnt(ctx, l)` → `ixfMemberListURLOut`; the pdbcompat-local `ixLanResponse` carries the URL as `*string` + `,omitempty`, so Redact's `omit` flag (not the value) decides the key: an admitted empty value keeps the key with `""` (upstream `permissions.py:344-353`). Exception: an empty `Users` value omits the key at every tier (an anonymous sync stores `""` for every `Users` row). `peeringdb.IxLan` stays a plain string: it decodes sync input.
- **ConnectRPC** — `internal/grpcserver/ixlan.go` `ixLanToProto(ctx, il)`; nil `*wrapperspb.StringValue` → wire omission. Convert closures at `ListIxLans` / `StreamIxLans` capture `ctx` via an adapter so the generic pagination helper's `Convert func(*E) *P` signature stays intact.
- **GraphQL** — `graph/gqlgen.yml` opts `IxLan.ixfIxpMemberListURL` into a custom resolver; `graph/schema.resolvers.go` `ixLanResolver.IxfIxpMemberListURL` returns `nil` (GraphQL `null`) when `omit=true`.
- **entrest** — `internal/middleware` `RESTFieldRedact` buffers ALL `/rest/v1/` responses and recursively deletes the JSON key from every object carrying the `_visible` companion (entrest eager-loads the ixlan edge unconditionally, so the gated field also appears under `edges.ix_lans`/`edges.ix_lan` on internet-exchange, ix-prefix, and network-ix-lan responses — path-scoping to `/rest/v1/ix-lans*` leaked it; fixed 2026-06-10). Wraps INSIDE `middleware.RESTError` so `application/problem+json` error bodies pass through untouched.
- **Web UI** — no current render path for the URL; when/if one is added, call `privfield.Redact` in the template data preparation step.

**Adding a new gated field:** call `privfield.Redact` at EACH of the 5 surfaces above (missing one = privacy leak); seed both gated + Public rows in `internal/testutil/seed.Full`; extend `cmd/peeringdb-plus/field_privacy_e2e_test.go` with `Redacted{Anon,UsersTier}` sub-tests and a fail-closed-bypass assertion.

**`_visible` companion emission:** the `_visible` field itself is STILL emitted for anonymous callers (upstream parity).

**NULL handling:** privacy policy treats NULL `*_visible` as the column default (`Public`), never `Users`. New auth-gated fields use `field.String` (not `Enum`); `internal/visbaseline/schema_alignment_test.go` flags upstream re-captures.

**Schema hygiene drop procedure.** `migrate.WithDropColumn(true)` + `migrate.WithDropIndex(true)` are permanently on. To drop an ent field, edit `schema/peeringdb.json`, run `go generate ./...`, remove references across `internal/{peeringdb,pdbcompat,grpcserver,sync}`, regenerate goldens (`go test -update ./internal/pdbcompat ./internal/sync`), deploy. See `docs/DEVELOPMENT.md` for the full step list.

**Hand-edited schema methods MUST live in sibling files** — `cmd/pdb-schema-generate` rewrites `ent/schema/{type}.go` from `schema/peeringdb.json` on every `go generate`, silently stripping anything hand-edited. Today's siblings:

- `ent/schema/poc_policy.go` — `(Poc).Policy()` privacy rule.
- `ent/schema/fold_mixin.go` + `ent/schema/{type}_fold.go` — `Mixin()` wiring for the 6 folded entities.
- `ent/schema/pdb_allowlists.go` — `schema.PrepareQueryAllows` map consumed by `cmd/pdb-compat-allowlist`.

When adding new hand-edited methods (Hooks, Policy, Annotations, Edges, Mixin), MOVE them to a sibling named `{type}_{method}.go`. ent's codegen discovers methods via reflection on the schema type — the file split is transparent.

Proto is hand-maintained since v1.6: entproto stays wired in `ent/entc.go` (`entproto.SkipGenFile`) but no ent schema carries an entproto annotation, so it writes nothing, and a new ent field reaches gRPC only through a deliberate hand edit of `proto/peeringdb/v1/v1.proto` (append with the next free number, never renumber; `buf generate` runs inside `go generate ./...`). Deliberate additions so far: `IxLan.ixf_ixp_member_list_url = 14` and `google.protobuf.Struct meta` on `Network` (41) / `NetworkIxLan` (19) for PeeringDB 2.83.0. `metaStruct` (`internal/grpcserver/convert.go`) turns nil/empty into an empty Struct (upstream `{}`) and logs + omits a document structpb cannot convert. Ent fields never added by hand (e.g. `Network.ixp_update_exclude`, `social_media`) are not on gRPC. Dropped ent fields whose proto wrappers still exist (e.g. `IxPrefix.notes`, `Organization.{fac,net}_count`) remain in `v1.proto` but serialize as zero-value pointers (absent on the wire). `internal/grpcserver/*` filter tables exempt them via `deprecatedFilterFields` in `filter_test.go`.

### Soft-delete tombstones

Tombstones (`status='deleted'`) are sourced **only** from upstream's explicit signal — the `?since=N` matrix returns the live statuses plus `deleted` (`ok`, plus `not-operational` on netixlan; per 2.83.0 `peeringdb_server/rest.py:719-750`). Inference-by-absence (the prior `markStaleDeleted*` family + `internal/sync/delete.go`) was removed: it mis-classified rows missing from partial responses and dropped children whose upstream-deleted parents we never synced. The dormant tombstone-GC work stays dormant.

**Bootstrap (zero-cursor handling):** v1.18.2's `?since=1` bootstrap was reverted in v1.18.3 — the full-historical fetch tripped upstream's `API_THROTTLE_REPEATED_REQUEST` cap. Current behaviour: zero cursor → fall through to bare `/api/<type>` (live statuses only). Historical-delete capture for fresh installs is deferred to a multi-cycle bootstrap design (v1.19+); FK backfill catches the orphans that matter on demand.

**Tombstone window on full-mode fetches (2026-06-10):** a bare list carries only live statuses and committing the snapshot advances the derived `MAX(updated)` cursor past the pre-cycle window, so full-mode staging over a populated table (daily `PDBPLUS_FULL_SYNC_INTERVAL` escalation, or the per-type incremental-fallback) ALSO fetches `?since=<cursor>` on top of the bare snapshot (`stageOneTypeToScratch`); scratch `INSERT OR REPLACE` makes window rows (incl. tombstones) win. Window-fetch failure fails the type — committing without it would permanently lose the window's deletes. Locked by `TestSync_FullModeFetchesTombstoneWindow` / `...FailureFailsCycle`.

**Deleted poc contact fields (PII):** upstream blanks `name`/`phone`/`email`/`url` of a `status='deleted'` poc whenever `status` is among the rendered fields (2.83.0 `serializers.py:2941-2954`, #569; pdbcompat blanks before `?fields=` projection, so it is stricter: `DIVERGENCE_deleted_poc_blanked_without_status_field`). `peeringdb.Poc.BlankDeletedContact` is the single rule. pdbcompat `pocFromEnt` applies it at render, so a stored tombstone that still holds contact data (the removed inference-by-absence code, v1.16.0-v1.18.1, kept it) never reaches `/api/`. Sync `upsertPocs` applies it at store, so the native surfaces (which serve stored values) never see contact data on a new tombstone. Legacy rows: `scrubDeletedPocContacts` (`internal/sync/poc_scrub.go`) runs on the primary once at scheduler start (`scrubPocContactsAtStartup`, own short tx, holds the `running` latch; closes the up-to-one-interval window before the first cycle commits) and in every sync tx after the upsert pass; status-indexed UPDATE, does NOT touch `updated` (cursor), zero writes when clean; WARN `scrubbed contact fields of deleted pocs` count>0 (DEBUG at 0) + span attr `pdbplus.sync.poc_contacts_scrubbed`. Locked by `TestParity_Status/deleted_poc_blanks_contact_fields`, `TestUpsert_BlanksDeletedPocContact`, `TestScrubDeletedPocContacts`, `TestSync_ScrubsLegacyPocTombstones`, `TestStartScheduler_ScrubsPocTombstonesAtStartup`.

**FK backfill on miss (`internal/sync/fk_backfill.go`):** `fkCheckParent` calls `fkBackfillBatch` when a parent isn't in our DB. Per-chunk pre-pass in `dispatchScratchChunk` collects all missing parent IDs across the chunk and issues ONE batched `/api/<parent>?since=1&id__in=<csv>` request per parent type via `peeringdb.Client.FetchByIDs` (chunked at `peeringdb.FetchByIDsBatchSize=100` IDs/HTTP-request internally). `upsertSingleRaw` (`internal/sync/upsert.go`) lands each fetched parent. The single-row `fkBackfillParent` is preserved as a thin wrapper over `fkBackfillBatch([]int{id})` for the worker.go callers. Recursive grandparent backfill **is** chained: when a backfilled parent has its own required-non-null FK to a missing grandparent, the grandparent is fetched too (BFS by parent type, bounded by FK depth = 3 max, deduped by the same per-cycle cache). Per-cycle bookkeeping: `Worker.fkBackfillTried` (dedup), `Worker.fkBackfillRequestCount` (HTTP requests issued, NOT rows), `Worker.fkBackfillRequestCap` (env `PDBPLUS_FK_BACKFILL_MAX_REQUESTS_PER_CYCLE` default 20; 0 disables backfill entirely), `Worker.fkBackfillDeadline` (env `PDBPLUS_FK_BACKFILL_TIMEOUT` default 5m — backfill HTTP work happens inside the sync tx, so the deadline keeps tx hold time bounded for LiteFS replication). `cycleStart` is still captured once at the top of `Worker.Sync` for memory telemetry + `fkBackfillTried` reset; **do NOT** call `time.Now()` inside per-entity closures.

**NetworkIxLan side FKs:** `net_side_id` and `ix_side_id` are nullable upstream (`null=True, on_delete=SET_NULL`). On miss, `fkFilter` calls `fkBackfillParent` first; if backfill fails or is disabled, the FK is nulled out (mirrors the existing `fac → campus` pattern at line 1234). Action recorded as `null` in the orphan summary.

**Rate-limited transport** (`internal/peeringdb/transport.go`): every PeeringDB call goes through a `*rate.Limiter` (env `PDBPLUS_PEERINGDB_RPS` default 2, burst 1 — no concurrency; auth path overrides to 1 req/sec = 60 req/min). On 429, parses `Retry-After` (numeric or HTTP-date), bounded retry (3 attempts) with cap. On 403 with WAF body signature (`AWS WAF`, `Request blocked`), logs WARN with headers and returns error (no retry). Telemetry: `pdbplus.peeringdb.requests{status_class}`, `pdbplus.peeringdb.rate_limit_wait_ms`, `pdbplus.peeringdb.retries{cause}`.

**No per-mutation tracing.** v1.18.6 removed `otelMutationHook` from the schema generator template (it created one OTel span per ent mutation, which inflated sync traces past Tempo's 7.5MB cap during 270k-object catch-up cycles). All `Hooks() []ent.Hook` returns `nil`. Per-cycle/per-type observability remains via `pdbplus.sync.{type.objects,duration}` + `sync-fetch-{type}` / `sync-upsert-{type}` step spans. If a per-Op tracing need re-emerges, restore at a coarser granularity (per-batch / per-chunk) — never per-mutation.

**pdbcompat invariants** (security-load-bearing):
- List path (`internal/pdbcompat/registry_funcs.go`) MUST append `applyStatusMatrix(live, isCampus, opts.Since != nil)` LAST in `preds` — mirrors upstream 2.83.0 `rest.py:719-750` status × since matrix. `live` = `pdbtypes.LiveStatuses(name)` (upstream `live_statuses()`: netixlan `ok`+`not-operational`, all others `ok`); no since → live, since → live+deleted(+pending on campus). A single live status emits `status = ?` so the `status` index serves the default `id` order with no sort; a multi-status set (netixlan, every `?since`) emits `likely(status IN (...))` so the planner reads the rowid table / `updated` index in list order instead of a status-leading index + temp B-tree (no `ANALYZE` stats in prod; plans locked by `TestPdbcompatListPlan_NoTempBTree`). `status` is an ordinary `Fields` key on all 13 types (upstream `status__iexact`, ANDed with the matrix; locked by `TestRegistryFields_AlignWithEntColumns`); LAST is what keeps `?status=` narrow-only.
- List order (`listOrder` in `registry_funcs.go`): plain list `id ASC` (upstream has no `ORDER BY` and no model `Meta.ordering`; live-verified 2026-09-23), `?since` list `updated ASC, id ASC` (`rest.py:744`). entrest/ConnectRPC keep their own `(-updated, -created, -id)`. `applySince` is `updated >= N`: upstream compares microsecond `updated` with `N.000000` and we store only the shown second (locked by `TestParity_Status/since_boundary_includes_same_second`).
- Unique-query 404 (`isUniqueQuery` in `handler.go`): a list with the `id` key (any type) or `asn` key (net), no `page` key, and zero served rows returns 404 `Entity not found` (upstream `rest.py:809-815`). It fires on all three empty exits of `serveList` (empty `__in`, budget `count == 0`, empty `List`) and runs after privacy filtering, so a hidden poc id is a 404 (registered divergence). A non-integer or empty `id`/`asn` value is still a 400 from `buildExact` (upstream `__iexact` matches nothing → 404; registered divergence `DIVERGENCE_unique_key_non_integer_returns_400`).
- PK-lookup (`internal/pdbcompat/depth.go`) MUST use `Query().Where(foo.ID(id), foo.StatusIn("ok", "pending")).Only(ctx)` — never `client.Foo.Get(ctx, id)` bare; netixlan uses `StatusIn("ok", "not-operational", "pending")` (live + pending, `rest.py:750`). Inline the `StatusIn` literal at each of the 27 call sites; grep-ability trumps DRY here.

**Native netixlan listings** (web fragments `internal/web/detail.go`, `internal/catalog` network/IX/compare, MCP `lookup_ip`) inline `networkixlan.StatusIn("ok", "not-operational", "pending")`: upstream 2.83.0 lists not-operational connections in its views and counts them in IX stats. REST/GraphQL/gRPC have no default status filter. Row markers (not operational, planned removal/activation `<date>`, RFC8950) come from `catalog.ConnectionMarkersFor` only: HTML badges via templ `connectionMarkers`, terminal via `writeConnectionMarkers`; not-operational = status `not-operational` OR (`ok` AND `operational=false`), i.e. upstream's `not x.operational` before and after its migration.

**Depth expansion** (`internal/pdbcompat/depth.go`, brought to full upstream parity in v1.20.5 — validated live 2026-06-08, locked by `depth_test.go`): `serveDetail` clamps `?depth=` to `[0,4]` (`handler.go`); each `getXWithDepth` branches bare (≤0) / `depth==1` (forward FK objects flat + reverse `_set` as bare ID lists) / `depth>=2` (full). The `nested<Type>Map` builders (`nestedOrg/Net/Fac/Ix/IxLan/Carrier/CampusMap`) render a singular FK object one level down — full object + its own reverse sets as ID lists + a FLAT sub-FK — and ARE the `depth==1` top-level shape, so parent getters reuse them. Direct reverse sets sort ascending (`sortedIDsOrEmpty`), EXCEPT the three facility-link sets (`net.netfac_set`, `ix.fac_set` via ixfac, `carrier.carrierfac_set`), which order by `(fac_id, id)` at depth 1 AND 2 (`intsOrEmpty` + query `Order`): upstream's prefetch has no ORDER BY and MySQL reads them through the unique `(<parent>, facility)` index (`models.py:3284/5998/6603`), confirmed live 2026-09-23 (net 20, ix 26). `ixlan.net_set` via netixlan keeps join order WITH duplicates (`intsOrEmpty`). `ixlan` exposes `net_set` (Networks resolved through the netixlan join, `getter="network"`), NOT `netixlan_set`. Sets are live-only at every depth — `StatusIn("ok")`, netixlan `StatusIn("ok", "not-operational")` (upstream nested prefetch, 2.83.0 `serializers.py:1140-1148`) — so a pending child (in practice a campus) is left out of its parent's set while its own PK lookup still returns it; through-relation sets filter the join row only (the resolved fac/net is unfiltered, `serializers.py:1678-1681`); `detailChildSets` repeats the set literals. Campus-less facilities emit `campus:null` at detail depth. Per-serializer back-ref strips differ (campus.fac_set drops `org_id`/keeps `campus_id`; carrier.carrierfac_set keeps `carrier_id`). Intentional non-parity: `poc_set` ID lists apply `poc.visible` privacy (omit non-Public ids upstream leaks); depths 3-4 render the depth-2 shape. Second-level FK objects stay flat.

Tombstone GC is dormant deferred work (triggers: storage >5% MoM, tombstone ratio >10%, operator request).

### Shadow-column folding

`internal/unifold` is the single source of truth for diacritic-insensitive folding (`Fold(s string) string` — NFKD normalisation via `golang.org/x/text/unicode/norm` + a hand-rolled ligature map for `ß→ss`, `æ→ae`, `ø→o`, `ł→l`, `þ→th`, `đ→d`, etc.). This mirrors upstream PeeringDB's `unidecode.unidecode(v)` (2.83.0 `peeringdb_server/rest.py:597`) without taking a third-party dep.

**16 `<field>_fold` shadow columns live across 6 entities:**

| Entity | Folded fields |
|---|---|
| `organization` | `name`, `aka`, `city` |
| `network` | `name`, `aka`, `name_long` |
| `facility` | `name`, `aka`, `city` |
| `internetexchange` | `name`, `aka`, `name_long`, `city` |
| `carrier` | `name`, `aka` |
| `campus` | `name` |

Each `_fold` column is declared with `entgql.Skip(SkipAll)` + `entrest.WithSkip(true)` annotations so it never leaks onto the GraphQL / REST / proto wire surfaces — these columns are server-side plumbing only. The 7 entity types without folded fields (`poc`, `ixlan`, `ixpfx`, `netixlan`, `netfac`, `ixfac`, `carrierfac`) leave `TypeConfig.FoldedFields` nil; nil-map reads in `ParseFilters` return `false` without a nil-check.

**Sync-side populate pattern (`internal/sync/upsert.go`):** every upsert in the 6 affected entity functions chains `.Set<Field>Fold(unifold.Fold(x.<Field>))` setters as a trailing grep-able block on the create builder. `OnConflict().UpdateNewValues()` rewrites `_fold` columns on every re-sync — no backfill script needed.

**pdbcompat filter-side routing pattern (`internal/pdbcompat/filter.go`):** `ParseFilters` reads `tc.FoldedFields[field]` (nil-safe) and threads `folded bool` into `buildPredicate`. When `folded == true`, `buildContains` / `buildStartsWith` route to `<field>_fold` with `unifold.Fold(value)` on the RHS via `sql.FieldContainsFold` / `FieldHasPrefixFold`. `__contains` and `__startswith` are coerced to their case-insensitive variants by `coerceToCaseInsensitive` per 2.83.0 `rest.py:657-662`.

**Adding a fold field** (existing or new entity): extend `foldMixin{fields: …}` in the entity's `ent/schema/{type}_fold.go` sibling, add the `.Set<Field>Fold(unifold.Fold(...))` setter to the matching `upsert<Type>s` chain in `internal/sync/upsert.go`, set `"<field>": true` in the entity's `FoldedFields` map in `internal/pdbcompat/registry.go`, and add a round-trip test in `internal/pdbcompat/fold_filter_test.go`. For a new (7th+) entity, also create the sibling file declaring `Mixin()`.

**Do NOT:**
- Edit the generated `ent/schema/{type}.go` to add `_fold` fields — `cmd/pdb-schema-generate` strips them. Use the sibling.
- Use `_fold` in non-pdbcompat surfaces — entrest/entgql/grpcserver already do ent-level `FieldContainsFold` / NOCASE. If a new surface needs diacritic folding, route through `internal/unifold.Fold`.
- Drop `entgql.Skip` / `entrest.WithSkip` from `foldMixin` — exposing the shadow as a separate filterable surface is meaningless to callers.

### Cross-entity `__` traversal

See `docs/API.md § Cross-entity traversal` for Path A (allowlist) / Path B (ent-edge introspection), 2-hop cap, `parseFieldOp` 3-tuple, and unknown-field diagnostics.

**Codegen invariants.** Static map emission, NOT runtime `client.Schema.Tables` walk. `cmd/pdb-compat-allowlist` reads `schema.PrepareQueryAllows` from `ent/schema/pdb_allowlists.go` → emits `internal/pdbcompat/allowlist_gen.go`. Every entry carries `// Source: serializers.py:<line>` (audit-required). Path B introspection: `internal/pdbcompat/introspect.go` (`LookupEdge` / `ResolveEdges` / `TargetFields`).

**Adding filters:** for 1-hop / 2-hop, add the key to the relevant entry's `Fields` slice in `ent/schema/pdb_allowlists.go` with a `// Source:` comment, then `go generate ./...`. Codegen routes 3-segment keys into `AllowlistEntry.Via` automatically. For excluded edges, attach `pdbcompat.WithFilterExcludeFromTraversal()` to the edge definition. For a 14th entity, add the mapping in `cmd/pdb-compat-allowlist/main.go` `pdbTypeMap` (`TestPdbTypeFor_AllThirteen` will fail until extended).

**Do NOT:**

- Hand-edit `internal/pdbcompat/allowlist_gen.go` — overwritten on every codegen run; CI drift gate catches it.
- Add traversal allowlists to grpcserver / entrest / GraphQL — out of v1.16 scope; those surfaces have their own filter models.
- Invent filter keys that don't exist upstream — contract is parity with `peeringdb/peeringdb@465931c0` (PeeringDB 2.83.0; `docs/API.md § Validation Notes` pins the full SHA).
- Add 3+-hop keys — dropped by codegen AND by the 2-hop cap in `parseFieldOp` at request time.
- Introduce runtime ent-client introspection or `sync.Once` lazy-init for the Edges map — map is codegen-time static, which avoids init-order coupling.

**Status-matrix and fold composition.** Traversal predicates compose with the status matrix (`applyStatusMatrix` still appended LAST in all 13 `registry_funcs.go` closures) and with the `_fold` routing (a folded traversal target uses `<field>_fold` with `unifold.Fold(value)` even when reached via `<fk>__<field>`). Regression-guarded by `TestTraversal_StatusMatrix_Preserved`, `TestTraversal_FoldRouting_Preserved`, `TestTraversal_EmptyIn_ShortCircuits` in `internal/pdbcompat/handler_test.go`.

**netixlan `meta__*` filters (`internal/pdbcompat/meta_filter.go`).** `ParseFiltersCtx` resolves them via `lookupMetaFilter` BEFORE `parseFieldOp`, mirroring upstream `finalize_query_params` (2.83.0 `serializers.py:3129-3149`), so the 3-/4-segment keys never reach traversal or the 2-hop cap. Keys come from `metaFilterColumns` (upstream `meta_registry.py:277-313`; the raw `meta_*` column names are aliases). Predicates read the stored document with `json_extract` (`json_type` for the bool), so an absent key is NULL and never matches (`meta__rfc8950=false` excludes rows that never declared it). Only upstream's operator set applies (`metaOperators`: lt/lte/gt/gte/contains/startswith/in); `__iexact`/`__icontains`/`__istartswith` on a meta key are ignored, as upstream does. net meta keys stay unfilterable (upstream ignores them). When upstream registers a new filterable key, add a `metaColumn` entry and a `TestParity_Meta` sub-test.

**Campus traversal fix (v1.18.0).** `<entity>?campus__<field>=X` previously returned 500 due to go-openapi/inflect mis-singularising "campus" → "campu" on the `cmd/pdb-compat-allowlist` codegen path. Fixed by sibling-file mixin `ent/schema/campus_annotations.go` (`entsql.Annotation{Table: "campuses"}`). The `entc.LoadGraph` runtime patch in `ent/entc.go` (`fixCampusInflection`) remains; the two are complementary, not redundant.

### Response memory envelope

See `docs/ARCHITECTURE.md § Response Memory Envelope` for budget, sizing table, lifecycle, telemetry. Invariants:

**Closure pairing:** the 13 List/Count pairs in `internal/pdbcompat/registry_funcs.go` are built by one generic `wireEntity` helper from a single shared predicate builder (v1.23.0), so budget pre-check and served response cannot disagree (the 413 guarantee). `applyStatusMatrix` LAST and the `opts.EmptyResult` short-circuit both live in exactly one place inside `wireEntity` — do not add per-entity closures outside it.

**Single-call-site telemetry:** `memStatsHeapInuseBytes` in `internal/pdbcompat/telemetry.go` is the ONLY call site for `runtime.ReadMemStats`; `recordResponseHeapDelta` fires once per request via `defer` in `dispatch` (covers list + detail terminal paths).

**Detail-path admission:** depth≥2 details charge the shared `inflightBytes` pool with a count-based fan-out estimate (child `COUNT(*)` × child `Depth0` per embedded `_set`, table in `internal/pdbcompat/detail_budget.go` mirroring the `get<Type>WithDepth` eager-loads). Changing a depth expansion's set list means updating `detailChildSets` too. The 413 check stays flat (`CheckBudget(1, type, depth, …)`) — fan-out feeds only the pool.

**Adding an entity type:** add a `typicalRowBytes` entry to `internal/pdbcompat/rowsize.go` (bench via `BenchmarkRowSize`, double the mean, round to 64 bytes), add a `wireEntity(entityWiring[...]{...})` entry in `registry_funcs.go` `init()`, extend the sizing table in `docs/ARCHITECTURE.md`, add under-/over-budget E2E cases mirroring `TestServeList_UnderBudgetStreams` / `TestServeList_OverBudget413`.

**Do NOT:**

- Call `runtime.ReadMemStats` per row — STW cost is µs but compounds quickly; the single-call-site `memStatsHeapInuseBytes` invariant is grep-enforceable.
- Skip the `CheckBudget` pre-flight for "trusted" entity types — none are trusted; the 256 MB replica cap is symmetric across all 13 types.
- Add a per-endpoint budget override — a single global budget keeps the operator mental model (and the Grafana panel legend) manageable.
- Bypass `wireEntity` with hand-written List/Count closures — the generic helper exists so the budget check and the served response can never become different queries (413 guarantee).
- Extend streaming/budget to grpcserver / entrest / GraphQL / Web UI — those surfaces have their own memory stories (see `docs/ARCHITECTURE.md § Response Memory Envelope` → Out of scope).

### Upstream parity regression

`internal/pdbcompat/parity/` locks pdbcompat semantics via 8 category-split test files (`{ordering,status,limit,unicode,in,traversal,meta,serializer}_test.go`) + `harness_helpers_test.go`. Each test seeds its own clean rows **inline** via the ent client and cites the upstream source line in a comment. The earlier ported-fixture pipeline (`internal/testutil/parity` + `cmd/pdb-fixture-port`) was removed: the ports carried unseedable Python-source artefacts (`**kwargs` splats, `SHARED[...]` refs) and 5 of 6 slices had zero behavioural consumers, while the `--check` drift gate was wired into nothing.

**Adding a parity test:** pick the category file matching the behaviour under test, add a sub-test under `TestParity_<Category>` with `t.Parallel()` and a citation comment (`// upstream: pdb_api_test.py:<line>` or `// synthesised: <context>`). Seed clean rows inline via `c.<Entity>.Create()` and the shared `harness_helpers_test.go` request/decode helpers (`newTestServer`, `httpGet`, `decodeDataArray`, `extractIDs`, `mustDecodeProblem`) — do NOT reach into `internal/testutil/seed.Full` (cross-test contamination).

**Divergence registry:** `docs/API.md § Known Divergences` is the SoT for intentional non-parity. Every entry has a matching `DIVERGENCE_<…>` sub-test. To add one: (1) write the parity test with `DIVERGENCE_` prefix, (2) append a Known Divergences row, (3) if it corrects a pdbfe-style claim, also append a Validation Notes row.

Bench envelopes in `bench_test.go` run locally — no CI benchstat gate.

### Middleware
- Response writer wrappers MUST implement `http.Flusher` (delegate to underlying writer) — gRPC streaming requires it.
- Add `Unwrap() http.ResponseWriter` for middleware-aware interface detection.
- Full chain (outermost first): `Recovery -> MaxBytesBody -> CORS -> OTel HTTP -> Logging -> PrivacyTier -> Readiness -> SecurityHeaders -> CSP -> Caching -> Gzip -> RouteTag -> mux`

### ConnectRPC / gRPC
- Services registered via loop in `cmd/peeringdb-plus/main.go` with otelconnect interceptor.
- Handler implementations in `internal/grpcserver/` — one file per entity type.
- `gen/peeringdb/v1/peeringdbv1connect/` contains generated handler interfaces.
- Proto `optional` fields generate pointer types (`*int64`, `*string`) — check `!= nil` for presence.

### Environment Variables

Authoritative table: `docs/CONFIGURATION.md`. Standard `OTEL_*` env vars also apply (autoexport).

Operationally-critical defaults worth retaining in-context (the surprising or load-bearing ones):

- `PDBPLUS_SYNC_MODE=incremental` (default flipped 2026-04-26 after the incremental-sync evaluation; `full` is operator escape-hatch for first-sync / recovery)
- `PDBPLUS_SYNC_INTERVAL` defaults `1h` unauthenticated / `15m` when `PDBPLUS_PEERINGDB_API_KEY` is set (auth-conditional)
- `PDBPLUS_FULL_SYNC_INTERVAL=24h`: the forced full cycle is the ONLY automatic repair for what incremental sync cannot see: count fields, netfac/ixfac name/city/country + carrierfac name (upstream copies them from the fac without bumping `updated`), live rows skipped by `updated` ties under offset paging (0160 gives ~600 netixlans one `updated`), values set upstream before our column existed (`meta`), orphan-nulled FKs. `0` leaves them stale until `POST /sync?mode=full`. Upstream `?since` is strict `updated > N`; it only looks inclusive because the wire `updated` is truncated to the second. See `docs/ARCHITECTURE.md § Daily full reconcile`.
- `PDBPLUS_RESPONSE_MEMORY_LIMIT=128MiB` — pdbcompat list pre-flight 413 budget; mandatory unit suffix (`KB`/`MB`/`GB`/`TB`); bare numbers rejected except literal `0` (disabled — dev only)
- `PDBPLUS_SYNC_MEMORY_LIMIT=400MB` — sync-cycle peak heap ceiling; unit suffix required; `0` disables
- `PDBPLUS_HEAP_WARN_MIB=400`, `PDBPLUS_RSS_WARN_MIB=384` — sync-cycle telemetry warn thresholds (Fly 512 MB cap; sustained breach re-opens the incremental-sync evaluation)
- `PDBPLUS_PEERINGDB_RPS=2` — sustained req/sec cap on every upstream PeeringDB call (sync + FK backfill share the budget); burst hardcoded at 1 to forbid concurrency; 429s honored via `Retry-After` (3-retry cap)
- `PDBPLUS_FK_BACKFILL_MAX_REQUESTS_PER_CYCLE=20` — per-cycle cap on **HTTP requests** issued by FK backfill (renamed v1.18.5 from MAX_PER_CYCLE which counted rows; rows is the wrong unit once `?id__in=` batches collapse N rows into 1 request). At 1 req/sec auth, 20 ≈ 20s of upstream pressure max per cycle. With `FetchByIDsBatchSize=100`, that covers up to 2,000 missing-parent rows. `0` disables backfill (orphans fall back to drop-on-miss)
- `PDBPLUS_FK_BACKFILL_TIMEOUT=5m` — wall-clock budget for backfill HTTP activity per sync cycle; bounds tx-hold time (backfill happens inside the sync tx). On deadline → drop-on-miss with `result=deadline_exceeded` metric
- `PDBPLUS_LOG_LEVEL=INFO` — minimum severity for the OTel logging branch (Loki). Stdout handler stays at INFO independently. Set `DEBUG` for opt-in deep debugging; invalid values fall back to INFO without crashing.
- `PDBPLUS_CSP_ENFORCE=false` — defaults to report-only; flip to `true` after v1.13 user-acceptance verification
- `PDBPLUS_PUBLIC_TIER=public` — set `users` only for private deployments (WARN at startup)
- `PDBPLUS_IS_PRIMARY=true` — fallback primary detection when LiteFS not present

### Testing
- Live tests (against `beta.peeringdb.com`): gated by `-peeringdb-live` flag, not run in CI.
- Test helpers: `internal/testutil/` — `SetupClient(t)` creates isolated in-memory SQLite client with `t.Cleanup`.
- Seed data: `internal/testutil/seed/` — `Full(tb, client)` seeds all 13 entity types with deterministic IDs.
- Fixtures: `testdata/fixtures/` — 13 JSON files matching PeeringDB API types, used by sync integration tests.

### Build
- Pure Go: `CGO_ENABLED=0` in Docker (modernc.org/sqlite is CGo-free); CI flips it to `1` only for the race detector.
- Chainguard base images (`cgr.dev/chainguard/go`, `cgr.dev/chainguard/glibc-dynamic`); prod flags: `-trimpath -ldflags="-s -w"`.
- LiteFS is a separate FUSE process — the app does not link to it. Prod Dockerfile uses `litefs mount` as entrypoint.

### LiteFS
- Lease file semantics are **inverted**: `/litefs/.primary` file **absent** = primary, **present** = replica (file contains primary hostname).
- Detection fallback (`internal/litefs/primary.go`): (1) check `.primary` file, (2) check if `/litefs/` dir exists, (3) fall back to `PDBPLUS_IS_PRIMARY` env var (default true for local dev).
- App serves directly on `:8080` with h2c — does NOT use LiteFS proxy (needed for gRPC/ConnectRPC support).

### CI
- 2 jobs on PR + main: `ci` (one cached mise/Go job running, in order: generated-code drift check, build, gotestsum race tests with coverage comment, lint, advisory vulnerability scan) and `docker-build` (dev + prod images, separate BuildKit `type=gha` cache).
- **Generated code drift check**: first step of the `ci` job — runs `mise run generate` then fails if `ent/`, `gen/`, `graph/`, `internal/web/templates/`, Tailwind output, or the pdbcompat allowlist differ from committed files.
- Coverage excludes `ent/` and `gen/` (generated code).
- govulncheck is advisory (`continue-on-error`): a flagged vuln warns but does not block merge.
- Linters: contextcheck, exhaustive, gocritic, gosec, misspell, modernize, nolintlint, revive (see `.golangci.yml`).

### Go Module
- `GONOSUMCHECK=* GONOSUMDB=*` may be needed for `go mod tidy` / `go get` when sumdb is read-only.
- `TMPDIR=/tmp/claude-1000` required for go commands in sandbox mode.

### Shell environment
- The Bash tool runs under `zsh`, which performs history expansion on `!` even in non-interactive scripts. Avoid `if ! cmd; then` and `! cmd1 | cmd2` — they silently drop the negation. Use count-based equivalents instead: `test "$(cmd | grep -c X)" -eq 0`.

### Deployment
- `fly deploy` from project root. App: peeringdb-plus. Primary region: lhr.
- LiteFS FUSE mount takes a moment — "not listening" warnings during rolling deploy are normal.
- **Asymmetric fleet (v1.15+).** Fly app `peeringdb-plus` runs two
  process groups: `primary` (1 machine, LHR, `shared-cpu-2x`/512 MB,
  persistent `litefs_data` volume) and `replica` (7 machines in other
  regions, `shared-cpu-1x`/256 MB, ephemeral rootfs). Replicas cold-sync
  the 88 MB DB from primary over LiteFS HTTP on boot (5-45s per region);
  `/readyz` fail-closes during hydration so Fly Proxy excludes them
  until ready. Replica recovery = destroy-and-recreate (no volume
  management). See `docs/DEPLOYMENT.md` § Asymmetric fleet.
- **Volume-only-on-primary.** `[[mounts]]` in `fly.toml` is scoped to
  `processes = ["primary"]`. Never re-introduce a mount on the replica
  group — the architecture assumes replicas are cattle.

### Sync observability

End-of-sync-cycle memory telemetry surfaces the sustained-high-heap trigger that re-opens the incremental-sync evaluation. Implementation: `internal/sync/worker.go` `emitMemoryTelemetry`, called from `recordSuccess`/`rollbackAndRecord`/`recordFailure` (the three terminal paths of `Worker.Sync`). Span attrs `pdbplus.sync.peak_heap_bytes` + `pdbplus.sync.peak_rss_bytes` are mirrored as Prom gauges `pdbplus_sync_peak_heap_bytes` / `pdbplus_sync_peak_rss_bytes` (bytes is the canonical Prom unit; dashboards format MiB at render). Zero-valued observations are suppressed.

**Log signal:** when a threshold is breached, worker emits `slog.Warn("heap threshold crossed", peak_heap_bytes, heap_warn_bytes, peak_rss_bytes, rss_warn_bytes, heap_over, rss_over)`. Thresholds gated by `PDBPLUS_HEAP_WARN_MIB` / `PDBPLUS_RSS_WARN_MIB` (defaults sit under the Fly 512 MB VM cap so order under pressure is: log → app crash → Fly OOM-kill).

**FK-orphan summary:** each sync cycle emits one `slog.Warn("fk orphans summary", total, groups)` (DEBUG when `total=0`) and increments `pdbplus.sync.type.orphans{type, parent_type, field, action}` per row. Per-row events log at DEBUG only — replaces the prior per-row WARN spam that breached Tempo's 7.5 MB per-trace cap.

**Escalation:** sustained peak heap above `PDBPLUS_HEAP_WARN_MIB` across multiple cycles re-opens the incremental-sync evaluation.

**Resource attribute filtering** (`internal/otel/provider.go` `buildResourceFiltered`): Grafana Cloud's hosted OTLP receiver only promotes `service.*` / `cloud.*` / `host.*` / `k8s.*` to Prom labels; custom `fly.*` keys are dropped on the metrics path. `service.instance.id` is stripped via `includeInstanceID=false` to bound fleet cardinality; `service.namespace` + `cloud.region` stay on metrics. Full attribute table + `http.route` middleware rationale in `docs/ARCHITECTURE.md`.

**Dashboards + alerts** in `deploy/grafana/{dashboards,alerts}/` — see `docs/DEPLOYMENT.md`. OTel runtime metrics (`go_memory_used_bytes` etc.) come from `runtime.Start(...)` and tick on every machine; `pdbplus_sync_peak_*` is primary-only.

**Prod debugging:** image ships with `sqlite3` — `fly ssh console -a peeringdb-plus -C 'sqlite3 /litefs/peeringdb-plus.db'`. Replicas expose the FUSE path read-only.
## Architecture

### API Surfaces (5)
- **Web UI**: `/ui/` — templ + htmx + Tailwind CSS (search, detail pages, ASN comparison). Content-negotiates: browsers get HTML, plain User-Agents (curl, wget, scripts) get ANSI-styled terminal text via `internal/web/termrender`. To smoke-test with curl, either send `-H 'User-Agent: Mozilla/5.0'` or strip ANSI: `sed 's/\x1b\[[0-9;]*[mGKH]//g'`.
- **GraphQL**: `/graphql` — gqlgen via entgql, interactive playground
- **REST**: `/rest/v1/` — entrest, OpenAPI-compliant
- **PeeringDB Compat**: `/api/` — drop-in replacement for PeeringDB API
- **ConnectRPC**: `/peeringdb.v1.*/` — Get/List RPCs for all 13 types with typed filtering, reflection, health check

### Key Packages

Most `cmd/*` and `internal/*` paths are self-describing; only the non-obvious ones are listed here.

Codegen tools (run by `go generate ./...`):
- `cmd/pdb-schema-extract/` — extract PeeringDB API responses to JSON
- `cmd/pdb-schema-generate/` — generate ent schemas from PeeringDB JSON
- `cmd/pdb-compat-allowlist/` — emits `internal/pdbcompat/allowlist_gen.go` from `ent/schema/pdb_allowlists.go`
- `cmd/pdbcompat-check/` — validate PeeringDB API compatibility

Operator tooling (NOT shipped in prod images, NOT invoked by CI):
- `cmd/loadtest/` — read-only HTTP traffic generator with 4 modes: `endpoints` (sweep), `sync` (replay 13-step ordered GET sequence), `soak` (sustained QPS-capped mixed load), `ramp` (per-surface inflection-point capacity probe → markdown table to stdout, added v1.18.7). Default `--target=https://peeringdb-plus.fly.dev`; **never** point at upstream `https://www.peeringdb.com` (1 req/hour/IP cap will block). See `cmd/loadtest/README.md` and `docs/DEPLOYMENT.md § Capacity probing`.

Single-source-of-truth packages:
- `internal/pdbtypes/` — leaf package (imports nothing) naming the 13 PeeringDB types across three domains (`Name`/`GoName`/`DjangoModel`); consumers derive their lists/maps from `pdbtypes.All`. `LiveStatuses(name)` mirrors upstream `live_statuses()` (feeds the pdbcompat status matrix). Exception: `internal/sync` keeps its own ordered step list — loadtest's ordering parity test cross-checks the two.
- `internal/pdbcompat/` — PeeringDB-compatible `/api` layer (filter routing, allowlist, status matrix, response budget)
- `internal/privfield/` `Redact(ctx, visible, value)` — field-level redaction across all 5 surfaces
- `internal/privctx/` `TierFrom(ctx)` — privacy tier reader
- `internal/unifold/` `Fold(s string) string` — diacritic-insensitive folding (mirrors upstream `unidecode`)
- `internal/visbaseline/` — visibility baseline + schema-alignment regression test
- `internal/litefs/` — primary/replica detection (inverted lease semantics)
- `internal/otel/` — dual logger, autoexport, resource-attr filtering, runtime metrics
- `ent/schema/` — entgo schemas; generated `{type}.go` files + hand-edited `{type}_{method}.go` siblings
- `gen/peeringdb/v1/` — generated proto Go types + ConnectRPC interfaces
- `proto/peeringdb/v1/` — proto source files
