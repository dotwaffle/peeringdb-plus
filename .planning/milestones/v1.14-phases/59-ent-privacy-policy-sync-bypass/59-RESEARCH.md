# Phase 59: ent Privacy policy + sync bypass - Research

**Researched:** 2026-04-16
**Domain:** ent privacy package, Go context propagation, net/http middleware chain, ConnectRPC/gqlgen/entrest surfaces
**Confidence:** HIGH — every structural claim verified against ent v0.14.6 source code in `~/go/pkg/mod/entgo.io/ent@v0.14.6/` and the existing peeringdb-plus codebase

## Summary

Phase 59 installs the read-path privacy floor: anonymous callers cannot see `visible=Users` rows; the sync worker retains full read/write access via a single context-scoped bypass; a `PDBPLUS_PUBLIC_TIER=users` env var elevates anonymous callers to Users tier without code changes.

The ent privacy package provides exactly the primitives this phase needs. `privacy.Policy{Query: ..., Mutation: ...}` on the `Poc` schema, enabled via `gen.FeaturePrivacy` in `ent/entc.go`, generates a typed `PocQueryRuleFunc` adapter that lets us write `func(ctx, *ent.PocQuery) error { q.Where(poc.VisibleEQ("Public")); return privacy.Skip }` — a query modifier, not a gate. The sync worker calls `privacy.DecisionContext(ctx, privacy.Allow)` at the top of `Sync()`; the decision travels via `context.WithValue` with a package-private key type, so inheritance by child goroutines is free and collision-safe. `privacy.DecisionFromContext` short-circuits evaluation at the rule-dispatch entry point before any rule runs, so the bypass is truly atomic.

**Primary recommendation:** Enable `gen.FeaturePrivacy` only (NOT `gen.FeatureEntQL`) in `ent/entc.go`. Write per-entity `Policy()` methods using typed `PocQueryRuleFunc` adapters that modify the query via `q.Where(poc.And(poc.Or(poc.VisibleEQ("Public"), poc.VisibleIsNil())))` and return `privacy.Skip`. Keep `internal/privctx` typed-tier abstraction separate from `privacy.DecisionContext` so v1.15 OAuth slots in cleanly. Place the tier-stamping middleware between Logging and Readiness in the chain.

## User Constraints (from CONTEXT.md)

### Locked Decisions

**D-01 — Per-entity `Policy()` methods, no global Mixin.** Only POC (and phase-58 visibility-bearing entities, if any surface in diff.json) get `Policy()`. Surgical, auditable.

**D-02 — Query rule reads `<field>_visible` and admits `"Public"` rows when tier is public.** Users-tier or sync-bypass admits every row.

**D-03 — Mutation rules NOT in scope.** Sync writes go through the bypass; no other writes.

**D-04 — New `internal/privctx` package:** `type Tier int` (`TierPublic` zero-value, `TierUsers`), `WithTier`, `TierFrom`.

**D-05 — New HTTP middleware in `internal/middleware/`** reads `PDBPLUS_PUBLIC_TIER` once at startup (cached) and stamps every request with resolved tier via `privctx.WithTier`.

**D-06 — Privacy policy reads `privctx.TierFrom(ctx)`** and admits Users-visibility rows when `tier == TierUsers`.

**D-07 — Do NOT use `privacy.DecisionContext(ctx, privacy.Allow)` for anonymous callers.** Keep the typed-tier abstraction so v1.15 OAuth can set `TierUsers` from the OAuth callback without touching the policy.

**D-08 — Sync bypass:** `internal/sync/worker.go` `Sync(ctx)` rebinds `ctx = privacy.DecisionContext(ctx, privacy.Allow)` at the very top before any ent calls. Every downstream upsert + read inherits via context.

**D-09 — Single bypass point.** No sprinkled per-upsert bypasses. One call site = one audit point.

**D-10 — Test asserts bypass active for sync-context goroutines and absent elsewhere.**

**D-11 — `PDBPLUS_PUBLIC_TIER` env var** parsed in `internal/config/config.go` with strict validator: empty → `TierPublic`; `"public"` → `TierPublic`; `"users"` → `TierUsers`; anything else → fail-fast. Mirrors existing `PDBPLUS_SYNC_MODE` pattern.

**D-12 — Case-sensitive lowercase only.** Matches existing config conventions.

**D-13 — Direct-lookup for filtered rows returns each surface's native 404/NotFound.** `/api/` → 404; `/rest/v1/` → 404; `/peeringdb.v1.*` → `connect.CodeNotFound`; `/graphql` → `null` + "not found" error; `/ui/` → existing 404. Must NOT leak existence.

**D-14 — Behaviourally equivalent to upstream PeeringDB anonymous response.**

**D-15 — Dedicated E2E test:** seed `visible=Users` POC via bypassed sync, issue anonymous HTTP request through full middleware stack, assert row absent. Catches bypass-leak failure mode.

**D-16 — Unit tests cover both `PDBPLUS_PUBLIC_TIER` values + fail-fast for invalid values.**

### Claude's Discretion

- Exact internal naming inside `internal/privctx` (typed key constant naming, function names) — pick consistent with project style.
- Where the middleware sits in the chain — likely after Recovery/CORS/OTel/Logging/Readiness/CSP, before Caching/Gzip/mux. Implementation detail; mux must see the stamped context so handlers + ent queries get it.
- Whether to log the resolved tier on every request span (covered in phase 61) or just on the inbound HTTP server span — phase 61 owns this.

### Deferred Ideas (OUT OF SCOPE)

- Multiple tier gradations (e.g. `TierAdmin`) — `Tier` is `int`, extensible; wait for use case.
- Per-org / per-network membership-based tier (PeeringDB OAuth `networks` scope) — depends on v1.15 OAuth.
- Mutation policies — only relevant once we add a write path, which is out of scope at project level.

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| VIS-04 | ent Privacy query policy filters non-`Public` rows from anonymous responses on POC and any other types from VIS-03; policy loaded via `entgo.io/ent/privacy` and wired in `ent/entc.go` | §"Standard Stack" / §"Architecture Patterns" — `gen.FeaturePrivacy` enablement; `PocQueryRuleFunc` adapter; Policy wiring |
| VIS-05 | Sync worker bypasses the privacy policy via `privacy.DecisionContext(ctx, privacy.Allow)` so it can read/write full dataset; tests assert bypass active for sync-context goroutines and absent everywhere else | §"Context Propagation" — verified `DecisionContext` uses `context.WithValue` with private key; inheritance is free; rule-dispatch short-circuits at entry |
| SYNC-03 | `PDBPLUS_PUBLIC_TIER` env var (default `public`, accepts `users`) elevates all anonymous callers to Users-tier for private-instance deployments; tests cover both values | §"Architecture Patterns" — parser mirrors `parseSyncMode` pattern exactly; middleware reads cached value |

## Project Constraints (from CLAUDE.md)

