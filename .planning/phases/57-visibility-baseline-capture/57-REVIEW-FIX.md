---
phase: 57
fixed_at: 2026-04-16T00:00:00Z
review_path: .planning/phases/57-visibility-baseline-capture/57-REVIEW.md
iteration: 1
findings_in_scope: 5
fixed: 5
skipped: 0
status: all_fixed
---

# Phase 57: Code Review Fix Report

**Fixed at:** 2026-04-16
**Source review:** `.planning/phases/57-visibility-baseline-capture/57-REVIEW.md`
**Iteration:** 1

**Summary:**
- Findings in scope (Warnings only): 5
- Fixed: 5
- Skipped / Deferred: 0
- Info findings intentionally skipped (scope = critical+warning): 8

## Fixed Issues

### WR-01: `runCapture` leaks the private /tmp dir on context-cancelled error

**Severity:** Warning
**Files modified:** `cmd/pdbcompat-check/capture.go`
**Commit:** `563a468`
**Applied fix:** After the `capt.Run(ctx)` call, emit an Info-level log line naming `rawAuthDir` whenever the returned path is non-empty, regardless of whether `err` is nil. This surfaces the staging dir to operators on both success and failure paths so they can `rm -rf` a leaked dir or resume. The existing `return fmt.Errorf("capture run: %w", err)` path is preserved; the new log line is a strict addition.

### WR-02: `runRedact` path-leaf check mis-handles trailing-slash and non-leaf `outDir`

**Severity:** Warning
**Files modified:** `cmd/pdbcompat-check/redactdiff.go`
**Commit:** `9fc0783`
**Applied fix:** Replaced the `filepath.Split` + `strings.TrimRight` combination with `filepath.Dir` / `filepath.Base` on the cleaned path. Added an explicit reject for `outParent == "."` and `outParent == string(filepath.Separator)` so degenerate inputs like `-out=auth` or `-out=/auth` fail fast with a clear error rather than writing the anon sibling into the CWD or the filesystem root. Removed the now-unused `strings` import.

### WR-03: `parsePagePath` accepts page-0.json and rejects it silently via `n < 1`, but `Sscanf("%d")` is lax

**Severity:** Warning
**Files modified:** `internal/visbaseline/redactcli.go`, `internal/visbaseline/redactcli_test.go`
**Commit:** `193434c`
**Applied fix:** Replaced `fmt.Sscanf(numStr, "%d", &n)` with an explicit ASCII-digit-only whitelist followed by `strconv.Atoi`. This rejects previously-accepted inputs like `"1abc"`, `"+5"`, `" 5"`, and `"--1"` so filenames like `page-1_backup.json` no longer collide with real `page-1.json` captures. Extended `TestParsePagePath` with six new negative cases covering trailing garbage, signed values, and embedded whitespace. Added `strconv` import.

### WR-04: `loadConcatenatedPages` silently ignores duplicate pages after parse-path filtering

**Severity:** Warning
**Files modified:** `internal/visbaseline/reportcli.go`
**Commit:** `6155d61`
**Applied fix:** After sorting `files` by ascending page number, iterate adjacent pairs and return a `fmt.Errorf("duplicate page %d: %s and %s", ...)` on any collision. Positioned as defense-in-depth behind WR-03; the WR-03 strict filename parse already makes well-formed capture output immune, but the extra check catches operator mistakes (e.g. `cp page-1.json page-1.json.bak` and later rename).

### WR-05: `PromptResumeOrRestart` returns Restart on EOF but leaks the previous checkpoint's tmp dir

**Severity:** Warning
**Files modified:** `internal/visbaseline/capture.go`
**Commit:** `e9e9754`
**Applied fix:** In `resolveState`, after the operator answers `Restart`, emit a Warn-level log line prompting manual cleanup of the prior `/tmp/pdb-vis-capture-*` dir. Reviewer's Option (a) was chosen over Option (b) because recording the prior `rawAuthDir` in `State` would widen the T-57-04 exposure surface — `State` lives in `/tmp` with 0600 on a multi-tenant host and the existing `TestCheckpointContainsNoPayload` assertion enforces a minimal key set.

## Skipped / Deferred Issues

None. All five in-scope warnings were applied cleanly.

## Info findings (out of scope)

The following info-level findings were intentionally not addressed in this review cycle per the configured scope (critical+warning). They are documented in `57-REVIEW.md` and remain open for a future follow-up pass:

- IN-01: duplicate signal-handling boilerplate across runCapture / runRedact / runDiff
- IN-02: `removeString` helper is a one-shot where `slices.DeleteFunc` exists
- IN-03: `allTypes` package-var-plus-init is noise
- IN-04: `findGoldenDir` swallows `filepath.Abs` error
- IN-05: `walkPII` does not catch structured values under PII keys (latent, not current issue)
- IN-06: `envelope` and `envelopeForDiff` are duplicated shapes
- IN-07: `WriteJSON` mutates the caller's `rep.Types` map
- IN-08: `LoadState` accepts `Version == 1` but `Save` only stamps on zero-value

## Verification

- `go build ./...` — passes
- `go test -race ./...` — passes (all packages; no regressions)
- `go test -race -run TestParsePagePath ./internal/visbaseline/...` — passes with expanded table covering new strict-parse negative cases

---

_Fixed: 2026-04-16_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
