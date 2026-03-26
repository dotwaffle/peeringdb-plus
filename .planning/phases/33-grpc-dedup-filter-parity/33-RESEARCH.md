# Phase 33: gRPC Deduplication & Filter Parity - Research

**Researched:** 2026-03-26
**Domain:** Go generics, ConnectRPC handler deduplication, protobuf schema evolution, ent query abstraction
**Confidence:** HIGH

## Summary

The `internal/grpcserver/` package contains 13 per-entity service handler files totaling 2,879 lines of handler code (excluding tests, convert.go, and pagination.go). Every file follows an identical 3-method pattern (Get/List/Stream) with the only variation being: (1) the ent entity type and predicate package, (2) the proto request/response message types, (3) the filter fields on List/Stream, and (4) the entity-to-proto conversion function. This is a textbook case for Go generics with callback functions.

The filter parity gap between pdbcompat (PeeringDB compat layer) and ConnectRPC is significant. The compat layer exposes every field on every entity as filterable (via Django-style query parameters), while ConnectRPC List/Stream messages only expose a handful of fields (typically 3-7 per entity). The CONTEXT.md decision constrains this: match every filterable field from pdbcompat, but use proper proto types.

**Primary recommendation:** Extract generic `ListEntities` and `StreamEntities` functions parameterized by `[E any, P any]` with callback structs. Each per-type file shrinks to ~30-50 lines: service struct, Get method, filter function, and callback wiring. Proto request messages need ~70 new optional fields across all 13 entities to reach parity. Total effort is moderate -- the pattern is mechanical and well-understood.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **Generic Approach:** Go generics with type parameters for ent entity and proto message types. Pass query/convert/filter functions as callback parameters. Each per-type service file becomes ~30 lines: create callbacks, call generic List/Stream. NOT interface-based dispatch, NOT code generation.
- **Callback Signature Pattern:** ListParams struct with Query, Count, Convert, and ApplyFilters function fields.
- **Filter Application:** Per-type ApplyFilters function. Each service file keeps its own `applyNetworkFilters(req, query)` function. Type-specific, explicit, grep-friendly. Do NOT try to make filter application generic.
- **ConnectRPC Filter Parity:** Match every filterable field that PeeringDB compat exposes. Use proper proto types: int64 for IDs, google.protobuf.Timestamp for dates (not string-only). Requires updating proto request messages in services.proto, then buf generate, then update per-type filter functions.
- **Test Strategy:** Integration tests with in-memory SQLite. Unit tests for filter application and conversion functions. Target: grpcserver 60%+ coverage, middleware 60%+ coverage.

### Scope Boundaries (Locked)
- Do NOT add filters that PeeringDB compat doesn't have (beyond typed improvements)
- Do NOT change the proto service definitions (Get/List/Stream RPCs) -- only request message fields
- Do NOT refactor the conversion helpers in convert.go -- they work and are all used
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| QUAL-01 | gRPC service handlers share a generic List/Stream implementation, eliminating ~1,154 lines of duplicated logic across 13 files | Generic helper pattern documented in Architecture section; line count analysis shows 2,879 lines of handler code reducible to ~600-800 lines |
| QUAL-03 | Test coverage for grpcserver reaches 60%+ and middleware reaches 60%+ | Current coverage: grpcserver 22.2%, middleware 26.7%. Gap analysis in Validation Architecture; test strategy documented |
| ARCH-02 | ConnectRPC List RPCs expose the same filterable fields as PeeringDB compat layer for each entity type | Complete filter parity gap analysis in Filter Parity section; ~70 proto fields to add across 26 request messages |
</phase_requirements>

## Standard Stack

No new dependencies. This phase uses the existing stack exclusively.

