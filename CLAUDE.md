## Project

**PeeringDB Plus**

A high-performance, globally distributed, read-only mirror of PeeringDB data.
It syncs PeeringDB objects incrementally by default (a full re-fetch runs once per `PDBPLUS_FULL_SYNC_INTERVAL`, default 24h, or on operator request) on a regular schedule (default 1h, 15m when authenticated, or on-demand), stores them in SQLite on LiteFS for edge-local reads on Fly.io, and serves the data through six read surfaces: the PeeringDB-compatible `/api`, REST, GraphQL, ConnectRPC, MCP, and a Web UI.
Built in Go using entgo as the ORM.

**Core Value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.

### Constraints

- **Language**: Go 1.27.1
- **ORM**: entgo (non-negotiable — ecosystem drives GraphQL/gRPC/REST generation)
- **Storage**: SQLite + LiteFS (enables edge distribution without a central database)
- **Platform**: Fly.io (LiteFS dependency, global edge deployment)
- **Observability**: OpenTelemetry — mandatory for tracing, metrics, and logs
- **Data fidelity**: Must handle PeeringDB's actual API responses, not their documented spec

## Documentation

- Canonical user/operator/contributor docs live in `docs/` (`ARCHITECTURE.md`, `CONFIGURATION.md`, `GETTING-STARTED.md`, `DEVELOPMENT.md`, `TESTING.md`, `API.md`, `DEPLOYMENT.md`, `meta-generated-behavior.md`) and `CONTRIBUTING.md` at the root; operator tools: `cmd/loadtest/README.md`, `deploy/grafana/alerts/README.md`.
  Read the relevant doc before re-deriving information from code or duplicating content into a response.
- `CLAUDE.md` is Claude's project memory, not user-facing docs.
  Keep it out of any docs-generation workflow; edit it directly.

## Technology Stack

Dependencies and standard build/test/lint commands are derivable from `go.mod` and `docs/DEVELOPMENT.md`.

LiteFS is in **maintenance mode** — stable but unsupported by Fly.io.
No drop-in alternative exists for edge SQLite replication.

## Conventions

### Code Generation

