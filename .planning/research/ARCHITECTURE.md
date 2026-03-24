# Architecture Patterns

**Domain:** v1.4 Web UI -- templ/htmx/Tailwind integration with existing Go HTTP server
**Researched:** 2026-03-24
**Focus:** How templ templates, htmx endpoints, and Tailwind CSS integrate with the existing HTTP server architecture; new components needed; data flow from ent to rendered HTML; route organization

## Existing Architecture (Context)

The v1.3 architecture is a single Go binary with this structure:

```
cmd/peeringdb-plus/main.go     (wiring, HTTP server, graceful shutdown)
  |
  +-- internal/config/          Config from env vars (immutable after load)
  +-- internal/database/        SQLite open (modernc.org, WAL, FK)
  +-- internal/otel/            OTel pipeline: Tracer, Meter, Logger providers
  +-- internal/peeringdb/       PeeringDB API client (HTTP, retry, rate limit, OTel tracing)
  +-- internal/sync/            Sync worker (fetch -> filter -> upsert -> delete, per-type metrics)
  +-- internal/health/          /healthz (liveness), /readyz (readiness + sync freshness)
  +-- internal/middleware/       Logging, Recovery, CORS
  +-- internal/litefs/          LiteFS primary detection
  +-- internal/graphql/         GraphQL handler factory (gqlgen server config)
  +-- internal/pdbcompat/       PeeringDB-compatible REST layer (13 types, Django-style filters)
  +-- graph/                    gqlgen resolvers, generated code
  +-- ent/                      entgo ORM (13 schemas), generated code
  +-- ent/schema/               Schema definitions with entgql + entrest annotations
  +-- ent/rest/                 entrest-generated REST handlers
```

**Existing HTTP route map:**

