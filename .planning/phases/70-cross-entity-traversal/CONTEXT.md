---
phase: 70
slug: cross-entity-traversal
milestone: v1.16
status: context-locked
has_context: true
locked_at: 2026-04-19
---

# Phase 70 Context: Cross-entity `__` traversal (Path A + Path B + 2-hop)

## Goal

pdbcompat resolves `<fk>__<field>` and `<fk>__<fk>__<field>` filter paths the way upstream does: per-serializer `prepare_query` allowlists (Path A) AND automatic relation introspection minus a `FILTER_EXCLUDE` list (Path B), capped at 2 hops.

## Requirements

- **TRAVERSAL-01** — Path A allowlists for all 13 entity types
- **TRAVERSAL-02** — Path B auto-exposed `<fk>__<field>` via ent introspection
- **TRAVERSAL-03** — 2-hop traversal works (`fac?ixlan__ix__fac_count__gt=0`)
- **TRAVERSAL-04** — Unknown fields silently ignored (no HTTP 400)

## Locked decisions

_D-02 amended 2026-04-19 during plan-checker review: codegen-time static emission replaces runtime client.Schema.Tables walk. Technical rationale in Plan 70-04._

- **D-01 — Path A representation: codegen from ent schema annotations**. Introduce a new annotation `pdbcompat.WithPrepareQueryAllow(fields ...string)` consumed by a new codegen target `cmd/pdb-compat-allowlist/` that reads ent schema graph and emits `internal/pdbcompat/allowlist_gen.go`. Each entity's `Allowlist()` function returns `AllowlistEntry{Direct: []string{...}, Via: map[string][]string{...}}`. Single source of truth is the ent schema file. `go generate ./...` pipeline wires this in after ent codegen, before buf codegen.
- **D-02 — Path B mechanism: codegen-emitted static edge map**. `cmd/pdb-compat-allowlist/` (introduced in D-01) walks the ent schema graph at `go generate` time and emits `internal/pdbcompat/allowlist_gen.go` with a frozen `Edges map[string][]EdgeMetadata` alongside the per-entity `Allowlist()` functions. `internal/pdbcompat/introspect.go` exposes `LookupEdge(entity, name)` / `ResolveEdges(entity)` / `TargetFields(entity, edge)` reading the generated map — no runtime walk, no sync.Once, no init-order coupling. Freshness is enforced by the same `go generate ./...` drift-check CI gate as the rest of ent codegen. Matches the v1.15 Phase 63 codegen precedent.
- **D-03 — `FILTER_EXCLUDE` as ent annotation**: New annotation `pdbcompat.WithFilterExcludeFromTraversal()` on specific edges (initial set: `network.social_media` JSON field, ent-generated Relay `id` aliases). Codegen emits into same `allowlist_gen.go`. Matches upstream `serializers.py:128-157` pattern 1:1.
- **D-04 — Traversal depth cap: 2 hops exactly**. Filter keys with >2 `__`-separated relationship segments (e.g. `a__b__c__d`) are silently ignored per D-05 (same as unknown fields). Keys with exactly 2 hops are resolved via recursive Path B lookup. Cap hard-enforced in parser, not in edge walker, so hostile inputs can't cause recursion blow-up.
- **D-05 — Unknown-filter diagnostics**: Silent-ignore (per TRAVERSAL-04) PLUS `slog.Debug("unknown filter field", slog.String("field", k), slog.String("endpoint", r.URL.Path))` AND OTel span attribute `pdbplus.filter.unknown_fields` as a comma-separated string. Zero behaviour change for clients; operators can query Grafana/logs. Not LOG-level (too noisy) — DEBUG only, gated by existing OTEL_LOG_LEVEL/slog handler level.
- **D-06 — Where `__` segment splitting happens**: `parseFieldOp` in `internal/pdbcompat/filter.go` already splits on `__`. Extend it to return `(relationSegments []string, finalField string, op string)`. Max relationSegments length = 2. Zero-length final field or malformed splits fall through to the unknown-field handler.
- **D-07 — Query plan cost safeguards**: For 2-hop queries, `EXPLAIN QUERY PLAN` is not automatically consulted at request time (too expensive per-request). Instead: benchmarks in `internal/pdbcompat/bench_traversal_test.go` verify no query exceeds 50ms at 10k-row scale; CI gates on regression. If a query pattern regresses, add an ent `Index` annotation on the FK column.

## Out of scope

- 3+ hop traversal — capped at 2 per D-04
- Cross-entity filter parsing in grpcserver / entrest / GraphQL — out of v1.16 scope per REQUIREMENTS.md "Out of Scope" section
- Cross-entity `__` traversal on `_fold` columns from Phase 69 — initial scope is ASCII/non-folded traversal only. If users complain about `?org__city__contains=Zürich`, expand in a follow-up.

## Dependencies

- **Depends on**: Phase 69 (shares `internal/pdbcompat/filter.go`; operator coercion and empty-`__in` short-circuit applied uniformly to traversal filters too)
- **Enables**: Phase 71 (memory budget accounting must consider 2-hop join row counts)

## Plan hints for executor

- Touchpoints: new `internal/pdbcompat/introspect.go` (Path B walker), new `cmd/pdb-compat-allowlist/` (codegen tool), new `pdbcompat.WithPrepareQueryAllow` annotation type, `internal/pdbcompat/filter.go` (parser extension), 13 ent schema edits (annotations), `ent/generate.go` or `Makefile` (wire new codegen step).
- Mirror upstream: compile a checklist from `peeringdb_server/serializers.py:1823, 2244, 2361, 2423, 2573, 2732, 2947, 2995, 3315, 3451, 3622, 3925, 4041` (all 13 `get_relation_filters(...)` lists) → translate verbatim into ent annotations.
- Benchmarks: add 2-hop test case (`fac?ixlan__ix__fac_count__gt=0`) at seed.Full scale; ensure <50ms.

## References

- ROADMAP.md Phase 70
- REQUIREMENTS.md TRAVERSAL-01..04
- Upstream: `peeringdb_server/rest.py:544-662` (filter-param loop), `peeringdb_server/serializers.py:128-157` (`FILTER_EXCLUDE`), `serializers.py:419-431` (`queryable_field_xl`), `serializers.py:493-502` (relation walker), `serializers.py:754-780` (`queryable_relations`), `pdb_api_test.py:2340, 2348, 4234, 4241, 4249, 5047, 5081` (2-hop + 1-hop test patterns)
- ent introspection: codegen-time gen.Graph walk via `entc.LoadGraph` (see `cmd/pdb-compat-allowlist/main.go`); no runtime introspection API needed per D-02.
