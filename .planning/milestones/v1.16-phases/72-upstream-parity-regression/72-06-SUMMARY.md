---
phase: 72
plan: 06
subsystem: docs + traceability
tags: [parity, docs, milestone-close, traceability]
requires:
  - docs/API.md § Known Divergences (Phase 68/69/70 seed rows)
  - internal/testutil/parity/fixtures.go (SHA-pinned to peeringdb/peeringdb@99e92c72)
  - internal/pdbcompat/parity/ (TestParity_<Category> names for cross-refs)
  - .planning/phases/70-cross-entity-traversal/deferred-items.md (DEFER-70-06-01 + DEFER-70-verifier-01)
provides:
  - docs/API.md § Known Divergences extended (3 new rows + parity cross-refs on existing DEFER rows)
  - docs/API.md § Validation Notes NEW sub-section (5 invalid-pdbfe-claims with pinned SHA refs)
  - CLAUDE.md § Upstream parity regression (Phase 72) convention subsection
  - CHANGELOG.md v1.16 [Unreleased] Phase 72 Added block + milestone-close note
  - REQUIREMENTS.md PARITY-02 flipped to complete (25/25 v1.16 REQ-IDs now complete)
  - ROADMAP.md Phase 72 [x] + v1.16 ✅ complete 2026-04-19
affects:
  - docs/API.md (34 insertions, 3 deletions — 540-line § Known Divergences + new § Validation Notes at 559)
  - CHANGELOG.md (+65 -7 lines — Phase 72 Added block + milestone-close note)
  - CLAUDE.md (+18 -0 lines — § Upstream parity regression subsection under Conventions)
  - .planning/REQUIREMENTS.md (PARITY-02 checkbox + traceability row)
  - .planning/ROADMAP.md (Phase 72 checkbox + plan listing + Progress table + milestone status)
tech-stack:
  added: []
  patterns:
    - "Divergence registry as single source of truth: every docs/API.md § Known Divergences row has a matching TestParity_*/DIVERGENCE_* assertion in internal/pdbcompat/parity/*_test.go (bidirectional grep)"
    - "Validation Notes companion pattern: third-party invalid-claim registry with pinned upstream SHA refs so future audits don't re-research the same gotchas"
    - "Advisory drift detection (CONTEXT.md D-03): cmd/pdb-fixture-port/ --check compares current upstream against pinned SHA; not blocking for PR merges"
key-files:
  created: []
  modified:
    - docs/API.md
    - CHANGELOG.md
    - CLAUDE.md
    - .planning/REQUIREMENTS.md
    - .planning/ROADMAP.md
decisions:
  - "Validation Notes anchor into Known Divergences via the pdbfe limit=0 row's See-§-Validation-Notes pointer — single-navigation path from a divergence row to the invalid-claim rationale"
  - "All 5 Validation Notes rows pin the same SHA (peeringdb/peeringdb@99e92c726172ead7d224ce34c344eff0bccb3e63) matching the fixtures.go header — ensures the divergence registry and the parity test suite always cite the same upstream snapshot"
  - "DEFER-70-06-01 + DEFER-70-verifier-01 Since columns updated in-place (not new rows) so the audit trail shows the Phase 70 provenance + Phase 72 lock-in atomically"
  - "ROADMAP.md Progress table corrected for phases 68 + 69 (stale in-progress rows from earlier runs; actual on-disk state was 4/4 + 6/6 complete)"
metrics:
  duration_minutes: 18
  completed_date: 2026-04-19
  commits: 2
  files_touched: 5
  lines_added: 128
  lines_removed: 17
---

# Phase 72 Plan 06: v1.16 milestone close — docs/API.md § Known Divergences + § Validation Notes

**One-liner:** Phase 72 docs close — extend docs/API.md § Known Divergences with 3 new rows + NEW § Validation Notes sub-section (5 invalid-pdbfe-claims pinned to peeringdb/peeringdb@99e92c72), add CLAUDE.md § Upstream parity regression convention, append Phase 72 block to CHANGELOG.md v1.16, flip PARITY-02 + Phase 72 to complete — v1.16 Django-compat Correctness milestone complete.

## Goal

Close Phase 72 with the docs-and-traceability delta per PARITY-02: establish `docs/API.md § Known Divergences` as the permanent single source of truth for intentional non-parity outcomes, add the companion `§ Validation Notes` sub-section so future conformance auditors distinguish regression from design, and flip all 25 v1.16 REQ-IDs to complete so `/gsd-complete-milestone` can archive v1.16.

## Commits

- `1f0b120` — `docs(72-06): extend docs/API.md § Known Divergences + add § Validation Notes`
- `aaef0d1` — `docs(72-06): close phase 72 — CLAUDE.md + CHANGELOG + REQ/ROADMAP traceability`

