# Phase 46: Search & Compare Density - Research

**Researched:** 2026-03-26
**Domain:** templ templates, Go struct enrichment, client-side table sorting, CSS flag-icons
**Confidence:** HIGH

## Summary

Phase 46 transforms two existing UI surfaces -- search results and ASN comparison -- from their current card/div layouts to dense, information-rich presentations with country flags. The work is purely frontend template and backing Go struct changes with no new dependencies, no new routes, and no schema changes.

The search results surface requires: (1) enriching the `SearchResult` and `SearchHit` structs with decomposed Country/City/ASN fields instead of the current `Subtitle` string, (2) updating all 6 `query*` functions to populate those fields, (3) rewriting `search_results.templ` from bordered cards to compact divider-separated rows with inline metadata, and (4) updating the `convertToSearchGroups` bridge function. The comparison surface requires converting three div-based sections (IXPs, Facilities, Campuses) to `<table>` elements matching the Phase 43 style (zebra striping, sortable headers, responsive column hiding, flag-icons). All existing data is already loaded by the compare handler -- no new queries are needed.

**Primary recommendation:** Organize as two waves: (1) search data enrichment + template rewrite, (2) compare table conversions. Both are independent template rewrites backed by the same Phase 43 table styling patterns and the same existing sort JS.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Remove per-result card borders and backgrounds. Use divider lines between results with compact padding (py-2). Denser than current py-3 bordered cards.
- **D-02:** Metadata (flag+country, city, ASN) displayed right-aligned inline on the same line as the entity name. Single-line per result.
- **D-03:** Remove per-row type slug badge ('net', 'ix', 'fac'). The group header already shows the type. Saves horizontal space for metadata.
- **D-04:** Keyboard navigation (arrow keys, Enter) preserved with updated styling for compact rows. Same aria-selected / ring-2 highlight pattern.
- **D-05:** Add Country (string), City (string), and ASN (int) fields to SearchResult struct. Covers all entity types.
- **D-06:** Remove Subtitle field from SearchResult. Country, City, ASN replace it as properly decomposed fields.
- **D-07:** Enrich existing search queries to include country/city/ASN selects. Minimal overhead -- indexed columns already loaded by ent.
- **D-08:** Small flag icon + 2-letter country code as part of the right-aligned metadata badges. Same flag-icons CSS from Phase 43.
- **D-09:** Entities without country data show empty space where the flag would be. Consistent with Phase 43 missing-country behavior.
- **D-10:** On narrow screens: hide city, keep flag+country code and ASN visible. Same aggressive approach as Phase 43 detail tables.
- **D-11:** All three comparison sections (IXPs, Facilities, Campuses) converted to sortable tables consistent with Phase 43 style (zebra striping, subtle header, compact padding, text-sm).
- **D-12:** Comparison tables are sortable using the same vanilla JS sorting from Phase 43.
- **D-13:** Non-shared rows in Full View mode keep opacity-40 dimming. Clear visual hierarchy between shared and unique presences.
- **D-14:** Flat columns: IX Name | Speed A | Speed B | IPv4 A | IPv4 B | IPv6 A | IPv6 B | RS A | RS B. Full data for both networks. IPv4/IPv6/RS columns hide on mobile (responsive).
- **D-15:** Columns: Name | Flag+Country | City | ASN A | ASN B. Full facility identity with location flag and per-network local ASN.
- **D-16:** Simple table: Campus Name | Shared Facilities (count). Click campus name to navigate. Compact tabular form replacing nested layout.

