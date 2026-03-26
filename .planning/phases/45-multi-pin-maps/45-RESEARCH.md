# Phase 45: Multi-Pin Maps - Research

**Researched:** 2026-03-26
**Domain:** Leaflet multi-marker maps with clustering, data enrichment for facility coordinates
**Confidence:** HIGH

## Summary

Phase 45 adds multi-pin maps to IX detail, network detail, and ASN comparison pages. The existing Phase 44 MapContainer component provides the foundation (Leaflet 1.9.4 CDN, CARTO tiles, dark mode swap, `window.__pdbMaps` array). This phase extends it with marker clustering via Leaflet.markercluster, circleMarker-based pins with category coloring, fitBounds for auto-zoom, and a legend on comparison maps.

The main technical concern is that `L.circleMarker` does not natively support `setOpacity()` which markercluster requires for cluster/uncluster animations. This is a well-documented issue (GitHub #62, #183) with a proven shim: extending `L.CircleMarker.prototype` with a `setOpacity` method. The shim is a few lines of JavaScript and has been the standard workaround since 2012.

Data enrichment is straightforward: both `IxFacility` and `NetworkFacility` ent entities have edges to `Facility` via `fac_id`, and the generated code provides `WithFacility()` eager-loading. The Facility entity has nullable `Latitude *float64` and `Longitude *float64` fields. The compare handler already computes facility unions -- it needs `WithFacility()` added to the NetworkFacility queries to access coordinates.

**Primary recommendation:** Extend MapContainer to accept a mode parameter (single vs multi-pin), add a new `initMultiMap` script function for markercluster+circleMarker+fitBounds, add Latitude/Longitude fields to row structs, and enrich handler queries with `WithFacility()` to load coordinates.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Use Leaflet.markercluster plugin delivered via CDN in layout.templ alongside Leaflet. Standard clustering with animated expand on click and numbered circles.
- **D-02:** Custom dark-mode-aware cluster styling. Override default markercluster CSS with neutral/emerald colors matching the app palette. Different backgrounds for dark vs light mode.
- **D-03:** No marker count limit -- trust clustering to handle any dataset size. PeeringDB's largest datasets (~200 facilities per network) are well within markercluster's capability.
- **D-04:** Three-color scheme on comparison maps: shared facilities in emerald (app accent), Network A unique in sky blue, Network B unique in amber. Three distinct categories.
- **D-05:** Use L.circleMarker with fillColor on ALL multi-pin maps (IX, network, compare). Default teardrop markers only on single-facility maps (Phase 44). Consistent multi-pin style.
- **D-06:** Comparison map includes a simple legend in a map corner: colored dot + label for each category (Shared, AS{X} only, AS{Y} only).
- **D-07:** Multi-pin maps appear above collapsible sections on all pages -- same position as the Phase 44 facility map. Consistent placement across IX, network, and comparison pages.
- **D-08:** On comparison results page, map appears after header/stats, before the IXP/Facility/Campus comparison sections. Visual overview of geographic overlap.
- **D-09:** Use Leaflet fitBounds() with maxZoom capped at 13 (city level). Prevents over-zooming when only 1-2 nearby pins exist.
- **D-10:** If a multi-pin map has zero mappable facilities (all lack valid coordinates), hide the map entirely -- no container rendered.
- **D-11:** Show count of unmapped facilities below the map when some facilities lack coordinates. E.g., "3 facilities not shown (no location data)." Honest about completeness.
- **D-12:** Eager-load facility coordinates with page data. Add Latitude/Longitude to existing facility queries in IX/network/comparison detail handlers. No extra requests.
- **D-13:** Add Latitude/Longitude fields to ALL facility row structs: IXFacilityRow, NetworkFacRow, OrgFacilityRow, CampusFacilityRow, CarrierFacilityRow, and comparison facility types. Consistent enrichment for future extensibility.
- **D-14:** IX/network map popups: facility name + city/country + link to facility detail page. No network/IX counts (irrelevant in this context).
- **D-15:** Comparison map popups: facility name + city/country + which networks are present (e.g., "Equinix DC2 -- Washington, US -- Cloudflare + Google") + link to facility detail page.
- **D-16:** Multi-pin maps are taller than single-facility maps: mobile 250px, desktop 450px (vs Phase 44's 200px/350px). More vertical space for wider geographic spread and clustering.

### Claude's Discretion
- Exact markercluster CSS override values for dark/light modes
- CircleMarker radius and stroke styling for multi-pin maps
- Legend position (top-right, bottom-left, etc.) and styling
- Whether "N facilities not shown" message is inside or below the map container
- How comparison facility data includes network presence info for popup content

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| MAP-02 | User sees an interactive map on IX and network detail pages showing all associated facility locations with clustering | Leaflet.markercluster 1.5.3 CDN, `WithFacility()` eager-loading for IxFacility/NetworkFacility queries, circleMarker shim for clustering, fitBounds with maxZoom:13 |
| MAP-03 | User sees an interactive map on ASN comparison page with colored pins (shared vs unique facilities) | Three-color circleMarker scheme (emerald/sky/amber), existing compare handler facility union already computed, needs Latitude/Longitude from Facility edge, legend component |
</phase_requirements>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Leaflet | 1.9.4 | Map rendering | Already in project via Phase 44. CDN delivery in layout.templ. |
| Leaflet.markercluster | 1.5.3 | Marker clustering | Standard Leaflet clustering plugin. Latest stable. D-01. |

### CDN Resources (markercluster)

**CSS (2 files):**
```html
<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/leaflet.markercluster/1.5.3/MarkerCluster.min.css"
    integrity="sha512-ENrTWqddXrLJsQS2A86QmvA17PkJ0GVm1bqj5aTgpeMAfDKN2+SIOLpKG8R/6KkimnhTb+VW5qqUHB/r1zaRgg=="
    crossorigin=""/>
<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/leaflet.markercluster/1.5.3/MarkerCluster.Default.min.css"
    integrity="sha512-fYyZwU1wU0QWB4Yutd/Pvhy5J1oWAwFXun1pt+Bps04WSe4Aq6tyHlT4+MHSJhD8JlLfgLuC4CbCnX5KHSjyCg=="
    crossorigin=""/>
```

**JS (1 file):**
```html
<script src="https://cdnjs.cloudflare.com/ajax/libs/leaflet.markercluster/1.5.3/leaflet.markercluster.min.js"
    integrity="sha512-TiMWaqipFi2Vqt4ugRzsF8oRoGFlFFuqIi30FFxEPNw58Ov9mOy6LgC05ysfkxwLE0xVeZtmr92wVg9siAFRWA=="
    crossorigin=""></script>
```

Note: `MarkerCluster.Default.min.css` provides the default cluster icon styling. Since D-02 overrides with custom dark-mode-aware colors, we still include it as a base and apply CSS overrides via a `<style>` block.

## Architecture Patterns

### Data Flow Pattern

```
Ent Query (WithFacility) -> Handler populates Row struct with Lat/Lng -> Template filters valid coords ->
templ renders markers JSON into script -> JS creates L.markerClusterGroup + L.circleMarker -> fitBounds
```

### Key Structural Changes

**1. Row Struct Enrichment (detailtypes.go + comparetypes.go)**

Add `Latitude float64` and `Longitude float64` to:
- `IXFacilityRow` -- for IX detail map
- `NetworkFacRow` -- for network detail map
- `OrgFacilityRow` -- future extensibility (D-13)
- `CampusFacilityRow` -- future extensibility (D-13)
- `CarrierFacilityRow` -- future extensibility (D-13)
- `CompareFacility` -- for comparison map

**2. Handler Query Enrichment (detail.go + compare.go)**

For IX facilities:
```go
// In queryIX, enrich the IxFacility query:
ixFacItems, err := h.client.IxFacility.Query().
    Where(ixfacility.HasInternetExchangeWith(internetexchange.ID(id))).
    WithFacility(). // <-- ADD THIS
    Order(ixfacility.ByName()).
    All(ctx)
```
Then extract coordinates: `if ixf.Edges.Facility != nil && ixf.Edges.Facility.Latitude != nil { ... }`

For network facilities:
```go
// In queryNetwork, enrich the NetworkFacility query:
facItems, facErr := h.client.NetworkFacility.Query().
    Where(networkfacility.HasNetworkWith(network.ID(net.ID))).
    WithFacility(). // <-- ADD THIS
    Order(networkfacility.ByName()).
    All(ctx)
```

For compare facilities:
```go
// In compare.go fan-out queries, add WithFacility() to the 4 NetworkFacility queries.
// Propagate Latitude/Longitude through computeSharedFacilities and computeAllFacilities.
```

**3. MapContainer Extension (map.templ)**

The existing `MapContainer` and `initMap` handle single-pin mode. For multi-pin:
- New `MultiPinMapContainer` templ component with taller height classes (`h-[250px] md:h-[450px]`)
- New `initMultiMap` script function that creates `L.markerClusterGroup`, adds `L.circleMarker` instances, calls `fitBounds`
- Separate `CompareMapContainer` templ component or parameterized variant that includes the legend

**4. Template Integration Points**

- `detail_ix.templ`: Insert map between Notes and collapsible sections (same position as facility map in `detail_fac.templ`)
- `detail_net.templ`: Insert map between Notes and collapsible sections
- `compare.templ`: Insert map between stat badges and IXP section in `CompareResultsPage`

### CircleMarker + Markercluster Shim

```javascript
// Required shim: L.circleMarker lacks setOpacity which markercluster needs for animations.
// Add once in layout.templ after markercluster script loads.
L.CircleMarker.include({
    setOpacity: function(opacity) {
        this.setStyle({ opacity: opacity, fillOpacity: opacity });
    }
});
```

This is the standard community workaround (GitHub issue #62, resolved 2012). The shim adds `setOpacity` which delegates to `setStyle`, allowing markercluster's animation code to function with circleMarkers.

### CircleMarker Styling

```javascript
L.circleMarker([lat, lng], {
    radius: 7,
    fillColor: '#10b981',  // emerald for shared
    color: '#fff',          // white stroke
    weight: 2,
    opacity: 1,
    fillOpacity: 0.85
}).bindPopup(popupHTML);
```

**Color mapping (D-04):**
| Category | fillColor | Hex | Tailwind |
|----------|-----------|-----|----------|
| Shared (comparison) | emerald | `#10b981` | emerald-500 |
| Network A only | sky blue | `#0ea5e9` | sky-500 |
| Network B only | amber | `#f59e0b` | amber-500 |
| Default (IX/network maps) | emerald | `#10b981` | emerald-500 |

### Custom Cluster Styling (D-02)

Override `MarkerCluster.Default.css` with dark-mode-aware colors:

```css
/* Light mode cluster icons */
.marker-cluster-small { background-color: rgba(16, 185, 129, 0.3); }
.marker-cluster-small div { background-color: rgba(16, 185, 129, 0.6); }
.marker-cluster-medium { background-color: rgba(16, 185, 129, 0.4); }
.marker-cluster-medium div { background-color: rgba(16, 185, 129, 0.7); }
.marker-cluster-large { background-color: rgba(16, 185, 129, 0.5); }
.marker-cluster-large div { background-color: rgba(16, 185, 129, 0.8); }
.marker-cluster div { color: #fff; }

/* Dark mode overrides */
.dark .marker-cluster-small { background-color: rgba(16, 185, 129, 0.2); }
.dark .marker-cluster-small div { background-color: rgba(16, 185, 129, 0.5); }
.dark .marker-cluster-medium { background-color: rgba(16, 185, 129, 0.3); }
.dark .marker-cluster-medium div { background-color: rgba(16, 185, 129, 0.6); }
.dark .marker-cluster-large { background-color: rgba(16, 185, 129, 0.4); }
.dark .marker-cluster-large div { background-color: rgba(16, 185, 129, 0.7); }
```

### Popup Content Patterns

**IX/Network map popup (D-14):**
Server-side Go function `buildMultiPinPopupHTML` generates escaped HTML with inline styles (same pattern as Phase 44 `buildPopupHTML`). Content: facility name (bold), city/country, link to facility detail page. No network/IX counts.

**Comparison map popup (D-15):**
Server-side Go function `buildComparePopupHTML` generates escaped HTML. Content: facility name (bold), city/country, which networks are present (e.g. "Cloudflare + Google"), link to facility detail page.

Both use `html.EscapeString` for all user-provided content (facility names, city names) to prevent XSS. Inline styles required because Tailwind classes do not penetrate Leaflet's popup DOM (established in Phase 44 D-11).

### Legend Component (D-06)

A Leaflet Control positioned at bottom-left:

```javascript
var legend = L.control({ position: 'bottomleft' });
legend.onAdd = function() {
    var div = L.DomUtil.create('div', 'leaflet-legend');
    // Build legend content with inline styles
    // Three rows: emerald dot + "Shared", sky dot + "AS{X} only", amber dot + "AS{Y} only"
    return div;
};
legend.addTo(map);
```

The legend uses inline styles for the same reason as popups -- Tailwind doesn't penetrate Leaflet DOM.

### Unmapped Facilities Message (D-11)

Render a `<p>` below the map container showing unmapped count. Filter logic in templ:
```
total facilities - mappable facilities = unmapped count
```
Only render if unmapped > 0.

### Anti-Patterns to Avoid
- **Fetching coordinates in a separate request:** Per D-12, coordinates must be eager-loaded with page data. No lazy-loading or AJAX for coordinate data.
- **Using L.marker for multi-pin maps:** Per D-05, multi-pin maps use L.circleMarker. Only single-facility maps (Phase 44) use the default teardrop marker.
- **Rendering map for zero-coordinate datasets:** Per D-10, the entire map container must be hidden (not rendered) if no facilities have valid coordinates.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Marker clustering | Custom grid-based clustering | Leaflet.markercluster 1.5.3 | Battle-tested, handles edge cases (animation, spiderfying, zoom levels). D-01. |
| Auto-fit bounds | Manual min/max coordinate calculation | `L.featureGroup(markers).getBounds()` + `map.fitBounds()` | Leaflet's built-in bounds calculation handles wrap-around, padding, and maxZoom natively. |
| Legend control | Manual DOM positioning | `L.control` API | Proper z-index management, responsive positioning, Leaflet-aware lifecycle. |

## Common Pitfalls

### Pitfall 1: CircleMarker + Markercluster setOpacity
**What goes wrong:** Adding `L.circleMarker` to an `L.markerClusterGroup` throws errors or freezes because markercluster calls `setOpacity()` during animations, which `L.circleMarker` doesn't implement.
**Why it happens:** Markercluster was designed for `L.marker` which has `setOpacity`. `L.circleMarker` inherits from `L.Path` which uses `setStyle` instead.
**How to avoid:** Add the `L.CircleMarker.include({ setOpacity: ... })` shim before creating any cluster groups. Place in layout.templ after the markercluster script tag.
**Warning signs:** Map freezes on zoom, or "setOpacity is not a function" errors in console.

### Pitfall 2: Null Latitude/Longitude in Facility Entity
**What goes wrong:** Dereferencing nil pointer when Facility.Latitude or Facility.Longitude is nil.
**Why it happens:** Facility lat/lng are `Optional().Nillable()` in the ent schema, generating `*float64` fields.
**How to avoid:** Always check `fac.Edges.Facility != nil && fac.Edges.Facility.Latitude != nil && fac.Edges.Facility.Longitude != nil` before accessing. Also apply the null-island check: treat (0,0) as missing per Phase 44 D-18.
**Warning signs:** Panic on nil pointer dereference in handler code.

### Pitfall 3: Facility Edge Missing When WithFacility Not Called
**What goes wrong:** `ixf.Edges.Facility` is nil even though `fac_id` is set.
**Why it happens:** Ent requires explicit `WithFacility()` on the query to eager-load the edge. Without it, `Edges.Facility` is always nil.
**How to avoid:** Always chain `.WithFacility()` on IxFacility and NetworkFacility queries that need coordinates.
**Warning signs:** All facilities show as "unmapped" despite having coordinates in the database.

### Pitfall 4: FitBounds with No Markers
**What goes wrong:** Calling `fitBounds` on an empty FeatureGroup throws "Bounds are not valid" error.
**Why it happens:** No markers means no bounds to compute.
**How to avoid:** Per D-10, the map should not render at all if zero facilities have valid coordinates. The templ template filters markers server-side, so the JS should never receive an empty set. But add a defensive check in JS: `if (markers.length === 0) return;`
**Warning signs:** JavaScript console error on pages with no coordinated facilities.

### Pitfall 5: Compare Handler Needs Facility Coordinates Through Union Functions
**What goes wrong:** Coordinates are loaded via `WithFacility()` on the query but not propagated through `computeSharedFacilities` and `computeAllFacilities`.
**Why it happens:** These functions work with `*ent.NetworkFacility` which has `Edges.Facility` when eager-loaded, but the output `CompareFacility` struct currently lacks lat/lng fields.
**How to avoid:** Add Latitude/Longitude to `CompareFacility` struct. In the compute functions, extract coordinates from `nf.Edges.Facility.Latitude`/`Longitude` when the edge and fields are non-nil.
**Warning signs:** Compare map shows no pins despite facilities having coordinates.

### Pitfall 6: MarkerCluster CSS Override Specificity
**What goes wrong:** Custom cluster colors don't apply because MarkerCluster.Default.css has equal or higher specificity.
**Why it happens:** CSS load order matters. If the override `<style>` block loads before the CDN CSS, it gets overridden.
**How to avoid:** Place custom cluster CSS overrides AFTER the MarkerCluster.Default.css `<link>` in layout.templ `<head>`. The `.dark` prefix provides additional specificity for dark mode overrides.
**Warning signs:** Clusters show default green/yellow/red colors instead of emerald palette.

## Code Examples

### Filtering Valid Coordinates in Go (Template Helper)

```go
// filterMappableMarkers returns only markers with valid (non-zero, non-nil) coordinates.
// Returns the mappable markers and the count of unmapped facilities.
func filterMappableMarkers(markers []MapMarker) (mappable []MapMarker, unmapped int) {
    for _, m := range markers {
        if m.Lat != 0 || m.Lng != 0 {
            mappable = append(mappable, m)
        } else {
            unmapped++
        }
    }
    return mappable, unmapped
}
```

### Extracting Coordinates from IxFacility Edge

```go
// In queryIX handler, after WithFacility() eager-load:
for _, ixf := range ixFacItems {
    if ixf.FacID == nil {
        continue
    }
    row := templates.IXFacilityRow{
        FacName: ixf.Name,
        FacID:   *ixf.FacID,
        City:    ixf.City,
        Country: ixf.Country,
    }
    if fac := ixf.Edges.Facility; fac != nil {
        if fac.Latitude != nil {
            row.Latitude = *fac.Latitude
        }
        if fac.Longitude != nil {
            row.Longitude = *fac.Longitude
        }
    }
    facRows = append(facRows, row)
}
```

### Compare Facility Coordinate Propagation

```go
// In computeAllFacilities, extract coordinates from eager-loaded Facility edge:
for _, nf := range a {
    if nf.FacID == nil {
        continue
    }
    entry := &facEntry{
        facID:   *nf.FacID,
        facName: nf.Name,
        city:    nf.City,
        country: nf.Country,
        netA:    &templates.CompareFacPresence{LocalASN: nf.LocalAsn},
    }
    if fac := nf.Edges.Facility; fac != nil {
        if fac.Latitude != nil {
            entry.lat = *fac.Latitude
        }
        if fac.Longitude != nil {
            entry.lng = *fac.Longitude
        }
    }
    entries[*nf.FacID] = entry
}
```

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) + enttest |
| Config file | none (stdlib) |
| Quick run command | `go test ./internal/web/templates/ -run TestBuildPopup -race -count=1` |
| Full suite command | `go test ./internal/web/... -race -count=1` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| MAP-02 | IX detail map shows facility pins with clustering | unit | `go test ./internal/web/templates/ -run TestBuildMultiPinPopup -race -count=1` | Wave 0 |
| MAP-02 | Network detail map shows facility pins | unit | `go test ./internal/web/templates/ -run TestBuildMultiPinPopup -race -count=1` | Wave 0 |
| MAP-02 | Unmapped facility count filtering | unit | `go test ./internal/web/templates/ -run TestFilterMappable -race -count=1` | Wave 0 |
| MAP-02 | IX handler enriches facility rows with coordinates | integration | `go test ./internal/web/ -run TestIXFacilityCoordinates -race -count=1` | Wave 0 |
| MAP-02 | Network handler enriches facility rows with coordinates | integration | `go test ./internal/web/ -run TestNetworkFacilityCoordinates -race -count=1` | Wave 0 |
| MAP-03 | Compare handler propagates coordinates through facility unions | integration | `go test ./internal/web/ -run TestCompareFacilityCoordinates -race -count=1` | Wave 0 |
| MAP-03 | Compare popup includes network names | unit | `go test ./internal/web/templates/ -run TestBuildComparePopup -race -count=1` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/web/templates/ -race -count=1`
- **Per wave merge:** `go test ./internal/web/... -race -count=1`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/web/templates/map_test.go` -- add tests for multi-pin popup variants and coordinate filtering
- [ ] `internal/web/compare_test.go` -- add test for coordinate propagation through facility comparison
- [ ] `internal/web/detail_test.go` -- add test for coordinate enrichment in IX and network handlers

## Project Constraints (from CLAUDE.md)

- **CS-0 (MUST):** Modern Go code guidelines
- **CS-5 (MUST):** Input structs for functions with >2 args (relevant if new helper functions are created)
- **ERR-1 (MUST):** Wrap errors with `%w` and context
- **T-1 (MUST):** Table-driven tests, deterministic and hermetic
- **T-2 (MUST):** Run `-race` in CI
- **SEC-1 (MUST):** Validate inputs (popup HTML must be escaped -- already handled by `html.EscapeString` in existing `buildPopupHTML`)
- **API-1 (MUST):** Document exported items
- **OBS-1 (MUST):** Structured logging with slog (for any new error paths in handlers)
- **Code Generation:** `templ generate ./internal/web/templates/` after .templ changes. Commit `*_templ.go` alongside `.templ`.

## Open Questions

1. **templ script function parameter limits**
   - What we know: templ `script` blocks accept typed Go parameters. The existing `initMap` takes 5 params (string, float64, float64, int, string).
   - What's unclear: For multi-pin maps, we need to pass an array of marker objects. The cleanest approach is serializing markers to a JSON string in Go and passing it as a single string parameter to the JS function, then parsing with `JSON.parse()`.
   - Recommendation: Use JSON serialization approach. The Phase 44 pattern of individual params works for single markers but doesn't scale to arrays.

2. **MarkerCluster.Default.css necessity**
   - What we know: D-02 specifies custom cluster styling overriding defaults.
   - What's unclear: Whether to include Default.css and override, or skip it entirely and use `iconCreateFunction` for fully custom cluster icons.
   - Recommendation: Include Default.css for base layout (circle shape, sizing) and override only colors. Simpler than a full custom `iconCreateFunction`.

## Sources

### Primary (HIGH confidence)
- [Leaflet.markercluster GitHub](https://github.com/Leaflet/Leaflet.markercluster) - API, features, limitations
- [cdnjs leaflet.markercluster 1.5.3](https://cdnjs.com/libraries/leaflet.markercluster) - CDN URLs with SRI hashes
- [Leaflet reference](https://leafletjs.com/reference.html) - circleMarker, fitBounds, L.control APIs
- Project codebase - Phase 44 MapContainer, detail handlers, compare handler, ent schemas

### Secondary (MEDIUM confidence)
- [GitHub issue #62](https://github.com/Leaflet/Leaflet.markercluster/issues/62) - circleMarker + markercluster shim (setOpacity workaround, verified by multiple users since 2012)
- [GitHub issue #183](https://github.com/Leaflet/Leaflet.markercluster/issues/183) - circles/circleMarkers with markercluster confirmed working

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - Leaflet.markercluster is the canonical clustering solution, version verified on cdnjs with SRI hashes
- Architecture: HIGH - extending existing Phase 44 patterns, ent eager-loading verified in generated code
- Pitfalls: HIGH - circleMarker compatibility issue well-documented with proven shim, null handling patterns established in Phase 44
- Data enrichment: HIGH - WithFacility() verified in generated ent code, Facility entity Latitude/Longitude fields confirmed as `*float64`

**Research date:** 2026-03-26
**Valid until:** 2026-04-26 (stable libraries, no fast-moving changes)
