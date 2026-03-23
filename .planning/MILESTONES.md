# Milestones

## v1.1 REST API & Observability (Shipped: 2026-03-23)

**Phases completed:** 3 phases, 8 plans, 16 tasks

**Key accomplishments:**

- OTel trace spans on PeeringDB HTTP client with otelhttp transport wrapping and manual span hierarchy (FetchAll parent, per-attempt children, page events, rate limiter wait events)
- 5 per-type sync metric instruments registered and wired with SyncDuration/SyncOperations recording and freshness observable gauge computing seconds-since-last-sync on demand
- entrest v1.0.2 dual codegen with entgql producing read-only REST handlers and OpenAPI spec for all 13 PeeringDB types
- REST API mounted at /rest/v1/ with CORS and 7 integration tests covering all 13 entity types, OpenAPI spec, sorting, pagination, eager-loading, readiness gate, and write rejection
- Per-field filtering via entrest.WithFilter annotations on all 13 PeeringDB schemas with 8 integration test cases
- Django-style filter parser with 8 operators, type registry for all 13 PeeringDB types with field metadata, entity serializers with correct field mapping, and response envelope producing PeeringDB-identical JSON output
- HTTP handlers for all 13 PeeringDB types with list/detail/index endpoints, pagination, since filter, Django-style query filters, and trailing slash handling
- Depth expansion with _set field serialization for all 13 types, text search via ?q=, and field projection via ?fields=

---

## v1.0 PeeringDB Plus (Shipped: 2026-03-22)

**Phases completed:** 3 phases, 14 plans, 27 tasks

**Key accomplishments:**

- Go module bootstrapped with entgo code generation pipeline, Organization schema with full PeeringDB field coverage, SQLite/WAL database setup via modernc.org/sqlite, and OTel trace provider -- all validated end-to-end with in-memory CRUD test
- All 13 PeeringDB object types modeled as entgo schemas with complete field inventories, FK edges, OTel mutation hooks, and CRUD tests validating creation, nullable FKs, and edge traversal
- Rate-limited HTTP client with pagination and retry for all 13 PeeringDB object types, using golang.org/x/time/rate at 20 req/min with exponential backoff on transient errors
- Full PeeringDB sync worker with 13-type bulk upsert in FK dependency order, single-transaction atomicity, hard delete, mutex, 30s/2m/8m exponential backoff retry, and sync_status tracking
- Go-based schema extraction pipeline: regex parser for Django Python source producing intermediate JSON with all 13 PeeringDB types, plus entgo code generator with formatted output, edges, indexes, and entgql annotations
- Application entry point wiring config, database, OTel, and sync worker with HTTP endpoints and multi-stage Docker build
- 13 PeeringDB fixture files and 4 integration tests verifying full sync pipeline: upsert, delete, status filtering, edge traversal, and idempotency
- Relay-compliant GraphQL schema with 13 connection types, offset/limit list queries, and gqlgen resolver scaffold for read-only PeeringDB API
- All 13 PeeringDB type queries with cursor and offset/limit pagination, custom resolvers (syncStatus, networkByAsn), DataLoader middleware for N+1 prevention, and Relay global ID encoding
- HTTP middleware (CORS, logging, recovery), GraphQL handler factory with complexity/depth limits and D-16 error presenter, and config extensions for port, CORS origins, and drain timeout
- Full GraphQL API wired into main.go with middleware stack, playground with example queries, exported SDL, and 16 integration tests covering all 8 requirements
- Autoexport-driven OTel initialization with all three signals (traces, metrics, logs), dual slog handler for stdout+OTel output, and custom sync metrics with configurable sampling
- Liveness/readiness HTTP handlers with sync freshness checking and LiteFS primary/replica detection via .primary file semantics
- Full application wiring with OTel observability, health endpoints, LiteFS primary detection, otelhttp middleware, Fly-Replay write forwarding, and Fly.io deployment artifacts (Dockerfile.prod, litefs.yml, fly.toml)

---
