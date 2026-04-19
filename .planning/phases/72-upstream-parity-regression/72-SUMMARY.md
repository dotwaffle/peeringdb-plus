---
phase: 72
subsystem: pdbcompat + parity + docs
tags: [parity, regression, milestone-close, v1.16, django-compat]
milestone: v1.16
status: complete
completed: 2026-04-19
requires:
  - Phases 67-71 (all v1.16 semantics must be shipped before parity can lock them)
  - peeringdb/peeringdb@99e92c726172ead7d224ce34c344eff0bccb3e63 (pinned upstream snapshot)
provides:
  - cmd/pdb-fixture-port/ (fixture-porting tool with --check drift detection)
  - internal/testutil/parity/fixtures.go (5560 ported rows across 6 category vars)
  - internal/pdbcompat/parity/ (6 TestParity_<Category> regression test files + harness + bench)
  - docs/API.md § Known Divergences (permanent divergence registry)
  - docs/API.md § Validation Notes (invalid-pdbfe-claims registry)
  - CLAUDE.md § Upstream parity regression (Phase 72) convention subsection
affects:
  - internal/testutil/testutil.go (testing.TB widening across SetupClient/SetupClientWithDB)
  - docs/API.md, CHANGELOG.md, CLAUDE.md (docs deliverables)
  - .planning/REQUIREMENTS.md (PARITY-01 + PARITY-02 → complete; 25/25 v1.16 REQ-IDs complete)
  - .planning/ROADMAP.md (Phase 72 [x]; v1.16 ✅ complete 2026-04-19)
tech-stack:
  added:
    - cmd/pdb-fixture-port (local Go tool for upstream fixture port)
    - internal/testutil/parity (generated fixture package)
    - internal/pdbcompat/parity (regression test suite + benchmarks)
  patterns:
    - "Category-split regression test files (TestParity_<Category> entry + sub-tests)"
    - "SHA-pinned upstream snapshot with --check drift alert (advisory, not blocking per D-03)"
    - "Bidirectional grep invariant between docs/API.md § Known Divergences and parity test names"
    - "Validation Notes registry (companion to Known Divergences for invalid third-party claims)"
    - "testing.TB-widened harness for bench + test co-use (b.Loop() per GO-TOOL-1)"
requirements_completed:
  - PARITY-01
  - PARITY-02
plans:
  - 72-01
  - 72-02
  - 72-03
  - 72-04
  - 72-05
  - 72-06
---

# Phase 72 Summary: Upstream parity regression + divergence docs

**One-liner:** Lock the v1.16 pdbcompat semantics in place with a 6-category-split regression suite fed by 5560 upstream fixtures ported from `peeringdb/peeringdb@99e92c72`'s `pdb_api_test.py`, backed by 3 `b.Loop()` performance benchmarks, with permanent divergence + invalid-pdbfe-claims registries in `docs/API.md`. v1.16 Django-compat Correctness milestone complete.

## Goal

Close v1.16 by asserting that every behaviour introduced in Phases 67-71 matches upstream PeeringDB's `pdb_api_test.py` ground truth, documenting every intentional divergence with cross-referenced parity sub-tests, and establishing a quarterly drift protocol so future upstream changes are visible without blocking merges.

## Deliverables

| Deliverable | Location | Size / Metric |
|-------------|----------|---------------|
| Fixture port tool | `cmd/pdb-fixture-port/` | 1060 LOC (main.go + main_test.go) across 3 category parsers (ordering / status+limit / unicode+in+traversal) + `--upstream-commit` override + `--check` drift flag + `--append` flag |
| Ported fixtures (Go literals) | `internal/testutil/parity/fixtures.go` | 5560 rows across 6 category vars (12 ord + 46 status + 270 limit + 216 unicode + 5002 in + 14 traversal); SHA-pinned header |
| Regression test suite | `internal/pdbcompat/parity/*.go` | 6 test files + harness (`ordering_test.go` / `status_test.go` / `limit_test.go` / `unicode_test.go` / `in_test.go` / `traversal_test.go` / `harness_helpers_test.go`); 31 hard-pass tests (27 v1.16-semantic + 4 harness probes); 2 explicit DIVERGENCE asserts; 15 citation hits; 36 `t.Parallel()`; 4 DIVERGENCE markers |
| Performance lock-in | `internal/pdbcompat/parity/bench_test.go` | 3 `b.Loop()` benchmarks (2-hop traversal ~580μs/op; limit=0 streaming ~82.7ms/op; 5001-element IN ~98.6ms/op on Ryzen 5 3600) |
| Divergence registry | `docs/API.md § Known Divergences` | Extended with 3 Phase 72 rows + TestParity_* cross-refs on 2 existing DEFER rows |
| Validation Notes | `docs/API.md § Validation Notes` (NEW) | 5 invalid-pdbfe-claims pinned to `peeringdb/peeringdb@99e92c72` |
| Developer convention | `CLAUDE.md § Upstream parity regression (Phase 72)` | 5 maintainer blocks (add test / port fixtures / drift check / registry / benchmarks) |
| Release notes | `CHANGELOG.md v1.16 [Unreleased]` | Phase 72 Added block (5 bullets) + milestone-close note |
| Traceability | `.planning/REQUIREMENTS.md` + `.planning/ROADMAP.md` | 25/25 v1.16 REQ-IDs complete; v1.16 flipped 🟡 → ✅ |

