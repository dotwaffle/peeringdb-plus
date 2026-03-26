# Phase 39: gRPC Handler Coverage - Research

**Researched:** 2026-03-26
**Domain:** Go test coverage for ConnectRPC/gRPC handler package (internal/grpcserver)
**Confidence:** HIGH

## Summary

Phase 39 targets 80%+ test coverage on `internal/grpcserver` by closing two specific gaps: (1) adding List filter tests for the 6 entity types that lack them, and (2) adding Stream tests for the 4 entity types currently at 0% stream coverage. The package is currently at 61.7% coverage. The dominant source of uncovered code is the `apply*ListFilters` and `apply*StreamFilters` functions -- each has many `if req.Field != nil` branches, and most entity types only test 2-3 of their filter fields. The stream gap is simpler: 4 types have zero stream test coverage (CarrierFacility, IxPrefix, NetworkIxLan, Poc).

The codebase already has well-established test patterns. All 13 entity types have Get and basic List tests. 7 of 13 have dedicated `TestList*Filters` tests, and 9 of 13 have stream tests. The missing pieces are formulaic -- they follow the exact same patterns as existing tests. No new libraries, helpers, or architectural changes are needed. The existing `testutil.SetupClient` (in-memory SQLite) and HTTP/2 test server setup pattern for streams are proven and sufficient.

**Primary recommendation:** Write ~6 new `TestList*Filters` functions and ~4 new `TestStream*` functions following existing patterns, each exercising at least one filter field with a data assertion. This is purely additive test code with zero risk to production.

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
| GRPC-01 | All 13 entity types have List filter tests covering optional proto field nil-checks | Gap analysis shows 6 types missing dedicated filter tests; 7 types already have them. Each missing type needs a table-driven test setting at least one optional proto field to non-nil and asserting filtered results |
| GRPC-02 | All 13 entity types have Stream tests (4 types currently missing) | CarrierFacility, IxPrefix, NetworkIxLan, and Poc have 0% stream coverage. Established test pattern exists: setup HTTP/2 TLS test server, call Stream RPC, drain and count messages, assert field values |
| GRPC-03 | Filter branch coverage reaches 80%+ across all 13 types using generic test helpers | Current coverage is 61.7%. Filter functions are the largest uncovered area. Adding filter tests for the 6 missing types plus the 4 stream tests should push coverage well past 80%. The existing generic test helpers (ListEntities, StreamEntities in generic_test.go) already have full coverage |
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
- **Package total**: 61.7% of statements
- **Target**: 80%+
- **Gap**: ~18.3 percentage points

### Per-Function Coverage (key gaps)

| Function | Current | File |
|----------|---------|------|
| applyCarrierFacilityStreamFilters | 0.0% | carrierfacility.go |
| StreamCarrierFacilities | 0.0% | carrierfacility.go |
| applyIxPrefixStreamFilters | 0.0% | ixprefix.go |
| StreamIxPrefixes | 0.0% | ixprefix.go |
| applyNetworkIxLanStreamFilters | 0.0% | networkixlan.go |
| StreamNetworkIxLans | 0.0% | networkixlan.go |
| applyCampusListFilters | 61.8% | campus.go |
| applyCampusStreamFilters | 53.3% | campus.go |
| applyCarrierListFilters | 61.5% | carrier.go |
| applyCarrierStreamFilters | 54.5% | carrier.go |
| applyCarrierFacilityListFilters | 55.6% | carrierfacility.go |
| applyFacilityListFilters | 54.4% | facility.go |
| applyFacilityStreamFilters | 51.6% | facility.go |
| applyInternetExchangeListFilters | 56.9% | internetexchange.go |
| applyInternetExchangeStreamFilters | 51.9% | internetexchange.go |
| applyIxFacilityListFilters | 63.6% | ixfacility.go |
| applyIxFacilityStreamFilters | 55.6% | ixfacility.go |
| applyIxLanListFilters | 60.0% | ixlan.go |
| applyIxLanStreamFilters | 57.7% | ixlan.go |
| applyIxPrefixListFilters | 55.0% | ixprefix.go |
| applyNetworkListFilters | 61.4% | network.go |
| applyNetworkStreamFilters | 60.6% | network.go |
| applyNetworkFacilityListFilters | 61.5% | networkfacility.go |
| applyNetworkFacilityStreamFilters | 50.0% | networkfacility.go |
| applyNetworkIxLanListFilters | 43.8% | networkixlan.go |
| applyOrganizationListFilters | 55.6% | organization.go |
| applyOrganizationStreamFilters | 56.2% | organization.go |
| applyPocListFilters (inferred) | ~60% | poc.go |

