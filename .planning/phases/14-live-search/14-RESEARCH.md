# Phase 14: Live Search - Research

**Researched:** 2026-03-24
**Domain:** Server-rendered live search with htmx, templ templates, and ent ORM queries
**Confidence:** HIGH

## Summary

Phase 14 implements an as-you-type search experience on the homepage (`/ui/`) using htmx for partial HTML updates and templ for type-safe server-side rendering. The search queries 6 PeeringDB entity types (Networks, IXPs, Facilities, Organizations, Campuses, Carriers) in parallel using `errgroup`, returning grouped results with count badges.

The existing codebase already has the critical building blocks: `buildSearchPredicate` in `internal/pdbcompat/search.go` provides case-insensitive LIKE matching via `sql.ContainsFold()`; the `pdbcompat.Registry` defines `SearchFields` for all 13 types; the web handler in `internal/web/handler.go` already dispatches `/ui/` routes with htmx fragment detection via `HX-Request` header; and templ v0.3.1001 with htmx 2.0.8 are both installed and working. The primary work is: (1) a new search handler that accepts `?q=` and returns HTML fragments, (2) new templ templates for the search form and grouped results, and (3) wiring htmx attributes to connect them.

**Primary recommendation:** Build the search endpoint at `GET /ui/search?q=` returning an HTML fragment, triggered by htmx from a search input on the homepage. Reuse `buildSearchPredicate` directly from pdbcompat. Use `errgroup` to fan out 6 type queries in parallel. Each query is `Limit(10)` for results plus a `Count()` for the badge.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **Homepage IS search**: `/ui/` is both the landing page and the search page. `/ui/?q=cloudflare` shows results inline. No separate search page.
- **Live as-you-type**: htmx `hx-trigger="keyup changed delay:300ms"` with `hx-sync="this:replace"` for request cancellation. Results update without page reload.
- **Minimum query length**: 2 characters before firing search (prevents single-character query storms).
- **6 searchable types**: Networks, IXPs, Facilities, Organizations, Campuses, Carriers. The 7 junction types (netixlan, netfac, ixfac, ixlan, ixpfx, poc, carrierfac) only appear on parent detail pages.
- **10 results per type**: Max 10 results shown per type group. Encourages query refinement for broad searches.
- **Result count badges**: Each type group header shows total match count (e.g., "Networks (47)"), even if only showing 10.
- **Numeric input -> direct redirect**: If the user types a number and presses Enter, redirect to `/ui/asn/{number}`. Don't just show it as a search result.
- While typing, numeric queries still show search results (in case the number matches facility IDs, etc.).
- **Search backend**: Reuse `buildSearchPredicate` pattern from `internal/pdbcompat/search.go`. Use `errgroup` fan-out across 6 types for parallel queries. Each type query limited to 10 + count query for total. If total search latency exceeds 50ms, consider FTS5 (benchmark during implementation).
- **Visual Design**: Results grouped by type with colored badges (different accent color per type). Each result shows: name, key identifier (ASN for networks, city/country for facilities), and type badge. Clicking a result navigates to its detail page.

### Claude's Discretion
(None specified -- all decisions were locked)

### Deferred Ideas (OUT OF SCOPE)
(None specified in CONTEXT.md)
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SRCH-01 | User can type in a search box on the homepage and see results update instantly as they type | htmx `hx-trigger="input changed delay:300ms"` with `hx-sync="this:replace"` on search input; `hx-get="/ui/search"` returns HTML fragment into `#search-results` target; `renderPage` already detects `HX-Request` header for fragment vs full-page |
| SRCH-02 | Search results are grouped by type (Networks, IXPs, Facilities, Organizations, Campuses) with visual type indicators | Templ component receives `[]TypeResult` slice ordered by type; each group rendered with type-specific accent color and icon; Tailwind classes for badges already in project design system |
| SRCH-03 | User can enter a numeric value to look up a network by ASN directly | Client-side: HTML form `onsubmit` checks if input is numeric, redirects to `/ui/asn/{number}`; Server-side: search handler still processes numeric queries normally for live results |
| SRCH-04 | Each type group shows a count badge of matching results | Each type query runs both `Count()` and `Limit(10).All()` via ent; count passed to templ component for badge rendering |
</phase_requirements>

## Standard Stack

