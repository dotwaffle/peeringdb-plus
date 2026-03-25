# Phase 25 Context: Streaming RPCs

## Decisions

- **RPC naming:** `StreamNetworks`, `StreamOrganizations`, etc. — mirrors `ListNetworks` pattern
- **Service organization:** Add `Stream*` RPCs to existing services (e.g., `StreamNetworks` on `NetworkService`) — one compile-breaking change, use stubs
- **Chunk size:** Hardcoded 500 rows per batch — simple, sufficient for PeeringDB data volumes
- **Stream timeout:** Configurable via `PDBPLUS_STREAM_TIMEOUT` env var, default 60 seconds
- **OTel trace events:** Suppress per-message events on streaming handlers — one span per stream lifecycle, not per row. Use separate otelconnect interceptor with `WithoutTraceEvents` for streaming, keep events on Get/List
- **Total count header:** `grpc-total-count` — gRPC metadata convention
- **Format documentation:** Proto file comments for developers + README section for consumers (STRM-07)

## Implementation Notes

- Proto change breaks all 13 handler interfaces simultaneously — add all 13 `Stream*` RPC definitions and implement stubs before any handler logic
- Batched keyset pagination: `WHERE id > lastID ORDER BY id ASC LIMIT 500` pattern, not `OFFSET`
- Each batch is a separate read transaction (ent default) — avoids blocking SQLite WAL checkpointing
- Recovery middleware cannot handle mid-stream panics (HTTP 200 already committed) — never panic in streaming loops
- ConnectRPC `simple` codegen flag produces: `func(ctx, *connect.Request[Req], *connect.ServerStream[Res]) error`
- Reuse existing predicate accumulation pattern from List handlers for filter support
- `stream.ResponseHeader().Set("grpc-total-count", strconv.Itoa(count))` before first `Send()`

## Existing Code References

- `proto/peeringdb/v1/services.proto` — add `rpc Stream*(Stream*Request) returns (stream *)` to each service
- `internal/grpcserver/` — 13 handler files, add `Stream*` method to each
- `internal/grpcserver/pagination.go` — add batched keyset iteration helper
- `cmd/peeringdb-plus/main.go` — service registration (no changes needed, interface-based)
- `buf.gen.yaml` — already has `simple` flag enabled
