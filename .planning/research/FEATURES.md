# Feature Landscape: ConnectRPC / gRPC API Surface

**Domain:** gRPC/ConnectRPC API layer for a read-only PeeringDB data mirror
**Researched:** 2026-03-24
**Milestone context:** Adding gRPC API surface to existing PeeringDB Plus app (already has GraphQL, REST, PeeringDB-compat REST, and web UI)

## Table Stakes

Features that gRPC API consumers expect. Missing = API feels incomplete or unusable for production integrations.

### Get by ID (per entity type)

| Feature | Why Expected | Complexity | Dependencies | Notes |
|---------|--------------|------------|--------------|-------|
| `Get{Type}(id)` for all 13 types | The fundamental read operation. Every gRPC data service must support retrieving a single entity by its primary key. Google AIP-131 defines this as a standard method. entproto generates this via `MethodGet`. | Low | entproto `MethodGet` flag on each schema. Requires `entproto.Message()` and `entproto.Service()` annotations on all 13 ent schemas. | entproto generates the full implementation including ent-to-proto conversion. The `id` field must have an `entproto.Field()` annotation with field number 1. Returns `NOT_FOUND` (gRPC code 5) if entity does not exist. |

### List with Pagination (per entity type)

| Feature | Why Expected | Complexity | Dependencies | Notes |
|---------|--------------|------------|--------------|-------|
| `List{Type}s(page_size, page_token)` for all 13 types | Retrieving collections is the second fundamental operation. Google AIP-132/AIP-158 define the standard: `page_size` (int32) + `page_token` (string) request fields, `next_page_token` response field. entproto generates this with keyset pagination. | Med | entproto `MethodList` flag. Keyset pagination requires int/uuid/string ID fields (all 13 types use int IDs -- compatible). | entproto's keyset pagination is descending-order only (newest first by ID). The `page_token` is a base64-encoded cursor. Max page size should be enforced server-side (default 100, max 1000 matches PeeringDB's own limits). Empty `next_page_token` signals end of collection. |

### Proto Message Definitions for All 13 Types

