# Phase 20: Deferred Human Verification - Context

**Gathered:** 2026-03-24
**Status:** Ready for planning

<domain>
## Phase Boundary

Verify all 26 deferred human verification items from v1.2, v1.3, and v1.4 against the live Fly.io deployment. Fix failures inline (no deferral). Document results in a structured verification report.

</domain>

<decisions>
## Implementation Decisions

### Verification Approach
- **D-01:** Structured report — create `20-VERIFICATION-ITEMS.md` with pass/fail/blocked table per item, browser/OS noted, text descriptions of what was observed
- **D-02:** Text only — no screenshots. Pass/fail with description is sufficient and diff-friendly.
- **D-03:** Chrome only for browser testing — this is a niche tool, cross-browser coverage isn't needed
- **D-04:** Use Chrome DevTools for responsive testing at 375px (mobile), 768px (tablet), 1024px+ (desktop)

### Verification Order (per Pitfall #12)
- **D-05:** Verify in dependency order:
  1. Infrastructure: CI pipeline on GitHub (VFY-01, VFY-02)
  2. Data layer: API key auth (VFY-03, VFY-04)
  3. Foundation: content negotiation, responsive layout, syncing page (VFY-05)
  4. Search: live search speed, type badges, ASN redirect (VFY-06)
  5. Detail pages: collapsible sections, lazy loading, stats, cross-links (VFY-07)
  6. Comparison: results layout, view toggle, compare flow (VFY-08)
  7. Polish: dark mode, keyboard nav, animations, loading indicators, error pages, About (VFY-09)
- **D-06:** If an earlier item fails, stop and fix before proceeding to dependent items

### CI Verification (VFY-01, VFY-02)
- **D-07:** Live push test — push a trivial commit or open a test PR to verify CI runs and coverage comments work. Check if the 12 commits we just pushed triggered CI.
- **D-08:** Verify all 4 CI jobs pass (lint + generate drift, test -race, build, govulncheck)
- **D-09:** Verify coverage comment posts on PR and deduplicates on subsequent pushes

### API Key Verification (VFY-03, VFY-04)
- **D-10:** User has a PeeringDB API key available for testing
- **D-11:** Test `pdbcompat-check --api-key <key>` locally
- **D-12:** Test `-peeringdb-live` integration test with `PDBPLUS_PEERINGDB_API_KEY=<key>`
- **D-13:** Test invalid key rejection: `--api-key invalid-key-here` should produce WARN log, not crash

### Failure Handling
- **D-14:** Fix inline — if a verification item fails and the fix is needed, fix it in Phase 20 and re-verify. No deferral.
- **D-15:** Each fix gets its own atomic commit with a clear description of what failed and how it was fixed

### Claude's Discretion
- Whether to throttle network in DevTools for loading indicator verification
- How to trigger the syncing page animation (may require deploying a fresh instance or clearing DB)
- Whether to test keyboard navigation with screen reader or just keyboard alone
- How to structure the verification report (per-phase grouping vs. flat list)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Verification Item Sources
- `.planning/research/FEATURES.md` — Complete inventory of all 26 items with source phases and verification steps
- `.planning/milestones/v1.2-MILESTONE-AUDIT.md` — 3 deferred items (CI execution, coverage comments, comment dedup)
- `.planning/milestones/v1.3-MILESTONE-AUDIT.md` — 3 deferred items (API key CLI, integration test, invalid key)
- `.planning/milestones/v1.4-MILESTONE-AUDIT.md` — 20 deferred items (visual/browser UX)
- `.planning/research/PITFALLS.md` — Pitfall #7 (cross-environment UX), Pitfall #12 (dependency ordering)

### Live Deployment
- `fly.toml` — Deployment config
- `.github/workflows/` — CI workflow files to verify
- `cmd/pdbcompat-check/` — CLI tool for API key testing

### Web UI
- `internal/web/` — Web handler code
- `internal/web/templates/` — Templ templates for all pages

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- Live Fly.io deployment at https://peeringdb-plus.fly.dev/
- pdbcompat-check CLI in `cmd/pdbcompat-check/`
- GitHub Actions workflows in `.github/workflows/`

### Established Patterns
- Content negotiation: `Accept: text/html` → redirect to /ui/, otherwise JSON
- htmx fragment endpoints: `/ui/asn/{asn}/ix`, `/ui/asn/{asn}/fac`, etc.
- Dark mode: class-based via @custom-variant, localStorage persistence

### Integration Points
- CI: push to main triggers workflow
- API key: `PDBPLUS_PEERINGDB_API_KEY` env var or `--api-key` flag
- Web UI: all pages accessible at /ui/ prefix

</code_context>

<specifics>
## Specific Ideas

- The 12 commits we just pushed may have already triggered CI — check GitHub Actions first before pushing a test commit
- For the syncing page animation, a fresh deployment without data would show it, but that's destructive — may need to verify by reading the code and confirming the middleware logic instead
- Content negotiation can be tested with curl: `curl -H 'Accept: text/html' https://peeringdb-plus.fly.dev/` should redirect

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 20-deferred-human-verification*
*Context gathered: 2026-03-24*
