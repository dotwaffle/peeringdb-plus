---
task: 260419-ski-auth-sync-interval
completed: 2026-04-19
type: quick
---

# Auth-conditional sync interval default — Summary

## What changed

`PDBPLUS_SYNC_INTERVAL` now has an auth-conditional default:

| API key set? | Explicit override? | Effective interval |
|---|---|---|
| no  | no  | `1h`            |
| no  | yes | override value  |
| yes | no  | `15m`           |
| yes | yes | override value  |

Rationale: authenticated callers have a much higher PeeringDB rate-limit budget,
so the tighter cadence keeps the mirror fresher without risking throttling;
unauthenticated deployments stay on the conservative 1h default.

## Files modified

- `internal/config/config.go` — replaced `parseDuration(...)` call with a
  switch over `os.LookupEnv("PDBPLUS_SYNC_INTERVAL")` + `cfg.PeeringDBAPIKey`
  state; added a single `slog.Info("sync interval configured", ...)` at the
  end of `Load()` with `interval`, `authenticated`, `explicit_override` attrs
  (the API key itself is never logged).
- `internal/config/config_test.go` — added `TestLoad_SyncInterval_AuthConditional`
  (4 subtests: `unauth_default` / `unauth_explicit` / `auth_default` /
  `auth_explicit`).
- `CLAUDE.md` — env-var table row for `PDBPLUS_SYNC_INTERVAL` updated with
  the auth-conditional default.
- `docs/CONFIGURATION.md` — same default change + new "Sync cadence"
  subsection explaining the rationale and override precedence.

## Verification

- `go build ./...` — clean.
- `go vet ./...` — clean.
- `go test -race ./internal/config/...` — PASS, 1.0s. All new and existing
  `TestLoad_SyncInterval*` subtests pass.
- `golangci-lint run ./internal/config/...` — 0 issues.

## Non-goals respected

- Rate-limit handling unchanged (already in place from v1.13 Phase 51).
- No runtime re-evaluation when the key is rotated — config remains immutable
  after init (GO-CFG-2).

## Commit

Single atomic commit per PLAN.md template.
