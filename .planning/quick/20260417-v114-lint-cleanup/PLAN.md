---
slug: v114-lint-cleanup
type: quick
created: 2026-04-17
---

# Quick Task: v1.14 Post-Ship Lint Cleanup

## Objective

Bring `golangci-lint run ./...` to zero issues after v1.14 ship. 7 issues surfaced:

| Linter | Count | Files |
|--------|-------|-------|
| gosec G304 | 3 | `internal/visbaseline/redactcli.go`, `reportcli.go` (x2) |
| exhaustive | 1 | `internal/visbaseline/reportcli.go` |
| nolintlint | 2 | `internal/visbaseline/reportcli.go`, `internal/middleware/privacy_tier.go` |
| revive (unused-parameter) | 1 | `graph/custom.resolvers.go` |

5 were baselined as tracked debt in Phase 58's `deferred-items.md`; 1 was new v1.14 (privacy_tier nolint); 1 was pre-existing generated-code hygiene (graph resolver).

## Strategy

Mechanical file edits. No behaviour change.

1. `reportcli.go` — add `shapeUnknown` case to exhaustive switch (specific error message).
2. `reportcli.go` — remove unused `//nolint:gosec` on pf.path ReadFile (gosec already silent there); add `//nolint:gosec // G304` on two os.OpenFile calls with outDir-derived paths.
3. `redactcli.go` — add `//nolint:gosec // G304` on anonPath ReadFile.
4. `privacy_tier.go` — remove unused `//nolint:exhaustive`; switch is exhaustive over the 2-value enum, panic-after-switch retained as future-proof guard.
5. `custom.resolvers.go` — rename unused `ctx` to `_` in `ObjectCounts` (data is already on `obj`, no lookup needed).

## Verification

- `go build ./...` clean
- `go vet ./...` clean
- `golangci-lint run ./...` exits with `0 issues.`
- `go test -race ./internal/visbaseline/... ./internal/middleware/... ./graph/...` green
