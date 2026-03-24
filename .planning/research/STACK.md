# Technology Stack: ConnectRPC / gRPC API Surface

**Project:** PeeringDB Plus - ConnectRPC/gRPC Milestone
**Researched:** 2026-03-24
**Scope:** New dependencies and changes required to add a ConnectRPC/gRPC API surface to the existing application.

## Context: What Already Exists

The project already has these relevant dependencies in `go.mod`:
- `entgo.io/contrib v0.7.1-0.20260306055004-3625dcc2e035` (includes entproto)
- `google.golang.org/grpc v1.79.3` (indirect, via OTel exporters)
- `google.golang.org/protobuf v1.36.11` (indirect, via OTel exporters)
- `go.opentelemetry.io/otel v1.42.0` (already direct)
- Go 1.26.1 with `net/http` server using `http.NewServeMux()`

The `ent/entc.go` currently registers `entgql` and `entrest` extensions. It does **not** use entproto yet.

## Recommended Stack Additions

### ConnectRPC Core

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| connectrpc.com/connect | v1.19.1 | RPC framework (handlers + clients) | Implements Connect, gRPC, and gRPC-Web protocols over standard `net/http`. Handlers are plain `http.Handler` -- they mount directly onto the existing `http.NewServeMux()` without requiring a separate gRPC server. Supports HTTP/1.1 (Connect + gRPC-Web) and HTTP/2 (all protocols). Apache-2.0 licensed. Stable v1 with semver guarantees. Published 2025-10-07. | HIGH |
| connectrpc.com/connect/cmd/protoc-gen-connect-go | v1.19.1 | Code generator for ConnectRPC handler interfaces | Generates handler interfaces and client constructors from .proto files. Each generated handler returns `(string, http.Handler)` for direct registration with `http.NewServeMux`. Same module as connect core -- version tracks together. Install as Go tool dependency. | HIGH |

### ConnectRPC Ecosystem

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| connectrpc.com/otelconnect | v0.9.0 | OpenTelemetry interceptor for ConnectRPC | Implements OTel RPC metrics spec and RPC spans spec. Provides `otelconnect.NewInterceptor()` that adds tracing and metrics to all ConnectRPC handlers. Requires OTel v1 SDK -- compatible with project's v1.42.0 (otelconnect's go.mod pins v1.40.0 minimum, Go module semver allows v1.42.0). Published 2026-01-05. | HIGH |
| connectrpc.com/grpcreflect | v1.3.0 | gRPC server reflection | Enables `grpcurl`, `grpcui`, and BloomRPC to introspect services without .proto files. Returns `(string, http.Handler)` for mux registration. `NewStaticReflector()` for static service list is simplest -- no runtime registration needed since our services are fixed. Published 2025-01-17. | HIGH |
| connectrpc.com/grpchealth | v1.4.0 | gRPC health checking protocol | Implements standard gRPC health check protocol. `NewStaticChecker()` with `SetStatus()` integrates with existing readiness logic. Returns `(string, http.Handler)` for mux registration. Required for gRPC-native load balancers and `grpc-health-probe`. Published 2025-04-07. | HIGH |

### Protobuf Toolchain

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| google.golang.org/protobuf | v1.36.11 | Protobuf Go runtime | Already in go.mod as indirect dependency. Will become direct dependency when generated .pb.go files are used. Standard protobuf runtime for Go. Published 2025-12-12. | HIGH |
| google.golang.org/protobuf/cmd/protoc-gen-go | v1.36.11 | Protobuf Go code generator | Generates Go message structs and serialization from .proto files. Install as Go tool dependency. Required by buf code generation. | HIGH |
| buf.build/buf/cli | v1.66.0+ | Protobuf toolchain | Replaces raw `protoc` for proto file management. Handles dependency resolution, linting, breaking change detection. Already recommended in project CLAUDE.md (TL-4). Install via system package manager or direct download -- not a Go module dependency. Published 2026-02-23. | HIGH |

