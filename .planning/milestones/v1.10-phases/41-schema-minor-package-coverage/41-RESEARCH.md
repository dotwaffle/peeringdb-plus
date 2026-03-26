# Phase 41: Schema & Minor Package Coverage - Research

**Researched:** 2026-03-26
**Domain:** Go test coverage for ent schema hooks, relationship constraints, and three utility packages
**Confidence:** HIGH

## Summary

This phase targets six requirements across two domains: ent/schema hand-written code (hooks error path, FK edge cases, 65%+ coverage) and three minor utility packages (internal/otel, internal/health, internal/peeringdb each at 90%+). All four packages already have substantial test suites with clear coverage baselines.

Current coverage baselines measured with `go test -race -cover`:
- `ent/schema/` -- 47.4% (target: 65%). Fields and Hooks at 100%, but all 39 Edges/Indexes/Annotations methods at 0%. The hook error path (span.RecordError) is the only untested branch in hooks.go (90% coverage).
- `internal/otel/` -- 84.0% (target: 90%). Gaps: InitMetrics error branches (70%), Setup error branches (80%), buildResource VCS fallback path (85.7%).
- `internal/health/` -- 84.6% (target: 90%). Gaps: checkSync at 65.2% -- missing "running" with no previous completed sync, "unknown sync status" branch, and sync.GetLastStatus returning error.
- `internal/peeringdb/` -- 83.2% (target: 90%). Gaps: FetchAll io.ReadAll error path, FetchType unmarshal error, parseMeta edge cases, and SetRateLimit/SetRetryBaseDelay at 0%.

**Primary recommendation:** Write focused, targeted tests for each uncovered branch. No new dependencies or architectural changes needed -- this is pure test expansion work.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
None -- auto-generated infrastructure phase.

### Claude's Discretion
All implementation choices are at Claude's discretion -- pure infrastructure phase. Use ROADMAP phase goal, success criteria, and codebase conventions to guide decisions.

### Deferred Ideas (OUT OF SCOPE)
None
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SCHEMA-01 | otelMutationHook error paths tested | hooks.go line 28 (`span.RecordError(err)`) is the only untested branch. Test by triggering a mutation failure through the hook and asserting error propagation + no panic. |
| SCHEMA-02 | Relationship constraint validation tested for FK edge cases | FK enforcement is enabled (`_pragma=foreign_keys(1)` in testutil). Test creating entities with non-existent FK references and verify ent returns errors. Existing TestNullableFK covers nil FKs but not invalid FK references. |
| SCHEMA-03 | ent/schema hand-written code reaches 65%+ coverage | Currently 47.4%. The 39 Edges/Indexes/Annotations methods at 0% are the bulk of uncovered statements. Directly calling these methods in tests raises coverage. Combined with SCHEMA-01 error path, achievable target. |
| MINOR-01 | internal/otel reaches 90%+ coverage with error path tests | Currently 84.0%. Need: InitMetrics error branches (register failure simulation), Setup early-return errors, buildResource VCS revision fallback. ~6% gap to close. |
| MINOR-02 | internal/health reaches 90%+ coverage with edge case tests | Currently 84.6%. Need: checkSync "running with no previous completed sync", "unknown sync status", GetLastStatus error propagation, getLastCompletedSync scan error. ~5.4% gap. |
| MINOR-03 | internal/peeringdb reaches 90%+ coverage with error path tests | Currently 83.2%. Need: FetchAll body read error, JSON decode error, FetchType unmarshal error, parseMeta edge cases, SetRateLimit/SetRetryBaseDelay coverage. ~6.8% gap. |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **T-1 (MUST)**: Table-driven tests; deterministic and hermetic by default
- **T-2 (MUST)**: Run `-race` in CI; add `t.Cleanup` for teardown
- **T-3 (SHOULD)**: Mark safe tests with `t.Parallel()`
- **CS-0 (MUST)**: Use modern Go code guidelines
- **ERR-1 (MUST)**: Wrap with `%w` and context
- **ERR-2 (MUST)**: Use `errors.Is`/`errors.As` for control flow; no string matching
- **OBS-1 (MUST)**: Structured logging (`slog`)
- Out of scope: no new test framework (testify, gomock) per REQUIREMENTS.md