### Claude's Discretion
- Search result divider styling (divide-y or border-b on each row)
- Exact metadata badge spacing and ordering (flag first? ASN first?)
- How search result hover works without card borders (background highlight vs text color)
- Campus "shared facilities count" display style (plain number vs badge)
- Whether IXP comparison table shows "---" or empty cell for absent network data

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| DENS-04 | User sees search results in a denser layout with country/city information | SearchResult struct enrichment (D-05, D-06, D-07), template rewrite from cards to compact rows (D-01, D-02, D-03), responsive metadata hiding (D-10) |
| DENS-05 | User sees ASN comparison results (shared IXPs, facilities, campuses) as dense tables | Compare template rewrite from divs to Phase 43-style sortable tables (D-11, D-12, D-14, D-15, D-16), opacity dimming for non-shared rows (D-13) |
| FLAG-02 | User sees country flags in search result entries | Flag-icons CSS already loaded (Phase 43), CountryFlag component reusable (D-08, D-09), search struct enrichment provides Country field |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

Key directives that apply to this phase:

- **CS-5 (MUST):** Input structs for functions with >2 args -- not triggered here (template components take structs already).
- **ERR-1 (MUST):** Wrap errors with `%w` and context -- applies to any search query enrichment changes.
- **T-1 (MUST):** Table-driven tests; deterministic and hermetic -- existing search/compare tests follow this pattern.
- **T-2 (MUST):** Run `-race` in CI; add `t.Cleanup` for teardown.
- **API-1 (MUST):** Document exported items -- SearchResult struct field changes need updated comments.
- **OBS-1 (MUST):** Structured logging with slog -- no new logging needed for template changes.
- **Code generation:** `templ generate ./internal/web/templates/` regenerates templ Go files. Always commit `*_templ.go` alongside `.templ` changes.

## Standard Stack

No new dependencies. All work uses existing project infrastructure.

### Core (Already Present)
| Library | Version | Purpose | Status |
|---------|---------|---------|--------|
| github.com/a-h/templ | v0.3.x | Type-safe HTML templating | Already in project |
| flag-icons | v7.5.0 | SVG country flag CSS | Already loaded in layout.templ via CDN |
| Tailwind CSS | v4 (browser) | Utility CSS | Already loaded in layout.templ via CDN |
| Sort JS | N/A | Client-side table sorting | Already in layout.templ global script |

### No New Dependencies
This phase adds zero new Go modules, zero new CDN links, zero new JS libraries. All patterns are reuse of Phase 43 infrastructure.

## Architecture Patterns

### File Change Map

```
internal/web/templates/
  searchtypes.go         # SearchResult: add Country, City, ASN; remove Subtitle
  search_results.templ   # Rewrite: cards -> compact divider rows with inline metadata
  search_results_templ.go # Regenerated
  comparetypes.go        # No changes needed (structs already have all fields)
  compare.templ          # Rewrite: 3 div sections -> 3 sortable table sections
  compare_templ.go       # Regenerated

internal/web/
  search.go              # SearchHit: add Country, City, ASN; remove Subtitle
                         # 6 query* functions: populate new fields
  handler.go             # convertToSearchGroups: map new fields

internal/web/
  search_test.go         # Update assertions for new fields (Subtitle -> Country/City/ASN)
  compare_test.go        # Possibly add table-rendering assertions
```

### Pattern 1: Search Result Struct Enrichment

**What:** Replace `Subtitle string` with `Country string`, `City string`, `ASN int` on both `SearchHit` (web package) and `SearchResult` (templates package).

**Why:** The current `Subtitle` is a pre-formatted string ("Frankfurt, DE" or "AS13335"). Decomposing into typed fields lets the template render flag icons via `CountryFlag(result.Country)` and format ASN/city/country independently for responsive hiding.

**Field mapping per entity type:**

| Entity | Country | City | ASN |
|--------|---------|------|-----|
| Network | "" (no field) | "" (no field) | n.Asn |
| IXP | ix.Country | ix.City | 0 |
| Facility | fac.Country | fac.City | 0 |
| Organization | org.Country | org.City | 0 |
| Campus | c.Country | c.City | 0 |
| Carrier | "" (no field) | "" (no field) | 0 |

