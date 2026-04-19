# Phase 67: Default Ordering Flip — Research

**Researched:** 2026-04-19
**Domain:** Multi-surface query layer (pdbcompat + grpcserver + entrest) default ordering
**Confidence:** HIGH for code inventory, MEDIUM-HIGH for entrest surface, HIGH for cursor/goldens

## User Constraints (from CONTEXT.md)

### Locked Decisions

- **D-01 — grpcserver cursor strategy.** Compound `(last_updated, last_id)` cursor. Base64-encoded cursor body changes shape; existing proto types for cursor are opaque (wire type is `string`, not `bytes` — see note under §4); no proto regen required. Cursor is stable under concurrent edits because the `id` tiebreaker is monotonic. No grpc public consumers currently — breaking change is acceptable.
- **D-02 — entrest default ordering source.** Per-schema `entrest.WithDefaultOrder(...)` on all 13 ent schemas; `go generate ./...` rerun. **See §3 for API-shape correction and §8 G-01 for scope surprise.**
- **D-03 — Golden file regeneration strategy.** Regenerate 39 pdbcompat goldens atomically; `git diff` must show only row-reorder changes.
- **D-04 — Ordering scope.** List endpoints only (`/api/<type>`, `/rest/v1/<type>`, `List*`/`Stream*`). Single-object lookups and nested `_set` fields at `depth>=1` unchanged.
- **D-05 — Streaming cursor resume semantics.** `since_id` and `updated_since` predicates apply BEFORE ordering.
- **D-06 — `grpc-total-count` header semantics unchanged.**

### Claude's Discretion

- Tie-breaker field when `(updated, created)` collide — not enumerated in CONTEXT.md. Recommendation in §8 G-02.
- Exact entrest annotation expression (given the compound-order limitation in §3).
- Cursor body serialization format (JSON vs custom delimiter) — see §4 recommendation.

### Deferred Ideas (OUT OF SCOPE)

- GraphQL default ordering (Relay connection spec).
- Web UI / terminal renderer ordering.
- Performance optimisation for `ORDER BY updated DESC` (CONTEXT.md claims existing `updated` indexes on all 13 schemas from "v1.9 Phase 46" — **see §8 G-03: no such indexes exist**).

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| ORDER-01 | `pdbcompat` list endpoints (`/api/<type>`) return rows ordered by `(-updated, -created)` matching `django-handleref` base `Meta.ordering` | §1 pdbcompat current state, §2 target state, §5 golden regen |
| ORDER-02 | `grpcserver` List and Stream RPCs return rows ordered by `(-updated, -created)`; cursor pagination remains stable under this order | §1 grpcserver state, §4 cursor migration, §6 test impact |
| ORDER-03 | `entrest` (`/rest/v1/*`) default list ordering matches upstream; explicit `?sort=` overrides still honoured | §1 entrest state, §3 annotation API, §8 G-01 scope correction |

## Summary

Phase 67 flips the base ORDER BY from `id ASC` to `(-updated, -created, -id)` across three code paths that each own their own SQL construction. The three surfaces are cleanly partitioned in code and have no shared query builder, so the change fans out to ~30 touchpoints but is mechanically simple on each one.

**Three surprises the planner must absorb before writing plans:**

1. **entrest's `WithDefaultOrder` annotation cannot express compound order.** `WithDefaultOrder` takes a single `SortOrder` enum (`Asc`|`Desc`), and `WithDefaultSort` takes a single field name. The REST `?sort=<field>&order=<asc|desc>` wire protocol is itself single-field. This is a schematic mismatch with the `(-updated, -created)` requirement — we can pick a single primary default (`updated` desc) via the annotation, but a true compound tie-break has to be injected elsewhere (generator template override, or a per-schema query middleware). **Detailed options in §3.**
2. **Golden fixtures seed ONE row per type.** A reorder of a single-element list is a no-op in the diff. Ordering correctness cannot be proved by the existing golden corpus. The reorder-audit review mandated by D-03 will be a tautology unless multi-row fixtures are introduced specifically for this phase.
3. **No `updated` indexes exist on any of the 13 entities.** CONTEXT.md "Out of scope" bullet 3 references "existing `updated` indexes on all 13 schemas (added v1.9 Phase 46)" — Phase 46 was actually UI-layer work (`search-compare-density`), and a direct audit of `ent/schema/*.go` `Indexes()` blocks confirms no created/updated indexes ever existed. Full-table sort of ~65k rows by `updated` on every list query is the new steady-state. See §8 G-03 for memory/perf implications.

**Primary recommendation:** Plan a Wave-0 task to seed a multi-row golden fixture (≥3 rows per type with distinguishable `updated` timestamps) before any code edit, so goldens become a meaningful regression signal. Ship the entrest compound-order via an `entc.TemplateDir`/template override (post-processing the generated `applySorting*` functions) rather than fighting the single-field annotation API.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| ORDER BY clause emission | ent query layer (SQL) | — | All three surfaces terminate in an `*ent.<Type>Query` — the SQL is emitted by ent itself, not the surface |
| Default field/direction selection (pdbcompat) | `internal/pdbcompat/registry_funcs.go` | — | Each List closure calls `.Order(...)` directly before `.All()`; no middleware hook |
| Default field/direction selection (grpcserver) | `internal/grpcserver/<entity>.go` per-entity `Query`/`QueryBatch` closures | `generic.go` StreamEntities wires cursor | Each handler file literally invokes `.Order(ent.Asc(...))`; no shared default |
| Default field/direction selection (entrest) | `ent/schema/<Entity>.go` `Annotations()` | `ent/rest/sorting.go` (generated) | Declarative via `entrest.WithDefaultSort/Order` → generator emits `SortConfig.DefaultField/Order` |
| Cursor encoding | `internal/grpcserver/pagination.go` | each entity's List/Stream handler | Wire type is `string page_token` (base64 of an opaque body); body shape is owned by pagination.go |
| Tie-breaking determinism | ent query `.Order(...)` call sites | — | SQLite will not add implicit id tiebreak — must be explicit |

