# Phase 1: Data Foundation - Context

**Gathered:** 2026-03-22
**Status:** Ready for planning

<domain>
## Phase Boundary

Model all 13 PeeringDB object types in entgo, store in SQLite, and sync all data via full re-fetch from the PeeringDB API. This phase delivers a working sync pipeline and validated schemas — no API surfaces yet (those are Phase 2).

</domain>

<decisions>
## Implementation Decisions

### API Parsing Strategy
- **D-01:** Use both Django serializer source analysis AND live API response validation to determine actual PeeringDB response shapes
- **D-02:** Do NOT reference existing Go libraries (e.g., gmazoyer/peeringdb) — derive everything independently from Python source + live API
- **D-03:** Parse PeeringDB's `{"meta": {...}, "data": [...]}` wrapper with a single generic parser, then unmarshal each object by type
- **D-04:** Build the PeeringDB API client as a standalone reusable package at `internal/peeringdb`
- **D-05:** Unauthenticated API access at 20 req/min with a rate limiter — no PeeringDB API key required
- **D-06:** Fetch all 13 object types in dependency order in a single sync pass — no priority ordering
- **D-07:** Use maximum page size per request, loop through all pages sequentially per object type
- **D-08:** Handle unknown fields leniently — log at warn level and skip, don't break sync
- **D-09:** PeeringDB base URL configurable via environment variable
- **D-10:** HTTP timeouts with 3 retries using exponential backoff on transient errors (429, 5xx)

### Schema Extraction Pipeline
- **D-11:** Build a Go-based extraction tool in `cmd/` that parses PeeringDB's Python Django serializer definitions (AST-level, not Django introspection)
- **D-12:** Extraction tool accepts a local PeeringDB repo path as argument (does NOT auto-clone)
- **D-13:** Output an intermediate JSON schema representation, not entgo schemas directly
- **D-14:** JSON schema uses FK references format: `{"field": "net_id", "references": "net"}`
- **D-15:** JSON schema includes full metadata: read-only, required, deprecated, help_text
- **D-16:** Extraction tool includes built-in validation against live PeeringDB API responses
- **D-17:** A separate Go tool in `cmd/` reads the intermediate JSON and generates entgo schema `.go` files
- **D-18:** The full pipeline (extraction → JSON → entgo generation → ent codegen) is chained via `//go:generate` directives

### Sync Behavior
- **D-19:** Entire sync wrapped in a single database transaction — readers never see partial sync state
- **D-20:** Nullable FK fields in entgo schemas to handle PeeringDB's referential integrity violations
- **D-21:** On sync failure: roll back the transaction (preserve previous good data), then retry with 3x exponential backoff (30s, 2m, 8m)
- **D-22:** Hourly sync via in-process Go `time.Ticker` — no external scheduler
- **D-23:** On-demand sync trigger via HTTP endpoint (`POST /sync`), protected by shared secret header (`X-Sync-Token`)
- **D-24:** Sync mutex — if a sync is already running, skip the new request and log it
- **D-25:** Log per-object-type progress during sync (start/complete for each of 13 types) plus a summary with total objects and duration at the end
- **D-26:** Persistent `sync_status` metadata table in SQLite with last_sync_at, duration, object_counts, status
- **D-27:** Same code path for initial sync and subsequent syncs — no special first-sync behavior
- **D-28:** SQLite WAL journal mode enabled for concurrent reads during sync writes
- **D-29:** Sync worker runs only on the LiteFS primary node — replicas receive updates via replication
- **D-30:** Application returns 503 until first sync completes — no empty results served
- **D-31:** Hard delete: remove local rows that no longer appear in PeeringDB's response
- **D-32:** Inclusion of PeeringDB objects with `status=deleted` is configurable via env var, default to excluding them
- **D-33:** All application config via environment variables only (12-factor, Fly.io native)
- **D-34:** Database path configurable via `PDBPLUS_DB_PATH` env var with sensible default
- **D-35:** Basic OTel spans around the full sync and per-object-type fetches from Phase 1