Networks have no country/city on the entity itself. They inherit location from their Organization, but enriching via Organization edge would require an additional join (`WithOrganization()`) in the search query. Per D-07, the search should use "minimal overhead -- indexed columns already loaded by ent." Since Network's direct columns already include ASN but not country/city, the search result for networks will show ASN only (no flag). This is correct per the current data model.

**Current code flow:**
```
SearchService.Search() -> queryNetworks/queryIXPs/etc -> []SearchHit
  -> handler convertToSearchGroups() -> []templates.SearchGroup{Results: []SearchResult}
    -> templates.SearchResults(groups) -> HTML
```

Each `query*` function currently constructs `SearchHit{Name, Subtitle, DetailURL}`. Change to populate `Country`, `City`, `ASN` instead of `Subtitle`.

### Pattern 2: Compact Search Row Template

**What:** Replace the current bordered card `<a>` per result with a simple divider-separated row. Entity name left-aligned, metadata right-aligned on the same line.

**Current structure (to remove):**
```html
<a class="flex items-center justify-between px-4 py-3 rounded-lg border border-neutral-200 ...">
  <div class="flex flex-col min-w-0">
    <span>Name</span>
    <span>Subtitle</span>  <!-- Two lines per result -->
  </div>
  <span>type-slug</span>   <!-- Remove per D-03 -->
</a>
```

**New structure:**
```html
<a class="flex items-center justify-between px-4 py-2 hover:bg-neutral-100 dark:hover:bg-neutral-800/50 transition-colors ..."
   role="option" tabindex="-1" aria-selected="false">
  <span class="text-neutral-900 dark:text-neutral-100 font-medium truncate">Name</span>
  <span class="flex items-center gap-3 shrink-0 ml-3 text-sm text-neutral-500">
    <!-- Flag+Country (always visible) -->
    <!-- City (hidden on mobile: hidden md:inline) -->
    <!-- ASN (always visible when non-zero) -->
  </span>
</a>
```

**Key decisions for discretion areas:**
- Use `divide-y divide-neutral-200/50 dark:divide-neutral-700/50` on the container (consistent with Phase 43 table tbody dividers).
- Metadata ordering: Flag+Country first, City second, ASN last -- geographic context before network identity.
- Hover: `hover:bg-neutral-100 dark:hover:bg-neutral-800/50` (same as Phase 43 table row hover -- background highlight, not text color).

### Pattern 3: Compare Table Conversion

**What:** Convert three `<div>` sections in compare.templ to `<table>` elements matching Phase 43 style.

**IXP table (D-14):**
```
<table class="w-full text-sm text-left sortable">
  <thead>
    <tr class="bg-neutral-50/50 dark:bg-neutral-800/70 text-xs font-semibold ...">
      <th data-sortable data-sort-col="0" data-sort-type="alpha" data-sort-default="asc">IX Name</th>
      <th data-sortable data-sort-col="1" data-sort-type="numeric">Speed A</th>
      <th data-sortable data-sort-col="2" data-sort-type="numeric">Speed B</th>
      <th class="hidden md:table-cell">IPv4 A</th>
      <th class="hidden md:table-cell">IPv4 B</th>
      <th class="hidden md:table-cell">IPv6 A</th>
      <th class="hidden md:table-cell">IPv6 B</th>
      <th class="hidden md:table-cell">RS A</th>
      <th class="hidden md:table-cell">RS B</th>
    </tr>
  </thead>
  <tbody>
    <!-- rows with opacity-40 class on tr for non-shared -->
  </tbody>
</table>
```

**Facility table (D-15):**
5 columns: Name | Flag+Country | City (hidden md:table-cell) | ASN A | ASN B

**Campus table (D-16):**
2 columns: Campus Name | Shared Facilities (count as plain number)