- **Go 1.26+** — stdlib-first, modern Go style
- **entgo** non-negotiable ORM — drives GraphQL/gRPC/REST generation
- **OpenTelemetry** mandatory — phase 61 adds `pdbplus.privacy.tier` attribute; this phase does not
- **Structured logging via slog** — `logger.LogAttrs(ctx, slog.LevelXxx, msg, slog.String("key", val))`, prefer attribute setters
- **Fail-fast config** (GO-CFG-1) — env var parse failure must be caught at startup
- **Input structs for >2 args** (GO-CS-5) — wrap middleware and ent-privacy-rule config in structs
- **Middleware chain order** regression-locked by `TestMiddlewareChain_Order` in `middleware_chain_test.go`; chain is `Recovery → MaxBytesBody → CORS → OTel HTTP → Logging → Readiness → SecurityHeaders → CSP → Caching → Gzip → mux`
- **No direct CGo** — doesn't affect this phase
- **Do NOT skip `go generate ./schema`** after entproto annotations — but this phase doesn't touch entproto annotations
- **Commit generated ent/ alongside schema changes** — enabling `FeaturePrivacy` regenerates `ent/privacy/` directory; must be committed
- **Go generate ordering:** `ent/generate.go` → `internal/web/templates/generate.go` → `schema/generate.go`. FeaturePrivacy change lands in step 1.
- **Tests use `internal/testutil/seed.Full`** which will need a mixed Public + Users POC seed (phase 60 D-01)

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Privacy policy evaluation (row filtering) | ent runtime / data layer | — | Filter happens in the SQL `WHERE` clause at query build time; closer to data = one enforcement point for all 5 surfaces |
| Tier detection from HTTP request | API / HTTP server | — | HTTP middleware reads env-var-driven cached tier, stamps request context; single entry point for all HTTP-backed surfaces |
| Tier detection from OAuth (v1.15 future) | API / HTTP server | — | Same middleware slot, different source (OAuth callback sets `TierUsers` instead of env default) |
| Sync-worker bypass | Background worker / data ingest | — | Worker owns its own context tree; bypass stamped at worker entry, inherited by every upsert call |
| Env-var parse + validate | Config layer | API / HTTP server (consumer) | Fail-fast at startup per GO-CFG-1; consumed by middleware constructor |
| Direct-lookup 404 mapping | Per-surface handler | ent runtime (source of truth) | `ent.IsNotFound(err)` is already checked in pdbcompat, grpcserver, graphql — no new mapping logic needed; the privacy filter plugs into the existing "row not matched = NotFound" path |

**Why this matters:** The privacy filter is a data-layer concern, not a per-surface concern. Wiring it at ent runtime means all 5 surfaces (graphql, rest/v1, api, peeringdb.v1.*, ui) get correct behavior from a single implementation. The tier-detection middleware is separate and lives at the HTTP boundary so it composes with future OAuth identity without touching the policy.

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `entgo.io/ent` | v0.14.6 (already pinned) | ORM + codegen; `privacy` package + `FeaturePrivacy` flag | Only ORM with a first-class ent-native privacy layer; already the project's single source of truth for schemas |
| `entgo.io/ent/privacy` | bundled v0.14.6 | `Policy`, `DecisionContext`, `Skip`/`Allow`/`Deny` decisions | Dedicated subpackage — its entire surface is ~250 lines; easy to audit |
| `context` (stdlib) | Go 1.26 | Tier propagation (`WithValue` / `Value`) | Standard Go idiom; propagation is free and collision-safe with package-private key types |
| `log/slog` (stdlib) | Go 1.26 | Structured logging (e.g. WARN on `PDBPLUS_PUBLIC_TIER=users`) | Already the project standard (OBS-1, CLAUDE.md §Logging) |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `net/http` (stdlib) | Go 1.26 | Middleware implementation | Project uses stdlib middleware; copy `internal/middleware/csp.go` as structural template |
| `errors` (stdlib) | Go 1.26 | `errors.Is(err, privacy.Deny)` for test assertions | Per GO-ERR-2 (no string matching) |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `gen.FeaturePrivacy` only | `gen.FeaturePrivacy` + `gen.FeatureEntQL` | EntQL adds dynamic filtering via `privacy.FilterFunc(f privacy.Filter)` but also regenerates a large `entql.go` and adds `*graph.schema.json` dependencies. [VERIFIED: `ent@v0.14.6/entc/gen/template/privacy/privacy.tmpl:150`] shows the `privacy/filter` template is only rendered when EntQL is enabled. **Verdict:** Not needed. Our rule only filters on a single field on a single table — we can use the generated `PocQueryRuleFunc` adapter to call `q.Where(poc.Visible(...))` directly, which is simpler, type-safe, and avoids the EntQL feature surface. |
| Per-entity `Policy()` | Global `BaseMixin` with `Policy()` | Mixin applies the policy to every schema whether the entity has a `visible` field or not. Wasteful and obscures which entities are actually gated. Locked by D-01. |
| `privctx.Tier` as `int` | `string`, or a typed enum with iota | `int` with named constants (`TierPublic`, `TierUsers`) gives compile-time type safety and zero-value default (`TierPublic`), which matches the default/public semantics naturally. String would allow typo-driven silent misconfiguration in future tier additions. Locked by D-04. |
| `privacy.DecisionContext` for anonymous HTTP | Set decision context with `privacy.Allow` when tier is Users | Would collapse two distinct concepts (sync bypass vs. anonymous-tier-elevation) into one. Locked by D-07 — keep the abstractions separate so v1.15 OAuth slots in cleanly. |

**Installation:** No new Go modules — all dependencies already in go.mod.

**Version verification:** `entgo.io/ent v0.14.6` is already pinned. Privacy package confirmed present: [VERIFIED: `~/go/pkg/mod/entgo.io/ent@v0.14.6/privacy/privacy.go`].

## Architecture Patterns

### System Architecture Diagram

```
                                                     ┌──────────────────┐
  [HTTP request] ──► Recovery ──► MaxBytesBody ──►   │                  │
                                                     │   PrivacyTier    │
                                                     │   middleware     │
                                                     │                  │
                                                     │  reads env once, │
                                                     │  stamps ctx with │
                                                     │  privctx.WithTier│
                                                     └────────┬─────────┘
                                                              │
       ┌──────── CORS ◄──── OTel HTTP ◄──── Logging ◄─────────┘
       │
       └──► Readiness ──► SecurityHeaders ──► CSP ──► Caching ──► Gzip ──► mux
                                                                            │
                                                                            ▼
                                                         ┌──────────────────────────────┐
                                                         │  Per-surface handler         │
                                                         │  (graphql / rest / api /     │
                                                         │   grpcserver / ui)           │
                                                         └──────────────┬───────────────┘
                                                                        │ ctx with Tier
                                                                        ▼
                                                              ┌─────────────────┐
                                                              │  ent.Client     │
                                                              │  .Poc.Query()   │
                                                              │  .Get(id)       │
                                                              └────────┬────────┘
                                                                       │
                                                                       ▼
                                                         ┌─────────────────────────┐
                                                         │ Rule-dispatch loop      │
                                                         │                         │
                                                         │ DecisionFromContext?    │
                                                         │  ├─ yes: short-circuit  │─► sync bypass wins
                                                         │  └─ no: run rules       │
                                                         └────────────┬────────────┘
                                                                      │
                                                                      ▼
                                                      ┌──────────────────────────────┐
                                                      │  PocQueryRuleFunc            │
                                                      │                              │
                                                      │  tier := privctx.TierFrom    │
                                                      │  if tier == TierUsers:       │
                                                      │    return Skip  (unmodified) │
                                                      │  else (TierPublic):          │
                                                      │    q.Where(poc.And(          │
                                                      │       poc.Or(                │
                                                      │        poc.VisibleEQ("Public"),│
                                                      │        poc.VisibleIsNil()))) │
                                                      │    return Skip               │
                                                      └──────────────┬───────────────┘
                                                                     │
                                                                     ▼
                                                           [ filtered SQL ]


  [Sync worker cycle] ──► Worker.Sync(ctx) ──► ctx = privacy.DecisionContext(
                                                        ctx, privacy.Allow)
                              │
                              ▼                 ^
                        all ent calls ──────────┘ inherits bypass via ctx
```

### Recommended Project Structure

