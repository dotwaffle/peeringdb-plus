# Feature Landscape

**Domain:** Test coverage and quality improvement for a Go application with 5 API surfaces
**Researched:** 2026-03-26
**Milestone:** v1.10

## Table Stakes

Features that a robust test suite for this project must have. Missing = false confidence in coverage metrics.

### Critical Coverage Gaps (Must Fix)

| Feature | Why Expected | Complexity | Existing Infrastructure | Current -> Target |
|---------|--------------|------------|------------------------|-------------------|
| **GraphQL offset/limit list resolver tests** | 11 of 13 `*List` resolvers in `custom.resolvers.go` at 0% coverage. Only `networksList` is partially covered (50%). These are hand-written resolvers with real validation and filter logic -- `ValidateOffsetLimit` + `WhereInput.P()` error handling in each. | Low | `graph/resolver_test.go` has `seedTestData`, `postGraphQL`, `gqlResponse` helpers. Pattern: add one GraphQL query per list type to hit the resolver. | graph: 2.6% -> 50%+ |
| **GraphQL custom resolver error paths** | `NetworkByAsn` at 50% (only happy path). `SyncStatus` at 70% (nil status untested). `ObjectCounts` resolver at 0%. `SyncStatus` factory method at 0%. Error branch in `ValidateOffsetLimit` partially tested only through `networksList`. | Low | Same helpers. Need: query for nonexistent ASN (returns null, not error), query SyncStatus with no sync data, query ObjectCounts sub-resolver. | graph: +10% |
| **gRPC streaming tests for 4 missing types** | `StreamCarrierFacilities`, `StreamIxPrefixes`, `StreamNetworkIxLans`, `StreamPocs` have 0% coverage. Their corresponding `apply*StreamFilters` functions also at 0%. The other 9 types all have stream tests. | Med | `grpcserver_test.go` has 43 tests. Streaming pattern is established: create entities, call `Stream*` directly on service struct, collect results. Copy pattern for 4 remaining types. | grpcserver: 61.7% -> 70%+ |
| **gRPC filter branch coverage** | Every `apply*ListFilters` and `apply*StreamFilters` function has uncovered branches from optional proto field nil-checks. Coverage ranges from 43.8% (`applyNetworkIxLanListFilters`) to 63.6% (`applyIxFacilityListFilters`). Each filter function has 5-20 conditional branches, most untested. | Med | Filter tests exist for 7 entity types but test only 1-2 filter fields each. Need table-driven tests with each filter field populated individually. ~13 types x ~8 avg filter fields = ~104 filter branches. | grpcserver: +10-15% |
| **Web detail fragment handler coverage** | 6 lazy-loaded fragment handlers at 58-69%: `handleNetIXLansFragment` (60%), `handleNetContactsFragment` (60%), `handleIXParticipantsFragment` (66.7%), `handleIXFacilitiesFragment` (58.3%), `handleIXPrefixesFragment` (60%), `handleFacCarriersFragment` (uncounted). Untested branches are entity-not-found errors and empty-relationship paths. | Med | `detail_test.go` (515 lines) tests full detail pages. Fragment tests need: `HX-Request: true` header, seeded relationship data (e.g., NetworkIxLan records for network IX fragments), and error cases (invalid IDs). | web: 74.8% -> 82%+ |
| **Web `renderPage` multi-mode dispatch** | `renderPage` at 41.8%. Only HTML and htmx paths exercised by existing tests. Terminal mode branches (rich/plain/JSON/WHOIS/short) plus error-title branches (Not Found, Server Error) are all untested through `renderPage` despite termrender itself being tested. | Med | `handler_test.go` has `newTestMux`. Need requests with `User-Agent: curl/8.0` to exercise terminal paths, `?T` for plain, `?format=json` for JSON, `?format=whois` for WHOIS. All through existing handler endpoints. | web: +5-8% |

### Moderate Coverage Gaps (Should Fix)

