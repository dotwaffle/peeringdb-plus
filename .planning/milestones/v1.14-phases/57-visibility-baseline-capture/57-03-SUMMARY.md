---
phase: 57-visibility-baseline-capture
plan: 03
subsystem: visibility
tags: [visibility, diff, report, pii-guard, golden-tests]

# Dependency graph
requires:
  - phase: 57-visibility-baseline-capture
    provides: PIIFields allow-list, IsPIIField, Placeholder constants, envelope redaction (plan 01)
provides:
  - Pure-function Diff(typeName, anonBytes, authBytes) emitting a per-type TypeReport
  - Report / TypeReport / FieldDelta types carrying counts + field names only (no raw values)
  - WriteMarkdown emitter producing TOC + one table per PeeringDB type
  - WriteJSON emitter producing the 57-RESEARCH.md Example 5 schema with greppable placeholders
  - TestCommittedFixturesHaveNoPII canary guarding every future commit of auth fixtures
affects: [57-04, 58, 60]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Structural differ narrower than conformance.CompareStructure: FIXED envelope, tracks row counts + visible enum set"
    - "Pure function emitters with injected GeneratedAt for deterministic golden tests"
    - "testing.TB -> errorReporter interface decoupling so self-tests can drive walkPII with a recorder"
    - "Golden fixture regeneration via -update-goldens flag gated behind t.Skip"

key-files:
  created:
    - internal/visbaseline/diff.go
    - internal/visbaseline/diff_test.go
    - internal/visbaseline/report.go
    - internal/visbaseline/report_test.go
    - internal/visbaseline/report_golden_gen_test.go
    - internal/visbaseline/fixtures_guard_test.go
    - internal/visbaseline/testdata/diff_golden/anon_identical.json
    - internal/visbaseline/testdata/diff_golden/auth_identical.json
    - internal/visbaseline/testdata/diff_golden/anon_simple.json
    - internal/visbaseline/testdata/diff_golden/auth_simple.json
    - internal/visbaseline/testdata/diff_golden/anon_rowdrift.json
    - internal/visbaseline/testdata/diff_golden/auth_rowdrift.json
    - internal/visbaseline/testdata/diff_golden/expected_empty.md
    - internal/visbaseline/testdata/diff_golden/expected_empty.json
    - internal/visbaseline/testdata/diff_golden/expected_simple.md
    - internal/visbaseline/testdata/diff_golden/expected_simple.json
  modified: []

key-decisions:
  - "FieldDelta struct carries Name/AuthOnly/Placeholder/RowsAdded/ValueSetDrift/IsPII only — no Length, Size, Hash, Digest, Value, or Values fields (T-57-02 mitigation)"
  - "WriteJSON uses json.Encoder with SetEscapeHTML(false) so placeholder sentinels like <auth-only:string> remain greppable in diff.json"
  - "WriteJSON normalises nil Fields slices to []FieldDelta{} so `fields` always serialises as [] rather than null"
  - "Markdown layout: TOC + 13 small per-type tables with a stable column set (Field, Auth-only, Placeholder, Rows added, PII?, Notes)"
  - "Diff validates BOTH anon and auth envelopes carry a top-level data key, returning wrapped errors naming the failing side — forces TestDiffMissingDataKeyReturnsErrorAuth to actually exercise the auth-side probe"
  - "walkPII takes a minimal errorReporter interface (Errorf + Helper) rather than testing.TB, so the Go 1.26 TB.Context addition does not block self-tests"

patterns-established:
  - "Pure-function emitter pattern: WriteMarkdown(io.Writer, Report) and WriteJSON(io.Writer, Report) with deterministic output for golden comparison"
  - "Canary guard test pattern: scan repo-local testdata with filepath.WalkDir, skip gracefully when absent, fail loudly on violation"

requirements-completed: [VIS-02]

# Metrics
duration: 10min
completed: 2026-04-16
---

# Phase 57 Plan 03: Visibility diff + emitters + PII guard Summary

**Structural differ with Report/TypeReport/FieldDelta types, Markdown+JSON emitters with golden fixtures, and a committed-fixture PII canary — all pure-function, zero live-network dependencies, deterministic output.**