| Method | Path | Handler | Purpose |
|--------|------|---------|---------|
| GET | / | inline func | JSON discovery endpoint |
| GET/POST | /graphql | pdbgql.PlaygroundHandler / gqlHandler | GraphiQL playground / GraphQL API |
| GET | /api/{rest...} | pdbcompat.Handler.dispatch | PeeringDB-compatible REST |
| ANY | /rest/v1/* | entrest.Handler (StripPrefix) | OpenAPI REST (read-only) |
| POST | /sync | inline func | On-demand sync trigger |
| GET | /healthz | health.LivenessHandler | Liveness probe |
| GET | /readyz | health.ReadinessHandler | Readiness probe |

**Existing middleware stack (outermost first):**

```
Recovery -> OTel HTTP -> Logging -> CORS -> Readiness -> mux
```

**Key architectural facts:**
- Uses `http.NewServeMux()` (Go 1.22+ with method-based routing)
- Readiness middleware gates all routes except `/sync`, `/healthz`, `/readyz`, `/`
- CORS middleware applied both globally (via stack) and per-route on `/rest/v1/`
- `*ent.Client` is the single dependency for data access
- `*sql.DB` passed alongside for raw sync_status queries
- All handler factories accept `*ent.Client` and return `http.Handler`

## Recommended Architecture for v1.4

### Component Overview

```
NEW PACKAGES:

internal/web/                  Web UI handler package
  handler.go                   HTTP handlers for search, detail, compare pages
  handler_test.go              Handler tests (httptest)
  search.go                    Search query logic (reuses pdbcompat search patterns)

internal/web/templates/        templ template files (.templ)
  layout.templ                 Base HTML layout (head, body, nav, footer)
  home.templ                   Landing page / search page
  search_results.templ         Search results fragment (htmx partial)
  detail.templ                 Record detail page (all 13 types)
  detail_section.templ         Collapsible related-records sections (htmx partial)
  compare.templ                ASN comparison page
  compare_results.templ        Comparison results fragment (htmx partial)
  components.templ             Reusable UI components (cards, badges, tables, pills)
  nav.templ                    Navigation bar component
  error.templ                  Error pages (404, 500)

internal/web/static/           Static assets
  css/
    input.css                  Tailwind CSS input file (@tailwind directives)
    output.css                 Generated Tailwind CSS (build artifact, git-tracked)
  htmx.min.js                 HTMX library (vendored, single file)

MODIFIED FILES:

cmd/peeringdb-plus/main.go    Mount web routes on mux, serve static assets
internal/config/config.go     (No changes needed -- web UI has no config)
Dockerfile.prod               Add templ generate + tailwind build steps
```

### New vs. Modified Components

| Component | Status | Integration Point |
|-----------|--------|-------------------|
| `internal/web/handler.go` | NEW | Receives `*ent.Client`, registers on `*http.ServeMux` |
| `internal/web/templates/*.templ` | NEW | Compiled to Go by `templ generate`, called from handlers |
| `internal/web/static/` | NEW | Embedded via `//go:embed`, served by `http.FileServer` |
| `cmd/peeringdb-plus/main.go` | MODIFIED | ~10 lines added: create web handler, mount routes, serve static |
| `Dockerfile.prod` | MODIFIED | Add `templ generate` and `tailwindcss` build steps |

### Route Organization

**Principle:** Web UI routes live under root paths that do not conflict with existing API routes. All API routes are prefixed (`/graphql`, `/api/`, `/rest/v1/`, `/sync`, `/healthz`, `/readyz`), leaving clean paths for UI.

**New routes:**

| Method | Path | Handler | Returns | Purpose |
|--------|------|---------|---------|---------|
| GET | / | web.HomeHandler | Full page | Landing page with search box (replaces JSON discovery) |
| GET | /search | web.SearchHandler | Full page or fragment | Search results page |
| GET | /net/{id} | web.DetailHandler | Full page | Network detail |
| GET | /ix/{id} | web.DetailHandler | Full page | IX detail |
| GET | /fac/{id} | web.DetailHandler | Full page | Facility detail |
| GET | /org/{id} | web.DetailHandler | Full page | Organization detail |
| GET | /campus/{id} | web.DetailHandler | Full page | Campus detail |
| GET | /carrier/{id} | web.DetailHandler | Full page | Carrier detail |
| GET | /compare | web.CompareHandler | Full page | ASN comparison page |
| GET | /static/* | http.FileServer | CSS/JS | Static assets |

**htmx fragment endpoints** (return HTML fragments, not full pages):

| Method | Path | Handler | Returns | Purpose |
|--------|------|---------|---------|---------|
| GET | /search?q=... | web.SearchHandler | Fragment (if HX-Request) | Live search results |
| GET | /net/{id}/related/{edge} | web.RelatedHandler | Fragment | Lazy-load related records section |
| GET | /compare/results?asn1=...&asn2=... | web.CompareResultsHandler | Fragment | Comparison results |

**Route registration pattern:**

```go
// In cmd/peeringdb-plus/main.go, after existing route setup:

webHandler := web.NewHandler(entClient)
webHandler.Register(mux)

// Static assets (embedded)
mux.Handle("GET /static/", http.StripPrefix("/static/",
    http.FileServerFS(web.StaticFS)))
```

**Root route migration:** The existing `GET /` handler returns JSON discovery. Move this to `GET /api/` (where it logically belongs) and make `GET /` serve the web UI home page. The JSON discovery endpoint becomes:

```go
mux.HandleFunc("GET /api/", func(w http.ResponseWriter, r *http.Request) {
    if r.URL.Path != "/api/" { /* existing dispatch handles sub-paths */ }
    w.Header().Set("Content-Type", "application/json")
    fmt.Fprint(w, `{"name":"peeringdb-plus",...}`)
})
```

This is compatible because `pdbcompat.Handler.Register()` already registers `GET /api/{rest...}` which matches `/api/` via the empty rest wildcard. The JSON discovery can be integrated into pdbcompat's index handler (it already serves an index at `/api/`).

### Data Flow: Ent Query to Rendered HTML

**Full page request (direct navigation or refresh):**

```
Browser GET /net/13335
    |
    v
readinessMiddleware (pass -- sync completed)
    |
    v
