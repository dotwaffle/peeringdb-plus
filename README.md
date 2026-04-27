<!-- generated-by: gsd-doc-writer -->
# PeeringDB Plus

[![CI](https://github.com/dotwaffle/peeringdb-plus/actions/workflows/ci.yml/badge.svg)](https://github.com/dotwaffle/peeringdb-plus/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/dotwaffle/peeringdb-plus.svg)](https://pkg.go.dev/github.com/dotwaffle/peeringdb-plus)
[![License: BSD-3-Clause](https://img.shields.io/badge/License-BSD_3--Clause-blue.svg)](LICENSE)

A high-performance, globally distributed, read-only mirror of
[PeeringDB](https://www.peeringdb.com) data. PeeringDB Plus periodically
re-fetches every PeeringDB object, stores the result in SQLite with edge
replication via [LiteFS](https://fly.io/docs/litefs/) on
[Fly.io](https://fly.io), and serves the same dataset through five coexisting
API surfaces.

**Live instance:** <https://peeringdb-plus.fly.dev>

---

## Why

The upstream PeeringDB API is the canonical source for peering coordination
data, but it is single-region and rate-limited. PeeringDB Plus offers:

- **Low-latency reads** from the nearest Fly.io region (LiteFS replicates the
  SQLite WAL to every replica).
- **Multiple wire formats from a single dataset** — drop-in PeeringDB API
  compatibility, OpenAPI REST, GraphQL, ConnectRPC/gRPC, and a Web UI all read
  from the same `ent.Client`.
- **Mandatory observability** — OpenTelemetry traces, metrics, and structured
  logs are first-class, not an afterthought.
- **No CGO, no Java, no orchestrator** — a single static Go binary plus an
  out-of-process LiteFS FUSE mount.

## API surfaces

All five surfaces are mounted on the same HTTP server and read from the same
SQLite database.

| Surface | Path | Description |
|---|---|---|
| Web UI | `/ui/` | templ + htmx search, detail pages, ASN comparison; content-negotiates terminal vs browser |
| GraphQL | `/graphql` | gqlgen via entgql; interactive playground on `GET`, queries on `POST` |
| REST | `/rest/v1/` | OpenAPI-compliant (entrest); spec at [`/rest/v1/openapi.json`](https://peeringdb-plus.fly.dev/rest/v1/openapi.json) |
| PeeringDB Compat | `/api/` | Drop-in replacement for `api.peeringdb.com` — same paths, same envelope |
| ConnectRPC / gRPC | `/peeringdb.v1.*/` | Get / List / Stream RPCs for all 13 entity types; reflection + health checks enabled |

`GET /` returns a JSON service-discovery document; browsers are redirected to
the Web UI, terminal clients receive plain help text.

See [`docs/API.md`](docs/API.md) for the full surface catalogue, including
filter semantics, ordering guarantees, divergences, and the response-memory
envelope governing pdbcompat list responses.

## Quick start

### Local (Go)

```bash
git clone https://github.com/dotwaffle/peeringdb-plus.git
cd peeringdb-plus
go build -o peeringdb-plus ./cmd/peeringdb-plus
./peeringdb-plus
# Listening on :8080, syncing from api.peeringdb.com every hour
```

The first sync takes 30-60 seconds against the public PeeringDB API. While it
runs, `/healthz` returns 200 and `/readyz` returns 503; once the database is
populated, `/readyz` flips to 200 and every API surface is usable.

### Local (Docker)

```bash
docker build -t peeringdb-plus .
docker run -p 8080:8080 -v pdbdata:/data peeringdb-plus
```

The image stores the database at `/data/peeringdb-plus.db`; mount a volume to
persist data across container restarts.

### Verify it's working

```bash
curl -s http://localhost:8080/healthz                    # liveness
curl -sI http://localhost:8080/readyz                    # readiness (200 once first sync completes)
curl -s http://localhost:8080/api/net/1 | head -c 500    # PeeringDB-compatible API
open http://localhost:8080/ui/                           # Web UI
open http://localhost:8080/graphql                       # GraphQL playground
```

For the full first-30-minutes walkthrough, see
[`docs/GETTING-STARTED.md`](docs/GETTING-STARTED.md).

## Usage examples

### PeeringDB-compatible API

```bash
# List networks
curl https://peeringdb-plus.fly.dev/api/net

# Fetch a specific network by PeeringDB numeric ID
curl https://peeringdb-plus.fly.dev/api/net/42

# Search with query parameters (depth/limit/skip/fields/since/q all supported)
curl 'https://peeringdb-plus.fly.dev/api/net?q=cloudflare&limit=5'
```

### ConnectRPC / gRPC

ConnectRPC, gRPC, and gRPC-Web all share the same endpoints. Reflection
(`grpcreflect.NewHandlerV1` / `V1Alpha`) and the standard gRPC health check
service are enabled, so `grpcurl` and gRPC-aware load balancers work
out-of-the-box.

```bash
# List all services
grpcurl -plaintext localhost:8080 list

# Get a single network by ID
buf curl --protocol grpc --http2-prior-knowledge \
  http://localhost:8080/peeringdb.v1.NetworkService/GetNetwork \
  -d '{"id": 42}'

# Stream all networks (server-streaming, no manual pagination)
buf curl --protocol grpc --http2-prior-knowledge \
  http://localhost:8080/peeringdb.v1.NetworkService/StreamNetworks \
  -d '{"asn": 15169}'
```

Streams accept the same filters as `List*` RPCs; the `grpc-total-count`
response header carries the approximate total. Server-side timeout defaults to
60s (`PDBPLUS_STREAM_TIMEOUT`) and clients can cancel at any time.

### GraphQL

```bash
curl -X POST http://localhost:8080/graphql \
  -H 'Content-Type: application/json' \
  -d '{"query":"{ networks(first: 3) { edges { node { id name asn } } } }"}'
```

## Configuration

PeeringDB Plus is configured exclusively via environment variables, validated
at startup with fail-fast diagnostics. Operationally-relevant defaults:

| Variable | Default | Purpose |
|---|---|---|
| `PDBPLUS_LISTEN_ADDR` | `:8080` | HTTP listen address |
| `PDBPLUS_DB_PATH` | `./peeringdb-plus.db` | SQLite database file path |
| `PDBPLUS_PEERINGDB_URL` | `https://api.peeringdb.com` | Upstream PeeringDB API |
| `PDBPLUS_PEERINGDB_API_KEY` | _(unset)_ | Optional — raises rate limit and shortens default sync interval |
| `PDBPLUS_SYNC_MODE` | `incremental` | `incremental` (delta) or `full` (re-fetch); `full` is the operator escape-hatch |
| `PDBPLUS_SYNC_INTERVAL` | `1h` (15m if API key set) | Time between sync cycles |
| `PDBPLUS_RESPONSE_MEMORY_LIMIT` | `128MiB` | pdbcompat list pre-flight 413 budget |
| `PDBPLUS_PUBLIC_TIER` | `public` | Anonymous-caller tier; set `users` only for private deployments |

The full catalogue (sync, observability, LiteFS, Fly.io, CSP, security
headers, and OAuth-gated visibility) lives in
[`docs/CONFIGURATION.md`](docs/CONFIGURATION.md). Standard `OTEL_*` env vars
are honoured via OpenTelemetry autoexport.

## Documentation

| Doc | Topic |
|---|---|
| [`docs/GETTING-STARTED.md`](docs/GETTING-STARTED.md) | First-30-minutes walkthrough: prerequisites, build, first run, verification |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | Component diagram, data flow, code-generation pipeline, middleware chain, privacy layer, sampling matrix |
| [`docs/API.md`](docs/API.md) | All five API surfaces, filter semantics, ordering, cross-entity traversal, divergences |
| [`docs/CONFIGURATION.md`](docs/CONFIGURATION.md) | Full environment-variable catalogue with validation rules |
| [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md) | Local dev workflow, code generation, conventions, sibling-file pattern |
| [`docs/TESTING.md`](docs/TESTING.md) | Test layout, fixtures, parity harness, live tests against `beta.peeringdb.com` |
| [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md) | Fly.io rollout, asymmetric fleet topology, LiteFS operations |
| [`CONTRIBUTING.md`](CONTRIBUTING.md) | How to propose changes, run the verification suite, and submit a PR |

## Technology

- **Language:** Go 1.26.1+
- **ORM / codegen:** [entgo](https://entgo.io/) drives all five API surfaces
  from a single set of schemas in `ent/schema/` (entgql + entrest + entproto)
- **Database:** [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite)
  (pure Go, no CGO) plus [LiteFS](https://fly.io/docs/litefs/) for edge
  replication
- **RPC:** [ConnectRPC](https://connectrpc.com/) — gRPC, gRPC-Web, and the
  Connect protocol on the same handlers
- **GraphQL:** [gqlgen](https://gqlgen.com/) wired through entgql
- **Web UI:** [templ](https://templ.guide/) + [htmx](https://htmx.org/) +
  Tailwind CSS — zero JavaScript build toolchain
- **Observability:** [OpenTelemetry](https://opentelemetry.io/) with the
  stdlib `log/slog` bridge; per-route trace sampling; per-VM runtime metrics
- **Platform:** [Fly.io](https://fly.io) with an asymmetric `primary` /
  `replica` process-group fleet
- **Container base:** [Chainguard](https://www.chainguard.dev/) minimal
  images (`cgr.dev/chainguard/go`, `cgr.dev/chainguard/glibc-dynamic`)

## Development

```bash
go build ./...                    # Build all packages
go test -race ./...               # Run tests with race detector
go generate ./...                 # Full codegen pipeline (ent + buf + templ)
golangci-lint run                 # Lint
govulncheck ./...                 # Vulnerability scan
```

CI runs the full pipeline plus a generated-code-drift check on every PR. See
[`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md) for the conventions, including
the sibling-file pattern that protects hand-edited schema methods from being
overwritten by `cmd/pdb-schema-generate`.

## Contributing

PeeringDB Plus is open source. Bug reports, feature suggestions, and pull
requests are welcome — please read [`CONTRIBUTING.md`](CONTRIBUTING.md)
before opening a PR.

## License

BSD 3-Clause. See [`LICENSE`](LICENSE).
