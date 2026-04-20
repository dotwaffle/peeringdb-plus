---
phase: 70-cross-entity-traversal
reviewed: 2026-04-19T20:10:12Z
depth: standard
files_reviewed: 25
files_reviewed_list:
  - internal/pdbcompat/annotations.go
  - internal/pdbcompat/schemaannot/schemaannot.go
  - internal/pdbcompat/introspect.go
  - internal/pdbcompat/filter.go
  - internal/pdbcompat/handler.go
  - cmd/pdb-compat-allowlist/main.go
  - internal/pdbcompat/allowlist_gen.go
  - internal/pdbcompat/introspect_test.go
  - internal/pdbcompat/filter_traversal_test.go
  - internal/pdbcompat/traversal_e2e_test.go
  - internal/pdbcompat/handler_traversal_test.go
  - ent/schema/network.go
  - ent/schema/facility.go
  - ent/schema/internetexchange.go
  - ent/schema/organization.go
  - ent/schema/campus.go
  - ent/schema/carrier.go
  - ent/schema/ixlan.go
  - ent/schema/ixprefix.go
  - ent/schema/ixfacility.go
  - ent/schema/networkfacility.go
  - ent/schema/networkixlan.go
  - ent/schema/carrierfacility.go
  - ent/schema/poc.go
findings:
  critical: 1
  warning: 3
  info: 4
  total: 8
status: issues_found
---

# Phase 70: Code Review Report

**Reviewed:** 2026-04-19T20:10:12Z
**Depth:** standard
**Files Reviewed:** 25
**Status:** issues_found

## Summary

Phase 70 introduces cross-entity `__` traversal in pdbcompat, adding Path A
allowlists (codegen-emitted from `schemaannot.WithPrepareQueryAllow`) and
Path B automatic introspection (`LookupEdge` + codegen-emitted `Edges` map).
Overall architecture is sound: the leaf `schemaannot` package breaks the
would-be import cycle correctly, Phase 68 status-matrix and Phase 69
`_fold` / empty-`__in` invariants are preserved everywhere they matter,
the 2-hop cap enforces at the parser (parseFieldOp / ParseFiltersCtx
len>2 check) before any walker recursion, the silent-ignore path returns
HTTP 200 + unfiltered rows as required, and all SQL identifiers in
`buildSinglHop` / `buildTwoHop` come from codegen-emitted `EdgeMetadata`
— no user-string lands in raw SQL. Codegen determinism is solid (three
`sort.Slice` / `sort.Strings` passes before template rendering, plus
`go/format.Source` post-process).

The critical finding below is a **SQL correctness bug for O2M Path B
traversals** that happens to be untested: `buildSinglHop` unconditionally
assumes the FK column lives on the parent table (correct for M2O edges
like `net→org`), but for O2M edges (e.g. `net→pocs`, `net→network_ix_lans`)
the FK lives on the child table and the generated SQL references a
non-existent column on the parent. This is not documented in
`deferred-items.md` and would trigger a request-time SQL error the first
time any caller exercises an O2M-shaped Path B query such as
`/api/net?poc__name=NOC` or `/api/ix?ixlan__name=...`. The three warnings
flag dead/unreachable Path A allowlist entries whose key shape
(`network_facilities`, `ixlan`, `ix`) doesn't match the parser's
`TraversalKey` lookup format (`netfac`, PDB-type strings), causing them
to silent-ignore indistinguishably from unknown fields despite appearing
"configured".

## Critical Issues

### CR-01: `buildSinglHop` / `buildTwoHop` generate invalid SQL for O2M Path B traversals

**File:** `internal/pdbcompat/filter.go:359-368` (`buildSinglHop`),
`internal/pdbcompat/filter.go:412-422` (`buildTwoHop`)

