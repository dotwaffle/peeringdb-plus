---
phase: quick
plan: 260331-cxk
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/web/templates/detail_fac.templ
  - internal/web/templates/detail_ix.templ
  - internal/web/templates/detail_net.templ
  - internal/web/templates/detail_shared.templ
  - internal/web/templates/compare.templ
autonomous: true
requirements: []
must_haves:
  truths:
    - "Maps appear below all collapsible sections on fac, ix, net, and compare pages"
    - "Each collapsible section header shows a chevron arrow indicating expandability"
    - "Chevron rotates when section is expanded"
  artifacts:
    - path: "internal/web/templates/detail_fac.templ"
      provides: "Facility detail with map moved below collapsibles"
    - path: "internal/web/templates/detail_ix.templ"
      provides: "IX detail with map moved below collapsibles"
    - path: "internal/web/templates/detail_net.templ"
      provides: "Network detail with map moved below collapsibles"
    - path: "internal/web/templates/detail_shared.templ"
      provides: "CollapsibleSection and CollapsibleSectionWithBandwidth with chevron indicators"
    - path: "internal/web/templates/compare.templ"
      provides: "Compare page with map moved below comparison tables"
  key_links: []
---

<objective>
Move maps below collapsible sections on all detail pages, and add chevron expand/collapse indicators to all collapsible section headers.

Purpose: Improve page layout by showing data-dense collapsible sections first (before the map), and provide visual affordance that sections are expandable.
Output: Updated templ files with reordered maps and chevron indicators.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@internal/web/templates/detail_fac.templ
@internal/web/templates/detail_ix.templ
@internal/web/templates/detail_net.templ
@internal/web/templates/detail_shared.templ
@internal/web/templates/compare.templ

<interfaces>
From detail_shared.templ:
- `CollapsibleSection(title string, count int, loadURL string)` — uses `<details>/<summary>` with htmx lazy load
- `CollapsibleSectionWithBandwidth(title string, count int, bandwidth string, loadURL string)` — same pattern with bandwidth display

From map.templ:
- `MapContainer(id string, markers []MapMarker, zoom int)` — single-pin map (used by fac)
- `MultiPinMapContainer(id string, markers []MapMarker, ariaLabel string, showLegend bool, legendLabels map[string]string)` — multi-pin map (used by ix, net, compare)

Current map positions (all ABOVE collapsibles):
- detail_fac.templ line 55-71: MapContainer call, then collapsibles at line 72-76
- detail_ix.templ line 42: @ixFacilityMap(data), then collapsibles at line 43-47
- detail_net.templ line 72: @netFacilityMap(data), then collapsibles at line 73-77
- compare.templ line 108: @compareFacilityMap(data), then tables at line 109-111
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: Add chevron indicators to collapsible sections</name>
  <files>internal/web/templates/detail_shared.templ</files>
  <action>
In both `CollapsibleSection` and `CollapsibleSectionWithBandwidth` templates, add a chevron SVG inside the `<summary>` element that indicates the section is expandable.

The chevron should:
- Be a small right-pointing chevron (>) that rotates 90 degrees clockwise when the `<details>` is open
- Use CSS transition for smooth rotation
- Use Tailwind's group/open pattern: add `group` class to `<details>`, use `group-open:rotate-90` on the chevron wrapper
- Be placed at the LEFT side of the summary, before the title text
- Use neutral-400 color to match existing UI style

Implementation for both templates:

1. Add `group` to the `<details>` class list (alongside existing classes)

2. Inside `<summary>`, add a chevron SVG BEFORE the title `<span>`:
```
<svg class="w-4 h-4 text-neutral-400 transition-transform duration-200 group-open:rotate-90 shrink-0" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor">
  <path stroke-linecap="round" stroke-linejoin="round" d="M8.25 4.5l7.5 7.5-7.5 7.5"/>
</svg>
```

3. Update the summary flex layout: change `flex items-center justify-between` to `flex items-center gap-2` on the `<summary>`, and wrap the right-side count/bandwidth span in a `ml-auto` container (or keep justify-between and insert the chevron before the title).

