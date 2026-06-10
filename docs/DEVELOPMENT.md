# Development

This guide is for contributors making code changes to PeeringDB Plus. For a
high-level overview of the system, see [ARCHITECTURE.md](ARCHITECTURE.md). For
environment variables and runtime configuration, see
[CONFIGURATION.md](CONFIGURATION.md).

## Prerequisites

- **Go 1.26+** (declared in `go.mod` as `go 1.26.1`).
- Git and a local clone of this repository.

No external install is required for code generation. `buf`, `templ`, and
`gqlgen` are declared as Go tool dependencies in `go.mod` and are invoked via
`go tool <name>`.

Pure-Go SQLite is provided by `modernc.org/sqlite` — **no CGO is required** for
normal builds. CGO is only enabled in CI to run the race detector.

## Local setup

```bash
git clone https://github.com/dotwaffle/peeringdb-plus
cd peeringdb-plus
go build ./...
```

There is no live-reload tooling. After a code change, rebuild and restart the
binary manually:

```bash
go build -o peeringdb-plus ./cmd/peeringdb-plus && ./peeringdb-plus
```

The server serves all five API surfaces on a single port (`:8080` by default).

## Project layout

See [ARCHITECTURE.md](ARCHITECTURE.md) for the full component diagram and
package rationale. Code-change-relevant directories:

| Path | Purpose |
|---|---|
| `cmd/peeringdb-plus/` | Main binary — config load, server wiring, handler registration |
| `cmd/pdb-schema-extract/` | Parses the PeeringDB Django source into `schema/peeringdb.json` |
| `cmd/pdb-schema-generate/` | Generates `ent/schema/*.go` from `schema/peeringdb.json` |
| `cmd/pdb-compat-allowlist/` | Codegens `internal/pdbcompat/allowlist_gen.go` from `ent/schema/pdb_allowlists.go` |
| `cmd/pdbcompat-check/` | Validates PeeringDB API compatibility |
| `ent/schema/` | **Hand-edited** ent schemas — see "Sibling-file convention" |
| `ent/` | Generated ent client code (do not edit) |
| `proto/peeringdb/v1/` | Proto sources: `v1.proto` (generated), `services.proto` + `common.proto` (hand-written) |
| `gen/peeringdb/v1/` | Generated protobuf Go types and ConnectRPC interfaces |
| `graph/` | GraphQL resolver glue (`schema.resolvers.go`, `custom.resolvers.go` are hand-edited) |
| `internal/grpcserver/` | ConnectRPC handler implementations (one file per entity) |
| `internal/pdbcompat/` | PeeringDB-compatible API layer (`/api/`) |
| `internal/pdbcompat/parity/` | Upstream-parity regression suite |
| `internal/privctx/` | Privacy tier in request context |
| `internal/privfield/` | Field-level redaction single source of truth |
| `internal/unifold/` | Diacritic-insensitive folding |
| `internal/web/` | Web UI handlers, search, compare, query helpers |
| `internal/web/templates/` | `.templ` source + generated `*_templ.go` |
| `internal/sync/` | PeeringDB sync worker |
| `internal/testutil/` | `SetupClient(t)` and `seed/` helpers for tests |
| `schema/` | Intermediate `peeringdb.json` + schema-extract wiring |
| `testdata/fixtures/` | JSON fixtures for 13 PeeringDB entity types, used by sync tests |

## Build commands

| Command | Description |
|---|---|
| `go build ./...` | Build all packages; verifies compilation |
| `go test -race ./...` | Run all tests with the race detector (CI sets `CGO_ENABLED=1`) |
| `go generate ./...` | Run the full codegen pipeline in order |
| `gofmt -s -w .` | Format all Go files (also enforced by `golangci-lint`) |
| `go vet ./...` | Standard vet |
| `golangci-lint run` | Lint with project config (`.golangci.yml`) |
| `govulncheck ./...` | Vulnerability check against the Go vulnerability database |

## Code generation pipeline

PeeringDB Plus is heavily code-generated. A single `go generate ./...` invocation
runs every stage in the correct order and converges in a single pass. The
`go:generate` directives live in three files:

