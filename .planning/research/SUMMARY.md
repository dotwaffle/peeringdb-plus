# Project Research Summary

**Project:** PeeringDB Plus
**Domain:** Terminal CLI interface for curl-friendly access to PeeringDB data (v1.8 milestone)
**Researched:** 2026-03-25
**Confidence:** HIGH

## Executive Summary

The v1.8 milestone adds a curl-friendly terminal interface to PeeringDB Plus, enabling network engineers to query PeeringDB data directly from the command line with zero setup -- just `curl peeringdb-plus.fly.dev/ui/asn/13335`. This is purely a presentation layer addition. The data layer, query logic, and existing API surfaces remain completely unchanged.

Two new Go dependencies are needed: `charm.land/lipgloss/v2` (v2.0.2, stable, published 2026-03-11) for terminal styling with CSS-like APIs, Unicode box-drawing borders, and table rendering; and `github.com/charmbracelet/colorprofile` (v0.4.3, published 2026-03-09) for explicit color profile control in server-side rendering. These are the dominant libraries in the Go terminal ecosystem (charmbracelet), well-maintained, MIT licensed, and designed specifically for the non-TTY rendering scenario we need.

The architecture follows the wttr.in pattern: User-Agent detection determines whether a request comes from a terminal client (curl, wget, HTTPie) or a browser. Terminal clients receive 256-color ANSI output with Unicode box-drawing tables. Browsers receive the existing HTML web UI, completely unchanged. Content negotiation happens at the handler level, after data is loaded but before rendering, branching into completely separate code paths. The existing `renderPage()` function and templ templates are untouched -- the terminal path is additive.

The primary risks are: (1) CDN/proxy cache serving the wrong format to the wrong client (mitigated by `Vary: User-Agent, Accept, HX-Request` and `Cache-Control: private` on terminal responses); (2) ANSI escape codes polluting JSON output if the mode detection happens after rendering rather than before (mitigated by detecting mode FIRST, then branching); (3) User-Agent false positives/negatives (mitigated by conservative allowlist plus explicit `?format=` overrides). All risks are well-understood with clear mitigations.

## Key Findings

**Stack:** Two new dependencies: lipgloss v2 (terminal styling + tables) and colorprofile (color profile management). Both from charmbracelet ecosystem. No other new packages needed -- User-Agent detection, content negotiation, and JSON output all use stdlib.

**Architecture:** Handler-level content negotiation. Detect output mode from User-Agent/Accept/query params, branch before rendering. New `internal/terminal/` package with renderers for each entity type. Existing data types in `detailtypes.go` are already decoupled from HTML and consumed directly by terminal renderers.

**Critical pitfall:** Cache poisoning from missing `Vary` header. Without `Vary: User-Agent`, a CDN could serve ANSI to browsers or HTML to curl. Must be set on ALL `/ui/*` responses from day one.

## Implications for Roadmap

Based on research, suggested phase structure:

1. **Terminal Detection + Infrastructure** - Content negotiation, User-Agent detection, style palette, help text, error pages
   - Addresses: All detection table stakes, help/error text, shared rendering utilities
   - Avoids: Pitfall 2 (content negotiation conflicts), Pitfall 3 (Vary header), Pitfall 9 (missed negotiation points)

2. **Network Detail (Reference Implementation)** - Network entity terminal renderer with whois-style headers and IX/fac tables
   - Addresses: Network detail (primary use case), establishes rendering patterns
   - Avoids: Pitfall 4 (width assumptions), Pitfall 10 (string allocation)

3. **Remaining Entity Types + Search** - IX, Facility, Org, Campus, Carrier renderers + search results + plain text + JSON modes
   - Addresses: All 6 entity types, search, `?T` mode, `?format=json`
   - Avoids: Pitfall 1 (ANSI in pipes via `?T`), Pitfall 8 (non-ASCII width via go-runewidth)

4. **Differentiators** - ASN comparison, one-line summary, section filtering, timestamp footer
   - Addresses: Comparison output, script-friendly modes
   - Avoids: Pitfall 13 (compare output length via summary-first design)

