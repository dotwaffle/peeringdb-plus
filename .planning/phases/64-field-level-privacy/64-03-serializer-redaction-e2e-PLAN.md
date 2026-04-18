---
phase: 64-field-level-privacy
plan: 03
type: execute
wave: 2
depends_on:
  - 64-01
  - 64-02
files_modified:
  - internal/pdbcompat/serializer.go
  - internal/pdbcompat/depth.go
  - internal/pdbcompat/registry_funcs.go
  - internal/pdbcompat/serializer_test.go
  - internal/grpcserver/ixlan.go
  - internal/grpcserver/ixlan_test.go
  - graph/gqlgen.yml
  - graph/custom.resolvers.go
  - graph/generated.go
  - cmd/peeringdb-plus/main.go
  - cmd/peeringdb-plus/field_privacy_e2e_test.go
  - CLAUDE.md
autonomous: true
requirements:
  - VIS-08
  - VIS-09
tags:
  - privacy
  - serializer
  - e2e

must_haves:
  truths:
    - "Anonymous callers get `{\"data\":[…]}` responses with NO `ixf_ixp_member_list_url` key on any row whose `ixf_ixp_member_list_url_visible` is not `Public`, across all 5 surfaces (pdbcompat `/api`, entrest `/rest/v1`, ConnectRPC `/peeringdb.v1.*`, GraphQL `/graphql`, Web UI — UI does not currently render the URL so this surface is a no-op per RESEARCH.md)."
    - "Users-tier callers (via `PDBPLUS_PUBLIC_TIER=users` or a test fixture that stamps TierUsers on ctx) see the URL for rows with `_visible=Users` or `_visible=Public`."
    - "Private-visible rows NEVER emit the URL regardless of tier (matches upstream — the URL is never exposed when `_visible=Private`)."
    - "E2E test `TestE2E_FieldLevel_IxlanURL_*` exercises all 5 surfaces at both tiers and fails if any surface leaks the URL when it shouldn't, or hides it when it shouldn't."
    - "Fail-closed verification: with a handler bypassing `middleware.PrivacyTier` (unstamped ctx), the URL is redacted — matching privfield.Redact's fail-closed default."
    - "The pre-existing `ixf_ixp_member_list_url_visible` field is STILL emitted in anon responses (matches upstream — D-05)."
  artifacts:
    - path: internal/pdbcompat/serializer.go
      provides: "ixLanFromEnt(ctx, l) applies privfield.Redact to the URL."
      contains: "privfield.Redact"
    - path: internal/grpcserver/ixlan.go
      provides: "ixLanToProto(ctx, il) leaves IxfIxpMemberListUrl as nil when redacted."
      contains: "privfield.Redact"
    - path: graph/gqlgen.yml
      provides: "models: IxLan: fields: ixfIxpMemberListURL: resolver: true — opt-in for custom resolver."
      contains: "ixfIxpMemberListURL"
    - path: graph/custom.resolvers.go
      provides: "IxLanResolver.IxfIxpMemberListURL(ctx, obj) calls privfield.Redact; returns nil when omit."
      contains: "IxfIxpMemberListURL"
    - path: cmd/peeringdb-plus/main.go
      provides: "restFieldRedactMiddleware registered in the /rest/v1 chain; mutates JSON body of /rest/v1/ix-lans* responses."
      contains: "restFieldRedact"
    - path: cmd/peeringdb-plus/field_privacy_e2e_test.go
      provides: "5-surface E2E test mirroring Phase 59 D-15 pattern (D-10 in CONTEXT.md)."
      contains: "TestE2E_FieldLevel_IxlanURL"
    - path: CLAUDE.md
      provides: "§Schema & Visibility updated — replaces 'the privacy policy nulls the value field' with the serializer-layer privfield.Redact reality. Also documents: any new <field>_visible companion must add a privfield.Redact call at all 5 surfaces."
      contains: "privfield.Redact"
  key_links:
    - from: internal/pdbcompat/serializer.go
      to: internal/privfield/privfield.go
      via: "ixLanFromEnt(ctx, l) → privfield.Redact(ctx, l.IxfIxpMemberListURLVisible, l.IxfIxpMemberListURL)"
      pattern: "privfield\\.Redact.*IxfIxpMemberListURL"
    - from: internal/grpcserver/ixlan.go
      to: internal/privfield/privfield.go
      via: "ixLanToProto(ctx, il) → privfield.Redact"
      pattern: "privfield\\.Redact"
    - from: graph/custom.resolvers.go
      to: internal/privfield/privfield.go
      via: "IxLanResolver.IxfIxpMemberListURL → privfield.Redact"
      pattern: "privfield\\.Redact"
    - from: cmd/peeringdb-plus/main.go
      to: internal/privfield/privfield.go
      via: "restFieldRedactMiddleware body rewriter → privfield.Redact for each `data` entry"
      pattern: "privfield\\.Redact"
    - from: cmd/peeringdb-plus/field_privacy_e2e_test.go
      to: cmd/peeringdb-plus/e2e_privacy_test.go
      via: "Reuses buildE2EFixture from Phase 59 (may need a minor extension signature)."
      pattern: "buildE2EFixture"
---

<objective>
Apply field-level redaction to every API surface, wire the new `restFieldRedactMiddleware` into the main HTTP chain, and lock the full contract with a 5-surface E2E test that mirrors Phase 59 D-15 (per CONTEXT.md D-10).

Purpose: This plan operationalises VIS-08 and VIS-09. Plans 64-01 and 64-02 produced the helper and the plumbed-through data; this plan is the sole enforcement point. A regression here = a privacy leak.

Output:
- pdbcompat `ixLanFromEnt` takes ctx; calls `privfield.Redact`; empty result combined with `omitempty` tag → key omitted.
- ConnectRPC `ixLanToProto` takes ctx; leaves `IxfIxpMemberListUrl` as nil when redacted.
- GraphQL opt-in via `gqlgen.yml` + resolver in `graph/custom.resolvers.go` returning `*string` (nil when redacted).
- REST: new `restFieldRedactMiddleware` wraps entrest for `/rest/v1/ix-lans*` prefixes. Mirrors `restErrorMiddleware` pattern at main.go:527-569.
- E2E test: `TestE2E_FieldLevel_IxlanURL_RedactedAnon` + `TestE2E_FieldLevel_IxlanURL_VisibleToUsersTier` covering all 5 surfaces.
- CLAUDE.md §"Schema & Visibility" updated to reflect serializer-layer reality.

This plan runs in Wave 2; depends on Plan 64-01 (privfield package) and Plan 64-02 (ent + proto + gqlgen regen + seed row). File overlap with 64-02 on `graph/generated.go` (gqlgen regenerates when gqlgen.yml changes) forces this to a later wave.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/64-field-level-privacy/64-CONTEXT.md
@.planning/phases/64-field-level-privacy/64-RESEARCH.md
@CLAUDE.md
@cmd/peeringdb-plus/e2e_privacy_test.go
@internal/pdbcompat/serializer.go
@internal/pdbcompat/depth.go
@internal/pdbcompat/registry_funcs.go
@internal/grpcserver/ixlan.go
@graph/gqlgen.yml
@graph/custom.resolvers.go
@cmd/peeringdb-plus/main.go
@internal/privfield/privfield.go
@internal/privctx/privctx.go

<interfaces>
<!-- Post-Plan-64-02 interface surface this plan consumes. -->
<!-- Executor does NOT need to re-explore; everything needed is below. -->

From internal/privfield/privfield.go (Plan 64-01):
```go
func Redact(ctx context.Context, visible, value string) (out string, omit bool)
```

From internal/peeringdb/types.go (Plan 64-02):
```go
type IxLan struct {
    // …existing fields…
    IXFIXPMemberListURLVisible string    `json:"ixf_ixp_member_list_url_visible"`
    IXFIXPMemberListURL        string    `json:"ixf_ixp_member_list_url,omitempty"`
    // …
}
```

From ent/ixlan.go (regenerated by Plan 64-02):
```go
type IxLan struct {
    // …
    IxfIxpMemberListURLVisible string    // field 7 (unchanged position)
    IxfIxpMemberListURL        string    // new field
    // …
}
```
(field accessor is the same Go identifier as peeringdb struct, but capitalised per ent codegen convention)