### Test Gap Inventory

#### List Filter Tests -- Present (7/13)
| Entity | Test Function | Filters Tested |
|--------|---------------|----------------|
| Network | TestListNetworksFilters | asn, status, name, combined, invalid asn, invalid org_id |
| Facility | TestListFacilitiesFilters | country, city, name+country, invalid org_id |
| Organization | TestListOrganizationsFilters | name, status |
| Poc | TestListPocsFilters | role, net_id, invalid net_id |
| IxPrefix | TestListIxPrefixesFilters | protocol, status |
| NetworkIxLan | TestListNetworkIxLansFilters | asn, status |
| CarrierFacility | TestListCarrierFacilitiesFilters | carrier_id, invalid carrier_id |

#### List Filter Tests -- MISSING (6/13)
| Entity | List Filter Fields | Validation Fields (need invalid test) |
|--------|-------------------|--------------------------------------|
| Campus | id, org_id, name, aka, name_long, country, city, status, website, notes, state, zipcode, logo, org_name | id, org_id |
| Carrier | id, org_id, name, aka, name_long, status, website, notes, org_name, logo | id, org_id |
| InternetExchange | id, org_id, name, aka, name_long, country, city, status, region_continent, media, notes, proto_unicast, proto_multicast, proto_ipv6, website, url_stats, tech_email, tech_phone, policy_email, policy_phone, sales_email, sales_phone, service_level, terms, status_dashboard, logo | id, org_id |
| IxFacility | id, ix_id, fac_id, country, city, status, name | id, ix_id, fac_id |
| IxLan | id, ix_id, name, status, descr, mtu, dot1q_support, rs_asn, arp_sponge, ixf_ixp_member_list_url_visible, ixf_ixp_import_enabled | id, ix_id, rs_asn |
| NetworkFacility | id, net_id, fac_id, country, city, status, name, local_asn | id, net_id, fac_id, local_asn |

#### Stream Tests -- Present (9/13)
Networks, Facilities, Organizations, Campuses, Carriers, InternetExchanges, IxLans, IxFacilities, NetworkFacilities

#### Stream Tests -- MISSING (4/13)
| Entity | Stream Filter Fields | FK Dependencies |
|--------|---------------------|-----------------|
| CarrierFacility | carrier_id, fac_id, status, name | Carrier (FK) |
| IxPrefix | ixlan_id, protocol, status, prefix, in_dfz, notes | None (no FK needed for basic seed) |
| NetworkIxLan | net_id, ixlan_id, asn, name, status, ix_id, speed, ipaddr4, ipaddr6, is_rs_peer, bfd_support, operational, notes, net_side_id, ix_side_id | None (no FK needed) |
| Poc | net_id, role, name, status, visible, phone, email, url | Network (FK) |

## Architecture Patterns

### Existing Test Pattern: List Filter Tests (direct service call)
```go
func TestList{Entity}Filters(t *testing.T) {
    t.Parallel()
    ctx := context.Background()
    client := testutil.SetupClient(t)
    now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

    // Seed 2-3 entities with distinct field values for filtering.
    client.Entity.Create().SetID(1).SetName("A")./* ... */.SaveX(ctx)
    client.Entity.Create().SetID(2).SetName("B")./* ... */.SaveX(ctx)

    svc := &EntityService{Client: client}

    tests := []struct {
        name    string
        req     *pb.ListEntitiesRequest
        wantLen int
        wantErr connect.Code
    }{
        {name: "filter by field_x", req: &pb.ListEntitiesRequest{FieldX: proto.String("value")}, wantLen: 1},
        {name: "invalid id", req: &pb.ListEntitiesRequest{Id: proto.Int64(-1)}, wantErr: connect.CodeInvalidArgument},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
            resp, err := svc.ListEntities(ctx, tt.req)
            // Assert error or result count + field values
        })
    }
}
```

