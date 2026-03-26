# Phase 34: Query Optimization & Architecture - Research

**Researched:** 2026-03-26
**Domain:** Performance optimization, error standardization, code architecture refactoring
**Confidence:** HIGH

## Summary

This phase addresses six requirements spanning three distinct areas: query performance (PERF-01, PERF-03, PERF-05), error consistency (ARCH-01), and code structure (ARCH-04, QUAL-04). All six areas have well-understood solutions grounded in the existing codebase patterns, and the research confirms that every change can be made with stdlib and existing dependencies only -- no new libraries are needed.

The search query optimization (PERF-01) replaces the current pattern where each of 6 entity type queries issues two SQL statements (item fetch + count) with a single fetch-limit+1 approach. The database indexes (PERF-03) add `updated` and `created` index entries to all 13 ent schemas. The field projection fix (PERF-05) replaces `json.Marshal`/`json.Unmarshal` roundtripping with a `reflect`-based field map built once at init time. The error format (ARCH-01) introduces RFC 9457 Problem Details across 5 of 6 API surfaces (excluding GraphQL per CONTEXT.md). The renderer interface (ARCH-04) replaces the type-switch in `RenderPage()` with interface dispatch. The detail handler refactor (QUAL-04) extracts query logic from 6 handler functions into separate methods.

**Primary recommendation:** Execute as 3 plans: (1) query performance (PERF-01 + PERF-03 + PERF-05), (2) error format (ARCH-01), (3) code structure (ARCH-04 + QUAL-04).

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **Search Query Optimization**: Fetch limit+1 (11 items, return 10, use 11th as `hasMore` signal). Drop separate `Count()` query entirely. Update `SearchResult` struct to use `HasMore bool` instead of `TotalCount int`. Update search results template to show "..." or similar instead of count badges.
- **Database Indexes**: Add indexes on `updated` and `created` fields across schemas that use incremental sync. Verify with `EXPLAIN QUERY PLAN` in tests.
- **Field Projection**: Pre-built `map[string]func(any) any` per entity type at init time. Field accessors compiled once at startup. No intermediate JSON serialization. Located in `internal/pdbcompat/` replacing `itemToMap()`. Field map generated from struct tags at init, not hand-coded per field.
- **Error Format**: RFC 9457 (Problem Details for HTTP APIs). Fields: `type` (URI), `title`, `status` (int), `detail`, `instance` (optional). Apply to: REST, PeeringDB compat, ConnectRPC, Web UI, Terminal. NOT GraphQL -- keep gqlgen standard error format with extensions.code. Create shared error response helper in `internal/`.
- **Renderer Interface**: Interface `RenderTerminal(w io.Writer, r *Renderer) error` in `internal/web/termrender/`. Data types implement this interface. Replaces type-switch in `RenderPage()`.
- **Detail Handler Refactor**: Extract query logic to methods on Handler: `h.queryNetwork(ctx, asn)` returns data struct. Handler methods have ent client access via receiver. Each handler function body target: under 80 lines. Query methods return the typed data struct.

### Claude's Discretion
None specified -- all decisions are locked.