From gen/peeringdb/v1/v1.pb.go (regenerated by Plan 64-02):
```go
type IxLan struct {
    // …
    IxfIxpMemberListUrlVisible string                     // proto field ~7
    IxfIxpMemberListUrl        *wrapperspb.StringValue    // proto field 14 — NIL for omit
    // …
}
```

From internal/pdbcompat/serializer.go (current — this plan changes it):
```go
// ixLanFromEnt maps an ent IxLan to a peeringdb IxLan.
func ixLanFromEnt(l *ent.IxLan) peeringdb.IxLan {
    return peeringdb.IxLan{
        ID:                         l.ID,
        // … all existing fields …
        IXFIXPMemberListURLVisible: l.IxfIxpMemberListURLVisible,
        // … (IXFIXPMemberListURL is NOT yet wired — this plan adds it)
    }
}
```

From internal/grpcserver/ixlan.go (current — this plan changes it):
```go
func ixLanToProto(il *ent.IxLan) *pb.IxLan { /* existing body */ }

// Call sites (3 in this file):
//   line 92:  ixLanToProto(il)
//   line 125: Convert: ixLanToProto  (struct field, fn value)
//   line 160: Convert: ixLanToProto  (struct field, fn value)
```

The Convert struct-field usage means changing the signature to `ixLanToProto(ctx, il)` requires either:
  (a) Changing the pagination helper's Convert field to `func(ctx, T) R` — this is a breaking change across every `*_to_proto` fn in `internal/grpcserver/`, OR
  (b) Using a closure at the call site: `Convert: func(il *ent.IxLan) *pb.IxLan { return ixLanToProto(ctx, il) }`.

**Strongly prefer (b)** — scoped change, no cascade to the other 12 entity types. RESEARCH.md's suggestion of "change signature" implicitly assumed no struct-field call sites; the `Convert` pattern here is load-bearing and must not be perturbed.

From Phase 59 buildE2EFixture (`cmd/peeringdb-plus/e2e_privacy_test.go:114`):
```go
func buildE2EFixture(t *testing.T, tier privctx.Tier) *e2eFixture {
    // Creates in-memory SQLite, seeds an org/net/poc + e2eUsersPocID row,
    // spins up all 5 surfaces on an httptest.Server, and returns
    // handlers + URLs.  Used by both RedactedAnon and VisibleToUsersTier.
}
```
The Phase 64 test reuses this. The existing fixture seeds a POC but not an ixlan with a gated URL — either:
  (a) Extend buildE2EFixture to also seed a gated ixlan, OR
  (b) Build a sibling `buildFieldPrivacyFixture` in the new e2e test file, reusing the Server construction code via a shared helper extracted from Phase 59.

Option (a) is cleanest but perturbs Phase 59 test file. Option (b) duplicates setup. RESEARCH.md §Wave 0 recommends option (b) — sibling file `field_privacy_e2e_test.go` with its own fixture. Go with (b); extract a shared "server construction" helper only if the duplication is egregious.

From graph/gqlgen.yml current state:
```yaml
models:
  ID:
    model: [github.com/99designs/gqlgen/graphql.IntID]
  Node:
    model: [github.com/dotwaffle/peeringdb-plus/ent.Noder]
```
No `IxLan` entry currently. This plan adds one.

From cmd/peeringdb-plus/main.go:304 (REST middleware wiring):
```go
mux.Handle("/rest/v1/", restCORS(restErrorMiddleware(restSrv.Handler())))
```
This plan wraps one additional layer: `restCORS(restErrorMiddleware(restFieldRedactMiddleware(restSrv.Handler())))`.

