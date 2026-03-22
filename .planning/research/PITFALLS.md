# Domain Pitfalls

**Domain:** Distributed PeeringDB data mirror (SQLite/LiteFS, entgo, multi-API)
**Researched:** 2026-03-22

## Critical Pitfalls

Mistakes that cause rewrites, data loss, or fundamental architecture problems.

### Pitfall 1: LiteFS Is Unsupported and Pre-1.0

**What goes wrong:** LiteFS is in a "workable but not 100% polished state" with Fly.io explicitly stating they "are not able to provide support or guidance for this product." LiteFS Cloud was sunset in October 2024. Active development is deprioritized. Teams build on LiteFS expecting production-grade tooling and discover they are entirely on their own for debugging, operational issues, and future compatibility.

**Why it happens:** LiteFS was heavily promoted by Fly.io in 2022-2023, creating strong ecosystem buzz. Many blog posts and tutorials still recommend it without noting the support status change. The technology itself works, but the organizational commitment behind it has evaporated.

**Consequences:** No support tickets, no guaranteed bug fixes, no roadmap. If a subtle replication bug surfaces in production, the team must debug FUSE-level filesystem behavior and Consul-based leader election with zero vendor assistance. Operational knowledge must be built entirely in-house.

**Prevention:**
- Accept this risk explicitly as a project decision, documented in KEY DECISIONS.
- Pin to a specific LiteFS version and do not upgrade without testing.
- Build comprehensive integration tests that exercise the full write-replicate-read cycle.
- Implement a fallback plan: design the sync layer so it could target Turso, Litestream+S3, or even PostgreSQL with minimal refactoring. Keep the storage layer behind an interface.
- Monitor the LiteFS GitHub repository for activity. If it goes fully dormant, migrate.

**Detection:** Watch for: LiteFS GitHub repo inactivity (no commits for 6+ months), Fly.io docs removing LiteFS pages, community forum threads going unanswered.

**Phase impact:** Must be acknowledged in Phase 1 (infrastructure setup). The fallback abstraction should be designed before any sync logic is written.

**Confidence:** HIGH -- based on official Fly.io community statements and documentation warnings.

---

### Pitfall 2: PeeringDB API Responses Don't Match Their OpenAPI Spec

**What goes wrong:** Teams generate client code from PeeringDB's published OpenAPI spec, discover it fails validation, produces incorrect types, or misses fields entirely. The actual API response shapes diverge from the spec in multiple ways: duplicate parameter definitions, request body schemas that aren't type `object`, and field presence/format differences.

**Why it happens:** PeeringDB is a Django application where the API is defined by Python serializers, not by the OpenAPI spec. The spec is generated/maintained separately and lags behind reality. PeeringDB issue #1878 documented concrete schema validation failures with openapi-generator. The production database also contains data that fails validation against their own schema (issue #637).

**Consequences:** Any code-generated client will produce incorrect types. Fields may be missing, have wrong types, or contain unexpected null values. Sync will fail silently (wrong data) or loudly (deserialization errors) depending on how strict the parser is.

**Prevention:**
- Do NOT trust the OpenAPI spec for type definitions. Instead, analyze the actual Django serializers in the PeeringDB Python source code to understand real response shapes.
- Write a custom Go client by hand, informed by source analysis and live API response sampling.
- Build a response validation layer that compares expected vs. actual field presence/types during sync, logging discrepancies.
- Collect sample responses from every PeeringDB endpoint and use them as golden test fixtures.
- Subscribe to PeeringDB release notes -- response shapes can change between versions.

**Detection:** Deserialization errors during sync. Fields showing as zero values when they should have data. Missing relationships. Type assertion panics.

**Phase impact:** This is the FIRST thing to tackle -- before writing any entgo schema. The entgo schema must reflect reality, not the published spec.

**Confidence:** HIGH -- confirmed via PeeringDB GitHub issues #1878, #637, #1658.

---

### Pitfall 3: Full Re-Fetch Sync Hitting Rate Limits and WAL Size Limits

**What goes wrong:** A full re-fetch downloads every PeeringDB object every sync cycle. PeeringDB rate-limits anonymous requests to 20/minute and authenticated to 40/minute. A full sync that fetches each object type sequentially may take many minutes and risks hitting throttling. Additionally, replacing all data in a single large SQLite transaction causes the WAL file to grow to the size of the entire database, and WAL mode performs poorly for transactions exceeding ~100MB and may fail entirely above ~1GB.

