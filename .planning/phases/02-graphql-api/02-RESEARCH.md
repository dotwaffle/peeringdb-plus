# Phase 2: GraphQL API - Research

**Researched:** 2026-03-22
**Domain:** GraphQL API with entgql + gqlgen on top of existing entgo schemas
**Confidence:** HIGH

## Summary

Phase 2 builds the GraphQL API surface on top of the 13 entgo schemas and sync infrastructure from Phase 1. The existing codebase already has substantial entgql-generated code (gql_pagination.go, gql_collection.go, gql_where_input.go, gql_node.go, gql_edge.go) from the `entgql.WithSchemaGenerator()` and `entgql.WithWhereInputs(true)` configuration in entc.go. However, several schema annotation changes are needed before the generated code will produce a correct GraphQL schema for a read-only API with Relay-compliant pagination.

The critical gap is that the current ent schemas use `entgql.QueryField()` with `entgql.Mutations(...)` annotations but are missing `entgql.RelayConnection()`. This means the generated GraphQL schema will expose list queries returning `[Type!]!` instead of Relay connection types (`TypeConnection`). Additionally, the mutation annotations generate mutation types that should not exist in a read-only mirror. The schemas need to be updated, code regenerated, and then the gqlgen resolver layer built on top.

The gqlgen + entgql stack is well-documented and the integration is mature. The main implementation work is: (1) fix schema annotations, (2) regenerate, (3) create gqlgen.yml and resolver scaffold, (4) implement custom resolvers for `networkByAsn`, `syncStatus`, and offset/limit pagination, (5) wire up the HTTP handler with GraphiQL playground, CORS, and complexity/depth limits.

**Primary recommendation:** Fix entgo schema annotations (add RelayConnection, remove Mutations), regenerate all ent code, scaffold gqlgen resolvers, then implement the GraphQL HTTP handler with playground, CORS, and request logging.

<user_constraints>

## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Support both Relay cursor-based pagination AND offset/limit pagination
- **D-02:** GraphQL field names use camelCase (e.g., `infoPrefixes4`, `irrAsSet`) -- idiomatic GraphQL, entgql default
- **D-03:** Use entgql's generated WhereInput types for filtering -- automatic, type-safe, supports all field operators
- **D-04:** Query complexity and depth limits enabled to prevent abuse
- **D-05:** Dedicated `networkByAsn(asn: Int!)` top-level query AND filter support via `networks(where: {asn: 42})`
- **D-06:** Use Relay-style opaque global IDs (base64 type:id) for Node interface compliance
- **D-07:** No GraphQL subscriptions -- read-only mirror with hourly sync, subscriptions add complexity with little value
- **D-08:** Query-only schema -- no mutations (writes happen via sync only)
- **D-09:** All types implement Relay-compliant Node interface -- enables `node(id:)` query and Relay client compatibility
- **D-10:** No cross-type search query in v1 -- per-type queries only (FTS5 search is v2)
- **D-11:** Expose `syncStatus` query returning lastSyncAt, duration, objectCounts from the sync_status table
- **D-12:** totalCount field in connections is optional -- only returned when explicitly requested
- **D-13:** Use DataLoader pattern for relationship traversal batching (not entgql eager loading)
- **D-14:** Maximum page size of 1000 items per request
- **D-15:** Export GraphQL schema as `.graphql` SDL file in the repo for consumers
- **D-16:** Detailed query errors with field paths, validation details, and helpful messages
- **D-17:** GraphiQL as the embedded playground IDE
- **D-18:** Playground served at same path as API (`/graphql`) -- GET serves playground, POST handles queries
- **D-19:** Ship with pre-built example queries: ASN lookup, IX network listing, facility details, relationship traversal
- **D-20:** GraphQL introspection always enabled -- public data, no reason to hide schema
- **D-21:** Playground always enabled -- no disable option, it's part of the public DX
- **D-22:** 99designs/gqlgen as the GraphQL library -- entgql integrates natively
- **D-23:** stdlib net/http for HTTP server -- Go 1.22+ routing, no external router dependency
- **D-24:** Port configurable via PDBPLUS_PORT env var, default 8080
- **D-25:** Graceful shutdown on SIGTERM with configurable drain timeout -- important for Fly.io rolling deploys
- **D-26:** CORS origins configurable via PDBPLUS_CORS_ORIGINS env var, default to `*`
- **D-27:** Structured request logging middleware via slog (method, path, status, duration) -- feeds into OTel in Phase 3
- **D-28:** Root endpoint (GET /) returns JSON with version, links to /graphql, sync status -- helpful for API discovery

### Claude's Discretion
- GraphQL complexity/depth limit values
- Exact DataLoader implementation approach
- GraphiQL configuration and example query content
- Graceful shutdown drain timeout default
- Root endpoint JSON shape

