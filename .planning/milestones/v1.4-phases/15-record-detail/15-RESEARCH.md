# Phase 15: Record Detail Pages - Research

**Researched:** 2026-03-24
**Domain:** Go web UI (templ + htmx), ent ORM queries, detail page rendering
**Confidence:** HIGH

## Summary

Phase 15 adds detail pages for all 6 PeeringDB entity types (Network, IXP, Facility, Organization, Campus, Carrier). The existing codebase from Phases 13-14 establishes clear patterns: a wildcard dispatch in `handler.go`, templ components in `internal/web/templates/`, type definitions in the templates package to avoid circular imports, and the `renderPage` helper for full-page vs htmx fragment rendering.

The primary technical challenge is wiring up 6 new URL patterns through the existing dispatch mechanism, building ent queries to fetch each entity plus count queries for summary stats, and creating templ components that render detail pages with collapsible `<details>` sections that lazy-load related records via htmx on first expand. The ent schema already has all the edges and fields needed -- no schema changes are required.

The secondary concern is the IXP participants path. Network participants for an IXP are not directly on the InternetExchange entity -- they go through IxLan (InternetExchange -> ix_lans -> network_ix_lans). Similarly, IXP prefixes go through IxLan (InternetExchange -> ix_lans -> ix_prefixes). Queries must traverse these edges.

**Primary recommendation:** Extend the existing dispatch switch in `handler.go` to match the 6 detail URL path prefixes (asn, ix, fac, org, campus, carrier), add per-type handler methods that fetch the entity + count stats, and create templ components that render the detail header + lazy-loadable `<details>` sections with `hx-trigger="toggle once"`.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **Networks by ASN**: `/ui/asn/13335` -- users think in ASNs, not internal IDs. Dedicated `/ui/asn/` path segment.
- **Other types by PeeringDB ID**: `/ui/ix/456`, `/ui/fac/789`, `/ui/org/123`, `/ui/campus/1`, `/ui/carrier/1`
- 6 detail page types total (matching the 6 searchable types).
- Every type shows all its relationships in collapsible sections (see full relationship map below).
- Related record sections load on first expand via `hx-trigger="toggle"` or `hx-trigger="revealed"` on the `<details>` element.
- Summary stats computed as cheap count queries, loaded with the main page (not lazy-loaded).
- Every related record links to its own detail page. Parent org always linked.

### Claude's Discretion
None specified -- all decisions are locked.

### Deferred Ideas (OUT OF SCOPE)
None specified.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| DETL-01 | User can view a full detail page for any Network, IXP, Facility, Organization, or Campus | URL routing via dispatch switch, per-type handler methods, ent queries for entity lookup, templ detail page components |
| DETL-02 | Related records appear in collapsible sections | HTML `<details>/<summary>` elements with Tailwind styling, one section per relationship edge |
| DETL-03 | Related record sections load on first expand, not on initial page load | htmx `hx-trigger="toggle once"` on `<details>` elements targeting per-section endpoints |
| DETL-04 | Detail pages show computed summary statistics | Count queries via ent client (cheap per-entity counts), rendered in header area |
| DETL-05 | Related records cross-link to their own detail pages | Each row in related sections links to `/ui/{type}/{id}` (or `/ui/asn/{asn}` for networks) |
</phase_requirements>

## Standard Stack

No new dependencies. This phase uses only what Phases 13-14 already established.

### Core (already in project)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| github.com/a-h/templ | v0.3.x | Type-safe HTML templates | Already used for all web UI templates |
| htmx | 2.0.8 | Frontend interactivity | Already served from /static/htmx.min.js |
| entgo.io/ent | v0.14.5 | ORM / entity queries | Already used for all data access |
| net/http (stdlib) | Go 1.26 | HTTP routing | Already used via wildcard dispatch |

**Installation:** None required -- all dependencies already present.

## Architecture Patterns

