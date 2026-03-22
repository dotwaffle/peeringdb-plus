---
phase: 01-data-foundation
plan: 01
subsystem: database
tags: [entgo, sqlite, modernc, otel, codegen, organization]

# Dependency graph
requires: []
provides:
  - Go module with all Phase 1 dependencies
  - entgo code generation pipeline (entc.go with entgql extension and sql/upsert)
  - Organization proof-of-concept schema with full PeeringDB field coverage
  - SQLite database helper with WAL mode, FK constraints, busy timeout
  - OpenTelemetry trace provider initialization
  - Environment-based configuration loading with validation
  - In-memory SQLite test helper for ent client tests
  - Multi-stage Dockerfile with -trimpath
  - BSD 3-Clause LICENSE
affects: [01-02, 01-03, 01-04, 01-05, 01-06, 01-07]

# Tech tracking
tech-stack:
  added: [entgo.io/ent v0.14.5, entgo.io/contrib v0.7.0, modernc.org/sqlite v1.47.0, go.opentelemetry.io/otel v1.42.0, github.com/99designs/gqlgen v0.17.88, github.com/lrstanley/entrest v1.0.2]
  patterns: [entgo schema-first code generation, modernc sqlite3 driver registration, WAL+FK pragmas via DSN, in-memory SQLite for tests, env-based config with fail-fast validation]

key-files:
  created:
    - go.mod
    - go.sum
    - LICENSE
    - Dockerfile
    - ent/entc.go
    - ent/generate.go
    - ent/schema/organization.go
    - ent/schema/organization_test.go
    - ent/client.go (generated)
    - internal/config/config.go
    - internal/database/database.go
    - internal/otel/provider.go
    - internal/testutil/testutil.go
    - .gitignore
  modified: []

key-decisions:
  - "enttest package is generated per-project in ent/enttest/, not imported from entgo.io/ent/enttest (v0.14.5 change)"
  - "graph/ directory created under ent/ for entgql schema generation output"
  - "sql.Register sqlite3 kept in both database.go and testutil.go since they never coexist in the same binary"
  - "golang.org/x/time and other unused-at-compile-time deps pruned by go mod tidy; will be re-added when sync client is built"

patterns-established:
  - "Schema pattern: field.Int(id).Positive().Immutable() for PeeringDB IDs"
  - "JSON field pattern: field.JSON(name, []Type{}).Optional() for structured data like social_media"
  - "Nullable coordinate pattern: field.Float(name).Optional().Nillable() for lat/lon"
  - "Test pattern: testutil.SetupClient(t) for in-memory SQLite ent client"
  - "Config pattern: envOrDefault() with parseBool/parseDuration helpers, fail-fast validation"
  - "OTel pattern: InitProvider returns shutdown function for deferred cleanup"

requirements-completed: [STOR-01]

# Metrics
duration: 8min
completed: 2026-03-22
---

# Phase 01 Plan 01: Project Bootstrap Summary

**Go module bootstrapped with entgo code generation pipeline, Organization schema with full PeeringDB field coverage, SQLite/WAL database setup via modernc.org/sqlite, and OTel trace provider -- all validated end-to-end with in-memory CRUD test**

## Performance

- **Duration:** 8 min
- **Started:** 2026-03-22T14:51:51Z
- **Completed:** 2026-03-22T14:59:38Z
- **Tasks:** 2
- **Files modified:** 43

## Accomplishments
- Go module initialized with all core dependencies (entgo v0.14.5, modernc/sqlite v1.47.0, OTel v1.42.0, entgql, entrest, gqlgen)
- entgo code generation pipeline produces working client with GraphQL support from Organization schema
- Organization CRUD test validates full stack: schema -> codegen -> in-memory SQLite -> create/read/update/delete/query
- Infrastructure packages ready: config loading, database setup with WAL/FK/busy_timeout pragmas, OTel trace provider, test helpers

## Task Commits

Each task was committed atomically:

1. **Task 1: Initialize Go module, install dependencies, create project structure and infrastructure** - `2444cfc` (feat)
2. **Task 2: Create proof-of-concept Organization schema and validate entgo code generation pipeline** - `6b1b3aa` (feat)

