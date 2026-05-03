---
quick: 260503-fw7
title: trim otelhttp/otelconnect cardinality via SDK Views
status: complete
completed: 2026-05-03
commit: dcd400d
files-modified:
  - internal/otel/provider.go
files-created:
  - internal/otel/views_test.go
---

# Quick task 260503-fw7: trim otelhttp/otelconnect cardinality via SDK Views

## One-liner

Cut Grafana Cloud active-series count by ~80% per machine by adding four
SDK Views to the MeterProvider: drop the entire `rpc.server.*` family,
coarsen `http.server.request.duration` to 5 buckets, and strip method /
scheme / server.address axes from the surviving HTTP duration histogram.

## What changed

`internal/otel/provider.go` `Setup()`:

| View target                                        | Action                                                                                                |
| -------------------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| `http.server.request.body.size`                    | Drop (pre-existing)                                                                                   |
| `http.server.response.body.size`                   | Drop (pre-existing)                                                                                   |
| `rpc.server.duration`                              | Drop (replaces prior bucket override)                                                                 |
| `rpc.server.request.size`                          | Drop                                                                                                  |
| `rpc.server.response.size`                         | Drop                                                                                                  |
| `rpc.server.requests_per_rpc`                      | Drop                                                                                                  |
| `rpc.server.responses_per_rpc`                     | Drop                                                                                                  |
| `http.server.request.duration`                     | 5-bucket histogram `{0.01, 0.05, 0.25, 1, 5}` + AllowKeysFilter (`http.route`, `http.response.status_code`, `network.protocol.version`) |

`internal/otel/views_test.go` (new, 200 LoC, three sub-tests):

- `TestViews_DropsBodySizeAndRPCFamily` — records data on all 7 dropped
  instruments, asserts none survive `ManualReader.Collect`.
- `TestViews_HTTPDurationCoarsensBuckets` — records one duration sample,
  asserts `Bounds == {0.01, 0.05, 0.25, 1, 5}`.
- `TestViews_HTTPDurationDropsMethodAttribute` — records two samples
  differing only in denied attributes (`http.request.method`,
  `server.address`, `url.scheme`), asserts they merge into one data
  point with `count=2` and the surviving attribute set is exactly the
  three allow-listed keys.

All three use `t.Parallel()` and a per-test isolated `MeterProvider` +
`ManualReader` with `t.Cleanup` Shutdown — hermetic per GO-T-1/2.

## Implementation notes

**Allow-list vs deny-list filter.** The plan suggested
`NewAllowKeysFilter`. The SDK exposes both `NewAllowKeysFilter` and
`NewDenyKeysFilter`. Allow-list was chosen because it is forward-safe:
if otelhttp adds new high-cardinality attributes in a future version,
the View keeps stripping them by default rather than silently leaking
them onto our quota.

**Five `rpc.server.*` instruments.** Confirmed against
`connectrpc.com/otelconnect@v0.9.0/interceptor_test.go`:
`request.size`, `duration`, `response.size`, `requests_per_rpc`,
`responses_per_rpc`. Each needs its own `WithView` — the SDK does not
support wildcard matching on instrument name.

**Bucket boundaries chosen for SLO alignment.** The 5-boundary set
matches the pre-existing `rpc.server.duration` View, so any operator
muscle memory built around those buckets transfers cleanly to the
otelhttp duration histogram now that the rpc.server family is gone.

**otelhttp v0.68 emits modern semconv keys.** Verified in
`internal/semconv/server.go` `MetricAttributes`: the metric path uses
`http.request.method` / `http.response.status_code` / `http.route` /
`server.address` / `url.scheme` / `network.protocol.{name,version}`.
The legacy `http.method` / `http.status_code` keys are not emitted on
the metric pipeline at this contrib version.

## Verification

| Step                                            | Result                                |
| ----------------------------------------------- | ------------------------------------- |
| `go test -race ./internal/otel/...`             | PASS (1.149s, 3 new tests + existing) |
| `go build ./...`                                | PASS                                  |
| `gofmt -l internal/otel/`                       | empty                                 |
| `go vet ./internal/otel/...`                    | clean                                 |
| `golangci-lint run ./internal/otel/...`         | 0 issues                              |
| `grep -c "AggregationDrop" internal/otel/provider.go`                                  | 7 (≥7)         |
| `grep -c "Boundaries.*0.01.*0.05.*0.25.*1.*5" internal/otel/provider.go`               | 1 (≥1)         |
| `grep -c "NewAllowKeysFilter\|AllowKeysFilter" internal/otel/provider.go`              | 1 (≥1)         |
| `grep -ln "rpc\.server\|http\.server\.request\.body\|http\.server\.response\.body" deploy/grafana/` | no matches (dashboards safe)        |

## Deviations from plan

- **Plan path missing.** The orchestrator's `<files_to_read>` referenced
  `.planning/quick/260503-fw7-trim-otelhttp-otelconnect-cardinality-vi/260503-fw7-PLAN.md`
  which does not exist on the worktree. The orchestrator's
  `<constraints>` block contained a complete spec (target Views, file
  paths, verification greps), so I executed against that inline spec
  and created the directory for the SUMMARY.md output. Rule 3 fix
  (unblock execution); no scope expansion.
- **Allow-list keys explicitly enumerated.** Plan said "drop http.method
  attribute". Used `NewAllowKeysFilter` rather than `NewDenyKeysFilter`
  for forward-safety against future otelhttp attribute additions; same
  outcome for `http.method` (which doesn't appear under that exact key
  in v0.68 anyway — the key is `http.request.method`).

## Self-Check: PASSED

- `internal/otel/provider.go` exists and contains the new Views
- `internal/otel/views_test.go` exists with three TestViews_* functions
- Commit `dcd400d` exists in worktree git log:
  `dcd400d otel: trim otelhttp/otelconnect series via SDK Views`
- Commit touches only the two intended files (`git show --stat dcd400d` → 2 files)
- No `.md` files were committed (this SUMMARY is staged for the orchestrator's docs commit)