### Deferred Ideas (OUT OF SCOPE)
- Do NOT change GraphQL error handling (keep gqlgen format)
- Do NOT add new API capabilities -- just optimize existing paths
- Do NOT change search UI behavior beyond removing total count (or adapting it to hasMore)
- The pre-built field map should be generated from struct tags at init, not hand-coded per field
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| PERF-01 | Search service uses single query per entity type instead of separate item + count queries | Current code in `internal/web/search.go` issues `.Limit(10).All()` then `.Count()` separately for each of 6 types (12 queries). Replace with `.Limit(11).All()` and derive `HasMore` from `len(items) > 10`. |
| PERF-03 | Database indexes on `updated` and `created` fields for incremental sync and filtered queries | All 13 schemas have `created`/`updated` fields but NONE have indexes on them. Add `index.Fields("updated")` and `index.Fields("created")` to every schema's `Indexes()` method, then run `go generate ./ent`. |
| PERF-05 | Field projection in pdbcompat avoids JSON marshal/unmarshal roundtrip per item | Current `itemToMap()` in `internal/pdbcompat/search.go:78` uses `json.Marshal` then `json.Unmarshal` for struct-to-map conversion. Replace with `reflect`-based field accessor map built at init from `json` struct tags on each peeringdb type. |
| ARCH-01 | All 6 API surfaces return errors in consistent format | Currently: pdbcompat uses `{"meta":{"error":"..."},"data":[]}`, ConnectRPC uses `connect.NewError()`, Web UI uses HTML error pages, terminal uses styled text, REST uses entrest defaults. Unify non-GraphQL surfaces to RFC 9457 format. |
| ARCH-04 | Terminal renderer dispatches via interface instead of type-switch | Current `RenderPage()` in `internal/web/termrender/renderer.go:72` has 8-case type-switch. Replace with `RenderTerminal(w io.Writer, r *Renderer) error` interface implemented by each data type. |
| QUAL-04 | Web detail handlers refactored to separate query logic from rendering (each under 80 lines) | Current `detail.go` is 1,309 lines with 6 handler functions ranging from ~80 lines (carrier) to ~170 lines (org/ix). Extract query logic into `h.queryXxx()` methods. |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **CS-0 (MUST)**: Modern Go code guidelines (Go 1.26)
- **CS-5 (MUST)**: Input structs for functions with >2 arguments
- **ERR-1 (MUST)**: Wrap errors with `%w` and context
- **T-1 (MUST)**: Table-driven tests, deterministic and hermetic
- **T-2 (MUST)**: Run `-race` in CI; add `t.Cleanup` for teardown
- **OBS-1 (MUST)**: Structured logging (`slog`) with levels and consistent fields
- **API-1 (MUST)**: Document exported items
- **PERF-1 (MUST)**: Measure before optimizing (relevant for verifying index improvements)
- **CS-3 (SHOULD)**: Small interfaces near consumers; prefer composition over inheritance
- **MD-1 (SHOULD)**: Prefer stdlib; introduce deps only with clear payoff

## Standard Stack

No new dependencies required for this phase. All work uses existing project libraries:

### Core (already in project)
| Library | Version | Purpose | Why Used Here |
|---------|---------|---------|---------------|
| `reflect` (stdlib) | Go 1.26 | Field map generation from struct tags | Build `map[string]fieldAccessor` at init from `json` struct tags on peeringdb types |
| `entgo.io/ent` | v0.14.5 | Schema indexes | Add `index.Fields("updated")`, `index.Fields("created")` to schema `Indexes()` methods |
| `encoding/json` (stdlib) | Go 1.26 | RFC 9457 error serialization | Encode problem detail responses |
| `net/http` (stdlib) | Go 1.26 | Error response writing | Shared error helper sets `Content-Type: application/problem+json` |

## Architecture Patterns

### Pattern 1: Fetch Limit+1 for HasMore Detection

**What:** Fetch N+1 items, return N, derive `HasMore` from whether the (N+1)th item exists.
**When to use:** When exact total count is unnecessary and query halving is desirable.
**Current state in codebase:**

```go
// CURRENT (search.go:148-166) -- 2 queries per type
items, err := s.client.Network.Query().Where(pred).Limit(10).All(ctx)
count, err := s.client.Network.Query().Where(pred).Count(ctx)
```

**Target pattern:**

```go
// NEW -- 1 query per type
const displayLimit = 10
items, err := s.client.Network.Query().Where(pred).Limit(displayLimit + 1).All(ctx)
hasMore := len(items) > displayLimit
if hasMore {
    items = items[:displayLimit]
}
```

**Impact:** The search handler currently issues 12 SQL queries (6 types x 2 queries each) per search request. This change halves it to 6.

### Pattern 2: Pre-built Reflect Field Map

