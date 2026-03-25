# Feature Landscape

**Domain:** Server-streaming RPCs for bulk data export + IX presence UI polish
**Researched:** 2026-03-25
**Milestone:** v1.7

## Table Stakes

Features users expect from a streaming bulk export API and a polished IX presence display. Missing = product feels incomplete or unprofessional.

### Streaming RPCs

| Feature | Why Expected | Complexity | Dependencies | Notes |
|---------|--------------|------------|--------------|-------|
| Server-streaming RPC per entity type | Users performing full database dumps expect a single call that returns all rows without manual pagination. PeeringDB's primary consumers (automation scripts, RPKI validators, looking glass aggregators) do hourly/daily full exports. Without streaming, they must loop through offset-based pages. | Med | Existing `services.proto`, ConnectRPC handler pattern in `internal/grpcserver/` | 13 new RPCs, one per entity type. Proto definition uses `stream` keyword on response. ConnectRPC handler signature: `func(ctx, *connect.Request[Req], *connect.ServerStream[Res]) error`. |
| Row-at-a-time marshaling from DB cursor | The entire point of streaming over paginated List is to avoid buffering N*pageSize rows in memory. Each row must be queried, converted to proto, and sent before the next row is fetched. | Med | Ent `.Query().All()` returns full slice -- need to switch to cursor iteration or chunked queries | Ent does not expose a native row-cursor API. Must use chunked fetches (e.g., batches of 500 by ID ascending) with a moving cursor, not `.All()`. |
| Graceful stream termination | Client may cancel mid-stream (e.g., they only needed networks, not all 200K networkixlan rows). Handler must honor `ctx.Done()` and stop querying. | Low | Go context cancellation, ConnectRPC handles this automatically | Check `ctx.Err()` between batch fetches. ConnectRPC propagates client cancellation to the handler context. |
| ConnectRPC + gRPC + gRPC-Web protocol support | Streaming must work over all three wire protocols that ConnectRPC supports. Browser consumers use Connect protocol or gRPC-Web; CLI consumers use gRPC. | Low | ConnectRPC handles this transparently for server streams | No extra work needed. ConnectRPC server streaming works identically across all three protocols. The existing CORS configuration already allows streaming content types. |
| Total record count in response headers | Consumers need to know how many records to expect for progress bars, allocation, and validation. PeeringDB's `meta.generated` field serves this purpose in the REST API. | Low | `stream.ResponseHeader()` on ConnectRPC's `*connect.ServerStream` | Set `x-total-count` (or equivalent) header before first `Send()`. Requires a `COUNT(*)` query upfront. |
| Filtering on streaming RPCs | Same filters available on List should be available on streaming RPCs. Users export "all networks for org X" or "all networkixlans for ASN Y". | Med | Existing predicate accumulation pattern from List handlers | Reuse the same optional filter fields and predicate-building logic. Request message can share the filter fields with the List request (or embed them). |
| OTel instrumentation on streams | otelconnect interceptor must produce meaningful spans for streaming RPCs -- one span per stream lifecycle, not per message. | Low | Existing `otelconnect.NewInterceptor` already in the handler chain | otelconnect handles streaming RPCs automatically. Span covers the full stream duration. |

### IX Presence UI

