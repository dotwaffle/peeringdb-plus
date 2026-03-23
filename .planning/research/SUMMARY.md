# Project Research Summary

**Project:** PeeringDB Plus v1.2 (Quality & CI)
**Domain:** Testing infrastructure, CI pipeline, and code quality enforcement for existing Go API project
**Researched:** 2026-03-23
**Confidence:** HIGH

## Executive Summary

PeeringDB Plus v1.2 is a quality and infrastructure milestone that introduces zero new runtime dependencies. The entire scope is testing, tooling configuration, and CI pipeline setup for an existing ~57K LOC Go codebase with ~300K LOC of generated code (ent + gqlgen). The project already has a comprehensive test suite (21 test files) and well-structured test infrastructure (`testutil.SetupClient` with isolated in-memory SQLite databases), making this milestone low-risk and high-confidence. The only Go module change is promoting `google/go-cmp` v0.7.0 from indirect to direct dependency for `cmp.Diff` usage in golden file tests.

The recommended approach is strictly sequential: fix existing lint and test violations first, then build golden file test infrastructure, then enable CI as the enforcement gate. This ordering is non-negotiable because CI cannot gate PRs if existing code does not pass, and golden files must exist before CI runs the test suite. The golden file pattern uses the standard Go `flag.Bool("-update")` + `testdata/*.golden` approach used by Go's own `cmd/gofmt` tests -- no third-party testing libraries. golangci-lint v2.11 with `generated: strict` mode handles the 37:1 ratio of generated-to-hand-written code. Two parallel CI jobs (lint and test) provide fast, independent feedback.

The primary risks are: (1) existing code may have latent lint violations or race conditions that surface when enforcement is first enabled, creating unpredictable scope in phase 1; (2) golden file timestamps must use fixed `time.Date()` values rather than `time.Now()`, which requires a separate test setup function from the existing handler tests; and (3) the 300K LOC of generated code must be correctly excluded from linting via both header detection and path exclusions to prevent CI timeouts and false positives. All three risks have well-documented mitigations and standard Go patterns.

## Key Findings

### Recommended Stack

No new Go module dependencies are required. The only change is promoting `google/go-cmp` from indirect to direct dependency (already at v0.7.0 in `go.sum`). All tooling additions are configuration-only: GitHub Actions YAML and golangci-lint YAML. This aligns with project constraint MD-1 (prefer stdlib). A quality/CI milestone should not introduce new runtime dependencies.

**Core technologies:**
- `flag` + `os` + `filepath` (stdlib): Golden file test infrastructure -- the standard Go pattern used by `cmd/gofmt` itself, ~20 lines of helper code
- `google/go-cmp` v0.7.0: Readable unified diffs for golden file mismatches -- already an indirect dependency, promoted to direct
- golangci-lint v2.11: Linter aggregator with v2 config format (`version: "2"`, `generated: strict`, `standard` preset plus `gosec`, `errorlint`, `gocritic`, and 6 others)
- `golangci-lint-action@v9`: Official GitHub Action, supports golangci-lint v2.11+, parallel binary download + cache
- `actions/setup-go@v6`: Auto-caches GOCACHE and GOMODCACHE using `go.sum` as cache key, reads Go version from `go.mod` via `go-version-file`
- `actions/checkout@v6`: Current stable (v6.0.2, Jan 2026, Node 24 runtime)

### Expected Features

**Must have (table stakes):**
- Golden file tests for core compat layer responses (net, org, fac, ix -- list and detail) -- existing tests check item counts only, not full JSON shape
- `-update` flag for golden file regeneration -- standard Go convention, stdlib-only
- Deterministic timestamps in test fixtures -- `time.Date(2025, 1, 1, ...)` not `time.Now()`
- Golden files stored in `internal/pdbcompat/testdata/golden/` following Go package convention
- GitHub Actions CI workflow with parallel lint + test jobs -- the project currently has no CI
- `go test -race ./...` in CI -- required by project rules T-2, G-3
- `go vet ./...` in CI -- required by project rule G-1 (included in golangci-lint default set)
- golangci-lint with generated code exclusions -- required by project rules G-2, TL-1
- All existing tests passing before CI gates are enabled -- prerequisite for everything
- Public access model documentation -- verify and document that all APIs are fully public

