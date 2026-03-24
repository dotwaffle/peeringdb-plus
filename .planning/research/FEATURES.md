# Feature Landscape

**Domain:** Web UI for network interconnection data exploration (PeeringDB mirror)
**Researched:** 2026-03-24
**Milestone context:** v1.4 Web UI -- adding a polished, interactive web interface to an existing Go API project that already serves GraphQL, REST, and PeeringDB-compatible APIs backed by entgo ORM with all 13 PeeringDB object types synced
**Existing:** Full sync of 13 PeeringDB types (org, net, fac, ix, poc, ixlan, ixpfx, netixlan, netfac, ixfac, carrier, carrierfac, campus), entgo ORM with typed queries and edge traversal, `buildSearchPredicate` for case-insensitive LIKE queries across `SearchFields` per type, PeeringDB compat layer with `?q=` search, existing `QueryOptions` struct, SQLite with indexes on name/asn/org_id/status fields

## Table Stakes

Features users expect from any PeeringDB data exploration interface. Missing = product feels like a raw API wrapper, not a usable tool.

| Feature | Why Expected | Complexity | Dependencies | Notes |
|---------|--------------|------------|--------------|-------|
| Search box on homepage | Every PeeringDB interface (peeringdb.com, bgp.tools, bgpview, he.net) puts search front-and-center. Users arrive with a specific ASN, network name, or facility in mind. A landing page without search is useless. | Low | templ, htmx | Single input field, prominent placement. The entire homepage *is* the search box with results area below. |
| As-you-type search results | PeeringDB.com, bgp.tools, and he.net all provide instant results. Users expect sub-second feedback. Without it, the UI feels like a form submission from 2005. | Med | htmx `hx-trigger="input changed delay:300ms"`, server-side search endpoint | Use htmx active search pattern. Server returns HTML fragments. Debounce at 300ms (not 500ms -- data is local SQLite, not remote API). Add `hx-sync="this:replace"` to cancel in-flight requests. |
| Results grouped by type | When searching "equinix", users need to distinguish Equinix the organization from Equinix facilities from Equinix IXPs. Flat ungrouped results are confusing for a multi-type database. PeeringDB groups by exchanges/networks/facilities/organizations. | Med | Search endpoint must query multiple types, templ components per type | Group into 5 user-facing categories: Networks, IXPs, Facilities, Organizations, Campuses. Do NOT show junction types (netixlan, netfac, ixfac, carrierfac) in search results -- they are meaningless without their parent context. Show carriers within Organizations section. |
| ASN direct lookup | Typing "AS13335" or "13335" should immediately resolve to Cloudflare. This is the primary lookup pattern for network engineers. Every tool in this space supports it. PeeringDB, bgp.tools, he.net all treat numeric input as ASN-first. | Low | Detect numeric input pattern, query Network by ASN | Strip "AS"/"as" prefix, query `Network.Where(network.ASN(asn))`. If exactly one match, consider auto-navigating to detail page. If input is numeric, prioritize ASN matches above name matches. |
| Record detail page for each type | Users click a search result and expect to see the full record. PeeringDB shows all fields plus related records. Without detail views, the UI is a search-only dead end. | Med | templ templates for each of the 5 primary types (org, net, ix, fac, campus), entgo edge queries | Each type needs its own template because fields differ significantly. Reuse patterns (header, metadata section, related records) but not templates. |
| Related records on detail pages | A network detail page must show its IXP presences, facility presences, and contacts. A facility must show which networks and IXPs are present. PeeringDB shows these as nested sections. Without them, users must manually cross-reference. | Med-High | entgo edge traversal (already defined: Network->network_ix_lans, Network->network_facilities, Network->pocs, Facility->network_facilities, etc.) | The ent schema already has all edges defined with `entrest.WithEagerLoad(true)`. Query edges via `entClient.Network.Query().Where(...).WithNetworkIxLans().WithNetworkFacilities().WithPocs().WithOrganization()`. |
| Collapsible sections for related records | Network records can have 50+ IXP presences and 20+ facility presences. Showing everything expanded overwhelms users. Collapsible sections let users focus on what matters. This is standard UX for data-dense detail pages. | Low | htmx or HTML `<details>/<summary>` elements | Use native HTML `<details>` with Tailwind styling. No JavaScript needed for basic expand/collapse. Add htmx lazy-loading for sections with many records (fetch on first expand, not on page load). |
| Clean, linkable URLs | Every page must have a URL that can be bookmarked or shared. "/net/13335" for Cloudflare by ASN, "/ix/26" for AMS-IX by PeeringDB ID, "/search?q=equinix" for search results. URL is the state -- refreshing the page reproduces it exactly. | Low | Go `http.ServeMux` route patterns, URL path design | Route design: `/net/{asn}`, `/ix/{id}`, `/fac/{id}`, `/org/{id}`, `/campus/{id}`, `/search?q=...`, `/compare?asn1=...&asn2=...`. Use `replaceState` for search refinement (no back-button entry per keystroke), `pushState` for navigation to detail pages. |
| Mobile-responsive layout | PeeringDB reports 20% of visitors use phones. Network engineers check peering info from conference floors, NOC rooms, and on-call situations. A layout that breaks on mobile is a dealbreaker. | Med | Tailwind CSS responsive utilities | Tailwind's mobile-first breakpoints (`sm:`, `md:`, `lg:`). Single-column layout by default, expand to multi-column on larger screens. Tables must scroll horizontally on small screens. |
| Visual type indicators | Users need to instantly distinguish a Network result from a Facility result from an IXP result. Text labels alone are insufficient at scan speed. | Low | Tailwind CSS, templ components | Use colored badges/pills per type: Networks (blue), IXPs (green), Facilities (orange), Organizations (gray), Campuses (purple). Consistent throughout search results, breadcrumbs, and cross-references. |

