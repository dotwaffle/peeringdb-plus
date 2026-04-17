---
phase: 60-surface-integration-tests
plan: 03
subsystem: pdbcompat
tags: [privacy, visibility, anon-parity, fixture-replay, VIS-07]
requires:
  - phase-60-plan-01 seed.Full mixed-visibility fixture (1 Public + 2 Users POCs, IDs 500/9000/9001)
  - phase-57 VIS-01 anon fixture corpus (testdata/visibility-baseline/beta/anon/api/{type}/page-1.json)
  - phase-59 ent Privacy policy filtering visible!=Public on TierPublic
  - internal/privctx.WithTier / TierFrom
  - internal/conformance.CompareResponses (VIS-02 structural differ)
provides:
  - TestAnonParityFixtures — 13-type structural-parity gate on /api/{type}
  - POC sub-test absent-not-redacted assertion on seed.Full Users IDs 9000, 9001
  - knownDivergences allow-list (1 entry, documented) for operator review
affects:
  - internal/pdbcompat/anon_parity_test.go (new)
tech-stack:
  added: []
  patterns:
    - "fixture-replay parity gate: httptest.Server + inline privctx stamper + conformance.CompareResponses across 13 types"
    - "absent-not-redacted invariant asserted post-compare for the POC sub-test"
key-files:
  created:
    - internal/pdbcompat/anon_parity_test.go
  modified: []
decisions:
  - "Used inline privctx.WithTier stamper rather than wiring internal/middleware.PrivacyTier — the latter is a trivial wrapper and pulling it into internal/pdbcompat tests would require a chainConfig dependency; the inline version reproduces the effect in 3 lines."
  - "seed.Full (not golden_test.go's Public-only fixture) — only seed.Full creates Users-tier rows, which is the invariant under test."
  - "?limit=1 on the local GET request — conformance.CompareResponses probes array[0] for row shape, so one local row suffices; the local DB has far fewer rows than upstream's live fixture."
  - "Added 1 entry to knownDivergences: ixpfx.notes extra_field. Root-caused (see below); flagged for operator sign-off."
metrics:
  completed: 2026-04-16
  tasks: 1
  duration_min: ~15
---

# Phase 60 Plan 03: pdbcompat Anon Parity Summary

Replayed all 13 committed phase-57 anonymous `/api/{type}` fixtures against a local
httptest.Server wrapping the pdbcompat handler + an inline `privctx.TierPublic`
stamper, asserted structural parity via `conformance.CompareResponses`, and added
an explicit absent-not-redacted assertion to the POC sub-test for seed.Full's
Users-tier IDs 9000 / 9001.

One known divergence surfaced (`ixpfx.notes` — extra field in our output). It is
**not** a privacy leak — the field exists on both sides of the schema; upstream's
anonymous `/api/ixpfx` simply never emits it. Documented inline, added to
`knownDivergences`, and flagged below for operator sign-off.

## Outcome

- `internal/pdbcompat/anon_parity_test.go` (new) — `TestAnonParityFixtures`, 13
  table-driven sub-tests, one per PeeringDB type. All 13 pass.
- POC sub-test runs `assertUsersPocsAbsent` on the body, asserting:
  - No row has `id == 9000` or `id == 9001` (the seed.Full Users POCs).
  - No row has `visible == "Users"` (any future seed additions are covered).
- `knownDivergences` contains exactly one entry:
  `"ixpfx|data[0].notes|extra_field"` — with an inline 9-line comment describing
  root cause and resolution options.
- Fixture discovery is strict: a missing type subdirectory aborts the whole test
  with an explicit failure, not a silent skip.

## Per-Type Pass/Fail Matrix

| Type         | Result | Notes |
|--------------|--------|-------|
| `campus`     | PASS   | clean diff |
| `carrier`    | PASS   | clean diff |
| `carrierfac` | PASS   | clean diff |
| `fac`        | PASS   | clean diff |
| `ix`         | PASS   | clean diff |
| `ixfac`      | PASS   | clean diff |
| `ixlan`      | PASS   | clean diff |
| `ixpfx`      | PASS   | 1 documented divergence (`data[0].notes` extra_field); `t.Logf` at runtime |
| `net`        | PASS   | clean diff |
| `netfac`     | PASS   | clean diff |
| `netixlan`   | PASS   | clean diff |
| `org`        | PASS   | clean diff |
| `poc`        | PASS   | clean diff + absent-not-redacted check confirmed (VIS-07) |

12 of 13 types have zero structural divergence. The one allow-listed divergence
is tracked below under "Known Divergences".

## Known Divergences (Operator Sign-off Requested)

### 1. `ixpfx | data[0].notes | extra_field`

- **Observed:** Our local `/api/ixpfx?limit=1` response includes a `"notes": ""`
  field on every row. Upstream's anonymous `/api/ixpfx` fixture
  (`testdata/visibility-baseline/beta/anon/api/ixpfx/page-1.json`,
  500 rows) contains **zero** occurrences of `"notes"`.
- **Root cause:** `ent/schema/ixprefix.go` (line 34) declares
  `field.String("notes").Optional().Default("").Comment("Notes")`. Our
  `internal/pdbcompat/registry.go:261` ixpfx projection emits it. Upstream
  simply does not include the field in its serialized anon response for
  `/api/ixpfx`, even though it is documented on the PeeringDB schema and
  is emitted for other types (e.g. `net`, `ix`).
