---
phase: 260503-imn-doc-drift-cleanup-bundle
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/otel/provider.go
  - CLAUDE.md
  - .planning/seeds/SEED-001-incremental-sync-evaluation.md
  - .planning/seeds/consumed/SEED-001-incremental-sync-evaluation.md
autonomous: true
requirements:
  - DRIFT-01-otel-bsp-overrides
  - DRIFT-02-claude-md-defer-70-06-01
  - DRIFT-03-seed-001-archive

must_haves:
  truths:
    - "internal/otel/provider.go has no explicit WithBatchTimeout/WithMaxExportBatchSize on the trace BatchSpanProcessor; OTEL_BSP_* env vars now control the values."
    - "CLAUDE.md no longer claims DEFER-70-06-01 is open; the paragraph cites Phase 73 (v1.18.0) closure and points at ent/schema/campus_annotations.go."
    - ".planning/seeds/ no longer contains SEED-001 at the top level; .planning/seeds/consumed/ contains both SEED-001 and SEED-002."
    - "SEED-001 yaml header has status: consumed plus consumed_in: v1.17.0 and consumed_by: quick-task-260426-pms."
    - "Three atomic commits land in order; each subject is kernel-style; each commit body is plain text wrapped at ~74 cols with no links."
    - "go build, go vet, go test -race ./internal/otel/..., and golangci-lint run ./internal/otel/... all pass after commit 1."
    - "git log --follow on the moved SEED-001 file shows pre-rename history (git mv preserved it)."
  artifacts:
    - path: "internal/otel/provider.go"
      provides: "Trace BatchSpanProcessor without hardcoded timeout/batch options."
      absent_lines: ["WithBatchTimeout", "WithMaxExportBatchSize"]
    - path: "CLAUDE.md"
      provides: "Updated DEFER-70-06-01 paragraph marked closed."
      contains: "Phase 73"
      absent_lines: ["Fix queued"]
    - path: ".planning/seeds/consumed/SEED-001-incremental-sync-evaluation.md"
      provides: "Archived SEED-001 with consumed yaml fields."
      contains: "status: consumed"
  key_links:
    - from: "internal/otel/provider.go comment block (lines 53-54)"
      to: "OTEL_BSP_SCHEDULE_DELAY / OTEL_BSP_MAX_EXPORT_BATCH_SIZE env vars"
      via: "autoexport SDK env defaults"
      pattern: "tuneable via OTEL_BSP_"
    - from: "CLAUDE.md DEFER-70-06-01 paragraph"
      to: "ent/schema/campus_annotations.go and MILESTONES.md Phase 73 line"
      via: "audit-trail cross-references"
      pattern: "campus_annotations.go"
    - from: ".planning/seeds/consumed/SEED-001-…"
      to: "quick-task-260426-pms"
      via: "consumed_by yaml field"
      pattern: "consumed_by: quick-task-260426-pms"
---

<objective>
Three independent doc-drift fixes flagged during v1.18.0 closeout that
never landed. Bundled into one quick task for orchestration; produced as
THREE atomic, bisectable commits so each can be reverted independently.

Purpose: backlog hygiene — keep on-disk truth in sync with shipped state.
Output: three commits on main, one SUMMARY.md.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@CLAUDE.md
@.planning/STATE.md
@internal/otel/provider.go
@internal/otel/provider_test.go
@.planning/seeds/SEED-001-incremental-sync-evaluation.md
@.planning/seeds/consumed/SEED-002-fly-asymmetric-fleet.md
@.planning/MILESTONES.md
@ent/schema/campus_annotations.go

<facts_already_verified>
The planner has already inspected the codebase and pre-verified the
following — executor MUST NOT re-litigate these:

1. `time` package usage in `internal/otel/provider.go` is **only** at
   line 69 (`5*time.Second`). Dropping that line means the `time`
   import on line 11 also becomes unused and MUST be removed in the
   same patch — otherwise `go vet` fails.

2. `internal/otel/provider_test.go` does NOT reference `BatchTimeout`
   or `MaxExportBatchSize`. No test assertions need updating. Verified
   by `grep -rn "BatchTimeout\|MaxExportBatchSize" internal/otel/`.

3. `ent/schema/campus_annotations.go` already exists (Phase 73 BUG-01
   shipped 2026-04-22). Its package comment confirms the
   sibling-mixin pattern and the cross-reference text to use in
   CLAUDE.md.

