---
phase: 16-asn-comparison
verified: 2026-03-24T07:35:00Z
status: passed
score: 5/5 must-haves verified
---

# Phase 16: ASN Comparison Verification Report

**Phase Goal:** Users can compare two networks to see where they share presence, answering "where can we peer?"
**Verified:** 2026-03-24T07:35:00Z
**Status:** passed
**Re-verification:** No -- initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | User can enter two ASNs on a dedicated /compare page and see their shared IXPs, facilities, and campuses | VERIFIED | `CompareFormPage` template with two `<input type="number">` fields (IDs `compare-asn1`, `compare-asn2`), JavaScript submit redirect to `/ui/compare/{asn1}/{asn2}`, `CompareResultsPage` renders SharedIXPs/SharedFacilities/SharedCampuses sections. Handler test `TestCompareResultsPage` confirms 200 response with "Cloudflare", "Google", "DE-CIX Frankfurt", "Equinix FR5" in body. |
| 2 | Shared IXP results display port speeds and IP addresses for both networks at each exchange | VERIFIED | `CompareIXPresence` struct holds Speed, IPAddr4, IPAddr6, IsRSPeer, Operational. Template `compareIXPresenceDetail` renders speed via `formatSpeed()`, IPv4, IPv6, and RS indicator. Test `TestCompareService_SharedIXPs` asserts NetA.Speed=10000, NetA.IPAddr4="80.81.193.100", NetB.Speed=100000, NetB.IPAddr4="80.81.193.200", NetB.IPAddr6="2001:7f8::3b41:0:1". |
| 3 | User can toggle between a shared-only view (default) and a full side-by-side view of all presences | VERIFIED | Template renders two toggle links: "Shared Only" (href `/ui/compare/{a}/{b}`) and "Full View" (href `/ui/compare/{a}/{b}?view=full`). Active state uses emerald styling, inactive uses neutral. Handler reads `?view=` query param and passes to CompareInput.ViewMode. `compareIXPsSection` and `compareFacilitiesSection` conditionally iterate SharedIXPs vs AllIXPs. Non-shared rows get `opacity-40` class via `compareRowClasses()`. Test `TestCompareResultsPage_FullView` confirms AMS-IX and Equinix AM5 appear in full view. |
| 4 | User can initiate a comparison from any network detail page via a "Compare with..." button that pre-fills one ASN | VERIFIED | `detail_net.templ` line 16-20: `<a href={templ.SafeURL(fmt.Sprintf("/ui/compare/%d", data.ASN))} ...>Compare with&hellip;</a>`. Handler dispatches `/ui/compare/{asn1}` to `handleCompare` which renders `CompareFormPage(parts[0], "")` pre-filling the first ASN. Test `TestNetworkDetailPage_CompareButton` confirms `/ui/compare/13335` link and "Compare with" text present on network detail page. Test `TestCompareFormPagePreFilled` confirms `/ui/compare/13335` returns 200 with "13335" in body. |
| 5 | The comparison URL captures both ASNs, making results shareable via link | VERIFIED | URL pattern `/ui/compare/{asn1}/{asn2}` is path-based (not query params). Handler `handleCompare` parses `path` via `strings.SplitN(path, "/", 2)`, converts both parts to ints, calls `CompareService.Compare`. Direct GET to `/ui/compare/13335/15169` renders results. View mode preserved via `?view=full` query param. Test `TestCompareResultsPage` confirms direct URL access returns 200 with results. |

