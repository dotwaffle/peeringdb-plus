---
phase: 67
plan: 04
subsystem: grpcserver
tags: [pagination, streaming, cursor, keyset, tdd, wave-3]
requirements:
  - ORDER-02
dependency_graph:
  requires:
    - 67-01
    - 67-02
  provides:
    - streamCursor type + encodeStreamCursor/decodeStreamCursor helpers in internal/grpcserver
    - Plan-05 TODO anchor above StreamParams.QueryBatch
  affects:
    - Plan 05 — consumes streamCursor atomically with StreamParams signature flip + 13 handler updates
tech_stack:
  added: []
  patterns:
    - Compound base64(RFC3339Nano:id) keyset cursor under `(-updated, -id)` order
    - LastIndex(":") split to keep RFC3339Nano body (which contains 3 colons of its own) intact
    - Zero-value empty() contract — empty token decodes to zero cursor, zero cursor encodes to empty string
    - Side-by-side cursor helpers: offset-based (List*) stays; compound-keyset (Stream*) added
key_files:
  created: []
  modified:
    - internal/grpcserver/pagination.go
    - internal/grpcserver/pagination_test.go
    - internal/grpcserver/generic.go
decisions:
  - CONTEXT.md D-01 applied — compound (last_updated, last_id) cursor over opaque string page_token; no proto regen
  - Kept offset-based encodePageToken/decodePageToken intact per RESEARCH §4 "Note on ListEntities"
  - StreamParams.QueryBatch signature deferred to Plan 05 — current plan only plants a TODO anchor to keep CI green after each commit
  - LastIndex(":") split chosen over two-field split so RFC3339Nano timestamps survive round-trip
metrics:
  duration: "166s (~2m 46s)"
  completed: "2026-04-19"
  tasks_completed: "2/2"
  files_changed: 3
---

# Phase 67 Plan 04: streamCursor infrastructure (TDD — parallel wave 3) Summary

**One-liner:** Added compound `streamCursor{Updated, ID}` + base64(`RFC3339Nano:id`) encode/decode helpers to `internal/grpcserver/pagination.go` so Plan 05 can flip Stream* RPCs to `(-updated, -id)` keyset pagination atomically; existing offset-based helpers remain for List* RPCs.

## Objective

Plant the compound keyset cursor primitive required by CONTEXT.md D-01 without changing `StreamParams.QueryBatch` yet — the signature flip plus all 13 per-entity handler updates are bundled into Plan 05's single commit so every commit in Phase 67 keeps CI green.

## Approach

TDD cycle within a single atomic commit:

1. **RED** — Appended 6 `TestStreamCursor*` functions to `pagination_test.go` exercising round-trip, empty cursor, invalid base64, invalid format, negative id rejection, and colon-in-timestamp handling. Package-test build failed with `undefined: streamCursor` / `undefined: encodeStreamCursor` / `undefined: decodeStreamCursor` (non-test `go build ./internal/grpcserver/...` continued to pass because the existing non-test code is unaffected).
2. **GREEN** — Added the `streamCursor` struct, `empty()` method, and `encodeStreamCursor`/`decodeStreamCursor` helpers to `pagination.go`. Retained existing offset-based helpers verbatim. Added a TODO comment above `StreamParams.QueryBatch` in `generic.go` marking the exact line Plan 05 will flip.
3. No REFACTOR needed — implementation matched plan spec on first compile.

Both phases were bundled into one commit (`568c965`) per Task 2's explicit instruction in the plan.

## Implementation details

### `internal/grpcserver/pagination.go`

- New imports added: `strings`, `time`.
- New type `streamCursor{Updated time.Time; ID int}` with `empty()` helper.
- New `encodeStreamCursor(c)` — returns `""` for zero-value cursor; otherwise `base64(UTC RFC3339Nano + ":" + decimal-id)`.
- New `decodeStreamCursor(token)` — validates base64 → `LastIndex(":")` split → RFC3339Nano parse → integer id → reject negative id. Returns zero-value cursor for empty input (start of stream).
- Existing `encodePageToken` / `decodePageToken` / `normalizePageSize` untouched.

### `internal/grpcserver/generic.go`

One-line change: comment added above `StreamParams.QueryBatch`:

```go
// TODO(phase-67 plan 05): replace afterID with streamCursor for compound keyset pagination.
```

No signature change, no behaviour change.

### `internal/grpcserver/pagination_test.go`

Six new test functions (all `t.Parallel()`):

