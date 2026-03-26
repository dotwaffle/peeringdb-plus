# Phase 40: Web Handler Coverage - Research

**Researched:** 2026-03-26
**Domain:** Go test coverage for web UI handler package (internal/web)
**Confidence:** HIGH

## Summary

Phase 40 targets three specific coverage gaps in `internal/web`: (1) integration tests for the 6 lazy-loaded fragment handlers named in WEB-01, (2) renderPage dispatch tests exercising terminal/JSON/WHOIS output modes on entity detail pages for WEB-02, and (3) edge case tests for `extractID`, `getFreshness`, and error response paths for WEB-03.

The package is currently at 74.8% coverage with 3,273 lines of test code across 6 test files. Substantial test infrastructure already exists -- `seedAllTestData` in `detail_test.go` creates all 13 PeeringDB entity types with deterministic IDs, and `TestFragments_AllTypes` already tests 14 of the fragment endpoints. The gaps are specific and narrow: (a) renderPage has only 41.8% coverage because terminal/JSON/WHOIS dispatch is only tested on the home page, not on entity detail pages where `Data != nil` hits different code paths; (b) `extractID` is at 37.5% with only the "net" case covered; (c) `getFreshness` is at 50% because the `db != nil` path is never tested (all handler tests pass `nil` for the db parameter); (d) `handleServerError` is at 0%; (e) `handleOrgCampusesFragment` and `handleOrgCarriersFragment` are at 0%.

**Primary recommendation:** Add ~3 focused test functions: one table-driven test for renderPage dispatch on a seeded entity detail page (curl/JSON/WHOIS modes), one for `extractID` edge cases, and one for `getFreshness` with a real sync_status table. The fragment handlers named in WEB-01 are already tested by `TestFragments_AllTypes` -- verify coverage and potentially add the missing org campuses/carriers fragments.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
None -- auto-generated infrastructure phase. All implementation choices at Claude's discretion.

### Claude's Discretion
All implementation choices are at Claude's discretion -- pure infrastructure phase. Use ROADMAP phase goal, success criteria, and codebase conventions to guide decisions.

### Deferred Ideas (OUT OF SCOPE)
None
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| WEB-01 | All 6 lazy-loaded fragment handlers have integration tests | The 6 named fragments (network IX presences, network facilities, IX networks, IX facilities, facility networks, org networks) ARE already tested in `TestFragments_AllTypes`. However, `handleOrgCampusesFragment` and `handleOrgCarriersFragment` have 0% coverage and should be added to complete the fragment test table. Seed data already includes all required entities. |
| WEB-02 | renderPage dispatch tested for terminal, JSON, and WHOIS output modes | `renderPage` is at 41.8% coverage. Terminal (curl UA) and JSON modes are tested on the home page only, which hits `RenderHelp` and `title: "Home"` paths. WHOIS mode is NOT tested at handler level at all. Entity detail pages with `Data != nil` exercise completely different renderPage branches (RenderPage, RenderJSON with data, RenderWHOIS). Need table-driven test on a seeded entity (e.g., `/ui/asn/13335`) with curl UA, `?format=json`, and `?format=whois` query params. |
| WEB-03 | Edge case coverage for extractID, getFreshness, and error paths | `extractID` at 37.5% (only "net" case covered by completion search tests). `getFreshness` at 50% (only `db == nil` path hit; no test creates sync_status table). `handleServerError` at 0%. Need: (a) `extractID` table test covering all 6 type slugs + unknown type + empty input, (b) `getFreshness` test with `testutil.SetupClientWithDB` + `sync.InitStatusTable` + inserted sync record, (c) 404 entity-not-found already tested, 500 path reachable indirectly. |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **T-1 (MUST)**: Table-driven tests; deterministic and hermetic by default
- **T-2 (MUST)**: Run `-race` in CI; add `t.Cleanup` for teardown
- **T-3 (SHOULD)**: Mark safe tests with `t.Parallel()`
- **CS-0 (MUST)**: Use modern Go code guidelines
- **CS-1 (MUST)**: Enforce `gofmt`, `go vet`
- Out of scope per REQUIREMENTS.md: no new test framework (testify, gomock) -- use stdlib assertions

## Current State Analysis

### Coverage Baseline
- **Package total**: 74.8% of statements
- **Target**: meaningful improvement driven by specific requirements (WEB-01, WEB-02, WEB-03)