| Feature | Why Expected | Complexity | Dependencies | Notes |
|---------|--------------|------------|--------------|-------|
| Field labels for speed, IPv4, IPv6 | Current display shows raw values (`100G`, `192.0.2.1`, `2001:db8::1`) in a flex row without labels. Users cannot tell which field is which at a glance, especially when some fields are absent. PeeringDB's own UI shows labeled columns. | Low | `NetworkIXLansList` in `detail_net.templ` | Add small `text-neutral-500` label spans before each value: "Speed:", "IPv4:", "IPv6:". |
| RS badge repositioned near peering data | RS badge is currently right-aligned with `shrink-0 ml-3`, separated from the data it describes. On wide screens it floats far from the IX name and IPs, easy to miss. | Low | `NetworkIXLansList` template | Move RS badge inline with the data row (after IX name) rather than in a separate `justify-between` right column. |
| Port speed color coding | Networking professionals expect visual differentiation of port speeds at a glance. 1G, 10G, 100G, 400G are fundamentally different scale tiers. Without color coding, scanning a list of 50+ IX presences is tedious. | Low | `formatSpeed()` in `detail_shared.templ`, Tailwind CSS color classes | Use color tiers: <=1G neutral/gray, 10G blue, 100G emerald, 400G+ amber. Colors are semantic -- faster = warmer/brighter. |
| Consistent IP address indentation | IPv4 and IPv6 addresses currently flow together in a `flex gap-4` row. When IPv4 is missing, IPv6 shifts left. When both are present, the varying widths cause visual jaggedness across rows. | Low | `NetworkIXLansList` template | Use a grid or fixed-width layout for the address columns so they align vertically across rows. `font-mono` is already applied. |
| Selectable/copyable text without selecting the IX link | The entire row is an `<a>` tag, so selecting text to copy an IP address also selects the IX name link. Users frequently need to copy IP addresses for router configuration. | Low | `NetworkIXLansList` template structure | Restructure: make only the IX name the clickable link, with the data row outside the `<a>` tag. Or use `user-select: text` on data elements with click event isolation. |

## Differentiators

Features that set the streaming API and UI apart from PeeringDB and other mirrors. Not expected, but create significant value.

### Streaming RPCs

| Feature | Value Proposition | Complexity | Dependencies | Notes |
|---------|-------------------|------------|--------------|-------|
| Streaming with format negotiation (proto vs JSON) | ConnectRPC supports both protobuf and JSON wire formats. Streaming protobuf is ~3-5x more compact than JSON for PeeringDB data. Offering both lets CLI users pick efficiency while browser/curl users pick readability. | Low | ConnectRPC handles this via `Content-Type` header automatically | No code needed. ConnectRPC's `ServerStream.Send()` marshals to whatever format the client negotiated. Differentiation is just documenting it. |
| SHA256 checksum in response trailers | After streaming all rows, send a SHA256 of the concatenated row data as a trailer. Consumers can verify data integrity without re-downloading. Particularly valuable for hourly automated dumps. | Med | `stream.ResponseTrailer()`, incremental hash computation during Send loop | Hash each proto message bytes as sent. Write `x-content-sha256` trailer after final Send. Trailer support varies by client -- gRPC clients handle it natively; HTTP/1.1 clients may not receive trailers. |
| Stream resume via `since_id` filter | Allow consumers to resume an interrupted stream by passing `since_id` (the last ID they received). Avoids re-downloading from the beginning after a network interruption. | Low | ID-ordered queries with `WHERE id > since_id` predicate | Add optional `since_id` field to the streaming request messages. This is idempotent because data is immutable between syncs. |
| Timestamp-based "modified since" filter | Filter stream output to records updated after a given timestamp. Enables efficient incremental sync without downloading unchanged records. | Low | Ent `Updated` field exists on all entities, add `updated_since` filter | Complements `since_id` for a different use case: catching modifications to existing records. |

### IX Presence UI

