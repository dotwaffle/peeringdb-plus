# Phase 29: Network Detail (Reference Implementation) - Research

**Researched:** 2026-03-25
**Domain:** Terminal text rendering for network entity detail pages
**Confidence:** HIGH

## Summary

This phase implements the network detail terminal renderer -- the first entity-specific renderer that replaces the generic `RenderPage` stub from Phase 28. The network entity is the "rich" type reference implementation: whois-style key-value header, compact one-line-per-entry IX presence/facility lists with color-coded speed tiers and [RS] badges, and curl-ready cross-reference paths.

The core challenge is threefold: (1) the `NetworkDetail` struct currently lacks IX presence and facility row data because the web UI lazy-loads those via htmx fragments, so the handler must eagerly fetch rows for terminal/JSON modes; (2) the rendering function must build styled text using lipgloss styles and route through the existing `colorprofile.Writer` for ANSI/plain mode switching; (3) the design must establish patterns that Phase 30 follows for all remaining entity types (IX, Facility, Org, Campus, Carrier).

All building blocks exist: lipgloss v2.0.2 styles and color constants are defined in `styles.go`, the `Renderer` type manages mode/noColor/Write plumbing, and the `renderPage()` switch in `render.go` dispatches to `renderer.RenderPage()` for terminal modes. The work is integrating existing data structures with new rendering logic.

**Primary recommendation:** Add `IXPresences []NetworkIXLanRow` and `FacPresences []NetworkFacRow` fields to `NetworkDetail`, populate them eagerly in `handleNetworkDetail`, then implement `RenderNetworkDetail()` on the Renderer that builds styled text line-by-line using `strings.Builder` and lipgloss styles. Type-switch on `page.Data` in `RenderPage` to dispatch to the entity-specific method.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Compact key-value pairs with aligned values, no border box. Human-readable labels ("Peering Policy" not "info_policy").
- **D-02:** Key fields: Name, ASN, Type, Peering Policy (color-coded), Website, IX Count, Fac Count, Prefixes v4, Prefixes v6, Aggregate Bandwidth
- **D-03:** Peering policy color-coded: Open=green, Selective=yellow, Restrictive=red
- **D-04:** Compact one-line per entry format -- no Unicode table borders, no column headers. Each IX/facility on a single line with key info.
- **D-05:** IX presence line format: `{IX Name} [{path}]  {speed}  {IPv4} / {IPv6}` with speed color-coded by tier
- **D-06:** Facility line format: `{Fac Name} [{path}]  {City}, {Country}`
- **D-07:** Route server peers marked with colored [RS] badge after IX name
- **D-08:** Inline path after entity name in square brackets: `DE-CIX Frankfurt [/ui/ix/31]`
- **D-09:** Paths are curl-ready -- user can copy-paste to follow up
- **D-10:** Network is a "rich" type -- full header with all key fields, detailed one-line lists with speed/IP/RS data
- **D-11:** This phase establishes the pattern for rich types (Network, IX, Facility). Phase 30 follows this for IX and Facility, and uses a minimal variant for Org, Campus, Carrier.

