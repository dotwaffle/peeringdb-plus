# Roadmap: PeeringDB Plus

## Milestones

- ✅ **v1.0 – v1.14** — shipped (see [MILESTONES.md](./MILESTONES.md))
- ✅ **v1.15 — Infrastructure Polish & Schema Hygiene** — shipped 2026-04-18 (Phases 63-66, 11 requirements)
- ✅ **v1.16 — Django-compat Correctness** — shipped 2026-04-19 (Phases 67-72, 25 requirements)
- 🚧 **v1.18.0 — Cleanup & Observability Polish** — in planning (Phases 73-78, 15 requirements)

## Phases

**v1.18.0 — Cleanup & Observability Polish (current)**

- [ ] **Phase 73: Code Defect Fixes** — Resolve the two known production bugs (campus inflection 500 in cross-entity traversal; `poc.role` NotEmpty validator extending the codegen-layer guard from 260426-pms)
- [ ] **Phase 74: Test & CI Debt** — Three deferred test failures (`TestGenerateIndexes`, `TestDashboard_RegionVariableUsed`) + 5 lint findings in `internal/visbaseline` flipped clean
- [ ] **Phase 75: Code-side Observability Fixes** — Cold-start gauge populate (OBS-01), zero-rate counter pre-warm (OBS-02), `http.route` middleware investigation (OBS-04)
- [ ] **Phase 76: Dashboard Hardening** — `service_name` filter sweep across `go_*` panel queries (OBS-03); confirm `pdbplus_response_heap_delta_bytes_bucket` flow on v1.17.0+ binary (OBS-05)
- [ ] **Phase 77: Telemetry Audit & Cleanup** — Loki log-level audit (OBS-06); trace sampling/batching review against PERF-08 baseline (OBS-07)
- [ ] **Phase 78: UAT Closeout** — v1.13 CSP enforcement verification (UAT-01), v1.13 security headers + body cap + slowloris (UAT-02), v1.5 Phase 20 archive (UAT-03)

<details>
<summary>✅ v1.16 — Django-compat Correctness (Phases 67-72) — SHIPPED 2026-04-19</summary>

- [x] Phase 67: Default ordering flip (6 plans)
- [x] Phase 68: Status × since matrix + limit=0 semantics (4 plans)
- [x] Phase 69: Filter-value Unicode folding, operator coercion, __in robustness (6 plans)
- [x] Phase 70: Cross-entity __ traversal (Path A + Path B + 2-hop) (8 plans)
- [x] Phase 71: Memory-safe response paths on 256 MB replicas (6 plans)
- [x] Phase 72: Upstream parity regression + divergence docs (6 plans)

Archive: [`.planning/milestones/v1.16-ROADMAP.md`](./milestones/v1.16-ROADMAP.md)
Requirements: [`.planning/milestones/v1.16-REQUIREMENTS.md`](./milestones/v1.16-REQUIREMENTS.md)
Audit: [`.planning/milestones/v1.16-MILESTONE-AUDIT.md`](./milestones/v1.16-MILESTONE-AUDIT.md)

</details>

<details>
<summary>✅ v1.15 — Infrastructure Polish & Schema Hygiene (Phases 63-66) — SHIPPED 2026-04-18</summary>

- [x] Phase 63: Schema hygiene — drop vestigial columns (1 plan)
- [x] Phase 64: Field-level privacy — ixlan.ixf_ixp_member_list_url (3 plans)
- [x] Phase 65: Asymmetric Fly fleet — process groups + ephemeral replicas (2 plans)
- [x] Phase 66: Observability + sqlite3 tooling (3 plans)

Archive: [`.planning/milestones/v1.15-ROADMAP.md`](./milestones/v1.15-ROADMAP.md)
Requirements: [`.planning/milestones/v1.15-REQUIREMENTS.md`](./milestones/v1.15-REQUIREMENTS.md)
Audit: [`.planning/milestones/v1.15-MILESTONE-AUDIT.md`](./milestones/v1.15-MILESTONE-AUDIT.md)

</details>

All shipped milestones are summarised in [MILESTONES.md](./MILESTONES.md). Per-milestone ROADMAP snapshots live at `.planning/milestones/v{X.Y}-ROADMAP.md`, and phase artifacts (plans, summaries, verification reports) at `.planning/milestones/v{X.Y}-phases/` (archived) or `.planning/phases/` (current milestone).

