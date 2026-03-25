# Architecture Patterns

**Domain:** Terminal CLI rendering integration for PeeringDB Plus v1.8
**Researched:** 2026-03-25

## Recommended Architecture

### Overview

Content negotiation within the existing `/ui/` handler dispatch, using a rendering branch that shares data types between HTML (templ) and terminal (lipgloss) renderers. No new routes. No middleware-based approach. The terminal rendering is a peer of the htmx fragment rendering already in `renderPage()`.

### Why Handler-Level, Not Middleware

1. **The existing pattern already does this.** `renderPage()` in `internal/web/render.go` already branches on `HX-Request` header. Adding a third branch for terminal output follows the identical pattern.
2. **Middleware cannot access typed data.** The terminal renderer needs `NetworkDetail`, `IXDetail`, etc. Middleware only sees `http.ResponseWriter` and `*http.Request`.
3. **Handler dispatch already exists.** Each handler constructs the typed detail struct. The renderer choice must happen after data construction.

### Architecture Diagram

```
Request flow (existing + new):

  GET /ui/asn/13335
       |
  [dispatch()] -- routes to handleNetworkDetail
       |
  [query ent, build NetworkDetail struct]  <-- no change
       |
  [detect output mode] -- User-Agent + Accept + ?format= + ?T
       |
       +-- OutputJSON         --> json.Encode(data)         (NEW)
       +-- OutputPlain        --> terminal.RenderNetwork    (NEW, colorprofile.ASCII)
       +-- OutputANSI         --> terminal.RenderNetwork    (NEW, colorprofile.ANSI256)
       +-- OutputHTML + htmx  --> templ fragment             (existing)
       +-- OutputHTML         --> templ full page             (existing)
```

## Component Boundaries

| Component | Responsibility | Communicates With | New/Modified |
|-----------|---------------|-------------------|--------------|
| `internal/web/handler.go` | Route dispatch, data assembly, output mode branch | ent client, detect.go, terminal pkg | **Modified** |
| `internal/web/render.go` | HTML rendering (templ), Vary header | templ templates | **Modified** (Vary header update) |
| `internal/web/detect.go` | User-Agent detection, format resolution | handler.go | **New** |
| `internal/web/templates/detailtypes.go` | Shared data types (NetworkDetail, etc.) | All renderers | **No change** |
| `internal/terminal/` | ANSI/plain rendering for all entity types | detailtypes, lipgloss | **New package** |
| `internal/terminal/style.go` | Color palette, lipgloss style definitions | lipgloss v2 | **New** |
| `internal/terminal/table.go` | Shared table rendering helpers | lipgloss/table | **New** |
| `internal/terminal/render.go` | Core rendering functions, writer setup | colorprofile | **New** |
| `internal/terminal/network.go` | NetworkDetail -> ANSI string | detailtypes, style, table | **New** |
| `internal/terminal/ix.go` | IXDetail -> ANSI string | detailtypes, style, table | **New** |
| `internal/terminal/facility.go` | FacilityDetail -> ANSI string | detailtypes, style, table | **New** |
| `internal/terminal/org.go` | OrgDetail -> ANSI string | detailtypes, style, table | **New** |
| `internal/terminal/campus.go` | CampusDetail -> ANSI string | detailtypes, style, table | **New** |
| `internal/terminal/carrier.go` | CarrierDetail -> ANSI string | detailtypes, style, table | **New** |
| `internal/terminal/search.go` | Search results -> ANSI string | searchtypes, style | **New** |
| `internal/terminal/compare.go` | Compare results -> ANSI string | comparetypes, style, table | **New** |
| `internal/terminal/help.go` | CLI help text for `/ui/` root | style | **New** |
| `internal/terminal/error.go` | Error pages -> plain text | None | **New** |

## Data Flow

### Current Flow (HTML)

```
handleNetworkDetail()
  -> query ent -> build templates.NetworkDetail
  -> PageContent{Title: net.Name, Content: templates.NetworkDetailPage(data)}
  -> renderPage(ctx, w, r, page)
     -> if HX-Request: page.Content.Render(ctx, w)
     -> else: templates.Layout(page.Title, page.Content).Render(ctx, w)
```

### New Flow (Terminal Detection Added)

```
handleNetworkDetail()
  -> query ent -> build templates.NetworkDetail  [UNCHANGED]
  -> mode := DetectOutput(r)
  -> switch mode:
     -> OutputJSON:  writeJSON(w, data)
     -> OutputANSI:  terminal.RenderNetwork(w, data, terminal.ModeANSI)
     -> OutputPlain: terminal.RenderNetwork(w, data, terminal.ModePlain)
     -> OutputHTML:  renderPage(ctx, w, r, page)  [existing path, unchanged]
```

