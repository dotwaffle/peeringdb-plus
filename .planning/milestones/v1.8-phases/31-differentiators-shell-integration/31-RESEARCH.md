# Phase 31: Differentiators & Shell Integration - Research

**Researched:** 2026-03-26
**Domain:** Terminal rendering enhancements, shell completion scripts, HTTP-served CLI tools
**Confidence:** HIGH

## Summary

Phase 31 adds five distinct power-user features to the terminal rendering pipeline: short format mode (`?format=short`), section filtering (`?section=`), width adaptation (`?w=N`), data freshness footer, and downloadable shell completion scripts. All features build on the well-established termrender package from Phases 28-30, which provides a clear Renderer struct, type-switched dispatch in `RenderPage`, and a `Detect()` function for format selection.

The short format requires adding a new `RenderMode` (`ModeShort`) and a `RenderShort` dispatch method that produces one-line summaries per entity type. Section filtering is cleanly isolated: detail renderers already render sections sequentially in `strings.Builder`, so a set of allowed section names can gate each section block. Width adaptation requires a column priority system where per-entity-type ordered lists determine which fields survive at narrow widths. The freshness footer reads `sync.GetLastSuccessfulSyncTime()` (already exists in `internal/sync/status.go`) and appends to every terminal response. Shell completions are static script endpoints at `/ui/completions/{bash,zsh}` plus a search endpoint at `/ui/completions/search` returning newline-delimited plain text.

**Primary recommendation:** Implement features in dependency order: freshness footer first (cross-cutting, affects all responses), then short format (new mode + simple formatters), section filtering (gate existing section blocks), width adaptation (column priority system), and completions last (new routes + static scripts).

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** One line per entity with key identity + primary metric, pipe-delimited
- **D-02:** Network: `AS13335 | Cloudflare, Inc. | Open | 304 IXs`
- **D-03:** IX: `DE-CIX Frankfurt | 900 peers | Frankfurt, DE`
- **D-04:** Facility: `Equinix DC1 | 85 nets | Ashburn, US`
- **D-05:** Other types follow same pattern: name, key metric, location/identifier
- **D-06:** Accept both short and long aliases: `ix` or `exchanges`, `fac` or `facilities`, `net` or `networks`, `carrier` or `carriers`, `campus` or `campuses`, `contact` or `contacts`, `prefix` or `prefixes`
- **D-07:** Comma-separated: `?section=ix,fac` shows only IX presences and facilities
- **D-08:** Only applies to detail views -- filters which collapsible sections render
- **D-09:** Progressive column dropping at narrow widths -- least-important columns dropped first
- **D-10:** Values stay full-length -- never truncated with ellipsis
- **D-11:** Column priority order defined per entity type (most important fields survive narrowest widths)
- **D-12:** No minimum width enforcement -- render what fits, drop what doesn't
- **D-13:** Every terminal response includes footer: `Data: 2026-03-25T14:30:00Z (12 minutes ago)`
- **D-14:** ISO 8601 timestamp + human-readable relative age
- **D-15:** Reads from sync metadata (last successful sync time)
- **D-16:** Server-side search-as-you-type completion -- completion script calls `/ui/completions/search?q=<prefix>&type=net` on each tab press
- **D-17:** Completion endpoint returns matching entity names/ASNs as newline-delimited plain text
- **D-18:** Both bash and zsh completion scripts downloadable from `/ui/completions/bash` and `/ui/completions/zsh`
- **D-19:** Help text includes alias/function setup instructions
- **D-20:** Search adds ~100ms latency per tab press -- acceptable tradeoff

