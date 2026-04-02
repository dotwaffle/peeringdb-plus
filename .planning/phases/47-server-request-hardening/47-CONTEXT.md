# Phase 47: Server & Request Hardening - Context

**Gathered:** 2026-04-02
**Status:** Ready for planning

<domain>
## Phase Boundary

Harden the HTTP server and request processing pipeline against connection exhaustion, oversized payloads, and invalid user inputs. All changes are server configuration or input validation — no new user-facing features.

</domain>

<decisions>
## Implementation Decisions

### HTTP Server Timeouts
- ReadHeaderTimeout: 10 seconds — protects against slowloris (slow header sending)
- IdleTimeout: 120 seconds — reaps idle keep-alive connections
- WriteTimeout: NOT set (0) — streaming RPCs (13 server-streaming endpoints) would be killed by a global write timeout. Per-stream timeouts already configured via PDBPLUS_STREAM_TIMEOUT (60s default).
- ReadTimeout: NOT set — would conflict with large GraphQL queries on slow connections

### SQLite Connection Pool
- MaxOpenConns: 10 — middle ground for SQLite with WAL mode (single-writer, concurrent readers)
- MaxIdleConns: 5 — half of max open, reduces idle connection overhead
- ConnMaxLifetime: 5 minutes — prevents stale connections
- These are hardcoded constants in database.go, NOT configurable via env vars (infrastructure constants, not user-tunable)

### Request Body Size Limits
- Limit: 1 MB (1 << 20 bytes) via http.MaxBytesReader
- Applied to POST endpoints: /graphql and POST /sync
- Returns 413 Payload Too Large on exceeded limit
- GraphQL queries rarely exceed 10 KB; 1 MB is generous

### Config Validation
- ListenAddr: validate contains ":" (basic format check)
- PeeringDBBaseURL: validate via net/url.Parse (must be valid URL)
- DrainTimeout: validate > 0 (already parsed as duration, add positivity check)
- Fail fast at startup with clear error messages per existing pattern

### ASN Input Validation
- Valid range: 0 < ASN < 4294967296 (32-bit unsigned)
- Out-of-range returns 400 Bad Request (not 404) — distinguishes invalid input from not-found
- Applied in handleNetworkDetail and any other ASN-accepting handlers
- strconv.Atoi already rejects non-numeric; add range check after parse

### Width Parameter Bounds
- Maximum: 500 columns
- Values > 500 silently capped to 500 (not an error — graceful degradation)
- Applied in render.go where ?w= is parsed (3 locations in renderPage)

### Claude's Discretion
- Exact error message wording for 400/413 responses
- Whether to log body-too-large events (probably yes, at WARN level)
- Whether to add the new config validations as separate functions or inline in validate()

</decisions>

<code_context>
## Existing Code Insights

### Key Files to Modify
- `cmd/peeringdb-plus/main.go:376-380` — http.Server creation (add timeouts)
- `internal/database/database.go:29-34` — sql.Open (add pool config after)
- `internal/config/config.go:140-151` — validate() method (add new checks)
- `internal/web/detail.go:45-49,202-206` — ASN parsing in handlers
- `internal/web/render.go:49-53,69-71,119-121` — width parameter parsing (3 locations)
- `cmd/peeringdb-plus/main.go:172,193-199` — POST endpoints needing body limits

### Established Patterns
- Config validation uses errors.New with descriptive messages, wrapped by caller
- Web handlers use handleNotFound/handleServerError for error responses
- httptest-based integration tests cover all web endpoints
- Middleware chain: Recovery -> CORS -> OTel HTTP -> Logging -> Readiness -> Caching -> mux

### Integration Points
- Body size limiting should be applied per-handler (MaxBytesReader wrapping r.Body), not as global middleware — different endpoints may need different limits
- ASN validation happens before ent query, so no database changes needed

</code_context>

<specifics>
## Specific Ideas

- Body size limit as a constant (e.g., `const maxRequestBodySize = 1 << 20`) in main.go near the handler registration
- Width capping should be silent (cap to 500, don't error) — terminal users shouldn't get errors for large values
- ASN error should use httperr.WriteProblem for consistency with existing RFC 9457 error responses

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>
