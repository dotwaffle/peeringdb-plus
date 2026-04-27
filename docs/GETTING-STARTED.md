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

If you change ent schemas, proto files, or templ templates, regenerate the
derived files first:

```bash
go generate ./...
```

`go generate` chains ent (entgql/entrest/entproto), `cmd/pdb-compat-allowlist`,
`buf generate`, `templ generate`, and `cmd/pdb-schema-generate` in the right
order. CI runs the same command and fails on any drift.

## 2. First run

```bash
./peeringdb-plus
```

With no configuration, the binary uses the defaults from
`internal/config/config.go`:

- Listens on `:8080` with h2c (HTTP/2 cleartext — required for ConnectRPC).
- Stores data in `./peeringdb-plus.db` in the current working directory.
- Syncs from `https://api.peeringdb.com` with a 1-hour interval (15 minutes if
  `PDBPLUS_PEERINGDB_API_KEY` is set — the authenticated rate-limit budget
  comfortably absorbs the 4× frequency).
- Sync mode defaults to `incremental` (since 2026-04-26 — see
  [CONFIGURATION.md](CONFIGURATION.md) for the full operator notes). Set
  `PDBPLUS_SYNC_MODE=full` for first-sync, recovery, or escape-hatch use.
- Assumes it is the LiteFS primary in local dev. The detection order is:
  (1) presence of `/litefs/.primary` → replica, (2) `/litefs/` directory
  exists but no `.primary` file → primary, (3) no `/litefs/` at all → fall
  back to `PDBPLUS_IS_PRIMARY` (defaults to `true`). Implemented in
  `internal/litefs/primary.go` `IsPrimaryWithFallback`. The lease semantics
  are inverted: `.primary` ABSENT means *this node IS the primary*.

You will see a `starting server` log line almost immediately. The HTTP
listener accepts connections right away, but **`/readyz` will return 503
until the first sync completes**.

### What happens on first start

1. The config loader validates every environment variable and aborts with a
   descriptive error if anything is wrong (fail-fast).
2. SQLite opens the database file and runs ent-generated schema migrations.
   Migrations run on the primary only, with `WithDropColumn(true)` and
   `WithDropIndex(true)` enabled for v1.15+ schema-hygiene drops.
3. The sync worker is scheduled. On a fresh database it immediately performs
   a sync pass against `api.peeringdb.com` covering all 13 PeeringDB entity
   types.
4. Any rows left in a stale `running` state from a previous crash are
   transitioned to `failed` so `/ui/about` and `/readyz` don't report
   phantom in-flight syncs.
5. Once the first sync completes, `/readyz` flips to 200 and the service is
   fully usable.

