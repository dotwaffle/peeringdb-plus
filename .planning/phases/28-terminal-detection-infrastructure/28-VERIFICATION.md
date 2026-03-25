---
phase: 28-terminal-detection-infrastructure
verified: 2026-03-25T23:58:00Z
status: passed
score: 5/5 must-haves verified
re_verification: false
---

# Phase 28: Terminal Detection & Infrastructure Verification Report

**Phase Goal:** Terminal clients (curl, wget, HTTPie) hitting any /ui/ URL receive appropriate text responses instead of HTML, with explicit format overrides available
**Verified:** 2026-03-25T23:58:00Z
**Status:** passed
**Re-verification:** No -- initial verification

## Goal Achievement

### Observable Truths

Truths derived from ROADMAP.md Success Criteria (5 criteria).

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Running `curl peeringdb-plus.fly.dev/ui/` returns CLI help text listing available endpoints, query parameters, and usage examples -- not an HTML page | VERIFIED | `renderPage()` in `render.go:44-53` dispatches "Home" title to `renderer.RenderHelp()`. Integration test `TestTerminalDetection/curl_/ui/_gets_help_text` confirms response contains "PeeringDB Plus", "Usage:", "curl peeringdb-plus.fly.dev/ui/asn/" with Content-Type `text/plain`. Help text in `help.go` includes Usage, Search, Compare, Format Options, Examples sections. |
| 2 | Running `curl peeringdb-plus.fly.dev/ui/asn/13335` returns ANSI-colored text output (not HTML), while the same URL in a browser returns the existing web UI unchanged | VERIFIED | `renderPage()` branches on `termrender.Detect()` result: ModeRich/ModePlain returns text/plain, ModeHTML returns HTML with Layout wrapper. Test `TestTerminalDetection/curl_rich_mode_has_ANSI_codes` confirms `\x1b[` present in curl response. Test `TestTerminalDetection/browser_/ui/_gets_HTML` confirms `<!doctype html>` in browser response. All 6 detail handlers in `detail.go` pass `Data: data` through PageContent. |
| 3 | Appending `?T` or `?format=plain` to any /ui/ URL returns plain ASCII output with no ANSI escape codes, and `?format=json` returns JSON | VERIFIED | `Detect()` in `detect.go:69-78` checks query params first (highest priority). Tests: `TestTerminalDetection/?T_returns_plain_text_without_ANSI` confirms no `\x1b[`, `TestTerminalDetection/?format=json_returns_JSON` confirms `application/json` Content-Type, `TestTerminalDetection/?format=plain_overrides_Accept_JSON` confirms plain text overrides JSON Accept. |
| 4 | Requesting a nonexistent path like `curl /ui/asn/99999999` returns a text-formatted 404 error (not HTML), and server errors return text-formatted 500 errors | VERIFIED | `renderPage()` in `render.go:45-47` dispatches title "Not Found" to `renderer.RenderError()`. Test `TestTerminalDetection/curl_404_returns_text_error` confirms status 404, Content-Type text/plain, body contains "404 Not Found", no `<!doctype html>`. `TestTerminal404JSON` confirms JSON 404 with `?format=json`. Server Error title also dispatches to RenderError at `render.go:48-49`. |
| 5 | Setting `?nocolor` suppresses all ANSI escape codes in terminal output while preserving layout | VERIFIED | `HasNoColor()` in `detect.go:104-108` checks `?nocolor` param. `renderPage()` passes `noColor` to `NewRenderer()`. Renderer uses `colorprofile.NoTTY` when noColor is true (`renderer.go:47`). Test `TestTerminalDetection/?nocolor_strips_ANSI_from_rich_output` confirms no `\x1b[` in output while content "PeeringDB Plus" is present. |

