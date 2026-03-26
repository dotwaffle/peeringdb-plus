# Technology Stack: Terminal CLI Interface

**Project:** PeeringDB Plus - v1.8 Terminal CLI Interface
**Researched:** 2026-03-25
**Scope:** Stack additions needed for curl-friendly terminal output with rich ANSI rendering, content negotiation, and User-Agent detection.

## Context: What Already Exists (DO NOT add)

The project has 5 API surfaces served from a single HTTP mux. The web UI uses `templ` + `htmx` + Tailwind CSS with a `renderPage` function that already performs content negotiation between full-page and htmx fragment renders via `HX-Request` header. The `readinessMiddleware` in `main.go` already distinguishes browsers (Accept: text/html) from API clients for 503 responses. The root handler (`GET /{$}`) already does Accept-header content negotiation.

| Existing Dependency | Role in v1.8 |
|---------------------|-------------|
| `net/http` stdlib | HTTP handlers, mux routing, header inspection -- all detection and routing logic |
| `internal/web` package | Existing dispatch pattern in `handler.go` is the integration point for terminal rendering |
| `internal/web/render.go` | `renderPage` already switches on request headers; terminal rendering adds a third path |
| `github.com/a-h/templ` | NOT used for terminal output -- templ generates HTML, terminal output is plain ANSI text |
| `ent` client | Data queries remain identical; only the rendering layer changes |

## New Dependencies Required

### Terminal Rendering

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| `charm.land/lipgloss/v2` | v2.0.2 | Terminal styling (colors, borders, padding, alignment) | The dominant Go terminal styling library. v2 (stable, published 2026-03-11) has deterministic rendering, explicit I/O control, and does NOT require a real terminal -- styles produce strings containing ANSI escape codes that can be written to any `io.Writer` including `http.ResponseWriter`. Built-in border types include `NormalBorder()` (Unicode box drawing: `─│┌┐└┘`), `RoundedBorder()` (`╭╮╰╯`), `DoubleBorder()` (`═║╔╗╚╝`), and `ASCIIBorder()` (`-|+`) for maximum compatibility. Import path changed from `github.com/charmbracelet/lipgloss` to `charm.land/lipgloss/v2` in v2. | HIGH |
| `charm.land/lipgloss/v2/table` | v2.0.2 | Structured table rendering with per-cell styling | Sub-package of lipgloss providing table creation with `Headers()`, `Row()`, `StyleFunc()`, `Border()`, `Width()`, and `Wrap()` methods. Renders to string via `.Render()` or `.String()`. Supports `StyleFunc(func(row, col int) lipgloss.Style)` for per-cell color control (headers, alternating rows, highlighted values). No separate dependency -- same module as lipgloss/v2. | HIGH |
| `github.com/charmbracelet/colorprofile` | v0.4.3 | Explicit color profile selection for server-side rendering | Provides `Profile` constants (`ANSI256`, `TrueColor`, `ANSI`, `ASCII`, `NoTTY`) and a `Writer` type that automatically downgrades ANSI escape sequences. Critical for server-side use: we do NOT detect terminal capabilities (no terminal exists on the server); instead we explicitly set `Profile = colorprofile.ANSI256` for rich output or `Profile = colorprofile.ASCII` for plain text mode. The `NewWriter(w, environ)` function wraps any `io.Writer`. Published 2026-03-09. | HIGH |

### No Additional Dependencies Needed

These capabilities are covered by the three packages above plus stdlib:

| Capability | Provided By | Notes |
|-----------|------------|-------|
| User-Agent detection | `net/http` stdlib | Simple string prefix matching on `r.Header.Get("User-Agent")` -- no library needed |
| Content negotiation | `net/http` stdlib | Check `Accept` header and `?format=` query parameter -- no library needed |
| Unicode box drawing | lipgloss `NormalBorder()` / `RoundedBorder()` | Built into border definitions, no separate Unicode library |
| 256-color ANSI codes | lipgloss `Color("123")` + colorprofile `ANSI256` | lipgloss generates ANSI escape sequences, colorprofile ensures correct profile |
| Terminal width handling | Explicit `Width(80)` on tables | Server has no terminal to detect width from. Default to 80 columns (standard). Allow `?w=N` query parameter override. |
| Plain text fallback | colorprofile `ASCII` profile | Strips all ANSI codes automatically when profile is `ASCII` |
| JSON output | `encoding/json` stdlib | Already in use throughout the project |

