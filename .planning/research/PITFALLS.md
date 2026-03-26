# Domain Pitfalls

**Domain:** Terminal CLI interface for curl-friendly access to PeeringDB data
**Researched:** 2026-03-25
**Milestone:** v1.8 Terminal CLI Interface

## Critical Pitfalls

Mistakes that cause broken output for users, content negotiation conflicts with existing API surfaces, or require rework of the middleware chain.

### Pitfall 1: ANSI Escape Codes in Non-Terminal Pipes Produce Garbage

**What goes wrong:** A user runs `curl https://peeringdb-plus.fly.dev/ui/asn/13335 | grep peering` and gets output littered with raw escape sequences like `[38;5;33m` mixed into every line, making grep/awk/sed results unusable.

**Why it happens:** The server has no way to know whether the client's stdout is a terminal or a pipe. Unlike a local CLI tool which can call `isatty(stdout)`, an HTTP server only sees the HTTP request -- it cannot detect whether `curl` is writing to a terminal or to a pipe/file. The decision to send ANSI codes is made server-side at request time, but the client's output destination is determined client-side at render time.

**Consequences:** Users who pipe output through text-processing tools get corrupted data. Users who redirect to files get files full of escape sequences. This is the single most common complaint about curl-friendly terminal services.

**Prevention:**
- Default to ANSI-colored output (the common case is direct terminal viewing).
- Provide an explicit plain-text mode via query parameter (`?T` or `?format=plain`) that strips all ANSI codes. Document this prominently in the CLI help text.
- Consider also supporting `?format=json` for programmatic consumers who want structured data.
- The `?T` parameter follows wttr.in convention and is easy to type: `curl peeringdb-plus.fly.dev/ui/asn/13335?T | grep peering`.
- Do NOT attempt server-side pipe detection. It is fundamentally impossible over HTTP.

**Detection:** Test by piping curl output through `cat -v` or `grep`. If escape sequences appear as `^[[` character sequences, ANSI stripping is needed.

### Pitfall 2: Content Negotiation Conflict with Existing Accept Header Logic

**What goes wrong:** The root handler (`GET /`) and readiness middleware already branch on `Accept: text/html` to distinguish browsers from API clients. Adding User-Agent-based terminal detection creates a second, conflicting negotiation axis. A curl request with `Accept: text/html` (which curl does NOT send by default, but some wrapper scripts do) could match both the "browser" path and the "terminal" path, producing inconsistent behavior.

**Why it happens:** The codebase has two different detection strategies for the same fundamental question ("is this a browser or a programmatic client?"):
1. **Existing (main.go:313, main.go:390):** `Accept` header contains `text/html` => browser path.
2. **New (v1.8):** `User-Agent` contains `curl/`, `wget/`, etc. => terminal path.

These can conflict when a client sends both `Accept: text/html` AND has a terminal User-Agent (e.g., `curl -H "Accept: text/html"`), or when a non-terminal client omits Accept headers.

**Consequences:** Some requests route to the wrong renderer. The readiness middleware returns HTML to curl when syncing (because curl with `-H "Accept: text/html"` triggers the HTML path). The root handler redirects curl to `/ui/` when the user wanted the JSON discovery document.

**Prevention:**
- Establish a clear priority: User-Agent detection takes precedence over Accept header for terminal-vs-browser decisions. If User-Agent matches a known terminal client, always use terminal rendering regardless of Accept headers.
- Update the readiness middleware to return plain text "sync not yet completed" for terminal clients (not JSON, not HTML).
- Update the root handler to return CLI help text for terminal clients (not redirect to /ui/, not JSON discovery).
- Centralize the terminal detection logic into a single function (e.g., `isTerminalClient(r *http.Request) bool`) used by ALL content negotiation points. Do not duplicate detection logic across handlers.
- Document the full decision tree: terminal User-Agent => terminal output; Accept: text/html => browser HTML; default => JSON.

**Detection:** Write integration tests that combine terminal User-Agents with various Accept headers and verify correct response Content-Type in every combination.

### Pitfall 3: Vary Header Incorrectly Set, Breaking Caches

**What goes wrong:** The existing `renderPage()` sets `Vary: HX-Request` for htmx fragment caching. Adding terminal detection requires varying on `User-Agent` as well. If the Vary header is set to `User-Agent` alone (overwriting `HX-Request`), htmx fragment caching breaks. If set to `Vary: User-Agent`, CDNs and intermediate caches treat it as effectively `Vary: *` because User-Agent has thousands of unique values, destroying cache hit rates.

