# Domain Pitfalls

**Domain:** Adding ConnectRPC/gRPC API Surface to Existing Go HTTP Server
**Researched:** 2026-03-24
**Milestone:** ConnectRPC/gRPC API Surface (subsequent milestone)

## Critical Pitfalls

Mistakes that cause production breakage, require architectural rework, or silently corrupt data/observability.

### Pitfall 1: LiteFS Proxy Is HTTP/1.1-Only -- h2c Traffic Will Not Pass Through

**What goes wrong:** The application enables h2c (HTTP/2 cleartext) on its HTTP server to support gRPC/ConnectRPC. The Go server listens on port 8081 (behind LiteFS proxy on 8080). Fly.io's edge proxy sends h2c traffic to port 8080 (LiteFS proxy). The LiteFS proxy is a plain `http.Server` with no h2c support -- it creates `&http.Server{Handler: http.HandlerFunc(s.serveHTTP)}` with no `http.Protocols` configuration and no `h2c.NewHandler` wrapper. The LiteFS proxy only speaks HTTP/1.1. All HTTP/2 frames sent by Fly.io's edge are rejected or misinterpreted. gRPC calls fail with connection errors. REST/GraphQL requests that were working on HTTP/1.1 continue to work, but only because Fly.io's default behavior downgrades to HTTP/1.1 when `h2_backend` is not set.

**Why it happens:** The current architecture places LiteFS proxy between Fly.io's edge and the application:

```
Client -> Fly.io Edge (TLS termination) -> LiteFS Proxy (:8080) -> App (:8081)
```

The LiteFS proxy (litefs.yml line 17-24) intercepts traffic on port 8080 and forwards to the app on port 8081. It provides write-forwarding to the primary node and TXID consistency headers for replicas. This proxy was designed for simple HTTP/1.1 REST APIs, not for h2c passthrough. Verified by reading `superfly/litefs` source: `proxy_server.go` creates a bare `http.Server` without any HTTP/2 configuration.

**Consequences:**
- Setting `h2_backend = true` in fly.toml causes ALL traffic (not just gRPC) to fail, because Fly.io sends h2c to the LiteFS proxy which cannot handle it.
- Without `h2_backend = true`, gRPC-native clients cannot connect because Fly.io downgrades to HTTP/1.1 and gRPC requires HTTP/2.
- ConnectRPC can work over HTTP/1.1 (it supports the Connect protocol and gRPC-Web over HTTP/1.1), but native gRPC clients will not work.

**Prevention:**
- **Option A (recommended): Use ConnectRPC over HTTP/1.1 only.** ConnectRPC's Connect protocol works fully over HTTP/1.1. gRPC-Web also works over HTTP/1.1. Only the native gRPC protocol requires HTTP/2. Since the primary consumers are likely API clients and browsers (not gRPC-native microservices), this covers the majority of use cases. Do NOT set `h2_backend = true` in fly.toml. Accept that `grpc-go` clients using the native gRPC protocol cannot connect directly.
- **Option B: Bypass LiteFS proxy for gRPC.** Run the gRPC/ConnectRPC server on a separate port (e.g., 8082) that is not behind LiteFS proxy. Add a second `[[services]]` block in fly.toml with `h2_backend = true` pointing to port 8082. The LiteFS proxy continues to handle REST/GraphQL on port 8080. Downside: adds deployment complexity and the gRPC port does not get LiteFS TXID consistency headers.
- **Option C: Remove LiteFS proxy entirely.** Handle write-forwarding in the application layer (already partially done with `Fly-Replay: leader` header in the sync endpoint). Move TXID consistency tracking into application middleware. This is the cleanest long-term solution but is a significant refactor.

**Detection:** gRPC clients receive connection refused or protocol errors. `curl --http2-prior-knowledge` to port 8080 fails. Application logs show no incoming requests despite Fly.io edge receiving them.

**Confidence:** HIGH -- verified by reading LiteFS proxy source code (`superfly/litefs` main branch, `http/proxy_server.go`). The proxy creates a plain `http.Server` with no h2c support.

---

### Pitfall 2: Protobuf Field Numbers Are Permanent -- Changing Them After Deployment Breaks All Clients

**What goes wrong:** The developer assigns `entproto.Field(n)` annotations to ent schema fields. Later, they reorder fields, add new fields between existing ones, or "clean up" the numbering. Any deployed client that serialized messages using the old field numbers now parses data into wrong fields. A field that was `name` (field 3) silently becomes `notes` (field 3 in the new numbering). No error is raised -- protobuf just puts data in whatever field matches the number.

**Why it happens:** Protobuf uses field numbers (not names) as identifiers in the binary wire format. Field names are cosmetic. Renaming a field is safe; renumbering it is a breaking change. Developers coming from JSON APIs (where field names are the identifiers) do not intuitively understand this. entproto's `entproto.Field(n)` annotation makes it easy to assign numbers, but provides no guardrail against reassignment.