## Architecture Patterns

### Test Organization
Tests live in `_test.go` files within each package (same package access for unexported functions like `otelMutationHook`, `checkSync`, `parseMeta`, etc.).

### ent/schema Test Strategy

**Package:** `ent/schema` (package `schema_test` for external tests, `schema` for internal hook testing)

The 47.4% baseline is dominated by 39 Edges/Indexes/Annotations methods at 0%. These are static configuration methods -- they return slices of ent descriptors. Calling them directly in a test is straightforward and raises coverage significantly.

**Statement count analysis:**
- 67 functions total in ent/schema
- 27 at 100% (all Fields + Hooks + types.go)
- 39 at 0% (13x Edges + 13x Indexes + 13x Annotations)
- 1 at 90% (otelMutationHook -- error branch)

To reach 65%: need to cover ~18 of the 39 zero-coverage functions (each is 1 return statement). Covering all 39 is simple and gets to ~100% of coverable statements.

**Key insight:** Edges/Indexes/Annotations are configuration methods called by ent's codegen/enttest framework. Coverage tool doesn't see those calls because they happen in the `enttest` package, not the `schema_test` package. A simple test that calls each method directly is sufficient.

### internal/otel Test Strategy

**Package:** `internal/otel` (package `otel` -- internal access)

Coverage gaps by function:
| Function | Current | Gap | Test Strategy |
|----------|---------|-----|---------------|
| InitMetrics | 70% | Error branch per metric registration | OTel API never returns metric registration errors with a valid MeterProvider, so these branches are defensive. Test is hard to trigger without mocking. Accept or skip. |
| Setup | 80% | Early returns on exporter creation errors | Set invalid OTEL_*_EXPORTER env var to trigger autoexport errors |
| buildResource | 85.7% | VCS revision fallback when version is "(devel)" | Already partially tested; the remaining branch is the `len(s.Value) >= 7` short-circuit |
| InitFreshnessGauge | 80% | The `!ok` return nil path in callback | Test with a lastSyncFn that returns `false` |

**Realistic assessment:** The InitMetrics error branches (70%) are nearly impossible to trigger without mocking the OTel meter API, because `meter.Float64Histogram()` and `meter.Int64Counter()` never return errors with a valid MeterProvider. To reach 90% overall, the other functions must be fully covered. Current math: 84% baseline, need +6%. Setup errors, buildResource edges, and InitFreshnessGauge no-sync path should be sufficient.

### internal/health Test Strategy

**Package:** `internal/health` (package `health_test` -- external tests)

Coverage gaps by function:
| Function | Current | Gap | Test Strategy |
|----------|---------|-----|---------------|
| checkSync | 65.2% | "running" with getLastCompletedSync returning nil, "unknown status" branch, GetLastStatus error | Insert rows with status="running" only (no prior completed), status="bogus", corrupt table |
| getLastCompletedSync | 86.7% | Scan error, completedAt.Valid false path | Already covers the happy path; need a row where completed_at IS NULL and error_message IS NULL |

**Approach:** Add table-driven test cases to the existing TestReadinessHandler for:
1. Running sync with no previous completed sync (only running row in table)
2. Unknown sync status value (e.g., "bogus")
3. GetLastStatus scan error (corrupt/missing table)

### internal/peeringdb Test Strategy

**Package:** `internal/peeringdb` (package `peeringdb` -- internal access)

Coverage gaps by function:
| Function | Current | Gap | Test Strategy |
|----------|---------|-----|---------------|
| parseMeta | 83.3% | Empty/nil meta, malformed JSON | Test with `nil`, `[]byte{}`, invalid JSON |
| FetchAll | 80.8% | io.ReadAll error, json.Unmarshal error | Server returning truncated/invalid body |
| FetchType | 80.0% | Unmarshal error on individual item | Server returning valid array with unmarshalable item |
| doWithRetry | 84.0% | Context cancellation between retries | Cancel context after first failed attempt |
| SetRateLimit | 0% | Never called in test code | Call once; trivial one-liner |
| SetRetryBaseDelay | 0% | Never called in test code | Call once; trivial one-liner |

