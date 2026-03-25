# Architecture Patterns

**Domain:** ConnectRPC / gRPC API surface integration into existing PeeringDB Plus
**Researched:** 2026-03-24
**Focus:** How ConnectRPC handlers, proto generation, h2c, observability, and gRPC ecosystem services integrate with the existing single-binary HTTP architecture

## Existing Architecture (Context)

The current architecture is a single Go binary (`cmd/peeringdb-plus/main.go`) with this request flow:

```
Fly.io Edge (TLS termination, HTTP/2 -> HTTP/1.1)
  |
  v
LiteFS Proxy (:8080, HTTP/1.1)
  - Write forwarding via Fly-Replay header
  - TXID cookie for read-your-writes consistency
  - Passthrough: /healthz, /readyz
  |
  v
Go http.Server (:8081, HTTP/1.1)
  |
  Middleware stack (outermost first):
  |  Recovery -> otelhttp -> Logging -> CORS -> Readiness
  |
  http.NewServeMux()
    POST /sync         (on-demand sync trigger)
    GET  /healthz      (liveness)
    GET  /readyz       (readiness + sync freshness)
    GET  /graphql      (GraphiQL playground)
    POST /graphql      (GraphQL API via gqlgen)
    /rest/v1/          (entrest-generated OpenAPI REST)
    /api/              (PeeringDB-compatible REST)
    /ui/               (templ + htmx web UI)
    /static/           (embedded static assets)
    GET  /              (content negotiation: HTML -> /ui/, JSON -> discovery)
```

**Key constraints for ConnectRPC integration:**
- LiteFS proxy is HTTP/1.1 only (uses `http.Transport` internally, no h2c)
- All traffic between Fly edge and app traverses LiteFS proxy
- Existing middleware stack wraps the entire mux
- 13 ent schemas (Organization, Network, Facility, InternetExchange, etc.)
- Read-only mirror -- all RPCs will be unary Get/List operations

## Recommended Architecture

### The LiteFS Proxy Constraint (Critical)

The LiteFS proxy at `:8080` uses Go's `http.Transport` to forward requests to the app at `localhost:8081`. This transport does **not** support h2c (unencrypted HTTP/2). The `ForceAttemptHTTP2: true` setting only enables HTTP/2 over TLS connections, which is not applicable for localhost forwarding.

**This means the gRPC wire protocol (which requires HTTP/2) cannot traverse the LiteFS proxy.**

However, ConnectRPC handlers support three wire protocols simultaneously via Content-Type detection:
1. **Connect protocol** -- Works over HTTP/1.1 for unary RPCs. No HTTP trailers needed.
2. **gRPC protocol** -- Requires HTTP/2. Will NOT work through LiteFS proxy.
3. **gRPC-Web protocol** -- Works over HTTP/1.1 (no trailers). Will work through LiteFS proxy.

Since PeeringDB Plus is a read-only mirror, all RPCs are unary (Get, List). This means:
- **Connect protocol**: Fully functional through LiteFS proxy (HTTP/1.1 unary)
- **gRPC-Web protocol**: Fully functional through LiteFS proxy (HTTP/1.1 unary)
- **gRPC protocol**: Only functional if the app enables h2c on its listener AND Fly's `h2_backend = true` is set AND LiteFS proxy is bypassed for these paths

**Recommendation:** Accept that the gRPC wire protocol will not work through LiteFS proxy. The Connect protocol and gRPC-Web protocol cover all practical use cases for a read-only API:
- Browser clients use Connect (JSON) or gRPC-Web
- CLI tools (grpcurl, buf curl) can use Connect protocol
- Go/Python/TypeScript clients use Connect protocol
- Only native `grpc-go` clients using `grpc.Dial()` require HTTP/2, and those clients can be configured to use Connect protocol instead

If native gRPC wire protocol support is later required, the options are:
1. Run a separate listener that bypasses LiteFS proxy (dual-port architecture)
2. Replace LiteFS proxy with application-level write forwarding (already partially implemented via Fly-Replay header)

