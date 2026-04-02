# Phase 50: CI & Linting - Context

**Gathered:** 2026-04-02
**Status:** Ready for planning

<domain>
## Phase Boundary

Enable additional linters to catch more defect classes and add Docker build validation to the CI pipeline. Pure CI/tooling changes — no application code changes except lint fixes.

</domain>

<decisions>
## Implementation Decisions

### Additional Linters
- Enable 3 new linters in .golangci.yml: exhaustive, contextcheck, gosec
- All 3 must pass cleanly on the full codebase (after Phase 49 refactoring)
- Order of enablement: enable all 3 at once in config, then fix any violations
- For contextcheck false positives: add targeted //nolint:contextcheck annotations with explanatory comments on intentional patterns (fire-and-forget sync goroutine in main.go, background context in metrics callbacks)
- For gosec false positives: assess after first run — SQLite DSN construction may trigger G304
- For exhaustive: ensure all type switches on custom enum types have complete cases
- Existing linters remain: standard preset + gocritic, misspell, nolintlint, revive

### Docker Build in CI
- Add a new job to .github/workflows/ci.yml: `docker-build`
- Build both Dockerfiles: `docker build -f Dockerfile .` and `docker build -f Dockerfile.prod .`
- Build only — no push, no smoke test, no container startup
- Job runs in parallel with existing lint, test, build, govulncheck jobs
- Uses docker/setup-buildx-action for layer caching
- Fails the CI pipeline if either Dockerfile fails to build

### Claude's Discretion
- Exact placement of //nolint annotations (line-level vs block-level)
- Whether to add specific gosec rule exclusions (e.g., G304) in .golangci.yml rather than per-line nolint
- Docker build job caching strategy (GitHub Actions cache vs no cache)
- Whether to add the docker-build job as a required check or optional

</decisions>

<code_context>
## Existing Code Insights

### Key Files to Modify
- `.golangci.yml` (19 lines) — add exhaustive, contextcheck, gosec to enable list
- `.github/workflows/ci.yml` (91 lines) — add docker-build job
- Various source files — fix lint violations found by new linters
- `cmd/peeringdb-plus/main.go` — likely needs //nolint:contextcheck on sync goroutine

### Established Patterns
- .golangci.yml uses v2 format with `default: standard` preset
- Generated code excluded via `generated: strict`
- gosec already excluded from test files (line 16-18)
- CI jobs use actions/checkout@v6, actions/setup-go@v6 with go-version: stable
- All CI jobs run in parallel (lint, test, build, govulncheck)

### Integration Points
- Linter additions may surface violations in code modified in Phases 47-49
- Phase 49 refactoring must complete before linters run — otherwise lint fixes apply to pre-refactored code that will change
- Docker build job is independent of all other jobs (can run in parallel)

</code_context>

<specifics>
## Specific Ideas

- Run golangci-lint locally first to assess the full violation list before committing config changes
- Consider using `--new-from-merge-base` in CI for incremental linting, but the config change itself needs a full pass
- Docker build job should be lightweight — no Go setup needed, just docker buildx

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>