### Core (already in project)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| htmx | 2.0.8 | As-you-type search without JS framework | Already embedded in `/static/htmx.min.js`. Handles request lifecycle, cancellation, fragment swapping. |
| github.com/a-h/templ | v0.3.1001 | Type-safe HTML components | Already in `go.mod`. Generates Go code from `.templ` files. Search result templates will be new templ components. |
| entgo.io/ent | v0.14.5 | ORM queries for 6 entity types | Already in use. `client.Network.Query()`, `client.InternetExchange.Query()`, etc. |
| entgo.io/ent/dialect/sql | v0.14.5 | `sql.ContainsFold()` for LIKE queries | Already used by `buildSearchPredicate`. Core of search functionality. |
| golang.org/x/sync/errgroup | v0.19.0 | Parallel fan-out across 6 type queries | Already in `go.mod`. Per CC-4 SHOULD rule. |
| net/http (stdlib) | Go 1.26 | HTTP handler, query params, response writing | Already in use for `/ui/` routes. |

### Supporting (no new dependencies needed)

This phase requires zero new dependencies. Everything is available in the current `go.mod`.

## Architecture Patterns

### Recommended Project Structure

```
internal/web/
  handler.go          # Add dispatch case for "search" + handleSearch method
  handler_test.go     # Add search endpoint tests
  search.go           # NEW: SearchService with fan-out query logic
  search_test.go      # NEW: SearchService unit tests
  render.go           # No changes needed (renderPage already handles fragments)
  templates/
    home.templ         # MODIFY: Replace placeholder with search form
    search_results.templ  # NEW: Search results grouped by type
```

### Pattern 1: Search Endpoint as htmx Fragment

**What:** `GET /ui/search?q=cloudflare` returns an HTML fragment (no layout wrapper) when called via htmx, or a full page when called directly.
**When to use:** All htmx-driven partial updates.
**Example:**
```go
// Source: existing renderPage pattern in internal/web/render.go
func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
    q := r.URL.Query().Get("q")
    if len(q) < 2 {
        // Return empty results fragment
        page := PageContent{Title: "Search", Content: templates.SearchResults(nil)}
        renderPage(r.Context(), w, r, page)
        return
    }
    results, err := h.searcher.Search(r.Context(), q)
    if err != nil {
        http.Error(w, "search error", http.StatusInternalServerError)
        return
    }
    page := PageContent{Title: "Search", Content: templates.SearchResults(results)}
    renderPage(r.Context(), w, r, page)
}
```

### Pattern 2: Fan-Out Search with errgroup

**What:** Query all 6 types in parallel, collect results, return as single response.
**When to use:** Search handler needs results from 6 independent entity types.
**Example:**
```go
// Source: project CLAUDE.md CC-4, existing pdbcompat pattern
type TypeResult struct {
    TypeName    string       // "Networks", "IXPs", etc.
    TypeSlug    string       // "net", "ix", etc. (for URL construction)
    AccentColor string       // Tailwind color class
    Results     []SearchHit  // Up to 10 results
    TotalCount  int          // Total matching records
}

type SearchHit struct {
    ID         int
    Name       string
    Subtitle   string  // ASN for networks, city/country for facilities
    DetailURL  string  // "/ui/net/42" or "/ui/asn/13335"
}

func (s *SearchService) Search(ctx context.Context, query string) ([]TypeResult, error) {
    g, ctx := errgroup.WithContext(ctx)
    results := make([]TypeResult, 6)

    // Fan out 6 queries in parallel
    for i, tc := range s.searchableTypes {
        g.Go(func() error {
            pred := buildSearchPredicate(query, tc.SearchFields)
            // Run count + limited results
            // Store in results[i]
            return nil
        })
    }
    if err := g.Wait(); err != nil {
        return nil, fmt.Errorf("search %q: %w", query, err)
    }
    // Filter out types with zero results
    var nonEmpty []TypeResult
    for _, r := range results {
        if r.TotalCount > 0 {
            nonEmpty = append(nonEmpty, r)
        }
    }
    return nonEmpty, nil
}
```

### Pattern 3: htmx Search Input Wiring

