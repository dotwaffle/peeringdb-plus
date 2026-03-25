# Architecture Patterns

**Domain:** Server-streaming RPCs + UI polish for PeeringDB Plus v1.7
**Researched:** 2026-03-25

## Recommended Architecture

### Overview

Two independent feature tracks that share no code dependencies between them:

1. **Server-streaming RPCs**: New `StreamAll` methods added to each of the 13 existing ConnectRPC services, using the same handler struct pattern but with streaming return semantics.
2. **IX presence UI polish**: Template-only changes to `detail_net.templ` and `detail_shared.templ` (and the IX detail equivalents), plus CSS adjustments. No new routes, no new Go handler code.

The streaming feature integrates surgically: add RPCs to `services.proto`, regenerate, implement handler methods, register (already handled by existing service registration). The UI changes are purely visual refinements to existing components.

### Component Boundaries

| Component | Responsibility | What Changes | Confidence |
|-----------|---------------|--------------|------------|
| `proto/peeringdb/v1/services.proto` | Service + message definitions | ADD: 13 `StreamAll*` RPCs + 13 request messages | HIGH |
| `gen/peeringdb/v1/peeringdbv1connect/` | Generated handler interfaces + clients | REGENERATED: interfaces gain streaming method signatures | HIGH |
| `internal/grpcserver/*.go` (13 files) | Handler implementations | MODIFIED: each gains a `StreamAll*` method | HIGH |
| `internal/grpcserver/pagination.go` | Pagination helpers | ADD: streaming batch size constant | HIGH |
| `cmd/peeringdb-plus/main.go` | Service registration | NO CHANGE: streaming methods are on the same service interfaces, same registration code | HIGH |
| `connectrpc.com/grpcreflect` | gRPC reflection | NO CHANGE: same service names, reflection auto-discovers new methods | HIGH |
| `internal/web/templates/detail_net.templ` | Network IX presence rendering | MODIFIED: layout, labels, badges, CSS classes | HIGH |
| `internal/web/templates/detail_shared.templ` | Shared helpers (formatSpeed) | MODIFIED: add speed color helper, possibly new components | HIGH |
| `internal/web/templates/detail_ix.templ` | IX participant rendering | MODIFIED: same layout changes for consistency | HIGH |
| `internal/web/templates/detailtypes.go` | Template data types | NO CHANGE: existing `NetworkIXLanRow` and `IXParticipantRow` structs already carry all needed data | HIGH |
| `internal/web/detail.go` | Fragment handlers | NO CHANGE: data loading is unchanged, only presentation changes | HIGH |

### Data Flow: Server-Streaming RPCs

```
Client sends StreamAllNetworksRequest (no page_token, just optional filters)
    |
    v
Handler receives request + *connect.ServerStream[StreamAllNetworksResponse]
    |
    v
Handler builds ent query with predicates (reuse existing filter logic)
    |
    v
Loop: query batch of 500 entities via .Limit(batchSize).Offset(cursor)
    |
    +-> For each entity in batch:
    |       Convert to proto (reuse existing entityToProto function)
    |       stream.Send(&StreamAllNetworksResponse{Network: proto})
    |       If Send returns error -> return error (client disconnected)
    |
    +-> If len(batch) < batchSize -> break (no more data)
    |   Else cursor += batchSize, loop again
    |
    v
Return nil (stream ends, ConnectRPC sends trailers)
```

**Key architectural decision:** Each `Send()` emits exactly one entity. This is deliberate -- streaming one entity per message gives the client maximum flexibility for processing (they can count, filter client-side, or write to storage as they go). The alternative of batching multiple entities per message adds complexity for marginal wire efficiency gains that protobuf framing already handles well.

### Data Flow: UI Template Changes

No data flow changes. The existing `handleNetIXLansFragment` handler loads `NetworkIxLan` entities and maps them to `NetworkIXLanRow` structs. The template receives the same data; only the HTML rendering changes:

```
Existing: row -> [IXName] [speed ipv4 ipv6] [RS]
New:      row -> [IXName] [Speed: 10G] [IPv4: x.x.x.x] [IPv6: x::x] [RS badge near data] [color-coded speed]
```

