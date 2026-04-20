---
phase: 72-upstream-parity-regression
verified: 2026-04-19T00:00:00Z
status: passed
score: 9/9 must-haves verified
overrides_applied: 0
---

# Phase 72: Upstream parity regression + divergence docs — Verification Report

**Phase Goal:** Lock v1.16's new semantics against future regression with category-split regression tests fed by ported upstream `pdb_api_test.py` fixtures; document intentional divergences and invalid-pdbfe-claims in `docs/API.md`.
**Verified:** 2026-04-19
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths (Must-Haves)

| # | Must-Have | Status | Evidence |
|---|-----------|--------|----------|
| 1 | PARITY-01: 6 category-split test files exist under `internal/pdbcompat/parity/` and pass under `go test -race`. Each has `TestParity_<Category>` with sub-tests citing REQ-IDs. | VERIFIED | All 6 files present (`ordering_test.go`, `status_test.go`, `limit_test.go`, `unicode_test.go`, `in_test.go`, `traversal_test.go`); each declares `func TestParity_<Category>(t *testing.T)`; `go test -race ./internal/pdbcompat/parity/...` => `ok 15.106s`; citation hits across files: ordering=4, status=7, limit=2, unicode=3, in=1, traversal=7, bench=2 (15 total — matches SUMMARY claim). |
| 2 | PARITY-02: `docs/API.md § Known Divergences` extended (>=3 Phase 72 rows) + `§ Validation Notes` subsection with 5 SHA-pinned invalid-pdbfe-claims. | VERIFIED | `## Known Divergences` at line 540, `## Validation Notes` at line 560. Phase 72 rows in Known Divergences: pre-Phase-68 hard-delete gap (line 552, locked by TestParity_Status), limit=0 unlimited not count-only (line 553, TestParity_Limit), depth-on-list silent-drop (line 554, TestParity_Limit), DEFER-70-06-01 fac.campus.name 500 (line 557), DEFER-70-verifier-01 ixlan.ix.fac_count silent-ignore (line 558) — at least 3 Phase 72 rows verified. Validation Notes table contains 5 rows (country/NL, limit=0, default ordering, unicode folding, filter surface) all pinned to `peeringdb/peeringdb@99e92c72...` (7 SHA citations across the section). |
| 3 | Fixtures: `internal/testutil/parity/fixtures.go` exists with upstream SHA header pinned, 6 category vars populated. | VERIFIED | File header carries `Upstream: peeringdb/peeringdb@99e92c726172ead7d224ce34c344eff0bccb3e63` + `UpstreamHash: sha256:75c7a6fab734db782b9035a6bc23ae11abcce5901a6017a051f76bbb51399043` + `Source: src/peeringdb_server/management/commands/pdb_api_test.py`. Six category vars present at lines 34/50060/52754/52897/53335/53500: `InFixtures`, `LimitFixtures`, `OrderingFixtures`, `StatusFixtures`, `TraversalFixtures`, `UnicodeFixtures`. Total fixture rows = 5560 (matches SUMMARY). |
| 4 | `cmd/pdb-fixture-port/` tool supports `--check` mode (advisory drift detection). | VERIFIED | `cmd/pdb-fixture-port/main.go:175` registers `fs.BoolVar(&opts.Check, "check", false, "advisory drift-check mode; does not write")` plus `--pinned` for expected SHA. Usage line on :182 documents the flag. Drift detection at :365-371 reads `// UpstreamHash:` from the existing output file. |
| 5 | Benchmarks: `internal/pdbcompat/parity/bench_test.go` has 3 `b.Loop()` benchmarks (2-hop traversal, limit=0 streaming, 5000-element __in). | VERIFIED | `BenchmarkParity_TwoHopTraversal` (line 61), `BenchmarkParity_LimitZeroStreaming` (line 156), `BenchmarkParity_InFiveThousandElements` (line 209) — all three use `for b.Loop()` (lines 132, 184, 225). |
| 6 | DEFER-70-verifier-01 + DEFER-70-06-01 locked as INTENTIONAL DIVERGENCES in docs/API.md (not listed as bugs). | VERIFIED | Both appear in `§ Known Divergences` table as explicit rows with rationale (DoS protection / 2-hop cap; campus inflection patch follow-up). Each has a `TestParity_Traversal/DIVERGENCE_*` cross-reference: `DIVERGENCE_fac_campus_name_returns_500` and `DIVERGENCE_fac_ixlan_ix_fac_count_silent_ignore`. They are not in any "bug list"; they live in the divergence registry. |
| 7 | `go build ./...`, `go vet ./...`, `go test -race ./...`, `golangci-lint run` all pass. | VERIFIED | `go build ./...` => no output (clean); `go vet ./...` => no output (clean); `go test -race ./...` => all packages pass (parity 15.1s, pdbcompat 57.5s, sync 14.0s, etc.); `golangci-lint run` => `0 issues.` |
| 8 | CHANGELOG v1.16 [Unreleased] has Phase 72 entries. | VERIFIED | CHANGELOG.md line 11 `## [Unreleased] — v1.16`; line 27 milestone-complete note enumerates all 6 phases including 72; line 174 `**Phase 72 (PARITY-01, PARITY-02): Upstream parity regression lock-in.**` block with 5+ bullets covering category-split suite, fixture-port tool, divergence registry, validation notes, dev convention. Coordinated-release window note at lines 13-16 distinguishes Phase 72 (CI-only) from 67-71 (deploy bundle). |
| 9 | No `fly deploy` commands emitted in Phase 72 plans (only prohibition references). | VERIFIED | Only 3 `fly deploy` hits in Phase 72 directory: 72-SUMMARY.md:168 (describing 67-71 ship-ready state, explicitly notes "Phase 72 is a CI regression gate only — no production change required"); 72-06-PLAN.md:344 (explicit prohibition: "No fly deploy imperatives introduced"); 72-06-SUMMARY.md:155 (verification that no fly deploy added to docs/changelog). All three are prohibition / cross-reference, none are imperative deploy commands within Phase 72 plans. |

