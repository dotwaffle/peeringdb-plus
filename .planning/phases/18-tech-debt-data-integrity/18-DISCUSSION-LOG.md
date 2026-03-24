# Phase 18: Tech Debt & Data Integrity - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-24
**Phase:** 18-tech-debt-data-integrity
**Areas discussed:** meta.generated verification, dead code cleanup scope

---

## meta.generated Verification

| Option | Description | Selected |
|--------|-------------|----------|
| Automated test | Add flag-gated live integration test against beta.peeringdb.com | |
| Manual with docs | curl API manually, document findings in markdown | |
| Both | Manual investigation first, then codify into automated test | ✓ |

**User's choice:** Both — manual investigation first, then automated test
**Notes:** Follows existing `-peeringdb-live` pattern. Test 3 request patterns (full, paginated incremental, empty result).

---

## Dead Code Cleanup Scope (DEBT-01)

| Option | Description | Selected |
|--------|-------------|----------|
| Docs only | DEBT-01 done via quick task, just update planning docs | |
| Full audit | Grep for remaining IsPrimary references, clean up test helpers, update all docs | ✓ |

**User's choice:** Full audit
**Notes:** Quick task 260324-lc5 already converted IsPrimary to func() bool. Phase 18 does a comprehensive sweep.

---

## Claude's Discretion

- Which additional PeeringDB types to test for meta.generated consistency
- Whether to add DEBUG-level logging for meta.generated values
- Test file organization

## Deferred Ideas

None
