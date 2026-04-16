---
phase: 57-visibility-baseline-capture
reviewed: 2026-04-16T00:00:00Z
depth: standard
files_reviewed: 13
files_reviewed_list:
  - cmd/pdbcompat-check/capture.go
  - cmd/pdbcompat-check/main.go
  - cmd/pdbcompat-check/redactdiff.go
  - internal/peeringdb/client.go
  - internal/visbaseline/capture.go
  - internal/visbaseline/checkpoint.go
  - internal/visbaseline/config.go
  - internal/visbaseline/diff.go
  - internal/visbaseline/fixtures_guard_test.go
  - internal/visbaseline/pii.go
  - internal/visbaseline/redact.go
  - internal/visbaseline/redactcli.go
  - internal/visbaseline/report.go
  - internal/visbaseline/reportcli.go
  - .gitignore
findings:
  critical: 0
  warning: 5
  info: 8
  total: 13
status: issues_found
---

# Phase 57: Code Review Report

**Reviewed:** 2026-04-16
**Depth:** standard
**Files Reviewed:** 13 (plus `.gitignore` + project conventions)
**Status:** issues_found

## Summary

Phase 57 delivers a visibility-baseline capture subsystem with solid PII hygiene and defence-in-depth. The hard constraint — "no unredacted PII under any committed testdata path" — appears to hold across every code path I traced:

- `capture.writeBytes` routes auth bytes **exclusively** to the private `/tmp` dir (never `OutDir`), with a `default` branch that rejects unknown modes.
- `Redact` always replaces PIIFields regardless of anon presence (defence-in-depth against upstream anon bugs).
- `TestCommittedFixturesHaveNoPII` is a strong canary with self-tests for both positive and negative cases.
- `.gitignore` covers the `/tmp/pdb-vis-capture-*` and `.raw-auth/` staging paths.
- `State` carries only tuple metadata; no response bytes or API keys.
- `runTuple` correctly delegates to the existing `peeringdb.Client` rate-limit path (no second retry layer) and sleeps `RetryAfter + jitter` before retrying.

Findings are limited to: a handful of warning-level correctness issues around path/layout edge cases in the CLI wiring, one resource-cleanup concern on cancellation, and several info items around minor idioms. No critical issues found.

## Warnings

### WR-01: `runCapture` leaks the private /tmp dir on context-cancelled error

**File:** `internal/visbaseline/capture.go:109-156`
**Issue:** `Run` creates `tmpDir` via `os.MkdirTemp` and stores it in `c.rawAuthDir`. On context cancellation or `runTuple` error, `Run` returns the tmp path to the caller but the caller (`cmd/pdbcompat-check/capture.go:86`) only logs/prints it on success. On error the path is swallowed and the directory (containing raw auth bytes if any tuples completed) is never cleaned up. Over many aborted runs this accumulates PII-carrying dirs under `/tmp`. This is partly intentional (resume semantics), but at least two failure modes never produce useful resume state yet still leak: (a) pre-`resolveState` MkdirTemp success followed by a LoadState error, (b) ctx cancellation before any tuple completes.

**Fix:** Either defer-cleanup `tmpDir` when zero tuples completed, or surface the path back to the CLI on error paths too. Minimal fix:
```go
// cmd/pdbcompat-check/capture.go
rawAuthDir, err := capt.Run(ctx)
if rawAuthDir != "" {
    logger.LogAttrs(ctx, slog.LevelInfo, "raw auth staging dir",
        slog.String("path", rawAuthDir),
    )
}
if err != nil {
    return fmt.Errorf("capture run: %w", err)
}
```
The current code *does* return `c.rawAuthDir` alongside errors (capture.go:122, 130, 135, 137, 143) — good — but the caller discards it on error. Pipe it through so operators can `rm -rf` or resume.

### WR-02: `runRedact` path-leaf check mis-handles trailing-slash and non-leaf `outDir`

