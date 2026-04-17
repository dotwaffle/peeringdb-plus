---
phase: 57-visibility-baseline-capture
plan: 01
subsystem: infra
tags: [visibility, redaction, pii, scaffolding, json, stdlib]

# Dependency graph
requires:
  - phase: 11-3-authenticated-sync
    provides: internal/peeringdb.Client with WithAPIKey functional option (identifies which fields carry PII via types.go)
provides:
  - internal/visbaseline package skeleton with PII allow-list and pure-function Redact
  - Typed placeholder contract (<auth-only:string|number|bool>) for redacted auth fixtures
  - .gitignore rules excluding raw auth bytes and /tmp checkpoint files from repo
affects: [57-02, 57-03, 57-04, 58-schema-audit, 59-privacy-policy, 60-parity-tests]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Pure-function redactor with deterministic output (json.Encoder + SetEscapeHTML(false) + SetIndent)"
    - "PII allow-list as single source of truth (PIIFields slice, O(1) IsPIIField lookup) used by redactor + future guard tests"
    - "Synthetic testdata fixtures (no real PeeringDB contents) for unit tests against PII-handling code"

key-files:
  created:
    - internal/visbaseline/pii.go
    - internal/visbaseline/pii_test.go
    - internal/visbaseline/redact.go
    - internal/visbaseline/redact_test.go
    - internal/visbaseline/testdata/anon_sample.json
    - internal/visbaseline/testdata/auth_sample.json
  modified:
    - .gitignore

key-decisions:
  - "Placeholder format is <auth-only:TYPE> (angle-bracketed, not valid JSON identifier — trivially greppable by plan 03 PII guard)"
  - "json.Encoder.SetEscapeHTML(false) required so placeholders remain literal in committed fixtures (default MarshalIndent would emit \\u003c / \\u003e)"
  - "PII fields are ALWAYS redacted regardless of whether they appear in anon (belt-and-braces against upstream anon bugs — threat T-57-02)"
  - "Complex nested values (arrays, objects) in auth-only rows collapse to PlaceholderString (Pitfall 4: structural reveal is also a leak surface)"

patterns-established:
  - "Pure-function module design: Redact has no filesystem, network, or global state — all IO is done by callers"
  - "Alphabetised string-slice constants (PIIFields) with a companion O(1) lookup map built in a package-level immediately-invoked func"
  - "Typed placeholders over string-only (bool/number values get distinct placeholders so the diff tracks type changes, not just presence)"

requirements-completed: [VIS-01]

# Metrics
duration: 5min
completed: 2026-04-16
---

# Phase 57 Plan 01: Redactor scaffolding Summary

**Pure-function Redact with PII allow-list and three typed placeholders, landing the foundational safety net that subsequent visibility-baseline plans depend on.**

## Performance

- **Duration:** 5 min
- **Started:** 2026-04-16T20:59:52Z
- **Completed:** 2026-04-16T21:04:59Z
- **Tasks:** 3 (two TDD, one chore)
- **Files modified:** 7 (6 created, 1 modified)

## Accomplishments

- `internal/visbaseline` package exists and compiles with a 16-field PII allow-list (`PIIFields`) and an O(1) `IsPIIField` lookup shared with future plans
- `Redact(anonBytes, authBytes []byte) ([]byte, error)` is a deterministic pure function that produces byte-identical output on repeat calls
- 11 passing unit tests cover every behaviour listed in the plan: canary PII-substring assertion, determinism, auth-only rows and fields, preservation of `id` / `visible` / envelope shape, defence-in-depth PII redaction, and invalid-input error paths
- `.gitignore` closes the raw-auth leak surface (T-57-01) while still allowing the redacted `testdata/visibility-baseline/**/auth/api/*/page-*.json` paths to be committed

## Task Commits

Each task was TDD-disciplined (RED test → GREEN implementation) and committed atomically:

1. **Task 1 RED: Failing PII allow-list tests** — `0f74d59` (test)
2. **Task 1 GREEN: PIIFields + IsPIIField** — `f3c748c` (feat)
3. **Task 2 RED: Failing Redact tests + synthetic fixtures** — `42d4533` (test)
4. **Task 2 GREEN: Redact pure function** — `022d119` (feat)
5. **Task 3: .gitignore raw-auth and /tmp rules** — `5886ca6` (chore)

