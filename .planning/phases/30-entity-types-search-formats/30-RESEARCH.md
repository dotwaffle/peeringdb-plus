# Phase 30: Entity Types, Search & Formats - Research

**Researched:** 2026-03-26
**Domain:** Terminal rendering (ANSI/plain/JSON/WHOIS) for all PeeringDB entity types, search results, and ASN comparison
**Confidence:** HIGH

## Summary

Phase 30 extends the terminal rendering system established in Phases 28-29 to cover all remaining entity types (IX, Facility, Organization, Campus, Carrier), search results, ASN comparison, and three output format modes (plain text, JSON, WHOIS). The codebase is well-structured for this expansion: the `termrender` package has clear patterns from `RenderNetworkDetail`, all 6 detail data structs exist in `templates/detailtypes.go`, all fragment handlers contain the query logic for child entity rows, and the `RenderPage` type-switch is ready for new cases.

The primary challenge is volume, not complexity. There are 5 entity type renderers to write, each needing: (1) eager-loading of child rows in the detail handler, (2) new fields on the detail struct, (3) a renderer function following the network pattern, and (4) WHOIS format output. The WHOIS format requires mapping PeeringDB fields to RPSL-like attribute classes per RFC 2622. Search and compare renderers consume already-computed data structures (`[]SearchGroup` and `*CompareData`).

**Primary recommendation:** Organize into 3 plans: (1) rich/minimal entity type renderers + handler eager-loading, (2) search + compare terminal renderers, (3) WHOIS format + detect.go extension. Each is independently testable.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Two layout categories -- rich and minimal
- **D-02:** Rich types: Network (Phase 29), IX, Facility -- full header with all key fields + detailed one-line lists with relevant metrics (speed, IPs, peer counts, net counts)
- **D-03:** Minimal types: Org, Campus, Carrier -- compact header with key identity fields + simple name-only lists of child entities
- **D-04:** All types use same structural pattern (header + lists) but rich types have more fields and data per list entry
- **D-05:** Results grouped by entity type with headers: "Networks (N results)", "IXPs (N results)", etc.
- **D-06:** One line per result with entity name, key identifier (ASN for networks), and curl path
- **D-07:** Match web UI's result count per type (10)
- **D-08:** JSON returns identical JSON shape as the REST API (`/rest/v1/{type}/{id}`). No new schema -- just a convenience shortcut.
- **D-09:** For search results, JSON returns the same grouped structure as the search API
- **D-10:** Strict RPSL compliance where possible
- **D-11:** Networks map to `aut-num` RPSL class with proper fields (aut-num, as-name, descr, admin-c, tech-c, etc.)
- **D-12:** IXes map to custom `ix:` class. Facilities map to `site:` class. Fill available RPSL-compatible fields, leave unavailable ones empty.
- **D-13:** Orgs, Campuses, Carriers use best-fit RPSL-inspired classes
- **D-14:** Multi-value fields use repeated keys (RPSL convention): `ix: DE-CIX Frankfurt` repeated per IX
- **D-15:** Include `% Source: PeeringDB-Plus` comment header and `% Query: {query}` line
- **D-16:** Identical layout to ANSI output but with ASCII box drawing and no ANSI escape codes
- **D-17:** Consistent across all entity types -- not just networks