## How It Works: Server-Side ANSI Rendering

The key insight: this is **server-side rendering** of ANSI escape codes, not a TUI. There is no terminal to detect capabilities from. The server generates ANSI-encoded strings and writes them to the HTTP response body. The user's terminal interprets the ANSI codes.

### Rendering Pipeline

```
HTTP Request
    |
    v
Content Negotiation (User-Agent + Accept + ?format= + ?T)
    |
    +--> Browser detected? --> Existing templ/htmx rendering (unchanged)
    |
    +--> Terminal detected, rich mode? --> lipgloss styles + table + colorprofile.ANSI256
    |                                      --> Write to ResponseWriter
    |
    +--> Terminal detected, plain mode (?T)? --> lipgloss + colorprofile.ASCII
    |                                            --> Strips all ANSI, Unicode borders become ASCII
    |
    +--> ?format=json? --> encoding/json (existing pattern)
```

### Color Profile Strategy

| Mode | Profile | Border Style | When |
|------|---------|-------------|------|
| Rich (default for terminals) | `colorprofile.ANSI256` | `lipgloss.RoundedBorder()` | Terminal client detected, no `?T` param |
| Plain text | `colorprofile.ASCII` | `lipgloss.ASCIIBorder()` | `?T` query param present, or `Accept: text/plain` |
| No color | `colorprofile.NoTTY` | `lipgloss.ASCIIBorder()` | Pipe detection hint or explicit `?nocolor` |

### Why ANSI 256-Color (Not TrueColor)

Use 256-color as the default profile, not TrueColor (24-bit), because:

1. **256-color is universally supported** by every modern terminal emulator (iTerm2, Terminal.app, GNOME Terminal, Windows Terminal, xterm-256color, tmux, screen). TrueColor support is widespread but not universal -- notably `screen` and some SSH session multiplexers strip 24-bit sequences.
2. **The PeeringDB data domain has limited color needs.** We need ~10-15 distinct colors for headers, entity types, status indicators, and port speed categories. 256 colors is more than sufficient.
3. **Smaller payload.** 256-color escape codes are shorter than TrueColor codes: `\x1b[38;5;123m` (13 bytes) vs `\x1b[38;2;107;80;255m` (19 bytes). Over a large table this adds up.
4. **lipgloss auto-downgrades.** Use `lipgloss.Color("#6a00ff")` or `lipgloss.CompleteColor{TrueColor: "#6a00ff", ANSI256: "57", ANSI: "5"}` and the colorprofile writer handles downsampling automatically.

### User-Agent Detection Pattern

Detect terminal clients via User-Agent prefix matching. This is the same pattern used by wttr.in, cheat.sh, and similar curl-friendly services.

| Client | Default User-Agent | Detection |
|--------|-------------------|-----------|
| curl | `curl/8.x.x` | `strings.HasPrefix(ua, "curl/")` |
| wget | `Wget/1.x` | `strings.HasPrefix(ua, "Wget/")` |
| HTTPie | `HTTPie/3.x` | `strings.HasPrefix(ua, "HTTPie/")` |
| PowerShell | `Mozilla/5.0 (Windows NT; ... PowerShell/...)` | `strings.Contains(ua, "PowerShell")` |
| fetch (Go) | `Go-http-client/1.1` | `strings.HasPrefix(ua, "Go-http-client/")` |
| libwww-perl | `libwww-perl/6.x` | `strings.HasPrefix(ua, "libwww-perl/")` |
| Python requests | `python-requests/2.x` | `strings.HasPrefix(ua, "python-requests/")` |

**Fallback logic:**
1. Check `?format=` query param first (explicit override: `json`, `text`, `plain`)
2. Check `Accept` header (`text/plain` or `application/json` prefers text/JSON)
3. Check User-Agent for known terminal clients
4. If User-Agent contains `Mozilla/` AND does not match PowerShell/etc, assume browser
5. Default: treat as terminal (safer -- terminals handle HTML poorly, browsers handle text fine)

### Content-Type Response Headers

| Output Mode | Content-Type |
|-------------|-------------|
| Rich ANSI | `text/plain; charset=utf-8` |
| Plain text | `text/plain; charset=utf-8` |
| JSON | `application/json; charset=utf-8` |
| HTML (browser) | `text/html; charset=utf-8` (existing) |

