# Phase 24: List Filtering - Research

**Researched:** 2026-03-25
**Domain:** Protobuf filter fields, ent query builder composition, ConnectRPC error handling
**Confidence:** HIGH

## Summary

This phase adds typed filter fields to all 13 List RPC request messages in `services.proto`, then applies those filters as ent `Where` predicates in the service handlers. The work is entirely additive -- existing pagination, ordering, and proto message types are unchanged. Each List request message gains `optional` typed fields that the handler checks for presence (nil check on pointer types) and converts to ent predicates before querying.

The ent ORM already generates all needed predicate functions (`AsnEQ`, `StatusEQ`, `NameContainsFold`, `CountryEQ`, etc.) from the schema definitions. No new ent code generation is required. The proto changes are confined to `services.proto` (hand-written, not entproto-generated), and the handler changes are straightforward query builder composition.

**Primary recommendation:** Use proto3 `optional` keyword for all filter fields, which generates pointer types (`*int64`, `*string`) in Go. Check presence via `!= nil` before appending `Where` predicates to the ent query builder. Apply all present filters as AND conditions.

<user_constraints>

## User Constraints (from CONTEXT.md)

### Locked Decisions
- Individual typed fields on List request messages (e.g., `optional int64 asn = 2;`) -- type-safe, self-documenting
- Use proto `optional` keyword for presence detection via pointer types -- distinguishes unset from zero values
- Entity-specific filter fields: Network gets asn/name/country/status/org_id, Facility gets country/city/status, etc. -- only fields that make sense per type
- Invalid filter field values return `INVALID_ARGUMENT` with field name in message: "invalid filter: asn must be positive"
- Multiple filters combine with AND logic -- all filters must match
- Case-insensitive LIKE matching for name/string fields, exact match for country/status codes and numeric fields

### Claude's Discretion
- Exact set of filter fields per entity type beyond the examples (asn, country, name, org_id, status)
- Internal ent query builder composition for filter application
- Test fixture design for filter combinations

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope.

</user_constraints>

<phase_requirements>

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| API-03 | List RPCs support typed filter fields for querying | Proto `optional` fields on List request messages + ent Where predicates in handlers. All 13 entities covered. Validated that ent generates all needed predicate functions. |

</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **CS-5 (MUST)**: Use input structs for functions receiving more than 2 arguments. Filter application functions may need this if they accept query + multiple filter values.
- **ERR-1 (MUST)**: Wrap errors with `%w` and context. Filter validation errors must include the field name.
- **T-1 (MUST)**: Table-driven tests; deterministic and hermetic. Filter tests should be table-driven with filter combinations.
- **T-2 (MUST)**: Run `-race` in CI; add `t.Cleanup` for teardown.
- **T-3 (SHOULD)**: Mark safe tests with `t.Parallel()`.
- **API-1 (MUST)**: Document exported items.
- **SEC-1 (MUST)**: Validate inputs. Filter values must be validated (e.g., ASN must be positive).
- **TL-4 (CAN)**: Use `buf` for Protobuf. Already configured via `buf.gen.yaml`.

## Standard Stack

No new dependencies are introduced. All work uses existing libraries.

### Core (already in project)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| google.golang.org/protobuf | latest | Proto `optional` keyword support | Generates pointer types for `optional` fields in proto3 |
| entgo.io/ent | v0.14.5 | ORM with generated predicate functions | All `Where` predicates (`AsnEQ`, `NameContainsFold`, `StatusEQ`, etc.) already generated |
| connectrpc.com/connect | latest | ConnectRPC error codes | `connect.CodeInvalidArgument` for filter validation errors |
| buf.build/buf/cli | latest | Proto compilation and Go code generation | `buf generate` regenerates after proto changes |

## Architecture Patterns

### Current List Handler Pattern (before filtering)
```go
func (s *NetworkService) ListNetworks(ctx context.Context, req *pb.ListNetworksRequest) (*pb.ListNetworksResponse, error) {
    pageSize := normalizePageSize(req.GetPageSize())
    offset, err := decodePageToken(req.GetPageToken())
    if err != nil {
        return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid page_token: %w", err))
    }

    results, err := s.Client.Network.Query().
        Order(ent.Asc(network.FieldID)).
        Limit(pageSize + 1).
        Offset(offset).
        All(ctx)
    // ... pagination + conversion
}
```

