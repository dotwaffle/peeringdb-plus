# Testing

PeeringDB Plus tests use the Go `testing` package.
Some tests also use `github.com/google/go-cmp/cmp` for diffs,
and the `internal/mcpserver` tests use `github.com/stretchr/testify`.
Tests follow the Go convention of living next to the code they exercise
as `*_test.go` files in the same package
(or a `_test` sibling package for black-box tests).
The project targets Go 1.27.1
and all tests must pass with the race detector enabled.

## Test Layout

Tests are co-located with source files: for any `foo.go`,
tests live in `foo_test.go` in the same directory.
Test helpers shared across packages live under `internal/testutil/`.

Key test locations:

| Area | Location | Notes |
|------|----------|-------|
| Shared ent client helper | `internal/testutil/testutil.go` | `SetupClient`, `SetupClientWithDB` |
| Seed fixtures for ent | `internal/testutil/seed/seed.go` | `Full(tb, client)` — all 13 entity types |
| PeeringDB API fixtures | `testdata/fixtures/` | 13 JSON files, one per object type |
| Golden files (pdbcompat) | `internal/pdbcompat/testdata/golden/` | Per-type `list.json`, `detail.json`, `depth.json` |
| Golden file (sync) | `internal/sync/testdata/refactor_parity.golden.json` | `TestSync_RefactorParity` |
| Sync integration tests | `internal/sync/integration_test.go` | Uses `httptest.Server` + fixtures |
| Conformance tests | `internal/conformance/` | Structural JSON comparison |
| Response-budget tests | `internal/pdbcompat/stream_integration_test.go` | `TestServeList_UnderBudgetStreams`, `TestServeList_OverBudget413` |
| Parity tests | `internal/pdbcompat/parity/` | 9 category files, `harness_helpers_test.go`, `harness_test.go`, `bench_test.go`, and `doc.go`. Each sub-test seeds clean rows inline through the ent client. |
| Fuzz tests | `internal/pdbcompat/fuzz_test.go` | `FuzzFilterParser` |
| Benchmarks | `bench_test.go`, `bench_*_test.go`, and `*_bench_test.go` files in `internal/pdbcompat`, `internal/pdbcompat/parity`, `internal/grpcserver`, `internal/sync`, and `internal/web`. Also `internal/web/termrender/network_test.go`. | For example `BenchmarkApplyFieldProjection`, `BenchmarkRowSize`, `BenchmarkParity_*` |
| Live gated tests | `internal/conformance/live_test.go`, `internal/peeringdb/client_live_test.go` | Require the `-peeringdb-live` flag |

