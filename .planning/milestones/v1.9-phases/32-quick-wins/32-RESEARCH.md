# Phase 32: Quick Wins - Research

**Researched:** 2026-03-26
**Domain:** HTTP middleware ordering, structured error logging (Go slog)
**Confidence:** HIGH

## Summary

This phase addresses two independent, low-risk improvements: reordering the HTTP middleware chain so CORS handles OPTIONS preflight requests before OTel tracing creates spans, and replacing all `slog.String("error", err.Error())` calls with `slog.Any("error", err)` to preserve structured error type information through the log pipeline.

Both changes are mechanical. The middleware reorder is a single line swap in `cmd/peeringdb-plus/main.go`. The slog fix is a 90-instance find-and-replace across 6 files. No API surface changes, no behavioral changes to request handling, no new dependencies.

**Primary recommendation:** Execute as two independent tasks -- middleware reorder first (1 file, verifiable with a test), slog fix second (6 files, verifiable with grep).

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **Current chain**: Recovery -> OTel -> Logging -> CORS -> Readiness (in `cmd/peeringdb-plus/main.go:354-360`)
- **New chain**: Recovery -> CORS -> OTel -> Logging -> Readiness
- CORS before OTel: preflights short-circuit before tracing. Recovery still outermost for panic safety.
- This means OPTIONS requests hit CORS, get response, never reach OTel/Logging/Readiness.
- ~90 instances of `slog.String("error", err.Error())` across web handlers
- Replace with `slog.Any("error", err)` -- preserves structured error info through OTel pipeline
- Keep the attribute key name as `"error"` (don't rename to `"err"`)
- Primary locations: `internal/web/detail.go`, `internal/web/handler.go`, `internal/web/about.go`, `internal/web/search.go`
- Also check: `internal/web/compare.go`, `internal/sync/`, `cmd/`

### Scope Boundaries (Locked)
- Do NOT change logging levels or add new log calls
- Do NOT refactor middleware internals -- just reorder the chain in main.go
- Do NOT touch any handler logic -- this is purely a find-and-replace on slog patterns

### Claude's Discretion
None specified.

### Deferred Ideas (OUT OF SCOPE)
None specified.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| ARCH-03 | CORS middleware runs before OTel tracing in the middleware chain so OPTIONS preflight requests are not traced/logged | Middleware chain reorder in main.go lines 354-360. rs/cors v1.11.1 confirmed to short-circuit OPTIONS without calling next handler. |
| QUAL-02 | All error logging uses `slog.Any("error", err)` instead of `slog.String("error", err.Error())` | 90 instances across 6 files identified. slog.Any preserves error as KindAny, enabling OTel bridge and custom handlers to access full error chain. |
</phase_requirements>

## Architecture Patterns

### Current Middleware Chain (main.go:354-360)

```go
// Build middleware stack (outermost first):
// Recovery -> OTel HTTP -> Logging -> CORS -> Readiness -> mux
handler := readinessMiddleware(syncWorker, mux)
handler = middleware.CORS(middleware.CORSInput{AllowedOrigins: cfg.CORSOrigins})(handler)
handler = middleware.Logging(logger)(handler)
handler = otelhttp.NewMiddleware("peeringdb-plus")(handler)
handler = middleware.Recovery(logger)(handler)
```

Request flow: Recovery -> OTel -> Logging -> CORS -> Readiness -> mux

**Problem:** OPTIONS preflight requests pass through OTel (creating a span) and Logging (emitting a log line) before reaching CORS, which short-circuits them. This creates noise in traces and logs.

### Target Middleware Chain

```go
// Build middleware stack (outermost first):
// Recovery -> CORS -> OTel HTTP -> Logging -> Readiness -> mux
handler := readinessMiddleware(syncWorker, mux)
handler = middleware.Logging(logger)(handler)
handler = otelhttp.NewMiddleware("peeringdb-plus")(handler)
handler = middleware.CORS(middleware.CORSInput{AllowedOrigins: cfg.CORSOrigins})(handler)
handler = middleware.Recovery(logger)(handler)
```

Request flow: Recovery -> CORS -> OTel -> Logging -> Readiness -> mux

**Why this works:** `rs/cors` v1.11.1 short-circuits preflight OPTIONS requests by default (`optionPassthrough` is `false`). When CORS receives an OPTIONS request with proper `Origin` and `Access-Control-Request-Method` headers, it writes the CORS response headers, sets the status code (204 by default), and returns **without calling the next handler**. This means OTel, Logging, Readiness, and the mux are never invoked for preflights.

### Why Recovery Stays Outermost

Recovery must remain the outermost middleware. If CORS (or any downstream middleware) panics, Recovery catches it and returns a 500 instead of crashing the process. The CORS library is well-tested and unlikely to panic, but defense-in-depth requires Recovery to wrap everything.

### slog.Any vs slog.String for Errors

```go
// Before: loses error type information
slog.String("error", err.Error())  // stores string, KindString

// After: preserves error chain
slog.Any("error", err)             // stores error interface, KindAny
```

**How slog.Any handles errors:**
- `slog.AnyValue(err)` creates a `slog.Value` of `KindAny` containing the original error interface
- Built-in handlers (JSONHandler, TextHandler) call `.Error()` for display -- output is identical
- The OTel slog bridge can access the original error value, preserving type information
- Custom log handlers can use `errors.Is`/`errors.As` on the preserved error chain
- **No visible behavior change** in log output -- this is a type-fidelity improvement

**Confidence:** HIGH -- this is documented slog behavior (pkg.go.dev/log/slog#Any, pkg.go.dev/log/slog#AnyValue).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| CORS preflight handling | Custom OPTIONS detection | `rs/cors` short-circuit behavior | Library already handles all CORS preflight edge cases (malformed Origin, missing headers, credentials) |

## Common Pitfalls

### Pitfall 1: CORS After OTel Means Non-Preflight OPTIONS Still Traced

**What goes wrong:** Only actual CORS preflight requests (OPTIONS with `Origin` + `Access-Control-Request-Method`) are short-circuited by `rs/cors`. A bare `OPTIONS` request without CORS headers passes through to the next handler.
**Why it happens:** `rs/cors` distinguishes preflight from non-preflight OPTIONS.
**How to avoid:** This is fine -- bare OPTIONS requests reaching the mux are legitimate HTTP requests that should be traced and logged. Only browser-generated CORS preflights should be filtered.
**Warning signs:** None -- this is correct behavior.

### Pitfall 2: Variable Names in slog Replacement

**What goes wrong:** Not all instances use `err` as the variable name. Some use `cursorErr`, `stepErr`, `rbErr`, `facErr`.
**Why it happens:** Different error variables in different scopes.
**How to avoid:** The replacement must match the full pattern `slog.String("error", <varname>.Error())` and replace with `slog.Any("error", <varname>)`, preserving the variable name. Do not blindly replace `err.Error()` with `err`.
**Warning signs:** Compilation errors if the variable name is wrong.

### Pitfall 3: Middleware Chain Order Is Bottom-Up in Code

**What goes wrong:** The middleware wrapping code reads bottom-to-top. The last `handler = ...` line is the outermost middleware (first to execute). Swapping lines incorrectly reverses the intended order.
**Why it happens:** Middleware wrapping is nested function composition: `f(g(h(handler)))` means f runs first.
**How to avoid:** The variable assignment chain in main.go builds from innermost (first line) to outermost (last line). To move CORS before OTel, the CORS wrap line must appear **after** the OTel wrap line in the code (making it the outer layer).
**Warning signs:** Test showing OTel spans still created for OPTIONS requests.

## Code Examples

### Middleware Reorder (main.go:354-360)

```go
// Before:
handler := readinessMiddleware(syncWorker, mux)
handler = middleware.CORS(middleware.CORSInput{AllowedOrigins: cfg.CORSOrigins})(handler)
handler = middleware.Logging(logger)(handler)
handler = otelhttp.NewMiddleware("peeringdb-plus")(handler)
handler = middleware.Recovery(logger)(handler)

// After:
handler := readinessMiddleware(syncWorker, mux)
handler = middleware.Logging(logger)(handler)
handler = otelhttp.NewMiddleware("peeringdb-plus")(handler)
handler = middleware.CORS(middleware.CORSInput{AllowedOrigins: cfg.CORSOrigins})(handler)
handler = middleware.Recovery(logger)(handler)
```

### slog Error Fix

```go
// Before (all 90 instances follow one of these patterns):
slog.String("error", err.Error())
slog.String("error", cursorErr.Error())
slog.String("error", stepErr.Error())
slog.String("error", rbErr.Error())
slog.String("error", facErr.Error())

// After:
slog.Any("error", err)
slog.Any("error", cursorErr)
slog.Any("error", stepErr)
slog.Any("error", rbErr)
slog.Any("error", facErr)
```

## Exact Instance Inventory

Verified by grep across `cmd/` and `internal/` (excluding `.claude/worktrees/`):

| File | Count | Variable Names |
|------|-------|----------------|
| `cmd/peeringdb-plus/main.go` | 13 | `err` |
| `cmd/pdbcompat-check/main.go` | 1 | `err` |
| `internal/web/detail.go` | 62 | `err` (61), `facErr` (1) |
| `internal/web/handler.go` | 2 | `err` |
| `internal/web/about.go` | 1 | `err` |
| `internal/sync/worker.go` | 6 (`err`) + 4 (other) = 10 | `err`, `cursorErr`, `stepErr`, `rbErr` |
| **Total** | **89** (slog.String("error", err.Error())) + **1** (facErr) = **90** | |

Note: `internal/web/compare.go` and `internal/web/search.go` have zero instances -- confirmed clean.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) |
| Config file | none (standard go test) |
| Quick run command | `go test ./internal/middleware/ ./cmd/peeringdb-plus/ -race -count=1` |
| Full suite command | `go test -race ./...` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| ARCH-03 | OPTIONS preflight does not create OTel span or log line | integration | `go test ./internal/middleware/ -run TestCORSPreflight -race -count=1` | Existing cors_test.go tests preflight headers but does NOT verify OTel span absence -- needs new test or manual verification |
| QUAL-02 | All slog error calls use slog.Any | static analysis | `grep -r 'slog.String("error"' cmd/ internal/ \| grep -c '.Error()'` returns 0 | N/A (grep check, not test) |

### Sampling Rate
- **Per task commit:** `go test ./internal/middleware/ ./cmd/peeringdb-plus/ -race -count=1`
- **Per wave merge:** `go test -race ./...`
- **Phase gate:** Full suite green + grep confirms zero `slog.String("error"` instances

### Wave 0 Gaps
- Existing `cors_test.go` tests CORS headers but does not verify span suppression. A new integration test could verify that the middleware chain ordering prevents OTel spans for preflights, but this would require constructing a full middleware chain with a test OTel exporter. Given the scope boundaries ("do NOT refactor middleware internals"), a manual `curl -X OPTIONS` against a running server with trace export is the most appropriate verification.
- No new test files required for QUAL-02 -- grep is the appropriate verification tool.

## Project Constraints (from CLAUDE.md)

Relevant constraints for this phase:

| ID | Rule | Relevance |
|----|------|-----------|
| CS-0 | Use modern Go code guidelines | slog.Any is the modern pattern |
| OBS-1 | Structured logging (slog) with levels and consistent fields | slog.Any preserves structure better than slog.String |
| OBS-5 | When logging with attributes, prefer using attribute setters like slog.String() | slog.Any() is the correct setter for error types |
| ERR-1 | Wrap with %w and context | slog.Any preserves wrapped error chains |

## Sources

### Primary (HIGH confidence)
- `cmd/peeringdb-plus/main.go:354-360` -- current middleware chain, directly inspected
- `internal/middleware/cors.go` -- CORS middleware using rs/cors, directly inspected
- `internal/middleware/logging.go` -- Logging middleware, directly inspected
- `rs/cors` v1.11.1 source (github.com/rs/cors/cors.go) -- confirmed OPTIONS short-circuit behavior when `optionPassthrough` is false (default)
- `pkg.go.dev/log/slog#Any` -- slog.Any creates KindAny value preserving error interface
- `pkg.go.dev/log/slog#AnyValue` -- AnyValue stores non-primitive types as KindAny
- grep of 6 source files -- exact 90 instances verified

### Secondary (MEDIUM confidence)
- OTel slog bridge behavior with KindAny error values -- documented but not directly tested in this project

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- no new dependencies, pure code reorganization
- Architecture: HIGH -- middleware chain and slog behavior verified from source code and official documentation
- Pitfalls: HIGH -- all edge cases identified from direct code inspection (variable names, middleware ordering semantics)

**Research date:** 2026-03-26
**Valid until:** Indefinite -- this is project-internal code with no external dependency changes
