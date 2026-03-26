# Technology Stack

**Project:** PeeringDB Plus -- v1.11 Web UI Density & Interactivity
**Researched:** 2026-03-26
**Confidence:** HIGH

## Recommended Stack

This document covers ONLY the new additions needed for v1.11. The existing stack (templ v0.3.x, htmx 2.0.8, Tailwind CSS v4 via CDN, Go 1.26, net/http, entgo) is validated and unchanged.

### Client-Side Libraries (CDN)

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| Leaflet.js | 1.9.4 | Interactive map with clickable pins, popups | The standard open-source mapping library. 42K GitHub stars. Stable since Sep 2022 (1.9.x line). v2.0.0 is alpha-only -- not production ready. Lightweight (~42KB gzipped). No build toolchain needed: one CSS + one JS via CDN. | HIGH |
| Leaflet.markercluster | 1.5.3 | Marker clustering for map with many facilities | Official Leaflet plugin. Clusters overlapping markers at low zoom levels, spiderifies on click. Required when displaying hundreds of facilities on a single map. Available via unpkg CDN. Compatible with Leaflet 1.x. | HIGH |
| sortable-tablesort | 4.1.7 | Client-side column sorting for dense tables | 899 bytes gzipped (lightweight flavor). Vanilla JS, zero dependencies. CSS class-based activation (`class="sortable"` on `<table>`). Auto version includes MutationObserver for htmx-loaded content. `data-sort` attribute allows custom sort values (useful for speed columns: display "10G" but sort by 10000). Unlicense. | HIGH |

### Tile Provider

| Provider | URL Pattern | Purpose | Why | Confidence |
|----------|-------------|---------|-----|------------|
| CARTO Dark Matter | `https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png` | Dark-themed map tiles matching app's dark UI | No API key required. No registration. Free with attribution (CC-BY 4.0). Dark background matches the dark-mode-first design. Subdomains a/b/c/d for parallel loading. Attribution: "CARTO, OpenStreetMap contributors". | HIGH |
| CARTO Positron | `https://{s}.basemaps.cartocdn.com/light_all/{z}/{x}/{y}{r}.png` | Light-themed map tiles for light mode | Same terms as Dark Matter. Switch tile URL based on dark/light mode toggle. | MEDIUM |

### Go Server-Side Additions

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| No new Go dependencies | -- | Country code to emoji flag conversion | Pure Go function: 6 lines. ISO 3166-1 alpha-2 country codes map to Unicode Regional Indicator Symbols (U+1F1E6 to U+1F1FF). Formula: `rune(char) - 'A' + 0x1F1E6` for each letter. No library needed. PeeringDB stores 2-letter country codes on facilities, IXPs, orgs, and campuses. | HIGH |
| No new Go dependencies | -- | Latitude/longitude extraction for map | Facility and Organization ent schemas already have `latitude` and `longitude` fields (Optional, Nillable, Float). These are already synced from PeeringDB. No additional data fetching needed -- just include lat/lng in the view model structs. | HIGH |

## Integration Architecture

### Sortable Tables + htmx

Two sorting modes needed, driven by data volume:

**Client-side sorting** (sortable-tablesort): For htmx fragment data already loaded in the DOM. Applies to collapsible sections (IX participants, facility networks, etc.) where all rows are returned in a single fragment response. The `sortable.auto.min.js` flavor uses MutationObserver to automatically initialize sorting on tables added by htmx swaps -- no `htmx.onLoad()` glue code needed.

**Server-side sorting** (htmx hx-get with query params): For future paginated views where not all data is in the DOM. Use `hx-get="/ui/fragment/ix/{id}/participants?sort=speed&dir=desc"` with `hx-target` replacing the table body. The server reads sort/dir params and applies ent `.Order()`. Not needed in v1.11 -- all collapsible sections load full datasets.

**Recommendation for v1.11:** Client-side only via sortable-tablesort. All data sections load complete datasets into the DOM (IX participants lists are ~500 rows max for the largest exchanges). Server-side sorting is premature optimization for this data volume.

