# Phase 28: Terminal Detection & Infrastructure - Research

**Researched:** 2026-03-25
**Domain:** HTTP content negotiation, terminal text rendering, ANSI color output
**Confidence:** HIGH

## Summary

Phase 28 adds a third rendering branch to `renderPage()` in `internal/web/render.go`. When a terminal client (curl, wget, HTTPie) requests any `/ui/` URL, the server returns styled text output instead of HTML. Detection uses a priority chain: query params > Accept header > User-Agent. The rendering framework uses Charm's lipgloss v2 for styled ANSI text and table formatting, with colorprofile to control color depth for different output modes (rich 256-color, plain ASCII, no-color).

The existing codebase is well-prepared for this extension. `renderPage()` already branches on `HX-Request` for htmx fragments, and all detail handlers populate data structs (`templates.NetworkDetail`, `templates.IXDetail`, etc.) before calling `renderPage()`. The terminal path receives the same structs and renders them as text. No handler changes are needed -- only `renderPage()` and the new `internal/web/termrender/` package.

**Primary recommendation:** Extend `renderPage()` with terminal detection before the HX-Request check. Create `internal/web/termrender/` package with a `Renderer` type that accepts a `RenderMode` (Rich/Plain/JSON) and renders data structs to `io.Writer`. Use lipgloss v2 `table.New()` for tables and `lipgloss.NewStyle()` for styled text. Control ANSI output via `colorprofile.Writer` with forced profile.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Priority order (highest first): Query param (`?format=`, `?T`) > Accept header (`text/plain`, `application/json`) > User-Agent prefix match (`curl/`, `Wget/`, `HTTPie/`, `xh/`, `PowerShell/`, `fetch`)
- **D-02:** Query params always win -- explicit user intent overrides all implicit signals
- **D-03:** Accept header outranks User-Agent -- standard HTTP content negotiation before UA sniffing
- **D-04:** `?nocolor` suppresses ANSI codes regardless of other detection signals (RND-18)
- **D-05:** Extend `renderPage()` in `internal/web/render.go` with a third branch: HTML (browser) / fragment (htmx) / terminal (text). No separate handler dispatch.
- **D-06:** Data structs (`templates.NetworkDetail`, etc.) are already populated before rendering -- the terminal path receives the same structs and renders them as text instead of HTML
- **D-07:** Terminal rendering code lives in a new `internal/web/termrender/` package imported by `renderPage()`
- **D-08:** Use Charm's lipgloss/termenv library for styled text and table formatting. Not hand-rolled ANSI escapes.
- **D-09:** Map existing web UI Tailwind color tiers to 256-color ANSI equivalents (gray sub-1G, neutral 1G, blue 10G, emerald 100G, amber 400G+)
- **D-10:** Unicode box drawing characters for rich mode, ASCII equivalents for plain text mode (`?T`)
- **D-11:** `curl /ui/` returns endpoint listing with curl examples, query parameter documentation, format options, and current data freshness timestamp
- **D-12:** Help text is ANSI-colored with sections in rich mode, plain text in `?T` mode
- **D-13:** Style inspired by wttr.in's help page -- practical, example-driven
- **D-14:** Terminal clients receive text-formatted 404 and 500 errors (not HTML)
- **D-15:** Error format detected by the same priority chain as normal responses