### Deferred Ideas (OUT OF SCOPE)
- None -- discussion stayed within phase scope

</user_constraints>

<phase_requirements>

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| API-01 | GraphQL API exposing all PeeringDB objects via entgql | entgql.RelayConnection() + QueryField() annotations on all 13 schemas; gqlgen resolvers delegate to ent Paginate() |
| API-02 | Relationship traversal in single GraphQL query | entgql's CollectFields auto-eager-loading + DataLoader for batched relationship resolution |
| API-03 | Filter by any field (equality matching) | entgql.WithWhereInputs(true) already generates WhereInput types with equality, comparison, contains, prefix/suffix, in/not-in, nil checks for every field |
| API-04 | Lookup by ASN | Custom `networkByAsn(asn: Int!)` resolver + WhereInput filter `networks(where: {asn: 42})` |
| API-05 | Lookup by ID | Relay Node interface via `node(id: ID!)` query + per-type where filter `(where: {id: 42})` |
| API-06 | Pagination (limit/skip) | entgql generates Relay cursor pagination; offset/limit requires custom GraphQL fields alongside cursor pagination |
| API-07 | Interactive GraphQL playground | gqlgen's built-in `playground.Handler()` serving GraphiQL at /graphql (GET) |
| OPS-06 | CORS headers for browser integrations | rs/cors middleware wrapping the HTTP handler; configurable origins via PDBPLUS_CORS_ORIGINS env var |

</phase_requirements>

## Standard Stack

### Core (Already in go.mod)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| entgo.io/ent | v0.14.5 | ORM + code generation | Already in go.mod; generates GraphQL types, pagination, filtering |
| entgo.io/contrib (entgql) | v0.7.0 | GraphQL extension for ent | Already in go.mod; generates gql_*.go files, WhereInput, pagination |
| github.com/99designs/gqlgen | v0.17.68 | GraphQL server library | Already in go.mod; required by entgql; schema-first with codegen |
| github.com/vektah/gqlparser/v2 | v2.5.23 | GraphQL parser | Already in go.mod; transitive dep of gqlgen |

### New Dependencies

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| github.com/rs/cors | latest | CORS middleware for net/http | D-26: configurable CORS origins; standard Go CORS library, implements W3C spec, works with any http.Handler |
| github.com/vikstrous/dataloadgen | v0.0.10 | Generic DataLoader implementation | D-13: batch relationship loading; Go generics, no codegen, OTel tracing support via WithTracer option |
| github.com/oyyblin/gqlgen-depth-limit-extension | v0.1.0 | Query depth limiting for gqlgen | D-04: prevents deeply nested queries; separate from complexity limit |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| rs/cors | Manual CORS headers | rs/cors handles preflight, varies, credentials correctly; manual implementation is error-prone |
| dataloadgen | graph-gophers/dataloader | dataloadgen uses generics (no codegen), is faster in benchmarks, and recommended by gqlgen docs |
| dataloadgen | entgql eager loading (CollectFields) | CollectFields works but D-13 explicitly chose DataLoader pattern; DataLoader provides better control over batching |
| depth-limit-extension | Manual depth counting | Extension integrates cleanly with gqlgen plugin system; no need to hand-roll |

### Installation

```bash
go get github.com/rs/cors
go get github.com/vikstrous/dataloadgen
go get github.com/oyyblin/gqlgen-depth-limit-extension
```

## Architecture Patterns

### Recommended Project Structure

```
graph/                          # NEW: gqlgen resolver layer
├── schema.graphqls             # Generated by entgql (ent types, connections, inputs)
├── custom.graphql              # Hand-written: syncStatus, networkByAsn, offset/limit queries
├── generated.go                # gqlgen generated executable schema
├── resolver.go                 # Root resolver struct (holds ent client, sql.DB)
├── ent.resolvers.go            # Generated resolver stubs for ent types
├── custom.resolvers.go         # Hand-written resolvers for custom queries
├── model/                      # Custom GraphQL model types (SyncStatus, etc.)
│   └── models.go
└── dataloader/                 # DataLoader middleware and batch functions
    └── loader.go
internal/
├── config/config.go            # MODIFY: add Port, CORSOrigins, DrainTimeout
├── database/database.go        # Unchanged
├── graphql/                    # NEW: GraphQL HTTP handler wiring
│   └── handler.go              # Creates gqlgen handler with all middleware
├── middleware/                  # NEW: HTTP middleware
│   ├── cors.go                 # CORS middleware using rs/cors
│   ├── logging.go              # Request logging middleware (slog)
│   └── recovery.go             # Panic recovery middleware
└── sync/                       # Unchanged
ent/
├── entc.go                     # MODIFY: add WithRelaySpec, WithConfigPath
├── schema/*.go                 # MODIFY: add RelayConnection, remove Mutations
├── gql_*.go                    # REGENERATED: updated after schema changes
└── ...
cmd/
└── peeringdb-plus/main.go      # MODIFY: add GraphQL handler, CORS, playground routes
```

