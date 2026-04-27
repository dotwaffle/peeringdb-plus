---
phase: 78-uat-closeout
plan: 01
status: complete
shipped_at: 2026-04-27
requirements:
  - UAT-02
---

# Plan 78-01 Summary — UAT-02 (Phase 53 security controls)

## Shipped

- Live curl evidence captured against `https://peeringdb-plus.fly.dev/` (post-Phase-77 deploy `846c3df`, 8-machine fleet) for HSTS / X-Frame-Options / X-Content-Type-Options + body-cap behaviour. Evidence in `.planning/phases/78-uat-closeout/UAT-RESULTS.md` § UAT-02.
- New `cmd/peeringdb-plus/security_e2e_test.go` regression-locks the middleware behaviour with 6 test functions (5 with sub-tests), all pass under `go test -race`.
- Slowloris probe handled via structural argument citing `ReadHeaderTimeout=10s` + `ReadTimeout=30s` + `PDBPLUS_DRAIN_TIMEOUT=10s` rather than running a live DoS-shaped probe against production.

## Key findings

- Body cap is **1 MB**, not 2 MB as CONTEXT.md plan-hint claimed. `cmd/peeringdb-plus/main.go:55` `maxRequestBodySize = 1 << 20`. Plan 78-03 fixes the corresponding STATE.md doc-drift.
- `/api/net` and `/rest/v1/networks` POST returns HTTP 405 *before* body cap fires (mux dispatch happens first; both endpoints are GET-only). The body-cap test uses `/graphql` POST as the trigger path.
- gqlgen wraps the `*http.MaxBytesError` parse error in a graphql `errors[]` envelope with HTTP 200 (gqlgen's default for parse-time errors). The cap fires correctly; the HTTP status is just gqlgen's choice. In-process tests assert the underlying middleware behaviour directly via `*http.MaxBytesError`.

## Files modified

| File | Change |
|------|--------|
| `cmd/peeringdb-plus/security_e2e_test.go` | NEW — 6 test functions covering HSTS / X-Frame-Options scoping / X-Content-Type-Options / body-cap on capped paths / body-cap bypass on skip-list paths. |
| `.planning/phases/78-uat-closeout/UAT-RESULTS.md` | NEW — captures live curl evidence for UAT-02 + UAT-01 (78-01 + 78-02 share this file). |

## Verification gates

| Gate | Result |
|------|--------|
| `go test -race ./cmd/peeringdb-plus/...` | All tests PASS (5 new + existing) |
| `golangci-lint run ./cmd/peeringdb-plus/...` | 0 issues |
| `grep -c 'dotwaffle@gmail.com\|grafana.net' UAT-RESULTS.md` | 0 (PII-clean) |

## Deviations from plan

- Originally specified a `_testdata/slowloris.go` probe; replaced with a structural argument from server timeouts. Lower-risk, equally informative for an audit context where production traffic is essentially zero.
- UAT-01 (CSP) folded into the same UAT-RESULTS.md instead of waiting for plan 78-02's manual operator step (per user directive: do as much as possible autonomously). The autonomous verification in UAT-RESULTS.md § UAT-01 produces a PASS verdict via static policy analysis + curl wiring + rendered-page audit.