### Architecture After Integration

```
Fly.io Edge (TLS termination)
  |
  v
LiteFS Proxy (:8080, HTTP/1.1)
  |
  v
Go http.Server (:8081)
  |
  Middleware stack:
  |  Recovery -> otelhttp -> Logging -> CORS -> Readiness
  |
  http.NewServeMux()
    [existing routes unchanged]
    POST /sync, GET /healthz, GET /readyz
    /graphql, /rest/v1/, /api/, /ui/, /static/, GET /
    |
    [new ConnectRPC routes]
    /peeringdb.v1.NetworkService/       (Connect handler)
    /peeringdb.v1.OrganizationService/  (Connect handler)
    /peeringdb.v1.FacilityService/      (Connect handler)
    /peeringdb.v1.InternetExchangeService/  (Connect handler)
    ... (one per ent entity with Service annotation)
    |
    [new gRPC ecosystem routes]
    /grpc.reflection.v1.ServerReflection/        (grpcreflect V1)
    /grpc.reflection.v1alpha.ServerReflection/   (grpcreflect V1Alpha)
    /grpc.health.v1.Health/                      (grpchealth)
```

### Component Boundaries

| Component | Responsibility | New/Modified | Communicates With |
|-----------|---------------|--------------|-------------------|
| `ent/schema/*.go` | Add `entproto.Message()`, `entproto.Field(N)` annotations | Modified | entproto code generator |
| `ent/entc.go` | Add entproto extension with `SkipGenFile()` | Modified | ent code generation |
| `ent/generate.go` | Add entproto generate directive | Modified | go generate toolchain |
| `proto/peeringdb/v1/*.proto` | Generated .proto files from entproto | New (generated) | buf generate |
| `buf.yaml` | Buf module configuration | New | buf CLI |
| `buf.gen.yaml` | Code generation config: protoc-gen-go + protoc-gen-connect-go | New | buf generate |
| `gen/peeringdb/v1/*.pb.go` | Generated protobuf Go types | New (generated) | service handlers |
| `gen/peeringdb/v1/peeringdbv1connect/*.go` | Generated ConnectRPC handler interfaces | New (generated) | service handlers |
| `internal/connectrpc/` | Hand-written service implementations | New | ent client, gen types |
| `internal/connectrpc/network.go` | NetworkService: Get, List | New | ent.Client |
| `internal/connectrpc/organization.go` | OrganizationService: Get, List | New | ent.Client |
| `internal/connectrpc/facility.go` | FacilityService: Get, List | New | ent.Client |
| `internal/connectrpc/interceptors.go` | Shared interceptors (logging, readonly enforcement) | New | connect.Interceptor |
| `cmd/peeringdb-plus/main.go` | Mount ConnectRPC handlers on mux | Modified | all components |

### Data Flow

**Proto generation flow (build time):**

```
ent/schema/*.go (entproto annotations)
    |
    v
go generate ./ent/...
    |  entproto cmd generates .proto files
    v
proto/peeringdb/v1/*.proto
    |
    v
buf generate
    |  protoc-gen-go -> gen/peeringdb/v1/*.pb.go
    |  protoc-gen-connect-go -> gen/peeringdb/v1/peeringdbv1connect/*.go
    v
Generated Go types + ConnectRPC handler interfaces
```

**Request flow (runtime, Connect protocol over HTTP/1.1):**

```
Client: POST /peeringdb.v1.NetworkService/GetNetwork
Content-Type: application/proto (or application/json)
    |
    v
LiteFS Proxy (HTTP/1.1, passthrough -- not a write)
    |
    v
otelhttp middleware (creates HTTP span)
    |
    v
ConnectRPC handler (otelconnect interceptor creates RPC child span)
    |
    v
internal/connectrpc/network.go GetNetwork()
    |  Queries ent.Client
    v
ent -> SQLite (read from LiteFS FUSE mount)
    |
    v
Protobuf/JSON response back through middleware stack
```