### Per-Function Coverage (key gaps)

| Function | Current | File | Required By |
|----------|---------|------|-------------|
| renderPage | 41.8% | render.go | WEB-02 |
| extractID | 37.5% | completions.go | WEB-03 |
| getFreshness | 50.0% | detail.go | WEB-03 |
| handleServerError | 0.0% | handler.go | WEB-03 |
| handleOrgCampusesFragment | 0.0% | detail.go | WEB-01 (adjacent) |
| handleOrgCarriersFragment | 0.0% | detail.go | WEB-01 (adjacent) |
| handleAbout | 40.0% | about.go | -- |

### Already Well-Covered Areas

| Function/Area | Coverage | Notes |
|--------------|----------|-------|
| TestFragments_AllTypes | 14 fragments tested | Covers all 6 named in WEB-01 |
| TestDetailPages_AllTypes | 6 entity types | Full-page HTML rendering tested |
| TestDetailPages_NotFound | 12 cases | Invalid IDs + non-existent IDs |
| TestTerminalDetection | 12 cases | Home page dispatch for curl/wget/httpie/browser/json/plain |
| TestTerminal404JSON | 1 case | JSON 404 RFC 9457 compliance |

### Existing Test Infrastructure

| Component | Location | Purpose |
|-----------|----------|---------|
| `seedAllTestData` | detail_test.go:16 | Creates all 13 entity types with deterministic IDs |
| `setupAllTestMux` | detail_test.go:172 | Seeds data + creates mux in one call |
| `newTestMux` | handler_test.go:29 | Empty-database mux for tests not needing entities |
| `testutil.SetupClient` | testutil/testutil.go:27 | In-memory SQLite ent client with auto-cleanup |
| `testutil.SetupClientWithDB` | testutil/testutil.go:38 | Same + returns `*sql.DB` for raw SQL (needed for sync_status) |
| `testutil/seed.Full` | testutil/seed/seed.go:42 | Deterministic seed with same IDs as seedAllTestData |
| `renderComponent` | handler_test.go:19 | Renders templ component to string |
| `truncateBody` | handler_test.go:891 | Truncates body for error message context |

## Architecture Patterns

### Test Pattern: Handler Integration Test with Dispatch Modes

The established pattern for testing renderPage dispatch on real entities:

```go
func TestDetailPages_DispatchModes(t *testing.T) {
    t.Parallel()
    mux := setupAllTestMux(t) // seeds all entities with deterministic IDs

    tests := []struct {
        name        string
        path        string
        userAgent   string
        query       string // appended to path
        wantStatus  int
        wantCT      string   // Content-Type prefix
        wantContain []string // mode-specific markers
        wantAbsent  []string
    }{
        {
            name:        "terminal rich mode",
            path:        "/ui/asn/13335",
            userAgent:   "curl/8.5.0",
            wantStatus:  200,
            wantCT:      "text/plain",
            wantContain: []string{"\x1b[", "Cloudflare"}, // ANSI codes + entity name
        },
        {
            name:        "JSON mode",
            path:        "/ui/asn/13335?format=json",
            wantStatus:  200,
            wantCT:      "application/json",
            wantContain: []string{"{", "Cloudflare"}, // JSON braces
        },
        {
            name:        "WHOIS mode",
            path:        "/ui/asn/13335?format=whois",
            wantStatus:  200,
            wantCT:      "text/plain",
            wantContain: []string{":", "Cloudflare"}, // RPSL key-value colons
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
            url := tt.path
            if tt.query != "" {
                url += "?" + tt.query
            }
            req := httptest.NewRequest(http.MethodGet, url, nil)
            if tt.userAgent != "" {
                req.Header.Set("User-Agent", tt.userAgent)
            }
            rec := httptest.NewRecorder()
            mux.ServeHTTP(rec, req)
            // assert status, Content-Type, body contains/absent
        })
    }
}
```

### Test Pattern: getFreshness with Real sync_status

