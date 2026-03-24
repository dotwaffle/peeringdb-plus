# Phase 15: Record Detail Pages — Discussion Context

**Gathered:** 2026-03-24

## Decisions

### URL Structure
- **Networks by ASN**: `/ui/asn/13335` — users think in ASNs, not internal IDs. Dedicated `/ui/asn/` path segment makes it unambiguous.
- **Other types by PeeringDB ID**: `/ui/ix/456`, `/ui/fac/789`, `/ui/org/123`, `/ui/campus/1`, `/ui/carrier/1`
- 6 detail page types total (matching the 6 searchable types).

### Related Records — All Edges Shown
Every type shows all its relationships in collapsible sections:

**Network (`/ui/asn/{asn}`):**
- Parent organization (link)
- IX presences (netixlan) — with speeds, IPs, VLAN, RS peer status
- Facility presences (netfac)
- Contacts (poc)

**IXP (`/ui/ix/{id}`):**
- Participants (netixlan) — networks present at this exchange
- Facilities (ixfac) — physical locations of the exchange
- Prefixes (ixpfx) — peering LAN prefixes

**Facility (`/ui/fac/{id}`):**
- Networks (netfac) — networks present in this facility
- IXPs (ixfac) — exchanges present in this facility
- Carriers (carrierfac) — carriers serving this facility

**Organization (`/ui/org/{id}`):**
- Networks — networks owned by this org
- IXPs — exchanges operated by this org
- Facilities — facilities operated by this org
- Campuses — campuses owned by this org
- Carriers — carriers operated by this org

**Campus (`/ui/campus/{id}`):**
- Facilities — facilities in this campus

**Carrier (`/ui/carrier/{id}`):**
- Facilities (carrierfac) — facilities served by this carrier

### Lazy Loading
- Related record sections load on first expand via `hx-trigger="toggle"` or `hx-trigger="revealed"` on the `<details>` element.
- Prevents loading 50+ IX presences on initial page load for large networks.

### Summary Stats
- Computed stats in header area, e.g.: "Present at 47 IXPs, 23 Facilities" for a network.
- Stats are cheap count queries, loaded with the main page (not lazy-loaded).

### Cross-Linking
- Every related record links to its own detail page (e.g., clicking an IXP name in a network's IX presences → `/ui/ix/{id}`).
- Parent org always linked from network/IXP/facility detail pages.