**Should have (differentiators):**
- Golden files for depth-expanded responses (`?depth=2`) -- highest-risk response shapes with nested object arrays
- Golden files for field projection (`?fields=id,name`) -- validates subset serialization
- `cmp.Diff` for readable golden file mismatch output -- dramatically improves debugging for 30+ field JSON objects
- `govulncheck` as informational (non-blocking) CI job -- catches known CVEs in dependencies
- Reusable `goldenTest()` helper in test code -- consistent behavior across all golden file tests
- CI status badge in README -- visual build health indicator
- `.gitattributes` for golden file line ending consistency

**Defer:**
- Golden files for remaining 9 PeeringDB types -- the 4 core types validate the pattern; remaining types are structurally simpler and covered by `TestSerializerAllTypesCompile`
- Golden files for filter combinations -- filter parser is generic across all types, testing one validates all
- Docker/container builds in CI -- Fly.io handles this via `fly deploy`
- Code coverage thresholds -- creates perverse incentives without an established baseline
- E2E tests against live PeeringDB -- flaky, data changes constantly
- Benchmark tests in CI -- no performance baseline exists yet (PERF-1: measure before optimizing)
- Pre-commit hooks -- CI is the single source of truth for enforcement

### Architecture Approach

Three additions integrate into the existing architecture without new packages or runtime code changes. All are testing, tooling, and CI configuration. The golden file tests add a `golden_test.go` file and `testdata/golden/` directory to the `internal/pdbcompat/` package, reusing the existing `httptest` test infrastructure. The CI pipeline is a single `.github/workflows/ci.yml` with two parallel jobs. The linter config is a `.golangci.yml` at the project root with two-layer generated code exclusion (header detection + path patterns).

**Major components:**
1. **Golden file test infrastructure** (`internal/pdbcompat/golden_test.go`) -- `goldenTest()` helper function, `-update` flag, deterministic `setupGoldenTestHandler()` with fixed timestamps, and ~15-20 `.json` golden reference files in `testdata/golden/`
2. **GitHub Actions CI pipeline** (`.github/workflows/ci.yml`) -- two parallel jobs: `lint` (golangci-lint v2.11 via official action) and `test` (`go test -race -count=1 -v ./...` + `go build -trimpath`)
3. **golangci-lint v2 configuration** (`.golangci.yml`) -- `standard` preset (errcheck, govet, ineffassign, staticcheck, unused), 9 additional linters (gosec, errorlint, nilerr, bodyclose, unconvert, misspell, copyloopvar, intrange, gocritic), `gofumpt`/`goimports` formatters, `generated: strict` + explicit path exclusions for `ent/` generated code, test file relaxations for gosec

### Critical Pitfalls

1. **Non-deterministic timestamps in golden files** -- Existing `handler_test.go` uses `time.Now()`. Golden file tests MUST use fixed `time.Date()` values via a separate `setupGoldenTestHandler()` function. The existing `serializer_test.go` already demonstrates the correct pattern. Without this, every test run produces different output and golden files never match.
2. **300K LOC generated code swamps the linter** -- Use golangci-lint v2's `generated: strict` mode plus explicit path exclusions. Do NOT blanket-exclude `ent/` because `ent/schema/` contains hand-written schema definitions that must be linted. Two-layer defense: header detection catches most generated files, path patterns catch any without the standard header.
3. **Existing code fails under new enforcement** -- Run `golangci-lint run` and `go test -race ./...` locally BEFORE creating the CI workflow. Fix ALL existing violations first. Phase 1 scope is unpredictable until this assessment is done. Known tech debt includes exported-but-unused functions in `graph/globalid.go`.
4. **Auto-increment ID instability in golden files** -- Entity creation order determines SQLite auto-increment IDs. Changing creation order silently breaks all golden files including nested `_set` IDs from depth expansion. Do not use `t.Parallel()` in golden file setup. Document creation order as a contract.
5. **Golden file update workflow masks regressions** -- Running `go test -update && git add .` commits whatever the current output is without verifying correctness. For a compatibility layer, golden files ARE the compatibility contract. CI must NEVER run with `-update`. Golden file updates must be in dedicated commits with explanatory messages.

## Implications for Roadmap

Based on research, suggested phase structure:

### Phase 1: Lint Configuration and Existing Issue Remediation

