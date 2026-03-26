# Phase 36 Context: UI & Terminal Polish

## Requirements
- **UI-01**: WCAG AA contrast in dark mode (4.5:1 minimum)
- **UI-02**: ARIA attributes on all interactive elements
- **UI-03**: Bookmarkable search results (URL history push)
- **UI-04**: htmx error handling with retry on collapsible sections
- **UI-05**: Breadcrumb navigation on detail pages
- **UI-06**: Mobile menu auto-close
- **UI-07**: Compare button visual distinction
- **TUI-01**: Terminal line wrapping for long names
- **TUI-02**: Styled terminal error responses

## Decisions

### WCAG AA Fixes (UI-01)
- Fix known issue: `text-neutral-600` in dark mode (~3:1 ratio) -> use `text-neutral-400` or `text-neutral-500`
- Full systematic audit of all templates for contrast issues
- Check all color combinations: text on bg, badges, links, borders
- Target 4.5:1 for normal text, 3:1 for large text (WCAG AA)

### ARIA Attributes (UI-02)
- Add `role="navigation"` to nav
- Add `aria-expanded` and `aria-controls` to mobile menu toggle button
- Add `<label for="search-input">` with sr-only styling to search input
- Check all interactive elements: buttons, links, form controls

### Bookmarkable Search (UI-03)
- Add `hx-push-url="true"` to search input's htmx attributes
- URL updates to `/ui/?q=equinix` as user types
- On page load with `?q=` param, pre-fill search and trigger results
- Handle back/forward navigation correctly

### htmx Error Handling (UI-04)
- Use `hx-on::afterRequest` to detect errors on collapsible sections
- On error: swap in styled error template with retry button
- Retry button has `hx-get` pointing to same endpoint, `hx-swap="innerHTML"`
- Error template shows brief message: "Failed to load. [Retry]"

### Breadcrumbs (UI-05)
- Server-rendered templ component
- Receives entity type + name from handler
- Pattern: Home > Networks > AS13335 (Cloudflare)
- Uses existing link styling (emerald hover)
- Added above DetailHeader in all 6 detail templates

### Mobile Menu Close (UI-06)
- Use `hx-on:click` on nav links inside mobile menu
- Toggle the `hidden` class on the mobile menu element
- Simple, no new dependencies

### Compare Button (UI-07)
- Emerald outline button: `border-emerald-500 text-emerald-500 hover:bg-emerald-500/10`
- Applied on network detail pages where "Compare with..." appears
- Dark mode variant: `dark:border-emerald-400 dark:text-emerald-400`

### Terminal Line Wrapping (TUI-01)
- Ellipsis with full name on separate line above the table row
- When entity name exceeds column width: truncate with '...' in the table cell
- Show full name on a dedicated line before the row: `  Cloudflare Systems Inc International`
- Only triggers when name > column width

### Terminal Error Responses (TUI-02)
- Style 404, 500, sync-not-ready errors using the same termrender styles as normal output
- Error title in bold/red, message in normal text
- Include help text suggestion: "Try: curl /ui/ for available endpoints"
- Consistent across all terminal error paths

## Scope Boundaries
- Do NOT target WCAG AAA — AA is sufficient
- Do NOT add keyboard shortcuts beyond what exists (search already has arrow/enter/escape)
- Do NOT change page layout or information architecture beyond breadcrumbs
- Do NOT add animations or transitions beyond what exists
- Terminal wrapping only applies to table columns, not key-value header fields
