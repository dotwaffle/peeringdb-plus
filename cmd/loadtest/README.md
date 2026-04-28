# loadtest — peeringdb-plus operator load tool

> ## SAFETY WARNING
>
> **NEVER point `--base` at `https://www.peeringdb.com`.** Upstream
> PeeringDB enforces a 1-request-per-hour rate limit per IP address
> and will block your IP if you exceed it. This tool is for the
> **mirror** at `https://peeringdb-plus.fly.dev` (default) or your
> own local deployment via `--base http://localhost:8080`.
>
> **Do NOT run this tool from CI.** The package compiles as a normal
> `cmd/` binary, but the binary is never invoked by CI / Dockerfiles
> / deployment scripts — only by operators against deployed
> instances. Unit tests in this directory are hermetic
> (`httptest`-based, no outbound calls) and are safe to run in CI as
> part of the standard `go test ./...` sweep.

## What it does

Four modes drive read-only HTTP traffic against a peeringdb-plus
mirror, exercising every entity type across all five API surfaces
(pdbcompat `/api`, entrest `/rest/v1`, GraphQL `/graphql`,
ConnectRPC `/peeringdb.v1.*`, Web UI `/ui`):

| mode        | purpose                                                                 |
| ----------- | ----------------------------------------------------------------------- |
| `endpoints` | One-shot inventory sweep (~114 distinct requests). Validates every API surface returns 2xx. |
| `sync`      | Replays the 13-step ordered sync sequence (full or incremental). Mirrors `internal/sync/worker.go syncSteps()`. |
| `soak`      | Sustained QPS-capped mixed-surface load. Defaults to 30 s × 4 workers × 5 req/s. |
| `ramp`      | Per-surface concurrency ramp; finds the inflection point where p95/p99 latency or error rate degrades. Sequential per surface (no cross-surface contention). |

Read-only verbs only: `GET` for pdbcompat / entrest / Web UI, `POST`
for GraphQL queries and ConnectRPC unary calls. No `PUT`, `PATCH`,
or `DELETE` anywhere — peeringdb-plus is a read-only mirror.

## Build

```bash
go build -o loadtest ./cmd/loadtest
```

The binary is **not committed** and `go build ./...` only compiles
it (it does not run it). CI compiles + lints + tests this package
along with the rest of the repository, but never invokes the
resulting binary.

## Modes

### `endpoints` — inventory sweep

```bash
./loadtest endpoints --base http://localhost:8080
```

Sequentially hits every endpoint in the registry (~114 requests:
13 entities × ~8 shapes per entity, plus 3 surface-wide UI routes).
Sequential by design: this is a once-per-run sweep for dashboard
warmup and post-deploy validation, not concurrent stress.

| flag         | default                              | description                                         |
| ------------ | ------------------------------------ | --------------------------------------------------- |
| `--base`     | `https://peeringdb-plus.fly.dev`     | Target deployment URL.                              |
| `--timeout`  | `30s`                                | Per-request timeout.                                |
| `--verbose`  | `false`                              | Emit one log line per request.                      |

### `sync` — replay sync GET sequence

```bash
./loadtest sync --mode=full --base http://localhost:8080
./loadtest sync --mode=incremental --since=2026-04-27T12:00:00Z
./loadtest sync --mode=incremental --since=1714219200
```

Issues exactly 13 GETs against `/api/<short>?limit=250&skip=0&depth=0`
in **FK dependency order** — `org, campus, fac, carrier, carrierfac,
ix, ixlan, ixpfx, ixfac, net, poc, netfac, netixlan` — mirroring the
live worker at `internal/sync/worker.go syncSteps()`. The
`internal/sync.StepOrder()` export is the single source of truth;
the loadtest's parity test (`TestSync_OrderingMatchesWorker`) fails
the build if a future syncSteps() reorder happens without updating
the loadtest.

| flag       | default          | description                                                                                                                               |
| ---------- | ---------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| `--mode`   | `full`           | `full` or `incremental`.                                                                                                                  |
| `--since`  | (empty)          | Cursor for `incremental` mode. Accepts RFC3339 (`2026-04-27T12:00:00Z`) or unix seconds (`1714219200`). Defaults to `now - 1h` when unset. |
| `--base`   | `https://peeringdb-plus.fly.dev` | (as above)                                                                                                                  |
| `--timeout` | `30s`           | (as above)                                                                                                                                |
| `--verbose` | `false`         | (as above)                                                                                                                                |

### `soak` — sustained mixed-surface load

```bash
./loadtest soak --duration=30s --concurrency=4 --qps=5
./loadtest soak --duration=5m --concurrency=10 --qps=20  # heavier — see notes
```

