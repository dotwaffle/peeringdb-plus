# Phase 62: API key default & docs - Discussion Log

> **Audit trail only.** Decisions are captured in CONTEXT.md.

**Date:** 2026-04-16
**Phase:** 62-api-key-default-docs
**Areas discussed:** Fly secret rollout, CONFIGURATION.md layout, ARCHITECTURE.md depth

---

## Production Fly secret rollout

| Option | Description | Selected |
|--------|-------------|----------|
| Operator sets manually; docs include the command (Recommended) | Single command + verification; no CI surface to secure. | ✓ |
| GitHub Actions sets it from a repo secret | Adds CI complexity; bigger blast radius. | |
| Manual + Makefile target as memory aid | Trivial helper; more surface to maintain. | |

**User's choice:** Operator sets manually
**Notes:** —

---

## CONFIGURATION.md presentation

| Option | Description | Selected |
|--------|-------------|----------|
| Update env table + add 'Privacy & Tiers' subsection (Recommended) | Canonical table updated; focused subsection explains the model. | ✓ |
| Just the table; privacy model in ARCHITECTURE.md | Strict separation; operators must read two files. | |
| New top-level docs/PRIVACY.md page | Most discoverable; third doc to maintain consistency across. | |

**User's choice:** Update env table + 'Privacy & Tiers' subsection
**Notes:** —

---

## ARCHITECTURE.md depth

| Option | Description | Selected |
|--------|-------------|----------|
| Prose section + sequence diagram (Recommended) | Diagram makes bypass+filter inversion obvious. | ✓ |
| Prose only | Quicker; loses the visual. | |
| Prose + diagram + per-surface mapping table | Most thorough; longest to maintain. | |

**User's choice:** Prose section + sequence diagram
**Notes:** —

---

## Claude's Discretion

- Exact link target for PeeringDB API-key obtaining instructions
- Diagram format (Mermaid preferred; ASCII acceptable)
- Cross-link layout between CONFIGURATION/DEPLOYMENT/ARCHITECTURE

## Deferred Ideas

- Multi-environment fly secret management
- Automated post-deploy smoke test in CI
- Documentation page on visibility history at PeeringDB
