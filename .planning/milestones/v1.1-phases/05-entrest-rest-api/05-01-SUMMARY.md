---
phase: 05-entrest-rest-api
plan: 01
subsystem: api
tags: [entrest, rest, openapi, codegen, ent, eager-loading]

# Dependency graph
requires:
  - phase: 01-data-foundation
    provides: ent schemas for all 13 PeeringDB types
  - phase: 02-graphql-api
    provides: entgql extension config in entc.go
provides:
  - entrest codegen extension alongside entgql in ent/entc.go
  - Generated REST handlers under ent/rest/ for all 13 PeeringDB types
  - OpenAPI 3.1 spec at ent/rest/openapi.json with read-only GET operations
  - entrest schema and edge annotations on all 13 ent schemas
affects: [05-02, rest-handler-mounting, integration-tests]

# Tech tracking
tech-stack:
  added: [github.com/lrstanley/entrest@v1.0.2, github.com/go-playground/form/v4@v4.3.0]
  patterns: [dual-extension-codegen, entrest-schema-annotation, eager-load-edge-annotation, ogen-schema-for-json-fields]

key-files:
  created: [ent/rest/server.go, ent/rest/list.go, ent/rest/eagerload.go, ent/rest/sorting.go, ent/rest/openapi.json, ent/rest/create.go, ent/rest/update.go, ent/rest/optional.go]
  modified: [ent/entc.go, ent/schema/organization.go, ent/schema/network.go, ent/schema/facility.go, ent/schema/internetexchange.go, ent/schema/ixlan.go, ent/schema/ixprefix.go, ent/schema/ixfacility.go, ent/schema/networkfacility.go, ent/schema/networkixlan.go, ent/schema/carrier.go, ent/schema/carrierfacility.go, ent/schema/campus.go, ent/schema/poc.go, ent/schema/types.go, go.mod, go.sum]

key-decisions:
  - "entrest and entgql coexist successfully as dual extensions in entc.go"
  - "social_media JSON fields require explicit ogen.Schema annotation for OpenAPI type inference"
  - "go-playground/form/v4 is a transitive dependency required by generated REST code"

patterns-established:
  - "Dual-extension codegen: entgql and entrest registered together via entc.Extensions(gqlExt, restExt)"
  - "entrest schema annotation: entrest.WithIncludeOperations(Read, List) in each schema's Annotations()"
  - "Edge eager-loading: entrest.WithEagerLoad(true) on every edge definition"
  - "JSON field schema: socialMediaSchema() helper with ogen DSL for custom struct OpenAPI types"

requirements-completed: [REST-01, REST-02, REST-03, REST-04]

# Metrics
duration: 8min
completed: 2026-03-22
---

# Phase 05 Plan 01: entrest REST API Codegen Summary

**entrest v1.0.2 dual codegen with entgql producing read-only REST handlers and OpenAPI spec for all 13 PeeringDB types**

## Performance

- **Duration:** 8 min
- **Started:** 2026-03-22T22:01:45Z
- **Completed:** 2026-03-22T22:10:41Z
- **Tasks:** 2
- **Files modified:** 51

## Accomplishments
- Installed entrest v1.0.2 and configured as second codegen extension alongside entgql
- Annotated all 13 PeeringDB schemas with entrest read-only operations (Read + List)
- Annotated all 34 edges across 13 schemas with eager-loading support
- Generated ent/rest/ directory with REST handlers and OpenAPI 3.1 spec containing only GET operations (61 paths)
- Validated entgql and entrest coexist: single `go generate` produces both GraphQL and REST code without conflicts

## Task Commits

Each task was committed atomically:

1. **Task 1: Install entrest and configure dual codegen extension** - `9db37d0` (feat)
2. **Task 2: Annotate all 13 schemas and edges, run codegen, verify coexistence** - `35cd1ce` (feat)