### Pattern 1: Filter Application via Predicate Accumulation
**What:** Build a `[]predicate.Network` slice from present filter fields, then pass to `.Where(network.And(predicates...))`.
**When to use:** Every List handler that has filter fields.
**Example:**
```go
// Source: Codebase analysis + ent generated predicates
func (s *NetworkService) ListNetworks(ctx context.Context, req *pb.ListNetworksRequest) (*pb.ListNetworksResponse, error) {
    pageSize := normalizePageSize(req.GetPageSize())
    offset, err := decodePageToken(req.GetPageToken())
    if err != nil {
        return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid page_token: %w", err))
    }

    // Build filter predicates from optional fields.
    var predicates []predicate.Network
    if req.Asn != nil {
        if *req.Asn <= 0 {
            return nil, connect.NewError(connect.CodeInvalidArgument,
                fmt.Errorf("invalid filter: asn must be positive"))
        }
        predicates = append(predicates, network.AsnEQ(int(*req.Asn)))
    }
    if req.Name != nil {
        predicates = append(predicates, network.NameContainsFold(*req.Name))
    }
    if req.Country != nil {
        predicates = append(predicates, network.StatusEQ(*req.Country))
        // Actually: Would need a Country field on Network -- see note below
    }
    if req.Status != nil {
        predicates = append(predicates, network.StatusEQ(*req.Status))
    }
    if req.OrgId != nil {
        predicates = append(predicates, network.OrgIDEQ(int(*req.OrgId)))
    }

    query := s.Client.Network.Query().
        Order(ent.Asc(network.FieldID)).
        Limit(pageSize + 1).
        Offset(offset)

    if len(predicates) > 0 {
        query = query.Where(network.And(predicates...))
    }

    results, err := query.All(ctx)
    // ... pagination + conversion
}
```

### Pattern 2: Proto3 Optional Fields in services.proto
**What:** Add `optional` typed fields to List request messages starting at field number 3 (after page_size=1, page_token=2).
**Example:**
```protobuf
message ListNetworksRequest {
  int32 page_size = 1;
  string page_token = 2;
  optional int64 asn = 3;
  optional string name = 4;
  optional string status = 5;
  optional int64 org_id = 6;
}
```

**Go generated struct:**
```go
type ListNetworksRequest struct {
    PageSize  int32   `protobuf:"varint,1,..."`
    PageToken string  `protobuf:"bytes,2,..."`
    Asn       *int64  `protobuf:"varint,3,opt,..."`      // pointer = presence-detectable
    Name      *string `protobuf:"bytes,4,opt,..."`
    Status    *string `protobuf:"bytes,5,opt,..."`
    OrgId     *int64  `protobuf:"varint,6,opt,..."`
}
```

### Anti-Patterns to Avoid
- **Generic string-based filter map:** Don't use `map<string, string> filters`. This loses type safety and requires runtime parsing. The user decision locks this to typed fields.
- **Wrapper types for filter fields:** The existing entity messages use `google.protobuf.StringValue` etc., but filter fields on request messages should use the `optional` keyword instead. Wrapper types are for response fields where nil/empty distinction matters in the data model. Filter fields are simpler -- the `optional` keyword with pointer nil-check is sufficient and cleaner.
- **Putting filter logic in middleware/interceptor:** Filter fields are entity-specific. Keep filter application in each handler where the ent predicate types are known.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| String LIKE matching | Custom SQL or string operations | `network.NameContainsFold(v)` | ent generates case-insensitive substring predicates for every string field |
| Predicate composition (AND) | Manual SQL WHERE clause building | `network.And(predicates...)` | ent's `And()` composes any number of predicates safely |
| Proto presence detection | Custom flags or sentinel values | Proto3 `optional` keyword (generates `*T` pointer) | Native proto3 feature, nil = unset, non-nil = set |
| Input validation errors | Custom error types | `connect.NewError(connect.CodeInvalidArgument, ...)` | Standard gRPC/ConnectRPC error code for bad input |