### Recommended Project Structure
```
internal/web/
  handler.go           # Extend dispatch() switch for detail routes
  detail.go            # NEW: per-type detail handler methods
  detail_test.go       # NEW: handler tests for detail pages
  render.go            # Existing renderPage helper (no changes)
  search.go            # Existing (no changes)
  templates/
    searchtypes.go     # Existing search types
    detailtypes.go     # NEW: data types for detail templates
    detail_net.templ   # NEW: network detail page template
    detail_ix.templ    # NEW: IXP detail page template
    detail_fac.templ   # NEW: facility detail page template
    detail_org.templ   # NEW: organization detail page template
    detail_campus.templ # NEW: campus detail page template
    detail_carrier.templ # NEW: carrier detail page template
    detail_sections.templ # NEW: lazy-loaded related record section fragments
```

### Pattern 1: Dispatch Extension
**What:** Extend the existing `dispatch()` switch in `handler.go` to route detail page URL paths.
**When to use:** All 6 detail page types route through the single `GET /ui/{rest...}` wildcard.
**Example:**
```go
// In handler.go dispatch()
func (h *Handler) dispatch(w http.ResponseWriter, r *http.Request) {
    rest := r.PathValue("rest")
    switch {
    case rest == "" || rest == "/":
        h.handleHome(w, r)
    case rest == "search":
        h.handleSearch(w, r)
    case strings.HasPrefix(rest, "asn/"):
        h.handleNetworkDetail(w, r, strings.TrimPrefix(rest, "asn/"))
    case strings.HasPrefix(rest, "ix/"):
        h.handleIXDetail(w, r, strings.TrimPrefix(rest, "ix/"))
    case strings.HasPrefix(rest, "fac/"):
        h.handleFacilityDetail(w, r, strings.TrimPrefix(rest, "fac/"))
    case strings.HasPrefix(rest, "org/"):
        h.handleOrgDetail(w, r, strings.TrimPrefix(rest, "org/"))
    case strings.HasPrefix(rest, "campus/"):
        h.handleCampusDetail(w, r, strings.TrimPrefix(rest, "campus/"))
    case strings.HasPrefix(rest, "carrier/"):
        h.handleCarrierDetail(w, r, strings.TrimPrefix(rest, "carrier/"))
    // Lazy-load fragment endpoints for related sections:
    case strings.HasPrefix(rest, "fragment/"):
        h.handleFragment(w, r, strings.TrimPrefix(rest, "fragment/"))
    default:
        h.handleNotFound(w, r)
    }
}
```

### Pattern 2: Network Lookup by ASN
**What:** Networks are looked up by ASN (not internal ID) per CONTEXT.md decision.
**When to use:** `/ui/asn/{asn}` handler.
**Example:**
```go
func (h *Handler) handleNetworkDetail(w http.ResponseWriter, r *http.Request, asnStr string) {
    asn, err := strconv.Atoi(asnStr)
    if err != nil {
        h.handleNotFound(w, r)
        return
    }
    net, err := h.client.Network.Query().
        Where(network.Asn(asn)).
        WithOrganization().
        Only(r.Context())
    if err != nil {
        if ent.IsNotFound(err) {
            h.handleNotFound(w, r)
            return
        }
        http.Error(w, "internal server error", http.StatusInternalServerError)
        return
    }
    // Count queries for summary stats
    ixCount, _ := h.client.NetworkIxLan.Query().
        Where(networkixlan.NetID(net.ID)).Count(r.Context())
    facCount, _ := h.client.NetworkFacility.Query().
        Where(networkfacility.NetID(net.ID)).Count(r.Context())
    pocCount, _ := h.client.Poc.Query().
        Where(poc.NetID(net.ID)).Count(r.Context())
    // ... render template
}
```

### Pattern 3: Lazy-Load Related Sections via htmx
**What:** Each related section is a `<details>` element with an htmx attribute that fires a GET request on first toggle.
**When to use:** All related record sections across all 6 detail page types.
**Example (templ):**
```
// In detail_net.templ
templ NetworkIXPresencesSection(netID int) {
    <details class="border border-neutral-700 rounded-lg">
        <summary class="px-4 py-3 cursor-pointer ...">
            IX Presences
        </summary>
        <div
            hx-get={ fmt.Sprintf("/ui/fragment/net/%d/ixlans", netID) }
            hx-trigger="toggle once from:closest details"
            hx-swap="innerHTML"
        >
            <div class="px-4 py-3 text-neutral-500">Loading...</div>
        </div>
    </details>
}
```

