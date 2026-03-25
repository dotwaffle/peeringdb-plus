# Phase 28: Terminal Detection & Infrastructure - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-25
**Phase:** 28-terminal-detection-infrastructure
**Areas discussed:** Detection priority, Rendering architecture, ANSI rendering, Help text

---

## Detection Priority Chain

| Option | Description | Selected |
|--------|-------------|----------|
| Query > UA > Accept (Recommended) | ?format=/?T always wins, then User-Agent sniffing, then Accept header | |
| Query > Accept > UA | ?format=/?T first, then Accept header, then User-Agent fallback | ✓ |
| Accept > Query > UA | Standard HTTP content negotiation first, query params override, UA last | |

**User's choice:** Query > Accept > UA
**Notes:** Accept header outranks User-Agent as it follows standard HTTP content negotiation conventions.

---

## Rendering Architecture

| Option | Description | Selected |
|--------|-------------|----------|
| Third branch in renderPage (Recommended) | Extend renderPage() with third mode. Minimal change to existing flow. | ✓ |
| New internal/termrender/ package | Separate package with Renderer interface. Cleaner separation but more plumbing. | |
| Separate handler dispatch | Terminal requests dispatch to different handler functions. Duplicates data-fetching. | |

**User's choice:** Third branch in renderPage
**Notes:** None

---

## ANSI Rendering Approach

| Option | Description | Selected |
|--------|-------------|----------|
| lipgloss/termenv library | Charm's lipgloss for styled text + table formatting. Well-maintained. | ✓ |
| Hand-rolled ANSI escapes | Own thin ANSI helper. Zero deps, full control, more code. | |
| tablewriter + raw ANSI | tablewriter for tables, hand-roll colors. Middle ground. | |

**User's choice:** lipgloss/termenv library
**Notes:** None

---

## Help Text Content

| Option | Description | Selected |
|--------|-------------|----------|
| Endpoints + examples + freshness (Recommended) | Full help with curl examples, params, and freshness. Like wttr.in. | ✓ |
| Minimal endpoint listing | Just paths and descriptions. Short. | |
| Rich ANSI-colored guide | Full colored help with sections. Verbose. | |

**User's choice:** Endpoints + examples + freshness
**Notes:** None

---

## Claude's Discretion

- Exact 256-color ANSI code mappings
- lipgloss style definitions
- Help text exact wording
- Error message wording

## Deferred Ideas

None