### Pattern 1: Schema Annotation Fix (Critical First Step)

**What:** All 13 entgo schemas currently have `entgql.Mutations(entgql.MutationCreate(), entgql.MutationUpdate())` which generates mutation types. Since this is a read-only API (D-08), mutations must be removed and `entgql.RelayConnection()` must be added for pagination.

**When to use:** Before any other GraphQL work -- this changes the generated code.

**Current annotations (all 13 schemas):**
```go
func (Network) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entgql.QueryField(),
        entgql.Mutations(entgql.MutationCreate(), entgql.MutationUpdate()),
    }
}
```

**Required annotations:**
```go
func (Network) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entgql.RelayConnection(),
        entgql.QueryField(),
    }
}
```

This changes the generated GraphQL schema from `networks: [Network!]!` to `networks(...): NetworkConnection!` with proper Relay pagination types.

### Pattern 2: entc.go Extension Configuration

**What:** The entc.go needs additional options for Relay spec compliance and gqlgen config path.

**Current:**
```go
gqlExt, err := entgql.NewExtension(
    entgql.WithSchemaGenerator(),
    entgql.WithSchemaPath("graph/schema.graphqls"),
    entgql.WithWhereInputs(true),
)
```

**Required:**
```go
gqlExt, err := entgql.NewExtension(
    entgql.WithSchemaGenerator(),
    entgql.WithSchemaPath("graph/schema.graphqls"),
    entgql.WithWhereInputs(true),
    entgql.WithConfigPath("graph/gqlgen.yml"),
    entgql.WithRelaySpec(true),
)
```

`WithRelaySpec(true)` generates the Relay Node interface in the schema. `WithConfigPath` tells entgql where gqlgen.yml lives so it can configure autobind correctly.

### Pattern 3: gqlgen.yml Configuration

**What:** The gqlgen configuration file that drives code generation.

```yaml
# graph/gqlgen.yml
schema:
  - schema.graphqls    # Generated by entgql (ent types)
  - custom.graphql     # Hand-written custom queries

exec:
  filename: generated.go
  package: graph

resolver:
  layout: follow-schema
  dir: .
  package: graph

autobind:
  - github.com/dotwaffle/peeringdb-plus/ent
  - github.com/dotwaffle/peeringdb-plus/ent/schema
  - github.com/dotwaffle/peeringdb-plus/graph/model

models:
  ID:
    model:
      - github.com/99designs/gqlgen/graphql.IntID
  Node:
    model:
      - github.com/dotwaffle/peeringdb-plus/ent.Noder
```

**Note on ID type:** The project uses PeeringDB integer IDs (D-37 from Phase 1). The gqlgen model maps `ID` to `graphql.IntID`. For D-06 (Relay opaque global IDs), a custom ID marshaler wrapping base64(type:id) will need to be implemented -- but this requires careful design since PeeringDB IDs are not globally unique across types.

### Pattern 4: Resolver Wiring

**What:** The root resolver holds dependencies; query resolvers delegate to ent.

```go
// graph/resolver.go
package graph

import (
    "database/sql"
    "github.com/dotwaffle/peeringdb-plus/ent"
)

// Resolver is the root resolver providing dependencies to all resolvers.
type Resolver struct {
    client *ent.Client
    db     *sql.DB // for sync_status queries
}

// NewResolver creates a resolver with dependencies.
func NewResolver(client *ent.Client, db *sql.DB) *Resolver {
    return &Resolver{client: client, db: db}
}
```

**Generated ent resolvers (ent.resolvers.go) pattern:**
```go
func (r *queryResolver) Networks(
    ctx context.Context,
    after *ent.Cursor, first *int,
    before *ent.Cursor, last *int,
    orderBy *ent.NetworkOrder,
    where *ent.NetworkWhereInput,
) (*ent.NetworkConnection, error) {
    return r.client.Network.Query().
        Paginate(ctx, after, first, before, last,
            ent.WithNetworkOrder(orderBy),
            ent.WithNetworkFilter(where.Filter),
        )
}
```

### Pattern 5: Custom Queries (syncStatus, networkByAsn)

**What:** Hand-written GraphQL schema and resolvers for non-generated queries.

```graphql
# graph/custom.graphql
type SyncStatus {
  lastSyncAt: Time
  durationMs: Int
  objectCounts: Map
  status: String!
  errorMessage: String
}

scalar Map
scalar Time

extend type Query {
  syncStatus: SyncStatus
  networkByAsn(asn: Int!): Network
}
```