| Feature | Value Proposition | Complexity | Dependencies | Notes |
|---------|-------------------|------------|--------------|-------|
| Sortable IX presence table | Let users sort IX presences by speed, name, or RS status. Networks with 50+ IX presences need to quickly find their 100G ports. | Med | JavaScript sort (htmx compatible) or server-side sort with htmx swap | Could use htmx `hx-trigger="click"` on column headers to re-fetch with sort parameter. Or client-side with a small JS sort function. |
| Copy-to-clipboard button on IP addresses | One-click copy of IPv4/IPv6 addresses. Networking professionals constantly paste IPs into router configs, looking glasses, and traceroute tools. | Low | Browser Clipboard API, small inline button | `navigator.clipboard.writeText()` with a clipboard icon button. Requires HTTPS (already on Fly.io with TLS). |
| Aggregate bandwidth display | Show total aggregate bandwidth across all IX presences in the section header. "IX Presences (47) -- 1.2 Tbps total" gives immediate network scale context. | Low | Sum of speed values computed during data loading | Compute in the handler, pass to the template. Display next to the count badge. |
| Speed distribution mini-chart | Tiny inline bar chart showing distribution of port speeds (e.g., 3x1G, 12x10G, 30x100G, 2x400G). Visual fingerprint of a network's peering investment. | Med | Either SVG generation in templ or a small client-side chart | Interesting differentiator but may be overdesign for v1.7. Consider for later. |

## Anti-Features

Features to explicitly NOT build in this milestone.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| Bidirectional streaming RPCs | Read-only mirror has no write path. Client has no data to stream to server. Adds API complexity for zero value. | Server-streaming only. Client sends request, server streams response. |
| Client-streaming RPCs | Same as above. No write path, no use case for client sending multiple messages. | Unary request, streaming response. |
| Streaming with cursor-based page tokens | Mixing pagination concepts with streaming creates confusion. A stream IS the full result set; pagination is for bounded windows. | Streaming replaces pagination for full exports. Keep existing List RPCs for paginated access. |
| Streaming via WebSocket fallback | ConnectRPC's Connect protocol handles streaming over HTTP/2 natively. Adding WebSocket as an alternative transport adds complexity with no benefit. | Rely on ConnectRPC's built-in protocol handling. |
| Real-time change streaming / subscriptions | This is a periodic sync mirror, not a live database. Real-time streaming implies event sourcing, change data capture, or long-lived connections -- none of which match the hourly-sync architecture. | Consumers poll or re-stream periodically. |
| Custom download formats (CSV, NDJSON) | Scope creep. Protobuf and JSON via ConnectRPC are sufficient for programmatic consumers. CSV is a different export surface entirely. | Stick to ConnectRPC's native wire formats (protobuf, JSON). |
| IX presence interactive map | Geographic visualization of IX presences on a world map. Looks impressive but requires a mapping library, geodata enrichment, and significant frontend complexity. | Text-based list with city/country is sufficient. Map is a separate future feature. |
| Drag-and-drop column reordering | Over-engineering the IX presence table. Fixed column order is fine for data display. | Static column layout matching PeeringDB conventions. |
| Inline editing of IX presence data | This is a read-only mirror. No editing. | Link to PeeringDB for data corrections. |

## Feature Dependencies

```
Proto definition (stream RPCs) --> buf generate --> ConnectRPC handler codegen
                                                        |
                                                        v
Chunked DB query helper --> Streaming handler implementation --> OTel (automatic)
                                                        |
                                                        v
                                              Total count header
                                              SHA256 trailer (optional)
                                              since_id filter (optional)
                                              updated_since filter (optional)

IX presence field labels --> Port speed color coding (uses same label structure)
                        --> RS badge repositioning (same template refactor)

Selectable text fix --> Copy-to-clipboard button (depends on text being selectable)

Aggregate bandwidth --> Speed distribution chart (depends on same computed data)
```

Key dependency: Streaming handler implementation requires a chunked query helper because Ent's `.All()` loads everything into memory. This helper is the critical new infrastructure needed.

## MVP Recommendation

### Phase 1: Streaming RPCs (Core)

Prioritize:
1. **Proto definitions** for 13 streaming RPCs (table stakes, low complexity, unlocks codegen)
2. **Chunked DB query helper** (table stakes, medium complexity, critical infrastructure)
3. **Streaming handler implementation** for all 13 types (table stakes, medium complexity due to repetition)
4. **Total count header** (table stakes, low complexity, high value for consumers)
5. **Filtering on streaming RPCs** (table stakes, medium complexity, reuses existing predicate logic)

