# Feature Landscape

**Domain:** Curl-friendly terminal CLI interface for PeeringDB data
**Researched:** 2026-03-25
**Milestone:** v1.8

## Table Stakes

Features that users of curl-friendly terminal services universally expect. These are established by wttr.in, cheat.sh, ifconfig.co, and similar services, adapted to the network engineering domain. Missing any of these makes the terminal interface feel broken or incomplete.

### Terminal Client Detection

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| User-Agent-based terminal detection | Every curl-friendly service (wttr.in, ifconfig.co, cheat.sh) auto-detects terminal clients by User-Agent. Users expect `curl peeringdb-plus.fly.dev/ui/asn/13335` to just work without passing headers. | Low | Match substrings (case-insensitive): `curl`, `wget`, `httpie`, `go-http-client`, `python-requests`, `xh`, `fetch`, `powershell`, `lwp-request`, `aiohttp`, `nushell`. Empty UA also triggers terminal mode. |
| Content negotiation on existing URLs | wttr.in serves ANSI at `wttr.in/London` for curl and HTML at the same URL for browsers. No separate endpoints. `/ui/asn/13335` must serve terminal output for curl and HTML for browsers. | Medium | Architectural keystone. Existing `renderPage()` already branches on `HX-Request` for htmx fragments. Add a third branch for terminal clients. Set `Vary: User-Agent, Accept, HX-Request`. |
| Explicit format override via query parameter | wttr.in uses `?T` for plain text, cheat.sh uses `?T` to disable highlighting. Users must be able to force a format regardless of User-Agent. | Low | Three formats: `?T` (plain text, no ANSI), `?format=json` (JSON), default (ANSI for terminals). `?T` shorthand is muscle-memory for users of curl-friendly services. |
| `Accept` header as secondary signal | Programmatic clients use `Accept: text/plain` or `Accept: application/json` to negotiate format. Standard HTTP content negotiation. | Low | Priority: `?format=` query param > `?T` > `Accept` header > User-Agent detection. |

### Output Formatting

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Rich ANSI output with 256-color | wttr.in uses ANSI escape sequences extensively. Network engineers' terminals universally support 256-color. Box-drawing characters and color-coded sections make data scannable. | Medium | lipgloss v2 + colorprofile.ANSI256 for server-side rendering. Unicode box-drawing via lipgloss border types. Color scheme should match existing web UI accent colors. |
| Plain text fallback mode (`?T`) | Pipeable output without ANSI codes for `grep`, `awk`, `cut`. Table stakes for any terminal service. | Low | colorprofile.ASCII strips all escape codes. lipgloss ASCIIBorder for tables. Same data layout, ASCII characters only. |
| JSON output mode (`?format=json`) | Machine-readable structured output. Users want `curl ... \| jq .policy_general` to extract fields. | Low | Marshal the same detail data structs as JSON. Use `encoding/json` stdlib. |
| Whois-style key-value layout for headers | Network engineers' mental model for "looking up an ASN" is shaped by WHOIS output: key-value pairs, colon-separated, with section grouping. bgp.tools WHOIS and peeringdb-py both use this format. | Medium | Format: `ASN:            13335` with fixed-width labels (left-padded to align colons). Group related fields with blank line separators. This is the primary format for detail page headers. |
| Formatted tables for list sections | IX presences, facility lists, participant lists need columnar display with headers and borders. | Medium | lipgloss/table with StyleFunc for per-cell coloring. Column widths computed from data. Headers, separators, and borders. |
| Speed formatting with color tiers | Port speeds as human-readable (1G, 10G, 100G, 400G) with the 5-tier color scheme from v1.7 web UI. | Low | Map Tailwind colors to ANSI 256-color palette: sub-1G = 245 (gray), 1G = 250 (neutral), 10G = 33 (blue), 100G = 35 (emerald), 400G+ = 214 (amber). |

### Navigation and Help

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Help text at `/ui/` root | When a terminal client hits `/ui/` with no path, show available endpoints, query parameters, and example curl commands. This is the discoverability mechanism. | Low | Static text template. Include endpoint list with examples, query parameter reference (`?T`, `?format=json`). Format as clean, readable text -- not heavy on ANSI color. |
| Text-formatted error pages | Terminal clients hitting `/ui/nonexistent` must not get HTML garbage. Return: `404 Not Found: No network with ASN 99999`. | Low | Short, informative, one-line errors. Include a hint: `Try: curl peeringdb-plus.fly.dev/ui/` to guide to help page. |
| Correct HTTP status codes | Terminal clients rely on `curl -f` failing on 4xx/5xx. Status codes must be accurate. | Low | Already correct in existing handlers. Must not regress through terminal rendering path. |

