---
phase: 45-multi-pin-maps
verified: 2026-03-26T23:15:00Z
status: passed
score: 8/8 must-haves verified
re_verification: false
---

# Phase 45: Multi-Pin Maps Verification Report

**Phase Goal:** Users see maps with multiple facility pins on IX, network, and ASN comparison pages, with clustering for dense regions and colored pins distinguishing shared vs unique facilities
**Verified:** 2026-03-26T23:15:00Z
**Status:** passed
**Re-verification:** No -- initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | IX detail pages display a map with pins for all associated facilities | VERIFIED | `detail_ix.templ:42` calls `ixFacilityMap(data)` which builds MapMarker slice from `data.Facilities` with Lat/Lng from eager-loaded Facility edge (`detail.go:311` WithFacility), renders `MultiPinMapContainer` with `map-ix-{id}`. Integration test `TestIXDetail_FacilityMapMarkers` passes confirming `map-ix-20` div and `initMultiPinMap` in rendered HTML. |
| 2 | Network detail pages display a map with pins for all facility presences | VERIFIED | `detail_net.templ:72` calls `netFacilityMap(data)` which builds MapMarker slice from `data.FacPresences` with Lat/Lng from eager-loaded Facility edge (`detail.go:166` WithFacility), renders `MultiPinMapContainer` with `map-net-{id}`. Integration test `TestNetworkDetail_FacilityMapMarkers` passes confirming `map-net-10` div in rendered HTML. |
| 3 | When many pins overlap, they cluster into numbered circles that expand on click | VERIFIED | `layout.templ:32` loads `leaflet.markercluster@1.5.3` JS, `layout.templ:34-38` applies `L.CircleMarker.setOpacity` shim. `map.templ:134` creates `L.markerClusterGroup()` and adds all circleMarkers to it. Cluster CSS overrides in `layout.templ:41-56` provide emerald theme with light/dark mode variants. |
| 4 | Comparison page displays a map with colored pins distinguishing shared vs unique facilities | VERIFIED | `compare.templ:105` calls `compareFacilityMap(data)` which builds markers via `compareFacilityMarkers` with three-color scheme: emerald `#10b981` shared, sky `#38bdf8` net-A-only, amber `#f59e0b` net-B-only (`compare.templ:305-317`). Legend rendered via `MultiPinMapContainer` with `showLegend=true` and labels `"Shared"`, `"AS{N} only"` (`compare.templ:341-346`). |
| 5 | All multi-pin maps auto-fit bounds to show all pins with maxZoom 13 | VERIFIED | `map.templ:152` calls `map.fitBounds(bounds, { maxZoom: 13, padding: [20, 20] })` where bounds are computed from all marker positions in the loop at lines 137-149. |
| 6 | Maps with zero mappable facilities render no map container | VERIFIED | `MultiPinMapContainer` (`map.templ:201`) calls `filterMappableMarkers` and only renders when `len(mappable) > 0`. Unit test `TestFilterMappableMarkers/all_unmappable` confirms (0,0) markers are filtered. |
| 7 | Unmapped facility count message shown below map when some facilities lack coordinates | VERIFIED | `map.templ:209-219` renders singular/plural unmapped message when `unmapped > 0` via templ if/else on count. |
| 8 | Clicking any pin shows popup with facility name linking to its detail page | VERIFIED | `map.templ:146` calls `.bindPopup(m.popup)` on each circleMarker. Popup HTML is pre-built server-side by `buildMultiPinPopupHTML` which includes facility name, location, optional extra line, and "View facility" link to `/ui/fac/{id}` (`map.templ:54-66`). All user content HTML-escaped (`html.EscapeString`). XSS test case passes. |

