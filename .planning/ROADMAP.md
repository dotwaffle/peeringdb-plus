# Roadmap: PeeringDB Plus

## Milestones

- ✅ **v1.0 PeeringDB Plus** — Phases 1-3 (shipped 2026-03-22)
- ✅ **v1.1 REST API & Observability** — Phases 4-6 (shipped 2026-03-23)
- ✅ **v1.2 Quality, Incremental Sync & CI** — Phases 7-10 (shipped 2026-03-24)
- ✅ **v1.3 PeeringDB API Key Support** — Phases 11-12 (shipped 2026-03-24)
- ✅ **v1.4 Web UI** — Phases 13-17 (shipped 2026-03-24)
- 🚧 **v1.5 Tech Debt & Observability** — Phases 18-20 (in progress)

## Phases

<details>
<summary>✅ v1.0 PeeringDB Plus (Phases 1-3) — SHIPPED 2026-03-22</summary>

- [x] Phase 1: Data Foundation (7/7 plans) — completed 2026-03-22
- [x] Phase 2: GraphQL API (4/4 plans) — completed 2026-03-22
- [x] Phase 3: Production Readiness (3/3 plans) — completed 2026-03-22

See: `.planning/milestones/v1.0-ROADMAP.md` for full details.

</details>

<details>
<summary>✅ v1.1 REST API & Observability (Phases 4-6) — SHIPPED 2026-03-23</summary>

- [x] Phase 4: Observability Foundations (2/2 plans) — completed 2026-03-22
- [x] Phase 5: entrest REST API (3/3 plans) — completed 2026-03-22
- [x] Phase 6: PeeringDB Compatibility Layer (3/3 plans) — completed 2026-03-22

See: `.planning/milestones/v1.1-ROADMAP.md` for full details.

</details>

<details>
<summary>✅ v1.2 Quality, Incremental Sync & CI (Phases 7-10) — SHIPPED 2026-03-24</summary>

- [x] Phase 7: Lint & Code Quality (2/2 plans) — completed 2026-03-23
- [x] Phase 8: Incremental Sync (3/3 plans) — completed 2026-03-23
- [x] Phase 9: Golden File Tests & Conformance (2/2 plans) — completed 2026-03-23
- [x] Phase 10: CI Pipeline & Public Access (2/2 plans) — completed 2026-03-24

See: `.planning/milestones/v1.2-ROADMAP.md` for full details.

</details>

<details>
<summary>✅ v1.3 PeeringDB API Key Support (Phases 11-12) — SHIPPED 2026-03-24</summary>

- [x] Phase 11: API Key & Rate Limiting (2/2 plans) — completed 2026-03-24
- [x] Phase 12: Conformance Tooling Integration (1/1 plan) — completed 2026-03-24

See: `.planning/milestones/v1.3-ROADMAP.md` for full details.

</details>

<details>
<summary>✅ v1.4 Web UI (Phases 13-17) — SHIPPED 2026-03-24</summary>

- [x] Phase 13: Foundation (2/2 plans) — completed 2026-03-24
- [x] Phase 14: Live Search (2/2 plans) — completed 2026-03-24
- [x] Phase 15: Record Detail Pages (2/2 plans) — completed 2026-03-24
- [x] Phase 16: ASN Comparison (2/2 plans) — completed 2026-03-24
- [x] Phase 17: Polish & Accessibility (3/3 plans) — completed 2026-03-24

See: `.planning/milestones/v1.4-ROADMAP.md` for full details.

</details>

### v1.5 Tech Debt & Observability (In Progress)

- [x] **Phase 18: Tech Debt & Data Integrity** - Correct stale planning docs, verify meta.generated field behavior, document findings (completed 2026-03-24)
- [ ] **Phase 19: Prometheus Metrics & Grafana Dashboard** - Register per-type object count gauges, create comprehensive Grafana dashboard JSON
- [ ] **Phase 20: Deferred Human Verification** - Verify all 26 deferred items against live Fly.io deployment

## Phase Details

### Phase 18: Tech Debt & Data Integrity
**Goal**: Planning documentation accurately reflects resolved tech debt and the sync pipeline's meta.generated cursor behavior is empirically verified and documented
**Depends on**: Nothing (first phase of v1.5)
**Requirements**: DEBT-01, DEBT-02, DATA-01, DATA-02, DATA-03
**Success Criteria** (what must be TRUE):
  1. Planning documentation accurately reflects that WorkerConfig.IsPrimary was converted from dead bool to live func() bool by quick task 260324-lc5 (not removed)
  2. Planning documentation accurately reflects which dead code items were already removed vs. newly removed in this phase
  3. meta.generated field behavior is documented with actual observed response structures for depth=0 full fetch, paginated incremental, and empty result set patterns
  4. Sync pipeline handles absent meta.generated gracefully (zero-time triggers started_at - 5min fallback) without data loss or sync failure