**What:** At init time, use `reflect` to build a `map[string]int` mapping JSON field names to struct field indices per entity type. At request time, use `reflect.ValueOf(item).Field(idx)` instead of `json.Marshal`/`json.Unmarshal`.
**When to use:** When converting typed structs to filtered maps without JSON roundtripping.

**Current state (search.go:78-91):**

```go
func itemToMap(item any) (map[string]any, bool) {
    if m, ok := item.(map[string]any); ok {
        return m, true
    }
    b, err := json.Marshal(item)     // ALLOCATION + SERIALIZATION
    var m map[string]any
    json.Unmarshal(b, &m)            // ALLOCATION + DESERIALIZATION
    return m, true
}
```

**Target pattern:**

```go
// Built once at init from reflect on peeringdb.Organization, etc.
type fieldAccessor struct {
    index int  // struct field index for reflect.Value.Field()
}

// Per-type map: json tag name -> field index
var fieldMaps = map[string]map[string]fieldAccessor{}

func init() {
    registerType[peeringdb.Organization]("org")
    registerType[peeringdb.Network]("net")
    // ... all 13 types
}

func registerType[T any](typeName string) {
    var zero T
    t := reflect.TypeOf(zero)
    m := make(map[string]fieldAccessor, t.NumField())
    for i := 0; i < t.NumField(); i++ {
        f := t.Field(i)
        tag := f.Tag.Get("json")
        name, _, _ := strings.Cut(tag, ",")
        if name == "" || name == "-" {
            continue
        }
        m[name] = fieldAccessor{index: i}
    }
    fieldMaps[typeName] = m
}

// At request time: zero allocations for field lookup
func projectFields(item any, typeName string, wantFields map[string]bool) map[string]any {
    fm := fieldMaps[typeName]
    v := reflect.ValueOf(item)
    result := make(map[string]any, len(wantFields))
    for field := range wantFields {
        if acc, ok := fm[field]; ok {
            result[field] = v.Field(acc.index).Interface()
        }
    }
    return result
}
```

**Key constraint from CONTEXT.md:** "The pre-built field map should be generated from struct tags at init, not hand-coded per field."

### Pattern 3: RFC 9457 Problem Details

**What:** Standardized error response format per RFC 9457.
**Content-Type:** `application/problem+json`

```json
{
    "type": "about:blank",
    "title": "Not Found",
    "status": 404,
    "detail": "network with id 99999 not found",
    "instance": "/api/net/99999"
}
```

**Current error formats by surface:**
| Surface | Current Format | Target |
|---------|---------------|--------|
| PeeringDB compat | `{"meta":{"error":"..."},"data":[]}` | Keep PeeringDB envelope format for backward compat, add RFC 9457 support via Accept header or separate endpoint |
| ConnectRPC | `connect.NewError(code, err)` | ConnectRPC already has structured errors; wrap message in problem detail JSON |
| REST (entrest) | entrest default error format | Override error handler to produce RFC 9457 |
| Web UI (HTML) | `templates.NotFoundPage()` / `templates.ServerErrorPage()` | HTML errors stay HTML; add `code`/`message` to JSON mode |
| Terminal | `renderer.RenderError(w, status, title, detail)` | Already structured; map to problem detail fields |
| GraphQL | gqlgen extensions.code | **EXCLUDED** per CONTEXT.md |

**CRITICAL note on PeeringDB compat:** The CONTEXT.md says "Apply to: REST, PeeringDB compat, ConnectRPC, Web UI, Terminal" but the PeeringDB compat layer currently uses PeeringDB's exact error envelope format (`{"meta":{"error":"..."},"data":[]}`). Changing this would break PeeringDB API compatibility. The shared error helper should provide the RFC 9457 structure, but the pdbcompat `WriteError` function may need to retain the PeeringDB envelope format while also setting the shared structure fields. The planner should determine whether to: (a) change pdbcompat errors to RFC 9457 (breaking PeeringDB compat), or (b) keep pdbcompat envelope but map the fields internally. Given CONTEXT.md explicitly lists "PeeringDB compat" in the apply-to list, option (a) is the user's intent.

