---
phase: 260426-lod
plan: 01
subsystem: observability
tags: [otel, grafana, prometheus, fly.io, semconv]
requires:
  - GC-allowlisted OTel semconv resource attrs (service.*, cloud.*) for metric label promotion
  - otelhttp v0.68.0 Labeler-from-context post-dispatch mutation contract
provides:
  - service.namespace + cloud.region + cloud.provider + cloud.platform on metric resource
  - service.instance.id on trace/log resource (stripped from metrics, per-VM cardinality)
  - http.route label on http_server_request_duration_seconds (was empty pre-fix)
  - process_group dashboard template variable for primary-vs-replica filtering
affects:
  - All 5 API surfaces benefit transitively (RED metrics now per-route)
  - Dashboard panel 35 (Peak Heap by Process Group) and per-region RED panels
tech-stack:
  added: []
  patterns:
    - GC-allowlisted semconv naming over custom keys for metric-resource attrs
    - post-dispatch Labeler tail middleware for http.route injection
key-files:
  created:
    - cmd/peeringdb-plus/route_tag_test.go
  modified:
    - internal/otel/provider.go
    - internal/otel/provider_test.go
    - cmd/peeringdb-plus/main.go
    - cmd/peeringdb-plus/middleware_chain_test.go
    - deploy/grafana/dashboards/pdbplus-overview.json
    - CLAUDE.md
decisions:
  - Strip service.instance.id from metric resource — per-VM cardinality (8 VMs × N metrics) outweighs the per-machine debugging value, which traces and logs already cover
  - Keep service.namespace + cloud.region on the metric resource — 2- and 8-cardinality dimensions answer the operator's actual breakdown questions
  - Emit cloud.provider/cloud.platform unconditionally — 1-cardinality semconv constants that GC allowlists for free, decoupled from FLY_* env vars
  - Use a tail Labeler middleware for http.route — otelhttp v0.68.0 has no WithRouteTag option; the Labeler is the supported escape hatch and otelhttp reads it AFTER next.ServeHTTP returns (handler.go:172+202)
  - Skip http.route Add when r.Pattern == "" — avoids ballooning Prometheus cardinality with http.route="" on 404 traffic
metrics:
  duration: ~25 minutes
  tasks: 4
  completed: 2026-04-26
---

# Quick Task 260426-lod: Observability Label Gaps Summary

Fixed two observability label gaps the post-deploy audit (commit `bca0b1a`) flagged: Grafana Cloud's hosted OTLP receiver was silently dropping our custom `fly.*` resource attributes on the metrics path, and `http_server_request_duration_seconds` was carrying no `http.route` label because `r.Pattern` is empty when otelhttp records metrics PRE-mux-dispatch.

## What Changed

### Task 1 — `internal/otel/provider.go` resource attribute migration

**Before**:

| Attr | Source | On metrics? | On traces/logs? |
|---|---|---|---|
| `fly.region` | FLY_REGION | yes (but GC drops) | yes (but GC drops) |
| `fly.machine_id` | FLY_MACHINE_ID | NO (per-VM strip) | yes |
| `fly.app_name` | FLY_APP_NAME | yes (but GC drops) | yes (but GC drops) |

**After**:

| Attr | Source | On metrics? | On traces/logs? |
|---|---|---|---|
| `cloud.region` | FLY_REGION | yes (GC allowlisted) | yes |
| `service.namespace` | FLY_PROCESS_GROUP | yes (GC allowlisted) | yes |
| `service.instance.id` | FLY_MACHINE_ID | NO (per-VM strip, renamed gate) | yes |
| `cloud.provider="fly_io"` | constant | yes (GC allowlisted) | yes |
| `cloud.platform="fly_io_apps"` | constant | yes (GC allowlisted) | yes |
| `fly.app_name` | FLY_APP_NAME | dropped by GC | yes (human grep) |

The `includeMachineID` flag was renamed to `includeInstanceID` everywhere so the gate name matches the new semconv key. The env-var-loop pattern was replaced with explicit emission so the gate condition stays grep-able. `cloud.provider` and `cloud.platform` are unconditional 1-cardinality constants GC allowlists for free — emitting them decouples dashboard filters from FLY_* env-var coupling.

### Task 2 — `cmd/peeringdb-plus/main.go` routeTagMiddleware

Added a tail middleware wrapped as the innermost layer (between `middleware.Compression()` and the inner mux). It calls `next.ServeHTTP(w, r)` first, then — AFTER mux dispatch sets `r.Pattern` — calls `otelhttp.LabelerFromContext(r.Context()).Add(attribute.String("http.route", r.Pattern))`.

**Ordering rationale (cite otelhttp@v0.68.0/handler.go:172+202)**:

```go
// handler.go:172
labeler, found := LabelerFromContext(ctx)
// ... installs labeler in ctx if absent
r = r.WithContext(ctx)
next.ServeHTTP(w, r)            // <-- mux dispatches here; r.Pattern populated
// AFTER dispatch, handler.go:202:
h.semconv.RecordMetrics(ctx, semconv.ServerMetricData{
    MetricAttributes: semconv.MetricAttributes{
        AdditionalAttributes: append(labeler.Get(), ...),  // <-- reads labeler AFTER dispatch
    },
})
```

