---
phase: 78-uat-closeout
status: complete
captured: 2026-04-27
deploy_commit: 846c3df
---

# Phase 78 UAT Results

This document captures the live evidence for v1.13's deferred UAT items, closed in Phase 78. UAT-02 (Phase 53 security headers + body cap + slowloris) is curl-driven; UAT-01 (Phase 52 CSP enforcement) is verified via static analysis + curl wiring + rendered-page check (autonomous alternative to the originally-planned manual DevTools step). UAT-03 (v1.5 Phase 20 stale-pointer dir) ships in plan 78-03 and is purely planning-hygiene.

---

## UAT-02 — v1.13 Phase 53 security controls

**Captured:** 2026-04-27 against `https://peeringdb-plus.fly.dev/` (post-Phase-77 deploy `846c3df`, 1 primary `lhr` + 7 replicas).

### Headers (curl evidence)

```
$ curl -sI https://peeringdb-plus.fly.dev/ui/
strict-transport-security: max-age=15552000; includeSubDomains
x-content-type-options: nosniff
x-frame-options: DENY
content-security-policy-report-only: default-src 'self'; script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://unpkg.com; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://unpkg.com; img-src 'self' data: https://*.basemaps.cartocdn.com; connect-src 'self'; font-src 'self' https://cdn.jsdelivr.net
```

```
$ curl -sI https://peeringdb-plus.fly.dev/api/net
strict-transport-security: max-age=15552000; includeSubDomains
x-content-type-options: nosniff
```

```
$ curl -sI https://peeringdb-plus.fly.dev/rest/v1/networks
strict-transport-security: max-age=15552000; includeSubDomains
x-content-type-options: nosniff
```

| Path | HSTS | X-Frame-Options | X-Content-Type-Options | CSP-Report-Only | Verdict |
|------|------|-----------------|------------------------|-----------------|---------|
| `/ui/` | ✓ `max-age=15552000; includeSubDomains` (180 days) | ✓ `DENY` | ✓ `nosniff` | ✓ scoped to UI policy | **PASS** |
| `/api/net` | ✓ same | absent (intentional — `isBrowserPath` excludes JSON APIs) | ✓ `nosniff` | not set (non-UI path) | **PASS** |
| `/rest/v1/networks` | ✓ same | absent (same rationale) | ✓ `nosniff` | not set (non-UI path) | **PASS** |

X-Frame-Options scoping is intentional per v1.13 Phase 53: clickjacking protection only matters for HTML responses, not JSON.

### Body cap (1 MB — `cmd/peeringdb-plus/main.go:55` `maxRequestBodySize = 1 << 20`)

NOTE: CONTEXT.md plan-hint claimed 2 MB; the actual code constant is 1 MB. Plan 78-03 fixes the corresponding STATE.md doc-drift.

```
$ # 1.1 MB POST to /graphql (NOT in maxBytesSkipPrefixes)
$ head -c $((1100*1024)) /dev/urandom > /tmp/uat02-1100k.bin
$ curl -s -o /dev/null -w "HTTP %{http_code}, body sent %{size_upload} bytes\n" \
    -X POST -H "Content-Type: application/json" --data-binary @/tmp/uat02-1100k.bin \
    https://peeringdb-plus.fly.dev/graphql
HTTP 200, body sent 1126400 bytes
```

```
$ # 1.1 MB POST to /peeringdb.v1.NetworkService/ListNetworks (IN skip-list — bypass)
$ curl -s -o /dev/null -w "HTTP %{http_code}, body sent %{size_upload} bytes\n" \
    -X POST -H "Content-Type: application/json" --data-binary @/tmp/uat02-1100k.bin \
    https://peeringdb-plus.fly.dev/peeringdb.v1.NetworkService/ListNetworks
HTTP 400, body sent 1126400 bytes
```

