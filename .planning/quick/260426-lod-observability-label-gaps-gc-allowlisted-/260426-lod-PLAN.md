---
phase: 260426-lod
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/otel/provider.go
  - internal/otel/provider_test.go
  - cmd/peeringdb-plus/main.go
  - cmd/peeringdb-plus/middleware_chain_test.go
  - cmd/peeringdb-plus/route_tag_test.go
  - deploy/grafana/dashboards/pdbplus-overview.json
  - CLAUDE.md
autonomous: true
requirements:
  - OBS-LABELS-01  # GC-allowlisted resource attrs on metrics path
  - OBS-LABELS-02  # http.route label on http_server_request_duration_seconds

must_haves:
  truths:
    - "Metric resource carries service.namespace=<process_group>, cloud.region=<region>, cloud.provider=fly_io, cloud.platform=fly_io_apps"
    - "Metric resource still OMITS service.instance.id (per-VM cardinality blocker)"
    - "Trace/log resource carries service.instance.id, service.namespace, cloud.region, cloud.provider, cloud.platform, fly.app_name"
    - "http_server_request_duration_seconds carries an http.route label populated from r.Pattern after mux dispatch"
    - "Dashboard panel 35 description no longer mentions 'missing in v1.15'; query groups by service_namespace"
    - "Dashboard `region` template variable queries cloud_region (not fly_region)"
    - "Dashboard adds a `process_group` template variable using service_namespace"
    - "Existing legendFormat/expr references to fly_region migrated to cloud_region"
  artifacts:
    - path: "internal/otel/provider.go"
      provides: "Renamed includeMachineID -> includeInstanceID; new GC-allowlisted attr emission"
      contains: "semconv.ServiceInstanceID"
    - path: "internal/otel/provider_test.go"
      provides: "Coverage for new resource attrs and metric-resource omission of service.instance.id"
      contains: "TestBuildMetricResource_OmitsServiceInstanceID"
    - path: "cmd/peeringdb-plus/main.go"
      provides: "routeTagMiddleware wired innermost between Compression and the mux"
      contains: "routeTagMiddleware"
    - path: "cmd/peeringdb-plus/route_tag_test.go"
      provides: "End-to-end test asserting labeler carries http.route after mux dispatch"
      contains: "TestRouteTagMiddleware_PopulatesLabeler"
    - path: "deploy/grafana/dashboards/pdbplus-overview.json"
      provides: "Panel 35 + region var migrated to GC-allowlisted labels; new process_group var"
      contains: "service_namespace"
    - path: "CLAUDE.md"
      provides: "Sync observability section notes the GC-allowlisted naming and env-var mapping"
      contains: "service.namespace"
  key_links:
    - from: "internal/otel/provider.go buildResourceFiltered"
      to: "metric resource via buildMetricResource (keeps service.namespace + cloud.region; drops service.instance.id)"
      via: "includeInstanceID flag"
      pattern: "includeInstanceID"
    - from: "cmd/peeringdb-plus/main.go routeTagMiddleware"
      to: "otelhttp.LabelerFromContext"
      via: "post-dispatch labeler.Add(attribute.String(\"http.route\", r.Pattern))"
      pattern: "LabelerFromContext"
---

<objective>
Fix two observability label gaps from the post-deploy audit (commit bca0b1a):

1. Grafana Cloud's hosted OTLP receiver drops `fly.*` custom resource attributes on the metrics path. Switch to GC-allowlisted semconv keys (`service.instance.id`, `service.namespace`, `cloud.region`, `cloud.provider`, `cloud.platform`) so panel 35 (Peak Heap by Process Group) and the dashboard's `region` template variable work end-to-end. Re-evaluate per-VM cardinality: keep `service.instance.id` STRIPPED from the metric resource (per-machine = high cardinality, low value); KEEP `service.namespace` (process group, 2 values) and `cloud.region` (8 values) ON the metric resource since they answer the operator's actual question.

2. `http_server_request_duration_seconds` has no `http.route` label because otelhttp wraps the mux outermost — `r.Pattern` is empty when otelhttp records metrics PRE-dispatch. otelhttp v0.68.0 has NO `WithRouteTag` option (verified against `/home/dotwaffle/go/pkg/mod/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp@v0.68.0/`). The library reads `LabelerFromContext` AFTER `next.ServeHTTP` returns (handler.go:172 + 202). Inject `http.route` via a post-dispatch tail middleware that mutates the labeler after the mux populates `r.Pattern`.

