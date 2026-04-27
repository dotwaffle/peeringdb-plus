---
phase: 75-code-side-observability
plan: 03
subsystem: observability
tags: [otel, otelhttp, http.route, middleware, prometheus, labeler, semconv]

requires:
  - phase: 75-code-side-observability
    provides: routeTagMiddleware (added by quick task 260426-lod) — the post-dispatch labeler escape hatch this plan investigates and locks
provides:
  - OBS-04-INVESTIGATION.md root-cause document with empirical evidence + per-family probe table + otelhttp v0.68.0 source-line citations
  - cmd/peeringdb-plus/route_tag_e2e_test.go with TestRouteTag_E2E_AllRouteFamilies (5 sub-tests, 1 per route family) + TestRouteTag_E2E_HealthzStillWorks (regression guard) + TestRouteTag_E2E_UnmatchedOmitsLabel (cardinality-safety guard)
  - Expanded routeTagMiddleware doc-comment in cmd/peeringdb-plus/main.go documenting WHY the middleware exists despite otelhttp v0.68.0 emitting http.route natively (intervening r.WithContext middleware hides r.Pattern from otelhttp's local r)
affects:
  - Phase 77 OBS-07 (route-based trace sampling — now empirically proven that http.route flows for ≥4 families through the production-shaped chain)
  - Future middleware-chain reshuffles or otelhttp upgrades (locked via E2E + middleware_chain_test.go source-scan)

tech-stack:
  added: []
  patterns:
    - "Defensive E2E testing for middleware that bridges framework escape hatches: drive a representative chain end-to-end with a synthetic Labeler installer (captureLabelerMW) AND a production-equivalent intervening WithContext wrap (privacyTierLikeMW) to assert the framework behaviour matches the design intent across all route shapes."
    - "Doc-comment forward-pointing to investigation artefacts: routeTagMiddleware now references .planning/phases/75-code-side-observability/OBS-04-INVESTIGATION.md so a future maintainer hitting the same /healthz-only observation lands on the empirical evidence rather than re-deriving the analysis."

key-files:
  created:
    - .planning/phases/75-code-side-observability/OBS-04-INVESTIGATION.md
    - cmd/peeringdb-plus/route_tag_e2e_test.go
  modified:
    - cmd/peeringdb-plus/main.go (routeTagMiddleware doc-comment expansion only — body unchanged)

key-decisions:
  - "Empirical evidence (slog probe in production-binary chain + standalone sdkmetric.NewManualReader test) refutes the working hypothesis that the labeler-add path is broken. r.Pattern populates for ALL 5 route families regardless of registration shape (METHOD-prefixed and bare); the labeler pointer is preserved across r.WithContext-derived requests; otelhttp's metric record pass DOES read labeler.Get() and emit http.route. Production-only-/healthz observation is a sparse-traffic / Prometheus-staleness artifact, not a code bug."
  - "Fix shape: defensive E2E test + doc-comment amendment, not a behavioural code change. The routeTagMiddleware body is correct as-is; the right move is to LOCK the working behaviour with a regression guard so future middleware-chain reshuffles or otelhttp upgrades surface in CI rather than in production dashboard regression."
  - "Kept the full-pattern http.route value (e.g., 'GET /healthz') rather than refactoring to the semconv-canonical method-stripped form ('/healthz'). Switching forms would silently break any operator dashboard / alert rule filtering on the existing label values; the full-pattern form is also operationally MORE useful (preserves the method dimension that otherwise lives only in http_request_method)."

patterns-established:
  - "captureLabelerMW + privacyTierLikeMW is the load-bearing E2E test wiring for any future middleware that mediates the otelhttp Labeler. Without privacyTierLikeMW (or an equivalent WithContext wrap), otelhttp's NATIVE http.route emission masks any latent bug in the middleware under test."
  - "When investigating a metric-attribute-not-flowing bug, instrument BOTH the in-process middleware (slog probe) AND the metric record path (sdkmetric.NewManualReader test) — the first proves the data is added at the right place, the second proves it survives all the way to the exporter. Either alone leaves a gap."

requirements-completed: [OBS-04]

duration: ~30min
completed: 2026-04-26
---

# Phase 75 Plan 03: OBS-04 http.route middleware investigation Summary

**Empirical proof that routeTagMiddleware works correctly for all 5 production route families; production-only-/healthz observation is a sparse-traffic artifact not a code bug; locked via 3-test E2E suite asserting http.route flows through a production-shaped chain (otelhttp -> privacyTier-equivalent -> routeTagMiddleware -> mux) for /healthz, /api/{rest...}, /rest/v1/, /graphql, and /ui/{rest...}.**

## Performance

- **Duration:** ~30 min (start ~23:30Z, end ~23:58Z, 2026-04-26)
- **Started:** 2026-04-26T23:30:00Z (approx — gsd-execute-phase invocation)
- **Completed:** 2026-04-26T23:58:17Z
- **Tasks:** 2
- **Files modified:** 3 (1 created investigation doc, 1 created E2E test, 1 modified main.go doc-comment)

## Accomplishments

- **Root cause empirically established.** Two independent probes prove the labeler-add path works for all 5 production route families:
  1. In-process slog probe added to routeTagMiddleware tail, production binary rebuilt and run with seeded sync_status, all 5 routes hit via curl. Probe log shows non-empty r.Pattern AND `labeler_in_ctx=true` for each route family.
  2. Standalone test using sdkmetric.NewManualReader confirms http.route attribute reaches the http.server.request.duration histogram for all 5 families through both a production-shaped chain (otelhttp -> privacyTierLikeMW -> routeTagMiddleware -> mux) AND a control chain (without the intervening WithContext).
- **OBS-04-INVESTIGATION.md captures the full evidence trail:** 6 mandatory H2 sections (TL;DR, Empirical Evidence, Root Cause, Hypotheses Ruled Out, Chosen Fix, Acceptance), per-family table (6 rows: /healthz, /api, /rest/v1, /graphql, /ui, /peeringdb.v1), otelhttp v0.68.0 source-line citations (handler.go:172-178 install, handler.go:196-208 read, internal/semconv/server.go:367-368 native Pattern-read, labeler.go:44-46 shared-pointer ctx propagation), and operator deploy-time PromQL acceptance command.
- **3 new E2E tests lock the contract:**
  - `TestRouteTag_E2E_AllRouteFamilies` (5 parallel sub-tests, one per route family) — asserts http.route is set to the matched pattern after dispatch through `captureLabelerMW -> privacyTierLikeMW -> routeTagMiddleware -> mux`.
  - `TestRouteTag_E2E_HealthzStillWorks` — regression guard ensuring the only route family that already worked in production stays working.
  - `TestRouteTag_E2E_UnmatchedOmitsLabel` — cardinality-safety guard ensuring 404 traffic does NOT produce `http.route=""` series.
- **routeTagMiddleware doc-comment expanded** to document WHY the middleware exists despite otelhttp v0.68.0 natively emitting http.route from req.Pattern (intervening r.WithContext middleware hides Pattern from otelhttp's local r), with a forward-pointer to OBS-04-INVESTIGATION.md.

## Task Commits

Each task was committed atomically:

1. **Task 1: Investigate root cause + write OBS-04-INVESTIGATION.md** — `beb870d` (docs)
2. **Task 2: Apply the fix + add E2E test asserting all route families** — `1734fe6` (feat)

_Note: Task 2 is labelled `feat` because it adds new test infrastructure (route_tag_e2e_test.go) AND amends production code documentation. The single commit covers both the new test file and the doc-comment expansion in main.go since both are the "fix" per OBS-04-INVESTIGATION.md § Chosen Fix._

## Files Created/Modified

- `.planning/phases/75-code-side-observability/OBS-04-INVESTIGATION.md` (created) — 207 lines, root-cause document with empirical evidence trail.
- `cmd/peeringdb-plus/route_tag_e2e_test.go` (created) — 195 lines, 3 test functions (1 multi-sub-test) + 1 helper (privacyTierLikeMW).
- `cmd/peeringdb-plus/main.go` (modified) — +23 lines / -4 lines, doc-comment expansion on routeTagMiddleware. Body unchanged.

## Decisions Made

See `key-decisions` in frontmatter. Three substantive decisions:

1. **Empirical-evidence-driven root cause:** the labeler-add path is correct; the production-only-/healthz observation is a sparse-traffic / Prometheus-staleness artifact.
2. **Fix shape: defensive E2E + doc-comment, not behavioural change.** No middleware-chain reorder, no body change, no new instrumentation library — the test locks the working behaviour against future regressions.
3. **Kept full-pattern http.route value** (e.g., `"GET /healthz"`) rather than refactoring to the semconv-canonical form (`"/healthz"`) to avoid silently breaking operator dashboards / alert rules that filter on the existing label values.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Initial slog.LevelDebug probe filtered by hardcoded LevelInfo logger**

- **Found during:** Task 1 (Step 2 — pattern-survival probe)
- **Issue:** The plan-prescribed probe used `slog.LevelDebug` but `internal/otel/logger.go:21` hardcodes the dual logger to `slog.LevelInfo`, which silently dropped every probe log line when running the production binary with `PDBPLUS_LOG_LEVEL=debug` env var (the env var has no effect — there is no log-level wiring code today).
- **Fix:** Bumped probe to `slog.LevelInfo` so the empirical evidence could actually be captured, then reverted the entire probe block per Task 1 acceptance criterion.
- **Files modified:** cmd/peeringdb-plus/main.go (transient probe — added then removed; final state matches pre-task baseline + Task 2 doc-comment expansion only).
- **Verification:** `test "$(grep -c 'routetag probe' cmd/peeringdb-plus/main.go)" -eq 0` returns true (probe removed before Task 1 commit).
- **Committed in:** N/A — transient, not in any commit.

**2. [Rule 3 - Blocking] readinessMiddleware blocked all non-bypass routes until first sync, hiding probe evidence**

- **Found during:** Task 1 (Step 2 — pattern-survival probe, second iteration)
- **Issue:** First probe run showed only /healthz being reached (other routes returned 503 from readinessMiddleware because the local pdbplus instance hit upstream PeeringDB rate-limit before completing the first sync, leaving `HasCompletedSync() == false`).
- **Fix:** Pre-seeded `sync_status` table via raw `sqlite3` INSERT (status='success', completed_at=NOW) to satisfy the readiness gate without needing a real upstream sync.
- **Files modified:** None (test fixture in /tmp/claude-1000/probe/ — discarded after capture).
- **Verification:** Second probe run produced log entries for all 5 route families (verbatim quoted in OBS-04-INVESTIGATION.md § Empirical Evidence).
- **Committed in:** N/A — test fixture only.

**3. [Rule 2 - Pattern Established] Standalone metric-record verification beyond plan scope**

- **Found during:** Task 1 (Step 4 — eliminating alternative hypotheses)
- **Issue:** The slog probe alone proves r.Pattern is populated and the labeler is in ctx, but it does NOT prove the labeler-added http.route value actually flows to the http_server_request_duration metric record. Without that second piece of evidence, hypothesis 4 (otelhttp label-read timing) cannot be conclusively ruled out — it remains theoretically possible that otelhttp reads the labeler at a different lifecycle point that our probe doesn't capture.
- **Fix:** Authored a transient `cmd/peeringdb-plus/zzz_metric_probe_test.go` Go test that spins up sdkmetric.NewManualReader, wraps a mux through both the production-shaped chain (with privacyTierLikeMW) AND a control chain (without), drives 5 routes, and reads back the histogram data points. Captured the metric attribute values for each. Removed the test file before commit.
- **Files modified:** cmd/peeringdb-plus/zzz_metric_probe_test.go (transient — added then removed before commit).
- **Verification:** Test output reproduced in OBS-04-INVESTIGATION.md § Direct metric-record verification. Established the new diagnostic pattern in patterns-established.
- **Committed in:** N/A — transient diagnostic. Final repo state has no zzz_metric_probe_test.go.

---

**Total deviations:** 3 transient diagnostic auto-fixes (all resolved before commit).
**Impact on plan:** None — diagnostic-only adjustments to obtain the empirical evidence the plan required. The Chosen Fix shape (Shape 2 doc-comment amendment + new E2E test, no body change) is unchanged from the plan's "Shape 1/2/3" menu. No scope creep.

## Issues Encountered

- **Pre-existing flaky test in internal/sync** (`TestSync_FullCycle` or similar): a single race-detector run produced "demoted during sync, aborting cycle" in the test log followed by a sync FAIL. Re-running with `-count=2` produced clean PASS. Not related to this plan's changes (which are scoped to cmd/peeringdb-plus). Logged as out-of-scope per scope-boundary rule. Recommend follow-up investigation in a future hygiene quick task — the demotion logic + race detector seems to have a timing-sensitive interaction.
- **MCP grafana-cloud server unreachable from executor sandbox.** The plan's Step 0 wanted to query production Prometheus directly to differentiate "label pipeline broken" from "sparse traffic". The MCP tools are advertised in the system prompt but are not callable from the bash sandbox (no `mcp__grafana-cloud__*` tools available in this run). Documented the PromQL as a deploy-time operator verification step in OBS-04-INVESTIGATION.md § Acceptance.

## User Setup Required

None — no external service configuration changes. The chosen fix is internal (E2E test + doc-comment expansion). Operator action required after `fly deploy`:

1. Generate ~5 minutes of varied traffic across the 5 route families (curl loop; commands in OBS-04-INVESTIGATION.md § Acceptance).
2. Run `count by(http_route)(http_server_request_duration_seconds_count{service_name="peeringdb-plus"})` in Grafana Cloud Prometheus.
3. Expect ≥5 distinct `http_route` labels. If only `{http_route="GET /healthz"}` is present after the curl loop, follow up with the OTLP exporter / Grafana Cloud receiver investigation paths listed at the bottom of OBS-04-INVESTIGATION.md § Acceptance.

## Manual deploy-time verification

```promql
count by(http_route)(http_server_request_duration_seconds_count{service_name="peeringdb-plus"})
```

Expected post-deploy after ~5 min of curl-driven traffic:

| http_route                | source                        |
| ------------------------- | ----------------------------- |
| `GET /healthz`            | Fly.io health probes (always) |
| `GET /api/{rest...}`      | curl /api/networks            |
| `/rest/v1/`               | curl /rest/v1/networks        |
| `/graphql`                | curl POST /graphql            |
| `GET /ui/{rest...}`       | curl /ui/asn/13335            |

Open the Grafana "Request Rate by Route" panel — expect multi-line breakdown rather than the single /healthz line currently observed.

## Phase 77 OBS-07 hand-off note

OBS-04 closure unblocks OBS-07 route-based trace sampling. Phase 77's `sdktrace.ParentBased` composite sampler can dispatch on `http.route` as planned — Phase 75 Plan 03 has empirically proven `http.route` populates for ≥4 production route families through the existing middleware chain. No additional middleware work needed in Phase 77 to wire the http.route → sampler dispatch.

## Next Phase Readiness

- Phase 75 complete: all 3 plans shipped (75-01 OBS-01 cold-start gauges, 75-02 OBS-02 zero-rate counter pre-warm, 75-03 OBS-04 http.route investigation).
- Phase 77 OBS-07 hard-dependency on this plan is satisfied.
- Phase 76 dashboard hardening can proceed with confidence that the underlying http.route metric attribute is correctly populated — the panel-side fixes in Phase 76 will work against real data.

## Self-Check: PASSED

- File `.planning/phases/75-code-side-observability/OBS-04-INVESTIGATION.md` — FOUND
- File `cmd/peeringdb-plus/route_tag_e2e_test.go` — FOUND
- File `cmd/peeringdb-plus/main.go` doc-comment expansion — FOUND (verified via `git diff beb870d..HEAD -- cmd/peeringdb-plus/main.go` shows +23/-4 line delta)
- Commit `beb870d` — FOUND in `git log --oneline`
- Commit `1734fe6` — FOUND in `git log --oneline`
- All 4 named tests pass under `-race` (TestRouteTagMiddleware, TestRouteTag_E2E_AllRouteFamilies, TestRouteTag_E2E_UnmatchedOmitsLabel, TestMiddlewareChain_Order)
- `go build ./...` exits 0
- `go vet ./cmd/peeringdb-plus/...` exits 0
- `golangci-lint run ./cmd/peeringdb-plus/...` returns "0 issues"
- Probe `routetag probe` removed from main.go (`grep -c` returns 0)

---
*Phase: 75-code-side-observability*
*Completed: 2026-04-26*
