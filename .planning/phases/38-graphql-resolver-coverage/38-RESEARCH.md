# Phase 38: GraphQL Resolver Coverage - Research

**Researched:** 2026-03-26
**Domain:** Go test coverage for hand-written GraphQL resolvers (gqlgen + entgo)
**Confidence:** HIGH

## Summary

Phase 38 targets 80%+ test coverage on three hand-written files in the `graph/` package: `custom.resolvers.go` (315 lines, 15 offset/limit list resolvers + SyncStatus + NetworkByAsn + ObjectCounts), `schema.resolvers.go` (191 lines, 13 cursor-based resolvers + validatePageSize + Node/Nodes), and `pagination.go` (42 lines, ValidateOffsetLimit). The generated file `generated.go` (57K lines) dominates package-level coverage at ~2.6% total, but coverage targets are per-file, not per-package.

Current coverage on these files is minimal: 11 of 13 offset/limit list resolvers are at 0%, 11 of 13 cursor resolvers are at 0%, and ValidateOffsetLimit is at 58%. The existing `resolver_test.go` (546 lines) provides a solid test infrastructure -- httptest server, `postGraphQL` helper, `gqlResponse` envelope parsing -- but seeds only 3 entity types (Org, Network, Facility) via hand-rolled creation. Phase 37's `seed.Full()` creates all 13 entity types with deterministic IDs, which is exactly what's needed to exercise the remaining resolvers.

**Primary recommendation:** Replace the legacy `seedTestData()` function with `seed.Full()`, then add table-driven tests for all 13 offset/limit list resolvers and all 13 cursor resolvers, plus targeted error-path tests for NetworkByAsn not-found, SyncStatus-missing, validatePageSize rejection, and ValidateOffsetLimit edge cases.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
None -- auto-generated infrastructure phase, all choices at Claude's discretion.

### Claude's Discretion
All implementation choices are at Claude's discretion -- pure infrastructure phase. Use ROADMAP phase goal, success criteria, and codebase conventions to guide decisions.

### Deferred Ideas (OUT OF SCOPE)
None
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| GQL-01 | All 13 offset/limit list resolvers have integration tests with data assertions | Seed.Full() provides all 13 entity types; table-driven test iterates each resolver's GraphQL query and asserts returned data matches seeded entities |
| GQL-02 | Custom resolver error paths tested (NetworkByAsn not found, SyncStatus missing, validatePageSize) | Direct resolver invocation or GraphQL query with known-bad inputs; SyncStatus-missing = query with no sync_status rows; NetworkByAsn not-found = non-existent ASN returns null |
| GQL-03 | Hand-written resolver files reach 80%+ coverage | Coverage gap analysis below identifies exactly which branches are untested; achieving 80%+ requires exercising where-filter and error branches in list resolvers |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **T-1 (MUST)**: Table-driven tests; deterministic and hermetic
- **T-2 (MUST)**: Run `-race` in CI; add `t.Cleanup` for teardown
- **T-3 (SHOULD)**: Mark safe tests with `t.Parallel()`
- **CS-5 (MUST)**: Input structs for functions with >2 args (relevant if adding helpers)
- **ERR-1 (MUST)**: Wrap with `%w` and context
- **No new test frameworks**: Out of scope per REQUIREMENTS.md -- use stdlib assertions only

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| testing (stdlib) | Go 1.26 | Test framework | Project convention, T-1 |
| net/http/httptest | Go 1.26 | HTTP test server | Existing pattern in resolver_test.go |
| encoding/json (stdlib) | Go 1.26 | Parse GraphQL responses | Existing pattern |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| internal/testutil | project | SetupClientWithDB for isolated SQLite | Every test needing an ent client |
| internal/testutil/seed | project | Deterministic entity seeding | Replace hand-rolled seedTestData |
| internal/sync | project | InitStatusTable, RecordSyncStart/Complete | SyncStatus tests |
| graph (package under test) | project | NewResolver, ValidateOffsetLimit | Direct unit tests for pagination |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| HTTP integration tests | Direct resolver method calls | HTTP tests exercise the full middleware stack but are slower; direct calls are faster but skip gqlgen transport. Use HTTP for resolver tests (matches existing pattern), direct calls for ValidateOffsetLimit unit tests |