Defer to later in phase or separate phase:
- SHA256 trailer (differentiator, medium complexity)
- `since_id` and `updated_since` filters (differentiator, low complexity but adds request message fields)

### Phase 2: IX Presence UI Polish

Prioritize:
1. **Field labels** for speed/IPv4/IPv6 (table stakes, low complexity)
2. **RS badge repositioning** (table stakes, low complexity)
3. **Port speed color coding** (table stakes, low complexity)
4. **Consistent IP address indentation** (table stakes, low complexity)
5. **Selectable/copyable text** (table stakes, low complexity)

Defer:
- Sortable table (differentiator, medium complexity)
- Copy-to-clipboard button (differentiator, low complexity, depends on text fix)
- Aggregate bandwidth (differentiator, low complexity)

### Phase Ordering Rationale

Streaming RPCs first because:
- Proto changes require `buf generate` and affect the codegen pipeline
- The chunked query helper is new infrastructure that needs testing
- Streaming RPCs are the primary deliverable of v1.7

UI polish second because:
- Pure template changes, no infrastructure impact
- All changes are independent of each other (can be done in any order)
- Lower risk -- worst case is a visual regression, easily reverted

## Detailed Feature Specifications

### Streaming RPC Proto Pattern

Each of the 13 services gets a new `Stream` RPC. Example for NetworkService:

```protobuf
service NetworkService {
  rpc GetNetwork(GetNetworkRequest) returns (GetNetworkResponse);
  rpc ListNetworks(ListNetworksRequest) returns (ListNetworksResponse);
  rpc StreamNetworks(StreamNetworksRequest) returns (stream Network);
}

message StreamNetworksRequest {
  // Filter fields -- same as ListNetworksRequest minus pagination.
  optional int64 asn = 1;
  optional string name = 2;
  optional string status = 3;
  optional int64 org_id = 4;
}
```

Key design decisions:
- RPC name: `StreamX` (not `ListAllX` or `ExportX`) -- clear, consistent, matches the `stream` keyword
- Response type: bare entity message (e.g., `Network`), not wrapped in a response envelope -- each streamed message IS one entity
- Request message: separate from `ListNetworksRequest` -- no `page_size`/`page_token` fields since streaming replaces pagination
- Filter fields: same optional fields as List, reusing the predicate accumulation pattern

### ConnectRPC Handler Signature

```go
func (s *NetworkService) StreamNetworks(
    ctx context.Context,
    req *connect.Request[pb.StreamNetworksRequest],
    stream *connect.ServerStream[pb.Network],
) error {
    // 1. Count total records (for header)
    // 2. Set x-total-count header via stream.ResponseHeader()
    // 3. Build query with filters
    // 4. Iterate in chunks by ID, Send() each row
    // 5. Return nil on success
}
```

### Chunked Query Pattern

Since Ent does not expose a cursor-based row iterator for SQLite, implement chunked fetches:

```go
const defaultChunkSize = 500

// Iterate in chunks ordered by ID ascending:
lastID := 0
for {
    if err := ctx.Err(); err != nil {
        return err
    }
    results, err := query.Where(entity.IDGT(lastID)).
        Order(ent.Asc(entity.FieldID)).
        Limit(chunkSize).
        All(ctx)
    if err != nil {
        return err
    }
    if len(results) == 0 {
        return nil // done
    }
    for _, r := range results {
        if err := stream.Send(toProto(r)); err != nil {
            return err
        }
    }
    lastID = results[len(results)-1].ID
}
```

Chunk size of 500 balances memory usage against query overhead. With ~200K networkixlan records (the largest table), this means ~400 queries -- each taking <1ms on SQLite, totaling <1s of query time.

### Port Speed Color Tiers

