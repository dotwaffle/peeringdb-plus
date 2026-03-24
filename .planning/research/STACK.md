# Technology Stack: v1.4 Web UI Additions

**Project:** PeeringDB Plus v1.4 (Web UI)
**Researched:** 2026-03-24
**Scope:** Stack additions for Web UI features: live search, record detail views, ASN comparison tool. Does NOT re-research validated backend stack (Go 1.26, entgo, SQLite, GraphQL, REST, OTel).

## New Dependencies

### HTML Templating -- templ

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| `github.com/a-h/templ` | v0.3.1001 | Type-safe HTML templating | Compiles `.templ` files to Go code at build time. Full type checking -- template errors are compile errors, not runtime panics. Components are pure functions: `func MyPage(data MyData) templ.Component`. Implements `templ.Component` interface with `Render(ctx, io.Writer)` method, which plugs directly into `net/http` handlers via `component.Render(r.Context(), w)` or `templ.Handler(component)`. 5,400+ importers on pkg.go.dev. Published 2026-02-28 (latest on pkg.go.dev). MIT licensed. | HIGH |

**Why templ over html/template (stdlib):**
- **Compile-time type safety.** Passing wrong data to a template is a compile error, not a runtime blank page. With 13 PeeringDB types each having detail/list views, type safety prevents an entire class of bugs.
- **Component composition.** Layouts, partials, and shared UI elements compose like function calls. A `Layout(title string)` component wraps page content naturally.
- **IDE support.** VSCode extension provides autocomplete, syntax highlighting, go-to-definition for `.templ` files.
- **No runtime template parsing.** Templates compile to Go code that writes bytes directly. No `template.ParseFiles` at startup, no `template.Execute` reflection overhead.
- **Standard interface.** `templ.Component` implements `io.WriterTo`-style rendering. Works with any `http.Handler` -- no framework coupling.

**Integration with existing HTTP server:**

The existing `main.go` uses `http.NewServeMux()` with method-based routing (Go 1.22+). Templ handlers mount as standard `http.Handler` or `http.HandlerFunc`:

```go
// Static handler for a page component
mux.Handle("GET /", templ.Handler(pages.Home()))

// Dynamic handler with data from ent queries
mux.HandleFunc("GET /net/{id}", func(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    network, err := entClient.Network.Get(r.Context(), id)
    if err != nil {
        http.NotFound(w, r)
        return
    }
    pages.NetworkDetail(network).Render(r.Context(), w)
})
```

**Code generation step:** `templ generate` compiles `*.templ` files to `*_templ.go` files. These generated Go files are committed to the repo (same pattern as ent-generated code). Add to CI: `templ generate && git diff --exit-code` to detect uncommitted changes.

**CLI installation:**

```bash
# Project-local (Go 1.24+ tool directive, preferred)
go get -tool github.com/a-h/templ/cmd/templ@v0.3.1001

# Then invoke as:
go tool templ generate
```

### Frontend Interactivity -- htmx

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| htmx | 2.0.7 | Server-driven UI interactions | AJAX requests via HTML attributes (`hx-get`, `hx-post`, `hx-trigger`). No JavaScript to write for search-as-you-type, partial page updates, or dynamic content loading. 14KB min+gzip. Zero dependencies. Single JS file. Delivered from the Go binary via `embed.FS`, not a CDN. Published 2024-09-11 (latest stable). | HIGH |

**Why htmx over a JavaScript framework:**
- **No build toolchain.** No webpack, no vite, no node_modules. The Go binary serves everything.
- **Server-rendered HTML.** templ renders HTML fragments on the server; htmx swaps them into the DOM. The server is the single source of truth. No client-side state management, no API contract duplication.
- **Perfect fit for search/detail/compare.** Live search = `hx-get="/search?q=..." hx-trigger="input changed delay:300ms"`. Record detail = `hx-get="/net/13335" hx-target="#content"`. Collapsible sections = `hx-get="/net/13335/peers" hx-trigger="revealed"`. All patterns map to htmx primitives.
- **URL-as-state.** `hx-push-url="true"` updates the browser URL bar, making every view linkable/shareable (project requirement).
- **Tiny payload.** 14KB gzipped. Contrast with React (44KB) or Vue (34KB) before any application code.