### Claude's Discretion
- Exact 256-color ANSI code mappings for each Tailwind color tier
- lipgloss style definitions and component structure
- Help text exact wording and example selection
- Error message wording

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| DET-01 | Terminal clients (curl, wget, HTTPie, xh, PowerShell, fetch) auto-detected via User-Agent prefix matching | UA prefix list verified against wttr.in patterns. Detection function in `termrender/detect.go`. |
| DET-02 | User can force plain text via ?T or ?format=plain query parameter | Query param check is first in priority chain. Plain mode uses colorprofile.ASCII to strip ANSI. |
| DET-03 | User can force JSON via ?format=json query parameter | JSON mode uses `encoding/json` marshal of data structs. No lipgloss involved. |
| DET-04 | Accept header (text/plain, application/json) serves as secondary format signal | Standard HTTP content negotiation. Check after query params, before UA. |
| DET-05 | Content negotiation applies to all /ui/ paths -- browsers get HTML unchanged | Detection runs in `renderPage()` before htmx check. Browser UA does not match terminal prefixes. |
| RND-01 | Rich 256-color ANSI output with Unicode box-drawing for terminal clients | lipgloss v2 `table.New().Border(lipgloss.NormalBorder())` with 256-color styles. |
| RND-18 | NO_COLOR convention respected -- suppress ANSI codes when ?nocolor param present | `?nocolor` forces colorprofile.ASCII profile, stripping all ANSI from output. |
| NAV-01 | Help text at /ui/ for terminal clients listing endpoints, params, and examples | Help text renderer in `termrender/help.go`. Style: wttr.in-inspired sections with curl examples. |
| NAV-02 | Text-formatted 404 error for terminal clients (not HTML) | Error renderer in `termrender/error.go`. Returns plain/ANSI "404 Not Found" with suggestion text. |
| NAV-03 | Text-formatted 500 error for terminal clients (not HTML) | Error renderer in `termrender/error.go`. Returns plain/ANSI "500 Internal Server Error". |
| NAV-04 | Root handler (/) returns help text for terminal clients (not redirect) | Modify root `GET /{$}` handler in `main.go` to detect terminal clients and return help text instead of redirect. |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **CS-5 (MUST):** Input structs for functions with >2 arguments. The new `RenderInput` struct must bundle render parameters.
- **CS-6 (SHOULD):** Declare function input structs before the consuming function.
- **ERR-1 (MUST):** Wrap errors with `%w` and context.
- **OBS-1 (MUST):** Structured slog logging with levels and consistent fields.
- **T-1 (MUST):** Table-driven tests, deterministic and hermetic.
- **T-2 (MUST):** Run `-race` in CI; add `t.Cleanup` for teardown.
- **T-3 (SHOULD):** Mark safe tests with `t.Parallel()`.
- **API-1 (MUST):** Document exported items: `// Foo does ...`; keep exported surface minimal.
- **API-2 (MUST):** Accept interfaces where variation is needed; return concrete types.
- **MD-1 (SHOULD):** Prefer stdlib; introduce deps only with clear payoff.
- **Middleware convention:** Response writer wrappers MUST implement `http.Flusher` and `Unwrap()`.
- **Vary header:** Must add `User-Agent` and `Accept` to Vary for terminal responses (noted in STATE.md).

## Standard Stack

### Core (New Dependencies)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| charm.land/lipgloss/v2 | v2.0.2 | Terminal text styling and table rendering | Locked decision D-08. Published 2026-03-11. 542 importers. Provides Style, table, list sub-packages. MIT license. Import path is vanity domain `charm.land/lipgloss/v2` (not github.com). |
| github.com/charmbracelet/colorprofile | v0.4.3 | Color profile detection and ANSI downsampling | Transitive dependency of lipgloss v2. Used directly to force color profiles for HTTP output (non-TTY). Provides Writer with Profile override. Pre-1.0 but minimal API surface. |

### Existing (Already in go.mod)

| Library | Version | Purpose | Relevance |
|---------|---------|---------|-----------|
| encoding/json (stdlib) | Go 1.26 | JSON serialization | Used for `?format=json` output mode (DET-03) |
| net/http (stdlib) | Go 1.26 | HTTP request/response | Query params, headers, User-Agent access |
| log/slog (stdlib) | Go 1.26 | Structured logging | Log terminal detection decisions for debugging |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| lipgloss v2 | fatih/color + tablewriter | fatih/color is simpler but no table support, no list/tree rendering, no built-in color downsampling. lipgloss is locked decision D-08. |
| lipgloss v2 | Hand-rolled ANSI escapes | Explicitly prohibited by D-08. Error-prone, no box-drawing table support. |
| colorprofile | Manual ANSI stripping regex | colorprofile handles all edge cases (nested escapes, cursor codes). Comes free with lipgloss. |

**Installation:**
```bash
TMPDIR=/tmp/claude-1000 GONOSUMCHECK='*' GONOSUMDB='*' go get charm.land/lipgloss/v2@v2.0.2
```

**Version verification:**
- lipgloss v2.0.2: confirmed via `go list -m charm.land/lipgloss/v2@latest` (published 2026-03-11)
- colorprofile v0.4.3: confirmed via `go list -m github.com/charmbracelet/colorprofile@latest`

## Architecture Patterns

### Recommended Project Structure

```
internal/web/
  render.go              # renderPage() -- add terminal branch + detection
  termrender/
    detect.go            # Terminal detection: isTerminal(), parseFormat()
    detect_test.go       # Table-driven detection tests
    renderer.go          # Renderer type, RenderMode enum, style definitions
    renderer_test.go     # Renderer unit tests
    help.go              # Help text rendering (NAV-01)
    help_test.go         # Help text tests
    error.go             # Error page rendering (NAV-02, NAV-03)
    error_test.go        # Error page tests
    styles.go            # lipgloss style constants (colors, borders)
    table.go             # Shared table rendering utilities
```

