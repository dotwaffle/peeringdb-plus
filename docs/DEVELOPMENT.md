<!-- generated-by: gsd-doc-writer -->
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
| `cmd/pdbcompat-check/` | Validates PeeringDB API compatibility |
| `ent/schema/` | **Hand-edited** ent schemas (with entproto annotations) |
| `ent/` | Generated ent client code (do not edit) |
| `proto/peeringdb/v1/` | Proto sources: `v1.proto` (generated), `services.proto` + `common.proto` (hand-written) |
| `gen/peeringdb/v1/` | Generated protobuf Go types and ConnectRPC interfaces |
| `graph/` | GraphQL resolver glue (`schema.resolvers.go`, `custom.resolvers.go` are hand-edited) |
| `internal/grpcserver/` | ConnectRPC handler implementations (one file per entity) |
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
| `go vet ./...` | Standard vet |
| `golangci-lint run` | Lint with project config (`.golangci.yml`) |
| `govulncheck ./...` | Vulnerability check against the Go vulnerability database |

## Code generation pipeline

PeeringDB Plus is heavily code-generated. A single `go generate ./...` invocation
runs every stage in the correct order. The directives live in three files:

1. `ent/generate.go` — runs `entc.go` (ent + entgql + entrest + entproto), then
   `go tool buf generate` for protobuf Go types + ConnectRPC handlers.
2. `internal/web/templates/generate.go` — runs `go tool templ generate` to
   regenerate `*_templ.go` from `.templ` sources.
3. `schema/generate.go` — runs `cmd/pdb-schema-generate` to regenerate
   `ent/schema/*.go` from `schema/peeringdb.json`.

Execution order matters: Go processes `go generate` directives in filesystem
traversal order, so the ent pipeline (which reads `ent/schema/`) runs before
buf (which reads `proto/`) and templ (which reads templates). The
`schema/generate.go` step re-runs `pdb-schema-generate`.

### The "do NOT run go generate ./schema after adding entproto annotations" rule

This is the single most important thing to remember.

`cmd/pdb-schema-generate` regenerates `ent/schema/*.go` from the canonical
PeeringDB JSON schema. **It does not know about entproto annotations.** If you
have added `entproto.Message()` / `entproto.Field()` annotations to hand-edit
an ent schema (and you will, for new ConnectRPC services), re-running the
schema generator will **strip those annotations** and silently lose your work.

Rules of thumb:

- `go generate ./...` is safe after schema extraction was run intentionally
  and you have re-applied annotations.
- `go generate ./schema` on its own is a **regeneration-from-upstream-JSON**
  step. Only run it when you are pulling in changes from PeeringDB's canonical
  schema and are prepared to re-apply any hand-edits (including entproto).
- Day-to-day, you are editing `ent/schema/*.go` directly. Run
  `go generate ./ent` (or `go generate ./...` after skipping schema) to
  regenerate ent, graph, gen, and templates.

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
    │  (cmd/pdb-schema-generate via schema/generate.go)
    ▼
ent/schema/*.go          ← hand-edited after first generation
    │  (go generate ./ent → entc.go)
    ▼
ent/, graph/, proto/peeringdb/v1/v1.proto, REST handlers, GraphQL schema
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
go generate ./ent   # regenerates ent/, graph/, gen/, proto/peeringdb/v1/v1.proto
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
`ent/`, `gen/`, `graph/` files alongside the schema changes that produced them.
CI enforces this — see "Generated code drift check" below.

## Adding a new ent field

1. Edit the relevant file in `ent/schema/`, e.g. `ent/schema/network.go`.
   Add the field to the `Fields()` slice:
   ```go
   field.String("new_field").
       Optional().
       Default("").
       Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
       Comment("Free-form description"),
   ```
2. If the field should appear in ConnectRPC filters, also update the
   corresponding `proto/peeringdb/v1/services.proto` `List*Request` message
   (add an `optional string new_field = N;`).
3. Regenerate:
   ```bash
   go generate ./ent
   ```
   This updates `ent/`, `graph/`, `gen/peeringdb/v1/v1.proto`, and the REST
   OpenAPI spec.
4. Extend the ConnectRPC filter table in `internal/grpcserver/<entity>.go` so
   the new field is honoured at query time. See the `networkListFilters` slice
   in `internal/grpcserver/network.go` for the pattern.
5. Update `internal/sync/` mapping if the field is populated from the PeeringDB
   upstream response.
6. Add a test case to `internal/grpcserver/` and `internal/testutil/seed/seed.go`
   if the field is used by seed data.
7. Commit all regenerated files together.

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
   slice and add a `registerService(peeringdbv1connect.NewXxxServiceHandler(...))`
   call in the block starting near line 329.

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

## Code style

- **`gofmt -s` + `go vet`** are mandatory (enforced by CI / `golangci-lint`).
- **`golangci-lint run`** uses `.golangci.yml`:
  - Enabled (beyond the default `standard` set): `contextcheck`,
    `exhaustive`, `gocritic`, `gosec`, `misspell`, `nolintlint`, `revive`.
  - Generated code is excluded via `exclusions.generated: strict`.
  - `_test.go` files and the schema-generation/compat-check binaries are
    exempt from `gosec` (they shell out and write files by design).
- See the global Go guidelines in user/org docs for wrapping errors, context
  propagation, table-driven tests, and structured logging conventions.

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
   `internal/web/templates/*_templ.go`) alongside the changes that produced
   them. CI will fail on generated-code drift otherwise.
3. Open a PR against `main`. CI runs five jobs: `lint` (includes the drift
   check), `test` (with `-race` and coverage comment), `build`, `govulncheck`,
   and `docker-build` (both `Dockerfile` and `Dockerfile.prod`).
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
  `internal/web/templates/` against HEAD.
- **`campus` shows up as `campu` somewhere:** the inflection patch in
  `ent/entc.go` is not applying. Rebuild and re-run `go generate ./ent`.
- **Trace / log noise:** `PDBPLUS_OTEL_SAMPLE_RATE=0` turns off sampling for
  local runs. `OTEL_*` env vars follow the autoexport conventions.
- **ent schema change didn't propagate:** you probably skipped
  `go generate ./ent`. The `ent/` directory, `graph/schema.graphqls`,
  `gen/peeringdb/v1/v1.proto`, and the REST OpenAPI spec are **all** derived
  from `ent/schema/`.

## Next steps

- [ARCHITECTURE.md](ARCHITECTURE.md) — system design and package layout.
- [CONFIGURATION.md](CONFIGURATION.md) — environment variables and runtime
  configuration.