The `hx-trigger="toggle once from:closest details"` pattern:
- `toggle` -- fires when the `<details>` element is toggled open or closed
- `once` -- ensures the GET request only fires on the first toggle (no re-fetch)
- `from:closest details` -- listens for the toggle event from the enclosing `<details>` element

### Pattern 4: Fragment Endpoints for Lazy Loading
**What:** Dedicated endpoints that return HTML fragments (no layout wrapper) for related record sections.
**When to use:** Called by htmx from lazy-loaded `<details>` sections.
**Example:**
```go
func (h *Handler) handleFragment(w http.ResponseWriter, r *http.Request, path string) {
    // Parse: "net/{netID}/ixlans", "ix/{ixID}/participants", etc.
    // Query ent for the related records
    // Render only the fragment template (not the full layout)
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    component.Render(r.Context(), w)
}
```
Fragment endpoints always render directly without the `renderPage` wrapper (which adds layout on non-htmx requests). Fragment endpoints are always called by htmx, so they never need the full page layout.

### Pattern 5: Template Data Types in templates Package
**What:** Define data structs in the templates package (like `SearchGroup`/`SearchResult` in `searchtypes.go`) to avoid circular imports.
**When to use:** All detail page data passed from handler to template.
**Example:**
```go
// In templates/detailtypes.go
type NetworkDetail struct {
    ASN         int
    Name        string
    NameLong    string
    OrgName     string
    OrgID       int
    // ... other fields
    IXCount     int  // summary stat
    FacCount    int  // summary stat
    PocCount    int  // summary stat
}
```

### Anti-Patterns to Avoid
- **Eager-loading all related records on detail page load:** This defeats the lazy-loading requirement. Only load the entity itself + count stats on initial load. Related records are loaded via htmx fragment requests when sections are expanded.
- **Using ent WithXxx() eager loading for related records in detail handlers:** The ent schema has `entrest.WithEagerLoad(true)` annotations for the REST API, but detail page handlers should NOT use `WithNetworkIxLans()` etc. on the initial query. Only use `WithOrganization()` for the parent org link.
- **Creating separate HTTP routes outside the dispatch function:** The Phase 13 decision uses a single wildcard `GET /ui/{rest...}` pattern. Adding new top-level routes like `GET /ui/asn/{asn}` would conflict with Go 1.22+ route matching. All routing must go through `dispatch()`.
- **Returning full-page layout from fragment endpoints:** Fragment endpoints are only ever called by htmx. They must return bare HTML fragments, not wrapped in Layout().

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Collapsible sections | Custom JS accordion | HTML `<details>`/`<summary>` | Native browser support, accessible, keyboard-navigable, no JS needed |
| Lazy loading trigger | Custom JS intersection observer | htmx `hx-trigger="toggle once"` | htmx already loaded, handles request lifecycle |
| Entity queries | Raw SQL | ent client predicates | Type-safe, consistent with rest of codebase |
| URL parameter parsing | Regex router | `strings.HasPrefix`/`strings.TrimPrefix` + `strconv.Atoi` | Consistent with existing dispatch pattern |

## Common Pitfalls

### Pitfall 1: IXP Participant Query Path
**What goes wrong:** Querying `InternetExchange.QueryNetworkIxLans()` -- this edge does not exist directly. Network participants connect through IxLan.
**Why it happens:** The ent schema has IX -> ix_lans -> network_ix_lans, not IX -> network_ix_lans directly.
**How to avoid:** Query NetworkIxLan where ixlan_id is in the set of IxLan IDs belonging to the IX. Or traverse: `client.IxLan.Query().Where(ixlan.IxID(ixID)).QueryNetworkIxLans().All(ctx)`.
**Warning signs:** Compilation errors about missing QueryNetworkIxLans on InternetExchange.

