---
phase: 60-surface-integration-tests
plan: 01
subsystem: testutil
tags: [seed, privacy, visibility, test-infrastructure, VIS-06]
requires:
  - phase-59 ent Privacy policy on Poc (visible != "Public" filtered for TierPublic)
  - phase-59 bypass audit (internal/sync/bypass_audit_test.go)
  - internal/privctx.WithTier / TierFrom
  - entgo.io/ent/privacy.DecisionContext (re-exported via ent/privacy)
provides:
  - seed.Full mixed-visibility fixture: 1 Public POC (ID 500) + 2 Users POCs (IDs 9000, 9001)
  - Result.UsersPoc, Result.UsersPoc2, Result.AllPocs typed handles
  - Regression tests locking the fixture shape for phase 60 plans 02-05
affects:
  - internal/testutil/seed/seed.go
  - internal/testutil/seed/seed_mixed_visibility_test.go
  - internal/sync/bypass_audit_test.go (skip testutil subtree + add TestTestutilIsTestOnly guard)
tech-stack:
  added: []
  patterns:
    - "privacy.DecisionContext(ctx, privacy.Allow) to seed Users-tier rows in test infrastructure"
    - "Black-box _test package for fixture contract assertions"
key-files:
  created:
    - internal/testutil/seed/seed_mixed_visibility_test.go
  modified:
    - internal/testutil/seed/seed.go
    - internal/sync/bypass_audit_test.go
decisions:
  - "Used github.com/dotwaffle/peeringdb-plus/ent/privacy (project re-export) rather than entgo.io/ent/privacy directly, to match the convention in internal/sync/policy_test.go and ent/schema/poc.go."
  - "Added new tests in a separate file seed_mixed_visibility_test.go (package seed_test) rather than extending the existing seed_test.go (package seed) — keeps the privctx/ent-privacy imports out of package seed and exercises the exported surface, which is what downstream callers see."
  - "Extended the bypass audit to skip internal/testutil/ AND added TestTestutilIsTestOnly to keep the exemption honest (fails if anything outside *_test.go ever imports testutil)."
metrics:
  completed: 2026-04-16
  tasks: 2
  duration_min: ~20
---

# Phase 60 Plan 01: Mixed-visibility seed Summary

Extended `seed.Full` in place so every phase 60 plan 02-05 surface test lands on the
same canonical fixture — 1 `visible="Public"` POC (ID 500) plus 2 `visible="Users"` POCs
(IDs 9000, 9001) — without forking the helper.

## Outcome

- `internal/testutil/seed/seed.go`: new `Result` fields `UsersPoc` (ID 9000 on
  `r.Network`), `UsersPoc2` (ID 9001 on `r.Network2`), and `AllPocs` (deterministic
  `[r.Poc, r.UsersPoc, r.UsersPoc2]`). The two Users POCs are written via
  `privacy.DecisionContext(ctx, privacy.Allow)`, mirroring the sync-worker bypass.
- `internal/testutil/seed/seed_mixed_visibility_test.go` (new, package `seed_test`):
  three tests — `TestFull_HasUsersPocs`, `TestFull_PublicCountsUnchanged`,
  `TestFull_PrivacyFilterShapes` — lock the shape in. Filter test asserts
  `TierPublic=1, TierUsers=3, bypass=3` through the canonical fixture.
- `internal/sync/bypass_audit_test.go`: skip `internal/testutil/` subtree, add
  `TestTestutilIsTestOnly` guard so the exemption is self-enforcing (fails build if any
  non-`_test.go` file ever imports a testutil subpackage).

## Final ID assignments

| Handle | Entity type | ID | Visibility | Owning network |
|---|---|---|---|---|
| `r.Poc` (pre-existing) | Poc | 500 | Public | Network (ID 10, Cloudflare) |
| `r.UsersPoc` | Poc | 9000 | Users | Network (ID 10) |
| `r.UsersPoc2` | Poc | 9001 | Users | Network2 (ID 11, Hurricane Electric) |

`AllPocs` is set to `[r.Poc, r.UsersPoc, r.UsersPoc2]` — consumers can assert
`AllPocs[0]` is the Public row without a sort step.

## New `Result` fields

```go
UsersPoc   *ent.Poc    // visible="Users" POC on r.Network, ID 9000
UsersPoc2  *ent.Poc    // visible="Users" POC on r.Network2, ID 9001
AllPocs    []*ent.Poc  // deterministic [Public, Users, Users2]
```

## Verification

Explicit plan verification commands, all run with `TMPDIR=/tmp/claude-1000`:

- `go test -race ./internal/testutil/seed/...` PASS (pre-existing TestFull, TestFull_EntityCounts, TestFull_Relationships still green plus 3 new tests)
- `go test -race ./internal/sync/...` PASS (including `TestSyncBypass_SingleCallSite` and the new `TestTestutilIsTestOnly`)
- `go test -race ./internal/grpcserver/...` PASS
- `go test -race ./internal/web/...` PASS
- `go test -race ./internal/pdbcompat/...` PASS
- `go test -race ./cmd/peeringdb-plus/...` PASS — critically `TestE2E_AnonymousCannotSeeUsersPoc` (phase 59 contract) is unaffected by the new mix
- `go test -race ./graph/...` PASS — `TestGraphQLAPI_OffsetLimitListResolvers.pocsList` still asserts `NOC Contact` because the privacy policy filters the 2 new Users rows at TierPublic
- `go build ./...` PASS
- `grep -rn FullWithVisibility internal/ cmd/ graph/` — **no hits** (D-03 enforced)

## Deviations from Plan

### Adjustments

**1. [Adjustment] Import path uses project-local `ent/privacy` not `entgo.io/ent/privacy`**

