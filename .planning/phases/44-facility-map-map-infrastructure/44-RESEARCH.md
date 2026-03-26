# Phase 44: Facility Map & Map Infrastructure - Research

**Researched:** 2026-03-26
**Domain:** Leaflet.js interactive maps, CARTO tile providers, templ script components, Go-to-JS data passing
**Confidence:** HIGH

## Summary

This phase adds an interactive Leaflet map to facility detail pages and establishes reusable map infrastructure for phases 45-46. The implementation touches four layers: CDN delivery (Leaflet JS/CSS in layout.templ), a shared templ MapContainer component, data plumbing (Latitude/Longitude fields in FacilityDetail struct and query), and dark mode tile swapping (CARTO Voyager/Dark Matter).

The technical surface is well-understood. Leaflet 1.9.4 is the latest stable release, CARTO tiles are free and keyless via `basemaps.cartocdn.com`, and the existing codebase already has established patterns for CDN delivery, dark mode toggling, and templ `script` components. The ent schema already has `Latitude` and `Longitude` as `Optional().Nillable()` float fields (`*float64` in generated Go code), so no schema changes are needed -- only the detail handler query and display struct need enrichment.

**Primary recommendation:** Use Leaflet 1.9.4 from unpkg CDN (matching existing CDN delivery pattern), CARTO raster tiles with `setUrl()` for live dark mode switching, and a templ `script` component for the map initialization function following the existing `copyToClipboard` pattern.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Map appears above collapsible sections (after detail fields and notes, before Networks/IXPs/Carriers). Natural reading order: identity -> location -> relationships.
- **D-02:** Map height is responsive: 200px on mobile, 350px on desktop. Implemented via Tailwind responsive classes.
- **D-03:** Map is full content width with rounded corners, matching existing detail section styling.
- **D-04:** Default zoom level ~10 (region level) for single facility pins. Shows the wider area for geographic context.
- **D-05:** Zoom +/- buttons visible (standard Leaflet controls). Discoverable across all devices.
- **D-06:** Scroll-wheel zoom disabled until the user clicks the map (`scrollWheelZoom: false`, enable on click). Prevents accidental zoom while scrolling the page.
- **D-07:** CARTO tiles: Voyager (light mode), Dark Matter (dark mode). Free keyless public CDN endpoint (`basemaps.cartocdn.com`). No API key needed.
- **D-08:** Map tile layer swaps live on dark mode toggle. Listen for the existing dark mode toggle event in layout.templ, swap tile URL between Voyager and Dark Matter.
- **D-09:** Standard OpenStreetMap + CARTO attribution via Leaflet's default attribution control (bottom-right). Required by OSM/CARTO terms.
- **D-10:** Default Leaflet marker (standard blue teardrop pin). Per REQUIREMENTS Out of Scope: "Standard Leaflet markers with popups are sufficient for v1.11."
- **D-11:** Popup shows facility name, address (city/country), and network/IX counts. Richer than name-only.
- **D-12:** Popup is closed by default on single-facility maps -- user clicks pin to open. Cleaner initial view.
- **D-13:** Popup always includes a link to the facility detail page. Consistent template across all map contexts (own page, IX page, network page in Phase 45).
- **D-14:** Leaflet JS and CSS loaded via CDN `<link>`/`<script>` in layout.templ `<head>`. Matches existing CDN pattern (Tailwind browser, htmx, flag-icons).
- **D-15:** Pin to Leaflet 1.9.x (latest stable). Mature, well-documented.
- **D-16:** Create a shared templ MapContainer component that accepts Go params (lat/lng/zoom/markers) and renders init JS. Reusable across facility, IX, network, and compare pages in phases 44-46.
- **D-17:** If lat/lng are both 0.0 or missing, the map div does not render at all. No placeholder, no message, no empty container.
- **D-18:** Treat (0.0, 0.0) as missing data. No real PeeringDB facility is at null island.
- **D-19:** Lat/lng passed from Go to JS via templ script params. templ component accepts float64 lat/lng as Go parameters, renders them into a `<script>` block via templ's script support. Type-safe, no extra fetch.
- **D-20:** FacilityDetail struct needs Latitude/Longitude float64 fields added. Detail handler query needs to select these from the ent Facility entity (fields already exist in ent schema).
- **D-21:** Basic ARIA: `aria-label` on map container ("Map showing facility location"), `role="application"`. Screen readers get descriptive text.
- **D-22:** No full keyboard navigation for map panning -- beyond scope. Leaflet's default keyboard zoom support is sufficient.