### Core (Already Installed)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go generics | Go 1.26 | Type-parameterized List/Stream helpers | stdlib feature, zero dependency |
| connectrpc.com/connect | existing | ConnectRPC handler interfaces | Already in use for all 13 services |
| entgo.io/ent | v0.14.5 | ORM, query builders, predicate types | Already generates all entity and predicate types |
| buf.build/buf/cli | existing | Proto code generation | Required after services.proto changes |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| google/protobuf/timestamp.proto | existing | Typed date filters in proto | Already imported; new filter fields use Timestamp instead of string |
| enttest | existing | In-memory SQLite test clients | Integration tests for generic helpers |
| google.golang.org/protobuf/proto | existing | proto.Int64, proto.String for test construction | Test helper for optional field construction |

## Architecture Patterns

### Current Structure (Before)
```
internal/grpcserver/
  campus.go            212 lines  (Get + List + Stream + campusToProto)
  carrier.go           197 lines  (Get + List + Stream + carrierToProto)
  carrierfacility.go   201 lines  (Get + List + Stream + carrierFacilityToProto)
  facility.go          235 lines  (Get + List + Stream + facilityToProto)
  internetexchange.go  233 lines  (Get + List + Stream + internetExchangeToProto)
  ixfacility.go        215 lines  (Get + List + Stream + ixFacilityToProto)
  ixlan.go             197 lines  (Get + List + Stream + ixLanToProto)
  ixprefix.go          194 lines  (Get + List + Stream + ixPrefixToProto)
  network.go           239 lines  (Get + List + Stream + networkToProto)
  networkfacility.go   217 lines  (Get + List + Stream + networkFacilityToProto)
  networkixlan.go      233 lines  (Get + List + Stream + networkIxLanToProto)
  organization.go      205 lines  (Get + List + Stream + organizationToProto)
  poc.go               201 lines  (Get + List + Stream + pocToProto)
  convert.go            76 lines  (shared helpers, DO NOT CHANGE)
  pagination.go         63 lines  (shared pagination, DO NOT CHANGE)
  --- Handler files total: 2,879 lines ---
```

### Target Structure (After)
```
internal/grpcserver/
  generic.go           ~120 lines (ListEntities + StreamEntities generic functions)
  campus.go            ~50 lines  (Get + applyFilters + callback wiring)
  carrier.go           ~45 lines
  carrierfacility.go   ~40 lines
  facility.go          ~55 lines
  internetexchange.go  ~55 lines
  ixfacility.go        ~50 lines
  ixlan.go             ~45 lines
  ixprefix.go          ~45 lines
  network.go           ~60 lines  (most filters)
  networkfacility.go   ~50 lines
  networkixlan.go      ~55 lines
  organization.go      ~50 lines
  poc.go               ~45 lines
  convert.go            76 lines  (unchanged)
  pagination.go         63 lines  (unchanged)
  --- Handler files total: ~900 lines ---
  --- Reduction: ~1,979 lines (68% reduction) ---
```

### Pattern 1: Generic List Helper

The List helper abstracts the pagination, query execution, error handling, and result conversion. The only type-specific parts are passed as callbacks.

```go
// ListParams holds the type-specific callbacks for a paginated list query.
type ListParams[E any, P any] struct {
    // EntityName is used in error messages (e.g. "network", "facility").
    EntityName string
    // ApplyFilters validates filter fields on the request and returns typed
    // ent predicates plus any validation error. This is the only type-specific
    // logic -- each entity defines its own filter function.
    ApplyFilters func() ([]func(*sql.Selector), error)
    // Query creates an ent query builder with the given predicates, limit,
    // and offset already applied, ordered by ID ascending.
    Query func(ctx context.Context, predicates []func(*sql.Selector), limit, offset int) ([]*E, error)
    // Convert transforms a single ent entity to its proto message.
    Convert func(*E) *P
    // GetID extracts the integer ID from an ent entity (for keyset pagination).
    GetID func(*E) int
}
```

