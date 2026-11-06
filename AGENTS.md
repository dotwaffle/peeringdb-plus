# Agent guide

PeeringDB Plus is a read-only PeeringDB mirror built with Go, ent, SQLite, LiteFS, and OpenTelemetry.
It serves `/api/`, REST, GraphQL, ConnectRPC, MCP, and the Web UI from the same stored data.
Keep ent as the ORM and preserve upstream data fidelity.

## Boundaries

- Obtain explicit per-batch authorization before pushing, deploying, changing billable cloud resources, or creating credentials.
  Local commits and merges are permitted.
- Obtain approval before creating GitHub issues, comments, or pull requests.
- Never start a local application server against real PeeringDB.
  Use fixture servers for development and browser checks.
  Never point `cmd/loadtest` at upstream PeeringDB.
- A forced production sync needs authorization and must not run more than once per hour.
- Never open the live LiteFS database with `sqlite3`, including for reads.
  Use APIs and Grafana first, then an exported copy if SQL is necessary.
  Production writes need approval of the exact SQL and affected counts.
- Never print or commit credentials, password hashes, or authenticated connection URLs.
- Keep scope limited to the request.
  Deferred work and historical incidents are not authorization to resume that work.

## Sources and workflow

Read the relevant document before changing a contract:

| Area | Source |
|---|---|
| Design and invariants | [Architecture](docs/ARCHITECTURE.md) |
| API behavior and accepted differences | [API reference](docs/API.md), especially [Known Divergences](docs/API.md#known-divergences) |
| Generation and change procedures | [Development](docs/DEVELOPMENT.md) |
| Test commands, fixtures, and parity conventions | [Testing](docs/TESTING.md) |
| Environment settings | [Configuration](docs/CONFIGURATION.md) |
| Production procedures | [Deployment](docs/DEPLOYMENT.md) |
| Patch conventions and validation | [Contributing](CONTRIBUTING.md) |

- Treat `go.mod`, `mise.toml`, and `mise.lock` as dependency-version sources.
  Do not copy version inventories into this guide.
  Keep floating pins floating where practical.
  Keep SQLite and libc matched to the exact versions required by the SQLite module.
- Use `mise install --locked`, then the applicable [build commands](docs/DEVELOPMENT.md#build-commands).
  `mise run check` is the full local gate.
  Regenerate affected outputs with `mise run generate` and include them with their sources.
- Before non-trivial code changes, state the approach, contract impact, failure modes, test plan, and observability signals.
  Ask before changing a contentious API or schema contract.
- For an audit or design question, read existing decisions and relevant tests before proposing another design.
- Use CodeGraph first when the repository has a `.codegraph/` index.
  Otherwise use targeted source searches.
- Stage explicit paths only.
  Follow the Linux-kernel commit style in [Contributing](CONTRIBUTING.md#commit-message-style).
- Use American English and plain technical prose.
  Do not use em dashes in files or commit messages.

Keep task state outside the repository in `~/.agents/handoffs/peeringdb-plus.md`.
Update it at task transitions, with active grants, runs, worktrees, and scratch paths.
Use the persistent-handoff skill when available.
Put scratch under a project/task directory, never directly in `/tmp`.
Delete scratch after completion and keep necessary evidence under `~/.cache/agents/peeringdb-plus/`.
Do not put task logs or deployment snapshots into this guide.

## Generated code

Follow the [generation pipeline](docs/DEVELOPMENT.md#code-generation-pipeline) and [sibling-file convention](docs/DEVELOPMENT.md#sibling-file-convention-load-bearing).

- `schema/peeringdb.json` is the curated schema source.
  `cmd/pdb-schema-extract` reports upstream drift.
  It does not replace that file.
- Never hand-edit generated `ent/schema/{type}.go` or `internal/pdbcompat/allowlist_gen.go`.
  Put manual schema methods and mixins in sibling files.
- Preserve producer-before-consumer generation order and both campus inflection fixes.
  Generated output must converge in one pass.
- Proto files under `proto/peeringdb/v1/` are authoritative, hand-maintained contracts.
  The configured entproto extension currently emits no files.
  An ent field does not automatically become a protobuf field.
- Follow the [proto workflow](docs/DEVELOPMENT.md#proto--buf-workflow) for messages, request filters, converters, and generation.
  Never reuse or renumber protobuf fields.
- Application handlers use `connectrpc.com/connect/v2`.
  The indirect v1 module does not authorize v1 imports in application code.
- Run `TestResponseFieldContract` when changing stored fields, response messages, or converters.
  Add a response mapping or a reasoned exclusion for each new stored field.
  Keep legacy response fields absent when their storage fields no longer exist.
- Follow the [schema drop procedure](docs/DEVELOPMENT.md#schema-hygiene-drop-procedure).
  Do not use the database pool before startup `Schema.Create` finishes.

GraphQL playground dependencies have separate CDN and embedded-worker graphs.
Follow [GraphQL playground assets](docs/DEVELOPMENT.md#graphql-playground-assets) when changing them.
Update import-map integrity, stylesheet integrity, worker modules, and license manifests together.
Verify normal browser behavior, corrupted-module rejection, and denied-worker behavior against fixtures.

## Privacy

Read [Privacy layer](docs/ARCHITECTURE.md#privacy-layer) and the [new gated-field checklist](docs/DEVELOPMENT.md#adding-a-new-field-level-privacy-gated-field).

- Use `privctx.Tier.AdmittedVisibilities()` for tier membership and `privctx.TierFrom(ctx)` for request context.
  Do not duplicate tier rules.
  No tier admits `Private` contacts.
- The POC ent policy handles row visibility, including its legacy NULL-as-Public rule.
  SQL subqueries can bypass that policy.
  Any filter, sort, traversal, or count over contacts must apply visibility or remain unavailable.
- Use [`privfield.Redact`](internal/privfield/privfield.go) for field visibility in every serializer that exposes the field.
  Unknown, empty, and NULL field visibility fails closed.
  Preserve the public `_visible` companion field.
- Audit all six APIs, including nested REST objects, GraphQL resolvers, Connect converters, and raw ent results from MCP.
  Keep REST field redaction inside REST error handling.
  A hidden field is omitted on `/api/` and REST, null on GraphQL, and absent on ConnectRPC.
- Preserve admitted NULL versus empty values where the wire contract distinguishes them.
  Seed public and gated rows and extend `cmd/peeringdb-plus/field_privacy_e2e_test.go` for new gated fields.
- Preserve [`Poc.BlankDeletedContact`](internal/peeringdb/types.go) on storage and compatibility serialization.
  Deleted contact data must not survive through projection or another API.

## Sync and tombstones

Read [Incremental cursor](docs/ARCHITECTURE.md#incremental-cursor), [FK backfill](docs/ARCHITECTURE.md#fk-backfill), and [Soft-delete tombstones](docs/ARCHITECTURE.md#soft-delete-tombstones).

- Do not infer deletion from absence in a partial or cached list.
  Preserve the explicit deleted-network/netixlan cascade rules and the contact purge exception.
- Keep watermarks transactional with synced rows.
  Backfilled rows must not advance a type's cursor past unfetched changes.
  Record newly landed backfill IDs in `fkBackfilled`.
- Cursor read errors fail before upstream requests.
  Never replace an erroneous cursor with zero or `MAX(updated)`.
- Full snapshots still need tombstone windows and stale-snapshot protection.
  Use the snapshot's newest row and cursor, never `meta.generated`, to select the window.
- Keep required and optional FK behavior distinct.
  Required misses drop or withhold rows.
  Optional misses become NULL.
  Preserve batched campus prefetch, request caps, and the backfill deadline.
- In [`upsert.go`](internal/sync/upsert.go), retain `resolveWithRow` and `writeRowDiffers`.
  `sql.ResolveWithNewValues()` misses columns absent from an entire INSERT batch and fails to clear stored nullable values.
  Avoid unchanged full-mode writes, which cause unnecessary LiteFS replication.
- Use the shared upstream client and its limiter for all PeeringDB requests.
  The configured default origin is `https://www.peeringdb.com`.
  The client appends `/api/` paths.
  Do not substitute historical hosts from incident notes.
- Retry only the short writes listed in [Lock-error retry](docs/ARCHITECTURE.md#lock-error-retry-of-short-writes).
  Do not retry the main sync transaction.
- Emit success counters and committed-change logs after commit.
  Keep sync tracing at cycle, type, or batch granularity, never one span per mutation.

## Compatibility and traversal

Compatibility work targets `/api/`, within reason.
Other APIs have project-defined contracts.
Prioritize equal aggregate record sets over incidental ordering differences.
Read [Validation Notes](docs/API.md#validation-notes) for upstream source anchors and later corrections.
Do not treat an old upstream revision in a comment as the current release.

- Preserve `wireEntity` as the shared builder for List, Count, Match, ListIDs, and ListDepth.
  Append `applyStatusMatrix` last so caller filters can narrow, but cannot broaden, visible statuses.
- Keep status-gated PK lookups in [`depth.go`](internal/pdbcompat/depth.go).
  A bare ent `Get` is not equivalent.
  Expanded sets remain live-only and apply `setDateFilter(ctx)`.
- Use `pyInt` for compatibility integer parsing and `writeError` for its negotiated error envelope.
  Preserve parsing precedence, detail-filter behavior, and registered divergences.
- Follow the [traversal procedure](docs/DEVELOPMENT.md#adding-a-pdbcompat-1-hop-or-2-hop-traversal-filter).
  Edit source allowlists with upstream citations, then regenerate static maps.
  Do not walk `client.Schema.Tables` at runtime or exceed the key-segment cap.
- Distinguish model fields from serializer-only `NonModelFields`.
  Preserve relation-seed precedence, field exclusions, privacy predicates, and folded target routing.
  Add new `prepare_query` resolvers to `isPrepareQueryKey` so name-search misses preserve error precedence.
- Use [`internal/unifold`](internal/unifold) for folding and keep shadow columns off wire APIs.
  Preserve canonical stored IP and prefix strings used by indexed lookups.
- Add parity tests with isolated inline seeds and upstream source citations.
  Each intentional difference needs a `DIVERGENCE_` test and a [Known Divergences](docs/API.md#known-divergences) entry.
  Do not silently broaden compatibility work to other APIs or add an external geocoder.

## Response memory

The [Response Memory Envelope](docs/ARCHITECTURE.md#response-memory-envelope) applies to pdbcompat.
Other APIs have separate limits.

- Preserve the pre-flight budget check and shared in-flight admission pool.
  Count and render must use the same predicates.
- Keep depth expansion, [`childSets`](internal/pdbcompat/detail_budget.go), list loaders, and row-size estimates consistent.
  Preserve chunked list rendering and selected-set loading.
  Validate changes with the list-depth and budget tests.
- Keep `/api/as_set` outside the entity registry and use its own estimate through the shared budget and pool.
- Sample response heap through [`telemetry.go`](internal/pdbcompat/telemetry.go), at entry and deferred exit, never per row.
- Do not add per-endpoint budget overrides or copy this budget to another API without a separate design.

## Middleware and RPC

- Response-writer wrappers must preserve `http.Flusher` and expose `Unwrap() http.ResponseWriter`.
- Preserve the tested [middleware order](docs/ARCHITECTURE.md#middleware-chain), including both recovery layers and deferred route tagging.
  A recovered panic must still produce the HTTP metric.
- Use [`queryError`](internal/grpcserver/errors.go) for RPC query failures.
  Do not return internal ent error text to clients.
- Keep ETag updates on every node through the database-version watcher, not the primary-only sync worker.

## Operations

Use [Deployment](docs/DEPLOYMENT.md) for fleet topology, rollout, and debugging procedures.
Read current configuration instead of assuming machine counts, memory sizes, or volume capacity.

- Preserve LiteFS role detection through [`IsPrimaryWithFallback`](internal/litefs/primary.go).
  `.primary` exists on replicas.
  Its absence alone is insufficient during a lease gap.
- Keep volumes on the primary process group and application HTTP/2 traffic outside the LiteFS proxy.
- Never sweep a shared temporary directory for sync scratch files.
  Preserve scratch locks, the free-space guard, and the fallback directory.
- Keep telemetry cardinality controls and existing detection behavior unless the task explicitly changes them.
  Configuration defaults belong in [Configuration](docs/CONFIGURATION.md), not this guide.
