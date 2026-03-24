# Feature Landscape

**Domain:** Production observability dashboards and tech debt cleanup for a Go/OTel edge-deployed service
**Researched:** 2026-03-24
**Milestone context:** v1.5 Tech Debt & Observability -- adding Grafana dashboard provisioning, verifying meta.generated field behavior, removing dead code, and verifying 26 deferred human-verification items against live Fly.io deployment
**Existing:** Full OTel pipeline (traces + metrics + logs) with autoexport, 9 custom sync metrics, Go runtime metrics via `go.opentelemetry.io/contrib/instrumentation/runtime`, otelhttp middleware on all HTTP requests, Fly.io deployment with LiteFS replication

## Table Stakes

Features that production Go/OTel services must have for effective monitoring. Missing = operating blind in production.

| Feature | Why Expected | Complexity | Dependencies | Notes |
|---------|--------------|------------|--------------|-------|
| Sync health dashboard row | The sync pipeline is the core data path. Without visibility into sync duration, success/failure rate, and freshness, operators cannot tell if the service is delivering current data. Every production data pipeline has a sync/ETL health dashboard. | Low | Existing metrics: `pdbplus.sync.duration`, `pdbplus.sync.operations`, `pdbplus.sync.freshness` | Three panels minimum: sync duration over time (histogram), sync success/failure rate (counter), data freshness gauge (seconds since last sync with threshold alert). All metrics already recorded. |
| HTTP RED metrics row (Rate, Errors, Duration) | The RED method is the standard for request-driven services. otelhttp middleware already emits `http.server.request.duration`, `http.server.active_requests`, and status codes. Not visualizing these means no awareness of API health. | Low | Existing otelhttp middleware metrics, no new instrumentation needed | Rate: `rate(http.server.request.duration_count)`. Errors: filter by `http.response.status_code >= 400`. Duration: p50/p95/p99 from histogram. Break down by route pattern (`http.route`). |
| Go runtime metrics row | Go runtime metrics (goroutines, memory, GC) are already collected via `runtime.Start()` in provider.go. Not visualizing them means no visibility into memory leaks, goroutine leaks, or GC pressure. Standard for any production Go service. | Low | Existing `go.opentelemetry.io/contrib/instrumentation/runtime` already initialized in provider.go | Key panels: goroutine count (`go.goroutine.count`), heap memory used (`go.memory.used`), allocation rate (`go.memory.allocated`), GC goal (`go.memory.gc.goal`). All automatically emitted. |
| Per-type sync metrics row | 13 PeeringDB types are synced independently. Knowing which type is slow, failing, or producing errors is essential for debugging sync issues. Per-type metrics already exist. | Low | Existing metrics: `pdbplus.sync.type.duration`, `pdbplus.sync.type.objects`, `pdbplus.sync.type.deleted`, `pdbplus.sync.type.fetch_errors`, `pdbplus.sync.type.upsert_errors`, `pdbplus.sync.type.fallback` | Filter/group by `type` attribute. Show as stacked bar or heatmap for duration, table for counts. All metrics already recorded with `type` attribute (net, ix, fac, etc.). |
| Dashboard JSON provisioning | Dashboards stored as JSON files in the repository enable version control, code review, and reproducible deployments. Grafana file provisioning is the standard approach for infrastructure-as-code. Not provisioning means dashboards are ephemeral and lost on Grafana restart. | Low | Grafana provisioning YAML config file + dashboard JSON file(s) | Single `dashboards.yaml` provider config pointing to a directory of JSON files. Each dashboard is a self-contained JSON file exportable from Grafana UI. |
| meta.generated field graceful handling | The sync pipeline uses `meta.generated` for incremental sync cursor timestamps. If this field is absent for depth=0 full-sync responses, the fallback to `started_at - 5min` must work correctly. Without verification, incremental sync could silently use wrong cursors. | Low | Existing `parseMeta()` in client.go already returns zero time if field is absent | Verify empirically against beta.peeringdb.com whether depth=0 no-pagination responses include `meta.generated`. Current code already handles absence gracefully (falls back to zero time, worker uses `started_at - 5min`). |
| Dead code removal (DataLoader, IsPrimary) | Dead code creates confusion for future maintainers and triggers linter warnings. DataLoader middleware is wired but unused (entgql handles N+1 natively). WorkerConfig.IsPrimary is a vestigial field replaced by LiteFS detection. Both were explicitly deferred from earlier milestones. | Low | No dependencies -- pure deletion | DataLoader: remove middleware registration. IsPrimary: remove field from WorkerConfig struct, update constructor callers. Both changes are additive (deleting unused code). |
| Deferred human verification pass | 26 items were deferred from v1.2-v1.4 because they require a running browser/deployment to verify. These cover CI pipeline behavior, API key integration, and UI/UX quality. Without verification, we are shipping untested user-facing behavior. | Med | Live Fly.io deployment, browser access, PeeringDB API key for 3 items | 6 items from v1.2/v1.3 (CI, API key). 20 items from v1.4 (visual/browser UX). Must be verified against live deployment, not in CI. |