## Patterns to Follow

### Pattern 1: Handler Mounting (ConnectRPC on stdlib mux)

ConnectRPC handler constructors return `(path string, handler http.Handler)`, which mount directly on `http.NewServeMux()`:

```go
// In cmd/peeringdb-plus/main.go

// Create otelconnect interceptor (RPC-level tracing)
otelInterceptor, err := otelconnect.NewInterceptor(
    otelconnect.WithoutServerPeerAttributes(), // reduce cardinality
)
if err != nil {
    logger.Error("failed to create otelconnect interceptor", slog.String("error", err.Error()))
    os.Exit(1)
}

// Mount ConnectRPC service handlers
networkSvc := connectrpc.NewNetworkService(entClient)
path, handler := peeringdbv1connect.NewNetworkServiceHandler(
    networkSvc,
    connect.WithInterceptors(otelInterceptor),
)
mux.Handle(path, handler)

// Mount gRPC reflection (both V1 and V1Alpha for tool compatibility)
reflector := grpcreflect.NewStaticReflector(
    "peeringdb.v1.NetworkService",
    "peeringdb.v1.OrganizationService",
    // ... all service names
)
mux.Handle(grpcreflect.NewHandlerV1(reflector))
mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))

// Mount gRPC health check
checker := grpchealth.NewStaticChecker(
    "peeringdb.v1.NetworkService",
    "peeringdb.v1.OrganizationService",
    // ... all service names
)
mux.Handle(grpchealth.NewHandler(checker))
```

**Why this works:** ConnectRPC handlers are plain `http.Handler` implementations. They share the same mux as GraphQL, REST, and web UI handlers. No separate server or port needed. The existing middleware stack (otelhttp, CORS, recovery, logging) wraps everything uniformly.

### Pattern 2: Service Implementation (Hand-Written, Querying Ent)

Do NOT use entproto's `protoc-gen-entgrpc` plugin. It generates gRPC service implementations targeting `google.golang.org/grpc`, not ConnectRPC. Instead, write service handlers that implement the ConnectRPC-generated interface and query ent directly:

```go
// internal/connectrpc/network.go
package connectrpc

import (
    "context"

    "connectrpc.com/connect"
    "github.com/dotwaffle/peeringdb-plus/ent"
    peeringdbv1 "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
    "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1/peeringdbv1connect"
)

// NetworkService implements the peeringdb.v1.NetworkService ConnectRPC interface.
type NetworkService struct {
    peeringdbv1connect.UnimplementedNetworkServiceHandler
    client *ent.Client
}

// NewNetworkService creates a new NetworkService handler.
func NewNetworkService(client *ent.Client) *NetworkService {
    return &NetworkService{client: client}
}

// GetNetwork retrieves a single network by ID.
func (s *NetworkService) GetNetwork(
    ctx context.Context,
    req *connect.Request[peeringdbv1.GetNetworkRequest],
) (*connect.Response[peeringdbv1.Network], error) {
    net, err := s.client.Network.Get(ctx, int(req.Msg.Id))
    if err != nil {
        if ent.IsNotFound(err) {
            return nil, connect.NewError(connect.CodeNotFound, err)
        }
        return nil, connect.NewError(connect.CodeInternal, err)
    }
    return connect.NewResponse(networkToProto(net)), nil
}
```

**Why hand-written:** entproto's `protoc-gen-entgrpc` generates implementations for `google.golang.org/grpc`, not ConnectRPC. The conversion layer (ent model -> protobuf message) must be written regardless. Hand-written handlers give full control over query behavior (eager loading, filtering, pagination) and error mapping.

### Pattern 3: OTel Instrumentation (otelconnect + otelhttp Coexistence)

Use **both** `otelhttp` (outer middleware) and `otelconnect` (interceptor) together. They instrument at different semantic levels:

