---
phase: 59-ent-privacy-policy-sync-bypass
plan: 05
subsystem: sync-privacy-bypass
tags: [sync, privacy, ent, bypass, audit, vis-05, t-59-02]

requires:
  - phase: 59-04
    provides: ent/privacy package (generated) + (Poc).Policy() + privacy.DecisionContext/privacy.Allow re-exports
provides:
  - Worker.Sync rebinds ctx with privacy.DecisionContext(ctx, privacy.Allow) as its first statement — the sole production bypass
  - Three behavioural locks on the bypass pattern (admits Users, does not leak to fresh ctx, inherited by child goroutines via context.WithCancel)
  - TestSyncBypass_SingleCallSite — source-scanning invariant that fails CI if a second production call site appears anywhere under internal/, cmd/, or ent/schema/
  - TestWorkerSync_HasBypassCall — structural lock pinning the bypass to worker.go Sync() before the running-CAS guard
affects: [59-06, 60, 61]

tech-stack:
  added: []
  patterns:
    - "Single-call-site bypass: one privacy.DecisionContext call at the sync-worker entry point, enforced by a source-scanning test"
    - "Comment-aware regex audit: strip //-line and /*…*/ block comments before matching so doc-comments referencing the pattern do not count"
    - "Structural tests on worker.go text: assert the VIS-05 bypass appears BEFORE the running-CAS line (location lock) and the ent/privacy import is present"

key-files:
  created:
    - internal/sync/bypass_audit_test.go (source-scanning audit per D-09 + RESEARCH Pitfall 4; 252 lines incl. comment-stripping helper)
  modified:
    - internal/sync/worker.go (add ent/privacy import; prepend `ctx = privacy.DecisionContext(ctx, privacy.Allow)` to Sync; extend Sync godoc with VIS-05 + D-08/D-09 rationale)
    - internal/sync/worker_test.go (add 4 tests: HasBypassCall, BypassesPrivacy, BypassDoesNotLeak, ChildGoroutineInheritsBypass; add ent/privacy import)

key-decisions:
  - "Bypass line kept to a single Go statement (not wrapped in a helper). Keeps the audit invariant trivially greppable and keeps Worker.Sync within the 100-line REFAC-03 budget (now at 100 exactly)."
  - "Structural lock (TestWorkerSync_HasBypassCall) is kept separate from the single-call-site audit (TestSyncBypass_SingleCallSite). The former pins the location inside worker.go; the latter pins the cardinality across the tree. A failure in either immediately identifies the regression category."
  - "Audit regex upgraded from the plan's literal `[^)]*` bracket class to `(?s)privacy\\.DecisionContext\\([^;]*?privacy\\.Allow\\b`. The original could not match calls with nested parens (e.g. `DecisionContext(context.Background(), privacy.Allow)`) — verified by scratch-file test; the upgraded regex catches those, handles multi-line gofmt-split calls via dotall, and refuses false-match on hypothetical identifiers like `privacy.AllowAll` via `\\b`."
  - "Audit strips //-line and /*…*/ block comments before matching. ent/schema/poc.go's godoc mentions the pattern in prose; a naive scan counts it as a second call site. The comment stripper respects string and rune literals so the exclusion is robust."
  - "Audit's 'wanted path' check uses filepath.Rel + ToSlash so the invariant is portable across OSes — any future cross-platform contributor gets the same result."

patterns-established:
  - "Pattern: the single-call-site invariant for security-sensitive APIs (ent privacy bypass). Generalises: stamp the decision once at the boundary, enforce 'exactly one call site' via a source-scanning test that strips comments and handles multi-line call syntax."
  - "Pattern: structural regression locks via os.ReadFile('worker.go') + strings.Index. Sibling of the existing TestWorkerSync_LineBudget — same technique applied to placement ordering."

requirements-completed: [VIS-05]

duration: ~25min
completed: 2026-04-17
---

# Phase 59 Plan 05: Sync-worker privacy bypass Summary

**Worker.Sync now rebinds ctx with `privacy.DecisionContext(ctx, privacy.Allow)` as its first statement (D-08/D-09 single call site), a source-scanning audit (`TestSyncBypass_SingleCallSite`) fails CI if a second production bypass appears anywhere under internal/, cmd/, or ent/schema/, and three behavioural tests lock the context-scoped semantics.**

## Performance

- **Duration:** ~25 min (single session, base-reset at start)
- **Started:** 2026-04-17T02:16:00Z (worktree base reset to c1ae5b2)
- **Completed:** 2026-04-17T02:41:14Z
- **Tasks:** 2 tasks, 3 commits (test/RED → feat/GREEN → test/audit)
- **Files modified:** 2 (worker.go, worker_test.go); 1 created (bypass_audit_test.go)