**Shared helper location:** New file `internal/httperr/problem.go` (or `internal/apierr/problem.go`). The package should be minimal -- just the struct and a `WriteProblem(w http.ResponseWriter, input WriteProblemInput)` function.

### Pattern 4: Interface-Based Renderer Dispatch

**What:** Replace type-switch with interface method call.
**Current (renderer.go:72-93):**

```go
func (r *Renderer) RenderPage(w io.Writer, title string, data any) error {
    switch d := data.(type) {
    case templates.NetworkDetail:
        return r.RenderNetworkDetail(w, d)
    case templates.IXDetail:
        return r.RenderIXDetail(w, d)
    // ... 6 more cases
    default:
        return r.renderStub(w, title)
    }
}
```

**Target:**

```go
// TerminalRenderable is implemented by data types that can render themselves
// to a terminal writer with the given renderer configuration.
type TerminalRenderable interface {
    RenderTerminal(w io.Writer, r *Renderer) error
}

func (r *Renderer) RenderPage(w io.Writer, title string, data any) error {
    if tr, ok := data.(TerminalRenderable); ok {
        return tr.RenderTerminal(w, r)
    }
    return r.renderStub(w, title)
}
```

**Types that must implement the interface (8):**
1. `templates.NetworkDetail`
2. `templates.IXDetail`
3. `templates.FacilityDetail`
4. `templates.OrgDetail`
5. `templates.CampusDetail`
6. `templates.CarrierDetail`
7. `[]templates.SearchGroup`
8. `*templates.CompareData`

**Implementation location:** Methods on each type in `internal/web/termrender/` (e.g., `network.go` already has `RenderNetworkDetail` -- add a `RenderTerminal` method on `templates.NetworkDetail` that delegates to it). However, since `templates.NetworkDetail` is defined in the `templates` package and `Renderer` is in `termrender`, the interface method must be defined on the templates types but call through to the renderer. The simpler approach per CONTEXT.md is: define the interface in `termrender`, and implement it with adapter functions or by having each `Render*Detail` function available as a method on the data type that takes the renderer.

**Circular import concern:** `templates` package cannot import `termrender` (which imports `templates`). Solution: define the interface in a third package or use a function type. Alternatively, since CONTEXT.md says "Data types implement this interface" and the `Renderer` is passed as an argument, the method signatures would be on the templates types but the actual rendering logic stays in termrender files. The templates package would import only `io` (for `io.Writer`) -- the `Renderer` type would need to be in a package importable by templates. Since `Renderer` is in `termrender` which already imports `templates`, this creates a circular dependency.

**Resolution:** Define the interface in `termrender` and have the implementation as methods in the `termrender` package that wrap the existing `Render*Detail` functions, dispatched via a registry or by keeping a thin interface check. The cleanest approach: keep `RenderTerminal` as a `func(data any, w io.Writer, r *Renderer) error` registered per type, avoiding the need for the templates types to import termrender. But CONTEXT.md says "Data types implement this interface" -- so the user wants methods ON the data types. This means the interface must be defined in a package that `templates` can import. The solution is to define the interface in `termrender` and implement it via methods in separate files that go in the `termrender` package, using receiver types from `templates`. Go allows defining methods on imported types only if the type is defined in the same package -- so this won't work directly.

**Final resolution:** Define a small interface package (e.g., `internal/web/termrender` keeps the interface) and use a wrapper type pattern:

```go
// In termrender/renderer.go
type TerminalRenderable interface {
    RenderTerminal(w io.Writer, r *Renderer) error
}

// In termrender/network.go (existing file)
func (d templates.NetworkDetail) RenderTerminal(w io.Writer, r *Renderer) error {
    return r.RenderNetworkDetail(w, d)
}
```

