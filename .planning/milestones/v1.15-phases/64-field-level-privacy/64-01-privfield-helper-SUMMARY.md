---
phase: 64-field-level-privacy
plan: 01
subsystem: privacy
tags: [privacy, visibility, helper-package, privfield, privctx]

# Dependency graph
requires:
  - phase: 59-authenticated-user-tier
    provides: "internal/privctx.TierFrom/WithTier + Tier constants (TierPublic zero, TierUsers)"
provides:
  - "internal/privfield.Redact — serializer-layer field-level privacy primitive"
  - "Reusable helper pattern for all 5 API surfaces (pdbcompat, REST, GraphQL, ConnectRPC, Web UI)"
  - "Substrate for v1.16+ OAuth gated fields"
affects: [64-02-schema-sync-wiring, 64-03-serializer-redaction-e2e, future field-level gated fields]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Serializer-layer field-level redaction (distinct from ent Policy row-level redaction)"
    - "Fail-closed default inherits from privctx.TierFrom (un-stamped ctx → TierPublic → redact)"
    - "Non-generic string-typed visible parameter (avoids entgql/entrest/entproto codegen churn per v1.14 Key Decision)"

key-files:
  created:
    - internal/privfield/privfield.go
    - internal/privfield/privfield_test.go
    - internal/privfield/doc.go
  modified: []

key-decisions:
  - "Package doc lives on privfield.go (matches internal/privctx convention); doc.go is a placeholder-only file to avoid duplicating package godoc — revive's package-comments linter would flag duplicates."
  - "No refactor phase needed — RESEARCH.md §'`internal/privfield` Package Shape' locks the implementation verbatim; it is already minimal (single switch, no intermediate state)."
  - "Test file uses `package privfield_test` (external test package) to exercise only the exported API, confirming the zero-export-creep surface."

patterns-established:
  - "Field-level privacy gates compose privctx (row tier) with a per-field <field>_visible companion string; one helper call per gated field."
  - "Fail-closed-by-default: any unknown visible string redacts (not just 'Private'); combined with privctx's TierPublic zero-value, mis-plumbed ctx cannot leak."

requirements-completed: [VIS-08]

# Metrics
duration: 2min
completed: 2026-04-18
---

# Phase 64 Plan 01: privfield Helper Summary

**Serializer-layer field-level privacy primitive — `privfield.Redact(ctx, visible, value)` — composing `privctx.TierFrom` with per-field `_visible` companions, fail-closed by default, with 11 admission-matrix unit tests.**

## Performance

- **Duration:** ~2 min
- **Started:** 2026-04-18T03:39:07Z
- **Completed:** 2026-04-18T03:40:56Z
- **Tasks:** 1 (TDD RED + GREEN)
- **Files created:** 3

## Accomplishments

- New `internal/privfield` package with exactly one exported function, `Redact(ctx context.Context, visible, value string) (string, bool)`.
- Full D-11 truth table covered by 11 table-driven sub-tests (TierPublic × 5 visible values + TierUsers × 4 + 2 un-stamped-ctx edge cases).
- Fail-closed behaviour (D-03) locked by `unstamped-ctx-fail-closed` sub-test: `context.Background()` + `visible="Users"` ⇒ omit.
- Implementation is stdlib + `internal/privctx` only — no new `go.mod` dependencies.
- Substrate ready for Plan 64-03 to wire into all 5 API surfaces and for v1.16+ OAuth gated-field additions.

## Task Commits

TDD cycle produced two commits (no refactor needed):

1. **RED — failing test harness** — `d521a8c` (test)
   - `test(64-01): add failing tests for privfield.Redact admission matrix`
   - Adds `internal/privfield/privfield_test.go` with 11 table-driven sub-tests covering the D-11 truth table + D-03 fail-closed case. Fails at compile time because `privfield` has no non-test files yet.

