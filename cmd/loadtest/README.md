# loadtest — peeringdb-plus operator load tool

> ## SAFETY WARNING
>
> **NEVER point `--base` at `https://www.peeringdb.com`.** Upstream
> PeeringDB enforces a 1-request-per-hour rate limit per IP address
> and will block your IP if you exceed it. This tool is for the
> **mirror** at `https://peeringdb-plus.fly.dev` (default) or your
> own local deployment via `--base http://localhost:8080`.
>
> **Do NOT run this tool from CI.** It is intentionally build-tag
> isolated behind `//go:build loadtest` so `go build ./...`,
> `go test ./...`, `golangci-lint run`, and the GitHub Actions CI
> jobs ignore the package entirely. Only operators with a clear
> intent should ever produce a binary.

## What it does

Three modes drive read-only HTTP traffic against a peeringdb-plus
mirror, exercising every entity type across all five API surfaces
(pdbcompat `/api`, entrest `/rest/v1`, GraphQL `/graphql`,
ConnectRPC `/peeringdb.v1.*`, Web UI `/ui`):

| mode        | purpose                                                                 |
| ----------- | ----------------------------------------------------------------------- |
| `endpoints` | One-shot inventory sweep (~114 distinct requests). Validates every API surface returns 2xx. |
| `sync`      | Replays the 13-step ordered sync sequence (full or incremental). Mirrors `internal/sync/worker.go syncSteps()`. |
| `soak`      | Sustained QPS-capped mixed-surface load. Defaults to 30 s × 4 workers × 5 req/s. |

Read-only verbs only: `GET` for pdbcompat / entrest / Web UI, `POST`
for GraphQL queries and ConnectRPC unary calls. No `PUT`, `PATCH`,
or `DELETE` anywhere — peeringdb-plus is a read-only mirror.

## Build

```bash
go build -tags loadtest -o loadtest ./cmd/loadtest
```

The binary is **not committed** and `go build ./...` (without `-tags
loadtest`) will not produce it. CI does not include the tag, so
`go test ./...` and lint runs ignore the package.

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

## CI exclusion

The `//go:build loadtest` constraint at the top of every `.go` file
in this directory means:

- `go build ./...` — does not compile the package. Verified.
- `go test ./...` — does not see `*_test.go` files in the package.
  Verified.
- `golangci-lint run` — skips tag-gated files by default.
  Verified.
- `.github/workflows/ci.yml` — does not pass `-tags loadtest`. The
  loadtest binary never ships in CI artefacts, never appears in
  Docker images, and never runs in production deployments.

To run loadtest's own tests locally during development:

```bash
go test -tags loadtest -race ./cmd/loadtest/...
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
- `endpoints.go` — `runEndpoints` sequential sweep driver
- `sync.go` — `runSync` 13-step ordered driver + `syncOrder` parity mirror
- `soak.go` — `runSoak` errgroup × rate-limited worker pool
- `report.go` — per-surface aggregation, percentile calculation, table printer
- `*_test.go` — TDD tests, all `//go:build loadtest`-gated
