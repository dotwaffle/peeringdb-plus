# Phase 70 Deferred Items

## DEFER-70-06-01 — allowlist_gen.go emits wrong TargetTable for campus edges

**Status:** CLOSED — fixed in v1.18.0 Phase 73 (2026-04-26). `entsql.Annotation{Table: "campuses"}` added to `ent/schema/campus_annotations.go` (sibling-file mixin per Phase 73 CONTEXT.md D-01 + CLAUDE.md § Schema & Visibility sibling-file convention). `internal/pdbcompat/allowlist_gen.go` lines 174 + 212 now emit `TargetTable: "campuses"`. The traversal E2E sub-test `path_a_1hop_fac_campus_name` (in `internal/pdbcompat/traversal_e2e_test.go`) and the parity sub-test `TRAVERSAL-05_path_a_1hop_fac_campus_name` (replacing the prior `DIVERGENCE_fac_campus_name_returns_500` canary in `internal/pdbcompat/parity/traversal_test.go`) lock the post-fix HTTP 200 contract. See `.planning/phases/73-code-defect-fixes/73-01-SUMMARY.md` for execution details.

**Discovered by:** Plan 70-06 (traversal_e2e_test.go `path_a_1hop_fac_campus_name`)

**Symptom:** `GET /api/fac?campus__name=TestCampus1` returns HTTP 500 with
`SQL logic error: no such table: campus (1)`.

**Root cause:** `cmd/pdb-compat-allowlist/main.go` calls `e.Type.Table()` on the
Campus ent type via `entc.LoadGraph`. The returned table name is `"campus"` —
the entc.LoadGraph path does not apply the `fixCampusInflection` patch that the
ent runtime codegen applies (see `ent/entc.go`). The runtime migrate schema and
ent predicate layer correctly pluralise the table as `"campuses"` (see
`ent/migrate/schema.go:33 Name: "campuses"` and `ent/campus/campus.go:55 Table = "campuses"`),
but `internal/pdbcompat/allowlist_gen.go` lines 162, 163, 174, 212 emit
`TargetTable: "campus"` for all incoming edges targeting Campus.

The outgoing edges FROM Campus are correct (lines 161-167 of allowlist_gen.go:
`TargetTable: "facilities"` and `TargetTable: "organizations"`) because those
resolve `.Table()` on non-Campus types.

**Affected traversal keys:**

- `fac?campus__name=<X>` — Path A Allowlist hit
- Any future `<entity>?campus__<field>` query via Path A or Path B
- `org` outgoing edge `campuses` — Target table should be `campuses`, is `campus`