Specifically, the summary structure should become:
```
<summary class="px-4 py-3 cursor-pointer flex items-center gap-2 bg-neutral-50/50 dark:bg-neutral-800/50 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors select-none">
  <svg ...chevron.../>
  <span class="font-medium text-neutral-900 dark:text-neutral-100">{ title }</span>
  <span class="ml-auto text-neutral-400 dark:text-neutral-500 text-sm font-mono">{ count }</span>
</summary>
```

Also hide the browser default disclosure triangle by adding this CSS to the summary:
- Add `[&::-webkit-details-marker]:hidden` and `list-none` classes to the `<summary>` element (templ renders Tailwind arbitrary selectors fine, but the simpler approach is to add `marker:hidden` -- however, the most reliable cross-browser way is to use `list-none` on summary which removes the default marker in modern browsers).

Apply the same changes to `CollapsibleSectionWithBandwidth` maintaining its existing bandwidth display in the right-side span.
  </action>
  <verify>
    <automated>cd /home/dotwaffle/Code/pdb/peeringdb-plus && go build ./internal/web/templates/...</automated>
  </verify>
  <done>Both CollapsibleSection and CollapsibleSectionWithBandwidth render a right-pointing chevron that rotates 90 degrees when expanded, with no browser default disclosure triangle visible.</done>
</task>

<task type="auto">
  <name>Task 2: Move maps below collapsible sections on all detail pages</name>
  <files>internal/web/templates/detail_fac.templ, internal/web/templates/detail_ix.templ, internal/web/templates/detail_net.templ, internal/web/templates/compare.templ</files>
  <action>
Move the map rendering to AFTER the collapsible sections on each page that has a map.

**detail_fac.templ:**
Move the entire map block (lines 55-71, the `if data.Latitude != 0 || data.Longitude != 0` block containing `@MapContainer(...)`) to AFTER the collapsibles `<div class="space-y-3 pt-4">...</div>` block. Keep the `<div class="pt-4">` wrapper around the map.

Result order: ...Notes -> Collapsibles (Networks, IXPs, Carriers) -> Map

**detail_ix.templ:**
Move `@ixFacilityMap(data)` (line 42) to AFTER the collapsibles `<div class="space-y-3 pt-4">...</div>` block (after line 47). Add a `<div class="pt-4">` wrapper around it for consistent spacing.

Result order: ...Notes -> Collapsibles (Participants, Facilities, Prefixes) -> Map

**detail_net.templ:**
Move `@netFacilityMap(data)` (line 72) to AFTER the collapsibles `<div class="space-y-3 pt-4">...</div>` block (after line 77). Add a `<div class="pt-4">` wrapper around it for consistent spacing.

Result order: ...Notes -> Collapsibles (IX Presences, Facility Presences, Contacts) -> Map

**compare.templ:**
Move `@compareFacilityMap(data)` (line 108) to AFTER the three comparison table sections (`@compareIXPsSection`, `@compareFacilitiesSection`, `@compareCampusesSection` at lines 109-111), but BEFORE the "New Comparison" link. Add a `<div class="pt-4">` wrapper.

Result order: ...StatBadges -> IXPs table -> Facilities table -> Campuses table -> Map -> New Comparison link

After editing the .templ files, run `go tool templ generate` to regenerate the `*_templ.go` files.
  </action>
  <verify>
    <automated>cd /home/dotwaffle/Code/pdb/peeringdb-plus && go tool templ generate && go build ./...</automated>
  </verify>
  <done>Maps render below all collapsible/table sections on fac, ix, net, and compare pages. Build succeeds with no errors.</done>
</task>

</tasks>

<verification>
1. `go tool templ generate` succeeds with no errors
2. `go build ./...` succeeds
3. `go vet ./...` passes
4. Visual check: facility, IX, network, and compare pages show collapsibles before map, and chevrons on collapsible headers
</verification>

<success_criteria>
- All maps appear below collapsible sections / comparison tables on their respective pages
- Collapsible section headers show a right-pointing chevron that rotates when expanded
- Browser default disclosure triangle is hidden
- All templ files generate successfully and project builds clean
</success_criteria>

<output>
After completion, create `.planning/quick/260331-cxk-move-maps-to-bottom-of-pages-and-add-fol/260331-cxk-SUMMARY.md`
</output>
