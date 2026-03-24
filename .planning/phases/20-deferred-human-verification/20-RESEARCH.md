# Phase 20: Deferred Human Verification - Research

**Researched:** 2026-03-24
**Domain:** Manual verification of 26 deferred items across CI, API key auth, and browser-based UX against live Fly.io deployment
**Confidence:** HIGH

## Summary

Phase 20 is a pure verification phase -- no new code is expected unless verification reveals failures. The 26 deferred items come from three milestones: v1.2 (3 CI items), v1.3 (3 API key items), and v1.4 (20 browser UX items). All items were deferred because they require either a live GitHub Actions run, a real PeeringDB API key, or a running browser against the live deployment at `https://peeringdb-plus.fly.dev/`.

The live deployment is confirmed accessible (HTTP 200 on all key endpoints as of research time). CI is running and all 4 jobs are passing on main. Content negotiation is working (verified via curl). The codebase has all the features implemented -- this phase is about confirming they work correctly in production.

**Primary recommendation:** Execute verification in dependency order (CI -> API key -> foundation -> search -> detail -> comparison -> polish), fix failures inline with atomic commits, and produce a structured verification report as `20-VERIFICATION-ITEMS.md`.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Structured report -- create `20-VERIFICATION-ITEMS.md` with pass/fail/blocked table per item, browser/OS noted, text descriptions of what was observed
- **D-02:** Text only -- no screenshots. Pass/fail with description is sufficient and diff-friendly.
- **D-03:** Chrome only for browser testing -- this is a niche tool, cross-browser coverage isn't needed
- **D-04:** Use Chrome DevTools for responsive testing at 375px (mobile), 768px (tablet), 1024px+ (desktop)
- **D-05:** Verify in dependency order: Infrastructure -> Data layer -> Foundation -> Search -> Detail pages -> Comparison -> Polish
- **D-06:** If an earlier item fails, stop and fix before proceeding to dependent items
- **D-07:** Live push test -- push a trivial commit or open a test PR to verify CI runs and coverage comments work. Check if the 12 commits we just pushed triggered CI.
- **D-08:** Verify all 4 CI jobs pass (lint + generate drift, test -race, build, govulncheck)
- **D-09:** Verify coverage comment posts on PR and deduplicates on subsequent pushes
- **D-10:** User has a PeeringDB API key available for testing
- **D-11:** Test `pdbcompat-check --api-key <key>` locally
- **D-12:** Test `-peeringdb-live` integration test with `PDBPLUS_PEERINGDB_API_KEY=<key>`
- **D-13:** Test invalid key rejection: `--api-key invalid-key-here` should produce WARN log, not crash
- **D-14:** Fix inline -- if a verification item fails and the fix is needed, fix it in Phase 20 and re-verify. No deferral.
- **D-15:** Each fix gets its own atomic commit with a clear description of what failed and how it was fixed

### Claude's Discretion
- Whether to throttle network in DevTools for loading indicator verification
- How to trigger the syncing page animation (may require deploying a fresh instance or clearing DB)
- Whether to test keyboard navigation with screen reader or just keyboard alone
- How to structure the verification report (per-phase grouping vs. flat list)

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| VFY-01 | CI workflow executes on GitHub Actions push with all 4 jobs passing | CI confirmed running (5 recent runs visible, latest passing). Workflow file verified: 4 jobs (lint, test, build, govulncheck). Check existing runs or push test commit. |
| VFY-02 | Coverage comment posts on PRs and deduplicates on subsequent pushes | coverage-comment.sh verified: posts via gh api, deduplicates by searching for existing bot comment. Requires creating a real PR to test. |
| VFY-03 | pdbcompat-check CLI works with real PeeringDB API key | CLI code verified: `--api-key` flag with env var fallback. Sets `Authorization: Api-Key <key>` header. Rate limits at 3s between types. |
| VFY-04 | Live integration test passes with API key, invalid key produces WARN log | Live test verified: `-peeringdb-live` flag, env var `PDBPLUS_PEERINGDB_API_KEY`, 1s sleep with key vs 3s without. Invalid key produces error containing "API key may be invalid". |
| VFY-05 | Web UI foundation verified (content negotiation, responsive layout, syncing page animation) | Content negotiation confirmed working via curl. Responsive layout uses Tailwind browser CDN with viewport meta tag. Syncing page is a standalone template with auto-refresh. |
| VFY-06 | Live search verified (latency < 300ms, type badges, ASN redirect on Enter) | Search uses htmx with 300ms debounce delay. Colored badges per type (emerald/sky/violet/amber/rose/cyan). ASN redirect via client-side JS `handleSearchSubmit`. |
| VFY-07 | Detail pages verified (collapsible sections, lazy loading, summary stats, cross-links) | CollapsibleSection uses HTML `<details>` with htmx `toggle once` trigger. StatBadge shows counts. Cross-links use `/ui/{type}/{id}` URLs. |
| VFY-08 | ASN comparison verified (results layout, view toggle, compare flow) | Compare form at `/ui/compare`, results at `/ui/compare/{asn1}/{asn2}`, view toggle via `?view=full` query param. "Compare with..." button on network detail pages. |
| VFY-09 | Polish verified (dark mode toggle, keyboard nav, CSS animations, loading indicators, error pages, About page freshness) | Dark mode via localStorage + class toggle. Keyboard nav in layout.templ JS. FadeIn animation CSS. htmx indicator bar. Error templates with search box (404) and home link (500). About page queries last sync status. |
</phase_requirements>