4. MILESTONES.md line 9 already records DEFER-70-06-01 as closed via
   `ent/schema/campus_annotations.go`. The CLAUDE.md paragraph is the
   only stale claim left.

5. `.planning/seeds/` directory contents (verified 2026-05-03):
     SEED-001-incremental-sync-evaluation.md   (top level — to move)
     SEED-003-primary-ha-hot-standby.md
     SEED-004-tombstone-gc.md
     SEED-005-periodic-full-sync-schedule.md
     consumed/SEED-002-fly-asymmetric-fleet.md
   Post-fix-3 expected state: SEED-001 also under consumed/.

6. SEED-002's consumed-state yaml schema (the precedent to mirror):
     status: consumed
     activated: <date>
     consumed: <date>
     consumed_by: <ref>
     priority: <was-original-priority>
   For SEED-001, the task description specifies the exact field set:
     status: consumed
     consumed_in: v1.17.0
     consumed_by: quick-task-260426-pms
   plus retain the existing `resolved_unknown: 2026-04-26` field.
   (Task description specifies `consumed_in` + `consumed_by`; SEED-002
    used `consumed:` as a date and `consumed_by:` as a ref. We follow
    the explicit task instruction here — it is consistent with SEED-002
    in spirit even though the field name set differs slightly.)
</facts_already_verified>

<interfaces>
Current state of `internal/otel/provider.go` lines 52-74 (target site
for fix 1):

```go
	// TracerProvider with configurable sampling per D-02.
	// Batching is explicitly enabled per PERF-08; defaults to 5s/512 items,
	// tuneable via OTEL_BSP_SCHEDULE_DELAY and OTEL_BSP_MAX_EXPORT_BATCH_SIZE.
	spanExporter, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating span exporter: %w", err)
	}
	// Per-route sampler with deny-by-default …
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(spanExporter,
			sdktrace.WithBatchTimeout(5*time.Second),
			sdktrace.WithMaxExportBatchSize(512),
		),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(NewPerRouteSampler(defaultSamplerInput(in)))),
	)
```

Target post-edit:

```go
	// TracerProvider with configurable sampling per D-02.
	// Batching is enabled with SDK defaults (5s schedule delay, 512 max
	// batch size); both tuneable via OTEL_BSP_SCHEDULE_DELAY and
	// OTEL_BSP_MAX_EXPORT_BATCH_SIZE per the autoexport env interface.
	spanExporter, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating span exporter: %w", err)
	}
	// Per-route sampler with deny-by-default …
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(spanExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(NewPerRouteSampler(defaultSamplerInput(in)))),
	)
```

Plus: remove `"time"` from the import block on lines 6-25 (sole user
was the dropped option). The full sampler comment block above the
TracerProvider (lines 59-66) stays untouched.

Current state of `CLAUDE.md` line 167 (target site for fix 2):

```
**Known gap (DEFER-70-06-01).** `<entity>?campus__<field>=X` returns 500 (table-name inflection — see `.planning/milestones/v1.16-phases/70-cross-entity-traversal/deferred-items.md`). Fix queued: `entsql.Annotation{Table: "campuses"}` on `ent/schema/campus.go`.
```

Target post-edit (single line, same paragraph slot):

```
**Closed in Phase 73 (v1.18.0): DEFER-70-06-01.** `<entity>?campus__<field>=X` previously returned 500 due to go-openapi/inflect mis-singularising "campus" → "campu" on the `cmd/pdb-compat-allowlist` codegen path. Fixed by sibling-file mixin `ent/schema/campus_annotations.go` (`entsql.Annotation{Table: "campuses"}`) — see `MILESTONES.md` Phase 73 BUG-01 for audit trail. The `entc.LoadGraph` runtime patch in `ent/entc.go` (`fixCampusInflection`) remains; the two are complementary, not redundant.
```

Surgical Edit only. Adjacent paragraphs ("Phase 68 + 69 composition"
above, "### Response memory envelope" header below) MUST NOT change.

Current state of SEED-001 yaml header (target site for fix 3):

```yaml
---
id: SEED-001
slug: incremental-sync-evaluation
planted: 2026-04-17
planted_after: v1.14
surface_at: v1.15+
status: ready
priority: active
resolved_unknown: 2026-04-26
triggers:
  - …
---
```

