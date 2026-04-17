# Roadmap: PeeringDB Plus

## Milestones

- [x] **v1.0 MVP** - Phases 1-3 (shipped 2026-03-22)
- [x] **v1.1 REST API & Observability** - Phases 4-6 (shipped 2026-03-23)
- [x] **v1.2 Quality, Incremental Sync & CI** - Phases 7-10 (shipped 2026-03-24)
- [x] **v1.3 PeeringDB API Key Support** - Phases 11-12 (shipped 2026-03-24)
- [x] **v1.4 Web UI** - Phases 13-17 (shipped 2026-03-24)
- [x] **v1.5 Tech Debt & Observability** - Phases 18-20 (shipped 2026-03-24)
- [x] **v1.6 ConnectRPC / gRPC API** - Phases 21-24 (shipped 2026-03-25)
- [x] **v1.7 Streaming RPCs & UI Polish** - Phases 25-27 (shipped 2026-03-25)
- [x] **v1.8 Terminal CLI Interface** - Phases 28-31 (shipped 2026-03-26)
- [x] **v1.9 Hardening & Polish** - Phases 32-36 (shipped 2026-03-26)
- [x] **v1.10 Code Coverage & Test Quality** - Phases 37-42 (shipped 2026-03-26)
- [x] **v1.11 Web UI Density & Interactivity** - Phases 43-46 (shipped 2026-03-26)
- [x] **v1.12 Hardening & Tech Debt** - Phases 47-50 (shipped 2026-04-02)
- [x] **v1.13 Security & Sync Hardening** - Phases 51-56 (shipped 2026-04-11)
- [ ] **v1.14 Authenticated Sync & Visibility Layer** - Phases 57-62 (in progress, started 2026-04-16)

## Phase Numbering

- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

## History

All shipped milestones are summarised in [MILESTONES.md](./MILESTONES.md). Per-milestone ROADMAP snapshots live at `.planning/milestones/v{X.Y}-ROADMAP.md`, and phase artifacts (plans, summaries, verification reports) at `.planning/milestones/v{X.Y}-phases/`.

## Active Work

### v1.14 Authenticated Sync & Visibility Layer

**Goal:** Make it safe to run the sync with a PeeringDB API key by honouring upstream visibility on all read paths — defaulting anonymous reads to `Public`-only.

**Phases:**

- [x] **Phase 57: Visibility baseline capture** - Empirically capture unauth + auth PeeringDB responses for all 13 types and emit a structural diff (completed 2026-04-16)
- [x] **Phase 58: Visibility schema alignment** - Confirm `poc.visible` and add fields for any other auth-gated data identified in Phase 57
- [x] **Phase 59: ent Privacy policy + sync bypass** - Wire `entgo.io/ent/privacy` query policy that filters non-`Public` rows from anonymous reads, with sync-write bypass and `PDBPLUS_PUBLIC_TIER` override (completed 2026-04-16) (completed 2026-04-17)
- [x] **Phase 60: Surface integration + tests** - Verify privacy policy fires through all 5 read surfaces and pdbcompat anonymous shape matches upstream (completed 2026-04-17)
- [ ] **Phase 61: Operator-facing observability** - Startup log classification, `/about` rendering, `pdbplus.privacy.tier` OTel attribute on read spans
- [ ] **Phase 62: API key default & docs** - Set Fly.io secret, document authenticated deployment as recommended path (manual verification + docs)

## Phase Details

### Phase 57: Visibility baseline capture

**Goal**: Produce a committed, reviewable empirical baseline showing exactly which fields/rows differ between unauthenticated and authenticated PeeringDB API responses across all 13 types — without which the privacy filter cannot be scoped correctly.

**Depends on**: Nothing (first phase of v1.14)

**Requirements**: VIS-01, VIS-02

**Success Criteria** (what must be TRUE):
  1. `cmd/pdbcompat-check/` (or equivalent tooling) has a `--capture` mode that walks all 13 PeeringDB types in both unauth and auth modes, with explicit per-request sleeps honouring the documented rate ceilings (≤ 20 anon/min, ≤ 40 auth/min) and resumability across interruption.
  2. `testdata/visibility-baseline/beta/` contains committed JSON fixtures for all 13 types in both unauth and auth modes, captured against `beta.peeringdb.com`.
  3. `testdata/visibility-baseline/prod/` contains a confirmation pass against `www.peeringdb.com` for poc, org, and net (high-signal types).
  4. A structural diff report (committed alongside the fixtures) lists every field and row that differs between unauth and auth responses, organised as a per-type table reviewable in code review.

