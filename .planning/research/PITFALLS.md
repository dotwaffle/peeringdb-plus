# Domain Pitfalls

**Domain:** Web UI density overhaul -- dense tables, client-side sorting, Leaflet maps, emoji country flags -- in an existing templ + htmx + Tailwind Go application
**Researched:** 2026-03-26
**Milestone:** v1.11 Web UI Density & Interactivity

## Critical Pitfalls

Mistakes that cause rewrites, broken user experiences, or fundamental architectural conflicts with the existing htmx patterns.

### Pitfall 1: sortable.js Not Auto-Initializing on htmx Fragment Loads

**What goes wrong:** The standard `sortable.min.js` initializes on `DOMContentLoaded`. Tables loaded later via htmx `hx-get` (collapsible section fragments) are never initialized -- clicking headers does nothing.

**Why it happens:** htmx injects HTML fragments after page load. The sortable library does not know about new elements unless it uses a MutationObserver or explicit re-initialization.

**Consequences:** Every sortable table inside a collapsible section (IX participants, facility networks, etc.) would be non-sortable. Only top-level tables rendered in the initial page load would work.

**Prevention:** Use `sortable.auto.min.js` (1.36KB gzipped) instead of `sortable.min.js`. The `.auto` variant includes a MutationObserver that detects new `<table class="sortable">` elements added to the DOM and automatically attaches sort handlers. This is explicitly designed for dynamic content injection scenarios.

**Detection:** Open a detail page, expand a collapsible section, click a table header. If nothing happens, the wrong variant is loaded.

### Pitfall 2: Leaflet Map Container Height Zero on Hidden Elements

**What goes wrong:** If the map `<div>` is inside a collapsed `<details>` element, Leaflet initializes with zero height because the container is `display: none`. The map renders as a 0px-tall invisible element. Even after opening the section, the tiles are corrupted or missing.

**Why it happens:** Leaflet calculates its viewport dimensions on initialization. A hidden container reports 0x0 dimensions. Leaflet cannot recover from this without `map.invalidateSize()`.

**Consequences:** Maps inside collapsible sections appear broken -- either invisible or showing grey tiles with misaligned markers.

**Prevention:** Render the map OUTSIDE of collapsible `<details>` sections. The map should be a top-level section on the detail page, always visible. This matches the design intent (the map provides a geographic overview, not a drill-down detail). If a map must be inside a collapsible section, call `map.invalidateSize()` on the `toggle` event of the `<details>` element.

**Detection:** Map shows grey rectangles or no tiles after expanding a section.

### Pitfall 3: Emoji Flags Not Rendering on Windows Without Segoe UI Emoji

**What goes wrong:** Regional Indicator Symbol pairs render as two-letter codes ("U" + "S") instead of a flag emoji on some Windows configurations or older Android devices.

**Why it happens:** Emoji flag rendering depends on the OS and font stack. Windows historically uses Segoe UI Emoji which supports flag rendering on Windows 10+. Older Windows versions or stripped-down installations may lack the font. Some Android devices also have incomplete flag emoji support.

**Consequences:** Instead of a visual flag, users see the two-letter regional indicator symbols, which look like random Unicode characters.

**Prevention:** Always render the country code text alongside or as a fallback next to the flag. Structure: `<span title="US">flag-emoji</span>` or `<td>flag-emoji <span class="text-neutral-500 text-xs">US</span></td>`. This way, even if the flag does not render, the country code is visible. Do not rely solely on the emoji for conveying country information.

**Detection:** Test on a Windows machine without Segoe UI Emoji installed, or check with a font that does not include Regional Indicator Symbols.

### Pitfall 4: N+1 Queries When Fetching Facility Lat/Lng for IX/Network Maps

**What goes wrong:** For each IX facility association, a separate query fetches the actual Facility entity to get lat/lng. With 50 facilities, that is 50 queries.

**Why it happens:** The junction entities (`IxFacility`, `NetworkFacility`) store the name, city, and country but NOT latitude/longitude. The lat/lng lives on the `Facility` entity itself. A naive implementation queries each facility individually.

**Consequences:** IX detail page with 50 facilities makes 50+ SQL queries instead of 1. On SQLite this is still fast (~1ms each), but it is architecturally wrong and will show up in OTel traces as span explosion.

**Prevention:** Collect all facility IDs from the junction entities into a slice, then batch-query with `facility.IDIn(ids...)`. One query returns all facilities with their lat/lng.

**Detection:** OTel traces show dozens of `ent.Facility.Query` spans per detail page load. Alternatively, enable SQLite query logging and count queries.

## Moderate Pitfalls

### Pitfall 5: data-sort Attribute Missing on Numeric Columns

**What goes wrong:** Speed column displays "10G" as text. sortable.js sorts it lexicographically: "100G" < "10G" < "1G" (string comparison). ASN column sorts "13335" < "2" because "1" < "2" as strings.

**Prevention:** Add `data-sort` attribute with the raw numeric value to every cell that displays a formatted number:
```html
<td data-sort="10000"><span class="text-blue-400 font-mono">10G</span></td>
<td data-sort="13335"><span class="font-mono">AS13335</span></td>
```
sortable.js prefers `data-sort` over cell text content when present.

### Pitfall 6: CARTO Tile Attribution Missing

**What goes wrong:** CARTO basemaps require attribution to OpenStreetMap and CARTO. Missing attribution violates OSM's license (ODbL) and CARTO's terms of service.

**Prevention:** Always include the attribution string in the L.tileLayer options:
```javascript
attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> &copy; <a href="https://carto.com/attributions">CARTO</a>'
```

### Pitfall 7: Map Initialization Race with Theme Detection

