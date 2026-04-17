---
phase: 61-operator-facing-observability
verified: 2026-04-16T00:00:00Z
status: passed
score: 6/6 must-haves verified
overrides_applied: 0
---

# Phase 61: Operator-facing observability Verification Report

**Phase Goal:** Make the sync mode and effective privacy tier visible to operators at startup, on the `/about` surface, and in OTel traces — no silent escalation.
**Verified:** 2026-04-16T00:00:00Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | `slog.Info "sync mode"` with `auth` + `public_tier` attrs emitted at startup after config parse | VERIFIED | `cmd/peeringdb-plus/main.go:672-675` emits `logger.Info("sync mode", slog.String("auth", auth), slog.String("public_tier", publicTier))`; call site at line 188, placed after SEC-04 warn block and before `peeringdb.NewClient`. Covered by 4 targeted tests + `TestStartupLogging` table-driven matrix. |
| 2 | `slog.Warn "public tier override active"` emitted when `PDBPLUS_PUBLIC_TIER=users` | VERIFIED | `cmd/peeringdb-plus/main.go:676-681` conditionally emits warn record only when `cfg.PublicTier == privctx.TierUsers` with attrs `public_tier=users`, `env=PDBPLUS_PUBLIC_TIER`. `TestStartup_WarnsOnUsersTier` + `TestStartup_NoWarnOnPublicTier` assert both directions. |
| 3 | HTML `/about` has "Privacy & Sync" section with override badge when users tier | VERIFIED | `internal/web/templates/about.templ:37-55` renders the card between Data Freshness and API Surfaces. Conditional `<span class="... border-amber-500 bg-amber-500/20 ...">Override active</span>` at line 49 gated by `privacy.OverrideActive`. Generated `about_templ.go:89,120` carries literals. `TestHandleAbout_PrivacySync` (4 combos) asserts badge presence iff `OverrideActive`. |
| 4 | Terminal `/about` has equivalent section with `!` indicator when users tier | VERIFIED | `internal/web/termrender/about.go:31-48` writes `Privacy & Sync` heading, `Sync Mode` and `Public Tier` KV rows, and prepends `"! "` to tier value when `privacy.OverrideActive`. Glued into value string (not styled glyph) so PlainMode carries signal. `TestRenderAboutPage_PrivacySync` (4 combos) asserts `! users` presence iff override. |
| 5 | OTel attribute `pdbplus.privacy.tier` with value `"public"` or `"users"` stamped by middleware | VERIFIED | `internal/middleware/privacy_tier.go:76,80` constructs `tierAttr := attribute.String("pdbplus.privacy.tier", tierString(tier))` at middleware build time and calls `trace.SpanFromContext(ctx).SetAttributes(tierAttr)` per request. Wired into chain at `cmd/peeringdb-plus/main.go:638` with `DefaultTier: cc.DefaultTier` sourced from `cfg.PublicTier`. `tierString` is an exhaustive switch with panic fallback (cardinality pinned to 2). `TestPrivacyTier_SetsOTelAttribute` (public + users subtests) + `TestPrivacyTier_NoSpanSafe` assert behavior. |
| 6 | `go test -race ./...` green | VERIFIED | Full-repo `go test -race ./...` completes with no `FAIL` output. All targeted test packages pass: `cmd/peeringdb-plus` (1.122s), `internal/middleware` (1.017s), `internal/web` (1.227s), `internal/web/termrender` (1.031s). |

