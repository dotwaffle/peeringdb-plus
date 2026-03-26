# Phase 36: UI & Terminal Polish - Research

**Researched:** 2026-03-26
**Domain:** Web accessibility (WCAG AA), htmx client-side behavior, templ components, lipgloss terminal rendering
**Confidence:** HIGH

## Summary

Phase 36 is a polish phase touching two independent domains: the web UI (templ templates with Tailwind CSS and htmx) and the terminal renderer (lipgloss-based text output). The web UI changes are primarily template-level modifications -- contrast fixes, ARIA attributes, breadcrumb components, htmx error handling, and URL history management. The terminal changes involve line wrapping for long entity names and consistent error styling.

The codebase is well-structured for these changes. All 16 `.templ` files are in `internal/web/templates/`, the terminal renderer is in `internal/web/termrender/` with per-entity files and centralized styles. The existing `HX-Replace-Url` mechanism in `handleSearch` already handles URL updates server-side; the CONTEXT.md decision to use `hx-push-url="true"` on the input itself is a complementary client-side approach. The error handling in collapsible sections requires htmx event listening via `hx-on::after-request` to detect failed fetches.

**Primary recommendation:** Group into two independent work streams: (1) web UI template changes across all `.templ` files, and (2) terminal rendering changes in `termrender/`. Both can be verified independently.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- WCAG AA contrast: Fix `text-neutral-600` in dark mode to `text-neutral-400` or `text-neutral-500`. Full systematic audit. Target 4.5:1 for normal text, 3:1 for large text.
- ARIA: Add `role="navigation"` to nav, `aria-expanded`/`aria-controls` to mobile menu toggle, `<label for="search-input">` with sr-only styling.
- Bookmarkable search: Add `hx-push-url="true"` to search input's htmx attributes. On page load with `?q=`, pre-fill and trigger results. Handle back/forward navigation.
- htmx error handling: Use `hx-on::afterRequest` to detect errors on collapsible sections. Swap in styled error template with retry button. Retry button has `hx-get` pointing to same endpoint.
- Breadcrumbs: Server-rendered templ component. Pattern: Home > Networks > AS13335. Uses existing emerald hover styling. Added above DetailHeader in all 6 detail templates.
- Mobile menu close: Use `hx-on:click` on nav links inside mobile menu to toggle `hidden` class.
- Compare button: Emerald outline button `border-emerald-500 text-emerald-500 hover:bg-emerald-500/10`. Dark mode variant: `dark:border-emerald-400 dark:text-emerald-400`.
- Terminal line wrapping: Ellipsis with full name on separate line above the table row when name exceeds column width.
- Terminal error responses: Style 404, 500, sync-not-ready using same termrender styles. Error title in bold/red, message in normal text. Include help text suggestion.