## Architecture Patterns

### Verification Report Structure

The output artifact is `20-VERIFICATION-ITEMS.md`, structured as a table-based report. Recommended structure follows the dependency order from D-05:

```
# Verification Report

**Date:** YYYY-MM-DD
**Browser:** Chrome [version], [OS]
**Deployment:** https://peeringdb-plus.fly.dev/

## Summary
- Total: 26 items
- Passed: X
- Failed: X
- Blocked: X

## VFY-01: CI Pipeline (v1.2)
| # | Item | Status | Observation |
|---|------|--------|-------------|
| 1 | CI workflow executes on push | PASS/FAIL/BLOCKED | [description] |
| 2 | Coverage comment posts on PR | PASS/FAIL/BLOCKED | [description] |
| 3 | Comment deduplication | PASS/FAIL/BLOCKED | [description] |

## VFY-02: API Key Auth (v1.3)
[same structure]

## VFY-05-09: Web UI (v1.4)
[grouped by phase: Foundation, Search, Detail, Compare, Polish]
```

### Verification Execution Pattern

Each verification item follows a consistent flow:

1. **Precondition check** -- is the item's dependency verified?
2. **Execute verification step** -- perform the action described
3. **Observe result** -- record what happened
4. **Record status** -- PASS (works as expected), FAIL (broken, needs fix), BLOCKED (cannot verify, with justification)
5. **If FAIL** -- fix inline, re-verify, record fix commit hash

### Fix-Inline Pattern

When a verification item fails (per D-14, D-15):

1. Diagnose the failure
2. Implement the fix in the relevant source file
3. Verify the fix locally (test suite, or re-check against deployment)
4. Commit with message: `fix(web): [what failed] -- [what was fixed]`
5. Re-verify the item against the deployment (may require redeployment)
6. Update the verification report with the fix commit hash

### Anti-Patterns to Avoid
- **Batch-fixing after verification:** Fix each failure as discovered, before continuing to dependent items (D-06)
- **Marking items PASS without actually testing:** Each item must have a concrete observation recorded
- **Marking failures as BLOCKED:** A failure is a failure. BLOCKED is only for items that genuinely cannot be tested (e.g., syncing page requires a fresh deployment with no data)

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Visual regression testing | Playwright/Cypress screenshot comparison | Manual Chrome verification per D-03 | Setup cost exceeds value for 20 one-time items (Anti-Feature in FEATURES.md) |
| CI verification | Custom CI trigger script | `gh run list` + existing CI triggers on push/PR | CI already runs, just check the existing runs |
| Coverage comment testing | Mock PR comments | Create a real test PR with `gh pr create` | The script uses `gh api` calls that only work in real GitHub context |

## Common Pitfalls

### Pitfall 1: Dependency Order Violations (from PITFALLS.md #12)
**What goes wrong:** Verifying detail page features before confirming search works, or verifying comparison before confirming detail pages work.
**Why it happens:** The 26 items look like an independent checklist but have implicit dependencies.
**How to avoid:** Follow the exact order in D-05. If infrastructure fails, all browser items are meaningless. If search is broken, detail page cross-links cannot be verified.
**Warning signs:** An item marked PASS depends on a feature that was never explicitly verified.