### Pattern 1: Detection Priority Chain

**What:** A function that examines the HTTP request and returns a `RenderMode` enum indicating how to render the response.

**When to use:** Called at the top of `renderPage()` before any rendering decision.

```go
// internal/web/termrender/detect.go

// RenderMode describes the output format for a response.
type RenderMode int

const (
    // ModeHTML renders the standard web UI page.
    ModeHTML RenderMode = iota
    // ModeHTMX renders an htmx fragment (no layout shell).
    ModeHTMX
    // ModeRich renders ANSI-colored terminal output with Unicode box drawing.
    ModeRich
    // ModePlain renders plain ASCII text with no ANSI codes.
    ModePlain
    // ModeJSON renders the data as JSON.
    ModeJSON
)

// DetectInput holds parameters for detecting the render mode.
type DetectInput struct {
    Query     url.Values
    Accept    string
    UserAgent string
    HXRequest bool
}

// Detect returns the appropriate render mode based on the priority chain:
// query params > Accept header > User-Agent > default (HTML).
func Detect(input DetectInput) RenderMode {
    // 1. Query param overrides (highest priority)
    if _, ok := input.Query["T"]; ok {
        return ModePlain
    }
    switch input.Query.Get("format") {
    case "plain":
        return ModePlain
    case "json":
        return ModeJSON
    }

    // 2. Accept header (secondary)
    if strings.Contains(input.Accept, "text/plain") {
        return ModeRich
    }
    if strings.Contains(input.Accept, "application/json") {
        return ModeJSON
    }

    // 3. User-Agent prefix match (tertiary)
    if isTerminalUA(input.UserAgent) {
        return ModeRich
    }

    // 4. HX-Request (htmx fragment)
    if input.HXRequest {
        return ModeHTMX
    }

    return ModeHTML
}

// terminalPrefixes are User-Agent prefixes that identify terminal/CLI clients.
// Sourced from wttr.in's PLAIN_TEXT_AGENTS and user decisions (D-01).
var terminalPrefixes = []string{
    "curl/",
    "Wget/",
    "HTTPie/",
    "xh/",
    "PowerShell/",
    "fetch",
}

// isTerminalUA checks whether the User-Agent identifies a terminal client.
func isTerminalUA(ua string) bool {
    lower := strings.ToLower(ua)
    for _, prefix := range terminalPrefixes {
        if strings.HasPrefix(lower, strings.ToLower(prefix)) {
            return true
        }
    }
    return false
}
```

### Pattern 2: Extended renderPage() with Terminal Branch

**What:** Modify `renderPage()` to check for terminal mode before htmx/HTML rendering.

**When to use:** Every request through the web UI handler.

```go
// internal/web/render.go (modified)

func renderPage(ctx context.Context, w http.ResponseWriter, r *http.Request, page PageContent) error {
    mode := termrender.Detect(termrender.DetectInput{
        Query:     r.URL.Query(),
        Accept:    r.Header.Get("Accept"),
        UserAgent: r.Header.Get("User-Agent"),
        HXRequest: r.Header.Get("HX-Request") == "true",
    })

    // Check for ?nocolor override (applies to terminal modes only).
    noColor := r.URL.Query().Has("nocolor")

    switch mode {
    case termrender.ModeRich, termrender.ModePlain:
        w.Header().Set("Vary", "HX-Request, User-Agent, Accept")
        w.Header().Set("Content-Type", "text/plain; charset=utf-8")
        renderer := termrender.NewRenderer(mode, noColor)
        return renderer.RenderPage(w, page)

    case termrender.ModeJSON:
        w.Header().Set("Vary", "HX-Request, User-Agent, Accept")
        w.Header().Set("Content-Type", "application/json; charset=utf-8")
        return termrender.RenderJSON(w, page)

    case termrender.ModeHTMX:
        w.Header().Set("Vary", "HX-Request, User-Agent, Accept")
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        return page.Content.Render(ctx, w)

    default: // ModeHTML
        w.Header().Set("Vary", "HX-Request, User-Agent, Accept")
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        return templates.Layout(page.Title, page.Content).Render(ctx, w)
    }
}
```

### Pattern 3: Renderer with Forced Color Profile

**What:** A `Renderer` that renders terminal output and forces the correct colorprofile for HTTP responses.

**When to use:** For all terminal text rendering. HTTP response writers are not TTYs, so colorprofile auto-detection would strip all ANSI. We must force the profile.