- **Found during:** Task 1 (file-level review of existing imports)
- **Issue:** The plan's Task 1 step 3 said to import `entgo.io/ent/privacy`. Existing
  call sites (`internal/sync/policy_test.go:10`, `ent/schema/poc.go:16`,
  `internal/sync/worker.go`) uniformly use the project-local
  `github.com/dotwaffle/peeringdb-plus/ent/privacy`, which re-exports the upstream
  symbols (`Allow`, `Deny`, `Skip`, `DecisionContext`) via `ent/privacy/privacy.go`.
- **Fix:** Used the project-local path to match convention. Both paths expose
  identical symbols; the local path is also what the bypass audit regex
  (`privacy\.DecisionContext\(..., privacy\.Allow\)`) scans for, so either would pass
  the audit — picking the local one avoids a stylistic anomaly in the codebase.
- **Files modified:** `internal/testutil/seed/seed.go`
- **Commit:** 29cee5e

**2. [Rule 3 - Blocking] Extended bypass audit to skip `internal/testutil/`**

- **Found during:** Task 1 verification — ran
  `go test -run TestSyncBypass_SingleCallSite ./internal/sync/` and it failed with
  "found multiple: internal/sync/worker.go:260, internal/testutil/seed/seed.go:247".
- **Issue:** Phase 59 plan 59-05 installed a single-call-site audit that scans all
  non-`_test.go` Go files in `internal/`, `cmd/`, `ent/schema/`, `graph/` and
  requires exactly one hit for `privacy.DecisionContext(ctx, privacy.Allow)` and
  exactly one reference to `privacy.Allow`, both in `internal/sync/worker.go`. Adding
  the bypass to `seed.go` in the `internal/testutil/seed` subtree violates that
  invariant.
- **Fix:** Two-part change in `internal/sync/bypass_audit_test.go`:
  1. Added a skip clause for paths containing
     `{sep}internal{sep}testutil{sep}` in `TestSyncBypass_SingleCallSite`'s
     WalkDir callback, next to the existing `ent/` and `graph/generated.go` skips.
  2. Added a new sibling test, `TestTestutilIsTestOnly`, that walks the same scan
     roots and asserts no non-`_test.go` file imports
     `github.com/dotwaffle/peeringdb-plus/internal/testutil{,/*}`. This makes the
     exemption self-enforcing: the exemption is safe precisely because testutil is
     only reachable from tests, so the new test turns that invariant into a hard
     guard rail.
- **Why this is principled:** `internal/testutil/` is test-only infrastructure
  that never ships in a production binary (no non-test file imports it, enforced
  by the new guard). The bypass the seed performs mirrors the runtime
  sync-writer's bypass pattern exactly — per the plan's own design note in
  Task 1's `<action>` block: "this usage is legitimate and mirrors runtime
  sync-writer behaviour." Rule 3 deviation (blocks progress on a task the plan
  explicitly mandates).
- **Files modified:** `internal/sync/bypass_audit_test.go`
- **Commit:** 29cee5e

**3. [Adjustment] Separate file for new tests (package `seed_test`) rather than extending existing `seed_test.go` (package `seed`)**

- **Found during:** Task 2 — discovered existing `internal/testutil/seed/seed_test.go`
  already uses `package seed` (white-box internal tests: `TestFull`,
  `TestFull_EntityCounts`, etc.).
- **Issue:** Plan Task 2 said "Create `internal/testutil/seed/seed_test.go`" but the
  file exists with internal-package tests. Plan also specifies `package seed_test`
  (black-box), which would have conflicted.
- **Fix:** Created `seed_mixed_visibility_test.go` in package `seed_test` alongside
  the existing internal-package `seed_test.go`. Both files coexist in the same
  directory (legal Go pattern for black-box + white-box tests on the same
  package). The three new tests (`TestFull_HasUsersPocs`,
  `TestFull_PublicCountsUnchanged`, `TestFull_PrivacyFilterShapes`) all live in the
  new file with the specified signatures.
- **Why this is principled:** Keeps `privctx` and `ent/privacy` imports out of
  package `seed`'s public API surface (they're used only in tests), and the
  plan's explicit `package seed_test` requirement drove the split. Naming the
  file `seed_mixed_visibility_test.go` makes the concern obvious to future
  readers.
- **Files modified:** Created `internal/testutil/seed/seed_mixed_visibility_test.go`
- **Commit:** 84b63a2

### Non-deviations

- No `seed.FullWithVisibility` fork (D-03) — confirmed via
  `grep -rn FullWithVisibility internal/ cmd/ graph/` returning zero hits.
- The existing `TestFull_EntityCounts` assertion `Poc count == 1` now implicitly
  relies on the phase 59 privacy policy filtering the 2 new Users POCs at the
  default TierPublic context. The test still passes — this is the intended D-02
  behaviour (privacy filter is a no-op on Public rows, but *does* filter the Users
  rows for anonymous readers).

## Threat Flags

None. No new surface introduced — extends an existing test-only fixture and
widens one audit's skip list with a compensating guard.

## Authentication Gates

None.

## Known Stubs

None.

## Commits

- 29cee5e — `feat(60-01): extend seed.Full with Users-tier POCs (VIS-06)`
- 84b63a2 — `test(60-01): lock in mixed-visibility seed contract`

## Self-Check

Created/modified files:

- FOUND: `internal/testutil/seed/seed.go`
- FOUND: `internal/testutil/seed/seed_mixed_visibility_test.go`
- FOUND: `internal/sync/bypass_audit_test.go`

Commits:

- FOUND: 29cee5e
- FOUND: 84b63a2

## Self-Check: PASSED
