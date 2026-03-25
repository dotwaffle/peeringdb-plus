# Phase 27 Context: IX Presence UI Polish

## Decisions

- **Port speed color tiers:** Sub-1G gray (`text-neutral-500`), 1G neutral (`text-neutral-400`), 10G blue (`text-blue-400`), 100G emerald (`text-emerald-400`), 400G+ amber (`text-amber-400`)
- **RS badge styling:** Emerald pill badge — `border border-emerald-400/30 rounded text-emerald-400` inline after IX name
- **Copy-to-clipboard UX:** Both click-on-IP-to-copy AND clipboard icon on hover — best discoverability
- **Aggregate bandwidth:** Short form in collapsible header ("47 IXPs — 1.2 Tbps"), detailed breakdown (3x1G, 12x10G...) on hover/tooltip
- **IX detail page:** Identical layout to network detail page — same grid, colors, labels, copy buttons for consistency
- **Stream timeout config:** `PDBPLUS_STREAM_TIMEOUT` env var (separate from UI but noted here for completeness)

## Implementation Notes

### Template Restructure
- Break the `<a>` wrapper — only IX name is a link, data fields are plain `<div>` elements for selectability
- Use CSS grid (`grid grid-cols-[auto_1fr_1fr]`) for IP address column alignment across rows
- Speed label gets color via new `speedColorClass(speed int)` helper in `detail_shared.templ`
- RS badge moves from right-aligned `shrink-0 ml-3` to inline pill after IX name with `gap-2`

### Copy-to-Clipboard
- `navigator.clipboard.writeText()` with inline `onclick` handler (no JS framework needed)
- Show clipboard icon on hover (`opacity-0 group-hover:opacity-100 transition`)
- Brief "Copied!" tooltip via CSS animation or small JS state toggle
- Requires HTTPS — already on Fly.io with TLS

### Aggregate Bandwidth
- Compute sum of speeds in the handler/fragment endpoint, pass to template
- Format with `formatSpeed()` helper (already exists for individual speeds)
- Tooltip breakdown: count per speed tier, e.g., "3x1G, 12x10G, 30x100G, 2x400G"

### Applies To Both Templates
- `internal/web/templates/detail_net.templ` — `NetworkIXLansList` component
- `internal/web/templates/detail_ix.templ` — `IXParticipantsList` component (identical layout)
- `internal/web/templates/detail_shared.templ` — `formatSpeed()` → add `speedColorClass()`, update `CollapsibleSection` for bandwidth display

## Existing Code References

- `internal/web/templates/detail_net.templ` lines 88-119 — `NetworkIXLansList`
- `internal/web/templates/detail_ix.templ` — `IXParticipantsList` (similar structure)
- `internal/web/templates/detail_shared.templ` lines 100-111 — `formatSpeed()` helper
- `internal/web/detail.go` — handler for network detail fragments
- `internal/web/handler.go` — fragment endpoint registration