web.DetailHandler
    |
    +-- Parse path: type="net", id=13335
    +-- Query ent: client.Network.Query().Where(network.ID(13335)).
    |       WithOrganization().WithNetworkFacilities().Only(ctx)
    +-- Build view model: DetailViewModel{Type: "net", Record: net, ...}
    +-- Render full page: layout.templ wrapping detail.templ
    |       templates.Layout(templates.Detail(vm)).Render(ctx, w)
    |
    v
Browser receives full HTML page (head, nav, content, footer)
```

**htmx fragment request (live search, lazy-load sections):**

```
Browser HX-Request: true
GET /search?q=cloudflare
    |
    v
web.SearchHandler
    |
    +-- Check r.Header.Get("HX-Request") == "true"
    +-- Parse query: q="cloudflare"
    +-- Query ent: search across net (name, aka, asn), ix, fac, org
    |       (reuse pdbcompat.buildSearchPredicate logic)
    +-- Build results: []SearchResult{Type, ID, Name, ASN, ...}
    +-- Render fragment only: search_results.templ (no layout wrapper)
    |       templates.SearchResults(results).Render(ctx, w)
    |
    v
Browser swaps #search-results div with fragment HTML
```

**Key design decision:** Handlers detect htmx requests via the `HX-Request` header and conditionally render either a full page (with layout) or a bare fragment. This enables both direct URL access (shareable links) and htmx partial updates.

```go
func (h *Handler) renderPage(ctx context.Context, w http.ResponseWriter, r *http.Request,
    page templ.Component) {
    if r.Header.Get("HX-Request") == "true" {
        // htmx request: render fragment only
        page.Render(ctx, w)
        return
    }
    // Direct navigation: wrap in layout
    templates.Layout(page).Render(ctx, w)
}
```

### View Model Layer

**Do NOT pass `*ent.Network` directly to templates.** Create view model structs that contain exactly the data each template needs. This:

1. Decouples templates from ent-generated code (ent regeneration cannot break templates)
2. Makes templates testable with plain struct construction
3. Enables pre-computation (e.g., format ASN as "AS13335", compute "shared IXPs" count)

```go
// internal/web/viewmodel.go

// SearchResult is a unified search result across all PeeringDB types.
type SearchResult struct {
    Type    string // "net", "ix", "fac", "org", etc.
    ID      int
    Name    string
    Detail  string // Type-specific detail line (ASN for net, city for fac, etc.)
    URL     string // Link to detail page
}

// DetailView holds data for a record detail page.
type DetailView struct {
    Type       string
    TypeLabel  string // "Network", "Internet Exchange", etc.
    Record     any    // Type-specific struct (NetworkDetail, FacilityDetail, etc.)
    RelatedEdges []RelatedEdge // Edges available for lazy-loading
}

// CompareView holds data for the ASN comparison page.
type CompareView struct {
    ASN1        int
    ASN2        int
    Network1    *NetworkSummary
    Network2    *NetworkSummary
    SharedIXPs  []SharedIXP
    SharedFacs  []SharedFacility
    OnlyASN1IXPs []IXPSummary
    OnlyASN2IXPs []IXPSummary
    // ... similar for facilities, campuses
}
```

### Template Composition Pattern

**Layout hierarchy:**

```
layout.templ
  +-- <html>, <head> (Tailwind CSS, htmx.js, meta)
  +-- nav.templ (navigation bar with search)
  +-- {children...} <-- page content injected here
  +-- <footer>

home.templ
  +-- Search input with hx-get="/search" hx-trigger="keyup changed delay:300ms"
  +-- #search-results div (target for htmx swap)

detail.templ
  +-- Record header (name, type badge, key fields)
  +-- Field grid (all record fields in organized sections)
  +-- Related records sections (collapsible, lazy-loaded)
      +-- Each section: hx-get="/net/{id}/related/network_facilities"
          hx-trigger="revealed" hx-swap="innerHTML"

compare.templ
  +-- Two ASN input fields
  +-- hx-get="/compare/results" hx-include="[name='asn1'],[name='asn2']"
  +-- #compare-results div (target for htmx swap)
