# Stack Research

**Domain:** Go test coverage improvement
**Researched:** 2026-03-26
**Confidence:** HIGH

## Recommended Stack

### Core Technologies

Already in place — no new core dependencies needed for test coverage work.

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| `testing` (stdlib) | Go 1.26 | Test framework | Already used. Table-driven tests, -race flag, b.Loop() benchmarks. |
| `entgo.io/ent/enttest` | v0.14.5 | Ent test helpers | Already used. Auto-migration SQLite test databases. |
| `net/http/httptest` | Go 1.26 | HTTP test server/recorder | Already used. Required for web handler and GraphQL tests. |
| `connectrpc.com/connect` | latest | ConnectRPC test clients | Already in use. Connect client for gRPC handler integration tests. |

### Supporting Libraries

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `go tool cover` (stdlib) | Go 1.26 | Coverage profiling and reporting | Already available. `-coverprofile` flag for per-function coverage. |
| `go tool covdata` (stdlib) | Go 1.26 | Coverage data merging and filtering | Use to merge profiles and exclude generated code directories from reporting. |

### Development Tools

| Tool | Purpose | Notes |
|------|---------|-------|
| `gotestsum` | Test runner with better output | Optional. Provides JUnit XML for CI, colored output, rerun failures. Already listed in project tooling (TL-3). |
| `go test -coverprofile` | Per-function coverage analysis | Use `go tool cover -func=coverage.out` to identify specific uncovered functions. |

## What NOT to Add

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| `testify/assert` | Project uses stdlib assertions consistently. Mixing assertion styles creates inconsistency. | `if got != want { t.Errorf(...) }` pattern already established. |
| `gomock` / `mockgen` | Project tests against real SQLite via enttest. Mocking the ORM would test implementation details, not behavior. | Real enttest clients with in-memory SQLite. |
| `go-mutesting` | Mutation testing is expensive to run and Go's ecosystem tool is unmaintained (last release 2020). | Manual test quality review during dedicated phase. |
| `testcontainers-go` | No Docker dependencies in this project. SQLite runs in-process. | enttest with `sqlite://file:test?mode=memory` |
| Coverage badge services | Adds external dependency for vanity metric. Generated code distorts numbers. | Per-function `go tool cover` analysis in CI. |

## Coverage Reporting Strategy

### Problem: Generated Code Distorts Metrics

| Package | Total Lines | Generated Lines | Hand-Written Lines | Reported Coverage |
|---------|-------------|-----------------|--------------------|--------------------|
| `graph` | ~57,700 | ~57,144 (`generated.go`) | ~556 | 2.6% |
| `ent/*` | ~50,000+ | ~50,000+ | ~500 (schema) | 0-47% |
| `gen/*` | ~15,000+ | ~15,000+ | 0 | 0% |

### Solution: Filter Coverage by Package

```bash
# Run coverage for hand-written packages only
go test -coverprofile=coverage.out \
  ./internal/... \
  ./graph/... \
  ./ent/schema/...

# Per-function analysis excluding generated files
go tool cover -func=coverage.out | grep -v generated | grep -v _templ.go
```

No new tooling needed — stdlib `go tool cover` with grep filtering is sufficient.

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| stdlib assertions | testify/assert | Only if team unanimously prefers it. Not worth mixing styles. |
| Real SQLite (enttest) | SQL mocks (go-sqlmock) | Never for this project — real DB catches schema/migration issues. |
| Manual test review | Mutation testing (go-mutesting) | If test quality is still suspect after manual review. |
| go tool cover filtering | codecov/coveralls | If team wants hosted coverage tracking with PR comments. |

## Sources

- [Go 1.26 testing docs](https://pkg.go.dev/testing) — b.Loop(), coverage profiling
- [go tool covdata](https://pkg.go.dev/cmd/covdata) — Coverage data merging
- [gotestsum](https://github.com/gotestyourself/gotestsum) — Test runner with JUnit output
- [enttest docs](https://entgo.io/docs/testing/) — Ent test helpers

---
*Stack research for: Go test coverage improvement*
*Researched: 2026-03-26*
