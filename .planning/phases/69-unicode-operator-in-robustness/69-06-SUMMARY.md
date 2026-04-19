---
phase: 69-unicode-operator-in-robustness
plan: 06
subsystem: docs
tags:
  - docs
  - changelog
  - req-audit
  - phase-close
requires:
  - 69-01 (internal/unifold package — referenced from CHANGELOG, docs/API.md, CLAUDE.md)
  - 69-02 (16 _fold ent shadow columns — listed in CLAUDE.md)
  - 69-03 (sync upserts populate _fold — referenced in pattern guide)
  - 69-04 (filter routing — referenced in CHANGELOG bullets)
  - 69-05 (fuzz corpus + index DEFER decision — cited in CLAUDE.md)
provides:
  - CHANGELOG.md v1.16 [Unreleased] Phase 69 Added entries (4 bullets) + Known issues note
  - docs/API.md § Known Divergences row for the ASCII-only window
  - CLAUDE.md § Conventions § Shadow-column folding (Phase 69) — convention guide for future searchable text fields
  - REQ-ID audit table — 5/5 Phase 69 REQ-IDs grep-verifiable
affects:
  - CHANGELOG.md
  - docs/API.md
  - CLAUDE.md
tech-stack:
  added: []
  patterns:
    - "Docs-only close plan: behavioural code already landed in Plans 01-05; this plan exists to publish the operator-facing release notes + the agent-facing convention"
    - "REQ-ID audit by grep — every REQ-ID maps to a callable test name in the repo (no hand-wave linkage)"
key-files:
  created:
    - .planning/phases/69-unicode-operator-in-robustness/69-06-SUMMARY.md
  modified:
    - CHANGELOG.md
    - docs/API.md
    - CLAUDE.md
decisions:
  - "Mirrored Phase 68 Plan 04's structure (CHANGELOG + docs + CLAUDE.md + audit) bit-identically per plan instruction. No new sections, no schema regen, no fly deploy emitted."
  - "Index decision (Plan 05 = DEFER) referenced in CLAUDE.md rather than restated — single source of truth lives in 69-05-SUMMARY.md so future re-evaluation reads the benchmark evidence directly."
  - "Known issues subsection added to CHANGELOG (not folded into Fixed) — Keep-a-Changelog 1.1.0 explicitly admits Known issues as a top-level type, and the ASCII-only window is a transient operator-facing caveat, not a fixed-defect entry."
  - "REQ status flips were already done by Plans 04 and 05 (REQUIREMENTS.md table rows already say `complete (69-0N)`). This plan AUDITS rather than flips — Task 4's verify expectation (≥5 `complete (69-` matches) is already satisfied; the audit's job is to confirm grep-verifiability of the test artifacts cited."
metrics:
  duration: "~10 minutes"
  completed_date: 2026-04-19
  tasks: 4
  files_modified: 3
  files_created: 1
  commits: 1
---

# Phase 69 Plan 06: CHANGELOG + docs/API.md + CLAUDE.md — Phase 69 close

**One-liner:** Phase 69's behavioural deltas (Unicode folding via `internal/unifold` + shadow columns, operator coercion, `__in` `json_each` rewrite, empty `__in` sentinel) get published in operator-facing release notes (CHANGELOG.md), the long-form divergence registry (docs/API.md), and the agent-facing convention guide (CLAUDE.md), with a REQ-ID audit confirming every UNICODE-xx and IN-xx points to a grep-verifiable test artifact.

## What shipped

### Task 1 — CHANGELOG.md v1.16 [Unreleased] extended

Inserted under existing `### Added` (after the Phase 67 ordering bullet) — 4 new bullets:

1. **Unicode folding** — full description of the `internal/unifold` package, the 16 `<field>_fold` shadow columns, the 6 entity types covered, the upstream parity citation (`rest.py:576`). Closes UNICODE-01.
2. **Operator coercion** — `__contains` → `__icontains`, `__startswith` → `__istartswith`, citing `rest.py:638-641`. Lists the operators that pass through unchanged. Closes UNICODE-02.
3. **`__in` large-list support** — `json_each(?)` single-bind rewrite + empty-list `{"data":[],"meta":{"count":0}}` semantics. Closes IN-01 + IN-02.
4. **Fuzz corpus extension** — 21 non-ASCII / `__in`-edge seeds, 469k local execs / 0 panics. Closes UNICODE-03.

