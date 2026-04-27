---
phase: 74-test-ci-debt
plan: 03
subsystem: testing
tags: [lint, hygiene, gosec, visbaseline, nolintlint]

requires:
  - phase: 64-field-level-privacy
    provides: internal/visbaseline package (added in v1.14 phase 57)
provides:
  - filepath.Clean defense-in-depth on all 5 operator-supplied path I/O sites in internal/visbaseline
  - Canonical "operator-supplied by contract" rationale comments at each site (uniform wording across the package)
  - Whole-repo golangci-lint baseline restored to 0 issues with the lint hygiene approach future-proofed against the gosec/nolintlint interaction
affects: [internal/visbaseline future maintenance, future visbaseline CLI extensions]

tech-stack:
  added: []
  patterns:
    - "operator-supplied-by-contract rationale: leading comment + filepath.Clean + (optionally) rule-specific //nolint when a sanitiser-resistant rule like gosec G122 fires"

key-files:
  created: []
  modified:
    - internal/visbaseline/redactcli.go
    - internal/visbaseline/reportcli.go
    - internal/visbaseline/checkpoint.go

key-decisions:
  - "Removed //nolint:gosec directives at 4/5 sites because filepath.Clean satisfies gosec G304's static analysis (sanitiser-recognised) and nolintlint correctly flags then-stale directives. Preserved the canonical rationale as a leading comment at each site so the disposition is still self-documenting."
  - "Added a narrow rule-specific //nolint:gosec // G122 directive at the auth-side os.ReadFile inside the filepath.WalkDir callback because G122 (TOCTOU symlink risk) is not silenced by filepath.Clean — the pre-existing wildcard //nolint:gosec was suppressing G122 as well as G304."

patterns-established:
  - "When applying filepath.Clean to a gosec G304 site, drop the //nolint directive — gosec recognises Clean as a sanitiser; the directive becomes nolintlint-stale."
  - "When a path I/O happens inside filepath.WalkDir/Walk callback, gosec G122 fires independently of G304 and IS NOT silenced by filepath.Clean — apply rule-specific //nolint:gosec // G122 with explicit rationale."

requirements-completed: [TEST-03]

duration: 12min
completed: 2026-04-26
---

# Phase 74-03: visbaseline Lint Hygiene Summary

**filepath.Clean defense-in-depth + canonical rationale comments at all 5 operator-supplied path I/O sites; gosec/nolintlint interaction handled correctly so whole-repo lint stays at 0 issues.**

## Performance

- **Duration:** ~12 min
- **Started:** 2026-04-26T22:00:00Z (worktree spawn)
- **Completed:** 2026-04-26T22:12:00Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments

- All five operator-supplied path I/O sites in `internal/visbaseline` now apply `filepath.Clean()` before `os.ReadFile` / `os.OpenFile` — defense-in-depth against the genuine `..` traversal sub-class of G304 risk.
- Canonical rationale text "visbaseline is a CLI tool — paths are operator-supplied by contract" present as a leading comment block at every site, so future readers see uniform, honest documentation.
- Whole-repo `golangci-lint run ./...` and `go test -count=1 -race ./internal/visbaseline/... ./cmd/...` both pass clean.
- `exhaustive` shapeUnknown switch case and `nolintlint` stale directive findings cited in CONTEXT.md plan-hints confirmed already resolved in current source (no edit required at those sites).

## Task Commits

