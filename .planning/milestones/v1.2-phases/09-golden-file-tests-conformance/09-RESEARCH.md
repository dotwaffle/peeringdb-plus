# Phase 9: Golden File Tests & Conformance - Research

**Researched:** 2026-03-23
**Domain:** Go golden file testing, PeeringDB API conformance, structural comparison
**Confidence:** HIGH

## Summary

Phase 9 adds golden file test infrastructure to the existing `internal/pdbcompat` package and creates a new `internal/conformance` package plus `cmd/pdbcompat-check` CLI tool. The codebase is well-structured for this work: the `Registry` maps all 13 PeeringDB types with `List`/`Get` functions, serializers convert ent entities to `peeringdb.*` types, and depth expansion is fully implemented. The existing test infrastructure (`testutil.SetupClient`, `httptest.NewRecorder`, `testEnvelope`) provides all building blocks.

The primary challenge is determinism. The existing `setupTestHandler` uses `time.Now()` and auto-increment IDs, making it unsuitable for golden files. The CONTEXT.md decision to use explicit `SetID()` and a clock interface is correct. A dedicated `setupGoldenTestData` function must create entities with fixed timestamps and explicit IDs so that JSON output is byte-for-byte reproducible across runs.

**Primary recommendation:** Build golden file infrastructure as a single `golden_test.go` file using the standard Go pattern (`-update` flag, `testdata/golden/` directory, `go-cmp` for diffs). Build the conformance library as `internal/conformance` with structure-only comparison (field names, types, nesting). The CLI tool is a thin wrapper around this library.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- Location: `internal/pdbcompat/testdata/golden/`
- File naming: `{type}/{scenario}.json` (e.g., `net/list.json`, `org/detail.json`, `fac/depth.json`)
- JSON format: compact (matches actual API response, not pretty-printed)
- `-update` flag pattern: standard Go `flag.Bool("update", ...)` for regenerating files
- Use `google/go-cmp` v0.7.0 (already indirect dep) promoted to direct for `cmp.Diff`
- Clock interface injected for testability (not time.Now())
- Entity IDs explicitly set with `SetID()` (not relying on auto-increment)
- Dedicated golden file test setup (separate from existing setupTestHandler which uses time.Now())
- All 13 PeeringDB types x 3 scenarios (list, detail, depth) = 39 golden files
- Scenarios: list endpoint response, detail endpoint response, depth-expanded response with `_set` fields
- Standalone binary: `cmd/pdbcompat-check/`
- Structure-only comparison: field names, types, nesting -- not values
- Approach: fetch from beta.peeringdb.com, create entities locally from that data, render through our serializer, compare
- Uses beta.peeringdb.com (not production api.peeringdb.com)
- Gated by `-peeringdb-live` flag (skipped in normal CI)
- Shares comparison library with CLI tool (e.g., `internal/conformance` package)
- Also verifies `meta.generated` field presence across response types

### Claude's Discretion
No specific areas called out -- all key decisions are locked.

### Deferred Ideas (OUT OF SCOPE)
- Golden files for GraphQL and entrest REST surfaces (adequate test coverage exists, PeeringDB compat is the compatibility contract)
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| GOLD-01 | Golden file test infrastructure with `-update` flag for regenerating files | Standard Go `flag.Bool("update", ...)` + `os.WriteFile` + `os.ReadFile` + `cmp.Diff` pattern. All building blocks verified available. |
| GOLD-02 | Golden files for all 13 PeeringDB types -- list endpoint responses | Registry maps all 13 types. `ListFunc` returns `[]any`. Golden tests iterate Registry, call each type's list endpoint via httptest, compare JSON output. |
| GOLD-03 | Golden files for all 13 PeeringDB types -- detail endpoint responses | `GetFunc` returns single entity. Golden tests call `GET /api/{type}/{id}` via httptest. Detail response wraps single object in array per Pitfall 7. |
| GOLD-04 | Golden files for depth-expanded responses with `_set` fields | Depth expansion is fully implemented in `depth.go`. Each parent type has `_set` fields and FK expansion at depth=2. Golden tests call `GET /api/{type}/{id}?depth=2`. |
| CONF-01 | CLI tool that fetches from beta.peeringdb.com and compares response structure | `internal/peeringdb.Client` already handles PeeringDB API fetching. New `internal/conformance` package provides structural comparison. `cmd/pdbcompat-check/` wraps both. |
| CONF-02 | Integration test gated by `-peeringdb-live` flag using beta.peeringdb.com | Same conformance library as CONF-01. Test skips with `t.Skip` unless flag is set. Uses `net/http` to fetch from `https://beta.peeringdb.com/api/`. |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