## Filter Fields Per Entity Type

Based on analysis of ent schemas (indexed fields, FK relationships, and common query patterns), the recommended filter fields per entity:

| Entity | Filter Fields | Proto Types | Ent Predicates |
|--------|--------------|-------------|----------------|
| **Network** | asn, name, status, org_id | `optional int64`, `optional string`, `optional string`, `optional int64` | `AsnEQ`, `NameContainsFold`, `StatusEQ`, `OrgIDEQ` |
| **Facility** | name, country, city, status, org_id | `optional string` x4, `optional int64` | `NameContainsFold`, `CountryEQ`, `CityContainsFold`, `StatusEQ`, `OrgIDEQ` |
| **Organization** | name, country, city, status | `optional string` x4 | `NameContainsFold`, `CountryEQ`, `CityContainsFold`, `StatusEQ` |
| **InternetExchange** | name, country, city, status, org_id | `optional string` x4, `optional int64` | `NameContainsFold`, `CountryEQ`, `CityContainsFold`, `StatusEQ`, `OrgIDEQ` |
| **Campus** | name, country, city, status, org_id | `optional string` x4, `optional int64` | `NameContainsFold`, `CountryEQ`, `CityContainsFold`, `StatusEQ`, `OrgIDEQ` |
| **Carrier** | name, status, org_id | `optional string` x2, `optional int64` | `NameContainsFold`, `StatusEQ`, `OrgIDEQ` |
| **CarrierFacility** | carrier_id, fac_id, status | `optional int64` x2, `optional string` | `CarrierIDEQ`, `FacIDEQ`, `StatusEQ` |
| **IxFacility** | ix_id, fac_id, country, city, status | `optional int64` x2, `optional string` x3 | `IxIDEQ`, `FacIDEQ`, `CountryEQ`, `CityContainsFold`, `StatusEQ` |
| **IxLan** | ix_id, name, status | `optional int64`, `optional string` x2 | `IxIDEQ`, `NameContainsFold`, `StatusEQ` |
| **IxPrefix** | ixlan_id, protocol, status | `optional int64`, `optional string` x2 | `IxlanIDEQ`, `ProtocolEQ`, `StatusEQ` |
| **NetworkFacility** | net_id, fac_id, country, city, status | `optional int64` x2, `optional string` x3 | `NetIDEQ`, `FacIDEQ`, `CountryEQ`, `CityContainsFold`, `StatusEQ` |
| **NetworkIxLan** | net_id, ixlan_id, asn, name, status | `optional int64` x3, `optional string` x2 | `NetIDEQ`, `IxlanIDEQ`, `AsnEQ`, `NameContainsFold`, `StatusEQ` |
| **Poc** | net_id, role, name, status | `optional int64`, `optional string` x3 | `NetIDEQ`, `RoleEQ`, `NameContainsFold`, `StatusEQ` |

### Field Selection Rationale
- **status**: Present on all entities -- universally useful filter (most queries want `status=ok`)
- **name**: Present on entities that have a name -- LIKE matching for search
- **country/city**: Geographic filters for location-based entities
- **FK IDs** (org_id, net_id, fac_id, ix_id, etc.): Filter by parent entity -- the most common API query pattern
- **asn**: Network-specific identifier, extremely common query
- **role**: Poc-specific, to find contacts by role type
- **protocol**: IxPrefix-specific, to filter IPv4 vs IPv6 prefixes

### String Matching Strategy
Per locked decision:
- **Name fields** (`name`): Use `NameContainsFold` -- case-insensitive substring match (LIKE '%value%')
- **Code fields** (`country`, `status`, `protocol`, `role`): Use exact match (`CountryEQ`, `StatusEQ`) -- these are short coded values where exact match is expected
- **City fields** (`city`): Use `CityContainsFold` -- case-insensitive substring for partial city name matching

### Numeric Validation Rules
- **asn**: Must be positive (> 0). Return `INVALID_ARGUMENT: invalid filter: asn must be positive`
- **FK IDs** (org_id, net_id, fac_id, etc.): Must be positive (> 0). Return `INVALID_ARGUMENT: invalid filter: {field} must be positive`

## Common Pitfalls

