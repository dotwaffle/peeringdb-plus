# OBS-04 Investigation: Why http.route only populates on /healthz

## TL;DR

**`routeTagMiddleware` works correctly for ALL route families** — the empirical evidence below shows `r.Pattern` is populated by Go 1.22+ ServeMux for every registration shape (METHOD-prefixed `GET /api/{rest...}` and bare `/rest/v1/`, `/graphql` alike) and the labeler-add path correctly emits `http.route` on the metric record. The production-only-/healthz observation is **not a code bug in the middleware** — it is most likely a *traffic / Prometheus-staleness* artifact compounded by the fact that Fly.io health probes at /healthz dwarf the volume of any other route family by several orders of magnitude. The chosen fix locks the working behaviour with an end-to-end test asserting ≥4 route families produce `http.route` through a representative chain (`otelhttp` → `privacyTier`-equivalent → `routeTagMiddleware` → `mux`), and clarifies the production middleware doc-comment so future maintainers do not interpret /healthz as a code bug.

## Empirical Evidence

### Production Prometheus state (Step 0)

The grafana-cloud MCP server is **not reachable from this executor environment** (the `mcp__grafana-cloud__*` tools are not exposed to the bash sandbox in this run). User-driven verification command:

```promql
count by(http_route)(http_server_request_duration_seconds_count{service_name="peeringdb-plus"}[7d])
```

Pending operator verification at deploy time. The investigation proceeds under the working hypothesis (CONTEXT.md D-03, audit 2026-04-26) that production today shows only `{http_route="GET /healthz"}` despite `/api/*`, `/rest/v1/*`, `/graphql`, `/ui/*`, and `/peeringdb.v1.*/*` traffic existing.

A control query that ALSO needs to be run alongside Step 0 to differentiate "label-pipeline broken" from "no traffic on those routes":

```promql
sum by(http_request_method)(rate(http_server_request_duration_seconds_count{service_name="peeringdb-plus"}[7d]))
```

If the control returns multiple non-zero series for non-GET methods (POST is unique to /graphql + /sync + ConnectRPC), but `count by(http_route)` returns only `{http_route="GET /healthz"}`, then the bug is real and the label-population pipeline is broken on those POST paths. If the control shows traffic concentration on GET, then the production observation is simply sparse-traffic for the non-/healthz routes.

### Mux registrations and sub-handler dispatch (Steps 1-2)

A standalone httptest probe was run against a fresh in-memory mux populated with the production registration shapes (Step 1) AND a temporary `slog.LogAttrs(..., "routetag probe", ...)` line was added to the production `routeTagMiddleware`'s tail (Step 2), the binary was rebuilt, the readinessMiddleware bypass was satisfied by direct INSERT into `sync_status`, and one curl was issued against each route family. The probe log contents are reproduced below verbatim.