### Claude's Discretion
- Map container border/shadow treatment in dark mode
- Exact responsive height breakpoint (could use `h-[200px] md:h-[350px]` or similar)
- Whether the shared MapContainer component accepts a markers slice or individual lat/lng for single-pin mode
- Leaflet marker image CDN URL (standard Leaflet marker images from unpkg)

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| MAP-01 | User sees an interactive map on facility detail pages showing the facility's location | Leaflet 1.9.4 + CARTO tiles, FacilityDetail struct enrichment with Latitude/Longitude, templ MapContainer component, conditional rendering when coords present |
| MAP-04 | User can click map pins to see popup with facility name and navigate to detail page | Leaflet L.marker().bindPopup() with HTML content including facility name, location, counts, and `/ui/fac/{id}` link |
| MAP-05 | User sees map tiles switch between light/dark themes matching app dark mode | CARTO Voyager (light) / Dark Matter (dark) tile URLs, `tileLayer.setUrl()` called from dark mode toggle listener in layout.templ |
</phase_requirements>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Leaflet | 1.9.4 | Interactive map library | Latest stable (released 2023-05-18). 2.0.0-alpha.1 exists but is not stable. 1.9.4 is the production-recommended version per leafletjs.com/download.html |
| CARTO basemaps | N/A (CDN tiles) | Map tile provider | Free keyless raster tiles. Voyager (light) and Dark Matter (dark) styles. No API key required via `basemaps.cartocdn.com` |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| templ | v0.3.x (existing) | Server-side HTML templating | Already in stack. Use `script` keyword for map init function (matches existing `copyToClipboard` pattern) |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Leaflet 1.9.4 | Leaflet 2.0.0-alpha.1 | Alpha, API may change, not recommended for production |
| CARTO tiles | OpenStreetMap default tiles | OSM tiles have no dark mode variant. CARTO provides both light and dark. |
| CARTO tiles | Stadia Maps (Alidade) | Requires API key for production use. CARTO is keyless. |

**CDN URLs (verified):**
```html
<!-- Leaflet CSS -->
<link rel="stylesheet" href="https://unpkg.com/leaflet@1.9.4/dist/leaflet.css"/>
<!-- Leaflet JS -->
<script src="https://unpkg.com/leaflet@1.9.4/dist/leaflet.js"></script>
```

## Architecture Patterns

### Files Modified/Created
```
internal/web/templates/
  layout.templ           # ADD: Leaflet CSS/JS CDN links in <head>, dark mode tile swap hook
  detailtypes.go         # ADD: Latitude/Longitude float64 fields to FacilityDetail
  detail_fac.templ       # ADD: MapContainer call between Notes and collapsible sections
  map.templ              # NEW: MapContainer templ component + initMap script function
internal/web/
  detail.go              # MOD: queryFacility to populate Latitude/Longitude from ent entity
```

### Pattern 1: CDN Delivery in layout.templ `<head>`
**What:** Add Leaflet CSS and JS to the global `<head>` alongside existing CDN assets.
**When to use:** Always -- Leaflet is needed on any page that renders a map.
**Why global:** Maps appear on facility, IX, network, and compare pages across phases 44-46. Loading conditionally per-page adds complexity for no benefit (browser caches the CDN response).

```html
<!-- Existing pattern in layout.templ <head>: -->
<script src="https://cdn.jsdelivr.net/npm/@tailwindcss/browser@4"></script>
<script src="/static/htmx.min.js"></script>
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/flag-icons@7.5.0/css/flag-icons.min.css"/>

<!-- ADD: Leaflet (same pattern) -->
<link rel="stylesheet" href="https://unpkg.com/leaflet@1.9.4/dist/leaflet.css"
      integrity="sha256-p4NxAoJBhIIN+hmNHrzRCf9tD/miZyoHS5obTRR9BMY="
      crossorigin=""/>
<script src="https://unpkg.com/leaflet@1.9.4/dist/leaflet.js"
        integrity="sha256-20nQCchB9co0qIjJZRGuk2/Z9VM+kNiyxNV1lvTlZBo="
        crossorigin=""></script>
```

### Pattern 2: Templ Script Component for Map Init
**What:** Use templ's `script` keyword to define a map initialization function that accepts Go parameters (matching existing `copyToClipboard` pattern).
**When to use:** For passing typed Go data (lat, lng, zoom, marker info) to client-side JavaScript.

The project already uses the legacy `script` pattern for `copyToClipboard` in `detail_shared.templ`. Use the same approach for consistency:

```go
// map.templ

// initMap initializes a Leaflet map in the given container element.
// Parameters are JSON-encoded by templ and passed type-safely from Go.
script initMap(elementID string, lat float64, lng float64, zoom int, popupHTML string) {
    var isDark = document.documentElement.classList.contains('dark');
    var lightURL = 'https://{s}.basemaps.cartocdn.com/rastertiles/voyager/{z}/{x}/{y}{r}.png';
    var darkURL = 'https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png';

    var map = L.map(elementID, {
        scrollWheelZoom: false
    }).setView([lat, lng], zoom);

    var tileLayer = L.tileLayer(isDark ? darkURL : lightURL, {
        attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>, &copy; <a href="https://carto.com/attributions">CARTO</a>',
        subdomains: 'abcd',
        maxZoom: 19
    }).addTo(map);

    L.marker([lat, lng]).addTo(map).bindPopup(popupHTML);

    map.on('click', function() { map.scrollWheelZoom.enable(); });

    // Store reference for dark mode toggle
    window.__pdbMaps = window.__pdbMaps || [];
    window.__pdbMaps.push({ tileLayer: tileLayer, lightURL: lightURL, darkURL: darkURL });
}
```

### Pattern 3: Dark Mode Tile Swap Hook
**What:** Extend the existing dark mode toggle listener in layout.templ to also swap map tile URLs.
**When to use:** When any map is on the page.
**Integration point:** The existing dark mode toggle JS in layout.templ (lines 59-74) adds/removes the `dark` class. Add tile swap logic immediately after the class toggle.

```javascript
// After: html.classList.add/remove('dark')
// Add:
if (window.__pdbMaps) {
    var url = html.classList.contains('dark') ? darkURL : lightURL;
    window.__pdbMaps.forEach(function(m) {
        m.tileLayer.setUrl(html.classList.contains('dark') ? m.darkURL : m.lightURL);
    });
}
```

### Pattern 4: Conditional Map Rendering
**What:** Only render the map container when latitude/longitude are present and non-zero.
**When to use:** Every map render site.

```go
// In detail_fac.templ, between Notes and collapsible sections:
if data.Latitude != 0 || data.Longitude != 0 {
    @MapContainer(MapData{
        ID:        fmt.Sprintf("map-fac-%d", data.ID),
        Latitude:  data.Latitude,
        Longitude: data.Longitude,
        Zoom:      10,
        PopupHTML: facPopupHTML(data),
    })
}
```

### Pattern 5: MapContainer Component Design (Claude's Discretion)
**Recommendation:** Use a struct-based approach for forward compatibility with multi-marker maps in Phase 45.

```go
// MapData holds parameters for the shared map container component.
type MapData struct {
    // ID is the unique DOM element ID for this map instance.
    ID string
    // Latitude is the center latitude.
    Latitude float64
    // Longitude is the center longitude.
    Longitude float64
    // Zoom is the initial zoom level.
    Zoom int
    // PopupHTML is the HTML content for the single marker popup.
    PopupHTML string
}
```

For Phase 44, `MapContainer` accepts a single `MapData` with one lat/lng. Phase 45 will extend this with a markers slice for multi-pin maps. Designing as a struct from the start avoids a breaking change.

### Anti-Patterns to Avoid
- **Fetching map data via htmx/AJAX:** The lat/lng data is already available server-side during the detail page render. Do not add an extra round-trip to fetch coordinates.
- **Bundling Leaflet:** The project uses CDN delivery for all frontend assets. Do not npm-install or bundle Leaflet.
- **CSS filter dark mode:** Using CSS `filter: invert()` on tiles produces poor visual results. Use dedicated CARTO dark tiles instead.
- **Rendering an empty map container:** When lat/lng are missing, do not render a `<div id="map">` and hide it with CSS. Do not render it at all (D-17).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Map rendering | Custom canvas/SVG map | Leaflet 1.9.4 | Leaflet handles tile loading, projection, zoom, touch events, accessibility |
| Tile serving | Self-hosted tile server | CARTO CDN (basemaps.cartocdn.com) | Free, global CDN, no infrastructure to manage |
| Marker icons | Custom SVG markers | Leaflet default marker | Decision D-10 explicitly uses standard markers. Custom markers are out of scope per REQUIREMENTS. |
| Popup templating | Custom tooltip system | Leaflet L.marker.bindPopup() | Handles positioning, overflow, close button, accessible focus |
| Dark mode detection | Custom JS dark mode tracker | Read `document.documentElement.classList.contains('dark')` | Already the source of truth in the app's dark mode system |

## Common Pitfalls