Spawns `--concurrency` workers behind a **global**
`golang.org/x/time/rate.Limiter` capped at `--qps` requests per
second (burst=1). Each worker picks a random endpoint from the
registry, hits it, and records latency. After `--duration` elapses
the soak terminates and prints per-surface p50/p95/p99 latency +
success rates.

| flag             | default | description                                            |
| ---------------- | ------- | ------------------------------------------------------ |
| `--duration`     | `30s`   | Total soak duration.                                   |
| `--concurrency`  | `4`     | Number of concurrent workers (sharing the rate cap).   |
| `--qps`          | `5`     | Global request-per-second cap. Combined across workers. |
| `--base`         | `https://peeringdb-plus.fly.dev` | (as above)                          |
| `--timeout`      | `30s`   | (as above)                                             |
| `--verbose`      | `false` | (as above)                                             |

#### QPS rationale

The defaults (4 × 5 req/s) are conservative for `shared-cpu-1x`
replicas. Reasonable knobs:

- **Smoke / warmup:** `--qps=5 --concurrency=4` (default). Light
  enough to hit a Fly machine without tripping replica-side
  middleware rate limits.
- **Stress:** `--qps=20 --concurrency=10`. Approaches the upper end
  of what a `shared-cpu-2x` primary can sustain across all 5
  surfaces. Watch the Grafana `Live Heap by Instance` panel during
  the run.
- **DO NOT** push `--qps` past ~50 against the deployed Fly app
  without coordinating — middleware rate limiting and Fly Proxy
  back-pressure both cut in.

### `ramp` — find inflection point per surface

Per-surface concurrency ramp that discovers, surface by surface,
the concurrency level at which p95/p99 latency or error rate
visibly degrades. Replaces ad-hoc operator probing with a
deterministic, scriptable, per-surface capacity probe.

```bash
./loadtest ramp --target http://localhost:8080 --entity net --max-concurrency 64
./loadtest ramp --target https://peeringdb-plus.fly.dev --entity org --surfaces pdbcompat,graphql
```

Ramp loop per surface:

1. **Baseline** at `--start` concurrency for `--step-duration`.
2. Multiply concurrency by `--growth` each step (capped at `--max-concurrency`).
3. **Inflection** triggers on the first step where:
   - `p95 > baseline.p95 × --p95-multiplier`, OR
   - `p99 > --p99-absolute`, OR
   - `error rate > --error-rate-threshold`.
4. **Hold** at the inflection concurrency for `--hold-duration` to
   gather a stable p99 reading.
5. **Past-inflection**: 1-2 additional steps (concurrency permitting)
   so the operator can see how badly things degrade just past the knee.
6. Print a markdown table for the surface to stdout, then move on.

Surfaces are exercised **sequentially** (in `--surfaces` order) so
cross-surface contention does not bias results. Each surface fetches
the same prefetched ID list (round-robin) so requests don't all hit
a single hot row.

| flag                         | default                          | description                                                                                          |
| ---------------------------- | -------------------------------- | ---------------------------------------------------------------------------------------------------- |
| `--target`                   | `https://peeringdb-plus.fly.dev` | Alias for `--base`. Either flag works.                                                                |
| `--entity`                   | `net`                            | Entity type for the ramp: `net` (uses `/ui/asn/<asn>` for webui) or `org` (uses `/ui/org/<id>`).      |
| `--start`                    | `1`                              | Initial concurrency level (the baseline step).                                                       |
| `--growth`                   | `1.5`                            | Per-step concurrency multiplier (`next = ceil(prev * growth)`).                                       |
| `--step-duration`            | `2s`                             | Wall-clock time at each ramp step.                                                                    |
| `--hold-duration`            | `10s`                            | Wall-clock time held at the inflection step for stable p99.                                           |
| `--max-concurrency`          | `256`                            | Upper bound on per-step concurrency. Ramp terminates once a step would exceed this cap.               |
| `--p95-multiplier`           | `2.0`                            | Inflection trigger: step.p95 > baseline.p95 × this multiplier.                                        |
| `--p99-absolute`             | `1s`                             | Inflection trigger: step.p99 > this absolute duration.                                                |
| `--error-rate-threshold`     | `0.01`                           | Inflection trigger: step error rate > this fraction (0.01 = 1%).                                      |
| `--surfaces`                 | (all five)                       | Comma-separated surfaces in execution order. Valid: `pdbcompat,entrest,graphql,connectrpc,webui`.     |
| `--prefetch-count`           | `20`                             | Number of IDs to prefetch via `/api/<entity>?limit=N` for round-robin selection across ramp steps.    |

#### Ramp vs soak — when to use which

- **Soak** is for **sustained known-rate validation**: "does the
  mirror handle 20 req/s × 5 minutes without dropping responses?"
  You set the rate, run for a duration, and check the success
  percentage and latency tail. The traffic mix is randomly drawn
  from the full registry — every surface contributes.
