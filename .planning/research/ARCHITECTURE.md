# Architecture Patterns

**Domain:** Test infrastructure for a multi-surface Go API application
**Researched:** 2026-03-26

## Current Test Architecture Assessment

### Inventory

The codebase has 60 test files totaling ~404K lines (overwhelmingly in generated code). Meaningful hand-written test code is approximately 8,500 lines across 15 packages. Overall statement coverage is 5.7% (dragged down by untestable generated code in `ent/`, `gen/`, and `graph/generated.go`).

**Hand-written package coverage:**

| Package | Coverage | LOC (tests) | LOC (source) | Assessment |
|---------|----------|-------------|--------------|------------|
| graph (resolvers only) | 2.6% | 546 | 548 | Critical gap -- 13 List resolvers untested, offset/limit variants 0% |
| ent/schema | 47.4% | 908 | 2,143 | Fields/Hooks covered, Edges/Indexes/Annotations at 0% |
| internal/grpcserver | 61.7% | 3,253 | 3,344 | Good breadth but 4 entity types have only Get, no List/Stream tests |
| internal/web | 74.8% | 3,273 | ~2,500 | Detail/search/compare covered, some terminal render paths thin |
| internal/web/termrender | 88.2% | ~2,000 | ~1,800 | Strong coverage, minor gaps in edge cases |
| internal/pdbcompat | 85.4% | ~1,500 | ~1,200 | Golden files excellent, filter edge cases could be deeper |
| internal/sync | 86.9% | ~800 | ~900 | Integration + unit, good |
| internal/middleware | 98.2% | ~300 | ~200 | Near-complete |
| internal/config | 86.1% | ~150 | ~200 | Adequate |
| internal/otel | 84.0% | ~300 | ~400 | Provider/metrics covered, logger thin |
| internal/health | 84.6% | ~100 | ~100 | Adequate |
| internal/peeringdb | 83.2% | ~400 | ~500 | Client well tested |
| internal/httperr | 100.0% | ~100 | ~50 | Complete |
| internal/litefs | 87.5% | ~50 | ~50 | Adequate |

### Existing Shared Infrastructure

**`internal/testutil` (1 file, 57 lines):**
- `SetupClient(t) *ent.Client` -- in-memory SQLite with auto-migration
- `SetupClientWithDB(t) (*ent.Client, *sql.DB)` -- adds raw sql.DB access
- Atomic counter for unique DB names enabling parallel tests
- Registers `sqlite3` driver once via `init()`

This is the single shared helper. All data seeding is package-local.

### Data Seeding Patterns (Current State)

Every package that needs test data seeds it independently:

| Package | Seeding Approach | Entities Created | Duplication |
|---------|-----------------|------------------|-------------|
| graph | `seedTestData()` -- Org, 3 Networks, Facility + sync_status | Local to graph | HIGH |
| grpcserver | Per-test inline `client.Network.Create()...` chains | Per entity type | HIGH |
| pdbcompat | `setupTestHandler()` -- 3 Networks; `setupGoldenTestData()` -- all 13 types | Two separate setups | MEDIUM |
| web | `seedAllTestData()` -- full relationship graph; `seedSearchData()` -- 6 types | Two separate setups | HIGH |
| ent/schema | Per-subtest inline creation | All 13 types | LOW (correct) |
| sync | `fixture` struct with mock HTTP server; `newFixtureServer()` with JSON fixtures | Via JSON testdata | LOW (correct) |

**Key observation:** There are at least 6 independent implementations of "create a test Organization with required fields." The `setupGoldenTestData()` in pdbcompat is the most comprehensive (all 13 types with relationships), but it returns a `*http.ServeMux` specific to pdbcompat handlers, making it non-reusable.

## Recommended Architecture

### Component Boundaries

| Component | Responsibility | Used By |
|-----------|---------------|---------|
| `internal/testutil` (expanded) | Ent client setup, entity factory functions, shared timestamps | All packages with DB tests |
| `internal/testutil/seed` (new) | Comprehensive data seeding with all 13 types + relationships | graph, grpcserver, pdbcompat, web |
| Package-local test helpers | HTTP server setup, response parsing, package-specific assertions | Each package independently |
| `testdata/fixtures/` (existing) | JSON fixture files from real PeeringDB API responses | sync integration tests |
| `internal/pdbcompat/testdata/golden/` (existing) | Golden file snapshots | pdbcompat only |