**Why it happens:** `Vary: User-Agent` tells caches to store a separate copy for every distinct User-Agent string. Since User-Agent strings include version numbers (e.g., `curl/8.7.1` vs `curl/8.8.0`), this fragments the cache into thousands of entries that are never reused. Many CDNs (Cloudflare, Fastly) interpret `Vary: User-Agent` as uncacheable.

**Consequences:** Either cache pollution (thousands of near-identical entries) or total cache bypass (CDN treats as uncacheable). For an edge-deployed app on Fly.io, this matters because Fly.io's HTTP proxy may cache responses.

**Prevention:**
- Do NOT use `Vary: User-Agent`. Instead, normalize the detection result into a custom header or a binary classification.
- Set `Vary: HX-Request, X-Output-Format` where `X-Output-Format` is a response header you control, set to `terminal` or `html`. This keeps the Vary space small (2 x 2 = 4 variants max).
- Alternatively, since terminal responses are for programmatic clients that rarely benefit from caching, set `Cache-Control: no-store` on terminal responses and keep the existing Vary for browser responses.
- The simplest correct approach: terminal responses get `Cache-Control: no-store` with no Vary header. Browser responses keep the existing `Vary: HX-Request`.

**Detection:** Test with `curl -v` and inspect Vary and Cache-Control response headers for both terminal and browser requests.

### Pitfall 4: Terminal Width Assumption Breaks Wide Tables

**What goes wrong:** Network IX presence tables in PeeringDB can be very wide -- columns for IX name (up to ~40 chars), ASN (up to 10 chars), port speed, IPv4, IPv6 (up to 39 chars each for full IPv6 addresses), RS status, and operational status. A table rendered at 120 columns wraps catastrophically on an 80-column terminal, producing unreadable output where box-drawing characters no longer align.

**Why it happens:** The server has no way to know the client's terminal width. Unlike SSH (which transmits terminal dimensions in the pty-req), HTTP has no mechanism for communicating terminal geometry. The server must pick a width and hope.

**Consequences:** On narrow terminals (80 columns, common on default macOS Terminal, many Linux distributions), tables wrap mid-row, breaking Unicode box-drawing alignment. On very wide terminals (200+ columns), output looks needlessly cramped. IPv6 addresses alone consume 39 characters per column.

**Prevention:**
- Design for 80 columns as the floor. All table layouts must render correctly at 80 columns.
- Truncate long fields with ellipsis rather than wrapping. Network names beyond ~25 chars get `Cloudflare Data Cent...`. IX names beyond ~20 chars get truncated similarly.
- For IPv6 addresses, abbreviate using standard :: notation (the data from PeeringDB should already be abbreviated, but verify).
- Support a `?cols=N` query parameter for users who want to specify width: `curl peeringdb-plus.fly.dev/ui/asn/13335?cols=120`.
- Use a tiered layout strategy: at 80 cols show essential columns; at 120+ cols show additional columns. The tier is determined by the `?cols` parameter with 80 as default.
- For entity detail views (single network, single IX), use a key-value layout instead of tables -- this naturally fits any width.

**Detection:** Render every entity type at 80 columns and visually inspect. Check that no line exceeds 80 characters (including ANSI escape sequences, which contribute zero visible width but do add bytes).

## Moderate Pitfalls

### Pitfall 5: User-Agent Detection False Positives and False Negatives

**What goes wrong:** A Python script using `requests` library (User-Agent: `python-requests/2.31.0`) gets ANSI-colored output when it expected JSON. A user running `xh` (a modern HTTPie alternative) gets HTML because `xh` isn't in the detection list. Monitoring tools like Prometheus, Datadog agent, or uptime checkers get terminal output.

**Why it happens:** User-Agent-based detection is inherently fragile. The list of terminal clients must be maintained manually. New clients appear (xh, nushell, hurl), old clients change their User-Agent format, and many programmatic HTTP clients use default User-Agents that overlap with neither browsers nor terminals.

**Consequences:** API consumers get ANSI garbage in their responses. Monitoring tools parse ANSI-contaminated output and generate false alerts. Developer scripts break silently.