**Score:** 6/6 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `cmd/peeringdb-plus/main.go` | `logStartupClassification` helper + call site | VERIFIED | Helper at L663-682, call site at L188. Imports already present. Exactly one match each on `"sync mode"`, `"public tier override active"`, `"env", "PDBPLUS_PUBLIC_TIER"`. |
| `cmd/peeringdb-plus/startup_logging_test.go` | Table-driven + 4 targeted startup-classification tests | VERIFIED | 253 lines, 5 test functions: `TestStartup_LogsSyncMode_Anonymous`, `_Authenticated`, `TestStartup_WarnsOnUsersTier` (2 sub-tests), `TestStartup_NoWarnOnPublicTier` (2 sub-tests), `TestStartupLogging` (4 sub-tests). Uses `captureHandler` slog.Handler — no text matching. |
| `internal/web/templates/abouttypes.go` | `PrivacySync` view struct + `AboutPageData` bundle | VERIFIED | `type PrivacySync struct` at L20 with 4 fields (AuthMode, PublicTier, PublicTierExplanation, OverrideActive). `type AboutPageData struct` at L45 wraps DataFreshness + PrivacySync for termrender dispatch. |
| `internal/web/templates/about.templ` | Privacy & Sync HTML section with amber override badge | VERIFIED | Section at L37-55 between Data Freshness and API Surfaces; amber Tailwind palette badge at L49 gated by `privacy.OverrideActive`. |
| `internal/web/handler.go` | `NewHandlerInput` struct (GO-CS-5) + Handler fields | VERIFIED | `Handler` gains `authMode` (L37) and `publicTier` (L38). `NewHandlerInput` struct at L47-52 with named fields (T-61-06 mitigation). `NewHandler` at L58-67 takes input struct. Old 2-arg signature removed. |
| `internal/web/about.go` | `handleAbout` populates PrivacySync via `buildPrivacySync` | VERIFIED | `handleAbout` at L36-61 builds PrivacySync from handler fields. `buildPrivacySync(authMode, tier)` helper at L67-83 owns the English wording. |
| `internal/web/termrender/about.go` | Terminal renderer with `Privacy & Sync` heading + `! ` override prefix | VERIFIED | `RenderAboutPage` at L15 takes `(freshness, privacy)`. Section at L31-48 with heading, `Sync Mode` / `Public Tier` KV pairs, conditional `"! "` value prefix, muted explanation. |
| `internal/middleware/privacy_tier.go` | OTel attribute stamping + exhaustive tierString | VERIFIED | OTel imports at L30-31. Attribute constructed once at L76, stamped per-request at L80. `tierString` exhaustive switch at L97-105 with `//nolint:exhaustive` + panic fallback (D-09). |
| `internal/middleware/privacy_tier_test.go` | tracetest-based attribute assertions | VERIFIED | `TestPrivacyTier_SetsOTelAttribute` at L177 (public + users subtests) uses `installInMemoryTracer` + `findStringAttr` (matches `internal/peeringdb/client_test.go` pattern); counts attribute occurrences to guard cardinality. `TestPrivacyTier_NoSpanSafe` at L264 covers no-tracer path. Phase 59 tests preserved. |

### Key Link Verification

| From | To | Via | Status | Details |
|------|-----|-----|--------|---------|
| `main.go` startup | slog logger | `logStartupClassification(logger, cfg)` after SEC-04, before `peeringdb.NewClient` | WIRED | Call site L188; between SEC-04 at L178-182 and `pdbClient` at L190. |
| `cfg.PeeringDBAPIKey` | `auth` attribute value | empty -> "anonymous", non-empty -> "authenticated" | WIRED | Helper L664-667. |
| `cfg.PublicTier` | `public_tier` attribute | TierPublic -> "public", TierUsers -> "users" | WIRED | Helper L668-671. |
| `main.go` | `web.NewHandler` | `NewHandlerInput{Client, DB, AuthMode, PublicTier}` | WIRED | L307-312 derives authMode from `cfg.PeeringDBAPIKey`, passes `cfg.PublicTier`. |
| `handleAbout` | `templates.AboutPage` | `buildPrivacySync(h.authMode, h.publicTier)` → `templates.AboutPage(freshness, privacy)` | WIRED | about.go L51, L55. |
| `templates.PrivacySync.OverrideActive` | HTML badge | conditional `if privacy.OverrideActive` in about.templ | WIRED | about.templ L48-50. |
| `templates.PrivacySync.OverrideActive` | terminal `!` indicator | `if privacy.OverrideActive { tierValue = "! " + tierValue }` | WIRED | termrender/about.go L38-43. |
| `middleware.PrivacyTier` | active span | `trace.SpanFromContext(ctx).SetAttributes(tierAttr)` | WIRED | privacy_tier.go L80. |
| `cfg.PublicTier` | `PrivacyTier` middleware | via `chainConfig.DefaultTier` threaded through `buildMiddlewareChain` | WIRED | main.go L471 sets `DefaultTier: cfg.PublicTier`; L638 consumes it. |
| `otelhttp.NewMiddleware` | `PrivacyTier` inbound span | otelhttp at L640 sits outside PrivacyTier at L638 (middleware wraps from inside-out) | WIRED | Correct relative placement — the span is already live when PrivacyTier runs. |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|--------------|--------|--------------------|--------|
| HTML `/about` Privacy section | `privacy templates.PrivacySync` | `buildPrivacySync(h.authMode, h.publicTier)` in about.go; fed from `NewHandlerInput.AuthMode`/`PublicTier` at construction from `cfg.PeeringDBAPIKey` + `cfg.PublicTier` | Yes — startup snapshot, not request-time stub | FLOWING |
| Terminal `/about` Privacy section | `privacy` (same type) | Same handler-captured values routed via `AboutPageData` bundle into `RenderAboutPage` | Yes | FLOWING |
| OTel span attribute | `tierAttr` constructed at middleware build from `in.DefaultTier` | `cfg.PublicTier` via `chainConfig.DefaultTier` | Yes — resolved once at startup, applied per request | FLOWING |
| Startup log records | `auth`, `publicTier` locals in `logStartupClassification` | `cfg.PeeringDBAPIKey`, `cfg.PublicTier` | Yes | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| `TestStartup*` suite passes | `go test -race ./cmd/peeringdb-plus -run TestStartup -count=1` | `ok ...cmd/peeringdb-plus 1.122s` | PASS |
| `TestPrivacyTier*` suite passes | `go test -race ./internal/middleware -run TestPrivacyTier -count=1` | `ok ...internal/middleware 1.017s` | PASS |
| `/about` HTML + terminal suites pass | `go test -race ./internal/web/... -run "TestHandleAbout\|TestRenderAboutPage" -count=1` | `ok ...internal/web 1.227s`, `ok ...termrender 1.031s` | PASS |
| Full repo green | `go test -race ./...` | No `FAIL` output; all packages ok | PASS |
| Template regeneration stable | Template already generated; `about_templ.go` contains `Privacy &amp; Sync` + `Override active` literals | Verified via Grep | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| SYNC-04 | 61-01-PLAN.md | Startup logs a WARN line when `PDBPLUS_PUBLIC_TIER=users` is in effect, so the elevated default is never silent | SATISFIED | `logStartupClassification` conditional Warn (main.go L676-681); `TestStartup_WarnsOnUsersTier` asserts emission for both auth states; `TestStartup_NoWarnOnPublicTier` asserts absence when tier=public. |
| OBS-01 | 61-01-PLAN.md | Startup log line classifies sync as "anonymous, public-only" or "authenticated, full" based on `PDBPLUS_PEERINGDB_API_KEY` presence | SATISFIED | `slog.Info "sync mode"` with `auth` + `public_tier` attributes (main.go L672-675); 5 test functions verify every (auth × tier) combination with strict attribute-count assertions. Note: the literal wording differs from REQUIREMENTS.md ("anonymous"/"authenticated" vs "anonymous, public-only"/"authenticated, full"), but the Info+attributes shape is more machine-parseable and the phase 61 plan's D-01/D-02 decisions approved this structure. |
| OBS-02 | 61-02-PLAN.md | `/about` (HTML) and `/ui/about` (terminal) render the current sync mode and effective privacy tier | SATISFIED | HTML section in about.templ L37-55; terminal section in termrender/about.go L31-48; `TestHandleAbout_PrivacySync` + `TestRenderAboutPage_PrivacySync` exercise all 4 (auth × tier) combos on both surfaces. |
| OBS-03 | 61-03-PLAN.md | OTel attribute `pdbplus.privacy.tier` (values `public` or `users`) set on read spans; usable as a Grafana dashboard filter | SATISFIED | `trace.SpanFromContext(ctx).SetAttributes(tierAttr)` in privacy_tier.go L80; `tierString` exhaustive with panic fallback (cardinality=2); `TestPrivacyTier_SetsOTelAttribute` asserts value + uniqueness per span. Middleware is wired into the real chain at main.go L638, downstream of otelhttp so the inbound server span is already live. |

