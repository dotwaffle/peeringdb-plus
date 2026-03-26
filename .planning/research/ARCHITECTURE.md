# Architecture Patterns

**Domain:** Web UI density, sortable tables, Leaflet maps, emoji country flags
**Researched:** 2026-03-26

## Recommended Architecture

The v1.11 features integrate into the existing templ + htmx + Tailwind architecture with minimal new dependencies. No Go libraries are added. Three small JS/CSS assets are loaded via CDN (matching the Tailwind CDN pattern already established). All data transformations happen server-side in Go.

### Component Boundaries

| Component | Responsibility | Communicates With |
|-----------|---------------|-------------------|
| `internal/web/templates/*.templ` | Dense table markup, map container, flag rendering | Data types in `detailtypes.go`, `comparetypes.go`, `searchtypes.go` |
| `internal/web/detail.go` | Query lat/lng from ent, populate new type fields (Latitude, Longitude, CountryFlag) | ent client, template data types |
| `internal/web/templates/layout.templ` | CDN script/link tags for Leaflet, MarkerCluster, sortable.css | Browser loads at page level |
| `internal/web/countryflags.go` (new) | Pure Go function: ISO 3166-1 alpha-2 code to emoji flag string | Called by detail.go, search.go, compare.go when populating template data |
| Client-side JS (inline in templ) | Leaflet map initialization, GeoJSON layer from `<script type="application/json">` block | Reads data attributes / embedded JSON from server-rendered HTML |
| `sortable` library (CDN) | Client-side table column sorting via `class="sortable"` on `<table>` | Operates on DOM, no htmx interaction needed |

### Data Flow

```
[ent DB] --> detail.go queries facility lat/lng, country code
         --> countryflags.go converts "US" to unicode regional indicators
         --> template data struct gets Latitude, Longitude, CountryFlag fields
         --> templ renders:
             - <table class="sortable"> with dense columnar rows
             - data-sort attributes on <td> for numeric sort keys
             - emoji flags in dedicated <td> column
             - <div id="map"> container
             - <script type="application/json" id="map-data"> with GeoJSON
         --> browser loads:
             - sortable.min.js attaches click handlers to <th> in .sortable tables
             - inline JS reads #map-data JSON, creates Leaflet map with markers
```

## Integration Patterns

### Pattern 1: Country Code to Emoji Flag (Go Server-Side)

**What:** Convert ISO 3166-1 alpha-2 country codes to Unicode regional indicator symbol pairs.

**When:** Every template data struct that has a `Country` field also gets a computed `CountryFlag` field.

**Why server-side:** Emoji flags are pure Unicode -- no JS needed. Computing in Go means the flag renders in the initial HTML, works in terminal mode (ANSI/WHOIS), and avoids a flash of unstyled content. The algorithm is trivial (two character shifts), no external library needed.

**Implementation:**

```go
// CountryFlag converts an ISO 3166-1 alpha-2 country code to its emoji flag.
// Returns empty string for invalid or empty input.
func CountryFlag(code string) string {
    if len(code) != 2 {
        return ""
    }
    code = strings.ToUpper(code)
    r1 := rune(code[0]) + 0x1F1A5
    r2 := rune(code[1]) + 0x1F1A5
    return string([]rune{r1, r2})
}
```

**Confidence:** HIGH -- This is a well-documented Unicode standard (Regional Indicator Symbols U+1F1E6 to U+1F1FF). The algorithm is deterministic and has been validated across multiple sources. Every modern browser and terminal renders these correctly.

**Integration points:**
- Add `CountryFlag string` to: `IXDetail`, `FacilityDetail`, `CampusDetail`, `OrgDetail`, `NetworkFacRow`, `IXFacilityRow`, `OrgFacilityRow`, `CampusFacilityRow`, `CompareFacility`, `SearchResult`
- Populate in `detail.go` query functions alongside existing `Country` field mapping
- Populate in `search.go` when building `SearchResult` subtitle
- Populate in `compare.go` when building facility comparison data

### Pattern 2: Dense Sortable Tables (sortable.js + Tailwind)

**What:** Replace the current vertical card/list layouts (`divide-y` stacked divs) with proper `<table>` elements using `<thead>`/`<tbody>`, styled with Tailwind for density, and made sortable via the `sortable` library.

**When:** All child-entity lists in detail pages (participants, facilities, networks, contacts, etc.), search results, and comparison tables.