**Approach:** httptest server returning malformed responses. SetRateLimit/SetRetryBaseDelay are trivially tested by just calling them.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| OTel test providers | Custom mock TracerProvider | `sdktrace.NewTracerProvider(sdktrace.WithSyncer(tracetest.NewInMemoryExporter()))` | Already used in client_test.go; standard OTel test pattern |
| HTTP test servers | Custom TCP listeners | `httptest.NewServer` | Already used extensively in client_test.go |
| SQLite test DBs | File-based temp DBs | `testutil.SetupClient(t)` / `openTestDB(t)` with `:memory:` | Already established patterns |
| Assertion helpers | Custom test utilities | stdlib `testing` + direct assertions | Project convention: no testify/gomock |

## Common Pitfalls

### Pitfall 1: OTel InitMetrics Error Branches Are Unreachable
**What goes wrong:** Trying to force OTel meter registration to return errors
**Why it happens:** The OTel API is designed so that meter.Float64Histogram() etc. never return errors with a valid MeterProvider -- they return noop instruments instead
**How to avoid:** Accept that InitMetrics individual registration branches may not be coverable. Focus coverage efforts on Setup() and buildResource() which have reachable error paths
**Warning signs:** Spending time trying to construct a broken MeterProvider

### Pitfall 2: Edges/Indexes/Annotations Coverage Appears Low But Is Framework-Called
**What goes wrong:** Assuming these methods are untested
**Why it happens:** enttest calls them during schema setup, but coverage only instruments the package under test. Since enttest is in a different package, these calls aren't counted.
**How to avoid:** Simple direct-call tests. Each method returns a static slice -- call it and assert length > 0. This is not testing ent behavior; it's covering the configuration code.
**Warning signs:** Overcomplicating tests for these static methods

### Pitfall 3: FK Constraint Tests Depend on SQLite Foreign Key Pragma
**What goes wrong:** FK constraint violations silently succeed
**Why it happens:** SQLite requires `PRAGMA foreign_keys = ON` to enforce FK constraints. If the pragma isn't set, invalid FK references are silently accepted.
**How to avoid:** testutil.SetupClient already sets `_pragma=foreign_keys(1)`. Verify this is the case before writing FK tests.
**Warning signs:** FK tests passing when they should fail

### Pitfall 4: SetRateLimit/SetRetryBaseDelay Coverage
**What goes wrong:** These show 0% but are actually used by existing tests (via direct field assignment)
**Why it happens:** Tests set `client.limiter.SetLimit()` and `client.retryBaseDelay =` directly instead of using the public methods
**How to avoid:** Either call the public methods in a dedicated test, or accept that the field-level access achieves the same purpose. A single test calling each method suffices.

### Pitfall 5: otelMutationHook Testing Requires ent Client With Hooks Enabled
**What goes wrong:** Testing the hook in isolation doesn't prove it works in the ent pipeline
**Why it happens:** The hook is returned by schema methods and wired into the ent client by the framework
**How to avoid:** Use `testutil.SetupClient(t)` which creates a real ent client with hooks enabled. Force a mutation error (e.g., duplicate primary key) and verify error propagates. Use an in-memory OTel span exporter to verify span.RecordError was called.

## Code Examples

### Testing otelMutationHook Error Path (SCHEMA-01)

