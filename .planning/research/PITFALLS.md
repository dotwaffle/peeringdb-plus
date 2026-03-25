# Domain Pitfalls

**Domain:** Server-streaming RPCs (ConnectRPC) and IX presence UI polish for PeeringDB Plus
**Researched:** 2026-03-25
**Milestone:** v1.7 Streaming RPCs & UI Polish

## Critical Pitfalls

Mistakes that cause production breakage, require architectural rework, or silently corrupt data.

### Pitfall 1: Recovery Middleware Cannot Return Error Responses After Streaming Begins

**What goes wrong:** The existing `middleware.Recovery` wraps the handler and attempts to write a 500 JSON error on panic. Once a streaming RPC has sent its first message, HTTP headers (including status 200) are already committed to the wire. A panic mid-stream means the recovery middleware's `http.Error()` call is silently dropped -- the client sees a truncated stream with no error indication instead of a clean error.

**Why it happens:** gRPC/ConnectRPC streaming always returns HTTP 200 with errors encoded in trailers. The recovery middleware was designed for unary request/response, where headers haven't been sent when the handler panics. Streaming breaks this assumption because the handler calls `stream.Send()` multiple times, each of which writes to the response body.

**Consequences:** Silent data corruption from the client's perspective -- they receive a partial result set with no error status. Difficult to debug because the server logs the panic but the client has no indication of failure beyond a broken stream.

**Prevention:**
- ConnectRPC handles panic recovery internally via `net/http`'s built-in panic recovery (which sends RST_STREAM on HTTP/2 or closes the connection on HTTP/1.1). The client will see a connection error.
- Do NOT attempt to add custom panic recovery logic inside streaming handlers. Instead, ensure streaming handler code cannot panic: validate all inputs before entering the streaming loop, and use explicit error returns within the loop.
- The existing recovery middleware is fine for the outer HTTP layer -- it catches panics before ConnectRPC processes the request. But panics inside `stream.Send()` loops are caught by `net/http`, not by the middleware.
- Verify with a test: inject a panic mid-stream and confirm the client receives a transport error (connection reset or EOF), not a partial success.

**Detection:** Test by injecting a panic mid-stream and verifying the client receives an error (not a truncated success). Monitor for `panic recovered` log entries during streaming operations.

**Confidence:** HIGH -- verified from net/http source, ConnectRPC streaming docs, and go-grpc-middleware recovery patterns.

---

### Pitfall 2: Loading All Rows Into Memory Before Streaming Defeats the Purpose

**What goes wrong:** Using `entClient.Network.Query().All(ctx)` to fetch all rows, then iterating through the slice to call `stream.Send()` for each one. This loads the entire table (potentially 100K+ rows for NetworkIxLan) into memory before sending anything, negating the streaming benefit entirely.

**Why it happens:** Ent's `.All()` is the natural query method and what the existing `List` RPCs use. Developers copy the pattern without considering that streaming's purpose is to avoid buffering the full result set. The existing `ListNetworks` handler uses `query.All(ctx)` with a limit of 1001 (page size + 1), which works fine for pagination but would be catastrophic for a full-table stream.

**Consequences:** OOM on Fly.io machines (256MB-512MB RAM). Even if it fits, GC pressure from allocating and discarding 100K+ ent entity structs causes latency spikes affecting concurrent requests. Defeats the stated goal of "stream rows one at a time from DB query results via protobuf."

**Prevention:**
- Implement internal batched pagination: query in batches of 500-1000 rows using keyset pagination (`WHERE id > lastID ORDER BY id LIMIT batchSize`) in a loop. For each batch, convert to proto and `stream.Send()` each row individually.
- Ent supports keyset pagination via `Where(entity.IDGT(lastID)).Order(ent.Asc(entity.FieldID)).Limit(batchSize)`.
- Close each batch's result slice reference before fetching the next to allow GC to reclaim memory.
- Do NOT use OFFSET-based pagination for streaming -- OFFSET performance degrades linearly with depth in SQLite as it must scan and discard rows.

**Detection:** Monitor `go_memstats_alloc_bytes` during a full-table stream. If it spikes proportionally to table size, the batching is wrong. A correct implementation should have roughly constant memory usage regardless of total row count.

**Confidence:** HIGH -- ent's `.All()` is documented to load all results into memory. Keyset pagination is the standard SQLite approach for large tables.

---

### Pitfall 3: Long-Running Streams Block LiteFS WAL Checkpointing

