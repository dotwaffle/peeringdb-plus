# Phase 34 Context: Query Optimization & Architecture

## Requirements
- **PERF-01**: Search service uses single query per entity type (not separate item + count)
- **PERF-03**: Database indexes on `updated` and `created` fields
- **PERF-05**: Field projection avoids JSON marshal/unmarshal roundtrip
- **ARCH-01**: Unified error format across 5 API surfaces (not GraphQL)
- **ARCH-04**: Interface-based terminal renderer dispatch
- **QUAL-04**: Web detail handlers refactored (each under 80 lines)

## Decisions

### Search Query Optimization: Fetch limit+1
- Fetch 11 items, return 10, use 11th as `hasMore` signal
- Drop the separate `Count()` query per entity type entirely
- Search results show items without total count badge
- This halves queries from 12 to 6 per search request
- Update `SearchResult` struct to have `HasMore bool` instead of `TotalCount int`
- Update search results template to show "..." or similar instead of count badges, or keep count badges from the items returned

### Database Indexes
- Add indexes on `updated` and `created` fields across schemas that use incremental sync
- Check which ent schemas have these fields and add `Indexes()` entries
- Verify with `EXPLAIN QUERY PLAN` in tests

### Field Projection: Pre-built Field Map Per Type
- At init time, build `map[string]func(any) any` per entity type
- Field accessors compiled once at startup, called per item at request time
- No intermediate JSON serialization
- Located in `internal/pdbcompat/` — replace `itemToMap()` function

### Error Format: RFC 9457 (Problem Details for HTTP APIs)
- Fields: `type` (URI), `title`, `status` (int), `detail`, `instance` (optional)
- Apply to: REST, PeeringDB compat, ConnectRPC, Web UI, Terminal
- **NOT GraphQL** — keep gqlgen standard error format with extensions.code
- Create shared error response helper in `internal/` (new package or add to existing)
- Each surface's error paths call the shared helper

### Renderer Interface: In termrender Package
- Interface: `RenderTerminal(w io.Writer, r *Renderer) error`
- Data types (templates.NetworkDetail, etc.) implement this interface
- Renderer passed in for access to mode/noColor/width/sections state
- Replaces type-switch in `RenderPage()`
- Interface defined in `internal/web/termrender/`

### Detail Handler Refactor: Handler Methods
- Extract query logic to methods on Handler: `h.queryNetwork(ctx, asn)` returns data struct
- Handler methods have ent client access via receiver
- Each handler function body target: under 80 lines
- Query methods return the typed data struct that templates/renderer consume

## Scope Boundaries
- Do NOT change GraphQL error handling (keep gqlgen format)
- Do NOT add new API capabilities — just optimize existing paths
- Do NOT change search UI behavior beyond removing total count (or adapting it to hasMore)
- The pre-built field map should be generated from struct tags at init, not hand-coded per field