**Relevant MUST rules:**
- **T-1**: Table-driven tests; deterministic and hermetic by default -- golden file tests iterate all 13 types x 3 scenarios as table entries
- **T-2**: Run `-race` in CI; add `t.Cleanup` for teardown -- golden tests must be race-safe
- **T-3**: Mark safe tests with `t.Parallel()` -- golden tests can run in parallel per type
- **CS-0**: Use modern Go code guidelines -- Go 1.26
- **ERR-1**: Wrap with `%w` and context -- conformance comparison errors must be wrapped
- **API-1**: Document exported items -- `internal/conformance` exports must be documented
- **MD-1**: Prefer stdlib -- use stdlib `encoding/json`, `os`, `flag`, `path/filepath` for golden file IO; only `go-cmp` is non-stdlib
- **CS-5**: Use input structs for functions receiving more than 2 arguments
- **SEC-1**: Validate inputs; set explicit I/O timeouts -- conformance CLI must set HTTP timeouts when fetching from beta.peeringdb.com

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| github.com/google/go-cmp | v0.7.0 | Semantic diff for golden file comparison | Already in go.sum as indirect. Standard for Go test comparisons. Produces human-readable diffs. Promote to direct dependency. |
| testing (stdlib) | Go 1.26 | Test framework | Project standard |
| encoding/json (stdlib) | Go 1.26 | JSON marshal/unmarshal for golden files | Already used throughout pdbcompat |
| os (stdlib) | Go 1.26 | File read/write for golden files | Standard approach |
| flag (stdlib) | Go 1.26 | `-update` and `-peeringdb-live` test flags | Standard Go test flag pattern |
| net/http (stdlib) | Go 1.26 | HTTP client for conformance tool | Already used in peeringdb.Client |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| net/http/httptest (stdlib) | Go 1.26 | HTTP handler testing | Golden file tests hit handlers via httptest.NewRecorder |
| path/filepath (stdlib) | Go 1.26 | Cross-platform path construction | Golden file paths |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| go-cmp | bytes.Equal | go-cmp gives human-readable diff output on failure, bytes.Equal only says "not equal" |
| go-cmp | custom diff | Don't hand-roll JSON diff when go-cmp exists -- per MD-1 preference |
| gregoryv/golden | stdlib + go-cmp | Third-party golden libraries add dependency for minimal value; stdlib pattern is 20 lines |

**Installation:**
```bash
# Promote go-cmp from indirect to direct
go get github.com/google/go-cmp@v0.7.0
```

## Architecture Patterns

### Recommended Project Structure
```
internal/pdbcompat/
  testdata/golden/
    org/
      list.json
      detail.json
      depth.json
    net/
      list.json
      detail.json
      depth.json
    ... (all 13 types)
  golden_test.go         # Golden file test infrastructure
  handler.go             # (existing)
  handler_test.go        # (existing)
  ...

internal/conformance/
  compare.go             # Structural comparison: field names, types, nesting
  compare_test.go        # Unit tests for comparison logic

cmd/pdbcompat-check/
  main.go                # CLI tool wrapping conformance library
```

### Pattern 1: Golden File Test with -update Flag
**What:** Standard Go pattern for golden file testing using a package-level `-update` flag
**When to use:** All golden file tests in golden_test.go
**Example:**
```go
// Source: standard Go testing pattern (used by Go compiler, gofmt, etc.)
var update = flag.Bool("update", false, "update golden files")

func TestGolden(t *testing.T) {
    // ... set up handler with deterministic data ...

    got := rec.Body.Bytes()
    goldenPath := filepath.Join("testdata", "golden", typeName, scenario+".json")

    if *update {
        os.MkdirAll(filepath.Dir(goldenPath), 0o755)
        os.WriteFile(goldenPath, got, 0o644)
        return
    }

    want, err := os.ReadFile(goldenPath)
    if err != nil {
        t.Fatalf("read golden file %s: %v (run with -update to create)", goldenPath, err)
    }

    if diff := cmp.Diff(string(want), string(got)); diff != "" {
        t.Errorf("golden mismatch (-want +got):\n%s", diff)
    }
}
```