- `go generate ./...` runs the full codegen pipeline and converges in a SINGLE pass on a clean tree (the schema producer is sequenced ahead of entc, its consumer):
  1. `ent/generate.go` — runs `cmd/pdb-schema-generate` (peeringdb.json → ent/schema/*.go) FIRST, then entc.go (ent + entgql + entrest + entproto), then `cmd/pdb-compat-allowlist`, then `buf generate` for proto Go types
  2. `graph/generate.go`: runs `gqlgen generate` for GraphQL resolvers/models.
     GOTCHA: gqlgen's config loader takes the package name from the alphabetically-FIRST `.go` file in `graph/` (today `complexity.go`); a `package graph_test` file sorting before it breaks generation with "exec and model define the same import path (graph vs graph_test)".
     Name new test files so they sort after it (e.g. `resolver_*_test.go`).
  3. `internal/web/templates/generate.go` — runs `templ generate` for templ Go files
  - GOTCHA: `scalar Map` lives in `graph/schema.graphqls`, emitted by entgql because `Network.meta` / `NetworkIxLan.meta` use it.
    `graph/custom.graphql` must not redeclare it ("Cannot redeclare type Map"). entgql cannot see the custom.graphql declaration: its gqlgen schema load fails when run from `ent/`, so it always emits the builtin.
  - `schema/generate.go` carries no `go:generate` directive (package doc for the manual `pdb-schema-extract` step); the schema-regen step now lives first in `ent/generate.go`.
- `mise.toml` and `mise.lock` own Go and all contributor CLI versions; run `mise install --locked`, then invoke generators as ordinary binaries.
- Hand-edited schema methods live in `{type}_{method}.go` siblings — see "Hand-edited schema methods" below.
- Always commit `*_templ.go` alongside `.templ` changes, and generated `ent/`/`gen/`/`graph/` files alongside schema changes.
- Proto files: `proto/peeringdb/v1/v1.proto` (messages; entproto output frozen at v1.6, hand-maintained since), `services.proto` (RPCs, hand-written), `common.proto` (manual types like SocialMedia).
- `ent/entc.go` patches go-openapi/inflect to fix "campus" → "campu" mangling via `go:linkname`.

### Schema & Visibility

Two ent fields carry upstream PeeringDB visibility signals:

- `poc.visible` — row-level (`Public` / `Users` / `Private`).
  The ent Privacy policy admits `visible IN tier.AdmittedVisibilities() OR NULL`: TierPublic → `Public`; TierUsers → `Public`+`Users`; NO tier sees `Private` (upstream: owning-org members only; mirror has no org membership).
  Only entity where a whole row can be hidden.
- `privctx.Tier.AdmittedVisibilities()` is the single tier→visibility mapping; the poc policy, pdbcompat `applyVisibilityGate` (traversal subqueries) and `privfield.Redact` all use it; never hand-code a tier/visibility check.
- Edge predicates bypass the policy: a `Has<Edge>With` neighbor predicate is plain SQL, so a filter over the `pocs` edge is a boolean oracle on hidden contact data.
  `cmd/pdb-schema-generate` `rowGatedEdgeTargets` emits `entgql.Skip(entgql.SkipWhereInput)` on every edge to `poc` (no `hasPocs`/`hasPocsWith` anywhere, incl. nested where-inputs).
  Any new filter path that reaches poc rows MUST apply `AdmittedVisibilities()` or be dropped.
  Locked by `TestGraphQLAPI_PocEdgeNotFilterable` + e2e `graphql_poc_edge_filter_rejected`.
- `ixlan.ixf_ixp_member_list_url_visible`: per-field (`Public` / `Users` / `Private`).
  Gates the sibling `ixf_ixp_member_list_url`; `internal/privfield.Redact` nulls/omits at the serializer layer of each surface that exposes it (all 6 surfaces must be checked). ent's built-in Privacy operates at query/row level only; field-level redaction is a serializer-layer concern.

### Field-level privacy

`internal/privfield.Redact(ctx, visible, value) (out string, omit bool)` is the single source of truth.
Every API serializer calls it for each gated field; `privctx.TierFrom(ctx)` reads the tier stamped by `middleware.PrivacyTier`, and unstamped contexts fail-closed to `TierPublic`.
Serializer surfaces that must call `Redact` today:

- **pdbcompat** — `internal/pdbcompat/serializer.go` `ixLanFromEnt(ctx, l)` → `ixfMemberListURLOut`; the pdbcompat-local `ixLanResponse` carries the URL as `*string` + `,omitempty`, so Redact's `omit` flag (not the value) decides the key: an admitted empty value keeps the key with `""` (upstream `permissions.py:344-353`).
  Exception: an empty `Users` value omits the key at every tier (an anonymous sync stores `""` for every `Users` row).
  `peeringdb.IxLan` stays a plain string: it decodes sync input.
- **ConnectRPC** — `internal/grpcserver/ixlan.go` `ixLanToProto(ctx, il)`; nil `*wrapperspb.StringValue` → wire omission.
  Convert closures at `ListIxLans` / `StreamIxLans` capture `ctx` via an adapter so the generic pagination helper's `Convert func(*E) *P` signature stays intact.
- **GraphQL** — `graph/gqlgen.yml` opts `IxLan.ixfIxpMemberListURL` into a custom resolver; `graph/schema.resolvers.go` `ixLanResolver.IxfIxpMemberListURL` returns `nil` (GraphQL `null`) when `omit=true`.
- **entrest**: `internal/middleware` `RESTFieldRedact` buffers ALL `/rest/v1/` responses except `/rest/v1/openapi.json` and walks the JSON recursively; in every object carrying the `_visible` companion it deletes the gated key when `Redact` returns `omit=true` (entrest eager-loads the ixlan edge unconditionally, so the gated field also appears under `edges.ix_lans`/`edges.ix_lan` on internet-exchange, ix-prefix, and network-ix-lan responses; path-scoping to `/rest/v1/ix-lans*` leaked it, fixed 2026-06-10).
  Wraps INSIDE `middleware.RESTError` so `application/problem+json` error bodies pass through untouched.
- **Web UI** — no current render path for the URL; when/if one is added, call `privfield.Redact` in the template data preparation step.
- **MCP**: no current path.
  Most tools return `internal/catalog` DTOs; `lookup_ip` (`internal/mcpserver/server.go`) returns raw ent `NetworkIxLan`/`IxPrefix` rows via their ent JSON tags, so a gated field added to those entities leaks there even if no DTO carries it.
  Call `Redact` where the output is built (for `lookup_ip`, map to a DTO first).

**Adding a new gated field:** call `privfield.Redact` at EACH of the 6 surfaces above (missing one = privacy leak); seed both gated + Public rows in `internal/testutil/seed.Full`; extend `cmd/peeringdb-plus/field_privacy_e2e_test.go` with tests modeled on `TestE2E_FieldLevel_IxlanURL_RedactedAnon` / `TestE2E_FieldLevel_IxlanURL_VisibleToUsersTier` and a `fail-closed-bypass-middleware` sub-test.
Contributor checklist: `docs/DEVELOPMENT.md § Adding a new field-level-privacy gated field`.

**`_visible` companion emission:** the `_visible` field itself is STILL emitted for anonymous callers (upstream parity).

**NULL handling:** only the poc row policy treats NULL `visible` as `Public`.
Field-level `*_visible` columns fail closed: `privfield.Redact` redacts NULL/empty/unknown values, and the ixlan `_visible` column defaults to `Private`.
New auth-gated fields use `field.String` (not `Enum`); `internal/visbaseline/schema_alignment_test.go` flags upstream re-captures.

**Schema hygiene drop procedure.**
`migrate.WithDropColumn(true)` + `migrate.WithDropIndex(true)` are permanently on.
To drop an ent field, edit `schema/peeringdb.json`, run `go generate ./...`, remove references across `internal/{peeringdb,pdbcompat,grpcserver,sync}`, regenerate goldens (`go test -update ./internal/pdbcompat ./internal/sync`), deploy.
See `docs/DEVELOPMENT.md` for the full step list.

**Hand-edited schema methods MUST live in sibling files** — `cmd/pdb-schema-generate` rewrites `ent/schema/{type}.go` from `schema/peeringdb.json` on every `go generate`, silently stripping anything hand-edited.
Today's siblings:

- `ent/schema/poc_policy.go` — `(Poc).Policy()` privacy rule.
- `ent/schema/fold_mixin.go` + `ent/schema/{type}_fold.go` — `Mixin()` wiring for the 6 folded entities.
- `ent/schema/pdb_allowlists.go` — `schema.PrepareQueryAllows` map consumed by `cmd/pdb-compat-allowlist`.

When adding new hand-edited methods (Hooks, Policy, Annotations, Edges, Mixin), MOVE them to a sibling named `{type}_{method}.go`. ent's codegen discovers methods via reflection on the schema type — the file split is transparent.

Proto is hand-maintained since v1.6: entproto stays wired in `ent/entc.go` (`entproto.SkipGenFile`) but no ent schema carries an entproto annotation, so it writes nothing, and a new ent field reaches gRPC only through a deliberate hand edit of `proto/peeringdb/v1/v1.proto` (append with the next free number, never renumber; `buf generate` runs inside `go generate ./...`).
Deliberate additions so far: `IxLan.ixf_ixp_member_list_url = 14` and `google.protobuf.Struct meta` on `Network` (41) / `NetworkIxLan` (19) for PeeringDB 2.83.0.
`metaStruct` (`internal/grpcserver/convert.go`) turns nil/empty into an empty Struct (upstream `{}`) and logs + omits a document structpb cannot convert.
Ent fields never added by hand (e.g. `Network.ixp_update_exclude`, `social_media`) are not on gRPC.
Dropped ent fields whose proto wrappers still exist (e.g. `IxPrefix.notes`, `Organization.{fac,net}_count`) remain in `v1.proto` but serialize as zero-value pointers (absent on the wire).
`internal/grpcserver/*` filter tables exempt them via `deprecatedFilterFields` in `filter_test.go`.

### Soft-delete tombstones

Tombstones (`status='deleted'`) come from upstream's explicit signal, plus one derived case, the netixlan cascade of a deleted net (below).
The `?since=N` matrix returns the live statuses plus `deleted` (`ok`, plus `not-operational` on netixlan; per 2.83.0 `peeringdb_server/rest.py:719-750`).
Inference-by-absence (the prior `markStaleDeleted*` family + `internal/sync/delete.go`) was removed: it mis-classified rows missing from partial responses and dropped children whose upstream-deleted parents we never synced.
The dormant tombstone-GC work stays dormant.

**Bootstrap (zero-cursor handling):** v1.18.2's `?since=1` bootstrap was reverted in v1.18.3: the full-historical fetch tripped upstream's `API_THROTTLE_REPEATED_REQUEST` cap.
Current behavior: zero cursor → bare `/api/<type>` (live statuses only), then a `?since=<newest updated in the snapshot>` window when the snapshot is non-empty (`snapshotWindowStart`; small, never `?since=1`; failure tolerated).
The history sweep (below) fetches the older tombstones over later cycles; FK backfill catches the orphans that matter on demand.

**Cursor = watermark (`internal/sync/watermark.go`, 2026-09-25):** raw table `sync_watermark` (type PK, max_updated Unix s, updated_at; `InitStatusTable`, and the sync tx runs the DDL again; NOT named `sync_cursors`: a6701e8 removed that DDL with no DROP, so prod can still hold a stale one).
Per type: M = pre-cycle `MAX(updated)` (`GetMaxUpdated`), K = stored mark.
Cursor E = 0 on an empty table (bare list, whatever K is), M with no row (migration fallback, INFO `sync watermark missing, using MAX(updated)`: expected once after deploy, and that cycle writes 13 rows), else `min(K, M)`; held = K < M.
`writeSyncWatermarks` runs in the sync tx after the cascade and `writeHistoryProgress`: S = newest `updated` without the ids in `Worker.fkBackfilled[type]` (`maxUpdatedExcluding`, ids bound as ONE JSON array via `json_each`); next W = E when held AND the type's tombstone window was discarded (`stageOutcome.windowDiscarded`), else max(E, S); when E = S = 0 (table empty at cycle start, only backfilled rows at the end) W = the oldest `updated` of the backfilled rows (`oldestUpdatedOf`: two backfill requests of one cycle can land rows at different times, and the newest row can be later than a delete of an older one; Codex 2026-09-25); W = 0 writes no row; conditional UPSERT (`WHERE max_updated IS NOT excluded.max_updated`) writes nothing when unchanged.
Why: FK backfill lands a parent that upstream changed after its type's fetch; a MAX(updated) cursor then jumped the gap and its tombstones were lost for good.
Without backfill W = MAX, so URLs and data writes stay byte-for-byte the same.
The hold rule applies to held types only (a type that is not held loses a discarded window's deletes, as before).
`fkBackfillBatch` is the only `upsertSingleRaw` caller (`TestUpsertSingleRaw_SingleCaller`); a new path that lands a row after its type's fetch must record its ids in `fkBackfilled` too.
Obs: fetch span `pdbplus.sync.cursor` / `.cursor.source` (`watermark`/`max_updated`/`empty`) / `.cursor.behind_seconds`; root span `pdbplus.sync.watermarks_written` / `_held` (in-tx); INFO `sync cursor held behind newest row` before the first request; after commit only, INFO `sync watermark behind backfilled rows` or WARN `sync watermark kept, tombstone window discarded` (repeats = that type's `?since` requests fail).
No new metric.
Deletes lost before this fix: history sweep / `POST /sync?mode=history`.
Locked by `watermark_test.go`: `TestSync_BackfilledParentKeepsCursor` (fails on the MAX cursor), `TestSync_EmptyTableBackfillKeepsOldestRow`, `TestSync_FacCampusBackfillKeepsCampusCursor`, `TestSync_GrandparentBackfillKeepsCursor`, `TestSync_SideFKBackfillKeepsFacCursor`, `TestSync_WatermarkEqualsMaxWithoutBackfill`, `TestSync_WatermarkFallsBackToMaxUpdated`, `TestSync_WatermarkMissingTable`, `TestSync_WatermarkClamp`, `TestSync_WatermarkReadErrorSendsNoRequest`, `TestSync_WatermarkRollsBackWithCycle`, `TestSync_FKDroppedNewestRowRetried`, `TestSync_FullModeWindowStartsAtWatermark`, `TestSync_WatermarkKeptWhenWindowDiscarded`, `TestSync_WatermarkSpanAttributes`, `TestNextWatermark`, `TestNewSyncCursor`, `TestReadSyncWatermarks`, `TestWriteSyncWatermarks`; `TestInitStatusTable_CreatesWatermarkTable`.

**History sweep (`internal/sync/history_sweep.go`):** each incremental cycle (never a full one) stages up to `PDBPLUS_HISTORY_MAX_REQUESTS_PER_CYCLE` (default 15, 0 = off) id windows `/api/<type>?since=1&status=deleted&id__gte=A&id__lt=B&depth=0` into scratch after `syncFetchPass`, before the cascade plan.
Types in `canonicalStepOrder` minus poc (upstream purges poc tombstones after 30 days); `historySpecs` widths keep a window under ~0.9 MB (upstream's repeated-URL throttle needs a prior response >= 1 MB); `hide_ix_no_fac=0` on ix/ixlan/net/netixlan; campus = one request without `status=deleted` (gets pending campuses).
A cycle stops the sweep after the last window of a type (`type_done`), so a type's parent windows are committed and the cycle's normal `?since` fetch lands any parent row the cursor rule dropped before the children are swept (no backfill request for a parent that upstream returns).
The last window of a type is open-ended (`from+width > local MAX(id)`).
Rows with `updated` later than the type's pre-cycle `MAX(updated)` are dropped (committing them would move the next watermark past rows changed after the normal fetch; the gate is NOT the watermark: a held cursor is below MAX and the normal fetch covers the rows between); scratch `ON CONFLICT` keeps the newer `updated` (`json_extract(CAST(data AS TEXT), ...)`: a BLOB argument is read as JSONB, so always CAST).
Phase B merges with the incremental gate; no deletes.
A type with an empty table is skipped, not marked done, and later types go on (`zero_cursor` when only such types are left).
Only fac `campus_id` (nullable) can point into an empty type (upstream has no live campus): `prefetchStagedFacCampuses` backfills a swept fac's campus, which then lets campus be swept; backfill off nulls it as on the normal path (Codex re-check 2026-09-24: kept as a known limit, no zero-cursor window).
Stops: budget, `type_done`, `complete`, memo hit, first failed window (429/WAF: WARN, `rate_limited`; other: WARN, `error`; never fails the cycle, except `errScratchDB`).
`Worker.historyMemo` (key = window, TTL 65m > 30s+2m+8m retry ladder and upstream's 1h repeat window) blocks resending a URL.
Progress = raw table `sync_history_sweep` (type, next_id, done, updated_at; `InitStatusTable`), written in the sync tx (`writeHistoryProgress`, nothing when unchanged), logged after commit (`logHistoryCommitted`).
`POST /sync?mode=history` (`config.SyncModeHistory`, rejected by `PDBPLUS_SYNC_MODE`) sets `withHistoryRestart`: `resolveEffectiveMode` returns incremental (never escalates to full) and the tx DELETEs all progress first.
Side effects: swept net tombstones with the RIR signature trigger cascade class A; swept rows use the shared FK backfill cap; a live row with an upstream tombstone that has a later `updated` becomes a tombstone (repairs pre-v1.28.1 rollback damage).
Full sweep 2026-09-24: 184 windows in 21 cycles at the default, <= ~115k new tombstones (id-gap bound).
Locked by `history_sweep_test.go` (`TestSync_HistorySweep*`, `TestNextHistoryWindow`, `TestHistoryWindowParams`, `TestHistorySweepTypes`), `TestSyncHandler_Mode`, `TestStreamWithParams`.

**Tombstone window on full-mode fetches (2026-06-10):** a bare list carries only live statuses and committing the snapshot advances the next watermark past the pre-cycle window, so full-mode staging (daily `PDBPLUS_FULL_SYNC_INTERVAL` escalation, or the per-type incremental-fallback) ALSO fetches a `?since=` window on top of the bare snapshot (`stageOneTypeToScratch`); scratch `INSERT OR REPLACE` makes window rows (incl. tombstones) win.
Window-fetch failure fails the type over a populated table in explicit full mode, because committing without it would permanently lose the window's deletes.
It is logged (WARN + `tombstone_window.discarded` span event) and tolerated on an empty table (next cycle's `?since=<cursor>` is the same window) and on the per-type incremental-fallback path, where the window is retried once and a second failure loses those deletes, unless the cursor is held (it keeps its watermark, see Cursor = watermark).
Locked by `TestSync_FullModeFetchesTombstoneWindow`, `TestSync_FullModeTombstoneWindowFailureFailsCycle` and `TestSync_ZeroCursorWindowFailureTolerated`.

**Sync fetch failures (v1.31.0):** `readSyncCursors` reads `sync_watermark` (`readSyncWatermarks`) and all 13 `MAX(updated)` before the first upstream request.
A read error fails the cycle (`failFetchStep`: span error status, `pdbplus.sync.type.fetch_errors`).
A watermark that is not a positive integer (text, real, <= 0; `badWatermarkError`) fails the fetch step of its type; an unreadable table fails the first step (org); a missing table = no marks (MAX fallback, the tx recreates it); a row of an unknown type is ignored.
Never substitute a zero cursor: its window starts at the snapshot's newest row, so a populated table loses the deletes before it.
Never fall back to MAX(updated) on a watermark error either: that brings back the backfill gap.
A scratch DB fault carries `errScratchDB`: `windowFailureTolerated` never tolerates it, and in the incremental attempt it fails the type with no fallback.
A failed window commits the rows it streamed (`keepRows`).
The incremental attempt and the snapshot roll back (`discardRows`).
Locked by `TestSync_CursorReadErrorFailsType`, `TestSync_CursorReadErrorSendsNoRequest`, `TestWindowFailureTolerated`, `TestStageOneTypeToScratch_ScratchFaultSkipsFallback`, `TestScratchDB_FailedStage`, `TestSync_FallbackWindowKeepsStagedTombstones`.

**Stale upstream snapshot (v1.28.1):** upstream serves the bare list from its `pdb_api_cache` files, which can be hours/days stale; `meta.generated` is the cache FILE mtime, later than the cache query cutoff (`updated__lte=<build start>`).
So (1) the window starts at `min(cursor, newest updated in the snapshot)` (`snapshotWindowStart`), never at `meta.generated`; the window is paged, so `updated` ties can still be skipped (next full cycle retries); (2) full mode's upsert gate is `(excluded.updated >= updated OR updated < <snapshot cutoff>) AND <a column differs>` (not `1=1`): equal rows still reconcile, a stored row newer than the snapshot's newest row is never rolled back, and an older stored row takes the snapshot's version even with an older `updated` (kept for raw saves outside a revision; an IX-F import-log rollback gets a new `updated`: it runs under `create_revision` and handleref `handle_version` re-saves the row, 2.83.0 `models.py:3745-3747`, handleref `models.py:9-16`).
Cutoffs flow `syncFetchPass` → `withReconcileAll(ctx, cutoffs)` keyed by table.
Pre-v1.28.1 full cycles also turned upstream deletes back into live rows; no sync repairs those (bare lists are live-only), prod had 352 such netixlans on 2026-09-23 (one-off repair, user decision: no reconcile code; unreturned ids stay live unless their net is deleted (netixlan cascade)).
Before v1.28.1 a full cycle over a stale cache rewrote rows (incl.
`updated`) back to their cached versions and the cursor never re-fetched them.
Locked by `stale_snapshot_test.go` (incl.
`TestSync_FullModeRepairsRevertedRow`) + `TestSync_FullModeReconcilesLocallyDivergedRows/full_keeps_newer_stored_row`.

**Deleted poc contact fields (PII):** upstream blanks `name`/`phone`/`email`/`url` of a `status='deleted'` poc whenever `status` is among the rendered fields (2.83.0 `serializers.py:2941-2954`, #569; pdbcompat blanks before `?fields=` projection, so it is stricter: `DIVERGENCE_deleted_poc_blanked_without_status_field`).
`peeringdb.Poc.BlankDeletedContact` is the single rule. pdbcompat `pocFromEnt` applies it at render, so a stored tombstone that still holds contact data (the removed inference-by-absence code, v1.16.0-v1.18.1, kept it) never reaches `/api/`.
Sync `upsertPocs` applies it at store, so the native surfaces (which serve stored values) never see contact data on a new tombstone.
Legacy rows: `scrubDeletedPocContacts` (`internal/sync/poc_scrub.go`) runs on the primary once at scheduler start (`scrubPocContactsAtStartup`, own short tx retried on lock errors (see Lock-error retry), holds the `running` latch; closes the up-to-one-interval window before the first cycle commits) and in every sync tx after the upsert pass; status-indexed UPDATE, does NOT touch `updated` (cursor), zero writes when clean; WARN `scrubbed contact fields of deleted pocs` count>0 (DEBUG at 0) from `logScrubbedPocContacts`, only after the owning tx commits (a failed commit logs only its failure) + span attr `pdbplus.sync.poc_contacts_scrubbed` (in-tx UPDATE count; the span ends before commit).
Locked by `TestParity_Status/deleted_poc_blanks_contact_fields`, `TestUpsert_BlanksDeletedPocContact`, `TestScrubDeletedPocContacts`, `TestSync_ScrubsLegacyPocTombstones`, `TestStartScheduler_ScrubsPocTombstonesAtStartup`, `TestScrubPocContactsAtStartup_CommitFails`.

**Deleted-net netixlan cascade (v1.28.2):**

- Upstream `pdb_rir_status` (2.83.0 `management/commands/pdb_rir_status.py:440-443`; cron ~22:55Z, ~0.7 nets/day) SQL-deletes a reclaimed net's `netixlan_set_active` (no tombstone), then soft-deletes the net.
- `cascadeDeletedNetIxLans` (`internal/sync/netixlan_cascade.go`) sets `status='deleted'`, `operational=false` (never `updated`) on live netixlans of a `status='deleted'` net whose `updated` is not later than the net's.
- Two sources:
  - (A) nets that turn deleted in this cycle: a scratch tombstone whose JSON has `rir_status` null and `rir_status_updated` set, and that is not `deleted` in the committed DB.
    Data only.
    The signature is read from the JSON, not from the stored columns.
  - (B) ids that `verifyNetIxLanCandidates` (`netixlan_verify.go`, Phase A, no tx) found absent, or deleted, in `/api/netixlan?hide_ix_no_fac=0&id__in=..&since=1`.
    Uses `peeringdb.Client.StreamByIDs`: one request per chunk; a body without `data` fails the chunk.
    Cap min(10, `PDBPLUS_FK_BACKFILL_MAX_REQUESTS_PER_CYCLE`), 2m.
- `hide_ix_no_fac=0` turns off upstream's per-user IXFilterMixin (`rest.py:1270-1297`).
- `Worker.netIxLanVerifyMemo` (keyed by netixlan id; an entry is dropped when the stored nix or net `updated` changes or the id is no longer a candidate):
  - live 24h (upstream-live orphans stay live);
  - gone 24h (retries send no repeat requests for absent ids);
  - failed 6h.
    A budget error (429/WAF/ctx/no HTTP response, `verifyBudgetError`) stops the pass with no memo entry.
    Any other error backs off the chunk, and retries use chunks of 10, then 1.
    The pass goes on after a failed chunk only if the chunk before it succeeded (a failing endpoint gets 1 request per pass).
- Upstream `deleted` verdicts at or before the netixlan cursor are staged into scratch and get NO memo entry (a retry has a new scratch, so it must re-stage; a gone entry would derive a tombstone that hides upstream's `updated`).
  Later ones: gone memo, the next `?since` lands upstream's row.
  Never derived at startup (`waiting`, no WARN).
- Kill switch: `PDBPLUS_FK_BACKFILL_MAX_REQUESTS_PER_CYCLE=0` stops verification (own request count, not backfill's); class A ignores the cap and no setting disables it.
- Runs in every sync tx after the upsert pass + poc scrub, and at primary start (`cascadeNetIxLansAtStartup`: own tx retried on lock errors (see Lock-error retry), `running` latch, panic firewall, verification once before the first attempt).
- `netIxLanUpsertPredicate` keeps a stored tombstone against a live row with the same `updated`.
  Real undeletes bump `updated`; residual: a same-second delete+revive.
- Residual: a net that upstream undeleted for a new owner (`undelete_for_new_owner`, `models.py:5542-5580`) before the mirror cascaded it keeps the old owner's hard-deleted netixlans live (only `status='deleted'` nets produce candidates).
- Log: WARN `cascaded network deletes to netixlans` {count, nets, backlog = class B rows, mode} (DEBUG at 0) and counter `pdbplus.sync.type.deleted{type=netixlan}`: both from `recordCascadeCommitted`, only after the owning tx commits.
  A failed commit logs only its failure.
  Span attrs `pdbplus.sync.netixlans_cascaded*` count the in-tx UPDATE; the span ends before commit.
  Also `verified netixlan cascade candidates` (Phase A HTTP work, not the commit).
- `/api/netixlan?since=N` returns the tombstone: `DIVERGENCE_deleted_net_netixlan_tombstone_in_since_window`.
- Locked by `TestCascadeDeletedNetIxLans*`, `TestVerifyNetIxLanCandidates*`, `TestRIRTransitionNets`, `TestSync_CascadesDeletedNetIxLans`, `TestSync_FullModeKeepsCascadedNetIxLanTombstone`, `TestUpsertNetworkIxLans_TombstoneGate`, `TestStartScheduler_CascadesNetIxLansAtStartup`, `TestStreamByIDs`.

**FK backfill on miss (`internal/sync/fk_backfill.go`):** `fkCheckParent` calls `fkBackfillBatch` when a parent isn't in our DB.
Per-chunk pre-pass in `dispatchScratchChunk` collects all missing parent IDs across the chunk and issues ONE batched `/api/<parent>?since=1&id__in=<csv>` request per parent type via `peeringdb.Client.FetchByIDs` (chunked at `peeringdb.FetchByIDsBatchSize=100` IDs/HTTP-request internally).
`upsertSingleRaw` (`internal/sync/upsert.go`) lands each fetched parent, and `fkBackfillBatch` records each landed id in `Worker.fkBackfilled[parentType]` (reset per cycle by `resetFKState`), so the row does not move the next watermark of its type (see Cursor = watermark).
The single-row `fkBackfillParent` is preserved as a thin wrapper over `fkBackfillBatch([]int{id})` for the worker.go callers.
A backfilled row skips `fkFilter`, so `nullMissingOptionalFKs` (`optionalFKSpec`: fac `campus_id`) nulls its missing nullable parents before `upsertSingleRaw`, with no campus backfill (a backfilled fac is in practice a deleted one, so this spares the request cap).
Before this fix a backfilled fac stored a dangling `campus_id` and the deferred FK check failed every cycle at COMMIT (`TestFKBackfill_NullsMissingCampusOfBackfilledFac`).
`fkHasParent` treats an id <= 0 as missing (a null or absent upstream FK decodes to 0): a required FK drops the row (chunk) or withholds the backfilled parent (`firstMissingRequiredFK`), an optional one is nulled, and no backfill request is sent (`TestSync_NonPositiveFKIDs`).
The sync tx checks FKs per statement (the SQLite default; `PRAGMA defer_foreign_keys` was dropped 2026-09-24): a dangling write fails its own statement and the tx stays usable (`TestSync_ForeignKeysCheckedPerStatement`).
Recursive grandparent backfill **is** chained: when a backfilled parent has its own required-non-null FK to a missing grandparent, the grandparent is fetched too (BFS by parent type, bounded by FK depth = 3 max, deduped by the same per-cycle cache).
Per-cycle bookkeeping: `Worker.fkBackfillTried` (dedup), `Worker.fkBackfillRequestCount` (HTTP requests issued, NOT rows), `Worker.fkBackfillRequestCap` (env `PDBPLUS_FK_BACKFILL_MAX_REQUESTS_PER_CYCLE` default 20; 0 disables backfill entirely; also the netixlan cascade verification cap, see above), `Worker.fkBackfillDeadline` (env `PDBPLUS_FK_BACKFILL_TIMEOUT` default 5m: backfill HTTP work happens inside the sync tx, so the deadline keeps tx hold time bounded for LiteFS replication).
`cycleStart` is still captured once at the top of `Worker.Sync` for memory telemetry + `fkBackfillTried` reset; **do NOT** call `time.Now()` inside per-entity closures.

**Nullable FKs (NetworkIxLan side FKs, fac `campus_id`):** `net_side_id` and `ix_side_id` are nullable upstream (`null=True, on_delete=SET_NULL`), and so is fac `campus_id`.
On a miss, the `fkFilter` closure (`internal/sync/registry.go`) calls `nullOptionalFK` (`worker.go`, `nullSideFK` wraps it for the side FKs), which tries `fkBackfillParent` first.
If backfill fails or is off, it sets the FK to NULL and keeps the row.
Action recorded as `null` in the orphan summary.
Nullable FKs stay out of `fkRefs`.
Fac campuses are fetched ONCE per type instead: the fac descriptor's `prefetchStaged` hook (`prefetchStagedFacCampuses`, run by `drainAndUpsertType` before the first chunk) reads the distinct `campus_id`s of the staged facs from scratch and batches the missing ones, so the per-row step sends no request.
Do not put `campus_id` in `fkRefs`: a pending campus has at most one fac, so the missing campuses spread across chunks and a per-chunk fetch used one request per chunk from the shared cap, starving the required backfills of later types (`TestSync_FacCampusBackfillSparesRequestCap`).
The single pass still draws on the shared cap (1 request per 100 missing campuses, 0 once they are stored), so a cap of 1 can still starve a later required parent.
Accepted (user, 2026-09-24, Codex review), as for the side FKs.
A bare `/api/campus` list holds only `ok` rows, so a pending campus (upstream: fewer than two facilities) that did not change since bootstrap reaches the mirror only through this backfill.
Before the fix every full cycle nulled 3 prod facs' `campus_id` (`TestSync_BackfillsPendingFacCampus`, `TestSync_FacCampusNulledWhenBackfillOff`, `TestSync_FacCampusNulledWhenAbsentUpstream`).
The per-row backfill step is the only path for the side FKs (`TestFKFilter_NetworkIxLan_BackfillsSideFK`).

**Rate-limited transport** (`internal/peeringdb/transport.go`; the limiter is built in `client.go`): every PeeringDB call goes through a `*rate.Limiter` (env `PDBPLUS_PEERINGDB_RPS` default 1/3 = 20 req/min, burst 1 = no concurrency; auth path overrides to 30 req/min).
Upstream documents 20/min per IP anonymous and 40/min per user or org key authenticated, and asks for 2 s between queries (docs.peeringdb.com "Work Within PeeringDB's Query Limits"; v1.31.1 and earlier ran 2 rps anonymous and 60/min authenticated).
500/502/503/504 are retried separately by `doWithRetry` in `client.go` (3 attempts, backoff).
On 429, parses `Retry-After` (numeric or HTTP-date), bounded retry (3 attempts) with cap.
On 403 with WAF body signature (`AWS WAF`, `Request blocked`), logs WARN with headers and returns error (no retry).
Telemetry: `pdbplus.peeringdb.requests{status_class}`, `pdbplus.peeringdb.rate_limit_wait_ms`, `pdbplus.peeringdb.retries{cause}`.

**Upsert conflict action (`internal/sync/upsert.go`):** every entity upsert passes `resolveWithRow(migrate.<Table>Table)` to `OnConflict`, which sets every non-key column from `excluded`.
Never use `sql.ResolveWithNewValues()`: it sets only the INSERT's columns, and a bulk INSERT leaves out a `SetNillable*` column that is nil on every row of the batch, so a value that upstream cleared stayed stored (fixed in v1.28.4; 33 columns in 8 tables, e.g. netixlan `ipaddr6`, fac `campus_id`, net `rir_status`).
SQLite fills a column that the INSERT does not list with its default in `excluded`.
Locked by `TestUpsert_ConflictSetsEveryColumn` (all 13 upserts) and `TestUpsert_NilValueClearsStoredValue`.
Full mode ANDs `writeRowDiffers` (`excluded.c IS NOT c` over the same `nonKeyColumns`) into the gate: SQLite deletes and re-inserts the index entries of every SET column even when the value is unchanged, so without it each daily full commit wrote about every index page (indexes = 53 of 121 MiB, 2026-09-24) and replicas applied it with all WAL locks held (readers fail with SQLITE_PROTOCOL after ~10s).
Locked by `TestSync_FullModeWritesOnlyChangedRows` (second identical full cycle updates 0 rows, via triggers) and the `compared` check in `TestUpsert_ConflictSetsEveryColumn`.

**No per-mutation tracing.** v1.18.6 removed `otelMutationHook` from the schema generator template (it created one OTel span per ent mutation, which inflated sync traces past Tempo's 7.5MB cap during 270k-object catch-up cycles).
All `Hooks() []ent.Hook` returns `nil`.
Per-cycle/per-type observability remains via `pdbplus.sync.{type.objects,duration}` + `sync-fetch-{type}` / `sync-upsert-{type}` step spans.
If a per-Op tracing need re-emerges, restore at a coarser granularity (per-batch / per-chunk) — never per-mutation.

**pdbcompat invariants** (security-load-bearing):

- List path (`internal/pdbcompat/registry_funcs.go`) MUST append `applyStatusMatrix(live, isCampus, opts.Since != nil)` LAST in `preds` — mirrors upstream 2.83.0 `rest.py:719-750` status × since matrix.
  `live` = `pdbtypes.LiveStatuses(name)` (upstream `live_statuses()`: netixlan `ok`+`not-operational`, all others `ok`); no since → live, since → live+deleted(+pending on campus).
  A single live status emits `status = ?` so the `status` index serves the default `id` order with no sort; a multi-status set (netixlan, every `?since`) emits `likely(status IN (...))` so the planner reads the rowid table / `updated` index in list order instead of a status-leading index + temp B-tree (no `ANALYZE` stats in prod; plans locked by `TestPdbcompatListPlan_NoTempBTree`).
  `status` is an ordinary `Fields` key on all 13 types (upstream `status__iexact`, ANDed with the matrix; locked by `TestRegistryFields_AlignWithEntColumns`); LAST is what keeps `?status=` narrow-only.
- List order (`listOrder` in `registry_funcs.go`): plain list `id ASC` (upstream has no `ORDER BY` and no model `Meta.ordering`; live-verified 2026-09-23), `?since` list `updated ASC, id ASC` (`rest.py:744`). entrest/ConnectRPC keep their own `(-updated, -created, -id)`.
  `applySince` is `updated >= N`: upstream compares microsecond `updated` with `N.000000` and we store only the shown second (locked by `TestParity_Status/since_boundary_includes_same_second`).
- Unique-query 404 (`isUniqueQuery` in `handler.go`): a list with the `id` key (any type) or `asn` key (net), no `page` key, and zero served rows returns 404 `Entity not found` (upstream `rest.py:809-815`).
  It fires on all three empty exits of `serveList` (empty `__in`, budget `count == 0`, empty `List`) and runs after privacy filtering, so a hidden poc id is a 404 (registered divergence).
  A non-integer or empty `id`/`asn` value is still a 400 from `buildExact` (upstream `__iexact` matches nothing → 404; registered divergence `DIVERGENCE_unique_key_non_integer_returns_400`).
- PK-lookup (`internal/pdbcompat/depth.go`) MUST use `Query().Where(foo.ID(id), foo.StatusIn("ok", "pending")).Only(ctx)` — never `client.Foo.Get(ctx, id)` bare; netixlan uses `StatusIn("ok", "not-operational", "pending")` (live + pending, `rest.py:750`).
  Inline the `StatusIn` literal at each of the 27 call sites; grep-ability trumps DRY here.

**Native netixlan listings** (web fragments `internal/web/detail.go`, `internal/catalog` network/IX/compare, MCP `lookup_ip`) inline `networkixlan.StatusIn("ok", "not-operational", "pending")`: upstream 2.83.0 lists not-operational connections in its views and counts them in IX stats.
REST/GraphQL/gRPC have no default status filter.
Row markers (not operational, planned removal/activation `<date>`, RFC8950) come from `catalog.ConnectionMarkersFor` only: HTML badges via templ `connectionMarkers`, terminal via `writeConnectionMarkers`; not-operational = status `not-operational` OR (`ok` AND `operational=false`), i.e. upstream's `not x.operational` before and after its migration.

**Depth expansion** (`internal/pdbcompat/depth.go`, brought to full upstream parity in v1.20.5 — validated live 2026-06-08, locked by `depth_test.go`): `serveDetail` clamps `?depth=` to `[0,4]` (`handler.go`); each `getXWithDepth` branches bare (≤0) / `depth==1` (forward FK objects flat + reverse `_set` as bare ID lists) / `depth>=2` (full).
The `nested<Type>Map` builders (`nestedOrg/Net/Fac/Ix/IxLan/Carrier/CampusMap`) render a singular FK object one level down — full object + its own reverse sets as ID lists + a FLAT sub-FK — and ARE the `depth==1` top-level shape, so parent getters reuse them.
Direct reverse sets sort ascending (`sortedIDsOrEmpty`), EXCEPT the three facility-link sets (`net.netfac_set`, `ix.fac_set` via ixfac, `carrier.carrierfac_set`), which order by `(fac_id, id)` at depth 1 AND 2 (`intsOrEmpty` + query `Order`): upstream's prefetch has no ORDER BY and MySQL reads them through the unique `(<parent>, facility)` index (`models.py:3284/5998/6603`), confirmed live 2026-09-23 (net 20, ix 26).
`ixlan.net_set` via netixlan keeps join order WITH duplicates (`intsOrEmpty`).
`ixlan` exposes `net_set` (Networks resolved through the netixlan join, `getter="network"`), NOT `netixlan_set`.
Sets are live-only at every depth: `likelyOK` (`likely(status IN ('ok'))`, which keeps the set query on the FK index; plans locked by `TestDetailPlan_KeepsFKIndex`), netixlan `StatusIn("ok", "not-operational")` (upstream nested prefetch, 2.83.0 `serializers.py:1140-1148`).
A pending child (in practice a campus) is left out of its parent's set while its own PK lookup still returns it; through-relation sets filter the join row only (the resolved fac/net is unfiltered, `serializers.py:1678-1681`); `detailChildSets` repeats the set filters.
Campus-less facilities emit `campus:null` at detail depth.
Per-serializer back-ref strips differ (campus.fac_set drops `org_id`/keeps `campus_id`; carrier.carrierfac_set keeps `carrier_id`).
Intentional non-parity: `poc_set` ID lists apply `poc.visible` privacy (omit non-Public ids upstream leaks); depths 3-4 render the depth-2 shape.
Second-level FK objects stay flat.

Tombstone GC is dormant deferred work (triggers: storage >5% MoM, tombstone ratio >10%, operator request).

### Shadow-column folding

`internal/unifold` is the single source of truth for diacritic-insensitive folding (`Fold(s string) string` — NFKD normalisation via `golang.org/x/text/unicode/norm` + a hand-rolled ligature map for `ß→ss`, `æ→ae`, `ø→o`, `ł→l`, `þ→th`, `đ→d`, etc.).
This mirrors upstream PeeringDB's `unidecode.unidecode(v)` (2.83.0 `peeringdb_server/rest.py:597`) without taking a third-party dep.

**16 `<field>_fold` shadow columns live across 6 entities:**

| Entity | Folded fields |
|---|---|
| `organization` | `name`, `aka`, `city` |
| `network` | `name`, `aka`, `name_long` |
| `facility` | `name`, `aka`, `city` |
| `internetexchange` | `name`, `aka`, `name_long`, `city` |
| `carrier` | `name`, `aka` |
| `campus` | `name` |

Each `_fold` column is declared with `entgql.Skip(SkipAll)` + `entrest.WithSkip(true)` annotations so it never leaks onto the GraphQL / REST / proto wire surfaces — these columns are server-side plumbing only.
The 7 entity types without folded fields (`poc`, `ixlan`, `ixpfx`, `netixlan`, `netfac`, `ixfac`, `carrierfac`) leave `TypeConfig.FoldedFields` nil; nil-map reads in `ParseFilters` return `false` without a nil-check.

**Sync-side populate pattern (`internal/sync/upsert.go`):** every upsert in the 6 affected entity functions chains `.Set<Field>Fold(unifold.Fold(x.<Field>))` setters as a trailing grep-able block on the create builder.
The upsert (`resolveWithRow` + `UpdateWhere(skipUnchangedPredicate)`) rewrites a row and its `_fold` columns only when upstream `updated` advanced; a full-mode cycle (`withReconcileAll`, daily by default) also rewrites equal-`updated` rows that differ, so a newly added `_fold` column fills then; no backfill script needed.

**pdbcompat filter-side routing pattern (`internal/pdbcompat/filter.go`):** `ParseFilters` reads `tc.FoldedFields[field]` (nil-safe) and threads `folded bool` into `buildPredicate`.
When `folded == true`, `buildContains` / `buildStartsWith` route to `<field>_fold` with `unifold.Fold(value)` on the RHS via `sql.FieldContainsFold` / `FieldHasPrefixFold`.
`__contains` and `__startswith` are coerced to their case-insensitive variants by `coerceToCaseInsensitive` per 2.83.0 `rest.py:657-662`.

**Adding a fold field** (existing or new entity): extend `foldMixin{fields: …}` in the entity's `ent/schema/{type}_fold.go` sibling, add the `.Set<Field>Fold(unifold.Fold(...))` setter to the matching `upsert<Type>s` chain in `internal/sync/upsert.go`, set `"<field>": true` in the entity's `FoldedFields` map in `internal/pdbcompat/registry.go`, and add a round-trip test in `internal/pdbcompat/fold_filter_test.go`.
For a new (7th+) entity, also create the sibling file declaring `Mixin()`.

**Do NOT:**

- Edit the generated `ent/schema/{type}.go` to add `_fold` fields — `cmd/pdb-schema-generate` strips them.
  Use the sibling.
- Use `_fold` in non-pdbcompat surfaces — entrest/entgql/grpcserver already do ent-level `FieldContainsFold` / NOCASE.
  If a new surface needs diacritic folding, route through `internal/unifold.Fold`.
- Drop `entgql.Skip` / `entrest.WithSkip` from `foldMixin` — exposing the shadow as a separate filterable surface is meaningless to callers.

### Cross-entity `__` traversal

See `docs/API.md § Cross-entity traversal` for Path A (allowlist) / Path B (ent-edge introspection), 2-hop cap, `parseFieldOp` 3-tuple, and unknown-field diagnostics.

**Non-model targets.**
`TypeConfig.NonModelFields` (serializer fields / properties upstream, e.g. fac `org_name`, campus `city`) are never a traversal target (`traversalTargetField`) nor a relation-seed tail: upstream `queryable_relations` offers model fields only.
Do NOT key this on `UpstreamIgnored`: it also holds renamed MODEL fields (carrier `fac_count`) that stay valid targets (`carrierfac?carrier__fac_count=`).

**Codegen invariants.**
Static map emission, NOT runtime `client.Schema.Tables` walk.
`cmd/pdb-compat-allowlist` reads `schema.PrepareQueryAllows` from `ent/schema/pdb_allowlists.go` → emits `internal/pdbcompat/allowlist_gen.go`.
Each entry's block comment cites the upstream `peeringdb_server/serializers.py:<line>` it derives from (usually `<Serializer>.prepare_query`, else `related_fields` / `queryable_relations`); audit-required.
There is no `// Source:` tag.
Path B introspection: `internal/pdbcompat/introspect.go` (`LookupEdge` / `ResolveEdges` / `TargetFields`).

**Adding filters:** for 1-hop / 2-hop, add the key to the relevant entry's `Fields` slice in `ent/schema/pdb_allowlists.go` with a comment citing the upstream `serializers.py:<line>`, then `go generate ./...`.
Codegen routes 3-segment keys into `AllowlistEntry.Via` automatically.
For excluded edges, attach `pdbcompat.WithFilterExcludeFromTraversal()` to the edge definition.
For a 14th entity, add the mapping in `cmd/pdb-compat-allowlist/main.go` `pdbTypeMap` (`TestPdbTypeFor_AllThirteen` will fail until extended).

**Do NOT:**

- Hand-edit `internal/pdbcompat/allowlist_gen.go` — overwritten on every codegen run; CI drift gate catches it.
- Add traversal allowlists to grpcserver / entrest / GraphQL — out of v1.16 scope; those surfaces have their own filter models.
- Invent filter keys that don't exist upstream — contract is parity with `peeringdb/peeringdb@465931c0` (PeeringDB 2.83.0; `docs/API.md § Validation Notes` pins the full SHA).
- Add 3+-hop keys — dropped by codegen AND by the 2-hop cap in `parseFieldOp` at request time.
  The cap counts key segments: a relation filter (below) has at most 2 segments but its path can reach 3 tables (`net?ix__name=` walks netixlan → ixlan → ix).
- Introduce runtime ent-client introspection or `sync.Once` lazy-init for the Edges map — map is codegen-time static, which avoids init-order coupling.

**Status-matrix and fold composition.**
Traversal predicates compose with the status matrix (`wireEntity` appends `applyStatusMatrix` LAST for all 13 types) and with the `_fold` routing (a folded traversal target uses `<field>_fold` with `unifold.Fold(value)` even when reached via `<fk>__<field>`).
Regression-guarded by `TestTraversal_StatusMatrix_Preserved`, `TestTraversal_FoldRouting_Preserved`, `TestTraversal_EmptyIn_ShortCircuits` in `internal/pdbcompat/handler_test.go`.

**Relation filters (`internal/pdbcompat/relation_filter.go`).**
The relation keys that an upstream `prepare_query` handles (fac `net`/`ix`/`org_name`, ix `ixlan`/`ixfac`/`fac`/`net`, net `ix`/`ixlan`/`netixlan`/`netfac`/`fac`, netixlan `ix`/`name` (`name__iexact`/`__icontains`/`__istartswith` filter the ixlan name), ixpfx `ix`, netfac+ixfac `name`/`country`/`city`, campus `facility`, org `asn`, carrier `carrierfac_set__facility_id`) live in `relationSeeds` and resolve in `ParseFiltersCtx` BEFORE `parseFieldOp` and Path A/B.
Most seeds pin ONE row of their path to `status='ok'` (`make_relation_filter`, 2.83.0 `models.py:221-234`; `pinAt`, `noPin` for fac `org_name` and carrier); a bare `status` filter on that row is replaced by the pin.
A tail field in `TypeConfig.NonModelFields` (serializer field or property upstream) is ignored.
They read `vals[0]` (upstream `v[0]`), not the last value.
Do not re-add these keys to `pdb_allowlists.go`: Path A never sees them.
Semantics table: `docs/API.md § Relation filters`.

**netixlan `meta__*` filters (`internal/pdbcompat/meta_filter.go`).**
`ParseFiltersCtx` resolves them via `lookupMetaFilter` BEFORE `parseFieldOp`, mirroring upstream `finalize_query_params` (2.83.0 `serializers.py:3129-3149`), so the 3-/4-segment keys never reach traversal or the 2-hop cap.
Keys come from `metaFilterColumns` (upstream `meta_registry.py:277-313`; the raw `meta_*` column names are aliases).
Predicates read the stored document with `json_extract` (`json_type` for the bool), so an absent key is NULL and never matches (`meta__rfc8950=false` excludes rows that never declared it).
Only upstream's operator set applies (`metaOperators`: lt/lte/gt/gte/contains/startswith/in); `__iexact`/`__icontains`/`__istartswith` on a meta key are ignored, as upstream does. net meta keys stay unfilterable (upstream ignores them).
When upstream registers a new filterable key, add a `metaColumn` entry and a `TestParity_Meta` sub-test.

**Multi-value choice fields (`internal/pdbcompat/multichoice_filter.go`).** net `info_types` and fac `available_voltage_services` are `FieldMultiChoice`.
Upstream stores a MultipleChoiceField as a CSV string in choice-list order (django-peeringdb `fields.py:61-71`); the mirror stores the API's JSON array (DRF set order), so every filter compares `multiChoiceExpr`, a correlated `json_each` + `group_concat(... ORDER BY CASE ...)` that rebuilds the upstream string. exact/iexact/contains/startswith use the value as given; `__in` items and comparisons convert the value first (`canonicalChoices`, upstream `get_prep_value`), as does a relation-seed key without an operator (`opCanonicalExact`).
The choice lists in `multiChoiceLists` are copied from django-peeringdb `const.py`; update them when upstream adds a choice.
Legacy net `info_type` is NOT a filter field (upstream property): `legacyInfoTypePatterns` ports `NetworkSerializer.finalize_query_params` (2.83.0 `serializers.py:3765-3813`) for `info_type`, `info_type__contains`, `info_type(s)__in` (OR of substrings, empty item matches all) and `info_type(s)__startswith`; every other `info_type` key is ignored.
`multiChoiceLikeAny` binds all patterns as ONE JSON array in a single `EXISTS` that builds the string once per row (`LIMIT 1` derived table stops SQLite flattening it); never emit one SQL term per pattern (1000+ items hit SQLite's expression-depth limit).
Cost stays rows × items, like upstream; do not cap the item count (new divergence).

**Campus traversal fix (v1.18.0).**
`<entity>?campus__<field>=X` previously returned 500 due to go-openapi/inflect mis-singularising "campus" → "campu" on the `cmd/pdb-compat-allowlist` codegen path.
Fixed by sibling-file mixin `ent/schema/campus_annotations.go` (`entsql.Annotation{Table: "campuses"}`).
The `entc.LoadGraph` runtime patch in `ent/entc.go` (`fixCampusInflection`) remains; the two are complementary, not redundant.

### Response memory envelope

See `docs/ARCHITECTURE.md § Response Memory Envelope` for budget, sizing table, lifecycle, telemetry.
Invariants:

**Closure pairing:** the 13 List/Count pairs in `internal/pdbcompat/registry_funcs.go` are built by one generic `wireEntity` helper from a single shared predicate builder (v1.23.0), so budget pre-check and served response cannot disagree (the 413 guarantee).
`applyStatusMatrix` LAST and the `opts.EmptyResult` short-circuit both live in exactly one place inside `wireEntity` — do not add per-entity closures outside it.

**Single-call-site telemetry:** `memStatsHeapInuseBytes` in `internal/pdbcompat/telemetry.go` is the ONLY call site for `runtime.ReadMemStats`; `recordResponseHeapDelta` fires once per request via `defer` in `dispatch` (covers list + detail terminal paths).

**Detail-path admission:** depth≥2 details charge the shared `inflightBytes` pool with a count-based fan-out estimate (child `COUNT(*)` × child `Depth0` per embedded `_set`, table in `internal/pdbcompat/detail_budget.go` mirroring the `get<Type>WithDepth` eager-loads).
Changing a depth expansion's set list means updating `detailChildSets` too.
The 413 check stays flat (`CheckBudget(1, type, depth, …)`) — fan-out feeds only the pool.

**Adding an entity type:** add a `typicalRowBytes` entry to `internal/pdbcompat/rowsize.go` (bench via `BenchmarkRowSize`, double the mean, round to 64 bytes), add a `wireEntity(entityWiring[...]{...})` entry in `registry_funcs.go` `init()`, extend the sizing table in `docs/ARCHITECTURE.md`, add under-/over-budget E2E cases mirroring `TestServeList_UnderBudgetStreams` / `TestServeList_OverBudget413`.

**Do NOT:**

- Call `runtime.ReadMemStats` per row — STW cost is µs but compounds quickly; the single-call-site `memStatsHeapInuseBytes` invariant is grep-enforceable.
- Skip the `CheckBudget` pre-flight for "trusted" entity types — none are trusted; the 256 MB replica cap is symmetric across all 13 types.
- Add a per-endpoint budget override — a single global budget keeps the operator mental model (and the Grafana panel legend) manageable.
- Bypass `wireEntity` with hand-written List/Count closures — the generic helper exists so the budget check and the served response can never become different queries (413 guarantee).
- Extend streaming/budget to grpcserver / entrest / GraphQL / Web UI — those surfaces have their own memory stories (see `docs/ARCHITECTURE.md § Response Memory Envelope` → Out of scope).

### Upstream parity regression

`internal/pdbcompat/parity/` locks pdbcompat semantics via 9 category-split test files (`{ordering,status,limit,unicode,in,traversal,meta,serializer,multichoice}_test.go`) + `harness_helpers_test.go`.
Each test seeds its own clean rows **inline** via the ent client and cites the upstream source line in a comment.
The earlier ported-fixture pipeline (`internal/testutil/parity` + `cmd/pdb-fixture-port`) was removed: the ports carried unseedable Python-source artefacts (`**kwargs` splats, `SHARED[...]` refs) and 5 of 6 slices had zero behavioural consumers, while the `--check` drift gate was wired into nothing.

**Adding a parity test:** pick the category file matching the behaviour under test, add a sub-test under `TestParity_<Category>` with `t.Parallel()` and a citation comment (`// upstream: pdb_api_test.py:<line>` or `// synthesised: <context>`).
Seed clean rows inline via `c.<Entity>.Create()` and the shared `harness_helpers_test.go` request/decode helpers (`newTestServer`, `httpGet`, `decodeDataArray`, `extractIDs`, `mustDecodeProblem`) — do NOT reach into `internal/testutil/seed.Full` (cross-test contamination).

**Divergence registry:** `docs/API.md § Known Divergences` is the SoT for intentional non-parity.
Every entry has a matching `DIVERGENCE_<…>` sub-test.
To add one: (1) write the parity test with `DIVERGENCE_` prefix, (2) append a Known Divergences row, (3) if it corrects a pdbfe-style claim, also append a Validation Notes row.

Bench envelopes in `bench_test.go` run locally — no CI benchstat gate.

### Middleware

- Response writer wrappers MUST implement `http.Flusher` (delegate to underlying writer) — gRPC streaming requires it.
- Add `Unwrap() http.ResponseWriter` for middleware-aware interface detection.
- Full chain (outermost first): `Recovery -> MaxBytesBody -> CORS -> OTel HTTP -> Recovery -> Logging -> PrivacyTier -> Readiness -> SecurityHeaders -> CSP -> Caching -> Gzip -> RouteTag -> mux`
- Recovery wraps twice: the inner one MUST stay inside OTel HTTP and RouteTag MUST tag in a defer (otelhttp records the request metric only when its inner handler returns, so otherwise a recovered panic leaves no 500 sample and the 5xx error rate misses it); the outer one keeps the 500 for a panic in CORS/MaxBytesBody/otelhttp.
  A panic after the response started records the status already sent, and a panic before mux dispatch has no route.
  Locked by `TestRecoveredPanicRecordsRequestMetric` + `TestMiddlewareChain_Order`.
- **ETag (v1.28.3):** `Caching` sends a weak ETag of the local database version.
  `startETagWatcher` (`cmd/peeringdb-plus/etag.go`) runs on every node and polls once a second.
  It reads the LiteFS `<db>-pos` file, or `PRAGMA data_version` on a pinned connection when LiteFS is absent.
  A node sends no caching headers until it has seen a successful sync, and a read error clears the ETag.
  Each committed write changes the ETag, so one sync cycle can change it more than once.
  Do not set the ETag from the sync worker: replicas never run it.
  Before v1.28.3, replicas kept the ETag from process start and answered 304 with old bodies.

### ConnectRPC / gRPC

- `cmd/peeringdb-plus/main.go` registers each of the 13 services with one `registerService` call; each gets the otelconnect interceptor with `connectOTelOpts` (spans only, no `rpc.server.*` metrics).
- Never return an ent error inside `connect.CodeInternal`.
  Use `queryError` (`internal/grpcserver/errors.go`): it sends "internal error", records the cause on the span, and maps context cancel/deadline to `CANCELED`/`DEADLINE_EXCEEDED`.
- Handler implementations in `internal/grpcserver/` — one file per entity type.
- `gen/peeringdb/v1/peeringdbv1connect/` contains generated handler interfaces.
- Proto `optional` fields generate pointer types (`*int64`, `*string`) — check `!= nil` for presence.

### Environment Variables

Authoritative table: `docs/CONFIGURATION.md`.
Standard `OTEL_*` env vars also apply (autoexport).

Operationally-critical defaults worth retaining in-context (the surprising or load-bearing ones):

- `PDBPLUS_SYNC_MODE=incremental` (default flipped 2026-04-26 after the incremental-sync evaluation; `full` is operator escape-hatch for first-sync / recovery)
- `PDBPLUS_SYNC_INTERVAL` defaults `1h` unauthenticated / `15m` when `PDBPLUS_PEERINGDB_API_KEY` is set (auth-conditional)
- `PDBPLUS_FULL_SYNC_INTERVAL=24h`: the forced full cycle is the ONLY automatic repair for what incremental sync cannot see: count fields, netfac/ixfac name/city/country + carrierfac name (upstream copies them from the fac without bumping `updated`), live rows skipped by `updated` ties under offset paging (0160 gives ~600 netixlans one `updated`), values set upstream before our column existed (`meta`), orphan-nulled FKs.
  `0` leaves them stale until `POST /sync?mode=full`.
  Upstream `?since` is strict `updated > N`; it only looks inclusive because the wire `updated` is truncated to the second.
  See `docs/ARCHITECTURE.md § Daily full reconcile`.
- `PDBPLUS_RESPONSE_MEMORY_LIMIT=128MiB` — pdbcompat list pre-flight 413 budget; mandatory unit suffix (`KB`/`MB`/`GB`/`TB`); bare numbers rejected except literal `0` (disabled — dev only)
- `PDBPLUS_SYNC_MEMORY_LIMIT=400MB` — sync-cycle peak heap ceiling; unit suffix required; `0` disables
- `PDBPLUS_HEAP_WARN_MIB=400`, `PDBPLUS_RSS_WARN_MIB=384` — sync-cycle telemetry warn thresholds (Fly 512 MB cap; sustained breach re-opens the incremental-sync evaluation)
- `PDBPLUS_PEERINGDB_RPS=0.333` (1/3, 20 req/min, the documented anonymous limit; authenticated ignores it and runs 30 req/min): sustained req/sec cap on every upstream PeeringDB call (sync, FK backfill and netixlan cascade verification share the budget); burst hardcoded at 1 to forbid concurrency; 429s honored via `Retry-After` (3-retry cap)
- `PDBPLUS_FK_BACKFILL_MAX_REQUESTS_PER_CYCLE=20`: per-cycle cap on **HTTP requests** issued by FK backfill (renamed v1.18.5 from MAX_PER_CYCLE which counted rows; rows is the wrong unit once `?id__in=` batches collapse N rows into 1 request).
  At 30 req/min auth, 20 ≈ 40s of upstream pressure max per cycle.
  With `FetchByIDsBatchSize=100`, that covers up to 2,000 missing-parent rows.
  `0` disables backfill (orphans fall back to drop-on-miss); also caps netixlan cascade verification (min(10, cap) per pass, counted apart from backfill; `0` turns it off, class A still runs)
- `PDBPLUS_HISTORY_MAX_REQUESTS_PER_CYCLE=15`: per-cycle cap on history sweep windows (incremental cycles only; `0` = off; `POST /sync?mode=history` restarts a finished sweep)
- `PDBPLUS_FK_BACKFILL_TIMEOUT=5m` — wall-clock budget for backfill HTTP activity per sync cycle; bounds tx-hold time (backfill happens inside the sync tx).
  On deadline → drop-on-miss with `result=deadline_exceeded` metric
- `PDBPLUS_LOG_LEVEL=INFO` — minimum severity for the OTel logging branch (Loki).
  Stdout handler stays at INFO independently.
  Set `DEBUG` for opt-in deep debugging; invalid values fall back to INFO without crashing.
- `PDBPLUS_CSP_ENFORCE=false`: defaults to report-only; set `true` after browser verification of the current CSP
- `PDBPLUS_PUBLIC_TIER=public` — set `users` only for private deployments (WARN at startup)
- `PDBPLUS_IS_PRIMARY=true` — fallback primary detection when LiteFS not present
- `PDBPLUS_SCRATCH_DIR` (empty = `os.TempDir()`, absolute path required): `fly.toml` sets `/var/lib/litefs/scratch`, on the primary volume (the rootfs is capped at 2000 IOPS and 8 MiB/s).
  That is the LiteFS data dir, but LiteFS 0.5 uses only `dbs/`, `clusterid` and `id` there.
  When set, the primary removes stale `pdbplus-sync-scratch-*` files once per process: at scheduler start, or in the first cycle when the start sweep did not run (`scratchSwept`, `sweepScratchDir` via `sweepScratchDirAtStartup` or `scratchDirForCycle`, holds the `running` latch and the exclusive flock on `<dir>/.pdbplus-scratch.lock`, then converts it to LOCK_SH and keeps it).
  Each cycle takes LOCK_SH first (`scratchDirForCycle`), and stages in `os.TempDir()` with a WARN when it cannot.
  When the held fd is no longer the file at the lock path (dir or file removed or replaced), the lock call opens the path again and recreates the dir.
  The lock file must be a regular file (opened `O_NONBLOCK`, so a FIFO cannot block the latch-holding sweep).
  Then the free-space guard: statfs `Bavail*Frsize` (`Bsize` when `Frsize` <= 0, via `Worker.scratchFreeBytes`, default `dirFreeBytes`) below `scratchDirMinFreeBytes` (512 MiB), or a statfs error, also stages in `os.TempDir()` with a WARN, so the scratch file (about 100 MB on a full cycle) does not starve LiteFS on the shared volume.
  On the 974 MB volume the guard trips at about 45% used, below the 80% auto-extend, so a repeating WARN (or alert `PdbPlusPrimaryVolumeLow`, gauge `pdbplus_scratch_free_bytes`) means `fly volumes extend`.
  Span attr `pdbplus.sync.scratch_dir` = the dir used.
  Another holder: sweep skipped (WARN), then a LOCK_SH try that fails against LOCK_EX.
  Lock and statfs code is Linux-only (`scratch_dir_linux.go`): other OSes never sweep and skip the free-space guard.
  Never sweep `os.TempDir()` (empty setting), and never set the dir to a shared temp dir such as `/tmp`: a process that stages in `os.TempDir()` takes no lock.

### Testing

- Live tests (against `beta.peeringdb.com`): gated by `-peeringdb-live` flag, not run in CI.
- Test helpers: `internal/testutil/` — `SetupClient(t)` creates isolated in-memory SQLite client with `t.Cleanup`.
- Seed data: `internal/testutil/seed/` — `Full(tb, client)` seeds all 13 entity types with deterministic IDs.
- Fixtures: `testdata/fixtures/` — 13 JSON files matching PeeringDB API types, used by sync integration tests.

### Build

- Pure Go: `CGO_ENABLED=0` in Docker (modernc.org/sqlite is CGo-free); CI flips it to `1` only for the race detector.
- Chainguard base images: `cgr.dev/chainguard/go` (build), `cgr.dev/chainguard/static` (standalone `Dockerfile` runtime), `cgr.dev/chainguard/glibc-dynamic:latest-dev` (`Dockerfile.litefs` runtime); flags: `-trimpath -ldflags="-s -w"`, version via `-X …buildinfo.injected`.
- LiteFS is a separate FUSE process: the app does not link to it.
  `Dockerfile.litefs` (the Fly image, `fly.toml` `[build]`) uses `litefs mount` as entrypoint.

### LiteFS

- Lease file semantics are **inverted**: `/litefs/.primary` file **absent** = primary, **present** = replica (file contains primary hostname).
- Detection fallback (`internal/litefs/primary.go`): (1) check `.primary` file, (2) check if `/litefs/` dir exists: primary only when `IsCandidate(FLY_REGION, PRIMARY_REGION)` (mirrors `litefs.yml` `candidate`; LiteFS also hides `.primary` on a replica while no node holds the lease, so before this a replica acted as primary for a few seconds on each deploy), (3) fall back to `PDBPLUS_IS_PRIMARY` env var (default true for local dev).
- App serves directly on `:8080` with h2c — does NOT use LiteFS proxy (needed for gRPC/ConnectRPC support).
- Metrics: Fly.io scrapes LiteFS `:20202/metrics` on every machine (`fly.toml` `[[metrics]]`, 15s; the section needs `processes = ["primary", "replica"]`: without it flyctl sets the metrics config of the default group only (here primary), as in v1.35.0) into the Fly.io managed Prometheus (Grafana data source `fly.io`), as native `litefs_*` with labels `app`/`instance`/`region`/`host` only: split primary/replica with `litefs_is_primary` joined `on (instance)`.
  The app scraper (`PDBPLUS_LITEFS_METRICS_URL`, `InitLiteFSGauges`, `pdbplus.litefs.*`) was removed in v1.35.0.
  LiteFS 0.5 quirks: `litefs_http_frame_send_count` never increments (missing `.Inc()`, `http/server.go:735/774`); `litefs_db_ltx_bytes` = new LTX file size after a commit, on-disk total after the 1m retention pass; `commit_count`, `ltx_bytes`, `ltx_count` and db `lag_seconds` appear only after the first commit/apply/retention pass since LiteFS started (`commit_count` only after the node commits as primary: only CommitWAL/CommitJournal/Drop set it); a restarted primary reports the age of its last LTX file as db `lag_seconds`, a replica the size of its snapshot as `ltx_bytes`, so the dashboard filters by role.

### CI

- Triggers: PR, push to main, push of `v*` tags.
  3 jobs:
  - `ci` (PR + main push, `if: github.ref_type != 'tag'`, never on a `v*` tag push): one cached mise/Go job running, in order: generated-code drift check, build, gotestsum race tests, lint, advisory vulnerability scan.
  - `docker-build` (PR events only, parallel, pushes nothing): `Dockerfile` for amd64+arm64 (gha cache `scope=dev`), `Dockerfile.litefs` for amd64 (`scope=prod`).
  - `docker-publish` (push events only, `needs: [ci]`, the only Docker job on a push): pushes `Dockerfile` (amd64+arm64) to `ghcr.io/dotwaffle/peeringdb-plus` with SBOM + `provenance: mode=max`, then `actions/attest` (pushed to the registry).
    Tags: `X.Y.Z`/`X.Y`/`latest` on `v*` tags, `main` + `sha-<short>` on main (`type=sha,enable={{is_default_branch}}`: until v1.32.0 the tag run also pushed `sha-<short>`, and the later of the two pushes won).
    Writes the main `dev` cache (PR builds read it). A tag run reads it and writes none (`cache-to` is empty on tags): a cache saved on a tag ref can be restored only by that tag.
    Before v1.32.0 a push also ran `docker-build`, and publish recompiled anyway: its full-history checkout (for the version stamp) changes the `COPY . .` context (`.git`) and the stamped version.
    `Dockerfile.litefs` is never published (needs the Fly LiteFS lease).
    Job `if` = `!cancelled()` + push + (`ci` success, or tag AND `ci` skipped): a skipped need skips the job unless the `if` calls a status function.
    Tag gate (first step, tag runs only, needs `actions: read`): passes when a `ci.yml` run for a push to main (`gh run list --commit $SHA --branch main --event push`) has a successful job named `CI` (a run that failed only in another job still counts).
    Fails at once when every such run completed without it.
    Polls every 30s while no run exists yet (main + tag pushed together) or `CI` is pending, and gives up after 30 min.
    So tag a commit that a main push tested (e.g. the tip of a push): a commit from the middle of a multi-commit push fails publish.
    Renaming the `CI` job or `ci.yml` breaks the gate.
    The tag run still builds its own image: `go build` stamps the tag as the version, and the main image can carry a pseudo-version.
- **Generated code drift check**: first step of the `ci` job — runs `mise run generate` then fails if `ent/`, `gen/`, `graph/`, `internal/web/templates/`, Tailwind output, or the pdbcompat allowlist differ from committed files.
- govulncheck is advisory (`continue-on-error`): a flagged vuln warns but does not block merge.
- Linters: contextcheck, exhaustive, gocritic, gosec, misspell, modernize, nolintlint, revive (see `.golangci.yml`).

### Go Module

- `GONOSUMCHECK=* GONOSUMDB=*` may be needed for `go mod tidy` / `go get` when sumdb is read-only.
- `TMPDIR=/tmp/claude-1000` required for go commands in sandbox mode.

### Shell environment

- The Bash tool runs under `zsh`, which performs history expansion on `!` even in non-interactive scripts.
  Avoid `if ! cmd; then` and `! cmd1 | cmd2` — they silently drop the negation.
  Use count-based equivalents instead: `test "$(cmd | grep -c X)" -eq 0`.

### Deployment

- `fly deploy` from project root.
  App: peeringdb-plus.
  Primary region: lhr.
- LiteFS FUSE mount takes a moment — "not listening" warnings during rolling deploy are normal.
- **Asymmetric fleet (v1.15+).**
  Fly app `peeringdb-plus` runs two process groups: `primary` (1 machine, LHR, `shared-cpu-2x`/512 MB, persistent `litefs_data` volume) and `replica` (7 machines in other regions, `shared-cpu-1x`/256 MB, ephemeral rootfs).
  Replicas cold-sync the DB (126 MB on 2026-09-24) from primary over LiteFS HTTP on boot (5-45s per region).
  LiteFS starts the app only after it applies the snapshot, and later LTX files stream in the background.
  Fly caps the ephemeral rootfs at 2000 IOPS and 8 MiB/s.
  `/readyz` fail-closes during hydration so Fly Proxy excludes them until ready.
  Replica recovery = destroy-and-recreate (no volume management).
  See `docs/DEPLOYMENT.md` § Asymmetric fleet.
- **Volume-only-on-primary.**
  `[[mounts]]` in `fly.toml` is scoped to `processes = ["primary"]`.
  Never re-introduce a mount on the replica group — the architecture assumes replicas are cattle.

### Sync observability

End-of-sync-cycle memory telemetry surfaces the sustained-high-heap trigger that re-opens the incremental-sync evaluation.
Implementation: `internal/sync/worker.go` `emitMemoryTelemetry`, called from `recordSuccess` and `recordFailure`; `rollbackAndRecord` reaches it through `recordFailure`, so each cycle emits once (do not add a third call).
Span attrs `pdbplus.sync.peak_heap_bytes` + `pdbplus.sync.peak_rss_bytes` are mirrored as Prom gauges `pdbplus_sync_peak_heap_bytes` / `pdbplus_sync_peak_rss_bytes` (bytes is the canonical Prom unit; dashboards format MiB at render).
Zero-valued observations are suppressed.

**Log signal:** when a threshold is breached, worker emits `slog.Warn("heap threshold crossed", peak_heap_bytes, heap_warn_bytes, peak_rss_bytes, rss_warn_bytes, heap_over, rss_over)`.
Thresholds gated by `PDBPLUS_HEAP_WARN_MIB` / `PDBPLUS_RSS_WARN_MIB` (defaults sit under the Fly 512 MB VM cap so order under pressure is: log → app crash → Fly OOM-kill).

**Lock-error retry (`internal/sync/lockretry.go`):** `retryOnLock` reruns the short primary writes when the error holds a `*sqlite.Error` with `code & 0xff` = 5 (`SQLITE_BUSY`, incl.
`BUSY_SNAPSHOT` 517) or 15 (`SQLITE_PROTOCOL`, the prod LiteFS commit failure): `sync_status` INSERT/UPDATE (`Worker.recordSyncStart` / `recordSyncComplete`), `ReapStaleRunningRows`, the startup poc scrub tx, the startup netixlan cascade tx.
Never the main sync tx or the sync_status prune.
`SQLITE_LOCKED` (6) is excluded: same-connection or shared-cache conflicts only, and prod has no shared cache.
Policy `LockRetry` (default 4 attempts, 250ms doubling) is passed as a value (`Worker.lockRetry`, `ReapStaleRunningRows(ctx, db, logger, retry)`).
A done ctx stops the wait.
Per retry: WARN `retrying write after sqlite lock error` {op, attempt, max_attempts, delay, error} + counter `pdbplus.sync.lock_retries{op}`.
The caller's final-failure log is unchanged.
A retry is safe because a failed attempt leaves no open tx on its pooled connection: callers roll back after a statement error, and on a failed COMMIT modernc `tx.Commit` checks `sqlite3_get_autocommit` and issues ROLLBACK. database/sql marks the Tx done before the driver Commit, so `tx.Rollback()` after a failed `Commit()` is a no-op (`sql.ErrTxDone`).
Do not add one.
Locked by `lockretry_test.go` (incl.
`TestScrubPocContactsAtStartup_RetriesRealLock`: real BUSY at the UPDATE (WAL) and at the COMMIT (rollback journal) on a 1-connection pool).

**sync_status retention (`internal/sync/status.go`):** `pruneSyncStatus` deletes rows older than the newest 3000 (`syncStatusKeepRows`, about 31 days at 15m), oldest first, at most 1000 per call (`syncStatusPruneBatch`).
The batch keeps the LTX files of the first drain small.
The LIMIT sits in the IN-subquery: the SQLite build has no `DELETE ... LIMIT`.
Two anchors are never deleted: the newest `success` row (`GetLastSuccessfulSyncTime` / `GetLastSuccessfulStatus`: synced latch, warm start, ETag `seenSync`, freshness) and the newest `success` + `mode='full'` row (`GetLastSuccessfulFullSyncTime`: full-sync interval).
Anchors use `IS NOT`, so a missing (NULL) anchor does not stop the prune.
With `!=` it would delete nothing.
`Worker.pruneStatusRows` runs in `Worker.Sync` right after a successful `recordSyncStart` INSERT and before `syncCycle`.
The window counts the new row, so the `GetLastStatus` / `GetLastCompletedStatus` rows stay.
It runs inside the running latch and the panic firewall, on the cycle ctx, with one attempt and no `retryOnLock` (the next cycle retries).
A failure logs WARN `failed to prune sync_status rows` and never fails the cycle.
A success sets span attr `pdbplus.sync.status_rows_pruned` and logs DEBUG `pruned sync_status rows` {deleted} when > 0.
Rows, not days: the DSN sets no `_time_format`, so modernc stores a `time.Time` as `t.String()`, which SQLite date functions cannot parse.
Never compare `sync_status` times in SQL.
`auto_vacuum` is off, so freed pages go to the freelist and the file does not shrink.
Locked by `TestPruneSyncStatus` (incl. a failed full cycle after the full success), `TestPruneSyncStatus_Batch`, `TestSync_PrunesSyncStatus`, `TestSync_PruneFailureDoesNotFailCycle`, `TestSync_SkipsPruneWhenStartFails`.

**Per-type objects counter:** `pdbplus.sync.type.objects{type}` comes from `recordObjectCounts` in `recordSuccess`, after the commit.
A cycle that rolls back adds nothing.
Locked by `TestSync_ObjectsCounterAfterCommit`.

**FK-orphan summary:** each sync cycle emits one `slog.Warn("fk orphans summary", total, groups)` (DEBUG when `total=0`) and increments `pdbplus.sync.type.orphans{type, parent_type, field, action}` per row.
Per-row events log at DEBUG only — replaces the prior per-row WARN spam that breached Tempo's 7.5 MB per-trace cap.

**Escalation:** sustained peak heap above `PDBPLUS_HEAP_WARN_MIB` across multiple cycles re-opens the incremental-sync evaluation.

**Resource attribute filtering** (`internal/otel/provider.go` `buildResourceFiltered`): Grafana Cloud's hosted OTLP receiver only promotes `service.*` / `cloud.*` / `host.*` / `k8s.*` to Prom labels; custom `fly.*` keys are dropped on the metrics path.
`service.instance.id` and `service.version` are stripped from the metric resource (`forMetrics`) to bound fleet cardinality (with the version label, each deploy started a new copy of every series); the version is on the `pdbplus_build_info` gauge instead (`count by (service_version) (pdbplus_build_info{service_name="peeringdb-plus"})`; a restarted machine shows both versions for up to 5 min (OTLP has no staleness markers), so a per-version join must use `topk by (service_namespace, cloud_region) (1, timestamp(...))`, see `docs/ARCHITECTURE.md`).
`service.namespace` + `cloud.region` stay on metrics.
Full attribute table + `http.route` middleware rationale in `docs/ARCHITECTURE.md`.

**Dashboards + alerts** in `deploy/grafana/{dashboards,alerts}/` — see `docs/DEPLOYMENT.md`.
OTel runtime metrics (`go_memory_used_bytes` etc.) come from `runtime.Start(...)` and tick on every machine; `pdbplus_sync_peak_*` is primary-only.

**Prod debugging:** agents never open the live `/litefs/peeringdb-plus.db` with `sqlite3` (user decision, 2026-09-24): a read transaction on a replica makes the LTX apply wait, and on the primary it blocks WAL checkpoints.
Use the read APIs (`/api/`, `/rest/v1/`, GraphQL) and Grafana first.
For SQL, query a copy: `fly ssh console -a peeringdb-plus --process-group primary -C "sh -c 'litefs export -name peeringdb-plus.db /tmp/pdb.db && sqlite3 -readonly /tmp/pdb.db \"<SQL>\"; rm -f /tmp/pdb.db'"`.
`litefs export` (LiteFS 0.5) holds a LiteFS read lock only while it copies the database (126 MB).
The image ships `sh` and `sqlite3`.
A write to the prod DB needs the user's approval of the exact SQL.

## Architecture

### API Surfaces (6)

- **Web UI**: `/ui/` — templ + htmx + Tailwind CSS (search, detail pages, ASN comparison).
  Content-negotiates: browsers get HTML, plain User-Agents (curl, wget, scripts) get ANSI-styled terminal text via `internal/web/termrender`.
  To smoke-test with curl, either send `-H 'User-Agent: Mozilla/5.0'` or strip ANSI: `sed 's/\x1b\[[0-9;]*[mGKH]//g'`.
- **GraphQL**: `/graphql` — gqlgen via entgql, interactive playground
- **REST**: `/rest/v1/` — entrest, OpenAPI-compliant
- **PeeringDB Compat**: `/api/` — drop-in replacement for PeeringDB API
- **ConnectRPC**: `/peeringdb.v1.*/`: Get, List and Stream RPCs for all 13 types with typed filtering, reflection, health check
- **MCP**: `/mcp` (`internal/mcpserver`): read-only tools over `internal/catalog` DTOs, plus `lookup_ip` (raw ent rows); `internal/agentdocs` serves the skill, `/.well-known/` files and `llms.txt`

### Key Packages

Most `cmd/*` and `internal/*` paths are self-describing; only the non-obvious ones are listed here.

Codegen tools (run by `go generate ./...`):

- `cmd/pdb-schema-generate/` — generate ent schemas from PeeringDB JSON
- `cmd/pdb-compat-allowlist/` — emits `internal/pdbcompat/allowlist_gen.go` from `ent/schema/pdb_allowlists.go`

Manual tools (not run by `go generate` or CI):

- `cmd/pdb-schema-extract/`: parses an upstream PeeringDB checkout (`<repo>/src`) and prints the extracted schema as JSON on stdout.
  Diff it against the hand-curated `schema/peeringdb.json` to find drift; it never writes that file.
- `cmd/pdbcompat-check/`: subcommands `check` (compares the structure of live upstream responses with local golden files) and `capture` / `redact` / `diff` (build the visibility baseline).

Operator tooling (NOT shipped in prod images, NOT invoked by CI):

- `cmd/loadtest/` — read-only HTTP traffic generator with 4 modes: `endpoints` (sweep), `sync` (replay 13-step ordered GET sequence), `soak` (sustained QPS-capped mixed load), `ramp` (per-surface inflection-point capacity probe → markdown table to stdout, added v1.18.7).
  Default `--target=https://peeringdb-plus.fly.dev`; **never** point at upstream `https://www.peeringdb.com` (1 req/hour/IP cap will block).
  See `cmd/loadtest/README.md` and `docs/DEPLOYMENT.md § Capacity probing`.

Single-source-of-truth packages:

- `internal/pdbtypes/` — leaf package (imports nothing) naming the 13 PeeringDB types across three domains (`Name`/`GoName`/`DjangoModel`); consumers derive their lists/maps from `pdbtypes.All`.
  `LiveStatuses(name)` mirrors upstream `live_statuses()` (feeds the pdbcompat status matrix).
  Exception: `internal/sync` keeps its own ordered step list — loadtest's ordering parity test cross-checks the two.
- `internal/pdbcompat/` — PeeringDB-compatible `/api` layer (filter routing, allowlist, status matrix, response budget)
- `internal/privfield/` `Redact(ctx, visible, value)`: field-level redaction across all 6 surfaces
- `internal/privctx/` `TierFrom(ctx)` — privacy tier reader
- `internal/unifold/` `Fold(s string) string` — diacritic-insensitive folding (mirrors upstream `unidecode`)
- `internal/visbaseline/` — visibility baseline + schema-alignment regression test
- `internal/litefs/` — primary/replica detection (inverted lease semantics)
- `internal/otel/` — dual logger, autoexport, resource-attr filtering, runtime metrics
- `ent/schema/` — entgo schemas; generated `{type}.go` files + hand-edited `{type}_{method}.go` siblings
- `gen/peeringdb/v1/` — generated proto Go types + ConnectRPC interfaces
- `proto/peeringdb/v1/` — proto source files