## Differentiators

Features that elevate monitoring beyond basics. Not strictly required for production, but make the service significantly more operable.

| Feature | Value Proposition | Complexity | Dependencies | Notes |
|---------|-------------------|------------|--------------|-------|
| Business metrics row (object counts per type) | Showing total object counts per PeeringDB type over time reveals data completeness and growth trends. "We have 85,000 networks and growing" is a business KPI, not just a technical metric. No competing PeeringDB mirror exposes this visibility. | Low-Med | New gauge metric or compute from sync type objects counter. Could also query SQLite count directly via observable gauge. | Sum of `pdbplus.sync.type.objects` per type gives the count synced per cycle. Alternatively, register new observable gauges that count rows per type in SQLite. The observable gauge pattern (already used for freshness) is clean. |
| Fly.io region breakdown | The service runs on edge nodes across multiple Fly.io regions. Breaking down HTTP metrics by `fly.region` resource attribute shows which regions are serving traffic and whether any region has degraded performance. | Low | Resource attribute `fly.region` already emitted in provider.go via `FLY_REGION` env var | Group HTTP panels by `fly.region`. Useful for spotting region-specific issues. No new instrumentation needed -- just dashboard panel configuration. |
| Sync fallback tracking panel | Incremental-to-full sync fallback events (`pdbplus.sync.type.fallback`) indicate that incremental sync failed for a type. Tracking these over time reveals whether specific types consistently cause fallback, suggesting PeeringDB API instability for those types. | Low | Existing `pdbplus.sync.type.fallback` counter | Single panel: fallback events over time grouped by type. If this is consistently non-zero, investigate that type's API behavior. |
| Data freshness alerting threshold | A gauge panel with color thresholds (green < 1h, yellow 1-2h, red > 2h) for `pdbplus.sync.freshness` gives instant visual health status. The threshold values match the hourly sync schedule. | Low | Existing `pdbplus.sync.freshness` observable gauge | Grafana stat panel with value mappings or threshold colors. No new metrics. |
| Dashboard documentation panels | Grafana text panels at the top of each dashboard section explaining what the metrics mean and what actions to take. Reduces the "what am I looking at?" moment for on-call operators who are not the dashboard author. | Low | No dependencies -- just text panels in JSON | Add a text panel to each row explaining the metrics, expected ranges, and troubleshooting actions. Grafana best practice per official docs. |
| Annotation markers for sync events | Grafana annotations marking sync start/completion on timeline panels. Correlating metric changes with sync events is critical for root cause analysis ("latency spiked -- was a sync running?"). | Med | Requires publishing annotations from sync worker, either via Grafana API or by using OTel span events that map to annotations | Could implement via: (1) OTel trace spans already exist for sync -- configure Tempo-to-Grafana annotation mapping, (2) direct Grafana annotation API call from sync worker (adds HTTP dependency), or (3) log-based annotations via Loki. Option 1 is cleanest if using Tempo. |

## Anti-Features