The key insight: **the detail data types are already decoupled from HTML rendering.** `templates.NetworkDetail` is a pure Go struct with zero dependencies on templ. Both the HTML renderer and the new terminal renderer consume the same struct.

### Shared Data Types -- No Changes Needed

The types in `detailtypes.go` are already pure data:

```go
type NetworkDetail struct {
    ID       int
    ASN      int
    Name     string
    // ... 25+ fields, all plain Go types
}
```

Same applies to `IXDetail`, `FacilityDetail`, `OrgDetail`, `CampusDetail`, `CarrierDetail`, and all row types.

## Patterns to Follow

### Pattern 1: Output Mode Detection

**What:** Determine output format from User-Agent, Accept header, and query parameters. Resolve once per request.

**Implementation:**

```go
// internal/web/detect.go

type OutputMode int

const (
    OutputHTML  OutputMode = iota // Browser: full HTML or htmx fragment
    OutputANSI                   // Terminal: 256-color ANSI with box drawing
    OutputPlain                  // Terminal: no color, plain text (?T)
    OutputJSON                   // Any client: JSON (?format=json)
)

// DetectOutput determines output format from the request.
// Priority: query params > Accept header > User-Agent.
func DetectOutput(r *http.Request) OutputMode {
    q := r.URL.Query()
    if q.Get("format") == "json" {
        return OutputJSON
    }
    if q.Has("T") || q.Get("format") == "text" {
        return OutputPlain
    }
    if isTerminalClient(r) {
        return OutputANSI
    }
    return OutputHTML
}
```

### Pattern 2: User-Agent Detection (wttr.in Pattern)

**What:** Case-insensitive substring match against known terminal HTTP clients.

```go
var terminalAgents = []string{
    "curl", "httpie", "wget", "lwp-request",
    "python-requests", "python-httpx", "openbsd ftp",
    "powershell", "fetch", "aiohttp", "http_get",
    "xh", "nushell", "go-http-client", "libfetch",
}

func isTerminalClient(r *http.Request) bool {
    ua := strings.ToLower(r.Header.Get("User-Agent"))
    if ua == "" {
        return true // No UA = likely bare TCP tool
    }
    for _, agent := range terminalAgents {
        if strings.Contains(ua, agent) {
            return true
        }
    }
    return false
}
```

**Rationale for empty User-Agent = terminal:** Tools like `nc` send no UA. Returning terminal output is more useful than HTML. Browsers always send a UA.

### Pattern 3: Handler-Level Mode Branching

**What:** Each handler detects mode and branches. The existing `renderPage()` stays untouched for the HTML path.

```go
func (h *Handler) handleNetworkDetail(w http.ResponseWriter, r *http.Request, asnStr string) {
    // ... existing ent query and data construction, unchanged ...

    mode := DetectOutput(r)
    switch {
    case mode == OutputJSON:
        w.Header().Set("Content-Type", "application/json; charset=utf-8")
        w.Header().Set("Vary", "User-Agent, Accept, HX-Request")
        json.NewEncoder(w).Encode(data)
    case mode == OutputANSI || mode == OutputPlain:
        w.Header().Set("Content-Type", "text/plain; charset=utf-8")
        w.Header().Set("Vary", "User-Agent, Accept, HX-Request")
        w.Header().Set("Cache-Control", "private")
        terminal.RenderNetwork(w, data, terminalMode(mode))
    default:
        page := PageContent{Title: net.Name, Content: templates.NetworkDetailPage(data)}
        renderPage(r.Context(), w, r, page)
    }
}
```

**Why detect-in-handler instead of modifying renderPage:**
- No changes to `PageContent` struct or existing `renderPage` function.
- Terminal rendering added incrementally, one entity at a time.
- Each path is isolated and testable.

### Pattern 4: Server-Side ANSI Rendering with lipgloss v2

**What:** Use lipgloss v2 for styling and lipgloss/table for tables. Explicitly set color profile since there is no terminal to detect.

```go
// internal/terminal/render.go

import (
    "github.com/charmbracelet/colorprofile"
    lipgloss "charm.land/lipgloss/v2"
    "charm.land/lipgloss/v2/table"
)

// Mode determines the terminal output format.
type Mode int

const (
    ModeANSI  Mode = iota // 256-color with Unicode box drawing
    ModePlain             // ASCII only, no escape codes
)

// writeOutput writes styled content to w with appropriate color profile.
func writeOutput(w io.Writer, content string, mode Mode) error {
    switch mode {
    case ModePlain:
        // colorprofile.Writer with ASCII profile strips all ANSI codes
        pw := &colorprofile.Writer{Forward: w, Profile: colorprofile.ASCII}
        _, err := pw.Write([]byte(content))
        return err
    default:
        // ANSI256 profile -- write escape codes as-is
        _, err := io.WriteString(w, content)
        return err
    }
}
```