### Pitfall 2: Cross-Environment UX Differences (from PITFALLS.md #7)
**What goes wrong:** Verifying on a local dev build instead of the live Fly.io deployment. CSS renders differently, network latency affects loading indicators, CDN availability affects Tailwind/htmx.
**Why it happens:** It is tempting to run locally for faster iteration, but the items were deferred precisely because they need the real deployment.
**How to avoid:** All browser-based verification (VFY-05 through VFY-09) MUST target `https://peeringdb-plus.fly.dev/`. Only CLI-based items (VFY-03, VFY-04) run locally.
**Warning signs:** "It works on my machine" without verifying against the live URL.

### Pitfall 3: Syncing Page Cannot Be Verified Non-Destructively
**What goes wrong:** The syncing page animation only shows before the first sync completes. The live deployment has already synced. Verifying this would require deploying a fresh instance with an empty database.
**Why it happens:** The readiness middleware checks `HasCompletedSync()` and only shows the syncing page when false. Once a sync completes, it never shows again (until restart with empty DB).
**How to avoid:** This item may need to be marked BLOCKED with justification, or verified by code review of the readiness middleware and SyncingPage template. The middleware code is confirmed correct (line 296-321 of main.go), and the template renders a pulse animation with auto-refresh. Alternatively, use Claude's discretion per CONTEXT.md to verify by reading the code logic.
**Warning signs:** Attempting to clear the production database to see the syncing page.

### Pitfall 4: Coverage Comment Requires a Real PR
**What goes wrong:** Trying to verify coverage comment posting without creating an actual PR. The script uses `gh api` with `PR_NUMBER` and `GITHUB_REPOSITORY` environment variables set by GitHub Actions.
**Why it happens:** Coverage comments are posted only on `pull_request` events (line 72-73 of ci.yml). A push to main does not trigger the comment step.
**How to avoid:** Create a real test PR (even with a trivial change) to trigger the coverage comment flow. Push a second commit to verify deduplication.
**Warning signs:** Checking that CI passes on main and assuming coverage comments work -- they only fire on PRs.

### Pitfall 5: API Key Tests Are Rate-Limited
**What goes wrong:** Running `pdbcompat-check` with all 13 types takes ~39 seconds (3s sleep between each type, or 13s with API key at 1s sleep). Running the live integration test takes even longer. If the API key is invalid or expired, the test hangs waiting for rate limits.
**Why it happens:** PeeringDB enforces rate limits (20 req/min unauthenticated, 60 req/min authenticated). The tooling respects this with `time.Sleep`.
**How to avoid:** Test with `--type net` first (single type, fast) to verify the API key works before running all 13 types.
**Warning signs:** Long execution times or 401/403 errors on the first request.

### Pitfall 6: Dark Mode System Preference Detection Is Stateful
**What goes wrong:** Testing dark mode toggle without first clearing localStorage. The toggle writes to `localStorage.setItem('darkMode', ...)` which overrides system preference detection. If a previous test set it to "dark", the system preference detection cannot be verified.
**Why it happens:** The layout.templ script checks localStorage first, then `prefers-color-scheme`. localStorage takes precedence.
**How to avoid:** Clear localStorage before testing system preference detection. Verify in this order: (1) clear localStorage, verify system preference is respected, (2) toggle manually, verify switch works, (3) reload, verify persistence.
**Warning signs:** Dark mode appears to work but system preference detection was never actually tested.

## Verification Item Detailed Procedures

### Group 1: Infrastructure (VFY-01, VFY-02) -- CI Pipeline

**Items:**
1. CI workflow executes on push with all 4 jobs passing
2. Coverage comment posts on PR
3. Comment deduplication on subsequent pushes

**Procedure:**
- Check `gh run list --limit 5` for recent passing runs on main
- CI already runs on push to main (confirmed: 5 recent runs, latest passing)
- For coverage comment: create a test PR from the current phase branch
- Push initial commit, verify coverage comment appears
- Push a second commit, verify comment is updated (not duplicated)
- The coverage-comment.sh script searches for existing `## Test Coverage` comments from `github-actions[bot]` and PATCHes if found

**Key code paths:**
- `.github/workflows/ci.yml` -- 4 jobs: lint (golangci-lint + ent drift + templ drift), test (-race + coverage), build, govulncheck
- `.github/scripts/coverage-comment.sh` -- gh api POST/PATCH with dedup logic

### Group 2: Data Layer (VFY-03, VFY-04) -- API Key Auth