## Current State

### 1.1 pdbcompat (13 list closures)

**File:** `internal/pdbcompat/registry_funcs.go` — all 13 entity List closures live in one file and follow an identical pattern. Representative example (Organization, lines 56–81):

```go
// internal/pdbcompat/registry_funcs.go:63
q := client.Organization.Query().Where(preds...).Order(ent.Asc("id"))
```

Every one of the 13 `wire*Funcs()` calls the same shape:
```
q := client.<Type>.Query().Where(preds...).Order(ent.Asc("id"))
total, err := q.Count(ctx)
rows, err := q.Limit(opts.Limit).Offset(opts.Skip).All(ctx)
```

Touchpoint line numbers (grep-confirmed, `Order(ent.Asc("id"))` literal):

| Entity | File:line |
|--------|-----------|
| Organization | registry_funcs.go:63 |
| Network | registry_funcs.go:90 |
| Facility | registry_funcs.go:117 |
| InternetExchange | registry_funcs.go:144 |
| Poc | registry_funcs.go:171 |
| IxLan | registry_funcs.go:198 |
| IxPrefix | registry_funcs.go:225 |
| NetworkIxLan | registry_funcs.go:252 |
| NetworkFacility | registry_funcs.go:279 |
| IxFacility | registry_funcs.go:306 |
| Carrier | registry_funcs.go:333 |
| CarrierFacility | registry_funcs.go:360 |
| Campus | registry_funcs.go:387 |

**`since` parameter handling** (pdbcompat): `registry_funcs.go:49-54` — `applySince` returns `sql.FieldGTE("updated", *opts.Since)` and is added to `preds` **before** `.Order(...)`. Order is emitted AFTER predicates, so re-ordering does not change filter semantics. No code change required for `since` to compose with the new default.

### 1.2 grpcserver (26 order sites: 13 List + 13 Stream)

**Shared pagination config:** `internal/grpcserver/pagination.go`
- `defaultPageSize = 100`, `maxPageSize = 1000`, `streamBatchSize = 500`
- `decodePageToken(token string) (int, error)` — decodes base64 → integer offset
- `encodePageToken(offset int) string` — encodes integer offset → base64
- Cursor body is a **decimal-string offset** (e.g. base64 encode of `"100"` → `"MTAw"`). This is NOT structured; it is an offset-based pagination token.

**Per-entity order sites** (grep confirmed, `Order(ent.Asc(<type>.FieldID))`):

| Entity | List file:line | Stream file:line |
|--------|---------------|------------------|
| Campus | campus.go:131 | campus.go:166 |
| Carrier | carrier.go:111 | carrier.go:146 |
| CarrierFacility | carrierfacility.go:94 | carrierfacility.go:129 |
| Facility | facility.go:197 | facility.go:232 |
| InternetExchange | internetexchange.go:178 | internetexchange.go:213 |
| IxFacility | ixfacility.go:98 | ixfacility.go:133 |
| IxLan | ixlan.go:120 | ixlan.go:160 |
| IxPrefix | ixprefix.go:92 | ixprefix.go:127 |
| Network | network.go:203 | network.go:239 |
| NetworkFacility | networkfacility.go:104 | networkfacility.go:139 |
| NetworkIxLan | networkixlan.go:146 | networkixlan.go:181 |
| Organization | organization.go:137 | organization.go:173 |
| Poc | poc.go:104 | poc.go:139 |

**`SinceID` / `UpdatedSince` handling** (grpcserver): `internal/grpcserver/generic.go:99-104`:

```go
// Applied BEFORE the batched ORDER BY in QueryBatch
if params.SinceID != nil {
    predicates = append(predicates, sql.FieldGT("id", int(*params.SinceID)))
}
if params.UpdatedSince != nil {
    predicates = append(predicates, sql.FieldGT("updated", params.UpdatedSince.AsTime()))
}
```

**Current batch iteration (`generic.go:118-147`):** uses `lastID` keyset pagination — each `QueryBatch` receives `afterID` and per-handler code does `client.<Type>.Query().Where(<type>.IDGT(afterID)).Order(ent.Asc(<type>.FieldID)).Limit(...)`. **This is id-only keyset pagination.** Flipping the default ORDER BY to `(-updated, -created)` requires migrating the keyset cursor from `afterID int` to `(afterUpdated time.Time, afterID int)` — otherwise the streaming loop either skips rows or duplicates them under the new sort.

**Header contract unchanged:** `generic.go:107-116` — `grpc-total-count` preflight fires only when both `SinceID == nil && UpdatedSince == nil`. This semantics does NOT change under D-06.

### 1.3 entrest (13 schemas + 1 generated sorting.go)

**Generated sort configs** (`ent/rest/sorting.go`):

```go
CampusSortConfig = &SortConfig{
    Fields:       []string{"facilities.count", "id", "random"},
    DefaultField: "id",
    DefaultOrder: "asc",
}
```

Every one of the 13 `*SortConfig` blocks has `DefaultField: "id"` and `DefaultOrder: "asc"`. `Fields` is narrow — only `"id"`, `"random"`, and any field explicitly annotated with `entrest.WithSortable(true)`. **Today NO entity has `WithSortable(true)` on `updated` or `created`** (grep confirmed: zero matches for `entrest.WithSortable|entgql.OrderField` on `updated`/`created`).

**Per-schema annotation entry points** (13 files, all have a `func (<Type>) Annotations() []schema.Annotation` that currently returns three entries):

