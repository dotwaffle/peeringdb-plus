# Feature Landscape

**Domain:** Web UI density, sortable tables, country flags, interactive map
**Researched:** 2026-03-26
**Milestone:** v1.11 Web UI Density & Interactivity

## Table Stakes

Features users expect from a data-dense PeeringDB interface. Missing = product feels incomplete compared to PeeringDB itself or Peercortex.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Dense columnar table layout for child entities | PeeringDB and Peercortex both use tables, not card lists. Current layout wastes 3-4x vertical space per row. | Medium | Largest change by template line count. Every detail page's child lists need conversion from `divide-y` divs to `<table>`. |
| Sortable table columns | Users expect to sort IX participants by ASN, speed, name. PeeringDB has this. Standard UX for any data table. | Low | sortable.js handles everything. Add `class="sortable"` and proper `<table>`/`<thead>`/`<tbody>` structure. |
| Country emoji flags | Visual scanning aid. Peercortex shows flags. Raw country codes ("US", "DE") require mental parsing. | Low | Pure Go Unicode math on Regional Indicator Symbols. No library needed. ~15 lines of code. |
| Numeric sort keys via data-sort | Speed columns display "10G" but must sort as 10000. ASN columns must sort numerically, not lexicographically. | Low | Built into sortable.js via `data-sort` attribute on `<td>`. |
| Responsive table behavior | Must not break on mobile. Columns should hide gracefully on narrow viewports. | Low | Tailwind `hidden md:table-cell` on lower-priority columns (IPv6, RS badge). |

## Differentiators

Features that set the product apart. Not expected, but valued.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Interactive Leaflet map with facility pins | Visual geographic overview of where an IX/network is present. PeeringDB has no map. Peercortex has a basic one. | Medium | Requires geo data enrichment from facility lat/lng. CARTO dark/light tiles. |
| Marker clustering on map | Large IXes have 50+ facility locations. Unclustered pins overlap into an unreadable blob. | Low | MarkerCluster plugin, ~10KB, auto-clusters and animates. |
| Clickable map popups linking to detail pages | Click a facility pin, see name + link to detail page. | Low | Popup HTML rendered server-side, embedded in inline GeoJSON data. |
| Dark mode map tiles | App has dark mode; map should match without looking broken. | Low | CARTO `dark_all`/`light_all` tile URLs. Check `html.dark` class at init time. |
| Comparison page map with colored pins | Shared facilities in green, unique-to-one-network in grey/muted. Visual answer to "where do they overlap?" | Medium | Extends base map pattern with marker color differentiation. |
| Copy-to-clipboard in table cells | CopyableIP component already exists. Needs to work inside `<td>` elements preserving the click-to-copy and hover icon UX. | Low | Minor CSS/structure adjustment to existing `CopyableIP` templ component. |

## Anti-Features

Features to explicitly NOT build in this milestone.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| Server-side table sorting via htmx | Adds latency, complexity, breaks the existing load-once collapsible section pattern. Datasets are small (<2K rows max). | Use client-side sortable.js. Instant sort, no round-trip. |
| Paginated tables for child entities | Child entity counts are bounded (largest IX ~2K participants). Pagination adds UI complexity, multiple states, and URL management for no benefit at this scale. | Load all rows in one fragment. Let sortable.js handle in-DOM sorting. |
| Custom map tile server / self-hosted tiles | Massive infrastructure overhead to serve tile images from edge nodes. | Use CARTO CDN (free, no API key, native dark/light variants). |
| Map on every page regardless of data | Carriers and some orgs have no facility associations with lat/lng. An empty map is worse than no map. | Conditionally render map only when `MapPoints` slice is non-empty. |
| Full-text search from within map popups | Scope creep. Map is for visualization, not search. | Popup contains a link to the detail page. Search remains via the existing search bar. |
| CSS filter dark mode for map tiles | Inverts labels, making them unreadable. Also inverts markers and popups. | CARTO native dark tiles via separate tile URL. |
| GeoJSON endpoint / separate map data fetch | Data is already available in the page handler. Extra round-trip adds latency. | Embed GeoJSON inline in `<script type="application/json">`. |
| Drawing tools / measurement on map | Out of scope for a read-only data mirror. | Pins + popups + clustering only. |

## Feature Dependencies

```
CountryFlag utility function
  --> Flag columns in all detail page tables
  --> Flag in search result subtitle
  --> Flag in comparison table rows

Dense table layout (div-to-table conversion)
  --> Sortable tables (sortable.js needs <table>/<thead>/<tbody>)
  --> data-sort attributes for numeric columns

sortable.js + CSS in layout.templ
  --> All sortable tables across all detail pages

Lat/lng data enrichment from Facility ent entity
  --> MapPoint generation in detail.go
  --> Map section rendering in templates

MapSection shared templ component
  --> Facility detail map (single pin)
  --> IX detail map (multiple pins from facilities)
  --> Network detail map (multiple pins from facility presences)
  --> Comparison map (colored pins for shared vs unique)

CARTO tile provider URLs
  --> Dark mode map tile switching

MarkerCluster plugin CDN
  --> IX/network/comparison maps with many pins
```

## MVP Recommendation

Prioritize based on dependencies and value delivery:

1. **Country flag utility** -- zero risk, immediate visual improvement everywhere, blocks nothing
2. **Type definition additions** -- add fields to Go structs, no template changes yet
3. **Dense table layouts with sortable columns** -- highest density improvement, most user-visible change
4. **Leaflet map on facility detail page** -- simplest map case (single pin, facility's own lat/lng)
5. **Leaflet map on IX detail page** -- moderate complexity (multiple pins from facility associations)
6. **Leaflet map on network detail page** -- same pattern as IX
7. **Comparison map with colored pins** -- builds on established map pattern, highest novelty

Defer: Search results layout density redesign. Current search results are vertically stacked cards that work well for scanning. Can be improved in a later milestone without blocking other features.

## Sources

- Current codebase analysis: `detail_ix.templ`, `detail_net.templ`, `detail_fac.templ`, `compare.templ`, `search_results.templ`
- PeeringDB web interface (baseline table stakes comparison)
- [sortable by tofsjonas](https://github.com/tofsjonas/sortable) -- 899B gzipped table sort library
- [Leaflet.js](https://leafletjs.com/) -- v1.9.4 stable mapping library
- [CARTO Basemaps](https://carto.com/basemaps) -- free dark/light tile provider
