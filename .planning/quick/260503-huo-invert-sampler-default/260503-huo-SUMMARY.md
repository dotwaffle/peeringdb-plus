---
phase: 260503-huo-invert-sampler-default
plan: 01
status: complete
commit: 03cc619
files-changed:
  - internal/otel/provider.go
  - internal/otel/sampler_test.go
  - internal/otel/provider_test.go
  - docs/ARCHITECTURE.md
  - docs/CONFIGURATION.md
---

# 260503-huo: Invert per-route OTel head sampler default

Single atomic commit `03cc619` on branch `worktree-agent-a67e314cc7945836d`,
based on pre-dispatch commit `3b3c18e`.

## Incident motivation

A vulnerability scanner from `45.148.10.238` (User-Agent
`SecurityScanner/1.0`) hammered ~9M spans/hour against `/.env`, `/.git/*`,
`/.aws/*`, `/.kube/*`, `/phpinfo.php`, `/wp-admin`, `/wp-login.php` on
2026-05-02. The scanner consumed ~2 GB of the 50 GB/month free-tier trace
budget on day 2 of the month and tripped `live_traces_exceeded` discards in
Grafana Cloud Tempo, peaking at 384 KB/s vs the free-tier 500 KB/s cap.

The pre-existing per-route sampler in `internal/otel/provider.go` used
`DefaultRatio = in.SampleRate` (default `1.0`), so any URL path that did
not match an explicit prefix entry was traced at 100%. The scanner's
randomised dotfile / WordPress / phpinfo paths fell through that gap.

## Policy change (before / after)

| Route prefix | Before | After |
|--------------|--------|-------|
| (default — unknown paths, sync worker, internal spans) | `in.SampleRate` (default 1.0) | **0.01** (hardcoded) |
| `/.` | (no entry → fell to default 1.0) | **0.001** |
| `/wp-` | (no entry → fell to default 1.0) | **0.001** |
| `/api/`, `/rest/v1/`, `/peeringdb.v1.`, `/graphql` | literal `1.0` | `in.SampleRate` (default 1.0) |
| `/healthz`, `/readyz`, `/grpc.health.v1.Health/` | 0.01 | 0.01 (unchanged) |
| `/ui/` | 0.5 | 0.5 (unchanged) |
| `/static/`, `/favicon.ico` | 0.01 | 0.01 (unchanged) |

Effect on scanner traffic: 1.0 → 0.01 (default) and 1.0 → 0.001 (explicit
deny-prefixes) — roughly 100×–1000× reduction in trace volume for
unmatched paths. The 9M-spans/hour spike would have dropped to ~9k–90k
spans/hour, well inside the free-tier budget.

`PDBPLUS_OTEL_SAMPLE_RATE` remains a working operator knob, but it now
dampens known-app-route volume only, not the deny-by-default floor.
`sdktrace.ParentBased` composition is unchanged, so sampled-in parents
still force the child's `RecordAndSample` regardless of the inverted
default — cross-service trace continuity invariant preserved.

## Code changes

### `internal/otel/provider.go`

- Extracted the inline `PerRouteSamplerInput{...}` literal from
  `Setup()`'s `sdktrace.WithSampler` call into a new package-private
  helper `defaultSamplerInput(in SetupInput) PerRouteSamplerInput`.
  Lifted so `provider_test.go` can lock the inverted policy without
  round-tripping through the opaque `sdktrace.Sampler` interface.
- Set `DefaultRatio: 0.01` (was `in.SampleRate`).
- Added `"/.": 0.001` and `"/wp-": 0.001` deny-prefix entries.
- Changed `"/api/"`, `"/rest/v1/"`, `"/peeringdb.v1."`, `"/graphql"`
  from literal `1.0` to `in.SampleRate`.
- Replaced the doc comment block above the `WithSampler` call. The new
  block describes deny-by-default + allow-list policy and forward-points
  to `docs/ARCHITECTURE.md § Sampling Matrix` and this SUMMARY. The
  doc comment on `defaultSamplerInput` records that
  `.planning/phases/77-telemetry-audit/AUDIT.md`'s "Recommended sampling
  matrix" section is superseded by this policy.

### `internal/otel/sampler_test.go`

Four new tests, all `t.Parallel()`, all using the existing
`sampleParams(allOnesTraceID, attribute.String("url.path", path))` idiom:

1. `TestPerRouteSampler_DotDeniedAtLowRatio` — table-driven sweep over 8
   dotfile paths (`/.env`, `/.git/config`, `/.git/HEAD`, `/.aws/credentials`,
   `/.docker/config.json`, `/.kube/config`, `/.htpasswd`, `/.npmrc`),
   asserts `sdktrace.Drop` under `Routes={"/." : 0.001}` + `DefaultRatio=1.0`.