**Key insight:** The ent query types (`NetworkQuery`, `CarrierQuery`, etc.) share no common Go interface -- they are separate generated types. Therefore we cannot abstract the query builder itself. Instead, each per-type file provides a closure that creates and executes the query. The predicate types (`predicate.Network`, `predicate.Carrier`) are all `func(*sql.Selector)` under the hood, which is how pdbcompat already handles this via `castPredicates`.

### Pattern 2: Generic Stream Helper

The Stream helper abstracts the timeout setup, count query, header metadata, keyset pagination loop, and batch sending. Callbacks provide type-specific query and conversion.

```go
// StreamParams holds the type-specific callbacks for a streaming query.
type StreamParams[E any, P any] struct {
    EntityName   string
    ApplyFilters func() ([]func(*sql.Selector), error)
    Count        func(ctx context.Context, predicates []func(*sql.Selector)) (int, error)
    QueryBatch   func(ctx context.Context, predicates []func(*sql.Selector), afterID, limit int) ([]*E, error)
    Convert      func(*E) *P
    GetID        func(*E) int
    SinceID      *int64   // from request
    UpdatedSince *timestamppb.Timestamp // from request
}
```

### Pattern 3: Per-Type File After Refactor

Each per-type file retains: (1) the service struct, (2) the Get method (simple, not worth abstracting further), (3) the applyFilters function, (4) the toProto conversion function, and (5) thin List/Stream methods that create callbacks and call the generic helpers.

```go
// In network.go (after refactor, ~60 lines including filters)
func (s *NetworkService) ListNetworks(ctx context.Context, req *pb.ListNetworksRequest) (*pb.ListNetworksResponse, error) {
    result, nextToken, err := ListEntities(ctx, ListParams[ent.Network, pb.Network]{
        EntityName:   "network",
        PageSize:     req.GetPageSize(),
        PageToken:    req.GetPageToken(),
        ApplyFilters: func() ([]func(*sql.Selector), error) {
            return applyNetworkFilters(req)
        },
        Query: func(ctx context.Context, preds []func(*sql.Selector), limit, offset int) ([]*ent.Network, error) {
            q := s.Client.Network.Query().
                Order(ent.Asc(network.FieldID)).
                Limit(limit).Offset(offset)
            if len(preds) > 0 {
                typedPreds := castPredicates[predicate.Network](preds)
                q = q.Where(network.And(typedPreds...))
            }
            return q.All(ctx)
        },
        Convert: networkToProto,
        GetID:   func(n *ent.Network) int { return n.ID },
    })
    if err != nil {
        return nil, err
    }
    return &pb.ListNetworksResponse{Networks: result, NextPageToken: nextToken}, nil
}
```

### Anti-Patterns to Avoid
- **Abstracting Get methods:** Get is 5 lines per type. Abstracting saves ~50 lines across 13 files but introduces interface complexity for minimal gain. Keep Get concrete per CONTEXT.md.
- **Making filter application generic:** CONTEXT.md explicitly forbids this. Each entity has unique fields with different types and validation rules. Keep `applyNetworkFilters`, `applyFacilityFilters`, etc. as explicit functions.
- **Sharing predicate types across ent entities:** `predicate.Network` and `predicate.Carrier` are distinct types even though they're both `func(*sql.Selector)` underneath. Use `castPredicates` from pdbcompat pattern for conversion.
- **Over-abstracting the response construction:** Each List response has a different repeated field name (`Networks`, `Facilities`, etc.). The caller must construct the response -- the generic helper returns `[]*P` and a page token.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Predicate type conversion | Manual per-entity cast loops | `castPredicates[T]` generic from pdbcompat | Already exists in codebase, handles all 13 types |
| Page token encoding | New encoding scheme | Existing `encodePageToken`/`decodePageToken` | Already battle-tested, keep it |
| In-memory test database | Mock interfaces | `testutil.SetupClient(t)` + `enttest` | Real SQLite with auto-migration, already working |

## Filter Parity Gap Analysis