```go
func TestGetFreshness_WithSyncRecord(t *testing.T) {
    t.Parallel()
    client, db := testutil.SetupClientWithDB(t)
    ctx := context.Background()

    // Create sync_status table (not ent-managed).
    if err := sync.InitStatusTable(ctx, db); err != nil {
        t.Fatalf("init status table: %v", err)
    }

    // Insert a success record.
    id, err := sync.RecordSyncStart(ctx, db, time.Now().Add(-time.Hour))
    if err != nil {
        t.Fatalf("record sync start: %v", err)
    }
    _ = sync.RecordSyncComplete(ctx, db, id, sync.Status{
        LastSyncAt: time.Now().Add(-30 * time.Minute),
        Duration:   5 * time.Second,
        Status:     "success",
    })

    h := NewHandler(client, db) // pass real db, not nil
    freshness := h.getFreshness(ctx)
    if freshness.IsZero() {
        t.Error("expected non-zero freshness from sync record")
    }
}
```

### Test Pattern: extractID Edge Cases

```go
func TestExtractID(t *testing.T) {
    t.Parallel()
    tests := []struct {
        name      string
        detailURL string
        typeSlug  string
        want      string
    }{
        {"network", "/ui/asn/13335", "net", "13335"},
        {"ix", "/ui/ix/20", "ix", "20"},
        {"facility", "/ui/fac/30", "fac", "30"},
        {"org", "/ui/org/1", "org", "1"},
        {"campus", "/ui/campus/40", "campus", "40"},
        {"carrier", "/ui/carrier/50", "carrier", "50"},
        {"unknown type", "/ui/foo/1", "foo", ""},
        {"empty url", "", "net", ""},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
            got := extractID(tt.detailURL, tt.typeSlug)
            if got != tt.want {
                t.Errorf("extractID(%q, %q) = %q, want %q", tt.detailURL, tt.typeSlug, got, tt.want)
            }
        })
    }
}
```

### Anti-Patterns to Avoid
- **Testing templates instead of handlers:** Don't test templ component output directly for handler coverage -- test through the HTTP handler so renderPage dispatch is exercised
- **Not asserting mode-specific markers:** Asserting status 200 alone doesn't prove the right mode was used. Must check Content-Type AND mode-specific body markers (ANSI codes for terminal, `{` for JSON, RPSL keys for WHOIS)
- **Using `db == nil` everywhere:** Current tests always pass `nil` for `db`, which means getFreshness always returns zero time. Must use `SetupClientWithDB` for freshness tests

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Test database setup | Custom SQLite init | `testutil.SetupClient(t)` / `testutil.SetupClientWithDB(t)` | Handles unique DB names, cleanup, foreign keys |
| Test entity seeding | Per-test entity creation | `seedAllTestData(t, client)` / `setupAllTestMux(t)` | Already creates all 13 types with fixed IDs |
| sync_status table | Manual SQL CREATE | `sync.InitStatusTable(ctx, db)` | Already handles schema creation and error handling |
| WHOIS output verification | String parsing | Check for `:` key-value pairs | RPSL format uses `key: value` lines |

## Common Pitfalls

### Pitfall 1: Fragment Test vs renderPage Coverage
**What goes wrong:** Writing fragment endpoint tests (like TestFragments_AllTypes) doesn't increase renderPage coverage because fragments render directly via `templates.Component.Render()`, bypassing `renderPage` entirely.
**Why it happens:** Fragment handlers set `Content-Type: text/html` and render the component directly. Only full page handlers (handleNetworkDetail, handleIXDetail, etc.) call renderPage.
**How to avoid:** For WEB-02, test entity DETAIL page paths (/ui/asn/13335, /ui/ix/20) with different User-Agent/query params, not fragment endpoints.
**Warning signs:** renderPage coverage not increasing despite adding more handler tests.

### Pitfall 2: WHOIS Mode Markers
**What goes wrong:** Asserting generic text in WHOIS mode passes because all modes include the entity name.
**Why it happens:** Not checking for RPSL-specific formatting.
**How to avoid:** Assert RPSL-specific markers. The WHOIS renderer outputs key-value pairs with `:` separators and specific keys like `aut-num:`, `as-name:`, `descr:`. Check for at least one RPSL-style key.
**Warning signs:** Test passes with wrong mode because assertion is too generic.

### Pitfall 3: getFreshness Requires sync_status Table
**What goes wrong:** Test creates db via SetupClientWithDB but doesn't create the sync_status table, causing query errors that are silently swallowed.
**Why it happens:** sync_status is not an ent-managed table. It's created by `sync.InitStatusTable` which runs `CREATE TABLE IF NOT EXISTS`.
**How to avoid:** Always call `sync.InitStatusTable(ctx, db)` before testing getFreshness with a non-nil db.
**Warning signs:** getFreshness returns zero time even after inserting sync records.

