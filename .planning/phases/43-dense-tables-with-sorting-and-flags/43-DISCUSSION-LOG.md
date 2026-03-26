# Phase 43: Dense Tables with Sorting and Flags - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-26
**Phase:** 43-dense-tables-with-sorting-and-flags
**Areas discussed:** Table columns, Sorting mechanism, Country flag approach, Responsive column hiding
**Mode:** --auto (all decisions auto-selected)

---

## Table Columns

| Option | Description | Selected |
|--------|-------------|----------|
| All available data in columns | Show every field from row structs as a table column | ✓ |
| Curated subset | Only show 2-3 most important fields per table | |

**User's choice:** [auto] All available data in columns (recommended default)
**Notes:** Data-rich tables (IX participants, contacts) get full column sets; simple name-only lists get single-column tables for consistency.

| Option | Description | Selected |
|--------|-------------|----------|
| Tables for all lists | Even single-field lists become tables | ✓ |
| Tables only for multi-field lists | Simple name lists stay as div-based rows | |

**User's choice:** [auto] Tables for consistency (recommended default)

---

## Sorting Mechanism

| Option | Description | Selected |
|--------|-------------|----------|
| Vanilla JS click handler | Custom sort on th click, no library | ✓ |
| External library (tablesort, list.js) | Third-party sorting library | |

**User's choice:** [auto] Vanilla JS click handler (recommended default)

| Option | Description | Selected |
|--------|-------------|----------|
| No URL persistence | Sort is ephemeral client-side state | ✓ |
| URL query param persistence | Sort column/direction in URL | |

**User's choice:** [auto] No URL persistence (recommended default)

---

## Country Flag Approach

| Option | Description | Selected |
|--------|-------------|----------|
| flag-icons CSS (CDN) | SVG flag sprites via CSS classes | ✓ |
| Inline SVG per country | Bundle individual SVG files | |
| Emoji flags | Unicode flag emoji (inconsistent rendering) | |

**User's choice:** [auto] flag-icons CSS via CDN (recommended default, matches REQUIREMENTS.md FLAG-01 spec)

---

## Responsive Column Hiding

| Option | Description | Selected |
|--------|-------------|----------|
| Hide city/speed/IP below md | Keep name, country+flag, ASN visible | ✓ |
| Hide only IP addresses | Keep all other columns | |
| Horizontal scroll | No column hiding, scroll on narrow | |

**User's choice:** [auto] Hide city, speed, IP addresses below md (recommended default)

---

## Claude's Discretion

- Sort indicator styling (CSS arrow vs inline SVG)
- Table cell padding/spacing for visual density
- Speed color tier and RS badge adaptation for table context
- Table header styling approach

## Deferred Ideas

None
