# Requirements: PeeringDB Plus

**Defined:** 2026-03-24
**Core Value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.

## v1.4 Requirements

Requirements for Web UI milestone. Each maps to roadmap phases.

### Search

- [x] **SRCH-01**: User can type in a search box on the homepage and see results update instantly as they type
- [x] **SRCH-02**: Search results are grouped by type (Networks, IXPs, Facilities, Organizations, Campuses) with visual type indicators
- [x] **SRCH-03**: User can enter a numeric value to look up a network by ASN directly
- [x] **SRCH-04**: Each type group shows a count badge of matching results
- [ ] **SRCH-05**: User can navigate search results with keyboard (arrow keys to move, Enter to select)

### Record Detail

- [x] **DETL-01**: User can view a full detail page for any Network, IXP, Facility, Organization, or Campus
- [x] **DETL-02**: Related records (e.g., network's IX presences, facilities, contacts) appear in collapsible sections
- [x] **DETL-03**: Related record sections load on first expand, not on initial page load
- [x] **DETL-04**: Detail pages show computed summary statistics (e.g., "present at 47 IXPs, 23 facilities")
- [x] **DETL-05**: Related records cross-link to their own detail pages

### Comparison

- [x] **COMP-01**: User can compare two ASNs on a dedicated /compare page with two input fields
- [x] **COMP-02**: Comparison results show shared IXPs, facilities, and campuses where both networks are present
- [x] **COMP-03**: Shared IXP results display port speeds and IP addresses for both networks
- [x] **COMP-04**: User can toggle between shared-only view and full side-by-side view
- [x] **COMP-05**: User can initiate comparison from a network detail page via a "Compare with..." button

### Design

- [x] **DSGN-01**: All pages are styled with Tailwind CSS with a polished, visually appealing design
- [x] **DSGN-02**: Layout is mobile-responsive and works on all screen sizes
- [x] **DSGN-03**: Every page has a clean, shareable URL that captures the full page state
- [x] **DSGN-04**: Dark mode is supported with system preference detection and manual toggle
- [x] **DSGN-05**: Smooth CSS transitions on search results, collapsible sections, and page changes
- [x] **DSGN-06**: Loading indicators appear during HTMX requests
- [x] **DSGN-07**: Styled 404 and 500 error pages match the overall design

## Future Requirements

Deferred to future milestones. Tracked but not in current roadmap.

### API Surfaces

- **GRPC-01**: Expose data via gRPC (entproto)

### Advanced Search

- **ASRCH-01**: Multi-field advanced search UI (GraphQL/REST already covers this)
- **ASRCH-02**: Full-text search with FTS5 (only if LIKE performance proves insufficient)

### Visualization

- **VIZ-01**: Map visualization of facility locations
- **VIZ-02**: Data export UI (REST/GraphQL APIs already provide this)

### Extended Comparison

- **XCOMP-01**: Multi-ASN comparison (>2 ASNs)

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| User accounts / authentication | Read-only public mirror, login-free is an advantage |
| Data modification / write UI | This is a read-only mirror of PeeringDB |
| Real-time streaming | Periodic sync is sufficient |
| Mobile app | Web-first, responsive design covers mobile |
| SPA / client-side framework | Server-rendered with htmx covers all interaction needs |
| Pagination of search results | Cap results per type, encourage query refinement |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| SRCH-01 | Phase 14 | Complete |
| SRCH-02 | Phase 14 | Complete |
| SRCH-03 | Phase 14 | Complete |
| SRCH-04 | Phase 14 | Complete |
| SRCH-05 | Phase 17 | Pending |
| DETL-01 | Phase 15 | Complete |
| DETL-02 | Phase 15 | Complete |
| DETL-03 | Phase 15 | Complete |
| DETL-04 | Phase 15 | Complete |
| DETL-05 | Phase 15 | Complete |
| COMP-01 | Phase 16 | Complete |
| COMP-02 | Phase 16 | Complete |
| COMP-03 | Phase 16 | Complete |
| COMP-04 | Phase 16 | Complete |
| COMP-05 | Phase 16 | Complete |
| DSGN-01 | Phase 13 | Complete |
| DSGN-02 | Phase 13 | Complete |
| DSGN-03 | Phase 13 | Complete |
| DSGN-04 | Phase 17 | Complete |
| DSGN-05 | Phase 17 | Complete |
| DSGN-06 | Phase 17 | Complete |
| DSGN-07 | Phase 17 | Complete |

**Coverage:**
- v1.4 requirements: 22 total
- Mapped to phases: 22
- Unmapped: 0

---
*Requirements defined: 2026-03-24*
*Last updated: 2026-03-24 after roadmap creation*
