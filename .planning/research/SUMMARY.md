# Project Research Summary

**Project:** PeeringDB Plus
**Domain:** Server-streaming RPCs for bulk data export + IX presence UI polish (v1.7 milestone)
**Researched:** 2026-03-25
**Confidence:** HIGH

## Executive Summary

The v1.7 milestone adds two independent feature tracks to PeeringDB Plus: server-streaming RPCs for bulk data export across all 13 entity types, and visual polish to the IX presence UI. Research confirms that zero new Go dependencies are needed -- ConnectRPC v1.19.1 already has full server-streaming support, and all UI improvements use existing Tailwind CSS utilities within existing templ templates. The existing infrastructure (h2c, otelconnect, CORS, gRPC reflection, health checks) handles streaming RPCs without modification. This is a surgically additive milestone.

The recommended approach is batch-then-stream: query ent in batches of 500 rows using keyset pagination (`WHERE id > lastID`), convert each to proto, and `stream.Send()` individually. This keeps memory bounded regardless of table size while delivering the streaming benefit on the wire. The proto design streams bare entity messages (e.g., `stream Network`), not wrapped response envelopes. The UI track restructures IX presence rows to separate the clickable link (IX name) from selectable data (IPs, speeds), adds field labels, color-codes port speeds by tier, and improves alignment.

The primary risks are: (1) loading all rows into memory via `ent.All()` instead of batching -- this must be avoided or the streaming feature is worse than useless; (2) long-running streams blocking SQLite WAL checkpointing if wrapped in a single transaction -- each batch must be a separate read; (3) the proto change breaks all 13 handler interfaces simultaneously, requiring stub implementations before the code compiles. All three are well-understood and have clear mitigations documented in the research.

## Key Findings

### Recommended Stack

No stack changes required. Every dependency needed for streaming RPCs and UI improvements is already in `go.mod`. ConnectRPC v1.19.1 includes the `simple` flag (already configured in `buf.gen.yaml`) and stable server-streaming support. The `connect.ServerStream[T]` API provides `Send()`, `ResponseHeader()`, and `ResponseTrailer()` methods. The existing `otelconnect.NewInterceptor()` automatically instruments streaming RPCs with per-message trace events and `rpc.server.responses_per_rpc` histograms. See [STACK.md](STACK.md) for full details.

**Core technologies (all existing):**
- `connectrpc.com/connect` v1.19.1: Server-streaming handler types, all three wire protocols (Connect, gRPC, gRPC-Web)
- `connectrpc.com/otelconnect` v0.9.0: Automatic streaming observability, no configuration changes
- `github.com/a-h/templ` v0.3.1001: Templ templates for UI changes
- buf v2 toolchain: Processes `stream` return types in proto definitions automatically

### Expected Features

See [FEATURES.md](FEATURES.md) for complete feature landscape, dependency graph, and detailed specifications.

**Must have (table stakes):**
- Server-streaming RPC per entity type (13 total) -- bulk export without manual pagination
- Batched keyset pagination inside handlers -- bounded memory regardless of table size
- Graceful stream termination on client disconnect -- honor `ctx.Done()`
- Filtering on streaming RPCs -- reuse existing predicate logic from List handlers
- Total record count in response headers -- `x-total-count` via `stream.ResponseHeader()`
- OTel instrumentation on streams -- automatic via otelconnect, no code needed
- Field labels for speed/IPv4/IPv6 in IX presence rows
- RS badge repositioned near peering data, not far-right
- Port speed color coding by tier (sub-1G/1G/10G/100G+)
- Consistent IP address indentation via grid/fixed-width layout
- Selectable/copyable IP addresses without triggering row navigation

**Should have (differentiators):**
- Format negotiation (proto vs JSON) -- ConnectRPC handles automatically, just document it
- `since_id` filter for stream resumption after interruption
- `updated_since` timestamp filter for incremental sync
- Aggregate bandwidth display in IX presence section header
- Copy-to-clipboard button on IP addresses