Worktree mode: no plan-metadata commit in this agent — the orchestrator merges and writes STATE.md/ROADMAP.md centrally.

## Files Created/Modified

- `internal/visbaseline/pii.go` — `PIIFields` slice (16 canonical field names, alphabetised) and `IsPIIField(name string) bool` O(1) lookup
- `internal/visbaseline/pii_test.go` — table-driven tests covering 26 field-name cases plus sort-order invariant
- `internal/visbaseline/redact.go` — `Redact` pure function, `PlaceholderString`/`PlaceholderNumber`/`PlaceholderBool` constants, private `envelope`/`redactValue`/`placeholderFor` helpers
- `internal/visbaseline/redact_test.go` — 9 `TestRedaction*` tests including the canary substring-leak assertion
- `internal/visbaseline/testdata/anon_sample.json` — 2-row synthetic anonymous envelope using `.example` hostnames
- `internal/visbaseline/testdata/auth_sample.json` — 3-row synthetic auth envelope with fake PII on all three rows, plus one auth-only row (id=99, visible=Users)
- `.gitignore` — appended 7 lines under a "Visibility baseline capture (phase 57)" header

## Decisions Made

- **Placeholder format `<auth-only:TYPE>`**: angle brackets make the placeholder syntactically impossible to confuse with real PeeringDB data (no quoted field in any type uses them), and the substring is trivially greppable by plan 03's PII guard test.
- **`json.Encoder.SetEscapeHTML(false)` over `json.MarshalIndent`**: default `MarshalIndent` escapes `<` and `>` as `\u003c` / `\u003e`, which would break the greppability contract. Using `Encoder` with explicit `SetEscapeHTML(false)` and `SetIndent("", "  ")` keeps the literal characters while preserving deterministic output (Go's `encoding/json` sorts map keys since 1.12).
- **PII fields ALWAYS redacted**: even when present in both anon and auth with identical values. Rationale: an upstream bug could leak PII anonymously (threat T-57-02 / Pitfall 2 of phase 57 research). The redactor does not trust anon exoneration for known-PII field names.
- **Complex nested values collapse to `PlaceholderString`**: arrays and objects inside auth-only rows could themselves carry signal-rich structure (Pitfall 4: structural reveal). Collapsing to a flat string placeholder removes this leak surface; the baseline phase only cares about top-level field presence and type.
- **Trim trailing newline from `json.Encoder.Encode` output**: keeps `Redact` output equivalent to what `json.MarshalIndent` would produce (no gratuitous trailing `\n`), so callers writing the bytes directly to a file and readers diffing them see the same shape.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 — Bug] Replaced `json.MarshalIndent` with `json.Encoder` + `SetEscapeHTML(false)`**

- **Found during:** Task 2 (GREEN phase of redactor implementation — `TestRedactionStripsPII` positive-assertion failed)
- **Issue:** The plan's proposed implementation used `json.MarshalIndent`, which HTML-escapes `<` and `>` to `\u003c` and `\u003e`. The redacted output therefore did not contain the literal `<auth-only:string>` substring; the canary positive-assertion (`strings.Contains(out, PlaceholderString)`) failed, and any downstream `git grep '<auth-only:'` (as the plan 03 PII guard test will do) would silently match nothing and fail to detect leaks.
- **Fix:** Swapped `json.MarshalIndent(&auth, "", "  ")` for a `bytes.Buffer` + `json.NewEncoder(&buf)` pipeline with `SetEscapeHTML(false)` + `SetIndent("", "  ")`, followed by `bytes.TrimRight(..., "\n")` to drop the encoder's trailing newline. Output remains deterministic (same map-key ordering rules apply).
- **Files modified:** `internal/visbaseline/redact.go`
- **Verification:** `TestRedactionStripsPII` and all other `TestRedaction*` tests pass; `golangci-lint run ./internal/visbaseline/...` reports `0 issues`; manual inspection of output confirms `<auth-only:string>` appears literally in the redacted bytes.
- **Committed in:** `022d119` (Task 2 GREEN commit)

**2. [Rule 2 — Missing critical] Added `bytes` import for the trimmed-encoder pattern**