**What goes wrong:** SQLite WAL mode allows concurrent reads and writes, but WAL checkpointing (flushing WAL back to the main database file) requires that no readers hold snapshots from before the checkpoint. A streaming query that holds a single read transaction open for the duration of the stream (potentially minutes) prevents WAL checkpointing, causing the WAL file to grow unbounded.

**Why it happens:** If the entire streaming operation is wrapped in a single `entClient.Tx()` for read consistency, SQLite keeps the WAL snapshot open for the full stream duration. Each batch query within the transaction reads from the same snapshot, which is correct for consistency but prevents checkpoint.

**Consequences:** WAL file grows to hundreds of MB during long streams. On LiteFS replicas, replication lag increases because the primary's WAL checkpoint is blocked. Disk usage grows on Fly.io machine's ephemeral storage (typically limited).

**Prevention:**
- Do NOT wrap the entire streaming operation in a single database transaction. Accept that rows may reflect minor changes between batches -- the data is a read-only mirror that syncs hourly, so inter-batch consistency is acceptable for bulk export use cases.
- Each batch query should be a separate short-lived read transaction. Ent's default query behavior (no explicit `Tx`) already does this -- each `.All(ctx)` call opens and closes its own transaction.
- Add a timeout to the overall stream operation (e.g., 5 minutes) to prevent indefinite reads.
- Monitor WAL file size during streaming operations.

**Detection:** Check WAL file size on disk during active streams. Alert if WAL exceeds normal checkpoint size (typically a few MB) for extended periods.

**Confidence:** HIGH -- SQLite WAL behavior is well-documented. LiteFS WAL mode blog post confirms the checkpointing semantics.

## Moderate Pitfalls

### Pitfall 4: Streaming RPC Timeouts Differ From Unary RPC Timeouts

**What goes wrong:** ConnectRPC documentation states: "Timeouts for streaming RPCs apply to the whole message exchange." The server's existing `http.Server` has no explicit read/write timeouts set in `main.go`. For unary RPCs this is tolerable because requests complete quickly. For streaming RPCs, a slow or malicious client can keep a connection open indefinitely, tying up a goroutine and database resources.

**Why it happens:** Unary RPCs are naturally short-lived. When streaming is added, the same server config that was fine for unary becomes a resource leak vector. ConnectRPC's deployment docs warn specifically: "Keep timeouts short and avoid streaming for untrusted client scenarios" because `net/http` uses `SetDeadline` on connections, meaning any read/write blocks until the complete stream timeout expires.

**Prevention:**
- Set a per-RPC deadline using `context.WithTimeout` inside each streaming handler (e.g., 5 minutes for a full table dump).
- Check `ctx.Done()` inside the streaming loop between batches.
- Consider adding `ReadHeaderTimeout` to the `http.Server` config, but keep it generous enough for legitimate streams.
- The streaming endpoint is public and unauthenticated -- resource exhaustion is a real concern.

**Confidence:** HIGH -- ConnectRPC deployment docs explicitly warn about this.

---

### Pitfall 5: Adding Streaming RPCs to Existing Services Changes Generated Handler Interfaces

**What goes wrong:** Adding a `stream` server-streaming RPC to an existing service (e.g., adding `StreamAllNetworks` to `NetworkService`) changes the generated `NetworkServiceHandler` interface. The existing `NetworkService` struct no longer satisfies the interface because it doesn't implement the new method. This is a compile-breaking change.

**Why it happens:** `protoc-gen-connect-go` generates a single handler interface per service. Any new RPC method changes the interface contract. All 13 services break simultaneously when the proto is regenerated.

**Consequences:** Compilation failure across all 13 service handler files immediately after proto regeneration. If not anticipated, this creates a large diff that must be addressed all at once.

**Prevention:**
- Plan the proto change: add all 13 streaming RPC definitions to `services.proto` in one commit, regenerate, then implement stub handlers for all 13 before any of them will compile.
- ConnectRPC generates `Unimplemented*Handler` structs. Check if the existing service structs embed these -- if not, adding them now would allow incremental implementation (unimplemented methods return `connect.CodeUnimplemented`).
- Alternatively, create a separate proto service for streaming (e.g., `NetworkExportService` with just the streaming RPCs), avoiding breaking the existing handler interfaces. This creates more services but decouples streaming from the existing unary API.

**Confidence:** HIGH -- this is how protoc-gen-connect-go works. Verified from existing generated code in `services.connect.go`.

---

### Pitfall 6: HTTP/1.1 Clients Receive Streaming Differently Than HTTP/2 Clients

**What goes wrong:** ConnectRPC supports server streaming over both HTTP/1.1 (chunked transfer encoding) and HTTP/2 (native frame-level multiplexing with flow control). The Connect protocol works over HTTP/1.1 for server streaming. The gRPC protocol requires HTTP/2. Through Fly.io's proxy, the protocol may be negotiated differently depending on client capability and `fly.toml` configuration.

