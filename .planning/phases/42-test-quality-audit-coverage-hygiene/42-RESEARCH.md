# Phase 42: Test Quality Audit & Coverage Hygiene - Research

**Researched:** 2026-03-26
**Domain:** Go test quality, coverage tooling, fuzz testing, CI coverage reporting
**Confidence:** HIGH

## Summary

This phase is a cross-cutting quality audit with four distinct workstreams: (1) assertion density audit across all test files, (2) error path coverage verification against every `fmt.Errorf` and `connect.NewError` call site, (3) a fuzz test for the filter parser, and (4) CI coverage reporting that excludes generated code. The codebase has 63 test files across 25+ packages, with approximately 337 `fmt.Errorf` and 91 `connect.NewError` call sites in hand-written code spread across 39 source files.

The primary technical challenge is the coverage exclusion: the project has 23 generated packages under `ent/` and `gen/`, a 57K-line `graph/generated.go`, and 16 `*_templ.go` files. The current CI runs `go test -race -coverprofile=coverage.out ./...` and reports via octocov with no exclusion configuration. Octocov supports `coverage.exclude` with glob patterns that will handle this cleanly. The `-coverpkg` flag provides a complementary approach at the Go toolchain level.

**Primary recommendation:** Use octocov's `coverage.exclude` patterns for CI reporting (simplest, handles file-level exclusion), and also use `-coverpkg` in the `go test` command to limit the coverage denominator to hand-written packages only. Both approaches together ensure accurate numbers.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
None -- auto-generated infrastructure phase, all implementation choices at Claude's discretion.

### Claude's Discretion
All implementation choices are at Claude's discretion -- pure infrastructure phase. Use ROADMAP phase goal, success criteria, and codebase conventions to guide decisions.