**Why it happens:** "Full re-fetch" sounds simple but has compounding costs: network (rate limits), compute (parsing), and storage (WAL growth during bulk delete+insert). The PeeringDB dataset includes organizations, networks, facilities, IXPs, and all their derived objects (netfac, netixlan, ixpfx, ixlan, poc). Deleting and reinserting all of them in one transaction is a massive write.

**Consequences:** Rate limiting causes sync to fail partway through, leaving partial data. Large transactions cause WAL bloat, slow checkpointing, and potentially I/O errors. LiteFS must replicate these large LTX files to all replicas, causing replication lag spikes.

**Prevention:**
- Use authenticated API access with an API key (40 req/min vs 20).
- Batch PeeringDB queries efficiently -- use `limit`/`skip` pagination and fetch multiple IDs per request (up to 150 ASNs per query).
- Space requests at least 2 seconds apart as PeeringDB recommends.
- Break the sync into per-table transactions rather than one giant transaction. Sync org, then net, then fac, etc. -- each in its own transaction.
- Consider using the `since` parameter for incremental updates after the first full sync, even though the design says "full re-fetch." The first sync must be full; subsequent ones can be incremental for efficiency.
- Monitor WAL file size and checkpoint aggressively between table syncs.
- Set a meaningful User-Agent header identifying the software.

**Detection:** HTTP 429 responses from PeeringDB. WAL file growing beyond 100MB. Sync taking longer than expected. Replica lag exceeding the configured LiteFS batch interval (default 1 second).

**Phase impact:** Sync strategy design (Phase 2 or whenever sync is built). Must be designed before implementation, not discovered during testing.

**Confidence:** HIGH -- PeeringDB rate limits are documented; SQLite WAL limits are in official SQLite documentation.

---

### Pitfall 4: PeeringDB Data Has Referential Integrity Violations

**What goes wrong:** The PeeringDB production database contains records with foreign key references to entities that don't exist. For example, a `netfac` record referencing a facility ID that has been deleted, or an `ixlan` referencing a non-existent IX. Syncing this data into a local database with foreign key constraints enabled causes constraint violations and sync failures.

