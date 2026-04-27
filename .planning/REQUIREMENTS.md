# Requirements: PeeringDB Plus — v1.18.0

**Defined:** 2026-04-26
**Core Value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.
**Theme:** Cleanup & observability polish — close all outstanding deferred items from prior milestones, fix the two known production bugs, complete a Grafana Cloud telemetry audit & remediation pass, and close the small UAT slice that's been pending since v1.13. Goal is a durable observability story before the next feature cycle (v1.19+).

## Source-of-truth references

Every requirement below cites a deferred-items.md, a SUMMARY.md "Out of Scope" entry, a memory record, or an empirical telemetry audit performed against live `peeringdb-plus.fly.dev` Grafana Cloud on 2026-04-26. The canonical references:

- `.planning/milestones/v1.14-phases/58-visibility-schema-alignment/deferred-items.md` (5 lint findings)
- `.planning/milestones/v1.16-phases/67-default-ordering-flip/deferred-items.md` (TestGenerateIndexes)
- `.planning/milestones/v1.16-phases/70-cross-entity-traversal/deferred-items.md` (DEFER-70-06-01 campus inflection)
- `.planning/quick/260426-mei-production-observability-alerts-per-inst/deferred-items.md` (TestDashboard_RegionVariableUsed)
- `.planning/quick/260426-pms-flip-default-pdbplus-sync-mode-to-increm/260426-pms-SUMMARY.md` § Out of Scope Findings (`poc.role` NotEmpty)
- `memory/project_human_verification.md` § v1.13 (CSP + headers)
- Live audit 2026-04-26: Grafana Cloud `pdbplus-overview` dashboard + Prometheus metric inventory

## v1.18.0 Requirements

### Code Defects (BUG)

- [x] **BUG-01**: `GET /api/<type>?campus__<field>=<value>` cross-entity traversal returns HTTP 200 (currently 500: `SQL logic error: no such table: campus (1)`). Root cause is `cmd/pdb-compat-allowlist` calling `e.Type.Table()` via entc.LoadGraph, which doesn't apply the `fixCampusInflection` patch from `ent/entc.go`; result is `internal/pdbcompat/allowlist_gen.go` emitting `TargetTable: "campus"` for incoming edges to Campus while the runtime uses `"campuses"`. Acceptance: `traversal_e2e_test.go` `path_a_1hop_fac_campus_name` passes; `?fac?campus__name=X` returns matching facilities. Tracked at DEFER-70-06-01.
- [x] **BUG-02**: Sync upsert path accepts upstream `poc.role=""` tombstones without validator failure. Currently `poc.role` retains `field.String("role").NotEmpty()` from the schema generator; if upstream PeeringDB scrubs `role` on tombstone (matching the `name=""` GDPR pattern already empirically confirmed for org/network/etc.), incremental sync will fail at the validator and abort the cycle. Fix shape mirrors the 260426-pms `name` change: extend `cmd/pdb-schema-generate` to skip emitting `NotEmpty()` on `role` (and any other tombstone-vulnerable fields surfaced during planning). Acceptance: `go generate ./...` produces `ent/schema/poc.go` without `.NotEmpty()` on role; conformance test seeds a poc tombstone with `role=""` and asserts the upsert succeeds.

### Test / CI Debt (TEST)

- [ ] **TEST-01**: `cmd/pdb-schema-generate/TestGenerateIndexes` passes against current main (currently fails: `unexpected index "updated"`). Plan 67-01 added the `"updated"` index to `generateIndexes()` but the test's allow-list-style assertion rejects it. Fix is test-level: extend the allow-list (or invert to a deny-list). Acceptance: `go test ./cmd/pdb-schema-generate/...` passes on a clean tree without `-skip` flags.
- [ ] **TEST-02**: `deploy/grafana/dashboard_test.go:316` `TestDashboard_RegionVariableUsed` passes against current dashboard (currently fails: no PromQL references `fly_region`). The test went stale after the post-260426-lod migration moved region grouping to `cloud_region`. Fix: assert against `cloud_region` instead, or rework the `$region` template variable. Acceptance: `go test ./deploy/grafana/...` passes; dashboard region template variable correctly drives a panel query.
- [ ] **TEST-03**: `golangci-lint run ./internal/visbaseline/...` returns `0 issues`. Currently 5 issues: 1 exhaustive (missing `shapeUnknown` case), 3 gosec G304 (CLI-supplied `os.ReadFile` / `os.OpenFile` paths), 1 nolintlint (unused `//nolint:gosec` directive). Fix is hygiene-only — either resolve each finding properly (G304: `filepath.Clean` + safe-root validation; exhaustive: add the missing case) or document with explicit `//nolint:<linter> // reason` blocks. Acceptance: lint clean on the package; CI drift gate green.