```

**templ component pattern:**

```
templ Layout(contents templ.Component) {
    <!DOCTYPE html>
    <html>
    <head>
        <link rel="stylesheet" href="/static/css/output.css"/>
        <script src="/static/htmx.min.js"></script>
    </head>
    <body class="bg-gray-50 min-h-screen">
        @Nav()
        <main class="container mx-auto px-4 py-8">
            @contents
        </main>
        <footer>...</footer>
    </body>
    </html>
}
```

### Static Asset Strategy

**Tailwind CSS build:** Use the Tailwind CSS v4 standalone CLI binary. No Node.js dependency.

```
# Development: watch mode
./tailwindcss -i internal/web/static/css/input.css \
              -o internal/web/static/css/output.css --watch

# Production: minified
./tailwindcss -i internal/web/static/css/input.css \
              -o internal/web/static/css/output.css --minify
```

The `input.css` file uses `@import "tailwindcss"` (v4 syntax) and a `@source` directive pointing at `.templ` files:

```css
@import "tailwindcss";
@source "../templates/**/*.templ";
```

**htmx:** Vendor `htmx.min.js` (single file, ~14KB gzipped) into `internal/web/static/`. No CDN dependency -- the app should work without external network access.

**Embedding for single-binary deployment:**

```go
// internal/web/static.go
package web

import "embed"

//go:embed static
var StaticFS embed.FS
```

This maintains the existing single-binary deployment model. The compiled Tailwind CSS and vendored htmx.js are embedded at build time. No runtime filesystem access needed for static assets.

**Dockerfile.prod changes:**

```dockerfile
# Build stage
FROM golang:1.26-alpine AS builder
WORKDIR /build

# Install templ
RUN go install github.com/a-h/templ/cmd/templ@latest

# Download Tailwind standalone CLI
RUN wget -O /usr/local/bin/tailwindcss \
    https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-linux-x64 && \
    chmod +x /usr/local/bin/tailwindcss

COPY go.mod go.sum ./
RUN go mod download
COPY . .

# Generate templ code
RUN templ generate

# Build Tailwind CSS
RUN tailwindcss -i internal/web/static/css/input.css \
                -o internal/web/static/css/output.css --minify

# Build Go binary (static assets embedded via //go:embed)
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /peeringdb-plus ./cmd/peeringdb-plus
```

### Search Architecture

**Cross-type search** is the most complex new capability. The approach:

1. **Reuse existing search infrastructure.** `internal/pdbcompat` already has `buildSearchPredicate()` which generates case-insensitive `LIKE` predicates across configurable search fields per type. The `Registry` already defines `SearchFields` for all 13 types.

2. **Query multiple types in parallel.** For a search query "cloudflare", run queries against the 5 most user-relevant types simultaneously: net, ix, fac, org, carrier. Use `errgroup` for fan-out.

3. **Unified results.** Map ent entities to `SearchResult` view models with type-specific detail lines:
   - Network: "AS13335 -- Content"
   - IX: "Amsterdam, NL"
   - Facility: "Equinix NY1, New York, US"
   - Organization: "Cloudflare, Inc."
   - Carrier: "Zayo Group"

4. **Limit per type.** Return at most 5 results per type to keep the response fast and the UI compact. Total max: 25 results.

5. **No separate search index.** SQLite is fast enough for LIKE queries on the existing indexes. The dataset is small (~200K total rows). No need for FTS5 or external search.

```go
func (h *Handler) search(ctx context.Context, query string) ([]SearchResult, error) {
    g, ctx := errgroup.WithContext(ctx)
    var mu sync.Mutex
    var results []SearchResult

    // Search networks
    g.Go(func() error {
        nets, err := h.client.Network.Query().
            Where(func(s *sql.Selector) {
                s.Where(sql.Or(
                    sql.ContainsFold("name", query),
                    sql.ContainsFold("aka", query),
                    sql.ContainsFold("name_long", query),
                ))
            }).
            Limit(5).All(ctx)
        // ... map to SearchResult, append under mu.Lock
    })

    // Similar for ix, fac, org, carrier...

    if err := g.Wait(); err != nil {
        return nil, err
    }
    return results, nil
}
```

### ASN Comparison Architecture

The comparison tool finds shared resources between two ASNs.

**Data flow:**

```
1. User enters ASN1=13335, ASN2=15169
2. htmx GET /compare/results?asn1=13335&asn2=15169
3. Handler:
   a. Query Network by ASN for both (get net IDs)
   b. Query NetworkIxLan WHERE net_id IN (net1.ID) -> get IX IDs for ASN1
   c. Query NetworkIxLan WHERE net_id IN (net2.ID) -> get IX IDs for ASN2
   d. Set intersection: shared IXPs = ix_ids_1 & ix_ids_2
   e. Similar for NetworkFacility -> shared facilities
   f. Build CompareView with shared/unique lists