### Pitfall 4: handleServerError 0% Coverage
**What goes wrong:** Trying to directly trigger handleServerError is difficult because it requires a database error in a handler path.
**Why it happens:** The error paths that call handleServerError (e.g., search error, render error) are hard to trigger with a valid in-memory SQLite database.
**How to avoid:** The simplest approach is to accept that handleServerError at 0% is hard to raise organically. It's a 4-line wrapper calling renderPage with a fixed template. Focus on paths that are achievable (404 error page is already tested). If coverage on this specific function is required, consider testing it indirectly via a context cancellation or by closing the client before making a request.
**Warning signs:** Overly complex test setup just to trigger a 500 error.

## Code Examples

### renderPage Mode Branches (from render.go)

The function has 7 branches based on `termrender.Detect()` result:

| Mode | Content-Type | Triggers | Code Path When Data Present |
|------|-------------|----------|---------------------------|
| ModeShort | text/plain | `?format=short` | `renderer.RenderShort(w, page.Data)` |
| ModeRich | text/plain | curl/wget UA or `Accept: text/plain` | `renderer.RenderPage(w, page.Title, page.Data)` |
| ModePlain | text/plain | `?T` or `?format=plain` | Same as ModeRich but noColor |
| ModeJSON | application/json | `?format=json` or `Accept: application/json` | `termrender.RenderJSON(w, page.Data)` |
| ModeWHOIS | text/plain | `?format=whois` | `renderer.RenderWHOIS(w, page.Title, page.Data)` |
| ModeHTMX | text/html | `HX-Request: true` | `page.Content.Render(ctx, w)` |
| ModeHTML | text/html | default (browser) | `templates.Layout(page.Title, page.Content).Render(ctx, w)` |

Currently tested on detail pages: ModeHTML (yes), ModeHTMX (yes). NOT tested on detail pages: ModeRich, ModePlain, ModeJSON, ModeWHOIS, ModeShort.

### Fragment Handlers (from detail.go)

The success criteria names 6 specific fragment types. All are tested by `TestFragments_AllTypes`:

| Fragment | URL Pattern | Test Case | Coverage |
|----------|-------------|-----------|----------|
| network IX presences | `/ui/fragment/net/{id}/ixlans` | "net ixlans" | 60.0% |
| network facilities | `/ui/fragment/net/{id}/facilities` | "net facilities" | 69.2% |
| IX networks (participants) | `/ui/fragment/ix/{id}/participants` | "ix participants" | 66.7% |
| IX facilities | `/ui/fragment/ix/{id}/facilities` | "ix facilities" | 58.3% |
| facility networks | `/ui/fragment/fac/{id}/networks` | "fac networks" | 60.0% |
| org networks | `/ui/fragment/org/{id}/networks` | "org networks" | 60.0% |

NOT tested (0% coverage): `handleOrgCampusesFragment`, `handleOrgCarriersFragment`. These are not in the WEB-01 named list but should be added for completeness.

### extractID Function (from completions.go:141)

```go
func extractID(detailURL, typeSlug string) string {
    switch typeSlug {
    case "net":
        return strings.TrimPrefix(detailURL, "/ui/asn/")
    case "ix":
        return strings.TrimPrefix(detailURL, "/ui/ix/")
    case "fac":
        return strings.TrimPrefix(detailURL, "/ui/fac/")
    case "org":
        return strings.TrimPrefix(detailURL, "/ui/org/")
    case "campus":
        return strings.TrimPrefix(detailURL, "/ui/campus/")
    case "carrier":
        return strings.TrimPrefix(detailURL, "/ui/carrier/")
    default:
        return ""
    }
}
```