### Existing Test Pattern: Stream Tests (HTTP/2 test server)
```go
func setup{Entity}StreamServer(t *testing.T, client *ent.Client) peeringdbv1connect.{Entity}ServiceClient {
    t.Helper()
    svc := &{Entity}Service{Client: client, StreamTimeout: 30 * time.Second}
    mux := http.NewServeMux()
    mux.Handle(peeringdbv1connect.New{Entity}ServiceHandler(svc))
    srv := httptest.NewUnstartedServer(mux)
    srv.EnableHTTP2 = true
    srv.StartTLS()
    t.Cleanup(srv.Close)
    return peeringdbv1connect.New{Entity}ServiceClient(srv.Client(), srv.URL)
}

func TestStream{Entity}(t *testing.T) {
    t.Parallel()
    ctx := context.Background()
    client := testutil.SetupClient(t)
    // Seed data
    // ...
    rpcClient := setup{Entity}StreamServer(t, client)

    tests := []struct {
        name    string
        req     *pb.Stream{Entity}Request
        wantLen int
    }{
        {name: "all records", req: &pb.Stream{Entity}Request{}, wantLen: 2},
        {name: "filter by field", req: &pb.Stream{Entity}Request{Field: proto.String("value")}, wantLen: 1},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
            stream, err := rpcClient.Stream{Entity}(ctx, tt.req)
            // Drain stream, count messages, assert field values
        })
    }
}
```

### Critical Pattern: Field Value Assertions
Success criteria SC-1 says tests must "assert the response contains only matching entities (not just no error)". The existing TestListNetworksFilters does this correctly -- it checks `len(resp.GetNetworks())` against `wantLen`. SC-1 further requires asserting specific field values. Extend each test to check at least one field value on the first result:
```go
if got := resp.GetNetworks()[0].GetAsn(); got != 15169 {
    t.Errorf("Asn = %d, want 15169", got)
}
```

### Critical Pattern: Stream Field Value Assertions
Success criteria SC-2 says each stream test must "assert at least one field value". Current stream tests only count messages. The new tests must capture `stream.Msg()` and assert a field:
```go
for stream.Receive() {
    msg := stream.Msg()
    if msg == nil {
        t.Fatal("received nil message")
    }
    if count == 0 {
        // Assert at least one field on the first message.
        if msg.GetStatus() != "ok" {
            t.Errorf("first message Status = %q, want %q", msg.GetStatus(), "ok")
        }
    }
    count++
}
```

### Anti-Patterns to Avoid
- **Asserting only `err == nil`**: Every test must assert data correctness (count + at least one field value)
- **Over-testing filter branches**: Testing every single filter field would require ~200+ test cases and create massive test files. Instead, test 2-3 representative fields per entity (one string filter, one FK filter with validation, one status filter). The existing pattern tests this way.
- **Shared mutable state between stream subtests**: Each stream subtest must share the same `rpcClient` but the seeded data must be identical across subtests. Seed once per test function, not per subtest.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| In-memory test DB | Custom SQLite setup | `testutil.SetupClient(t)` | Already handles isolation, cleanup, and FK constraints |
| HTTP/2 stream test server | Manual transport setup | `httptest.NewUnstartedServer` + `EnableHTTP2 = true` + `StartTLS()` | Proven pattern used 9 times already in this file |
| Proto field pointer helpers | Manual pointer construction | `proto.String()`, `proto.Int64()`, `proto.Bool()` | Standard protobuf helpers, already imported |

## Common Pitfalls

### Pitfall 1: FK Constraints on Entity Creation
**What goes wrong:** Creating a CarrierFacility with `SetCarrierID(10)` fails if Carrier with ID=10 does not exist, because SQLite FK constraints are enabled.
**Why it happens:** `testutil.SetupClient` opens SQLite with `_pragma=foreign_keys(1)`.
**How to avoid:** Always create parent entities before children. The existing tests demonstrate this (e.g., TestListPocsFilters creates Networks before POCs, TestStreamNetworkFacilities creates Network and Facility before NetworkFacility).
**Warning signs:** `FOREIGN KEY constraint failed` errors in test output.

### Pitfall 2: Parallel Test Isolation with Shared Test Server
**What goes wrong:** Stream subtests share a `rpcClient` that points to one server backed by one database. If a subtest mutates data, other subtests see unexpected results.
**Why it happens:** The HTTP/2 test server is created once per test function and shared across table-driven subtests.
**How to avoid:** Seed all test data once before the subtest loop. Never create/delete entities inside subtests. The existing pattern handles this correctly.
**Warning signs:** Flaky test counts in parallel runs.