### Pitfall 1: Leaflet Marker Icons Not Loading from CDN
**What goes wrong:** When Leaflet is loaded via CDN but marker images fail to load, showing broken image icons instead of the blue teardrop.
**Why it happens:** Leaflet auto-detects its image path from its own `<script>` src attribute. When using some CDN configurations or SRI integrity attributes, the path detection can fail (GitHub issue #9466).
**How to avoid:** When loading via CDN `<script>` tag (not bundler), the icon path detection works correctly. If broken icons appear, explicitly set `L.Icon.Default.imagePath` to `'https://unpkg.com/leaflet@1.9.4/dist/images/'` before creating markers.
**Warning signs:** Markers appear as broken image icons or transparent squares.

### Pitfall 2: Map Container Size Zero on Init
**What goes wrong:** Map tiles appear gray/blank because Leaflet initializes before the container has a rendered size.
**Why it happens:** If the map `<div>` is inside a hidden container (e.g., `display: none`, collapsed accordion), Leaflet calculates zero width/height.
**How to avoid:** The facility map renders above collapsible sections and is always visible on page load. No issue expected. For Phase 45 (if maps are inside collapsible sections), call `map.invalidateSize()` on expand.
**Warning signs:** Gray tiles, tiles only in top-left corner, map not filling container.

### Pitfall 3: Scroll Wheel Zoom Hijacking Page Scroll
**What goes wrong:** User scrolls the page, mouse passes over the map, and the page scroll stops while the map zooms instead.
**Why it happens:** Leaflet's default `scrollWheelZoom: true` captures wheel events on the map.
**How to avoid:** Set `scrollWheelZoom: false` in map options (D-06). Enable on click: `map.on('click', function() { map.scrollWheelZoom.enable(); })`.
**Warning signs:** Users complain about "getting stuck" scrolling past the map.

### Pitfall 4: Null Island (0, 0) Treated as Valid Coordinates
**What goes wrong:** Facilities with no geo data show a map centered on the Gulf of Guinea (latitude 0, longitude 0).
**Why it happens:** PeeringDB stores (0.0, 0.0) or NULL for facilities without coordinates. If the code treats any non-nil float64 as valid, null island appears.
**How to avoid:** Check both that lat/lng are non-nil AND non-zero (D-17, D-18). In the ent schema, the fields are `*float64` (nillable). In the FacilityDetail struct, use plain `float64` and only populate when both ent fields are non-nil and at least one is non-zero.
**Warning signs:** Maps appearing for facilities that clearly have no real location data.

### Pitfall 5: Tile Layer Race on Dark Mode Toggle
**What goes wrong:** Rapidly toggling dark mode causes tile layers to overlap or flicker as multiple `setUrl()` calls race.
**Why it happens:** `setUrl()` triggers async tile fetches. If called again before the first batch loads, both sets of tiles appear briefly.
**How to avoid:** `setUrl()` with default behavior (redraw=true) replaces the previous request. Leaflet handles this internally. No debounce needed for the toggle button use case.
**Warning signs:** Brief flash of light tiles visible through dark tile gaps during rapid toggling (cosmetic only).

## Code Examples

### Example 1: FacilityDetail Struct Enrichment
```go
// In detailtypes.go - add to FacilityDetail struct:
type FacilityDetail struct {
    // ... existing fields ...
    // Latitude is the facility's geographic latitude. Zero means no data.
    Latitude float64
    // Longitude is the facility's geographic longitude. Zero means no data.
    Longitude float64
}
```

### Example 2: Query Enrichment in detail.go
```go
// In queryFacility, after building the FacilityDetail struct:
if fac.Latitude != nil && fac.Longitude != nil {
    if *fac.Latitude != 0 || *fac.Longitude != 0 {
        data.Latitude = *fac.Latitude
        data.Longitude = *fac.Longitude
    }
}
```

### Example 3: CARTO Tile URLs
```
Light (Voyager):    https://{s}.basemaps.cartocdn.com/rastertiles/voyager/{z}/{x}/{y}{r}.png
Dark (Dark Matter): https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png
Subdomains:         a, b, c, d
Max zoom:           19
Attribution:        &copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>, &copy; <a href="https://carto.com/attributions">CARTO</a>
```

### Example 4: Conditional Map Rendering in Templ
```go
// In detail_fac.templ, between @DetailField("Notes", data.Notes) and <div class="space-y-3 pt-4">:
if data.Latitude != 0 || data.Longitude != 0 {
    @MapContainer(MapData{
        ID:        fmt.Sprintf("map-fac-%d", data.ID),
        Latitude:  data.Latitude,
        Longitude: data.Longitude,
        Zoom:      10,
        PopupHTML: facMapPopup(data),
    })
}
```

### Example 5: Popup HTML Generator
```go
// In map.templ - helper to build popup HTML string
func facMapPopup(data FacilityDetail) string {
    var b strings.Builder
    b.WriteString(fmt.Sprintf(`<div><strong><a href="/ui/fac/%d">%s</a></strong>`, data.ID, data.Name))
    loc := formatFacLocation(data.City, data.Country)
    if loc != "" {
        b.WriteString(fmt.Sprintf(`<br><span>%s</span>`, loc))
    }
    if data.NetCount > 0 || data.IXCount > 0 {
        b.WriteString(fmt.Sprintf(`<br><span>%d networks, %d IXPs</span>`, data.NetCount, data.IXCount))
    }
    b.WriteString(`</div>`)
    return b.String()
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Leaflet 1.x | Leaflet 2.0 alpha | 2025-08 | Alpha only. 1.9.4 remains the production recommendation. |
| CARTO Positron (light) | CARTO Voyager (light) | 2018 | Voyager is CARTO's recommended light basemap with labeled streets and colored land use |
| Manual tile URL construction | CARTO CDN with subdomains | Stable | URL pattern `{s}.basemaps.cartocdn.com` unchanged for years |
| CSS filter dark mode | Dedicated dark tile provider | N/A | CSS filter approach produces poor results. Dedicated Dark Matter tiles are purpose-built |

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) + enttest |
| Config file | None (Go convention) |
| Quick run command | `go test -count=1 -run TestFacility ./internal/web/` |
| Full suite command | `go test -race -count=1 ./internal/web/...` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| MAP-01 | Facility detail page with lat/lng renders map container div | unit (HTTP response body check) | `go test -count=1 -run TestFacilityDetail_MapRendered ./internal/web/` | Wave 0 |
| MAP-01 | Facility detail page without lat/lng omits map container | unit (HTTP response body check) | `go test -count=1 -run TestFacilityDetail_NoMapWithoutCoords ./internal/web/` | Wave 0 |
| MAP-04 | Map popup HTML contains facility name and detail link | unit (Go function test) | `go test -count=1 -run TestFacMapPopup ./internal/web/templates/` | Wave 0 |
| MAP-05 | Dark mode tile swap JS present in layout | unit (HTTP response body check) | `go test -count=1 -run TestLayout_MapDarkModeHook ./internal/web/` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test -count=1 -run TestFacility ./internal/web/`
- **Per wave merge:** `go test -race -count=1 ./internal/web/...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/web/detail_test.go` -- add facility map rendering tests (with and without coords)
- [ ] `internal/web/detail_test.go` -- update `seedAllTestData` to set Latitude/Longitude on test facility
- [ ] Popup HTML generation is testable as a pure Go function (no template rendering needed)

## Sources

### Primary (HIGH confidence)
- [Leaflet 1.9.4 download page](https://leafletjs.com/download.html) - Version confirmation, CDN URLs
- [Leaflet API reference](https://leafletjs.com/reference.html) - L.map, L.tileLayer, L.marker APIs
- [CARTO basemap-styles repo](https://github.com/CartoDB/basemap-styles) - Tile URL patterns, subdomains, attribution text
- [templ script templates docs](https://templ.guide/syntax-and-usage/script-templates/) - Script component syntax, parameter passing
- [unpkg Leaflet 1.9.4 dist/images](https://app.unpkg.com/leaflet@1.9.4/files/dist/images) - Marker icon CDN paths

### Secondary (MEDIUM confidence)
- [Leaflet GitHub issue #9466](https://github.com/Leaflet/Leaflet/issues/9466) - Default marker icon path issue (affects bundlers, not CDN delivery)
- [Leaflet GitHub issue #6659](https://github.com/Leaflet/Leaflet/issues/6659) - TileLayer setUrl flicker minimization
- [CARTO basemaps page](https://carto.com/basemaps) - Terms of use, free tier confirmation

### Tertiary (LOW confidence)
- None -- all findings verified against primary sources.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - Leaflet 1.9.4 is well-established, CARTO tiles are widely documented
- Architecture: HIGH - All patterns match existing codebase conventions (CDN delivery, templ script, dark mode toggle)
- Pitfalls: HIGH - Known issues documented from official Leaflet GitHub, verified against CDN delivery context

**Research date:** 2026-03-26
**Valid until:** 2026-06-26 (stable domain -- Leaflet 1.9.4 and CARTO tiles unlikely to change)
