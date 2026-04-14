<!-- generated-by: gsd-doc-writer -->
# Getting Started

This guide walks a new contributor or operator through their first 30 minutes
with PeeringDB Plus: installing prerequisites, cloning and building the source,
starting the binary, understanding what happens on first launch, and verifying
the service is healthy across its API surfaces.

For a quick-reference command list, see the [README](../README.md). For the
full environment variable catalogue, see [CONFIGURATION.md](CONFIGURATION.md).
For how the pieces fit together, see [ARCHITECTURE.md](ARCHITECTURE.md).

## Prerequisites

| Requirement | Version | Why |
|-------------|---------|-----|
| Go toolchain | `1.26.1` or newer | Declared in `go.mod`. The project uses Go 1.22+ `net/http` method routing and Go 1.26 toolchain features. |
| Git | any recent | Cloning the repo. |
| Docker (optional) | any recent | Only if you want to run the container image instead of a local binary. |
| `grpcurl` or `buf` CLI (optional) | any recent | Only needed to poke the ConnectRPC/gRPC endpoints manually. |
| A few hundred MB of disk | — | The SQLite database grows to roughly 200–400 MB after a full sync. |

Everything else — `buf`, `templ`, `gqlgen` — is a Go tool dependency declared
in `go.mod`. You do not need to install them separately; `go generate` and
`go build` will fetch them on demand.

No CGO is required. The project uses `modernc.org/sqlite` (pure Go) and builds
with `CGO_ENABLED=0` by default.

## 1. Clone and build

```bash
git clone https://github.com/dotwaffle/peeringdb-plus.git
cd peeringdb-plus
go build -o peeringdb-plus ./cmd/peeringdb-plus
```

The build produces a single static binary at `./peeringdb-plus`. A bare
`go build ./...` also works and is what CI runs.

If you intend to make changes, run the full verification suite once to
confirm your toolchain is set up correctly:

```bash
go test -race ./...
go vet ./...
```

## 2. First run

```bash
./peeringdb-plus
```

With no configuration, the binary uses the defaults from
`internal/config/config.go`:

- Listens on `:8080` with h2c (HTTP/2 cleartext — required for ConnectRPC).
- Stores data in `./peeringdb-plus.db` in the current working directory.
- Syncs from `https://api.peeringdb.com` with a 1-hour interval.
- Assumes it is the LiteFS primary (`PDBPLUS_IS_PRIMARY=true`), so the sync
  worker is active.

You will see a `starting server` log line almost immediately. The HTTP
listener accepts connections right away, but **`/readyz` will return 503
until the first sync completes**.

### What happens on first start

1. The config loader validates every environment variable and aborts with a
   descriptive error if anything is wrong (fail-fast).
2. SQLite opens the database file and runs ent-generated schema migrations.
3. The sync worker is scheduled. On a fresh database it immediately performs
   a full fetch of all 13 PeeringDB entity types from `api.peeringdb.com`.
4. Any rows left in a stale `running` state from a previous crash are
   transitioned to `failed` so `/ui/about` and `/readyz` don't report
   phantom in-flight syncs.
5. Once the first sync completes, `/readyz` flips to 200 and the service is
   fully usable.

A full sync against the public PeeringDB API typically takes **30–60 seconds**
on a reasonable connection. The peak working set stays under the default
400 MB memory guardrail (`PDBPLUS_SYNC_MEMORY_LIMIT`).

If you do not want the hourly background sync while experimenting, set
`PDBPLUS_SYNC_INTERVAL` to a large duration such as `24h`. The first sync
still runs immediately on startup.

## 3. Verify it's working

Open a second terminal while the server is still running.

### Liveness — does the process answer?

```bash
curl -s http://localhost:8080/healthz
# {"status":"ok"}
```

`/healthz` is a pure liveness probe. It returns 200 as soon as the HTTP
listener is up, even before the first sync.

### Readiness — is the database populated?

```bash
curl -sI http://localhost:8080/readyz
# HTTP/1.1 503 Service Unavailable   (while first sync is in flight)
# HTTP/1.1 200 OK                    (after first sync completes)
```