- **Ramp** is for **capacity discovery**: "at what concurrency does
  the pdbcompat surface fall over?" You don't know the answer in
  advance; ramp finds it. Per-surface ramps tell you that, e.g.,
  the GraphQL surface tips at C=24 while the pdbcompat surface
  doesn't tip until C=64 — useful for capacity planning and for
  knowing which surface dominates your incident response when
  load spikes hit.

#### Sample output

```markdown
### pdbcompat (entity=net)

| label              |   C |     p50 |     p95 |     p99 |  err % |     rps |
|--------------------|----:|--------:|--------:|--------:|-------:|--------:|
| baseline           |   1 |    12ms |    29ms |    41ms |  0.00% |    54.2 |
| inflection         |   8 |    74ms |   412ms |  1.21s  |  0.00% |   189.3 |
| hold               |   8 |    79ms |   428ms |  1.18s  |  0.00% |   192.6 |
| past-inflection    |  12 |   156ms |   891ms |  2.04s  |  0.42% |   201.0 |

inflection reason: p99 1.21s > 1s absolute
```

The output is markdown verbatim — paste directly into incident
reports, capacity-planning docs, or `tee` to a file:

```bash
./loadtest ramp --target http://localhost:8080 | tee ramp-results.md
```

## Auth

Set `PDBPLUS_LOADTEST_AUTH_TOKEN` to send `Authorization: Bearer
<token>` on every request:

```bash
PDBPLUS_LOADTEST_AUTH_TOKEN=$(cat ~/.pdbplus-token) ./loadtest soak --duration=1m
```

When unset, requests are anonymous (matching what an unauthenticated
external client sees).

## Output

Every mode prints a tab-separated summary table at the end. Pipe
through `column -t -s$'\t'` for fixed-width formatting:

```text
=== loadtest soak summary ===
wall-clock      30.012s
observed-rps    4.97 req/s

surface     count  ok   err  success%  p50    p95    p99
pdbcompat   53     53   0    100.0%    8ms    24ms   48ms
entrest     31     31   0    100.0%    9ms    27ms   52ms
graphql     16     16   0    100.0%    18ms   62ms   89ms
connectrpc  29     29   0    100.0%    11ms   36ms   71ms
webui       20     20   0    100.0%    7ms    19ms   34ms
TOTAL       149    149  0    100.0%    10ms   34ms   71ms
```

Latency percentiles use the **nearest-rank** method on a sorted
slice of observed latencies. p99 reports the 99th-percentile of the
observed distribution; a single anomalous outlier in 100 requests
will surface only at p100, which is intentionally not printed (read
the `err` column to surface anomalies).

## CI behaviour

This package is a regular `cmd/` binary — no build tags. CI:

- `go build ./...` — compiles `cmd/loadtest/loadtest` along with
  every other binary. The artefact is discarded.
- `go test -race ./...` — runs the unit tests. They use
  `httptest.NewServer` for the server side; nothing reaches a real
  Fly.io endpoint.
- `golangci-lint run` — lints the package as part of the default
  sweep.
- `.github/workflows/ci.yml` — never invokes the resulting binary.
  The loadtest binary never ships in Docker images and never runs
  in production deployments.

To run loadtest's own tests locally during development:

```bash
go test -race ./cmd/loadtest/...
```

## Out of scope

- **Distributed load** — single-process only. Use `wrk`, `k6`, or
  `vegeta` for multi-region distributed traffic.
- **Recording / replay** — no HAR or HTTP capture. The registry is
  declarative.
- **Write endpoints** — peeringdb-plus is a read-only mirror; the
  upstream PeeringDB is the source of truth for writes.
- **Custom queries** — GraphQL queries are minimal `{ id }`
  selectors; ConnectRPC bodies are `{"id":1}` / `{"limit":10}`. If
  you need richer query payloads, extend `cmd/loadtest/registry.go`.

## Files

- `main.go` — flag parsing + mode dispatch
- `surfaces.go` — `Surface` enum + `Hit()` request executor
- `registry.go` — 13-entity × 5-surface inventory (~114 endpoints)
- `discover.go` — `discoverIDs` + `discoverRampIDs` (id/asn prefetch)
- `endpoints.go` — `runEndpoints` sequential sweep driver
- `sync.go` — `runSync` 13-step ordered driver + `syncOrder` parity mirror
- `soak.go` — `runSoak` errgroup × rate-limited worker pool
- `ramp.go` — `runRamp` per-surface concurrency ramp + inflection detector + markdown emitter
- `report.go` — per-surface aggregation, percentile calculation, table printer
- `*_test.go` — hermetic httptest-based unit tests