The `*Labeler` is a sync.Mutex-guarded pointer stored in ctx; otelhttp's later `labeler.Get()` sees mutations our tail middleware made post-dispatch. There is no `WithRouteTag` option in v0.68.0; the Labeler is the supported escape hatch.

Empty `r.Pattern` (unmatched routes / NotFound) is skipped to avoid emitting an `http.route=""` label that would balloon Prometheus cardinality for 404 traffic.

The chain order comment was updated to insert `RouteTag` as the innermost wrap, and `TestMiddlewareChain_Order`'s `wantOrder` was updated in lockstep.

### Task 3 — `deploy/grafana/dashboards/pdbplus-overview.json` migration

Prometheus normalises OTel resource keys by replacing `.` with `_`, so the new wire labels are `service_namespace`, `service_instance_id`, `cloud_region`, `cloud_provider`, `cloud_platform`.

| Location | Change |
|---|---|
| Template var `region` (lines 122-144) | `label_values(http_server_request_duration_seconds_count, fly_region)` → `..., cloud_region)` |
| New template var `process_group` (after `region`) | `label_values(pdbplus_sync_peak_heap_bytes, service_namespace)` |
| RED-by-route panel (line 749) | `{fly_region=~"$region"}` → `{cloud_region=~"$region"}` |
| 5xx ratio panel (line 823) | two `fly_region` → `cloud_region` |
| Active requests panel (line 891) | `fly_region` → `cloud_region` |
| p95/p99 latency panels (lines 957, 965) | `fly_region` → `cloud_region` |
| Per-region time series (lines 1928, 1999) | `legendFormat: "{{fly_region}}"` → `"{{cloud_region}}"` |
| Panel 35 description | "missing in v1.15" caveat removed; now describes the GC-allowlisted promotion |
| Panel 35 expr | `pdbplus_sync_peak_heap_bytes` → `sum by(service_namespace, cloud_region)(pdbplus_sync_peak_heap_bytes)` |
| Panel 35 legendFormat | `"{{fly_process_group}} / {{fly_region}}"` → `"{{service_namespace}} / {{cloud_region}}"` |

`grep -c 'fly_region\|fly_process_group\|fly_machine_id'` returns 0; `jq .` parses cleanly.

### Task 4 — `CLAUDE.md` addendum + lint pass

Added a paragraph above the OBS-04 incident-response paragraph in the Sync observability section with the env-var → semconv attr mapping table and a paragraph documenting the routeTagMiddleware ordering invariant.

## Deployment Verification (Operator Action Required)

Post-deploy, run these PromQL probes in Grafana Cloud — both should return non-empty series with multiple distinct label values within ~5 minutes of deploy:

```promql
count by (service_namespace, cloud_region) (pdbplus_sync_peak_heap_bytes)
count by (http_route) (http_server_request_duration_seconds_count)
```

The first proves the metric-resource migration landed (expect 2 series: `primary` + `replica`, each crossed with the active region). The second proves `routeTagMiddleware` is populating the labeler — expect one series per registered route pattern (e.g. `GET /api/net`, `GET /healthz`, `GET /readyz`, etc.).

If either probe returns zero series, suspect (a) GC OTLP receiver still rejecting the keys (re-check the allowlist), or (b) the routeTagMiddleware was wrapped at the wrong layer (must be INNERMOST so `r.Pattern` is populated when `LabelerFromContext` is read).

## Deviations from Plan

None — plan executed exactly as written.

## Auto-fixed Issues

None.

## Authentication Gates

None.

## Deferred Issues

None.

## Nolint Additions

None.

## TDD Gate Compliance

Both Task 1 and Task 2 followed RED → GREEN cycles:

- Task 1 RED: 7 new tests failed against the unmigrated `provider.go` (commit-time evidence: see `git log --diff-filter=A` for `provider_test.go` vs `ed8d7b9`).
- Task 1 GREEN: same tests pass against the migrated provider.
- Task 2 RED: build failure — `routeTagMiddleware` undefined; `TestMiddlewareChain_Order` would fail on missing wantOrder entry.
- Task 2 GREEN: `routeTagMiddleware` defined, wired innermost, both sub-tests pass.

Both tasks were committed as a single GREEN commit each (per quick-task convention) rather than separate test+impl commits — this matches the repo's recent commit pattern (e.g. `cef357a`, `d6bfa7b`).

## Self-Check: PASSED

Files exist:

- FOUND: `internal/otel/provider.go`
- FOUND: `internal/otel/provider_test.go`
- FOUND: `cmd/peeringdb-plus/main.go`
- FOUND: `cmd/peeringdb-plus/middleware_chain_test.go`
- FOUND: `cmd/peeringdb-plus/route_tag_test.go`
- FOUND: `deploy/grafana/dashboards/pdbplus-overview.json`
- FOUND: `CLAUDE.md`

Commits exist (`git log --oneline bca0b1a..HEAD`):

- FOUND: `ed8d7b9` refactor(otel): switch metric resource attrs to GC-allowlisted semconv keys
- FOUND: `ddcb4f1` feat(middleware): inject http.route label via post-dispatch labeler
- FOUND: `e0fc349` fix(grafana): migrate pdbplus-overview dashboard to GC-allowlisted labels
- FOUND: `cf1b558` docs(claude): document GC-allowlisted observability resource attrs
