# Phase 32 Context: Quick Wins

## Requirements
- **ARCH-03**: CORS middleware runs before OTel tracing so OPTIONS preflights are not traced/logged
- **QUAL-02**: All error logging uses `slog.Any("error", err)` instead of `slog.String("error", err.Error())`

## Decisions

### Middleware Reorder
- **Current chain**: Recovery -> OTel -> Logging -> CORS -> Readiness (in `cmd/peeringdb-plus/main.go:354-360`)
- **New chain**: Recovery -> CORS -> OTel -> Logging -> Readiness
- CORS before OTel: preflights short-circuit before tracing. Recovery still outermost for panic safety.
- This means OPTIONS requests hit CORS, get response, never reach OTel/Logging/Readiness.

### slog Error Fix
- ~90 instances of `slog.String("error", err.Error())` across web handlers
- Replace with `slog.Any("error", err)` — preserves structured error info through OTel pipeline
- Keep the attribute key name as `"error"` (don't rename to `"err"`)
- Primary locations: `internal/web/detail.go`, `internal/web/handler.go`, `internal/web/about.go`, `internal/web/search.go`
- Also check: `internal/web/compare.go`, `internal/sync/`, `cmd/`

## Scope Boundaries
- Do NOT change logging levels or add new log calls
- Do NOT refactor middleware internals — just reorder the chain in main.go
- Do NOT touch any handler logic — this is purely a find-and-replace on slog patterns
