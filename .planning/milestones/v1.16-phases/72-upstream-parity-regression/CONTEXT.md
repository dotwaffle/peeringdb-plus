---
phase: 72
slug: upstream-parity-regression
milestone: v1.16
status: context-locked
has_context: true
locked_at: 2026-04-19
---

# Phase 72 Context: Upstream parity regression + divergence docs

## Goal

Lock v1.16's new semantics against future regression with category-split regression tests fed by ported upstream `pdb_api_test.py` fixtures; document intentional divergences and invalid-pdbfe-claims in `docs/API.md` so future auditors can distinguish regression from design.

## Requirements

- **PARITY-01** — category-split regression tests covering ordering, status×since, limit=0, __in, traversal
- **PARITY-02** — intentional-divergence docs update

## Locked decisions

- **D-01 — Test file layout: category-split**. New files under `internal/pdbcompat/parity/`:
  - `ordering_test.go` — ORDER-01..03 assertions
  - `status_test.go` — STATUS-01..05 matrix
  - `limit_test.go` — LIMIT-01..02 (+ budget-breach interaction with Phase 71)
  - `unicode_test.go` — UNICODE-01..02 (fuzz stays in `filter_fuzz_test.go` from Phase 69)
  - `in_test.go` — IN-01..02
  - `traversal_test.go` — TRAVERSAL-01..04 (1-hop + 2-hop + unknown-field diagnostics)
  Each file has a `TestParity_<Category>` entry test with sub-tests. `t.Parallel()` used liberally.
- **D-02 — Fixture source: port upstream `pdb_api_test.py` fixtures directly**. New tool `cmd/pdb-fixture-port/` reads `peeringdb_server/management/commands/pdb_api_test.py` locally (or via `gh api`) and extracts Django fixture definitions into Go `internal/testutil/parity/fixtures.go`. Tool runs offline once per upstream sync (approximately milestone boundary, not per-PR). Output committed; regenerate via `go generate ./internal/testutil/parity`. This is higher-effort than reusing `seed.Full` but gives behavioural-exact parity on the input side — the user chose this over my recommendation for higher confidence.
- **D-03 — Upstream SHA pinning**: `cmd/pdb-fixture-port/` records the `peeringdb/peeringdb` commit SHA it ported from in a header comment of `fixtures.go`. Upstream drift detection: a CI job runs `cmd/pdb-fixture-port/ --check` quarterly (manual or scheduled) and fails if current upstream `pdb_api_test.py` hash differs from pinned SHA. Not blocking for PR merges — advisory alert.
- **D-04 — Divergence registry: `docs/API.md` § Known Divergences**. New table with columns: `Behaviour | Upstream | PeeringDB Plus | Reason | Since Version`. Initial entries:
  - `status=deleted` pre-Phase-68 rows — hard-deleted, not tombstoned; `?status=deleted + since>0` returns only rows marked deleted after Phase 68 shipped
  - Shadow `_fold` columns — internal implementation detail, not a behavioural divergence
  - Any others that emerge during Phase 67-71 implementation
- **D-05 — Invalid-pdbfe-claims registry: `docs/API.md` § Validation Notes**. Separate sub-section documenting:
  1. `net?country=NL` does NOT work on upstream (country is on `org`, not `net`) — cite `peeringdb/peeringdb@99e92c72 serializers.py:2938-2992`
  2. `limit=0` is unlimited, NOT count-only — cite `rest.py:494-497`
  3. Default ordering is `(-updated, -created)`, NOT `id ASC` — cite `django-handleref/models.py:95-101`
  4. Unicode folding is Python `unidecode`, NOT MySQL collation — cite `rest.py:576`
  5. Filter surface is `prepare_query` + auto-`queryable_relations`, NOT a DRF `filterset_class` — cite `serializers.py:754-780`
  Each with `peeringdb/peeringdb` SHA ref. Future auditors reading our codebase against pdbfe's gotchas doc don't re-research.
- **D-06 — CI enforcement: standard tier**. Parity tests run via `go test -race ./...` on every PR — same gate as any other test. No separate job or status check. Matches GO-CI-1 and keeps the workflow simple.
- **D-07 — Benchmark companion**: `internal/pdbcompat/parity/bench_test.go` covers the performance-sensitive cases (2-hop traversal, `limit=0` streaming, 5000-element `__in`). `b.Loop()` style per GO-TOOL-1 conventions from v1.9 Phase 46. Published to CI for benchstat comparison on main branch.

## Out of scope

- Live integration tests against `beta.peeringdb.com` — existing `-peeringdb-live` flag is conformance-scoped, not parity-scoped
- Web UI / GraphQL / entrest parity — v1.16 is pdbcompat-first
- Automated upstream-drift PR generation — manual review only per D-03

## Dependencies

- **Depends on**: Phases 67, 68, 69, 70, 71 (all semantics must be implemented before parity tests can lock them)
- **Enables**: Ships v1.16. Archive in `.planning/milestones/v1.16-*` via `/gsd-complete-milestone`.

## Plan hints for executor

- Touchpoints: new `internal/pdbcompat/parity/` directory with ~6 test files, new `cmd/pdb-fixture-port/` tool, new `internal/testutil/parity/fixtures.go`, `docs/API.md` (two new sections), `.github/workflows/ci.yml` (no changes — existing `go test -race` picks up the new tests).
- `cmd/pdb-fixture-port/` reads Python fixture blocks (YAML-embedded-in-test-file) and emits Go struct literals that `seed.Full`-style initialisation can consume. Initial proof via one category (ordering); expand to others as plans progress.
- Keep parity tests' fixture setup isolated from `seed.Full` to avoid cross-test contamination — each parity test gets its own ent client via `testutil.SetupClient(t)`.

## References

- ROADMAP.md Phase 72
- REQUIREMENTS.md PARITY-01..02
- Upstream: `peeringdb_server/management/commands/pdb_api_test.py` (6537 lines — the ground-truth corpus)
- CLAUDE.md § Testing conventions
- v1.2 Phase 9 (golden file tests) and v1.10 Phase 48 (fuzz corpus) for testing-pattern precedent