## Work

### Task 1 — docs/API.md (commit 1f0b120)

**§ Known Divergences extension — 3 new rows:**

1. **Pre-Phase-68 hard-delete gap parity cross-ref.** Existing row at line 552 updated to cross-reference `TestParity_Status/STATUS-04_list_since_admits_deleted_excludes_pending_noncampus` and pin the upstream `rest.py:694-727` SHA. Tombstone visibility gap remains correctly documented; parity test now locks the post-v1.16 tombstone visibility via `?status=deleted&since=N`.

2. **pdbfe `limit=0` count-only invalid claim.** New row — upstream `rest.py:494-497` treats `limit=0` as unlimited, not count-only. Cross-references § Validation Notes entry 2 and `TestParity_Limit/LIMIT-01_zero_returns_all_rows_unbounded` + `LIMIT-01b_zero_over_budget_returns_413_problem_json`. Callers wanting a count should use `meta.count` on a depth=0 list.

3. **Depth-on-list silent-drop LIMIT-02 guardrail.** New row — upstream `rest.py:744-748` accepts `?depth=` on list requests with `API_DEPTH_ROW_LIMIT=250` cap; we silently drop `?depth=` on list endpoints via Phase 68 LIMIT-02 guardrail. Memory envelope on 256 MB replicas (Phase 71 D-06) would refuse the resulting response sizes. Cross-references `TestParity_Limit/LIMIT-02_depth_on_list_silently_dropped_DIVERGENCE`.

**§ Known Divergences — existing DEFER rows cross-referenced:**

- **DEFER-70-06-01 (`fac?campus__name=X` HTTP 500)** — Since column now reads `v1.16 (Phase 70; locked by TestParity_Traversal/DIVERGENCE_fac_campus_name_returns_500 in Phase 72)`.
- **DEFER-70-verifier-01 (`fac?ixlan__ix__fac_count__gt=0` silent-ignore)** — Since column now reads `v1.16 (Phase 70; locked by TestParity_Traversal/DIVERGENCE_fac_ixlan_ix_fac_count_silent_ignore in Phase 72)`.

**§ Validation Notes — NEW sub-section (5 rows):**

All 5 pin `peeringdb/peeringdb@99e92c726172ead7d224ce34c344eff0bccb3e63` (matches fixtures.go header):

1. `net?country=NL` is **NOT** a valid filter — `country` lives on `org`. Cite `serializers.py:2938-2992`. Parity: `TestParity_Traversal/TRAVERSAL-04_unknown_field_silently_ignored_with_otel_attr`.
2. `?limit=0` is unlimited, **NOT** count-only — cite `rest.py:494-497`. Parity: `TestParity_Limit/LIMIT-01_*`.
3. Default ordering is `(-updated, -created)`, **NOT** `id ASC` — cite `django-handleref/models.py:95-101`. Parity: `TestParity_Ordering/default_list_order_updated_desc`.
4. Unicode folding is Python `unidecode`, **NOT** MySQL collation — cite `rest.py:576`. Parity: `TestParity_Unicode/UNICODE-01_*`.
5. Filter surface is `prepare_query` + `queryable_relations` with `FILTER_EXCLUDE` denylist, **NOT** a DRF `filterset_class` — cite `serializers.py:754-780` + `:128-157`. Parity: `TestParity_Traversal/TRAVERSAL-01_path_a_1hop_org_name` + `TRAVERSAL-03_path_b_1hop_org_city`.

Each row carries specific upstream file:line citations and cross-references the parity sub-test locking the corrected behaviour. Closing paragraph documents quarterly drift-detection policy (advisory per CONTEXT.md D-03, not blocking).

### Task 2 — CLAUDE + CHANGELOG + REQ + ROADMAP (commit aaef0d1)

**CLAUDE.md:** New `### Upstream parity regression (Phase 72)` subsection under `## Conventions` (inserted after `### Response memory envelope (Phase 71)`, before `### Middleware`). Covers 5 maintainer blocks:

1. Adding a new parity test (pick category file by REQ-ID prefix, `t.Parallel()`, citation comment, `seedFixtures`, `testutil.SetupClient(tb)` with `testing.TB` widening)
2. Porting new fixtures (`go generate ./internal/testutil/parity/` idempotency + SHA-pinned header + `--upstream-commit` override + `--check` drift flag)
3. Quarterly drift check (advisory per D-03, not blocking; drift triages into § Validation Notes updates or new § Known Divergences rows)
4. Divergence registry (bidirectional grep: every docs/API.md row has a matching parity test; new divergences follow a 3-step workflow)
5. Benchmarks (3 `b.Loop()` benchmarks, local-run only per D-06, no benchstat-on-main gate)