### Pattern 2: Deterministic Test Data Setup
**What:** Fixed timestamps and explicit IDs for reproducible JSON output
**When to use:** Golden file test data setup (separate from setupTestHandler)
**Example:**
```go
// Fixed reference time for all golden test entities.
var goldenTime = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

func setupGoldenTestData(t *testing.T) (*Handler, *http.ServeMux) {
    t.Helper()
    client := testutil.SetupClient(t)
    ctx := context.Background()

    // Create org with explicit ID and fixed timestamp.
    org, err := client.Organization.Create().
        SetID(100).
        SetName("Golden Org").
        SetCreated(goldenTime).
        SetUpdated(goldenTime).
        SetStatus("ok").
        Save(ctx)
    // ... create all related entities with explicit IDs ...
}
```

### Pattern 3: Table-Driven Golden Tests
**What:** Iterate all 13 types x 3 scenarios from Registry
**When to use:** Main golden test function
**Example:**
```go
func TestGoldenFiles(t *testing.T) {
    t.Parallel()
    _, mux := setupGoldenTestData(t)

    for typeName := range Registry {
        for _, scenario := range []string{"list", "detail", "depth"} {
            t.Run(typeName+"/"+scenario, func(t *testing.T) {
                t.Parallel()
                url := buildGoldenURL(typeName, scenario, entityID)
                // ... httptest request, compare with golden file ...
            })
        }
    }
}
```

### Pattern 4: Structural Comparison (Conformance)
**What:** Compare JSON response shapes without comparing values
**When to use:** Conformance tool and live integration test
**Example:**
```go
// StructuralDiff compares two JSON objects by field names, types, and nesting.
// Returns nil if structures match, or a list of differences.
func StructuralDiff(reference, actual []byte) ([]Difference, error) {
    var refMap, actMap map[string]any
    json.Unmarshal(reference, &refMap)
    json.Unmarshal(actual, &actMap)

    return compareStructure("", refMap, actMap), nil
}
```

### Anti-Patterns to Avoid
- **Non-deterministic test data:** Using `time.Now()`, auto-increment IDs, or random values in golden file setup. These cause golden files to change on every run.
- **Pretty-printing golden files:** The CONTEXT.md specifies compact JSON (matching actual API output). Using `json.MarshalIndent` would create mismatches with handler output.
- **Comparing values in conformance tool:** The conformance tool compares structure only (field names, types, nesting). PeeringDB beta has real data that changes constantly -- value comparison would always fail.
- **Using `json.NewEncoder` output for golden comparison:** `json.Encoder.Encode()` appends a trailing `\n` to output. The golden file must include this trailing newline to match byte-for-byte.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| JSON diff | Custom JSON differ | `cmp.Diff(string(want), string(got))` | go-cmp produces readable unified diffs with context |
| Test HTTP calls | Custom HTTP caller | `httptest.NewRecorder` + `mux.ServeHTTP` | Existing pattern in handler_test.go, zero network overhead |
| PeeringDB API fetching | New HTTP client | `peeringdb.Client` | Already handles rate limiting, pagination, retry. Reuse for conformance tool. |
| Golden file directory creation | Manual mkdir | `os.MkdirAll(filepath.Dir(path), 0o755)` | Standard one-liner |

**Key insight:** All the hard infrastructure already exists. The handler dispatches via `Registry`, serializers produce PeeringDB-format JSON, and `httptest` captures output. Golden testing is just "save the output, compare next time."

## Common Pitfalls

### Pitfall 1: Non-deterministic JSON Field Ordering
**What goes wrong:** Go maps do not guarantee iteration order. The `toMap()` function in `depth.go` uses `json.Marshal` then `json.Unmarshal` into `map[string]any`, which produces non-deterministic field order when re-marshaled.
**Why it happens:** `json.NewEncoder(w).Encode(resp)` serializes the envelope. For flat entities (peeringdb.Network etc.), struct fields are marshaled in declared order (deterministic). For depth=2 responses, the `toMap()` approach loses struct ordering.
**How to avoid:** For golden file comparison, compare parsed JSON structures (not raw bytes) OR ensure `json.Marshal(map)` produces stable output. In Go, `encoding/json` actually marshals map keys in sorted order, so this should be deterministic. Verify by running tests multiple times.
**Warning signs:** Golden tests pass locally but fail intermittently in CI.

