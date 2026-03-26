# Project Research Summary

**Project:** PeeringDB Plus -- v1.11 Web UI Density & Interactivity
**Domain:** Web UI enhancement for data-dense network infrastructure portal
**Researched:** 2026-03-26
**Confidence:** HIGH

## Executive Summary

v1.11 is a UI density and interactivity milestone that transforms the existing templ + htmx detail pages from vertical card-style layouts into proper dense data tables with client-side sorting, adds interactive Leaflet maps with facility pins, and introduces emoji country flags as a visual scanning aid. The scope is well-bounded: no new Go dependencies are required, no backend architecture changes, and all additions are either pure Go utility functions or lightweight CDN-delivered client-side libraries (Leaflet 1.9.4 at 42KB, MarkerCluster at 10KB, sortable-tablesort at under 1KB).

The recommended approach follows a strict dependency chain: country flag utility first (zero risk, unblocks all flag columns), then data layer additions (lat/lng and flag fields on view model structs), then the largest change -- converting all detail page child-entity lists from `divide-y` div layouts to semantic `<table>` elements with sortable columns -- and finally Leaflet map integration starting with the simplest case (single-pin facility page) and progressing to multi-pin IX/network/comparison pages. All geographic data already exists in the ent schema (facility lat/lng is synced from PeeringDB); the work is surfacing it through queries and templates.

The primary risks are operational, not architectural: sortable.js must use the `.auto` variant (MutationObserver) or htmx-loaded fragment tables will not be sortable; Leaflet maps must render outside collapsible `<details>` elements to avoid zero-height initialization; facility lat/lng queries must be batched (not N+1) to avoid span explosion in OTel traces; and emoji flags must always display the country code alongside the emoji because Windows does not render Regional Indicator Symbols as flags. None of these are hard problems -- they are known gotchas with documented prevention strategies.

## Key Findings

### Recommended Stack

No new Go dependencies. Three small client-side libraries delivered via CDN, plus a 6-line pure Go function for country flags. See [STACK.md](STACK.md) for full details.

**Core technologies:**
- **Leaflet 1.9.4**: Interactive maps with clickable pins and popups -- the standard open-source mapping library (42KB gzipped, stable since 2023, do NOT use 2.0.0-alpha)
- **Leaflet.markercluster 1.5.3**: Marker clustering for pages with many facility pins -- official Leaflet plugin, required for IX/network pages with 50+ facility locations
- **sortable-tablesort 4.1.7**: Client-side column sorting -- 899 bytes gzipped, zero dependencies, MutationObserver auto-init variant works with htmx fragment loading
- **CARTO basemaps**: Dark and light map tiles -- no API key, no registration, free with CC-BY 4.0 attribution, native dark/light variants eliminate CSS invert hack
- **CountryFlag() Go function**: Unicode Regional Indicator Symbol math -- 6 lines of Go, no library needed, converts ISO 3166-1 alpha-2 codes to emoji flags server-side

### Expected Features

See [FEATURES.md](FEATURES.md) for full feature landscape.

**Must have (table stakes):**
- Dense columnar table layouts replacing current card/div lists (3-4x vertical space reduction)
- Sortable table columns with numeric sort keys (data-sort attributes)
- Country emoji flags as visual scanning aid alongside country codes
- Responsive table behavior (hide low-priority columns on mobile)

**Should have (differentiators):**
- Interactive Leaflet map with facility pins on detail pages (PeeringDB has no map)
- Marker clustering for large IXes with many facility locations
- Clickable map popups linking to facility detail pages
- Dark mode map tiles matching app theme
- Comparison page map with colored pins (shared vs unique facilities)

**Defer (v2+):**
- Server-side table sorting via htmx (unnecessary at current data volumes, all datasets under 2K rows)
- Paginated tables for child entities (bounded data, pagination adds complexity for no benefit)
- Search results layout density redesign (current card layout works, improve later)
- Map on search results page (scope creep)
- Drawing tools / measurement on map (out of scope for read-only mirror)

### Architecture Approach

The features integrate into the existing architecture with no structural changes. All data transformations happen server-side in Go (flag conversion, lat/lng extraction, MapPoint generation). Client-side libraries operate independently on the rendered DOM (sortable attaches to `<table class="sortable">`, Leaflet reads embedded GeoJSON from `<script type="application/json">`). See [ARCHITECTURE.md](ARCHITECTURE.md) for component boundaries and data flow.

**Major components:**
1. **countryflags.go** (new) -- `CountryFlag()` function and `MapPoint` type definition
2. **detail.go / search.go / compare.go** (modified) -- populate new fields (CountryFlag, Latitude, Longitude, MapPoints) in query functions, batch-query facility lat/lng for IX/network pages
3. **layout.templ** (modified) -- CDN script/link tags for Leaflet, MarkerCluster, sortable; custom sort indicator CSS for dark mode
4. **detail_*.templ** (modified, largest change) -- convert all child-entity lists from `divide-y` divs to `<table class="sortable">` with flag columns, data-sort attributes, and responsive hiding
5. **detail_shared.templ** (modified) -- new `MapSection` shared templ component; adapt `CopyableIP` for inline table cell use