Features to explicitly NOT build for this milestone.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| Grafonnet/Jsonnet dashboard generation | Grafonnet adds a Jsonnet toolchain dependency for generating dashboard JSON. The project has one dashboard with a fixed structure. Hand-authored JSON is simpler, easier to review in PRs, and eliminates the Jsonnet learning curve. Worth it for 50+ dashboards, not one. | Author dashboard JSON directly. Export from Grafana UI after visual editing, commit the JSON. |
| Custom alerting rules in dashboard provisioning | Grafana alerting rules have their own provisioning path separate from dashboards. Mixing alerting into dashboard provisioning creates coupling. Alerts also require a notification channel which is deployment-specific. | Define threshold colors on panels for visual alerting. Defer formal alerting rules to when a notification channel is established. |
| SLO/SLI tracking dashboard | SLO definitions require defining error budgets, burn rates, and alerting windows. Premature for a service not yet monitored in production. | Track RED metrics and sync freshness. After 2-4 weeks of production data, define SLOs based on observed patterns. |
| Per-endpoint latency breakdown for all API surfaces | 4 API surfaces with dozens of endpoints. Per-endpoint panels produces a dashboard too large to scan. | Use `http.route` aggregation in the RED row. Add drill-down panels only for endpoints that show problems. |
| Real-time dashboard streaming | Grafana supports streaming via live/websocket panels. Marginal value when sync runs hourly and metrics are scraped on 15-60 second intervals. | Standard 30-second refresh interval on Grafana dashboard auto-refresh. |
| Automated visual regression testing | Building Playwright/Cypress screenshot comparison for 20 UI verification items costs more than manually checking them once. One-time verification items. | Manually verify against live deployment. Consider automation only if web UI gets frequent changes. |

## Feature Dependencies

```
OTel metrics already emitted (sync, HTTP, runtime)
    |
    v
Grafana dashboard JSON (single file, multiple rows)
    |
    +--> Row 1: Sync Health
    |        |-- Sync duration histogram
    |        |-- Sync success/failure rate
    |        |-- Data freshness gauge with thresholds
    |        |-- Sync fallback events
    |
    +--> Row 2: HTTP RED Metrics
    |        |-- Request rate by route
    |        |-- Error rate (4xx, 5xx)
    |        |-- Latency percentiles (p50, p95, p99)
    |        |-- Active requests gauge
    |
    +--> Row 3: Per-Type Sync Detail
    |        |-- Duration heatmap by type
    |        |-- Object counts per type
    |        |-- Delete counts per type
    |        |-- Fetch/upsert errors per type
    |
    +--> Row 4: Go Runtime
    |        |-- Goroutine count
    |        |-- Heap memory used
    |        |-- Allocation rate
    |        |-- GC goal
    |
    +--> Provisioning config (dashboards.yaml)

meta.generated verification (independent of dashboard)
    |
    +--> Verify against beta.peeringdb.com
    +--> Update parseMeta/fallback logic if needed
    +--> Document findings

Dead code removal (independent)
    |
    +--> Remove DataLoader middleware registration
    +--> Remove WorkerConfig.IsPrimary field

Human verification (depends on live deployment)
    |
    +--> 6 items from v1.2/v1.3 (CI, API key)
    +--> 20 items from v1.4 (visual/browser UX)
```

## Metrics Inventory (Already Emitting)

### Custom Application Metrics (meter: "peeringdb-plus")

| Metric Name | Type | Unit | Attributes | Description |
|-------------|------|------|------------|-------------|
| `pdbplus.sync.duration` | Float64Histogram | s | `status` (success/failed) | Duration of full sync operations |
| `pdbplus.sync.operations` | Int64Counter | {operation} | `status` (success/failed) | Count of sync operations |
| `pdbplus.sync.freshness` | Float64ObservableGauge | s | (none) | Seconds since last successful sync |
| `pdbplus.sync.type.duration` | Float64Histogram | s | `type` (net/ix/fac/...) | Per-type sync step duration |
| `pdbplus.sync.type.objects` | Int64Counter | {object} | `type` | Objects synced per type |
| `pdbplus.sync.type.deleted` | Int64Counter | {object} | `type` | Objects deleted per type |
| `pdbplus.sync.type.fetch_errors` | Int64Counter | {error} | `type` | PeeringDB API fetch errors per type |
| `pdbplus.sync.type.upsert_errors` | Int64Counter | {error} | `type` | Database upsert errors per type |
| `pdbplus.sync.type.fallback` | Int64Counter | {event} | `type` | Incremental-to-full fallback events |