Purpose: per-region / per-process-group / per-endpoint RED breakdowns become possible on the production dashboard.

Output: GC-allowlisted resource attrs, working `http.route` label, updated dashboard, CLAUDE.md note.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@CLAUDE.md
@internal/otel/provider.go
@internal/otel/provider_test.go
@cmd/peeringdb-plus/main.go
@cmd/peeringdb-plus/middleware_chain_test.go
@deploy/grafana/dashboards/pdbplus-overview.json

<interfaces>
<!-- Verified against installed module @v0.68.0 — Labeler is mutated by reference and read AFTER next.ServeHTTP returns. -->

From go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp@v0.68.0/labeler.go:
```go
type Labeler struct { /* ... */ }
func (l *Labeler) Add(ls ...attribute.KeyValue)
func (l *Labeler) Get() []attribute.KeyValue
func ContextWithLabeler(parent context.Context, l *Labeler) context.Context
func LabelerFromContext(ctx context.Context) (*Labeler, bool)  // returns (l, found)
```

From otelhttp@v0.68.0/handler.go (request flow inside otelhttp.NewMiddleware):
```go
labeler, found := LabelerFromContext(ctx)
if !found {
    ctx = ContextWithLabeler(ctx, labeler)
}
r = r.WithContext(ctx)
next.ServeHTTP(w, r)              // <-- mux dispatches here; r.Pattern populated
// AFTER dispatch:
h.semconv.RecordMetrics(ctx, semconv.ServerMetricData{
    MetricAttributes: semconv.MetricAttributes{
        AdditionalAttributes: append(labeler.Get(), ...),  // <-- reads labeler AFTER dispatch
    },
})
```

From go.opentelemetry.io/otel/semconv/v1.26.0 (already imported as `semconv`):
```go
func ServiceInstanceID(val string) attribute.KeyValue   // "service.instance.id"
func ServiceNamespace(val string) attribute.KeyValue    // "service.namespace"
func CloudRegion(val string) attribute.KeyValue         // "cloud.region"
const CloudProviderKey = attribute.Key("cloud.provider")  // value: free-form string
const CloudPlatformKey = attribute.Key("cloud.platform")  // value: free-form string
```

There is no `CloudProviderFlyIO` or `CloudPlatformFlyIOApps` enum constant — Fly.io isn't in the semconv enum. Use `attribute.String("cloud.provider", "fly_io")` and `attribute.String("cloud.platform", "fly_io_apps")` directly. Lowercase + underscore matches the semconv enum naming style.

