# Phase 37: Test Seed Infrastructure - Research

**Researched:** 2026-03-26
**Domain:** Go test infrastructure / ent entity factory / test data seeding
**Confidence:** HIGH

## Summary

The codebase has 6+ nearly-identical seed functions scattered across 4 packages (`graph`, `internal/web`, `internal/grpcserver`, `internal/pdbcompat`) that manually create the same entities with minor variations. Each function is 30-170 lines of boilerplate calling ent Create builders in the correct FK order. This duplication is the root problem INFRA-01 addresses.

The seed package must create all 13 PeeringDB entity types in FK-valid topological order, return typed references to every entity via a Result struct, and be importable from any test package without import cycles. The existing `internal/testutil` package (used by 15+ test files) only provides `SetupClient` / `SetupClientWithDB` -- it has zero entity creation helpers, making it the natural parent for the seed sub-package.

**Primary recommendation:** Create `internal/testutil/seed` package with `Full(t, client)`, `Minimal(t, client)`, and `Networks(t, client, n)` functions that return a `Result` struct with typed pointers to all created entities.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
None -- auto-generated infrastructure phase.

### Claude's Discretion
All implementation choices are at Claude's discretion -- pure infrastructure phase. Use ROADMAP phase goal, success criteria, and codebase conventions to guide decisions.

### Deferred Ideas (OUT OF SCOPE)
None.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| INFRA-01 | Shared test seed package provides deterministic entity factories for all 13 PeeringDB types | Entity relationship graph mapped, creation order determined, existing duplication catalogued across 6+ seed functions in 4 packages |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **T-1 (MUST):** Table-driven tests; deterministic and hermetic by default
- **T-2 (MUST):** Run -race in CI; add t.Cleanup for teardown
- **T-3 (SHOULD):** Mark safe tests with t.Parallel()
- **CS-2 (MUST):** Avoid stutter in names -- package `seed`; type `Result` (not `SeedResult`)
- **CS-5 (MUST):** Use input structs for functions receiving more than 2 arguments (ctx excluded)
- **API-1 (MUST):** Document exported items
- **API-2 (MUST):** Accept interfaces where variation is needed; return concrete types
- **MD-1 (SHOULD):** Prefer stdlib; no new dependencies needed for this phase

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| testing (stdlib) | Go 1.26 | Test framework | Project convention. Seeds accept `testing.TB` for test/bench compatibility. |
| entgo.io/ent | v0.14.5 | ORM client for entity creation | All entities created via ent builders. No alternative. |
| modernc.org/sqlite | v1.36.0+ | SQLite driver for test DBs | Already used by testutil.SetupClient. |
| entgo.io/ent/enttest | v0.14.5 | Test client setup | Already used by testutil.SetupClient. |

No new dependencies required.

## Architecture Patterns

### Recommended Package Structure
```
internal/testutil/
  testutil.go          # existing: SetupClient, SetupClientWithDB
  seed/
    seed.go            # Full, Minimal, Networks functions + Result struct
    seed_test.go       # tests verifying all 13 types created correctly
```

### Pattern 1: Result Struct with Typed Entity References
**What:** A concrete struct that holds pointers to every created entity, allowing test code to reference specific IDs and fields without magic numbers.
**When to use:** Always returned from seed functions.
**Example:**
```go
// Result holds references to all entities created by a seed function.
// Each field is a pointer to the created ent entity, allowing tests
// to reference specific IDs, names, and relationships deterministically.
type Result struct {
    Org              *ent.Organization
    Network          *ent.Network
    Network2         *ent.Network        // only in Full
    IX               *ent.InternetExchange
    Facility         *ent.Facility
    Campus           *ent.Campus
    Carrier          *ent.Carrier
    IxLan            *ent.IxLan
    IxPrefix         *ent.IxPrefix
    NetworkIxLan     *ent.NetworkIxLan
    NetworkFacility  *ent.NetworkFacility
    IxFacility       *ent.IxFacility
    CarrierFacility  *ent.CarrierFacility
    Poc              *ent.Poc
}
```

### Pattern 2: testing.TB for Test/Benchmark Compatibility
**What:** Accept `testing.TB` interface instead of `*testing.T` so seed functions work in both tests and benchmarks.
**When to use:** All public seed functions.
**Rationale:** The existing `seedBenchNetworks` in `grpcserver/list_bench_test.go` shows benchmarks need seeding too.

