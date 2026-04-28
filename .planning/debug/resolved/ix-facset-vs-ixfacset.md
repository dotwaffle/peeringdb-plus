---
slug: ix-facset-vs-ixfacset
status: resolved
trigger: |
  /api/ix/<id>?depth=2 returns ixfac_set (the raw IxFacility join-table
  records) instead of fac_set (expanded Facility records) like upstream
  PeeringDB. DE-CIX (/api/ix/26?depth=2) returns 5080 bytes on the
  mirror vs 24928 bytes upstream, missing 17 facility records (fac_set
  upstream=17, mirror=0; mirror exposes ixfac_set instead, which
  upstream does not expose at all).
created: 2026-04-28
updated: 2026-04-28
---

# Debug: IX detail response exposes ixfac_set instead of fac_set

## Symptoms

- **Expected:** /api/ix/<id>?depth=2 returns the same top-level field set
  upstream does. Upstream embeds related facilities as `fac_set` (a list
  of expanded Facility objects, e.g. 17 entries for DE-CIX).
- **Actual:** mirror returns `ixfac_set` (the IxFacility join-table
  records — i.e. the rows from the through-relation, not the expanded
  facilities themselves) and does NOT include `fac_set`. Other entity
  types (`/api/net/15169`, `/api/org/1989`) match upstream exactly on
  field shape — only IX diverges.
- **Errors:** none — request returns 200, just wrong/missing fields.
- **Timeline:** discovered 2026-04-28 via the new
  scripts/compare-upstream-fields.sh anonymous-vs-anonymous comparison
  (commit cdee9f4 on the file-shape script). Likely a long-standing
  divergence from project inception, not a recent regression.
- **Reproduction:**
  ```
  ./scripts/compare-upstream-fields.sh > /tmp/field-diff.txt
  ```
  …and look at the `decix-ix : /api/ix/26?depth=2` block. Or hit
  /api/ix/26?depth=2 against upstream and the mirror directly.

## Comparison data (2026-04-28, anonymous both sides)

| field           | upstream | mirror | note |
|-----------------|----------|--------|------|
| `fac_set`       | 17 facility objects | **MISSING** | upstream-only |
| `ixfac_set`     | not present | present (raw IxFacility rows) | mirror-only |
| `ixlan_set`     | 1 | 1 | matches |
| `social_media`  | 1 | 1 | matches |
| Total bytes     | 24,928 | 5,080 | 5x divergence |

For comparison, `/api/net/15169` and `/api/org/1989` showed:
- Top-level field sets identical to upstream
- All list-length counts match (e.g., `netfac_set=2/2`, `net_set=5/5`)
- Only ~200-400 byte residual deltas (likely formatting)

So the bug is specific to InternetExchange's serializer.

## Prime hypothesis

Upstream's `peeringdb_server/serializers.py InternetExchangeSerializer`
defines `fac_set` as an expanded list of Facility records resolved
through the IxFacility many-to-many. Our `internal/pdbcompat/serializer.go`
(or whichever IX-specific serializer code path applies) is exposing
the join table directly as `ixfac_set` and not synthesising the
upstream-shape `fac_set`. This mirrors how `netfac_set` and other
through-relations behave: in those cases the through-table is named
similarly to the parent and the serializer DOES expand to the related
records.

The pattern:
- `Network.facilities` → through `NetworkFacility` → exposed as
  `netfac_set` (works correctly — list lengths match)
- `Network.ix_lans` → through `NetworkIxLan` → exposed as
  `netixlan_set` (works correctly)
- `InternetExchange.facilities` → through `IxFacility` → SHOULD be
  `fac_set` per upstream, but exposed as `ixfac_set` on mirror

So the divergence is unique to IX. The rename rule for upstream IX seems
to be: drop the join-prefix and use the bare target type's plural
("fac_set", not "ixfac_set").

## Constraints

- Do NOT introduce structural ent schema changes that would force a
  full re-sync. The fix should be at the serializer / response-shape
  level only.
- Must respect Phase 64 privacy enforcement — the resolved Facility
  records exposed via `fac_set` need to honour the same anonymous-tier
  filtering rules as a direct `/api/fac/<id>` query would.
