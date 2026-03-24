# Phase 13: Foundation — Discussion Context

**Gathered:** 2026-03-24

## Decisions

### Route Integration
- **Content negotiation on `GET /`**: Same route serves HTML to browsers (`Accept: text/html`) and JSON to API clients (`Accept: application/json`). No need to move the JSON discovery endpoint.
- **Web UI routes live under `/ui/` prefix**: `/ui/` (home/search), `/ui/asn/13335` (network by ASN), `/ui/ix/456`, `/ui/fac/789`, `/ui/compare/X/Y`, etc. Clean separation from API routes.

### Build Pipeline
- **Commit `*_templ.go` files**: Consistent with ent pattern. CI adds drift detection (`templ generate && git diff --exit-code`).
- **Tailwind via CDN**: Include Tailwind CSS via CDN `<script>` tag in the base layout. No standalone CLI, no build step, no Node.js. Simplifies development and CI. Trade-off: ~300KB download, no tree-shaking, requires internet.

### Navigation & Layout
- **Full navigation bar**: Logo/title, search box, Compare link, About page link, links to API endpoints (GraphQL playground, REST docs, PeeringDB compat), dark mode toggle (sun/moon icon).
- **Footer**: Minimal — project info, GitHub link.

### Branding & Visual Identity
- **Color scheme**: Neon green on dark — terminal/hacker aesthetic, bold and modern with tech focus.
  - Background: neutral-900 (#171717)
  - Accent: emerald-500 (#10b981)
  - Text: neutral-100 (#f5f5f5)
  - Vibe: terminal, Matrix, htop — immediately distinct from PeeringDB's blue
- **Typography**: Monospace touches for data values (ASNs, IPs, speeds), sans-serif for UI text.
- **No logo for now**: Text-based "PeeringDB Plus" title.

## Codebase Integration Points

- `cmd/peeringdb-plus/main.go` lines 144-231: Route registration, middleware stack
- `internal/pdbcompat/handler.go`: Existing handler pattern to follow (`Handler` struct with `*ent.Client`, `Register(mux)` method)
- New package: `internal/web/` — mirrors pdbcompat pattern
- Static assets: htmx.min.js vendored, embedded via `//go:embed`. Tailwind via CDN (no local CSS file).

## Key Constraints

- Every handler must support dual rendering: full HTML page (direct navigation) vs. htmx fragment (`HX-Request` header detection)
- `Vary: HX-Request` response header required to prevent caching conflicts
- Readiness middleware must serve HTML "syncing" page for browser requests (currently returns JSON 503)
