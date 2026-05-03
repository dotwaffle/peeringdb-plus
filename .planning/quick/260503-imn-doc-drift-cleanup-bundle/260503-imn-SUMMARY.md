---
phase: 260503-imn-doc-drift-cleanup-bundle
plan: 01
status: complete
type: execute
completed: 2026-05-03
duration_sec: 187
commits:
  - hash: d2e7496
    full_hash: d2e749616cbb3f9bf13bdb175bcc597b12c65383
    subject: "otel: drop redundant OTEL_BSP option overrides"
    files: ["internal/otel/provider.go"]
  - hash: 75823c2
    full_hash: 75823c2c2d505048fc50040afced7cb0035c3d72
    subject: "docs: close DEFER-70-06-01 in CLAUDE.md"
    files: ["CLAUDE.md"]
  - hash: 44e883a
    full_hash: 44e883afb360992e34e7f41e86a2a9e1d315356a
    subject: "seeds: archive SEED-001 to consumed/"
    files:
      - ".planning/seeds/SEED-001-incremental-sync-evaluation.md (R)"
      - ".planning/seeds/consumed/SEED-001-incremental-sync-evaluation.md (R+M)"
requirements:
  - DRIFT-01-otel-bsp-overrides
  - DRIFT-02-claude-md-defer-70-06-01
  - DRIFT-03-seed-001-archive
---

# Quick Task 260503-imn — doc drift cleanup bundle (3 commits)

## Outcome

Three independent v1.18.0 closeout doc-drift fixes that had been
flagged but never landed are now on `main` as three atomic, bisectable
commits. The bundle is pure backlog hygiene: keep on-disk truth in
sync with the shipped state of v1.18.0 (no behaviour change).

## Commits

- `d2e7496` `otel: drop redundant OTEL_BSP option overrides` —
  `internal/otel/provider.go` — autoexport env knobs were dead because
  the explicit SDK `WithBatchTimeout(5*time.Second)` and
  `WithMaxExportBatchSize(512)` shadow `OTEL_BSP_*` env defaults; the
  hardcoded values were the SDK's own defaults so dropping them is
  behaviour-neutral and re-engages the documented env interface.
- `75823c2` `docs: close DEFER-70-06-01 in CLAUDE.md` — `CLAUDE.md` —
  Phase 73 BUG-01 shipped the campus-inflection fix in v1.18.0 via
  the sibling-file mixin `ent/schema/campus_annotations.go`, but the
  CLAUDE.md "Known gap" paragraph still claimed the fix was queued.
- `44e883a` `seeds: archive SEED-001 to consumed/` —
  `.planning/seeds/SEED-001-incremental-sync-evaluation.md` →
  `.planning/seeds/consumed/SEED-001-incremental-sync-evaluation.md`
  (rename via `git mv` plus yaml frontmatter flip to `status:
  consumed`, `consumed_in: v1.17.0`, `consumed_by:
  quick-task-260426-pms`) — quick task 260426-pms consumed SEED-001
  in v1.17.0 but the file still sat at the top level alongside the
  genuinely-active SEED-003/004/005, masking the live seed backlog.

## Gate results (commit 1 only)

Commit 1 is the only Go-touching commit. Gates run after the edit,
before the commit:

| Gate                                                    | Result |
| ------------------------------------------------------- | ------ |
| `go build ./...`                                        | PASS   |
| `go vet ./...`                                          | PASS   |
| `go test -race ./internal/otel/... -count=1`            | PASS (`ok`, 1.148s) |
| `golangci-lint run ./internal/otel/...`                 | PASS (`0 issues.`) |
| `grep -cE 'WithBatchTimeout\|WithMaxExportBatchSize' internal/otel/provider.go` | 0 |
| `grep -c '"time"' internal/otel/provider.go`            | 0 |

Commits 2 and 3 are docs / yaml / file-move only — no Go gates
required. Sanity checks ran clean:

- Commit 2: `git diff --stat` shows 1 file / 1 line changed; `grep -c
  'Known gap (DEFER-70-06-01)' CLAUDE.md` = 0; `grep -c 'Closed in
  Phase 73' CLAUDE.md` = 1; `grep -c 'campus_annotations.go'
  CLAUDE.md` = 1.