## Differentiators

Features that set PeeringDB Plus apart from peeringdb.com and other tools. Not strictly expected, but make users choose this over alternatives.

| Feature | Value Proposition | Complexity | Dependencies | Notes |
|---------|-------------------|------------|--------------|-------|
| ASN comparison tool | PeeringDB added this in 2025 but it is clunky (tab-based, separate from main navigation). PeerFinder and peeringmatcher exist as CLI tools but have no web UI. A polished, in-browser comparison tool with instant results is a genuine differentiator. The primary use case: "I want to peer with AS13335, where can we meet?" | High | Comparison endpoint, dedicated templ template, data from `NetworkIxLan` and `NetworkFacility` joins | Query pattern: fetch all `NetworkIxLan` records for both ASNs, group by `ix_id`, identify shared IXPs. Same for `NetworkFacility` grouped by `fac_id`. For campuses, group facilities by `campus_id`. |
| Shared-only default with side-by-side toggle | Show only shared IXPs/facilities by default (what users actually need for peering decisions). Toggle to side-by-side view showing all presences with shared ones highlighted. PeeringDB's tool shows everything, requiring mental filtering. | Med | JavaScript toggle (htmx swap between two templ renders), or CSS-only with Tailwind `hidden` classes | Default "shared only" answers the question "where can we peer?" directly. Side-by-side answers "what does each network look like?" -- useful for broader analysis. |
| Compare from network detail page | "Compare with..." button on every network detail page. Pre-fills one ASN, user types the second. Eliminates the workflow of: copy ASN, navigate to compare page, paste ASN, add second ASN. PeeringDB does NOT have this flow. | Low | Link/button on network detail template, pre-filled compare page | `<a href="/compare?asn1={current_asn}">Compare with...</a>` takes user to compare page with first ASN pre-filled. Second ASN input auto-focuses. |
| Speed and port information in comparison | When showing shared IXPs, include port speed for each network. When two networks are both at DE-CIX Frankfurt, knowing one has 100G and the other has 10G is critical for peering decisions. PeerFinder shows this; PeeringDB's web comparison does not. | Med | `NetworkIxLan.speed` field (already in schema), templ template rendering | Speed is already stored in the `NetworkIxLan` entity. Display as human-readable: "100G", "10G", "1G". Sum multiple ports at same IXP. |
| IP address display in comparison | Show IPv4/IPv6 peering addresses at shared IXPs. This is the data network engineers actually need to configure BGP sessions. PeerFinder shows this; PeeringDB's web comparison shows it in their table. Essential for the comparison to be actionable. | Low | `NetworkIxLan.ipaddr4` and `NetworkIxLan.ipaddr6` fields (already in schema) | Display in a compact table row per shared IXP. Null addresses shown as "-" or omitted. |
| Sub-100ms search latency | Data is in local SQLite on the same machine (edge node). There is no network round-trip to a database server. PeeringDB's search hits a remote PostgreSQL. This project can deliver search results in <50ms consistently. This makes the UI feel "instant" in a way PeeringDB cannot. | Low (arch advantage) | SQLite indexes already exist on name, asn, org_id, status | Measure and display latency in HTML comment or footer badge. The architectural advantage (SQLite on edge) is the product's core value proposition -- the UI should make this viscerally obvious through speed. |
| Keyboard navigation in search results | Power users (network engineers) prefer keyboard. Arrow keys to navigate results, Enter to select. Tab between type groups. None of the competing tools do this well. | Med | JavaScript event handlers, ARIA attributes for accessibility | Use `role="listbox"` and `role="option"` on search results. Track active descendant. Handle ArrowUp/ArrowDown/Enter/Escape. This also enables screen reader accessibility for free. |
| Smooth transitions and animations | Tailwind CSS transitions on expand/collapse, search result appearance, page navigation. Makes the UI feel polished and modern vs. PeeringDB's utilitarian interface. | Low | Tailwind `transition-all`, `duration-200`, `ease-in-out` classes | Use `htmx:afterSwap` event for entrance animations. CSS transitions on `max-height` for collapsible sections. Keep animations subtle (200ms) -- this is a data tool, not a marketing site. |
| Dark mode | PeeringDB is adding dark mode in their 2025 UI refresh. Supporting it from day one shows polish. Network engineers often work in NOCs with dim lighting. | Low-Med | Tailwind `dark:` variant, `prefers-color-scheme` media query | Use Tailwind's dark mode with `class` strategy. Store preference in localStorage. Default to system preference. Toggle button in header. |
| Search result count badges | Show "3 Networks, 5 Facilities, 2 IXPs" as summary badges above grouped results. Gives users immediate understanding of result scope without scrolling. | Low | Server counts per type, templ badge component | Return counts alongside HTML fragments. Display as pill badges: "Networks (3)" etc. |
| Sortable and filterable comparison tables | PeeringDB added sorting and dynamic filtering to their comparison tool based on user feedback. Including this from the start (sort by IXP name, country, speed; filter by country) shows maturity. | Med | JavaScript table sorting or htmx-driven re-sort via query params | For comparison results: sort by IXP/facility name, country, speed. Filter by country. Use query params (`/compare?asn1=...&asn2=...&sort=name&country=US`) to keep URLs shareable. |