Target post-edit:

```yaml
---
id: SEED-001
slug: incremental-sync-evaluation
planted: 2026-04-17
planted_after: v1.14
surface_at: v1.15+
status: consumed
consumed_in: v1.17.0
consumed_by: quick-task-260426-pms
priority: active
resolved_unknown: 2026-04-26
triggers:
  - …
---
```

Body content (everything below the closing `---`) is preserved
verbatim. The file moves from `.planning/seeds/` to
`.planning/seeds/consumed/` via `git mv`.
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: otel — drop redundant OTEL_BSP option overrides (commit 1 of 3)</name>
  <files>internal/otel/provider.go</files>
  <action>
Edit `internal/otel/provider.go`:

1. In the `import` block (lines 6-25), remove the `"time"` line. After
   the edit, the imports must still group cleanly (stdlib block on
   top, third-party below). Run `gofmt -s -w internal/otel/provider.go`
   if needed to normalise.

2. Replace lines 52-54 (the comment claiming the values are tuneable
   while in fact hardcoding them) with the corrected three-line
   comment from the `<interfaces>` block above. The corrected comment
   must explicitly call out that batching is enabled with SDK defaults
   and that the two env vars are the tuning interface (no behaviour
   claim that we "explicitly tune" anything).

3. In the `sdktrace.NewTracerProvider` call (currently lines 67-74),
   collapse the multi-line `sdktrace.WithBatcher(spanExporter, …)` to
   a single-arg form: `sdktrace.WithBatcher(spanExporter)`. The two
   inner options (`WithBatchTimeout(5*time.Second)`,
   `WithMaxExportBatchSize(512)`) are removed. The `WithResource` and
   `WithSampler` calls below remain unchanged.

4. The full sampler-rationale comment block (currently lines 59-66,
   the "Per-route sampler with deny-by-default …" paragraph) is OUT
   OF SCOPE — DO NOT touch it.

5. The metric Views block (lines 90-189-ish — HTTP body-size drops,
   rpc.server.* drops, otelhttp duration histogram view) is OUT OF
   SCOPE — DO NOT touch it.

After edit, run the gates listed in `<verify>`. If `go vet` complains
about the unused `time` import, you missed step 1; go fix it and
re-run. If a test in `provider_test.go` fails, STOP — the planner
verified at planning time that no test asserts on these options;
investigate the actual failure before changing tests.

Then commit. Subject and body (kernel-style, plain text, no links,
~74 col body wrap):

```
otel: drop redundant OTEL_BSP option overrides

Lines 67-69 of internal/otel/provider.go set
WithBatchTimeout(5*time.Second) and WithMaxExportBatchSize(512) on the
trace BatchSpanProcessor. The neighbouring comment claimed the values
were tuneable via OTEL_BSP_SCHEDULE_DELAY and
OTEL_BSP_MAX_EXPORT_BATCH_SIZE, but explicit SDK options take
precedence over env defaults inside autoexport — so the env knobs were
silently dead.

The two values we hardcoded (5s, 512) are the SDK's own defaults.
Dropping the explicit options preserves identical behaviour at default
config and re-engages the documented env interface for operators that
need to tune batch flushing under different export back-pressure.

Comment block updated to describe the new (and now truthful)
mechanism. The unused "time" import drops out as a consequence.

v1.18.0 closeout backlog hygiene; bisectable.
```

Commit only `internal/otel/provider.go`.
  </action>
  <verify>
    <automated>cd /home/dotwaffle/Code/pdb/peeringdb-plus &amp;&amp; go build ./... &amp;&amp; go vet ./... &amp;&amp; go test -race ./internal/otel/... -count=1 &amp;&amp; golangci-lint run ./internal/otel/... &amp;&amp; test "$(grep -cE 'WithBatchTimeout|WithMaxExportBatchSize' internal/otel/provider.go)" -eq 0 &amp;&amp; test "$(grep -c '\"time\"' internal/otel/provider.go)" -eq 0 &amp;&amp; git log -1 --pretty=%s | grep -qE '^otel: '</automated>
  </verify>
  <done>
- `internal/otel/provider.go` no longer references `WithBatchTimeout`,
  `WithMaxExportBatchSize`, or the `"time"` import.
- All four gates pass: build, vet, test -race ./internal/otel/...,
  golangci-lint ./internal/otel/...