### Observability (OBS)

- [x] **OBS-01**: `pdbplus_data_type_count` gauge reports correct row counts within 30s of process startup (currently reads 0 for ~15 min until the first sync cycle completes). Fix: populate the cache from a one-shot `COUNT(*)` query at process init, separate from the sync-completion path that currently primes it. Acceptance: post-deploy, the "Total Objects" / "Objects by Type" / "Object Counts Over Time" panels never show false zeros.
- [x] **OBS-02**: Zero-rate panels (Fallback Events, Role Transitions, Fetch / Upsert Errors, Deletes per Type) display "0" rather than "No data" when nothing has fired. Root cause: OTel cumulative counters only export points after the first `.Add()`; the named metrics (`pdbplus_sync_type_fallback_total`, `pdbplus_role_transitions_total`, `pdbplus_sync_type_upsert_errors_total`, `pdbplus_sync_type_fetch_errors_total`, `pdbplus_sync_type_deleted_total`) have never been recorded in steady state. Fix: pre-warm with `.Add(ctx, 0, baseline_attrs)` at startup so each (type, status) tuple registers with at least one observation. Acceptance: dashboard panels render `0` instead of `No data` after a fresh deploy on a healthy fleet.
- [ ] **OBS-03**: All `go_*` panel queries in `deploy/grafana/dashboards/pdbplus-overview.json` filter by `service_name="peeringdb-plus"`. Currently they don't — the system works only because the local syncthing scrape happens to use `go_goroutines` (plural) while peeringdb-plus uses `go_goroutine_count` (singular). Future scrape targets sharing metric names with us would silently double-count. Acceptance: dashboard panels filter on `service_name`; query against a shared Prom shows clean panels even when other applications emit `go_*` metrics.
- [x] **OBS-04**: `http_route` label on `http_server_request_duration_seconds_count` populates for all routes, not just `GET /healthz`. Currently the only series visible carries `http_route="GET /healthz"` despite real traffic against `/api/*`, `/rest/v1/*`, `/peeringdb.v1.*`, `/graphql`, and `/ui/*`. Investigate `routeTagMiddleware` (added post-260426-lod): is `r.Pattern` empty for non-mux routes? Is the middleware ordering wrong? Acceptance: dashboard "Request Rate by Route" shows multi-route breakdown; `count by(http_route)(http_server_request_duration_seconds_count)` returns ≥5 routes during normal traffic.
- [x] **OBS-05**: `pdbplus_response_heap_delta_bytes_bucket` (post-bytes-canonicalisation name) flowing on the v1.17.0+ binary; the legacy `pdbplus_response_heap_delta_kib_KiB_bucket` confirmed as expiring retention data, not active emission. Audit task — no code change expected unless flow is broken. Acceptance: `count(pdbplus_response_heap_delta_bytes_bucket{service_version=~"v1\.1[78]\..*"})` returns non-zero during normal pdbcompat list traffic. *(Confirmed 2026-04-27: count=13 after a synthetic /api/net request flushed the histogram. Original `v1.17.0|v1.18.*` regex was wrong because Prom anchors `=~` matches and prod labels itself `v1.17.0-64-g565b762` (git-describe). Dashboard panel-description documentation requirement was deliberately dropped per CONTEXT.md D-02 — let retention expire the legacy `_kib_KiB_*` series naturally.)*
- [ ] **OBS-06**: Loki log-level audit complete. Identify INFO logs that should be DEBUG (e.g. per-step sync progress), WARN logs that fire under non-error conditions (e.g. `_visible` field redactions on routine traffic), and DEBUG logs that should be removed. Reclassify each finding via slog level adjustments or removal. Acceptance: log-level audit document captured under `.planning/phases/<N>-*/AUDIT.md` listing each reclassification with before/after; production log volume measurably down (or qualitatively cleaner) post-merge.
- [ ] **OBS-07**: Trace sampling and batching review against PERF-08 baseline. Current `OTEL_BSP_SCHEDULE_DELAY=5s` and `OTEL_BSP_MAX_EXPORT_BATCH_SIZE=512` were set in v1.15. Verify no individual trace approaches Tempo's 7.5 MB per-trace cap (the prior FK-orphan WARN spam incident). Audit task — adjust sampling ratio or batching only if a problem is found. Acceptance: empirical Tempo trace inspection shows max trace size <2 MB during normal traffic; sampling ratio remains at 1.0 unless cardinality requires reduction.

