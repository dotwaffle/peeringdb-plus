# Architecture Patterns

**Domain:** v1.2 Quality & CI -- golden file tests, GitHub Actions CI, linting enforcement
**Researched:** 2026-03-23
**Focus:** How golden file tests, CI pipelines, and linting integrate with the existing Go/ent codebase

## Existing Architecture (Context)

The v1.1 architecture is a single Go binary with the following structure:

```
cmd/peeringdb-plus/main.go     (wiring, HTTP server, graceful shutdown)
  |
  +-- internal/config/          Config from env vars (immutable after load)
  +-- internal/database/        SQLite open (modernc.org, WAL, FK)
  +-- internal/otel/            OTel pipeline: Tracer, Meter, Logger providers
  +-- internal/peeringdb/       PeeringDB API client (HTTP, retry, rate limit, OTel tracing)
  +-- internal/sync/            Sync worker (fetch -> filter -> upsert -> delete, per-type metrics)
  +-- internal/health/          /healthz (liveness), /readyz (readiness + sync freshness)
  +-- internal/middleware/       Logging, Recovery, CORS
  +-- internal/litefs/          LiteFS primary detection
  +-- internal/graphql/         GraphQL handler factory (gqlgen server config)
  +-- internal/pdbcompat/       PeeringDB-compatible REST layer (13 types, Django-style filters)
  +-- graph/                    gqlgen resolvers, generated code
  +-- ent/                      entgo ORM (13 schemas), generated code
  +-- ent/schema/               Schema definitions with entgql + entrest annotations
  +-- ent/rest/                 entrest-generated REST handlers
  +-- testdata/fixtures/        Sync fixture data (13 JSON files for sync integration tests)
```

**Existing test structure (21 test files):**
```
cmd/pdb-schema-extract/main_test.go        Schema extraction tool tests
cmd/pdb-schema-generate/main_test.go       Schema generation tool tests
cmd/peeringdb-plus/rest_test.go            entrest integration tests (httptest)
ent/schema/organization_test.go            Schema validation tests
ent/schema/schema_test.go                  Schema validation tests
graph/resolver_test.go                     GraphQL resolver tests
internal/config/config_test.go             Config parsing tests
internal/health/handler_test.go            Health endpoint tests
internal/litefs/primary_test.go            LiteFS detection tests
internal/middleware/cors_test.go           CORS middleware tests
internal/otel/logger_test.go              Dual slog logger tests
internal/otel/metrics_test.go             Metric instrument tests
internal/otel/provider_test.go            OTel provider lifecycle tests
internal/pdbcompat/depth_test.go          Depth expansion tests (httptest)
internal/pdbcompat/filter_test.go         Django-style filter parser tests
internal/pdbcompat/handler_test.go        PDB compat endpoint tests (httptest)
internal/pdbcompat/serializer_test.go     ent -> peeringdb type mapping tests
internal/peeringdb/client_test.go         PeeringDB HTTP client tests
internal/peeringdb/types_test.go          PeeringDB type deserialization tests
internal/sync/integration_test.go         Full sync cycle tests (httptest)
internal/sync/worker_test.go              Sync worker unit tests
```

**Test infrastructure:**
- `internal/testutil/testutil.go` provides `SetupClient(t)` which creates in-memory SQLite-backed ent clients via `enttest.Open`. Each call gets a unique database (atomic counter) so parallel tests do not conflict.
- Tests use `httptest.NewRecorder()` and `httptest.NewRequest()` for HTTP endpoint testing.
- `testdata/fixtures/` contains 13 JSON files with PeeringDB-format response data used by sync integration tests.
- No golden file tests exist yet.
- No CI pipeline exists (no `.github/` directory).
- No golangci-lint configuration exists (no `.golangci.yml`).

## Recommended Architecture for v1.2

Three new capabilities integrate into the existing architecture. None require new packages or runtime dependencies. All additions are testing, tooling, and CI configuration.

### 1. Golden File Tests for PeeringDB Compat Layer

**What changes:** The PeeringDB compatibility layer (`internal/pdbcompat/`) gains golden file tests that capture exact HTTP response bodies (JSON) and compare them against reference files.

**Where golden files live:**