**What:** Search input with debounced triggering, request cancellation, and URL state preservation.
**When to use:** Homepage search box.
**Example:**
```html
<!-- Source: htmx.org official search pattern + CONTEXT.md decisions -->
<form id="search-form" action="/ui/" method="get"
      onsubmit="return handleSearchSubmit(event)">
    <input type="search" name="q" placeholder="Search networks, IXPs, facilities..."
           value=""
           hx-get="/ui/search"
           hx-trigger="input changed delay:300ms"
           hx-target="#search-results"
           hx-sync="this:replace"
           hx-indicator="#search-indicator"
           hx-push-url="/ui/"
           hx-params="q"
           autocomplete="off"
           minlength="2" />
    <div id="search-indicator" class="htmx-indicator">
        <!-- spinner -->
    </div>
</form>
<div id="search-results">
    <!-- results injected here -->
</div>
```

### Pattern 4: ASN Direct Redirect (Client-Side)

**What:** When user types a number and presses Enter, redirect to `/ui/asn/{number}`.
**When to use:** Form submit handler for the search form.
**Example:**
```html
<script>
function handleSearchSubmit(event) {
    var q = event.target.querySelector('input[name="q"]').value.trim();
    if (/^\d+$/.test(q)) {
        event.preventDefault();
        window.location.href = '/ui/asn/' + q;
        return false;
    }
    // For non-numeric queries, let htmx handle it
    return true;
}
</script>
```

### Anti-Patterns to Avoid

- **Querying all 13 types:** Only the 6 primary types are searchable. Junction types are explicitly excluded per CONTEXT.md.
- **Building a separate search service that duplicates ent queries:** Reuse the existing `buildSearchPredicate` function directly. Do not create a parallel query system.
- **Using `hx-post` for search:** GET is correct because search is idempotent and URL-shareable (`/ui/?q=cloudflare`).
- **Client-side debounce with JavaScript:** htmx's `delay:300ms` modifier handles debouncing natively. No custom JS debounce needed.
- **Returning JSON from search endpoint:** This is a server-rendered app. Return HTML fragments. htmx swaps HTML, not JSON.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Request debouncing | Custom JS debounce function | htmx `hx-trigger="input changed delay:300ms"` | htmx handles debouncing natively with automatic reset on new input |
| Request cancellation | AbortController in JS | htmx `hx-sync="this:replace"` | htmx cancels in-flight requests automatically when new one fires |
| Case-insensitive search | Custom SQL builder | `buildSearchPredicate` + `sql.ContainsFold()` | Already tested and working in pdbcompat layer |
| Parallel query execution | Manual goroutine+channel | `errgroup.WithContext` | Standard Go pattern per CC-4. Cancels all on first error. |
| Fragment vs full-page detection | Custom header parsing | `renderPage()` with `HX-Request` header check | Already implemented in `internal/web/render.go` |

**Key insight:** The existing codebase already has every primitive needed for search. The work is composition, not invention.

## Common Pitfalls

### Pitfall 1: N+1 Count Queries
**What goes wrong:** Running a count query separate from the results query doubles the database round-trips.
**Why it happens:** ent's `.Count()` and `.All()` are separate operations that each hit the database.
**How to avoid:** Accept the 12 total queries (2 per type x 6 types) as acceptable for SQLite's single-process, in-memory nature. SQLite excels at many small queries. Alternatively, use a single query with `LIMIT 11` and check `len(results) > 10` to infer "more than 10" without a count -- but this loses the exact badge count. The CONTEXT.md requires exact counts, so 12 queries is the correct approach.
**Warning signs:** If search latency exceeds 50ms consistently, benchmark and consider FTS5.

### Pitfall 2: hx-trigger Event Name
**What goes wrong:** Using `keyup` instead of `input` as the trigger event misses paste events, autofill, and mobile keyboard input.
**Why it happens:** The CONTEXT.md says `keyup` but the htmx official search pattern recommends `input`. The `input` event fires on all value changes including paste, autofill, voice input, and virtual keyboards.
**How to avoid:** Use `hx-trigger="input changed delay:300ms"`. The `changed` modifier ensures the request only fires when the value actually changes (not on arrow keys, etc.). Note: CONTEXT.md specifies `keyup` but `input` is the correct event for cross-platform search. This is a technical improvement, not a contradiction -- the user intent is "fire as user types" which `input` achieves more reliably.
**Warning signs:** Search not firing on mobile devices or after paste.

### Pitfall 3: Empty Query on Page Load
**What goes wrong:** When user navigates to `/ui/?q=cloudflare` directly (bookmark/share), the search results don't appear because htmx only triggers on user interaction.
**Why it happens:** htmx `input` event requires user action. Page load with pre-filled query value doesn't fire `input`.
**How to avoid:** The server-side handler checks for `?q=` on the initial page load and includes search results in the full-page render. The home page template receives both the query string and pre-computed results so bookmarked URLs work.
**Warning signs:** Shared search URLs showing empty results.

