# Phase 25: Streaming RPCs - Research

**Researched:** 2026-03-25
**Domain:** ConnectRPC server-streaming, keyset pagination, protobuf schema extension
**Confidence:** HIGH

## Summary

Phase 25 adds 13 server-streaming RPCs to the existing ConnectRPC services, enabling consumers to stream entire entity tables without manual pagination. The work is primarily additive: proto schema changes (new RPCs + request messages), code generation, a shared batched keyset pagination helper, and 13 handler implementations that follow an identical pattern.

The critical sequence constraint is that adding `rpc Stream*` to each service in `services.proto` breaks all 13 generated handler interfaces simultaneously -- every handler file must gain a stub (or real) `Stream*` method before the project compiles again. The CONTEXT.md locks this approach: add all 13 RPC definitions and stubs first, then implement handler logic.

**Primary recommendation:** Structure implementation as proto+codegen+stubs first (compilation restored), then keyset pagination helper + one handler as reference, then remaining 12 handlers following the template.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **RPC naming:** `StreamNetworks`, `StreamOrganizations`, etc. -- mirrors `ListNetworks` pattern
- **Service organization:** Add `Stream*` RPCs to existing services (e.g., `StreamNetworks` on `NetworkService`) -- one compile-breaking change, use stubs
- **Chunk size:** Hardcoded 500 rows per batch -- simple, sufficient for PeeringDB data volumes
- **Stream timeout:** Configurable via `PDBPLUS_STREAM_TIMEOUT` env var, default 60 seconds
- **OTel trace events:** Suppress per-message events on streaming handlers -- one span per stream lifecycle, not per row. Use separate otelconnect interceptor with `WithoutTraceEvents` for streaming, keep events on Get/List
- **Total count header:** `grpc-total-count` -- gRPC metadata convention
- **Format documentation:** Proto file comments for developers + README section for consumers (STRM-07)

### Implementation Notes (from CONTEXT.md)
- Proto change breaks all 13 handler interfaces simultaneously -- add all 13 `Stream*` RPC definitions and implement stubs before any handler logic
- Batched keyset pagination: `WHERE id > lastID ORDER BY id ASC LIMIT 500` pattern, not `OFFSET`
- Each batch is a separate read transaction (ent default) -- avoids blocking SQLite WAL checkpointing
- Recovery middleware cannot handle mid-stream panics (HTTP 200 already committed) -- never panic in streaming loops
- ConnectRPC `simple` codegen flag produces: `func(ctx, *connect.Request[Req], *connect.ServerStream[Res]) error`
- Reuse existing predicate accumulation pattern from List handlers for filter support
- `stream.ResponseHeader().Set("grpc-total-count", strconv.Itoa(count))` before first `Send()`

### Existing Code References
- `proto/peeringdb/v1/services.proto` -- add `rpc Stream*(Stream*Request) returns (stream *)` to each service
- `internal/grpcserver/` -- 13 handler files, add `Stream*` method to each
- `internal/grpcserver/pagination.go` -- add batched keyset iteration helper
- `cmd/peeringdb-plus/main.go` -- service registration (no changes needed, interface-based)
- `buf.gen.yaml` -- already has `simple` flag enabled