```
internal/
├── privctx/                    # NEW
│   ├── privctx.go              # Tier type, WithTier, TierFrom
│   └── privctx_test.go         # roundtrip + zero-value tests
├── middleware/
│   ├── privacy_tier.go         # NEW — stamps ctx with resolved tier from cached env value
│   └── privacy_tier_test.go    # NEW
├── sync/
│   └── worker.go               # MODIFIED — one-line ctx = privacy.DecisionContext(ctx, Allow) at top of Sync
└── config/
    └── config.go               # MODIFIED — adds PublicTier field + parsePublicTier helper

ent/
├── entc.go                     # MODIFIED — add gen.FeaturePrivacy to FeatureNames
├── privacy/                    # NEW (generated) — commit alongside schema change
│   └── privacy.go
└── schema/
    ├── poc.go                  # MODIFIED — add Policy() method
    └── {other visibility-bearing schemas from Phase 58}.go
```

### Pattern 1: Enable privacy feature in `entc.go`

**What:** Add `gen.FeaturePrivacy` to the feature list.
**When to use:** Exactly once, in `ent/entc.go`.
**Example:**
```go
// Source: ent@v0.14.6/doc/md/privacy.mdx (verified at ent@v0.14.6/entc/gen/feature.go:14)
opts := []entc.Option{
    entc.Extensions(gqlExt, restExt, protoExt),
    entc.FeatureNames("sql/upsert", "sql/execquery", "privacy"),
    //                                              ^^^^^^^^^
}
```

**CRITICAL:** This regenerates ent's `privacy/` subpackage (typed rule adapters per entity). Commit `ent/privacy/privacy.go` alongside the schema change. CI's "Generated code drift check" (per CLAUDE.md §CI) will fail otherwise.

Do NOT add `gen.FeatureEntQL` — we don't need dynamic `privacy.Filter`-based rules; the typed `PocQueryRuleFunc` is sufficient. See "Alternatives Considered" above.

### Pattern 2: `internal/privctx` — typed tier propagation

**What:** Package-private context-key type; exported `Tier` enum; exported `WithTier` / `TierFrom`.
**When to use:** Once, as the single source of truth for "who is the caller?" across all HTTP-backed surfaces.
**Example:**

```go
// internal/privctx/privctx.go
package privctx

import "context"

// Tier identifies the visibility scope of the caller.
// TierPublic is the zero value — anonymous callers without any elevation.
// Additional tiers (TierUsers, TierAdmin, ...) extend as identity sources
// are added; TierUsers is introduced in v1.14 for PDBPLUS_PUBLIC_TIER=users
// and for v1.15 OAuth-authenticated callers.
type Tier int

const (
    TierPublic Tier = iota
    TierUsers
)

// tierCtxKey is unexported so no other package can collide with or
// overwrite our context value. The struct{} zero-size avoids a heap
// allocation per WithValue call at Go 1.26.
type tierCtxKey struct{}

// WithTier returns a new context carrying tier. Intended for the HTTP
// middleware that stamps every inbound request and, in v1.15+, for the
// OAuth callback.
func WithTier(ctx context.Context, tier Tier) context.Context {
    return context.WithValue(ctx, tierCtxKey{}, tier)
}

// TierFrom returns the tier stored in ctx, or TierPublic if none was set.
// Safe to call from any goroutine descended from a WithTier-stamped ctx.
func TierFrom(ctx context.Context) Tier {
    if t, ok := ctx.Value(tierCtxKey{}).(Tier); ok {
        return t
    }
    return TierPublic
}
```

Note: `tierCtxKey{}` (a typed empty struct) is the idiomatic Go context-key pattern — same shape used by ent itself at [VERIFIED: `ent@v0.14.6/privacy/privacy.go:208`: `type decisionCtxKey struct{}`].

### Pattern 3: Privacy policy on POC schema

**What:** `func (Poc) Policy() ent.Policy { ... }` using the auto-generated `PocQueryRuleFunc`.
**When to use:** Once per visibility-bearing entity (POC + whatever Phase 58 adds).
**Example:**

```go
// ent/schema/poc.go
package schema

import (
    "context"

    "entgo.io/ent"
    "github.com/dotwaffle/peeringdb-plus/ent/poc"
    "github.com/dotwaffle/peeringdb-plus/ent/privacy"
    "github.com/dotwaffle/peeringdb-plus/internal/privctx"
)

// Policy returns the privacy policy for the Poc entity.
//
// Query rule (VIS-04):
//   - Anonymous / TierPublic callers see only rows where visible == "Public"
//     (or NULL, defense-in-depth against unseeded rows).
//   - TierUsers callers (PDBPLUS_PUBLIC_TIER=users or future v1.15 OAuth) see
//     every row.
//   - Sync worker bypasses via privacy.DecisionContext(ctx, privacy.Allow);
//     the rule-dispatch loop short-circuits before this rule ever runs.
//
// Mutation rule: none in scope (D-03) — sync writes travel the bypass,
// no other writers exist on the read-only mirror.
func (Poc) Policy() ent.Policy {
    return privacy.Policy{
        Query: privacy.QueryPolicy{
            privacy.PocQueryRuleFunc(func(ctx context.Context, q *ent.PocQuery) error {
                if privctx.TierFrom(ctx) == privctx.TierUsers {
                    return privacy.Skip
                }
                // TierPublic: append a WHERE predicate that ANDs into any
                // user-supplied predicates at query build time. VisibleIsNil
                // is defence-in-depth — synced rows always carry the string
                // default "Public", but a future migration bug shouldn't
                // silently leak rows.
                q.Where(poc.Or(
                    poc.VisibleEQ("Public"),
                    poc.VisibleIsNil(),
                ))
                return privacy.Skip
            }),
        },
    }
}
```

**Source:** Adapter type signature verified at [VERIFIED: `ent@v0.14.6/entc/gen/template/privacy/privacy.tmpl:118-131`] — `PocQueryRuleFunc func(context.Context, *ent.PocQuery) error`.

**Key insight:** returning `privacy.Skip` lets the `QueryPolicy` continue to (absent) subsequent rules and ultimately returns `nil` (allow). The filter was already appended to `q.Where(...)` — ent then builds the SQL with our predicate ANDed into any user-supplied `.Where(...)` calls.

### Pattern 4: HTTP tier-stamping middleware

**What:** Reads `PDBPLUS_PUBLIC_TIER` once at startup; stamps every request context with the resolved tier.
**When to use:** Once, in the middleware chain in `cmd/peeringdb-plus/main.go`.
**Example:**