**Items:**
4. pdbcompat-check CLI with real API key
5. Live integration test with API key
6. Invalid key rejection

**Procedure:**
- Run `go run ./cmd/pdbcompat-check --api-key <key> --type net` (single type first)
- If OK, run full `go run ./cmd/pdbcompat-check --api-key <key>` (all 13 types, ~13s with auth)
- Run `PDBPLUS_PEERINGDB_API_KEY=<key> go test -peeringdb-live ./internal/conformance/`
- Run `go run ./cmd/pdbcompat-check --api-key invalid-key-here --type net` and verify it produces error containing "API key may be invalid" without crashing

**Key code paths:**
- `cmd/pdbcompat-check/main.go` -- CLI flag parsing, auth header injection
- `internal/conformance/live_test.go` -- `-peeringdb-live` flag, env var, sleep timing

### Group 3: Foundation (VFY-05) -- Content Negotiation, Layout, Syncing

**Items:**
7. Content negotiation (browser redirect vs JSON for API clients)
8. Responsive layout at breakpoints
9. Syncing page animation

**Procedure:**
- Content negotiation already verified via curl during research:
  - `curl -H 'Accept: text/html' https://peeringdb-plus.fly.dev/` returns 302 to /ui/
  - `curl https://peeringdb-plus.fly.dev/` returns JSON discovery document
- Responsive: open in Chrome, use DevTools device toolbar at 375px, 768px, 1024px+
  - Verify nav collapses to hamburger on mobile
  - Verify grid layouts adapt (API cards stack on mobile, 3-column on desktop)
- Syncing page: likely BLOCKED -- requires fresh deployment with empty DB. Can verify code correctness instead (readiness middleware at main.go:296-321, syncing.templ template)

**Key code paths:**
- `cmd/peeringdb-plus/main.go:239-247` -- content negotiation handler
- `cmd/peeringdb-plus/main.go:296-321` -- readiness middleware
- `internal/web/templates/layout.templ` -- responsive Tailwind classes, viewport meta
- `internal/web/templates/syncing.templ` -- self-contained syncing page with pulse animation
- `internal/web/templates/nav.templ` -- mobile hamburger menu (hidden/md:flex)

### Group 4: Search (VFY-06) -- Live Search, Badges, ASN Redirect

**Items:**
10. Live search latency < 300ms
11. Type badge colors
12. ASN redirect on Enter

**Procedure:**
- Navigate to `https://peeringdb-plus.fly.dev/ui/`
- Type "cloudflare" in search box, observe results appear (htmx triggers after 300ms debounce)
- Verify colored badges (emerald for networks, sky for IXPs, etc.)
- Clear search, type "13335", press Enter, verify redirect to `/ui/asn/13335`
- For latency: use Chrome DevTools Network tab, check XHR timing on search requests

**Key code paths:**
- `internal/web/templates/home.templ` -- search form with htmx attributes, ASN redirect JS
- `internal/web/templates/search_results.templ` -- colored badges per type
- `internal/web/search.go` -- search handler

### Group 5: Detail Pages (VFY-07) -- Collapsible, Lazy Load, Stats, Cross-links

**Items:**
13. Collapsible sections expand/collapse smoothly
14. Lazy loading triggers only on first expand
15. Summary stats visible in header
16. Cross-links navigate between record types

**Procedure:**
- Navigate to `/ui/asn/13335` (Cloudflare -- large network with IXPs and facilities)
- Verify stat badges (IXPs: N, Facilities: N, Contacts: N) visible in header
- Click "IX Presences" section header, verify it expands with htmx load
- Check DevTools Network tab: htmx GET to `/ui/fragment/net/{id}/ixlans` fires once
- Collapse and re-expand: verify no second network request (htmx `once` trigger)
- Click an IX name from the list, verify navigation to `/ui/ix/{id}` detail page

**Key code paths:**
- `internal/web/templates/detail_net.templ` -- StatBadge, CollapsibleSection, cross-links
- `internal/web/templates/detail_shared.templ` -- CollapsibleSection with htmx `toggle once`
- `internal/web/detail.go` -- fragment handlers for lazy loading

### Group 6: Comparison (VFY-08) -- Results, Toggle, Multi-step Flow

**Items:**
17. Comparison results layout
18. View toggle (shared-only vs full side-by-side)
19. Multi-step compare flow (network detail -> compare)

