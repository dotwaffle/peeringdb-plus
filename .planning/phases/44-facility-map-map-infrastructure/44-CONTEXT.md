# Phase 44: Facility Map & Map Infrastructure - Context

**Gathered:** 2026-03-26
**Status:** Ready for planning

<domain>
## Phase Boundary

Add an interactive Leaflet map to facility detail pages showing the facility's geographic location. Establish the shared map component, CDN infrastructure, and tile theming for all subsequent map phases (45, 46). Facilities without lat/lng data render normally with no map.

</domain>

<decisions>
## Implementation Decisions

### Map Component Design
- **D-01:** Map appears above collapsible sections (after detail fields and notes, before Networks/IXPs/Carriers). Natural reading order: identity -> location -> relationships.
- **D-02:** Map height is responsive: 200px on mobile, 350px on desktop. Implemented via Tailwind responsive classes.
- **D-03:** Map is full content width with rounded corners, matching existing detail section styling.
- **D-04:** Default zoom level ~10 (region level) for single facility pins. Shows the wider area for geographic context.
- **D-05:** Zoom +/- buttons visible (standard Leaflet controls). Discoverable across all devices.
- **D-06:** Scroll-wheel zoom disabled until the user clicks the map (`scrollWheelZoom: false`, enable on click). Prevents accidental zoom while scrolling the page.

### Tile Provider & Theming
- **D-07:** CARTO tiles: Voyager (light mode), Dark Matter (dark mode). Free keyless public CDN endpoint (`basemaps.cartocdn.com`). No API key needed.
- **D-08:** Map tile layer swaps live on dark mode toggle. Listen for the existing dark mode toggle event in layout.templ, swap tile URL between Voyager and Dark Matter.
- **D-09:** Standard OpenStreetMap + CARTO attribution via Leaflet's default attribution control (bottom-right). Required by OSM/CARTO terms.

### Pin & Popup Behavior
- **D-10:** Default Leaflet marker (standard blue teardrop pin). Per REQUIREMENTS Out of Scope: "Standard Leaflet markers with popups are sufficient for v1.11."
- **D-11:** Popup shows facility name, address (city/country), and network/IX counts. Richer than name-only.
- **D-12:** Popup is closed by default on single-facility maps — user clicks pin to open. Cleaner initial view.
- **D-13:** Popup always includes a link to the facility detail page. Consistent template across all map contexts (own page, IX page, network page in Phase 45).

### Leaflet Delivery
- **D-14:** Leaflet JS and CSS loaded via CDN `<link>`/`<script>` in layout.templ `<head>`. Matches existing CDN pattern (Tailwind browser, htmx, flag-icons).
- **D-15:** Pin to Leaflet 1.9.x (latest stable). Mature, well-documented.
- **D-16:** Create a shared templ MapContainer component that accepts Go params (lat/lng/zoom/markers) and renders init JS. Reusable across facility, IX, network, and compare pages in phases 44-46.

### Missing Lat/Lng Handling
- **D-17:** If lat/lng are both 0.0 or missing, the map div does not render at all. No placeholder, no message, no empty container. Per ROADMAP success criteria.
- **D-18:** Treat (0.0, 0.0) as missing data. No real PeeringDB facility is at null island.

### Map Data Plumbing
- **D-19:** Lat/lng passed from Go to JS via templ script params. templ component accepts float64 lat/lng as Go parameters, renders them into a `<script>` block via templ's script support. Type-safe, no extra fetch.
- **D-20:** FacilityDetail struct needs Latitude/Longitude float64 fields added. Detail handler query needs to select these from the ent Facility entity (fields already exist in ent schema).

### Accessibility
- **D-21:** Basic ARIA: `aria-label` on map container ("Map showing facility location"), `role="application"`. Screen readers get descriptive text.
- **D-22:** No full keyboard navigation for map panning — beyond scope. Leaflet's default keyboard zoom support is sufficient.

### Claude's Discretion
- Map container border/shadow treatment in dark mode
- Exact responsive height breakpoint (could use `h-[200px] md:h-[350px]` or similar)
- Whether the shared MapContainer component accepts a markers slice or individual lat/lng for single-pin mode
- Leaflet marker image CDN URL (standard Leaflet marker images from unpkg)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Templates
- `internal/web/templates/detail_fac.templ` — Facility detail page (map insertion point)
- `internal/web/templates/detail_shared.templ` — Shared components (pattern for new MapContainer component)
- `internal/web/templates/layout.templ` — `<head>` section for Leaflet CDN links; dark mode toggle JS for tile swapping

### Data Types & Handlers
- `internal/web/templates/detailtypes.go` — FacilityDetail struct (needs Latitude/Longitude fields)
- `internal/web/detail.go` — Facility detail handler (needs lat/lng query enrichment)

### Ent Schema
- `ent/schema/facility.go` — Facility entity with existing Latitude/Longitude float64 fields

### Requirements
- `.planning/REQUIREMENTS.md` — MAP-01, MAP-04, MAP-05

### Prior Phase Context
- `.planning/phases/43-dense-tables-with-sorting-and-flags/43-CONTEXT.md` — CDN delivery pattern, dark mode pattern

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- Dark mode toggle JS in layout.templ — pattern for detecting theme changes (tile swap hook)
- `detailBadgeClasses()` color mapping — accent colors per entity type (could inform popup styling)
- `formatFacLocation()` — city+country formatter for popup content

### Established Patterns
- CDN delivery in layout.templ `<head>` (Tailwind browser, htmx, flag-icons from Phase 43)
- templ `script` blocks for component-scoped JS (copyToClipboard pattern)
- Collapsible sections with htmx lazy-loading — map sits above these

### Integration Points
- Leaflet CSS/JS links added to layout.templ `<head>`
- New MapContainer templ component in detail_shared.templ (or new map.templ file)
- FacilityDetail struct enriched with Latitude/Longitude
- Facility detail handler query enriched to select lat/lng fields
- Dark mode toggle listener extended to swap tile layers

</code_context>

<specifics>
## Specific Ideas

- Map appears only on facility pages in this phase — IX/network/compare maps are Phase 45
- Shared MapContainer component designed for reuse: accepts markers data, renders one or many pins
- Region-level zoom (~10) gives geographic context for a single facility

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 44-facility-map-map-infrastructure*
*Context gathered: 2026-03-26*