```
internal/pdbcompat/testdata/golden/
    net_list.json                List response for /api/net
    net_detail.json              Detail response for /api/net/{id}
    net_detail_depth2.json       Detail with depth=2 for /api/net/{id}?depth=2
    org_list.json                List response for /api/org
    org_detail_depth2.json       Detail with depth=2 for /api/org/{id}?depth=2
    fac_list.json                ...
    ix_list.json                 ...
    index.json                   API index at /api/
    error_not_found.json         404 error envelope
    error_unknown_type.json      404 for unknown type
    fields_projection.json       ?fields=id,name projection
```

**Why `internal/pdbcompat/testdata/golden/` not root `testdata/`:**
Go convention places test fixtures in a `testdata/` subdirectory of the package being tested. The root `testdata/fixtures/` already holds sync fixture data. Keeping golden files colocated with the pdbcompat package follows Go stdlib convention.

**New/modified files:**

| File | Change Type | What |
|------|-------------|------|
| `internal/pdbcompat/golden_test.go` | NEW | Golden file test functions + helpers (update, format, diff) |
| `internal/pdbcompat/testdata/golden/*.json` | NEW | Golden reference files (~15-20 files) |

### 2. GitHub Actions CI Pipeline

**Workflow file:** `.github/workflows/ci.yml`

**Workflow structure (two parallel jobs):**

```yaml
name: CI
on:
  push:
    branches: [main]
  pull_request:

permissions:
  contents: read

jobs:
  lint:
    name: Lint
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
      - uses: golangci/golangci-lint-action@v9
        with:
          version: v2.11

  test:
    name: Test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
      - run: go test -race -count=1 -v ./...
      - run: go build -trimpath ./cmd/peeringdb-plus
```

**Key design decisions:**

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Separate lint/test jobs | YES | Run in parallel, faster feedback. golangci-lint-action recommends separate job. |
| Go version via `go-version-file` | YES | Reads `go 1.26.1` from `go.mod`. No hardcoded version to maintain. |
| `-race` flag on tests | YES | Per project rule T-2 (MUST). Catches data races. |
| `-count=1` on tests | YES | Disables test caching in CI. Ensures every run is fresh. |
| `-trimpath` on build | YES | Per project rule CI-2 (MUST). Reproducible builds. |
| Build in test job not separate | YES | Build verifies compilation after tests pass. Does not need to be a separate job -- it is fast (seconds) and logically sequential with testing. |
| `actions/checkout@v6` | YES | Current stable (v6.0.2, Jan 2026). Node 24 runtime. |
| `actions/setup-go@v6` | YES | Current stable. Auto-caches GOCACHE and GOMODCACHE using go.sum as cache key. |
| `golangci-lint-action@v9` | YES | Current stable. Supports golangci-lint v2.11+. Parallel binary download + cache. |

**New files:**

| File | Change Type | What |
|------|-------------|------|
| `.github/workflows/ci.yml` | NEW | CI workflow (lint + test) |

### 3. golangci-lint v2 Configuration

**Configuration file:** `.golangci.yml`

Uses golangci-lint v2 format (`version: "2"`). Key structural changes from v1:
- `linters-settings` split into `linters.settings` and `formatters.settings`
- Formatting linters (`gofumpt`, `goimports`) moved to `formatters` section
- `gosimple` and `stylecheck` merged into `staticcheck`
- `enable-all`/`disable-all` replaced by `default: standard|all|none|fast`
- `issues.exclude-dirs` replaced by `linters.exclusions.paths`

**Generated code exclusion strategy:**

Two-layer defense:
1. `generated: strict` -- matches `// Code generated ... DO NOT EDIT.` headers (standard Go convention used by ent, gqlgen, entrest)
2. Explicit `exclusions.paths` -- catches any generated files missing the header

**Hand-written code that MUST be linted:**

| Directory/File | Status | Notes |
|----------------|--------|-------|
| `cmd/peeringdb-plus/main.go` | LINT | Entry point, wiring |
| `internal/*/` | LINT | All hand-written internal packages |
| `ent/schema/*.go` | LINT | Hand-written schema definitions |
| `graph/resolver.go` | LINT | Hand-written resolver |
| `graph/custom.resolvers.go` | LINT | Hand-written custom resolvers |
| `graph/pagination.go` | LINT | Hand-written pagination helpers |

**New files:**

| File | Change Type | What |
|------|-------------|------|
| `.golangci.yml` | NEW | Linter configuration |

## Data Flow

### Test Data Flow (Golden Files)