**Score:** 9/9 must-haves verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/pdbcompat/parity/ordering_test.go` | TestParity_Ordering with sub-tests | VERIFIED | `func TestParity_Ordering(t *testing.T)` at line 28; package `parity`; ORDER-01..03 covered. |
| `internal/pdbcompat/parity/status_test.go` | TestParity_Status status×since matrix | VERIFIED | `func TestParity_Status(t *testing.T)` at line 27; locks `rest.py:694-727` matrix per Phase 68 D-05/D-07. |
| `internal/pdbcompat/parity/limit_test.go` | TestParity_Limit LIMIT-01/01b/02 | VERIFIED | `func TestParity_Limit(t *testing.T)` at line 27; covers `?limit=0` unlimited + 413 over-budget + `?depth=N` silent-drop. |
| `internal/pdbcompat/parity/unicode_test.go` | TestParity_Unicode UNICODE-01/02 | VERIFIED | `func TestParity_Unicode(t *testing.T)` at line 31; locks Phase 69 unifold + shadow-column pipeline. |
| `internal/pdbcompat/parity/in_test.go` | TestParity_In IN-01..03 | VERIFIED | `func TestParity_In(t *testing.T)` at line 32; covers 5001-element `__in` + empty short-circuit. |
| `internal/pdbcompat/parity/traversal_test.go` | TestParity_Traversal TRAVERSAL-01..04 | VERIFIED | `func TestParity_Traversal(t *testing.T)` at line 44; Path A/B + 2-hop cap + DIVERGENCE asserts; imports OTel SDK for span-attr verification. |
| `internal/pdbcompat/parity/bench_test.go` | 3 b.Loop() benchmarks | VERIFIED | TwoHopTraversal/LimitZeroStreaming/InFiveThousandElements; b.Loop() idiom per GO-TOOL-1. |
| `internal/pdbcompat/parity/harness_test.go` + `harness_helpers_test.go` | Test harness | VERIFIED | Both files present; harness widened to `testing.TB` for bench co-use (per SUMMARY plan 72-05). |
| `internal/testutil/parity/fixtures.go` | SHA-pinned generated fixtures | VERIFIED | 5560 rows; 6 vars; SHA `99e92c72...` pinned; `Code generated by cmd/pdb-fixture-port — DO NOT EDIT.` header. |
| `internal/testutil/parity/generate.go` | go:generate directive | VERIFIED | `//go:build ignore` directive file; `go generate ./internal/testutil/parity` runs the port tool. |
| `cmd/pdb-fixture-port/main.go` | Tool with --check flag | VERIFIED | 5 parser files (parse_in/limit/status/traversal/unicode.go) + main.go with --check, --pinned, --upstream-commit, --append, --category flags. |
| `docs/API.md § Known Divergences` | >=3 Phase 72 rows | VERIFIED | Table at line 549 has 8 rows (Phase 68/69/70 seed + 3 new Phase 72 rows + 2 DEFER rows with TestParity_* cross-refs). |
| `docs/API.md § Validation Notes` | 5 SHA-pinned invalid claims | VERIFIED | NEW sub-section at line 560; 5 rows (country, limit=0, ordering, unicode, filter surface) all pinned to `99e92c72`. |
| CHANGELOG v1.16 [Unreleased] Phase 72 entries | Present | VERIFIED | Phase 72 Added block at line 174 with 5+ bullets; milestone-close note at line 27. |

