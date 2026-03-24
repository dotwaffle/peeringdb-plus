# Roadmap: PeeringDB Plus

## Milestones

- ✅ **v1.0 PeeringDB Plus** — Phases 1-3 (shipped 2026-03-22)
- ✅ **v1.1 REST API & Observability** — Phases 4-6 (shipped 2026-03-23)
- ✅ **v1.2 Quality, Incremental Sync & CI** — Phases 7-10 (shipped 2026-03-24)
- ✅ **v1.3 PeeringDB API Key Support** — Phases 11-12 (shipped 2026-03-24)
- 🚧 **v1.4 Web UI** — Phases 13-17 (in progress)

## Phases

<details>
<summary>✅ v1.0 PeeringDB Plus (Phases 1-3) — SHIPPED 2026-03-22</summary>

- [x] Phase 1: Data Foundation (7/7 plans) — completed 2026-03-22
- [x] Phase 2: GraphQL API (4/4 plans) — completed 2026-03-22
- [x] Phase 3: Production Readiness (3/3 plans) — completed 2026-03-22

See: `.planning/milestones/v1.0-ROADMAP.md` for full details.

</details>

<details>
<summary>✅ v1.1 REST API & Observability (Phases 4-6) — SHIPPED 2026-03-23</summary>

- [x] Phase 4: Observability Foundations (2/2 plans) — completed 2026-03-22
- [x] Phase 5: entrest REST API (3/3 plans) — completed 2026-03-22
- [x] Phase 6: PeeringDB Compatibility Layer (3/3 plans) — completed 2026-03-22

See: `.planning/milestones/v1.1-ROADMAP.md` for full details.

</details>

<details>
<summary>✅ v1.2 Quality, Incremental Sync & CI (Phases 7-10) — SHIPPED 2026-03-24</summary>

- [x] Phase 7: Lint & Code Quality (2/2 plans) — completed 2026-03-23
- [x] Phase 8: Incremental Sync (3/3 plans) — completed 2026-03-23
- [x] Phase 9: Golden File Tests & Conformance (2/2 plans) — completed 2026-03-23
- [x] Phase 10: CI Pipeline & Public Access (2/2 plans) — completed 2026-03-24

See: `.planning/milestones/v1.2-ROADMAP.md` for full details.

</details>

<details>
<summary>✅ v1.3 PeeringDB API Key Support (Phases 11-12) — SHIPPED 2026-03-24</summary>

- [x] Phase 11: API Key & Rate Limiting (2/2 plans) — completed 2026-03-24
- [x] Phase 12: Conformance Tooling Integration (1/1 plan) — completed 2026-03-24

See: `.planning/milestones/v1.3-ROADMAP.md` for full details.

</details>

### v1.4 Web UI (In Progress)

**Milestone Goal:** A polished, interactive web interface for exploring PeeringDB data with live search, detailed record views, and network comparison.

- [ ] **Phase 13: Foundation** — templ + Tailwind + htmx scaffolding, base layout, route integration
- [ ] **Phase 14: Live Search** — Homepage search with instant results grouped by type
- [ ] **Phase 15: Record Detail Pages** — Full detail views for all 5 entity types with lazy-loaded related records
- [ ] **Phase 16: ASN Comparison** — Compare two networks showing shared IXPs, facilities, and campuses
- [ ] **Phase 17: Polish & Accessibility** — Dark mode, transitions, keyboard navigation, error pages

## Phase Details

### Phase 13: Foundation
**Goal**: The web UI infrastructure is in place -- templ templates compile, Tailwind CSS generates styles, htmx is vendored, static assets are embedded, and the base layout renders on every route
**Depends on**: Phase 12
**Requirements**: DSGN-01, DSGN-02, DSGN-03
**Success Criteria** (what must be TRUE):
  1. Visiting any web URL in a browser renders a styled HTML page with consistent header, navigation, and footer
  2. The layout adapts correctly on mobile, tablet, and desktop screen widths
  3. Every page has a clean, bookmarkable URL that reloads to the same content (no JavaScript-only state)
  4. `templ generate` and Tailwind CSS compilation are integrated into the build pipeline and CI
**Plans**: 2 plans
Plans:
- [x] 13-01-PLAN.md -- Create internal/web package with templates, handler, embedded htmx, and tests
- [ ] 13-02-PLAN.md -- Wire web handler into main.go, update middleware, add templ CI drift detection
**UI hint**: yes