### Critical Pitfalls

See [PITFALLS.md](PITFALLS.md) for full analysis with 13 identified pitfalls.

1. **sortable.js not auto-initializing on htmx fragments** -- Use `sortable.auto.min.js` (MutationObserver variant), not the standard `sortable.min.js`. Without this, every table inside a collapsible section is silently non-sortable.
2. **Leaflet map zero height in collapsed sections** -- Render maps as top-level page sections, never inside `<details>` elements. Leaflet calculates viewport on init; hidden containers report 0x0 dimensions.
3. **N+1 queries for facility lat/lng** -- Junction entities (IxFacility, NetworkFacility) lack lat/lng. Batch-query actual Facility entities with `facility.IDIn(ids...)` instead of per-facility queries. Otherwise OTel traces show span explosion.
4. **Missing data-sort on numeric columns** -- Speed "10G" sorts lexicographically without `data-sort="10000"`. ASN "13335" sorts after "2" as strings. Every formatted numeric cell needs a data-sort attribute.
5. **Emoji flags invisible on Windows** -- Always display country code text alongside the flag emoji. Windows renders Regional Indicator Symbols as two-letter codes, not flags.

## Implications for Roadmap

Based on research, suggested phase structure:

### Phase 1: Country Flag Utility and Data Layer Additions

**Rationale:** Zero dependencies on other features. Unblocks all subsequent phases that display flags. Type definition changes (adding fields to view model structs) must happen before any template work.
**Delivers:** `CountryFlag()` Go function with tests, `MapPoint` type, and updated view model structs with CountryFlag/Latitude/Longitude/MapPoints fields.
**Addresses:** Country emoji flags (table stakes), data plumbing for maps.
**Avoids:** Pitfall 3 (flag rendering) by establishing the pattern of always showing country code alongside emoji from the start.

### Phase 2: Dense Table Layouts with Sortable Columns

**Rationale:** Largest change by template line count and highest user-visible impact. Must happen before map integration because it restructures all detail page templates. Sortable tables require semantic `<table>` markup, so this conversion is a prerequisite.
**Delivers:** All detail page child-entity lists converted from div-based cards to dense sortable tables. CDN integration for sortable-tablesort. Flag columns populated. Data-sort attributes on all numeric cells. Dark mode sort indicator CSS. Updated `CopyableIP` for table cell use. Responsive column hiding.
**Uses:** sortable-tablesort 4.1.7 (CDN), CountryFlag() from Phase 1.
**Avoids:** Pitfall 1 (use `.auto` variant), Pitfall 5 (data-sort on every numeric cell), Pitfall 8 (CopyableIP inline adaptation), Pitfall 12 (currentColor sort indicators for dark mode).

### Phase 3: Facility Detail Page Map (Single Pin)

**Rationale:** Simplest map case -- single facility with its own lat/lng, no junction queries, no clustering complexity. Establishes the `MapSection` shared component and Leaflet CDN integration that all subsequent map work builds on.
**Delivers:** Leaflet + MarkerCluster CDN in layout.templ. `MapSection` templ component. Single-pin map on facility detail page with dark/light CARTO tiles. Conditional rendering when lat/lng exists.
**Uses:** Leaflet 1.9.4, CARTO basemaps.
**Avoids:** Pitfall 2 (render outside collapsible sections), Pitfall 6 (CARTO attribution), Pitfall 9 (CSS load order testing), Pitfall 10 (filter nil coordinates).

### Phase 4: Multi-Pin Maps (IX, Network, Comparison)

**Rationale:** Builds on established MapSection component from Phase 3. Adds the junction-entity-to-facility query pattern and MarkerCluster usage. Comparison map with colored pins is the most novel feature.
**Delivers:** Maps on IX detail pages (multi-pin from facility associations), network detail pages (multi-pin from facility presences), and comparison pages (colored pins for shared vs unique facilities). Batch facility lat/lng queries.
**Uses:** MapSection component from Phase 3, MarkerCluster for clustering.
**Avoids:** Pitfall 4 (batch-query with IDIn, not N+1), Pitfall 10 (filter nil coords), Pitfall 11 (both MarkerCluster CSS files).

### Phase 5: Search Results and Polish

**Rationale:** Low dependency, lower priority. Search results already work; this adds flag display and optional density improvements. Also addresses any dark mode map tile toggle edge cases identified during Phase 3-4 testing.
**Delivers:** Country flags in search result subtitles. Any remaining table or map polish identified during prior phases.
**Avoids:** Pitfall 7 (document dark mode toggle as known limitation, defer MutationObserver tile swap).