**Plans:** 4/4 plans complete
- [x] 57-01-PLAN.md — PII allow-list + pure-function redactor + .gitignore guards (Wave 1)
- [x] 57-02-PLAN.md — Checkpoint + capture loop + FetchRawPage + -capture CLI flag (Wave 2)
- [x] 57-03-PLAN.md — Structural differ + Markdown/JSON emitters + committed-fixture PII guard test (Wave 2)
- [x] 57-04-PLAN.md — Operator-run beta+prod capture + redact + diff commit (Wave 3, autonomous: false)

**Wall-clock note**: Walking all 13 types under both auth modes against beta with rate-limit-honouring sleeps will take ≥ 1 hour wall-clock. This is intrinsic to the rate ceiling, not a planning estimate to be compressed.

### Phase 58: Visibility schema alignment

**Goal**: Bring the ent schemas into agreement with the empirical visibility baseline — every auth-gated entity discovered in Phase 57 has a visibility-bearing field that the privacy policy can key off.

**Depends on**: Phase 57

**Requirements**: VIS-03

**Success Criteria** (what must be TRUE):
  1. `poc.visible` is confirmed as the existing visibility-bearing field for POCs and surveyed against the Phase 57 diff for completeness.
  2. Any additional auth-gated fields/entities surfaced by VIS-02 (candidates: `social_media` on org/net, billing/legal address on org, `policy_general` on net) have ent schema fields that the privacy policy can use.
  3. `go generate ./...` regenerates ent cleanly with the new field(s); committed `ent/` files are byte-identical with what the generator produces.
  4. Findings are documented in PROJECT.md Key Decisions so future maintainers can trace why each field exists.

**Plans:** 1/1 plans complete

- [x] 58-01-PLAN.md — Schema-alignment regression test + PROJECT.md/CLAUDE.md documentation + go generate drift check (Wave 1, autonomous)

### Phase 59: ent Privacy policy + sync bypass

**Goal**: Install the read-path privacy floor — anonymous queries cannot return rows whose upstream visibility is `Users`-only, while the sync worker retains full read/write access.

**Depends on**: Phase 58

**Requirements**: VIS-04, VIS-05, SYNC-03

**Success Criteria** (what must be TRUE):
  1. `entgo.io/ent/privacy` is enabled in `ent/entc.go` and a query policy is defined for POC (and any other entity from Phase 58) that filters `visible != "Public"` when the request context lacks a trusted-tier marker.
  2. The sync worker (`internal/sync/worker.go`, `internal/sync/upsert.go`) wraps its ent client calls in `privacy.DecisionContext(ctx, privacy.Allow)`; tests assert the bypass is active for sync-context goroutines and absent for HTTP-handler goroutines.
  3. `PDBPLUS_PUBLIC_TIER` env var is parsed in `internal/config/config.go` (default `public`, accepts `users`); when set to `users`, anonymous request contexts are stamped with the Users-tier marker so the policy admits Users-visibility rows.
  4. Unit tests cover both `PDBPLUS_PUBLIC_TIER` values and the sync-context bypass behaviour.

**Plans:** 6/6 plans complete
- [x] 59-01-PLAN.md — internal/privctx package (Tier type, WithTier, TierFrom) (Wave 1)
- [x] 59-02-PLAN.md — PDBPLUS_PUBLIC_TIER env parser + Config field + validator (Wave 2)
- [x] 59-03-PLAN.md — PrivacyTier HTTP middleware + chain wiring + chain-order regression test (Wave 3)
- [x] 59-04-PLAN.md — FeaturePrivacy codegen + POC Policy() + VIS-04 behaviour tests (Wave 4)
- [x] 59-05-PLAN.md — sync-worker bypass + single-call-site audit test (Wave 5)
- [x] 59-06-PLAN.md — D-15 end-to-end test: all 5 surfaces × both tiers (Wave 6)

### Phase 60: Surface integration + tests

**Goal**: Prove the privacy policy fires correctly through every read surface and that the pdbcompat anonymous shape matches upstream's anonymous shape.

**Depends on**: Phase 59

**Requirements**: VIS-06, VIS-07, SYNC-02

