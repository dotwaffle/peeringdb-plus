---
phase: 64-field-level-privacy
plan: 03
subsystem: privacy
tags: [privacy, serializer, e2e, graphql, grpc, rest, pdbcompat]
requires:
  - 64-01-privfield-helper
  - 64-02-schema-sync-wiring
provides:
  - field-level-privacy-enforcement-at-5-surfaces
  - TestE2E_FieldLevel_IxlanURL_{RedactedAnon,VisibleToUsersTier}
  - restFieldRedactMiddleware
  - graph.IxLanResolver.IxfIxpMemberListURL
affects:
  - internal/pdbcompat/serializer.go
  - internal/pdbcompat/depth.go
  - internal/pdbcompat/registry_funcs.go
  - internal/pdbcompat/serializer_test.go
  - internal/grpcserver/ixlan.go
  - internal/grpcserver/ixlan_test.go
  - graph/gqlgen.yml
  - graph/generated.go
  - graph/schema.resolvers.go
  - graph/custom.resolvers.go
  - cmd/peeringdb-plus/main.go
  - cmd/peeringdb-plus/e2e_privacy_test.go
  - cmd/peeringdb-plus/field_privacy_e2e_test.go
  - CLAUDE.md
tech-stack:
  added: []
  patterns:
    - serializer-layer-field-redaction (D-01)
    - fail-closed-unstamped-ctx (D-03)
    - omitempty-for-wire-omission (D-04)
    - closure-adapter-for-pagination-Convert (non-cascading ctx plumb)
    - entrest-post-process-middleware (no native per-field hook)
    - gqlgen-opt-in-custom-resolver (single-field resolver on autobind type)
key-files:
  created:
    - cmd/peeringdb-plus/field_privacy_e2e_test.go
    - internal/grpcserver/ixlan_test.go
  modified:
    - internal/pdbcompat/serializer.go
    - internal/pdbcompat/depth.go
    - internal/pdbcompat/registry_funcs.go
    - internal/pdbcompat/serializer_test.go
    - internal/grpcserver/ixlan.go
    - graph/gqlgen.yml
    - graph/generated.go
    - graph/schema.resolvers.go
    - graph/custom.resolvers.go
    - cmd/peeringdb-plus/main.go
    - cmd/peeringdb-plus/e2e_privacy_test.go
    - CLAUDE.md
decisions:
  - "Closure adapter for Convert field (preferred over changing pagination generic signature; scoped change, no cascade to 12 other *_to_proto funcs)."
  - "Keep gqlgen resolver impl in schema.resolvers.go where gqlgen emits the stub; add //nolint:revive inside godoc for the gqlgen-regenerated ObjectCounts signature (nolint comment survives regen)."
  - "restListWrapperKey constant = \"content\", confirmed by grep of ent/rest/list.go:153."
  - "Extend buildE2EFixture in-place (add ixlan seeds + IxLanService + rawIxLanHandler) rather than fork a Phase 64 fixture — minimises divergence and lets Plan 64-03 reuse the fixture's middleware chain verbatim."
metrics:
  duration: "0h51m33s"
  completed: "2026-04-18"
---

# Phase 64 Plan 03: Serializer Redaction + 5-Surface E2E Summary

**One-liner:** Apply `privfield.Redact` at all 5 API surfaces (pdbcompat, ConnectRPC, GraphQL, entrest REST, UI no-op), wire a new `restFieldRedactMiddleware` inside `restErrorMiddleware`, and lock the contract with `TestE2E_FieldLevel_IxlanURL_{RedactedAnon,VisibleToUsersTier}` (5 surfaces × 2 tiers + fail-closed-bypass-middleware sub-test) and a CLAUDE.md §Field-level privacy directive for future `<field>_visible` additions.

## Tasks Executed

| # | Task | Commit |
|---|------|--------|
| 1 | pdbcompat serializer `ixLanFromEnt(ctx, l)` + call-site threading + unit test | `e9e9289` |
| 2 | ConnectRPC `ixLanToProto(ctx, il)` + closure adapters + unit test | `ebd032f` |
| 3 | gqlgen opt-in `IxLan.ixfIxpMemberListURL` resolver + regen | `29d019e` |
| 4 | `restFieldRedactMiddleware` + chain wiring | `729e438` |
| 5 | 5-surface E2E + fail-closed bypass sub-test + CLAUDE.md update | `22afd7b` |

## Task 1 — pdbcompat

Call sites updated (5 total; every one has `ctx` already in scope as the first parameter of the enclosing function):

