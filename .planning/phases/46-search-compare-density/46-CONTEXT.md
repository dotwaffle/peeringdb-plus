# Phase 46: Search & Compare Density - Context

**Gathered:** 2026-03-26
**Status:** Ready for planning

<domain>
## Phase Boundary

Dense layouts for search results and ASN comparison with country flags. Search results get compact single-line rows with inline metadata (flag, country, city, ASN). Comparison sections (IXPs, Facilities, Campuses) become sortable tables consistent with Phase 43 style. Completes the information density overhaul for v1.11.

</domain>

<decisions>
## Implementation Decisions

### Search Result Layout
- **D-01:** Remove per-result card borders and backgrounds. Use divider lines between results with compact padding (py-2). Denser than current py-3 bordered cards.
- **D-02:** Metadata (flag+country, city, ASN) displayed right-aligned inline on the same line as the entity name. Single-line per result.
- **D-03:** Remove per-row type slug badge ('net', 'ix', 'fac'). The group header already shows the type. Saves horizontal space for metadata.
- **D-04:** Keyboard navigation (arrow keys, Enter) preserved with updated styling for compact rows. Same aria-selected / ring-2 highlight pattern.

### Search Data Enrichment
- **D-05:** Add Country (string), City (string), and ASN (int) fields to SearchResult struct. Covers all entity types.
- **D-06:** Remove Subtitle field from SearchResult. Country, City, ASN replace it as properly decomposed fields.
- **D-07:** Enrich existing search queries to include country/city/ASN selects. Minimal overhead — indexed columns already loaded by ent.

### Flag in Search Results
- **D-08:** Small flag icon + 2-letter country code as part of the right-aligned metadata badges. Same flag-icons CSS from Phase 43.
- **D-09:** Entities without country data show empty space where the flag would be. Consistent with Phase 43 missing-country behavior.

### Search Responsive Behavior
- **D-10:** On narrow screens: hide city, keep flag+country code and ASN visible. Same aggressive approach as Phase 43 detail tables.

### Compare Section Tables
- **D-11:** All three comparison sections (IXPs, Facilities, Campuses) converted to sortable tables consistent with Phase 43 style (zebra striping, subtle header, compact padding, text-sm).
- **D-12:** Comparison tables are sortable using the same vanilla JS sorting from Phase 43.
- **D-13:** Non-shared rows in Full View mode keep opacity-40 dimming. Clear visual hierarchy between shared and unique presences.

### Compare IXP Table Columns
- **D-14:** Flat columns: IX Name | Speed A | Speed B | IPv4 A | IPv4 B | IPv6 A | IPv6 B | RS A | RS B. Full data for both networks. IPv4/IPv6/RS columns hide on mobile (responsive).

### Compare Facility Table Columns
- **D-15:** Columns: Name | Flag+Country | City | ASN A | ASN B. Full facility identity with location flag and per-network local ASN.

### Compare Campus Table
- **D-16:** Simple table: Campus Name | Shared Facilities (count). Click campus name to navigate. Compact tabular form replacing nested layout.

### Claude's Discretion
- Search result divider styling (divide-y or border-b on each row)
- Exact metadata badge spacing and ordering (flag first? ASN first?)
- How search result hover works without card borders (background highlight vs text color)
- Campus "shared facilities count" display style (plain number vs badge)
- Whether IXP comparison table shows "---" or empty cell for absent network data

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Search Templates & Types
- `internal/web/templates/search_results.templ` — Current search result rendering (card layout to convert)
- `internal/web/templates/searchtypes.go` — SearchResult/SearchGroup structs (need Country/City/ASN, remove Subtitle)
- `internal/web/search.go` — Search handler and query builder (needs field enrichment)

### Compare Templates & Types
- `internal/web/templates/compare.templ` — Current comparison sections (div layouts to convert to tables)
- `internal/web/templates/comparetypes.go` — CompareIXP, CompareFacility, CompareCampus structs
- `internal/web/compare.go` — Comparison handler

### Shared Components
- `internal/web/templates/detail_shared.templ` — Shared table components from Phase 43 (reuse patterns)
- `internal/web/templates/layout.templ` — Sort JS from Phase 43, keyboard nav JS, flag-icons CSS

### Prior Phase Context
- `.planning/phases/43-dense-tables-with-sorting-and-flags/43-CONTEXT.md` — Table styling decisions, sorting mechanism, flag delivery, responsive hiding approach

### Requirements
- `.planning/REQUIREMENTS.md` — DENS-04, DENS-05, FLAG-02

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets (from Phase 43)
- Table styling patterns (zebra striping, subtle header, compact padding, text-sm)
- Vanilla JS sort handler with data-sort-value attributes
- flag-icons CSS via CDN
- Responsive column hiding via Tailwind `hidden md:table-cell`
- `compareRowClasses()` — existing shared/unique opacity distinction

### Established Patterns
- Search results grouped by entity type with accent-colored headers
- Keyboard navigation JS for search results (arrow keys, Enter, Escape)
- Compare handler already loads SharedIXPs/AllIXPs, SharedFacilities/AllFacilities
- htmx-driven search (results fragment returned from server)

### Integration Points
- SearchResult struct enrichment in searchtypes.go
- Search query enrichment in search.go (add country/city/ASN selects)
- search_results.templ rewrite (cards -> compact rows with metadata)
- compare.templ rewrite (div sections -> table sections)
- Sort JS from Phase 43 extended to comparison tables
- Keyboard nav JS updated for new search result DOM structure

</code_context>

<specifics>
## Specific Ideas

- IXP comparison table shows ALL fields for both networks (Speed, IPv4, IPv6, RS for each) — full comparison data
- Subtitle field removed from SearchResult — properly decomposed into Country/City/ASN
- Campus comparison simplified to name + count table (no nested facility lists in table form)

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 46-search-compare-density*
*Context gathered: 2026-03-26*