### Pattern 3: Deterministic Well-Known IDs
**What:** Use fixed, non-conflicting IDs for all entities so test assertions can reference exact values.
**When to use:** All seed functions.
**Rationale:** Every existing seed function in the codebase uses SetID with explicit integers. The ent schemas define `field.Int("id").Positive().Immutable()` -- PeeringDB uses pre-assigned IDs, not auto-increment.

### Pattern 4: Topological Creation Order
**What:** Create entities in FK-dependency order to satisfy foreign key constraints.
**When to use:** Every seed function must follow this order.
**The order (verified from schema edges):**
1. Organization (no FK deps)
2. Campus (FK: org_id)
3. Carrier (FK: org_id)
4. Facility (FK: org_id, campus_id)
5. InternetExchange (FK: org_id)
6. IxLan (FK: ix_id)
7. Network (FK: org_id)
8. IxPrefix (FK: ixlan_id)
9. NetworkIxLan (FK: net_id, ixlan_id)
10. NetworkFacility (FK: net_id, fac_id)
11. IxFacility (FK: ix_id, fac_id)
12. CarrierFacility (FK: carrier_id, fac_id)
13. Poc (FK: net_id)

### Anti-Patterns to Avoid
- **Magic number assertions:** Tests should use `result.Network.ID` not `10`. The Result struct eliminates this.
- **Overloaded seed data:** Do NOT try to make `Full()` serve every possible test scenario (comparison data, deletion data, etc.). Keep it a complete-but-simple entity graph. Tests needing specific scenarios should compose on top.
- **Package-level init state:** No `sync.Once`, no global variables for seeded data. Each test gets its own DB via `testutil.SetupClient`.

## Entity Relationship Graph (All 13 Types)

```
Organization (root)
  |-- Campus           (org_id -> Organization)
  |     `-- Facility   (campus_id -> Campus, org_id -> Organization)
  |-- Carrier          (org_id -> Organization)
  |     `-- CarrierFacility (carrier_id -> Carrier, fac_id -> Facility)
  |-- Facility         (org_id -> Organization, campus_id -> Campus [optional])
  |     |-- IxFacility     (fac_id -> Facility, ix_id -> InternetExchange)
  |     |-- NetworkFacility (fac_id -> Facility, net_id -> Network)
  |     `-- CarrierFacility (fac_id -> Facility, carrier_id -> Carrier)
  |-- InternetExchange (org_id -> Organization)
  |     `-- IxLan      (ix_id -> InternetExchange)
  |           |-- IxPrefix    (ixlan_id -> IxLan)
  |           `-- NetworkIxLan (ixlan_id -> IxLan, net_id -> Network)
  |-- Network          (org_id -> Organization)
  |     |-- NetworkFacility (net_id -> Network, fac_id -> Facility)
  |     |-- NetworkIxLan    (net_id -> Network, ixlan_id -> IxLan)
  |     `-- Poc             (net_id -> Network)
```

## Current Duplication Inventory

Seed functions that will be replaced/simplified by this infrastructure:

| Location | Function | Entities Created | Lines |
|----------|----------|-----------------|-------|
| `graph/resolver_test.go` | `seedTestData` | Org, 3x Network, Facility | ~80 |
| `internal/web/detail_test.go` | `seedAllTestData` | All 13 types + extra Facility | ~155 |
| `internal/web/compare_test.go` | `seedCompareTestData` | Org, 2x Net, 2x IX, 2x Fac, Campus, 3x NetIxLan, 3x NetFac | ~190 |
| `internal/web/search_test.go` | `seedSearchData` | Org, Net, IX, Fac, Campus, Carrier | ~55 |
| `internal/web/completions_test.go` | `seedCompletionData` | Org, 2x Net | ~30 |
| `internal/grpcserver/grpcserver_test.go` | `seedStreamNetworks` | 3x Network | ~25 |
| `internal/grpcserver/grpcserver_test.go` | (inline in 20+ test functions) | Network, IX, Fac, etc. | ~300+ |
| `internal/web/handler_test.go` | `seedCompareHandlerTestData` | delegates to seedCompareTestData | ~5 |