2. **GREEN — minimal implementation** — `17dc27d` (feat)
   - `feat(64-01): implement privfield.Redact for field-level privacy gating`
   - Adds `internal/privfield/privfield.go` (Redact + admission-rule godoc) and `internal/privfield/doc.go` (placeholder, package doc remains on privfield.go). All 11 sub-tests pass under `-race`; `go vet`, `golangci-lint run`, `govulncheck` all clean.

**Plan metadata commit:** pending (this SUMMARY.md commit below).

## Exported Surface

Exactly one exported symbol:

```go
func Redact(ctx context.Context, visible, value string) (out string, omit bool)
```

Contract (verbatim from Phase 64 CONTEXT.md and RESEARCH.md §"`internal/privfield` Package Shape"):

- `visible == "Public"` → return `(value, false)` regardless of tier.
- `visible == "Users"` && `tier >= TierUsers` → return `(value, false)`.
- `visible == "Users"` && `tier == TierPublic` → return `("", true)` — the gated case.
- `visible == "Private"` → return `("", true)` in all tiers (upstream parity).
- Any unrecognised `visible` string → return `("", true)` (fail-closed).
- Un-stamped ctx inherits `privctx.TierFrom`'s `TierPublic` fallback — no extra check in `Redact`.

## Unit Test Truth Table

| # | ctx tier | visible | want out | want omit |
|---|----------|---------|----------|-----------|
| 1 | TierPublic | `"Public"` | value | false |
| 2 | TierPublic | `"Users"` | `""` | true |
| 3 | TierPublic | `"Private"` | `""` | true |
| 4 | TierPublic | `""` | `""` | true |
| 5 | TierPublic | `"garbage"` | `""` | true |
| 6 | TierUsers  | `"Public"` | value | false |
| 7 | TierUsers  | `"Users"` | value | false |
| 8 | TierUsers  | `"Private"` | `""` | true |
| 9 | TierUsers  | `""` | `""` | true |
| 10 | unstamped ctx | `"Users"` | `""` | true (fail-closed per D-03) |
| 11 | unstamped ctx | `"Public"` | value | false |

Test output:

```
ok  	github.com/dotwaffle/peeringdb-plus/internal/privfield	1.014s

--- PASS: TestRedact (0.00s)
    --- PASS: TestRedact/public-tier-visible-public (0.00s)
    --- PASS: TestRedact/public-tier-visible-users (0.00s)
    --- PASS: TestRedact/public-tier-visible-private (0.00s)
    --- PASS: TestRedact/public-tier-visible-empty (0.00s)
    --- PASS: TestRedact/public-tier-visible-garbage (0.00s)
    --- PASS: TestRedact/users-tier-visible-public (0.00s)
    --- PASS: TestRedact/users-tier-visible-users (0.00s)
    --- PASS: TestRedact/users-tier-visible-private (0.00s)
    --- PASS: TestRedact/users-tier-visible-empty (0.00s)
    --- PASS: TestRedact/unstamped-ctx-fail-closed (0.00s)
    --- PASS: TestRedact/unstamped-ctx-public-admits (0.00s)
PASS
```

## Files Created/Modified

- **Created** `internal/privfield/privfield.go` — Redact function, package-level godoc documenting all admission rules + fail-closed semantics. 51 lines including license/blank lines, single `case`/`default` switch on `visible`.
- **Created** `internal/privfield/privfield_test.go` — 57 lines. Table-driven `TestRedact` under `package privfield_test` (external test package → validates the exported-only surface). Each sub-test runs `t.Parallel()`.
- **Created** `internal/privfield/doc.go` — 4-line placeholder. Not a `// Package …` doc comment (package doc remains on privfield.go to avoid revive `package-comments` duplicate warning); serves future accretion per the plan's hint.

## Decisions Made