```
setupGoldenTestHandler(t)
    |
    v
Create deterministic ent entities in in-memory SQLite
(fixed time.Date values, sequential creation for deterministic auto-increment IDs)
    |
    v
httptest.NewRequest -> mux.ServeHTTP -> httptest.NewRecorder
    |
    v
rec.Body.Bytes() -> formatJSON() -> compare against testdata/golden/*.json
                                        |
                    (if -update flag) -> os.WriteFile() to golden file
                    (if no flag)     -> cmp.Diff(want, got) -> PASS/FAIL
```

### CI Pipeline Flow

```
Push/PR -> GitHub Actions trigger
    |
    +-- [Job: lint] -----> checkout -> setup-go (from go.mod) -> golangci-lint v2.11
    |                       reads .golangci.yml
    |                       excludes generated code (headers + paths)
    |                       lints internal/*, cmd/*, ent/schema/*, graph/*.go
    |
    +-- [Job: test] -----> checkout -> setup-go (from go.mod)
                            -> go test -race -count=1 -v ./...
                            -> go build -trimpath ./cmd/peeringdb-plus
```

### Linter Resolution Flow

```
golangci-lint run
    |
    v
Read .golangci.yml (version: "2")
    |
    v
Identify Go files in module
    |
    v
Apply exclusions:
  1. generated: strict -> skip files with "// Code generated" header
  2. paths -> skip ent/(non-schema), graph/generated.go, graph/model/
  3. rules -> relax linters for _test.go files
    |
    v
Run enabled linters on remaining files
    |
    v
Report issues (fail CI if any)
```

## Patterns to Follow

### Pattern 1: Golden File Tests with `-update` Flag

**What:** Store expected HTTP response bodies as JSON files in `testdata/golden/`. Compare actual output against these files. Regenerate with `go test -update`.

**When:** Testing API response format fidelity, serialization correctness, or any output where the exact shape matters.

**Example:**

```go
var update = flag.Bool("update", false, "update golden files")

func goldenTest(t *testing.T, name string, got []byte) {
    t.Helper()
    golden := filepath.Join("testdata", "golden", name+".json")
    formatted := formatJSON(t, got)

    if *update {
        os.MkdirAll(filepath.Dir(golden), 0o755)
        os.WriteFile(golden, formatted, 0o644)
        return
    }

    want, err := os.ReadFile(golden)
    if err != nil {
        t.Fatalf("read golden %s: %v (run with -update to create)", golden, err)
    }
    if diff := cmp.Diff(string(want), string(formatted)); diff != "" {
        t.Errorf("golden mismatch for %s (-want +got):\n%s", name, diff)
    }
}
```

**Why:** Zero external dependencies beyond `cmp.Diff` (already in go.mod). Self-documenting golden files in version control. Easy regeneration after intentional changes.

### Pattern 2: Deterministic Test Fixtures for Golden Files

**What:** Use fixed timestamps and rely on SQLite auto-increment for deterministic golden file output.

**Key rules:**
- Never use `time.Now()` in golden file test setup
- Create entities in a deterministic order (auto-increment IDs are sequential from 1)
- Use a single `goldenTime` constant shared across all golden file tests
- Include enough entities to exercise edge cases (zero values, nil pointers, populated nested objects)

### Pattern 3: Separate CI Jobs for Independent Checks

**What:** Run linting and testing as separate parallel jobs.

**Why:**
- Lint failures surface faster (lint runs in ~30s, tests take longer)
- Test failures do not block lint results
- golangci-lint-action explicitly recommends running lint in its own job

### Pattern 4: Generated Code Exclusion via Headers + Paths

**What:** Use both golangci-lint's `generated: strict` mode AND explicit path exclusions for defense-in-depth.

**When:** Any project with code generation (ent, gqlgen, protobuf, etc.).

**Why:** The `generated: strict` mode is the primary mechanism. Path exclusions add a safety net in case a generated file lacks the standard header.

## Anti-Patterns to Avoid

### Anti-Pattern 1: Golden Files in Root testdata/

**What:** Placing golden files for pdbcompat in the root `testdata/` directory alongside sync fixtures.

**Why bad:** Violates Go convention. Creates confusion about which tests use which fixtures. The root `testdata/fixtures/` is already used by `internal/sync/integration_test.go` for sync testing.

**Instead:** `internal/pdbcompat/testdata/golden/` keeps golden files with the pdbcompat package.

### Anti-Pattern 2: Snapshot Testing Libraries for Simple JSON Comparison

**What:** Adding `goldie`, `cupaloy`, `go-snaps`, or similar libraries.

