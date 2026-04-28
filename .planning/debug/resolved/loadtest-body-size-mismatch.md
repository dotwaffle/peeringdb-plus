---
slug: loadtest-body-size-mismatch
status: resolved
trigger: |
  /api/org?depth=2 against peeringdb-plus.fly.dev returns 122.6 KiB to
  the loadtest tool, but the user expects ~64 MiB based on what real
  PeeringDB returns for the equivalent query. 500x discrepancy.
created: 2026-04-28
updated: 2026-04-28
---

# Debug: loadtest body-size mismatch on /api/org?depth=2

## Symptoms

- **Expected:** /api/org?depth=2 against the peeringdb-plus mirror should
  return ~64 MiB of data (matches what real PeeringDB returns for the
  equivalent query).
- **Actual:** loadtest verbose output shows 200 OK with body size 122.6 KiB.
  - Sample line: `[27/39] GET     /api/org?depth=2 -> 200 (102.70723ms, 122.6 KiB)`
- **Errors:** none — request returns 200, just way smaller than expected.
- **Timeline:** observed today (2026-04-28) after adding response-size
  measurement to the loadtest verbose output (commit bfa452f).
- **Reproduction:**
  ```
  go run ./cmd/loadtest sync --mode=full --verbose
  ```
  …and look at any /api/<type>?depth=N row.

## Prime hypothesis (pre-filled)

`internal/pdbcompat/response.go` defines `DefaultLimit=250` and
`MaxLimit=1000`. When pdbcompat receives a request with no `limit=`
parameter, it applies DefaultLimit=250. The loadtest URL
(`/api/org?depth=N`) doesn't supply `limit`, so the server is silently
returning 250 rows — that's the 122.6 KiB. Real upstream PeeringDB at
www.peeringdb.com has no such default and returns the entire table
(~234k orgs).

## Alternative hypotheses to rule out

1. **Loadtest mismeasurement** — `io.Copy(io.Discard, resp.Body)` might
   short-read on chunked transfer encoding, gzip decoding, etc.
   Disprove via direct `curl -w '%{size_download}'`.
2. **HTTP-level truncation** — Fly Proxy / LiteFS proxy / middleware
   imposing a body cap. Disprove via direct curl + headers inspection.
3. **Genuine server bug** — pdbcompat's depth=2 traversal might be
   broken and silently producing partial output.
4. **Compression weirdness** — server returns gzip but Content-Length
   reflects compressed size; loadtest measures decompressed size; some
   intermediate proxy strips content-encoding. Disprove via Accept-Encoding
   negotiation tests.

## Constraints (from orchestrator prompt)

- Do NOT hit `https://www.peeringdb.com` upstream — 1 req/hour rate limit
  and the user's IP could be blocked.
- All probing against the project's own mirror at
  `https://peeringdb-plus.fly.dev` is fair game.

## Current Focus

- hypothesis: CONFIRMED. pdbcompat applies DefaultLimit=250 when `limit=`
  is absent. The loadtest URL `/api/org?depth=N` therefore caps at 250
  rows. The mirror is parity-correct; the loadtest needs `&limit=0`.
- next_action: COMPLETE. Fix applied to cmd/loadtest/sync.go and
  cmd/loadtest/sync_test.go; verified end-to-end against
  peeringdb-plus.fly.dev.

## Evidence