**Issue:** Both functions hard-code the subquery direction to M2O — they
emit `WHERE parent.<ParentFKColumn> IN (SELECT target.<TargetIDColumn>
FROM <TargetTable> WHERE <inner>)`. This is correct when the edge is M2O
(FK lives on the parent table, e.g. `networks.org_id → organizations.id`).
It is **incorrect when the edge is O2M** — the FK lives on the child
table (e.g. `pocs.net_id`, not `networks.net_id`), and the correct shape
is `WHERE parent.<id> IN (SELECT child.<FKColumn> FROM <ChildTable>
WHERE <inner>)`.

The codegen tool (`cmd/pdb-compat-allowlist/main.go:310-325`)
acknowledges this in a comment ("both directions produce valid
subqueries using the same triple; only the WHERE/IN pairing differs and
that's Plan 70-05 territory") but Plan 70-05 in `filter.go` does not act
on the information — the direction is never inverted and `EdgeMetadata`
has no `OwnFK`/`direction` field to disambiguate.

Reproducer (currently untested at SQL-execution level):

```
GET /api/net?poc__name=NOC
```

Resolution trace:
1. `parseFieldOp` → relSegs=["poc"], field="name"
2. Path A: Allowlists["net"].Direct does NOT contain "poc__name" — falls through
3. Path B: `LookupEdge("net", "poc")` → `{ParentFKColumn: "net_id",
   TargetTable: "pocs", TargetIDColumn: "id"}` (from allowlist_gen.go:201)
4. `buildSinglHop` emits: `networks.net_id IN (SELECT pocs.id FROM pocs
   WHERE pocs.name = 'NOC')` — but `networks.net_id` does not exist.
5. Runtime: SQLite returns "no such column: networks.net_id", bubbling
   up as HTTP 500 via `WriteProblem` in `handler.go:224-229`.

Same bug applies to every O2M edge in the codegen-emitted `Edges` map:
- `net`: `network_facilities`, `network_ix_lans`, `pocs`
- `fac`: `carrier_facilities`, `ix_facilities`, `network_facilities`
- `ix`: `ix_facilities`, `ix_lans`
- `ixlan`: `ix_prefixes`, `network_ix_lans`
- `org`: `campuses`, `carriers`, `facilities`, `internet_exchanges`,
  `networks`
- `campus`: `facilities`
- `carrier`: `carrier_facilities`

Reason this slipped through: no integration or E2E test executes an O2M
Path B query. `TestParseFilters_Traversal_Table` line 49 has a `poc__name`
case but only asserts `wantPreds: 1` (predicate count, no SQL). All SQL-
executing tests (`TestBuildTraversal_SingleHop_Integration`,
`TestBuildTraversal_TwoHop_Integration`, `TestTraversal_E2E_Matrix`) use
M2O chains (`org__id`, `ixlan__ix__id`, `netfac?net__asn`). The Path A
allowlist entries that would trigger O2M are all unreachable due to the
`TraversalKey` mismatch flagged in WR-01 / WR-02 below, which is why
production hasn't hit this yet.

**Fix:** Extend `EdgeMetadata` with an `OwnFK bool` field (true = FK on
parent table / M2O; false = FK on child table / O2M) populated from
`gen.Edge.IsInverse()` or `edge.Rel.Type` (M2O vs O2M) at codegen time,
then branch in `buildSinglHop`:

```go
return func(s *sql.Selector) {
    t := sql.Table(targetTable)
    if edge.OwnFK {
        // M2O: FK on parent
        innerSel := sql.Select(t.C(targetID)).From(t)
        innerPred(innerSel)
        s.Where(sql.In(s.C(parentFK), innerSel))
    } else {
        // O2M: FK on child
        innerSel := sql.Select(t.C(parentFK)).From(t)
        innerPred(innerSel)
        s.Where(sql.In(s.C(targetID), innerSel))
    }
}, true, false, nil
```

(`targetID` here is the parent's PK column name in the O2M branch —
rename `parentID` for clarity when implementing.) Same branching needed
in `buildTwoHop` for each hop independently. Add integration tests:

```go
// Path B O2M — ix_lans O2M from ix
preds, _, _ := ParseFiltersCtx(WithUnknownFields(ctx),
    url.Values{"ixlan__name": {"Production LAN"}},
    Registry[peeringdb.TypeIX])
q := client.InternetExchange.Query().Where(func(s *sql.Selector) {
    for _, p := range preds { p(s) }
}).Where(internetexchange.StatusEQ("ok"))
ixs, err := q.All(ctx)  // MUST NOT error, MUST NOT return empty when seeded
```

Add one such test for each O2M edge category (net→pocs, ix→ixlans,
fac→network_facilities, org→networks, campus→facilities).

**Severity rationale (Critical):** Untested code path + runtime SQL error
= first caller to try an O2M Path B shape (including any upstream-parity
test in Phase 72) hits HTTP 500. Not a data-loss or auth-bypass
vulnerability, but it's a correctness failure that breaks the silent-
ignore contract from the user's perspective (they get a 500, not a 200
with unfiltered rows).

