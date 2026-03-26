# Phase 44: Facility Map & Map Infrastructure - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-26
**Phase:** 44-facility-map-map-infrastructure
**Areas discussed:** Map component design, Tile provider & theming, Pin & popup behavior, Leaflet delivery, Missing lat/lng handling, Map accessibility, Map data plumbing, Attribution & legal
**Mode:** Interactive

---

## Map Component Design

| Option | Description | Selected |
|--------|-------------|----------|
| Above collapsible sections | After detail fields, before entity lists | ✓ |
| Top of page below header | Hero placement right after header | |
| Inside collapsible section | Own expandable section | |

**User's choice:** Above collapsible sections

| Option | Description | Selected |
|--------|-------------|----------|
| Fixed 300px | Consistent compact height | |
| Fixed 400px | Taller, more context | |
| Responsive (200px/350px) | Shorter mobile, taller desktop | ✓ |

**User's choice:** Responsive

| Option | Description | Selected |
|--------|-------------|----------|
| Full content width | Spans full container with rounded corners | ✓ |
| Inset with margins | Narrower than content | |

**User's choice:** Full content width

| Option | Description | Selected |
|--------|-------------|----------|
| City-level ~13 | Surrounding streets | |
| Neighborhood ~15 | Very close, block level | |
| Region ~10 | Wider area, more context | ✓ |

**User's choice:** Region zoom ~10

| Option | Description | Selected |
|--------|-------------|----------|
| Zoom buttons visible | Standard Leaflet +/- | ✓ |
| Hidden controls | Scroll/pinch only | |
| Zoom + fullscreen | Buttons plus fullscreen toggle | |

**User's choice:** Zoom buttons visible

| Option | Description | Selected |
|--------|-------------|----------|
| Disabled until click | scrollWheelZoom false, enable on click | ✓ |
| Always enabled | Immediate scroll zoom | |
| Disabled entirely | No scroll zoom ever | |

**User's choice:** Disabled until click

---

## Tile Provider & Theming

| Option | Description | Selected |
|--------|-------------|----------|
| CARTO Voyager/Dark | Free, light+dark variants | ✓ |
| OpenStreetMap default | Free, no dark mode | |
| Stadia/Stamen | Light+dark but needs API key | |

**User's choice:** CARTO Voyager/Dark

| Option | Description | Selected |
|--------|-------------|----------|
| Swap tile layer on toggle | Live swap matching dark mode | ✓ |
| Match on page load only | Check at init, no live swap | |
| Always dark tiles | Dark Matter regardless | |

**User's choice:** Swap tile layer on toggle

| Option | Description | Selected |
|--------|-------------|----------|
| Keyless with attribution | Public CDN, standard attribution | ✓ |
| Register for API key | Account for higher limits | |

**User's choice:** Keyless with attribution

---

## Pin & Popup Behavior

| Option | Description | Selected |
|--------|-------------|----------|
| Default Leaflet marker | Standard blue teardrop | ✓ |
| Circle marker | Flat colored circle | |
| Custom SVG pin | Custom-designed pin | |

**User's choice:** Default Leaflet marker

| Option | Description | Selected |
|--------|-------------|----------|
| Name + address | Facility name, city/country | |
| Name only | Just facility name | |
| Name + stats | Name, address, network/IX counts | ✓ |

**User's choice:** Name + stats

| Option | Description | Selected |
|--------|-------------|----------|
| Open by default | Auto-opens on single pin | |
| Closed — click to open | Pin visible, click for popup | ✓ |

**User's choice:** Closed — click to open

| Option | Description | Selected |
|--------|-------------|----------|
| Always include link | Consistent template everywhere | ✓ |
| Context-aware | No link on own page | |

**User's choice:** Always include link

---

## Leaflet Delivery

| Option | Description | Selected |
|--------|-------------|----------|
| CDN in layout.templ | Leaflet from CDN in head | ✓ |
| Lazy-load on map pages | Dynamic inject when needed | |
| Self-hosted in /static/ | Local copy, no CDN | |

**User's choice:** CDN in layout.templ

| Option | Description | Selected |
|--------|-------------|----------|
| Shared templ component | MapContainer accepting Go params | ✓ |
| Inline JS per page | Each page has own map init | |

**User's choice:** Shared templ component

| Option | Description | Selected |
|--------|-------------|----------|
| Leaflet 1.9.x | Latest stable | ✓ |
| Leaflet 2.0 beta | Newer but beta | |

**User's choice:** Leaflet 1.9.x

---

## Missing Lat/Lng Handling

| Option | Description | Selected |
|--------|-------------|----------|
| Completely hidden | Map div doesn't render | ✓ |
| Subtle message | 'No location data' text | |
| Collapsed section | Disabled collapsible | |

**User's choice:** Completely hidden

| Option | Description | Selected |
|--------|-------------|----------|
| Treat (0,0) as missing | No facility at null island | ✓ |
| Only if DB NULL | Check nullable field | |
| Show all non-nil | Trust the data | |

**User's choice:** Treat (0,0) as missing

---

## Map Accessibility

| Option | Description | Selected |
|--------|-------------|----------|
| Basic ARIA + alt text | aria-label, role, skip link | ✓ |
| Full keyboard navigation | Tab, arrow keys, zoom keys | |
| Minimal | Default Leaflet only | |

**User's choice:** Basic ARIA + alt text

---

## Map Data Plumbing

| Option | Description | Selected |
|--------|-------------|----------|
| Go params → templ script | Float64 params in script block | ✓ |
| data-* attributes on div | JS reads from HTML attrs | |
| JSON in hidden element | Script type application/json | |

**User's choice:** Go params → templ script

---

## Attribution & Legal

| Option | Description | Selected |
|--------|-------------|----------|
| Leaflet default attribution | Standard bottom-right control | ✓ |
| Compact attribution | Behind (i) icon | |
| Custom footer text | Move to page footer | |

**User's choice:** Leaflet default attribution

---

## Claude's Discretion

- Map container border/shadow treatment in dark mode
- Exact responsive height Tailwind classes
- MapContainer component signature (markers slice vs individual lat/lng)
- Leaflet marker image CDN URL

## Deferred Ideas

None