| Feature | Why Expected | Complexity | Current -> Target |
|---------|--------------|------------|-------------------|
| **internal/otel `InitMetrics` error paths** | `InitMetrics` at 70%, `Setup` at 80%, `InitFreshnessGauge` at 80%. Error branches when OTel meter/provider creation fails. | Low | otel: 84% -> 92%+ |
| **internal/health `checkSync` edge cases** | `checkSync` at 65.2%. Needs tests for: sync age exactly at threshold, nil timestamp, SQL error during query. | Low | health: 84.6% -> 92%+ |
| **internal/peeringdb `doWithRetry` exhaustion** | `doWithRetry` at 84%. Missing: max-retry-exceeded path, non-retryable error (401/403) path. `SetRateLimit` and `SetRetryBaseDelay` at 0% (trivial setters). `FetchAll` at 80.8%. | Med | peeringdb: 83.2% -> 90%+ |
| **internal/sync `deleteStaleChunked` error path** | `deleteStaleChunked` at 66.7%. The error path during chunk deletion is untested. Status table functions (`InitStatusTable` 71.4%, `RecordSyncStart` 71.4%) have untested SQL error branches. | Low | sync: 86.9% -> 92%+ |
| **internal/web/termrender minor gaps** | `String()` method on RenderMode at 22.2% (only default case tested). `formatLocation` at 60%. `RenderJSON` at 70%. `RenderOrgDetail` at 72.5%. | Low | termrender: 88.2% -> 93%+ |
| **Web `extractID` and `getFreshness`** | `extractID` at 37.5% (partial match, error paths). `getFreshness` at 50% (nil DB path). `handleAbout` at 40% (terminal path untested). | Low | web: +2-3% |

### Coverage Metrics Hygiene

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| **Exclude generated code from coverage metrics** | `graph/generated.go` (57,144 lines) dominates the `graph` package coverage denominator. All `ent/*` generated packages, `gen/peeringdb/v1/*`, `ent/rest/*` show 0% but are code-generated and untestable. Including them inflates the denominator and makes real coverage gains invisible. | Low | Use `-coverpkg` flag listing only hand-written packages in CI. Or filter `go tool cover -func` output to exclude `/generated.go`, `/ent/`, `/gen/`. |
| **Per-package coverage tracking in CI** | Current CI reports overall coverage. Per-package breakdown reveals which hand-written packages are below threshold. | Low | `go test -coverprofile` + `go tool cover -func` per package. Report packages below 80% as CI annotations. |

## Differentiators

Features beyond basic line coverage that make tests genuinely trustworthy.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| **Test quality audit: assertion density** | Some existing tests may only assert `rec.Code == 200` without checking response bodies. A handler could return empty HTML with 200 and the test would pass. Audit each test function and flag those with status-only assertions. | Med | Review task, not code. Walk each `_test.go` file. Estimated: identify 5-10 weak tests. Fix by adding body content assertions. |
| **Error path coverage audit** | Systematically map every `fmt.Errorf` and `connect.NewError` in hand-written code to a test that exercises it. `graph` has 14 error returns, only 2 tested. `grpcserver` has 26 `connect.NewError` calls -- 13 "not found" tested, filter validation errors partially. | Med | Use `go tool cover -func` sub-100% functions, read uncovered lines, verify they are error returns. Produces a checklist of specific error paths to test. |
| **Cross-surface consistency tests** | Seed one entity, query it via GraphQL, gRPC, REST, and PeeringDB compat. Verify all 4 surfaces return equivalent data. Catches field mapping bugs that single-surface tests miss (e.g., a field named `info_type` in REST but `infoType` in GraphQL with different values). | High | Requires standing up all 4 API surfaces in a single test. New shared `setupFullServer` helper needed. Very high value for a project with 5 API surfaces -- unique to this codebase. |
| **Golden file tests for gRPC responses** | Mirror the existing `pdbcompat` golden file pattern (39 golden files for 13 types x 3 scenarios) for gRPC Get/List responses. Catches unintended proto field mapping changes. | High | Serialize proto messages via `protojson.Marshal` to deterministic JSON. 13 types x 2 RPCs = 26 golden files. Pattern well-established in `pdbcompat/golden_test.go`. |
| **Fuzz tests for untrusted inputs** | PeeringDB API responses contain user-generated content. The Django-style filter parser (`pdbcompat/filter.go`) and JSON deserializers (`peeringdb/types.go`) accept untrusted input. Per SEC-4 (CLAUDE.md). | Med | `testing.F` is stdlib. Focus on filter parser (complex string parsing with `__` operators) and custom JSON unmarshalers (handle PeeringDB spec-vs-reality discrepancies). |
| **Streaming RPC integration test** | Current stream tests call service methods directly. A full-stack test via `httptest.Server` with ConnectRPC client would validate wire format, `grpc-total-count` header propagation, and otelconnect interceptor. | High | Tests the actual HTTP transport layer, not just the Go method. Catches issues like missing Flusher implementation in middleware wrappers. |