**File:** `cmd/pdbcompat-check/redactdiff.go:37-43`
**Issue:** The code derives `anonDir` by requiring `outLeaf == "auth"`:
```go
outClean := filepath.Clean(cfg.outDir)
outParent, outLeaf := filepath.Split(outClean)
outLeaf = strings.TrimRight(outLeaf, string(filepath.Separator))
```
Two quirks:
1. `filepath.Clean` already strips trailing separators, so the `TrimRight` is a no-op — remove or document.
2. `filepath.Split("testdata/visibility-baseline/beta/auth")` returns `("testdata/visibility-baseline/beta/", "auth")`, which works. But `filepath.Split("auth")` returns `("", "auth")` — then `filepath.Join(filepath.Clean(""), "anon")` → `filepath.Clean("") == "."` → `anonDir == "anon"` (CWD-relative). This is a footgun: running `-redact -out=auth` accidentally writes next to whatever the CWD is. Combine with the `filepath.Dir(outClean)` on line 66 used to compute "next step" paths — also yields `.` in this degenerate case.

**Fix:** Require `cfg.outDir` to contain at least one separator or use `filepath.IsAbs`-or-relative-with-dir check before splitting. Minimal:
```go
outClean := filepath.Clean(cfg.outDir)
outParent := filepath.Dir(outClean)
outLeaf := filepath.Base(outClean)
if outLeaf != "auth" {
    return fmt.Errorf("-redact: -out %q must end in a /auth/ component", cfg.outDir)
}
if outParent == "." || outParent == string(filepath.Separator) {
    return fmt.Errorf("-redact: -out %q must have a parent directory holding the anon/ sibling", cfg.outDir)
}
anonDir := filepath.Join(outParent, "anon")
```
Use `filepath.Dir`/`filepath.Base` instead of `filepath.Split` — cleaner and the `TrimRight` dance goes away.

### WR-03: `parsePagePath` accepts page-0.json and rejects it silently via `n < 1`, but `Sscanf("%d")` is lax

**File:** `internal/visbaseline/redact.go:144-159` (and its reuse in `internal/visbaseline/reportcli.go:353`)
**Issue:** `fmt.Sscanf(numStr, "%d", &n)` accepts leading whitespace and signed values like `" +5"` as page 5, and returns `n=1, nil` for input "1abc" (Sscanf is permissive — it stops at the first non-matching byte without caring about trailing garbage). This means `page-1extra.json` would parse as page 1 and silently collide with a real `page-1.json`. Low-probability given Capture owns the staging tree, but `RedactDir` advertises that "stray files … are tolerated" — a file like `page-1_backup.json` would be processed as if it were `page-1.json`, potentially overwriting the real one's output at `dstPath`.

**Fix:** Use `strconv.Atoi` for strict parsing:
```go
n, err := strconv.Atoi(numStr)
if err != nil || n < 1 {
    return "", 0, false
}
```
Also consider asserting the basename matches `^page-\d+\.json$` exactly via a regex or by confirming `numStr` contains only digits.

### WR-04: `loadConcatenatedPages` silently ignores duplicate pages after parse-path filtering

**File:** `internal/visbaseline/reportcli.go:339-402`
**Issue:** If two files map to the same `page` number (e.g. `page-1.json` and — given WR-03 — `page-1extra.json`), both get appended to `merged.Data` without warning, doubling the row count and confusing `Diff`'s `AnonRowCount` / `AuthRowCount` aggregates. This cascades through to the report artifact silently.

**Fix:** After sorting by page number, check for duplicates:
```go
sort.Slice(files, func(i, j int) bool { return files[i].page < files[j].page })
for i := 1; i < len(files); i++ {
    if files[i].page == files[i-1].page {
        return nil, fmt.Errorf("duplicate page %d: %s and %s",
            files[i].page, files[i-1].path, files[i].path)
    }
}
```

### WR-05: `PromptResumeOrRestart` returns Restart on EOF but leaks the previous checkpoint's tmp dir

**File:** `internal/visbaseline/capture.go:161-198`, `checkpoint.go:154-168`
**Issue:** The "safe default = restart" choice is sound, but the Restart path does not clean up the *previous* run's `/tmp/pdb-vis-capture-*` directory. The new run creates a fresh `tmpDir` (capture.go:112) and enumerates fresh tuples; the earlier tmp dir with partially written auth bytes persists indefinitely under `/tmp`. Per `.gitignore` this is excluded from VCS, but it's still unredacted PII sitting on disk until reboot or tmpreaper.