### otelhttp Middleware Metrics (automatic)

| Metric Name | Type | Attributes | Description |
|-------------|------|------------|-------------|
| `http.server.request.duration` | Float64Histogram | `http.request.method`, `http.route`, `http.response.status_code`, `server.address` | Incoming request duration |
| `http.server.active_requests` | Int64UpDownCounter | `http.request.method`, `server.address` | Active concurrent requests |
| `http.server.request.body.size` | Int64Histogram | (same as duration) | Request body size |
| `http.server.response.body.size` | Int64Histogram | (same as duration) | Response body size |

### otelhttp Transport Metrics (PeeringDB client, automatic)

| Metric Name | Type | Attributes | Description |
|-------------|------|------------|-------------|
| `http.client.request.duration` | Float64Histogram | `http.request.method`, `server.address`, `http.response.status_code` | Outbound PeeringDB API request duration |

### Go Runtime Metrics (automatic via `runtime.Start()`)

| Metric Name | Type | Description |
|-------------|------|-------------|
| `go.goroutine.count` | Int64UpDownCounter | Live goroutine count |
| `go.memory.used` | Int64UpDownCounter | Memory used by Go runtime |
| `go.memory.allocated` | Int64Counter | Total memory allocated to heap |
| `go.memory.allocations` | Int64Counter | Count of heap allocations |
| `go.memory.gc.goal` | Int64UpDownCounter | Heap size target for end of GC cycle |
| `go.processor.limit` | Int64UpDownCounter | Number of OS threads for Go code |
| `go.config.gogc` | Int64UpDownCounter | GOGC configuration value |

## meta.generated Field Analysis

### What We Know (HIGH confidence)

1. **Response structure**: PeeringDB API responses have `{"meta": {...}, "data": [...]}` structure. The `meta` field is a JSON object, and the `generated` field is inside it as a float64 Unix epoch timestamp (e.g., `1595250699.701`).

2. **Cached responses include it**: GitHub issue #776 shows the `generated` field present in full-list responses. Two sequential requests returned different `generated` values, confirming it represents cache generation timestamp.

3. **The value is a float64 epoch**: Sub-second precision. Current `parseMeta()` truncates to integer seconds via `time.Unix(int64(meta.Generated), 0)`. Acceptable for cursor purposes.

4. **Current code handles absence gracefully**: `parseMeta()` returns zero time if meta is empty, meta has no `generated` field, or `generated` is zero. The sync worker falls back to `started_at - 5min` buffer when `Generated` is zero.

### What Is Unverified (needs empirical testing)

1. **depth=0 with no pagination (full sync path)**: Does this serve the cached response (with `meta.generated`) or a live query (without it)?
2. **depth=0 with `?since=` (incremental path)**: Does this serve from cache or live?
3. **Consistency across all 13 types**: Does `meta.generated` behave the same for all types?

### Impact Assessment

The current code is defensively written to handle all scenarios. This verification is about confirming behavior and documenting it, not about finding bugs to fix.

## Deferred Human Verification Items (Complete Inventory)

### From v1.2 (3 items)

| Item | Source Phase | What to Verify | How |
|------|-------------|----------------|-----|
| CI workflow execution on GitHub Actions | Phase 10 | Workflow runs on push, all 4 jobs pass | Push a commit, check GitHub Actions tab |
| Coverage comment posting on PRs | Phase 10 | Coverage comment appears on PR with percentage | Create a PR, check for coverage comment |
| Comment deduplication | Phase 10 | Only one coverage comment per PR (updated, not duplicated) | Push multiple commits to same PR, check comments |

### From v1.3 (3 items)

| Item | Source Phase | What to Verify | How |
|------|-------------|----------------|-----|
| Live CLI with real API key | Phase 12 | `pdbcompat-check --api-key <key>` works | Run CLI with real PeeringDB API key |
| Live integration test with real API key | Phase 12 | `-peeringdb-live` test passes with API key | `PDBPLUS_PEERINGDB_API_KEY=<key> go test -peeringdb-live` |
| Invalid key rejection | Phase 12 | Invalid API key produces WARN log, not crash | Run with `--api-key invalid-key-here` |