### Phase Ordering Rationale

- **Dependency chain drives order:** Types and utilities first (Phase 1), then the templates that consume them (Phase 2-5). Table conversion (Phase 2) before maps (Phase 3-4) because it restructures the templates that maps will be added to.
- **Risk gradient:** Each phase adds one category of complexity. Phase 1 is pure Go with zero risk. Phase 2 is template refactoring with a new JS library. Phase 3 introduces Leaflet with the simplest map case. Phase 4 adds query complexity and clustering. Phase 5 is polish.
- **Biggest value first:** Dense tables (Phase 2) deliver the largest UX improvement -- 3-4x vertical space reduction on every detail page. Maps (Phase 3-4) are differentiators but less impactful than fixing the fundamental data density problem.
- **Each phase is independently shippable:** The product improves after every phase. If the milestone needs to be cut short, Phase 1-2 alone deliver major value.

### Research Flags

Phases with standard patterns (skip research-phase):
- **Phase 1:** Pure Go utility function and struct field additions. Well-documented Unicode standard. No research needed.
- **Phase 2:** Table HTML conversion with Tailwind styling. Standard web development. sortable-tablesort behavior is well-documented. No research needed.
- **Phase 5:** Search result template changes. Trivial. No research needed.

Phases that may benefit from brief validation during planning:
- **Phase 3:** Leaflet initialization in templ templates. The pattern of embedding GeoJSON in `<script type="application/json">` and initializing Leaflet from a raw `<script>` tag is well-documented, but test the Tailwind CSS interaction (Pitfall 9) early. Brief validation, not full research.
- **Phase 4:** Multi-pin maps with batch facility queries. The ent query pattern (`IDIn`) is standard, but the junction-entity-to-facility join for IX/network geo data should be validated against the actual query structure in `detail.go`. Brief code review during planning is sufficient.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All libraries are stable, widely used, and well-documented. Leaflet 1.9.4 is the current stable line. sortable-tablesort is tiny and battle-tested. No new Go dependencies. |
| Features | HIGH | Feature set is clearly scoped against existing codebase analysis and competitor comparison (PeeringDB, Peercortex). Table stakes vs differentiators are well-defined. |
| Architecture | HIGH | Integrates into existing templ + htmx patterns with no structural changes. Data flow is straightforward: ent queries -> Go view models -> templ templates -> client-side JS. |
| Pitfalls | HIGH | 13 pitfalls identified with concrete prevention strategies. The critical ones (MutationObserver, zero-height map, N+1 queries) are well-known issues with documented solutions. |

**Overall confidence:** HIGH

### Gaps to Address

- **Tailwind CDN vs Leaflet CSS interaction:** Pitfall 9 flags a potential conflict between Tailwind's CSS reset and Leaflet control styling. The Tailwind browser CDN may not apply full Preflight reset, making this a non-issue -- but it should be tested in Phase 3 before committing to the CDN load order.
- **MarkerCluster dark mode styling:** MarkerCluster.Default.css uses blue/green cluster circles that may clash with the dark theme. Custom CSS overrides for `.marker-cluster-small/medium/large` background colors may be needed. Assess during Phase 4 implementation.
- **Actual data coverage for facility lat/lng:** The percentage of PeeringDB facilities with populated lat/lng is unknown. If coverage is low, maps will appear sparse. Check data coverage during Phase 3 to set expectations for the map feature's utility.

## Sources

### Primary (HIGH confidence)
- [Leaflet.js](https://leafletjs.com/) -- v1.9.4 stable, CDN links with SRI hashes
- [sortable-tablesort](https://github.com/tofsjonas/sortable) -- MutationObserver behavior, data-sort attributes, CDN
- [CARTO Basemaps](https://carto.com/basemaps) -- free dark/light tile variants, attribution requirements
- [Unicode Regional Indicator Symbols](https://en.wikipedia.org/wiki/Regional_indicator_symbol) -- flag emoji encoding standard
- [Leaflet MarkerCluster](https://github.com/Leaflet/Leaflet.markercluster) -- v1.5.3 clustering plugin

### Secondary (MEDIUM confidence)
- [CARTO attribution requirements](https://carto.com/attributions) -- CC-BY 4.0 licensing terms
- [Stadia Maps pricing](https://stadiamaps.com/pricing) -- alternative tile provider comparison (rejected)
- [OSM Tile Usage Policy](https://operations.osmfoundation.org/policies/tiles/) -- why not to use OSM tiles directly
- [htmx Table Sorting Pattern](https://dev.to/vladkens/table-sorting-and-pagination-with-htmx-3dh8) -- server-side sort approach (deferred)

### Tertiary (LOW confidence)
- MarkerCluster dark mode compatibility -- not explicitly documented, needs testing during implementation

---
*Research completed: 2026-03-26*
*Ready for roadmap: yes*
