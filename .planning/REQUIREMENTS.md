# Requirements: PeeringDB Plus

**Defined:** 2026-03-26
**Core Value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.

## v1.9 Requirements

Requirements for Hardening & Polish milestone. Each maps to roadmap phases.

### Performance

- [ ] **PERF-01**: Search service uses a single query per entity type instead of separate item + count queries
- [ ] **PERF-02**: API responses include HTTP caching headers (Cache-Control, ETag) derived from sync timestamp
- [ ] **PERF-03**: Database indexes exist on `updated` and `created` fields for incremental sync and filtered queries
- [ ] **PERF-04**: Benchmark suite covers search, field projection, gRPC streaming conversion, and sync upsert hot paths
- [ ] **PERF-05**: Field projection in pdbcompat avoids JSON marshal/unmarshal roundtrip per item

### Code Quality

- [ ] **QUAL-01**: gRPC service handlers share a generic List/Stream implementation, eliminating ~1,154 lines of duplicated logic across 13 files
- [ ] **QUAL-02**: All error logging uses `slog.Any("error", err)` instead of `slog.String("error", err.Error())`
- [ ] **QUAL-03**: Test coverage for `internal/grpcserver` reaches 60%+ and `internal/middleware` reaches 60%+
- [ ] **QUAL-04**: Web detail handlers in `detail.go` are refactored to separate query logic from rendering (each under 80 lines)

### Architecture

- [ ] **ARCH-01**: All 6 API surfaces return errors in a consistent format with code, message, and optional details
- [ ] **ARCH-02**: ConnectRPC List RPCs expose the same filterable fields as the PeeringDB compat layer for each entity type
- [ ] **ARCH-03**: CORS middleware runs before OTel tracing in the middleware chain so OPTIONS preflight requests are not traced/logged
- [ ] **ARCH-04**: Terminal renderer dispatches to entity renderers via interface rather than type-switch on concrete template types

### Web UI

- [ ] **UI-01**: Dark mode text passes WCAG AA contrast ratio (4.5:1 minimum) on all pages
- [ ] **UI-02**: All interactive elements have ARIA attributes (nav role, aria-expanded on mobile menu, form labels on search)
- [ ] **UI-03**: Search results update the browser URL so searches are bookmarkable and shareable
- [ ] **UI-04**: Failed htmx collapsible section loads show an error message with retry option instead of perpetual "Loading..."
- [ ] **UI-05**: Detail pages include breadcrumb navigation (Home > Type > Entity)
- [ ] **UI-06**: Mobile navigation menu closes after clicking a link
- [ ] **UI-07**: Compare button on network detail pages is visually distinct from the page background

### Terminal UI

- [ ] **TUI-01**: Long entity names in terminal output wrap intelligently instead of being truncated
- [ ] **TUI-02**: Terminal error responses (404, 500, sync-not-ready) use styled text formatting consistent with normal output

## Future Requirements

### Deferred from previous milestones

- **SYNC-01**: SyncStatus custom RPC — available via existing REST/GraphQL
- **BGP-01**: Per-ASN BGP summary from bgp.tools (prefix counts, RPKI coverage)
- **IRR-01**: IRR/AS-SET membership from WHOIS source
- **LOOKUP-01**: IP prefix lookup with origin ASN, RPKI status

## Out of Scope

| Feature | Reason |
|---------|--------|
| New API surfaces or entity types | v1.9 is hardening-only; no new features |
| Write-path / data modification | Read-only mirror by design |
| Response caching beyond HTTP headers | Application-level cache (e.g., sync.Map) adds complexity; HTTP caching sufficient for hourly sync |
| gRPC handler code generation | Deduplication via generic helpers preferred over maintaining a codegen tool |
| Full WCAG AAA compliance | AA is the target; AAA exceeds scope for a developer-focused tool |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| PERF-01 | Phase 34 | Pending |
| PERF-02 | Phase 35 | Pending |
| PERF-03 | Phase 34 | Pending |
| PERF-04 | Phase 35 | Pending |
| PERF-05 | Phase 34 | Pending |
| QUAL-01 | Phase 33 | Pending |
| QUAL-02 | Phase 32 | Pending |
| QUAL-03 | Phase 33 | Pending |
| QUAL-04 | Phase 34 | Pending |
| ARCH-01 | Phase 34 | Pending |
| ARCH-02 | Phase 33 | Pending |
| ARCH-03 | Phase 32 | Pending |
| ARCH-04 | Phase 34 | Pending |
| UI-01 | Phase 36 | Pending |
| UI-02 | Phase 36 | Pending |
| UI-03 | Phase 36 | Pending |
| UI-04 | Phase 36 | Pending |
| UI-05 | Phase 36 | Pending |
| UI-06 | Phase 36 | Pending |
| UI-07 | Phase 36 | Pending |
| TUI-01 | Phase 36 | Pending |
| TUI-02 | Phase 36 | Pending |

**Coverage:**
- v1.9 requirements: 22 total
- Mapped to phases: 22
- Unmapped: 0

---
*Requirements defined: 2026-03-26*
*Last updated: 2026-03-26 after roadmap creation*