### Pattern 5: Whois-Style Key-Value Rendering

**What:** Network engineers expect RPSL/WHOIS-style layout for entity details.

```go
// internal/terminal/render.go

// renderKeyValue formats a label:value pair with aligned colons.
func renderKeyValue(label, value string, labelWidth int, styles Styles) string {
    styledLabel := styles.Label.Render(fmt.Sprintf("%-*s", labelWidth, label))
    return fmt.Sprintf("%s  %s", styledLabel, value)
}

// Example output:
// ASN:            13335
// Name:           Cloudflare, Inc.
// Organization:   Cloudflare, Inc.
// Policy:         Open
// Traffic:        20000+ Gbps
```

### Pattern 6: lipgloss/table for List Sections

**What:** Tables for IX presences, facility lists, participants. Per-cell styling via StyleFunc.

```go
t := table.New().
    Headers("IX", "Speed", "IPv4", "IPv6", "RS").
    Border(lipgloss.RoundedBorder()).
    BorderHeader(true).
    BorderColumn(true).
    Width(80).
    StyleFunc(func(row, col int) lipgloss.Style {
        if row == table.HeaderRow {
            return styles.TableHeader
        }
        if row%2 == 0 {
            return styles.TableEvenRow
        }
        return styles.TableOddRow
    })

for _, ix := range data.IXPresences {
    t.Row(ix.IXName, formatSpeed(ix.Speed), ix.IPv4, ix.IPv6, rsIndicator(ix.IsRouteServer))
}

output := t.Render()
```

## Anti-Patterns to Avoid

### Anti-Pattern 1: Separate Route Tree for Terminal

**What:** Creating `/cli/asn/13335` alongside `/ui/asn/13335`.
**Why bad:** Duplicates route registration and data assembly. Breaks "same URL, different representation" model.
**Instead:** Content negotiation under existing `/ui/` URLs.

### Anti-Pattern 2: Middleware-Based Content Negotiation

**What:** Middleware that intercepts responses and transforms HTML to text.
**Why bad:** Cannot produce rich ANSI tables from HTML. Middleware lacks access to typed data. Adds latency.
**Instead:** Branch at the render point where typed data is available.

### Anti-Pattern 3: Accept Header as Primary Detection

