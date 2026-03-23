# Feature Landscape

**Domain:** Golden file testing, CI pipeline enforcement, and public access documentation for a Go API project
**Researched:** 2026-03-23
**Milestone context:** v1.2 Quality & CI -- adding golden file tests for PeeringDB compat layer, GitHub Actions CI, linter/test enforcement to an existing ~57K LOC Go codebase with 3 API surfaces
**Existing:** 13 PeeringDB type sync, GraphQL API, entrest REST API, PeeringDB compat REST layer with Django-style filters/depth expansion/field projection/search, OpenTelemetry, health endpoints, Fly.io deployment

## Table Stakes

Features users (contributors, operators, downstream consumers) expect. Missing = project feels incomplete or untrustworthy for production use.

| Feature | Why Expected | Complexity | Dependencies | Notes |
|---------|--------------|------------|--------------|-------|
| Golden file tests for compat layer list responses | Existing tests only check item counts and field presence, never the full JSON shape. Regressions in field names, types, ordering, or envelope structure would go undetected. Golden files make shape changes visible in code review diffs. | Med | Existing `pdbcompat` package, `testutil.SetupClient`, deterministic test fixtures | 13 PeeringDB types exist, but only 4 need golden files (net, org, fac, ix -- the most complex serializers). The remaining 9 types follow identical structural patterns and are already covered by `TestSerializerAllTypesCompile`. |
| Golden file tests for compat detail responses (depth=0) | Detail endpoint wraps a single object in a `[...]` array per PeeringDB convention (Pitfall 7 in existing tests). The full field set must match PeeringDB exactly. Golden files capture the complete shape. | Med | Same as list golden files | Detail uses the same serializer as list; golden files catch envelope differences (single-item array wrapping, meta field). |
| `-update` flag for golden file regeneration | Standard Go convention. Running `go test ./... -update` regenerates golden files when intentional changes occur. Without it, developers must manually craft JSON files, which is error-prone for objects with 30+ fields. | Low | `flag` stdlib package only | Use `var update = flag.Bool("update", false, "update golden files")` in test file. No external library needed -- stdlib `flag`, `os.WriteFile`, `os.ReadFile`, `bytes.Equal` are sufficient. This follows project CLAUDE.md MD-1 (prefer stdlib). |
| Golden files stored in `testdata/` | Go convention. The `go` tool ignores `testdata/` directories during builds. Keeps fixtures co-located with tests. Golden file changes show up clearly in `git diff` for code review. | Low | None | Use `internal/pdbcompat/testdata/golden/` for compat layer golden files. Naming convention: `{type}_{scenario}.golden.json` (e.g., `net_list.golden.json`, `org_detail.golden.json`). |
| Deterministic timestamps in golden file test fixtures | Existing handler tests use `time.Now()` which means golden files would differ on every run. Must use fixed `time.Date(...)` values in test data setup for golden file tests. | Low | Test fixture refactoring | Use `time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)` and related fixed dates. This is the standard Go approach -- deterministic test data, not regex scrubbing of dynamic values. |
| GitHub Actions CI workflow | Industry standard for Go open source. Without CI, PRs have no automated quality gate. Contributors have no feedback loop. | Low | GitHub repository access | Single `.github/workflows/ci.yml` file. Trigger on pull requests and pushes to main. |
| `go test -race ./...` in CI | Catches data races in concurrent code. Required by project CLAUDE.md (T-2, G-3). The sync worker, LiteFS detection, and OTel pipeline all use goroutines -- race detection is not optional. | Low | CI workflow | Run as a dedicated step for clear failure attribution. modernc.org/sqlite is pure Go, so `-race` works without CGo complications on Linux runners (CGO_ENABLED=1 by default). |
| `go vet ./...` in CI | Required by project CLAUDE.md (G-1). Catches common mistakes: printf format mismatches, unreachable code, shadowed variables, etc. | Low | CI workflow | Built into Go toolchain, zero configuration needed. golangci-lint v2 includes govet by default. |
| `golangci-lint` in CI | Required by project CLAUDE.md (G-2, TL-1). Aggregates 50+ linters including staticcheck, errcheck, govet, gosec. | Low | `.golangci.yml` config file, `golangci-lint-action@v9` | golangci-lint v2.11 with new config format. Config needs `version: "2"` at top. Must exclude generated code directories (`ent/`, `graph/generated.go`). |
| All existing tests pass before CI gates | CI must gate PRs on test passage. If existing tests or lints are broken, CI will never go green. Must fix existing issues as a prerequisite. | Med (unknown scope) | Run `golangci-lint run` and `go test -race ./...` locally first to assess | Could surface issues in generated code, unused parameters, missing error checks. Lint exclusions for generated code are the likely fix. |
| Public access model documentation | PROJECT.md lists "Fully public -- verify no auth barriers, document public access model" as an active v1.2 requirement. Users need to know: no API keys needed, no rate limits, no authentication, all data publicly accessible. | Low | None | Verify no middleware blocks unauthenticated requests. Document in README or a dedicated `docs/PUBLIC-ACCESS.md`. List all API endpoints and their access model. |

