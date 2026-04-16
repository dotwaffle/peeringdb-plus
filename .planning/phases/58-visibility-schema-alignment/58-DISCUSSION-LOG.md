# Phase 58: Visibility schema alignment - Discussion Log

> **Audit trail only.** Decisions are captured in CONTEXT.md.

**Date:** 2026-04-16
**Phase:** 58-visibility-schema-alignment
**Areas discussed:** Field shape, default-value strategy, migration approach

---

## Field shape for new visibility-bearing fields

| Option | Description | Selected |
|--------|-------------|----------|
| String field with `visible` companion (Recommended) | `field.String` plus sibling `<field>_visible` defaulting to "Public". Mirrors PeeringDB schema. | ✓ |
| Custom Visibility Go enum type | `field.Enum` typed Go. Stronger compile-time checks; regen + json marshalling churn. | |
| Bitfield on the row, not per-field | `visibility_mask uint8`. Compact but opaque. | |

**User's choice:** String field with `visible` companion
**Notes:** Matches existing `poc.visible` and `ixlan.ixf_ixp_member_list_url_visible` patterns.

---

## Default value strategy

| Option | Description | Selected |
|--------|-------------|----------|
| Mirror upstream default per-field (Recommended) | Match what PeeringDB itself defaults. | ✓ |
| Always default to Public | Permissive baseline. Risk of leaks if upstream omits the field. | |
| Always default to Users | Restrictive baseline. Risk of hiding legitimate Public rows. | |

**User's choice:** Mirror upstream default per-field
**Notes:** —

---

## Migration handling

| Option | Description | Selected |
|--------|-------------|----------|
| Ent auto-migrate at startup (Recommended) | Existing pattern; LiteFS replicates schema via LTX. | ✓ |
| Explicit migration file checked in | More control, fights ent's normal flow. | |
| Trigger a full re-sync after upgrade | Heavy; loses cursor state. | |

**User's choice:** Ent auto-migrate at startup
**Notes:** —

---

## Claude's Discretion

- Exact field naming for new `*_visible` siblings
- Plan splitting (per-entity vs bundled) once phase 57's diff is known

## Deferred Ideas

- Migrating `poc.visible` to a typed enum
- Backfilling NULL `*_visible` post-upgrade