**Score:** 8/8 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/web/templates/map.templ` | MultiPinMapContainer, initMultiPinMap, buildMultiPinPopupHTML, filterMappableMarkers, formatMapLocation | VERIFIED | 260 lines, all functions present with full implementations. MapMarker has Color/Stroke/Extra fields (lines 27-31). marshalMarkers/marshalLegend JSON serialization helpers present. |
| `internal/web/templates/detailtypes.go` | Latitude/Longitude on IXFacilityRow, NetworkFacRow, OrgFacilityRow, CampusFacilityRow, CarrierFacilityRow | VERIFIED | All 5 row structs have `Latitude float64` and `Longitude float64` fields with doc comments. |
| `internal/web/templates/comparetypes.go` | Latitude/Longitude on CompareFacility | VERIFIED | CompareFacility has `Latitude float64` and `Longitude float64` at lines 72-74. |
| `internal/web/templates/layout.templ` | Markercluster CDN links, cluster CSS overrides, circleMarker shim | VERIFIED | 2 CSS links (lines 27-28), 1 JS link (line 32), setOpacity shim (lines 34-38), emerald cluster CSS with dark mode variants (lines 41-56). |
| `internal/web/detail.go` | WithFacility() on IX and network facility queries | VERIFIED | IX query at line 311, network query at line 166, both extract coordinates via `Edges.Facility` nil-safe pointer dereference. |
| `internal/web/compare.go` | WithFacility() on all facility queries, coordinate propagation | VERIFIED | WithFacility() on both facNetsA (line 93) and facNetsB (line 105) queries. `computeSharedFacilities` propagates coords at lines 215-221. `computeAllFacilities` uses `extractCoords` closure at lines 412-422. AllFacilities computed unconditionally at line 147. |
| `internal/web/templates/detail_ix.templ` | MultiPinMapContainer rendered on IX detail page | VERIFIED | `ixFacilityMap` templ at line 169, `ixFacilityMarkers` Go func at line 150, inserted at line 42 between Notes and collapsible sections. |
| `internal/web/templates/detail_net.templ` | MultiPinMapContainer rendered on network detail page | VERIFIED | `netFacilityMap` templ at line 113, `netFacilityMarkers` Go func at line 94, inserted at line 72 between Notes and collapsible sections. |
| `internal/web/templates/compare.templ` | MultiPinMapContainer with legend on comparison page | VERIFIED | `compareFacilityMap` templ at line 335, `compareFacilityMarkers` Go func at line 302, inserted at line 105 between stat badges and IX section. Three-color scheme and legend labels present. |
| `internal/web/templates/map_test.go` | Unit tests for buildMultiPinPopupHTML and filterMappableMarkers | VERIFIED | TestBuildMultiPinPopupHTML (5 cases), TestFilterMappableMarkers (5 cases), TestFormatMapLocation (4 cases). All pass with -race. |
| `internal/web/detail_test.go` | Integration tests for IX and network map rendering | VERIFIED | TestIXDetail_FacilityMapMarkers and TestNetworkDetail_FacilityMapMarkers verify map container div presence. Both pass with -race. |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `detail.go` | `detail_ix.templ` | IXDetail.Facilities rows populated with Lat/Lng from WithFacility() | WIRED | WithFacility() at line 311, Edges.Facility nil-check at line 326, Latitude/Longitude extracted at lines 327-332, assigned to data.Facilities at line 336. Template reads data.Facilities at line 173. |
| `detail.go` | `detail_net.templ` | NetworkDetail.FacPresences rows with Lat/Lng from WithFacility() | WIRED | WithFacility() at line 166, Edges.Facility nil-check at line 181, coordinates extracted at lines 182-187, assigned to data.FacPresences at line 191. Template reads data.FacPresences at line 117. |
| `compare.go` | `compare.templ` | CompareFacility.Latitude/Longitude from Facility edge coordinates | WIRED | WithFacility() on both queries (lines 93, 105). computeSharedFacilities extracts coords at lines 215-221. computeAllFacilities extracts via extractCoords closure at lines 412-422, propagates to result at lines 468-469. Template reads data.AllFacilities at line 339. |
| `detail_ix.templ` | `map.templ` | MultiPinMapContainer called with MapMarker slice | WIRED | ixFacilityMarkers builds []MapMarker at line 150, ixFacilityMap passes to MultiPinMapContainer at line 171. |
| `map.templ` | `layout.templ` | markercluster JS makes L.markerClusterGroup available | WIRED | layout.templ loads leaflet.markercluster.js at line 32. map.templ initMultiPinMap calls L.markerClusterGroup() at line 134. Script executes after page load when CDN resources are available. |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| `detail_ix.templ` (ixFacilityMap) | data.Facilities | detail.go IxFacility.Query().WithFacility().All() | Yes -- DB query with ent ORM | FLOWING |
| `detail_net.templ` (netFacilityMap) | data.FacPresences | detail.go NetworkFacility.Query().WithFacility().All() | Yes -- DB query with ent ORM | FLOWING |
| `compare.templ` (compareFacilityMap) | data.AllFacilities | compare.go computeAllFacilities(facNetsA, facNetsB) | Yes -- facNetsA/B from NetworkFacility.Query().WithFacility().All() | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Build passes | `go build ./internal/web/...` | Clean exit, no errors | PASS |
| Vet passes | `go vet ./internal/web/templates/` | Clean exit, no errors | PASS |
| Unit tests pass | `go test ./internal/web/templates/ -run "TestBuildMultiPinPopup\|TestFilterMappable\|TestFormatMapLocation" -race` | 14 test cases, all PASS | PASS |
| Integration tests pass | `go test ./internal/web/ -run "TestIXDetail_Facility\|TestNetworkDetail_Facility" -race` | 2 test cases, all PASS | PASS |
| Commits exist | `git log --oneline` | 1d7f074, 5285479, e9ed252, 928353f all present | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| MAP-02 | 45-01, 45-02 | User sees an interactive map on IX and network detail pages showing all associated facility locations with clustering | SATISFIED | IX detail page renders MultiPinMapContainer with clustered circleMarkers for all IX facilities. Network detail page renders same for all network facility presences. WithFacility() eager-loads coordinates. Integration tests verify map container presence. |
| MAP-03 | 45-01, 45-02 | User sees an interactive map on ASN comparison page with colored pins (shared vs unique facilities) | SATISFIED | Comparison page renders MultiPinMapContainer with three-color scheme (emerald shared, sky net-A-only, amber net-B-only) and legend. AllFacilities computed unconditionally for map rendering. compareFacilityMarkers implements color logic. |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| None found | - | - | - | - |

No TODO, FIXME, placeholder, stub, or empty implementation patterns detected in any modified files.

### Human Verification Required

### 1. Visual Map Rendering

**Test:** Open an IX detail page (e.g., `/ui/ix/20`) and a network detail page (e.g., `/ui/asn/13335`) in a browser. Verify maps render with visible circleMarker pins at facility locations.
**Expected:** Maps appear between the Notes field and the collapsible sections. Pins are emerald green circles. Clicking a pin shows a popup with facility name and "View facility" link. Map auto-zooms to fit all pins.
**Why human:** Cannot verify Leaflet rendering, tile loading, or visual appearance programmatically.

### 2. Cluster Behavior

**Test:** Navigate to a network or IX page with many facilities in a single region (e.g., a large IX like DE-CIX or a network like Cloudflare).
**Expected:** Closely grouped pins cluster into numbered circles with the emerald theme. Clicking a cluster zooms in to reveal individual pins.
**Why human:** Clustering behavior depends on zoom level, pin density, and JS runtime. Cannot verify without a running browser.

### 3. Comparison Map Three-Color Pins and Legend

**Test:** Navigate to `/ui/compare?asn1=13335&asn2=15169` (or any two ASNs with both shared and unique facilities).
**Expected:** Map shows emerald pins for shared facilities, sky blue for net-A-only, amber for net-B-only. Legend in bottom-left corner shows "Shared", "AS{N} only" labels with colored dots.
**Why human:** Color accuracy, legend placement, and visual distinction require visual inspection.

### 4. Dark Mode Map Theming

**Test:** Toggle dark mode on any page with a multi-pin map.
**Expected:** Map tiles switch to CARTO Dark Matter. Cluster circles use darker emerald opacity variants. Legend background becomes dark gray.
**Why human:** Dark mode tile switching and CSS override behavior require visual confirmation.

### 5. Scroll-Zoom Interaction

**Test:** Scroll over a multi-pin map without clicking it first, then click the map and scroll again.
**Expected:** Scroll wheel does not zoom the map until clicked. After clicking, scroll wheel zooms. On mouse-out, scroll zoom disables again.
**Why human:** Interaction behavior requires manual testing in a browser.

### Gaps Summary

No gaps found. All 8 observable truths verified against the codebase. All artifacts exist, are substantive, are properly wired, and have real data flowing through them. Requirements MAP-02 and MAP-03 are fully satisfied. Build, vet, and all tests pass with -race. No anti-patterns detected.

---

_Verified: 2026-03-26T23:15:00Z_
_Verifier: Claude (gsd-verifier)_