Critical timing note: the labeler is mutated by reference. A tail middleware that adds `http.route` AFTER `next.ServeHTTP(mux)` returns will be visible to otelhttp's later `labeler.Get()` call because both operate on the same `*Labeler` value stored in ctx.
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Switch resource attributes to GC-allowlisted semconv keys</name>
  <files>internal/otel/provider.go, internal/otel/provider_test.go</files>
  <behavior>
    - buildResource (trace/log path) MUST emit: service.name, service.version, service.instance.id (from FLY_MACHINE_ID), service.namespace (from FLY_PROCESS_GROUP), cloud.region (from FLY_REGION), cloud.provider="fly_io", cloud.platform="fly_io_apps", fly.app_name (from FLY_APP_NAME, kept as-is for human grep).
    - buildMetricResource MUST emit everything above EXCEPT service.instance.id (per-VM cardinality blocker — same rationale as the existing fly.machine_id strip).
    - Empty env vars MUST NOT produce empty-string attrs (existing behaviour: `if v := os.Getenv(...); v != ""`).
    - cloud.provider and cloud.platform MUST be unconditional constants (independent of env vars).
    - Tests: replace TestBuildResource_WithFlyRegion / TestBuildResource_IncludesFlyMachineID / TestBuildMetricResource_OmitsFlyMachineID with the new attribute keys. Add: TestBuildMetricResource_IncludesServiceNamespace, TestBuildMetricResource_IncludesCloudRegion, TestBuildMetricResource_OmitsServiceInstanceID, TestBuildResource_IncludesServiceInstanceID, TestBuildResource_CloudProviderConstant.
  </behavior>
  <action>
    Edit `internal/otel/provider.go`:

    1. Rename `includeMachineID` -> `includeInstanceID` everywhere (parameter, callers, doc comments). The semantic is identical — per-VM cardinality strip — but the new name matches the new attr key.

    2. Replace the env-var loop at lines 178-192 with explicit emission. The old struct-slice pattern hardcoded the gate against the literal string `"fly.machine_id"`; the new code gates against the boolean directly so the conditional intent stays grep-able:

       ```go
       // Always-on: cloud provider / platform constants (1-cardinality, GC-allowlisted).
       attrs = append(attrs,
           attribute.String(string(semconv.CloudProviderKey), "fly_io"),
           attribute.String(string(semconv.CloudPlatformKey), "fly_io_apps"),
       )
       // Env-driven: only emit when the env var is set (avoids empty-string attrs in local dev).
       if v := os.Getenv("FLY_REGION"); v != "" {
           attrs = append(attrs, semconv.CloudRegion(v))
       }
       if v := os.Getenv("FLY_PROCESS_GROUP"); v != "" {
           attrs = append(attrs, semconv.ServiceNamespace(v))
       }
       if v := os.Getenv("FLY_APP_NAME"); v != "" {
           // Custom key kept for human grep against historical logs/traces;
           // GC drops this on the metrics path but harmless on traces/logs.
           attrs = append(attrs, attribute.String("fly.app_name", v))
       }
       // Per-VM identity: stripped from metric resource to prevent per-VM
       // metric fan-out (8 machines × N metrics × M label combos). Traces
       // and logs keep it for per-VM debugging — that's the includeInstanceID
       // gate.
       if includeInstanceID {
           if v := os.Getenv("FLY_MACHINE_ID"); v != "" {
               attrs = append(attrs, semconv.ServiceInstanceID(v))
           }
       }
       ```

    3. Update the comment block above `buildResourceFiltered` to document:
       - WHY the metric resource keeps service.namespace + cloud.region (operator wants primary-vs-replica + per-region health, low cardinality).
       - WHY the metric resource strips service.instance.id (per-VM = high cardinality, low value — the operator does not want 8 series per metric).
       - WHY cloud.provider + cloud.platform are unconditional (semconv resource attrs that GC allowlists for free).

    4. Update the comment in `Setup` (lines 68-72) referencing "fly.machine_id" to reference "service.instance.id" instead.

    Edit `internal/otel/provider_test.go`:

    5. Replace TestBuildResource_WithFlyRegion -> TestBuildResource_WithCloudRegion: assert `cloud.region=iad` from `FLY_REGION=iad`.
    6. Replace TestBuildResource_IncludesFlyMachineID -> TestBuildResource_IncludesServiceInstanceID: assert `service.instance.id=abc123` from `FLY_MACHINE_ID=abc123`.
    7. Replace TestBuildMetricResource_OmitsFlyMachineID -> TestBuildMetricResource_OmitsServiceInstanceID: assert metric resource does NOT contain `service.instance.id` even when FLY_MACHINE_ID is set, AND DOES contain `service.namespace`+`cloud.region`+`cloud.provider`+`cloud.platform`.
    8. Add TestBuildResource_IncludesServiceNamespace: set FLY_PROCESS_GROUP=primary, assert `service.namespace=primary`.
    9. Add TestBuildResource_CloudProviderConstant: assert `cloud.provider=fly_io` AND `cloud.platform=fly_io_apps` are present unconditionally (no env vars set).
    10. Add TestBuildResource_EmptyEnvOmitsAttr: with FLY_REGION unset (use t.Setenv("FLY_REGION", "")), assert `cloud.region` is NOT in the attribute set (no empty-string leak).

    Style: keep the table-driven helper `findAttr(res, key)` if you write one — match GO-T-1.
  </action>
  <verify>
    <automated>cd /home/dotwaffle/Code/pdb/peeringdb-plus && TMPDIR=/tmp/claude-1000 go test -race ./internal/otel/...</automated>
  </verify>
  <done>All new tests pass; no test for an old fly.* metric-resource key remains; `go vet ./internal/otel/...` clean.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Add http.route label via post-dispatch labeler tail middleware</name>
  <files>cmd/peeringdb-plus/main.go, cmd/peeringdb-plus/middleware_chain_test.go, cmd/peeringdb-plus/route_tag_test.go</files>
  <behavior>
    - A new tail middleware `routeTagMiddleware` MUST sit between `middleware.Compression()` and the inner mux (innermost wrap of buildMiddlewareChain). It calls `next.ServeHTTP(w, r)` first, then — AFTER mux dispatch sets `r.Pattern` — adds `attribute.String("http.route", r.Pattern)` to the labeler retrieved via `otelhttp.LabelerFromContext(r.Context())`. Empty `r.Pattern` (unmatched routes / NotFound) MUST NOT produce an empty-string label — skip the Add call when r.Pattern == "".
    - The labeler MUST be the same `*Labeler` that otelhttp installed in ctx; `LabelerFromContext` returns it by reference (sync.Mutex-guarded), so post-dispatch mutation is visible when otelhttp later calls `labeler.Get()`.
    - Test: spin up a real http.ServeMux, register a `GET /test/{id}` handler, wrap with `otelhttp.NewMiddleware("test")` THEN `routeTagMiddleware`, fire a request, and assert the labeler attached to the request ctx contains `http.route="GET /test/{id}"` after the response. Use `otelhttp.ContextWithLabeler` + a captured *Labeler to inspect post-hoc — the production wiring relies on otelhttp installing it, but the test exercises the ordering invariant directly.
    - Pattern empty case: GET /unmatched -> labeler MUST NOT contain an http.route attribute (zero attrs).
  </behavior>
  <action>
    Edit `cmd/peeringdb-plus/main.go`:

    1. Add the middleware near the other middleware/helper functions (e.g. just after `readinessMiddleware`):

       ```go
       // routeTagMiddleware injects http.route into the otelhttp labeler AFTER
       // the mux dispatches a request. otelhttp.NewMiddleware reads the labeler
       // AFTER its inner next.ServeHTTP returns (otelhttp@v0.68.0/handler.go:172
       // +202), so a post-dispatch mutation here is visible to the metric
       // recording pass — see /home/dotwaffle/go/pkg/mod/go.opentelemetry.io/
       // contrib/instrumentation/net/http/otelhttp@v0.68.0/handler.go.
       //
       // Why a tail middleware instead of otelhttp.WithRouteTag: that option
       // does not exist in v0.68.0. The Labeler is the supported escape hatch
       // for adding metric attributes after the framework has dispatched.
       //
       // Empty r.Pattern (unmatched routes / NotFound) is skipped so we do not
       // emit an http.route="" label that would balloon Prometheus cardinality
       // for 404 traffic.
       func routeTagMiddleware(next http.Handler) http.Handler {
           return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
               next.ServeHTTP(w, r)
               if r.Pattern == "" {
                   return
               }
               labeler, ok := otelhttp.LabelerFromContext(r.Context())
               if !ok {
                   return
               }
               labeler.Add(attribute.String("http.route", r.Pattern))
           })
       }
       ```

       Add the `go.opentelemetry.io/otel/attribute` import if not already present in main.go.

    2. In `buildMiddlewareChain`, wrap routeTagMiddleware as the innermost layer (Compression already sits closest to the handler, but routeTagMiddleware needs to be EVEN MORE inner so that `next.ServeHTTP(w, r)` IS the mux dispatch and `r.Pattern` is populated when control returns). Insert one line before `h := middleware.Compression()(inner)`:

       ```go
       h := routeTagMiddleware(inner)
       h = middleware.Compression()(h)
       // ... rest unchanged
       ```

       Update the doc comment block above buildMiddlewareChain (the chain order list near lines 786-789 and 791-793) to insert `RouteTag` before Compression in the innermost-first wrap order. The wrap order comment AND the runtime-order comment AND the trace-style comment ("// h := middleware.Compression()(inner)" → now wraps routeTagMiddleware, which wraps inner). Also update the chain comment in main() at lines 478-490.

    3. Update `cmd/peeringdb-plus/middleware_chain_test.go`:
       - Add `"routeTagMiddleware("` as the FIRST entry of `wantOrder` (innermost-first; runtime-last). The existing source-scan logic auto-detects via `strings.Index` so position-zero is the right slot.

    Create `cmd/peeringdb-plus/route_tag_test.go`:

    4. Two table-driven sub-tests in a single TestRouteTagMiddleware function:
       - `populates_labeler_with_pattern`: build a mux with `GET /test/{id}` returning 200; build chain `otelhttp.NewMiddleware("test")(routeTagMiddleware(mux))`; fire `GET /test/42`; capture the labeler by intercepting via a second middleware that runs AFTER otelhttp installs the labeler (use the technique of calling `otelhttp.ContextWithLabeler` with a known-pointer labeler in a setup-middleware that runs INSIDE otelhttp's NewMiddleware — wrap order: `otelhttp.NewMiddleware("test")(captureLabelerMW(routeTagMiddleware(mux)))`. captureLabelerMW shoves a fresh `*otelhttp.Labeler` into ctx via `otelhttp.ContextWithLabeler`. After ServeHTTP returns, assert the captured labeler's `Get()` contains `attribute.String("http.route", "GET /test/{id}")`).
       - `unmatched_route_omits_label`: hit a path the mux does not match (NotFound). Assert labeler `Get()` returns zero entries with key `http.route`.
    5. Use `httptest.NewRecorder` + `httptest.NewRequest`; no real network. Mark with `t.Parallel()`.
    6. Per GO-T-1 / GO-T-2: table-driven, race-safe.
  </action>
  <verify>
    <automated>cd /home/dotwaffle/Code/pdb/peeringdb-plus && TMPDIR=/tmp/claude-1000 go test -race ./cmd/peeringdb-plus/... -run 'TestRouteTagMiddleware|TestMiddlewareChain_Order'</automated>
  </verify>
  <done>Both new sub-tests pass; existing TestMiddlewareChain_Order passes with the updated wantOrder.</done>
