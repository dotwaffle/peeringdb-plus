---
phase: 06-peeringdb-compatibility-layer
plan: 03
subsystem: api
tags: [depth, eager-loading, search, field-projection, peeringdb-compat, ent, sqlite]

# Dependency graph
requires:
  - phase: 06-peeringdb-compatibility-layer
    provides: "Handler dispatch, registry, serializers, filters from plans 01 and 02"
provides:
  - "Depth-aware Get functions for all 13 PeeringDB types with eager-loading"
  - "Search (?q=) with case-insensitive OR matching across type-specific fields"
  - "Field projection (?fields=) limiting response to requested fields"
  - "_set field serialization for depth=2 responses"
affects: [peeringdb-compatibility-layer]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "toMap helper for converting structs to map[string]any for dynamic _set field injection"
    - "orEmptySlice generic helper ensuring nil slices become [] in JSON"
    - "buildSearchPredicate creating OR-combined ContainsFold SQL predicates"
    - "applyFieldProjection preserving _set and expanded FK objects during field filtering"

key-files:
  created:
    - internal/pdbcompat/depth.go
    - internal/pdbcompat/depth_test.go
    - internal/pdbcompat/search.go
  modified:
    - internal/pdbcompat/registry_funcs.go
    - internal/pdbcompat/handler.go
    - internal/pdbcompat/handler_test.go

key-decisions:
  - "Depth expansion uses ent Query + With* eager loading rather than separate queries per relationship"
  - "Depth=2 responses use map[string]any (not structs) to dynamically add _set fields"
  - "Field projection preserves _set and expanded FK objects regardless of field list"

patterns-established:
  - "Depth-aware Get pattern: depth >= 2 uses Query().Where(type.ID(id)).With*().Only(ctx)"
  - "toMap via JSON round-trip for struct-to-map conversion in depth responses"

requirements-completed: [PDBCOMPAT-03, PDBCOMPAT-02]

# Metrics
duration: 9min
completed: 2026-03-22
---

# Phase 06 Plan 03: Depth, Search & Field Projection Summary

**Depth expansion with _set field serialization for all 13 types, text search via ?q=, and field projection via ?fields=**

## Performance

- **Duration:** 9 min
- **Started:** 2026-03-22T23:25:52Z
- **Completed:** 2026-03-22T23:34:52Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments

- Depth-aware Get functions for all 13 PeeringDB types with correct _set fields per type
- Types with children (org, net, fac, ix, ixlan, carrier, campus) return _set arrays at depth=2
- Leaf entities (poc, ixpfx, netfac, netixlan, ixfac, carrierfac) expand FK edges at depth=2
- Empty _set arrays serialize as [] (never null) via generic orEmptySlice helper
- Depth only applies to detail endpoints, not list endpoints (per Pitfall 3)
- Search (?q=) performs case-insensitive OR matching across type-specific SearchFields
- Field projection (?fields=) limits response fields while preserving _set and expanded FK objects

## Task Commits

Each task was committed atomically (TDD pattern):

1. **Task 1: Depth expansion** - `af2201a` (test) / `362bd5c` (feat)
2. **Task 2: Search and field projection** - `034885b` (test) / `c915823` (feat)

## Files Created/Modified

- `internal/pdbcompat/depth.go` - Depth-aware Get functions for all 13 types with eager-loading and _set serialization
- `internal/pdbcompat/depth_test.go` - Integration tests for depth=0, depth=2, empty sets, list ignoring depth, leaf entities
- `internal/pdbcompat/search.go` - Search predicate builder and field projection logic
- `internal/pdbcompat/registry_funcs.go` - Wired depth-aware Get functions replacing basic Get closures
- `internal/pdbcompat/handler.go` - Integrated search and field projection into serveList and serveDetail
- `internal/pdbcompat/handler_test.go` - Tests for search, search on facilities, field projection, unknown fields, projection with depth

## Decisions Made

- Used ent's Query().With*().Only(ctx) pattern for depth expansion rather than separate queries, leveraging ent's eager-loading for efficient single-query fetching
- Depth=2 responses convert base structs to map[string]any via JSON round-trip, then inject _set fields dynamically -- this avoids creating parallel struct types for depth responses
- Field projection preserves _set arrays and expanded FK objects (identified by "id" key presence) regardless of the requested field list, per research recommendation D-14

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- PeeringDB compatibility layer now supports depth expansion, search, and field projection
- All 13 types have depth-aware Get functions with correct _set field mappings
- Phase 06 plans are complete -- layer provides paths, envelope, filters, depth, search, and projection

---
*Phase: 06-peeringdb-compatibility-layer*
*Completed: 2026-03-22*

## Self-Check: PASSED