### Claude's Discretion
- IX detail: which fields in header, how to present participant list
- Facility detail: how to present address and network/IX/carrier lists
- Org/Campus/Carrier: exact fields in minimal header
- ASN comparison terminal layout specifics
- RPSL field mapping details for non-network types

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| RND-03 | IX detail (/ui/ix/{id}) renders with participant table, facility list, prefix list | Rich layout pattern from network renderer; IXDetail struct + IXParticipantRow/IXFacilityRow/IXPrefixRow types exist; handler eager-loading needed |
| RND-04 | Facility detail (/ui/fac/{id}) renders with address, network/IX/carrier lists | Rich layout; FacilityDetail struct + FacNetworkRow/FacIXRow/FacCarrierRow types exist; handler eager-loading needed |
| RND-05 | Org detail (/ui/org/{id}) renders with child entity lists | Minimal layout; OrgDetail struct + 5 child row types exist; handler eager-loading needed |
| RND-06 | Campus detail (/ui/campus/{id}) renders with facility list | Minimal layout; CampusDetail struct + CampusFacilityRow type exists; handler eager-loading needed |
| RND-07 | Carrier detail (/ui/carrier/{id}) renders with facility list | Minimal layout; CarrierDetail struct + CarrierFacilityRow type exists; handler eager-loading needed |
| RND-08 | Search results (/ui/?q=...) render as grouped text list for terminal clients | SearchGroup/SearchResult types already populated; RenderPage needs type-switch case; handleHome needs Data field set |
| RND-09 | ASN comparison renders shared IXPs/facilities/campuses | CompareData struct already fully populated by CompareService; handleCompare already passes Data; needs renderer |
| RND-10 | Plain text mode (?T) produces identical layout with ASCII box drawing, no ANSI codes | Already working for network via colorprofile.NoTTY stripping; new renderers automatically inherit this |
| RND-11 | JSON mode (?format=json) outputs same data structures as JSON | RenderJSON already works for any `any` type; detail structs need child row fields for complete JSON output |
| RND-17 | WHOIS-style output mode (?format=whois) using RPSL-like key-value format | New RenderMode + detect.go case + per-entity WHOIS renderer functions; RFC 2622 aut-num class for networks |
</phase_requirements>

## Architecture Patterns

### Extension Points in Existing Code

The codebase has clear extension points that this phase needs to modify:

#### 1. Type-Switch in RenderPage (renderer.go:66-83)
Currently handles only `templates.NetworkDetail`. Must add cases for: `templates.IXDetail`, `templates.FacilityDetail`, `templates.OrgDetail`, `templates.CampusDetail`, `templates.CarrierDetail`, `[]templates.SearchGroup`, `*templates.CompareData`.

```go
// Current pattern to follow:
func (r *Renderer) RenderPage(w io.Writer, title string, data any) error {
    switch d := data.(type) {
    case templates.NetworkDetail:
        return r.RenderNetworkDetail(w, d)
    // Phase 30 adds:
    case templates.IXDetail:
        return r.RenderIXDetail(w, d)
    case templates.FacilityDetail:
        return r.RenderFacilityDetail(w, d)
    // ... etc
    }
}
```

#### 2. Detail Struct Child Row Fields (detailtypes.go)
Phase 29 pattern: `NetworkDetail` has `IXPresences []NetworkIXLanRow` and `FacPresences []NetworkFacRow` with `json:"...,omitempty"` tags. Each detail struct needs similar fields for its child entities.

**IX needs:** Participants (`[]IXParticipantRow`), Facilities (`[]IXFacilityRow`), Prefixes (`[]IXPrefixRow`)
**Facility needs:** Networks (`[]FacNetworkRow`), IXPs (`[]FacIXRow`), Carriers (`[]FacCarrierRow`)
**Org needs:** Networks (`[]OrgNetworkRow`), IXPs (`[]OrgIXRow`), Facilities (`[]OrgFacilityRow`), Campuses (`[]OrgCampusRow`), Carriers (`[]OrgCarrierRow`)
**Campus needs:** Facilities (`[]CampusFacilityRow`)
**Carrier needs:** Facilities (`[]CarrierFacilityRow`)

#### 3. Handler Eager-Loading (detail.go)
Phase 29 pattern in `handleNetworkDetail`: queries child entities and populates the row slice fields on the detail struct. Each handler needs similar eager-loading. The query logic already exists in fragment handlers -- it just needs to be duplicated into the main detail handler (same queries, but unconditional for terminal/JSON, conditional on mode detection if desired for performance).

#### 4. Detect.go Extension for WHOIS Mode
Current `Detect()` handles `?format=plain` and `?format=json`. Must add `?format=whois` returning a new `ModeWHOIS` constant, and the `renderPage` function in `render.go` needs a case for it.