### Deferred Ideas (OUT OF SCOPE)
- STRM-08: `since_id` stream resume (Phase 26)
- STRM-09: `updated_since` filter (Phase 26)
- STRM-10: SHA256 checksum in response trailers
- STRM-11: SyncStatus custom RPC
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| STRM-01 | Server-streaming RPC per entity type -- 13 `Stream*` RPCs returning one proto message per row | Proto schema extension pattern verified; `simple` codegen produces `func(ctx, *Req, *connect.ServerStream[Res]) error` signature |
| STRM-02 | Batched keyset pagination in streaming handlers -- chunk queries by ID to avoid loading full result sets | Ent supports `Where(entity.IDGT(lastID)).Order(Asc(FieldID)).Limit(500)` for keyset pagination; pattern documented in Architecture section |
| STRM-03 | Graceful stream cancellation -- honor `ctx.Done()` between batch fetches | `ctx.Done()` check between batches; ConnectRPC propagates client disconnect via context cancellation |
| STRM-04 | Total record count in response header -- `COUNT(*)` query, set via `stream.ResponseHeader()` before first `Send()` | `ServerStream.ResponseHeader()` confirmed: "Headers are sent with the first call to Send" |
| STRM-05 | Filter support on streaming RPCs -- same optional filter fields as List, reusing predicate accumulation | Existing predicate accumulation pattern (`[]predicate.Entity` + `entity.And()`) reusable from List handlers |
| STRM-06 | OTel instrumentation on streaming RPCs -- otelconnect interceptor produces per-stream spans | `otelconnect.WithoutTraceEvents()` suppresses per-message events; use globally since unary trace events are low-value |
| STRM-07 | Proto/JSON format negotiation -- ConnectRPC handles automatically, document for consumers | ConnectRPC natively supports proto, JSON, and gRPC-Web on same endpoint; document `Content-Type` header usage |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **CS-0 (MUST):** Modern Go code guidelines
- **CS-5 (MUST):** Input structs for functions receiving more than 2 arguments
- **ERR-1 (MUST):** Wrap errors with `%w` and context
- **CC-2 (MUST):** Tie goroutine lifetime to `context.Context`
- **CTX-1 (MUST):** `ctx context.Context` as first parameter
- **T-1 (MUST):** Table-driven tests
- **T-2 (MUST):** Run `-race` in CI; `t.Cleanup` for teardown
- **T-3 (SHOULD):** Mark safe tests with `t.Parallel()`
- **OBS-1 (MUST):** Structured logging with slog
- **CFG-1 (MUST):** Config via env/flags; validate on startup; fail fast
- **CFG-2 (MUST):** Config immutable after init
- **API-1 (MUST):** Document exported items
- **TL-4 (CAN):** `buf` for Protobuf; codegen conventions

**Middleware convention:** Response writer wrappers MUST implement `http.Flusher` -- already done in `internal/middleware/logging.go`.

**Codegen convention:** `buf generate` regenerates proto Go types. Generated output: `gen/peeringdb/v1/`. Proto files: `proto/peeringdb/v1/services.proto`.

## Architecture Patterns

### Recommended Implementation Structure

```
proto/peeringdb/v1/
  services.proto           # Add 13 Stream* RPCs + 13 Stream*Request messages

gen/peeringdb/v1/          # Regenerated by buf generate
  peeringdbv1connect/
    services.connect.go    # New handler interface methods + Unimplemented stubs

internal/grpcserver/
  pagination.go            # Add streamBatched() keyset iteration helper
  network.go               # Add StreamNetworks() method
  facility.go              # Add StreamFacilities() method
  ... (11 more)            # Add Stream* to each handler file

internal/config/
  config.go                # Add StreamTimeout field (PDBPLUS_STREAM_TIMEOUT)

cmd/peeringdb-plus/
  main.go                  # OTel interceptor change: WithoutTraceEvents() globally
```

### Pattern 1: Proto Schema for Server-Streaming RPC

**What:** Each service gains a `Stream*` RPC that returns a `stream` of the entity message.
**When to use:** All 13 entity types follow this identical pattern.

```protobuf
// In services.proto, for each service (e.g., NetworkService):
service NetworkService {
  rpc GetNetwork(GetNetworkRequest) returns (GetNetworkResponse);
  rpc ListNetworks(ListNetworksRequest) returns (ListNetworksResponse);
  // NEW: Server-streaming RPC -- streams all matching rows one at a time.
  rpc StreamNetworks(StreamNetworksRequest) returns (stream Network);
}

// Stream request mirrors List filters but omits pagination fields.
message StreamNetworksRequest {
  // Filter fields -- all optional for presence detection.
  optional int64 asn = 1;
  optional string name = 2;
  optional string status = 3;
  optional int64 org_id = 4;
}
```

