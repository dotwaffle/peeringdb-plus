---
phase: 61-operator-facing-observability
plan: 02
subsystem: internal/web
tags:
  - observability
  - about
  - ui
  - terminal
  - OBS-02
requires:
  - privctx.Tier (phase 59)
  - config.Config.PublicTier (phase 59)
  - config.Config.PeeringDBAPIKey
provides:
  - templates.PrivacySync struct
  - templates.AboutPageData bundle
  - web.NewHandlerInput struct (GO-CS-5 constructor)
  - web.buildPrivacySync helper
  - HTML "Privacy & Sync" card on /ui/about
  - Terminal "Privacy & Sync" section on /ui/about
  - TestHandleAbout_PrivacySync (4 combos)
  - TestRenderAboutPage_PrivacySync (4 combos)
affects:
  - internal/web/templates/abouttypes.go
  - internal/web/templates/about.templ
  - internal/web/templates/about_templ.go (generated)
  - internal/web/handler.go
  - internal/web/about.go
  - internal/web/termrender/about.go
  - internal/web/termrender/dispatch.go
  - internal/web/termrender/about_test.go
  - internal/web/handler_test.go
  - internal/web/detail_test.go
  - internal/web/completions_test.go
  - cmd/peeringdb-plus/main.go
  - cmd/peeringdb-plus/e2e_privacy_test.go
  - cmd/peeringdb-plus/privacy_surfaces_test.go
tech-stack:
  added: []
  patterns:
    - GO-CS-5 NewHandlerInput struct (>2 non-ctx args)
    - PrivacySync value type mirroring DataFreshness shape
    - AboutPageData bundle for termrender dispatch (single concrete type per page)
    - Render-time value built at startup and stashed on *Handler — no request-time env reads
key-files:
  created: []
  modified:
    - internal/web/templates/abouttypes.go
    - internal/web/templates/about.templ
    - internal/web/templates/about_templ.go
    - internal/web/handler.go
    - internal/web/about.go
    - internal/web/termrender/about.go
    - internal/web/termrender/dispatch.go
    - internal/web/termrender/about_test.go
    - internal/web/handler_test.go
    - internal/web/detail_test.go
    - internal/web/completions_test.go
    - cmd/peeringdb-plus/main.go
    - cmd/peeringdb-plus/e2e_privacy_test.go
    - cmd/peeringdb-plus/privacy_surfaces_test.go
decisions:
  - D-04 (61-CONTEXT): Privacy & Sync section placed AFTER Data Freshness on both HTML and terminal
  - D-05 (61-CONTEXT): Section exposes Sync mode + Public tier + one-line explanation
  - D-06 (61-CONTEXT): amber "Override active" badge in HTML / "! " prefix on terminal Public Tier line when tier=users
  - D-12 (61-CONTEXT): render-layer tests per (mode, tier) combo — 4 subtests in each renderer test
  - Claude (D-05 discretion): public explanation = "Anonymous callers see Public-only data (default)."
  - Claude (D-05 discretion): users explanation = "Anonymous callers see Users-tier data (internal/private deployment — override active)."
  - Claude (D-06 discretion): terminal override prefix is "! " (plain ASCII) — part of the value string, not a styled glyph, so PlainMode / ANSI-stripped output still carries the signal
  - GO-CS-5: NewHandler switched to NewHandlerInput struct (client + db + authMode + publicTier)
  - T-61-06 mitigation: named fields on NewHandlerInput prevent transposing AuthMode ↔ PublicTier at the call site
  - AboutPageData bundle introduced so the termrender dispatch registry can carry both freshness and privacy through a single reflect.Type lookup without changing the generic dispatch machinery
metrics:
  duration: ~25m
  completed: 2026-04-16
---

# Phase 61 Plan 02: Privacy & Sync Section on /about — Summary

A new "Privacy & Sync" section on `/ui/about` — rendered on both the HTML and terminal (lipgloss) surfaces — names the active sync auth mode and the effective public tier, and visually flags the `PDBPLUS_PUBLIC_TIER=users` override. Requirement **OBS-02** lands here; the sibling work in 61-01 (startup log) and 61-03 (OTel span attribute) makes the tier/auth classification visible at startup and per-request, and this plan closes the loop by making it visible to any human visiting `/about` or curling `/ui/about`.

## What Landed

### New view types (`internal/web/templates/abouttypes.go`)

Two siblings to the existing `DataFreshness`:

```go
// PrivacySync carries the Phase 61 OBS-02 Privacy & Sync section payload
// for both the HTML (about.templ) and terminal (termrender/about.go)
// About page renderings.
type PrivacySync struct {
    AuthMode              string // "Authenticated with PeeringDB API key" | "Anonymous (no key)"
    PublicTier            string // "public" | "users" — lowercase, matches env-var value
    PublicTierExplanation string // one-line plain-language gloss per tier
    OverrideActive        bool   // true iff PublicTier == "users"
}

// AboutPageData bundles the two payloads consumed by the terminal About
// renderer. The dispatch table registers a single concrete type per page;
// this struct is that type for /ui/about post-Phase-61.
type AboutPageData struct {
    Freshness DataFreshness
    Privacy   PrivacySync
}
```

