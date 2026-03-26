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
- **D-01:** All child-entity lists become `<table>` elements for consistency, even single-column lists (e.g., Fac IXPs with just name). This provides uniform styling and future extensibility.
- **D-02:** Data-rich tables show all available fields as columns:
  - IX Participants: Name (linked), ASN, Speed (color-tiered), IPv4, IPv6, RS badge
  - Network IX Presences: IX Name (linked), Speed, IPv4, IPv6, RS badge
  - Network/IX/Org/Campus Facilities: Name (linked), City, Country + flag
  - Fac Networks / Org Networks: Name (linked), ASN
  - Contacts: Name, Role, Email, Phone
  - IX Prefixes: Prefix, Protocol, DFZ badge
  - Simple name-only lists (Fac IXPs, Fac Carriers, Org IXPs, Org Campuses, Org Carriers, Carrier Facilities): Name (linked) as single column
- **D-03:** Country flag appears in a dedicated column next to the country code, not inline with the name. Flag is the 4x3 SVG variant from flag-icons CSS.

### Sorting
- **D-04:** Client-side sorting via vanilla JS click handler on `<th>` elements. No external sorting library — consistent with existing vanilla JS patterns (keyboard nav, copy-to-clipboard, htmx error handling in layout.templ).
- **D-05:** Sort direction indicator is a small arrow (up/down) rendered via CSS pseudo-element or inline SVG on the active `<th>`.
- **D-06:** Sort state is ephemeral (client-side only, not persisted in URL). Expanding a section always loads with default sort.
- **D-07:** Default sort orders: IX Participants by ASN ascending, facility lists by country ascending, network lists by name ascending, contacts by role.
- **D-08:** Only columns with meaningful sort semantics are sortable. Name-only single-column tables have no sort UI.

### Country Flags
- **D-09:** Use flag-icons CSS library delivered via CDN `<link>` in layout.templ `<head>`. This matches the existing CDN delivery pattern (Tailwind browser, htmx).
- **D-10:** Country code in the data is ISO 3166-1 alpha-2 (PeeringDB's format), which maps directly to flag-icons CSS class names (`fi fi-{lowercase code}`).
- **D-11:** Missing or empty country codes render no flag (empty cell), not a placeholder.

### Responsive Behavior
- **D-12:** On screens below `md` (768px), hide low-priority columns: city, speed, IP addresses. Always show: name, country + flag, ASN (where applicable).
- **D-13:** Responsive hiding uses Tailwind utility classes (`hidden md:table-cell`) — pure CSS, zero JS.
- **D-14:** Tables get `overflow-x-auto` wrapper to prevent layout breakage on very narrow screens if column count is still too wide after hiding.

### Claude's Discretion
- Sort indicator styling (CSS arrow vs inline SVG — choose whatever looks cleanest in dark mode)
- Exact table cell padding/spacing (maintain visual density while keeping readability)
- Whether to keep existing speed color tiers and RS badge styling or adjust for table context
- Table header styling (sticky headers not needed — lists are inside collapsible sections that are bounded in size)

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

### Requirements
- `.planning/REQUIREMENTS.md` — DENS-01, DENS-02, DENS-03, SORT-01, SORT-02, SORT-03, FLAG-01

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `speedColorClass()` / `formatSpeed()` in detail_shared.templ — speed color tiers for table cells
- `CopyableIP` component — can be used inside table cells for IPv4/IPv6 columns
- `CollapsibleSection` / `CollapsibleSectionWithBandwidth` — htmx lazy-loading wrapper (tables load inside these)
- Keyboard navigation JS in layout.templ — pattern for vanilla JS event handling

### Established Patterns
- All child lists are htmx fragments loaded via `hx-get` on `<details>` expand
- Tailwind CSS via CDN browser plugin (not build-step compiled)
- Dark mode via `.dark` class on `<html>` with `dark:` variant
- Fragment handlers in `internal/web/detail.go` serve individual list sections

### Integration Points
- Each list template is a standalone templ component called from fragment handlers
- Adding flag-icons CSS link goes in layout.templ `<head>` (alongside tailwind/htmx)
- Sort JS can go in layout.templ or as a templ `script` block in a shared table component
- Row struct types in detailtypes.go may need City/Country fields added for types that currently lack them (e.g., FacNetworkRow, OrgNetworkRow currently lack Country)

</code_context>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches for table layout, sorting interaction, and flag presentation. The inspiration is Peercortex-style density (per PROJECT.md v1.11 goal).

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 43-dense-tables-with-sorting-and-flags*
*Context gathered: 2026-03-26*
