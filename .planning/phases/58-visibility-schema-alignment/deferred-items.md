# Phase 58 Deferred Items

Out-of-scope findings encountered during Phase 58 execution. Not fixed in this
phase per executor scope-boundary rules; logged here for future triage.

## Pre-existing golangci-lint issues in `internal/visbaseline`

`golangci-lint run ./internal/visbaseline/...` on the base commit
(`60bf023382ab56a86067b1ee6b20b6a11bc09e4f`) — before any Phase 58 changes —
already reports 5 issues unrelated to the Phase 58 regression test:

```
internal/visbaseline/reportcli.go:70:2:   exhaustive — missing cases in switch of type visbaseline.shape: visbaseline.shapeUnknown
internal/visbaseline/redactcli.go:97:21:  gosec G304 — os.ReadFile(anonPath) — path from CLI caller
internal/visbaseline/reportcli.go:422:12: gosec G304 — os.OpenFile(jsonPath, ...) — path from CLI caller
internal/visbaseline/reportcli.go:436:12: gosec G304 — os.OpenFile(p, ...) — path from CLI caller
internal/visbaseline/reportcli.go:387:36: nolintlint — unused //nolint:gosec directive
```

These are tracked debt in the Phase 57 CLI helpers. They neither block the
Phase 58 regression test (which lints clean in isolation) nor change
behaviour. Fix in a dedicated cleanup pass — not in Phase 58's scope (the
scope is schema alignment, not CLI hygiene).
