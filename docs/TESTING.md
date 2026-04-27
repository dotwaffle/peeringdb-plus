<!-- generated-by: gsd-doc-writer -->
# Testing

PeeringDB Plus uses the Go standard `testing` package exclusively — no third-party test framework is
required. Tests follow the Go convention of living next to the code they exercise as `*_test.go`
files in the same package (or a `_test` sibling package for black-box tests). The project targets
Go 1.26+ and all tests must pass with the race detector enabled.

## Test Layout

Tests are co-located with source files: for any `foo.go`, tests live in `foo_test.go` in the same
directory. Test helpers shared across packages live under `internal/testutil/`.

Key test locations:

| Area | Location | Notes |
|------|----------|-------|
| Shared ent client helper | `internal/testutil/testutil.go` | `SetupClient`, `SetupClientWithDB` |
| Seed fixtures for ent | `internal/testutil/seed/seed.go` | `Full(tb, client)` — all 13 entity types |
| PeeringDB API fixtures | `testdata/fixtures/` | 13 JSON files, one per object type |
| Golden files (pdbcompat) | `internal/pdbcompat/testdata/golden/` | Per-type `list.json`, `detail.json` |
| Sync integration tests | `internal/sync/integration_test.go` | Uses `httptest.Server` + fixtures |
| Conformance tests | `internal/conformance/` | Structural JSON comparison |
| Phase 71 response-budget tests | `internal/pdbcompat/stream_integration_test.go` | `TestServeList_UnderBudgetStreams`, `TestServeList_OverBudget413` |
| Phase 72 parity tests | `internal/pdbcompat/parity/` | 6 category files + `harness_helpers_test.go` + `bench_test.go` |
| Phase 72 parity fixtures | `internal/testutil/parity/fixtures.go` | 5560 rows ported from upstream `pdb_api_test.py` |
| Fixture port tool | `cmd/pdb-fixture-port/` | Regenerates `internal/testutil/parity/fixtures.go` |
| Fuzz tests | `internal/pdbcompat/fuzz_test.go` | `FuzzFilterParser` |
| Benchmarks | `internal/pdbcompat/projection_bench_test.go`, `internal/pdbcompat/parity/bench_test.go` | `BenchmarkApplyFieldProjection`, `BenchmarkParity_*` |
| Live gated tests | `*_live_test.go` | Require `-peeringdb-live` flag |