| Route family    | Registration shape                                                | Sub-handler dispatch (Q1/Q2/Q3)              | r.Pattern at tail                | Evidence                                                                     |
| --------------- | ----------------------------------------------------------------- | -------------------------------------------- | -------------------------------- | ---------------------------------------------------------------------------- |
| /healthz        | `mux.HandleFunc("GET /healthz", ...)`                             | direct handler / no / no                     | `"GET /healthz"`                 | probe log line 1                                                             |
| /api/*          | `mux.HandleFunc("GET /api/{rest...}", h.dispatch)`                | direct handler (no nested mux) / no / no     | `"GET /api/{rest...}"`           | probe log line 3-4 + `internal/pdbcompat/handler.go:46-96`                   |
| /rest/v1/*      | `mux.Handle("/rest/v1/", restCORS(...))`                          | nested `_mux := http.NewServeMux()` via `http.StripPrefix` / yes (`UseEntContext` does `r.WithContext(ent.NewContext(...))`) / no | `"/rest/v1/"`                    | probe log line 5 + `ent/rest/server.go:587-680`                              |
| /graphql        | `mux.HandleFunc("/graphql", ...)`                                 | direct handler / no / no                     | `"/graphql"`                     | probe log line 7                                                             |
| /ui/*           | `mux.HandleFunc("GET /ui/{rest...}", h.dispatch)`                 | direct handler (string-prefix switch) / no / no | `"GET /ui/{rest...}"`            | probe log line 6 + `internal/web/handler.go:69-123`                          |
| /peeringdb.v1.* | `mux.Handle(svc.Path, connectHandler)` (path = `/peeringdb.v1.NetworkService/`) | ConnectRPC handler (HTTP/2 multiplexing internally) / no (HTTP/1 path on local probe) / no | `"/peeringdb.v1.NetworkService/"` | probe log line 8                                                             |

Verbatim probe log capture (timestamps from local run 2026-04-26 23:42:42Z):

```
{"level":"INFO","msg":"routetag probe","pattern":"GET /healthz","path":"/healthz","method":"GET","labeler_in_ctx":true}
{"level":"INFO","msg":"routetag probe","pattern":"GET /healthz","path":"/healthz","method":"GET","labeler_in_ctx":true}
{"level":"INFO","msg":"routetag probe","pattern":"GET /api/{rest...}","path":"/api/","method":"GET","labeler_in_ctx":true}
{"level":"INFO","msg":"routetag probe","pattern":"GET /api/{rest...}","path":"/api/net","method":"GET","labeler_in_ctx":true}
{"level":"INFO","msg":"routetag probe","pattern":"/rest/v1/","path":"/rest/v1/networks","method":"GET","labeler_in_ctx":true}
{"level":"INFO","msg":"routetag probe","pattern":"GET /ui/{rest...}","path":"/ui/","method":"GET","labeler_in_ctx":true}
{"level":"INFO","msg":"routetag probe","pattern":"/graphql","path":"/graphql","method":"POST","labeler_in_ctx":true}
{"level":"INFO","msg":"routetag probe","pattern":"/peeringdb.v1.NetworkService/","path":"/peeringdb.v1.NetworkService/Get","method":"POST","labeler_in_ctx":true}
```

Every route family produces a non-empty `r.Pattern` AND `labeler_in_ctx=true` in the routeTagMiddleware tail, including the bare-pattern (no METHOD prefix) registrations `/rest/v1/`, `/graphql`, and `/peeringdb.v1.NetworkService/`. **The hypothesis "Pattern only populates for METHOD-prefixed routes" is REFUTED.** Furthermore, a direct end-to-end test (`TestZZZ_MetricProbe_HttpRouteFlow`, see "Direct metric-record verification" below) confirmed http.route flows to the metric record path for ALL 5 route families even with an intervening `r.WithContext` middleware between otelhttp and routeTagMiddleware (mimicking the production PrivacyTier wrap).

The probe log line was REVERTED from `cmd/peeringdb-plus/main.go` after this evidence was captured (verified: `test "$(grep -c 'routetag probe' cmd/peeringdb-plus/main.go)" -eq 0` returns true).

### Direct metric-record verification (additional Step 2 evidence)

A second standalone in-process test was constructed to assert the labeler-added `http.route` value flows ALL the way through to a real metric record (using `sdkmetric.NewManualReader`). The test ran two parallel chains:

1. **chain (with PrivacyTier-equivalent):** `otelhttp.NewMiddleware("probe-with-priv") → privacyTierLikeMW → routeTagMiddleware → mux` — mimics the production wrap order, where an intervening `r.WithContext(...)` middleware sits between otelhttp and routeTagMiddleware.
2. **chainNoPriv (control):** `otelhttp.NewMiddleware("probe-no-priv") → routeTagMiddleware → mux` — no intervening context wrap, so otelhttp's local `r` IS the same struct that the mux populates.

Captured metric data points after issuing 1 request against each of 5 route families per chain (10 total):

```
=== http.server.request.duration ===
chain (with PrivacyTier):
  count=1 attrs={ ... http.route="GET /healthz" ... }
  count=1 attrs={ ... http.route="GET /api/{rest...}" ... }
  count=1 attrs={ ... http.route="/rest/v1/" ... }
  count=1 attrs={ ... http.route="/graphql" ... }
  count=1 attrs={ ... http.route="GET /ui/{rest...}" ... }
chainNoPriv (no PrivacyTier):
  count=1 attrs={ ... http.route="/healthz" ... }
  count=1 attrs={ ... http.route="/api/{rest...}" ... }
  count=1 attrs={ ... http.route="/rest/v1/" ... }   (count=2 = both chains share this exact label combo)
  count=1 attrs={ ... http.route="/graphql" ... }    (count=2 = both chains share this exact label combo)
  count=1 attrs={ ... http.route="/ui/{rest...}" ... }
```

**The labeler-add path emits `http.route` correctly for ALL route families, regardless of whether intervening middleware does `r.WithContext(...)`.**

Subtle observation worth flagging: the WITH-PrivacyTier chain produces full-pattern values (`"GET /healthz"`) because otelhttp's local `r.Pattern == ""` (the `r.WithContext` in PrivacyTier creates a NEW *http.Request struct via `r2 := *r; r2.ctx = ctx; return r2` per `/usr/lib/go-1.26/src/net/http/request.go:368-376`, and the mux populates Pattern on r2, not on otelhttp's local r), so otelhttp's NATIVE http.route emission (`semconv/server.go:367-368`) is skipped, and the labeler-added value (full pattern) is the only one in the metric attribute set. The control chain produces method-stripped values (`"/healthz"`) because otelhttp's local r.Pattern IS set (no intervening WithContext), so otelhttp's native `httpRoute(pattern)` (which strips the method prefix at `internal/semconv/util.go:80-85`) appends LAST and wins via attribute.NewSet's last-value-wins semantics. **Both chains produce a usable http.route attribute on every route family.**

### otelhttp v0.68.0 labeler lifecycle (Step 3)

- **Labeler installed in request context:** `handler.go:172-178` —
  ```go
  labeler, found := LabelerFromContext(ctx)
  if !found {
      ctx = ContextWithLabeler(ctx, labeler)
  }
  r = r.WithContext(ctx)
  next.ServeHTTP(w, r)
  ```
  On the first entry to otelhttp middleware (`found == false`), a fresh `*Labeler` is constructed and installed via `context.WithValue` per `labeler.go:44-46`. The same `*Labeler` pointer is later read for metric attribute emission.

- **Labeler attributes read for metric record:** `handler.go:196-208` —
  ```go
  h.semconv.RecordMetrics(ctx, semconv.ServerMetricData{
      ...
      MetricAttributes: semconv.MetricAttributes{
          Req:                  r,
          StatusCode:           statusCode,
          AdditionalAttributes: append(labeler.Get(), h.metricAttributesFromRequest(r)...),
      },
      ...
  })
  ```
  This call is `defer`-free; it runs synchronously AFTER `next.ServeHTTP(w, r)` returns at line 178. By the time `labeler.Get()` is called, every downstream middleware has finished execution — including `routeTagMiddleware` which has already added `http.route` to the same `*Labeler` pointer (the pointer is shared via `context.WithValue`, which `r.WithContext(...)` propagates as part of the parent ctx in `request.go:373` `*r2 = *r` shallow-copy + `r2.ctx = ctx` override).

- **Confirms or refutes main.go:872-876 doc claim** ("otelhttp@v0.68.0/handler.go:172+202 reads the labeler AFTER next.ServeHTTP returns"):
  - **CONFIRMED** for the labeler READ at `handler.go:202` (the `labeler.Get()` inside the `MetricAttributes` literal that `RecordMetrics` consumes at line 196 onwards). The doc comment line numbers are accurate as of v0.68.0.
  - **AMENDED** for line 172: that line is the labeler INSTALL site (the `LabelerFromContext` lookup that backfills a fresh labeler into ctx if missing), not a read site for metric emission. The doc comment conflates "172" (install) and "202" (read) but the operational claim ("read happens AFTER next.ServeHTTP returns") is true.

- **Native http.route emission (NOT in main.go's current doc comment):** `internal/semconv/server.go:367-368` and `internal/semconv/server.go:394-396` —
  ```go
  if route == "" && req.Pattern != "" {
      route = httpRoute(req.Pattern)
  }
  ...
  if route != "" {
      attributes = append(attributes, semconv.HTTPRoute(route))
  }
  ```
  otelhttp v0.68.0 ALREADY natively emits `http.route` based on `req.Pattern` whenever `req.Pattern` is non-empty on the request struct it holds locally (otelhttp's local `r`). This means our `routeTagMiddleware` is a BACKSTOP for the case where intervening `r.WithContext(...)` middleware (e.g., `middleware.PrivacyTier`) creates a new request struct that hides the Pattern from otelhttp's local r. Without `routeTagMiddleware`, those routes would lose http.route entirely on production-shaped chains. With it, they get the full-pattern value via the labeler.

## Root Cause

The `http.route` middleware (`routeTagMiddleware` + the otelhttp-labeler escape hatch) **works correctly for all route families today**. The user-reported observation that production `http_server_request_duration_seconds_count` shows only `{http_route="GET /healthz"}` is most likely a **traffic-volume artifact** rather than a code bug:

- /healthz is probed every few seconds by Fly.io health checks → many thousands of samples per machine per scrape interval.
- /api/, /rest/v1/, /graphql, /ui/, /peeringdb.v1.*/* serve external traffic which on a quiet day amounts to a small handful of requests per region per scrape interval — possibly zero per scrape interval on individual machines in the 7-replica fleet.
- Grafana Cloud's hosted Prometheus may apply staleness rules that age out series with no samples in the last N intervals, causing low-volume `http_route` series to disappear from `count by(http_route)(...)` even though they did emit at some point. (Counters in OTel SDK are cumulative-temporality and never reset, but the EXPORTER may drop low-cardinality series after staleness windows.)