</task>

<task type="auto">
  <name>Task 3: Migrate dashboard to GC-allowlisted labels + add process_group var</name>
  <files>deploy/grafana/dashboards/pdbplus-overview.json</files>
  <action>
    Edit `deploy/grafana/dashboards/pdbplus-overview.json`. Prometheus normalises OTel resource keys by replacing `.` with `_` — the new label names on the wire are `service_namespace`, `service_instance_id`, `cloud_region`, `cloud_provider`, `cloud_platform`.

    1. Template variable `region` (lines ~122-144): change `definition` and `query.query` from `label_values(http_server_request_duration_seconds_count, fly_region)` to `label_values(http_server_request_duration_seconds_count, cloud_region)`. Keep label "Region", name "region" — the variable's $region expansion works unchanged downstream because all consuming queries are also migrated below.

    2. Add a new template variable `process_group` (insert after the `region` block in `templating.list`):
       ```json
       {
         "current": {},
         "datasource": { "uid": "${datasource}" },
         "definition": "label_values(pdbplus_sync_peak_heap_bytes, service_namespace)",
         "hide": 0,
         "includeAll": true,
         "allValue": ".*",
         "label": "Process Group",
         "multi": true,
         "name": "process_group",
         "options": [],
         "query": {
           "query": "label_values(pdbplus_sync_peak_heap_bytes, service_namespace)",
           "refId": "StandardVariableQuery"
         },
         "refresh": 2,
         "regex": "",
         "skipUrlSync": false,
         "sort": 1,
         "type": "query"
       }
       ```
       Note: `pdbplus_sync_peak_heap_bytes` is used as the label-discovery metric because it's emitted on every sync cycle by every machine in both process groups — guaranteed to populate both primary and replica.

    3. In every panel target `expr` and `legendFormat`, replace `fly_region` with `cloud_region`. Affected lines per the earlier grep:
       - line 726: `sum by(http_route)(rate(...{fly_region=~"$region"}...))` -> `cloud_region=~"$region"`
       - line 800: two occurrences of `fly_region=~"$region"` -> `cloud_region=~"$region"`
       - line 868: `fly_region=~"$region"` -> `cloud_region=~"$region"`
       - line 934: `fly_region=~"$region"` -> `cloud_region=~"$region"`
       - line 942: `fly_region=~"$region"` -> `cloud_region=~"$region"`
       - line 1905: `legendFormat: "{{fly_region}}"` -> `"{{cloud_region}}"`
       - line 1976: `legendFormat: "{{fly_region}}"` -> `"{{cloud_region}}"`

    4. Panel 35 "Peak Heap by Process Group" (line ~2028):
       - Update `description` to remove the "missing in v1.15" caveat. New text:
         `"Same pdbplus_sync_peak_heap_bytes grouped by service.namespace (Fly process group: primary vs replica). Resource attribute promoted to a Prometheus label by Grafana Cloud's OTLP receiver. Bytes unit; Grafana auto-formats MiB / GiB."`
       - Update `targets[0].expr` from bare `pdbplus_sync_peak_heap_bytes` to `sum by(service_namespace, cloud_region)(pdbplus_sync_peak_heap_bytes)` so the legend has clean dimensions instead of one series per machine.
       - Update `targets[0].legendFormat` from `"{{fly_process_group}} / {{fly_region}}"` to `"{{service_namespace}} / {{cloud_region}}"`.

    5. RED-by-endpoint sanity: panel at line 726 already does `sum by(http_route)(rate(http_server_request_duration_seconds_count...))`. This was producing a single empty-route series before Task 2; after deployment it'll fan out by route. No JSON change needed — this panel just becomes useful.

    6. Validate the JSON parses: `python3 -c 'import json; json.load(open("deploy/grafana/dashboards/pdbplus-overview.json"))'` (or `jq . deploy/grafana/dashboards/pdbplus-overview.json > /dev/null`).
  </action>
  <verify>
    <automated>cd /home/dotwaffle/Code/pdb/peeringdb-plus && jq . deploy/grafana/dashboards/pdbplus-overview.json > /dev/null && ! grep -n 'fly_region\|fly_process_group\|fly_machine_id' deploy/grafana/dashboards/pdbplus-overview.json</automated>
  </verify>
  <done>JSON is valid; no `fly_region`/`fly_process_group`/`fly_machine_id` strings remain; panel 35 description and query updated; new `process_group` template variable present.</done>