4. Render compare_results.templ fragment
```

**Optimization:** All queries use existing ent edges and indexes. The NetworkIxLan table has indexes on `net_id` and `ix_id`. The dataset is small enough that in-memory set intersection is fine.

### URL-as-State Pattern

**Every page must be linkable.** This means:

- Search: `/search?q=cloudflare` -- query string is the state
- Detail: `/net/13335` -- path is the state
- Compare: `/compare?asn1=13335&asn2=15169` -- query string is the state

**htmx integration with hx-push-url:**

```html
<!-- Search input pushes URL state -->
<input type="search" name="q"
       hx-get="/search"
       hx-trigger="keyup changed delay:300ms"
       hx-target="#search-results"
       hx-push-url="true"
       hx-swap="innerHTML" />
```

When the user types "cloudflare", the URL updates to `/search?q=cloudflare`. Refreshing or sharing this URL renders the full search results page.

**Handler must support both modes:**

```go
func (h *Handler) SearchHandler(w http.ResponseWriter, r *http.Request) {
    query := r.URL.Query().Get("q")
    results, err := h.search(r.Context(), query)
    // ... error handling

    component := templates.SearchResults(results)

    if r.Header.Get("HX-Request") == "true" {
        // htmx partial update
        component.Render(r.Context(), w)
        return
    }
    // Full page with layout (direct navigation or refresh)
    templates.Layout(templates.SearchPage(query, results)).Render(r.Context(), w)
}
```

### Integration with Existing Middleware Stack

The web UI routes pass through the **same middleware stack** as API routes:

```
Recovery -> OTel HTTP -> Logging -> CORS -> Readiness -> mux
```

This means:

- **OTel tracing:** Every UI page load gets a trace span automatically via otelhttp
- **Logging:** Every request logged with method, path, status, duration, trace_id
- **Readiness gating:** UI routes return 503 until first sync completes (correct behavior -- no data to show yet)
- **CORS:** Not needed for same-origin UI requests, but harmless

**One addition needed:** The readiness middleware bypass list should include `/static/` so that CSS/JS assets load even before sync completes (the 503 page needs styling):

```go
if r.URL.Path == "/sync" || r.URL.Path == "/healthz" ||
   r.URL.Path == "/readyz" || strings.HasPrefix(r.URL.Path, "/static/") {
    next.ServeHTTP(w, r)
    return
}
```

### Error Handling in Web UI

**404 pages:** When a record ID is not found, render a styled 404 page:

```go
result, err := h.client.Network.Get(ctx, id)
if ent.IsNotFound(err) {
    w.WriteHeader(http.StatusNotFound)
    templates.Layout(templates.NotFound("Network", id)).Render(ctx, w)
    return
}
```

**500 pages:** The existing Recovery middleware catches panics. For non-panic errors, render a styled error page instead of raw JSON.

**Sync-not-ready page:** When readiness middleware returns 503, the user sees an unstyled JSON error. Add a web-aware readiness response that renders a "syncing data, please wait" HTML page when the request Accept header prefers text/html.

## Patterns to Follow

### Pattern 1: Handler Registration Convention

**What:** Each handler package exposes `NewHandler(deps)` and `Register(mux)`, matching the existing `pdbcompat.Handler` pattern.

**When:** Always. This is how the codebase already works.

**Example:**

```go
// internal/web/handler.go
type Handler struct {
    client *ent.Client
}