## Differentiators

Features that set the project apart. Not expected by default, but demonstrate quality and maturity.

| Feature | Value Proposition | Complexity | Dependencies | Notes |
|---------|-------------------|------------|--------------|-------|
| Golden files for depth-expanded responses (`?depth=2`) | PeeringDB compat layer supports `?depth=2` which inlines related objects (`net_set`, `fac_set`, `org` expansion). No other PeeringDB mirror validates this complex nesting via golden files. These are the highest-risk response shapes -- nested arrays of full objects. | High | List/detail golden files completed first; `setupDepthTestData` already exists in `depth_test.go` | Test org depth=2 (5 `_set` arrays), net depth=2 (`_set` arrays + expanded `org` object), netfac depth=2 (expanded FK edges). 3-4 golden files. Review carefully -- nesting depth affects all downstream consumers. |
| Golden files for field projection (`?fields=id,name`) | Validates that field subsetting returns exactly the requested fields and nothing else. Catches regressions where new fields leak into projected responses. | Med | List golden files completed first | Test `?fields=id,name` on net type. 1-2 golden files. Verify `_set` fields are preserved even when `fields` param is used (per existing `TestFieldProjectionWithDepth`). |
| Golden files for filter combinations | Validates that Django-style filters (`__contains`, `__gt`, `__in`, etc.) return correct subsets. Catches filter parser regressions. | Med | List golden files completed first | Test a few representative filters on `net` type. The filter parser is generic across all 13 types, so testing on one type validates the pattern. 2-3 golden files. |
| `cmp.Diff` for golden file mismatches | When golden files mismatch, test output shows a readable unified diff instead of raw JSON dumps. Dramatically improves debugging experience for large JSON responses (30+ fields). | Low | `google/go-cmp` v0.7.0 (already indirect dep, promote to direct) | Standard Go testing practice per Go wiki TestComments. |
| `govulncheck` in CI | Official Go vulnerability scanner. Catches known CVEs in dependencies. Recommended by project CLAUDE.md (TL-2, CI-4). | Low | CI workflow | Run `govulncheck ./...` directly. Can output SARIF format for GitHub Security tab integration. Run as informational (non-blocking) initially. |
| CI status badge in README | Visual indicator of build health at a glance. Standard for open source Go projects. Signals project is actively maintained. | Low | CI workflow running and green | Add GitHub Actions badge: `![CI](https://github.com/{owner}/{repo}/actions/workflows/ci.yml/badge.svg)` |
| `.golangci.yml` configuration file | Codifies which linters run, their settings, and exclusion rules. Without it, different developers get different lint results. Ensures CI and local development match. | Low | None | Use v2 format with `version: "2"`. Enable `staticcheck`, `errcheck`, `govet`, `gosec`, `errorlint`, `gofumpt`. Exclude generated code paths. |
| Separate lint and test CI jobs | Lint failures surface faster (lint typically completes in seconds). Running lint and test as parallel jobs reduces total CI wall-clock time. | Low | CI workflow | Two jobs: `lint` (golangci-lint) and `test` (go test -race). Both triggered on PRs and pushes to main. |
| Golden file helper as reusable `testutil` function | Instead of inline golden file logic in each test, a shared helper function handles read/compare/update. Consistent behavior across all golden file tests. | Low | None | ~30 lines of code. Reads `testdata/golden/{name}.golden.json`, compares with `got`, writes if `-update` flag is set. Pretty-prints JSON for readable diffs. |