```text
internal/pdbcompat/depth.go:172:		m["ixlan_set"] = orEmptySlice(ixLansFromEnt(ctx, ix.Edges.IxLans))
internal/pdbcompat/depth.go:197:		base := ixLanFromEnt(ctx, l)
internal/pdbcompat/depth.go:211:	return ixLanFromEnt(ctx, l), nil
internal/pdbcompat/depth.go:321:			m["ixlan"] = ixLanFromEnt(ctx, nixl.Edges.IxLan)
internal/pdbcompat/depth.go:432:			m["ixlan"] = ixLanFromEnt(ctx, p.Edges.IxLan)
internal/pdbcompat/registry_funcs.go:207:			result := ixLansFromEnt(ctx, lans)
```

New `TestIxLanFromEnt_FieldPrivacy` asserts:
- TierPublic + `_visible=Users` → `IXFIXPMemberListURL == ""`, JSON omits key.
- TierUsers + `_visible=Users` → URL admitted.
- `_visible=Public` → URL admitted at any tier.
- `_visible` companion (`IXFIXPMemberListURLVisible`) remains populated at all tiers (D-05).

## Task 2 — ConnectRPC

Call sites (3 total): 1 direct in `GetIxLan` (line 94), 2 closure adapters in `ListIxLans` and `StreamIxLans` `Convert:` fields. The closure pattern avoids changing `grpcserver.ListParams.Convert` from `func(*E) *P` to `func(context.Context, *E) *P`, which would have cascaded to every `*_to_proto` function across the 13 entity types.

```go
// ListIxLans:
Convert: func(il *ent.IxLan) *pb.IxLan { return ixLanToProto(ctx, il) },

// StreamIxLans: same pattern
```

New `TestIxLanToProto_FieldPrivacy` covers TierPublic / TierUsers / un-stamped-ctx (fail-closed) branches and the Public-always-admit branch.

## Task 3 — GraphQL

`graph/gqlgen.yml` — opt-in:

```yaml
IxLan:
  fields:
    ixfIxpMemberListURL:
      resolver: true
```

`go tool gqlgen` regenerated:
- `graph/generated.go` (+190 lines): `IxLanResolver` interface + `IxLan()` factory on ResolverRoot.
- `graph/schema.resolvers.go`: stub `ixLanResolver.IxfIxpMemberListURL` + `ixLanResolver` type + `IxLan() IxLanResolver` factory.

Stub replaced with:

```go
func (r *ixLanResolver) IxfIxpMemberListURL(ctx context.Context, obj *ent.IxLan) (*string, error) {
    url, omit := privfield.Redact(ctx, obj.IxfIxpMemberListURLVisible, obj.IxfIxpMemberListURL)
    if omit {
        return nil, nil
    }
    return &url, nil
}
```

Idempotency verified: second `go tool gqlgen` produces no diff.

**Lint/regen friction:** gqlgen's latest version also regenerated `syncStatusResolver.ObjectCounts` signature from `_ context.Context` back to `ctx context.Context`, tripping revive's `unused-parameter` check. Added `//nolint:revive // gqlgen regenerates this signature with ctx…` inside the godoc; verified the nolint comment survives regen.

## Task 4 — entrest REST middleware

Implementation in `cmd/peeringdb-plus/main.go` (~lines 571-737). Wiring:

```go
// was
mux.Handle("/rest/v1/", restCORS(restErrorMiddleware(restSrv.Handler())))
// now
mux.Handle("/rest/v1/", restCORS(restErrorMiddleware(restFieldRedactMiddleware(restSrv.Handler()))))
```

**entrest list-wrapper key confirmation** (mandatory audit per W-4):

```text
$ grep -rn 'json:"content"' ent/rest/
ent/rest/list.go:153:	Content    []*T `json:"content"`      // Paged data.
```

Struct definition:
```text
$ grep -n "type PagedResponse" ent/rest/list.go
ent/rest/list.go:148:type PagedResponse[T any] struct {
```

So `const restListWrapperKey = "content"` in `main.go` is correct. The E2E list sub-test would fail immediately if this key drifted.

Writer wrapper `restFieldRedactWriter` implements:
- `WriteHeader(code)` — defer to flush.
- `Write(b)` — buffers to `bytes.Buffer`.
- `Flush()` — no-op during buffering (CLAUDE.md §Middleware `http.Flusher` contract).
- `Unwrap() http.ResponseWriter` — middleware-aware interface detection (CLAUDE.md §Middleware).

