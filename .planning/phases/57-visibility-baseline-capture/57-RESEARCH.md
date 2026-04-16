# Phase 57: Visibility baseline capture - Research

**Researched:** 2026-04-16
**Domain:** PeeringDB API fixture capture, structural JSON diffing, PII redaction, resumable CLI tooling
**Confidence:** HIGH

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- **D-01:** Extend `cmd/pdbcompat-check/` with a new `--capture` mode (NOT a separate binary). The existing tool is already auth-aware, already walks all 13 types, already shares `internal/peeringdb` client wiring.
- **D-02:** **HARD CONSTRAINT — privacy in the repo:** captured PeeringDB data must NOT be committed in raw form. Never commit personal data (email, phone, names, addresses) from authenticated responses, and never commit large raw blobs of upstream data even from anon responses.
- **D-03:** Walk **all 13 PeeringDB types**, both auth modes, **first 2 pages each**. Total ≈ 52 anon + 52 auth requests.
- **D-04:** Run against **`beta.peeringdb.com` first**, then a confirmation pass against **`www.peeringdb.com`** for `poc`, `org`, `net` only.
- **D-05:** **Anonymous fixtures** committed as raw upstream JSON at `testdata/visibility-baseline/{beta|prod}/anon/api/{type}/page-1.json`.
- **D-06:** **Authenticated fixtures** redacted before commit. Keep field names, types, row IDs, `visible` value, counts, structural shape. Replace any string field value absent from the corresponding anon response with `"<auth-only:string>"` (or `"<auth-only:int>"` etc. by type).
- **D-07:** Layout: `testdata/visibility-baseline/{beta|prod}/auth/api/{type}/page-1.json` (redacted form). Raw auth bytes stay local — gitignored or `/tmp`.
- **D-08:** Two artifacts in sync: per-type Markdown (`testdata/visibility-baseline/DIFF.md`) + machine-readable JSON (`testdata/visibility-baseline/diff.json`).
- **D-09:** Diff describes deltas in terms of field names + row counts + `visible` values + placeholder type — never real values.
- **D-10:** Rate-limit pacing: **dynamic via `Retry-After` + exponential backoff**, NOT hardcoded sleeps. Reuse the `RateLimitError` path added in v1.13.
- **D-11:** Resumability: **checkpoint state file in `/tmp/pdb-vis-capture-state.json`**. On startup, if present, ask operator: Resume / Restart.

### Claude's Discretion

- Exact placeholder string format for redacted fields — pick one and keep it consistent.
- Whether `--capture` reuses the existing CLI plumbing or adds a switch — implementation detail.
- Per-request progress UX (stderr log vs progress bar) — operator preference, no privacy impact.

### Deferred Ideas (OUT OF SCOPE)

- Live drift detection on a schedule.
- Historical archive of past baselines (git already covers this).
- Multi-region capture to confirm no IP-based rate variation.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| VIS-01 | Capture both unauthenticated and authenticated PeeringDB responses for all 13 types as committed baseline fixtures under `testdata/visibility-baseline/{beta\|prod}/` | Standard Stack (reuse `internal/peeringdb.Client`), Architecture Patterns (fixture layout), Redaction Algorithm (auth-side safety) |
| VIS-02 | Emit a structural diff report listing every field/row that differs between unauth and auth responses, per-type Markdown table + machine-readable JSON | Diff Algorithm, Code Examples (diff emitter), Don't Hand-Roll (reuse `internal/conformance` shape) |
</phase_requirements>

## Summary

All inputs needed to plan this phase are already in the repo or on the public PeeringDB docs — no novel library work is required. The phase is a pure composition: reuse `internal/peeringdb.Client` (which already handles rate-limit + `Retry-After` via the `RateLimitError` path added in v1.13), reuse the shape idioms of `internal/conformance.CompareStructure`, and add a `--capture` subcommand to `cmd/pdbcompat-check` that walks `(target, mode, type, page)` tuples, writes raw bytes to `/tmp`, runs a redaction pass, and emits two diff artifacts.

The single hard risk is **accidental PII commit**. The defence is layered: (1) write raw auth bytes only under `/tmp` (never inside the repo), (2) redact on a one-way pass with an allow-list of structural fields and a string-level "present in anon? keep : replace with placeholder" rule, (3) a unit test that fails if any known PII field name (`email`, `phone`, `name`, `address1`, `address2`, `tech_email`, `sales_email`, `policy_email`, `tech_phone`, `sales_phone`, `policy_phone`) contains a non-placeholder value in any committed `auth/` fixture, and (4) `.gitignore` exclusion of the raw-auth working directory.

The second risk is **burning rate-limit quota on retries**. The v1.13 client already solves this via typed `RateLimitError` + parsed `Retry-After`. The capture loop must surface that error and sleep for `RetryAfter` before the next request; it must NOT retry inside the same request. This matches D-10.

**Primary recommendation:** Add `--capture` as a subcommand-shaped flag (e.g. `pdbcompat-check -capture -target=beta -mode=both -out=testdata/visibility-baseline/beta`). Reuse `internal/peeringdb.Client` unchanged; add a new package `internal/visbaseline/` containing: capture loop, checkpoint state, redactor, differ, Markdown emitter, JSON emitter. Keep `internal/peeringdb` and `internal/conformance` untouched — this phase is additive.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| HTTP fetch + rate-limit pacing | `internal/peeringdb` (existing) | — | Already solved: `Client`, `RateLimitError`, `Retry-After` parsing |
| 13-type walk, both auth modes | `cmd/pdbcompat-check` CLI | `internal/visbaseline` | CLI is the operator entrypoint; business logic lives in the internal package |
| Checkpoint state (resume) | `internal/visbaseline/checkpoint` | — | Single-file JSON write/read; no other tier needs this |
| Redaction pass (auth bytes → committed fixture) | `internal/visbaseline/redact` | — | Pure function over two JSON trees (anon + auth raw) |
| Structural diff emission | `internal/visbaseline/diff` | Reuse shape of `internal/conformance.CompareStructure` | Existing conformance differ doesn't track row counts or `visible` values — new code, but same shape |
| Markdown + JSON artifact writing | `internal/visbaseline/report` | — | Small, pure, testable |