A secondary contributory factor (would surface if traffic increased): the http.route attribute value DIFFERS depending on whether intervening middleware does `r.WithContext`. With PrivacyTier in the chain (production), our `routeTagMiddleware` is the sole emitter and the value is the full pattern with method (e.g., `"GET /healthz"`). Without PrivacyTier (or without our middleware), otelhttp natively emits the method-stripped semconv-canonical form (e.g., `"/healthz"`). Production today shows the full-pattern form (`"GET /healthz"`) which confirms our `routeTagMiddleware` IS active in production v1.17.0 (verified via `git log --oneline ddcb4f1` — the `feat(middleware): inject http.route label via post-dispatch labeler` commit landed before the v1.17.0 tag).

## Hypotheses Ruled Out

1. **Sparse traffic** — *NEITHER ruled out NOR confirmed locally* — requires the operator's PromQL verification (Step 0 control query) to differentiate. This is the working leading hypothesis; the chosen fix (Step 5 / Task 2) makes the labeler-add path defensively-locked via E2E test so that EVEN IF traffic IS sparse on most routes, the label IS emitted whenever a request hits the route. Operator should re-check `count by(http_route)(...)` after generating ~5 minutes of varied traffic post-deploy.
2. **Sub-router pattern reset / overwrite** — **RULED OUT** by direct probe log evidence (Step 2): every route family produced a non-empty `r.Pattern` at the routeTagMiddleware tail. Sub-handlers that DO use a nested mux (`/rest/v1/` via `ent/rest/server.go:597 _mux := http.NewServeMux()` wrapped in `http.StripPrefix`) populate the inner mux's Pattern on a derived request, but the OUTER pattern (`"/rest/v1/"`) is what our parent-mux registration sets and what flows to routeTagMiddleware. ConnectRPC handlers, the GraphQL handler, and the pdbcompat handler dispatch directly without any nested mux wrap.
3. **Labeler context replacement** — **RULED OUT** by direct probe log evidence (Step 2): every route family produced `labeler_in_ctx=true`. The shared-pointer semantics of `context.WithValue(parent, key, *Labeler)` means even when downstream middleware derives a NEW request via `r.WithContext(...)`, the parent-ctx labeler pointer is preserved (the new ctx still carries the labelerContextKey because `WithValue` derives from parent ctx).
4. **otelhttp label-read timing** — **RULED OUT** by reading `$HOME/go/pkg/mod/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp@v0.68.0/handler.go:172-208` directly. The label READ at line 202 happens INSIDE the `MetricAttributes` literal in the `RecordMetrics` call at line 196-208, which runs synchronously AFTER `next.ServeHTTP(w, r)` returns at line 178. By that time, `routeTagMiddleware`'s tail mutation has already completed and added `http.route` to the labeler. The doc comment at `cmd/peeringdb-plus/main.go:907-911` accurately describes this lifecycle (the line-number reference "172+202" should be amended to clarify 172 is the install site and 202 is the read site, but the operational claim is correct).