</task>

<task type="auto">
  <name>Task 4: Lint pass + CLAUDE.md sync-observability addendum</name>
  <files>CLAUDE.md</files>
  <action>
    1. Update `CLAUDE.md` "Sync observability" section. Find the paragraph ending with the `OBS-04` incident-response note (look for `**Incident-response debugging (OBS-04).**`). Insert a new paragraph BEFORE it:

       ```markdown
       **OTel resource attributes (post-260426-lod).** Grafana Cloud's hosted OTLP receiver only promotes a small allowlist of OTel semconv resource attrs to Prometheus labels (`service.*`, `cloud.*`, `host.*`, `k8s.*`); custom `fly.*` keys are silently dropped on the metrics path. Resource attrs are emitted from `internal/otel/provider.go` `buildResourceFiltered`:

       | Env var | Resource attr | semconv | On metrics? | On traces/logs? |
       |---|---|---|---|---|
       | `FLY_REGION` | `cloud.region` | `semconv.CloudRegion` | yes | yes |
       | `FLY_PROCESS_GROUP` | `service.namespace` | `semconv.ServiceNamespace` | yes | yes |
       | `FLY_MACHINE_ID` | `service.instance.id` | `semconv.ServiceInstanceID` | NO (per-VM cardinality) | yes |
       | `FLY_APP_NAME` | `fly.app_name` | (custom) | dropped by GC | yes (human grep) |
       | (constant) | `cloud.provider="fly_io"` | `semconv.CloudProviderKey` | yes | yes |
       | (constant) | `cloud.platform="fly_io_apps"` | `semconv.CloudPlatformKey` | yes | yes |

       The `service.instance.id` strip on the metric resource is gated by `includeInstanceID` in `buildResourceFiltered` — same rationale as the prior `fly.machine_id` strip (8 machines × N metrics × M label combos). `service.namespace` (2-cardinality: primary / replica) and `cloud.region` (8-cardinality) stay on metrics because they answer the operator's actual breakdown questions; the dashboard's `process_group` template variable + panel 35 grouping depend on `service.namespace`.

       **`http.route` label on `http_server_request_duration_seconds`.** Injected via `routeTagMiddleware` in `cmd/peeringdb-plus/main.go`, wrapped as the innermost middleware (between `middleware.Compression` and the mux). otelhttp v0.68.0 has no `WithRouteTag`; the middleware mutates `otelhttp.LabelerFromContext` AFTER `next.ServeHTTP` (mux dispatch) so `r.Pattern` is populated. Empty `r.Pattern` (unmatched routes) is skipped to avoid `http.route=""` cardinality bloat on 404 traffic.
       ```

       Keep tone terse — match the existing section's prose density.

    2. Run linters and the broader test sweep to catch any drift the focused tests in Task 1 / Task 2 missed:

       ```bash
       cd /home/dotwaffle/Code/pdb/peeringdb-plus
       TMPDIR=/tmp/claude-1000 go test -race ./internal/otel/... ./internal/middleware/... ./cmd/peeringdb-plus/...
       TMPDIR=/tmp/claude-1000 go vet ./...
       golangci-lint run --timeout 5m ./...
       ```

       Address any failures (formatting, unused imports, etc.). Do NOT silence findings with `//nolint` unless the alternative breaks the design — call out any nolint addition in the SUMMARY.

    3. Manual deployment verification note for the SUMMARY (no automated check possible from this environment): operator should, post-deploy, run a PromQL probe in Grafana Cloud:
       ```promql
       count by (service_namespace, cloud_region) (pdbplus_sync_peak_heap_bytes)
       count by (http_route) (http_server_request_duration_seconds_count)
       ```
       Both should return non-empty series with multiple distinct label values within ~5 minutes of deploy.
  </action>
  <verify>
    <automated>cd /home/dotwaffle/Code/pdb/peeringdb-plus && TMPDIR=/tmp/claude-1000 go test -race ./internal/otel/... ./internal/middleware/... ./cmd/peeringdb-plus/... && TMPDIR=/tmp/claude-1000 go vet ./... && golangci-lint run --timeout 5m ./...</automated>
  </verify>
  <done>All tests pass, lint clean, CLAUDE.md addendum in place referencing the env-to-attr mapping and the routeTagMiddleware wiring.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| FLY_* env -> resource attrs | Untrusted-by-default per GO-SEC-2; values stamped into telemetry but never used for auth or DB lookups |
