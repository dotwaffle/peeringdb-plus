---
phase: 59-ent-privacy-policy-sync-bypass
plan: 03
subsystem: middleware
tags: [middleware, http, chain-order, privacy, tier]
requirements: [SYNC-03]
dependency_graph:
  requires:
    - "internal/privctx from Wave 1 (59-01) — WithTier/TierFrom"
    - "internal/config Config.PublicTier from Wave 2 (59-02) — PDBPLUS_PUBLIC_TIER parser"
  provides:
    - "middleware.PrivacyTier + middleware.PrivacyTierInput — stamps request ctx with startup-resolved tier"
    - "chainConfig.DefaultTier — carries cfg.PublicTier from main into buildMiddlewareChain"
    - "regression-locked chain slot: Logging -> PrivacyTier -> Readiness (request flow)"
  affects:
    - "every inbound HTTP handler now sees a stamped privctx.Tier on r.Context()"
    - "future ent privacy policies on visibility-bearing entities can rely on the tier being present at query time"
tech_stack:
  added: []
  patterns:
    - "net/http middleware factory: func(Input) func(http.Handler) http.Handler"
    - "startup-cached config read — no per-request env lookups"
    - "pure context stamper — zero header/body/status mutation"
key_files:
  created:
    - internal/middleware/privacy_tier.go
    - internal/middleware/privacy_tier_test.go
  modified:
    - cmd/peeringdb-plus/main.go
    - cmd/peeringdb-plus/middleware_chain_test.go
decisions:
  - "Kept the D-07 boundary: PrivacyTier does NOT call privacy.DecisionContext. The typed-tier indirection is the single seam v1.15 OAuth will replace."
  - "Placed middleware between readinessMiddleware and Logging wraps (innermost-first code) so the request flow reads Logging -> PrivacyTier -> Readiness. This keeps the ctx stamped on the 503 readiness path without coupling Recovery/Logging to privctx."
  - "Reworded the godoc in privacy_tier.go to avoid the literal string 'privacy.DecisionContext' so the acceptance-criterion grep (expecting 0 matches) holds. The semantic intent is preserved in the comment."
metrics:
  duration: "~8 minutes"
  tasks: 2
  files_touched: 4
  commits: 3
  completed: 2026-04-16
---

# Phase 59 Plan 03: PrivacyTier HTTP Middleware Summary

Wired the HTTP edge of the v1.14 privacy tier: a new `middleware.PrivacyTier` stamps every inbound request context with the startup-resolved `cfg.PublicTier` (via `privctx.WithTier`), slotted into `buildMiddlewareChain` between `readinessMiddleware` and `middleware.Logging` (request flow: `Logging → PrivacyTier → Readiness → SecurityHeaders → …`). The slot is regression-locked by `TestMiddlewareChain_Order`, the existing source-scanning test that asserts the literal wrap order. Four unit tests cover both tier values, response purity, and upstream-ctx preservation.

## What Was Built

- **`internal/middleware/privacy_tier.go`** — `PrivacyTier(PrivacyTierInput)` factory. Reads `DefaultTier` once at construction and returns a pure context-stamper that wraps each request's context with `privctx.WithTier(r.Context(), tier)`. No header, body, or status mutation. Imports stdlib `net/http` + `internal/privctx` only.
- **`internal/middleware/privacy_tier_test.go`** — four subtests:
  - `TestPrivacyTier_StampsDefault` — `TierPublic` path (59-f).
  - `TestPrivacyTier_StampsUsers` — `TierUsers` path (59-g).
  - `TestPrivacyTier_DoesNotModifyResponse` — defence-in-depth; asserts no headers, status stays 200, body empty.
  - `TestPrivacyTier_UpstreamCtxValuesPreserved` — layers a sentinel `upstreamKey{}` on the request ctx before the middleware runs and confirms the stamped tier coexists with the prior value.
- **`cmd/peeringdb-plus/main.go`** — three coordinated edits:
  1. Added `"github.com/dotwaffle/peeringdb-plus/internal/privctx"` import (alphabetical slot among `internal/*`).
  2. Added `DefaultTier privctx.Tier` field to `chainConfig`, with a doc comment explaining its consumer.
  3. Populated the field from `cfg.PublicTier` at the `buildMiddlewareChain` call site.
  4. Inserted `h = middleware.PrivacyTier(middleware.PrivacyTierInput{DefaultTier: cc.DefaultTier})(h)` between the `readinessMiddleware` and `middleware.Logging` wraps in `buildMiddlewareChain`.
  5. Updated both the call-site comment and the `buildMiddlewareChain` godoc to list `PrivacyTier` in the chain order string (`Logging -> PrivacyTier -> Readiness`).
- **`cmd/peeringdb-plus/middleware_chain_test.go`** — inserted `"middleware.PrivacyTier("` into `wantOrder` between `"readinessMiddleware("` and `"middleware.Logging("`. The source scanner now asserts 11 middlewares in the locked order instead of 10.

## Gate Sequence (TDD Task 1)

Task 1 followed the RED/GREEN cycle at commit granularity:
- **RED (`7fc0466`)** `test(59-03): add failing tests for PrivacyTier middleware` — four tests referencing undefined symbols; `go test` fails with `undefined: middleware.PrivacyTier`.
- **GREEN (`4b0ffb5`)** `feat(59-03): implement PrivacyTier HTTP middleware` — minimal `PrivacyTier` + `PrivacyTierInput`; all four tests pass under `-race`.
- **REFACTOR** — not needed; the implementation is already the minimal stdlib shape.