**Total estimated duplicated seed code:** ~840+ lines across 4 packages.

Note: Not all of these will be replaced by Phase 37. The `seed.Full` / `seed.Minimal` / `seed.Networks` functions provide a foundation. Individual test files that need specialized data (e.g., compare_test.go's overlapping network presences) may continue to have local helpers that compose on top of the seed package. The goal is to eliminate the basic boilerplate, not to force every test into a single data shape.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Test DB setup | Custom SQLite + migration | `testutil.SetupClient(t)` | Already handles unique DB names, auto-migration, cleanup |
| Entity ID generation | Auto-increment or random IDs | Fixed well-known IDs | PeeringDB uses pre-assigned IDs; tests need deterministic references |
| FK relationship wiring | Manual SetOrgID + SetOrganization | Both-set pattern (existing convention) | Existing code always sets both the FK int field AND the edge setter. This is required for ent eager-loading to work correctly in tests. |

**Key insight:** The ent builder pattern (fluent API with `Set*` methods) is the correct abstraction. The seed package wraps ent builders for convenience, not to replace them.

## Common Pitfalls

### Pitfall 1: Name Uniqueness Constraints
**What goes wrong:** Multiple tests calling `seed.Full` in parallel create entities with the same names, hitting UNIQUE constraints on `name` fields.
**Why it happens:** Organization.name, Network.name, Network.asn, InternetExchange.name, Facility.name, Campus.name, Carrier.name, IxPrefix.prefix all have UNIQUE constraints.
**How to avoid:** Each seed call uses `testutil.SetupClient(t)` which creates an isolated in-memory SQLite database. As long as seed functions take the client as a parameter (not create their own), uniqueness is scoped to the test.
**Warning signs:** "UNIQUE constraint failed" errors in parallel tests.

### Pitfall 2: Import Cycles
**What goes wrong:** The seed package imports a package that transitively imports the seed package.
**Why it happens:** If the seed package were placed in `internal/web` or `graph`, other packages couldn't import it.
**How to avoid:** Place in `internal/testutil/seed` which only imports `ent` and `ent/enttest` (both are leaf packages in the dependency graph). Never import `graph`, `internal/web`, `internal/grpcserver`, or `internal/pdbcompat` from the seed package.
**Warning signs:** "import cycle not allowed" at build time.

### Pitfall 3: Ent Edge vs FK Field Inconsistency
**What goes wrong:** Setting only the FK int field (e.g., `SetOrgID(1)`) without also calling the edge setter (e.g., `SetOrganization(org)`) causes eager-loaded edges to be nil in queries.
**Why it happens:** Ent maintains both an FK column and an in-memory edge cache. Tests that only set the FK column get correct query results but nil edges on the returned entity.
**How to avoid:** Always call both setters, matching the pattern used in every existing seed function: `.SetOrgID(org.ID).SetOrganization(org)`.
**Warning signs:** Nil pointer dereference when accessing entity edges in test assertions.

### Pitfall 4: Required Fields Not Set
**What goes wrong:** Seed function omits a required field, causing ent validation errors.
**Why it happens:** Each entity has multiple required fields (created, updated, status, plus entity-specific ones like Network.asn). Easy to miss one.
**How to avoid:** Set all non-optional fields with realistic values. For Boolean fields, set explicit values (ent defaults apply, but being explicit documents intent). Use a shared timestamp constant.
**Warning signs:** "validator failed for field" errors at save time.

### Pitfall 5: testing.TB vs *testing.T for Helper
**What goes wrong:** `t.Helper()` does not exist on `testing.TB` interface.
**Why it happens:** `testing.TB` is the common interface between `*testing.T` and `*testing.B`, but `.Helper()` was added to `testing.TB` in Go 1.9, so this actually works. However, `t.Parallel()` is NOT on `testing.TB`.
**How to avoid:** Use `testing.TB` for the parameter type. Call `tb.Helper()` (it works). Do NOT call `t.Parallel()` inside seed functions -- that's the caller's responsibility.
**Warning signs:** Compile errors if accidentally using `*testing.T` methods.

## Code Examples

### Full Seed Function Skeleton
```go
// Source: synthesized from internal/web/detail_test.go seedAllTestData pattern

package seed

import (
    "context"
    "testing"
    "time"

    "github.com/dotwaffle/peeringdb-plus/ent"
)

// Timestamp provides a fixed, deterministic timestamp for all seed data.
var Timestamp = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// Result holds typed references to all entities created by seed functions.
type Result struct {
    Org             *ent.Organization
    Network         *ent.Network
    IX              *ent.InternetExchange
    Facility        *ent.Facility
    Campus          *ent.Campus
    Carrier         *ent.Carrier
    IxLan           *ent.IxLan
    IxPrefix        *ent.IxPrefix
    NetworkIxLan    *ent.NetworkIxLan
    NetworkFacility *ent.NetworkFacility
    IxFacility      *ent.IxFacility
    CarrierFacility *ent.CarrierFacility
    Poc             *ent.Poc
}

// Full creates one entity of each of the 13 PeeringDB types with realistic
// field values and correct FK relationships.
func Full(tb testing.TB, client *ent.Client) *Result {
    tb.Helper()
    ctx := context.Background()
    r := &Result{}

    // 1. Organization (root -- no FK deps)
    var err error
    r.Org, err = client.Organization.Create().
        SetID(1).
        SetName("Test Organization").
        SetCity("Frankfurt").
        SetCountry("DE").
        SetCreated(Timestamp).
        SetUpdated(Timestamp).
        Save(ctx)
    if err != nil {
        tb.Fatalf("seed: create organization: %v", err)
    }

    // ... (remaining 12 entity types in topological order)
    return r
}
```

### Minimal Seed Function
```go
// Minimal creates the minimum entity graph needed for relationship traversal:
// Org + Network + IX + Facility (and their FK connections).
func Minimal(tb testing.TB, client *ent.Client) *Result {
    tb.Helper()
    // Creates 4 entities with correct FK wiring.
    // Does NOT create junction types (NetworkIxLan, IxFacility, etc.)
}
```

### Networks Convenience Function
```go
// Networks creates exactly n networks with their required Organization
// dependency. Each network gets a unique ASN (65001+i) and name.
func Networks(tb testing.TB, client *ent.Client, n int) *Result {
    tb.Helper()
    // Creates 1 Org + n Networks.
    // Result.Network points to the first; additional networks accessible
    // via a slice field or by querying the client.
}
```

### Consumer Usage Pattern
```go
// In graph/resolver_test.go
func TestSomeQuery(t *testing.T) {
    t.Parallel()
    client := testutil.SetupClient(t)
    data := seed.Full(t, client)

    // Use typed references instead of magic numbers
    result := queryNetwork(t, data.Network.ID)
    if result.OrgName != data.Org.Name {
        t.Errorf("org name = %q, want %q", result.OrgName, data.Org.Name)
    }
}
```

## Design Decisions

### 1. Package Location: `internal/testutil/seed`
**Rationale:** Sub-package of existing `testutil` avoids import cycles. All consumer packages (`graph`, `internal/web`, `internal/grpcserver`) already import `internal/testutil`. Adding a `seed` sub-package follows the same import path pattern. The alternative (`internal/seed`) would work too but `testutil/seed` groups test helpers together.

### 2. Accept `*ent.Client` Not `*testing.T`
**Rationale:** The function signature `Full(tb testing.TB, client *ent.Client)` separates DB creation from data creation. Tests that need `*sql.DB` for sync_status operations (like `graph/resolver_test.go`) can call `testutil.SetupClientWithDB(t)` and pass only the client to seed. This matches the existing pattern where `seedAllTestData(t, client)` in `internal/web/detail_test.go` already takes the client as a parameter.

### 3. Fixed IDs Starting at Non-Overlapping Ranges
**Rationale:** Use ID ranges that don't conflict if a test creates additional entities on top of seed data. Suggestion: Org=1, Campus=40, Carrier=50, Facility=30, IX=20, IxLan=100, Network=10, IxPrefix=700, NetworkIxLan=200, NetworkFacility=300, IxFacility=400, CarrierFacility=600, Poc=500. These match the existing `seedAllTestData` IDs from `internal/web/detail_test.go` which already creates all 13 types -- minimizing test changes.

### 4. Networks() Returns Extra Networks in a Slice
**Rationale:** The success criteria require `seed.Networks(t, client, 2)` to create exactly 2 networks. Since `Result.Network` holds one, a `Networks []*ent.Network` slice field on Result captures all created networks. For `Full()` and `Minimal()`, this slice contains the same network(s) as the named fields.

### 5. Realistic Field Values from PeeringDB Domain
**Rationale:** Use real-world-like data (ASN 13335/Cloudflare, DE-CIX Frankfurt, Equinix FR5) rather than generic "Test Entity 1" names. This matches the existing test data style in `internal/web/detail_test.go` and makes test output more readable. However, names should not exactly match real PeeringDB entries to avoid confusion -- use "Test Organization" as the org name, real-ish IXP/facility names.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | testing (stdlib) + ent/enttest |
| Config file | none -- stdlib testing |
| Quick run command | `go test ./internal/testutil/seed/ -v -count=1` |
| Full suite command | `go test -race ./...` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| INFRA-01a | Full() creates 13 entity types | unit | `go test ./internal/testutil/seed/ -run TestFull -v` | Wave 0 |
| INFRA-01b | Minimal() creates Org+Net+IX+Fac | unit | `go test ./internal/testutil/seed/ -run TestMinimal -v` | Wave 0 |
| INFRA-01c | Networks(n) creates exactly n networks | unit | `go test ./internal/testutil/seed/ -run TestNetworks -v` | Wave 0 |
| INFRA-01d | Result struct fields are non-nil | unit | `go test ./internal/testutil/seed/ -run TestFull -v` | Wave 0 |
| INFRA-01e | No import cycles from graph, grpcserver, web | build | `go build ./graph/... ./internal/grpcserver/... ./internal/web/...` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/testutil/seed/ -race -v -count=1`
- **Per wave merge:** `go test -race ./... -count=1`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/testutil/seed/seed.go` -- seed package implementation
- [ ] `internal/testutil/seed/seed_test.go` -- tests covering all 13 entity types, Minimal, Networks

## Open Questions

1. **Should Networks() return a modified Result or a dedicated NetworksResult?**
   - What we know: The success criteria say `seed.Networks(t, client, 2)` creates exactly 2 networks. A full Result struct would have many nil fields.
   - What's unclear: Whether callers of Networks() would benefit from a simpler return type.
   - Recommendation: Use the same Result struct with nil fields for uncreated entities. This is consistent and avoids type proliferation. The Org field will be set (since networks need an org), Network points to the first, and a `Networks []*ent.Network` slice field holds all created networks.

2. **Should Phase 37 also refactor existing seed functions to use the new package?**
   - What we know: Success criteria 3 requires tests in 3+ packages to import and use the seed package.
   - What's unclear: Whether "import and use" means new tests or refactoring existing ones.
   - Recommendation: Write new usage tests in at least 3 packages to prove import-cycle-freedom and usability. Defer full migration of existing seed functions to Phase 38-42 when those packages are being modified anyway. Adding usage examples is sufficient to prove INFRA-01.

## Sources

### Primary (HIGH confidence)
- Codebase analysis: `ent/schema/*.go` -- all 13 entity schemas with FK relationships
- Codebase analysis: `internal/testutil/testutil.go` -- existing test helper package
- Codebase analysis: `internal/web/detail_test.go` `seedAllTestData` -- most complete existing seed function (all 13 types)
- Codebase analysis: `graph/resolver_test.go` `seedTestData` -- GraphQL test seed pattern
- Codebase analysis: `internal/grpcserver/grpcserver_test.go` -- gRPC test entity creation patterns
- Codebase analysis: `internal/web/compare_test.go` `seedCompareTestData` -- complex multi-entity seed

### Secondary (MEDIUM confidence)
- Go testing.TB interface documentation -- TB.Helper() availability verified

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- no new libraries, pure stdlib + existing ent
- Architecture: HIGH -- package placement and Result struct pattern derived from existing codebase patterns
- Pitfalls: HIGH -- all pitfalls observed directly in existing test code
- Entity relationships: HIGH -- verified from ent schema source files

**Research date:** 2026-03-26
**Valid until:** 2026-04-26 (stable -- internal infrastructure, no external deps)
