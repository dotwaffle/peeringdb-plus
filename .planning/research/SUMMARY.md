# Project Research Summary

**Project:** PeeringDB Plus v1.5 -- Tech Debt & Observability
**Domain:** Production observability dashboards and tech debt cleanup for an edge-deployed Go/OTel service
**Researched:** 2026-03-24
**Confidence:** HIGH

## Executive Summary

PeeringDB Plus v1.5 is a cleanup and observability milestone requiring zero new Go module dependencies. The existing OTel instrumentation pipeline (9 custom sync metrics, otelhttp HTTP metrics, Go runtime metrics via `runtime.Start()`) already emits everything needed. The core deliverable is making these metrics visible through Grafana dashboards backed by Prometheus scraping, alongside clearing accumulated tech debt (dead code, stale documentation) and verifying 26 deferred human-testing items from prior milestones.

The recommended approach leverages Fly.io's managed Grafana (v10.4) and built-in Prometheus scraping. The Prometheus metrics endpoint is enabled entirely through environment variables (`OTEL_METRICS_EXPORTER=prometheus`) -- the `autoexport` package already in `go.mod` handles everything. Dashboard JSON files are authored in Grafana's UI, exported, parameterized for datasource portability, and committed to the repo under `deploy/grafana/dashboards/`. This is a configuration-and-verification milestone, not a feature-development milestone; the only Go code change is deleting the `WorkerConfig.IsPrimary` dead field from `internal/sync/worker.go`.

The primary risks are operational, not technical. The most critical pitfall is Grafana datasource UID coupling -- exported dashboard JSON contains instance-specific UIDs that break portability unless parameterized with template variables or deterministic UIDs. The OTel-to-Prometheus metric name translation (dots to underscores, unit suffixes, counter `_total` suffixes) is a second source of confusion when writing PromQL queries. The `meta.generated` field used for incremental sync cursors is undocumented by PeeringDB and must be empirically verified against the live API, though the existing fallback code is already defensive. All risks have clear prevention strategies documented in the research.

## Key Findings

### Recommended Stack

No new dependencies. Every capability needed is present in the existing dependency tree or achievable through configuration changes. See [STACK.md](STACK.md) for full details.

**Core technologies (already present):**
- **`autoexport` (v0.67.0):** Setting `OTEL_METRICS_EXPORTER=prometheus` enables a standalone HTTP server on port 9091 exposing all OTel metrics in Prometheus format. Zero code changes.
- **Hand-written Grafana JSON:** Classic dashboard model (pre-v12.2 schema). Designed in Grafana UI, exported, version-controlled. The Grafana Foundation SDK was evaluated and explicitly rejected -- it is "public preview" and overkill for 1-3 static dashboards.
- **fly.toml `[metrics]` section:** Tells Fly.io's scraper where to find the Prometheus endpoint. Two lines of config.

**Explicit non-additions:** No Grafana Foundation SDK, no `grafana-tools/sdk`, no `prometheus/client_golang` as direct dependency, no OTel Collector sidecar, no Grafonnet/Jsonnet, no new `go get` additions of any kind.

### Expected Features

See [FEATURES.md](FEATURES.md) for complete feature landscape and metrics inventory.

**Must have (table stakes):**
- Sync health dashboard row (duration, success rate, freshness gauge)
- HTTP RED metrics row (rate, errors, duration by route)
- Go runtime metrics row (goroutines, memory, GC)
- Per-type sync metrics row (13 PeeringDB types broken down)
- Dashboard JSON provisioning in version control
- `meta.generated` field graceful handling verification
- Dead code removal (`WorkerConfig.IsPrimary`)
- Deferred human verification pass (26 items across v1.2-v1.4)

**Should have (differentiators):**
- Business metrics row (object counts per type as observable gauges)
- Fly.io region breakdown in HTTP panels
- Sync fallback tracking panel
- Data freshness alerting thresholds (green/yellow/red)
- Dashboard documentation text panels
- Annotation markers for sync events