## Files Created/Modified
- `ent/entc.go` - Added entrest extension alongside entgql with read-only operations
- `ent/schema/types.go` - Added socialMediaSchema() helper for OpenAPI spec generation
- `ent/schema/organization.go` - entrest annotations on schema and 5 edges
- `ent/schema/network.go` - entrest annotations on schema and 4 edges
- `ent/schema/facility.go` - entrest annotations on schema and 5 edges
- `ent/schema/internetexchange.go` - entrest annotations on schema and 3 edges
- `ent/schema/ixlan.go` - entrest annotations on schema and 3 edges
- `ent/schema/ixprefix.go` - entrest annotations on schema and 1 edge
- `ent/schema/ixfacility.go` - entrest annotations on schema and 2 edges
- `ent/schema/networkfacility.go` - entrest annotations on schema and 2 edges
- `ent/schema/networkixlan.go` - entrest annotations on schema and 2 edges
- `ent/schema/carrier.go` - entrest annotations on schema and 2 edges
- `ent/schema/carrierfacility.go` - entrest annotations on schema and 2 edges
- `ent/schema/campus.go` - entrest annotations on schema and 2 edges
- `ent/schema/poc.go` - entrest annotations on schema and 1 edge
- `ent/rest/server.go` - Generated REST server with route registration
- `ent/rest/list.go` - Generated list/pagination logic
- `ent/rest/eagerload.go` - Generated edge eager-loading
- `ent/rest/sorting.go` - Generated sort parameter handling
- `ent/rest/openapi.json` - Generated OpenAPI 3.1 specification (61 GET-only paths)
- `go.mod` / `go.sum` - Added entrest v1.0.2, go-playground/form/v4

## Decisions Made
- **entrest + entgql coexistence confirmed:** Both extensions produce separate output files (entgql: gql_*.go, entrest: ent/rest/) without template or import conflicts. This was the primary risk identified in research.
- **social_media JSON fields need explicit OpenAPI schema:** entrest cannot auto-infer the OpenAPI type for custom Go struct types like `[]SocialMedia`. Added `entrest.WithSchema(socialMediaSchema())` using ogen DSL to provide the schema. This is a minimal, targeted per-field annotation consistent with D-05 (minimal annotations).
- **go-playground/form/v4 transitive dependency:** Generated REST code imports this for form decoding. Added via `go get` as part of build fix.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Added OpenAPI schema for social_media JSON fields**
- **Found during:** Task 2 (codegen)
- **Issue:** entrest panicked with "no openapi type exists for type []schema.SocialMedia" because it cannot auto-infer OpenAPI types for custom Go struct types
- **Fix:** Created `socialMediaSchema()` helper in types.go using ogen DSL, added `entrest.WithSchema(socialMediaSchema())` annotation to all 6 social_media field definitions
- **Files modified:** ent/schema/types.go, ent/schema/organization.go, ent/schema/network.go, ent/schema/facility.go, ent/schema/internetexchange.go, ent/schema/carrier.go, ent/schema/campus.go
- **Verification:** `go generate ./ent/` succeeds, openapi.json describes social_media as array of objects with service/identifier string properties
- **Committed in:** 35cd1ce (Task 2 commit)

**2. [Rule 3 - Blocking] Installed missing go-playground/form/v4 dependency**
- **Found during:** Task 2 (build after codegen)
- **Issue:** Generated ent/rest/server.go imports github.com/go-playground/form/v4 which was not in go.mod
- **Fix:** Ran `go get github.com/go-playground/form/v4`
- **Files modified:** go.mod, go.sum
- **Verification:** `go build ./...` succeeds
- **Committed in:** 35cd1ce (Task 2 commit)

---

**Total deviations:** 2 auto-fixed (2 blocking issues)
**Impact on plan:** Both auto-fixes necessary for codegen to succeed. The social_media schema annotation is a known pitfall (Pitfall 3 in research). No scope creep.

## Issues Encountered
None beyond the auto-fixed deviations above.

## User Setup Required
None - no external service configuration required.

## Known Stubs
None - all generated code is fully functional, no placeholder data or TODO items.

## Next Phase Readiness
- Generated REST handlers ready for mounting on the shared HTTP mux (Plan 05-02)
- OpenAPI spec ready for serving at /rest/v1/openapi.json
- All 13 entity types have read + list operations with eager-loading on all edges
- CORS middleware and readiness gating still need to be wired (Plan 05-02)

## Self-Check: PASSED

- All key files verified to exist (ent/entc.go, ent/rest/server.go, ent/rest/openapi.json, etc.)
- Both commits verified (9db37d0, 35cd1ce)
- 13 schemas with entrest annotations confirmed
- 34 eager-load edge annotations confirmed
- OpenAPI spec contains only GET methods confirmed

---
*Phase: 05-entrest-rest-api*
*Completed: 2026-03-22*
