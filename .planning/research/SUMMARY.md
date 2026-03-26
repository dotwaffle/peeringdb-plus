# Project Research Summary

**Project:** PeeringDB Plus -- v1.10 Code Coverage & Test Quality
**Domain:** Test coverage improvement for a Go application with 5 API surfaces
**Researched:** 2026-03-26
**Confidence:** HIGH

## Executive Summary

This milestone is about raising test coverage and quality across a mature Go codebase with 5 API surfaces (GraphQL, gRPC/ConnectRPC, REST, PeeringDB compat, Web UI). The project has 60 test files but overall coverage reads 5.7% -- a misleading number caused by ~120K lines of generated code (ent, gqlgen, protobuf, templ) inflating the denominator. Hand-written code coverage is approximately 60-65%, with critical gaps in the GraphQL resolvers (2.6% package-level, ~0% on the 13 List resolvers), gRPC filter/streaming branches (61.7%), and web handler fragment/terminal-mode paths (74.8%). The remaining packages (sync, pdbcompat, middleware, config, health, otel) are all at 83-98% and need only targeted gap-filling.

The recommended approach is infrastructure-first: build a shared test data seeding package (`internal/testutil/seed/`) to eliminate the 6+ duplicate entity creation sites across packages, then systematically expand tests per-surface in dependency order. No new external dependencies are needed -- stdlib `testing`, `httptest`, `enttest`, and `go tool cover` are sufficient. The entire effort is estimated at ~930 lines of new test code to bring all hand-written packages above 80%.

The key risk is coverage-padding: writing tests that execute code paths but assert nothing meaningful, producing false confidence. The graph package is especially dangerous -- naive coverage targets applied to a package where generated code is 99% of the source will either produce unreachable goals or incentivize testing gqlgen's internals. All coverage targets must be stated in terms of hand-written files, not package-level percentages. The second risk is boilerplate explosion in the gRPC tests (13 near-identical entity types), mitigated by extending the existing generic test helper pattern rather than copy-pasting per-type tests.

## Key Findings

### Recommended Stack

No new dependencies are needed. The existing test infrastructure is sufficient for all coverage goals. See [STACK.md](STACK.md) for full details.

**Core technologies (all already in use):**
- `testing` (stdlib): Table-driven tests, `-race` flag, `b.Loop()` benchmarks
- `entgo.io/ent/enttest`: In-memory SQLite test databases with auto-migration
- `net/http/httptest`: HTTP test server/recorder for API surface testing
- `go tool cover` / `go tool covdata`: Coverage profiling and per-function analysis with generated code filtering

**What NOT to add:** testify/assert (inconsistent with established stdlib assertion pattern), gomock (real SQLite via enttest is fast enough), mutation testing tools (expensive, run manually after coverage work completes), coverage badge services (generated code distorts numbers).

### Expected Features

See [FEATURES.md](FEATURES.md) for full analysis with effort estimates.

**Must have (table stakes):**
- GraphQL offset/limit list resolver tests -- 11 of 13 resolvers at 0%, biggest coverage gap
- GraphQL custom resolver error paths -- NetworkByAsn, SyncStatus, ObjectCounts untested branches
- gRPC streaming tests for 4 missing types -- CarrierFacility, IxPrefix, NetworkIxLan, Poc
- gRPC filter branch coverage -- ~104 uncovered filter branches across 13 types
- Web detail fragment handler tests -- 6 lazy-loaded fragments at 58-69%
- Web `renderPage` multi-mode dispatch tests -- terminal/JSON/WHOIS paths at 0%
- Coverage metrics hygiene -- exclude generated code from CI reporting

**Should have (differentiators):**
- Test quality audit: assertion density review across existing tests
- Error path coverage audit: map every `fmt.Errorf` and `connect.NewError` to a test
- Cross-surface consistency tests: verify same data across all 4 API surfaces
- Fuzz tests for untrusted inputs (filter parser, JSON deserializers)

**Defer:**
- Cross-surface consistency tests (high complexity, do after individual surfaces hit 80%)
- Golden files for gRPC responses (do after filter coverage is complete)
- Property-based testing (low ROI for straightforward data transformations)
- Mutation testing in CI (run manually once after coverage work completes)

### Architecture Approach

The test architecture centers on a shared seed package that provides deterministic test data for all 13 PeeringDB entity types, eliminating the current duplication of 6+ independent entity creation implementations. Each package retains its own server/handler setup but delegates entity creation to the shared infrastructure. See [ARCHITECTURE.md](ARCHITECTURE.md) for full component boundaries and data flow.