```go
func TestOtelMutationHook_ErrorPath(t *testing.T) {
    // Set up in-memory span exporter
    exporter := tracetest.NewInMemoryExporter()
    tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
    otel.SetTracerProvider(tp)
    t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

    client := testutil.SetupClient(t)
    ctx := context.Background()

    // Create an org to establish the ID
    _, err := client.Organization.Create().
        SetID(1).SetName("Test").
        SetCreated(time.Now()).SetUpdated(time.Now()).
        Save(ctx)
    if err != nil {
        t.Fatalf("first create: %v", err)
    }

    // Attempt duplicate ID -- triggers mutation error through hook
    _, err = client.Organization.Create().
        SetID(1).SetName("Dupe").
        SetCreated(time.Now()).SetUpdated(time.Now()).
        Save(ctx)
    if err == nil {
        t.Fatal("expected error on duplicate ID")
    }

    // Verify the hook recorded the error on the span
    spans := exporter.GetSpans()
    var found bool
    for _, s := range spans {
        if s.Name == "ent.Organization.Create" {
            for _, evt := range s.Events {
                if evt.Name == "exception" {
                    found = true
                }
            }
        }
    }
    if !found {
        t.Error("expected span with RecordError event")
    }
}
```

### Testing FK Constraint Violations (SCHEMA-02)

```go
func TestFKConstraintViolation(t *testing.T) {
    t.Parallel()
    client := testutil.SetupClient(t)
    ctx := context.Background()

    // Create Network referencing non-existent org_id
    _, err := client.Network.Create().
        SetID(1).SetName("Test").SetAsn(65000).
        SetOrgID(99999). // Does not exist
        SetOrganization(/* can't set edge to non-existent org */).
        SetCreated(time.Now()).SetUpdated(time.Now()).
        Save(ctx)
    // With foreign_keys(1), this should fail
    if err == nil {
        t.Fatal("expected FK constraint error")
    }
}
```

### Testing Schema Edges/Indexes/Annotations (SCHEMA-03)

```go
func TestSchemaEdges(t *testing.T) {
    t.Parallel()
    tests := []struct {
        name  string
        edges func() []ent.Edge
    }{
        {"Organization", Organization{}.Edges},
        {"Network", Network{}.Edges},
        // ... all 13 types
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            edges := tt.edges()
            if len(edges) == 0 {
                t.Error("expected at least one edge")
            }
        })
    }
}
```

### Testing Health checkSync Unknown Status (MINOR-02)

```go
{
    name: "unknown sync status",
    setupDB: func(t *testing.T) *sql.DB {
        db := openTestDB(t)
        initSyncTable(t, db)
        insertSync(t, db, time.Now().Add(-1*time.Hour), "bogus_status", "")
        return db
    },
    wantHTTPStatus: http.StatusServiceUnavailable,
    wantStatus:     "not_ready",
    wantDBStatus:   "ok",
    wantSyncStatus: "failed",
    wantSyncMsg:    "unknown sync status",
},
```

### Testing parseMeta Edge Cases (MINOR-03)

