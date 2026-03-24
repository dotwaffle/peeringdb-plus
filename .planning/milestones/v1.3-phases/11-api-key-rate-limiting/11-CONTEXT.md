# Phase 11: API Key & Rate Limiting - Context

**Gathered:** 2026-03-24
**Status:** Ready for planning

<domain>
## Phase Boundary

Wire optional PeeringDB API key into the HTTP client with `Authorization: Api-Key <key>` header injection, increase rate limiter from 20 req/min to 60 req/min when authenticated, and provide clear error logging on auth rejection. All existing unauthenticated behavior must be preserved when no key is configured.

</domain>

<decisions>
## Implementation Decisions

### Client Constructor Pattern
- Use functional options on NewClient: `NewClient(baseURL, logger, WithAPIKey(key))` — consistent with existing WithSince pattern on FetchAll
- Store apiKey as a field on the Client struct
- Backward compatible: existing callers without WithAPIKey continue to work unchanged

### API Key Header
- Header format: `Authorization: Api-Key <key>` (PeeringDB's documented format)
- Injected in `doWithRetry()` method (single authoritative location for all requests, line ~230)
- Conditional: only set if `c.apiKey != ""`

### Rate Limiting
- Unauthenticated: 20 req/min (1 per 3 seconds) — current default, unchanged
- Authenticated: 60 req/min (1 per second) — 3x increase when API key is present
- Rate limiter configured in NewClient based on whether WithAPIKey was provided
- No dynamic rate limit switching — set at client creation time

### Configuration
- Env var: `PDBPLUS_PEERINGDB_API_KEY` loaded in config.Load()
- Empty string = unauthenticated (current behavior preserved)
- Non-empty string = authenticated (header injected, rate limit increased)
- Passed to NewClient via WithAPIKey(cfg.PeeringDBAPIKey) in main.go

### Validation / Error Handling
- NO upfront validation at startup — no probe request
- When PeeringDB returns 401/403, log a clear WARN message with status code indicating the API key may be invalid
- Existing retry logic handles transient errors; auth errors should NOT be retried
- The error logging happens in the existing doWithRetry method

### Log Masking (SEC-2)
- API key NEVER logged in plaintext
- Log as `api_key=[set]` or `api_key=[not set]` at startup
- No API key value in request-level logging

### Claude's Discretion
- Exact authenticated rate limit value (60 req/min specified, but implementation detail of how burst is configured)
- Whether 401/403 should abort the entire sync or just log and continue with next type

</decisions>

<code_context>
## Existing Code Insights

### Key Files
- `internal/config/config.go` — Config struct at line 24, Load() at line 67, env var pattern via envOrDefault()
- `internal/peeringdb/client.go` — Client struct at line 32, NewClient at line 43, rate limiter at line 50, doWithRetry at line 216, request headers at line 230
- `cmd/peeringdb-plus/main.go` — Client creation at line 117

### Reusable Patterns
- `envOrDefault()` for config loading (line 138)
- Functional options already exist for FetchAll (WithSince at line 64-69)
- `SetRateLimit()` method exists for testing (line 310)
- OTel transport wrapping already in place (line 47)

### Integration Points
- main.go line 117: `pdbClient := peeringdb.NewClient(cfg.PeeringDBBaseURL, logger)` — add WithAPIKey option
- doWithRetry line 230: `req.Header.Set("User-Agent", ...)` — add auth header after this

</code_context>

<specifics>
## Specific Ideas

No specific references — standard HTTP auth header injection pattern.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>