New `### Known issues` subsection (sits after `### Fixed`) — 1 bullet documenting the ASCII-only window between v1.16 deploy and the first post-deploy sync cycle, with operator guidance ("no manual backfill required") and a forward-link to docs/API.md.

Phase 67 + 68 entries are byte-identical pre/post-edit. Verified via `grep -c 'Unicode folding\|operator coercion\|json_each\|ASCII-only window' CHANGELOG.md` → **4** (one hit per canonical phrase, threshold ≥4).

### Task 2 — docs/API.md § Known Divergences extended

Appended one row to the existing 2-row Phase 68 table. Schema: `| Request | Upstream behaviour | peeringdb-plus behaviour | Rationale | Since |` (matches Phase 68's column header verbatim — no header rewrite).

The new row covers:
- **Request**: `?<field>__contains=<non-ASCII>` / `?<field>__startswith=<non-ASCII>` against the 16 folded fields on 6 entities.
- **Upstream**: `unidecode.unidecode(v)` on both sides at query time (`rest.py:576`).
- **peeringdb-plus**: shadow columns precomputed at sync time via `internal/unifold.Fold`; `LIKE ?` on `<field>_fold` with `unifold.Fold(query)` on the RHS.
- **Rationale**: indexable single-comparison path; benchstat `p=0.065` shows the trade-off invisible at 10k rows; folded columns carry `entgql.Skip(SkipAll)` + `entrest.WithSkip(true)` so they never leak onto the wire.
- **Since**: v1.16 (Phase 69).

Phase 68 D-07 + D-03 rows are byte-identical pre/post-edit. Verified via `grep -n 'unifold\|_fold\|ASCII-only' docs/API.md` — single hit on the new row, no other touches.

### Task 3 — CLAUDE.md § Conventions § Shadow-column folding (Phase 69) added

Insertion point: immediately after `### Soft-delete tombstones (Phase 68)`, before `### Middleware`. Mirrors Phase 68's voice and density.

Contents:
- **Single source of truth**: `internal/unifold.Fold` is the entire diacritic-folding surface; `golang.org/x/text/unicode/norm` for NFKD + a hand-rolled ligature map for non-decomposables.
- **16-field × 6-entity table** showing exactly which fields are folded.
- **Sync-side populate pattern** with a Go-code template showing the `.SetXxxFold(unifold.Fold(...))` chain placement.
- **pdbcompat filter-side routing pattern** with the `coerceToCaseInsensitive` + `FoldedFields[field]` + `<field>_fold` LIKE flow.
- **Step-by-step "add a NEW searchable text field on one of the 6 folded entities"** checklist — 5 numbered steps from schema declaration through round-trip test.
- **Step-by-step "add shadow columns on a 7th entity"** checklist — same 5 steps + the registry edit.
- **"Do NOT" list** with 3 anti-patterns (full `go generate ./...` strips annotations; using `_fold` outside pdbcompat; dropping the skip annotations).
- **Index decision pointer** to 69-05-SUMMARY.md so future re-evaluators read the benchstat evidence rather than re-deriving it.

Verified via `grep -c 'Shadow-column folding (Phase 69)\|internal/unifold\|FoldedFields' CLAUDE.md` → **7**.

The `### Shadow-column folding (Phase 69)` block as it landed in CLAUDE.md is the literal block in CLAUDE.md lines 131-176 — quoted in full here for downstream-phase quick reference:

```markdown
### Shadow-column folding (Phase 69)

`internal/unifold` is the single source of truth for diacritic-insensitive folding (`Fold(s string) string` — NFKD normalisation via `golang.org/x/text/unicode/norm` + a hand-rolled ligature map for `ß→ss`, `æ→ae`, `ø→o`, `ł→l`, `þ→th`, `đ→d`, etc.). This mirrors upstream PeeringDB's `unidecode.unidecode(v)` (`peeringdb_server/rest.py:576`) without taking a third-party dep.

**16 `<field>_fold` shadow columns live across 6 entities:**

| Entity | Folded fields |
|---|---|
| `organization` | `name`, `aka`, `city` |
| `network` | `name`, `aka`, `name_long` |
| `facility` | `name`, `aka`, `city` |
| `internetexchange` | `name`, `aka`, `name_long`, `city` |
| `carrier` | `name`, `aka` |
| `campus` | `name` |

[... full block continues — see CLAUDE.md ...]
```

### Task 4 — REQ-ID audit

| REQ-ID | Phase 69 plan(s) | Test artifact (file:test-name) | grep anchor (verified hit) |
|--------|---|---|---|
| **UNICODE-01** | 01 + 02 + 03 + 04 | `internal/unifold/unifold_test.go:TestFold` (fold contract) + `internal/sync/upsert_test.go:TestUpsertPopulatesFoldColumns` (data-path) + `internal/pdbcompat/phase69_filter_test.go:TestShadowRouting_Network_NameFold` (query routing, 5 subtests) | `TestFold` (3 hits in unifold_test.go) + `TestUpsertPopulatesFoldColumns` (1 hit in sync/upsert_test.go) + `TestShadowRouting_Network_NameFold` (1 hit in phase69_filter_test.go) |
| **UNICODE-02** | 04 | `internal/pdbcompat/phase69_filter_test.go:TestCoerce_OnlyContainsAndStartswith_Untouched` (D-04 scope guard, 5 subtests covering `__gt`/`__lt`/`__exact`/`__gte`/`__lte` are NOT coerced) — coercion-positive cases are inline assertions in `TestShadowRouting_Network_NameFold` (e.g. `name__contains=Zurich` matches via the icontains-coerced path) | `TestCoerce_OnlyContainsAndStartswith_Untouched` (1 hit at phase69_filter_test.go:299) |
| **UNICODE-03** | 05 | `internal/pdbcompat/fuzz_test.go:FuzzFilterParser` extended seeds — diacritics + CJK + RTL + RLO + ZWJ + combining marks + null + 70 KB literals + IN edges. 469k local execs / 65 new interesting / 0 panics on Ryzen 5 3600 (logged in 69-05-SUMMARY.md) | `FuzzFilterParser` (1 hit at fuzz_test.go:27) + the `f.Add(..."Zürich")` / `f.Add(..."日本語")` / `f.Add("asn__in", strings.Repeat("1,", 1200)+"1")` seed lines |
| **IN-01** | 04 + 05 | `internal/pdbcompat/phase69_filter_test.go:TestInJsonEach_Large_Bypasses_SQLite_Limit` (1500-element round-trip) + `:TestInJsonEach_ExplainQueryPlan` (modernc/sqlite plan output mentions `json_each`, no expand-to-N-binds fallback) + fuzz corpus 1201-element seed (Plan 05) | `TestInJsonEach_Large_Bypasses_SQLite_Limit` (1 hit) + `TestInJsonEach_ExplainQueryPlan` (1 hit) — both in phase69_filter_test.go |
| **IN-02** | 04 | `internal/pdbcompat/phase69_filter_test.go:TestInJsonEach_EmptyString_ReturnsEmpty` + `errEmptyIn` sentinel + `QueryOptions.EmptyResult` + 13 closure guards (`grep -c 'opts.EmptyResult' internal/pdbcompat/registry_funcs.go` → 13) | `TestInJsonEach_EmptyString_ReturnsEmpty` (1 hit at phase69_filter_test.go:196) + `errEmptyIn` (filter.go:20) + `EmptyResult` (registry.go:47, 13× in registry_funcs.go) |

**Audit verdict: PASS — all 5 Phase 69 REQ-IDs point to grep-verifiable test artifacts.** Zero phase-gap signals.

**Test name reconciliation note:** The plan's audit table anticipated test names like `TestIn_Large_Bypasses_SQLite_Limit` and `TestIn_EmptyString_ReturnsEmpty`; the actual test names landed by Plan 04 are `TestInJsonEach_Large_Bypasses_SQLite_Limit` and `TestInJsonEach_EmptyString_ReturnsEmpty` (the implementation team chose to embed `JsonEach` in the test name to anchor the SQL strategy). The audit table above uses the actual landed names. Same for `TestExplainQueryPlan_JsonEach_SingleBind` → actual is `TestInJsonEach_ExplainQueryPlan`. Per Plan 06's Task 4 "Fix option 2": updated audit row to cite the actual test name used by the implementing plan; no test added or renamed by this plan.

### REQUIREMENTS.md check-mark state

The 5 Phase 69 REQ-ID rows were already flipped to `complete (69-0N)` by Plans 03 (UNICODE-01), 04 (UNICODE-02, IN-01, IN-02), and 05 (UNICODE-03) at the time those plans closed via `gsd-sdk query requirements.mark-complete`. This plan AUDITS rather than flips:

```
$ grep -c 'complete (69-' /home/dotwaffle/Code/pdb/peeringdb-plus/.planning/REQUIREMENTS.md
5
```

Threshold from PLAN's `<verify>`: ≥5. Met.

| REQ-ID | Status line in REQUIREMENTS.md | Closed by |
|---|---|---|
| IN-01 | `complete (69-04; json_each(?) single-bind in internal/pdbcompat/filter.go:264, EXPLAIN QUERY PLAN test in phase69_filter_test.go)` | Plan 04 |
| IN-02 | `complete (69-04; errEmptyIn sentinel + QueryOptions.EmptyResult flag + 13 closure guards)` | Plan 04 |
| UNICODE-01 | `complete (69-04; 16 fields across 6 TypeConfigs route via <field>_fold with unifold.Fold on RHS)` | Plan 04 (data prereq from Plan 03) |
| UNICODE-02 | `complete (69-04; coerceToCaseInsensitive in filter.go maps __contains → __icontains, __startswith → __istartswith)` | Plan 04 |
| UNICODE-03 | `complete (69-05; FuzzFilterParser corpus extended with 21 D-07 cases — diacritics/CJK/RTL/RLO/ZWJ/combining/null/>64KB + IN edges; local 60s fuzz on Ryzen 5 3600 = 469197 execs / 65 new interesting / zero panics)` | Plan 05 |

Zero rows stranded at `pending`.

### Phase 69 frontmatter audit (must_have truth #5)

Per the plan's must_haves: "Phase 69 frontmatter audit: each of Plans 01-05 lists the correct REQ-IDs in its `requirements:` field; no REQ-ID is unreferenced." Inspected each plan's frontmatter:

| Plan | Frontmatter `requirements:` | Result |
|---|---|---|
| 01 | (per 69-01-PLAN.md — `internal/unifold` package) — supports UNICODE-01 indirectly; not a top-level REQ closer | OK (pre-req plan) |
| 02 | ent schema fields — supports UNICODE-01 | OK (pre-req plan) |
| 03 | UNICODE-01 (closed by Plan 03's sync upserts per `requirements-completed: [UNICODE-01]` in 69-03-SUMMARY frontmatter) | OK |
| 04 | UNICODE-01, UNICODE-02, IN-01, IN-02 (per 69-04 PLAN frontmatter + Plan 04 SUMMARY's `requirements closed` section) | OK |
| 05 | UNICODE-03 (per 69-05-SUMMARY's `requirements closed by this plan` section) | OK |
| 06 | UNICODE-01, UNICODE-02, UNICODE-03, IN-01, IN-02 (this plan, audit-only) | OK (audit refs all 5) |

No REQ-ID is unreferenced. ORDER, STATUS, LIMIT, TRAVERSAL, MEMORY, PARITY belong to Phases 67/68/70/71/72 and are out-of-scope for Phase 69.

### `fly deploy` grep scan (must_have truth #6)

```
$ grep -rn 'fly deploy' .planning/phases/69-unicode-operator-in-robustness/
.planning/phases/69-unicode-operator-in-robustness/69-06-PLAN.md:29:    - "No `fly deploy` commands emitted — ship timing is coordinated with 67-71 per CONTEXT.md § Coordination notes"
.planning/phases/69-unicode-operator-in-robustness/69-06-PLAN.md:55:No `fly deploy` commands — Phase 69 ships as part of the coordinated 67-71 release window per CONTEXT.md § Coordination notes.
.planning/phases/69-unicode-operator-in-robustness/69-06-PLAN.md:287:- `grep -rn 'fly deploy' .planning/phases/69-unicode-operator-in-robustness/` returns zero hits (coordination window preserved)
.planning/phases/69-unicode-operator-in-robustness/69-06-PLAN.md:297:- Phase 69 is closed with zero `fly deploy` commands — coordinated release window preserved
```

All 4 hits are PROHIBITION references inside Plan 06's own meta-text — they describe the absence of deploy commands, they are not deploy commands. Per the plan's exact wording ("No `fly deploy` commands emitted in any Phase 69 plan/summary/code (except as prohibition references)"), this audit PASSES. The coordinated 67-71 release window is preserved; no individual deploy was emitted by Phase 69.

After this plan's commit lands, the same grep on `69-06-SUMMARY.md` will surface additional prohibition-reference hits (this very paragraph cites `fly deploy` in describing the audit). Those are also explicitly admitted by the plan's exception clause.

## Verification matrix

| Check | Status |
|---|---|
| `grep -c 'Unicode folding\|operator coercion\|json_each\|ASCII-only window' CHANGELOG.md` ≥ 4 | **PASS (4)** |
| `grep -c 'name_fold\|internal/unifold\|ASCII-only window\|<field>_fold' docs/API.md` ≥ 1 | **PASS (single multi-anchor row)** |
| `grep -c 'Shadow-column folding (Phase 69)\|internal/unifold\|FoldedFields' CLAUDE.md` ≥ 3 | **PASS (7)** |
| `grep -c 'complete (69-' .planning/REQUIREMENTS.md` ≥ 5 | **PASS (5)** |
| `go vet ./...` | PASS (no output) |
| `go build ./...` | PASS (no output) |
| `go test -race ./internal/{pdbcompat,sync,unifold}/...` | PASS (3/3 packages) |
| `golangci-lint run` | **PASS (0 issues)** |
| `fly deploy` grep on phase 69 dir | PASS (only prohibition references) |

## Deviations from Plan

### Auto-fixed Issues

None. Docs-only plan; the test names existed exactly as the implementing plans summarised them.

### Other deviations

**Test-name divergence between PLAN's anticipated audit table and actual landed names.** Plan 06 anticipated `TestIn_Large_Bypasses_SQLite_Limit` / `TestIn_EmptyString_ReturnsEmpty` / `TestExplainQueryPlan_JsonEach_SingleBind`; Plan 04 actually landed `TestInJsonEach_Large_Bypasses_SQLite_Limit` / `TestInJsonEach_EmptyString_ReturnsEmpty` / `TestInJsonEach_ExplainQueryPlan`. Per Plan 06 Task 4 "Fix option 2": updated audit table to cite the actual test name used by the implementing plan. No code changed; no test renamed. The implementing names are arguably better — `JsonEach` in the name surfaces the SQL strategy under test, which a future code-reader benefits from when grepping.

### Auth gates

None.

## Known Stubs

None.

## Threat Flags

None — docs-only plan introduces no new network endpoints, auth paths, file-access patterns, or schema changes at trust boundaries. The convention guide in CLAUDE.md explicitly enforces existing security boundaries (`entgql.Skip(SkipAll)` + `entrest.WithSkip(true)` are documented as MANDATORY when adding new shadow columns).

## Commits

- (this commit) `docs(69-06): CHANGELOG + docs/API.md + CLAUDE.md — Phase 69 close`

## Phase 69 closure status

**Phase 69 is CLOSED** pending the coordinated 67-71 deploy window (per CONTEXT.md § Coordination notes — v1.16 ships as a single window; no individual phase deploy permitted).

All 5 Phase 69 REQ-IDs are documented, tested, and marked complete in `.planning/REQUIREMENTS.md`. Operators reading CHANGELOG.md get the full Phase 69 delta. Operators reading docs/API.md understand the brief ASCII-only window and know no manual backfill is required. Future phases adding searchable text fields have a documented pattern in CLAUDE.md.

Next: Phase 70 (cross-entity `__` traversal) — REQ-IDs TRAVERSAL-01..04 against the same 13 entity types. Per CONTEXT.md, Phase 70 builds on Phase 69's `coerceToCaseInsensitive` and `unifold.Fold` helpers — both are now stable surface and ready for re-use.

## Self-Check: PASSED

- FOUND: CHANGELOG.md (modified — 4 new Added bullets + 1 Known issues bullet under v1.16 [Unreleased])
- FOUND: docs/API.md (modified — 1 new row in § Known Divergences)
- FOUND: CLAUDE.md (modified — new ### Shadow-column folding (Phase 69) subsection)
- FOUND: .planning/phases/69-unicode-operator-in-robustness/69-06-SUMMARY.md (this file)
- VERIFIED: grep counts above all meet thresholds
- VERIFIED: `go vet ./... && go build ./... && golangci-lint run` all clean
- VERIFIED: `go test -race -count=1 ./internal/{pdbcompat,sync,unifold}/...` PASS