### Pitfall 2: IXP Prefix Query Path
**What goes wrong:** Similar to Pitfall 1 -- IxPrefix connects through IxLan, not directly to InternetExchange.
**Why it happens:** Schema: InternetExchange -> ix_lans -> ix_prefixes.
**How to avoid:** `client.IxLan.Query().Where(ixlan.IxID(ixID)).QueryIxPrefixes().All(ctx)`.
**Warning signs:** Same as above.

### Pitfall 3: Network Lookup by ASN Returns Multiple Results
**What goes wrong:** Using `Query().Where(network.Asn(asn)).Only(ctx)` could return a "not singular" error if data integrity is violated.
**Why it happens:** Although ASN is marked Unique() in the schema, edge cases during sync could temporarily have duplicates.
**How to avoid:** Use `Only()` which returns `ent.NotSingularError` -- handle both NotFound and NotSingular. Or use `First()` and accept the first match.
**Warning signs:** 500 errors on valid ASN lookups.

### Pitfall 4: Fragment Endpoint Must Not Use renderPage
**What goes wrong:** Fragment endpoints wrapped in `renderPage()` return full HTML layout when the request is not an htmx request (e.g., direct browser navigation to fragment URL).
**Why it happens:** `renderPage` checks `HX-Request` header and wraps in Layout() if absent.
**How to avoid:** Fragment endpoints write directly to the response writer, bypassing `renderPage`. If someone navigates directly to a fragment URL, return just the fragment (it will look unstyled, which is acceptable -- these are internal htmx endpoints).
**Warning signs:** Double-nested layout HTML in htmx responses.

### Pitfall 5: Tailwind CSS Dynamic Class Generation
**What goes wrong:** Tailwind (via the browser CDN build) cannot detect dynamically generated class names at runtime.
**Why it happens:** The project uses `@tailwindcss/browser@4` which scans the DOM for class names. Dynamically constructed strings like `"text-" + color + "-400"` work in browser Tailwind mode but are fragile.
**How to avoid:** The existing codebase already uses switch statements for color classes (see `groupBadgeClasses`). Continue this pattern for detail page type badges.
**Warning signs:** Missing colors, unstyled elements.

### Pitfall 6: Nullable Foreign Keys in Related Records
**What goes wrong:** Many FK fields (org_id, net_id, fac_id, etc.) are Optional+Nillable. Querying with predicates like `networkixlan.NetID(id)` expects an int, but the field could be nil.
**Why it happens:** PeeringDB data has nullable FKs.
**How to avoid:** Use the ent-generated predicate functions which handle nil correctly. When displaying, check for nil/zero values before rendering links.
**Warning signs:** Nil pointer dereferences when accessing edge data.

### Pitfall 7: Carrier Type Missing from DETL-01 Description
**What goes wrong:** DETL-01 says "Network, IXP, Facility, Organization, or Campus" but CONTEXT.md lists 6 types including Carrier.
**Why it happens:** Requirements text omits Carrier, but CONTEXT.md includes it with URL `/ui/carrier/{id}`.
**How to avoid:** Implement all 6 types per CONTEXT.md. Carrier is the 6th type and has its own detail page with carrier_facilities.
**Warning signs:** Missing carrier detail page.

## Code Examples

### Entity Detail Handler Pattern
```go
// Source: existing handler.go pattern extended for detail pages
func (h *Handler) handleIXDetail(w http.ResponseWriter, r *http.Request, idStr string) {
    id, err := strconv.Atoi(idStr)
    if err != nil {
        h.handleNotFound(w, r)
        return
    }
    ix, err := h.client.InternetExchange.Query().
        Where(internetexchange.ID(id)).
        WithOrganization().
        Only(r.Context())
    if err != nil {
        if ent.IsNotFound(err) {
            h.handleNotFound(w, r)
            return
        }
        http.Error(w, "internal server error", http.StatusInternalServerError)
        return
    }
    // Count stats for header
    netCount := ix.NetCount // pre-computed field from sync
    facCount := ix.FacCount // pre-computed field from sync
    // ... build template data, render
}
```

