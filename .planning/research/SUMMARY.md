# Project Research Summary

**Project:** PeeringDB Plus v1.4 -- Web UI
**Domain:** Server-rendered web interface for network interconnection data exploration
**Researched:** 2026-03-24
**Confidence:** HIGH

## Executive Summary

PeeringDB Plus v1.4 adds a polished, interactive web UI to an existing Go API server that already syncs all 13 PeeringDB object types, stores them in SQLite on the edge, and exposes them via GraphQL, REST, and PeeringDB-compatible APIs. The research converges on a server-rendered approach using templ (type-safe Go HTML templates), htmx (server-driven interactivity via HTML attributes), and Tailwind CSS v4 (standalone CLI, no Node.js). This stack adds exactly one new Go module dependency (`github.com/a-h/templ`), keeps the project in pure Go with no JavaScript build toolchain, and maintains the single-binary deployment model via `embed.FS`.

The recommended approach builds three user-facing capabilities in dependency order: (1) live search with as-you-type results grouped by type, (2) record detail pages for all 5 primary entity types with lazy-loaded related records, and (3) an ASN comparison tool showing shared IXPs and facilities between two networks. The architecture follows the existing codebase pattern -- a `Handler` struct accepting `*ent.Client`, registering routes on `*http.ServeMux`, with a view model layer decoupling ent-generated types from templates. Every handler serves both full pages (direct navigation) and HTML fragments (htmx partial updates), keyed on the `HX-Request` header.

The primary risks are well-understood and preventable. The most critical pitfall is the full-page vs. fragment rendering duality: every handler that serves HTML must work for both bookmarked URLs and htmx requests, or shareable links break. Search performance requires attention -- the existing `LIKE '%term%'` queries do full table scans, which may be acceptable for the current dataset (~200K rows) but should be benchmarked early and replaced with FTS5 if latency exceeds 50ms. Tailwind's class scanner does not auto-detect `.templ` files, requiring an explicit `@source` directive. The LiteFS read-only replica constraint is safe for a read-only UI but demands vigilance against hidden write paths (no sessions, no analytics writes).

## Key Findings

### Recommended Stack

The v1.4 stack adds three technologies to the existing Go backend, all chosen to avoid a JavaScript build toolchain and maintain single-binary deployment. See [STACK.md](STACK.md) for full details.

**Core technologies:**
- **templ v0.3.1001:** Type-safe HTML templating. Compiles `.templ` files to Go code at build time. Components are pure functions with compile-time type checking. Implements `templ.Component` interface, plugs directly into `net/http` handlers. 5,400+ importers. The only new Go module dependency.
- **htmx 2.0.7:** Server-driven UI interactivity via HTML attributes. 14KB gzipped. Zero dependencies. Self-hosted via `embed.FS` (no CDN). Covers live search (`hx-trigger` with debounce), lazy-loading (`hx-trigger="revealed"`), URL state (`hx-push-url`), and partial page updates.
- **Tailwind CSS v4.2.2 (standalone CLI):** Utility-first CSS framework. Single binary, no Node.js. CSS-first configuration with `@source` directive for scanning `.templ` files. Output committed to repo and embedded in binary. Production output typically 5-15KB.

**Explicit non-additions:** No React/Vue/Svelte, no Node.js/npm, no webpack/vite, no Alpine.js, no chi router, no component libraries. The existing `http.NewServeMux()` handles all new routes.

### Expected Features

See [FEATURES.md](FEATURES.md) for full feature landscape with interaction pattern details.

**Must have (table stakes):**
- Search box on homepage with as-you-type results (300ms debounce, htmx active search pattern)
- Results grouped by type (Networks, IXPs, Facilities, Organizations, Campuses) with colored badges
- ASN direct lookup (numeric input treated as ASN-first)
- Record detail pages for all 5 primary types with organized field sections
- Related records on detail pages in collapsible sections with lazy-loading
- Clean, linkable URLs (`/net/13335`, `/search?q=cloudflare`, `/compare?asn1=X&asn2=Y`)
- Mobile-responsive layout (Tailwind responsive utilities)

**Should have (differentiators):**
- ASN comparison tool with shared IXPs/facilities, port speeds, and IP addresses
- "Compare with..." button on network detail pages (pre-fills one ASN)
- Shared-only default view in comparison (answers "where can we peer?" directly)
- Sub-100ms search latency (architectural advantage: local SQLite, no network round-trip)
- Dark mode (Tailwind `dark:` variant, system preference default)
- Keyboard navigation in search results (ARIA roles provide accessibility for free)