### Ent Proto Generation

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| entgo.io/contrib/entproto | v0.7.1+ | Proto file generation from ent schemas | Already available in go.mod (`entgo.io/contrib`). Generates .proto message definitions from annotated ent schemas. Requires `entproto.Message()` and `entproto.Field(N)` annotations on each schema entity and field. The `entproto` CLI reads ent schemas and outputs .proto files. | HIGH |

### h2c Support (Stdlib)

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| net/http `Protocols` | Go 1.26.1 (stdlib) | Unencrypted HTTP/2 (h2c) for gRPC protocol | Go 1.24+ added `http.Server.Protocols` with `SetUnencryptedHTTP2(true)` for "HTTP/2 with Prior Knowledge" (RFC 9113 section 3.3). Enables gRPC protocol support on the same port as HTTP/1.1 without TLS. No external dependency -- replaces the deprecated `golang.org/x/net/http2/h2c` wrapper. | HIGH |

## Integration Architecture

### How ConnectRPC Fits the Existing Server

The current server uses `http.NewServeMux()` with handlers registered via `mux.Handle()` and `mux.HandleFunc()`. ConnectRPC handlers produce `(string, http.Handler)` tuples that register identically:

```go
// Current pattern (REST, GraphQL):
mux.Handle("/rest/v1/", restCORS(restSrv.Handler()))
mux.HandleFunc("/graphql", ...)

// ConnectRPC pattern (same mux, same middleware stack):
path, handler := peeringdbv1connect.NewNetworkServiceHandler(svc, opts...)
mux.Handle(path, handler)  // path = "/peeringdb.v1.NetworkService/"
```

All ConnectRPC handlers go through the existing middleware stack (Recovery, OTel HTTP, Logging, CORS, Readiness) automatically because they are standard `http.Handler` values registered on the same mux.

### Proto Generation Pipeline

The generation flow has two distinct stages:

**Stage 1: ent schema -> .proto files** (entproto)
```
ent/schema/*.go  --(entproto annotations)-->  proto/peeringdb/v1/*.proto
```

**Stage 2: .proto files -> Go code** (buf generate)
```
proto/peeringdb/v1/*.proto  --(protoc-gen-go)-->       gen/peeringdb/v1/*.pb.go
                             --(protoc-gen-connect-go)--> gen/peeringdb/v1/peeringdbv1connect/*.go
```

**Critical decision: Skip `protoc-gen-entgrpc`.** The standard entproto workflow generates gRPC service implementations via `protoc-gen-entgrpc` that target `google.golang.org/grpc`. We do NOT want those implementations because:

1. They generate standard gRPC server code, not ConnectRPC handlers.
2. They create CRUD service implementations that we may want to customize (read-only, different query patterns).
3. ConnectRPC handlers are generated by `protoc-gen-connect-go` instead.

Use entproto **only** for .proto file generation (message definitions + service definitions), then use buf with `protoc-gen-go` + `protoc-gen-connect-go` for the Go code generation step.

### h2c Configuration

```go
// Go 1.24+ stdlib h2c -- no golang.org/x/net/http2/h2c needed
server := &http.Server{
    Addr:    cfg.ListenAddr,
    Handler: handler,
}
server.Protocols.SetHTTP1(true)
server.Protocols.SetUnencryptedHTTP2(true)
```

This enables gRPC protocol (which requires HTTP/2) on the same port alongside HTTP/1.1 (used by Connect protocol and gRPC-Web). Behind Fly.io's proxy (which terminates TLS), the internal connection is cleartext, so h2c is the correct mode.

### OTel Integration

```go
otelInterceptor, err := otelconnect.NewInterceptor()
// Apply as connect.WithInterceptors(otelInterceptor) to all handlers
path, handler := peeringdbv1connect.NewNetworkServiceHandler(
    svc,
    connect.WithInterceptors(otelInterceptor),
)
```

The OTel interceptor adds RPC-specific spans and metrics (per OTel RPC semantic conventions) in addition to the existing `otelhttp.NewMiddleware()` HTTP-level instrumentation. The interceptor implements both the RPC metrics specification and RPC spans specification from OpenTelemetry semantic conventions.

