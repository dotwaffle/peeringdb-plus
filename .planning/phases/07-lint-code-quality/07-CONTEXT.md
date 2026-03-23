# Phase 7: Lint & Code Quality - Context

**Gathered:** 2026-03-23
**Status:** Ready for planning

<domain>
## Phase Boundary

Configure golangci-lint v2 with generated code exclusions, fix all existing lint violations in hand-written code, and ensure go vet passes clean across the entire codebase. Also clean up known unused code (globalid.go, dataloader middleware, vestigial config.IsPrimary).

</domain>

<decisions>
## Implementation Decisions

### Linter Configuration
- golangci-lint v2 config at `.golangci.yml` (repo root, auto-discovered)
- Linters: defaults (govet, errcheck, staticcheck, unused, gosimple, ineffassign, typecheck) + gocritic, misspell, nolintlint, revive
- No gofumpt (stick with standard gofmt formatting)
- No lll (no line length limit)
- Generated code exclusion: `generated: strict` header detection only (no path-based exclusions)
- Trust the standard `// Code generated ... DO NOT EDIT.` header present in all ent and gqlgen files

### Code Cleanup
- Delete `graph/globalid.go` entirely (2 exported funcs, unused — ent Noder handles global IDs)
- Delete `graph/dataloader/loader.go` AND remove all middleware wiring from main.go (entgql handles N+1 natively)
- Delete `config.IsPrimary` field AND all references throughout codebase (replaced by LiteFS .primary file detection)

### Fix Approach
- Auto-fix mechanical issues (formatting, unused vars, simple errcheck)
- Ask user for guidance on design-level judgment calls

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- 193 Go files total, ~57K lines generated in graph/generated.go alone
- All generated files have standard `// Code generated ... DO NOT EDIT.` headers
- ent/ directory contains both generated code and hand-written schemas in ent/schema/

### Established Patterns
- go vet already passes clean
- All 21 test files pass
- No existing .golangci.yml, Taskfile, Makefile, or CI config

### Integration Points
- graph/dataloader/loader.go is imported in cmd/peeringdb-plus/main.go (line 22) and used at line 137
- config.IsPrimary is set in config.go (line 87) and used in main.go
- graph/globalid.go exports MarshalGlobalID and UnmarshalGlobalID (no callers)

</code_context>

<specifics>
## Specific Ideas

No specific requirements beyond what's captured in decisions.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>
