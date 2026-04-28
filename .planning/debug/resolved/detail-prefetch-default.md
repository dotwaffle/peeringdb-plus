---
gsd_debug_version: 1.0
slug: detail-prefetch-default
status: resolved
trigger: "pdbcompat detail endpoints (/api/<type>/<id>) are missing embedded related collections + parent org at depth=0; upstream embeds them on every detail request because rest.py:750 fires prefetch_related when self.kwargs is non-empty (always true for single-id) — same class of bug as 0d39654 but applied to all entity types"
created: 2026-04-28
updated: 2026-04-28
---

# Debug Session: detail-prefetch-default

## Symptoms

**Expected behavior:**
`GET /api/<type>/<id>` (single-id detail, no explicit `?depth=`) should embed:
- Parent `org` object (for `net`, `fac`, `ix`, `carrier`, `campus`)
- Related collections via through-relations:
  - `net` → `netfac_set` (NetworkFacility), `netixlan_set` (NetworkIxLan), `poc_set` (Poc)
  - `org` → `net_set`, `fac_set`, `ix_set`, `carrier_set`, `campus_set`
  - `ix` → `fac_set` (already fixed at depth=2 in 0d39654, must also fire at depth=0), `ixlan_set`
  - `fac` → `net_set`, `ix_set`
  - `carrier` → `fac_set`
  - `campus` → `fac_set`

This must match upstream PeeringDB's anonymous-tier response shape exactly (modulo Phase 64 anonymous-tier privacy filtering, which already correctly hides `visible="Users"` POCs).

**Actual behavior:**
Mirror returns the bare row only. Concrete observation:

```
==== /api/net/15169 (single-id) ====
  upstream  /api/net/15169                 200 rows=1 bytes=2026
  mirror    /api/net/15169                 200 rows=1 bytes=1014

ONLY-UPSTREAM: netfac_set     (2 expanded NetworkFacility records embedded)
ONLY-UPSTREAM: netixlan_set   (list of NetworkIxLan records)
ONLY-UPSTREAM: org            (fully embedded Organization object)
ONLY-UPSTREAM: poc_set        (empty for Google anonymous — visibility=Users)

Top-level key count:  upstream=45  mirror=41
```

**Error messages:**
None — no HTTP error, no panic, no log warning. Pure data-shape divergence.

**Timeline:**
Always present. This code path has never called `prefetch_related` for bare detail URLs. The depth=2 path was extended in commit `0d39654` (IX `fac_set` fix) but the depth=0-detail code path was not adjusted.

**Reproduction steps:**
```bash
# Quick proof:
curl -fsS https://www.peeringdb.com/api/net/15169 | jq '.data[0] | keys' > /tmp/up.txt
curl -fsS https://peeringdb-plus.fly.dev/api/net/15169 | jq '.data[0] | keys' > /tmp/mr.txt
diff /tmp/up.txt /tmp/mr.txt
# upstream has 4 more keys: netfac_set, netixlan_set, org, poc_set

# Full sweep:
./scripts/compare-upstream-fields.sh > /tmp/field-diff.txt
# All entity types likely have similar gaps.
```

## Upstream root cause (already located)

`peeringdb_server/rest.py:750` (cached at `/tmp/claude/upstream-rest.py`; refetch from
`https://raw.githubusercontent.com/peeringdb/peeringdb/master/src/peeringdb_server/rest.py`):

```python
if depth > 0 or self.kwargs:
    return self.serializer_class.prefetch_related(qset, self.request, is_list=...)
else:
    return qset
```

`self.kwargs` is non-empty for ANY detail (single-id) request (it carries the URL `pk`). So upstream's matrix is:

| URL shape                          | prefetch_related | embedded sets |
|------------------------------------|------------------|---------------|
| `/api/net` (list, depth=0)         | no               | no            |
| `/api/net?depth=2` (list, d=2)     | yes              | yes           |
| **`/api/net/15169` (detail, d=0)** | **yes**          | **yes**       |
| `/api/net/15169?depth=2`           | yes              | yes           |

Our mirror runs the prefetch chain only when `?depth=2` is explicit. The detail-depth=0 path goes through a non-prefetch branch.

## Code map (entry points to read)

