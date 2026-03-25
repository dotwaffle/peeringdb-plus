# Phase 28: Terminal Detection & Infrastructure - Context

**Gathered:** 2026-03-25
**Status:** Ready for planning

<domain>
## Phase Boundary

Terminal clients (curl, wget, HTTPie) hitting any /ui/ URL receive appropriate text responses instead of HTML, with explicit format overrides available. This phase builds the detection, rendering framework, help text, and error pages — not the entity-specific renderers (Phase 29+).

</domain>

<decisions>
## Implementation Decisions

### Detection Priority Chain
- **D-01:** Priority order (highest first): Query param (`?format=`, `?T`) > Accept header (`text/plain`, `application/json`) > User-Agent prefix match (`curl/`, `Wget/`, `HTTPie/`, `xh/`, `PowerShell/`, `fetch`)
- **D-02:** Query params always win — explicit user intent overrides all implicit signals
- **D-03:** Accept header outranks User-Agent — standard HTTP content negotiation before UA sniffing
- **D-04:** `?nocolor` suppresses ANSI codes regardless of other detection signals (RND-18)

### Rendering Architecture
- **D-05:** Extend `renderPage()` in `internal/web/render.go` with a third branch: HTML (browser) / fragment (htmx) / terminal (text). No separate handler dispatch.
- **D-06:** Data structs (`templates.NetworkDetail`, etc.) are already populated before rendering — the terminal path receives the same structs and renders them as text instead of HTML
- **D-07:** Terminal rendering code lives in a new `internal/web/termrender/` package imported by `renderPage()`

### ANSI Rendering
- **D-08:** Use Charm's lipgloss/termenv library for styled text and table formatting. Not hand-rolled ANSI escapes.
- **D-09:** Map existing web UI Tailwind color tiers to 256-color ANSI equivalents (gray sub-1G, neutral 1G, blue 10G, emerald 100G, amber 400G+)
- **D-10:** Unicode box drawing characters for rich mode (`─│┌┐└┘├┤┬┴┼`), ASCII equivalents for plain text mode (`?T`)

### Help Text
- **D-11:** `curl /ui/` returns endpoint listing with curl examples, query parameter documentation, format options, and current data freshness timestamp
- **D-12:** Help text is ANSI-colored with sections in rich mode, plain text in `?T` mode
- **D-13:** Style inspired by wttr.in's help page — practical, example-driven

### Error Responses
- **D-14:** Terminal clients receive text-formatted 404 and 500 errors (not HTML)
- **D-15:** Error format detected by the same priority chain as normal responses

### Claude's Discretion
- Exact 256-color ANSI code mappings for each Tailwind color tier
- lipgloss style definitions and component structure
- Help text exact wording and example selection
- Error message wording

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Detection & Content Negotiation
- `.planning/REQUIREMENTS.md` — DET-01 through DET-05, RND-18 define detection and format override requirements

### Existing Code
- `internal/web/render.go` — Current `renderPage()` dual-render logic (HTML vs htmx fragment) — extend with third branch
- `internal/web/handler.go` — Route dispatcher, `dispatch()` method, `/ui/{rest...}` pattern
- `internal/web/templates/error.templ` — Existing HTML error pages (NotFoundPage, ServerErrorPage) — terminal equivalents needed
- `cmd/peeringdb-plus/main.go` — Root handler already does Accept-based content negotiation (lines 309-320), middleware chain

### Rendering Libraries
- lipgloss/termenv documentation — styling approach for ANSI output

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `renderPage()` in `internal/web/render.go` — already handles dual-render, natural extension point for third mode
- Root handler `GET /{$}` in `main.go` — already does Accept header content negotiation (text/html → redirect, else → JSON)
- `PageContent` struct — carries Title + Content, can carry terminal content too
- Data structs (`templates.NetworkDetail`, `templates.IXDetail`, etc.) — all populated before rendering, reusable as-is

### Established Patterns
- `HX-Request` header check for fragment vs full page — extend pattern with terminal detection
- `Vary` header set on responses — must add User-Agent and Accept to Vary for terminal responses
- Response writer wrapper in `middleware/logging.go` implements `Flush()` and `Unwrap()`

### Integration Points
- `renderPage()` is THE integration point — all detail handlers call it
- Error handlers (`handleNotFound`, `handleServerError`) need terminal-aware variants
- Middleware chain (Recovery → OTel → Logging → CORS → Readiness → mux) — readiness check already returns JSON for API clients

</code_context>

<specifics>
## Specific Ideas

- Help text style inspired by wttr.in — example-driven, shows curl commands users can copy-paste
- Detection must handle edge cases: curl with custom Accept headers, wget (no User-Agent customization), HTTPie's default Accept

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 28-terminal-detection-infrastructure*
*Context gathered: 2026-03-25*
