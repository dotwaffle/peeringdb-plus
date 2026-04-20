---
task: 260419-ski-auth-sync-interval
created: 2026-04-19
type: quick
---

# Auth-conditional sync interval default

## Objective

Make `PDBPLUS_SYNC_INTERVAL` default to **15 minutes** when `PDBPLUS_PEERINGDB_API_KEY` is set (authenticated), keep the current **1 hour** default when the API key is empty (unauthenticated). Explicit `PDBPLUS_SYNC_INTERVAL=...` overrides win regardless of auth state.

Rationale: authenticated callers have a much higher PeeringDB rate limit budget; unauthenticated 1h default stays conservative against the shared anonymous ceiling.

## Scope

### Code (`internal/config/config.go`)

1. Replace the existing `parseDuration("PDBPLUS_SYNC_INTERVAL", 1*time.Hour)` call with logic that:
   - Uses `os.LookupEnv("PDBPLUS_SYNC_INTERVAL")` to detect whether the var is explicitly set.
   - If set: parse the value (existing `parseDuration` logic path — error on invalid).
   - If not set: default to `15 * time.Minute` when `cfg.PeeringDBAPIKey != ""`, else `1 * time.Hour`.
2. After `cfg` is fully populated and validated, emit one structured `slog.Info` line showing the effective interval + auth mode:
   ```go
   slog.Info("sync interval configured",
       slog.Duration("interval", cfg.SyncInterval),
       slog.Bool("authenticated", cfg.PeeringDBAPIKey != ""),
       slog.Bool("explicit_override", intervalExplicit),
   )
   ```
   Place at the end of `Load()` just before `return cfg, nil`. Do NOT log the API key value itself.

### Test (`internal/config/config_test.go`)

Add `TestLoad_SyncInterval_AuthConditional` with 4 sub-tests:

| Sub-test | API_KEY | SYNC_INTERVAL | Expected |
|---|---|---|---|
| `unauth_default` | unset | unset | `1h` |
| `unauth_explicit` | unset | `"30m"` | `30m` |
| `auth_default` | `"test-key"` | unset | `15m` |
| `auth_explicit` | `"test-key"` | `"45m"` | `45m` |

Use `t.Setenv` for isolation. Assert `cfg.SyncInterval` == expected.

### Docs

- **`CLAUDE.md`** — env-var table row for `PDBPLUS_SYNC_INTERVAL`: change default cell from `1h` to `1h (unauthenticated) / 15m (when PDBPLUS_PEERINGDB_API_KEY is set)`.
- **`docs/CONFIGURATION.md`** — same change + a short paragraph explaining the auth-conditional default and the override precedence.

## Out of scope

- Rate-limit handling changes (already in place from v1.13 Phase 51).
- Runtime re-evaluation when the key is rotated — config is immutable after init (GO-CFG-2).

## Verification

- `go build ./...`
- `go vet ./...`
- `go test -race -run TestLoad_SyncInterval ./internal/config/...`
- `golangci-lint run ./internal/config/...`

## Commit

Single atomic commit:

```
feat(config): auth-conditional SYNC_INTERVAL default (15m authenticated / 1h unauthenticated)

PDBPLUS_SYNC_INTERVAL now defaults to 15m when PDBPLUS_PEERINGDB_API_KEY
is set, 1h when unset. Explicit override via PDBPLUS_SYNC_INTERVAL
wins regardless of auth state.

Startup slog line announces the effective interval + auth mode.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
```