- **doc.go kept as a non-package-doc placeholder.** The plan listed `doc.go` as a must-have artifact but also said "If the project convention is to keep package doc on the main file (check `ls internal/privctx/` — if no doc.go, follow that convention and skip doc.go). Either way is acceptable". `internal/privctx/` has no `doc.go`. I kept `doc.go` present (to satisfy the artifact list) but stripped the `// Package privfield …` comment so package doc lives exclusively on `privfield.go`. This avoids `revive`'s `package-comments` rule, which flags duplicate package-level doc comments.
- **No refactor phase.** RESEARCH.md §"`internal/privfield` Package Shape" locks the implementation verbatim. The code is already minimal (stdlib + privctx, single switch). Any refactor would drift from the locked RESEARCH shape without benefit.
- **External test package (`package privfield_test`).** Ensures tests exercise only the exported API — verifies the "one exported function" discipline from the start and prevents accidental reliance on unexported helpers.

## Deviations from Plan

None of consequence.

One minor convention call (doc.go without a package doc comment) is documented above under **Decisions Made**. It satisfies both the artifact list in `must_haves` and the plan's explicit "either way is acceptable" guidance.

## Issues Encountered

- **Worktree base drift on startup.** The worktree initially checked out at an earlier commit (`3f4c8ad`, pre-Phase-63) and the required `.planning/phases/64-field-level-privacy/` tree plus `internal/privctx/` were absent. Ran `git reset --hard 18bfe65662b91fc3e49148b7a70a907f233d12bb` per the `<worktree_branch_check>` protocol. After reset, both Phase 64 planning docs and `internal/privctx/` were present at the expected revisions. No impact on deliverable.

## Known Stubs

None. `privfield.Redact` is a pure function with no stubbed data paths. Callers wire data in Plan 64-02 (schema/sync) and Plan 64-03 (serializers).

## Threat Flags

None. No new network endpoints, auth paths, file access, or trust-boundary schema changes. The package is a pure function over `(context.Context, string, string)` with no I/O.

## Verification (Success Criteria Evidence)

- [x] `TMPDIR=/tmp/claude-1000 go test -race -count=1 ./internal/privfield/...` → `ok …/privfield	1.014s` with 11/11 sub-tests PASS.
- [x] `TMPDIR=/tmp/claude-1000 go vet ./internal/privfield/...` → clean.
- [x] `TMPDIR=/tmp/claude-1000 golangci-lint run ./internal/privfield/...` → `0 issues.`
- [x] `TMPDIR=/tmp/claude-1000 govulncheck ./internal/privfield/...` → `No vulnerabilities found.`
- [x] `grep -n "^func " internal/privfield/privfield.go` → exactly one line: `func Redact(...)`.
- [x] Package decls: 2× `package privfield` (privfield.go, doc.go) + 1× `package privfield_test` (test file).
- [x] No new `go.mod` entries (only stdlib `context` + `github.com/dotwaffle/peeringdb-plus/internal/privctx`).

## Next Phase Readiness

Plan 64-02 (schema + sync wiring) can proceed independently — no dependency on this package. Plan 64-03 (serializer redaction + 5-surface E2E) now has its primitive: import `"github.com/dotwaffle/peeringdb-plus/internal/privfield"` and call `privfield.Redact(ctx, row.IxfIxpMemberListURLVisible, row.IxfIxpMemberListURL)` at each serializer's field-assembly site; when `omit=true`, drop the JSON key entirely (pair with `json:",omitempty"` for struct-tag surfaces; `delete(m, key)` for map-based surfaces; leave the proto wrapper nil for ConnectRPC).

## Self-Check: PASSED

- `internal/privfield/privfield.go` exists (FOUND).
- `internal/privfield/privfield_test.go` exists (FOUND).
- `internal/privfield/doc.go` exists (FOUND).
- Commit `d521a8c` (RED) present in `git log --oneline` (FOUND).
- Commit `17dc27d` (GREEN) present in `git log --oneline` (FOUND).
- TDD gate sequence: `test(64-01): …` precedes `feat(64-01): …` (FOUND — matches type: tdd gate policy).

---
*Phase: 64-field-level-privacy*
*Plan: 01-privfield-helper*
*Completed: 2026-04-18*