**Major components:**
1. `internal/testutil/seed/` (new) -- Deterministic entity factory with `Full()`, `Minimal()`, `Networks()`, `WithSyncStatus()` functions returning a `Result` struct with all entity references
2. `internal/testutil/` (existing, unchanged) -- Client setup with in-memory SQLite, atomic counter for unique DB names
3. Package-local test helpers (existing) -- HTTP server setup, response parsing, package-specific assertions per surface

**Key architectural patterns to follow:**
- Table-driven tests with shared setup (established in grpcserver)
- Golden file tests for serialization-sensitive surfaces (established in pdbcompat -- do NOT proliferate)
- HTTP-level integration for API surfaces, white-box for adapter layers
- Direct function calls for resolver testing (avoid full HTTP stack overhead)

### Critical Pitfalls

See [PITFALLS.md](PITFALLS.md) for all 12 pitfalls with detailed prevention strategies.

1. **Coverage-padding tests** -- Tests that execute paths but assert only `err == nil` or `status == 200`. Every test must assert at least one behavioral property. If removing the function under test would not cause the test to fail, the test is worthless.
2. **Testing generated code** -- The 57K-line `generated.go` and all `ent/`/`gen/` packages are untestable noise. Exclude from targets. Focus on the ~550 lines of hand-written resolvers, ~3,344 lines of grpcserver handlers.
3. **Boilerplate explosion for 13 entity types** -- Copy-pasting tests per entity type creates a 6,000+ line maintenance nightmare. Extend the existing `generic_test.go` pattern with type-parameterized helpers.
4. **SQLite contention in parallel tests** -- Each `t.Parallel()` subtest that writes must use its own `testutil.SetupClient(t)`. Subtests sharing parent data must be read-only. Run `-race -count=10` to detect.
5. **Graph package denominator distortion** -- Even 100% hand-written resolver coverage yields only ~15-20% package-level coverage. Targets must be stated per-file (`custom.resolvers.go`, `schema.resolvers.go`, `pagination.go`), not per-package.

## Implications for Roadmap

Based on research, suggested phase structure:

### Phase 1: Shared Test Seed Package
**Rationale:** All subsequent phases depend on shared test data infrastructure. Building this first eliminates duplication and provides a stable foundation.
**Delivers:** `internal/testutil/seed/` package with `Full()`, `Minimal()`, `Networks()`, `WithSyncStatus()` functions.
**Addresses:** Data seeding duplication (6+ independent implementations -> 1 shared implementation)
**Avoids:** Pitfall 3 (boilerplate explosion) by establishing reusable factories before test expansion begins
**Estimated effort:** Small -- extract and generalize from existing `setupGoldenTestData()` in pdbcompat

### Phase 2: GraphQL Resolver Coverage
**Rationale:** Highest coverage delta per effort. 11 resolvers at 0% can be covered with ~150 lines of test code using existing `postGraphQL` helper.
**Delivers:** Tests for all 13 `*List` offset/limit resolvers, error paths for NetworkByAsn/SyncStatus/ObjectCounts/validatePageSize
**Addresses:** graph package from 2.6% to 60%+ (hand-written code)
**Avoids:** Pitfall 2 (do NOT test generated.go), Pitfall 8 (measure by file, not package), Pitfall 1 (assert response data, not just status)

### Phase 3: gRPC Handler Coverage
**Rationale:** Second-highest coverage gap. Can run in parallel with Phase 2 (different packages). Extend existing generic_test.go pattern.
**Delivers:** Missing filter tests (6 entity types), missing stream tests (4 entity types), filter branch coverage for all 13 types
**Addresses:** grpcserver from 61.7% to 82%+
**Avoids:** Pitfall 3 (use generic helpers, not 13 copies), Pitfall 4 (isolated DBs per write-heavy subtest), Pitfall 9 (nil-check proto wrappers)

### Phase 4: Web Handler Coverage
**Rationale:** Depends on Phase 1 (seed package) for data setup. Third-highest gap.
**Delivers:** Fragment handler tests (6 fragments), renderPage terminal/JSON/WHOIS mode tests, extractID/getFreshness edge cases
**Addresses:** web from 74.8% to 85%+
**Avoids:** Pitfall 5 (use httptest.NewRecorder, not full server), Pitfall 6 (targeted assertions, not golden files for every mode)

### Phase 5: Schema Hook Coverage
**Rationale:** Independent of Phases 2-4. Small, focused effort.
**Delivers:** otelMutationHook error path test, relationship constraint tests
**Addresses:** ent/schema from 47.4% to 65-70% (realistic ceiling given static config methods)
**Avoids:** Pitfall 2 (do NOT test Edges/Indexes/Annotations -- they are configuration, not behavior)