## Anti-Features

Features to explicitly NOT build. Including them would add complexity, maintenance burden, or scope creep without proportionate value for a read-only PeeringDB mirror.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| User accounts and authentication | The product is a read-only public mirror. Authentication adds session management, password handling, security surface area, and GDPR obligations. PeeringDB requires login for advanced search -- being login-free is an advantage. | Keep everything public. No login, no sessions, no cookies (except dark mode preference in localStorage). |
| Advanced search with multi-field filters | PeeringDB's advanced search has 15+ filter fields per type. Building this is high complexity for a v1 UI. The search box with as-you-type results covers 80% of use cases. | Simple text search only. Users needing complex queries can use the GraphQL playground or REST API, both already available. |
| Map visualization | PeeringDB offers KMZ downloads and map views for facilities. Maps require a mapping library (Leaflet/Mapbox), geocoding validation, and significant frontend complexity. | Link to facility coordinates as a Google Maps link on detail pages. Defer map views to a future milestone if there is user demand. |
| Data export (JSON/CSV/KMZ) | PeeringDB offers export from search results. This is a power-user feature that duplicates what the REST and GraphQL APIs already provide. Building export UIs is low ROI. | Document that export is available via `/api/`, `/rest/v1/`, and `/graphql` endpoints. Add "API link" buttons on detail pages that link to the corresponding API endpoint for the displayed record. |
| Compare more than 2 ASNs | PeeringDB supports up to 10 ASNs. Multi-ASN comparison creates combinatorial UI complexity (which pairs share which IXPs?). Two-ASN comparison covers the dominant use case: "can I peer with this specific network?" | Support exactly 2 ASNs. The UI is simpler, the results are clearer, and the URL is clean (`/compare?asn1=X&asn2=Y`). |
| Real-time search suggestions / autocomplete dropdown | A dropdown overlay with suggestions is a different interaction pattern from inline results. It requires z-index management, focus trapping, keyboard navigation of the dropdown vs. the page, and mobile touch handling. Inline results below the search box achieve the same goal with simpler implementation. | Show results inline below the search box, not as a floating dropdown. This is the htmx active search pattern and works better on mobile. |
| Client-side JavaScript framework (React, Vue, Svelte) | The stack uses templ + htmx for a reason: server-rendered HTML with minimal JavaScript. Adding a JS framework creates a second build toolchain, doubles the maintenance surface, and contradicts the project's server-first architecture. | All interactivity via htmx attributes and minimal vanilla JS (keyboard navigation, dark mode toggle). No bundler, no node_modules. |
| Full-text search / search engine (FTS5, Bleve, etc.) | SQLite LIKE queries with `ContainsFold` are sufficient for the data volume (~30K networks, ~1K IXPs, ~5K facilities). FTS5 would add indexing complexity and schema migration concerns for marginal improvement on a dataset this small. | Use existing `buildSearchPredicate` with `ContainsFold` (case-insensitive LIKE). Query is already fast on SQLite with btree indexes. If performance becomes an issue, add FTS5 later -- it is a SQLite-native feature, not an external dependency. |
| Pagination of search results | With debounced as-you-type search, users refine their query until results are manageable. Showing the top 10-20 results per type is sufficient. Pagination adds URL state complexity, "load more" UI, and edge cases (results changing between pages). | Limit results to top 15 per type. If more exist, show "and N more -- refine your search". This encourages better queries rather than browsing through pages. |
| Custom CSS framework or design system | Building a design system is a project in itself. Tailwind CSS provides utility classes that are sufficient for a data exploration UI. Component libraries (shadcn, daisyUI) add abstraction layers that interfere with templ's server-rendered approach. | Use Tailwind CSS utility classes directly in templ components. No component library. Establish a small set of conventions (color palette, spacing scale, typography) and apply them consistently. |
| Breadcrumb navigation | The app has at most 3 levels: Home -> Search/Compare -> Detail. Breadcrumbs add visual noise for minimal navigational value at this depth. The browser's back button and the persistent search bar serve the same purpose. | Include the persistent search bar in the header on all pages. Back button works because URLs are clean and pushState is used correctly. |
| Contact information (POC) in search results | POC records contain email addresses and phone numbers. Displaying these in search results raises privacy expectations and creates visual clutter. POC data belongs on the network detail page, gated by a "show contacts" expansion. | Show POCs only on network detail pages, inside a collapsible "Contacts" section. |
| Offline/PWA support | Edge-deployed SQLite gives fast responses globally. Offline support requires service workers, cache invalidation strategy, and sync conflict handling. Massive complexity for a mirror that should always be online. | Rely on edge deployment for low latency. If the service is down, the data is stale anyway. |