`AboutPageData` solves the termrender dispatch quirk: `internal/web/termrender/dispatch.go` uses `reflect.Type` to pick a renderer, so a second argument on `RenderAboutPage` had to be bundled into a single concrete value before reaching the dispatch. No generic changes — one entry in the registry now points at the bundle type.

### Handler constructor upgrade (`internal/web/handler.go`)

Old: `NewHandler(client *ent.Client, db *sql.DB) *Handler` — 2 positional args.
New: `NewHandler(in NewHandlerInput) *Handler` with a `NewHandlerInput` struct (GO-CS-5 compliance after growing to 4 values). `Handler` now carries two additional unexported fields:

```go
authMode   string       // "authenticated" | "anonymous"
publicTier privctx.Tier // TierPublic | TierUsers
```

Both are captured at construction from `cmd/peeringdb-plus/main.go` (production) or from each test's chosen fixture values. No env var or config lookup happens at request time — matches the "diagnostic snapshot" semantics of the rest of the `/about` page.

### HTML section (`internal/web/templates/about.templ`, line 38 after regen)

Inserted between the existing Data Freshness `<div>` (previously at line 35) and the `<h2>API Surfaces</h2>` heading:

```html
<div class="mt-6 p-6 bg-neutral-100 dark:bg-neutral-800 rounded-lg border border-neutral-200 dark:border-neutral-700">
    <h2 ...>Privacy &amp; Sync</h2>
    <dl>
        <div><dt>Sync mode:</dt><dd>{ privacy.AuthMode }</dd></div>
        <div>
            <dt>Public tier:</dt>
            <dd>
                <span>{ privacy.PublicTier }</span>
                if privacy.OverrideActive {
                    <span class="... border-amber-500 bg-amber-500/20 text-amber-700 dark:text-amber-400">Override active</span>
                }
            </dd>
        </div>
    </dl>
    <p>{ privacy.PublicTierExplanation }</p>
</div>
```

The override badge uses the Tailwind amber-500 family (matches the existing ColorWarning lipgloss palette / the 400G port-speed accent used in search results for visual consistency). The wording "Override active" is short enough to fit alongside the tier value on mobile without line-wrap regressions.

### Terminal section (`internal/web/termrender/about.go`)

```
Description        Read-only PeeringDB mirror with GraphQL, gRPC, and REST APIs
          Last Sync  2026-04-16 00:00:00 UTC
           Data Age  1m0s

Privacy & Sync
          Sync Mode  Anonymous (no key)
        Public Tier  ! users
Anonymous callers see Users-tier data (internal/private deployment — override active).

API Endpoints
             Web UI  /ui/
            GraphQL  /graphql
               REST  /rest/v1/
      PeeringDB API  /api/
         ConnectRPC  /peeringdb.v1.*/
```

The `! ` prefix on the Public Tier value is the D-06 override indicator. It is glued onto the value string (not applied as a styled glyph) so it survives `ANSI stripping` and `PlainMode` rendering — attacker-controlled log processors cannot silently drop the flag without also destroying the value.

### main.go wire-up

```go
authMode := "anonymous"
if cfg.PeeringDBAPIKey != "" {
    authMode = "authenticated"
}
webHandler := web.NewHandler(web.NewHandlerInput{
    Client:     entClient,
    DB:         db,
    AuthMode:   authMode,
    PublicTier: cfg.PublicTier,
})
```

Two additional production call sites (`cmd/peeringdb-plus/e2e_privacy_test.go`, `cmd/peeringdb-plus/privacy_surfaces_test.go`) migrated to the struct constructor with safe defaults (authMode="" → falls back to anonymous label; PublicTier zero value → TierPublic). Per plan note, the tier plumbing in those tests comes from `chainConfig` on the request path; the handler field is cosmetic-only there and so neutral defaults are fine.

## Chosen Wording (D-05 Claude's discretion)

| PublicTier | Explanation |
|---|---|
| `public` | `Anonymous callers see Public-only data (default).` |
| `users`  | `Anonymous callers see Users-tier data (internal/private deployment — override active).` |

The `override active` phrase is reused across the HTML badge ("Override active") and the terminal explanation. This gives the D-12 test an additional assertion hook — a substring search for `override active` in the users-tier explanation fails immediately if the wording drifts.

## Tests

Added two table-driven tests, each with 4 subtests:

