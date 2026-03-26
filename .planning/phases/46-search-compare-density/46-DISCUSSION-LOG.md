# Phase 46: Search & Compare Density - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-26
**Phase:** 46-search-compare-density
**Areas discussed:** Search result layout, Compare section tables, Search data enrichment, Flag placement, Search responsive behavior, Keyboard navigation, Compare table columns, Subtitle handling
**Mode:** Interactive

---

## Search Result Layout

| Option | Description | Selected |
|--------|-------------|----------|
| Compact rows, drop card borders | Divider lines, tighter padding, metadata inline | ✓ |
| Table per type group | Each group becomes a full table | |
| Keep cards, add inline metadata | Existing cards + small badges | |

**User's choice:** Compact rows, drop card borders

| Option | Description | Selected |
|--------|-------------|----------|
| Right-aligned inline | Name left, metadata badges right, single line | ✓ |
| Below name as subtitle | Two-line per result | |
| Mixed: ASN next to name, flag far right | Split metadata placement | |

**User's choice:** Right-aligned inline

| Option | Description | Selected |
|--------|-------------|----------|
| Remove per-row badge | Group header shows type, per-row is redundant | ✓ |
| Keep per-row badge | Type context on every row | |

**User's choice:** Remove per-row badge

---

## Compare Section Tables

| Option | Description | Selected |
|--------|-------------|----------|
| Yes — full table conversion | All three sections become sortable tables | ✓ |
| Facilities only | Only convert facilities section | |
| Keep current, just add flags | Don't restructure, add flags to divs | |

**User's choice:** Full table conversion

| Option | Description | Selected |
|--------|-------------|----------|
| Flat columns | IX Name + separate columns per network field | ✓ |
| Grouped sub-columns | Column groups with network headers | |
| Keep 3-column with tables | Mini tables per network | |

**User's choice:** Flat columns

| Option | Description | Selected |
|--------|-------------|----------|
| Yes — sortable | Same vanilla JS from Phase 43 | ✓ |
| No sorting | Static tables | |

**User's choice:** Sortable

| Option | Description | Selected |
|--------|-------------|----------|
| Yes — keep dimming | Non-shared at opacity-40 | ✓ |
| No dimming | Badge/icon for distinction | |

**User's choice:** Keep dimming

---

## Search Data Enrichment

| Option | Description | Selected |
|--------|-------------|----------|
| Country + City + ASN | All three fields added | ✓ |
| Country + ASN only | Skip city | |
| Country only | Just for the flag | |

**User's choice:** Country + City + ASN

| Option | Description | Selected |
|--------|-------------|----------|
| Enrich existing query | Add selects to current queries | ✓ |
| Post-query lookup | Second pass for metadata | |
| Cache at sync time | Pre-computed search index | |

**User's choice:** Enrich existing query

---

## Flag Placement in Search

| Option | Description | Selected |
|--------|-------------|----------|
| Small flag + code in metadata area | Right-aligned with other metadata | ✓ |
| Flag before entity name | Left of name | |
| Flag only, no text code | Flag icon without country code | |

**User's choice:** Small flag + code in metadata area

| Option | Description | Selected |
|--------|-------------|----------|
| Empty — consistent with Phase 43 | No flag, empty space | ✓ |
| Show ASN instead | Fill space with ASN for networks | |

**User's choice:** Empty

---

## Search Responsive Behavior

| Option | Description | Selected |
|--------|-------------|----------|
| Hide city, keep flag+ASN | Mobile: name + flag + ASN | ✓ |
| Hide city and ASN, keep flag only | Mobile: name + flag | |
| Hide all metadata | Mobile: name only | |

**User's choice:** Hide city, keep flag+ASN

---

## Keyboard Navigation

| Option | Description | Selected |
|--------|-------------|----------|
| Preserve with updated styling | Same aria pattern, updated for compact rows | ✓ |
| Simplify to Tab-only | Remove custom arrow key handler | |

**User's choice:** Preserve with updated styling

---

## Compare Table Columns

### IXP Table
| Option | Description | Selected |
|--------|-------------|----------|
| IX Name + Speed A + Speed B + RS A + RS B | Core comparison | |
| IX Name + Speed A + Speed B + IPv4 A + IPv4 B | Include IPv4 | |
| IX Name + All A fields + All B fields | Full data both networks | ✓ |

**User's choice:** All fields for both networks

### Facility Table
| Option | Description | Selected |
|--------|-------------|----------|
| Name + Flag+Country + City + ASN A + ASN B | Full with flag | ✓ |
| Name + Flag+Country + ASN A + ASN B | Drop city | |
| Name + Flag+Country + City | Drop ASNs | |

**User's choice:** Name + Flag+Country + City + ASN A + ASN B

### Campus Table
| Option | Description | Selected |
|--------|-------------|----------|
| Simple table: Name + Shared Facilities count | Compact tabular | ✓ |
| Flat table per campus-facility pair | One row per facility | |
| Keep current nested layout | Already compact enough | |

**User's choice:** Simple table with count

---

## Subtitle Handling

| Option | Description | Selected |
|--------|-------------|----------|
| Remove Subtitle | Replaced by Country/City/ASN fields | ✓ |
| Keep as fallback | Belt and suspenders | |

**User's choice:** Remove Subtitle

---

## Claude's Discretion

- Search result divider styling
- Metadata badge spacing and ordering
- Search hover effect without card borders
- Campus count display style
- IXP table empty network data display

## Deferred Ideas

None