**Why this matters:** Keeping the capture CLI thin and pushing every stateful/testable piece into `internal/visbaseline` lets the planner scope one file per concern (capture loop, checkpoint, redactor, differ, emitters) — each with its own unit test. Mixing these into `main.go` would produce one 600-line file that's impossible to test without hitting the live API.

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `internal/peeringdb` (existing) | current | HTTP client with rate limit + Retry-After + auth | [VERIFIED: internal/peeringdb/client.go:90-393] already has `Client`, `WithAPIKey`, `RateLimitError`, `parseRetryAfter`. No new client needed. |
| `encoding/json` (stdlib) | Go 1.26+ | JSON parse + emit; used for both fixture write and diff walk | [VERIFIED: CLAUDE.md] project uses stdlib for JSON everywhere |
| `log/slog` (stdlib) | Go 1.26+ | Per-request progress output | [VERIFIED: CLAUDE.md] "Logging: log/slog (stdlib) with OTel bridge" |
| `flag` (stdlib) | Go 1.26+ | CLI arg parsing — extend the existing `cmd/pdbcompat-check` flag set | [VERIFIED: cmd/pdbcompat-check/main.go:38-44] |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `bufio` (stdlib) | Go 1.26+ | Line-buffered stdin for Resume/Restart prompt | When reading operator answer to checkpoint prompt |
| `os/signal` (stdlib) | Go 1.26+ | SIGINT handling so Ctrl+C still writes the checkpoint | To guarantee D-11 "survives Ctrl+C, crashes, machine sleep" |
| `context` (stdlib) | Go 1.26+ | Cancellation propagation from signal handler into in-flight fetch | Standard Go pattern; required by `internal/peeringdb.Client` |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Extending `cmd/pdbcompat-check` (D-01 locked) | New `cmd/pdb-vis-capture` binary | Rejected by D-01 — duplicates ~150 lines of client/auth/rate scaffolding |
| A cobra/urfave-style subcommand library | `flag` with `-capture` boolean + mode switch | Project is stdlib-first (CLAUDE.md). A single added boolean flag that changes code path is fine. [CITED: user global rules: "prefer stdlib solutions and minimize new dependencies"] |
| JSON-Patch (RFC 6902) for diff output | Hand-rolled structural differ per this phase | JSON-Patch operates on values, not structural presence. Wrong tool; we explicitly must NOT leak values. |
| `jsondiff` third-party library | Hand-rolled structural differ | Same value-leak concern; and introduces a dep for ~80 LOC of pure Go. |

**Installation:** No new Go module dependencies required. [VERIFIED: CLAUDE.md "Key dependencies (see go.mod for exact versions)" — all needed pieces are already in go.mod]

**Version verification:** `go version` shows the toolchain; the `go.mod` `go` directive governs language level. No package install needed.

## Architecture Patterns

### System Architecture Diagram

```
  operator shell                        PeeringDB API
       │                                     │
       │  $ pdbcompat-check --capture        │
       │    -target=beta -mode=both          │
       │                                     │
       ▼                                     │
  ┌──────────────────────────┐               │
  │ cmd/pdbcompat-check/main │               │
  │  (flag parse, dispatch)  │               │
  └──────────┬───────────────┘               │
             │  capture mode?                │
             ▼                               │
  ┌────────────────────────────────────┐     │
  │ internal/visbaseline/capture       │     │
  │  - load/prompt checkpoint          │     │
  │  - for (target, mode, type, page): │     │
  │      fetch via peeringdb.Client ───┼─────┤ GET /api/{type}?limit=N&skip=M
  │      write raw → /tmp/.../auth     │     │
  │      write raw → repo anon/        │     │  (HTTP 200 / 429 + Retry-After)
  │      save checkpoint               │     │
  │  - on RateLimitError: sleep N, loop│◄────┤
  └──────────┬─────────────────────────┘     │
             │                               │
             ▼  all tuples captured          │
  ┌────────────────────────────┐             │
  │ internal/visbaseline/redact│             │
  │  read anon + raw auth →    │             │
  │  write redacted auth       │             │
  │  to repo auth/             │             │
  └──────────┬─────────────────┘             │
             ▼                               │
  ┌────────────────────────────┐             │
  │ internal/visbaseline/diff  │             │
  │  walk anon ↔ auth trees,   │             │
  │  emit DIFF.md + diff.json  │             │
  └────────────────────────────┘             │
                                             │
             committed artifacts (repo):     │
             testdata/visibility-baseline/   │
               ├── DIFF.md                   │
               ├── diff.json                 │
               ├── beta/anon/api/{type}/…    │
               ├── beta/auth/api/{type}/…    │  (redacted)
               ├── prod/anon/api/{type}/…    │  (poc/org/net only)
               └── prod/auth/api/{type}/…    │  (redacted)
```

**Component Responsibilities:**

| File | Responsibility |
|------|----------------|
| `cmd/pdbcompat-check/main.go` | Flag parsing; dispatch to `capture.Run()` or existing checker |
| `cmd/pdbcompat-check/capture.go` | Thin adapter calling `internal/visbaseline.Run(cfg)` (keeps main.go small) |
| `internal/visbaseline/capture.go` | The (target, mode, type, page) loop, calls checkpoint + fetch |
| `internal/visbaseline/checkpoint.go` | `State` struct, `Load`, `Save`, `PromptResumeOrRestart` |
| `internal/visbaseline/redact.go` | `Redact(anon, authRaw []byte) ([]byte, error)` pure function |
| `internal/visbaseline/diff.go` | `Diff(anon, auth []byte) (Report, error)` — per-type walk |
| `internal/visbaseline/report.go` | `WriteMarkdown(...)`, `WriteJSON(...)` |
| `internal/visbaseline/pii.go` | The PII field allow-list used by the "never leave these fields non-placeholder" unit test |

