# Roadmap: PeeringDB Plus

## Milestones

- [x] **v1.0 MVP** - Phases 1-3 (shipped 2026-03-22)
- [x] **v1.1 REST API & Observability** - Phases 4-6 (shipped 2026-03-23)
- [x] **v1.2 Quality, Incremental Sync & CI** - Phases 7-10 (shipped 2026-03-24)
- [x] **v1.3 PeeringDB API Key Support** - Phases 11-12 (shipped 2026-03-24)
- [x] **v1.4 Web UI** - Phases 13-17 (shipped 2026-03-24)
- [x] **v1.5 Tech Debt & Observability** - Phases 18-20 (shipped 2026-03-24)
- [x] **v1.6 ConnectRPC / gRPC API** - Phases 21-24 (shipped 2026-03-25)
- [x] **v1.7 Streaming RPCs & UI Polish** - Phases 25-27 (shipped 2026-03-25)
- [ ] **v1.8 Terminal CLI Interface** - Phases 28-31 (in progress)

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

<details>
<summary>v1.7 Streaming RPCs & UI Polish (Phases 25-27) - SHIPPED 2026-03-25</summary>

- [x] **Phase 25: Streaming RPCs** - Proto definitions, code generation, and 13 streaming handlers with batched keyset pagination (completed 2026-03-25)
- [x] **Phase 26: Stream Resume & Incremental Filters** - since_id resume and updated_since timestamp filtering on streaming RPCs (completed 2026-03-25)
- [x] **Phase 27: IX Presence UI Polish** - Field labels, speed colors, RS badge, IP alignment, copyable text, and aggregate bandwidth (completed 2026-03-25)

</details>

- [ ] **Phase 28: Terminal Detection & Infrastructure** - Content negotiation, User-Agent detection, rendering framework, help text, and error pages for terminal clients
- [ ] **Phase 29: Network Detail (Reference Implementation)** - Network entity terminal renderer with whois-style header, IX/facility tables, colored speed tiers, and cross-reference paths
- [ ] **Phase 30: Entity Types, Search & Formats** - Terminal renderers for remaining 5 entity types, search results, ASN comparison, plus plain text, JSON, and WHOIS output modes
- [ ] **Phase 31: Differentiators & Shell Integration** - One-line summary, section filtering, width control, freshness footer, and downloadable bash/zsh completions

## Phase Details

<details>
<summary>v1.7 Streaming RPCs & UI Polish (Phases 25-27) - SHIPPED 2026-03-25</summary>

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
**Plans:** 2/2 plans complete

Plans:
- [x] 27-01-PLAN.md -- Shared helpers (speed colors, copyable IPs, bandwidth section) + NetworkIXLansList redesign
- [x] 27-02-PLAN.md -- IXParticipantsList redesign + visual verification checkpoint

</details>

### Phase 28: Terminal Detection & Infrastructure
**Goal**: Terminal clients (curl, wget, HTTPie) hitting any /ui/ URL receive appropriate text responses instead of HTML, with explicit format overrides available
**Depends on**: Phase 27
**Requirements**: DET-01, DET-02, DET-03, DET-04, DET-05, RND-01, RND-18, NAV-01, NAV-02, NAV-03, NAV-04
**Success Criteria** (what must be TRUE):
  1. Running `curl peeringdb-plus.fly.dev/ui/` returns CLI help text listing available endpoints, query parameters, and usage examples -- not an HTML page
  2. Running `curl peeringdb-plus.fly.dev/ui/asn/13335` returns ANSI-colored text output (not HTML), while the same URL in a browser returns the existing web UI unchanged
  3. Appending `?T` or `?format=plain` to any /ui/ URL returns plain ASCII output with no ANSI escape codes, and `?format=json` returns JSON
  4. Requesting a nonexistent path like `curl /ui/asn/99999999` returns a text-formatted 404 error (not HTML), and server errors return text-formatted 500 errors
  5. Setting `?nocolor` suppresses all ANSI escape codes in terminal output while preserving layout
**Plans:** 3/3 plans complete