- `otelhttp`: Creates HTTP-level spans (`HTTP GET /peeringdb.v1.NetworkService/GetNetwork`) with HTTP attributes (method, status code, content length). Applied uniformly to ALL routes.
- `otelconnect`: Creates RPC-level child spans (`peeringdb.v1.NetworkService/GetNetwork`) with RPC attributes (rpc.system, rpc.service, rpc.method, error code). Applied only to ConnectRPC handlers.

This produces a trace hierarchy: `HTTP span -> RPC span -> ent query spans`. This is the recommended approach per ConnectRPC documentation -- they "integrate seamlessly."

```go
// otelconnect interceptor configuration
otelInterceptor, err := otelconnect.NewInterceptor(
    otelconnect.WithoutServerPeerAttributes(), // drop IP/port for cardinality
    // Do NOT use WithTrustRemote() -- this is internet-facing
)
```

**Do NOT remove otelhttp from the middleware stack.** It instruments non-ConnectRPC routes (GraphQL, REST, UI) and provides the parent span for ConnectRPC requests.

### Pattern 4: buf.gen.yaml Configuration

```yaml
# buf.gen.yaml
version: v2
plugins:
  - local: protoc-gen-go
    out: gen
    opt:
      - paths=source_relative
  - local: protoc-gen-connect-go
    out: gen
    opt:
      - paths=source_relative
```

```yaml
# buf.yaml
version: v2
modules:
  - path: proto
deps:
  - buf.build/googleapis/googleapis
lint:
  use:
    - STANDARD
breaking:
  use:
    - WIRE_JSON
```

**Why NOT protoc-gen-go-grpc:** The project uses ConnectRPC, not google.golang.org/grpc for the server. `protoc-gen-connect-go` generates handler interfaces and client constructors that return `http.Handler`, which is what we need. `protoc-gen-go-grpc` generates interfaces for `grpc.Server` which would require a separate gRPC server and port.

### Pattern 5: entproto Schema Annotations

Add `entproto.Message()` and `entproto.Field(N)` annotations to ent schemas. Use `entproto.Service()` with `Methods(entproto.MethodGet | entproto.MethodList)` to generate service definitions in the .proto files:

```go
// ent/schema/network.go -- additions to existing Annotations()
func (Network) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entgql.RelayConnection(),
        entgql.QueryField(),
        entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
        // New: proto generation
        entproto.Message(),
        entproto.Service(entproto.Methods(entproto.MethodGet | entproto.MethodList)),
    }
}
```

Each field needs `entproto.Field(N)` with a unique field number (starting from 2, since 1 is reserved for ID):

```go
field.String("name").
    NotEmpty().
    Unique().
    Annotations(
        entgql.OrderField("NAME"),
        entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray),
        entproto.Field(2), // New
    ).
    Comment("Network name"),
```

**SkipGenFile is essential:** By default, entproto generates a `generate.go` file next to each `.proto` that invokes `protoc` with `protoc-gen-go-grpc`. We explicitly skip this because we use `buf generate` with `protoc-gen-connect-go` instead:

```go
// ent/entc.go -- add entproto extension
protoExt, err := entproto.NewExtension(entproto.SkipGenFile())
if err != nil {
    log.Fatalf("creating entproto extension: %v", err)
}

opts := []entc.Option{
    entc.Extensions(gqlExt, restExt, protoExt),
    entc.FeatureNames("sql/upsert"),
}
```

### Pattern 6: Proto-to-Ent Conversion

Create bidirectional conversion functions in each service file. Since we only need ent-to-proto (read-only), keep it simple:

```go
// internal/connectrpc/convert.go

func networkToProto(n *ent.Network) *peeringdbv1.Network {
    return &peeringdbv1.Network{
        Id:    int32(n.ID),
        Name:  n.Name,
        Asn:   int32(n.Asn),
        // ... map all fields
    }
}
```

**Nullable fields:** ent uses pointer types for Optional+Nillable fields. Proto uses wrapper types or optional keyword. Map nil ent fields to zero-value proto fields or use `google.protobuf.StringValue` wrappers. Decide per field based on whether nil vs empty has semantic meaning.