**CHANGELOG.md v1.16 [Unreleased]:**

- Coordinated release window note refined: Phase 72 is a CI regression gate only, no production deploy required.
- v1.16 milestone-complete note added: all 6 phases (67-72) shipped, 25/25 requirements across 8 categories (ORDER, STATUS, LIMIT, IN, UNICODE, TRAVERSAL, MEMORY, PARITY) traced and complete. Next action: `/gsd-complete-milestone`.
- New `Phase 72 (PARITY-01, PARITY-02): Upstream parity regression lock-in` block with 5 bullets under Added:
  - internal/pdbcompat/parity/ category-split regression suite (31 hard-pass tests + 4 DIVERGENCE markers)
  - cmd/pdb-fixture-port/ fixture-porting tool (5560 rows @ 99e92c72...)
  - bench_test.go performance lock-in (3 `b.Loop()` benchmarks)
  - docs/API.md § Known Divergences extension (3 new rows + DEFER cross-refs)
  - docs/API.md § Validation Notes NEW sub-section (5 invalid-pdbfe-claims)

**REQUIREMENTS.md:** `PARITY-02` checkbox `[ ]` → `[x]`. Traceability row flipped from `pending` to `complete (72-06; ...)` with the full docs/API.md + CLAUDE.md + CHANGELOG.md deliverable manifest. All 25 v1.16 REQ-IDs now show complete.

**ROADMAP.md:** Phase 72 `[ ]` → `[x]`. Plans 6/6 executed. v1.16 milestone status `🟡 defined` → `✅ complete 2026-04-19`. Progress table corrected for Phase 68 (was stale 1/4) + Phase 69 (was stale 3/6) + Phase 72 (was 5/6, now 6/6).

## Deviations from Plan

None - plan executed exactly as written.

Minor additions beyond the plan scope (Rule 2 — auto-add missing critical functionality): the ROADMAP.md Progress table for Phases 68 + 69 was stale from earlier in-progress runs (on-disk SUMMARY.md files prove 4/4 + 6/6 complete). Correcting the rows to match reality is a traceability-integrity requirement for the milestone-close invariant. Documented in the Task 2 commit message.

## Verification

**docs/API.md grep invariants (per plan <verification>):**

```
grep -c "Validation Notes" docs/API.md       # 2 (header + in-body reference)
grep -c "peeringdb/peeringdb@" docs/API.md   # 8 (≥5 required)
grep -c "TestParity_" docs/API.md            # 10 (≥5 required)
```

**CLAUDE.md + CHANGELOG.md grep invariants:**

```
grep -c "Upstream parity regression (Phase 72)" CLAUDE.md   # 1
grep -c "Phase 72" CHANGELOG.md                             # 4
```

**REQUIREMENTS.md completeness:**

```
grep -cE 'PARITY-0[12] \| 72 \| complete' .planning/REQUIREMENTS.md   # 2
# All 25 v1.16 REQ-IDs complete (grep-audited)
```

**ROADMAP.md:**

```
grep -c "Phase 72.*\[x\]" .planning/ROADMAP.md   # 1
```

**No fly deploy imperatives added** (verified — zero hits in docs/API.md + CHANGELOG.md; 2 pre-existing CLAUDE.md hits are unchanged Phase 65 deployment guidance).

**Guardrail sanity:** `go build ./...` clean (no code edits in this plan; sanity check passed).

## Human verification

Auto-mode active — `checkpoint:human-verify` auto-approved per `/gsd-execute-phase` protocol. All grep invariants pre-verified before commit. No production change; this plan is docs-and-traceability only.

## Self-Check

Paths:

- `docs/API.md` — Known Divergences extended + Validation Notes added (34 insertions, 3 deletions via commit 1f0b120) — PRESENT
- `CHANGELOG.md` — Phase 72 block + milestone-close note (commit aaef0d1) — PRESENT
- `CLAUDE.md` — § Upstream parity regression subsection (commit aaef0d1) — PRESENT
- `.planning/REQUIREMENTS.md` — PARITY-02 complete (commit aaef0d1) — PRESENT
- `.planning/ROADMAP.md` — Phase 72 [x] + v1.16 ✅ (commit aaef0d1) — PRESENT

Commits:

- `1f0b120` — FOUND
- `aaef0d1` — FOUND

## Self-Check: PASSED

## Next action

`/gsd-complete-milestone` — archive v1.16 (Django-compat Correctness) into `.planning/milestones/v1.16-*`. Snapshots to archive: ROADMAP.md (current v1.16 rows), REQUIREMENTS.md (25/25 REQ-IDs with traceability notes), and all Phase 67-72 artifacts (PLAN.md / SUMMARY.md / VERIFICATION.md / REVIEW.md where present) into `.planning/milestones/v1.16-phases/`.