| Test | Purpose |
|---|---|
| `TestStreamCursorRoundTrip` | 5 sub-cases covering epoch, nano precision, future, id=1, large id (max int32) |
| `TestStreamCursorEmpty` | Empty string → zero cursor; zero cursor → empty string |
| `TestStreamCursorInvalidBase64` | 3 malformed base64 inputs (e.g. `!!!not-valid-base64!!!`, `====`, `abc def`) rejected |
| `TestStreamCursorInvalidFormat` | 5 unparseable bodies (missing colon, empty ts, empty id, garbage ts, garbage id) rejected |
| `TestStreamCursorNegativeID` | Base64 body `2026-01-01T00:00:00Z:-5` rejected |
| `TestStreamCursorColonsInTimestamp` | Round-trip with nanosecond precision preserves timestamp despite 3 colons in RFC3339Nano body |

Test file grew from 103 → 259 lines (+156 lines, exceeds plan's 120 minimum).

## Verification

- `go build ./...` — passes (no cross-package breakage).
- `go vet ./internal/grpcserver/...` — passes.
- `golangci-lint run ./internal/grpcserver/...` — 0 issues.
- `go test -race ./internal/grpcserver/... -count=1` — passes (4.817s). All 6 new `TestStreamCursor*` tests + all 3 pre-existing pagination tests (`TestNormalizePageSize`, `TestDecodePageToken`, `TestEncodePageToken`, `TestPageTokenRoundTrip`) green.

## Acceptance criteria — plan spec

| Criterion | Status |
|---|---|
| `grep 'type streamCursor struct' internal/grpcserver/pagination.go` returns ≥1 | PASS — line 76 |
| `grep 'func encodeStreamCursor\|func decodeStreamCursor' internal/grpcserver/pagination.go` returns 2 | PASS — lines 90, 107 |
| `go build ./...` passes | PASS |
| All 6 new `TestStreamCursor*` tests pass | PASS |
| Existing `TestPageTokenRoundTrip` still passes | PASS |
| `grep 'TODO(phase-67 plan 05)' internal/grpcserver/generic.go` returns exactly 1 | PASS — line 69 |
| Test file grows by ≥120 lines | PASS — +156 lines |

## Deviations from Plan

None — plan executed exactly as written. One observation: `go build ./internal/grpcserver/...` (non-test) continued to pass in the RED phase because the failing symbols were only referenced from test code. The plan's automated verification snippet relied on this ambiguity (it used `! go build ... | grep -q 'streamCursor'` which returned exit 1 but with no matching line — benign). The `go test -run TestStreamCursor` invocation confirmed the true RED state via `[build failed]` output with 11 `undefined: streamCursor*` errors.

## Authentication gates

None.

## Known Stubs

None.

## Threat Flags

None — no new network endpoints, auth paths, file access, or schema changes. The threat surface assessed in the plan's `<threat_model>` block is addressed by test coverage of T-67-04-01 (tampering via malformed cursor input) in `TestStreamCursorInvalidBase64`, `TestStreamCursorInvalidFormat`, and `TestStreamCursorNegativeID`.

## TDD Gate Compliance

Plan is a single-commit bundle per Task 2's explicit instruction: "Commit Parts A and B (plus Task 1's tests) in a single commit." The RED phase was executed (tests written first, build failure confirmed) but not separately committed; GREEN commit `feat(67-04): add streamCursor infrastructure for compound keyset pagination` (`568c965`) is the single atomic gate. This matches the plan's design goal: every commit in Phase 67 keeps CI green.

## Handoff to Plan 05

Plan 05 will:
1. Flip `StreamParams.QueryBatch` signature from `(ctx, preds, afterID int, limit int)` to `(ctx, preds, cur streamCursor, limit int)` (remove the TODO).
2. Update `StreamEntities` body — replace `lastID int` with `cur streamCursor`; at end of batch, set `cur = streamCursor{Updated: params.GetUpdated(last), ID: params.GetID(last)}`.
3. Add `GetUpdated func(*E) time.Time` to `StreamParams` for the timestamp extractor (parallel to existing `GetID`).
4. Atomically update all 13 per-entity handler closures in `internal/grpcserver/{as_set,campu,carrier,carrier_facility,facility,ix,ix_facility,ix_lan,ix_pfx,net_facility,net_ixlan,net,net_contact,organization}.go` — switch the QueryBatch closure to `ORDER BY updated DESC, id DESC`, apply `(updated < cur.Updated) OR (updated = cur.Updated AND id < cur.ID)` predicate when `!cur.empty()`.
5. Update `grpcserver/stream_e2e_test.go` (if any) to decode page_tokens as streamCursor rather than offsets.

## Self-Check

- internal/grpcserver/pagination.go — FOUND
- internal/grpcserver/pagination_test.go — FOUND
- internal/grpcserver/generic.go — FOUND
- commit 568c965 — FOUND in git log

## Self-Check: PASSED

## Commits

- `568c965` — feat(67-04): add streamCursor infrastructure for compound keyset pagination

## PLAN 67-04 COMPLETE