The `NetworkIXLanRow` struct already contains `Speed`, `IPAddr4`, `IPAddr6`, and `IsRSPeer` -- all fields needed for the UI improvements. No new data fetching required.

## Integration Points

### 1. Proto File Changes (services.proto)

**What:** Add a `StreamAll*` server-streaming RPC to each of the 13 services, plus corresponding request/response messages.

**Pattern for each service** (using NetworkService as example):

```protobuf
service NetworkService {
  rpc GetNetwork(GetNetworkRequest) returns (GetNetworkResponse);
  rpc ListNetworks(ListNetworksRequest) returns (ListNetworksResponse);
  rpc StreamAllNetworks(StreamAllNetworksRequest) returns (stream Network);  // NEW
}

// NEW: No pagination fields -- streaming replaces pagination.
message StreamAllNetworksRequest {
  // Filter fields -- same as ListNetworksRequest minus page_size/page_token.
  optional int64 asn = 1;
  optional string name = 2;
  optional string status = 3;
  optional int64 org_id = 4;
}
```

**Critical design decision:** The streaming response is `stream Network` (the raw entity message), NOT `stream StreamAllNetworksResponse` wrapping it. Rationale:

- The entity proto messages (`Network`, `Facility`, etc.) already exist in `v1.proto`
- No need for a wrapper when streaming one entity per message
- Reduces proto file verbosity (13 fewer wrapper message types)
- Client code is simpler: `for msg := stream.Receive(); ...` gives them the entity directly

**Generated handler interface change:**

```go
// Current (v1.6):
type NetworkServiceHandler interface {
    GetNetwork(context.Context, *v1.GetNetworkRequest) (*v1.GetNetworkResponse, error)
    ListNetworks(context.Context, *v1.ListNetworksRequest) (*v1.ListNetworksResponse, error)
}

// After adding streaming (v1.7) -- simple mode:
type NetworkServiceHandler interface {
    GetNetwork(context.Context, *v1.GetNetworkRequest) (*v1.GetNetworkResponse, error)
    ListNetworks(context.Context, *v1.ListNetworksRequest) (*v1.ListNetworksResponse, error)
    StreamAllNetworks(context.Context, *v1.StreamAllNetworksRequest, *connect.ServerStream[v1.Network]) error
}
```

The `simple` codegen option (configured in `buf.gen.yaml`) produces the handler signature `func(context.Context, *Req, *connect.ServerStream[Res]) error` rather than `func(context.Context, *connect.Request[Req], *connect.ServerStream[Res]) error`. This is consistent with the existing unary signatures which use `*Req` directly.

**Generated handler constructor change:** `NewNetworkServiceHandler` will internally create the streaming handler via `connect.NewServerStreamHandlerSimple` and add a case to the path switch:

```go
// Generated code will include:
networkServiceStreamAllNetworksHandler := connect.NewServerStreamHandlerSimple(
    NetworkServiceStreamAllNetworksProcedure,
    svc.StreamAllNetworks,
    connect.WithSchema(...),
    connect.WithHandlerOptions(opts...),
)
// ... in the switch:
case NetworkServiceStreamAllNetworksProcedure:
    networkServiceStreamAllNetworksHandler.ServeHTTP(w, r)
```

**Confidence:** HIGH -- verified against ConnectRPC v1.19.1 pkg.go.dev docs and the connectrpc/examples-go Eliza service which uses identical patterns.

### 2. Handler Implementation Pattern

**What:** Each of the 13 handler structs (e.g., `NetworkService`) gains a `StreamAllNetworks` method.

**Pattern:**