2. `TestPerRouteSampler_WpDeniedAtLowRatio` — table-driven sweep over 3
   WordPress paths (`/wp-admin`, `/wp-login.php`,
   `/wp-content/themes/foo/setup-config.php`), asserts `Drop`.
3. `TestPerRouteSampler_UnknownPathDropsAtOnePercent` — `DefaultRatio=0.01`
   case: `/phpinfo.php` drops, `/api/networks` (allow-list match)
   sample-ins.
4. `TestPerRouteSampler_DotDoesNotMatchPlainSlash` — defends against
   `"/."` accidentally matching `"/"` alone. Parameterises `DefaultRatio`
   over `{1.0, 0.0}` so the assertion proves "fell through to default"
   rather than "matched and dropped".

### `internal/otel/provider_test.go`

One new test, `TestSetup_InvertedSamplerDefault`, parameterised over
`SampleRate ∈ {0.0, 0.5, 1.0}`. Asserts on the struct returned by
`defaultSamplerInput`:

- `DefaultRatio == 0.01` exactly.
- `Routes["/."] == 0.001`, `Routes["/wp-"] == 0.001`.
- `Routes["/api/"] == in.SampleRate` and likewise for `/rest/v1/`,
  `/peeringdb.v1.`, `/graphql` — proves the operator knob is wired
  through and not pinned to literal 1.0.
- Health-probe / static / UI ratios survive unchanged with exact map
  values for `/healthz`, `/readyz`, `/grpc.health.v1.Health/`, `/ui/`,
  `/static/`, `/favicon.ico`.

### `docs/ARCHITECTURE.md`

- Reworded the prose paragraph on the `TracerProvider` sampler to
  describe the new policy (known-app-route ratio honours the env var;
  unknown paths drop to 1%).
- Replaced the Sampling Matrix table with the inverted layout: added a
  scanner-bait deny-prefix row, merged `/api/`, `/rest/v1/`,
  `/peeringdb.v1.`, `/graphql` into one env-var-driven row, replaced
  the default row with the deny-by-default explanation.

### `docs/CONFIGURATION.md`

Updated the `PDBPLUS_OTEL_SAMPLE_RATE` row description to scope its
effect to the four known app surfaces and forward-point to
`docs/ARCHITECTURE.md § Sampling Matrix` for the hardcoded ratios.

## Verification

| Command | Status |
|---------|--------|
| `go build ./...` | PASS |
| `go vet ./...` | PASS |
| `go test -race ./internal/otel/... -count=1` | PASS (full suite) |
| `go test -race ./internal/otel/... -run 'PerRouteSampler\|Setup_InvertedSamplerDefault'` | PASS |
| `golangci-lint run ./internal/otel/...` | PASS (0 issues) |
| `go test -race ./... -count=1` | PASS (module-wide, all packages green) |

Manual sanity checks (from PLAN.md `<verification>`):

- `grep -n 'DefaultRatio' internal/otel/provider.go` → 2 hits: line 60 (doc comment, mentions `DefaultRatio=0.01`), line 241 (the only assignment, value `0.01`). The plan asked for "exactly one hit, value 0.01" — there is exactly one assignment, the second hit is a comment reference.
- `grep -n 'in.SampleRate' internal/otel/provider.go` → 5 hits: 4 inside `defaultSamplerInput`'s Routes map plus 1 in the policy doc comment that references the historical `unknown == in.SampleRate` inheritance. None remain in the `Setup` function path.
- `grep -nE '"/\.":|"/wp-":' internal/otel/provider.go` → 2 hits as expected.
- `grep -n 'AUDIT.md' internal/otel/provider.go internal/otel/sampler.go` → `provider.go` reference is inside `defaultSamplerInput`'s doc comment marking it superseded; `sampler.go`'s historical header reference remains (out of scope per plan).
- `grep -rn 'default ratio honours' docs/` → no matches (stale wording removed from `docs/ARCHITECTURE.md`).

## Note on AUDIT.md

The `.planning/phases/77-telemetry-audit/AUDIT.md` "Recommended sampling
matrix" is now historical. The doc comment on `defaultSamplerInput`
records the supersession explicitly. `docs/ARCHITECTURE.md § Sampling
Matrix` and this SUMMARY are the live source of truth.

## Self-Check: PASSED

- `internal/otel/provider.go` — present, contains `DefaultRatio: 0.01` and
  `defaultSamplerInput` helper.
- `internal/otel/sampler_test.go` — present, contains four new tests.
- `internal/otel/provider_test.go` — present, contains
  `TestSetup_InvertedSamplerDefault`.
- `docs/ARCHITECTURE.md` — present, table contains `0.001` row for `/.`
  and `/wp-`.
- `docs/CONFIGURATION.md` — present, updated env var row.
- Commit `03cc619` exists in `git log`.