| HTTP request -> http.route label | r.Pattern is mux-populated (server-controlled), not client-controlled; safe to use as a label |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-260426-lod-01 | Information disclosure | Empty FLY_* env -> empty-string Prom label | mitigate | Existing `if v != ""` gate preserved in Task 1 + new gate added for FLY_PROCESS_GROUP |
| T-260426-lod-02 | Denial of service (cardinality) | Per-VM service.instance.id on metric resource | mitigate | Stripped via includeInstanceID=false in buildMetricResource (Task 1) |
| T-260426-lod-03 | Denial of service (cardinality) | Empty r.Pattern -> http.route="" label on 404 traffic | mitigate | routeTagMiddleware skips Add when r.Pattern == "" (Task 2) |
| T-260426-lod-04 | Tampering | Client-spoofed http.route via header | accept | r.Pattern is set by net/http ServeMux from the registered pattern, not from request headers; no client surface |
</threat_model>

<verification>
- All four task `<automated>` commands green.
- `grep -c 'fly_region\|fly_machine_id\|fly_process_group' deploy/grafana/dashboards/pdbplus-overview.json` returns 0.
- `grep -n 'service.instance.id\|service.namespace\|cloud.region' internal/otel/provider.go` returns lines for all three keys.
- `grep -n 'routeTagMiddleware' cmd/peeringdb-plus/main.go cmd/peeringdb-plus/middleware_chain_test.go` returns at least one hit per file.
- Manual PromQL deployment probe documented in SUMMARY for the operator.
</verification>