**Procedure:**
- From `/ui/asn/13335`, click "Compare with..." button
- Verify navigation to `/ui/compare/13335` with ASN1 pre-filled
- Enter second ASN (e.g., 15169 for Google), submit
- Verify results page at `/ui/compare/13335/15169` shows shared IXPs/facilities
- Click "Full View" toggle, verify URL changes to `?view=full` and all items show
- Click "Shared Only" toggle, verify return to shared-only view

**Key code paths:**
- `internal/web/templates/detail_net.templ:16-20` -- "Compare with..." button
- `internal/web/templates/compare.templ` -- form, results, view toggle
- `internal/web/compare.go` -- compare handler

### Group 7: Polish (VFY-09) -- Dark Mode, Keyboard, Animations, Errors, About

**Items:**
20. Dark mode toggle and system preference detection
21. Keyboard navigation of search results
22. CSS animations (fadeIn on search results)
23. Loading indicators (htmx loading bar)
24. Styled 404 error page (with search box)
25. Styled 500 error page (with home link)
26. About page data freshness indicator

**Procedure:**
- Dark mode: Clear localStorage, verify system preference respected. Toggle button, verify switch. Reload, verify persistence.
- Keyboard nav: Type search query, press down arrow, verify focus moves through results. Press Enter on focused result, verify navigation.
- CSS animations: Search for a term, observe fadeIn animation on results appearing (`animate-fade-in` class).
- Loading indicators: Watch for green bar at top of page during htmx requests. Throttle network in DevTools if needed to make indicator visible.
- 404 page: Navigate to `/ui/nonexistent`, verify styled page with search box and "Back to home" link.
- 500 page: Cannot trigger naturally. Verify by code review of `ServerErrorPage` template.
- About page: Navigate to `/ui/about`, verify "Last synced: [timestamp]" and "[X] ago" freshness indicator. Verify API surface links.

**Key code paths:**
- `internal/web/templates/layout.templ` -- dark mode JS, keyboard nav JS, CSS animations, htmx indicator
- `internal/web/templates/nav.templ` -- dark mode toggle button (sun/moon icons)
- `internal/web/templates/error.templ` -- NotFoundPage (404 with search), ServerErrorPage (500 with home link)
- `internal/web/templates/about.templ` -- freshness display with formatAge helper
- `internal/web/about.go` -- queries last sync status from DB

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Screenshot-based verification | Text-only pass/fail reports (D-02) | This phase | Diff-friendly, reviewable in PRs |
| Cross-browser testing | Chrome-only (D-03) | This phase | Sufficient for niche tool |
| Manual deployment for syncing page | Code review verification | This phase | Non-destructive alternative |

## Open Questions

1. **Syncing page verification**
   - What we know: The syncing page only renders before first sync completion. The live deployment has already synced.
   - What is unclear: Whether we can verify this non-destructively without deploying a fresh instance.
   - Recommendation: Mark as BLOCKED with code-review justification, or verify by reading the readiness middleware and template. The middleware logic is confirmed correct (checked during research).

2. **500 error page verification**
   - What we know: The ServerErrorPage template exists and is wired into `handleServerError`. It renders correctly in tests.
   - What is unclear: How to trigger a real 500 error in production without causing actual problems.
   - Recommendation: Verify by code review. The template content is confirmed (red 500, "Something went wrong", home link). An alternative is to request a path that triggers a DB error, but this is risky against production.

3. **CI coverage comment deduplication**
   - What we know: The script searches for existing comments from `github-actions[bot]` containing `## Test Coverage` and PATCHes if found.
   - What is unclear: Whether the `github-actions[bot]` user login matches across different GitHub org/repo configurations.
   - Recommendation: Create a real test PR with two pushes. This is the only reliable way to verify.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | VFY-03, VFY-04 (CLI + test) | Yes | 1.26.1 | -- |
| curl | VFY-05 (content negotiation) | Yes | 8.14.1 | -- |
| gh CLI | VFY-01, VFY-02 (CI runs, PR creation) | Yes | 2.88.1 | -- |
| Chrome browser | VFY-05 through VFY-09 | User machine | -- | -- |
| Live deployment | VFY-05 through VFY-09 | Yes (200 OK) | -- | -- |
| PeeringDB API key | VFY-03, VFY-04 | User has one (D-10) | -- | -- |
| fly CLI | Not needed | Exists but may not be in PATH | -- | -- |

**Missing dependencies with no fallback:** None -- all dependencies are available.