```go
// internal/web/termrender/renderer.go

// Renderer produces terminal text output.
type Renderer struct {
    mode    RenderMode
    noColor bool
    writer  *colorprofile.Writer
}

// NewRenderer creates a terminal renderer with the appropriate color profile.
func NewRenderer(mode RenderMode, noColor bool) *Renderer {
    r := &Renderer{mode: mode, noColor: noColor}
    return r
}

// Write renders styled text to the given writer, applying the correct
// color profile based on mode and noColor settings.
func (r *Renderer) Write(w io.Writer, content string) error {
    cw := &colorprofile.Writer{Forward: w}
    switch {
    case r.noColor || r.mode == ModePlain:
        cw.Profile = colorprofile.ASCII
    case r.mode == ModeRich:
        cw.Profile = colorprofile.ANSI256
    default:
        cw.Profile = colorprofile.NoTTY
    }
    _, err := cw.WriteString(content)
    return err
}
```

**Key insight:** Since `http.ResponseWriter` is not a TTY, `colorprofile.Detect()` would always return `NoTTY` and strip all ANSI. Instead, we:
1. Render everything to string using lipgloss (which generates full ANSI256 escape codes)
2. Pipe through `colorprofile.Writer` with a forced profile to downsample/strip as needed

### Pattern 4: lipgloss Style Definitions

**What:** Centralized style constants mapping Tailwind color tiers to ANSI 256.

```go
// internal/web/termrender/styles.go

// Color tier mapping from web UI Tailwind classes to ANSI 256 colors.
// D-09: gray sub-1G, neutral 1G, blue 10G, emerald 100G, amber 400G+.
var (
    // Port speed tier colors (ANSI 256 codes).
    ColorSpeedSub1G = lipgloss.Color("245")  // gray-400 equivalent
    ColorSpeed1G    = lipgloss.Color("250")  // neutral-300 equivalent
    ColorSpeed10G   = lipgloss.Color("33")   // blue-500 equivalent
    ColorSpeed100G  = lipgloss.Color("42")   // emerald-500 equivalent
    ColorSpeed400G  = lipgloss.Color("214")  // amber-500 equivalent

    // Semantic colors.
    ColorHeading    = lipgloss.Color("42")   // emerald
    ColorLabel      = lipgloss.Color("245")  // gray
    ColorValue      = lipgloss.Color("255")  // bright white
    ColorLink       = lipgloss.Color("33")   // blue (for URLs/cross-refs)
    ColorError      = lipgloss.Color("196")  // red
    ColorWarning    = lipgloss.Color("214")  // amber
    ColorSuccess    = lipgloss.Color("42")   // green/emerald
    ColorMuted      = lipgloss.Color("240")  // dim gray

    // Peering policy colors (D-09 discretion area).
    ColorPolicyOpen        = lipgloss.Color("42")   // green
    ColorPolicySelective   = lipgloss.Color("214")  // yellow
    ColorPolicyRestrictive = lipgloss.Color("196")  // red

    // Style definitions.
    StyleHeading = lipgloss.NewStyle().Bold(true).Foreground(ColorHeading)
    StyleLabel   = lipgloss.NewStyle().Foreground(ColorLabel)
    StyleValue   = lipgloss.NewStyle().Foreground(ColorValue)
    StyleMuted   = lipgloss.NewStyle().Foreground(ColorMuted)
    StyleError   = lipgloss.NewStyle().Bold(true).Foreground(ColorError)
    StyleLink    = lipgloss.NewStyle().Foreground(ColorLink).Underline(true)
)

// TableBorder returns the appropriate border style for the render mode.
// D-10: Unicode box drawing for rich mode, ASCII for plain.
func TableBorder(mode RenderMode) lipgloss.Border {
    if mode == ModePlain {
        return lipgloss.ASCIIBorder()
    }
    return lipgloss.NormalBorder()
}
```

### Pattern 5: Help Text Rendering (NAV-01)

**What:** Help text displayed when terminal clients hit `/ui/` with no specific path.

```go
// internal/web/termrender/help.go

// RenderHelp writes the terminal help text listing available endpoints,
// query parameters, format options, and usage examples.
// D-11: curl examples, query param docs, format options, freshness timestamp.
// D-13: Style inspired by wttr.in -- practical, example-driven.
func (r *Renderer) RenderHelp(w io.Writer, freshness time.Time) error {
    // Build help text using lipgloss styles, then pipe through
    // colorprofile.Writer to apply ANSI mode.
    var buf strings.Builder

    // Title section
    buf.WriteString(StyleHeading.Render("PeeringDB Plus"))
    buf.WriteString(" - Terminal Interface\n\n")

    // Usage examples section
    buf.WriteString(StyleHeading.Render("Usage:"))
    buf.WriteString("\n")
    buf.WriteString("  $ curl peeringdb-plus.fly.dev/ui/asn/13335\n")
    buf.WriteString("  $ curl peeringdb-plus.fly.dev/ui/ix/31\n")
    // ... more examples

    // Render through color profile writer
    return r.Write(w, buf.String())
}
```

