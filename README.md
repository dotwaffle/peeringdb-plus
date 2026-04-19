# PeeringDB Plus

[![CI](https://github.com/dotwaffle/peeringdb-plus/actions/workflows/ci.yml/badge.svg)](https://github.com/dotwaffle/peeringdb-plus/actions/workflows/ci.yml)

A high-performance, globally distributed, read-only mirror of
[PeeringDB](https://www.peeringdb.com) data. Syncs all PeeringDB objects on a
configurable schedule, stores them in SQLite with edge replication via LiteFS on
Fly.io, and serves the data through five API surfaces.

**Live instance:** [peeringdb-plus.fly.dev](https://peeringdb-plus.fly.dev)

## API Surfaces

| Surface | Path | Description |
|---------|------|-------------|
| Web UI | `/ui/` | Search, detail pages, and multi-entity comparison |
| GraphQL | `/graphql` | Interactive GraphiQL playground + query endpoint |
| REST | `/rest/v1/` | OpenAPI-compliant ([spec](/rest/v1/openapi.json)) |
| PeeringDB Compat | `/api/` | Drop-in replacement for the PeeringDB API |
| ConnectRPC | `/peeringdb.v1.*/` | Get/List/Stream RPCs for all 13 entity types |

The root endpoint (`GET /`) returns a JSON service discovery document. Terminal
clients receive help text; browsers redirect to the Web UI.

## Quick Start

### Local (Go)

```bash
go build -o peeringdb-plus ./cmd/peeringdb-plus
./peeringdb-plus
# Listening on :8080, syncing from api.peeringdb.com every hour
```

### Local (Docker)

```bash
docker build -t peeringdb-plus .
docker run -p 8080:8080 peeringdb-plus
```

The database is stored at `/data/peeringdb-plus.db` inside the container. Mount
a volume to persist data across restarts:

```bash
docker run -p 8080:8080 -v pdbdata:/data peeringdb-plus
```

### Prerequisites

- Go 1.26+

All code generation tools (`buf`, `templ`, `gqlgen`) are Go tool dependencies
declared in `go.mod` and require no separate installation.

## Data Sync

PeeringDB data is synced automatically on a configurable interval (default:
hourly). The sync runs on the primary node only; replicas receive updates via
LiteFS replication.

- **Modes:** `full` (complete re-fetch, default) or `incremental` (only
  modified objects since last sync).
- **On-demand trigger:** `POST /sync` with `X-Sync-Token` header (requires
  `PDBPLUS_SYNC_TOKEN` to be set). Accepts `?mode=full|incremental`.
- **Readiness:** `/readyz` reports unhealthy until the first sync completes and
  degrades if sync data exceeds the staleness threshold (default: 24h).

## Configuration

All configuration is via environment variables, validated at startup (fail-fast).

| Variable | Default | Description |
|---|---|---|
| `PDBPLUS_LISTEN_ADDR` | `:8080` | HTTP listen address (overridden by `PDBPLUS_PORT`) |
| `PDBPLUS_DB_PATH` | `./peeringdb-plus.db` | SQLite database file path |
| `PDBPLUS_PEERINGDB_URL` | `https://api.peeringdb.com` | PeeringDB API base URL |
| `PDBPLUS_PEERINGDB_API_KEY` | | Optional API key for higher rate limits |
| `PDBPLUS_SYNC_TOKEN` | | Shared secret for `POST /sync`; empty = disabled |
| `PDBPLUS_SYNC_INTERVAL` | `1h` | Duration between automatic syncs |
| `PDBPLUS_SYNC_MODE` | `full` | Sync strategy: `full` or `incremental` |
| `PDBPLUS_SYNC_STALE_THRESHOLD` | `24h` | Max sync age before readiness degrades |
| `PDBPLUS_CORS_ORIGINS` | `*` | Comma-separated allowed CORS origins |
| `PDBPLUS_DRAIN_TIMEOUT` | `10s` | Graceful shutdown drain timeout |
| `PDBPLUS_OTEL_SAMPLE_RATE` | `1.0` | Trace sampling ratio (0.0-1.0) |
| `PDBPLUS_STREAM_TIMEOUT` | `60s` | Max duration for streaming RPCs |
| `PDBPLUS_IS_PRIMARY` | `true` | Fallback primary detection without LiteFS |

Standard `OTEL_*` environment variables are also supported (autoexport).

## ConnectRPC / gRPC

All 13 PeeringDB entity types are available via ConnectRPC with Get, List, and
Stream RPCs. The server supports Connect, gRPC, and gRPC-Web protocols on the
same endpoints.

### Service Discovery

```bash
# List all services (requires grpcurl or buf curl)
grpcurl -plaintext localhost:8080 list

# Describe a service
grpcurl -plaintext localhost:8080 describe peeringdb.v1.NetworkService
```

gRPC reflection (v1 and v1alpha) and health checks are enabled.

### Unary RPCs

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

All entity types support server-streaming for bulk data retrieval, eliminating
manual pagination. Streams accept the same filters as List RPCs.

| Service | Stream RPC |
|---------|-----------|
| CampusService | StreamCampuses |
| CarrierService | StreamCarriers |
| CarrierFacilityService | StreamCarrierFacilities |
| FacilityService | StreamFacilities |
| InternetExchangeService | StreamInternetExchanges |
| IxFacilityService | StreamIxFacilities |
| IxLanService | StreamIxLans |
| IxPrefixService | StreamIxPrefixes |
| NetworkService | StreamNetworks |
| NetworkFacilityService | StreamNetworkFacilities |
| NetworkIxLanService | StreamNetworkIxLans |
| OrganizationService | StreamOrganizations |
| PocService | StreamPocs |

```bash
# Stream all networks
buf curl --protocol grpc --http2-prior-knowledge \
  http://localhost:8080/peeringdb.v1.NetworkService/StreamNetworks \
  -d '{}'

# Stream with filter
buf curl --protocol grpc --http2-prior-knowledge \
  http://localhost:8080/peeringdb.v1.NetworkService/StreamNetworks \
  -d '{"asn": 15169}'
```

The `grpc-total-count` response header contains the approximate total matching
records. Streams have a server-side timeout (default 60s, configurable via
`PDBPLUS_STREAM_TIMEOUT`) and can be cancelled by the client at any time.

### Content Types

| Content-Type | Format | Use case |
|---|---|---|
| `application/proto` | Protocol Buffers | Smallest wire size |
| `application/json` | JSON | Human-readable, debugging |
| `application/grpc` | gRPC (proto) | Standard gRPC clients |
| `application/grpc+json` | gRPC (JSON) | gRPC clients wanting JSON |

## PeeringDB Compat API

The `/api/` endpoint is a drop-in replacement for the PeeringDB REST API:

```bash
# List networks
curl https://peeringdb-plus.fly.dev/api/net

# Get a specific network
curl https://peeringdb-plus.fly.dev/api/net/42

# Search with query parameters
curl 'https://peeringdb-plus.fly.dev/api/net?q=cloudflare&limit=5'
```

Supports `depth`, `limit`, `skip`, `fields`, `since`, and `q` query parameters.

## Development

```bash
go build ./...                    # Build all packages
go test -race ./...               # Run tests with race detector
go generate ./...                 # Full codegen pipeline (ent, templ, proto)
go vet ./...                      # Vet
golangci-lint run                 # Lint
govulncheck ./...                 # Vulnerability check
```

## Technology

- **Go 1.26+** with [entgo](https://entgo.io) ORM
- **SQLite** via [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) (pure Go, no CGO)
- **LiteFS** for edge replication on Fly.io
- **ConnectRPC** for gRPC/Connect/gRPC-Web on standard net/http
- **OpenTelemetry** for tracing, metrics, and structured logging
- **templ** + **htmx** + **Tailwind CSS** for the web UI
- **gqlgen** via entgql for GraphQL
- **entrest** for OpenAPI-compliant REST
- **Chainguard** base images for minimal container footprint

## License

BSD 3-Clause. See [LICENSE](LICENSE).
