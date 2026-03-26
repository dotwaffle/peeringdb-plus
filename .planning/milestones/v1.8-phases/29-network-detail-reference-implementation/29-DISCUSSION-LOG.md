# Phase 29: Network Detail (Reference Implementation) - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-25
**Phase:** 29-network-detail-reference-implementation
**Areas discussed:** Header layout, Table design, Cross-references

---

## Header Layout

| Option | Description | Selected |
|--------|-------------|----------|
| Compact key-value, no border (Recommended) | 8-10 key fields as aligned pairs. Human labels. Like whois output. | ✓ |
| Comprehensive with border | All fields inside Unicode box border. More complete but noisier. | |
| Two-column compact | Key fields split into two columns. Denser. | |

**User's choice:** Compact key-value, no border
**Notes:** None

---

## Table Design (IX Presences / Facilities)

| Option | Description | Selected |
|--------|-------------|----------|
| Unicode box tables (Recommended) | Full Unicode box drawing with column headers. | |
| Aligned columns, no borders | Space-padded columns with header underline. Lighter. | |
| Compact one-line per entry | No table structure — each entry on one line. Most compact, pipeable. | ✓ |

**User's choice:** Compact one-line per entry
**Notes:** Preferred for compactness and ability to pipe to grep/awk.

---

## Cross-Reference Format

| Option | Description | Selected |
|--------|-------------|----------|
| Inline path after name (Recommended) | 'DE-CIX Frankfurt [/ui/ix/31]' — easy to copy-paste. | ✓ |
| Separate Path column | Dedicated column in tables for follow-up path. | |
| Numbered footnotes | Numbers in text, paths at bottom. | |

**User's choice:** Inline path after name
**Notes:** None

---

## Claude's Discretion

- Exact field ordering in header
- Spacing and alignment details
- Missing/empty field handling
- Section headers between IX presences and facilities

## Deferred Ideas

None
