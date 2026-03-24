# Domain Pitfalls

**Domain:** Adding HTMX + Templ + Tailwind Web UI to existing Go API server (PeeringDB Plus)
**Researched:** 2026-03-24
**Milestone:** v1.4 Web UI

## Critical Pitfalls

Mistakes that cause rewrites, broken user experience, or fundamental architectural problems.

### Pitfall 1: Full-Page vs Partial Render Blindness -- Every HTMX Endpoint Must Serve Both

**What goes wrong:** Handlers return only HTML fragments (partials) for HTMX requests. When a user bookmarks a URL, shares it, or refreshes the page, the server receives a normal (non-HTMX) request and returns a bare fragment without layout, navigation, or CSS. The page renders as raw unstyled HTML.

**Why it happens:** Developers build the HTMX path first (fragments swapped into an existing page) and forget that the same URL must also work as a direct browser navigation. HTMX sends an `HX-Request: true` header on its requests, but direct browser navigation does not. Without checking this header, the handler cannot distinguish the two cases.

**Consequences:** Every shareable URL in the app breaks on direct access. This is especially devastating for a project requirement of "linkable/shareable URLs for every page -- URL is the state." Users sharing comparison URLs, search results, or record details get broken pages.

**Prevention:**
- Every handler that serves HTML must check for the `HX-Request` header. If present, return the fragment. If absent, wrap the fragment in the full page layout (head, nav, CSS, scripts).
- Build this as a single middleware or helper function, not per-handler logic. A `RenderPage(ctx, w, r, title, component)` function that checks `r.Header.Get("HX-Request")` and wraps in layout accordingly.
- In templ, compose this as: the fragment is a templ component; the full page is a layout component that accepts the fragment as a child.
- Add the `Vary: HX-Request` response header so caches (CDN, browser, reverse proxy) do not conflate the two responses.
- Write tests for BOTH paths for every UI endpoint from the start.

**Detection:** Bookmarked or shared URLs render without styling. Browser refresh breaks the page.

---

### Pitfall 2: Live Search Without Debounce and Request Cancellation -- SQLite Query Storm

**What goes wrong:** HTMX fires a request on every keystroke via `hx-trigger="keyup"`. A user typing "Hurricane Electric" sends 18 requests in rapid succession. Each request hits SQLite with a `LIKE '%...%'` query across multiple columns and 13 entity types. On a Fly.io edge node with a small VM, this saturates the CPU and creates a backlog of SQLite read transactions that prevent WAL checkpointing.

**Why it happens:** The default `hx-trigger` for inputs is `change`, but live search typically uses `keyup`. Without explicit delay and request cancellation modifiers, every keystroke triggers a full request cycle. The existing `buildSearchPredicate` function in `pdbcompat/search.go` uses `sql.ContainsFold` (which compiles to `LIKE '%term%'` with `LOWER()`) -- this is a full table scan with no index support.

**Consequences:**
- On fast typists, 5-10 concurrent queries execute simultaneously, all doing full table scans.
- SQLite WAL file grows because checkpoint cannot complete while readers are active.
- User sees flickering results as responses arrive out of order (response for "Hurr" arrives after "Hurricane").
- Server resources wasted on queries whose results are immediately discarded.

**Prevention:**
- Use `hx-trigger="keyup changed delay:300ms"` to debounce input. The `changed` modifier prevents firing if the value has not actually changed. The `delay:300ms` waits for a pause in typing.
- Add `hx-sync="this:replace"` on the search input to cancel in-flight requests when a new one starts. This is critical -- without it, stale responses can replace newer ones.
- Require a minimum query length (2-3 characters) before triggering search. Return an empty state for shorter queries.
- Build FTS5 virtual tables for searchable fields. FTS5 prefix queries (`term*`) are orders of magnitude faster than `LIKE '%term%'`. Configure `prefix='2,3'` in the FTS5 table definition for 2- and 3-character prefix indexes.
- For prefix matching (autocomplete-style), use FTS5 prefix queries: `SELECT * FROM search_idx WHERE search_idx MATCH 'hurr*'`. This uses the inverted index instead of scanning every row.
- Cap results returned to the UI (e.g., 20 results) with `LIMIT`.

**Detection:** Typing quickly in the search box causes visible flicker. Network tab shows many concurrent requests to the search endpoint. Server logs show overlapping search queries. WAL file grows during active search sessions.

---