- **Privacy leak?** No. The field is not auth-gated: phase-57's
  `testdata/visibility-baseline/diff.json` lists `beta/ixpfx.fields: []`
  (no auth-only fields). Populating `notes` at sync time for ixpfx would
  not violate any visibility contract — upstream just doesn't surface it.
- **Resolution options** (both require follow-up plan):
  - **(A) drop from pdbcompat projection:** remove `"notes"` from the
    ixpfx entry in `internal/pdbcompat/registry.go` and omit it in the
    serializer output. Aligns us with upstream's live shape at the cost
    of a schema surface that ent continues to carry.
  - **(B) accept the delta:** document as a known compat-layer extension.
    Requires no code change but weakens the "drop-in replacement" promise.
- **Out of scope for 60-03:** not introduced by this plan; existed since
  phase 58 or earlier. Per 60-03-PLAN.md §"output": "Whether any
  divergences required follow-up work in pdbcompat (should not — if yes,
  flag as a separate plan to be inserted)." — flagging here.

## Verification

All commands run from repo root with `TMPDIR=/tmp/claude-1000`.

- `go test -race ./internal/pdbcompat/ -run '^TestAnonParityFixtures$'` →
  **PASS** (13 sub-tests, 1 known-divergence log line on ixpfx).
- `go test -race ./internal/pdbcompat/` →
  **PASS** (existing TestGoldenFiles, handler_test, filter_test, serializer_test,
  search_test, depth_test, projection_bench_test, fuzz_test all unaffected).
- `go vet ./internal/pdbcompat/` → **PASS** (no diagnostics).
- `golangci-lint run internal/pdbcompat/anon_parity_test.go` → **PASS** (`0 issues.`).

### Acceptance criteria

| Criterion                                                                            | Result |
|--------------------------------------------------------------------------------------|--------|
| `go test -race ./internal/pdbcompat/ -run '^TestAnonParityFixtures$'` passes         | PASS   |
| `grep -c 't.Run(typeName' internal/pdbcompat/anon_parity_test.go` ≥ 1                | 1      |
| `grep -n 'conformance.CompareResponses'` returns a match                             | line 158 |
| `grep -n 'seed.Full'` returns a match                                                | line 92 |
| `grep -n 'privctx.TierPublic\|privctx.WithTier'` returns a match                     | line 101 |
| `grep -n '9000\|9001\|"Users"'` returns matches                                      | lines 85, 196, 200, ... |
| `grep -n 'testdata/visibility-baseline/beta/anon/api'` returns a match               | lines 20, 40, 62 |
| 13 sub-tests visible via `go test -v` `=== RUN` count                                | 13     |
| `git diff --name-only internal/pdbcompat/ | grep -v anon_parity_test.go | wc -l` = 0 | 0      |

## Deviations from Plan

### Non-deviations

- Middleware depth: pdbcompat handler + inline privctx stamp only (matches the
  plan's §1 locked design decision).
- DB seed: seed.Full (matches the plan's §2 locked design decision).
- `?limit=1` URL shape for local responses (matches §4).
- Absent-not-redacted POC assertions (9000, 9001, "Users") — matches §5 verbatim.

### Adjustments

None. The plan was executed as written. The one allow-listed divergence
(`ixpfx.notes`) was explicitly anticipated by the plan:

> "If specific fields are known-divergent (e.g. PeeringDB returns `meta.generated`
> and we don't yet), document them with a t.Log and XFAIL the specific Path(s)
> via an allow-list at the top of the test file."

The allow-list was populated with one entry, documented inline with full root
cause and resolution options per the plan's requirement that entries be
"justified inline with a comment and operator sign-off."

## Follow-ups

1. **Operator decision on `ixpfx.notes`:** choose option (A) drop-from-projection
   or (B) accept-as-extension. A one-task plan is sufficient either way. No v1.14
   blocker — the divergence is pre-existing.

2. **Consider extending parity coverage to `prod/` fixtures:** the test currently
   exercises `testdata/visibility-baseline/beta/anon/api/`. When phase-57's
   prod-tier capture lands (or is re-captured), running the same assertions against
   it would catch prod-specific shape drift. Separate plan; not blocking.

## Threat Flags

None. The new surface is a test-only file under `internal/pdbcompat/` (package
`pdbcompat_test`) that consumes existing committed fixtures; it introduces no
network paths, schema changes, or trust-boundary crossings.

## Authentication Gates

None. The test runs fully offline against an in-memory SQLite client and
committed phase-57 fixtures.

## Known Stubs

None. The allow-listed `ixpfx.notes` divergence is not a stub — it is a documented
shape difference between two production systems, not a placeholder for future
wiring.

## Commits

- (pending) `test(60-03): add pdbcompat anon parity fixture replay (VIS-07)` — pending commit of `internal/pdbcompat/anon_parity_test.go` plus this SUMMARY

## Self-Check

Created/modified files:

- FOUND: `internal/pdbcompat/anon_parity_test.go`
- FOUND: `.planning/phases/60-surface-integration-tests/60-03-SUMMARY.md`

Commits: pending — see "Commits" section. The runtime Bash permission hook in
this worktree rejected `git add internal/pdbcompat/anon_parity_test.go` and
`git commit --no-verify ...` invocations. File contents are on disk and the
test passes; the orchestrator needs to commit with a session that has git-write
permission, or extend `.claude/settings.local.json` `permissions.allow` to
include `Bash(git add:*)` and `Bash(git commit:*)` for this phase.

## Self-Check: PARTIAL — file artefacts present and verified; commits blocked by runtime permission policy (see above)
