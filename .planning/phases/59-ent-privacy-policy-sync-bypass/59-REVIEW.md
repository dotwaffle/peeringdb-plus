---
phase: 59-ent-privacy-policy-sync-bypass
reviewed: 2026-04-16T00:00:00Z
depth: standard
files_reviewed: 11
files_reviewed_list:
  - internal/privctx/privctx.go
  - internal/config/config.go
  - internal/middleware/privacy_tier.go
  - internal/middleware/privacy_tier_test.go
  - ent/entc.go
  - ent/schema/poc.go
  - ent/schema/types.go
  - ent/schematypes/schematypes.go
  - internal/sync/worker.go
  - internal/sync/bypass_audit_test.go
  - cmd/peeringdb-plus/main.go
  - cmd/peeringdb-plus/e2e_privacy_test.go
  - cmd/peeringdb-plus/middleware_chain_test.go
findings:
  critical: 0
  warning: 1
  info: 4
  total: 5
status: issues_found
---

# Phase 59: Code Review Report

**Reviewed:** 2026-04-16T00:00:00Z
**Depth:** standard
**Files Reviewed:** 13 (source + tests)
**Status:** issues_found

## Summary

Phase 59 establishes the read-path privacy floor with a tight, well-documented
design. The bypass is correctly scoped to a single call site in
`internal/sync/worker.go:260`, the HTTP tier stamp is placed between `Logging`
and `Readiness` in the middleware chain (so the ctx is set before any cached
response path executes), and all 5 API surfaces return native not-found idioms
(404 / `CodeNotFound` / empty GraphQL edges / absent HTML) rather than a
distinguishable 403 — closing the D-14 existence-leak channel. The ent Policy's
`Or(poc.VisibleEQ("Public"), poc.VisibleIsNil())` predicate is SQL-correct
for SQLite's three-valued logic.

The security-critical properties hold:

- **Bypass propagation** — `Sync()` reassigns `ctx` via `context.WithValue`
  locally, so the bypass is scoped to the `Sync` call stack. No goroutine (incl.
  the demotion monitor in `runSyncCycle`) inherits a bypass'd ctx after `Sync`
  returns. The `sync_status` / `sync_cursor` bookkeeping runs on raw `*sql.DB`
  and never interacts with the Policy.
- **NULL-defence** — The `VisibleIsNil()` branch is correct for SQLite; the
  schema's `Default("Public")` on inserts backstops post-migration rows.
- **Middleware position** — `PrivacyTier` is wrapped after `Logging` and before
  `Readiness` in the code (innermost-first), so in request flow the tier is
  stamped BEFORE `Caching` runs. The 304 short-circuit bypasses the inner
  handler, but since no ent query runs on a 304, the tier is irrelevant there.
  `TestMiddlewareChain_Order` locks the order.
- **Single audit point** — `TestSyncBypass_SingleCallSite` correctly catches
  the canonical `privacy.DecisionContext(..., privacy.Allow)` shape, including
  gofmt-split multi-line calls, and strips comments to avoid false positives
  from prose mentions.
- **Fail-safe-closed defaults** — `privctx.TierFrom` returns `TierPublic` on a
  missing or wrong-typed value; `parsePublicTier` rejects anything outside the
  exact `"public"` / `"users"` lowercase set (no `strings.ToLower` fall-open);
  `config.Load()` failure triggers `os.Exit(1)` in `main.go`.
- **Go conventions** — GO-CS-5 (`PrivacyTierInput`), GO-CTX-1 (ctx first
  where applicable; HTTP middleware is `http.Handler`-shaped), GO-ERR-1
  (`fmt.Errorf` wraps with `%w` in `config.Load`), GO-SEC-2 (no secret
  logging; `SyncToken` never appears in a slog attribute), and GO-CFG-1
  (fail-fast on unknown env values) all hold.

The findings below are non-blocking hardening opportunities in the audit
machinery and a handful of minor code-quality notes. None affect the
correctness of the privacy floor.

## Warnings

### WR-01: Bypass audit regex cannot detect aliased `privacy.Allow`

**File:** `internal/sync/bypass_audit_test.go:40`
**Issue:** The audit regex
`(?s)privacy\.DecisionContext\([^;]*?privacy\.Allow\b` requires the literal
token `privacy.Allow` to appear INSIDE the `DecisionContext(...)` parens. A
maintainer can trivially evade the audit by aliasing the decision sentinel
to a local variable before the call — the test will still report
"exactly 1 call site" and PASS:

```go
// Evades TestSyncBypass_SingleCallSite despite adding a second bypass.
allow := privacy.Allow
ctx = privacy.DecisionContext(ctx, allow)
```

Same for:

```go
var bypass = privacy.Allow  // package-level
...
ctx = privacy.DecisionContext(ctx, bypass)
```

This is a realistic attack surface on the "single audit point" invariant
(D-09) because any future refactor that hoists `privacy.Allow` into a
`var bypassDecision = privacy.Allow` constant — a fairly normal Go pattern —
would silently break the guarantee. The audit is currently load-bearing for
the D-14 existence-leak defence; a single additional bypass in a handler
would leak every Users-tier row through that surface with no test failure.

**Fix:** Tighten the audit to also fail the build if any package-level
`= privacy.Allow` assignment appears outside `internal/sync/worker.go`, OR
(simpler) forbid all references to `privacy.Allow` in production code except
the one known line. The stricter form is a second regex over the same
scan dirs:

```go
// bypassRefRE catches both the DecisionContext call and any alias assignment.
var bypassRefRE = regexp.MustCompile(`privacy\.Allow\b`)

// Existing loop: record hits that contain privacy.Allow. Then assert
// hits come from the one allowed line in internal/sync/worker.go.
```