### Pitfall 1: Proto Field Number Conflicts
**What goes wrong:** Adding filter fields with field numbers that conflict with existing or future fields.
**Why it happens:** The List request messages currently use field numbers 1 (page_size) and 2 (page_token). New filter fields must start at 3.
**How to avoid:** Use sequential field numbers starting at 3 for each List request message. Each message has its own field number space.
**Warning signs:** buf lint or protoc errors about duplicate field numbers.

### Pitfall 2: Zero Value vs. Absent Distinction
**What goes wrong:** Without `optional`, a proto3 `int64` field set to 0 is indistinguishable from an unset field (both are the zero value). A client cannot filter for "asn = 0" (which is meaningless) but more critically, cannot distinguish "no filter" from "filter by zero."
**Why it happens:** Proto3 defaults to implicit presence (no distinction between default and unset).
**How to avoid:** The `optional` keyword generates pointer types in Go. A nil pointer means "not set" (no filter), a non-nil pointer means "apply this filter." This is the locked decision.
**Warning signs:** Tests pass when they should fail because zero-value filters are silently ignored.

### Pitfall 3: Network Has No Country Field
**What goes wrong:** The CONTEXT.md examples mention "country" as a Network filter, but the Network ent schema has no `country` field. Networks are associated with countries only through their facilities or IX connections.
**Why it happens:** PeeringDB networks don't have a direct country attribute.
**How to avoid:** Do NOT add a `country` filter to ListNetworksRequest. The example in CONTEXT.md was illustrative. Only add filter fields that correspond to actual ent schema fields with database indexes.
**Warning signs:** Compile error when trying to reference `network.CountryEQ`.

### Pitfall 4: ent Nullable Int Fields Use *int
**What goes wrong:** FK fields like `org_id` are `Optional().Nillable()` in ent, meaning the Go type is `*int`. The ent predicate `OrgIDEQ(v int)` takes a plain `int`, not `*int`. But the nullable fields also have `OrgIDIsNil()` and `OrgIDNotNil()` predicates.
**Why it happens:** ent generates EQ predicates that match non-null values. Filtering by `org_id=5` correctly finds records where org_id is 5 (not null).
**How to avoid:** Use `OrgIDEQ(int(*req.OrgId))` -- dereference the proto optional pointer and convert to int. This correctly queries for the specified value.
**Warning signs:** Type mismatch compile errors.

### Pitfall 5: Regeneration Required After Proto Changes
**What goes wrong:** Proto file is updated but `buf generate` is not run, so Go types don't have the new filter fields.
**Why it happens:** Code generation is a manual step.
**How to avoid:** Run `buf generate` after every proto change. The ConnectRPC interfaces will update automatically since the handler method signatures use the generated request/response types.
**Warning signs:** Missing field errors in Go code, stale generated files.

### Pitfall 6: ContainsFold vs EQ for String Fields
**What goes wrong:** Using case-insensitive LIKE for country codes returns unexpected results (e.g., `country=us` matching "US" is OK, but also matching records with "us" embedded in other values would be wrong if using Contains instead of EqualFold).
**Why it happens:** Confusion between `ContainsFold` (substring LIKE) and `EqualFold` (case-insensitive exact match).
**How to avoid:** For code fields (country, status, role, protocol), use `EqualFold` for case-insensitive exact match. For name fields, use `ContainsFold` for substring search. Country codes are 2-letter ISO codes -- `EqualFold` is correct.
**Warning signs:** Filtering by country "US" also returns records for countries containing "us" in other field values. (Not actually likely with `CountryEqualFold` since country is its own column, but the principle matters.)

## Code Examples

### Proto: Adding Filter Fields to a List Request
```protobuf
// Source: services.proto pattern analysis
message ListNetworksRequest {
  int32 page_size = 1;
  string page_token = 2;

  // Filter fields -- all optional for presence detection.
  optional int64 asn = 3;
  optional string name = 4;
  optional string status = 5;
  optional int64 org_id = 6;
}
```