### Anti-Patterns to Avoid

- **Anti-pattern: Rendering ANSI in templ templates.** Do not create `.templ` files for terminal output. Templ is for HTML. Terminal rendering uses Go string building with lipgloss.
- **Anti-pattern: Passing http.ResponseWriter to lipgloss.Fprint directly.** lipgloss.Fprint auto-detects TTY and would strip all colors since HTTP responses are not TTYs. Always use colorprofile.Writer with a forced profile.
- **Anti-pattern: Checking User-Agent with exact string match.** User-Agent strings include version numbers (`curl/8.5.0`). Always use prefix matching, case-insensitive.
- **Anti-pattern: Modifying handler dispatch.** Per D-05, detection happens in `renderPage()`, not in `dispatch()` or individual handlers. Handlers remain unchanged.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| ANSI escape code generation | Manual `\x1b[38;5;42m` sequences | `lipgloss.NewStyle().Foreground(lipgloss.Color("42"))` | Locked decision D-08. lipgloss handles reset codes, nesting, and edge cases. |
| Table formatting | Manual column alignment with fmt.Sprintf | `charm.land/lipgloss/v2/table` | Table package handles Unicode width, column auto-sizing, word wrap, border styles, and cell styling. |
| ANSI code stripping | Regex `\x1b\[[0-9;]*m` | `colorprofile.Writer{Profile: colorprofile.ASCII}` | colorprofile handles ALL escape types (colors, cursor, OSC), not just SGR. Regex approaches miss edge cases. |
| Content negotiation | Custom parsing of Accept header | Simple `strings.Contains()` for `text/plain`, `application/json` | Full RFC 7231 negotiation (q-values, wildcards) is overkill for two content types. Simple prefix check suffices. |
| Color downsampling | Manual 256-to-16 color mapping | `colorprofile.Writer` with forced profile | The Writer handles all color depth conversions correctly. |

**Key insight:** lipgloss v2 + colorprofile together handle the entire ANSI rendering pipeline. The only custom code needed is detection logic and content-specific renderers.

## Common Pitfalls

### Pitfall 1: colorprofile Auto-Detection Strips ANSI for HTTP Responses
**What goes wrong:** Using `lipgloss.Fprint(httpResponseWriter, ...)` or `colorprofile.NewWriter(httpResponseWriter, os.Environ())` results in `NoTTY` profile because HTTP response writers are not TTYs. All ANSI is stripped.
**Why it happens:** colorprofile checks `isatty()` on the writer. `http.ResponseWriter` always fails this check.
**How to avoid:** Create `colorprofile.Writer` with `Forward` field set to the HTTP response writer and `Profile` forced to `ANSI256` for rich mode or `ASCII` for plain mode. Never rely on auto-detection for HTTP output.
**Warning signs:** Terminal output appears as plain text even when ANSI was expected. No color codes in response body.

### Pitfall 2: Vary Header Must Include User-Agent
**What goes wrong:** Shared HTTP caches serve the HTML version to curl users or vice versa.
**Why it happens:** Without `Vary: User-Agent`, caches consider requests with different User-Agents as equivalent.
**How to avoid:** Set `Vary: HX-Request, User-Agent, Accept` on all `/ui/` responses. Already noted in STATE.md as accepted tradeoff (effectively disables shared caching, but no CDN layer exists).
**Warning signs:** curl returning HTML fragments, or browsers getting ANSI text.

### Pitfall 3: HX-Request Check Must Come After Terminal Detection
**What goes wrong:** An htmx fragment request from a terminal client (unlikely but possible with custom headers) gets HTML instead of text.
**Why it happens:** If HX-Request is checked first in the priority chain, it overrides terminal detection.
**How to avoid:** In `renderPage()`, run `termrender.Detect()` which includes HXRequest as the lowest-priority signal. Terminal query params and UA override HX-Request.
**Warning signs:** Tests that set both `HX-Request: true` and `User-Agent: curl/8.0` returning HTML.