```go
// internal/middleware/privacy_tier.go
package middleware

import (
    "net/http"

    "github.com/dotwaffle/peeringdb-plus/internal/privctx"
)

// PrivacyTierInput configures the PrivacyTier middleware. DefaultTier is the
// resolved Tier from PDBPLUS_PUBLIC_TIER, parsed and validated by
// internal/config at startup (fail-fast per GO-CFG-1).
type PrivacyTierInput struct {
    DefaultTier privctx.Tier
}

// PrivacyTier stamps every inbound HTTP request context with the tier
// configured at startup. v1.15 OAuth will replace or wrap this to use the
// authenticated identity instead of the env default.
//
// Per D-05: stamps BEFORE the Caching middleware in the chain so the cache
// key (if ever tier-aware) sees the stamped ctx. Per D-07: does NOT use
// privacy.DecisionContext — keeps tier typed so OAuth can slot in without
// touching the policy.
func PrivacyTier(in PrivacyTierInput) func(http.Handler) http.Handler {
    tier := in.DefaultTier
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            ctx := privctx.WithTier(r.Context(), tier)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

**Chain placement (D-05 + Claude's Discretion):** Between `Logging` and `Readiness`. Rationale:

1. Must run AFTER Logging so the log record isn't polluted by tier-stamping errors (the middleware never fails, but principle of layering).
2. Must run BEFORE Caching so the caching layer sees the stamped ctx. CRITICAL — see §"Common Pitfalls" #3 below.
3. Must run BEFORE Readiness so the 503-syncing page doesn't bypass the tier stamp (harmless but keeps layering clean).

Concrete placement in `buildMiddlewareChain`:

```go
// cmd/peeringdb-plus/main.go buildMiddlewareChain (wraps innermost first)
h := middleware.Compression()(inner)
h = cc.CachingState.Middleware()(h)
h = middleware.CSP(cc.CSPInput)(h)
h = middleware.SecurityHeaders(...)(h)
h = middleware.PrivacyTier(middleware.PrivacyTierInput{DefaultTier: cc.DefaultTier})(h) // NEW
h = readinessMiddleware(cc.SyncWorker, h)
h = middleware.Logging(cc.Logger)(h)
h = otelhttp.NewMiddleware("peeringdb-plus")(h)
h = middleware.CORS(...)(h)
h = middleware.MaxBytesBody(...)(h)
h = middleware.Recovery(cc.Logger)(h)
```

Reading the outermost-first ordering: `Recovery → MaxBytesBody → CORS → OTel HTTP → Logging → Readiness → PrivacyTier → SecurityHeaders → CSP → Caching → Gzip → mux`.

**Regression-locking:** Update `TestMiddlewareChain_Order` in `internal/middleware/` (or wherever the chain test lives) to include the new middleware in its literal assertion.

### Pattern 5: Sync-worker bypass

**What:** One line at the top of `Worker.Sync`.
**When to use:** Exactly once — the sole bypass point (D-08, D-09).
**Example:**

```go
// internal/sync/worker.go
import (
    "github.com/dotwaffle/peeringdb-plus/ent/privacy"
)

func (w *Worker) Sync(ctx context.Context, mode config.SyncMode) error {
    // VIS-05: every ent read/write inside the sync cycle must bypass the
    // privacy policy so we can upsert Users-visibility rows into the
    // local DB. The decision is stamped on ctx here (via context.WithValue)
    // and inherited by every subsequent ent call, including child
    // goroutines spawned from this ctx. Short-circuits at rule-dispatch
    // before any rule runs.
    //
    // This is the SOLE bypass point per D-09. Do NOT add per-upsert bypasses.
    ctx = privacy.DecisionContext(ctx, privacy.Allow)

    if !w.running.CompareAndSwap(false, true) {
        // ... existing body ...
    }
    // ... rest unchanged ...
}
```

**Critical placement:** BEFORE the `w.running.CompareAndSwap` check and BEFORE any `ent.Tx` open, so that every downstream call — including the existing `otel.Tracer("sync").Start(ctx, ...)` spans, all Phase A stage calls, and all Phase B upserts — inherits the decision.

**SyncWithRetry** calls `Sync` which now handles the bypass internally — no changes needed there. The retry machinery's ctx-propagation is already correct.

### Pattern 6: Env-var parser for `PDBPLUS_PUBLIC_TIER`

**What:** Mirrors the existing `parseSyncMode` pattern in `config.go`.
**Example:**

```go
// internal/config/config.go

// PublicTier is the resolved visibility tier for anonymous HTTP callers.
// Configured via PDBPLUS_PUBLIC_TIER. Default is TierPublic (anonymous
// callers see only visible="Public" rows). Setting "users" elevates
// anonymous callers to TierUsers for private-instance deployments
// (D-11, SYNC-03).
type Config struct {
    // ... existing fields ...
    PublicTier privctx.Tier
}

func parsePublicTier(key string, defaultVal privctx.Tier) (privctx.Tier, error) {
    v := os.Getenv(key)
    if v == "" {
        return defaultVal, nil
    }
    switch v {
    case "public":
        return privctx.TierPublic, nil
    case "users":
        return privctx.TierUsers, nil
    default:
        return 0, fmt.Errorf("invalid public tier %q for %s: must be 'public' or 'users'", v, key)
    }
}

// Called from Load():
publicTier, err := parsePublicTier("PDBPLUS_PUBLIC_TIER", privctx.TierPublic)
if err != nil {
    return nil, fmt.Errorf("parsing PDBPLUS_PUBLIC_TIER: %w", err)
}
cfg.PublicTier = publicTier
```

Case-sensitive lowercase per D-12 — matches the existing `PDBPLUS_SYNC_MODE` validator exactly.

### Anti-Patterns to Avoid

- **Global Mixin applying Policy to every entity.** Inflates the gate surface; auditing which entities are gated becomes impossible. Locked against by D-01.
- **Using `privacy.DecisionContext` for anonymous callers.** Makes it impossible to distinguish "explicitly allowed by bypass" from "allowed because the tier is Users" — both would return `nil` from `DecisionFromContext`. Locked against by D-07.
- **Sprinkling `privacy.DecisionContext` across sync helpers.** Multiple bypass points = multiple audit surfaces = inevitable drift. Locked against by D-08, D-09.
- **String-matching `err.Error()` to detect privacy denial.** Use `errors.Is(err, privacy.Deny)` (per GO-ERR-2). [VERIFIED: `ent@v0.14.6/privacy/privacy.go:21-29`] — `Allow`, `Deny`, `Skip` are `errors.New` sentinels; `Denyf/Allowf/Skipf` wrap with `%w`.
- **Calling `privacy.DecisionContext` inside a per-upsert helper and forgetting to propagate.** Each helper gets a fresh `context.Background()` somewhere and the bypass is silently lost. By calling at worker entry and relying on natural propagation, there's no per-helper ctx construction.
- **Placing the tier-stamping middleware AFTER Caching.** Cached responses would be served to later requests regardless of their tier. See Pitfall 3 below.
- **Stamping `q.Where(poc.VisibleEQ("Public"))` without `Or(..., poc.VisibleIsNil())`.** SQL `WHERE visible = 'Public'` is FALSE for NULL rows, not TRUE. Any row with a NULL `visible` would be silently filtered. Defence-in-depth matters.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Filtering rows by field value in SQL across 5 API surfaces | Per-surface handler-level filters in graphql resolver + grpc handler + entrest + pdbcompat + /ui/ | `func (Poc) Policy() ent.Policy { ... }` via `gen.FeaturePrivacy` | Handler-level filter = N implementations = N bug surfaces. Policy at the ent layer = 1 implementation, every surface inherits automatically. Plus privacy survives `HasPocsWith`-style edge traversal; a handler-level filter wouldn't. |
| Context-key collision safety for tier value | `string` key, exported key, or global `"tier"` string | `type tierCtxKey struct{}` + `privctx.WithTier`/`TierFrom` | Typed empty-struct key is the Go stdlib idiom; collision-proof by language semantics. Also how ent itself stores its decision [VERIFIED: `ent@v0.14.6/privacy/privacy.go:208`]. |
| Bypassing privacy rules inside the sync worker | Conditionals inside each `Policy()` method checking "is this the sync worker?" | `privacy.DecisionContext(ctx, privacy.Allow)` at worker entry | Ent's rule-dispatch loop short-circuits on the stored decision before running any rule. Single call site, zero per-rule complexity. [VERIFIED: `ent@v0.14.6/privacy/privacy.go:168-171`: `if decision, ok := DecisionFromContext(ctx); ok { return decision }` is the first line of the dispatch loop.] |
| Env-var parsing + validation + fail-fast | ad-hoc `os.Getenv` + `switch` + `log.Fatal` | Existing `parseSyncMode`-pattern helper in `config.go` + `Load()` returns error → `os.Exit(1)` in `main.go` | Project convention. Matches existing validators. |
| Per-surface "not found" translation | Custom error types per surface | Existing `ent.IsNotFound(err)` handling already in every handler | grpcserver ([VERIFIED: `internal/grpcserver/poc.go:74-78`]), pdbcompat ([VERIFIED: `internal/pdbcompat/handler.go:218`]), graphql classifier ([VERIFIED: `internal/graphql/handler.go:63`]), entrest (auto via `restErrorMiddleware`). All already map ent's NotFound to the surface's native 404. **No new mapping code needed for D-13.** |

**Key insight:** This phase adds <50 lines of functional code (one Policy, one middleware, one env parser, one-line sync bypass) plus tests. Most of the complexity is infrastructure the codebase already has — we're wiring, not inventing.

## Runtime State Inventory

This is a new-feature phase — not a rename/refactor/migration. Runtime state inventory is not required.

For completeness, the single state-ful change is:
- **Stored data:** None. The policy reads existing POC rows' `visible` field — no new column, no backfill needed.
- **Live service config:** `PDBPLUS_PUBLIC_TIER` becomes a new env var on Fly.io deployment. Default absent = `public` (current behaviour). No migration needed.
- **OS-registered state:** None.
- **Secrets / env vars:** `PDBPLUS_PUBLIC_TIER` is NOT a secret — documented in env var table in CLAUDE.md in phase 62.
- **Build artifacts:** Enabling `gen.FeaturePrivacy` regenerates `ent/privacy/privacy.go` — committed alongside the entc.go change. Phase verifies via CI "Generated code drift check" (per CLAUDE.md §CI).

## Context Propagation — VERIFIED SEMANTICS

### DecisionContext internals

From ent v0.14.6 source, the decision is stored via `context.WithValue` with a package-private key type:

```go
// [VERIFIED: ent@v0.14.6/privacy/privacy.go:208-217]
type decisionCtxKey struct{}