| Speed Range | Color | Tailwind Class | Rationale |
|-------------|-------|---------------|-----------|
| < 1G (100M, 200M etc.) | Gray | `text-neutral-500` | Legacy speeds, de-emphasized |
| 1G | Neutral | `text-neutral-400` | Baseline, current default |
| 10G | Blue | `text-blue-400` | Standard modern peering |
| 100G | Emerald | `text-emerald-400` | High-capacity, matches project accent |
| 400G+ | Amber | `text-amber-400` | Top-tier, attention-drawing |

This follows the networking industry's intuitive gradient from "basic" to "premium" without using red/yellow (which imply errors/warnings in UI contexts).

### IX Presence Row Restructure

Current structure (problematic):
```
<a href="/ui/ix/123" class="flex items-center justify-between ...">
  <div class="flex flex-col">
    <span>IX Name</span>           <!-- clickable, selectable -->
    <div class="flex gap-4">
      <span>100G</span>            <!-- clickable, not separately selectable -->
      <span>192.0.2.1</span>       <!-- clickable, not separately selectable -->
      <span>2001:db8::1</span>     <!-- clickable, not separately selectable -->
    </div>
  </div>
  <span>RS</span>                  <!-- far right, disconnected -->
</a>
```

Proposed structure:
```
<div class="... px-4 py-3">
  <div class="flex items-center gap-2">
    <a href="/ui/ix/123" class="... hover:text-emerald-400">
      IX Name
    </a>
    <span class="... border border-emerald-400/30 rounded">RS</span>
  </div>
  <div class="grid grid-cols-[auto_1fr_1fr] gap-x-4 text-sm font-mono mt-1">
    <span class={speedColor}>Speed: 100G</span>
    <span class="text-neutral-400">IPv4: 192.0.2.1</span>
    <span class="text-neutral-400">IPv6: 2001:db8::1</span>
  </div>
</div>
```

Key changes:
- IX name is the only `<a>` tag -- data fields are plain text, freely selectable
- RS badge is inline after the IX name, not right-aligned
- Grid layout ensures IP addresses align vertically across rows
- Speed gets color-coded via `speedColorClass()` helper function
- Labels ("Speed:", "IPv4:", "IPv6:") provide context
- Same restructure applies to `IXParticipantsList` in `detail_ix.templ` for consistency

## Sources

- [ConnectRPC Streaming Documentation](https://connectrpc.com/docs/go/streaming/) -- Server streaming handler patterns and protocol support
- [ConnectRPC Go Package Documentation](https://pkg.go.dev/connectrpc.com/connect) -- ServerStream type API: Send, ResponseHeader, ResponseTrailer methods
- [gRPC Flow Control](https://grpc.io/docs/guides/flow-control/) -- Backpressure and HTTP/2 flow control mechanics
- [gRPC Streaming Best Practices](https://dev.to/ramonberrutti/grpc-streaming-best-practices-and-performance-insights-219g) -- When to use streaming, message size considerations
- [gRPC Core Concepts](https://grpc.io/docs/what-is-grpc/core-concepts/) -- Server streaming lifecycle (headers, messages, trailers)
- [gRPC Metadata Documentation](https://grpc.io/docs/guides/metadata/) -- Headers vs trailers, when to use each
- [PeeringDB Network Detail (AS8075)](https://www.peeringdb.com/net/694) -- Reference IX presence table layout with labeled columns (Exchange, IPv4, ASN, IPv6, Speed, RS Peer)
- [gRPC Performance Best Practices](https://grpc.io/docs/guides/performance/) -- Streaming lifecycle management, graceful completion
- [Microsoft gRPC Performance](https://learn.microsoft.com/en-us/aspnet/core/grpc/performance) -- Message size limits, flow control activation
- Existing codebase: `internal/grpcserver/network.go`, `internal/grpcserver/pagination.go`, `proto/peeringdb/v1/services.proto`, `internal/web/templates/detail_net.templ`, `internal/web/templates/detail_ix.templ`, `internal/web/templates/detail_shared.templ`
