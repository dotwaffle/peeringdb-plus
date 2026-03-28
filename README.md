# PeeringDB Plus

A high-performance, globally distributed, read-only mirror of PeeringDB data.
Syncs all PeeringDB objects via full re-fetch (hourly or on-demand), stores them
in SQLite on LiteFS for edge-local reads on Fly.io, and exposes the data through
five API surfaces.

## API Surfaces

| Surface | Path | Description |
|---------|------|-------------|
| Web UI | `/ui/` | templ + htmx + Tailwind CSS (search, detail pages, ASN comparison) |
| GraphQL | `/graphql` | gqlgen via entgql, interactive playground |
| REST | `/rest/v1/` | entrest, OpenAPI-compliant |
| PeeringDB Compat | `/api/` | Drop-in replacement for PeeringDB API |
| ConnectRPC/gRPC | `/peeringdb.v1.*/` | Get/List/Stream RPCs for all 13 types |

## ConnectRPC / gRPC API

All 13 PeeringDB entity types are available via ConnectRPC with Get, List, and
Stream RPCs. The server supports Connect, gRPC, and gRPC-Web protocols on the
same endpoints.

### Unary RPCs

Each entity type has `Get` (by ID) and `List` (paginated with filters) RPCs.

```bash
# Get a single network by ID
buf curl --protocol grpc --http2-prior-knowledge \
  http://localhost:8080/peeringdb.v1.NetworkService/GetNetwork \
  -d '{"id": 42}'

# List networks with filters and pagination
buf curl --protocol grpc --http2-prior-knowledge \
  http://localhost:8080/peeringdb.v1.NetworkService/ListNetworks \
  -d '{"page_size": 10, "status": "ok"}'
```

### Streaming RPCs

All 13 entity types support server-streaming RPCs for bulk data retrieval.
Streaming RPCs deliver records one message at a time, eliminating the need
for manual pagination.

**Available RPCs:**

| Service | RPC | Streams |
|---------|-----|---------|
| CampusService | StreamCampuses | Campus |
| CarrierService | StreamCarriers | Carrier |
| CarrierFacilityService | StreamCarrierFacilities | CarrierFacility |
| FacilityService | StreamFacilities | Facility |
| InternetExchangeService | StreamInternetExchanges | InternetExchange |
| IxFacilityService | StreamIxFacilities | IxFacility |
| IxLanService | StreamIxLans | IxLan |
| IxPrefixService | StreamIxPrefixes | IxPrefix |
| NetworkService | StreamNetworks | Network |
| NetworkFacilityService | StreamNetworkFacilities | NetworkFacility |
| NetworkIxLanService | StreamNetworkIxLans | NetworkIxLan |
| OrganizationService | StreamOrganizations | Organization |
| PocService | StreamPocs | Poc |

**Usage with buf curl:**

```bash
# Stream all networks (proto format)
buf curl --protocol grpc --http2-prior-knowledge \
  http://localhost:8080/peeringdb.v1.NetworkService/StreamNetworks \
  -d '{}'

# Stream networks filtered by ASN (JSON format)
buf curl --protocol grpc --http2-prior-knowledge \
  http://localhost:8080/peeringdb.v1.NetworkService/StreamNetworks \
  -d '{"asn": 15169}'
```

**Format negotiation:**

ConnectRPC supports multiple formats on the same endpoint:

| Content-Type | Format | Use case |
|-------------|--------|----------|
| `application/proto` | Protocol Buffers | Smallest wire size, fastest parsing |
| `application/json` | JSON | Human-readable, debugging |
| `application/grpc` | gRPC (proto) | Standard gRPC clients (grpcurl, etc.) |
| `application/grpc+json` | gRPC (JSON) | gRPC clients wanting JSON payloads |

Clients set the format via the `Content-Type` request header. ConnectRPC
handles serialization automatically.

**Response headers:**

| Header | Value | Description |
|--------|-------|-------------|
| `grpc-total-count` | integer | Approximate total records matching the filter. Available before the first message. May differ from actual streamed count due to concurrent updates. |

**Filters:**

Each streaming RPC accepts the same optional filter fields as its `List`
counterpart (minus pagination fields). Multiple filters combine with AND
logic. Omitted fields impose no constraint.

**Cancellation:**

Clients can cancel a stream at any time by closing the connection. The
server terminates the query loop within one batch cycle (~500 rows).

**Timeouts:**

Streams have a server-side timeout (default 60 seconds, configurable via
`PDBPLUS_STREAM_TIMEOUT` environment variable). The timeout covers the
entire stream duration, not individual messages.

## Development

### Prerequisites

- Go 1.26+

All code generation tools (`buf`, `templ`, `gqlgen`) are declared as Go tool
dependencies in `go.mod` and require no separate installation.

### Build

```bash
go build ./...
```

### Code Generation

```bash
go generate ./...
```

This runs the full pipeline in dependency order: ent codegen (schemas, GraphQL,
REST, protobuf), buf generate (proto Go types), and templ generate (HTML
templates).

### Test

```bash
go test -race ./...
```

### Lint

```bash
go vet ./...
golangci-lint run
govulncheck ./...
```

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `PDBPLUS_STREAM_TIMEOUT` | `60s` | Maximum duration for streaming RPCs |
| `PDBPLUS_PEERINGDB_API_KEY` | (none) | Optional PeeringDB API key for higher sync rate limits |

## Technology

- **Go 1.26** with entgo ORM
- **SQLite** via modernc.org/sqlite (CGo-free)
- **LiteFS** for edge replication on Fly.io
- **ConnectRPC** for gRPC/Connect/gRPC-Web on standard net/http
- **OpenTelemetry** for tracing, metrics, and logs
- **templ** + **htmx** + **Tailwind CSS** for the web UI
- **gqlgen** via entgql for GraphQL
- **entrest** for OpenAPI-compliant REST
- **Chainguard** base images for minimal container footprint