Middleware order: errorMiddleware OUTSIDE redactMiddleware so error responses (which don't contain data[]) pass through without being parsed as JSON. Verify by: after error middleware transforms a 404 into an RFC 9457 problem+json response, redact middleware sees `content-type: application/problem+json` — it must early-out on non-`application/json` content types.
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Thread ctx through pdbcompat ixLanFromEnt and apply privfield.Redact</name>
  <files>internal/pdbcompat/serializer.go, internal/pdbcompat/depth.go, internal/pdbcompat/registry_funcs.go, internal/pdbcompat/serializer_test.go</files>

  <read_first>
    1. `internal/pdbcompat/serializer.go` lines 258-285 — the full ixLanFromEnt function and its sibling `ixLansFromEnt` helper.
    2. `internal/pdbcompat/depth.go` lines 190-440 — find all 4 call sites: 197, 211, 321, 432. For EACH call site, confirm ctx is already in scope (RESEARCH.md Finding #6 claims yes; verify before refactoring).
    3. `internal/pdbcompat/registry_funcs.go` — RESEARCH.md says 1 call site at line 207 in a closure. Re-verify: `grep -n "ixLanFromEnt" internal/pdbcompat/registry_funcs.go`. If zero matches, the function ISN'T called there — update the plan to reflect actual call-site count in the SUMMARY.
    4. `internal/pdbcompat/serializer_test.go` — find the ixLanFromEnt test (if any). RESEARCH.md mentions lines 355 and 370. Read them to understand how the test invokes the function; ctx will need to be provided.
    5. `internal/privfield/privfield.go` (from Plan 64-01) — confirm the signature is `Redact(ctx, visible, value) (string, bool)`.
  </read_first>

  <behavior>
    - `ixLanFromEnt(ctx, l)` new signature; all call sites updated to pass their local ctx.
    - When `privfield.Redact(ctx, l.IxfIxpMemberListURLVisible, l.IxfIxpMemberListURL)` returns `(value, false)`, the returned `peeringdb.IxLan.IXFIXPMemberListURL` = value; JSON marshal emits the key.
    - When Redact returns `("", true)`, the returned struct's field is `""`; the `,omitempty` tag (from Plan 64-02) causes json.Marshal to omit the key entirely.
    - For the seed row (id=100, visible=Users, URL=…), anon ctx → key absent from JSON output; Users ctx → key present.
    - `ixLansFromEnt` takes ctx and passes it through to each ixLanFromEnt call.
    - The `IXFIXPMemberListURLVisible` field is STILL emitted (D-05) — no change to that line.
    - Existing pdbcompat anon parity test in `internal/pdbcompat/anon_parity_test.go` (or similar) still passes.
  </behavior>

  <action>
    1. **Edit `internal/pdbcompat/serializer.go`**. Change the signature of `ixLanFromEnt` and `ixLansFromEnt`:

    ```go
    // ixLanFromEnt maps an ent IxLan to a peeringdb IxLan, applying
    // serializer-layer field-level privacy redaction for the
    // ixf_ixp_member_list_url field per Phase 64 VIS-08/VIS-09.
    //
    // The caller MUST pass a context that has the privacy tier stamped by
    // middleware.PrivacyTier; unstamped contexts default to TierPublic
    // (fail-closed) per privfield.Redact semantics.
    func ixLanFromEnt(ctx context.Context, l *ent.IxLan) peeringdb.IxLan {
        url, _ := privfield.Redact(ctx, l.IxfIxpMemberListURLVisible, l.IxfIxpMemberListURL)
        // The `,omitempty` tag on peeringdb.IxLan.IXFIXPMemberListURL
        // (added in Plan 64-02) means an empty string == key omitted
        // at json.Marshal time, which matches upstream behaviour exactly
        // (D-04). No explicit map-of-any construction needed.
        return peeringdb.IxLan{
            ID:                         l.ID,
            IXID:                       derefInt(l.IxID),
            Name:                       l.Name,
            Descr:                      l.Descr,
            MTU:                        l.Mtu,
            Dot1QSupport:               l.Dot1qSupport,
            RSASN:                      l.RsAsn,
            ARPSponge:                  l.ArpSponge,
            IXFIXPMemberListURLVisible: l.IxfIxpMemberListURLVisible,
            IXFIXPMemberListURL:        url,
            IXFIXPImportEnabled:        l.IxfIxpImportEnabled,
            Created:                    l.Created,
            Updated:                    l.Updated,
            Status:                     l.Status,
        }
    }

    func ixLansFromEnt(ctx context.Context, lans []*ent.IxLan) []peeringdb.IxLan {
        out := make([]peeringdb.IxLan, len(lans))
        for i, l := range lans {
            out[i] = ixLanFromEnt(ctx, l)
        }
        return out
    }
    ```

    Add the import `"github.com/dotwaffle/peeringdb-plus/internal/privfield"` at top of file; add `"context"` if not already imported.

    DO NOT discard the `omit` return from privfield.Redact by accident — the `_` discard is intentional (the `omitempty` tag handles omission). Add a comment so future editors understand this is deliberate, not a bug.

    2. **Update all ixLanFromEnt call sites**. Use `grep -n "ixLanFromEnt\|ixLansFromEnt" internal/pdbcompat/` to enumerate:
      - `internal/pdbcompat/depth.go:197` — pass ctx
      - `internal/pdbcompat/depth.go:211` — pass ctx
      - `internal/pdbcompat/depth.go:321` — pass ctx
      - `internal/pdbcompat/depth.go:432` — pass ctx
      - `internal/pdbcompat/serializer.go:281` — `ixLansFromEnt` internal call; pass ctx from the function's own ctx arg (requires that function also gain ctx if it doesn't have one — check signature).

    For EACH site, confirm `ctx` is already in scope. RESEARCH.md Finding #6 asserts yes for all of them; verify with `grep -B 20 "ixLanFromEnt(l)" internal/pdbcompat/depth.go` showing a `ctx context.Context` parameter earlier in the enclosing function. If any site doesn't have ctx, thread it through from the outermost caller — DO NOT use `context.Background()` as a fallback (defeats the fail-closed design).

    3. **Update `registry_funcs.go`** if it invokes `ixLanFromEnt` or `ixLansFromEnt`. Initial grep suggests it does NOT (no matches shown in setup), but a secondary call path may exist via a different name. Re-check with: `grep -n "ixLan.*FromEnt\|ixLan.*FromMap\|ixLanEnt" internal/pdbcompat/registry_funcs.go`. If found, update.

    4. **Update `internal/pdbcompat/serializer_test.go`** if it exists and calls the function. RESEARCH.md mentioned lines 355, 370 — open and read:
      - If the test uses `context.Background()` as the fixture's ctx, accept that (tests don't exercise the gate unless they stamp a tier — for serializer-level unit testing, `context.Background()` is fine because privfield.Redact is independently covered in Plan 64-01).
      - If the test doesn't yet pass ctx, refactor signature; pass `t.Context()` (Go 1.24+) or `context.Background()`.
      - Add one NEW sub-test asserting `ixLanFromEnt(ctx, ent-row-with-visible-Users-and-URL)` returns a struct with empty URL when ctx has TierPublic, and full URL when ctx has TierUsers. This is unit-level coverage that runs faster than the full E2E in Task 5.

    5. Run `TMPDIR=/tmp/claude-1000 go build ./... && TMPDIR=/tmp/claude-1000 go test -race -count=1 ./internal/pdbcompat/...`. Expect all green, including existing `anon_parity_test.go`.
  </action>

  <verify>
    <automated>TMPDIR=/tmp/claude-1000 go test -race -count=1 ./internal/pdbcompat/...</automated>
    Also:
    - `grep -c "ixLanFromEnt(ctx" internal/pdbcompat/` should show every call site passes ctx.
    - `grep -c "ixLanFromEnt(l)" internal/pdbcompat/` should be 0 (the old signature fully retired).
    - New sub-test covers both tiers; runs under 1 s.
  </verify>

  <done>
    - `ixLanFromEnt` signature is `(ctx context.Context, l *ent.IxLan) peeringdb.IxLan`.
    - All call sites in `internal/pdbcompat/` pass ctx.
    - `ixLansFromEnt` also takes ctx and threads it through.
    - `privfield.Redact` is called for the URL field; empty result relies on `omitempty` in the json tag for omission.
    - Existing `anon_parity_test.go` or any parity/serializer test passes.
    - New unit sub-test asserts both tier cases on the seed-shape row.
    - `go test -race ./internal/pdbcompat/...` passes.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Thread ctx through grpcserver ixLanToProto and redact the URL proto field</name>
  <files>internal/grpcserver/ixlan.go, internal/grpcserver/ixlan_test.go (if exists)</files>

  <read_first>
    1. `internal/grpcserver/ixlan.go` — full file. Note the 3 call sites at lines 92, 125, 160. Line 92 is a direct call; lines 125 and 160 are `Convert: ixLanToProto` struct-field assignments in what looks like a generic pagination helper.
    2. `internal/grpcserver/pagination.go` (or equivalent) — understand the `Convert` field's type. If it's `func(T) R`, changing the fn signature to `func(ctx, T) R` would break every other *_to_proto user (all 13 entity types).
    3. `internal/grpcserver/convert.go` — any shared ctx-aware conversion helpers already in use.
    4. The `context.Context` for the RPC handler — at the top of GetIxLan, ListIxLans, StreamIxLans, ctx is the first arg. Confirm.
    5. RESEARCH.md §"Code Examples" — the ixLanToProto new version shows passing ctx directly. Applies to the direct call at line 92; for struct-field call sites, the closure adapter is required (see Interfaces note above).
  </read_first>

  <behavior>
    - `ixLanToProto(ctx, il)` new signature.
    - Call site at line 92 passes the handler's ctx directly.
    - Call sites at lines 125, 160 adapt via an inline closure: `Convert: func(il *ent.IxLan) *pb.IxLan { return ixLanToProto(ctx, il) }` — this captures the enclosing handler's ctx without altering the pagination helper's Convert field type.
    - When privfield.Redact returns omit=true, `IxfIxpMemberListUrl *wrapperspb.StringValue` is nil on the outgoing proto message. Proto3 semantics omit nil wrappers on the wire.
    - When Redact returns omit=false with a non-empty URL, the wrapper is populated.
    - No changes to the other 12 entity types' `*_to_proto` functions.
  </behavior>

  <action>
    1. **Edit `internal/grpcserver/ixlan.go`**. Change the signature of `ixLanToProto`:

    ```go
    // ixLanToProto converts an ent IxLan entity to a protobuf IxLan
    // message, applying Phase 64 VIS-09 field-level redaction for the
    // ixf_ixp_member_list_url field via internal/privfield.Redact.
    //
    // ctx MUST carry the caller's privacy tier (stamped by the
    // PrivacyTier HTTP middleware). Unstamped ctx fail-closed to
    // TierPublic per privfield.Redact semantics.
    func ixLanToProto(ctx context.Context, il *ent.IxLan) *pb.IxLan {
        urlOut, omit := privfield.Redact(ctx, il.IxfIxpMemberListURLVisible, il.IxfIxpMemberListURL)
        var urlProto *wrapperspb.StringValue
        if !omit && urlOut != "" {
            urlProto = wrapperspb.String(urlOut)
        }
        return &pb.IxLan{
            Id:                         int64(il.ID),
            IxId:                       int64PtrVal(il.IxID),
            ArpSponge:                  stringPtrVal(il.ArpSponge),
            Descr:                      stringVal(il.Descr),
            Dot1QSupport:               il.Dot1qSupport,
            IxfIxpImportEnabled:        il.IxfIxpImportEnabled,
            IxfIxpMemberListUrlVisible: stringVal(il.IxfIxpMemberListURLVisible),
            IxfIxpMemberListUrl:        urlProto,
            Mtu:                        int64Val(il.Mtu),
            Name:                       stringVal(il.Name),
            RsAsn:                      int64PtrVal(il.RsAsn),
            Created:                    timestampVal(il.Created),
            Updated:                    timestampVal(il.Updated),
            Status:                     il.Status,
        }
    }
    ```

    Add imports: `"github.com/dotwaffle/peeringdb-plus/internal/privfield"` and `"google.golang.org/protobuf/types/known/wrapperspb"`.

    The `IxfIxpMemberListUrlVisible` field (existing) is unchanged — D-05 locks that it MUST still be emitted for anon callers.

    2. **Update call site at line 92** (direct call in GetIxLan):

    ```go
    return &pb.GetIxLanResponse{IxLan: ixLanToProto(ctx, il)}, nil
    ```

    3. **Update call sites at lines 125 and 160** (struct-field usage in ListIxLans and StreamIxLans). DO NOT change the pagination helper's `Convert` field type. Use closure adapters:

    ```go
    // In ListIxLans (~line 125):
    Convert: func(il *ent.IxLan) *pb.IxLan { return ixLanToProto(ctx, il) },

    // In StreamIxLans (~line 160):
    Convert: func(il *ent.IxLan) *pb.IxLan { return ixLanToProto(ctx, il) },
    ```

    Both call sites have `ctx` in scope as the handler's first parameter — confirm with `grep -B 5 "Convert: ixLanToProto" internal/grpcserver/ixlan.go` showing the enclosing function signature.

    4. **Update `internal/grpcserver/ixlan_test.go`** if it exists. Most handler tests pass `context.Background()`; that's fine for existing tests (they exercise happy-path conversion). Add ONE new unit sub-test: given an ent.IxLan with `_visible="Users"` and a populated URL, call `ixLanToProto(privctx.WithTier(ctx, privctx.TierPublic), il)` and assert `result.IxfIxpMemberListUrl == nil`. Then with `TierUsers`, assert non-nil and correct value.

    5. Run `TMPDIR=/tmp/claude-1000 go build ./... && TMPDIR=/tmp/claude-1000 go test -race -count=1 ./internal/grpcserver/...`.
  </action>

  <verify>
    <automated>TMPDIR=/tmp/claude-1000 go test -race -count=1 ./internal/grpcserver/...</automated>
    Also:
    - `grep -n "ixLanToProto(ctx" internal/grpcserver/ixlan.go` = 1 line (direct call in GetIxLan).
    - `grep -n "ixLanToProto" internal/grpcserver/ixlan.go` = 4 lines (signature + 3 call sites).
    - `grep -n "ixLanToProto(il)" internal/grpcserver/ixlan.go` = 0 lines (old signature retired — note this excludes the closure adapters which contain `ixLanToProto(ctx, il)`).
    - No modifications to other *_to_proto functions or to pagination.go.
  </verify>

  <done>
    - `ixLanToProto` signature takes ctx first.
    - All 3 call sites updated (1 direct + 2 closure adapters in Convert fields).
    - Proto wrapper is nil when Redact says omit.
    - Unit test covers both tiers.
    - `go test -race ./internal/grpcserver/...` passes.
    - No side-effect changes to other entity types.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 3: Opt-in gqlgen resolver for IxLan.ixfIxpMemberListURL; implement in custom.resolvers.go</name>
  <files>graph/gqlgen.yml, graph/custom.resolvers.go, graph/generated.go (regenerated), graph/schema.resolvers.go (if gqlgen routes the resolver there by default)</files>

  <read_first>
    1. `graph/gqlgen.yml` — full file. Understand the autobind section (binds `github.com/dotwaffle/peeringdb-plus/ent` so `IxLan` is `*ent.IxLan`). Confirm current `models:` section has no IxLan entry.
    2. `graph/custom.resolvers.go` — understand the existing resolver pattern. Note package name, Resolver struct shape, any existing helpers.
    3. `graph/generated.go` — find the current `ResolverRoot` interface (~line 34-37 per RESEARCH.md). After regen it should grow an `IxLan()` method returning `IxLanResolver`.
    4. `graph/schema.resolvers.go` — gqlgen's default layout places resolvers per-schema-file; after regen, gqlgen may add a new resolver method stub here OR in a new `graph/ixlan.resolvers.go`. Executor must accept wherever gqlgen puts it.
    5. RESEARCH.md §"gqlgen custom field resolver opt-in" — the full pattern is documented. Copy the `IxLanResolver` resolver body verbatim.
    6. RESEARCH.md §"Pitfall 3: GraphQL exposes the field even when omitted on wire" — confirm the resolver returns `(*string, error)` with nil when omit; this yields a GraphQL `null`, not an empty string.
    7. RESEARCH.md §"Open Question 2" — if gqlgen complains about autobind + fields, add an explicit `model:` entry; executor should START with the simpler config first.
  </read_first>

  <behavior>
    - `graph/gqlgen.yml` gains a `models:` entry for `IxLan` with `fields: { ixfIxpMemberListURL: { resolver: true } }`.
    - Running `go tool gqlgen` (or `go generate ./graph` if a generate.go exists, otherwise `go generate ./ent` triggers a graph rerun via the ent/generate.go pipeline) regenerates `graph/generated.go` to include the resolver interface method.
    - `graph/custom.resolvers.go` implements `IxLanResolver.IxfIxpMemberListURL(ctx, obj *ent.IxLan) (*string, error)` calling `privfield.Redact`; returns nil when omit=true, `&url` otherwise.
    - The Resolver struct in `graph/resolver.go` satisfies the new `IxLan() IxLanResolver` method via a `IxLan()` factory that returns a `&ixLanResolver{r}` — identical pattern to any existing type resolver (if there were one) or mirrored from the gqlgen docs.
    - GraphQL query `{ ixLansList { edges { node { ixfIxpMemberListURL } } } }` returns null for rows with `_visible != Public` when caller is anonymous, and the actual URL when the caller is Users-tier.
    - `TMPDIR=/tmp/claude-1000 go build ./graph/... && go test -race ./graph/...` passes.
  </behavior>

  <action>
    1. **Edit `graph/gqlgen.yml`**. Append to the `models:` section:

    ```yaml
    models:
      ID:
        model:
          - github.com/99designs/gqlgen/graphql.IntID
      Node:
        model:
          - github.com/dotwaffle/peeringdb-plus/ent.Noder
      # Phase 64 (VIS-08/VIS-09): field-level privacy via custom resolver.
      # The autobind above maps IxLan → *ent.IxLan; we opt-in one field
      # to have a user-written resolver so it can call
      # internal/privfield.Redact against the caller's ctx tier.
      IxLan:
        fields:
          ixfIxpMemberListURL:
            resolver: true
    ```

    2. **Regenerate**. The project's codegen pipeline runs gqlgen as part of `ent/generate.go` (per CLAUDE.md §"Code Generation" — ent's generate.go calls buf which may also trigger gqlgen; verify exact chain). Safest is:

    ```bash
    TMPDIR=/tmp/claude-1000 go generate ./ent
    ```

    After regen, `git status` MUST show `graph/generated.go` modified (adds `IxLan() IxLanResolver` method on ResolverRoot, and the `IxLanResolver` interface). It should ALSO create a stub in either `graph/schema.resolvers.go` or a new file (gqlgen's behaviour depends on layout). If a new stub file appears containing only the stub, OK; if the stub is appended to an existing file, also OK.

    If gqlgen fails with "cannot find IxLan in autobinds" or similar, add an explicit `model:` entry:

    ```yaml
    IxLan:
      model:
        - github.com/dotwaffle/peeringdb-plus/ent.IxLan
      fields:
        ixfIxpMemberListURL:
          resolver: true
    ```

    3. **Implement the resolver**. After regen, find the generated stub (grep the new regen output for `IxfIxpMemberListURL`). If gqlgen left an unimplemented stub (returning `panic("not implemented")`) in `graph/schema.resolvers.go` or a new file, REPLACE the stub body with the real implementation. If gqlgen did not emit a stub (gqlgen's behaviour varies by version), add a new implementation to `graph/custom.resolvers.go`:

    ```go
    // IxfIxpMemberListURL implements the Phase 64 VIS-09 field-level
    // privacy gate for ixlan.ixf_ixp_member_list_url. See internal/privfield
    // for the redaction contract.
    func (r *ixLanResolver) IxfIxpMemberListURL(ctx context.Context, obj *ent.IxLan) (*string, error) {
        url, omit := privfield.Redact(ctx, obj.IxfIxpMemberListURLVisible, obj.IxfIxpMemberListURL)
        if omit {
            return nil, nil
        }
        return &url, nil
    }
    ```

    Plus the factory on `*Resolver`:

    ```go
    // IxLan returns the Phase-64 field-resolver for *ent.IxLan.
    func (r *Resolver) IxLan() IxLanResolver { return &ixLanResolver{r} }

    type ixLanResolver struct{ *Resolver }
    ```

    (Only add the factory + ixLanResolver type if gqlgen's generated `IxLanResolver` interface requires it and a factory doesn't already exist elsewhere in `graph/resolver.go` — grep first to avoid double-definition.)

    Add imports as needed. Verify there's no existing `ixLanResolver` name collision.

    4. **Verify wire behaviour**. After regen, run:

    ```bash
    TMPDIR=/tmp/claude-1000 go build ./graph/...
    TMPDIR=/tmp/claude-1000 go test -race -count=1 ./graph/...
    ```

    If a GraphQL integration test exists that queries `ixfIxpMemberListURL`, confirm it still passes. If not, the E2E test in Task 5 will cover GraphQL.

    5. **Regen idempotency check**. Run `go generate ./ent` a second time — `git diff` must be empty. If gqlgen regenerates non-deterministically (hash ordering etc.), investigate before committing.
  </action>

  <verify>
    <automated>TMPDIR=/tmp/claude-1000 go generate ./ent &amp;&amp; git diff --exit-code graph/ ; TMPDIR=/tmp/claude-1000 go test -race -count=1 ./graph/...</automated>
    Also:
    - `grep -c "IxLanResolver" graph/generated.go` ≥ 1 (new interface emitted).
    - `grep -c "IxfIxpMemberListURL" graph/*.go` shows both the interface and the resolver implementation.
    - `grep -c "privfield.Redact" graph/custom.resolvers.go` ≥ 1 (or wherever the resolver lives).
  </verify>

  <done>
    - `graph/gqlgen.yml` opts IxLan.ixfIxpMemberListURL into a custom resolver.
    - Regen is clean and idempotent.
    - Resolver implementation returns nil-vs-&url based on privfield.Redact.
    - `go build ./graph/...` and `go test -race ./graph/...` pass.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 4: Build restFieldRedactMiddleware and wire into /rest/v1 chain</name>
  <files>cmd/peeringdb-plus/main.go</files>

  <read_first>
    1. `cmd/peeringdb-plus/main.go` lines 300-320 — the current REST handler wiring (`mux.Handle("/rest/v1/", restCORS(restErrorMiddleware(restSrv.Handler())))`).
    2. `cmd/peeringdb-plus/main.go` lines 527-569 — the full `restErrorMiddleware` + `restErrorWriter` pattern. This is the direct template for the new middleware. Note the `Unwrap() http.ResponseWriter` method (CLAUDE.md §Middleware mandates it).
    3. CLAUDE.md §"Middleware" — the ResponseWriter wrapper must implement `http.Flusher` (delegate to underlying writer) AND `Unwrap()` method. RESEARCH.md §"Pitfall 4" — REST is non-streaming so Flusher is a non-issue in practice, but implementing it costs 3 lines and keeps the middleware contract consistent.
    4. RESEARCH.md §"entrest response middleware (new — mirrors restErrorMiddleware)" — the full middleware sketch lives in the research doc.
    5. RESEARCH.md §"Open Question 3" — Content-Length handling. Solution: after rewriting, clear `Content-Length` header so Go's http server writes chunked encoding OR computes a fresh length.
    6. Entrest REST output shapes:
       - Detail: `GET /rest/v1/ix-lans/{id}` → a single JSON object of the ixlan.
       - List: `GET /rest/v1/ix-lans` → `{"content": [...]}` paginated wrapper (verify by querying the running app or by grepping generated entrest code for the response wrapper type).
       Both cases MUST redact the URL key. Test BOTH in the E2E test (Task 5).
  </read_first>

  <behavior>
    - `/rest/v1/ix-lans*` (prefix match, both detail and list paths) responses have the `ixf_ixp_member_list_url` key removed from the body when:
      - Response Content-Type is `application/json`.
      - The row has `ixf_ixp_member_list_url_visible != "Public"`.
      - The caller's ctx has tier < TierUsers.
    - Non-ixlan `/rest/v1/*` paths are unaffected (middleware early-outs on path mismatch).
    - Non-JSON responses (error bodies with `application/problem+json`, empty bodies, binary) are passed through unchanged.
    - `Content-Length` header is recomputed or cleared after body rewrite to match the new body length.
    - Middleware wrapper implements `Unwrap() http.ResponseWriter` and `http.Flusher` per CLAUDE.md §Middleware.
    - `go test -race ./cmd/peeringdb-plus/...` passes including the E2E test in Task 5.
  </behavior>

  <action>
    1. **Edit `cmd/peeringdb-plus/main.go`**. Add the new middleware implementation BELOW `restErrorMiddleware` (~after line 569). Full skeleton:

    ```go
    // restFieldRedactMiddleware removes the `ixf_ixp_member_list_url` key
    // from /rest/v1/ix-lans* JSON responses when the caller's ctx tier
    // does not admit the field (per internal/privfield.Redact).
    //
    // entrest has no native per-field conditional-omission hook (verified
    // against lrstanley/entrest annotation reference, Phase 64 RESEARCH.md
    // Finding #1). This middleware is the workaround: it buffers the
    // response body on the ixlan paths, parses the JSON, walks the ixlan
    // object(s), and re-emits with the field deleted when privfield.Redact
    // says omit.
    //
    // Scope: only /rest/v1/ix-lans and /rest/v1/ix-lans/{id} (prefix match).
    // Other REST paths pass through untouched. Non-JSON responses pass through.
    //
    // Phase 64 VIS-08 / VIS-09.
    func restFieldRedactMiddleware(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if !strings.HasPrefix(r.URL.Path, "/rest/v1/ix-lans") {
                next.ServeHTTP(w, r)
                return
            }
            rw := &restFieldRedactWriter{ResponseWriter: w, r: r}
            next.ServeHTTP(rw, r)
            rw.flushRewrite(w, r.Context())
        })
    }

    type restFieldRedactWriter struct {
        http.ResponseWriter
        r          *http.Request
        status     int
        buf        bytes.Buffer
        headerSent bool
    }

    func (w *restFieldRedactWriter) WriteHeader(code int) {
        w.status = code
        // Defer writing the header to the upstream writer until we've
        // had a chance to rewrite the body + recompute Content-Length.
    }

    func (w *restFieldRedactWriter) Write(b []byte) (int, error) {
        return w.buf.Write(b)
    }

    func (w *restFieldRedactWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

    func (w *restFieldRedactWriter) Flush() {
        // No-op during buffering; the real flush happens in flushRewrite.
    }

    // flushRewrite is called by the middleware after next.ServeHTTP returns.
    // It decides whether to rewrite the body, then writes headers + body
    // to the underlying ResponseWriter.
    func (w *restFieldRedactWriter) flushRewrite(upstream http.ResponseWriter, ctx context.Context) {
        status := w.status
        if status == 0 {
            status = http.StatusOK
        }
        body := w.buf.Bytes()

        // Pass through non-JSON bodies unchanged.
        contentType := w.Header().Get("Content-Type")
        if !strings.HasPrefix(contentType, "application/json") || len(body) == 0 {
            // Copy headers, write status, write body.
            // (http.ResponseWriter.Header() on w already reflects headers
            // set by next.ServeHTTP — they're on the same map pointer.)
            upstream.WriteHeader(status)
            _, _ = upstream.Write(body)
            return
        }

        // Attempt JSON rewrite. Two shapes supported:
        //   - Detail:  top-level object with `ixf_ixp_member_list_url` + `ixf_ixp_member_list_url_visible`
        //   - List:    wrapper with "content": [ { ... }, ... ]
        rewritten, err := redactIxlanJSON(ctx, body)
        if err != nil {
            // Fail-closed: if we can't parse the body, pass through
            // unchanged. A subsequent conformance test will flag this,
            // but we MUST NOT corrupt a legitimate response.
            upstream.Header().Del("Content-Length")
            upstream.WriteHeader(status)
            _, _ = upstream.Write(body)
            return
        }

        // Clear Content-Length — the rewritten body is a different length.
        upstream.Header().Del("Content-Length")
        upstream.WriteHeader(status)
        _, _ = upstream.Write(rewritten)
    }

    // redactIxlanJSON parses body as JSON, walks any ixlan objects it finds,
    // and drops the `ixf_ixp_member_list_url` key when privfield.Redact
    // says omit. Returns the re-encoded body.
    func redactIxlanJSON(ctx context.Context, body []byte) ([]byte, error) {
        // Try detail shape first.
        var detail map[string]any
        if err := json.Unmarshal(body, &detail); err != nil {
            return nil, err
        }
        // If the top-level has a "content" key (list), walk each entry.
        if content, ok := detail["content"].([]any); ok {
            for _, entry := range content {
                obj, ok := entry.(map[string]any)
                if !ok {
                    continue
                }
                redactIxlanObject(ctx, obj)
            }
            return json.Marshal(detail)
        }
        // Otherwise treat as a single ixlan object.
        redactIxlanObject(ctx, detail)
        return json.Marshal(detail)
    }

    // redactIxlanObject drops the url key in-place if redaction applies.
    func redactIxlanObject(ctx context.Context, obj map[string]any) {
        visible, _ := obj["ixf_ixp_member_list_url_visible"].(string)
        url, _ := obj["ixf_ixp_member_list_url"].(string)
        _, omit := privfield.Redact(ctx, visible, url)
        if omit {
            delete(obj, "ixf_ixp_member_list_url")
        }
    }
    ```

    Add imports at top of main.go: `"bytes"`, `"encoding/json"`, `"strings"` (if not already), `"github.com/dotwaffle/peeringdb-plus/internal/privfield"`.

    The list-wrapper key is assumed to be `content` (entrest default). Verify by hitting `/rest/v1/ix-lans?limit=1` on a running test server OR by reading the entrest-generated response type in `ent/rest/`. If the wrapper key is different (e.g., `data`), adjust.

    2. **Wire the middleware into the chain**. Change line 304:

    Before:
    ```go
    mux.Handle("/rest/v1/", restCORS(restErrorMiddleware(restSrv.Handler())))
    ```

    After:
    ```go
    mux.Handle("/rest/v1/", restCORS(restErrorMiddleware(restFieldRedactMiddleware(restSrv.Handler()))))
    ```

    Ordering: errorMiddleware OUTSIDE redactMiddleware. Rationale:
      - errorMiddleware transforms error responses into `application/problem+json`.
      - redactMiddleware only rewrites `application/json` bodies, so error responses pass through unchanged.
      - If the ordering were reversed, the redact middleware would see raw error bodies (some of which are JSON) and potentially corrupt them.

    3. **Run the existing test suite** to confirm no regression on non-ixlan REST paths:

    ```bash
    TMPDIR=/tmp/claude-1000 go test -race -count=1 ./cmd/peeringdb-plus/...
    ```

    The full E2E assertion lives in Task 5. Here, verify the middleware doesn't break the existing `e2e_privacy_test.go` Phase 59 tests, any REST integration tests, or the conformance test.

    4. **Inspect the response for content-type** once manually via a running test server (or via the E2E test in Task 5). Request `/rest/v1/ix-lans/100` with anon tier → expect no `ixf_ixp_member_list_url` key. Request with Users tier → expect the key present.
  </action>

  <verify>
    <automated>TMPDIR=/tmp/claude-1000 go test -race -count=1 ./cmd/peeringdb-plus/...</automated>
    Also:
    - `grep -c "restFieldRedactMiddleware" cmd/peeringdb-plus/main.go` = 2 (definition + wiring).
    - `grep -c "restFieldRedactWriter" cmd/peeringdb-plus/main.go` = 1 type + several method receivers ≥ 5.
    - `grep "Unwrap()" cmd/peeringdb-plus/main.go | grep restFieldRedact` ≥ 1 (CLAUDE.md §Middleware requirement).
    - Existing Phase 59 privacy tests still pass: `go test -race -run TestE2E_AnonymousCannotSeeUsersPoc ./cmd/peeringdb-plus/...`.
  </verify>

  <done>
    - `restFieldRedactMiddleware` implemented, wired into the /rest/v1 chain INSIDE errorMiddleware.
    - Wrapper writer implements `Unwrap()` and `Flush()`.
    - Non-ixlan paths pass through untouched.
    - Non-JSON bodies pass through untouched.
    - Failed-parse falls back to unchanged body (fail-closed for correctness, not for privacy — a follow-up Task 5 E2E test will catch privacy failures).
    - `go test -race ./cmd/peeringdb-plus/...` passes, including the unmodified Phase 59 tests.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 5: Write 5-surface E2E test mirroring Phase 59's D-15 pattern + update CLAUDE.md</name>
  <files>cmd/peeringdb-plus/field_privacy_e2e_test.go, CLAUDE.md</files>

  <read_first>
    1. `cmd/peeringdb-plus/e2e_privacy_test.go` — the ENTIRE file. This is the template (CONTEXT.md D-10). Pay special attention to:
       - `buildE2EFixture(t, tier)` — constructs the in-memory SQLite + all 5 surface handlers on an httptest.Server.
       - The nested `t.Run` structure covering all 5 surfaces.
       - How the tier is stamped (via `middleware.PrivacyTier` OR via `PDBPLUS_PUBLIC_TIER` env var).
       - The POC seed IDs (e2eUsersPocID etc.) — this is the row that was seeded as `visible=Users`.
    2. `internal/testutil/seed/seed.go` — confirm Plan 64-02 seeded the primary IxLan (id=100) with `_visible=Users` and a URL. If it instead added a new row (id=101), adjust the test to target that row.
    3. `CLAUDE.md` §"Schema & Visibility" — the current wording says "the privacy policy nulls the value field when the visibility field is not Public." This is aspirational / not yet true; Phase 64 makes it real via the serializer layer. Update wording per RESEARCH.md §"Project Constraints".
    4. CONTEXT.md D-10 — "Mirror Phase 59's D-15 E2E test pattern. New `TestE2E_FieldLevel_IxlanURL_RedactedAnon` (and companion `_VisibleToUsersTier`) spanning all 5 surfaces."
    5. RESEARCH.md §"Phase Requirements → Test Map" — the complete list of assertions required.
    6. Quality-gate criterion: "E2E test mirrors Phase 59's TestE2E_AnonymousCannotSeeUsersPoc pattern (D-10)".
  </read_first>

  <behavior>
    Two top-level tests (mirroring Phase 59):
    - `TestE2E_FieldLevel_IxlanURL_RedactedAnon` — builds fixture at TierPublic, asserts:
      - `GET /api/ixlan/100` (pdbcompat detail) → JSON has NO `ixf_ixp_member_list_url` key.
      - `GET /api/ixlan?id=100` (pdbcompat list) → each entry has NO `ixf_ixp_member_list_url` key.
      - `GET /rest/v1/ix-lans/100` (entrest detail) → JSON has NO `ixf_ixp_member_list_url` key.
      - `GET /rest/v1/ix-lans` (entrest list) → each entry has NO `ixf_ixp_member_list_url` key.
      - ConnectRPC `GetIxLan(id=100)` → response.IxLan.IxfIxpMemberListUrl is nil.
      - ConnectRPC `ListIxLans` → response items have nil IxfIxpMemberListUrl.
      - GraphQL `{ ixLansList { edges { node { id ixfIxpMemberListURL ixfIxpMemberListURLVisible } } } }` → ixfIxpMemberListURL is null (JSON null, not omitted); ixfIxpMemberListURLVisible is `"Users"` (still emitted per D-05).
      - Web UI `/ui/ixlan/100` (if it exists) — SKIP with a TODO comment (UI currently doesn't render the URL per RESEARCH.md; re-enable if the UI ever adds it).
      - ALL surfaces STILL emit `ixf_ixp_member_list_url_visible` on each ixlan row (locks D-05).
    - `TestE2E_FieldLevel_IxlanURL_VisibleToUsersTier` — builds fixture at TierUsers, asserts:
      - Same surfaces above — ixfIxpMemberListURL is present with value `https://example.test/ix/100/members.json` (or whatever seed.Full uses).

    Each top-level test uses `t.Parallel()`. Each surface is a sub-test via `t.Run(surfaceName, ...)`.

    Test also asserts that when `_visible=Public` (for any public-seeded row), BOTH tiers emit the URL. Add a second seeded ixlan with `_visible=Public` if seed.Full doesn't already have one (or create it inline in the fixture).

    Plus: one test asserts fail-closed behaviour — call `ixLanFromEnt(context.Background(), seedRow)` directly (unit-level) and confirm URL is redacted. Wait — that's Plan 64-01 unit coverage. At E2E level, simulate by constructing an httptest request that does NOT go through `middleware.PrivacyTier` (e.g., call the ConnectRPC handler directly with a bare ctx) and assert URL is nil. This catches regressions where some surface forgets to route through the middleware.
  </behavior>

  <action>
    1. **Create `cmd/peeringdb-plus/field_privacy_e2e_test.go`**. File header and structure:

    ```go
    // Package main field_privacy_e2e_test.go — Phase 64 D-10 end-to-end
    // field-level privacy contract for ixlan.ixf_ixp_member_list_url.
    //
    // This test mirrors e2e_privacy_test.go's 5-surface pattern (Phase 59
    // D-15) but operates at field level instead of row level. It asserts:
    //
    //   - Anonymous callers (TierPublic) get NO ixf_ixp_member_list_url
    //     key in responses for rows with _visible="Users" or "Private".
    //   - Users-tier callers DO get the URL for _visible="Users" or
    //     _visible="Public" rows.
    //   - The companion _visible field is ALWAYS emitted regardless of
    //     tier (D-05 — upstream parity).
    //
    // The test uses the same buildE2EFixture(t, tier) helper as Phase 59;
    // if that helper doesn't already seed a gated ixlan, we either
    // extend it or seed inline here (Plan 64-02's seed.Full should have
    // pre-seeded the primary ixlan with _visible=Users + URL populated).
    package main

    import (
        "context"
        "encoding/json"
        "fmt"
        "io"
        "net/http"
        "testing"

        "connectrpc.com/connect"

        pbv1 "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
        "github.com/dotwaffle/peeringdb-plus/internal/privctx"
    )

    // Phase 64 uses the Plan 64-02 seed — primary IxLan id=100 with
    // _visible="Users" and URL "https://example.test/ix/100/members.json".
    const e2eGatedIxlanID = 100
    const e2eGatedIxlanURL = "https://example.test/ix/100/members.json"

    func TestE2E_FieldLevel_IxlanURL_RedactedAnon(t *testing.T) {
        t.Parallel()
        fix := buildE2EFixture(t, privctx.TierPublic)

        t.Run("pdbcompat/detail", func(t *testing.T) {
            body := fetchJSON(t, fix.serverURL+fmt.Sprintf("/api/ixlan/%d", e2eGatedIxlanID))
            assertHasKey(t, body, "ixf_ixp_member_list_url_visible") // D-05
            assertLacksKey(t, body, "ixf_ixp_member_list_url")       // VIS-09
        })

        t.Run("pdbcompat/list", func(t *testing.T) {
            body := fetchJSON(t, fix.serverURL+fmt.Sprintf("/api/ixlan?id=%d", e2eGatedIxlanID))
            entries := extractListEntries(t, body)
            if len(entries) == 0 {
                t.Fatal("expected at least one ixlan entry")
            }
            for _, e := range entries {
                assertHasKey(t, e, "ixf_ixp_member_list_url_visible")
                assertLacksKey(t, e, "ixf_ixp_member_list_url")
            }
        })

        t.Run("entrest/detail", func(t *testing.T) {
            body := fetchJSON(t, fix.serverURL+fmt.Sprintf("/rest/v1/ix-lans/%d", e2eGatedIxlanID))
            assertHasKey(t, body, "ixf_ixp_member_list_url_visible")
            assertLacksKey(t, body, "ixf_ixp_member_list_url")
        })

        t.Run("entrest/list", func(t *testing.T) {
            body := fetchJSON(t, fix.serverURL+"/rest/v1/ix-lans")
            entries := extractRESTContent(t, body)
            if len(entries) == 0 {
                t.Fatal("expected at least one ixlan entry")
            }
            for _, e := range entries {
                assertHasKey(t, e, "ixf_ixp_member_list_url_visible")
                assertLacksKey(t, e, "ixf_ixp_member_list_url")
            }
        })

        t.Run("connectrpc/get", func(t *testing.T) {
            resp, err := fix.ixLanClient.GetIxLan(context.Background(), connect.NewRequest(&pbv1.GetIxLanRequest{Id: e2eGatedIxlanID}))
            if err != nil {
                t.Fatalf("GetIxLan: %v", err)
            }
            if resp.Msg.IxLan.IxfIxpMemberListUrl != nil {
                t.Errorf("anon tier received url = %v, want nil", resp.Msg.IxLan.IxfIxpMemberListUrl)
            }
            if resp.Msg.IxLan.IxfIxpMemberListUrlVisible == "" {
                t.Error("expected visible companion field to remain populated (D-05)")
            }
        })

        t.Run("connectrpc/list", func(t *testing.T) {
            resp, err := fix.ixLanClient.ListIxLans(context.Background(), connect.NewRequest(&pbv1.ListIxLansRequest{}))
            if err != nil {
                t.Fatalf("ListIxLans: %v", err)
            }
            for _, il := range resp.Msg.Items {
                if il.Id == e2eGatedIxlanID && il.IxfIxpMemberListUrl != nil {
                    t.Errorf("anon tier list received url for gated row")
                }
            }
        })

        t.Run("graphql", func(t *testing.T) {
            query := `{ ixLans(where:{id:` + fmt.Sprintf("%d", e2eGatedIxlanID) + `}) { edges { node { id ixfIxpMemberListURL ixfIxpMemberListURLVisible } } } }`
            result := fetchGraphQL(t, fix.serverURL+"/graphql", query)
            node := extractFirstEdgeNode(t, result, "ixLans")
            if node["ixfIxpMemberListURL"] != nil {
                t.Errorf("anon tier received URL = %v, want null", node["ixfIxpMemberListURL"])
            }
            if v, _ := node["ixfIxpMemberListURLVisible"].(string); v == "" {
                t.Error("expected visible companion to remain populated")
            }
        })

        t.Run("webui", func(t *testing.T) {
            // Phase 64 RESEARCH Finding: UI does not currently render the
            // URL. If a future phase adds it to /ui/ixlan/{id} or similar,
            // extend this sub-test to parse the rendered HTML and assert
            // the URL is NOT present at TierPublic.
            t.Skip("UI does not render ixf_ixp_member_list_url (Phase 64 RESEARCH)")
        })
    }

    func TestE2E_FieldLevel_IxlanURL_VisibleToUsersTier(t *testing.T) {
        t.Parallel()
        fix := buildE2EFixture(t, privctx.TierUsers)

        // Mirror the structure above but assert the URL IS present on all
        // 5 surfaces. For ConnectRPC, assert the *wrapperspb.StringValue
        // is non-nil and its GetValue() == e2eGatedIxlanURL. For GraphQL,
        // assert node["ixfIxpMemberListURL"] == e2eGatedIxlanURL.
        // For pdbcompat + entrest detail/list, assert `.ixf_ixp_member_list_url == e2eGatedIxlanURL`.
        // For webui, Skip.
    }

    // Helper functions (fetchJSON, assertHasKey, assertLacksKey,
    // extractListEntries, extractRESTContent, extractFirstEdgeNode,
    // fetchGraphQL) — implement inline or extract from e2e_privacy_test.go.
    // If e2e_privacy_test.go already defines these, reuse (same package).
    ```

    Implementation notes:
    - `buildE2EFixture` signature may need extension. Read it first. If it already builds + exposes all 5 surface clients (`fix.ixLanClient` etc.), great. If it's POC-focused only, extend it or create a parallel `buildFieldPrivacyFixture` function in the new test file. Prefer extension if cheap; duplication if extension perturbs Phase 59 tests.
    - The ConnectRPC client path requires a gRPC handle on the test server. Phase 59's fixture already builds it — check the `ixLanClient` field or equivalent. If missing, add.
    - GraphQL POST format: `{"query":"..."}`. Helper `fetchGraphQL` should handle that.
    - For the Users-tier test, `buildE2EFixture(t, TierUsers)` presumably stamps the tier via `middleware.PrivacyTier` + `PDBPLUS_PUBLIC_TIER=users` test override. Confirm Phase 59's fixture does this.

    **DO NOT hardcode expected string values** that depend on seed.Full details — import the seed's exported constants if available, or define the constants in this test file mirroring seed.Full.

    2. **Update CLAUDE.md §"Schema & Visibility"** (existing section, a couple paragraphs long). Current text says:

    > `ixlan.ixf_ixp_member_list_url_visible` (`ent/schema/ixlan.go`) — per-field: `Public`, `Users`, or `Private`. Gates the sibling `ixf_ixp_member_list_url` field; the privacy policy nulls the value field when the visibility field is not `Public`.

    Replace the final sentence with:

    > `ixlan.ixf_ixp_member_list_url_visible` (`ent/schema/ixlan.go`) — per-field: `Public`, `Users`, or `Private`. Gates the sibling `ixf_ixp_member_list_url` field; `internal/privfield.Redact` nulls/omits the value field at the serializer layer across all 5 API surfaces (pdbcompat, entrest, ConnectRPC, GraphQL, Web UI). ent's built-in privacy package operates at query/row level only — field-level redaction is a serializer-layer concern per Phase 64 decision D-01.

    Also add a new bullet (or expand the existing guidance):

    > **Adding a new `<field>_visible` companion** (future OAuth fields, etc.): in addition to the existing ent schema edit, add a `privfield.Redact` call at each of the 5 serializer surfaces (`internal/pdbcompat/serializer.go`, `internal/grpcserver/{entity}.go`, `graph/custom.resolvers.go`, the `restFieldRedactMiddleware` path-scope filter in `cmd/peeringdb-plus/main.go`, and the web UI render path if applicable). Add a corresponding E2E assertion in `cmd/peeringdb-plus/field_privacy_e2e_test.go` patterned on Phase 64's `TestE2E_FieldLevel_IxlanURL_*`.

    3. Run the full test suite:

    ```bash
    TMPDIR=/tmp/claude-1000 go test -race -count=1 ./...
    ```

    Expect every surface's sub-test to pass. If any fails, the failure message MUST point at which surface is leaking (sub-test name identifies it directly).

    4. Run `golangci-lint run` and `govulncheck ./...` per project CI gates.
  </action>

  <verify>
    <automated>TMPDIR=/tmp/claude-1000 go test -race -count=1 -run TestE2E_FieldLevel_IxlanURL ./cmd/peeringdb-plus/... &amp;&amp; TMPDIR=/tmp/claude-1000 go test -race -count=1 ./...</automated>
    Also:
    - `grep -c "TestE2E_FieldLevel_IxlanURL_RedactedAnon" cmd/peeringdb-plus/field_privacy_e2e_test.go` = 1.
    - `grep -c "TestE2E_FieldLevel_IxlanURL_VisibleToUsersTier" cmd/peeringdb-plus/field_privacy_e2e_test.go` = 1.
    - `grep -c "t.Run(" cmd/peeringdb-plus/field_privacy_e2e_test.go` ≥ 14 (7 surfaces × 2 test fns, including skipped webui).
    - `grep -c "privfield.Redact" CLAUDE.md` ≥ 1 (updated text refers to the helper).
    - Phase 59 TestE2E_AnonymousCannotSeeUsersPoc still passes (regression guard).
  </verify>

  <done>
    - `cmd/peeringdb-plus/field_privacy_e2e_test.go` exists; both test functions implemented; all 5 surfaces covered (webui skipped with clear TODO).
    - CLAUDE.md §"Schema & Visibility" updated to reflect serializer-layer reality + new-field checklist.
    - `go test -race ./...` passes.
    - golangci-lint clean.
    - govulncheck clean.
    - Plan-checker can confirm every surface has a dedicated assertion.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| HTTP request → middleware chain | Untrusted tier inference; PrivacyTier middleware owns stamping. Redaction surfaces downstream read ctx — fail-closed on absence. |
| entrest response → restFieldRedactMiddleware | Middleware rewrites JSON bodies for ixlan paths. Must not corrupt non-ixlan bodies. Scoped by path prefix + content-type. |
| ent row → serializer | Row data is trusted (from sync); serializer MUST redact based on `_visible` companion, not silently trust the URL. |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-64-08 | Information Disclosure | pdbcompat serializer | mitigate | `ixLanFromEnt` calls `privfield.Redact` unconditionally; `,omitempty` tag omits the key. Test in Task 1 + E2E in Task 5. |
| T-64-09 | Information Disclosure | ConnectRPC serializer | mitigate | `ixLanToProto` calls `privfield.Redact`; proto wrapper nil → wire omission. Test in Task 2 + E2E in Task 5. |
| T-64-10 | Information Disclosure | GraphQL autobind bypass | mitigate | gqlgen `models:` opt-in forces the resolver; gqlgen generates the interface; cannot be silently skipped. E2E in Task 5 asserts null. |
| T-64-11 | Information Disclosure | entrest JSON body rewrite | mitigate | `restFieldRedactMiddleware` rewrites body; path-scoped to `/rest/v1/ix-lans*`. Task 5 E2E asserts absence on both detail + list. |
| T-64-12 | Tampering | body-rewrite Content-Length | mitigate | Middleware clears Content-Length before writing rewritten body; Go http server either computes fresh length or chunked encoding. RESEARCH §Open Question 3. |
| T-64-13 | Denial of Service | REST middleware buffers body | accept | `/rest/v1/ix-lans*` responses are bounded (pagination caps); RESEARCH §Pitfall 4 confirms REST is non-streaming. No DoS vector. |
| T-64-14 | Information Disclosure | _visible companion field | accept | D-05: `_visible` is STILL emitted for anon callers (matches upstream; minor side-channel). Accepted per user decision. |
| T-64-15 | Elevation of Privilege | fail-open on unstamped ctx | mitigate | `privfield.Redact` fail-closes via `privctx.TierFrom` (which defaults to TierPublic when unstamped). Locked by Plan 64-01 unit test. Verified at E2E level by Task 5's webui/bypass sub-cases. |
| T-64-16 | Spoofing | client forges tier | accept | Tier is derived server-side from `PDBPLUS_PUBLIC_TIER` (Phase 61) or future OAuth middleware — never from client-supplied data. Not a new attack surface. |
</threat_model>

<verification>
**Code surface:**
- `internal/pdbcompat/serializer.go` + `depth.go` — all `ixLanFromEnt` call sites pass ctx; `privfield.Redact` called once per row.
- `internal/grpcserver/ixlan.go` — `ixLanToProto` takes ctx; proto wrapper nil on redact; pagination helper Convert field type untouched.
- `graph/gqlgen.yml` + `graph/custom.resolvers.go` — IxLan.ixfIxpMemberListURL routed through custom resolver returning `*string`.
- `cmd/peeringdb-plus/main.go` — `restFieldRedactMiddleware` defined; wired inside `restErrorMiddleware`.
- `cmd/peeringdb-plus/field_privacy_e2e_test.go` — both test functions present; 5 surfaces covered (webui skipped).
- `CLAUDE.md` — §"Schema & Visibility" reflects serializer-layer reality.

**Tests:**
- `TestE2E_FieldLevel_IxlanURL_RedactedAnon` passes; each sub-test asserts URL absent + `_visible` present.
- `TestE2E_FieldLevel_IxlanURL_VisibleToUsersTier` passes; each sub-test asserts URL present with expected value.
- Existing `TestE2E_AnonymousCannotSeeUsersPoc` (Phase 59) still passes.
- Existing pdbcompat anon parity test passes.
- `go test -race ./...` clean.
- `golangci-lint run` clean.
- `govulncheck ./...` clean.

**Regeneration integrity:**
- `go generate ./ent` produces empty diff on second run.
- `graph/generated.go` regenerated to include `IxLanResolver` interface; resolver stub/impl present.
</verification>

<success_criteria>
- Every one of the 5 API surfaces redacts `ixf_ixp_member_list_url` based on `_visible` + ctx tier via a single helper call (`privfield.Redact`).
- E2E test locks all 5 surfaces at both tiers; a regression in any surface fails a named sub-test.
- Fail-closed semantics: unstamped ctx at any surface → URL absent (provable at unit level via Plan 64-01 tests; surface-level provable by bypassing PrivacyTier middleware).
- `_visible` companion emitted at anon tier (D-05 locked).
- `_visible=Private` rows NEVER emit URL (locks upstream parity).
- CLAUDE.md updated to reflect post-Phase-64 truth + guidance for future gated fields.
- CI gates pass: build, test, lint, vuln.
- VIS-08 and VIS-09 requirements complete.
</success_criteria>

<output>
After completion, create `.planning/phases/64-field-level-privacy/64-03-SUMMARY.md` documenting:
- Exact call sites updated in pdbcompat/depth.go, serializer.go, grpcserver/ixlan.go.
- gqlgen regen diff (IxLanResolver interface + stub).
- restFieldRedactMiddleware wrapper methods (Unwrap, Flush, Write, WriteHeader — line numbers in main.go).
- E2E test — paste the `go test -race -v -run TestE2E_FieldLevel_IxlanURL ./cmd/peeringdb-plus/...` output showing every sub-test PASS.
- Phase 59 regression guard — paste the `go test -race -run TestE2E_AnonymousCannotSeeUsersPoc` output.
- CLAUDE.md diff (the §"Schema & Visibility" update).
- Note if `buildE2EFixture` needed extension — what was added.
- Any deviations from RESEARCH.md code examples (expect: the closure-adapter pattern for Convert: ixLanToProto call sites, which RESEARCH didn't explicitly cover).
</output>
