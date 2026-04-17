---
phase: 59-ent-privacy-policy-sync-bypass
plan: 02
subsystem: config
tags: [config, env-var, fail-fast, privacy, wave-2]
requirements: [SYNC-03]
dependency_graph:
  requires:
    - internal/privctx (Wave 1, plan 59-01)
  provides:
    - Config.PublicTier (privctx.Tier) populated at Load()
    - parsePublicTier strict validator (case-sensitive lowercase)
  affects:
    - internal/middleware/ (plan 59-03 reads cfg.PublicTier)
    - cmd/peeringdb-plus/main.go (plan 59-06 wires middleware)
tech_stack:
  added: []
  patterns:
    - "strict-switch env-var parser (mirrors parseSyncMode)"
    - "fail-safe-closed: typos error at startup, not silent default"
key_files:
  created: []
  modified:
    - internal/config/config.go
    - internal/config/config_test.go
decisions:
  - "Case-sensitive lowercase only (D-12): 'Users', 'PUBLIC', 'public ' all fail fast — matches PDBPLUS_SYNC_MODE=full|incremental convention"
  - "parsePublicTier mirrors parseSyncMode structure verbatim so reviewers audit one pattern, not two"
  - "Strict switch (not strings.ToLower + parse) is the T-59-05 mitigation: a typo cannot silently escalate or silently default to either tier"
metrics:
  duration: ~10m
  completed: 2026-04-16
  tasks: 1
  files_changed: 2
  commits: 2
---

# Phase 59 Plan 02: PDBPLUS_PUBLIC_TIER Config Parser Summary

PDBPLUS_PUBLIC_TIER is now parsed at `config.Load()` into `Config.PublicTier`, a `privctx.Tier` whose value feeds the Wave 3 tier-stamping middleware. Strict lowercase switch; mis-cased or unknown values fail fast per GO-CFG-1.

## What Shipped

- **`Config.PublicTier privctx.Tier`** — new field in the Config struct, grouped with `SyncMode` because both are strict-enum env-driven knobs parsed at startup and immutable thereafter.
- **`parsePublicTier(key, defaultVal)`** — adjacent to `parseSyncMode`. Accepts: empty/unset → `defaultVal`; `"public"` → `privctx.TierPublic`; `"users"` → `privctx.TierUsers`; any other value → `fmt.Errorf("invalid value %q for %s: must be 'public' or 'users'", v, key)`.
- **Load() wiring** — single call right after `parseSyncMode`, error-wrapped with `%w` and the env-var name so operators can diagnose from the first log line.
- **Four tests** — `TestLoad_PublicTierDefault`, `TestLoad_PublicTierPublicExplicit`, `TestLoad_PublicTierUsers`, `TestLoad_PublicTierInvalid` (table-driven: `"Users"`, `"admin"`, `"public "`, `"PUBLIC"`, `"anon"`). All use `t.Setenv` for hermetic env control per GO-T-1.

## Gate Sequence (TDD)

| Gate | Commit | Verified |
|------|--------|----------|
| RED  | `b8364cf` test(59-02): add failing tests for PDBPLUS_PUBLIC_TIER parser | Build fails: `cfg.PublicTier undefined` |
| GREEN | `58691c1` feat(59-02): add PDBPLUS_PUBLIC_TIER parser and Config.PublicTier field | `go test -race ./internal/config/... -run 'TestLoad_PublicTier'` passes 4 tests (9 including sub-tests) |

No REFACTOR commit — the code is already minimal and mirrors the established `parseSyncMode` pattern exactly; refactoring would diverge from the pattern the plan required.

## Acceptance Criteria

| Criterion | Result |
|-----------|--------|
| `go test -race ./internal/config/... -run 'TestLoad_PublicTier'` | PASS (4 tests, 5 invalid sub-tests, all green) |
| `go test -race ./internal/config/...` (full package) | PASS — no regressions |
| `grep "PublicTier privctx.Tier" internal/config/config.go` | 1 match (struct field) |
| `grep "func parsePublicTier" internal/config/config.go` | 1 match |
| `grep "PDBPLUS_PUBLIC_TIER" internal/config/config.go` | 5 matches (2 code: Load call + error wrap; 3 docstring references — informational) |
| `grep "must be 'public' or 'users'" internal/config/config.go` | 1 match |
| `go vet ./internal/config/...` | clean |
| `golangci-lint run ./internal/config/...` | 0 issues |
| `go build ./...` | clean |

## Deviations from Plan

None — plan executed exactly as written. Plan's "expected 2 matches" for `PDBPLUS_PUBLIC_TIER` grep was a lower bound; actual is 5 (2 code references + 3 doc-comment references for operator clarity). All code references match the planned wiring.

## Threat Mitigation Applied

- **T-59-05 (Tampering / DoS / fail-open on misconfig)** — mitigated as planned. The strict-switch validator (not `strings.ToLower` + parse) means `PDBPLUS_PUBLIC_TIER=Users` (capital U) produces a hard error at `Load()`, not a silent default to either tier. `main.go`'s existing `os.Exit(1)` on config error completes the fail-fast chain per GO-CFG-1.

## Self-Check: PASSED

- File `internal/config/config.go` — FOUND, contains `PublicTier privctx.Tier`, `func parsePublicTier`, Load() call.
- File `internal/config/config_test.go` — FOUND, contains all four new test functions.
- Commit `b8364cf` (RED) — FOUND in git log.
- Commit `58691c1` (GREEN) — FOUND in git log.
- `go test -race ./internal/config/...` — green.
- `go vet ./internal/config/...` — clean.
- `golangci-lint run ./internal/config/...` — 0 issues.

## Downstream Handoffs

- **Plan 59-03** (middleware) — reads `cfg.PublicTier` at middleware construction, stamps every inbound request via `privctx.WithTier`.
- **Plan 59-06** (main wiring) — passes `cfg.PublicTier` into the middleware constructor.
- **Phase 61** (observability) — will surface the resolved tier as an OTel span attribute `pdbplus.privacy.tier`.
- **Phase 62** (docs) — adds `PDBPLUS_PUBLIC_TIER` row to the CLAUDE.md + `docs/CONFIGURATION.md` env var tables.