## Phase Details

### Phase 73: Code Defect Fixes
**Goal**: The two known production defects from the v1.16 / 260426-pms tail are fixed and regression-locked: `campus__<field>=` cross-entity traversal returns 200, and `poc.role` no longer carries a `NotEmpty()` validator that aborts incremental sync on tombstoned rows.
**Depends on**: Nothing (independent of all other v1.18.0 phases).
**Requirements**: BUG-01, BUG-02
**Success Criteria** (what must be TRUE):
  1. `GET /api/<type>?campus__<field>=<value>` returns HTTP 200 with the matching rows for every entity type that has a `campus` FK; the existing `traversal_e2e_test.go` `path_a_1hop_fac_campus_name` sub-test passes without `t.Skip`
  2. The DEFER-70-06-01 entry in `.planning/milestones/v1.16-phases/70-cross-entity-traversal/deferred-items.md` and the matching `DIVERGENCE_` canary in `internal/pdbcompat/parity/traversal_test.go` flip from "documented divergence" to "fixed", with `docs/API.md § Known Divergences` updated accordingly
  3. `go generate ./...` produces an `ent/schema/poc.go` (and any other tombstone-vulnerable schema files surfaced during planning) without `.NotEmpty()` on `role`; the codegen drift gate stays green
  4. A conformance / unit test seeds a `poc` tombstone with `role=""` and the `internal/sync` upsert path completes the cycle without a validator-aborted rollback
**Plans** (2 plans, both Wave 1, can run in parallel):
- [x] 73-01-PLAN.md — BUG-01: Campus inflection — entsql.Annotation{Table: "campuses"} sibling-file mixin + traversal E2E un-skip + parity DIVERGENCE canary flip + docs cleanup
- [x] 73-02-PLAN.md — BUG-02: Audit + drop NotEmpty() on poc.role at codegen layer (cmd/pdb-schema-generate isTombstoneVulnerableField predicate); httptest TestSync_IncrementalRoleTombstone regression guard

### Phase 74: Test & CI Debt
**Goal**: A clean-tree CI run (`go test ./... && golangci-lint run ./...`) passes without `-skip` flags or scope-boundary excuses — the three deferred test failures and the five `internal/visbaseline` lint findings are resolved.
**Depends on**: Nothing (independent — the test failures are unrelated to the BUG-01/02 fixes).
**Requirements**: TEST-01, TEST-02, TEST-03
**Success Criteria** (what must be TRUE):
  1. `go test ./cmd/pdb-schema-generate/...` passes — `TestGenerateIndexes` accepts the `"updated"` index that Plan 67-01 added (allow-list extended or inverted to a deny-list)
  2. `go test ./deploy/grafana/...` passes — `TestDashboard_RegionVariableUsed` asserts against the post-260426-lod `cloud_region` label (or the `$region` template variable is reworked) and the assertion drives a real panel query rather than dead matching
  3. `golangci-lint run ./internal/visbaseline/...` returns `0 issues` — the 1 `exhaustive`, 3 `gosec G304`, and 1 `nolintlint` findings are either properly resolved (e.g. `filepath.Clean` + safe-root validation, `shapeUnknown` case added) or carry explicit per-line `//nolint:<linter> // reason` directives with sound justifications
  4. The full repo `go generate ./...` drift gate and the lint job stay green on PR + main
**Plans** (3 plans, all Wave 1, can run in parallel):
- [x] 74-01-PLAN.md — TEST-01: TestGenerateIndexes derived from schema/peeringdb.json (per D-01)
- [x] 74-02-PLAN.md — TEST-02: drop $region template var + flip TestDashboard_RegionVariableUsed to TestDashboard_NoOrphanTemplateVars (per D-02)
- [x] 74-03-PLAN.md — TEST-03: filepath.Clean + canonical nolint reason on visbaseline G304 sites (per D-03)

