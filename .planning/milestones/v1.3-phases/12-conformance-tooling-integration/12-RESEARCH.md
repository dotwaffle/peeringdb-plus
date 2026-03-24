# Phase 12: Conformance Tooling Integration - Research

**Researched:** 2026-03-24
**Domain:** CLI flag wiring, HTTP auth header injection, test env-var integration
**Confidence:** HIGH

## Summary

Phase 12 is a straightforward wiring phase. The conformance CLI (`cmd/pdbcompat-check/main.go`) and the live integration test (`internal/conformance/live_test.go`) both make direct `http.Client` requests to PeeringDB (they do NOT use the `internal/peeringdb.Client` from Phase 11). The work is injecting the `Authorization: Api-Key <key>` header into these existing request sites and adding the `--api-key` flag to the CLI.

Both tools currently hardcode a 3-second inter-request sleep to respect PeeringDB's unauthenticated rate limit of 20 req/min. The CONTEXT.md decision says the CLI keeps its current delay unchanged, while the test reduces from 3s to 1s when authenticated (since authenticated allows 60 req/min = 1 req/s).

**Primary recommendation:** Add the `--api-key` flag to the CLI's `runConfig` struct, read `PDBPLUS_PEERINGDB_API_KEY` as fallback, pass the key through to `checkType`, and set the auth header. In the live test, read the env var at test start, conditionally set the header and sleep duration. No new dependencies needed.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- CLI Tool (pdbcompat-check):
  - Add `--api-key` flag that takes precedence over `PDBPLUS_PEERINGDB_API_KEY` env var
  - When key is present, set `Authorization: Api-Key <key>` header on all PeeringDB requests
  - CLI makes raw http.Client requests (does NOT use the peeringdb.Client) -- header injection is in checkType() function
  - No rate limit change in CLI -- it already has manual inter-request delays
- Live Integration Test:
  - Read `PDBPLUS_PEERINGDB_API_KEY` env var in TestLiveConformance
  - Set auth header on requests when key is present
  - Reduce inter-request sleep from 3s to 1s when authenticated (faster test runs)
  - Keep 3s sleep when unauthenticated (current behavior)
  - Test continues to be gated by `-peeringdb-live` flag

### Claude's Discretion
- Whether the CLI should also adjust its rate limiting when a key is present (recommendation: no, keep simple)
- Error message format when CLI encounters auth rejection

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| CONFORM-01 | The `pdbcompat-check` CLI accepts an API key flag or env var and uses it for PeeringDB requests | CLI flag/env var pattern is standard Go `flag` package usage. Header set at line 124 of main.go. Key is passed through `runConfig` struct to `checkType` function. |
| CONFORM-02 | The `-peeringdb-live` integration test uses the API key when available for higher rate limits | Env var read via `os.Getenv` in `TestLiveConformance`. Sleep duration becomes conditional. Header injection mirrors CLI pattern at line 54 of live_test.go. |
</phase_requirements>

## Standard Stack

No new dependencies. This phase uses only what already exists:

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| flag (stdlib) | Go 1.26 | CLI flag parsing | Already used in `cmd/pdbcompat-check/main.go` for `--url`, `--type`, `--golden-dir`, `--timeout` |
| os (stdlib) | Go 1.26 | Environment variable reading | Already used in `internal/conformance/live_test.go` for golden dir lookup |
| net/http (stdlib) | Go 1.26 | HTTP client and request headers | Already used in both files for making PeeringDB requests |
| testing (stdlib) | Go 1.26 | Test framework | Already used in live_test.go |

**No `go get` or `go install` needed.**

## Architecture Patterns

### Current Code Structure (no changes to layout)
```
cmd/pdbcompat-check/
    main.go          # CLI tool -- modify runConfig, flag parsing, checkType
internal/conformance/
    compare.go       # Structural comparison (untouched)
    compare_test.go  # Unit tests for comparison (untouched)
    live_test.go     # Live integration test -- modify TestLiveConformance
```

### Pattern 1: Flag-with-Env-Fallback (CLI)
**What:** CLI flag takes precedence over env var, with empty string as default (unauthenticated)
**When to use:** When a value should be configurable both interactively and in CI/CD
**Example:**
```go
// In runConfig struct:
type runConfig struct {
    baseURL    string
    typeName   string
    goldenDir  string
    timeout    time.Duration
    apiKey     string
}

// In flag parsing:
flag.StringVar(&cfg.apiKey, "api-key", "", "PeeringDB API key (overrides PDBPLUS_PEERINGDB_API_KEY env var)")

// After flag.Parse(), env fallback:
if cfg.apiKey == "" {
    cfg.apiKey = os.Getenv("PDBPLUS_PEERINGDB_API_KEY")
}
```

### Pattern 2: Auth Header Injection
**What:** Set `Authorization: Api-Key <key>` header when key is non-empty
**When to use:** Both in CLI checkType and test request building
**Example:**
```go
req.Header.Set("User-Agent", "pdbcompat-check/1.0")
if apiKey != "" {
    req.Header.Set("Authorization", "Api-Key "+apiKey)
}
```
This matches the exact pattern used in `internal/peeringdb/client.go` line 257-259.

