---
phase: 01-data-foundation
plan: 05
subsystem: database
tags: [schema-extraction, code-generation, django-parser, entgo, json-schema]

# Dependency graph
requires:
  - phase: 01-data-foundation/01-01
    provides: Go module with dependencies, project structure, Organization proof-of-concept schema
provides:
  - Schema extraction tool parsing Django Python source to intermediate JSON
  - Schema generation tool producing entgo schema files from JSON
  - Complete peeringdb.json with all 13 PeeringDB object types and field metadata
  - go:generate pipeline for automated schema updates
affects: [01-02, 01-03, 01-06, 01-07]

# Tech tracking
tech-stack:
  added: []
  patterns: [regex pattern-matching for Python parsing, text/template for Go code generation, go/format.Source for output formatting, go/parser for test validation]

key-files:
  created:
    - cmd/pdb-schema-extract/main.go
    - cmd/pdb-schema-extract/main_test.go
    - cmd/pdb-schema-extract/testdata/serializers.py
    - cmd/pdb-schema-extract/testdata/abstract.py
    - cmd/pdb-schema-extract/testdata/concrete.py
    - cmd/pdb-schema-extract/testdata/const.py
    - cmd/pdb-schema-generate/main.go
    - cmd/pdb-schema-generate/main_test.go
    - schema/peeringdb.json
    - schema/generate.go
  modified:
    - .gitignore

key-decisions:
  - "Pattern-matching (regex) parser chosen over full Python AST per Pitfall 8 -- more robust for Django's predictable field definition patterns"
  - "Generated schemas include FK fields as Optional().Nillable() per D-20 for referential integrity violation handling"
  - "peeringdb.json committed as reference artifact documenting current PeeringDB data model for all 13 types"
  - "go:generate directive in schema/generate.go only chains generation step (not extraction) since extraction requires local PeeringDB repo"

patterns-established:
  - "Extraction pattern: regex per-line parsing of Django model/serializer source"
  - "Generation pattern: text/template with go/format.Source for entgo schema output"
  - "Computed field pattern: serializer-computed fields stored as regular entgo fields per D-40"
  - "FK field pattern: all FK fields Optional().Nillable() per D-20"

requirements-completed: [DATA-01, DATA-02]

# Metrics
duration: 10min
completed: 2026-03-22
---

# Phase 01 Plan 05: Schema Extraction Pipeline Summary

**Go-based schema extraction pipeline: regex parser for Django Python source producing intermediate JSON with all 13 PeeringDB types, plus entgo code generator with formatted output, edges, indexes, and entgql annotations**

## Performance

- **Duration:** 10 min
- **Started:** 2026-03-22T15:03:04Z
- **Completed:** 2026-03-22T15:13:24Z
- **Tasks:** 2
- **Files modified:** 10

## Accomplishments
- Schema extraction tool parses Django serializer/model Python source via regex pattern matching, outputs intermediate JSON with field metadata, FK references, read-only annotations, and optional live API validation
- Schema generation tool reads JSON and produces formatted entgo schema Go files with fields, edges, indexes, entgql annotations
- Complete peeringdb.json committed with all 13 PeeringDB object types (org, campus, fac, carrier, carrierfac, ix, ixlan, ixpfx, ixfac, net, poc, netfac, netixlan) and full field inventory
- Pipeline chainable via go:generate per D-18

## Task Commits

Each task was committed atomically:

1. **Task 1: Build schema extraction tool that parses Django Python source** - `af76c9a` (feat)
2. **Task 2: Build schema generation tool that produces entgo schemas from JSON, create sample peeringdb.json** - `2329d94` (feat)

## Files Created/Modified
- `cmd/pdb-schema-extract/main.go` - Schema extraction tool with regex-based Django Python parser, JSON output, optional API validation
- `cmd/pdb-schema-extract/main_test.go` - Table-driven tests for serializer parsing, model field extraction, FK detection, choice constants
- `cmd/pdb-schema-extract/testdata/*.py` - Fixture Django source files for testing
- `cmd/pdb-schema-generate/main.go` - Schema generation tool producing formatted entgo schema Go files from JSON
- `cmd/pdb-schema-generate/main_test.go` - Tests verifying generated code parses as valid Go, field code generation, indexes
- `schema/peeringdb.json` - Complete intermediate JSON schema for all 13 PeeringDB object types
- `schema/generate.go` - go:generate directive chaining the pipeline
- `.gitignore` - Added compiled binary entries for pdb-schema-extract and pdb-schema-generate

## Decisions Made
- Used regex pattern-matching over Python AST parsing per Pitfall 8 recommendation -- Django model/serializer definitions are predictable enough for reliable regex extraction
- FK fields generated as Optional().Nillable() per D-20 to handle PeeringDB's referential integrity violations
- Committed peeringdb.json as a reference artifact -- it documents the current PeeringDB data model and serves as both extraction output reference and generation input
- go:generate only chains the generation step in schema/generate.go because extraction requires a local PeeringDB repo clone

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Added compiled binaries to .gitignore**
- **Found during:** Task 2 (post-commit cleanup)
- **Issue:** `go build ./cmd/pdb-schema-extract/` and `go build ./cmd/pdb-schema-generate/` produce binaries in the project root that were not in .gitignore
- **Fix:** Added `/pdb-schema-extract` and `/pdb-schema-generate` to .gitignore
- **Files modified:** .gitignore
- **Verification:** `git status` no longer shows binaries as untracked
- **Committed in:** docs commit (plan metadata)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Minor housekeeping fix. No scope creep.

## Issues Encountered
None -- both tools built and tested cleanly on first attempt.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Extraction pipeline complete -- can regenerate entgo schemas from JSON whenever PeeringDB data model changes
- peeringdb.json serves as reference for sync client field mapping (Plan 03/04)
- Generated schemas match hand-written schemas in structure (fields, edges, indexes, annotations)
- Pipeline can be extended with extraction step when user clones PeeringDB Python repo locally

## Self-Check: PASSED

All 6 key files verified present. Both task commits (af76c9a, 2329d94) verified in git log.

---
*Phase: 01-data-foundation*
*Completed: 2026-03-22*
