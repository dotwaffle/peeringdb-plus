# Roadmap: PeeringDB Plus

## Milestones

- [x] **v1.0 MVP** - Phases 1-3 (shipped 2026-03-22)
- [x] **v1.1 REST API & Observability** - Phases 4-6 (shipped 2026-03-23)
- [x] **v1.2 Quality, Incremental Sync & CI** - Phases 7-10 (shipped 2026-03-24)
- [x] **v1.3 PeeringDB API Key Support** - Phases 11-12 (shipped 2026-03-24)
- [x] **v1.4 Web UI** - Phases 13-17 (shipped 2026-03-24)
- [x] **v1.5 Tech Debt & Observability** - Phases 18-20 (shipped 2026-03-24)
- [x] **v1.6 ConnectRPC / gRPC API** - Phases 21-24 (shipped 2026-03-25)
- [ ] **v1.7 Streaming RPCs & UI Polish** - Phases 25-27 (in progress)

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

- [x] **Phase 25: Streaming RPCs** - Proto definitions, code generation, and 13 streaming handlers with batched keyset pagination (completed 2026-03-25)
- [x] **Phase 26: Stream Resume & Incremental Filters** - `since_id` resume and `updated_since` timestamp filtering on streaming RPCs (completed 2026-03-25)
- [ ] **Phase 27: IX Presence UI Polish** - Field labels, speed colors, RS badge, IP alignment, copyable text, and aggregate bandwidth

## Phase Details

### Phase 25: Streaming RPCs
**Goal**: Consumers can stream entire entity tables via gRPC/ConnectRPC without manual pagination
**Depends on**: Phase 24
**Requirements**: STRM-01, STRM-02, STRM-03, STRM-04, STRM-05, STRM-06, STRM-07
**Success Criteria** (what must be TRUE):
  1. A `buf curl` or `grpcurl` call to any of the 13 `Stream*` RPCs returns all rows streamed one message at a time
  2. Memory usage stays bounded during a full-table stream (batched keyset pagination, not full `ent.All()`)
  3. Cancelling a stream mid-flight (client disconnect) terminates the server-side query loop promptly
  4. Total record count is available in the response header metadata before the first message arrives
  5. Applying filter fields on a streaming RPC returns only matching records, consistent with the corresponding List RPC
**Plans:** 3/3 plans complete

Plans:
- [x] 25-01-PLAN.md -- Proto schema + codegen + config + stubs + OTel update
- [x] 25-02-PLAN.md -- StreamNetworks reference implementation + integration tests
- [x] 25-03-PLAN.md -- Remaining 12 streaming handlers + consumer documentation

### Phase 26: Stream Resume & Incremental Filters
**Goal**: Automation consumers can resume interrupted streams and fetch only recently-changed records
**Depends on**: Phase 25
**Requirements**: STRM-08, STRM-09
**Success Criteria** (what must be TRUE):
  1. Passing `since_id` to a streaming RPC returns only records with ID greater than the given value
  2. Passing `updated_since` to a streaming RPC returns only records modified after the given timestamp
  3. Combining `since_id` or `updated_since` with other filters works correctly (filters compose via AND)
**Plans:** 1/1 plans complete

Plans:
- [x] 26-01-PLAN.md -- Proto fields + codegen + all 13 handler updates + integration tests

### Phase 27: IX Presence UI Polish
**Goal**: IX presence sections display connection details clearly with labeled fields, visual speed indicators, and copyable addresses
**Depends on**: Phase 24 (no dependency on streaming phases)
**Requirements**: IXUI-01, IXUI-02, IXUI-03, IXUI-04, IXUI-05, IXUI-06, IXUI-07
**Success Criteria** (what must be TRUE):
  1. Each IX presence row shows labeled Speed, IPv4, and IPv6 fields (not just bare values)
  2. Port speeds are color-coded by tier (sub-1G muted, 1G neutral, 10G blue, 100G emerald, 400G+ amber) and the RS badge sits inline after the IX name
  3. IP addresses align consistently across rows via grid layout, are selectable as plain text, and have a copy-to-clipboard button
  4. The IX presence section header shows aggregate bandwidth across all listed connections
  5. The same layout improvements apply to both the network detail page (`detail_net.templ`) and the IX detail page (`detail_ix.templ`)
**Plans**: TBD
**UI hint**: yes

## Progress

**Execution Order:**
Phases execute in numeric order: 25 -> 26 -> 27

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 25. Streaming RPCs | 3/3 | Complete    | 2026-03-25 |
| 26. Stream Resume & Incremental Filters | 1/1 | Complete    | 2026-03-25 |
| 27. IX Presence UI Polish | 0/TBD | Not started | - |