### Detail Pages for All 6 Entity Types

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Network detail (`/ui/asn/{asn}`) | The primary use case. Shows name, ASN, org, policy, traffic, IX presences, facility presences, contacts. | Medium | Header: whois-style key-value. IX presences: table with IX name, speed, IPv4, IPv6, RS status. Facility presences: table. Contacts: table. |
| IX detail (`/ui/ix/{id}`) | IX info, location, protocols, participant count, participant table, facility list, prefix list. | Medium | Participants table potentially 1000+ rows. Render all rows -- terminal users expect it and can pipe to `less -R`. |
| Facility detail (`/ui/fac/{id}`) | Facility record: name, address, CLLI, network list, IX list, carrier list. | Medium | Address formatting: combine fields into multi-line block. |
| Organization detail (`/ui/org/{id}`) | Org record: name, address, network list, facility list, IX list, campus list, carrier list. | Medium | Container entity -- primarily lists of owned sub-entities. |
| Campus detail (`/ui/campus/{id}`) | Campus: name, org, location, facility list. | Low | Simplest entity type. Few fields, short facility list. |
| Carrier detail (`/ui/carrier/{id}`) | Carrier: name, org, facility list. | Low | Similar to campus. |

### Search

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Terminal search results (`/ui/?q=cloudflare`) | Terminal users need to discover entity IDs. Search returns grouped results matching web UI. | Medium | Grouped list: type header ("Networks (3)"), then results with name, subtitle, URL path. ANSI mode: colored type headers. 10 per type. |

## Differentiators

Features that set this apart from "just curl the REST API." Not expected, but create significant value.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| ASN comparison in terminal | `curl .../ui/compare/13335/32934` shows shared IXPs, facilities, campuses. No other terminal tool offers this. | Medium | Three tables: shared IXPs, shared facilities, shared campuses. Side-by-side data with colored columns per network. |
| One-line summary mode | `curl ".../ui/asn/13335?format=short"` returns `AS13335 Cloudflare, Inc. \| Open \| 150 IXs \| 47 Facs`. Useful for scripts, ChatOps. | Low | Single-line formatter per entity type. Pipeable, greppable. |
| Section filtering | `curl ".../ui/asn/13335?section=ix"` returns only IX presences. Reduces output noise for scripts. | Low | Parse `?section=` param, render only requested sections. Combinable: `?section=ix,fac`. |
| RS badge indicator | Route server peers marked with colored `[RS]` badge in IX presence tables. Plain text: `*` suffix. | Low | Operationally important for peering decisions. |
| Color-coded peering policy | Open = green, Selective = yellow, Restrictive = red, No Policy = gray. Instant visual signal. | Low | Policy status is constantly scanned by network engineers. |
| Updated timestamp footer | "Data last synced: 2026-03-25T14:00:00Z (2 hours ago)" at bottom of every response. | Low | Users need data freshness awareness for a periodic-sync mirror. |
| Width parameter (`?w=N`) | Tables adapt to specified width. Default 80 columns. | Medium | lipgloss table Width(n) with wrapping. Useful for wide terminals. |

## Anti-Features

Features to explicitly NOT build.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| Interactive TUI (Bubble Tea) | This is server-side HTTP, not a client-side TUI. User runs `curl`, not a Go binary. | Render static ANSI text that curl pipes to stdout. |
| Terminal width auto-detection | Server has no terminal. Cannot detect client width via HTTP. | Default 80 columns. Accept `?w=N` query parameter. |
| Pager support | Cannot control client-side pager from HTTP response. | Document `curl ... \| less -R` in help text. |
| Client-side CLI binary | Out of scope. This is zero-setup -- curl only. | gRPC/ConnectRPC API exists for programmatic access. |
| Streaming/live updates | curl is request-response. | ConnectRPC streaming RPCs exist for bulk export. |
| TrueColor (24-bit) default | Not universally supported (screen, some multiplexers). | 256-color is sufficient and universal. |
| Custom color themes | Complexity for minimal gain. | Ship one well-designed color palette. |
| Markdown output | Format proliferation. | JSON for machines, ANSI/text for humans. |
| WHOIS protocol (port 43) | Separate application, Fly.io doesn't easily expose TCP ports. | HTTP-only via curl. |
| CSV/TSV output | Format proliferation. | JSON with jq for extraction. |