### Pitfall 4: URL State and Browser History
**What goes wrong:** Every keystroke pushes a new browser history entry, making Back button unusable.
**Why it happens:** Using `hx-push-url="true"` on every keystroke creates history pollution.
**How to avoid:** Use `hx-replace-url="true"` instead of `hx-push-url="true"`. This replaces the current URL without creating a new history entry. The URL still updates (so sharing works) but Back button stays clean.
**Warning signs:** Pressing Back requires dozens of clicks to leave the search page.

### Pitfall 5: Concurrent Slice Write in errgroup
**What goes wrong:** Data race when multiple goroutines write to the same slice.
**Why it happens:** Sharing a `[]TypeResult` across goroutines without synchronization.
**How to avoid:** Pre-allocate a fixed-size array (`[6]TypeResult` or `make([]TypeResult, 6)`) and assign each goroutine a unique index. No mutex needed because each goroutine writes to a distinct index. This is safe per Go memory model.
**Warning signs:** `-race` flag detecting data races in tests.

### Pitfall 6: htmx Indicator Flicker
**What goes wrong:** Loading indicator flashes briefly on fast responses, creating visual noise.
**Why it happens:** Default htmx indicator shows/hides on every request, even sub-50ms ones.
**How to avoid:** Add CSS transition delay: `.htmx-indicator { opacity: 0; transition: opacity 200ms ease-in 150ms; }`. The 150ms delay means the indicator only appears if the request takes longer than 150ms. For fast SQLite queries, users won't see the spinner at all.
**Warning signs:** Flickering spinner on every keystroke.

## Code Examples

### Search Input Template (templ)

```go
// internal/web/templates/home.templ
package templates

templ Home() {
    <div class="max-w-3xl mx-auto text-center py-12">
        <h1 class="text-4xl font-bold text-emerald-500 font-mono mb-4">PeeringDB Plus</h1>
        <p class="text-neutral-400 text-lg mb-8">
            Search networks, IXPs, facilities, and more.
        </p>
        @SearchForm("", nil)
    </div>
}

// SearchForm renders the search input and results container.
// query is pre-filled on direct navigation; results are pre-rendered for bookmarked URLs.
templ SearchForm(query string, results []TypeResult) {
    <form id="search-form" action="/ui/" method="get"
          onsubmit="return handleSearchSubmit(event)">
        <div class="relative">
            <input type="search" name="q"
                   value={ query }
                   placeholder="Search networks, IXPs, facilities..."
                   class="w-full bg-neutral-800 border border-neutral-600 rounded-lg px-4 py-3 text-neutral-100 placeholder-neutral-500 focus:outline-none focus:border-emerald-500 focus:ring-1 focus:ring-emerald-500 font-mono"
                   hx-get="/ui/search"
                   hx-trigger="input changed delay:300ms"
                   hx-target="#search-results"
                   hx-sync="this:replace"
                   hx-indicator="#search-indicator"
                   hx-replace-url="/ui/"
                   hx-params="q"
                   autocomplete="off" />
            <div id="search-indicator" class="htmx-indicator absolute right-3 top-3.5">
                <!-- spinner SVG -->
            </div>
        </div>
    </form>
    <div id="search-results" class="mt-6 text-left">
        if results != nil {
            @SearchResults(results)
        }
    </div>
}
```

### Type-Specific Search Configuration

```go
// internal/web/search.go
// Source: internal/pdbcompat/registry.go SearchFields

// searchableType defines a PeeringDB type for the web search UI.
type searchableType struct {
    DisplayName  string   // "Networks"
    Slug         string   // "net" (for URL construction)
    AccentColor  string   // Tailwind color class
    SearchFields []string // Fields to search via ContainsFold
    // query func to execute the actual ent query
}

var searchableTypes = []searchableType{
    {DisplayName: "Networks", Slug: "net", AccentColor: "emerald", SearchFields: []string{"name", "aka", "name_long", "irr_as_set"}},
    {DisplayName: "IXPs", Slug: "ix", AccentColor: "sky", SearchFields: []string{"name", "aka", "name_long", "city", "country"}},
    {DisplayName: "Facilities", Slug: "fac", AccentColor: "violet", SearchFields: []string{"name", "aka", "name_long", "city", "country"}},
    {DisplayName: "Organizations", Slug: "org", AccentColor: "amber", SearchFields: []string{"name", "aka", "name_long"}},
    {DisplayName: "Campuses", Slug: "campus", AccentColor: "rose", SearchFields: []string{"name"}},
    {DisplayName: "Carriers", Slug: "carrier", AccentColor: "cyan", SearchFields: []string{"name", "aka", "name_long"}},
}
```