## Plans

| Plan | Title | Commits | Key output |
|------|-------|---------|------------|
| 72-01 | `cmd/pdb-fixture-port` scaffold + ordering PoC | `6db76d4`, `dd66a47`, `b468986` | Tool scaffold + OrderingFixtures (12 rows) emission + SHA-pinned header |
| 72-02 | Extend to STATUS + LIMIT categories + `--category all` | `235eaf0`, `6700f83`, `dc1139e`, `cca6c3c`, `d34fcc6` | StatusFixtures (46) + LimitFixtures (270) + `--append` flag |
| 72-03 | Extend to UNICODE + IN + TRAVERSAL + full 5560-row emission | `0d97493`, `6c79e26`, `056bf51`, `2846283`, `6d8ef8e` | UnicodeFixtures (216) + InFixtures (5002) + TraversalFixtures (14); `--upstream-commit` override |
| 72-04 | `internal/pdbcompat/parity/` — 6 category-split regression tests + harness | `d192859`, `1a83c8c`, `2b8bf1d` | 31 hard-pass tests + 2 DIVERGENCE asserts; 27 v1.16-semantic subtests covering ORDER-01..03 / STATUS-01..06 / LIMIT-01/01b/02 / UNICODE-01/02 / IN-01/02/03 / TRAVERSAL-01..04 (PARITY-01) |
| 72-05 | `parity/bench_test.go` — 3 `b.Loop()` benchmarks (D-07) | `e325752`, `bc40b50` | 237 LOC; BenchmarkParity_{TwoHopTraversal, LimitZeroStreaming, InFiveThousandElements}; testing.TB widening across testutil.SetupClient + 9 harness helpers (PARITY-01) |
| 72-06 | docs/API.md § Known Divergences + NEW § Validation Notes + v1.16 milestone close | `1f0b120`, `aaef0d1` | 3 new divergence rows + 5 validation notes (SHA-pinned) + CLAUDE.md convention + CHANGELOG v1.16 close + PARITY-02 flipped complete (PARITY-02) |

## Aggregate metrics

| Metric | Value |
|--------|-------|
| Total plans | 6/6 complete |
| Total commits | 20 (feature/test/docs + state commits) |
| Fixture rows ported | 5560 (12 + 46 + 270 + 216 + 5002 + 14) |
| Upstream SHA pinned | `peeringdb/peeringdb@99e92c726172ead7d224ce34c344eff0bccb3e63` |
| Source SHA256 | `sha256:75c7a6fab734db782b9035a6bc23ae11abcce5901a6017a051f76bbb51399043` |
| Parity test subtests | 31 hard-pass (27 v1.16-semantic + 4 harness probes) |
| DIVERGENCE assertions | 2 explicit (`DIVERGENCE_fac_ixlan_ix_fac_count_silent_ignore` + `DIVERGENCE_fac_campus_name_returns_500`) |
| `t.Parallel()` call sites | 36 |
| `pdb_api_test.py` / synthesised citation hits | 15 |
| Benchmark functions | 3 (all `b.Loop()` style per GO-TOOL-1) |
| Suite wall time under `-race` | 15.4s |
| Validation Notes entries | 5 (invalid-pdbfe-claims with pinned SHA refs) |
| Known Divergences rows | 8 total (existing Phase 68/69/70 seed + 3 new Phase 72 rows) |
| v1.16 REQ-IDs complete | 25/25 across 8 categories (ORDER, STATUS, LIMIT, IN, UNICODE, TRAVERSAL, MEMORY, PARITY) |

## Key decisions (CONTEXT.md-locked)

- **D-01** Category-split test layout (`ordering_test.go` + `status_test.go` + ... + `traversal_test.go`) — one entry test per category with sub-tests using `t.Parallel()` liberally.
- **D-02** Fixture source: port upstream `pdb_api_test.py` directly via `cmd/pdb-fixture-port/`. Higher effort than reusing `seed.Full`, higher confidence in input-side parity.
- **D-03** Upstream SHA pinning: `cmd/pdb-fixture-port/` records the `peeringdb/peeringdb` commit SHA in the `fixtures.go` header; `--check` flag detects drift (advisory only, not blocking for PRs).
- **D-04** Divergence registry: `docs/API.md § Known Divergences` is the permanent single source of truth (not a transient phase-level `deferred-items.md`).
- **D-05** Invalid-pdbfe-claims registry: `docs/API.md § Validation Notes` is the companion sub-section so future auditors don't re-research pdbfe's incorrect claims.
- **D-06** CI enforcement: standard `go test -race ./...` tier. No separate benchstat-on-main gate; parity tests run as regular tests on every PR.
- **D-07** Benchmark companion: `bench_test.go` covers the 3 performance-sensitive cases (2-hop traversal, `limit=0` streaming, 5001-element `__in`) with `b.Loop()` per GO-TOOL-1.

## CHANGELOG entries added

