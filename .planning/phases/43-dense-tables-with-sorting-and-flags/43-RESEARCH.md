# Phase 43: Dense Tables with Sorting and Flags - Research

**Researched:** 2026-03-26
**Domain:** Web UI template conversion (templ + Tailwind CSS + vanilla JS)
**Confidence:** HIGH

## Summary

This phase converts all 16 child-entity list templates across 6 detail pages from stacked div-based card layouts to HTML `<table>` elements. The conversion is primarily a templ template rewrite with supporting changes to Go row structs, fragment handler queries, and a small vanilla JS sort function. No new Go dependencies are needed -- the only external addition is a CSS CDN link for country flag icons.

The work touches 4 files systematically: `detailtypes.go` (struct enrichment), `detail.go` (query enrichment for 2 handlers), 6 `detail_*.templ` files (16 list template rewrites), `detail_shared.templ` (shared table components), and `layout.templ` (CDN link + sort JS). The existing htmx fragment loading pattern, CollapsibleSection wrappers, and CopyableIP component are preserved unchanged -- tables render inside the same fragment boundaries.

**Primary recommendation:** Implement as a layered conversion: (1) shared table infrastructure in detail_shared.templ + layout.templ, (2) struct/query enrichment for FacNetworkRow and OrgNetworkRow, (3) multi-column table conversions, (4) single-column table conversions.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** All child-entity lists become `<table>` elements for consistency, even single-column lists
- **D-02:** Column layouts per table type (IX Participants: Name/ASN/Speed/IPv4/IPv6/RS; Network IX: IX Name/Speed/IPv4/IPv6/RS; Facility lists: Name/City/Country+flag; Fac/Org Networks: Name/ASN/Country+flag; Contacts: Name/Role/Email/Phone; IX Prefixes: Prefix/Protocol/DFZ; Simple lists: Name only)
- **D-03:** Country flag in dedicated column next to country code, 4x3 SVG rectangle from flag-icons CSS
- **D-04:** Enrich FacNetworkRow and OrgNetworkRow with City/Country via relationship joins
- **D-05:** CopyableIP reuse inside table cells
- **D-06:** Visible `<thead>` with column names
- **D-07:** Client-side sorting via vanilla JS click handler on `<th>` elements, no external library
- **D-08:** Sort direction indicator via CSS triangle (border trick or ::after pseudo-element)
- **D-09:** Ephemeral sort state (client-side only, not URL-persisted)
- **D-10:** Default sort: IX Participants by ASN asc, facilities by country asc, networks by name asc, contacts by role
- **D-11:** Only multi-column tables get sort UI; single-column pre-sorted alphabetically
- **D-12:** Sort values in `data-sort-value` attributes on `<td>` elements
- **D-13:** Empty/missing values sort last regardless of direction
- **D-14:** flag-icons CSS via CDN `<link>` in layout.templ
- **D-15:** ISO 3166-1 alpha-2 maps to `fi fi-{lowercase}` class
- **D-16:** Missing country = no flag, not placeholder
- **D-17:** Mobile (<768px) hides: city, speed, IP addresses, RS badge. Keeps: name, ASN, country+flag
- **D-18:** Responsive hiding via Tailwind `hidden md:table-cell`
- **D-19:** `overflow-x-auto` wrapper div as safety net
- **D-20:** Zebra striping with neutral-800/30 on even rows
- **D-21:** Sticky-styled headers: neutral-800/70 bg, smaller text, bottom border
- **D-22:** Row hover: `hover:bg-neutral-800/50`
- **D-23:** Compact padding: `px-3 py-1.5`
- **D-24:** Font size: `text-sm` (14px) throughout
- **D-25:** Empty states as colspan text cell