### Pitfall 3: Tailwind v4 Does Not Auto-Detect .templ Files -- Missing Styles in Production

**What goes wrong:** Tailwind v4's automatic content detection scans project files for class names, but it operates on heuristics about which files to scan. `.templ` files are not a standard web file extension. If Tailwind's scanner skips `.templ` files, all Tailwind classes used in templ components are purged from the production CSS output. The development experience may appear fine if using Tailwind's development mode (which includes all classes), but the production build strips them.

**Why it happens:** Tailwind v4 replaced the explicit `content` array with automatic detection that respects `.gitignore` and skips binary files. It scans files as plain text, but uses heuristics to determine which files to scan. The `.templ` extension is not in Tailwind's default recognized set of template file extensions. It may or may not be scanned depending on the project structure and Tailwind version.

**Consequences:** Components render without any styling in production. Everything looks correct in development. This is a classic "works on my machine" failure that only surfaces in the production build pipeline.

**Prevention:**
- Use the `@source` directive in the CSS entry point to explicitly include `.templ` files:
  ```css
  @import "tailwindcss";
  @source "../internal/web/";
  ```
- Alternatively, use `@source` with explicit glob: `@source "../**/*.templ"` to be unambiguous.
- Verify by running the Tailwind standalone CLI in production mode and checking the output CSS contains the expected classes. Add this as a CI check.
- Never construct Tailwind class names dynamically in templ components. `class={ fmt.Sprintf("bg-%s-600", color) }` will NEVER work because Tailwind scans source text statically. Use complete, literal class strings.
- If generated Go files (`*_templ.go`) contain the class strings, Tailwind may pick them up from there -- but do not rely on this. Configure scanning of `.templ` source files explicitly.

**Detection:** Components render unstyled in production but look fine in development. The production CSS file is suspiciously small. Missing utility classes in the compiled CSS.

---

### Pitfall 4: Templ Code Generation Step Missing or Misordered -- Build Fails or Serves Stale Templates

**What goes wrong:** Templ files (`.templ`) must be compiled to Go files (`*_templ.go`) before `go build` runs. The project already has a `go generate` step for ent and gqlgen. Adding templ introduces a second code generation step with ordering dependencies. If `templ generate` does not run, or runs after `go build`, the build either fails (missing generated files) or silently serves stale templates from a previous generation.

**Why it happens:** The project uses `//go:generate` directives in `ent/generate.go` for ent codegen. Templ uses its own CLI (`templ generate`) rather than `//go:generate`. Developers may add `templ generate` to the Taskfile but forget to add it to CI. Or CI runs `go generate ./...` which does not invoke `templ generate`.

**Consequences:**
- Build failure if `*_templ.go` files are gitignored (correct practice) but `templ generate` is not in the build pipeline.
- Stale HTML rendering if `*_templ.go` files are committed (incorrect practice) and become outdated relative to `.templ` source files.
- CI drift detection (`go generate` + `git diff`) may not catch templ drift if templ is invoked separately from `go generate`.

**Prevention:**
- Add `templ generate` to the Taskfile as a prerequisite for `build` and `test` tasks.
- Add `templ generate` to CI before the build step. Run it in the same CI job as other code generation.
- Add `*_templ.go` to `.gitignore`. Generated files should not be committed -- they can be regenerated from `.templ` source. This matches the existing pattern where ent generated files ARE committed but only because ent is special (schema files are hand-written in the ent/schema/ directory).
- Add templ drift detection to CI: run `templ generate` then `git diff --exit-code` on `*_templ.go` files. This catches cases where `.templ` was modified but `templ generate` was not run.
- Install templ in CI: `go install github.com/a-h/templ/cmd/templ@latest` (or pin version).

**Detection:** Build fails with "undefined" errors referencing templ component names. Or UI shows old content after `.templ` file changes.

---

## Moderate Pitfalls

### Pitfall 5: Air/Templ Hot Reload Infinite Loop -- Development Environment Unusable

**What goes wrong:** When using `air` for hot reload during development, air watches for `.go` file changes and rebuilds. But `templ generate` produces `*_templ.go` files. If air is configured to watch all `.go` files AND to run `templ generate` as part of its build command, this creates an infinite loop: templ generates `.go` -> air detects `.go` change -> air runs templ generate -> templ generates `.go` -> loop forever.

**Why it happens:** Air's default configuration watches for `*.go` changes. The `templ generate` command produces files matching `*_templ.go`. Without excluding these files, air's file watcher triggers on its own output.