A full sync against the public PeeringDB API typically takes **30–60 seconds**
on a reasonable connection. The peak working set stays under the default
400 MB heap warning (`PDBPLUS_HEAP_WARN_MIB`) and the 400 MB sync memory
guardrail (`PDBPLUS_SYNC_MEMORY_LIMIT`).

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
[api.peeringdb.com](https://api.peeringdb.com). See
[API.md](API.md) for documented divergences.

### REST (entrest)

```bash
# OpenAPI spec
curl -s http://localhost:8080/rest/v1/openapi.json | head -c 200

# List a few networks via the entrest surface
curl -s 'http://localhost:8080/rest/v1/networks?limit=3'
```

### GraphQL

```bash
# Send a tiny query (the playground also lives at this URL via GET)
curl -s -X POST http://localhost:8080/graphql \
  -H 'Content-Type: application/json' \
  -d '{"query":"{ networks(first:3) { edges { node { id name asn } } } }"}'
```

Open `http://localhost:8080/graphql` in a browser to access the GraphiQL
playground.

### ConnectRPC / gRPC

```bash
# Reflection — list every registered service
grpcurl -plaintext localhost:8080 list

# Call a Get RPC against the Network service
grpcurl -plaintext -d '{"id":1}' localhost:8080 peeringdb.v1.NetworkService/Get
```

`buf curl` and `grpcui` work too — reflection is enabled and gRPC health
checks are wired to sync readiness.

### Web UI

Open `http://localhost:8080/ui/` in a browser — or let the root path
redirect you:

```bash
# Browsers (Accept: text/html) get an HTTP 302 redirect to /ui/
curl -sI http://localhost:8080/
```

The UI offers search across all entity types, detail pages, and an
ASN-comparison tool at `/ui/compare/{asn1}/{asn2}`.

> **curl gotcha** — `/ui/` does User-Agent / Accept content negotiation via
> `internal/web/termrender`. Plain CLI clients (curl, wget, HTTPie) get
> ANSI-styled terminal text, not HTML. If you want to inspect the HTML
> from the command line, masquerade as a browser:
>
> ```bash
> curl -sH 'User-Agent: Mozilla/5.0' http://localhost:8080/ui/ | head -c 500
> ```
>
> Or strip ANSI escape sequences from the terminal-mode output:
>
> ```bash
> curl -s http://localhost:8080/ui/ | sed 's/\x1b\[[0-9;]*[mGKH]//g' | head
> ```

## 4. Running in Docker (optional)

If you prefer a container, the dev image (single-process, no LiteFS) is the
easiest way to get going:

```bash
docker build -f Dockerfile -t peeringdb-plus .
docker run -p 8080:8080 -v pdbdata:/data peeringdb-plus
```

The image stores the database at `/data/peeringdb-plus.db` (set by
`ENV PDBPLUS_DB_PATH=/data/peeringdb-plus.db` in `Dockerfile`), so mount
a volume if you want data to persist across `docker run` invocations.

The container is built from Chainguard base images (`cgr.dev/chainguard/go`
for build, `cgr.dev/chainguard/glibc-dynamic` for runtime), runs as the
`nonroot` user, and exposes port 8080.

For the production image with LiteFS edge replication, see
`Dockerfile.prod` and [DEPLOYMENT.md](DEPLOYMENT.md). The prod image runs
`litefs mount` as its entrypoint and is intended for the Fly.io fleet.

## Common setup issues

- **`go: go.mod requires go >= 1.26`** — Upgrade your Go toolchain. The
  project pins `go 1.26.1` in `go.mod`.
- **`/readyz` stays 503 forever** — Check the server log for the sync
  worker. The most common causes are: no outbound network to
  `api.peeringdb.com`, a corporate proxy rewriting TLS, or rate-limiting on
  the PeeringDB side. Setting `PDBPLUS_PEERINGDB_API_KEY` raises your rate
  limit if you have one.
- **`PDBPLUS_PEERINGDB_URL uses http:// against a non-local host` on
  startup** — The URL validator only accepts `https://`, or `http://`
  against `localhost`/loopback IPs and RFC 1918 private ranges. Any other
  `http://` host is rejected. See [CONFIGURATION.md](CONFIGURATION.md) for
  the full rule.
- **Port 8080 already in use** — Set `PDBPLUS_PORT=9090` (or any free port)
  before launching, or use `PDBPLUS_LISTEN_ADDR=:9090`. `PDBPLUS_PORT` takes
  precedence over `PDBPLUS_LISTEN_ADDR` when both are set.
- **`no such table: ...` on first request** — The process crashed before
  migrations completed. Delete `peeringdb-plus.db` and restart; migrations
  run on every primary boot and are idempotent.
- **Sync aborts with `ErrSyncMemoryLimitExceeded`** — Phase A fetch peaked
  above `PDBPLUS_SYNC_MEMORY_LIMIT` (default `400MB`). Raise the limit or
  set it to `0` to disable the guardrail entirely. Operator-visible heap /
  RSS warnings are governed independently by `PDBPLUS_HEAP_WARN_MIB`
  (default `400`) and `PDBPLUS_RSS_WARN_MIB` (default `384`).
- **`/api/...` list returns 413** — The pre-flight count multiplied by the
  per-row byte estimate exceeded `PDBPLUS_RESPONSE_MEMORY_LIMIT` (default
  `128MB`). Narrow the filter, lower `limit`, or raise the budget. Bare
  numbers without a unit suffix are rejected; use `KB`/`MB`/`GB`/`TB`.
- **Curl-ing `/ui/` gets ANSI escape codes** — see the curl gotcha in step 3
  above. Pass `User-Agent: Mozilla/5.0` or strip ANSI with `sed`.

## Next steps

- [ARCHITECTURE.md](ARCHITECTURE.md) — System overview, component diagram,
  data flow, and the key abstractions you'll touch when making changes.
- [CONFIGURATION.md](CONFIGURATION.md) — Full environment variable catalogue,
  including OpenTelemetry, LiteFS, and Fly.io-specific knobs not covered here.
- [API.md](API.md) — Surface-by-surface contract notes, including documented
  divergences from upstream PeeringDB.
- `cmd/peeringdb-plus/main.go` — The HTTP wiring, middleware chain, and
  graceful shutdown logic. Good entry point for understanding how requests
  flow through the binary.
- `ent/schema/` — The hand-edited entgo schemas that drive the entire API
  surface via code generation.
- `internal/sync/` — The PeeringDB sync worker, including full vs incremental
  modes and the memory guardrail.