### Claude's Discretion
- Speed color tier and RS badge adaptation for table cell context
- Table border treatment (borderless vs subtle grid lines)
- Sort JS placement (layout.templ script block or templ `script` in shared component)
- Whether table headers should be `position: sticky` within scroll container

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| DENS-01 | Dense columnar tables for detail page child-entity lists | All 16 list templates identified; conversion pattern documented with exact column specs per D-02 |
| DENS-02 | Parsed city and country in dedicated columns | IXFacilityRow, NetworkFacRow, OrgFacilityRow, CampusFacilityRow already have City/Country; FacNetworkRow needs NetworkFacility.City/Country populated; OrgNetworkRow needs enrichment via Network->Organization.Country |
| DENS-03 | Responsive column hiding on narrow screens | Tailwind `hidden md:table-cell` pattern confirmed working with Tailwind CSS browser CDN; mobile keeps name/ASN/country only per D-17 |
| SORT-01 | Sortable columns via click | Vanilla JS sort using `data-sort-value` attributes; ~40 lines of JS; no library needed |
| SORT-02 | Sort direction indicators | CSS border-trick triangle on `<th>::after`; active column indicator with ascending/descending state |
| SORT-03 | Sensible default sort | Server-side ordering already applied in fragment handlers; matches D-10 defaults. JS preserves server order as initial state |
| FLAG-01 | SVG country flag icons | flag-icons v7.5.0 via jsdelivr CDN; `fi fi-{cc}` class on `<span>`; 4x3 aspect ratio by default |
</phase_requirements>

## Standard Stack

### Core (no new Go dependencies)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| flag-icons | 7.5.0 | Country flag SVG sprites via CSS | 10K+ GitHub stars, all 250+ country flags as SVG, CSS-only integration via CDN. ISO 3166-1 alpha-2 codes map directly to class names. No JS, no build step. |

### Supporting (already in project)

| Library | Version | Purpose | When Used |
|---------|---------|---------|-----------|
| Tailwind CSS Browser | 4.x (CDN) | Table styling, responsive hiding | All table classes: `table-cell`, `hidden md:table-cell`, zebra striping |
| templ | v0.3.x | Table template components | All 16 list template rewrites |
| htmx | 2.0.x (vendored) | Fragment loading unchanged | Tables load inside existing CollapsibleSection htmx containers |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| flag-icons CSS | Self-hosted SVG files | Requires managing 250+ SVG files, build step, larger repo. CDN is zero-maintenance and matches existing CDN pattern (Tailwind, htmx) |
| Vanilla JS sort | Alpine.js / htmx sort extension | Adds new dependency for ~40 lines of logic. Project convention is vanilla JS (keyboard nav, copy-to-clipboard, htmx error handling all use vanilla JS in layout.templ) |
| CSS triangles for sort indicators | SVG chevrons | CSS triangles are simpler, lighter, and widely used for sort indicators. No additional assets needed |

**Installation:**
```html
<!-- Add to layout.templ <head> section -->
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/flag-icons@7.5.0/css/flag-icons.min.css" />
```

No `go get` or `npm install` needed.

## Architecture Patterns

### Conversion Inventory

16 list templates across 6 detail pages need conversion:

**Multi-column tables (10 templates, WITH sort UI):**

| Template | File | Columns | Sort Default |
|----------|------|---------|-------------|
| `IXParticipantsList` | detail_ix.templ | Name(linked), ASN, Speed, IPv4, IPv6, RS | ASN asc |
| `NetworkIXLansList` | detail_net.templ | IX Name(linked), Speed, IPv4, IPv6, RS | Name asc |
| `NetworkFacilitiesList` | detail_net.templ | Name(linked), City, Country+flag | Country asc |
| `NetworkContactsList` | detail_net.templ | Name, Role, Email, Phone | Role asc |
| `IXFacilitiesList` | detail_ix.templ | Name(linked), City, Country+flag | Country asc |
| `IXPrefixesList` | detail_ix.templ | Prefix, Protocol, DFZ | Prefix asc |
| `FacNetworksList` | detail_fac.templ | Name(linked), ASN, Country+flag | Name asc |
| `OrgNetworksList` | detail_org.templ | Name(linked), ASN, Country+flag | Name asc |
| `OrgFacilitiesList` | detail_org.templ | Name(linked), City, Country+flag | Country asc |
| `CampusFacilitiesList` | detail_campus.templ | Name(linked), City, Country+flag | Country asc |