This is the core of ARCH-02. Below is every field in the pdbcompat Registry compared to the current ConnectRPC proto request messages. Fields marked "MISSING" need to be added to services.proto.

### Organization (pdbcompat: 17 filterable fields, proto: 4)
| Field | pdbcompat Type | Proto Status | Proto Type |
|-------|---------------|-------------|------------|
| id | FieldInt | MISSING | optional int64 |
| name | FieldString | EXISTS | optional string |
| aka | FieldString | MISSING | optional string |
| name_long | FieldString | MISSING | optional string |
| website | FieldString | MISSING | optional string |
| notes | FieldString | MISSING | optional string |
| logo | FieldString | MISSING | optional string |
| address1 | FieldString | MISSING | optional string |
| address2 | FieldString | MISSING | optional string |
| city | FieldString | EXISTS | optional string |
| state | FieldString | MISSING | optional string |
| country | FieldString | EXISTS | optional string |
| zipcode | FieldString | MISSING | optional string |
| suite | FieldString | MISSING | optional string |
| floor | FieldString | MISSING | optional string |
| latitude | FieldFloat | SKIP (low utility) | - |
| longitude | FieldFloat | SKIP (low utility) | - |
| created | FieldTime | SKIP (use updated_since) | - |
| updated | FieldTime | SKIP (use updated_since) | - |
| status | FieldString | EXISTS | optional string |

**Missing fields to add: 11** (aka, name_long, website, notes, logo, address1, address2, state, zipcode, suite, floor)

### Network (pdbcompat: 28 filterable fields, proto: 4)
| Field | pdbcompat Type | Proto Status | Action |
|-------|---------------|-------------|--------|
| id | FieldInt | MISSING | Add optional int64 |
| org_id | FieldInt | EXISTS | - |
| name | FieldString | EXISTS | - |
| aka | FieldString | MISSING | Add optional string |
| name_long | FieldString | MISSING | Add optional string |
| website | FieldString | MISSING | Add optional string |
| asn | FieldInt | EXISTS | - |
| looking_glass | FieldString | MISSING | Add optional string |
| route_server | FieldString | MISSING | Add optional string |
| irr_as_set | FieldString | MISSING | Add optional string |
| info_type | FieldString | MISSING | Add optional string |
| info_prefixes4 | FieldInt | MISSING | Add optional int64 |
| info_prefixes6 | FieldInt | MISSING | Add optional int64 |
| info_traffic | FieldString | MISSING | Add optional string |
| info_ratio | FieldString | MISSING | Add optional string |
| info_scope | FieldString | MISSING | Add optional string |
| info_unicast | FieldBool | MISSING | Add optional bool |
| info_multicast | FieldBool | MISSING | Add optional bool |
| info_ipv6 | FieldBool | MISSING | Add optional bool |
| info_never_via_route_servers | FieldBool | MISSING | Add optional bool |
| notes | FieldString | MISSING | Add optional string |
| policy_url | FieldString | MISSING | Add optional string |
| policy_general | FieldString | MISSING | Add optional string |
| policy_locations | FieldString | MISSING | Add optional string |
| policy_ratio | FieldBool | MISSING | Add optional bool |
| policy_contracts | FieldString | MISSING | Add optional string |
| allow_ixp_update | FieldBool | MISSING | Add optional bool |
| status | FieldString | EXISTS | - |
| (others: time/count fields) | various | SKIP | - |

**Missing fields to add: ~22**

### Facility (pdbcompat: 27 filterable fields, proto: 5)
**Missing fields to add: ~18** (org_id, org_name, aka, name_long, website, clli, rencode, npanxx, tech_email, tech_phone, sales_email, sales_phone, property, diverse_serving_substations, notes, region_continent, status_dashboard, state, zipcode, suite, floor, address1, address2 -- scope to the most useful; many are low-utility)

### InternetExchange (pdbcompat: 22 filterable fields, proto: 5)
**Missing fields to add: ~13** (aka, name_long, region_continent, media, notes, proto_unicast, proto_multicast, proto_ipv6, website, tech_email, policy_email, service_level, terms)