| Phase | CHANGELOG block |
|-------|-----------------|
| 67 | Pdbcompat cross-surface default ordering flipped to `(-updated, -created, -id)` |
| 68 | pdbcompat status × since matrix + `?limit=0` unlimited semantics + sync soft-delete |
| 69 | pdbcompat Unicode folding + operator coercion + `__in` large-list support + fuzz corpus |
| 70 | Cross-entity `__` traversal (Path A allowlists + Path B introspection + 2-hop cap) + unknown-field diagnostics |
| 71 | Memory-safe response paths (StreamListResponse + PDBPLUS_RESPONSE_MEMORY_LIMIT + heap-delta telemetry + ARCHITECTURE.md § Response Memory Envelope) |
| 72 | Upstream parity regression lock-in (`internal/pdbcompat/parity/` 6 test files + bench_test.go + `cmd/pdb-fixture-port/` + docs/API.md § Known Divergences + § Validation Notes) |

All under a single `[Unreleased] — v1.16` heading; coordinated-release window note states Phases 67-71 ship as a deploy bundle while Phase 72 ships independently as a CI regression gate only. v1.16 milestone-complete note flipped on 2026-04-19.

## All 25 v1.16 REQ-IDs complete

| Category | REQ-IDs | Phase | Status |
|----------|---------|-------|--------|
| ORDER | 01, 02, 03 | 67 | Complete |
| STATUS | 01, 02, 03, 04, 05 | 68 | complete (68-01/02/03) |
| LIMIT | 01, 02 | 68 | complete (68-03) |
| IN | 01, 02 | 69 | complete (69-04) |
| UNICODE | 01, 02, 03 | 69 | complete (69-04/05) |
| TRAVERSAL | 01, 02, 03, 04 | 70 | complete (70-03/04/05/06/07) |
| MEMORY | 01, 02, 03, 04 | 71 | complete (71-03/04/05/06) |
| PARITY | 01, 02 | 72 | complete (72-01..05 for 01; 72-06 for 02) |

Grep invariant: `grep -cE '(ORDER|STATUS|LIMIT|IN|UNICODE|TRAVERSAL|MEMORY|PARITY)-0[1-6] +\|.*complete' .planning/REQUIREMENTS.md` = **25**.

## Deferred items

### Scheduled follow-ups (post-v1.16 backlog)

1. **DEFER-70-06-01** — `cmd/pdb-compat-allowlist` emits wrong TargetTable for campus edges (`"campus"` instead of `"campuses"`). Preferred fix per `.planning/phases/70-cross-entity-traversal/deferred-items.md`: add `entsql.Annotation{Table: "campuses"}` to `ent/schema/campus.go`. Phase 72 locks the current HTTP 500 outcome via `TestParity_Traversal/DIVERGENCE_fac_campus_name_returns_500`; when fixed, flip the assertion and remove the divergence row.
2. **DEFER-70-verifier-01** — `fac?ixlan__ix__fac_count__gt=0` requires 3-hop via ixfac, exceeds the D-04 2-hop cap. Phase 72 locked the silent-ignore outcome as a documented divergence. Reopen only if scope widens to entity-specific `prepare_query` SQL hooks.

### Explicitly out of scope (per CONTEXT.md D-06)

- CI benchstat-on-main gate for the 3 parity benchmarks. Benchmarks run locally only; standard `go test -race` tier.
- Automated upstream-drift PR generation. Manual review only per CONTEXT.md D-03.

## Threat surface

Docs-and-test-only change set across all 6 plans. No trust boundaries moved. Security-sensitive items (TRAVERSAL-04 silent-ignore cost ceiling, LIMIT-01 unbounded + Phase 71 memory envelope) are documented in their owning phases' Known Divergences rows — Phase 72 cross-references them via parity sub-tests without changing the underlying guardrails.

## Self-Check

All plans 72-01..06 have SUMMARY.md files present. All commits verified via `git log`. docs/API.md + CLAUDE.md + CHANGELOG.md + REQUIREMENTS.md + ROADMAP.md all updated. All grep invariants pass (see 72-06-SUMMARY.md for commands).

## Self-Check: PASSED

## v1.16 milestone close

**Next action:** `/gsd-complete-milestone` — archive v1.16 (Django-compat Correctness) into `.planning/milestones/v1.16-*`:

1. Snapshot `.planning/ROADMAP.md` (v1.16 rows) → `.planning/milestones/v1.16-ROADMAP.md`
2. Snapshot `.planning/REQUIREMENTS.md` (25 REQ-IDs with traceability) → `.planning/milestones/v1.16-REQUIREMENTS.md`
3. Move `.planning/phases/{67,68,69,70,71,72}-*/` → `.planning/milestones/v1.16-phases/`
4. Update `.planning/MILESTONES.md` with v1.16 entry (ship date, phase count, REQ count, key outcomes)
5. Flip `CHANGELOG.md v1.16 [Unreleased]` → `[v1.16] — 2026-04-19`

**Ship-ready state:** Phases 67-71 ready to deploy as a coordinated bundle via `fly deploy`. Phase 72 is a CI regression gate only — no production change required.