**Self-hosting via embed.FS (not CDN):**

Download `htmx.min.js` v2.0.7 and embed it in the Go binary. This eliminates external CDN dependencies, ensures the app works in air-gapped environments, and guarantees version consistency.

```go
//go:embed static
var staticFS embed.FS

// In main.go mux setup:
mux.Handle("GET /static/", http.FileServerFS(staticFS))
```

**htmx 4.0 note:** htmx 4.0 is expected mid-2026 (marked "latest" early 2027). It replaces XMLHttpRequest with fetch() internally and makes attribute inheritance explicit. Build on 2.0.7 now -- migration is straightforward when 4.0 stabilizes.

### Styling -- Tailwind CSS

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| Tailwind CSS (standalone CLI) | v4.2.2 | Utility-first CSS framework | Generates only the CSS actually used. CSS-first configuration (no `tailwind.config.js` in v4). Standalone CLI binary -- no Node.js, no npm, no node_modules. Scans `.templ` files for utility classes via `@source` directive. Published 2025-03-18. | HIGH |

**Why Tailwind CSS over alternatives:**
- **Utility-first eliminates CSS file management.** No separate `.css` files per component. Classes in templ templates are the styling. Reduces context switching.
- **Standalone CLI.** A single binary (downloaded from GitHub releases). No Node.js ecosystem infection in a Go project. The binary scans source files, extracts used classes, outputs a single minified CSS file.
- **Automatic purging.** v4 only includes CSS for classes that appear in source files. Production CSS is typically 5-15KB for a full application.
- **v4 CSS-first config.** No JavaScript config file. Theme customization via `@theme` in CSS. Simpler than v3.

**Why v4 specifically:**
- v4 requires only an `input.css` file with `@import "tailwindcss"` -- no init step, no config file
- `@source` directive tells Tailwind where to scan for classes (point it at `.templ` files and generated `_templ.go` files)
- Rebuilds are 3.5x faster (full) and 8x faster (incremental) than v3
- Built-in container queries, 3D transforms, `@starting-style` support

**Standalone CLI setup:**

```bash
# Download (one-time, or in CI)
curl -sLO https://github.com/tailwindlabs/tailwindcss/releases/download/v4.2.2/tailwindcss-linux-x64
chmod +x tailwindcss-linux-x64
mv tailwindcss-linux-x64 /usr/local/bin/tailwindcss

# macOS (for dev):
curl -sLO https://github.com/tailwindlabs/tailwindcss/releases/download/v4.2.2/tailwindcss-macos-arm64
chmod +x tailwindcss-macos-arm64
mv tailwindcss-macos-arm64 /usr/local/bin/tailwindcss
```

**CSS input file (`internal/web/static/input.css`):**

```css
@import "tailwindcss";

/* Scan templ source files and generated Go files for utility classes */
@source "../templates/**/*.templ";
@source "../templates/**/*_templ.go";
```

**Build command:**

```bash
# Development (watch mode)
tailwindcss -i internal/web/static/input.css -o internal/web/static/output.css --watch

# Production (minified)
tailwindcss -i internal/web/static/input.css -o internal/web/static/output.css --minify
```

**Output CSS is committed to the repo** and embedded via `embed.FS`. Same pattern as htmx.min.js. CI verifies the output is fresh: `tailwindcss -i ... -o ... --minify && git diff --exit-code`.

### Development Tooling

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| templ CLI (watch + proxy) | v0.3.1001 | Hot reload during development | `templ generate --watch --proxy="http://localhost:8080" --cmd="go run ./cmd/peeringdb-plus"`. Watches `.templ` files, regenerates Go code, restarts the server, injects browser reload script via HTTP proxy on `:7331`. Single command replaces air + manual rebuild. | HIGH |
| air | v1.64.5 | Alternative hot reload (Go file changes) | File-watching rebuild. Use ONLY if templ's built-in `--watch --cmd` mode proves insufficient (e.g., watching `.go` files outside templ's scope). Published 2026-02-02. Do not install unless needed -- templ's watch mode covers the primary workflow. | LOW |

**Recommended development workflow:**

Run two terminals:

```bash
# Terminal 1: Tailwind CSS watcher
tailwindcss -i internal/web/static/input.css -o internal/web/static/output.css --watch

# Terminal 2: templ watcher with proxy and auto-restart
templ generate --watch --proxy="http://localhost:8080" --cmd="go run ./cmd/peeringdb-plus"
```

Open browser at `http://localhost:7331` (templ proxy). Changes to `.templ` files trigger: regenerate Go code, rebuild binary, restart server, reload browser -- automatically.

**Taskfile integration (when adopted):**

```yaml
tasks:
  dev:
    deps: [dev:css, dev:templ]
  dev:css:
    cmd: tailwindcss -i internal/web/static/input.css -o internal/web/static/output.css --watch
  dev:templ:
    cmd: templ generate --watch --proxy="http://localhost:8080" --cmd="go run ./cmd/peeringdb-plus"
  build:css:
    cmd: tailwindcss -i internal/web/static/input.css -o internal/web/static/output.css --minify
  build:templ:
    cmd: templ generate
```

## Explicit Non-Additions

Things that might seem tempting but should NOT be added.

| Anti-Addition | Why Not |
|---------------|---------|
| React / Vue / Svelte / any SPA framework | Server-rendered HTML with htmx is simpler, faster, and eliminates client-side state management for a read-only data browser. No API contract duplication. No JS build toolchain. |
| Node.js / npm / node_modules | Tailwind standalone CLI and self-hosted htmx.min.js eliminate the only reasons to have Node. Go project stays Go. |
| Webpack / Vite / esbuild / Rollup | No JavaScript to bundle. Tailwind CLI handles CSS. htmx is a single pre-minified file. |
| Alpine.js | Sometimes paired with htmx for client-side state. Unnecessary here -- all UI state lives in the URL and server. Collapsible sections use htmx `hx-trigger="click"` directly. |
| PostCSS | Tailwind v4 standalone CLI includes its own CSS processing. No PostCSS pipeline needed. |
| SASS / Less / CSS-in-JS | Tailwind's utility classes replace custom CSS. Custom styling goes in `@theme` in the input.css file. |
| Bootstrap / Bulma / other CSS frameworks | Tailwind is the styling layer. Mixing frameworks creates conflicts and bloat. |
| chi router | The existing `http.NewServeMux()` with Go 1.22+ method routing handles all current and new routes. Web UI routes are simple `GET /path/{param}` patterns. chi adds value only for middleware grouping, which is not needed with the current flat middleware stack. |
| gorilla/mux | Same reasoning as chi. Stdlib mux is sufficient. |
| templ component library (templUI) | templUI provides pre-built Tailwind components for templ. Adds a dependency for components we can build in ~50 lines each. The project has only ~5 page types (home, search, detail, compare, about). Build them directly. |

## Project Structure for Web UI

```
internal/
  web/
    handler.go          # HTTP handlers (mount on mux)
    handler_test.go     # Handler tests
    static/
      input.css         # Tailwind source CSS
      output.css        # Tailwind compiled CSS (committed, embedded)
      htmx.min.js       # htmx 2.0.7 (committed, embedded)
      embed.go          # //go:embed directive for static files
    templates/
      layout.templ      # Base HTML layout (head, nav, footer)
      home.templ         # Landing page
      search.templ       # Search results (full page + fragment)
      detail.templ       # Record detail view
      compare.templ      # ASN comparison view
      components/
        nav.templ        # Navigation bar
        search_input.templ # Search input with htmx attributes
        record_card.templ  # Record summary card
        related.templ      # Collapsible related records section
```

**Why this structure:**
- `internal/web/` keeps all web UI code in one package, separate from API handlers
- `templates/` holds `.templ` source files; generated `*_templ.go` files appear alongside them
- `static/` holds assets embedded via `embed.FS` -- the Go binary is fully self-contained
- `components/` holds reusable templ components shared across pages
- Handlers in `handler.go` query ent and pass typed data to templ components

## Integration with Existing Server

The Web UI mounts on the existing `http.ServeMux` in `main.go` alongside the existing API routes:

```go
// Existing routes (unchanged):
// POST /sync, GET /healthz, GET /readyz, /graphql, /rest/v1/*, /api/*

// New Web UI routes:
webHandler := web.NewHandler(entClient)
webHandler.Register(mux)
// Registers: GET /, GET /search, GET /net/{id}, GET /ix/{id}, etc.
// GET /compare, GET /static/*
```