### POC (pdbcompat: 8 filterable fields, proto: 4)
**Missing fields to add: 4** (visible, phone, email, url)

### IxLan (pdbcompat: 9 filterable fields, proto: 3)
**Missing fields to add: 4** (descr, mtu, dot1q_support, rs_asn)

### IxPrefix (pdbcompat: 5 filterable fields, proto: 3)
**Missing fields to add: 2** (prefix, in_dfz)

### NetworkIxLan (pdbcompat: 12 filterable fields, proto: 5)
**Missing fields to add: 7** (ix_id, speed, ipaddr4, ipaddr6, is_rs_peer, bfd_support, operational)

### NetworkFacility (pdbcompat: 6 filterable fields, proto: 5)
**Missing fields to add: 1** (local_asn)

### IxFacility (pdbcompat: 5 filterable fields, proto: 5)
**Missing fields to add: 1** (name)

### Carrier (pdbcompat: 8 filterable fields, proto: 3)
**Missing fields to add: 5** (aka, name_long, website, notes, org_name)

### CarrierFacility (pdbcompat: 4 filterable fields, proto: 3)
**Missing fields to add: 1** (name)

### Campus (pdbcompat: 12 filterable fields, proto: 5)
**Missing fields to add: 7** (org_name, name_long, aka, website, notes, state, zipcode)

### Total Proto Field Additions
Across all 13 entity types (List + Stream request messages = 26 messages total): approximately **~96 new optional fields**. Each field must appear in both the List and Stream request messages.

**Note on scope:** CONTEXT.md says "Match every filterable field that PeeringDB compat exposes." The pdbcompat Registry defines exactly which fields are filterable. Some low-utility fields (latitude/longitude, created/updated timestamps for exact match) could be argued as unnecessary, but the requirement is clear. Implement all fields from the Registry for full parity.

## Common Pitfalls

### Pitfall 1: Ent Predicate Type Safety
**What goes wrong:** Go's type system treats `predicate.Network` and `predicate.Carrier` as distinct types even though they're both `func(*sql.Selector)` under the hood. You cannot pass `[]predicate.Network` where `[]func(*sql.Selector)` is expected without explicit conversion.
**Why it happens:** Ent defines type aliases for each entity's predicates to prevent cross-entity predicate mixing.
**How to avoid:** Use the `castPredicates[T]` generic function already in the codebase (pdbcompat/registry_funcs.go). The generic helper works with `[]func(*sql.Selector)` internally, and each per-type file converts at the boundary.
**Warning signs:** Compile errors like "cannot use predicates (variable of type []func(*sql.Selector)) as []predicate.Network"

### Pitfall 2: Proto Field Number Conflicts
**What goes wrong:** Adding new optional fields to proto request messages with duplicate or out-of-sequence field numbers breaks wire format compatibility.
**Why it happens:** Existing messages already use field numbers 1-7 for the current filter fields. New fields must use higher numbers.
**How to avoid:** Start new fields after the highest existing field number per message. Check each message individually -- they have different current maximums.
**Warning signs:** `buf lint` failures, `buf breaking` violations

### Pitfall 3: ConnectRPC Optional Field Semantics
**What goes wrong:** Proto3 `optional` fields generate pointer types (`*string`, `*int64`, `*bool`) in Go. Checking `req.Asn` without checking `req.Asn != nil` causes nil pointer dereferences. Checking `req.GetAsn()` returns 0/"" for unset fields, which is ambiguous (0 could be a valid ASN filter).
**Why it happens:** Proto3's default zero values conflict with "field not set" semantics. The `optional` keyword was added specifically to distinguish "set to zero" from "not set."
**How to avoid:** Always check pointer != nil before dereferencing. The existing code does this correctly -- maintain the pattern.
**Warning signs:** Nil pointer panics in filter application functions.

