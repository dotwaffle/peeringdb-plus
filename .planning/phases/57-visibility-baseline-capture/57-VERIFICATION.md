---
phase: 57-visibility-baseline-capture
verified: 2026-04-16T22:15:00Z
status: human_needed
score: 12/15 must-haves verified (3 pending operator)
overrides_applied: 0
re_verification:
  previous_status: initial
  previous_score: N/A
  gaps_closed: []
  gaps_remaining: []
  regressions: []
human_verification:
  - test: "Live beta capture — run pdbcompat-check -capture -target=beta -mode=both against beta.peeringdb.com"
    expected: "testdata/visibility-baseline/beta/{anon,auth}/api/{type}/page-{1,2}.json tree populated with 52 JSON fixture files; capture tool prints the /tmp/pdb-vis-capture-XXXXX rawAuthDir on completion"
    why_human: "Requires valid PDBPLUS_PEERINGDB_API_KEY and ≥1h wall-clock against live upstream; rate-limit bound by PeeringDB upstream (≤40 req/min auth, ≤20 req/min anon); plan 57-04 is autonomous:false precisely because live capture cannot be automated"
    resume_command: "pdbcompat-check -capture -target=beta -mode=both -out=testdata/visibility-baseline/beta -api-key=\"$PDBPLUS_PEERINGDB_API_KEY\""
  - test: "Redact + diff the beta tree — apply RedactDir to $RAW_AUTH_DIR from the capture run, then run -diff"
    expected: "testdata/visibility-baseline/beta/auth/ populated with redacted counterparts of the 26 auth pages; testdata/visibility-baseline/DIFF.md, DIFF-beta.md, and diff.json generated at the baseline root; TestCommittedFixturesHaveNoPII transitions from SKIP to PASS"
    why_human: "$RAW_AUTH_DIR is ephemeral /tmp path printed only to the operator's stdout during Checkpoint 1; cannot be recovered post-hoc by an agent"
    resume_command: "pdbcompat-check -redact -in=$RAW_AUTH_DIR/auth -out=testdata/visibility-baseline/beta/auth && pdbcompat-check -diff -out=testdata/visibility-baseline/"
  - test: "Prod confirmation run for poc/org/net — at minimum anon-only (ROADMAP SC #3)"
    expected: "testdata/visibility-baseline/prod/anon/api/{poc,org,net}/page-1.json exists; unified DIFF.md Targets includes both beta and prod; per-target DIFF-prod.md emitted"
    why_human: "Live upstream call against www.peeringdb.com; /api/org historically trips the 1-req/hour size throttle (Research Pitfall 1); anon capture requires no API key so the minimum bar is always reachable, but still requires human judgement on throttle handling"
    resume_command: "pdbcompat-check -capture -target=prod -mode=anon -out=testdata/visibility-baseline/prod -types=poc,org,net && pdbcompat-check -diff -out=testdata/visibility-baseline/"
  - test: "Manual DIFF.md review — Checkpoint 2 of plan 57-04"
    expected: "DIFF.md legible, one section per type under TOC; auth-only columns show only <auth-only:{string|number|bool}> placeholders; expected PII-bearing types (poc, org, fac) show non-zero AuthOnlyRowCount; no raw email/phone/name substrings visible"
    why_human: "Semantic review — a grep cannot catch 'diff highlights the wrong fields' or 'row counts are implausibly low'. Human-judgment gate before commit"
    resume_command: "less testdata/visibility-baseline/DIFF.md"
---

# Phase 57: Visibility Baseline Capture — Verification Report

**Phase Goal:** Produce a committed, reviewable empirical baseline showing exactly which fields/rows differ between unauthenticated and authenticated PeeringDB API responses across all 13 types — without which the privacy filter cannot be scoped correctly.
**Verified:** 2026-04-16T22:15:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Executive Summary