### Lazy-Loaded Section templ Pattern
```
// Source: templ component pattern for collapsible lazy-load section
templ CollapsibleSection(title string, count int, loadURL string) {
    <details class="border border-neutral-700 rounded-lg overflow-hidden">
        <summary class="px-4 py-3 cursor-pointer flex items-center justify-between
            bg-neutral-800/50 hover:bg-neutral-800 transition-colors select-none">
            <span class="font-medium text-neutral-100">{ title }</span>
            <span class="text-neutral-500 text-sm font-mono">{ fmt.Sprintf("(%d)", count) }</span>
        </summary>
        <div
            hx-get={ loadURL }
            hx-trigger="toggle once from:closest details"
            hx-swap="innerHTML"
        >
            <div class="px-4 py-6 text-center text-neutral-500">
                <span class="animate-pulse">Loading...</span>
            </div>
        </div>
    </details>
}
```

### Related Record Row templ Pattern
```
// Source: pattern for clickable related record rows with cross-links
templ NetworkIXLanRow(name string, speed int, ipv4 string, ipv6 string, isRSPeer bool, ixID int) {
    <a href={ templ.SafeURL(fmt.Sprintf("/ui/ix/%d", ixID)) }
        class="flex items-center justify-between px-4 py-3 border-t border-neutral-700/50
            hover:bg-neutral-800/50 transition-colors">
        <div class="flex flex-col min-w-0">
            <span class="text-neutral-100 font-medium truncate">{ name }</span>
            <div class="flex gap-4 text-sm text-neutral-400 font-mono">
                if speed > 0 {
                    <span>{ formatSpeed(speed) }</span>
                }
                if ipv4 != "" {
                    <span>{ ipv4 }</span>
                }
                if ipv6 != "" {
                    <span>{ ipv6 }</span>
                }
            </div>
        </div>
        if isRSPeer {
            <span class="text-xs text-emerald-400/70 font-mono">RS</span>
        }
    </a>
}
```

### Entity Relationship Map (Complete)

This table maps every detail page type to its related sections with the ent query path:

| Detail Type | Section Name | Ent Query Path | Link Target |
|-------------|-------------|----------------|-------------|
| **Network** (`/ui/asn/{asn}`) | Organization | `net.Edges.Organization` (eager) | `/ui/org/{id}` |
| | IX Presences | `NetworkIxLan.Query().Where(NetID(net.ID))` | `/ui/ix/{ix_id}` |
| | Facility Presences | `NetworkFacility.Query().Where(NetID(net.ID))` | `/ui/fac/{fac_id}` |
| | Contacts | `Poc.Query().Where(NetID(net.ID))` | (no detail page for poc) |
| **IXP** (`/ui/ix/{id}`) | Organization | `ix.Edges.Organization` (eager) | `/ui/org/{id}` |
| | Participants | `IxLan.Query().Where(IxID(id)).QueryNetworkIxLans()` | `/ui/asn/{asn}` |
| | Facilities | `IxFacility.Query().Where(IxID(id))` | `/ui/fac/{fac_id}` |
| | Prefixes | `IxLan.Query().Where(IxID(id)).QueryIxPrefixes()` | (no detail page for prefix) |
| **Facility** (`/ui/fac/{id}`) | Organization | `fac.Edges.Organization` (eager) | `/ui/org/{id}` |
| | Campus | `fac.Edges.Campus` (eager) | `/ui/campus/{id}` |
| | Networks | `NetworkFacility.Query().Where(FacID(id))` | `/ui/asn/{local_asn}` |
| | IXPs | `IxFacility.Query().Where(FacID(id))` | `/ui/ix/{ix_id}` |
| | Carriers | `CarrierFacility.Query().Where(FacID(id))` | `/ui/carrier/{carrier_id}` |
| **Organization** (`/ui/org/{id}`) | Networks | `Network.Query().Where(OrgID(id))` | `/ui/asn/{asn}` |
| | IXPs | `InternetExchange.Query().Where(OrgID(id))` | `/ui/ix/{id}` |
| | Facilities | `Facility.Query().Where(OrgID(id))` | `/ui/fac/{id}` |
| | Campuses | `Campus.Query().Where(OrgID(id))` | `/ui/campus/{id}` |
| | Carriers | `Carrier.Query().Where(OrgID(id))` | `/ui/carrier/{id}` |
| **Campus** (`/ui/campus/{id}`) | Organization | `campus.Edges.Organization` (eager) | `/ui/org/{id}` |
| | Facilities | `Facility.Query().Where(CampusID(id))` | `/ui/fac/{id}` |
| **Carrier** (`/ui/carrier/{id}`) | Organization | `carrier.Edges.Organization` (eager) | `/ui/org/{id}` |
| | Facilities | `CarrierFacility.Query().Where(CarrierID(id))` | `/ui/fac/{fac_id}` |