**Why it happens:** The app supports both HTTP/1.1 and h2c via `http.Protocols`. ConnectRPC abstracts the protocol difference at the API level, but the transport behavior matters: HTTP/2 has native backpressure via flow control windows; HTTP/1.1 relies on TCP-level backpressure and chunked transfer encoding. Intermediate proxies may buffer chunked responses, defeating the streaming benefit on HTTP/1.1.

**Prevention:**
- Verify streaming works through Fly.io's proxy with both `grpcurl` (h2c/gRPC), `buf curl` (Connect protocol), and a plain HTTP/1.1 client.
- The v1.6 milestone already removed the LiteFS proxy and enabled h2c directly. Confirm Fly.io's internal routing does not re-introduce buffering.
- For the Connect protocol over HTTP/1.1, the stream uses `Content-Type: application/connect+proto` with chunked encoding. Ensure no intermediate proxy buffers the full response before forwarding.
- Server streaming is fully supported on HTTP/1.1 in ConnectRPC (confirmed via GitHub issue #639). Bidirectional streaming requires HTTP/2, but this milestone only adds server streaming.

**Confidence:** HIGH -- verified from ConnectRPC issue #639 and deployment docs.

---

### Pitfall 7: Logging Middleware Logs Only After Handler Returns -- Streams Appear as Single Long Request

**What goes wrong:** The existing `middleware.Logging` captures the status code and logs the request after `next.ServeHTTP(wrapped, r)` returns. For a streaming RPC that takes 30+ seconds, the log entry appears only after the stream completes. There is no visibility into in-progress streams, and the logged duration is the total stream time.

**Why it happens:** The middleware was designed for unary request/response where duration is a meaningful metric. For streams, operators want to know about in-progress streams, message rates, and completion status.

**Prevention:**
- Accept this limitation for the HTTP-level logging middleware. The otelconnect interceptor already creates a span covering the full stream lifecycle with per-message events (trace events are enabled by default unless `WithoutTraceEvents` is used).
- Add explicit logging inside each streaming handler: a log at stream start ("starting stream for Network, N total rows") and at completion ("stream completed, sent N messages in D duration").
- Do NOT add per-message logging -- at 100K+ messages, this creates millions of log lines that overwhelm log ingestion.

**Confidence:** HIGH -- verified from existing `middleware.Logging` source code and otelconnect documentation.

---

### Pitfall 8: Graceful Shutdown Waits for All Active Streams to Complete

**What goes wrong:** `server.Shutdown(ctx)` waits for all in-flight requests to complete. A streaming RPC sending 100K rows could take minutes. During rolling deploys on Fly.io, the old machine waits for streams to finish before stopping, delaying the deploy.

**Why it happens:** HTTP/2 connections with active streams are not immediately closed by `Shutdown()`. The shutdown context has a timeout (`cfg.DrainTimeout`), but if it's too short, active streams are forcefully terminated mid-transfer.

**Consequences:** Either deploys take too long (waiting for streams) or streams are cut off mid-transfer during deploys.

**Prevention:**
- Set `DrainTimeout` to a reasonable value (30-60 seconds) and accept that long-running streams may be interrupted during deploys.
- Document that streaming clients should handle reconnection and resume from the last received ID (the messages include the entity ID, so clients know where they stopped).
- Check `ctx.Done()` in the streaming loop -- the shutdown context cancellation propagates through the request context.
- This is manageable because streaming is for bulk export (not real-time feeds). Clients can retry.

**Confidence:** MEDIUM -- standard HTTP/2 server shutdown behavior, but exact Fly.io rolling deploy timing needs runtime verification.

---

### Pitfall 9: gRPC Reflection Shows Stale Method Lists If Codegen Not Re-Run

**What goes wrong:** The existing `grpcreflect.NewStaticReflector(serviceNames...)` uses compiled-in file descriptors from the generated Go code. Service names don't change when streaming RPCs are added, so the registration code looks correct. But if `buf generate` is not re-run after updating `services.proto`, the reflection descriptors still show only `Get` and `List` methods -- the new streaming RPCs are invisible to discovery tools.

**Why it happens:** The static reflector reads from the proto file descriptor registry, which is populated at init time from the generated `.pb.go` files. If the `.proto` is updated but codegen is not re-run, there's a mismatch.

**Prevention:**
- Always re-run the full codegen pipeline (`buf generate` + `go generate ./ent`) after proto changes.
- Verify with `grpcurl -plaintext localhost:8080 list peeringdb.v1.NetworkService` that new streaming RPCs appear.
- Add a CI check that regenerated code matches committed code (this already exists as "go generate drift" check in the GitHub Actions pipeline).

**Confidence:** HIGH -- verified from existing reflection setup in `main.go`.

## Minor Pitfalls

### Pitfall 10: Anchor Tag Wrapping Entire Row Prevents Text Selection of Data Fields

**What goes wrong:** In the current IX participants list (`IXParticipantsList`) and network IX presences list (`NetworkIXLansList`), the entire row is wrapped in an `<a>` tag. Users cannot select and copy individual IP addresses, ASN numbers, or speed values without triggering navigation. Clicking anywhere in the row navigates away.

**Why it happens:** The `<a>` wrapping pattern creates clean clickable rows for navigation, but IP addresses and ASNs are data that network engineers frequently need to copy for configuration, whois lookups, or traceroutes.

**Prevention:**
- Restructure the row: keep the `<a>` link on the entity name only, move data fields (speed, IPv4, IPv6) outside the anchor tag into a parent `<div>`.
- Add `select-text` Tailwind class to data spans to explicitly enable text selection where needed.
- Use a click handler on the parent `<div>` for row-level navigation if desired, with `e.target` checks to avoid navigating when clicking on data text. Or accept that only the name is clickable.
- Fix both `NetworkIXLansList` and `IXParticipantsList` simultaneously -- they share the same pattern.

**Confidence:** HIGH -- verified from current templ source code.

---

### Pitfall 11: Color-Coded Port Speeds May Fail WCAG Contrast Requirements

**What goes wrong:** Adding color-coding to port speeds (e.g., different colors for 100G, 10G, 1G) can fail WCAG 2.1 AA contrast requirements (4.5:1 for normal text) against the dark background (`bg-neutral-900`, which is `#171717`). Opacity modifiers like `/70` reduce contrast below the threshold.

**Why it happens:** Tailwind color classes at full opacity often meet contrast requirements, but adding `/70` or `/50` opacity modifiers (as seen in the existing RS badge: `text-sky-400/70`) reduces the effective contrast ratio. Dark mode requires separate contrast verification -- WCAG applies to both light and dark modes independently.

**Prevention:**
- Use only Tailwind colors at full opacity that meet 4.5:1 contrast against `#171717` (neutral-900): `text-emerald-400`, `text-sky-400`, `text-amber-400`, `text-red-400` all pass at full opacity.
- Do NOT rely on color alone to convey meaning (WCAG SC 1.4.1). The existing `formatSpeed()` function produces text labels ("100G", "10G", "1G") -- the color supplements but does not replace the text.
- Avoid opacity modifiers on colored text. The existing RS badge uses `/70` which should be reviewed.
- Test both light and dark mode contrast ratios.

**Confidence:** HIGH -- WCAG requirements are well-defined. Tailwind color contrast values are calculable.

---

### Pitfall 12: RS Badge Repositioning May Break Flex Layout on Narrow Screens

**What goes wrong:** Moving the "RS" (route server) badge from its current `justify-between` right-aligned position to inline with data fields changes the flex layout. The current layout uses `flex items-center justify-between` with the RS badge as a `shrink-0` element. Moving it inline with speed/IP data may cause wrapping or overflow on mobile viewports.

**Prevention:**
- Test on mobile viewports (320px-480px) after repositioning.
- Keep the badge as an inline element near the data it describes. Use `inline-flex` for the badge to keep it in text flow.
- If placing the badge near the speed value, put it in the same `flex gap-4` container as the other data fields.

**Confidence:** MEDIUM -- depends on exact placement chosen.

---

### Pitfall 13: Field Labels Create Inconsistent Row Width When IP Fields Are Optional

**What goes wrong:** Adding field labels ("Speed:", "IPv4:", "IPv6:") to IX presence rows creates inconsistent alignment when some rows have IPv6 and others don't. The conditional rendering (`if row.IPAddr6 != ""`) means some rows have two data items and others have three, causing visual jaggedness in the list.

**Prevention:**
- Use a consistent grid or fixed-width layout for data fields, regardless of whether IPv6 is present.
- Consider a CSS Grid layout with defined columns instead of inline flex with gaps.
- Show placeholder text (e.g., "--") for missing fields to maintain alignment, or use a tabular layout where columns are always present.

**Confidence:** MEDIUM -- depends on how labels are implemented.

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Mitigation |
|-------------|---------------|------------|
| Proto definition for streaming RPCs | Pitfall 5: Handler interface change breaks compilation of all 13 services | Add all 13 streaming RPC definitions in one proto change, regenerate, then implement stubs. Consider separate export service. |
| Streaming handler implementation | Pitfall 2: All rows loaded into memory | Use batched keyset pagination inside handler. Query 500-1000 rows per batch, send individually, advance cursor by last ID. |
| Streaming handler implementation | Pitfall 3: WAL growth during long streams | Do NOT wrap entire stream in a transaction. Each batch is a separate read. |
| Streaming handler implementation | Pitfall 1: Panic recovery broken mid-stream | Validate all inputs before entering stream loop. Use explicit error returns, never panic. |
| Streaming handler implementation | Pitfall 4: No timeout on stream | Add `context.WithTimeout` in handler (5 min ceiling). Check `ctx.Done()` between batches. |
| Server configuration | Pitfall 8: Graceful shutdown delayed by streams | Set reasonable `DrainTimeout`. Document client reconnection from last received ID. |
| Middleware integration | Pitfall 7: Logging shows only final status for streams | Add explicit start/complete logs in streaming handlers. Rely on otelconnect for per-message events. |
| Fly.io deployment | Pitfall 6: HTTP/1.1 vs HTTP/2 streaming differences | Test with grpcurl (gRPC), buf curl (Connect), and curl (HTTP/1.1). Verify Fly.io proxy doesn't buffer. |
| Codegen pipeline | Pitfall 9: Stale reflection descriptors | Re-run full codegen pipeline after proto changes. CI drift check catches this. |
| IX presence UI restructure | Pitfall 10: Text not selectable in anchor-wrapped rows | Link on name only, data fields outside `<a>` tag. |
| Port speed colors | Pitfall 11: Color contrast failures | Full-opacity Tailwind colors only. Pair color with text label. Never color alone. |
| RS badge repositioning | Pitfall 12: Flex layout break on mobile | Test on narrow viewports. Use inline-flex. |
| Field labels and alignment | Pitfall 13: Inconsistent row widths | Fixed-width columns or placeholder values for missing fields. |

## Sources

- [ConnectRPC Streaming Documentation](https://connectrpc.com/docs/go/streaming/) -- Server streaming protocol details, HTTP 200 status for all streaming responses
- [ConnectRPC Deployment & h2c](https://connectrpc.com/docs/go/deployment/) -- Timeout considerations: "timeouts for streaming RPCs apply to the whole message exchange"
- [ConnectRPC HTTP/1.1 Streaming Issue #639](https://github.com/connectrpc/connect-go/issues/639) -- Confirms server streaming works over HTTP/1.1, bidirectional requires HTTP/2
- [connect package API](https://pkg.go.dev/connectrpc.com/connect) -- ServerStream type, Send method, WithReadMaxBytes/WithSendMaxBytes options
- [otelconnect-go](https://github.com/connectrpc/otelconnect-go) -- Streaming span covers full lifecycle, per-message trace events enabled by default
- [Go HTTP Handlers, Panic, and Deadlocks](https://iximiuz.com/en/posts/go-http-handlers-panic-and-deadlocks/) -- net/http built-in panic recovery sends RST_STREAM or closes connection
- [go-grpc-middleware Recovery](https://pkg.go.dev/github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery) -- StreamServerInterceptor for streaming panic recovery
- [gRPC Graceful Server Stop](https://grpc.io/docs/guides/server-graceful-stop/) -- GracefulStop waits for all requests including streaming
- [SQLite WAL Mode](https://www.sqlite.org/wal.html) -- Readers do not block writers, but open read transactions prevent WAL checkpoint
- [WAL Mode in LiteFS](https://fly.io/blog/wal-mode-in-litefs/) -- LiteFS WAL support, checkpoint behavior
- [Ent CRUD API](https://entgo.io/docs/crud/) -- All() loads entire result set into memory
- [Why Does gRPC Insist on Trailers?](https://carlmastrangelo.com/blog/why-does-grpc-insist-on-trailers) -- Error encoding in trailers, server can stream N records then report failure
- [WCAG Color Contrast Guidelines](https://webaim.org/articles/contrast/) -- 4.5:1 minimum for normal text
- [Dark Mode Accessibility](https://www.boia.org/blog/offering-a-dark-mode-doesnt-satisfy-wcag-color-contrast-requirements) -- Dark mode does not exempt WCAG compliance
- [Tailwind CSS User Select](https://tailwindcss.com/docs/user-select) -- select-text and select-none utilities
- [gRPC-Go Performance Improvements](https://grpc.io/blog/grpc-go-perf-improvements/) -- Memory allocation per frame, GC pressure in streaming
