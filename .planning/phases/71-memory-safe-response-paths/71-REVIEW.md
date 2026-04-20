---
phase: 71
slug: memory-safe-response-paths
milestone: v1.16
reviewed_at: 2026-04-19
depth: standard
status: findings_found
counts:
  critical: 0
  high: 0
  medium: 3
  low: 0
  info: 7
---

# Phase 71 — Code Review

Phase 71 architecture is sound: pre-flight budget check, streaming envelope, per-request telemetry all correctly wired. No critical/high findings. 3 hardening warnings + 7 info-level nits.

## Warnings

### WR-01 — Missing CountFunc silently disables budget check
- **File:** `internal/pdbcompat/handler.go:262`
- **Issue:** Budget pre-flight is gated by `tc.Count != nil`. Future entity added to `Registry` without `CountFunc` silently bypasses Phase 71 protection.
- **Fix:** `init()` invariant — `panic` if any Registry entry has `List` but nil `Count`.

### WR-02 — Rowsize calibration not CI-enforced
- **File:** `internal/pdbcompat/rowsize.go`
- **Issue:** Hardcoded 2× values are correct today but nothing fails if a serializer grows a field without recalibration.
- **Fix:** Non-`-short` drift test that runs a seed.Full row through `json.Marshal` and fails if `typicalRowBytes[X].Depth0 < 2 × measured_mean`.

### WR-03 — Stream/middleware gzip compatibility untested
- **File:** `internal/pdbcompat/stream.go`
- **Issue:** Flush cadence tested via `httptest.NewRecorder` (no middleware). Full chain (`gzip -> ...`) might buffer and defeat streaming.
- **Fix:** Integration test exercising full middleware stack with 10k-row response; assert no `Content-Length` header, `http.Flusher` honoured end-to-end.

## Info

- **IN-01** `stream.go` discards `ctx` for pull-iterator future.
- **IN-02** OTel histogram `Unit: "KiB"` is non-canonical (prefer `By` or `KiBy`).
- **IN-03** Budget problem body encode error silently swallowed.
- **IN-04** `FlushEvery = 100` lacks empirical justification in godoc.
- **IN-05** `endpoint` label cardinality — pass `/api/<name>` not raw `r.URL.Path`.
- **IN-06** CountFunc doc should note the extra `COUNT(*)` roundtrip.
- **IN-07** Bench uses deprecated `for i := 0; i < b.N` form; Go 1.24 idiom is `for b.Loop()`.