## Anti-Features

Features to explicitly NOT build.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| **Testing generated code** | `graph/generated.go` (57K lines), all `ent/*` generated packages, `gen/peeringdb/v1/` are code-generated. Covering them inflates metrics without catching bugs. Schema changes regenerate them. | Exclude from coverage metrics. Focus on hand-written code in `graph/custom.resolvers.go`, `graph/schema.resolvers.go`, `graph/pagination.go`, `graph/resolver.go`. |
| **Unit testing ent/schema Edges/Indexes/Annotations** | All 13 schema types have 0% on `Edges()`, `Indexes()`, `Annotations()`. These return static configuration structs consumed by ent codegen, not application logic. If wrong, codegen or migration fails -- caught by integration tests already. | Accept ~50% as reasonable for `ent/schema`. The hand-written logic (hooks in `hooks.go`, types in `types.go`, validation in Organization model) matters. Schema config methods do not. |
| **Mock-heavy unit tests for thin wrappers** | The 13 cursor-based resolvers in `schema.resolvers.go` are 4-line functions: validate page size, call ent Paginate. Mocking the ent client to test these in isolation adds complexity for zero bug-finding value. | Test through integration: real ent client with SQLite, real GraphQL queries. This is the established pattern in `resolver_test.go`. |
| **100% coverage target** | Leads to testing trivial getters, generated code, and impossible error paths. Go standard library averages ~80%. Diminishing returns above 90% for hand-written code. | Target 80%+ on hand-written packages. Accept lower numbers where generated code dominates the denominator. |
| **Mutation testing in CI** | `go-mutesting` and `ooze` are viable but slow (10-100x test runtime) and noisy. Better as periodic audit, not CI gate. | Run mutation testing once manually after coverage improvements complete. Use to identify tests that execute code without asserting on results. |
| **Property-based testing** | `pgregory.net/rapid` is powerful but the codebase has straightforward data transformations, not complex algorithms. ROI is low compared to closing basic coverage gaps. | Consider later for filter parser or JSON deserializers if those prove buggy in production. |
| **Separate test binaries with `go build -cover`** | Go 1.20+ supports coverage for integration test binaries. Overkill for this project -- all tests run via `go test`, not external binary invocation. | Standard `go test -coverprofile` is sufficient. |

## Feature Dependencies

```
GraphQL list resolver tests -----> testutil.SetupClientWithDB (exists)
                             |---> seedTestData helper (exists in resolver_test.go)
                             |---> postGraphQL helper (exists)
                             `---> pdbsync.InitStatusTable (for SyncStatus tests, exists)

gRPC stream filter tests -------> testutil.SetupClient (exists)
                             |---> entity seeding patterns (established in grpcserver_test.go)
                             |---> ConnectRPC streaming test pattern (established for 9/13 types)
                             `---> proto filter field population (manual per-type)

gRPC filter branch tests -------> same as stream tests
                             `---> table-driven per filter field (new, but pattern is mechanical)