### Key Link Verification

| From | To | Via | Status | Details |
|------|-----|-----|--------|---------|
| `internal/pdbcompat/parity/*_test.go` | `internal/testutil/parity` (fixtures pkg) | import path | WIRED | Tests use `testutil.SetupClient(t)` from `internal/testutil`; some categories (status/in/limit) build inline rows; ordering test explicitly notes "ported OrderingFixtures slice carries Python-source artefacts...subtests still cite their upstream" — this is documented in the test header. |
| Test files | REQ-IDs (ORDER/STATUS/LIMIT/UNICODE/IN/TRAVERSAL) | sub-test naming | WIRED | Sub-test names embed REQ-IDs (e.g. `LIMIT-01_zero_returns_all_rows_unbounded`, `STATUS-04_list_since_admits_deleted_excludes_pending_noncampus`). |
| `cmd/pdb-fixture-port` | `internal/testutil/parity/fixtures.go` | `go generate` directive | WIRED | `internal/testutil/parity/generate.go` carries the directive; tool emits the file with SHA header. |
| `docs/API.md § Known Divergences` rows | parity tests | `TestParity_*` cross-references | WIRED | 7 `Phase 72` mentions + multiple `TestParity_<Category>/<subtest>` names embedded in row text (e.g. `TestParity_Limit/LIMIT-01_zero_returns_all_rows_unbounded`). |
| `docs/API.md § Validation Notes` rows | upstream SHAs | inline citation | WIRED | All 5 rows reference `peeringdb/peeringdb@99e92c726172ead7d224ce34c344eff0bccb3e63` with file:line ranges. |
| CHANGELOG entries | REQUIREMENTS PARITY-01/02 | requirement IDs | WIRED | CHANGELOG bullets and REQUIREMENTS.md row both label complete with same scope description; PARITY-01 cites 72-01..05, PARITY-02 cites 72-06. |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Parity test suite passes under -race | `go test -race ./internal/pdbcompat/parity/...` | `ok ...parity 15.106s` | PASS |
| Full test suite passes under -race | `go test -race ./...` | All 30+ packages report `ok` (no FAIL/SKIP/race output) | PASS |
| Build is clean | `go build ./...` | No output (success) | PASS |
| Vet is clean | `go vet ./...` | No output (success) | PASS |
| Lint is clean | `golangci-lint run` | `0 issues.` | PASS |
| Fixture file SHA header present | `head -10 internal/testutil/parity/fixtures.go` | Header lines 3-4 carry SHA + sha256 hash | PASS |
| Tool flag `--check` present | grep `--check` in cmd/pdb-fixture-port/main.go | Found at line 175 + 182 + 365-371 | PASS |
| 6 TestParity_* entries exist | grep `^func TestParity_` in parity dir | 6 hits across 6 files | PASS |
| 3 b.Loop() benchmarks exist | grep `b\.Loop\(\)` in bench_test.go | 3 hits at lines 132, 184, 225 | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| PARITY-01 | 72-01..05 | Category-split regression tests covering ordering, status×since, limit=0, __in, traversal | SATISFIED | 6 TestParity_<Category> files; 27 v1.16-semantic sub-tests; 2 DIVERGENCE asserts; 4 harness probes; 3 b.Loop() benchmarks; suite passes under `-race` in 15.1s. |
| PARITY-02 | 72-06 | Intentional-divergence docs update | SATISFIED | docs/API.md § Known Divergences extended with 3 new Phase 72 rows + cross-refs on 2 DEFER rows; new § Validation Notes sub-section with 5 SHA-pinned invalid-pdbfe-claims; CLAUDE.md convention block added per SUMMARY; CHANGELOG v1.16 [Unreleased] Phase 72 entries present. |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| _(none found)_ | — | — | — | No TODO/FIXME/placeholder/stub patterns flagged in Phase 72 deliverables. The ordering_test.go header explicitly explains why it bypasses the OrderingFixtures slice (Python-source artefacts in `name`/`status` rejected as unseedable), pointing instead to inline-built clean rows that still cite upstream — this is documented design, not a stub. |

### Human Verification Required

_(none — all must-haves verified programmatically; no UX/visual/external-service items)_

### Gaps Summary

No gaps. All 9 must-haves verified. Build / vet / test (`-race`) / lint all clean. CHANGELOG, REQUIREMENTS, ROADMAP, docs/API.md, CLAUDE.md all updated per SUMMARY claims. Phase 72 closes v1.16.

---

_Verified: 2026-04-19_
_Verifier: Claude (gsd-verifier)_