- `TestHandleAbout_PrivacySync` (internal/web/handler_test.go) — HTML surface. Exercises each `(authMode, tier)` pair through the `mux.ServeHTTP` path with `User-Agent: Mozilla/5.0` + `Accept: text/html` to force HTML negotiation. Asserts section heading, auth text, tier text, and presence/absence of the "Override active" badge.
- `TestRenderAboutPage_PrivacySync` (internal/web/termrender/about_test.go) — terminal surface. Same matrix. Asserts heading, auth text, tier text, explanation fragment, and presence/absence of the `! users` override prefix.

The three pre-existing `TestRenderAboutPage_*` tests were updated to pass a neutral `templates.PrivacySync` (anon/public) alongside the freshness argument. Their assertions were not changed — they remain valid coverage for the non-privacy sections.

The threat T-61-05 (badge suppression / tampering) is covered by the HTML test's `wantBadge` conditional: a refactor that unconditionally drops the amber badge makes `anon_users` and `auth_users` subtests fail. T-61-06 (constructor transposition) is covered structurally by the named-field `NewHandlerInput` struct — no test needed because the Go type system catches it.

## Deviations from Plan

### Bundling into `AboutPageData` (Rule 3 - blocking, no user permission needed)

The plan said "pass `privacySync` alongside `freshness`" to the terminal renderer. The termrender dispatch machinery (`dispatch.go`) keys on `reflect.Type`, so the single-argument dispatch path can carry at most one concrete type. A second argument on `RenderAboutPage` was fine for direct callers but the `Register(func(d templates.DataFreshness, ...))` call had to become something. I introduced `templates.AboutPageData` to wrap both payloads and registered that instead. This stays within D-04/D-05/D-06 — it's an implementation detail that keeps the generic dispatch machinery untouched.

### Testing defaults on cmd/peeringdb-plus tests (Rule 2 - correctness)

`e2e_privacy_test.go` and `privacy_surfaces_test.go` were migrated to `NewHandlerInput` but with zero-value `AuthMode` and `PublicTier` (per the plan's note that the handler fields are cosmetic-only in those tests — tier enforcement comes from the privacy middleware, not the handler). Explicit defaults would add noise without adding coverage.

No other deviations.

## Production Call Sites for `web.NewHandler`

Three call sites exist. All were updated as part of this plan:

1. `cmd/peeringdb-plus/main.go:300` — production; reads from `cfg`
2. `cmd/peeringdb-plus/e2e_privacy_test.go:208` — test; neutral defaults
3. `cmd/peeringdb-plus/privacy_surfaces_test.go:148` — test; neutral defaults

Plus the test helpers in `internal/web/*_test.go` (~9 sites) which use `NewHandlerInput{Client: client}` for the simple mux construction pattern.

## Verification Evidence

```
$ go generate ./internal/web/templates   # no drift on re-run
$ go build ./...                         # passes
$ go vet ./...                           # passes
$ go test -race ./internal/web/... ./cmd/peeringdb-plus/... -count=1
ok  github.com/dotwaffle/peeringdb-plus/internal/web               7.186s
ok  github.com/dotwaffle/peeringdb-plus/internal/web/templates     1.029s
ok  github.com/dotwaffle/peeringdb-plus/internal/web/termrender    1.195s
ok  github.com/dotwaffle/peeringdb-plus/cmd/peeringdb-plus         2.755s
$ golangci-lint run ./internal/web/... ./cmd/peeringdb-plus/...
0 issues.
```

Full repo `go test -race ./...` also passes with no regressions.

## Commits

- `77aeb89` test(61-02): add failing tests for Privacy & Sync section on /about
- `435bf36` feat(61-02): render Privacy & Sync section on HTML and terminal /about (OBS-02)

## Self-Check: PASSED

All created/modified files exist:
- internal/web/templates/abouttypes.go (modified) — FOUND
- internal/web/templates/about.templ (modified) — FOUND
- internal/web/templates/about_templ.go (regenerated) — FOUND
- internal/web/handler.go (modified) — FOUND
- internal/web/about.go (modified) — FOUND
- internal/web/termrender/about.go (modified) — FOUND
- internal/web/termrender/dispatch.go (modified) — FOUND
- internal/web/termrender/about_test.go (modified) — FOUND
- internal/web/handler_test.go (modified) — FOUND
- internal/web/detail_test.go (modified) — FOUND
- internal/web/completions_test.go (modified) — FOUND
- cmd/peeringdb-plus/main.go (modified) — FOUND
- cmd/peeringdb-plus/e2e_privacy_test.go (modified) — FOUND
- cmd/peeringdb-plus/privacy_surfaces_test.go (modified) — FOUND

Commits verified in `git log`:
- 77aeb89 — FOUND (test RED)
- 435bf36 — FOUND (feat GREEN)

## TDD Gate Compliance

- RED commit `77aeb89` (test) — present
- GREEN commit `435bf36` (feat) — present, after RED
- No REFACTOR commit needed (implementation is already at the clean shape; no duplication or lingering scaffolding).