### Go: Filter Predicate Accumulation Pattern
```go
// Source: ent/network/where.go generated predicates
func (s *NetworkService) ListNetworks(ctx context.Context, req *pb.ListNetworksRequest) (*pb.ListNetworksResponse, error) {
    pageSize := normalizePageSize(req.GetPageSize())
    offset, err := decodePageToken(req.GetPageToken())
    if err != nil {
        return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid page_token: %w", err))
    }

    // Build filter predicates from optional fields.
    var predicates []predicate.Network
    if req.Asn != nil {
        if *req.Asn <= 0 {
            return nil, connect.NewError(connect.CodeInvalidArgument,
                fmt.Errorf("invalid filter: asn must be positive"))
        }
        predicates = append(predicates, network.AsnEQ(int(*req.Asn)))
    }
    if req.Name != nil {
        predicates = append(predicates, network.NameContainsFold(*req.Name))
    }
    if req.Status != nil {
        predicates = append(predicates, network.StatusEQ(*req.Status))
    }
    if req.OrgId != nil {
        if *req.OrgId <= 0 {
            return nil, connect.NewError(connect.CodeInvalidArgument,
                fmt.Errorf("invalid filter: org_id must be positive"))
        }
        predicates = append(predicates, network.OrgIDEQ(int(*req.OrgId)))
    }

    query := s.Client.Network.Query().
        Order(ent.Asc(network.FieldID)).
        Limit(pageSize + 1).
        Offset(offset)
    if len(predicates) > 0 {
        query = query.Where(network.And(predicates...))
    }

    results, err := query.All(ctx)
    if err != nil {
        return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list networks: %w", err))
    }

    // ... pagination + conversion (unchanged)
}
```

### Go: Table-Driven Filter Test
```go
// Source: Existing test patterns in grpcserver_test.go
func TestListNetworksFilters(t *testing.T) {
    t.Parallel()
    ctx := context.Background()
    client := testutil.SetupClient(t)
    now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

    // Seed test data with distinct values for filtering.
    client.Network.Create().SetID(1).SetName("Google").SetAsn(15169).
        SetStatus("ok").SetCreated(now).SetUpdated(now).
        SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
        SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).
        SetAllowIxpUpdate(false).SaveX(ctx)
    client.Network.Create().SetID(2).SetName("Cloudflare").SetAsn(13335).
        SetStatus("ok").SetCreated(now).SetUpdated(now).
        SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
        SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).
        SetAllowIxpUpdate(false).SaveX(ctx)
    client.Network.Create().SetID(3).SetName("Deleted Net").SetAsn(64512).
        SetStatus("deleted").SetCreated(now).SetUpdated(now).
        SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
        SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).
        SetAllowIxpUpdate(false).SaveX(ctx)

    svc := &NetworkService{Client: client}

    tests := []struct {
        name    string
        req     *pb.ListNetworksRequest
        wantLen int
        wantErr connect.Code
    }{
        {
            name:    "filter by ASN",
            req:     &pb.ListNetworksRequest{Asn: proto.Int64(15169)},
            wantLen: 1,
        },
        {
            name:    "filter by status",
            req:     &pb.ListNetworksRequest{Status: proto.String("ok")},
            wantLen: 2,
        },
        {
            name:    "filter by name substring",
            req:     &pb.ListNetworksRequest{Name: proto.String("cloud")},
            wantLen: 1, // case-insensitive match on "Cloudflare"
        },
        {
            name:    "combined filters (AND)",
            req:     &pb.ListNetworksRequest{Status: proto.String("ok"), Asn: proto.Int64(15169)},
            wantLen: 1,
        },
        {
            name:    "no matches",
            req:     &pb.ListNetworksRequest{Asn: proto.Int64(99999)},
            wantLen: 0,
        },
        {
            name:    "invalid ASN",
            req:     &pb.ListNetworksRequest{Asn: proto.Int64(-1)},
            wantErr: connect.CodeInvalidArgument,
        },
    }
    // ... run table-driven tests
}
```

