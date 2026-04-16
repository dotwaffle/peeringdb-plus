# Phase 59: ent Privacy policy + sync bypass - Discussion Log

> **Audit trail only.** Decisions are captured in CONTEXT.md.

**Date:** 2026-04-16
**Phase:** 59-ent-privacy-policy-sync-bypass
**Areas discussed:** Policy structure, tier propagation, sync bypass scope, env var parsing strictness, direct-lookup behaviour, bypass-leak verification

---

## Privacy policy structure

| Option | Description | Selected |
|--------|-------------|----------|
| Per-entity Policy() methods (Recommended) | Only entities with visibility get Policy(); surgical, audit-friendly. | ✓ |
| Global Mixin applied to every schema | Less duplication; does work for non-affected entities. | |
| Single composed policy via QueryRules | Middle ground. | |

**User's choice:** Per-entity Policy() methods
**Notes:** —

---

## Tier propagation

| Option | Description | Selected |
|--------|-------------|----------|
| Typed context key set by middleware (Recommended) | New `internal/privctx` package; standard Go pattern. | ✓ |
| ent privacy.DecisionContext directly | Middleware skips policy for users-tier callers. | |
| Custom ent context Decoration | Tighter coupling to ent internals. | |

**User's choice:** Typed context key set by middleware
**Notes:** Crucial for v1.15 OAuth — same `privctx.WithTier` call site from the OAuth callback.

---

## Sync-worker bypass scope

| Option | Description | Selected |
|--------|-------------|----------|
| Wrap once at the worker entry (Recommended) | `Sync(ctx)` re-binds ctx with `privacy.Allow` at the top; one audit point. | ✓ |
| Per upsert call | More explicit, more diff, more risk. | |
| Dedicated sync ent client wired with privacy.Allow | Clean separation; doubles client construction. | |

**User's choice:** Wrap once at the worker entry
**Notes:** —

---

## PDBPLUS_PUBLIC_TIER parsing strictness

| Option | Description | Selected |
|--------|-------------|----------|
| Strict: only 'public' or 'users' accepted (Recommended) | Matches PDBPLUS_SYNC_MODE pattern; typo = startup error. | ✓ |
| Permissive: any non-empty / 'true' / '1' = users | Looser; typo silently treats as public. | |
| Strict but case-insensitive accept | Tiny ergonomics win. | |

**User's choice:** Strict: only 'public' or 'users' accepted
**Notes:** —

---

## Direct-lookup behaviour for filtered rows

| Option | Description | Selected |
|--------|-------------|----------|
| 404 Not Found | Mirrors upstream; doesn't leak existence; symmetrical with list filtering. | |
| 403 Forbidden | Honest; leaks existence via probing. | |
| 401 Unauthorized | Misleading without an auth flow. | |
| Mixed: 404 from /api/, gRPC NotFound, GraphQL null + error | Native idiom per surface; same semantic. | ✓ |

**User's choice:** Mixed: per-surface "doesn't exist" idioms
**Notes:** Same semantic as 404 across the board, just rendered in each surface's native style.

---

## Bypass-leak verification

| Option | Description | Selected |
|--------|-------------|----------|
| Dedicated test with a Users-visibility row (Recommended) | Behavioural; catches actual leaks. | ✓ |
| Source-scan regression lock | Cheap drift detection; doesn't catch behavioural leaks. | |
| Both: behavioural + source-scan | Belt and braces. | |

**User's choice:** Dedicated test with a Users-visibility row
**Notes:** Test belongs in this phase, not phase 60 — it's the policy's correctness contract.

---

## Claude's Discretion

- Internal naming inside `internal/privctx`
- Exact middleware placement in the chain (between Logging and Caching is the constraint)
- Whether to emit per-request slog of tier in this phase or defer to phase 61

## Deferred Ideas

- Multiple tier gradations (TierAdmin etc.)
- Per-org membership-based tier (depends on v1.15 OAuth)
- Mutation policies