## Architecture Patterns

### Test File Structure
```
graph/
  resolver_test.go          # Existing: helpers + 12 test functions (546 lines)
  # No new files needed -- all tests go in resolver_test.go
```

### Pattern 1: Table-Driven List Resolver Tests
**What:** Single test function with a table of 13 entries, one per entity type offset/limit resolver
**When to use:** GQL-01 -- all 13 list resolvers need identical test structure
**Example:**
```go
func TestGraphQLAPI_OffsetLimitListResolvers(t *testing.T) {
    t.Parallel()
    srv := seedFullTestServer(t)

    tests := []struct {
        name      string
        query     string
        dataField string
        wantMin   int // minimum expected results
    }{
        {
            name:      "organizationsList",
            query:     `{ organizationsList { name } }`,
            dataField: "organizationsList",
            wantMin:   1,
        },
        // ... 12 more entries
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
            result := postGraphQL(t, srv.URL, tt.query)
            // assert no errors, unmarshal, check count >= wantMin
            // assert specific field values match seeded data
        })
    }
}
```

### Pattern 2: Error Path Tests as Individual Functions
**What:** Targeted tests for each custom error path
**When to use:** GQL-02 -- error paths need specific setup and assertions
**Example:**
```go
func TestGraphQLAPI_NetworkByAsn_NotFound(t *testing.T) {
    t.Parallel()
    srv := seedFullTestServer(t)
    result := postGraphQL(t, srv.URL, `{ networkByAsn(asn: 99999) { name } }`)
    // Assert no errors (null is valid, not an error)
    // Assert data.networkByAsn is null
}

func TestGraphQLAPI_SyncStatus_Missing(t *testing.T) {
    t.Parallel()
    srv := setupTestServer(t) // no sync data seeded
    result := postGraphQL(t, srv.URL, `{ syncStatus { status } }`)
    // Assert no errors
    // Assert data.syncStatus is null
}
```

### Pattern 3: Direct Unit Tests for Pagination Validation
**What:** Call `ValidateOffsetLimit` directly with edge-case inputs
**When to use:** Coverage of all 5 branches in pagination.go
**Example:**
```go
func TestValidateOffsetLimit(t *testing.T) {
    t.Parallel()
    tests := []struct {
        name       string
        offset     *int
        limit      *int
        wantOffset int
        wantLimit  int
        wantErr    string
    }{
        {name: "defaults", wantOffset: 0, wantLimit: 100},
        {name: "negative offset", offset: intPtr(-1), wantErr: "non-negative"},
        {name: "zero limit", limit: intPtr(0), wantErr: "at least 1"},
        {name: "over max limit", limit: intPtr(1001), wantErr: "must not exceed"},
        {name: "custom values", offset: intPtr(50), limit: intPtr(25), wantOffset: 50, wantLimit: 25},
    }
    // ...
}
```