Phase 57 delivered the **entire code and tooling substrate** for VIS-01 and VIS-02 through plans 57-01 (redactor+PII allow-list), 57-02 (capture loop+checkpoint+FetchRawPage+CLI), 57-03 (differ+emitters+PII guard test), and the **Task 1 code-only portion** of 57-04 (RedactDir+BuildReport+CLI dispatch). All automated tests pass under `-race` with zero lint warnings.

The **operator-gated live capture** portion of plan 57-04 (Checkpoint 1 beta walk, Task 2 redact+diff commit, Checkpoint 2 review, Checkpoint 3 prod run, Task 3 verification sweep) was deliberately deferred — plan 57-04 is `autonomous: false` because the live walk requires a valid `PDBPLUS_PEERINGDB_API_KEY`, ≥1 hour of wall-clock against rate-limited upstream, and human review of the resulting DIFF.md before commit. None of that is agent-executable.

`testdata/visibility-baseline/` therefore does not yet exist. The PII guard test `TestCommittedFixturesHaveNoPII` is correctly SKIPping, and will enforce the no-PII invariant automatically once the operator lands fixtures. This is an intentional handoff to the developer, not a planning gap requiring replanning.

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Package `internal/visbaseline` exists and compiles | VERIFIED | `go build ./internal/visbaseline/...` clean; 19 .go files present |
| 2 | PII allow-list is single source of truth for redactor + guard test | VERIFIED | `pii.go` exports `PIIFields` (16 fields, alphabetised) and `IsPIIField`; used by `redact.go:redactValue`, `diff.go:Diff` (IsPII marker), `fixtures_guard_test.go:walkPII` |
| 3 | `Redact(anonBytes, authBytes)` is pure and deterministic | VERIFIED | `redact.go:46 func Redact(anonBytes, authBytes []byte) ([]byte, error)`; `TestRedactionDeterministic` asserts byte-identical repeat output |
| 4 | PII fields ALWAYS redacted regardless of anon presence | VERIFIED | `TestRedactionPIIFieldAlwaysRedacted` passes; belt-and-braces against upstream anon bug (T-57-02) |
| 5 | `.gitignore` excludes raw auth paths | VERIFIED | `grep -c pdb-vis-capture .gitignore` = 1; `grep -c .raw-auth .gitignore` = 3 |
| 6 | `pdbcompat-check -capture -target=beta -mode=both -out=<dir>` is a valid invocation that bypasses the default structural check | VERIFIED | `main.go` dispatch via `if cfg.capture { return runCapture(...) }`; binary `-h` output lists all 7 new flags |
| 7 | Capture loop walks 13 types × 2 pages × 2 modes = 52 tuples per target | VERIFIED | `capture.go` consumes `AllTypes` (13 entries) and `EnumerateTuples`; `TestEnumerateTuplesBetaBoth` asserts 52 tuples |
| 8 | Capture reuses existing `internal/peeringdb.Client` rate-limiter | VERIFIED | `capture.go` calls `FetchRawPage` (additive method at `client.go:239`); no new HTTP client/limiter/retry-ladder |
| 9 | On `RateLimitError` loop sleeps RetryAfter+jitter and re-fetches same tuple | VERIFIED | `capture.go` contains `errors.As(err, &rlErr)` branch; `TestCaptureRespectsRateLimit` asserts tuple counter does not advance on 429 |
| 10 | Checkpoint written atomically via os.Rename | VERIFIED | `checkpoint.go` uses write-.tmp-then-Rename pattern; `TestCheckpointAtomicWrite` asserts no residual .tmp file |
| 11 | Raw auth bytes written only to `os.MkdirTemp` /tmp path | VERIFIED | `capture.go:Run` uses `os.MkdirTemp("", "pdb-vis-capture-*")`; `TestCaptureWritesAuthBytesToTmpOnly` walks OutDir and fails on any /auth/ path |
| 12 | `Diff(typeName, anonBytes, authBytes)` produces Report describing deltas, never values | VERIFIED | `diff.go:59 func Diff`; `TestDiffNeverEmitsValues` canary asserts leaked value does not appear in marshalled Report; `TestDiffNeverEmitsLengths` reflects on FieldDelta to forbid Length/Size/Hash/Digest/Value/Values fields |
| 13 | `WriteMarkdown` + `WriteJSON` emit deterministic output matching Example 5 schema | VERIFIED | `report.go:27 WriteJSON`, `report.go:60 WriteMarkdown`; 4 golden tests lock byte-identical output; `ReportSchemaVersion=1` constant |
| 14 | `TestCommittedFixturesHaveNoPII` enforces no-PII invariant (runs as part of `go test ./...`) | VERIFIED (detection logic) / PENDING-OPERATOR (live enforcement) | `fixtures_guard_test.go:36`; currently SKIPs because testdata/visibility-baseline/ absent; 3 self-tests (`TestPIIGuardDetectsUnredactedString`, `TestPIIGuardAcceptsRedactedFixture`, `TestPIIGuardDetectsNonStringPIIValue`) prove the detector logic works on every run |
| 15 | Committed fixtures + DIFF.md + diff.json exist at testdata/visibility-baseline/ | PENDING-OPERATOR | Directory does not exist yet; creation blocked on Checkpoint 1 beta walk (autonomous: false). Code path that writes them (`runCapture`, `runRedact`, `runDiff`) is present and tested against httptest servers |