### Data Flow for Shared Fixtures

```
testutil.SetupClient(t)                 -- Creates isolated in-memory SQLite
    |
    v
seed.Full(t, client) -> seed.Result     -- Seeds all 13 types with relationships
    |                                       Returns struct with IDs for assertions
    v
Package-local server setup               -- Wires seeded client into package handler
    |                                       (GraphQL resolver, gRPC service, HTTP handler)
    v
Package-local test assertions             -- Tests exercise API surface with known IDs
```

### The `seed` Package Design

```go
// Package seed provides deterministic test data for all 13 PeeringDB entity types.
package seed

// Timestamp is the fixed time used for all seeded entities.
var Timestamp = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// Result holds the IDs and references of all seeded entities.
// Tests use these for deterministic assertions.
type Result struct {
    Org              *ent.Organization  // ID=100
    Networks         []*ent.Network     // IDs 200-202, ASNs 65001-65003
    Facility         *ent.Facility      // ID=300
    IX               *ent.InternetExchange // ID=400
    Campus           *ent.Campus        // ID=500
    Carrier          *ent.Carrier       // ID=600
    IxLan            *ent.IxLan         // ID=700
    IxPrefix         *ent.IxPrefix      // ID=800
    IxFacility       *ent.IxFacility    // ID=900
    NetworkIxLan     *ent.NetworkIxLan  // ID=1000
    NetworkFacility  *ent.NetworkFacility // ID=1100
    CarrierFacility  *ent.CarrierFacility // ID=1200
    Poc              *ent.Poc           // ID=1300
}

// Full seeds all 13 entity types with relationships and returns their references.
func Full(t *testing.T, client *ent.Client) Result { ... }

// Minimal seeds only Org + Network (most common test scenario).
func Minimal(t *testing.T, client *ent.Client) Result { ... }

// Networks seeds n networks with sequential IDs and ASNs.
func Networks(t *testing.T, client *ent.Client, n int) []*ent.Network { ... }

// WithSyncStatus seeds the sync_status table with a successful sync record.
func WithSyncStatus(t *testing.T, db *sql.DB) { ... }
```

**Why a `seed` sub-package instead of expanding `testutil`:** The `testutil` package handles infrastructure (client setup). Seeding is a separate concern -- it creates domain-specific test data. Keeping them separate means packages that only need a client (like ent/schema tests) do not pull in all 13 entity creation paths.

### Test Boundary Recommendations Per Package

#### 1. `graph/` -- GraphQL Resolvers (2.6% -> 80%+ target)

**What to test:** The hand-written resolvers in `custom.resolvers.go` (SyncStatus, NetworkByAsn, 13 offset/limit List resolvers, ObjectCounts) and `schema.resolvers.go` (validatePageSize, Node, Nodes, 13 cursor-based Paginate resolvers).

**What NOT to test:** `generated.go` (57K lines of gqlgen internals -- this is the gqlgen execution engine, not business logic).

**Test boundary:** Integration tests via HTTP. Tests POST GraphQL queries to an httptest.Server and verify JSON response structure. This is the existing pattern (resolver_test.go) -- it just needs to be expanded to cover all 13 List/offset-limit resolvers and error paths.

**Why HTTP-level, not unit:** The resolvers are thin wrappers around ent queries. Testing them at the HTTP level validates the full stack (argument parsing, resolver dispatch, ent query, serialization) with minimal test code. Unit-testing a resolver function directly would require constructing gqlgen context objects which is fragile and undocumented.

**Infrastructure needed:**
- Reuse existing `setupTestServer()` and `postGraphQL()` helpers (already in graph)
- Replace `seedTestData()` with `seed.Full()` from shared package
- Add test cases for each of the 13 `*List` offset/limit resolvers
- Add error path tests (negative offset, limit > 1000, invalid where filters)

#### 2. `ent/schema/` -- Schema Validation & Hooks (47.4% -> 70%+ target)

**What to test:** Hook behavior (`otelMutationHook`), field validators (if any custom ones exist), and relationship constraints via CRUD operations. The Fields() functions show 100% coverage because enttest exercises them during migration; the 0% on Edges/Indexes/Annotations is **expected and acceptable** -- these return static configuration structs consumed by ent codegen, not runtime code paths.