**But this won't compile** -- you cannot define methods on a type from another package. The real solution is one of:
1. **Wrap in termrender:** Create `type NetworkDetailView struct { templates.NetworkDetail }` in termrender, implement interface on wrapper.
2. **Func map in termrender:** Keep a `map[reflect.Type]func(any, io.Writer, *Renderer) error` registered at init.
3. **Move interface to templates:** Define `TerminalRenderable` in templates with `RenderTerminal(w io.Writer, renderer any) error` and type-assert the renderer inside each implementation.

Option 2 (func map) is the most Go-idiomatic for this cross-package scenario and avoids wrapping. But the user specifically wants "Data types implement this interface." Option 3 is the cleanest -- define the interface in templates with the renderer as an `any` parameter, and each method type-asserts to `*termrender.Renderer` inside the implementation. This avoids circular imports because `templates` defines the interface using `any` for the renderer.

**Recommended approach (option 3):**
```go
// templates/detailtypes.go
type TerminalRenderable interface {
    RenderTerminal(w io.Writer, renderer any) error
}

// termrender/network.go -- implement via standalone function that templates calls
// Actually this still requires templates importing termrender for Renderer methods.
```

After careful analysis: the cleanest approach is to **keep the type-switch but make it dispatch through a registered map** in the termrender package. This satisfies the spirit of ARCH-04 (extensible dispatch, no hardcoded switch) while avoiding circular imports:

```go
// termrender/dispatch.go
var renderers = map[reflect.Type]func(any, io.Writer, *Renderer) error{}

func Register[T any](fn func(T, io.Writer, *Renderer) error) {
    var zero T
    renderers[reflect.TypeOf(zero)] = func(v any, w io.Writer, r *Renderer) error {
        return fn(v.(T), w, r)
    }
}

func init() {
    Register(func(d templates.NetworkDetail, w io.Writer, r *Renderer) error {
        return r.RenderNetworkDetail(w, d)
    })
    // ... etc
}

func (r *Renderer) RenderPage(w io.Writer, title string, data any) error {
    if fn, ok := renderers[reflect.TypeOf(data)]; ok {
        return fn(data, w, r)
    }
    return r.renderStub(w, title)
}
```

This eliminates the type-switch, is extensible, and has zero circular import issues.

### Pattern 5: Detail Handler Extraction

**What:** Extract query logic from handler functions into typed query methods.
**Current handler sizes (lines including blank lines):**
| Handler | Lines | Query Queries | Complexity |
|---------|-------|---------------|------------|
| `handleNetworkDetail` | ~135 | 4 (network + poc count + ixlans + facilities) | HIGH |
| `handleIXDetail` | ~140 | 5 (ix + prefix count + participants + facilities + prefixes) | HIGH |
| `handleFacilityDetail` | ~125 | 4 (facility + networks + ixps + carriers) | MEDIUM |
| `handleOrgDetail` | ~170 | 8 (org + 3 counts + 5 child queries) | HIGH |
| `handleCampusDetail` | ~80 | 3 (campus + fac count + facilities) | LOW |
| `handleCarrierDetail` | ~68 | 2 (carrier + carrier facilities) | LOW |

**Target pattern:**

```go
func (h *Handler) handleNetworkDetail(w http.ResponseWriter, r *http.Request, asnStr string) {
    asn, err := strconv.Atoi(asnStr)
    if err != nil {
        h.handleNotFound(w, r)
        return
    }
    data, err := h.queryNetwork(r.Context(), asn)
    if err != nil {
        if ent.IsNotFound(err) {
            h.handleNotFound(w, r)
            return
        }
        slog.Error("query network by ASN", slog.Int("asn", asn), slog.Any("error", err))
        h.handleServerError(w, r)
        return
    }
    page := PageContent{
        Title:     data.Name,
        Content:   templates.NetworkDetailPage(data),
        Data:      data,
        Freshness: h.getFreshness(r.Context()),
    }
    if err := renderPage(r.Context(), w, r, page); err != nil {
        slog.Error("render network detail", slog.Int("asn", asn), slog.Any("error", err))
        h.handleServerError(w, r)
    }
}

func (h *Handler) queryNetwork(ctx context.Context, asn int) (templates.NetworkDetail, error) {
    // All query logic extracted here
    // Returns fully populated data struct or error
}
```