Note: There is no standard MIME type for ANSI-encoded text. `text/plain; charset=utf-8` is correct and is what wttr.in uses. Terminals interpret ANSI codes embedded in the text stream; the Content-Type does not need to signal this.

## Integration Points

### Where Terminal Rendering Hooks In

The integration point is `internal/web/handler.go` dispatch and `internal/web/render.go`. The existing pattern:

```go
// Current: 2-way negotiation (browser vs htmx fragment)
func renderPage(ctx context.Context, w http.ResponseWriter, r *http.Request, page PageContent) error {
    if r.Header.Get("HX-Request") == "true" {
        return page.Content.Render(ctx, w) // htmx fragment
    }
    return templates.Layout(page.Title, page.Content).Render(ctx, w) // full page
}
```

Becomes 3-way negotiation:

```go
// v1.8: 3-way negotiation (terminal vs htmx fragment vs full page)
// Terminal check happens FIRST -- before any templ rendering
if isTerminalClient(r) {
    renderTerminal(ctx, w, r, terminalData) // lipgloss rendering
    return
}
// ... existing htmx/browser paths unchanged
```

The terminal rendering path is entirely separate from templ -- it writes plain text with ANSI codes, not HTML.

### New Package Structure

```
internal/
  terminal/           # NEW - terminal rendering package
    detect.go         # User-Agent detection, content negotiation
    render.go         # Core rendering functions using lipgloss
    styles.go         # Color palette, style definitions
    network.go        # Network entity terminal renderer
    ix.go             # IX entity terminal renderer
    facility.go       # Facility entity terminal renderer
    org.go            # Organization entity terminal renderer
    campus.go         # Campus entity terminal renderer
    carrier.go        # Carrier entity terminal renderer
    help.go           # CLI help text renderer
    error.go          # Terminal-formatted error responses
```

### Vary Header

Responses from `/ui/*` endpoints MUST include `Vary: User-Agent, Accept` to prevent CDN/proxy caching conflicts between browser HTML and terminal ANSI output. The existing `Vary: HX-Request` must be extended:

```go
w.Header().Set("Vary", "HX-Request, User-Agent, Accept")
```

## Alternatives Considered

| Category | Recommended | Alternative | Why Not |
|----------|-------------|-------------|---------|
| Terminal styling | lipgloss v2 | Raw ANSI escape codes | Raw codes are error-prone, no auto-downsampling, no layout utilities. lipgloss provides borders, padding, alignment, width control, and color profile management. The styling API is CSS-like and maintainable. |
| Terminal styling | lipgloss v2 | muesli/termenv | termenv is the low-level layer under lipgloss. It handles color profiles and ANSI output but has no layout, borders, or table support. lipgloss builds on termenv and adds everything we need. Using termenv directly would mean reimplementing lipgloss. |
| Table rendering | lipgloss/table | jedib0t/go-pretty v6 | go-pretty (v6.7.8) is feature-rich but heavy -- includes progress bars, lists, and text utilities we do not need. Its color API uses its own `text.Color` type, not standard `color.Color` or lipgloss styles. lipgloss/table integrates natively with lipgloss styles, borders, and color profiles. One ecosystem, not two. |
| Table rendering | lipgloss/table | olekukonko/tablewriter v1 | tablewriter (v1.1.4) is popular but has a different styling model (method chaining vs style functions) and does not integrate with lipgloss color profiles. Using two separate styling systems creates inconsistency between table headers and standalone styled text. |
| Table rendering | lipgloss/table | rodaine/table | Minimal, no ANSI color support built-in. |
| Color profile | charmbracelet/colorprofile | Manual ANSI code selection | colorprofile provides automatic downsampling (TrueColor to 256 to 16 to ASCII) via a Writer wrapper. Manual selection means maintaining parallel color definitions for each profile. |
| Content negotiation | Stdlib header checks | jchannon/negotiator library | The negotiation logic is 20 lines of User-Agent prefix checks and Accept header parsing. A library adds a dependency for trivial logic. The existing codebase already does this pattern in `main.go` and `readinessMiddleware`. |
| Unicode box drawing | lipgloss built-in borders | Manual Unicode characters | lipgloss `NormalBorder()`, `RoundedBorder()`, `DoubleBorder()` already define all needed box-drawing characters with proper corner/intersection pieces. No benefit to hand-crafting them. |