```go
// graph/custom.resolvers.go
func (r *queryResolver) SyncStatus(ctx context.Context) (*model.SyncStatus, error) {
    status, err := sync.GetLastSyncStatus(ctx, r.db)
    if err != nil {
        return nil, err
    }
    // Convert to GraphQL model type
    return convertSyncStatus(status), nil
}

func (r *queryResolver) NetworkByAsn(ctx context.Context, asn int) (*ent.Network, error) {
    return r.client.Network.Query().
        Where(network.ASN(asn)).
        Only(ctx)
}
```

### Pattern 6: DataLoader for Relationship Batching

**What:** DataLoader middleware batches N+1 relationship queries per D-13.

```go
// graph/dataloader/loader.go
package dataloader

import (
    "context"
    "github.com/vikstrous/dataloadgen"
    "github.com/dotwaffle/peeringdb-plus/ent"
)

type ctxKey string
const loadersKey ctxKey = "dataloaders"

// Loaders holds all DataLoader instances for a request.
type Loaders struct {
    OrganizationByID *dataloadgen.Loader[int, *ent.Organization]
    NetworkByID      *dataloadgen.Loader[int, *ent.Network]
    FacilityByID     *dataloadgen.Loader[int, *ent.Facility]
    // ... one per type with relationships
}

// NewLoaders creates fresh DataLoader instances for a request.
func NewLoaders(client *ent.Client) *Loaders {
    return &Loaders{
        OrganizationByID: dataloadgen.NewMappedLoader(
            func(ctx context.Context, ids []int) (map[int]*ent.Organization, error) {
                orgs, err := client.Organization.Query().
                    Where(organization.IDIn(ids...)).
                    All(ctx)
                if err != nil {
                    return nil, err
                }
                m := make(map[int]*ent.Organization, len(orgs))
                for _, o := range orgs {
                    m[o.ID] = o
                }
                return m, nil
            },
            dataloadgen.WithWait(2*time.Millisecond),
        ),
        // ... similar for other types
    }
}

// Middleware injects DataLoaders into the request context.
func Middleware(client *ent.Client, next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        loaders := NewLoaders(client)
        ctx := context.WithValue(r.Context(), loadersKey, loaders)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// For retrieves loaders from context.
func For(ctx context.Context) *Loaders {
    return ctx.Value(loadersKey).(*Loaders)
}
```

### Pattern 7: GraphQL HTTP Handler with Playground

**What:** Wire gqlgen handler with GraphiQL playground at /graphql per D-18.

```go
// The handler serves GraphiQL on GET and handles GraphQL on POST
import "github.com/99designs/gqlgen/graphql/playground"
import "github.com/99designs/gqlgen/graphql/handler"
import "github.com/99designs/gqlgen/graphql/handler/extension"

srv := handler.NewDefaultServer(graph.NewExecutableSchema(graph.Config{
    Resolvers: graph.NewResolver(entClient, db),
}))

// Complexity and depth limits per D-04
srv.Use(extension.FixedComplexityLimit(500))
srv.AddPlugin(depth.FixedDepthLimit(15))

// Introspection is always enabled per D-20 (default behavior)

// GraphiQL playground at same path per D-18
playgroundHandler := playground.Handler("PeeringDB Plus", "/graphql")

mux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodGet {
        playgroundHandler.ServeHTTP(w, r)
        return
    }
    srv.ServeHTTP(w, r)
})
```

### Pattern 8: Relay Global IDs (D-06)

**What:** PeeringDB IDs are NOT globally unique across types (network ID 1 != facility ID 1). For Relay Node interface compliance, IDs must be encoded as opaque global identifiers.

**Approach:** Use a custom gqlgen ID marshaler that encodes `base64("TypeName:IntID")` and decodes back. The `node(id:)` resolver decodes the global ID, extracts the type, and calls `client.Noder()` with `WithFixedNodeType(typeName)`.

```go
// Custom ID marshaling for Relay global IDs
func MarshalGlobalID(typeName string, id int) string {
    return base64.StdEncoding.EncodeToString(
        []byte(fmt.Sprintf("%s:%d", typeName, id)),
    )
}

func UnmarshalGlobalID(globalID string) (typeName string, id int, err error) {
    decoded, err := base64.StdEncoding.DecodeString(globalID)
    if err != nil {
        return "", 0, fmt.Errorf("invalid global ID: %w", err)
    }
    parts := strings.SplitN(string(decoded), ":", 2)
    if len(parts) != 2 {
        return "", 0, fmt.Errorf("invalid global ID format")
    }
    id, err = strconv.Atoi(parts[1])
    return parts[0], id, err
}
```