## Accomplishments

- **Sync worker bypass installed.** `Worker.Sync` rebinds ctx with `privacy.DecisionContext(ctx, privacy.Allow)` before the running-CAS guard, before any otel span Start, before any ent call. Every downstream read/write inherits allow-all; the ent rule-dispatch loop short-circuits at the stored decision before any Policy predicate runs. Worker.Sync body sits at 100 lines exactly — right at the REFAC-03 budget; the VIS-05 comment moved into the function's godoc (outside the body) to preserve it.
- **Single-call-site audit.** `TestSyncBypass_SingleCallSite` walks internal/, cmd/, ent/schema/ (skipping `*_test.go` and the generated ent/ subtree), strips Go comments, and asserts exactly ONE bypass call site — and that it lives in `internal/sync/worker.go`. Validated fails-loud by introducing a scratch second-site in `cmd/peeringdb-plus/scratch_bypass.go`; test failed with both call sites listed; removing the scratch restored the pass.
- **Structural regression lock.** `TestWorkerSync_HasBypassCall` reads worker.go, asserts the bypass line appears BEFORE `w.running.CompareAndSwap`, and asserts the `ent/privacy` import is present. A future edit that reorders the bypass after the CAS guard — or drops the import — fails CI.
- **Three behavioural tests.** `TestWorkerSync_BypassesPrivacy` (Users row admitted on bypass ctx), `TestWorkerSync_BypassDoesNotLeak` (fresh `context.Background()` does not inherit the decision), `TestWorkerSync_ChildGoroutineInheritsBypass` (derived ctx from `context.WithCancel(bypassCtx)` keeps allow-all — pins standard Go context-value propagation semantics, mirrors the runSyncCycle demotion-monitor pattern around worker.go:1209).

## Task Commits

1. **Task 1a (RED):** `d9fde4e` — `test(59-05): add failing test for sync-worker privacy bypass` — 4 tests added to worker_test.go. `TestWorkerSync_HasBypassCall` initially fires (no bypass in worker.go yet); the other 3 pass as they exercise the pattern independently from worker.go.
2. **Task 1b (GREEN):** `7155ce8` — `feat(59-05): install sync-worker privacy bypass (VIS-05)` — one-line bypass + import in worker.go; extended Sync godoc with VIS-05/D-08/D-09 rationale.
3. **Task 2 (audit):** `ce325dd` — `test(59-05): add single-call-site audit (T-59-02 mitigation)` — bypass_audit_test.go with comment-aware regex and fails-loud validation.

## Files Created/Modified

- **internal/sync/worker.go** — added `"github.com/dotwaffle/peeringdb-plus/ent/privacy"` import (line 22); Sync godoc extended with the VIS-05 D-08/D-09 rationale and single-call-site warning; first statement of Sync is now `ctx = privacy.DecisionContext(ctx, privacy.Allow)` with a trailing comment pointing at the audit test.
- **internal/sync/worker_test.go** — 4 new tests (HasBypassCall, BypassesPrivacy, BypassDoesNotLeak, ChildGoroutineInheritsBypass); added `"github.com/dotwaffle/peeringdb-plus/ent/privacy"` import.
- **internal/sync/bypass_audit_test.go** — new. Package `sync`. Walks internal/, cmd/, ent/schema/; strips Go comments; scans with comment-aware, paren-nesting-aware, multi-line-tolerant regex; fails loud with all offending file:line locations when the invariant is violated.

## Decisions Made