### Table Structure

Current templates use `<div>` with flexbox/grid for lists. Tables need conversion to semantic `<table>` elements for sortable-tablesort to work. This is the correct direction anyway -- the data IS tabular.

```html
<!-- Current pattern (div-based cards) -->
<div class="divide-y divide-neutral-700/50">
  <div class="px-4 py-3">...</div>
</div>

<!-- New pattern (dense sortable table) -->
<table class="sortable w-full text-sm">
  <thead>
    <tr><th>Name</th><th>Country</th><th>Speed</th></tr>
  </thead>
  <tbody>
    <tr><td>...</td><td>...</td><td data-sort="10000">10G</td></tr>
  </tbody>
</table>
```

### Leaflet Map Integration

Maps appear in two contexts:

1. **Facility detail page**: Single marker showing facility location. Simple, no clustering.
2. **IX detail page / Org detail page / Network detail page**: Multiple markers showing all facilities. Needs MarkerCluster for IXs with many facilities in close proximity (e.g., Amsterdam, Frankfurt metro areas).
3. **Search results** (deferred): Optional map view of search hits -- can be added later.

Map loads conditionally -- only when lat/lng data exists. Templ renders a `<div id="map">` with `data-lat` and `data-lng` attributes (or a JSON array for multi-marker). A small inline `<script>` block initializes Leaflet after the div renders. No htmx interaction needed for the map itself.

**Dark/light mode tile switching:** Read `document.documentElement.classList.contains('dark')` on map init. Listen for the dark mode toggle event to swap tile layers.

### Country Flag Emoji

Conversion happens server-side in Go, injected into templ templates as a string. Implementation:

```go
// CountryFlag converts an ISO 3166-1 alpha-2 country code to its flag emoji.
// Returns empty string for invalid or empty codes.
func CountryFlag(code string) string {
    if len(code) != 2 {
        return ""
    }
    code = strings.ToUpper(code)
    if code[0] < 'A' || code[0] > 'Z' || code[1] < 'A' || code[1] > 'Z' {
        return ""
    }
    return string(rune(code[0])-'A'+0x1F1E6) + string(rune(code[1])-'A'+0x1F1E6)
}
```

Place in `internal/web/templates/` as a template helper function (same package as `formatSpeed`, `speedColorClass`). Use in templates as `{ CountryFlag(row.Country) }` alongside the country code text.

**Platform note:** Emoji flags render correctly on macOS, Linux (with Noto Color Emoji), iOS, and Android. Windows 10/11 shows two-letter codes instead of flags (Microsoft policy decision). This is acceptable -- the two-letter country code is always displayed alongside the flag as a text column, so Windows users still see the country.

### Tailwind Dense Table Styling

No Tailwind plugins needed. Standard utility classes achieve dense tables:

| Class Pattern | Purpose |
|---------------|---------|
| `text-xs` / `text-sm` | Smaller text for data density |
| `px-2 py-1` | Tight cell padding (vs current `px-4 py-3`) |
| `font-mono` | Monospace for numeric/technical columns (ASN, speed, IP) |
| `tabular-nums` | Fixed-width numerals for numeric column alignment |
| `whitespace-nowrap` | Prevent wrapping in narrow columns (country, speed) |
| `truncate` | Ellipsis overflow for long names in constrained widths |
| `sticky top-0` | Sticky table headers for scrollable sections |

Dark mode table styling via existing `@custom-variant dark` already in layout.templ. Table rows: `hover:bg-neutral-800/50 dark:hover:bg-neutral-800/50` (already established pattern).

### CDN Loading Strategy