- One new commit on HEAD with subject `otel: drop redundant
  OTEL_BSP option overrides` and body explaining WHY (autoexport env
  knobs were dead) and WHAT was wrong (hardcoded SDK defaults).
- Working tree otherwise clean (no other files modified by this
  commit).
  </done>
</task>

<task type="auto">
  <name>Task 2: docs — close DEFER-70-06-01 in CLAUDE.md (commit 2 of 3)</name>
  <files>CLAUDE.md</files>
  <action>
Surgical Edit on `CLAUDE.md` line 167 only.

Find the exact line:

```
**Known gap (DEFER-70-06-01).** `<entity>?campus__<field>=X` returns 500 (table-name inflection — see `.planning/milestones/v1.16-phases/70-cross-entity-traversal/deferred-items.md`). Fix queued: `entsql.Annotation{Table: "campuses"}` on `ent/schema/campus.go`.
```

Replace with the single-line corrected paragraph from the
`<interfaces>` block above (the "Closed in Phase 73 (v1.18.0): …"
version that points at `ent/schema/campus_annotations.go` and
`MILESTONES.md` Phase 73 BUG-01 and notes the complementary
`fixCampusInflection` runtime patch).

Constraints:

- Use `Edit`, not `Write`. Only one paragraph changes. The blank line
  before it (`Phase 68 + 69 composition` paragraph above) and the
  section header below (`### Response memory envelope`) MUST be
  preserved exactly.
- No regeneration via `/gsd-docs-update` or any doc workflow — CLAUDE.md
  is governed by `/claude-md-management:revise-claude-md`. This is a
  surgical Edit and stays surgical.
- Do not add backticks around `Phase 73`, `v1.18.0`, or
  `DEFER-70-06-01` beyond what the target text shows. Do not
  introduce links (CLAUDE.md commit-style rule and consistency with
  the rest of the file).

After edit:

1. `git diff --stat CLAUDE.md` — must show one file, ~1 line changed
   (Edit replaces the line).
2. `grep -c "Known gap (DEFER-70-06-01)" CLAUDE.md` — must be 0.
3. `grep -c "Closed in Phase 73" CLAUDE.md` — must be 1.
4. `grep -c "campus_annotations.go" CLAUDE.md` — must be ≥1.

Commit only `CLAUDE.md`. Subject and body (kernel-style, plain text,
no links, ~74 col body wrap):

```
docs: close DEFER-70-06-01 in CLAUDE.md

The "Known gap" paragraph in CLAUDE.md still claimed
<entity>?campus__<field>=X returned 500 with the fix queued. Phase 73
shipped that fix in v1.18.0 via the sibling-file mixin pattern at
ent/schema/campus_annotations.go, not as a hand-edit of the generated
ent/schema/campus.go (which would have been silently stripped by
cmd/pdb-schema-generate on the next codegen run).

Replaced with a "Closed in Phase 73 (v1.18.0)" entry that points at
the sibling annotation file and MILESTONES.md Phase 73 BUG-01 for
audit trail, and clarifies that the entc.LoadGraph runtime patch in
ent/entc.go (fixCampusInflection) remains as the complementary fix
for ent's own codegen path.

Surgical Edit; no other CLAUDE.md sections touched. v1.18.0 closeout
backlog hygiene; bisectable.
```

Commit only `CLAUDE.md`.
  </action>
  <verify>
    <automated>cd /home/dotwaffle/Code/pdb/peeringdb-plus &amp;&amp; test "$(grep -c 'Known gap (DEFER-70-06-01)' CLAUDE.md)" -eq 0 &amp;&amp; test "$(grep -c 'Closed in Phase 73' CLAUDE.md)" -eq 1 &amp;&amp; test "$(grep -c 'campus_annotations.go' CLAUDE.md)" -ge 1 &amp;&amp; test "$(git diff HEAD~1 HEAD --name-only | wc -l)" -eq 1 &amp;&amp; git log -1 --pretty=%s | grep -qE '^docs: close DEFER-70-06-01'</automated>
  </verify>
  <done>
- `CLAUDE.md` line 167 paragraph is updated; no other CLAUDE.md
  content has changed.
- New paragraph references `Phase 73`, `v1.18.0`,
  `ent/schema/campus_annotations.go`, and `MILESTONES.md`.
- One new commit on HEAD with subject `docs: close DEFER-70-06-01 in
  CLAUDE.md` and body explaining the wrong-claim situation.