### Anti-Patterns to Avoid
- **Asserting only `err == nil`:** Every test must assert returned data matches seeded values (per QUAL-01 and success criteria #1)
- **One setup per test function:** Reuse a shared `seedFullTestServer` per top-level test; subtests share the server since tests are read-only
- **Testing generated code paths:** Don't try to cover `generated.go` -- it's 57K lines and dominates package coverage. Per-file targets bypass this.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Entity seeding | Per-test entity creation | `seed.Full(tb, client)` | Phase 37 created this exactly for this purpose; 13 types with deterministic IDs |
| GraphQL response parsing | Custom per-test unmarshaling | `postGraphQL` + `json.RawMessage` | Existing helper already handles this |
| Test server setup | Per-test handler wiring | Shared `seedFullTestServer(t)` helper | Reuse pattern from existing `seedTestData` but with seed.Full |

## Coverage Gap Analysis

### custom.resolvers.go (current: ~12% average across functions)
| Function | Current | Missing Branches | Effort |
|----------|---------|-------------------|--------|
| SyncStatus | 70% | err path (GetLastStatus fails), errorMessage="" branch | LOW |
| NetworkByAsn | 50% | not-found path (returns nil,nil), generic error path | LOW |
| OrganizationsList | 0% | All branches | LOW |
| NetworksList | 50% | where!=nil branch | LOW |
| FacilitiesList | 0% | All branches | LOW |
| InternetExchangesList | 0% | All branches | LOW |
| PocsList | 0% | All branches | LOW |
| IxLansList | 0% | All branches | LOW |
| IxPrefixesList | 0% | All branches | LOW |
| IxFacilitiesList | 0% | All branches | LOW |
| NetworkIxLansList | 0% | All branches | LOW |
| NetworkFacilitiesList | 0% | All branches | LOW |
| CarriersList | 0% | All branches | LOW |
| CarrierFacilitiesList | 0% | All branches | LOW |
| CampusesList | 0% | All branches | LOW |
| ObjectCounts | 0% | nil + non-nil map paths | LOW |
| SyncStatus (factory) | 0% | Trivial -- exercised by any SyncStatus query | LOW |

**To reach 80%+:** Exercise the happy path for all 13 list resolvers (covers ~50% each), the where-filter branch for at least a few (pushes to ~80%), plus SyncStatus/NetworkByAsn/ObjectCounts error paths. The `where!=nil` branches are harder to exercise via GraphQL since WhereInput filter generation is in generated code -- but the happy path without `where` still covers 50% per resolver. Since there are 15 functions plus 2 utility functions, getting all happy paths to 50%+ and a few to 100% should cross the 80% file-level threshold.

### schema.resolvers.go (current: ~20% average)
| Function | Current | Missing Branches | Effort |
|----------|---------|-------------------|--------|
| validatePageSize | 80% | `last > max` branch | LOW |
| Node | 100% | Complete | NONE |
| Nodes | 0% | Happy path | LOW |
| 11 cursor resolvers (0%) | 0% | Happy path + page size error | LOW |
| Networks | 100% | Complete | NONE |
| Organizations | 66.7% | validatePageSize error branch | LOW |
| Pocs | 0% | All branches | LOW |
| Query (factory) | 100% | Complete | NONE |

**To reach 80%+:** Exercise the happy path for all 13 cursor resolvers. Each has only 2 branches (validatePageSize error vs success), so a single successful query covers 66.7%. Testing `last > 1000` covers the remaining validatePageSize branch. With Nodes at 0%, adding one test pushes the file total well above 80%.

### pagination.go (current: 58.3%)
| Function | Current | Missing Branches | Effort |
|----------|---------|-------------------|--------|
| ValidateOffsetLimit | 58.3% | negative offset, zero/negative limit | LOW |

**To reach 80%+:** Add 2 test cases: negative offset and limit < 1. Currently only tests defaults, custom values, and over-max limit.

## Common Pitfalls

### Pitfall 1: Package-Level Coverage Distortion
**What goes wrong:** `go test -cover` reports 2.6% because generated.go is 57K lines and dominates
**Why it happens:** Coverage is calculated per-package by default, not per-file
**How to avoid:** Use `go tool cover -func` filtered to specific files as success criteria requires
**Warning signs:** Seeing "2.6% coverage" and thinking tests failed

### Pitfall 2: GraphQL Where Filter Branches
**What goes wrong:** The `where != nil` branches in list resolvers are hard to hit via integration tests because `WhereInput.P()` can fail with invalid filter specs but the only way to pass filters is through valid GraphQL syntax which gqlgen validates before reaching the resolver
**Why it happens:** gqlgen validates filter input types at the transport layer; by the time the resolver runs, `where.P()` will succeed
**How to avoid:** Accept that the `where.P()` error branch may be unreachable via HTTP integration tests. The happy-path `where != nil` branch IS reachable by passing valid filter arguments. For 80%+ file coverage, covering the happy path (no filter) and happy path (with filter) for a representative set of resolvers is sufficient.
**Warning signs:** Trying to force a `where.P()` error and finding no valid way to trigger it through GraphQL

### Pitfall 3: SyncStatus Missing vs Error
**What goes wrong:** Confusing "no sync_status rows" (returns nil,nil) with "GetLastStatus error" (returns nil,error)
**Why it happens:** Both are error paths but have different test setups
**How to avoid:** "Missing" = create server with sync_status table but no rows. "Error" = would require broken SQL which is hard to simulate. Focus on the missing path since the success criteria specifically calls it out.
**Warning signs:** Trying to trigger GetLastStatus error without mocking the database

### Pitfall 4: Shared Test Server Performance
**What goes wrong:** Each test creating its own server + database slows down the suite
**Why it happens:** `seed.Full()` creates 15 entities across 13 types, plus sync_status table init
**How to avoid:** Share a single test server across subtests within a top-level test function. Read-only tests can safely share a seeded database. Use `t.Parallel()` on subtests.
**Warning signs:** Test suite taking >10 seconds

### Pitfall 5: Null vs Error for Not-Found
**What goes wrong:** Testing NetworkByAsn with a non-existent ASN and expecting a GraphQL error
**Why it happens:** The resolver returns `nil, nil` for not-found (per ent.IsNotFound check), which GraphQL renders as `{ "data": { "networkByAsn": null } }` with no errors
**How to avoid:** Assert `data.networkByAsn` is JSON null, NOT that errors array is non-empty
**Warning signs:** Test expects errors but gets null data

## Code Examples

### Shared Test Helper: Server with Full Seed
```go
// seedFullTestServer creates an httptest.Server with all 13 entity types seeded.
// Uses seed.Full for deterministic data and initializes sync_status table.
func seedFullTestServer(t *testing.T) *httptest.Server {
    t.Helper()
    client, db := testutil.SetupClientWithDB(t)
    ctx := context.Background()

    if err := pdbsync.InitStatusTable(ctx, db); err != nil {
        t.Fatalf("init sync_status table: %v", err)
    }

    _ = seed.Full(t, client)

    resolver := graph.NewResolver(client, db)
    gqlHandler := pdbgql.NewHandler(resolver)
    mux := http.NewServeMux()
    playgroundHandler := pdbgql.PlaygroundHandler("/graphql")
    mux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
        if r.Method == http.MethodGet {
            playgroundHandler.ServeHTTP(w, r)
            return
        }
        gqlHandler.ServeHTTP(w, r)
    })

    handler := middleware.CORS(middleware.CORSInput{AllowedOrigins: "*"})(mux)
    srv := httptest.NewServer(handler)
    t.Cleanup(srv.Close)
    return srv
}
```

### Data Assertion Pattern
```go
// Extract JSON array length and first element field value
var raw map[string]json.RawMessage
if err := json.Unmarshal(result.Data, &raw); err != nil {
    t.Fatalf("unmarshal data: %v", err)
}
var items []json.RawMessage
if err := json.Unmarshal(raw["organizationsList"], &items); err != nil {
    t.Fatalf("unmarshal list: %v", err)
}
if len(items) < 1 {
    t.Fatal("expected at least 1 item")
}
// Unmarshal first item and check known field
var org struct { Name string `json:"name"` }
if err := json.Unmarshal(items[0], &org); err != nil {
    t.Fatalf("unmarshal item: %v", err)
}
if org.Name != "Test Organization" {
    t.Errorf("name = %q, want %q", org.Name, "Test Organization")
}
```

### Seed Entity to GraphQL Query Mapping
| Resolver | Query | Expected Data from seed.Full() |
|----------|-------|-------------------------------|
| organizationsList | `{ organizationsList { name } }` | name="Test Organization" |
| networksList | `{ networksList { name asn } }` | 2 networks: Cloudflare/13335, Hurricane Electric/6939 |
| facilitiesList | `{ facilitiesList { name city } }` | 2 facilities: "Equinix FR5"/"Frankfurt", "Campus Facility"/"Berlin" |
| internetExchangesList | `{ internetExchangesList { name } }` | name="DE-CIX Frankfurt" |
| pocsList | `{ pocsList { name role } }` | name="NOC Contact", role="NOC" |
| ixLansList | `{ ixLansList { id } }` | id=100 |
| ixPrefixesList | `{ ixPrefixesList { prefix protocol } }` | prefix="80.81.192.0/22", protocol="IPv4" |
| ixFacilitiesList | `{ ixFacilitiesList { name } }` | name="DE-CIX Frankfurt" |
| networkIxLansList | `{ networkIxLansList { asn speed } }` | asn=13335, speed=10000 |
| networkFacilitiesList | `{ networkFacilitiesList { name localAsn } }` | name="Equinix FR5", localAsn=13335 |
| carriersList | `{ carriersList { name } }` | name="Test Carrier" |
| carrierFacilitiesList | `{ carrierFacilitiesList { name } }` | name="Equinix FR5" |
| campusesList | `{ campusesList { name city } }` | name="Test Campus", city="Berlin" |

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing stdlib + enttest |
| Config file | none -- standard `go test` |
| Quick run command | `go test -race -count=1 ./graph/...` |
| Full suite command | `go test -race -count=1 -coverprofile=cover.out ./graph/... && go tool cover -func=cover.out` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| GQL-01 | 13 offset/limit list resolvers return seeded data | integration | `go test -race -run TestGraphQLAPI_OffsetLimitListResolvers ./graph/... -x` | Partial (1 of 13) |
| GQL-02a | NetworkByAsn not-found returns null | integration | `go test -race -run TestGraphQLAPI_NetworkByAsn_NotFound ./graph/... -x` | No |
| GQL-02b | SyncStatus missing returns null | integration | `go test -race -run TestGraphQLAPI_SyncStatus_Missing ./graph/... -x` | No |
| GQL-02c | validatePageSize rejects limit > max | integration | `go test -race -run TestGraphQLAPI_PageSizeLimit ./graph/... -x` | Yes (exists) |
| GQL-03 | 80%+ per-file coverage | coverage | `go tool cover -func=cover.out \| grep -E '(custom\.resolvers\|schema\.resolvers\|pagination)\.'` | No (need new tests) |

### Sampling Rate
- **Per task commit:** `go test -race -count=1 ./graph/...`
- **Per wave merge:** `go test -race -count=1 -coverprofile=cover.out ./graph/... && go tool cover -func=cover.out | grep -E '(custom\.resolvers|schema\.resolvers|pagination)\.'`
- **Phase gate:** All three files show 80%+ per-function average before `/gsd:verify-work`

### Wave 0 Gaps
None -- test infrastructure is fully in place. The `resolver_test.go` file has all necessary helpers (postGraphQL, gqlResponse, server setup). Phase 37 seed package is available. Only new test functions are needed.

## Open Questions

1. **Can `where.P()` error branch be triggered via HTTP?**
   - What we know: gqlgen validates WhereInput at the transport layer before resolver execution
   - What's unclear: Whether any valid GraphQL input can cause `P()` to return an error
   - Recommendation: Accept this branch may be unreachable in integration tests. The happy path with and without filters still provides sufficient coverage for 80%+. Do not mock to force this path.

2. **SyncStatus GetLastStatus error path**
   - What we know: Requires database-level failure (table missing, corrupt DB)
   - What's unclear: Whether it's worth mocking sql.DB to test this
   - Recommendation: Skip -- the `status == nil` path (no rows) is the success criteria requirement. The generic error path is defensive code that doesn't need dedicated testing for 80%+ coverage.

## Sources

### Primary (HIGH confidence)
- `graph/custom.resolvers.go` -- 315 lines, 17 functions, all code paths analyzed
- `graph/schema.resolvers.go` -- 191 lines, 17 functions, all code paths analyzed
- `graph/pagination.go` -- 42 lines, 1 function, all branches analyzed
- `graph/resolver_test.go` -- 546 lines, existing test infrastructure and patterns
- `internal/testutil/seed/seed.go` -- Phase 37 output, seed.Full() creates all 13 types
- `go tool cover -func` output -- exact per-function coverage percentages

### Secondary (MEDIUM confidence)
- Branch analysis of `where != nil` reachability via gqlgen transport layer

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all tools are project-internal, no new dependencies
- Architecture: HIGH -- follows existing test patterns exactly
- Pitfalls: HIGH -- verified through code analysis and existing test behavior

**Research date:** 2026-03-26
**Valid until:** 2026-04-26 (stable -- test infrastructure doesn't change frequently)