**Score:** 5/5 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/web/termrender/detect.go` | Terminal detection priority chain | VERIFIED | 121 lines. Exports Detect, DetectInput, RenderMode, ModeHTML, ModeHTMX, ModeRich, ModePlain, ModeJSON, HasNoColor. Full priority chain: query params > Accept > UA > HX-Request > HTML. All 6 terminal UA prefixes. |
| `internal/web/termrender/detect_test.go` | Table-driven detection tests | VERIFIED | 214 lines. TestDetect (17 subtests), TestHasNoColor (3 subtests), TestIsTerminalUA (13 subtests). All use t.Parallel(). |
| `internal/web/termrender/renderer.go` | Renderer with colorprofile ANSI control | VERIFIED | 95 lines. Exports Renderer, NewRenderer, Write, Writef, RenderPage, RenderJSON. Uses colorprofile.Writer with forced profiles: ANSI256 for Rich, NoTTY for Plain/noColor. |
| `internal/web/termrender/renderer_test.go` | Renderer tests | VERIFIED | 170 lines. 8 test functions covering Rich/Plain/NoColor modes, Writef, Mode/NoColor accessors, RenderJSON, TableBorder. All use t.Parallel(). |
| `internal/web/termrender/styles.go` | lipgloss style constants and color tiers | VERIFIED | 79 lines. 5 speed color tiers, 8 general-purpose colors, 3 policy colors, 7 predefined styles, TableBorder function. lipgloss imported via vanity domain `charm.land/lipgloss/v2`. |
| `internal/web/termrender/help.go` | Help text renderer with curl examples | VERIFIED | 69 lines. RenderHelp method with Usage, Search, Compare, Format Options, Examples sections. Data freshness footer (conditional on non-zero time). |
| `internal/web/termrender/help_test.go` | Help text tests | VERIFIED | 82 lines. 3 tests: RichMode (ANSI present + content checks), PlainMode (no ANSI), ZeroTimestamp (no freshness line). |
| `internal/web/termrender/error.go` | Terminal error pages | VERIFIED | 22 lines. RenderError method with status code, title, message, and "Try: curl" hint. |
| `internal/web/termrender/error_test.go` | Error page tests | VERIFIED | 77 lines. 3 tests: 404, 500, PlainMode (no ANSI). |
| `internal/web/render.go` | renderPage with terminal detection branch | VERIFIED | 83 lines. Calls termrender.Detect, branches on ModeRich/ModePlain, ModeJSON, ModeHTMX, ModeHTML. Sets Vary header on all branches. Title-based dispatch for errors and home. PageContent has Data field. |
| `internal/web/detail.go` | All 6 entity handlers pass Data | VERIFIED | 6 occurrences of `Data: data` confirmed by grep. |
| `internal/web/handler.go` | Error handlers and home calling renderPage | VERIFIED | handleNotFound (line 132), handleServerError (line 141) both call renderPage with appropriate titles. handleHome passes Title "Home". |
| `cmd/peeringdb-plus/main.go` | Root handler with terminal detection | VERIFIED | Lines 314-352. Calls termrender.Detect, branches ModeRich/ModePlain (help text with freshness), ModeJSON (JSON discovery), default (browser redirect or JSON). |
| `internal/web/handler_test.go` | Integration tests for content negotiation | VERIFIED | TestTerminalDetection (13 subtests) and TestTerminal404JSON. Covers curl/wget/HTTPie UA detection, browser HTML, ?format=json, ?T, ?nocolor, ANSI presence/absence, 404 text/HTML, Accept header overrides, query param priority. |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `internal/web/render.go` | `internal/web/termrender` | `termrender.Detect` call | WIRED | Line 31: `mode := termrender.Detect(termrender.DetectInput{...})` |
| `internal/web/render.go` | `internal/web/termrender` | `termrender.NewRenderer` | WIRED | Line 43: `renderer := termrender.NewRenderer(mode, noColor)` |
| `internal/web/render.go` | `internal/web/termrender` | `termrender.RenderJSON` | WIRED | Lines 62, 66, 68, 70: `termrender.RenderJSON(w, ...)` |
| `internal/web/handler.go` | `internal/web/render.go` | `PageContent.Data` | WIRED | `Data: groups` in search (line 126), `Data: data` in compare (line 234) |
| `internal/web/detail.go` | `internal/web/render.go` | `PageContent.Data` | WIRED | 6 instances of `Data: data` across all entity handlers |
| `cmd/peeringdb-plus/main.go` | `internal/web/termrender` | `termrender.Detect` | WIRED | Line 315: `mode := termrender.Detect(termrender.DetectInput{...})` |
| `cmd/peeringdb-plus/main.go` | `internal/web/termrender` | `renderer.RenderHelp` | WIRED | Line 332: `renderer.RenderHelp(w, freshness)` |
| `internal/web/termrender/detect.go` | `net/url` | `url.Values` for query parsing | WIRED | DetectInput.Query is `url.Values`, used in lines 69-78 |
| `internal/web/termrender/renderer.go` | `colorprofile.Writer` | Forced ANSI profile | WIRED | Line 44: `cw := &colorprofile.Writer{Forward: w}`, forced profiles at lines 47-51 |
| `internal/web/termrender/styles.go` | `charm.land/lipgloss/v2` | Style definitions | WIRED | Import at line 6, used throughout for NewStyle, Color, Border |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|--------------|--------|-------------------|--------|
| `render.go` | `mode` (RenderMode) | `termrender.Detect(DetectInput{...})` from HTTP request headers | Yes -- live request headers | FLOWING |
| `render.go` | `page.Data` | Passed from detail/search/compare handlers | Yes -- ent DB queries in handlers | FLOWING |
| `help.go` | `freshness` (time.Time) | `pdbsync.GetLastStatus` in main.go root handler | Yes -- DB query | FLOWING |
| `help.go` | `freshness` (time.Time) | Zero value in renderPage Home branch | Static (zero time) | STATIC (by design -- omits freshness line) |
| `error.go` | `statusCode, title, message` | Hardcoded strings in renderPage | Static (intentional) | FLOWING (error content is inherently static) |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| termrender package tests pass | `go test -race ./internal/web/termrender/` | PASS -- all 43 subtests across 12 test functions | PASS |
| Integration tests pass | `go test -race ./internal/web/ -run TestTerminal` | PASS -- 13 subtests + TestTerminal404JSON | PASS |
| Full web package tests pass (no regression) | `go test -race ./internal/web/` | PASS -- all tests pass in 4.1s | PASS |
| Full project builds | `go build ./...` | PASS -- clean build | PASS |
| go vet clean | `go vet ./...` | PASS -- no issues | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-----------|-------------|--------|----------|
| DET-01 | 28-01 | Terminal clients auto-detected via User-Agent prefix matching | SATISFIED | `isTerminalUA()` matches 6 prefixes: curl/, Wget/, HTTPie/, xh/, PowerShell/, fetch. 13 test cases for UA matching. |
| DET-02 | 28-01 | User can force plain text via ?T or ?format=plain | SATISFIED | `Detect()` checks query params first. Test cases confirm ?T and ?format=plain return ModePlain. |
| DET-03 | 28-01 | User can force JSON via ?format=json | SATISFIED | `Detect()` returns ModeJSON for ?format=json. Integration test confirms application/json Content-Type. |
| DET-04 | 28-01 | Accept header serves as secondary format signal | SATISFIED | `Detect()` checks Accept after query params. Tests confirm text/plain -> ModeRich, application/json -> ModeJSON. |
| DET-05 | 28-02 | Content negotiation applies to all /ui/ paths -- browsers get HTML unchanged | SATISFIED | `renderPage()` defaults to HTML with Layout for ModeHTML. Integration test confirms browser UA gets `<!doctype html>`. |
| RND-01 | 28-01 | Rich 256-color ANSI output with Unicode box-drawing | SATISFIED | Renderer uses `colorprofile.ANSI256` for Rich mode. lipgloss styles use 256-color codes. TableBorder returns NormalBorder (Unicode) for Rich. Integration test confirms `\x1b[` in curl output. |
| RND-18 | 28-01 | NO_COLOR convention -- suppress ANSI when ?nocolor present | SATISFIED | `HasNoColor()` checks ?nocolor param. Renderer uses `colorprofile.NoTTY` when noColor=true. Integration test confirms no `\x1b[` with ?nocolor. |
| NAV-01 | 28-02 | Help text at /ui/ for terminal clients | SATISFIED | renderPage routes "Home" title to `renderer.RenderHelp()`. Help text includes Usage, Search, Compare, Format Options, Examples sections. Integration test confirms content. |
| NAV-02 | 28-02 | Text-formatted 404 error for terminal clients | SATISFIED | renderPage routes "Not Found" title to `renderer.RenderError()` with 404. Integration test confirms text/plain 404 with "Not Found". |
| NAV-03 | 28-02 | Text-formatted 500 error for terminal clients | SATISFIED | renderPage routes "Server Error" title to `renderer.RenderError()` with 500. Error page includes styled message and help hint. |
| NAV-04 | 28-03 | Root handler (/) returns help text for terminal clients | SATISFIED | `main.go` root handler calls `termrender.Detect`, returns help text for ModeRich/ModePlain with data freshness from pdbsync.GetLastStatus. |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `internal/web/termrender/renderer.go` | 70 | "Detailed terminal view coming in a future update." | Info | Intentional placeholder in `RenderPage()` for entity detail rendering. Phase 29 replaces this with entity-specific renderers. `?format=json` works as full alternative. Not a blocker for Phase 28 goals. |

### Human Verification Required

### 1. Visual Quality of Terminal Help Text

**Test:** Run `curl peeringdb-plus.fly.dev/ui/` in a 256-color terminal and visually inspect the help text formatting.
**Expected:** Headings appear in bold emerald, labels in gray, content readable with clear section separation. No garbled output.
**Why human:** ANSI color rendering quality and visual readability cannot be verified programmatically.

### 2. Root Handler Content Negotiation with Real curl

**Test:** From a terminal, run: `curl peeringdb-plus.fly.dev/` and `curl -H "Accept: text/html" peeringdb-plus.fly.dev/` and `curl -H "Accept: application/json" peeringdb-plus.fly.dev/`.
**Expected:** First returns help text, second redirects (302), third returns JSON discovery.
**Why human:** Root handler in main.go is not covered by handler_test.go's newTestMux (which only tests the web.Handler). Requires running server or a separate main.go test.

### 3. ?nocolor Preserves Layout

**Test:** Compare `curl peeringdb-plus.fly.dev/ui/` with `curl "peeringdb-plus.fly.dev/ui/?nocolor"`.
**Expected:** Same text layout with section headings, spacing, and content. Only colors stripped.
**Why human:** Layout preservation requires visual comparison; automated test only checks ANSI absence.

### Gaps Summary

No gaps found. All 5 success criteria verified. All 11 requirements satisfied. All artifacts exist, are substantive, and are properly wired. All tests pass with -race. No regressions in existing tests.

The one informational item is the `RenderPage()` placeholder message for entity detail pages, which is intentional -- Phase 29 delivers entity-specific terminal renderers. The `?format=json` path is fully functional as an alternative for entity detail data.

---

_Verified: 2026-03-25T23:58:00Z_
_Verifier: Claude (gsd-verifier)_