## Anti-Features

Features to explicitly NOT build in this milestone. Including them would add complexity without proportionate value.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| External golden file library (goldie, abide, etc.) | Adds a dependency for something achievable in ~30 lines of stdlib code. Project CLAUDE.md prefers stdlib (MD-1). The stdlib approach with `flag.Bool` + `os.ReadFile`/`os.WriteFile` + `cmp.Diff` is the standard Go pattern. | Write a small `goldenTest()` helper function that handles read/compare/update in one function. |
| Golden files for ALL 13 types at ALL depth levels | Combinatorial explosion: 13 types x 3 depth levels (0, 2, with-filters) x list/detail = 78+ golden files. Serializers follow a uniform code-generated pattern -- testing the 4 most complex types validates the pattern for all types. | Golden files for `net`, `org`, `fac`, `ix` (the 4 types with the most fields and edge relationships). Remaining 9 types are structurally simpler and covered by `TestSerializerAllTypesCompile`. |
| Golden files for GraphQL or entrest REST APIs | These surfaces are code-generated by entgql and entrest respectively. Their response format correctness is the upstream library's responsibility. The compat layer is custom hand-written code that needs golden file validation. | Keep existing tests for GraphQL (`resolver_test.go`) and REST (`rest_test.go`). Golden files only for `pdbcompat` package. |
| Docker/container build in CI | Not needed -- project deploys via `fly deploy` which handles container builds. Adding Docker build steps increases CI time for no immediate value. | Add Docker build to CI only when/if container image publishing is needed. |
| E2E tests against live PeeringDB | Requires network access in CI, is inherently flaky, and PeeringDB data changes constantly. Cannot produce stable golden files. | Defer to human verification items already tracked in PROJECT.md (response fidelity vs live PeeringDB requires real deployment). |
| Pre-commit hooks | Enforcing lints locally via git hooks creates friction for contributors and can cause confusion when hooks fail differently than CI. CI is the single source of truth for enforcement. | CI runs `golangci-lint` and `go vet`. Developers can optionally run locally but are not required to. |
| Code coverage gating (minimum % threshold) | Coverage thresholds create perverse incentives (testing trivial code to hit numbers). The compat layer has meaningful tests; adding a threshold would block PRs for generated code coverage gaps. | Report coverage as informational only. Review coverage reports manually to find gaps. |
| Matrix builds across Go versions or OS | Project targets Go 1.26 only on Linux (Fly.io). No backward compat requirement. Wastes CI minutes. | Single Go version from `go.mod`, `ubuntu-latest` only. |
| Benchmark tests in CI | Premature -- no performance baseline exists. Adding benchmarks before profiling creates false confidence. Per PERF-1 (MUST), measure before optimizing. | Defer to a future milestone focused on performance characterization. |
| Snapshot testing of HTTP headers | Headers like `Content-Type: application/json` and `X-Powered-By` are already tested by `TestResponseHeaders`. Golden files for headers add diff noise without catching meaningful bugs. | Keep existing header assertions in `handler_test.go`. |
| Auto-generated OpenAPI docs hosting | entrest already generates and serves an OpenAPI spec at runtime. Hosting it separately is premature optimization. | Document the existing `/rest/v1/openapi.json` endpoint in the public access docs. |

## Feature Dependencies