**What NOT to test:** The static schema declarations (Edges, Indexes, Annotations). These are configuration, not behavior. Testing "does Fields() return the right fields" is testing ent's codegen, not your application.

**Test boundary:** Unit tests using ent client directly. The existing `schema_test.go` pattern of CRUD-per-type is correct. Focus expansion on:
- Hook behavior: Verify `otelMutationHook` creates spans (currently 90% -- the miss is likely the error path)
- Relationship cascading: Creating child entities without required parents should fail
- Social media JSON field round-tripping (already partially covered in organization_test.go)

**Infrastructure needed:**
- `testutil.SetupClient(t)` is sufficient (already used)
- No shared seeding needed -- schema tests should create their own entities to test specific constraints

**Realistic target: 65-70%.** The Edges/Indexes/Annotations functions account for ~50% of the schema source code and are not worth testing. Raising coverage to 80% would require testing static configuration returns, which is test theater.

#### 3. `internal/grpcserver/` -- ConnectRPC Handlers (61.7% -> 80%+ target)

**What to test:** Get/List/Stream for all 13 entity types, filter validation, pagination, proto conversion, error mapping (not-found, invalid-argument).

**Current gaps:**
- Only Network, Facility, Organization, Poc, IxPrefix, NetworkIxLan, CarrierFacility have List filter tests
- Missing List filter tests: Campus, Carrier, InternetExchange, IxFacility, IxLan, NetworkFacility (these have Get/List tests but no filter-specific tests)
- Stream tests exist for Network, Facility, Organization, Campus, Carrier, InternetExchange, IxLan, IxFacility, NetworkFacility -- but not IxPrefix, NetworkIxLan, CarrierFacility, Poc

**Test boundary:** White-box unit tests calling service methods directly (current pattern). The grpcserver tests are `package grpcserver` (not `_test`), giving access to unexported types. This is correct because the services are thin adapters -- testing through a full ConnectRPC client would add transport serialization overhead without catching different bugs.

**Infrastructure needed:**
- Replace per-test inline seeding with `seed.Networks(t, client, 3)` or `seed.Full(t, client)` for multi-type tests
- The existing stream test setup (`setupStreamTestServer`) should be generalized to accept any service type
- Filter test pattern is already table-driven and consistent -- just needs replication to remaining entity types

#### 4. `internal/pdbcompat/` -- PeeringDB Compatibility Layer (85.4% -> 90%+ target)

**What to test:** Handler routing, filter parsing, depth parameter, serialization, field projection, search endpoint, golden file output correctness.

**Already well-structured:** The golden file pattern (`compareOrUpdate`, `-update` flag, deterministic timestamps) is the strongest test pattern in the codebase. The `setupGoldenTestData()` creates all 13 types -- this should become the shared `seed.Full()`.

**Test boundary:** HTTP-level integration via httptest. Tests create handlers, register routes on a ServeMux, and assert response structure. This is the right boundary for a compatibility layer where exact HTTP response format matters.

**Infrastructure needed:**
- Extract `setupGoldenTestData()` entity creation logic into `seed.Full()` -- keep the mux/handler setup local
- Expand filter tests for remaining filter operators (currently covers basic filtering; Django-style `__contains`, `__gte` etc. may have gaps)

#### 5. `internal/web/` -- Web UI Handlers (74.8% -> 85%+ target)

**What to test:** Route handling, template rendering (HTML contains expected elements), content negotiation (full page vs htmx fragment vs terminal), search service, comparison logic, detail page data loading.

**Test boundary:** HTTP-level integration. Tests exercise handlers via httptest, assert on HTML content (string contains checks), HTTP headers (Vary, Content-Type), and status codes. This is the right level -- the templates are compiled Go code (templ), so testing them as rendered output catches template errors.

**Infrastructure needed:**
- `seedAllTestData()` and `seedSearchData()` should use `seed.Full()` or `seed.Minimal()` for entity creation, keeping only the handler/mux wiring local
- Terminal render tests (`internal/web/termrender/`) are self-contained and work well -- no changes needed

#### 6. `internal/sync/` -- Data Sync Worker (86.9% -> stable)

**What NOT to change:** The sync package has two complementary test approaches:
1. `worker_test.go` -- mock HTTP server (`fixture` struct) with programmatic responses for unit testing sync logic
2. `integration_test.go` -- JSON fixture files from real PeeringDB API for integration testing the full parse-and-upsert pipeline

