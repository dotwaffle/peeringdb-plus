# Phase 30: Entity Types, Search & Formats - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-25
**Phase:** 30-entity-types-search-formats
**Areas discussed:** Layout consistency, Search results, JSON shape, WHOIS format, Layout categories, RPSL scope

---

## Layout Consistency

| Option | Description | Selected |
|--------|-------------|----------|
| Same template for all (Recommended) | Every type uses same structure. Consistent. | |
| Unique per type | Each type gets tailored layout. More work. | |
| Two categories | Complex types rich, simple types minimal. Balance. | ✓ |

**User's choice:** Two categories
**Notes:** None

---

## Rich Type Selection

| Option | Description | Selected |
|--------|-------------|----------|
| Network + IX rich, rest minimal (Recommended) | Two rich, four minimal. | |
| Network + IX + Facility rich | Three rich (Facility has address, net counts, IX presence). | ✓ |
| Only Network rich | Simplest — one rich type. | |

**User's choice:** Network + IX + Facility rich
**Notes:** Facility's address data and network/IX presence warrants the richer layout.

---

## Search Results

| Option | Description | Selected |
|--------|-------------|----------|
| Grouped by type with paths (Recommended) | Results under type headers, one line per result with curl path. | ✓ |
| Flat list with type prefix | All results in one list with [NET]/[IX] prefix. | |
| Grouped, higher result count | Same as grouped but 25 per type instead of 10. | |

**User's choice:** Grouped by type with paths
**Notes:** None

---

## JSON Output Shape

| Option | Description | Selected |
|--------|-------------|----------|
| Same as REST API (Recommended) | Identical to /rest/v1/{type}/{id}. No new schema. | ✓ |
| Display-oriented JSON | Includes computed fields like aggregate bandwidth. | |
| Minimal scripting JSON | Stripped to key fields only. | |

**User's choice:** Same as REST API
**Notes:** None

---

## WHOIS Format Strictness

| Option | Description | Selected |
|--------|-------------|----------|
| Loose key-value with source header (Recommended) | RPSL-inspired but not strict. | |
| Strict RPSL | Follow RPSL spec closely. Machine-parseable. | ✓ |
| INI-style sections | [headers] with key=value. Not RPSL-like. | |

**User's choice:** Strict RPSL
**Notes:** None

---

## RPSL Scope

| Option | Description | Selected |
|--------|-------------|----------|
| aut-num only, rest loose (Recommended) | Networks strict, others loose. | |
| All types strict where possible | Networks=aut-num, IXes=ix:, Facilities=site:. Fill what we can. | ✓ |
| All types use aut-num-style format | aut-num conventions for all types. | |

**User's choice:** All types strict where possible
**Notes:** Map each type to the closest RPSL class, fill available fields, leave unavailable ones empty.

---

## Claude's Discretion

- IX/Facility detail: which fields in header
- Org/Campus/Carrier: minimal header fields
- ASN comparison terminal layout specifics
- RPSL field mapping details per type

## Deferred Ideas

None
