# Phase 45: Multi-Pin Maps - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-26
**Phase:** 45-multi-pin-maps
**Areas discussed:** Marker clustering, Colored pins, Map placement, Bounds & auto-fit, Multi-pin data loading, Popup content, Map height, Performance
**Mode:** Interactive

---

## Marker Clustering

| Option | Description | Selected |
|--------|-------------|----------|
| Leaflet.markercluster plugin | Standard plugin, CDN delivery | ✓ |
| Custom distance-based grouping | Write our own clustering | |
| No clustering | Show all pins individually | |

**User's choice:** Leaflet.markercluster plugin

| Option | Description | Selected |
|--------|-------------|----------|
| CDN alongside Leaflet | In layout.templ head | ✓ |
| Lazy-load when needed | Inject on multi-pin pages only | |

**User's choice:** CDN alongside Leaflet

| Option | Description | Selected |
|--------|-------------|----------|
| Custom dark-mode-aware | Override CSS with app palette | ✓ |
| Plugin defaults | Standard green/yellow/red | |
| Monochrome neutral | Single neutral color | |

**User's choice:** Custom dark-mode-aware

---

## Colored Pins (Comparison)

| Option | Description | Selected |
|--------|-------------|----------|
| Three colors: shared/netA/netB | Emerald/sky/amber | ✓ |
| Two colors: shared vs unique | Emerald/gray | |
| Opacity-based | Same color, different opacity | |

**User's choice:** Three colors

| Option | Description | Selected |
|--------|-------------|----------|
| L.circleMarker with fill color | Flat colored circles | ✓ |
| CSS-filtered Leaflet markers | Hue-rotate on teardrop | |
| Custom icon images | Colored PNGs/SVGs | |

**User's choice:** L.circleMarker

| Option | Description | Selected |
|--------|-------------|----------|
| Yes — simple legend | Colored dot + label per category | ✓ |
| No legend | Colors self-evident | |

**User's choice:** Yes — simple legend

| Option | Description | Selected |
|--------|-------------|----------|
| CircleMarkers everywhere | All multi-pin maps use circles | ✓ |
| Teardrop for IX/net, circles for compare | Mixed markers | |
| CircleMarkers replace teardrops everywhere | Retroactive change | |

**User's choice:** CircleMarkers on all multi-pin maps, teardrops for single-facility (Phase 44)

---

## Map Placement

| Option | Description | Selected |
|--------|-------------|----------|
| Above collapsible sections | Same as facility map position | ✓ |
| Inside Facilities section | Embedded in collapsible | |
| Below all sections | Bottom of page | |

**User's choice:** Above collapsible sections (all pages)

| Option | Description | Selected |
|--------|-------------|----------|
| Above comparison sections | After header/stats, before tables | ✓ |
| Between header and view toggle | Most prominent | |
| Below all sections | Geographic summary at bottom | |

**User's choice:** Above comparison sections

---

## Bounds & Auto-Fit

| Option | Description | Selected |
|--------|-------------|----------|
| fitBounds with padding | Auto-zoom to all pins | |
| Fixed zoom, center on centroid | Predictable but may clip | |
| fitBounds with maxZoom cap | Auto-fit capped at zoom 13 | ✓ |

**User's choice:** fitBounds with maxZoom cap at 13

| Option | Description | Selected |
|--------|-------------|----------|
| Hide map entirely | No container if zero pins | ✓ |
| Show empty map with message | Map tiles + text overlay | |
| Show map centered default | World view, no pins | |

**User's choice:** Hide map entirely

---

## Multi-Pin Data Loading

| Option | Description | Selected |
|--------|-------------|----------|
| Eager-load with page data | Add lat/lng to existing queries | ✓ |
| Separate htmx fragment | Lazy-load coordinates | |
| Embedded JSON blob | Script tag with coords | |

**User's choice:** Eager-load

| Option | Description | Selected |
|--------|-------------|----------|
| Yes — all facility rows | Add lat/lng to every row struct | ✓ |
| Only map-relevant rows | Only types used in this phase | |

**User's choice:** All facility rows

---

## Popup Content

| Option | Description | Selected |
|--------|-------------|----------|
| Name + city/country + link | Consistent, no counts | ✓ |
| Same as Phase 44 (name + stats) | Full popup with counts | |
| Name + link only | Minimal | |

**User's choice:** Name + city/country + link (IX/network maps)

| Option | Description | Selected |
|--------|-------------|----------|
| Yes — show both network names | Facility + which networks present | ✓ |
| Same as IX/network popups | Just facility info | |

**User's choice:** Show both network names (comparison popups)

---

## Map Height

| Option | Description | Selected |
|--------|-------------|----------|
| Taller: 250px/450px | More space for multi-pin | ✓ |
| Same as Phase 44: 200px/350px | Consistent height | |
| Even taller: 300px/500px | Maximum real estate | |

**User's choice:** 250px/450px

---

## Performance

| Option | Description | Selected |
|--------|-------------|----------|
| Trust clustering, no limit | markercluster handles thousands | ✓ |
| Cap at 500 markers | Defensive limit | |
| Progressive loading | Load more on zoom/pan | |

**User's choice:** Trust clustering, no limit

---

## Partial Data

| Option | Description | Selected |
|--------|-------------|----------|
| Only show facilities with coords | Silently omit missing | |
| Show count of unmapped | Note below map: "3 not shown" | ✓ |
| Placeholder markers | Default location for missing | |

**User's choice:** Show count of unmapped

---

## Claude's Discretion

- Markercluster CSS override values
- CircleMarker radius and stroke styling
- Legend position and styling
- "N facilities not shown" message placement
- Comparison popup network presence data flow

## Deferred Ideas

None