Additionally, extend the scan to cover `graph/` (hand-written GraphQL
resolvers — see IN-01) so a bypass inserted there is also caught. The
current `scanDirs` list is `{"internal", "cmd", "ent/schema"}` which
silently excludes `graph/resolver.go`, `graph/custom.resolvers.go`, and
`graph/pagination.go`.

## Info

### IN-01: Bypass audit scan directories miss `graph/`

**File:** `internal/sync/bypass_audit_test.go:67`
**Issue:** `scanDirs := []string{"internal", "cmd", "ent/schema"}` excludes
the `graph/` directory which contains hand-written GraphQL resolver code
(`graph/custom.resolvers.go`, `graph/resolver.go`, `graph/pagination.go`).
A bypass added to a custom resolver would evade the audit entirely. The
existing exclusion logic for "generated ent/" subtrees is correct but does
not extend to `graph/`, which is partly generated (`graph/generated.go`) and
partly hand-written. Today there is no `privacy.DecisionContext` call in
`graph/`, so no active regression — but the coverage gap is silent.

**Fix:** Add `graph/` to `scanDirs`. The generated `graph/generated.go`
is very unlikely to ever contain `privacy.Allow` (it is gqlgen output); if
false positives appear, extend the "generated" skip list to include
`graph/generated.go` explicitly rather than the whole directory:

```go
scanDirs := []string{"internal", "cmd", "ent/schema", "graph"}
// ...
// In the walk: also skip graph/generated.go specifically.
if strings.HasSuffix(path, string(os.PathSeparator)+"graph"+string(os.PathSeparator)+"generated.go") {
    return nil
}
```

### IN-02: Bypass ctx allocated before `running` CAS check

**File:** `internal/sync/worker.go:259-264`
**Issue:** `Sync` calls `privacy.DecisionContext(ctx, privacy.Allow)` on
line 260, then line 261 checks `w.running.CompareAndSwap(false, true)` and
returns early on contention. When the CAS loses, the bypass'd ctx is
allocated (one `context.WithValue` envelope) and immediately discarded.
Harmless but wasteful.

More importantly, this ordering makes the bypass stamp happen on EVERY
call, even ones that will bail out — which complicates any future audit
that wants to count "actual bypass invocations" via an OTel event or
counter. Moving the bypass inside the CAS success branch would make it
strictly semantic: the bypass happens iff we are actually going to do
the sync work.

**Fix:** Reorder so the CAS runs first and the bypass happens only on
the winning path:

```go
func (w *Worker) Sync(ctx context.Context, mode config.SyncMode) error {
    if !w.running.CompareAndSwap(false, true) {
        w.logger.Warn("sync already running, skipping")
        return nil
    }
    defer w.running.Store(false)
    ctx = privacy.DecisionContext(ctx, privacy.Allow) // VIS-05 bypass — sole call site
    // ... rest unchanged
}
```

Note: this would require updating the exact line number in
`TestSyncBypass_SingleCallSite`'s messaging if it has one hardcoded
(it does not today — the audit only counts call sites, not line numbers).
The comment on line 260 and the godoc at line 246-258 should be updated
to match the new position.

### IN-03: GraphQL/REST expose `visible` filter predicates under TierPublic

**File:** `ent/schema/poc.go:60-64`
**Issue:** The POC schema declares
`field.String("visible").Optional().Default("Public").Annotations(entrest.WithFilter(...))`
which generates `visible.eq`, `visible.neq`, `visible.in`, `visible.null`, etc.
query parameters on `/rest/v1/pocs` and corresponding `where: { visible: ... }`
inputs on GraphQL. Under TierPublic, these filters are AND-ed with the Policy
predicate (`visible = 'Public' OR visible IS NULL`), so they cannot leak
Users-tier rows — the correctness is preserved. However:

- A `visibleEQ: "Users"` filter from an anonymous caller silently returns
  an empty set, which is confusing (it looks like the column has no Users
  values, when in fact they exist and are being filtered by policy).
- The mere exposure of a `visible` filter in the public schema is a small
  information-leak about the existence of the visibility column and its
  possible values. Low severity — the column name is public via the
  OpenAPI spec anyway.

**Fix:** Consider stripping the `visible` filter from the public query
surface (via `entrest.WithFilter(0)` or similar; entgql supports
skipping fields from `WithWhereInputs`). Alternatively, accept as-is
and document the behaviour — this is a UX nit, not a security issue.
Out of scope for v1.14 if not already decided.

### IN-04: `CLAUDE.md` middleware chain documentation is stale

**File:** `CLAUDE.md:80`
**Issue:** The `### Middleware` section documents the chain as:

```
Recovery -> CORS -> OTel HTTP -> Logging -> Readiness -> CSP -> Caching -> Gzip -> mux
```

The actual chain after Phase 59 (see `cmd/peeringdb-plus/main.go:428-436`,
confirmed by `TestMiddlewareChain_Order`) is:

```
Recovery -> MaxBytesBody -> CORS -> OTel HTTP -> Logging -> PrivacyTier ->
Readiness -> SecurityHeaders -> CSP -> Caching -> Gzip -> mux
```

Drift from Phase 59 (`PrivacyTier`), Phase 52 (`SecurityHeaders`), and
an earlier phase (`MaxBytesBody`). This is not a bug — the canonical
order is locked by `TestMiddlewareChain_Order` — but the doc is misleading
for contributors reading `CLAUDE.md`. Per the project's CLAUDE.md
management rules, this file is updated via the
`/claude-md-management:revise-claude-md` workflow, so this is a backlog
item for a later doc-sync pass, not a Phase 59 blocker.

**Fix:** Queue a `CLAUDE.md` revision to include `PrivacyTier`,
`SecurityHeaders`, and `MaxBytesBody` in the middleware chain line.

---

_Reviewed: 2026-04-16T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