### Pitfall 4: Stream Helper Must Not Double-Apply SinceID
**What goes wrong:** The Stream helper uses keyset pagination (`WHERE id > lastID`). If `SinceID` is also added as a predicate in the filter list, AND both are applied to the batch query, the keyset condition may conflict with the filter predicate.
**Why it happens:** The current code adds `SinceID` as a predicate AND initializes `lastID` from it. The batch query then adds another `IDGT(lastID)`. This is correct because the batch query's IDGT supersedes the predicate's IDGT once pagination begins, but it could confuse future maintainers.
**How to avoid:** In the generic Stream helper, handle `SinceID` and `UpdatedSince` outside the per-type filter function. The generic helper owns these fields because they exist on every Stream request message.
**Warning signs:** Duplicate IDGT predicates in SQL debug logs.

### Pitfall 5: Test Coverage Math
**What goes wrong:** Reducing code by deduplication can accidentally lower coverage percentage if the generic helper lines aren't covered by tests.
**Why it happens:** Coverage = (executed lines / total lines). If you extract 1,000 lines into 100 generic lines but only test via 3 entity types, the generic code is covered but the per-type filter functions for untested entities are not.
**How to avoid:** Write at least one List and one Stream test per entity type. Use table-driven tests grouped by entity to make this efficient. Current tests only cover Network, Facility, Organization, Poc, IxPrefix, NetworkIxLan, and CarrierFacility -- missing Campus, Carrier, InternetExchange, IxFacility, IxLan, NetworkFacility.
**Warning signs:** Coverage below 60% after refactor despite all generic code being tested.

### Pitfall 6: Middleware Coverage Gap
**What goes wrong:** Middleware package currently at 26.7% coverage. The CORS test (163 lines) covers cors.go well, but logging.go and recovery.go have minimal/no tests.
**Why it happens:** Logging and recovery middleware were added without corresponding tests.
**How to avoid:** Add table-driven tests for logging.go (verify slog output attributes, status code capture, trace correlation) and recovery.go (verify panic recovery returns 500, logs stack trace).
**Warning signs:** Coverage stuck below 60% even after adding a few tests.

## Code Examples

### Generic ListEntities Function
```go
// Source: Based on pattern analysis of all 13 existing handler files
func ListEntities[E any, P any](ctx context.Context, params ListParams[E, P]) ([]*P, string, error) {
    pageSize := normalizePageSize(params.PageSize)
    offset, err := decodePageToken(params.PageToken)
    if err != nil {
        return nil, "", connect.NewError(connect.CodeInvalidArgument,
            fmt.Errorf("invalid page_token: %w", err))
    }

    predicates, err := params.ApplyFilters()
    if err != nil {
        return nil, "", err // ApplyFilters returns connect errors directly
    }

    results, err := params.Query(ctx, predicates, pageSize+1, offset)
    if err != nil {
        return nil, "", connect.NewError(connect.CodeInternal,
            fmt.Errorf("list %s: %w", params.EntityName, err))
    }

    var nextPageToken string
    if len(results) > pageSize {
        results = results[:pageSize]
        nextPageToken = encodePageToken(offset + pageSize)
    }

    items := make([]*P, len(results))
    for i, e := range results {
        items[i] = params.Convert(e)
    }
    return items, nextPageToken, nil
}
```