### Pattern 7: Readiness Gating

The existing readiness middleware gates all non-infrastructure routes until the first sync completes. ConnectRPC paths (`/peeringdb.v1.*`) should be gated by this middleware. No changes needed -- the readiness middleware already applies to all routes not in the bypass list (`/sync`, `/healthz`, `/readyz`, `/`, `/static/`).

gRPC health checks (`/grpc.health.v1.Health/Check`) should **also** be gated. The `grpchealth.NewStaticChecker()` always returns SERVING status. For dynamic health that reflects sync readiness, implement a custom checker:

```go
type grpcHealthChecker struct {
    syncReady func() bool
}

func (c *grpcHealthChecker) Check(ctx context.Context, req *connect.Request[healthv1.HealthCheckRequest]) (*connect.Response[healthv1.HealthCheckResponse], error) {
    status := healthv1.HealthCheckResponse_SERVING
    if !c.syncReady() {
        status = healthv1.HealthCheckResponse_NOT_SERVING
    }
    return connect.NewResponse(&healthv1.HealthCheckResponse{
        Status: status,
    }), nil
}
```

## Anti-Patterns to Avoid

### Anti-Pattern 1: Using protoc-gen-entgrpc

**What:** Using entproto's `protoc-gen-entgrpc` to auto-generate service implementations.

**Why bad:** It generates implementations targeting `google.golang.org/grpc` server interfaces, not ConnectRPC. The generated code expects to be registered with `grpc.NewServer()`, which requires a separate gRPC server on a different port. It would bypass the existing HTTP middleware stack (otelhttp, CORS, recovery, logging, readiness).

**Instead:** Use entproto only for `.proto` file generation (messages + service definitions). Use `buf generate` with `protoc-gen-connect-go` for Go code. Write service implementations by hand.

### Anti-Pattern 2: Running a Separate gRPC Server

**What:** Starting `grpc.NewServer()` on a separate port alongside the HTTP server.

**Why bad:** Doubles operational complexity (two ports, two health checks, two middleware stacks). Cannot share the existing otelhttp instrumentation. Requires separate Fly.io service configuration. Bypasses LiteFS proxy write forwarding.

**Instead:** Mount ConnectRPC handlers on the same `http.NewServeMux()` as all other routes. Single port, single middleware stack, single health check surface.

### Anti-Pattern 3: Enabling h2c Without Understanding the LiteFS Constraint

**What:** Setting `http.Protocols{}.SetUnencryptedHTTP2(true)` on the Go server and `h2_backend = true` in fly.toml, expecting gRPC wire protocol to work.

**Why bad:** The LiteFS proxy between Fly edge and the app is HTTP/1.1 only. Even if the Go server supports h2c, the LiteFS proxy downgrades the connection. Clients using the gRPC wire protocol will get protocol errors. The error messages will be confusing ("unexpected HTTP/1.1 response").

**Instead:** Rely on the Connect protocol (HTTP/1.1 compatible) for all clients. Document that the gRPC wire protocol is not supported. Do NOT set `h2_backend = true` in fly.toml -- it will break LiteFS proxy's ability to parse and forward requests.

### Anti-Pattern 4: Duplicating CORS Configuration

**What:** Adding CORS handling inside ConnectRPC interceptors in addition to the existing CORS middleware.

**Why bad:** The existing `middleware.CORS()` already wraps the entire mux, including ConnectRPC handlers. Adding CORS in interceptors creates conflicting headers. The Connect protocol uses standard HTTP Content-Types (`application/json`, `application/proto`) that work with standard CORS.

**Instead:** Rely on the existing CORS middleware. If Connect-specific headers need allowlisting (e.g., `Connect-Protocol-Version`, `Connect-Timeout-Ms`), add them to the existing CORS configuration's `AllowedHeaders`.

### Anti-Pattern 5: Generating Proto Files Manually Instead of From Ent Schemas

**What:** Writing `.proto` files by hand instead of generating them from ent schemas via entproto.