## Performance

- **Duration:** ~10 min
- **Started:** 2026-04-16T21:10:17Z
- **Completed:** 2026-04-16T21:20:03Z
- **Tasks:** 3/3
- **Files modified:** 16 (all newly created under `internal/visbaseline/` and `internal/visbaseline/testdata/diff_golden/`)

## Accomplishments

- Diff is a pure function over FIXED envelope structure `{"meta":..., "data":[...rows...]}` — parses both sides, indexes anon rows by id, accumulates per-field RowsAdded counts, extracts visible enum set, produces deterministic sorted output.
- Report / TypeReport / FieldDelta types carry ONLY counts + field names + placeholder sentinels + IsPII markers — explicitly forbid Length, Size, Hash, Digest, Value, Values fields (enforced by `TestDiffNeverEmitsLengths`).
- WriteJSON + WriteMarkdown produce byte-stable output on identical inputs; `<auth-only:string>` placeholder survives emission with SetEscapeHTML(false) so downstream CI greps work.
- PII canary test (TestCommittedFixturesHaveNoPII) walks committed auth fixtures, skips when absent, fails loudly on any non-placeholder PII field value.

## Task Commits

1. **Task 1: Differ + Report type + golden fixtures** — `f8e4d3c` (feat)
   - `internal/visbaseline/diff.go`, `diff_test.go`, 6 synthetic JSON fixtures under `testdata/diff_golden/`
   - 14 test functions (TestDiff*)

2. **Task 2: Markdown + JSON emitters with golden output tests** — `b98bb54` (feat)
   - `internal/visbaseline/report.go`, `report_test.go`, `report_golden_gen_test.go`, 4 golden expected_* files
   - 13 test functions (TestWrite*, TestReportConsistency) + TestUpdateGoldens helper

3. **Task 3: TestCommittedFixturesHaveNoPII guard test** — `d368dfc` (feat)
   - `internal/visbaseline/fixtures_guard_test.go`
   - TestCommittedFixturesHaveNoPII (canary, skips cleanly when fixtures absent)
   - TestPIIGuardDetectsUnredactedString (self-test: detector flags unredacted string)
   - TestPIIGuardAcceptsRedactedFixture (self-test: detector accepts placeholder/null)
   - TestPIIGuardDetectsNonStringPIIValue (self-test: detector flags numeric lat/long)

## Files Created/Modified

**Production code (2 files):**
- `internal/visbaseline/diff.go` — Diff(), Report/TypeReport/FieldDelta types, ReportSchemaVersion=1, requireDataKey probe, extractVisibleSet, stringSliceEqual
- `internal/visbaseline/report.go` — WriteMarkdown(w, rep), WriteJSON(w, rep), bracketList helper

**Test code (4 files):**
- `internal/visbaseline/diff_test.go` — 14 Diff tests
- `internal/visbaseline/report_test.go` — 13 emitter tests incl. 4 golden tests
- `internal/visbaseline/report_golden_gen_test.go` — TestUpdateGoldens regeneration helper behind -update-goldens flag
- `internal/visbaseline/fixtures_guard_test.go` — TestCommittedFixturesHaveNoPII canary + 3 self-tests + walkPII walker

**Golden fixtures (10 files under `internal/visbaseline/testdata/diff_golden/`):**
- Inputs: `anon_identical.json`, `auth_identical.json`, `anon_simple.json`, `auth_simple.json`, `anon_rowdrift.json`, `auth_rowdrift.json`
- Expected outputs: `expected_empty.md`, `expected_empty.json`, `expected_simple.md`, `expected_simple.json`

All fixtures are synthetic (example.com/example.org domains, fake names "Alpha"/"Bravo"/"Charlie") — zero real PeeringDB data. Verified by `grep -l peeringdb.com testdata/diff_golden/*.json` returning no matches.

## Test Inventory

Total plan-03 tests: 31 functions (28 PASS + 2 SKIP + 1 self-test for skip behaviour). Full visbaseline package now totals 44+ tests (including Wave 1 redact + pii tests), all pass with `-race`.