### Pitfall 3: Required Fields on Entity Creation
**What goes wrong:** `SaveX(ctx)` panics if a required ent schema field is not set.
**Why it happens:** Ent schemas have required fields (e.g., Network requires `info_unicast`, `info_multicast`, `info_ipv6`, etc.).
**How to avoid:** Copy the field sets from existing test seeds. All entity types have at least one working seed example in grpcserver_test.go.
**Warning signs:** `missing required field` panics from enttest.

### Pitfall 4: Stream Error Handling
**What goes wrong:** Stream errors may arrive on `Receive()` rather than the initial call. Checking only the initial `err` misses filter validation errors.
**Why it happens:** ConnectRPC streams deliver errors lazily -- the initial handshake succeeds, but the first `Receive()` returns the error.
**How to avoid:** The existing TestStreamNetworks pattern handles this correctly: check initial err, then drain stream and check `stream.Err()`. Use this pattern for all stream error tests.

### Pitfall 5: Coverage Target Math
**What goes wrong:** Adding tests without understanding where coverage gaps actually live leads to diminishing returns.
**Why it happens:** The 0% stream functions (4 types) contribute significantly to the gap because each Stream* function has 15-20 statements. The apply*Filters functions have many branches but each `if req.X != nil` check is only 2-3 statements.
**How to avoid:** Prioritize the 4 missing stream types first (each adds ~15-20 covered statements). Then add list filter tests for the 6 missing types. Monitor with `go test -cover` after each addition.

## Coverage Budget Estimate

The gap from 61.7% to 80% requires covering approximately 18.3% more statements. The package has ~700 total statements (estimated from line counts and coverage percentage).

| Gap Category | Approx. Uncovered Statements | Strategy |
|--------------|------------------------------|----------|
| 4 missing stream types (StreamX + applyXStreamFilters) | ~80-100 statements | New stream tests |
| 6 missing list filter types (applyXListFilters) | ~60-80 statements | New list filter tests |
| Existing filter functions (50-60% covered) | ~40-60 statements | Additional filter field tests on existing types |
| GetX internal error paths | ~13 statements (13 types x 1 uncovered branch) | Not required -- these are database error paths |
| convert.go nil-pointer branches | ~4 statements | Exercised indirectly by testing entities with nil optional fields |

**Estimated new coverage from planned tests:** ~140-180 statements (out of ~267 uncovered), bringing total to approximately 82-87%.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib testing + enttest v0.14.5 |
| Config file | None (stdlib test runner) |
| Quick run command | `go test -race -cover ./internal/grpcserver/...` |
| Full suite command | `go test -race -cover ./...` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| GRPC-01 | All 13 entity types have List filter tests | integration | `go test -race -run 'TestList.*Filters' ./internal/grpcserver/... -v` | Partial (7/13 exist) |
| GRPC-02 | All 13 entity types have Stream tests | integration | `go test -race -run 'TestStream' ./internal/grpcserver/... -v` | Partial (9/13 exist) |
| GRPC-03 | 80%+ package coverage | coverage | `go test -race -cover ./internal/grpcserver/...` | Measured at 61.7% |

### Sampling Rate
- **Per task commit:** `go test -race -cover ./internal/grpcserver/...`
- **Per wave merge:** `go test -race -cover ./...`
- **Phase gate:** Coverage >= 80% reported by `go test -cover ./internal/grpcserver/...`

### Wave 0 Gaps
None -- existing test infrastructure covers all phase requirements. `testutil.SetupClient`, `httptest` HTTP/2 server pattern, and `peeringdbv1connect` client are all in place.

## Code Examples