**Key design choices:**
- Returns `stream Network` (single entity), not `stream ListNetworksResponse` (batch)
- Request message has filter fields only -- no `page_size`/`page_token` (streaming handles iteration)
- Field numbers start at 1 (independent message, not sharing field space with List request)

### Pattern 2: ConnectRPC Simple Handler Signature for Server Streaming

**What:** The `simple` codegen flag produces simplified handler signatures.
**Verified via:** `go doc connectrpc.com/connect NewServerStreamHandlerSimple`

```go
// Generated handler interface (with simple flag):
type NetworkServiceHandler interface {
    GetNetwork(context.Context, *pb.GetNetworkRequest) (*pb.GetNetworkResponse, error)
    ListNetworks(context.Context, *pb.ListNetworksRequest) (*pb.ListNetworksResponse, error)
    StreamNetworks(context.Context, *pb.StreamNetworksRequest, *connect.ServerStream[pb.Network]) error  // NEW
}
```

**NOTE on simple flag:** The CONTEXT.md states the signature is `func(ctx, *connect.Request[Req], *connect.ServerStream[Res]) error`. However, verified via `go doc`, the `simple` variant is `func(ctx, *Req, *connect.ServerStream[Res]) error` -- it eliminates the `*connect.Request` wrapper. The existing handler code confirms this: current handlers use `func(ctx context.Context, req *pb.GetNetworkRequest)` not `func(ctx, *connect.Request[...])`. The simple mode unwraps the request.

### Pattern 3: Batched Keyset Pagination Helper

**What:** A generic helper function that iterates through all matching records in fixed-size batches using keyset pagination.
**Why keyset over OFFSET:** OFFSET-based pagination degrades with depth (must skip N rows). Keyset uses `WHERE id > lastID` which is O(1) via index seek.

```go
// streamBatched iterates an ent query in keyset-paginated batches,
// calling sendFn for each record. Returns on context cancellation or
// when all records are exhausted.
//
// batchSize is the number of records fetched per database query.
// The caller provides queryFn which builds the filtered, ordered query
// starting after lastID with the given limit.
func streamBatched[T interface{ GetId() int64 }](
    ctx context.Context,
    batchSize int,
    queryFn func(ctx context.Context, lastID int, limit int) ([]T, error),
    sendFn func(T) error,
) error {
    lastID := 0
    for {
        // Check for cancellation before each batch.
        if err := ctx.Err(); err != nil {
            return err
        }

        batch, err := queryFn(ctx, lastID, batchSize)
        if err != nil {
            return fmt.Errorf("query batch after id %d: %w", lastID, err)
        }
        if len(batch) == 0 {
            return nil // Exhausted all records.
        }

        for _, item := range batch {
            if err := sendFn(item); err != nil {
                return err // Client disconnected or stream error.
            }
        }

        lastID = int(batch[len(batch)-1].GetId())

        if len(batch) < batchSize {
            return nil // Last batch was partial -- no more records.
        }
    }
}
```

**IMPORTANT:** The generic constraint `interface{ GetId() int64 }` does NOT work directly because ent entities have `int` IDs, not proto messages with `GetId() int64`. Two approaches:

**Option A (recommended):** Make the helper non-generic, operating on `int` IDs. The handler converts ent -> proto inside the loop:
```go
func streamBatched(
    ctx context.Context,
    batchSize int,
    queryFn func(ctx context.Context, afterID int, limit int) ([]int, int, error),
    // queryFn returns: count of items processed, last ID seen, error
) error
```

**Option B:** Have each handler implement its own loop using a shared `const streamBatchSize = 500` and the keyset pattern. Given that the loop is ~15 lines, this may be clearer than a generic helper that obscures the control flow.

**Recommendation:** Option B (explicit loops) is simpler for 13 handlers that each need slightly different query construction. A shared constant and a documented pattern is sufficient. The helper adds abstraction without reducing code volume significantly, since each handler's query differs.

### Pattern 4: Stream Handler Implementation

**What:** The actual `StreamNetworks` handler method.