**Single-column tables (6 templates, NO sort UI, pre-sorted alphabetically):**

| Template | File | Column |
|----------|------|--------|
| `FacIXPsList` | detail_fac.templ | Name(linked) |
| `FacCarriersList` | detail_fac.templ | Name(linked) |
| `OrgIXPsList` | detail_org.templ | Name(linked) |
| `OrgCampusesList` | detail_org.templ | Name(linked) |
| `OrgCarriersList` | detail_org.templ | Name(linked) |
| `CarrierFacilitiesList` | detail_carrier.templ | Name(linked) |

### Data Enrichment Required

Two row structs need additional fields (D-04):

**FacNetworkRow** -- currently `{NetName, ASN}`, needs `{NetName, ASN, City, Country}`:
- Source: `NetworkFacility` entity already stores `City` and `Country` as computed fields
- The `handleFacNetworksFragment` handler already queries NetworkFacility but does not populate City/Country into the row
- Fix: add `City string` and `Country string` to FacNetworkRow; populate from `nf.City` / `nf.Country` in handler
- No query changes needed -- data is already fetched

**OrgNetworkRow** -- currently `{NetName, ASN}`, needs `{NetName, ASN, Country}`:
- Source: `Network` entity does NOT have a country field
- The network's org has Country, but since all networks on an org page share the same org, showing the org's country is redundant
- Better approach: skip country enrichment for OrgNetworkRow since it provides no useful information on the org detail page. Instead, display Name + ASN only (still a multi-column table for sort support)
- Alternative (if user insists): eager-load Network.Organization.Country, but this adds a JOIN for a column that shows "DE" on every row of DE-CIX's org page
- **Recommendation for planner:** Implement OrgNetworkRow as a 2-column table (Name, ASN) with sort. Flag D-04 enrichment as technically impractical for this type specifically. If the user wants country on org networks, the only source is the org's own country (same for all rows).

### Template Pattern: Multi-Column Sortable Table

```go
// detail_shared.templ - shared table wrapper
templ SortableTable(tableID string) {
    <div class="overflow-x-auto">
        <table id={ tableID } class="w-full text-sm">
            { children... }
        </table>
    </div>
}
```

```go
// Example: IXParticipantsList conversion
templ IXParticipantsList(rows []IXParticipantRow) {
    if len(rows) == 0 {
        <div class="px-4 py-3 text-neutral-500 text-sm">No participants found.</div>
    } else {
        <div class="overflow-x-auto">
            <table class="w-full text-sm sortable">
                <thead>
                    <tr class="text-left text-xs text-neutral-400 bg-neutral-800/70 border-b border-neutral-700">
                        <th class="px-3 py-1.5 cursor-pointer" data-sort-type="string">Name</th>
                        <th class="px-3 py-1.5 cursor-pointer" data-sort-type="number">ASN</th>
                        <th class="px-3 py-1.5 cursor-pointer hidden md:table-cell" data-sort-type="number">Speed</th>
                        <th class="px-3 py-1.5 hidden md:table-cell">IPv4</th>
                        <th class="px-3 py-1.5 hidden md:table-cell">IPv6</th>
                        <th class="px-3 py-1.5 hidden md:table-cell">RS</th>
                    </tr>
                </thead>
                <tbody>
                    for _, row := range rows {
                        <tr class="border-b border-neutral-800/50 hover:bg-neutral-800/50 even:bg-neutral-800/30">
                            <td class="px-3 py-1.5" data-sort-value={ row.NetName }>
                                <a href={ templ.SafeURL(fmt.Sprintf("/ui/asn/%d", row.ASN)) }
                                    class="text-neutral-100 hover:text-sky-400 transition-colors">
                                    if row.NetName != "" {
                                        { row.NetName }
                                    } else {
                                        { fmt.Sprintf("AS%d", row.ASN) }
                                    }
                                </a>
                            </td>
                            <td class="px-3 py-1.5 font-mono text-neutral-400" data-sort-value={ fmt.Sprintf("%d", row.ASN) }>
                                { fmt.Sprintf("AS%d", row.ASN) }
                            </td>
                            <td class={ "px-3 py-1.5 font-mono hidden md:table-cell " + speedColorClass(row.Speed) } data-sort-value={ fmt.Sprintf("%d", row.Speed) }>
                                { formatSpeed(row.Speed) }
                            </td>
                            <td class="px-3 py-1.5 hidden md:table-cell">
                                if row.IPAddr4 != "" {
                                    @CopyableIP("", row.IPAddr4)
                                }
                            </td>
                            <td class="px-3 py-1.5 hidden md:table-cell">
                                if row.IPAddr6 != "" {
                                    @CopyableIP("", row.IPAddr6)
                                }
                            </td>
                            <td class="px-3 py-1.5 hidden md:table-cell">
                                if row.IsRSPeer {
                                    <span class="text-xs text-emerald-400 border border-emerald-400/30 rounded px-1.5 py-0.5 font-mono">RS</span>
                                }
                            </td>
                        </tr>
                    }
                </tbody>
            </table>
        </div>
    }
}
```