**What goes wrong:** The map JS reads `document.documentElement.classList.contains('dark')` to pick the tile URL. If the dark mode class is toggled AFTER the map initializes (user clicks dark mode toggle), the map shows light tiles in dark mode or vice versa.

**Prevention:** For v1.11, accept this as a known limitation. The user can refresh the page. Toggling dark mode while viewing a map is an uncommon action. If it becomes an issue, add a MutationObserver on the `<html>` class list to swap tile layers. This is a follow-up, not a blocker.

### Pitfall 8: CopyableIP Component Breaking Table Cell Layout

**What goes wrong:** The existing `CopyableIP` component is a `<div>` with flexbox layout, copy icon SVG, and "Copied!" feedback span. Placing a `<div>` inside a `<td>` can cause layout issues, and the hover icon may overflow the cell.

**Prevention:** Adapt `CopyableIP` to use `<span>` wrapper instead of `<div>` for table contexts, or create a `CopyableIPInline` variant. Keep the SVG icon and "Copied!" message but constrain with `inline-flex` and `whitespace-nowrap`.

### Pitfall 9: Leaflet CSS Conflicting with Tailwind Reset

**What goes wrong:** Tailwind's CSS reset (via the browser CDN) may strip default styling from Leaflet elements (popups, controls, attribution). Leaflet controls become unstyled boxes.

**Prevention:** Leaflet CSS should be loaded BEFORE the Tailwind `<script>` tag so Tailwind's reset applies first and Leaflet's styles override where needed. Alternatively, the Tailwind browser CDN does not apply a full Preflight reset the same way the PostCSS version does, so this may not be an issue. Test early.

**Detection:** Map zoom controls (+/-) appear as unstyled text. Popup has no border or background.

## Minor Pitfalls

### Pitfall 10: PeeringDB Facilities Without Lat/Lng

**What goes wrong:** Not all PeeringDB facilities have latitude and longitude populated. The map shows fewer pins than expected, or the map section renders for an IX where zero facilities have coordinates.

**Prevention:** Filter `MapPoints` to only include facilities where both `Latitude` and `Longitude` are non-nil and non-zero. Only render the map section when `len(MapPoints) > 0`. Document in the UI that some facilities may not appear on the map.

### Pitfall 11: MarkerCluster CSS Not Loaded

**What goes wrong:** MarkerCluster requires two CSS files (`MarkerCluster.css` and `MarkerCluster.Default.css`). Missing CSS causes cluster circles to appear as invisible/unstyled elements.

**Prevention:** Include both CSS files in `layout.templ` `<head>`. The Default.css provides the standard blue/green cluster circle styling. Without it, clusters are functional but invisible.

### Pitfall 12: sortable CSS Sort Indicators Invisible in Dark Mode

**What goes wrong:** sortable.js default CSS uses black arrows for sort direction indicators. In dark mode (dark background), these arrows are invisible.

**Prevention:** Override the sort indicator CSS to use `currentColor` instead of hardcoded colors. The custom CSS in `layout.templ` should use `border-bottom-color: currentColor` (or `border-top-color`) so the arrows inherit the text color of the `<th>` element, which is already styled for dark mode by Tailwind.

### Pitfall 13: Excessive CDN Requests on Layout

**What goes wrong:** Adding Leaflet CSS, Leaflet JS, MarkerCluster CSS (x2), MarkerCluster JS, sortable CSS, and sortable JS adds 7 CDN requests to every page load, even pages without maps or tables.

**Prevention:** Accept this for v1.11. All CDN resources are small, cached aggressively by browsers, and served from global CDNs (jsDelivr, CARTO). The total payload increase is ~57KB gzipped. If it becomes a concern, self-host the assets in `/static/` (same pattern as htmx.min.js) and let the Go embed bundle them. Or use lazy-loading (`<link rel="preload" as="style">`) for Leaflet assets on pages without maps. This is optimization, not a blocker.

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Mitigation |
|-------------|---------------|------------|
| Country flag utility | Pitfall 3 (rendering on Windows) | Always show country code alongside emoji. |
| Dense table layout conversion | Pitfall 5 (missing data-sort), Pitfall 8 (CopyableIP in tables), Pitfall 12 (dark mode sort arrows) | Add data-sort to all numeric cells. Adapt CopyableIP for inline table use. Override sort indicator CSS. |
| sortable.js integration | Pitfall 1 (auto-init on htmx fragments) | Use `sortable.auto.min.js` variant with MutationObserver. |
| Leaflet map on facility page | Pitfall 2 (height zero), Pitfall 6 (attribution), Pitfall 9 (Tailwind CSS conflict), Pitfall 10 (missing lat/lng) | Render map outside collapsible sections. Include attribution. Test CSS order. Filter nil coordinates. |
| Leaflet map on IX/network page | Pitfall 4 (N+1 queries), Pitfall 10 (missing lat/lng), Pitfall 11 (MarkerCluster CSS) | Batch-query facility lat/lng. Filter nil coords. Include both MarkerCluster CSS files. |
| Dark mode map tiles | Pitfall 7 (theme toggle race) | Accept as known limitation for v1.11. Document for follow-up. |
| Comparison map | Pitfall 4 (N+1 queries) | Same batch-query pattern as IX/network maps. |

## Sources

- [sortable library MutationObserver docs](https://github.com/tofsjonas/sortable) -- `.auto` variant behavior
- [Leaflet invalidateSize](https://leafletjs.com/reference.html#map-invalidatesize) -- required for hidden containers
- [Unicode Regional Indicator rendering](https://en.wikipedia.org/wiki/Regional_indicator_symbol) -- OS/font dependency for flag display
- [CARTO attribution requirements](https://carto.com/attributions) -- OSM + CARTO required
- Current codebase: `layout.templ` (CDN loading), `detail_shared.templ` (CopyableIP), `detail.go` (query patterns)