**Route conflict resolution:** The existing `GET /` handler returns JSON discovery. The Web UI replaces it with an HTML landing page. The JSON discovery info moves to `GET /api/` (already served by the compat layer) or a new `GET /meta` endpoint.

**Readiness gating:** Web UI routes should be gated by readiness middleware (same as API routes). No data to show if sync hasn't completed. The existing `readinessMiddleware` already handles this for all non-infrastructure paths.

**Static file serving:** `http.FileServerFS(staticFS)` at `GET /static/` serves embedded CSS and JS. Set `Cache-Control: public, max-age=31536000, immutable` on static assets (content-hash in filenames for cache busting).

## CI Pipeline Additions

| Step | Command | Purpose |
|------|---------|---------|
| templ generate check | `go tool templ generate && git diff --exit-code` | Ensure generated `*_templ.go` files are up to date |
| Tailwind CSS build check | `tailwindcss -i internal/web/static/input.css -o internal/web/static/output.css --minify && git diff --exit-code` | Ensure compiled CSS is up to date |
| Tailwind CLI install in CI | Download standalone binary in CI job | No Node.js required on CI runner |

**CI job modification:** Add templ and Tailwind checks to the existing `lint` job (or a new `generate` job). The `test` job already runs `go test -race ./...` which will cover web handler tests.

## Installation Summary

```bash
# New Go dependency (templ library, not CLI)
go get github.com/a-h/templ@v0.3.1001

# templ CLI (project-local tool)
go get -tool github.com/a-h/templ/cmd/templ@v0.3.1001

# Tailwind CSS standalone CLI (download binary)
curl -sLO https://github.com/tailwindlabs/tailwindcss/releases/download/v4.2.2/tailwindcss-linux-x64
chmod +x tailwindcss-linux-x64 && mv tailwindcss-linux-x64 /usr/local/bin/tailwindcss

# htmx (download single file, commit to repo)
curl -sLo internal/web/static/htmx.min.js https://unpkg.com/htmx.org@2.0.7/dist/htmx.min.js
```

**Total new Go module dependencies: 1** (`github.com/a-h/templ`). Everything else is a build tool (Tailwind CLI) or a static asset (htmx.min.js).

## Alternatives Considered

| Category | Recommended | Alternative | Why Not |
|----------|-------------|-------------|---------|
| Templating | templ v0.3.1001 | html/template (stdlib) | Runtime template parsing, stringly-typed, no compile-time type checking. Acceptable for trivial pages, not for 13-type data browser with complex layouts. |
| Templating | templ v0.3.1001 | gomponents | Functional HTML builder in pure Go. No separate template language. But: harder to read for HTML-heavy pages, no IDE HTML support, smaller ecosystem. |
| Interactivity | htmx 2.0.7 | Stimulus (Hotwire) | Larger (30KB+), more complex, designed for Rails. htmx is simpler and framework-agnostic. |
| Interactivity | htmx 2.0.7 | Vanilla JS fetch() | More code to write, maintain, and test. htmx declarative attributes replace 80% of custom JS. |
| Interactivity | htmx 2.0.7 | Datastar | Newer, smaller community, less battle-tested. htmx is the established choice for server-driven HTML. |
| CSS | Tailwind v4.2.2 standalone | Tailwind via npm | Adds Node.js to the project. Standalone CLI is functionally identical, no npm dependency. |
| CSS | Tailwind v4.2.2 | Pico CSS / Simple.css | Classless CSS frameworks. Good for documentation, not for custom data-heavy UIs with specific layout needs. |
| CSS | Tailwind v4.2.2 | vanilla CSS | More CSS to write and maintain. Tailwind's utility classes are faster to develop and produce smaller output after purging. |
| Dev reload | templ watch + proxy | air v1.64.5 | templ's `--watch --cmd` already rebuilds Go and reloads browser. air adds value only for watching non-templ Go files, which is a rare workflow in web UI development. |
| Dev reload | templ watch + proxy | wgo | Similar to air. templ's built-in watch covers the primary need. |
| Component library | Build custom | templUI | 5 page types don't justify a dependency. Custom components are simpler and tailored to PeeringDB data structures. |
| htmx delivery | embed.FS (self-hosted) | CDN (unpkg/cdnjs) | External CDN is a runtime dependency. Self-hosted in the binary ensures availability and version consistency. Edge deployment on Fly.io means CDN latency advantage is negligible. |