**Scope:** Plan 70-06 is test-only per plan frontmatter ("Zero production code
edits in pdbcompat"). The fix is in `cmd/pdb-compat-allowlist/main.go` — either
(a) call `inflect.AddIrregular("campus", "campuses")` + `AddSingular` before
`entc.LoadGraph`, matching `ent/entc.go:fixCampusInflection`; or (b) patch the
generated `allowlist_gen.go` post-processing step to rewrite `"campus"` →
`"campuses"`; or (c) add an `entsql.Annotation{Table: "campuses"}` to
`ent/schema/campus.go` so the table name is explicit and survives any codegen
path.

Option (c) is the cleanest — it eliminates the dependency on inflection heuristics
entirely, mirroring v1.15 Phase 63's preference for explicit schema annotations
over implicit convention.

**Workaround in Plan 70-06:** The E2E matrix test substitutes a different Path A
1-hop case (`/api/fac?org__name=TestOrg1`) for the campus case. The plan's
TRAVERSAL-04 coverage is preserved by the 4 other 1-hop subtests (net, fac, ix,
carrier via org__name) plus the 2 upstream parity + 1 Path B cases.

**Recommended next step:** Schedule a follow-up plan (e.g. 70-09 or a quick
task) applying fix option (c) — add `entsql.Annotation` to `ent/schema/campus.go`,
rerun `go generate ./...`, extend `TestTraversal_E2E_Matrix` with a
`path_a_1hop_fac_campus_name` subtest asserting `[8001]`.

## DEFER-70-verifier-01 — `fac?ixlan__ix__fac_count__gt=0` requires 3-hop-via-ixfac or entity-specific prepare_query

**Discovered by:** Phase 70 verifier (`70-VERIFICATION.md` Gap 1, 2026-04-19)

**Owner:** Phase 72 (parity regression test will lock the silent-ignore
semantics OR reopen if scope widens).

**Symptom:** The TRAVERSAL-03-cited canonical upstream case
`GET /api/fac?ixlan__ix__fac_count__gt=0` (upstream citation
`pdb_api_test.py:2340, 2348`) is silently ignored by the generic 2-hop
mechanism rather than resolved. The E2E case
`upstream_2340_fac_ixlan_ix_fac_count_gt` at
`internal/pdbcompat/traversal_e2e_test.go:161-173` explicitly asserts
the silent-ignore outcome (returns all live facs, unfiltered). The
generic 2-hop mechanism does work for entity pairs with direct edges:
`ixpfx?ixlan__ix__id=20` resolves end-to-end (covered by
`TestBuildTraversal_TwoHop_Integration`).

**Root cause:** `fac` has no direct `ixlan` edge in the ent schema —
`ent/schema/facility.go` Edges list (lines 223-234) does not declare
one, because in the PeeringDB data model `ixlan` belongs to `ix`
(InternetExchange), not to `fac`. The Path A allowlist entry
`Allowlists["fac"].Via["ixlan"] = ["ix__fac_count"]` emitted into
`internal/pdbcompat/allowlist_gen.go` is therefore effectively dead
code — it references an edge that does not exist in the Path B `Edges`
map, so `buildTwoHop` returns `(nil, false, false, nil)` and the filter
key falls through to the unknown-field silent-ignore path.

Upstream Django reaches this via a `prepare_query`-specific SQL
construction that joins through `ixfac` (IXFacility — the
many-to-many bridge between `ix` and `fac`), which is a 3-hop walk
(`fac` → `ixfac` → `ix` → `fac_count`) and therefore exceeds the hard
2-hop cap decided in Phase 70 D-04. The generic 2-hop mechanism cannot
reach this without either (a) relaxing the 2-hop cap, which re-opens
the cost-ceiling concerns D-04 was designed to contain, or (b) adding a
custom per-serializer hook that emits entity-specific SQL for this
single case, which is the mechanism upstream uses but which doesn't fit
the D-01/D-04 generic model cleanly.

**Scope:** Documentation-only in Phase 70 Verifier fix (2026-04-19). The
generic 2-hop mechanism ships working; the specific upstream citation
case remains silent-ignored with a documented divergence entry in
`docs/API.md` § Known Divergences and a one-line Phase 70 known-issue
note in `CHANGELOG.md`.

**Reasoning:** upstream uses a bespoke per-serializer `prepare_query`
that exceeds our 2-hop cap; the generic mechanism cannot reach it
without a custom hook. The risk of relaxing the cap (unbounded
Cartesian-product joins in SQLite under replica memory pressure)
outweighs the payoff of paving this single upstream citation case —
the TRAVERSAL-03 requirement is satisfied in the generic sense
(2-hop traversal works for entity pairs with direct edges; test
coverage via `ixpfx→ixlan→ix`).

**Next step:** Phase 72 (upstream parity regression) either:

- Lock the silent-ignore semantics for `fac?ixlan__ix__fac_count__gt=0`
  as a documented divergence — add an upstream parity test that
  asserts the unfiltered-facs response and cross-references this item.
- OR reopen this item if the scope widens to a bespoke per-serializer
  hook model (entity-specific SQL for high-value upstream citation
  cases beyond the generic 2-hop mechanism).