**Plans**: 2 plans

Plans:
- [ ] 18-01-PLAN.md -- Correct stale planning docs (PROJECT.md, Phase 7 summary)
- [ ] 18-02-PLAN.md -- Live meta.generated test and documentation

### Phase 19: Prometheus Metrics & Grafana Dashboard
**Goal**: All existing OTel metrics are exported via OTLP to Grafana Cloud and visualized in a portable Grafana dashboard covering sync health, HTTP traffic, per-type sync detail, runtime metrics, and business metrics
**Depends on**: Phase 18
**Requirements**: OBS-01, OBS-02, OBS-03, OBS-04, OBS-05, OBS-06, OBS-07, OBS-08, OBS-09, OBS-10
**Success Criteria** (what must be TRUE):
  1. Fly.io Prometheus scraper collects metrics from the application's /metrics endpoint (port 9091) without errors
  2. Grafana dashboard displays sync health (freshness gauge with green/yellow/red thresholds, sync duration, success/failure rate) using live data
  3. Grafana dashboard displays HTTP RED metrics (request rate by route, error rate, latency percentiles) and per-type sync detail (duration, object counts, errors by PeeringDB type)
  4. Dashboard JSON is committed to deploy/grafana/ with datasource template variables (no hardcoded UIDs) and imports cleanly into a fresh Grafana instance
  5. Each dashboard row contains documentation text panels explaining the metrics and troubleshooting guidance
**Plans**: 2 plans

Plans:
- [ ] 19-01-PLAN.md -- Register per-type object count gauges and wire into application startup
- [ ] 19-02-PLAN.md -- Author Grafana dashboard JSON and provisioning YAML

### Phase 20: Deferred Human Verification
**Goal**: All 26 deferred human verification items from v1.2, v1.3, and v1.4 are verified as working correctly against the live Fly.io deployment
**Depends on**: Phase 19
**Requirements**: VFY-01, VFY-02, VFY-03, VFY-04, VFY-05, VFY-06, VFY-07, VFY-08, VFY-09
**Success Criteria** (what must be TRUE):
  1. CI workflow executes on push with all 4 jobs passing, and coverage comments post on PRs with deduplication on subsequent pushes
  2. pdbcompat-check CLI and live integration test work correctly with a real PeeringDB API key, and invalid keys produce WARN log without crash
  3. Web UI foundation, live search, detail pages, and ASN comparison all function correctly in a browser against the live deployment
  4. Dark mode toggle, keyboard navigation, CSS animations, loading indicators, error pages, and About page freshness display all work as designed
**Plans**: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 18 → 19 → 20

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1. Data Foundation | v1.0 | 7/7 | Complete | 2026-03-22 |
| 2. GraphQL API | v1.0 | 4/4 | Complete | 2026-03-22 |
| 3. Production Readiness | v1.0 | 3/3 | Complete | 2026-03-22 |
| 4. Observability Foundations | v1.1 | 2/2 | Complete | 2026-03-22 |
| 5. entrest REST API | v1.1 | 3/3 | Complete | 2026-03-22 |
| 6. PeeringDB Compatibility Layer | v1.1 | 3/3 | Complete | 2026-03-22 |
| 7. Lint & Code Quality | v1.2 | 2/2 | Complete | 2026-03-23 |
| 8. Incremental Sync | v1.2 | 3/3 | Complete | 2026-03-23 |
| 9. Golden File Tests & Conformance | v1.2 | 2/2 | Complete | 2026-03-23 |
| 10. CI Pipeline & Public Access | v1.2 | 2/2 | Complete | 2026-03-24 |
| 11. API Key & Rate Limiting | v1.3 | 2/2 | Complete | 2026-03-24 |
| 12. Conformance Tooling Integration | v1.3 | 1/1 | Complete | 2026-03-24 |
| 13. Foundation | v1.4 | 2/2 | Complete | 2026-03-24 |
| 14. Live Search | v1.4 | 2/2 | Complete | 2026-03-24 |
| 15. Record Detail Pages | v1.4 | 2/2 | Complete | 2026-03-24 |
| 16. ASN Comparison | v1.4 | 2/2 | Complete | 2026-03-24 |
| 17. Polish & Accessibility | v1.4 | 3/3 | Complete | 2026-03-24 |
| 18. Tech Debt & Data Integrity | v1.5 | 0/2 | Complete    | 2026-03-24 |
| 19. Prometheus Metrics & Grafana Dashboard | v1.5 | 0/2 | Not started | - |
| 20. Deferred Human Verification | v1.5 | 0/? | Not started | - |
