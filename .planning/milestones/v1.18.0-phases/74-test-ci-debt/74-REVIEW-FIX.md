---
phase: 74
fixed_at: 2026-04-26T23:30:00Z
review_path: .planning/phases/74-test-ci-debt/74-REVIEW.md
iteration: 1
findings_in_scope: 3
fixed: 3
skipped: 0
status: applied
---

# Phase 74: Code Review Fix Report

**Fixed at:** 2026-04-26T23:30:00Z
**Source review:** `.planning/phases/74-test-ci-debt/74-REVIEW.md`
**Iteration:** 1

**Summary:**
- Findings in scope: 3 (all warnings; critical+warning auto-mode)
- Fixed: 3
- Skipped: 0

All three warnings from REVIEW.md were applied as atomic per-finding
commits. Repo-wide `golangci-lint run ./...` returns `0 issues.` after
the fixes; targeted `go test -race -count=1` for both modified packages
passes.

Info findings IN-01 through IN-04 were OUT OF SCOPE for this `--auto`
pass (critical+warning only). They remain documented in REVIEW.md for
a follow-on `--all` pass if the operator chooses to address them.

## Fixed Issues

### WR-01: Orphan-template-var test has substring-prefix false-negative

**Files modified:** `deploy/grafana/dashboard_test.go`
**Commit:** `b67afef`
**Applied fix:** Replaced the unbounded `strings.Contains(haystack, "$"+v.Name)`
substring check with a per-variable precompiled regex
`\$(?:\{NAME[}:]|NAME(?:[^A-Za-z0-9_]|$))`. The bare form now requires a
non-identifier byte (or end-of-string) after the variable name; the brace
form requires `}` or `:` (the formatted-variable separator). A future
variable named `type` will no longer be falsely marked referenced by an
expression containing `$type_total` or `${type_count}`. Added the `regexp`
import. Test is hermetic — verified with `go test -race -count=1`.

### WR-02: Orphan test only scans Expr and Datasource.UID

**Files modified:** `deploy/grafana/dashboard_test.go`
**Commit:** `33a6e79`
**Applied fix:** Replaced the typed-struct walk (which only scanned
`tgt.Expr` and `tgt.Datasource.UID`) with a recursive walk of the untyped
JSON tree (`map[string]any` / `[]any`), concatenating every string leaf
into a single haystack. This catches every present and future Grafana
variable surface: `legendFormat`, panel `title`, panel `description`,
alert annotations, links, options.content, and any field added by future
Grafana schema versions — no per-field plumbing needed. The
`templating.list` subtree is excluded from the haystack via path-prefix
check; without that exclusion every variable would self-reference via its
own definition (`query`, `current.text`) and the orphan check would never
fire. Existing tests pass.

### WR-03: legacy_net_fixture sub-test contains a tautological assertion

**Files modified:** `cmd/pdb-schema-generate/main_test.go`
**Commit:** `7f31e06`
**Applied fix:** Two surgical drops, no behaviour change to the helper:

1. `legacy_net_fixture`: removed the `exp := ExpectedIndexesFor(...)` block
   following the explicit `want := []string{"asn","name","org_id","status",
   "updated"}` literal. The literal is the INDEPENDENT expectation that
   actually catches generator-rule drift; the round-trip was f(x)==f(x).

2. `per_entity_from_schema_source`: removed the `exp := ExpectedIndexesFor(
   apiPath, ot)` round-trip and its slice-equality assertion. Drift
   protection in this loop now comes solely from the structural-sanity
   invariants (every emitted index is a real field, an always-on synthetic
   `status`/`updated`, or a documented apiPath special-case; slice is
   strictly ascending). Per the SUMMARY's own framing.

`ExpectedIndexesFor` itself stays exported as the documented contract
anchor (per the doc comment in `main.go`) — only the call sites that
pretended to assert something were removed. Test passes.

## Verification

| Check | Result |
|---|---|
| `go test -race -count=1 ./deploy/grafana/...` | ok (1.031s) |
| `go test -race -count=1 ./cmd/pdb-schema-generate/...` | ok (1.245s) |
| `golangci-lint run ./...` (whole repo) | `0 issues.` |

## Out of scope (Info findings deferred)

The following INFO findings were not addressed in this auto-mode pass
(critical+warning scope only). They are intentionally low-priority polish
and would be addressed under `--all` mode if requested:

- **IN-01** — `slicesEqual` reinvents `slices.Equal` from stdlib (CLAUDE.md
  GO-MD-1 nit; works correctly today).
- **IN-02** — `process_group` template variable populated from primary-only
  metric (`pdbplus_sync_peak_heap_bytes`); reviewer flagged for the next
  dashboard revision, no immediate action.
- **IN-03** — `filepath.Clean` comment overstates path-traversal containment;
  reword the canonical comment in five visbaseline call sites.
- **IN-04** — Per-entity sub-test reads `schema/peeringdb.json` via
  cwd-relative path; correct under `go test ./...` but fragile under
  alternative test runners.

---

_Fixed: 2026-04-26T23:30:00Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
