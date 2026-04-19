# Phase 70 Deferred Items

## DEFER-70-06-01 — allowlist_gen.go emits wrong TargetTable for campus edges

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