## Warnings

### WR-01: Path A allowlist `network_facilities__facility__name` on net is unreachable

**File:** `ent/schema/network.go:266`,
`internal/pdbcompat/allowlist_gen.go:97-100`

**Issue:** The Network schema declares
`schemaannot.WithPrepareQueryAllow("network_facilities__facility__name", ...)`
(line 266), which codegens into
`Allowlists["net"].Via["network_facilities"] = ["facility__name"]`. At
filter-parse time, `buildTraversalPredicate` matches this entry and
calls `buildTwoHop("net", "network_facilities", "facility", "name", ...)`.
`buildTwoHop` then calls `LookupEdge("net", "network_facilities")` —
which iterates `Edges["net"]` looking for `TraversalKey ==
"network_facilities"`. But all `TraversalKey` values in `Edges["net"]`
(allowlist_gen.go:198-201) are PDB type strings: `netfac`, `netixlan`,
`org`, `poc` — never the ent edge Go name. The lookup fails, buildTwoHop
returns `(nil, false, false, nil)`, and the key falls into the silent-
ignore / unknown-field bucket.

Net effect: the allowlist entry is dead code — upstream-parity-looking
but unreachable. A caller using `?network_facilities__facility__name=X`
gets the same unfiltered response as `?bogus__name=X`.

The fix either re-writes the schema annotation to use the TraversalKey
form (`"netfac__fac__name"`) so codegen emits `Via["netfac"] =
["fac__name"]`, or teaches `buildTraversalPredicate` to normalise the
relSegs through an edge-Go-name-to-TraversalKey map before calling
`LookupEdge`. The former is simpler (no parser changes) and matches the
convention already used elsewhere:
`Allowlists["fac"].Via["ixlan"] = ["ix__fac_count"]` uses PDB type
strings. Updating `network.go:266` to `"netfac__fac__name"` would align
with the rest of the codebase.

**Fix:**
```go
// ent/schema/network.go:266
schemaannot.WithPrepareQueryAllow(
    "org__name",
    "org__id",
    "ix__name",
    "ixlan__name",
    "fac__name",
    "netfac__fac__name",  // was "network_facilities__facility__name"
),
```

Note: this entry is ALSO blocked by CR-01 (O2M direction bug) once
reachable — the edge `net→netfac` is O2M, so even a correctly-keyed
entry produces wrong SQL today. CR-01 is the gating fix; WR-01 can land
alongside or separately.

### WR-02: Path A allowlists `ix__*`, `ixlan__name`, `fac__name` on net are unreachable

**File:** `ent/schema/network.go:261-267`,
`internal/pdbcompat/allowlist_gen.go:88-95`

**Issue:** `Allowlists["net"].Direct` contains `fac__name`, `ix__name`,
`ixlan__name`. None of these correspond to an edge on `net` —
`Edges["net"]` exposes `netfac`, `netixlan`, `org`, `poc` (no direct
`fac`, `ix`, or `ixlan` edge because those are reachable only through
the junction types). `buildSinglHop` fails at `LookupEdge("net", "fac")`
and friends, returns `(nil, false, false, nil)`, and the key silent-
ignores.

