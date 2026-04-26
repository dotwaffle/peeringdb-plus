# Quick Task 260426-mei: Production Observability — Alerts + Per-instance Runtime Memory - Context

**Gathered:** 2026-04-26
**Status:** Ready for planning

<domain>
## Task Boundary

Two coupled gaps from the post-deploy audit (after quick task 260426-lod added GC-allowlisted resource attribute labels):

- **Gap A — No alert rules.** The `pdbplus-overview` dashboard exists but no Grafana Cloud alert rule pages on sync stalls, sync failures, fleet-size drops, or sustained heap pressure (SEED-001 trigger).
- **Gap B — Per-instance runtime memory visibility.** Panel 35 ("Peak Heap by Process Group") only ever shows `primary` because `pdbplus_sync_peak_heap_bytes` fires from sync-completion paths and replicas don't sync. Operators have no per-replica heap signal in the dashboard.

**Live finding during discussion:** `go.opentelemetry.io/contrib/instrumentation/runtime` v0.68.0 is already wired at `internal/otel/provider.go:121` (`runtime.Start(runtime.WithMeterProvider(mp))`). Post-260426-lod deploy, its emissions carry our new `service_namespace` + `cloud_region` labels and ALL 8 machines are reporting `go_memory_used_bytes` (Prom-translated from OTel `go.memory.used`, semconv v1.36.0 goconv naming). **Gap B is therefore a dashboard-only fix** — no Go code work, just re-source panel 35 to `go_memory_used_bytes`.

</domain>

<decisions>
## Implementation Decisions

### Alert rule format

**Decision:** Both — Prometheus rule YAML as source of truth + GC sync via `mimirtool`.

- Author rules in `deploy/grafana/alerts/pdbplus-alerts.yaml` using standard Prometheus `groups:`/`rules:` schema (the format Mimir alertmanager loads natively).
- Document `mimirtool rules sync` (or `mimirtool rules load`) as the apply mechanism in the new `deploy/grafana/alerts/README.md`.
- Same convention shape as the existing `deploy/grafana/dashboards/*.json` (file in repo, manual apply step) — keeps observability config under version control without requiring CI integration.
- Lints via `mimirtool rules check` and the standard `promtool check rules`.

### Alert notification channel

**Decision:** Bind to Grafana Cloud's default email contact point by literal name string.

- The receiver string is `grafana-default-email` (same canonical name for every GC user — NOT an email address itself).
- **MUST NOT** commit user's email address, personal Grafana stack URL (`*.grafana.net`), or any stack/tenant identifier to any repo file. This includes the alert YAML, README, dashboard JSON, CLAUDE.md, SUMMARY.md — everywhere.
- Documentation that points the user at where to view/edit notification routing must use a generic phrasing ("your Grafana Cloud notifications UI"), never a specific URL.

### Severity tiering

**Decision:** Two tiers — `critical` (page) and `warning` (notify-only).

- `critical`: sync freshness >2h (8× the 15-min authenticated cadence), sync failure-rate sustained breach, fleet machine-count drop ≥2 below expected
- `warning`: sustained heap above `PDBPLUS_HEAP_WARN_MIB` (SEED-001 trigger), sustained RSS above `PDBPLUS_RSS_WARN_MIB`, single sync failure
- Both tiers route to the same `grafana-default-email` contact point at the receiver level; routing/escalation is configured out-of-band in the GC UI based on the `severity` label.

### Runtime gauge naming + implementation approach

**Decision:** Use the OTel runtime contrib package that's already running. NO new Go gauge code.

- `go.opentelemetry.io/contrib/instrumentation/runtime` v0.68.0 emits semconv v1.36.0 `goconv` metrics on every instance — confirmed live in Prom under names like `go_memory_used_bytes`, `go_goroutine_count`, `go_gc_gogc_percent`, `go_gc_duration_seconds`, `go_memory_limit_bytes`, etc.
- These already carry our `service_namespace` + `cloud_region` labels post-260426-lod (verified: `primary @ lhr` + 7 replica regions all reporting).
- Panel 35 ("Peak Heap by Process Group") gets re-sourced to:
  ```promql
  sum by (service_namespace, cloud_region) (go_memory_used_bytes{service_name="peeringdb-plus"})
  ```
- The OTel `go_memory_type` dimension (`stack` / `other`) is summed away by the panel — operators want one number per instance, not a stack/heap split.

### Existing pdbplus_sync_peak_{heap,rss}_bytes metrics

**Decision:** Keep them as sync-watermark metrics. No changes.