- **Regex upgraded from plan literal.** The plan's `privacy\.DecisionContext\([^)]*privacy\.Allow[^)]*\)` uses `[^)]*` which refuses to cross `)` — it fails to match any call with nested parens like `DecisionContext(context.Background(), privacy.Allow)`. Verified empirically with a scratch second-call-site that the plan's regex did NOT detect. Upgraded to `(?s)privacy\.DecisionContext\([^;]*?privacy\.Allow\b` which handles nested parens (forbids `;` instead of `)`), multi-line gofmt-split calls (`(?s)` dotall), and rejects identifiers that merely start with "Allow" (`\b` word boundary). This is a Rule 1 auto-fix: the plan-prescribed regex would have let a silent-bypass regression through. Documented in the commit message and in the regex comment inside bypass_audit_test.go.
- **Comment stripping is mandatory.** `ent/schema/poc.go`'s godoc already contains the string `privacy.DecisionContext(ctx, privacy.Allow)` (in prose, line 124); the godoc I added to worker.go's Sync also mentions it (line 246). Without a comment-strip pass, any raw grep would count these as additional call sites. Implemented `stripGoComments` as a byte-level scanner that respects string and rune literals — simpler than pulling in go/parser for this one-off test, and avoids adding a generated-code dependency on the test.
- **Two structural tests, not one.** `TestWorkerSync_HasBypassCall` (location) and `TestSyncBypass_SingleCallSite` (cardinality) could be merged into one super-test. Kept separate because the failure messages are more actionable — a future regression either "moved the bypass below the CAS" or "added a second call site somewhere", and the two tests name exactly that.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Regex in the plan failed to match nested-paren call syntax**
- **Found during:** Task 2 (audit test), during fails-loud validation
- **Issue:** Plan's regex `privacy\.DecisionContext\([^)]*privacy\.Allow[^)]*\)` uses `[^)]*` (no close-paren). That pattern refuses to match `privacy.DecisionContext(context.Background(), privacy.Allow)` because the first `)` in `context.Background()` terminates the character class early. I verified by adding a scratch file containing that shape; the plan regex did NOT detect it (test still reported "1 match"). Any future developer copy-pasting the worker.go pattern but inlining `context.Background()` would slip through the audit — the opposite of what the test is meant to catch.
- **Fix:** Replaced with `(?s)privacy\.DecisionContext\([^;]*?privacy\.Allow\b`:
  - `(?s)` dotall — handles multi-line gofmt splits of the call across lines (belt-and-braces; gofmt keeps the short call on one line, but a deliberately-obfuscated multi-line form would otherwise evade the audit)
  - `[^;]*?` non-greedy, statement-bounded — allows nested parens but forbids crossing statement boundaries, so we can't false-match two unrelated `DecisionContext` + `privacy.Allow` tokens on the same line
  - `\b` word boundary — refuses to match a hypothetical future `privacy.AllowAll` identifier
- **Files modified:** internal/sync/bypass_audit_test.go (regex literal + full whole-file scan instead of per-line split)
- **Verification:** Re-introduced scratch call site in cmd/peeringdb-plus/scratch_bypass.go using `DecisionContext(context.Background(), privacy.Allow)`; test failed with both worker.go:260 AND scratch_bypass.go:10 listed. Removed scratch; test passed again.
- **Committed in:** ce325dd (Task 2 test commit, entire audit landing together)

**2. [Rule 2 - Missing Critical] VIS-05 godoc moved out of function body to respect REFAC-03 line budget**
- **Found during:** Task 1b (GREEN phase), line-counting during edit
- **Issue:** Worker.Sync body was already at 98 lines (budget 100 per TestWorkerSync_LineBudget). The plan's prescribed VIS-05 comment block inside the function body (≈13 lines) would have pushed Sync to ~111 lines and failed the line-budget test — creating a spurious GREEN→RED→REFACTOR cycle.
- **Fix:** Moved the VIS-05 rationale into the function's pre-existing godoc block (outside the braces, outside the line counter). Kept a terse inline comment on the bypass line itself: `// VIS-05 bypass — sole call site (D-08/D-09)`. Sync body now sits at 100 lines exactly — at the budget, but within it.
- **Files modified:** internal/sync/worker.go
- **Verification:** TestWorkerSync_LineBudget reports "Worker.Sync body: 100 lines (budget: 100)" — within budget.
- **Committed in:** 7155ce8 (Task 1b feat commit)

---

**Total deviations:** 2 auto-fixed (1 bug in plan regex that would have silently failed the invariant, 1 correctness-driven re-layout of comment placement to honour a pre-existing structural invariant)
**Impact on plan:** Both auto-fixes are essential. Deviation 1 is load-bearing — the plan's regex literally cannot enforce the invariant against realistic call syntax. Deviation 2 is correctness-at-seams — the plan didn't account for the REFAC-03 line budget, which is enforced by an existing test. No scope creep; every acceptance criterion in both tasks is satisfied.

## Issues Encountered

- Worktree started at base `3f4c8ad` (wave-4-predecessor) instead of the expected `c1ae5b2`. Hard-reset to `c1ae5b2` per executor worktree_branch_check protocol before starting — resolved cleanly. All Plan 04 artefacts (ent/privacy/, poc.Policy(), ent/schematypes/) were present after reset.
- `.planning/phases/` directory initially missing from the worktree. After the base-reset it materialised. No impact.

## Verification Evidence

