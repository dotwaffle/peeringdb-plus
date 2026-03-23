# Phase 9: Golden File Tests & Conformance - Context

**Gathered:** 2026-03-23
**Status:** Ready for planning

<domain>
## Phase Boundary

Create golden file test infrastructure for the PeeringDB compat layer with `-update` flag support. Generate golden files for all 13 types across list, detail, and depth-expanded scenarios. Build a conformance CLI tool and integration test that compare against beta.peeringdb.com.

</domain>

<decisions>
## Implementation Decisions

### Golden File Infrastructure
- Location: `internal/pdbcompat/testdata/golden/`
- File naming: `{type}/{scenario}.json` (e.g., `net/list.json`, `org/detail.json`, `fac/depth.json`)
- JSON format: compact (matches actual API response, not pretty-printed)
- `-update` flag pattern: standard Go `flag.Bool("update", ...)` for regenerating files
- Use `google/go-cmp` v0.7.0 (already indirect dep) promoted to direct for `cmp.Diff`

### Deterministic Test Data
- Clock interface injected for testability (not time.Now())
- Entity IDs explicitly set with `SetID()` (not relying on auto-increment)
- Dedicated golden file test setup (separate from existing setupTestHandler which uses time.Now())

### Golden File Scope
- All 13 PeeringDB types x 3 scenarios (list, detail, depth) = 39 golden files
- Scenarios: list endpoint response, detail endpoint response, depth-expanded response with `_set` fields

### Conformance CLI Tool
- Standalone binary: `cmd/pdbcompat-check/`
- Structure-only comparison: field names, types, nesting — not values
- Approach: fetch from beta.peeringdb.com, create entities locally from that data, render through our serializer, compare
- Uses beta.peeringdb.com (not production api.peeringdb.com)

### Conformance Integration Test
- Gated by `-peeringdb-live` flag (skipped in normal CI)
- Shares comparison library with CLI tool (e.g., `internal/conformance` package)
- Also verifies `meta.generated` field presence across response types

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `setupTestHandler()` in handler_test.go creates 3 test networks — golden tests need separate setup with fixed timestamps
- Serializer tests in serializer_test.go already use `time.Date()` — correct pattern for golden files
- `testutil.SetupClient()` creates isolated in-memory SQLite databases per test
- `pdbcompat.Registry` maps all 13 type names to TypeConfig with ListFunc, GetFunc, serializers

### Established Patterns
- Tests use httptest.NewRecorder for HTTP handler testing
- testEnvelope struct for decoding PeeringDB-style JSON responses
- Tests are table-driven and use t.Parallel()

### Integration Points
- Golden files in internal/pdbcompat/testdata/golden/{type}/{scenario}.json
- New golden_test.go file alongside existing handler_test.go
- cmd/pdbcompat-check/ standalone binary
- internal/conformance/ shared comparison library (new package)

</code_context>

<specifics>
## Specific Ideas

No specific requirements beyond what's captured in decisions.

</specifics>

<deferred>
## Deferred Ideas

- Golden files for GraphQL and entrest REST surfaces (adequate test coverage exists, PeeringDB compat is the compatibility contract).

</deferred>