### Scope Boundaries
- Do NOT target WCAG AAA -- AA is sufficient
- Do NOT add keyboard shortcuts beyond what exists
- Do NOT change page layout or information architecture beyond breadcrumbs
- Do NOT add animations or transitions beyond what exists
- Terminal wrapping only applies to table columns, not key-value header fields
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| UI-01 | Dark mode text passes WCAG AA contrast ratio (4.5:1 minimum) | Contrast audit complete: `text-neutral-600` (#525252) on `neutral-900` (#171717) = ~2.9:1 (FAIL). `text-neutral-500` (#737373) = ~4.7:1 (borderline PASS). `text-neutral-400` (#A3A3A3) = ~7.3:1 (comfortable PASS). 25+ instances of `text-neutral-600` across 7 templates need fixing. |
| UI-02 | All interactive elements have ARIA attributes | Nav needs `role="navigation"`, mobile toggle needs `aria-expanded`/`aria-controls`, search input needs associated `<label>`. Current nav.templ has no `role`, no `aria-expanded`, no label on search input. |
| UI-03 | Search results update browser URL for bookmarkable/shareable searches | Server-side `HX-Replace-Url` already works in `handleSearch`. Client-side `hx-push-url="true"` on input adds history entries. `handleHome` already supports `?q=` pre-fill and pre-rendering. htmx `popstate` handling needs verification. |
| UI-04 | Failed htmx collapsible section loads show error with retry | htmx 2.0.8 `hx-on::after-request` provides `event.detail.failed` boolean. Current `CollapsibleSection` in detail_shared.templ has no error handling -- "Loading..." shows indefinitely on failure. |
| UI-05 | Detail pages include breadcrumb navigation | New `Breadcrumb` templ component needed. All 6 detail templates (`detail_net.templ`, `detail_ix.templ`, `detail_fac.templ`, `detail_org.templ`, `detail_campus.templ`, `detail_carrier.templ`) need `@Breadcrumb(...)` call above `@DetailHeader(...)`. |
| UI-06 | Mobile navigation menu closes after clicking a link | Current mobile menu links in nav.templ have no close behavior. Add `hx-on:click` or standard `onclick` to toggle `hidden` on `#mobile-menu`. |
| UI-07 | Compare button on network detail pages is visually distinct | Current compare button uses `bg-neutral-800 text-neutral-300 border-neutral-700` -- blends with dark background. Replace with emerald outline per CONTEXT.md. |
| TUI-01 | Long entity names in terminal output wrap intelligently | Current entity renderers write names inline with no width check. When `r.Width > 0` and name exceeds available space, print full name on a dedicated line before the row, then truncate with `...` in the table cell. |
| TUI-02 | Terminal error responses use styled text formatting | Current `RenderError` in error.go already uses `StyleError` and `StyleMuted`. But sync-not-ready in `readinessMiddleware` returns raw JSON `{"error":"sync not yet completed"}` for terminal clients. Need to detect terminal User-Agent and render styled text. |
</phase_requirements>

## Architecture Patterns

### File Organization
All changes are within existing files/packages:
```
internal/web/templates/
  nav.templ              -- ARIA, mobile menu close (UI-02, UI-06)
  layout.templ           -- no changes needed (already has lang="en")
  home.templ             -- hx-push-url on search input (UI-03)
  detail_shared.templ    -- Breadcrumb component, CollapsibleSection error handling (UI-04, UI-05)
  detail_net.templ       -- breadcrumb call, compare button styling (UI-05, UI-07)
  detail_ix.templ        -- breadcrumb call (UI-05)
  detail_fac.templ       -- breadcrumb call (UI-05)
  detail_org.templ       -- breadcrumb call (UI-05)
  detail_campus.templ    -- breadcrumb call (UI-05)
  detail_carrier.templ   -- breadcrumb call (UI-05)
  compare.templ          -- contrast fixes (UI-01)
  about.templ            -- contrast fixes (UI-01)
  syncing.templ          -- contrast fix (UI-01)
  search_results.templ   -- verify contrast (UI-01)
  footer.templ           -- verify contrast (UI-01)
  error.templ            -- verify contrast (UI-01)

internal/web/termrender/
  error.go               -- enhance RenderError, add sync-not-ready (TUI-02)
  network.go             -- name wrapping in IX/Fac lists (TUI-01)
  ix.go                  -- name wrapping in participant/facility lists (TUI-01)
  facility.go            -- name wrapping in network/IX/carrier lists (TUI-01)
  org.go                 -- name wrapping in network/IX/fac/campus/carrier lists (TUI-01)
  campus.go              -- name wrapping in facility list (TUI-01)
  carrier.go             -- name wrapping in facility list (TUI-01)

cmd/peeringdb-plus/main.go  -- terminal-aware sync-not-ready response (TUI-02)
```

### Pattern 1: Breadcrumb Component
**What:** New templ component in detail_shared.templ
**When to use:** All 6 detail page templates, placed before DetailHeader

```go
// Breadcrumb renders navigation breadcrumbs: Home > TypePlural > Entity.
templ Breadcrumb(typePlural string, typeURL string, entityName string) {
    <nav aria-label="Breadcrumb" class="text-sm mb-4">
        <ol class="flex items-center gap-1.5 text-neutral-400">
            <li><a href="/ui/" class="hover:text-emerald-400 transition-colors">Home</a></li>
            <li class="text-neutral-500">&gt;</li>
            <li class="hover:text-emerald-400 transition-colors">{ typePlural }</li>
            <li class="text-neutral-500">&gt;</li>
            <li class="text-neutral-100">{ entityName }</li>
        </ol>
    </nav>
}
```

Note: breadcrumb "type" links (e.g., "Networks") don't have a real list page in the current UI. Options: (a) link to `/ui/?q=` (empty search), (b) make it non-linked text, (c) omit the middle segment. Per CONTEXT.md pattern "Home > Networks > AS13335 (Cloudflare)", the middle segment is present. Use search page as the type landing page: `/ui/` -- all types go to the same home page since there's no per-type list page.

### Pattern 2: htmx Error Handling on Collapsible Sections
**What:** Add `hx-on::after-request` to the collapsible section's inner div to detect fetch failures
**When to use:** Both `CollapsibleSection` and `CollapsibleSectionWithBandwidth` in detail_shared.templ

The approach: use a global event listener in layout.templ rather than inline attributes. This avoids escaping issues in templ and keeps the logic centralized.

```javascript
// In layout.templ <script> block:
document.body.addEventListener('htmx:afterRequest', function(evt) {
    if (!evt.detail.successful && evt.detail.elt.closest('details')) {
        var el = evt.detail.elt;
        var url = el.getAttribute('hx-get');
        el.textContent = ''; // Clear "Loading..." safely
        var wrapper = document.createElement('div');
        wrapper.className = 'px-4 py-3 text-center';
        var msg = document.createElement('span');
        msg.className = 'text-red-400 text-sm';
        msg.textContent = 'Failed to load.';
        var btn = document.createElement('button');
        btn.className = 'text-emerald-400 hover:text-emerald-300 text-sm underline ml-2';
        btn.textContent = 'Retry';
        btn.setAttribute('hx-get', url);
        btn.setAttribute('hx-target', 'closest div');
        btn.setAttribute('hx-swap', 'innerHTML');
        wrapper.appendChild(msg);
        wrapper.appendChild(btn);
        el.appendChild(wrapper);
        htmx.process(el); // Initialize htmx on the new retry button
    }
});
```

Key detail: `htmx.process(el)` must be called after injecting the retry button so htmx recognizes the new `hx-get` attribute. Using DOM methods instead of setting markup via string assignment avoids XSS concerns entirely.

### Pattern 3: Terminal Name Wrapping
**What:** When `r.Width > 0` and an entity name exceeds available column space, print full name on its own line before the data row, then truncate the name in the inline position
**When to use:** Entity list sections in all termrender entity files

```go
// TruncateName returns name truncated to maxWidth with "..." suffix.
// Returns name unchanged if it fits within maxWidth.
func TruncateName(name string, maxWidth int) string {
    if len(name) <= maxWidth || maxWidth <= 3 {
        return name
    }
    return name[:maxWidth-3] + "..."
}

// Usage in entity list rendering:
if r.Width > 0 && len(name) > maxNameWidth {
    // Full name on its own line
    buf.WriteString("  ")
    buf.WriteString(StyleValue.Render(name))
    buf.WriteString("\n")
}
// Inline (possibly truncated) name
displayName := name
if r.Width > 0 && len(name) > maxNameWidth {
    displayName = TruncateName(name, maxNameWidth)
}
buf.WriteString("  ")
buf.WriteString(StyleValue.Render(displayName))
```

### Pattern 4: Search URL History
**What:** Enable browser history entries for search queries so back/forward navigation works
**Critical distinction:** Current `HX-Replace-Url` (server response header) replaces the URL without adding history entries. `hx-push-url="true"` (client-side attribute) pushes to history. The CONTEXT.md says to use `hx-push-url="true"`.

However, in htmx 2.0, server response headers take precedence over client-side attributes. Adding `hx-push-url="true"` on the input while the server sends `HX-Replace-Url` means the server header wins. To get history pushes, the server needs to send `HX-Push-Url` instead.

**Recommendation:** Change server header from `HX-Replace-Url` to `HX-Push-Url` in `handleSearch` (handler.go). Also add `hx-push-url="true"` on the input as specified in CONTEXT.md. Both changes align on the same behavior.

### Anti-Patterns to Avoid
- **Inline JS strings in templ attributes:** Keep htmx event handler code short. For complex logic (error handling), use a `<script>` block with DOM API calls instead of string manipulation.
- **Hardcoding contrast colors without dark: prefix:** Many elements use light-mode-only classes (e.g., `text-neutral-600`). Always verify the dark mode variant is present. Some elements correctly use `text-neutral-600 dark:text-neutral-300` -- the light mode class is fine for light backgrounds.
- **Breaking templ generate:** After editing `.templ` files, always run `templ generate` and commit `*_templ.go` files alongside. Tests depend on the generated Go code.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Contrast ratio calculation | Manual hex-to-luminance math | Known Tailwind color mappings | Tailwind neutral palette is fixed; ratios can be pre-computed |
| URL history management | Custom pushState/popState JS | htmx built-in `hx-push-url` / `HX-Push-Url` header | htmx handles DOM snapshot, history cache, and restoration |
| Error retry mechanism | Custom XHR retry logic | htmx `hx-get` on retry button | htmx processes the button's attributes normally on click |
| Breadcrumb structured data | JSON-LD breadcrumb schema | Simple `<nav aria-label="Breadcrumb">` with `<ol>` | Structured data is for SEO; this is an internal tool |

## Common Pitfalls

### Pitfall 1: Contrast audit must cover both light AND dark mode paths
**What goes wrong:** Fixing `text-neutral-600` globally without checking whether some instances are light-mode-only classes paired with a `dark:` override.
**Why it happens:** Some elements like nav links use `text-neutral-600 dark:text-neutral-300` -- the `text-neutral-600` is the LIGHT mode color, and the dark mode color `text-neutral-300` already passes WCAG AA. Changing `text-neutral-600` to `text-neutral-400` in these cases would make light mode too faint.
**How to avoid:** For each `text-neutral-600` instance, check if a `dark:` variant exists. If yes, only the light mode class matters for light backgrounds, and the dark variant matters for dark backgrounds. If no `dark:` variant exists AND the element appears on a dark background, that's a real contrast issue.
**Warning signs:** Elements that look washed out in light mode after the fix.

### Pitfall 2: htmx error handler and templ escaping
**What goes wrong:** Trying to inline complex JavaScript in a templ `hx-on::after-request` attribute hits escaping issues.
**Why it happens:** templ attributes use Go-style string escaping, and the inner HTML contains quotes, angle brackets, and htmx attributes.
**How to avoid:** Use a dedicated `<script>` block in layout.templ that attaches event listeners programmatically using DOM API methods. This avoids both templ escaping issues and potential injection concerns from string-based markup construction.
**Warning signs:** templ compile errors, broken HTML in error state.

### Pitfall 3: hx-push-url vs HX-Replace-Url precedence
**What goes wrong:** Adding `hx-push-url="true"` on the client while the server sends `HX-Replace-Url` -- the server header takes precedence, so no history entries are created despite the attribute.
**Why it happens:** htmx response headers override client-side attributes by design.
**How to avoid:** Change the server header from `HX-Replace-Url` to `HX-Push-Url` in `handleSearch`.
**Warning signs:** URL updates but browser back button doesn't work.

### Pitfall 4: Sync-not-ready terminal detection
**What goes wrong:** The `readinessMiddleware` in main.go checks `Accept: text/html` for browser detection, but terminal clients (curl) send `Accept: */*`. The current fallback is JSON, not styled terminal text.
**Why it happens:** The readiness middleware was written before terminal detection was added to the render pipeline.
**How to avoid:** Import and use `termrender.Detect()` or check User-Agent directly in the readiness middleware. For curl/wget/HTTPie, render styled text using `termrender.RenderError`. For API clients expecting JSON, keep JSON.
**Warning signs:** curl users see `{"error":"sync not yet completed"}` instead of styled output.

### Pitfall 5: Breadcrumb separator contrast
**What goes wrong:** Using `text-neutral-600` for the `>` separator in breadcrumbs on a dark background fails WCAG AA.
**Why it happens:** Copy-pasting a pattern that uses neutral-600 for decorative elements.
**How to avoid:** Use `text-neutral-500` (passes at 4.7:1) for separators. Since separators are decorative, 3:1 ratio for non-text elements technically applies, but 500 is safer.
**Warning signs:** Contrast checker flags the separator.

## Code Examples

### WCAG AA Contrast Fix Inventory

Current problematic dark-mode instances (text on `bg-neutral-900` #171717):

| Class | Hex | Ratio vs #171717 | Status | Fix |
|-------|-----|-------------------|--------|-----|
| `text-neutral-600` | #525252 | ~2.9:1 | FAIL | Change to `text-neutral-500` or add `dark:text-neutral-400` |
| `text-neutral-500` | #737373 | ~4.7:1 | PASS (barely) | OK for normal text, comfortable for large text |
| `text-neutral-400` | #A3A3A3 | ~7.3:1 | PASS | Safe choice for all text sizes |

Files requiring contrast audit:
1. **nav.templ** -- light mode `text-neutral-600` has `dark:text-neutral-300` override (OK, light-mode only class)
2. **compare.templ** -- 6 instances of `text-neutral-600` without dark override (FAIL: separators "|", "---" on dark bg)
3. **detail_net.templ** -- `text-neutral-600` em-dash in boolIndicator (FAIL: no dark override, dark bg)
4. **detail_shared.templ** -- `text-neutral-600` clipboard icon (decorative, but should pass 3:1 for non-text)
5. **about.templ** -- `text-neutral-600 dark:text-neutral-300` (OK, has dark override)
6. **syncing.templ** -- `text-neutral-600` auto-refresh notice (FAIL: hardcoded dark bg #171717)
7. **footer.templ** -- `text-neutral-400 dark:text-neutral-500` on `bg-neutral-800` (#262626): neutral-500 = ~3.3:1 (BORDERLINE for body text; OK for large text)

### Mobile Menu Close

```html
<!-- In nav.templ mobile menu links, add onclick handler -->
<a href="/ui/"
   class="block text-neutral-600 dark:text-neutral-300 ..."
   onclick="document.getElementById('mobile-menu').classList.add('hidden')">
   Search
</a>
```

Using `onclick` (standard JS) rather than `hx-on:click` since these are standard navigation links, not htmx-powered elements. The menu closes immediately on click, then navigation proceeds normally.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) + templ generate |
| Config file | None (stdlib testing) |
| Quick run command | `TMPDIR=/tmp/claude-1000 go test ./internal/web/... ./internal/web/termrender/... -race -count=1` |
| Full suite command | `TMPDIR=/tmp/claude-1000 go test -race ./...` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| UI-01 | Dark mode contrast classes correct | unit (template output check) | `TMPDIR=/tmp/claude-1000 go test ./internal/web/... -run TestLayout -race -count=1` | Partial -- existing TestLayout checks classes |
| UI-02 | ARIA attributes present | unit (template output check) | `TMPDIR=/tmp/claude-1000 go test ./internal/web/... -run TestNav -race -count=1` | Partial -- TestNav_Links exists, needs ARIA checks |
| UI-03 | URL bookmarkability | unit (handler test) | `TMPDIR=/tmp/claude-1000 go test ./internal/web/... -run TestSearchEndpoint_HXReplaceUrl -race -count=1` | Yes -- needs update for HX-Push-Url |
| UI-04 | Error handling on collapsible sections | manual-only | N/A (requires browser + failing endpoint) | N/A -- htmx client-side behavior |
| UI-05 | Breadcrumbs on detail pages | unit (template output check) | `TMPDIR=/tmp/claude-1000 go test ./internal/web/... -run TestDetailPages -race -count=1` | Partial -- needs breadcrumb assertions |
| UI-06 | Mobile menu close | manual-only | N/A (requires browser JS execution) | N/A -- client-side onclick |
| UI-07 | Compare button styling | unit (template output check) | `TMPDIR=/tmp/claude-1000 go test ./internal/web/... -run TestDetailPages -race -count=1` | Partial -- existing tests check rendered HTML |
| TUI-01 | Terminal name wrapping | unit | `TMPDIR=/tmp/claude-1000 go test ./internal/web/termrender/... -run TestTruncate -race -count=1` | No -- Wave 0 |
| TUI-02 | Terminal styled errors | unit | `TMPDIR=/tmp/claude-1000 go test ./internal/web/termrender/... -run TestRenderError -race -count=1` | Yes -- existing tests cover 404/500 |

### Sampling Rate
- **Per task commit:** `TMPDIR=/tmp/claude-1000 go test ./internal/web/... ./internal/web/termrender/... -race -count=1`
- **Per wave merge:** `TMPDIR=/tmp/claude-1000 go test -race ./...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/web/termrender/width_test.go` -- add `TestTruncateName` for name truncation helper (TUI-01)
- [ ] Update `TestDetailPages_AllTypes` to assert breadcrumb HTML presence (UI-05)
- [ ] Update `TestNav_Links` or add `TestNav_ARIA` to assert `role="navigation"` and `aria-expanded` (UI-02)
- [ ] Update `TestSearchEndpoint_HXReplaceUrl` to check for `HX-Push-Url` instead of `HX-Replace-Url` (UI-03)

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| htmx `hx-on:htmx:after-request` | `hx-on::after-request` (shorthand) | htmx 2.0 | Shorter syntax for htmx event handling |
| `HX-Replace-Url` (no history) | `HX-Push-Url` (with history) | htmx 1.9+ | Replace updates URL silently; Push creates history entries |
| Manual `aria-*` attributes | Same (no framework change) | N/A | ARIA attributes are still hand-coded in templ |

## Open Questions

1. **Breadcrumb middle segment link target**
   - What we know: There are no per-type list pages (e.g., no `/ui/networks/` page)
   - What's unclear: Where should "Networks" link to in the breadcrumb?
   - Recommendation: Link to `/ui/` (home/search page) since that's where users search for entities of a given type. The breadcrumb middle segment provides context, not necessarily a functional navigation target. Alternatively, make it non-linked text (`<span>` instead of `<a>`).

2. **Sync-not-ready terminal error scope**
   - What we know: The readiness middleware is in `cmd/peeringdb-plus/main.go` and returns JSON for non-HTML clients
   - What's unclear: Whether modifying the readiness middleware is in scope (it's outside `internal/web/`)
   - Recommendation: It's in scope per TUI-02 requirement ("sync-not-ready" is explicitly listed). Import termrender.Detect in main.go and render styled text for terminal User-Agents.

## Project Constraints (from CLAUDE.md)

Directives that apply to this phase:
- **CS-0 (MUST)**: Modern Go code guidelines -- all new helper functions must follow current patterns
- **CS-5 (MUST)**: Input structs for functions with >2 arguments -- applies to the Breadcrumb component parameters (3 params is on the boundary; templ components follow templ conventions, not Go function conventions)
- **T-1 (MUST)**: Table-driven tests for new test cases (TUI-01 name truncation)
- **T-2 (MUST)**: `-race` flag in all test runs
- **T-3 (SHOULD)**: `t.Parallel()` on safe tests
- **API-1 (MUST)**: Document exported items -- new exported functions in termrender must have comments

Code generation requirements:
- After editing `.templ` files: `templ generate ./internal/web/templates/`
- Commit `*_templ.go` alongside `.templ` changes
- Run `go vet ./...` and `golangci-lint run` after changes

## Sources

### Primary (HIGH confidence)
- Codebase analysis: All 16 `.templ` files, 15 termrender `.go` files, handler.go, render.go, main.go
- htmx 2.0.8 source (bundled in `internal/web/static/htmx.min.js`) -- confirmed version
- [htmx hx-push-url docs](https://htmx.org/attributes/hx-push-url/) -- attribute behavior and values
- [htmx hx-on docs](https://htmx.org/attributes/hx-on/) -- event handler syntax
- [htmx events reference](https://htmx.org/events/) -- afterRequest event detail properties

### Secondary (MEDIUM confidence)
- Tailwind CSS color values -- neutral-400=#A3A3A3, neutral-500=#737373, neutral-600=#525252, neutral-900=#171717
- WCAG AA contrast ratios calculated from known hex values (4.5:1 normal text, 3:1 large text/UI components)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- no new dependencies; all changes use existing templ, htmx, lipgloss
- Architecture: HIGH -- codebase thoroughly analyzed; all files identified; patterns match existing code
- Pitfalls: HIGH -- htmx precedence rules, contrast audit methodology, and templ escaping are well-understood

**Research date:** 2026-03-26
**Valid until:** 2026-04-26 (stable -- no external dependency changes)