**Prevention:**
- Use an allowlist of known terminal clients, not a blocklist of browsers. The terminal client list should be conservative:
  - `curl/` (most common -- note the trailing slash to avoid matching "curling-club-api")
  - `Wget/` (capital W, standard wget format)
  - `HTTPie/` (HTTPie's actual User-Agent format)
  - `xh/` (modern HTTPie alternative)
  - `fetch` (OpenBSD ftp/fetch)
  - `PowerShell/` (Invoke-WebRequest)
  - `lwp-request` (Perl LWP)
- Do NOT include generic HTTP libraries (`python-requests`, `aiohttp`, `Go-http-client`, `axios`) -- these are programmatic API clients that want JSON, not ANSI.
- Provide an explicit opt-in: `Accept: text/x-ansi` (custom media type) or `?format=terminal` forces terminal rendering regardless of User-Agent. This is the escape hatch for unrecognized clients.
- Provide an explicit opt-out: `?format=json` forces JSON regardless of User-Agent. This is the escape hatch for scripts using curl.
- Log unrecognized User-Agents that hit /ui/ paths at DEBUG level for future allowlist tuning.

**Detection:** Set up table-driven tests with ~20 User-Agent strings covering: major browsers, curl/wget/httpie, Python/Go/Node HTTP libraries, monitoring agents, empty User-Agent, and custom strings.

### Pitfall 6: Unicode Box Drawing Broken on Windows Terminals

**What goes wrong:** A network engineer on Windows using PowerShell or cmd.exe with the legacy console host sees garbled characters instead of box-drawing lines. Characters like `+`, `|`, and `-` appear, or worse, mojibake (raw UTF-8 bytes displayed as Windows-1252).

**Why it happens:** Windows Console Host (conhost.exe, the legacy terminal before Windows Terminal) does not support UTF-8 output by default. The code page defaults to the system locale (e.g., 437 for US English, 1252 for Western European). Box-drawing characters like `\u2500` (horizontal line) and `\u2502` (vertical line) are multi-byte UTF-8 sequences that display as garbage under non-UTF-8 code pages. Windows Terminal (the modern replacement) handles UTF-8 correctly, but many engineers still use conhost or PuTTY.

**Consequences:** Broken table rendering for any Windows user not using Windows Terminal. PuTTY users need to configure character encoding to UTF-8 manually.

**Prevention:**
- Provide ASCII-safe fallback table rendering using `+`, `-`, `|` characters (no Unicode box drawing). Activate via `?ascii` query parameter.
- Document the requirement: "For best results on Windows, use Windows Terminal or configure your terminal for UTF-8."
- When rendering with box drawing, stick to the basic Box Drawing Unicode block (U+2500-U+257F) which has the best cross-terminal support. Avoid Block Elements (U+2580-U+259F) and other decorative Unicode ranges.
- Set the Content-Type response header to `text/plain; charset=utf-8` to signal UTF-8 encoding to HTTP clients.

**Detection:** Test on Windows with both Windows Terminal and legacy conhost. Test via PuTTY from a Linux/macOS client.

### Pitfall 7: 256-Color ANSI Codes on Terminals That Only Support 16 Colors

**What goes wrong:** A user SSHed into a FreeBSD server with `TERM=vt100` runs curl and gets raw escape sequences like `[38;5;33m` displayed literally -- the terminal doesn't understand 256-color extended ANSI sequences and prints them as text.

**Why it happens:** The project specification calls for "rich 256-color ANSI output." Extended 256-color codes use the `\033[38;5;Nm` syntax (SGR 38), which requires a terminal that supports xterm-256color or equivalent. Basic terminals (vt100, xterm without 256-color, some SSH sessions to embedded devices) only support the base 16 colors (SGR 30-37, 90-97 for foreground).

**Consequences:** On basic terminals, every 256-color escape sequence is printed as literal characters, making the entire output unreadable. This is worse than no color at all.

**Prevention:**
- Use 256-color as the default for ANSI output (the vast majority of modern terminals support it), but design the color scheme to degrade gracefully if a terminal strips unknown sequences (most terminals simply ignore unrecognized SGR codes rather than printing them literally).
- Provide a `?color=16` parameter for users on basic terminals who want limited color.
- Provide the `?T` (plain text) option for terminals that cannot handle any ANSI codes.
- Choose 256-color palette values that have reasonable 16-color equivalents. For example, use color 33 (blue) which maps to ANSI blue (34) in the base palette.
- Test: Most terminals that don't support 256-color will silently ignore the escape sequence rather than displaying it literally. The vt100 literal-display case is rare in practice (2026 -- most terminals support at least 16-color SGR).

**Detection:** Test with `TERM=vt100 curl ...` and verify output is not garbled. Test with `TERM=xterm curl ...` (16-color) and verify colors appear reasonable.

### Pitfall 8: PeeringDB Data with Non-ASCII Characters in Address Fields

**What goes wrong:** A facility detail page showing "Equinix SP4 - Sao Paulo" renders the address field containing "Av. Ceci, 1900 - Res. Tambore" but the actual PeeringDB data has accented characters: "Sao Paulo" is really "Sao Paulo" (note: PeeringDB uses ASCII for city names in the name field but UTF-8 in address fields). Address fields contain characters like `e` with accent (`e`), `u` with umlaut (`u`), `n` with tilde, and ordinal markers like `2o` and `3a`.

**Why it happens:** PeeringDB stores address fields in UTF-8. The data is primarily ASCII for entity names (networks, IXes) but address/notes fields for non-English-speaking countries contain accented Latin characters. East Asian characters are rare but possible in notes fields.

**Consequences:** Terminal column width calculations break because multi-byte UTF-8 characters occupy 1 display column (for accented Latin) but 2 display columns (for CJK characters). A string that is 20 bytes may display as 18 columns (accented Latin) or 14 columns (mixed CJK). Box-drawing table alignment breaks if column width is calculated by byte count or rune count instead of display width.

**Prevention:**
- Calculate display width using Unicode East Asian Width property, not `len()` (byte count) or `utf8.RuneCountInString()` (rune count). The `go-runewidth` library (`github.com/mattn/go-runewidth`) handles this correctly.
- Truncation must also account for display width: cutting a string to "20 display columns" may mean cutting at byte offset 22 (accented Latin) or byte offset 12 (CJK).
- For entity names (which are primarily ASCII), this is low risk. For address and notes fields, it is moderate risk.
- Test with real PeeringDB data from Brazil (accented characters), Germany (umlauts in addresses), Japan (potential CJK in notes), and China (potential CJK in addresses).

**Detection:** Query facilities from Brazil, Germany, Japan via the live PeeringDB API and render in terminal mode. Check that table columns align correctly.

### Pitfall 9: Forgetting to Update All Content Negotiation Points

**What goes wrong:** Terminal detection is added to the `/ui/` dispatch handler but not to the readiness middleware, the root handler, or the error handlers. A curl user hits the readiness gate during sync and gets an HTML page (from the existing `Accept: text/html` check). A curl user hits a 404 and gets an HTML error page.

**Why it happens:** The codebase has multiple points where browser-vs-API detection occurs:
1. `GET /` root handler (main.go:312-320) -- branches on Accept header.
2. `readinessMiddleware` (main.go:376-404) -- branches on Accept header.
3. `renderPage` (render.go:27-35) -- branches on HX-Request header.
4. `handleNotFound` (handler.go:132-138) -- renders HTML unconditionally.
5. `handleServerError` (handler.go:141-147) -- renders HTML unconditionally.

Each of these must be updated to handle terminal clients correctly, but it is easy to miss one.

**Consequences:** Inconsistent experience: some endpoints return nice ANSI output while others return raw HTML that is unreadable in a terminal.

**Prevention:**
- Create a centralized `internal/terminal/detect.go` package with a single `IsTerminal(r *http.Request) bool` function.
- Audit every code path that writes Content-Type or branches on Accept/HX-Request. Create a checklist:
  - [ ] `GET /` root handler
  - [ ] Readiness middleware (503 during sync)
  - [ ] `renderPage` (full page vs fragment)
  - [ ] `handleNotFound` (404)
  - [ ] `handleServerError` (500)
  - [ ] Any future health/status endpoints that return human-readable output
- Write integration tests that exercise each path with a curl User-Agent and verify non-HTML Content-Type.

**Detection:** Run `curl -v` against every URL path category (root, /ui/*, 404, 500-triggering path, readiness-blocked path) and check that no response has `Content-Type: text/html`.

### Pitfall 10: ANSI String Building Allocations on Hot Path

**What goes wrong:** Every request to a terminal-detected client constructs ANSI output by concatenating escape sequences with data. For a network detail page with IX presences (potentially 200+ IXes for large networks like Cloudflare or Akamai), naive string concatenation creates thousands of intermediate strings, causing excessive GC pressure under load.

**Why it happens:** ANSI output requires interleaving escape codes (`\033[38;5;33m`) with data (`AS13335`) and reset codes (`\033[0m`) for every colored segment. A single colored cell requires 3 string operations minimum. A table with 200 rows and 6 columns = 3,600 string operations per request.

**Consequences:** At high concurrency (edge node serving many terminal requests), GC overhead increases. Not a correctness issue but a latency issue -- P99 latency spikes during GC pauses.

**Prevention:**
- Use `strings.Builder` with `Grow()` pre-allocation. Estimate output size: ~100 bytes per table row (including ANSI codes) * number of rows. Call `builder.Grow(estimatedSize)` before building.
- Use `sync.Pool` for builder reuse across requests if benchmarks show allocation is significant. Profile before optimizing per PERF-1.
- Pre-compute ANSI color code strings as package-level constants (e.g., `const colorBlue = "\033[38;5;33m"`, `const reset = "\033[0m"`). Do not call `fmt.Sprintf` to generate escape codes per cell.
- Consider building the ANSI output as `[]byte` using `bytes.Buffer` and writing directly to `http.ResponseWriter` via `w.Write()` instead of building a string and then converting. This avoids the final string allocation entirely.

**Detection:** Benchmark with `go test -bench=BenchmarkRenderNetwork -benchmem` and check allocations per operation. Profile with `pprof` under concurrent load (100 rps terminal requests).

## Minor Pitfalls

### Pitfall 11: curl Follows Redirects Only with -L Flag

**What goes wrong:** The root handler (`GET /`) currently redirects browsers to `/ui/` via 302. If a terminal client hits `/`, they get a redirect response instead of useful output. Users who type `curl peeringdb-plus.fly.dev` see `<a href="/ui/">Found</a>` because curl does not follow redirects by default.

**Prevention:** For terminal clients, return CLI help text directly at `/` instead of redirecting. The help text should show available endpoints and usage examples.

### Pitfall 12: Search Results Require Different Rendering Strategy

**What goes wrong:** The web UI search uses htmx to dynamically update search results. In terminal mode, there is no interactive search -- the user needs a one-shot query/response. Trying to replicate the interactive search experience for terminals leads to over-engineering.

**Prevention:** Terminal search should be simple: `curl peeringdb-plus.fly.dev/ui/search?q=cloudflare` returns a table of results. No pagination, no incremental updates. Limit to top 10 per type (matching the web UI behavior). This is a fundamentally different interaction model from the htmx UI.

### Pitfall 13: Compare View Complexity for Terminal

**What goes wrong:** The ASN comparison view works well in HTML with tabs and collapsible sections. Rendering this in terminal mode produces extremely long output that scrolls off screen.

**Prevention:** For terminal compare views, show only the summary (shared IX count, shared facility count) plus the shared items table. Skip the "only in A" / "only in B" sections by default. Provide `?view=full` to show everything. Design for pipe-to-less: `curl .../compare/13335/32934 | less -R` (the `-R` flag preserves ANSI colors in less).

### Pitfall 14: Missing Content-Length Header for Terminal Responses

**What goes wrong:** Terminal responses built with `strings.Builder` written directly to `http.ResponseWriter` don't set `Content-Length`. While HTTP/1.1 allows this (chunked transfer encoding is the default), some clients (notably wget) display progress bars based on Content-Length and show "unknown size" without it.

**Prevention:** Since terminal responses are generated in memory (not streamed), calculate the byte length and set `Content-Length` before writing. This also enables wget progress bars and `curl --progress-bar` to show accurate progress.

### Pitfall 15: Test Infrastructure for ANSI Output

**What goes wrong:** Testing ANSI output is inherently visual -- comparing strings full of escape codes in test failures is unreadable. A test failure showing `expected "\033[38;5;33mAS13335\033[0m" but got "\033[38;5;34mAS13335\033[0m"` (color 33 vs 34) is nearly impossible to debug by reading the diff.

**Prevention:**
- Use golden file testing (the project already uses this pattern for pdbcompat). Store expected ANSI output in `.golden` files and compare with `go-cmp`.
- Provide an `-update` flag to regenerate golden files.
- Also test the plain-text (`?T`) output separately -- this is easier to debug and validates the data correctness independent of styling.
- Consider a test helper that strips ANSI codes for structural comparison (correct data) and a separate test that validates ANSI codes for style comparison (correct colors).

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Mitigation |
|-------------|---------------|------------|
| Terminal detection (User-Agent parsing) | Pitfall 5: False positives/negatives with HTTP libraries | Conservative allowlist, explicit opt-in/opt-out query params |
| Content negotiation integration | Pitfall 2: Conflict with existing Accept-header logic | Single centralized detection function, clear priority order |
| Content negotiation integration | Pitfall 9: Missing updates to readiness, root, error handlers | Audit checklist of all negotiation points |
| ANSI rendering implementation | Pitfall 1: Escape codes in pipes | Mandatory `?T` plain-text mode from day one |
| ANSI rendering implementation | Pitfall 4: Width assumptions | Design for 80-column floor, truncation with ellipsis |
| ANSI rendering implementation | Pitfall 10: String building performance | `strings.Builder` with `Grow()`, pre-computed constants |
| Table rendering for entity details | Pitfall 8: Non-ASCII character width calculation | Use `go-runewidth` for display width, not byte/rune count |
| Table rendering for entity details | Pitfall 6: Windows terminal Unicode support | ASCII fallback mode via `?ascii` parameter |
| 256-color output | Pitfall 7: Basic terminals don't support 256-color | Graceful degradation, `?color=16` option |
| Caching and Vary headers | Pitfall 3: `Vary: User-Agent` destroys cache hit rates | `Cache-Control: no-store` on terminal responses |
| Error handling | Pitfall 9: Error pages render as HTML for terminal clients | Update handleNotFound/handleServerError to check terminal |
| Help text and root handler | Pitfall 11: curl doesn't follow redirects by default | Return help text at `/` for terminal clients |
| Search endpoint | Pitfall 12: Different interaction model needed | Simple one-shot query/response, no htmx |
| Compare endpoint | Pitfall 13: Output too long for terminal | Summary-first design, `?view=full` opt-in |
| Testing | Pitfall 15: ANSI output hard to debug in test failures | Golden files + ANSI-stripping test helpers |

## Sources

- [wttr.in GitHub repository](https://github.com/chubin/wttr.in) -- Console-oriented weather service, uses PLAIN_TEXT_AGENTS list for User-Agent detection (curl, httpie, wget, python-requests, python-httpx, lwp-request, powershell, fetch, aiohttp, http_get, xh, nushell, zig)
- [FOSDEM 2019 talk on console-oriented services](https://archive.fosdem.org/2019/schedule/event/console_services/) -- Architecture and lessons from wttr.in, cheat.sh, rate.sx
- [NO_COLOR standard](https://no-color.org/) -- Environment variable convention for disabling ANSI color output
- [CLICOLOR / CLICOLOR_FORCE](http://bixense.com/clicolors/) -- Alternative color control convention
- [MDN Content Negotiation](https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/Content_negotiation) -- Accept header vs User-Agent for server-driven negotiation
- [Fastly Vary header best practices](https://www.fastly.com/blog/best-practices-using-vary-header) -- Why `Vary: User-Agent` destroys cache hit rates
- [Smashing Magazine Vary header guide](https://www.smashingmagazine.com/2017/11/understanding-vary-header/) -- CDN caching implications of Vary
- [Microsoft Terminal Unicode issues](https://github.com/microsoft/terminal/issues/13680) -- Windows Console doesn't support Unicode out of the box
- [Microsoft Terminal box drawing issues](https://github.com/microsoft/terminal/issues/5897) -- Alternative box drawing rendering approaches
- [charmbracelet/lipgloss table width discussion](https://github.com/charmbracelet/lipgloss/discussions/430) -- Making tables fit terminal width
- [PeeringDB UTF-8 encoding issue #663](https://github.com/peeringdb/peeringdb/issues/663) -- Historical iso-8859-1 vs UTF-8 encoding problems
- [go-isatty](https://github.com/mattn/go-isatty) -- Terminal detection for local CLI tools (not applicable for HTTP servers)
- [bgp.tools API documentation](https://bgp.tools/kb/api) -- Requires custom User-Agent, blocks default/generic agents
- [strings.Builder vs bytes.Buffer](https://brandur.org/fragments/bytes-buffer-vs-strings-builder) -- strings.Builder ~33% faster for string building
- [everything curl: User-Agent](https://everything.curl.dev/http/modify/user-agent.html) -- Default curl User-Agent format: `curl/VERSION`
- [GNU libtextstyle terminal emulators](https://www.gnu.org/software/gettext/libtextstyle/manual/html_node/Terminal-emulators.html) -- Comprehensive terminal color support matrix
- Live PeeringDB API testing -- Confirmed: entity names are primarily ASCII; address fields contain accented Latin characters (Brazilian facilities, German addresses); CJK characters are rare but possible in notes fields
