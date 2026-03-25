# Requirements: PeeringDB Plus

**Defined:** 2026-03-25
**Core Value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.

## v1.7 Requirements

Requirements for milestone v1.7: Streaming RPCs & UI Polish.

### Streaming RPCs

- [x] **STRM-01**: Server-streaming RPC per entity type — 13 `Stream*` RPCs returning one proto message per row
- [ ] **STRM-02**: Batched keyset pagination in streaming handlers — chunk queries by ID to avoid loading full result sets
- [ ] **STRM-03**: Graceful stream cancellation — honor `ctx.Done()` between batch fetches
- [ ] **STRM-04**: Total record count in response header — `COUNT(*)` query, set via `stream.ResponseHeader()` before first `Send()`
- [ ] **STRM-05**: Filter support on streaming RPCs — same optional filter fields as List, reusing predicate accumulation
- [x] **STRM-06**: OTel instrumentation on streaming RPCs — otelconnect interceptor produces per-stream spans
- [x] **STRM-07**: Proto/JSON format negotiation — ConnectRPC handles automatically, document for consumers
- [x] **STRM-08**: `since_id` stream resume — optional field to resume from last received ID
- [x] **STRM-09**: `updated_since` filter — stream only records modified after a timestamp

### IX Presence UI

- [x] **IXUI-01**: Field labels for speed, IPv4, IPv6 in IX presence rows
- [x] **IXUI-02**: RS badge repositioned inline after IX name
- [x] **IXUI-03**: Port speed color coding by tier (sub-1G gray, 1G neutral, 10G blue, 100G emerald, 400G+ amber)
- [x] **IXUI-04**: Consistent IP address alignment via grid layout across rows
- [x] **IXUI-05**: Selectable/copyable text — IX name is the only link, data fields are plain text
- [x] **IXUI-06**: Copy-to-clipboard button on IPv4/IPv6 addresses
- [x] **IXUI-07**: Aggregate bandwidth display in IX presence section header

## Future Requirements

### Data Enrichment (deferred — needs more design)

- **ENRCH-01**: Per-ASN BGP summary from bgp.tools daily table dump (prefix counts v4/v6, RPKI coverage %)
- **ENRCH-02**: IRR/AS-SET membership from WHOIS source (rr.ntt.net or whois.radb.net TBD)
- **ENRCH-03**: IP prefix lookup showing origin ASN, RPKI status, AS-SET membership

### Streaming Extensions (deferred)

- **STRM-10**: SHA256 checksum in response trailers for data integrity verification
- **STRM-11**: SyncStatus custom RPC — deferred, available via existing REST/GraphQL

## Out of Scope

| Feature | Reason |
|---------|--------|
| Bidirectional/client-streaming RPCs | Read-only mirror has no write path |
| WebSocket streaming fallback | ConnectRPC handles streaming over HTTP/2 natively |
| Real-time change streaming / subscriptions | Periodic sync mirror, not a live database |
| Custom download formats (CSV, NDJSON) | Protobuf and JSON via ConnectRPC are sufficient |
| IX presence interactive map | High complexity, separate future feature |
| Sortable IX presence table | Defer to future milestone |
| Inline editing of IX presence data | Read-only mirror |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| STRM-01 | Phase 25 | Complete |
| STRM-02 | Phase 25 | Pending |
| STRM-03 | Phase 25 | Pending |
| STRM-04 | Phase 25 | Pending |
| STRM-05 | Phase 25 | Pending |
| STRM-06 | Phase 25 | Complete |
| STRM-07 | Phase 25 | Complete |
| STRM-08 | Phase 26 | Complete |
| STRM-09 | Phase 26 | Complete |
| IXUI-01 | Phase 27 | Complete |
| IXUI-02 | Phase 27 | Complete |
| IXUI-03 | Phase 27 | Complete |
| IXUI-04 | Phase 27 | Complete |
| IXUI-05 | Phase 27 | Complete |
| IXUI-06 | Phase 27 | Complete |
| IXUI-07 | Phase 27 | Complete |

**Coverage:**
- v1.7 requirements: 16 total
- Mapped to phases: 16
- Unmapped: 0

---
*Requirements defined: 2026-03-25*
*Last updated: 2026-03-25 after roadmap creation*