**Why bad:** Creates a maintenance burden -- every schema change requires updating both ent schemas and proto files. Field names, types, and relationships will drift. The ent schema is the single source of truth.

**Instead:** Use entproto to generate `.proto` files from ent schemas. Treat proto files as generated artifacts. If entproto's output needs adjustment, use proto file post-processing or accept entproto's conventions.

## Scalability Considerations

| Concern | Current (HTTP/1.1 only) | With ConnectRPC | Notes |
|---------|-------------------------|-----------------|-------|
| Protocol support | REST, GraphQL | REST, GraphQL, Connect, gRPC-Web | gRPC wire protocol blocked by LiteFS proxy |
| Serialization overhead | JSON only | JSON + Protobuf binary | Protobuf is 3-10x smaller for typed data |
| Client ecosystem | curl, browsers, GraphQL clients | + grpcurl, buf curl, connect-go clients, gRPC-Web clients | Significantly broader client support |
| Handler count | ~20 mux routes | ~40 mux routes (2 per service + reflection + health) | No performance concern -- mux lookup is O(1) per pattern |
| Build complexity | go generate (ent + gqlgen) | + entproto + buf generate | Two additional code generation steps |
| Binary size | ~30MB | +2-5MB (protobuf runtime) | Marginal increase |

## New Dependencies

| Package | Purpose | Size Impact |
|---------|---------|-------------|
| `connectrpc.com/connect` | ConnectRPC runtime | Small (~5k LOC) |
| `connectrpc.com/otelconnect` | OTel interceptor | Small |
| `connectrpc.com/grpcreflect` | gRPC server reflection | Small |
| `connectrpc.com/grpchealth` | gRPC health checking | Small |
| `google.golang.org/protobuf` | Already in go.mod (indirect) | No change |
| `entgo.io/contrib/entproto` | Already in go.mod (via entgql) | No change |

**Not needed:**
- `google.golang.org/grpc` -- NOT needed as a direct dependency. ConnectRPC does not depend on it.
- `connectrpc.com/validate` -- Premature for read-only service. Add later if needed.
- `connectrpc.com/authn` -- No auth on public read-only API.

## Fly.io Configuration

**No changes to fly.toml are required.** The existing configuration works:

```toml
[http_service]
  internal_port = 8080  # LiteFS proxy
  force_https = true
```

Do NOT add `h2_backend = true` -- this would tell Fly's edge proxy to use HTTP/2 to connect to LiteFS proxy, which does not support h2c and would break.

Do NOT add `alpn = ["h2"]` -- this is for gRPC-only services that bypass LiteFS.

The Connect protocol works over HTTP/1.1 through the existing infrastructure unchanged.

## Suggested Build Order

The build order is driven by code generation dependencies:

1. **Schema annotations** -- Add `entproto.Message()`, `entproto.Field(N)`, `entproto.Service()` to all 13 ent schemas. Add `entproto.NewExtension(entproto.SkipGenFile())` to `ent/entc.go`. This is the foundation.

2. **Proto generation pipeline** -- Create `buf.yaml`, `buf.gen.yaml`. Run `go generate ./ent/...` to produce `.proto` files. Run `buf generate` to produce Go types and ConnectRPC interfaces. Verify generated code compiles.

3. **First service implementation** -- Pick one simple entity (e.g., Organization -- fewest fields/edges). Write `internal/connectrpc/organization.go` with Get and List. Write conversion functions. Write tests using `enttest` + `connect.NewClient`.

4. **Handler mounting** -- Wire the first service into `main.go`. Add `otelconnect.NewInterceptor()`. Verify end-to-end with `buf curl` or `grpcurl` (via Connect protocol). Verify existing routes still work.

5. **Remaining services** -- Implement remaining 12 entity services following the pattern from step 3. Each service is independent and can be done in parallel.

6. **gRPC ecosystem** -- Add `grpcreflect` handlers (V1 + V1Alpha) and `grpchealth` handler. Test with `grpcurl --plaintext` to verify reflection works.