Web fragment tests -------------> newTestMux (exists)
                             |---> entity seeding WITH relationships (partial -- need expansion)
                             |---> HX-Request header in test requests (trivial)
                             `---> seeded NetworkIxLan, IxFacility, etc. for relationship data

Web renderPage mode tests ------> newTestMux (exists)
                             |---> User-Agent strings: "curl/8.0", "wget/1.21"
                             |---> Query params: "?T", "?format=json", "?format=whois"
                             `---> seeded entity for data rendering (exists in detail tests)

Cross-surface consistency ------> ALL surface handlers in one test server (NEW helper)
                             |---> shared entity seeding
                             `---> response comparison logic
```

## MVP Recommendation

Prioritize by coverage delta per effort:

1. **GraphQL offset/limit list resolvers** -- 11 resolvers at 0%, each testable with a single GraphQL query using existing helpers. Moves `graph` package from 2.6% to ~40%+ with ~80 lines of test code.
2. **GraphQL custom resolver edge cases** -- `NetworkByAsn` nonexistent ASN, `SyncStatus` with no data, `ObjectCounts` resolver. Another ~15% on `graph` package.
3. **gRPC streaming for 4 missing types** -- `CarrierFacility`, `IxPrefix`, `NetworkIxLan`, `Poc`. Copy-paste from established stream test pattern. ~120 lines.
4. **gRPC filter branch coverage** -- Table-driven tests with each filter field populated. Repetitive but high coverage delta. ~200 lines for all 13 types.
5. **Web fragment handlers** -- Seed relationship data, send HX-Request, verify fragment HTML. ~100 lines.
6. **Web renderPage terminal modes** -- Add User-Agent/query-param variations to existing handler tests. ~60 lines.
7. **Remaining minor package gaps** (otel, health, peeringdb, sync) -- Targeted tests for specific uncovered functions. ~100 lines total.
8. **Coverage metrics hygiene** -- Exclude generated code from CI coverage tracking.

Defer:
- **Cross-surface consistency tests**: High value but high complexity. Do after individual surfaces hit 80%+.
- **Golden files for gRPC**: Valuable for regression prevention but better after filter coverage is complete.
- **Fuzz tests**: Good for hardening, separate effort.
- **Test quality audit**: Do after coverage numbers are raised -- easier to audit fewer weak tests.

## Effort Estimates

| Category | Packages Affected | Estimated Test Code | Coverage Impact |
|----------|-------------------|--------------------|-----------------|
| GraphQL list resolvers + edge cases | graph | ~150 lines | 2.6% -> 60%+ (hand-written code) |
| gRPC streaming + filters | internal/grpcserver | ~350 lines | 61.7% -> 82%+ |
| Web fragments + renderPage | internal/web | ~200 lines | 74.8% -> 85%+ |
| Minor package gaps | otel, health, peeringdb, sync | ~150 lines | Each to 90%+ |
| termrender minor gaps | internal/web/termrender | ~50 lines | 88.2% -> 93%+ |
| ent/schema hooks edge case | ent/schema | ~20 lines | 47.4% -> 50% (ceiling due to static config) |
| Coverage metrics config | CI / Taskfile | ~10 lines config | Accurate reporting |
| **Total** | | **~930 lines of test code** | **All hand-written packages at 80%+** |

## Sources

- [Go integration test coverage (go.dev)](https://go.dev/blog/integration-test-coverage) -- Coverage profiling for integration tests
- [Go build coverage documentation (go.dev)](https://go.dev/doc/build-cover) -- `go build -cover` and `-coverpkg` usage
- [Go coverage tracking best practices (OtterWise)](https://getotterwise.com/blog/go-code-coverage-tracking-best-practices-cicd) -- CI integration patterns
- [gqlgen resolver reference](https://gqlgen.com/reference/resolvers/) -- Resolver testing approaches
- [go-mutesting](https://github.com/zimmski/go-mutesting) -- Go mutation testing framework
- [pgregory.net/rapid](https://pkg.go.dev/pgregory.net/rapid) -- Property-based testing for Go
- [Go unit testing structure 2025](https://www.glukhov.org/post/2025/11/unit-tests-in-go/) -- Structural patterns
- Project coverage data from `go test -coverprofile` and `go tool cover -func` (run 2026-03-26)
