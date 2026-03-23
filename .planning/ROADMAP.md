# Roadmap: PeeringDB Plus

## Milestones

- ✅ **v1.0 PeeringDB Plus** — Phases 1-3 (shipped 2026-03-22)
- ✅ **v1.1 REST API & Observability** — Phases 4-6 (shipped 2026-03-23)
- 🚧 **v1.2 Quality, Incremental Sync & CI** — Phases 7-10 (in progress)

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

### 🚧 v1.2 Quality, Incremental Sync & CI (In Progress)

**Milestone Goal:** Harden the codebase with linting enforcement, add configurable incremental sync, golden file tests for the PeeringDB compat layer, conformance testing against live PeeringDB, and CI via GitHub Actions.

- [ ] **Phase 7: Lint & Code Quality** — Configure golangci-lint v2 and fix all existing violations
- [ ] **Phase 8: Incremental Sync** — Configurable sync mode with per-type delta fetches via PeeringDB ?since= parameter
- [ ] **Phase 9: Golden File Tests & Conformance** — Golden file test infrastructure for PeeringDB compat layer and conformance tooling against beta.peeringdb.com
- [ ] **Phase 10: CI Pipeline & Public Access** — GitHub Actions enforcement and public access documentation

## Phase Details

### Phase 7: Lint & Code Quality
**Goal**: The codebase passes all linting and vetting cleanly, with generated code correctly excluded
**Depends on**: Phase 6
**Requirements**: LINT-01, LINT-02, LINT-03
**Success Criteria** (what must be TRUE):
  1. Running `golangci-lint run` passes with zero violations on all hand-written code
  2. Running `go vet ./...` passes clean across the entire codebase
  3. Generated code (ent, gqlgen) is excluded from linting without suppressing hand-written code in `ent/schema/`
**Plans**: 2 plans
Plans:
- [x] 07-01-PLAN.md — Configure golangci-lint v2 and remove dead code
- [ ] 07-02-PLAN.md — Fix all lint violations and verify clean pass

### Phase 8: Incremental Sync
**Goal**: Sync mode is configurable between full re-fetch and incremental delta fetch, with per-type timestamp tracking and automatic fallback on failure
**Depends on**: Phase 7
**Requirements**: SYNC-01, SYNC-02, SYNC-03, SYNC-04, SYNC-05
**Success Criteria** (what must be TRUE):
  1. Setting `PDBPLUS_SYNC_MODE=incremental` causes the sync worker to fetch only objects modified since the last successful sync per type
  2. Setting `PDBPLUS_SYNC_MODE=full` (default) preserves existing full re-fetch behavior with no regressions
  3. Per-type last-sync timestamps are tracked in the extended sync_status table
  4. When an incremental sync fails for a specific type, it immediately falls back to a full sync for that type
  5. First sync always performs a full fetch regardless of mode (no ?since= on empty database)
**Plans**: TBD

### Phase 9: Golden File Tests & Conformance
**Goal**: PeeringDB compat layer responses are verified against committed reference files, and a conformance tool can compare output against the real PeeringDB API
**Depends on**: Phase 8
**Requirements**: GOLD-01, GOLD-02, GOLD-03, GOLD-04, CONF-01, CONF-02
**Success Criteria** (what must be TRUE):
  1. Running `go test ./internal/pdbcompat/...` compares all compat layer responses against committed golden files and fails on any diff
  2. Running `go test ./internal/pdbcompat/... -update` regenerates all golden files from current output
  3. Golden files exist for list, detail, and depth-expanded responses across all 13 PeeringDB types
  4. A CLI tool can fetch from beta.peeringdb.com and report structural differences against local compat layer output
  5. An integration test gated by `-peeringdb-live` validates conformance (skipped in normal test runs)
**Plans**: TBD

### Phase 10: CI Pipeline & Public Access
**Goal**: Every PR is automatically validated by GitHub Actions, and the public access model is verified and documented
**Depends on**: Phase 9
**Requirements**: CI-01, CI-02, CI-03, CI-04, PUB-01, PUB-02
**Success Criteria** (what must be TRUE):
  1. A GitHub Actions workflow runs lint, test (with `-race`), and build on every PR with parallel jobs
  2. `go test -race ./...` runs with `CGO_ENABLED=1` in CI and passes
  3. govulncheck runs as part of the CI pipeline
  4. Test coverage percentage is reported on each CI run
  5. All API endpoints (GraphQL, REST, PeeringDB compat) are accessible without authentication and this is documented
**Plans**: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 7 -> 8 -> 9 -> 10

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1. Data Foundation | v1.0 | 7/7 | Complete | 2026-03-22 |
| 2. GraphQL API | v1.0 | 4/4 | Complete | 2026-03-22 |
| 3. Production Readiness | v1.0 | 3/3 | Complete | 2026-03-22 |
| 4. Observability Foundations | v1.1 | 2/2 | Complete | 2026-03-22 |
| 5. entrest REST API | v1.1 | 3/3 | Complete | 2026-03-22 |
| 6. PeeringDB Compatibility Layer | v1.1 | 3/3 | Complete | 2026-03-22 |
| 7. Lint & Code Quality | v1.2 | 0/2 | Planning | - |
| 8. Incremental Sync | v1.2 | 0/0 | Not started | - |
| 9. Golden File Tests & Conformance | v1.2 | 0/0 | Not started | - |
| 10. CI Pipeline & Public Access | v1.2 | 0/0 | Not started | - |