### Pitfall 4: lipgloss v2 Import Path is Vanity Domain
**What goes wrong:** `go get github.com/charmbracelet/lipgloss/v2` may not resolve correctly or may pull a different module.
**Why it happens:** lipgloss v2 uses `charm.land/lipgloss/v2` as its module path (vanity domain), not the GitHub path.
**How to avoid:** Always use `go get charm.land/lipgloss/v2@v2.0.2`. The import in Go files must be `charm.land/lipgloss/v2`.
**Warning signs:** `go mod tidy` errors, "module declares its path as charm.land/lipgloss/v2" errors.

### Pitfall 5: ?T vs ?format=plain Semantic Equivalence
**What goes wrong:** `?T` and `?format=plain` produce different output.
**Why it happens:** Implementing them as separate code paths that diverge.
**How to avoid:** Both resolve to `ModePlain` in the detection function. Single code path for plain rendering.
**Warning signs:** Tests comparing `?T` output with `?format=plain` output showing differences.

### Pitfall 6: PageContent Struct Needs Terminal Data Carrier
**What goes wrong:** `renderPage()` receives `PageContent{Title, Content templ.Component}` but terminal rendering needs the raw data struct, not a templ component.
**Why it happens:** The current `PageContent` wraps data in a templ component before `renderPage()` sees it.
**How to avoid:** Add a `Data any` field to `PageContent` that carries the raw data struct (e.g., `templates.NetworkDetail`). Detail handlers set both `Content` (for HTML) and `Data` (for terminal). `renderPage()` passes `Data` to the terminal renderer when in terminal mode.
**Warning signs:** Terminal renderer receiving nil data, or needing to reverse-engineer data from templ components.

## Code Examples

### Example 1: Detection Function with Table-Driven Tests

```go
// internal/web/termrender/detect_test.go
func TestDetect(t *testing.T) {
    t.Parallel()
    tests := []struct {
        name string
        input DetectInput
        want  RenderMode
    }{
        {"curl UA defaults to rich", DetectInput{UserAgent: "curl/8.5.0"}, ModeRich},
        {"wget UA defaults to rich", DetectInput{UserAgent: "Wget/1.21"}, ModeRich},
        {"httpie UA defaults to rich", DetectInput{UserAgent: "HTTPie/3.2.4"}, ModeRich},
        {"xh UA defaults to rich", DetectInput{UserAgent: "xh/0.22.2"}, ModeRich},
        {"powershell UA", DetectInput{UserAgent: "PowerShell/7.4"}, ModeRich},
        {"browser UA returns HTML", DetectInput{UserAgent: "Mozilla/5.0"}, ModeHTML},
        {"?T forces plain", DetectInput{
            Query: url.Values{"T": {""}}, UserAgent: "curl/8.5.0",
        }, ModePlain},
        {"?format=plain forces plain", DetectInput{
            Query: url.Values{"format": {"plain"}}, UserAgent: "Mozilla/5.0",
        }, ModePlain},
        {"?format=json forces JSON", DetectInput{
            Query: url.Values{"format": {"json"}}, UserAgent: "Mozilla/5.0",
        }, ModeJSON},
        {"Accept text/plain from browser", DetectInput{
            Accept: "text/plain", UserAgent: "Mozilla/5.0",
        }, ModeRich},
        {"Accept application/json", DetectInput{
            Accept: "application/json", UserAgent: "Mozilla/5.0",
        }, ModeJSON},
        {"query param overrides Accept", DetectInput{
            Query: url.Values{"format": {"plain"}},
            Accept: "application/json", UserAgent: "curl/8.5.0",
        }, ModePlain},
        {"Accept overrides UA", DetectInput{
            Accept: "application/json", UserAgent: "curl/8.5.0",
        }, ModeJSON},
        {"HX-Request yields htmx fragment", DetectInput{
            HXRequest: true, UserAgent: "Mozilla/5.0",
        }, ModeHTMX},
        {"empty request defaults to HTML", DetectInput{}, ModeHTML},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
            if tt.input.Query == nil {
                tt.input.Query = url.Values{}
            }
            got := Detect(tt.input)
            if got != tt.want {
                t.Errorf("Detect(%+v) = %v, want %v", tt.input, got, tt.want)
            }
        })
    }
}
```

### Example 2: Rendering Styled Text Through colorprofile.Writer

```go
// Rendering ANSI text and piping through colorprofile for HTTP output.
import (
    lipgloss "charm.land/lipgloss/v2"
    "github.com/charmbracelet/colorprofile"
)

// renderStyledText demonstrates rendering styled content to an HTTP response.
func renderStyledText(w http.ResponseWriter, mode RenderMode, noColor bool) {
    heading := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
    content := heading.Render("AS13335 - Cloudflare, Inc.")

    // Force the appropriate color profile for HTTP output.
    cw := &colorprofile.Writer{Forward: w}
    switch {
    case noColor || mode == ModePlain:
        cw.Profile = colorprofile.ASCII
    default:
        cw.Profile = colorprofile.ANSI256
    }
    cw.WriteString(content + "\n")
}
```

