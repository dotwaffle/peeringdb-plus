# Phase 17: Polish & Accessibility — Discussion Context

**Gathered:** 2026-03-24

## Decisions

### Dark Mode
- **Toggle location**: Sun/moon icon in the navigation bar, always visible.
- **Default**: System preference via `prefers-color-scheme` media query.
- **Override**: Manual toggle persisted in `localStorage`.
- **Implementation**: Tailwind `dark:` variant with class-based dark mode (`class="dark"` on `<html>`).

### About Page
- **Content**: Project description ("what is PeeringDB Plus"), links to all 3 API surfaces (GraphQL playground, REST docs, PeeringDB compat), link to GitHub repo, and **live data freshness indicator** showing when data was last synced.
- **Data freshness**: Query sync_status table for last sync timestamp, display "Last synced: X minutes ago" or similar.
- **URL**: `/ui/about`

### Error Pages
- **404**: Styled page matching overall design, includes a search box so users can immediately search for what they were looking for. "Page not found — try searching instead."
- **500**: Styled page matching overall design, apologetic message, link to homepage.
- Both use the same base layout (header, nav, footer) as all other pages.

### Keyboard Navigation
- **Search results**: Arrow keys (Up/Down) to move between results, Enter to navigate to selected result.
- **ARIA roles**: `role="listbox"` on results container, `role="option"` on each result, `aria-selected` on focused result.
- **Focus management**: Search box gets focus on page load. Arrow keys move focus to results. Escape returns focus to search box.

### Transitions
- **Search results**: Fade-in when results appear/update.
- **Collapsible sections**: Smooth height transition on expand/collapse.
- **Page transitions**: Subtle fade on htmx page swaps.
- **Loading indicators**: Spinner or skeleton during htmx requests (`htmx:beforeRequest` / `htmx:afterRequest` events, or `hx-indicator` attribute).

### Visual Polish
- Monospace font for data values: ASNs, IP addresses, port speeds, prefixes.
- Sans-serif for UI labels and navigation.
- Consistent spacing, rounded corners on cards/badges.
- Hover effects on interactive elements.
- Responsive: works on mobile, tablet, desktop.