### gRPC Health Integration with Existing Readiness

The project already has `/healthz` (liveness) and `/readyz` (readiness) HTTP endpoints. The gRPC health check protocol (`grpc.health.v1.Health`) is separate and needed for gRPC-native clients and load balancers:

```go
checker := grpchealth.NewStaticChecker(
    "peeringdb.v1.NetworkService",
    "peeringdb.v1.OrganizationService",
    // ... all registered services
)
mux.Handle(grpchealth.NewHandler(checker))

// Toggle based on sync readiness (reuse existing HasCompletedSync logic):
if syncWorker.HasCompletedSync() {
    checker.SetStatus("", grpchealth.StatusServing)
} else {
    checker.SetStatus("", grpchealth.StatusNotServing)
}
```

### gRPC Reflection for Debugging

```go
reflector := grpcreflect.NewStaticReflector(
    "peeringdb.v1.NetworkService",
    "peeringdb.v1.OrganizationService",
    // ... all registered services
)
mux.Handle(grpcreflect.NewHandlerV1(reflector))
mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))  // For older tools like grpcurl < 1.5
```

This enables introspection with `grpcurl -plaintext localhost:8081 list` and similar debugging tools.

## What NOT to Add

| Technology | Why Not |
|------------|---------|
| google.golang.org/grpc (as direct dependency) | ConnectRPC replaces the need for a separate gRPC server. The existing indirect dependency (via OTel exporters) is sufficient. Do not register a `grpc.NewServer()`. |
| golang.org/x/net/http2/h2c | Superseded by `http.Server.Protocols.SetUnencryptedHTTP2(true)` in Go 1.24+. The x/net/http2/h2c wrapper is no longer needed. |
| protoc-gen-go-grpc | Generates gRPC server/client stubs targeting google.golang.org/grpc. Not needed -- protoc-gen-connect-go generates ConnectRPC handlers instead. |
| protoc-gen-entgrpc | Generates gRPC service implementations using ent client. We write our own ConnectRPC service implementations to control query behavior (read-only, custom filtering, edge-loading). |
| connectrpc.com/validate | Request validation middleware for ConnectRPC. Premature -- the API is read-only and validation needs are minimal. Add later if write operations are introduced. |
| connectrpc.com/cors | CORS middleware for ConnectRPC. The project already has custom CORS middleware (`internal/middleware/cors.go`) that wraps the rs/cors library. That existing middleware works because ConnectRPC handlers are standard http.Handler. |
| connectrpc.com/vanguard | gRPC-to-REST transcoding. Not needed -- we already have a dedicated REST surface via entrest. |

## buf Configuration Files

### buf.yaml (proto module configuration)

```yaml
version: v2
modules:
  - path: proto
lint:
  use:
    - STANDARD
breaking:
  use:
    - FILE
```

### buf.gen.yaml (code generation)

```yaml
version: v2
plugins:
  - local: [go, tool, protoc-gen-go]
    out: gen
    opt: paths=source_relative
  - local: [go, tool, protoc-gen-connect-go]
    out: gen
    opt:
      - paths=source_relative
      - simple
```

The `simple` option (added in connect-go v1.19.0) generates simplified handler interfaces without `connect.Request`/`connect.Response` wrappers for unary RPCs, producing cleaner function signatures:

```go
// Without simple:
func (s *NetworkService) GetNetwork(ctx context.Context, req *connect.Request[v1.GetNetworkRequest]) (*connect.Response[v1.GetNetworkResponse], error)

// With simple:
func (s *NetworkService) GetNetwork(ctx context.Context, req *v1.GetNetworkRequest) (*v1.GetNetworkResponse, error)
```

## Entproto Schema Annotations Required

Each ent schema entity that should be exposed via gRPC needs two annotations, plus field-level annotations:

```go
func (Network) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entproto.Message(),   // generates protobuf message definition
        entproto.Service(),   // generates service definition in .proto
        // ... existing entgql, entrest annotations
    }
}

func (Network) Fields() []ent.Field {
    return []ent.Field{
        field.Int("id").
            Annotations(entproto.Field(1)),
        field.String("name").
            Annotations(entproto.Field(2)),
        // ... each field needs a unique proto field number
    }
}
```

