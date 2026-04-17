---
phase: 59-ent-privacy-policy-sync-bypass
plan: 01
subsystem: privacy
tags: [context, privacy, tier, foundation, stdlib]

# Dependency graph
requires:
  - phase: 58-visibility-schema-alignment
    provides: "*_visible fields on POC and any other entities"
provides:
  - "internal/privctx package exporting Tier, TierPublic, TierUsers, WithTier, TierFrom"
  - "Private tierCtxKey struct type preventing cross-package collisions"
  - "Fail-safe-closed default: un-stamped contexts report TierPublic"
affects:
  - 59-02 (config parse of PDBPLUS_PUBLIC_TIER → privctx.Tier)
  - 59-03 (HTTP middleware stamps privctx.WithTier on every request)
  - 59-04 (ent privacy policy reads privctx.TierFrom)
  - 59-05 (sync bypass — uses privacy.DecisionContext, NOT privctx)
  - 59-06 (E2E test asserts tier propagation through the chain)
  - 60 (verification phase — consumes this package)
  - v1.15 OAuth (callback stamps privctx.TierUsers)

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Typed context key (struct{}) — mirrors ent's decisionCtxKey pattern"
    - "Typed tier enum with zero-value = restrictive tier (fail-safe-closed)"
    - "Separation of concerns: typed-tier abstraction decoupled from ent privacy.DecisionContext so v1.15 OAuth slots in without policy changes"

key-files:
  created:
    - internal/privctx/privctx.go
    - internal/privctx/privctx_test.go
  modified: []

key-decisions:
  - "TierPublic is iota (zero value) so any un-stamped context defaults to the safest tier"
  - "tierCtxKey is an unexported struct{} — type identity guarantees no external package can collide with or overwrite our key"
  - "TierFrom's type-assertion fallback returns TierPublic (never panics, never leaks Users rows on mis-wiring)"
  - "No dependency on ent/privacy — keeps HTTP-tier abstraction orthogonal to ent's DecisionContext per CONTEXT.md D-07"

patterns-established:
  - "Pattern: private context key via struct{} — reuse across any future context-bound values"
  - "Pattern: zero-value-as-safe-default for security-relevant enums"

requirements-completed: [SYNC-03]

# Metrics
duration: 6min
completed: 2026-04-17
---

# Phase 59 Plan 01: privctx Foundation Summary

**Typed visibility-tier context propagation package (`internal/privctx`) with collision-safe private key and fail-safe-closed TierPublic default, wired to stdlib only.**

## Performance

- **Duration:** ~6 min
- **Started:** 2026-04-17T01:27Z
- **Completed:** 2026-04-17T01:33Z
- **Tasks:** 1 (TDD: RED + GREEN, no REFACTOR needed)
- **Files created:** 2
- **Files modified:** 0

## Accomplishments
- Shipped the Wave 1 foundation for phase 59: a ~50-line package defining `Tier` + `WithTier` + `TierFrom`.
- Private `tierCtxKey struct{}` prevents any external package from forging or reading the tier under our key — mirrors ent's own `decisionCtxKey` pattern verbatim.
- `TierPublic` is the iota zero value, so unstamped contexts default to the most restrictive tier (fail-safe-closed per CONTEXT.md D-04).
- 4 table-driven / focused tests (2 subtests under `TestTier_Roundtrip`) covering roundtrip, zero-value default, defence-in-depth against foreign keys, and derived-ctx inheritance. All pass under `-race`.

## Task Commits

Each task was committed atomically (TDD cycle):

1. **Task 1 RED — failing tests** — `2863a43` (test): TestTier_Roundtrip, TestTierFrom_ZeroValueIsPublic, TestTierFrom_WrongTypeIsPublic, TestWithTier_ChildCtxInheritsValue
2. **Task 1 GREEN — implementation** — `7d64112` (feat): Tier / TierPublic / TierUsers / WithTier / TierFrom with private tierCtxKey

No REFACTOR commit — the implementation is ~50 lines and already idiomatic.

## Files Created/Modified

- `internal/privctx/privctx.go` (52 lines) — Package docs, Tier enum, private tierCtxKey, WithTier, TierFrom. Stdlib-only (imports `context`).
- `internal/privctx/privctx_test.go` (78 lines) — External test package (`privctx_test`) per Go convention; 4 tests + 2 subtests; all `t.Parallel()` safe per GO-T-3.

## Decisions Made