**Fix:** On Restart, either (a) log a warning prompting the operator to clean up the old tmp dir, or (b) record the most-recent `rawAuthDir` in State (as a new field) and `os.RemoveAll` it on Restart. Option (a) is lighter and keeps State PII-free:
```go
// In resolveState after Restart answer:
c.cfg.Logger.LogAttrs(ctx, slog.LevelWarn,
    "restart discards checkpoint; prior /tmp/pdb-vis-capture-* dir (if any) must be cleaned manually",
    slog.String("state_path", c.cfg.StatePath),
)
```
Option (b) adds a `PrevRawAuthDir string` field to `State` — no PII, just a path — and lets Restart remove it. This also mitigates the WR-01 accumulation issue.

## Info

### IN-01: `runRedact` / `runDiff` / `runCapture` duplicate signal-handling boilerplate

**File:** `cmd/pdbcompat-check/capture.go:76-84`, `redactdiff.go:45-53`, `redactdiff.go:82-90`
**Issue:** Three copies of the same `context.WithCancel` + `signal.Notify(SIGINT,SIGTERM)` + goroutine pattern. Per GO-CS-3 ("small interfaces, composition"), factor into a helper.
**Fix:**
```go
// signalContext returns a context cancelled on SIGINT/SIGTERM plus a cleanup func.
func signalContext(logger *slog.Logger, msg string) (context.Context, context.CancelFunc) {
    ctx, cancel := context.WithCancel(context.Background())
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
    go func() {
        select {
        case <-sigCh:
            logger.Info(msg)
            cancel()
        case <-ctx.Done():
        }
    }()
    return ctx, cancel
}
```
Also note each existing copy leaks the signal goroutine until process exit — low impact for a one-shot CLI, but a `select { case <-sigCh: ...; case <-ctx.Done(): return }` avoids the leak.

### IN-02: `removeString` is a one-shot helper where `slices.DeleteFunc` is stdlib

**File:** `cmd/pdbcompat-check/capture.go:147-155`
**Issue:** Go 1.21+ `slices.DeleteFunc` does this in one line.
**Fix:**
```go
modes = slices.DeleteFunc(modes, func(m string) bool { return m == "auth" })
```
Drop the helper.

### IN-03: `main.go` `allTypes` is package-var-plus-init — prefer `const` or sorted literal

**File:** `cmd/pdbcompat-check/main.go:22-26, 259-262`
**Issue:** `init()` calls `slices.Sort(allTypes)` on a list that is already lexicographically sorted in source. Either the sort is dead (the source is already ordered) or it's defensive. Either way the `init` function is noise.
**Fix:** Drop the `init` — the source order is correct, and if it drifts, the CI golangci-lint `unsorted` rule (if enabled) would catch it. Alternatively, promote `visbaseline.AllTypes` as the single source of truth and drop `allTypes` entirely.

### IN-04: `findGoldenDir` fallback swallows `filepath.Abs` error

**File:** `cmd/pdbcompat-check/main.go:248-252`
**Issue:** `abs, _ := filepath.Abs(c)` discards the error. `filepath.Abs` only fails if it can't get CWD, in which case the subsequent `os.ReadFile(goldenPath)` would also fail with a more confusing error.
**Fix:** Return the error or at least log at debug level. Minor.

### IN-05: `Redact`'s `placeholderFor` collapses nested objects/arrays to `PlaceholderString`

**File:** `internal/visbaseline/redact.go:129-144`
**Issue:** Nested structure on a PII field (e.g. hypothetical `"address": {"street": "..."}` if upstream schema changed) becomes `"<auth-only:string>"`. This is *correct* per the phase 57 policy (Pitfall 4: structural reveal is a leak surface) and the comment at line 128 documents it — but it means `TestCommittedFixturesHaveNoPII`'s `case float64, bool` branch at fixtures_guard_test.go:111-116 would not fire for an unredacted *nested* PII value, because `walkPII` recurses into the map. If someone forgets to run `Redact` and commits a raw nested object, the guard would walk *into* it and check each child against `IsPIIField`. For current PeeringDB types this is a non-issue (PII is all scalar today), but it's a latent hole if the schema ever changes.

**Fix:** Extend `walkPII` to also flag when a PII key's value is `map[string]any` or `[]any`. Keep the rest of the recursion:
```go
case map[string]any, []any:
    r.Errorf("%s: PII field %s has structured value of type %T (redactor was bypassed)",
        file, childPath, cv)
```
This catches the future-schema leak without making the current tests stricter than needed.