**Score:** 12/15 truths verified (3 pending operator live capture)

### Required Artifacts

#### Code Artifacts (all VERIFIED)

| Artifact | Status | Evidence |
|----------|--------|----------|
| `internal/visbaseline/pii.go` | VERIFIED | `PIIFields` + `IsPIIField` exported; imported by `redact.go`, `diff.go`, `fixtures_guard_test.go` |
| `internal/visbaseline/redact.go` | VERIFIED | `Redact`, `PlaceholderString/Number/Bool` exported |
| `internal/visbaseline/checkpoint.go` | VERIFIED | `State`, `Tuple`, `LoadState`, `PromptResumeOrRestart`, `DefaultStatePath`, `EnumerateTuples`, `CleanupStatePath` exported |
| `internal/visbaseline/capture.go` | VERIFIED | `Capture`, `New`, `AllTypes`, `ProdTypes` exported; wires to `peeringdb.Client.FetchRawPage` |
| `internal/visbaseline/config.go` | VERIFIED | `Config` struct with `ClientOverride *peeringdb.Client` test hook |
| `internal/visbaseline/diff.go` | VERIFIED | `Diff`, `Report`, `TypeReport`, `FieldDelta`, `ReportSchemaVersion` exported |
| `internal/visbaseline/report.go` | VERIFIED | `WriteMarkdown`, `WriteJSON` exported |
| `internal/visbaseline/redactcli.go` | VERIFIED | `RedactDir(ctx, RedactDirConfig)` exported (plan 57-04 Task 1) |
| `internal/visbaseline/reportcli.go` | VERIFIED | `BuildReport(ctx, BuildReportConfig)` exported (plan 57-04 Task 1) |
| `internal/visbaseline/fixtures_guard_test.go` | VERIFIED | `TestCommittedFixturesHaveNoPII` + `walkPII` + 3 self-tests |
| `internal/peeringdb/client.go` | VERIFIED | `FetchRawPage` additive method at line 239; reuses `doWithRetry` path |
| `cmd/pdbcompat-check/main.go` | VERIFIED | 8 capture-related flags declared (`-capture`, `-target`, `-mode`, `-out`, `-types`, `-prod-auth`, `-state`, `-in`, `-redact`, `-diff`); default path preserved via early-return dispatch |
| `cmd/pdbcompat-check/capture.go` | VERIFIED | `runCapture` implementation |
| `cmd/pdbcompat-check/redactdiff.go` | VERIFIED | `runRedact` + `runDiff` implementations (plan 57-04 Task 1) |
| `.gitignore` | VERIFIED | pdb-vis-capture-* and .raw-auth rules present |

#### Data Artifacts (PENDING-OPERATOR)