```go
const streamBatchSize = 500

func (s *NetworkService) StreamAllNetworks(
    ctx context.Context,
    req *pb.StreamAllNetworksRequest,
    stream *connect.ServerStream[pb.Network],
) error {
    // Build filter predicates -- reuse same pattern as ListNetworks.
    var predicates []predicate.Network
    if req.Asn != nil {
        if *req.Asn <= 0 {
            return connect.NewError(connect.CodeInvalidArgument,
                fmt.Errorf("invalid filter: asn must be positive"))
        }
        predicates = append(predicates, network.AsnEQ(int(*req.Asn)))
    }
    // ... other filters identical to ListNetworks ...

    // Stream all matching entities in batches.
    offset := 0
    for {
        query := s.Client.Network.Query().
            Order(ent.Asc(network.FieldID)).
            Limit(streamBatchSize).
            Offset(offset)
        if len(predicates) > 0 {
            query = query.Where(network.And(predicates...))
        }

        results, err := query.All(ctx)
        if err != nil {
            return connect.NewError(connect.CodeInternal,
                fmt.Errorf("stream networks at offset %d: %w", offset, err))
        }

        for _, n := range results {
            if err := stream.Send(networkToProto(n)); err != nil {
                return err // Client disconnected or context cancelled.
            }
        }

        if len(results) < streamBatchSize {
            break // No more data.
        }
        offset += streamBatchSize
    }
    return nil
}
```

**Key reuse points:**
- Filter predicate building is identical to `ListNetworks` -- extract into a shared helper
- `networkToProto` conversion function is reused as-is
- `streamBatchSize` is a package-level constant in `pagination.go`

**Confidence:** HIGH -- the `ServerStream.Send()` API is well-documented, and the ent `All()` with `Limit`/`Offset` is the existing proven pattern.

### 3. Main.go Registration (NO CHANGE)

**What:** No changes needed to `cmd/peeringdb-plus/main.go`.

The existing registration code:
```go
registerService(peeringdbv1connect.NewNetworkServiceHandler(
    &grpcserver.NetworkService{Client: entClient}, handlerOpts))
```

This already passes the full `*grpcserver.NetworkService` struct. Since the struct gains the new `StreamAllNetworks` method, it automatically satisfies the expanded `NetworkServiceHandler` interface. The generated `NewNetworkServiceHandler` constructor handles routing to the new method internally.

**Confidence:** HIGH -- this is the standard ConnectRPC service extension pattern.

### 4. gRPC Reflection + Health (NO CHANGE)

The static reflector uses service names (e.g., `peeringdb.v1.NetworkService`), not method names. Adding methods to existing services does not change the service name, so reflection automatically discovers the new streaming RPCs. Health check status is per-service, not per-method.

**Confidence:** HIGH -- verified by examining the `grpcreflect.NewStaticReflector` call.

### 5. Middleware Compatibility

The existing `responseWriter` in `internal/middleware/logging.go` already implements `http.Flusher` (added in v1.6 specifically for gRPC streaming):

```go
func (rw *responseWriter) Flush() {
    if f, ok := rw.ResponseWriter.(http.Flusher); ok {
        f.Flush()
    }
}
```

The `Unwrap()` method is also present. Server-streaming works over the existing middleware stack without changes.

**Confidence:** HIGH -- verified in existing codebase, explicitly documented as "Required for gRPC streaming".

### 6. h2c / HTTP/2 Support (NO CHANGE)

Server-streaming requires HTTP/2 (or HTTP/1.1 with chunked transfer for Connect protocol). The server already configures h2c:

```go
var protocols http.Protocols
protocols.SetHTTP1(true)
protocols.SetUnencryptedHTTP2(true)
```

ConnectRPC server-streaming works over both HTTP/2 (for gRPC/gRPC-Web clients) and HTTP/1.1 (for Connect protocol clients using chunked encoding). No protocol changes needed.

**Confidence:** HIGH -- h2c was specifically added in v1.6 for gRPC support.

### 7. UI Template Changes (Isolated)

**Files modified:**
- `internal/web/templates/detail_net.templ` -- `NetworkIXLansList` component
- `internal/web/templates/detail_shared.templ` -- `formatSpeed` helper, possibly new speed-color helper
- `internal/web/templates/detail_ix.templ` -- `IXParticipantsList` component (same changes for consistency)

**No files created.** All changes are within existing components.

**Changes within `NetworkIXLansList`:**
1. Add field labels: `Speed:`, `IPv4:`, `IPv6:` before the respective values
2. Reposition RS badge: currently far-right of row, move to inline near the data fields
3. Color-coded port speeds: map speed ranges to Tailwind color classes (e.g., 100G+ = emerald, 10G = sky, 1G = amber, < 1G = neutral)
4. Consistent IP address indentation: use grid or flex layout with fixed-width columns for IP addresses
5. Selectable/copyable text: ensure `select-text` CSS class and remove click-target overlap for IP text (currently the entire row is a link anchor)

