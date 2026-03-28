<!-- GSD:project-start source:PROJECT.md -->
## Project

**PeeringDB Plus**

A high-performance, globally distributed, read-only mirror of PeeringDB data. It syncs all PeeringDB objects via full re-fetch on a regular schedule (hourly or on-demand), stores them in SQLite on LiteFS for edge-local reads on Fly.io, and presents the data through modern API surfaces: GraphQL, gRPC, and OpenAPI-compliant REST. Built in Go using entgo as the ORM.

**Core Value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.

### Constraints

- **Language**: Go 1.26
- **ORM**: entgo (non-negotiable — ecosystem drives GraphQL/gRPC/REST generation)
- **Storage**: SQLite + LiteFS (enables edge distribution without a central database)
- **Platform**: Fly.io (LiteFS dependency, global edge deployment)
- **Observability**: OpenTelemetry — mandatory for tracing, metrics, and logs
- **Data fidelity**: Must handle PeeringDB's actual API responses, not their documented spec
<!-- GSD:project-end -->

<!-- GSD:stack-start source:research/STACK.md -->
## Technology Stack

## Recommended Stack
### Language & Runtime
| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| Go | 1.26 | Application language | Project constraint. Released 2026-02-10. Green Tea GC enabled by default (10-40% lower GC overhead). Enhanced `go fix` with modernizers. Stack allocation improvements for slices. | HIGH |
### ORM & Code Generation
| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| entgo.io/ent | v0.14.5 | ORM / schema-first code generation | Project constraint. Schema-first with code generation produces type-safe Go clients. Ecosystem packages (entgql, entproto, entrest) generate all three API surfaces from a single schema definition. Published 2025-07-21. | HIGH |
| entgo.io/contrib (entgql) | v0.7.0 | GraphQL API generation from ent schemas | Mature extension for ent. Generates Relay-compliant GraphQL with pagination, filtering, sorting, and eager-loading. Works with gqlgen. Published 2024-08-02, with module updates through 2025-03. | HIGH |
| entgo.io/contrib (entproto) | v0.7.0 | Protobuf/gRPC generation from ent schemas | Generates .proto files and gRPC service implementations from ent schemas. Experimental but functional -- requires protoc toolchain. Same module as entgql. | MEDIUM |
| github.com/lrstanley/entrest | v1.0.2 | OpenAPI REST generation from ent schemas | Generates compliant OpenAPI specs and HTTP handler implementations. Supports pagination, filtering, eager-loading, sorting. v1.0.2 published 2025-08-21. MIT licensed. Documentation warns "expect breaking changes" but module is v1.x. | MEDIUM |
| github.com/99designs/gqlgen | latest | GraphQL server library for Go | Required by entgql. Schema-first GraphQL server with code generation. Actively maintained (last published 2026-03-09). The standard Go GraphQL server library. | HIGH |
### Database & Storage
| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| modernc.org/sqlite | v1.36.0+ | CGo-free SQLite driver | Pure Go SQLite implementation via C-to-Go transpilation. No CGo dependency means simpler cross-compilation and deployment. Works with ent via standard database/sql interface. Bundles SQLite 3.49.0+. Actively maintained. | HIGH |
| superfly/litefs | v0.5.14 | SQLite replication across Fly.io regions | FUSE-based filesystem that transparently replicates SQLite databases across a cluster. Intercepts writes to detect transaction boundaries and streams changes to replicas. Published 2025-04-22. | MEDIUM |
- LiteFS Cloud (managed backups) was **discontinued October 2024**
- LiteFS itself is in **maintenance mode** with limited Fly.io support
- Pre-1.0, APIs may change, no guaranteed roadmap
- Fly.io states they "cannot provide support or guidance" for this product
- Still stable and running in production, but budget extra development time for troubleshooting
- No viable drop-in alternative exists for the same use case (edge SQLite replication)
- **Mitigation:** Implement Litestream as backup strategy alongside LiteFS for disaster recovery
### HTTP & Routing
| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| net/http (stdlib) | Go 1.26 | HTTP server, base mux | Go 1.22+ ServeMux supports method-based routing and path parameters natively. Sufficient for this project's needs. No external router dependency needed for the API surfaces since entrest and gqlgen provide their own handlers. | HIGH |
| github.com/go-chi/chi/v5 | v5.2.x | HTTP router (if needed) | Use only if net/http ServeMux proves insufficient for middleware composition. Chi is lightweight (<1000 LOC), implements standard http.Handler, and provides route grouping with middleware. Published 2026-02-05. Prefer stdlib first. | MEDIUM |
### gRPC
| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| google.golang.org/grpc | latest | gRPC server implementation | Required by entproto-generated code. Standard gRPC-Go implementation. Higher raw performance (~20K rps) than ConnectRPC (~16K rps). entproto generates code targeting protoc-gen-go-grpc. | HIGH |
| google.golang.org/protobuf | latest | Protocol Buffers runtime | Required by entproto and gRPC. Standard protobuf Go runtime. | HIGH |
| buf.build/buf/cli | latest | Protobuf toolchain | Use buf instead of raw protoc for proto file management. Better dependency management, linting, breaking change detection. Recommended by project CLAUDE.md (TL-4). | MEDIUM |
### Observability
| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| go.opentelemetry.io/otel | v1.35+ | OpenTelemetry API | Project constraint. Stable for traces and metrics. Published 2026-03-06. | HIGH |
| go.opentelemetry.io/otel/sdk | v1.35+ | OpenTelemetry SDK | Trace and metric SDK. Published 2026-02-02. | HIGH |
| go.opentelemetry.io/contrib/bridges/otelslog | latest | slog-to-OTel log bridge | Bridges stdlib slog to OpenTelemetry log pipeline. <1% overhead. Adds trace/span IDs to log records for correlation. | HIGH |
| go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp | latest | HTTP instrumentation | Automatic span creation for incoming HTTP requests. Works with any http.Handler (chi, stdlib, etc). Published 2026-03-06. | HIGH |
| log/slog (stdlib) | Go 1.26 | Structured logging | Stdlib structured logging. Use with otelslog bridge for OTel integration. Zero external dependency. Per project CLAUDE.md (OBS-1, OBS-5). | HIGH |
### Web UI (Secondary Priority)
| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| github.com/a-h/templ | v0.3.x | Type-safe HTML templating | Compiles .templ files to Go code with full type checking. No runtime template parsing. 5,400+ importers. Published 2026-02-28. | HIGH |
| htmx | 2.0.8 | Frontend interactivity without JS | Server-driven UI updates via HTML attributes. Delivered as a single JS file from CDN. No build toolchain. htmx 4.0 expected mid-2026 but 2.0.x is stable. | HIGH |
### PeeringDB Sync
| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| net/http (stdlib) | Go 1.26 | HTTP client for PeeringDB API | Standard HTTP client is sufficient for fetching JSON from PeeringDB REST API. No need for a PeeringDB client library -- the only Go library (gmazoyer/peeringdb) is read-only, unmaintained, and doesn't handle the API spec discrepancies noted in PROJECT.md. Roll our own sync client. | HIGH |
| encoding/json (stdlib) | Go 1.26 | JSON deserialization | Parse PeeringDB API responses. Custom unmarshaling needed to handle spec-vs-reality discrepancies. | HIGH |
### Build & Development Tooling
| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| go-task/task | v3.x | Task runner | YAML-based task definitions. Cross-platform. Checksum-based dependency tracking. Cleaner than Makefile for Go projects. Widely adopted in Go ecosystem 2025-2026. | MEDIUM |
| golangci-lint | latest | Linter aggregator | Per project CLAUDE.md (TL-1, G-2). Runs gofumpt, staticcheck, vet, and custom linters. | HIGH |
| govulncheck | latest | Vulnerability scanning | Per project CLAUDE.md (TL-2, CI-4). Checks dependencies for known vulnerabilities. | HIGH |
| github.com/air-verse/air | latest | Hot reload for development | File-watching rebuild for development. Watches .go and .templ files, rebuilds and restarts. | LOW |
### Testing
| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| testing (stdlib) | Go 1.26 | Test framework | Standard Go testing. Table-driven tests per T-1. -race flag per T-2. | HIGH |
| entgo.io/ent/enttest | v0.14.5 | Ent test helpers | Provides test client setup with auto-migration for SQLite. Spins up in-memory or file-backed test databases. | HIGH |
### Dependency Injection
## Alternatives Considered
| Category | Recommended | Alternative | Why Not |
|----------|-------------|-------------|---------|
| SQLite driver | modernc.org/sqlite | mattn/go-sqlite3 | Requires CGo, complicates cross-compilation and Fly.io deployment. modernc.org works with ent. |
| SQLite driver | modernc.org/sqlite | ncruces/go-sqlite3 (wazero) | Newer, less proven with ent. 314% faster reads in some benchmarks but uses WASM runtime (wazero) which adds complexity. modernc.org is the established CGo-free choice. |
| Edge replication | LiteFS | Turso/LibSQL | Turso is a managed service with its own pricing. LiteFS is self-hosted on Fly.io. LibSQL is a fork of SQLite which may diverge. LiteFS keeps us on standard SQLite with ent compatibility. |
| Edge replication | LiteFS | Litestream | Litestream is disaster recovery (backup to S3), not read replication. Does not provide edge-local read copies. Use alongside LiteFS, not instead of. |
| HTTP router | net/http stdlib | chi v5 | Stdlib is sufficient for generated handlers. Chi adds middleware composition but is premature until proven needed. |
| HTTP router | net/http stdlib | Fiber / Echo / Gin | Heavy frameworks with custom context types. Don't compose well with generated ent handlers that expect standard http.Handler. |
| GraphQL | gqlgen (via entgql) | graphql-go/graphql | gqlgen is required by entgql. No choice here. |
| gRPC | google.golang.org/grpc | ConnectRPC | entproto generates standard gRPC code. ConnectRPC would require custom codegen or manual adaptation. |
| REST | entrest (lrstanley) | entoas (official) | entoas only generates OpenAPI specs, not HTTP handlers. entrest generates both spec and implementation. |
| Logging | slog (stdlib) | zerolog / zap | slog is stdlib, has OTel bridge, and meets all requirements (OBS-1, OBS-5). External loggers add dependencies for marginal gains in a non-latency-critical path. |
| Templating | templ | html/template (stdlib) | templ provides compile-time type checking, better error messages, and component composition. html/template has runtime errors and stringly-typed templates. |
| DI | Manual | Wire / fx | Unnecessary complexity for a project with a simple, linear dependency graph. |
| Task runner | task | Makefile | YAML syntax is clearer, cross-platform, checksum-based deps. Make works but tab-sensitivity and arcane syntax are friction. |
## Version Pinning Strategy
## Installation
# Initialize module
# Core ORM & code generation
# GraphQL
# SQLite
# gRPC / Protobuf
# OpenTelemetry
# Web UI
# Dev tools (install, not go get)
## Risk Register
| Risk | Severity | Likelihood | Mitigation |
|------|----------|------------|------------|
| LiteFS maintenance mode / eventual abandonment | HIGH | MEDIUM | Implement Litestream backup alongside. Architecture should abstract storage layer so migration to Turso/LibSQL is possible. Monitor LiteFS GitHub for activity. |
| entrest breaking changes despite v1.x | MEDIUM | MEDIUM | Pin version. The REST surface is lowest priority of the three APIs. Can fall back to manual OpenAPI spec + handler if entrest breaks. |
| entproto experimental status | MEDIUM | LOW | entproto has been "experimental" for years but is functionally stable. gRPC surface is secondary. Can generate protos manually from ent schema if needed. |
| modernc.org/sqlite performance vs CGo driver | LOW | LOW | For a read-only mirror with hourly syncs, modernc.org performance is more than adequate. Benchmark during phase 1 to confirm. |
| PeeringDB API response divergence from spec | HIGH | HIGH | This is a known issue (PROJECT.md). Must analyze actual Python source code to understand real response shapes. Build custom deserializers, not generated clients. |
| HTMX 4.0 migration | LOW | LOW | htmx 4.0 not expected as "latest" until early 2027. Build on 2.0.x. Migration path is straightforward (fetch() replaces XMLHttpRequest internally). |
## Sources
- [Go 1.26 Release Notes](https://go.dev/doc/go1.26) - Go 1.26 features and release date
- [entgo.io](https://entgo.io/) - Ent ORM documentation
- [ent/ent Releases](https://github.com/ent/ent/releases) - v0.14.5 release
- [ent/contrib Releases](https://github.com/ent/contrib/releases) - v0.7.0 release
- [lrstanley/entrest](https://github.com/lrstanley/entrest) - REST extension for ent
- [pkg.go.dev entrest](https://pkg.go.dev/github.com/lrstanley/entrest) - v1.0.2 published 2025-08-21
- [entgql docs](https://entgo.io/docs/extensions/) - GraphQL extension documentation
- [entproto docs](https://entgo.io/docs/grpc-generating-proto/) - Protobuf generation
- [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) - CGo-free SQLite driver
- [go-sqlite-bench](https://github.com/cvilsmeier/go-sqlite-bench) - SQLite driver benchmarks
- [LiteFS Docs](https://fly.io/docs/litefs/) - Fly.io LiteFS documentation
- [LiteFS Status Discussion](https://community.fly.io/t/what-is-the-status-of-litefs/23883) - Maintenance mode confirmation
- [LiteFS Cloud Sunset](https://community.fly.io/t/sunsetting-litefs-cloud/20829) - Cloud service discontinuation
- [OpenTelemetry Go](https://opentelemetry.io/docs/languages/go/) - OTel Go SDK documentation
- [otelslog bridge](https://pkg.go.dev/go.opentelemetry.io/contrib/bridges/otelslog) - slog to OTel bridge
- [otelhttp](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp) - HTTP instrumentation
- [gqlgen](https://github.com/99designs/gqlgen) - GraphQL server library
- [ConnectRPC Conformance](https://buf.build/blog/grpc-conformance-deep-dive) - gRPC vs ConnectRPC comparison
- [templ](https://github.com/a-h/templ) - Type-safe HTML templating
- [htmx](https://htmx.org/) - Frontend interactivity library
- [PeeringDB API Docs](https://www.peeringdb.com/apidocs/) - Official API documentation
- [chi router](https://github.com/go-chi/chi) - Lightweight HTTP router
- [Taskfile](https://taskfile.dev/) - Modern task runner
<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->
## Conventions

### Code Generation
- `go generate ./...` runs the full codegen pipeline in correct order:
  1. `ent/generate.go` — runs entc.go (ent + entgql + entrest + entproto), then `buf generate` for proto Go types
  2. `internal/web/templates/generate.go` — runs `templ generate` for templ Go files
  3. `schema/generate.go` — regenerates ent schemas from PeeringDB JSON (safe to re-run)
- `buf`, `templ`, and `gqlgen` are Go tool dependencies (`go tool buf`, `go tool templ`, `go tool gqlgen`) — no external install needed.
- Do NOT run `go generate ./schema` after entproto annotations are added — the schema generator strips entproto annotations.
- Always commit `*_templ.go` alongside `.templ` changes, and generated `ent/`/`gen/`/`graph/` files alongside schema changes.
- Proto files: `proto/peeringdb/v1/v1.proto` (messages, generated by entproto), `services.proto` (RPCs, hand-written), `common.proto` (manual types like SocialMedia).

### Middleware
- Response writer wrappers MUST implement `http.Flusher` (delegate to underlying writer) — gRPC streaming requires it.
- Add `Unwrap() http.ResponseWriter` for middleware-aware interface detection.

### ConnectRPC / gRPC
- Services registered via loop in `cmd/peeringdb-plus/main.go` with otelconnect interceptor.
- Handler implementations in `internal/grpcserver/` — one file per entity type.
- `gen/peeringdb/v1/peeringdbv1connect/` contains generated handler interfaces.
- Proto `optional` fields generate pointer types (`*int64`, `*string`) — check `!= nil` for presence.

### Go Module
- `GONOSUMCHECK=* GONOSUMDB=*` may be needed for `go mod tidy` / `go get` when sumdb is read-only.
- `TMPDIR=/tmp/claude-1000` required for go commands in sandbox mode.

### Deployment
- `fly deploy` from project root. App: peeringdb-plus. 5 machines across lhr, iad, sin, lax.
- LiteFS FUSE mount takes a moment — "not listening" warnings during rolling deploy are normal.
<!-- GSD:conventions-end -->

<!-- GSD:architecture-start source:ARCHITECTURE.md -->
## Architecture

### API Surfaces (5)
- **Web UI**: `/ui/` — templ + htmx + Tailwind CSS (search, detail pages, ASN comparison)
- **GraphQL**: `/graphql` — gqlgen via entgql, interactive playground
- **REST**: `/rest/v1/` — entrest, OpenAPI-compliant
- **PeeringDB Compat**: `/api/` — drop-in replacement for PeeringDB API
- **ConnectRPC/gRPC**: `/peeringdb.v1.*/` — Get/List RPCs for all 13 types with typed filtering, reflection, health check

### Middleware Chain
`Recovery -> OTel HTTP -> Logging -> CORS -> Readiness -> mux`

### Key Packages
- `cmd/peeringdb-plus/` — main entry, handler registration, server config
- `internal/grpcserver/` — ConnectRPC service handlers (13 files + convert.go + pagination.go)
- `internal/middleware/` — CORS, logging, recovery
- `internal/sync/` — PeeringDB data sync worker
- `internal/pdbcompat/` — PeeringDB-compatible API layer
- `ent/schema/` — entgo schemas (hand-edited with entproto annotations, also regenerated by pdb-schema-generate)
- `gen/peeringdb/v1/` — generated proto Go types and ConnectRPC interfaces
- `proto/peeringdb/v1/` — proto source files
<!-- GSD:architecture-end -->

<!-- GSD:workflow-start source:GSD defaults -->
## GSD Workflow Enforcement

Before using Edit, Write, or other file-changing tools, start work through a GSD command so planning artifacts and execution context stay in sync.

Use these entry points:
- `/gsd:quick` for small fixes, doc updates, and ad-hoc tasks
- `/gsd:debug` for investigation and bug fixing
- `/gsd:execute-phase` for planned phase work

Do not make direct repo edits outside a GSD workflow unless the user explicitly asks to bypass it.
<!-- GSD:workflow-end -->



<!-- GSD:profile-start -->
## Developer Profile

> Profile not yet configured. Run `/gsd:profile-user` to generate your developer profile.
> This section is managed by `generate-claude-profile` -- do not edit manually.
<!-- GSD:profile-end -->