### Claude's Discretion
- Exact field ordering in header
- Spacing and alignment details
- How to handle missing/empty fields (omit vs show as empty)
- Section headers between IX presences and facilities

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| RND-02 | Network detail (/ui/asn/{asn}) renders with whois-style key-value header + IX/facility tables | Core deliverable: `RenderNetworkDetail()` method on Renderer, dispatched via type-switch in `RenderPage()` |
| RND-12 | Port speed tiers color-coded (gray/neutral/blue/emerald/amber) matching web UI | Speed color constants already defined in `styles.go` (ColorSpeedSub1G through ColorSpeed400G). Need `SpeedStyle(mbps)` helper function |
| RND-13 | Peering policy color-coded (Open=green, Selective=yellow, Restrictive=red) | Policy color constants already defined in `styles.go` (ColorPolicyOpen/Selective/Restrictive). Need `PolicyStyle(policy)` helper function |
| RND-14 | Route server peers marked with colored [RS] badge in IX presence tables | Inline `[RS]` text styled with ColorSuccess (emerald), appended after IX name in the one-line format |
| RND-15 | Aggregate bandwidth displayed in network detail header | `NetworkDetail.AggregateBW` already populated by handler. Need `formatSpeed()`-equivalent in termrender package |
| RND-16 | Entity IDs and cross-reference paths shown in output for easy follow-up curls | Inline `[/ui/ix/{id}]` and `[/ui/fac/{id}]` paths styled with StyleLink after entity names |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **CS-5 (MUST):** Input structs for functions with >2 args. Relevant for any new rendering functions.
- **CS-6 (SHOULD):** Declare function input structs before the function consuming them.
- **T-1 (MUST):** Table-driven tests; deterministic and hermetic.
- **T-2 (MUST):** Run `-race` in CI; `t.Cleanup` for teardown.
- **T-3 (SHOULD):** Mark safe tests with `t.Parallel()`.
- **API-1 (MUST):** Document exported items: `// Foo does ...`
- **ERR-1 (MUST):** Wrap errors with `%w` and context.
- **OBS-1 (MUST):** Structured logging (slog) with levels and consistent fields.

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| charm.land/lipgloss/v2 | v2.0.2 | Terminal text styling | Already in use. Provides `NewStyle().Foreground().Bold()` for per-element styling. Handles ANSI code generation. |
| github.com/charmbracelet/colorprofile | v0.4.3 | Color profile stripping | Already in use via `Renderer.Write()`. Strips ANSI codes for Plain/noColor modes automatically. |

### Supporting
No new dependencies needed. All rendering uses existing lipgloss styles and stdlib `strings.Builder` + `fmt.Sprintf`.

## Architecture Patterns

### Recommended File Structure
```
internal/web/termrender/
  detect.go          # Existing -- terminal detection
  renderer.go        # Existing -- Renderer type, Write(), RenderPage() dispatch
  network.go         # NEW -- RenderNetworkDetail() + helpers (formatSpeed, speedStyle, policyStyle)
  styles.go          # Existing -- color constants and predefined styles
  help.go            # Existing -- help page renderer
  error.go           # Existing -- error page renderer
  network_test.go    # NEW -- tests for network detail rendering
internal/web/templates/
  detailtypes.go     # MODIFIED -- add IXPresences/FacPresences fields to NetworkDetail
internal/web/
  detail.go          # MODIFIED -- eagerly fetch IX/facility rows for terminal/JSON modes
  render.go          # No changes needed -- RenderPage() already dispatches to renderer
```

### Pattern 1: Type-Switch Dispatch in RenderPage
**What:** `RenderPage()` already receives `data any`. Add a type switch to dispatch to entity-specific renderers.
**When to use:** Every entity detail page (this phase and Phase 30).
**Example:**
```go
// In renderer.go, modify RenderPage:
func (r *Renderer) RenderPage(w io.Writer, title string, data any) error {
    switch d := data.(type) {
    case templates.NetworkDetail:
        return r.RenderNetworkDetail(w, d)
    // Phase 30 adds: case templates.IXDetail, templates.FacilityDetail, etc.
    default:
        // Fallback: generic stub (existing behavior)
        var buf strings.Builder
        buf.WriteString(StyleHeading.Render(title))
        buf.WriteString("\n\n")
        if data != nil {
            buf.WriteString(StyleMuted.Render("Detailed terminal view coming in a future update."))
            buf.WriteString("\n")
            buf.WriteString(StyleMuted.Render("Use ?format=json for structured data."))
            buf.WriteString("\n")
        }
        return r.Write(w, buf.String())
    }
}
```