### Phase 14: Live Search
**Goal**: Users can find any PeeringDB record by typing in a search box on the homepage and seeing results appear instantly, grouped by type
**Depends on**: Phase 13
**Requirements**: SRCH-01, SRCH-02, SRCH-03, SRCH-04
**Success Criteria** (what must be TRUE):
  1. User types in the homepage search box and sees matching results appear within 300ms, updating as they type
  2. Results are visually grouped by type (Networks, IXPs, Facilities, Organizations, Campuses) with distinct type indicators
  3. Entering a numeric value shows the matching network by ASN at the top of results
  4. Each type group displays a count badge showing how many records matched
  5. Clicking a search result navigates to that record's detail page
**Plans**: TBD
**UI hint**: yes

### Phase 15: Record Detail Pages
**Goal**: Users can view complete information for any PeeringDB record with organized sections and navigate between related records
**Depends on**: Phase 14
**Requirements**: DETL-01, DETL-02, DETL-03, DETL-04, DETL-05
**Success Criteria** (what must be TRUE):
  1. User can view a full detail page for any Network, IXP, Facility, Organization, or Campus by navigating to its URL
  2. Related records (e.g., a network's IX presences, facilities, contacts) appear in collapsible sections that load their content on first expand
  3. Detail pages show computed summary statistics (e.g., "present at 47 IXPs, 23 facilities") in a visible header area
  4. Related records are clickable links that navigate to their own detail pages
**Plans**: TBD
**UI hint**: yes

### Phase 16: ASN Comparison
**Goal**: Users can compare two networks to see where they share presence, answering "where can we peer?"
**Depends on**: Phase 15
**Requirements**: COMP-01, COMP-02, COMP-03, COMP-04, COMP-05
**Success Criteria** (what must be TRUE):
  1. User can enter two ASNs on a dedicated /compare page and see their shared IXPs, facilities, and campuses
  2. Shared IXP results display port speeds and IP addresses for both networks at each exchange
  3. User can toggle between a shared-only view (default) and a full side-by-side view of all presences
  4. User can initiate a comparison from any network detail page via a "Compare with..." button that pre-fills one ASN
  5. The comparison URL captures both ASNs, making results shareable via link
**Plans**: TBD
**UI hint**: yes

### Phase 17: Polish & Accessibility
**Goal**: The web UI feels polished and professional with smooth interactions, accessibility support, and graceful error handling
**Depends on**: Phase 16
**Requirements**: SRCH-05, DSGN-04, DSGN-05, DSGN-06, DSGN-07
**Success Criteria** (what must be TRUE):
  1. User can navigate search results using keyboard (arrow keys to move, Enter to select) without touching the mouse
  2. Dark mode activates automatically based on system preference and can be toggled manually, persisting across sessions
  3. Search results, collapsible sections, and page transitions have smooth CSS animations
  4. A loading indicator appears during any htmx request so the user knows the system is working
  5. Visiting an invalid URL shows a styled 404 page, and server errors show a styled 500 page, both matching the overall design
**Plans**: TBD
**UI hint**: yes

## Progress

**Execution Order:**
Phases execute in numeric order: 13 → 14 → 15 → 16 → 17

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1. Data Foundation | v1.0 | 7/7 | Complete | 2026-03-22 |
| 2. GraphQL API | v1.0 | 4/4 | Complete | 2026-03-22 |
| 3. Production Readiness | v1.0 | 3/3 | Complete | 2026-03-22 |
| 4. Observability Foundations | v1.1 | 2/2 | Complete | 2026-03-22 |
| 5. entrest REST API | v1.1 | 3/3 | Complete | 2026-03-22 |
| 6. PeeringDB Compatibility Layer | v1.1 | 3/3 | Complete | 2026-03-22 |
| 7. Lint & Code Quality | v1.2 | 2/2 | Complete | 2026-03-23 |
| 8. Incremental Sync | v1.2 | 3/3 | Complete | 2026-03-23 |
| 9. Golden File Tests & Conformance | v1.2 | 2/2 | Complete | 2026-03-23 |
| 10. CI Pipeline & Public Access | v1.2 | 2/2 | Complete | 2026-03-24 |
| 11. API Key & Rate Limiting | v1.3 | 2/2 | Complete | 2026-03-24 |
| 12. Conformance Tooling Integration | v1.3 | 1/1 | Complete | 2026-03-24 |
| 13. Foundation | v1.4 | 1/2 | In Progress|  |
| 14. Live Search | v1.4 | 0/? | Not started | - |
| 15. Record Detail Pages | v1.4 | 0/? | Not started | - |
| 16. ASN Comparison | v1.4 | 0/? | Not started | - |
| 17. Polish & Accessibility | v1.4 | 0/? | Not started | - |