### Template Pattern: Country Flag Cell

```go
// Reusable inline pattern for country + flag column
<td class="px-3 py-1.5" data-sort-value={ row.Country }>
    if row.Country != "" {
        <span class={ "fi fi-" + strings.ToLower(row.Country) + " mr-1.5" }></span>
        <span class="text-neutral-400 font-mono text-xs">{ row.Country }</span>
    }
</td>
```

### Template Pattern: Single-Column Name-Only Table

```go
// No <thead> sort UI. Pre-sorted alphabetically by server.
templ FacIXPsList(rows []FacIXRow) {
    if len(rows) == 0 {
        <div class="px-4 py-3 text-neutral-500 text-sm">No IXPs found.</div>
    } else {
        <div class="overflow-x-auto">
            <table class="w-full text-sm">
                <tbody>
                    for _, row := range rows {
                        <tr class="border-b border-neutral-800/50 hover:bg-neutral-800/50 even:bg-neutral-800/30">
                            <td class="px-3 py-1.5">
                                <a href={ templ.SafeURL(fmt.Sprintf("/ui/ix/%d", row.IXID)) }
                                    class="text-neutral-100 hover:text-sky-400 transition-colors">
                                    { row.IXName }
                                </a>
                            </td>
                        </tr>
                    }
                </tbody>
            </table>
        </div>
    }
}
```

### Sort JavaScript Pattern

```javascript
// ~40 lines, placed in layout.templ <script> block (matches existing JS patterns)
(function() {
    document.addEventListener('click', function(e) {
        var th = e.target.closest('table.sortable th[data-sort-type]');
        if (!th) return;

        var table = th.closest('table');
        var tbody = table.querySelector('tbody');
        var colIndex = Array.from(th.parentNode.children).indexOf(th);
        var sortType = th.getAttribute('data-sort-type');

        // Toggle direction
        var asc = th.getAttribute('data-sort-dir') !== 'asc';
        // Clear all sort indicators in this table
        th.parentNode.querySelectorAll('th').forEach(function(h) {
            h.removeAttribute('data-sort-dir');
        });
        th.setAttribute('data-sort-dir', asc ? 'asc' : 'desc');

        var rows = Array.from(tbody.querySelectorAll('tr'));
        rows.sort(function(a, b) {
            var aVal = a.children[colIndex] ? a.children[colIndex].getAttribute('data-sort-value') || '' : '';
            var bVal = b.children[colIndex] ? b.children[colIndex].getAttribute('data-sort-value') || '' : '';

            // Empty values sort last regardless of direction
            if (aVal === '' && bVal !== '') return 1;
            if (aVal !== '' && bVal === '') return -1;
            if (aVal === '' && bVal === '') return 0;

            var cmp;
            if (sortType === 'number') {
                cmp = parseFloat(aVal) - parseFloat(bVal);
            } else {
                cmp = aVal.localeCompare(bVal, undefined, {sensitivity: 'base'});
            }
            return asc ? cmp : -cmp;
        });

        rows.forEach(function(row) { tbody.appendChild(row); });
    });
})();
```