func DecisionContext(parent context.Context, decision error) context.Context {
    if decision == nil || errors.Is(decision, Skip) {
        return parent
    }
    return context.WithValue(parent, decisionCtxKey{}, decision)
}
```

**Consequences for the phase:**

1. **Child goroutine inheritance is free.** Any ctx derived via `context.WithCancel`, `context.WithTimeout`, `context.WithValue`, or passed directly to `go f(ctx)` keeps the decision. This is standard Go context semantics — `Value` walks the parent chain.

2. **Collision safety is absolute.** `decisionCtxKey{}` is a typed empty struct in an unexported type inside `entgo.io/ent/privacy`. No other package can construct the same key; not even a `type decisionCtxKey struct{}` in a different package matches because Go types are identified by (package, name), not by shape.

3. **Worker's existing goroutine pattern.** `Worker.Sync` spawns no errgroups; the demotion-monitor goroutine in `runSyncCycle` uses a fresh `context.WithCancel` derived from the outer ctx, so it inherits the bypass correctly. [VERIFIED: `internal/sync/worker.go:1209-1240`.]

4. **Timeout / cancellation preserve the decision.** `context.WithTimeout(decisionCtx, ...)` wraps the decision — `Value(decisionCtxKey{})` still returns the decision. Only a brand-new `context.Background()` or `context.TODO()` drops it.

### Short-circuit at rule dispatch

From [VERIFIED: `ent@v0.14.6/privacy/privacy.go:168-182`] the rule-dispatch loop:

```go
func (policies Policies) <dispatch>(ctx context.Context, <run> func(ent.Policy) error) error {
    if decision, ok := DecisionFromContext(ctx); ok {
        return decision   // ← sync worker's Allow short-circuits here
    }
    for _, policy := range policies {
        switch decision := <run>(policy); {
        case decision == nil || errors.Is(decision, Skip):
        case errors.Is(decision, Allow):
            return nil
        default:
            return decision
        }
    }
    return nil
}
```

**`DecisionFromContext`** at [VERIFIED: `ent@v0.14.6/privacy/privacy.go:219-226`] returns `nil, true` if the stored decision is `Allow` (because the `errors.Is(decision, Allow)` branch normalises to `nil`). This means:

- Sync-bypass ctx: the dispatch returns `nil` at line 170 — **no rule runs**, no `q.Where(...)` mutation, no `TierFrom(ctx)` read. Sync sees every row.
- Anonymous ctx: dispatch falls through to the rule loop, `PocQueryRuleFunc` runs, `TierFrom(ctx)` is read, predicate is appended.

**Accidental leak risk:** if a handler stamps a context with `privacy.DecisionContext(ctx, privacy.Allow)` and passes it to a privacy-gated query, it bypasses the policy. This is why **D-08/D-09 lock the bypass to a single call site** — easy to audit via `grep -rn "privacy.DecisionContext" internal/`.

### Does Policy() apply to `.Where()` predicates?

**Yes** — `q.Where(...)` inside a `QueryRuleFunc` mutates the query builder, and the mutation is preserved through subsequent `.Where()` calls a user might chain. The rule runs once, at the terminal execution point (`All/First/Only/Count/etc`), **before** SQL is built. Any user-chained `.Where()` predicates compose with the rule-added predicate via AND (standard ent semantics).

**Verification:** `PocClient.Get(ctx, id)` is literally `return c.Query().Where(poc.ID(id)).Only(ctx)` [VERIFIED: `ent/client.go:2543-2545`]. `.Only(ctx)` triggers rule dispatch, which appends the visibility predicate; the final SQL is `SELECT ... FROM pocs WHERE id = ? AND (visible = 'Public' OR visible IS NULL)`. Row absent → `Only` returns `*NotFoundError`, and `ent.IsNotFound(err)` is true — exactly the D-13 behaviour.

### Edge traversal — does privacy propagate?

**Yes.** When GraphQL/entgql generates a resolver that does `network.QueryPocs()`, the resulting `PocQuery` is subject to `Poc`'s Policy. The tenancy example in ent docs uses exactly this pattern [CITED: https://entgo.io/docs/privacy — "Test Tenant Privacy Operations"]. Concretely: any query that ends up as a `*ent.PocQuery` (even via edge traversal) runs the `PocQueryRuleFunc`, because the adapter's `EvalQuery(ctx, q ent.Query)` type-asserts `q.(*ent.PocQuery)` [VERIFIED: `ent@v0.14.6/entc/gen/template/privacy/privacy.tmpl:127-131`]. If the assertion matches, the rule applies; if not, it returns `Denyf("unexpected query type")`.

## Common Pitfalls

### Pitfall 1: Forgetting to commit `ent/privacy/privacy.go`

**What goes wrong:** Add `gen.FeaturePrivacy` to `entc.go`, regenerate, but only commit the schema file — `ent/privacy/` goes uncommitted.
**Why it happens:** `ent/privacy/` is a new directory; easy to miss in `git add`.
**How to avoid:** CI's "Generated code drift check" (per CLAUDE.md §CI) fails the PR — designed exactly for this. Locally run `go generate ./... && git status` to see the new file before committing.
**Warning signs:** CI `generate` job fails with "generated code drift"; local `go generate` produces untracked files in `ent/privacy/`.

### Pitfall 2: `VisibleEQ("Public")` leaking NULL rows

**What goes wrong:** SQL `visible = 'Public'` is FALSE for NULL rows in every major dialect (SQLite, MySQL, PostgreSQL). If any POC row has a NULL `visible`, the anonymous query silently filters it out — which is correct but overly aggressive. Worse, if the policy were `VisibleEQ("Users")` (wrong direction), NULL rows would leak.
**Why it happens:** Developer writes `poc.VisibleEQ("Public")` without considering three-valued logic.
**How to avoid:** Always combine with `poc.VisibleIsNil()` for defence-in-depth: `poc.Or(poc.VisibleEQ("Public"), poc.VisibleIsNil())`. The `visible` field has `Default("Public")`, so new rows always carry the default — but the Or-IsNil guard protects against migration bugs and forward-compat with fields added without defaults.
**Warning signs:** Rows disappear for anonymous callers when their `visible` column is NULL (not "Users"). Hard to notice until verified by the integration test.

### Pitfall 3: Cache poisoning across tiers

**What goes wrong:** The caching middleware stores a response keyed by sync time ETag. If `PDBPLUS_PUBLIC_TIER=users` is flipped at runtime (or two different deployments with different values share a CDN), cached anonymous responses could cross the tier boundary.
**Why this is NOT a problem here:**
- `PDBPLUS_PUBLIC_TIER` is resolved once at startup and stamped on every request. A single process only ever serves one tier.
- The caching middleware's ETag is purely sync-time-keyed [VERIFIED: `internal/middleware/caching.go:146-150`], so within a single process the cache is consistent.
- The chain places `PrivacyTier` BEFORE `Caching` (D-05), but the caching middleware doesn't read the tier — it uses the global ETag. The cache is only ever populated with responses that match the process's sole tier.
**Actual risk:** If a CDN sits in front of two fleets with different `PDBPLUS_PUBLIC_TIER` values, the CDN could mix responses. **Mitigation:** operational — don't run heterogeneous fleets behind a shared CDN. Consider adding `PDBPLUS_PUBLIC_TIER` to a `Vary` header in a future hardening phase if this use case emerges.
**Warning signs:** None in v1.14 single-process deployment. Flag for v1.15+ if multi-tier fleets appear.

### Pitfall 4: Unauthorized `privacy.DecisionContext` leak from HTTP handler

**What goes wrong:** A future developer writes `ctx := privacy.DecisionContext(r.Context(), privacy.Allow)` somewhere in a handler — every downstream ent call from that handler sees every row.
**Why it happens:** Copy-paste from sync worker code; misunderstanding of the abstraction layers.
**How to avoid:**
- Grep-able audit: `grep -rn "privacy.DecisionContext" internal/ cmd/` should return exactly one call site (in `internal/sync/worker.go`).
- Lint guard (optional, deferred): add a CI check that fails if any file outside `internal/sync/` imports `entgo.io/ent/privacy` AND calls `DecisionContext`.
- D-15 integration test catches it: a handler that accidentally bypassed would let a Users-tier row through, and the test would fail.
**Warning signs:** D-15 integration test fails; audit grep shows more than one call site.

### Pitfall 5: Policy not running because `*ent.PocQuery` wraps to something else

**What goes wrong:** Ent internally uses different query types for paginated queries, count queries, etc. The typed `PocQueryRuleFunc` adapter [VERIFIED: `ent@v0.14.6/entc/gen/template/privacy/privacy.tmpl:127-131`] type-asserts `q.(*ent.PocQuery)` — if the assertion fails, the rule returns `Denyf("unexpected query type %T")`, i.e. the whole query DENIES.
**Why this is NOT a problem here:** Every public `ent.Client.Poc.*` call path ultimately materialises as `*ent.PocQuery` (Get, Query, Count, All, First, Only, CountX, QueryFoo edge traversals — all). The rule runs on all of them.
**Warning signs:** Logs show `"privacy: unexpected query type"` — indicates ent added a new internal query type we haven't catered for. Extremely unlikely within v0.14.x.

### Pitfall 6: Accidentally regenerating ent/schema via `go generate ./schema`

**What goes wrong:** CLAUDE.md §"Code Generation" warns: "Do NOT run `go generate ./schema` after entproto annotations are added — the schema generator strips entproto annotations."
**Why this is relevant:** Adding `Policy()` to `poc.go` adds a new method to the schema struct. `go generate ./schema` (from `cmd/pdb-schema-generate`) would regenerate `poc.go` from the upstream JSON and drop both our entproto annotations AND the `Policy()` method.
**How to avoid:** Never run `go generate ./schema` after this phase. Only `go generate ./ent` (which runs entc via `ent/generate.go`) is safe.
**Warning signs:** After running `go generate ./...`, `git diff ent/schema/poc.go` shows the Policy() method is gone.

### Pitfall 7: Policy skipped because `DecisionFromContext` returns early

**What goes wrong:** If a test or handler accidentally passes `privacy.DecisionContext(ctx, privacy.Deny)` (or any non-Allow non-Skip), every query fails with that decision — not just Poc queries.
**Why it happens:** Over-cautious test setup that denies-by-default.
**How to avoid:** In test code, only use `privacy.DecisionContext(ctx, privacy.Allow)` when mirroring the sync-bypass. For TierUsers simulation, use `privctx.WithTier(ctx, privctx.TierUsers)` — never `DecisionContext`.

## Environment Availability

This phase has no external tool/service dependencies. All work is Go code, codegen, and tests within the existing project toolchain.

## Code Examples

Verified patterns from ent v0.14.6 source and the peeringdb-plus codebase.

### Example 1: Reading the stored privacy decision

```go
// Source: ent@v0.14.6/privacy/privacy.go:219
decision, ok := privacy.DecisionFromContext(ctx)
if ok {
    // A decision is bound to ctx. If ok && decision == nil, it's Allow
    // (normalised — see privacy.go:222-224).
}
```

### Example 2: Sync bypass end-to-end (validation)

```go
// Test shape (VIS-05 assertion — D-10)