### Claude's Discretion
- Exact column priority ordering per entity type for width adaptation
- Completion script implementation details (bash vs zsh completion API differences)
- Freshness footer formatting (separator line, color, placement)
- Alias/function examples in help text

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| DIF-01 | One-line summary mode (`?format=short`) outputs single-line entity summary | New `ModeShort` in detect.go, `RenderShort` dispatch in renderer.go, per-entity formatters |
| DIF-02 | Data freshness timestamp footer on all terminal responses | `sync.GetLastSuccessfulSyncTime()` already exists; inject footer in renderPage after all terminal writes |
| DIF-03 | Section filtering (`?section=ix,fac`) renders only requested sections | Parse comma-separated query param, pass section set to renderers, gate section blocks |
| DIF-04 | Width parameter (`?w=N`) adapts table rendering to specified column width | Column priority definitions per entity type, progressive dropping in list-section renderers |
| SHL-01 | Bash completion script downloadable from server | Static script served at `/ui/completions/bash`, uses `COMPREPLY` + `curl` to `/ui/completions/search` |
| SHL-02 | Zsh completion script downloadable from server | Static script served at `/ui/completions/zsh`, uses `compadd` + `curl` to `/ui/completions/search` |
| SHL-03 | Shell alias/function setup instructions in help text | Add to `RenderHelp()` output and completion script comments |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **CS-5 (MUST):** Use input structs for functions receiving more than 2 arguments
- **CS-6 (SHOULD):** Declare function input structs before the function consuming them
- **T-1 (MUST):** Table-driven tests; deterministic and hermetic by default
- **T-2 (MUST):** Run `-race` in CI; add `t.Cleanup` for teardown
- **T-3 (SHOULD):** Mark safe tests with `t.Parallel()`
- **ERR-1 (MUST):** Wrap errors with `%w` and context
- **API-1 (MUST):** Document exported items: `// Foo does ...`
- **API-2 (MUST):** Accept interfaces where variation needed; return concrete types
- **OBS-1 (MUST):** Structured logging (`slog`) with levels and consistent fields

## Architecture Patterns

### Current Rendering Pipeline

The rendering pipeline flows: `handler.dispatch()` -> `handleXDetail()` -> `renderPage()` -> `termrender.Detect()` -> `termrender.Renderer.RenderPage()`.

Key dispatch points:

1. **detect.go `Detect()`**: Maps `?format=` query param to `RenderMode`. Currently handles `plain`, `json`, `whois`. Must add `short`.
2. **render.go `renderPage()`**: Switches on mode to select Rich/Plain/JSON/WHOIS/HTML/HTMX branches. Must add Short branch and freshness footer injection.
3. **renderer.go `RenderPage()`**: Type-switches on data to dispatch to entity-specific renderers. Must add `RenderShort()` parallel dispatch.

### New Mode: ModeShort

Add `ModeShort` to the `RenderMode` enum in detect.go. The `Detect()` function's `?format=` switch gains a `"short"` case returning `ModeShort`. In `renderPage()`, `ModeShort` gets its own case that:
1. Sets `Content-Type: text/plain; charset=utf-8`
2. Creates a renderer
3. Calls `renderer.RenderShort(w, page.Data)` which type-switches and calls per-entity `formatShortX()` functions

### Short Format per Entity Type (D-01 through D-05)

Each entity type produces a single pipe-delimited line:

```go
// Network: "AS13335 | Cloudflare, Inc. | Open | 304 IXs"
func formatShortNetwork(data templates.NetworkDetail) string {
    return fmt.Sprintf("AS%d | %s | %s | %d IXs", data.ASN, data.Name, data.PolicyGeneral, data.IXCount)
}

// IX: "DE-CIX Frankfurt | 900 peers | Frankfurt, DE"
func formatShortIX(data templates.IXDetail) string {
    return fmt.Sprintf("%s | %d peers | %s", data.Name, data.NetCount, formatLocation(data.City, data.Country))
}

// Facility: "Equinix DC1 | 85 nets | Ashburn, US"
func formatShortFacility(data templates.FacilityDetail) string {
    return fmt.Sprintf("%s | %d nets | %s", data.Name, data.NetCount, formatLocation(data.City, data.Country))
}

// Org: "Cloudflare, Inc. | 3 nets | 0 facs"
func formatShortOrg(data templates.OrgDetail) string {
    return fmt.Sprintf("%s | %d nets | %d facs", data.Name, data.NetCount, data.FacCount)
}

// Campus: "Ashburn Campus | 5 facs | Ashburn, US"
func formatShortCampus(data templates.CampusDetail) string {
    return fmt.Sprintf("%s | %d facs | %s", data.Name, data.FacCount, formatLocation(data.City, data.Country))
}

// Carrier: "Zayo | 12 facs"
func formatShortCarrier(data templates.CarrierDetail) string {
    return fmt.Sprintf("%s | %d facs", data.Name, data.FacCount)
}
```