- `git diff HEAD~1 HEAD --name-only` lists exactly one file.
  </done>
</task>

<task type="auto">
  <name>Task 3: seeds — archive SEED-001 to consumed/ (commit 3 of 3)</name>
  <files>.planning/seeds/SEED-001-incremental-sync-evaluation.md, .planning/seeds/consumed/SEED-001-incremental-sync-evaluation.md</files>
  <action>
Two-step operation: rename via `git mv`, then in-place yaml update on
the moved file.

Step 1 — rename:

```
git mv .planning/seeds/SEED-001-incremental-sync-evaluation.md \
       .planning/seeds/consumed/SEED-001-incremental-sync-evaluation.md
```

Use `git mv` (not `mv` + `git add`) so history follows the file. After
this, `git status` should show one rename, no other changes.

Step 2 — yaml flip on the moved file:

In `.planning/seeds/consumed/SEED-001-incremental-sync-evaluation.md`,
edit the yaml frontmatter only. Current header:

```yaml
---
id: SEED-001
slug: incremental-sync-evaluation
planted: 2026-04-17
planted_after: v1.14
surface_at: v1.15+
status: ready
priority: active
resolved_unknown: 2026-04-26
triggers:
  …
---
```

Target header:

```yaml
---
id: SEED-001
slug: incremental-sync-evaluation
planted: 2026-04-17
planted_after: v1.14
surface_at: v1.15+
status: consumed
consumed_in: v1.17.0
consumed_by: quick-task-260426-pms
priority: active
resolved_unknown: 2026-04-26
triggers:
  …
---
```

Three line-level changes:

1. `status: ready` → `status: consumed`
2. After the `status:` line, insert two new lines:
   `consumed_in: v1.17.0`
   `consumed_by: quick-task-260426-pms`
3. Body content (the `# SEED-001:` heading and everything below the
   closing `---`) is preserved verbatim — DO NOT reflow, retitle, or
   touch the body.

Note on schema: the task description specifies the field names
`consumed_in` and `consumed_by`. SEED-002 used a different field name
set (`activated:` + `consumed:` as dates plus `consumed_by:` as a
ref). We follow the explicit task instruction here. If a future seeds
schema audit standardises on one set, that is a separate quick task —
out of scope for v1.18.0 closeout.

After edits:

1. `ls .planning/seeds/` — must show only:
     SEED-003-primary-ha-hot-standby.md
     SEED-004-tombstone-gc.md
     SEED-005-periodic-full-sync-schedule.md
     consumed/
   (no SEED-001 at top level)
2. `ls .planning/seeds/consumed/` — must show:
     SEED-001-incremental-sync-evaluation.md
     SEED-002-fly-asymmetric-fleet.md
3. `head -15 .planning/seeds/consumed/SEED-001-incremental-sync-evaluation.md`
   shows the updated yaml with `status: consumed`, `consumed_in:
   v1.17.0`, `consumed_by: quick-task-260426-pms`.
4. `git log --follow .planning/seeds/consumed/SEED-001-incremental-sync-evaluation.md`
   shows pre-rename commits (history preserved).

Commit subject and body (kernel-style, plain text, no links, ~74 col
body wrap):

```
seeds: archive SEED-001 to consumed/

SEED-001 (Switch PDBPLUS_SYNC_MODE default to incremental) was
consumed by quick task 260426-pms in v1.17.0 — see body's "Status
(2026-04-26)" section. The file's yaml header still claimed
status: ready and the file still sat at .planning/seeds/ alongside
the genuinely-active SEED-003/004/005, masking the actual seed
backlog.

Renamed to .planning/seeds/consumed/ alongside SEED-002 (the existing
consumed-state precedent) via git mv so blame follows the file.
Frontmatter flipped to status: consumed with consumed_in: v1.17.0
and consumed_by: quick-task-260426-pms; body content preserved.

v1.18.0 closeout backlog hygiene; bisectable. No code changes.
```