**Template structure change for copyable text:**

The current design wraps each entire row in an `<a>` tag, making all text part of the link. For copyable IP addresses, the row needs restructuring: the IX name remains a link, but IP addresses and speed should be outside the `<a>` tag or use a separate click handler.

```
Current:  <a href="..."> [IXName] [speed ipv4 ipv6] [RS] </a>
Proposed: <div class="flex ...">
            <a href="..."> [IXName] </a>
            <div class="select-all font-mono"> [Speed: 10G] [IPv4: ...] [IPv6: ...] [RS] </div>
          </div>
```

This is the most structurally significant UI change -- breaking the `<a>` wrapper.

**Speed color helper (new function in `detail_shared.templ`):**

```go
func speedColorClass(mbps int) string {
    switch {
    case mbps >= 100_000:  // 100G+
        return "text-emerald-400"
    case mbps >= 10_000:   // 10G+
        return "text-sky-400"
    case mbps >= 1_000:    // 1G+
        return "text-amber-400"
    default:               // < 1G
        return "text-neutral-400"
    }
}
```

**Confidence:** HIGH -- all template changes are within existing components, no new routes or handlers needed.

## Patterns to Follow

### Pattern 1: Predicate Builder Extraction

**What:** Extract the filter predicate building from `ListNetworks` into a shared function so both `ListNetworks` and `StreamAllNetworks` use the same validation + predicate logic.

**When:** When implementing the streaming handler for each entity type.

**Example:**

```go
// buildNetworkPredicates validates filter fields and returns ent predicates.
// Returns a connect error for invalid input.
func buildNetworkPredicates(req interface{ ... }) ([]predicate.Network, error) {
    // Shared validation + predicate accumulation
}
```

However, since the `ListNetworksRequest` and `StreamAllNetworksRequest` are different proto types, a Go interface or separate function per type is needed. The pragmatic approach: duplicate the 5-10 lines of predicate building in the streaming method. The filter logic is simple enough that DRY extraction adds more complexity (interface definitions, type assertions) than it saves.

**Recommendation:** Accept the small duplication. Each streaming method copies its filter predicates from the corresponding List method. The `entityToProto` conversion functions are already shared and remain so.

### Pattern 2: Batch-Then-Stream

**What:** Load entities in fixed-size batches via ent's `Limit`/`Offset`, convert and send each one individually.

**When:** All 13 streaming handlers.

**Why not load all at once:** Some entity types (e.g., NetworkIxLan) have 100K+ rows. Loading all into memory defeats the purpose of streaming. Batching with offset pagination keeps memory bounded.

**Why not use database/sql cursor:** ent's generated code doesn't expose cursor iteration. Using raw SQL would bypass ent's type-safe converters and require maintaining parallel conversion logic. The Limit/Offset approach is idiomatic ent.

**Batch size:** 500 entities per batch. This balances:
- Memory: ~500 ent structs in memory at a time (few KB each)
- Round trips: NetworkIxLan (~100K rows) = ~200 DB queries
- SQLite performance: SQLite handles `LIMIT 500 OFFSET N` efficiently with ID-ordered queries

### Pattern 3: Error Semantics for Streaming

**What:** Errors during streaming are sent as gRPC status in trailers, not HTTP status.

**When:** A query fails mid-stream or the client disconnects.

**Key difference from unary:** In unary RPCs, errors map to HTTP status codes. In streaming, the HTTP status is always 200 (sent with first message). Errors are encoded in trailers. ConnectRPC handles this transparently -- `return connect.NewError(...)` from a streaming handler does the right thing.

**Client disconnect detection:** `stream.Send()` returns an error when the client has disconnected. Always check the error return from Send and return it immediately rather than continuing to query.

## Anti-Patterns to Avoid

### Anti-Pattern 1: Wrapping Stream Responses

**What:** Creating `StreamAllNetworksResponse { Network network = 1; }` wrapper messages.

**Why bad:** Adds 13 unnecessary message types to the proto file. The streaming response IS the entity -- there's no pagination token or metadata to include alongside it.

**Instead:** Use `stream Network` directly as the return type. The entity messages already exist.

### Anti-Pattern 2: Loading All Then Streaming