### Proto Helper Functions
```go
// Use google.golang.org/protobuf/proto for creating optional values in tests:
import "google.golang.org/protobuf/proto"

req := &pb.ListNetworksRequest{
    Asn:    proto.Int64(15169),     // sets *int64 to non-nil
    Status: proto.String("ok"),     // sets *string to non-nil
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Wrapper types (`google.protobuf.Int64Value`) for optional fields | `optional` keyword in proto3 | Proto3 3.15.0 (Feb 2021) | Generates pointer types in Go, simpler than wrapper messages |
| No filter support on List RPCs | Typed optional filter fields on request messages | This phase | Enables server-side filtering instead of client-side |

**Key note on proto API versions:** This project uses the standard Open API (protoc-gen-go), not the newer Opaque API. The `optional` keyword generates `*T` pointer fields, not `Has_()` methods. Presence is checked via `field != nil`. The CONTEXT.md mentions `has_` methods which are from the Opaque API -- the equivalent in the Open API is the nil pointer check.

## Open Questions

1. **EqualFold vs ContainsFold for name fields**
   - What we know: The locked decision says "case-insensitive LIKE matching for name/string fields." `ContainsFold` does case-insensitive substring matching. `EqualFold` does case-insensitive exact matching.
   - What's unclear: "LIKE matching" most naturally maps to `ContainsFold` (substring), which is likely the intent for name searches.
   - Recommendation: Use `ContainsFold` for name fields (substring search), `EqualFold` for code fields (country, status) where exact match is expected but case tolerance is reasonable.

2. **Country field on Network entity**
   - What we know: The Network ent schema has no `country` field. The CONTEXT.md example was illustrative.
   - Recommendation: Omit `country` from ListNetworksRequest. The filter fields table above reflects only fields that exist in the ent schema.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) |
| Config file | None (stdlib) |
| Quick run command | `go test ./internal/grpcserver/ -run TestList -race -count=1` |
| Full suite command | `go test -race ./...` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| API-03a | Single filter field returns matching results | unit | `go test ./internal/grpcserver/ -run TestListNetworksFilters -race -count=1` | Wave 0 |
| API-03b | Multiple filters combine with AND logic | unit | `go test ./internal/grpcserver/ -run TestListNetworksFilters/combined -race -count=1` | Wave 0 |
| API-03c | Invalid filter values return INVALID_ARGUMENT | unit | `go test ./internal/grpcserver/ -run TestListNetworksFilters/invalid -race -count=1` | Wave 0 |
| API-03d | All 13 entity types support their respective filter fields | unit | `go test ./internal/grpcserver/ -run TestList.*Filter -race -count=1` | Wave 0 |
| API-03e | Filters compose correctly with pagination | unit | `go test ./internal/grpcserver/ -run TestList.*Filter.*paginated -race -count=1` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/grpcserver/ -race -count=1`
- **Per wave merge:** `go test -race ./...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- None -- existing test infrastructure (`testutil.SetupClient`, table-driven patterns in `grpcserver_test.go`) covers all phase requirements. New test functions will be added alongside the handler changes.

## Sources

### Primary (HIGH confidence)
- Codebase analysis: `proto/peeringdb/v1/services.proto` -- current List request message structure (13 services, field numbers 1-2)
- Codebase analysis: `internal/grpcserver/*.go` -- current List handler pattern (query builder + pagination)
- Codebase analysis: `ent/schema/*.go` -- all 13 entity schemas with field types, indexes, and annotations
- Codebase analysis: `ent/network/where.go` -- generated predicate functions (AsnEQ, NameContainsFold, StatusEQ, etc.)
- Codebase analysis: `gen/peeringdb/v1/peeringdbv1connect/services.connect.go` -- ConnectRPC simple mode interface definitions
- [Proto3 Optional Fields Go Generated Code](https://protobuf.dev/reference/go/go-generated/) -- proto3 `optional` generates pointer types in Open API

### Secondary (MEDIUM confidence)
- [Proto3 Optional Field Presence](https://protobuf.dev/programming-guides/proto3/) -- `optional` keyword enables explicit presence tracking

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- no new dependencies, all tools already in project
- Architecture: HIGH -- pattern is straightforward predicate accumulation on existing ent queries
- Filter fields per entity: HIGH -- derived directly from ent schema field definitions and database indexes
- Pitfalls: HIGH -- validated against actual codebase (e.g., Network has no country field)

**Research date:** 2026-03-25
**Valid until:** 2026-04-25 (stable -- no fast-moving dependencies)