- `go.memory.used` is a tick gauge (live value sampled per OTel push interval).
- `pdbplus.sync.peak_heap_bytes` is a sync-cycle watermark (peak observed during one sync cycle) — different semantics, both useful for SEED-001 watching.
- The metric naming makes the watermark-vs-live distinction implicit; not worth a rename in this task. CLAUDE.md "Sync observability" section can stay as-is.

### Dashboard updates (in scope)

- Re-source panel 35 query and description so it works for all 8 instances via `go_memory_used_bytes`.
- Optionally: add a new panel for `go_goroutine_count` and `go_gc_duration_seconds` as part of the SEED-001 watch row — both are signals operators care about. **Decision: defer — out of scope for this quick task.** Open an SEED or future quick task if desired.

### Tick cadence (Gap B)

**Decision:** N/A — using OTel runtime contrib defaults.

- The `runtime.Start()` package emits at the OTel SDK's metric reader push interval (default 60s for OTLP). No knob to tune in this task.

### Claude's Discretion

- Exact threshold values for each alert (will pick conservative defaults grounded in observed baselines: heap 22 MiB primary / 23 MiB replicas, RSS 79 MiB primary).
- Alert rule `for:` durations (sustained-breach windows) — pick values that avoid flapping on transient blips.
- Whether to add a `runbook_url` annotation pointing at a docs/RUNBOOK.md or just an inline `description` annotation. Default: inline `description` only; runbook docs are a separate effort.
- Whether to mirror the alert rules into the dashboard as alert annotations (Grafana 10+ supports this). Default: skip — keeps the dashboard JSON clean.

</decisions>

<specifics>
## Specific Ideas

### Alert rule shortlist (final list at planner discretion within these brackets)

Critical (page tier):

| Alert | Expression sketch | `for:` | Severity |
|---|---|---|---|
| PdbPlusSyncFreshnessHigh | `max(pdbplus_sync_freshness_seconds) > 7200` (2h) | 5m | critical |
| PdbPlusSyncFailureRateHigh | `sum(rate(pdbplus_sync_operations_total{status="failed"}[15m])) / sum(rate(pdbplus_sync_operations_total[15m])) > 0.5` | 15m | critical |
| PdbPlusFleetMachineCountLow | `count(go_memory_used_bytes{service_name="peeringdb-plus", go_memory_type="other"}) < 6` (expected 8, 2-machine grace) | 10m | critical |

Warning (notify tier):

| Alert | Expression sketch | `for:` | Severity |
|---|---|---|---|
| PdbPlusHeapHigh | `max(pdbplus_sync_peak_heap_bytes) > 419430400` (400 MiB; SEED-001 trigger threshold) | 30m | warning |
| PdbPlusRssHigh | `max(pdbplus_sync_peak_rss_bytes) > 402653184` (384 MiB) | 30m | warning |
| PdbPlusSyncOperationFailed | `increase(pdbplus_sync_operations_total{status="failed"}[1h]) > 0` | 5m | warning |

Planner may adjust thresholds/durations within reason (e.g. tighten freshness to 30m if the 15-min cadence justifies it). Add or omit one rule each side if it improves signal quality. Keep total rule count ≤8 to stay below GC's free-tier alertmanager limits.

### File layout

```
deploy/grafana/alerts/
├── README.md                 # apply mechanism, mimirtool examples, NO stack URLs
└── pdbplus-alerts.yaml       # standard Prom rule groups schema
```

Dashboard JSON edit: re-source panel 35 (`Peak Heap by Process Group`) — change query, description, legendFormat, and `targets[].expr`. Validate with `jq` after edit.

### Tests

- `deploy/grafana/dashboard_test.go` already exists and validates dashboard JSON. Mirror it: add `deploy/grafana/alerts_test.go` that runs `promtool check rules` (via `os/exec` with skip-if-not-installed) on the YAML and asserts each rule has `severity` + `summary`/`description` annotations. The `promtool` binary is in the Go toolchain via `go tool`.

</specifics>

<canonical_refs>
## Canonical References

- Prometheus rule format: https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules/
- Mimir rule sync (`mimirtool rules sync`): https://grafana.com/docs/mimir/latest/manage/tools/mimirtool/
- OTel Go runtime contrib v0.68.0 (already in go.mod): https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/runtime
- OTel semconv v1.36.0 goconv namespace (the source of `go.memory.used` etc.): inspect at `/home/dotwaffle/go/pkg/mod/go.opentelemetry.io/otel/semconv/v1.36.0/goconv/`
- SEED-001: `.planning/seeds/SEED-001-incremental-sync-evaluation.md` — the heap-pressure watch this task wires alerts for
- CLAUDE.md "Sync observability" section — describes `pdbplus_sync_peak_{heap,rss}_bytes` semantics and SEED-001 trigger conditions

</canonical_refs>