Path scope: only `/rest/v1/ix-lans` prefix. Content scope: only `application/json`; `application/problem+json` error bodies pass through (matters because `restErrorMiddleware` wraps us outside — order inverted would have us mis-parsing error bodies).

Content-Length: cleared after body rewrite so Go's HTTP server computes a fresh length.

## Task 5 — 5-surface E2E + CLAUDE.md

### Fixture extensions (`buildE2EFixture`)

Additive — does not perturb Phase 59 assertions:

- Added `e2eIxID = 900002` + Phase 64 constants (`e2eGatedIxlanID = 100`, `e2ePublicIxlanID = 101`, matching seed.Full Plan 64-02).
- Seed parent `InternetExchange` + two ixlan rows at the fixture level so every tier's fixture has both rows.
- Register `IxLanService` on the test mux alongside `PocService`.
- Expose `rawIxLanHandler` + `rawIxLanPath` on the fixture struct so the fail-closed sub-test can bypass the full middleware chain and hit the ConnectRPC handler with a bare `context.Background()`.
- Wire `restFieldRedactMiddleware` into the fixture's `/rest/v1/` chain so the test exercises the exact production wiring.

### Test run (`go test -race -v -run TestE2E_FieldLevel_IxlanURL ./cmd/peeringdb-plus/...`):

```text
--- PASS: TestE2E_FieldLevel_IxlanURL_VisibleToUsersTier (0.17s)
    --- PASS: TestE2E_FieldLevel_IxlanURL_VisibleToUsersTier/pdbcompat/detail/gated (0.00s)
    --- PASS: TestE2E_FieldLevel_IxlanURL_VisibleToUsersTier/pdbcompat/detail/public (0.00s)
    --- PASS: TestE2E_FieldLevel_IxlanURL_VisibleToUsersTier/entrest/detail/gated (0.01s)
    --- PASS: TestE2E_FieldLevel_IxlanURL_VisibleToUsersTier/entrest/detail/public (0.01s)
    --- PASS: TestE2E_FieldLevel_IxlanURL_VisibleToUsersTier/connectrpc/get/gated (0.01s)
    --- PASS: TestE2E_FieldLevel_IxlanURL_VisibleToUsersTier/connectrpc/get/public (0.00s)
    --- PASS: TestE2E_FieldLevel_IxlanURL_VisibleToUsersTier/connectrpc/list (0.01s)
    --- PASS: TestE2E_FieldLevel_IxlanURL_VisibleToUsersTier/graphql/gated (0.00s)
    --- PASS: TestE2E_FieldLevel_IxlanURL_VisibleToUsersTier/graphql/public (0.00s)
    --- SKIP: TestE2E_FieldLevel_IxlanURL_VisibleToUsersTier/webui (0.00s)
--- PASS: TestE2E_FieldLevel_IxlanURL_RedactedAnon (0.18s)
    --- PASS: TestE2E_FieldLevel_IxlanURL_RedactedAnon/pdbcompat/detail/gated (0.00s)
    --- PASS: TestE2E_FieldLevel_IxlanURL_RedactedAnon/pdbcompat/detail/public (0.00s)
    --- PASS: TestE2E_FieldLevel_IxlanURL_RedactedAnon/pdbcompat/list (0.00s)
    --- PASS: TestE2E_FieldLevel_IxlanURL_RedactedAnon/entrest/detail/gated (0.01s)
    --- PASS: TestE2E_FieldLevel_IxlanURL_RedactedAnon/entrest/detail/public (0.01s)
    --- PASS: TestE2E_FieldLevel_IxlanURL_RedactedAnon/entrest/list (0.01s)
    --- PASS: TestE2E_FieldLevel_IxlanURL_RedactedAnon/connectrpc/get/gated (0.00s)
    --- PASS: TestE2E_FieldLevel_IxlanURL_RedactedAnon/connectrpc/get/public (0.00s)
    --- PASS: TestE2E_FieldLevel_IxlanURL_RedactedAnon/connectrpc/list (0.00s)
    --- PASS: TestE2E_FieldLevel_IxlanURL_RedactedAnon/fail-closed-bypass-middleware (0.00s)
    --- PASS: TestE2E_FieldLevel_IxlanURL_RedactedAnon/graphql/gated (0.00s)
    --- PASS: TestE2E_FieldLevel_IxlanURL_RedactedAnon/graphql/public (0.00s)
    --- SKIP: TestE2E_FieldLevel_IxlanURL_RedactedAnon/webui (0.00s)
PASS
ok  	github.com/dotwaffle/peeringdb-plus/cmd/peeringdb-plus	1.308s
```

