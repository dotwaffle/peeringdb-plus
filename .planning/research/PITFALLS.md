# Domain Pitfalls

**Domain:** Tech Debt Cleanup, Observability Dashboards, and Deferred Verification for Existing Go Service
**Researched:** 2026-03-24
**Milestone:** v1.5 Tech Debt & Observability

## Critical Pitfalls

Mistakes that cause data loss, production incidents, or require significant rework.

### Pitfall 1: Grafana Dashboard JSON Becomes Unmaintainable -- Datasource UID Coupling

**What goes wrong:** The dashboard JSON is exported from a Grafana instance and committed to version control. The JSON contains hardcoded datasource UIDs that are specific to the Grafana instance where the dashboard was created. When the dashboard is provisioned into a different Grafana instance (or re-provisioned after a Grafana reinstall), the datasource UIDs do not match, causing every panel to show "No data" or "Datasource not found" errors. The dashboard appears broken despite the data being available.

**Why it happens:** Grafana assigns unique UIDs to datasources per instance. Since version 8.3.0, exported dashboard JSON contains these UIDs embedded in every panel's `datasource` field. Developers export the dashboard from their local Grafana, commit it, and assume it will work everywhere. It works on their instance and breaks on everyone else's.

**Consequences:**
- Dashboard provisioning fails silently -- panels render but show no data, which looks like a metrics issue rather than a configuration issue.
- Debugging takes hours because the error is not "datasource missing" but "no data returned" (the datasource reference is resolved but points to a UID that does not exist or resolves to the wrong datasource).
- Every environment (dev, staging, production) needs manual datasource UID patching, defeating the purpose of dashboards-as-code.

**Prevention:**
- Use datasource template variables instead of hardcoded UIDs. Define a Grafana variable `$datasource` of type "Datasource" and reference it in all panels as `"datasource": {"uid": "${datasource}"}`. This lets the user select the appropriate datasource per environment.
- Alternatively, provision the datasource with a deterministic UID (e.g., `"uid": "prometheus"`) in the datasource provisioning YAML, and reference that exact UID in the dashboard JSON. This couples the dashboard to the datasource provisioning config (which is fine if both are version-controlled together).
- Never export a dashboard from the Grafana UI and commit the raw JSON without stripping or parameterizing datasource UIDs. Use `jq` or a script to replace UIDs with template variables before committing.
- Validate that the dashboard JSON loads cleanly in a fresh Grafana instance as part of a CI check or manual verification step.

**Detection:** Dashboard panels show "No data" or "Panel plugin not found" errors. The Grafana provisioning log shows warnings about datasource resolution.

