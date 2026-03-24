# Phase 13: Foundation - Research

**Researched:** 2026-03-24
**Domain:** Web UI infrastructure -- templ templates, Tailwind CSS (CDN), htmx (vendored), static asset embedding, base layout
**Confidence:** HIGH

## Summary

Phase 13 establishes the web UI infrastructure for PeeringDB Plus. The core stack is templ (type-safe Go HTML templating), Tailwind CSS v4 via CDN browser script (no build step), and htmx 2.0.x (vendored and embedded via `//go:embed`). The phase creates the `internal/web/` package following the existing handler pattern (`Handler` struct with `*ent.Client`, `Register(mux)` method), implements a base layout with navigation and footer, and ensures every route renders a styled, responsive HTML page with a clean bookmarkable URL.

The key architectural decision is that web UI routes live under the `/ui/` prefix, cleanly separating them from existing API routes (`/api/`, `/rest/v1/`, `/graphql`). The existing `GET /` root endpoint uses content negotiation: browsers (`Accept: text/html`) get the HTML UI, API clients (`Accept: application/json`) get the JSON discovery response. Every handler must support dual rendering -- full page for direct navigation vs. htmx fragment for partial updates -- detected via the `HX-Request` header, with `Vary: HX-Request` on responses.

Tailwind via CDN eliminates the need for a standalone CLI, Node.js, or any CSS build step. The trade-off is a ~300KB script download and no tree-shaking, but this dramatically simplifies both development and CI. The `*_templ.go` generated files are committed to git (matching the ent pattern) with CI drift detection via `templ generate && git diff --exit-code`.