### Per-Type Filter Function (Network Example)
```go
// Source: Derived from current network.go List filter logic
func applyNetworkListFilters(req *pb.ListNetworksRequest) ([]func(*sql.Selector), error) {
    var preds []func(*sql.Selector)
    if req.Asn != nil {
        if *req.Asn <= 0 {
            return nil, connect.NewError(connect.CodeInvalidArgument,
                fmt.Errorf("invalid filter: asn must be positive"))
        }
        preds = append(preds, sql.FieldEQ(network.FieldAsn, int(*req.Asn)))
    }
    if req.Name != nil {
        preds = append(preds, sql.FieldContainsFold(network.FieldName, *req.Name))
    }
    if req.Status != nil {
        preds = append(preds, sql.FieldEQ(network.FieldStatus, *req.Status))
    }
    if req.OrgId != nil {
        if *req.OrgId <= 0 {
            return nil, connect.NewError(connect.CodeInvalidArgument,
                fmt.Errorf("invalid filter: org_id must be positive"))
        }
        preds = append(preds, sql.FieldEQ(network.FieldOrgID, int(*req.OrgId)))
    }
    // ... new parity fields ...
    if req.InfoType != nil {
        preds = append(preds, sql.FieldEQ(network.FieldInfoType, *req.InfoType))
    }
    return preds, nil
}
```