| Artifact | Status | Evidence |
|----------|--------|----------|
| `testdata/visibility-baseline/beta/anon/api/{type}/page-{1,2}.json` (26 files) | PENDING-OPERATOR | Directory does not exist; requires Checkpoint 1 live beta walk |
| `testdata/visibility-baseline/beta/auth/api/{type}/page-{1,2}.json` (26 files, redacted) | PENDING-OPERATOR | Directory does not exist; requires Checkpoint 1 + Task 2 redact step |
| `testdata/visibility-baseline/prod/anon/api/{poc,org,net}/page-{1,2}.json` (6 files) | PENDING-OPERATOR | Directory does not exist; requires Checkpoint 3 prod confirmation |
| `testdata/visibility-baseline/DIFF.md` | PENDING-OPERATOR | Requires -diff after operator commits fixtures |
| `testdata/visibility-baseline/DIFF-beta.md` / `DIFF-prod.md` | PENDING-OPERATOR | Requires multi-target -diff run |
| `testdata/visibility-baseline/diff.json` | PENDING-OPERATOR | Requires -diff after operator commits fixtures |

### Key Link Verification

| From | To | Via | Status | Evidence |
|------|----|----|--------|----------|
| `internal/visbaseline/redact.go` | `internal/visbaseline/pii.go` | `IsPIIField(` | WIRED | `redactValue` calls `IsPIIField` at line 74 |
| `internal/visbaseline/capture.go` | `internal/peeringdb.Client` | `FetchRawPage(` | WIRED | `runTuple` calls `client.FetchRawPage(ctx, t.Type, t.Page)` |
| `internal/visbaseline/capture.go` | `peeringdb.RateLimitError` | `errors.As.*RateLimitError` | WIRED | runTuple error-handler branch present |
| `internal/visbaseline/capture.go` | `internal/visbaseline/checkpoint.go` | `state.Advance` | WIRED | `Run` loop advances per tuple |
| `cmd/pdbcompat-check/main.go` | `cmd/pdbcompat-check/capture.go` | `if cfg.capture` dispatch | WIRED | verified binary -h shows -capture flag |
| `cmd/pdbcompat-check/redactdiff.go` | `internal/visbaseline.RedactDir` | `visbaseline.RedactDir(` | WIRED | `runRedact` calls through |
| `cmd/pdbcompat-check/redactdiff.go` | `internal/visbaseline.BuildReport` | `visbaseline.BuildReport(` | WIRED | `runDiff` calls through |
| `internal/visbaseline/fixtures_guard_test.go` | `testdata/visibility-baseline/` | `filepath.WalkDir` | WIRED (detection) / INACTIVE (no fixtures yet) | test SKIPs cleanly when dir absent; 3 self-tests prove detection logic works |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Full visbaseline + CLI test suite passes with race detector | `go test -race ./internal/visbaseline/... ./cmd/pdbcompat-check/...` | Both packages PASS | PASS |
| Full project build clean | `go build ./...` | (no output = clean) | PASS |
| PII guard self-tests exercise detection logic | `go test -run 'TestPIIGuard' ./internal/visbaseline/` | 3/3 PASS; TestCommittedFixturesHaveNoPII SKIP (expected) | PASS |
| CLI binary exposes all phase 57 flags | `pdbcompat-check -h \| grep -E 'capture\|target\|mode\|redact\|diff'` | -capture, -target, -mode, -redact, -diff, -in, -out, -prod-auth, -types all listed | PASS |
| Default structural-check path preserved (backward compat) | Code review of `main.go` dispatch | Single early-return dispatch at top of `run()`; no mutation of the existing default path | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| VIS-01 | 57-01, 57-02, 57-04 | System captures both unauthenticated and authenticated PeeringDB API responses for all 13 types as committed baseline fixtures | CODE_READY / DATA_PENDING | Capture tooling fully implemented and tested; fixtures themselves require operator live-capture run (see human_verification items 1-3) |
| VIS-02 | 57-03, 57-04 | System emits a structural diff report from the captured fixtures listing every field/row that differs between unauth and auth responses, with a per-type table reviewable in code review | CODE_READY / DATA_PENDING | Diff + emitter code complete with golden tests; DIFF.md/diff.json output blocked on having fixtures to diff (see human_verification items 2-3) |