<success_criteria>
- Metric resource carries `service.namespace`, `cloud.region`, `cloud.provider`, `cloud.platform` (GC-allowlisted) and OMITS `service.instance.id`.
- Trace/log resource carries all six attrs including `service.instance.id`.
- `http_server_request_duration_seconds` carries `http.route` label populated from `r.Pattern`.
- Empty `r.Pattern` (404s) does NOT produce `http.route=""`.
- Dashboard panel 35 query groups by `service_namespace`; description no longer claims the wiring is missing.
- Dashboard `region` template variable queries `cloud_region`; new `process_group` variable queries `service_namespace`.
- CLAUDE.md "Sync observability" section documents the GC-allowlisted naming + env-var mapping.
- `go test -race ./internal/otel/... ./internal/middleware/... ./cmd/peeringdb-plus/...` + `go vet ./...` + `golangci-lint run` all green.
</success_criteria>

<output>
After completion, create `.planning/quick/260426-lod-observability-label-gaps-gc-allowlisted-/260426-lod-SUMMARY.md` covering:
- What changed in provider.go (with before/after attr table).
- The routeTagMiddleware ordering rationale (cite otelhttp@v0.68.0/handler.go:172+202).
- Dashboard label migrations (which lines, which panels).
- The PromQL deployment-verification probe for the operator to run post-deploy.
- Any nolint additions (should be none).
</output>