**Library choice:** `sortable` by tofsjonas (https://github.com/tofsjonas/sortable)
- 899 bytes gzipped -- smaller than a single SVG icon
- Zero dependencies, vanilla JS
- Works via `class="sortable"` on `<table>` -- no initialization code needed
- Includes CSS for sort indicators (arrows on `<th>`)
- MutationObserver in `sortable.auto.min.js` (1.36K gzipped) auto-initializes tables added to the DOM after page load -- this is critical for htmx fragment loading

**Confidence:** HIGH -- The library is well-maintained, tiny, and its MutationObserver feature specifically addresses the htmx lazy-load concern.

**Why not server-side sort via htmx:** The child entity lists (IX participants, facility networks, etc.) are fully loaded into the DOM when the collapsible section opens. Data volumes are small (largest IX has ~2,000 participants). Client-side sort is instant, avoids a server round-trip, and keeps the current lazy-load-once pattern intact. Server-side sort would require either: (a) refetching the fragment with sort params on every column click (latency, complexity), or (b) maintaining sort state server-side (session state on a stateless edge app). Neither makes sense for datasets that fit in the DOM.

**Why not htmx hx-trigger on th click:** This would work but adds latency per sort action, network requests for what should be instant client-side reordering, and complicates the fragment handler with sort parameters. The sortable library is a better fit.

**CDN integration in layout.templ:**

```html
<!-- In <head>, after htmx -->
<link rel="stylesheet" href="https://cdn.jsdelivr.net/gh/tofsjonas/sortable@latest/sortable-base.min.css"/>
<script src="https://cdn.jsdelivr.net/gh/tofsjonas/sortable@latest/dist/sortable.auto.min.js"></script>
```

**Custom Tailwind overrides for sort indicators and dense table styling:**

```css
/* In <style> block of layout.templ */
table.sortable th { cursor: pointer; user-select: none; }
table.sortable th::after { content: ""; display: inline-block; width: 0; height: 0; margin-left: 6px; vertical-align: middle; }
table.sortable th[aria-sort="ascending"]::after { border-left: 4px solid transparent; border-right: 4px solid transparent; border-bottom: 4px solid currentColor; }
table.sortable th[aria-sort="descending"]::after { border-left: 4px solid transparent; border-right: 4px solid transparent; border-top: 4px solid currentColor; }
```

**Numeric sort via data-sort attribute:**

The sortable library uses `data-sort` attributes for custom sort values. This is essential for speed columns (display "10G" but sort by 10000) and for consistent country flag sorting (sort by country code, not emoji):

```html
<td data-sort="10000">
  <span class="text-blue-400 font-mono">10G</span>
</td>
```

**htmx compatibility:** The `sortable.auto.min.js` variant uses MutationObserver to detect new `<table class="sortable">` elements added to the DOM. When htmx swaps in a fragment containing a sortable table (e.g., opening a collapsible section), the library automatically attaches sort handlers. No custom `htmx:afterSwap` event listener needed. Verified via library documentation.

### Pattern 3: Leaflet Map with Facility Pins

**What:** Interactive OpenStreetMap-based map showing facility locations as clickable pins with clustering and popups linking to detail pages. Displayed on facility detail pages (single pin), IX detail pages (multiple facility pins), network detail pages (all facility locations), and comparison pages (shared/unique facilities on map).

**When:** Only on pages where geographic data (lat/lng) is available. Facilities are the primary geo-located entities. IXes get their locations from their associated facilities. Networks get locations from their facility presences.

**Leaflet CDN (in layout.templ `<head>`):**

```html
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/leaflet@1.9.4/dist/leaflet.css"/>
<script src="https://cdn.jsdelivr.net/npm/leaflet@1.9.4/dist/leaflet.js"></script>
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/leaflet.markercluster@1.5.3/dist/MarkerCluster.css"/>
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/leaflet.markercluster@1.5.3/dist/MarkerCluster.Default.css"/>
<script src="https://cdn.jsdelivr.net/npm/leaflet.markercluster@1.5.3/dist/leaflet.markercluster.js"></script>
```

**Total CDN payload:** Leaflet JS ~42KB gzipped, CSS ~4KB. MarkerCluster JS ~10KB gzipped, CSS ~1KB. Combined ~57KB -- comparable to htmx (15KB) + Tailwind CDN (~300KB) already loaded.

**Confidence:** HIGH for Leaflet 1.9.4 (stable, widely deployed). MEDIUM for MarkerCluster 1.5.3 (last release 2022, but stable and compatible with Leaflet 1.9.x). Leaflet 2.0 is alpha, do not use.

**Data flow -- Go to Leaflet:**

The map data is serialized as GeoJSON in a `<script type="application/json">` block rendered by templ. This avoids inline JS in templ (which requires `script` blocks), avoids a separate AJAX fetch (the data is already in the handler), and is SSR-friendly.

```go
// MapPoint represents a single geo-located entity for the map.
type MapPoint struct {
    Lat     float64 `json:"lat"`
    Lng     float64 `json:"lng"`
    Name    string  `json:"name"`
    URL     string  `json:"url"`
    PopupHTML string `json:"popup"` // pre-rendered HTML for popup content
}
```

Templ renders:

```html
<div id="map" class="h-80 rounded-lg border border-neutral-700 overflow-hidden"></div>
<script type="application/json" id="map-data">
  [{"lat":51.5074,"lng":-0.1278,"name":"Equinix LD8","url":"/ui/fac/123","popup":"..."}]
</script>
```

Client-side initialization (inline `<script>` at bottom of map-containing templates):

```javascript
(function() {
    var dataEl = document.getElementById('map-data');
    if (!dataEl) return;
    var points = JSON.parse(dataEl.textContent);
    if (points.length === 0) return;

    var isDark = document.documentElement.classList.contains('dark');
    var tileURL = isDark
        ? 'https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png'
        : 'https://{s}.basemaps.cartocdn.com/light_all/{z}/{x}/{y}{r}.png';

    var map = L.map('map');
    L.tileLayer(tileURL, {
        attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> &copy; <a href="https://carto.com/attributions">CARTO</a>',
        maxZoom: 19
    }).addTo(map);

    var markers = L.markerClusterGroup();
    points.forEach(function(p) {
        var marker = L.marker([p.lat, p.lng]);
        if (p.popup) marker.bindPopup(p.popup);
        markers.addLayer(marker);
    });
    map.addLayer(markers);
    map.fitBounds(markers.getBounds().pad(0.1));
})();
```

**Tile provider choice -- CARTO (CartoDB) basemaps:**

| Provider | Dark Mode | Free Tier | API Key | Attribution |
|----------|-----------|-----------|---------|-------------|
| CARTO (CartoDB) | `dark_all` / `light_all` native variants | Unlimited for non-commercial, reasonable use | No API key needed | Required: OSM + CARTO |
| OpenStreetMap tile.openstreetmap.org | No dark mode | Fair use only, strict policy | None | Required: OSM |
| Stadia Maps Alidade Smooth Dark | Purpose-built dark tiles | 2,500 credits/month (~25K tiles) | Required (free signup) | Required |
| CSS filter invert | Any light tiles inverted | N/A | N/A | Labels become inverted and unreadable |

**Recommendation: CARTO basemaps.** Native dark/light variants avoid the CSS invert hack (which makes labels unreadable). No API key or signup required. No usage limits for reasonable traffic. Both `dark_all` and `light_all` are available, enabling dark mode switching that matches the app's existing dark mode toggle.

**Confidence:** HIGH -- CARTO basemaps are widely used, free, and require no registration. The `{s}.basemaps.cartocdn.com` URLs have been stable for years.

**Dark mode tile switching:**

The map checks `document.documentElement.classList.contains('dark')` at initialization time. This aligns with the existing class-based dark mode system (see `layout.templ` dark mode toggle). A full theme switch while a map is visible would require reinitializing the tile layer, but this is an edge case -- the dark mode toggle does not currently trigger page reloads or htmx swaps. If needed later, a `MutationObserver` on the `<html>` class list could swap tile layers. Defer this to a follow-up.

**Map conditional rendering:**

The map container and script block should only render when there are geo-points. In templ:

```
if len(data.MapPoints) > 0 {
    @MapSection(data.MapPoints)
}
```

This keeps pages without geographic data (e.g., carriers, networks without facility presences) clean.

### Pattern 4: Dense Table Layout (Replacing Stacked Cards)

**What:** Convert the current `divide-y` stacked div layouts to proper `<table>` elements with compact row heights, monospace data columns, and multi-column density.

**Current layout (IX Participants):**
```
[Name + ASN badge + RS badge]
  Speed: 10G
  IPv4: 192.0.2.1
  IPv6: 2001:db8::1
--- divider ---
[Next participant...]
```

**New layout (IX Participants table):**
```
| Flag | Network         | ASN      | Speed | IPv4       | IPv6            | RS |
|------|-----------------|----------|-------|------------|-----------------|----|
| US   | Cloudflare      | AS13335  | 100G  | 192.0.2.1  | 2001:db8::1     | RS |
| US   | Google          | AS15169  | 100G  | 192.0.2.2  | 2001:db8::2     |    |
```

**Density gains:** The current layout uses ~4-5 lines per participant. The table layout uses 1 line. For an IX with 200 participants, this reduces from ~1000 lines of vertical scroll to ~200 lines.

**Template structure:**

```html
<table class="sortable w-full text-sm">
  <thead>
    <tr class="text-left text-neutral-500 border-b border-neutral-700">
      <th class="px-2 py-1.5"></th>           <!-- flag, no-sort -->
      <th class="px-2 py-1.5">Network</th>
      <th class="px-2 py-1.5 font-mono">ASN</th>
      <th class="px-2 py-1.5">Speed</th>
      <th class="px-2 py-1.5 font-mono">IPv4</th>
      <th class="px-2 py-1.5 font-mono">IPv6</th>
      <th class="px-2 py-1.5 no-sort">RS</th>
    </tr>
  </thead>
  <tbody>
    <!-- rows rendered by templ range -->
  </tbody>
</table>
```

**Responsive behavior:** On narrow screens (mobile), hide lower-priority columns via Tailwind responsive classes: `class="hidden md:table-cell"` on IPv6, RS columns. Keep flag, name, ASN, speed always visible. This is simpler than the current approach which shows everything stacked.

### Pattern 5: Geo Data Enrichment from ent

**What:** The `facility` entity has `Latitude *float64` and `Longitude *float64` fields in ent schema. These are currently not surfaced in the web UI at all. IXes and networks do NOT have lat/lng -- they derive location from their facility associations.

**Facility detail page:** Single map pin from the facility's own lat/lng. Straightforward.

**IX detail page:** Multiple pins from IX facilities (already eager-loaded in `queryIX`). Need to join through to the actual `Facility` entity to get lat/lng, since `IxFacility` (the junction entity) does not carry lat/lng -- only `city` and `country`.

**Implementation for IX geo data:**

```go
// In queryIX, after eager-loading IX facilities, also fetch actual Facility entities for lat/lng.
facIDs := make([]int, 0, len(ixFacItems))
for _, ixf := range ixFacItems {
    if ixf.FacID != nil {
        facIDs = append(facIDs, *ixf.FacID)
    }
}
if len(facIDs) > 0 {
    facs, err := h.client.Facility.Query().
        Where(facility.IDIn(facIDs...)).
        All(ctx)
    // Build MapPoints from facs with non-nil Latitude/Longitude
}
```

**Network detail page:** Same pattern -- fetch Facility entities from NetworkFacility junction rows to get lat/lng.

**Comparison page:** Merge facility locations from both networks, coloring shared vs unique pins differently (green for shared, grey for unique-to-one-network).

**Data availability concern:** Not all PeeringDB facilities have lat/lng populated. The map should gracefully handle empty coordinates by simply not placing a marker for those facilities. The map section should not render at all if zero facilities have valid coordinates.

**Confidence:** HIGH -- lat/lng fields exist in ent schema and are synced from PeeringDB. The join pattern through junction entities to get facility coordinates is standard ent querying.

## Anti-Patterns to Avoid

### Anti-Pattern 1: Inline JavaScript in templ `script` Blocks for Map Init

**What:** Using templ's `script` directive to define map initialization functions.

**Why bad:** templ `script` blocks generate named JavaScript functions that get called via `onclick` or similar attributes. They work well for small actions (like `copyToClipboard`) but are awkward for initialization code that should run once when the element appears. They also make it harder to access DOM elements by ID since the function runs in a different scope.

**Instead:** Use a raw `<script>` tag at the end of the map-containing template. The script reads data from a `<script type="application/json">` element, keeping the data flow explicit and the initialization self-contained.

### Anti-Pattern 2: Fetching Map Data via Separate AJAX Endpoint

**What:** Creating a `/ui/fragment/ix/{id}/mapdata` endpoint that returns JSON, then fetching it client-side to populate the map.

**Why bad:** The geographic data is already available when the page loads -- it comes from the same ent queries that populate the detail page. Adding a separate endpoint means: an extra HTTP round-trip, a flash of empty map while loading, a new handler to maintain, and duplicated query logic.

**Instead:** Embed the GeoJSON data inline in the HTML response. The data is small (a few hundred bytes for typical facility counts) and is always needed when the map renders.

### Anti-Pattern 3: Server-Side Table Sorting via htmx for Small Datasets

**What:** Using `hx-get` with sort parameters on `<th>` clicks to reload the table from the server with different ordering.

**Why bad for this project:** The collapsible sections use `hx-trigger="toggle once"` -- they load data once and keep it in the DOM. Server-side sorting would either break this "load once" pattern (requiring re-fetch on every sort) or need complex client-side state management. The datasets are small enough (largest: ~2K IX participants) that client-side sorting is instant.

**When server-side sorting IS appropriate:** If a future feature adds paginated tables (e.g., browsing all 80K+ networks), server-side sorting with htmx would be the right approach. The current architecture is per-entity detail pages where child counts are in the hundreds, not tens of thousands.

### Anti-Pattern 4: CSS Filter Invert for Dark Mode Maps

**What:** Applying `filter: invert(1) hue-rotate(180deg)` to map tiles for dark mode.

**Why bad:** Inverts all tile colors including text labels, making them unreadable. Also inverts marker icons and popup content. Requires exception rules for every interactive element.

**Instead:** Use a tile provider with native dark/light variants (CARTO basemaps). Switch the tile URL based on the app's dark mode state.

### Anti-Pattern 5: Computing Emoji Flags in JavaScript

**What:** Sending country codes to the browser and converting to emoji flags client-side.

**Why bad:** Adds unnecessary JS, creates a flash of "US" text before the flag renders, doesn't work in terminal rendering mode, and the computation is trivial to do server-side. Also, some edge cases (invalid codes, missing data) are better handled with Go string validation.

**Instead:** Compute in Go when populating template data. The flag appears in the initial HTML render.

## New Components vs Modified Components

### New Files

| File | Purpose |
|------|---------|
| `internal/web/countryflags.go` | `CountryFlag(code string) string` function + `MapPoint` type |
| `internal/web/countryflags_test.go` | Table-driven tests for flag conversion, edge cases |

### Modified Files (Summary)

| File | Changes |
|------|---------|
| `internal/web/templates/layout.templ` | Add Leaflet CSS/JS CDN links, MarkerCluster CSS/JS, sortable CSS/JS, custom sort indicator CSS |
| `internal/web/templates/detailtypes.go` | Add `CountryFlag string`, `Latitude *float64`, `Longitude *float64`, `MapPoints []MapPoint` to relevant structs |
| `internal/web/templates/searchtypes.go` | Add `CountryFlag string` to `SearchResult` |
| `internal/web/templates/comparetypes.go` | Add `CountryFlag string` to `CompareFacility`, add `MapPoints []MapPoint` to `CompareData` |
| `internal/web/templates/detail_ix.templ` | Convert `IXParticipantsList`, `IXFacilitiesList` from div-based to table-based. Add map section. Add flag column. |
| `internal/web/templates/detail_net.templ` | Convert `NetworkIXLansList`, `NetworkFacilitiesList` from div-based to table-based. Add map section. Add flag column. |
| `internal/web/templates/detail_fac.templ` | Convert `FacNetworksList`, `FacIXPsList` from div-based to table-based. Add single-pin map. Add flag to header. |
| `internal/web/templates/detail_org.templ` | Convert child lists to table-based. Add flag columns where applicable. |
| `internal/web/templates/detail_campus.templ` | Convert facility list to table-based. Add flag column. |
| `internal/web/templates/detail_carrier.templ` | Convert facility list to table-based. |
| `internal/web/templates/detail_shared.templ` | Add `MapSection` templ component. Modify `CopyableIP` to work in table cells. |
| `internal/web/templates/compare.templ` | Convert comparison sections to table-based. Add flag columns. Add map with colored pins. |
| `internal/web/templates/search_results.templ` | Add flag to subtitle area in search results. |
| `internal/web/detail.go` | Populate CountryFlag, Latitude, Longitude, MapPoints in all query functions. Add facility geo lookup for IX/network. |
| `internal/web/search.go` | Populate CountryFlag in search result building. |
| `internal/web/compare.go` | Populate CountryFlag in comparison building. Add MapPoints for comparison map. |
| `internal/web/handler_test.go` | Verify table structure in rendered HTML, flag presence, map container presence. |
| `internal/web/detail_test.go` | Test geo data population, flag conversion, map point generation. |

## Build Order (Dependency-Aware)

The features have clear dependencies. Build in this order:

### Phase 1: Country Flag Utility

No other features depend on external changes. Self-contained.
- Create `countryflags.go` with `CountryFlag()` function
- Create `countryflags_test.go`
- Zero impact on existing templates

### Phase 2: Type Additions (Data Layer)

All template changes depend on having the right fields.
- Add `CountryFlag`, `Latitude`, `Longitude`, `MapPoints` to type definitions
- No template changes yet -- just data plumbing

### Phase 3: Dense Table Layouts

This is the largest change by line count. Independent of Leaflet/map work.
- Add sortable.js CDN and CSS to `layout.templ`
- Convert all child-entity list templates from `divide-y` divs to `<table class="sortable">`
- Add `data-sort` attributes for numeric columns
- Insert flag columns
- Update `detail.go` to populate new fields (CountryFlag, lat/lng)
- Update handlers to call `CountryFlag()` during data population
- Test that sortable works with htmx-loaded fragments

### Phase 4: Leaflet Map Integration

Depends on lat/lng being populated (Phase 2-3 data plumbing).
- Add Leaflet + MarkerCluster CDN to `layout.templ`
- Create `MapSection` shared templ component
- Add map to facility detail page (simplest: single pin, own lat/lng)
- Add map to IX detail page (multiple pins from facility associations)
- Add map to network detail page (multiple pins from facility presences)
- Add map to comparison page (colored pins for shared/unique)
- Handle dark mode tile switching

### Phase 5: Search Results Density

Depends on flag utility (Phase 1) but independent of tables/maps.
- Add flag display to search result rows
- Optionally convert search results to denser layout

## Scalability Considerations

| Concern | Current Scale | At v1.11 | Notes |
|---------|--------------|----------|-------|
| JS payload size | htmx (15KB) + Tailwind CDN (~300KB) | +sortable (1.4KB) +Leaflet (42KB) +MarkerCluster (10KB) = ~368KB total | Acceptable for non-mobile-first app. All CDN-cached. |
| Map markers per page | N/A | Largest IX has ~2,000 participants across ~50 facilities. Map shows facilities (50 pins), not participants (2,000). | MarkerCluster handles hundreds of pins efficiently. |
| Table rows in DOM | Current: ~2,000 max (large IX participants list) | Same data, different layout. No change in DOM node count. | sortable.js handles 10K+ rows. |
| Geo data queries | Not queried | 1 additional query per IX/network page to fetch facility lat/lng | Batch query via `IDIn()`, not N+1. Sub-millisecond on SQLite. |
| CDN availability | Tailwind CDN, self-hosted htmx | CARTO tiles, jsDelivr for libraries | CARTO has been stable for years. jsDelivr is the recommended Leaflet CDN. Self-hosting Leaflet/sortable in `/static/` is trivial if CDN reliability is a concern (same pattern as htmx). |

## Sources

- [Leaflet.js download](https://leafletjs.com/download.html) -- v1.9.4 stable, v2.0.0-alpha.1 (do not use alpha)
- [Leaflet GeoJSON tutorial](https://leafletjs.com/examples/geojson/) -- inline GeoJSON data loading
- [Leaflet MarkerCluster](https://github.com/Leaflet/Leaflet.markercluster) -- v1.5.3 clustering plugin
- [CARTO Basemaps](https://carto.com/basemaps) -- free dark_all/light_all tile variants, no API key
- [sortable by tofsjonas](https://github.com/tofsjonas/sortable) -- 899B gzipped table sort with MutationObserver for dynamic content
- [Unicode Regional Indicator Symbols](https://en.wikipedia.org/wiki/Regional_indicator_symbol) -- U+1F1E6 to U+1F1FF for country flags
- [htmx table sorting example](https://htmx.org/examples/sortable/) -- server-side approach (not recommended for our use case)
- [OpenStreetMap tile usage policy](https://operations.osmfoundation.org/policies/tiles/) -- restrictions on tile.openstreetmap.org usage
- [Stadia Maps pricing](https://stadiamaps.com/pricing) -- free tier requires API key, limited credits
- [Leaflet Provider Demo](https://leaflet-extras.github.io/leaflet-providers/preview/) -- tile provider comparison