1. `ent/generate.go` — four sequenced directives:
   1. `cd ../schema && go run ../cmd/pdb-schema-generate/main.go peeringdb.json ../ent/schema`
      — regenerates `ent/schema/{type}.go` from `schema/peeringdb.json`. Runs
      **first**, before entc consumes those schemas.
   2. `go run -mod=mod entc.go` — runs ent + entgql + entrest + entproto.
   3. `cd .. && go run ./cmd/pdb-compat-allowlist` — emits
      `internal/pdbcompat/allowlist_gen.go` from `ent/schema/pdb_allowlists.go`.
   4. `cd .. && go tool buf generate` — emits protobuf Go types and ConnectRPC
      handler interfaces from the proto sources.
2. `graph/generate.go` — runs `go tool gqlgen generate` to produce the GraphQL
   resolvers and models from `graph/schema.graphqls` + `graph/gqlgen.yml`.
3. `internal/web/templates/generate.go` — runs `go tool templ generate` to
   regenerate `*_templ.go` from `.templ` sources.

`schema/generate.go` carries no `go:generate` directive; it only documents the
manual `pdb-schema-extract` step. Sequencing `pdb-schema-generate` **first**
within `ent/generate.go` — rather than under `schema/`, which `go generate ./...`
visits after `ent/` — is what lets a single pass converge: the schema producer
always runs before entc, its consumer.

A clean tree must produce zero drift after `go generate ./...`. CI enforces
this — see "Generated code drift check" below.

### Sibling-file convention (load-bearing)

`cmd/pdb-schema-generate` regenerates `ent/schema/{type}.go` from
`schema/peeringdb.json` on every `go generate ./...` run. **Anything
hand-edited in those files is silently stripped.** The fix is architectural:
keep hand-edits in **sibling files the generator never touches**.

The generator only writes files named after the model type (e.g.
`network.go`); any sibling with an additional `_suffix` is invisible to it.
ent's codegen still discovers methods on the schema type via reflection — the
file split is transparent to ent.

Today's sibling files:

| File | Purpose |
|---|---|
| `ent/schema/poc_policy.go` | `(Poc).Policy()` privacy rule |
| `ent/schema/fold_mixin.go` | The `foldMixin` Mixin implementation |
| `ent/schema/{type}_fold.go` | Per-entity `Mixin()` wiring for the 6 folded types: `campus`, `carrier`, `facility`, `internetexchange`, `network`, `organization` |
| `ent/schema/pdb_allowlists.go` | `PrepareQueryAllows` map consumed by `cmd/pdb-compat-allowlist` |

**If you add new hand-edited methods (`Hooks`, `Policy`, `Annotations`,
`Edges`, `Mixin`) to any generated `{type}.go` schema file, MOVE them to a
sibling named `{type}_{method}.go` instead.** This is the only mechanism that
prevents the schema generator from undoing your work.

### The `campus` inflection patch

`go-openapi/inflect` singularises `campus` → `campu` (and plural handling is
equally broken). Without a fix, ent generates `Campu`-themed code and entrest
produces `/campu` URL paths.

`ent/entc.go` patches this in two places via `go:linkname`:

1. The global `inflect` default ruleset — used by `entrest.Pluralize` for URL
   paths.
2. Ent's internal, unexported `gen.rules` ruleset — used by
   `Edge.MutationAdd`/`Remove`, graph column naming, and template funcs that
   feed both Go code and templates.

`AddIrregular("campus", "campuses")` alone is not enough — it only adds the
plural→singular mapping, so the bare word `campus` still falls through to the
default `s` → `∅` rule. The patch explicitly calls `AddSingular("campus",
"campus")` plus PascalCase `AddSingularExact`/`AddPluralExact` entries for
entrest (which passes type names directly without case folding).

If you see `Campu`, `campue`, or `/campuses/campu/` anywhere in generated
output, the patch is broken — check `ent/entc.go`.

### ent schema flow