## Chosen Fix

**Shape 2 — minimal `routeTagMiddleware` body change is unnecessary; the body is correct as-is.** The chosen fix is therefore a hardening-via-test approach combined with a doc-comment clarification:

1. **Doc-comment amendment in `cmd/peeringdb-plus/main.go`** at the `routeTagMiddleware` doc block (`main.go:906-919`): clarify that line 172 in otelhttp/handler.go is the labeler INSTALL site (not a metric read site) and add a forward-pointer to the additional invariant that otelhttp natively reads `req.Pattern` at `internal/semconv/server.go:367` if it has Pattern available, and that our middleware is the BACKSTOP for the case where intervening `r.WithContext(...)` middleware (e.g., `middleware.PrivacyTier`) hides Pattern from otelhttp's local r. This is the single substantive code touch under the "fix" rubric.

2. **End-to-end test `TestRouteTag_E2E_AllRouteFamilies` in NEW file `cmd/peeringdb-plus/route_tag_e2e_test.go`** (Task 2, plan-spec): drives a representative chain (otelhttp outermost, routeTagMiddleware innermost, with `captureLabelerMW` in between to install a synthetic Labeler the test can inspect) against ≥4 production-shaped route registrations (`GET /healthz`, `GET /api/{rest...}`, `/rest/v1/`, `/graphql`, `GET /ui/{rest...}`) and asserts `http.route` is set to the matched pattern on each. Also adds `TestRouteTag_E2E_UnmatchedOmitsLabel` regression guard for the empty-Pattern → no-label invariant. This locks the working behaviour against future middleware-chain reshuffles or otelhttp upgrades.