**Prevention:**
- In `.air.toml`, exclude templ-generated files from the watch pattern: `exclude_regex = [".*_templ\\.go"]`
- Run `templ generate` as part of the air build command: `cmd = "templ generate && go build -o ./tmp/main ./cmd/peeringdb-plus"`
- Include `.templ` in air's watched extensions: `include_ext = ["go", "templ"]`
- Alternative: Run templ in watch mode (`templ generate --watch`) in a separate terminal and configure air to only watch `.go` files but exclude `*_templ.go`.
- Run Tailwind CLI in watch mode (`tailwindcss -i input.css -o output.css --watch`) in a third terminal.

**Detection:** CPU spikes immediately after saving any file. Air's logs show continuous rebuild cycles. Terminal fills with repeated "building..." messages.

---

### Pitfall 6: HTMX Response Caching Breaks Back Button and Browser History

**What goes wrong:** When using `hx-push-url` to update the browser URL (required for shareable links), HTMX snapshots the current DOM to localStorage for history restoration. If the server also sets caching headers, the browser may cache partial HTML responses. When the user presses the back button, the browser cache returns a partial fragment instead of a full page, rendering a broken page.

**Why it happens:** The existing server sets CORS headers via `middleware.CORS()` but does not set `Vary` headers or cache-control on HTML responses. When HTMX and non-HTMX responses share the same URL but have different content (full page vs fragment), the browser cache cannot distinguish them without `Vary: HX-Request`.

**Consequences:** Back button navigation shows broken pages -- either unstyled fragments or stale content. This is difficult to reproduce consistently because it depends on browser caching behavior.

**Prevention:**
- Set `Vary: HX-Request` on all HTML responses that differ based on HTMX vs non-HTMX request.
- Set `Cache-Control: no-store` on HTML responses to prevent browser caching of fragments entirely. The data is served from local SQLite so re-rendering is fast -- there is no latency penalty.
- Consider using `hx-replace-url` instead of `hx-push-url` for filter/search refinements that should not create new history entries. Reserve `hx-push-url` for actual navigation (record detail pages, comparison pages).
- Test back-button behavior explicitly during development. This class of bug only manifests during actual browser interaction, not in automated tests.

**Detection:** Pressing back button shows broken or partial HTML. Network tab shows cached responses being served for HTMX requests.

---

### Pitfall 7: Fourth API Surface Leaks Into Existing Three -- Routing and Middleware Conflicts

**What goes wrong:** The existing server has four route groups: `/graphql`, `/rest/v1/`, `/api/`, and infrastructure routes (`/healthz`, `/readyz`, `/sync`, `/`). Adding a web UI introduces a fifth group that must coexist. Common mistakes: (a) The UI routes catch-all `"GET /"` conflicts with the existing root discovery endpoint. (b) CORS middleware applied globally interferes with same-origin HTML responses. (c) Readiness middleware blocks UI access before first sync, showing a JSON error to browser users.

**Why it happens:** The existing `main.go` registers `"GET /"` as a JSON discovery endpoint that returns API surface URLs. A web UI naturally wants to own `/` as the homepage. The existing readiness middleware returns `application/json` error responses, which display as raw JSON in a browser.

**Consequences:**
- Routing conflict: either the existing `"GET /"` or the new UI homepage handler wins, breaking one surface.
- Browser users see `{"error":"sync not yet completed"}` as raw JSON during the readiness window.
- CORS headers on HTML responses add unnecessary complexity and can confuse browsers.

**Prevention:**
- Move the existing JSON discovery endpoint to a specific path like `/api/discovery` or return it only when `Accept: application/json` is in the request header.
- Create a dedicated route group for UI routes: `/` (homepage), `/search`, `/net/{id}`, `/ix/{id}`, `/fac/{id}`, `/compare`, etc.
- Update the readiness middleware to return an HTML "loading" page for non-API requests (check `Accept` header or path prefix).
- Apply CORS middleware only to API route groups (`/graphql`, `/rest/v1/`, `/api/`), not to the UI routes. Same-origin HTML does not need CORS.
- Use content negotiation middleware: if `Accept: application/json`, route to API discovery; if `Accept: text/html`, route to UI homepage.

**Detection:** Existing API tests break after adding UI routes. Browser shows JSON instead of HTML. CORS preflight errors on UI pages.

---

### Pitfall 8: SQLite Full-Table Scan on LIKE Queries -- Live Search is Slow on Large Tables