**Success Criteria** (what must be TRUE):
  1. Per-surface integration tests for `/ui/`, `/graphql`, `/rest/v1/`, `/api/`, and `/peeringdb.v1.*` each issue an anonymous request and assert no row in the response has `visible="Users"` (or any other auth-gated marker from Phase 58).
  2. Anonymous `/api/poc` and embedded `poc_set` shapes match upstream anonymous shape — Users-tier rows are absent from the response, not present-with-redacted-fields. Verified by replaying VIS-01 fixtures against the local endpoint with empty diff.
  3. Integration test for unauthenticated sync mode confirms: with no `PDBPLUS_PEERINGDB_API_KEY` set, the worker syncs only the upstream anonymous payload, no Users-tier rows ever land in the DB, and the privacy filter is correctly a no-op.
  4. `internal/conformance/` is updated so the live conformance comparison contrasts our anonymous shape against upstream's anonymous shape.

**Plans:** 5/5 plans complete
- [x] 60-01-PLAN.md — Extend seed.Full with mixed-visibility POCs + lock-in regression tests (Wave 1, autonomous, VIS-06)
- [x] 60-02-PLAN.md — Per-surface anonymous-leak list-count tests across all 5 surfaces (Wave 2, autonomous, VIS-06)
- [x] 60-03-PLAN.md — pdbcompat anon parity via fixture replay (13 types, empty structural diff) (Wave 3, autonomous, VIS-07)
- [x] 60-04-PLAN.md — Conformance live test flipped to anon-vs-anon only (Wave 2, autonomous, VIS-07)
- [x] 60-05-PLAN.md — No-key sync integration test with fake upstream + surface read (Wave 2, autonomous, SYNC-02)

### Phase 61: Operator-facing observability

**Goal**: Make the sync mode and effective privacy tier visible to operators at startup, on the `/about` surface, and in OTel traces — no silent escalation.

**Depends on**: Phase 59

**Requirements**: SYNC-04, OBS-01, OBS-02, OBS-03

**Success Criteria** (what must be TRUE):
  1. Startup logs a single classification line based on `PDBPLUS_PEERINGDB_API_KEY` presence: "anonymous, public-only" when absent, "authenticated, full" when present.
  2. Startup additionally logs a WARN line whenever `PDBPLUS_PUBLIC_TIER=users` is in effect, naming the override so the elevated default is never silent.
  3. Both `/about` (HTML) and `/ui/about` (terminal) render the current sync mode and effective privacy tier.
  4. Read-path spans carry an OTel attribute `pdbplus.privacy.tier` with value `public` or `users`, usable as a Grafana dashboard filter.

**Plans**: TBD

**UI hint**: yes

**Parallelism note**: Phases 60 and 61 can run in parallel after Phase 59 lands. Phase 60 lives in test files for each surface (`internal/web/`, `graph/`, REST/RPC test scaffolding, `internal/pdbcompat/`, `internal/grpcserver/`, `internal/conformance/`). Phase 61 touches `cmd/peeringdb-plus/main.go` startup logging, `internal/web/handler.go` (about renderer), terminal renderers, and OTel span attribute wiring. No file overlap.

### Phase 62: API key default & docs

**Goal**: Switch the production deployment to authenticated sync and document it as the recommended path. Intentionally a manual-verification + docs phase with minimal code.

**Depends on**: Phase 60, Phase 61

**Requirements**: SYNC-01, DOC-01, DOC-02, DOC-03

**Success Criteria** (what must be TRUE):
  1. Production Fly.io app `peeringdb-plus` has `PDBPLUS_PEERINGDB_API_KEY` set as a fly secret; presence verified via `fly secrets list`.
  2. `docs/CONFIGURATION.md` documents `PDBPLUS_PEERINGDB_API_KEY`, `PDBPLUS_PUBLIC_TIER`, and the privacy guarantees they provide.
  3. `docs/DEPLOYMENT.md` documents the recommended authenticated deployment, including the `fly secrets set` command and the operational implication (Users-tier rows present in DB, filtered on anonymous reads).
  4. `docs/ARCHITECTURE.md` describes the ent Privacy layer and how each of the 5 surfaces honours it, including the sync-write bypass.

**Plans**: TBD

**Scope note**: This phase is intentionally minimal-code. Output is fly secret + three doc edits + manual verification against the deployed instance. Resist over-planning.

## Progress

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 57. Visibility baseline capture | 4/4 | Complete    | 2026-04-16 |
| 58. Visibility schema alignment | 1/1 | Complete    | 2026-04-17 |
| 59. ent Privacy policy + sync bypass | 6/6 | Complete    | 2026-04-17 |
| 60. Surface integration + tests | 5/5 | Complete    | 2026-04-17 |
| 61. Operator-facing observability | 0/0 | Not started | - |
| 62. API key default & docs | 0/0 | Not started | - |