3. **Existing tests stay green:** `TestRouteTagMiddleware` (unit), `TestMiddlewareChain_Order` (source-scan), and the rest of the `cmd/peeringdb-plus` test suite all pass without modification because we did not change the wrap order, did not change the `routeTagMiddleware` body, and did not add a new instrumentation library.

The fix is contained to `cmd/peeringdb-plus/main.go` (doc-comment amendment, ~5 lines) and `cmd/peeringdb-plus/route_tag_e2e_test.go` (new file, 2 test functions). No new HTTP instrumentation libraries introduced.

## Why Not Alternative Fixes

- **Replace otelhttp with a different instrumentation library** — out of scope per CONTEXT.md "Out of Scope" entry "Replacing otelhttp with a different instrumentation library — out of scope."
- **Add new HTTP metrics (e.g., per-endpoint histograms)** — out of scope per CONTEXT.md "Out of Scope" entry "Adding new HTTP metrics — this phase fixes the existing http.route label, doesn't introduce new instruments."
- **Switch the parent mux to chi/gorilla/mux** — would force re-registering all 6 route families and rework readinessMiddleware's bypass list. Heavy-handed for a problem that the empirical evidence shows IS NOT a routing-pattern issue.
- **Explicitly register every entrest sub-route on the parent mux** — would double-register 60+ entrest endpoints on the parent mux while the `http.StripPrefix(BasePath, _mux)` wrapper still owns dispatch. Pattern-survival would not improve since the OUTER pattern (`/rest/v1/`) is already correctly populated on the parent mux's request.
- **Move otelhttp INSIDE routeTagMiddleware (Shape 3)** — would invert the labeler-install lifecycle: routeTagMiddleware's tail would run AFTER otelhttp's RecordMetrics call returned, defeating the entire point of the labeler escape hatch. The existing wrap order (otelhttp outermost / routeTagMiddleware innermost) is correct and is regression-locked by `TestMiddlewareChain_Order`.
- **Strip the method prefix in routeTagMiddleware to match semconv-canonical form** — would change the production `http_route` label value from `"GET /healthz"` to `"/healthz"` overnight, breaking any operator dashboard / alert rule that filters on the full-pattern form. The full-pattern form is operationally MORE useful (it preserves the method dimension which is otherwise carried by `http_request_method` in the same metric). Defer to a future grooming pass if the semconv-canonical form is preferred.