**Phase ordering rationale:**
- Detection infrastructure must come first -- everything depends on knowing whether the client is a terminal.
- Network detail second because it is the most complex entity and establishes patterns for all other types.
- Remaining types follow mechanically from the network pattern. Search, plain text, and JSON are added across all types simultaneously.
- Differentiators are high-value but not core. They can ship incrementally without blocking the base terminal experience.

**Research flags for phases:**
- Phase 1: Standard patterns -- User-Agent matching, header parsing. No deeper research needed.
- Phase 2: May need research on lipgloss table performance for large IX presence lists (1000+ rows). Profile before optimizing.
- Phase 3: May need research on go-runewidth for CJK character width in PeeringDB address data. Low priority (CJK rare in PeeringDB).
- Phase 4: Standard patterns for comparison rendering.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | lipgloss v2 verified at v2.0.2 (2026-03-11). colorprofile verified at v0.4.3 (2026-03-09). Both confirmed on pkg.go.dev. Import paths verified (`charm.land/lipgloss/v2`, not `github.com/charmbracelet/lipgloss`). |
| Features | HIGH | Feature landscape well-defined by wttr.in/cheat.sh precedent. All 6 entity detail types already have data structs ready. |
| Architecture | HIGH | Additive to existing patterns. Handler-level branching follows existing `renderPage()` HX-Request pattern. New `internal/terminal/` package is cleanly separated. |
| Pitfalls | HIGH | CDN cache, User-Agent detection, and ANSI-in-pipes are well-documented problems with proven mitigations from wttr.in and similar services. |

## Gaps to Address

- **lipgloss v2 import path:** The vanity domain `charm.land/lipgloss/v2` is unusual. Document clearly in the codebase. The old `github.com/charmbracelet/lipgloss` path is v1 only.
- **colorprofile pre-1.0 stability:** v0.4.3 is pre-1.0. We use minimal API surface (Profile constants, Writer type). Low risk but pin version.
- **Large table performance:** DE-CIX has 1000+ IX participants. lipgloss table rendering for 1000+ rows needs benchmarking during Phase 2. Likely fast (<100ms) but verify.
- **Non-ASCII character display width:** PeeringDB addresses contain accented Latin characters. May need `go-runewidth` for accurate column alignment. Verify with real data during Phase 3.
- **Vary header cache impact:** `Vary: User-Agent` effectively disables shared caching. Acceptable for v1.8 (no CDN layer). Revisit if CDN added.

## Sources

### Primary (HIGH confidence)
- [charm.land/lipgloss/v2 on pkg.go.dev](https://pkg.go.dev/charm.land/lipgloss/v2) -- v2.0.2, published 2026-03-11
- [charm.land/lipgloss/v2/table on pkg.go.dev](https://pkg.go.dev/charm.land/lipgloss/v2/table) -- Table rendering sub-package
- [charmbracelet/colorprofile on pkg.go.dev](https://pkg.go.dev/github.com/charmbracelet/colorprofile) -- v0.4.3, published 2026-03-09
- [Lip Gloss v2: What's New](https://github.com/charmbracelet/lipgloss/discussions/506) -- v2 migration, deterministic rendering
- [wttr.in source](https://github.com/chubin/wttr.in) -- Reference curl-friendly service, User-Agent detection
- [curl User-Agent docs](https://everything.curl.dev/http/modify/user-agent.html) -- Default UA format

### Secondary (MEDIUM confidence)
- [lipgloss GitHub releases](https://github.com/charmbracelet/lipgloss/releases) -- v2.0.2 changelog
- [User-Agent detection gist](https://gist.github.com/nahakiole/843fb9a29292bfcf012b) -- Detection pattern
- [jedib0t/go-pretty](https://github.com/jedib0t/go-pretty) -- Alternative considered, rejected
- [olekukonko/tablewriter](https://github.com/olekukonko/tablewriter) -- Alternative considered, rejected

### Existing Codebase (verified)
- `internal/web/handler.go` -- dispatch pattern, handler structure
- `internal/web/render.go` -- renderPage with HX-Request branching
- `internal/web/templates/detailtypes.go` -- shared data types (pure Go, no templ dependency)
- `cmd/peeringdb-plus/main.go` -- root handler, readiness middleware, content negotiation

---
*Research completed: 2026-03-25*
*Ready for roadmap: yes*