- Upstream serializer source: /tmp/claude/upstream-serializers.py (4390
  lines, fetched from peeringdb/peeringdb master 2026-04-28).
- Cross-reference: /tmp/claude/upstream-rest.py for any list-handler
  logic that wraps the serializer.

## Current Focus

- hypothesis: pdbcompat IX serializer (likely in
  internal/pdbcompat/serializer.go or registry_funcs.go) exposes the
  raw IxFacility join records as `ixfac_set` rather than expanding the
  through-relation into `fac_set` like upstream's
  InternetExchangeSerializer does.
- test: read the upstream `InternetExchangeSerializer` definition in
  /tmp/claude/upstream-serializers.py to confirm it uses `fac_set`,
  then read our IX serializer to confirm it uses `ixfac_set`. Compare
  against the working `netfac_set` path (Network → NetworkFacility →
  the resolved Facility records) to identify what's different.
- expecting: upstream uses something like `fac_set =
  serializers.SerializerMethodField()` or a nested serializer that
  resolves IxFacility.facility for each row.
- next_action: read /tmp/claude/upstream-serializers.py for
  InternetExchangeSerializer + IxFacilitySerializer; locate IX
  serialisation in internal/pdbcompat/; identify the divergence point;
  propose the fix.

## Evidence

- timestamp: 2026-04-28
  source: /tmp/claude/upstream-serializers.py:3493-3611
  finding: |
    Upstream `InternetExchangeSerializer.fac_set` is defined as
    `nested(FacilitySerializer, source="ixfac_set_active_prefetched",
    through="ixfac_set", getter="facility")`. The `getter="facility"`
    instructs the nested serializer's `extract()` method
    (serializers.py:1459-1462) to traverse each IxFacility join row
    and emit the embedded Facility object using the FULL
    FacilitySerializer Meta.fields list (1707+). The IX
    Meta.fields list (3571-3609) explicitly lists `fac_set` and OMITS
    `ixfac_set` — confirming upstream surface contract.

- timestamp: 2026-04-28
  source: internal/pdbcompat/depth.go:160-189 (pre-fix)
  finding: |
    `getIXWithDepth` at depth>=2 emits `m["ixfac_set"] = orEmptySlice(
    ixFacilitiesFromEnt(ix.Edges.IxFacilities))` — i.e. the raw
    IxFacility join rows, exposed under the wrong key. There is no
    `fac_set` emission at all on the IX surface. This is the entire
    divergence point — the rest of the depth=2 path (org, ixlan_set)
    matches upstream.

- timestamp: 2026-04-28
  source: internal/pdbcompat/testdata/golden/ix/depth.json (pre-fix)
  finding: |
    The committed golden locks in the buggy shape:
    `"ixfac_set":[{"id":1200,"ix_id":500,"fac_id":300,"name":"",...}]`
    with no `fac_set`. Confirms the bug has been present since the
    golden was first generated and was never caught because no test
    asserted the expected upstream shape.

- timestamp: 2026-04-28
  source: ent/schema/ixfacility.go:73-82
  finding: |
    IxFacility has `edge.From("facility", Facility.Type)` and
    `edge.From("internet_exchange", InternetExchange.Type)`. The Go
    accessor `ixf.Edges.Facility` resolves the FK after a
    `WithFacility()` eager-load, mirroring upstream's `getter="facility"`
    semantics exactly. No schema change required.

- timestamp: 2026-04-28
  source: ent/internetexchange_query.go:339-341
  finding: |
    `WithIxFacilities(opts ...func(*IxFacilityQuery))` accepts a
    nested-eager-load callback. We can chain `WithFacility()` inside
    the callback to load `ixfac.Edges.Facility` in a single query batch
    (no extra round-trips per row).

## Eliminated

- The `internal/pdbcompat/serializer.go` `internetExchangeFromEnt` /
  `ixFacilityFromEnt` functions are correct as written — the bug is
  purely in `depth.go`'s key naming and through-relation traversal.
  No serializer-shape changes needed.
