---
phase: 59-ent-privacy-policy-sync-bypass
plan: 06
subsystem: testing

tags: [privacy, e2e, integration, pdbcompat, entrest, connectrpc, graphql, templ]

# Dependency graph
requires:
  - phase: 59-ent-privacy-policy-sync-bypass
    provides: "gen.FeaturePrivacy codegen (Plan 01), privctx package + middleware (Plan 02/03), Poc Policy() (Plan 04), sync worker bypass (Plan 05)"
provides:
  - "D-15 end-to-end correctness contract: visible=Users POC hidden on all 5 surfaces under TierPublic and admitted under TierUsers"
  - "19 sub-tests exercising buildMiddlewareChain + production mux against the real pdbcompat, entrest, ConnectRPC, GraphQL, and /ui/ surfaces"
  - "Cross-surface regression guard: any future handler bypass of the ent Policy (custom resolver, direct Client.Poc call outside the Policy envelope) will flip a deterministic E2E assertion"
affects: [60-verification, 61-observability, oauth-v1.15]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "E2E test exercises buildMiddlewareChain + full mux via httptest.NewServer — not a mock"
    - "Bypass seeding via privacy.DecisionContext(Allow) inside a *_test.go file is audit-exempt (bypass_audit_test.go skips *_test.go)"
    - "Surface-native not-found assertion shape: 404 / CodeNotFound / empty edges — never 403 (D-13/D-14 existence-leak defence)"

key-files:
  created:
    - "cmd/peeringdb-plus/e2e_privacy_test.go — TestE2E_AnonymousCannotSeeUsersPoc + TestE2E_PublicTierUsersAdmitsRow (740 lines)"
  modified: []

key-decisions:
  - "Inline mux wiring inside the test fixture rather than extracting buildProductionMux from main.go — keeps main.go untouched and the test self-contained"
  - "Use e2eAlwaysReady stub for the readiness middleware — test is about privacy, not readiness, and plumbing a real sync worker in would pull in sync_status table setup unrelated to the assertion"
  - "GraphQL lookup via pocs(where:{id:N}) instead of the proposed poc(id:N) — project schema exposes no direct poc/{id} root field; pocs connection still routes through *ent.PocQuery and therefore through the Policy"
  - "/ui/ surface assertion against /ui/fragment/net/{netID}/contacts rather than /ui/poc/{id} — web.Handler has no dedicated POC detail route; contacts fragment exercises the same edge-traversal (network → pocs) code path that Pitfall 5 in RESEARCH.md warns about"
  - "TierUsers sanity-check on /ui/asn/{asn} rendering omitted as a standalone sub-test — the ui_contacts_fragment_present sub-test already covers the full render path"

patterns-established:
  - "E2E cross-surface privacy contract: one seed + one middleware-chain server + N surface sub-tests via t.Run keeps signal-to-noise high and isolates regressions"
  - "Tier parameterisation via chainConfig.DefaultTier rather than env-var mutation — matches production wiring and avoids t.Setenv ordering hazards"
  - "Discarded slog logger (slog.New(slog.NewTextHandler(io.Discard, nil))) keeps E2E test output clean while still exercising Logging middleware code paths"

requirements-completed: [VIS-04, VIS-05, SYNC-03]

# Metrics
duration: ~15min
completed: 2026-04-17
---

# Phase 59-06: D-15 End-to-End Privacy Contract Summary

**Full-stack integration test proving visible="Users" POCs are hidden from anonymous callers across pdbcompat, entrest, ConnectRPC, GraphQL, and /ui/ under TierPublic, and admitted under TierUsers — D-15 correctness contract fulfilled.**

## Performance

- **Duration:** ~15 min
- **Started:** 2026-04-17T02:45 UTC (approx — worktree hard-reset)
- **Completed:** 2026-04-17T02:55Z
- **Tasks:** 1
- **Files modified:** 1 (created)

## Accomplishments

- `cmd/peeringdb-plus/e2e_privacy_test.go` created (740 lines) with two top-level tests and 19 sub-tests:
  - **`TestE2E_AnonymousCannotSeeUsersPoc`** (TierPublic): 10 sub-tests asserting the Users POC is absent on every read surface.
  - **`TestE2E_PublicTierUsersAdmitsRow`** (TierUsers): 9 sub-tests asserting the Users POC is returned on every read surface.