```
.golangci.yml (v2 config with generated-code exclusions)
    |
    v
Fix existing test/lint issues (PREREQUISITE for everything else)
    |
    v
GitHub Actions CI workflow (.github/workflows/ci.yml)
    |
    +--> CI lint job (golangci-lint v2, go vet)
    |
    +--> CI test job (go test -race ./...)
    |        |
    |        +--> Golden file helper (goldenTest function)
    |        |        |
    |        |        +--> Deterministic test fixtures (fixed time.Date values)
    |        |        |        |
    |        |        |        +--> Golden files: list responses (net, org, fac, ix)
    |        |        |        |
    |        |        |        +--> Golden files: detail responses (net, org, fac, ix)
    |        |        |        |
    |        |        |        +--> Golden files: depth=2 responses (org, net, netfac)
    |        |        |        |
    |        |        |        +--> Golden files: field projection
    |        |        |        |
    |        |        |        +--> Golden files: filter results
    |        |        |
    |        |        +--> -update flag wired into test binary
    |        |
    |        +--> All existing tests passing
    |
    +--> CI vuln job (govulncheck, informational) [OPTIONAL]

Public access documentation --> independent of CI, no code dependencies
    +--> Verify no auth middleware blocks requests
    +--> Document all endpoint paths and access model
    +--> Add to README or docs/
```

## MVP Recommendation

**Phase ordering rationale:** Fix existing issues first (you cannot gate on CI if tests/lints fail), then build golden file infrastructure, then set up CI to enforce everything going forward. Public access docs are independent and can be done in parallel.

Prioritize:
1. **Fix existing test and lint issues** -- prerequisite for everything else. Cannot enable CI gating if the codebase does not pass. Create `.golangci.yml` v2 config with generated-code exclusions (`ent/`, `graph/generated.go`). Run `golangci-lint run` locally and fix violations in non-generated code.
2. **Golden file test infrastructure** -- the `goldenTest()` helper function and the `-update` flag. Small, stdlib-only (plus `cmp.Diff`), reusable. Establish deterministic `time.Date` fixtures.
3. **Golden files for core types (list + detail at depth=0)** -- `net`, `org`, `fac`, `ix`. These 4 types have the most complex serializers (most fields, optional pointers, social media arrays). Generate initial golden files with `go test ./internal/pdbcompat/ -update`.
4. **Golden files for depth-expanded responses** -- org depth=2, net depth=2, netfac depth=2. Validates the complex nested response shapes that are highest risk for regressions.
5. **GitHub Actions CI workflow** -- lint job (golangci-lint v2) and test job (go test -race) running on PRs and main pushes. Separate parallel jobs.
6. **Public access documentation** -- document that the API is fully public, list all endpoint paths (`/api/`, `/rest/v1/`, `/graphql`), note no auth/rate limits required.

Defer:
- **Golden files for remaining 9 types**: Diminishing returns. Add when/if those specific types have regressions.
- **Golden files for filter combinations**: Useful but not blocking. Add after core golden files are stable.
- **govulncheck in CI**: Nice-to-have. Add as a non-blocking informational job after CI is green.
- **CI badges in README**: Cosmetic. Add after CI has been green for a few runs.

## Sources

- [Testing with golden files in Go](https://medium.com/soon-london/testing-with-golden-files-in-go-7fccc71c43d3) -- stdlib golden file pattern with `flag.Bool`
- [Testing in Go: Golden Files](https://ieftimov.com/posts/testing-in-go-golden-files/) -- canonical blog post on the pattern
- [Golden Files -- Why you should use them](https://jarifibrahim.github.io/blog/golden-files-why-you-should-use-them/) -- best practices and pitfalls
- [Go wiki TestComments](https://go.dev/wiki/TestComments) -- `cmp.Diff` recommendation for test comparisons
- [golangci-lint-action](https://github.com/golangci/golangci-lint-action) -- official GitHub Action, v9 supports golangci-lint v2
- [golangci-lint v2 announcement](https://ldez.github.io/blog/2025/03/23/golangci-lint-v2/) -- v2 config changes, `version: "2"`, migration command
- [golangci-lint v2 migration guide](https://golangci-lint.run/docs/product/migration-guide/) -- official migration documentation
- [Go CI with GitHub Actions](https://www.alexedwards.net/blog/ci-with-go-and-github-actions) -- comprehensive Go CI setup guide
- [How to Set Up Go CI Pipeline](https://oneuptime.com/blog/post/2025-12-20-go-ci-pipeline-github-actions/view) -- 2025 Go CI reference
- [Go standard library testing package](https://go.dev/src/go/doc/testdata/testing.1.golden) -- stdlib itself uses .golden files in testdata