## Installation

```bash
# Terminal rendering (lipgloss v2 includes table sub-package)
go get charm.land/lipgloss/v2@v2.0.2

# Color profile management
go get github.com/charmbracelet/colorprofile@v0.4.3

# No other new dependencies needed
```

## Version Pinning Strategy

| Package | Pin Strategy | Notes |
|---------|-------------|-------|
| `charm.land/lipgloss/v2` | Pin to v2.0.x | Stable v2 release. Module path changed from `github.com/charmbracelet/lipgloss` to `charm.land/lipgloss/v2`. The lipgloss/table sub-package is part of the same module. |
| `github.com/charmbracelet/colorprofile` | Pin to v0.4.x | Pre-1.0 but actively maintained by Charm. API surface we use (Profile constants, NewWriter) is stable. Published 2026-03-09. |

## Risk Register

| Risk | Severity | Likelihood | Mitigation |
|------|----------|------------|------------|
| lipgloss v2 import path confusion (`charm.land` vs `github.com`) | LOW | MEDIUM | The v2 module uses `charm.land/lipgloss/v2`. The old `github.com/charmbracelet/lipgloss` path is v1 only. Document import path clearly. |
| colorprofile is pre-1.0 (v0.4.x) | LOW | LOW | We use 3 things: Profile constants, NewWriter, and Profile assignment. These are stable across all 0.x releases. If API changes, the migration is trivial. |
| User-Agent spoofing/false detection | LOW | MEDIUM | Users who override User-Agent get the wrong output format. Provide explicit `?format=text` and `?format=html` overrides that take precedence over User-Agent detection. |
| Unicode box-drawing characters display incorrectly | LOW | LOW | Provide `?T` plain text mode that uses `ASCIIBorder()` fallback. Modern terminals (post-2010) universally support Unicode box drawing. Windows Terminal, iTerm2, GNOME Terminal, and even PuTTY handle them correctly. |
| 256-color not supported in user's terminal | LOW | LOW | Extremely rare in 2026. colorprofile's Writer auto-downgrades to 16-color ANSI if needed. Plain text mode (`?T`) strips all color. |
| ANSI output cached by CDN/proxy as if it were HTML | MEDIUM | MEDIUM | Set `Vary: User-Agent, Accept, HX-Request` on all `/ui/*` responses. Add `Cache-Control: private` for terminal responses to prevent shared caching. |
| Large table output for entities with many presences | LOW | LOW | lipgloss table rendering is string-based (in-memory). For the largest PeeringDB entities (~2000 IX presences), this is <100KB of ANSI text. Not a memory concern. |

## Sources

- [charm.land/lipgloss/v2 on pkg.go.dev](https://pkg.go.dev/charm.land/lipgloss/v2) - v2.0.2 published 2026-03-11, MIT license
- [charm.land/lipgloss/v2/table on pkg.go.dev](https://pkg.go.dev/charm.land/lipgloss/v2/table) - Table sub-package API
- [Lip Gloss v2: What's New](https://github.com/charmbracelet/lipgloss/discussions/506) - v2 migration guide, deterministic rendering, I/O changes
- [lipgloss GitHub releases](https://github.com/charmbracelet/lipgloss/releases) - v2.0.2 changelog
- [charmbracelet/colorprofile on pkg.go.dev](https://pkg.go.dev/github.com/charmbracelet/colorprofile) - v0.4.3 published 2026-03-09
- [colorprofile source](https://github.com/charmbracelet/colorprofile/blob/main/profile.go) - Profile constants and Writer implementation
- [jedib0t/go-pretty on GitHub](https://github.com/jedib0t/go-pretty) - v6.7.8 alternative considered
- [olekukonko/tablewriter on GitHub](https://github.com/olekukonko/tablewriter) - v1.1.4 alternative considered
- [wttr.in source on GitHub](https://github.com/chubin/wttr.in) - Reference implementation for curl-friendly HTTP service with User-Agent detection
- [User-Agent detection gist](https://gist.github.com/nahakiole/843fb9a29292bfcf012b) - curl/wget detection pattern
- [curl User-Agent docs](https://everything.curl.dev/http/modify/user-agent.html) - Default `curl/VERSION` format
- [lipgloss borders.go](https://github.com/charmbracelet/lipgloss/blob/master/borders.go) - Unicode box-drawing character definitions