**Defer (v2+):**
- User accounts / authentication (read-only public mirror, login-free is an advantage)
- Advanced multi-field search (GraphQL/REST already covers this)
- Map visualization (link to Google Maps instead)
- Data export UI (REST/GraphQL APIs already provide this)
- Multi-ASN comparison (>2 ASNs; two covers the dominant use case)
- Full-text search / FTS5 (only if LIKE performance proves insufficient)
- Pagination (limit to 15 results per type, encourage query refinement)

### Architecture Approach

The web UI mounts as a new `internal/web` package on the existing `http.ServeMux`, following the same handler pattern as `internal/pdbcompat`. A view model layer sits between ent queries and templ templates, preventing template coupling to ORM-generated types. Handlers detect `HX-Request` headers to serve full pages or fragments from the same endpoint. Static assets (compiled CSS, vendored htmx.js) are embedded via `embed.FS` for single-binary deployment. See [ARCHITECTURE.md](ARCHITECTURE.md) for full details.

**Major components:**
1. **`internal/web/handler.go`** -- HTTP handlers for search, detail, compare. Receives `*ent.Client`, registers routes, renders templ components. Dual-mode rendering (full page vs. fragment) via `HX-Request` header detection.
2. **`internal/web/templates/*.templ`** -- Templ components for layout, pages, and reusable UI elements. Import view model types only, never ent types. Compiled to Go code by `templ generate`.
3. **`internal/web/search.go`** -- Cross-type search using `errgroup` fan-out across 5 entity types. Reuses search predicate patterns from pdbcompat. Returns at most 5 results per type.
4. **`internal/web/viewmodel.go`** -- View model structs (`SearchResult`, `DetailView`, `CompareView`) decoupling templates from ent-generated types. Enables testing with hand-constructed data.
5. **`internal/web/static/`** -- Tailwind output CSS and htmx.min.js, embedded at build time. Served via `http.FileServerFS`.

### Critical Pitfalls

See [PITFALLS.md](PITFALLS.md) for all 14 pitfalls with detailed prevention strategies.