### Section Filtering Architecture (D-06 through D-08)

**Section alias map:** A `map[string]string` normalizes aliases to canonical names:

```go
var sectionAliases = map[string]string{
    "ix": "ix", "exchanges": "ix",
    "fac": "fac", "facilities": "fac",
    "net": "net", "networks": "net",
    "carrier": "carrier", "carriers": "carrier",
    "campus": "campus", "campuses": "campus",
    "contact": "contact", "contacts": "contact",
    "prefix": "prefix", "prefixes": "prefix",
}
```

**Parse function:** `ParseSections(raw string) map[string]bool` splits on comma, normalizes via alias map, returns canonical section set. Empty raw string means "show all" (nil map).

**Renderer integration:** Pass `sections map[string]bool` to `RenderPage`/each entity renderer. Before each section block, check `sections == nil || sections["ix"]`. This is a minimal change: each renderer gains a conditional guard around its existing section blocks.

**Where to parse:** In `renderPage()`, extract `r.URL.Query().Get("section")`, parse into section set, pass through to renderer methods. The Renderer struct could gain an optional `Sections` field, or it can be passed as an argument to `RenderPage`. Passing as argument is cleaner since it avoids mutable state on the Renderer.

### Width Adaptation Architecture (D-09 through D-12)

**Column priority per entity type:** Each entity's list-section renderer (IX presences, participants, facilities, etc.) currently renders fields inline in the `strings.Builder`. For width adaptation, define column priorities:

```go
// NetworkIXColumns defines field priority for network IX presences (highest = survives narrowest).
// Priority 1 (name) is always shown. Lower priorities are dropped as width decreases.
type ColumnPriority struct {
    Name     string
    MinWidth int // Approximate minimum width needed when this column is included
}

// Network IX presences column priority:
// 1. IX Name (always)
// 2. Speed (almost always)
// 3. IPv4 address
// 4. Cross-ref link
// 5. RS badge
// 6. IPv6 address
```

**Implementation approach:** Rather than calculating exact column widths (complex, fragile), use a threshold-based system:
- Define which fields are included at which `?w=` breakpoints
- At `w >= 120`: all fields
- At `w >= 100`: drop IPv6
- At `w >= 80`: drop IPv6 + cross-ref links
- At `w >= 60`: drop IPv6 + cross-ref + RS badge
- At `w < 60`: name + speed only

**Key constraint (D-10):** Values are never truncated. Only entire columns/fields are dropped.

**Where width flows:** Parse `?w=N` in `renderPage()`, pass to Renderer (or as argument). Each section renderer checks width to decide which fields to include. The Renderer struct could store width as a field since it's immutable per request.

### Freshness Footer Architecture (D-13 through D-15)

**Data source:** `sync.GetLastSuccessfulSyncTime(ctx, db)` in `internal/sync/status.go` returns `(time.Time, error)`. The `Handler` already has `db *sql.DB`.

**Injection point:** In `renderPage()`, after the main content is written for terminal modes (Rich, Plain, Short), append the freshness footer. This requires the function to have access to `db`.

**Current gap:** `renderPage()` is a package-level function that does not receive `db`. Two options:
1. Make `renderPage` a method on `Handler` (has `db` field)
2. Pass `db` as an additional parameter
3. Pre-fetch freshness time in each handler and include it in `PageContent`

Option 3 is cleanest: add `Freshness time.Time` to `PageContent`. Each handler sets it from a shared helper. The freshness footer is then appended in `renderPage()` for all terminal modes.