### Anti-Patterns to Avoid
- **Changing ListFunc return signature prematurely:** The `ListFunc` returns `([]any, int, error)` where `int` is total count. The pdbcompat list handler ignores the count (`_, _, err`). Don't change the ListFunc signature for the search optimization -- that's a separate concern. The search optimization is in the web search service, not pdbcompat.
- **Over-abstracting the error helper:** Keep it simple -- one struct, one write function. Don't build an error registry or middleware.
- **Reflect on hot paths:** The field map is built at init. The per-request path uses `reflect.ValueOf().Field()` which is a single pointer dereference -- much cheaper than json.Marshal/Unmarshal but still uses reflect at request time. This is acceptable per PERF-1 since it replaces two allocating JSON operations.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| RFC 9457 format | Custom error struct | Simple struct + `encoding/json` | RFC 9457 is just a JSON object with 5 fields -- no library needed |
| Struct-to-map conversion | Custom per-field mappers | `reflect` with cached field indices | Struct tags already define the mapping; reflect reads them once at init |
| Index management | Manual SQL DDL | ent schema `Indexes()` method | ent generates the CREATE INDEX DDL from schema definitions |

## Common Pitfalls

### Pitfall 1: PeeringDB Compat Error Format Breakage
**What goes wrong:** Changing pdbcompat errors to RFC 9457 format breaks PeeringDB API consumers that expect `{"meta":{"error":"..."}, "data":[]}` envelope.
**Why it happens:** CONTEXT.md says apply RFC 9457 to pdbcompat, but this conflicts with PeeringDB API compatibility.
**How to avoid:** The CONTEXT.md decision is explicit. Apply RFC 9457 to pdbcompat. Document that this is a deliberate divergence from PeeringDB's error format. The data format (`{"data":[...]}`) for successful responses remains unchanged.
**Warning signs:** Existing pdbcompat handler_test.go tests will fail when error format changes.

### Pitfall 2: Circular Import with TerminalRenderable Interface
**What goes wrong:** Defining `TerminalRenderable` in termrender and trying to implement it on templates types creates a circular import.
**Why it happens:** `termrender` imports `templates` (for the data types), so `templates` cannot import `termrender`.
**How to avoid:** Use a registered function map in termrender instead of requiring templates types to implement the interface directly. Or define the interface with `any` parameter. See Architecture Pattern 4 above.
**Warning signs:** Compilation errors about import cycles.

### Pitfall 3: Forgetting to Regenerate Ent After Index Changes
**What goes wrong:** Adding `index.Fields("updated")` to schemas without running `go generate ./ent` means the migration/creation code doesn't include the new indexes.
**Why it happens:** ent index definitions only take effect after code generation.
**How to avoid:** Run `go generate ./ent` immediately after schema changes. Verify with `EXPLAIN QUERY PLAN` in tests.
**Warning signs:** `EXPLAIN QUERY PLAN` still shows `SCAN TABLE` instead of `SEARCH TABLE ... USING INDEX`.

### Pitfall 4: Reflect Field Map Missing Nested Types
**What goes wrong:** Fields like `SocialMedia []SocialMedia` have JSON tag `json:"social_media"` but the value is a slice of structs. The reflect accessor returns the raw value, which is correct -- but `_set` fields and expanded FK objects (depth > 0) need special handling.
**Why it happens:** The current `applyFieldProjection` has special logic for `_set` suffix fields and `isExpandedObject` checks.
**How to avoid:** The reflect-based projection must preserve the same special-case logic: always include `_set` fields, always include expanded FK objects, always include `id`.
**Warning signs:** Depth=2 responses missing nested objects.