**Why it happens:** PeeringDB is a Django application that has accumulated data inconsistencies over years of operation. Their own sync tools (django-peeringdb, peeringdb-py) have documented issues with sync failures due to these inconsistencies (issues #31, #46, #17, #38).

**Consequences:** If entgo schemas enforce foreign key constraints (which they should for data integrity), sync will fail on the first referential integrity violation. The sync process halts, leaving an incomplete dataset. Disabling foreign keys to work around this sacrifices data integrity guarantees.

**Prevention:**
- Sync objects in dependency order: org first, then fac/ix/net (which reference org), then derived objects (netfac, netixlan, ixpfx, ixlan) last.
- Implement a "soft foreign key" strategy: store foreign key IDs as plain integer fields in the entgo schema, validate relationships at query time rather than insert time, and log orphaned references.
- Alternatively, sync into a staging database without FK constraints, validate referential integrity post-sync, log violations, then swap the staging database into place.
- Build a reconciliation report that identifies and logs all orphaned references after each sync.

**Detection:** Foreign key constraint errors during sync. Missing related objects when traversing edges in the API. Sync processes that complete but with error logs showing skipped records.

**Phase impact:** Entgo schema design (Phase 1) and sync implementation (Phase 2). The schema must be designed with this constraint in mind from day one.

**Confidence:** HIGH -- documented in multiple PeeringDB GitHub issues with concrete error traces.

---

### Pitfall 5: LiteFS Write Routing and Primary Election Complexity

**What goes wrong:** LiteFS uses a single-writer model where only the primary node can write. All writes must be routed to the primary. The primary is determined by a Consul lease. If the primary dies unexpectedly, there's a TTL-based failover delay (default 10 seconds). During this window, writes fail. If autoscale stop/start is enabled, a stale machine can win the lease and LiteFS discards newer changes, causing data rollback.

**Why it happens:** Distributed consensus is fundamentally complex. LiteFS uses Consul for leader election rather than embedding its own consensus protocol. The FUSE layer adds another abstraction that can mask failures. The system was designed for applications with mixed read/write workloads, but the failure modes around write availability are subtle.

**Consequences:** For this project (read-only mirror with periodic sync writes), the impact is somewhat mitigated -- writes only happen during sync. But if sync runs on a replica node, it will fail silently or the proxy will attempt to forward it. If the primary fails during sync, the sync is lost and must retry. If autoscale is misconfigured, data can roll back to a previous state.

**Prevention:**
- Pin the sync process to run ONLY on the primary node. Check the `.primary` file in the LiteFS directory before starting sync -- if it exists, this node is a replica and should not sync.
- Disable autoscale stop for the primary region, or use static leasing where one specific region is always primary.
- Configure the Consul lease TTL appropriately -- 10 seconds is reasonable for this use case since sync is infrequent.
- Implement sync idempotency so a failed/partial sync can be safely retried.
- Use `fly-replay` header for any write requests if using the HTTP proxy.

**Detection:** Sync failures with "read-only database" errors. Data appearing to revert to older state. The `.primary` file appearing/disappearing unexpectedly. Consul lease acquisition failures in logs.

**Phase impact:** Infrastructure and deployment configuration (Phase 1). Must be designed before sync implementation.

**Confidence:** HIGH -- based on official LiteFS documentation and Fly.io community reports.

---

## Moderate Pitfalls

### Pitfall 6: entproto Does Not Support M2M Edges

**What goes wrong:** entproto (the gRPC code generator for entgo) does not support many-to-many (M2M) edges, particularly M2M same-type edges. PeeringDB's data model has inherent M2M relationships (networks present at multiple facilities, facilities hosting multiple networks). Attempting to generate gRPC service definitions for schemas with M2M edges produces errors or generates incorrect proto definitions.

**Why it happens:** entproto generates proto messages from ent schemas, and protobuf messages cannot self-reference or easily represent join tables. The entproto codebase has a TODO comment acknowledging this gap (GitHub issue #2476).

**Prevention:**
- Design the entgo schema with entproto limitations in mind. Use intermediate "through" entities (e.g., `NetworkFacility` as a first-class entity rather than an M2M edge between `Network` and `Facility`) which maps naturally to PeeringDB's derived objects anyway.
- Prioritize GraphQL (entgql) as the primary API surface -- it handles M2M relationships well via Relay connections.
- For gRPC, consider hand-writing proto definitions for the M2M-heavy parts rather than relying on entproto generation.
- Evaluate whether gRPC is truly needed for v1, or if GraphQL + REST covers the use cases.

**Detection:** Code generation errors mentioning edge types. Proto files with missing or incorrect repeated fields. gRPC responses that don't include expected relationship data.

**Phase impact:** Schema design (Phase 1) and gRPC API surface (later phase). Must inform the entgo schema structure from the beginning.

**Confidence:** HIGH -- confirmed via ent/ent GitHub issue #2476.

---

### Pitfall 7: entgo Eager Loading Generates N+1 Queries

**What goes wrong:** When loading edges in entgo, each edge requires a separate SQL query. A GraphQL query requesting networks with their facilities and IX presences will generate 1 query for networks + 1 query for facilities + 1 query for IX presences per network. With hundreds of networks, this becomes thousands of queries.

**Why it happens:** entgo's eager loading uses `With*()` methods that execute additional queries per association. It cannot currently combine all associations into a single JOIN. The entgo team has acknowledged this will be "optimized in future versions." With SQLite, these queries are fast (local I/O, no network round trip), but the overhead is still meaningful at scale.

**Prevention:**
- Accept that N+1 queries are less catastrophic with SQLite than with a network database -- all queries hit local disk/memory.
- Use pagination aggressively in GraphQL (Relay cursor connections) to limit the number of root objects, which bounds the N+1 fan-out.
- Implement query complexity analysis in the GraphQL layer to reject excessively deep/wide queries.
- Consider caching frequently-accessed edge data at the application level if profiling shows hot paths.
- Monitor query counts per request using OpenTelemetry spans on the entgo client.

**Detection:** Slow API responses on queries with many edges. High query counts visible in OTel traces. SQLite busy timeout errors under concurrent read load.

**Phase impact:** GraphQL API implementation (Phase 2-3). Design pagination and query limits early.

**Confidence:** MEDIUM -- entgo documentation confirms the behavior; severity depends on dataset size and query patterns.

---

### Pitfall 8: SQLite Driver Choice and Foreign Key PRAGMA Per-Connection

**What goes wrong:** SQLite's `foreign_keys` PRAGMA is a per-connection setting that defaults to OFF. If any code path opens a connection without setting `PRAGMA foreign_keys = ON`, that connection can silently violate referential integrity. Additionally, choosing the wrong SQLite driver (mattn/go-sqlite3 vs modernc.org/sqlite) causes name registration conflicts, migration failures on subsequent runs, and CGO build complexity.

**Why it happens:** SQLite PRAGMAs are connection-scoped, not database-scoped. `journal_mode=WAL` persists, but `foreign_keys`, `busy_timeout`, and `synchronous` must be set on every new connection. The entgo SQLite dialect uses `dialect.SQLite` which maps to the driver name `sqlite3` (mattn's driver), but `modernc.org/sqlite` registers as `sqlite`. mattn's driver requires CGO; modernc doesn't but has known migration bugs (issue #2209).

**Prevention:**
- Use `modernc.org/sqlite` to avoid CGO dependency (critical for reproducible Docker builds on Fly.io). Pin to a tested version.
- Write a custom `database/sql` driver wrapper or use entgo's `sql.OpenDB` with a connector that executes PRAGMAs on every new connection:
  ```
  PRAGMA journal_mode=WAL;
  PRAGMA foreign_keys=ON;
  PRAGMA busy_timeout=5000;
  PRAGMA synchronous=NORMAL;
  ```
- Register the modernc driver with the name `sqlite3` to match entgo's dialect expectation, OR use entgo's `dialect.SQLite` with the correct driver name configuration.
- Test migrations end-to-end: create schema, modify schema, verify no "invalid type INTEGER" errors on subsequent migrations.

**Detection:** Silent data integrity violations (missing FK enforcement). Build failures requiring GCC/CGO toolchain. Migration panics on the second `Schema.Create()` call.

**Phase impact:** Project bootstrap (Phase 1). Driver and PRAGMA configuration must be correct from day one.

**Confidence:** HIGH -- confirmed via entgo GitHub issues #1667, #2209 and SQLite documentation.

---

### Pitfall 9: entrest Is Work-In-Progress with Breaking Changes Expected

**What goes wrong:** entrest (lrstanley/entrest) explicitly warns: "Documentation & entrest itself are a work in progress (expect breaking changes)." Teams build their REST API surface on entrest and then face breaking changes in the generated code, API structure, or annotations between versions.

**Why it happens:** entrest is a community project (not part of the official ent/contrib ecosystem like entgql and entproto). It's maintained by a single developer. The project is functional but not yet stable.

**Prevention:**
- Pin the entrest version strictly in go.mod. Do not auto-upgrade.
- Treat entrest as a convenience for v1, not a permanent dependency. Be prepared to replace it with a hand-written REST layer if it becomes unmaintained.
- Prioritize GraphQL (entgql, which is mature and officially maintained) as the primary API surface.
- If REST is needed early, consider generating an OpenAPI spec from entrest but implementing handlers manually for critical endpoints.

**Detection:** Build failures after `go get -u`. Generated code changes behavior between versions. Missing or renamed annotations.

**Phase impact:** REST API surface (later phase, Phase 3+). Not blocking for MVP if GraphQL is prioritized.

**Confidence:** MEDIUM -- based on entrest's own documentation stating WIP status.

---

### Pitfall 10: Full Re-Fetch Causes Stale Read Windows During Sync

**What goes wrong:** During a full re-fetch sync, the database is in a transitional state. If the sync deletes all records from a table before repopulating it, any API request during that window returns empty results. Even if done in a transaction, the transaction holds a write lock that blocks other writers (though readers can continue in WAL mode). If the sync takes minutes, readers see stale data from before the transaction started.

**Why it happens:** SQLite's WAL mode provides snapshot isolation -- readers see the database as it was when their read transaction began. A long-running write transaction (sync) doesn't block readers, but readers won't see the new data until the write commits. This is actually correct behavior, but the staleness window equals the sync duration.

**Prevention:**
- Use a two-database strategy: sync into a "staging" database, then atomically swap it with the "live" database. This minimizes the staleness window to the swap time rather than the sync duration.
- If using a single database, sync per-table using UPSERT (INSERT OR REPLACE) rather than DELETE-then-INSERT. This keeps existing data available throughout.
- If using per-table transactions, sync in dependency order and accept brief inconsistency windows between tables.
- Add a `/health` or `/status` endpoint that reports the last successful sync timestamp and current sync state, so consumers know data freshness.

**Detection:** API responses returning empty results during sync. Monitoring showing data age exceeding sync interval. User reports of missing data that reappears after sync completes.

**Phase impact:** Sync strategy design (Phase 2). Must be decided before implementing the sync mechanism.

**Confidence:** MEDIUM -- based on SQLite WAL semantics (documented) applied to this specific use case.

---

## Minor Pitfalls

### Pitfall 11: OpenTelemetry in entgo Uses OpenCensus Bridge

**What goes wrong:** entgo's native tracing support uses OpenCensus, not OpenTelemetry. Since OpenCensus merged into OpenTelemetry, using entgo's built-in tracing requires the OpenCensus-to-OpenTelemetry bridge, which adds complexity and may have subtle compatibility issues.

**Prevention:**
- Use the `go.opentelemetry.io/otel/bridge/opencensus` package to bridge entgo's OpenCensus traces to OpenTelemetry.
- Instrument HTTP/gRPC/GraphQL layers directly with OTel middleware (otelhttp, otelgrpc) rather than relying solely on entgo's built-in tracing.
- Consider writing a custom entgo hook/interceptor that creates OTel spans directly, bypassing the bridge entirely.

**Phase impact:** Observability setup (Phase 2-3). Not blocking but adds integration complexity.

**Confidence:** MEDIUM -- entgo GitHub issue #1232 confirms the OpenCensus dependency; bridge behavior needs validation.

---

### Pitfall 12: HTMX + Templ Web UI as Secondary Priority Can Become Scope Creep

**What goes wrong:** The web UI is listed as "secondary priority" but HTMX + Templ require server-side rendering logic, additional routes, template compilation, and CSS/styling decisions. What starts as "a simple browse UI" expands to search, filtering, comparison views, and network topology visualization -- consuming disproportionate development time.

**Prevention:**
- Defer the web UI to a dedicated phase after all API surfaces are stable.
- Define a strict scope for v1 UI: read-only browsing of entities with links between related objects. No search, no comparison, no visualization.
- Use the GraphQL API as the data source for the UI (dogfooding the primary API).
- Set a hard time-box for UI work and enforce it.

**Phase impact:** Should be the last phase. Must not block API development.

**Confidence:** HIGH -- this is a general software engineering pattern; the specific risk is amplified by the "secondary priority" framing which invites deprioritization-then-catch-up cycles.

---

### Pitfall 13: PeeringDB `depth` Parameter Behavior Differs Between Single and List Endpoints

**What goes wrong:** PeeringDB's `depth` parameter behaves differently for single-object GET (`/api/net/1?depth=2`) vs. list GET (`/api/net?depth=2`). For single objects, depth expands both sets AND single relationships (e.g., `net_id` becomes an object). For list operations, depth ONLY expands sets, not single relationships. Teams assume uniform behavior and get different response shapes depending on the endpoint.

**Prevention:**
- Document the depth behavior difference in the sync client code.
- For full re-fetch sync, use `depth=0` (default, no expansion) and resolve relationships via IDs. This produces the simplest, most predictable response format.
- If depth expansion is used, have separate deserialization paths for single vs. list responses.

**Phase impact:** Sync client implementation (Phase 2).

**Confidence:** HIGH -- documented in PeeringDB API specs.

---

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Mitigation |
|-------------|---------------|------------|
| Infrastructure / LiteFS setup | LiteFS unsupported, primary election complexity (#1, #5) | Pin version, design storage abstraction, configure static leasing |
| Entgo schema design | PeeringDB spec mismatch (#2), FK violations (#4), entproto M2M gaps (#6) | Analyze Python source, use "through" entities, defer FK enforcement |
| SQLite driver setup | Driver conflicts, PRAGMA per-connection (#8) | Use modernc, connection wrapper for PRAGMAs |
| Sync implementation | Rate limits, WAL size, stale reads (#3, #10) | Per-table transactions, UPSERT strategy, staging DB |
| GraphQL API | N+1 queries (#7) | Pagination, query complexity limits, OTel monitoring |
| gRPC API | M2M edge generation failure (#6) | Use intermediate entities, consider hand-written protos |
| REST API | entrest breaking changes (#9) | Pin version, treat as convenience not dependency |
| Observability | OpenCensus bridge (#11) | Direct OTel instrumentation, bridge for entgo only |
| Web UI | Scope creep (#12) | Strict scope, time-box, defer to last phase |

## Sources

### LiteFS / Fly.io
- [LiteFS Docs](https://fly.io/docs/litefs/)
- [LiteFS Status Discussion](https://community.fly.io/t/what-is-the-status-of-litefs/23883)
- [Sunsetting LiteFS Cloud](https://community.fly.io/t/sunsetting-litefs-cloud/20829)
- [LiteFS Production Concerns (GitHub #259)](https://github.com/superfly/litefs/issues/259)
- [LiteFS How It Works](https://fly.io/docs/litefs/how-it-works/)
- [LiteFS Primary Detection](https://fly.io/docs/litefs/primary/)
- [LiteFS HTTP Proxy](https://fly.io/docs/litefs/proxy/)
- [WAL Mode in LiteFS](https://fly.io/blog/wal-mode-in-litefs/)

### PeeringDB
- [PeeringDB API Specs](https://docs.peeringdb.com/api_specs/)
- [PeeringDB OpenAPI Schema Errors (GitHub #1878)](https://github.com/peeringdb/peeringdb/issues/1878)
- [PeeringDB Sync Validation Failures (GitHub #637)](https://github.com/peeringdb/peeringdb/issues/637)
- [peeringdb-py FK Constraint Errors (GitHub #46)](https://github.com/peeringdb/peeringdb-py/issues/46)
- [django-peeringdb Sync Errors (GitHub #31)](https://github.com/peeringdb/django-peeringdb/issues/31)
- [PeeringDB Query Limits Guide](https://docs.peeringdb.com/howto/work_within_peeringdbs_query_limits/)
- [Net/ixlan Keys Missing (GitHub #1658)](https://github.com/peeringdb/peeringdb/issues/1658)

### entgo / Ent Ecosystem
- [Ent Supported Dialects](https://entgo.io/docs/dialects/)
- [Ent Eager Loading](https://entgo.io/docs/eager-load/)
- [entproto M2M Same-Type Edge Issue (GitHub #2476)](https://github.com/ent/ent/issues/2476)
- [Ent SQLite modernc Migration Bug (GitHub #2209)](https://github.com/ent/ent/issues/2209)
- [Ent CGo-Free SQLite Discussion (GitHub #1667)](https://github.com/ent/ent/discussions/1667)
- [Ent OpenTelemetry Issue (GitHub #1232)](https://github.com/ent/ent/issues/1232)
- [entrest (lrstanley)](https://lrstanley.github.io/entrest/)
- [entproto Edges Documentation](https://entgo.io/docs/grpc-edges/)

### SQLite
- [SQLite WAL Documentation](https://sqlite.org/wal.html)
- [SQLite Atomic Commit](https://sqlite.org/atomiccommit.html)
- [SQLite PRAGMAs](https://sqlite.org/pragma.html)
- [SQLite Production Configuration (glorifiedgluer)](https://gluer.org/blog/sqlite-production-configuration/)
- [SQLite Recommended PRAGMAs](https://highperformancesqlite.com/articles/sqlite-recommended-pragmas)
- [Gotchas with SQLite in Production (Anze Pecar)](https://blog.pecar.me/sqlite-prod/)
- [SQLite Concurrent Writes and Locking](https://tenthousandmeters.com/blog/sqlite-concurrent-writes-and-database-is-locked-errors/)
- [SQLITE_BUSY Despite Timeout (Bert Hubert)](https://berthub.eu/articles/posts/a-brief-post-on-sqlite3-database-locked-despite-timeout/)