**What goes wrong:** The existing search implementation (`buildSearchPredicate`) uses `sql.ContainsFold` which generates `LOWER(column) LIKE '%term%'` SQL. This cannot use any index and requires a full table scan. PeeringDB has ~100K network-IX-LAN records, ~30K networks, and ~15K facilities. A single search query may scan multiple tables. With live search firing on keystrokes (even with debounce), this creates unacceptable latency.

**Why it happens:** `LIKE '%term%'` (contains) queries cannot use B-tree indexes because the wildcard is at the start. Even `LIKE 'term%'` (prefix) queries can use indexes but only if the column has one. The current schema has no text search indexes.

**Consequences:**
- Search latency of 50-200ms per query on a full PeeringDB dataset, multiplied across multiple entity types.
- On Fly.io micro VMs (shared CPU), this can spike to 500ms+.
- User perceives the search as sluggish, defeating the "instant results" requirement.

**Prevention:**
- Create FTS5 virtual tables for searchable fields. A single FTS5 table can index fields from multiple entity types:
  ```sql
  CREATE VIRTUAL TABLE IF NOT EXISTS search_idx USING fts5(
    entity_type, entity_id UNINDEXED, name, aka,
    content='', content_rowid='rowid',
    prefix='2,3'
  );
  ```
- Populate FTS5 tables after each sync completes. Since this is a read-only mirror with hourly syncs, the FTS5 index only needs rebuilding after sync.
- Use FTS5 prefix queries for autocomplete: `WHERE search_idx MATCH 'hurr*'` -- this is a single index lookup, not a table scan.
- For numeric searches (ASN lookup), use a direct index query, not FTS5. ASN searches are exact or prefix matches on an integer field.
- Benchmark query performance with the real PeeringDB dataset size (~130K records across all types) during development, not with test fixtures of 5 records.

**Detection:** Search endpoint latency >100ms with real data. `EXPLAIN QUERY PLAN` shows "SCAN TABLE" instead of using an index.

---

### Pitfall 9: Templ Context Abuse -- Passing Request-Scoped Data via context.Context

**What goes wrong:** Templ components have access to an implicit `ctx` variable (the Go context). Developers use `context.WithValue` in middleware to pass data like "current search query," "active tab," "page title" through the context to deeply nested components. This creates invisible dependencies -- a component silently requires specific context values that are only set by specific middleware, making components non-reusable and hard to test.

**Why it happens:** Templ's documentation shows context as the mechanism for sharing data across component hierarchies (authentication, theming). Developers extend this pattern to all shared state. Since context values are not type-checked at compile time, errors only surface at runtime as nil values or panics.

**Prevention:**
- Use context only for truly cross-cutting concerns: authentication status, request ID, feature flags. The PeeringDB Plus UI is fully public with no auth, so context usage should be minimal.
- Pass all data to components as explicit parameters. This is type-safe and makes dependencies visible:
  ```
  templ SearchResults(query string, results []SearchResult) {
    // ...
  }
  ```
  Not:
  ```
  templ SearchResults() {
    // reads query from ctx -- invisible dependency
  }
  ```
- If context is used, define typed accessor functions (not raw string keys) in a single package. Never use string keys for context values.

**Detection:** Components render incorrectly when used outside their expected middleware chain. Test failures with nil pointer dereferences in template rendering.

---

### Pitfall 10: LiteFS Read-Only Replicas and the Web UI -- No Write Path Needed, But Beware of Side Effects

**What goes wrong:** The existing app correctly handles the LiteFS primary/replica split: replicas serve reads, the primary handles writes (sync). The web UI is read-only, so it should work perfectly on replicas. However, if any UI handler inadvertently triggers a write (e.g., logging to SQLite, session storage, analytics tracking, updating a "last viewed" timestamp), replicas will fail with "SQLITE_READONLY" errors.

**Why it happens:** The UI is explicitly read-only by design, but libraries or middleware may introduce hidden writes. For example, some session middleware writes session data to the database. Rate limiting middleware may store counters in SQLite.

**Consequences:** UI works on the primary node but fails on replicas. Since Fly.io routes requests to the nearest edge node (usually a replica), most users experience the failure.

**Prevention:**
- No session storage. The app is fully public with no user accounts -- there is no session to track.
- No analytics writes to SQLite. If analytics are needed, send them to an external service or via OTel.
- No write-through caching in SQLite. Computed values (like search indexes) must be built during sync, not on first access.
- Add integration tests that run against a read-only SQLite database to catch accidental writes. Open the database with `?mode=ro` in tests.
- Review every middleware in the UI chain for hidden write operations.