Commit covers both the rename and the yaml diff in one logical
patch.
  </action>
  <verify>
    <automated>cd /home/dotwaffle/Code/pdb/peeringdb-plus &amp;&amp; test ! -e .planning/seeds/SEED-001-incremental-sync-evaluation.md &amp;&amp; test -e .planning/seeds/consumed/SEED-001-incremental-sync-evaluation.md &amp;&amp; test -e .planning/seeds/consumed/SEED-002-fly-asymmetric-fleet.md &amp;&amp; test "$(ls .planning/seeds/ | grep -cE '^SEED-00[1-9]')" -eq 3 &amp;&amp; test "$(grep -c 'status: consumed' .planning/seeds/consumed/SEED-001-incremental-sync-evaluation.md)" -eq 1 &amp;&amp; test "$(grep -c 'consumed_in: v1.17.0' .planning/seeds/consumed/SEED-001-incremental-sync-evaluation.md)" -eq 1 &amp;&amp; test "$(grep -c 'consumed_by: quick-task-260426-pms' .planning/seeds/consumed/SEED-001-incremental-sync-evaluation.md)" -eq 1 &amp;&amp; test "$(git log --follow --oneline .planning/seeds/consumed/SEED-001-incremental-sync-evaluation.md | wc -l)" -ge 2 &amp;&amp; git log -1 --pretty=%s | grep -qE '^seeds: archive SEED-001'</automated>
  </verify>
  <done>
- `.planning/seeds/SEED-001-…` no longer exists; the file lives at
  `.planning/seeds/consumed/SEED-001-…` with full pre-rename history.
- `.planning/seeds/` contains only SEED-003, SEED-004, SEED-005, and
  the `consumed/` subdir.