```go
// ent/schema/network.go:222-228 — representative
func (Network) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entgql.RelayConnection(),
        entgql.QueryField(),
        entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
    }
}
```

Confirmed present on: campus, carrier, carrierfacility, facility, internetexchange, ixfacility, ixlan, ixprefix, network, networkfacility, networkixlan, organization, poc — 13/13.

**Regeneration hazard** (CRITICAL for D-02): `cmd/pdb-schema-generate/main.go:694-700` includes the `Annotations()` method in its template. Running `go generate ./schema` regenerates every `<entity>.go` from `schema/peeringdb.json`, overwriting any hand-edited annotation chain. CLAUDE.md § Code Generation already warns about this (entproto precedent). There are two mitigation paths:
- Update the generator template at `cmd/pdb-schema-generate/main.go:693-700` to emit `entrest.WithDefaultSort("updated")` and `entrest.WithDefaultOrder(entrest.OrderDesc)` alongside the existing three annotations (preferred; one-shot template edit).
- Split `Annotations()` into a sibling file `<entity>_annotations.go` — **NOT POSSIBLE for `Annotations()`** because Go forbids two methods with the same receiver+name in a package. Only workable for NEW methods (as `poc_policy.go` does for `Policy()`); any extension of an already-generated method must happen in the generator.

## Target State

### 2.1 pdbcompat target

Each of the 13 list closures in `registry_funcs.go` changes from:

```go
q := client.Organization.Query().Where(preds...).Order(ent.Asc("id"))
```

to:

```go
q := client.Organization.Query().Where(preds...).
    Order(ent.Desc("updated"), ent.Desc("created"), ent.Desc("id"))
```

The explicit `Desc("id")` tertiary tiebreak is required (§8 G-02) — SQLite does not impose an implicit id ordering when the first two columns tie.

### 2.2 grpcserver target

**Per-entity** (all 26 sites): change `.Order(ent.Asc(<type>.FieldID))` to `.Order(ent.Desc(<type>.FieldUpdated), ent.Desc(<type>.FieldCreated), ent.Desc(<type>.FieldID))`.