### Pattern 2: Eager Row Fetching for Terminal/JSON
**What:** The handler already fetches NetworkIxLan records (for aggregate BW). Extend to build `[]NetworkIXLanRow` and `[]NetworkFacRow` and attach to `NetworkDetail`.
**When to use:** All detail handlers where the web UI lazy-loads but terminal needs everything up-front.
**Why safe:** The rows are always fetched for aggregate BW computation anyway (IX). Adding facility fetch is one more query. The row slices are `json:"ixPresences,omitempty"` so JSON mode also benefits. The templ templates ignore extra struct fields.
**Example:**
```go
// In handleNetworkDetail, after the existing ixlans fetch:
ixRows := make([]templates.NetworkIXLanRow, len(ixlans))
for i, nix := range ixlans {
    row := templates.NetworkIXLanRow{
        IXName:   nix.Name,
        IXID:     nix.IxID,
        Speed:    nix.Speed,
        IsRSPeer: nix.IsRsPeer,
    }
    if nix.Ipaddr4 != nil {
        row.IPAddr4 = *nix.Ipaddr4
    }
    if nix.Ipaddr6 != nil {
        row.IPAddr6 = *nix.Ipaddr6
    }
    ixRows[i] = row
}
data.IXPresences = ixRows

// Facility rows (new query):
facItems, err := h.client.NetworkFacility.Query().
    Where(networkfacility.HasNetworkWith(network.ID(net.ID))).
    Order(networkfacility.ByName()).
    All(r.Context())
if err == nil {
    facRows := make([]templates.NetworkFacRow, len(facItems))
    for i, nf := range facItems {
        // ... same pattern as handleNetFacilitiesFragment
    }
    data.FacPresences = facRows
}
```

### Pattern 3: Line-by-Line Rendering with strings.Builder
**What:** Build the full output in a `strings.Builder`, applying lipgloss styles per-element, then pass the complete string to `r.Write(w, buf.String())` for color profile application.
**When to use:** All entity renderers. This is how `RenderHelp()` and `RenderError()` already work.
**Why:** Single `r.Write()` call ensures consistent color profile application. The `colorprofile.Writer` strips ANSI codes in Plain/noColor mode automatically.
**Example:**
```go
func (r *Renderer) RenderNetworkDetail(w io.Writer, data templates.NetworkDetail) error {
    var buf strings.Builder

    // Header: "Cloudflare  AS13335"
    buf.WriteString(StyleHeading.Render(data.Name))
    buf.WriteString("  ")
    buf.WriteString(StyleMuted.Render(fmt.Sprintf("AS%d", data.ASN)))
    buf.WriteString("\n\n")

    // Key-value fields with aligned values
    writeKV(&buf, "Type", data.InfoType)
    writeKV(&buf, "Peering Policy", policyStyled(data.PolicyGeneral))
    // ... more fields ...

    // IX Presences section
    if len(data.IXPresences) > 0 {
        buf.WriteString("\n")
        buf.WriteString(StyleHeading.Render(fmt.Sprintf("IX Presences (%d)", len(data.IXPresences))))
        buf.WriteString(StyleMuted.Render(fmt.Sprintf("  %s", formatBandwidth(data.AggregateBW))))
        buf.WriteString("\n")
        for _, ix := range data.IXPresences {
            // One-line per entry
        }
    }

    return r.Write(w, buf.String())
}
```

### Pattern 4: Reusable Helper Functions (Reference for Phase 30)
**What:** Extract common formatting into package-level helpers that Phase 30 reuses.
**When to use:** Speed formatting, speed coloring, policy coloring, cross-reference paths, key-value alignment, bandwidth formatting.
```go
// SpeedStyle returns a lipgloss style for the given port speed in Mbps.
func SpeedStyle(mbps int) lipgloss.Style { ... }

// FormatSpeed converts Mbps to human-readable (e.g., "10G", "100G", "400M").
func FormatSpeed(mbps int) string { ... }

// PolicyStyle returns styled text for a peering policy value.
func PolicyStyle(policy string) string { ... }

// FormatBandwidth formats aggregate bandwidth in Mbps as "1.2 Tbps", "480 Gbps", etc.
func FormatBandwidth(mbps int) string { ... }

// CrossRef formats an inline cross-reference path: "[/ui/ix/31]"
func CrossRef(path string) string { ... }

// writeKV writes a key-value pair with label alignment to buf.
// Unexported -- used by all entity renderers in this package.
func writeKV(buf *strings.Builder, label, value string) { ... }
```