**What:** Calling `query.All(ctx)` for the entire table, then iterating over the slice to Send.

**Why bad:** Defeats the purpose of streaming. A full table load of NetworkIxLan (~100K rows) would consume significant memory before sending any data. The client receives nothing until the entire query completes.

**Instead:** Use the batch-then-stream pattern with `Limit`/`Offset`.

### Anti-Pattern 3: Sharing Service Structs for Streaming

**What:** Creating a separate `NetworkStreamingService` struct and registering it as a different service.

**Why bad:** Adds a new service name to reflection, health checking, and the service name list. Violates the principle that streaming is a different access pattern for the same data, not a different service.

**Instead:** Add the streaming method to the existing `NetworkService` struct. The generated interface naturally extends.

### Anti-Pattern 4: Making IP Addresses Non-Selectable

**What:** Keeping the entire IX presence row as a single `<a>` tag while trying to make text copyable.

**Why bad:** Text inside link elements cannot be easily selected without triggering navigation. Users trying to copy an IP address will accidentally navigate to the IX detail page.

**Instead:** Break the row into a clickable IX name (link) and a non-link data area (speeds, IPs, RS badge).

## New vs Modified Files

### New Files

None. All changes are additions to existing files.

### Modified Files

| File | Change Type | Scope |
|------|-------------|-------|
| `proto/peeringdb/v1/services.proto` | ADD | 13 streaming RPCs + 13 request messages (~200 lines) |
| `gen/peeringdb/v1/peeringdbv1connect/services.connect.go` | REGENERATED | Interfaces gain streaming methods, constructors gain streaming handlers |
| `gen/peeringdb/v1/*.pb.go` | REGENERATED | New request message types |
| `internal/grpcserver/campus.go` | ADD method | `StreamAllCampuses` (~40 lines) |
| `internal/grpcserver/carrier.go` | ADD method | `StreamAllCarriers` (~35 lines) |
| `internal/grpcserver/carrierfacility.go` | ADD method | `StreamAllCarrierFacilities` (~35 lines) |
| `internal/grpcserver/facility.go` | ADD method | `StreamAllFacilities` (~45 lines) |
| `internal/grpcserver/internetexchange.go` | ADD method | `StreamAllInternetExchanges` (~45 lines) |
| `internal/grpcserver/ixfacility.go` | ADD method | `StreamAllIxFacilities` (~35 lines) |
| `internal/grpcserver/ixlan.go` | ADD method | `StreamAllIxLans` (~35 lines) |
| `internal/grpcserver/ixprefix.go` | ADD method | `StreamAllIxPrefixes` (~35 lines) |
| `internal/grpcserver/network.go` | ADD method | `StreamAllNetworks` (~45 lines) |
| `internal/grpcserver/networkfacility.go` | ADD method | `StreamAllNetworkFacilities` (~40 lines) |
| `internal/grpcserver/networkixlan.go` | ADD method | `StreamAllNetworkIxLans` (~40 lines) |
| `internal/grpcserver/organization.go` | ADD method | `StreamAllOrganizations` (~35 lines) |
| `internal/grpcserver/poc.go` | ADD method | `StreamAllPocs` (~35 lines) |
| `internal/grpcserver/pagination.go` | ADD constant | `streamBatchSize = 500` (1 line) |
| `internal/grpcserver/grpcserver_test.go` | ADD tests | Streaming tests for representative types (~100 lines) |
| `internal/web/templates/detail_net.templ` | MODIFY | `NetworkIXLansList` component layout |
| `internal/web/templates/detail_shared.templ` | ADD function | `speedColorClass` helper |
| `internal/web/templates/detail_ix.templ` | MODIFY | `IXParticipantsList` component layout (consistency) |

### Unchanged Files (notable)

| File | Why Unchanged |
|------|--------------|
| `cmd/peeringdb-plus/main.go` | Service registration is interface-based; new methods satisfy expanded interface |
| `proto/peeringdb/v1/v1.proto` | Entity messages are unchanged |
| `proto/peeringdb/v1/common.proto` | No new shared types needed |
| `buf.gen.yaml` / `buf.yaml` | Codegen config unchanged |
| `internal/grpcserver/convert.go` | Conversion helpers reused as-is |
| `internal/middleware/*.go` | Already supports streaming (Flusher, Unwrap) |
| `internal/web/detail.go` | Fragment handlers unchanged -- same data, different presentation |
| `internal/web/templates/detailtypes.go` | Data types unchanged |