**Defer (v2+):**
- SHA256 checksum in response trailers (trailer support varies by client)
- Sortable IX presence table
- Speed distribution mini-chart
- Bidirectional/client streaming (no write path exists)
- Real-time change streaming/subscriptions (hourly sync model)
- Custom download formats (CSV, NDJSON)
- IX presence interactive map

### Architecture Approach

Two fully independent feature tracks with zero code dependencies between them. The streaming track adds `Stream*` methods to each of the 13 existing handler structs in `internal/grpcserver/`, extending the generated ConnectRPC interfaces. No changes to `main.go` registration, reflection, health checks, middleware, or h2c configuration. The UI track modifies three existing templ files (`detail_net.templ`, `detail_ix.templ`, `detail_shared.templ`) with no new routes or handlers. No new files are created in either track. See [ARCHITECTURE.md](ARCHITECTURE.md) for full component boundaries, data flow diagrams, and build order.

**Major components modified:**
1. `proto/peeringdb/v1/services.proto` -- 13 new streaming RPCs + request messages
2. `internal/grpcserver/*.go` (13 files) -- each gains a `Stream*` method (~35-45 lines each)
3. `internal/grpcserver/pagination.go` -- new `streamBatchSize` constant
4. `internal/web/templates/detail_net.templ` -- IX presence row restructure
5. `internal/web/templates/detail_shared.templ` -- `speedColorClass()` helper
6. `internal/web/templates/detail_ix.templ` -- matching layout changes for consistency

### Critical Pitfalls

See [PITFALLS.md](PITFALLS.md) for all 13 pitfalls with detailed prevention strategies and phase-specific warnings.

1. **Loading all rows into memory before streaming** -- Use batched keyset pagination (`WHERE id > lastID ORDER BY id LIMIT 500`), not `ent.All()` for the full table. Monitor `go_memstats_alloc_bytes` during streams.
2. **Long-running streams blocking WAL checkpointing** -- Do NOT wrap the entire stream in a single transaction. Each batch must be a separate short-lived read. Ent's default query behavior (no explicit `Tx`) already does this correctly.
3. **Recovery middleware broken mid-stream** -- Once streaming starts, HTTP 200 is already committed. Panics mid-stream produce truncated results with no error indication. Validate all inputs before the streaming loop; never panic inside it.
4. **No timeout on streaming RPCs** -- Add `context.WithTimeout` (5 min) inside each streaming handler. Check `ctx.Done()` between batches. The endpoint is public and unauthenticated.
5. **Proto change breaks all 13 handler interfaces simultaneously** -- Plan for this: add all 13 streaming RPCs in one proto commit, regenerate, then implement stubs to restore compilation before implementing real logic.

## Implications for Roadmap

Based on research, suggested phase structure:

### Phase 1: Proto Definitions & Code Generation

**Rationale:** Everything in the streaming track depends on generated code from proto definitions. This must come first and breaks all 13 handler interfaces, requiring immediate stub implementation to restore compilation.
**Delivers:** 13 streaming RPC definitions in `services.proto`, 13 request messages, regenerated handler interfaces, compilation-restoring stubs on all 13 handler structs.
**Addresses:** Proto definitions (table stakes), interface compatibility (Pitfall 5)
**Avoids:** Pitfall 5 (interface breakage) by implementing all stubs in the same commit as codegen, Pitfall 9 (stale reflection) by running full codegen pipeline.

### Phase 2: Streaming Handler Implementation

**Rationale:** With stubs in place, implement real streaming logic. The batch-then-stream pattern is the core infrastructure. Start with one reference implementation (Network), validate it end-to-end with `grpcurl`/`buf curl`, then replicate across remaining 12 types.
**Delivers:** 13 working streaming handlers with batched keyset pagination, `streamBatchSize` constant, filtering support, total count header.
**Addresses:** Row-at-a-time streaming (table stakes), filtering (table stakes), total count header (table stakes), graceful termination (table stakes)
**Avoids:** Pitfall 2 (memory blowup) via batched queries, Pitfall 3 (WAL blocking) via separate transactions per batch, Pitfall 4 (no timeout) via `context.WithTimeout`, Pitfall 1 (panic recovery) via defensive input validation before streaming loop.