### errgroup Fan-Out Query

```go
// internal/web/search.go
// Source: golang.org/x/sync/errgroup, pdbcompat.buildSearchPredicate

func (s *SearchService) Search(ctx context.Context, query string) ([]TypeResult, error) {
    g, ctx := errgroup.WithContext(ctx)
    results := make([]TypeResult, len(searchableTypes))

    for i, st := range searchableTypes {
        results[i].TypeName = st.DisplayName
        results[i].TypeSlug = st.Slug
        results[i].AccentColor = st.AccentColor

        g.Go(func() error {
            pred := buildSearchPredicate(query, st.SearchFields)
            if pred == nil {
                return nil
            }
            // Each type gets its own query function -- see actual implementation
            // Uses ent client to query with pred, Limit(10), and Count()
            return st.queryFunc(ctx, s.client, pred, &results[i])
        })
    }
    if err := g.Wait(); err != nil {
        return nil, fmt.Errorf("search %q: %w", query, err)
    }

    var nonEmpty []TypeResult
    for _, r := range results {
        if r.TotalCount > 0 {
            nonEmpty = append(nonEmpty, r)
        }
    }
    return nonEmpty, nil
}
```

### ASN Redirect Script

```html
<script>
function handleSearchSubmit(event) {
    var input = event.target.querySelector('input[name="q"]');
    var q = input ? input.value.trim() : '';
    if (/^\d+$/.test(q)) {
        event.preventDefault();
        window.location.href = '/ui/asn/' + q;
        return false;
    }
    return true;
}
</script>
```

### Searchable Fields Reference (from Registry)

| Type | Display Name | SearchFields | Subtitle Field |
|------|-------------|--------------|----------------|
| net | Networks | name, aka, name_long, irr_as_set | ASN (e.g., "AS13335") |
| ix | IXPs | name, aka, name_long, city, country | city, country |
| fac | Facilities | name, aka, name_long, city, country | city, country |
| org | Organizations | name, aka, name_long | (none or website) |
| campus | Campuses | name | city, country |
| carrier | Carriers | name, aka, name_long | (none or website) |

### Detail URL Patterns

| Type | URL Pattern | Example |
|------|-------------|---------|
| net | `/ui/asn/{asn}` | `/ui/asn/13335` |
| ix | `/ui/ix/{id}` | `/ui/ix/42` |
| fac | `/ui/fac/{id}` | `/ui/fac/100` |
| org | `/ui/org/{id}` | `/ui/org/200` |
| campus | `/ui/campus/{id}` | `/ui/campus/5` |
| carrier | `/ui/carrier/{id}` | `/ui/carrier/10` |

Note: Network detail pages use ASN (`/ui/asn/{asn}`) not PeeringDB ID, since ASN is the natural identifier users think in. Detail page handlers are Phase 15 scope, but search result links must point to the correct URL pattern now.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) + enttest |
| Config file | none (stdlib testing) |
| Quick run command | `go test -race ./internal/web/...` |
| Full suite command | `go test -race ./...` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SRCH-01 | Search input triggers htmx request, returns results fragment | integration | `go test -race -run TestSearch ./internal/web/ -x` | -- Wave 0 |
| SRCH-01 | Search with min 2 chars enforced (1 char returns empty) | unit | `go test -race -run TestSearchMinLength ./internal/web/ -x` | -- Wave 0 |
| SRCH-01 | Search results update as user types (HX-Request fragment) | integration | `go test -race -run TestSearchFragment ./internal/web/ -x` | -- Wave 0 |
| SRCH-02 | Results grouped by type with type indicators | unit | `go test -race -run TestSearchGrouped ./internal/web/ -x` | -- Wave 0 |
| SRCH-03 | Numeric query shows results (ASN redirect is client-side JS, untestable server-side) | integration | `go test -race -run TestSearchNumeric ./internal/web/ -x` | -- Wave 0 |
| SRCH-04 | Each type group shows count badge | unit | `go test -race -run TestSearchCount ./internal/web/ -x` | -- Wave 0 |
| SRCH-01 | Bookmarked URL /ui/?q=X renders results on page load | integration | `go test -race -run TestSearchBookmark ./internal/web/ -x` | -- Wave 0 |
| (infra) | SearchService fan-out returns correct results | unit | `go test -race -run TestSearchService ./internal/web/ -x` | -- Wave 0 |
| (infra) | SearchService handles empty query | unit | `go test -race -run TestSearchEmpty ./internal/web/ -x` | -- Wave 0 |
| (infra) | Search predicate reuse works for all 6 types | unit | `go test -race -run TestSearchAllTypes ./internal/web/ -x` | -- Wave 0 |