- `.planning/seeds/consumed/` contains SEED-001 and SEED-002.
- SEED-001's yaml header has `status: consumed`, `consumed_in:
  v1.17.0`, `consumed_by: quick-task-260426-pms`; body unchanged.
- One new commit on HEAD with subject `seeds: archive SEED-001 to
  consumed/`. HEAD~3 is the pre-bundle starting point.
  </done>
</task>

<task type="auto">
  <name>Task 4: write SUMMARY.md covering all three commits</name>
  <files>.planning/quick/260503-imn-doc-drift-cleanup-bundle/260503-imn-SUMMARY.md</files>
  <action>
Create `.planning/quick/260503-imn-doc-drift-cleanup-bundle/260503-imn-SUMMARY.md`
covering all three commits in one document. Sections required:

1. **Title** — `# Quick Task 260503-imn — doc drift cleanup bundle (3
   commits)`
2. **Outcome** — one-paragraph summary: three independent v1.18.0
   closeout doc-drift fixes landed as three atomic commits.
3. **Commits** — bullet list with commit hash (short), subject,
   files-touched count, and one-line WHY for each:
     - `<sha1>` `otel: drop redundant OTEL_BSP option overrides` —
       `internal/otel/provider.go` — autoexport env knobs were dead;
       SDK explicit options shadowed env defaults.
     - `<sha2>` `docs: close DEFER-70-06-01 in CLAUDE.md` —
       `CLAUDE.md` — Phase 73 BUG-01 shipped fix; paragraph still
       claimed open.
     - `<sha3>` `seeds: archive SEED-001 to consumed/` —
       `.planning/seeds/{,consumed/}SEED-001-…md` — quick task
       260426-pms consumed it in v1.17.0.
   Use the actual short SHAs from `git log --oneline -3`.
4. **Gate results** — only commit 1 ran Go gates. Record the four
   gate outputs (PASS / number of tests, etc.). Commits 2-3 were
   docs/yaml/file-move only — note "no Go gates required;
   `git diff --stat` and `ls`-based sanity checks ran clean".
5. **Bisectability** — confirm each commit on its own compiles and
   that gates were run after commit 1 (commit 1 is the only
   code-touching commit in the series).
6. **Files touched** — full enumeration:
     - `internal/otel/provider.go` (commit 1)
     - `CLAUDE.md` (commit 2)
     - `.planning/seeds/SEED-001-incremental-sync-evaluation.md` →
       `.planning/seeds/consumed/SEED-001-…md` (commit 3, rename +
       yaml)
7. **Out of scope / not touched** — explicitly mention:
     - metric Views block in `provider.go` (lines 90-189) — not
       touched.
     - other CLAUDE.md sections — not touched.
     - SEED-003/004/005 yaml — not touched.
     - The body content of SEED-001 — preserved verbatim.

Keep it concise — this is a bundle of three small fixes, not a phase.

After writing, commit on its own:

```
docs: 260503-imn doc drift cleanup bundle summary
```

Body: one short paragraph linking the three commits to v1.18.0
closeout backlog hygiene; no further detail (the SUMMARY.md itself
holds the detail).

Commit only the SUMMARY.md.
  </action>
  <verify>
    <automated>cd /home/dotwaffle/Code/pdb/peeringdb-plus &amp;&amp; test -e .planning/quick/260503-imn-doc-drift-cleanup-bundle/260503-imn-SUMMARY.md &amp;&amp; test "$(grep -cE '^# Quick Task 260503-imn' .planning/quick/260503-imn-doc-drift-cleanup-bundle/260503-imn-SUMMARY.md)" -eq 1 &amp;&amp; test "$(grep -cE 'otel: drop redundant|docs: close DEFER-70-06-01|seeds: archive SEED-001' .planning/quick/260503-imn-doc-drift-cleanup-bundle/260503-imn-SUMMARY.md)" -eq 3 &amp;&amp; git log -1 --pretty=%s | grep -qE '^docs: 260503-imn'</automated>
  </verify>
  <done>
- SUMMARY.md exists at the planned path and lists all three commits
  with short SHAs, subjects, files touched, and WHY blurbs.
- Gate results from commit 1 are recorded; commits 2-3 are noted as
  docs/yaml/file-move only.
- Out-of-scope items (metric Views, other CLAUDE.md, SEED-001 body,
  SEED-003/004/005) are explicitly enumerated.
- One new commit on HEAD covering only the SUMMARY.md, with subject
  `docs: 260503-imn doc drift cleanup bundle summary`.
- HEAD~4 is now the pre-bundle starting point; HEAD..HEAD~4 is a
  clean 4-commit series (3 fixes + 1 summary), each independently
  revertable, the 3 fixes individually bisectable.
  </done>
</task>

</tasks>

<verification>
End-of-bundle sanity:

1. `git log --oneline -4` shows in order (newest first):
     - `<sha4>` docs: 260503-imn doc drift cleanup bundle summary
     - `<sha3>` seeds: archive SEED-001 to consumed/
     - `<sha2>` docs: close DEFER-70-06-01 in CLAUDE.md
     - `<sha1>` otel: drop redundant OTEL_BSP option overrides

2. From the pre-bundle HEAD (`git rev-parse HEAD~4`), checking out
   `<sha1>` alone runs:
     - `go build ./...` PASS
     - `go vet ./...` PASS
     - `go test -race ./internal/otel/... -count=1` PASS
     - `golangci-lint run ./internal/otel/...` PASS

3. `<sha2>` and `<sha3>` are docs/file-move only — checking each out
   leaves the build at the same state as `<sha1>` for `<sha2>` and as
   `<sha2>` for `<sha3>`. No regressions possible (they don't touch
   `.go` files).

4. No conventional-commits prefixes (`feat:`, `fix:`, `chore:`) in
   any of the four subjects. Confirm with:
   `git log --oneline -4 | grep -cE 'feat|fix:|chore'` → 0.

5. Working tree clean after all four commits:
   `git status --porcelain | wc -l` → 0.
</verification>

<success_criteria>
- 4 new commits on `main` (3 fixes + 1 SUMMARY), each with kernel-style
  subject; no Conventional Commits, no `chore`.
- Each fix commit is independently revertable; no cross-commit
  dependency.
- Commit 1 passes go build, go vet, go test -race
  ./internal/otel/..., and golangci-lint ./internal/otel/...
- `internal/otel/provider.go` lines that mentioned `WithBatchTimeout`,
  `WithMaxExportBatchSize`, and `"time"` are gone.
- CLAUDE.md line 167 reads "Closed in Phase 73 (v1.18.0): …" with
  references to `ent/schema/campus_annotations.go` and
  `MILESTONES.md`.
- `.planning/seeds/` no longer hosts SEED-001; `.planning/seeds/consumed/`
  hosts both SEED-001 and SEED-002. `git log --follow` on the moved
  SEED-001 file traverses the rename correctly.
- SUMMARY.md exists at the planned path covering all three commits with
  hashes, files touched, and gate results for commit 1.
- No collateral changes anywhere else in the tree.
</success_criteria>

<output>
After completion, create
`.planning/quick/260503-imn-doc-drift-cleanup-bundle/260503-imn-SUMMARY.md`
(produced by Task 4 above; the file IS the output artefact).
</output>