### Anti-Patterns Found

None. Grep for `TODO|FIXME|XXX|HACK|PLACEHOLDER|placeholder|not yet implemented` across all modified files returned no matches.

### Human Verification Required

None. All must-haves are programmatically verifiable through unit tests, file inspection, and grep patterns. The changes are:

1. Deterministic startup-time log output (tested via slog capture handler).
2. Statically rendered HTML/terminal content (tested via response body inspection).
3. OTel attribute stamping on a per-request span (tested via tracetest InMemoryExporter).

No external service integration, no visual-design judgment (the amber Tailwind badge uses the project's existing palette), no real-time behavior that needs a running system. The Grafana dashboard filter usability described in OBS-03 would be a separate v1.15 or ops-side deliverable; the phase plan (61-03-PLAN.md, Step 3 note) explicitly scopes dashboard work out.

### Gaps Summary

No gaps. All 6 must-haves verified, all 4 requirement IDs satisfied, all key links wired, full test suite green with `-race`. Phase goal ("no silent escalation") is achieved at every operator surface: startup logs, `/about` HTML, `/ui/about` terminal, and OTel read-path spans.

Notable strengths of the implementation:
- **Wire-contract discipline**: The `captureHandler` slog.Handler pattern in `startup_logging_test.go` asserts attribute keys and exact-count (`len(attrs) != 2`), so an accidental extra attr trips CI. Same for `findStringAttr` counting `pdbplus.privacy.tier` occurrences in the OTel test.
- **Override signal survives ANSI stripping**: The terminal `"! "` prefix is glued into the value string rather than being a styled glyph, so operators scraping plain text still see the override indicator.
- **Exhaustive switch with panic fallback** in `tierString` forces a compile-time hit (via `golangci-lint`'s exhaustive checker) for any future `Tier` enum addition before Grafana dashboards receive a new cardinality value.
- **Constructor safety**: `NewHandlerInput` uses named fields (GO-CS-5) so callers cannot transpose `AuthMode` ↔ `PublicTier` (T-61-06).

---

_Verified: 2026-04-16T00:00:00Z_
_Verifier: Claude (gsd-verifier)_