#### 5. Search Data Path Gap
`handleHome` currently sets `Title: "Home"` and does NOT set `Data` on `PageContent`, so terminal users hitting `/ui/?q=equinix` get help text instead of search results. Must be fixed: when `query != ""` and groups are non-empty, set `Data: groups` and change title (or add RenderPage logic to detect search data).

The `handleSearch` at `/ui/search?q=...` already passes `Data: groups`, so the htmx search path works. But the success criteria specifies `/ui/?q=equinix`, which hits `handleHome`.

### Recommended File Structure

```
internal/web/termrender/
    detect.go       -- add ModeWHOIS constant + ?format=whois case
    renderer.go     -- extend RenderPage type-switch, add RenderSearch, RenderCompare
    network.go      -- existing (Phase 29)
    ix.go           -- NEW: RenderIXDetail (rich layout)
    facility.go     -- NEW: RenderFacilityDetail (rich layout)
    org.go          -- NEW: RenderOrgDetail (minimal layout)
    campus.go       -- NEW: RenderCampusDetail (minimal layout)
    carrier.go      -- NEW: RenderCarrierDetail (minimal layout)
    search.go       -- NEW: RenderSearch
    compare.go      -- NEW: RenderCompare
    whois.go        -- NEW: WHOIS format renderers for all entity types
    styles.go       -- existing (may need minor additions)

    ix_test.go      -- NEW
    facility_test.go -- NEW
    org_test.go     -- NEW
    campus_test.go  -- NEW
    carrier_test.go -- NEW
    search_test.go  -- NEW
    compare_test.go -- NEW
    whois_test.go   -- NEW

internal/web/
    detail.go       -- add eager-loading to 5 handlers
    handler.go      -- fix handleHome search data path
    render.go       -- add ModeWHOIS case in renderPage
    templates/
        detailtypes.go -- add child row slice fields to 5 detail structs
```

### Pattern: Rich Layout (IX, Facility)

Following the network renderer pattern:
1. Title line: `Name` + muted identifier (e.g., location for IX/Facility)
2. Key-value header using `writeKV()` with right-aligned labels
3. Aggregate stats in header (participant count, bandwidth for IX; net/IX/carrier counts for facility)
4. Named sections with counts: `"Participants (N)"`, `"Facilities (N)"`
5. One line per child entity with cross-references via `CrossRef()`
6. Rich entries include metrics (speed, IPs for IX participants; ASN for facility networks)