**Detection:** UI works locally (single node, primary mode) but returns 500 errors when deployed to Fly.io replicas. Error logs show "attempt to write a readonly database."

---

## Minor Pitfalls

### Pitfall 11: Dynamic Tailwind Class Construction in Templ -- Classes Silently Stripped

**What goes wrong:** Templ supports Go expressions in attributes. Developers write:
```
<div class={ fmt.Sprintf("bg-%s-500", statusColor) }>
```
Tailwind's static analysis never sees the full class name `bg-green-500` because it only exists at runtime. The class is purged from the production CSS.

**Prevention:**
- Always use complete, literal class names. Map dynamic values to complete classes:
  ```
  var statusClasses = map[string]string{
    "active":   "bg-green-500 text-white",
    "inactive": "bg-gray-500 text-gray-200",
  }
  ```
- Use the `@source inline("bg-green-500 bg-gray-500")` safelist directive in the CSS entry point for classes that genuinely must be dynamic, but prefer the mapping approach.

---

### Pitfall 12: HTMX Accessibility -- Live Search Without ARIA Roles Breaks Screen Readers

**What goes wrong:** HTMX swaps HTML fragments into the DOM without notifying assistive technology. A live search that updates results as the user types is invisible to screen readers unless the results container has `aria-live` attributes. The search input lacks `role="combobox"` and `aria-autocomplete` attributes.

**Prevention:**
- Add `aria-live="polite"` to the search results container so screen readers announce updates.
- Add `role="combobox"`, `aria-autocomplete="list"`, and `aria-expanded` to the search input.
- Add a visually hidden live region that announces the result count: "5 results found" after each search update.
- Use `aria-activedescendant` if keyboard navigation through results is implemented.
- Test with a screen reader (VoiceOver, NVDA) at least once per milestone.

---

### Pitfall 13: Tailwind Standalone CLI Version Pinning -- CI and Local Builds Diverge

**What goes wrong:** The Tailwind standalone CLI is a platform-specific binary downloaded from GitHub releases. If CI downloads "latest" and local development uses a pinned version (or vice versa), CSS output may differ. Tailwind v4 had breaking changes from v3 in how utilities are generated.

**Prevention:**
- Pin the Tailwind standalone CLI version in the Taskfile and CI workflow. Download a specific version, not "latest."
- Check the Tailwind CLI binary into the project or use a checksum-verified download script.
- Add the compiled CSS file to `.gitignore` (it is a build artifact). Generate it in CI.
- Use Tailwind v4, not v3, from the start. Do not start with v3 and migrate later.

---

### Pitfall 14: Templ Component Testing Gap -- No Built-In Test Renderer

**What goes wrong:** Templ components compile to Go functions that write to an `io.Writer`. Testing them requires rendering to a buffer and asserting on the HTML output, which is fragile and verbose. Developers skip component tests entirely, relying only on integration tests, leading to hard-to-debug regressions.

**Prevention:**
- Create a test helper that renders a templ component to a string:
  ```go
  func renderComponent(t *testing.T, c templ.Component) string {
    t.Helper()
    var buf bytes.Buffer
    err := c.Render(context.Background(), &buf)
    require.NoError(t, err)
    return buf.String()
  }
  ```
- Test critical components (search results, record detail, comparison table) by asserting on the rendered HTML containing expected content -- not exact HTML matching.
- Use `strings.Contains` or regex assertions rather than golden file comparison for HTML output. HTML whitespace and attribute ordering make golden files brittle.