7. **CORS update** -- Add Connect-specific headers (`Connect-Protocol-Version`, `Connect-Timeout-Ms`, `Grpc-Timeout`) to the CORS middleware's `AllowedHeaders` list.

8. **Documentation** -- Update the root discovery endpoint (`GET /`) to include the ConnectRPC service paths. Update API docs.

**Rationale for this order:**
- Steps 1-2 are prerequisites (no code runs without generated types)
- Step 3 proves the pattern works before committing to all 13 entities
- Step 4 validates integration with existing middleware
- Steps 5-8 are additive and low-risk

## Sources

- [ConnectRPC Getting Started](https://connectrpc.com/docs/go/getting-started/) -- Handler mounting, h2c configuration
- [ConnectRPC Deployment & h2c](https://connectrpc.com/docs/go/deployment/) -- HTTP/2 configuration guidance
- [ConnectRPC Observability](https://connectrpc.com/docs/go/observability/) -- otelconnect interceptor usage
- [ConnectRPC gRPC Compatibility](https://connectrpc.com/docs/go/grpc-compatibility/) -- grpcreflect, grpchealth, protocol detection
- [ConnectRPC Protocol Reference](https://connectrpc.com/docs/protocol/) -- HTTP/1.1 support, wire format
- [ConnectRPC Multi-Protocol Support](https://connectrpc.com/docs/multi-protocol/) -- Connect vs gRPC vs gRPC-Web detection
- [otelconnect-go](https://github.com/connectrpc/otelconnect-go) -- OTel interceptor, cardinality options
- [otelconnect Issue #164](https://github.com/connectrpc/otelconnect-go/issues/164) -- otelhttp + otelconnect coexistence
- [grpcreflect-go](https://github.com/connectrpc/grpcreflect-go) -- Server reflection handler
- [grpchealth-go](https://github.com/connectrpc/grpchealth-go) -- Health check handler
- [entproto docs](https://entgo.io/docs/grpc-generating-proto/) -- Proto generation from ent schemas
- [entproto API](https://pkg.go.dev/entgo.io/contrib/entproto) -- SkipGenFile, Message, Service, Field annotations
- [Fly.io gRPC Services](https://fly.io/docs/app-guides/grpc-and-grpc-web-services/) -- h2_backend, TLS ALPN configuration
- [Fly.io LiteFS Proxy](https://fly.io/docs/litefs/proxy/) -- HTTP proxy limitations, write forwarding
- [Go 1.26 http.Protocols](https://pkg.go.dev/net/http#Protocols) -- h2c support in stdlib
- [buf.gen.yaml](https://buf.build/docs/configuration/v1/buf-gen-yaml/) -- Code generation configuration
- [protoc-gen-connect-go](https://pkg.go.dev/connectrpc.com/connect/cmd/protoc-gen-connect-go) -- ConnectRPC code generator

## Confidence Assessment

| Component | Confidence | Notes |
|-----------|------------|-------|
| ConnectRPC handler mounting on stdlib mux | HIGH | Documented pattern, handlers are http.Handler |
| LiteFS proxy HTTP/1.1 constraint | HIGH | Verified in LiteFS source -- uses http.Transport without h2c |
| Connect protocol over HTTP/1.1 | HIGH | Core design principle of ConnectRPC, documented extensively |
| otelconnect + otelhttp coexistence | HIGH | Different instrumentation layers, confirmed in docs and issues |
| entproto SkipGenFile + buf generate flow | MEDIUM | Each component documented independently; combined flow not documented as a pattern but mechanically sound |
| grpcreflect/grpchealth on same mux | HIGH | Documented, returns (path, handler) like all ConnectRPC handlers |
| No fly.toml changes needed | HIGH | Connect protocol is HTTP/1.1, no h2c needed |
| Hand-written service handlers over entgrpc | HIGH | entgrpc targets grpc.Server, not ConnectRPC -- verified in entproto docs |