```go
// streamBatchSize is the number of rows fetched per database round-trip
// during streaming RPCs.
const streamBatchSize = 500

func (s *NetworkService) StreamNetworks(
    ctx context.Context,
    req *pb.StreamNetworksRequest,
    stream *connect.ServerStream[pb.Network],
) error {
    // Build filter predicates (reuse List handler pattern).
    var predicates []predicate.Network
    if req.Asn != nil {
        if *req.Asn <= 0 {
            return connect.NewError(connect.CodeInvalidArgument,
                fmt.Errorf("invalid filter: asn must be positive"))
        }
        predicates = append(predicates, network.AsnEQ(int(*req.Asn)))
    }
    // ... other filters identical to ListNetworks ...

    // Count total matching records for header metadata.
    countQuery := s.Client.Network.Query()
    if len(predicates) > 0 {
        countQuery = countQuery.Where(network.And(predicates...))
    }
    total, err := countQuery.Count(ctx)
    if err != nil {
        return connect.NewError(connect.CodeInternal,
            fmt.Errorf("count networks: %w", err))
    }
    stream.ResponseHeader().Set("grpc-total-count", strconv.Itoa(total))

    // Stream records in batches using keyset pagination.
    lastID := 0
    for {
        if err := ctx.Err(); err != nil {
            return err
        }

        query := s.Client.Network.Query().
            Where(network.IDGT(lastID)).
            Order(ent.Asc(network.FieldID)).
            Limit(streamBatchSize)
        if len(predicates) > 0 {
            query = query.Where(network.And(predicates...))
        }

        batch, err := query.All(ctx)
        if err != nil {
            return connect.NewError(connect.CodeInternal,
                fmt.Errorf("stream networks batch after id %d: %w", lastID, err))
        }
        if len(batch) == 0 {
            return nil
        }

        for _, n := range batch {
            if err := stream.Send(networkToProto(n)); err != nil {
                return err
            }
        }

        lastID = batch[len(batch)-1].ID
        if len(batch) < streamBatchSize {
            return nil
        }
    }
}
```

### Pattern 5: OTel Interceptor Configuration

**What:** Suppress per-message trace events for streaming RPCs.
**Decision note:** The CONTEXT.md says "Use separate otelconnect interceptor with `WithoutTraceEvents` for streaming, keep events on Get/List." However, since streaming and unary RPCs live in the same service (same handler registration), we cannot apply different interceptors per-RPC within the same service handler.

**Practical approach:** Apply `WithoutTraceEvents()` globally. Unary RPC trace events (one request + one response per call) add minimal value. The important thing is that streaming RPCs (500+ messages) do not generate 500+ trace events.

```go
// In main.go: replace current interceptor creation
otelInterceptor, err := otelconnect.NewInterceptor(
    otelconnect.WithoutServerPeerAttributes(),
    otelconnect.WithoutTraceEvents(), // Suppress per-message events (critical for streaming)
)
```

**Alternative (if per-RPC distinction is truly needed):** Use `otelconnect.WithFilter()` to create two interceptors -- one with events for unary RPCs, one without for streaming. But this would require checking `Spec.StreamType` in the filter function, and more importantly, `WithFilter` controls whether any instrumentation is emitted at all, not just events. A custom wrapper interceptor would be needed for true per-RPC control. This is not worth the complexity for this use case.

### Anti-Patterns to Avoid