## Suggested Build Order

The two feature tracks are independent and can be built in parallel. Within each track:

### Track A: Server-Streaming RPCs

1. **Proto definitions** -- Add all 13 `StreamAll*` RPCs and request messages to `services.proto`. Run `buf lint` to validate. This must come first as everything else depends on generated code.

2. **Code generation** -- Run `buf generate` to regenerate `gen/peeringdb/v1/`. This produces the expanded handler interfaces and new request message types. Compilation will fail until handlers implement the new methods.

3. **Batch constant + first handler** -- Add `streamBatchSize` to `pagination.go`. Implement `StreamAllNetworks` on `NetworkService` as the reference implementation. Verify it compiles and works with `grpcurl`.

4. **Remaining 12 handlers** -- Implement all other `StreamAll*` methods following the Network pattern. Each is a copy-adapt of the reference implementation with entity-specific predicates and conversion functions.

5. **Tests** -- Add streaming tests for at least Network, NetworkIxLan (high row count), and one simple type. Test: empty stream, filtered stream, cancellation behavior.

### Track B: IX Presence UI Polish

1. **Speed color helper** -- Add `speedColorClass()` to `detail_shared.templ`. Run `templ generate`.

2. **NetworkIXLansList redesign** -- Modify `detail_net.templ` to add field labels, color-coded speeds, RS badge repositioning, and selectable text layout. Run `templ generate`.

3. **IXParticipantsList consistency** -- Apply the same layout changes to `detail_ix.templ`. Run `templ generate`.

4. **Visual verification** -- Check in browser: dark mode, responsive layout, text selection, link behavior.

### Dependency Graph

```
Track A:
  services.proto -> buf generate -> pagination.go + network.go -> other 12 handlers -> tests

Track B:
  detail_shared.templ -> detail_net.templ -> detail_ix.templ -> visual QA

Tracks A and B have zero code dependencies.
```

## Scalability Considerations

| Concern | At current scale (~100K NetworkIxLans) | At 10x scale | Notes |
|---------|---------------------------------------|-------------|-------|
| Streaming memory | ~500 entities in flight per batch | Same | Batch size is constant |
| Streaming duration | ~3-5 seconds for full NetworkIxLan stream | ~30-50 seconds | Consider context timeout |
| SQLite OFFSET performance | Good with ID-ordered index | Degrades at high offsets | Could switch to keyset pagination (`WHERE id > last_seen_id`) if needed |
| Concurrent streams | SQLite WAL handles concurrent reads | Same | Read-only workload, no contention |
| Client backpressure | HTTP/2 flow control handles naturally | Same | ConnectRPC respects flow control |

## Sources

- [ConnectRPC Streaming Documentation](https://connectrpc.com/docs/go/streaming/) - Server-streaming handler patterns
- [ConnectRPC ServerStream API](https://pkg.go.dev/connectrpc.com/connect#ServerStream) - Send(), ResponseHeader(), ResponseTrailer() methods
- [ConnectRPC NewServerStreamHandlerSimple](https://pkg.go.dev/connectrpc.com/connect#NewServerStreamHandlerSimple) - Simple mode handler constructor
- [ConnectRPC examples-go Eliza Service](https://github.com/connectrpc/examples-go/blob/main/internal/gen/connectrpc/eliza/v1/elizav1connect/eliza.connect.go) - Generated code for mixed unary + streaming service
- [ConnectRPC connect-go handler.go](https://github.com/connectrpc/connect-go/blob/main/handler.go) - Handler construction internals
- [Ent ORM Query API](https://entgo.io/docs/crud) - All(), Limit(), Offset() query methods
- Existing codebase: `gen/peeringdb/v1/peeringdbv1connect/services.connect.go` -- confirmed `simple` mode handler signatures
- Existing codebase: `internal/middleware/logging.go` -- confirmed Flusher + Unwrap support for streaming
- Existing codebase: `cmd/peeringdb-plus/main.go` -- confirmed interface-based service registration