### List Filter Test Template (verified from existing TestListNetworksFilters)
```go
// Source: internal/grpcserver/grpcserver_test.go:200
func TestListCampusesFilters(t *testing.T) {
    t.Parallel()
    ctx := context.Background()
    client := testutil.SetupClient(t)
    now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

    client.Campus.Create().
        SetID(1).SetName("Campus Alpha").SetCountry("US").SetCity("Dallas").
        SetCreated(now).SetUpdated(now).SetStatus("ok").
        SaveX(ctx)
    client.Campus.Create().
        SetID(2).SetName("Campus Beta").SetCountry("GB").SetCity("London").
        SetCreated(now).SetUpdated(now).SetStatus("ok").
        SaveX(ctx)

    svc := &CampusService{Client: client}

    tests := []struct {
        name    string
        req     *pb.ListCampusesRequest
        wantLen int
        wantErr connect.Code
    }{
        {
            name:    "filter by name substring",
            req:     &pb.ListCampusesRequest{Name: proto.String("alpha")},
            wantLen: 1,
        },
        {
            name:    "filter by country",
            req:     &pb.ListCampusesRequest{Country: proto.String("US")},
            wantLen: 1,
        },
        {
            name:    "invalid org_id",
            req:     &pb.ListCampusesRequest{OrgId: proto.Int64(0)},
            wantErr: connect.CodeInvalidArgument,
        },
    }
    // ... table-driven test loop with field value assertions
}
```

### Stream Test Template (verified from existing TestStreamFacilities)
```go
// Source: internal/grpcserver/grpcserver_test.go:2320
func setupCarrierFacilityStreamServer(t *testing.T, client *ent.Client) peeringdbv1connect.CarrierFacilityServiceClient {
    t.Helper()
    svc := &CarrierFacilityService{Client: client, StreamTimeout: 30 * time.Second}
    mux := http.NewServeMux()
    mux.Handle(peeringdbv1connect.NewCarrierFacilityServiceHandler(svc))
    srv := httptest.NewUnstartedServer(mux)
    srv.EnableHTTP2 = true
    srv.StartTLS()
    t.Cleanup(srv.Close)
    return peeringdbv1connect.NewCarrierFacilityServiceClient(srv.Client(), srv.URL)
}

func TestStreamCarrierFacilities(t *testing.T) {
    t.Parallel()
    ctx := context.Background()
    client := testutil.SetupClient(t)
    now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

    // FK parent: Carrier
    client.Carrier.Create().
        SetID(10).SetName("Test Carrier").
        SetCreated(now).SetUpdated(now).SetStatus("ok").
        SaveX(ctx)
    // Carrier facilities
    client.CarrierFacility.Create().
        SetID(1).SetCarrierID(10).SetName("CF-A").
        SetCreated(now).SetUpdated(now).SetStatus("ok").
        SaveX(ctx)
    client.CarrierFacility.Create().
        SetID(2).SetCarrierID(10).SetName("CF-B").
        SetCreated(now).SetUpdated(now).SetStatus("ok").
        SaveX(ctx)

    rpcClient := setupCarrierFacilityStreamServer(t, client)

    tests := []struct {
        name    string
        req     *pb.StreamCarrierFacilitiesRequest
        wantLen int
    }{
        {name: "all records", req: &pb.StreamCarrierFacilitiesRequest{}, wantLen: 2},
        {name: "filter by status", req: &pb.StreamCarrierFacilitiesRequest{Status: proto.String("ok")}, wantLen: 2},
    }
    // ... table-driven test loop with stream.Msg() field assertion
}
```

## Open Questions

1. **Coverage ceiling realism**
   - What we know: The success criteria says 80%+. Current is 61.7%. The planned tests should bring it to ~82-87%.
   - What's unclear: Some code paths (GetX internal DB error, convert.go nil branches for unused pointer types) may be difficult to reach without artificial error injection. These represent ~5% of total statements.
   - Recommendation: Target 80% and accept it. Do not inject fake DB errors just to hit a higher number -- those are tested by enttest itself.

## Sources

### Primary (HIGH confidence)
- Source code analysis of all 13 handler files in `internal/grpcserver/`
- `go test -cover` output from actual test run: 61.7%
- `go tool cover -func` per-function breakdown
- Existing test patterns in `grpcserver_test.go` (2920 lines)

### Secondary (MEDIUM confidence)
- Statement count estimates based on line counts and coverage percentages

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- no new dependencies, all existing tools
- Architecture: HIGH -- patterns copied verbatim from existing tests
- Pitfalls: HIGH -- based on direct observation of FK constraints and stream error handling in existing code
- Coverage estimate: MEDIUM -- exact statement counts are estimates; 80% is achievable but margin depends on which specific branches get hit

**Research date:** 2026-03-26
**Valid until:** 2026-04-26 (stable -- no external dependencies)