### Deferred Ideas (OUT OF SCOPE)
None
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| QUAL-01 | Existing tests audited for assertion density -- no test asserts only err == nil without data checks | Audit methodology documented below: grep for test functions, analyze assertion patterns, identify weak tests needing data assertions |
| QUAL-02 | Every fmt.Errorf and connect.NewError call site has at least one test exercising the error path | 337 fmt.Errorf sites in 39 files + 91 connect.NewError sites inventoried; cross-reference methodology with coverage output documented |
| QUAL-03 | Fuzz test for filter parser covering untrusted input patterns | ParseFilters/buildPredicate in internal/pdbcompat/filter.go identified; Go fuzz test patterns documented; seed corpus strategy defined |
| INFRA-02 | CI coverage reporting excludes generated code (ent/*, gen/*, generated.go, *_templ.go) | Octocov exclude patterns and -coverpkg flag both documented; current .octocov.yml and ci.yml analyzed |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **T-1 (MUST):** Table-driven tests; deterministic and hermetic by default
- **T-2 (MUST):** Run -race in CI; add t.Cleanup for teardown
- **T-3 (SHOULD):** Mark safe tests with t.Parallel()
- **SEC-4 (CAN):** Add fuzz tests for untrusted inputs (directly relevant to QUAL-03)
- **CS-0 (MUST):** Use modern Go code guidelines
- **ERR-1 (MUST):** Wrap with %w and context
- **CI-1 (MUST):** Lint, vet, test (-race), and build on every PR; cache modules/builds

## Standard Stack

### Core (already in project)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| testing (stdlib) | Go 1.26 | Test framework + fuzz | Built-in fuzz testing since Go 1.18. No external fuzzer needed. |
| k1LoW/octocov-action | v1 | CI coverage reporting | Already used in CI. Supports `coverage.exclude` glob patterns for excluding generated files. |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| go tool cover | Go 1.26 | Coverage analysis | Post-processing coverage.out to verify error path coverage |
| go test -coverpkg | Go 1.26 | Coverage package scoping | Limits coverage denominator to hand-written packages |

No new dependencies needed. This phase uses only stdlib and existing CI tools.

## Architecture Patterns

### QUAL-01: Assertion Density Audit Pattern

The audit must identify test functions that assert ONLY `err == nil` (or `err != nil`) and/or status code checks without verifying any returned data property. A test that checks `rec.Code != http.StatusOK` and then also checks `strings.Contains(body, "something")` passes the audit. A test that ONLY checks `rec.Code != http.StatusOK` fails.

**Known weak patterns observed in codebase:**
- Some HTTP handler tests check status code + body content (these are fine)
- Some tests use `t.Fatalf` on error then proceed to data checks (these are fine)
- The `TestParseFilters` in `filter_test.go` checks `wantCount` of predicates but not predicate behavior -- borderline acceptable since predicates are opaque functions

**Audit methodology:**
1. For each `_test.go` file, identify all test functions
2. For each test function (or table-driven subtest), check if it asserts at least one data property beyond err/status
3. If a test only checks error/status, update it with at least one data assertion

### QUAL-02: Error Path Cross-Reference Pattern

**Inventory of error sites in hand-written code:**

| Package | fmt.Errorf count | connect.NewError count | Notes |
|---------|-----------------|----------------------|-------|
| internal/grpcserver/ | ~60 | ~91 | 13 entity files + generic.go + pagination.go. Filter validation (CodeInvalidArgument) and Get not-found (CodeNotFound) are the main patterns. Many already covered by Phase 39 generic_test.go. |
| internal/pdbcompat/ | ~25 | 0 | filter.go (~15), depth.go (~30), registry_funcs.go (~26). filter_test.go covers some. |
| internal/web/ | ~28 | 0 | detail.go, search.go, compare.go. Error paths are DB query failures. |
| internal/sync/ | ~15 | 0 | worker.go, upsert.go, delete.go, status.go. worker_test.go covers some. |
| internal/peeringdb/ | ~10 | 0 | client.go HTTP errors. client_test.go covers pagination errors. |
| graph/ | ~22 | 0 | custom.resolvers.go, schema.resolvers.go, pagination.go. resolver_test.go covers some. |
| internal/config/ | ~5 | 0 | config.go validation errors. Well tested. |
| internal/otel/ | ~8 | 0 | provider.go, metrics.go. Phase 41 added coverage. |
| internal/health/ | ~1 | 0 | handler.go sync status error. |
| internal/conformance/ | ~3 | 0 | compare.go. Test-support code, lower priority. |
| cmd/ | ~10 | 0 | CLI tools. Lower priority (not runtime code). |

**Cross-reference approach:**
1. Run `go test -race -coverprofile=coverage.out -coverpkg=./... ./...`
2. Use `go tool cover -func=coverage.out` to get per-function coverage
3. For each `fmt.Errorf`/`connect.NewError` line, check if the coverage profile shows that line was hit
4. Lines not hit need new tests

### QUAL-03: Fuzz Test Pattern

The filter parser (`internal/pdbcompat/filter.go`) takes untrusted URL query parameters and produces SQL predicates. This is a prime fuzz target because:
- Input comes from user HTTP requests (untrusted)
- It does type conversion (string to int, bool, time, float)
- It has operator dispatch logic
- A panic or incorrect parse would be a production issue

**Fuzz test structure:**
```go
func FuzzFilterParser(f *testing.F) {
    // Seed corpus with known-good inputs
    f.Add("name", "Cloudflare")
    f.Add("asn__gt", "1000")
    f.Add("name__contains", "cloud")
    f.Add("asn__in", "13335,174")
    f.Add("name__regex", ".*")       // unsupported op
    f.Add("asn", "not-a-number")     // type error
    f.Add("created__gte", "1700000000")
    f.Add("info_unicast", "true")
    f.Add("", "")                    // empty
    f.Add("__", "value")             // edge case

    fields := map[string]FieldType{
        "name":         FieldString,
        "asn":          FieldInt,
        "info_unicast": FieldBool,
        "created":      FieldTime,
        "latitude":     FieldFloat,
    }

    f.Fuzz(func(t *testing.T, key, value string) {
        params := url.Values{key: {value}}
        // Must not panic. Errors are acceptable for invalid input.
        result, err := ParseFilters(params, fields)
        if err != nil {
            return // Expected for invalid inputs
        }
        // If no error, predicates must be non-nil slice (possibly empty)
        if result == nil && key != "" {
            // ParseFilters returns nil slice for empty/reserved/unknown params
            // which is fine -- not an error
        }
    })
}
```

**Run command:** `go test -fuzz=FuzzFilterParser -fuzztime=30s ./internal/pdbcompat/`

### INFRA-02: Coverage Exclusion Pattern

**Current CI state:**
- `.github/workflows/ci.yml` line 63: `CGO_ENABLED=1 go test -race -coverprofile=coverage.out ./...`
- `.octocov.yml`: Only specifies `coverage.paths` and `comment.deletePrevious`, no exclusions
- `graph/generated.go`: 57,144 lines (massive -- dominates coverage denominator)
- `ent/` directory: 23 packages of generated code
- `gen/` directory: Generated proto Go types
- `*_templ.go`: 16 files of generated templ code

**Two-pronged approach:**

1. **octocov exclusion** (report-level): Add `coverage.exclude` patterns to `.octocov.yml`:
```yaml
coverage:
  paths:
    - coverage.out
  exclude:
    - 'ent/**/*.go'
    - 'gen/**/*.go'
    - 'graph/generated.go'
    - '**/*_templ.go'
comment:
  deletePrevious: true
```

2. **-coverpkg flag** (measurement-level): Modify CI to scope coverage collection:
```yaml
- name: Run tests with race detector
  run: |
    COVERPKG=$(go list ./... | grep -vE '(ent|gen)/' | tr '\n' ',')
    CGO_ENABLED=1 go test -race -coverprofile=coverage.out -coverpkg="${COVERPKG}" ./...
```

The `-coverpkg` approach excludes entire packages (`ent/`, `gen/`) from the denominator at the Go toolchain level. The octocov `exclude` patterns handle file-level exclusion (`generated.go`, `*_templ.go`) from the report. Together they ensure the reported percentage reflects hand-written code only.

**Note on graph package:** `graph/schema.resolvers.go` and `graph/custom.resolvers.go` are marked as "Code generated" by gqlgen but contain hand-written resolver implementations that gqlgen preserves during regeneration. These files SHOULD be included in coverage (they contain ~500 lines of hand-written logic with error paths). Only `graph/generated.go` (57K lines, pure codegen) should be excluded.

### Anti-Patterns to Avoid
- **Excluding the entire graph/ package**: Would hide 500+ lines of hand-written resolvers. Only exclude `graph/generated.go`.
- **Testing generated code for coverage numbers**: ent/gen code is tested by its generators. Don't write tests for generated ent client methods.
- **Fuzz test that only checks "no panic"**: The fuzz test should also verify that successful parses produce valid predicates (at minimum, non-nil function slice).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Coverage exclusion | Custom coverage post-processing script | octocov `coverage.exclude` + `-coverpkg` | octocov already handles this with glob patterns; -coverpkg is built into Go |
| Error site inventory | Manual file-by-file audit | `grep -rn 'fmt.Errorf\|connect.NewError'` + `go tool cover -func` | Grep finds call sites, cover output shows which lines were hit |
| Fuzz testing | Custom random input generator | `testing.F` stdlib fuzz | Go's built-in fuzzer has coverage-guided mutation, corpus management, and minimization |

## Common Pitfalls

### Pitfall 1: graph/generated.go Dominates Package Coverage
**What goes wrong:** The `graph` package has 57K lines of generated code and ~500 lines of hand-written code. Package-level coverage for `graph` will be ~1% even with 100% hand-written coverage.
**Why it happens:** `go test -coverprofile` reports per-package, and generated.go dwarfs the resolvers.
**How to avoid:** Use octocov's file-level exclude pattern for `graph/generated.go` specifically. Do NOT exclude the entire `graph/` package from coverage.
**Warning signs:** Coverage report showing graph package at <5% despite good resolver tests.

### Pitfall 2: -coverpkg Package List Too Long for Shell
**What goes wrong:** The `go list` output piped to `-coverpkg` can exceed shell argument limits on very large projects.
**Why it happens:** Comma-separated package list with full import paths is verbose.
**How to avoid:** This project has ~26 non-generated packages. The list is ~1.5KB -- well within shell limits. Not a concern here, but worth noting for awareness.
**Warning signs:** Shell error "argument list too long".

### Pitfall 3: Fuzz Test Flakiness in CI
**What goes wrong:** Fuzz tests with `-fuzztime=30s` may find issues on CI that are false positives (e.g., timeout-related, not actual bugs).
**Why it happens:** CI environments have variable CPU speed and load.
**How to avoid:** Run fuzz tests in CI as regression tests (without `-fuzz` flag) using seed corpus only. Run fuzzing separately with `-fuzztime` for discovery, and commit any failure corpus entries to `testdata/fuzz/`.
**Warning signs:** CI test flakes that only occur during fuzzing.

### Pitfall 4: Confusing "Code Generated" Header with Actual Generated Code
**What goes wrong:** `graph/schema.resolvers.go` and `graph/custom.resolvers.go` have "Code generated" headers but contain hand-written resolver implementations.
**Why it happens:** gqlgen adds this header to all files it manages, even those with user-written code that gqlgen preserves.
**How to avoid:** Only exclude files that are purely generated: `generated.go`, `*_templ.go`, and everything under `ent/` and `gen/`. Do NOT exclude `schema.resolvers.go` or `custom.resolvers.go`.
**Warning signs:** Error paths in resolver files showing 0% coverage despite supposedly being "excluded generated code".

### Pitfall 5: Error Path Tests That Just Assert "err != nil"
**What goes wrong:** A test exercises an error path but only checks `err != nil`, not what KIND of error was returned.
**Why it happens:** It's the minimum to get coverage. But it doesn't verify error wrapping, error codes, or error messages.
**How to avoid:** Error path tests should verify the error type (e.g., `connect.CodeOf(err) == connect.CodeInvalidArgument`) and ideally check the error message contains expected context.
**Warning signs:** Tests that pass even when the error type/message changes.

## Code Examples

### Fuzz Test for Filter Parser
```go
// Source: Go fuzz testing documentation (https://go.dev/doc/fuzz/)
func FuzzFilterParser(f *testing.F) {
    // Seed corpus: representative inputs covering all field types and operators
    f.Add("name", "Cloudflare")           // string exact
    f.Add("asn__gt", "1000")              // int comparison
    f.Add("name__contains", "cloud")       // string contains
    f.Add("asn__in", "13335,174")          // int IN
    f.Add("info_unicast", "true")          // bool exact
    f.Add("created__gte", "1700000000")    // time comparison
    f.Add("latitude", "37.7749")           // float exact
    f.Add("name__regex", ".*")             // unsupported operator
    f.Add("asn", "not-a-number")           // type conversion error
    f.Add("", "")                          // empty key
    f.Add("__", "val")                     // empty field name

    fields := map[string]FieldType{
        "name":         FieldString,
        "asn":          FieldInt,
        "info_unicast": FieldBool,
        "created":      FieldTime,
        "latitude":     FieldFloat,
    }

    f.Fuzz(func(t *testing.T, key, value string) {
        params := url.Values{key: {value}}
        _, _ = ParseFilters(params, fields)
        // Success criterion: no panic, no data race.
        // Errors from invalid input are expected and acceptable.
    })
}
```

### octocov Configuration with Exclusions
```yaml
# Source: octocov documentation (https://github.com/k1LoW/octocov)
coverage:
  paths:
    - coverage.out
  exclude:
    - 'ent/**/*.go'
    - 'gen/**/*.go'
    - 'graph/generated.go'
    - '**/*_templ.go'
comment:
  deletePrevious: true
```

### CI Coverage Command with -coverpkg
```yaml
# Source: Go documentation (https://go.dev/doc/build-cover)
- name: Run tests with race detector
  run: |
    COVERPKG=$(go list ./... | grep -vE '/(ent|gen)/' | tr '\n' ',' | sed 's/,$//')
    CGO_ENABLED=1 go test -race -coverprofile=coverage.out -coverpkg="${COVERPKG}" ./...
```

### Error Path Test Pattern (connect.NewError)
```go
// Pattern for testing gRPC error paths
{
    name:    "negative filter ID returns InvalidArgument",
    request: &pb.ListXxxRequest{Filter: &pb.ListXxxRequest_Filter{Id: proto.Int64(-1)}},
    wantCode: connect.CodeInvalidArgument,
    wantMsg:  "id must be positive",
},
```

### Assertion Density: Weak vs Strong Test
```go
// WEAK: Only checks error (fails QUAL-01 audit)
func TestFetchNetwork_Success(t *testing.T) {
    result, err := fetchNetwork(ctx, client, 42)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    // No data assertion -- what did we actually get?
}

// STRONG: Checks error AND data properties (passes QUAL-01 audit)
func TestFetchNetwork_Success(t *testing.T) {
    result, err := fetchNetwork(ctx, client, 42)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if result.ASN != 42 {
        t.Errorf("ASN = %d, want 42", result.ASN)
    }
    if result.Name == "" {
        t.Error("Name is empty, want non-empty")
    }
}
```

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | testing (stdlib) Go 1.26 |
| Config file | None (stdlib) |
| Quick run command | `go test -race -count=1 ./internal/pdbcompat/...` |
| Full suite command | `go test -race ./...` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| QUAL-01 | No test function asserts only err==nil without data checks | manual audit + code review | `go test -race ./...` (validates updated tests compile and pass) | N/A -- audit task |
| QUAL-02 | Every error call site has test coverage | coverage analysis | `go test -race -coverprofile=coverage.out ./... && go tool cover -func=coverage.out` | N/A -- coverage verification |
| QUAL-03 | Fuzz test for filter parser | fuzz + regression | `go test -fuzz=FuzzFilterParser -fuzztime=30s ./internal/pdbcompat/` | Wave 0: internal/pdbcompat/fuzz_test.go |
| INFRA-02 | CI excludes generated code from coverage | CI config + local verify | `go test -race -coverprofile=coverage.out -coverpkg=... ./...` | Wave 0: .octocov.yml, .github/workflows/ci.yml |

### Sampling Rate
- **Per task commit:** `go test -race -count=1 ./...`
- **Per wave merge:** `go test -race ./...` + verify coverage excludes generated code
- **Phase gate:** Full suite green + fuzz test passes + coverage report excludes generated code

### Wave 0 Gaps
- [ ] `internal/pdbcompat/fuzz_test.go` -- FuzzFilterParser fuzz test (QUAL-03)
- No framework install needed -- stdlib testing + fuzz is built-in

## Open Questions

1. **Threshold for "data assertion"**
   - What we know: Tests must assert at least one data property beyond err/status
   - What's unclear: Are HTML body content checks (e.g., `strings.Contains(body, "PeeringDB Plus")`) sufficient data assertions for web handler tests?
   - Recommendation: Yes -- checking that specific expected content appears in the response body counts as a data assertion. The rule targets tests that ONLY check `err == nil` or `status == 200` with zero data inspection.

2. **cmd/ package error paths**
   - What we know: `cmd/pdb-schema-extract/`, `cmd/pdb-schema-generate/`, and `cmd/pdbcompat-check/` have error paths
   - What's unclear: These are development tools, not runtime production code
   - Recommendation: Lower priority for QUAL-02. Focus on runtime packages first. Include cmd/ error paths only if time permits.

3. **internal/conformance/ error paths**
   - What we know: `compare.go` has 3 fmt.Errorf sites
   - What's unclear: This is test-support code used for conformance checking
   - Recommendation: Exclude from QUAL-02 scope -- testing test-support code has diminishing returns.

## Sources

### Primary (HIGH confidence)
- Go fuzz testing documentation: https://go.dev/doc/fuzz/ - Fuzz test API, seed corpus, running with -fuzztime
- Go coverage profiling: https://go.dev/doc/build-cover - -coverpkg flag, coverage profile format
- octocov repository: https://github.com/k1LoW/octocov - coverage.exclude glob patterns

### Secondary (MEDIUM confidence)
- Codebase analysis: 39 source files with fmt.Errorf, 15 files with connect.NewError (direct grep)
- octocov coverage.exclude documentation confirmed via GitHub README

### Tertiary (LOW confidence)
- None

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - stdlib testing, already in use, no new dependencies
- Architecture: HIGH - patterns based on direct codebase analysis and official Go documentation
- Pitfalls: HIGH - identified from direct codebase observation (graph/generated.go size, gqlgen header patterns)
- Coverage exclusion: HIGH - octocov exclude syntax verified from official docs

**Research date:** 2026-03-26
**Valid until:** 2026-04-26 (stable patterns, no fast-moving dependencies)