### UAT Closeout (UAT)

- [ ] **UAT-01**: v1.13 Phase 52 CSP behaviour verified against production. With `PDBPLUS_CSP_ENFORCE=true` set, Chrome DevTools shows zero CSP violation reports on `/ui/`, `/ui/asn/13335`, and `/ui/compare`. Tailwind v4 JIT runtime behaviour specifically verified (the original concern). Acceptance: human-runtime verification documented in `.planning/phases/<N>-*/UAT-RESULTS.md`; v1.13 phase 52 deferred-items entry marked resolved.
- [ ] **UAT-02**: v1.13 Phase 53 security headers verified against production. `curl -I` output for `/ui/`, `/api/net`, `/rest/v1/networks` shows `Strict-Transport-Security`, `X-Frame-Options`, and `X-Content-Type-Options` headers per the original phase scope. The 2 MB body-cap is enforced on REST/pdbcompat surfaces but skip-listed for gRPC/ConnectRPC streams. Slowloris TCP smoke test (slow-write multi-connection probe) does not exhaust connection pool. Acceptance: human-runtime verification documented; v1.13 phase 53 deferred-items entry marked resolved.
- [ ] **UAT-03**: `.planning/milestones/v1.5-phases/20-deferred-human-verification/` directory archived. Per memory `project_human_verification.md`, all 26 v1.2-v1.4 items were verified against live deployment 2026-03-24 — the directory is just a stale pointer. Verify each item against the memory record, then move the dir into the `consumed/` style archive (matching the seed convention). Acceptance: dir relocated; STATE.md "Outstanding Human Verification" section updated to reflect closure.

## Out of Scope (Explicit Exclusions)

- **SEED-004 — tombstone GC.** Per user 2026-04-26: "I don't think we need to do SEED-004 (tombstone GC) as the amount of data involved is going to be tiny." Re-evaluate when triggers fire (storage growth >5% MoM, tombstone ratio >10%, operator request).
- **v1.6 / v1.7 / v1.11 human-verification UI/visual items (~33 items combined).** Too large to combine with the cleanup theme; deferred to a future "UI verification sweep" milestone where browser/devtools work is the explicit theme.
- **SEED-003 — primary HA hot-standby.** Separate seed, not raised; surface independently when triggers fire.
- **Removing `full` sync mode entirely.** Keep as explicit operator override per 260426-pms scope decision (first-sync, recovery, escape hatch).
- **Architectural-shape changes to OTel pipeline.** Audit-and-remediate within the existing pipeline; no new exporters or backends in scope.

## Future Requirements (Deferred)

| ID | Description | Reason | Surface |
|---|---|---|---|
| `UI-VERIFY` (placeholder) | Sweep all v1.6 / v1.7 / v1.11 deferred human-verification items | ~33 items, browser/devtools work, distinct theme | Future milestone |
| `TOMBSTONE-GC` (SEED-004) | Implement tombstone garbage collection | Triggers haven't fired; data tiny | When SEED-004 surfaces |
| `PRIMARY-HA` (SEED-003) | Primary HA hot-standby | Independent capability | When SEED-003 surfaces |

## Traceability

Maps each REQ-ID to its target phase. Filled by the roadmapper 2026-04-26.

| REQ-ID | Phase | Status |
|---|---|---|
| BUG-01 | Phase 73 — Code Defect Fixes | open |
| BUG-02 | Phase 73 — Code Defect Fixes | open |
| TEST-01 | Phase 74 — Test & CI Debt | open |
| TEST-02 | Phase 74 — Test & CI Debt | open |
| TEST-03 | Phase 74 — Test & CI Debt | open |
| OBS-01 | Phase 75 — Code-side Observability Fixes | open |
| OBS-02 | Phase 75 — Code-side Observability Fixes | open |
| OBS-03 | Phase 76 — Dashboard Hardening | open |
| OBS-04 | Phase 75 — Code-side Observability Fixes | open |
| OBS-05 | Phase 76 — Dashboard Hardening | open |
| OBS-06 | Phase 77 — Telemetry Audit & Cleanup | open |
| OBS-07 | Phase 77 — Telemetry Audit & Cleanup | open |
| UAT-01 | Phase 78 — UAT Closeout | open |
| UAT-02 | Phase 78 — UAT Closeout | open |
| UAT-03 | Phase 78 — UAT Closeout | open |
