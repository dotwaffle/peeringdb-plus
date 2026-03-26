# Phase 43: Dense Tables with Sorting and Flags - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-26
**Phase:** 43-dense-tables-with-sorting-and-flags
**Areas discussed:** Table columns, Sorting mechanism, Country flags, Responsive hiding, Table visual style, Empty states, Row density, Sort data types
**Mode:** Interactive

---

## Table Columns

| Option | Description | Selected |
|--------|-------------|----------|
| All available fields | Every field from row struct becomes a column — maximum density | ✓ |
| Curated key fields | Only 3-4 most important fields per table | |
| Progressive disclosure | Key fields default with expand row for the rest | |

**User's choice:** All available fields
**Notes:** Maximum density approach.

| Option | Description | Selected |
|--------|-------------|----------|
| Tables for all | Even single-field lists become tables | ✓ |
| Tables only for multi-field | Simple name lists stay as div-based rows | |
| Compact inline list | Name-only lists become comma-separated or pills | |

**User's choice:** Tables for all
**Notes:** Uniform treatment for consistency.

| Option | Description | Selected |
|--------|-------------|----------|
| Dedicated flag + country column | Flag icon and country code in own column | ✓ |
| Flag inline with name | Small flag next to entity name | |
| Flag replacing country text | Flag icon only, tooltip for country name | |

**User's choice:** Dedicated flag + country column

| Option | Description | Selected |
|--------|-------------|----------|
| Enrich with country | Add City/Country to row structs via joins | ✓ |
| Keep existing fields only | Only show flags where Country already exists | |

**User's choice:** Enrich with country
**Notes:** Requires query changes for FacNetworkRow, OrgNetworkRow.

| Option | Description | Selected |
|--------|-------------|----------|
| Keep CopyableIP | Reuse component in table cells | ✓ |
| Plain text monospace | Display IP as text, manual select+copy | |
| Copyable on hover only | Plain text with copy button on hover | |

**User's choice:** Keep CopyableIP

| Option | Description | Selected |
|--------|-------------|----------|
| Visible headers | Standard thead with column names | ✓ |
| Hidden headers (sr-only) | Screen-reader only | |

**User's choice:** Visible headers

---

## Sorting Mechanism

| Option | Description | Selected |
|--------|-------------|----------|
| Vanilla JS click handler | Custom sort on th click, no library | ✓ |
| Lightweight library | tablesort.js or similar | |
| htmx server-side | Re-fetch sorted data from server | |

**User's choice:** Vanilla JS click handler

| Option | Description | Selected |
|--------|-------------|----------|
| Contextual defaults | ASN for IX participants, country for facilities, name for networks | ✓ |
| All by name ascending | Uniform alphabetical default | |
| Unsorted (server order) | Database order, no client-side default | |

**User's choice:** Contextual defaults

| Option | Description | Selected |
|--------|-------------|----------|
| No — ephemeral | Client-side only, resets on reload | ✓ |
| Yes — URL params | Sort in URL query params | |

**User's choice:** No — ephemeral

| Option | Description | Selected |
|--------|-------------|----------|
| CSS triangle | Border trick or ::after pseudo-element | ✓ |
| Inline SVG arrow | Chevron SVG icon | |
| Text characters | Unicode ▲/▼ | |

**User's choice:** CSS triangle

| Option | Description | Selected |
|--------|-------------|----------|
| Multi-column only | Sort UI only on tables with 2+ columns | ✓ |
| All tables | Even single-column tables get sort | |

**User's choice:** Multi-column only

---

## Country Flags

| Option | Description | Selected |
|--------|-------------|----------|
| CDN link | flag-icons CSS from CDN in layout.templ head | ✓ |
| Self-hosted static | Download into /static/ | |
| NPM + bundled | Install via npm | |

**User's choice:** CDN link

| Option | Description | Selected |
|--------|-------------|----------|
| 4x3 rectangle | Standard flag proportions | ✓ |
| 1x1 square | Square crop, compact | |

**User's choice:** 4x3 rectangle

| Option | Description | Selected |
|--------|-------------|----------|
| Empty cell | No flag, no placeholder | ✓ |
| Globe icon placeholder | Generic globe when unknown | |
| Question mark placeholder | ? or unknown-flag icon | |

**User's choice:** Empty cell

---

## Responsive Hiding

| Option | Description | Selected |
|--------|-------------|----------|
| City, Speed, IPs, RS | Aggressive — mobile shows name, ASN, country+flag only | ✓ |
| City, Speed, IPs | Keep RS badge visible on mobile | |
| Only IPs | Hide only IPv4/IPv6, keep speed and city | |

**User's choice:** City, Speed, IPs, RS (aggressive hiding)
**Notes:** Mobile prioritizes identity (name, ASN) and geography (country+flag).

| Option | Description | Selected |
|--------|-------------|----------|
| Tailwind classes | hidden md:table-cell | ✓ |
| CSS media queries | Custom @media rules | |

**User's choice:** Tailwind classes

| Option | Description | Selected |
|--------|-------------|----------|
| Yes — overflow-x-auto wrapper | Safety net for edge cases | ✓ |
| No — trust responsive hiding | No scroll wrapper | |

**User's choice:** Yes — overflow-x-auto wrapper

---

## Table Visual Style

| Option | Description | Selected |
|--------|-------------|----------|
| Subtle alternating | Zebra striping with faint even/odd background | ✓ |
| Dividers only | Horizontal lines between rows | |
| No decoration | Plain rows with hover only | |

**User's choice:** Subtle alternating

| Option | Description | Selected |
|--------|-------------|----------|
| Subtle sticky header | Faint background, smaller text, bottom border | ✓ |
| Transparent header | No background, just bold text | |
| Strong contrast header | Solid neutral-700 background | |

**User's choice:** Subtle sticky header

| Option | Description | Selected |
|--------|-------------|----------|
| Yes — highlight row | hover:bg-neutral-800/50 on tr | ✓ |
| No hover | Static rows | |

**User's choice:** Yes — highlight row

---

## Empty State Handling

| Option | Description | Selected |
|--------|-------------|----------|
| Keep current pattern | 'No X found.' in colspan cell | ✓ |
| No table at all | Just text message when count is 0 | |
| Empty table with headers | Show headers with message row | |

**User's choice:** Keep current pattern

---

## Row Density & Spacing

| Option | Description | Selected |
|--------|-------------|----------|
| Compact (px-3 py-1.5) | ~6px vertical padding | ✓ |
| Medium (px-3 py-2) | ~8px vertical padding | |
| Keep current (px-4 py-3) | Same as current lists | |

**User's choice:** Compact

| Option | Description | Selected |
|--------|-------------|----------|
| text-sm throughout | 14px for all cells | ✓ |
| Mixed sizes | 16px name, 14px others | |
| text-xs for data columns | 12px for non-name columns | |

**User's choice:** text-sm throughout

---

## Sort Data Types

| Option | Description | Selected |
|--------|-------------|----------|
| Data attributes | data-sort-value on td for raw sortable values | ✓ |
| Parse displayed text | JS determines type from cell text | |
| Column type hints | data-sort-type on th | |

**User's choice:** Data attributes

| Option | Description | Selected |
|--------|-------------|----------|
| Empty values last | Missing data always at bottom | ✓ |
| Empty values first | Missing data at top | |
| Treat as zero/empty string | Normal sort order | |

**User's choice:** Empty values last

---

## Claude's Discretion

- Speed color tier and RS badge adaptation for table cell context
- Table border treatment (borderless vs subtle grid lines)
- Sort JS placement (layout.templ vs shared table component)
- Whether table headers should be position: sticky

## Deferred Ideas

None