**What:** Using `Accept: text/plain` vs `Accept: text/html` as the primary mechanism.
**Why bad:** curl sends `Accept: */*` by default. So does wget. Users would need `curl -H 'Accept: text/plain'` which defeats zero-setup.
**Instead:** User-Agent detection as primary (wttr.in's proven approach). Accept header as secondary.

### Anti-Pattern 4: Global Color Profile State

**What:** Using `lipgloss.SetColorProfile()` globally.
**Why bad:** In lipgloss v2, the rendering is deterministic -- styles produce ANSI strings based on their definition. The colorprofile.Writer handles downsampling when writing. Using a global profile is the v1 approach.
**Instead:** Use lipgloss v2 styles directly (they produce full ANSI strings), then pass output through colorprofile.Writer with the appropriate profile for the output mode. For ANSI mode, write directly. For plain mode, use colorprofile.ASCII writer to strip codes.

### Anti-Pattern 5: Buffering Both HTML and Terminal Output

**What:** Rendering both formats, then choosing which to send.
**Why bad:** Doubles the work.
**Instead:** Detect output mode first, render only the selected format.

## Scalability Considerations

Not applicable for scalability concerns -- this is a rendering layer change. Terminal rendering is pure string formatting with no I/O beyond writing to the HTTP response. Performance is bounded by the ent query (unchanged), not string formatting.

One consideration: ANSI output for large IX participant lists (DE-CIX: 1000+ participants) could produce large responses (~50-100KB of ANSI text). This is fine -- the existing HTML path produces larger responses for the same data. Terminal users expect full output and can pipe to `less -R`.

## Caching Impact

The `Vary` header must be updated from `Vary: HX-Request` to `Vary: HX-Request, User-Agent, Accept` to prevent CDN/proxy caches from serving HTML to curl or ANSI to browsers. Additionally, terminal responses should set `Cache-Control: private` to prevent shared caches from storing ANSI output.

## Build Order (Dependency-Ordered)

1. **`internal/web/detect.go`** -- User-Agent detection + output mode resolution. No dependencies on terminal package. Build and test independently.

2. **`internal/terminal/style.go`** -- Color palette and shared style definitions. Depends only on lipgloss v2.

3. **`internal/terminal/table.go`** -- Shared table rendering helpers (border config, default widths). Depends on lipgloss/table.

4. **`internal/terminal/render.go`** -- Core rendering functions, colorprofile writer setup, key-value formatter. Depends on colorprofile.

5. **`internal/terminal/help.go`** -- CLI help text for `/ui/` root. No entity dependencies. Good first integration test.

6. **`internal/terminal/error.go`** -- Error pages (404, 500) for terminal clients.

7. **`internal/terminal/network.go`** -- Network detail ANSI renderer. First entity type. Validates the pattern.

8. **`internal/terminal/ix.go`** -- IX detail. Includes participant table (largest tables in dataset).

9. **`internal/terminal/facility.go`**, **`org.go`**, **`campus.go`**, **`carrier.go`** -- Remaining entity types.

10. **`internal/terminal/search.go`** -- Search results in terminal format.

11. **`internal/terminal/compare.go`** -- ASN comparison in terminal format.

12. **Modify `internal/web/handler.go`** -- Wire up detection and terminal rendering in each handler.

13. **Modify `internal/web/render.go`** -- Update Vary header.

14. **Modify `cmd/peeringdb-plus/main.go`** -- readinessMiddleware terminal-aware syncing message.

Items 1-6 can be built without touching any existing code. Items 7-11 are independent of each other (parallelizable). Items 12-14 are integration points.

## Package Structure

```
internal/
  web/
    detect.go          (NEW) -- OutputMode detection, isTerminalClient
    detect_test.go     (NEW) -- User-Agent detection tests (table-driven)
    handler.go         (MODIFIED) -- mode branching in each handler
    render.go          (MODIFIED) -- Vary header update
    templates/
      detailtypes.go   (UNCHANGED) -- shared data types
      comparetypes.go  (UNCHANGED) -- shared data types
      searchtypes.go   (UNCHANGED) -- shared data types
      *.templ          (UNCHANGED) -- HTML templates
  terminal/
    doc.go             (NEW) -- package documentation
    style.go           (NEW) -- color palette, shared lipgloss styles
    table.go           (NEW) -- table rendering helpers
    render.go          (NEW) -- core rendering, colorprofile writer, key-value
    help.go            (NEW) -- /ui/ root help text
    error.go           (NEW) -- 404/500 for terminal
    network.go         (NEW) -- NetworkDetail renderer
    ix.go              (NEW) -- IXDetail renderer
    facility.go        (NEW) -- FacilityDetail renderer
    org.go             (NEW) -- OrgDetail renderer
    campus.go          (NEW) -- CampusDetail renderer
    carrier.go         (NEW) -- CarrierDetail renderer
    search.go          (NEW) -- search results renderer
    compare.go         (NEW) -- comparison renderer
    render_test.go     (NEW) -- table-driven tests for all renderers
```

## Integration Points Summary

| Integration Point | What Changes | Risk |
|-------------------|-------------|------|
| `internal/web/detect.go` | New file. User-Agent detection. | LOW -- purely additive |
| `internal/web/render.go` | `Vary` header adds `User-Agent, Accept` | LOW -- one-line change |
| `internal/web/handler.go` | Each handle* function gains mode check + terminal branch | LOW -- pattern is mechanical |
| `internal/web/handler.go` handleHome | Terminal root shows help text | LOW |
| `internal/web/handler.go` handleSearch | Terminal search returns text results | LOW |
| `internal/web/handler.go` handleNotFound / handleServerError | Terminal-aware error messages | LOW |
| `cmd/peeringdb-plus/main.go` readinessMiddleware | Terminal clients during sync get text message | LOW |
| `go.mod` | Add `charm.land/lipgloss/v2`, `github.com/charmbracelet/colorprofile` | LOW |

No existing tests should break. Default output mode is `OutputHTML` (current behavior). All terminal paths are additive.

## Sources

- [wttr.in](https://github.com/chubin/wttr.in) -- Content negotiation via User-Agent, `?T` for plain text
- [charm.land/lipgloss/v2](https://pkg.go.dev/charm.land/lipgloss/v2) -- v2.0.2, deterministic styles, border types
- [charm.land/lipgloss/v2/table](https://pkg.go.dev/charm.land/lipgloss/v2/table) -- Table rendering with StyleFunc
- [charmbracelet/colorprofile](https://pkg.go.dev/github.com/charmbracelet/colorprofile) -- v0.4.3, Profile constants, Writer type
- Existing codebase: `internal/web/handler.go`, `internal/web/render.go`, `internal/web/templates/detailtypes.go`
