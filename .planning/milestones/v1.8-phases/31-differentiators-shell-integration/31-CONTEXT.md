# Phase 31: Differentiators & Shell Integration - Context

**Gathered:** 2026-03-25
**Status:** Ready for planning

<domain>
## Phase Boundary

Power-user features: one-line summary mode, section filtering, width control, data freshness footer, and downloadable bash/zsh completion scripts with server-side search.

</domain>

<decisions>
## Implementation Decisions

### Short Format (?format=short)
- **D-01:** One line per entity with key identity + primary metric, pipe-delimited
- **D-02:** Network: `AS13335 | Cloudflare, Inc. | Open | 304 IXs`
- **D-03:** IX: `DE-CIX Frankfurt | 900 peers | Frankfurt, DE`
- **D-04:** Facility: `Equinix DC1 | 85 nets | Ashburn, US`
- **D-05:** Other types follow same pattern: name, key metric, location/identifier

### Section Filtering (?section=)
- **D-06:** Accept both short and long aliases: `ix` or `exchanges`, `fac` or `facilities`, `net` or `networks`, `carrier` or `carriers`, `campus` or `campuses`, `contact` or `contacts`, `prefix` or `prefixes`
- **D-07:** Comma-separated: `?section=ix,fac` shows only IX presences and facilities
- **D-08:** Only applies to detail views — filters which collapsible sections render

### Width Adaptation (?w=N)
- **D-09:** Progressive column dropping at narrow widths — least-important columns dropped first (e.g., IPv6 before IPv4, path before name)
- **D-10:** Values stay full-length — never truncated with ellipsis
- **D-11:** Column priority order defined per entity type (most important fields survive narrowest widths)
- **D-12:** No minimum width enforcement — render what fits, drop what doesn't

### Data Freshness Footer
- **D-13:** Every terminal response includes footer: `Data: 2026-03-25T14:30:00Z (12 minutes ago)`
- **D-14:** ISO 8601 timestamp + human-readable relative age
- **D-15:** Reads from sync metadata (last successful sync time)

### Shell Completions
- **D-16:** Server-side search-as-you-type completion — completion script calls `/ui/completions/search?q=<prefix>&type=net` on each tab press
- **D-17:** Completion endpoint returns matching entity names/ASNs as newline-delimited plain text
- **D-18:** Both bash and zsh completion scripts downloadable from `/ui/completions/bash` and `/ui/completions/zsh`
- **D-19:** Help text includes alias/function setup instructions (e.g., `pdb() { curl -s "peeringdb-plus.fly.dev/ui/$@" }`)
- **D-20:** Search adds ~100ms latency per tab press — acceptable tradeoff for always-fresh results

### Claude's Discretion
- Exact column priority ordering per entity type for width adaptation
- Completion script implementation details (bash vs zsh completion API differences)
- Freshness footer formatting (separator line, color, placement)
- Alias/function examples in help text

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements
- `.planning/REQUIREMENTS.md` — DIF-01 through DIF-04, SHL-01 through SHL-03 define differentiator and shell integration requirements

### Existing Code
- `internal/sync/` — Sync worker stores last sync time, needed for freshness footer
- `internal/web/handler.go` — Route dispatcher, add completion endpoints
- Phase 28 rendering framework — termrender package, format detection
- Phase 29-30 renderers — section filtering hooks into existing renderer structure

### Shell Completion References
- bash-completion project conventions — `_complete` function registration
- zsh completion system — `#compdef` and `_arguments` patterns

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- Sync metadata (last sync time) already tracked in sync worker — expose for freshness footer
- Search endpoint already exists (`/ui/search?q=...`) — adapt for completion (return plain text instead of HTML)
- Entity type renderers from Phases 29-30 — add section filtering hooks

### Established Patterns
- Query parameter parsing already used for `?q=`, `?format=`, `?T` — extend for `?section=`, `?w=`
- One-line format from Phase 29 — `?format=short` uses same compact representation

### Integration Points
- Freshness footer injected at the end of every terminal response by the rendering framework
- Section filtering applied in termrender before rendering lists
- Width parameter passed to termrender for column adaptation
- New `/ui/completions/` routes registered in handler.go

</code_context>

<specifics>
## Specific Ideas

- Shell completions should feel like native CLI tools — instant, responsive, no user-visible delay beyond network latency
- Width adaptation should degrade gracefully — even at 40 columns, output should be usable
- Freshness footer reassures users the data is current without them having to check

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 31-differentiators-shell-integration*
*Context gathered: 2026-03-25*