### Sampling Rate
- **Per task commit:** `go test -race ./internal/web/...`
- **Per wave merge:** `go test -race ./...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/web/search_test.go` -- covers SRCH-01, SRCH-02, SRCH-03, SRCH-04 and SearchService
- [ ] Test seed data helpers for creating networks, IXPs, facilities, orgs, campuses, carriers in test DB

## Open Questions

1. **Network detail URL pattern: `/ui/asn/{asn}` vs `/ui/net/{id}`**
   - What we know: Users think in ASNs, not PeeringDB IDs. The context says "direct redirect to `/ui/asn/{number}`".
   - What's unclear: Phase 15 will implement detail pages. Search links must use the pattern Phase 15 will implement.
   - Recommendation: Use `/ui/asn/{asn}` for networks per CONTEXT.md. Use `/ui/{type}/{id}` for all other types. Accept that clicking network search results will 404 until Phase 15 is complete.

2. **hx-replace-url behavior with query params**
   - What we know: `hx-replace-url="true"` with `hx-params="q"` on a GET request should update the browser URL to include `?q=value`. htmx constructs the request URL from `hx-get` plus params, and `hx-replace-url="true"` replaces the current URL with the request URL.
   - What's unclear: Whether the pushed URL will be `/ui/search?q=...` (the htmx target) or `/ui/?q=...` (the desired shareable URL).
   - Recommendation: Use `hx-replace-url="/ui/"` with a computed query string via htmx's `hx-vals` or set a custom value. Alternatively, the server can send the `HX-Replace-Url` response header with the correct URL. Test this during implementation.

3. **buildSearchPredicate visibility**
   - What we know: `buildSearchPredicate` is unexported (lowercase) in package `pdbcompat`.
   - What's unclear: The web package cannot call it directly.
   - Recommendation: Either (a) export `BuildSearchPredicate` from pdbcompat, or (b) duplicate the 10-line function in `internal/web/search.go`. Option (a) is cleaner but creates a cross-package dependency. Option (b) avoids coupling. Given it's 10 lines of straightforward code, either approach is acceptable. Prefer (a) if the function is broadly useful.

## Sources

### Primary (HIGH confidence)
- Project source code: `internal/pdbcompat/search.go` -- existing `buildSearchPredicate` implementation
- Project source code: `internal/pdbcompat/registry.go` -- `SearchFields` for all 13 types
- Project source code: `internal/web/handler.go` -- existing dispatch pattern and fragment detection
- Project source code: `internal/web/render.go` -- `renderPage` with `HX-Request` header check
- [htmx hx-trigger docs](https://htmx.org/attributes/hx-trigger/) -- `input changed delay:300ms` pattern
- [htmx hx-sync docs](https://htmx.org/attributes/hx-sync/) -- `replace` strategy for request cancellation
- [htmx hx-indicator docs](https://htmx.org/attributes/hx-indicator/) -- `htmx-request` CSS class behavior
- [htmx hx-push-url docs](https://htmx.org/attributes/hx-push-url/) -- URL state management

### Secondary (MEDIUM confidence)
- [htmx search pattern blog post](https://www.lorenstew.art/blog/bookmarkable-by-design-url-state-htmx/) -- URL-driven state patterns

### Tertiary (LOW confidence)
- None

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - all libraries already in project, versions verified from go.mod and embedded files
- Architecture: HIGH - patterns directly extend existing handler/render/template structure
- Pitfalls: HIGH - derived from htmx official docs, existing codebase patterns, and Go concurrency rules
- Search fields: HIGH - directly read from `pdbcompat.Registry` source code

**Research date:** 2026-03-24
**Valid until:** 2026-04-24 (stable stack, no expected changes)