**Primary recommendation:** Create `internal/web/` package with handler, templ templates, and embedded htmx.min.js. Use Tailwind CDN script tag in the layout head. Commit `*_templ.go` files and add templ drift detection to CI.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- **Content negotiation on `GET /`**: Same route serves HTML to browsers (`Accept: text/html`) and JSON to API clients (`Accept: application/json`). No need to move the JSON discovery endpoint.
- **Web UI routes live under `/ui/` prefix**: `/ui/` (home/search), `/ui/asn/13335` (network by ASN), `/ui/ix/456`, `/ui/fac/789`, `/ui/compare/X/Y`, etc. Clean separation from API routes.
- **Commit `*_templ.go` files**: Consistent with ent pattern. CI adds drift detection (`templ generate && git diff --exit-code`).
- **Tailwind via CDN**: Include Tailwind CSS via CDN `<script>` tag in the base layout. No standalone CLI, no build step, no Node.js. Simplifies development and CI. Trade-off: ~300KB download, no tree-shaking, requires internet.
- **Full navigation bar**: Logo/title, search box, Compare link, About page link, links to API endpoints (GraphQL playground, REST docs, PeeringDB compat), dark mode toggle (sun/moon icon).
- **Footer**: Minimal -- project info, GitHub link.
- **Color scheme**: Neon green on dark -- terminal/hacker aesthetic, bold and modern with tech focus.
  - Background: neutral-900 (#171717)
  - Accent: emerald-500 (#10b981)
  - Text: neutral-100 (#f5f5f5)
  - Vibe: terminal, Matrix, htop -- immediately distinct from PeeringDB's blue
- **Typography**: Monospace touches for data values (ASNs, IPs, speeds), sans-serif for UI text.
- **No logo for now**: Text-based "PeeringDB Plus" title.
- **Every handler must support dual rendering**: Full HTML page (direct navigation) vs. htmx fragment (`HX-Request` header detection).
- **`Vary: HX-Request` response header required** to prevent caching conflicts.
- **Readiness middleware must serve HTML "syncing" page** for browser requests (currently returns JSON 503).

### Codebase Integration Points (from CONTEXT.md)

- `cmd/peeringdb-plus/main.go` lines 144-231: Route registration, middleware stack
- `internal/pdbcompat/handler.go`: Existing handler pattern to follow (`Handler` struct with `*ent.Client`, `Register(mux)` method)
- New package: `internal/web/` -- mirrors pdbcompat pattern
- Static assets: htmx.min.js vendored, embedded via `//go:embed`. Tailwind via CDN (no local CSS file).

### Claude's Discretion

No explicit discretion areas defined in CONTEXT.md.

### Deferred Ideas (OUT OF SCOPE)

No deferred ideas listed in CONTEXT.md. However, per REQUIREMENTS.md, the following DSGN requirements are deferred to Phase 17: DSGN-04 (dark mode toggle), DSGN-05 (CSS transitions), DSGN-06 (loading indicators), DSGN-07 (error pages). Phase 13 should not implement these, though the layout should be structurally ready for them.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| DSGN-01 | All pages are styled with Tailwind CSS with a polished, visually appealing design | Tailwind CSS v4 CDN browser script (`@tailwindcss/browser@4`) included in base layout. Color scheme: neutral-900 bg, emerald-500 accent, neutral-100 text. Monospace for data values, sans-serif for UI text. |
| DSGN-02 | Layout is mobile-responsive and works on all screen sizes | Tailwind's responsive utilities (sm:, md:, lg:, xl:) available via CDN. Layout uses `container mx-auto`, responsive grid, flexbox with `flex-col md:flex-row` patterns. Navigation collapses to hamburger on mobile. |
| DSGN-03 | Every page has a clean, shareable URL that captures the full page state | All routes under `/ui/` prefix with clean paths (`/ui/asn/13335`, `/ui/ix/456`). Every handler serves full page on direct navigation. `hx-push-url` updates browser URL on htmx interactions. No JavaScript-only state. |
</phase_requirements>

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| github.com/a-h/templ | v0.3.1001 | Type-safe HTML templating | Project constraint (CLAUDE.md). Compiles .templ to Go code. Installed locally at this version. |
| htmx | 2.0.8 | Frontend interactivity without JS | Project constraint (CLAUDE.md). Vendored as single file (~47KB uncompressed, ~16KB gzipped). Embedded via go:embed. |
| Tailwind CSS v4 (CDN) | @tailwindcss/browser@4 | Utility-first CSS framework | User decision: CDN only, no build step. Single script tag in layout head. |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| embed (stdlib) | Go 1.26 | Embed htmx.min.js in binary | Required for single-binary deployment. htmx.min.js embedded via `//go:embed`. |
| net/http (stdlib) | Go 1.26 | HTTP handler, ServeMux routing | Existing pattern. Web handlers register on the same mux. |
| entgo.io/ent | v0.14.5 | ORM for data queries | Already in project. Web handlers query ent client for page data. |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Tailwind CDN | Tailwind standalone CLI | CLI provides tree-shaking and smaller CSS, but requires build step and Node.js/binary in CI. User explicitly chose CDN for simplicity. |
| Vendored htmx | htmx CDN | CDN adds external dependency. Vendoring ensures single-binary works offline. User decision: vendor htmx. |
| templ | html/template (stdlib) | html/template has runtime errors and stringly-typed templates. templ provides compile-time type checking and component composition. Project constraint. |

**Installation:**
```bash
# templ CLI (already installed: v0.3.1001)
go install github.com/a-h/templ/cmd/templ@latest

# Add templ module dependency
go get github.com/a-h/templ@latest

# Download htmx for vendoring
curl -o internal/web/static/htmx.min.js https://unpkg.com/htmx.org@2.0.8/dist/htmx.min.js
```

**Tailwind CDN (no installation -- script tag in HTML):**
```html
<script src="https://cdn.jsdelivr.net/npm/@tailwindcss/browser@4"></script>
```

## Architecture Patterns

### Recommended Project Structure

```
internal/web/
  handler.go              # Handler struct, Register(mux), route registration
  handler_test.go         # HTTP handler tests (httptest)
  render.go               # renderPage helper (full page vs htmx fragment)
  static.go               # //go:embed static directive, embed.FS
  static/
    htmx.min.js           # Vendored htmx (committed to git)
  templates/
    layout.templ           # Base HTML layout (head, body, nav, footer)
    layout_templ.go        # Generated (committed to git, drift-detected)
    home.templ             # Home/landing page
    home_templ.go          # Generated
    nav.templ              # Navigation bar component
    nav_templ.go           # Generated
    components.templ       # Reusable UI primitives (cards, badges)
    components_templ.go    # Generated
```

### Pattern 1: Handler Registration (mirrors pdbcompat)

**What:** Web handler follows the same pattern as pdbcompat: struct with `*ent.Client`, constructor, `Register(mux)` method.
**When to use:** All web UI routes.
**Example:**

```go
// internal/web/handler.go
package web

import (
    "net/http"
    "github.com/dotwaffle/peeringdb-plus/ent"
)

// Handler serves web UI pages.
type Handler struct {
    client *ent.Client
}

// NewHandler creates a web UI handler.
func NewHandler(client *ent.Client) *Handler {
    return &Handler{client: client}
}

// Register mounts web UI routes on the given mux.
func (h *Handler) Register(mux *http.ServeMux) {
    // Static assets (embedded)
    mux.Handle("GET /static/", http.StripPrefix("/static/",
        http.FileServerFS(StaticFS)))

    // Web UI pages under /ui/ prefix
    mux.HandleFunc("GET /ui/", h.homeHandler)
    mux.HandleFunc("GET /ui/{rest...}", h.dispatch)
}
```

### Pattern 2: Dual Rendering (full page vs htmx fragment)

**What:** Every handler checks `HX-Request` header. If present, renders fragment only. If absent, wraps fragment in full layout.
**When to use:** Every web UI handler that serves HTML content.
**Example:**

```go
// internal/web/render.go
package web

import (
    "context"
    "net/http"
    "github.com/a-h/templ"
)

// renderPage renders a templ component as either a full page (with layout)
// or an htmx fragment, based on the HX-Request header.
func renderPage(ctx context.Context, w http.ResponseWriter, r *http.Request,
    title string, content templ.Component) error {
    w.Header().Set("Vary", "HX-Request")
    w.Header().Set("Content-Type", "text/html; charset=utf-8")

    if r.Header.Get("HX-Request") == "true" {
        // htmx partial update: render fragment only
        return content.Render(ctx, w)
    }
    // Direct navigation: wrap in full layout
    return templates.Layout(title, content).Render(ctx, w)
}
```

### Pattern 3: Static Asset Embedding

**What:** Embed vendored htmx.min.js via `//go:embed` for single-binary deployment.
**When to use:** All static files served by the web UI.
**Example:**

```go
// internal/web/static.go
package web

import "embed"

//go:embed static
var staticFiles embed.FS

// StaticFS provides access to embedded static files.
// Used with http.FileServerFS to serve /static/ routes.
var StaticFS = staticFiles
```

### Pattern 4: Content Negotiation on Root

**What:** `GET /` serves HTML to browsers, JSON to API clients, based on `Accept` header.
**When to use:** The root endpoint only.
**Example:**

```go
// In cmd/peeringdb-plus/main.go, modify the existing GET / handler:
mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
    if r.URL.Path != "/" {
        http.NotFound(w, r)
        return
    }
    // Content negotiation: HTML for browsers, JSON for API clients
    accept := r.Header.Get("Accept")
    if strings.Contains(accept, "text/html") {
        // Redirect to /ui/ for browser users
        http.Redirect(w, r, "/ui/", http.StatusFound)
        return
    }
    // Existing JSON discovery response
    w.Header().Set("Content-Type", "application/json")
    fmt.Fprint(w, `{"name":"peeringdb-plus","version":"0.1.0",...}`)
})
```

### Pattern 5: Templ Layout with Children

**What:** Base layout component accepts a title and content component as children.
**When to use:** Every full-page render.
**Example:**

```
// internal/web/templates/layout.templ
package templates

templ Layout(title string, contents templ.Component) {
    <!DOCTYPE html>
    <html lang="en" class="dark">
    <head>
        <meta charset="UTF-8"/>
        <meta name="viewport" content="width=device-width, initial-scale=1.0"/>
        <title>{ title } - PeeringDB Plus</title>
        <script src="https://cdn.jsdelivr.net/npm/@tailwindcss/browser@4"></script>
        <script src="/static/htmx.min.js"></script>
        <style type="text/tailwindcss">
            @theme {
                --color-accent: #10b981;
            }
        </style>
    </head>
    <body class="bg-neutral-900 text-neutral-100 min-h-screen flex flex-col">
        @Nav()
        <main class="flex-1 container mx-auto px-4 py-8">
            @contents
        </main>
        @Footer()
    </body>
    </html>
}
```

### Pattern 6: Readiness Middleware Update for HTML

**What:** The readiness middleware currently returns JSON 503 when sync is not complete. For browser requests, it should return a styled HTML "syncing" page instead.
**When to use:** The readiness middleware function in `main.go`.
**Example:**

```go
// Updated readinessMiddleware in cmd/peeringdb-plus/main.go
func readinessMiddleware(sr syncReadiness, next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Infrastructure and static paths bypass readiness
        if r.URL.Path == "/sync" || r.URL.Path == "/healthz" ||
           r.URL.Path == "/readyz" || r.URL.Path == "/" ||
           strings.HasPrefix(r.URL.Path, "/static/") {
            next.ServeHTTP(w, r)
            return
        }
        if !sr.HasCompletedSync() {
            accept := r.Header.Get("Accept")
            if strings.Contains(accept, "text/html") {
                // Browser: serve HTML syncing page
                w.Header().Set("Content-Type", "text/html; charset=utf-8")
                w.WriteHeader(http.StatusServiceUnavailable)
                templates.SyncingPage().Render(r.Context(), w)
                return
            }
            // API client: existing JSON response
            w.Header().Set("Content-Type", "application/json")
            w.WriteHeader(http.StatusServiceUnavailable)
            fmt.Fprint(w, `{"error":"sync not yet completed"}`)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

### Anti-Patterns to Avoid

- **Dynamic Tailwind class construction:** Never use `fmt.Sprintf("bg-%s-500", color)` in templ. Tailwind CDN scans the document at runtime, but dynamically constructed class names in Go code are never rendered to the DOM until after Tailwind has processed the page. Use complete literal class names and map dynamic values in Go.
- **Passing data via context instead of parameters:** Do not use `context.WithValue` to pass page data to nested templ components. Pass all data as explicit function parameters. This is type-safe and testable.
- **Serving generated files as the source of truth:** The `.templ` files are the source. `*_templ.go` files are generated artifacts committed for build reproducibility. Never edit `*_templ.go` directly.
- **Forgetting `Vary: HX-Request`:** Every handler that responds differently to htmx vs direct requests MUST set this header, or caches will serve wrong content.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| HTML templating | Custom string concatenation or html/template | templ | Compile-time type safety, component composition, Go-native |
| CSS framework | Custom stylesheets | Tailwind CSS CDN | Utility-first, responsive, consistent design system |
| Frontend interactivity | Custom JavaScript | htmx | Server-rendered partials, no client-side framework needed |
| Static asset serving | Custom file reader | embed.FS + http.FileServerFS | Stdlib, zero-allocation, single-binary deployment |
| Content negotiation | Manual Accept header parsing | `strings.Contains(accept, "text/html")` | Simple and sufficient for HTML vs JSON split |

**Key insight:** The entire web UI stack avoids client-side JavaScript frameworks. All interactivity comes from htmx (HTML attributes) driving server-rendered templ fragments. This keeps the architecture server-side Go, not a Go+JS hybrid.

## Common Pitfalls

### Pitfall 1: Full-Page vs Fragment Blindness

**What goes wrong:** Handler returns only an htmx fragment. User bookmarks URL, refreshes, or shares link -- gets unstyled fragment without layout/nav/CSS.
**Why it happens:** Developer builds htmx path first, forgets the direct-navigation path.
**How to avoid:** Use the `renderPage()` helper for every handler. It checks `HX-Request` header and wraps in layout when absent. Test both paths for every endpoint.
**Warning signs:** Bookmarked URLs render without styling. Page refresh shows bare HTML.

### Pitfall 2: Missing Vary Header Causes Cache Confusion

**What goes wrong:** Browser caches the htmx fragment response. On back-button navigation, cached fragment is served instead of full page.
**Why it happens:** Without `Vary: HX-Request`, caches key only on URL, not on whether the request was htmx.
**How to avoid:** The `renderPage()` helper always sets `Vary: HX-Request`. Never bypass it.
**Warning signs:** Back button shows broken pages. Inconsistent behavior across page loads.

### Pitfall 3: Templ Code Generation Drift

**What goes wrong:** Developer edits `.templ` files but forgets to run `templ generate`. The committed `*_templ.go` files are stale. Build compiles but serves old templates.
**Why it happens:** templ uses its own CLI, not `//go:generate`. Easy to forget when ent codegen is already in the workflow.
**How to avoid:** Add `templ generate` + `git diff --exit-code` to CI (same pattern as existing ent drift detection). Update CI workflow to install templ and run the check.
**Warning signs:** UI shows old content after template changes. CI drift check catches uncommitted changes.

### Pitfall 4: Tailwind CDN Performance Misconception

**What goes wrong:** Developer worries about CDN performance. Tailwind v4 CDN browser script processes styles client-side, which adds a brief unstyled flash (FOUC) on first load. This is acceptable for a development/internal tool but noticeable.
**Why it happens:** CDN approach trades build-time processing for runtime processing.
**How to avoid:** Accept this trade-off as a user decision. Mitigate FOUC by adding a minimal inline `<style>` for critical above-the-fold styling (dark background, basic text color). The CDN script processes quickly (~50ms on modern browsers).
**Warning signs:** Brief flash of white/unstyled content before Tailwind classes apply.

### Pitfall 5: Static Asset Path Mismatch with embed.FS

**What goes wrong:** `//go:embed static` creates an `embed.FS` rooted at the package directory. The embedded file paths include the `static/` prefix (e.g., `static/htmx.min.js`). If `http.FileServerFS` is mounted with `StripPrefix("/static/")`, the paths must match exactly.
**Why it happens:** `embed.FS` preserves the directory structure relative to the Go source file containing the directive. The `static/` prefix is part of the embedded path.
**How to avoid:** Either use `fs.Sub(staticFiles, "static")` to strip the prefix from the embed.FS, or adjust the HTTP handler accordingly. Test by requesting `/static/htmx.min.js` and verifying 200 response.
**Warning signs:** 404 on `/static/htmx.min.js`. Browser console errors about missing script.

### Pitfall 6: Readiness Middleware Blocking Static Assets

**What goes wrong:** Before the first sync completes, the readiness middleware returns 503 for all routes. This blocks CSS/JS assets, so the "syncing" page itself is unstyled.
**Why it happens:** Current readiness middleware bypass list only includes `/sync`, `/healthz`, `/readyz`, `/`. Static assets at `/static/` are blocked.
**How to avoid:** Add `/static/` to the readiness middleware bypass list. The syncing page needs htmx.min.js and Tailwind CDN to render properly.
**Warning signs:** Syncing page renders as plain unstyled HTML.

## Code Examples

### Templ Component with Tailwind Classes

```
// internal/web/templates/nav.templ
package templates

templ Nav() {
    <nav class="bg-neutral-800 border-b border-neutral-700">
        <div class="container mx-auto px-4">
            <div class="flex items-center justify-between h-16">
                <div class="flex items-center space-x-4">
                    <a href="/ui/" class="text-emerald-500 font-bold text-xl font-mono">
                        PeeringDB Plus
                    </a>
                </div>
                <div class="hidden md:flex items-center space-x-6">
                    <a href="/ui/" class="text-neutral-300 hover:text-emerald-400 transition-colors">
                        Search
                    </a>
                    <a href="/ui/compare" class="text-neutral-300 hover:text-emerald-400 transition-colors">
                        Compare
                    </a>
                    <a href="/graphql" class="text-neutral-300 hover:text-emerald-400 transition-colors">
                        GraphQL
                    </a>
                    <a href="/rest/v1/" class="text-neutral-300 hover:text-emerald-400 transition-colors">
                        REST API
                    </a>
                    <a href="/api/" class="text-neutral-300 hover:text-emerald-400 transition-colors">
                        PeeringDB API
                    </a>
                </div>
                <!-- Mobile menu button -->
                <button class="md:hidden text-neutral-300 hover:text-emerald-400"
                        onclick="document.getElementById('mobile-menu').classList.toggle('hidden')">
                    <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                              d="M4 6h16M4 12h16M4 18h16"></path>
                    </svg>
                </button>
            </div>
            <!-- Mobile menu -->
            <div id="mobile-menu" class="hidden md:hidden pb-4 space-y-2">
                <a href="/ui/" class="block text-neutral-300 hover:text-emerald-400 py-1">Search</a>
                <a href="/ui/compare" class="block text-neutral-300 hover:text-emerald-400 py-1">Compare</a>
                <a href="/graphql" class="block text-neutral-300 hover:text-emerald-400 py-1">GraphQL</a>
                <a href="/rest/v1/" class="block text-neutral-300 hover:text-emerald-400 py-1">REST API</a>
                <a href="/api/" class="block text-neutral-300 hover:text-emerald-400 py-1">PeeringDB API</a>
            </div>
        </div>
    </nav>
}
```

### Templ Footer Component

```
// internal/web/templates/footer.templ
package templates

templ Footer() {
    <footer class="bg-neutral-800 border-t border-neutral-700 py-6 mt-auto">
        <div class="container mx-auto px-4 text-center text-neutral-500 text-sm">
            <p>
                PeeringDB Plus -- A read-only mirror of
                <a href="https://www.peeringdb.com" class="text-emerald-500 hover:text-emerald-400">
                    PeeringDB
                </a>
            </p>
            <p class="mt-1">
                <a href="https://github.com/dotwaffle/peeringdb-plus"
                   class="text-emerald-500 hover:text-emerald-400">
                    GitHub
                </a>
            </p>
        </div>
    </footer>
}
```

### Handler Test Pattern

```go
// internal/web/handler_test.go
package web

import (
    "bytes"
    "context"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"

    "github.com/a-h/templ"
    "github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// renderComponent renders a templ component to a string for test assertions.
func renderComponent(t *testing.T, c templ.Component) string {
    t.Helper()
    var buf bytes.Buffer
    if err := c.Render(context.Background(), &buf); err != nil {
        t.Fatalf("render component: %v", err)
    }
    return buf.String()
}

func TestHomeHandler_FullPage(t *testing.T) {
    t.Parallel()
    client := testutil.SetupClient(t)
    h := NewHandler(client)
    mux := http.NewServeMux()
    h.Register(mux)

    req := httptest.NewRequest("GET", "/ui/", nil)
    req.Header.Set("Accept", "text/html")
    rec := httptest.NewRecorder()
    mux.ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d", rec.Code)
    }
    body := rec.Body.String()
    if !strings.Contains(body, "<!DOCTYPE html>") {
        t.Error("expected full HTML page with DOCTYPE")
    }
    if !strings.Contains(body, "PeeringDB Plus") {
        t.Error("expected page title")
    }
    if !strings.Contains(body, "htmx.min.js") {
        t.Error("expected htmx script reference")
    }
}

func TestHomeHandler_HtmxFragment(t *testing.T) {
    t.Parallel()
    client := testutil.SetupClient(t)
    h := NewHandler(client)
    mux := http.NewServeMux()
    h.Register(mux)

    req := httptest.NewRequest("GET", "/ui/", nil)
    req.Header.Set("HX-Request", "true")
    rec := httptest.NewRecorder()
    mux.ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d", rec.Code)
    }
    body := rec.Body.String()
    if strings.Contains(body, "<!DOCTYPE html>") {
        t.Error("htmx fragment should NOT contain DOCTYPE")
    }
    if rec.Header().Get("Vary") != "HX-Request" {
        t.Error("expected Vary: HX-Request header")
    }
}
```

### Static Asset Handler Test

```go
func TestStaticAssets(t *testing.T) {
    t.Parallel()
    h := NewHandler(testutil.SetupClient(t))
    mux := http.NewServeMux()
    h.Register(mux)

    req := httptest.NewRequest("GET", "/static/htmx.min.js", nil)
    rec := httptest.NewRecorder()
    mux.ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("expected 200 for htmx.min.js, got %d", rec.Code)
    }
    if !strings.Contains(rec.Body.String(), "htmx") {
        t.Error("expected htmx content in response")
    }
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Tailwind v3 CDN (`cdn.tailwindcss.com`) | Tailwind v4 CDN (`@tailwindcss/browser@4`) | Tailwind v4 release (2025) | Different script tag URL, new `@theme` directive syntax, `type="text/tailwindcss"` for custom CSS |
| templ v0.2.x | templ v0.3.x | 2025 | Major version bump. `templ generate` remains the same. CSS component syntax updated. |
| htmx 1.x | htmx 2.0.x | June 2024 | htmx 2.0 dropped IE support, changed some default behaviors (self-closing tags, etc). No breaking changes for our use case. |
| html/template | templ | Ongoing | templ provides compile-time type safety. Industry trend toward typed templates. |

**Deprecated/outdated:**
- `cdn.tailwindcss.com` script tag: This is Tailwind v3 CDN. Use `@tailwindcss/browser@4` for v4.
- htmx 1.x: htmx 2.0 is the current stable line. htmx 1.x is maintained but no new features.

## Project Constraints (from CLAUDE.md)

The following CLAUDE.md directives apply to this phase:

- **CS-0 (MUST):** Modern Go code guidelines
- **CS-1 (MUST):** Enforce `gofmt`, `go vet`
- **CS-2 (MUST):** Avoid name stutter (`web.Handler` not `web.WebHandler`)
- **CS-5 (MUST):** Input structs for functions with >2 arguments
- **ERR-1 (MUST):** Wrap errors with `%w` and context
- **API-1 (MUST):** Document exported items
- **API-2 (MUST):** Accept interfaces where needed, return concrete types
- **T-1 (MUST):** Table-driven tests, deterministic and hermetic
- **T-2 (MUST):** Run `-race` in CI, add `t.Cleanup`
- **T-3 (SHOULD):** Mark safe tests with `t.Parallel()`
- **OBS-1 (MUST):** Structured logging with `slog`
- **SEC-1 (MUST):** Validate inputs, set I/O timeouts
- **CI-1 (MUST):** Lint, vet, test (`-race`), build on every PR
- **CI-2 (MUST):** Reproducible builds with `-trimpath`

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go testing stdlib + enttest |
| Config file | None needed (stdlib) |
| Quick run command | `go test -race ./internal/web/...` |
| Full suite command | `CGO_ENABLED=1 go test -race ./...` |

### Phase Requirements to Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| DSGN-01 | Pages styled with Tailwind (script tag present, classes in HTML) | unit | `go test -race ./internal/web/... -run TestLayout` | No -- Wave 0 |
| DSGN-02 | Layout responsive (responsive classes present in rendered HTML) | unit | `go test -race ./internal/web/... -run TestResponsive` | No -- Wave 0 |
| DSGN-03 | Clean URLs, full page on direct access | unit | `go test -race ./internal/web/... -run TestFullPage` | No -- Wave 0 |
| N/A | Static assets served (htmx.min.js) | unit | `go test -race ./internal/web/... -run TestStaticAssets` | No -- Wave 0 |
| N/A | HX-Request fragment rendering | unit | `go test -race ./internal/web/... -run TestHtmxFragment` | No -- Wave 0 |
| N/A | Vary: HX-Request header present | unit | `go test -race ./internal/web/... -run TestVaryHeader` | No -- Wave 0 |
| N/A | Content negotiation on GET / | unit | `go test -race ./cmd/peeringdb-plus/... -run TestContentNeg` or integration | No -- Wave 0 |

### Sampling Rate

- **Per task commit:** `go test -race ./internal/web/...`
- **Per wave merge:** `CGO_ENABLED=1 go test -race ./...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps

- [ ] `internal/web/handler_test.go` -- handler tests for DSGN-01, DSGN-02, DSGN-03
- [ ] `internal/web/render_test.go` -- dual rendering tests (full page vs fragment)
- [ ] templ install in CI: `go install github.com/a-h/templ/cmd/templ@latest`
- [ ] templ drift detection in CI: `templ generate && git diff --exit-code`

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | Build, test | Yes | 1.26.1 | -- |
| templ CLI | .templ to Go code generation | Yes | v0.3.1001 | `go install github.com/a-h/templ/cmd/templ@latest` |
| golangci-lint | Linting | Yes | 2.11.3 | -- |
| curl/wget | Download htmx.min.js | Yes | system | -- |
| task runner | Build orchestration | No | -- | Not needed -- manual commands or shell script |
| air | Hot reload | No | -- | Manual restart during development |
| Node.js | Not needed | N/A | -- | Tailwind via CDN eliminates Node.js dependency |

**Missing dependencies with no fallback:** None -- all required tools are available.

**Missing dependencies with fallback:**
- `task` runner not installed -- not required. Build commands run directly.
- `air` not installed -- not required for phase 13. Manual restart sufficient.

## Open Questions

1. **htmx exact version to vendor**
   - What we know: 2.0.8 is the latest per npm registry. GitHub releases page shows 2.0.7 as latest with downloadable assets.
   - What's unclear: Whether 2.0.8 has been published to GitHub releases with downloadable binary.
   - Recommendation: Download from unpkg.com/htmx.org@2.0.8/dist/htmx.min.js (npm-based CDN) which is guaranteed to have the latest published version. If 2.0.8 is not available, fall back to 2.0.7.

2. **Tailwind CDN FOUC (Flash of Unstyled Content)**
   - What we know: CDN processes styles client-side, which can cause a brief unstyled flash.
   - What's unclear: Severity on first load with the dark theme (white flash on dark page is very noticeable).
   - Recommendation: Add minimal inline `<style>` for `body { background-color: #171717; color: #f5f5f5; }` to prevent white flash while Tailwind loads. This is a ~2 line addition to the layout template.

3. **Mobile hamburger menu without JavaScript**
   - What we know: Navigation needs to collapse on mobile. Pure CSS hamburger menus exist but are complex.
   - What's unclear: Whether a tiny inline `onclick` toggle is acceptable or if htmx should handle it.
   - Recommendation: Use a minimal inline `onclick` that toggles a `hidden` class, as shown in the nav component example. This is 1 line of JS and does not require htmx.

## Sources

### Primary (HIGH confidence)

- [templ guide - Creating Components](https://templ.guide/quick-start/creating-a-simple-templ-component) - Component syntax, parameters, rendering
- [templ guide - Template Composition](https://templ.guide/syntax-and-usage/template-composition) - Layout pattern with children, cross-package components
- [templ guide - CSS Style Management](https://templ.guide/syntax-and-usage/css-style-management) - Tailwind class usage, conditional classes, templ.KV
- [templ guide - Creating HTTP Server](https://templ.guide/server-side-rendering/creating-an-http-server-with-templ/) - templ.Handler(), Render() pattern, handler integration
- [templ guide - htmx Integration](https://templ.guide/server-side-rendering/htmx/) - Fragment rendering, hx-select, partial updates
- [htmx Documentation](https://htmx.org/docs/) - HX-Request header, hx-boost, Vary header, hx-push-url
- [Tailwind CSS Play CDN](https://tailwindcss.com/docs/installation/play-cdn) - v4 CDN script tag, @theme customization
- Existing codebase: `internal/pdbcompat/handler.go` - Handler pattern to mirror
- Existing codebase: `cmd/peeringdb-plus/main.go` - Route registration, middleware stack
- Existing codebase: `.github/workflows/ci.yml` - CI configuration to extend
- Existing v1.4 research: `.planning/research/ARCHITECTURE.md` - Architecture patterns
- Existing v1.4 research: `.planning/research/PITFALLS.md` - Comprehensive pitfall catalog

### Secondary (MEDIUM confidence)

- [htmx releases (GitHub)](https://github.com/bigskysoftware/htmx/releases) - Version history, latest release
- [htmx npm](https://www.npmjs.com/package/htmx.org) - Latest published version (2.0.8)
- [templ project structure](https://templ.guide/project-structure/project-structure/) - Recommended directory layout

### Tertiary (LOW confidence)

- htmx 2.0.8 exact release date not confirmed from primary sources

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - All technologies are locked decisions from CONTEXT.md and project CLAUDE.md. Versions verified against installed tools and npm registry.
- Architecture: HIGH - Mirrors existing codebase patterns (pdbcompat handler, ent client injection, middleware stack). Route organization decided by user (`/ui/` prefix). Dual rendering pattern well-documented in existing v1.4 research.
- Pitfalls: HIGH - Comprehensive pitfall catalog already exists in `.planning/research/PITFALLS.md` from v1.4 milestone research. Phase-specific pitfalls identified and cross-referenced.

**Research date:** 2026-03-24
**Valid until:** 2026-04-24 (30 days -- stable technology, locked decisions)
