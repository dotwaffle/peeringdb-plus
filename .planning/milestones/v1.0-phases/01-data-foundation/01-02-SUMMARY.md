---
phase: 01-data-foundation
plan: 02
subsystem: database
tags: [entgo, schemas, peeringdb, otel, hooks, sqlite, crud]

# Dependency graph
requires:
  - phase: 01-01
    provides: "Go module with entgo code generation pipeline, Organization schema, testutil helper"
provides:
  - All 13 PeeringDB object types as entgo schemas with complete field inventories
  - Edge relationships between all entity types (parent-child and junction tables)
  - OTel mutation hooks on all 13 schemas for write tracing
  - Shared SocialMedia type and otelMutationHook helper
  - CRUD and edge traversal tests for all 13 types
  - Isolated test database pattern (unique per-test DB names)
affects: [01-03, 01-04, 01-05, 01-06, 01-07]

# Tech tracking
tech-stack:
  added: []
  patterns: [entgo edge pattern with explicit FK fields, nullable FK for referential integrity violations, shared OTel mutation hook, per-test isolated SQLite databases]

key-files:
  created:
    - ent/schema/types.go
    - ent/schema/hooks.go
    - ent/schema/campus.go
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
    - ent/schema/schema_test.go
  modified:
    - ent/schema/organization.go
    - internal/testutil/testutil.go

key-decisions:
  - "SocialMedia type moved to shared types.go since it is used by 6 schemas (org, net, fac, ix, carrier, campus)"
  - "OTel mutation hook implemented as shared helper function in hooks.go to avoid code duplication across 13 schemas"
  - "testutil.SetupClient uses unique database names (atomic counter) to prevent table-already-exists errors in parallel tests"
  - "All FK fields are Optional().Nillable() per D-20 to handle PeeringDB referential integrity violations"
  - "Choice fields use field.String not field.Enum to avoid sync failures when PeeringDB adds new values"

patterns-established:
  - "Edge pattern: edge.From(name, Type).Ref(back_ref).Field(fk_id).Unique() for FK relationships"
  - "Junction schema pattern: two edge.From() calls for many-to-many junction tables"
  - "Computed field pattern: field.String/Int(name).Optional().Default(value) for serializer-computed fields"
  - "OTel hook pattern: otelMutationHook(typeName) creates spans with ent.type and ent.op attributes"
  - "Test isolation pattern: unique in-memory SQLite database per test function via atomic counter"

requirements-completed: [DATA-01, DATA-02]

# Metrics
duration: 9min
completed: 2026-03-22
---

# Phase 01 Plan 02: Complete PeeringDB Schema Definitions Summary

**All 13 PeeringDB object types modeled as entgo schemas with complete field inventories, FK edges, OTel mutation hooks, and CRUD tests validating creation, nullable FKs, and edge traversal**

## Performance

- **Duration:** 9 min
- **Started:** 2026-03-22T15:02:50Z
- **Completed:** 2026-03-22T15:11:59Z
- **Tasks:** 2
- **Files modified:** 123

## Accomplishments
- All 13 PeeringDB object types defined as entgo schemas with every field from the API field inventory
- Edge relationships model the full PeeringDB entity graph (parent-child and junction tables)
- OTel mutation hooks on all 13 schemas create spans for every write operation (create, update, delete)
- CRUD tests verify all 13 types including nullable FK handling (D-20) and multi-hop edge traversal

## Task Commits

Each task was committed atomically:

1. **Task 1: Create all 13 entgo schemas with fields, edges, indexes, annotations, and OTel mutation hooks** - `af76c9a` (feat, committed as part of parallel execution)
2. **Task 2: Schema CRUD verification tests for all 13 types** - `1b2856e` (test)