`/readyz` probes the SQLite connection and checks sync freshness against
`PDBPLUS_SYNC_STALE_THRESHOLD` (default `24h`). If it stays 503 for more
than a minute or two, check the server logs for sync errors (the most
common cause is network failure talking to `api.peeringdb.com`).

### PeeringDB-compatible API

Once `/readyz` returns 200, try the drop-in PeeringDB API:

```bash
# Fetch a specific network by PeeringDB numeric ID
curl -s http://localhost:8080/api/net/1 | head -c 500

# List the first few facilities
curl -s 'http://localhost:8080/api/fac?limit=3'
```

The response envelope and field names match
[api.peeringdb.com](https://api.peeringdb.com) exactly.

### Web UI

Open `http://localhost:8080/ui/` in a browser — or let the root path
redirect you:

```bash
# Browsers get an HTTP 302 redirect to /ui/
curl -sI http://localhost:8080/
```

The UI offers search across all entity types, detail pages, and an
ASN-comparison tool.

### Other surfaces (optional)

- **GraphQL playground**: `http://localhost:8080/graphql`
- **REST (entrest)**: `http://localhost:8080/rest/v1/` — OpenAPI spec at
  `/rest/v1/openapi.json`
- **ConnectRPC**: `http://localhost:8080/peeringdb.v1.*/` — gRPC reflection
  is enabled, so `grpcurl -plaintext localhost:8080 list` describes every
  service.

See the README's ConnectRPC section for worked `buf curl` examples.

## 4. Running in Docker (optional)

If you prefer a container:

```bash
docker build -t peeringdb-plus .
docker run -p 8080:8080 -v pdbdata:/data peeringdb-plus
```

The image stores the database at `/data/peeringdb-plus.db` (set by
`ENV PDBPLUS_DB_PATH=/data/peeringdb-plus.db` in the Dockerfile), so mount
a volume if you want data to persist across `docker run` invocations.

The container is built from Chainguard base images (`cgr.dev/chainguard/go`
for build, `cgr.dev/chainguard/glibc-dynamic` for runtime), runs as the
`nonroot` user, and exposes port 8080.

## Common setup issues

- **`go: go.mod requires go >= 1.26`** — Upgrade your Go toolchain. The
  project pins `go 1.26.1` in `go.mod`.
- **`/readyz` stays 503 forever** — Check the server log for the sync
  worker. The most common causes are: no outbound network to
  `api.peeringdb.com`, a corporate proxy rewriting TLS, or rate-limiting on
  the PeeringDB side. Setting `PDBPLUS_PEERINGDB_API_KEY` raises your rate
  limit if you have one.
- **`invalid PDBPLUS_PEERINGDB_URL` on startup** — The URL validator only
  accepts `https://`, or `http://` against loopback and RFC 1918 private
  ranges. Any other `http://` host is rejected. See
  [CONFIGURATION.md](CONFIGURATION.md) for the full rule.
- **Port 8080 already in use** — Set `PDBPLUS_PORT=9090` (or any free port)
  before launching, or use `PDBPLUS_LISTEN_ADDR=:9090`.
- **`no such table: ...` on first request** — The process crashed before
  migrations completed. Delete `peeringdb-plus.db` and restart; migrations
  run on every boot and are idempotent.
- **Sync OOM or aborts with memory guardrail message** — The Phase-A fetch
  exceeded `PDBPLUS_SYNC_MEMORY_LIMIT` (default `400MB`). Raise the limit
  or set it to `0` to disable the guardrail entirely.

## Next steps

- [ARCHITECTURE.md](ARCHITECTURE.md) — System overview, component diagram,
  data flow, and the key abstractions you'll touch when making changes.
- [CONFIGURATION.md](CONFIGURATION.md) — Full environment variable catalogue,
  including OpenTelemetry, LiteFS, and Fly.io-specific knobs not covered here.
- `cmd/peeringdb-plus/main.go` — The HTTP wiring, middleware chain, and
  graceful shutdown logic. Good entry point for understanding how requests
  flow through the binary.
- `ent/schema/` — The hand-edited entgo schemas that drive the entire API
  surface via code generation.
- `internal/sync/` — The PeeringDB sync worker, including full vs incremental
  modes and the memory guardrail.