### Pattern 3: Conditional Sleep Duration (Test Only)
**What:** Use 1s inter-request sleep when authenticated, 3s when not
**When to use:** In TestLiveConformance loop
**Example:**
```go
apiKey := os.Getenv("PDBPLUS_PEERINGDB_API_KEY")
sleepDuration := 3 * time.Second
if apiKey != "" {
    sleepDuration = 1 * time.Second
}

// In loop:
if i > 0 {
    time.Sleep(sleepDuration)
}
```

### Anti-Patterns to Avoid
- **Using peeringdb.Client:** Both tools use raw `http.Client`. Do NOT refactor to use `peeringdb.NewClient` -- it adds OTel transport, rate limiter, retry logic, and pagination that the conformance tools don't need.
- **Changing CLI sleep timing:** The CONTEXT.md decision says no rate limit change in CLI. Keep the existing 3s sleep.
- **Logging the API key value:** Per SEC-2, never log secrets. Log that a key "is configured" or "is not configured", but never the key itself.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Flag parsing | Custom arg parser | `flag` stdlib | Already used; consistent with existing CLI |
| Env var reading | Config package loading | `os.Getenv` | CLI is standalone; config package is for server startup |
| Auth header format | Custom auth scheme | `"Api-Key " + key` literal | PeeringDB uses this exact format; matches peeringdb.Client |

**Key insight:** This phase is purely additive wiring. No new abstractions needed. Both integration points already exist and just need a conditional header set.

## Common Pitfalls

### Pitfall 1: Flag Precedence Over Env Var
**What goes wrong:** Env var overrides flag, or flag's zero value prevents env fallback
**Why it happens:** `flag.StringVar` with default "" means the flag is always "set" even when not provided
**How to avoid:** Check `cfg.apiKey == ""` after `flag.Parse()` to fall back to env var. An explicitly passed `--api-key ""` will also trigger env fallback, which is acceptable behavior.
**Warning signs:** CI sets env var but developer passes empty flag

### Pitfall 2: Auth Error Handling in CLI
**What goes wrong:** CLI gets 401/403 from PeeringDB when key is invalid, but error message is generic "HTTP 401"
**Why it happens:** Current `checkType` returns `fmt.Errorf("fetch %s: HTTP %d", typeName, resp.StatusCode)` for non-200
**How to avoid:** Add specific handling for 401/403 that mentions the API key may be invalid (mirrors peeringdb.Client pattern). Return a clear error without exposing the key value.
**Warning signs:** User provides wrong key and gets a confusing error

### Pitfall 3: Forgetting to Pass API Key Through Function Signatures
**What goes wrong:** `apiKey` is added to `runConfig` but `checkType` signature is not updated
**Why it happens:** `checkType` currently takes `client, baseURL, goldenDir, typeName` as separate params
**How to avoid:** Add `apiKey string` parameter to `checkType`, or pass the entire `runConfig` (but per CS-5, since it would be 5+ args, consider passing `runConfig` directly). The simpler approach: just add `apiKey` as a parameter since checkType already takes 5 params (ctx, client, baseURL, goldenDir, typeName), and one more is acceptable.
**Warning signs:** Compiles but never sends auth header

### Pitfall 4: Test Sleep Duration Too Aggressive
**What goes wrong:** 1s sleep with authenticated 60 req/min still triggers rate limiting
**Why it happens:** PeeringDB rate limit is per-minute rolling window, not per-second
**How to avoid:** 1s between requests = 60 req/min which exactly matches the authenticated limit. This is safe because the test only makes 13 requests (one per type). At 1 req/s, all 13 complete in ~13 seconds -- well within a 1-minute window.
**Warning signs:** Sporadic 429 responses in CI when env var is set

## Code Examples

### CLI: Complete runConfig Change
```go
// Source: cmd/pdbcompat-check/main.go (current line 29)
type runConfig struct {
    baseURL    string
    typeName   string
    goldenDir  string
    timeout    time.Duration
    apiKey     string  // NEW: PeeringDB API key
}
```

### CLI: Flag + Env Fallback
```go
// Source: cmd/pdbcompat-check/main.go (after current line 41)
flag.StringVar(&cfg.apiKey, "api-key", "", "PeeringDB API key (overrides PDBPLUS_PEERINGDB_API_KEY env var)")
flag.Parse()

// Env var fallback when flag not provided.
if cfg.apiKey == "" {
    cfg.apiKey = os.Getenv("PDBPLUS_PEERINGDB_API_KEY")
}
```

### CLI: Auth Header in checkType
```go
// Source: cmd/pdbcompat-check/main.go (after current line 124)
req.Header.Set("User-Agent", "pdbcompat-check/1.0")
if apiKey != "" {
    req.Header.Set("Authorization", "Api-Key "+apiKey)
}
```