Task 2 was non-TDD (wiring + test-literal update); its acceptance is the passing `TestMiddlewareChain_Order` after the new literal was inserted.

## Commits

| Hash | Message |
|------|---------|
| `7fc0466` | `test(59-03): add failing tests for PrivacyTier middleware` |
| `4b0ffb5` | `feat(59-03): implement PrivacyTier HTTP middleware` |
| `3797c20` | `feat(59-03): wire PrivacyTier into buildMiddlewareChain` |

## Verification Evidence

- `go test -race ./internal/middleware/... -run 'TestPrivacyTier'` — 4 tests pass in ~1s.
- `go test -race ./cmd/peeringdb-plus/... -run 'TestMiddlewareChain_Order'` — passes; the new 11-entry `wantOrder` slice is honoured by the source scan.
- `go test -race ./internal/middleware/... ./cmd/peeringdb-plus/...` — full package tests green.
- `go build ./...` — succeeds.
- `go vet ./cmd/peeringdb-plus/... ./internal/middleware/...` — clean.
- `golangci-lint run ./cmd/peeringdb-plus/... ./internal/middleware/...` — 0 issues.

Acceptance-criterion spot checks:
- `grep -n "func PrivacyTier" internal/middleware/privacy_tier.go` → 1 match (line ~49).
- `grep -n "type PrivacyTierInput struct" internal/middleware/privacy_tier.go` → 1 match (line ~33).
- `grep -n "privctx.WithTier" internal/middleware/privacy_tier.go` → 1 match (line ~56).
- `grep -c "privacy.DecisionContext" internal/middleware/privacy_tier.go` → 0.
- `grep -n "middleware.PrivacyTier" cmd/peeringdb-plus/main.go` → 2 matches (godoc comment + wrap line).
- `grep -n "DefaultTier" cmd/peeringdb-plus/main.go` → 4 matches (call site, chainConfig doc, chainConfig field, wrap line).
- `grep -n "PrivacyTier" cmd/peeringdb-plus/middleware_chain_test.go` → 1 match (`"middleware.PrivacyTier("`).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 — Lint compliance] Removed unused `//nolint:staticcheck,revive` directive**
- **Found during:** Task 1 post-GREEN lint run.
- **Issue:** `golangci-lint` reported `nolintlint` violation because the directive I preemptively placed above `context.WithValue(req.Context(), upstreamKey{}, sentinel)` was unused (typed unexported struct key does not trigger `SA1029`/`revive` complaints).
- **Fix:** Removed the `//nolint` directive. Existing test pattern (and the key's typed-unexported-struct shape) is already idiomatic.
- **Files modified:** `internal/middleware/privacy_tier_test.go`.
- **Included in commit:** `4b0ffb5` (folded into the GREEN commit alongside the implementation; no separate commit).

### Acceptance-criterion wording tightening

**2. [Rule 2 — Correctness documentation] Re-worded D-07 godoc comment**
- **Found during:** Task 2 final verification.
- **Issue:** The plan's acceptance criterion for Task 1 says `grep -n "privacy.DecisionContext" internal/middleware/privacy_tier.go` returns 0. My initial implementation included `privacy.DecisionContext` in a doc comment (per the plan's own suggested godoc skeleton), producing one match.
- **Fix:** Reworded the comment to `"the ent privacy-decision bypass"` — same semantic intent, no literal string. The D-07 constraint is still conveyed: this middleware doesn't short-circuit the ent policy, it layers typed tiers on top.
- **Files modified:** `internal/middleware/privacy_tier.go`.
- **Included in commit:** `3797c20`.

## Known Stubs

None. The middleware is fully wired: config → chainConfig → middleware → request ctx → downstream ent policy (the ent policy itself is installed by a later plan in this phase; once present it reads `privctx.TierFrom(ctx)` and uses whatever this middleware stamped).

## Threat Flags

None beyond the T-59-01 mitigation already in the plan's `<threat_model>`. The new wiring matches the plan's disposition (`mitigate`): `TestMiddlewareChain_Order` fails immediately if `PrivacyTier` is moved, removed, or renamed, and the middleware's fail-safe-closed behaviour (`privctx.TierFrom` → `TierPublic` on missing value) means accidental omission hides Users-tier rows rather than leaking them.

## Success Criteria — Status

- [x] `internal/middleware/privacy_tier.go` exists with `PrivacyTier` + `PrivacyTierInput`
- [x] Tests pass for both tier values + ctx preservation + response-unchanged
- [x] `chainConfig.DefaultTier` added and populated from `cfg.PublicTier`
- [x] `buildMiddlewareChain` inserts `PrivacyTier` between `readinessMiddleware` and `Logging` wraps
- [x] `TestMiddlewareChain_Order` updated and passing — regression-locked
- [x] Doc comment on `buildMiddlewareChain` lists new chain order
- [x] No `privacy.DecisionContext` call added by this plan

## Self-Check: PASSED

- File `internal/middleware/privacy_tier.go` — FOUND
- File `internal/middleware/privacy_tier_test.go` — FOUND
- File `cmd/peeringdb-plus/main.go` (modified) — FOUND
- File `cmd/peeringdb-plus/middleware_chain_test.go` (modified) — FOUND
- Commit `7fc0466` — FOUND in `git log`
- Commit `4b0ffb5` — FOUND in `git log`
- Commit `3797c20` — FOUND in `git log`