**Missing dependencies with fallback:** None.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing stdlib + manual browser verification |
| Config file | None -- manual verification phase |
| Quick run command | `go test -race ./...` |
| Full suite command | `go test -race ./...` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| VFY-01 | CI workflow executes | manual | `gh run list --limit 5` | N/A |
| VFY-02 | Coverage comment posts and deduplicates | manual | Create test PR, observe | N/A |
| VFY-03 | CLI with real API key | manual | `go run ./cmd/pdbcompat-check --api-key <key> --type net` | N/A |
| VFY-04 | Live integration test + invalid key | manual | `PDBPLUS_PEERINGDB_API_KEY=<key> go test -peeringdb-live ./internal/conformance/` | N/A |
| VFY-05 | Content negotiation, responsive, syncing | manual + curl | `curl -H 'Accept: text/html' https://peeringdb-plus.fly.dev/` | N/A |
| VFY-06 | Search latency, badges, ASN redirect | manual-only | Browser verification | N/A |
| VFY-07 | Collapsible, lazy load, stats, cross-links | manual-only | Browser verification | N/A |
| VFY-08 | Compare results, toggle, flow | manual-only | Browser verification | N/A |
| VFY-09 | Dark mode, keyboard, animations, errors, about | manual-only | Browser verification | N/A |

### Sampling Rate
- **Per task commit:** `go test -race ./...` (only if code changes were made as fixes)
- **Per wave merge:** Full test suite
- **Phase gate:** All 26 items have pass/fail/blocked status in verification report

### Wave 0 Gaps
None -- this is a manual verification phase with no new test infrastructure needed.

## Project Constraints (from CLAUDE.md)

Applicable constraints for this phase:
- **T-2 (MUST):** Run `-race` in CI -- already enforced, part of VFY-01 verification
- **G-1 (MUST):** `go vet ./...` passes -- part of CI lint job verification
- **G-2 (MUST):** `golangci-lint run` passes -- part of CI lint job verification
- **G-3 (MUST):** `go test -race ./...` passes -- part of CI test job verification
- **ERR-1 (MUST):** Wrap errors with `%w` and context -- applies to any inline fixes
- **OBS-1 (MUST):** Structured slog logging -- applies to any inline fixes
- **SEC-2 (MUST):** Never log secrets -- API key must not appear in verification report text

## Sources

### Primary (HIGH confidence)
- Codebase: `.github/workflows/ci.yml` -- CI workflow with 4 jobs verified
- Codebase: `.github/scripts/coverage-comment.sh` -- dedup logic verified
- Codebase: `cmd/pdbcompat-check/main.go` -- API key handling verified
- Codebase: `internal/conformance/live_test.go` -- live test flag and key handling verified
- Codebase: `cmd/peeringdb-plus/main.go:239-247` -- content negotiation verified
- Codebase: `cmd/peeringdb-plus/main.go:296-321` -- readiness middleware verified
- Codebase: `internal/web/templates/*.templ` -- all UI templates verified
- Live deployment: `https://peeringdb-plus.fly.dev/` -- confirmed accessible (200 OK)
- Live content negotiation: confirmed working (302 with Accept: text/html, JSON without)
- GitHub Actions: `gh run list` shows 5 recent runs, latest passing

### Secondary (MEDIUM confidence)
- `.planning/research/PITFALLS.md` -- Pitfall #7 (cross-environment UX) and #12 (dependency ordering)
- `.planning/milestones/v1.2-MILESTONE-AUDIT.md` -- 3 deferred CI items
- `.planning/milestones/v1.3-MILESTONE-AUDIT.md` -- 3 deferred API key items
- `.planning/milestones/v1.4-MILESTONE-AUDIT.md` -- 20 deferred UX items

### Tertiary (LOW confidence)
None -- all findings verified against codebase or live deployment.

## Metadata

**Confidence breakdown:**
- Verification procedures: HIGH -- all code paths examined, live deployment confirmed accessible
- CI verification: HIGH -- workflow file verified, recent runs confirmed passing
- API key verification: HIGH -- CLI and test code verified, procedures clear
- Browser UX verification: MEDIUM -- procedures documented but actual visual results depend on deployment state and data availability
- Syncing page: LOW -- cannot verify non-destructively against live deployment

**Research date:** 2026-03-24
**Valid until:** 2026-04-24 (stable -- no code changes expected in verification targets)