All 22 sub-tests pass (2 webui skipped per design).

### Phase 59 regression guard

```text
$ go test -race -run TestE2E_AnonymousCannotSeeUsersPoc ./cmd/peeringdb-plus/...
ok  	github.com/dotwaffle/peeringdb-plus/cmd/peeringdb-plus	1.290s
```

### CLAUDE.md diff

Replaced the §Schema & Visibility line:

> ~~the privacy policy nulls the value field when the visibility field is not Public.~~
> `internal/privfield.Redact` nulls/omits the value field at the serializer layer across all 5 API surfaces…

Added new §Field-level privacy section documenting:
- The `Redact` contract and the 5-surface enforcement list.
- The new-field checklist (schema → serializers × 5 → seed.Full → E2E).
- D-05 note that `_visible` is still emitted at anon tier for upstream parity.

## Wave-merge gate

| Gate | Result |
|------|--------|
| `go test -race ./...` | ok (all 19 packages) |
| `golangci-lint run ./...` on touched packages | 0 issues |
| `govulncheck ./...` | No vulnerabilities found |
| `go generate ./ent` idempotency | clean (run twice → no diff) |
| `go tool gqlgen` idempotency | clean (run twice → no diff with nolint trick) |

## Deviations from Plan

**None material.** All deviations are either already documented in the plan itself or trivial cleanups:

- **[Plan-anticipated]** Closure-adapter pattern for `Convert:` fields at `ListIxLans` / `StreamIxLans`. The plan explicitly called this out as the preferred approach over changing the pagination helper signature (13-entity cascade).
- **[Regen friction, not plan-scope]** gqlgen's current version regenerates `syncStatusResolver.ObjectCounts` with `ctx context.Context` (not `_`), tripping `revive`. Fixed with `//nolint:revive` inside the godoc — verified survives regen. This is cross-phase tech debt (not introduced by Phase 64) but surfaced by Task 3's regen; fixing it here keeps the lint gate green.
- **[Test-shape adjustment]** Fail-closed bypass sub-test decodes the raw ConnectRPC protojson body using camelCase keys (`ixLan`, `ixfIxpMemberListUrl`, `ixfIxpMemberListUrlVisible`) — protojson convention for `google.protobuf.StringValue` wrappers and proto fields. Plan's initial code sketch used snake_case tags which don't match the wire shape. Corrected during implementation.

No Rule-4 architectural decisions required.

## Auto-fixed issues

None beyond the regen-friction cleanup noted above.

## Authentication gates

None — plan is fully server-side.

## Known stubs

None — every surface in the plan has a real implementation. Web UI is explicitly deferred via `t.Skip` at each tier's `webui` sub-test with a clear TODO: "UI does not render ixf_ixp_member_list_url (Phase 64 RESEARCH)". This is documented in CLAUDE.md's new `<field>_visible` checklist as an explicit surface to wire when a future render path is added.

## TDD Gate Compliance

Plan type is `execute` (not `tdd`) per frontmatter. Each task marked `tdd="true"` followed the RED/GREEN pattern loosely: unit tests accompanied implementation within a single commit (not separate test + feat commits) to keep per-task feedback under 5 seconds. Given the plan's scope (thread ctx through 8+ call sites + E2E wiring), the phase-merge gate E2E run is the sole cross-surface RED→GREEN moment.

## Self-Check: PASSED

- [x] `.planning/phases/64-field-level-privacy/64-03-serializer-redaction-e2e-SUMMARY.md` — this file.
- [x] `internal/pdbcompat/serializer.go` — `ixLanFromEnt(ctx, l)` calls `privfield.Redact`.
- [x] `internal/grpcserver/ixlan.go` — `ixLanToProto(ctx, il)` calls `privfield.Redact`; closure adapters at Convert call sites.
- [x] `graph/gqlgen.yml` — `IxLan.ixfIxpMemberListURL: resolver: true`.
- [x] `graph/schema.resolvers.go` — `IxLanResolver.IxfIxpMemberListURL` implementation.
- [x] `cmd/peeringdb-plus/main.go` — `restFieldRedactMiddleware` + wiring.
- [x] `cmd/peeringdb-plus/field_privacy_e2e_test.go` — 22 sub-tests PASS including `fail-closed-bypass-middleware`.
- [x] `CLAUDE.md` — §Schema & Visibility updated; new §Field-level privacy section.
- [x] Commits exist: e9e9289, ebd032f, 29d019e, 729e438, 22afd7b.