**Score:** 5/5 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/web/templates/comparetypes.go` | Comparison data types for templates | VERIFIED | 102 lines. Contains CompareData, CompareNetwork, CompareIXP, CompareIXPresence, CompareFacility, CompareFacPresence, CompareCampus, CompareCampusFacility -- all with doc comments per API-1. |
| `internal/web/compare.go` | CompareService with Compare method | VERIFIED | 462 lines. CompareService struct, NewCompareService constructor, CompareInput struct (CS-5), Compare method with errgroup fan-out (CC-4), map-based set intersection for IXPs/facilities, campus derivation via WithCampus() eager loading, full view union computation. All errors wrapped with %w (ERR-1). |
| `internal/web/compare_test.go` | Tests for comparison logic | VERIFIED | 568 lines. 7 test functions: SharedIXPs, SharedFacilities, SharedCampuses, NoOverlap, FullViewIXPs, FullViewFacilities, InvalidASN. All use t.Parallel() (T-3). Comprehensive seed data helper. All pass with -race. |
| `internal/web/templates/compare.templ` | Comparison page templates | VERIFIED | 306 lines. CompareFormPage (form with two inputs + JS redirect), CompareResultsPage (header, view toggle, stat badges, IXP/facility/campus sections). Generated Go code: 956 lines in compare_templ.go. |
| `internal/web/handler.go` | Compare route dispatch and handlers | VERIFIED | Handler struct has `comparer *CompareService` field. Dispatch routes `compare` and `compare/` cases present. `handleCompareForm` and `handleCompare` methods implemented with error handling. |
| `internal/web/templates/detail_net.templ` | Compare with... button on network detail page | VERIFIED | Line 16-20: anchor tag with href `/ui/compare/{asn}` and "Compare with..." text. |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `handler.go` | `compare.go` | `h.comparer.Compare()` | WIRED | Line 198: `data, err := h.comparer.Compare(r.Context(), CompareInput{...})` |
| `handler.go` | `compare.templ` | `templates.CompareFormPage`, `templates.CompareResultsPage` | WIRED | Lines 157, 178, 214 |
| `compare.go` | `comparetypes.go` | Returns `*templates.CompareData` | WIRED | 30+ references to templates.Compare* types throughout compare.go |
| `compare.go` | ent client | NetworkIxLan, NetworkFacility, Facility queries | WIRED | Lines 46, 51, 69, 80, 91, 102, 297 -- all query via s.client |
| `detail_net.templ` | `/ui/compare/{asn}` | href link | WIRED | Line 16: `href={templ.SafeURL(fmt.Sprintf("/ui/compare/%d", data.ASN))}` |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|--------------|--------|-------------------|--------|
| `compare.go` | CompareData | ent client queries (Network, NetworkIxLan, NetworkFacility, Facility) | Yes -- real DB queries via ent ORM | FLOWING |
| `compare.templ` | data CompareData | Passed from handler via CompareService.Compare() | Yes -- handler calls service, service queries DB | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Comparison tests pass with race detector | `go test -race -count=1 ./internal/web/ -run TestCompareService` | ok (1.848s) | PASS |
| Full web test suite passes | `go test -race -count=1 ./internal/web/...` | ok (3.791s) | PASS |
| Web package builds cleanly | `go build ./internal/web/...` | exit 0 | PASS |
| Web package passes vet | `go vet ./internal/web/...` | exit 0 | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-----------|-------------|--------|----------|
| COMP-01 | 16-01, 16-02 | User can compare two ASNs on a dedicated /compare page with two input fields | SATISFIED | CompareFormPage template, dispatch routing, TestCompareFormPage |
| COMP-02 | 16-01, 16-02 | Comparison results show shared IXPs, facilities, and campuses | SATISFIED | computeSharedIXPs, computeSharedFacilities, computeSharedCampuses in compare.go; compareIXPsSection, compareFacilitiesSection, compareCampusesSection in compare.templ |
| COMP-03 | 16-01, 16-02 | Shared IXP results display port speeds and IP addresses for both networks | SATISFIED | CompareIXPresence struct with Speed/IPAddr4/IPAddr6; compareIXPresenceDetail template; TestCompareService_SharedIXPs asserts specific values |
| COMP-04 | 16-01, 16-02 | User can toggle between shared-only view and full side-by-side view | SATISFIED | ViewMode in CompareInput/CompareData; view toggle links in template; computeAllIXPs/computeAllFacilities; compareRowClasses opacity dimming; TestCompareResultsPage_FullView |
| COMP-05 | 16-02 | User can initiate comparison from a network detail page via a "Compare with..." button | SATISFIED | detail_net.templ Compare with... link; handleCompare pre-fill path; TestNetworkDetailPage_CompareButton, TestCompareFormPagePreFilled |

No orphaned requirements found. All 5 COMP requirements mapped to this phase in REQUIREMENTS.md are covered by plan frontmatter and verified.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (none) | - | - | - | - |

No TODOs, FIXMEs, placeholders, stub returns, or hardcoded empty data found in any phase files. All `return nil` occurrences in compare.go are within errgroup goroutines returning nil on success or early-return for empty input slices -- not stubs.

### Human Verification Required

### 1. Visual Comparison Page Layout

**Test:** Navigate to `/ui/compare/13335/15169` in a browser
**Expected:** Both network names displayed as header, shared IXPs/facilities/campuses listed with port speeds, IP addresses, and proper formatting. Dark theme styling with emerald accents matches existing detail pages.
**Why human:** Visual layout, spacing, and styling quality cannot be verified programmatically.

### 2. View Toggle Behavior

**Test:** Click "Full View" toggle on comparison results page
**Expected:** Page reloads showing all presences for both networks. Non-shared entries are visually dimmed (opacity-40). Shared entries appear at full opacity. URL updates to include `?view=full`.
**Why human:** Visual opacity difference and active toggle state appearance require human eyes.

### 3. Compare with... Button Flow

**Test:** Navigate to a network detail page (e.g., `/ui/asn/13335`), click "Compare with..." button
**Expected:** Navigates to `/ui/compare/13335` with first ASN pre-filled. Enter second ASN and submit to see results.
**Why human:** Multi-step user flow across pages.

### 4. Mobile Responsiveness

**Test:** View `/ui/compare/13335/15169` on a narrow viewport
**Expected:** Form inputs stack vertically on mobile. IXP comparison rows stack (grid-cols-1) instead of side-by-side (lg:grid-cols-3). Network labels appear on mobile (lg:hidden class).
**Why human:** Responsive layout behavior requires visual inspection at different viewport widths.

### Gaps Summary

No gaps found. All 5 success criteria from the roadmap are verified through code inspection and passing tests. The CompareService correctly implements set intersection for IXPs, facilities, and campuses. Templates render both form and results views with view toggle. Handler wiring is complete with dispatch routing, error handling, and pre-fill support. "Compare with..." button is present on network detail pages. All 14 tests (7 service + 7 handler) pass with race detector.

---

_Verified: 2026-03-24T07:35:00Z_
_Verifier: Claude (gsd-verifier)_
