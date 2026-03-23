# Requirements: PeeringDB Plus

**Defined:** 2026-03-23
**Core Value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.

## v1.2 Requirements

Requirements for v1.2 Quality & CI milestone. Each maps to roadmap phases.

### Linting & Code Quality

- [ ] **LINT-01**: golangci-lint v2 configuration with `generated: strict` to exclude generated code
- [ ] **LINT-02**: All existing lint violations in hand-written code fixed
- [ ] **LINT-03**: `go vet ./...` passes clean across entire codebase

### Incremental Sync

- [ ] **SYNC-01**: Configurable sync mode via `PDBPLUS_SYNC_MODE` env var (`full` or `incremental`, default `full`)
- [ ] **SYNC-02**: Optional `?since=` parameter on FetchAll for delta fetches
- [ ] **SYNC-03**: Per-type last-sync timestamp tracking in extended sync_status table
- [ ] **SYNC-04**: Incremental sync fetches only objects modified since last successful sync per type
- [ ] **SYNC-05**: On incremental failure for a type, immediately falls back to full sync for that type

### Golden File Tests

- [ ] **GOLD-01**: Golden file test infrastructure with `-update` flag for regenerating files
- [ ] **GOLD-02**: Golden files for all 13 PeeringDB types — list endpoint responses
- [ ] **GOLD-03**: Golden files for all 13 PeeringDB types — detail endpoint responses
- [ ] **GOLD-04**: Golden files for depth-expanded responses with `_set` fields

### PeeringDB Conformance

- [ ] **CONF-01**: CLI tool that fetches from beta.peeringdb.com and compares response structure against compat layer output
- [ ] **CONF-02**: Integration test gated by `-peeringdb-live` flag using beta.peeringdb.com that validates conformance (skipped in normal CI)

### CI Pipeline

- [ ] **CI-01**: GitHub Actions workflow with parallel lint, test, and build jobs
- [ ] **CI-02**: `go test -race ./...` with `CGO_ENABLED=1` in CI
- [ ] **CI-03**: govulncheck security scanning in CI
- [ ] **CI-04**: Test coverage percentage tracking and reporting

### Public Access

- [ ] **PUB-01**: Verify no auth barriers exist on any endpoint
- [ ] **PUB-02**: Document public access model (no auth required, read-only, open data)

## Future Requirements

Deferred to future milestones. Tracked but not in current roadmap.

### gRPC API

- **GRPC-01**: Expose data via gRPC using entproto
- **GRPC-02**: Protobuf schema generation from ent schemas

### Web UI

- **UI-01**: Web UI for browsing data (HTMX + Templ)
- **UI-02**: Interactive data explorer

## Out of Scope

| Feature | Reason |
|---------|--------|
| Write-path / data modification | Read-only mirror |
| User accounts or authentication | Fully public service |
| OAuth or API key gating | Not needed for current scope |
| Mobile app | Web-first |
| Real-time streaming | Periodic sync is sufficient |
| Tech debt cleanup (unused exports, vestigial config) | Not blocking quality or CI; defer to future |
| Golden files for GraphQL or entrest REST surfaces | PeeringDB compat is the compatibility contract; other surfaces have adequate test coverage |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| LINT-01 | Phase 7 | Pending |
| LINT-02 | Phase 7 | Pending |
| LINT-03 | Phase 7 | Pending |
| SYNC-01 | Phase 8 | Pending |
| SYNC-02 | Phase 8 | Pending |
| SYNC-03 | Phase 8 | Pending |
| SYNC-04 | Phase 8 | Pending |
| SYNC-05 | Phase 8 | Pending |
| GOLD-01 | Phase 9 | Pending |
| GOLD-02 | Phase 9 | Pending |
| GOLD-03 | Phase 9 | Pending |
| GOLD-04 | Phase 9 | Pending |
| CONF-01 | Phase 9 | Pending |
| CONF-02 | Phase 9 | Pending |
| CI-01 | Phase 10 | Pending |
| CI-02 | Phase 10 | Pending |
| CI-03 | Phase 10 | Pending |
| CI-04 | Phase 10 | Pending |
| PUB-01 | Phase 10 | Pending |
| PUB-02 | Phase 10 | Pending |

**Coverage:**
- v1.2 requirements: 20 total
- Mapped to phases: 20
- Unmapped: 0

---
*Requirements defined: 2026-03-23*
*Last updated: 2026-03-23 after adding incremental sync phase*