## Files Created/Modified
- `ent/schema/types.go` - Shared SocialMedia struct used by 6 schemas
- `ent/schema/hooks.go` - Shared otelMutationHook helper creating OTel spans for mutations
- `ent/schema/organization.go` - Updated with edges to Network, Facility, IX, Carrier, Campus and Hooks()
- `ent/schema/network.go` - Network schema with ASN, policy fields, computed counts, edges to POC/netfac/netixlan
- `ent/schema/facility.go` - Facility schema with CLLI, address fields, edges to org/campus/netfac/ixfac/carrierfac
- `ent/schema/internetexchange.go` - IX schema with protocol flags, IXF import fields, edges to org/ixlan/ixfac
- `ent/schema/poc.go` - Point of Contact schema with role and visibility, edge to network
- `ent/schema/ixlan.go` - IXLan schema with MTU, RS ASN, edges to IX/ixprefix/netixlan
- `ent/schema/ixprefix.go` - IX Prefix schema with protocol and prefix, edge to ixlan
- `ent/schema/ixfacility.go` - IX-Facility junction with edges to IX and facility
- `ent/schema/networkixlan.go` - Network-IXLan junction with IP addresses, speed, BFD support
- `ent/schema/networkfacility.go` - Network-Facility junction with local ASN
- `ent/schema/carrier.go` - Carrier schema with edges to org and carrier_facilities
- `ent/schema/carrierfacility.go` - Carrier-Facility junction with edges to carrier and facility
- `ent/schema/campus.go` - Campus schema with location fields, edges to org and facilities
- `ent/schema/schema_test.go` - CRUD tests for all 13 types, nullable FK tests, edge traversal tests
- `internal/testutil/testutil.go` - Updated for per-test database isolation
- `ent/` (generated) - ~106 generated files for all 13 entity types

## Decisions Made
- SocialMedia type extracted to shared types.go (used by 6 schemas)
- OTel mutation hook as shared function avoids 13x code duplication
- All FK fields Optional().Nillable() per D-20 for PeeringDB FK violations
- Choice fields use field.String (not field.Enum) per research findings -- PeeringDB adds new values without notice
- testutil.SetupClient updated to use atomic counter for unique DB names (fixes parallel test conflicts)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed testutil.SetupClient for parallel test isolation**
- **Found during:** Task 2 (test execution)
- **Issue:** All parallel tests shared the same in-memory SQLite database name ("file:ent"), causing "table already exists" errors when multiple enttest.Open() calls auto-migrated concurrently
- **Fix:** Added atomic counter to generate unique database names per test (file:test_N)
- **Files modified:** internal/testutil/testutil.go
- **Verification:** All 4 test functions run in parallel without conflicts
- **Committed in:** 1b2856e (Task 2 commit)

**2. [Rule 3 - Blocking] Task 1 schemas committed by parallel agent**
- **Found during:** Task 1 (commit phase)
- **Issue:** Parallel agent (01-05) ran go generate after this agent's schema files were on disk, and committed the generated output along with the schema source files in af76c9a
- **Fix:** Accepted the parallel commit as the Task 1 commit; no redundant commit needed since all files are identical
- **Files modified:** None (already committed)
- **Verification:** git diff HEAD confirms no pending changes for schema files

---

**Total deviations:** 2 auto-fixed (1 bug, 1 blocking)
**Impact on plan:** Both necessary for correct operation in parallel execution environment. No scope creep.

## Issues Encountered
- Parallel agent (01-05) committed the generated ent code that included this plan's schema files. This is expected behavior in parallel execution -- the files are identical.
- Go build cache was read-only, required GOCACHE=/tmp override for code generation.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- All 13 PeeringDB schemas defined with complete field coverage and FK edges
- Generated ent client ready for sync worker (01-04), API type definitions (01-03), and code generation tools (01-05, 01-06, 01-07)
- OTel mutation hooks active on all write operations
- Test patterns established for future schema additions or modifications

## Self-Check: PASSED

All 15 key schema files verified present. Both task commits (af76c9a, 1b2856e) verified in git log. All tests pass with -race.

---
*Phase: 01-data-foundation*
*Completed: 2026-03-22*