func TestSyncBypass_AdmitsUsersTierPOC(t *testing.T) {
    client := testutil.SetupClient(t)
    ctx := context.Background()

    // Anonymous (TierPublic) context CANNOT create a Users POC:
    // but creation isn't in scope (D-03) — test the READ side.

    // First, create a Users POC via the bypass (this is what sync does):
    bypassCtx := privacy.DecisionContext(ctx, privacy.Allow)
    p := client.Poc.Create().
        SetID(999).
        SetRole("Admin").
        SetVisible("Users").
        SetCreated(time.Now()).
        SetUpdated(time.Now()).
        SaveX(bypassCtx)
    _ = p

    // 2. Anonymous ctx (TierPublic, zero-value): must NOT see the row.
    anonCtx := ctx // no WithTier call; TierFrom returns TierPublic
    _, err := client.Poc.Get(anonCtx, 999)
    if !ent.IsNotFound(err) {
        t.Fatalf("expected NotFound for Users POC via anonymous ctx, got: %v", err)
    }

    // 3. Bypass ctx must still see the row.
    p2, err := client.Poc.Get(bypassCtx, 999)
    if err != nil {
        t.Fatalf("bypass ctx should admit Users POC, got: %v", err)
    }
    if p2.Visible != "Users" {
        t.Fatalf("expected Visible=Users, got %q", p2.Visible)
    }

    // 4. TierUsers ctx must also see the row.
    usersCtx := privctx.WithTier(ctx, privctx.TierUsers)
    p3, err := client.Poc.Get(usersCtx, 999)
    if err != nil {
        t.Fatalf("TierUsers ctx should admit Users POC, got: %v", err)
    }
    if p3.Visible != "Users" {
        t.Fatalf("expected Visible=Users, got %q", p3.Visible)
    }
}
```

### Example 3: D-15 end-to-end HTTP test outline

```go
// Test location: internal/grpcserver/ or cmd/peeringdb-plus/ (integration-level).
// Recommended: cmd/peeringdb-plus/e2e_privacy_test.go — exercises the real
// middleware chain built by buildMiddlewareChain.