Generated code under `ent/` and `gen/` is excluded from coverage
(see [Coverage](#coverage) below)
and should not be tested directly —
tests exercise the handlers and services that consume the generated code.

## Running Tests

Run the full suite with the race detector:

```bash
mise run test
```

CI runs `mise run coverage`.
That task runs the same suite and also writes a coverage profile.

Run a single package:

```bash
go test -race ./internal/sync/...
```

Run a single test by name:

```bash
go test -race -run TestFullSyncWithFixtures ./internal/sync/
```

Update the golden files after an intentional serializer, handler,
or sync-mapping change.
`internal/pdbcompat/golden_test.go` and `internal/sync/worker_test.go`
each define an `-update` flag.
Put `-update` after the package path.
If it comes first, `go test` tests the current directory and fails.

```bash
go test ./internal/pdbcompat/ -run TestGoldenFiles -update
go test ./internal/sync/ -run TestSync_RefactorParity -update
```

Run benchmarks:

```bash
# Projection benchmark only
go test -run='^$' -bench=BenchmarkApplyFieldProjection -benchmem ./internal/pdbcompat/

# Parity benchmarks (not run in CI)
go test -run='^$' -bench=BenchmarkParity -benchtime=5x -count=6 ./internal/pdbcompat/parity/
```

The quotes around `^$` stop zsh with `extendedglob`
from reading it as a glob pattern.

Run fuzz tests (stops on first panic; run explicitly per package):

```bash
go test -run='^$' -fuzz=FuzzFilterParser -fuzztime=30s ./internal/pdbcompat/
```

### Cgo and the race detector

The race detector needs cgo and a C compiler.
`modernc.org/sqlite` is pure Go, so the server binary does not need cgo.
The Docker images build with `CGO_ENABLED=0`.
The `test` and `coverage` mise tasks set `CGO_ENABLED=1`.

If your machine has no C compiler, run the tests without the race detector:

```bash
go test ./...
```

## Test Helpers

### `internal/testutil`

`SetupClient(tb)` and `SetupClientWithDB(tb)` in `internal/testutil/testutil.go`
construct an isolated ent client backed by an in-memory SQLite database
(shared-cache mode with foreign keys enabled).
Each call gets a unique DSN
(`file:test_N?mode=memory&cache=shared&_pragma=foreign_keys(1)`) so tests that
call `t.Parallel()` do not see each other's data.
Both helpers accept `testing.TB` so they work under `*testing.T`
and `*testing.B` (the widening supports the parity benchmarks).
The ent client and, when returned, the raw `*sql.DB`,
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

`seed.Full(tb, client)` in `internal/testutil/seed/seed.go` creates one row
of each of the 13 types with fixed IDs
(org 1, net 10, ix 20, fac 30, campus 40, carrier 50).
It also creates these rows:

- a second network (ID 11) and a campus facility (ID 31)
- a second ixlan with a `Public` member-list URL
  (`seed.IxLanPublicID` = 101).
  The gated ixlan is `seed.IxLanGatedID` = 100.
- two POCs with `visible=Users` (IDs 9000 and 9001)
- a separate tenant in the 8001 ID band
  (org, campus, ix, ixlan, fac, and networks 8001-8003).
  Network 8003 has `status=deleted`.
  Traversal, fold, and status tests use this tenant.

All timestamps are `seed.Timestamp` (`2024-01-01T00:00:00Z`).
`Full` returns a `*seed.Result` whose fields hold typed pointers to the rows:

```go
import "github.com/dotwaffle/peeringdb-plus/internal/testutil/seed"

func TestNetworkLookup(t *testing.T) {
    t.Parallel()
    client := testutil.SetupClient(t)
    r := seed.Full(t, client)

    // r.Org, r.Network (ID=10, ASN=13335 "Cloudflare"), r.IX (ID=20),
    // r.Facility (ID=30), r.Campus (ID=40), r.Carrier (ID=50),
    // r.IxLan, r.IxPrefix, r.NetworkIxLan, r.NetworkFacility,
    // r.IxFacility, r.CarrierFacility, r.Poc, r.Network2, r.Facility2,
    // r.IxLanPublic, r.UsersPoc, r.UsersPoc2, r.AllPocs, r.AllNetworks
    got, err := client.Network.Get(t.Context(), r.Network.ID)
    if err != nil { t.Fatal(err) }
    if got.Asn != 13335 { t.Errorf("unexpected ASN: %d", got.Asn) }
}
```

Tests in `cmd/peeringdb-plus`, `graph`, `internal/pdbcompat`,
and `internal/sync` use the IDs and names that `seed.Full` creates.
If you need a different shape, add a new helper.
Do not change `Full`.
The golden tests (`setupGoldenTestData`), the `internal/grpcserver` tests,
and the `internal/web` tests seed their own rows.
Parity tests deliberately do **not** use `seed.Full` —
each sub-test seeds the clean rows it needs inline via the ent client to avoid
cross-test contamination (see [Parity Tests](#parity-tests) below).

## Fixtures (`testdata/fixtures/`)

The `testdata/fixtures/` directory contains 13 JSON files —
one per PeeringDB object type — that match the actual PeeringDB API envelope
(`{"meta": {...}, "data": [...]}`).
The full list:

```text
campus.json     carrier.json    carrierfac.json  fac.json
ix.json         ixfac.json      ixlan.json       ixpfx.json
net.json        netfac.json     netixlan.json    org.json
poc.json
```

These drive sync integration tests:
`internal/sync/integration_test.go` spins up an `httptest.Server`
that serves each fixture
when the sync worker requests the corresponding `/api/{type}` path,
then asserts on the resulting database state.
The mock server returns the fixture data on the first page (`skip=0`)
and an empty array on subsequent pages to terminate pagination.

### Writing a new sync integration test using a fixture

1. If the scenario needs a new record shape,
   edit the relevant JSON file in `testdata/fixtures/`
   (keep it matching the real PeeringDB envelope).
2. Write your test in `internal/sync/integration_test.go`
   (or a sibling `_test.go` in `package sync_test`).
   Re-use the existing `newFixtureServer(t)` helper to get a mock API server
   plus `testutil.SetupClientWithDB(t)` for an isolated database.
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

4. Assert on the resulting database state with the ent client: row counts,
   specific field values, or join traversals.
   For per-fixture overrides,
   call `fs.setFixtureData(type, rawJSON)` before running the sync.

## Conformance Tests (`internal/conformance`)

`internal/conformance/` validates
that PeeringDB Plus's JSON output is structurally compatible with the real
PeeringDB API.
`conformance.CompareResponses`
(and the lower-level `CompareStructure`)
compares field names, value types, null/array/object shapes, and nesting depth —
not actual values.
The per-object `meta` document on net and netixlan (PeeringDB 2.83.0) is opaque:
the comparer checks only its JSON type, not its keys,
because its keys differ per row.
The top-level envelope `meta` is compared in full.
`internal/conformance/compare_test.go` exercises the comparer itself;
`live_test.go` compares a live fetch against the golden files in
`internal/pdbcompat/testdata/golden/`.

## Response Budget Tests

The pdbcompat list path carries a 128 MiB pre-flight memory budget.
Two integration tests in `internal/pdbcompat/stream_integration_test.go` lock
the contract:

| Test | Asserts |
|------|---------|
| `TestServeList_UnderBudgetStreams` | An under-budget list streams a complete response with the expected row count and content type |
| `TestServeList_OverBudget413` | An over-budget request returns HTTP 413 (Payload Too Large) before any row is emitted |

When adding a new entity type to `internal/pdbcompat/registry_funcs.go`,
extend `internal/pdbcompat/budget_test.go`
and the streaming integration tests with under-budget
and over-budget assertions mirroring the existing pattern.
See [DEVELOPMENT.md § Adding a new pdbcompat entity](DEVELOPMENT.md#adding-a-new-pdbcompat-entity-response-memory-budget)
for the full checklist.

## Parity Tests

`internal/pdbcompat/parity/` locks the `/api/` behavior
that matches upstream PeeringDB.
A test fails when that behavior changes.
The package is split into 9 category-specific test files plus shared
infrastructure:

| File | Entry test | Covers |
|------|------------|--------|
| `ordering_test.go` | `TestParity_Ordering` | Default list order (`id` ascending), `?limit=`/`?skip=` pages, and the tie-break for rows with the same `updated` value in a `?since=` list |
| `status_test.go` | `TestParity_Status` | Status (status × since matrix, tombstone visibility) |
| `limit_test.go` | `TestParity_Limit` | `?limit=0` streaming and its 413 budget check, no upper cap on `?limit=`, `?depth=` on lists, bad `?limit=`/`?skip=` values |
| `unicode_test.go` | `TestParity_Unicode` | Unicode (fold-column routing) |
| `in_test.go` | `TestParity_In` | `__in` filters (large `__in` sets, empty-`__in` short-circuit) |
| `traversal_test.go` | `TestParity_Traversal` | Traversal (1-hop and 2-hop traversal) |
| `meta_test.go` | `TestParity_Meta` | netixlan `meta__*` filters (typed keys, absent key never matches, net keys ignored) |
| `serializer_test.go` | `TestParity_Serializer` | Serializer values and keys (IX-F URL key for permitted callers, `ix.media`/`ixlan.dot1q_support` constants, `info_types` as a list) |
| `multichoice_test.go` | `TestParity_MultiChoice` | Multi-value choice filters (net `info_types` and legacy `info_type`, fac `available_voltage_services`) |
| `harness_helpers_test.go` | (helpers only) | `newTestServer` / `newTestServerWithBudget`, `httpGet`, `decodeDataArray`, `extractIDs`, `mustDecodeProblem` (server wiring + response decoding; no seeders) |
| `harness_test.go` | `TestHarness_*` | Self-tests for the helpers |
| `bench_test.go` | `BenchmarkParity_*` | 3 perf envelopes (run locally, not gated in CI) |
| `doc.go` | (package doc) | Package documentation |

### Seeding

Each parity sub-test seeds the clean rows it needs inline via the ent client
(`c.Network.Create()...`) and cites the upstream source line it mirrors in a
comment.
There is no generated fixtures package and no per-category seeder —
the upstream test cases in `pdb_api_test.py` are transcribed directly into the
relevant sub-test, citing `// upstream: pdb_api_test.py:<line>`.

### Conventions for parity tests

- **Isolation**: every parity test calls `testutil.SetupClient(tb)`
  for a fresh in-memory ent client.
  Do **not** reach into `internal/testutil/seed.Full` —
  it seeds a different shape and causes cross-test contamination.
- **Seeding**: seed clean rows inline via the ent client
  (`c.Network.Create()...`), seeding only the rows a single sub-test needs.
- **Parallelism**: every sub-test calls `t.Parallel()`.
- **Citation comments**: every sub-test carries one of:
  - `// upstream: pdb_api_test.py:<line>` —
    when the assertion mirrors an upstream test case.
    A case from another upstream test file names that file,
    for example `// upstream: tests/test_meta_registry.py:<line>`.
  - `// synthesised: <context>` marks a behavior that no upstream test covers
    (for example tombstones, folding, traversal, or budgets).
- **Divergence prefix**: sub-tests whose names begin with `DIVERGENCE_` mark
  intentional non-parity outcomes.
  A divergence test that is its own top-level function ends its name
  with `_DIVERGENCE` (for example `TestParity_Unicode_FoldWindow_DIVERGENCE`).
  Each such test must have a matching row in `docs/API.md § Known Divergences`
  cross-referencing it.
- **TB widening**: parity helpers accept `testing.TB`
  (not `*testing.T`) so the same code paths run under benchmarks.
  Applied across the 6 helper functions in `harness_helpers_test.go`
  (`newTestServer`, `newTestServerWithBudget`, `httpGet`, `decodeDataArray`,
  `extractIDs`, `mustDecodeProblem`).

### Benchmarks

`bench_test.go` defines three named envelopes using the modern `b.Loop()` idiom:

| Benchmark | Locks |
|-----------|-------|
| `BenchmarkParity_TwoHopTraversal` | Cross-entity traversal performance |
| `BenchmarkParity_LimitZeroStreaming` | `stream.go` path latency |
| `BenchmarkParity_InFiveThousandElements` | Large-`__in` planner cost |

Run locally:

```bash
# Quick sanity (1 iteration each)
go test -run='^$' -bench=BenchmarkParity -benchtime=1x ./internal/pdbcompat/parity/

# Statistical run (5 iterations × 6 samples for benchstat)
go test -run='^$' -bench=BenchmarkParity -benchtime=5x -count=6 ./internal/pdbcompat/parity/
```

Benchmarks are **not** gated in CI (no benchstat threshold).
They exist to detect order-of-magnitude regressions during local development.

## Fuzz Tests

`internal/pdbcompat/fuzz_test.go` defines `FuzzFilterParser`,
which feeds arbitrary `(key, value)` pairs to `ParseFilters` to assert
that the filter parser never panics on untrusted input.
Errors are acceptable; panics are failures.
The seed corpus covers all six `FieldType` values
(string, int, bool, time, float, multichoice)
and known edge cases (empty key, unsupported operator, type conversion error).

Run it with:

```bash
go test -run='^$' -fuzz=FuzzFilterParser -fuzztime=30s ./internal/pdbcompat/
```

## Live Tests (`-peeringdb-live` gate)

Tests that hit `https://beta.peeringdb.com` are gated behind a package-level
`-peeringdb-live` boolean flag and `t.Skip()` when it is not set, so they never
run in CI.
Two such tests exist:

| Test | File | Purpose |
|------|------|---------|
| `TestLiveConformance` | `internal/conformance/live_test.go` | Fetches each type from beta and compares structure against golden files |
| `TestMetaGeneratedLive` | `internal/peeringdb/client_live_test.go` | Verifies `meta.generated` field presence across fetch patterns |

The tests wait between requests to stay under the PeeringDB rate limits.
`TestLiveConformance` always runs anonymously and waits 3 s between requests.
`TestMetaGeneratedLive` waits 3 s,
or 1 s when `PDBPLUS_PEERINGDB_API_KEY` is set.
Put `-peeringdb-live` after the package path:

```bash
# Anonymous, 3 s between requests
go test -race ./internal/conformance/ -run TestLiveConformance -peeringdb-live

# 1 s between requests with an API key, 3 s without
PDBPLUS_PEERINGDB_API_KEY=... go test -race ./internal/peeringdb/ -run TestMetaGeneratedLive -peeringdb-live
```

These tests are intentionally excluded from CI because:

- They depend on an external service (beta.peeringdb.com) being reachable
  and healthy.
- They must be rate-limited to avoid abusing the upstream API.
- Their output is not deterministic (the live dataset changes).

## Conventions

### Table-driven tests

Subtests are table-driven where practical.
The canonical shape used throughout the codebase:

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

Call `t.Parallel()` at the top of every test and subtest where safe.
`SetupClient` constructs per-test isolated databases specifically to make
`t.Parallel()` safe.
The live conformance test is deliberately **not** parallel
because it must sequence requests to respect upstream rate limits.

### `t.Cleanup`

Prefer `t.Cleanup(func() { ... })` over `defer` in helpers
so teardown runs in the correct LIFO order regardless of
which test function the helper is called from.
`testutil.SetupClient` already registers cleanups for the ent client
and raw `*sql.DB`.

### Context

Use `t.Context()` instead of `context.Background()` in tests —
it is cancelled when the test finishes,
ensuring goroutines started by handlers or workers do not leak between tests.

### Naming

- Test files: `foo_test.go` (co-located).
- Test functions: `TestFoo`, `TestFoo_Subcase` or `TestFooSubcase`.
- Benchmarks: `BenchmarkFoo`.
- Fuzz tests: `FuzzFoo`.
- Live tests: gate them with the package-level `-peeringdb-live` flag
  and call `t.Skip` when the flag is not set.
- Parity tests: one `TestParity_<Category>` entry function for each category
  (`TestParity_Ordering`, `TestParity_Status`, `TestParity_Limit`,
  `TestParity_Unicode`, `TestParity_In`, `TestParity_Traversal`,
  `TestParity_Meta`, `TestParity_Serializer`, `TestParity_MultiChoice`).
  Give each `t.Run` sub-test a descriptive snake_case name
  (e.g. `list_no_since_status_ok_only`).
  Start the name of an intentional non-parity sub-test with `DIVERGENCE_`
  (e.g. `DIVERGENCE_negative_limit_returns_400`).
  A divergence test that is its own top-level function ends its name
  with `_DIVERGENCE` (e.g. `TestParity_Unicode_FoldWindow_DIVERGENCE`).

## Coverage

There is no enforced coverage threshold —
the `.octocov.yml` configuration records coverage for reporting only.
Generated code is excluded so the headline number reflects hand-written code:

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

The coverage task names the hand-written package trees explicitly.
This avoids a shell-built `go list | grep | tr | sed` package list,
keeps generated `ent/` and `gen/` packages out of instrumentation,
and leaves file-level exclusions such as `graph/generated.go`
to `.octocov.yml`:

```bash
gotestsum -- \
  -race \
  -coverprofile=coverage.out \
  -coverpkg=./cmd/...,./deploy/...,./graph/...,./internal/...,./schema/... \
  ./...
```

Run this through `mise run coverage`;
gotestsum prints compact package-level progress, failure details,
and the 10 slowest tests.
The `k1LoW/octocov-action` CI step posts the resulting summary as a PR comment.

## CI Integration

The `.github/workflows/ci.yml` workflow runs two jobs on every pull request
and every push to `main`.
The `ci` job is a single cached Go job whose steps run in order,
each reusing the prior compile; `docker-build` runs in parallel:

| Job | Step (in order) | Command |
|-----|------|---------|
| `ci` | Install tools | `mise install --locked` via `jdx/mise-action` |
| `ci` | Generated code drift check | `mise run generate`, then scoped tracked/untracked checks |
| `ci` | Compile check | `mise run build` |
| `ci` | Tests with race detector + coverage | `mise run coverage` |
| `ci` | Lint | `mise run lint` |
| `ci` | Vulnerability scan (advisory, `continue-on-error`) | `mise run vulncheck` |
| `docker-build` | Dev and prod image builds | `docker build` using `./Dockerfile` and `./Dockerfile.prod` |

Any test failure, race detection, coverage file write failure,
or generated-code drift fails the workflow.
`govulncheck` is advisory:
a flagged vulnerability warns but does not block the merge.