## Feature Dependencies

```
Tailwind CSS setup (CDN or bundled CSS)
    |
    v
templ component library (base layout, header, footer, type badges)
    |
    +--> Homepage with search box
    |        |
    |        +--> Search endpoint (server-side, returns HTML fragments)
    |        |        |
    |        |        +--> Type-grouped results rendering
    |        |        |        |
    |        |        |        +--> ASN direct lookup (numeric input detection)
    |        |        |        |
    |        |        |        +--> Result count badges per type
    |        |        |
    |        |        +--> htmx wiring (debounce, target swap, indicator)
    |        |
    |        +--> Keyboard navigation (post-search, enhancement layer)
    |
    +--> Record detail pages
    |        |
    |        +--> Network detail (fields + IXP presences + facility presences + contacts)
    |        |        |
    |        |        +--> "Compare with..." button
    |        |
    |        +--> IXP detail (fields + member networks + facilities + LANs)
    |        |
    |        +--> Facility detail (fields + networks + IXPs + carriers)
    |        |
    |        +--> Organization detail (fields + networks + IXPs + facilities + campuses)
    |        |
    |        +--> Campus detail (fields + facilities)
    |        |
    |        +--> Collapsible sections with lazy-load (htmx hx-get on expand)
    |
    +--> Comparison tool
    |        |
    |        +--> /compare page with dual ASN input
    |        |        |
    |        |        +--> ASN search/validation (reuse search endpoint)
    |        |        |
    |        |        +--> Shared IXPs query + display (with speed, IPs)
    |        |        |
    |        |        +--> Shared facilities query + display
    |        |        |
    |        |        +--> Shared campuses query + display
    |        |        |
    |        |        +--> View toggle: shared-only / side-by-side
    |        |
    |        +--> Sort and filter on comparison results
    |
    +--> Dark mode toggle
    |
    +--> Mobile-responsive layout (throughout)
```