### Recommended Project Structure

```
cmd/pdbcompat-check/
├── main.go            # extended: adds --capture boolean + new flags
└── capture.go         # new: thin wiring to internal/visbaseline

internal/visbaseline/
├── capture.go         # walk loop
├── capture_test.go    # unit tests against httptest.NewServer (no live calls)
├── checkpoint.go      # state file r/w
├── checkpoint_test.go
├── redact.go          # redaction transform
├── redact_test.go     # extensive: PII must never pass through
├── diff.go            # structural differ
├── diff_test.go       # golden tests against known-shape inputs
├── report.go          # markdown + json writers
├── report_test.go
└── pii.go             # PII field allow-list (used by redactor + tests)

testdata/visibility-baseline/         # new tree
├── DIFF.md
├── diff.json
├── beta/
│   ├── anon/api/{campus,carrier,...}/page-{1,2}.json
│   └── auth/api/{campus,carrier,...}/page-{1,2}.json   # redacted only
└── prod/
    ├── anon/api/{org,net,poc}/page-{1,2}.json
    └── auth/api/{org,net,poc}/page-{1,2}.json          # redacted only

.gitignore           # add: testdata/visibility-baseline/**/.raw-auth/
                     #      /tmp/pdb-vis-capture-*
```

### Pattern 1: Reusing the existing client for both anon + auth modes

**What:** Build two `*peeringdb.Client` instances per target. Anon: `NewClient(baseURL, logger)`. Auth: `NewClient(baseURL, logger, WithAPIKey(key))`. The client auto-selects the correct rate limit (1/3s anon, 1s auth) and applies the Authorization header only when the key is set.

**When to use:** Always, in `capture.Run`. Do NOT hand-roll a second HTTP client.

**Example:**

```go
// Source: internal/peeringdb/client.go:106-135 (existing)
anon := peeringdb.NewClient(cfg.BaseURL, logger)
auth := peeringdb.NewClient(cfg.BaseURL, logger, peeringdb.WithAPIKey(cfg.APIKey))
```

### Pattern 2: First-2-pages walk using `limit=250&skip=N`

**What:** PeeringDB's pagination is `?limit=<N>&skip=<M>`, which the existing sync code already uses. [VERIFIED: internal/peeringdb/stream.go:77] `"...limit=%d&skip=%d&depth=0&since=%d"`. For the baseline walk we want `depth=0` (no expansion) and `limit=250`, fetch `skip=0` and `skip=250`. Total pages per (target, mode, type) = 2. Types with fewer than 250 total rows produce an empty page 2 — that's a valid observation and should be captured as-is.

**When to use:** In `capture.Run`, for every (target, mode, type) tuple.

**Example URL template:**

```
https://beta.peeringdb.com/api/poc?limit=250&skip=0&depth=0
https://beta.peeringdb.com/api/poc?limit=250&skip=250&depth=0
```

### Pattern 3: Treating `RateLimitError` as a sleep signal, not a retry

**What:** On `errors.As(err, &rlErr)`, sleep `rlErr.RetryAfter + 5s` jitter and re-fetch the SAME tuple. Do NOT increment the checkpoint. Do NOT call the existing client's internal retry ladder (it's already disabled for 429 per `client.go:312-327` — the client short-circuits 429 to the caller).

**When to use:** In the capture loop's per-tuple error handler.

**Example:**

```go
// Source: internal/peeringdb/client.go:32-49 (existing RateLimitError)
for {
    resp, err := client.FetchAll(ctx, typeName)
    var rlErr *peeringdb.RateLimitError
    if errors.As(err, &rlErr) {
        wait := rlErr.RetryAfter + 5*time.Second // jitter
        logger.LogAttrs(ctx, slog.LevelWarn, "rate-limited, sleeping",
            slog.Duration("retry_after", rlErr.RetryAfter),
            slog.String("type", typeName),
        )
        select {
        case <-time.After(wait):
        case <-ctx.Done():
            return ctx.Err()
        }
        continue
    }
    if err != nil {
        return err
    }
    break
}
```

### Pattern 4: Checkpoint-before-write ordering (resume correctness)

**What:** For each tuple, the order is:
1. Fetch bytes from PeeringDB.
2. Write bytes to final destination on disk (raw anon → repo; raw auth → `/tmp`).
3. fsync + rename the checkpoint state file atomically, advancing past this tuple.

If interrupted between (1) and (2): we re-fetch on resume — one wasted request, no data loss.
If interrupted between (2) and (3): we re-fetch and overwrite — one wasted request, no data loss, no corruption.
If interrupted after (3): we proceed to the next tuple on resume.

**Atomic checkpoint write:** Write to `/tmp/pdb-vis-capture-state.json.tmp`, then `os.Rename` to final path. POSIX rename on the same filesystem is atomic.

### Anti-Patterns to Avoid