### Test: Env Var + Conditional Sleep
```go
// Source: internal/conformance/live_test.go (around line 35)
apiKey := os.Getenv("PDBPLUS_PEERINGDB_API_KEY")
sleepDuration := 3 * time.Second
if apiKey != "" {
    sleepDuration = 1 * time.Second
    t.Log("using API key for authenticated access (1s sleep)")
} else {
    t.Log("no API key configured, using unauthenticated access (3s sleep)")
}
```

### Test: Auth Header in Request
```go
// Source: internal/conformance/live_test.go (after current line 54)
req.Header.Set("User-Agent", "pdbcompat-check-test/1.0")
if apiKey != "" {
    req.Header.Set("Authorization", "Api-Key "+apiKey)
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Unauthenticated PeeringDB access only | API key support via `peeringdb.WithAPIKey()` | Phase 11 (this milestone) | Conformance tools now need the same auth header support |

**No deprecated patterns apply.** The `Authorization: Api-Key <key>` header format is PeeringDB's current authentication mechanism.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing stdlib |
| Config file | None needed |
| Quick run command | `go test -race ./internal/conformance/ -run TestCompareStructure -count=1` |
| Full suite command | `go test -race ./...` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| CONFORM-01 | CLI accepts --api-key flag and reads PDBPLUS_PEERINGDB_API_KEY env var | unit | `go test -race ./cmd/pdbcompat-check/ -run TestAPIKeyFlag -count=1` | No -- Wave 0 |
| CONFORM-01 | CLI sets auth header when key present | unit | `go test -race ./cmd/pdbcompat-check/ -run TestAPIKeyHeader -count=1` | No -- Wave 0 |
| CONFORM-01 | CLI works without key (no auth header) | unit | `go test -race ./cmd/pdbcompat-check/ -run TestNoAPIKey -count=1` | No -- Wave 0 |
| CONFORM-02 | Live test reads env var and adjusts sleep | manual-only | `PDBPLUS_PEERINGDB_API_KEY=test go test -race ./internal/conformance/ -run TestLiveConformance -peeringdb-live -count=1` | Yes (live_test.go) but needs modification |

**Note on CONFORM-02 testing:** The live test by definition hits the real PeeringDB API and requires `-peeringdb-live` flag. It cannot be run in CI without network access and a valid key. The auth header addition can be verified by code review. The conditional sleep logic can be tested via a unit test that checks the sleep calculation without making network calls.

### Sampling Rate
- **Per task commit:** `go test -race ./cmd/pdbcompat-check/ -count=1`
- **Per wave merge:** `go test -race ./...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `cmd/pdbcompat-check/main_test.go` -- covers CONFORM-01 (flag parsing, header injection, env var fallback)
- [ ] Unit test for conditional sleep logic (optional, since the sleep values are trivial constants)

Note: `cmd/pdbcompat-check` currently has NO test files at all. Adding basic tests for the flag/header logic provides regression coverage. The tests should use `httptest.NewServer` to verify the auth header is sent correctly.

## Open Questions

1. **Auth rejection error format (Claude's Discretion)**
   - What we know: The CLI's `checkType` currently returns `fmt.Errorf("fetch %s: HTTP %d", typeName, resp.StatusCode)` for any non-200
   - Recommendation: Add a specific check for 401/403 that returns `fmt.Errorf("fetch %s: HTTP %d (API key may be invalid)", typeName, resp.StatusCode)` -- mirrors the message in `peeringdb.Client.doWithRetry` at line 285
   - This is simple, consistent with Phase 11's pattern, and helps users debug misconfiguration

2. **CLI rate limit adjustment (Claude's Discretion)**
   - What we know: CONTEXT.md recommends "no, keep simple"
   - Recommendation: Agree with CONTEXT.md. The CLI only makes 13 requests max (one per type) with 3s delays. Even at 39 seconds total, faster isn't meaningfully better for a debugging tool. Keep it simple.

## Sources

### Primary (HIGH confidence)
- `cmd/pdbcompat-check/main.go` -- current CLI implementation, line-by-line analysis
- `internal/conformance/live_test.go` -- current test implementation, line-by-line analysis
- `internal/peeringdb/client.go` -- Phase 11 auth header pattern (lines 257-259), auth error handling (lines 280-289)
- `internal/config/config.go` -- env var name `PDBPLUS_PEERINGDB_API_KEY` (line 84)
- `12-CONTEXT.md` -- locked decisions from discussion phase

### Secondary (MEDIUM confidence)
- PeeringDB API rate limits (20 req/min unauth, 60 req/min auth) -- from Phase 11 research and confirmed in client.go

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- no new dependencies, all stdlib
- Architecture: HIGH -- direct code inspection of both files, clear injection points identified
- Pitfalls: HIGH -- well-understood Go patterns, codebase already has the auth pattern from Phase 11

**Research date:** 2026-03-24
**Valid until:** indefinite (stdlib patterns, no external dependency versioning)