- `go test -race ./internal/sync/... -run 'TestWorkerSync_Bypass|TestSyncBypass|TestWorkerSync_HasBypassCall|TestWorkerSync_ChildGoroutine'` — 5 tests green (HasBypassCall, BypassesPrivacy, BypassDoesNotLeak, ChildGoroutineInheritsBypass, SingleCallSite).
- `go test -race -count=1 ./internal/sync/...` — whole sync suite green including the pre-existing TestWorkerSync_LineBudget which reports "body: 100 lines (budget: 100)".
- `go vet ./...` — clean.
- `golangci-lint run ./internal/sync/...` — 0 issues. (Six pre-existing issues remain under `internal/visbaseline/` and `graph/custom.resolvers.go` from earlier phases — out of scope per the executor deviation-rule SCOPE BOUNDARY.)
- `grep -n "privacy.DecisionContext(ctx, privacy.Allow)" internal/sync/worker.go` — 2 matches (one in godoc prose on line 246, one live call on line 260). TestSyncBypass_SingleCallSite correctly filters the godoc via stripGoComments; raw grep does not.
- `grep -nA2 "VIS-05 bypass" internal/sync/worker.go` — shows the bypass on line 260, immediately followed by `w.running.CompareAndSwap` on line 261 (confirms BEFORE-CAS placement).
- Manual fails-loud smoke: introduced `cmd/peeringdb-plus/scratch_bypass.go` containing `privacy.DecisionContext(context.Background(), privacy.Allow)`; TestSyncBypass_SingleCallSite failed with both worker.go:260 and scratch_bypass.go:10 listed. Removed the scratch file; test passed.

## TDD Gate Compliance

- **RED commit (d9fde4e):** `TestWorkerSync_HasBypassCall` correctly fires at the structural invariant before worker.go is modified — the worker.go text lacks the bypass line, so the structural scan fails. The three pattern-tests (BypassesPrivacy, BypassDoesNotLeak, ChildGoroutineInheritsBypass) pass even at RED because they exercise the `privacy.DecisionContext(ctx, privacy.Allow)` shape directly against the ent client; they do not drive Sync(). This is intentional per the plan — they are contract tests on the bypass pattern, orthogonal to worker.go's call site. The structural test (HasBypassCall) is the RED gate.
- **GREEN commit (7155ce8):** worker.go adds the bypass + import + extended godoc. HasBypassCall now passes, as do all four tests.
- **Task 2 test commit (ce325dd):** additive audit test — not a RED/GREEN cycle on its own; it's a post-hoc invariant lock that would already pass once Task 1b landed.

Gate sequence in `git log` is: `test(59-05)` → `feat(59-05)` → `test(59-05)`. The audit-test commit at the end is additive to the invariant surface, not a repeat of the RED/GREEN cycle.

## User Setup Required

None — no external service or env-var configuration. The bypass is context-stamped in-process; no deployment change needed.

## Next Phase Readiness

- **59-06 ready.** The sole bypass at worker.go is in place; 59-06 (integration test seeding Users-tier rows through the real worker + making an anonymous HTTP request) can now assert the policy denies the row at the surface without touching the bypass mechanism. The `seedPocWithVisible` pattern in `internal/sync/policy_test.go` (which already uses `privacy.DecisionContext`) is the reference shape for 59-06's seed step.
- **60-surface-integration-tests ready.** The audit invariant is in place; any surface-level test that accidentally stamps the bypass into a request ctx will fail CI at build time.
- **Note for future edits to worker.go:** Sync body is at 100 lines exactly, at the REFAC-03 budget ceiling. Any additional inline code inside Sync must be offset by removing an existing line, or by extracting a helper (see `checkMemoryLimit` for the established extraction pattern).

## Self-Check: PASSED

- [x] internal/sync/worker.go exists and contains `privacy.DecisionContext(ctx, privacy.Allow)` at Sync entry
- [x] internal/sync/bypass_audit_test.go exists and compiles
- [x] internal/sync/worker_test.go contains TestWorkerSync_HasBypassCall, TestWorkerSync_BypassesPrivacy, TestWorkerSync_BypassDoesNotLeak, TestWorkerSync_ChildGoroutineInheritsBypass
- [x] Commit d9fde4e exists in git log (RED — 4 tests)
- [x] Commit 7155ce8 exists in git log (GREEN — worker.go bypass + import)
- [x] Commit ce325dd exists in git log (audit test)
- [x] `go build ./...` passes
- [x] `go vet ./...` passes
- [x] `go test -race ./internal/sync/... -run 'TestWorkerSync_Bypass|TestSyncBypass|TestWorkerSync_HasBypassCall|TestWorkerSync_Child'` — 5 tests green
- [x] `go test -race ./internal/sync/...` — full sync suite green (including TestWorkerSync_LineBudget at 100/100)
- [x] `golangci-lint run ./internal/sync/...` — 0 issues

---
*Phase: 59-ent-privacy-policy-sync-bypass*
*Completed: 2026-04-17*