### castPredicates Reuse
```go
// Source: internal/pdbcompat/registry_funcs.go line 40-46 (already exists)
// Move to a shared internal package or duplicate in grpcserver (small function).
func castPredicates[T ~func(*sql.Selector)](filters []func(*sql.Selector)) []T {
    out := make([]T, len(filters))
    for i, f := range filters {
        out[i] = T(f)
    }
    return out
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Copy-paste per entity | Go generics with callbacks | Go 1.18+ (2022) | Eliminates ~2,000 lines of duplication |
| String-only proto filters | `optional` typed fields | Proto3 optional (2020) | Distinguishes "unset" from "zero value" |
| Separate List/Stream predicate builders | Shared filter function returning `[]func(*sql.Selector)` | This phase | Each filter defined once, used by both List and Stream |

**Note on `sql.Selector` predicates vs typed predicates:** Using `sql.FieldEQ`/`sql.FieldContainsFold` directly (as pdbcompat does) instead of the typed ent predicates (e.g., `network.AsnEQ`) eliminates the need for per-entity predicate type imports in the filter functions. The trade-off is less compile-time type safety on field names, but the field names are string constants from the ent-generated package (`network.FieldAsn`). This is the pattern pdbcompat already uses successfully.

## Open Questions

1. **Filter function approach: sql.Selector vs typed predicates**
   - What we know: pdbcompat uses `sql.Selector` functions for all filtering. Current grpcserver uses typed predicates like `network.AsnEQ()`. Both work.
   - What's unclear: Whether to switch grpcserver to `sql.Selector` style (simpler generic helper, matches pdbcompat) or keep typed predicates (more type safety, more boilerplate in callback closures).
   - Recommendation: Use `sql.Selector` style in the filter functions. The field names are still compile-time constants from ent packages. This avoids needing `castPredicates` entirely in the Query callbacks -- the Query callback just applies `sql.Selector` functions directly via query modifiers.

2. **Proto field number allocation for new filter fields**
   - What we know: Each message has existing fields numbered sequentially. New fields must not conflict.
   - What's unclear: Exact field numbers depend on the current state of each message.
   - Recommendation: Use sequential numbers starting after the current highest per message. No backward compatibility concern since this is pre-1.0.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing stdlib (Go 1.26) |
| Config file | none (stdlib) |
| Quick run command | `TMPDIR=/tmp/claude-1000 go test -race ./internal/grpcserver/... ./internal/middleware/...` |
| Full suite command | `TMPDIR=/tmp/claude-1000 go test -race -cover ./internal/grpcserver/... ./internal/middleware/...` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| QUAL-01 | Generic List helper returns paginated results with correct page tokens | integration | `TMPDIR=/tmp/claude-1000 go test -race -run TestListEntities ./internal/grpcserver/ -x` | Wave 0 |
| QUAL-01 | Generic Stream helper streams all records in keyset-paginated batches | integration | `TMPDIR=/tmp/claude-1000 go test -race -run TestStreamEntities ./internal/grpcserver/ -x` | Wave 0 |
| QUAL-01 | Per-type service delegates to generic helper correctly | integration | `TMPDIR=/tmp/claude-1000 go test -race -run TestList ./internal/grpcserver/ -x` | Partial (existing tests cover 7/13 types) |
| QUAL-03 | grpcserver coverage >= 60% | coverage | `TMPDIR=/tmp/claude-1000 go test -race -cover ./internal/grpcserver/...` | Existing: 22.2% |
| QUAL-03 | middleware coverage >= 60% | coverage | `TMPDIR=/tmp/claude-1000 go test -race -cover ./internal/middleware/...` | Existing: 26.7% |
| ARCH-02 | Network List accepts info_type filter and returns filtered results | integration | `TMPDIR=/tmp/claude-1000 go test -race -run TestListNetworksFilters ./internal/grpcserver/ -x` | Partial (existing test covers some filters) |
| ARCH-02 | Every pdbcompat filterable field has corresponding proto optional field | manual inspection | `grep -c 'optional' proto/peeringdb/v1/services.proto` | Wave 0 |

### Sampling Rate
- **Per task commit:** `TMPDIR=/tmp/claude-1000 go test -race ./internal/grpcserver/... ./internal/middleware/...`
- **Per wave merge:** `TMPDIR=/tmp/claude-1000 go test -race -cover ./internal/grpcserver/... ./internal/middleware/...`
- **Phase gate:** Full suite green before `/gsd:verify-work`; both packages at 60%+ coverage

### Wave 0 Gaps
- [ ] Tests for 6 untested entity types: Campus, Carrier, InternetExchange, IxFacility, IxLan, NetworkFacility (List and Stream)
- [ ] Tests for generic ListEntities and StreamEntities helper functions
- [ ] Tests for middleware logging.go (slog output verification)
- [ ] Tests for middleware recovery.go (panic recovery, 500 response)
- [ ] Tests for new filter fields added for ARCH-02 parity

## Project Constraints (from CLAUDE.md)

**Applicable directives for this phase:**
- **CS-0 (MUST):** Modern Go code guidelines -- generics are appropriate here
- **CS-2 (MUST):** No stutter. The generic helper is in package grpcserver, so `ListEntities` not `GrpcListEntities`
- **CS-4 (SHOULD):** Prefer generics when it clarifies and speeds -- this is exactly that case
- **CS-5 (MUST):** Input structs for functions with >2 args. `ListParams` and `StreamParams` satisfy this
- **CS-6 (SHOULD):** Declare input structs before the function consuming them
- **ERR-1 (MUST):** Wrap errors with `%w` and context
- **T-1 (MUST):** Table-driven tests; deterministic and hermetic
- **T-2 (MUST):** Run `-race` in CI; add `t.Cleanup`
- **T-3 (SHOULD):** Mark safe tests with `t.Parallel()`
- **API-1 (MUST):** Document exported items
- **TL-4 (CAN):** Use `buf` for Protobuf (already in project)

## Sources

### Primary (HIGH confidence)
- Direct code analysis of `internal/grpcserver/*.go` (13 handler files, 2,879 handler lines)
- Direct code analysis of `internal/pdbcompat/registry.go` (filter field definitions for all 13 entity types)
- Direct code analysis of `proto/peeringdb/v1/services.proto` (current proto request message fields)
- `go test -cover` output for both packages (grpcserver: 22.2%, middleware: 26.7%)
- Go 1.26 generics support (confirmed via `go version`)

### Secondary (MEDIUM confidence)
- `ent/rest/list.go` PagableQuery interface pattern -- confirms ent query types lack a shared interface for generic use

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - no new dependencies, all existing
- Architecture: HIGH - pattern directly extracted from code analysis of all 13 files
- Filter parity gap: HIGH - exhaustive comparison of pdbcompat Registry vs services.proto
- Pitfalls: HIGH - based on known Go generics constraints and existing codebase patterns

**Research date:** 2026-03-26
**Valid until:** 2026-04-26 (stable -- no external dependencies changing)