## Version Pinning Strategy

| Component | Pin Strategy | Rationale |
|-----------|-------------|-----------|
| templ (Go module) | v0.3.1001 in `go.mod` | Semver pre-1.0, pin exact version. Update deliberately. |
| templ CLI | Same version as module | CLI and library must match to avoid codegen mismatches. |
| htmx | 2.0.7 file committed to repo | Static asset, not a package manager dependency. Update by downloading new file. |
| Tailwind CLI | v4.2.2 binary | Downloaded in CI and dev setup. Pin version in download script/Taskfile. |
| Tailwind CSS output | Committed `output.css` | Derived artifact. Regenerated by CI check. |

## Risk Register

| Risk | Severity | Likelihood | Mitigation |
|------|----------|------------|------------|
| templ pre-1.0 breaking changes | MEDIUM | LOW | templ has been stable in the v0.3.x line for 18+ months with no breaking changes to the component interface. Pin version, update deliberately. The generated Go code is standard -- worst case, hand-edit `*_templ.go` files. |
| htmx 4.0 migration required | LOW | LOW | htmx 4.0 won't be "latest" until early 2027. 2.0.x is stable. Migration is mostly internal (fetch replaces XHR). Explicit attribute inheritance is the main breaking change -- audit `hx-boost` usage. |
| Tailwind v4 standalone CLI bugs | LOW | LOW | v4.2.2 is 3 months post-GA. Standalone CLI is well-tested. Fallback: install via npm temporarily if standalone has issues. |
| Tailwind class scanning misses templ files | MEDIUM | MEDIUM | Use `@source` directive to explicitly point at `.templ` and `*_templ.go` files. Test by verifying expected classes appear in output. The generated Go code contains class strings as literals, so scanning `*_templ.go` is reliable. |
| templ watch mode conflicts with existing server | LOW | MEDIUM | templ proxy listens on `:7331`, app on `:8080`. No port conflict. The proxy passes through all requests including API paths. Development uses proxy URL; production uses app directly. |
| `embed.FS` increases binary size | LOW | LOW | htmx.min.js is ~48KB uncompressed, ~14KB gzipped. Tailwind output CSS is typically 5-15KB. Total static asset overhead: <100KB. Negligible for a server binary. |

## Sources

- [templ v0.3.1001 on pkg.go.dev](https://pkg.go.dev/github.com/a-h/templ) -- published Feb 28, 2026
- [templ GitHub releases](https://github.com/a-h/templ/releases) -- version history
- [templ installation docs](https://templ.guide/quick-start/installation/) -- Go 1.24+ tool directive
- [templ HTTP server guide](https://templ.guide/server-side-rendering/creating-an-http-server-with-templ/) -- templ.Handler, Render method
- [templ live reload docs](https://templ.guide/developer-tools/live-reload/) -- watch + proxy workflow
- [htmx 2.0.7 on GitHub](https://github.com/bigskysoftware/htmx/releases) -- latest stable release
- [htmx documentation](https://htmx.org/docs/) -- attributes, triggers, swap modes
- [htmx future essay](https://htmx.org/essays/future/) -- htmx 4.0 timeline
- [Tailwind CSS v4.2.2 release](https://github.com/tailwindlabs/tailwindcss/releases) -- published Mar 18, 2025
- [Tailwind CLI docs](https://tailwindcss.com/docs/installation/tailwind-cli) -- standalone CLI usage
- [Tailwind v4 @source directive](https://tailwindcss.com/docs/functions-and-directives) -- template scanning
- [Tailwind v4 in Go projects](https://github.com/tailwindlabs/tailwindcss/discussions/15815) -- standalone build step
- [air v1.64.5 on GitHub](https://github.com/air-verse/air/releases) -- published Feb 2, 2026
- [Go embed package](https://pkg.go.dev/embed) -- static file embedding
- [GoTTH stack guide](https://medium.com/ostinato-rigore/go-htmx-templ-tailwind-complete-project-setup-hot-reloading-2ca1ba6c28be) -- integration patterns