**Important consideration:** This changes the `id` field in the GraphQL schema from returning the raw integer to returning the opaque string. Clients that want the raw PeeringDB integer ID should use a separate field (e.g., `peeringdbId: Int!`). This needs to be designed carefully to avoid confusion.

### Pattern 9: Offset/Limit Pagination (D-01)

**What:** entgql only generates Relay cursor pagination. Offset/limit must be added via custom GraphQL fields.

**Approach:** Add `offset` and `limit` arguments to connection queries in the custom schema. The resolver translates these to ent Query `.Offset()` and `.Limit()` calls, returning results within the connection type structure.

```graphql
# In custom.graphql, extend the generated query
# Alternatively, the resolver can accept offset/limit and convert to first/after internally
```

The simplest approach: treat `offset`+`limit` as syntactic sugar that converts to cursor-based pagination internally. With `first` = limit and skipping the first N results via `.Offset()`, the connection type still works.

### Anti-Patterns to Avoid

- **Generating mutations for a read-only API:** The current schema annotations include mutation generation. Remove all `entgql.Mutations()` calls before generating code. These waste generation time and expose a confusing API surface.
- **Using eager loading everywhere instead of DataLoader:** entgql's `CollectFields` adds eager-loading which works for simple cases but leads to over-fetching for complex relationship traversals. DataLoader gives better control.
- **Hand-rolling the GraphQL schema:** The entgql WithSchemaGenerator produces the schema from ent schemas. Do not maintain a separate hand-written schema for ent types -- only use custom.graphql for non-generated queries.
- **Registering the sqlite3 driver twice:** Both `internal/database/database.go` and `internal/testutil/testutil.go` call `sql.Register("sqlite3", ...)`. This panics if both paths execute. The existing code handles this by having separate init() functions, but adding a third registration point would cause issues.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| GraphQL pagination types | Connection/Edge/PageInfo structs | entgql.RelayConnection() annotation | Generates correct Relay-compliant types with cursor encoding |
| Filter input types | WhereInput structs with operators | entgql.WithWhereInputs(true) | Generates 13,000+ lines of type-safe filter code with all operators |
| Node interface | Custom Noder interface + type switch | entgql WithRelaySpec(true) + generated gql_node.go | Handles all 13 types, field collection, error masking |
| GraphQL schema SDL | Hand-written .graphql file | entgql.WithSchemaGenerator() | Schema stays in sync with ent schemas automatically |
| N+1 query batching | Custom query caching | dataloadgen | Handles batching, caching, concurrency, configurable wait time |
| CORS handling | Manual header setting | rs/cors | Handles preflight, credentials, Vary headers, max-age correctly |
| Query complexity | Manual AST traversal | gqlgen extension.FixedComplexityLimit | Built into gqlgen, counts fields and depth automatically |
| Query depth limiting | Manual recursion counting | oyyblin/gqlgen-depth-limit-extension | Integrates with gqlgen plugin system cleanly |

**Key insight:** entgql + gqlgen generate approximately 20,000 lines of correct, type-safe GraphQL integration code. The custom code needed is only: resolvers (~200 lines), DataLoader setup (~150 lines), HTTP handler wiring (~100 lines), and custom queries (~50 lines).

## Common Pitfalls

### Pitfall 1: Missing RelayConnection Annotation

**What goes wrong:** Without `entgql.RelayConnection()` on schema annotations, the generated GraphQL query returns `[Network!]!` instead of `NetworkConnection!`. This means no cursor pagination, no pageInfo, no edges/nodes structure.
**Why it happens:** The Phase 1 schemas were generated with `entgql.QueryField()` and `entgql.Mutations()` but not `entgql.RelayConnection()`.
**How to avoid:** Add `entgql.RelayConnection()` to ALL 13 schema Annotations() methods before regenerating.
**Warning signs:** GraphQL queries return arrays instead of connection types; no `first`/`after`/`before`/`last` arguments on queries.

### Pitfall 2: graph/ Directory Must Exist Before codegen

**What goes wrong:** `entgql.WithSchemaPath("graph/schema.graphqls")` writes to the graph/ directory. If it does not exist, codegen fails.
**Why it happens:** Phase 1 STATE.md explicitly notes this: "entgql WithSchemaPath requires graph/ directory pre-created before codegen."
**How to avoid:** `mkdir -p graph` before running `go generate ./ent`.
**Warning signs:** Code generation error about missing directory.

### Pitfall 3: Mutation Annotations on Read-Only Schema