### Phase 3: Streaming Differentiators & Testing

**Rationale:** After core streaming works, add `since_id` and `updated_since` filters for stream resumption and incremental sync. Add comprehensive tests. These features are low complexity but add significant value for automation consumers.
**Delivers:** `since_id` and `updated_since` filter fields on streaming requests, streaming tests for representative types (empty stream, filtered stream, cancellation, mid-stream error handling).
**Addresses:** Stream resume (differentiator), timestamp filter (differentiator), test coverage
**Avoids:** Pitfall 8 (graceful shutdown) by documenting client reconnection from last received ID.

### Phase 4: IX Presence UI Polish

**Rationale:** Pure template changes with no infrastructure dependencies. All changes are independent of each other and independent of the streaming track. Lower risk than streaming -- worst case is a visual regression, easily reverted.
**Delivers:** Field labels, color-coded port speeds, RS badge repositioning, consistent IP indentation, selectable/copyable text, aggregate bandwidth display.
**Addresses:** All UI table stakes (field labels, RS badge, speed colors, IP alignment, selectable text), aggregate bandwidth (differentiator)
**Avoids:** Pitfall 10 (anchor wrapping) by restructuring rows, Pitfall 11 (contrast) by using full-opacity Tailwind colors only, Pitfall 12 (flex break on mobile) by testing narrow viewports, Pitfall 13 (inconsistent row widths) by using grid layout with fixed columns.

### Phase Ordering Rationale

- **Proto first, handlers second:** Codegen must precede implementation. The interface breakage across 13 files demands a deliberate stub-then-implement approach.
- **Reference implementation before replication:** Implement Network streaming first, validate end-to-end with `grpcurl`/`buf curl`, then replicate the proven pattern across 12 remaining types. This catches batching bugs early.
- **Differentiators after core:** `since_id` and `updated_since` are additive filter fields that modify existing request messages. Adding them after core streaming works avoids scope creep in the critical path.
- **UI last:** Zero dependencies on the streaming track. Can be parallelized with any streaming phase if desired, but sequencing it last keeps the milestone focused.

### Research Flags

Phases likely needing deeper research during planning:
- **Phase 2:** Keyset pagination performance at high row counts (100K+ NetworkIxLan) -- benchmark `WHERE id > N ORDER BY id LIMIT 500` vs OFFSET-based queries on SQLite with the actual dataset. OFFSET degrades linearly; keyset should be O(1) per batch but must verify with ent's generated SQL.
- **Phase 2:** Fly.io proxy behavior with HTTP/1.1 chunked streaming -- verify no intermediate buffering defeats streaming benefit (Pitfall 6). Requires runtime testing, not just docs.
- **Phase 4:** RS badge repositioning on mobile viewports (320px-480px) -- exact placement needs visual QA, not just code review.

Phases with standard patterns (skip research-phase):
- **Phase 1:** Proto syntax for server-streaming RPCs is standard protobuf. `buf generate` handles it. Well-documented.
- **Phase 3:** `since_id` / `updated_since` are simple predicate additions to existing filter logic. Standard ent query patterns.
- **Phase 4 (except RS badge):** Field labels, speed colors, IP alignment, selectable text are all CSS/HTML changes using documented Tailwind utilities.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Zero new dependencies. All capabilities verified against existing `go.mod` versions and pkg.go.dev docs. |
| Features | HIGH | Table stakes are clear and well-scoped. Feature dependency graph is simple. |
| Architecture | HIGH | Additive changes to existing patterns. No new files, no new routes, no new services. Verified against existing codebase structure. |
| Pitfalls | HIGH | All critical pitfalls have well-documented mitigations. SQLite WAL behavior, ConnectRPC streaming semantics, and ent query patterns are well-understood. |