### Pre-Computed Count Fields Available

Several entities have pre-computed count fields synced from PeeringDB (stored per D-40):

| Entity | Field | Description |
|--------|-------|-------------|
| Network | `ix_count` | Number of IX presences |
| Network | `fac_count` | Number of facility presences |
| InternetExchange | `net_count` | Number of network participants |
| InternetExchange | `fac_count` | Number of facilities |
| Facility | `net_count` | Number of networks present |
| Facility | `ix_count` | Number of IXPs present |
| Facility | `carrier_count` | Number of carriers |
| Organization | `net_count` | Number of networks |
| Organization | `fac_count` | Number of facilities |

Use these pre-computed fields for summary stats in the header (DETL-04). They are stored on the entity itself and require no additional count queries.

### Fragment Endpoint URL Pattern

For consistency and parsability, fragment URLs follow:
```
/ui/fragment/{parent_type}/{parent_id}/{relation}
```

Examples:
- `/ui/fragment/net/10/ixlans` -- Network's IX presences
- `/ui/fragment/net/10/facilities` -- Network's facility presences
- `/ui/fragment/net/10/contacts` -- Network's contacts
- `/ui/fragment/ix/20/participants` -- IXP's network participants
- `/ui/fragment/ix/20/facilities` -- IXP's facilities
- `/ui/fragment/ix/20/prefixes` -- IXP's peering LAN prefixes
- `/ui/fragment/fac/30/networks` -- Facility's networks
- `/ui/fragment/fac/30/ixps` -- Facility's IXPs
- `/ui/fragment/fac/30/carriers` -- Facility's carriers
- `/ui/fragment/org/1/networks` -- Org's networks
- `/ui/fragment/org/1/ixps` -- Org's IXPs
- `/ui/fragment/org/1/facilities` -- Org's facilities
- `/ui/fragment/org/1/campuses` -- Org's campuses
- `/ui/fragment/org/1/carriers` -- Org's carriers
- `/ui/fragment/campus/40/facilities` -- Campus's facilities
- `/ui/fragment/carrier/50/facilities` -- Carrier's facilities

### Speed Formatting Helper

NetworkIxLan speeds are stored in Mbps. Display should be human-readable:
```go
func formatSpeed(mbps int) string {
    switch {
    case mbps >= 1_000_000:
        return fmt.Sprintf("%dT", mbps/1_000_000)
    case mbps >= 1000:
        return fmt.Sprintf("%dG", mbps/1000)
    default:
        return fmt.Sprintf("%dM", mbps)
    }
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Custom JS accordion | HTML `<details>/<summary>` + htmx | Native HTML support widespread since 2020+ | No JS needed for collapsible sections |
| SPA client routing | Server-rendered pages with htmx fragments | Project decision (Phase 13) | Each detail page has a clean shareable URL |
| Full page reload for related data | htmx partial swap on `<details>` toggle | htmx 1.x+ | Related sections lazy-load without full page navigation |

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing stdlib + enttest |
| Config file | None needed -- standard `go test` |
| Quick run command | `go test -race ./internal/web/...` |
| Full suite command | `go test -race ./...` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| DETL-01 | Detail page renders for each entity type | integration | `go test -race -run TestDetailHandler ./internal/web/ -x` | Wave 0 |
| DETL-01 | 404 for non-existent entity | integration | `go test -race -run TestDetailNotFound ./internal/web/ -x` | Wave 0 |
| DETL-01 | Network lookup by ASN (not ID) | integration | `go test -race -run TestNetworkDetailByASN ./internal/web/ -x` | Wave 0 |
| DETL-02 | Detail page contains `<details>` sections | unit | `go test -race -run TestDetailSections ./internal/web/ -x` | Wave 0 |
| DETL-03 | Fragment endpoint returns HTML fragment | integration | `go test -race -run TestFragment ./internal/web/ -x` | Wave 0 |
| DETL-03 | Fragment endpoint does not include layout | unit | `go test -race -run TestFragmentNoLayout ./internal/web/ -x` | Wave 0 |
| DETL-04 | Detail header contains summary counts | unit | `go test -race -run TestDetailStats ./internal/web/ -x` | Wave 0 |
| DETL-05 | Related record rows contain cross-links | unit | `go test -race -run TestDetailCrossLinks ./internal/web/ -x` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test -race ./internal/web/...`
- **Per wave merge:** `go test -race ./...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/web/detail_test.go` -- covers DETL-01 through DETL-05
- [ ] Test data seeding for related records (NetworkIxLan, NetworkFacility, IxFacility, CarrierFacility, Poc, IxPrefix via IxLan)