| Feature | Why Expected | Complexity | Dependencies | Notes |
|---------|--------------|------------|--------------|-------|
| Protobuf message types for: Organization, Network, Facility, Campus, Carrier, InternetExchange, IxFacility, IxLan, IxPrefix, NetworkFacility, NetworkIxLan, Poc, CarrierFacility | gRPC APIs are typed. Every entity needs a proto message definition with all fields mapped to appropriate proto types. | Med | entproto `entproto.Message()` annotation on all schemas. Every field needs `entproto.Field(N)` with unique field numbers. | **Critical blocker**: `field.JSON` types (e.g., `social_media []SocialMedia`, `info_types []string`) are NOT supported by entproto (GitHub issue #2929, confirmed by maintainer). These fields must be handled with workarounds: either (a) skip them from proto generation and handle manually, (b) use `google.protobuf.Struct` for arbitrary JSON, or (c) define separate proto message types for the nested structs. Option (c) is best for `SocialMedia`; option (b) or `repeated string` for `info_types`. |

### Error Handling with Standard gRPC/Connect Codes

| Feature | Why Expected | Complexity | Dependencies | Notes |
|---------|--------------|------------|--------------|-------|
| Proper gRPC status codes on errors | Clients expect standard error codes for programmatic handling. ConnectRPC maps these to HTTP status codes automatically. | Low | ConnectRPC or google.golang.org/grpc/status package | Key mappings for this read-only service: `NOT_FOUND` (5/404) when entity ID does not exist, `INVALID_ARGUMENT` (3/400) for bad page_size/page_token/filter values, `INTERNAL` (13/500) for database errors, `UNAVAILABLE` (14/503) if database is not ready (pre-first-sync). Error messages should include the entity type and ID for debuggability. |

### Server Reflection

| Feature | Why Expected | Complexity | Dependencies | Notes |
|---------|--------------|------------|--------------|-------|
| gRPC server reflection enabled | Enables grpcurl, grpcui, Postman, and other tooling to discover services without .proto files. Standard for any gRPC service that is not purely internal. Google and most cloud providers enable it on all services. | Low | `google.golang.org/grpc/reflection` for standard gRPC, `connectrpc.com/grpcreflect` for ConnectRPC (supports both V1 and V1Alpha) | Two lines of code to enable. No security concern for a public read-only API. Enables `grpcurl list`, `grpcurl describe`, and grpcui web interface for exploration. ConnectRPC's grpcreflect supports both reflection protocol versions for maximum tool compatibility. |

### Health Check Service

| Feature | Why Expected | Complexity | Dependencies | Notes |
|---------|--------------|------------|--------------|-------|
| `grpc.health.v1.Health/Check` and `/Watch` | Standard gRPC health checking protocol. Required for load balancers, Kubernetes probes, and monitoring tools. The existing HTTP health endpoint at `/healthz` does not serve gRPC clients. | Low | `google.golang.org/grpc/health` for standard gRPC, `connectrpc.com/grpchealth` for ConnectRPC | Map existing health check logic (database reachable, sync has completed at least once) to gRPC health status. SERVING = healthy, NOT_SERVING = database unreachable or no sync completed. Service-specific health checks can report per-service status. |

### OpenTelemetry Instrumentation

| Feature | Why Expected | Complexity | Dependencies | Notes |
|---------|--------------|------------|--------------|-------|
| Automatic tracing and metrics on all RPCs | Project constraint: OTel is mandatory. Every API surface must have consistent observability. The existing GraphQL and REST surfaces both have otelhttp instrumentation. | Low | For standard gRPC: `go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc` (stats handler, not interceptors -- interceptors are deprecated). For ConnectRPC: `connectrpc.com/otelconnect` (interceptor). | otelgrpc stats handler creates spans per RPC with method name, status code, and duration. otelconnect interceptor does the same and additionally tags `rpc.system` (grpc/grpc-web/connect). Both propagate trace context via gRPC metadata. Existing OTel provider setup (provider.go) provides TracerProvider and MeterProvider. |

### Protobuf/buf Toolchain Integration

| Feature | Why Expected | Complexity | Dependencies | Notes |
|---------|--------------|------------|--------------|-------|
| buf.yaml, buf.gen.yaml for proto management | buf is the standard protobuf toolchain (per project CLAUDE.md TL-4). Provides linting, breaking change detection, and code generation in one tool. | Low | `buf.build/buf/cli` (already in STACK.md) | buf lint enforces style guide on proto files. buf breaking detects backwards-incompatible changes between commits. buf generate replaces raw `protoc` invocations. Place proto files in `proto/` directory with `buf.yaml` at root. |

### Read-Only Service (No Mutations)

| Feature | Why Expected | Complexity | Dependencies | Notes |
|---------|--------------|------------|--------------|-------|
| Only Get + List methods, no Create/Update/Delete | This is a read-only mirror. Exposing mutation RPCs would be confusing at best and dangerous at worst (they would fail against LiteFS replicas). entproto allows selecting which methods to generate. | Low | `entproto.Methods(entproto.MethodGet \| entproto.MethodList)` on each schema's `entproto.Service()` annotation. | Do NOT use `entproto.MethodAll`. Explicitly select only `MethodGet` and `MethodList`. This prevents accidental mutation endpoints and makes the API contract clear. |

## Differentiators

Features that set this gRPC API apart from a minimal implementation. Not expected by all clients, but valued by power users and automation tooling.

### Filtering on List RPCs

| Feature | Value Proposition | Complexity | Dependencies | Notes |
|---------|-------------------|------------|--------------|-------|
| Filter by common fields (ASN, country, name, org_id, status) | The existing REST and GraphQL APIs support filtering. A gRPC API without filtering forces clients to fetch all records and filter client-side, which is wasteful for a 85K+ record dataset. Google AIP-160 defines a `string filter` field with expression syntax. | High | **Not generated by entproto.** The entproto roadmap (GitHub #2446) lists "Query Predicates" as unimplemented. Must be implemented manually: parse filter string, translate to ent predicates, apply to query. | Two approaches: (a) AIP-160 filter string (`filter: "asn > 13000 AND country = 'US'"`) -- powerful but complex parser needed. (b) Dedicated filter fields on the request message (`int32 asn`, `string country`, etc.) -- simpler, mirrors PeeringDB's own query params. Recommend option (b) for initial implementation because it matches the existing REST API filter patterns and avoids building an expression parser. Add typed filter fields to ListRequest messages manually (not generated by entproto). |

### Edge/Relationship Traversal

| Feature | Value Proposition | Complexity | Dependencies | Notes |
|---------|-------------------|------------|--------------|-------|
| Nested messages for related entities (e.g., Organization with Networks) | The GraphQL API's killer feature is relationship traversal. A gRPC API that only returns flat entities with foreign key IDs forces clients to make N+1 calls. entproto can include edge fields in proto messages. | Med | entproto edge annotations. Edges must have `entproto.Field(N)` annotations. Cyclic dependencies are NOT supported in protobuf -- back-references require same proto package. | entproto can generate nested messages for edges. For this project, the key traversals are: Organization -> Networks/Facilities/IXs, Network -> NetworkFacilities/NetworkIxLans/Pocs, InternetExchange -> IxFacilities/IxLans. Cyclic deps (Network -> Org -> Network) must be in the same proto package, which is fine for a single-service API. Eager-loading edges avoids N+1 at the database level (ent handles this). |

### ConnectRPC Protocol Support (Triple-Protocol)

| Feature | Value Proposition | Complexity | Dependencies | Notes |
|---------|-------------------|------------|--------------|-------|
| Serve gRPC, gRPC-Web, and Connect protocols simultaneously | ConnectRPC handlers serve all three protocols on the same port without proxies. gRPC clients use standard grpc-go, browser clients use gRPC-Web or Connect (no Envoy proxy needed), and developers debug with curl using the Connect protocol's JSON format. | Med | `connectrpc.com/connect`, `protoc-gen-connect-go`. Requires generating Connect handlers instead of (or alongside) standard gRPC handlers. **Key tension**: entproto generates standard gRPC service implementations (protoc-gen-go-grpc), not ConnectRPC handlers (protoc-gen-connect-go). | Two paths: (a) Use entproto-generated standard gRPC service, serve with grpc-go on a separate port. Simple but loses ConnectRPC benefits (curl-ability, shared mux, browser compat). (b) Generate proto files with entproto, then generate ConnectRPC handlers with protoc-gen-connect-go, and write thin adapter implementations that call ent client methods. More work upfront but integrates cleanly with existing net/http server. **Recommend (b)** -- the Connect handler pattern (`func(ctx, *Request) (*Response, error)`) is simpler than gRPC's and shares the HTTP mux with GraphQL/REST. |

### Server Streaming for Bulk Export

| Feature | Value Proposition | Complexity | Dependencies | Notes |
|---------|-------------------|------------|--------------|-------|
| `StreamList{Type}s` server-streaming RPC for full dataset export | For automation clients that need the entire dataset (e.g., building local caches, feeding into IRR tools), a server-streaming RPC sends all records without pagination overhead. Single TCP connection, backpressure via flow control, no token management. | Med | Server-streaming RPC definition in proto. ConnectRPC supports streaming. Standard gRPC supports streaming. | Useful for bulk consumers who would otherwise paginate through 85K+ records. Implementation streams ent query results row-by-row, converting each to proto message and sending. Natural fit for go channels. **Defer to post-MVP** -- pagination covers the use case adequately, and streaming adds complexity to error handling (partial failures). |

### Field Masks for Partial Responses

| Feature | Value Proposition | Complexity | Dependencies | Notes |
|---------|-------------------|------------|--------------|-------|
| `google.protobuf.FieldMask read_mask` on Get/List requests | Clients request only the fields they need, reducing response size and (potentially) database load. Netflix, Google, and most large gRPC API providers support this. Per AIP-157. | High | `google.protobuf.FieldMask` well-known type. Must implement field filtering in the service layer. | For a read-only mirror where most queries hit local SQLite, the performance benefit is marginal. The main benefit is bandwidth reduction for mobile/constrained clients. **Defer** -- adds significant implementation complexity (must filter proto message fields before serialization) for minimal gain in this use case. Ent queries already select all columns; partial selection would require custom query building. |

### Sync Status RPC

| Feature | Value Proposition | Complexity | Dependencies | Notes |
|---------|-------------------|------------|--------------|-------|
| `GetSyncStatus()` returning last sync time, data freshness, record counts | The GraphQL API already exposes `syncStatus`. gRPC clients need equivalent visibility into data freshness. Critical for automation that needs to know if data is stale. | Low | No entproto generation needed -- this is a custom RPC on a custom service definition. Read existing sync metrics/health state. | Define a `SyncService` with `GetSyncStatus` RPC. Return: `last_sync_at` (timestamp), `data_freshness_seconds` (float), `status` (enum: SYNCING/HEALTHY/STALE/ERROR), `type_counts` (map of type name to record count). Mirrors the GraphQL `syncStatus` query. |

### buf-Powered Breaking Change Detection in CI

| Feature | Value Proposition | Complexity | Dependencies | Notes |
|---------|-------------------|------------|--------------|-------|
| `buf breaking` in CI pipeline | Prevents accidental backwards-incompatible proto changes (renamed fields, changed field numbers, removed RPCs). Critical for any API with external consumers. | Low | buf CLI, `buf.yaml` config with `WIRE_JSON` or `WIRE` breaking rules | Add `buf breaking --against .git#branch=main` to CI. Catches: field number reuse, field type changes, RPC removal, service removal. Essential because proto field numbers are forever -- reusing one silently corrupts data on the wire. |

## Anti-Features

Features to explicitly NOT build for this milestone.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| Mutation RPCs (Create, Update, Delete) | This is a read-only mirror. Data comes from PeeringDB sync only. Mutation endpoints would either fail (on replicas) or corrupt data (on primary). Confuses API contract. | Generate only `MethodGet` and `MethodList`. Document read-only nature in proto comments and API docs. |
| Bidirectional streaming | No use case for client-to-server streaming on a read-only data query service. Adds complexity without value. The data does not change in real-time (hourly sync). | Use unary Get/List for queries. If real-time notifications are ever needed, server streaming is sufficient. |
| AIP-160 filter expression parser | Full AIP-160 filter syntax (`"field > value AND other_field = 'x'"`) is powerful but requires building a parser, type checker, and ent predicate translator. Massive effort for marginal gain over typed filter fields. | Use typed filter fields on List request messages (`int32 asn_filter`, `string country_filter`, etc.). Matches PeeringDB's own REST query parameter style. Simpler to implement, simpler to use, strongly typed. |
| Custom proto options/extensions | Proto custom options (e.g., field validation rules, custom annotations) add complexity to the proto toolchain and are rarely used by clients. | Use server-side validation. Return `INVALID_ARGUMENT` with descriptive messages. |
| gRPC-Web proxy (Envoy) | ConnectRPC eliminates the need for an Envoy proxy to serve gRPC-Web. Do not add infrastructure complexity. | If using ConnectRPC: native gRPC-Web support built in. If using standard gRPC: browser clients should use the existing REST or GraphQL APIs instead. |
| Proto-to-OpenAPI generation (grpc-gateway) | The project already has a full OpenAPI REST API via entrest. Generating another REST surface from proto definitions creates confusion and maintenance burden. | Direct gRPC/Connect API consumers to the gRPC endpoint. REST consumers to the existing REST API. No translation layer. |
| Automatic client SDK generation/publishing | Generating and publishing client libraries in multiple languages (Go, Python, TypeScript) is a maintenance commitment. Premature before the API stabilizes. | Publish `.proto` files. Consumers generate their own clients with `buf generate` or `protoc`. Provide a `buf.gen.yaml` example in docs. |

## Feature Dependencies

```
Existing ent schemas (13 types, all fields defined)
    |
    v
Add entproto.Message() + entproto.Field(N) annotations to all schemas
    |
    +--[BLOCKER] Handle field.JSON types (social_media, info_types)
    |     |
    |     +--> Option A: Skip from entproto, add manually to .proto files
    |     +--> Option B: Define SocialMedia as separate proto message
    |     +--> Option C: Use google.protobuf.Struct (loses type safety)
    |
    v
Add entproto.Service(entproto.Methods(MethodGet | MethodList)) annotations
    |
    v
Run entproto code generation --> .proto files + gRPC service stubs
    |
    +--> buf.yaml + buf.gen.yaml configuration
    +--> buf lint validation
    |
    v
[DECISION POINT] Standard gRPC vs ConnectRPC handlers
    |
    +--> Path A: Standard gRPC (entproto generates everything)
    |     |
    |     +--> Separate gRPC port (e.g., :8082)
    |     +--> otelgrpc stats handler for OTel
    |     +--> grpc reflection registration
    |     +--> grpc health service
    |
    +--> Path B: ConnectRPC (generate protos with entproto, handlers with connect-go)
          |
          +--> Shared HTTP mux with REST/GraphQL (port :8081)
          +--> otelconnect interceptor for OTel
          +--> grpcreflect for reflection
          +--> grpchealth for health checks
          +--> curl-able with JSON (Connect protocol)
    |
    v
Custom service: SyncStatus RPC (not generated, hand-written)
    |
    v
[OPTIONAL] Add typed filter fields to ListRequest messages
    |
    v
[OPTIONAL] Server streaming for bulk export
    |
    v
Integration tests (using generated client stubs)
    |
    v
buf breaking in CI
```

## entproto JSON Field Workaround Strategy

This is the most significant technical blocker. All 6 primary entity types (Organization, Network, Facility, InternetExchange, Carrier, Campus) have `social_media` as `field.JSON([]SocialMedia{})`. Network also has `info_types` as `field.JSON([]string{})`.

**Recommended approach: Hybrid generation**

1. Let entproto generate proto messages for all scalar fields (it handles string, int, bool, float, time.Time natively).
2. For `social_media`: Define `SocialMedia` as a separate proto message type (`string service = 1; string identifier = 2;`) and add `repeated SocialMedia social_media = N;` to entity messages manually.
3. For `info_types`: Add `repeated string info_types = N;` to the Network message manually.
4. For the conversion layer: Write custom `toProto{Type}()` functions that handle the JSON fields alongside entproto's generated conversion code.

This means proto files will be partially generated (entproto) and partially hand-maintained. Use `// DO NOT EDIT` markers on generated sections and clear comments on manual sections. Alternatively, fully hand-write proto files using entproto output as a starting point, then maintain them manually going forward.

## Standard vs ConnectRPC Decision Matrix

| Criterion | Standard gRPC (grpc-go) | ConnectRPC (connect-go) |
|-----------|------------------------|------------------------|
| entproto compatibility | Direct -- entproto generates service implementations | Indirect -- entproto generates .proto files, need separate handler impl |
| HTTP mux sharing | No -- grpc-go uses its own HTTP/2 implementation | Yes -- standard net/http, shares mux with REST/GraphQL |
| Browser compatibility | No (needs Envoy proxy for gRPC-Web) | Yes (native gRPC-Web + Connect protocol) |
| curl debugging | No (binary protocol) | Yes (Connect protocol uses JSON over HTTP) |
| Performance | Higher (~20K rps) | Slightly lower (~16K rps) |
| OTel integration | otelgrpc stats handler | otelconnect interceptor |
| Implementation effort | Lower (entproto generates service impl) | Higher (must write service implementations manually) |
| Operational simplicity | Extra port to manage | Same port as everything else |
| gRPC client compatibility | Full | Full (serves gRPC protocol by default) |

**Recommendation: ConnectRPC** because:
1. This is a read-only data API -- the ~20% performance difference is irrelevant for SQLite reads.
2. Sharing the HTTP mux eliminates port management complexity on Fly.io.
3. curl-ability with JSON dramatically improves developer experience.
4. Browser compatibility opens future possibilities without infrastructure changes.
5. The service implementations are thin (Get calls `client.{Type}.Get()`, List calls `client.{Type}.Query()`) -- writing them manually is not a large burden for 13 types.

## MVP Recommendation

Prioritize:
1. **Proto message definitions + buf toolchain** -- Define all 13 entity messages with field mappings. Set up buf.yaml, buf.gen.yaml. Validate with buf lint. This is the foundation everything else depends on.
2. **Get + List for all 13 types with ConnectRPC** -- Core read operations with keyset pagination. Register handlers on existing HTTP mux. Add otelconnect interceptor.
3. **Server reflection + health check** -- Two lines each. Enables tooling (grpcurl/grpcui) and monitoring.
4. **SyncStatus custom RPC** -- Small custom service, high value for automation clients.
5. **buf breaking in CI** -- Protect proto contract from accidental breakage.

Defer:
- **Filtering on List RPCs**: Adds significant complexity. Clients can paginate + filter client-side initially. Add in a follow-up phase based on demand.
- **Server streaming bulk export**: Pagination covers the use case. Streaming is a nice-to-have for automation clients.
- **Field masks**: Marginal benefit for local SQLite reads. Defer indefinitely.
- **Edge/relationship traversal in proto messages**: Start with flat entities (foreign key IDs only). Add nested messages after the basic API works.

## Sources

- [AIP-131: Standard methods: Get](https://google.aip.dev/131) -- Get method pattern
- [AIP-132: Standard methods: List](https://google.aip.dev/132) -- List method pattern with pagination, filtering, ordering
- [AIP-157: Partial responses](https://google.aip.dev/157) -- Field masks for read operations
- [AIP-158: Pagination](https://google.aip.dev/158) -- page_size, page_token, next_page_token specification
- [AIP-160: Filtering](https://google.aip.dev/160) -- Filter expression syntax specification
- [ConnectRPC Errors](https://connectrpc.com/docs/go/errors/) -- Error codes, HTTP status mapping, error details
- [ConnectRPC gRPC Compatibility](https://connectrpc.com/docs/go/grpc-compatibility/) -- Triple-protocol support, reflection, health
- [ConnectRPC Observability](https://connectrpc.com/docs/go/observability/) -- otelconnect interceptor
- [ConnectRPC Getting Started](https://connectrpc.com/docs/go/getting-started/) -- Handler registration on http.ServeMux
- [entproto Service Generation](https://entgo.io/docs/grpc-service-generation-options/) -- Method flags, available methods
- [entproto gRPC Extension Roadmap](https://github.com/ent/ent/issues/2446) -- Missing features: query predicates, JSON support
- [entproto JSON Field Issue](https://github.com/ent/ent/issues/2929) -- Confirmed: field.JSON not supported
- [gRPC Health Checking Protocol](https://github.com/grpc/grpc/blob/master/doc/health-checking.md) -- grpc.health.v1 spec
- [gRPC Server Reflection](https://grpc.io/docs/guides/reflection/) -- Reflection protocol for tooling
- [otelgrpc Package](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc) -- Stats handler instrumentation
- [otelconnect Package](https://pkg.go.dev/connectrpc.com/otelconnect) -- ConnectRPC OTel interceptor
- [buf Breaking Change Detection](https://buf.build/docs/breaking/) -- CI integration for proto contract safety
- [buf Style Guide](https://buf.build/docs/best-practices/style-guide/) -- Proto file organization best practices
- [gRPC Streaming Best Practices](https://dev.to/ramonberrutti/grpc-streaming-best-practices-and-performance-insights-219g) -- When to use streaming
- [Netflix FieldMask Blog](https://netflixtechblog.com/practical-api-design-at-netflix-part-1-using-protobuf-fieldmask-35cfdc606518) -- Practical FieldMask usage patterns