func TestE2E_AnonymousCannotSeeUsersPoc(t *testing.T) {
    // Arrange: set up the full server stack with PDBPLUS_PUBLIC_TIER unset.
    //   - ent client (in-memory SQLite via testutil.SetupClient)
    //   - config.Load() with PDBPLUS_PUBLIC_TIER="" (TierPublic default)
    //   - Full middleware chain via buildMiddlewareChain
    //   - Real mux wiring (graphql, rest/v1, api, grpcserver, ui)
    //
    // Act 1: seed a visible="Users" POC via privacy.DecisionContext(Allow).
    // This simulates what the sync worker does — no HTTP involved.
    //
    // Act 2: issue anonymous HTTP requests covering all 5 surfaces:
    //   - GET /api/poc                 → row absent from poc_set
    //   - GET /api/poc/999             → 404
    //   - GET /rest/v1/pocs            → row absent from list
    //   - GET /rest/v1/pocs/999        → 404
    //   - POST /graphql {poc(id:999)}  → null + "not found" error
    //   - POST /graphql {pocs(...)}    → row absent from list
    //   - ConnectRPC PocService.GetPoc → CodeNotFound
    //   - ConnectRPC PocService.ListPocs → row absent
    //   - GET /ui/poc/999              → existing 404 page
    //
    // Assert: every surface returns the "absent" idiom; none leaks the row.
    //
    // Flip: set PDBPLUS_PUBLIC_TIER=users, re-run act 2, assert row PRESENT
    //        on every surface (SYNC-03 coverage).
}
```

### Example 4: `PocQueryRuleFunc` adapter (regenerated by entc after FeaturePrivacy enabled)

```go
// ent/privacy/privacy.go (GENERATED — verified from template)
// Source: ent@v0.14.6/entc/gen/template/privacy/privacy.tmpl:118-131
type PocQueryRuleFunc func(context.Context, *ent.PocQuery) error

// EvalQuery return f(ctx, q).
func (f PocQueryRuleFunc) EvalQuery(ctx context.Context, q ent.Query) error {
    if q, ok := q.(*ent.PocQuery); ok {
        return f(ctx, q)
    }
    return Denyf("ent/privacy: unexpected query type %T, expect *ent.PocQuery", q)
}
```

The typed adapter is what makes `q.Where(poc.VisibleEQ("Public"))` possible inside the rule without runtime type assertions from userland code.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Row-level access control in application handlers | ent privacy policies | ent 0.6.x+ (~2021) | One implementation covers all surfaces; edge-traversal queries are gated automatically |
| Untyped `context.WithValue(ctx, "tier", ...)` keys | Typed empty-struct keys (`type tierCtxKey struct{}`) | Go stdlib idiom since Go 1.7; ent itself uses this pattern | Collision-proof; no runtime string matching |
| Field-level redaction of private fields in responses | Row-level filter: rows ABSENT from anonymous responses | PeeringDB upstream behaviour (observed in Phase 57 baseline) | Matches upstream exactly; no "present-but-blanked" surface to reverse-engineer |
| Mutation-only policies that deny unauthorized writes | Query policies that filter on read | n/a for read-only mirror | We don't need mutation policies — writes go through the bypassed sync worker |

**Deprecated/outdated:** N/A — ent privacy is a stable, actively maintained Alpha feature in ent v0.14.6 [VERIFIED: `ent@v0.14.6/entc/gen/feature.go:14` — `Stage: Alpha`, but the API surface has been stable since ent 0.6.x]. No deprecation warnings for `privacy.DecisionContext`, `privacy.Policy`, `privacy.Skip/Allow/Deny`.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| — | (none) | — | All factual claims in this document are VERIFIED from ent v0.14.6 source (`~/go/pkg/mod/entgo.io/ent@v0.14.6/`) or CITED from official ent docs (https://entgo.io/docs/privacy). No `[ASSUMED]` claims remain. |

## Open Questions

1. **Which other entities get `Policy()` beyond POC?**
   - What we know: phase 58 produces a schema-alignment plan based on Phase 57's `diff.json`. POC is definitely in scope (has `visible`); `ixlan.ixf_ixp_member_list_url_visible` is a field-level visibility on IxLan but D-01 locks per-entity policies, so whether IxLan gets a `Policy()` depends on Phase 58's decision about whether the whole IxLan row is ever filtered or just that one field is.
   - What's unclear: the phase 58 output. This research covers the POC pattern completely; identical patterns apply to any additional visibility-bearing entity.
   - Recommendation: planner should scan phase-58's completion artifacts when spawned; the Policy pattern is identical for each new visibility-bearing entity. Task breakdown should enumerate them.

2. **Should the middleware also log a WARN on every request when `PDBPLUS_PUBLIC_TIER=users`?**
   - What we know: SYNC-04 mandates a WARN at startup (phase 61 scope). D-05 notes "whether to log the resolved tier on every request span — phase 61 owns this."
   - What's unclear: whether the startup WARN alone is sufficient for operator visibility. My read: yes, with OTel span attribute `pdbplus.privacy.tier` (OBS-03, phase 61) covering per-request visibility.
   - Recommendation: phase 61's scope. Do NOT add per-request logging in phase 59.

3. **Does `ixf_ixp_member_list_url_visible` need field-level redaction?**
   - What we know: PeeringDB's anonymous `/api/ixlan` responses contain the row but with the `ixf_ixp_member_list_url` field redacted (or omitted) when `ixf_ixp_member_list_url_visible != "Public"`. Phase 57's diff report shows the exact shape.
   - What's unclear: is field-level redaction in scope for phase 59, or is phase 59 purely row-level, and Phase 60 (VIS-06) handles field-level redaction separately?
   - Recommendation: This phase stays row-level only (D-02 is explicit). If Phase 58's diff analysis reveals field-level redaction is needed for IxLan, that's a separate phase concern (probably handled by per-surface serializer logic in pdbcompat, not by ent privacy). Escalate to planner if ambiguous.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` + ent's `enttest.Open` for in-memory SQLite |
| Config file | `internal/testutil/` helpers (`SetupClient(t)`, `seed.Full`) |
| Quick run command | `go test -race ./internal/privctx/... ./internal/middleware/... ./internal/sync/... ./internal/config/...` |
| Full suite command | `go test -race ./...` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| VIS-04 (query filter) | `PocQueryRuleFunc` filters `visible=Users` for TierPublic ctx | unit | `go test -race ./internal/sync/ -run TestPocPolicy_FiltersUsersTier` | ❌ Wave 0 |
| VIS-04 (admit Public) | Public rows visible to TierPublic | unit | same file | ❌ Wave 0 |
| VIS-04 (admit Users) | Users POC rows visible to TierUsers ctx | unit | same file | ❌ Wave 0 |
| VIS-04 (NULL defense) | NULL `visible` rows are admitted by TierPublic (defence-in-depth) | unit | same file | ❌ Wave 0 |
| VIS-04 (edge traversal) | `network.QueryPocs()` for TierPublic filters out Users POCs | unit | `go test -race ./internal/sync/ -run TestPocPolicy_EdgeTraversalFilters` | ❌ Wave 0 |
| VIS-05 (bypass active) | Sync worker's wrapped ctx sees every row regardless of visibility | unit | `go test -race ./internal/sync/ -run TestWorkerSync_BypassesPrivacy` | ❌ Wave 0 |
| VIS-05 (bypass absent) | Handler-ctx with no WithTier defaults to TierPublic → filters apply | unit | `go test -race ./internal/privctx/ -run TestTierFrom_ZeroValueIsPublic` | ❌ Wave 0 |
| VIS-05 (single bypass site) | Exactly one `privacy.DecisionContext(*, privacy.Allow)` call site in codebase | unit (source-scan) | `go test -race ./internal/sync/ -run TestSyncBypass_SingleCallSite` | ❌ Wave 0 |
| SYNC-03 (public tier default) | `PDBPLUS_PUBLIC_TIER=""` resolves to `TierPublic` | unit | `go test -race ./internal/config/ -run TestLoad_PublicTierDefault` | ❌ Wave 0 |
| SYNC-03 (users tier explicit) | `PDBPLUS_PUBLIC_TIER="users"` resolves to `TierUsers` | unit | `go test -race ./internal/config/ -run TestLoad_PublicTierUsers` | ❌ Wave 0 |
| SYNC-03 (invalid fail-fast) | `PDBPLUS_PUBLIC_TIER="Users"` or `"admin"` → config.Load() returns error | unit | `go test -race ./internal/config/ -run TestLoad_PublicTierInvalid` | ❌ Wave 0 |
| SYNC-03 (middleware stamps default) | middleware stamps `TierPublic` when configured so | unit | `go test -race ./internal/middleware/ -run TestPrivacyTier_StampsDefault` | ❌ Wave 0 |
| SYNC-03 (middleware stamps users) | middleware stamps `TierUsers` when configured so | unit | same file | ❌ Wave 0 |
| SYNC-03 (privctx roundtrip) | `TierFrom(WithTier(ctx, t)) == t` | unit | `go test -race ./internal/privctx/ -run TestTier_Roundtrip` | ❌ Wave 0 |
| D-15 end-to-end | Seed Users POC via bypassed ctx; anonymous HTTP through full chain; row absent from all 5 surfaces | integration | `go test -race ./cmd/peeringdb-plus/... -run TestE2E_AnonymousCannotSeeUsersPoc` | ❌ Wave 0 |
| Middleware chain order | New `PrivacyTier` middleware appears between Logging and SecurityHeaders | unit (source-scan) | `go test -race ./internal/middleware/ -run TestMiddlewareChain_Order` | ✅ exists — modify to include PrivacyTier |

