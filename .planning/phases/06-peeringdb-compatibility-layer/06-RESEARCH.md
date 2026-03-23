# Phase 6: PeeringDB Compatibility Layer - Research

**Researched:** 2026-03-22
**Domain:** PeeringDB-compatible REST API adapter -- Django-style filters, response envelope, depth expansion over ent ORM
**Confidence:** HIGH

## Summary

Phase 6 builds a hand-written PeeringDB API compatibility layer at `/api/` that reproduces PeeringDB's exact REST API behavior: URL paths (`/api/net`, `/api/ix/{id}`), response envelope (`{"meta": {}, "data": [...]}`), Django-style query filters (`__contains`, `__in`, `__lt`), depth-based relationship expansion, and `limit`/`skip` pagination. This is a separate handler tree from the entrest-generated REST API (Phase 5) -- it queries the ent client directly and serializes responses using custom Go structs that match PeeringDB's exact JSON field names.

The critical technical challenge is the generic filter parser (D-11). PeeringDB consumers use Django-style double-underscore filter suffixes (`?name__contains=Equinix`, `?asn__in=13335,174`) that must be parsed and translated to ent `sql.Field*` predicates. The key insight is that all 13 ent predicate types share the same underlying type (`func(*sql.Selector)`), so a single generic parser can construct predicates using `sql.FieldEQ`, `sql.FieldContains`, `sql.FieldGT`, etc. with string field names, then cast to the appropriate predicate type per entity.

The second challenge is response serialization. Ent's generated Go structs use `omitempty` JSON tags and slightly different field naming conventions. PeeringDB's response format includes all fields (including zero values) and requires specific snake_case names. The existing `internal/peeringdb/types.go` structs already have the correct JSON tags -- these should be reused as the serialization target, with ent query results mapped to these structs before JSON encoding.

**Primary recommendation:** Build `internal/pdbcompat/` package with four components: (1) a type registry mapping PeeringDB type names to ent query builders and field metadata, (2) a generic filter parser using `sql.Field*` predicates, (3) response serializers that map ent entities to PeeringDB-format JSON structs, and (4) HTTP handlers for list/detail endpoints. Use the existing `peeringdb.Response[T]` envelope type for output serialization.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Exact PeeringDB paths: /api/net, /api/ix, /api/fac, /api/org, /api/poc, /api/ixlan, /api/ixpfx, /api/netixlan, /api/netfac, /api/ixfac, /api/carrier, /api/carrierfac, /api/campus
- **D-02:** Accept both with and without trailing slash (/api/net and /api/net/) -- more forgiving, matches PeeringDB
- **D-03:** Single object by ID: /api/net/42 -- exact PeeringDB path format
- **D-04:** Exact PeeringDB field names in snake_case -- audit ent JSON tags against PeeringDB's actual response and use custom serializers where they differ
- **D-05:** Always return results sorted by ID -- matches PeeringDB's default behavior, no sort parameter
- **D-06:** depth=0 (default): flat objects with FK IDs only (e.g. org_id: 42)
- **D-07:** depth=2: replaces FK ID with full nested object AND includes reverse _set fields (e.g. net_set, fac_set on org) -- exact PeeringDB behavior
- **D-08:** Use ent's With* eager-loading to fetch related objects for depth=2, then serialize the full graph
- **D-09:** Core filter set: __contains, __startswith, __in, __lt, __gt, __lte, __gte, exact match (no suffix) -- covers most common use cases
- **D-10:** Case-insensitive matching -- use SQLite COLLATE NOCASE or LIKE to match PeeringDB's behavior
- **D-11:** Generic filter parser -- one parser handles any field + operator + value, validates field exists, maps operator to ent predicate. Reusable across all 13 types.
- **D-12:** Support ?status= filter including status=deleted -- matches PeeringDB behavior
- **D-13:** Implement ?q= full-text-like search across name fields (and type-specific fields like ASN, irr_as_set)
- **D-14:** Implement ?fields= for field projection -- ?fields=id,name,asn returns only those fields
- **D-15:** ?since= accepts Unix timestamps only -- matches PeeringDB exactly
- **D-16:** ?limit= and ?skip= query parameters for pagination
- **D-24:** Error responses match PeeringDB's exact error format per status code
- **D-25:** X-Powered-By: PeeringDB-Plus/1.1 header on all compat responses -- transparent but non-intrusive branding
- **D-26:** Match PeeringDB's HTTP response headers where applicable
- **D-27:** New `internal/pdbcompat/` package -- handler, filter parser, serializers, tests all in one package
- **D-28:** Separate CORS middleware instance for /api/ -- consistent with Phase 5 REST (same config, independently configurable)
- **D-29:** Readiness-gated like GraphQL and REST -- 503 until first sync completes
- **D-30:** Both fixture-based integration tests (using Phase 1 PeeringDB fixtures) AND golden file tests (compare against real PeeringDB API responses)

