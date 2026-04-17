---
phase: 60-surface-integration-tests
plan: 04
subsystem: testing
tags: [conformance, peeringdb, live-test, anon-parity, visibility]

# Dependency graph
requires:
  - phase: 60-surface-integration-tests
    provides: "Phase 60 decisions D-10 (anon-only conformance) and D-11 (no authenticated mode)"
  - phase: 59-ent-privacy-policy-sync-bypass
    provides: "ent privacy policy + anon middleware pass — the read-path floor this conformance check aligns with"
provides:
  - "Live conformance test that compares our anonymous /api/{type} shape against upstream beta.peeringdb.com anonymous /api/{type} for all 13 PeeringDB types"
  - "A single comparison mode (anon-vs-anon) — no authenticated code path remains in internal/conformance/live_test.go"
affects:
  - "Any future authenticated-conformance work (must land as its own file/workflow, not as a flag)"
  - "Operator smoke-test workflow for beta drift detection"

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Live external-API conformance tests gated by package-level flag (-peeringdb-live), skipped in CI by default"

key-files:
  created: []
  modified:
    - "internal/conformance/live_test.go"

key-decisions:
  - "Deleted (not gated) the authenticated branch — aligns with D-11 and prevents drift if an operator runs with an API key locally"
  - "Kept the existing golden reference (internal/pdbcompat/testdata/golden/{type}/list.json) rather than switching to testdata/visibility-baseline — golden files are the shape we serve, which is exactly what we want upstream to continue matching (plan-time decision, confirmed)"
  - "Collapsed sleepDuration to unconditional 3s — anon rate ceiling is the only relevant bound now"

patterns-established:
  - "anon-vs-anon conformance: our locally-committed golden shape is the reference, upstream live response is the candidate, structural diff via conformance.CompareResponses"

requirements-completed: [VIS-07]

# Metrics
duration: 5min
completed: 2026-04-17
---

# Phase 60 Plan 04: Conformance anon-vs-anon Summary

**Stripped authenticated branch from internal/conformance/live_test.go — conformance is now a single anon-vs-anon comparison (D-10/D-11), no API-key handling, no env var read.**

## Performance

- **Duration:** ~5 min
- **Started:** 2026-04-17T03:30:00Z (approx)
- **Completed:** 2026-04-17T03:37:00Z
- **Tasks:** 1
- **Files modified:** 1

## Accomplishments
- Removed PDBPLUS_PEERINGDB_API_KEY read and the Authorization: Api-Key request header branch.
- Collapsed sleepDuration to an unconditional 3s (respects anon ≤20/min ceiling).
- Rewrote the test docstring and primary log line to name the mode (anon-vs-anon) and cite D-10/D-11.
- Preserved the -peeringdb-live gate, per-type loop, checkMetaGenerated, findGoldenDir, and the golden-file reference path — no other behaviour changed.

## Task Commits

Each task was committed atomically:

1. **Task 1: Strip authenticated branch from live_test.go, leaving anon-only comparison** — `deb1ecc` (refactor)

## Files Created/Modified
- `internal/conformance/live_test.go` — docstring/log updates + removal of the authenticated code path. Diff: 7 insertions, 14 deletions.

## Scope Confirmation

Only `internal/conformance/live_test.go` was touched. `git diff --name-only internal/conformance/` after the commit returns only that file; `compare.go` and `compare_test.go` are unchanged.

## Verification

All acceptance criteria green:

| Check | Result |
|-------|--------|
| `go vet ./internal/conformance/...` | PASS (no output) |
| `go test -race ./internal/conformance/` | PASS (`ok 1.016s`) |
| `golangci-lint run ./internal/conformance/...` | `0 issues.` |
| `grep 'PDBPLUS_PEERINGDB_API_KEY\|Authorization.*Api-Key\|apiKey'` in live_test.go | No matches |
| `grep 'os.Getenv'` in live_test.go | No matches |
| `grep '-peeringdb-live'` in live_test.go | Matches (gate unchanged) |
| `grep 'anon-vs-anon'` in live_test.go | Matches (new log line + docstring) |
| `grep '3 \* time.Second'` in live_test.go | Matches (unconditional 3s) |
| `grep -c 'if apiKey'` in live_test.go | 0 |
| `grep 't.Run(typeName'` in live_test.go | Matches (per-type loop preserved) |
| `git diff --name-only internal/conformance/compare.go` | Empty |

## Live Smoke Test

Not run — plan explicitly marks this operator-only. Flag-off path (CI default) exercised and passes via `go test -race ./internal/conformance/`, which hits the skip branch as expected.

## Decisions Made
- Followed plan decision 8 verbatim: did NOT switch the golden reference from `internal/pdbcompat/testdata/golden/{type}/list.json` to `testdata/visibility-baseline/beta/anon/api/{type}/page-1.json`. The two tests target complementary halves of D-08/D-10.
- Kept `"os"` import — still used by `os.Stat` in findGoldenDir and `os.ReadFile`/`os.IsNotExist` in the test body.

## Deviations from Plan

None — plan executed exactly as written.

## Issues Encountered

- Worktree initial HEAD (3f4c8ad) was not a descendant of the specified base (d9f8c20). Hard-reset the worktree branch to d9f8c20 per the `<worktree_branch_check>` directive, then applied the planned change on top. This was expected housekeeping, not a functional issue.

## Self-Check: PASSED

- Modified file exists: `internal/conformance/live_test.go`
- Task 1 commit exists: `deb1ecc`
- compare.go NOT modified (confirmed by `git diff --name-only`)
- All acceptance grep checks pass (see Verification table)

## Next Phase Readiness

- Phase 60 conformance axis is now single-mode. Any future "authenticated conformance" work will land as its own file with an explicit operator workflow — not as a flag on this test.
- No blockers for downstream waves in phase 60.

---
*Phase: 60-surface-integration-tests*
*Completed: 2026-04-17*