`TestTraversal_E2E_Matrix.upstream_5081_net_ix_name_contains` at
`internal/pdbcompat/traversal_e2e_test.go:176-181` explicitly documents
and locks this silent-ignore behaviour. So the behaviour is at least
intentional and tested — but the upstream-parity allowlist comment at
`network.go:253-259` says the entries exist to mirror upstream
`serializers.py:2947`. They don't actually produce any functional
difference vs. deleting them. Future readers may be misled into
expecting `?ix__name=X` on /api/net to work when it doesn't.

**Fix:** Either (a) add a comment on these specific entries noting they
are upstream-parity-only and always silent-ignore in pdbcompat-plus
because the traversal path runs through the junction types
(`net→netixlan→ixlan→ix`, exceeding the 2-hop cap); or (b) remove them
and add a one-line note in the `Phase 70 TRAVERSAL-01` comment block
citing the D-04 cap as the reason upstream's one-hop-through-junction
shortcuts are silent-ignored. Option (a) is lower-risk.

### WR-03: Path A match on unreachable entry short-circuits Path B fallback

**File:** `internal/pdbcompat/filter.go:288-301`

**Issue:** When Path A matches an allowlist entry
(`slices.Contains(entry.Direct, fullKey)`) but the downstream
`buildSinglHop` fails (e.g. `LookupEdge` returns false), the code
returns the result directly:

```go
if slices.Contains(entry.Direct, fullKey) {
    return buildSinglHop(tc.Name, relSegs[0], field, op, value)
}
```

It does NOT fall through to the Path B block. For the specific cases
flagged in WR-02 this is currently fine — Path B would also fail. But
the coupling is fragile: if someone adds a Path A allowlist entry
`?xyz__name=...` whose `xyz` happens to also be a valid Path B
`TraversalKey` on a different schema, the Path A match can suppress a
Path B resolution that would have worked. The intent described in the
Plan 70-05 comments is "Path A first, then Path B" — implemented
literally, but without the "if Path A match failed silently, retry
Path B" fallback the phrasing suggests.

**Fix:** Either (a) extract a helper
`tryPathA(...) (pred, matched, ok, empty, err)` where `matched` is true
iff the key was in the allowlist, and only Path-B-fallback when
`matched` was true and `ok` came back false; or (b) simply re-execute
the Path B lookup whenever `buildSinglHop` / `buildTwoHop` from Path A
returns `ok=false` without an explicit emptyResult/err. Option (a) is
clearer; (b) is a three-line patch. A code comment pointing at the
current "silent fall-through" decision (with citation to the test cases
that lock it) would also discharge the warning.

Relevant test: currently no test asserts "Path A allowlist entry
matches but buildSinglHop fails AND Path B would have succeeded" because
no schema has this exact shape today. If WR-01 is fixed by renaming
`network_facilities__facility__name` → `netfac__fac__name`, that case
collapses (Path A would succeed). If CR-01 is fixed, the question of
what happens when Path A returns `ok=false` remains (e.g. target entity
missing from Registry).

## Info

### IN-01: `extractAllowlist` case-2 accepts malformed `__`-leading keys

**File:** `cmd/pdb-compat-allowlist/main.go:178-193`

**Issue:** `strings.Split("__", "__")` returns `["", ""]`, which has
`len(parts) == 2` and gets appended to `entry.Direct` as the literal
string `"__"`. Same for any key starting with `__` (e.g. `"__foo"` →
`["", "foo"]`, appended as direct). The Direct slice then contains a
nonsense key that can never match a legitimate filter param (every
inbound param has a non-empty field name per the `finalField == ""`
guard in `ParseFiltersCtx`).

This is only reachable if a developer writes a malformed annotation —
not a user-input concern. But the case-0,1 branch explicitly logs a
`"skipping malformed field"` warning while case-2 silently accepts
leading/trailing empty segments. Inconsistent error reporting.