### Schema Design
- **D-36:** Go-style field names internally (e.g., `InfoPrefixes4`), expose PeeringDB-style names (`info_prefixes4`) in APIs via JSON struct tags
- **D-37:** Use PeeringDB's integer IDs as entgo primary keys — direct mapping, no translation
- **D-38:** Model relationships as proper entgo edges (not plain FK integers) — enables GraphQL relationship traversal in Phase 2
- **D-39:** Junction/derived objects (netixlan, netfac, carrierfac) modeled as entgo edge-through tables
- **D-40:** Mirror ALL fields PeeringDB returns — complete data fidelity
- **D-41:** Use PeeringDB's `created`/`updated` timestamps only — no entgo time mixins
- **D-42:** Include entgql, entproto, and entrest annotations on schemas upfront — avoid rework in later phases
- **D-43:** Auto-migrate schema on startup (`entclient.Schema.Create()`), but only on the LiteFS primary node
- **D-44:** Mirror POC (point of contact) data as-is, including personal info — it's public data
- **D-45:** Add indexes on commonly-queried fields (ASN, name, status, FK fields) upfront
- **D-46:** No entgo privacy policies; add basic mutation hooks for OTel tracing on writes
- **D-47:** PeeringDB timestamps stored using PeeringDB's original field names — no local sync timestamps per row

### Project Structure
- **D-48:** Single Go module: `github.com/dotwaffle/peeringdb-plus`
- **D-49:** Standard Go project layout: `cmd/`, `internal/`, `ent/schema/`
- **D-50:** Main binary: `peeringdb-plus` (in `cmd/peeringdb-plus/`)
- **D-51:** SQLite driver: `modernc.org/sqlite` (CGo-free)
- **D-52:** Commit generated entgo code (`ent/` directory) to git
- **D-53:** BSD 3-Clause license
- **D-54:** Include multi-stage Dockerfile in Phase 1

### Testing
- **D-55:** Fixture-based tests for CI (recorded real API responses) + optional live integration tests gated behind a build tag or env var
- **D-56:** All live integration tests MUST target `beta.peeringdb.com`, NOT `api.peeringdb.com` — the beta instance is far less loaded and appropriate for testing

### Claude's Discretion
- IP address field storage strategy (netip.Addr custom type vs text — pick what balances correctness and simplicity)
- Exact exponential backoff timing
- Loading skeleton for any interim CLI output
- Exact `sync_status` table column set beyond the specified fields

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### PeeringDB Source
- `https://github.com/peeringdb/peeringdb` — Django source code for serializer analysis (clone locally, provide path to extraction tool)

### PeeringDB API
- PeeringDB API docs at `https://docs.peeringdb.com/api_specs/` — Official API documentation (note: spec diverges from actual responses)
- GitHub Issue #1878 — OpenAPI spec validation failures (duplicate params, invalid requestBody)
- GitHub Issue #1658 — API response format does not match documentation

### SQLite + entgo Integration
- `.planning/phases/01-data-foundation/01-REFERENCE-SQLITE-ENTGO.md` — **CRITICAL:** User-provided code snippets for modernc.org/sqlite + entgo integration. Covers driver registration, DSN pragma syntax, test setup, and Fly.io memory limits. Must be followed exactly.

### Research Artifacts
- `.planning/research/STACK.md` — Technology recommendations with versions
- `.planning/research/ARCHITECTURE.md` — System architecture and component boundaries
- `.planning/research/PITFALLS.md` — Domain-specific pitfalls and prevention strategies
- `.planning/research/FEATURES.md` — Feature landscape and categorization

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- None — greenfield project, no existing code

### Established Patterns
- None yet — Phase 1 establishes the foundational patterns

### Integration Points
- entgo schema definitions drive all downstream code generation (GraphQL, gRPC, REST)
- `internal/peeringdb` client package will be used by the sync worker
- `sync_status` metadata table feeds health endpoints in Phase 3
- OTel spans established here will be extended in Phase 3

</code_context>

<specifics>
## Specific Ideas

- Extraction pipeline: PeeringDB Python repo → Go AST parser → intermediate JSON → Go entgo schema generator → ent codegen. All chained via `//go:generate`.
- The extraction tool validates its output against live API responses as part of its run — catches drift between Python source and actual API behavior.
- Single transaction for the entire sync keeps the data consistent — readers on WAL mode continue serving the previous snapshot until the new sync commits.

</specifics>

<deferred>
## Deferred Ideas

- None — discussion stayed within phase scope

</deferred>

---

*Phase: 01-data-foundation*
*Context gathered: 2026-03-22*
