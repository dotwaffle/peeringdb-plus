# Phase 49: Refactoring & Tech Debt - Context

**Gathered:** 2026-04-02
**Status:** Ready for planning

<domain>
## Phase Boundary

Split large files for maintainability, extract duplicated patterns, add test coverage for untested packages, and resolve known tech debt items. All changes are internal — no new user-facing features, no API changes, all existing routes and behaviors preserved.

</domain>

<decisions>
## Implementation Decisions

### detail.go Split
- Split strategy: query helpers separate from handlers
- detail.go keeps the handleXxxDetail functions (routing/rendering logic)
- New files: query_network.go, query_ix.go, query_facility.go, query_org.go, query_campus.go, query_carrier.go
- Each query file contains the queryXxx function and any helper functions specific to that entity type
- No single file should exceed 300 lines after the split
- All functions stay in the `web` package — no new packages
- All existing routes must continue to work (verify with existing tests)
- getFreshness() stays in detail.go (shared by all handlers)

### Upsert Deduplication
- Interface-based approach: define an UpsertBuilder interface
- Interface contract: each entity type implements a method that takes PeeringDB data and returns ent create builders
- Shared batch loop: a single function handles the batching (batchSize=500), error wrapping, and ID collection
- Keep convertSocialMedia as a shared helper (already used by multiple types)
- The 13 per-type functions (upsertOrganizations, upsertNetworks, etc.) are replaced by:
  1. Per-type builder functions implementing the interface
  2. A single generic upsert function that handles batching
- worker.go call sites updated to use the new generic function

### Test Coverage: GraphQL Handler
- Test internal/graphql/handler.go for:
  - classifyError() with various ent error types (IsNotFound, IsValidationError, etc.)
  - Error presenter populates path and extensions correctly
  - Complexity limit (500) rejects overly complex queries
  - Depth limit (15) rejects overly deep queries
- Use httptest with actual GraphQL queries against in-memory ent client
- Test file: internal/graphql/handler_test.go

### Test Coverage: Database Package
- Test internal/database/database.go for:
  - Open() with valid path creates ent client successfully
  - Open() with invalid path returns error
  - WAL journal mode is set (query PRAGMA journal_mode)
  - Foreign keys are enabled (query PRAGMA foreign_keys)
  - Busy timeout is configured (query PRAGMA busy_timeout)
- Use temp directory for test databases
- Test file: internal/database/database_test.go

### /ui/about Terminal Rendering
- Implement full rich terminal rendering for the About page
- Content: project name/version, data freshness timestamp, list of API endpoints with URLs, project description
- Use existing termrender.Renderer.RenderPage pattern — register an AboutDetail renderer
- Data is already available via templates.DataFreshness struct passed as page.Data
- Should work with all terminal modes (rich, plain, JSON, WHOIS, short)

### Seed Consolidation
- Unexport Minimal -> minimal (lowercase)
- Unexport Networks -> networks (lowercase)
- Both functions remain usable within seed_test.go (same package)
- seed.Full remains the only exported function (the public API)
- Update any imports that reference seed.Minimal or seed.Networks (grep confirms only seed_test.go uses them)

### Claude's Discretion
- Exact file boundaries when splitting detail.go (where to cut if a function is borderline)
- UpsertBuilder interface method signatures (exact generic parameter names)
- GraphQL test query complexity for complexity limit testing
- About page terminal rendering layout details (field alignment, colors)

</decisions>

<code_context>
## Existing Code Insights

### Key Files to Modify
- `internal/web/detail.go` (1422 LOC) — split into detail.go + 6 query_*.go files
- `internal/sync/upsert.go` (613 LOC) — extract interface + generic batch function
- `internal/graphql/handler.go` (169 LOC) — already modified in Phase 48 (sentinel errors); tests added here
- `internal/database/database.go` (35 LOC) — tests added, no modification to source
- `internal/web/about.go` (32 LOC) — no change to source; terminal rendering via termrender registration
- `internal/web/termrender/` — register About page renderer
- `internal/testutil/seed/seed.go` — unexport Minimal and Networks
- `internal/testutil/seed/seed_test.go` — update references to lowercase

### Established Patterns
- Terminal renderers registered via termrender.Register[T]() generic function
- Test files use testutil/seed.Full(t, client) for entity creation
- Table-driven tests with t.Parallel() per GO-T-1, GO-T-3
- Existing handler_test.go in web package provides test patterns for httptest-based tests

### Integration Points
- detail.go split must not change package boundaries — all files stay in internal/web/
- Upsert interface changes must not change the worker.go call pattern (it still calls upsertOrganizations etc.)
- Actually, worker.go call sites WILL change — they'll call the generic function with a type-specific builder
- GraphQL tests need the full ent client stack (ent + sqlite) for realistic testing

</code_context>

<specifics>
## Specific Ideas

- For the UpsertBuilder interface: type UpsertBuilder[T any] interface { Build(tx *ent.Tx, item T) *ent.XxxCreate }
- Actually, since ent create types differ per entity, may need a simpler approach: func upsertBatch[B any](ctx context.Context, tx *ent.Tx, builders []B, saveFn func(context.Context, []*B) error) error
- About page terminal: render as a simple key-value block (like network detail header) with project info fields

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>