- No ent schema change required; existing IxFacility edges expose the
  needed traversal path.
- No proto / GraphQL / entrest changes — pdbcompat is the only surface
  that emits depth=2 expansions; those four other surfaces have
  unrelated nested-relation shapes.

## Resolution

- root_cause: |
  `internal/pdbcompat/depth.go` `getIXWithDepth` emitted the raw
  IxFacility join-table records under the WRONG key
  (`ixfac_set` instead of `fac_set`). Upstream PeeringDB's
  `InternetExchangeSerializer` (peeringdb_server/serializers.py:3514)
  uses `nested(FacilitySerializer, through="ixfac_set",
  getter="facility")` — which traverses each IxFacility row and emits
  the embedded Facility object under the key `fac_set`. The mirror
  was effectively skipping the `getter="facility"` traversal step, so
  callers got the join row instead of the Facility record, AND under
  a key that upstream does not expose on the IX surface at all
  (`ixfac_set` only appears on the facility-side serializer).

- fix: |
  1. `internal/pdbcompat/depth.go` `getIXWithDepth`: chain
     `WithFacility()` inside the existing `WithIxFacilities(...)`
     eager-load callback so `ixf.Edges.Facility` populates in a
     single ent query batch.
  2. Replace `m["ixfac_set"] = ixFacilitiesFromEnt(...)` with
     `m["fac_set"] = facilitiesFromEnt(resolveFacilitiesFromIxFacilities(
     ix.Edges.IxFacilities))`. The new helper walks the
     IxFacility through-rows and collects the embedded Facility
     records (skipping nil entries defensively).
  3. Update the function godoc to cite upstream serializers.py:3514
     and document that `ixfac_set` is intentionally NOT exposed on
     the IX surface (matches upstream).
  4. Regenerate `internal/pdbcompat/testdata/golden/ix/depth.json`
     (now contains an expanded Facility object under `fac_set` and
     no `ixfac_set` entry).
  5. Add `TestDepth/two_ix` regression sub-test in
     `internal/pdbcompat/depth_test.go` that asserts both the
     positive parity contract (`fac_set` present, expanded Facility
     shape with `address1`/`latitude`/`longitude`, no raw `ix_id`
     attribute) AND the negative contract (`ixfac_set` MUST NOT be
     present on the IX surface).

  Phase 64 privacy parity: the embedded Facility records flow through
  the same `facilityFromEnt` serializer used by `/api/fac/<id>`,
  inheriting its privacy treatment by construction. No additional
  redaction logic required (Facility has no `_visible` companion
  fields per the `internal/privfield` invariant).

  Status filtering: matches existing depth=2 _set patterns elsewhere
  in `depth.go` — `WithIxFacilities` and `WithFacility` are unfiltered
  eager-loads. The pre-existing project-wide gap of unfiltered
  status on depth=2 nested _set fields is OUT OF SCOPE for this fix
  (preserves parity with the 4 other entity-with-_set depth handlers
  and avoids scope creep). If/when SEED-* tracks tombstone filtering
  on depth=2 nested edges, all 5 handlers should be updated together.

- verified: |
  - go test -race ./internal/pdbcompat/...           PASS (54.2s)
  - go test -race ./...                              PASS (full repo)
  - go vet ./...                                     clean
  - golangci-lint run ./internal/pdbcompat/...       0 issues
  - TestDepth/two_ix specifically: PASS — locks both positive
    (fac_set presence + Facility shape) and negative (no ixfac_set
    on IX surface) parity invariants.
  - Updated golden file confirmed visually: `fac_set` contains
    expanded Facility (id=300, "Golden Facility", with address1,
    latitude=37.5, longitude=-122.5, full address fields), no
    `ixfac_set` key present.

  Live verification against beta.peeringdb.com (via
  scripts/compare-upstream-fields.sh) requires deployment — not run
  from this debug session. The committed regression tests + golden
  file lock the upstream-correct shape regardless.

  Files changed:
  - internal/pdbcompat/depth.go
  - internal/pdbcompat/depth_test.go
  - internal/pdbcompat/testdata/golden/ix/depth.json