### Phase 75: Code-side Observability Fixes
**Goal**: The three observability defects rooted in app-layer code (gauge cold-start, counter pre-warm, route-tag middleware) emit correct telemetry within 30s of process startup and cover all real routes.
**Depends on**: Nothing (independent of dashboard / audit phases — those phases consume the metric outputs this phase produces).
**Requirements**: OBS-01, OBS-02, OBS-04
**Success Criteria** (what must be TRUE):
  1. Within 30s of process startup, `pdbplus_data_type_count` reports correct row counts for all 13 PeeringDB types — the "Total Objects", "Objects by Type", and "Object Counts Over Time" Grafana panels never render false zeros during the ~15min pre-first-sync window
  2. After a fresh deploy on a healthy fleet, the zero-rate panels (Fallback Events, Role Transitions, Fetch Errors, Upsert Errors, Deletes per Type) render `0` instead of `No data` — every `(type, status)` tuple has been pre-warmed via `.Add(ctx, 0, baseline_attrs)` at startup
  3. During normal traffic, `count by(http_route)(http_server_request_duration_seconds_count)` returns ≥5 distinct route patterns covering at least `/api/*`, `/rest/v1/*`, `/peeringdb.v1.*`, `/graphql`, and `/ui/*` — not just `GET /healthz`
  4. The "Request Rate by Route" dashboard panel shows multi-route breakdown after merge; the root cause for the prior single-route series (empty `r.Pattern` for non-mux routes / middleware ordering) is documented in the phase summary
**Plans** (3 plans, sequential due to shared cmd/peeringdb-plus/main.go edits — Wave 1 / Wave 2 / Wave 3):
- [x] 75-01-PLAN.md — OBS-01: synchronous one-shot Count(ctx) per entity at startup; new internal/sync/initialcounts.go helper + main.go wire-up before InitObjectCountGauges (per D-01)
- [x] 75-02-PLAN.md — OBS-02: pre-warm 4 per-type counters × 13 types + RoleTransitions × 2 directions = 54 baseline series; new internal/otel/prewarm.go + main.go call after InitMetrics (per D-02)
- [x] 75-03-PLAN.md — OBS-04: investigate root cause of http.route only populating /healthz, then ship minimal fix in routeTagMiddleware or buildMiddlewareChain; OBS-04-INVESTIGATION.md + new route_tag_e2e_test.go (per D-03)
**UI hint**: yes

