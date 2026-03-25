# Phase 23: ConnectRPC Services - Context

**Gathered:** 2026-03-25
**Status:** Ready for planning

<domain>
## Phase Boundary

Users can query all 13 PeeringDB types via ConnectRPC with Get and List RPCs, observable via OTel, discoverable via reflection, and monitored via health checks.

</domain>

<decisions>
## Implementation Decisions

### API Behavior
- Get RPCs return `NOT_FOUND` gRPC status with "entity {type} {id} not found" message for non-existent IDs
- Default page size for List RPCs is 100 (matches PeeringDB's default)
- Maximum page size cap is 1000 (prevents accidental full-table dumps)
- Page tokens use opaque base64-encoded cursors (offset-based internally), stateless

### Service Registration
- Use default ConnectRPC path prefix (`/peeringdb.v1.XxxService/`) — standard convention, no custom prefix
- Register all 13 services via loop over a slice of `(path, handler)` pairs in main.go
- Single `grpcserver` package with one file per service type, each implementing the generated handler interface

### Observability & Ecosystem
- CORS content types to add: `application/grpc`, `application/grpc-web`, `application/connect+proto`, `application/connect+json`
- Health check reports SERVING when DB is readable (sync complete at least once) — reuse existing readiness logic
- Reflection via `connectrpc.com/grpcreflect` — native ConnectRPC reflection, works with grpcurl and grpcui

### Claude's Discretion
- Internal implementation details of ent-to-proto field conversion in service handlers
- Exact structure of helper functions for pagination cursor encoding/decoding
- Test organization and fixture patterns

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `gen/peeringdb/v1/peeringdbv1connect/services.connect.go` — 13 handler interfaces with Get/List methods
- `gen/peeringdb/v1/v1.pb.go` — Generated protobuf Go types for all 13 message types
- `gen/peeringdb/v1/services.pb.go` — Request/response message types with pagination fields
- `internal/middleware/cors.go` — Existing CORS middleware with `CORSInput` struct
- `internal/health/handler.go` — Existing readiness handler with DB check logic

### Established Patterns
- HTTP handler registration in `cmd/peeringdb-plus/main.go` via `mux.Handle`/`mux.HandleFunc`
- Middleware chain: Recovery → OTel HTTP → Logging → CORS → Readiness → mux
- `internal/` package convention for domain logic
- ent client passed as dependency to handlers

### Integration Points
- `cmd/peeringdb-plus/main.go` — Register ConnectRPC handlers on existing mux
- `internal/middleware/cors.go` — Update allowed content types for Connect protocol
- Existing OTel setup in main.go — Add otelconnect interceptor

</code_context>

<specifics>
## Specific Ideas

No specific requirements beyond ROADMAP success criteria and the decisions above.

</specifics>

<deferred>
## Deferred Ideas

- **Streaming RPCs for large result sets** — User raised as consideration. Already listed in REQUIREMENTS.md as "deferred pending demand signal". The 1000 max page size provides a safety valve. Can be added in a future phase if demand emerges.

</deferred>