**Consequences:**
- Silent data corruption: clients decode fields into wrong struct members.
- No error at deserialization time -- protobuf is designed for forward/backward compatibility and silently ignores unknown field numbers while mapping known ones.
- Debugging is extremely difficult because the data "looks right" on the server but "looks wrong" on the client, and the binary wire format is not human-readable.
- Real-world example: [Teleport issue #24817](https://github.com/gravitational/teleport/issues/24817) -- a field number change between versions caused a production-breaking backward compatibility issue.

**Prevention:**
- Treat field number assignment as a one-time, permanent decision. Document this in a comment above each schema's `Annotations()` method: "Field numbers are permanent. Never change or reuse a number."
- Use `buf breaking` to detect field number changes between commits. Add to CI. Configure `buf.yaml` with `WIRE` or `WIRE_JSON` breaking change detection level.
- When removing a field, use protobuf `reserved` declarations in the generated `.proto` file to prevent the number from being reused. entproto does not generate `reserved` automatically -- add it manually to the generated proto file and add a CI check that it is not removed.
- Assign field numbers with gaps (2, 3, 4, 10, 11, 12, 20, 21...) to leave room for related fields to be added later without awkward numbering.
- Start field numbering at 2 (entproto reserves 1 for the ID field) and increment sequentially per logical group.

**Detection:** Client-side data appears scrambled or has unexpected values. `buf breaking` fails in CI. Wireshark/protobuf decode shows field numbers that do not match the schema.

**Confidence:** HIGH -- this is fundamental to protobuf wire format and is [extensively documented](https://protobuf.dev/programming-guides/proto3/#assigning) and the [#1 protobuf backward compatibility pitfall](https://earthly.dev/blog/backward-and-forward-compatibility/).

---

### Pitfall 3: otelhttp + otelconnect Double Instrumentation -- Duplicate Spans and Inflated Metrics

**What goes wrong:** The existing middleware stack (main.go line 254) wraps the entire mux with `otelhttp.NewMiddleware("peeringdb-plus")`. When ConnectRPC handlers are mounted on the same mux, each gRPC/Connect request gets instrumented twice: once by otelhttp (which sees the raw HTTP request) and once by otelconnect (which sees the RPC-level request). This produces two spans per RPC call (one named `HTTP POST /grpc.package.Service/Method`, one named `package.Service/Method`), and two sets of metrics (http.server.request.duration AND rpc.server.duration). Dashboards show inflated request counts. Latency percentiles become meaningless because half the data points are parent spans and half are child spans at different granularity.

**Why it happens:** otelhttp and otelconnect instrument at different abstraction layers. otelhttp sees HTTP requests (method, path, status code). otelconnect sees RPC calls (service, method, status, protocol). Both create root server spans. When both are active on the same request path, each creates its own span. The otelconnect documentation acknowledges that "any logging, tracing, or metrics that work with an `http.Handler` or `http.Client` will also work with Connect" but does NOT explicitly warn about double instrumentation or recommend against using both.

**Consequences:**
- Trace views show confusing nested spans: otelhttp span -> otelconnect span -> actual handler.
- Metrics double-count: `http_server_request_duration_seconds` and `rpc_server_duration_seconds` both increment for every RPC call.
- Alerting on request rate fires at 2x the actual rate for gRPC traffic.
- Cardinality explosion: otelhttp attributes (http.route, http.method) plus otelconnect attributes (rpc.service, rpc.method, rpc.system) create a cross-product of label combinations.

**Prevention:**
- **Use route-based filtering on otelhttp.** Configure otelhttp to skip ConnectRPC paths. Since ConnectRPC handlers are mounted at specific path prefixes (e.g., `/grpc.peeringdb.v1.NetworkService/`), use `otelhttp.WithFilter()` to exclude these paths from HTTP-level instrumentation:

```go
otelhttp.NewMiddleware("peeringdb-plus",
    otelhttp.WithFilter(func(r *http.Request) bool {
        // Skip gRPC/ConnectRPC paths -- otelconnect handles these
        return !strings.HasPrefix(r.URL.Path, "/grpc.") &&
               !strings.HasPrefix(r.URL.Path, "/connectrpc.")
    }),
)
```

- **Alternatively, mount ConnectRPC on a separate sub-mux** that is NOT wrapped by otelhttp. Only the REST/GraphQL/web mux gets otelhttp middleware. The ConnectRPC mux gets otelconnect interceptors only.
- Do NOT use both otelhttp and otelconnect on the same request path. Choose one per route group.
- Verify in staging by checking trace output: each RPC call should produce exactly one server span, not two.

**Detection:** Trace viewer shows two root-level server spans per gRPC request. Prometheus metrics show both `http_server_request_duration_seconds` and `rpc_server_duration_seconds` incrementing for the same calls. Request rate dashboards show 2x actual traffic for ConnectRPC endpoints.

**Confidence:** MEDIUM -- the otelconnect documentation does not explicitly warn about this, but the architecture makes double instrumentation inevitable when both middlewares are active on the same handler chain. Verified by reading the middleware application order in main.go (otelhttp wraps the entire mux at line 254) and otelconnect's architecture (interceptor per-handler).

---

### Pitfall 4: CORS Configuration Incomplete for gRPC-Web and Connect Protocols

**What goes wrong:** The existing CORS middleware (internal/middleware/cors.go) allows headers `Content-Type` and `Authorization` with methods `GET`, `POST`, `OPTIONS`. gRPC-Web and Connect protocol require additional headers that are not in the allow list. Browser-based gRPC-Web clients send preflight OPTIONS requests with `Access-Control-Request-Headers` containing `X-Grpc-Web`, `Grpc-Timeout`, `X-User-Agent`, or `Connect-Protocol-Version`. The CORS middleware rejects these preflights. The browser blocks the actual request. The error message is cryptic: "missing trailer" or "fetch failed" in the browser console, not "CORS blocked."

**Why it happens:** The existing CORS config was designed for REST and GraphQL (which only need `Content-Type` and `Authorization`). The gRPC-Web and Connect protocols use custom headers that are not CORS-safelisted. The [ConnectRPC CORS documentation](https://connectrpc.com/docs/cors/) specifies exact header requirements:

- **gRPC-Web requires:** Allow headers: `Content-Type, Grpc-Timeout, X-Grpc-Web, X-User-Agent`. Expose headers: `Grpc-Status, Grpc-Message, Grpc-Status-Details-Bin`.
- **Connect protocol requires:** Allow headers: `Content-Type, Connect-Protocol-Version, Connect-Timeout-Ms, X-User-Agent`. Expose headers: `Grpc-Status, Grpc-Message, Grpc-Status-Details-Bin`.
- **Both require:** POST method allowed. Connect GET requests (cacheable unary RPCs) need GET allowed too.

**Consequences:**
- All browser-based gRPC-Web and Connect clients fail silently.
- Error manifests as "missing trailer" or generic network error, not as a CORS error, because the browser blocks the response before the application can read it.
- Server-side logs show no errors because the OPTIONS preflight never reaches the handler.
- Developers test with curl or grpcurl (which do not enforce CORS) and everything works. Deploy to production, browsers break.

**Prevention:**
- Update the CORS middleware to include all required headers. The ConnectRPC docs provide a canonical list. For this project, update `internal/middleware/cors.go`:

```go
AllowedMethods: []string{"GET", "POST", "OPTIONS"},
AllowedHeaders: []string{
    "Content-Type",
    "Authorization",
    // gRPC-Web headers
    "X-Grpc-Web",
    "Grpc-Timeout",
    "X-User-Agent",
    // Connect protocol headers
    "Connect-Protocol-Version",
    "Connect-Timeout-Ms",
},
ExposedHeaders: []string{
    // gRPC-Web / Connect response headers
    "Grpc-Status",
    "Grpc-Message",
    "Grpc-Status-Details-Bin",
},
```

- Test CORS with a browser-based client, NOT just curl/grpcurl. Use the ConnectRPC TypeScript client (`@connectrpc/connect-web`) for verification.
- If the application uses custom response trailers (unlikely for read-only mirror), expose them with `Trailer-` prefix per ConnectRPC CORS docs.
- Do NOT use wildcard (`*`) for AllowedHeaders in production -- it does not work with credentials and some browsers handle it inconsistently.

**Detection:** Browser console shows CORS errors on gRPC-Web or Connect requests. Network tab shows OPTIONS preflight returning 200 but without the required `Access-Control-Allow-Headers`. gRPC-Web client reports "missing trailer."

**Confidence:** HIGH -- the required headers are [documented by ConnectRPC](https://connectrpc.com/docs/cors/) and the existing CORS config (verified in `internal/middleware/cors.go` line 26-27) does not include them.

---

## Moderate Pitfalls

### Pitfall 5: entproto Enum Naming Does Not Follow buf Conventions -- buf lint Fails

**What goes wrong:** entproto generates enum values without the enum type name as a prefix. For example, a `View` enum generates values `BASIC` and `WITH_EDGE_IDS` instead of `VIEW_BASIC` and `VIEW_WITH_EDGE_IDS`. Running `buf lint` fails with `ENUM_VALUE_PREFIX` violations. This blocks the `buf generate` workflow and prevents using buf's breaking change detection.

**Why it happens:** entproto was designed before buf conventions became widely adopted. [Issue #3063](https://github.com/ent/ent/issues/3063) documents this explicitly: "buf recommends prefixing enum values with their enum name for clarity." The issue also lists other buf incompatibilities: wrong default package name (`entpb` instead of versioned like `peeringdb.v1`), unused imports in generated protos, and generating `generate.go` instead of `buf.yaml`.

**Consequences:**
- `buf lint` fails, preventing integration with buf toolchain.
- Without buf, no automated breaking change detection (critical given Pitfall 2).
- Workaround options all involve post-processing generated files, which is fragile.
- If future entproto versions fix this, existing clients may break because enum wire values change.

**Prevention:**
- Accept buf lint warnings for now. Configure `buf.yaml` to disable `ENUM_VALUE_PREFIX` and `ENUM_ZERO_VALUE_SUFFIX` linting rules for entproto-generated files. These are style rules, not correctness rules -- the wire format uses integers, not names.
- Keep `buf breaking` enabled even with lint warnings disabled. Breaking change detection works on field numbers, not enum value names.
- Set the protobuf package name explicitly using `entproto.Message(entproto.PackageName("peeringdb.v1"))` if supported by entproto, or post-process the generated `.proto` files to set `option go_package` and `package` correctly.
- Monitor [ent/ent #3063](https://github.com/ent/ent/issues/3063) for resolution. If fixed, adopting the new naming requires coordinated client updates.

**Detection:** `buf lint` output shows `ENUM_VALUE_PREFIX` violations. `buf generate` may still succeed (lint is separate from generation).

**Confidence:** HIGH -- verified via [GitHub issue #3063](https://github.com/ent/ent/issues/3063) and [entproto documentation](https://entgo.io/docs/grpc-generating-proto/).

---

### Pitfall 6: entproto Optional/Nillable Fields Generate Wrapper Types -- Increased Message Size and Client Complexity

**What goes wrong:** The ent schema uses `Optional()` and `Nillable()` extensively (the Network schema alone has ~15 optional/nillable fields). entproto maps these to Google well-known wrapper types (`google.protobuf.StringValue`, `google.protobuf.BoolValue`, `google.protobuf.Int32Value`). Each wrapper is a nested message, adding 2-4 bytes of framing overhead per field. For a Network message with 15 wrapper fields, this adds ~30-60 bytes per message. When listing all ~50,000 networks, this is ~1.5-3MB of overhead. More importantly, client-side code becomes verbose: instead of `network.Name`, clients must use `network.GetInfoScope().GetValue()` with nil checks.

**Why it happens:** Protobuf3 has no native concept of "optional" for scalar types (proto3 made all fields optional by default, meaning you cannot distinguish "not set" from "zero value"). entproto uses wrapper types to preserve ent's Optional/Nillable semantics. This is semantically correct but creates ergonomic friction.

**Consequences:**
- Client code is verbose with nil-check boilerplate for every optional field.
- Message size increases for list responses (though the overhead is small compared to the actual data).
- TypeScript/JavaScript clients using `@connectrpc/connect-web` must handle wrapper types differently than plain scalars.
- If proto3 `optional` keyword (available since protobuf 3.15) is used instead, entproto-generated code may not match.

**Prevention:**
- Accept wrapper types for now -- they are semantically correct and the overhead is manageable for a read-only mirror.
- Generate client helper functions or extension methods that unwrap values: `func NetworkName(n *pb.Network) string { return n.GetName().GetValue() }`.
- Consider using proto3 `optional` keyword instead of wrapper types for new fields if entproto adds support. The `optional` keyword was stabilized in protobuf 3.15 and generates `has_*` methods without the wrapper overhead.
- Do NOT manually edit generated `.proto` files to replace wrappers with `optional` -- entproto will overwrite on the next `go generate`.
- Document the wrapper type pattern in API documentation so clients know to expect `StringValue` instead of plain `string` for nullable fields.

**Detection:** Generated `.proto` files contain `import "google/protobuf/wrappers.proto"` and use `google.protobuf.StringValue` instead of `string` for optional fields. Client code requires nested `.GetValue()` calls.

**Confidence:** HIGH -- verified via [entproto optional fields documentation](https://entgo.io/docs/grpc-optional-fields/) and the ent schema in `ent/schema/network.go`.

---

### Pitfall 7: Readiness Middleware Blocks gRPC Health Checks Before First Sync

**What goes wrong:** The readiness middleware (main.go lines 296-321) returns 503 for all paths except `/sync`, `/healthz`, `/readyz`, `/`, and `/static/` until the first sync completes. gRPC health check endpoints (served by ConnectRPC at `/grpc.health.v1.Health/Check`) are not in the bypass list. Kubernetes/Fly.io gRPC health probes fail during initial startup. Load balancers mark the instance as unhealthy. If using Fly.io's machine auto-stop/start, the machine may be stopped before it can sync.

**Why it happens:** The readiness middleware was written before gRPC was added. It uses a hardcoded path allowlist for infrastructure endpoints. gRPC health checks use a different path pattern (`/grpc.health.v1.Health/Check`) that was not anticipated.

**Consequences:**
- gRPC health probes return 503 during startup (before first sync).
- If Fly.io health checks are configured to use gRPC health protocol, the machine is marked unhealthy.
- The HTTP `/healthz` endpoint continues to work (it is in the bypass list), so HTTP health checks are unaffected.
- This only matters if gRPC-specific health checking is configured in the deployment.

**Prevention:**
- Add the gRPC health check path to the readiness bypass list:

```go
if r.URL.Path == "/sync" || r.URL.Path == "/healthz" ||
    r.URL.Path == "/readyz" || r.URL.Path == "/" ||
    strings.HasPrefix(r.URL.Path, "/static/") ||
    strings.HasPrefix(r.URL.Path, "/grpc.health.v1.Health/") {
    next.ServeHTTP(w, r)
    return
}
```

- Use `connectrpc.com/grpchealth` package to serve gRPC health checks over HTTP. It implements the `grpc.health.v1.Health` service and works with `grpc-health-probe` and Kubernetes gRPC liveness probes.
- Keep the existing HTTP `/healthz` and `/readyz` endpoints. They serve different purposes (process liveness vs. data readiness). The gRPC health check can delegate to the same readiness logic.
- Consider a more generic bypass pattern: allow any path starting with `/grpc.health.` rather than hardcoding the exact path, in case additional health service versions are added.

**Detection:** gRPC health probe returns `SERVING: false` or connection refused during startup. Fly.io logs show health check failures before first sync. The standard HTTP `/healthz` returns 200 at the same time.

**Confidence:** HIGH -- verified by reading the readiness middleware implementation in `cmd/peeringdb-plus/main.go` lines 296-321.

---

### Pitfall 8: Fly.io ALPN Misconfiguration -- h2 + http/1.1 Coexistence Issues

**What goes wrong:** When configuring `[http_service.tls_options]` with `alpn = ["h2"]` to support gRPC, Fly.io's edge has been observed to advertise both `h2` and `http/1.1` regardless of configuration. Conversely, setting `alpn = ["h2"]` without `h2_backend = true` causes Fly.io to negotiate HTTP/2 with clients but then downgrade to HTTP/1.1 when forwarding to the backend, breaking gRPC's HTTP/2 requirement. Setting both `alpn = ["h2", "http/1.1"]` and `h2_backend = true` causes ALL traffic (including REST/GraphQL) to be sent as h2c to the backend, which breaks if the backend cannot handle h2c (see Pitfall 1 re: LiteFS proxy).

**Why it happens:** Fly.io's edge proxy handles TLS termination and protocol negotiation independently from backend forwarding. The ALPN setting controls what the edge advertises to clients. The `h2_backend` setting controls how the edge communicates with your backend. These two settings interact in non-obvious ways:

| Client ALPN | h2_backend | Edge -> Backend | Result |
|---|---|---|---|
| h2 | false | HTTP/1.1 | gRPC fails (needs HTTP/2) |
| h2 | true | h2c | gRPC works, but LiteFS proxy breaks |
| h2, http/1.1 | false | HTTP/1.1 | REST works, gRPC fails |
| h2, http/1.1 | true | h2c | Everything uses h2c, LiteFS proxy breaks |

There is also a [known Fly.io bug](https://community.fly.io/t/alpn-offers-h2-http-1-1-even-though-only-h2-is-configured/13434) where ALPN configuration is not correctly honored.

Additionally, when `h2_backend = true` is enabled, a [known issue](https://community.fly.io/t/h2-backend-no-host-header/27100) exists where the `:authority` pseudo-header may not be set correctly, causing framework-specific routing failures.

**Consequences:**
- gRPC clients cannot connect unless h2_backend is true.
- h2_backend = true breaks LiteFS proxy (Pitfall 1).
- Catch-22: cannot serve both gRPC (needs HTTP/2) and REST-behind-LiteFS (needs HTTP/1.1) on the same port/service.

**Prevention:**
- **Accept ConnectRPC over HTTP/1.1** as the primary strategy (Option A from Pitfall 1). ConnectRPC's Connect protocol and gRPC-Web both work over HTTP/1.1. Do not set `h2_backend = true`. Do not change ALPN configuration. Leave `fly.toml` as-is.
- If native gRPC is required, use a **separate Fly.io service** (separate `[[services]]` block) on a different port with `h2_backend = true`, bypassing LiteFS proxy entirely.
- Do NOT attempt to use `alpn = ["h2"]` without `h2_backend = true` -- it will negotiate HTTP/2 with clients but fail on the backend.
- Monitor Fly.io community forum for ALPN bug fixes.

**Detection:** gRPC clients receive protocol errors or timeouts. `fly logs` show connection resets. REST/GraphQL endpoints suddenly break when `h2_backend` is enabled.

**Confidence:** HIGH -- verified via [Fly.io networking docs](https://fly.io/docs/networking/services/), [Fly.io gRPC guide](https://fly.io/docs/app-guides/grpc-and-grpc-web-services/), and community-reported issues.

---

### Pitfall 9: entproto Edge Field Numbers Collide with Schema Field Numbers

**What goes wrong:** entproto requires explicit `entproto.Field(n)` annotations on both schema fields AND edges. Field number 1 is reserved for the ID field. If a developer assigns field numbers 2-40 to the schema's ~39 fields and then assigns edge field numbers starting at 2, the numbers collide. Protobuf requires all field numbers within a message to be unique. The collision causes `entproto` code generation to fail with a cryptic error or, worse, silently generates invalid proto definitions that `protoc` rejects.

**Why it happens:** The Network schema has ~39 fields (counting all optional, computed, and common fields). Edges need their own field numbers in the same protobuf message. Developers naturally start edge numbering at "the next number" but may miscalculate, or may not realize edges share the same number space as fields.

For the Network schema specifically:
- Fields: `id` (1), then 38 more fields (2-39)
- Edges: `network_facilities`, `network_ix_lans`, `organization`, `pocs` (4 edges needing numbers 40-43)
- If a developer assigns edges starting at 30 (thinking there are only 29 fields), numbers 30-33 collide.

**Consequences:**
- `go generate` fails with confusing protobuf compilation errors.
- If the collision is between fields of different types, protoc may produce corrupt Go code that compiles but deserializes incorrectly.
- Time wasted debugging field number collisions across 15+ schema files.

**Prevention:**
- Use a systematic numbering convention. Recommended approach:
  - Fields: 2 through N (where N is the number of fields + 1)
  - Edges: start at 100 (or the next hundred after the field count)
  - This leaves a gap for adding fields without renumbering edges.
- Count fields carefully for each schema before assigning edge numbers. The Network schema has 39 fields (2-40), so edges should start at 50 or 100.
- Add a CI check that parses generated `.proto` files and verifies all field numbers within each message are unique.
- Consider writing a helper function or linter that reads ent schema annotations and validates field number uniqueness across fields and edges.

**Detection:** `go generate` fails with protobuf compilation errors mentioning duplicate field numbers. `buf lint` reports `FIELD_NUMBER_*` violations.

**Confidence:** HIGH -- this is a structural requirement of protobuf, and the ent schema files show 30-40+ fields per entity. The risk of miscounting is high.

---

### Pitfall 10: entproto Does Not Handle JSON Fields ([]string, []SocialMedia) -- Code Generation Fails or Produces Unusable Types

**What goes wrong:** Several ent schema fields use `field.JSON("info_types", []string{})` and `field.JSON("social_media", []SocialMedia{})`. entproto has limited support for JSON fields. A `[]string` field may be mapped to `repeated string` (which is correct), but a custom struct type like `[]SocialMedia` has no automatic protobuf mapping. entproto may fail to generate the proto definition, generate an opaque `bytes` field (losing type information), or require manual `entproto.OverrideType()` annotations to specify the mapping.

**Why it happens:** JSON fields in ent are stored as JSON text in SQLite and deserialized into Go types. Protobuf has no equivalent of "arbitrary JSON" -- every field must have a defined type. For `[]string`, the mapping to `repeated string` is straightforward. For custom struct types, entproto must generate a nested message type or the developer must define one manually.

**Consequences:**
- `go generate` fails for schemas with complex JSON fields.
- If `bytes` is used as a fallback, clients receive raw JSON bytes instead of typed protobuf messages and must deserialize manually, defeating the purpose of protobuf.
- The `SocialMedia` type is used by 6 schemas (Organization, Network, Facility, InternetExchange, Carrier, Campus), so the fix must be applied consistently.

**Prevention:**
- Define a `SocialMedia` protobuf message manually in a shared `.proto` file:

```protobuf
message SocialMedia {
  string service = 1;
  string identifier = 2;
}
```

- Use `entproto.OverrideType()` on the `social_media` field to reference this message type, or exclude the field from protobuf generation and handle it separately.
- For `[]string` fields like `info_types`, verify that entproto maps them to `repeated string`. If not, use `entproto.OverrideType()`.
- Test code generation early in the phase -- do not wait until all field numbers are assigned. Run `go generate` after annotating the first schema to verify the pipeline works.
- Consider skipping complex JSON fields from the protobuf API entirely if they are not needed by gRPC clients. Use `entproto.SkipGen()` annotation if available.

**Detection:** `go generate` fails with "unsupported type" or "cannot map type" errors from entproto. Generated `.proto` files contain `bytes` instead of typed fields for JSON columns.

**Confidence:** MEDIUM -- entproto documentation does not explicitly cover JSON field handling. The behavior depends on the specific entproto version. Needs validation during implementation.

---

## Minor Pitfalls

### Pitfall 11: buf generate and go generate Pipeline Ordering -- Stale Protobuf Files

**What goes wrong:** The project has two code generation steps: `go generate ./ent/...` (runs entc, which generates ent code AND entproto `.proto` files) and `buf generate` (compiles `.proto` files into Go code). If `buf generate` runs before or concurrently with `go generate`, it uses stale `.proto` files from a previous run. The generated Go code does not match the current ent schema. Compilation fails with type mismatches or missing methods.

**Why it happens:** The project already has `//go:generate` directives in `ent/generate.go`. Adding protobuf generation creates a two-stage pipeline where stage 2 (buf) depends on stage 1 (entproto) output. `go generate ./...` runs all generate directives but does not guarantee ordering across packages. If `buf generate` is also triggered by a `//go:generate` directive, its execution order relative to entproto is undefined.

**Consequences:**
- Build fails intermittently depending on execution order.
- Developer runs `go generate` twice to "fix" it, masking the ordering issue.
- CI builds fail on clean checkouts because generated files are not committed or are stale.

**Prevention:**
- Use a Taskfile (or Makefile) that explicitly sequences the steps:

```yaml
tasks:
  generate:
    cmds:
      - go generate ./ent/...    # Stage 1: ent schemas + entproto .proto files
      - buf generate              # Stage 2: .proto -> .pb.go + connect handlers
      - go generate ./graph/...   # Stage 3: gqlgen (if needed after proto changes)
    desc: "Run all code generation in correct order"
```

- Do NOT put `buf generate` in a `//go:generate` directive. Keep it in the Taskfile only.
- Commit generated `.proto` files and `.pb.go` files to version control. This ensures `go build` works without running the full generation pipeline, and `buf breaking` can compare against the committed protos.
- Add a CI step that runs the full generation pipeline and verifies no diff: `task generate && git diff --exit-code`.

**Detection:** Build fails with "undefined: pb.NetworkServiceHandler" or similar missing type errors. Running `go generate` twice fixes the issue.

**Confidence:** HIGH -- this is a standard multi-stage code generation issue. The existing `ent/generate.go` and `ent/entc.go` confirm the project already uses `go generate` for ent.

---

### Pitfall 12: ConnectRPC Handlers Use Different Path Patterns Than REST/GraphQL -- Route Conflicts

**What goes wrong:** ConnectRPC handlers are mounted at paths like `/grpc.peeringdb.v1.NetworkService/GetNetwork`. The existing REST API is at `/rest/v1/networks/{id}`. The existing GraphQL is at `/graphql`. The PeeringDB compat API is at `/api/net/{id}`. No conflicts exist between these prefixes. However, if the protobuf package name is not namespaced (e.g., `package entpb` instead of `package peeringdb.v1`), the handler paths become `/entpb.NetworkService/GetNetwork`, which is ugly but functional. The real issue is that Go 1.22+ ServeMux pattern matching treats gRPC paths as literal path matches, not pattern matches. Registering `/grpc.peeringdb.v1.NetworkService/` as a prefix catches all methods for that service, which is correct. But if a path like `/grpc.peeringdb.v1.NetworkService/GetNetwork` is registered AND a catch-all `/grpc.peeringdb.v1.NetworkService/` is also registered, Go's ServeMux uses longest-match, which may not be the intended behavior for gRPC routing.

**Why it happens:** ConnectRPC's `NewHandler` returns a path prefix and an http.Handler. The standard pattern is `mux.Handle(path, handler)` where `path` is the service path prefix. This works correctly with `http.ServeMux`. The pitfall is when developers register handlers incorrectly or when other middleware intercepts the gRPC paths.

**Consequences:**
- Minor: path aesthetics if package name is not properly namespaced.
- Moderate: routing conflicts if gRPC paths overlap with existing route patterns (unlikely given the distinct prefixes, but possible with catch-all routes).
- The readiness middleware (Pitfall 7) intercepts gRPC paths because it checks all non-infrastructure paths.

**Prevention:**
- Use a properly namespaced protobuf package: `package peeringdb.v1` not `package entpb`.
- Register ConnectRPC handlers exactly as the library returns them: `path, handler := svcConnect.NewNetworkServiceHandler(svc); mux.Handle(path, handler)`.
- Do not add trailing slash or modify the path returned by ConnectRPC.
- Verify all registered routes do not conflict by printing the mux routes at startup (Go 1.22+ ServeMux does not have a route listing method, so log each registration).

**Detection:** gRPC calls return 404 Not Found. Route registration logs show unexpected path patterns.

**Confidence:** MEDIUM -- ConnectRPC's route registration is straightforward and well-documented. The risk is low but worth noting for completeness.

---

### Pitfall 13: entproto Requires Explicit Annotations on Every Field and Edge -- Missing Annotations Cause Silent Omission

**What goes wrong:** entproto only generates protobuf definitions for fields and edges that have `entproto.Field(n)` annotations. Fields without annotations are silently omitted from the generated `.proto` file. The developer adds `entproto.Message()` to the schema's `Annotations()` and runs `go generate`. The proto file is generated but is missing half the fields. The developer does not notice because `go generate` succeeds without warnings.

**Why it happens:** entproto's design requires opt-in at the field level, not just the schema level. This is documented but easy to miss. With 30-40 fields per schema and 15+ schemas, annotating every field is tedious and error-prone.

**Consequences:**
- Generated protobuf messages are incomplete.
- gRPC clients receive messages with missing data.
- The omission is silent -- no error, no warning. Only careful comparison of the `.proto` file against the ent schema reveals the gap.
- Adding annotations later changes field numbers if not done carefully, potentially breaking clients (see Pitfall 2).

**Prevention:**
- Write a CI check that compares the number of fields in each ent schema against the number of fields in the generated `.proto` message. Flag schemas where the counts diverge by more than expected (some fields like computed/internal fields may be intentionally excluded).
- When annotating schemas, work through them systematically: open the schema file and the generated `.proto` file side-by-side. Verify each field appears.
- Consider writing a linter or generator helper that auto-assigns field numbers to unannotated fields (starting from the maximum existing number + 1). This reduces manual annotation burden but must be used carefully to avoid number instability.
- Document which fields are intentionally excluded from protobuf generation and why.

**Detection:** Generated `.proto` file has fewer fields than expected. gRPC client receives messages with zero-valued or missing fields that have data in the REST/GraphQL APIs.

**Confidence:** HIGH -- this is fundamental to entproto's design. Verified via [entproto documentation](https://entgo.io/docs/grpc-generating-proto/): "annotations must be supplied on the schema to help entproto determine how to generate Protobuf definitions."

---

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Mitigation |
|-------------|---------------|------------|
| LiteFS + h2c architecture decision | LiteFS proxy is HTTP/1.1 only (#1, #8) | Use ConnectRPC over HTTP/1.1 (Connect protocol + gRPC-Web). Do NOT set h2_backend = true. Accept that native gRPC protocol is not supported through LiteFS proxy. |
| entproto schema annotation | Missing annotations (#13), field number collisions (#9), JSON field types (#10) | Annotate all fields systematically. Use numbering gaps between fields and edges (fields 2-N, edges 100+). Test code generation early. |
| Protobuf field number assignment | Permanent field numbers (#2) | Treat as one-time decision. Use `buf breaking` in CI. Reserve removed field numbers. |
| entproto + buf integration | Enum naming (#5), pipeline ordering (#11) | Disable buf lint for enum prefix rules. Use Taskfile for ordered generation. Commit generated files. |
| Optional field handling | Wrapper types (#6) | Accept wrapper types. Generate client helpers. Document in API docs. |
| CORS for gRPC-Web/Connect | Missing headers (#4) | Update CORS config with gRPC-Web and Connect protocol headers. Test with browser client. |
| Observability instrumentation | Double spans/metrics (#3) | Filter ConnectRPC paths from otelhttp. Use otelconnect interceptors for gRPC routes only. |
| Health checking | Readiness middleware blocks gRPC health (#7) | Add gRPC health path to readiness bypass list. Use connectrpc.com/grpchealth. |
| Route registration | Path conflicts (#12) | Use namespaced package name. Register handlers as ConnectRPC returns them. |

## Sources

### ConnectRPC / gRPC
- [ConnectRPC Deployment & h2c](https://connectrpc.com/docs/go/deployment/) -- h2c configuration, HTTP/1.1 coexistence
- [ConnectRPC CORS Documentation](https://connectrpc.com/docs/cors/) -- required CORS headers for gRPC-Web and Connect
- [ConnectRPC Observability](https://connectrpc.com/docs/go/observability/) -- otelconnect instrumentation
- [ConnectRPC gRPC Compatibility](https://connectrpc.com/docs/go/grpc-compatibility/) -- wire compatibility with grpc-go
- [ConnectRPC gRPC Health](https://github.com/connectrpc/grpchealth-go) -- gRPC-compatible health checks for net/http
- [gRPC Health Checking Protocol](https://grpc.io/docs/guides/health-checking/) -- standard health check service

### Protobuf
- [Protobuf Field Number Assignment](https://protobuf.dev/programming-guides/proto3/#assigning) -- permanent field numbers
- [Protobuf Backward Compatibility Best Practices](https://earthly.dev/blog/backward-and-forward-compatibility/) -- field number stability, reserved keyword
- [Teleport Issue #24817](https://github.com/gravitational/teleport/issues/24817) -- real-world field number breakage

### entproto
- [entproto Code Generation Guide](https://entgo.io/docs/grpc-generating-proto/) -- field numbering, annotation requirements
- [entproto Optional Fields](https://entgo.io/docs/grpc-optional-fields/) -- wrapper type handling
- [entproto Edge Handling](https://entgo.io/docs/grpc-edges/) -- edge field numbers, unique vs repeated
- [ent/ent Issue #3063](https://github.com/ent/ent/issues/3063) -- buf compatibility issues (enum naming, package versioning)

### Fly.io
- [Fly.io gRPC and gRPC-Web Guide](https://fly.io/docs/app-guides/grpc-and-grpc-web-services/) -- h2_backend configuration
- [Fly.io Public Network Services](https://fly.io/docs/networking/services/) -- h2_backend option, HTTP/2 backend
- [Fly.io ALPN Bug Report](https://community.fly.io/t/alpn-offers-h2-http-1-1-even-though-only-h2-is-configured/13434) -- ALPN configuration not honored
- [Fly.io h2_backend Host Header Issue](https://community.fly.io/t/h2-backend-no-host-header/27100) -- missing :authority pseudo-header

### LiteFS
- [LiteFS Source: proxy_server.go](https://github.com/superfly/litefs/blob/main/http/proxy_server.go) -- HTTP/1.1-only proxy server implementation
- [LiteFS Architecture](https://github.com/superfly/litefs/blob/main/docs/ARCHITECTURE.md) -- proxy design

### Go HTTP/2
- [Go 1.24 h2c Support](https://github.com/golang/go/issues/67816) -- stdlib h2c via http.Protocols
- [x/net/http2/h2c Deprecation](https://github.com/golang/go/issues/72039) -- h2c package deprecated in favor of stdlib
- [VictoriaMetrics: How HTTP/2 Works in Go](https://victoriametrics.com/blog/go-http2/) -- http.Protocols.SetUnencryptedHTTP2

### OpenTelemetry
- [otelconnect Package](https://pkg.go.dev/connectrpc.com/otelconnect) -- RPC-level OTel instrumentation
- [otelhttp Package](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp) -- HTTP-level OTel instrumentation

### Codebase (verified against source)
- `cmd/peeringdb-plus/main.go` -- middleware stack (lines 249-256), readiness middleware (lines 296-321), route registration
- `internal/middleware/cors.go` -- CORS allowed headers (line 26-27): only Content-Type and Authorization
- `ent/schema/network.go` -- 39 fields, 4 edges, JSON fields (info_types, social_media)
- `ent/schema/types.go` -- SocialMedia struct definition
- `ent/entc.go` -- code generation setup with entgql and entrest extensions (no entproto yet)
- `fly.toml` -- LiteFS proxy on port 8080, app on port 8081, no h2_backend setting
- `litefs.yml` -- proxy configuration (port 8080 -> 8081), passthrough paths
- `internal/otel/provider.go` -- otelhttp middleware setup