**Reusable Phase 43 patterns:**
- Table wrapper: `<div class="overflow-x-auto">`
- Table class: `class="w-full text-sm text-left sortable"`
- Header row: `class="bg-neutral-50/50 dark:bg-neutral-800/70 text-xs font-semibold text-neutral-500 dark:text-neutral-400 border-b border-neutral-200 dark:border-neutral-700"`
- Body: `class="divide-y divide-neutral-200/50 dark:divide-neutral-700/50"`
- Data row: `class="hover:bg-neutral-100 dark:hover:bg-neutral-800/50 even:bg-neutral-50/50 dark:even:bg-neutral-800/30"`
- Cell: `class="px-3 py-1.5"`
- Responsive hide: `class="hidden md:table-cell"`
- Sort attributes: `data-sortable data-sort-col="N" data-sort-type="alpha|numeric"`
- Default sort: `data-sort-default="asc"`
- Sort value: `data-sort-value="..."` on `<td>`

### Anti-Patterns to Avoid

- **Adding new JS for sort:** The sort JS in layout.templ already handles all `table.sortable` tables dynamically, including those loaded via htmx. Do NOT add duplicate sort handlers.
- **Two-line search results:** D-02 explicitly requires single-line per result. Do not create a flex-col with name on one line and metadata on another.
- **Fetching Organization for Network search:** D-07 says "minimal overhead -- indexed columns already loaded by ent." Networks have ASN directly. Do not add WithOrganization() join to the search query to get country.
- **Using resultBadgeClasses:** D-03 removes the type slug badge. The `resultBadgeClasses()` function becomes dead code after this phase and should be removed.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Table sorting | Custom sort handler for compare tables | Existing global sort JS in layout.templ | The sort handler works on any `table.sortable` with `th[data-sortable]` attributes. Adding the right data attributes is sufficient. |
| Country flags | Custom SVG rendering | `CountryFlag(code)` component from detail_shared.templ | Already renders `<span class="fi fi-{code}">` with the flag-icons CSS. |
| Responsive hiding | Custom media queries or JS | Tailwind `hidden md:table-cell` / `hidden md:inline` | Pure CSS, zero JS, consistent with Phase 43 approach. |
| Opacity dimming | Custom classes for non-shared rows | `compareRowClasses(shared)` function | Already returns `"opacity-40"` for non-shared rows. Apply to `<tr>` class. |

## Common Pitfalls

### Pitfall 1: Keyboard Navigation DOM Mismatch
**What goes wrong:** The keyboard navigation JS in layout.templ queries `#search-results [role="option"]` to find navigable elements. Changing the DOM structure of search results (from `<a>` cards to compact rows) could break the selector if `role="option"` is not preserved on each result `<a>`.
**Why it happens:** D-04 says keyboard nav must be preserved, but the template rewrite changes the class list and structure of each result.
**How to avoid:** Keep `role="option"`, `tabindex="-1"`, `aria-selected="false"` on each result `<a>`. The keyboard JS uses `getAttribute('href')` to navigate, so the `<a>` must keep its `href`. The ring-2 highlight classes (`ring-2 ring-emerald-500 ring-offset-1`) are toggled by JS -- they work on any block element.
**Warning signs:** Arrow keys do not move between search results after the template change.

### Pitfall 2: Sort Activation on htmx-Loaded Compare Content
**What goes wrong:** Compare tables are rendered as full page loads (not htmx fragments). The sort JS in layout.templ runs `applyDefaultSort()` on DOMContentLoaded and on `htmx:afterSwap`. Since compare results are not htmx-loaded, only the DOMContentLoaded handler applies default sort.
**Why it happens:** The compare page renders inline, not via htmx lazy-load. The sort JS already handles this case correctly (runs `applyDefaultSort()` on DOMContentLoaded).
**How to avoid:** Verify that `data-sort-default="asc"` on the default column header triggers sort on page load. The existing JS handles both full-page and htmx-swapped tables.
**Warning signs:** Compare tables appear unsorted despite having `data-sort-default`.