1. **Task 1: Apply filepath.Clean + canonical nolint to all five G304 sites** — `f3e0056` (fix)
2. **Task 2: Confirm whole-repo lint stays green** — `9f5aacb` (fix; corrects Task 1's nolintlint regression by removing now-stale directives)

## Files Created/Modified

- `internal/visbaseline/redactcli.go` — added `filepath.Clean(anonPath)` + `filepath.Clean(path)` before the two `os.ReadFile` calls inside `RedactDir`'s `WalkDir` callback; replaced vague `G304: path derived from CLI caller` rationale with canonical "operator-supplied by contract" leading comment block. Auth-side site (line 110) carries a targeted `//nolint:gosec // G122` directive because the path flows from the WalkDir callback.
- `internal/visbaseline/reportcli.go` — added `filepath.Clean(jsonPath)` and `filepath.Clean(p)` before the two `os.OpenFile` calls in `writeReportArtifacts` / `writeMarkdownOnly`; removed nolint directives, replaced with canonical leading comments.
- `internal/visbaseline/checkpoint.go` — added `path/filepath` import; added `filepath.Clean(path)` before `os.ReadFile(path)` in `LoadState`; removed nolint directive, replaced with canonical leading comment.

## Five sites updated

| File | Line (post-edit) | Call | Disposition |
|---|---|---|---|
| `internal/visbaseline/redactcli.go` | 100 | `os.ReadFile(anonPath)` | filepath.Clean + leading comment, no nolint |
| `internal/visbaseline/redactcli.go` | 110 | `os.ReadFile(path)` (auth-side, in WalkDir) | filepath.Clean + leading comment + targeted `//nolint:gosec // G122` (TOCTOU rule, not silenced by Clean) |
| `internal/visbaseline/reportcli.go` | 428 | `os.OpenFile(jsonPath, ...)` | filepath.Clean + leading comment, no nolint |
| `internal/visbaseline/reportcli.go` | 444 | `os.OpenFile(p, ...)` | filepath.Clean + leading comment, no nolint |
| `internal/visbaseline/checkpoint.go` | 119 | `os.ReadFile(path)` | filepath.Clean + leading comment, no nolint |

## Decisions Made

- **Removed //nolint:gosec at 4/5 sites; kept rule-specific G122 nolint at 1/5.** Discovered during Task 2: after applying `filepath.Clean`, gosec recognises Clean as a sanitiser and stops firing G304 — making the canonical-text nolint directives stale per nolintlint. The plan's must_haves stated both "use filepath.Clean" AND "carry the canonical nolint directive", which is internally inconsistent given gosec's actual sanitiser handling. Resolution per the plan's own escape hatch (Task 1 step 7: "removal is the correct fix for stale directives") — drop the directives at 4/5 sites but preserve the canonical rationale as a leading comment so the documentation goal is still achieved. The 5th site (`redactcli.go:110`, inside `filepath.WalkDir` callback) trips gosec G122 (TOCTOU symlink traversal risk), which Clean does NOT silence, so a targeted `//nolint:gosec // G122` is genuinely needed there. The pre-existing wildcard `//nolint:gosec` had been silently suppressing G122 too; removing the wildcard exposed it.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Plan must_haves were internally inconsistent given gosec's filepath.Clean sanitiser recognition; removed the now-stale //nolint:gosec directives at 4/5 sites and preserved the canonical rationale as leading comments**

- **Found during:** Task 2 (whole-repo lint verification)
- **Issue:** After Task 1 applied filepath.Clean and reworded the //nolint:gosec directives to canonical text per the plan, `golangci-lint run ./...` reported 3 nolintlint findings — gosec recognises filepath.Clean as a G304 sanitiser, so the //nolint:gosec directives became stale (unused for linter "gosec"). The plan's must_haves required BOTH `filepath.Clean` AND the canonical //nolint directive simultaneously, which is impossible because nolintlint correctly flags the now-stale directive.
- **Fix:** Removed the //nolint:gosec markers at 4/5 sites and converted the canonical rationale text into a leading comment block above each `filepath.Clean` call. This satisfies the plan's documentation intent ("future readers see uniform, honest documentation of why visbaseline suppresses G304") and the lint-clean must_have, even though it diverges from the literal "carry the canonical nolint directive" must_have. Per Plan 74-03 Task 1 step 7: "removal is the correct fix for stale directives."
- **Files modified:** `internal/visbaseline/redactcli.go`, `internal/visbaseline/reportcli.go`, `internal/visbaseline/checkpoint.go`
- **Verification:** `golangci-lint run ./...` returns `0 issues.`; canonical rationale text grep-able at every site as a comment.
- **Committed in:** `9f5aacb` (Task 2 commit)

**2. [Rule 2 - Missing critical functionality] Re-added a narrow //nolint:gosec // G122 directive at redactcli.go:110 because filepath.Clean does not silence G122 (TOCTOU symlink traversal in WalkDir callback)**

- **Found during:** Task 2 (after removing the wildcard //nolint:gosec directives)
- **Issue:** The pre-existing `//nolint:gosec` directives carried no rule ID and were silently suppressing both G304 (path traversal) AND G122 (TOCTOU symlink race in `filepath.WalkDir` callback). Once I removed the wildcard directives at all 5 sites, G122 fired at the auth-side `os.ReadFile(path)` site in `redactcli.go` because that site is inside a `WalkDir` callback. G122 is NOT silenced by `filepath.Clean` — Clean addresses lexical traversal, not race conditions. The site genuinely needs a suppression directive (visbaseline is a single-tenant CLI run by an operator against a staging tree they control; a TOCTOU attacker would need write access to the operator's redaction workspace, which is out of the threat model).
- **Fix:** Added a targeted `//nolint:gosec // G122: visbaseline CLI staging tree, see comment above` directive at `redactcli.go:110` only (4 lines after the `filepath.Clean(path)`). Rule-specific (G122-only) so it is not stale w.r.t. G304, and it has a specific reason.
- **Files modified:** `internal/visbaseline/redactcli.go`
- **Verification:** `golangci-lint run ./internal/visbaseline/...` returns `0 issues.`; the directive is grep-able and rule-specific.
- **Committed in:** `9f5aacb` (Task 2 commit, alongside the directive removals)

---

**Total deviations:** 2 auto-fixed (1 plan-internal contradiction, 1 missing critical suppression for a different gosec rule)
**Impact on plan:** Both deviations were necessary to satisfy the plan's "lint stays at 0 issues" must_have. The literal "carry the canonical nolint directive at three G304 sites" must_have was impossible given gosec's actual sanitiser behaviour; the canonical rationale TEXT survives as leading comments so the documentation intent is satisfied. No scope creep — only the 5 sites listed in the plan were touched.

## Confirmation of pre-existing-clean findings

Per the plan's verification step:

- **`reportcli.go:75` exhaustive shapeUnknown case:** Confirmed present in current source (line 75-76, `case shapeUnknown: return fmt.Errorf("BuildReport: shape not detected in %q", cfg.BaselineRoot)`). `golangci-lint run --enable-only=exhaustive ./internal/visbaseline/...` returns 0 issues. No edit needed.
- **`reportcli.go:387` stale nolintlint directive:** Current source line 387 is comment text (`// pf.path is composed from dir...`), not a `//nolint` directive. `golangci-lint run --enable-only=nolintlint ./internal/visbaseline/...` returns 0 issues post-edit. No edit needed.

## Final lint output (whole-repo)

```
$ golangci-lint run ./...
0 issues.
```

## Issues Encountered

The plan's must_haves had an internal contradiction (gosec recognises filepath.Clean as a G304 sanitiser, so a Clean'd path with a //nolint:gosec G304 directive trips nolintlint). Resolved by following the plan's own escape hatch (Task 1 step 7) — see Deviation 1.

The pre-existing wildcard `//nolint:gosec` was suppressing both G304 and G122 silently. Once removed, G122 surfaced at the WalkDir site. Resolved with a rule-specific G122 directive — see Deviation 2.

## User Setup Required

None.

## Self-Check

Verified:
- `internal/visbaseline/redactcli.go` exists at `/home/dotwaffle/Code/pdb/peeringdb-plus/.claude/worktrees/agent-affe5281e4ccff6ce/internal/visbaseline/redactcli.go` — FOUND
- `internal/visbaseline/reportcli.go` exists — FOUND
- `internal/visbaseline/checkpoint.go` exists — FOUND
- Commit `f3e0056` exists in `git log` — FOUND
- Commit `9f5aacb` exists in `git log` — FOUND
- `golangci-lint run ./internal/visbaseline/...` → 0 issues — VERIFIED
- `golangci-lint run ./...` → 0 issues — VERIFIED
- `go test -count=1 -race ./internal/visbaseline/...` → PASS — VERIFIED
- `go test -count=1 -race ./cmd/...` → PASS — VERIFIED

## Self-Check: PASSED

## Next Phase Readiness

TEST-03 fully closed. Phase 74's other plans (74-01 TEST-01 schema-driven index assertion + 74-02 TEST-02 region template var drop) are independent and can be merged in any order. No blockers for the rest of v1.18.0.

---
*Phase: 74-test-ci-debt*
*Completed: 2026-04-26*