**Why bad:** Adds external dependencies for functionality that is ~20 lines of stdlib code. Project constraint MD-1 prefers stdlib.

**Instead:** A single `goldenTest()` helper function handles everything.

### Anti-Pattern 3: Coverage Enforcement Without Baseline

**What:** Adding `-coverprofile` and a minimum coverage threshold to CI before establishing what current coverage is.

**Why bad:** Arbitrary thresholds either block everything or provide no value.

**Instead:** First establish the CI pipeline without coverage. Measure baseline later. Decide whether to enforce after understanding the codebase.

### Anti-Pattern 4: Linting Generated Code

**What:** Running golangci-lint against ent-generated, entrest-generated, or gqlgen-generated code.

**Why bad:** Generated code has its own style. Fixes are overwritten on next generation. Dramatically increases lint time.

**Instead:** Use `generated: strict` and explicit path patterns.

### Anti-Pattern 5: Non-Deterministic Golden File Data

**What:** Using `time.Now()`, random values, or relying on map iteration order in golden file test setup.

**Why bad:** Golden files regenerated on different runs produce different content. Tests become flaky.

**Instead:** Fixed timestamps, deterministic entity creation order, and JSON formatting with consistent indentation.

## File System Layout After v1.2

```
.github/
    workflows/
        ci.yml                          NEW - CI pipeline
.golangci.yml                           NEW - Linter config
internal/pdbcompat/
    testdata/
        golden/
            net_list.json               NEW - Golden files
            net_detail.json             NEW
            ...                         NEW (~15-20 golden files)
    golden_test.go                      NEW - Golden file tests
    depth_test.go                       EXISTING (no changes)
    filter_test.go                      EXISTING (no changes)
    handler_test.go                     EXISTING (no changes)
    serializer_test.go                  EXISTING (no changes)
testdata/fixtures/                      EXISTING (sync fixtures, no changes)
```

## Suggested Build Order

### Phase 1: Lint Configuration and Fix Issues (FIRST)

1. Create `.golangci.yml` with generated code exclusions
2. Run `golangci-lint run` locally to identify existing issues
3. Fix all linter issues in hand-written code
4. Verify `golangci-lint run` passes clean

**Rationale:** Establishing linting first means all subsequent code (golden file tests) is written to pass the linter from the start.

### Phase 2: Golden File Tests (SECOND)

1. Create `internal/pdbcompat/golden_test.go` with helper functions
2. Create `setupGoldenTestHandler(t)` with deterministic data
3. Write golden file tests for core types (list + detail)
4. Write golden file tests for edge cases (depth, projection, errors, index)
5. Run with `-update` to generate golden files
6. Review generated golden files for correctness
7. Verify `go test -race ./internal/pdbcompat/` passes

**Rationale:** Golden files must exist before CI is enabled, otherwise the CI test job will fail on the first run.

### Phase 3: GitHub Actions CI Pipeline (LAST)

1. Create `.github/workflows/ci.yml`
2. Push and verify both jobs pass (lint, test)
3. Fix any CI-specific issues

**Rationale:** CI validates everything before it. If CI is added before golden files exist, the test job fails.

```
Phase 1: Lint config + fix issues  (no deps, establishes code quality baseline)
    |
    v
Phase 2: Golden file tests         (depends on Phase 1 for linter compliance)
    |
    v
Phase 3: CI pipeline               (depends on Phase 1 + 2 for green build)
```

## Sources

- [File-driven testing in Go (Eli Bendersky)](https://eli.thegreenplace.net/2022/file-driven-testing-in-go/) -- golden file pattern overview
- [Testing with golden files in Go](https://medium.com/soon-london/testing-with-golden-files-in-go-7fccc71c43d3) -- `-update` flag pattern
- [Go wiki TestComments](https://go.dev/wiki/TestComments) -- `cmp.Diff` recommendation
- [golangci-lint v2 Configuration File](https://golangci-lint.run/docs/configuration/file/) -- YAML config structure
- [golangci-lint v2 announcement](https://ldez.github.io/blog/2025/03/23/golangci-lint-v2/) -- v2 release, config migration
- [golangci-lint-action v9](https://github.com/golangci/golangci-lint-action) -- GitHub Action setup, version compatibility
- [actions/setup-go v6](https://github.com/actions/setup-go) -- Go version setup, auto-caching
- [actions/checkout v6](https://github.com/actions/checkout/releases) -- v6.0.2, Jan 2026
