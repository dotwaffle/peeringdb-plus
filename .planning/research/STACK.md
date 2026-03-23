# Technology Stack: v1.2 Additions

**Project:** PeeringDB Plus v1.2 (Quality & CI)
**Researched:** 2026-03-23
**Scope:** Stack additions/changes for golden file testing, GitHub Actions CI pipeline, and golangci-lint enforcement. Does NOT re-research validated v1.0/v1.1 stack.

## New Dependencies

### Golden File Testing

No new Go module dependencies required. The golden file pattern uses stdlib only.

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| `flag` (stdlib) | Go 1.26 | `-update` flag for golden file regeneration | Standard Go pattern: `var update = flag.Bool("update", false, "update golden files")`. Used by Go's own `cmd/gofmt` tests. No library needed. | HIGH |
| `os` (stdlib) | Go 1.26 | Read/write `.golden` files in `testdata/` | `os.ReadFile` / `os.WriteFile` for golden file I/O. | HIGH |
| `filepath` (stdlib) | Go 1.26 | Auto-discover test input/golden file pairs | `filepath.Glob` discovers `testdata/*.golden` files, each becomes a subtest. | HIGH |
| `github.com/google/go-cmp/cmp` | v0.7.0 | Readable diffs when golden files mismatch | Already an indirect dependency (v0.7.0). Promote to direct. `cmp.Diff(want, got)` produces human-readable diffs in test failure output. Standard practice per Go wiki TestComments. | HIGH |

**Pattern chosen:** Stdlib `flag.Bool("-update")` + `testdata/*.golden` files.

**Why not a golden file library (goldie, xorcare/golden, gotest.tools/golden)?**
- The project's golden files are JSON HTTP responses with a well-defined structure
- The stdlib pattern is ~20 lines of helper code, fully understood, no dependency
- Third-party libraries add abstraction over trivially simple file comparison
- The `-update` flag pattern is the Go standard (used in `cmd/gofmt`, `cmd/go` itself)

**Implementation approach:**
```go
// In pdbcompat package test file:
var update = flag.Bool("update", false, "update .golden files")

// Per-test:
golden := filepath.Join("testdata", tc.name+".golden")
if *update {
    os.WriteFile(golden, got, 0644)
}
want, _ := os.ReadFile(golden)
if diff := cmp.Diff(string(want), string(got)); diff != "" {
    t.Errorf("mismatch (-want +got):\n%s", diff)
}
```

**Golden file naming convention:** `testdata/<type>_<scenario>.golden` (e.g., `testdata/net_list.golden`, `testdata/net_detail_13335.golden`, `testdata/net_filter_asn_contains.golden`).

**What gets golden-filed:**
- Full HTTP response bodies (JSON) for each of the 13 PeeringDB types
- List endpoints, detail endpoints, filtered queries, pagination, depth expansion
- Error responses (404, unknown type)
- The `/api/` index endpoint

### GitHub Actions CI Pipeline

No Go module dependencies. GitHub Actions configuration only.

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| `actions/checkout` | v6 | Repository checkout | Current stable (v6.0.2, Jan 2026). Node 24 runtime. | HIGH |
| `actions/setup-go` | v6 | Go toolchain setup | Current stable. Auto-caches `GOCACHE` and `GOMODCACHE` using `go.sum` as cache key. Supports `go-version-file: go.mod` to read Go version from `go 1.26.1` directive. | HIGH |
| `golangci/golangci-lint-action` | v9 | Lint execution | Current stable (v9.0.0+). Requires golangci-lint >= v2.1.0. Parallel binary download + cache retrieval. Produces GitHub-native annotations on PR diffs. Run as separate job for parallelism with tests. | HIGH |