- timestamp: 2026-04-28T00:13Z
  source: curl probe against peeringdb-plus.fly.dev
  data: |
    /api/org?depth=2          → 125,499 bytes, 250 rows  (matches loadtest's 122.6 KiB)
    /api/org?depth=2&limit=0  → 15,111,696 bytes, 33,555 rows  (full table at depth=2)
    /api/org?depth=2&limit=1000 → 479,322 bytes, 1,000 rows  (MaxLimit ceiling)
    /api/org?depth=0          → 125,499 bytes, 250 rows  (depth doesn't matter; same default)
    /api/org?depth=0&limit=0  → 15,111,696 bytes, 33,555 rows
  - The byte counts are identical between depth=0 and depth=2 because
    pdbcompat silently drops `?depth=N` on list endpoints (documented
    divergence — see docs/API.md § Known Divergences "list endpoints
    silently drop ?depth=" and `LIMIT-02` in
    internal/pdbcompat/parity/limit_test.go). depth=2 only takes effect
    on detail endpoints.
- timestamp: 2026-04-28T00:13Z
  source: source code trace
  data: |
    internal/pdbcompat/response.go:61-77 ParsePaginationParams
      → defaults `limit = DefaultLimit (250)` when query param absent
      → respects `limit=0` as upstream-compatible "unlimited" sentinel
      → comment cites rest.py:734-737 + rest.py:494-497 for parity
    internal/pdbcompat/parity/limit_test.go:32-72 LIMIT-01
      → parity-locked: bare URL must return 250 (DefaultLimit)
      → parity-locked: limit=0 must return all rows unbounded
    docs/API.md:253
      → publicly documented: "Page size. Default 250, clamped to 1000."
- timestamp: 2026-04-28T00:13Z
  source: loadtest source review (cmd/loadtest/sync.go:50-54)
  data: |
    Author comment in buildSyncEndpoints already warns about a similar
    issue: an earlier revision used `?limit=250&skip=0&depth=0` and
    "made full sync finish suspiciously fast because the server returned
    at most 250 rows per type". The author then switched to bare
    `/api/<type>?depth=N` to "mirror StreamAll" — but didn't realise
    StreamAll works against UPSTREAM PeeringDB (no DefaultLimit) while
    this mirror ALSO caps at 250. The fix the author intended (full
    streaming) requires `&limit=0`.
- timestamp: 2026-04-28T00:30Z
  source: post-fix loadtest run against peeringdb-plus.fly.dev
  data: |
    [ 1/39] GET /api/org?depth=0&limit=0 -> 200 (1.54s, 14.4 MiB)
    [10/39] GET /api/net?depth=0&limit=0 -> 200 (2.26s, 33.9 MiB)
    [13/39] GET /api/netixlan?depth=0&limit=0 -> 200 (1.77s, 21.5 MiB)
    [27/39] GET /api/org?depth=2&limit=0 -> 200 (0.98s, 14.4 MiB)
    [36/39] GET /api/net?depth=2&limit=0 -> 200 (1.89s, 33.9 MiB)
  - All 39 requests now return MiB-scale bodies (not KiB) confirming
    the mirror is now serving full tables. /api/net at any depth is
    the largest single body at 33.9 MiB.
  - Total wall-clock: 25.674s for 39 requests, observed RPS 1.52.
  - No 413s seen — every full-table response is comfortably under the
    Phase 71 PDBPLUS_RESPONSE_MEMORY_LIMIT default of 128 MiB.

## Eliminated

- **Loadtest mismeasurement** — eliminated. `curl -w '%{size_download}'`
  reports the same 125,499 bytes that the loadtest measures (122.6 KiB).
  The loadtest's `io.Copy(io.Discard, resp.Body)` is correctly counting
  bytes received.
- **HTTP-level truncation** — eliminated. The body parses as valid JSON
  with a complete `{meta, data}` envelope and `data.length === 250`.
  `Content-Length` matches `size_download` exactly. No proxy is
  truncating.
- **Genuine server bug at depth=2** — eliminated. depth=0 and depth=2
  both return 125,499 bytes for the same reason: list endpoints silently
  drop `?depth=` (intentional, documented divergence — LIMIT-02). The
  size is governed entirely by row count × row size, both of which are
  correct.
- **Compression weirdness** — eliminated. `Content-Type: application/json`
  with no `Content-Encoding` header in the response. Body is plain JSON.
- **User's "64 MiB" expectation** — partially eliminated. The user's
  intuition was right in order of magnitude — the largest single
  endpoint after fix is /api/net?depth=2&limit=0 at 33.9 MiB, not 64.
  The 64 MiB figure may have been a rough estimate or a recollection
  of total bytes per cycle (39 requests × varied sizes ≈ several
  hundred MiB total, of which the largest single request is 33.9 MiB).

## Resolution

**Root cause:** The pdbcompat server applies `DefaultLimit=250` when
the `limit=` query param is absent. This is intentional, parity-locked
to upstream PeeringDB DRF semantics (`rest.py:734-737`), and publicly
documented in `docs/API.md:253`. The loadtest's URL shape
`/api/<type>?depth=N` (no limit) therefore caps at 250 rows per
request — which is what the 122.6 KiB measurement represented.

The previous loadtest comment block (sync.go:50-54) shows the author
already navigated this trap once — they removed an earlier
`?limit=250&skip=0` URL shape after noticing full sync finished
"suspiciously fast". They replaced it with the bare URL believing
that mirrored StreamAll's against-upstream behaviour. They didn't
realise the mirror enforces its own DefaultLimit=250 — making bare and
explicit-250 behaviourally identical for full mode.

**Fix applied:** Two-line change in `cmd/loadtest/sync.go` and a
matching test update in `cmd/loadtest/sync_test.go`:

- `cmd/loadtest/sync.go:88` — full-mode URL is now
  `/api/<type>?depth=N&limit=0` (was `/api/<type>?depth=N`). The
  surrounding doc comment block (lines 31-77) was rewritten to
  document why `limit=0` is required and to cross-reference this
  debug session.
- `cmd/loadtest/sync_test.go` — `TestSync_BuildSyncEndpointsFull`
  asserts the new URL shape; the old assertion ("full mode must NOT
  include limit") is replaced with "full mode MUST include limit=0
  (mirror DefaultLimit=250 would cap otherwise — see LIMIT-01 /
  debug-session loadtest-body-size-mismatch)". The `skip=` exclusion
  assertion stays — full mode is a single request, not paginated.

**Verification:**
- `go build ./cmd/loadtest/...` — clean.
- `go vet ./cmd/loadtest/...` — clean.
- `go test -race ./cmd/loadtest/...` — pass (1.979s).
- Live verification against `https://peeringdb-plus.fly.dev`:
  - Pre-fix: `/api/org?depth=2` → 122.6 KiB (250 rows).
  - Post-fix: `/api/org?depth=2&limit=0` → 14.4 MiB (33,555 rows).
  - All 39 requests now return MiB-scale bodies; no 413s.

**Behavioural notes for the future operator** (captured in the
sync.go doc-comment block; reproduced here for the debug record):

- Bare `/api/<type>` against the mirror returns 250 rows (DefaultLimit).
- `/api/<type>?limit=0` returns the full table; gated by Phase 71
  response-memory-budget (default 128 MiB) which returns 413 if the
  precount × TypicalRowBytes would breach.
- `?depth=N` is silently dropped on list endpoints (LIMIT-02
  divergence) — depth=0/1/2 give identical bodies for any given
  list URL. The depth band still has value as a signal in case the
  divergence is reverted in a future phase.

**Specialist hint:** general (loadtest tooling change is a small Go
edit; no language-specific concerns).