## Open Questions

1. **Poc contacts -- what to display?**
   - What we know: Poc has name, email, phone, role, url, visible fields. The `visible` field indicates visibility level ("Public", "Private", "Users").
   - What's unclear: Should we filter by `visible="Public"` only, or show all since this is a read-only mirror that already has the data?
   - Recommendation: Show all contacts that are in the database (PeeringDB already controls what data is synced based on API key permissions). Group by role.

2. **Empty related sections -- show or hide?**
   - What we know: Some entities have zero related records for certain types (e.g., a campus with no facilities yet).
   - What's unclear: Should we show an empty `<details>` section with count "(0)" or hide it entirely?
   - Recommendation: Show sections with count "(0)" but don't make them expandable (no htmx trigger). The count badge communicates the emptiness. Use a `disabled` or non-clickable summary style.

## Project Constraints (from CLAUDE.md)

Directives that apply to this phase:

- **CS-5 (MUST):** Use input structs for functions receiving more than 2 arguments. Detail handler methods pass entity data through template data structs.
- **CS-6 (SHOULD):** Declare function input structs before the function consuming them. `detailtypes.go` defines types before the templ files that use them.
- **ERR-1 (MUST):** Wrap errors with `%w` and context. All ent query errors must be wrapped.
- **T-1 (MUST):** Table-driven tests. Test detail handlers with table-driven test cases covering all 6 entity types.
- **T-2 (MUST):** Run `-race` in CI; add `t.Cleanup` for teardown. Use `testutil.SetupClient(t)` which handles cleanup.
- **T-3 (SHOULD):** Mark safe tests with `t.Parallel()`.
- **API-1 (MUST):** Document exported items. All exported template types need doc comments.
- **OBS-1 (MUST):** Structured logging with slog. Log entity lookups and errors.

## Sources

### Primary (HIGH confidence)
- Ent schema files: `ent/schema/*.go` -- entity fields, edges, indexes
- Existing web handler: `internal/web/handler.go` -- dispatch pattern, renderPage
- Existing templates: `internal/web/templates/*.templ` -- templ component patterns
- Existing tests: `internal/web/*_test.go` -- test patterns, testutil usage

### Secondary (MEDIUM confidence)
- [htmx hx-trigger docs](https://htmx.org/attributes/hx-trigger/) -- trigger events including `toggle`, `once` modifier
- [htmx lazy load example](https://htmx.org/examples/lazy-load/) -- lazy loading patterns
- [MDN details/toggle event](https://developer.mozilla.org/en-US/docs/Web/API/HTMLElement/toggle_event) -- details toggle event fires when opened/closed

### Tertiary (LOW confidence)
- None -- all critical patterns are verified from codebase + official docs.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- no new dependencies, all tools already in project
- Architecture: HIGH -- extends existing dispatch pattern, follows established templ/htmx conventions
- Pitfalls: HIGH -- identified from direct codebase analysis (ent schema edges, nullable FKs)
- Query paths: HIGH -- verified edge definitions in ent schema files

**Research date:** 2026-03-24
**Valid until:** 2026-04-24 (stable -- no external dependency changes expected)