**Rationale:** Nothing else can proceed until the codebase passes linting and tests under the new enforcement rules. This phase has unpredictable scope -- the existing ~8K lines of hand-written code have never been linted. Must be completed first so all subsequent code (golden file tests) is written to pass the linter from the start.
**Delivers:** Clean `golangci-lint run` pass, all existing tests passing under `-race`, `.golangci.yml` v2 config with generated code exclusions
**Addresses:** golangci-lint config (table stakes), go vet enforcement (table stakes), fix existing test/lint issues (table stakes prerequisite)
**Avoids:** Pitfall 2 (generated code false positives -- via `generated: strict` + path exclusions), Pitfall 3 (existing code fails under enforcement -- fix before enabling CI), Pitfall 7 (version mismatch -- pin v2.11 in config, `version: "2"` header warns on mismatch)

### Phase 2: Golden File Tests

**Rationale:** With lint passing, golden file tests can be written to lint standards from the start. Golden files must exist before CI is enabled, otherwise the test job fails on the first run. This phase is the core deliverable of the milestone.
**Delivers:** `goldenTest()` helper function, deterministic test fixtures with fixed timestamps, golden files for 4 core types (net, org, fac, ix) at list and detail endpoints, golden files for depth-expanded responses (`?depth=2`), field projection, error cases, and the API index endpoint (~15-20 golden reference files total)
**Addresses:** Golden file tests for list/detail responses (table stakes), deterministic fixtures (table stakes), `-update` flag (table stakes), depth-expanded golden files (differentiator), field projection golden files (differentiator), `cmp.Diff` output (differentiator), reusable `goldenTest()` helper (differentiator)
**Avoids:** Pitfall 1 (non-deterministic timestamps -- use `time.Date()` in separate setup function), Pitfall 4 (JSON field ordering -- use `json.MarshalIndent` for consistent output), Pitfall 6 (auto-ID instability -- deterministic sequential creation, no parallel setup), Pitfall 8 (accidental regeneration -- never `-update` in CI), Pitfall 9 (line endings -- add `.gitattributes`)

### Phase 3: CI Pipeline and Public Access Documentation

**Rationale:** Everything CI validates (lint config, golden files, existing tests) is already in place and passing locally. CI is the enforcement mechanism, not the fix-it mechanism. Public access docs are independent of CI and slot naturally alongside.
**Delivers:** `.github/workflows/ci.yml` with two parallel jobs (lint + test), `go build -trimpath` verification, public access documentation (all endpoints, no auth required), `.gitattributes` for golden file line endings, CI status badge in README
**Addresses:** GitHub Actions CI workflow (table stakes), race detection in CI (table stakes), build verification with `-trimpath` (table stakes), public access documentation (table stakes), separate lint/test jobs (differentiator), CI badge (differentiator)
**Avoids:** Pitfall 3 (CI fails on first run -- everything is already fixed in Phase 1), Pitfall 5 (SQLite locking under race detector -- verify `busy_timeout` in test DSN), Pitfall 7 (version mismatch -- `version: v2.11` pinned in action config)

### Phase Ordering Rationale

- **Phase 1 before Phase 2:** Linting must be configured and passing before writing new test code. Otherwise `golden_test.go` itself fails linting, creating rework.
- **Phase 2 before Phase 3:** Golden files must exist before CI runs the test suite. Adding CI first would produce a test job that fails because golden files do not exist.
- **Phase 3 last:** CI validates everything that came before. It is the capstone, not the foundation. The first CI run should pass green.
- **Public access docs in Phase 3:** No code dependencies, can run in parallel with CI setup. Logically groups "external-facing deliverables" together.

### Research Flags

Phases likely needing deeper research during planning:
- **Phase 1:** Scope is unpredictable until `golangci-lint run` is executed against the existing codebase. The number of existing violations determines how much work this phase requires. Run lint locally as the first action to assess scope before finalizing the phase plan.