### CSS for Sort Indicators

```css
/* In layout.templ <style> block */
table.sortable th[data-sort-type] { position: relative; user-select: none; }
table.sortable th[data-sort-type]::after {
    content: '';
    display: inline-block;
    margin-left: 4px;
    border: 4px solid transparent;
    vertical-align: middle;
}
table.sortable th[data-sort-dir="asc"]::after {
    border-bottom-color: #10b981;
    border-top: 0;
}
table.sortable th[data-sort-dir="desc"]::after {
    border-top-color: #10b981;
    border-bottom: 0;
}
```

### CopyableIP Adaptation for Table Cells

The existing `CopyableIP` component renders a `<div>` with flex layout. Inside a `<td>`, this works correctly because `<div>` is valid content in `<td>`. The "IPv4"/"IPv6" label prefix should be dropped (or made empty string) in table cells since the column header already identifies the protocol. Pass `""` as the label parameter.

### File Change Map

| File | Change Type | Scope |
|------|-------------|-------|
| `internal/web/templates/detailtypes.go` | Add fields | FacNetworkRow: +City, +Country |
| `internal/web/detail.go` | Enrich handler | handleFacNetworksFragment: populate City/Country from nf.City/nf.Country |
| `internal/web/detail.go` | Enrich handler | queryFacility eager-load: populate City/Country |
| `internal/web/templates/layout.templ` | Add CDN link | flag-icons CSS in `<head>` |
| `internal/web/templates/layout.templ` | Add JS | Sort function in `<script>` block |
| `internal/web/templates/layout.templ` | Add CSS | Sort indicator styles in `<style>` block |
| `internal/web/templates/detail_ix.templ` | Rewrite | IXParticipantsList, IXFacilitiesList, IXPrefixesList |
| `internal/web/templates/detail_net.templ` | Rewrite | NetworkIXLansList, NetworkFacilitiesList, NetworkContactsList |
| `internal/web/templates/detail_fac.templ` | Rewrite | FacNetworksList, FacIXPsList, FacCarriersList |
| `internal/web/templates/detail_org.templ` | Rewrite | OrgNetworksList, OrgIXPsList, OrgFacilitiesList, OrgCampusesList, OrgCarriersList |
| `internal/web/templates/detail_campus.templ` | Rewrite | CampusFacilitiesList |
| `internal/web/templates/detail_carrier.templ` | Rewrite | CarrierFacilitiesList |
| `internal/web/templates/detail_shared.templ` | Unchanged | speedColorClass, formatSpeed, CopyableIP all reused as-is |
| `internal/web/detail_test.go` | Update assertions | Fragment tests should check for `<table>`, `<th>`, flag-icon classes, data-sort-value |

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Country flag rendering | Custom SVG embedding or flag image hosting | flag-icons CSS CDN (fi fi-{cc} classes) | 250+ countries, maintained by community, CSS-only, zero-JS, sub-100KB |
| Table sort library | Full-featured sort library (e.g., tablesort.js) | ~40 lines vanilla JS with data-sort-value | Project convention is vanilla JS; sort needs are simple (string/number comparison, empty-last); no dependency needed |
| Responsive table framework | CSS framework table component | Tailwind `hidden md:table-cell` | Built-in to Tailwind, zero overhead, exact control over which columns hide |

**Key insight:** This phase is a template rewrite, not a feature build. The complexity is in the number of templates (16) and the consistency requirement, not in any individual conversion. The sort JS and flag CSS are both tiny additions.

## Common Pitfalls

### Pitfall 1: CopyableIP Inside Table Cells
**What goes wrong:** The existing `CopyableIP` component uses `<div>` wrappers with `group` hover. Inside a `<td>`, the hover group may interfere with the row-level hover.
**Why it happens:** CSS `group` class on a `<div>` inside `<td>` creates a nested hover context.
**How to avoid:** The `group` hover on CopyableIP is scoped to the clipboard icon (opacity-0 -> opacity-100). This works fine inside `<td>` because `group` is scoped to the element with the class. The row hover (`hover:bg-neutral-800/50`) applies to `<tr>` and does not conflict. Test visually.
**Warning signs:** Clipboard icon appears/disappears at wrong times; hover background flickers.