**What goes wrong:** `entgql.Mutations(entgql.MutationCreate(), entgql.MutationUpdate())` generates CreateXxxInput and UpdateXxxInput types plus mutation resolvers. For a read-only API (D-08), these are dead code that confuses the schema.
**Why it happens:** Phase 1 included mutation annotations per D-42 ("Include entgql, entproto, and entrest annotations on schemas upfront"). For Phase 2, mutations must be removed.
**How to avoid:** Remove all `entgql.Mutations()` calls from schema annotations before generating the Phase 2 GraphQL schema.
**Warning signs:** GraphQL introspection shows `createNetwork`, `updateNetwork` mutations that do not work.

### Pitfall 4: PeeringDB IDs Are Not Globally Unique

**What goes wrong:** The Relay Node interface assumes globally unique IDs. PeeringDB assigns IDs per-type (Network ID 1, Facility ID 1 are different objects). Without global ID encoding, `node(id: 1)` is ambiguous.
**Why it happens:** PeeringDB uses per-table auto-increment IDs, not UUIDs or globally unique identifiers.
**How to avoid:** Implement D-06: encode IDs as `base64("TypeName:IntegerID")` for the GraphQL `id` field. The `node()` resolver decodes the type name and calls `client.Noder()` with `WithFixedNodeType()`.
**Warning signs:** `node(id: 1)` returns the wrong type or errors; different types with the same numeric ID collide.

### Pitfall 5: DataLoader + entgql CollectFields Conflict

**What goes wrong:** entgql's CollectFields automatically adds eager-loading (WithXxx) to queries based on the GraphQL selection set. If DataLoaders are also resolving the same edges, data is fetched twice.
**Why it happens:** Two different N+1 solutions (CollectFields and DataLoader) operating on the same edges.
**How to avoid:** For edges resolved by DataLoader, use `entgql.Skip(entgql.SkipAll)` on the edge annotation to prevent CollectFields from adding eager loading. Or configure entgql to not eagerly load edges that DataLoaders handle.
**Warning signs:** Duplicate SQL queries for the same relationship; performance worse than expected.

### Pitfall 6: gqlgen.yml Models Section for IntID

**What goes wrong:** If the `models.ID.model` is not set to `graphql.IntID`, gqlgen defaults to string IDs. Ent uses int IDs, causing type mismatch compilation errors.
**Why it happens:** gqlgen defaults assume string-based IDs (common in most GraphQL APIs).
**How to avoid:** Explicitly set `ID.model: github.com/99designs/gqlgen/graphql.IntID` in gqlgen.yml. With Relay global IDs (D-06), this may need to be a custom scalar instead.
**Warning signs:** Compilation errors about `int` vs `string` type mismatches in generated resolvers.

### Pitfall 7: totalCount Performance with Large Datasets

**What goes wrong:** Requesting `totalCount` on connections triggers a COUNT(*) query on every paginated request. With 13 types and thousands of objects, this adds latency.
**Why it happens:** The generated Paginate() method only runs the count query when `totalCount` is requested in the selection set (via `hasCollectedField(ctx, totalCountField)`). This is already optimized per D-12.
**How to avoid:** D-12 already addresses this -- totalCount is only computed when the client explicitly requests it. Document this in the playground examples.
**Warning signs:** Slow queries that include `totalCount` in the selection.

### Pitfall 8: Offset/Limit vs Cursor Pagination Schema Design

**What goes wrong:** Adding offset/limit alongside cursor pagination creates confusing API surface where both can be specified simultaneously.
**Why it happens:** D-01 requires both pagination styles but entgql only generates cursor-based.
**How to avoid:** Design the GraphQL schema so offset/limit are additional arguments on connection queries, but validate that offset/limit and cursor arguments are mutually exclusive. Return an error if both are provided.
**Warning signs:** Clients send both `after` cursor and `offset`, getting unpredictable results.

## Code Examples

### Complete gqlgen Handler Setup

```go
// Source: gqlgen docs + entgql integration pattern
package graphql

import (
    "net/http"

    "github.com/99designs/gqlgen/graphql/handler"
    "github.com/99designs/gqlgen/graphql/handler/extension"
    "github.com/99designs/gqlgen/graphql/playground"
    "github.com/oyyblin/gqlgen-depth-limit-extension/depth"

    "github.com/dotwaffle/peeringdb-plus/graph"
)

// NewHandler creates the GraphQL HTTP handler with all middleware.
func NewHandler(resolver *graph.Resolver) http.Handler {
    srv := handler.NewDefaultServer(
        graph.NewExecutableSchema(graph.Config{
            Resolvers: resolver,
        }),
    )

    // Query complexity limit per D-04
    srv.Use(extension.FixedComplexityLimit(500))

    // Query depth limit per D-04
    srv.AddPlugin(depth.FixedDepthLimit(15))

    // Introspection enabled by default per D-20

    return srv
}

// PlaygroundHandler returns the GraphiQL playground handler.
func PlaygroundHandler() http.HandlerFunc {
    return playground.Handler("PeeringDB Plus - GraphQL", "/graphql")
}
```

