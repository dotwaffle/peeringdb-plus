# Phase 29: Network Detail (Reference Implementation) - Context

**Gathered:** 2026-03-25
**Status:** Ready for planning

<domain>
## Phase Boundary

Network entity terminal renderer with whois-style header, IX/facility lists, colored speed tiers, and cross-reference paths. This is the reference implementation that Phase 30 will follow for remaining entity types.

</domain>

<decisions>
## Implementation Decisions

### Header Layout
- **D-01:** Compact key-value pairs with aligned values, no border box. Human-readable labels ("Peering Policy" not "info_policy").
- **D-02:** Key fields: Name, ASN, Type, Peering Policy (color-coded), Website, IX Count, Fac Count, Prefixes v4, Prefixes v6, Aggregate Bandwidth
- **D-03:** Peering policy color-coded: Open=green, Selective=yellow, Restrictive=red

### Table Design (IX Presences / Facilities)
- **D-04:** Compact one-line per entry format — no Unicode table borders, no column headers. Each IX/facility on a single line with key info.
- **D-05:** IX presence line format: `{IX Name} [{path}]  {speed}  {IPv4} / {IPv6}` with speed color-coded by tier
- **D-06:** Facility line format: `{Fac Name} [{path}]  {City}, {Country}`
- **D-07:** Route server peers marked with colored [RS] badge after IX name

### Cross-References
- **D-08:** Inline path after entity name in square brackets: `DE-CIX Frankfurt [/ui/ix/31]`
- **D-09:** Paths are curl-ready — user can copy-paste to follow up

### Layout Categories
- **D-10:** Network is a "rich" type — full header with all key fields, detailed one-line lists with speed/IP/RS data
- **D-11:** This phase establishes the pattern for rich types (Network, IX, Facility). Phase 30 follows this for IX and Facility, and uses a minimal variant for Org, Campus, Carrier.

### Claude's Discretion
- Exact field ordering in header
- Spacing and alignment details
- How to handle missing/empty fields (omit vs show as empty)
- Section headers between IX presences and facilities

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements
- `.planning/REQUIREMENTS.md` — RND-02, RND-12, RND-13, RND-14, RND-15, RND-16 define network detail rendering

### Existing Code
- `internal/web/detail.go` — `handleNetworkDetail()` (line ~28) — data fetching logic to reuse
- `internal/web/templates/detailtypes.go` — `NetworkDetail` struct with all display fields
- `internal/web/templates/detail_net.templ` — HTML network detail template — shows which fields are displayed
- Phase 28 rendering framework — `internal/web/termrender/` package and `renderPage()` third branch

### Color Mapping
- `internal/web/templates/components.templ` — `speedColor()` and `policyBadge()` functions define existing web UI color tiers

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `templates.NetworkDetail` struct — already populated with Name, ASN, InfoType, Policy, Website, IXCount, FacCount, Prefixes, AggregateBandwidth
- `speedColor()` in components.templ — maps speed tiers to Tailwind colors, needs 256-color ANSI equivalent
- `handleNetworkDetail()` — fetches all data including eager-loaded IX presences, facilities, contacts

### Established Patterns
- Speed tier colors: sub-1G gray, 1G neutral, 10G blue, 100G emerald, 400G+ amber
- RS badge: emerald colored [RS] inline after IX name
- Aggregate bandwidth: sum of all IX presence speeds, displayed in header

### Integration Points
- `renderPage()` third branch (from Phase 28) passes `NetworkDetail` struct to termrender package
- termrender package receives typed data struct, returns formatted text

</code_context>

<specifics>
## Specific Ideas

- Output should feel like running `whois AS13335` — familiar to network engineers
- One-line-per-entry format keeps output compact and pipeable to grep/awk

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 29-network-detail-reference-implementation*
*Context gathered: 2026-03-25*