func NewHandler(client *ent.Client) *Handler {
    return &Handler{client: client}
}

func (h *Handler) Register(mux *http.ServeMux) {
    mux.HandleFunc("GET /{$}", h.HomeHandler)
    mux.HandleFunc("GET /search", h.SearchHandler)
    mux.HandleFunc("GET /net/{id}", h.DetailHandler)
    // ...
}
```

### Pattern 2: Full Page vs. Fragment Rendering

**What:** Every handler that serves both full pages and htmx fragments uses a shared `renderPage` helper that checks `HX-Request`.

**When:** Every page handler.

**Why:** Ensures linkable URLs work (full page on direct nav) while htmx gets fast fragments.

### Pattern 3: View Model Separation

**What:** Handlers query ent, map to view model structs, pass view models to templ. Templates never import `ent`.

**When:** Always. Non-negotiable for testability and decoupling.

### Pattern 4: Search with errgroup Fan-out

**What:** Cross-type search queries run in parallel using `errgroup.WithContext`, with per-type limits.

**When:** Search handler.

**Why:** 5 sequential SQLite queries would be ~50ms total. Parallel cuts to ~15ms. Use `sync.Mutex` to collect results (simpler than channels for small fan-out).

### Pattern 5: Lazy-Loading Related Records

**What:** Detail pages show related records in collapsible sections that load on reveal via `hx-trigger="revealed"`.

**When:** Detail pages with relationships (e.g., network -> facilities, network -> IX connections).

**Why:** A network like Cloudflare has 200+ facility connections. Loading all of them on page load would slow initial render. Lazy-load when the user expands the section.

## Anti-Patterns to Avoid

### Anti-Pattern 1: Passing ent Entities to Templates

**What:** Calling `templates.Detail(entNetwork)` directly.

**Why bad:** ent-generated types change on schema regeneration. Template compile errors become entangled with ORM changes. Templates cannot be unit tested without a database.

**Instead:** Map to view model structs. Test templates with hand-constructed view models.

### Anti-Pattern 2: htmx-go Helper Library

**What:** Adding `github.com/angelofallars/htmx-go` for header detection.

**Why bad:** Checking `r.Header.Get("HX-Request") == "true"` is a one-liner. The library adds a dependency for trivial functionality. Per MD-1: prefer stdlib, introduce deps only with clear payoff.

**Instead:** Direct header check. If needed frequently, write a two-line helper in the web package.

### Anti-Pattern 3: SPA-Style Client-Side Routing

**What:** Using htmx hx-boost on the entire page for SPA-like navigation.

**Why bad:** Breaks browser expectations (form submission, new tab behavior). Makes debugging harder. The server already renders fast (SQLite local reads).

**Instead:** Use hx-push-url on specific interactions (search, compare). Let normal navigation use full page loads -- they are fast enough from edge nodes with local SQLite.

### Anti-Pattern 4: CDN Dependencies

**What:** Loading htmx.js or Tailwind CSS from a CDN.

**Why bad:** Adds external network dependency. Edge nodes may have limited connectivity. Breaks offline development. CSP headers become more complex.

**Instead:** Vendor htmx.min.js. Build Tailwind CSS at compile time and embed the output.

### Anti-Pattern 5: Separate API Calls from UI

**What:** Having the web UI make fetch() calls to /api/ or /graphql and render client-side.

**Why bad:** Adds client-side rendering complexity, loses server-side rendering benefits (SEO, initial load speed, no JS requirement). Defeats the purpose of templ + htmx.

**Instead:** Web handlers query ent directly (same as pdbcompat handlers do). The UI has its own data access path optimized for display needs.

## Component Boundaries

| Component | Responsibility | Communicates With |
|-----------|---------------|-------------------|
| `internal/web/handler.go` | HTTP request handling, routing, response rendering | ent.Client, templates |
| `internal/web/search.go` | Cross-type search query logic | ent.Client |
| `internal/web/viewmodel.go` | View model struct definitions | None (pure data types) |
| `internal/web/templates/*.templ` | HTML rendering with Tailwind classes | View model structs only |
| `internal/web/static/` | CSS (Tailwind output), JS (htmx) | Embedded at build time |
| `cmd/peeringdb-plus/main.go` | Wiring: creates Handler, mounts routes | web.Handler, ent.Client |

**Boundary rule:** Templates import view model types only. Handlers import ent and templates. Nothing imports handlers.

## Build Pipeline

**Development workflow:**

```bash
# Terminal 1: Watch and regenerate templ files
templ generate --watch

# Terminal 2: Watch and rebuild Tailwind CSS
./tailwindcss -i internal/web/static/css/input.css \
              -o internal/web/static/css/output.css --watch

# Terminal 3: Run server with hot reload
air  # or: go run ./cmd/peeringdb-plus
```

**CI pipeline additions:**

```yaml
# Add to existing CI steps:
- name: Install templ
  run: go install github.com/a-h/templ/cmd/templ@latest

- name: Generate templ
  run: templ generate

- name: Check templ generate drift
  run: git diff --exit-code  # Fail if generated files differ
```

**Taskfile integration:** If Taskfile is adopted (per STACK.md recommendation), add tasks for templ generate, tailwind build, and a combined dev mode.

## Scalability Considerations

| Concern | Current (single node) | At edge deployment |
|---------|----------------------|-------------------|
| Search latency | SQLite LIKE queries ~10ms, 5 types parallel | Same -- SQLite is local on each edge node |
| Template rendering | templ compiles to Go, sub-millisecond | Same -- CPU-bound, no I/O |
| Static assets | Embedded, served from memory | Same -- embedded in binary |
| Comparison queries | 4-6 ent queries, ~20ms | Same -- all data is local |
| CSS bundle size | Tailwind purge, ~15-30KB gzipped | Same -- served from edge |

**No scalability concerns for the web UI.** The entire architecture is read-only, local-data, server-rendered. This is the ideal case for edge deployment.

## Dependency Summary

| New Dependency | Purpose | Type | Impact |
|---------------|---------|------|--------|
| `github.com/a-h/templ` | Type-safe HTML templates | Build-time (templ generate) + runtime | Already in STACK.md, HIGH confidence |
| `htmx.min.js` (vendored) | Client-side interactivity | Static asset, no Go dependency | Single file, ~14KB gzipped |
| `tailwindcss` standalone CLI | CSS compilation | Build-time only, not a Go dependency | External binary, not in go.mod |

**No new Go module dependencies beyond templ.** htmx is a vendored JS file. Tailwind is a build tool.

## Sources

- [templ Project Structure](https://templ.guide/project-structure/project-structure/) -- Official recommended layout
- [templ Template Composition](https://templ.guide/syntax-and-usage/template-composition/) -- Layout and children patterns
- [htmx hx-trigger](https://htmx.org/attributes/hx-trigger/) -- Debounce with delay modifier
- [htmx hx-push-url](https://htmx.org/attributes/hx-push-url/) -- URL state management
- [htmx multi-swap](https://v1.htmx.org/extensions/multi-swap/) -- Multiple target updates (for compare tool)
- [Tailwind CSS Standalone CLI](https://tailwindcss.com/blog/standalone-cli) -- No Node.js dependency
- [Tailwind v4 Go Integration](https://github.com/tailwindlabs/tailwindcss/discussions/15815) -- Configuration for Go projects
- [Go embed](https://pkg.go.dev/embed) -- Static asset embedding (stdlib)
- [htmx-go (angelofallars)](https://github.com/angelofallars/htmx-go) -- Evaluated and rejected (anti-pattern 2)
- [Bookmarkable URL State in HTMX](https://www.lorenstew.art/blog/bookmarkable-by-design-url-state-htmx/) -- URL-as-state pattern
- [GoTTH Stack Production Deployment](https://4rkal.com/posts/deploy-go-htmx-templ-tailwind-to-production/) -- Build and deployment patterns
- Existing codebase analysis: `cmd/peeringdb-plus/main.go`, `internal/pdbcompat/handler.go`, `internal/pdbcompat/search.go`, `internal/middleware/*.go`