- Every sub-test runs through the REAL `buildMiddlewareChain` against a real ent client with a real `privacy.DecisionContext(Allow)`-seeded POC — no mocks at the privacy boundary.
- Direct-lookup sub-tests assert the surface-native not-found idiom (`404` / `CodeNotFound` / empty GraphQL edges), closing the existence-leak attack per D-13/D-14.
- Plan 59-05's single-call-site audit (`TestSyncBypass_SingleCallSite`) remains green — the test-file bypass is exempt.

## Task Commits

1. **Task 1: E2E anonymous-cannot-see + users-can-see across all 5 surfaces** — `ce87c94` (test)

No final metadata commit — orchestrator takes care of SUMMARY propagation.

## Files Created/Modified

- `cmd/peeringdb-plus/e2e_privacy_test.go` — new, 740 lines. Two top-level tests; `buildE2EFixture(t, tier)` helper that wires the full production middleware chain (`buildMiddlewareChain`) around an in-process mux carrying pdbcompat (`/api/`), entrest (`/rest/v1/`), ConnectRPC PocService (`/peeringdb.v1.PocService/`), GraphQL (`/graphql`), and web (`/ui/`, `/static/`, `/favicon.ico`) against an isolated in-memory SQLite ent client. One `visible="Users"` POC is seeded via `privacy.DecisionContext(ctx, privacy.Allow)` — the same bypass used by `internal/sync/worker.go`. Parallelism-safe via `sync/atomic` DB counter.

## Decisions Made

- **Inline mux construction in the fixture, not extraction into main.go.** The plan's "practicalities" section allows either; inline keeps the main-package test self-contained and avoids exporting internal wiring. `TestMiddlewareChain_Order` still regression-locks the production chain — the test fixture just replays the same call sequence.
- **GraphQL by-id lookup via `pocs(where:{id:N})`.** The project's root Query exposes no direct `poc(id:)` field (only `node(id:)` — which returns `Node` and cannot resolve POC IDs reliably without `GlobalUniqueID`, per `TestGraphQLAPI_NodeQuery`). The `pocs` connection with an id where-filter still materialises as `*ent.PocQuery` and therefore exercises the Policy, so the assertion shape is equivalent.
- **`/ui/` surface assertion uses the contacts fragment at `/ui/fragment/net/{netID}/contacts`.** There is no `/ui/poc/{id}` route in `web.Handler`; POCs surface through the network's contacts fragment. This is strictly better for the privacy assertion because it exercises the edge-traversal query path (`network.QueryPocs`) — RESEARCH.md Pitfall 5 specifically flagged that a regression in edge traversal would bypass a direct-lookup-only test.
- **Tier flipped via `chainConfig.DefaultTier` parameter, not `t.Setenv("PDBPLUS_PUBLIC_TIER", ...)`.** Matches how production wires the middleware and avoids env-var ordering hazards in parallel tests. The middleware reads from `chainConfig.DefaultTier` which is exactly what we control.
- **Readiness middleware bypassed via an `e2eAlwaysReady{}` stub.** Real sync-worker plumbing would pull in `sync_status` table setup that is privacy-irrelevant. The real readiness middleware still runs (the wrap happens in `buildMiddlewareChain`), it just receives `HasCompletedSync()==true`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 — Bug] GetPoc().GetVisible() is *wrapperspb.StringValue, not string**
- **Found during:** Task 1 (initial `go vet`)
- **Issue:** Proto `Poc.visible` is declared `optional string`, which `entproto` compiles to `*wrapperspb.StringValue`. The initial draft compared it directly to `"Users"`, failing compilation with `mismatched types *wrapperspb.StringValue and untyped string`.
- **Fix:** Unwrap via `.GetValue()` — `resp.GetPoc().GetVisible().GetValue()`.
- **Files modified:** `cmd/peeringdb-plus/e2e_privacy_test.go` (one line).
- **Verification:** `go vet ./cmd/peeringdb-plus/...` clean.
- **Committed in:** `ce87c94` (part of Task 1 commit).

**2. [Rule 3 — Blocking] Unused `context` import after helper refactor**
- **Found during:** Task 1 (second `go vet`)
- **Issue:** The initial import list included `context` but after consolidating the fixture to use `t.Context()` directly, the import was no longer referenced.
- **Fix:** Removed `"context"` from the import block.
- **Files modified:** `cmd/peeringdb-plus/e2e_privacy_test.go` (one line).
- **Verification:** `go vet ./cmd/peeringdb-plus/...` clean.
- **Committed in:** `ce87c94` (part of Task 1 commit).