Add to `layout.templ` `<head>` section, loaded on every page (they're small):

```html
<!-- Leaflet (loaded on all pages for consistency, 42KB gzipped) -->
<link rel="stylesheet" href="https://unpkg.com/leaflet@1.9.4/dist/leaflet.css"
  integrity="sha256-p4NxAoJBhIIN+hmNHrzRCf9tD/miZyoHS5obTRR9BMY=" crossorigin=""/>
<script src="https://unpkg.com/leaflet@1.9.4/dist/leaflet.js"
  integrity="sha256-20nQCchB9co0qIjJZRGuk2/Z9VM+kNiyxNV1lvTlZBo=" crossorigin=""></script>

<!-- MarkerCluster (loaded on all pages, 8KB gzipped) -->
<link rel="stylesheet" href="https://unpkg.com/leaflet.markercluster@1.5.3/dist/MarkerCluster.css"/>
<link rel="stylesheet" href="https://unpkg.com/leaflet.markercluster@1.5.3/dist/MarkerCluster.Default.css"/>
<script src="https://unpkg.com/leaflet.markercluster@1.5.3/dist/leaflet.markercluster.js"></script>

<!-- Sortable tables (loaded on all pages, <1KB gzipped) -->
<script src="https://unpkg.com/sortable-tablesort@4.1.7/dist/sortable.auto.min.js"></script>
```

**Alternative approach (deferred loading):** If total CDN payload is a concern, conditionally include Leaflet resources only on pages with maps by using templ component composition -- have a `MapHead()` component that detail pages opt into. The current layout already loads Tailwind Browser (300KB) and htmx, so 50KB more for Leaflet is marginal.

**Self-hosting consideration:** Currently htmx.min.js is embedded in `internal/web/static/` via `embed.FS`. The same pattern COULD be used for Leaflet and MarkerCluster to avoid CDN dependency. However, Leaflet is significantly larger and CDN caching (unpkg is backed by Cloudflare) means most users already have it cached from other sites. Recommendation: **keep CDN** for Leaflet, consistent with Tailwind CDN approach. Self-host only sortable-tablesort (trivially small).

## What NOT to Add

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| AG Grid / DataTables.js | Massive (~100KB+) for sorting that's achievable in <1KB. Brings jQuery or framework dependency. | sortable-tablesort (899 bytes gzipped) |
| MapLibre GL JS / Mapbox GL JS | Vector tile rendering -- overkill for pin maps. 200KB+. Requires WebGL. Complex dark mode. | Leaflet 1.9.4 with raster tiles |
| OpenLayers | Enterprise mapping library. 200KB+. Steep learning curve. | Leaflet 1.9.4 |
| Leaflet 2.0.0-alpha | Alpha since Aug 2025. ESM-only breaking change. Not stable. | Leaflet 1.9.4 (stable since May 2023) |
| SortableJS (Shopify/Sortable) | Drag-and-drop reordering library, NOT table sorting. Common name confusion. | sortable-tablesort |
| Stadia Maps tiles | Requires API key registration. Free tier is non-commercial only. | CARTO basemaps (no auth, CC-BY 4.0) |
| OpenStreetMap tiles directly | Usage policy prohibits heavy/commercial use. Requires custom User-Agent. Rate limits. | CARTO basemaps (designed for third-party use) |
| Go emoji/flag libraries | No Go library exists for this trivially -- it's 6 lines of code with zero edge cases for ISO 3166-1 alpha-2 codes. | Inline `CountryFlag()` function |
| country-flag-icons (SVG) | SVG flag images are heavier than emoji, require bundling, and don't work in terminal/JSON output modes. | Unicode Regional Indicator emoji |
| Tailwind plugins for tables | `@tailwindcss/typography` and table plugins add weight for styling achievable with base utilities. | Standard `text-sm`, `px-2 py-1` utilities |
| React/Vue/Svelte | This project deliberately avoids SPA frameworks and JS build toolchains. templ + htmx is the validated pattern. | templ server-rendered HTML + htmx fragments |

## Version Pinning Strategy

Pin all CDN URLs to exact versions with SRI hashes where available:

| Library | Pin | SRI Available |
|---------|-----|---------------|
| Leaflet JS | `@1.9.4` | Yes (sha256) |
| Leaflet CSS | `@1.9.4` | Yes (sha256) |
| MarkerCluster JS | `@1.5.3` | No (verify hash from unpkg) |
| MarkerCluster CSS | `@1.5.3` | No |
| sortable-tablesort | `@4.1.7` | No |

Generate SRI hashes for MarkerCluster and sortable-tablesort during implementation: `shasum -b -a 256 <file> | xxd -r -p | base64`.

## Alternatives Considered

| Category | Recommended | Alternative | Why Not |
|----------|-------------|-------------|---------|
| Table sorting | sortable-tablesort (client-side) | htmx server-side sort (hx-get with sort params) | All data fits in DOM. Client-side is faster (no round-trip). Server-side sort useful later for paginated views. |
| Table sorting | sortable-tablesort | Vanilla JS sort (~30 lines) | sortable-tablesort handles edge cases (numeric vs alpha detection, sort indicators, accessibility), is battle-tested, and is <1KB. Not worth hand-rolling. |
| Map library | Leaflet 1.9.4 | Leaflet 2.0.0-alpha.1 | Alpha. ESM-only (breaks script tag loading pattern). Unnecessary modernization for pin maps. |
| Map library | Leaflet 1.9.4 | MapLibre GL JS | Vector tiles need styling, 200KB+, WebGL required. Raster tiles with pins is sufficient. |
| Map tiles (dark) | CARTO Dark Matter | Stadia Alidade Smooth Dark | Stadia requires API key, free tier non-commercial only. CARTO is free, no auth. |
| Map tiles (dark) | CARTO Dark Matter | OSM tiles + CSS filter invert | CSS invert looks bad (inverts label colors). CARTO provides a properly designed dark map. |
| Clustering | MarkerCluster | Supercluster | Supercluster is for GeoJSON with MapLibre. MarkerCluster is the standard Leaflet solution. |
| Country flags | Unicode emoji (server-side Go) | SVG flag images | Emoji renders natively everywhere (except Windows flag limitation). No asset pipeline. Works in terminal output mode too (already using emoji in templ). |
| Country flags | Unicode emoji (server-side Go) | Client-side JS conversion | Server-side avoids FOUC (flag appears with initial HTML). Consistent with templ rendering model. |

## Data Requirements

Fields needed from ent schemas that are NOT yet in view model structs:

| View Model | Missing Field | Source | Purpose |
|------------|---------------|--------|---------|
| `FacilityDetail` | Latitude, Longitude | `ent.Facility` (already in schema) | Single-marker map on facility detail page |
| `IXFacilityRow` | Latitude, Longitude | `ent.Facility` via edge | Multi-marker map on IX detail page |
| `OrgFacilityRow` | Latitude, Longitude | `ent.Facility` via edge | Multi-marker map on org detail page |
| `NetworkFacRow` | Latitude, Longitude | `ent.Facility` via netfac edge | Multi-marker map on network detail page |
| `CampusFacilityRow` | Latitude, Longitude | `ent.Facility` via edge | Multi-marker map on campus detail page |
| `NetworkIXLanRow` | Country (optional) | IX edge -> IXLan -> IX | Country flag in IX presence table |
| `IXParticipantRow` | -- | Already has what's needed | Just needs table conversion |
| `CompareFacility` | Latitude, Longitude | Already has city/country | Map of shared/all facilities |

**Key observation:** Lat/lng is on `Facility` entities. For IX detail pages, IX facilities already have a `FacID` link. The handler queries need to eager-load facility lat/lng alongside existing data. This is a query change, not a schema change.

## Installation

No `go get` commands needed. All additions are client-side CDN or pure Go code.

```html
<!-- Add to internal/web/templates/layout.templ <head> -->
<link rel="stylesheet" href="https://unpkg.com/leaflet@1.9.4/dist/leaflet.css"
  integrity="sha256-p4NxAoJBhIIN+hmNHrzRCf9tD/miZyoHS5obTRR9BMY=" crossorigin=""/>
<script src="https://unpkg.com/leaflet@1.9.4/dist/leaflet.js"
  integrity="sha256-20nQCchB9co0qIjJZRGuk2/Z9VM+kNiyxNV1lvTlZBo=" crossorigin=""></script>
<link rel="stylesheet" href="https://unpkg.com/leaflet.markercluster@1.5.3/dist/MarkerCluster.css"/>
<link rel="stylesheet" href="https://unpkg.com/leaflet.markercluster@1.5.3/dist/MarkerCluster.Default.css"/>
<script src="https://unpkg.com/leaflet.markercluster@1.5.3/dist/leaflet.markercluster.js"></script>
<script src="https://unpkg.com/sortable-tablesort@4.1.7/dist/sortable.auto.min.js"></script>
```

```go
// Add to internal/web/templates/ (e.g., helpers.go or detail_shared.go)
func CountryFlag(code string) string {
    if len(code) != 2 {
        return ""
    }
    code = strings.ToUpper(code)
    if code[0] < 'A' || code[0] > 'Z' || code[1] < 'A' || code[1] > 'Z' {
        return ""
    }
    return string(rune(code[0])-'A'+0x1F1E6) + string(rune(code[1])-'A'+0x1F1E6)
}
```

## Risk Register

| Risk | Severity | Likelihood | Mitigation |
|------|----------|------------|------------|
| unpkg CDN outage | MEDIUM | LOW | SRI hashes ensure integrity. Fallback: self-host copies in `internal/web/static/`. unpkg is backed by Cloudflare CDN with global PoPs. |
| CARTO tile service discontinuation | MEDIUM | LOW | CARTO basemaps have been free since 2015. If discontinued, swap tile URL to OSM (for light) or Stadia (with registration). Tile URL is a single line change. |
| sortable-tablesort MutationObserver conflicts with htmx | LOW | LOW | The `.auto` version uses MutationObserver which fires on htmx DOM swaps. If issues arise, switch to lightweight version + `htmx.onLoad()` hook. |
| MarkerCluster CSS conflicts with Tailwind | LOW | MEDIUM | MarkerCluster.Default.css uses opinionated cluster styling. May need custom CSS to match dark theme. Override `.marker-cluster-small/medium/large` background colors. |
| Windows not rendering flag emoji | LOW | HIGH (by design) | Windows shows two-letter codes instead of flags. Acceptable because country code text column is always displayed alongside. Document this known behavior. |
| Leaflet 1.9.4 end-of-life before 2.0 stable | LOW | LOW | 1.9.4 is the current stable line. Even when 2.0 ships, 1.9.x will continue working -- it's a client-side library with no server dependency. Migration to 2.0 is optional. |

## Sources

- [Leaflet Downloads](https://leafletjs.com/download.html) -- v1.9.4 stable, CDN links with SRI hashes
- [Leaflet GitHub Releases](https://github.com/Leaflet/Leaflet/releases) -- v1.9.4 (May 2025), v2.0.0-alpha.1 (Aug 2025)
- [Leaflet.markercluster GitHub](https://github.com/Leaflet/Leaflet.markercluster) -- v1.5.3, official plugin
- [sortable-tablesort npm](https://www.npmjs.com/package/sortable-tablesort) -- v4.1.7
- [tofsjonas/sortable GitHub](https://github.com/tofsjonas/sortable) -- Features, MutationObserver, CSS options
- [CARTO Basemaps](https://carto.com/basemaps) -- Free tile service, no API key
- [CARTO Attribution](https://carto.com/attribution) -- CC-BY 4.0 licensing terms
- [Regional Indicator Symbols](https://en.wikipedia.org/wiki/Regional_indicator_symbol) -- Unicode flag emoji encoding
- [htmx Server-Side Sorting Pattern](https://dev.to/vladkens/table-sorting-and-pagination-with-htmx-3dh8) -- hx-get with sort params
- [OSM Tile Usage Policy](https://operations.osmfoundation.org/policies/tiles/) -- Why NOT to use OSM tiles directly
- [Stadia Maps Pricing](https://stadiamaps.com/pricing) -- Free tier is non-commercial only

---
*Stack research for: Web UI Density & Interactivity (v1.11)*
*Researched: 2026-03-26*