All 16 ent schema entities (Organization, Network, Facility, InternetExchange, IXLan, IXPrefix, IXFacility, NetworkFacility, NetworkIXLan, POC, Carrier, CarrierFacility, Campus, plus junction types) will need these annotations.

## Go Tool Dependencies

Go 1.26 supports tool dependencies in `go.mod` via `go get -tool`. Install code generators this way:

```bash
# Protobuf code generators (Go tool deps)
go get -tool google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
go get -tool connectrpc.com/connect/cmd/protoc-gen-connect-go@v1.19.1

# Runtime dependencies (go get)
go get connectrpc.com/connect@v1.19.1
go get connectrpc.com/otelconnect@v0.9.0
go get connectrpc.com/grpcreflect@v1.3.0
go get connectrpc.com/grpchealth@v1.4.0

# buf CLI (system install, not Go module)
# Via Homebrew: brew install bufbuild/buf/buf
# Or direct download from https://github.com/bufbuild/buf/releases
```

## Version Compatibility Matrix

| Package | Version | Requires Go | Requires OTel | Requires connect |
|---------|---------|-------------|---------------|------------------|
| connect | v1.19.1 | 1.22+ | -- | -- |
| otelconnect | v0.9.0 | 1.24+ | v1.40.0+ | v1.17.0+ |
| grpcreflect | v1.3.0 | 1.21+ | -- | v1.x |
| grpchealth | v1.4.0 | 1.21+ | -- | v1.x |
| protobuf | v1.36.11 | 1.21+ | -- | -- |

All version requirements are satisfied by the project's Go 1.26.1 and OTel v1.42.0.

## Alternatives Considered

| Category | Recommended | Alternative | Why Not |
|----------|-------------|-------------|---------|
| RPC framework | ConnectRPC | google.golang.org/grpc | Standard gRPC requires a separate `grpc.Server` with its own listener -- cannot share the existing `http.NewServeMux`. ConnectRPC handlers are `http.Handler`, sharing the same mux, middleware stack, and port. Supports Connect + gRPC + gRPC-Web on one endpoint. |
| RPC framework | ConnectRPC | Twirp (github.com/twitchtv/twirp) | Twirp only supports unary RPCs, no streaming. Smaller ecosystem. ConnectRPC is the modern standard backed by Buf. |
| Proto generation | entproto + buf | Manual .proto files | entproto generates message definitions from ent schemas, ensuring proto definitions stay in sync with the data model. Manual .proto files would duplicate schema definitions and risk drift. |
| Proto generation | entproto (protos only) | entproto + protoc-gen-entgrpc (full stack) | protoc-gen-entgrpc generates standard gRPC service implementations, not ConnectRPC handlers. The generated CRUD implementations also don't match our read-only, custom-query-pattern requirements. Generate protos, write custom ConnectRPC handlers. |
| h2c | stdlib Protocols | golang.org/x/net/http2/h2c | The x/net h2c wrapper is the legacy approach. Go 1.24+ Protocols API is cleaner, stdlib-native, and better maintained. The h2c wrapper is effectively deprecated for new code. |
| Health checks | connectrpc.com/grpchealth | Custom /healthz endpoint only | The project already has /healthz and /readyz. grpchealth adds the **gRPC-native** health protocol (grpc.health.v1.Health) that gRPC-aware load balancers and tools like `grpc-health-probe` expect. Both should coexist. |
| OTel for RPC | otelconnect interceptor | otelhttp only | otelhttp provides HTTP-level instrumentation (request duration, status codes). otelconnect adds RPC-specific semantics (RPC method names, RPC status codes, RPC message sizes) per OTel RPC semantic conventions. Both layers complement each other. |

## Risk Register