### Example 3: lipgloss Table for IX Presence List

```go
import (
    lipgloss "charm.land/lipgloss/v2"
    "charm.land/lipgloss/v2/table"
)

func renderIXTable(rows []templates.NetworkIXLanRow, mode RenderMode) string {
    headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))

    t := table.New().
        Border(TableBorder(mode)).
        BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("240"))).
        Headers("Exchange", "Speed", "IPv4", "IPv6", "RS").
        StyleFunc(func(row, col int) lipgloss.Style {
            if row == table.HeaderRow {
                return headerStyle
            }
            return lipgloss.NewStyle()
        })

    for _, r := range rows {
        rs := ""
        if r.IsRSPeer {
            rs = "[RS]"
        }
        t.Row(r.IXName, formatSpeed(r.Speed), r.IPAddr4, r.IPAddr6, rs)
    }

    return t.String()
}
```

### Example 4: Error Page Rendering

```go
// internal/web/termrender/error.go

// RenderNotFound writes a text-formatted 404 error.
func (r *Renderer) RenderNotFound(w io.Writer, path string) error {
    var buf strings.Builder
    buf.WriteString(StyleError.Render("404 Not Found"))
    buf.WriteString("\n\n")
    buf.WriteString(StyleMuted.Render("The path "))
    buf.WriteString(StyleValue.Render(path))
    buf.WriteString(StyleMuted.Render(" does not exist."))
    buf.WriteString("\n\n")
    buf.WriteString(StyleLabel.Render("Try:"))
    buf.WriteString("\n")
    buf.WriteString("  $ curl peeringdb-plus.fly.dev/ui/asn/13335\n")
    buf.WriteString("  $ curl peeringdb-plus.fly.dev/ui/ix/31\n")
    buf.WriteString("  $ curl peeringdb-plus.fly.dev/ui/\n")
    return r.Write(w, buf.String())
}

// RenderServerError writes a text-formatted 500 error.
func (r *Renderer) RenderServerError(w io.Writer) error {
    var buf strings.Builder
    buf.WriteString(StyleError.Render("500 Internal Server Error"))
    buf.WriteString("\n\n")
    buf.WriteString(StyleMuted.Render("An unexpected error occurred. Please try again later."))
    buf.WriteString("\n")
    return r.Write(w, buf.String())
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| lipgloss v1 (`github.com/charmbracelet/lipgloss`) | lipgloss v2 (`charm.land/lipgloss/v2`) | 2025-07 (v2.0.0) | New vanity import path, Writer-based rendering, improved table package, built-in color downsampling. v1 is still on GitHub but v2 is recommended. |
| Manual ANSI escape codes | colorprofile.Writer | 2024 | Automatic color downsampling and stripping. Handles all ANSI escape types, not just SGR colors. |
| termenv for terminal detection | colorprofile (extracted from termenv) | 2024 | colorprofile is the focused extraction of terminal capability detection from the broader termenv library. |

**Deprecated/outdated:**
- `github.com/charmbracelet/lipgloss` v1: Still functional but v2 has breaking API changes and vanity import path. Use v2 for new code.
- `github.com/charmbracelet/termenv`: Still exists but for this use case, lipgloss v2 + colorprofile cover all needs without importing termenv directly.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go testing stdlib + testutil.SetupClient |
| Config file | None (standard `go test`) |
| Quick run command | `go test ./internal/web/termrender/ -race -count=1` |
| Full suite command | `go test ./... -race -count=1` |

### Phase Requirements to Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| DET-01 | Terminal UA detection (6 agents) | unit | `go test ./internal/web/termrender/ -run TestDetect -race` | Wave 0 |
| DET-02 | ?T and ?format=plain force plain text | unit | `go test ./internal/web/termrender/ -run TestDetect -race` | Wave 0 |
| DET-03 | ?format=json forces JSON | unit | `go test ./internal/web/termrender/ -run TestDetect -race` | Wave 0 |
| DET-04 | Accept header as secondary signal | unit | `go test ./internal/web/termrender/ -run TestDetect -race` | Wave 0 |
| DET-05 | Browsers get HTML unchanged | integration | `go test ./internal/web/ -run TestTerminal -race` | Wave 0 |
| RND-01 | Rich ANSI output with Unicode box drawing | unit | `go test ./internal/web/termrender/ -run TestRenderRich -race` | Wave 0 |
| RND-18 | ?nocolor suppresses ANSI | unit | `go test ./internal/web/termrender/ -run TestNoColor -race` | Wave 0 |
| NAV-01 | Help text at /ui/ for terminal clients | integration | `go test ./internal/web/ -run TestHomeTerminal -race` | Wave 0 |
| NAV-02 | Text-formatted 404 | integration | `go test ./internal/web/ -run TestNotFoundTerminal -race` | Wave 0 |
| NAV-03 | Text-formatted 500 | unit | `go test ./internal/web/termrender/ -run TestRenderServerError -race` | Wave 0 |
| NAV-04 | Root handler returns help for terminal | integration | `go test ./cmd/peeringdb-plus/ -run TestRootTerminal -race` | Wave 0 |

### Sampling Rate

- **Per task commit:** `go test ./internal/web/termrender/ ./internal/web/ -race -count=1`
- **Per wave merge:** `go test ./... -race -count=1`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps

- [ ] `internal/web/termrender/detect_test.go` -- covers DET-01 through DET-05
- [ ] `internal/web/termrender/renderer_test.go` -- covers RND-01, RND-18
- [ ] `internal/web/termrender/help_test.go` -- covers NAV-01
- [ ] `internal/web/termrender/error_test.go` -- covers NAV-02, NAV-03

## Open Questions

1. **PageContent.Data field type**
   - What we know: `PageContent` currently has `Title string` and `Content templ.Component`. Terminal rendering needs the raw data struct (e.g., `templates.NetworkDetail`).
   - What's unclear: Best approach -- add `Data any` field (requires type switch in renderer), or create a `TerminalRenderable` interface that data structs implement.
   - Recommendation: Use `Data any` field. Type switch in the terminal renderer is simpler and keeps data structs unchanged. The interface approach would require modifying all 6 detail types plus search/compare types for a single consumer.

2. **NAV-04 root handler location**
   - What we know: Root handler `GET /{$}` is in `cmd/peeringdb-plus/main.go` (inline closure). Terminal detection for NAV-04 needs to happen there.
   - What's unclear: Should the root handler import `termrender` directly, or should it be refactored into the web package?
   - Recommendation: Add terminal detection inline in the root handler closure. It is a simple `termrender.Detect()` call followed by writing help text. Moving the root handler to the web package is unnecessary scope creep.

3. **Help text data freshness timestamp**
   - What we know: D-11 requires current data freshness timestamp in help text. The sync worker tracks last sync time.
   - What's unclear: How to pass last-sync timestamp to `renderPage()` / help text renderer without threading it through all handlers.
   - Recommendation: Add a `LastSync func() time.Time` field to `Handler` struct, set from the sync worker. Help text renderer receives it as parameter. Only needed for the home/help handler, not all detail pages.

## Sources

### Primary (HIGH confidence)
- [charm.land/lipgloss/v2 on pkg.go.dev](https://pkg.go.dev/charm.land/lipgloss/v2) - v2.0.2 API documentation, published 2026-03-11
- [charm.land/lipgloss/v2/table on pkg.go.dev](https://pkg.go.dev/charm.land/lipgloss/v2/table) - Table sub-package API
- [github.com/charmbracelet/colorprofile on pkg.go.dev](https://pkg.go.dev/github.com/charmbracelet/colorprofile) - v0.4.3 API, Profile type, Writer type
- [no-color.org](https://no-color.org/) - NO_COLOR specification
- [wttr.in globals.py](https://github.com/chubin/wttr.in) - PLAIN_TEXT_AGENTS list for UA detection patterns

### Secondary (MEDIUM confidence)
- [Lip Gloss v2: What's New discussion](https://github.com/charmbracelet/lipgloss/discussions/506) - Writer concept, non-TTY handling
- [lipgloss GitHub README](https://github.com/charmbracelet/lipgloss) - v2 import path, color profiles, downsampling

### Tertiary (LOW confidence)
- HTTPie User-Agent format: `HTTPie/<version>` -- verified from httpie.io docs but exact string format may vary

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - lipgloss v2.0.2 version confirmed against Go module proxy, colorprofile v0.4.3 confirmed, API verified against pkg.go.dev
- Architecture: HIGH - existing code (renderPage, PageContent, detail handlers) examined directly, extension pattern is natural
- Pitfalls: HIGH - colorprofile TTY detection behavior verified against source code and documentation, Vary header concern documented in project STATE.md

**Research date:** 2026-03-25
**Valid until:** 2026-04-25 (stable domain, libraries recently released)
