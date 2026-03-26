# Requirements: PeeringDB Plus

**Defined:** 2026-03-26
**Core Value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.

## v1.10 Requirements

Requirements for the Code Coverage & Test Quality milestone. Each maps to roadmap phases.

### Test Infrastructure

- [x] **INFRA-01**: Shared test seed package provides deterministic entity factories for all 13 PeeringDB types
- [ ] **INFRA-02**: CI coverage reporting excludes generated code (ent/*, gen/*, generated.go, *_templ.go)

### GraphQL Coverage

- [x] **GQL-01**: All 13 offset/limit list resolvers have integration tests with data assertions
- [x] **GQL-02**: Custom resolver error paths tested (NetworkByAsn not found, SyncStatus missing, validatePageSize)
- [x] **GQL-03**: Hand-written resolver files (custom.resolvers.go, schema.resolvers.go, pagination.go) reach 80%+ coverage

### gRPC Coverage

- [x] **GRPC-01**: All 13 entity types have List filter tests covering optional proto field nil-checks
- [x] **GRPC-02**: All 13 entity types have Stream tests (4 types currently missing)
- [x] **GRPC-03**: Filter branch coverage reaches 80%+ across all 13 types using generic test helpers

### Web Coverage

- [ ] **WEB-01**: All 6 lazy-loaded fragment handlers have integration tests
- [ ] **WEB-02**: renderPage dispatch tested for terminal, JSON, and WHOIS output modes
- [ ] **WEB-03**: Edge case coverage for extractID, getFreshness, and error paths

### Schema Coverage

- [ ] **SCHEMA-01**: otelMutationHook error paths tested
- [ ] **SCHEMA-02**: Relationship constraint validation tested for FK edge cases
- [ ] **SCHEMA-03**: ent/schema hand-written code reaches 65%+ coverage (realistic ceiling)

### Minor Package Gaps

- [ ] **MINOR-01**: internal/otel reaches 90%+ coverage with error path tests
- [ ] **MINOR-02**: internal/health reaches 90%+ coverage with edge case tests
- [ ] **MINOR-03**: internal/peeringdb reaches 90%+ coverage with error path tests

### Test Quality

- [ ] **QUAL-01**: Existing tests audited for assertion density -- no test asserts only err == nil without data checks
- [ ] **QUAL-02**: Every fmt.Errorf and connect.NewError call site has at least one test exercising the error path
- [ ] **QUAL-03**: Fuzz test for filter parser covering untrusted input patterns

## Future Requirements

Deferred to future milestones.

### Cross-Surface Consistency

- **XSURF-01**: Same entity returns identical data across GraphQL, REST, PeeringDB compat, and gRPC surfaces
- **XSURF-02**: Golden file tests for gRPC responses (after filter coverage is complete)

### Advanced Testing

- **ADV-01**: Property-based testing for data transformations
- **ADV-02**: Mutation testing integrated into CI pipeline

## Out of Scope

| Feature | Reason |
|---------|--------|
| Testing generated code (ent/*, gen/*, generated.go) | Generated code is tested by its generators. Testing it directly inflates effort without value. |
| 100% coverage targets | Diminishing returns past 85-90%. Static config methods in ent/schema are unreachable. |
| New test framework (testify, gomock) | Project uses stdlib assertions consistently. Mixing styles creates inconsistency. |
| Coverage badge services | Generated code distorts numbers. Per-function analysis is more useful. |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| INFRA-01 | Phase 37 | Complete |
| INFRA-02 | Phase 42 | Pending |
| GQL-01 | Phase 38 | Complete |
| GQL-02 | Phase 38 | Complete |
| GQL-03 | Phase 38 | Complete |
| GRPC-01 | Phase 39 | Complete |
| GRPC-02 | Phase 39 | Complete |
| GRPC-03 | Phase 39 | Complete |
| WEB-01 | Phase 40 | Pending |
| WEB-02 | Phase 40 | Pending |
| WEB-03 | Phase 40 | Pending |
| SCHEMA-01 | Phase 41 | Pending |
| SCHEMA-02 | Phase 41 | Pending |
| SCHEMA-03 | Phase 41 | Pending |
| MINOR-01 | Phase 41 | Pending |
| MINOR-02 | Phase 41 | Pending |
| MINOR-03 | Phase 41 | Pending |
| QUAL-01 | Phase 42 | Pending |
| QUAL-02 | Phase 42 | Pending |
| QUAL-03 | Phase 42 | Pending |

**Coverage:**
- v1.10 requirements: 20 total
- Mapped to phases: 20
- Unmapped: 0

---
*Requirements defined: 2026-03-26*
*Last updated: 2026-03-26 after roadmap creation*