### Phase 6: Remaining Package Gaps + Coverage Hygiene
**Rationale:** Clean-up phase. All packages at 83%+ already; this brings them to 90%+. Also implements CI coverage filtering.
**Delivers:** Targeted error path tests for otel, health, peeringdb, sync. Coverage exclusion config for CI.
**Addresses:** All hand-written packages at 80%+. Accurate CI coverage reporting.
**Avoids:** Pitfall 7 (do NOT mock OTel internals -- test that functions do not error, not that spans have correct names)

### Phase Ordering Rationale

- Phase 1 first because Phases 2-4 all benefit from shared seed data. Without it, each phase recreates test data independently (the exact duplication problem being fixed).
- Phases 2 and 3 can run in parallel -- they touch different packages (`graph/` vs `internal/grpcserver/`) with no overlap.
- Phase 4 after Phase 1 because web tests need relationship data (NetworkIxLan, IxFacility, etc.) that `seed.Full()` provides.
- Phase 5 is independent and small -- can slot anywhere after Phase 1 or run in parallel with 2-4.
- Phase 6 last because it is clean-up work and depends on all other phases being complete to set accurate coverage thresholds.

### Research Flags

Phases with standard patterns (skip research-phase):
- **Phase 1:** Well-established pattern in existing `setupGoldenTestData()`. Mechanical extraction.
- **Phase 2:** Existing `postGraphQL` helper and `resolver_test.go` pattern. Just needs replication.
- **Phase 3:** Existing `generic_test.go` and streaming test patterns. Mechanical extension.
- **Phase 4:** Existing `handler_test.go` and `detail_test.go` patterns. Add test cases.
- **Phase 5:** Existing `schema_test.go` pattern. Small scope.
- **Phase 6:** Targeted gap-filling in well-understood packages.

No phases need deeper research. All patterns are established in the existing codebase and need only extension, not invention.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | No new dependencies needed. All tools already in use and validated. |
| Features | HIGH | Based on direct codebase analysis with per-function coverage profiling. Exact line counts and coverage percentages from `go tool cover`. |
| Architecture | HIGH | Derived from analysis of 60 existing test files and 15 packages. Recommended patterns are generalizations of the best existing patterns, not new inventions. |
| Pitfalls | HIGH | Based on direct observation of current test code (6+ duplicate seed functions, 57K-line generated code denominator, 2,920-line grpcserver test file). Pitfalls are concrete, not theoretical. |

**Overall confidence:** HIGH

All research is based on direct codebase analysis and coverage profiling, not external documentation or community patterns. The recommendations extend patterns already proven in the codebase.

### Gaps to Address

- **Graph package coverage measurement:** Need to validate that `go tool cover -func` filtering by filename accurately measures hand-written resolver coverage. The 2.6% -> 60%+ target assumes generated.go exclusion works cleanly. Validate in Phase 2 execution.
- **Generic test helper feasibility for gRPC:** The existing `generic_test.go` uses type parameters. Need to verify that the approach scales to stream tests and filter tests during Phase 3 planning. If generics prove unwieldy for the filter variations, fall back to table-driven tests per type (still better than copy-paste).
- **SQLite parallel test performance:** Adding ~930 lines of tests with many new `testutil.SetupClient()` calls will increase test suite runtime. Monitor CI time after Phase 3. If it exceeds acceptable limits, consider reducing parallelism or sharing read-only databases across subtests.

## Sources

### Primary (HIGH confidence)
- Direct codebase coverage analysis via `go test -coverprofile` and `go tool cover -func` (2026-03-26)
- Existing test file examination: 60 test files, ~8,500 lines hand-written test code
- [Go testing documentation](https://go.dev/doc/test)
- [enttest documentation](https://entgo.io/docs/testing/)

### Secondary (MEDIUM confidence)
- [Go integration test coverage](https://go.dev/blog/integration-test-coverage) -- Coverage profiling patterns
- [Go build coverage documentation](https://go.dev/doc/build-cover) -- `-coverpkg` usage
- [Learn Go with Tests - Anti-patterns](https://quii.gitbook.io/learn-go-with-tests/meta/anti-patterns) -- Testing anti-patterns
- [Martin Fowler - Test Coverage](https://martinfowler.com/bliki/TestCoverage.html) -- Coverage metrics limitations
- [Unit Testing ConnectRPC Servers](https://kmcd.dev/posts/connectrpc-unittests/) -- ConnectRPC testing patterns
- [Parallel Table-Driven Tests in Go](https://www.glukhov.org/post/2025/12/parallel-table-driven-tests-in-go/) -- Parallel test pitfalls

### Tertiary (LOW confidence)
- None -- all findings are based on direct analysis or well-established sources

---
*Research completed: 2026-03-26*
*Ready for roadmap: yes*
