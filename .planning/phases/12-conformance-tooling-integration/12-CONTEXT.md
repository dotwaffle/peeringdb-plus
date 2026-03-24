# Phase 12: Conformance Tooling Integration - Context

**Gathered:** 2026-03-24
**Status:** Ready for planning

<domain>
## Phase Boundary

Wire API key support from Phase 11 into the conformance CLI tool and live integration test. Both tools should use the API key when available for authenticated PeeringDB access with higher rate limits. Both must continue working without a key.

</domain>

<decisions>
## Implementation Decisions

### CLI Tool (pdbcompat-check)
- Add `--api-key` flag that takes precedence over `PDBPLUS_PEERINGDB_API_KEY` env var
- When key is present, set `Authorization: Api-Key <key>` header on all PeeringDB requests
- CLI makes raw http.Client requests (does NOT use the peeringdb.Client) — header injection is in checkType() function
- No rate limit change in CLI — it already has manual inter-request delays

### Live Integration Test
- Read `PDBPLUS_PEERINGDB_API_KEY` env var in TestLiveConformance
- Set auth header on requests when key is present
- Reduce inter-request sleep from 3s to 1s when authenticated (faster test runs)
- Keep 3s sleep when unauthenticated (current behavior)
- Test continues to be gated by `-peeringdb-live` flag

### Claude's Discretion
- Whether the CLI should also adjust its rate limiting when a key is present (recommendation: no, keep simple)
- Error message format when CLI encounters auth rejection

</decisions>

<code_context>
## Existing Code Insights

### Key Files
- `cmd/pdbcompat-check/main.go` — runConfig struct at line 28, flag parsing at line 38, checkType at line 116, request headers at line 124
- `internal/conformance/live_test.go` — flag at line 18, http.Client at line 35, request headers at line 54, sleep at line 41

### Integration Points
- `cmd/pdbcompat-check/main.go` line 38 region: add `--api-key` flag definition
- `cmd/pdbcompat-check/main.go` line 124: add auth header after User-Agent
- `internal/conformance/live_test.go` line 35 region: read env var for API key
- `internal/conformance/live_test.go` line 41: conditional sleep (1s vs 3s)
- `internal/conformance/live_test.go` line 54: add auth header after User-Agent

</code_context>

<specifics>
## Specific Ideas

No specific references — straightforward flag/env var wiring.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>