## Files Created/Modified
- `go.mod` / `go.sum` - Go module definition with all Phase 1 dependencies
- `LICENSE` - BSD 3-Clause license
- `Dockerfile` - Multi-stage build with golang:1.26-alpine, -trimpath, CGO_ENABLED=0
- `.gitignore` - Project gitignore covering binaries, IDE files, database files
- `ent/entc.go` - Code generation config with entgql extension (schema generator, where inputs) and sql/upsert feature
- `ent/generate.go` - go:generate directive for ent codegen
- `ent/schema/organization.go` - Organization schema with 21 fields, SocialMedia JSON type, indexes, entgql annotations
- `ent/schema/organization_test.go` - Table-driven CRUD test with race detection
- `ent/client.go` + 20 generated files - Full ent client, CRUD operations, migration, GraphQL support
- `internal/config/config.go` - Environment-based config: PDBPLUS_DB_PATH, PDBPLUS_PEERINGDB_URL, PDBPLUS_SYNC_TOKEN, PDBPLUS_SYNC_INTERVAL, PDBPLUS_INCLUDE_DELETED, PDBPLUS_IS_PRIMARY, OTEL_EXPORTER_OTLP_ENDPOINT, PDBPLUS_LISTEN_ADDR
- `internal/database/database.go` - SQLite client setup with modernc driver, WAL mode, FK constraints, busy timeout
- `internal/otel/provider.go` - OpenTelemetry trace provider with stdout exporter and shutdown function
- `internal/testutil/testutil.go` - In-memory SQLite ent client helper for tests
- `cmd/peeringdb-plus/.gitkeep` - Main binary directory placeholder
- `cmd/pdb-schema-extract/.gitkeep` - Schema extraction tool directory placeholder
- `cmd/pdb-schema-generate/.gitkeep` - Schema generation tool directory placeholder
- `schema/.gitkeep` - Intermediate JSON schema directory placeholder
- `testdata/fixtures/.gitkeep` - Test fixtures directory placeholder

## Decisions Made
- enttest package is now generated per-project in v0.14.5 (import path: github.com/dotwaffle/peeringdb-plus/ent/enttest)
- Created graph/ directory under ent/ for entgql schema generation output path
- Kept separate sql.Register("sqlite3") calls in database.go and testutil.go since they serve different binaries
- Unused dependencies (golang.org/x/time, automemlimit) pruned by go mod tidy -- will be re-added when their importing code is written

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Created ent/graph/ directory for entgql schema output**
- **Found during:** Task 2 (code generation)
- **Issue:** entgql.WithSchemaPath("graph/schema.graphqls") requires the graph/ directory to exist
- **Fix:** Created ent/graph/ directory before running go generate
- **Files modified:** ent/graph/ (created)
- **Verification:** go generate ./ent succeeds
- **Committed in:** 6b1b3aa (Task 2 commit)

**2. [Rule 3 - Blocking] Fixed enttest import path for ent v0.14.5**
- **Found during:** Task 2 (compilation after codegen)
- **Issue:** enttest is no longer at entgo.io/ent/enttest in v0.14.5; it's generated per-project
- **Fix:** Changed import to github.com/dotwaffle/peeringdb-plus/ent/enttest
- **Files modified:** internal/testutil/testutil.go
- **Verification:** go build ./... succeeds
- **Committed in:** 6b1b3aa (Task 2 commit)

**3. [Rule 2 - Missing Critical] Added .gitignore for project**
- **Found during:** Task 1 (commit preparation)
- **Issue:** Home directory dotfiles and IDE configs would be committed without a .gitignore
- **Fix:** Created .gitignore covering binaries, IDE files, database files, env files, shell configs
- **Files modified:** .gitignore (created)
- **Verification:** git status no longer shows dotfiles as untracked
- **Committed in:** 2444cfc (Task 1 commit)

---

**Total deviations:** 3 auto-fixed (2 blocking, 1 missing critical)
**Impact on plan:** All auto-fixes necessary for correct operation. No scope creep.

## Issues Encountered
- go mod tidy pruned dependencies not yet imported by compilable source (expected; database.go and testutil.go had //go:build ignore initially)
- entgql WithSchemaPath requires the target directory to exist before generation runs

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Project foundation complete -- all subsequent plans can import ent client, config, database, OTel, and test helpers
- Organization schema validates the pattern for remaining 12 PeeringDB schemas (Plan 02)
- Code generation pipeline is proven and ready for bulk schema addition
- Directory structure established for sync client (internal/), extraction tools (cmd/), and test fixtures (testdata/)

## Self-Check: PASSED

All 14 key files verified present. Both task commits (2444cfc, 6b1b3aa) verified in git log.

---
*Phase: 01-data-foundation*
*Completed: 2026-03-22*