### CORS Middleware Setup

```go
// Source: rs/cors documentation
import "github.com/rs/cors"

func corsMiddleware(origins string) func(http.Handler) http.Handler {
    allowedOrigins := strings.Split(origins, ",")
    c := cors.New(cors.Options{
        AllowedOrigins:   allowedOrigins,
        AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
        AllowedHeaders:   []string{"Content-Type", "Authorization"},
        AllowCredentials: false,
        MaxAge:           86400, // 24 hours
    })
    return c.Handler
}
```

### Request Logging Middleware

```go
// Source: stdlib pattern per OBS-1, D-27
func loggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
        next.ServeHTTP(wrapped, r)
        logger.Info("http request",
            slog.String("method", r.Method),
            slog.String("path", r.URL.Path),
            slog.Int("status", wrapped.statusCode),
            slog.Duration("duration", time.Since(start)),
        )
    })
}
```

### Node Resolver with Global ID

```go
// Source: entgql Node interface pattern
func (r *queryResolver) Node(ctx context.Context, id string) (ent.Noder, error) {
    typeName, intID, err := graph.UnmarshalGlobalID(id)
    if err != nil {
        return nil, fmt.Errorf("invalid node ID %q: %w", id, err)
    }
    return r.client.Noder(ctx, intID, ent.WithFixedNodeType(typeName))
}

func (r *queryResolver) Nodes(ctx context.Context, ids []string) ([]ent.Noder, error) {
    noders := make([]ent.Noder, len(ids))
    for i, id := range ids {
        noder, err := r.Node(ctx, id)
        if err != nil {
            return nil, err
        }
        noders[i] = noder
    }
    return noders, nil
}
```

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | testing (stdlib) + enttest |
| Config file | none (Go conventions) |
| Quick run command | `go test ./graph/... ./internal/graphql/... ./internal/middleware/... -v -count=1` |
| Full suite command | `go test -race ./... -count=1` |

### Phase Requirements to Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| API-01 | All 13 types queryable via GraphQL | integration | `go test ./graph/... -run TestQueryAllTypes -v` | Wave 0 |
| API-02 | Relationship traversal in single query | integration | `go test ./graph/... -run TestRelationshipTraversal -v` | Wave 0 |
| API-03 | Filter by any field | integration | `go test ./graph/... -run TestWhereInputFiltering -v` | Wave 0 |
| API-04 | Lookup by ASN | integration | `go test ./graph/... -run TestNetworkByAsn -v` | Wave 0 |
| API-05 | Lookup by ID (Node interface) | integration | `go test ./graph/... -run TestNodeQuery -v` | Wave 0 |
| API-06 | Pagination (cursor + offset/limit) | integration | `go test ./graph/... -run TestPagination -v` | Wave 0 |
| API-07 | GraphQL playground accessible | integration | `go test ./internal/graphql/... -run TestPlaygroundServed -v` | Wave 0 |
| OPS-06 | CORS headers present | unit | `go test ./internal/middleware/... -run TestCORS -v` | Wave 0 |

### Sampling Rate

- **Per task commit:** `go test ./graph/... ./internal/graphql/... ./internal/middleware/... -v -count=1`
- **Per wave merge:** `go test -race ./... -count=1`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps

- [ ] `graph/resolver_test.go` -- integration tests for all ent resolvers with test database
- [ ] `graph/custom_resolver_test.go` -- tests for syncStatus, networkByAsn
- [ ] `graph/dataloader/loader_test.go` -- DataLoader batch function tests
- [ ] `internal/middleware/cors_test.go` -- CORS header verification
- [ ] `internal/middleware/logging_test.go` -- request logging verification

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| dataloaden (codegen) | dataloadgen (generics) | Go 1.18+ (2022) | No code generation step, simpler setup, type-safe |
| Manual GraphQL schema | entgql.WithSchemaGenerator() | entgql 0.4+ | Schema auto-generated from ent, stays in sync |
| Manual WhereInput types | entgql.WithWhereInputs(true) | entgql 0.4+ | All filter types auto-generated with full operator set |
| gqlgen playground (Apollo) | gqlgen playground (GraphiQL) | gqlgen 0.17+ | GraphiQL is now the default playground in gqlgen |

**Deprecated/outdated:**
- `entgql.WithGlobalUniqueID`: Not present in current entgql; use `WithRelaySpec(true)` instead
- Old `playground.Handler` from pre-0.17 gqlgen used Apollo Playground; now uses GraphiQL by default

## Open Questions