### Anti-Patterns to Avoid
- **Using lipgloss/v2/table for the IX/facility lists:** D-04 explicitly says no table borders, no column headers. Manual line formatting is correct.
- **Creating a separate data struct for terminal rendering:** Reuse `templates.NetworkDetail` with added row slices. Keeps JSON output identical to terminal data.
- **Applying lipgloss styles inside the Write() call:** Style ALL text first in the Builder, then call `r.Write()` once. This ensures the colorprofile.Writer strips everything consistently.
- **Fetching rows conditionally per render mode:** Always eagerly fetch rows regardless of mode. JSON mode needs them too, and the overhead is negligible (the BW query already fetches all ixlans).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| ANSI code stripping | Manual regex for stripping escape codes | `colorprofile.Writer` with `NoTTY` profile | Already handles all ANSI sequences correctly; tested in Phase 28 |
| Speed formatting | Custom speed formatter | Port `formatSpeed()` from `detail_shared.templ` to termrender package | Same logic, different package; keep consistent with web UI |
| Speed color tiers | New color mapping | `ColorSpeedSub1G` through `ColorSpeed400G` constants already in `styles.go` | Already mapped to ANSI 256-color equivalents of web UI Tailwind classes |
| Policy colors | New color mapping | `ColorPolicyOpen/Selective/Restrictive` constants already in `styles.go` | Already defined per D-03 color mapping |
| Bandwidth formatting | Custom formatter | Port `formatAggregateBW()` from `detail_shared.templ` | Same Tbps/Gbps/Mbps logic |

**Key insight:** The web UI already solved all the formatting and color-tier logic. The terminal renderer ports these decisions to ANSI equivalents using the color constants Phase 28 established.

## Common Pitfalls

### Pitfall 1: IX Presences Without Sorting
**What goes wrong:** IX presences display in database insertion order, not alphabetical.
**Why it happens:** The existing `ixlans` query in `handleNetworkDetail` (line 94) has no `Order()` clause because it only needed aggregate BW.
**How to avoid:** Add `Order(networkixlan.ByName())` to the ixlans query, matching the fragment handler pattern (line 533).
**Warning signs:** Output differs from web UI ordering when comparing same network.

### Pitfall 2: lipgloss Style Rendering Creates New Strings
**What goes wrong:** Each `style.Render("text")` allocates a new string with embedded ANSI codes.
**Why it happens:** lipgloss returns styled strings, not in-place modifications.
**How to avoid:** Build all styled segments into a single `strings.Builder`, then pass the complete string to `r.Write()`. For large networks (DE-CIX with 1000+ IX presences), pre-allocate the builder: `buf.Grow(len(data.IXPresences) * 120)`.
**Warning signs:** Excessive allocations visible in benchmarks for large networks.

### Pitfall 3: Nil Pointer Dereference on Optional Fields
**What goes wrong:** `NetworkIxLan.Ipaddr4` and `Ipaddr6` are `*string` (optional in ent schema). Directly accessing them panics.
**Why it happens:** PeeringDB allows IX presences without IP addresses (rare but possible).
**How to avoid:** Always check `!= nil` before dereferencing. The fragment handler already does this correctly (lines 549-554) -- follow the same pattern.
**Warning signs:** Nil pointer panic when rendering a network with incomplete IX presence data.