Plans:
- [x] 28-01-PLAN.md -- termrender package foundation: detection logic, renderer engine, style definitions
- [x] 28-02-PLAN.md -- renderPage integration, PageContent.Data wiring, help text and error renderers
- [x] 28-03-PLAN.md -- Root handler terminal detection, error handler wiring, integration tests

### Phase 29: Network Detail (Reference Implementation)
**Goal**: Network engineers can look up any network by ASN from the terminal and see a comprehensive, well-formatted detail view with colored status indicators and navigable cross-references
**Depends on**: Phase 28
**Requirements**: RND-02, RND-12, RND-13, RND-14, RND-15, RND-16
**Success Criteria** (what must be TRUE):
  1. Running `curl /ui/asn/13335` displays a whois-style key-value header (name, ASN, type, policy, website, etc.) followed by tabular IX presences and facility lists with Unicode box drawing
  2. Port speeds in IX presence tables are color-coded matching the web UI tiers (gray sub-1G, neutral 1G, blue 10G, emerald 100G, amber 400G+) and route server peers show a colored [RS] badge
  3. Peering policy is color-coded (green for Open, yellow for Selective, red for Restrictive) in the network header
  4. Aggregate bandwidth is displayed in the network header and per-IX section headers
  5. Each entity reference (IX name, facility name) includes its ID or path (e.g., `/ui/ix/123`) so the user can follow up with another curl command
**Plans:** 2 plans

Plans:
- [ ] 29-01-PLAN.md -- Data plumbing: NetworkDetail struct extension, eager IX/facility fetching, type-switch dispatch, formatting helpers
- [ ] 29-02-PLAN.md -- RenderNetworkDetail full implementation with whois-style output and comprehensive tests

### Phase 30: Entity Types, Search & Formats
**Goal**: All six PeeringDB entity types, search, and comparison are accessible from the terminal, with plain text, JSON, and WHOIS as alternative output formats
**Depends on**: Phase 29
**Requirements**: RND-03, RND-04, RND-05, RND-06, RND-07, RND-08, RND-09, RND-10, RND-11, RND-17
**Success Criteria** (what must be TRUE):
  1. Running `curl /ui/ix/{id}`, `/ui/fac/{id}`, `/ui/org/{id}`, `/ui/campus/{id}`, and `/ui/carrier/{id}` each returns a formatted terminal detail view appropriate to that entity type
  2. Running `curl /ui/?q=equinix` returns search results grouped by entity type as a text list, matching the web UI search behavior
  3. Running `curl /ui/compare/13335/15169` renders a terminal comparison of two networks showing shared IXPs, facilities, and campuses
  4. Appending `?format=whois` to any detail URL returns RPSL-like key-value output suitable for parsing by network automation scripts
  5. All alternative format modes (?T, ?format=json, ?format=whois) produce consistent output across all entity types -- not just networks

### Phase 31: Differentiators & Shell Integration
**Goal**: Power users can customize terminal output (summary mode, section filtering, width control) and install shell completions for a native CLI feel
**Depends on**: Phase 30
**Requirements**: DIF-01, DIF-02, DIF-03, DIF-04, SHL-01, SHL-02, SHL-03
**Success Criteria** (what must be TRUE):
  1. Running `curl /ui/asn/13335?format=short` returns a single-line summary suitable for scripting (e.g., `AS13335 | Cloudflare, Inc. | Open | 2847 prefixes`)
  2. Every terminal response includes a data freshness timestamp footer showing when PeeringDB data was last synced
  3. Appending `?section=ix,fac` to a detail URL renders only the IX presences and facilities sections, omitting other sections
  4. Appending `?w=120` adapts table rendering to 120-column width, and `?w=80` produces narrower tables that fit standard terminals
  5. Running `curl /ui/completions/bash` and `curl /ui/completions/zsh` downloads shell completion scripts, and the help text includes alias/function setup instructions

## Progress

**Execution Order:**
Phases execute in numeric order: 28 -> 29 -> 30 -> 31

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 28. Terminal Detection & Infrastructure | 3/3 | Complete    | 2026-03-25 |
| 29. Network Detail (Reference Implementation) | 0/2 | Not started | - |
| 30. Entity Types, Search & Formats | 0/? | Not started | - |
| 31. Differentiators & Shell Integration | 0/? | Not started | - |