**Note:** `RenderHelp()` already accepts `freshness time.Time` and formats it. Currently the Home handler passes `time.Time{}` because it lacks db access. With the `PageContent.Freshness` field, all handlers can pass the real value.

**Footer format (per D-13, D-14):**
```
Data: 2026-03-25T14:30:00Z (12 minutes ago)
```

Use `StyleMuted` to render, with a separator line above. Place after the final newline of entity output.

### Shell Completion Architecture (D-16 through D-20)

**New routes in handler.go dispatch:**
```
/ui/completions/bash    -> serve static bash completion script
/ui/completions/zsh     -> serve static zsh completion script
/ui/completions/search  -> newline-delimited search results
```

**Completion search endpoint:** Accepts `?q=<prefix>&type=<net|ix|fac|org|campus|carrier>`. Returns matching names as newline-delimited plain text. Limit to 20 results. Uses existing `SearchService` query methods (already per-type via `queryNetworks`, `queryIXPs`, etc.). For networks, also return ASN in format `AS13335 - Cloudflare`.

**Bash completion script pattern:**
```bash
#!/bin/bash
# PeeringDB Plus shell completions
# Install: eval "$(curl -s peeringdb-plus.fly.dev/ui/completions/bash)"
# Or save: curl -s peeringdb-plus.fly.dev/ui/completions/bash > ~/.pdb-completions.sh
#          source ~/.pdb-completions.sh

_PDB_HOST="${PDB_HOST:-peeringdb-plus.fly.dev}"

pdb() {
  curl -s "${_PDB_HOST}/ui/$@"
}

_pdb_completions() {
  local cur="${COMP_WORDS[$COMP_CWORD]}"
  local prev="${COMP_WORDS[$COMP_CWORD-1]}"

  case "$prev" in
    asn|net)
      COMPREPLY=($(compgen -W "$(curl -sf "${_PDB_HOST}/ui/completions/search?q=${cur}&type=net" 2>/dev/null)" -- "$cur"))
      ;;
    ix)
      COMPREPLY=($(compgen -W "$(curl -sf "${_PDB_HOST}/ui/completions/search?q=${cur}&type=ix" 2>/dev/null)" -- "$cur"))
      ;;
    fac)
      COMPREPLY=($(compgen -W "$(curl -sf "${_PDB_HOST}/ui/completions/search?q=${cur}&type=fac" 2>/dev/null)" -- "$cur"))
      ;;
    pdb)
      COMPREPLY=($(compgen -W "asn ix fac org campus carrier compare" -- "$cur"))
      ;;
  esac
}

complete -F _pdb_completions pdb
```

**Zsh completion script pattern:**
```zsh
#!/bin/zsh
# PeeringDB Plus shell completions for zsh
# Install: eval "$(curl -s peeringdb-plus.fly.dev/ui/completions/zsh)"
# Or save to a file in your fpath

_PDB_HOST="${PDB_HOST:-peeringdb-plus.fly.dev}"

pdb() {
  curl -s "${_PDB_HOST}/ui/$@"
}

_pdb() {
  local -a subcmds
  subcmds=(asn ix fac org campus carrier compare)

  _arguments \
    '1:entity type:(${subcmds})' \
    '2:identifier:->ident'

  case $state in
    ident)
      local type="${words[2]}"
      local -a completions
      completions=(${(f)"$(curl -sf "${_PDB_HOST}/ui/completions/search?q=${words[3]}&type=${type}" 2>/dev/null)"})
      compadd -a completions
      ;;
  esac
}

compdef _pdb pdb
```

**Key design choices:**
- Scripts define the `pdb()` wrapper function (D-19)
- `PDB_HOST` env var allows customization
- `curl -sf` (silent + fail) suppresses errors gracefully when offline
- Scripts are served as `text/plain` with `Content-Disposition: inline`

### Recommended Project Structure (New/Modified Files)