- Commit 3: `ls .planning/seeds/` shows SEED-003/004/005 + `consumed/`
  (no SEED-001 at top); `ls .planning/seeds/consumed/` shows SEED-001
  + SEED-002; yaml header has `status: consumed`, `consumed_in:
  v1.17.0`, `consumed_by: quick-task-260426-pms`; `git log --follow`
  on the moved path returns 4 commits (rename history preserved).

## Bisectability

Each of the three fixes is independently revertable with `git revert`
because they touch disjoint files:

- `d2e7496` touches only `internal/otel/provider.go`.
- `75823c2` touches only `CLAUDE.md`.
- `44e883a` touches only the SEED-001 file (rename + yaml).

`d2e7496` is the only commit that touches `.go` files, so it is the
only commit that needs to compile and pass tests on its own; the
gates above were run after that commit and PASSed. Checking out
`75823c2` or `44e883a` in isolation leaves the build state identical
to `d2e7496`'s — neither commit touches a `.go` file or any codegen
input.

## Files touched

- `internal/otel/provider.go` (commit 1) — drop `WithBatchTimeout`,
  `WithMaxExportBatchSize`, `"time"` import; rewrite the comment
  block to describe the SDK-default + env-tuneable behaviour.
- `CLAUDE.md` (commit 2) — single-paragraph surgical Edit at line 167;
  "Known gap" → "Closed in Phase 73 (v1.18.0)".
- `.planning/seeds/SEED-001-incremental-sync-evaluation.md` →
  `.planning/seeds/consumed/SEED-001-incremental-sync-evaluation.md`
  (commit 3) — `git mv` rename + yaml header flip to `status:
  consumed` with `consumed_in: v1.17.0` and `consumed_by:
  quick-task-260426-pms`.

## Out of scope / not touched

Explicitly preserved verbatim:

- Metric Views block in `internal/otel/provider.go` (HTTP body-size
  drops, `rpc.server.*` family drops, otelhttp duration histogram
  view) — out of scope.
- All other `CLAUDE.md` sections — out of scope. CLAUDE.md is
  governed by `/claude-md-management:revise-claude-md`; this was a
  surgical Edit and stays surgical.
- `SEED-003-primary-ha-hot-standby.md`,
  `SEED-004-tombstone-gc.md`,
  `SEED-005-periodic-full-sync-schedule.md` — yaml unchanged; still
  the genuinely-active backlog at the top level of `.planning/seeds/`.
- The body content of SEED-001 (everything below the closing yaml
  `---`) — preserved verbatim through the rename + yaml flip.

## Deviations from plan

One execution-level glitch worth recording for the orchestrator
spot-check, no semantic deviation:

- Commit 3 was first landed with only the `git mv` rename (the yaml
  flip was un-staged because the Bash one-liner that staged both
  paths failed on the now-removed source path). The fix was a
  `git commit --amend --no-edit` after staging the yaml diff —
  yielding the final hash `44e883a`. The amend predates any push and
  predates the SUMMARY.md commit, so no published commit was
  rewritten. End state matches the plan's "rename + yaml in one
  logical patch" requirement.

No Rule 1/2/3 auto-fixes were needed; no Rule 4 architectural
decisions were raised.

## Self-Check: PASSED

- `internal/otel/provider.go` — exists, no `WithBatchTimeout` /
  `WithMaxExportBatchSize` / `"time"` references.
- `CLAUDE.md` — exists, contains "Closed in Phase 73 (v1.18.0):
  DEFER-70-06-01" and `campus_annotations.go`; no "Known gap
  (DEFER-70-06-01)".
- `.planning/seeds/consumed/SEED-001-incremental-sync-evaluation.md`
  — exists with `status: consumed`, `consumed_in: v1.17.0`,
  `consumed_by: quick-task-260426-pms`.
- `.planning/seeds/SEED-001-incremental-sync-evaluation.md` — does
  not exist.
- `git log` — `d2e7496`, `75823c2`, `44e883a` all present on
  `worktree-agent-aa20a08eda907ccc3` HEAD chain.
