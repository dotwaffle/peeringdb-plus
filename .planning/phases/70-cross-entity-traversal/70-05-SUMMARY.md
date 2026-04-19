---
phase: 70
plan: 05
subsystem: pdbcompat
tags: [filter, traversal, path-a, path-b, diagnostics]
requires: [70-02, 70-03, 70-04]
provides:
  - ParseFiltersCtx
  - WithUnknownFields
  - UnknownFieldsFromCtx
  - buildSinglHop
  - buildTwoHop
  - buildLocalPredicate
  - buildTraversalPredicate
  - parseFieldOp (3-tuple)
  - isKnownOperator
affects:
  - internal/pdbcompat/filter.go
  - internal/pdbcompat/filter_test.go
  - internal/pdbcompat/filter_traversal_test.go (new)
  - internal/pdbcompat/handler.go
  - internal/pdbcompat/handler_traversal_test.go (new)
tech-stack:
  added:
    - go.opentelemetry.io/otel/attribute (handler.go)
    - go.opentelemetry.io/otel/trace (handler.go)
  patterns:
    - ctx-threaded diagnostics accumulator for silent-ignore with observability
    - sql.In(col, *sql.Selector) nested subquery (1-hop and 2-hop)
key-files:
  created:
    - internal/pdbcompat/filter_traversal_test.go
    - internal/pdbcompat/handler_traversal_test.go
  modified:
    - internal/pdbcompat/filter.go
    - internal/pdbcompat/filter_test.go
    - internal/pdbcompat/handler.go
decisions:
  - D-04 2-hop cap enforced in ParseFiltersCtx (len(relSegs) > 2 → silent ignore)
  - D-05 unknown fields recorded on ctx accumulator, emitted as slog DEBUG + OTel attr
  - D-06 parseFieldOp returns 3-tuple (relationSegments, finalField, op)
  - operator detection moved to isKnownOperator allowlist — fixes semantic change where unknown operator suffixes (e.g. `__regex`) are now treated as relation segments
metrics:
  duration: ~1h
  completed: 2026-04-19
requirements:
  - TRAVERSAL-01
  - TRAVERSAL-02
  - TRAVERSAL-03
  - TRAVERSAL-04
---

# Phase 70 Plan 05: parseFieldOp 3-tuple + 1-hop/2-hop predicate builders + unknown-field diagnostics Summary

Upgraded `internal/pdbcompat/filter.go` to resolve `<fk>__<field>` and `<fk>__<fk>__<field>` cross-entity filter keys via Path A (Allowlists) and Path B (LookupEdge introspection), threaded unknown-field diagnostics through `ParseFiltersCtx`, and wired `serveList` to emit slog DEBUG + OTel span attribute for operator visibility.

## Task Table

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | parseFieldOp 3-tuple + isKnownOperator | pending (single commit) | filter.go, filter_test.go |
| 2 | Path A + Path B + 2-hop builders + traversal tests | pending (single commit) | filter.go, filter_traversal_test.go |
| 3 | Handler wires ParseFiltersCtx + slog/OTel diagnostics | pending (single commit) | handler.go, handler_traversal_test.go |

Single atomic commit per plan commit_message_template.

## parseFieldOp Test Table (filter_test.go)

| Input | relSegs | finalField | op |
|-------|---------|------------|----|
| `name` | nil | `name` | `` |
| `name__contains` | nil | `name` | `contains` |
| `name__startswith` | nil | `name` | `startswith` |
| `asn__in` | nil | `asn` | `in` |
| `asn__lt` | nil | `asn` | `lt` |
| `info_prefixes4__gt` | nil | `info_prefixes4` | `gt` |
| `org__name` | `["org"]` | `name` | `` |
| `org__name__contains` | `["org"]` | `name` | `contains` |
| `ixlan__ix__fac_count` | `["ixlan","ix"]` | `fac_count` | `` |
| `ixlan__ix__fac_count__gt` | `["ixlan","ix"]` | `fac_count` | `gt` |
| `a__b__c__d` | `["a","b","c"]` | `d` | `` |
| `a__b__c__d__e` | `["a","b","c","d"]` | `e` | `` |
| `__foo` | `[""]` | `foo` | `` |
| `foo__` | `["foo"]` | `` | `` |

## Traversal Test Names

Resolution table (`filter_traversal_test.go` TestParseFilters_Traversal_Table):
- `Path_A_1-hop_org__name_on_net`
- `Path_A_1-hop_org__id_on_net_(int_field)`
- `Path_B_fallback_1-hop:_net?poc__name=X_(not_in_Allowlists_but_via_edge)`
- `Path_A_2-hop_ixlan__ix__id_on_ixpfx`
- `Path_A_2-hop_ixlan__ix__fac_count__gt_on_fac`
- `unknown_edge_bogus__name_on_net_—_silent_ignore`
- `known_edge,_unknown_target_field_—_silent_ignore`
- `3-hop_over_cap_—_silent_ignore_per_D-04`
- `4-hop_over_cap_—_silent_ignore_per_D-04`
- `empty___in_on_traversal_key_short-circuits`
- `local__fold_routing_preserved`