```
internal/web/
  termrender/
    short.go          # NEW: RenderShort() + per-entity formatShortX()
    short_test.go     # NEW: tests for short format
    sections.go       # NEW: ParseSections(), section alias map
    sections_test.go  # NEW: tests for section parsing
    width.go          # NEW: column priority definitions, width helpers
    width_test.go     # NEW: tests for width adaptation
    freshness.go      # NEW: FormatFreshness() helper
    freshness_test.go # NEW: tests for freshness formatting
    detect.go         # MODIFY: add ModeShort
    detect_test.go    # MODIFY: add ModeShort tests
    renderer.go       # MODIFY: add RenderShort dispatch, section/width params
    network.go        # MODIFY: add section filtering + width to RenderNetworkDetail
    ix.go             # MODIFY: add section filtering + width to RenderIXDetail
    facility.go       # MODIFY: add section filtering + width to RenderFacilityDetail
    org.go            # MODIFY: add section filtering + width to RenderOrgDetail
    campus.go         # MODIFY: add section filtering (minor, only one section)
    carrier.go        # MODIFY: add section filtering (minor, only one section)
    help.go           # MODIFY: add completion setup instructions
  handler.go          # MODIFY: add completions/* routes to dispatch
  completions.go      # NEW: completion handlers (bash, zsh scripts, search endpoint)
  completions_test.go # NEW: tests for completion endpoints
  render.go           # MODIFY: add ModeShort case, freshness footer, pass sections/width
  detail.go           # MODIFY: set PageContent.Freshness in each handler
```

### Anti-Patterns to Avoid

- **Mutable renderer state between requests:** Do not store sections/width on a shared Renderer. Create a new Renderer per request (already the pattern).
- **Truncating values:** D-10 explicitly forbids truncation with ellipsis. Only drop entire columns.
- **Hardcoded host in completion scripts:** Use `${PDB_HOST:-peeringdb-plus.fly.dev}` for customizability.
- **Blocking completion search:** The search endpoint must be fast (~100ms). Use existing indexed search, limit results to 20.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| ANSI stripping for plain mode | Custom ANSI parser | colorprofile.Writer (already used) | Handles all escape sequences correctly |
| Relative time formatting | Custom relative time | `time.Since().Truncate(time.Minute)` + manual format | stdlib is sufficient for this simple case |
| Section alias normalization | Complex parser | Simple `map[string]string` lookup | Only 7 entity types, no wildcards needed |

## Common Pitfalls

### Pitfall 1: Freshness Footer in JSON Mode
**What goes wrong:** Appending a text footer to JSON output breaks JSON parsing.
**Why it happens:** Footer injection is too broad.
**How to avoid:** Only append freshness footer in Rich, Plain, and Short modes. JSON and WHOIS get no footer. JSON can include freshness as a `"_meta"` field if desired.
**Warning signs:** `jq` fails to parse JSON responses.

### Pitfall 2: Section Filtering on Non-Detail Views
**What goes wrong:** `?section=ix` on search or compare views causes confusion.
**Why it happens:** Section filtering logic is applied too broadly.
**How to avoid:** D-08 explicitly states filtering only applies to detail views. In `renderPage()`, only pass sections to `RenderPage()`, not to `RenderSearch()`/`RenderCompare()`.
**Warning signs:** Search results disappear when `?section=` is present.

### Pitfall 3: Width Parameter Affecting Header KV Section
**What goes wrong:** Width adaptation changes the key-value header layout, which has fixed `labelWidth=19`.
**Why it happens:** Width logic applied to header section, not just list sections.
**How to avoid:** Width adaptation only affects list sections (IX presences, participants, facilities, etc.) where multiple columns exist. The KV header is always rendered at full width since labels + values fit in any reasonable terminal.
**Warning signs:** Header fields missing at narrow widths.

### Pitfall 4: Completion Script eval Injection
**What goes wrong:** If entity names contain shell metacharacters, completion could inject commands.
**Why it happens:** Completion results are interpolated into shell variables.
**How to avoid:** The completion search endpoint must sanitize output -- strip or escape shell metacharacters from entity names. Alternatively, return only alphanumeric + basic punctuation.
**Warning signs:** Tab completion causes unexpected shell behavior.