1. **Relay Global ID vs Raw Integer ID Coexistence**
   - What we know: D-06 requires Relay-style opaque global IDs. PeeringDB users expect to look up by raw integer IDs (ASN, network ID).
   - What's unclear: Should the GraphQL `id` field return the opaque global ID while a separate `databaseId` or `peeringdbId` field returns the raw integer? Or should the global ID only apply to the `node()` query?
   - Recommendation: Use opaque global IDs for the `id` field (Relay compliance), add a `databaseId: Int!` field to every type for users who need the raw PeeringDB integer. This is the GitHub GraphQL API pattern.

2. **DataLoader vs CollectFields Boundary**
   - What we know: D-13 says use DataLoader, but entgql's CollectFields is already wired into the generated code (gql_collection.go, gql_edge.go).
   - What's unclear: Which edges should use DataLoader vs CollectFields? Should we disable CollectFields entirely?
   - Recommendation: Use CollectFields for top-level query field collection (it optimizes which columns are selected), but use DataLoader for cross-type relationship edges. Do NOT disable CollectFields entirely -- it provides field-level optimization that DataLoader does not.

3. **Offset/Limit Implementation Strategy**
   - What we know: D-01 requires both cursor and offset/limit. entgql only generates cursor pagination.
   - What's unclear: Best way to add offset/limit without duplicating the entire query infrastructure.
   - Recommendation: Add `offset: Int` and `limit: Int` arguments to connection queries in custom.graphql. In the resolver, when offset/limit are provided, skip cursor-based pagination and use `query.Offset(offset).Limit(limit)` directly, wrapping results in the connection type. Validate mutual exclusivity with cursor args.

4. **GraphiQL Initial Query / Example Queries (D-19)**
   - What we know: gqlgen's GraphiQL playground does not have a built-in "initial query" option like Apollo Sandbox does.
   - What's unclear: How to pre-populate example queries in GraphiQL.
   - Recommendation: GraphiQL supports `defaultQuery` in its React props, but gqlgen's playground handler does not expose this. Options: (a) use ApolloSandboxHandler instead (which has WithApolloSandboxInitialStateDocument), (b) create a custom HTML template for GraphiQL, or (c) provide example queries in the API root endpoint documentation. Option (c) is simplest and avoids fighting the tooling.

## Sources

### Primary (HIGH confidence)
- [entgo.io/contrib/entgql package docs](https://pkg.go.dev/entgo.io/contrib/entgql) - RelayConnection, QueryField, WithWhereInputs, WithSchemaGenerator, WithRelaySpec APIs
- [entgo.io GraphQL integration guide](https://entgo.io/docs/graphql/) - Overall integration pattern
- [entgo.io todo-gql tutorial](https://entgo.io/docs/tutorial-todo-gql/) - Step-by-step gqlgen + entgql setup
- [entgo.io filter input tutorial](https://entgo.io/docs/tutorial-todo-gql-filter-input/) - WhereInput filtering pattern
- [entgo.io pagination tutorial](https://entgo.io/docs/tutorial-todo-gql-paginate/) - Relay cursor connections
- [entgo.io node tutorial](https://entgo.io/docs/tutorial-todo-gql-node/) - Relay Node interface
- [gqlgen complexity docs](https://gqlgen.com/reference/complexity/) - FixedComplexityLimit extension API
- [gqlgen dataloaders docs](https://gqlgen.com/reference/dataloaders/) - DataLoader integration with dataloadgen
- [gqlgen playground package](https://pkg.go.dev/github.com/99designs/gqlgen/graphql/playground) - Handler, GraphiqlConfigOption, ApolloSandboxHandler APIs

### Secondary (MEDIUM confidence)
- [vikstrous/dataloadgen](https://pkg.go.dev/github.com/vikstrous/dataloadgen) - v0.0.10, Loader API, WithWait, WithTracer options
- [rs/cors](https://github.com/rs/cors) - CORS middleware for net/http
- [oyyblin/gqlgen-depth-limit-extension](https://pkg.go.dev/github.com/oyyblin/gqlgen-depth-limit-extension/depth) - FixedDepthLimit plugin API

### Tertiary (LOW confidence)
- GraphiQL initial query support - could not verify gqlgen exposes defaultQuery prop; may need custom template or Apollo Sandbox

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - all libraries already in go.mod or well-documented, versions verified
- Architecture: HIGH - entgql + gqlgen integration is mature with official tutorials and extensive generated code already in codebase
- Pitfalls: HIGH - identified from direct codebase inspection (missing RelayConnection, mutation annotations) and official docs
- DataLoader approach: MEDIUM - D-13 vs CollectFields boundary needs testing in practice
- Global ID encoding: MEDIUM - approach is standard (GitHub does this) but implementation details need careful design

**Research date:** 2026-03-22
**Valid until:** 2026-04-22 (stable ecosystem, no breaking changes expected)
