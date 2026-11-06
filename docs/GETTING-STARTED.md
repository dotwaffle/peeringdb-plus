# Getting Started

This guide is for operators who want to run a mirror of upstream PeeringDB.
The first run downloads real upstream data.
For code changes and browser checks, use the [fixture-backed contributor setup](DEVELOPMENT.md#run-against-local-fixtures).

See [Configuration](CONFIGURATION.md) for environment settings and [Deployment](DEPLOYMENT.md) for production operations.

## Prerequisites

| Requirement | Version | Why |
|-------------|---------|-----|
| mise | `2026.10.22` or newer | Installs the exact project toolchain from `mise.lock` and runs contributor tasks. |
| Git | any recent | Cloning the repo. |
| Docker (optional) | any recent | Only if you want to run the container image instead of a local binary. |
| `grpcurl` (optional) | any recent | Only needed to poke the ConnectRPC/gRPC endpoints manually. |
| C compiler (gcc or clang) | any recent | Only for the race-detector tests in `mise run test` and `mise run check`. The server build does not use cgo. |
| A few hundred MB of disk | -- | The SQLite database sits around 90 MB after a full sync; keep headroom for growth and scratch space. |

Mise installs Go 1.27.3 and every contributor tool, including `buf`, `templ`, `gqlgen`, Tailwind, gotestsum, golangci-lint, and govulncheck.
The committed lockfile records exact tool versions and release checksums.

The server does not need cgo.
It uses `modernc.org/sqlite`, a pure-Go SQLite driver.
The Docker images build with `CGO_ENABLED=0`.
The race-detector tests (`mise run test` and `mise run check`) set `CGO_ENABLED=1` and need a C compiler.

## 1. Clone and build

```bash
git clone https://github.com/dotwaffle/peeringdb-plus.git
cd peeringdb-plus
mise trust
mise install --locked
export PDBPLUS_RUN_DIR="${TMPDIR:-/tmp}/peeringdb-plus/operator-run"
mkdir -p "$PDBPLUS_RUN_DIR"
mise exec -- go build -o "$PDBPLUS_RUN_DIR/peeringdb-plus" ./cmd/peeringdb-plus
```

The last command writes the binary outside the checkout.
The next step uses that binary.
`mise run build` compiles all packages, but it does not write a binary.

For code changes, use [Development](DEVELOPMENT.md) and the [pre-PR checklist](../CONTRIBUTING.md#pre-pr-checklist).

## 2. First run

```bash
"$PDBPLUS_RUN_DIR/peeringdb-plus"
```

With no configuration, the binary uses the defaults from `internal/config/config.go`:

- Listens on `:8080`.
  The listener accepts HTTP/1.1 and h2c (HTTP/2 without TLS). gRPC clients need h2c.
- Stores data in `./peeringdb-plus.db` in the current working directory.
- Syncs from `https://www.peeringdb.com` with a 1-hour interval (15 minutes if `PDBPLUS_PEERINGDB_API_KEY` is set -- the authenticated rate-limit budget comfortably absorbs the 4× frequency).
- Uses sync mode `incremental`.
  On an empty database, the first sync is always a full fetch.
  A full fetch also runs every `PDBPLUS_FULL_SYNC_INTERVAL` (default `24h`).
  Set `PDBPLUS_SYNC_MODE=full` only for recovery.
  See [CONFIGURATION.md § Sync Worker](CONFIGURATION.md#sync-worker).
- Runs as the LiteFS primary.
  When no `/litefs/` directory exists, `PDBPLUS_IS_PRIMARY` sets the role, and its default is `true`.
  See [CONFIGURATION.md § LiteFS / Primary Detection](CONFIGURATION.md#litefs--primary-detection).

You will see a `starting server` log line almost immediately.
The HTTP listener accepts connections right away, but **`/readyz` will return 503 until the first sync completes**.

### What happens on first start

1. The config loader (`internal/config`) checks the environment variables.
   If a value is not valid, the process stops with an error message.
2. On the primary, the process runs the ent schema migrations.
   These migrations can drop columns and indexes that the schema no longer defines.
3. On the primary, the process finds `sync_status` rows that a stopped process left in the `running` state, and changes them to `failed`.
   This stops `/ui/about` and `/readyz` from showing a sync in progress when no sync runs.
4. The sync scheduler starts in the background.
   On an empty database, it immediately runs a full sync of all 13 PeeringDB object types from `www.peeringdb.com`.
5. The HTTP listener starts while the first sync runs.
   Until the first sync completes, `/readyz` and the data routes return 503.
6. When the first sync completes, `/readyz` returns 200.

A full sync against the public PeeringDB API typically takes **30–60 seconds** on a reasonable connection.
The peak working set stays under the default 400 MB heap warning (`PDBPLUS_HEAP_WARN_MIB`) and the 400 MB sync memory guardrail (`PDBPLUS_SYNC_MEMORY_LIMIT`).

To reduce the frequency of an operator-managed sync, set `PDBPLUS_SYNC_INTERVAL` to a long duration such as `24h`.
On an empty database, the first sync still starts immediately.

## 3. Verify the service

Open a second terminal while the server is still running.

### Liveness -- does the process answer?

```bash
curl -s http://localhost:8080/healthz
# {"status":"ok"}
```

`/healthz` is a pure liveness probe.
It returns 200 as soon as the HTTP listener is up, even before the first sync.

### Readiness -- is the database populated?

```bash
curl -sI http://localhost:8080/readyz
# HTTP/1.1 503 Service Unavailable   (while first sync is in flight)
# HTTP/1.1 200 OK                    (after first sync completes)
```

`/readyz` probes the SQLite connection and checks sync freshness against `PDBPLUS_SYNC_STALE_THRESHOLD` (default `24h`).
If it stays 503 for more than a minute or two, check the server logs for sync errors (the most common cause is network failure talking to `www.peeringdb.com`).

### PeeringDB-compatible API

Once `/readyz` returns 200, try the drop-in PeeringDB API:

```bash
# Fetch a specific network by PeeringDB numeric ID
curl -s http://localhost:8080/api/net/1 | head -c 500

# List the first few facilities
curl -s 'http://localhost:8080/api/fac?limit=3'
```

The response envelope and field names match [www.peeringdb.com/api](https://www.peeringdb.com/api).
See [API.md](API.md) for documented divergences.

### REST (entrest)

```bash
# OpenAPI spec
curl -s http://localhost:8080/rest/v1/openapi.json | head -c 200

# List a few networks via the entrest surface
curl -s 'http://localhost:8080/rest/v1/networks?per_page=3'
```

### GraphQL

```bash
# Send a tiny query (the playground also lives at this URL via GET)
curl -s -X POST http://localhost:8080/graphql \
  -H 'Content-Type: application/json' \
  -d '{"query":"{ networks(first:3) { edges { node { id name asn } } } }"}'
```

Open `http://localhost:8080/graphql` in a browser to access the GraphiQL playground.

### ConnectRPC / gRPC

```bash
# Reflection -- list every registered service
grpcurl -plaintext localhost:8080 list

# Call a Get RPC against the Network service
grpcurl -plaintext -d '{"id":1}' localhost:8080 peeringdb.v1.NetworkService/GetNetwork
```

`buf curl` and `grpcui` work too -- reflection is enabled and gRPC health checks are wired to sync readiness.

### MCP and Agent Skill

The MCP endpoint supports MCP 2026-07-28 sessionless requests and legacy handshake clients:

```bash
# Inspect the curated agent index and MCP server card.
curl -fsS http://localhost:8080/llms.txt
curl -fsS http://localhost:8080/.well-known/mcp/server-card.json

# Fetch the origin-neutral skill document.
curl -fsS http://localhost:8080/.well-known/agent-skills/peeringdb-plus/SKILL.md

# Download the installable skill archive.
curl -fLO http://localhost:8080/skills/peeringdb-plus.zip
```

The discovery documents and ZIP metadata use the request origin.
Therefore, a self-hosted copy does not contact another deployment.
Set `PDBPLUS_PUBLIC_URL` only when a reverse proxy does not preserve the requested hostname.

An MCP client can connect directly to `http://localhost:8080/mcp`.
The server offers read-only search, detail, network comparison, IP lookup, and sync-freshness tools plus resources and prompts.

### Web UI

Open `http://localhost:8080/ui/` in a browser.
A browser that opens `http://localhost:8080/` gets a redirect to `/ui/`:

```bash
# A browser User-Agent with Accept: text/html gets an HTTP 302 redirect to /ui/.
curl -s -o /dev/null -w '%{http_code} %{redirect_url}\n' \
  -H 'User-Agent: Mozilla/5.0' -H 'Accept: text/html' http://localhost:8080/
# 302 http://localhost:8080/ui/
```

The UI searches networks, exchanges, facilities, organizations, campuses, and carriers.
It also has detail pages and an ASN-comparison tool at `/ui/compare/{asn1}/{asn2}`.

> **Terminal output from `/ui/`.**
> The server sends ANSI-colored text to curl, wget, and HTTPie.
> Add `?format=plain` to get plain text, or `?nocolor` to remove the color codes.
> To get HTML, send a browser User-Agent:
>
> ```bash
> curl -sH 'User-Agent: Mozilla/5.0' http://localhost:8080/ui/ | head -c 500
> ```
>
> For all options, see [API.md § The curl gotcha](API.md#the-curl-gotcha).

## 4. Running in Docker (optional)

If you prefer a container, the standalone image (single-process, no LiteFS) is the easiest way to get going.
CI publishes it for `linux/amd64` and `linux/arm64`:

```bash
docker run -p 8080:8080 -v pdbdata:/data ghcr.io/dotwaffle/peeringdb-plus:latest
```

`latest` follows the release tags, and `main` follows the `main` branch.
For all tags and for how to verify an image, see [DEPLOYMENT.md § Published image](DEPLOYMENT.md#published-image).

To build the image from your checkout:

```bash
docker build -f Dockerfile -t peeringdb-plus .
docker run -p 8080:8080 -v pdbdata:/data peeringdb-plus
```

The image stores the database at `/data/peeringdb-plus.db` (set by `ENV PDBPLUS_DB_PATH=/data/peeringdb-plus.db` in `Dockerfile`), so mount a volume if you want data to persist across `docker run` invocations.

The container is built from Chainguard base images (`cgr.dev/chainguard/go` for build, `cgr.dev/chainguard/static` for runtime), runs as the `nonroot` user, and exposes port 8080.

For the production image with LiteFS edge replication, see `Dockerfile.litefs` and [DEPLOYMENT.md](DEPLOYMENT.md).
The prod image runs `litefs mount` as its entrypoint and is intended for the Fly.io fleet.

## Common setup issues

- **`go: go.mod requires go >= 1.27.3`** -- Run `mise install --locked`.
  `go.mod` pins Go 1.27.3; `mise.toml` tracks the 1.27 line and `mise.lock` holds the exact patch.
- **`/readyz` stays 503.**
  Look for `sync failed, retrying` and `sync cycle failed` in the server log.
  A failed sync tries again after 30 seconds, 2 minutes, and 8 minutes.
  A rate limit (HTTP 429) or a WAF block stops these retries.
  When the retries end, the next attempt waits one `PDBPLUS_SYNC_INTERVAL` (default `1h`, or `15m` with an API key).
  To try again at once, correct the cause and restart the process.
  Do not repeatedly restart after an upstream throttle.
  Wait for its retry window.
  Common causes are no outbound access to `www.peeringdb.com`, a proxy that intercepts TLS, and upstream rate limits.
  With `PDBPLUS_PEERINGDB_API_KEY`, the upstream rate limit is higher.
- **`PDBPLUS_PEERINGDB_URL uses http:// against a non-local host` on startup** -- The URL validator only accepts `https://`, or `http://` against `localhost`/loopback IPs and RFC 1918 private ranges.
  Any other `http://` host is rejected.
  See [CONFIGURATION.md](CONFIGURATION.md) for the full rule.
- **Port 8080 already in use** -- Set `PDBPLUS_PORT=9090` (or any free port) before launching, or use `PDBPLUS_LISTEN_ADDR=:9090`.
  `PDBPLUS_PORT` takes precedence over `PDBPLUS_LISTEN_ADDR` when both are set.
- **`failed to seed initial object counts` with `no such table` at startup.**
  The process started as a replica with an empty database.
  A replica does not create the schema.
  For a standalone operator instance, unset `PDBPLUS_IS_PRIMARY` or set it to `true`.
  Make sure that no `/litefs/.primary` file exists.
  Then start the process again.
- **`sync aborted: memory limit exceeded` in the log.**
  After the fetch phase, the Go heap was above `PDBPLUS_SYNC_MEMORY_LIMIT` (default `400MB`).
  Increase the limit, or set it to `0` to turn off the check.
  `PDBPLUS_HEAP_WARN_MIB` (default `400`) and `PDBPLUS_RSS_WARN_MIB` (default `384`) control the heap and RSS warnings.
  These warnings do not stop a sync.
- **`/api/...` list returns 413** -- The pre-flight count multiplied by the per-row byte estimate exceeded `PDBPLUS_RESPONSE_MEMORY_LIMIT` (default `128MB`).
  Narrow the filter, lower `limit`, or raise the budget.
  Bare numbers without a unit suffix are rejected; use `KB`/`MB`/`GB`/`TB`.
- **Curl-ing `/ui/` gets ANSI escape codes.**
  See the terminal-output note in step 3.
  Add `?format=plain`, or send a browser User-Agent.

## Next steps

- [ARCHITECTURE.md](ARCHITECTURE.md): system overview, component diagram, data flow, and the key abstractions you'll touch when making changes.
- [CONFIGURATION.md](CONFIGURATION.md): the full list of environment variables, including OpenTelemetry, LiteFS, and Fly.io-specific knobs not covered here.
- [API.md](API.md): surface-by-surface contract notes, including documented divergences from upstream PeeringDB.
- `cmd/peeringdb-plus/main.go`: the HTTP wiring, middleware chain, and graceful shutdown logic.
  Good entry point for understanding how requests flow through the binary.
- `schema/peeringdb.json` and `ent/schema/`: the curated PeeringDB schema and the entgo schemas generated from it.
  Hand-edited methods are in sibling files, for example `ent/schema/poc_policy.go`.
  Code generation builds the ent client and the GraphQL and REST layers from these schemas.
- `internal/sync/`: the PeeringDB sync worker, including full vs incremental modes and the memory guardrail.