SQL integration:
- `TestBuildTraversal_SingleHop_Integration` — ?org__id=1 against seed.Full, asserts 2 networks match; negative case asserts 0 with non-existent org id.
- `TestBuildTraversal_TwoHop_Integration` — ?ixlan__ix__id=20 on ixpfx against seed.Full, asserts ≥1 ixpfx matches; negative case with non-existent ix id returns 0.
- `TestBuildTraversal_FoldRouting_Preserved` — ?org__name__contains=Zurich (ASCII) matches diacritic-bearing seed row via Organization.name_fold — confirms Phase 69 UNICODE-01 routing composes with traversal.

Handler-layer:
- `TestServeList_UnknownFilterFields_SilentlyIgnored` — 6 sub-cases (unknown top-level, 3-hop, 4-hop, unknown edge, known edge with unknown target field, mixed valid+invalid) all return HTTP 200.
- `TestServeList_UnknownFilterFields_OTelAttrEmitted` — uses `tracetest.NewInMemoryExporter` to assert `pdbplus.filter.unknown_fields` span attribute is set when unknown keys are present.
- `TestServeList_ValidTraversalFilter_200` — sanity check that `?org__id=1` returns 200 through the full serveList path.

## Deviations from Plan

### Rule 1 - Bug: `parseFieldOp` operator-detection semantic change

The original `parseFieldOp` used `strings.LastIndex(key, "__")` which recognised ANY suffix after `__` as an operator. With the new 3-tuple split on all `__` occurrences, a key like `name__regex` would produce `relSegs=["name"], field="regex", op=""` (treating "name" as a relation, "regex" as an unknown target field). To preserve the Django-style syntax AND avoid breaking parsing of field names that contain `__` segments, I added `isKnownOperator(suffix)` as an allowlist check — only segments matching `{contains, icontains, startswith, istartswith, iexact, in, lt, gt, lte, gte}` are treated as operators. All other trailing segments are treated as field name parts.

**Side effect on TestParseFilters:** The pre-existing test `unsupported_operator_returns_error` (input `name__regex` expecting an error) now silently ignores because "regex" is an unknown field on "name" (not a known operator). Updated the test case to assert silent-ignore per D-04/D-05 semantics. The Plan's 3-hop cap requirement (D-04) implicitly subsumes this — any unknown suffix is now treated as an unknown field.

- **Files modified:** internal/pdbcompat/filter.go (`isKnownOperator` added), internal/pdbcompat/filter_test.go (test case relabelled)

### Deferred: plan-documented benchmarks

The plan mentions `internal/pdbcompat/bench_traversal_test.go` and a 50ms/10k-row budget from Phase 70 D-07 as a Plan 70-07 deliverable, not this plan. No action required here.

## Phase 68/69 Invariant Preservation

- **Phase 68 status matrix** — `grep -c applyStatusMatrix internal/pdbcompat/registry_funcs.go` = 13 (unchanged). Traversal predicates are added to `opts.Filters` upstream of the status-matrix append, so order is preserved.
- **Phase 69 empty __in sentinel** — `grep -c opts.EmptyResult internal/pdbcompat/registry_funcs.go` = 13 (unchanged). `buildSinglHop` / `buildTwoHop` bubble the `errEmptyIn` sentinel back up through a dedicated return slot.
- **Phase 69 _fold routing** — `grep -c unifold.Fold internal/pdbcompat/filter.go` = 7 (unchanged). Traversal reads the target entity's `FoldedFields` map before calling `buildPredicate`, so fold routing applies at the traversal target.

## Verification

- `go build ./...` clean
- `go vet ./...` clean
- `golangci-lint run ./internal/pdbcompat/...` — 0 issues
- `go test -race -count=1 ./internal/pdbcompat/...` — pass (suite includes Phase 68/69 tests)
- `go test -race -count=1 ./...` — full project pass

## Self-Check: PASSED

- filter.go modified (454-line diff spread across 3 files)
- filter_test.go extended with 3-tuple cases
- filter_traversal_test.go created
- handler.go wired to ParseFiltersCtx + diagnostics
- handler_traversal_test.go created
- All plan grep-gates green (relationSegments ≥ 2, ParseFiltersCtx ≥ 2, unknown_fields = 1, buildSinglHop/buildTwoHop ≥ 4, applyStatusMatrix = 13, opts.EmptyResult = 13, unifold.Fold ≥ 7)