**Why these versions:**
- `actions/checkout@v6` and `actions/setup-go@v6` both use Node 24 runtime, matching GitHub's current runner requirements
- `golangci-lint-action@v9` is the only version that supports golangci-lint v2.11+ (the current release line)
- `setup-go@v6` with `go-version-file: go.mod` reads Go 1.26.1 from the existing `go.mod` -- no hardcoded version to maintain

**Workflow structure (two parallel jobs):**

1. **`lint` job:** checkout, setup-go, golangci-lint-action
2. **`test` job:** checkout, setup-go, `go test -race -count=1 ./...`

**Why two jobs, not one:**
- Lint and test run in parallel, reducing wall-clock CI time
- golangci-lint-action docs explicitly recommend this ("run it as a separate job")
- Lint failures don't block test results (and vice versa)
- Each job gets fresh caching behavior appropriate to its workload

**Caching strategy:**
- `actions/setup-go@v6` caches Go modules automatically (keyed on `go.sum`)
- `golangci-lint-action@v9` caches lint analysis results separately
- No manual cache configuration needed

### golangci-lint v2 Configuration

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| golangci-lint | v2.11 | Linter aggregator | Current stable release line (v2.11.4, 2026-03-22). v2 is a breaking change from v1 -- new config format with `version: "2"`. Merged `gosimple` + `stylecheck` into `staticcheck`. Moved formatters (gofumpt, goimports) to `formatters` section. | HIGH |