### Pitfall 5: Renderer Signature Explosion
**What goes wrong:** Adding `sections`, `width`, `freshness` parameters to every renderer method creates unwieldy signatures.
**Why it happens:** Each feature adds a parameter.
**How to avoid:** Store `sections`, `width` as fields on the Renderer struct (per-request, not shared). The Renderer is already created fresh per request in `renderPage()`. Add `Sections map[string]bool` and `Width int` fields. This follows CS-5 since we avoid >2 args.
**Warning signs:** Methods with 5+ parameters.

### Pitfall 6: ASN Completion Search Performance
**What goes wrong:** Searching networks by ASN prefix (e.g., "133") could be slow if treated as string search.
**Why it happens:** ASN is stored as integer, not string. LIKE queries on integer columns don't use indexes.
**How to avoid:** For the completion endpoint, detect numeric-only queries and use `WHERE asn >= X AND asn < Y` range queries for ASN prefix matching. For name queries, use existing `LIKE` search.
**Warning signs:** Completion takes >500ms for numeric queries.

## Code Examples

### Adding ModeShort to Detect

```go
// In detect.go - add to RenderMode const block:
// ModeShort renders a single-line summary for scripting.
ModeShort

// In Detect() switch:
case "short":
    return ModeShort
```

### Freshness Footer Helper

```go
// In freshness.go:

// FormatFreshness returns a styled freshness footer line.
// Returns empty string if t is zero (no sync has completed).
func FormatFreshness(t time.Time) string {
    if t.IsZero() {
        return ""
    }
    age := time.Since(t).Truncate(time.Minute)
    var ageStr string
    switch {
    case age < time.Minute:
        ageStr = "just now"
    case age < time.Hour:
        ageStr = fmt.Sprintf("%d minutes ago", int(age.Minutes()))
    case age < 24*time.Hour:
        ageStr = fmt.Sprintf("%d hours ago", int(age.Hours()))
    default:
        ageStr = fmt.Sprintf("%d days ago", int(age.Hours()/24))
    }
    return StyleMuted.Render(fmt.Sprintf("Data: %s (%s)", t.UTC().Format(time.RFC3339), ageStr))
}
```

### Section Filtering in Renderer

```go
// In renderer.go - updated RenderPage signature concept:
func (r *Renderer) RenderPage(w io.Writer, title string, data any) error {
    // r.Sections and r.Width are set on the Renderer before calling RenderPage
    // ...existing type switch, each renderer checks r.Sections internally
}

// In network.go - section gating:
if r.Sections == nil || r.Sections["ix"] {
    // render IX presences section
}
if r.Sections == nil || r.Sections["fac"] {
    // render facilities section
}
```

### PageContent Freshness Addition

```go
// In render.go:
type PageContent struct {
    Title     string
    Content   templ.Component
    Data      any
    Freshness time.Time  // Last successful sync time for footer
}

// In detail.go - each handler:
freshness, _ := sync.GetLastSuccessfulSyncTime(r.Context(), h.db)
page := PageContent{
    Title:     net.Name,
    Content:   templates.NetworkDetailPage(data),
    Data:      data,
    Freshness: freshness,
}
```

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) |
| Config file | None needed |
| Quick run command | `go test ./internal/web/termrender/... -race -count=1` |
| Full suite command | `go test ./internal/web/... -race -count=1` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| DIF-01 | Short format produces one-line pipe-delimited output per entity | unit | `go test ./internal/web/termrender/... -run TestRenderShort -race` | Wave 0 |
| DIF-02 | Freshness footer appended to terminal responses | unit | `go test ./internal/web/termrender/... -run TestFormatFreshness -race` | Wave 0 |
| DIF-03 | Section filtering omits non-requested sections | unit | `go test ./internal/web/termrender/... -run TestSectionFilter -race` | Wave 0 |
| DIF-04 | Width adaptation drops columns progressively | unit | `go test ./internal/web/termrender/... -run TestWidth -race` | Wave 0 |
| SHL-01 | Bash completion script served at /ui/completions/bash | unit | `go test ./internal/web/... -run TestCompletionBash -race` | Wave 0 |
| SHL-02 | Zsh completion script served at /ui/completions/zsh | unit | `go test ./internal/web/... -run TestCompletionZsh -race` | Wave 0 |
| SHL-03 | Help text includes alias/function setup | unit | `go test ./internal/web/termrender/... -run TestHelpCompletion -race` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/web/termrender/... -race -count=1`
- **Per wave merge:** `go test ./internal/web/... -race -count=1`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/web/termrender/short_test.go` -- covers DIF-01
- [ ] `internal/web/termrender/freshness_test.go` -- covers DIF-02
- [ ] `internal/web/termrender/sections_test.go` -- covers DIF-03
- [ ] `internal/web/termrender/width_test.go` -- covers DIF-04
- [ ] `internal/web/completions_test.go` -- covers SHL-01, SHL-02, SHL-03

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Static completions (hardcoded lists) | Server-side dynamic completion via HTTP | Common in 2024+ CLI tools | Always-fresh results without client-side caching |
| Fixed-width terminal output | Adaptive width via query param | wttr.in popularized ?w= pattern | Users can customize output to their terminal |