| Risk | Severity | Likelihood | Mitigation |
|------|----------|------------|------------|
| entproto annotation effort on 16 schema entities | MEDIUM | HIGH | Each of 16 ent schemas needs `entproto.Message()`, `entproto.Service()`, and `entproto.Field(N)` on every field. This is mechanical but tedious. Budget adequate time and validate generated .proto files. |
| entproto field number assignment conflicts | MEDIUM | MEDIUM | Proto field numbers must be unique per message and should never be reused. Establish a convention (e.g., match PeeringDB API field order or ent schema order) and document in CONVENTIONS.md. |
| entproto does not support all ent field types | MEDIUM | MEDIUM | Complex types (enums, JSON fields, custom types in `ent/schema/types.go` like FlexDate/FlexInt) may not map cleanly to protobuf. Requires custom proto message design or manual .proto additions for those fields. |
| entproto edge/relation mapping | MEDIUM | MEDIUM | Ent edges (relations) need to map to proto message references or repeated fields. entproto handles basic edges but complex edge patterns may need manual proto definitions. |
| otelconnect pre-v1 (v0.9.0) | LOW | LOW | Pre-v1 but actively maintained and follows OTel stability conventions. The API surface is small (one interceptor constructor with options). Pin version. |
| ConnectRPC v1.19.1 last release Oct 2025 | LOW | LOW | Stable v1 with semver guarantee. Not abandoned -- project is mature, maintained by Buf. No breaking changes expected. |
| h2c "Prior Knowledge" requirement | LOW | MEDIUM | Go stdlib h2c only supports "HTTP/2 with Prior Knowledge" (RFC 9113 s3.3), not the deprecated "Upgrade: h2c" header. All gRPC clients use Prior Knowledge, so this is correct for gRPC. Non-issue for intended use case. |
| Fly.io internal proxy and h2c | LOW | LOW | Fly.io's internal proxy forwards to app containers. h2c works for internal connections since TLS is terminated at the edge. Verify during deployment that gRPC protocol works end-to-end through Fly.io proxy. |

## Sources

- [connectrpc.com/connect v1.19.1](https://pkg.go.dev/connectrpc.com/connect) - Package documentation, version history
- [ConnectRPC Getting Started](https://connectrpc.com/docs/go/getting-started/) - Official setup guide
- [connect-go releases](https://github.com/connectrpc/connect-go/releases) - v1.19.1 release notes
- [connectrpc.com/otelconnect v0.9.0](https://pkg.go.dev/connectrpc.com/otelconnect) - OTel interceptor documentation
- [otelconnect-go go.mod](https://github.com/connectrpc/otelconnect-go/blob/main/go.mod) - Dependency verification (OTel v1.40.0+, connect v1.17.0+)
- [ConnectRPC Observability](https://connectrpc.com/docs/go/observability/) - OTel integration guide
- [connectrpc.com/grpcreflect v1.3.0](https://pkg.go.dev/connectrpc.com/grpcreflect) - Server reflection documentation
- [connectrpc.com/grpchealth v1.4.0](https://pkg.go.dev/connectrpc.com/grpchealth) - Health checking documentation
- [google.golang.org/protobuf v1.36.11](https://pkg.go.dev/google.golang.org/protobuf) - Protobuf runtime versions
- [entproto documentation](https://entgo.io/docs/grpc-generating-proto/) - Proto generation from ent schemas
- [ent/contrib entproto](https://pkg.go.dev/entgo.io/contrib/entproto) - Package documentation
- [buf CLI releases](https://github.com/bufbuild/buf/releases) - v1.66.0+
- [buf CLI installation](https://buf.build/docs/cli/installation/) - Installation methods
- [Go 1.24 Release Notes](https://go.dev/doc/go1.24) - http.Protocols / h2c support introduction
- [Go 1.26 Release Notes](https://tip.golang.org/doc/go1.26) - Confirmed no additional h2c changes
- [net/http Protocols API](https://pkg.go.dev/net/http@go1.26.1) - SetHTTP1, SetHTTP2, SetUnencryptedHTTP2 methods
- [ConnectRPC gRPC Conformance](https://buf.build/blog/grpc-conformance-deep-dive) - gRPC vs ConnectRPC comparison
