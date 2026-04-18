<!-- generated-by: gsd-doc-writer -->
# Deployment

PeeringDB Plus is designed to be deployed to [Fly.io](https://fly.io/) with
[LiteFS](https://fly.io/docs/litefs/) providing edge-local SQLite replication.
The production topology is a small fleet of Fly machines in one or more
regions, with a single Consul-elected LiteFS primary handling the sync worker's
writes and all other machines serving read-only traffic from a local FUSE
mount.

## Deployment targets

| Target | Config file | Notes |
| --- | --- | --- |
| Fly.io (production) | `fly.toml`, `Dockerfile.prod`, `litefs.yml` | App name `peeringdb-plus`, primary region `lhr`. |
| Generic Docker host | `Dockerfile` | Development image; runs the binary directly without LiteFS. |

- `fly.toml` — app (`peeringdb-plus`), primary region (`lhr`), rolling deploy
  strategy with `max_unavailable = 0.5`, Consul enabled for LiteFS leases,
  512 MB `shared-cpu-2x` machines, a 1 GB auto-extending `litefs_data`
  volume mounted at `/var/lib/litefs`, and an HTTP health check on
  `GET /healthz` every 15s.
- `Dockerfile.prod` — LiteFS-aware production image. Chainguard
  `glibc-dynamic` runtime with `fuse3` installed, copies the LiteFS 0.5
  binary from `flyio/litefs:0.5`, copies `litefs.yml` to `/etc/litefs.yml`,
  creates the `/litefs` mount point, and sets `ENTRYPOINT ["litefs", "mount"]`.
  The application binary is built with `CGO_ENABLED` unset (pure Go via
  `modernc.org/sqlite`) and `-trimpath -ldflags="-s -w"`.
- `Dockerfile` — development image. Chainguard `glibc-dynamic` runtime, same
  build flags but with `CGO_ENABLED=0` explicitly set. No LiteFS. Runs the
  binary directly as `ENTRYPOINT ["/usr/local/bin/peeringdb-plus"]` with
  `PDBPLUS_DB_PATH=/data/peeringdb-plus.db` and `EXPOSE 8080`. Used by
  GitHub Actions for the `Docker Build` CI job and as a base for local
  container-based development.

Both images use `cgr.dev/chainguard/go` as the build stage and
`cgr.dev/chainguard/glibc-dynamic:latest-dev` as the runtime stage.

## Build pipeline

GitHub Actions workflow `.github/workflows/ci.yml` runs on every pull request
and on pushes to `main`. It comprises five jobs:

1. **Lint** — `golangci-lint` plus a generated-code drift check that runs
   `go generate ./...` and fails if `ent/`, `gen/`, `graph/`, or
   `internal/web/templates/` differ from committed files.
2. **Test** — `CGO_ENABLED=1 go test -race -coverprofile=coverage.out` with
   coverage excluding `ent/` and `gen/`. Posts a coverage comment via
   `k1LoW/octocov-action`.
3. **Build** — `go build ./...` to confirm compilation.
4. **Govulncheck** — installs and runs `govulncheck ./...`.
5. **Docker Build** — builds both `Dockerfile` and `Dockerfile.prod` using
   `docker/build-push-action@v7` with GitHub Actions cache. Images are built
   but **not pushed** from CI.

There is no automated deploy step. Deployment to Fly.io is a manual action
run from a developer workstation (per the `D-22` decision recorded in
`fly.toml`).

## Environment setup

Runtime configuration is supplied by environment variables. See
[CONFIGURATION.md](CONFIGURATION.md) for the full list. For a Fly.io
deployment the following must be set via `fly secrets set` (or
`fly secrets import`):

| Secret | Purpose |
| --- | --- |
| `PDBPLUS_PEERINGDB_API_KEY` | Optional PeeringDB API key. Empty = unauthenticated. |
| `PDBPLUS_SYNC_TOKEN` | Shared secret required to call `POST /sync`. Empty = on-demand sync disabled. |

Example:

```bash
fly secrets set PDBPLUS_PEERINGDB_API_KEY=... PDBPLUS_SYNC_TOKEN=...
```

### Authenticated PeeringDB Sync (Recommended)

Running the sync with a PeeringDB API key is the recommended production
configuration. The sync worker then fetches both `Public`- and `Users`-tier
rows from PeeringDB; anonymous API callers still see `Public`-only thanks to
the [privacy policy](./ARCHITECTURE.md#privacy-layer) on the read path.

1. **Obtain a PeeringDB API key.** Sign in at
   <https://www.peeringdb.com/profile> and generate a key under the
   "API Keys" tab. The key is a long-lived bearer token scoped to your
   PeeringDB user.
2. **Set the key as a Fly secret.** The secret is applied to the app and
   triggers a rolling deploy so the sync worker picks it up on the next
   machine start:

   ```bash
   fly secrets set PDBPLUS_PEERINGDB_API_KEY=<key> --app peeringdb-plus
   ```

3. **Confirm rollout.** Tail the logs for the classification line emitted at
   startup. With the key set, it reports `auth=authenticated`:

   ```bash
   fly logs --app peeringdb-plus | grep -m1 sync.classification
   ```

   `fly secrets list --app peeringdb-plus` should also show
   `PDBPLUS_PEERINGDB_API_KEY` in the output (value is masked, only the
   digest is visible — this is expected).
4. **Operational implication.** `Users`-tier rows are now present in the
   local SQLite database. They are filtered out of anonymous HTTP responses
   by the ent privacy policy on every read path (see
   [ARCHITECTURE.md](./ARCHITECTURE.md#privacy-layer)). No response format
   or schema change is visible to anonymous callers; the mirror's anonymous
   API shape continues to match upstream's.

### Private/Internal Deployments

Deployments that are not reachable from the public internet (internal tools,
CI sidecars, pre-production mirrors) can elevate anonymous callers to
Users-tier so the privacy filter becomes a no-op:

```bash
fly secrets set PDBPLUS_PUBLIC_TIER=users --app peeringdb-plus
```

Startup then emits a WARN log (`privacy.override.active`) naming the override
every time the app boots. This is intentional — the elevated default must
never be silent. Use this only for deployments you would not want indexed by
a search engine. See
[CONFIGURATION.md §Privacy & Tiers](./CONFIGURATION.md#privacy--tiers) for
the full state matrix.

Non-secret configuration lives in `fly.toml`'s `[env]` block:

- `PDBPLUS_LISTEN_ADDR=:8080` — the app listens on 8080 directly (the
  LiteFS proxy is intentionally not used; see [LiteFS](#litefs) below).
- `PDBPLUS_DB_PATH=/litefs/peeringdb-plus.db` — database file inside the
  FUSE-mounted LiteFS directory.
- `PRIMARY_REGION=lhr` — consumed by both `litefs.yml` for lease candidacy
  and the `POST /sync` handler for `fly-replay` forwarding.

Fly.io injects `FLY_REGION`, `FLY_APP_NAME`, `FLY_CONSUL_URL`, and
`HOSTNAME` automatically. Consul must be attached to the app once via
`fly consul attach` so that `FLY_CONSUL_URL` is populated for LiteFS lease
election. <!-- VERIFY: fly consul attach must be run once per app to populate FLY_CONSUL_URL; this is a manual out-of-band step not captured in fly.toml -->

Standard `OTEL_*` environment variables apply via the
`go.opentelemetry.io/contrib/exporters/autoexport` package used in
`internal/otel/provider.go`. See [Monitoring](#monitoring) below.

## LiteFS

LiteFS is in maintenance mode — stable but no longer actively supported by
Fly.io. There is no drop-in alternative for edge SQLite replication, so the
project continues to use it.

- **FUSE mount.** `Dockerfile.prod`'s entrypoint is `litefs mount`, which
  starts the LiteFS FUSE process, mounts the database directory at
  `/litefs`, and then execs the application (see `litefs.yml` `exec:`
  stanza, which invokes `/usr/local/bin/peeringdb-plus`).
- **The app does NOT link to LiteFS.** It reads and writes plain SQLite
  files under `/litefs/`; LiteFS intercepts the filesystem operations and
  replicates them out-of-band.
- **Lease election via Consul.** `litefs.yml` sets `lease.type: "consul"`
  and uses `${FLY_CONSUL_URL}` as the backend. Only machines in
  `PRIMARY_REGION` are lease candidates (`candidate: ${FLY_REGION == PRIMARY_REGION}`).
- **Replication state volume.** A 1 GB `litefs_data` volume is mounted at
  `/var/lib/litefs` per `fly.toml`'s `[mounts]` block, auto-extending up to
  10 GB when 80 percent full.
- **Direct HTTP on :8080 with h2c.** The LiteFS proxy is intentionally not
  used; the app serves traffic directly on port 8080 with HTTP/2 cleartext
  so that gRPC/ConnectRPC requests work end-to-end (the LiteFS proxy does
  not support HTTP/2 for gRPC). `fly.toml` enables
  `[http_service.http_options] h2_backend = true` so the Fly edge talks
  h2c to the backend.
- **Inverted `.primary` file semantics.** The file at `/litefs/.primary`
  is **present on replicas** (containing the primary's hostname) and
  **absent on the primary** (the primary holds the lease). The detection
  logic in `internal/litefs/primary.go` is:
  1. If `/litefs/.primary` exists → replica.
  2. If `/litefs/` exists but `.primary` does not → primary.
  3. If `/litefs/` does not exist (LiteFS not mounted) → fall back to the
     `PDBPLUS_IS_PRIMARY` env var (default `true` for local dev).
- **Write forwarding via `fly-replay`.** When `POST /sync` lands on a
  replica, the handler returns HTTP 307 with a
  `fly-replay: region=${PRIMARY_REGION}` header so the Fly edge re-routes
  the request to the primary region.

### Rolling deploy behaviour

During a rolling deploy the LiteFS FUSE mount takes a brief moment to
come up on each new machine, and Fly's proxy may log "not listening"
warnings while the machine is between `litefs mount` starting and the app
binary binding to `:8080`. This is expected and self-clears once the mount
completes. The `grace_period = "30s"` on the `/healthz` check in `fly.toml`
is sized to accommodate this.

`fly.toml` sets `strategy = "rolling"` with `max_unavailable = 0.5`, which
replaces roughly half the fleet at a time (approximately four machines out
of nine in the tuned production topology). Blue-green deploys are not
usable here because running two parallel fleets would conflict with the
LiteFS + Consul primary election.

## Asymmetric fleet

As of v1.15 (Phase 65), the fleet is split into two Fly process groups
with different VM sizing and mount policies:

- **`primary` group** — 1 machine in `lhr`, `shared-cpu-2x` / 512 MB,
  persistent `litefs_data` volume mounted at `/var/lib/litefs`. Runs the
  sync worker, holds the LiteFS Consul lease, source of LiteFS HTTP
  replication.
- **`replica` group** — 7 machines (iad, nrt, syd, lax, jnb, sin, gru),
  `shared-cpu-1x` / 256 MB, **no persistent volume** (ephemeral rootfs).
  On boot, LiteFS cold-syncs the 88 MB database from the primary via
  HTTP. Typical hydration window is 5-45 seconds per region; `/readyz`
  returns 503 during this window so Fly Proxy routes around the machine
  until it is ready.

**Volume-only-on-primary contract:** `[[mounts]]` in `fly.toml` is
scoped to `processes = ["primary"]`. Only the LHR primary machine has a
mount. Replica machines are cattle — a damaged replica is recovered by
`fly machine destroy --force <id>`; the replacement machine that Fly
schedules has no volume concern, cold-syncs from the primary, and
becomes live when `/readyz` flips to 200.

**Replica cold-sync expectations:**

| Region | Expected hydration | Notes |
|--------|--------------------|-------|
| iad, lax | 5-15s | Low-latency path to LHR |
| nrt, sin | 15-30s | Transpacific |
| syd, gru, jnb | 30-45s | Furthest edges; long-haul to LHR |

If a replica stays on 503 >5 minutes with logs showing successful DB
pings, the `sync_status` row (replicated from the primary via LiteFS
cold-sync) is likely stale. Remediation: `POST /sync` with the
`PDBPLUS_SYNC_TOKEN` to force a fresh primary sync; replicas pick up
the updated `sync_status` within seconds via LTX replication.

**Sizing rationale:** Observed replica RSS is 58-59 MB steady-state;
`shared-cpu-1x` / 256 MB gives ~4× memory headroom and budget for
LiteFS LTX replay spikes. The primary keeps `shared-cpu-2x` / 512 MB —
it runs the sync worker whose memory profile was characterised in v1.13
and v1.14.

**Cost:** Asymmetric fleet is ~$20.75/mo vs the previous uniform
~$57.20/mo — saves ~$36/mo. Real win is operational simplicity (no
replica-volume orphans, destroy-and-recreate recovery in seconds).

## Regional rollout

The primary region is `lhr`; every additional region hosts read-only
replicas. To add a region:

```bash
fly regions add <region>
fly scale count <n> --region <region>
```

To scale the total fleet size (for example, to grow the `lhr` fleet):

```bash
fly scale count <n>                 # total machines across all regions
fly scale count <n> --region lhr    # regional scale
fly scale vm shared-cpu-2x --memory 512   # resize (matches fly.toml defaults)
```

Only machines in `PRIMARY_REGION=lhr` are eligible to hold the LiteFS
write lease, so scaling the primary region higher than one replica
increases primary-election redundancy but does not add write capacity
(LiteFS is single-writer).

## Monitoring

Observability uses OpenTelemetry end-to-end. The SDK is initialized in
`internal/otel/provider.go` and picks up exporters via
`autoexport.NewSpanExporter`, `autoexport.NewMetricReader`, and
`autoexport.NewLogExporter`. Signals are selected by the standard
environment variables documented at
<https://opentelemetry.io/docs/languages/sdk-configuration/>:

- `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_OTLP_HEADERS`,
  `OTEL_EXPORTER_OTLP_PROTOCOL` for OTLP export.
- `OTEL_TRACES_EXPORTER`, `OTEL_METRICS_EXPORTER`, `OTEL_LOGS_EXPORTER`
  (set to `none` to disable a signal per D-04).
- `OTEL_EXPORTER_PROMETHEUS_HOST` / `OTEL_EXPORTER_PROMETHEUS_PORT` for
  scrape-based metrics.
- `PDBPLUS_OTEL_SAMPLE_RATE` (app-specific) for the trace sampling ratio.

A Grafana dashboard is provided in `deploy/grafana/dashboards/pdbplus-overview.json`
with a provisioning manifest at `deploy/grafana/provisioning/dashboards.yaml`
for self-hosted Grafana instances.

The specific OTLP collector, metrics backend, and dashboard host used in
production are deployment-specific and must be configured via Fly secrets
(`fly secrets set OTEL_EXPORTER_OTLP_ENDPOINT=... OTEL_EXPORTER_OTLP_HEADERS=...`).
<!-- VERIFY: production OTLP endpoint / collector target (Honeycomb, Grafana Cloud, self-hosted, etc.) is not encoded in the repository -->
<!-- VERIFY: Grafana dashboard host URL is not encoded in the repository -->

Fly.io's built-in machine metrics (CPU, memory, network, disk) are
available through the Fly dashboard without additional configuration.

Runtime health:

- `GET /healthz` — liveness probe, used by `fly.toml`'s HTTP service check.
- `GET /readyz` — readiness probe that turns unready during graceful
  shutdown drain (`PDBPLUS_DRAIN_TIMEOUT`, default `10s`).

## Rollback

Fly.io tracks every deployed image as a release. To roll back:

```bash
fly releases                            # list recent releases
fly releases rollback <version>         # revert to a specific release
```

Alternatively, redeploy the previous Git commit explicitly:

```bash
git checkout <previous-sha>
fly deploy
```

Because deploys are manual and built from the local working tree,
`fly releases rollback` is the fastest path to revert without a rebuild.

## Deploy command summary

```bash
# Deploy the current working tree
fly deploy

# Deploy with a specific Dockerfile (defaults to Dockerfile.prod per fly.toml)
fly deploy --dockerfile Dockerfile.prod

# Check status after deploy
fly status
fly logs
fly machines list
```

Initial setup (one-time, per app):

```bash
fly apps create peeringdb-plus
fly consul attach                                          # populates FLY_CONSUL_URL
fly volumes create litefs_data --size 1 --region lhr       # one volume per machine
fly secrets set PDBPLUS_PEERINGDB_API_KEY=... PDBPLUS_SYNC_TOKEN=...
fly deploy
```

<!-- VERIFY: exact initial-setup sequence for a fresh Fly app including Consul attach ordering is not captured in the repository and must be confirmed against current Fly.io documentation -->