- **Loading full result sets with `.All(ctx)` without limit:** Always use `.Limit(streamBatchSize)` in the batch query. Never call `.All(ctx)` on an unbounded query during streaming.
- **Using OFFSET pagination for streaming:** OFFSET degrades at scale. Keyset (`WHERE id > lastID`) is O(1) per batch via index.
- **Panicking in streaming loops:** Recovery middleware cannot catch panics after HTTP 200 is sent. The response status is committed when the first `Send()` calls `ResponseHeader()`. Use error returns only.
- **Holding a single long transaction across all batches:** Each batch query should be its own transaction (ent default). Long-lived read transactions block SQLite WAL checkpointing.
- **Ignoring context cancellation between batches:** Always check `ctx.Err()` before starting a new batch query. Without this, a disconnected client causes the server to continue querying until the table is exhausted.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Server streaming transport | Custom HTTP chunked streaming | ConnectRPC `ServerStream` | Handles proto/JSON serialization, gRPC framing, HTTP/2 flow control, client cancellation |
| Proto codegen | Manual handler interfaces | `buf generate` with `protoc-gen-connect-go` | Generates type-safe handler interfaces, client stubs, and `Unimplemented*` embed types |
| Request/response headers | Custom header injection middleware | `stream.ResponseHeader()` / `stream.ResponseTrailer()` | Built into ConnectRPC; headers sent with first `Send()` |
| OTel instrumentation | Manual span creation | `otelconnect.NewInterceptor` | Automatic span lifecycle, metrics, semantic conventions |
| Keyset pagination index | Custom query planner | SQLite `INTEGER PRIMARY KEY` index | Ent entity IDs are SQLite rowids; `WHERE id > X ORDER BY id LIMIT N` uses the built-in B-tree |

## Common Pitfalls

### Pitfall 1: Compilation Break from Interface Change
**What goes wrong:** Adding `Stream*` RPCs to services.proto and running `buf generate` breaks all 13 handler files because each now must implement the new interface method.
**Why it happens:** ConnectRPC generates handler interfaces that all implementations must satisfy. Adding a method to the proto service adds a method to the Go interface.
**How to avoid:** Add all 13 `Stream*` RPCs to the proto, run `buf generate`, then immediately add stub implementations (embedding `Unimplemented*Handler` or explicit stubs returning `connect.CodeUnimplemented`) to all 13 handler files before attempting to compile.
**Warning signs:** `does not implement` compiler errors referencing `StreamX` methods.

### Pitfall 2: HTTP 200 Already Sent -- Cannot Change Status
**What goes wrong:** After the first `stream.Send()`, the HTTP status 200 is committed. Any subsequent error cannot change it to 500.
**Why it happens:** HTTP streaming sends headers (including status) with the first data frame. gRPC/ConnectRPC encode errors in trailers instead.
**How to avoid:** Validate inputs and run the count query BEFORE the first `Send()`. Errors during the stream are encoded in trailers by ConnectRPC automatically. Never `panic` -- use error returns.
**Warning signs:** Client sees HTTP 200 but receives an error in the trailer/stream-end frame.

### Pitfall 3: Memory Pressure from Large Batches
**What goes wrong:** If `batchSize` is too large or if proto conversion allocates heavily, memory spikes during streaming.
**Why it happens:** Each batch of 500 ent entities is held in memory, then each is converted to a proto message. The 500 ent entities + 500 proto messages coexist briefly.
**How to avoid:** The 500 batch size is appropriate for PeeringDB data volumes (largest table ~300K rows). Each batch processes sequentially, releasing the previous batch before allocating the next.
**Warning signs:** RSS growth proportional to total streamed rows rather than batch size.

### Pitfall 4: Count Query Races with Streaming
**What goes wrong:** The `COUNT(*)` query and the streaming batches are separate transactions. Records may be inserted/deleted between count and stream.
**Why it happens:** SQLite WAL mode allows concurrent reads. Sync worker may update data during a stream.
**How to avoid:** Accept eventual consistency. The count is a best-effort hint. Document that `grpc-total-count` is approximate and may not match the actual number of streamed messages.
**Warning signs:** Client receives fewer or more messages than `grpc-total-count` indicated.

### Pitfall 5: Stream Timeout Too Aggressive
**What goes wrong:** A full-table stream of ~300K rows with 500-row batches = 600 batches. At ~10ms per batch, that is ~6 seconds. But with large entity types (Network has 40+ fields), serialization adds up.
**Why it happens:** Default timeout of 60 seconds may not be enough for very large filtered result sets over slow connections where client backpressure slows `Send()`.
**How to avoid:** The 60-second default is a per-stream timeout, not per-message. Make it configurable via `PDBPLUS_STREAM_TIMEOUT`. For context: PeeringDB NetworkIxLan has ~300K rows. At 500 per batch with ~10ms per batch, a full stream takes ~6 seconds. 60 seconds provides 10x headroom.
**Warning signs:** Streams being terminated before completion on slow networks.