**Overall confidence:** HIGH

### Gaps to Address

- **OFFSET vs keyset pagination:** STACK.md uses `Limit/Offset`, FEATURES.md recommends keyset (`WHERE id > lastID`), PITFALLS.md warns OFFSET degrades linearly. Recommendation: use keyset pagination. Validate during Phase 2 with a benchmark against NetworkIxLan (~100K rows).
- **Handler signature in simple mode:** STACK.md shows `func(ctx, *Req, *connect.ServerStream[Res]) error` (simple mode), FEATURES.md shows `func(ctx, *connect.Request[Req], *connect.ServerStream[Res]) error` (non-simple). The codebase uses simple mode (configured in `buf.gen.yaml`). Confirm generated signature matches STACK.md's version after `buf generate`.
- **RPC naming convention:** STACK.md uses `StreamNetworks`, ARCHITECTURE.md uses `StreamAllNetworks`. Pick one and be consistent. Recommendation: `StreamNetworks` -- the `All` is redundant since streaming always returns all matching records.
- **Port speed color scheme:** Minor discrepancy between FEATURES.md and STACK.md on color mapping for 400G+. Recommendation: use STACK.md's scheme (emerald for 100G+, sky for 10G, amber for 1G, neutral for sub-1G) -- it aligns with the project's existing color vocabulary.
- **Fly.io proxy streaming verification:** Cannot be resolved through docs alone. Must be tested at runtime during Phase 2 deployment.

## Sources

### Primary (HIGH confidence)
- [ConnectRPC Streaming Documentation](https://connectrpc.com/docs/go/streaming/) -- handler patterns, protocol support, HTTP/1.1 compatibility
- [ConnectRPC ServerStream API](https://pkg.go.dev/connectrpc.com/connect#ServerStream) -- Send, ResponseHeader, ResponseTrailer
- [ConnectRPC Protocol Reference](https://connectrpc.com/docs/protocol/) -- wire format for streaming
- [ConnectRPC Deployment](https://connectrpc.com/docs/go/deployment/) -- timeout semantics for streaming RPCs
- [otelconnect](https://pkg.go.dev/connectrpc.com/otelconnect) -- streaming instrumentation, per-message trace events
- [SQLite WAL Mode](https://www.sqlite.org/wal.html) -- checkpoint blocking by open read transactions
- [Ent CRUD API](https://entgo.io/docs/crud/) -- All(), Limit(), Offset() semantics

### Secondary (MEDIUM confidence)
- [ConnectRPC HTTP/1.1 Streaming Issue #639](https://github.com/connectrpc/connect-go/issues/639) -- server streaming over HTTP/1.1 confirmation
- [WAL Mode in LiteFS](https://fly.io/blog/wal-mode-in-litefs/) -- LiteFS WAL checkpoint behavior
- [ConnectRPC examples-go Eliza Service](https://github.com/connectrpc/examples-go/) -- streaming handler reference implementation
- [WCAG Color Contrast](https://webaim.org/articles/contrast/) -- 4.5:1 minimum for normal text
- [Tailwind CSS User Select](https://tailwindcss.com/docs/user-select) -- select-text/select-none utilities
- Existing codebase: `internal/middleware/logging.go`, `cmd/peeringdb-plus/main.go`, `gen/peeringdb/v1/peeringdbv1connect/services.connect.go`

### Tertiary (LOW confidence)
- [Go HTTP Handlers, Panic, and Deadlocks](https://iximiuz.com/en/posts/go-http-handlers-panic-and-deadlocks/) -- net/http panic recovery behavior in streaming context
- [gRPC-Go Performance Improvements](https://grpc.io/blog/grpc-go-perf-improvements/) -- memory allocation per frame in streaming

---
*Research completed: 2026-03-25*
*Ready for roadmap: yes*