### Pitfall 4: NetworkFacility.FacID is *int
**What goes wrong:** Assuming `FacID` is always present. It's `*int` in the ent schema.
**Why it happens:** PeeringDB's API sometimes returns `null` for facility ID references.
**How to avoid:** Check `nf.FacID != nil` before dereferencing, matching the fragment handler pattern (line 583-585). If nil, omit the cross-reference path.
**Warning signs:** Nil pointer panic on networks with facilities that have null FacID.

### Pitfall 5: Plain Mode Must Produce Identical Layout
**What goes wrong:** Plain mode output has different alignment or missing elements compared to rich mode.
**Why it happens:** ANSI codes affect visible width. When stripped, alignment shifts.
**How to avoid:** Use lipgloss `Style.Render()` for ALL text elements, even "unstyled" ones. The `colorprofile.Writer` strips codes uniformly. Do NOT mix raw strings with styled strings in alignment-sensitive areas.
**Warning signs:** `curl "url?T"` output has misaligned columns compared to `curl url`.

### Pitfall 6: Aggregate Bandwidth of Zero
**What goes wrong:** Displaying "0 Mbps" in the header when a network has no IX presences.
**Why it happens:** `AggregateBW` defaults to 0 if no IX presences exist.
**How to avoid:** Omit the aggregate bandwidth line when `AggregateBW == 0`, matching the web UI's conditional rendering.
**Warning signs:** Networks with no IX presences showing "Aggregate Bandwidth: 0 Mbps".

## Code Examples

Verified patterns from existing codebase:

### Whois-Style Key-Value Header
```go
// writeKV writes a labeled key-value pair with right-aligned label.
// Labels are padded to labelWidth for column alignment (D-01).
func writeKV(buf *strings.Builder, label, value string, labelWidth int) {
    if value == "" {
        return // Omit empty fields per Claude's discretion
    }
    padded := fmt.Sprintf("%*s", labelWidth, label)
    buf.WriteString(StyleLabel.Render(padded))
    buf.WriteString("  ")
    buf.WriteString(StyleValue.Render(value))
    buf.WriteString("\n")
}
```

### Speed-Colored IX Presence Line (D-05)
```go
// Example output: "DE-CIX Frankfurt [/ui/ix/31]  [RS]  100G  80.81.192.123 / 2001:7f8::3347:0:1"
func (r *Renderer) writeIXPresenceLine(buf *strings.Builder, row templates.NetworkIXLanRow) {
    // IX Name
    buf.WriteString("  ")
    buf.WriteString(StyleValue.Render(row.IXName))

    // Cross-reference path (D-08, D-09)
    buf.WriteString(" ")
    buf.WriteString(StyleLink.Render(fmt.Sprintf("[/ui/ix/%d]", row.IXID)))

    // RS badge (D-07)
    if row.IsRSPeer {
        rsStyle := lipgloss.NewStyle().Foreground(ColorSuccess)
        buf.WriteString("  ")
        buf.WriteString(rsStyle.Render("[RS]"))
    }

    // Speed with color tier (D-05, RND-12)
    if row.Speed > 0 {
        buf.WriteString("  ")
        buf.WriteString(SpeedStyle(row.Speed).Render(FormatSpeed(row.Speed)))
    }

    // IP addresses
    if row.IPAddr4 != "" {
        buf.WriteString("  ")
        buf.WriteString(row.IPAddr4)
    }
    if row.IPAddr4 != "" && row.IPAddr6 != "" {
        buf.WriteString(" / ")
    } else if row.IPAddr6 != "" {
        buf.WriteString("  ")
    }
    if row.IPAddr6 != "" {
        buf.WriteString(row.IPAddr6)
    }
    buf.WriteString("\n")
}
```

