---
phase: 02-graphql-api
plan: 01
subsystem: api
tags: [graphql, entgql, gqlgen, relay, pagination, codegen]

# Dependency graph
requires:
  - phase: 01-data-foundation
    provides: entgo schemas with 13 PeeringDB types, sync_status table, database.Open
provides:
  - Relay-compliant GraphQL schema with connection types for all 13 PeeringDB types
  - gqlgen executable schema with NewExecutableSchema
  - Resolver scaffold with stubs for cursor pagination, offset/limit list queries, syncStatus, networkByAsn
  - Custom GraphQL types (SocialMedia, SyncStatus, Map scalar)
  - gqlgen.yml configuration with autobind to ent packages
affects: [02-02, 02-03, 02-04]

# Tech tracking
tech-stack:
  added: [gqlgen v0.17.68, entgql WithRelaySpec]
  patterns: [entgql RelayConnection annotation, gqlgen follow-schema resolver layout, offset/limit list queries as separate Query fields]

key-files:
  created:
    - graph/schema.graphqls
    - graph/custom.graphql
    - graph/gqlgen.yml
    - graph/generated.go
    - graph/resolver.go
    - graph/custom.resolvers.go
    - graph/schema.resolvers.go
    - graph/model/models.go
  modified:
    - ent/entc.go
    - ent/schema/organization.go
    - ent/schema/network.go
    - ent/schema/facility.go
    - ent/schema/internetexchange.go
    - ent/schema/poc.go
    - ent/schema/ixlan.go
    - ent/schema/ixprefix.go
    - ent/schema/ixfacility.go
    - ent/schema/networkixlan.go
    - ent/schema/networkfacility.go
    - ent/schema/carrier.go
    - ent/schema/carrierfacility.go
    - ent/schema/campus.go

key-decisions:
  - "entgql SchemaPath uses ../graph/ relative to ent/ CWD for project-root graph/ as canonical location"
  - "gqlgen.yml uses relative paths from graph/ directory (run gqlgen from graph/ not project root)"
  - "Offset/limit list queries return plain arrays [Type!]! not connection types"
  - "SocialMedia GraphQL type defined in custom.graphql to match ent JSON field type"
  - "PocWhereInput used instead of PointOfContactWhereInput (matching ent generated name)"

patterns-established:
  - "Run gqlgen from graph/ directory: cd graph && go run github.com/99designs/gqlgen generate"
  - "Run ent generate from project root: go generate ./ent"
  - "Custom queries go in graph/custom.graphql, ent-generated schema in graph/schema.graphqls"
  - "Resolver struct holds *ent.Client and *sql.DB for sync_status raw SQL"

requirements-completed: [API-01, API-03, API-05, API-06]

# Metrics
duration: 7min
completed: 2026-03-22
---

# Phase 02 Plan 01: Schema Annotations and GraphQL Scaffold Summary

**Relay-compliant GraphQL schema with 13 connection types, offset/limit list queries, and gqlgen resolver scaffold for read-only PeeringDB API**

## Performance

- **Duration:** 7 min
- **Started:** 2026-03-22T16:43:02Z
- **Completed:** 2026-03-22T16:50:23Z
- **Tasks:** 2
- **Files modified:** 22

## Accomplishments
- Updated all 13 ent schemas from mutation annotations to Relay connection annotations (read-only per D-08)
- Generated GraphQL schema with Relay connection types, Node interface, WhereInput filters, and lazy totalCount (D-12)
- Created 13 offset/limit list queries for simple pagination (D-01) alongside cursor-based Relay pagination
- Scaffolded complete gqlgen resolver layer with stubs for all query types ready for Plan 02

## Task Commits

Each task was committed atomically:

1. **Task 1: Fix schema annotations and regenerate ent code with Relay spec** - `c615f1f` (feat)
2. **Task 2: Create gqlgen configuration, custom schema with offset/limit extensions, resolver scaffold, and model types** - `47aed11` (feat)

