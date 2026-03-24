# Phase 14: Live Search — Discussion Context

**Gathered:** 2026-03-24

## Decisions

### Search UX
- **Homepage IS search**: `/ui/` is both the landing page and the search page. `/ui/?q=cloudflare` shows results inline. No separate search page.
- **Live as-you-type**: htmx `hx-trigger="keyup changed delay:300ms"` with `hx-sync="this:replace"` for request cancellation. Results update without page reload.
- **Minimum query length**: 2 characters before firing search (prevents single-character query storms).

### Search Scope
- **6 searchable types**: Networks, IXPs, Facilities, Organizations, Campuses, Carriers. The 7 junction types (netixlan, netfac, ixfac, ixlan, ixpfx, poc, carrierfac) only appear on parent detail pages.
- **10 results per type**: Max 10 results shown per type group. Encourages query refinement for broad searches.
- **Result count badges**: Each type group header shows total match count (e.g., "Networks (47)"), even if only showing 10.

### ASN Direct Lookup
- **Numeric input → direct redirect**: If the user types a number and presses Enter, redirect to `/ui/asn/{number}`. Don't just show it as a search result.
- While typing, numeric queries still show search results (in case the number matches facility IDs, etc.).

### Search Backend
- Reuse `buildSearchPredicate` pattern from `internal/pdbcompat/search.go` — `sql.ContainsFold()` for case-insensitive LIKE queries.
- Use `errgroup` fan-out across 6 types for parallel queries.
- Each type query limited to 10 + count query for total.
- If total search latency exceeds 50ms, consider FTS5 (benchmark during implementation).

## Visual Design
- Results grouped by type with colored badges (different accent color per type)
- Each result shows: name, key identifier (ASN for networks, city/country for facilities), and type badge
- Clicking a result navigates to its detail page