1. **Full-page vs. fragment rendering blindness (Critical #1).** Every handler must check `HX-Request` header and serve either a full page (with layout) or bare fragment. Without this, bookmarked/shared URLs render unstyled HTML. Build a single `renderPage` helper. Add `Vary: HX-Request` response header. Test both paths for every endpoint.

2. **Live search query storm (Critical #2).** Without debounce and request cancellation, every keystroke fires a SQLite query. Use `hx-trigger="keyup changed delay:300ms"` and `hx-sync="this:replace"`. Require minimum 2-character queries. Cap results with `LIMIT`.

3. **Tailwind not scanning .templ files (Critical #3).** Tailwind v4's automatic detection may skip `.templ` files. Use explicit `@source "../templates/**/*.templ"` directive. Never construct class names dynamically. Verify production CSS contains expected classes in CI.

4. **Templ codegen not in build pipeline (Critical #4).** `templ generate` must run before `go build`. Add to CI, Taskfile, and Dockerfile. Add drift detection: `templ generate && git diff --exit-code`.

5. **Routing conflicts with existing API surfaces (Moderate #7).** The existing `GET /` returns JSON discovery. Move it to `GET /api/` or use content negotiation. Update readiness middleware to serve an HTML "syncing" page for browser requests. Bypass readiness for `/static/` paths.

## Implications for Roadmap

Based on research, suggested phase structure with 5 phases:

### Phase 1: Foundation -- templ + Tailwind + Project Structure

**Rationale:** Everything else depends on the template system, CSS framework, and build pipeline being correctly configured. Pitfalls #3 (Tailwind scanning), #4 (templ codegen), and #5 (hot reload loop) all manifest during setup and are cheaper to fix before feature code exists.

**Delivers:** Base HTML layout (head, nav, footer), Tailwind CSS integration with `@source` directive, htmx.min.js vendored and embedded, static file serving via `embed.FS`, templ generate in CI pipeline, dev workflow (templ watch + Tailwind watch), route registration pattern for web handler, `GET /` migration from JSON discovery to HTML homepage.

**Addresses features:** Mobile-responsive layout skeleton, visual type indicator system (badge components), dark mode infrastructure (Tailwind `dark:` variant).

**Avoids pitfalls:** #3 (Tailwind scanning), #4 (codegen pipeline), #5 (hot reload loop), #7 (route conflicts -- resolve `GET /` immediately), #13 (CLI version pinning).

### Phase 2: Live Search

**Rationale:** The homepage IS the search experience. Search is the entry point for all other features (detail pages, comparison). Building search first validates the htmx integration, the `HX-Request` dual-rendering pattern, and the cross-type query architecture.

**Delivers:** Homepage with prominent search box, as-you-type search with 300ms debounce, results grouped by type with colored badges, ASN direct lookup for numeric input, result count badges, search endpoint returning full page or fragment based on `HX-Request`.

**Addresses features:** Search box on homepage, as-you-type results, type-grouped results, ASN direct lookup, visual type indicators, sub-100ms search latency, result count badges.

**Avoids pitfalls:** #1 (full/partial rendering -- establish the pattern here), #2 (debounce + hx-sync), #6 (Vary header + cache-control), #8 (benchmark LIKE query performance with real data).

### Phase 3: Record Detail Pages

**Rationale:** Search results link to detail pages. Without detail pages, search is a dead end. Network detail is the most complex and most visited type -- build it first, then replicate the pattern for IXP, Facility, Organization, Campus.

**Delivers:** Network detail page with organized field sections, related records (IXP presences with speed/IPs, facility presences, contacts) in collapsible sections, lazy-loaded via htmx `hx-trigger="revealed"`, cross-links between related records, "Compare with..." button on network pages. Then remaining 4 detail types (IXP, Facility, Organization, Campus) using the same patterns.

**Addresses features:** Record detail pages for all types, related records on detail pages, collapsible sections, clean linkable URLs, "Compare with..." button, computed summary stats, external links.

**Avoids pitfalls:** #1 (dual rendering), #9 (explicit component parameters, no context abuse), #10 (no accidental writes on read-only replicas), #14 (test detail components with view model structs).

### Phase 4: ASN Comparison Tool

**Rationale:** Comparison depends on network detail pages being done (the "Compare with..." button) and reuses search components (ASN input validation). It is the primary differentiator but not a prerequisite for other features.

**Delivers:** `/compare` page with dual ASN input, shared IXP/facility/campus results with port speeds and IP addresses, shared-only default view, side-by-side toggle, sortable results, URL captures both ASNs for sharing.

**Addresses features:** ASN comparison tool, shared-only default with side-by-side toggle, speed and IP display, compare from network detail page, sortable comparison tables.

**Avoids pitfalls:** #1 (dual rendering for compare results), #2 (debounce on ASN input search), #7 (route registration without conflicts).

### Phase 5: Polish and Accessibility

**Rationale:** Enhancements that layer on top of working features. These are individually small but collectively elevate the product from functional to polished.

**Delivers:** Keyboard navigation in search results (ArrowUp/Down/Enter with ARIA roles), smooth CSS transitions, dark mode toggle with localStorage persistence, loading indicators, empty state messages, error pages (styled 404/500), mobile layout verification and fixes, "syncing" HTML page for readiness middleware.

**Addresses features:** Keyboard navigation, smooth transitions, dark mode, mobile-responsive verification.

**Avoids pitfalls:** #7 (readiness middleware HTML response), #12 (ARIA accessibility).

### Phase Ordering Rationale

- **Dependencies flow downward:** Foundation enables search, search enables detail pages (via linking), detail pages enable comparison (via "Compare with..." button). This is not arbitrary grouping -- each phase produces the prerequisite for the next.
- **Risk front-loading:** The riskiest pitfalls (#1 dual rendering, #2 query storm, #3 Tailwind scanning, #4 codegen pipeline) are addressed in Phases 1-2, before the bulk of feature code is written.
- **Architecture validation early:** Phase 2 validates the core architectural patterns (htmx integration, view models, errgroup fan-out, `HX-Request` detection) that Phases 3-4 reuse heavily.
- **Polish last:** Phase 5 is additive enhancements that can be partially deferred without affecting core functionality.

### Research Flags

Phases likely needing deeper research during planning:
- **Phase 2 (Search):** Benchmark LIKE query performance with real PeeringDB dataset size (~200K rows). If latency exceeds 50ms, FTS5 integration needs research. The existing `buildSearchPredicate` approach may or may not be sufficient.
- **Phase 4 (Comparison):** The set intersection logic for shared IXPs/facilities is conceptually simple but needs careful query design to avoid N+1 patterns. Research optimal ent query patterns for this.

Phases with standard patterns (skip research-phase):
- **Phase 1 (Foundation):** Well-documented setup. templ, Tailwind CLI, and htmx all have clear getting-started guides. The STACK.md and ARCHITECTURE.md already contain exact commands and file structures.
- **Phase 3 (Detail Pages):** Follows patterns established in Phase 2. The ent schema already has all edges defined. Template composition is straightforward templ component calls.
- **Phase 5 (Polish):** ARIA attributes and CSS transitions are well-documented web standards. Dark mode is a Tailwind configuration concern with established patterns.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All three technologies (templ, htmx, Tailwind) are mature, well-documented, and widely used in Go projects. Versions are current. The GoTTH (Go + Templ + Tailwind + HTMX) stack is an established pattern with production references. |
| Features | HIGH | Feature landscape based on direct analysis of PeeringDB.com, bgp.tools, he.net, and PeeringDB's own product blog. Table stakes are unambiguous. Differentiators (comparison tool) are validated against PeerFinder and PeeringDB's own comparison feature. |
| Architecture | HIGH | Architecture extends the existing codebase patterns (`Handler` + `Register` + `*ent.Client`). No new architectural paradigms introduced. The dual-render pattern (full page vs. fragment) is a well-documented htmx pattern. View model layer follows standard Go practices. |
| Pitfalls | HIGH | All 14 pitfalls sourced from official documentation, community issue trackers, and verified against the actual codebase. Critical pitfalls (#1-#4) have concrete prevention strategies with code examples. Phase-specific warning matrix maps pitfalls to implementation phases. |

**Overall confidence:** HIGH

### Gaps to Address

- **LIKE query performance vs. FTS5:** Research identified the risk but deferred the decision. Must benchmark `LIKE '%term%'` across 5 entity types with real PeeringDB data volume during Phase 2. If total search latency exceeds 50ms, implement FTS5 virtual tables. The PITFALLS.md provides FTS5 table design but it has not been validated against ent's migration system.
- **Route migration for `GET /`:** Moving the JSON discovery endpoint from `/` to `/api/` is straightforward but needs to be validated against any external documentation or links that reference the root discovery URL. Check if the pdbcompat layer already handles this.
- **Templ generated file strategy:** ARCHITECTURE.md recommends committing `*_templ.go` files (matching the ent pattern). PITFALLS.md recommends `.gitignore` for `*_templ.go`. Decision needed: commit or generate in CI. Recommendation: commit them (consistency with ent, simpler CI, `go install` works without templ CLI).
- **Tailwind output.css commit strategy:** Both STACK.md and ARCHITECTURE.md recommend committing the compiled CSS. This is correct for the `embed.FS` approach but creates potential merge conflicts. Pin CI to verify freshness via `git diff --exit-code`.

## Sources

### Primary (HIGH confidence)
- [templ documentation](https://templ.guide/) -- component model, HTTP integration, hot reload, project structure
- [htmx documentation](https://htmx.org/docs/) -- trigger modifiers, sync behavior, push-url, active search pattern
- [Tailwind CSS v4 docs](https://tailwindcss.com/docs/) -- standalone CLI, `@source` directive, class detection
- [entgo.io documentation](https://entgo.io/) -- edge traversal, eager loading, query patterns
- [SQLite FTS5 documentation](https://www.sqlite.org/fts5.html) -- prefix queries, content tables
- [PeeringDB product blog](https://docs.peeringdb.com/blog/) -- comparison feature, search improvements, dark mode plans

### Secondary (MEDIUM confidence)
- [GoTTH stack guides](https://medium.com/ostinato-rigore/go-htmx-templ-tailwind-complete-project-setup-hot-reloading-2ca1ba6c28be) -- integration patterns, build pipeline
- [htmx caching patterns](https://www.tutorialspoint.com/htmx/htmx_caching.htm) -- Vary header usage
- [Air + templ infinite reload fix](https://jdo.sh/posts/solving-infinite-reloads-using-air-and-templ/) -- development workflow

### Tertiary (LOW confidence)
- FTS5 integration with ent ORM -- no direct documentation exists; needs validation during Phase 2
- templ v0.3.x stability guarantees -- pre-1.0 but has been stable for 18+ months; no breaking changes observed

---
*Research completed: 2026-03-24*
*Ready for roadmap: yes*