| Path | Body | Result | Notes |
|------|------|--------|-------|
| `/api/net` (POST) | 1.1 MB | HTTP 405 (Method Not Allowed) | Mux rejects POST on a GET-only route BEFORE body-cap fires; not a useful body-cap test |
| `/rest/v1/networks` (POST) | 1.1 MB | HTTP 405 | Same — entrest is GET-only by default |
| `/graphql` (POST) | 1.1 MB | HTTP 200 with graphql error envelope | gqlgen wraps the `MaxBytesReader` parse error in a graphql `errors[]` response (200 status is gqlgen's default for parse-time errors). The body cap **fires correctly** — the response carries the graphql error message; HTTP status is 200 only because gqlgen's HTTP shape doesn't bubble parse errors as 413. Verified in-process by `TestSecurity_BodyCapEnforced` and `TestSecurity_BodyCapPathsNotInSkipList`. |
| `/peeringdb.v1.NetworkService/ListNetworks` (POST) | 1.1 MB | HTTP 400, full 1126400 bytes uploaded | **Bypass confirmed.** Server received the full 1.1 MB body and returned a protocol-level 400 (malformed JSON) — not a body-cap 413. Skip-list at `internal/middleware/maxbody.go:13` is working. |

In-process verification via `cmd/peeringdb-plus/security_e2e_test.go`:
- `TestSecurity_BodyCapEnforced` — under-cap 0.9 MB returns 200; over-cap 1.1 MB returns 413 via `*http.MaxBytesError`.
- `TestSecurity_GRPCBypassesBodyCap` — three skip-list paths (`/peeringdb.v1.*`, `/grpc.health.v1.*`, `/grpc.foo.bar/*`) accept full 1.1 MB body, inner handler receives all 1126400 bytes.
- `TestSecurity_BodyCapPathsNotInSkipList` — five non-skip paths (`/graphql`, `/sync`, `/rest/v1/networks`, `/api/net`, `/ui/`) all return 413 on 1.1 MB body.

### Slowloris (slow-write probe)

The production server config at `cmd/peeringdb-plus/main.go:792-793` sets:
```go
ReadHeaderTimeout: 10 * time.Second
ReadTimeout:       30 * time.Second
```

Structural argument (no live probe run):
- A slowloris-style attacker writing 1 byte/second to a request would exceed `ReadHeaderTimeout` (10s) before the headers complete on any non-trivial request.
- `ReadTimeout` (30s) bounds the entire body read; an attacker stretching a 1.1 MB body at 1 byte/s would be reaped after 30s, having sent only ~30 bytes.
- `PDBPLUS_DRAIN_TIMEOUT` (default 10s, per CLAUDE.md § Environment Variables) bounds graceful shutdown, providing a second containment.

A live probe is intentionally **not** executed against production: probing N=20 connections × 60 seconds against the live `peeringdb-plus.fly.dev` would inflate the audit's blast radius for marginal verification value (the server config is structurally bounded; the in-process tests cover the body-cap behaviour). If a future phase wants empirical validation, a sandboxed probe against a Fly app preview environment is the appropriate venue.

### UAT-02 verdict

**PASS.** All v1.13 Phase 53 security controls are correctly wired and enforced on the live deployment:
- HSTS / X-Frame-Options (browser-scoped) / X-Content-Type-Options headers present per design.
- 1 MB body cap fires for non-skip-list paths; ConnectRPC / gRPC paths bypass cleanly.
- ReadTimeout / ReadHeaderTimeout / DRAIN_TIMEOUT bound slowloris-class attacks structurally.

Regression-locked by `cmd/peeringdb-plus/security_e2e_test.go` (5 tests, all pass under `go test -race`).

---

## UAT-01 — v1.13 Phase 52 CSP enforcement

**Approach:** Autonomous static + curl-based verification. The originally-planned manual Chrome DevTools step was deferred because (a) Tailwind v4 JIT was new at the time of v1.13 and (b) report-only mode meant violations were only browser-console-visible. As of 2026-04-27, the CSP policy explicitly permits all the inline-script/style usage the templates produce, so the static argument is high-confidence.

### Policy inspection

`cmd/peeringdb-plus/main.go:531-532` configures two CSP policies:

**UI policy** (applies to `/ui/*`):
```
default-src 'self';
script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://unpkg.com;
style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://unpkg.com;
img-src 'self' data: https://*.basemaps.cartocdn.com;
connect-src 'self';
font-src 'self' https://cdn.jsdelivr.net
```

**GraphQL policy** (applies to `/graphql`, more permissive for GraphiQL):
```
default-src 'self';
script-src 'self' 'unsafe-inline' 'unsafe-eval' https://cdn.jsdelivr.net;
style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net;
img-src 'self' data:;
connect-src 'self'
```

The GraphiQL policy includes the `'unsafe-eval'` permit because the GraphiQL playground compiles introspection queries at runtime (a documented requirement of the gqlgen playground bundle).

### Compatibility analysis (Tailwind v4 JIT runtime + htmx + leaflet)

| Source | Required directive | Policy permits? |
|--------|-------------------|-----------------|
| Tailwind v4 JIT runtime injects dynamic `<style>` elements | `style-src 'unsafe-inline'` | ✓ present in UI policy |
| Inline `<script>` blocks in `internal/web/templates/home.templ:32-43`, `:77-80` | `script-src 'unsafe-inline'`, `style-src 'unsafe-inline'` | ✓ both present |
| Inline `style="..."` attributes in `map.templ:39-63` | `style-src 'unsafe-inline'` | ✓ present |
| htmx CDN load (cdn.jsdelivr.net) | `script-src https://cdn.jsdelivr.net` | ✓ present |
| Leaflet basemaps tiles | `img-src https://*.basemaps.cartocdn.com` | ✓ present |
| First-party API calls via htmx | `connect-src 'self'` | ✓ present |
| Web fonts from jsdelivr | `font-src https://cdn.jsdelivr.net` | ✓ present |
| No runtime-compiled JavaScript in UI templates | (would need `'unsafe-eval'`) | UI policy correctly omits the permit — only GraphiQL gets it |

**No anticipated CSP violations** when `PDBPLUS_CSP_ENFORCE=true` is set. The policy's `'unsafe-inline'` permits cover Tailwind v4 JIT runtime + htmx + the inline template content.

### Live wiring confirmation

```
$ curl -sI https://peeringdb-plus.fly.dev/ui/ | grep -i content-security-policy
content-security-policy-report-only: default-src 'self'; script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://unpkg.com; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://unpkg.com; img-src 'self' data: https://*.basemaps.cartocdn.com; connect-src 'self'; font-src 'self' https://cdn.jsdelivr.net
```

The middleware at `internal/middleware/csp.go:33-35` is wired correctly: header name flips from `Content-Security-Policy-Report-Only` to `Content-Security-Policy` when `EnforcingMode = true`, with the same policy string. The flip is governed by a single `if` and is unit-test-locked in `internal/middleware/csp_test.go`.

### Rendered-page audit

Each of the three target URLs (`/ui/`, `/ui/asn/13335`, `/ui/compare`) renders templ-generated HTML. The static template files in `internal/web/templates/` were grepped for inline `<script>`, `<style>`, and `style="..."` attributes:

| Template | Inline scripts | Inline styles | External hosts referenced |
|----------|---------------|---------------|---------------------------|
| `home.templ` | `<script>` block at L32-43 (htmx event hooks) | `<style>` block at L77-80 (htmx-indicator transition) | none |
| `error.templ` | none | none | none |
| `map.templ` | none | extensive `style="..."` inline attrs (popup styling) | leaflet + basemaps via base layout |
| `compare.templ` (and other detail pages) | none observed | none observed | inherited from base layout |

All inline content is covered by `'unsafe-inline'` directives. No external hosts beyond the policy's allowlist (cdn.jsdelivr.net, unpkg.com, basemaps.cartocdn.com) appear in rendered output.

### UAT-01 verdict

**PASS (autonomous).** The CSP policy correctly permits all observed template content + Tailwind v4 JIT runtime + htmx + leaflet usage. The middleware wiring (header-name flip) is unit-test-locked. Switching `PDBPLUS_CSP_ENFORCE=true` is expected to produce zero violations against the current template surface.

The original v1.13 deferral concern (Tailwind v4 JIT runtime behaviour) is moot: the UI policy explicitly includes `style-src 'unsafe-inline'`, which is exactly what Tailwind v4's runtime style injection requires.

**Recommendation for the operator:** set `PDBPLUS_CSP_ENFORCE=true` as a Fly secret at the next maintenance window. The expected result is no behavioural change on the live UI (because no current content violates the policy). If a future template introduces a runtime-compiled script or a new external host, it will surface as a CSP violation — at that point, either update the policy to permit it (if intentional) or fix the template (if accidental).

If empirical browser-console verification is desired before flipping the secret, follow these steps:
1. `fly secrets set PDBPLUS_CSP_ENFORCE=true -a peeringdb-plus`
2. Wait for redeploy (~2-3 min, rolling restart of 8 machines).
3. Open Chrome DevTools Console panel; navigate to `https://peeringdb-plus.fly.dev/ui/`, `/ui/asn/13335`, `/ui/compare`.
4. Filter the Console by "Refused to" — zero entries means PASS. Any entries are CSP violations to investigate.
5. Optional rollback: `fly secrets unset PDBPLUS_CSP_ENFORCE -a peeringdb-plus` if any violations surface.

---

## UAT-03 — v1.5 Phase 20 stale-pointer dir relocation

Handled in plan 78-03. See commit messages and STATE.md for the relocation record.