### Pitfall 2: Even/Odd Zebra Striping Breaks With Hidden Rows
**What goes wrong:** If rows are ever filtered or hidden, CSS `even:` pseudo-class still counts hidden rows.
**Why it happens:** CSS `:nth-child(even)` counts DOM order, not visible order.
**How to avoid:** Not a concern for this phase -- all rows are always visible (no filtering). Zebra striping via `even:bg-neutral-800/30` on `<tr>` works correctly. If filtering is added later, switch to JS-applied striping.
**Warning signs:** Alternating colors appear inconsistent.

### Pitfall 3: Sort Breaks When htmx Reloads Fragment
**What goes wrong:** If a collapsible section is collapsed and re-opened, htmx re-fetches the fragment, replacing the sorted table with server-order data. Sort state is lost.
**Why it happens:** htmx uses `hx-trigger="toggle once from:closest details"` -- the "once" means it only loads once per page load. Fragment is NOT re-fetched on re-expand.
**How to avoid:** The existing `once` modifier on the htmx trigger means the fragment loads exactly once. Sorting is preserved across collapse/expand. No issue here, but document for future maintainers.
**Warning signs:** Sort indicators present but data unsorted.

### Pitfall 4: Tailwind Browser CDN and table-cell Class
**What goes wrong:** The Tailwind CSS Browser plugin generates styles on-the-fly. The class `md:table-cell` may not be recognized if Tailwind doesn't detect it.
**Why it happens:** Tailwind browser plugin scans the DOM for classes and generates CSS dynamically. htmx-injected content is scanned after swap.
**How to avoid:** Tailwind Browser v4 scans the DOM via MutationObserver, so htmx-swapped content IS detected. The `table-cell` display value is a standard Tailwind utility. No issues expected. Test with a fragment load to confirm.
**Warning signs:** Columns don't appear on desktop after htmx loads the fragment.

### Pitfall 5: data-sort-value for Speed Column
**What goes wrong:** Displaying "10G" but sorting by display text gives wrong order (100G < 10G lexicographically).
**Why it happens:** String sort on formatted values.
**How to avoid:** `data-sort-value` stores raw Mbps integer (e.g., `10000`). JS sort reads this attribute, not the displayed text. Per D-12.
**Warning signs:** Speed column sorts 100G before 10G.

### Pitfall 6: NetworkFacility City/Country for FacNetworkRow
**What goes wrong:** City/Country fields on NetworkFacility are "computed fields" that may be empty.
**Why it happens:** These fields are computed by PeeringDB's serializer and stored per D-40. They should always be populated for active records, but could be empty for incomplete data.
**How to avoid:** Per D-16, missing country renders no flag (empty cell). Template handles empty strings gracefully with conditional rendering.
**Warning signs:** Country columns empty for records that should have data.

### Pitfall 7: Flag-Icons CSS Specificity and Dark Mode
**What goes wrong:** Flag SVGs look wrong or have white backgrounds against dark theme.
**Why it happens:** Flag-icons CSS uses background-image on a span with specific dimensions. The SVGs are self-contained with their own backgrounds.
**How to avoid:** Flag SVGs have the actual flag colors baked in -- they don't inherit from the page theme. They look correct on both light and dark backgrounds. No custom dark mode handling needed.
**Warning signs:** Flags appear with unwanted white border or background.

## Code Examples

### Flag-Icons Usage (verified from official docs)

```html
<!-- Source: https://flagicons.lipis.dev/ -->
<!-- 4:3 ratio flag (default) -->
<span class="fi fi-de"></span>

<!-- 1:1 square flag -->
<span class="fi fi-de fis"></span>
```