- **Found during:** Task 2 (fix 1 above)
- **Issue:** `bytes.Buffer` and `bytes.TrimRight` are required by the encoder-based marshalling.
- **Fix:** Added `"bytes"` to the import block.
- **Files modified:** `internal/visbaseline/redact.go`
- **Verification:** `go build ./internal/visbaseline/...` exits 0.
- **Committed in:** `022d119` (same commit as fix 1).

---

**Total deviations:** 2 auto-fixed (1 bug, 1 blocking-import). Both were essential for the redactor's greppability contract. No scope creep.

**Impact on plan:** Preserves every documented contract; changes only the internal marshalling path. The public signature `Redact(anonBytes, authBytes []byte) ([]byte, error)` is unchanged.

## Issues Encountered

- **Plan's `git check-ignore /tmp/pdb-vis-capture-state.json` acceptance step cannot exit 0**: `git check-ignore` refuses absolute paths outside the repository root (returns fatal + exit 128). The in-repo equivalent (`tmp/pdb-vis-capture-state.json` relative to repo root) correctly reports as ignored. The `.gitignore` rule itself is correctly installed; the automated test command in the plan is script-brittle, not the rule.
- **Worktree did not contain PLAN.md files on entry**: the phase directory in the worktree only had CONTEXT/RESEARCH/VALIDATION. Plans were copied from the main repo (`/home/dotwaffle/Code/pdb/peeringdb-plus/.planning/phases/57-visibility-baseline-capture/57-0{1..4}-PLAN.md`) into the worktree's `.planning/` tree so the execution could proceed. No edits were made to the plan files.

## Handoff

- **Plan 02** can import `github.com/dotwaffle/peeringdb-plus/internal/visbaseline` and call `Redact` from the capture loop. The placeholder constants and `PIIFields` are exported for programmatic use.
- **Plan 03** (diff + PII guard test) will `grep` committed auth fixtures for non-placeholder PII substrings; it can rely on the literal `<auth-only:TYPE>` format being present because `SetEscapeHTML(false)` keeps angle brackets unescaped. The guard test should also assert that any field name in `visbaseline.PIIFields` only holds the known placeholder values within committed auth fixtures.
- **Plan 04** (operator's live capture run) inherits the `.gitignore` rules; the operator may stage raw auth bytes under `testdata/visibility-baseline/<target>/.raw-auth/` or under `/tmp/pdb-vis-capture-*` without risk of accidental commit.

## User Setup Required

None — this plan adds only package-level code and `.gitignore` entries; no external services, env vars, or dashboard configuration.

## Next Phase Readiness

- Package skeleton, PII allow-list, and redactor ready for plans 02/03 to import. Neither plan introduces circular dependencies with `internal/visbaseline`.
- Wave 0 gaps remaining per plan `<verification>`: `checkpoint.go`/`_test.go` and `capture.go`/`_test.go` (plan 02); `diff.go`/`_test.go` and `report.go`/`_test.go` (plan 03).

## Self-Check: PASSED

- `internal/visbaseline/pii.go` — FOUND
- `internal/visbaseline/pii_test.go` — FOUND
- `internal/visbaseline/redact.go` — FOUND
- `internal/visbaseline/redact_test.go` — FOUND
- `internal/visbaseline/testdata/anon_sample.json` — FOUND
- `internal/visbaseline/testdata/auth_sample.json` — FOUND
- `.gitignore` — FOUND (phase 57 section appended)
- Commit `0f74d59` (test: PII) — FOUND
- Commit `f3c748c` (feat: PII) — FOUND
- Commit `42d4533` (test: Redact) — FOUND
- Commit `022d119` (feat: Redact) — FOUND
- Commit `5886ca6` (chore: .gitignore) — FOUND

## TDD Gate Compliance

Plan type is `execute` (not `tdd`), so no plan-level TDD gate. However, both Task 1 and Task 2 carry `tdd="true"` and followed the RED → GREEN cycle explicitly: a failing test commit precedes each implementation commit (`0f74d59 → f3c748c`, `42d4533 → 022d119`). No REFACTOR phase was needed — implementations were minimal and already clean.

---
*Phase: 57-visibility-baseline-capture*
*Completed: 2026-04-16*