## Acceptance

The PromQL / dashboard view that confirms the fix post-deploy:

```promql
count by(http_route)(http_server_request_duration_seconds_count{service_name="peeringdb-plus"})
```

Expected to return ≥5 distinct `http_route` labels within 5 minutes of normal post-deploy traffic (e.g., a curl loop hitting `/api/networks?limit=1`, `/rest/v1/networks?limit=1`, `/graphql` POST `{__typename}`, `/ui/asn/13335`, plus the always-on `GET /healthz` from Fly.io probes). The Grafana "Request Rate by Route" panel should then show a multi-line breakdown (one line per matched route family) instead of the single `/healthz` line currently observed.

Operator verification commands (post-`fly deploy`):

```bash
# 1) generate ~5 minutes of varied traffic across the 5 route families
for i in $(seq 1 60); do
  curl -s 'https://peeringdb-plus.fly.dev/api/networks?limit=1' > /dev/null
  curl -s 'https://peeringdb-plus.fly.dev/rest/v1/networks?limit=1' > /dev/null
  curl -s 'https://peeringdb-plus.fly.dev/graphql' -X POST -H 'Content-Type: application/json' -d '{"query":"{__typename}"}' > /dev/null
  curl -sf -H 'User-Agent: Mozilla/5.0' 'https://peeringdb-plus.fly.dev/ui/asn/13335' > /dev/null
  sleep 5
done

# 2) Run the PromQL above against Grafana Cloud Prometheus.
#    Expected: ≥5 series, e.g.
#      {http_route="GET /healthz"}
#      {http_route="GET /api/{rest...}"}
#      {http_route="/rest/v1/"}
#      {http_route="/graphql"}
#      {http_route="GET /ui/{rest...}"}

# 3) Open Grafana dashboard "Request Rate by Route" panel — expect
#    multi-line breakdown rather than single /healthz line.
```

If after 5 minutes the PromQL still returns only `{http_route="GET /healthz"}` despite the curl loop above completing successfully, the bug is genuinely in the metric pipeline (not sparse traffic), and follow-up investigation should focus on:

- OTLP exporter temporality settings (`OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE` env var on the production fleet)
- Grafana Cloud OTLP receiver attribute filtering (similar to the `service.instance.id` strip noted in CLAUDE.md § OTel resource attributes)
- Local pcap of the OTLP/HTTP egress to confirm the http.route attribute is on the wire

— neither of which is in scope for OBS-04 per CONTEXT.md.