**Fix:** In the case-2 branch, add a guard:
```go
case 2:
    if parts[0] == "" || parts[1] == "" {
        log.Printf("pdb-compat-allowlist: %s skipping malformed field %q (empty segment)", node.Name, f)
        continue
    }
    entry.Direct = append(entry.Direct, f)
```
Same for case 3 — check all three segments non-empty.

### IN-02: `isKnownOperator` list duplicates operator subset between `coerceToCaseInsensitive`

**File:** `internal/pdbcompat/filter.go:68-78`, `106-114`

**Issue:** `isKnownOperator` accepts both `contains`/`icontains` and
`startswith`/`istartswith`. `coerceToCaseInsensitive` then rewrites
`contains`→`icontains` and `startswith`→`istartswith`. The `iexact`
operator is in `isKnownOperator` but has no corresponding
case-sensitive variant (which is correct — Django doesn't have
`exact`/`iexact` pairing symmetry). If someone adds a new case-sensitive
variant (e.g. `endswith`) to `isKnownOperator`, they must remember to
also extend `coerceToCaseInsensitive` — otherwise the coercion will
diverge silently.

**Fix:** Define a single source of truth:
```go
var operatorCoerceMap = map[string]string{
    "contains":   "icontains",
    "startswith": "istartswith",
    // endswith: "iendswith", etc.
}

func isKnownOperator(suffix string) bool {
    switch suffix {
    case "icontains", "istartswith", "iexact", "in", "lt", "gt", "lte", "gte":
        return true
    }
    _, ok := operatorCoerceMap[suffix]
    return ok
}

func coerceToCaseInsensitive(op string) string {
    if v, ok := operatorCoerceMap[op]; ok {
        return v
    }
    return op
}
```

Low-priority stylistic cleanup; the current two-function form is
entirely correct for today's operator set.

### IN-03: `indexBody` init swallows JSON marshal error

**File:** `internal/pdbcompat/handler.go:102-118`

**Issue:** `indexBody, _ = json.Marshal(index)` in the `init()` function
discards the error. `map[string]indexEntry` with stdlib types cannot
actually fail JSON marshal, so the discard is safe, but the leading
underscore hides a potentially latent bug if someone later extends
`indexEntry` with a type that CAN fail marshal (e.g. `json.RawMessage`
or a custom `MarshalJSON` that returns errors).

**Fix:**
```go
body, err := json.Marshal(index)
if err != nil {
    panic(fmt.Sprintf("pdbcompat: init marshal index: %v", err))
}
indexBody = body
```

Or at least a comment: `// safe: only primitive types`. The `init`-time
panic is appropriate because a broken index body is a startup-time
correctness failure, not a runtime user input error.

### IN-04: `ent/schema/ixlan.go:49-54` — `ixf_ixp_member_list_url_visible` has no `WithSkip(true)` annotation

**File:** `ent/schema/ixlan.go:51-54`

**Issue:** The field is declared as a regular `field.String("...")` with
no `entrest.WithSkip(true)` or `entgql.Skip(entgql.SkipAll)` annotation.
Per the CLAUDE.md "Field-level privacy" section, the `_visible`
companion IS meant to be emitted on all surfaces (Phase 64 D-05 matches
upstream behaviour), so this is actually correct — not a bug. Flagging
as INFO because a reviewer unfamiliar with D-05 might interpret the
absence of `WithSkip` as an oversight; a one-line comment on the field
would discharge the ambiguity:

```go
field.String("ixf_ixp_member_list_url_visible").
    Optional().
    Default("Private").
    // Intentionally emitted on all 5 API surfaces per Phase 64 D-05
    // (upstream PeeringDB always returns the visibility marker even
    // when the gated value is redacted via privfield.Redact).
    Comment("IXF member list URL visibility"),
```

No code change required — purely documentation hygiene.

---

**Review area cross-check (from `<review_context>` focus areas):**

