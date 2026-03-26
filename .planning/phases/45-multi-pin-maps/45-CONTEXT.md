# Phase 45: Multi-Pin Maps - Context

**Gathered:** 2026-03-26
**Status:** Ready for planning

<domain>
## Phase Boundary

Add multi-pin maps to IX detail, network detail, and ASN comparison pages using the shared MapContainer from Phase 44. Includes marker clustering for dense regions, colored circleMarkers for shared vs unique facilities on comparison maps, auto-fit bounds, and a map legend on comparison pages.

</domain>

<decisions>
## Implementation Decisions

### Marker Clustering
- **D-01:** Use Leaflet.markercluster plugin delivered via CDN in layout.templ alongside Leaflet. Standard clustering with animated expand on click and numbered circles.
- **D-02:** Custom dark-mode-aware cluster styling. Override default markercluster CSS with neutral/emerald colors matching the app palette. Different backgrounds for dark vs light mode.
- **D-03:** No marker count limit — trust clustering to handle any dataset size. PeeringDB's largest datasets (~200 facilities per network) are well within markercluster's capability.

### Colored Pins (Comparison)
- **D-04:** Three-color scheme on comparison maps: shared facilities in emerald (app accent), Network A unique in sky blue, Network B unique in amber. Three distinct categories.
- **D-05:** Use L.circleMarker with fillColor on ALL multi-pin maps (IX, network, compare). Default teardrop markers only on single-facility maps (Phase 44). Consistent multi-pin style.
- **D-06:** Comparison map includes a simple legend in a map corner: colored dot + label for each category (Shared, AS{X} only, AS{Y} only).

### Map Placement
- **D-07:** Multi-pin maps appear above collapsible sections on all pages — same position as the Phase 44 facility map. Consistent placement across IX, network, and comparison pages.
- **D-08:** On comparison results page, map appears after header/stats, before the IXP/Facility/Campus comparison sections. Visual overview of geographic overlap.

### Bounds & Auto-Fit
- **D-09:** Use Leaflet fitBounds() with maxZoom capped at 13 (city level). Prevents over-zooming when only 1-2 nearby pins exist.
- **D-10:** If a multi-pin map has zero mappable facilities (all lack valid coordinates), hide the map entirely — no container rendered.
- **D-11:** Show count of unmapped facilities below the map when some facilities lack coordinates. E.g., "3 facilities not shown (no location data)." Honest about completeness.

### Multi-Pin Data Loading
- **D-12:** Eager-load facility coordinates with page data. Add Latitude/Longitude to existing facility queries in IX/network/comparison detail handlers. No extra requests.
- **D-13:** Add Latitude/Longitude fields to ALL facility row structs: IXFacilityRow, NetworkFacRow, OrgFacilityRow, CampusFacilityRow, CarrierFacilityRow, and comparison facility types. Consistent enrichment for future extensibility.

### Popup Content
- **D-14:** IX/network map popups: facility name + city/country + link to facility detail page. No network/IX counts (irrelevant in this context).
- **D-15:** Comparison map popups: facility name + city/country + which networks are present (e.g., "Equinix DC2 — Washington, US — Cloudflare + Google") + link to facility detail page.

### Map Height
- **D-16:** Multi-pin maps are taller than single-facility maps: mobile 250px, desktop 450px (vs Phase 44's 200px/350px). More vertical space for wider geographic spread and clustering.

### Claude's Discretion
- Exact markercluster CSS override values for dark/light modes
- CircleMarker radius and stroke styling for multi-pin maps
- Legend position (top-right, bottom-left, etc.) and styling
- Whether "N facilities not shown" message is inside or below the map container
- How comparison facility data includes network presence info for popup content

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Templates
- `internal/web/templates/detail_ix.templ` — IX detail page (map insertion point, above collapsible sections)
- `internal/web/templates/detail_net.templ` — Network detail page (map insertion point)
- `internal/web/templates/compare.templ` — Comparison results page (map insertion point, compare data types)
- `internal/web/templates/detail_shared.templ` — Shared MapContainer component from Phase 44
- `internal/web/templates/layout.templ` — CDN links for markercluster CSS/JS

### Data Types & Handlers
- `internal/web/templates/detailtypes.go` — Row structs needing Latitude/Longitude (IXFacilityRow, NetworkFacRow, etc.)
- `internal/web/templates/comparetypes.go` — Comparison data types needing facility coordinates
- `internal/web/detail.go` — IX/network detail handlers (need coordinate queries)
- `internal/web/compare.go` — Comparison handler (needs coordinate data for facility pins)

### Prior Phase Context
- `.planning/phases/44-facility-map-map-infrastructure/44-CONTEXT.md` — MapContainer component, tile theming, Leaflet CDN, data flow pattern, single-facility popup design

### Requirements
- `.planning/REQUIREMENTS.md` — MAP-02, MAP-03

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets (from Phase 44)
- Shared MapContainer templ component — accepts Go params, renders map init JS
- CARTO Voyager/Dark tile layer with live theme swap
- Leaflet 1.9.x via CDN
- Scroll-wheel-zoom-on-click pattern

### Established Patterns
- Eager-loading facility data in detail handlers (existing pattern for terminal/JSON rendering: IXPresences, FacPresences fields)
- Compare handler already loads SharedFacilities and AllFacilities with city/country
- `compareRowClasses()` — existing shared/unique distinction pattern (opacity-based)

### Integration Points
- MapContainer component extended to accept markers slice with color/category metadata
- Markercluster CSS/JS added to layout.templ CDN links
- Row struct enrichment in detailtypes.go + comparetypes.go
- Detail handler queries enriched to join facility lat/lng
- Compare handler extended to pass facility coordinates to template

</code_context>

<specifics>
## Specific Ideas

- CircleMarkers on multi-pin maps (clean colored dots) vs teardrop markers on single-facility maps (recognizable pin) — deliberate visual distinction between "here is one location" and "here is a distribution of locations"
- Unmapped facility count shown honestly — users should know the map isn't the complete picture

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 45-multi-pin-maps*
*Context gathered: 2026-03-26*