## MVP Recommendation

**Phase ordering rationale:** Build the layout and search first because the homepage IS the search experience. Detail pages come second because search results link to them. Comparison comes last because it depends on network detail pages being done (the "Compare with..." button) and reuses search components (ASN input).

Prioritize:
1. **templ + Tailwind foundation** -- Base layout (header with search, main content, footer), Tailwind CSS integration (CDN for development, decide on build strategy later), dark mode support via Tailwind `dark:` classes. This is the skeleton everything mounts onto.
2. **Live search with grouped results** -- Search endpoint querying networks, IXPs, facilities, organizations, and campuses. htmx active search pattern with 300ms debounce. Type-grouped results with colored badges. ASN direct lookup for numeric input. This is the core interaction of the entire UI.
3. **Network detail page** -- Most-visited detail type. Show all fields in organized sections, IXP presences (from `network_ix_lans` with speed and IPs), facility presences (from `network_facilities`), contacts (from `pocs`), organization link. Collapsible `<details>` sections. "Compare with..." button.
4. **Remaining detail pages** -- IXP, Facility, Organization, Campus. Similar structure to network detail but different fields and edges. Reuse templ patterns established in network detail.
5. **ASN comparison tool** -- `/compare` page with dual ASN input, shared IXP/facility/campus results. Shared-only default view. Side-by-side toggle. Speed and IP display for shared IXPs. Sortable results.
6. **Polish pass** -- Keyboard navigation for search results, smooth transitions, result count badges, mobile layout verification, loading indicators, empty state messages.

Defer:
- **Sortable/filterable comparison tables**: Add after basic comparison works. Can use query params for server-side sort.
- **Dark mode**: Low effort but can be added at any point since Tailwind `dark:` classes are additive.
- **Keyboard navigation**: Enhancement that can be layered on after core search works.

## Interaction Pattern Details

### Search: What separates good from great

**Good:** Text box, results appear after submit, grouped by type.

**Great (target):**
- Results appear as you type (300ms debounce, `hx-trigger="input changed delay:300ms, keyup[key=='Enter']"`)
- In-flight requests are cancelled when new input arrives (`hx-sync="this:replace"`)
- Loading indicator appears during request (`hx-indicator`)
- Numeric input triggers ASN-first search with network name as fallback
- Empty results state says "No results for 'xyz'" not a blank page
- Results link to detail pages with the search term highlighted in the matching field
- Minimum 2 characters before search fires (`hx-trigger` with `htmx.config.minInputLength` or check server-side)
- Browser back button returns to search results with the same query (URL state via `?q=`)

### Detail Pages: What separates good from great

**Good:** All fields displayed, related records listed.

