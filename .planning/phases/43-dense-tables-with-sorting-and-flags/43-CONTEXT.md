# Phase 43: Dense Tables with Sorting and Flags - Context

**Gathered:** 2026-03-26
**Status:** Ready for planning

<domain>
## Phase Boundary

Convert all detail page child-entity lists from stacked div-based layouts to information-dense `<table>` elements with sortable column headers, parsed city/country columns with SVG country flags, and responsive column hiding on narrow screens. Covers all 6 entity detail pages (network, IX, facility, org, campus, carrier) and their ~15 child-entity list fragments.

</domain>

<decisions>
## Implementation Decisions

### Table Columns
- **D-01:** All child-entity lists become `<table>` elements for consistency, even single-column lists (e.g., Fac IXPs with just name). Uniform styling and future extensibility.
- **D-02:** Data-rich tables show all available fields as columns:
  - IX Participants: Name (linked), ASN, Speed (color-tiered), IPv4, IPv6, RS badge
  - Network IX Presences: IX Name (linked), Speed, IPv4, IPv6, RS badge
  - Network/IX/Org/Campus Facilities: Name (linked), City, Country + flag
  - Fac Networks / Org Networks: Name (linked), ASN, Country + flag
  - Contacts: Name, Role, Email, Phone
  - IX Prefixes: Prefix, Protocol, DFZ badge
  - Simple name-only lists (Fac IXPs, Fac Carriers, Org IXPs, Org Campuses, Org Carriers, Carrier Facilities): Name (linked) as single column
- **D-03:** Country flag appears in a dedicated column next to the country code, not inline with the name. Flag is the 4x3 SVG rectangle variant from flag-icons CSS.
- **D-04:** Enrich row types that currently lack City/Country fields (FacNetworkRow, OrgNetworkRow) by joining through the network/facility relationship in the query. All entity lists with geographic data get a country+flag column.
- **D-05:** IPv4/IPv6 columns reuse existing CopyableIP component inside table cells — click-to-copy behavior preserved.
- **D-06:** Table headers (`<thead>`) are visible with column names — needed for sort indicators and accessibility.

### Sorting
- **D-07:** Client-side sorting via vanilla JS click handler on `<th>` elements. No external sorting library — consistent with existing vanilla JS patterns (keyboard nav, copy-to-clipboard, htmx error handling in layout.templ).
- **D-08:** Sort direction indicator is a CSS triangle (border trick or ::after pseudo-element) on the active `<th>`.
- **D-09:** Sort state is ephemeral (client-side only, not persisted in URL). Expanding a section always loads with default sort.
- **D-10:** Default sort orders: IX Participants by ASN ascending, facility lists by country ascending, network lists by name ascending, contacts by role.
- **D-11:** Only multi-column tables get sort UI. Single-column name-only tables are pre-sorted alphabetically with no sort interaction.
- **D-12:** Sort values stored in `data-sort-value` attributes on `<td>` elements (e.g., raw numeric ASN, Mbps speed). JS compares these, not displayed text.
- **D-13:** Empty/missing values sort last regardless of sort direction.

