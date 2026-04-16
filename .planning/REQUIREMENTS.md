# Requirements: PeeringDB Plus

**Defined:** 2026-04-16
**Core Value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.

## v1.14 Requirements

Requirements for the Authenticated Sync & Visibility Layer milestone. Each maps to roadmap phases 57-62.

### Visibility (VIS)

- [ ] **VIS-01**: System captures both unauthenticated and authenticated PeeringDB API responses for all 13 types as committed baseline fixtures (`testdata/visibility-baseline/beta/` plus a confirmation pass against `www.peeringdb.com` for poc/org/net under `testdata/visibility-baseline/prod/`)
- [ ] **VIS-02**: System emits a structural diff report from the captured fixtures listing every field/row that differs between unauth and auth responses, with a per-type table that's reviewable in code review
- [ ] **VIS-03**: ent schemas have visibility-bearing fields for every auth-gated entity identified by VIS-02 (`poc.visible` already exists; add others as the diff surfaces them and regenerate ent)
- [ ] **VIS-04**: ent Privacy query policy filters non-`Public` rows from anonymous responses on POC and any other types from VIS-03; policy is loaded via `entgo.io/ent/privacy` and wired in `ent/entc.go`
- [ ] **VIS-05**: Sync worker bypasses the privacy policy via `privacy.DecisionContext(ctx, privacy.Allow)` so it can read/write the full dataset; tests assert the bypass is in effect for sync-context goroutines and absent everywhere else
- [ ] **VIS-06**: All 5 read surfaces (`/ui/`, `/graphql`, `/rest/v1/`, `/api/`, `/peeringdb.v1.*`) honour the privacy policy; per-surface integration tests assert no `visible=Users` row leaks on an anonymous request
- [ ] **VIS-07**: pdbcompat `/api/poc` (and embedded `poc_set`) anonymous response shape matches upstream anonymous shape — rows absent, not redacted; verified by replaying the VIS-01 fixtures against our endpoint with an empty diff

### Sync Configuration (SYNC)

- [ ] **SYNC-01**: Authenticated sync becomes the recommended deployment configuration; documented in `docs/CONFIGURATION.md`/`docs/DEPLOYMENT.md` and the production Fly.io app has `PDBPLUS_PEERINGDB_API_KEY` set as a fly secret
- [ ] **SYNC-02**: Unauthenticated sync remains a first-class supported configuration — omit the API key, worker syncs the upstream anonymous payload only, no `Users`-tier rows ever land in the DB; verified by an integration test
- [ ] **SYNC-03**: `PDBPLUS_PUBLIC_TIER` env var (default `public`, accepts `users`) elevates all anonymous callers to Users-tier for private-instance deployments; tests cover both values
- [ ] **SYNC-04**: Startup logs a WARN line when `PDBPLUS_PUBLIC_TIER=users` is in effect, so the elevated default is never silent

### Observability (OBS)

- [ ] **OBS-01**: Startup log line classifies sync as "anonymous, public-only" or "authenticated, full" based on `PDBPLUS_PEERINGDB_API_KEY` presence
- [ ] **OBS-02**: `/about` (HTML) and `/ui/about` (terminal) render the current sync mode and effective privacy tier
- [ ] **OBS-03**: OTel attribute `pdbplus.privacy.tier` (values `public` or `users`) set on read spans; usable as a Grafana dashboard filter

### Documentation (DOC)

- [ ] **DOC-01**: `docs/CONFIGURATION.md` documents `PDBPLUS_PEERINGDB_API_KEY`, `PDBPLUS_PUBLIC_TIER`, and the privacy guarantees they provide
- [ ] **DOC-02**: `docs/DEPLOYMENT.md` documents the recommended authenticated deployment + Fly.io secret setup
- [ ] **DOC-03**: `docs/ARCHITECTURE.md` describes the ent Privacy layer and how each of the 5 surfaces honours it

## Future Requirements

Deferred to v1.15 (or later).

### OAuth Identity (AUTH)

- **AUTH-01**: User can log in via PeeringDB OAuth (`auth.peeringdb.com`) using the authorization code flow with `profile`+`networks` scopes
- **AUTH-02**: An OAuth-identified caller's request context carries `tier=Users`, causing the ent Privacy policy to admit `Users`-visibility rows for that caller
- **AUTH-03**: Session/JWT mechanism plumbs OAuth identity from the OAuth callback through middleware into the ent context
- **AUTH-04**: `/about` shows the logged-in user (when authenticated) and which `networks` they administer per the OAuth `networks` scope

### Domain Extensions (Carried from v1.13 deferred list)

- **BGP-01**: Per-ASN BGP summary from bgp.tools (prefix counts, RPKI coverage)
- **IRR-01**: IRR/AS-SET membership from WHOIS source
- **PFX-01**: IP prefix lookup with origin ASN, RPKI status

### Operational Verification (Carried)

- **OPVR-01**: `fly_region` Grafana template variable verified against live multi-region deployment
- **OPVR-02**: Go runtime metric names verified against live Grafana Cloud
- **OPVR-03**: CI coverage pipeline verified on actual GitHub Actions run
- **OPVR-04**: v1.13 phase 52 CSP enforcement smoke test (Chrome devtools) and phase 53 header smoke tests (curl HSTS/XFO/XCTO, slowloris)

## Out of Scope

| Feature | Reason |
|---------|--------|
| Per-user PeeringDB API key issuance for downstream callers | Out of scope at the project level — server-side config only |
| OAuth in v1.14 | Deferred to v1.15 — privacy floor must ship first to avoid coupling concerns |
| Field-level redaction of non-`Public` rows in responses | We mirror upstream behaviour: rows are absent from anonymous responses, not present-with-redacted-fields |
| Two-way OAuth (we don't act as an OAuth provider) | Mirror is read-only; OAuth use is consumer-only against `auth.peeringdb.com` |
| Detecting whether an OAuth user has org membership for arbitrary records | PeeringDB OAuth `networks` scope is per-network only; we admit any OAuth-authenticated caller to Users-tier (matches what an authenticated PeeringDB API request would see) |
| Real-time visibility-change propagation | Visibility changes ride the existing sync interval — no push channel from upstream exists |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| (filled by gsd-roadmapper) | | |

**Coverage:**
- v1.14 requirements: 17 total
- Mapped to phases: (pending roadmap)
- Unmapped: (pending roadmap)

---
*Requirements defined: 2026-04-16*
*Last updated: 2026-04-16*