**Great (target):**
- Fields organized into logical sections (Identity, Peering Policy, Contact, Metadata) not a flat list
- Related records in collapsible sections, lazy-loaded on first expand to keep initial page load fast
- Counts shown in section headers before expanding ("IXP Presences (12)")
- Cross-links between related records (click IXP name from network detail to go to IXP detail)
- Computed summary stats at top (for networks: "Present at 12 IXPs across 8 countries, 15 facilities")
- External links rendered as links (website, looking glass, policy URL, status dashboard)
- Logo displayed if available
- "Last updated" timestamp showing data freshness
- "View in PeeringDB" external link for reference
- "API" link showing the raw API response for the record

### Comparison: What separates good from great

**Good:** Enter two ASNs, see shared IXPs and facilities.

**Great (target):**
- ASN input with search-as-you-type validation (reuse search component, filter to networks only)
- Pre-fill from network detail page ("Compare with..." button)
- Shared-only default view answers "where can we peer?" immediately
- Side-by-side toggle for comprehensive analysis
- Each shared IXP row shows: IXP name, country, both networks' speeds, both networks' IPv4/IPv6 addresses, whether each is a route-server peer
- Each shared facility row shows: facility name, city, country
- Shared campuses section groups shared facilities by campus
- Summary stats: "Share 8 IXPs, 3 facilities, 2 campuses"
- Sortable by name, country, speed
- URL captures both ASNs for sharing (`/compare?asn1=13335&asn2=15169`)
- Swap button to reverse ASN1/ASN2 (aesthetic, but helpful for URL sharing consistency)

## Sources

- [PeeringDB Advanced Search](https://www.peeringdb.com/advanced_search) -- PeeringDB's search interface, tabbed navigation, 6 entity types
- [PeeringDB ASN Comparison](https://docs.peeringdb.com/blog/asn_comparison/) -- Initial comparison feature announcement
- [PeeringDB More ASN Comparisons](https://docs.peeringdb.com/blog/more_asn_comparisons/) -- Expanded comparison with IXP/facility support, sorting, filtering
- [PeeringDB Search HOWTO](https://docs.peeringdb.com/howto/search/) -- PeeringDB search features, partial name matching, place normalization
- [April 2025 PeeringDB Product Update](https://docs.peeringdb.com/blog/april_2025_product_update/) -- v2 search improvements, new web UI rollout, dark mode
- [PeerFinder](https://github.com/rucarrol/PeerFinder) -- CLI tool for finding common IXPs between ASNs, shows IP addresses and speeds
- [htmx Active Search Example](https://htmx.org/examples/active-search/) -- Official htmx pattern for search-as-you-type
- [htmx hx-trigger Attribute](https://htmx.org/attributes/hx-trigger/) -- Debounce with `delay:`, `changed` modifier
- [htmx hx-sync Attribute](https://htmx.org/attributes/hx-sync/) -- Cancel in-flight requests with `replace` strategy
- [Hypermedia Systems - htmx Patterns](https://hypermedia.systems/htmx-patterns/) -- Active search, progressive enhancement patterns
- [templ htmx Guide](https://templ.guide/server-side-rendering/htmx/) -- templ + htmx integration patterns
- [Search UX Best Practices 2026](https://www.designmonks.co/blog/search-ux-best-practices) -- Grouped results, instant search, keyword highlighting
- [Data Table UX Patterns](https://www.pencilandpaper.io/articles/ux-pattern-analysis-enterprise-data-tables) -- Collapsible sections, expandable rows, lazy loading
- [URL as State](https://alfy.blog/2025/10/31/your-url-is-your-state.html) -- pushState vs replaceState, what belongs in URLs
- [Cloudscape Expandable Rows](https://cloudscape.design/patterns/resource-management/view/table-with-expandable-rows/) -- Expandable rows for nested/related records
- [BGP.Tools](https://bgp.tools/) -- Competitor UI reference for network data exploration
- [BGPView](https://bgpview.io/) -- Competitor reference (shutting down Nov 2025, recommends bgp.tools)
- [Hurricane Electric BGP Toolkit](https://bgp.he.net/) -- Competitor UI reference for ASN/network lookup