### Pitfall 6: Fly.io HTTP/1.1 Proxy and Streaming
**What goes wrong:** Fly.io's proxy may buffer or timeout long-lived HTTP/2 streams on the edge.
**Why it happens:** Fly.io uses a proxy layer between the client and the application. HTTP/2 streams need end-to-end support.
**How to avoid:** Fly.io supports HTTP/2 and h2c natively. The application already enables h2c via `http.Protocols`. Monitor for proxy timeout issues (Fly.io default is 60 seconds for idle connections). The stream is not idle -- data flows continuously -- so this should not be an issue.
**Warning signs:** Streams terminating at exactly 60 seconds regardless of data flow rate.

## Code Examples

### Example 1: Stream Request Proto Message Pattern

All 13 entities follow this pattern. Filter fields mirror the List request but omit pagination.

```protobuf
// Source: services.proto pattern (to be added)
// StreamNetworksRequest filters for the StreamNetworks server-streaming RPC.
// All filter fields are optional; omitted fields impose no constraint.
message StreamNetworksRequest {
  optional int64 asn = 1;
  optional string name = 2;
  optional string status = 3;
  optional int64 org_id = 4;
}
```

Entity types and their filter fields (matching existing List requests):

| Entity | Filter Fields |
|--------|--------------|
| Campus | name, country, city, status, org_id |
| Carrier | name, status, org_id |
| CarrierFacility | carrier_id, fac_id, status |
| Facility | name, country, city, status, org_id |
| InternetExchange | name, country, city, status, org_id |
| IxFacility | ix_id, fac_id, country, city, status |
| IxLan | ix_id, name, status |
| IxPrefix | ixlan_id, protocol, status |
| Network | asn, name, status, org_id |
| NetworkFacility | net_id, fac_id, country, city, status |
| NetworkIxLan | net_id, ixlan_id, asn, name, status |
| Organization | name, country, city, status |
| Poc | net_id, role, name, status |

### Example 2: Ent Keyset Query

```go
// Source: verified via ent API (entity.IDGT exists on all ent types)
query := s.Client.Network.Query().
    Where(network.IDGT(lastID)).
    Order(ent.Asc(network.FieldID)).
    Limit(streamBatchSize)
if len(predicates) > 0 {
    query = query.Where(network.And(predicates...))
}
batch, err := query.All(ctx)
```

### Example 3: Config Extension

```go
// In internal/config/config.go
type Config struct {
    // ... existing fields ...

    // StreamTimeout is the maximum duration for a single streaming RPC.
    // Configured via PDBPLUS_STREAM_TIMEOUT. Default is 60 seconds.
    StreamTimeout time.Duration
}

// In Load():
streamTimeout, err := parseDuration("PDBPLUS_STREAM_TIMEOUT", 60*time.Second)
if err != nil {
    return nil, fmt.Errorf("parsing PDBPLUS_STREAM_TIMEOUT: %w", err)
}
cfg.StreamTimeout = streamTimeout
```

### Example 4: Applying Stream Timeout in Handler

The stream timeout needs to be applied at the handler level, not the transport level. ConnectRPC does not have a built-in per-RPC timeout for handlers. The approach is to derive a deadline from the context at stream start.

```go
// The StreamTimeout is passed to each service struct.
type NetworkService struct {
    Client        *ent.Client
    StreamTimeout time.Duration
}

func (s *NetworkService) StreamNetworks(
    ctx context.Context,
    req *pb.StreamNetworksRequest,
    stream *connect.ServerStream[pb.Network],
) error {
    // Apply stream timeout.
    if s.StreamTimeout > 0 {
        var cancel context.CancelFunc
        ctx, cancel = context.WithTimeout(ctx, s.StreamTimeout)
        defer cancel()
    }
    // ... rest of handler
}
```

### Example 5: Buf Generate Command