Currently only the "net" branch is hit (37.5%). Need tests for all 6 cases plus default.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib testing (Go 1.26) |
| Config file | none (stdlib) |
| Quick run command | `go test -count=1 ./internal/web/` |
| Full suite command | `go test -race -count=1 ./internal/web/` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| WEB-01 | Fragment handler integration tests | integration | `go test -run TestFragments -count=1 ./internal/web/` | Partial (TestFragments_AllTypes exists, covers 6 named) |
| WEB-02 | renderPage dispatch for terminal/JSON/WHOIS | integration | `go test -run TestDetailPages_DispatchModes -count=1 ./internal/web/` | No |
| WEB-03-extractID | extractID edge cases | unit | `go test -run TestExtractID -count=1 ./internal/web/` | No |
| WEB-03-getFreshness | getFreshness with sync record | integration | `go test -run TestGetFreshness -count=1 ./internal/web/` | No |
| WEB-03-errors | Error response paths | integration | `go test -run TestError -count=1 ./internal/web/` | Partial (404 tested, 500 not) |

### Sampling Rate
- **Per task commit:** `go test -count=1 ./internal/web/`
- **Per wave merge:** `go test -race -count=1 ./internal/web/`
- **Phase gate:** Full suite green, coverage check with `go test -coverprofile` verifying renderPage > 60%, extractID > 80%

### Wave 0 Gaps
- [ ] `TestDetailPages_DispatchModes` -- covers WEB-02 (terminal/JSON/WHOIS on entity detail page)
- [ ] `TestExtractID` -- covers WEB-03 (all 6 type slugs + unknown + empty)
- [ ] `TestGetFreshness_WithSyncRecord` -- covers WEB-03 (non-nil db path)
- [ ] `TestFragments_OrgCampusesAndCarriers` -- covers WEB-01 adjacent gap (0% coverage functions)

## Gap Analysis Summary

### What Needs to Be Written

1. **renderPage dispatch on entity detail page** (WEB-02): Table-driven test requesting `/ui/asn/13335` (or similar seeded entity) with:
   - `User-Agent: curl/8.5.0` -- assert `text/plain`, contains ANSI escape `\x1b[`
   - `?format=json` -- assert `application/json`, contains `{` and entity name
   - `?format=whois` -- assert `text/plain`, contains RPSL-style key-value output
   - This single test function with 3+ subtests will significantly raise renderPage coverage from 41.8%

2. **extractID complete coverage** (WEB-03): Table-driven test with all 6 type slugs, unknown type, and empty input. Pure unit test, no database needed.

3. **getFreshness with real db** (WEB-03): Integration test using `testutil.SetupClientWithDB`, `sync.InitStatusTable`, inserting a sync success record, verifying non-zero time returned. Also test the no-sync-records case (table exists but empty).

4. **Missing org fragment handlers** (WEB-01 completeness): Add `{"org campuses", "/ui/fragment/org/1/campuses", ...}` and `{"org carriers", "/ui/fragment/org/1/carriers", ...}` to `TestFragments_AllTypes` or a new test. Seed data already includes campuses and carriers under org ID 1.

5. **Error paths** (WEB-03): 404 entity-not-found is already well-tested. For the 500 path, a context cancellation test would exercise `handleServerError` indirectly, but may not be worth the complexity. The function is a 4-line wrapper.

### Estimated Scope

| Item | New Lines (est.) | Risk |
|------|-----------------|------|
| renderPage dispatch test | ~80 lines | LOW -- follows existing TestTerminalDetection pattern |
| extractID test | ~40 lines | LOW -- pure unit test |
| getFreshness test | ~35 lines | LOW -- uses existing testutil infrastructure |
| Org campuses/carriers fragments | ~10 lines | LOW -- add rows to existing test table |
| Error path tests | ~20 lines | LOW -- optional, for handleServerError |
| **Total** | **~185 lines** | **LOW** |

## Sources

### Primary (HIGH confidence)
- Direct codebase analysis of `internal/web/*.go` and `internal/web/*_test.go`
- `go test -coverprofile` output for precise per-function coverage numbers
- `internal/web/render.go` renderPage function (7 mode branches, 136 lines)
- `internal/web/detail.go` fragment handler implementations (lines 839-1378)
- `internal/web/completions.go` extractID function (lines 141-158)
- `internal/web/termrender/detect.go` mode detection logic

### Secondary (MEDIUM confidence)
- Phase 39 research patterns for test structure and documentation approach

## Metadata

**Confidence breakdown:**
- Current state analysis: HIGH -- based on actual `go test -coverprofile` output
- Architecture: HIGH -- all patterns derived from existing test code in the same package
- Pitfalls: HIGH -- identified from actual coverage data and code inspection

**Research date:** 2026-03-26
**Valid until:** 2026-04-26 (stable -- test infrastructure unlikely to change)