```
PeeringDB Django source
    │  (cmd/pdb-schema-extract, needs PEERINGDB_REPO_PATH)
    ▼
schema/peeringdb.json   ← committed, canonical
    │  (cmd/pdb-schema-generate, sequenced first in ent/generate.go)
    ▼
ent/schema/{type}.go    ← regenerated; do NOT hand-edit (use siblings)
ent/schema/{type}_*.go  ← sibling files: hand-edited, never touched by generator
    │  (go generate ./ent → entc.go)
    ▼
ent/, graph/, proto/peeringdb/v1/v1.proto, REST handlers, GraphQL schema
    │  (cmd/pdb-compat-allowlist)
    ▼
internal/pdbcompat/allowlist_gen.go
```

### proto / buf workflow

- `proto/peeringdb/v1/v1.proto` — **generated** by entproto from ent schemas.
  Do not hand-edit; changes come from `ent/schema/*.go`.
- `proto/peeringdb/v1/services.proto` — **hand-written**. Defines the
  `Get*` / `List*` / `Stream*` RPCs and their request/response messages.
- `proto/peeringdb/v1/common.proto` — **hand-written**. Manual types like
  `SocialMedia` that don't map cleanly to an ent field.

`buf generate` (invoked via `go tool buf generate` by `ent/generate.go`) reads
`buf.gen.yaml` and produces:

- `gen/peeringdb/v1/*.pb.go` (protobuf Go types via `protoc-gen-go`).
- `gen/peeringdb/v1/peeringdbv1connect/*.go` (handler interfaces via
  `protoc-gen-connect-go`).

Proto `optional` fields generate Go pointer types (`*int64`, `*string`). Always
check `!= nil` for presence before dereferencing.

Note: proto is **frozen** since v1.6 (`entproto.SkipGenFile` in `ent/entc.go`),
so dropped ent fields whose proto wrappers still exist remain declared in
`v1.proto` but serialise as zero-value pointers (absent on the wire).

## Common dev loop

The typical inner loop depends on what you changed.

**Pure Go code (no schema, no proto, no templ):**

```bash
go vet ./...
go test -race ./...
golangci-lint run
```

**Edited `ent/schema/*.go`:**

```bash
go generate ./ent   # regenerates ent/, graph/, gen/, proto/peeringdb/v1/v1.proto, allowlist_gen.go
go vet ./...
go test -race ./...
```

**Edited `proto/peeringdb/v1/services.proto` or `common.proto`:**

```bash
go tool buf generate
go vet ./...
go test -race ./...
```

**Edited a `.templ` file:**

```bash
go generate ./internal/web/templates
# or: go tool templ generate
go test -race ./internal/web/...
```

**Edited anything that might ripple:**

```bash
go generate ./...
go vet ./...
go test -race ./...
golangci-lint run
```

Always commit `*_templ.go` alongside `.templ` changes, and commit generated
`ent/`, `gen/`, `graph/`, `internal/pdbcompat/allowlist_gen.go` files alongside
the schema changes that produced them. CI enforces this — see "Generated code
drift check" below.

## Adding a new ent field