### From v1.4 (20 items)

| Item | Source Phase | What to Verify | How |
|------|-------------|----------------|-----|
| Neon green on dark theme | Phase 13 | Visual appearance of accent color | Browser, toggle dark mode |
| Responsive layout at breakpoints | Phase 13 | Layout adapts at mobile/tablet/desktop | Browser, resize window |
| Syncing page animation | Phase 13 | Loading animation before first sync | Deploy fresh, visit before sync completes |
| Content negotiation | Phase 13 | Browser gets redirect, API client gets JSON | `curl -H 'Accept: text/html'` vs `curl` at root |
| Live search latency | Phase 14 | Results update within 300ms | Browser, type in search box, observe timing |
| Type badge colors | Phase 14 | Colored badges distinguish entity types | Browser, search for multi-type term |
| ASN redirect on Enter | Phase 14 | Typing "13335" + Enter goes to Cloudflare detail | Browser, search box |
| Collapsible sections | Phase 15 | Expand/collapse smoothly | Browser, click section headers on detail pages |
| Lazy loading triggers | Phase 15 | Content loads only on first expand | Browser, expand section, check network tab |
| Summary stats visible | Phase 15 | IX count, fac count shown in header | Browser, visit network detail page |
| Cross-links navigate | Phase 15 | Click IX name from network -> IX detail | Browser, navigate cross-links |
| Comparison results layout | Phase 16 | Shared IXPs/facilities display correctly | Browser, compare two ASNs |
| View toggle | Phase 16 | Shared-only vs full side-by-side works | Browser, toggle on comparison page |
| Multi-step compare flow | Phase 16 | "Compare with..." button -> compare page | Browser, network detail -> compare |
| Dark mode toggle | Phase 17 | Toggle works, system preference detected | Browser, toggle, check system preference |
| Keyboard navigation | Phase 17 | Arrow keys navigate search results | Browser, search, use arrow keys |
| CSS animations | Phase 17 | FadeIn on search results, transitions smooth | Browser, search, observe animations |
| Loading indicators | Phase 17 | htmx loading bar visible during requests | Browser, trigger htmx request, observe bar |
| Styled error pages | Phase 17 | 404 has search box, 500 has home link | Browser, visit invalid URL |
| About page freshness | Phase 17 | About page shows data age indicator | Browser, visit /ui/about |

## MVP Recommendation

**Phase ordering rationale:** Dashboard provisioning and meta.generated verification are independent of each other and of the deferred verification items. Dead code removal is trivial and can be grouped with either.

Prioritize:
1. **meta.generated verification + dead code removal** -- Quick investigation followed by removing DataLoader middleware and WorkerConfig.IsPrimary. Low risk, clears tech debt.
2. **Grafana dashboard JSON provisioning** -- Build the dashboard JSON with 4 rows. No new Go code needed for table-stakes panels.
3. **Deferred human verification** -- Verify all 26 items against the live Fly.io deployment. Most time-consuming but lowest risk.

Defer:
- **Business metrics (object count gauges)**: Requires new observable gauge instruments.
- **Annotation markers**: Requires OTel Collector or Tempo annotation mapping.
- **SLO definitions**: Premature without production baseline data.

## Sources

- [Grafana Dashboard Best Practices](https://grafana.com/docs/grafana/latest/visualizations/dashboards/build-dashboards/best-practices/)
- [Grafana Provisioning Documentation](https://grafana.com/docs/grafana/latest/administration/provisioning/)
- [OpenTelemetry for HTTP Services Dashboard](https://grafana.com/grafana/dashboards/21587-opentelemetry-for-http-services/)
- [Go Runtime Metrics OTel Dashboard](https://grafana.com/grafana/dashboards/22035-go-runtime/)
- [OTel Go Runtime Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/runtime/go-metrics/)
- [PeeringDB Issue #776](https://github.com/peeringdb/peeringdb/issues/776) -- Evidence of `meta.generated` field
- Codebase: `internal/otel/metrics.go`, `internal/otel/provider.go`, `internal/peeringdb/client.go`