### Pitfall 3: Opacity-40 on Table Rows vs Div Rows
**What goes wrong:** Currently `compareRowClasses()` returns a class string applied to a `<div>`. When converting to `<table>`, the class goes on `<tr>`. CSS `opacity` on `<tr>` works correctly in all browsers -- this is NOT a pitfall. However, the opacity should go on `<tr>` not `<td>`, so the entire row dims uniformly.
**How to avoid:** Apply `compareRowClasses(ix.Shared)` to the `<tr>` class attribute, not to individual `<td>` cells.

### Pitfall 4: SearchHit/SearchResult Struct Sync
**What goes wrong:** There are TWO parallel structs: `web.SearchHit` (in search.go) and `templates.SearchResult` (in searchtypes.go). Both need the same field changes (add Country/City/ASN, remove Subtitle), and the bridge function `convertToSearchGroups` in handler.go must map all fields.
**Why it happens:** The templates package cannot import web (circular import), so the structs are mirrored.
**How to avoid:** Update both structs in the same commit. Update `convertToSearchGroups` to map the new fields. Run tests -- the existing search tests assert on `Subtitle` and will fail until updated.
**Warning signs:** Compile error in `convertToSearchGroups` due to missing field.

### Pitfall 5: Test Assertions Reference Subtitle
**What goes wrong:** Multiple existing tests in `search_test.go` assert `hit.Subtitle == "AS13335"` or `hit.Subtitle == "Frankfurt, DE"`. These will fail when Subtitle is removed.
**How to avoid:** Update every test that asserts on Subtitle to assert on the new Country/City/ASN fields instead.
**Warning signs:** Test failures in TestSearchNetworkDetailURL, TestSearchIXPDetailURL, TestSearchFacilitySubtitle, TestSearchCampusDetailURL, TestSearchCarrierDetailURL.

## Code Examples

### Search Result Struct (new)
```go
// searchtypes.go
type SearchResult struct {
    Name      string
    DetailURL string
    Country   string // ISO 3166-1 alpha-2; empty if not available
    City      string // empty if not available
    ASN       int    // 0 if not applicable (non-network entity)
}
```

### Search Query Enrichment (network example)
```go
// search.go - queryNetworks
hits[i] = SearchHit{
    ID:        n.ID,
    Name:      n.Name,
    ASN:       n.Asn,
    // Country and City not available on Network entity directly
    DetailURL: fmt.Sprintf("/ui/asn/%d", n.Asn),
}
```

### Search Query Enrichment (IXP example)
```go
// search.go - queryIXPs
hits[i] = SearchHit{
    ID:        ix.ID,
    Name:      ix.Name,
    Country:   ix.Country,
    City:      ix.City,
    DetailURL: fmt.Sprintf("/ui/ix/%d", ix.ID),
}
```

### Bridge Function Update
```go
// handler.go - convertToSearchGroups
hits[j] = templates.SearchResult{
    Name:      h.Name,
    DetailURL: h.DetailURL,
    Country:   h.Country,
    City:      h.City,
    ASN:       h.ASN,
}
```

### Compact Search Row Template Pattern
```
// search_results.templ - each result row
<a href={...} role="option" tabindex="-1" aria-selected="false"
   class="flex items-center justify-between px-4 py-2 hover:bg-neutral-100 dark:hover:bg-neutral-800/50 ...">
  <span class="truncate font-medium text-neutral-900 dark:text-neutral-100 ...">
    { result.Name }
  </span>
  <span class="flex items-center gap-3 shrink-0 ml-3 text-sm text-neutral-400 font-mono">
    if result.Country != "" {
      @CountryFlag(result.Country)
    }
    if result.City != "" {
      <span class="hidden md:inline">{ result.City }</span>
    }
    if result.ASN > 0 {
      <span>{ fmt.Sprintf("AS%d", result.ASN) }</span>
    }
  </span>
</a>
```