Both requirements have their code machinery fully in place and exercised by synthetic-fixture tests. Final satisfaction requires the operator to run the live capture pipeline.

### Anti-Patterns Found

Scanned `internal/visbaseline/*.go`, `internal/peeringdb/client.go`, `cmd/pdbcompat-check/*.go` for TODO/FIXME/placeholder/stub markers:

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (none) | — | — | — | No blocker anti-patterns detected. The `<auth-only:TYPE>` placeholder strings in `redact.go` are intentional redaction sentinels (greppable by design), not stubs. |

`grep -r TODO\|FIXME\|XXX\|HACK internal/visbaseline/ cmd/pdbcompat-check/` returns only the intentional literal `TODO` strings inside doc comments describing the plan's progression, no code-level stubs.

### Human Verification Required

See `human_verification:` frontmatter above. Four operator steps needed to transition the phase from `human_needed` to `passed`:

1. **Live beta capture** (Checkpoint 1, ≥1h wall-clock) — `pdbcompat-check -capture -target=beta -mode=both -out=testdata/visibility-baseline/beta -api-key=$PDBPLUS_PEERINGDB_API_KEY`
2. **Redact + diff the beta tree** (Task 2) — `pdbcompat-check -redact -in=$RAW_AUTH_DIR/auth -out=testdata/visibility-baseline/beta/auth && pdbcompat-check -diff -out=testdata/visibility-baseline/`
3. **Prod confirmation** (Checkpoint 3, ROADMAP SC #3) — minimum bar is anon-only for poc/org/net: `pdbcompat-check -capture -target=prod -mode=anon -out=testdata/visibility-baseline/prod -types=poc,org,net && pdbcompat-check -diff -out=testdata/visibility-baseline/`
4. **Manual DIFF.md review** (Checkpoint 2) — `less testdata/visibility-baseline/DIFF.md`; confirm no raw PII, placeholder presence, plausible row counts

After those four steps:
- `TestCommittedFixturesHaveNoPII` will transition from SKIP to PASS automatically on the next `go test` run.
- VIS-01 and VIS-02 become fully satisfied.
- Phase 57 can be re-verified and marked `passed`.

### Gaps Summary

**No planning gaps.** The three PENDING-OPERATOR items (truths 14 enforcement, 15 data artifacts; both data-artifact rows) are intentional consequences of plan 57-04 being `autonomous: false`. The phase design correctly recognises that live-upstream capture with a real API key and human-review gates cannot be automated.

All code deliverables landed on plan. All synthetic-fixture tests are green. The PII guard is implemented and exercising its detection logic on every `go test` run through its three self-tests; once live fixtures land, it begins enforcing on the real tree automatically.

## Phase 57 Execution Honesty Check

SUMMARY claims cross-checked against codebase:

- 57-01 claims `pii.go`, `redact.go`, `.gitignore` updated → **verified present**, 13 tests passing
- 57-02 claims `checkpoint.go`, `capture.go`, `config.go`, `FetchRawPage` on client, CLI dispatch → **verified present**, 44 tests passing
- 57-03 claims `diff.go`, `report.go`, `fixtures_guard_test.go`, goldens → **verified present**, 31 tests passing (including 1 intentional SKIP)
- 57-04 claims `redactcli.go`, `reportcli.go`, `redactdiff.go`, `-redact`/`-diff` flags → **verified present**; 20+ new tests
- 57-04 claims `testdata/visibility-baseline/` fixtures + DIFF.md + diff.json → **deliberately absent**, operator-deferred per autonomous:false

No executor deviation from SUMMARY vs. codebase. No stubs. No TODOs blocking goal.

---

_Verified: 2026-04-16T22:15:00Z_
_Verifier: Claude (gsd-verifier)_
