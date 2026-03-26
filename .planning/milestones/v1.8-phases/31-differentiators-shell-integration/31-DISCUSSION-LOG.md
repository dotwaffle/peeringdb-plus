# Phase 31: Differentiators & Shell Integration - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-25
**Phase:** 31-differentiators-shell-integration
**Areas discussed:** Short format, Section filtering, Width adaptation, Freshness footer, Shell completions

---

## Short Format Design

| Option | Description | Selected |
|--------|-------------|----------|
| Key identity + primary metric (Recommended) | Pipe-delimited: AS13335 | Cloudflare, Inc. | Open | 304 IXs | ✓ |
| Minimal identity only | Just name + identifier. Ultra-compact. | |
| Tab-separated for scripting | TSV format. Machine-friendly, less human-readable. | |

**User's choice:** Key identity + primary metric
**Notes:** None

---

## Section Filtering Names

| Option | Description | Selected |
|--------|-------------|----------|
| Short names matching entity types (Recommended) | ix, fac, net, carrier, campus, contact, prefix. | |
| Descriptive names | exchanges, facilities, networks, etc. More readable. | |
| Both short and long aliases | Accept both: ix or exchanges. Most flexible. | ✓ |

**User's choice:** Both short and long aliases
**Notes:** None

---

## Width Adaptation

| Option | Description | Selected |
|--------|-------------|----------|
| Truncate values with ellipsis (Recommended) | Long values truncated with '…'. Columns never dropped. | |
| Drop columns progressively | Drop least-important columns first. Values stay full-length. | ✓ |
| Wrap long values | Long values wrap to next line. No truncation or dropping. | |

**User's choice:** Drop columns progressively
**Notes:** Least-important columns dropped first (e.g., IPv6 before IPv4, path before name). Values never truncated.

---

## Freshness Footer

| Option | Description | Selected |
|--------|-------------|----------|
| Timestamp + age (Recommended) | ISO timestamp + relative age: '12 minutes ago'. | ✓ |
| Timestamp only | Just ISO timestamp. Minimal. | |
| Timestamp + age + record count | Add total record count for completeness. | |

**User's choice:** Timestamp + age
**Notes:** None

---

## Shell Completion Scope

| Option | Description | Selected |
|--------|-------------|----------|
| Paths + params + cached ASNs (Recommended) | Complete paths, params, and cached entities. | |
| Paths + params only | Paths and params. No entity completion. | |
| Full server-side completion | Real-time server queries on each tab press. | ✓ |

**User's choice:** Full server-side completion
**Notes:** None

---

## Completion Implementation

| Option | Description | Selected |
|--------|-------------|----------|
| Search-as-you-type via server | Calls server on each tab press. Always fresh, ~100ms latency. | ✓ |
| Bulk download + local cache | Downloads full list, caches locally. Instant but stale. | |
| Hybrid: cached names, live search fallback | Cache for fast, server for fresh. Most complex. | |

**User's choice:** Search-as-you-type via server
**Notes:** ~100ms latency per tab press acceptable for always-fresh results.

---

## Claude's Discretion

- Column priority ordering per entity type for width adaptation
- Completion script implementation details (bash vs zsh API differences)
- Freshness footer formatting
- Alias/function examples in help text

## Deferred Ideas

None