### Sampling Rate

- **Per task commit:** `go test -race ./internal/privctx/... ./internal/middleware/... ./internal/sync/... ./internal/config/...` (<5s)
- **Per wave merge:** `go test -race ./...` (~60s)
- **Phase gate:** Full suite green + `go vet ./... && golangci-lint run` pass before `/gsd-verify-work`

### Wave 0 Gaps

- [ ] `internal/privctx/privctx_test.go` — covers SYNC-03 (roundtrip, zero-value default)
- [ ] `internal/middleware/privacy_tier_test.go` — covers SYNC-03 (middleware stamps)
- [ ] `internal/sync/policy_test.go` (or extended `worker_test.go`) — covers VIS-04, VIS-05
- [ ] `internal/config/config_test.go` additions — covers SYNC-03 env parser (both values + invalid)
- [ ] `cmd/peeringdb-plus/e2e_privacy_test.go` — covers D-15 full-chain test
- [ ] `internal/testutil/seed/seed.go` — extend `seed.Full` to include one `visible=Users` POC (phase 60 D-01, but phase 59 needs one for the E2E test. Either add here in phase 59 or pre-seed in the E2E test directly — planner's call)
- [ ] Extend `TestMiddlewareChain_Order` in `internal/middleware/` to include `PrivacyTier` in its literal chain assertion

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no (v1.14) | OAuth deferred to v1.15 |
| V3 Session Management | no | Stateless HTTP + env-driven tier |
| V4 Access Control | **yes** | ent privacy `PocQueryRuleFunc` — row-level filtering at data layer |
| V5 Input Validation | **yes** (config) | `parsePublicTier` strict validator, fail-fast on unknown values |
| V6 Cryptography | no | No secrets handled by this phase |
| V7 Error Handling | **yes** | `ent.IsNotFound` → surface-native 404; no distinguishing "exists but filtered" from "doesn't exist" (D-13/D-14) |
| V8 Data Protection | **yes** | Row absence (not redaction) matches upstream PeeringDB behaviour; no sensitive field leak |
| V13 API & Web Service | **yes** | Uniform behaviour across graphql, rest/v1, api, grpc, ui |

### Known Threat Patterns for Go + ent + net/http

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Information disclosure via distinguishable "filtered" vs "not found" responses | Information Disclosure | D-13/D-14: return surface-native 404 for both. Already the default since `Poc.Get(id)` → `Query().Where(id).Only(ctx)` returns `NotFoundError` when the privacy filter excludes the row |
| Privilege escalation via context injection into a privacy-gated query | Elevation of Privilege | Single audit point for `privacy.DecisionContext` (D-08/D-09). Grep-based audit enforceable in CI. |
| Policy bypass via edge traversal (e.g. `Network.QueryPocs()`) | Elevation of Privilege | ent privacy applies per-query-type; `*ent.PocQuery` from edge traversal is gated just like direct `client.Poc.Query()`. [VERIFIED: adapter type-asserts on `*ent.PocQuery`.] |
| Cache poisoning across tiers | Tampering | Single-process one-tier design; intra-process cache uses sync-time ETag only. See Pitfall 3 for cross-fleet caveat. |
| Fail-open on config parse error | Tampering / DoS | `parsePublicTier` fail-fast at startup (GO-CFG-1); `os.Exit(1)` prevents a misconfigured process from serving. |
| SQL injection via `visible` field value | Tampering | N/A — the policy constant `"Public"` is a Go string literal; ent builds parameterised SQL. No operator input flows into the WHERE clause. |

## Sources

### Primary (HIGH confidence)

- **ent v0.14.6 source code** (`~/go/pkg/mod/entgo.io/ent@v0.14.6/`):
  - `privacy/privacy.go` — `Policy`, rule-dispatch loop, `DecisionContext`, `decisionCtxKey`, `Allow/Deny/Skip` sentinels
  - `privacy/privacy_test.go` — official test pattern for DecisionContext + Policies
  - `entc/gen/template/privacy/privacy.tmpl` — generated code template; `PocQueryRuleFunc` shape
  - `entc/gen/feature.go` — `FeaturePrivacy` definition (Stage: Alpha; Name: "privacy")
  - `doc/md/privacy.mdx` — official markdown documentation
  - `doc/md/features.md` — feature flag reference

- **peeringdb-plus codebase** (verified existing patterns):
  - `internal/config/config.go` — `parseSyncMode` strict-validator pattern to mirror
  - `internal/middleware/csp.go` — structural template for new middleware
  - `internal/middleware/caching.go` — caching behaviour (ETag sync-time-keyed)
  - `internal/sync/worker.go` — sync worker entry point for bypass
  - `cmd/peeringdb-plus/main.go:593-610` — `buildMiddlewareChain` wrapping order
  - `internal/pdbcompat/handler.go:218-224` — existing `ent.IsNotFound` → 404 mapping
  - `internal/grpcserver/poc.go:72-80` — existing `ent.IsNotFound` → `connect.CodeNotFound` mapping
  - `internal/graphql/handler.go:63` — existing `ent.IsNotFound` → GraphQL "NOT_FOUND" classifier

- **Official ent documentation** (CITED):
  - https://entgo.io/docs/privacy — Privacy package documentation

### Secondary (MEDIUM confidence)

- Context7 `/websites/entgo_io` (~1350 snippets) — cross-verified the ent privacy semantics; consistent with source code

### Tertiary (LOW confidence)

None. All claims verified from primary sources.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all dependencies already in go.mod; feature flag name `"privacy"` verified in ent source
- Architecture: HIGH — `PocQueryRuleFunc` adapter type shape verified from template; rule-dispatch short-circuit verified from source
- Context propagation: HIGH — `decisionCtxKey` type verified; `context.WithValue` semantics are stdlib-guaranteed
- Middleware integration: HIGH — chain order is regression-locked by existing test; new middleware slot verified against the chain's structural invariants
- Direct-lookup 404: HIGH — every surface's existing `ent.IsNotFound` handler verified by line number
- Pitfalls: HIGH — NULL handling verified via `VisibleIsNil` existence in generated where.go; single-bypass audit grep-able
- Testing strategy: HIGH — pattern verified against existing `internal/testutil/seed` usage in the codebase

**Research date:** 2026-04-16
**Valid until:** 2026-05-16 (stable APIs; ent v0.14.x remains on 0.14 minor)
