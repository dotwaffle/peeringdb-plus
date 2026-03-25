# Phase 23: ConnectRPC Services - Research

**Researched:** 2026-03-25
**Domain:** ConnectRPC service implementation, ent-to-proto conversion, gRPC ecosystem (reflection, health, observability)
**Confidence:** HIGH

## Summary

Phase 23 implements the hand-written ConnectRPC service handlers that bridge the ent ORM layer to the generated protobuf types. Phase 22 produced all the generated code: 13 handler interfaces in `gen/peeringdb/v1/peeringdbv1connect/services.connect.go` (each with `Get` and `List` methods using the "simple" handler signature), 13 protobuf message types in `gen/peeringdb/v1/v1.pb.go`, and request/response types in `gen/peeringdb/v1/services.pb.go`. The handler interfaces accept plain `(context.Context, *Request) (*Response, error)` signatures (ConnectRPC's "simple" pattern), which simplifies implementation.

The core technical challenge is the ent-to-proto field conversion layer. Ent uses Go-native types (`int`, `*int`, `string`, `*string`, `time.Time`, `*time.Time`, `[]string`) while the generated proto types use protobuf wrappers (`wrapperspb.Int64Value`, `wrapperspb.StringValue`, `timestamppb.Timestamp`). There are approximately 366 wrapper type usages across the 13 message types. A set of generic or helper conversion functions will eliminate repetitive boilerplate. The pagination system uses stateless offset-based cursors encoded as opaque base64 tokens.

Three ConnectRPC ecosystem packages provide reflection, health checks, and observability: `connectrpc.com/grpcreflect` (v1.3.0), `connectrpc.com/grpchealth` (v1.4.0), and `connectrpc.com/otelconnect` (v0.9.0). All are stable, well-documented, and return `(string, http.Handler)` pairs that mount directly on the existing `http.ServeMux`. The CORS middleware needs updating to include Connect/gRPC protocol headers -- `connectrpc.com/cors` provides ready-made helper functions for this.

**Primary recommendation:** Build a shared conversion package with typed helper functions (`stringVal`, `int64Val`, `timestampVal`, etc.) to eliminate repetitive ent-to-proto mapping code across 13 service files. Implement one service (e.g., Network) as the template, then replicate the pattern for the remaining 12.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- Get RPCs return `NOT_FOUND` gRPC status with "entity {type} {id} not found" message for non-existent IDs
- Default page size for List RPCs is 100 (matches PeeringDB's default)
- Maximum page size cap is 1000 (prevents accidental full-table dumps)
- Page tokens use opaque base64-encoded cursors (offset-based internally), stateless
- Use default ConnectRPC path prefix (`/peeringdb.v1.XxxService/`) -- standard convention, no custom prefix
- Register all 13 services via loop over a slice of `(path, handler)` pairs in main.go
- Single `grpcserver` package with one file per service type, each implementing the generated handler interface
- CORS content types to add: `application/grpc`, `application/grpc-web`, `application/connect+proto`, `application/connect+json`
- Health check reports SERVING when DB is readable (sync complete at least once) -- reuse existing readiness logic
- Reflection via `connectrpc.com/grpcreflect` -- native ConnectRPC reflection, works with grpcurl and grpcui

### Claude's Discretion
- Internal implementation details of ent-to-proto field conversion in service handlers
- Exact structure of helper functions for pagination cursor encoding/decoding
- Test organization and fixture patterns

### Deferred Ideas (OUT OF SCOPE)
- Streaming RPCs for large result sets -- deferred pending demand signal
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| API-01 | Get RPC returns a single entity by ID for all 13 PeeringDB types | Ent `client.XxxClient.Get(ctx, id)` returns entity or `*NotFoundError`; map to `connect.CodeNotFound` via `ent.IsNotFound(err)` |
| API-02 | List RPC returns paginated results for all 13 PeeringDB types | Ent `client.XxxClient.Query().Limit(n).Offset(off).All(ctx)` with base64 offset cursor encoding; default 100, max 1000 |
| API-04 | Service handlers mounted on existing HTTP mux at ConnectRPC path prefix | `NewXxxServiceHandler(svc, opts...)` returns `(string, http.Handler)`; mount via `mux.Handle()` in loop |
| OBS-01 | otelconnect interceptor on all ConnectRPC handlers with WithoutServerPeerAttributes | `otelconnect.NewInterceptor(otelconnect.WithoutServerPeerAttributes())` returns `(*Interceptor, error)`; pass via `connect.WithInterceptors()` as `HandlerOption` |
| OBS-02 | CORS headers updated for Connect protocol and gRPC-Web content types | Use `connectrpc.com/cors` helpers (`AllowedHeaders()`, `AllowedMethods()`, `ExposedHeaders()`) merged with existing rs/cors config |
| OBS-03 | gRPC server reflection (v1 and v1alpha) enabled for grpcurl/grpcui discovery | `grpcreflect.NewStaticReflector(serviceNames...)` + `NewHandlerV1` + `NewHandlerV1Alpha`; mount both on mux |
| OBS-04 | gRPC health check service reports serving status for PeeringDB service | `grpchealth.NewStaticChecker(serviceNames...)` + `NewHandler(checker)`; call `checker.SetStatus()` to reflect sync readiness |
</phase_requirements>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| connectrpc.com/connect | v1.19.1 | ConnectRPC runtime (already in go.mod) | Required by generated handler code; already a project dependency |
| connectrpc.com/otelconnect | v0.9.0 | OpenTelemetry interceptor for ConnectRPC | Official ConnectRPC OTel integration; produces rpc.system, rpc.service, rpc.method attributes per OTel RPC conventions |
| connectrpc.com/grpcreflect | v1.3.0 | gRPC server reflection API | Official ConnectRPC reflection; wire-compatible with grpcurl/grpcui; stable v1.x semver |
| connectrpc.com/grpchealth | v1.4.0 | gRPC health check service | Official ConnectRPC health checks; wire-compatible with grpc-health-probe and Kubernetes gRPC probes; stable v1.x semver |
| connectrpc.com/cors | latest | CORS header helpers for Connect protocols | Official helpers for correct CORS configuration; integrates with rs/cors already in project |
| google.golang.org/protobuf | v1.36.11 | Protobuf runtime (already in go.mod) | Required for wrapperspb and timestamppb wrappers in field conversion |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| entgo.io/ent | v0.14.6 | ORM client (already in go.mod) | Query entities in service handlers |
| encoding/base64 (stdlib) | Go 1.26 | Page token encoding/decoding | Cursor serialization for pagination |
| strconv (stdlib) | Go 1.26 | Integer-to-string conversion | Offset encoding within cursor tokens |

**Installation:**
```bash
go get connectrpc.com/otelconnect@v0.9.0
go get connectrpc.com/grpcreflect@v1.3.0
go get connectrpc.com/grpchealth@v1.4.0
go get connectrpc.com/cors
```

## Architecture Patterns

### Recommended Project Structure
```
internal/
  grpcserver/
    campus.go              # CampusServiceHandler implementation
    carrier.go             # CarrierServiceHandler implementation
    carrierfacility.go     # CarrierFacilityServiceHandler implementation
    facility.go            # FacilityServiceHandler implementation
    internetexchange.go    # InternetExchangeServiceHandler implementation
    ixfacility.go          # IxFacilityServiceHandler implementation
    ixlan.go               # IxLanServiceHandler implementation
    ixprefix.go            # IxPrefixServiceHandler implementation
    network.go             # NetworkServiceHandler implementation
    networkfacility.go     # NetworkFacilityServiceHandler implementation
    networkixlan.go        # NetworkIxLanServiceHandler implementation
    organization.go        # OrganizationServiceHandler implementation
    poc.go                 # PocServiceHandler implementation
    convert.go             # Shared ent-to-proto conversion helpers
    pagination.go          # Cursor encoding/decoding, page size validation
    grpcserver_test.go     # Tests for all service handlers
    pagination_test.go     # Tests for cursor logic
  middleware/
    cors.go                # Updated with Connect protocol headers
```

### Pattern 1: Service Handler with Ent Client
**What:** Each service handler struct holds an `*ent.Client` and implements the generated handler interface.
**When to use:** All 13 service handlers follow this pattern.
**Example:**
```go
// Source: Derived from generated peeringdbv1connect interfaces
package grpcserver

import (
    "context"
    "fmt"

    "connectrpc.com/connect"
    "github.com/dotwaffle/peeringdb-plus/ent"
    pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// NetworkService implements peeringdbv1connect.NetworkServiceHandler.
type NetworkService struct {
    client *ent.Client
}

// GetNetwork returns a single network by ID.
func (s *NetworkService) GetNetwork(ctx context.Context, req *pb.GetNetworkRequest) (*pb.GetNetworkResponse, error) {
    network, err := s.client.Network.Get(ctx, int(req.GetId()))
    if err != nil {
        if ent.IsNotFound(err) {
            return nil, connect.NewError(connect.CodeNotFound,
                fmt.Errorf("entity network %d not found", req.GetId()))
        }
        return nil, connect.NewError(connect.CodeInternal,
            fmt.Errorf("get network %d: %w", req.GetId(), err))
    }
    return &pb.GetNetworkResponse{
        Network: networkToProto(network),
    }, nil
}
```

### Pattern 2: Ent-to-Proto Conversion Helpers
**What:** Typed helper functions that convert Go-native types to protobuf wrappers.
**When to use:** Every field conversion in every service handler's `xToProto` function.
**Example:**
```go
// Source: google.golang.org/protobuf/types/known/wrapperspb
package grpcserver

import (
    "time"

    "google.golang.org/protobuf/types/known/timestamppb"
    "google.golang.org/protobuf/types/known/wrapperspb"
)

// stringVal wraps a string as a StringValue. Returns nil for empty strings
// (matches ent Optional().Default("") fields that should appear as absent in proto).
func stringVal(s string) *wrapperspb.StringValue {
    if s == "" {
        return nil
    }
    return wrapperspb.String(s)
}

// stringPtrVal wraps a *string as a StringValue. Returns nil for nil pointers.
func stringPtrVal(s *string) *wrapperspb.StringValue {
    if s == nil {
        return nil
    }
    return wrapperspb.String(*s)
}

// int64Val wraps an int as an Int64Value. Returns nil for zero.
func int64Val(n int) *wrapperspb.Int64Value {
    return wrapperspb.Int64(int64(n))
}

// int64PtrVal wraps a *int as an Int64Value. Returns nil for nil pointers.
func int64PtrVal(n *int) *wrapperspb.Int64Value {
    if n == nil {
        return nil
    }
    return wrapperspb.Int64(int64(*n))
}

// timestampVal wraps a time.Time as a Timestamp.
func timestampVal(t time.Time) *timestamppb.Timestamp {
    return timestamppb.New(t)
}

// timestampPtrVal wraps a *time.Time as a Timestamp. Returns nil for nil pointers.
func timestampPtrVal(t *time.Time) *timestamppb.Timestamp {
    if t == nil {
        return nil
    }
    return timestamppb.New(*t)
}
```

### Pattern 3: Stateless Offset-Based Pagination
**What:** Page tokens encode the offset as a base64 string. Decoding produces the offset for the next query.
**When to use:** All List RPCs.
**Example:**
```go
// Source: Custom implementation following AIP-158 pattern
package grpcserver

import (
    "encoding/base64"
    "fmt"
    "strconv"
)

const (
    defaultPageSize = 100
    maxPageSize     = 1000
)

// normalizePageSize clamps page_size to valid range.
func normalizePageSize(requested int32) int {
    if requested <= 0 {
        return defaultPageSize
    }
    if requested > maxPageSize {
        return maxPageSize
    }
    return int(requested)
}

// decodePageToken returns the offset encoded in a page token.
// Returns 0 for empty tokens (first page).
func decodePageToken(token string) (int, error) {
    if token == "" {
        return 0, nil
    }
    b, err := base64.StdEncoding.DecodeString(token)
    if err != nil {
        return 0, fmt.Errorf("decode page token: %w", err)
    }
    offset, err := strconv.Atoi(string(b))
    if err != nil {
        return 0, fmt.Errorf("parse page offset: %w", err)
    }
    if offset < 0 {
        return 0, fmt.Errorf("invalid page offset: %d", offset)
    }
    return offset, nil
}

// encodePageToken encodes an offset as a page token.
// Returns empty string for offset 0 or negative (no next page).
func encodePageToken(offset int) string {
    if offset <= 0 {
        return ""
    }
    return base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(offset)))
}
```

### Pattern 4: Service Registration Loop in main.go
**What:** Register all 13 services and ecosystem handlers via a loop.
**When to use:** In `cmd/peeringdb-plus/main.go` where handlers are mounted on the mux.
**Example:**
```go
// Source: ConnectRPC handler constructor pattern
// Create shared handler options with OTel interceptor.
otelInterceptor, err := otelconnect.NewInterceptor(
    otelconnect.WithoutServerPeerAttributes(),
)
if err != nil {
    logger.Error("failed to create otel interceptor", slog.String("error", err.Error()))
    os.Exit(1)
}
handlerOpts := connect.WithInterceptors(otelInterceptor)

// Create service implementations.
networkSvc := &grpcserver.NetworkService{Client: entClient}
// ... 12 more services ...

// Register all ConnectRPC services.
services := []struct {
    path    string
    handler http.Handler
}{
    peeringdbv1connect.NewNetworkServiceHandler(networkSvc, handlerOpts),
    // ... 12 more ...
}
for _, svc := range services {
    // Note: NewXxxServiceHandler returns (string, http.Handler) which
    // destructures into the struct literal fields naturally, but we
    // need a small helper or explicit calls.
}
// Actually: use a helper to collect (path, handler) pairs
type pathHandler struct {
    path    string
    handler http.Handler
}
// ... then loop mux.Handle(ph.path, ph.handler)
```

### Pattern 5: Health Check Integration with Sync Readiness
**What:** Use `grpchealth.StaticChecker` with dynamic status updates from sync state.
**When to use:** OBS-04 requirement.
**Example:**
```go
// Source: connectrpc.com/grpchealth API
checker := grpchealth.NewStaticChecker(
    peeringdbv1connect.CampusServiceName,
    peeringdbv1connect.CarrierServiceName,
    // ... all 13 service names ...
)

// Mount health check handler on mux.
mux.Handle(grpchealth.NewHandler(checker, handlerOpts))

// Update health status based on sync readiness.
// The checker defaults to StatusServing. Set to NotServing until
// first sync completes by checking syncWorker.HasCompletedSync().
if !syncWorker.HasCompletedSync() {
    checker.SetStatus("", grpchealth.StatusNotServing)
}
// The status needs periodic updates -- simplest approach: update in
// the readiness middleware or a goroutine.
```

### Anti-Patterns to Avoid
- **Embedding UnimplementedXxxServiceHandler:** The generated `Unimplemented` types exist as safety nets. Do NOT embed them in the service structs because they return `CodeUnimplemented` errors. Instead, implement both `Get` and `List` methods directly. The compiler will catch missing methods since the handler interface requires both.
- **Exposing internal ent errors to clients:** Always wrap ent errors into appropriate gRPC status codes. Never return raw ent errors through the RPC boundary.
- **Using `int64` page tokens directly:** Even though the internal representation is an offset integer, the page token MUST be opaque to clients. Use base64 encoding so the implementation can change without breaking clients.
- **Forgetting to order List queries:** Without explicit ordering (`ORDER BY id`), pagination results may be inconsistent across pages. Always add `.Order(ent.Asc(xxx.FieldID))` to List queries.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| CORS for Connect protocols | Manual header lists for gRPC/Connect content types | `connectrpc.com/cors` helpers (`AllowedHeaders()`, `AllowedMethods()`, `ExposedHeaders()`) | Covers all three protocols (Connect, gRPC, gRPC-Web) with correct header sets; maintained by ConnectRPC team |
| gRPC server reflection | Custom proto descriptor serving | `connectrpc.com/grpcreflect` v1.3.0 | Wire-compatible with grpcurl/grpcui; handles both v1 and v1alpha APIs; uses global protobuf registry automatically |
| gRPC health checks | Custom health check proto implementation | `connectrpc.com/grpchealth` v1.4.0 | Wire-compatible with grpc-health-probe and Kubernetes; provides static checker with dynamic SetStatus |
| OpenTelemetry for RPC | Manual span creation in each handler | `connectrpc.com/otelconnect` v0.9.0 | Produces standard RPC attributes (rpc.system, rpc.service, rpc.method); handles both metrics and traces; one interceptor for all services |
| Protobuf wrapper constructors | Manual `&wrapperspb.StringValue{Value: s}` | `wrapperspb.String(s)`, `wrapperspb.Int64(n)`, `timestamppb.New(t)` | Standard protobuf helper constructors; less error-prone |

**Key insight:** The ConnectRPC ecosystem provides purpose-built packages for every cross-cutting concern in this phase. Each returns `(string, http.Handler)` pairs that compose naturally with `http.ServeMux`. Building custom solutions for reflection, health, or observability would be wrong.

## Common Pitfalls

### Pitfall 1: Ent ID Type Mismatch
**What goes wrong:** Ent uses `int` for entity IDs. Proto uses `int64`. Silent truncation on 32-bit systems or incorrect casts.
**Why it happens:** Go `int` is platform-dependent (32 or 64 bit). Proto `int64` is always 64-bit.
**How to avoid:** Always cast explicitly: `int(req.GetId())` for proto-to-ent, `int64(entity.ID)` for ent-to-proto. On 64-bit systems (Fly.io is amd64) this is safe but the cast must be explicit.
**Warning signs:** Compilation warnings about implicit int conversions.

### Pitfall 2: Nil Pointer Wrapping for Optional Fields
**What goes wrong:** Ent `Optional().Default("")` fields have Go type `string` (not `*string`), but the proto type is `*wrapperspb.StringValue`. Wrapping empty strings as `wrapperspb.String("")` sends an empty string instead of "absent."
**Why it happens:** Ent distinguishes `Optional()` (zero value present) from `Optional().Nillable()` (nil pointer). Proto wrappers use nil to indicate absent.
**How to avoid:** The `stringVal()` helper should return nil for empty strings when the ent field uses `Optional().Default("")`. For `Optional().Nillable()` fields (which produce `*string`), use `stringPtrVal()` which returns nil for nil pointers and wraps the value otherwise. Document per-field behavior.
**Warning signs:** API responses showing empty wrapper objects (`{"value":""}`) instead of absent fields.

### Pitfall 3: Missing ORDER BY in Paginated Queries
**What goes wrong:** Pages return inconsistent results; entities can be skipped or duplicated across pages.
**Why it happens:** SQLite does not guarantee row order without explicit ORDER BY. Offset-based pagination requires deterministic ordering.
**How to avoid:** Always append `.Order(ent.Asc(xxx.FieldID))` to List queries before applying `.Limit()` and `.Offset()`.
**Warning signs:** Flaky pagination tests; clients seeing duplicate or missing entities.

### Pitfall 4: Reflection Missing Service Names
**What goes wrong:** grpcurl/grpcui cannot discover services even though reflection is mounted.
**Why it happens:** `NewStaticReflector` takes fully-qualified service names. Missing any of the 13 means those services are invisible to reflection.
**How to avoid:** Use the generated constants (`peeringdbv1connect.CampusServiceName`, etc.) to build the service list. Assert length is 13 in tests.
**Warning signs:** grpcurl `list` shows fewer than 13 services.

### Pitfall 5: Health Checker Not Updating with Sync State
**What goes wrong:** Health check always returns SERVING even before first sync completes.
**Why it happens:** `grpchealth.NewStaticChecker` defaults all registered services to `StatusServing`. If the checker is not updated when sync hasn't completed, the health probe reports healthy prematurely.
**How to avoid:** Set initial status to `StatusNotServing` (or don't register services until sync completes), then transition to `StatusServing` after `syncWorker.HasCompletedSync()` returns true. A background goroutine or check on each request can manage this.
**Warning signs:** Load balancer routing traffic to nodes that have no data.

### Pitfall 6: CORS AllowedHeaders Must Include Connect-Specific Headers
**What goes wrong:** Browser-based gRPC-Web or Connect clients fail with CORS errors.
**Why it happens:** Connect protocol requires `Connect-Protocol-Version` and `Connect-Timeout-Ms` headers. gRPC-Web requires `X-Grpc-Web` and `Grpc-Timeout`. These are not standard CORS headers.
**How to avoid:** Use `connectcors.AllowedHeaders()` and merge with existing application headers (like `Authorization`, `Content-Type`). Also set `ExposedHeaders()` for response headers (`Grpc-Status`, `Grpc-Message`, `Grpc-Status-Details-Bin`).
**Warning signs:** Browser console shows CORS preflight failures for RPC requests.

## Code Examples

### Complete List RPC Implementation
```go
// Source: Derived from ent Query API + ConnectRPC patterns

// ListNetworks returns a paginated list of networks.
func (s *NetworkService) ListNetworks(ctx context.Context, req *pb.ListNetworksRequest) (*pb.ListNetworksResponse, error) {
    pageSize := normalizePageSize(req.GetPageSize())
    offset, err := decodePageToken(req.GetPageToken())
    if err != nil {
        return nil, connect.NewError(connect.CodeInvalidArgument,
            fmt.Errorf("invalid page token: %w", err))
    }

    networks, err := s.client.Network.Query().
        Order(ent.Asc(network.FieldID)).
        Limit(pageSize + 1). // Fetch one extra to detect next page
        Offset(offset).
        All(ctx)
    if err != nil {
        return nil, connect.NewError(connect.CodeInternal,
            fmt.Errorf("list networks: %w", err))
    }

    var nextPageToken string
    if len(networks) > pageSize {
        networks = networks[:pageSize] // Trim extra
        nextPageToken = encodePageToken(offset + pageSize)
    }

    pbNetworks := make([]*pb.Network, len(networks))
    for i, n := range networks {
        pbNetworks[i] = networkToProto(n)
    }

    return &pb.ListNetworksResponse{
        Networks:      pbNetworks,
        NextPageToken: nextPageToken,
    }, nil
}
```

### Ent-to-Proto Entity Conversion
```go
// Source: Field mapping from ent/network.go to gen/peeringdb/v1/v1.pb.go

func networkToProto(n *ent.Network) *pb.Network {
    return &pb.Network{
        Id:                       int64(n.ID),
        OrgId:                    int64PtrVal(n.OrgID),
        Aka:                      stringVal(n.Aka),
        AllowIxpUpdate:           n.AllowIxpUpdate,
        Asn:                      int64(n.Asn),
        InfoIpv6:                 n.InfoIpv6,
        InfoMulticast:            n.InfoMulticast,
        InfoNeverViaRouteServers: n.InfoNeverViaRouteServers,
        InfoPrefixes4:            int64PtrVal(n.InfoPrefixes4),
        InfoPrefixes6:            int64PtrVal(n.InfoPrefixes6),
        InfoRatio:                stringVal(n.InfoRatio),
        InfoScope:                stringVal(n.InfoScope),
        InfoTraffic:              stringVal(n.InfoTraffic),
        InfoType:                 stringVal(n.InfoType),
        InfoTypes:                n.InfoTypes,
        InfoUnicast:              n.InfoUnicast,
        IrrAsSet:                 stringVal(n.IrrAsSet),
        Logo:                     stringPtrVal(n.Logo),
        LookingGlass:             stringVal(n.LookingGlass),
        Name:                     n.Name,
        NameLong:                 stringVal(n.NameLong),
        Notes:                    stringVal(n.Notes),
        PolicyContracts:          stringVal(n.PolicyContracts),
        PolicyGeneral:            stringVal(n.PolicyGeneral),
        PolicyLocations:          stringVal(n.PolicyLocations),
        PolicyRatio:              n.PolicyRatio,
        PolicyUrl:                stringVal(n.PolicyURL),
        RirStatus:                stringPtrVal(n.RirStatus),
        RirStatusUpdated:         timestampPtrVal(n.RirStatusUpdated),
        RouteServer:              stringVal(n.RouteServer),
        StatusDashboard:          stringPtrVal(n.StatusDashboard),
        Website:                  stringVal(n.Website),
        IxCount:                  int64Val(n.IxCount),
        FacCount:                 int64Val(n.FacCount),
        NetixlanUpdated:          timestampPtrVal(n.NetixlanUpdated),
        NetfacUpdated:            timestampPtrVal(n.NetfacUpdated),
        PocUpdated:               timestampPtrVal(n.PocUpdated),
        Created:                  timestampVal(n.Created),
        Updated:                  timestampVal(n.Updated),
        Status:                   n.Status,
    }
}
```

### Reflection and Health Check Registration
```go
// Source: connectrpc.com/grpcreflect and connectrpc.com/grpchealth docs

import (
    "connectrpc.com/grpchealth"
    "connectrpc.com/grpcreflect"
    "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1/peeringdbv1connect"
)

// All 13 service names for reflection and health.
serviceNames := []string{
    peeringdbv1connect.CampusServiceName,
    peeringdbv1connect.CarrierServiceName,
    peeringdbv1connect.CarrierFacilityServiceName,
    peeringdbv1connect.FacilityServiceName,
    peeringdbv1connect.InternetExchangeServiceName,
    peeringdbv1connect.IxFacilityServiceName,
    peeringdbv1connect.IxLanServiceName,
    peeringdbv1connect.IxPrefixServiceName,
    peeringdbv1connect.NetworkServiceName,
    peeringdbv1connect.NetworkFacilityServiceName,
    peeringdbv1connect.NetworkIxLanServiceName,
    peeringdbv1connect.OrganizationServiceName,
    peeringdbv1connect.PocServiceName,
}

// Reflection: mount both v1 and v1alpha for tool compatibility.
reflector := grpcreflect.NewStaticReflector(serviceNames...)
mux.Handle(grpcreflect.NewHandlerV1(reflector, handlerOpts))
mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector, handlerOpts))

// Health check: create checker, mount handler.
checker := grpchealth.NewStaticChecker(serviceNames...)
mux.Handle(grpchealth.NewHandler(checker, handlerOpts))
```

### Updated CORS Configuration
```go
// Source: connectrpc.com/cors + existing internal/middleware/cors.go

import (
    connectcors "connectrpc.com/cors"
    "github.com/rs/cors"
)

func CORS(in CORSInput) func(http.Handler) http.Handler {
    origins := strings.Split(in.AllowedOrigins, ",")
    for i := range origins {
        origins[i] = strings.TrimSpace(origins[i])
    }

    // Merge application headers with Connect protocol headers.
    allowedHeaders := append([]string{"Content-Type", "Authorization"}, connectcors.AllowedHeaders()...)
    allowedMethods := append([]string{"GET", "OPTIONS"}, connectcors.AllowedMethods()...)
    exposedHeaders := connectcors.ExposedHeaders()

    c := cors.New(cors.Options{
        AllowedOrigins:   origins,
        AllowedMethods:   allowedMethods,
        AllowedHeaders:   allowedHeaders,
        ExposedHeaders:   exposedHeaders,
        AllowCredentials: false,
        MaxAge:           7200,
    })
    return c.Handler
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `connect.NewRequest(req)`/`connect.NewResponse(resp)` wrapper pattern | Simple handler: `(ctx, *Req) (*Resp, error)` | connect v1.18.0+ | Handlers accept/return plain proto types; no wrapper boilerplate |
| `bufbuild/connect-opentelemetry-go` | `connectrpc.com/otelconnect` | 2023 | Import path changed during ConnectRPC rename from bufbuild |
| `bufbuild/connect-grpcreflect-go` | `connectrpc.com/grpcreflect` | 2023 | Import path changed |
| Manual gRPC health proto implementation | `connectrpc.com/grpchealth` | 2023 | Static checker with SetStatus API |

**Deprecated/outdated:**
- `github.com/bufbuild/connect-opentelemetry-go`: Relocated to `connectrpc.com/otelconnect`
- `github.com/bufbuild/connect-grpcreflect-go`: Relocated to `connectrpc.com/grpcreflect`
- `github.com/bufbuild/connect-grpchealth-go`: Relocated to `connectrpc.com/grpchealth`

## Open Questions

1. **Health check status update mechanism**
   - What we know: `grpchealth.StaticChecker.SetStatus()` is thread-safe and can be called dynamically. The existing `syncWorker.HasCompletedSync()` provides the readiness signal. The initial state should be `StatusNotServing`.
   - What's unclear: Whether to use a goroutine polling `HasCompletedSync()`, or to integrate the status flip into the existing readiness middleware, or to use a callback from the sync worker.
   - Recommendation: Simplest approach: set initial status to NotServing, then start a one-shot goroutine that polls `HasCompletedSync()` every second and transitions to Serving once true. This is self-terminating and clear. Alternatively, the sync worker could accept a callback, but that couples the packages.

2. **social_media field handling in proto**
   - What we know: The `social_media` field is annotated with `entproto.Skip()` in the ent schema, so it is NOT present in the generated proto messages. The `common.proto` defines a `SocialMedia` message type but it's not referenced by the entity messages.
   - What's unclear: Whether this is intentional or an oversight from Phase 22.
   - Recommendation: This is intentional -- `social_media` is a JSON field of type `[]SocialMedia` that entproto cannot map automatically. It was explicitly skipped. The proto API simply omits this field. This is acceptable for v1; if needed later, the field can be added to the proto manually.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | Everything | Yes | 1.26.1 | -- |
| golangci-lint | Linting (CI) | Yes | installed | -- |
| buf | Proto generation (Phase 22 only) | Yes (alias) | go run latest | Not needed this phase |
| grpcurl | Manual testing of reflection/services | No | -- | Use `buf curl` via go run, or write Go test clients using generated client code |
| grpcui | Manual testing of reflection | No | -- | Not needed for automated testing; install via `go install` if desired |
| grpc-health-probe | Manual testing of health check | No | -- | Use `buf curl` or Go test client |

**Missing dependencies with no fallback:**
- None -- all missing tools have Go-based alternatives.

**Missing dependencies with fallback:**
- grpcurl/grpcui: Not installed. For automated testing, use the generated ConnectRPC client code directly in Go tests. For manual verification, install via `go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest` or use `buf curl`.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) + ent/enttest |
| Config file | None needed (Go conventions) |
| Quick run command | `go test ./internal/grpcserver/ -race -count=1` |
| Full suite command | `go test -race -count=1 ./...` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| API-01 | Get RPC returns entity by ID; returns NOT_FOUND for missing ID | unit | `go test ./internal/grpcserver/ -run TestGetNetwork -race -count=1` | Wave 0 |
| API-02 | List RPC returns paginated results; page_size/page_token work correctly | unit | `go test ./internal/grpcserver/ -run TestListNetworks -race -count=1` | Wave 0 |
| API-04 | All 13 services registered on mux at correct path prefixes | integration | `go test ./cmd/peeringdb-plus/ -run TestConnectRPC -race -count=1` | Wave 0 |
| OBS-01 | otelconnect interceptor produces RPC spans | unit | `go test ./internal/grpcserver/ -run TestOtelInterceptor -race -count=1` | Wave 0 |
| OBS-02 | CORS allows Connect protocol headers | unit | `go test ./internal/middleware/ -run TestCORS -race -count=1` | Existing (update needed) |
| OBS-03 | Reflection discovers all 13 services | integration | `go test ./cmd/peeringdb-plus/ -run TestReflection -race -count=1` | Wave 0 |
| OBS-04 | Health check reports correct serving status | unit | `go test ./internal/grpcserver/ -run TestHealthCheck -race -count=1` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/grpcserver/ -race -count=1`
- **Per wave merge:** `go test -race -count=1 ./...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/grpcserver/grpcserver_test.go` -- covers API-01, API-02, OBS-01
- [ ] `internal/grpcserver/pagination_test.go` -- covers cursor encoding/decoding edge cases
- [ ] Tests can use generated ConnectRPC clients (`peeringdbv1connect.NewXxxServiceClient`) against `httptest.Server` for clean integration testing

*(Existing `internal/middleware/cors_test.go` covers OBS-02 but needs updating for new Connect headers.)*

## Project Constraints (from CLAUDE.md)

The following directives from CLAUDE.md apply directly to this phase:

- **CS-2 (MUST):** Avoid stutter -- package `grpcserver`, type `NetworkService` (not `GRPCNetworkService`)
- **CS-5 (MUST):** Input structs for functions with >2 args -- service constructors should use struct if they grow beyond client
- **CS-6 (SHOULD):** Declare input structs before consuming functions
- **ERR-1 (MUST):** Wrap errors with `%w` and context
- **ERR-2 (MUST):** Use `errors.Is`/`errors.As` for control flow -- use `ent.IsNotFound(err)` not string matching
- **CTX-1 (MUST):** Context as first parameter -- matches ConnectRPC handler signatures
- **T-1 (MUST):** Table-driven tests for Get/List operations across entity types
- **T-2 (MUST):** `-race` flag in all test runs; `t.Cleanup` for teardown
- **T-3 (SHOULD):** Mark safe tests with `t.Parallel()`
- **OBS-1 (MUST):** Structured logging with slog
- **OBS-4 (SHOULD):** Use OpenTelemetry for observability -- otelconnect interceptor
- **API-1 (MUST):** Document exported items
- **API-2 (MUST):** Accept interfaces where variation needed; return concrete types
- **SEC-1 (MUST):** Validate inputs (page_size bounds, page_token format)

## Sources

### Primary (HIGH confidence)
- [connectrpc.com/otelconnect v0.9.0](https://pkg.go.dev/connectrpc.com/otelconnect) - OTel interceptor API, options, usage
- [connectrpc.com/grpcreflect v1.3.0](https://pkg.go.dev/connectrpc.com/grpcreflect) - Reflection API, NewStaticReflector, handler constructors
- [connectrpc.com/grpchealth v1.4.0](https://pkg.go.dev/connectrpc.com/grpchealth) - Health check API, StaticChecker, SetStatus
- [connectrpc.com/cors](https://pkg.go.dev/connectrpc.com/cors) - CORS helper functions for Connect protocols
- [ConnectRPC CORS documentation](https://connectrpc.com/docs/cors/) - Required CORS headers for Connect/gRPC/gRPC-Web
- [ConnectRPC error handling](https://connectrpc.com/docs/go/errors/) - Error codes and connect.NewError
- Generated code in `gen/peeringdb/v1/` - Handler interfaces, proto types, service name constants
- Ent generated code in `ent/` - Client.Get(), Query().Limit().Offset().All(), IsNotFound()
- Existing codebase (`cmd/peeringdb-plus/main.go`, `internal/middleware/cors.go`, `internal/health/handler.go`)

### Secondary (MEDIUM confidence)
- [ConnectRPC observability docs](https://connectrpc.com/docs/go/observability/) - WithoutServerPeerAttributes rationale
- [ConnectRPC getting started](https://connectrpc.com/docs/go/getting-started/) - Simple handler pattern confirmation

### Tertiary (LOW confidence)
- None

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - All packages verified via pkg.go.dev with specific versions; already compatible with project dependencies
- Architecture: HIGH - Pattern derives directly from generated code analysis and ConnectRPC documentation
- Pitfalls: HIGH - Based on ent schema analysis (wrapper types, optional fields) and pagination best practices
- Field mapping: HIGH - Verified by comparing ent generated structs against proto generated structs field-by-field

**Research date:** 2026-03-25
**Valid until:** 2026-04-25 (stable ecosystem, all packages at v1.x or near-v1.x)
