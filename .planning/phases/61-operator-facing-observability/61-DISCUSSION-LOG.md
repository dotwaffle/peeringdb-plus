# Phase 61: Operator-facing observability - Discussion Log

> **Audit trail only.** Decisions are captured in CONTEXT.md.

**Date:** 2026-04-16
**Phase:** 61-operator-facing-observability
**Areas discussed:** Startup log shape, /about location, OTel attribute name + placement

---

## Startup classification log shape

| Option | Description | Selected |
|--------|-------------|----------|
| One slog.Info with attributes (Recommended) | Single line, machine-parseable; plus separate WARN. | ✓ |
| Two separate Info lines + conditional Warn | More granular grep targets. | |
| Single multiline INFO with rendered table | Pretty for humans, ugly for aggregators. | |

**User's choice:** One slog.Info with attributes
**Notes:** —

---

## /about rendering location

| Option | Description | Selected |
|--------|-------------|----------|
| New 'Privacy & Sync' section after Sync Status (Recommended) | Discoverable; doesn't muddle existing layout. | ✓ |
| Inline addition to existing Sync Status block | Tighter; mixes purposes. | |
| New section + render only when override is in effect | Less noise; weakens the message for default deployments. | |

**User's choice:** New 'Privacy & Sync' section after Sync Status
**Notes:** Renders for all deployments (including default), with a visual flag when the override is active.

---

## OTel attribute name + placement

| Option | Description | Selected |
|--------|-------------|----------|
| pdbplus.privacy.tier on inbound HTTP server span (Recommended) | Single set; downstream spans inherit via context. | ✓ |
| peeringdb.tier on every ent query span | Redundant. | |
| Both HTTP and ent spans | Belt and braces; doubles writes. | |

**User's choice:** pdbplus.privacy.tier on inbound HTTP server span
**Notes:** —

---

## Claude's Discretion

- One-line plain-language explanation wording
- HTML override-flag visual treatment
- Terminal override indicator character

## Deferred Ideas

- Per-request log line of resolved tier
- Audit log of override changes
- `/about` per-attribute visibility statistics