### Speed Style Helper (RND-12)
```go
// SpeedStyle returns a lipgloss style colored by port speed tier.
// Matches web UI tiers: sub-1G gray, 1G neutral, 10G blue, 100G emerald, 400G+ amber.
func SpeedStyle(mbps int) lipgloss.Style {
    switch {
    case mbps >= 400_000:
        return lipgloss.NewStyle().Foreground(ColorSpeed400G).Bold(true)
    case mbps >= 100_000:
        return lipgloss.NewStyle().Foreground(ColorSpeed100G)
    case mbps > 1000:
        return lipgloss.NewStyle().Foreground(ColorSpeed10G)
    case mbps == 1000:
        return lipgloss.NewStyle().Foreground(ColorSpeed1G)
    default:
        return lipgloss.NewStyle().Foreground(ColorSpeedSub1G)
    }
}
```

### Policy Style Helper (RND-13)
```go
// PolicyStyle returns styled text for a peering policy value.
// Open=green, Selective=yellow, Restrictive=red, others=default.
func PolicyStyle(policy string) string {
    switch strings.ToLower(policy) {
    case "open":
        return lipgloss.NewStyle().Foreground(ColorPolicyOpen).Render(policy)
    case "selective":
        return lipgloss.NewStyle().Foreground(ColorPolicySelective).Render(policy)
    case "restrictive":
        return lipgloss.NewStyle().Foreground(ColorPolicyRestrictive).Render(policy)
    default:
        return StyleValue.Render(policy)
    }
}
```

## Data Flow

```
handleNetworkDetail()
  |-- Query network + org edge
  |-- Query NetworkIxLan (already done for AggregateBW)
  |      |-- Build []NetworkIXLanRow (NEW)
  |-- Query NetworkFacility (NEW query)
  |      |-- Build []NetworkFacRow (NEW)
  |-- Populate NetworkDetail{..., IXPresences, FacPresences}
  |-- renderPage(ctx, w, r, PageContent{Data: data})
        |
        +-- termrender.Detect() -> ModeRich/ModePlain
        |
        +-- renderer.RenderPage(w, title, data)
              |-- type switch: data.(templates.NetworkDetail)
              |-- renderer.RenderNetworkDetail(w, data)
                    |-- Build styled text in strings.Builder
                    |-- Header section (D-01, D-02)
                    |-- IX Presences section (D-04, D-05, D-07)
                    |-- Facilities section (D-04, D-06)
                    |-- r.Write(w, buf.String())
                          |-- colorprofile.Writer strips ANSI if needed
```

## Struct Modifications

### NetworkDetail Extension
```go
// Add to templates.NetworkDetail in detailtypes.go:

// IXPresences holds eager-loaded IX presence rows for terminal/JSON rendering.
// Nil for web UI (lazy-loaded via htmx fragments). Populated by handler
// when terminal or JSON mode is detected.
IXPresences []NetworkIXLanRow `json:"ixPresences,omitempty"`
// FacPresences holds eager-loaded facility presence rows for terminal/JSON rendering.
// Nil for web UI (lazy-loaded via htmx fragments). Populated by handler
// when terminal or JSON mode is detected.
FacPresences []NetworkFacRow `json:"facPresences,omitempty"`
```