## Files Created/Modified
- `ent/schema/*.go` (13 files) - Replaced entgql.Mutations with entgql.RelayConnection
- `ent/entc.go` - Added WithRelaySpec(true), WithConfigPath, updated SchemaPath to ../graph/
- `graph/schema.graphqls` - Auto-generated GraphQL schema with Relay connections, Node interface, WhereInputs
- `graph/custom.graphql` - Custom queries: syncStatus, networkByAsn, 13 offset/limit list queries, SocialMedia type
- `graph/gqlgen.yml` - gqlgen configuration with autobind to ent, ent/schema, graph/model
- `graph/generated.go` - gqlgen generated executable schema (1.8MB)
- `graph/resolver.go` - Root Resolver struct with NewResolver constructor
- `graph/custom.resolvers.go` - Resolver stubs for custom queries (15 list + syncStatus + networkByAsn)
- `graph/schema.resolvers.go` - Resolver stubs for ent connection queries (13 + Node/Nodes)
- `graph/model/models.go` - SyncStatus model type with pointer fields for nullable values

## Decisions Made
- entgql SchemaPath updated from `graph/schema.graphqls` (ent-relative, produced `ent/graph/`) to `../graph/schema.graphqls` (project-root `graph/` as canonical location)
- gqlgen runs from `graph/` directory with config paths relative to that directory
- `Time` scalar already defined by entgql schema generator -- only `Map` scalar added in custom.graphql
- Used `PocWhereInput` (not `PointOfContactWhereInput`) to match ent-generated GraphQL type names
- SocialMedia GraphQL type added to custom.graphql since ent's JSON field generates a reference but not a type definition

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Added SocialMedia GraphQL type definition**
- **Found during:** Task 2 (gqlgen code generation)
- **Issue:** The ent-generated schema.graphqls references `SocialMedia` type in 6 entity types but does not generate a GraphQL type definition for it (it's a Go struct used as a JSON field)
- **Fix:** Added `type SocialMedia { service: String!, identifier: String! }` to graph/custom.graphql
- **Files modified:** graph/custom.graphql
- **Verification:** gqlgen generate succeeds, go build ./... passes
- **Committed in:** 47aed11 (Task 2 commit)

**2. [Rule 3 - Blocking] Fixed entc.go path resolution for project-root graph/ directory**
- **Found during:** Task 1 (ent code generation)
- **Issue:** `go generate ./ent` runs entc.go from the `ent/` directory, so paths like `graph/gqlgen.yml` resolve to `ent/graph/gqlgen.yml` not project-root `graph/gqlgen.yml`
- **Fix:** Changed SchemaPath and ConfigPath to `../graph/schema.graphqls` and `../graph/gqlgen.yml`
- **Files modified:** ent/entc.go
- **Verification:** go generate ./ent succeeds, schema written to correct location
- **Committed in:** c615f1f (Task 1 commit)

**3. [Rule 3 - Blocking] Removed duplicate Time scalar from custom.graphql**
- **Found during:** Task 2 (gqlgen schema loading)
- **Issue:** `scalar Time` was defined in both generated schema.graphqls (by entgql) and custom.graphql -- gqlgen rejected the duplicate
- **Fix:** Removed `scalar Time` from custom.graphql, kept only `scalar Map`
- **Files modified:** graph/custom.graphql
- **Verification:** gqlgen generate succeeds
- **Committed in:** 47aed11 (Task 2 commit)

---

**Total deviations:** 3 auto-fixed (3 blocking)
**Impact on plan:** All fixes were necessary for code generation to succeed. No scope creep.

## Known Stubs

All resolver stubs in `graph/custom.resolvers.go` and `graph/schema.resolvers.go` contain `panic("not implemented")`. This is intentional -- Plan 02-02 implements the resolver bodies. The stubs compile but will panic at runtime until implemented.

## Issues Encountered
None beyond the deviations documented above.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- GraphQL schema and resolver scaffold are complete, ready for Plan 02-02 (resolver implementation)
- All generated code compiles, gqlgen configuration is validated
- Connection types, list queries, and custom queries are all scaffolded with correct signatures

## Self-Check: PASSED

All 8 key files verified present. Both task commit hashes (c615f1f, 47aed11) verified in git log.

---
*Phase: 02-graphql-api*
*Completed: 2026-03-22*