**Defer (v2+):**
- SLO/SLI tracking (premature without production baseline data)
- Per-endpoint latency breakdown for all API surfaces (panel sprawl)
- Custom alerting rules (requires notification channel)
- Automated visual regression testing (not worth it for one-time verification items)
- Real-time dashboard streaming (marginal value with hourly syncs)

### Architecture Approach

The architecture adds no new components to the Go application. The Prometheus metrics endpoint is activated purely through environment variables consumed by the existing `autoexport` package. Dashboard JSON files live in `deploy/grafana/dashboards/` as deployment artifacts, not application code. Dead code removal touches `internal/sync/worker.go` only. The `meta.generated` verification is an investigation with possible minor updates to `internal/peeringdb/client.go`. See [ARCHITECTURE.md](ARCHITECTURE.md) for full details.

**Major components:**
1. **Prometheus metrics endpoint (config-only)** -- `OTEL_METRICS_EXPORTER=prometheus` on port 9091. Fly.io scrapes every 15s. All existing custom and automatic metrics exposed automatically.
2. **Dashboard JSON files (`deploy/grafana/dashboards/`)** -- 1-4 JSON files with PromQL queries against Prometheus-formatted metric names. Classic schema, datasource template variables, stable UIDs.
3. **`WorkerConfig.IsPrimary` removal** -- Delete dead bool field from struct in `internal/sync/worker.go`. No other code references it.
4. **`meta.generated` verification** -- Test live PeeringDB API responses for field presence across depth=0, paginated, and incremental request patterns. Document findings.
5. **Human verification checklist** -- 26 items verified against live Fly.io deployment in dependency order.

### Critical Pitfalls

See [PITFALLS.md](PITFALLS.md) for all 12 pitfalls with detailed prevention strategies.