### Pitfall 2: Trailing Newline in json.Encoder.Encode
**What goes wrong:** `json.NewEncoder(w).Encode()` appends `\n` after the JSON. If golden files are created without this trailing newline, comparison fails.
**Why it happens:** The `WriteResponse` function uses `json.NewEncoder(w).Encode(resp)` which adds `\n`. Recording via `httptest.NewRecorder` captures this.
**How to avoid:** Golden files must be written from the exact bytes captured by `httptest.NewRecorder().Body.Bytes()`. The `-update` flag writes these bytes directly.
**Warning signs:** Diff shows only whitespace difference at end of file.

### Pitfall 3: Floating Point Serialization
**What goes wrong:** Latitude/longitude values like `37.7749` may serialize differently across runs or platforms.
**Why it happens:** Go's `json.Marshal` for float64 uses `strconv.FormatFloat` with `'f'` format and minimal precision. This is deterministic for the same input value.
**How to avoid:** Use fixed float64 values in golden test data (e.g., `37.5` not `37.7749123456789`). Verify with `go test -count=10` to catch any non-determinism.
**Warning signs:** Golden files differ only in decimal precision.

### Pitfall 4: SQLite ID Assignment with SetID
**What goes wrong:** `SetID()` may conflict with SQLite auto-increment behavior. If ent generates `AUTOINCREMENT` on the primary key, explicit IDs might cause issues.
**Why it happens:** ent schemas typically use auto-increment IDs. Setting explicit IDs should work with SQLite but requires careful ordering (don't create child before parent).
**How to avoid:** Use IDs in a high range (100+) to avoid any auto-increment collisions. Create entities in dependency order (org first, then net with org FK, etc.).
**Warning signs:** Test setup fails with UNIQUE constraint violation or FOREIGN KEY constraint.

### Pitfall 5: Depth Responses Have Different Structures Per Type
**What goes wrong:** Not all 13 types have the same depth expansion. Some types have `_set` fields, some only expand FK edges, and the leaf entities (poc, ixpfx, netfac, netixlan, ixfac, carrierfac) have no `_set` fields at all.
**Why it happens:** The depth behavior varies by entity type as implemented in `depth.go`.
**How to avoid:** The depth golden file for leaf entities will show expanded FK objects (e.g., `"net": {...}` on netfac) but no `_set` arrays. Each type's depth golden file captures its specific depth expansion behavior.
**Warning signs:** Missing golden files or golden files with unexpected structure.

### Pitfall 6: Conformance Tool Rate Limiting
**What goes wrong:** Making too many requests to beta.peeringdb.com triggers rate limiting (429 responses).
**Why it happens:** The conformance tool needs to fetch multiple types. PeeringDB enforces rate limits.
**How to avoid:** Reuse `peeringdb.Client` which has built-in rate limiting (1 req/3s). The conformance tool should fetch one type at a time with the existing client's rate limiter.
**Warning signs:** 429 HTTP responses, test timeouts.

## Code Examples

### Golden File Update and Compare
```go
// Source: standard Go testing pattern
package pdbcompat

import (
    "flag"
    "os"
    "path/filepath"
    "testing"

    "github.com/google/go-cmp/cmp"
)

var update = flag.Bool("update", false, "update golden files")

func compareOrUpdate(t *testing.T, goldenPath string, got []byte) {
    t.Helper()
    if *update {
        if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
            t.Fatalf("create golden dir: %v", err)
        }
        if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
            t.Fatalf("write golden file: %v", err)
        }
        return
    }
    want, err := os.ReadFile(goldenPath)
    if err != nil {
        t.Fatalf("read golden file %s: %v (run with -update to create)", goldenPath, err)
    }
    if diff := cmp.Diff(string(want), string(got)); diff != "" {
        t.Errorf("golden mismatch for %s (-want +got):\n%s", goldenPath, diff)
    }
}
```

### Deterministic Entity Creation
```go
// Source: project pattern from serializer_test.go (uses time.Date for fixed timestamps)
var goldenTime = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

func setupGoldenTestData(t *testing.T) (*Handler, *http.ServeMux, map[string]int) {
    t.Helper()
    client := testutil.SetupClient(t)
    ctx := context.Background()
    ids := make(map[string]int) // type -> entity ID for detail/depth requests

    org, _ := client.Organization.Create().
        SetID(100).
        SetName("Golden Org").
        SetAka("GO").
        SetCreated(goldenTime).
        SetUpdated(goldenTime).
        SetStatus("ok").
        Save(ctx)
    ids["org"] = org.ID

    // ... create all entities with explicit IDs and fixed timestamps ...

    h := NewHandler(client)
    mux := http.NewServeMux()
    h.Register(mux)
    return h, mux, ids
}
```

### Structural Comparison (Conformance)
```go
// Source: project-specific design from CONTEXT.md
package conformance

// Difference describes a structural mismatch between two JSON responses.
type Difference struct {
    Path    string // JSON path, e.g., "data[0].net_set[0].asn"
    Kind    string // "missing_field", "extra_field", "type_mismatch"
    Details string // Human-readable description
}

// CompareStructure compares the JSON structure of reference and actual responses.
// It checks field names, value types (string, number, bool, null, array, object),
// and nesting depth. Values are not compared.
func CompareStructure(reference, actual map[string]any) []Difference {
    var diffs []Difference
    for key, refVal := range reference {
        actVal, ok := actual[key]
        if !ok {
            diffs = append(diffs, Difference{
                Path: key, Kind: "missing_field",
                Details: "field present in reference but missing in actual",
            })
            continue
        }
        diffs = append(diffs, compareTypes(key, refVal, actVal)...)
    }
    for key := range actual {
        if _, ok := reference[key]; !ok {
            diffs = append(diffs, Difference{
                Path: key, Kind: "extra_field",
                Details: "field present in actual but missing in reference",
            })
        }
    }
    return diffs
}
```

### Conformance CLI Tool
```go
// Source: project-specific design
package main

import (
    "flag"
    "fmt"
    "os"

    "github.com/dotwaffle/peeringdb-plus/internal/conformance"
)

func main() {
    baseURL := flag.String("url", "https://beta.peeringdb.com", "PeeringDB API base URL")
    typeName := flag.String("type", "", "PeeringDB type to check (empty = all)")
    flag.Parse()

    // Fetch from PeeringDB, create local entities, render, compare structure
    diffs, err := conformance.Check(*baseURL, *typeName)
    if err != nil {
        fmt.Fprintf(os.Stderr, "error: %v\n", err)
        os.Exit(1)
    }
    // Report differences
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Custom golden file libraries | stdlib + go-cmp | 2023+ | Go community converged on stdlib pattern; go-cmp provides the diff |
| Exact value comparison for API conformance | Structural comparison | N/A | PeeringDB beta has live changing data; structural comparison is the only viable approach |
| CGo-based test databases | `modernc.org/sqlite` in-memory | Project convention | Fast, isolated test databases per test via `testutil.SetupClient` |

## Open Questions

1. **SetID() availability on all ent entity types**
   - What we know: ent schemas in this project define integer IDs. `SetID()` is typically available on ent create builders.
   - What's unclear: Whether all 13 entity types support `SetID()` or if some use auto-increment only.
   - Recommendation: Verify during implementation by checking generated code. If `SetID()` is unavailable for some types, create entities in dependency order and capture auto-assigned IDs.

2. **Compact JSON format for depth=2 responses**
   - What we know: `json.NewEncoder(w).Encode(resp)` produces compact JSON by default (no indentation). For flat entities, field order follows struct declaration order.
   - What's unclear: For depth=2 responses, the `toMap()` function converts to `map[string]any`. Go's `encoding/json` sorts map keys alphabetically, which may differ from PeeringDB's field ordering.
   - Recommendation: Accept Go's alphabetical key ordering for depth responses in golden files. The golden file captures OUR output, not PeeringDB's ordering. Conformance comparison ignores ordering.

3. **meta.generated field in our responses vs. PeeringDB**
   - What we know: Our `WriteResponse` uses `Meta: struct{}{}` which produces `"meta":{}`. PeeringDB also returns `"meta":{}` on non-paginated responses. The `meta.generated` field appears on paginated responses only.
   - What's unclear: Whether the conformance check should verify `meta.generated` in our responses (we don't currently produce it) or only in PeeringDB responses.
   - Recommendation: The CONTEXT.md says "verifies meta.generated field presence across response types." This likely means verifying that PeeringDB responses include meta.generated when expected, not that our responses include it. Implement as a PeeringDB-side check in the conformance tool.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | All | Yes | 1.26.1 | -- |
| go-cmp | Golden file diffs | Yes (indirect) | v0.7.0 | Promote to direct |
| beta.peeringdb.com | CONF-01, CONF-02 | External service | -- | Skip with warning if unreachable |
| SQLite (modernc.org) | Test database | Yes | In go.mod | -- |

**Missing dependencies with no fallback:**
- None

**Missing dependencies with fallback:**
- `beta.peeringdb.com` -- network-dependent. Conformance tests gated by `-peeringdb-live` flag. CLI tool exits with clear error if unreachable.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) + go-cmp v0.7.0 |
| Config file | None needed (standard `go test`) |
| Quick run command | `go test ./internal/pdbcompat/... -run TestGolden -count=1` |
| Full suite command | `go test -race ./internal/pdbcompat/... ./internal/conformance/... -count=1` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| GOLD-01 | `-update` flag regenerates golden files | unit | `go test ./internal/pdbcompat/... -run TestGolden -update -count=1` then `go test ./internal/pdbcompat/... -run TestGolden -count=1` | Wave 0 |
| GOLD-02 | List golden files for all 13 types | unit | `go test ./internal/pdbcompat/... -run TestGolden/.*list -count=1` | Wave 0 |
| GOLD-03 | Detail golden files for all 13 types | unit | `go test ./internal/pdbcompat/... -run TestGolden/.*detail -count=1` | Wave 0 |
| GOLD-04 | Depth golden files with _set fields | unit | `go test ./internal/pdbcompat/... -run TestGolden/.*depth -count=1` | Wave 0 |
| CONF-01 | CLI tool compares against beta.peeringdb.com | unit + smoke | `go build ./cmd/pdbcompat-check/` | Wave 0 |
| CONF-02 | Integration test gated by -peeringdb-live | integration | `go test ./internal/conformance/... -peeringdb-live -count=1` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/pdbcompat/... ./internal/conformance/... -count=1`
- **Per wave merge:** `go test -race ./internal/pdbcompat/... ./internal/conformance/... -count=1`
- **Phase gate:** Full suite green before verify-work

### Wave 0 Gaps
- [ ] `internal/pdbcompat/golden_test.go` -- golden file test infrastructure and tests
- [ ] `internal/pdbcompat/testdata/golden/**/*.json` -- 39 golden files (generated by -update)
- [ ] `internal/conformance/compare.go` -- structural comparison library
- [ ] `internal/conformance/compare_test.go` -- unit tests for comparison
- [ ] `cmd/pdbcompat-check/main.go` -- CLI tool
- [ ] Promote go-cmp from indirect to direct: `go get github.com/google/go-cmp@v0.7.0`

## Sources

### Primary (HIGH confidence)
- Project codebase: `internal/pdbcompat/` -- all 14 source files examined
- Project codebase: `internal/peeringdb/types.go` -- all 13 PeeringDB type structs
- Project codebase: `internal/peeringdb/client.go` -- API client with rate limiting
- Project codebase: `internal/testutil/testutil.go` -- test client setup
- Project codebase: `testdata/fixtures/*.json` -- 13 existing fixture files
- Project codebase: `go.mod`, `go.sum` -- go-cmp v0.7.0 confirmed as indirect dep
- [beta.peeringdb.com API](https://beta.peeringdb.com/api/org?limit=1) -- verified response format and structure

### Secondary (MEDIUM confidence)
- [Go golden file testing pattern](https://eli.thegreenplace.net/2022/file-driven-testing-in-go/) -- standard testdata pattern
- [go-cmp v0.7.0](https://pkg.go.dev/github.com/google/go-cmp@v0.7.0/cmp) -- published 2025-01-14, Go 1.21+

### Tertiary (LOW confidence)
- None -- all findings verified from codebase or official sources

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- go-cmp already in go.sum, stdlib covers everything else
- Architecture: HIGH -- all building blocks exist in codebase, patterns well-established
- Pitfalls: HIGH -- identified from direct code analysis of handler.go, depth.go, response.go

**Research date:** 2026-03-23
**Valid until:** 2026-04-23 (stable domain, no external API changes expected)