Phases with standard patterns (skip research-phase):
- **Phase 2:** Golden file testing is a well-documented Go pattern with canonical references (Go wiki, `cmd/gofmt` source, multiple blog posts). The architecture research provides complete implementation including code examples for the `goldenTest()` helper.
- **Phase 3:** GitHub Actions CI for Go is thoroughly documented. The research provides the exact YAML configuration needed. No unknowns.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Zero new dependencies. All tools verified current. golangci-lint v2.11.4 released 2026-03-22. Actions versions confirmed against official repositories. |
| Features | HIGH | Feature scope well-defined and constrained. Clear distinction between table stakes, differentiators, and anti-features. Anti-features justified with rationale. |
| Architecture | HIGH | No production code changes. All additions are test/tooling/config files. Clean integration with existing test infrastructure (`httptest`, `testutil.SetupClient`). |
| Pitfalls | HIGH | 10 pitfalls identified (3 critical, 5 moderate, 2 minor). All verified against actual codebase source code (handler_test.go timestamps, generated code headers, testutil setup). Every critical pitfall has a concrete prevention strategy. |

**Overall confidence:** HIGH

### Gaps to Address

- **Existing lint violation count unknown:** Until `golangci-lint run` is executed against the current codebase, Phase 1 scope cannot be precisely estimated. Could be 5 violations or 50. This is the only significant planning uncertainty in the milestone.
- **SQLite `busy_timeout` in test DSN:** Pitfall 5 identifies potential "database is locked" errors under `-race`. Check whether `busy_timeout` is already configured in the test DSN (`internal/testutil/testutil.go`). If not, add it as part of Phase 1 remediation.
- **Golden file count will be determined during Phase 2:** The research estimates 15-20 golden files. The actual count depends on how many depth/projection/error scenarios provide meaningful coverage. Start with core list + detail for 4 complex types; expand based on review of generated golden files.

## Sources

### Primary (HIGH confidence)
- [Go wiki TestComments](https://go.dev/wiki/TestComments) -- golden file pattern, `cmp.Diff` recommendation
- [Go gofmt test source](https://go.dev/src/cmd/gofmt/gofmt_test.go) -- canonical `flag.Bool("update")` pattern
- [golangci-lint v2 config reference](https://golangci-lint.run/docs/configuration/file/) -- v2 YAML format, `generated: strict`, `standard` preset
- [golangci-lint v2 migration guide](https://golangci-lint.run/docs/product/migration-guide/) -- v1 to v2 breaking changes
- [golangci-lint releases](https://github.com/golangci/golangci-lint/releases) -- v2.11.4 released 2026-03-22
- [golangci-lint-action v9](https://github.com/golangci/golangci-lint-action) -- GitHub Action, requires golangci-lint >= v2.1.0
- [actions/setup-go v6](https://github.com/actions/setup-go) -- auto-caching, `go-version-file`
- [actions/checkout v6](https://github.com/actions/checkout/releases) -- v6.0.2, Jan 2026
- [google/go-cmp v0.7.0](https://github.com/google/go-cmp/releases) -- released 2026-02-21

### Secondary (MEDIUM confidence)
- [File-driven testing in Go (Eli Bendersky)](https://eli.thegreenplace.net/2022/file-driven-testing-in-go/) -- golden file pattern overview
- [Testing with golden files in Go](https://medium.com/soon-london/testing-with-golden-files-in-go-7fccc71c43d3) -- stdlib golden file pattern
- [Golden Files -- Why you should use them](https://jarifibrahim.github.io/blog/golden-files-why-you-should-use-them/) -- best practices and pitfalls
- [Go CI with GitHub Actions](https://www.alexedwards.net/blog/ci-with-go-and-github-actions) -- CI setup reference
- [SQLite concurrent writes](https://tenthousandmeters.com/blog/sqlite-concurrent-writes-and-database-is-locked-errors/) -- busy_timeout mitigation
- [Golden config for golangci-lint](https://gist.github.com/maratori/47a4d00457a92aa426dbd48a18776322) -- comprehensive v2 example

### Codebase Verification
- `internal/pdbcompat/handler_test.go` -- confirmed `time.Now()` usage in test fixtures
- `internal/pdbcompat/serializer_test.go` -- confirmed fixed `time.Date()` usage (correct pattern)
- `internal/testutil/testutil.go` -- confirmed dual-connection SQLite setup, atomic counter for unique DBs
- `ent/client.go` line 1 -- confirmed `// Code generated ... DO NOT EDIT.` header present
- `graph/generated.go` line 1 -- confirmed standard generated header present
- `ent/schema/*.go` -- confirmed NO generated header (hand-written, must be linted)

---
*Research completed: 2026-03-23*
*Ready for roadmap: yes*
