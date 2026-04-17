---
slug: v114-lint-cleanup
type: quick
created: 2026-04-17
completed: 2026-04-17
status: complete
---

# Quick Task Summary: v1.14 Post-Ship Lint Cleanup

## Status: complete — golangci-lint clean (0 issues)

## Changes

| File | Change | Issue fixed |
|------|--------|-------------|
| `internal/visbaseline/reportcli.go` | Added `case shapeUnknown:` with descriptive error | exhaustive |
| `internal/visbaseline/reportcli.go` | Removed unused `//nolint:gosec` on pf.path ReadFile | nolintlint |
| `internal/visbaseline/reportcli.go` | Added `//nolint:gosec // G304` to jsonPath + p OpenFile calls | gosec G304 ×2 |
| `internal/visbaseline/redactcli.go` | Added `//nolint:gosec // G304` to anonPath ReadFile | gosec G304 |
| `internal/middleware/privacy_tier.go` | Removed unused `//nolint:exhaustive` (switch already exhaustive over 2-value enum) | nolintlint |
| `graph/custom.resolvers.go` | `ObjectCounts(ctx, obj)` → `ObjectCounts(_, obj)` (ctx unused; data is on obj) | revive |

## Verification

```
go build ./... → clean
go vet ./... → clean
golangci-lint run ./... → 0 issues.
go test -race ./internal/visbaseline/... ./internal/middleware/... ./graph/... → PASS
```

Full test suite re-run on changed packages; no behaviour change.

## Resolves

Phase 58 `deferred-items.md` is now empty of open issues. No follow-up planned.
