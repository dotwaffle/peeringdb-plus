# Phase 10: CI Pipeline & Public Access - Context

**Gathered:** 2026-03-23
**Status:** Ready for planning

<domain>
## Phase Boundary

Create a GitHub Actions CI pipeline with parallel lint, test, and build jobs. Add govulncheck security scanning, test coverage PR comments, and go generate drift detection. Verify and document the public access model.

</domain>

<decisions>
## Implementation Decisions

### CI Triggers & Go Version
- Triggers: PR + push to main
- Go version: latest stable only (not from go.mod, not matrix)
- Three parallel jobs: lint, test, build

### Test Job
- `go test -race ./...` with `CGO_ENABLED=1` (race detector requires CGo even though production build doesn't)
- Coverage output captured for PR comment

### Lint Job
- golangci-lint run (config from Phase 7's .golangci.yml)
- `go generate` then `git diff --exit-code` to catch schema drift (ent generate, gqlgen generate)

### Build Job
- `go build ./...` to verify compilation — no artifact upload

### Security & Coverage
- govulncheck: block on called vulns only (informational for unused)
- Coverage: custom shell script parsing `go test -cover` output, posted as PR comment via `gh api`
- No external services (no Codecov, no Coveralls)

### Public Access (PUB-01, PUB-02)
- PUB-01: Code review only — no integration test needed. All read endpoints are unauthenticated (verified by reading main.go)
- PUB-02: No separate documentation — root endpoint JSON (`/`) + `X-Powered-By` header already document the public API. POST /sync is an admin endpoint, out of scope for public access docs.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- No existing .github/ directory, Taskfile, or Makefile
- go.mod specifies Go 1.26.1
- All tests pass with `go test ./...`
- go vet already passes clean

### Established Patterns
- Config loaded from env vars (PDBPLUS_* prefix)
- POST /sync is the only auth-gated endpoint (X-Sync-Token)
- All other endpoints (GraphQL, REST, PeeringDB compat, health) are fully public

### Integration Points
- `.github/workflows/ci.yml` — new file
- golangci-lint config from Phase 7
- Golden file tests from Phase 9 will run in CI test job

</code_context>

<specifics>
## Specific Ideas

No specific requirements beyond what's captured in decisions.

</specifics>

<deferred>
## Deferred Ideas

- CI artifact upload (binary publication)
- Coverage trend tracking over time
- Matrix testing across multiple Go versions

</deferred>
