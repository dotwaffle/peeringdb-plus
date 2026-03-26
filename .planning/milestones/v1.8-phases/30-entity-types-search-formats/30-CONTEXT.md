# Phase 30: Entity Types, Search & Formats - Context

**Gathered:** 2026-03-25
**Status:** Ready for planning

<domain>
## Phase Boundary

Terminal renderers for remaining 5 entity types (IX, Facility, Org, Campus, Carrier), search results, ASN comparison, plus plain text, JSON, and WHOIS output formats. Follows the patterns established in Phase 29.

</domain>

<decisions>
## Implementation Decisions

### Layout Categories
- **D-01:** Two layout categories — rich and minimal
- **D-02:** Rich types: Network (Phase 29), IX, Facility — full header with all key fields + detailed one-line lists with relevant metrics (speed, IPs, peer counts, net counts)
- **D-03:** Minimal types: Org, Campus, Carrier — compact header with key identity fields + simple name-only lists of child entities
- **D-04:** All types use same structural pattern (header + lists) but rich types have more fields and data per list entry

### Search Results
- **D-05:** Results grouped by entity type with headers: "Networks (N results)", "IXPs (N results)", etc.
- **D-06:** One line per result with entity name, key identifier (ASN for networks), and curl path
- **D-07:** Match web UI's result count per type (10)

### JSON Output (?format=json)
- **D-08:** Returns identical JSON shape as the REST API (`/rest/v1/{type}/{id}`). No new schema — just a convenience shortcut.
- **D-09:** For search results, JSON returns the same grouped structure as the search API

### WHOIS Output (?format=whois)
- **D-10:** Strict RPSL compliance where possible
- **D-11:** Networks map to `aut-num` RPSL class with proper fields (aut-num, as-name, descr, admin-c, tech-c, etc.)
- **D-12:** IXes map to custom `ix:` class. Facilities map to `site:` class. Fill available RPSL-compatible fields, leave unavailable ones empty.
- **D-13:** Orgs, Campuses, Carriers use best-fit RPSL-inspired classes
- **D-14:** Multi-value fields use repeated keys (RPSL convention): `ix: DE-CIX Frankfurt` repeated per IX
- **D-15:** Include `% Source: PeeringDB-Plus` comment header and `% Query: {query}` line

### Plain Text (?T / ?format=plain)
- **D-16:** Identical layout to ANSI output but with ASCII box drawing and no ANSI escape codes
- **D-17:** Consistent across all entity types — not just networks

### Claude's Discretion
- IX detail: which fields in header, how to present participant list
- Facility detail: how to present address and network/IX/carrier lists
- Org/Campus/Carrier: exact fields in minimal header
- ASN comparison terminal layout specifics
- RPSL field mapping details for non-network types

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements
- `.planning/REQUIREMENTS.md` — RND-03 through RND-11, RND-17 define entity type rendering, search, and format requirements

### Existing Code
- `internal/web/detail.go` — All 6 entity detail handlers + search + compare logic
- `internal/web/templates/detailtypes.go` — All detail data structs (IXDetail, FacilityDetail, OrgDetail, CampusDetail, CarrierDetail)
- `internal/web/search.go` — SearchService with errgroup fan-out across 6 types
- `internal/web/compare.go` — CompareService with set intersection logic
- Phase 29 network renderer — reference implementation to follow

### RPSL Reference
- RFC 2622 (RPSL) — field names, object classes, attribute syntax for strict compliance

### REST API
- `ent/rest/` — entrest-generated REST handlers, defines the JSON shape to match for ?format=json

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `SearchService` in `internal/web/search.go` — already extracts search results grouped by type with counts
- `CompareService` in `internal/web/compare.go` — already computes shared IXPs/facilities/campuses
- All detail data structs populated by existing handlers — reuse directly
- Phase 29's network renderer — template for IX and Facility rich renderers

### Established Patterns
- errgroup fan-out for parallel search queries (search.go)
- Map-based set intersection for comparison (compare.go)
- One-line-per-entry list format (from Phase 29)

### Integration Points
- termrender package (Phase 28) extended with renderers for each entity type
- `renderPage()` routes to correct renderer based on entity type + format
- `?format=json` can redirect to REST API or inline the response

</code_context>

<specifics>
## Specific Ideas

- WHOIS format should be parseable by tools like `whois` command output parsers that network engineers use
- JSON format using REST API shape means existing scripts work with both endpoints

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 30-entity-types-search-formats*
*Context gathered: 2026-03-25*