### Claude's Discretion
- **D-17:** API index at /api/ -- Claude decides whether to include and what format
- **D-18:** Meta field in envelope -- Claude decides whether to include useful pagination info or keep empty like PeeringDB
- **D-19:** _set fields per type -- Claude determines which _set fields PeeringDB returns per type and implements those
- **D-20:** Unknown filter fields -- Claude decides whether to return 400 or silently ignore
- **D-21:** Page size limits -- Claude decides max limit based on PeeringDB behavior and SQLite performance
- **D-22:** Single object response format -- Claude checks PeeringDB's actual behavior (likely {data: [obj]} array)
- **D-23:** ?q= search fields per type -- Claude matches PeeringDB's actual behavior

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| PDBCOMPAT-01 | PeeringDB URL paths (/api/net, /api/ix, /api/fac, etc.) return data in PeeringDB's response envelope format ({data:[], meta:{}}) | Type registry maps 13 PeeringDB type constants to ent query builders; `peeringdb.Response[T]` reusable as output envelope |
| PDBCOMPAT-02 | Django-style query filters (__contains, __startswith, __in, __lt, __gt, __lte, __gte) work on string and numeric fields | Generic filter parser using `sql.Field*` predicates; all predicate types share `func(*sql.Selector)` signature enabling type-safe casting |
| PDBCOMPAT-03 | Depth parameter (?depth=0\|2) controls relationship expansion in responses | Ent eager-loading via `With*` methods for depth=2; conditional serialization of `_set` fields; depth only affects single-object retrieves per PeeringDB behavior |
| PDBCOMPAT-04 | Since parameter (?since=) returns only objects updated after the given timestamp | Filter on `updated` field using `sql.FieldGTE` with Unix timestamp converted to `time.Time` |
| PDBCOMPAT-05 | Pagination via limit/skip query parameters matches PeeringDB behavior | Ent `.Limit()` and `.Offset()` methods; default limit 250 matching PeeringDB |
</phase_requirements>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| net/http (stdlib) | Go 1.26 | HTTP handlers and routing | Go 1.22+ ServeMux supports method routing and path params. Sufficient for 13 GET endpoints. No external router needed. |
| encoding/json (stdlib) | Go 1.26 | JSON serialization of responses | Custom MarshalJSON on response structs to control field emission (no omitempty for PeeringDB compat). |
| entgo.io/ent | v0.14.5 | Database queries via generated client | Already in go.mod. Query builder pattern with `Where()`, `Limit()`, `Offset()`, `Order()`, `With*()`. |
| entgo.io/ent/dialect/sql | v0.14.5 | Generic field predicates | `sql.FieldEQ`, `sql.FieldContains`, `sql.FieldGT`, etc. enable dynamic predicate construction by field name string. |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| github.com/rs/cors | existing | CORS middleware | Already in go.mod. Create separate CORS instance for /api/ per D-28. |
| go.opentelemetry.io/otel | existing | Request tracing | Wrap compat handlers with otelhttp for incoming request spans. |
| internal/peeringdb/types.go | existing | Response serialization structs | Reuse existing PeeringDB types as JSON output format. All 13 types with correct JSON tags already exist. |
| testdata/fixtures/*.json | existing | Test data | 13 fixture files with PeeringDB-format JSON for all object types. |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Custom response structs | Ent-generated structs directly | Ent structs use `omitempty` and different naming; PeeringDB requires all fields present with exact snake_case |
| Generic sql.Field* predicates | Type-specific predicate functions (e.g., `network.NameContains`) | Type-specific functions are type-safe but require separate code per type; generic predicates enable the D-11 reusable parser |
| Stdlib ServeMux | chi v5 | Chi adds route grouping and middleware chaining but stdlib is sufficient for 13 simple GET routes |

## Architecture Patterns

### Recommended Project Structure
```
internal/pdbcompat/
  handler.go           # HTTP handler (list, detail, index)
  handler_test.go      # Integration tests with enttest
  registry.go          # Type registry: PeeringDB type name -> ent query builder + field metadata
  filter.go            # Generic Django-style filter parser -> sql predicates
  filter_test.go       # Unit tests for filter parsing
  serializer.go        # ent entity -> PeeringDB JSON struct mappers (13 types)
  serializer_test.go   # Serialization correctness tests
  depth.go             # Depth=2 eager-loading and _set field assembly
  depth_test.go        # Depth behavior tests
  response.go          # Response envelope, error formatting, headers
  testdata/
    golden/            # Golden file test data (captured from real PeeringDB API)
```

### Pattern 1: Type Registry
**What:** A central registry that maps PeeringDB type name strings (e.g., "net", "org") to query execution functions, field metadata (name, type, filterable), and serialization functions. This avoids a 13-way switch statement in the handler and makes adding new types trivial.

**When to use:** All handler dispatch, filter validation, and serialization.

**Example:**
```go
// Source: project-specific pattern based on ent architecture
type TypeConfig struct {
    // PeeringDB API type name (e.g., "net", "org")
    Name string
    // Valid field names and their types for filter validation
    Fields map[string]FieldType
    // Execute a list query with applied filters, returning serialized results
    List func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, error)
    // Execute a single-object query by ID
    Get func(ctx context.Context, client *ent.Client, id int, depth int) (any, error)
    // Fields to search for ?q= parameter
    SearchFields []string
}

var Registry = map[string]TypeConfig{
    peeringdb.TypeNet: {
        Name: "net",
        Fields: networkFields,
        List: listNetworks,
        Get:  getNetwork,
        SearchFields: []string{"name", "aka", "name_long", "asn", "irr_as_set"},
    },
    // ... 12 more types
}
```

### Pattern 2: Generic Filter Parser (sql.Field* Predicates)
**What:** Parse Django-style query parameters (`?name__contains=X`, `?asn__gt=100`) into ent `func(*sql.Selector)` predicates using the generic `sql.Field*` functions. All 13 predicate types share the underlying type `func(*sql.Selector)`, so a single parser works for all entity types.

**When to use:** Every filtered query across all 13 types.

**Example:**
```go
// Source: ent/dialect/sql API + project-specific filter parsing
func ParseFilters(params url.Values, fields map[string]FieldType) ([]func(*sql.Selector), error) {
    var predicates []func(*sql.Selector)
    for key, values := range params {
        // Skip non-filter params
        if isReservedParam(key) {
            continue
        }
        fieldName, op := parseFieldOp(key) // "name__contains" -> "name", "contains"
        ft, ok := fields[fieldName]
        if !ok {
            continue // D-20: silently ignore unknown fields (recommended)
        }
        value := values[0]
        p, err := buildPredicate(fieldName, op, value, ft)
        if err != nil {
            return nil, fmt.Errorf("filter %s: %w", key, err)
        }
        predicates = append(predicates, p)
    }
    return predicates, nil
}

func buildPredicate(field, op, value string, ft FieldType) (func(*sql.Selector), error) {
    switch op {
    case "":       // exact match
        return sql.FieldEQ(field, convertValue(value, ft)), nil
    case "contains":
        return sql.FieldContainsFold(field, value), nil // case-insensitive per D-10
    case "startswith":
        return sql.FieldHasPrefix(field, value), nil
    case "in":
        vals := strings.Split(value, ",")
        return sql.FieldIn(field, toAnySlice(vals, ft)...), nil
    case "lt":
        return sql.FieldLT(field, convertValue(value, ft)), nil
    case "gt":
        return sql.FieldGT(field, convertValue(value, ft)), nil
    case "lte":
        return sql.FieldLTE(field, convertValue(value, ft)), nil
    case "gte":
        return sql.FieldGTE(field, convertValue(value, ft)), nil
    default:
        return nil, fmt.Errorf("unsupported operator: %s", op)
    }
}
```

### Pattern 3: Response Envelope Adapter
**What:** Reuse `peeringdb.Response[T]` for output serialization. Map ent entities to the existing PeeringDB Go structs (from `internal/peeringdb/types.go`) which already have correct JSON tags without `omitempty`.

**When to use:** Every API response.

**Example:**
```go
// Source: internal/peeringdb/types.go (existing)
func writeResponse(w http.ResponseWriter, data any) {
    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("X-Powered-By", "PeeringDB-Plus/1.1") // D-25
    json.NewEncoder(w).Encode(map[string]any{
        "meta": map[string]any{},
        "data": data,
    })
}

// Map ent Network to peeringdb.Network
func networkFromEnt(n *ent.Network) peeringdb.Network {
    return peeringdb.Network{
        ID:        n.ID,
        OrgID:     derefInt(n.OrgID),
        Name:      n.Name,
        Aka:       n.Aka,
        // ... all fields mapped
        Created:   n.Created,
        Updated:   n.Updated,
        Status:    n.Status,
    }
}
```

### Pattern 4: Depth-Aware Serialization
**What:** At depth=0, serialize flat objects with FK IDs only. At depth=2, use ent eager-loading (`With*` methods) to fetch related objects, then include them as nested `_set` arrays in the response. Depth only applies to single-object endpoints (`/api/net/42?depth=2`), not list endpoints per PeeringDB behavior.

**When to use:** Single-object GET endpoints with depth parameter.

**Example:**
```go
// For org at depth=2, include net_set, fac_set, ix_set, carrier_set, campus_set
func getOrgWithDepth(ctx context.Context, client *ent.Client, id, depth int) (map[string]any, error) {
    q := client.Organization.Query().Where(organization.ID(id))
    if depth >= 2 {
        q = q.WithNetworks().WithFacilities().WithInternetExchanges().WithCarriers().WithCampuses()
    }
    org, err := q.Only(ctx)
    if err != nil { return nil, err }

    result := orgToMap(org) // flat fields
    if depth >= 2 {
        result["net_set"] = mapNetworks(org.Edges.Networks)
        result["fac_set"] = mapFacilities(org.Edges.Facilities)
        // etc.
    }
    return result, nil
}
```

### Anti-Patterns to Avoid
- **Wrapping entrest output:** Do NOT transform entrest-generated responses into PeeringDB format. The two APIs have fundamentally different response envelopes, pagination models, and query parameter syntax. Build as separate handler trees querying the same ent client.
- **Using ent-generated structs for JSON output:** Ent structs have `omitempty` tags and an `edges` field that PeeringDB responses do not have. Use custom serialization that matches PeeringDB's exact format.
- **Type-specific filter code:** Do NOT write separate filter parsing for each of the 13 types. Use the generic `sql.Field*` functions with a field metadata registry.
- **Implementing depth on list endpoints:** PeeringDB itself does not honor the depth parameter on list endpoints (confirmed via GitHub issue #1658). Only implement depth for single-object retrieves.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| SQL query construction | Raw SQL strings with string interpolation | ent query builder + `sql.Field*` predicates | SQL injection prevention, type safety, proper escaping |
| CORS handling | Custom CORS headers | `rs/cors` middleware (existing) | CORS has many edge cases (preflight, credentials, allowed headers) |
| HTTP routing with method matching | Custom router/dispatcher | Go 1.22+ `ServeMux` with `"GET /api/net"` patterns | Stdlib handles trailing slash normalization, method matching, path params |
| JSON response envelope | Manual string building | `json.NewEncoder` with structured types | Proper escaping of all values, consistent format |
| Case-insensitive comparison | Custom string lowering | SQLite `COLLATE NOCASE` via `sql.FieldContainsFold` | Database-level optimization, correct Unicode behavior |

**Key insight:** The ent ORM already provides all the query primitives needed. The compat layer is a thin adapter between PeeringDB's query parameter syntax and ent's predicate system, plus a serialization layer between ent's Go structs and PeeringDB's JSON format.

## Common Pitfalls

### Pitfall 1: Ent Struct omitempty Drops Zero/Empty Values
**What goes wrong:** Ent-generated Go structs have `json:"field,omitempty"` tags. PeeringDB always includes all fields in responses, including empty strings (`""`) and zero values (`0`). Using ent structs directly produces JSON missing fields that PeeringDB consumers expect.
**Why it happens:** Ent generates `omitempty` for optional/default fields to reduce JSON size. PeeringDB uses Django's default serialization which always includes all model fields.
**How to avoid:** Map ent entities to `internal/peeringdb/types.go` structs which do NOT have `omitempty` tags. These structs were designed for PeeringDB deserialization but work equally well for serialization.
**Warning signs:** JSON responses missing fields that are present in PeeringDB responses. Client libraries failing to parse responses because expected fields are absent.

### Pitfall 2: Case-Insensitive Contains vs Exact Match
**What goes wrong:** PeeringDB's `__contains` filter is case-insensitive (`?name__contains=equinix` matches "Equinix International"). Exact match (`?name=Equinix International`) is also case-insensitive on PeeringDB. SQLite's default behavior for `LIKE` is case-insensitive for ASCII but not for Unicode. Ent's `FieldContains` is case-sensitive.
**Why it happens:** PeeringDB runs on PostgreSQL with `icontains`/`istartswith` Django lookups. SQLite's `COLLATE NOCASE` only handles ASCII case folding.
**How to avoid:** Use `sql.FieldContainsFold` for `__contains` (ent's case-insensitive variant). For exact match, use `sql.FieldEqualFold`. Document that Unicode case folding may differ from PeeringDB for non-ASCII characters (edge case, very rare in PeeringDB data).
**Warning signs:** Filtered queries returning different results than PeeringDB for the same filter parameters.

### Pitfall 3: Depth Only Works on Single-Object Retrieves
**What goes wrong:** Implementing depth parameter on list endpoints (`/api/net?depth=2`) when PeeringDB itself does not honor depth on list endpoints. This produces massive N+1 query problems and responses that are far larger than expected.
**Why it happens:** PeeringDB documentation implies depth works everywhere, but GitHub issue #1658 confirms it only affects single-object retrieves. The Django REST Framework `depth` parameter in the serializer applies only to detail views.
**How to avoid:** Only apply depth expansion on `/api/{type}/{id}` endpoints. List endpoints always return depth=0 flat objects regardless of the `depth` query parameter.
**Warning signs:** List endpoint responses that include `_set` fields. Slow list queries due to eager-loading all relationships.

### Pitfall 4: PeeringDB _set Fields Are Type-Specific
**What goes wrong:** Assuming all types have the same `_set` fields. Each PeeringDB type has different reverse relationships that appear as `_set` fields at depth >= 1.
**Why it happens:** PeeringDB's Django models define different reverse relationships per model. Organization has `net_set`, `fac_set`, `ix_set`, `carrier_set`, `campus_set`. Network has `poc_set`, `netfac_set`, `netixlan_set`. Each type is different.
**How to avoid:** Define _set fields per type in the type registry. Map each to the corresponding ent edge:
- **org:** `net_set` (networks), `fac_set` (facilities), `ix_set` (internet_exchanges), `carrier_set` (carriers), `campus_set` (campuses)
- **net:** `poc_set` (pocs), `netfac_set` (network_facilities), `netixlan_set` (network_ix_lans)
- **fac:** `netfac_set` (network_facilities), `ixfac_set` (ix_facilities), `carrierfac_set` (carrier_facilities)
- **ix:** `ixlan_set` (ix_lans), `ixfac_set` (ix_facilities)
- **ixlan:** `ixpfx_set` (ix_prefixes), `netixlan_set` (network_ix_lans)
- **carrier:** `carrierfac_set` (carrier_facilities)
- **campus:** `fac_set` (facilities)
- **poc, ixpfx, netixlan, netfac, ixfac, carrierfac:** No `_set` fields (leaf entities)
**Warning signs:** Missing or extra `_set` fields compared to PeeringDB responses.

### Pitfall 5: filter __in Requires Proper Type Conversion
**What goes wrong:** The `__in` filter accepts comma-separated values (`?asn__in=13335,174,3356`). If these are passed as strings to an integer field's `In` predicate, the SQL query fails or returns no results.
**Why it happens:** URL query parameters are always strings. Numeric fields need values converted to `int` before passing to `sql.FieldIn`. The `__in` operator for string fields passes string slices; for integer fields, it passes integer slices.
**How to avoid:** The field metadata registry must include field types (string, int, bool, time). The filter parser converts values based on field type before constructing predicates.
**Warning signs:** `__in` queries on numeric fields returning empty results or database errors.

### Pitfall 6: Trailing Slash vs No Trailing Slash
**What goes wrong:** Go 1.22+ `ServeMux` treats `/api/net` and `/api/net/` as different patterns. PeeringDB accepts both. Without explicit handling, one form 404s.
**Why it happens:** ServeMux pattern matching is literal. A pattern `"GET /api/net"` does not match `/api/net/`.
**How to avoid:** Register both patterns, or use a middleware that strips trailing slashes. The cleanest approach: register `"GET /api/net/{$}"` (exact, no trailing slash) and `"GET /api/net/"` (with trailing slash, acts as prefix). Or use `http.StripPrefix` with redirect.
**Warning signs:** 404 responses when clients use trailing slashes (many HTTP libraries add them by default).

### Pitfall 7: Single-Object Response is Still an Array
**What goes wrong:** PeeringDB `GET /api/net/42` returns `{"meta": {}, "data": [{...}]}` -- the data field is always an array, even for single objects. Returning `{"data": {...}}` (object instead of array) breaks every PeeringDB client library.
**Why it happens:** PeeringDB's Django REST Framework always wraps results in a list regardless of endpoint type.
**How to avoid:** Always wrap single-object results in a one-element array.
**Warning signs:** Client libraries failing with "expected array, got object" errors.

## Code Examples

Verified patterns from the codebase:

### Ent Query with Dynamic Predicates
```go
// Source: ent/network/where.go + ent/predicate/predicate.go analysis
// All predicate types are func(*sql.Selector) -- identical underlying type
q := client.Network.Query()

// Apply dynamic predicates from filter parser
for _, p := range predicates {
    q = q.Where(predicate.Network(p)) // Cast generic predicate to typed predicate
}

// Apply pagination
q = q.Limit(limit).Offset(skip)

// Apply default sort by ID (D-05)
q = q.Order(ent.Asc(network.FieldID))

results, err := q.All(ctx)
```

### Ent Eager-Loading for Depth=2
```go
// Source: ent/schema/network.go edge definitions
q := client.Network.Query().Where(network.ID(id))
q = q.WithOrganization()        // org edge -> org_id expansion
q = q.WithPocs()                // poc_set
q = q.WithNetworkFacilities()   // netfac_set
q = q.WithNetworkIxLans()       // netixlan_set
net, err := q.Only(ctx)
// Access edges: net.Edges.Organization, net.Edges.Pocs, etc.
```

### Reuse PeeringDB Types for Serialization
```go
// Source: internal/peeringdb/types.go (existing, correct JSON tags)
// peeringdb.Network has `json:"asn"` (not omitempty)
// peeringdb.Response[T] has `json:"meta"` and `json:"data"`

resp := peeringdb.Response[peeringdb.Network]{
    Meta: json.RawMessage(`{}`),
    Data: []peeringdb.Network{networkFromEnt(entNetwork)},
}
json.NewEncoder(w).Encode(resp)
```

### Handler Registration with Trailing Slash Support (D-02)
```go
// Source: cmd/peeringdb-plus/main.go patterns
// Register both with and without trailing slash per D-02
mux.HandleFunc("GET /api/net", compatHandler.List("net"))
mux.HandleFunc("GET /api/net/", compatHandler.ListOrDetail("net")) // catches both /api/net/ and /api/net/{id}
```

## Discretion Recommendations

Based on research, here are recommendations for the areas marked as Claude's Discretion:

### D-17: API Index at /api/
**Recommendation:** Include a simple JSON index listing available endpoints. PeeringDB does this.
```json
{"net": {"list_endpoint": "/api/net"}, "org": {"list_endpoint": "/api/org"}, ...}
```

### D-18: Meta Field in Envelope
**Recommendation:** Keep empty `{}` like PeeringDB for maximum compatibility. PeeringDB's meta field is documented as containing `status` and `message` but in practice is always empty for successful responses. Include `status` and `message` only in error responses.

### D-19: _set Fields Per Type
**Recommendation:** See Pitfall 4 above for the complete mapping. Derived from ent schema edge definitions which mirror PeeringDB's Django model relationships.

### D-20: Unknown Filter Fields
**Recommendation:** Silently ignore unknown fields. PeeringDB silently ignores unknown filter parameters rather than returning errors. This matches the principle of being liberal in what you accept.

### D-21: Page Size Limits
**Recommendation:** Default limit=250 (matches PeeringDB's default page size and our sync client's `pageSize`). Maximum limit=1000. No minimum. `limit=0` means "use default" (not "unlimited"). This prevents accidental full-table dumps.

### D-22: Single Object Response Format
**Recommendation:** Always wrap in array: `{"data": [{obj}]}`. Confirmed by PeeringDB API documentation and GitHub issue analysis. Single-object 404 returns `{"data": []}` (empty array) with HTTP 404.

### D-23: ?q= Search Fields Per Type
**Recommendation:** Match PeeringDB's search behavior by searching across name-like fields:
- **org:** name, aka, name_long
- **net:** name, aka, name_long, asn (as string), irr_as_set
- **fac:** name, aka, name_long, city, country
- **ix:** name, aka, name_long, city, country
- **poc:** name, email
- **ixlan:** name, descr
- **ixpfx:** prefix
- **netixlan:** name, ipaddr4, ipaddr6, asn (as string)
- **netfac:** name
- **ixfac:** name
- **carrier:** name, aka, name_long
- **carrierfac:** name
- **campus:** name, aka, name_long

The `?q=` parameter should OR across all search fields with `ContainsFold` (case-insensitive contains).

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| PeeringDB Python/Django | Go ent ORM adapter | This project | Must translate Django filter conventions to ent predicates |
| depth works on all endpoints | depth only on detail views | GitHub #1658 (2024) | List endpoints ignore depth parameter |
| PeeringDB OpenAPI spec | Hand-written compat layer | This project | PeeringDB's OpenAPI spec is broken (GitHub #1878); compat layer provides the real API contract |

## Open Questions

1. **PeeringDB error response format for specific error codes**
   - What we know: Errors use `{"meta": {"status": N, "message": "..."}, "data": []}` format
   - What's unclear: Exact error messages for each status code (400 bad request, 404 not found, etc.)
   - Recommendation: Use generic error messages matching HTTP status semantics. Capture golden file test data from real PeeringDB API for error cases.

2. **Exact depth=2 response shape per type**
   - What we know: _set fields contain arrays of related objects. The field names follow the pattern `{type}_set` matching PeeringDB API endpoint names.
   - What's unclear: Whether the nested objects in _set fields at depth=2 also include their own relationship IDs or are fully flat.
   - Recommendation: Implement nested objects as flat (depth=0 format) within _set arrays. This matches PeeringDB behavior where depth=2 expands one level but nested objects are flat.

3. **?fields= projection on _set fields**
   - What we know: `?fields=id,name` limits which fields appear in the response
   - What's unclear: Whether ?fields= also applies to objects inside _set arrays at depth > 0
   - Recommendation: Apply ?fields= only to the top-level object, not to nested _set objects. This is simplest and likely matches PeeringDB behavior.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) |
| Config file | None -- standard `go test` |
| Quick run command | `go test ./internal/pdbcompat/ -race -count=1` |
| Full suite command | `go test ./... -race -count=1` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| PDBCOMPAT-01 | URL paths return PeeringDB envelope | integration | `go test ./internal/pdbcompat/ -run TestListEndpoint -race -count=1` | Wave 0 |
| PDBCOMPAT-01 | Single object by ID returns envelope | integration | `go test ./internal/pdbcompat/ -run TestDetailEndpoint -race -count=1` | Wave 0 |
| PDBCOMPAT-02 | __contains filter on string field | unit | `go test ./internal/pdbcompat/ -run TestFilter/contains -race -count=1` | Wave 0 |
| PDBCOMPAT-02 | __in filter on numeric field | unit | `go test ./internal/pdbcompat/ -run TestFilter/in_numeric -race -count=1` | Wave 0 |
| PDBCOMPAT-02 | __lt/__gt on numeric field | unit | `go test ./internal/pdbcompat/ -run TestFilter/numeric_comparison -race -count=1` | Wave 0 |
| PDBCOMPAT-03 | depth=0 returns flat objects | integration | `go test ./internal/pdbcompat/ -run TestDepth/zero -race -count=1` | Wave 0 |
| PDBCOMPAT-03 | depth=2 returns _set fields | integration | `go test ./internal/pdbcompat/ -run TestDepth/two -race -count=1` | Wave 0 |
| PDBCOMPAT-04 | ?since= filters by updated timestamp | integration | `go test ./internal/pdbcompat/ -run TestSince -race -count=1` | Wave 0 |
| PDBCOMPAT-05 | limit/skip pagination | integration | `go test ./internal/pdbcompat/ -run TestPagination -race -count=1` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/pdbcompat/ -race -count=1`
- **Per wave merge:** `go test ./... -race -count=1`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/pdbcompat/handler_test.go` -- integration tests with enttest
- [ ] `internal/pdbcompat/filter_test.go` -- unit tests for filter parser
- [ ] `internal/pdbcompat/serializer_test.go` -- serialization correctness
- [ ] `internal/pdbcompat/depth_test.go` -- depth expansion behavior
- [ ] Framework install: None needed -- stdlib testing

## Sources

### Primary (HIGH confidence)
- Codebase: `internal/peeringdb/types.go` -- All 13 PeeringDB Go struct definitions with exact JSON tags (no omitempty)
- Codebase: `ent/predicate/predicate.go` -- All predicate types are `func(*sql.Selector)`, enabling generic filter parser
- Codebase: `ent/network/where.go` -- Generated predicate functions confirming `sql.Field*` pattern
- Codebase: `ent/network/network.go` -- Field name constants (snake_case, matching PeeringDB)
- Codebase: `ent/network.go` -- Generated struct with `omitempty` JSON tags (confirming need for custom serialization)
- Codebase: All 13 `ent/schema/*.go` files -- Edge definitions mapping to _set fields
- Codebase: `testdata/fixtures/*.json` -- 13 PeeringDB fixture files with correct envelope format
- [PeeringDB API Specs](https://docs.peeringdb.com/api_specs/) -- Response envelope, depth, filters, since, limit/skip
- [PeeringDB Search HOWTO](https://docs.peeringdb.com/howto/search/) -- Filter parameter syntax

### Secondary (MEDIUM confidence)
- [PeeringDB GitHub Issue #1658](https://github.com/peeringdb/peeringdb/issues/1658) -- Depth parameter does NOT work on list endpoints (confirmed by PeeringDB developers)
- [PeeringDB API Workshop](https://docs.peeringdb.com/presentation/20191021-API-EuroIX35-Arnold-Nipper.pdf) -- _set field naming convention, depth behavior examples
- [PeeringDB GitHub Issue #72](https://github.com/peeringdb/docs/issues/72) -- Documentation gaps in API filter syntax

### Tertiary (LOW confidence)
- PeeringDB error response exact format -- derived from documentation, not verified against live API
- ?q= search fields per type -- inferred from Django model field names, not confirmed against source

## Project Constraints (from CLAUDE.md)

- **CS-0 (MUST):** Use modern Go code guidelines
- **CS-1 (MUST):** Enforce `gofmt`, `go vet`
- **CS-5 (MUST):** Use input structs for functions receiving more than 2 arguments
- **CS-6 (SHOULD):** Declare function input structs before the function consuming them
- **ERR-1 (MUST):** Wrap errors with `%w` and context
- **T-1 (MUST):** Table-driven tests; deterministic and hermetic
- **T-2 (MUST):** Run `-race` in CI; add `t.Cleanup` for teardown
- **T-3 (SHOULD):** Mark safe tests with `t.Parallel()`
- **OBS-1 (MUST):** Structured logging (`slog`) with levels and consistent fields
- **OBS-2 (SHOULD):** Correlate logs/metrics/traces via request IDs from context
- **SEC-1 (MUST):** Validate inputs; set explicit I/O timeouts
- **API-1 (MUST):** Document exported items
- **API-2 (MUST):** Accept interfaces where variation is needed; return concrete types
- **API-3 (SHOULD):** Keep functions small, orthogonal, and composable
- **MD-1 (SHOULD):** Prefer stdlib; introduce deps only with clear payoff

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- no new dependencies; all components exist in go.mod or stdlib
- Architecture: HIGH -- pattern directly follows ent's predicate system and existing PeeringDB types
- Pitfalls: HIGH -- 7 pitfalls identified with specific prevention strategies, verified against codebase and PeeringDB API behavior
- Depth behavior: MEDIUM -- depth=2 _set field mapping derived from ent edge definitions, needs golden file validation against real PeeringDB API

**Research date:** 2026-03-22
**Valid until:** 2026-04-22 (stable domain -- PeeringDB API changes rarely)
