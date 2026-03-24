# Phase 16: ASN Comparison — Discussion Context

**Gathered:** 2026-03-24

## Decisions

### URL Structure
- **Path-based**: `/ui/compare/13335/15169` — clean, shareable, fixed order (first ASN / second ASN).
- Empty state: `/ui/compare` shows two ASN input fields.

### Entry Points
1. **Dedicated /compare page**: `/ui/compare` with two ASN input fields. User enters both ASNs and submits.
2. **"Compare with..." button**: On every network detail page (`/ui/asn/{asn}`), a button that pre-fills the first ASN and prompts for the second.

### Shared IXP Results — Full Peering Info
For each shared IXP, show:
- Exchange name (linked to `/ui/ix/{id}`)
- Both networks' port speeds
- Both networks' IPv4 and IPv6 addresses
- VLAN information
- Route server peer status
- Operational status

### Shared Facility Results
For each shared facility, show:
- Facility name (linked to `/ui/fac/{id}`)
- City and country
- Network-facility specific data for both networks (local ASN, circuit info if available)

### Shared Campus Results
For each shared campus, show:
- Campus name (linked to `/ui/campus/{id}`)
- Facilities within the campus where both networks are present

### View Modes
- **Default: Shared-only view** — only shows IXPs/facilities/campuses where BOTH networks are present. Answers "where can we peer?" directly.
- **Toggle: Full side-by-side** — shows ALL presences for both networks, with non-shared ones visually grayed out. Shared ones highlighted.
- Toggle is a button/switch on the page, state captured in URL (e.g., `/ui/compare/13335/15169?view=full`).

### Query Approach
- Load both networks' IX presences (netixlan), facility presences (netfac), and campus memberships.
- Compute intersection in Go (set intersection by IXP ID, facility ID, campus ID).
- For side-by-side: compute union with shared flag.