**Streaming keyset pagination** (`generic.go` + every entity's `QueryBatch` closure): replace the `afterID int` keyset semantics with a compound cursor. Pseudocode for the new `QueryBatch` per-entity closure:

```go
QueryBatch: func(ctx context.Context, preds []func(*sql.Selector), cursor streamCursor, limit int) ([]*ent.Network, error) {
    q := s.Client.Network.Query().
        Order(ent.Desc(network.FieldUpdated), ent.Desc(network.FieldCreated), ent.Desc(network.FieldID)).
        Limit(limit)
    if !cursor.empty() {
        // keyset: (updated, id) < (cursor.Updated, cursor.ID) in the DESC order
        q = q.Where(sql.OrPred(
            sql.FieldLT("updated", cursor.Updated),
            sql.And(sql.FieldEQ("updated", cursor.Updated), sql.FieldLT("id", cursor.ID)),
        ))
    }
    if len(preds) > 0 { q = q.Where(network.And(castPredicates[predicate.Network](preds)...)) }
    return q.All(ctx)
}
```

Note the two-column keyset predicate — `created` is NOT part of the keyset because `(updated, id)` is already unique (id is unique on its own). Adding `created` to the keyset would be correctness-safe but wasteful.

**Cursor body** (pagination.go): see §4.

### 2.3 entrest target

See §3 for the annotation-API limitation. The working target is:

```go
// ent/schema/network.go:222 (and 12 analogous sites)
func (Network) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entgql.RelayConnection(),
        entgql.QueryField(),
        entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
        entrest.WithDefaultSort("updated"),
        entrest.WithDefaultOrder(entrest.OrderDesc),
    }
}
```

Plus `entrest.WithSortable(true)` on the `updated` field (and `created` if we want callers to be able to `?sort=created` — optional):

```go
field.Time("updated").
    Annotations(
        entrest.WithSortable(true),
        entrest.WithFilter(entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE),
    ).
    Comment("PeeringDB last update timestamp"),
```

`WithSortable` is required — without it, `GetSortableFields` (`schema_sorting.go:42`) excludes the field and codegen panics at line 80 with `"default sort field "updated" on schema "Network" does not exist (valid: ...) or does not have default sorting enabled"`.

**Compound default is not expressible via annotations — see §3 for the alternative.**

## entrest WithDefaultOrder API

**Verified against:** `github.com/lrstanley/entrest@v1.0.3` (downloaded to module cache; authoritative source read).

**Signatures** (`annotations.go:557-576`):

```go
// WithDefaultSort sets the default sort field for the schema in the REST API.
// If not specified, will default to the "id" field.
func WithDefaultSort(field string) Annotation { ... }

// WithDefaultOrder sets the default sorting order for the schema in the REST API.
// If not specified, will default to ASC.
func WithDefaultOrder(v SortOrder) Annotation { ... }
```

**`SortOrder` enum** (`schema_sorting.go:15-23`):

```go
type SortOrder string
const (
    OrderAsc  SortOrder = "asc"
    OrderDesc SortOrder = "desc"
)
```

**Wire protocol is single-field** (`ent/rest/sorting.go:28-34`, generated from `templates/sorting.tmpl`):

```go
type Sorted struct {
    Field *string         `json:"sort"  form:"sort,omitempty"`
    Order *orderDirection `json:"order" form:"order,omitempty"`
}
```

**`applySorting<Type>` is called once per query** (`ent/rest/list.go` via `templates/list.tmpl:272`):
```go
applySorting{{ $t.Name|zsingular }}(_query, *l.Field, *l.Order)
```

The helper in `templates/sorting.tmpl:121-161` does exactly one `_query.Order(withFieldSelector(_field, _order))` — **no mechanism for compound order exists**.

### CONTEXT.md D-02 correction

CONTEXT.md says: `entrest.WithDefaultOrder(entrest.OrderDesc("updated"), entrest.OrderDesc("created"))`.

This function signature does **not exist in entrest v1.0.3**. The actual signature is `WithDefaultOrder(v SortOrder)` with no field argument. Combined with the generated template's single-field `Sorted` struct, there is no way to express compound order through the annotation API alone.

### Three implementation options for compound default on entrest

**Option A — Accept single-field entrest default (`WithDefaultSort("updated")` + `WithDefaultOrder(OrderDesc)`), skip `created` tiebreak on REST only.** Simplest. Diverges from upstream ordering when two rows share `updated` but have different `created`. For PeeringDB's update cadence (minute-grained upstream timestamps, rarely colliding at second precision), the observable divergence is near-zero on real data.

**Option B — Override the entrest sorting template via `entc.TemplateDir`.** Ship a project-local copy of `sorting.tmpl` that changes the `applySorting<Type>` body to append a deterministic compound ORDER BY when the request's `Field` equals `"updated"`. Fully compound default, but creates a template-maintenance burden (entrest upgrades require a manual diff against the new upstream template).

**Option C — Wrap `restSrv.Handler()` with a query-interceptor middleware** or use ent hooks/interceptors (`entgo.io/ent/entc/gen` post-generation hook or `ent.Intercept` at runtime) that post-processes the query before `.All()`. Complex and brittle against entrest's generated `ExecutePaginated` call path; not recommended.

**Recommendation for planner:** Option A as the default. If ordering parity with pdbcompat/grpcserver must be byte-identical on REST list responses (verify with product owner), escalate to Option B and plan one additional task to vendor the entrest sorting template under `ent/templates/`. The CONTEXT.md scope ("matches upstream") is probably Option A territory — upstream PeeringDB's REST is itself single-field-sort at the HTTP layer.

Reference: `docs/content/docs/openapi-specs/annotation-reference.mdx:152-190` ships this as official guidance.

## Cursor shape migration

### Current cursor body

**File:** `internal/grpcserver/pagination.go`

Body: decimal-string integer offset.
Example: offset=100 → `base64.StdEncoding.EncodeToString([]byte("100"))` → `"MTAw"`.

Wire type: `string page_token` (not `bytes` — CONTEXT.md D-01 says "opaque bytes" which is wrong about the wire type but right about the proto-regen implication: string is already opaque from the caller's POV, so no proto change needed).

Current proto (unchanged): `proto/peeringdb/v1/services.proto` — `string page_token = 2;` on every `List*Request`, `string next_page_token = 2;` on every `List*Response` (26 total positions, grep confirmed).

### Target cursor body

Needs to encode `(last_updated time.Time, last_id int)`. Three viable formats:

| Format | Body | Pros | Cons |
|--------|------|------|------|
| JSON | `{"u":"2026-04-01T10:00:00Z","i":1234}` | Self-documenting, easy to extend | Larger (~45B vs 15B); whitespace/key-order hazards |
| Fixed delimiter | `"2026-04-01T10:00:00Z\x001234"` | Compact; no parse ambiguity | Custom parser; timezone/precision assumptions baked in |
| RFC3339Nano+id colon | `"2026-04-01T10:00:00.000000000Z:1234"` | Compact, human-readable pre-base64, one-line parser | Requires discipline around time.Format(RFC3339Nano); colon-in-value unsafe generally but safe here |

**Recommendation:** Option 3 (RFC3339Nano+id colon). Use `time.Format(time.RFC3339Nano)` on encode to eliminate precision-loss on sub-second timestamps, and split on the LAST `:` (not first — RFC3339 itself contains colons in the time portion).

### Encode/decode changes

New signatures in `pagination.go`:

```go
type streamCursor struct {
    Updated time.Time
    ID      int
}

func (c streamCursor) empty() bool { return c.Updated.IsZero() && c.ID == 0 }

func encodeStreamCursor(c streamCursor) string {
    if c.empty() { return "" }
    body := fmt.Sprintf("%s:%d", c.Updated.UTC().Format(time.RFC3339Nano), c.ID)
    return base64.StdEncoding.EncodeToString([]byte(body))
}

func decodeStreamCursor(token string) (streamCursor, error) {
    if token == "" { return streamCursor{}, nil }
    raw, err := base64.StdEncoding.DecodeString(token)
    if err != nil { return streamCursor{}, fmt.Errorf("decode cursor: %w", err) }
    s := string(raw)
    idx := strings.LastIndex(s, ":")
    if idx < 0 { return streamCursor{}, fmt.Errorf("invalid cursor body: %q", s) }
    t, err := time.Parse(time.RFC3339Nano, s[:idx])
    if err != nil { return streamCursor{}, fmt.Errorf("parse cursor timestamp: %w", err) }
    id, err := strconv.Atoi(s[idx+1:])
    if err != nil { return streamCursor{}, fmt.Errorf("parse cursor id: %w", err) }
    if id < 0 { return streamCursor{}, fmt.Errorf("invalid cursor: negative id %d", id) }
    return streamCursor{Updated: t, ID: id}, nil
}
```

**Note on ListEntities** (§1.2): `ListEntities` currently uses offset-based pagination (`decodePageToken` → offset int → `Limit(pageSize+1).Offset(offset)`). Under the new default ORDER BY, offset-based pagination is STILL correct for non-streaming List RPCs — the query is deterministic per-request. Only Stream* needs keyset cursor migration. **Simplification opportunity:** keep `encode/decodePageToken` (offset-based) for `List*`, introduce NEW `encode/decodeStreamCursor` for `Stream*`. Two cursor shapes, two helper pairs, zero collision.

Alternative: migrate both to the compound cursor. Adds complexity on `List*` without benefit (offset pagination under a deterministic ORDER BY is already stable). Not recommended.

### Callers affected

**In-tree base64 cursor consumers** (grep for `encodePageToken|decodePageToken`):

| File | Affected |
|------|----------|
| `internal/grpcserver/pagination.go` | Owner — rewrite per above |
| `internal/grpcserver/pagination_test.go` | All 4 test funcs need updated fixtures; keyset round-trip needs a new test |
| `internal/grpcserver/generic.go:28-51` (ListEntities) | No change if we split into two cursor types |
| `internal/grpcserver/generic.go:118-147` (StreamEntities batch loop) | Replace `lastID int` with `cursor streamCursor`; change every `QueryBatch` invocation |
| 13 per-entity `StreamParams.QueryBatch` closures | Signature changes from `(ctx, preds, afterID int, limit int)` to `(ctx, preds, cursor streamCursor, limit int)` |

**Out-of-tree consumers:** none (per CONTEXT.md D-01 — "no grpc public consumers").

## Golden regeneration workflow

### Current mechanics

**Command:** `go test -update ./internal/pdbcompat -run TestGoldenFiles`

**Framework:** custom. `golden_test.go:20` declares `var update = flag.Bool("update", false, "update golden files")`. `compareOrUpdate(t, goldenPath, got []byte)` at `golden_test.go:26-48` branches on `*update`:
- `true`: `os.WriteFile(goldenPath, got, 0o644)` (creates parent dir as needed).
- `false`: reads the file, `cmp.Diff` against received body, reports `golden mismatch for %s`.

**Fixture seed:** `setupGoldenTestData(t)` at `golden_test.go:54-273` creates exactly **one row per type** (13 types × 1 row = 13 entities). All rows share `goldenTime = 2025-01-01T00:00:00Z` for both `created` and `updated`.

**Goldens emitted:** 13 types × 3 scenarios (`list`, `detail`, `depth`) = **39 files** — matches CONTEXT.md D-03's count.

### Diff-audit hazard

With a single row per type, `ORDER BY updated DESC, created DESC, id DESC` produces byte-for-byte identical output to the current `ORDER BY id ASC`. **Every single `list.json` under the current fixture is a no-op under the reorder.** A diff audit will show zero lines changed — indistinguishable from "the code change did nothing." The D-03 "reorder-only" review criterion cannot be met because there is nothing to reorder.

### Recommended remediation (plan task)

1. **Extend `setupGoldenTestData`** to seed a second row for a representative subset of types (suggestion: all 4 "heavy" types with multiple rows in reality — network, facility, internetexchange, ixlan — plus organization). Use distinguishable timestamps:
   - Row 1: `goldenTime = 2025-01-01T00:00:00Z` (existing).
   - Row 2: `goldenTime.Add(24*time.Hour) = 2025-01-02T00:00:00Z`.
   - Expected sorted output under new default: row 2 then row 1 (newer first).
2. Regenerate the 39 goldens: `go test -update ./internal/pdbcompat -run TestGoldenFiles`.
3. Diff audit criterion: for the 5 seeded-twice types, `list.json` shows row 2 precede row 1; for the 8 single-row types, diff is empty. No structural changes (field additions/removals/renames) acceptable.
4. `depth.json` on the 5 seeded-twice types will gain a second row in the outer `data` array too — expected. Inner `_set` fields (eager-loaded edges) are NOT affected by this phase's ordering change per D-04.

### No tests for ordering stability exist today

Grep confirms: zero test functions named `*Ordering*` or `*Order*` in `internal/pdbcompat`. The only ordering-related test in the codebase is `TestListNetworks/"ordering by ID ascending"` at `internal/grpcserver/grpcserver_test.go:189-202` — it asserts `prev.GetId() >= curr.GetId()` and MUST be rewritten to assert the new order.

## Test impact

### pdbcompat

| File | Change required |
|------|-----------------|
| `internal/pdbcompat/golden_test.go` | Extend `setupGoldenTestData` to seed multi-row fixtures for 4–5 types (see §5). |
| `internal/pdbcompat/testdata/golden/**/list.json` | Regenerate 13 files (5 will show reorder, 8 unchanged). |
| `internal/pdbcompat/testdata/golden/**/detail.json` | Regenerate 13 files (all expected unchanged — detail has no list). |
| `internal/pdbcompat/testdata/golden/**/depth.json` | Regenerate 13 files (5 may see row 2 additions in outer array; inner `_set` unchanged). |
| NEW test file | `registry_funcs_ordering_test.go` — assert multi-row seed returns `(-updated, -created)` order via JSON response parse, per type. At minimum network+facility+organization as representative coverage. |

### grpcserver

| File | Change required |
|------|-----------------|
| `internal/grpcserver/pagination.go` | Add `streamCursor` struct + `encodeStreamCursor`/`decodeStreamCursor`. Keep existing `encodePageToken`/`decodePageToken` for `List*` offset pagination. |
| `internal/grpcserver/pagination_test.go` | Add round-trip tests for the new cursor type; existing offset-cursor tests stay as-is. |
| `internal/grpcserver/generic.go` | `StreamParams.QueryBatch` signature: change `afterID int` → `cursor streamCursor`. `StreamEntities` main loop: track `cursor` instead of `lastID int`. `SinceID`/`UpdatedSince` predicates still go BEFORE the ORDER BY (D-05) — no change in placement. |
| 13× `internal/grpcserver/<entity>.go` | Each has TWO `Order(ent.Asc(...))` sites (List + Stream). Each Stream handler's `QueryBatch` closure also needs the new keyset predicate (`WHERE (updated < cursor.Updated) OR (updated = cursor.Updated AND id < cursor.ID)`). |
| `internal/grpcserver/grpcserver_test.go:189` | Rewrite `"ordering by ID ascending"` subtest to assert `(-updated, -created, -id)` order; seed multiple rows with distinguishable timestamps. |
| `internal/grpcserver/grpcserver_test.go:1692+` | `seedStreamNetworks` and `TestStreamNetworks*` — current 3-row seed uses a single `time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)` for all rows. Update to spread timestamps across the 3 rows and assert the stream emits them in `(-updated)` order. |
| `internal/grpcserver/grpcserver_test.go:1950+` | `TestStreamNetworksSinceId` — re-verify resume semantics per D-05: resume from `since_id=N` means "rows with id > N in the NEW order." Assertions must be rewritten. |
| `internal/grpcserver/list_bench_test.go` | Benchmark is currently ordered by id ascending; retargeting to new default will shift perf numbers. Note baseline drift in commit message. |
| `internal/grpcserver/generic_test.go` | `TestListEntities` mocks `Query`; no ordering assumption. Likely no change unless new compound-cursor signature is plumbed through the mock. |
| `internal/grpcserver/ixlan_test.go` | Targeted tests for IxLan stream — audit for ordering assumptions. |
| `internal/grpcserver/organization_test.go` | Audit for ordering assumptions. |
| `internal/grpcserver/filter_test.go` | No direct ordering dependency; re-run to confirm. |

### entrest

| File | Change required |
|------|-----------------|
| `ent/schema/<entity>.go` × 13 | Add `entrest.WithSortable(true)` to the `updated` field declaration, plus `entrest.WithDefaultSort("updated")` + `entrest.WithDefaultOrder(entrest.OrderDesc)` in `Annotations()`. **Schema-generator template at `cmd/pdb-schema-generate/main.go:693-700` must also learn the new annotations** — otherwise the next `go generate ./schema` wipes them. |
| `cmd/pdb-schema-generate/main.go` | Update the entity template body (`func ({{.ModelName}}) Annotations()`) and the field-annotation builder (`fieldAnnotations` at line 429) to emit `WithSortable(true)` on `updated`. |
| `ent/rest/sorting.go` (generated) | Regenerated automatically via `go generate ./...` — will show new `DefaultField: "updated"` / `DefaultOrder: "desc"` on all 13 SortConfigs. |
| `cmd/peeringdb-plus/rest_test.go` | No known ordering assertions, but grep for `?sort=` and re-verify passing `?sort=id` still works (explicit override must still honour). |

### E2E / integration

| File | Change required |
|------|-----------------|
| `cmd/peeringdb-plus/e2e_privacy_test.go` | Multiple `?sort=` implicit assumptions around list bodies; audit and adjust any row-order assertions. |
| `cmd/peeringdb-plus/field_privacy_e2e_test.go` | Phase 64 helper calls `/rest/v1/ix-lans` (§1.3 entrest); audit response-shape assumptions. |
| `cmd/peeringdb-plus/privacy_surfaces_test.go` | Per CLAUDE.md Phase 64 § serializer list assertions may depend on position-0-is-public; verify. |

### CI drift check

CLAUDE.md § CI: "CI runs `go generate ./...` then fails if `ent/`, `gen/`, `graph/`, `internal/web/templates/` differ from committed files." Any hand-edit to a schema file must be mirrored in the generator template (or CI will never flag drift — but the NEXT schema regen will silently revert the change). Plan for BOTH.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go `testing` stdlib + `cmp.Diff` + custom `-update` flag for goldens |
| Config file | none; standard Go test discovery |
| Quick run command | `go test ./internal/pdbcompat/... ./internal/grpcserver/...` |
| Full suite command | `go test -race ./...` |
| Golden update | `go test -update ./internal/pdbcompat -run TestGoldenFiles` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| ORDER-01 | pdbcompat `/api/<type>` returns rows in `(-updated, -created, -id)` order for all 13 types | golden+unit | `go test ./internal/pdbcompat -run 'TestGoldenFiles\|TestRegistryFuncsOrdering'` | Golden: YES (regenerate); Unit: ❌ Wave 0 |
| ORDER-02 | grpcserver `List*` returns rows in new order; `Stream*` emits in new order with stable compound cursor across `SinceId`/`UpdatedSince` resume | unit | `go test ./internal/grpcserver -run 'TestList.*Ordering\|TestStream.*Ordering\|TestPageTokenRoundTrip\|TestStreamCursorRoundTrip'` | Partial — existing `"ordering by ID ascending"` must be rewritten; new `TestStreamCursorRoundTrip` needed |
| ORDER-03 | entrest `/rest/v1/<type>` default returns `(-updated)` order; explicit `?sort=id&order=asc` still honoured; other `?sort=` options remain sortable | integration | `go test ./cmd/peeringdb-plus -run 'TestREST.*Sort\|TestREST.*Ordering'` | ❌ Wave 0 |

### Dimension-8 validation signals

1. **Golden reorder audit (PRIMARY):** `git diff internal/pdbcompat/testdata/golden/` after regen shows only row-order changes (no field additions, removals, or renames). Requires multi-row fixture seeding (§5).
2. **Pagination stability (grpcserver):** unbounded streamed fetch `E_all`, then cursor-paged fetch `E_p1 ++ E_p2 ++ ... ++ E_pn`. Assert `E_all == concat(E_p*)` element-wise by `id`. Covers both `List*` offset cursor and `Stream*` compound cursor.
3. **Since predicate composition (D-05):** `since_id=N` returns exactly the rows with `id > N` in the new order; `updated_since=T` returns rows with `updated > T` in the new order. Assertion: for each filter, compare the emitted sequence to a reference slice computed in-test from the seed fixture.
4. **Concurrent-mutation cursor stability:** with a running stream mid-page (`n` rows emitted from page 1, cursor captured), mutate an already-emitted row's `updated` to a newer timestamp, then resume page 2 with the cursor. The mutated row MUST NOT reappear (would be a duplicate). This is the "monotonic id tiebreak" correctness claim from CONTEXT.md D-01; a test should pin it.
5. **Tie-break determinism:** two rows with byte-identical `(updated, created)` — assert output order is `DESC id`, not undefined. This is the §8 G-02 plan-choice tested.
6. **Cross-surface spot check:** one representative type (suggestion: `network` — already has the most test scaffolding) — assert the same 3+row seed produces the same row order through `/api/net`, `/rest/v1/networks`, and `ListNetworks` RPC. If the three diverge, it's a bug (Option A entrest compromise aside — REST may omit the `created` tiebreak; document if so).
7. **Grpc-total-count header unchanged (D-06):** assert header presence/absence under `SinceId==nil`/`SinceId!=nil` matches pre-phase behaviour.

### Sampling Rate

- **Per task commit:** `go test ./internal/pdbcompat/... ./internal/grpcserver/...`
- **Per wave merge:** `go test -race ./...`
- **Phase gate:** `go test -race ./... && go generate ./... && git diff --exit-code` (drift check)

### Wave 0 Gaps

- [ ] `internal/pdbcompat/golden_test.go` — extend `setupGoldenTestData` to seed multi-row fixtures for ≥4 types.
- [ ] `internal/pdbcompat/registry_funcs_ordering_test.go` — NEW unit coverage per §6 pdbcompat row, asserts row order directly from the ListFunc output.
- [ ] `internal/grpcserver/cursor_compound_test.go` — NEW round-trip and boundary tests for `streamCursor` encode/decode.
- [ ] `internal/grpcserver/stream_ordering_test.go` — NEW coverage for Stream* under multi-timestamp seed + concurrent-mutation scenario.
- [ ] `cmd/peeringdb-plus/rest_sort_e2e_test.go` — NEW entrest default-order coverage and explicit-`?sort=`-override coverage.
- [ ] Framework install: none — Go toolchain already required.

## Gotchas / open questions

### G-01 — entrest annotation API cannot express compound order (§3 primary)

CONTEXT.md D-02 calls a function signature that does not exist. The planner MUST choose between Option A (single-field `updated DESC`, acceptable REST-vs-pdbcompat divergence on collision), Option B (project-local entrest template override), or escalate to the user for a scope-adjustment decision.

### G-02 — No explicit tertiary tiebreak in CONTEXT.md

The locked decision says `(-updated, -created)`. With identical `(updated, created)` — common on bulk-synced data where the PeeringDB API echoes a single batch timestamp — SQLite's ORDER BY output is undefined. Recommendation: add `, -id` as the tertiary tiebreak at the ent layer on pdbcompat and grpcserver. This keeps output deterministic across replicas without touching the wire semantics. entrest Option A already has this implicitly because the Stream/List keyset uses `(updated, id)`, but the surface's returned row order for equal `updated` is NOT currently tiebroken by id — a plan task should add it.

### G-03 — No `updated` or `created` indexes exist on any schema

CONTEXT.md "Out of scope" bullet 3 claims "existing `updated` indexes on all 13 schemas (added v1.9 Phase 46)" — verified false. Phase 46 (`v1.11/46-search-compare-density`) was a UI-layer phase. Grep of every `func (<Type>) Indexes() []ent.Index` in `ent/schema/*.go` shows index fields on `asn`, `name`, `org_id`, `status`, and FK columns — never `updated` or `created`. Full-table sort by `updated` on a ~65k-row table (or whatever the upstream-parity replica size is — §docs/ARCHITECTURE.md) will cost O(n log n) on every unbounded list. Under the v1.15 Phase 65 256 MB replica budget (memory: ~58-59 MB steady per `memory/project_v1_milestone.md`) and v1.16 Phase 71's upcoming `limit=0` unbounded support, this is not free. The planner should consider whether to add `index.Fields("updated")` and `index.Fields("created")` to every `Indexes()` return. ent auto-migrate will create the index on next startup; runtime cost is one-time per replica on deploy.

Size check: per `memory/project_v1_milestone.md`, DB is 88 MB on LiteFS, steady replica heap ~59 MB — adding two b-tree indexes per 13 entities at ~65k rows/entity adds roughly 20-40 MB to the on-disk size. Within budget, but note it.

### G-04 — `cmd/pdb-schema-generate` template update required for D-02

Any hand-edit to `ent/schema/<entity>.go` `Annotations()` will be reverted the next time `cmd/pdb-schema-generate` runs (e.g., after a Phase 57-style upstream JSON re-capture). The schema-generator template MUST be updated in lock-step — same phase, same commit. CI drift check catches commit-time drift, NOT semantic drift where someone re-runs the generator later. Document this as a Plan step.

### G-05 — Keyset cursor correctness under mid-stream mutation

The claim "monotonic id tiebreaker" in CONTEXT.md D-01 is correct only for the ORDER BY clause — it does NOT protect against a mutation that changes `updated` for a row already emitted. Example: page 1 emits row X with `updated=T1`. After page 1 but before page 2, sync updates row X's `updated=T2` (T2 > T1). Page 2 resumes at cursor `(T1, X.id)` — the query `WHERE updated<T1 OR (updated=T1 AND id<X.id)` correctly excludes X from page 2. **But:** if the client then starts a fresh stream, X appears first under the new order. This is fine for sync semantics (no duplicates within a single stream), but a "replica view consistency" audit should pin it explicitly.

### G-06 — `depth>=1` nested `_set` ordering (D-04 said "unchanged")

Per D-04, nested `_set` fields (e.g. `network.poc_set` at `depth=2`) keep their current ordering. These are eagerly-loaded ent edges (`edge.To(...).Annotations(entrest.WithEagerLoad(true))` pattern). The eagerload path in entrest (`templates/eagerload.tmpl:29`) calls `applySorting<Type>(e, <sortField>, <defaultOrder>)` — which means adding `WithDefaultOrder(OrderDesc)` at the schema level will ALSO apply to nested `_set` arrays. This contradicts D-04. Either:
- Option 1: accept that nested `_set` arrays are also reordered under the new default (probably desirable — upstream behaves this way on depth>=1 too).
- Option 2: override eager-load ordering per-edge via `entrest.WithEagerLoad` options (check entrest docs for a per-edge sort override).

**Recommendation:** Accept Option 1; flip the D-04 scope to "nested `_set` fields follow the same ordering as top-level lists." Confirm with product owner during planning kick-off. If Option 2 is mandated, plan an extra audit task on the eager-load templates.

### G-07 — GraphQL (out of scope) is genuinely out of scope

Relay connection ordering is governed by its own `edges { node }` sort, parameterised by `OrderField` annotations on fields (grep: 6 schemas have `entgql.OrderField("NAME")` on the `name` field). The phase does not need to touch the GraphQL layer. Confirmed OUT OF SCOPE in CONTEXT.md.

### G-08 — Proto `page_token` wire type clarification

CONTEXT.md D-01 says cursor is "opaque `bytes`". The actual proto type is `string` (see `proto/peeringdb/v1/services.proto` lines 31/52/98/115/... for all 26 positions). Base64-encoded string is effectively opaque; the D-01 conclusion ("no proto regen required") is correct, the characterization of the type is not. Update the decision narrative when finalising the plan.

### G-09 — Bench impact drift in CI

`internal/grpcserver/list_bench_test.go` runs a `BenchmarkList*` harness seeded by `seedBenchNetworks` (id-ordered 1..n). Under the new compound order without an `updated` index (see G-03), the benchmark will regress measurably — the seed all shares one timestamp, so every comparison goes to the secondary/tertiary tiebreak. If bench comparison is used in CI gating (`go test -bench` + `benchstat`), call out the expected regression in the commit message or the benchmark will false-positive.

## Environment Availability

Not applicable — this phase is pure code/config change. No external dependencies.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Upstream PeeringDB observable row order for identical `(updated, created)` is `id DESC` (§G-02 recommendation) | §G-02, §2.1, §2.2 | Output diverges subtly from upstream on collision rows; detected by Phase 72 parity tests |
| A2 | Nested `_set` ordering should follow the new default (Option 1 in G-06) | §G-06 | D-04 literal interpretation violated; product owner must sign off |
| A3 | Option A (single-field entrest default) is acceptable for ORDER-03 despite the `created` tiebreak divergence from pdbcompat/grpcserver | §3 | If byte-identical cross-surface ordering is mandated, plan must add an Option B template-override task |
| A4 | Streamed fetch under concurrent mutation is allowed to skip rows whose `updated` moved forward mid-stream (normal read-committed semantics) | §G-05 | Downstream client expects strict snapshot semantics — rare for a mirror, document clearly |
| A5 | Adding `index.Fields("updated")` to all 13 schemas is within the phase's scope if perf requires it (G-03) | §G-03 | If treated as out-of-scope, `limit=0` streaming in Phase 71 will face a 3-5× regression on cold sort |

## Sources

### Primary (HIGH confidence)

- `github.com/lrstanley/entrest@v1.0.3` module cache `annotations.go`, `schema_sorting.go`, `templates/sorting.tmpl`, `templates/list.tmpl`, `templates/eagerload.tmpl`, `docs/content/docs/openapi-specs/annotation-reference.mdx` — direct source read, compound-order API absence confirmed.
- `internal/pdbcompat/registry_funcs.go` — all 13 order-site line numbers grep-confirmed.
- `internal/grpcserver/*.go` — all 26 order-site line numbers grep-confirmed.
- `internal/grpcserver/pagination.go`, `generic.go` — full file read.
- `ent/schema/*.go` — 13 entity audit for `Annotations()`, `Indexes()`, `field.Time("updated")`, `field.Time("created")`.
- `ent/entc.go` — codegen extension list.
- `cmd/pdb-schema-generate/main.go` lines 693-700 — regeneration template for `Annotations()`.
- `proto/peeringdb/v1/services.proto` — cursor wire type (`string`, not `bytes`).
- `internal/testutil/seed/seed.go` line 18 — `Timestamp` = single time.

### Secondary (MEDIUM confidence)

- CONTEXT.md D-01..D-06 (user-locked) — decisions accepted; corrections noted for A-shape (D-02 function signature, D-01 proto wire type).
- `CLAUDE.md` §Code Generation — regeneration-strip precedent from entproto.

### Tertiary (LOW confidence)

- Real PeeringDB upstream data cardinality (~65k rows) — cited from `memory/project_v1_milestone.md`; not verified against a current sync snapshot.

## Metadata

**Confidence breakdown:**
- Current-state code inventory: HIGH (direct grep + file reads)
- entrest API limitations: HIGH (module-cache source read)
- Cursor migration shape: HIGH (full pagination.go + generic.go read)
- Golden regeneration mechanics: HIGH (full golden_test.go read)
- Test impact: MEDIUM (representative files audited; a few per-entity Stream tests not individually read)
- Perf impact / G-03: MEDIUM (index-absence confirmed; ~65k-row claim is cited not measured)

**Research date:** 2026-04-19
**Valid until:** 2026-05-19 (stable area; revisit only if entrest minor-version bump or ent major-version bump)

## RESEARCH COMPLETE
