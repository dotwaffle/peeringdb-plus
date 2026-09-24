# Deployment

PeeringDB Plus is designed to be deployed to [Fly.io](https://fly.io/) with
[LiteFS](https://fly.io/docs/litefs/) providing edge-local SQLite replication.
The production topology is an [asymmetric fleet](#asymmetric-fleet):
one Consul-elected LiteFS primary in `lhr`
(persistent volume, runs the sync worker)
and a set of ephemeral, read-only replicas in other regions
that cold-sync from the primary on boot.

## Deployment targets

| Target | Config file | Notes |
| --- | --- | --- |
| Fly.io (production) | `fly.toml`, `Dockerfile.prod`, `litefs.yml` | App name `peeringdb-plus`, primary region `lhr`. |
| Generic Docker host | `Dockerfile` | Development image; runs the binary directly without LiteFS. |

- `fly.toml` — app (`peeringdb-plus`), primary region (`lhr`), rolling deploy
  strategy with `max_unavailable = 0.5`, Consul enabled for LiteFS leases,
  two `[processes]` groups (`primary`, `replica`) with separate `[[vm]]`
  blocks (`shared-cpu-2x` / 512 MB for `primary`, `shared-cpu-1x` / 256 MB
  for `replica`), a 1 GB auto-extending `litefs_data` volume mounted at
  `/var/lib/litefs` **scoped to `processes = ["primary"]`** (replicas have
  no volume), and an HTTP health check on `GET /readyz` every 15s (the
  readiness probe, so Fly Proxy routes around hydrating or stale machines;
  `GET /healthz` remains the always-200 liveness probe).
- `Dockerfile.prod` — LiteFS-aware production image.
  Chainguard `glibc-dynamic` runtime with `fuse3` and `sqlite`
  (CLI for incident response —
  see [Incident-response debug shell](#incident-response-debug-shell)) installed,
  copies the LiteFS 0.5 binary from `flyio/litefs:0.5`,
  copies `litefs.yml` to `/etc/litefs.yml`, creates the `/litefs` mount point,
  and sets `ENTRYPOINT ["litefs", "mount"]`.
  The application binary is built with `CGO_ENABLED=0`
  (pure Go via `modernc.org/sqlite`)
  and `-trimpath -ldflags="-s -w …"`
  (the version string is injected via
  `-X github.com/dotwaffle/peeringdb-plus/internal/buildinfo.injected=$VERSION`).
- `Dockerfile` — development image.
  Chainguard `glibc-dynamic` runtime,
  `CGO_ENABLED=0` with `-trimpath -ldflags="-s -w"`.
  Unlike the prod image it does **not** inject the build version
  (no `-X …buildinfo.injected=$VERSION`),
  so `internal/buildinfo` reports its default.
  No LiteFS.
  Runs the binary directly
  as `ENTRYPOINT ["/usr/local/bin/peeringdb-plus"]` with
  `PDBPLUS_DB_PATH=/data/peeringdb-plus.db` and `EXPOSE 8080`.
  Used by GitHub Actions for the `Docker Build` CI job and as a base
  for local container-based development.

Both images use `cgr.dev/chainguard/go` as the build stage.
`Dockerfile.prod` uses `cgr.dev/chainguard/glibc-dynamic:latest-dev`
as the runtime stage and runs as root.
`Dockerfile` uses `cgr.dev/chainguard/glibc-dynamic`, which has no shell,
and runs as `nonroot`.

## Build pipeline

GitHub Actions workflow `.github/workflows/ci.yml` runs on every pull request
and on pushes to `main`.
It comprises two jobs:

1. **`ci`** — a single mise/Go job that installs the committed lockfile,
   warms one module/build cache, then runs these steps in order:
   1. **Generated-code drift check** —
      `mise run generate` then
      `git diff --exit-code` over
      `ent/`, `gen/`, `graph/`, `internal/web/templates/`,
      `internal/web/static/tailwind.css`,
      and `internal/pdbcompat/allowlist_gen.go`
      (the security-load-bearing `/api` traversal allowlist).
      The step also fails when generation creates untracked files
      in these paths.
      Runs first so a forgotten regeneration fails in seconds,
      ahead of the expensive build and test steps.
      A `go mod tidy` gate follows it,
      failing the job when go.mod/go.sum are untidy.
   2. **Build** — `mise run build` to confirm compilation and warm the cache.
   3. **Test** — `mise run coverage` uses gotestsum and the race detector, with
      coverage excluding `ent/` and `gen/`; posts a coverage comment via
      `k1LoW/octocov-action`.
   4. **Lint** — `mise run lint` checks the workflow and Go sources.
   5. **Govulncheck** — `mise run vulncheck`.
      Advisory (`continue-on-error`):
      a flagged vulnerability surfaces as a workflow warning
      but does **not** block the merge.
2. **`docker-build`** — a separate parallel job that builds both `Dockerfile`
   and `Dockerfile.prod` using `docker/build-push-action@v7` with BuildKit's
   `type=gha` cache.
   Images are built but **not pushed** from CI.

`docker-build` is a separate job
because its BuildKit `type=gha` cache is separate from the Go build cache.

There is no automated deploy step.
Deployment to Fly.io is a manual action run from a developer workstation,
as documented in `fly.toml`.

### Production image: accepted risks

Two deliberate trade-offs in `Dockerfile.prod`,
recorded here so they read as decisions rather than oversights:

- **The process tree runs as root.**
  The LiteFS FUSE mount genuinely requires root,
  and the application process — which parses untrusted internet input
  on six API surfaces — currently inherits it
  (LiteFS `exec:`s the app without dropping privileges).
  Accepted for now because the container boundary
  (Fly microVM per machine) is the primary isolation layer.
  Revisit if LiteFS grows a privilege-drop option
  or the app moves off FUSE.
- **The runtime base is `glibc-dynamic:latest-dev`.**
  The `-dev` variant has a shell and `apk`,
  and `Dockerfile.prod` installs the `sqlite3` CLI with `apk`.
  This is deliberate incident-response tooling:
  `fly ssh console` + `sqlite3 /litefs/peeringdb-plus.db`
  is the documented production debugging path.
  The dev `Dockerfile` uses the plain (shell-less) variant,
  since nothing execs into it.
  Both base tags float (`:latest`);
  Chainguard's free tier offers no pinned tags,
  and digest-pinning without bump automation
  (deliberately not installed)
  would only freeze staleness in place.

## Environment setup

Runtime configuration is supplied by environment variables.
See [CONFIGURATION.md](CONFIGURATION.md) for the full list.
Set these secrets with `fly secrets set` (or `fly secrets import`):

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
configuration.
The sync worker then fetches both `Public`-
and `Users`-tier rows from PeeringDB;
anonymous API callers still see `Public`-only thanks to the
[privacy policy](./ARCHITECTURE.md#privacy-layer) on the read path.

1. **Obtain a PeeringDB API key.**
   Sign in at <https://www.peeringdb.com/profile>
   and generate a key under the "API Keys" tab.
   The key is a long-lived bearer token scoped to your PeeringDB user.
2. **Set the key as a Fly secret.**
   The secret is applied to the app and triggers a rolling deploy
   so the sync worker picks it up on the next machine start:

   ```bash
   fly secrets set PDBPLUS_PEERINGDB_API_KEY=<key> --app peeringdb-plus
   ```

3. **Confirm rollout.**
   Look for the startup classification line.
   With the key set, it contains `"auth":"authenticated"`:

   ```bash
   fly logs --app peeringdb-plus --no-tail | grep '"msg":"sync mode"'
   ```

   The "Sync mode" row on the `/ui/about` page also shows the
   authentication mode.

   `fly secrets list --app peeringdb-plus` should also show
   `PDBPLUS_PEERINGDB_API_KEY` in the output (value is masked, only the digest
   is visible — this is expected).
4. **Operational implication.**
   `Users`-tier rows are now present in the local SQLite database.
   If the key belongs to a member of an organization,
   that organization's `Private` contacts are also present.
   The ent privacy policy removes both from anonymous HTTP responses
   on every read path (see [ARCHITECTURE.md](./ARCHITECTURE.md#privacy-layer)).
   No response format or schema change is visible to anonymous callers;
   the mirror's anonymous API shape continues to match upstream's.

### Private/Internal Deployments

Deployments that are not reachable from the public internet
(internal tools, CI sidecars, pre-production mirrors)
can elevate anonymous callers to Users-tier,
so they also see `Users` rows.
`Private` rows stay hidden in every tier,
because upstream shows them only to members of the owning organization:

```bash
fly secrets set PDBPLUS_PUBLIC_TIER=users --app peeringdb-plus
```

Startup then emits a WARN log
(`public tier override active`) naming the override every time the app boots.
This is intentional — the elevated default must never be silent.
Use this only for deployments you would not want indexed by a search engine.
See [CONFIGURATION.md §Privacy & Tiers](./CONFIGURATION.md#privacy--tiers)
for the full state matrix.

Non-secret configuration lives in `fly.toml`'s `[env]` block:

- `PDBPLUS_LISTEN_ADDR=:8080` — the app listens on 8080 directly (the
  LiteFS proxy is intentionally not used; see [LiteFS](#litefs) below).
- `PDBPLUS_DB_PATH=/litefs/peeringdb-plus.db` — database file inside the
  FUSE-mounted LiteFS directory.
- `PRIMARY_REGION=lhr` — consumed by both `litefs.yml` for lease candidacy
  and the `POST /sync` handler for `fly-replay` forwarding.

Fly.io injects `FLY_REGION`, `FLY_APP_NAME`, and `HOSTNAME` automatically.
`fly consul attach` sets `FLY_CONSUL_URL` as an app secret.
Run it once for each app.
LiteFS uses this URL for lease election.

Standard `OTEL_*` environment variables apply via the
`go.opentelemetry.io/contrib/exporters/autoexport` package used in
`internal/otel/provider.go`.
See [Monitoring](#monitoring) below.

The default sync mode is `incremental`.
A primary with no successful sync recorded runs a full sync at startup,
and `PDBPLUS_FULL_SYNC_INTERVAL` (default `24h`) forces a periodic full cycle.
For one full sync, send `POST /sync?mode=full`
(see [Force a full sync](#5-force-a-full-sync)).
Set `PDBPLUS_SYNC_MODE=full` only when every cycle must fetch all data.

## LiteFS

LiteFS is in maintenance mode —
stable but no longer actively supported by Fly.io.
There is no drop-in alternative for edge SQLite replication,
so the project continues to use it.

- **FUSE mount.** `Dockerfile.prod`'s entrypoint is `litefs mount`, which
  starts the LiteFS FUSE process, mounts the database directory at
  `/litefs`, and then execs the application (see `litefs.yml` `exec:`
  stanza, which invokes `/usr/local/bin/peeringdb-plus`).
- **The app does NOT link to LiteFS.**
  It reads and writes plain SQLite files under `/litefs/`;
  LiteFS intercepts the filesystem operations and replicates them out-of-band.
- **Lease election via Consul.**
  `litefs.yml` sets `lease.type: "consul"` and uses `${FLY_CONSUL_URL}`
  as the backend.
  Only machines in `PRIMARY_REGION` are lease candidates
  (`candidate: ${FLY_REGION == PRIMARY_REGION}`).
- **Replication state volume.**
  A 1 GB `litefs_data` volume is mounted at `/var/lib/litefs` per `fly.toml`'s
  `[[mounts]]` block, auto-extending up to 10 GB when 80 percent full.
  The mount is scoped to `processes = ["primary"]` —
  replicas have no volume and cold-sync to ephemeral rootfs.
- **Direct HTTP on :8080 with h2c.**
  The LiteFS proxy is intentionally not used;
  the app serves traffic directly on port 8080 with HTTP/2 cleartext so
  that gRPC/ConnectRPC streaming requests work end-to-end
  (the LiteFS proxy does not support HTTP/2 for gRPC).
  `fly.toml` enables `[http_service.http_options] h2_backend = true`
  so the Fly edge talks h2c to the backend.
- **Inverted `.primary` file semantics.**
  The file at `/litefs/.primary` is **present on replicas**
  (containing the primary's hostname)
  and **absent on the primary** (the primary holds the lease).
  The detection logic in `internal/litefs/primary.go` is:
  1. If `/litefs/.primary` exists → replica.
  2. If `/litefs/` exists but `.primary` does not → primary.
  3. If `/litefs/` does not exist (LiteFS not mounted) → fall back to the
     `PDBPLUS_IS_PRIMARY` env var (default `true` for local dev).
- **Write forwarding via `fly-replay`.**
  When `POST /sync` lands on a replica,
  the handler returns HTTP 307 with a `fly-replay: region=${PRIMARY_REGION}`
  header so the Fly edge re-routes the request to the primary region.

### Rolling deploy behavior

During a rolling deploy the LiteFS FUSE mount takes a brief moment to come up on
each new machine, and Fly's proxy may log "not listening" warnings while the
machine is between `litefs mount` starting and the app binary binding to
`:8080`.
This is expected and self-clears once the mount completes.
The `grace_period = "30s"` on the `/readyz` check in `fly.toml` is sized to
accommodate this.

`fly.toml` sets `strategy = "rolling"` with `max_unavailable = 0.5`,
which replaces roughly half the fleet at a time.
Blue-green deploys are not usable here
because two parallel fleets would conflict in the LiteFS + Consul primary
election.

## Asymmetric fleet

The fleet is split into two Fly process groups,
with different VM sizing and mount policies
(declared in `fly.toml`'s `[processes]` and per-group `[[vm]]` blocks):

- **`primary` group** — 1 machine in `lhr`, `shared-cpu-2x` / 512 MB,
  persistent `litefs_data` volume mounted at `/var/lib/litefs`.
  Runs the sync worker, holds the LiteFS Consul lease,
  source of LiteFS HTTP replication.
- **`replica` group** — read-only edge machines, `shared-cpu-1x` / 256 MB,
  **no persistent volume** (ephemeral rootfs).
  On boot, LiteFS cold-syncs the database from the primary via HTTP.
  LiteFS starts the application only after this cold sync,
  so the `/readyz` check fails
  and Fly Proxy routes around the machine until it is ready.

The production fleet has 8 machines:
1 primary in `lhr` and 7 replicas,
one in each of `iad`, `nrt`, `syd`, `lax`, `jnb`, `sin`, and `gru`.
`fly scale count` sets these counts.
`fly.toml` does not.
The `PdbPlusFleetMachineCountLow` alert expects this fleet.

**Volume-only-on-primary contract:** `[[mounts]]` in `fly.toml` is scoped to
`processes = ["primary"]`.
Only the LHR primary machine has a volume.
Fly.io does not replace a destroyed machine.
To replace a damaged replica, do these steps:

1. Find the machine ID and region: `fly machines list --app peeringdb-plus`.
2. Destroy the machine: `fly machine destroy --force <id>`.
3. Create a new machine in the same region:
   `fly scale count <n> --process-group replica --region <region>`.
   Set `<n>` to the number of replica machines that you want in that region.
4. Wait until `/readyz` on the new machine returns 200.
   The machine cold-syncs the database from the primary before it serves
   traffic.

**Replica cold-sync expectations** (measured during the v1.15 rollout):

| Region | Expected hydration | Notes |
|--------|--------------------|-------|
| iad, lax | 5-15s | Low-latency path to LHR |
| nrt, sin | 15-30s | Transpacific |
| syd, gru, jnb | 30-45s | Furthest edges; long-haul to LHR |

Typical hydration window is 5-45 seconds per region.

If a replica returns 503 for more than 5 minutes,
look for `readyz sync marked failed` or `readyz sync stale` in its logs.
Both conditions come from the `sync_status` rows
that LiteFS replicates from the primary.
To clear them, send `POST /sync` with the `PDBPLUS_SYNC_TOKEN`.
The new cycle on the primary writes a new `sync_status` row,
and LTX replication copies it to the replicas within seconds.

**Sizing rationale:** Observed replica RSS is 58-59 MB steady-state;
`shared-cpu-1x` / 256 MB gives ~4× memory headroom and budget
for LiteFS LTX replay spikes.
The primary keeps `shared-cpu-2x` / 512 MB —
it runs the sync worker whose memory profile was characterized during production
load testing.

## Regional rollout

The primary region is `lhr`.
Every other region has `replica` machines only.
To add a region, or to change the number of replica machines in a region,
run:

```bash
fly scale count <n> --process-group replica --region <region>   # machines in one region (not lhr)
```

To resize a process group:

```bash
fly scale vm shared-cpu-1x --memory 256 --process-group replica   # resize replica group
fly scale vm shared-cpu-2x --memory 512 --process-group primary   # resize primary group (matches fly.toml defaults)
```

Do not put `replica` machines in `lhr`.
`litefs.yml` makes every machine in `PRIMARY_REGION` a lease candidate,
whatever its process group.
A `replica` machine in `lhr` can take the lease while the primary restarts.
It then runs the sync worker on a machine that has no volume.
Always give `--region` when you scale the `replica` group.
Without `--region`, `fly scale count` acts on every region that has a machine
of the app, and `lhr` is one of these regions.

Only machines in `PRIMARY_REGION=lhr` are eligible to hold the LiteFS write
lease, so the primary group is sized at exactly 1 — running multiple primary
candidates wastes the persistent volume on the standby and does not add write
capacity (LiteFS is single-writer).

The alert `PdbPlusFleetMachineCountLow` counts the pairs of process group and
region that send metrics.
It does not count machines.
It fires below 6 and expects 8.
When you add or remove a region, change the threshold and the description in
`deploy/grafana/alerts/pdbplus-alerts.yaml`.
Then apply the rules again (see `deploy/grafana/alerts/README.md`).
A second machine in a region that already has one does not change the count.

## Monitoring

Observability uses OpenTelemetry end-to-end.
The SDK is initialized in `internal/otel/provider.go`
and picks up exporters via `autoexport.NewSpanExporter`,
`autoexport.NewMetricReader`, and `autoexport.NewLogExporter`.
Signals are selected by the standard environment variables documented at
<https://opentelemetry.io/docs/languages/sdk-configuration/>:

- `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_OTLP_HEADERS`,
  `OTEL_EXPORTER_OTLP_PROTOCOL` for OTLP export.
- `OTEL_TRACES_EXPORTER`, `OTEL_METRICS_EXPORTER`, `OTEL_LOGS_EXPORTER`
  (set to `none` to disable a signal).
- `OTEL_EXPORTER_PROMETHEUS_HOST` / `OTEL_EXPORTER_PROMETHEUS_PORT` for
  scrape-based metrics.
- `PDBPLUS_OTEL_SAMPLE_RATE` (app-specific) for the trace sampling ratio.
- `PDBPLUS_OTEL_SYNC_SAMPLE_RATE` (app-specific) for the trace sampling
  ratio of scheduled sync cycles (default `1.0`: every cycle).

A Grafana dashboard is provided in
`deploy/grafana/dashboards/pdbplus-overview.json` with a provisioning manifest
at `deploy/grafana/provisioning/dashboards.yaml` for self-hosted Grafana
instances.
Its `LiteFS Replication` row reads the `pdbplus.litefs.*` instruments,
so it has data only when `PDBPLUS_LITEFS_METRICS_URL` is set.
The row shows replica stream lag, LTX apply lag, transactions behind the
primary, commits on the primary, the raw LTX size and connected replicas.
Production alert rules live in `deploy/grafana/alerts/pdbplus-alerts.yaml`
and are applied via `mimirtool rules sync`
(see `deploy/grafana/alerts/README.md` for the workflow).

The OTLP endpoint, the Grafana host, and the Mimir tenant are deployment
values.
They are not in the repository.
Set the OTLP endpoint and headers as Fly secrets:
`fly secrets set OTEL_EXPORTER_OTLP_ENDPOINT=... OTEL_EXPORTER_OTLP_HEADERS=...`.

Fly.io's built-in machine metrics
(CPU, memory, network, disk)
are available through the Fly dashboard without additional configuration.

Runtime health:

- `GET /healthz` is the liveness probe.
  It returns 200 while the process runs.
- `GET /readyz` is the readiness probe.
  The `fly.toml` HTTP check uses it.
  It returns 503 in these conditions:
  - The database ping fails or takes more than 2 seconds
    (log: `readyz db probe failed`).
  - The `sync_status` query fails (log: `readyz sync lookup failed`).
  - No sync has completed.
  - The newest `sync_status` row has the status `failed`
    (log: `readyz sync marked failed`).
    Replicas read the same replicated row,
    so all machines return 503 until a new sync cycle starts.
  - The newest successful sync is older than `PDBPLUS_SYNC_STALE_THRESHOLD`,
    default `24h` (log: `readyz sync stale`).

  While a replica cold-syncs at boot,
  LiteFS does not start the application yet,
  so the check fails until the application listens on `:8080`.
  During graceful shutdown the listener closes,
  so the check fails and Fly Proxy stops routing to the machine.

### Sync memory watch

Every sync cycle, the worker tracks the per-cycle
`runtime.MemStats.HeapInuse` high-water mark
(sampled after the Phase A fetch
and after each type's Phase B upsert,
*before* the forced GC that follows a type of 1000 or more rows
reclaims the spike)
and reads (on Linux)
`/proc/self/status` VmHWM, attaches both as OTel span attrs
(`pdbplus.sync.peak_heap_bytes`, `pdbplus.sync.peak_rss_bytes`)
on the `sync-full` / `sync-incremental` span,
and fires `slog.Warn("heap threshold crossed", ...)`
when either breaches its configured threshold.
Lifetimes differ:
the heap peak resets every cycle,
while VmHWM is a process-lifetime high-water mark
that includes API-serving load and only resets on restart.
The same values are exported as Prometheus gauges
(`pdbplus_sync_peak_heap_bytes`, `pdbplus_sync_peak_rss_bytes`)
for dashboard timeseries.
Bytes is the canonical Prometheus unit.
Grafana formats MiB and GiB at render time.

Thresholds via `PDBPLUS_HEAP_WARN_MIB` (default 400) and `PDBPLUS_RSS_WARN_MIB`
(default 384).
Defaults sit under the Fly 512 MB VM cap with margin
so the order under pressure is: log → app crash → Fly OOM-kill.
Zero disables the warn for that metric (attrs still fire).
A sustained breach of `PDBPLUS_HEAP_WARN_MIB` needs investigation
(see **Memory escalation** below).

**Dashboard.**
The `Sync Memory` row in `deploy/grafana/dashboards/pdbplus-overview.json`
contains four panels:

- `Peak Heap` — threshold line at 400 MiB
  (Grafana auto-formats MiB / GiB from the `bytes` field unit)
- `Peak RSS` — threshold line at 384 MiB
- `Live Heap by Instance` —
  sourced from the `go_memory_used_bytes` OTel runtime gauge,
  plots all fleet machines (primary + replicas) across the asymmetric fleet
- `Response Heap Delta`: p50, p95, and p99 of
  `pdbplus_response_heap_delta_bytes` for each endpoint

**Memory escalation.**
If the peak heap stays above `PDBPLUS_HEAP_WARN_MIB` for several sync cycles,
find the cause before the primary reaches its 512 MB limit.
Look at `pdbplus_sync_peak_heap_bytes` for each cycle
and at the sync mode of those cycles (`sync_status.mode`).
Full cycles use more memory than incremental cycles.
`sync_status` keeps the newest 3000 cycles (about 31 days at the 15m interval),
plus the newest success row and the newest full success row.
For older cycles, use the Grafana metrics and logs.
Observed baseline (2026-04-17): primary peak 83.8 MiB,
replicas 58-59 MiB.

### Incident-response debug shell

The production image contains the `sqlite3` CLI.
To open a SQLite shell on the primary, run:

```bash
fly ssh console -a peeringdb-plus --process-group primary --pty -C 'sqlite3 /litefs/peeringdb-plus.db'
```

On a replica, the same path is read-only.
LiteFS rejects writes on machines that do not hold the lease.

## Capacity probing

`cmd/loadtest` is an operator binary
(NOT shipped in the prod image, NOT invoked by CI)
that drives read-only traffic against a deployed mirror to validate capacity,
warm dashboards, and reproduce load.
Build with `go build -o loadtest ./cmd/loadtest`.

Four subcommands:

- `loadtest endpoints` — one-shot inventory sweep across all 5 API
  surfaces (~114 requests).
- `loadtest sync` — replays the 13-step ordered sync GET sequence.
- `loadtest soak` — sustained QPS-capped mixed-surface load.
- `loadtest ramp` — finds the per-surface inflection point by
  ramping concurrency C=1 ×1.5/2s, triggering on p95 > 2× baseline
  OR p99 > 1s OR error rate > 1% OR a step in which no request
  completes, holding past inflection, then
  emitting a markdown table per surface to stdout (paste into
  capacity-planning docs / incident reports).

The default `--base` is `https://peeringdb-plus.fly.dev`
(`--target` is an alias).
The tool refuses `peeringdb.com` and its subdomains,
except `beta.peeringdb.com`.
Do not point it at upstream PeeringDB.
Upstream rate limits are strict and can block your IP.
See `cmd/loadtest/README.md` for full flag documentation and example output.

## Rollback

Fly.io keeps the image of each release.
To deploy an earlier image again, do these steps:

1. List the releases with their image references:

   ```bash
   fly releases --app peeringdb-plus --image
   ```

2. Deploy the image of the release that you want:

   ```bash
   fly deploy --app peeringdb-plus --image <image-ref>
   ```

This procedure does not build a new image.
To build from an earlier commit, check out that commit and run `fly deploy`:

```bash
git checkout <previous-sha>
fly deploy
```

## Deploy command summary

```bash
# Deploy the current working tree with Fly's remote builder.
# After a long pause the remote builder starts cold. `go build -v` in
# Dockerfile.prod prints each package, so a slow build shows progress.
# If a transient api.machines.dev error occurs, run `fly deploy` again.
fly deploy

# Build on the local Docker daemon instead
fly deploy --local-only

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
fly volumes create litefs_data --size 1 --region lhr       # optional: the first deploy creates the volume from [[mounts]]
fly secrets set PDBPLUS_PEERINGDB_API_KEY=... PDBPLUS_SYNC_TOKEN=...
fly deploy
fly scale count 1 --process-group replica --region <region>   # repeat for each replica region
fly machines list                                          # find each replica-group machine in lhr
fly machine destroy --force <id>                           # repeat for each replica-group machine in lhr
```

The first deploy creates the `replica` group machines in `lhr`,
the `primary_region` of `fly.toml`.
Create the replicas in their regions first,
then destroy each `replica` machine in `lhr`
(see [Regional rollout](#regional-rollout)).

## After an upstream PeeringDB release

An upstream release can change stored data in a way that incremental sync
does not pick up completely.
The daily full reconcile repairs this within `PDBPLUS_FULL_SYNC_INTERVAL`
(see [Daily full reconcile](ARCHITECTURE.md#daily-full-reconcile)).
A manual full sync makes the window shorter.
If the interval is `0`, the manual full sync is mandatory.

To find out if upstream has deployed a release, look at the data on the
mirror.
Do not send test requests to upstream PeeringDB.

### PeeringDB 2.83.0

Upstream migration 0160 changes each netixlan with `status='ok'` and
`operational=false` to `status='not-operational'`.
All of these rows get one `updated` value,
so the paged `?since=` fetch can skip some of them.
On 2026-09-23 the mirror had 627 such rows.

The checks below use a SQLite shell on a machine of the fleet:

```bash
fly ssh console -a peeringdb-plus --pty -C 'sqlite3 /litefs/peeringdb-plus.db'
```

1. Make sure that the mirror runs v1.28.0 or later.
   To see the running version, run:

   ```bash
   curl -s -H 'Accept: application/json' https://peeringdb-plus.fly.dev/ | jq -r .version
   ```

   Older releases hide `not-operational` connections on `/api` and in the
   Web UI.
2. Wait until upstream has deployed 2.83.0.
   After the deploy, the next incremental sync stores `not-operational` rows:

   ```sql
   SELECT status, COUNT(*) FROM network_ix_lans GROUP BY status;
   ```

3. Start one full sync.
   The request returns `202`, and the sync runs on the primary.
   A `409` response means that a sync cycle is already running.
   Wait until it ends, then send the request again:

   ```bash
   curl -X POST -H "X-Sync-Token: $PDBPLUS_SYNC_TOKEN" \
     'https://peeringdb-plus.fly.dev/sync?mode=full'
   ```

   The full sync also stores each `meta` document that upstream set
   before the mirror had the column.
4. Wait until the newest full sync has `status` `success`.
   Scheduled incremental cycles continue to run and add newer rows,
   so select the full cycles only:

   ```sql
   SELECT id, mode, status, completed_at FROM sync_status
     WHERE mode = 'full' ORDER BY id DESC LIMIT 1;
   ```

   `sync_status` keeps the newest 3000 cycles and the newest full success
   row, so this query always finds the full sync that you started.

5. Make sure that no skipped row is left.
   The result must be `0`,
   because upstream now sets `operational` to `status == 'ok'` on every save
   (2.83.0 `models.py:6512`):

   ```sql
   SELECT COUNT(*) FROM network_ix_lans WHERE status = 'ok' AND operational = 0;
   ```

   If the result is not `0`, the paged window of the full sync skipped some
   rows. While the upstream API cache predates 2.83.0, only the window
   returns these rows in their current state. Compare the
   `pdbplus.sync.snapshot.generated` attribute on the `sync-fetch-netixlan`
   span with the time of the upstream deploy. Start another full sync
   later: each full sync fetches the window again, and a row stored as
   `not-operational` stays, because the upserts keep a stored row that is
   newer than the snapshot. When upstream rebuilds its cache, the snapshot
   itself carries the rows.

## Operational failure modes quick-runbook

### 1) LiteFS lease/primary flaps

- Symptoms: frequent primary-role transitions, write replay spikes,
  elevated sync churn.
- First checks: process-group placement, Fly region health, LiteFS lease logs.
- Immediate action: stabilize primary placement before tuning sync intervals.

### 2) Startup object-count seed timeout

- Symptom: the WARN log
  `initial object count seed timed out; continuing with zeroed gauges until first refresh`.
  The object-count gauges of that machine show 0.
- On the primary, the next successful sync cycle refreshes the counts.
- A replica does not run sync cycles,
  so its gauges stay at 0 until it restarts.
  The dashboard uses `max by (type)`,
  so a replica at 0 does not change the totals.
  To reset the gauges, restart the replica: `fly machine restart <id>`.
- A seed error that is not a timeout stops the process.
  Fly restarts the machine.

### 3) Repeated upstream 429 / long Retry-After

- Symptoms: sync lag growth, repeated upstream rate-limit logs.
- First checks: API key presence, effective RPS,
  concurrent external callers sharing quota.
- Immediate action: set/rotate API key, reduce anonymous traffic,
  lower configured RPS if needed.

### 4) `/sync` auth misconfiguration

- Symptoms: every `POST /sync` returns 401,
  and startup logs carry
  `PDBPLUS_SYNC_TOKEN not set — POST /sync is disabled`.
  With no token configured the endpoint fail-closes —
  it rejects all requests rather than accepting them unauthenticated,
  so a missing token disables on-demand sync entirely
  (scheduled syncs are unaffected).
- First checks: `PDBPLUS_SYNC_TOKEN` present in runtime env and deploy secrets.
- Immediate action: set the token with
  `fly secrets set PDBPLUS_SYNC_TOKEN=...`.
  This command restarts the machines.
  Then send an authenticated `POST /sync`.

### 5) Force a full sync

1. Send the request.
   A replica answers with a `fly-replay` header,
   and Fly Proxy then sends the request to the primary.

   ```bash
   curl -X POST -H "X-Sync-Token: $PDBPLUS_SYNC_TOKEN" \
     'https://peeringdb-plus.fly.dev/sync?mode=full'
   ```

2. Make sure that the response is `202`.
   A `409` means that a cycle is running.
   Wait until it ends, then send the request again.
   A `401` means that the token is wrong or not set.

The primary always traces this cycle,
whatever `PDBPLUS_OTEL_SYNC_SAMPLE_RATE` is.
To send the request without a trace, add `&trace=0` to the URL.
The primary then does not trace the cycle, also when it traces every
scheduled cycle.

### 6) Primary lost its database

- A primary with no successful sync recorded runs a full sync at startup.
  You do not need to start it.
- Until that sync completes, `/readyz` on the primary returns 503.
- A replica whose application started before the first successful sync
  checks `sync_status` once per `PDBPLUS_SYNC_INTERVAL`.
  Its data routes return 503 until that check,
  even when `/readyz` returns 200.
  To make it ready at once, restart it: `fly machine restart <id>`.