**Confidence:** HIGH -- multiple Grafana community threads confirm this as the most common provisioning mistake. ([Grafana Community](https://community.grafana.com/t/should-provisioned-dashboards-have-datasource-uids/65463), [Grafana Docs](https://grafana.com/docs/grafana/latest/administration/provisioning/))

---

### Pitfall 2: OTel Metric Names Silently Change When Exported to Prometheus -- Dashboard Queries Break

**What goes wrong:** The application defines OTel metrics with names like `pdbplus.sync.duration` and `pdbplus.sync.type.objects`. When these metrics are exported via the OTLP-to-Prometheus pipeline (which is how most Grafana dashboards consume them), the metric names are transformed: dots become underscores, unit suffixes are appended, and counter types get a `_total` suffix. So `pdbplus.sync.duration` (histogram with unit "s") becomes `pdbplus_sync_duration_seconds_bucket` in Prometheus. The dashboard author writes PromQL queries against the OTel names and gets no results.

**Why it happens:** The OpenTelemetry Prometheus compatibility specification defines a naming translation: dots to underscores, unit suffix appended (e.g., `_seconds` for `"s"`), counter suffix `_total` appended, histogram suffixes `_bucket`/`_count`/`_sum` appended. The developer writes the dashboard queries using the OTel SDK metric names (which appear in the code) rather than the Prometheus-exported names.

**Consequences:**
- Every PromQL query in the dashboard returns empty results.
- Developer assumes metrics are not being emitted and starts debugging the application telemetry pipeline.
- If the developer discovers the naming translation and manually translates names, the dashboard works but the mapping is fragile -- any change to the OTel metric definition (name, unit, type) requires updating the dashboard queries.

**Prevention:**
- Document the exact Prometheus metric names alongside the OTel metric definitions. For this project, the mapping is:

| OTel Name | OTel Type | OTel Unit | Prometheus Name |
|-----------|-----------|-----------|-----------------|
| `pdbplus.sync.duration` | Histogram | `s` | `pdbplus_sync_duration_seconds` (with `_bucket`, `_count`, `_sum`) |
| `pdbplus.sync.operations` | Counter | `{operation}` | `pdbplus_sync_operations_total` |
| `pdbplus.sync.type.duration` | Histogram | `s` | `pdbplus_sync_type_duration_seconds` |
| `pdbplus.sync.type.objects` | Counter | `{object}` | `pdbplus_sync_type_objects_total` |
| `pdbplus.sync.type.deleted` | Counter | `{object}` | `pdbplus_sync_type_deleted_total` |
| `pdbplus.sync.type.fetch_errors` | Counter | `{error}` | `pdbplus_sync_type_fetch_errors_total` |
| `pdbplus.sync.type.upsert_errors` | Counter | `{error}` | `pdbplus_sync_type_upsert_errors_total` |
| `pdbplus.sync.type.fallback` | Counter | `{event}` | `pdbplus_sync_type_fallback_total` |
| `pdbplus.sync.freshness` | Gauge | `s` | `pdbplus_sync_freshness_seconds` |

- Note: custom units like `{operation}`, `{object}`, `{error}`, `{event}` do NOT add a suffix per the OTel spec (only standard units like `s`, `ms`, `By` add suffixes). So `pdbplus.sync.operations` with unit `{operation}` becomes `pdbplus_sync_operations_total`, not `pdbplus_sync_operations_operation_total`.
- Write all dashboard PromQL queries against the Prometheus-exported names, not the OTel names.
- Test queries against the actual Prometheus endpoint before finalizing the dashboard JSON.
- Add a comment block at the top of the dashboard JSON (or in a README alongside it) that documents this mapping.

**Detection:** PromQL queries return empty results despite the application emitting metrics. Prometheus targets page shows the application as "UP" but metric explorer shows no `pdbplus_*` metrics (or shows them with unexpected names).

**Confidence:** HIGH -- the naming translation is specified in the [OTel Prometheus compatibility spec](https://opentelemetry.io/docs/specs/otel/compatibility/prometheus_and_openmetrics/) and is a documented behavior.

---

### Pitfall 3: meta.generated Field Is Undocumented and May Be Absent -- Sync Freshness Tracking Breaks

**What goes wrong:** The incremental sync system uses `meta.generated` from PeeringDB API responses to track data freshness. The code in `parseMeta()` extracts a `generated` epoch from the response meta field. If PeeringDB changes their API response structure, removes the field, or the field is absent for certain request patterns (e.g., `depth=0` without pagination, or specific object types), the sync worker records a zero timestamp. This causes the incremental sync cursor to reset, triggering unnecessary full re-fetches, or worse -- the sync considers data "stale" and constantly re-syncs.

**Why it happens:** The `meta.generated` field is **not documented** in the PeeringDB API specification. The official docs describe `meta` as containing `status` and `message` fields only. The `generated` epoch is an implementation detail of PeeringDB's Django REST Framework backend that may change without notice. The current code already has a graceful fallback (`parseMeta` returns zero time on absence), but the sync worker's behavior when `Generated` is zero has not been verified against the live API.

**Consequences:**
- If `generated` disappears: full sync always uses `started_at - 5min` as the cursor (the existing fallback), which is safe but means the incremental sync optimization provides less value.
- If `generated` format changes (e.g., string ISO timestamp instead of float epoch): `json.Unmarshal` into `float64` fails silently, `parseMeta` returns zero, same fallback behavior.
- If `generated` is present for paginated responses but absent for full fetches (depth=0 without limit/skip): the code path for full sync already handles this correctly (line 160 in client.go), but the incremental path tracks "earliest generated across pages" which may yield unexpected timestamps.

**Prevention:**
- Test `parseMeta()` against actual live PeeringDB API responses for each request pattern:
  1. Full fetch: `GET /api/net?depth=0` (no limit/skip)
  2. Paginated incremental: `GET /api/net?depth=0&limit=250&skip=0&since=...`
  3. Empty result set (all up to date): `GET /api/net?depth=0&since=<recent_timestamp>`
- Document the actual response structure observed, including whether `meta` is `{}`, `null`, absent entirely, or contains `generated`.
- Ensure the sync worker logs the meta.generated value (or its absence) at DEBUG level so behavior can be audited in production.
- The existing `parseMeta()` fallback behavior (return zero time on any parse failure) is defensive and correct. The risk is not a crash but a loss of incremental sync precision.
- Add a test that explicitly verifies the full sync path works correctly when `Meta.Generated.IsZero()` returns true.

**Detection:** Sync logs show `generated=0` or missing generated timestamp. Incremental sync falls back to full sync more often than expected. The `pdbplus.sync.type.fallback` counter increments unexpectedly.

**Confidence:** MEDIUM -- the field is real and observed in practice, but its contract is undocumented. Behavior may vary by object type or API version.

---

## Moderate Pitfalls

### Pitfall 4: Removing WorkerConfig.IsPrimary -- Stale Planning Docs Cause Confusion

**What goes wrong:** The PROJECT.md and planning documents reference "Remove unused DataLoader middleware and WorkerConfig.IsPrimary dead field" as active tech debt. However, investigation of the actual codebase reveals a mismatch: the DataLoader package was already removed in v1.2 Phase 7 (commit `ec182e1`), the `config.IsPrimary` field was removed from `config.go`, but **WorkerConfig.IsPrimary still exists** in `internal/sync/worker.go` at line 30. Meanwhile, the Phase 7 SUMMARY.md claims it was removed. A developer trusting the summary would skip the removal. A developer reading PROJECT.md would try to remove both DataLoader and IsPrimary, wasting time discovering DataLoader is already gone.

**Why it happens:** Documentation and code drifted apart. The Phase 7 plan called for removing `IsPrimary` from both `Config` and `WorkerConfig`, and the summary incorrectly reports both as removed, but the actual commit only removed it from `Config`. The field in `WorkerConfig` is truly dead -- `main.go` no longer sets it (line 132 creates `WorkerConfig{IncludeDeleted: cfg.IncludeDeleted, SyncMode: cfg.SyncMode}` without `IsPrimary`), and no code reads `w.config.IsPrimary`.

**Consequences:**
- Wasted time trying to remove code that is already gone (DataLoader).
- Wasted time not removing code that still exists (WorkerConfig.IsPrimary).
- If a future developer sees the field and assumes it should be used, they might wire it up incorrectly.
- The lint pass with `unused` linter should catch this, but struct fields are not flagged by the `unused` linter (only local variables and function parameters are).

**Prevention:**
- Verify the actual codebase state before planning tasks. Run `grep -rn "IsPrimary" --include="*.go"` and `grep -rn "dataloader\|DataLoader" --include="*.go"` to determine what actually needs removal.
- The task for v1.5 is: remove `IsPrimary bool` from `WorkerConfig` in `internal/sync/worker.go`. That is the entire change. The DataLoader task is already done.
- Update PROJECT.md to reflect that the DataLoader was removed in v1.2 Phase 7 and only WorkerConfig.IsPrimary remains.
- Use `staticcheck` or `fieldalignment` to detect truly unused struct fields in the future.

**Detection:** `grep -rn "IsPrimary" --include="*.go"` returns only the struct definition (line 30 of worker.go), no usages.

**Confidence:** HIGH -- verified directly against the codebase.

---

### Pitfall 5: Grafana Dashboard Panel Sprawl -- Too Many Panels Kill Load Time and Readability

**What goes wrong:** The dashboard author creates a separate panel for every metric and every dimension. With 9 custom sync metrics plus otelhttp metrics plus Go runtime metrics, each potentially sliced by `type` (13 PeeringDB types), `status`, `fly.region`, and `fly.machine_id`, the dashboard balloons to 50+ panels. Grafana renders all visible panels simultaneously, each firing its own PromQL query. On a free or shared Grafana instance, this causes timeout errors and 10+ second load times.

**Why it happens:** The temptation is to build "the one dashboard that shows everything." Every metric that exists gets a panel. Every label dimension gets a separate graph. The dashboard becomes a data dump rather than an operational tool.

**Consequences:**
- Dashboard takes 10+ seconds to load, users stop using it.
- Alert fatigue: too many panels means no panel is important.
- Maintenance burden: changing a metric name requires updating dozens of panels.
- Grafana performance degrades, especially on lower-tier instances.

**Prevention:**
- Limit to 4 focused rows, each with 3-5 panels, for a total of 15-20 panels maximum. Grafana official best practices recommend fewer than 20 panels per dashboard.
- Organize panels by operational concern, not by metric:
  1. **Sync Health row:** Last sync status, sync duration over time, freshness gauge, error rate
  2. **API Traffic row:** Request rate, error rate (4xx/5xx), latency p95/p99 (from otelhttp), requests by endpoint
  3. **Infrastructure row:** Go runtime goroutines, memory, GC pause (from runtime instrumentation), CPU
  4. **Business Metrics row:** Objects synced per type (stacked bar), objects deleted, type distribution
- Use template variables (`$type`, `$region`) to let users drill down rather than showing all dimensions at once. One graph with a type selector replaces 13 duplicate graphs.
- Use a separate "deep dive" dashboard linked from the overview dashboard for per-type or per-region analysis, rather than cramming everything into one.
- Review which metrics actually drive operational decisions. If nobody will page on it, it does not need a panel.

**Detection:** Dashboard load time exceeds 5 seconds. Users say "I never look at the dashboard." More than 25 panels exist.

**Confidence:** HIGH -- [Grafana best practices docs](https://grafana.com/docs/grafana/latest/dashboards/build-dashboards/best-practices/) explicitly recommend this.

---

### Pitfall 6: Removing "Unused" Middleware Without Checking All Request Paths

**What goes wrong:** The v1.5 milestone notes that "DataLoader middleware" is unused and should be removed. While the DataLoader is indeed already gone from the code, this pattern generalizes: when removing any middleware from the HTTP handler chain, developers check the direct import graph but miss indirect consumers. Middleware may be injected at a different layer (e.g., per-route vs. global), invoked conditionally (e.g., only on primary nodes), or relied upon by generated code that is not part of the main source tree.

**Why it happens:** Go's middleware pattern uses `http.Handler` wrapping, which makes the dependency chain invisible to static analysis tools. `grep` for direct function calls works, but middleware that injects values into `context.Context` has consumers that read from context -- the producer and consumer are decoupled.

**Consequences:**
- Removing middleware that injected context values causes nil pointer panics in handlers that read those values.
- Removing middleware that set response headers breaks CORS, caching, or security behavior.
- The failure may not appear in unit tests because tests often construct their own contexts.

**Prevention:**
- Before removing any middleware, trace its effects:
  1. Does it set any response headers? Search for `w.Header().Set` in the middleware source.
  2. Does it inject values into `context.Context`? Search for `context.WithValue` or typed context accessors.
  3. Does it modify the request? Search for `r.WithContext` or `r.Clone`.
  4. Is it referenced in generated code? Check `ent/rest/`, `graph/generated.go`, etc.
- For the specific v1.5 case, the DataLoader was already removed and the only remaining dead code is `WorkerConfig.IsPrimary` -- a struct field, not middleware. But the principle applies if any additional middleware cleanup is discovered.
- Run the full test suite with `-race` after removing any middleware. Specifically test the routes that were served through the removed middleware chain.
- Verify in production (or staging) by watching error rates after deployment.

**Detection:** Handlers panic with nil context value access. CORS requests fail. Response headers change unexpectedly.

**Confidence:** HIGH -- this is a general Go web development pattern risk, verified against the codebase middleware chain in `main.go` lines 240-244.

---

### Pitfall 7: Browser UX Verification Across Environments -- Flaky Results from System Differences

**What goes wrong:** The 20 deferred v1.4 human verification items cover visual/browser behaviors: dark mode, keyboard navigation, CSS transitions, responsive layout, loading indicators. A developer verifies these on their local browser and marks them as "passed." In production on Fly.io, the behavior differs: Tailwind CDN load timing affects initial render, htmx timing differs under network latency, system font rendering changes dark mode contrast, and mobile Safari handles keyboard navigation differently than desktop Chrome.

**Why it happens:** Browser behavior is inherently environment-dependent. The deferred items specifically call out behaviors that are hard to verify outside a real browser: "CSS animations smoothness," "dark mode toggle and system preference detection," "keyboard navigation of search results." These depend on the browser engine, OS dark mode setting, font availability, network conditions, and viewport size. A local verification on a developer's machine with fast network and a specific browser/OS combination does not guarantee production behavior.

**Consequences:**
- Items are marked "verified" but users on different browsers or devices experience broken behavior.
- Dark mode detection relies on `prefers-color-scheme` media query, which behaves differently across browsers and is not testable in headless mode.
- Keyboard navigation with ARIA roles may work in Chrome but fail in Safari or Firefox due to different focus management.
- CSS transitions may stutter on mobile devices or low-powered machines.
- The htmx CDN (or Tailwind CDN) may be blocked by corporate firewalls, breaking the entire UI for some users.

**Prevention:**
- Create a checklist document for each verification item with specific steps, expected behavior, and the browser/OS combinations to test.
- Test on at minimum: Chrome (latest), Firefox (latest), Safari (latest, if available), and one mobile browser.
- For dark mode: test both system-level dark mode preference AND the manual toggle. Test the persistence across page reloads (localStorage).
- For keyboard navigation: test with Tab, Enter, Escape, and arrow keys. Verify `aria-activedescendant` updates correctly.
- For responsive layout: use Chrome DevTools device emulation at 375px (mobile), 768px (tablet), and 1024px+ (desktop).
- For CSS transitions: record a short screen capture if transition smoothness is subjective. If a transition is jerky, the CSS `will-change` property or reducing DOM complexity may help.
- For loading indicators: artificially throttle network in DevTools to "Slow 3G" to make loading states visible.
- Accept that some items are inherently subjective ("CSS animations smoothness") and document what "good enough" means.
- Do NOT attempt to automate these with Playwright or similar -- the cost of setting up visual regression testing for 20 one-time verification items exceeds the value. Manual verification is appropriate here.

**Detection:** User reports of broken UI behavior on specific browsers. Dark mode flicker on page load. Keyboard navigation does not cycle through results. Layout breaks on mobile viewports.

**Confidence:** MEDIUM -- the specific browser behaviors vary, but the general principle of cross-environment testing is well-established. ([BrowserStack dark mode testing guide](https://www.browserstack.com/guide/how-to-test-apps-in-dark-mode))

---

### Pitfall 8: Grafana Dashboard JSON Version Schema Mismatch

**What goes wrong:** The dashboard JSON is created with one Grafana version (e.g., 10.x) but the production Grafana instance runs a different version (9.x or 11.x). Grafana's dashboard JSON schema has changed significantly between major versions. The `schemaVersion` field in the JSON must match what the target Grafana instance expects. Panels, variables, and datasource references may use syntax not supported by the target version.

**Why it happens:** The developer creates the dashboard in their local Grafana (latest version) and exports it. The production Grafana may be a managed instance (Grafana Cloud, managed hosting) running a different version, or a self-hosted instance that has not been upgraded.

**Consequences:**
- Import fails with cryptic JSON parsing errors.
- Import succeeds but panels render incorrectly (wrong visualization type, missing options).
- Template variables do not resolve because the variable type syntax changed.

**Prevention:**
- Document the target Grafana version and schema version in a README alongside the dashboard JSON.
- If using Grafana Cloud or Fly.io Grafana, check the running version before exporting.
- Use the Grafana v2 JSON schema if targeting Grafana 11+, as Grafana is moving toward a new schema format.
- If the deployment target is not yet decided, target Grafana 10.x (widely deployed, stable schema) with `"schemaVersion": 39` (the latest v1 schema version as of Grafana 10.x).
- Test the dashboard import on the target Grafana version before committing.

**Detection:** Dashboard import shows validation errors. Panels display "Panel plugin not found" for visualization types that changed names between versions.

**Confidence:** MEDIUM -- depends on the specific Grafana deployment, but version mismatch is a common issue.

---

## Minor Pitfalls

### Pitfall 9: otelhttp Metric Names Are Not Custom -- They Follow OTel Semantic Conventions

**What goes wrong:** The developer assumes they can query HTTP request metrics using `pdbplus_*` names. But otelhttp middleware (line 243 of main.go) emits metrics using the OTel HTTP semantic convention names: `http.server.request.duration`, `http.server.active_requests`, `http.server.request.body.size`, `http.server.response.body.size`. In Prometheus, these become `http_server_request_duration_seconds_bucket`, `http_server_active_requests`, etc. The developer writes dashboard queries against wrong names.

**Prevention:**
- Use the OTel HTTP semantic convention metric names in dashboard queries, not custom names.
- The otelhttp metrics include attributes: `http.request.method`, `url.scheme`, `http.response.status_code`, `http.route` (if net/http pattern matching is used), and `server.address`.
- Query example for request rate by route: `rate(http_server_request_duration_seconds_count{http_route=~"/graphql|/rest/v1/.*|/api/.*|/ui/.*"}[5m])`
- Query example for p95 latency: `histogram_quantile(0.95, rate(http_server_request_duration_seconds_bucket[5m]))`

**Confidence:** HIGH -- otelhttp metric names are defined by the [OTel HTTP semantic conventions](https://opentelemetry.io/docs/specs/semconv/http/http-metrics/).

---

### Pitfall 10: Go Runtime Metrics Use OTel Convention Names, Not Go-Native Names

**What goes wrong:** The developer writes dashboard queries like `go_goroutines` or `go_memstats_alloc_bytes` (Prometheus Go client convention). But the project uses `runtime.Start()` from `go.opentelemetry.io/contrib/instrumentation/runtime` (provider.go line 89), which emits metrics using OTel runtime semantic conventions: `process.runtime.go.goroutines`, `process.runtime.go.mem.heap_alloc`, etc. In Prometheus, dots become underscores.

**Prevention:**
- Use OTel runtime metric names in dashboard queries:
  - `process_runtime_go_goroutines` (not `go_goroutines`)
  - `process_runtime_go_mem_heap_alloc_bytes` (not `go_memstats_alloc_bytes`)
  - `process_runtime_go_gc_pause_ns` or similar
- The exact metric names depend on the OTel runtime instrumentation version. Check the actual Prometheus metrics endpoint (`/metrics` or OTLP debug output) for the exact names.
- Consider adding a comment in the dashboard JSON or README documenting which instrumentation library produces which metrics.

**Confidence:** MEDIUM -- the OTel runtime instrumentation library has changed metric names between versions. Verify against the actual exported metrics.

---

### Pitfall 11: Grafana Dashboard JSON Is Not Idempotent -- UID and Version Conflicts on Re-Provisioning

**What goes wrong:** The dashboard JSON contains a `"uid"` field and a `"version"` field. On first provisioning, this works. On re-provisioning (e.g., after updating the JSON), Grafana may reject the update if the `"version"` field does not match the current version in the database. Or if `"uid"` is null/absent, Grafana creates a new dashboard on each provisioning cycle instead of updating the existing one.

**Prevention:**
- Always include a stable `"uid"` in the dashboard JSON (e.g., `"uid": "pdbplus-overview"`). This ensures updates replace the existing dashboard.
- Set `"version"` to `null` or remove it entirely. Grafana auto-increments the version on save. Including a specific version number causes conflicts.
- Set `"id"` to `null` in the JSON. The numeric `id` is instance-specific and should never be hardcoded.
- Test re-provisioning by modifying a panel and re-applying. Verify the dashboard is updated, not duplicated.

**Confidence:** HIGH -- [Grafana provisioning docs](https://grafana.com/docs/grafana/latest/administration/provisioning/) explicitly document this behavior.

---

### Pitfall 12: Verifying Deferred Items in Wrong Order -- Dependencies Between Verification Items

**What goes wrong:** The 26 deferred verification items are treated as an unordered checklist. A developer verifies "ASN redirect on Enter with numeric input" (v1.4 phase 14) before verifying "live search results update within 300ms in browser" (same phase). The ASN redirect depends on the search working correctly. If the search is broken, the ASN redirect verification is meaningless -- but the developer marks it as "passed" because the redirect code path is technically correct.

**Why it happens:** The verification items come from 7 different phases across 2 milestones. Their dependencies are implicit, not documented. The items are listed per-phase but the cross-phase dependencies (e.g., search depends on sync, detail pages depend on search, comparison depends on detail pages) are not visible in the flat list.

**Prevention:**
- Verify in dependency order:
  1. **Infrastructure first:** CI pipeline runs on GitHub (v1.2), coverage comments work (v1.2)
  2. **Data layer:** sync runs against real PeeringDB API (v1.2), API key auth works (v1.3), conformance passes live (v1.2/v1.3)
  3. **Foundation:** content negotiation, responsive layout, syncing page animation (v1.4 phase 13)
  4. **Search:** live search speed, type badges, ASN redirect (v1.4 phase 14)
  5. **Detail pages:** collapsible sections, lazy loading, summary stats, cross-links (v1.4 phase 15)
  6. **Comparison:** results layout, view toggle, multi-step flow (v1.4 phase 16)
  7. **Polish:** dark mode, keyboard navigation, CSS animations, loading indicators, error pages, About page freshness (v1.4 phase 17)
- If an earlier item fails, stop and fix it before proceeding to dependent items.
- Create a structured verification report with pass/fail/blocked status for each item.

**Confidence:** HIGH -- this is a general verification best practice, not technology-specific.

---

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Mitigation |
|-------------|---------------|------------|
| Grafana dashboard creation | Datasource UID hardcoding (#1), OTel-to-Prometheus name translation (#2), Panel sprawl (#5) | Use datasource template variables; document Prometheus metric names; limit to 15-20 panels with template variables for drill-down |
| Grafana dashboard provisioning | JSON schema version mismatch (#8), UID/version conflicts on re-deploy (#11) | Document target Grafana version; set stable UID, null version and id |
| meta.generated field verification | Undocumented field behavior (#3), zero-value fallback path untested | Test against live PeeringDB API for each request pattern; document observed behavior |
| Dead code removal (DataLoader + IsPrimary) | Stale planning docs (#4), removing code that is already gone or missing code that still exists | Verify codebase state with grep before writing removal tasks |
| Middleware removal patterns | Hidden context consumers (#6), test gap for removed middleware paths | Trace middleware effects; run full test suite with -race after removal |
| Browser UX verification (20 items) | Cross-environment differences (#7), dependency ordering (#12) | Test on multiple browsers; verify in dependency order; document "good enough" criteria |
| Dashboard query authoring | otelhttp uses semantic convention names (#9), runtime metrics use OTel names (#10) | Check actual Prometheus endpoint for exact metric names before writing queries |

## Sources

### Grafana Dashboard Provisioning
- [Grafana Provisioning Documentation](https://grafana.com/docs/grafana/latest/administration/provisioning/) -- official provisioning guide
- [Grafana Dashboard Best Practices](https://grafana.com/docs/grafana/latest/dashboards/build-dashboards/best-practices/) -- panel count, layout, and design recommendations
- [Grafana Community: Datasource UID Conflicts](https://community.grafana.com/t/provisioning-dashboards-data-sources-uid-conflicts/127762) -- UID provisioning issues
- [Grafana Community: Should Provisioned Dashboards Have Datasource UIDs?](https://community.grafana.com/t/should-provisioned-dashboards-have-datasource-uids/65463) -- datasource UID patterns
- [Grafana Dashboard JSON Model](https://grafana.com/docs/grafana/latest/visualizations/dashboards/build-dashboards/view-dashboard-json-model/) -- JSON schema reference
- [Grafana Dashboard Automation with CI/CD](https://grafana.com/docs/grafana/latest/as-code/observability-as-code/foundation-sdk/dashboard-automation/) -- dashboards-as-code patterns

### OpenTelemetry Metrics and Prometheus
- [OTel Prometheus Compatibility Spec](https://opentelemetry.io/docs/specs/otel/compatibility/prometheus_and_openmetrics/) -- naming translation rules
- [OTel HTTP Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/http/http-metrics/) -- otelhttp metric names
- [OTel Metrics Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/general/metrics/) -- general metric naming guidelines
- [OTel Go Instrumentation](https://opentelemetry.io/docs/languages/go/) -- Go SDK documentation

### PeeringDB API
- [PeeringDB API Specs](https://docs.peeringdb.com/api_specs/) -- official API documentation (meta.generated NOT documented)
- [PeeringDB API Docs](https://www.peeringdb.com/apidocs/) -- Swagger-style API reference

### Browser Testing
- [BrowserStack: How to Test Dark Mode](https://www.browserstack.com/guide/how-to-test-apps-in-dark-mode) -- cross-browser dark mode verification

### Codebase (verified against source)
- `internal/otel/metrics.go` -- 9 custom sync metrics with OTel names and units
- `internal/otel/provider.go` -- autoexport setup, runtime instrumentation, resource attributes
- `internal/peeringdb/client.go` -- FetchAll with meta.generated parsing, FetchMeta struct
- `internal/sync/worker.go` -- WorkerConfig.IsPrimary dead field at line 30
- `cmd/peeringdb-plus/main.go` -- middleware chain (lines 240-244), readiness middleware, route registration
- `.planning/milestones/v1.2-phases/07-lint-code-quality/07-01-SUMMARY.md` -- claims IsPrimary removed from WorkerConfig (incorrect)
- `.planning/milestones/v1.4-MILESTONE-AUDIT.md` -- 20 deferred human verification items listed by phase