### Phase 76: Dashboard Hardening
**Goal**: The Grafana Cloud `pdbplus-overview` dashboard is collision-safe against shared Prometheus tenants (every `go_*` panel filters by `service_name`) and the v1.17.0+ canonicalised metric names flow correctly to the live binary.
**Depends on**: Phase 75 (consumes the OBS-01/02 metric flows for visual confirmation; soft dependency — JSON edits can land in parallel as long as final visual sign-off waits for Phase 75).
**Requirements**: OBS-03, OBS-05
**Success Criteria** (what must be TRUE):
  1. Every `go_*` PromQL query in `deploy/grafana/dashboards/pdbplus-overview.json` carries a `service_name="peeringdb-plus"` filter; a query against a shared Prom tenant emitting `go_*` from another application shows clean panels without double-counting
  2. `count(pdbplus_response_heap_delta_bytes_bucket{service_version=~"v1\.1[78]\..*"})` returns non-zero during normal pdbcompat list traffic, confirming the post-bytes-canonicalisation series flows on the live binary *(regex corrected 2026-04-27 — original `v1.17.0|v1.18.*` failed against prod's git-describe label format `v1.17.0-64-g565b762` because Prom anchors `=~` matches)*
  3. ~~The dashboard panel description for the response-heap-delta series captures the legacy `pdbplus_response_heap_delta_kib_KiB_bucket` series as expiring retention data, so future operators understand the duplicate metric name~~ *(OVERRIDDEN by CONTEXT.md D-02 "confirm only — no documentation in panel description, no Prom drop rule"; user explicitly chose to let retention expire the legacy series naturally)*
  4. The `deploy/grafana/dashboard_test.go` invariants (no forbidden content, panel structure) stay green after the JSON edits
**Plans** (1 plan, Wave 1 — task ordering inside the plan: Task 1 RED test → Task 2 GREEN JSON edits → Task 3 OBS-05 manual confirm):
- [x] 76-01-PLAN.md — OBS-03 service_name filter sweep on 5 go_* panels + $service template var + new TestDashboard_GoMetricsFilterByService invariant; OBS-05 live Prom confirm-only per D-02
**UI hint**: yes

### Phase 77: Telemetry Audit & Cleanup
**Goal**: Loki log volume is qualitatively cleaner and Tempo trace size stays well under the 7.5 MB per-trace cap; sampling and batching parameters are confirmed appropriate for current cardinality. No architectural pipeline changes.
**Depends on**: Phase 75 (the `http.route` label fix changes log/trace shape; auditing before that lands would be wasted work).
**Requirements**: OBS-06, OBS-07
**Success Criteria** (what must be TRUE):
  1. A log-level audit document at `.planning/phases/<N>-*/AUDIT.md` lists every reclassified log line (INFO→DEBUG, WARN→INFO, DEBUG→removed) with before/after slog level and rationale citing whether it fires under non-error conditions
  2. Production Loki log volume is measurably down (or qualitatively cleaner) post-merge — operator can grep for "WARN" in a 24h window without per-redaction or per-step-progress noise
  3. Empirical Tempo inspection of normal-traffic traces shows max per-trace size <2 MB; the FK-orphan WARN-spam regression mode that breached 7.5 MB is confirmed not present
  4. `OTEL_BSP_SCHEDULE_DELAY=5s` and `OTEL_BSP_MAX_EXPORT_BATCH_SIZE=512` are confirmed appropriate for current cardinality (or adjusted with documented justification); sampling ratio remains at 1.0 unless cardinality requires reduction
**Plans** (2 plans across 2 waves — 77-02 depends on 77-01 because both edit AUDIT.md):
- [ ] 77-01-PLAN.md — Wave 1 — OBS-06: Loki log-level audit (operator-driven Grafana MCP sample) + slog level changes inline (per-step sync INFO→DEBUG, /readyz pre-first-sync WARN→DEBUG, security signals KEEP); produces AUDIT.md
- [ ] 77-02-PLAN.md — Wave 2 — OBS-07: Tempo audit appendix to AUDIT.md + new internal/otel/sampler.go perRouteSampler wired through sdktrace.ParentBased; docs/ARCHITECTURE.md gains Sampling Matrix subsection

### Phase 78: UAT Closeout
**Goal**: All three v1.13 / v1.5 outstanding human-verification items are signed off against live production and recorded; no stale deferred-items pointers remain in the planning tree.
**Depends on**: Nothing (UAT can run in parallel with all preceding phases — these verify previously-shipped work, not anything in this milestone).
**Requirements**: UAT-01, UAT-02, UAT-03
**Success Criteria** (what must be TRUE):
  1. With `PDBPLUS_CSP_ENFORCE=true` set on the live `peeringdb-plus.fly.dev` deployment, Chrome DevTools shows zero CSP violation reports against `/ui/`, `/ui/asn/13335`, and `/ui/compare`; a `UAT-RESULTS.md` artefact under the phase directory records the human-runtime check (including the Tailwind v4 JIT runtime confirmation that was the original Phase 52 concern)
  2. `curl -I https://peeringdb-plus.fly.dev/{ui,api/net,rest/v1/networks}` confirms `Strict-Transport-Security`, `X-Frame-Options`, and `X-Content-Type-Options` headers per the v1.13 Phase 53 scope; the 2 MB body cap blocks oversize POSTs on REST/pdbcompat surfaces while gRPC/ConnectRPC streams pass through; a slowloris TCP slow-write smoke test does not exhaust the connection pool
  3. `.planning/milestones/v1.5-phases/20-deferred-human-verification/` is relocated to the consumed-style archive (matching the seed convention); STATE.md "Outstanding Human Verification" section is updated to reflect closure of all three UAT items; the v1.13 phase 52 + 53 deferred-items entries are flipped to resolved
**Plans** (1 plan, Wave 1 — task ordering inside the plan: Task 1 RED test → Task 2 GREEN JSON edits → Task 3 OBS-05 manual confirm):
- [ ] 76-01-PLAN.md — OBS-03 service_name filter sweep on 5 go_* panels + $service template var + new TestDashboard_GoMetricsFilterByService invariant; OBS-05 live Prom confirm-only per D-02
**UI hint**: yes

## Backlog

_No parked 999.x phases._

## Phase Numbering Note

v1.16 closed at Phase 72. v1.17.0 was used as a release tag for quick task 260426-pms (SEED-001 incremental-sync default flip), not a milestone — no phase numbers consumed. v1.18.0 therefore continues at Phase 73 and runs through Phase 78.