PeeringDB stores country codes as ISO 3166-1 alpha-2 uppercase (e.g., "DE", "US", "GB"). Flag-icons requires lowercase (e.g., "fi-de"). Use `strings.ToLower()` in Go.

### Tailwind Responsive Table Cell Hiding

```html
<!-- Hidden on mobile, visible as table-cell on md+ -->
<th class="px-3 py-1.5 hidden md:table-cell">Speed</th>
<td class="px-3 py-1.5 hidden md:table-cell">10G</td>
```

### data-sort-value Pattern

```html
<!-- Numeric sort: store raw value, display formatted -->
<td data-sort-value="10000" class="...">10G</td>

<!-- String sort: store sortable text -->
<td data-sort-value="cloudflare" class="...">
    <a href="/ui/asn/13335">Cloudflare</a>
</td>

<!-- Boolean sort: store 0/1 -->
<td data-sort-value="1" class="...">
    <span class="...">RS</span>
</td>
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Div-based card layout | HTML `<table>` with semantic markup | This phase | Tables are more accessible (screen readers), denser, and sortable |
| Inline city+country (formatFacLocation) | Separate City and Country columns | This phase | Better for sorting, flags, and scanning |
| No country flags | flag-icons CSS for visual country identification | This phase | Instant visual scanning of geographic data |

**Not deprecated:**
- `formatFacLocation()` in detail_shared.templ is still used by `DetailHeader` (IX, Fac, Campus pages) for the subtitle. Do NOT remove it.
- `speedColorClass()` and `formatSpeed()` are reused inside `<td>` cells.
- `CopyableIP` is reused inside `<td>` cells with empty label parameter.
- `CollapsibleSection` and `CollapsibleSectionWithBandwidth` are unchanged -- tables render inside them.

## Open Questions

1. **OrgNetworkRow Country Enrichment**
   - What we know: Network entities have no country field. The only source is Organization.Country, which is the same for all networks under one org.
   - What's unclear: Whether the user wants a redundant country column showing the same value for every row, or if this was an oversight in D-04.
   - Recommendation: Implement OrgNetworkRow as 2-column (Name, ASN) without country. If user insists, add Organization.Country but note it's the same value per row. The planner should flag this for user decision.

2. **Sort JS Placement**
   - What we know: Claude's discretion item. Two options: layout.templ global `<script>` block, or templ `script` function in detail_shared.templ.
   - Recommendation: Place in layout.templ `<script>` block. Reasons: (a) matches existing keyboard nav and htmx error handler patterns, (b) sort JS uses event delegation on `document`, not per-table initialization, (c) templ `script` functions are for per-component interactive behavior (like copyToClipboard). Global sort handler is infrastructure.

3. **Sticky Table Headers**
   - What we know: Claude's discretion. Sticky headers help for long participant lists (DE-CIX has ~1500 participants).
   - Recommendation: Do NOT use sticky headers. The tables render inside a `<details>` element within a scroll container. Sticky positioning inside a `<details>` element with htmx-injected content is fragile across browsers. The density improvement (from py-3 to py-1.5) already means more rows visible at once, reducing the need for sticky headers.

4. **Table Border Treatment**
   - What we know: Claude's discretion. Options: borderless cells, subtle grid lines, or bottom-border-only.
   - Recommendation: Bottom border only on `<tr>` elements (`border-b border-neutral-800/50`). This matches the existing divide-y pattern in the card layout, provides visual row separation for dense data, and avoids heavy grid appearance. Combined with zebra striping for additional visual separation.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) |
| Config file | None (standard Go test) |
| Quick run command | `go test ./internal/web/ -run TestFragment -race -count=1` |
| Full suite command | `go test ./internal/web/... -race -count=1` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| DENS-01 | Fragment responses contain `<table>` and `<th>` elements | unit | `go test ./internal/web/ -run TestFragments_AllTypes -race -count=1` | Existing (update assertions) |
| DENS-02 | Country flag class present in responses with country data | unit | `go test ./internal/web/ -run TestFragments_AllTypes -race -count=1` | Existing (add fi- class assertion) |
| DENS-03 | Hidden columns have `hidden md:table-cell` class | unit | `go test ./internal/web/ -run TestFragments_AllTypes -race -count=1` | Existing (add class assertion) |
| SORT-01 | Tables have `data-sort-type` attributes on sortable `<th>` | unit | `go test ./internal/web/ -run TestFragments_AllTypes -race -count=1` | Existing (add attribute assertion) |
| SORT-02 | Sort indicator CSS present in layout | unit | `go test ./internal/web/ -run TestDetailPages_AllTypes -race -count=1` | Existing (add CSS assertion) |
| SORT-03 | Fragment responses pre-ordered by default sort | unit | `go test ./internal/web/ -run TestFragments_AllTypes -race -count=1` | Existing (verify row order) |
| FLAG-01 | flag-icons CSS link in `<head>` | unit | `go test ./internal/web/ -run TestDetailPages_AllTypes -race -count=1` | Existing (add link assertion) |

### Sampling Rate
- **Per task commit:** `go test ./internal/web/ -run "TestFragment|TestDetailPages" -race -count=1`
- **Per wave merge:** `go test ./internal/web/... -race -count=1`
- **Phase gate:** Full suite green, plus manual visual inspection

### Wave 0 Gaps
None -- existing test infrastructure (detail_test.go) covers all fragment and detail page rendering. Tests need assertion updates for new HTML structure (tables instead of divs), not new test files.

## Project Constraints (from CLAUDE.md)

Directives relevant to this phase:
- **CS-0 (MUST):** Modern Go code guidelines
- **CS-2 (MUST):** No name stutter (row structs in `templates` package -- FacNetworkRow, not TemplatesFacNetworkRow)
- **CS-5 (MUST):** Input structs for >2 args (not applicable -- template functions take single slice arg)
- **T-1 (MUST):** Table-driven tests (existing test pattern)
- **T-2 (MUST):** `-race` in tests (existing pattern)
- **OBS-1 (MUST):** slog for logging (no logging changes needed -- fragment handlers already use slog)
- **MD-1 (SHOULD):** Prefer stdlib; only new dep is flag-icons CSS CDN (no Go deps added)
- **API-1 (MUST):** Document exported items (new/modified templ components need godoc)

## Sources

### Primary (HIGH confidence)
- Project codebase: `internal/web/templates/detailtypes.go` -- all row struct definitions
- Project codebase: `internal/web/detail.go` -- all 16 fragment handler queries
- Project codebase: `internal/web/templates/detail_*.templ` -- all current list template implementations
- Project codebase: `internal/web/templates/layout.templ` -- existing JS/CSS patterns, `<head>` structure
- Project codebase: `ent/schema/networkfacility.go` -- confirmed City/Country as stored computed fields
- Project codebase: `ent/schema/carrierfacility.go` -- confirmed no City/Country fields
- Project codebase: `internal/web/detail_test.go` -- existing test patterns

### Secondary (MEDIUM confidence)
- [flag-icons on jsdelivr](https://www.jsdelivr.com/package/npm/flag-icons) -- v7.5.0 confirmed as latest, CDN URL verified
- [flag-icons GitHub](https://github.com/lipis/flag-icons) -- usage pattern: `fi fi-{cc}` class, 4:3 default, `fis` for square
- [flag-icons docs](https://flagicons.lipis.dev/) -- ISO 3166-1 alpha-2 mapping confirmed

### Tertiary (LOW confidence)
None -- all findings verified from project code and official flag-icons documentation.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- only new dependency is a well-known CSS CDN; all Go changes use existing patterns
- Architecture: HIGH -- every template and handler fully read; conversion path is mechanical
- Pitfalls: HIGH -- based on actual codebase patterns (htmx once trigger, Tailwind browser CDN, CopyableIP group hover)
- Data enrichment: HIGH for FacNetworkRow (data already in DB), MEDIUM for OrgNetworkRow (design question about redundancy)

**Research date:** 2026-03-26
**Valid until:** 2026-04-26 (stable -- no external API or library version changes expected)