### Pitfall 5: Search Template TotalCount Removal
**What goes wrong:** Templates and terminal renderers reference `TotalCount` which is being replaced with `HasMore`.
**Why it happens:** The `SearchGroup.TotalCount` field is used in `search_results.templ` line 16 and in `termrender/search.go`.
**How to avoid:** Update all consumers: `templates.SearchGroup` struct, `search_results.templ`, `termrender/search.go`, `web.TypeResult` struct, `convertToSearchGroups()` in `handler.go`.
**Warning signs:** Compilation errors on removed field.

## Code Examples

### Example 1: Adding Index to Ent Schema

```go
// ent/schema/network.go -- add to existing Indexes() method
func (Network) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("asn"),
        index.Fields("name"),
        index.Fields("org_id"),
        index.Fields("status"),
        index.Fields("updated"),  // NEW for PERF-03
        index.Fields("created"),  // NEW for PERF-03
    }
}
```

All 13 schemas need this addition. Schemas with existing `Indexes()`:
- `network.go`, `organization.go`, `facility.go`, `internetexchange.go`, `poc.go`, `ixlan.go`, `ixprefix.go`, `networkixlan.go`, `networkfacility.go`, `ixfacility.go`, `carrier.go`, `carrierfacility.go`, `campus.go`

### Example 2: RFC 9457 Problem Detail Struct

```go
// internal/httperr/problem.go
package httperr

import (
    "encoding/json"
    "net/http"
)

// ProblemDetail represents an RFC 9457 Problem Details response.
type ProblemDetail struct {
    Type     string `json:"type"`
    Title    string `json:"title"`
    Status   int    `json:"status"`
    Detail   string `json:"detail,omitempty"`
    Instance string `json:"instance,omitempty"`
}

// WriteProblemInput holds parameters for writing a problem detail response.
type WriteProblemInput struct {
    Status   int
    Title    string
    Detail   string
    Instance string
}

// WriteProblem writes an RFC 9457 problem detail JSON response.
func WriteProblem(w http.ResponseWriter, input WriteProblemInput) {
    w.Header().Set("Content-Type", "application/problem+json")
    w.WriteHeader(input.Status)
    p := ProblemDetail{
        Type:     "about:blank",
        Title:    input.Title,
        Status:   input.Status,
        Detail:   input.Detail,
        Instance: input.Instance,
    }
    _ = json.NewEncoder(w).Encode(p)
}
```

### Example 3: EXPLAIN QUERY PLAN Test

```go
func TestUpdatedFieldIndex(t *testing.T) {
    t.Parallel()
    client := testutil.SetupClient(t)

    // Use raw SQL to check query plan.
    db := client.DB()
    rows, err := db.Query("EXPLAIN QUERY PLAN SELECT * FROM networks WHERE updated > ?",
        time.Now().Add(-time.Hour))
    if err != nil {
        t.Fatalf("explain query plan: %v", err)
    }
    defer rows.Close()

    var found bool
    for rows.Next() {
        var id, parent, notused int
        var detail string
        rows.Scan(&id, &parent, &notused, &detail)
        if strings.Contains(detail, "USING INDEX") && strings.Contains(detail, "updated") {
            found = true
        }
    }
    if !found {
        t.Error("query on updated field does not use index")
    }
}
```

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) + enttest |
| Config file | None (stdlib) |
| Quick run command | `go test -race -count=1 ./internal/pdbcompat/ ./internal/web/ ./internal/web/termrender/` |
| Full suite command | `go test -race -count=1 ./...` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| PERF-01 | Search issues 1 query per type, returns HasMore | unit | `go test -race -run TestSearch ./internal/web/ -x` | Exists (search_test.go) -- needs update |
| PERF-03 | EXPLAIN QUERY PLAN shows index usage on updated/created | integration | `go test -race -run TestUpdatedFieldIndex ./internal/pdbcompat/ -x` | Needs creation |
| PERF-05 | Field projection works without JSON roundtrip | unit | `go test -race -run TestFieldProjection ./internal/pdbcompat/ -x` | Needs creation (existing serializer_test.go can be extended) |
| ARCH-01 | Error responses have type/title/status/detail fields | unit | `go test -race -run TestProblemDetail ./internal/httperr/ -x` | Needs creation |
| ARCH-04 | RenderPage dispatches via registered renderers | unit | `go test -race -run TestRenderPage ./internal/web/termrender/ -x` | Exists (renderer_test.go) -- needs update |
| QUAL-04 | Detail handler bodies under 80 lines | unit | `go test -race -run TestDetailHandler ./internal/web/ -x` | Exists (detail_test.go) -- needs update |