Generated code under `ent/` and `gen/` is excluded from coverage (see
[Coverage](#coverage) below) and should not be tested directly — tests exercise the handlers and
services that consume the generated code.

## Running Tests

Run the full suite with the race detector (this is what CI runs):

```bash
go test -race ./...
```

Run a single package:

```bash
go test -race ./internal/sync/...
```

Run a single test by name:

```bash
go test -race -run TestFullSyncWithFixtures ./internal/sync/
```

Update golden files after an intentional serializer or handler change
(`internal/pdbcompat/golden_test.go` defines the `-update` flag):

```bash
go test ./internal/pdbcompat/ -update
```

Run benchmarks:

```bash
# Hand-written projection benchmark
go test -bench=. -benchmem ./internal/pdbcompat/

# Phase 72 parity benchmarks (b.Loop idiom; not run in CI per Phase 72 D-06)
go test -run=^$ -bench=BenchmarkParity -benchtime=5x -count=6 ./internal/pdbcompat/parity/
```

Run fuzz tests (stops on first panic; run explicitly per package):

```bash
go test -run=^$ -fuzz=FuzzFilterParser -fuzztime=30s ./internal/pdbcompat/
```

### CGO and the race detector

The race detector requires CGO. Local development and production builds use `CGO_ENABLED=0` because
`modernc.org/sqlite` is pure Go and needs no CGO. CI enables CGO **only** to run the race detector:

```bash
# CI step (see .github/workflows/ci.yml)
CGO_ENABLED=1 go test -race -coverprofile=coverage.out -coverpkg="..." ./...
```

On machines without a C toolchain, you can run tests without the race detector:

```bash
go test ./...
```

## Test Helpers

### `internal/testutil`

`SetupClient(tb)` and `SetupClientWithDB(tb)` in `internal/testutil/testutil.go` construct an
isolated ent client backed by an in-memory SQLite database (shared-cache mode with foreign keys
enabled). Each call gets a unique DSN (`file:test_N?mode=memory&cache=shared&_pragma=foreign_keys(1)`)
so tests that call `t.Parallel()` do not see each other's data. Both helpers accept `testing.TB`
so they work under `*testing.T` and `*testing.B` (the widening was established in Phase 72 Plan
72-05 to support the parity benchmarks). The ent client and, when returned, the raw `*sql.DB`,
are closed automatically via `t.Cleanup`.

```go
import "github.com/dotwaffle/peeringdb-plus/internal/testutil"

func TestSomething(t *testing.T) {
    t.Parallel()
    client := testutil.SetupClient(t)
    // client is ready to use; cleanup is automatic.
}

// When you need the raw *sql.DB (for example for the sync_status table):
func TestSyncStatus(t *testing.T) {
    t.Parallel()
    client, db := testutil.SetupClientWithDB(t)
    _ = client
    _ = db
}
```

### `internal/testutil/seed`

`seed.Full(tb, client)` in `internal/testutil/seed/seed.go` seeds one entity of each of the 13
PeeringDB types (plus a second Network and a campus-assigned Facility) with deterministic IDs and
a fixed `seed.Timestamp` of `2024-01-01T00:00:00Z`. It returns a `*seed.Result` whose fields hold
typed pointers to every entity created:

```go
import "github.com/dotwaffle/peeringdb-plus/internal/testutil/seed"

func TestNetworkLookup(t *testing.T) {
    t.Parallel()
    client := testutil.SetupClient(t)
    r := seed.Full(t, client)

    // r.Org, r.Network (ID=10, ASN=13335 "Cloudflare"), r.IX (ID=20),
    // r.Facility (ID=30), r.Campus (ID=40), r.Carrier (ID=50),
    // r.IxLan, r.IxPrefix, r.NetworkIxLan, r.NetworkFacility,
    // r.IxFacility, r.CarrierFacility, r.Poc, r.Network2, r.Facility2
    got, err := client.Network.Get(t.Context(), r.Network.ID)
    if err != nil { t.Fatal(err) }
    if got.Asn != 13335 { t.Errorf("unexpected ASN: %d", got.Asn) }
}
```

Deterministic IDs are important because golden tests, handler tests, and grpcserver tests all
assume the IDs and names produced by `seed.Full`. If you need a different shape, add a new
helper rather than mutating `Full`. Phase 72 parity tests deliberately do **not** use
`seed.Full` — they use the per-category helpers in `harness_helpers_test.go` to avoid
cross-test contamination (see [Phase 72 Parity Tests](#phase-72-parity-tests) below).

## Fixtures (`testdata/fixtures/`)

The `testdata/fixtures/` directory contains 13 JSON files — one per PeeringDB object type — that
match the actual PeeringDB API envelope (`{"meta": {...}, "data": [...]}`). The full list:

```
campus.json     carrier.json    carrierfac.json  fac.json
ix.json         ixfac.json      ixlan.json       ixpfx.json
net.json        netfac.json     netixlan.json    org.json
poc.json
```

These drive sync integration tests: `internal/sync/integration_test.go` spins up an
`httptest.Server` that serves each fixture when the sync worker requests the corresponding
`/api/{type}` path, then asserts on the resulting database state. The mock server returns the
fixture data on the first page (`skip=0`) and an empty array on subsequent pages to terminate
pagination.

### Writing a new sync integration test using a fixture

1. If the scenario needs a new record shape, edit the relevant JSON file in `testdata/fixtures/`
   (keep it matching the real PeeringDB envelope).
2. Write your test in `internal/sync/integration_test.go` (or a sibling `_test.go` in
   `package sync_test`). Re-use the existing `newFixtureServer(t)` helper to get a mock API
   server plus `testutil.SetupClientWithDB(t)` for an isolated database.
3. Build a `sync.Worker` with the mock server URL as the PeeringDB base:

    ```go
    fs := newFixtureServer(t)
    client, db := testutil.SetupClientWithDB(t)

    pdbClient := peeringdb.NewClient(fs.server.URL, slog.Default())
    pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
    pdbClient.SetRetryBaseDelay(0)

    if err := sync.InitStatusTable(t.Context(), db); err != nil {
        t.Fatalf("init status table: %v", err)
    }
    w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{}, slog.Default())

    if err := w.Sync(t.Context(), config.SyncModeFull); err != nil {
        t.Fatalf("sync failed: %v", err)
    }
    ```

4. Assert on the resulting database state with the ent client: row counts, specific field values,
   or join traversals. For per-fixture overrides, call `fs.setFixtureData(type, rawJSON)` before
   running the sync.

## Conformance Tests (`internal/conformance`)

`internal/conformance/` validates that PeeringDB Plus's JSON output is structurally compatible
with the real PeeringDB API. `conformance.CompareResponses` (and the lower-level `CompareStructure`)
compares field names, value types, null/array/object shapes, and nesting depth — not actual values.
`internal/conformance/compare_test.go` exercises the comparer itself; `live_test.go` compares a
live fetch against the golden files in `internal/pdbcompat/testdata/golden/`.

## Phase 71 Response Budget Tests

Phase 71 added a 128 MiB pre-flight memory budget to the pdbcompat list path. Two integration
tests in `internal/pdbcompat/stream_integration_test.go` lock the contract:

| Test | Asserts |
|------|---------|
| `TestServeList_UnderBudgetStreams` | An under-budget list streams a complete response with the expected row count and content type |
| `TestServeList_OverBudget413` | An over-budget request returns HTTP 413 (Payload Too Large) before any row is emitted |

When adding a new entity type to `internal/pdbcompat/registry_funcs.go`, extend
`internal/pdbcompat/budget_test.go` and the streaming integration tests with under-budget and
over-budget assertions mirroring the existing pattern. See `CLAUDE.md § Response memory envelope`
for the full maintainer checklist.

## Phase 72 Parity Tests

`internal/pdbcompat/parity/` locks the v1.16 pdbcompat semantics against future regression. The
package is split into 6 category-specific test files plus shared infrastructure:

| File | Entry test | Covers |
|------|------------|--------|
| `ordering_test.go` | `TestParity_Ordering` | ORDER REQ-IDs (sort key resolution, null handling) |
| `status_test.go` | `TestParity_Status` | STATUS REQ-IDs (Phase 68 status × since matrix, tombstone visibility) |
| `limit_test.go` | `TestParity_Limit` | LIMIT REQ-IDs (limit=0 streaming, max-cap, depth pairing) |
| `unicode_test.go` | `TestParity_Unicode` | UNICODE REQ-IDs (Phase 69 fold-column routing) |
| `in_test.go` | `TestParity_In` | IN REQ-IDs (large `__in` sets, empty-`__in` short-circuit) |
| `traversal_test.go` | `TestParity_Traversal` | TRAVERSAL REQ-IDs (Phase 70 1-hop and 2-hop traversal) |
| `harness_helpers_test.go` | (helpers only) | `seedFixtures(tb, client, fixtures)`, per-category seeders, request-builders |
| `bench_test.go` | `BenchmarkParity_*` | 3 perf envelopes (run locally, not gated in CI) |

### Fixtures (`internal/testutil/parity/fixtures.go`)

The parity tests are seeded from a generated file:

- 5560 rows ported from upstream `peeringdb/peeringdb` commit
  `99e92c726172ead7d224ce34c344eff0bccb3e63`, source path
  `src/peeringdb_server/management/commands/pdb_api_test.py`.
- Six exported category vars: `OrderingFixtures`, `StatusFixtures`, `LimitFixtures`,
  `UnicodeFixtures`, `InFixtures`, `TraversalFixtures`.
- The file header records the upstream commit SHA and the SHA256 of the source Python file so
  drift can be detected across regenerations.

Regenerate via `go generate ./internal/testutil/parity/`. The generator (`cmd/pdb-fixture-port/`)
is idempotent: two runs against the same upstream produce byte-identical output. It accepts
`--upstream-commit <sha>` to pin the SHA during snapshot-replay regeneration, and `--check` to
compare the current upstream against the pinned SHA (advisory only — does not block merges per
Phase 72 D-03).

### Conventions for parity tests

- **Isolation**: every parity test calls `testutil.SetupClient(tb)` for a fresh in-memory ent
  client. Do **not** reach into `internal/testutil/seed.Full` — it seeds a different shape and
  causes cross-test contamination (CONTEXT.md plan-hint).
- **Seeding**: use `seedFixtures(t, client, parity.<Cat>Fixtures)` or one of the targeted
  seeders in `harness_helpers_test.go` (e.g. seeding only the rows a single sub-test needs).
- **Parallelism**: every sub-test calls `t.Parallel()`.
- **Citation comments**: every sub-test carries one of:
  - `// upstream: pdb_api_test.py:<line>` — when the assertion mirrors an upstream test case.
  - `// synthesised: phase-<NN>-<context>` — when the semantic is v1.16-new and has no upstream
    counterpart (Phase 68 tombstones, Phase 69 folding, Phase 70 traversal, Phase 71 budgets).
- **Divergence prefix**: sub-tests whose names begin with `DIVERGENCE_` mark intentional
  non-parity outcomes. Each such test must have a matching row in `docs/API.md § Known
  Divergences` cross-referencing it.
- **TB widening**: parity helpers accept `testing.TB` (not `*testing.T`) so the same code paths
  run under benchmarks. Established in Phase 72 Plan 72-05 across the 9 helper functions.

### Benchmarks

`bench_test.go` defines three named envelopes using the modern `b.Loop()` idiom:

| Benchmark | Locks |
|-----------|-------|
| `BenchmarkParity_TwoHopTraversal` | Phase 70 cross-entity traversal performance |
| `BenchmarkParity_LimitZeroStreaming` | Phase 71 stream.go path latency |
| `BenchmarkParity_InFiveThousandElements` | Phase 69 D-05 large-`__in` planner cost |

Run locally:

```bash
# Quick sanity (1 iteration each)
go test -run=^$ -bench=BenchmarkParity -benchtime=1x ./internal/pdbcompat/parity/

# Statistical run (5 iterations × 6 samples for benchstat)
go test -run=^$ -bench=BenchmarkParity -benchtime=5x -count=6 ./internal/pdbcompat/parity/
```

Benchmarks are **not** gated in CI per Phase 72 CONTEXT.md D-06 (no benchstat threshold). They
exist to detect order-of-magnitude regressions during local development.

## Fuzz Tests

`internal/pdbcompat/fuzz_test.go` defines `FuzzFilterParser`, which feeds arbitrary `(key, value)`
pairs to `ParseFilters` to assert that the filter parser never panics on untrusted input. Errors
are acceptable; panics are failures. The seed corpus covers all five `FieldType` values (string,
int, bool, time, float) and known edge cases (empty key, unsupported operator, type conversion
error).

Run it with:

```bash
go test -run=^$ -fuzz=FuzzFilterParser -fuzztime=30s ./internal/pdbcompat/
```

## Live Tests (`-peeringdb-live` gate)

Tests that hit `https://beta.peeringdb.com` are gated behind a package-level
`-peeringdb-live` boolean flag and `t.Skip()` when it is not set, so they never run in CI. Two
such tests exist:

| Test | File | Purpose |
|------|------|---------|
| `TestLiveConformance` | `internal/conformance/live_test.go` | Fetches each type from beta and compares structure against golden files |
| `TestMetaGeneratedLive` | `internal/peeringdb/client_live_test.go` | Verifies `meta.generated` field presence across fetch patterns |

Run them locally (respect PeeringDB rate limits — the tests use a 3s sleep unauthenticated, 1s with
an API key):

```bash
# Unauthenticated (3s delay between requests)
go test -race ./internal/conformance/ -peeringdb-live

# With API key (1s delay)
PDBPLUS_PEERINGDB_API_KEY=... go test -race ./internal/peeringdb/ -peeringdb-live
```

These tests are intentionally excluded from CI because:

- They depend on an external service (beta.peeringdb.com) being reachable and healthy.
- They must be rate-limited to avoid abusing the upstream API.
- Their output is not deterministic (the live dataset changes).

## Conventions

### Table-driven tests

Subtests are table-driven where practical. The canonical shape used throughout the codebase:

```go
func TestParseBool(t *testing.T) {
    t.Parallel()
    tests := []struct {
        name    string
        input   string
        want    bool
        wantErr bool
    }{
        {"true", "true", true, false},
        {"false", "false", false, false},
        {"invalid", "nope", false, true},
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            t.Parallel()
            got, err := ParseBool(tc.input)
            if (err != nil) != tc.wantErr {
                t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
            }
            if got != tc.want {
                t.Errorf("got %v, want %v", got, tc.want)
            }
        })
    }
}
```

### Parallelism

Call `t.Parallel()` at the top of every test and subtest where safe (GO-T-3). `SetupClient`
constructs per-test isolated databases specifically to make `t.Parallel()` safe. The live
conformance test is deliberately **not** parallel because it must sequence requests to respect
upstream rate limits.

### `t.Cleanup`

Prefer `t.Cleanup(func() { ... })` over `defer` in helpers so teardown runs in the correct LIFO
order regardless of which test function the helper is called from. `testutil.SetupClient` already
registers cleanups for the ent client and raw `*sql.DB`.

### Context

Use `t.Context()` instead of `context.Background()` in tests — it is cancelled when the test
finishes, ensuring goroutines started by handlers or workers do not leak between tests.

### Naming

- Test files: `foo_test.go` (co-located).
- Test functions: `TestFoo`, `TestFoo_Subcase` or `TestFooSubcase`.
- Benchmarks: `BenchmarkFoo`.
- Fuzz tests: `FuzzFoo`.
- Live tests: `TestFooLive` in a `*_live_test.go` file, gated by the `-peeringdb-live` flag.
- Parity tests: `TestParity_<Category>` with sub-tests prefixed by REQ-ID
  (`ORDER_*`, `STATUS_*`, `LIMIT_*`, `UNICODE_*`, `IN_*`, `TRAVERSAL_*`); intentional
  non-parity uses `DIVERGENCE_<...>`.

## Coverage

There is no enforced coverage threshold — the `.octocov.yml` configuration records coverage for
reporting only. Generated code is excluded so the headline number reflects hand-written code:

```yaml
# .octocov.yml
coverage:
  paths:
    - coverage.out
  exclude:
    - 'ent/**/*.go'
    - 'gen/**/*.go'
    - 'graph/generated.go'
    - '**/*_templ.go'
```

In addition, the CI test step builds its `-coverpkg` list by excluding `ent/` and `gen/` at
`go list` time so that generated packages are not counted in either the numerator or the
denominator:

```bash
# .github/workflows/ci.yml (test job)
COVERPKG=$(go list ./... | grep -vE '/ent(/|$)|/gen(/|$)' | tr '\n' ',' | sed 's/,$//')
CGO_ENABLED=1 go test -race -coverprofile=coverage.out -coverpkg="${COVERPKG}" ./...
```

The `k1LoW/octocov-action` CI step posts the coverage summary as a PR comment.

## CI Integration

The `.github/workflows/ci.yml` workflow runs five jobs on every pull request and every push to
`main`:

| Job | Step | Command |
|-----|------|---------|
| `lint` | Lint | `golangci-lint run` |
| `lint` | Generated code drift check | `go generate ./...` then `git diff --exit-code -- ent/ gen/ graph/ internal/web/templates/` |
| `test` | Tests with race detector + coverage | `CGO_ENABLED=1 go test -race -coverprofile=coverage.out -coverpkg="${COVERPKG}" ./...` |
| `build` | Compile check | `go build ./...` |
| `govulncheck` | Vulnerability scan | `govulncheck ./...` |
| `docker-build` | Dev and prod image builds | `docker build` using `./Dockerfile` and `./Dockerfile.prod` |

Any test failure, race detection, coverage file write failure, or generated-code drift fails the
workflow.