### IXP Compare Table Row Pattern
```
// compare.templ - IXP table row
<tr class={ compareRowClasses(ix.Shared) + " hover:bg-neutral-100 dark:hover:bg-neutral-800/50 even:bg-neutral-50/50 dark:even:bg-neutral-800/30" }>
  <td class="px-3 py-1.5" data-sort-value={ strings.ToLower(ix.IXName) }>
    <a href={...} class="text-emerald-400 hover:text-emerald-300 font-medium transition-colors">
      { ix.IXName }
    </a>
  </td>
  <td class="px-3 py-1.5 font-mono" data-sort-value={...}>
    // Speed A (or "---" if nil)
  </td>
  // ... remaining columns
</tr>
```

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) |
| Config file | none (stdlib) |
| Quick run command | `TMPDIR=/tmp/claude-1000 go test -race ./internal/web/ -run TestSearch -count=1` |
| Full suite command | `TMPDIR=/tmp/claude-1000 go test -race ./internal/web/... -count=1` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| DENS-04 | Search results return Country/City/ASN fields | unit | `go test -race ./internal/web/ -run TestSearch -count=1` | Exists (search_test.go -- needs field assertion updates) |
| DENS-05 | Compare data structures support table rendering | unit | `go test -race ./internal/web/ -run TestCompare -count=1` | Exists (compare_test.go -- struct assertions already pass) |
| FLAG-02 | Search results include Country field for flag rendering | unit | `go test -race ./internal/web/ -run TestSearch -count=1` | Exists (needs new Country field assertions) |

### Sampling Rate
- **Per task commit:** `TMPDIR=/tmp/claude-1000 go test -race ./internal/web/ -count=1`
- **Per wave merge:** `TMPDIR=/tmp/claude-1000 go test -race ./internal/web/... -count=1`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
None -- existing test infrastructure in `search_test.go` and `compare_test.go` covers all phase requirements. Tests need assertion updates (Subtitle -> Country/City/ASN) but no new test files or framework setup.

## Open Questions

1. **Network country enrichment via Organization join**
   - What we know: Networks have no direct country/city field. Organization has both. D-07 says "minimal overhead."
   - What's unclear: Whether the user considers "no country for networks" acceptable or expects Organization-derived country.
   - Recommendation: Follow D-07 literally -- do NOT add WithOrganization() join. Networks show ASN only (their primary identifier). If the user wants network country later, it can be added in a future phase.

2. **Dead code cleanup -- resultBadgeClasses**
   - What we know: D-03 removes the per-row type slug badge. `resultBadgeClasses()` in search_results.templ becomes unused.
   - Recommendation: Remove the function in the same commit that removes the badge rendering. Clean dead code.

## Sources

### Primary (HIGH confidence)
- Direct codebase inspection: `internal/web/search.go`, `internal/web/handler.go`, `internal/web/templates/searchtypes.go`, `internal/web/templates/search_results.templ` -- current search implementation
- Direct codebase inspection: `internal/web/compare.go`, `internal/web/templates/comparetypes.go`, `internal/web/templates/compare.templ` -- current compare implementation
- Direct codebase inspection: `internal/web/templates/detail_shared.templ` -- CountryFlag component, formatSpeed, speedColorClass
- Direct codebase inspection: `internal/web/templates/layout.templ` -- sort JS, keyboard nav JS, flag-icons CSS link
- Direct codebase inspection: `internal/web/templates/detail_ix.templ`, `internal/web/templates/detail_net.templ` -- Phase 43 table patterns (class strings, sort attributes, responsive hiding)
- Direct codebase inspection: `ent/schema/` -- entity field availability (country/city per type)
- Phase 43 CONTEXT.md: table styling decisions, sorting mechanism, flag delivery, responsive hiding approach

### Secondary (MEDIUM confidence)
- None needed -- all information sourced from codebase

### Tertiary (LOW confidence)
- None

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- no new dependencies, all existing infrastructure
- Architecture: HIGH -- codebase read directly, all structs/templates/handlers inspected
- Pitfalls: HIGH -- identified from concrete code inspection (test assertions, struct mirroring, DOM selectors)

**Research date:** 2026-03-26
**Valid until:** 2026-04-26 (stable -- no external dependencies or version concerns)