1. **Datasource UID coupling in exported dashboard JSON (Critical #1).** Exported dashboards contain instance-specific UIDs causing "No data" errors on import elsewhere. Use `${datasource}` template variable or deterministic UID provisioning. Never commit raw exported JSON without stripping UIDs.

2. **OTel-to-Prometheus metric name translation (Critical #2).** OTel names like `pdbplus.sync.duration` become `pdbplus_sync_duration_seconds_bucket` in Prometheus. All PromQL queries must use Prometheus-format names. The full mapping is documented in both STACK.md and PITFALLS.md.

3. **`meta.generated` field is undocumented and may be absent (Critical #3).** The field is not in PeeringDB's API spec. Current fallback code is defensive (zero time triggers `started_at - 5min` buffer), but behavior must be verified empirically. Risk is loss of incremental sync precision, not data loss.

4. **Dashboard panel sprawl (Moderate #5).** 9 custom metrics + otelhttp + runtime, each sliceable by type/status/region, can balloon to 50+ panels. Limit to 4 focused rows with 15-20 panels total. Use template variables for drill-down.

5. **Stale planning docs on dead code (Moderate #4).** Phase 7 summary incorrectly claims `WorkerConfig.IsPrimary` was removed. DataLoader middleware IS already gone. Verify codebase state with grep before writing removal tasks.

## Implications for Roadmap

Based on research, suggested phase structure with 4 phases:

### Phase 1: Tech Debt Cleanup

**Rationale:** Pure deletion and documentation fixes. No external dependencies, no deployment needed. Clears the deck for observability work. The dead code confuses future maintainers and the stale docs mislead planners.

**Delivers:** Removal of `WorkerConfig.IsPrimary` from `internal/sync/worker.go`. Updated planning documentation to reflect that DataLoader was already removed in v1.2. Codebase audit confirming no other dead code.

**Addresses features:** Dead code removal (table stake).

**Avoids pitfalls:** #4 (stale planning docs -- verify codebase state first), #6 (middleware removal patterns -- trace effects before deleting, though DataLoader is already gone).

### Phase 2: meta.generated Field Verification

**Rationale:** This is an investigation phase that informs sync pipeline correctness. Must happen before observability because we need to understand whether the freshness gauge's backing data is reliable. Independent of dashboard work.

**Delivers:** Empirical documentation of `meta.generated` behavior across all request patterns (depth=0 full fetch, paginated incremental, empty result sets). Updated code comments. Possible minor fix to `parseMeta()` or sync worker fallback logic. Verification that incremental sync cursor tracking is correct.

**Addresses features:** meta.generated graceful handling (table stake).

**Avoids pitfalls:** #3 (undocumented field behavior -- test all 3 request patterns against live API).

### Phase 3: Prometheus Metrics & Grafana Dashboards

**Rationale:** Requires deployment to Fly.io to verify metrics scraping works. Dashboard authoring requires live metrics data in Prometheus. This is the largest phase but has well-documented patterns and zero Go code changes.

**Delivers:** fly.toml `[metrics]` section and env var configuration. Prometheus endpoint on port 9091 verified on Fly.io. Dashboard JSON with 4 rows: Sync Health, HTTP RED, Per-Type Sync Detail, Go Runtime. Provisioning config. Documentation of OTel-to-Prometheus metric name mapping.

**Addresses features:** All 4 dashboard rows (table stakes), dashboard JSON provisioning, freshness alerting thresholds, sync fallback tracking, Fly.io region breakdown (differentiators).

**Avoids pitfalls:** #1 (datasource UID -- use template variables), #2 (metric name translation -- use documented Prometheus names), #5 (panel sprawl -- 4 rows, 15-20 panels max), #8 (JSON schema version -- classic model for Grafana 10.4), #9 (otelhttp semantic convention names), #10 (runtime metric names), #11 (stable UID, null version/id).

### Phase 4: Deferred Human Verification

**Rationale:** Requires a live Fly.io deployment with working metrics (post-Phase 3). The 26 items span CI pipeline behavior, API key integration, and browser UX across 7 phases of prior milestones. Must be verified in dependency order. Most time-consuming phase but lowest technical risk.

**Delivers:** Structured verification report with pass/fail/blocked status for all 26 items. Bug fixes for any items that fail. Updated milestone audit documentation.

**Addresses features:** Deferred human verification pass (table stake).

**Avoids pitfalls:** #7 (cross-environment browser differences -- test on Chrome/Firefox/Safari minimum), #12 (verify in dependency order: infrastructure, data layer, foundation, search, detail pages, comparison, polish).

### Phase Ordering Rationale

- **Dependencies are linear:** Dead code cleanup has no prerequisites. meta.generated verification is independent but informs sync correctness. Prometheus/dashboards require deployment. Human verification requires a fully deployed and observable system.
- **Risk profile is inverted:** The riskiest unknown (meta.generated behavior) is investigated early. The most time-consuming work (human verification) is deferred until all technical changes are deployed.
- **No Go code changes in Phase 3:** The Prometheus endpoint and dashboards are pure configuration and JSON. This means Phase 3 can proceed without affecting application stability.
- **Phase 4 is a gate, not a feature:** The 26 verification items from prior milestones must pass before v1.5 can be considered complete. They are grouped last because they depend on everything else being deployed.

### Research Flags

Phases likely needing deeper research during planning:
- **Phase 2 (meta.generated):** The field is undocumented. Live API testing is the only way to determine behavior. The research identifies 3 specific request patterns to test but cannot predict the results.
- **Phase 3 (Dashboards):** PromQL query patterns are well-documented, but the exact Prometheus metric names emitted by the current OTel SDK version should be verified against the actual `/metrics` endpoint after deployment. The autoexport Prometheus integration has not been tested in this project before.

Phases with standard patterns (skip research-phase):
- **Phase 1 (Tech Debt):** Pure code deletion. Codebase state already verified by PITFALLS.md research. One struct field removal.
- **Phase 4 (Verification):** Manual testing checklist. The verification items are already fully enumerated in FEATURES.md with specific steps and expected behaviors. The dependency ordering is documented in PITFALLS.md.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Zero new dependencies. All capabilities verified against existing go.mod. autoexport Prometheus support confirmed in package documentation. Grafana delivery approach validated against Fly.io managed Grafana version (v10.4). |
| Features | HIGH | Feature landscape derived from existing metrics inventory (already emitting), Grafana best practices, and codebase audit. Table stakes are unambiguous -- every metric is already recorded, just not visualized. |
| Architecture | HIGH | No new architectural components. Prometheus endpoint is config-only. Dashboard files are deployment artifacts. Dead code removal verified against actual codebase with grep. |
| Pitfalls | HIGH | All 12 pitfalls sourced from official Grafana docs, OTel spec, Grafana community forums, and direct codebase verification. The datasource UID and metric name translation pitfalls are the most commonly reported issues in Grafana provisioning. |

**Overall confidence:** HIGH

### Gaps to Address

- **meta.generated actual behavior:** Cannot be determined from documentation alone. Must test against `beta.peeringdb.com` or production PeeringDB API during Phase 2. The research provides the test plan but not the results.
- **Exact Prometheus metric names from current OTel SDK:** The documented mapping follows the OTel spec, but the runtime instrumentation library has changed metric names between versions. Verify against the actual `/metrics` endpoint after enabling Prometheus export in Phase 3.
- **Fly.io Grafana current version:** Research identifies v10.4 from a March 2024 community post. It may have been upgraded since. Verify before authoring dashboard JSON. Stick to classic JSON model regardless (compatible with Grafana 9+).
- **OTLP metric export trade-off:** Setting `OTEL_METRICS_EXPORTER=prometheus` disables OTLP metric push. If OTLP push is also needed (e.g., Grafana Cloud), use comma-separated `otlp,prometheus`. For Fly.io managed Grafana alone, `prometheus` is sufficient. Decision needed during Phase 3 planning.

## Sources

### Primary (HIGH confidence)
- [Fly.io Metrics Documentation](https://fly.io/docs/monitoring/metrics/) -- custom metrics setup, managed Grafana, Prometheus scraping
- [OTel Autoexport Package](https://pkg.go.dev/go.opentelemetry.io/contrib/exporters/autoexport) -- OTEL_METRICS_EXPORTER=prometheus support
- [OTel Prometheus Compatibility Spec](https://opentelemetry.io/docs/specs/otel/compatibility/prometheus_and_openmetrics/) -- metric name translation rules
- [OTel HTTP Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/http/http-metrics/) -- otelhttp metric names
- [Grafana Dashboard Best Practices](https://grafana.com/docs/grafana/latest/dashboards/build-dashboards/best-practices/) -- panel count, layout recommendations
- [Grafana Provisioning Documentation](https://grafana.com/docs/grafana/latest/administration/provisioning/) -- datasource UID patterns, JSON model

### Secondary (MEDIUM confidence)
- [Fly.io Grafana v10.4 Upgrade](https://community.fly.io/t/fly-metrics-grafana-upgraded-to-v10-4/18823) -- managed Grafana version (may be outdated)
- [OTel Go Runtime Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/runtime/go-metrics/) -- runtime metric names
- [PeeringDB Issue #776](https://github.com/peeringdb/peeringdb/issues/776) -- evidence of meta.generated field presence

### Tertiary (LOW confidence)
- meta.generated field behavior for depth=0 requests -- undocumented, needs empirical verification
- Exact runtime metric names for current OTel instrumentation version -- may differ from semantic conventions

### Codebase (verified against source)
- `internal/otel/metrics.go` -- 9 custom sync metrics with OTel names and units
- `internal/otel/provider.go` -- autoexport setup, runtime instrumentation, resource attributes
- `internal/peeringdb/client.go` -- FetchAll with meta.generated parsing
- `internal/sync/worker.go` -- WorkerConfig.IsPrimary dead field at line 30
- `cmd/peeringdb-plus/main.go` -- middleware chain, readiness middleware

---
*Research completed: 2026-03-24*
*Ready for roadmap: yes*