### IN-06: `envelope` and `envelopeForDiff` are duplicated shapes

**File:** `internal/visbaseline/redact.go:22-25`, `diff.go:49-52`
**Issue:** Two structs with identical fields and json tags. Minor duplication, and the two uses (write vs. read-only) are structurally identical.
**Fix:** Collapse into one unexported `envelope` struct shared by both files. Rename the diff-local copy if it grows diff-specific fields later.

### IN-07: `WriteJSON` mutates the caller's `rep.Types` map

**File:** `internal/visbaseline/report.go:31-36`
**Issue:** The loop normalises nil `Fields` slices by *writing back* to `rep.Types[k]`. Since Go maps are reference types, this mutates the caller's Report. Callers with multiple emit paths could see a first call hydrate nil slices and a second call see them as `[]FieldDelta{}`. Minor — no current caller reuses — but violates the pure-emit contract suggested by the function name.
**Fix:** Copy the map first:
```go
types := make(map[string]TypeReport, len(rep.Types))
for k, tr := range rep.Types {
    if tr.Fields == nil {
        tr.Fields = make([]FieldDelta, 0)
    }
    types[k] = tr
}
rep.Types = types
```
Alternatively, normalise at construction in `Diff` instead of at emit time.

### IN-08: `LoadState` accepts Version == 1 but Save stamps Version only on zero-value

**File:** `internal/visbaseline/checkpoint.go:63-83`
**Issue:** `Save` only stamps `Version = stateVersion` when `s.Version == 0`. An in-memory State constructed with an explicit wrong version would write that wrong version and Save would not correct it. No caller does this today, but a future refactor that explicitly sets `Version` to a migration-in-progress value could confuse the LoadState round-trip.
**Fix:** Always stamp:
```go
s.Version = stateVersion
```
Or guard with `if s.Version != stateVersion { return fmt.Errorf(...) }` at Save time.

---

## Positive Observations (Focus-Area Verdicts)

1. **PII safety** — `Redact` + `PIIFields` + `TestCommittedFixturesHaveNoPII` form a solid three-layer defence. Placeholder literals are greppable (HTML-escaping disabled in both `redact.go` and `report.go`). The `<auth-only:TYPE>` format is genuinely un-confusable with real data. `walkPII` correctly handles unicode via `json.Unmarshal` and recurses into nested maps and arrays.

2. **Rate-limit correctness** — `capture.runTuple` does exactly one thing on `RateLimitError`: sleep `RetryAfter + jitter`, honour ctx, retry the same tuple. No second retry ladder. The underlying `peeringdb.Client.doWithRetry` short-circuits 429 as documented in its comment (client.go:340-360). Good layering.

3. **Checkpoint safety** — `State.Save` uses the correct `WriteFile(tmp, 0o600) → Rename` atomic pattern. `Advance` persists after every tuple, so Ctrl+C between steps preserves the invariant. State only carries tuple metadata — no PII, no API keys. The `TestCheckpointContainsNoPayload` expectation (mentioned in comments) is a clean fence.

4. **Resource cleanup** — HTTP bodies are all drained and closed in `doWithRetry` (including the 429 path at client.go:325). `FetchRawPage` uses `defer resp.Body.Close()`. The tmp-dir and signal-goroutine leaks noted in WR-01 / IN-01 are minor for a one-shot CLI but worth fixing.

5. **Cross-package API shape** — `FetchRawPage` is idiomatic: `ctx`-first (GO-CTX-1), returns `[]byte` + error, reuses `doWithRetry`'s rate limiter and API-key path. The 1-based-page contract is documented and bounds-checked.

6. **CLI wiring** — GO-CFG-1 fail-fast checks are present in `New` (capture.go:48-100), `RedactDir` (redactcli.go:50-64), and `validateBuildReportConfig` (reportcli.go:82-105). `main.go` correctly enforces mutual-exclusivity of `-capture` / `-redact` / `-diff`. API key is read from env var as fallback (main.go:89-91) and never logged anywhere I could find.

No secrets, `eval`, `exec`, `innerHTML`, or debug-artifact hits in any reviewed file.

---

_Reviewed: 2026-04-16_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
