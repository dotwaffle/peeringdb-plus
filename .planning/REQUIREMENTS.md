# Requirements: PeeringDB Plus

**Defined:** 2026-03-24
**Core Value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.

## v1.6 Requirements

Requirements for ConnectRPC / gRPC API milestone. Each maps to roadmap phases.

### Infrastructure

- [ ] **INFRA-01**: App listens directly on Fly.io internal port without LiteFS HTTP proxy intermediary
- [ ] **INFRA-02**: Sync requests on replicas are replayed to primary via fly-replay response header, gated on Fly.io environment detection
- [ ] **INFRA-03**: Sync requests are handled directly (no replay) when not running on Fly.io
- [ ] **INFRA-04**: Server supports HTTP/2 cleartext (h2c) alongside HTTP/1.1 via http.Protocols
- [ ] **INFRA-05**: fly.toml configured with h2_backend for HTTP/2 to backend

### Proto Generation

- [ ] **PROTO-01**: All 13 ent schemas annotated with entproto.Message and entproto.Field for proto generation
- [ ] **PROTO-02**: buf toolchain configured (buf.yaml, buf.gen.yaml) with protoc-gen-go + protoc-gen-connect-go
- [ ] **PROTO-03**: Proto files generated from ent schemas via entproto with SkipGenFile
- [ ] **PROTO-04**: ConnectRPC handler interfaces generated via buf generate

### API Surface

- [ ] **API-01**: Get RPC returns a single entity by ID for all 13 PeeringDB types
- [ ] **API-02**: List RPC returns paginated results for all 13 PeeringDB types
- [ ] **API-03**: List RPCs support typed filter fields for querying
- [ ] **API-04**: Service handlers mounted on existing HTTP mux at ConnectRPC path prefix

### Observability & Ecosystem

- [ ] **OBS-01**: otelconnect interceptor on all ConnectRPC handlers with WithoutServerPeerAttributes
- [ ] **OBS-02**: CORS headers updated for Connect protocol and gRPC-Web content types
- [ ] **OBS-03**: gRPC server reflection (v1 and v1alpha) enabled for grpcurl/grpcui discovery
- [ ] **OBS-04**: gRPC health check service reports serving status for PeeringDB service

## Future Requirements

### Deferred

- [ ] Expose data via gRPC streaming RPCs — deferred pending demand signal
- [ ] SyncStatus custom RPC — deferred, available via existing REST/GraphQL

## Out of Scope

| Feature | Reason |
|---------|--------|
| protoc-gen-entgrpc service stubs | Hardcoded to google.golang.org/grpc interfaces, incompatible with ConnectRPC |
| Write RPCs (Create/Update/Delete) | Read-only mirror — no write path |
| Bidirectional streaming | No use case for a read-only data mirror |
| Separate gRPC port | ConnectRPC serves all protocols on same port via standard http.Handler |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| INFRA-01 | — | Pending |
| INFRA-02 | — | Pending |
| INFRA-03 | — | Pending |
| INFRA-04 | — | Pending |
| INFRA-05 | — | Pending |
| PROTO-01 | — | Pending |
| PROTO-02 | — | Pending |
| PROTO-03 | — | Pending |
| PROTO-04 | — | Pending |
| API-01 | — | Pending |
| API-02 | — | Pending |
| API-03 | — | Pending |
| API-04 | — | Pending |
| OBS-01 | — | Pending |
| OBS-02 | — | Pending |
| OBS-03 | — | Pending |
| OBS-04 | — | Pending |

**Coverage:**
- v1.6 requirements: 17 total
- Mapped to phases: 0
- Unmapped: 17

---
*Requirements defined: 2026-03-24*
*Last updated: 2026-03-24 after initial definition*