---

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Mitigation |
|-------------|---------------|------------|
| Project scaffolding (templ + Tailwind setup) | Templ codegen not in build pipeline (#4), Tailwind not scanning .templ files (#3), Air infinite loop (#5) | Configure build toolchain correctly from day one; verify production CSS output |
| Search endpoint + live search | Query storm without debounce (#2), full table scan performance (#8), LIKE query on 100K+ rows (#8) | Build FTS5 index, debounce + hx-sync, minimum query length, LIMIT results |
| URL-driven state / shareable links | Full page vs partial render (#1), cache/history breaks (#6) | Render both full page and fragment from every handler; Vary header; no-store |
| Route integration with existing API surfaces | Routing conflicts with existing endpoints (#7) | Content negotiation; dedicated route groups; update readiness middleware |
| Record detail + comparison views | Same full/partial rendering issue (#1), context abuse for shared state (#9) | Explicit component parameters; middleware for HX-Request detection |
| LiteFS deployment to edge | Read-only replica failures from hidden writes (#10) | Test against read-only database; no session/analytics writes |
| Component composition and reuse | Templ context misuse (#9), component testing gap (#14) | Explicit parameters; test helper for rendering |
| Accessibility | Screen reader incompatibility (#12) | ARIA attributes on search input and results container |
| Styling and CSS build | Dynamic class stripping (#11), CLI version drift (#13) | Complete literal classes; pin Tailwind version |

## Sources

### HTMX
- [htmx Documentation](https://htmx.org/docs/) -- official docs, trigger modifiers, sync behavior
- [hx-push-url Attribute](https://htmx.org/attributes/hx-push-url/) -- URL history management
- [hx-sync Attribute](https://htmx.org/attributes/hx-sync/) -- request cancellation/queuing
- [hx-swap-oob Attribute](https://htmx.org/attributes/hx-swap-oob/) -- out-of-band swaps
- [Tricks of the Htmx Masters](https://hypermedia.systems/tricks-of-the-htmx-masters/) -- advanced patterns
- [HTMX Caching](https://www.tutorialspoint.com/htmx/htmx_caching.htm) -- Vary header and cache behavior
- [HTMX Web Security Basics](https://htmx.org/essays/web-security-basics-with-htmx/) -- security considerations

### Templ
- [templ htmx Integration](https://templ.guide/server-side-rendering/htmx/) -- official HTMX integration docs
- [templ Context](https://templ.guide/syntax-and-usage/context/) -- context.Context usage in components
- [templ Template Composition](https://templ.guide/syntax-and-usage/template-composition/) -- component composition patterns
- [templ Live Reload](https://templ.guide/developer-tools/live-reload/) -- hot reload configuration
- [templ CLI](https://templ.guide/developer-tools/cli/) -- code generation commands
- [Solving Infinite Reloads Using Air and Templ](https://jdo.sh/posts/solving-infinite-reloads-using-air-and-templ/) -- air configuration for templ

### Tailwind CSS
- [Tailwind v4 Detecting Classes](https://tailwindcss.com/docs/detecting-classes-in-source-files) -- automatic content detection, @source directive
- [Tailwind Standalone CLI](https://tailwindcss.com/blog/standalone-cli) -- Node.js-free usage
- [Tailwind CLI Installation](https://tailwindcss.com/docs/installation/tailwind-cli) -- standalone CLI setup

### SQLite / FTS5
- [SQLite FTS5 Extension](https://www.sqlite.org/fts5.html) -- official FTS5 documentation, prefix queries
- [SQLite WAL Mode](https://sqlite.org/wal.html) -- concurrent read/write behavior
- [High-Performance SQLite Reads in Go](https://dev.to/lovestaco/high-performance-sqlite-reads-in-a-go-server-4on3) -- WAL + read concurrency
- [How SQLite Scales Read Concurrency (Fly Blog)](https://fly.io/blog/sqlite-internals-wal/) -- WAL internals for edge deployment

### LiteFS
- [LiteFS Getting Started](https://fly.io/docs/litefs/getting-started-fly/) -- proxy configuration and write forwarding
- [Preventing Read Replica Writes (Fly Community)](https://community.fly.io/t/preventing-read-replica-from-trying-to-write-litefs/16372) -- read-only replica pitfalls

### Accessibility
- [ARIA aria-autocomplete](https://developer.mozilla.org/en-US/docs/Web/Accessibility/ARIA/Reference/Attributes/aria-autocomplete) -- autocomplete accessibility pattern
- [Anatomy of Accessible Auto-Suggest (Intopia)](https://intopia.digital/articles/anatomy-accessible-auto-suggest/) -- live region patterns for search

### Codebase (verified against source)
- `internal/pdbcompat/search.go` -- existing `buildSearchPredicate` uses `sql.ContainsFold` (LIKE query, no FTS)
- `internal/database/database.go` -- WAL mode enabled, 5s busy_timeout, modernc.org/sqlite driver
- `cmd/peeringdb-plus/main.go` -- existing route registration, readiness middleware, CORS middleware, JSON root endpoint
- `ent/schema/*.go` -- 13 entity types, hand-written schemas
- `ent/generate.go` -- existing `//go:generate` for ent codegen