```bash
# Regenerate proto Go types and ConnectRPC handlers
TMPDIR=/tmp/claude-1000 go run github.com/bufbuild/buf/cmd/buf@latest generate
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `protoc` direct invocation | `buf generate` with managed plugins | Phase 22 (v1.6) | Proto toolchain uses buf; `go tool` directive manages protoc-gen-* |
| `connect.NewUnaryHandler` (request wrapper) | `connect.NewUnaryHandlerSimple` | ConnectRPC `simple` codegen flag | Handler signatures use `(ctx, *Req)` not `(ctx, *connect.Request[Req])` |
| Standard gRPC server | ConnectRPC | Phase 22 (v1.6) | Handlers are `http.Handler`, mount on stdlib mux |
| OFFSET pagination (List RPCs) | OFFSET pagination for List, keyset for Stream | This phase | List RPCs keep OFFSET (backward compat); streaming uses keyset (performance) |

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) |
| Config file | None needed |
| Quick run command | `TMPDIR=/tmp/claude-1000 go test ./internal/grpcserver/... -count=1 -race` |
| Full suite command | `TMPDIR=/tmp/claude-1000 go test ./... -count=1 -race` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| STRM-01 | 13 Stream* RPCs return streamed messages | integration | `go test ./internal/grpcserver/... -run TestStreamNetworks -race` | No -- Wave 0 |
| STRM-02 | Batched keyset pagination (memory bounded) | unit | `go test ./internal/grpcserver/... -run TestStreamBatched -race` | No -- Wave 0 |
| STRM-03 | Context cancellation terminates stream | integration | `go test ./internal/grpcserver/... -run TestStreamCancellation -race` | No -- Wave 0 |
| STRM-04 | Total count in response header | integration | `go test ./internal/grpcserver/... -run TestStreamTotalCount -race` | No -- Wave 0 |
| STRM-05 | Filter fields on streaming RPCs | integration | `go test ./internal/grpcserver/... -run TestStreamFilters -race` | No -- Wave 0 |
| STRM-06 | OTel instrumentation | unit | `go test ./cmd/peeringdb-plus/... -run TestOTelConfig -race` | No -- Wave 0 (config only, OTel tested via interceptor presence) |
| STRM-07 | Proto/JSON format negotiation docs | manual | N/A -- verify proto comments exist, README section present | No -- Wave 0 |

### Sampling Rate
- **Per task commit:** `TMPDIR=/tmp/claude-1000 go test ./internal/grpcserver/... -count=1 -race`
- **Per wave merge:** `TMPDIR=/tmp/claude-1000 go test ./... -count=1 -race`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] Stream handler tests in `internal/grpcserver/grpcserver_test.go` -- covers STRM-01, STRM-02, STRM-03, STRM-04, STRM-05
- [ ] Config test for `PDBPLUS_STREAM_TIMEOUT` in `internal/config/config_test.go` -- covers STRM-06

**Testing streaming handlers without HTTP server:** The existing test pattern calls handler methods directly (`svc.StreamNetworks(ctx, req, stream)`) -- but `*connect.ServerStream` cannot be constructed outside ConnectRPC internals. Two approaches:

1. **In-process HTTP test server:** Use `httptest.NewServer` with ConnectRPC handler, create a ConnectRPC client, call the streaming RPC. This tests the full stack including serialization.

2. **Mock ServerStream:** Not directly possible since `ServerStream` has no exported constructor. ConnectRPC's `Conn()` method exposes the underlying `StreamingHandlerConn` interface which could theoretically be mocked, but this is fragile.

**Recommendation:** Use approach 1 (in-process HTTP server) for streaming tests. This is how ConnectRPC's own tests work. Create a test helper that starts a server with the handler and returns a typed client.

```go
// Test helper pattern for streaming RPCs:
func setupStreamTest(t *testing.T) peeringdbv1connect.NetworkServiceClient {
    t.Helper()
    client := testutil.SetupClient(t)
    svc := &NetworkService{Client: client, StreamTimeout: 30 * time.Second}
    mux := http.NewServeMux()
    mux.Handle(peeringdbv1connect.NewNetworkServiceHandler(svc))
    srv := httptest.NewUnstartedServer(mux)
    srv.EnableHTTP2 = true
    srv.StartTLS()
    t.Cleanup(srv.Close)
    return peeringdbv1connect.NewNetworkServiceClient(
        srv.Client(),
        srv.URL,
    )
}
```

**Note:** The `simple` codegen flag affects handler signatures but ConnectRPC client still uses the standard `*connect.ServerStreamForClient[Res]` type for receiving streamed messages. Client-side streaming consumption uses:
```go
stream, err := client.StreamNetworks(ctx, &pb.StreamNetworksRequest{})
// stream is *connect.ServerStreamForClient[pb.Network]
for stream.Receive() {
    msg := stream.Msg()
    // process msg
}
if err := stream.Err(); err != nil {
    // handle error
}
```

## Open Questions

1. **Fly.io HTTP/2 proxy timeout for streaming**
   - What we know: Fly.io supports HTTP/2. The app already uses h2c. Default Fly.io proxy idle timeout is 60 seconds.
   - What's unclear: Whether continuous data flow (no idle period) prevents proxy timeout. Fly.io docs are sparse on long-lived streaming specifics.
   - Recommendation: Implement and test in deployment. The 60-second stream timeout matches proxy timeout, providing natural alignment. If proxy kills streams, increase proxy timeout via Fly.io configuration or reduce batch pause.

2. **buf lint compliance for Stream* RPCs**
   - What we know: `buf lint` passes on current protos. Standard lint rules apply.
   - What's unclear: Whether `STANDARD` lint rules have opinions about server-streaming RPC naming or request message conventions.
   - Recommendation: Run `buf lint` after adding the stream RPCs. If lint fails, adjust naming. The `PACKAGE_VERSION_SUFFIX` exception is already in place.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | All code | Yes | 1.26.1 | -- |
| buf (via go run) | Proto codegen | Yes | latest (go run) | -- |
| protoc-gen-go | Proto codegen | Yes | go tool directive | -- |
| protoc-gen-connect-go | ConnectRPC codegen | Yes | go tool directive | -- |
| SQLite (modernc.org) | Database queries | Yes | go dependency | -- |

**Missing dependencies with no fallback:** None.
**Missing dependencies with fallback:** None.

## Sources

### Primary (HIGH confidence)
- `go doc connectrpc.com/connect ServerStream` -- verified `ResponseHeader()` sent with first `Send()`, `ResponseTrailer()` available at any time
- `go doc connectrpc.com/connect NewServerStreamHandlerSimple` -- verified simple mode signature: `func(ctx, *Req, *ServerStream[Res]) error`
- `go doc connectrpc.com/otelconnect WithoutTraceEvents` -- confirmed: disables per-message trace events for both unary and streaming
- `go doc connectrpc.com/connect Spec` -- confirmed `StreamType` field for detecting streaming RPCs
- Existing codebase: `internal/grpcserver/network.go`, `services.proto`, `services.connect.go` -- verified current handler patterns and interfaces

### Secondary (MEDIUM confidence)
- [ConnectRPC Streaming Docs](https://connectrpc.com/docs/go/streaming/) -- streaming best practices, limitations
- [ConnectRPC Headers & Trailers](https://connectrpc.com/docs/go/headers-and-trailers/) -- header/trailer API patterns

### Tertiary (LOW confidence)
- Fly.io HTTP/2 proxy behavior with long-lived streams -- sparse documentation, needs runtime verification

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all libraries already in use, versions pinned in go.mod
- Architecture: HIGH -- existing handler pattern is well-understood, streaming is additive
- Pitfalls: HIGH -- verified via ConnectRPC docs and codebase analysis (Flusher, recovery, WAL)
- Testing strategy: MEDIUM -- streaming test helper pattern needs validation (httptest approach)

**Research date:** 2026-03-25
**Valid until:** 2026-04-25 (stable domain, no expected breaking changes)