```go
func TestParseMeta_EdgeCases(t *testing.T) {
    tests := []struct {
        name string
        raw  json.RawMessage
        want time.Time
    }{
        {"nil", nil, time.Time{}},
        {"empty", json.RawMessage{}, time.Time{}},
        {"empty object", json.RawMessage(`{}`), time.Time{}},
        {"invalid json", json.RawMessage(`{invalid`), time.Time{}},
        {"generated zero", json.RawMessage(`{"generated":0}`), time.Time{}},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := parseMeta(tt.raw)
            if !got.Equal(tt.want) {
                t.Errorf("parseMeta = %v, want %v", got, tt.want)
            }
        })
    }
}
```

## Coverage Gap Analysis

### ent/schema: 47.4% -> 65%+ Target

| Category | Functions | Statements (est) | Strategy |
|----------|-----------|-------------------|----------|
| Edges (13x) | 0% each | ~39 stmts | Direct-call test |
| Indexes (13x) | 0% each | ~39 stmts | Direct-call test |
| Annotations (13x) | 0% each | ~39 stmts | Direct-call test |
| otelMutationHook error | 90% | ~1 stmt | Duplicate ID test |
| **Total coverable** | **40 functions** | **~118 stmts** | |

Covering all Edges/Indexes/Annotations (39 functions) plus the hook error path should push from 47.4% to well above 65%.

### internal/otel: 84.0% -> 90%+ Target

| Gap | Function | Strategy |
|-----|----------|----------|
| Setup span exporter error | Setup | Set OTEL_TRACES_EXPORTER to invalid value |
| Setup metric reader error | Setup | Set OTEL_METRICS_EXPORTER to invalid value |
| Setup log exporter error | Setup | Set OTEL_LOGS_EXPORTER to invalid value |
| InitFreshnessGauge no-sync | InitFreshnessGauge | lastSyncFn returns false |
| buildResource no VCS | buildResource | Already tested; remaining branch is edge case |

### internal/health: 84.6% -> 90%+ Target

| Gap | Function | Strategy |
|-----|----------|----------|
| Running with no completed | checkSync | Insert only "running" row |
| Unknown status | checkSync | Insert row with status "bogus" |
| GetLastStatus error | checkSync | Use DB with missing sync_status table |
| getLastCompletedSync no rows | getLastCompletedSync | Query when only running rows exist |

### internal/peeringdb: 83.2% -> 90%+ Target

| Gap | Function | Strategy |
|-----|----------|----------|
| parseMeta nil/empty/invalid | parseMeta | Direct unit tests with edge case inputs |
| FetchAll decode error | FetchAll | Server returns invalid JSON body |
| FetchAll read error | FetchAll | Server sends partial body then closes |
| FetchType unmarshal error | FetchType | Server returns `[{"invalid":true}]` with strict type |
| SetRateLimit | SetRateLimit | Call and verify limiter changed |
| SetRetryBaseDelay | SetRetryBaseDelay | Call and verify delay changed |

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` (Go 1.26) |
| Config file | None (stdlib) |
| Quick run command | `go test -race -cover ./ent/schema/ ./internal/otel/ ./internal/health/ ./internal/peeringdb/` |
| Full suite command | `go test -race -cover ./...` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SCHEMA-01 | otelMutationHook error path | integration | `go test -race -run TestOtelMutationHook ./ent/schema/ -count=1` | No -- Wave 0 |
| SCHEMA-02 | FK constraint edge cases | integration | `go test -race -run TestFKConstraint ./ent/schema/ -count=1` | Partially (TestNullableFK exists for nil FKs) |
| SCHEMA-03 | ent/schema 65%+ coverage | coverage | `go test -race -cover ./ent/schema/ -count=1` | No -- Wave 0 |
| MINOR-01 | internal/otel 90%+ | coverage | `go test -race -cover ./internal/otel/ -count=1` | Partial (84% baseline) |
| MINOR-02 | internal/health 90%+ | coverage | `go test -race -cover ./internal/health/ -count=1` | Partial (84.6% baseline) |
| MINOR-03 | internal/peeringdb 90%+ | coverage | `go test -race -cover ./internal/peeringdb/ -count=1` | Partial (83.2% baseline) |

### Sampling Rate
- **Per task commit:** `go test -race -cover ./ent/schema/ ./internal/otel/ ./internal/health/ ./internal/peeringdb/`
- **Per wave merge:** `go test -race ./...`
- **Phase gate:** All four packages meet coverage targets in `-cover` output

### Wave 0 Gaps
- None -- existing test infrastructure fully covers all phase requirements. testutil.SetupClient, openTestDB helpers, OTel test providers all established.

## Sources

### Primary (HIGH confidence)
- Direct source code analysis of all 4 target packages
- `go test -race -cover` and `go tool cover -func` output from live codebase
- Existing test files in each package

### Secondary (MEDIUM confidence)
- OTel SDK testing patterns from existing client_test.go (tracetest.InMemoryExporter)
- enttest behavior observed from testutil.SetupClient implementation

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all packages use stdlib testing, no new dependencies needed
- Architecture: HIGH -- patterns established in prior phases (37-40), all test helpers exist
- Pitfalls: HIGH -- verified through actual coverage analysis, not hypothetical
- Coverage math: HIGH -- measured baselines against actual code, gap analysis is precise

**Research date:** 2026-03-26
**Valid until:** 2026-04-26 (stable -- test code, no external dependency changes expected)