## Feature Dependencies

```
Terminal client detection (User-Agent parsing)
  |
  +--> Content negotiation (branches rendering path)
  |      |
  |      +--> ANSI renderer (all entity types, search, compare, errors)
  |      |      |
  |      |      +--> Whois-style key-value formatter (header sections)
  |      |      +--> lipgloss/table formatter (list sections)
  |      |      +--> Color mapping (speed tiers, policy, entity type accents)
  |      |      +--> Unicode box-drawing (lipgloss borders)
  |      |
  |      +--> Plain text renderer (same layout, ASCII-only via colorprofile.ASCII)
  |      |
  |      +--> JSON renderer (marshals view-model structs)
  |
  +--> Help text (terminal root endpoint)
  +--> Error formatting (404, 500 for terminal clients)

Format override (?T, ?format=) --> parsed in detection, overrides UA
Section filtering (?section=) --> applied after data load, before rendering
Width control (?w=N) --> passed to table formatter

Existing detail handlers --> provide data (no changes needed)
  |
  +--> Network detail data --> terminal network renderer
  +--> IX detail data --> terminal IX renderer
  +--> Facility detail data --> terminal facility renderer
  +--> Org detail data --> terminal org renderer
  +--> Campus detail data --> terminal campus renderer
  +--> Carrier detail data --> terminal carrier renderer
  +--> Search results data --> terminal search renderer
  +--> Compare data --> terminal compare renderer
```

Key dependency: Content negotiation is the single chokepoint. Everything flows through "is this a terminal client?" followed by "which format?".

Critical non-dependency: The existing detail handlers already load all needed data into typed structs (`NetworkDetail`, `IXDetail`, etc.). Terminal renderers consume these SAME structs. No new database queries or data loading logic needed.

## MVP Recommendation

### Phase 1: Detection + Infrastructure + Help/Errors

1. **Terminal client detection** (low, unlocks everything)
2. **Content negotiation with Vary header** (medium, architectural keystone)
3. **Style palette + shared rendering utilities** (low, colors/borders/table helpers)
4. **Help text at `/ui/`** for terminal clients (low, discoverability)
5. **Text error pages** (low, prevents HTML garbage)

### Phase 2: Network Detail (Reference Implementation)

1. **Network detail ANSI rendering** (medium, establishes patterns)
2. **Whois-style key-value formatter** (medium, shared utility)
3. **Table formatter for IX/fac presences** (medium, shared utility)
4. **ANSI color mapping** for speed tiers and policy (low)

### Phase 3: Remaining Entity Types + Search

1. **IX detail** (medium, large participant tables)
2. **Facility, Org, Campus, Carrier** (low-medium, follow network pattern)
3. **Search results** (medium)
4. **Plain text mode** (`?T`) for all types (low, colorprofile.ASCII)
5. **JSON mode** for all pages (low)

### Phase 4: Differentiators

1. **ASN comparison** terminal rendering (medium, high value)
2. **One-line summary mode** (low, high utility for scripts)
3. **Section filtering** (low)
4. **Updated timestamp footer** (low)

Defer: Width parameter, custom formatting modes.

### Phase Ordering Rationale

- **Detection first:** Terminal detection is the gate for all other work. Content negotiation must be proven before building on it.
- **Network detail second:** Most complex entity (most fields, IX/fac relations). Establishes ANSI rendering patterns. If network renderer works, other types are mechanical.
- **Remaining types third:** Leverages patterns from network. Search needed for discoverability. JSON/plain text can be added across all types at once.
- **Differentiators last:** High-value but not core. Can ship incrementally.

## Sources

- [wttr.in](https://github.com/chubin/wttr.in) -- Reference curl-friendly service pattern
- [cheat.sh](https://github.com/chubin/cheat.sh) -- `?T` plain text flag convention
- [bgp.tools](https://bgp.tools/kb/api) -- Network engineering terminal conventions
- [ifconfig.co](https://ifconfig.co/) -- Content negotiation: plain text for curl, HTML for browsers
- [curl User-Agent docs](https://everything.curl.dev/http/modify/user-agent.html) -- Default `curl/VERSION` format
- [lipgloss/table API](https://pkg.go.dev/charm.land/lipgloss/v2/table) -- Table rendering capabilities
- Existing codebase: `internal/web/handler.go`, `internal/web/render.go`, `internal/web/templates/detailtypes.go`
