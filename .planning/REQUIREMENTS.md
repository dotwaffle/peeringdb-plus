# Requirements: PeeringDB Plus

**Defined:** 2026-03-26
**Core Value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.

## v1.11 Requirements

Requirements for the Web UI Density & Interactivity milestone. Each maps to roadmap phases.

### Density

- [ ] **DENS-01**: User sees detail page child-entity lists (IX participants, facilities, contacts) as dense columnar tables instead of multi-line card entries
- [ ] **DENS-02**: User sees parsed city and country in dedicated columns on entity tables
- [ ] **DENS-03**: User sees responsive column hiding on narrow screens (low-priority columns drop instead of horizontal scroll)
- [ ] **DENS-04**: User sees search results in a denser layout with country/city information
- [ ] **DENS-05**: User sees ASN comparison results (shared IXPs, facilities, campuses) as dense tables

### Sorting

- [ ] **SORT-01**: User can sort key table columns (name, ASN, speed, country) by clicking column headers
- [ ] **SORT-02**: User sees sort direction indicators on sortable column headers
- [ ] **SORT-03**: User sees tables pre-sorted by a sensible default (IX participants by ASN, facilities by country)

### Flags

- [ ] **FLAG-01**: User sees SVG country flag icons alongside country codes in entity tables
- [ ] **FLAG-02**: User sees country flags in search result entries

### Maps

- [ ] **MAP-01**: User sees an interactive map on facility detail pages showing the facility's location
- [ ] **MAP-02**: User sees an interactive map on IX and network detail pages showing all associated facility locations with clustering
- [ ] **MAP-03**: User sees an interactive map on ASN comparison page with colored pins (shared vs unique facilities)
- [ ] **MAP-04**: User can click map pins to see popup with facility name and navigate to detail page
- [ ] **MAP-05**: User sees map tiles switch between light/dark themes matching app dark mode

## Future Requirements

Deferred to future milestones.

### Server-Side Sorting

- **SSORT-01**: Server-side table sorting via htmx for paginated datasets exceeding client-side limits

### Map Enhancements

- **MAPE-01**: Map on search results page showing locations of results
- **MAPE-02**: Drawing tools / measurement on map for distance calculation

### Cross-Surface Consistency

- **XSURF-01**: Same entity returns identical data across GraphQL, REST, PeeringDB compat, and gRPC surfaces
- **XSURF-02**: Golden file tests for gRPC responses (after filter coverage is complete)

## Out of Scope

| Feature | Reason |
|---------|--------|
| Server-side pagination for tables | PeeringDB data volumes are bounded (largest IX ~2500 participants). Client-side handling is sufficient. |
| Search results map | Scope creep — search works well without a map. Defer to future. |
| Map drawing/measurement tools | Read-only data mirror, not a network planning tool. |
| Custom map markers (SVG icons) | Standard Leaflet markers with popups are sufficient for v1.11. |
| Full Peercortex clone | Inspiration, not duplication. Focus on density, sorting, flags, maps. |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| DENS-01 | — | Pending |
| DENS-02 | — | Pending |
| DENS-03 | — | Pending |
| DENS-04 | — | Pending |
| DENS-05 | — | Pending |
| SORT-01 | — | Pending |
| SORT-02 | — | Pending |
| SORT-03 | — | Pending |
| FLAG-01 | — | Pending |
| FLAG-02 | — | Pending |
| MAP-01 | — | Pending |
| MAP-02 | — | Pending |
| MAP-03 | — | Pending |
| MAP-04 | — | Pending |
| MAP-05 | — | Pending |

**Coverage:**
- v1.11 requirements: 15 total
- Mapped to phases: 0
- Unmapped: 15 ⚠️

---
*Requirements defined: 2026-03-26*
*Last updated: 2026-03-26 after initial definition*