1. `internal/pdbcompat/depth.go` — `getIXWithDepth` (already extended in 0d39654 with `WithFacility()` chain inside `WithIxFacilities(...)`); presumably also has `getNetWithDepth`, `getOrgWithDepth`, etc., or needs them. Locate the calling site that decides whether to call `getXWithDepth(2)` vs returning the bare row.
2. `internal/pdbcompat/handler.go` — top-level pdbcompat HTTP dispatch. Find detail-vs-list branch and the depth=0-vs-explicit-depth condition. The fix likely needs to make `getXWithDepth` (or equivalent prefetch path) fire unconditionally for detail requests, mirroring upstream's `if depth > 0 or self.kwargs:`.
3. `internal/pdbcompat/registry_funcs.go` — list-side context, probably not the bug site but useful for shape parity.
4. `internal/pdbcompat/serializer.go` — `*FromEnt(ctx, …)` translation. May need extending if any entity's embedded `_set` shape doesn't already exist (most do, since depth=2 already produces them).
5. `internal/pdbcompat/depth_test.go` — has `TestDepth/two_ix` regression sub-test (the canonical pattern from 0d39654). Mirror that pattern for new `zero_*_detail` sub-tests.
6. `internal/pdbcompat/testdata/golden/` — `ix/depth.json` golden exists; extend or add per-entity goldens for the new behaviour.
7. `internal/peeringdb/types.go` — canonical Go struct shapes; `NetFacSet`, `NetIxLanSet` etc. should already exist since they fire at depth=2.
8. Upstream cross-check: `peeringdb_server/serializers.py` (cached at `/tmp/claude/upstream-serializers.py`). Each `*Serializer.Meta.fields` enumerates the `_set` collections to embed for that type.

## Canonical worked example

Commit `0d39654` (and its writeup at `.planning/debug/resolved/ix-facset-vs-ixfacset.md`) is the same class of bug: IX detail at `?depth=2` returned `ixfac_set` (raw join rows) instead of upstream's `fac_set` (expanded Facility). Fix: chain `WithFacility()` inside `WithIxFacilities(...)`, add `resolveFacilitiesFromIxFacilities` helper, replace `ixfac_set` emission with `fac_set`. Test pattern: `TestDepth/two_ix` with positive (Facility shape) and negative (no `ixfac_set`) assertions.

This session's fix should follow the same pattern but extend it across **all** detail endpoints AND make it fire at depth=0 (not just depth=2).

## Constraints