---

**Total deviations:** 2 auto-fixed (1 bug, 1 blocking). Both were trivial compiler-reported issues in the initial draft; no scope creep, no plan changes.
**Impact on plan:** None — the test's behavioural contract is exactly as the plan specified.

## Issues Encountered

- **`/ui/poc/{id}` route does not exist.** The plan listed `/ui/poc/{id}` as one possible `/ui/` endpoint. Web handler inspection showed the project never exposed a dedicated POC detail page; POCs only render through the network detail page's contacts fragment (`/ui/fragment/net/{netID}/contacts`). The plan explicitly anticipated this with "`/ui/poc/{id}` if present OR `/ui/asn/{asn}` showing the network's POCs", so the fallback path was taken. Strictly better coverage: the contacts fragment exercises edge-traversal privacy (`network.QueryPocs`), which is the exact path RESEARCH.md Pitfall 5 warned about.

- **GraphQL root `poc(id:)` field does not exist.** The plan's example sketch used `{poc(id:N){id}}`. Project schema offers `node(id:)` (Node interface, unreliable without `GlobalUniqueID`) and `pocs(where:PocWhereInput)` (connection). Chose the latter; the resulting query still materialises as `*ent.PocQuery` so the Policy runs. The assertion shape (`len(edges)==0` under TierPublic, `==1` under TierUsers) is equivalent to `data.poc == null` vs non-null and arguably stronger because it also exercises where-filter plumbing.

## User Setup Required

None — no external service configuration required. This phase is test-only.

## Next Phase Readiness

- **D-15 correctness contract fulfilled.** The full privacy floor — middleware → `privctx.TierFrom` → ent Policy → per-surface handler → surface-native not-found mapping — is end-to-end tested across all 5 surfaces and both tiers.
- **Regression guard in place.** Any future handler that bypasses the Policy (e.g. a custom resolver that opens a fresh `context.Background()`, a surface that skips `ent.IsNotFound` mapping) flips a deterministic E2E assertion.
- **Ready for Phase 60 verification** — the phase-level "does anonymous == upstream anonymous?" sweep. This E2E test is the per-surface backbone; Phase 60 can layer higher-granularity conformance checks on top.
- **Ready for v1.15 OAuth.** When OAuth lands and sets `privctx.WithTier(ctx, TierUsers)` from the callback, the same TierUsers assertions will pass without any changes — the policy path is identity-source-agnostic.

## Self-Check: PASSED

- **File exists:**
  - FOUND: `cmd/peeringdb-plus/e2e_privacy_test.go`
- **Commit exists:**
  - FOUND: `ce87c94` (`test(59-06): add D-15 end-to-end privacy contract across all 5 surfaces`)
- **Acceptance-criteria greps:**
  - `grep -c "t.Run" cmd/peeringdb-plus/e2e_privacy_test.go` = `19` (≥ 10 required)
  - `grep -n "privacy.DecisionContext" cmd/peeringdb-plus/e2e_privacy_test.go` = 3 matches incl. 1 call site (≥ 1 required)
  - `grep -n "buildMiddlewareChain" cmd/peeringdb-plus/e2e_privacy_test.go` = 3 matches (≥ 1 required)
  - `grep -c "TierPublic\|TierUsers\|DefaultTier" cmd/peeringdb-plus/e2e_privacy_test.go` = `23` (≥ 4 required)
- **Tests:**
  - `go test -race -run 'TestE2E_AnonymousCannotSeeUsersPoc' ./cmd/peeringdb-plus/...` → PASS (10 sub-tests)
  - `go test -race -run 'TestE2E_PublicTierUsersAdmitsRow' ./cmd/peeringdb-plus/...` → PASS (9 sub-tests)
  - `go test -race ./...` full suite → PASS, no regressions
  - `TestSyncBypass_SingleCallSite` (Plan 59-05 audit) → PASS (test file exempt)
  - `go vet ./... && golangci-lint run ./cmd/peeringdb-plus/...` → clean, 0 issues

---

*Phase: 59-ent-privacy-policy-sync-bypass*
*Plan: 06*
*Completed: 2026-04-17*