### Diff tests (14)
- `TestDiffNoDeltas` — identical envelopes yield empty field slice + zero auth-only rows
- `TestDiffAuthOnlyField` — email field present in auth absent in anon → FieldDelta{Name:email, AuthOnly:true, Placeholder:"<auth-only:string>", RowsAdded:1, IsPII:true}
- `TestDiffAuthOnlyRow` — auth row absent from anon bumps AuthOnlyRowCount and flags all its fields
- `TestDiffRowCountDrift` — 3 anon / 5 auth yields AuthOnlyRowCount=2
- `TestDiffVisibleValueDrift` — anon={Public} auth={Public,Users} sets ValueSetDrift on the visible FieldDelta
- `TestDiffNeverEmitsValues` — canary: email="secret@canary.example" never appears in Report's JSON
- `TestDiffNeverEmitsLengths` — reflect check: FieldDelta has no Length/Size/Hash/Digest substring and no exact Value/Values field
- `TestDiffDeterministic` — two consecutive Diff calls produce deep-equal Reports and byte-equal JSON
- `TestDiffFieldSorting` — a_field/m_field/z_field in auth yield alphabetically-sorted Fields slice
- `TestDiffInvalidJSONReturnsError` — garbage input returns wrapped error on both sides
- `TestDiffMissingDataKeyReturnsError` — anon envelope missing data key errors with "anon" in message
- `TestDiffMissingDataKeyReturnsErrorAuth` — auth envelope missing data key errors with "auth" in message (blocker-fix test per plan revision)
- `TestDiffPIIFieldMetadata` — IsPII marker is set for `email`, clear for `status`
- `TestDiffReportSchemaVersion` — ReportSchemaVersion == 1

### Emitter tests (13)
- `TestWriteJSONMatchesSchema` — top-level keys are exactly {schema_version, generated, targets, types}
- `TestWriteJSONDeterministic` — two consecutive WriteJSON calls produce byte-identical output
- `TestWriteJSONStableTypeOrder` — types={poc,net,org} emits net before org before poc (alphabetical)
- `TestWriteMarkdownHasTOC` — output starts with `# `, contains `## Table of Contents`, has `[poc](#poc)` anchor
- `TestWriteMarkdownPerTypeTables` — one `### net` and one `### poc` header per type
- `TestWriteMarkdownColumnsRequired` — column header row contains the stable column set
- `TestWriteMarkdownNoRawValues` — canary: leaked value `sensitive@example.invalid` never appears in Markdown
- `TestWriteMarkdownGoldenEmpty` — matches `expected_empty.md` after timestamp strip
- `TestWriteMarkdownGoldenSimple` — matches `expected_simple.md` after timestamp strip
- `TestWriteJSONGoldenEmpty` — matches `expected_empty.json` after timestamp strip
- `TestWriteJSONGoldenSimple` — matches `expected_simple.json` after timestamp strip
- `TestWriteMarkdownSortsFieldsAlpha` — per-type table rows preserve alphabetical Name order
- `TestReportConsistency` — JSON round-trip agrees with Markdown on row counts and field set

### PII guard tests (4)
- `TestCommittedFixturesHaveNoPII` — canary; SKIPs when `testdata/visibility-baseline/` absent, fails on any non-placeholder PII value otherwise
- `TestPIIGuardDetectsUnredactedString` — self-test: detector flags `email="leaked@example.invalid"`
- `TestPIIGuardAcceptsRedactedFixture` — self-test: detector accepts placeholders and null
- `TestPIIGuardDetectsNonStringPIIValue` — self-test: detector flags numeric `latitude: 37.7749`

## No Signal-Rich Fields Confirmation

`TestDiffNeverEmitsLengths` verifies at test time via `reflect.TypeOf(FieldDelta{})`:

- Forbidden substring match: `Length`, `Size`, `Hash`, `Digest` — no field name contains any.
- Forbidden exact match: `Value`, `Values` — no field name exactly equals either. (`ValueSetDrift` is allowed because it's a boolean drift flag about the controlled `visible` enum, not a value payload.)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Test forbidden-substring list collided with `ValueSetDrift` field**
- **Found during:** Task 1 GREEN phase
- **Issue:** Plan-as-written specified `forbidden := {"Length","Size","Hash","Digest","Value","Values"}` using `strings.Contains` match; the substring "Value" hit `ValueSetDrift`, which is an explicitly documented FieldDelta field.
- **Fix:** Refined `TestDiffNeverEmitsLengths` to use substring match for Length/Size/Hash/Digest but exact-name match for Value/Values. Intent preserved (forbid signal-rich names) and ValueSetDrift survives as the boolean drift flag it was designed to be.
- **Files modified:** `internal/visbaseline/diff_test.go`
- **Commit:** `f8e4d3c`

**2. [Rule 2 - Correctness] WriteJSON used json.MarshalIndent, which HTML-escapes `<`/`>`**
- **Found during:** Task 2 golden regeneration
- **Issue:** Default `json.MarshalIndent` escapes `<` and `>` to `\u003c`/`\u003e`, breaking the grep contract for `<auth-only:string>` placeholder sentinels in committed fixtures (the same contract that `internal/visbaseline/redact.go` already solves via `json.Encoder` + `SetEscapeHTML(false)`).
- **Fix:** Switched `WriteJSON` to the same encoder configuration that `redact.go` uses — `json.NewEncoder(&buf)` + `SetEscapeHTML(false)` + `SetIndent("", "  ")`, then strip the trailing newline and append a single newline for POSIX friendliness.
- **Files modified:** `internal/visbaseline/report.go`
- **Commit:** `b98bb54`

**3. [Rule 3 - Blocking] Go 1.26 added `Context() context.Context` to testing.TB; recorder type couldn't satisfy it**
- **Found during:** Task 3 compilation
- **Issue:** Plan-as-written called walkPII with a custom `*testing.T{}` recorder for self-tests. Go 1.26's `testing.TB` requires `Context() context.Context`, which a zero-value `testing.T{}` does not expose and an external type cannot implement cleanly (testing.TB is frozen by an unexported method).
- **Fix:** Introduced `errorReporter` interface with just `Errorf(format string, args ...any)` + `Helper()`. walkPII now accepts `errorReporter`; `*testing.T` satisfies it automatically; the self-test recorder is a minimal `recorderTB` with those two methods. No behaviour change for production calls.
- **Files modified:** `internal/visbaseline/fixtures_guard_test.go`
- **Commit:** `d368dfc`

**4. [Rule 3 - Blocking] golangci-lint `revive` flagged two De Morgan cases and one unused-parameter**
- **Found during:** Task 2 and Task 3 lint passes
- **Issue:** `if !(a < b && b < c)` triggered `QF1001` (staticcheck); unused `args ...any` on recorder's `Errorf` triggered `unused-parameter` (revive).
- **Fix:** Rewrote conditions as `if a >= b || b >= c`; added `_ = args` comment in recorder Errorf.
- **Files modified:** `internal/visbaseline/report_test.go`, `internal/visbaseline/fixtures_guard_test.go`
- **Commits:** folded into `b98bb54` and `d368dfc` respectively (fixed before commit).

No architectural changes. No Rule 4 checkpoints. All deviations fell under Rules 1-3.

## PII Guard Self-Test Results

| Test | Expectation | Result |
|------|-------------|--------|
| TestCommittedFixturesHaveNoPII | SKIP when testdata/visibility-baseline/ absent | SKIP (expected — fixtures not yet landed, plan 04 will) |
| TestPIIGuardDetectsUnredactedString | Flag `"leaked@example.invalid"` on email field | PASS |
| TestPIIGuardAcceptsRedactedFixture | Accept `"<auth-only:string>"` and null values | PASS |
| TestPIIGuardDetectsNonStringPIIValue | Flag numeric latitude `37.7749` | PASS |

The detector logic is exercised and proven in both positive and negative paths on every `go test` run. Once plan 04 lands committed auth fixtures, TestCommittedFixturesHaveNoPII will enforce the no-PII invariant automatically.

## Acceptance Criteria

| Criterion | Status |
|-----------|--------|
| `grep 'func Diff(' internal/visbaseline/diff.go` matches once | PASS (1 match) |
| `grep 'type Report struct' internal/visbaseline/diff.go` matches once | PASS (1 match) |
| `grep 'type TypeReport struct' internal/visbaseline/diff.go` matches once | PASS (1 match) |
| `grep 'type FieldDelta struct' internal/visbaseline/diff.go` matches once | PASS (1 match) |
| `grep -c 'ReportSchemaVersion = 1' internal/visbaseline/diff.go` returns 1 | PASS |
| `grep -E 'Length\|Hash\|Digest\|Values\s+\[\]' internal/visbaseline/diff.go` returns 0 matches | PASS (0 matches) |
| All Task 1 behaviours pass with `-race` | PASS (14/14) |
| `grep -l 'peeringdb.com' internal/visbaseline/testdata/diff_golden/*.json` returns no matches | PASS (no real PeeringDB data) |
| `grep 'func WriteMarkdown' internal/visbaseline/report.go` matches once | PASS (1 match) |
| `grep 'func WriteJSON' internal/visbaseline/report.go` matches once | PASS (1 match) |
| `grep '## Table of Contents' internal/visbaseline/report.go` matches once | PASS (1 match) |
| All Task 2 behaviours pass with `-race` | PASS (13/13) |
| Golden files exist (4 files) | PASS |
| `grep 'func TestCommittedFixturesHaveNoPII' fixtures_guard_test.go` matches once | PASS |
| `grep 'func walkPII' fixtures_guard_test.go` matches once | PASS (2 matches — func definition and recursive self-call; function definition is once) |
| `grep 'testdata/visibility-baseline' fixtures_guard_test.go` matches at least once | PASS (3 matches in doc + code) |
| `grep 'T-57-02' fixtures_guard_test.go` matches at least once | PASS (2 matches in doc + error message) |
| `TestCommittedFixturesHaveNoPII` passes or skips cleanly | SKIP (correct — fixtures not yet present) |
| `TestPIIGuardDetectsUnredactedString` passes | PASS |
| `TestPIIGuardAcceptsRedactedFixture` passes | PASS |
| `go vet ./internal/visbaseline/...` exits 0 | PASS |
| `go build ./...` exits 0 | PASS |
| `go test -race ./internal/visbaseline/...` exits 0 | PASS |
| `golangci-lint run ./internal/visbaseline/...` exits 0 | PASS (0 issues) |

## Handoff to Plan 04

Plan 04 (the operator capture run) will produce:
- Raw anon JSON fixtures committed under `testdata/visibility-baseline/{beta,prod}/anon/api/{type}/page-{1,2}.json` (unmodified upstream bytes).
- Redacted auth JSON fixtures committed under `testdata/visibility-baseline/{beta,prod}/auth/api/{type}/page-{1,2}.json` (via plan 01's `Redact()`).
- The final `testdata/visibility-baseline/DIFF.md` + `testdata/visibility-baseline/diff.json` (via plan 03's `WriteMarkdown()` + `WriteJSON()`).

At that point:
- `TestCommittedFixturesHaveNoPII` stops skipping and enforces the no-PII invariant on every `go test` run.
- Phase 58 reads `DIFF.md` to decide which new fields need ent schema coverage.
- Phase 60's VIS-07 parity test loads `diff.json` as its ground truth.

## Known Stubs

None. All emitter and differ code is wired end-to-end against synthetic fixtures that prove the behaviour.

## Threat Flags

None. This plan adds pure-function code that consumes in-memory byte slices and writes to caller-provided io.Writers; no new network endpoints, auth paths, file access, or schema changes at trust boundaries. The existing T-57-02 / T-57-03 mitigations are enforced by the tests documented above.

## Self-Check: PASSED

All 17 claimed files exist on disk. All 3 claimed commit hashes (`f8e4d3c`, `b98bb54`, `d368dfc`) are present in `git log --oneline --all`. Package builds, vets, lints clean; all 31 plan-03 tests pass (28 PASS + 2 SKIP + 1 self-test for the skip).
