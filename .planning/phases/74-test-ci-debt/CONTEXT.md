---
phase: 74
slug: test-ci-debt
milestone: v1.18.0
status: context-locked
has_context: true
locked_at: 2026-04-26
---

# Phase 74 Context: Test & CI Debt

## Goal

Clear three deferred test failures and the 5 pre-existing golangci-lint issues in `internal/visbaseline` so CI stays green without `-skip` flags or stale acceptance.

## Requirements

- **TEST-01** — `cmd/pdb-schema-generate/TestGenerateIndexes` passes against current main
- **TEST-02** — `deploy/grafana/dashboard_test.go` `TestDashboard_RegionVariableUsed` passes
- **TEST-03** — `golangci-lint run ./internal/visbaseline/...` returns `0 issues`

## Locked decisions

- **D-01 — TEST-01 fix: auto-derive expected indexes from schema source.** Replace the static allow-list in `TestGenerateIndexes` with one that reads `schema/peeringdb.json` (and any entgo schema declarations) to derive the expected index set per entity. Test compares actual generator output against the derived set. Future indexes added via the schema source pass without test edits; accidentally-created bad indexes (typos, duplicates, indexes on non-existent fields) still fail. This is the most rigorous of the three options considered — user explicitly picked it over allow-list extension or deny-list inversion. Acceptable cost: one-time investment in the derivation logic; long-term zero-touch as the schema evolves.

- **D-02 — TEST-02 fix: drop the `$region` template variable entirely.** No PromQL in the dashboard references `fly_region` after the post-260426-lod migration to `cloud_region`. The variable is dead UI cruft. Remove it from `deploy/grafana/dashboards/pdbplus-overview.json`, then update `TestDashboard_RegionVariableUsed` to either (a) be removed entirely if no `$region` exists to test against, or (b) flip into a positive test asserting that no orphan template variables exist (every declared variable must drive at least one panel query). User picked (b) implicitly via "drop the variable" — extending the test to catch future orphans is the natural follow-through.

- **D-03 — TEST-03 fix: mix proper validation + targeted nolint.** Per-finding decision:
  - **exhaustive (`reportcli.go:70` switch on `shape` missing `shapeUnknown` case)** — fix properly: add the missing case with a sensible default (likely `return "unknown"` or a fail-fast error). Real bug surface: an unhandled enum value would silently fall through.
  - **gosec G304 × 3 (`os.ReadFile(anonPath)`, `os.OpenFile(jsonPath)`, `os.OpenFile(p)`)** — these are CLI tools. `visbaseline` is operator-supplied-paths by design. Apply `filepath.Clean()` to the path before use (handles `..` traversal), then add `//nolint:gosec // visbaseline is a CLI tool — paths are operator-supplied by contract`. This is the "mix" answer: the validation step is genuinely useful (G304 catches naive `..` traversal), the nolint is honest about why the rest of G304 doesn't apply.
  - **nolintlint (`reportcli.go:387` unused `//nolint:gosec` directive)** — remove the directive. It's stale.

## Out of scope

- Adding a CI job that periodically re-runs gosec / golangci-lint with stricter rules — current rules are fine; this phase only resolves existing findings.
- Migrating away from the `exhaustive` linter — keep the current linter set per `.golangci.yml`.
- Refactoring `internal/visbaseline` beyond the lint findings — the package is shipped tooling; touch only what's needed.

## Dependencies

- **Depends on**: None.
- **Enables**: CI drift gate stays green during planning of phases 75-78. No phase has a hard dep on this work, but it removes per-PR friction.

## Plan hints for executor

- Touchpoints:
  - `cmd/pdb-schema-generate/main_test.go` — replace `TestGenerateIndexes` allow-list with derived-from-source comparison
  - `cmd/pdb-schema-generate/main.go` — possibly export a helper that returns the expected index set (refactor for testability)
  - `deploy/grafana/dashboards/pdbplus-overview.json` — remove `$region` template variable from `templating.list` array
  - `deploy/grafana/dashboard_test.go:316` — replace `TestDashboard_RegionVariableUsed` with `TestDashboard_NoOrphanTemplateVars` (asserts every declared variable is referenced by at least one panel query)
  - `internal/visbaseline/reportcli.go:70` — add `shapeUnknown` case to switch
  - `internal/visbaseline/reportcli.go:387` — remove stale `//nolint:gosec` directive
  - `internal/visbaseline/redactcli.go:97`, `reportcli.go:422`, `reportcli.go:436` — `filepath.Clean` + targeted `//nolint:gosec // CLI tool, operator-supplied path`
- Verify on completion:
  - `go test ./cmd/pdb-schema-generate/...` clean
  - `go test ./deploy/grafana/...` clean
  - `golangci-lint run ./internal/visbaseline/...` returns `0 issues.`
  - `golangci-lint run ./...` (whole repo) returns `0 issues.`
  - CI drift gate green