This dual approach is correct. The fixture JSON files in `testdata/fixtures/` represent real PeeringDB API responses and should not be replaced with synthetic data.

**Test boundary:** Internal package tests (access to unexported types). The `TestMain` initializing OTel metrics globally is a necessary workaround for global state in the metrics package.

### Patterns to Follow

#### Pattern 1: Table-Driven Tests With Shared Setup

**What:** Single setup function seeds data once, multiple subtests exercise different behaviors.
**When:** Testing the same handler/service with different inputs.
**Example:** grpcserver filter tests -- one `seedData` call, table of `{req, wantLen, wantErr}`.

```go
func TestListNetworksFilters(t *testing.T) {
    t.Parallel()
    client := testutil.SetupClient(t)
    result := seed.Full(t, client)
    svc := &NetworkService{Client: client}

    tests := []struct {
        name    string
        req     *pb.ListNetworksRequest
        wantLen int
        wantErr connect.Code
    }{
        {name: "by ASN", req: &pb.ListNetworksRequest{Asn: proto.Int64(65001)}, wantLen: 1},
        // ...
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
            resp, err := svc.ListNetworks(ctx, tt.req)
            // assertions...
        })
    }
}
```

#### Pattern 2: Golden File Tests for Serialization-Sensitive Surfaces

**What:** Compare actual output against committed golden files. Update with `-update` flag.
**When:** Testing exact output format (PeeringDB compat responses, terminal render output).
**Example:** The pdbcompat golden test pattern is the reference implementation.

#### Pattern 3: HTTP-Level Integration for API Surfaces

**What:** Full handler stack via httptest.Server, real HTTP requests, assert on response body/headers.
**When:** Testing API surfaces where transport behavior matters (CORS, content negotiation, status codes).
**Not when:** Testing pure business logic (use direct function calls instead).

#### Pattern 4: White-Box for Adapters

**What:** Internal package tests (`package foo`, not `package foo_test`) calling unexported types directly.
**When:** Testing thin adapter layers (gRPC services, sync workers) where the public API is defined by an external interface.
**Why:** Avoids constructing complex external client objects when the adapter's logic is just data transformation.

### Anti-Patterns to Avoid

#### Anti-Pattern 1: Testing Generated Code

**What:** Writing tests that exercise paths through `graph/generated.go`, `ent/*.go`, or `gen/peeringdb/v1/*.go`.
**Why bad:** These are generated by codegen tools (gqlgen, ent, buf). Testing them tests the tool, not the application. Coverage numbers for generated packages should be excluded from targets.
**Instead:** Test the hand-written code that uses the generated code (resolvers, services, handlers).

#### Anti-Pattern 2: Duplicating Entity Creation

**What:** Each test file has its own `seedData()` with slightly different field values for the same entity types.
**Why bad:** When schemas change (new required field), every seed function breaks independently. Currently there are 6+ independent Organization creation sites.
**Instead:** Use shared `seed` package with a single, maintained creation path per entity type.

#### Anti-Pattern 3: Asserting Internal Structure of Generated Responses

**What:** Tests that decode GraphQL/REST responses into deeply nested structs and assert on every field.
**Why bad:** Fragile to schema changes. A renamed field breaks tests even when behavior is correct.
**Instead:** Assert on the aspects that matter: presence of data, correct count, specific business-critical fields, error codes. Not every field in every response.

#### Anti-Pattern 4: Mocking ent Client

**What:** Creating mock implementations of the ent client interface.
**Why bad:** ent does not have a client interface -- it generates concrete types. Mocking would require creating an abstraction layer that does not exist. In-memory SQLite is fast enough (~5ms per test setup) and tests real SQL behavior.
**Instead:** Use `testutil.SetupClient(t)` with real in-memory SQLite. It is fast, hermetic, and catches real query bugs.

## Integration Points With Existing Architecture

### New Components

| Component | Location | Dependencies | Creates |
|-----------|----------|-------------|---------|
| `seed` package | `internal/testutil/seed/` | `ent`, `ent/enttest`, `internal/sync` (for sync_status) | Deterministic test entities |

### Modified Components