### Sampling Rate
- **Per task commit:** `go test -race -count=1 ./internal/pdbcompat/ ./internal/web/ ./internal/web/termrender/ ./internal/httperr/`
- **Per wave merge:** `go test -race -count=1 ./...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/httperr/problem_test.go` -- covers ARCH-01
- [ ] `internal/pdbcompat/projection_test.go` -- covers PERF-05 (or extend serializer_test.go)
- [ ] `internal/pdbcompat/index_test.go` -- covers PERF-03

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| RFC 7807 Problem Details | RFC 9457 Problem Details | June 2023 | RFC 9457 supersedes 7807. Same format, clarified semantics for `type` field. Use `about:blank` when no specific type URI exists. |
| Count + Limit pagination | Cursor-based or limit+1 | Industry standard | Count queries on large tables are expensive in SQLite (full index scan). Limit+1 avoids this entirely. |

## Open Questions

1. **PeeringDB compat error format migration**
   - What we know: CONTEXT.md explicitly says to apply RFC 9457 to pdbcompat. Current pdbcompat uses PeeringDB's exact error envelope.
   - What's unclear: Whether this is intentional API breakage or an oversight in CONTEXT.md.
   - Recommendation: Follow CONTEXT.md -- apply RFC 9457 to pdbcompat. The project is a "plus" mirror, not a strict drop-in. Update handler_test.go accordingly.

2. **REST (entrest) error customization**
   - What we know: entrest generates its own error responses. Customizing requires either an error handler override or wrapping the handler.
   - What's unclear: Whether entrest supports custom error formatters.
   - Recommendation: Wrap the entrest handler with middleware that intercepts error responses (status >= 400) and rewrites the body to RFC 9457 format. Alternatively, check entrest docs for error handler configuration.

3. **ConnectRPC error mapping**
   - What we know: ConnectRPC uses `connect.NewError(code, err)` which produces gRPC-standard error responses. The wire format for Connect protocol is already structured JSON.
   - What's unclear: Whether replacing ConnectRPC's error format with RFC 9457 would break gRPC clients.
   - Recommendation: For ConnectRPC, add error details using `connect.NewError` with a custom detail containing the problem detail fields. Don't replace the wire format -- augment it. This way gRPC clients still get standard error codes while Connect (HTTP) clients get additional detail.

## Sources

### Primary (HIGH confidence)
- Codebase analysis: `internal/web/search.go`, `internal/pdbcompat/search.go`, `internal/pdbcompat/registry_funcs.go`, `internal/web/detail.go`, `internal/web/termrender/renderer.go`, all 13 `ent/schema/*.go` files
- [RFC 9457: Problem Details for HTTP APIs](https://www.rfc-editor.org/rfc/rfc9457.html) - Standard format specification

### Secondary (MEDIUM confidence)
- Go `reflect` package documentation for struct field access patterns
- ent `index.Fields()` documentation for schema index definitions

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - no new dependencies, all patterns are well-established Go stdlib
- Architecture: HIGH - all patterns verified against existing codebase structure
- Pitfalls: HIGH - identified through direct code analysis, especially the circular import issue and pdbcompat format concern

**Research date:** 2026-03-26
**Valid until:** 2026-04-26 (stable domain, no fast-moving dependencies)