## Open Questions

1. **Renderer method signatures with sections/width**
   - What we know: Each renderer needs access to sections and width
   - What's unclear: Whether to use Renderer struct fields vs function parameters
   - Recommendation: Use Renderer struct fields (`Sections`, `Width`) set by `renderPage()` before calling `RenderPage()`. This avoids signature changes on all 6+ renderer methods and follows the existing pattern where `mode` and `noColor` are already Renderer fields.

2. **Freshness in JSON mode**
   - What we know: Text footer cannot be appended to JSON
   - What's unclear: Whether to add `_meta` field or omit freshness entirely from JSON
   - Recommendation: Omit freshness from JSON output. Keep it simple -- JSON consumers can check a dedicated `/api/status` endpoint if they need sync metadata.

3. **Completion search for ASNs**
   - What we know: Networks have both name and ASN as identifiers
   - What's unclear: Whether completion should return `AS13335 - Cloudflare` or just `13335`
   - Recommendation: For `type=net`, return `AS{asn}\t{name}` format (tab-separated). Bash/zsh completions can display the full line but complete only the ASN portion.

## Sources

### Primary (HIGH confidence)
- Existing codebase: `internal/web/termrender/` package (Phases 28-30) -- all renderer patterns
- Existing codebase: `internal/sync/status.go` -- `GetLastSuccessfulSyncTime()` function
- Existing codebase: `internal/web/render.go` -- rendering pipeline and mode dispatch
- Existing codebase: `internal/web/search.go` -- `SearchService` and per-type query methods
- Existing codebase: `internal/web/handler.go` -- route dispatch pattern

### Secondary (MEDIUM confidence)
- [GNU Bash Reference - Programmable Completion](https://www.gnu.org/software/bash/manual/html_node/A-Programmable-Completion-Example.html) -- COMPREPLY, COMP_WORDS, complete -F
- [Zsh Completion System](https://zsh.sourceforge.io/Doc/Release/Completion-System.html) -- compdef, compadd, _arguments
- [zsh-completions howto](https://github.com/zsh-users/zsh-completions/blob/master/zsh-completions-howto.org) -- practical zsh completion patterns
- [Bash completion tutorial](https://opensource.com/article/18/3/creating-bash-completion-script) -- COMPREPLY patterns

### Tertiary (LOW confidence)
- [wttr.in bash function](https://wttr.in/:bash.function) -- inspiration for shell function pattern (could not fetch content)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all features use existing Go stdlib + termrender patterns, no new dependencies
- Architecture: HIGH -- clear extension points in existing rendering pipeline, well-understood patterns
- Pitfalls: HIGH -- identified from direct code analysis and shell completion conventions
- Shell completions: MEDIUM -- bash/zsh completion APIs are well-documented but exact integration with HTTP-backed search needs testing

**Research date:** 2026-03-26
**Valid until:** 2026-04-26 (stable domain, no fast-moving dependencies)