| Component | Change | Reason |
|-----------|--------|--------|
| `internal/testutil/testutil.go` | No changes | Client setup is correct as-is |
| `graph/resolver_test.go` | Expand with 13 List resolver tests, use seed.Full | Currently only 5 test functions |
| `ent/schema/schema_test.go` | Add hook error path test | Raise hooks from 90% to 100% |
| `internal/grpcserver/grpcserver_test.go` | Add missing entity type filter/stream tests | 6 entity types lack filter tests |
| `internal/web/handler_test.go` | Swap seedAllTestData to use seed package | Reduce duplication |
| `internal/pdbcompat/golden_test.go` | Extract entity creation into seed.Full | setupGoldenTestData becomes thin wrapper |

### Unchanged Components

| Component | Why No Changes |
|-----------|---------------|
| `internal/sync/` | Test infrastructure is well-designed (mock server + JSON fixtures) |
| `internal/web/termrender/` | Self-contained, high coverage, no shared data needs |
| `internal/middleware/` | 98.2% coverage, tests are simple and correct |
| `internal/conformance/` | 96.3%, comparison logic tests are adequate |
| `internal/httperr/` | 100%, complete |
| `testdata/fixtures/` | Real PeeringDB API snapshots, must not be replaced with synthetic data |
| `internal/pdbcompat/testdata/golden/` | Golden file snapshots, updated by test runs |

## Build Order for Test Infrastructure

Phase ordering is driven by dependency: shared infrastructure must exist before per-package test expansion.

### Phase 1: Shared Seed Package

Create `internal/testutil/seed/` with `Full()`, `Minimal()`, `Networks()`, `WithSyncStatus()`. This has no test dependencies itself -- it is a helper library. Verify by running existing tests unchanged (seed package is additive, not breaking).

**Dependencies:** None (uses existing `ent` client)
**Validates:** Compiles, existing tests still pass

### Phase 2: GraphQL Resolver Coverage (graph/)

Expand `resolver_test.go` with tests for all 13 `*List` resolvers, error paths (negative offset, limit > MaxLimit), NetworkByAsn not-found, SyncStatus error path. Refactor `seedTestData()` to use `seed.Full()`.

**Dependencies:** Phase 1 (seed package)
**Validates:** graph coverage rises from 2.6% to target

### Phase 3: gRPC Handler Coverage (grpcserver/)

Add missing filter tests for Campus, Carrier, InternetExchange, IxFacility, IxLan, NetworkFacility. Add missing Stream tests for IxPrefix, NetworkIxLan, CarrierFacility, Poc. Refactor per-test seeding to use seed package where it simplifies code.

**Dependencies:** Phase 1 (seed package)
**Validates:** grpcserver coverage rises from 61.7% to target

### Phase 4: Schema Hook Coverage (ent/schema/)

Add test for otelMutationHook error path (the missing 10%). Add relationship constraint tests (create child without parent).

**Dependencies:** None (uses testutil.SetupClient directly)
**Validates:** ent/schema coverage rises from 47.4% toward 65-70%

### Phase 5: Web Handler Coverage (web/)

Expand handler tests for untested routes and error paths. Refactor seedAllTestData/seedSearchData to use seed package.

**Dependencies:** Phase 1 (seed package)
**Validates:** web coverage rises from 74.8% to target

### Phase 6: Remaining Packages

Raise otel, health, peeringdb packages above 85%. These are smaller, independent efforts.

**Dependencies:** None (independent packages)
**Validates:** Per-package coverage targets met

## Coverage Exclusion Strategy

The 5.7% total coverage number is misleading because it includes generated code. The coverage target should be stated per-package for hand-written code only.

**Packages to exclude from coverage targets:**
- `ent/*` (all sub-packages) -- fully generated by entgo
- `gen/peeringdb/v1/*` -- fully generated by buf/protoc
- `graph/generated.go` -- fully generated by gqlgen
- `graph/model/` -- fully generated by gqlgen
- `ent/rest/` -- fully generated by entrest
- `internal/web/templates/` -- generated by templ (compiled from .templ files)

**Meaningful total coverage** should be computed across only hand-written packages. Based on current numbers, that is approximately 60-65% across hand-written code, with a target of 80%+.

## Sources

- Direct codebase analysis: 60 test files examined, per-function coverage profiling
- Pattern analysis of existing test infrastructure in testutil, sync, pdbcompat, grpcserver
- Coverage profile from `go test -coverprofile` across all packages
- Go testing documentation: https://go.dev/doc/test
- enttest documentation: https://entgo.io/docs/testing/