1. **SQL injection risk in buildSinglHop / buildTwoHop** — PASS. All
   SQL identifiers (`parentFK`, `targetTable`, `targetID`, `fk1Col`,
   `midTable`, etc.) are read from the codegen-emitted `EdgeMetadata`
   map via `LookupEdge`. User input (field name, value) goes through
   `buildPredicate` which binds via `sql.FieldEQ` / `sql.FieldContainsFold`
   / `sql.ExprP(..., jsonStr)` — all parameterised. No user-controlled
   string lands in raw SQL at the subquery-construction layer.

2. **2-hop cap enforcement** — PASS. `ParseFiltersCtx` line 203-206
   hard-caps at `len(relSegs) > 2` BEFORE any call to
   `buildTraversalPredicate`. `buildTraversalPredicate` has no recursion
   and only two code paths (1-hop `buildSinglHop`, 2-hop `buildTwoHop`);
   no third-hop construction is reachable. D-04 cap enforced in the
   parser as specified.

3. **Silent-ignore correctness** — PASS for known/unknown-field
   dispatch. Unknown field → `appendUnknown(ctx, key); continue` →
   predicate slice does not include this param → list query runs with
   the other predicates only → HTTP 200 with rows that would otherwise
   match. `handler.go:168-182` emits diagnostic slog + OTel without
   changing the response. **Caveat:** CR-01 converts silent-ignore into
   HTTP 500 for O2M Path B cases once exercised.

4. **Phase 68/69 preservation** — PASS.
   `applyStatusMatrix`: 13 call sites in `registry_funcs.go`, 1
   helper in `filter.go:85-94`, 1 test assertion —
   matches the 13 entity types.
   `opts.EmptyResult`: 13 call sites in `registry_funcs.go` — matches.
   `unifold.Fold`: 7 call sites in `filter.go` (via `buildExact`,
   `buildContains`, `buildStartsWith`) plus 2 in tests — traversal
   predicates call `buildPredicate` on the TARGET TypeConfig
   (`filter.go:348, 398`) so fold routing composes correctly at the
   subquery level.
   `StatusIn("ok", "pending")`: 26 literal occurrences in `depth.go`
   — matches the 26 pk-lookup sites specified in CLAUDE.md.

5. **Codegen determinism** — PASS.
   `extractAllowlist` sorts `entry.Direct` and `viaMap[k]` before
   appending Via entries (main.go:194-205). `main.go:126-137` sorts the
   three top-level slices (`Entries`, `FilterExcludes`, `EdgeEntries`)
   by stable keys. Template uses `printf "%q"` for all strings.
   `go/format.Source` post-processes. Map iterations (viaMap, `hops`)
   are collected into slices and sorted before template emission — no
   map-ranging in the template itself. Byte-identical output across
   runs expected.

6. **Import hygiene** — PASS. Grep confirms `internal/pdbcompat/schemaannot/`
   has zero `ent` imports (verified via `Grep import.*\bent\b` returned
   no matches). All 13 ent/schema/*.go files import `schemaannot` not
   `pdbcompat` (verified via `Grep schemaannot` returning 13 files and
   `Grep pdbcompat"` in ent/schema returning zero). Leaf package
   isolation intact — import cycle cannot form.

7. **Privacy/surface leakage** — PASS for `_fold` columns. Spot-checked
   `ent/schema/network.go:201-215` and `ent/schema/organization.go:127-140`
   — both `*_fold` fields carry `entgql.Skip(entgql.SkipAll)` +
   `entrest.WithSkip(true)`. Proto is globally frozen via
   `entproto.SkipGenFile` in `ent/entc.go` per the CLAUDE.md note.
   pdbcompat serializers already excluded `_fold` pre-Phase 70.
   Traversal codegen (`Edges`, `Allowlists`, `FilterExcludes`) is
   runtime-only Go data, not an API surface — no entgql / entrest / ent
   serializer exposure. Confirmed no `_fold` reference appears in any of
   the 5 serializer paths for traversal-generated predicates.

---

_Reviewed: 2026-04-19T20:10:12Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