- **Re-implementing rate limiting.** The `internal/peeringdb.Client` already does it. Do not add a new `golang.org/x/time/rate` limiter in this phase — you'd double-limit or race with the existing one.
- **Writing raw auth bytes inside the repo tree, "just briefly".** git status screenshots, IDE auto-save, runaway `git add .` — any of these leaks PII. Write raw auth bytes to `/tmp` or an explicit gitignored subdirectory only.
- **Value-aware diff output.** The Markdown table and diff.json must describe *shape deltas*, never values. A diff line like `poc[0].email: "noc@example.org" (auth) vs absent (anon)` defeats the whole phase.
- **Depth > 0 in capture URLs.** PeeringDB's `?depth=N` expands related sets. The baseline phase is about single-type visibility, not joined graphs. Use `depth=0`.
- **Deterministic-order reliance on Go's `map` iteration.** When emitting DIFF.md or diff.json, sort field paths before output. Matches what `internal/conformance.CompareStructure` already does at compare.go:27-29.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| HTTP retries with exponential backoff on 5xx | New retry loop inside capture | Existing `peeringdb.Client.doWithRetry` | [VERIFIED: client.go:230-358] Already exists, already tested |
| Parsing `Retry-After` (seconds vs HTTP-date) | Custom parser | `peeringdb.parseRetryAfter` (unexported, but `RateLimitError.RetryAfter` is pre-parsed) | [VERIFIED: client.go:59-76] Already handles both RFC 7231 forms |
| Rate limiting to 20/min anon, 40/min auth | Ticker or manual sleep | `peeringdb.Client` limiter (auto-selects based on `WithAPIKey`) | [VERIFIED: client.go:122-134] `WithAPIKey` switches limiter to 60/min; match auth-mode flag to key presence |
| JSON streaming for large responses | Unmarshal-all-to-map | `peeringdb.StreamAll` | [VERIFIED: stream.go:35-110] Already handles both `{meta,data}` and `{data,meta}` orderings |
| Structural type diff (`null` vs `string`) | Bespoke walker | Pattern after `internal/conformance.jsonType` | [VERIFIED: compare.go:107-124] The 6-type map (`null`, `bool`, `number`, `string`, `array`, `object`) already exists |
| CLI flag parsing | `spf13/cobra` | stdlib `flag` | [CITED: CLAUDE.md + user's global go-guidelines GO-MD-1] Prefer stdlib |
| Atomic file rename | Custom lock file | `os.Rename` on same filesystem | stdlib, POSIX-atomic |

**Key insight:** This phase is ~80% glue over code that already exists. The only genuinely new code is the redactor, the differ (a narrower variant of conformance.Compare that tracks row counts + `visible`), and the two emitters. Everything else is composition.

## Runtime State Inventory

This phase is additive and does not rename or migrate anything. Included for explicit completeness:

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | **None** — no database writes, no schema changes. Anon fixtures are new files in `testdata/visibility-baseline/`. | None |
| Live service config | **None** — no Fly secrets, no LiteFS, no OTel config changes. Operator supplies PDB API key via existing `PDBPLUS_PEERINGDB_API_KEY` env var or `-api-key` flag. | None |
| OS-registered state | **None** — single-shot CLI, not a daemon, no systemd/launchd/scheduler registration. | None |
| Secrets/env vars | `PDBPLUS_PEERINGDB_API_KEY` is read via the existing env-var path. No new env vars introduced in this phase. `PDBPLUS_SYNC_TOKEN` is unrelated. | None |
| Build artifacts | `pdbcompat-check` binary rebuilds via `go build ./cmd/pdbcompat-check`. Flag set changes but signature stays backward-compatible (default run without `-capture` still works). | None |

## Common Pitfalls

### Pitfall 1: `Retry-After: 3600` when we're doing a one-shot walk

**What goes wrong:** Anonymous /api/org historically earned a 1-req/hr lockout under the response-size throttle. If the capture walk trips that throttle on type #N, it stalls for up to an hour on that single request.

**Why it happens:** PeeringDB's DRF `ResponseSizeThrottle` fires when the same anonymous source requests the same query-string with a response >100KB more than once per hour. [VERIFIED: PeeringDB docs + upstream throttle source] `/api/org?limit=250&skip=0` with depth=0 is still well under 100KB on beta, but prod /api/org historically clips this.

**How to avoid:** 
1. Capture beta first (smaller dataset, well clear of the size-throttle trigger).
2. On prod, only fetch `poc`, `org`, `net` (D-04).
3. Honour any `Retry-After` by sleeping — do NOT try a different endpoint expecting the throttle only applies to the one URL. The anon-wide 20/min limit is also in play.
4. If the operator has already run capture once today, the checkpoint short-circuit is the right escape hatch.

**Warning signs:** `RateLimitError.RetryAfter > 5m` — treat as a hard pause, log loudly, continue.

### Pitfall 2: `visible` field as a string enum, not a bool

**What goes wrong:** Code assumes `visible == "Public"` is the only public value and misclassifies "Users" rows when they *do* appear (they shouldn't anonymously, but API bugs exist).

**Why it happens:** `poc.visible` is a Django CharField with three historical values: `Public`, `Users`, `Private`. [CITED: https://docs.peeringdb.com/blog/contacts_marked_private/] Anonymous responses should only contain `Public`, authenticated adds `Users`. `Private` should never leave the server. Any `Private` row in an anon response is an upstream bug — the phase should *capture* that rather than crash.

**How to avoid:** The differ records the set of `visible` values seen per (type, mode, page). Never error on an unexpected value — record it.

### Pitfall 3: Page-boundary race during an ongoing sync on upstream

**What goes wrong:** If upstream mutates between page 1 and page 2, row IDs can shift. Page 2 may re-emit a row seen on page 1, or skip a row. The diff would see spurious "row count drift".

**Why it happens:** PeeringDB's list endpoint offers no snapshot semantics. Pagination with `skip` is a LIMIT/OFFSET SQL query against live data.

**How to avoid:** 
1. Capture both anon and auth for a given type back-to-back (within seconds), minimising the window.
2. Record the `meta.generated` timestamp (if present — it's usually 0 for PeeringDB; [VERIFIED: internal/peeringdb/types.go:13-16] `Meta is always empty in practice`). If non-zero, include in the diff report.
3. Don't dedupe across pages silently — if you observe the same `id` on pages 1 and 2 in anon mode, surface it in the diff as a "pagination anomaly".

### Pitfall 4: PII leaking via structural reveal

**What goes wrong:** Even with string values redacted, a structural line like `poc[0].email.present=true` tells an observer "row 0 has an email". For `name`, which can be a person's legal name, even *length* is fingerprintable.

**Why it happens:** "Structural diff" can still carry signal-rich metadata (length, count of elements, null/non-null).

**How to avoid:** The diff report says only (a) the field name, (b) the placeholder type (`<auth-only:string>`), (c) the number of rows where the field moved from absent-in-anon to present-in-auth — never any per-row flag, never any length, never any hash. `diff.json` entries look like `{"type":"poc","field":"email","auth_only":true,"placeholder":"<auth-only:string>","rows_added":N}`.

### Pitfall 5: Machine sleep mid-capture

**What goes wrong:** Laptop closes, `time.Sleep(2200 * time.Second)` for a `Retry-After` survives but the HTTP connection is dead on wake; the `select { case <-time.After(…) }` completes, the next request fails with EOF.

**How to avoid:** The existing `peeringdb.Client` uses stdlib `net/http` which creates a fresh TCP connection per request (default transport pools but reconnects on stale). The failure surface is just "one failed request" — the tuple-level retry loop handles it. Do NOT hold a long-lived connection across `time.Sleep`.

### Pitfall 6: JSON key-order instability between runs

**What goes wrong:** Committed anon fixtures re-emitted by a re-capture show zero semantic change but a huge `git diff` because key order shifted.

**Why it happens:** PeeringDB serialises response objects via Python dict, which has insertion order in CPython 3.7+ — generally stable per-endpoint, but undefined across releases.

**How to avoid:** Write raw bytes as-received (D-05 says "raw upstream JSON"). Do NOT re-marshal. A re-capture that sees different ordering is correctly surfacing an upstream change.

## Code Examples

Verified patterns from this codebase.

### Example 1: Detecting rate-limit error

```go
// Source: internal/peeringdb/client.go:32-49, 312-327 (existing)
var rlErr *peeringdb.RateLimitError
if errors.As(err, &rlErr) {
    // rlErr.RetryAfter is already parsed (integer seconds or HTTP-date)
    // rlErr.URL is the failing URL
    // rlErr.Status is always 429
    time.Sleep(rlErr.RetryAfter + 5*time.Second)
    continue
}
```

### Example 2: Structural type classification

```go
// Source: internal/conformance/compare.go:107-124 (existing, adapt for this phase)
func jsonType(v any) string {
    switch v.(type) {
    case nil:          return "null"
    case bool:         return "bool"
    case float64:      return "number"   // json.Number if decoder uses UseNumber
    case string:       return "string"
    case []any:        return "array"
    case map[string]any: return "object"
    }
    return "unknown"
}
```

### Example 3: Per-tuple capture with checkpointing (new, for this phase)

```go
// Pseudocode — for internal/visbaseline/capture.go
type Tuple struct {
    Target string // "beta" | "prod"
    Mode   string // "anon" | "auth"
    Type   string // "poc", "net", …
    Page   int    // 1 or 2
}

func (c *Capture) runTuple(ctx context.Context, t Tuple) error {
    url := fmt.Sprintf("%s/api/%s?limit=%d&skip=%d&depth=0",
        c.baseURL(t.Target), t.Type, pageSize, (t.Page-1)*pageSize)

    for {
        raw, err := c.fetch(ctx, t, url)
        var rlErr *peeringdb.RateLimitError
        if errors.As(err, &rlErr) {
            c.logger.LogAttrs(ctx, slog.LevelWarn, "rate-limited",
                slog.String("tuple", t.String()),
                slog.Duration("retry_after", rlErr.RetryAfter),
            )
            select {
            case <-time.After(rlErr.RetryAfter + 5*time.Second):
            case <-ctx.Done():
                return ctx.Err()
            }
            continue
        }
        if err != nil {
            return fmt.Errorf("fetch %s: %w", t, err)
        }

        if err := c.writeBytes(t, raw); err != nil {
            return err
        }
        return c.checkpoint.Advance(t) // atomic rename
    }
}
```

### Example 4: Redaction pass

```go
// Source: new — internal/visbaseline/redact.go
// Input: raw bytes of an anon response and a raw auth response for the same (type, page).
// Output: bytes of a redacted auth response safe to commit.
//
// Algorithm:
//   1. json.Unmarshal both into map[string]any (envelope).
//   2. Index anon data by id (primary key).
//   3. For each auth row:
//      a. Find the matching anon row by id. If anon row absent → the whole auth row is
//         auth-only. Replace every string value with "<auth-only:string>", every number
//         with a sentinel "<auth-only:number>" (emitted as a JSON string — breaks type
//         but that's what "redacted" means), bools with "<auth-only:bool>". Keep id.
//      b. For each field in the auth row:
//         - If anon row has the same key with the same value → keep the value.
//         - If anon row has the same key with a different value → this is a value drift;
//           still replace with a type placeholder (we do not want to leak values even
//           when they "happen to differ harmlessly" because harmless is subjective).
//         - If anon row lacks the key entirely → this is an auth-only field. Replace
//           its value with the type placeholder. Keep the key.
//   4. Always redact fields in the PII allow-list regardless of whether they appear in
//      anon, as belt-and-braces against upstream bugs that leak PII anonymously.
//   5. Re-serialise with json.MarshalIndent for reviewability.
//
// PII allow-list (from ent schemas + peeringdb/types.go):
//   email, phone, name  (user names on POCs)
//   address1, address2, city, state, zipcode  (org/fac)
//   tech_email, tech_phone, sales_email, sales_phone, policy_email, policy_phone  (ix, fac)
//   latitude, longitude  (org/fac — geocoded from address)
//
// The function is DETERMINISTIC: same inputs → identical output bytes. This is required
// for the "re-run doesn't change git" property.
func Redact(anonBytes, authBytes []byte) ([]byte, error) { … }
```

### Example 5: Diff report JSON schema

```json
{
  "schema_version": 1,
  "generated": "2026-04-16T12:34:56Z",
  "targets": ["beta", "prod"],
  "types": {
    "poc": {
      "anon_row_count": 120,
      "auth_row_count": 145,
      "auth_only_row_count": 25,
      "visible_values_anon": ["Public"],
      "visible_values_auth": ["Public", "Users"],
      "fields": [
        {"name": "email", "auth_only": true, "placeholder": "<auth-only:string>", "rows_added": 25},
        {"name": "phone", "auth_only": true, "placeholder": "<auth-only:string>", "rows_added": 25},
        {"name": "visible", "auth_only": false, "value_set_drift": true}
      ]
    },
    "org": { ... },
    "net": { ... }
  }
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Hardcoded `time.Sleep(3 * time.Second)` in pdbcompat-check | Rate limiter + `Retry-After` parsing in `internal/peeringdb.Client` | v1.13 (2026-04-11) | D-10 mandates reuse of the new path. Legacy sleep still exists at `cmd/pdbcompat-check/main.go:82-84` — the new `--capture` mode must not use it. |
| Retrying 429 inside the client | Short-circuit to caller with `RateLimitError` | v1.13 | Guarantees we never stack retries inside an hour-long Retry-After window. |
| `internal/conformance` structural-only diff | Keep using structural diff; extend for row-count + `visible`-value tracking | This phase | Existing `CompareStructure` is fine for shape; the differ here adds row-level aggregation. |

**Deprecated/outdated in this codebase (do not pattern after):**

- `cmd/pdbcompat-check/main.go:82-84` hardcoded sleep — the new capture mode must route through `internal/peeringdb.Client` which has proper limiting. The existing default path stays as-is (D-01 says default behaviour must not change).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | DRF's `Throttled` exception emits `Retry-After` as integer seconds (not HTTP-date) | Pattern 3, Example 1 | Low — `peeringdb.parseRetryAfter` already handles both forms [VERIFIED: client.go:64-68]. The existing v1.13 production incident (12→1 req/hour on /api/org) confirms integer-seconds is what PeeringDB sends in practice. |
| A2 | `?limit=250&skip=N&depth=0` is the correct URL shape for baseline capture | Pattern 2 | Low — same URL shape already in production use by `internal/peeringdb/stream.go:77`. |
| A3 | The PII field allow-list (email/phone/name/address/lat/long/tech_/sales_/policy_) is exhaustive for the 13 types | Example 4 | **MEDIUM** — derived from `internal/peeringdb/types.go` which mirrors PeeringDB's documented schema, but the phase's whole point is discovering auth-only fields we didn't know about. **Mitigation:** redactor defaults to redacting anything present in auth but absent in anon, so even fields NOT in the allow-list get placeholders. The allow-list is a belt-and-braces defence, not the primary one. |
| A4 | PeeringDB never returns `Private` rows to anonymous callers | Pitfall 2 | Low — documented. If wrong, the capture still succeeds and the diff surfaces it. |
| A5 | Writing raw auth bytes to `/tmp` is acceptable (world-readable on shared hosts) | D-07, Architecture Patterns | MEDIUM on multi-tenant systems. **Mitigation:** use `os.MkdirTemp("", "pdb-vis-capture-*")` (mode 0700) and delete on clean exit. Document this in the DEVELOPMENT doc update. |
| A6 | `json.Marshal` on the redacted result produces stable byte-for-byte output across Go toolchain versions | Example 4 | Low — `encoding/json` sorts map keys in `MarshalIndent` since Go 1.12. Using `json.MarshalIndent` with sorted keys (automatic) makes the output deterministic. |
| A7 | Operators running `--capture` accept a ≥1-hour wall clock | ROADMAP.md + this phase | Low — explicit in STATE.md and ROADMAP.md. |

## Open Questions

1. **Should we commit the diff.json before or after DIFF.md in the same PR?**
   - What we know: Both are outputs of the same run; they must agree.
   - What's unclear: Whether downstream phase 60 test should load diff.json or parse DIFF.md.
   - Recommendation: Commit both together (atomic artifact pair); phase 60 loads `diff.json`. This is already implicit in D-08.

2. **What goes in a "prod" auth capture when we don't have a prod-tier API key?**
   - What we know: D-04 says "run against prod for poc/org/net". An anonymous prod walk is always possible; an authenticated prod walk requires `PDBPLUS_PEERINGDB_API_KEY` aimed at prod.
   - What's unclear: Whether operators have a prod key at hand.
   - Recommendation: Make the `-target=prod` auth mode opt-in via `-prod-auth`. Default to anon-only on prod; if operator passes `-prod-auth` and an api key, do the auth pass. Plan should surface this as a graceful degradation.

3. **Should the Markdown table be one table per type, or a single matrix table?**
   - What we know: D-08 says "per-type Markdown table".
   - What's unclear: Whether "per-type" means one table per type (13 tables) or one table with type as a column.
   - Recommendation: 13 small tables, one per type, with a top-of-file TOC. Easier code review.

## Environment Availability

Single-host requirement — `go build ./cmd/pdbcompat-check` produces a binary the operator runs locally.

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain 1.26+ | Build | ✓ | per go.mod | — |
| Internet access to `beta.peeringdb.com` | Capture loop | operator-dependent | — | Fail fast with DNS error |
| Internet access to `www.peeringdb.com` | Prod confirmation pass | operator-dependent | — | Skip prod with warning if unreachable |
| PeeringDB API key | Auth-mode capture | operator-dependent | — | If absent, run anon-only and emit a partial diff with "auth mode skipped" flag |
| Writable `/tmp` | Checkpoint + raw auth staging | ✓ standard POSIX | — | Fall back to `$TMPDIR` via `os.MkdirTemp("", …)` |

**Missing dependencies with no fallback:** None — capture is best-effort on both targets.

**Missing dependencies with fallback:** API key absence → anon-only capture, diff emits structural shape of anon only.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` (project convention; no third-party runner) |
| Config file | none — stdlib `go test` discovers `*_test.go` |
| Quick run command | `go test -race ./internal/visbaseline/...` |
| Full suite command | `go test -race ./...` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| VIS-01 | Anon fixture is committed as byte-identical upstream JSON | unit | `go test -run TestCaptureWritesRawAnonBytes ./internal/visbaseline/` | ❌ Wave 0 |
| VIS-01 | Auth fixture never contains PII field values | unit | `go test -run TestRedactionStripsPII ./internal/visbaseline/` | ❌ Wave 0 |
| VIS-01 | Redaction is deterministic (same input → same output bytes) | unit | `go test -run TestRedactionDeterministic ./internal/visbaseline/` | ❌ Wave 0 |
| VIS-01 | Redaction handles auth-only rows (whole row absent in anon) | unit | `go test -run TestRedactionAuthOnlyRow ./internal/visbaseline/` | ❌ Wave 0 |
| VIS-01 | Redaction handles auth-only fields (field absent in anon row) | unit | `go test -run TestRedactionAuthOnlyField ./internal/visbaseline/` | ❌ Wave 0 |
| VIS-01 | Capture loop honours RateLimitError.RetryAfter | unit | `go test -run TestCaptureRespectsRateLimit ./internal/visbaseline/` (httptest server returning 429 + Retry-After) | ❌ Wave 0 |
| VIS-01 | Checkpoint persists across a simulated kill | smoke | `go test -run TestCheckpointRoundTrip ./internal/visbaseline/` | ❌ Wave 0 |
| VIS-01 | Checkpoint resume re-runs ONLY unfinished tuples | smoke | `go test -run TestCheckpointResumeSkipsDoneTuples ./internal/visbaseline/` | ❌ Wave 0 |
| VIS-01 | Checkpoint prompt defaults are safe (EOF input → Restart not Resume when state looks corrupted) | unit | `go test -run TestCheckpointPromptSafeDefaults ./internal/visbaseline/` | ❌ Wave 0 |
| VIS-02 | Diff emits identical structure → empty diff.json and DIFF.md with "no deltas" | integration | `go test -run TestDiffNoDeltas ./internal/visbaseline/` (golden fixtures) | ❌ Wave 0 |
| VIS-02 | Diff emits auth-only fields with correct placeholder, no values | integration | `go test -run TestDiffAuthOnlyField ./internal/visbaseline/` | ❌ Wave 0 |
| VIS-02 | Diff emits row-count drift correctly | integration | `go test -run TestDiffRowCountDrift ./internal/visbaseline/` | ❌ Wave 0 |
| VIS-02 | DIFF.md and diff.json agree (same inputs → consistent outputs) | integration | `go test -run TestReportConsistency ./internal/visbaseline/` | ❌ Wave 0 |
| VIS-02 | PII guard: assert no email/phone/name value in any committed auth fixture | integration | `go test -run TestCommittedFixturesHaveNoPII ./internal/visbaseline/` (walks `testdata/visibility-baseline/**/auth/` and grep-asserts placeholders only) | ❌ Wave 0 |

### Sampling Rate

- **Per task commit:** `go test -race ./internal/visbaseline/...` (< 5s, all unit + integration tests use `httptest` — no live network).
- **Per wave merge:** `go test -race ./...` (full suite).
- **Phase gate:** Full suite green + a single manual `pdbcompat-check -capture -target=beta -mode=both` run by the operator before `/gsd-verify-work`. This manual run is the "live test" — it is intentionally not a CI job (D-03 note: "CI does not run it").

### Wave 0 Gaps

- [ ] `internal/visbaseline/capture.go` — scaffolding for the `Capture` type
- [ ] `internal/visbaseline/capture_test.go` — httptest server + fixtures
- [ ] `internal/visbaseline/checkpoint.go` + `_test.go`
- [ ] `internal/visbaseline/redact.go` + `_test.go`
- [ ] `internal/visbaseline/diff.go` + `_test.go`
- [ ] `internal/visbaseline/report.go` + `_test.go`
- [ ] `internal/visbaseline/pii.go` (PII allow-list — shared by redactor and guard test)
- [ ] `internal/visbaseline/testdata/` — synthetic anon/auth JSON pairs for golden tests (tiny, 1-2 rows each, fully fake data — not sourced from real PeeringDB)

No framework install required — `go test` and `net/http/httptest` are stdlib.

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | yes | Reuse existing `peeringdb.WithAPIKey`; API key sourced from env var `PDBPLUS_PEERINGDB_API_KEY` or `-api-key` flag. Never log the key. |
| V3 Session Management | no | Single-shot CLI, no sessions. |
| V4 Access Control | yes | The entire phase is about discovering upstream's access-control behaviour. Redactor enforces repo-side access control (no PII in git). |
| V5 Input Validation | yes | All JSON decoded via `encoding/json` with defensive `map[string]any` handling; no `ioutil.ReadAll` without a size cap — the existing client already has a 30s timeout. |
| V6 Cryptography | no | No new crypto introduced. API key travels via existing `Authorization: Api-Key` header over HTTPS (already enforced by `internal/peeringdb.Client`). |
| V7 Error Handling | yes | Errors MUST NOT echo API key or raw auth bytes. Use `%w` wrapping per GO-ERR-1 but never embed `raw` in error messages. |
| V8 Data Protection | **CRITICAL** | The PII in auth responses is exactly the data we must never commit. See redactor + PII guard test. |
| V9 Communications | yes | HTTPS mandatory; the `peeringdb.Client` uses `otelhttp.NewTransport(http.DefaultTransport)` — no downgrade path. |

### Known Threat Patterns for this phase

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| PII leaked to git via raw auth fixture commit | Information Disclosure | (1) raw auth bytes only under `/tmp` or gitignored path; (2) redaction allow-list; (3) committed-fixture grep test fails CI if placeholder absent on known PII field |
| API key leaked to logs or error messages | Information Disclosure | Structured slog: never include `c.apiKey` or `req.Header` as an attribute. The existing client does not — match the pattern. |
| Redactor bug causes unredacted value to leak | Information Disclosure | Redactor is a pure function with extensive unit tests; PII guard test scans committed fixtures and fails if any field name in the PII allow-list has a non-placeholder string value |
| Checkpoint file contains PII | Information Disclosure | Checkpoint stores only (target, mode, type, page) tuples + status enums — never response bytes. Document this explicitly in the `State` struct comment. |
| Raw auth dir in `/tmp` survives and leaks | Information Disclosure | `os.MkdirTemp("", "pdb-vis-capture-*")` gives mode 0700. On clean exit, `os.RemoveAll`. Document that operators should manually clean if they Ctrl+C. |
| 429 on prod triggers retry storm | Denial of Service (of our own quota) | Already solved: `RateLimitError` path + sleep-on-retry-after. Do NOT add a second retry layer. |

## Sources

### Primary (HIGH confidence)

- **[VERIFIED: internal/peeringdb/client.go:32-393]** — `RateLimitError`, `parseRetryAfter`, `NewClient`, `WithAPIKey`, rate limiter switching on key presence, `doWithRetry` 429 short-circuit.
- **[VERIFIED: internal/peeringdb/stream.go:35-207]** — `StreamAll`, pagination loop, `limit&skip&depth` URL shape, both `{meta,data}` key orderings.
- **[VERIFIED: internal/peeringdb/types.go:1-336]** — canonical Go structs for all 13 types; source of the PII field allow-list.
- **[VERIFIED: internal/conformance/compare.go:1-161]** — reference pattern for structural diff (type classification, path building, deterministic sort).
- **[VERIFIED: cmd/pdbcompat-check/main.go:1-195]** — existing tool layout; D-01 says extend this, not replace.
- **[CITED: https://docs.peeringdb.com/howto/work_within_peeringdbs_query_limits/]** — 20/min anon, 40/min auth, response-size-based sub-throttle on repeat requests.
- **[CITED: https://docs.peeringdb.com/blog/contacts_marked_private/]** — POC visibility values (Public, Users) and anonymous vs authenticated semantics.
- **[VERIFIED: CLAUDE.md]** — stdlib-first, stdlib `flag`, stdlib `log/slog`, modernc.org/sqlite (not relevant here), OTel mandatory, testdata layout.
- **[VERIFIED: .gitignore]** — existing patterns; need to add `testdata/visibility-baseline/**/.raw-auth/` (or equivalent) and `/tmp/pdb-vis-capture-*`.

### Secondary (MEDIUM confidence)

- **[CITED: PeeringDB GitHub src/peeringdb_server/rest_throttles.py]** — DRF-based throttle. Uses `ResponseSizeThrottle`, `APIAnonUserThrottle`, `APIUserThrottle`. Rate values stored in DB settings, not hardcoded. Confirms our client's 429 + Retry-After handling matches upstream. [Source: https://github.com/peeringdb/peeringdb — upstream source, not Context7]
- **[CITED: Django REST Framework docs]** — DRF's `Throttled` exception emits `Retry-After` as an integer seconds value. Assumed via A1; corroborated by v1.13 production incident where the integer-seconds path fired correctly.

### Tertiary (LOW confidence)

- None. All claims in this research are either verified in the local codebase or cited to official PeeringDB documentation.

## Project Constraints (from CLAUDE.md)

These directives from the project's CLAUDE.md apply and the planner MUST verify task compliance:

- **Go 1.26+** — use modern stdlib features where appropriate.
- **Stdlib-first** — do not introduce new Go module dependencies. `flag`, `encoding/json`, `log/slog`, `os`, `context` cover this phase.
- **OpenTelemetry mandatory** — capture loop must create OTel spans for each fetched tuple. Reuse `otelhttp.NewTransport` which is already wired in `peeringdb.Client` — no new tracer plumbing needed inside `internal/visbaseline`.
- **Structured slog logging** — use `slog.LogAttrs(ctx, level, msg, attrs...)`; prefer typed attribute setters (`slog.String`, `slog.Int`, `slog.Duration`) per GO-OBS-5.
- **Fail-fast config** — missing API key in auth mode is an error, not a silent fallback, unless operator explicitly passes `-anon-only`.
- **Generated code drift check** — this phase touches no `ent/`, `gen/`, `graph/`, or `internal/web/templates/`; CI drift check is not a concern.
- **Testing convention** — live tests gated by `-peeringdb-live` flag. This phase's tests run against `httptest.NewServer` and are NOT live tests. The manual `pdbcompat-check -capture -target=beta` run is not a `go test` at all — it's an operator action.
- **GSD workflow enforcement** — all repo edits go through the GSD workflow. Implementation happens via `/gsd:execute-phase`.
- **User global go-guidelines** — GO-MD-1 (stdlib preference), GO-CS-5 (input structs for >2 args), GO-ERR-1 (`%w` wrapping), GO-CTX-1 (ctx first), GO-OBS-1 (structured logging), GO-SEC-1 (validate inputs, HTTPS), GO-SEC-2 (never log secrets).

## Metadata

**Confidence breakdown:**

- Standard stack: HIGH — everything reused from the existing codebase; no unverified external libraries.
- Architecture: HIGH — layout mirrors existing `internal/` package conventions; each concern is already proven in adjacent code (capture ↔ sync worker, diff ↔ conformance, checkpoint ↔ sync state).
- Pitfalls: HIGH for rate-limit and PII (both well-covered upstream + in v1.13 lessons); MEDIUM for PII allow-list exhaustiveness (mitigated by the "redact anything present in auth but absent in anon" default).
- Diff algorithm: HIGH — the shape is narrower than `internal/conformance.CompareStructure` but uses the same primitives.
- Resumability: HIGH — single-file atomic rename is stdlib-trivial; edge cases enumerated in Pattern 4.

**Research date:** 2026-04-16
**Valid until:** 2026-05-16 (30 days — PeeringDB API shape is stable; rate-limit values could change server-side but the `Retry-After` response shape is stable via DRF)
