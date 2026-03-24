# Requirements: PeeringDB Plus

**Defined:** 2026-03-24
**Core Value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.

## v1.4 Requirements

Requirements for Web UI milestone. Each maps to roadmap phases.

### Search

- [ ] **SRCH-01**: User can type in a search box on the homepage and see results update instantly as they type
- [ ] **SRCH-02**: Search results are grouped by type (Networks, IXPs, Facilities, Organizations, Campuses) with visual type indicators
- [ ] **SRCH-03**: User can enter a numeric value to look up a network by ASN directly
- [ ] **SRCH-04**: Each type group shows a count badge of matching results
- [ ] **SRCH-05**: User can navigate search results with keyboard (arrow keys to move, Enter to select)

### Record Detail

- [ ] **DETL-01**: User can view a full detail page for any Network, IXP, Facility, Organization, or Campus
- [ ] **DETL-02**: Related records (e.g., network's IX presences, facilities, contacts) appear in collapsible sections
- [ ] **DETL-03**: Related record sections load on first expand, not on initial page load
- [ ] **DETL-04**: Detail pages show computed summary statistics (e.g., "present at 47 IXPs, 23 facilities")
- [ ] **DETL-05**: Related records cross-link to their own detail pages

### Comparison

- [ ] **COMP-01**: User can compare two ASNs on a dedicated /compare page with two input fields
- [ ] **COMP-02**: Comparison results show shared IXPs, facilities, and campuses where both networks are present
- [ ] **COMP-03**: Shared IXP results display port speeds and IP addresses for both networks
- [ ] **COMP-04**: User can toggle between shared-only view and full side-by-side view
- [ ] **COMP-05**: User can initiate comparison from a network detail page via a "Compare with..." button

### Design

- [ ] **DSGN-01**: All pages are styled with Tailwind CSS with a polished, visually appealing design
- [ ] **DSGN-02**: Layout is mobile-responsive and works on all screen sizes
- [ ] **DSGN-03**: Every page has a clean, shareable URL that captures the full page state
- [ ] **DSGN-04**: Dark mode is supported with system preference detection and manual toggle
- [ ] **DSGN-05**: Smooth CSS transitions on search results, collapsible sections, and page changes
- [ ] **DSGN-06**: Loading indicators appear during HTMX requests
- [ ] **DSGN-07**: Styled 404 and 500 error pages match the overall design

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
| SRCH-01 | — | Pending |
| SRCH-02 | — | Pending |
| SRCH-03 | — | Pending |
| SRCH-04 | — | Pending |
| SRCH-05 | — | Pending |
| DETL-01 | — | Pending |
| DETL-02 | — | Pending |
| DETL-03 | — | Pending |
| DETL-04 | — | Pending |
| DETL-05 | — | Pending |
| COMP-01 | — | Pending |
| COMP-02 | — | Pending |
| COMP-03 | — | Pending |
| COMP-04 | — | Pending |
| COMP-05 | — | Pending |
| DSGN-01 | — | Pending |
| DSGN-02 | — | Pending |
| DSGN-03 | — | Pending |
| DSGN-04 | — | Pending |
| DSGN-05 | — | Pending |
| DSGN-06 | — | Pending |
| DSGN-07 | — | Pending |

**Coverage:**
- v1.4 requirements: 22 total
- Mapped to phases: 0
- Unmapped: 22

---
*Requirements defined: 2026-03-24*
*Last updated: 2026-03-24 after initial definition*