**IX header fields (Claude's discretion):**
- Organization, Website, City/Country, Region, Media, Protocols (unicast/multicast/IPv6), Participants count, Facilities count, Prefixes count, Aggregate Bandwidth

**IX sections:**
- Participants: NetName AS{ASN} [RS] {speed} {IPv4} / {IPv6} -- uses same SpeedStyle/rsBadge as network
- Facilities: FacName [/ui/fac/{id}] City, Country
- Prefixes: {prefix} {protocol} {inDFZ badge}

**Facility header fields (Claude's discretion):**
- Organization, Campus (if present), Address (formatted), City/State/Country/Zip, Website, CLLI, Region, Networks count, IXPs count, Carriers count

**Facility sections:**
- Networks: NetName AS{ASN} [/ui/asn/{asn}]
- IXPs: IXName [/ui/ix/{id}]
- Carriers: CarrierName [/ui/carrier/{id}]

### Pattern: Minimal Layout (Org, Campus, Carrier)

Compact version of the rich layout:
1. Title line: `Name`
2. Brief key-value header (identity fields only)
3. Simple name-only lists of child entities with cross-references

**Org header fields:** Website, Address, City/Country, child counts (Networks, IXPs, Facilities, Campuses, Carriers)
**Org sections:** Networks (name + ASN), IXPs (name), Facilities (name + location), Campuses (name), Carriers (name)

**Campus header fields:** Organization, Website, City/Country, Facility count
**Campus sections:** Facilities (name + location)

**Carrier header fields:** Organization, Website, Facility count
**Carrier sections:** Facilities (name)

### Pattern: Search Results Terminal Output

```
Search: "equinix"

Networks (15 results)
  Equinix (WAN)  AS47541  /ui/asn/47541
  Equinix LLC  AS21928  /ui/asn/21928
  ...

IXPs (8 results)
  Equinix Chicago  Chicago, US  /ui/ix/81
  Equinix Ashburn  Ashburn, US  /ui/ix/1
  ...

Facilities (45 results)
  Equinix AM1/AM2  Amsterdam, NL  /ui/fac/4
  ...
```

One line per result: `Name  Subtitle  DetailURL` -- using `StyleValue` for name, `StyleMuted` for subtitle, `StyleLink` for URL.

### Pattern: Compare Terminal Output

```
Cloudflare (AS13335) vs Google (AS15169)

Shared IXPs (42)
  DE-CIX Frankfurt [/ui/ix/31]
    AS13335: 100G [RS]  2001:db8::1
    AS15169: 400G  2001:db8::2
  ...

Shared Facilities (18)
  Equinix AM1/AM2 [/ui/fac/4]  Amsterdam, NL
  ...

Shared Campuses (3)
  Equinix Amsterdam [/ui/campus/1]
    Equinix AM1/AM2 [/ui/fac/4]
  ...
```

### Pattern: WHOIS Format

#### Network (aut-num class per RFC 2622 / D-11)
```
% Source: PeeringDB-Plus
% Query: AS13335

aut-num:        AS13335
as-name:        CLOUDFLARENET
descr:          Cloudflare, Inc.
org:            Cloudflare, Inc.
website:        https://www.cloudflare.com
irr-as-set:     AS-CLOUDFLARE
info-type:      NSP
policy:         Open
traffic:        100+ Gbps
ratio:          Mostly Outbound
scope:          Global
prefixes-v4:    600
prefixes-v6:    200
ix-count:       280
fac-count:      300
ix:             DE-CIX Frankfurt
ix:             AMS-IX
fac:            Equinix AM1/AM2
fac:            Equinix LD8
source:         PEERINGDB-PLUS
```

#### IX (custom `ix:` class per D-12)
```
% Source: PeeringDB-Plus
% Query: IX 31

ix:             31
ix-name:        DE-CIX Frankfurt
descr:          German Commercial Internet Exchange
org:            DE-CIX Management GmbH
website:        https://www.de-cix.net
city:           Frankfurt
country:        DE
region:         Europe
media:          Ethernet
proto:          unicast, multicast, IPv6
net-count:      1000
fac-count:      30
prefix-count:   4
bandwidth:      350 Tbps
source:         PEERINGDB-PLUS
```

#### Facility (custom `site:` class per D-12)
```
% Source: PeeringDB-Plus
% Query: FAC 4

site:           4
site-name:      Equinix AM1/AM2
descr:          Equinix Amsterdam AM1/AM2
org:            Equinix, Inc.
address:        Luttenbergweg 4
city:           Amsterdam
country:        NL
region:         Europe
clli:           AMSTNL02
website:        https://www.equinix.com
net-count:      200
ix-count:       15
carrier-count:  8
source:         PEERINGDB-PLUS
```

#### Org/Campus/Carrier (D-13 best-fit classes)
```
% Source: PeeringDB-Plus
% Query: ORG 1234

organisation:   1234
org-name:       Cloudflare, Inc.
address:        101 Townsend St
city:           San Francisco
country:        US
website:        https://www.cloudflare.com
net-count:      2
fac-count:      300
ix-count:       280
source:         PEERINGDB-PLUS
```

### Anti-Patterns to Avoid

- **Conditional eager-loading based on mode detection:** The detail handlers should always eager-load child rows. Checking the request mode in the handler to skip queries adds complexity and the queries are fast on SQLite. Keep it simple.
- **Duplicating query logic:** The fragment handlers already contain the correct query patterns. Extract the query+conversion logic into shared helper functions called by both the fragment handler and the eager-loading path, rather than duplicating the code.
- **Writing WHOIS output using fmt.Fprintf per field:** Use a helper function like `writeWHOIS(buf, key, value)` that handles RPSL formatting (key padding, empty value skip, multi-value handling).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| ANSI stripping for plain mode | Custom regex stripping | `colorprofile.Writer` with `NoTTY` profile | Already working; new renderers inherit it automatically via `r.Write()` |
| Speed/policy/RS badge styling | Per-renderer color logic | Existing `SpeedStyle()`, `PolicyStyle()`, `rsBadge`, `CrossRef()` from network.go | These are already extracted as package-level helpers |
| JSON serialization | Custom JSON builders | `RenderJSON()` (already in renderer.go) | Works with any struct; just ensure structs have proper json tags |
| Search result grouping | Manual type iteration | Existing `SearchService.Search()` returns `[]TypeResult` already grouped | The search service handles all 6 types with errgroup fan-out |

## Common Pitfalls

### Pitfall 1: handleHome Not Passing Search Data to Terminal
**What goes wrong:** Terminal users hitting `/ui/?q=equinix` get help text instead of search results
**Why it happens:** `handleHome` sets `Title: "Home"` without `Data`, and `renderPage` dispatches "Home" to `RenderHelp()`
**How to avoid:** When query is non-empty with results, set `Data: groups` and use a title like "Search" or add a new switch case in renderPage
**Warning signs:** `curl "/ui/?q=equinix"` shows help text instead of results

### Pitfall 2: IX Participant Queries Must Go Through IxLan
**What goes wrong:** Querying `InternetExchange -> NetworkIxLan` directly fails -- there's no direct edge
**Why it happens:** PeeringDB data model: `InternetExchange -> IxLan -> NetworkIxLan`. The IxLan is an intermediary.
**How to avoid:** Follow the existing fragment handler pattern: `IxLan.Query().Where(ixlan.HasInternetExchangeWith(...)).QueryNetworkIxLans()`
**Warning signs:** Compilation errors on edge traversal; empty result sets

### Pitfall 3: Missing JSON Tags on New Detail Struct Fields
**What goes wrong:** `?format=json` output doesn't include child entity rows or has wrong field names
**Why it happens:** Adding slice fields to detail structs without proper `json:"...,omitempty"` tags
**How to avoid:** Follow Phase 29 pattern: `IXPresences []NetworkIXLanRow \`json:"ixPresences,omitempty"\``
**Warning signs:** JSON output has uppercase Go field names or includes empty arrays

### Pitfall 4: WHOIS Key Alignment
**What goes wrong:** WHOIS output has inconsistent key padding, breaking RPSL parsers
**Why it happens:** Different key lengths without consistent padding
**How to avoid:** RPSL convention is key followed by colon, then spaces to column 16 (or at least 1 space). Use `fmt.Sprintf("%-16s%s", key+":", value)` for consistent formatting
**Warning signs:** Output doesn't look like standard whois output; keys and values don't align

### Pitfall 5: Compare Data Type Mismatch
**What goes wrong:** `handleCompare` passes `*templates.CompareData` but type-switch expects value type
**Why it happens:** Compare handler sets `Data: data` where `data` is `*templates.CompareData` (pointer)
**How to avoid:** Use pointer type in the type-switch: `case *templates.CompareData:`
**Warning signs:** Falls through to default case in RenderPage

### Pitfall 6: Large IX Participant Lists in WHOIS
**What goes wrong:** WHOIS output for large IXes (1000+ participants) produces massive output
**Why it happens:** WHOIS format uses repeated keys for multi-value fields
**How to avoid:** This is actually correct behavior for RPSL format. Don't truncate -- WHOIS consumers expect all entries. The ANSI renderer already handles large lists (benchmarked in Phase 29).
**Warning signs:** None -- this is expected behavior

### Pitfall 7: Search Path Inconsistency
**What goes wrong:** `/ui/search?q=...` and `/ui/?q=...` produce different results for terminal clients
**Why it happens:** `handleSearch` passes `Data: groups` and `Title: "Search"`, while `handleHome` doesn't pass data
**How to avoid:** Ensure both paths produce identical terminal output when a search query is present
**Warning signs:** Different output from the two URLs with same query

## Code Examples

### Renderer Function (Rich Entity -- IX)
```go
// Source: follows network.go pattern
func (r *Renderer) RenderIXDetail(w io.Writer, data templates.IXDetail) error {
    var buf strings.Builder
    buf.Grow(len(data.Participants)*120 + 500)

    // Title line
    buf.WriteString(StyleHeading.Render(data.Name))
    if data.City != "" || data.Country != "" {
        buf.WriteString("  ")
        buf.WriteString(StyleMuted.Render(formatLocation(data.City, data.Country)))
    }
    buf.WriteString("\n")

    // Key-value header
    writeKV(&buf, "Organization", styledVal(data.OrgName), labelWidth)
    writeKV(&buf, "Website", styledVal(data.Website), labelWidth)
    // ... more fields
    writeKV(&buf, "Participants", StyleValue.Render(strconv.Itoa(data.NetCount)), labelWidth)
    if data.AggregateBW > 0 {
        writeKV(&buf, "Aggregate Bandwidth", StyleValue.Render(FormatBandwidth(data.AggregateBW)), labelWidth)
    }

    // Participants section (rich -- with speed, IPs, RS badge)
    if len(data.Participants) > 0 {
        buf.WriteString("\n")
        buf.WriteString(StyleHeading.Render(fmt.Sprintf("Participants (%d)", len(data.Participants))))
        buf.WriteString("\n")
        for _, row := range data.Participants {
            buf.WriteString("  ")
            buf.WriteString(StyleValue.Render(row.NetName))
            buf.WriteString(" ")
            buf.WriteString(CrossRef(fmt.Sprintf("/ui/asn/%d", row.ASN)))
            // ... speed, RS badge, IPs (same pattern as network IX presences)
            buf.WriteString("\n")
        }
    }

    buf.WriteString("\n")
    return r.Write(w, buf.String())
}
```

### Renderer Function (Minimal Entity -- Carrier)
```go
// Source: follows network.go pattern, simplified per D-03
func (r *Renderer) RenderCarrierDetail(w io.Writer, data templates.CarrierDetail) error {
    var buf strings.Builder

    buf.WriteString(StyleHeading.Render(data.Name))
    buf.WriteString("\n")

    writeKV(&buf, "Organization", styledVal(data.OrgName), labelWidth)
    writeKV(&buf, "Website", styledVal(data.Website), labelWidth)
    writeKV(&buf, "Facilities", StyleValue.Render(strconv.Itoa(data.FacCount)), labelWidth)

    // Simple name-only list (minimal layout per D-03)
    if len(data.Facilities) > 0 {
        buf.WriteString("\n")
        buf.WriteString(StyleHeading.Render(fmt.Sprintf("Facilities (%d)", len(data.Facilities))))
        buf.WriteString("\n")
        for _, row := range data.Facilities {
            buf.WriteString("  ")
            buf.WriteString(StyleValue.Render(row.FacName))
            buf.WriteString(" ")
            buf.WriteString(CrossRef(fmt.Sprintf("/ui/fac/%d", row.FacID)))
            buf.WriteString("\n")
        }
    }

    buf.WriteString("\n")
    return r.Write(w, buf.String())
}
```

### WHOIS Helper
```go
// Source: new file whois.go
const whoisKeyWidth = 16

func writeWHOISField(buf *strings.Builder, key, value string) {
    if value == "" {
        return
    }
    buf.WriteString(fmt.Sprintf("%-*s%s\n", whoisKeyWidth, key+":", value))
}

func writeWHOISMulti(buf *strings.Builder, key string, values []string) {
    for _, v := range values {
        writeWHOISField(buf, key, v)
    }
}

func writeWHOISHeader(buf *strings.Builder, query string) {
    buf.WriteString("% Source: PeeringDB-Plus\n")
    buf.WriteString(fmt.Sprintf("%% Query: %s\n", query))
    buf.WriteString("\n")
}
```

### Eager-Loading in Handler
```go
// Source: follows handleNetworkDetail pattern in detail.go
// Add to handleIXDetail after existing code:
participants, err := h.client.IxLan.Query().
    Where(ixlan.HasInternetExchangeWith(internetexchange.ID(id))).
    QueryNetworkIxLans().
    WithNetwork().
    Order(networkixlan.ByAsn()).
    All(r.Context())
if err == nil {
    rows := make([]templates.IXParticipantRow, len(participants))
    for i, nix := range participants {
        // ... same conversion as handleIXParticipantsFragment
    }
    data.Participants = rows
}
```

### Detect.go WHOIS Mode
```go
// Add to RenderMode constants:
ModeWHOIS // after ModeJSON

// Add to Detect() switch on format:
case "whois":
    return ModeWHOIS

// Add to renderPage in render.go:
case termrender.ModeWHOIS:
    w.Header().Set("Vary", "HX-Request, User-Agent, Accept")
    w.Header().Set("Content-Type", "text/plain; charset=utf-8")
    renderer := termrender.NewRenderer(mode, false) // no color concept for WHOIS
    return renderer.RenderWHOIS(w, page.Title, page.Data)
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Fragment handlers only (lazy htmx) | Eager-load for terminal/JSON + lazy for web | Phase 29 | Terminal renderers need data pre-loaded in handler |
| Generic stub for non-network types | Type-switch to entity-specific renderers | Phase 29 | Each type gets its own renderer function |
| Two modes (Rich, Plain) + JSON | Three text modes (Rich, Plain, WHOIS) + JSON | Phase 30 | New ModeWHOIS in detect.go and renderPage |

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib testing |
| Config file | none -- stdlib |
| Quick run command | `go test ./internal/web/termrender/ -run TestRender -race` |
| Full suite command | `go test ./internal/web/termrender/ -race -count=1` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| RND-03 | IX detail terminal render | unit | `go test ./internal/web/termrender/ -run TestRenderIXDetail -race` | Wave 0 |
| RND-04 | Facility detail terminal render | unit | `go test ./internal/web/termrender/ -run TestRenderFacilityDetail -race` | Wave 0 |
| RND-05 | Org detail terminal render | unit | `go test ./internal/web/termrender/ -run TestRenderOrgDetail -race` | Wave 0 |
| RND-06 | Campus detail terminal render | unit | `go test ./internal/web/termrender/ -run TestRenderCampusDetail -race` | Wave 0 |
| RND-07 | Carrier detail terminal render | unit | `go test ./internal/web/termrender/ -run TestRenderCarrierDetail -race` | Wave 0 |
| RND-08 | Search results terminal render | unit | `go test ./internal/web/termrender/ -run TestRenderSearch -race` | Wave 0 |
| RND-09 | Compare terminal render | unit | `go test ./internal/web/termrender/ -run TestRenderCompare -race` | Wave 0 |
| RND-10 | Plain text mode | unit | `go test ./internal/web/termrender/ -run TestPlainMode -race` | Wave 0 |
| RND-11 | JSON mode | unit | `go test ./internal/web/termrender/ -run TestRenderJSON -race` | Existing (renderer_test.go) |
| RND-17 | WHOIS format | unit | `go test ./internal/web/termrender/ -run TestRenderWHOIS -race` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/web/termrender/ -race -count=1`
- **Per wave merge:** `go test ./internal/web/... -race -count=1`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/web/termrender/ix_test.go` -- covers RND-03
- [ ] `internal/web/termrender/facility_test.go` -- covers RND-04
- [ ] `internal/web/termrender/org_test.go` -- covers RND-05
- [ ] `internal/web/termrender/campus_test.go` -- covers RND-06
- [ ] `internal/web/termrender/carrier_test.go` -- covers RND-07
- [ ] `internal/web/termrender/search_test.go` -- covers RND-08
- [ ] `internal/web/termrender/compare_test.go` -- covers RND-09
- [ ] `internal/web/termrender/whois_test.go` -- covers RND-17

## Project Constraints (from CLAUDE.md)

Directives that affect this phase:

- **CS-5 (MUST):** Input structs for functions with >2 args. New renderer functions take `(w io.Writer, data Type)` -- only 2 args, compliant.
- **CS-2 (MUST):** Avoid stutter. Package is `termrender`, so functions are `RenderIXDetail` not `TermRenderIXDetail`.
- **ERR-1 (MUST):** Wrap errors with `%w`. New renderer functions return `error` from `r.Write()`.
- **T-1 (MUST):** Table-driven tests. Follow the pattern from `network_test.go`.
- **T-2 (MUST):** `-race` flag in tests.
- **T-3 (SHOULD):** `t.Parallel()` on safe tests.
- **API-1 (MUST):** Document exported items. Each new `Render*` function needs doc comment.
- **OBS-1 (MUST):** Structured logging with slog. Eager-loading errors in handlers should use `slog.Error()`.
- **MD-1 (SHOULD):** Prefer stdlib. No new dependencies needed -- uses existing lipgloss/colorprofile.

## Open Questions

1. **Format of address in facility WHOIS output**
   - What we know: Facility has Address1, Address2, City, State, Country, Zipcode
   - What's unclear: Whether to use single `address:` field with comma-separated or multiple RPSL address lines
   - Recommendation: Use RIPE-style multi-line address: `address: 101 Townsend St\naddress: San Francisco, CA 94107\naddress: US`

2. **Search WHOIS format**
   - What we know: WHOIS is entity-focused (one object per query), search returns multiple entities
   - What's unclear: Whether `?format=whois` on search results makes sense
   - Recommendation: Return a minimal listing format with one line per result (not full RPSL objects), or return 404/unsupported for WHOIS on search. Let the planner decide.

3. **Compare WHOIS format**
   - What we know: Compare is a custom view showing two networks' overlap
   - What's unclear: No RPSL class maps to comparison
   - Recommendation: Same as search -- either skip WHOIS for compare, or produce a simple key-value listing. Not every view needs every format.

## Sources

### Primary (HIGH confidence)
- Codebase analysis: `internal/web/termrender/` (Phase 28-29 implementation)
- Codebase analysis: `internal/web/detail.go` (all 6 handlers + fragment handlers)
- Codebase analysis: `internal/web/templates/detailtypes.go` (all detail structs + row types)
- Codebase analysis: `internal/web/search.go` (SearchService with errgroup fan-out)
- Codebase analysis: `internal/web/compare.go` (CompareService with set intersection)
- Codebase analysis: `internal/web/handler.go` (dispatch, handleHome, handleSearch, handleCompare)
- Codebase analysis: `internal/web/render.go` (renderPage mode dispatch)

### Secondary (MEDIUM confidence)
- [RFC 2622 - RPSL](https://www.rfc-editor.org/rfc/rfc2622) - aut-num class definition, RPSL syntax
- [RIPE Database RPSL Object Types](https://docs.db.ripe.net/RPSL-Object-Types/Descriptions-of-Primary-Objects) - Real-world RPSL object examples

### Tertiary (LOW confidence)
- RPSL field mapping for non-network types (ix:, site:, organisation:) -- these are custom PeeringDB-specific classes inspired by RPSL conventions but not standardized in RFC 2622

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - all dependencies already in use, no new packages needed
- Architecture: HIGH - direct extension of Phase 29 patterns with clear codebase evidence
- Pitfalls: HIGH - identified from code inspection, not speculation
- WHOIS format: MEDIUM - RPSL spec is clear for networks (aut-num), custom classes for other types are a design choice

**Research date:** 2026-03-26
**Valid until:** 2026-04-25 (stable -- codebase patterns established, no external dependency changes)
