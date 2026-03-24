# Roadmap: PeeringDB Plus

## Milestones

- ✅ **v1.0 PeeringDB Plus** — Phases 1-3 (shipped 2026-03-22)
- ✅ **v1.1 REST API & Observability** — Phases 4-6 (shipped 2026-03-23)
- ✅ **v1.2 Quality, Incremental Sync & CI** — Phases 7-10 (shipped 2026-03-24)
- 🚧 **v1.3 PeeringDB API Key Support** — Phases 11-12 (in progress)

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

### 🚧 v1.3 PeeringDB API Key Support (In Progress)

**Milestone Goal:** Support optional PeeringDB API key authentication for sync and conformance tooling, with higher rate limits when authenticated.

- [x] **Phase 11: API Key & Rate Limiting** — Wire API key into HTTP client with rate limit adjustment and startup validation (completed 2026-03-24)
- [x] **Phase 12: Conformance Tooling Integration** — Wire API key into pdbcompat-check CLI and live integration tests (completed 2026-03-24)

## Phase Details

### Phase 11: API Key & Rate Limiting
**Goal**: Authenticated PeeringDB API access with higher rate limits when an API key is configured, with graceful degradation to current behavior when unconfigured
**Depends on**: Phase 10
**Requirements**: KEY-01, KEY-02, RATE-01, RATE-02, VALIDATE-01
**Success Criteria** (what must be TRUE):
  1. When `PDBPLUS_PEERINGDB_API_KEY` is set, every PeeringDB API request includes the `Authorization: Api-Key <key>` header
  2. When no API key is set, the application starts and syncs identically to v1.2 behavior (no auth header, 20 req/min rate limit)
  3. When an API key is present, the HTTP client rate limiter uses a higher request-per-minute threshold than the unauthenticated 20 req/min
  4. When PeeringDB rejects the API key (401/403), the error is logged clearly with the status code and a message indicating the key may be invalid
**Plans**: 2 plans

Plans:
- [x] 11-01-PLAN.md — Config field + client options, header injection, rate limit adjustment, and auth error handling with tests
- [x] 11-02-PLAN.md — Wire API key from config to client in main.go with SEC-2 compliant startup logging

### Phase 12: Conformance Tooling Integration
**Goal**: Conformance CLI and live integration tests use the API key when available for authenticated PeeringDB access
**Depends on**: Phase 11
**Requirements**: CONFORM-01, CONFORM-02
**Success Criteria** (what must be TRUE):
  1. The `pdbcompat-check` CLI accepts an `--api-key` flag or reads `PDBPLUS_PEERINGDB_API_KEY` from the environment, and uses it for all PeeringDB requests
  2. The `-peeringdb-live` integration test uses the API key when available, enabling higher rate limits during conformance checks
  3. Both tools continue to work without an API key (unauthenticated, lower rate limits)
**Plans**: 1 plan

Plans:
- [x] 12-01-PLAN.md — API key flag/env var in CLI with auth header and tests, plus live test auth header and conditional sleep

## Progress

**Execution Order:**
Phases execute in numeric order: 11 → 12

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
| 11. API Key & Rate Limiting | v1.3 | 2/2 | Complete    | 2026-03-24 |
| 12. Conformance Tooling Integration | v1.3 | 1/1 | Complete    | 2026-03-24 |