**Design choice:** Always populate these fields (not just for terminal mode). Reasons:
1. JSON mode (`?format=json`) also needs the full data
2. The IX query already runs for aggregate BW -- just convert the results to rows
3. The facility query is one additional DB call -- negligible for read-only SQLite
4. Simpler handler logic -- no mode-detection branching in the handler
5. The web UI's templ template ignores these fields (it lazy-loads via fragments)

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `RenderPage()` generic stub | Entity-specific type-switch dispatch | Phase 29 | Each entity type gets dedicated rendering |
| IX/facility data lazy-loaded only | Eagerly loaded into detail struct | Phase 29 | Terminal and JSON modes get complete data |
| Speed/policy colors in templ only | Shared color constants in termrender | Phase 28 | Both web and terminal use same tier definitions |

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) |
| Config file | none (standard go test) |
| Quick run command | `go test ./internal/web/termrender/ -count=1 -race` |
| Full suite command | `go test ./... -count=1 -race` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| RND-02 | Network detail renders whois-style header + IX/fac lists | unit | `go test ./internal/web/termrender/ -run TestRenderNetworkDetail -count=1` | Wave 0 |
| RND-12 | Speed tiers color-coded by tier | unit | `go test ./internal/web/termrender/ -run TestSpeedStyle -count=1` | Wave 0 |
| RND-13 | Policy color-coded | unit | `go test ./internal/web/termrender/ -run TestPolicyStyle -count=1` | Wave 0 |
| RND-14 | RS badge in IX presence lines | unit | `go test ./internal/web/termrender/ -run TestRenderNetworkDetail_RSBadge -count=1` | Wave 0 |
| RND-15 | Aggregate bandwidth in header | unit | `go test ./internal/web/termrender/ -run TestRenderNetworkDetail_AggregateBW -count=1` | Wave 0 |
| RND-16 | Cross-reference paths in output | unit | `go test ./internal/web/termrender/ -run TestRenderNetworkDetail_CrossRefs -count=1` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/web/termrender/ -count=1 -race`
- **Per wave merge:** `go test ./... -count=1 -race`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/web/termrender/network_test.go` -- covers RND-02, RND-12, RND-13, RND-14, RND-15, RND-16
- No framework install needed -- Go testing stdlib already available
- No shared fixtures needed -- tests construct `templates.NetworkDetail` structs directly

## Open Questions

1. **Key-value label width alignment**
   - What we know: D-01 says "aligned values". The header has labels of varying length ("ASN" = 3, "Peering Policy" = 14, "Aggregate Bandwidth" = 19).
   - What's unclear: Whether to use the max label width for all KV pairs (wastes space for short labels) or group them.
   - Recommendation: Use max label width from the set of displayed fields. This matches whois output conventions. For a network with all fields, max is ~19 chars ("Aggregate Bandwidth"). Omitted fields reduce the visible max automatically.

2. **Performance with large IX presence lists**
   - What we know: STATE.md flags "Large IX tables (1000+ rows, e.g., DE-CIX) need benchmarking with lipgloss during Phase 29".
   - What's unclear: Exact performance characteristics of lipgloss style rendering for 1000+ lines.
   - Recommendation: Pre-allocate `strings.Builder` with estimated capacity (`len(rows) * 120`). The lipgloss styling is string concatenation, not parsing -- should be fast. Add a benchmark test (`BenchmarkRenderNetworkDetail_LargeIX`) to validate. If slow, the style calls can be cached (one `SpeedStyle` per tier, not per row).

## Sources

### Primary (HIGH confidence)
- `internal/web/termrender/renderer.go` -- current Renderer implementation and RenderPage stub
- `internal/web/termrender/styles.go` -- all color constants and styles (lipgloss v2.0.2)
- `internal/web/detail.go` -- handleNetworkDetail handler, data fetching patterns
- `internal/web/templates/detailtypes.go` -- NetworkDetail struct, all row types
- `internal/web/templates/detail_shared.templ` -- formatSpeed, speedColorClass, formatAggregateBW implementations
- `internal/web/templates/detail_net.templ` -- HTML template showing all displayed fields
- `internal/web/render.go` -- renderPage dispatch logic
- `internal/web/termrender/help.go` -- reference pattern for building styled text with Builder

### Secondary (MEDIUM confidence)
- `go.mod` -- lipgloss v2.0.2, colorprofile v0.4.3 pinned versions confirmed
- `charm.land/lipgloss/v2/table` -- table package exists but NOT needed per D-04 decisions

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all libraries already in use, no new dependencies
- Architecture: HIGH -- patterns established by Phase 28 (help, error renderers), just extending with entity-specific method
- Pitfalls: HIGH -- identified from direct code inspection of existing handlers and data types

**Research date:** 2026-03-25
**Valid until:** 2026-04-25 (stable -- no external dependency changes expected)
