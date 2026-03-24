# Phase 11: API Key & Rate Limiting - Research

**Researched:** 2026-03-24
**Domain:** HTTP client authentication, rate limiting, configuration
**Confidence:** HIGH

## Summary

This phase adds optional PeeringDB API key authentication to the existing HTTP client with adjusted rate limits when authenticated. The scope is narrow and well-constrained: add an `apiKey` field to the Client struct via a functional option, inject the `Authorization: Api-Key <key>` header in `doWithRetry`, configure the rate limiter based on authentication state, and handle 401/403 responses with clear logging.

All required libraries are already in the project (`golang.org/x/time/rate`, `net/http`, `log/slog`). No new dependencies are needed. The codebase has established patterns for functional options (`WithSince`), rate limiting, configuration loading (`envOrDefault`), and test structure (table-driven tests with `httptest.NewServer`).

**Primary recommendation:** Follow the existing functional options pattern on `NewClient`, inject the auth header in `doWithRetry` after the User-Agent header, and configure the rate limiter at construction time based on whether an API key was provided. Add 401/403 to the non-retryable path in `doWithRetry` with a WARN log.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- Use functional options on NewClient: `NewClient(baseURL, logger, WithAPIKey(key))` -- consistent with existing WithSince pattern on FetchAll
- Store apiKey as a field on the Client struct
- Backward compatible: existing callers without WithAPIKey continue to work unchanged
- Header format: `Authorization: Api-Key <key>` (PeeringDB's documented format)
- Injected in `doWithRetry()` method (single authoritative location for all requests, line ~230)
- Conditional: only set if `c.apiKey != ""`
- Unauthenticated: 20 req/min (1 per 3 seconds) -- current default, unchanged
- Authenticated: 60 req/min (1 per second) -- 3x increase when API key is present
- Rate limiter configured in NewClient based on whether WithAPIKey was provided
- No dynamic rate limit switching -- set at client creation time
- Env var: `PDBPLUS_PEERINGDB_API_KEY` loaded in config.Load()
- Empty string = unauthenticated (current behavior preserved)
- Non-empty string = authenticated (header injected, rate limit increased)
- Passed to NewClient via WithAPIKey(cfg.PeeringDBAPIKey) in main.go
- NO upfront validation at startup -- no probe request
- When PeeringDB returns 401/403, log a clear WARN message with status code indicating the API key may be invalid
- Existing retry logic handles transient errors; auth errors should NOT be retried
- The error logging happens in the existing doWithRetry method
- API key NEVER logged in plaintext (SEC-2)
- Log as `api_key=[set]` or `api_key=[not set]` at startup

### Claude's Discretion
- Exact authenticated rate limit value (60 req/min specified, but implementation detail of how burst is configured)
- Whether 401/403 should abort the entire sync or just log and continue with next type

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| KEY-01 | Setting `PDBPLUS_PEERINGDB_API_KEY` causes all PeeringDB API requests to include `Authorization: Api-Key <key>` header | Header injection in doWithRetry after User-Agent; functional option WithAPIKey on NewClient; config field PeeringDBAPIKey loaded via envOrDefault |
| KEY-02 | When no API key is configured, sync and conformance tools work identically to current behavior (no auth header) | Conditional header: only set if `c.apiKey != ""`; NewClient without WithAPIKey produces identical behavior to current code |
| RATE-01 | When an API key is configured, the HTTP client rate limiter increases from 20 req/min to a higher authenticated threshold | rate.NewLimiter(rate.Every(1*time.Second), 1) for authenticated; configured at NewClient construction time |
| RATE-02 | When no API key is configured, the rate limiter remains at 20 req/min | Default path in NewClient unchanged: rate.NewLimiter(rate.Every(3*time.Second), 1) |
| VALIDATE-01 | When PeeringDB rejects the API key (401/403), the error is logged clearly with the status code and a message indicating the key may be invalid | Special handling in doWithRetry for 401/403: log WARN with status code and "API key may be invalid" message before returning error |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

Directives relevant to this phase:
- **CS-5 (MUST):** Use input structs for functions receiving more than 2 arguments. NewClient currently takes 2 args (baseURL, logger); adding variadic options keeps it under the threshold, so no input struct needed.
- **CS-6 (SHOULD):** Declare function input structs before the function consuming them. The ClientOption type should be declared before NewClient.
- **ERR-1 (MUST):** Wrap errors with `%w` and context.
- **SEC-2 (MUST):** Never log secrets.
- **OBS-1 (MUST):** Structured logging with slog and consistent fields.
- **OBS-5 (SHOULD):** Use attribute setters like `slog.String()` when logging.
- **CFG-1 (MUST):** Config via env/flags; validate on startup; fail fast.
- **CFG-2 (MUST):** Config immutable after init; pass explicitly.
- **T-1 (MUST):** Table-driven tests; deterministic and hermetic.
- **T-2 (MUST):** Run `-race` in CI; add `t.Cleanup` for teardown.
- **T-3 (SHOULD):** Mark safe tests with `t.Parallel()`.

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| golang.org/x/time/rate | v0.15.0 | Token-bucket rate limiter | Already in use. Provides `rate.Limiter` with `Wait(ctx)` method. Stdlib-adjacent, maintained by Go team. |
| net/http | Go 1.26 | HTTP client requests | Already in use for PeeringDB client. Header injection via `req.Header.Set`. |
| log/slog | Go 1.26 | Structured logging | Already in use. WARN level for auth errors per OBS-1. |

### Supporting
No new libraries needed. Everything required is already imported.

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Functional options on NewClient | Config struct parameter | Functional options are more extensible and already match the existing WithSince pattern. Config struct would require all callers to construct it even when no options needed. |
| rate.Limiter in NewClient | Dynamic rate switching | Adds complexity for no benefit. API key does not change at runtime. Set once at construction. |

## Architecture Patterns

### Recommended Change Structure
```
internal/config/config.go     -- Add PeeringDBAPIKey field + envOrDefault
internal/peeringdb/client.go  -- Add ClientOption type, WithAPIKey, apiKey field, header injection, 401/403 handling
cmd/peeringdb-plus/main.go    -- Wire WithAPIKey from config to client
```

### Pattern 1: Functional Options on NewClient
**What:** Add variadic `ClientOption` type to NewClient signature.
**When to use:** Adding optional configuration to existing constructors without breaking callers.
**Example:**
```go
// ClientOption configures optional Client behavior.
type ClientOption func(*Client)

// WithAPIKey sets the PeeringDB API key for authenticated requests.
// When set, all requests include the Authorization header and the
// rate limiter is increased from 20 req/min to 60 req/min.
func WithAPIKey(key string) ClientOption {
    return func(c *Client) {
        c.apiKey = key
    }
}

// NewClient creates a PeeringDB API client. Options are applied after
// defaults, so WithAPIKey can override the rate limiter.
func NewClient(baseURL string, logger *slog.Logger, opts ...ClientOption) *Client {
    c := &Client{
        http: &http.Client{
            Timeout:   30 * time.Second,
            Transport: otelhttp.NewTransport(http.DefaultTransport),
        },
        limiter:        rate.NewLimiter(rate.Every(3*time.Second), 1), // 20 req/min default
        baseURL:        baseURL,
        logger:         logger,
        retryBaseDelay: 2 * time.Second,
    }
    for _, opt := range opts {
        opt(c)
    }
    // Upgrade rate limit if API key is set.
    if c.apiKey != "" {
        c.limiter = rate.NewLimiter(rate.Every(1*time.Second), 1) // 60 req/min
    }
    return c
}
```

### Pattern 2: Header Injection in doWithRetry
**What:** Conditionally add Authorization header after User-Agent.
**When to use:** Single place for all outgoing request headers.
**Example:**
```go
req.Header.Set("User-Agent", userAgent)
if c.apiKey != "" {
    req.Header.Set("Authorization", "Api-Key "+c.apiKey)
}
```

### Pattern 3: Auth Error Handling (401/403)
**What:** Log WARN and return immediately without retry for auth errors.
**When to use:** Non-retryable errors that indicate configuration issues.
**Example:**
```go
// In doWithRetry, after checking for 2xx success and before retryable check:
if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
    c.logger.LogAttrs(ctx, slog.LevelWarn, "PeeringDB API key may be invalid",
        slog.Int("status", resp.StatusCode),
        slog.String("url", url),
    )
    authErr := fmt.Errorf("fetch %s: HTTP %d (API key may be invalid)", url, resp.StatusCode)
    attemptSpan.RecordError(authErr)
    attemptSpan.End()
    return nil, authErr
}
```

### Pattern 4: Config Loading
**What:** Add PeeringDBAPIKey to Config struct, loaded via envOrDefault.
**When to use:** Simple string config values with empty-string default.
**Example:**
```go
// In Config struct:
// PeeringDBAPIKey is the optional PeeringDB API key for authenticated access.
// Configured via PDBPLUS_PEERINGDB_API_KEY. Empty string means unauthenticated.
PeeringDBAPIKey string

// In Load():
PeeringDBAPIKey: envOrDefault("PDBPLUS_PEERINGDB_API_KEY", ""),
```

### Pattern 5: Startup Logging (SEC-2 Compliant)
**What:** Log API key presence without revealing the key value.
**When to use:** At startup, after config is loaded, before first sync.
**Example:**
```go
// In main.go after creating the PeeringDB client:
if cfg.PeeringDBAPIKey != "" {
    logger.Info("PeeringDB API key configured", slog.String("api_key", "[set]"))
} else {
    logger.Info("PeeringDB API key not configured, using unauthenticated access", slog.String("api_key", "[not set]"))
}
```

### Anti-Patterns to Avoid
- **Logging the API key value:** Violates SEC-2. Never log `c.apiKey` directly. Only log `[set]` / `[not set]`.
- **Retrying on 401/403:** Auth failures are configuration errors, not transient errors. Retrying wastes rate limit budget.
- **Dynamic rate limit switching:** The API key does not change at runtime. Setting rate limit at construction time is correct.
- **Startup probe request:** The CONTEXT.md explicitly forbids upfront validation. Invalid keys are detected on first real request.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Rate limiting | Custom sliding window | `golang.org/x/time/rate` | Already in use. Token bucket is correct for this use case. |
| HTTP header injection | Custom transport decorator | Direct `req.Header.Set` in doWithRetry | Simpler, more readable, and all header logic stays in one place. |

**Key insight:** This phase requires zero new dependencies. Every building block is already in the codebase.

## Common Pitfalls

### Pitfall 1: Accidentally Logging the API Key
**What goes wrong:** API key appears in plaintext in logs or error messages.
**Why it happens:** Using `slog.String("api_key", c.apiKey)` or including the key in error wrapping.
**How to avoid:** Only log `[set]` or `[not set]`. Never reference `c.apiKey` in log calls or `fmt.Errorf`.
**Warning signs:** Any slog call or fmt.Errorf that references `c.apiKey` directly.

### Pitfall 2: Retrying Auth Errors
**What goes wrong:** 401/403 responses are retried 3 times, wasting rate limit tokens.
**Why it happens:** The existing `isRetryable` function only checks for 429/5xx. 401/403 fall through to the "non-retryable error" path. This is already correct -- but the new code must add the WARN log BEFORE the non-retryable return.
**How to avoid:** Add the 401/403 check with logging before the `isRetryable` check in `doWithRetry`. Do not modify `isRetryable`.
**Warning signs:** If test shows 3 attempts for a 401 response, the ordering is wrong.

### Pitfall 3: Breaking Existing Callers
**What goes wrong:** Adding `opts ...ClientOption` to `NewClient` could break callers.
**Why it happens:** In Go, adding a variadic parameter to a function signature is backward-compatible. Existing calls like `NewClient(url, logger)` continue to compile without changes.
**How to avoid:** Variadic options are specifically designed for this. No pitfall here -- just confirming it works.
**Warning signs:** Compile errors in main.go or tests after changing the signature.

### Pitfall 4: Rate Limit Mismatch with PeeringDB
**What goes wrong:** Setting client rate limit higher than PeeringDB's actual limit causes 429 responses.
**Why it happens:** PeeringDB's documented authenticated limit is 40 req/min per user/org. The CONTEXT.md decision specifies 60 req/min.
**How to avoid:** The CONTEXT.md decision is locked at 60 req/min. This is a valid choice because: (a) the client has retry logic for 429, (b) the actual limit may vary, (c) being slightly aggressive and relying on retry is a pragmatic approach. The existing retry-on-429 code handles any overshoot gracefully.
**Warning signs:** Frequent 429 responses in production logs. If this happens, the rate can be tuned down.

### Pitfall 5: Rate Limiter Burst Configuration
**What goes wrong:** Setting burst > 1 allows request spikes that trigger PeeringDB's throttling.
**Why it happens:** Token bucket burst allows consuming multiple tokens at once.
**How to avoid:** Keep burst at 1 for both authenticated and unauthenticated. This ensures smooth, evenly-spaced requests.
**Warning signs:** Multiple requests sent within the same second despite rate limiting.

## Code Examples

### Full NewClient with Options (Verified pattern from codebase)
```go
// Source: internal/peeringdb/client.go (existing pattern extended)

// ClientOption configures optional Client behavior.
type ClientOption func(*Client)

// WithAPIKey sets the PeeringDB API key for authenticated requests.
// When set, requests include the Authorization header and the rate
// limiter increases to 60 req/min.
func WithAPIKey(key string) ClientOption {
    return func(c *Client) {
        c.apiKey = key
    }
}

func NewClient(baseURL string, logger *slog.Logger, opts ...ClientOption) *Client {
    c := &Client{
        http: &http.Client{
            Timeout:   30 * time.Second,
            Transport: otelhttp.NewTransport(http.DefaultTransport),
        },
        limiter:        rate.NewLimiter(rate.Every(3*time.Second), 1),
        baseURL:        baseURL,
        logger:         logger,
        retryBaseDelay: 2 * time.Second,
    }
    for _, opt := range opts {
        opt(c)
    }
    if c.apiKey != "" {
        c.limiter = rate.NewLimiter(rate.Every(1*time.Second), 1)
    }
    return c
}
```

### Header Injection in doWithRetry
```go
// Source: internal/peeringdb/client.go line ~235 (after User-Agent)
req.Header.Set("User-Agent", userAgent)
if c.apiKey != "" {
    req.Header.Set("Authorization", "Api-Key "+c.apiKey)
}
```

### Auth Error Handling in doWithRetry
```go
// Source: internal/peeringdb/client.go (new block before isRetryable check)
// Auth errors indicate invalid API key -- log and fail immediately.
if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
    _, _ = io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    c.logger.LogAttrs(ctx, slog.LevelWarn, "PeeringDB API key may be invalid",
        slog.Int("status", resp.StatusCode),
        slog.String("url", url),
    )
    authErr := fmt.Errorf("fetch %s: HTTP %d (API key may be invalid)", url, resp.StatusCode)
    attemptSpan.RecordError(authErr)
    attemptSpan.End()
    return nil, authErr
}
```

### Config Addition
```go
// Source: internal/config/config.go

// In Config struct:
// PeeringDBAPIKey is the optional PeeringDB API key for authenticated access.
// Configured via PDBPLUS_PEERINGDB_API_KEY. Empty string means unauthenticated.
PeeringDBAPIKey string

// In Load() cfg initialization:
PeeringDBAPIKey: envOrDefault("PDBPLUS_PEERINGDB_API_KEY", ""),
```

### main.go Wiring
```go
// Source: cmd/peeringdb-plus/main.go line ~117

// Build client options.
var clientOpts []peeringdb.ClientOption
if cfg.PeeringDBAPIKey != "" {
    clientOpts = append(clientOpts, peeringdb.WithAPIKey(cfg.PeeringDBAPIKey))
    logger.Info("PeeringDB API key configured", slog.String("api_key", "[set]"))
} else {
    logger.Info("PeeringDB API key not configured, using unauthenticated access",
        slog.String("api_key", "[not set]"))
}
pdbClient := peeringdb.NewClient(cfg.PeeringDBBaseURL, logger, clientOpts...)
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| PeeringDB Basic Auth (username/password) | API Key auth (`Authorization: Api-Key <key>`) | ~2022 | API keys are preferred for programmatic access; basic auth still works but API keys are recommended |
| Silent downgrade on invalid auth | 401 response on invalid API key | PeeringDB v2.43.0 (2023) | Invalid keys now produce clear 401 responses instead of silently falling back to anonymous access |

**PeeringDB Rate Limits (verified from official docs):**
- Anonymous: 20 req/min per IP address
- Authenticated: 40 req/min per user/org (official docs say 40, CONTEXT.md decision says 60)
- Additional throttle: repeated identical anonymous requests above 100KB limited to 1/hour
- Rate limit exceeded returns 429 (already handled by existing retry logic)

**Note on 60 vs 40 req/min:** The CONTEXT.md locks the authenticated rate at 60 req/min. PeeringDB's current documented limit is 40 req/min. The 60 req/min decision is acceptable because: the existing retry-on-429 logic handles any overshoot, and PeeringDB limits may have changed since their docs were last updated. The worst case is occasional 429s that are retried successfully.

## Open Questions

1. **Should 401/403 abort the entire sync or just fail the current object type?**
   - What we know: The error propagates up from `doWithRetry` -> `FetchAll` -> `FetchType` -> sync worker. The sync worker iterates over object types.
   - What's unclear: Whether the sync worker should skip to the next type or abort entirely.
   - Recommendation: Abort the entire sync. A 401/403 means the API key is invalid for ALL requests, not just one type. Continuing would waste rate limit tokens and produce misleading partial results. The error message should be clear enough for operators to fix the key.

2. **Burst configuration for authenticated rate limiter**
   - What we know: Current unauthenticated limiter uses burst=1. CONTEXT.md says burst config is Claude's discretion.
   - Recommendation: Keep burst=1 for authenticated too. Burst=1 ensures evenly-spaced requests, which is friendliest to PeeringDB's rate limiting. No benefit to bursting since sync is sequential.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing stdlib |
| Config file | None (Go convention) |
| Quick run command | `go test ./internal/peeringdb/... ./internal/config/... -count=1 -race` |
| Full suite command | `go test ./... -count=1 -race` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| KEY-01 | Auth header sent when API key set | unit | `go test ./internal/peeringdb/... -run TestWithAPIKeyHeader -count=1 -race` | Will be added |
| KEY-02 | No auth header when no API key | unit | `go test ./internal/peeringdb/... -run TestNoAPIKeyNoHeader -count=1 -race` | Will be added |
| RATE-01 | Rate limiter uses 60 req/min when authenticated | unit | `go test ./internal/peeringdb/... -run TestAuthenticatedRateLimit -count=1 -race` | Will be added |
| RATE-02 | Rate limiter remains 20 req/min when unauthenticated | unit | `go test ./internal/peeringdb/... -run TestUnauthenticatedRateLimit -count=1 -race` | Partially covered by existing TestFetchAllRateLimiter |
| VALIDATE-01 | 401/403 logged with clear message, not retried | unit | `go test ./internal/peeringdb/... -run TestAuthErrorHandling -count=1 -race` | Will be added |

### Sampling Rate
- **Per task commit:** `go test ./internal/peeringdb/... ./internal/config/... -count=1 -race`
- **Per wave merge:** `go test ./... -count=1 -race`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
None -- existing test infrastructure covers all phase requirements. Tests will be added alongside implementation code in the same files (`client_test.go`, `config_test.go`).

## Sources

### Primary (HIGH confidence)
- Codebase: `internal/peeringdb/client.go` -- current Client implementation, rate limiter, doWithRetry
- Codebase: `internal/config/config.go` -- Config struct, Load(), envOrDefault pattern
- Codebase: `cmd/peeringdb-plus/main.go` -- client wiring at line 117
- Codebase: `internal/peeringdb/client_test.go` -- existing test patterns with httptest.NewServer
- [PeeringDB Auth Docs](https://docs.peeringdb.com/howto/authenticate/) -- `Authorization: Api-Key <key>` format confirmed
- [PeeringDB API Key HOWTO](https://docs.peeringdb.com/howto/api_keys/) -- API key creation and usage
- [PeeringDB Query Limits](https://docs.peeringdb.com/howto/work_within_peeringdbs_query_limits/) -- 20/min anonymous, 40/min authenticated

### Secondary (MEDIUM confidence)
- [PeeringDB Issue #1220](https://github.com/peeringdb/peeringdb/issues/1220) -- 401 response on invalid API key (since v2.43.0)
- [PeeringDB Issue #1126](https://github.com/peeringdb/peeringdb/issues/1126) -- Throttling framework details

### Tertiary (LOW confidence)
- Web search results suggested 60/min authenticated -- this may be outdated or from a different source. Official docs say 40/min. The CONTEXT.md decision of 60/min is locked regardless.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all libraries already in use, no new deps
- Architecture: HIGH -- all patterns verified against existing codebase, changes are minimal and well-constrained
- Pitfalls: HIGH -- based on direct code reading and PeeringDB API behavior verification

**Research date:** 2026-03-24
**Valid until:** 2026-04-24 (stable -- no fast-moving dependencies)