**Default linters (the `standard` preset):**
- `errcheck` -- unchecked error returns
- `govet` -- suspicious constructs (Go's built-in `go vet`)
- `ineffassign` -- assignments to variables that are never used
- `staticcheck` -- comprehensive static analysis (absorbed `gosimple` + `stylecheck`)
- `unused` -- unused code detection

**Additional linters to enable (beyond defaults):**
- `gosec` -- security-oriented checks (SEC-1, SEC-2 from CLAUDE.md)
- `errorlint` -- proper `errors.Is`/`errors.As` usage (ERR-2 from CLAUDE.md)
- `nilerr` -- returning nil when err is not nil
- `bodyclose` -- HTTP response body closure
- `unconvert` -- unnecessary type conversions
- `misspell` -- common misspellings in comments and strings
- `copyloopvar` -- Go 1.22+ loop variable semantics (modern Go per CS-0)
- `intrange` -- prefer `range N` over `for i := 0; i < N; i++` (Go 1.22+)
- `gocritic` -- opinionated Go best practices

**Formatters to enable:**
- `gofumpt` -- stricter gofmt (per TL-1 from CLAUDE.md)
- `goimports` -- import grouping and ordering

**Linters to NOT enable:**
- `depguard` -- dependency allowlisting is overkill for this project
- `funlen` -- the serializer functions are necessarily long (13 type mappers)
- `wsl` -- overly opinionated whitespace enforcement
- `gocognit` / `cyclop` -- the filter parser has inherent complexity, these would generate noise
- `ireturn` -- conflicts with API-2 (return concrete types) in cases where interfaces are needed
- `nlreturn` -- stylistic, not safety-related

**v2 config format (`.golangci.yml`):**

```yaml
version: "2"

run:
  timeout: 5m

linters:
  default: standard
  enable:
    - gosec
    - errorlint
    - nilerr
    - bodyclose
    - unconvert
    - misspell
    - copyloopvar
    - intrange
    - gocritic
  settings:
    govet:
      enable-all: true
    staticcheck:
      checks: ["all"]
    errcheck:
      check-type-assertions: true
      check-blank: true
    gocritic:
      enabled-tags:
        - diagnostic
        - performance
  exclusions:
    presets:
      - common-false-positives
    rules:
      - path: _test\.go
        linters:
          - gosec
      - path: ent/
        linters:
          - gocritic
          - gosec

formatters:
  enable:
    - gofumpt
    - goimports
  exclusions:
    paths:
      - ent/
```

**Key exclusions explained:**
- `ent/` directory: Generated code, not hand-written. Exclude from style linters and formatters.
- `_test.go` files: Security linter (`gosec`) is noisy on test code (hardcoded test values, etc.)
- `common-false-positives` preset: Built-in exclusion set that suppresses known false positives

### Test Execution

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| `go test` (stdlib) | Go 1.26 | Test runner | `-race` flag for race detection (T-2, G-3). `-count=1` to disable test caching in CI. `-v` for verbose output. | HIGH |

**Why NOT gotestsum:**
- `go test -v ./...` output is sufficient for GitHub Actions (setup-go's problem matcher already parses it)
- gotestsum adds a binary installation step and another tool to maintain
- JUnit XML reporting is not needed (no external test dashboard integration planned)
- Can add later if test output becomes unwieldy

**Race detection considerations with modernc.org/sqlite:**
- modernc.org/sqlite is pure Go, so `-race` works without CGo complications
- The existing `testutil.SetupClient` creates isolated in-memory databases per test (unique DSN per `dbCounter`)
- `t.Parallel()` is already used throughout existing tests
- No known race issues with modernc.org/sqlite when using separate connections (confirmed by their CI running with `-race` for 2+ years)

## Promote from Indirect to Direct

| Dependency | Current | Action | Rationale |
|------------|---------|--------|-----------|
| `github.com/google/go-cmp` | v0.7.0 indirect | Promote to direct | Used explicitly in golden file test assertions via `cmp.Diff`. Already in `go.sum`. No version change. |

## No New Go Dependencies

Everything else needed is:
- Stdlib (`flag`, `os`, `filepath`, `testing`, `net/http/httptest`)
- Already in `go.mod` (`google/go-cmp` as indirect)
- GitHub Actions configuration (YAML, not Go code)
- golangci-lint configuration (YAML, not Go code)

**This is deliberate.** A quality/CI milestone should not introduce new runtime dependencies.

## Alternatives Considered

| Category | Recommended | Alternative | Why Not |
|----------|-------------|-------------|---------|
| Golden file library | Stdlib `flag.Bool` pattern | `github.com/sebdah/goldie/v2` | Goldie is 1.5K stars but adds abstraction over trivial file I/O. The stdlib pattern is ~20 lines, fully transparent, used by Go stdlib itself. |
| Golden file library | Stdlib `flag.Bool` pattern | `gotest.tools/v3/golden` | Part of larger `gotest.tools` suite. Pulls in transitive deps. Overkill for JSON file comparison. |
| Golden file library | Stdlib `flag.Bool` pattern | `github.com/xorcare/golden` | Well-designed but small user base (100 stars). Adds a dependency for what stdlib handles natively. |
| Golden file format | Plain JSON `.golden` files | `txtar` archives | txtar is for multi-file test archives (compiler tests, gopls). Our golden files are single JSON responses. txtar adds unnecessary indirection. |
| Test diff output | `google/go-cmp` | `reflect.DeepEqual` + `fmt.Sprintf` | `cmp.Diff` produces readable unified diffs. `DeepEqual` gives only true/false -- useless for debugging JSON mismatches. |
| Test diff output | `google/go-cmp` | `encoding/json` manual comparison | Byte-level comparison is fragile (key ordering, whitespace). `cmp.Diff` compares semantically. |
| CI test output | Raw `go test -v` | `gotestsum` | Adds install step, another tool version to track. `go test -v` output is parsed by `actions/setup-go` problem matcher already. Reconsider if test suite grows past 100 tests. |
| CI lint action | `golangci/golangci-lint-action@v9` | `reviewdog/action-golangci-lint` | reviewdog is a wrapper around golangci-lint. Adds indirection. Official action has better caching and is maintained by golangci-lint authors. |
| golangci-lint version | v2.11 | v1.x (legacy) | v1.x is end-of-life. v2 is current. Config format is incompatible but `golangci-lint migrate` handles conversion. Starting fresh with v2 config. |
| CI runner OS | `ubuntu-latest` | Matrix with macOS/Windows | This is a server application deployed on Linux (Fly.io). macOS/Windows testing adds CI minutes for no deployment benefit. modernc.org/sqlite is pure Go, no platform-specific code. |
| Go version matrix | Single `go-version-file: go.mod` | Matrix with Go 1.25 + 1.26 | Project targets Go 1.26 exclusively. No backward compat requirement. Matrix testing wastes CI minutes. |

## Version Pinning Strategy

| Component | Pin Strategy | Rationale |
|-----------|-------------|-----------|
| Go version | Read from `go.mod` (`go 1.26.1`) | Single source of truth. `actions/setup-go` reads it via `go-version-file`. |
| golangci-lint | `version: v2.11` in action config | Pin to minor version. Action resolves latest patch. |
| `google/go-cmp` | v0.7.0 in `go.mod` | Already pinned. Stable API. |
| GitHub Actions | Major version tags (`@v6`, `@v9`) | Standard practice. Major versions are stable. |

## Risk Register

| Risk | Severity | Likelihood | Mitigation |
|------|----------|------------|------------|
| golangci-lint v2 config format unfamiliar | LOW | MEDIUM | Format is well-documented. `golangci-lint migrate` converts v1 configs. Starting fresh with v2 avoids migration issues. |
| Generated `ent/` code triggers lint warnings | MEDIUM | HIGH | Exclude `ent/` from linters and formatters via `exclusions.paths`. Already planned in config above. |
| Golden file tests become brittle (timestamp sensitivity) | MEDIUM | MEDIUM | Use fixed `time.Date()` values in test data (already done in existing tests). Normalize timestamps in golden file comparison if needed. |
| `-race` flag slows CI significantly | LOW | LOW | modernc.org/sqlite pure Go is not affected by CGo race detector overhead. Tests are fast (in-memory SQLite). Budget 2-3x normal test time for race detection. |
| GitHub Actions runner doesn't have Go 1.26.1 | LOW | LOW | `actions/setup-go` downloads Go versions on demand. Not limited to pre-installed versions. |

## Sources

- [Go wiki: TestComments](https://go.dev/wiki/TestComments) -- `cmp.Diff` recommendation
- [Go gofmt test source](https://go.dev/src/cmd/gofmt/gofmt_test.go?m=text) -- `var update = flag.Bool("update", ...)` pattern
- [File-driven testing in Go (Eli Bendersky)](https://eli.thegreenplace.net/2022/file-driven-testing-in-go/) -- golden file pattern with `testdata/`
- [Golden file testing (Ibrahim Jarif)](https://jarifibrahim.github.io/blog/golden-files-why-you-should-use-them/) -- `-update` flag pattern
- [google/go-cmp v0.7.0](https://github.com/google/go-cmp/releases) -- released 2026-02-21
- [golangci-lint releases](https://github.com/golangci/golangci-lint/releases) -- v2.11.4 released 2026-03-22
- [golangci-lint v2 migration guide](https://golangci-lint.run/docs/product/migration-guide/) -- config format changes
- [golangci-lint v2 config reference](https://golangci-lint.run/docs/configuration/file/) -- `version: "2"` format
- [golangci-lint default linters](https://golangci-lint.run/docs/welcome/quick-start/) -- `standard` preset: errcheck, govet, ineffassign, staticcheck, unused
- [golangci-lint-action](https://github.com/golangci/golangci-lint-action) -- v9.0.0, requires golangci-lint >= v2.1.0
- [actions/setup-go](https://github.com/actions/setup-go) -- v6, auto-caches Go modules
- [actions/checkout](https://github.com/actions/checkout/releases) -- v6.0.2, Jan 2026
- [Golden config for golangci-lint](https://gist.github.com/maratori/47a4d00457a92aa426dbd48a18776322) -- comprehensive v2 example
- [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) -- race detector compatibility confirmed