Do **NOT** add the field to the `Fields()` slice in `ent/schema/{type}.go` —
those files are regenerated wholesale from `schema/peeringdb.json` by
`cmd/pdb-schema-generate` on every `go generate ./...` run, so a hand-added
field is silently stripped before entc ever sees it (see "Sibling-file
convention" above). Add the field at its source instead:

1. Pick the path that matches the field's origin:
   - **Upstream-derived field** (mirrors a PeeringDB API field): add a field
     object to the entity's `fields` map in `schema/peeringdb.json`, copying
     the shape of an existing entry (`type`, `required`, `nullable`,
     `help_text`, `default`, …). The generator turns it into the ent field
     definition.
   - **peeringdb-plus-local field** (server-side only, like the `_fold`
     shadow columns): declare it in a sibling-file `Mixin()` that the
     generator never touches, following the `ent/schema/{type}_fold.go` +
     `ent/schema/fold_mixin.go` pattern.
2. If the field should appear in ConnectRPC filters, also update the
   corresponding `proto/peeringdb/v1/services.proto` `List*Request` message
   (add an `optional string new_field = N;`).
3. Regenerate:
   ```bash
   go generate ./...
   ```
   This rewrites `ent/schema/{type}.go` from `schema/peeringdb.json` first,
   then updates `ent/`, `graph/`, `proto/peeringdb/v1/v1.proto`, the REST
   OpenAPI spec, and `internal/pdbcompat/allowlist_gen.go` in a single pass.
4. Extend the ConnectRPC filter table in `internal/grpcserver/<entity>.go` so
   the new field is honoured at query time. See the `networkListFilters` slice
   in `internal/grpcserver/network.go` for the pattern.
5. Update `internal/sync/upsert.go` mapping if the field is populated from the
   PeeringDB upstream response.
6. Add a test case to `internal/grpcserver/` and `internal/testutil/seed/seed.go`
   if the field is used by seed data.
7. Commit all regenerated files together.

## Schema hygiene drop procedure

`migrate.WithDropColumn(true)` and `migrate.WithDropIndex(true)` are
permanently on (`cmd/peeringdb-plus/main.go`). Dropping a field is therefore a
checklist, not a migration script:

1. Edit `schema/peeringdb.json` (remove the field from the upstream-derived
   schema), OR remove it from its sibling-file Mixin (e.g.
   `ent/schema/{type}_fold.go`) if it is peeringdb-plus-local — never from
   the generated `ent/schema/{type}.go` itself.
2. Run `go generate ./...`.
3. Remove references in:
   - `internal/peeringdb/types.go` (PeeringDB API client types)
   - `internal/pdbcompat/*` (compat-layer serializer, registry, filters)
   - `internal/grpcserver/*` (ConnectRPC handlers and filter tables)
   - `internal/sync/upsert.go` (sync mapping)
4. Regenerate goldens:
   ```bash
   go test -update ./internal/pdbcompat -run TestGoldenFiles
   go test -update ./internal/sync -run TestSync_RefactorParity
   ```
5. Deploy. Primary emits `ALTER TABLE DROP COLUMN`; LiteFS replicates the
   schema change to all replicas.

## Adding a new ConnectRPC service

The 13 PeeringDB entity types already each have their own ConnectRPC service,
so the common case is **adding a new RPC** to an existing service rather than
a whole new service.

**New RPC on an existing service:**

1. Edit `proto/peeringdb/v1/services.proto` — add the `rpc` line inside the
   service block and any new request/response messages.
2. Run `go tool buf generate` (or `go generate ./ent`).
3. Implement the method on the corresponding struct in
   `internal/grpcserver/<entity>.go`. The handler interface it must satisfy is
   regenerated in `gen/peeringdb/v1/peeringdbv1connect/`.
4. Add tests in `internal/grpcserver/grpcserver_test.go` (or a new
   `<entity>_test.go` file) using `testutil.SetupClient(t)` + `seed.Full(t, c)`.

**Brand-new service:**

1. Add a new `service` block to `services.proto`.
2. Regenerate.
3. Create `internal/grpcserver/<newentity>.go` with a struct that holds
   `Client *ent.Client` and `StreamTimeout time.Duration`, and implements the
   generated handler interface.
4. Register it in `cmd/peeringdb-plus/main.go` — add it to the `serviceNames`
   slice and add a `registerService(...)` call alongside the existing 13.

All handlers are wrapped with `otelconnect` interceptors automatically via
`handlerOpts`; no extra wiring needed.

## Adding a new web handler

The web UI uses a single wildcard `GET /ui/{rest...}` route that internally
dispatches by path (see `internal/web/handler.go` `Handler.dispatch`).

1. Add a `.templ` file to `internal/web/templates/` for the new page. Follow
   the pattern of `detail_net.templ`, `compare.templ`, etc.
2. Run `go generate ./internal/web/templates` (or `go tool templ generate`).
3. Add a handler method to `internal/web/` (e.g. `handleNewPage` in
   `internal/web/handler.go` or a new `page_newthing.go`).
4. Add a `case` arm to the `switch` in `Handler.dispatch` routing the new URL
   sub-path to your handler.
5. If the page needs database queries, put them in a `query_*.go` file for
   reuse.
6. Write a test using `httptest` + `testutil.SetupClient(t)` + `seed.Full`.

Static assets (CSS, images, favicon) live under `internal/web/static/` and are
served from an embedded filesystem at `/static/`.

## Adding a new field-level-privacy gated field

When a new PeeringDB field gains a `<field>_visible` companion (or you
introduce a new auth-gated column), redaction must be applied at **every**
serializer surface or you have a privacy leak. `internal/privfield.Redact`
is the single source of truth — never hand-roll a tier check.

**Developer checklist:**

1. **Schema** — add the ent fields. Use `field.String` (not `Enum`) for the
   `_visible` column; the value field gets `,omitempty` on its JSON struct
   tag so absence on the wire is automatic.
2. **Sync mapping** — populate both fields in `internal/sync/upsert.go`.
3. **Call `privfield.Redact` at all 5 surfaces.** Missing any one = privacy
   leak:
   - **pdbcompat** — `internal/pdbcompat/serializer.go` in the relevant
     `<entity>FromEnt(ctx, e)` function.
   - **ConnectRPC** — `internal/grpcserver/<entity>.go` in the proto
     conversion function. Wrap the closure passed to the generic pagination
     helper so `ctx` is captured (the helper signature stays
     `Convert func(*E) *P`).
   - **GraphQL** — opt the field into a custom resolver via `graph/gqlgen.yml`,
     then return `nil` from the resolver in `graph/schema.resolvers.go` when
     `omit=true`.
   - **entrest** — extend `restFieldRedactMiddleware` in
     `cmd/peeringdb-plus/main.go` to buffer the response body, parse JSON,
     and delete the redacted key when `omit=true`. Wrap **inside**
     `restErrorMiddleware` so `application/problem+json` error bodies pass
     through untouched.
   - **Web UI** — call `privfield.Redact` in the template data-prep step if
     the field has any render path.
4. **Seed both rows** — extend `internal/testutil/seed.Full` to seed BOTH a
   gated row (`_visible=Users`) AND a `Public` row, so E2E tests can assert
   the helper does not over-redact.
5. **E2E tests** — extend `cmd/peeringdb-plus/field_privacy_e2e_test.go` with
   `Redacted{Anon,UsersTier}` sub-tests plus a
   `fail-closed-bypass-middleware` assertion against the ConnectRPC handler
   directly.

The `_visible` companion field itself is **always** emitted (even for anon
callers) — this matches upstream PeeringDB's behaviour. Do not strip it.

## Adding a new searchable text field on a folded entity

The 6 folded entities (`organization`, `network`, `facility`,
`internetexchange`, `carrier`, `campus`) carry `<field>_fold` shadow columns
populated by `internal/unifold.Fold` to give pdbcompat diacritic-insensitive
filtering parity with upstream's `unidecode.unidecode(v)`.

To add a new searchable text field on one of these entities (e.g. a future
`network.tagline_fold`):

1. **Sibling file** — extend the `fields` slice in
   `ent/schema/{type}_fold.go`:
   ```go
   func (Network) Mixin() []ent.Mixin {
       return []ent.Mixin{foldMixin{fields: []string{"name", "aka", "name_long", "tagline"}}}
   }
   ```
   Do NOT edit `ent/schema/network.go` directly — `pdb-schema-generate` will
   strip your changes.
2. **Regenerate**: `go generate ./...`. The mixin emits the `_fold` column
   with the required `entgql.Skip(SkipAll) + entrest.WithSkip(true)`
   annotations automatically.
3. **Sync upsert** — extend `internal/sync/upsert.go` `upsertNetworks` builder
   chain with `.SetTaglineFold(unifold.Fold(n.Tagline))`. Place the new setter
   in the trailing `_fold` block per the existing convention.
4. **Filter routing** — add `"tagline": true` to `network`'s `FoldedFields`
   map in `internal/pdbcompat/registry.go`. The filter layer reads this map
   to decide whether to route to the shadow column.
5. **Round-trip test** — extend `internal/pdbcompat/fold_filter_test.go`
   with a test proving `?tagline__contains=<ascii>` matches a
   `tagline="<diacritic>"` row.

To add shadow columns on a 7th entity, create a new sibling
`ent/schema/{type}_fold.go` with the `foldMixin` wiring, then steps 3-5 above
plus add the `FoldedFields` map for the entity in `registry.go`. The other 6
upsert functions stay untouched — per-entity surgery is the convention.

## Adding a pdbcompat 1-hop or 2-hop traversal filter

Cross-entity filter keys (e.g. `?net__asn=64500`) are gated by an allowlist
codegened from `ent/schema/pdb_allowlists.go`. Two-step:

1. Edit `ent/schema/pdb_allowlists.go` and add the new key to the relevant
   entry's `Fields` slice. Carry a `// Source: serializers.py:<line>` comment
   for audit. The sibling-file location is load-bearing —
   `cmd/pdb-schema-generate` does not touch sibling files.
2. `go generate ./...` — regenerates `internal/pdbcompat/allowlist_gen.go` via
   `cmd/pdb-compat-allowlist`. Codegen routes 3-segment keys (e.g.
   `first__second__field`) into `AllowlistEntry.Via` automatically.

To exclude an edge from traversal entirely, attach
`pdbcompat.WithFilterExcludeFromTraversal()` to the edge definition.

**Do NOT:**

- Hand-edit `internal/pdbcompat/allowlist_gen.go` — it is overwritten on
  every codegen run; the CI drift check catches stale output.
- Add traversal allowlists to grpcserver / entrest / GraphQL — those
  surfaces have their own filter models and traversal is out of scope.
- Add 3+-hop keys — they're dropped by codegen sanity checks AND by the
  2-hop cap in `parseFieldOp` at request time.

## Adding a new pdbcompat entity (response memory budget)

The pdbcompat list path enforces a `PDBPLUS_RESPONSE_MEMORY_LIMIT` budget
(default 128 MiB) via a pre-flight `count × typicalRowBytes` check that
short-circuits to HTTP 413 when the request would exceed the budget. Any new
entity wired into `/api/` must integrate with this budget:

1. **Row-size table** — add a `typicalRowBytes` entry in
   `internal/pdbcompat/rowsize.go` with `Depth0` + `Depth2` numbers. Compute
   them by running:
   ```bash
   go test -run=NONE -bench=BenchmarkRowSize ./internal/pdbcompat -benchtime=20x -count=3
   ```
   against a seeded fixture, doubling the measured mean, and rounding **up**
   to the nearest 64 bytes.
2. **Paired ListFunc / CountFunc** — in `internal/pdbcompat/registry_funcs.go`,
   pair a `ListFunc` closure with a sibling `CountFunc` closure via a shared
   `<entity>Predicates` local helper. The two closures **must never diverge** —
   if they do, the budget check and the served response become different
   queries and the 413 guarantee breaks. The shared helper preserves the
   `applyStatusMatrix(isCampus, opts.Since != nil)` last-predicate
   invariant and the `EmptyResult` short-circuit.
3. **Architecture doc** — add a row to the per-entity sizing table in
   `docs/ARCHITECTURE.md § Response Memory Envelope` with the computed
   `max_rows @ 128 MiB`.
4. **E2E coverage** — extend `cmd/peeringdb-plus/*_e2e_test.go` (or
   `internal/pdbcompat/stream_integration_test.go`) with an under-budget
   smoke test and an over-budget 413 assertion mirroring
   `TestServeList_UnderBudgetStreams` / `TestServeList_OverBudget413`.

`memStatsHeapInuseBytes` in `internal/pdbcompat/telemetry.go` is the **single
call site** for `runtime.ReadMemStats`. Do not call it elsewhere — STW cost
compounds, and the single-call-site invariant is grep-enforceable.

## Testing

### Test helpers

- **`testutil.SetupClient(t)`** — creates an isolated in-memory SQLite ent
  client with foreign keys enabled. Each call uses a unique shared-cache DSN
  (`file:test_N?mode=memory&cache=shared&_pragma=foreign_keys(1)`) so parallel
  tests do not collide. Cleanup is wired via `t.Cleanup`.
- **`testutil.SetupClientWithDB(t)`** — same as above but also returns the
  raw `*sql.DB` handle, for tests that need to touch tables ent doesn't model
  (e.g. `sync_status`).
- **`testutil/seed.Full(tb, client)`** — seeds one of every PeeringDB entity
  type (13 total) plus a second `Network` and a campus-assigned `Facility`,
  with deterministic IDs and timestamps (`seed.Timestamp` = 2024-01-01 UTC).
  Returns a `*seed.Result` with typed references (`r.Org`, `r.Network`, etc.)
  for assertions.

### Running tests

```bash
go test -race ./...                                # all tests
go test -race ./internal/grpcserver/...            # a specific package
go test -race -run TestNetworkService_Get ./internal/grpcserver/
go test -race -bench=. ./internal/grpcserver/      # benchmarks
```

### Fixtures

`testdata/fixtures/` contains 13 JSON files, one per PeeringDB entity type,
used by sync integration tests. They match the PeeringDB API response shape.

### Live PeeringDB tests

Tests that hit `beta.peeringdb.com` are gated behind the `-peeringdb-live`
flag and are **not** run by `go test ./...` by default or in CI:

```bash
go test -peeringdb-live ./internal/peeringdb/
go test -peeringdb-live ./internal/conformance/
```

Use these to verify compatibility after a PeeringDB upstream change.

### Adding a parity test

`internal/pdbcompat/parity/` locks v1.16 pdbcompat semantics against future
regression. The suite is split across 6 category test files:

| File | Category |
|---|---|
| `ordering_test.go` | `?order_by=` semantics |
| `status_test.go` | status × since matrix |
| `limit_test.go` | `?limit=`, `?skip=`, streaming pre-flight |
| `unicode_test.go` | diacritic folding |
| `in_test.go` | `?field__in=v1,v2,...` |
| `traversal_test.go` | cross-entity `__` filters |

To add a new parity test:

1. **Pick the matching file by category** (ordering → `ordering_test.go`,
   etc.).
2. **Add a sub-test under `TestParity_<Category>`**:
   ```go
   t.Run("descriptive_name", func(t *testing.T) {
       t.Parallel()
       // upstream: pdb_api_test.py:1234
       // ...
   })
   ```
   Every parity sub-test gets `t.Parallel()` and an upstream citation marker
   (either `// upstream: pdb_api_test.py:<line>` or
   `// synthesised: <context>` for v1.16-new semantics with no
   upstream counterpart).
3. **Seed clean rows inline via the ent client** (`c.Network.Create()...`,
   etc.), keeping each sub-test to only the rows it needs. Do NOT reach
   into `internal/testutil/seed.Full` — cross-test contamination causes
   flakes. Each parity test gets its own isolated ent client via
   `testutil.SetupClient(tb)`. The shared helpers in
   `harness_helpers_test.go` (`newTestServer` /
   `newTestServerWithBudget`, `httpGet`, `decodeDataArray`,
   `extractIDs`, `mustDecodeProblem`) handle server wiring and response
   decoding — there are no per-category seeders.
4. **For divergences from upstream**: prefix the sub-test name with
   `DIVERGENCE_`, then append a `§ Known Divergences` row in
   `docs/API.md` cross-referencing the sub-test.

## Code style

- **`gofmt -s`** is mandatory and enforced by `golangci-lint` (`gofmt`
  formatter is part of the default linter set).
- **`go vet ./...`** is mandatory.
- **`golangci-lint run`** uses `.golangci.yml`:
  - Default linter set: `standard`.
  - Additionally enabled: `contextcheck`, `exhaustive`, `gocritic`, `gosec`,
    `misspell`, `modernize`, `nolintlint`, `revive`.
  - Generated code is excluded via `exclusions.generated: strict`.
  - `_test.go` files and the schema-generation/compat-check binaries
    (`cmd/pdb-schema-extract`, `cmd/pdb-schema-generate`,
    `cmd/pdbcompat-check`) are exempt from `gosec` (they shell out and write
    files by design).
- See the global Go guidelines (in user/org docs) for wrapping errors,
  context propagation, table-driven tests, and structured logging
  conventions.

## Branch conventions

The default and only long-lived branch is `main`. Feature work is done on
short-lived topic branches and merged via PR. No explicit branch-name
convention is documented in the repo; match existing patterns in
`git log --oneline` if in doubt.

## PR process

1. Run the full local check before pushing:
   ```bash
   go generate ./...
   go vet ./...
   go test -race ./...
   golangci-lint run
   govulncheck ./...
   ```
2. Commit any regenerated files (`ent/`, `gen/`, `graph/`,
   `internal/web/templates/*_templ.go`, `internal/pdbcompat/allowlist_gen.go`)
   alongside the changes that produced them. CI will fail on
   generated-code drift otherwise.
3. Open a PR against `main`. CI runs two jobs: `ci` — a single cached Go job
   that runs, in order, the generated-code drift check, `go build`, race tests
   with a coverage comment, `golangci-lint`, and advisory `govulncheck` — and
   `docker-build`, which builds both `Dockerfile` and `Dockerfile.prod`.
4. Coverage excludes `ent/` and `gen/` (generated code). Aim to keep coverage
   on new hand-written code.

## Debugging tips

- **Server won't start:** config validation is fail-fast. Check the first
  `slog.Error("failed to load config", ...)` line in stderr — invalid
  durations, missing required files, and unparseable memory limits all print a
  clear error here.
- **gRPC / streaming breaks under a custom middleware:** response writer
  wrappers **must** implement `http.Flusher` (delegate to the underlying
  writer). Streaming RPCs will hang or buffer without it. Also add
  `Unwrap() http.ResponseWriter` so nested middleware can unwrap to the
  inner writer.
- **Sync never completes / `/readyz` stays unhealthy:** the sync worker only
  writes on the LiteFS primary. Check `internal/litefs/primary.go` —
  `.primary` file **absent** = this node is primary (inverted semantics). For
  local dev without LiteFS, `PDBPLUS_IS_PRIMARY=true` is the default.
- **Generated code drift in CI:** run `go generate ./...` locally and commit
  the resulting diff. The drift check compares `ent/`, `gen/`, `graph/`, and
  `internal/web/templates/` against the working tree. (`internal/pdbcompat/allowlist_gen.go`
  is regenerated too but lives outside the diffed paths, so commit it alongside
  schema changes — CI will not catch its drift on its own.)
- **Schema hand-edits keep disappearing:** you almost certainly added them
  to the generated `ent/schema/{type}.go` file. Move the methods to a
  sibling file `ent/schema/{type}_{method}.go`. See "Sibling-file
  convention" above.
- **`campus` shows up as `campu` somewhere:** the inflection patch in
  `ent/entc.go` is not applying. Rebuild and re-run `go generate ./ent`.
- **Trace / log noise:** `PDBPLUS_OTEL_SAMPLE_RATE=0` turns off sampling for
  local runs. `OTEL_*` env vars follow the autoexport conventions.
- **ent schema change didn't propagate:** you probably skipped
  `go generate ./ent`. The `ent/` directory, `graph/schema.graphqls`,
  `proto/peeringdb/v1/v1.proto`, the REST OpenAPI spec, and
  `internal/pdbcompat/allowlist_gen.go` are **all** derived from
  `ent/schema/`.

## Next steps

- [ARCHITECTURE.md](ARCHITECTURE.md) — system design and package layout.
- [CONFIGURATION.md](CONFIGURATION.md) — environment variables and runtime
  configuration.
- [TESTING.md](TESTING.md) — test framework, conventions, and coverage.
- [API.md](API.md) — API surfaces, traversal allowlists, divergence
  registry.