### Country Flags
- **D-14:** Use flag-icons CSS library delivered via CDN `<link>` in layout.templ `<head>`. Matches existing CDN delivery pattern (Tailwind browser, htmx).
- **D-15:** Country code in the data is ISO 3166-1 alpha-2 (PeeringDB's format), which maps directly to flag-icons CSS class names (`fi fi-{lowercase code}`).
- **D-16:** Missing or empty country codes render no flag (empty cell), not a placeholder.

### Responsive Behavior
- **D-17:** On screens below `md` (768px), aggressively hide: city, speed, IP addresses, RS badge. Mobile shows only: name, ASN (where applicable), country + flag.
- **D-18:** Responsive hiding uses Tailwind utility classes (`hidden md:table-cell`) — pure CSS, zero JS.
- **D-19:** Tables get `overflow-x-auto` wrapper div as a safety net for edge cases.

### Table Visual Style
- **D-20:** Subtle zebra striping — faint even/odd background difference (e.g., neutral-800/30 on even rows) to aid scanning dense data.
- **D-21:** Table headers have subtle sticky appearance: faint background (neutral-800/70), slightly smaller text, bottom border — distinct but not heavy.
- **D-22:** Row hover preserved: `hover:bg-neutral-800/50` on `<tr>` — consistent with current list hover behavior.

### Row Density
- **D-23:** Compact cell padding: `px-3 py-1.5` (~6px vertical) — true density improvement over current `px-4 py-3`.
- **D-24:** Font size: `text-sm` (14px) throughout all table cells — uniform, dense, readable.

### Empty States
- **D-25:** Keep current "No X found." pattern — render as text in a colspan cell when section has 0 rows. Consistent with existing empty states.

### Claude's Discretion
- Exact speed color tier and RS badge adaptation for table cell context (keep existing `speedColorClass()` or adjust)
- Table border treatment (borderless between cells or subtle grid lines)
- Sort JS placement (layout.templ script block or templ `script` in shared table component)
- Whether table headers should be `position: sticky` within their scroll container

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Templates (current card-based layouts to convert)
- `internal/web/templates/detail_ix.templ` — IX participants, facilities, prefixes lists
- `internal/web/templates/detail_net.templ` — Network IX presences, facilities, contacts lists
- `internal/web/templates/detail_fac.templ` — Facility networks, IXPs, carriers lists
- `internal/web/templates/detail_org.templ` — Org networks, IXPs, facilities, campuses, carriers lists
- `internal/web/templates/detail_campus.templ` — Campus facilities list
- `internal/web/templates/detail_carrier.templ` — Carrier facilities list
- `internal/web/templates/detail_shared.templ` — Shared components (CollapsibleSection, DetailHeader, speedColorClass, CopyableIP, formatSpeed)

### Data Types
- `internal/web/templates/detailtypes.go` — All row structs defining available fields per list type

### Layout
- `internal/web/templates/layout.templ` — `<head>` section for adding flag-icons CDN link; existing JS patterns for reference

### Fragment Handlers
- `internal/web/detail.go` — Fragment handler functions that populate row structs and render list templates

### Requirements
- `.planning/REQUIREMENTS.md` — DENS-01, DENS-02, DENS-03, SORT-01, SORT-02, SORT-03, FLAG-01

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `speedColorClass()` / `formatSpeed()` in detail_shared.templ — speed color tiers for table cells
- `CopyableIP` component — reuse inside table cells for IPv4/IPv6 columns (D-05)
- `CollapsibleSection` / `CollapsibleSectionWithBandwidth` — htmx lazy-loading wrapper (tables load inside these)
- Keyboard navigation JS in layout.templ — pattern for vanilla JS event handling
- `formatFacLocation()` — existing city+country formatter (may be split into separate columns)

### Established Patterns
- All child lists are htmx fragments loaded via `hx-get` on `<details>` expand
- Tailwind CSS via CDN browser plugin (not build-step compiled)
- Dark mode via `.dark` class on `<html>` with `dark:` variant
- Fragment handlers in `internal/web/detail.go` serve individual list sections

### Integration Points
- Each list template is a standalone templ component called from fragment handlers
- Adding flag-icons CSS link goes in layout.templ `<head>` (alongside tailwind/htmx)
- Sort JS goes in layout.templ or as a templ `script` block in a shared table component
- Row struct types in detailtypes.go need City/Country fields added for FacNetworkRow, OrgNetworkRow (D-04)
- Fragment handler queries in detail.go need enriched joins to populate new Country fields

</code_context>

<specifics>
## Specific Ideas

- Inspiration is Peercortex-style density (per PROJECT.md v1.11 goal)
- Aggressive mobile column hiding (D-17) — prioritize identity (name, ASN) and geography (country+flag) over detail data (IPs, speed)

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 43-dense-tables-with-sorting-and-flags*
*Context gathered: 2026-03-26*