1. **No ent schema changes.** Fix must be at serializer / response-shape level. Schema changes would force a full re-sync.
2. **Phase 64 privacy must hold.** Embedded `poc_set` items must respect anonymous-tier filtering (`poc.visible="Users"` rows hidden from anonymous callers). The IX fix preserved this; mirror that path. (Why Google's `poc_set` is empty at anonymous tier — correct behaviour.)
3. **Phase 71 response-memory budget.** Detail-endpoint expansions can blow up payload size (`/api/org/<id>` with all child net/fac/ix/etc. embedded could be many MiB). Verify detail responses go through `CheckBudget` pre-flight or are otherwise naturally bounded. Read `internal/pdbcompat/budget.go` and `cmd/peeringdb-plus/main.go` middleware chain.
4. **No N+1 queries.** Eager-load via ent `With*` chain inside `getXWithDepth`, exactly as the IX fix did with `WithFacility()` inside `WithIxFacilities(...)`.
5. **Don't touch:** `CLAUDE.md`, `ent/schema/*`, `scripts/compare-upstream-{parity,fields}.sh`.

## Verification target

Post-fix `./scripts/compare-upstream-fields.sh` should show `(top-level keys identical)` for `google-net` block plus parallel cleanliness for the `arin-org` and `decix-ix` blocks. Bare URL test (`./scripts/compare-upstream-parity.sh`) should also show `/api/net/15169` byte counts within ~50 bytes of upstream (residual ~200-byte deltas across all entities — number precision / omitempty / whitespace — are cosmetic and out of scope for this session).

## Workflow knobs in scope

- Run locally first: `go build ./...` → `go test -race ./internal/pdbcompat/...` → `golangci-lint run ./...`
- Commit with the same message style as `0d39654` (long-form, root cause cited, verifications listed).
- Push (requires `dangerouslyDisableSandbox: true` for the Bash call — SSH keys aren't reachable inside the sandbox). User redeploys.
- Re-run both comparison scripts against fly.dev.

## Current Focus

```yaml
hypothesis: |
  internal/pdbcompat/handler.go (or depth.go calling site) gates the prefetch chain on
  `depth > 0` only, instead of upstream's `depth > 0 OR is_detail_request`.
  Bare detail URLs (`/api/<type>/<id>`) hit the non-prefetch branch and emit only the
  raw row, dropping the per-type `_set` embeddings + parent `org` object that upstream
  always emits when serving a single-id request.
test: |
  Read internal/pdbcompat/handler.go and depth.go to find the depth-vs-detail dispatch.
  Confirm the conditional that omits prefetch_related for depth=0 + detail.
  Then verify by tracing a /api/net/15169 request through the dispatch tree.
expecting: |
  A single conditional like `if depth > 0 { ... } else { return bareRow }` (or similar)
  that needs to become `if depth > 0 || isDetail { ... }` matching upstream rest.py:750.
next_action: |
  Spawn gsd-debugger to read handler.go + depth.go, locate the depth-vs-detail branch,
  confirm the missing OR-branch, then propose the minimal fix that extends the existing
  getXWithDepth(2) call site to cover all entity types at depth=0 for detail requests.
reasoning_checkpoint: ""
tdd_checkpoint: ""
```

## Evidence

- timestamp: 2026-04-28 prior-session: ran `./scripts/compare-upstream-fields.sh` after deploying `0d39654`. Result: `/api/net/15169` mirror=1014 bytes vs upstream=2026 bytes; 4 missing top-level keys (`netfac_set`, `netixlan_set`, `org`, `poc_set`).
- timestamp: 2026-04-28 prior-session: located upstream gating logic at `peeringdb_server/rest.py:750` — `if depth > 0 or self.kwargs:` calls `prefetch_related`. `self.kwargs` is always non-empty on detail requests.
- timestamp: 2026-04-28 prior-session: confirmed `0d39654` (IX `fac_set` at `?depth=2`) is the canonical worked example; pattern is `WithChild()` chain inside `WithJoinTable(...)` + serializer extraction helper.
- timestamp: 2026-04-28 this-session: read `peeringdb_server/serializers.py:789-823` (depth_from_request + default_depth + max_depth). Confirmed `default_depth(is_list=False) = 2`; `default_depth(is_list=True) = 0`. So upstream's effective behaviour for a bare `/api/<type>/<id>` detail URL is `depth=2`, NOT `depth=0` — depth=2 triggers the prefetch loop; depth=0 short-circuits at `prefetch_related` line 852 (`if depth <= 0: return qset`).
- timestamp: 2026-04-28 this-session: empirically verified upstream behaviour. `curl https://www.peeringdb.com/api/net/15169` returns 45 keys including `netfac_set`/`netixlan_set`/`org`/`poc_set`. `curl https://www.peeringdb.com/api/net/15169?depth=0` returns 41 keys with NO `_set` fields and no expanded `org` — proving `?depth=0` is the bare-row escape hatch and the bare URL defaults to `depth=2`.
- timestamp: 2026-04-28 this-session: located the bug at `internal/pdbcompat/handler.go:328` (`depth := 0`) — the default for detail requests was 0, so `tc.Get(ctx, client, id, 0)` invoked the non-prefetch branch in every `getXWithDepth`. The depth-call sites in `depth.go` already had the correct `if depth >= 2` prefetch chains; only the default needed flipping.

## Eliminated

- Schema/serializer changes — none required. Existing `getXWithDepth` family already implements the prefetch chains correctly for all 13 entity types; only the default depth selection in the HTTP dispatch was wrong.
- Per-entity work — bug was a single conditional in `serveDetail`, not a per-entity rewrite. Generalises commit `0d39654` (which fixed only the IX `fac_set` shape at explicit `?depth=2`) by extending the prefetch trigger to every detail URL via the depth default flip.
- Response-budget concerns — `serveDetail` does not call `CheckBudget` (only `serveList` does). Upstream similarly has no per-detail size limit. Detail responses can grow with depth-2 expansion (e.g. an org with hundreds of nets) but this matches upstream parity by design. Out of scope for this fix.
- Phase 64 privacy — embedded sets flow through their respective `*FromEnt` serializers which already honour anonymous-tier filtering (POC `visible="Users"` rows hidden, `ixf_ixp_member_list_url` redaction). No new privacy paths introduced.

## Resolution

- root_cause: |
    `internal/pdbcompat/handler.go` `serveDetail` defaulted `depth := 0` when no `?depth=` query
    param was supplied. Each `getXWithDepth(ctx, client, id, depth)` only fires the prefetch chain
    `if depth >= 2`, so bare detail URLs went through the non-prefetch branch and returned the bare
    row — dropping every `_set` collection (`net_set`, `fac_set`, `ix_set`, `carrier_set`,
    `campus_set`, `poc_set`, `netfac_set`, `netixlan_set`, `ixlan_set`, `ixpfx_set`,
    `carrierfac_set`) and every parent FK expansion (`org`, `campus`, `ix`, `ixlan`, `net`, `fac`,
    `carrier`).

    Upstream's `peeringdb_server/serializers.py:default_depth(is_list=False)` (line 823) returns 2
    for single-object GETs versus 0 for lists. Combined with the `prefetch_related` gating at
    `rest.py:750` (`if depth > 0 or self.kwargs:`) and the `prefetch_related` early-return at line
    852 (`if depth <= 0: return qset`), this means:

    | URL                           | upstream effective depth | embeds sets? |
    |-------------------------------|--------------------------|--------------|
    | `/api/net` (list)             | 0 (default_depth list)   | no           |
    | `/api/net?depth=2` (list)     | 2                        | yes          |
    | `/api/net/15169` (detail)     | 2 (default_depth detail) | **yes**      |
    | `/api/net/15169?depth=0`      | 0 (explicit override)    | no           |
    | `/api/net/15169?depth=2`      | 2                        | yes          |

    The mirror's depth-handling matrix was off by one row: bare detail URLs hit the depth=0 path
    instead of the depth=2 path. Generalisation of the IX `fac_set` regression class fixed at
    commit `0d39654` (which only addressed the `?depth=2` IX-specific shape) — same root-cause
    family, broader symptom domain.

- fix: |
    1. `internal/pdbcompat/handler.go` `serveDetail`: change `depth := 0` to `depth := 2` as the
       default. Explicit `?depth=0` and `?depth=2` query params still parse and override correctly.
       Added a docstring citing `serializers.py:817-823` (`default_depth`) and `rest.py:750`
       (`prefetch_related` gating) so the next reader has the upstream provenance inline.
    2. `internal/pdbcompat/depth_test.go`: split the existing `TestDepth/zero` sub-test (which
       locked in the buggy bare-row behaviour for default detail URLs) into two upstream-parity
       sub-tests:
         - `TestDepth/explicit_depth_zero`: asserts `?depth=0` still returns the bare row (matches
           upstream `prefetch_related` line 852 short-circuit).
         - `TestDepth/default_detail_uses_depth_two`: asserts a bare `/api/org/<id>` URL now
           embeds all five `_set` fields (matches upstream `default_depth(is_list=False)=2`).
       Added a third sub-test `TestDepth/default_detail_net_uses_depth_two` that locks the four
       specific keys this debug session was opened to fix (`poc_set`, `netfac_set`, `netixlan_set`,
       `org`) on bare `/api/net/<id>`.
    3. Regenerated all 13 `internal/pdbcompat/testdata/golden/<type>/detail.json` files via
       `go test -update -run TestGoldenFiles ./internal/pdbcompat/`. Each `detail.json` now
       contains the depth=2 expansion shape and is byte-identical to its sibling `depth.json` —
       confirming the new behaviour matches upstream's "bare detail = depth=2" semantic.

    Phase 64 privacy parity: embedded sets continue to flow through their respective `*FromEnt`
    serializers, which already honour anonymous-tier filtering (POC `visible="Users"` rows hidden,
    `ixf_ixp_member_list_url` redacted via `internal/privfield`). No new privacy paths introduced
    — the fix only flips when prefetch fires, not what the serializer renders.

    Response-budget interaction: `serveDetail` does not call `CheckBudget` (only `serveList`
    does) — matches upstream which similarly has no per-detail size limit. Detail responses can
    grow with depth=2 expansion but this is upstream-parity behaviour by design. Future work
    (out of scope here) could add a per-detail soft cap if operators observe pathological cases.

- verified: |
    - `go build ./...`                                    PASS (clean)
    - `go test -race ./internal/pdbcompat/...`            PASS (54.5s, all sub-tests)
    - `go test -race ./...`                               PASS (full repo, no regressions)
    - `go vet ./...`                                      PASS (clean)
    - `golangci-lint run ./...`                           PASS (0 issues)
    - `go generate ./...`                                 PASS (no codegen drift)
    - `TestDepth/explicit_depth_zero`                     PASS (asserts ?depth=0 returns bare row)
    - `TestDepth/default_detail_uses_depth_two`           PASS (asserts bare /api/org/<id> embeds all 5 _set fields)
    - `TestDepth/default_detail_net_uses_depth_two`       PASS (asserts bare /api/net/<id> embeds poc_set/netfac_set/netixlan_set/org)
    - `TestGoldenFiles` (84 sub-tests)                    PASS
    - All 13 `testdata/golden/<type>/detail.json` are now byte-identical to their `depth.json` siblings — independent corroboration that `default_detail` and `?depth=2` produce the same shape.

    Live verification against beta.peeringdb.com requires deployment.
    Post-deploy verification command (per session orchestrator notes):
      ./scripts/compare-upstream-fields.sh
    Expected post-deploy outcome: top-level keys identical for `google-net`, `arin-org`, `decix-ix` blocks; residual ~200-byte deltas remain (number precision / omitempty / whitespace — out of scope per session brief).

    Files changed:
    - internal/pdbcompat/handler.go (default depth for detail: 0 → 2)
    - internal/pdbcompat/depth_test.go (zero sub-test split + new default_detail_* sub-tests)
    - internal/pdbcompat/testdata/golden/{org,net,ix,fac,carrier,campus,ixlan,ixfac,netfac,netixlan,poc,ixpfx,carrierfac}/detail.json (13 files; regenerated to match upstream-parity depth=2 shape)