- **External test package (`package privctx_test`) instead of white-box `package privctx`** — confirms the exported API is sufficient for downstream callers (plans 02-06 will consume exactly this surface). White-box access is not needed because the "wrong type" defence test uses a foreign key from the test's own package, which is actually more realistic than a same-package test.
- **Table-driven `TestTier_Roundtrip`** — two subtests (TierPublic, TierUsers) per GO-T-1; trivially extensible when `TierAdmin` or similar is added later.
- **`t.Cleanup(cancel)` for the context-with-timeout test** — per GO-T-2; avoids leaking the cancel func if the test panics.
- **Kept commit messages aligned with TDD RED/GREEN gate expectations** — `test(59-01):` for the failing-test commit, `feat(59-01):` for the implementation, so phase-level TDD-gate verification in future audits passes.

## Deviations from Plan

None — plan executed exactly as written. All acceptance criteria (every grep pattern, `go vet`, `golangci-lint run`, `go test -race`) passed without modification.

## Issues Encountered

- **Worktree was at `3f4c8ad` (pre-phase-59) not the expected base `775e67a`.** Per the `worktree_branch_check` protocol, hard-reset to `775e67a5b78b8bef4ab0dce07d31889f606f36be` to pick up the phase-59 plan artifacts (59-01-PLAN.md etc.). No work lost — the prior HEAD was simply older than the expected base. Resolved in seconds.

## Verification Evidence

```
$ go test -race -v ./internal/privctx/...
=== RUN   TestTier_Roundtrip
=== RUN   TestTier_Roundtrip/public
=== RUN   TestTier_Roundtrip/users
--- PASS: TestTier_Roundtrip (0.00s)
    --- PASS: TestTier_Roundtrip/users (0.00s)
    --- PASS: TestTier_Roundtrip/public (0.00s)
--- PASS: TestTierFrom_ZeroValueIsPublic (0.00s)
--- PASS: TestTierFrom_WrongTypeIsPublic (0.00s)
--- PASS: TestWithTier_ChildCtxInheritsValue (0.00s)
PASS
ok  	github.com/dotwaffle/peeringdb-plus/internal/privctx	1.014s

$ go vet ./internal/privctx/...
(clean)

$ golangci-lint run ./internal/privctx/...
0 issues.
```

All acceptance-criteria greps match:
- `type Tier int` — 1 match (line 17)
- `TierPublic Tier = iota` — 1 match (line 22)
- `TierUsers` — 1 match in declaration (line 27)
- `type tierCtxKey struct{}` — 1 match (line 34)
- `func WithTier` — 1 match (line 39)
- `func TierFrom` — 1 match (line 47)

Dependency surface confirmed stdlib-only via `go list -deps` (only `context` plus its transitive stdlib graph).

## User Setup Required

None — pure library code, no external services or env vars introduced in this plan (env var `PDBPLUS_PUBLIC_TIER` lands in plan 59-02).

## Threat Flags

None. The plan's declared threat `T-59-05 (Tampering of exported API)` is mitigated as planned: unexported `tierCtxKey struct{}` + typed `Tier int` + zero-value-is-restrictive. No new threat surface introduced beyond the plan's register.

## Next Plan Readiness

- **Plan 59-02 (config parser for `PDBPLUS_PUBLIC_TIER`)** — can import `github.com/dotwaffle/peeringdb-plus/internal/privctx` and map env strings to `privctx.TierPublic`/`privctx.TierUsers`. No blockers.
- **Plan 59-03 (HTTP middleware)** — can import the package and call `privctx.WithTier(ctx, cachedTier)`. No blockers.
- **Plan 59-04 (ent privacy policy)** — can import and call `privctx.TierFrom(ctx)` from query rule. No blockers.
- **Plan 59-05 (sync bypass)** — independent; uses `privacy.DecisionContext` directly, does NOT depend on this package per D-07/D-08.
- **Plan 59-06 (E2E test)** — will exercise the full chain via real HTTP; foundation ready.

## Self-Check: PASSED

Verified post-write:
- `internal/privctx/privctx.go` exists (52 lines) — FOUND
- `internal/privctx/privctx_test.go` exists (78 lines) — FOUND
- Commit `2863a43` (RED) — FOUND in `git log --all`
- Commit `7d64112` (GREEN) — FOUND in `git log --all`
- `go test -race ./internal/privctx/...` — GREEN (4 tests + 2 subtests pass)
- `go vet ./internal/privctx/...` — clean
- `golangci-lint run ./internal/privctx/...` — 0 issues
- Success criteria (all 4 plan-level + all 4 agent-prompt) — all met

## TDD Gate Compliance

- **RED gate:** `test(59-01):` commit present (`2863a43`) before any implementation; all 4 tests failed as expected (build error: "no non-test Go files").
- **GREEN gate:** `feat(59-01):` commit present (`7d64112`) after the RED commit; all tests green under `-race`.
- **REFACTOR gate:** Not applicable — implementation is already minimal and idiomatic.

---
*Phase: 59-ent-privacy-policy-sync-bypass*
*Completed: 2026-04-17*
