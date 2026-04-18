# Phase 64: Field-level privacy — ixlan.ixf_ixp_member_list_url - Research

**Researched:** 2026-04-18
**Domain:** Serializer-layer field-level privacy redaction across 5 API surfaces (pdbcompat/REST/GraphQL/ConnectRPC/UI), atop ent + entrest + entgql + entproto codegen pipeline.
**Confidence:** HIGH

## Summary

Phase 64 adds one data field (`ixlan.ixf_ixp_member_list_url`) and builds the first field-level privacy gate in the codebase. The work is mostly mechanical plumbing; the interesting decisions are confined to (a) how each of the 5 surfaces omits a single field based on `privctx.TierFrom(ctx)`, and (b) the shape of the new `internal/privfield` package.

Key findings:

1. **entrest has no native per-request field-omission hook** `[VERIFIED: lrstanley/entrest annotation reference]`. The idiomatic workaround is a response-body-rewriting middleware analogous to the existing `restErrorMiddleware` in `cmd/peeringdb-plus/main.go:527-569`. This is the only surface that requires a novel integration technique; the other four have straightforward integration points.
2. **entproto field numbers are positional**, not explicit `[VERIFIED: ent/entc.go shows no entproto.Field annotations; proto/peeringdb/v1/v1.proto shows sequential numbers 1..13 matching ent schema declaration order]`. Adding the URL field adjacent to the `_visible` companion (schema position ~8) would renumber fields 8-13 and **break proto wire compat**. The field must be **appended at the end** of the IxLan schema field list to safely claim field number 14.
3. **gqlgen requires explicit type resolver opt-in**. The current `ResolverRoot` in `graph/generated.go:34-37` only declares `Query()` and `SyncStatus()` resolvers — no type-level resolvers exist. To intercept `IxLan.ixfIxpMemberListURL`, the plan must add the type to gqlgen's `models:` map in `graph/gqlgen.yml` with a `fields:` subsection marking that single field as needing a resolver. This causes gqlgen to emit an `IxLanResolver` interface with an `IxfIxpMemberListURL(ctx, obj)` method. `[CITED: gqlgen docs — custom field resolver]`
4. **The backfill assumption in CONTEXT.md D-12 is correct but the row count is off.** CONTEXT.md says "18 rows get the URL on next sync." The auth fixture actually shows **46 rows** with the URL field populated (28 `Public` + 18 `Users`). Anon responses already show the URL for the 28 Public rows — so even without an API key set, the URL field will populate on 28 rows. `[VERIFIED: testdata/visibility-baseline/beta/auth/api/ixlan/page-{1,2}.json row counts]`
5. **The auth sync path is sufficient as-is** — no changes to `internal/peeringdb/client.go` are needed. The existing `Api-Key` header injection at `internal/peeringdb/client.go:301-302` already triggers the auth shape on `/api/ixlan` fetches; once `peeringdb.IxLan.IXFIXPMemberListURL` has a JSON tag, `json.Decode` picks it up on existing responses.
6. **`ctx` is already available** in all 5 serializer surface entry points; the only surface that needs plumbing through is pdbcompat's `ixLanFromEnt` which currently takes `*ent.IxLan` only. The other four (grpcserver, REST middleware, GraphQL resolver, UI — UI doesn't render the URL) already have ctx in scope at the redaction site.

**Primary recommendation:** Build `internal/privfield` with a non-generic `Redact(ctx, visible, value string) (string, bool)` signature (second return = `omit`). Wire it differently per surface: direct struct-field nil-ing for pdbcompat (map[string]any), proto pointer nil for ConnectRPC (wrapperspb.StringValue), custom field resolver for GraphQL, response-body-rewriting middleware for entrest. E2E test extends existing `TestE2E_AnonymousCannotSeeUsersPoc` pattern but lands in a new sibling file `field_privacy_e2e_test.go` (D-10 discretion).

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| VIS-08 | Field-level visibility gate pattern | `internal/privfield` package, applied uniformly at each of 5 serializer layers; ctx reads `privctx.TierFrom` (already stamped by Phase 59's `middleware.PrivacyTier` at `cmd/peeringdb-plus/main.go:652`). Fail-closed default matches `privctx.TierFrom`'s own fallback (returns `TierPublic` when ctx is unstamped). |
| VIS-09 | `ixlan.ixf_ixp_member_list_url` exposed to authenticated callers only | Field added to `peeringdb.IxLan` (types.go:208-223), `ent/schema/ixlan.go` (**appended to end of field list** for proto wire stability), `internal/sync/upsert.go:313-340` (via `SetIxfIxpMemberListURL`), then redacted at serializer layer in all 5 surfaces. Auth sync already fetches the URL via existing Api-Key header; no client changes. |

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Tier stamping on request ctx | Frontend Server (middleware) | — | Phase 59's `middleware.PrivacyTier` already owns this at `main.go:652`. No change this phase. |
| Data ingestion (URL population) | Backend — sync worker | External (PeeringDB API) | Existing `internal/sync/upsert.go` ixlan path + existing `Api-Key` header at `internal/peeringdb/client.go:302`. |
| Schema / persistence | Backend — ent + SQLite | — | `ent/schema/ixlan.go`; ent auto-migrate adds the column. |
| Field-level redaction | Backend — serializer layer (per surface) | — | `internal/privfield.Redact` called from each surface's response assembly. D-01 locked. |
| Wire-level omission | Varies by surface | — | pdbcompat map-delete, ConnectRPC proto nil, GraphQL null, REST middleware JSON-rewrite, UI skip. |

## Standard Stack

### Core (existing — no new deps)

| Library | Version (go.mod) | Purpose | Why Standard |
|---------|---------|---------|--------------|
| entgo.io/ent | as-pinned | ORM + schema | Already used; field.String("…").Optional().Default("") is the canonical way to add a nullable-on-the-wire-but-default-"" field. |
| entgo.io/contrib/entproto | as-pinned | Proto gen from ent schema | Already used; generates `google.protobuf.StringValue` wrapper for optional strings → `*wrapperspb.StringValue` on Go side, nil = omitted. |
| entgo.io/contrib/entgql | as-pinned | GraphQL schema + ent binding | Already used; emits `ixfIxpMemberListURL: String` when the ent field is `Optional()`. |
| github.com/lrstanley/entrest | as-pinned | OpenAPI REST handler gen | Already used; emits `IxfIxpMemberListURL *string` with `omitempty` in the generated ent struct (see `ent/ixlan.go:25` for `ArpSponge` pattern). **No conditional omission hook.** |
| github.com/99designs/gqlgen | v0.17.89 (graph/generated.go:6) | GraphQL runtime | Already used; supports per-field resolver override via `gqlgen.yml` `models:` section. |
| connectrpc.com/connect | as-pinned | ConnectRPC runtime | Already used; generated proto Go types use `*wrapperspb.StringValue` for optional strings. |

### New

| Package | Purpose | Location |
|---------|---------|----------|
| `internal/privfield` | Field-level redaction helper. Single exported function; ~20 LOC of implementation + ~80 LOC of unit tests. | NEW package, placed next to `internal/privctx`. |

### No new external dependencies required.

**Version verification (existing pins):**
- `gqlgen v0.17.89` — confirmed from `graph/generated.go:6` comment; verified current-as-of-generation.
- `peeringdb-plus` module already vendors all codegen tools (`go tool buf`, `go tool templ`, `go tool gqlgen`).

## Architecture Patterns

### System Architecture — Field-level redaction flow

```
HTTP Request
    │
    ▼
Recovery → CORS → OTel → Logging → PrivacyTier ─► ctx now carries Tier
    │                                              (TierPublic anon, TierUsers if PDBPLUS_PUBLIC_TIER=users or OAuth stamps it)
    ▼
Readiness → SecurityHeaders → CSP → Caching → Compression
    │
    ▼
http.ServeMux routes to one of 5 surfaces ────────────────────────────────────────────────┐
                                                                                          │
    ┌─────────────┬─────────────────┬──────────────────┬─────────────────┬────────────────┤
    ▼             ▼                 ▼                  ▼                 ▼                ▼
 /api         /rest/v1          /peeringdb.v1.*    /graphql           /ui/          (/ui/ omitted:
(pdbcompat)  (entrest)          (ConnectRPC)      (gqlgen+entgql)                    UI doesn't
    │             │                 │                  │                            render URL)
    │             │                 │                  │
    │             │                 │                  │
    ▼             ▼                 ▼                  ▼
ixLanFromEnt  Server.handle…    ixLanToProto      IxLan field resolver
(needs ctx)   → ent.IxLan       (existing, ctx-   (NEW, ctx from
    │         → JSON encoder    aware via handler  graphql.ResolveField)
    │         (no hook!)        signature)             │
    │             │                 │                  │
    │             ▼                 │                  │
    │         restFieldRedact…      │                  │
    │         Middleware (NEW)      │                  │
    │         rewrites JSON body    │                  │
    │             │                 │                  │
    ▼             ▼                 ▼                  ▼
  ╭────────────────────────────────────────────────────────────╮
  │  internal/privfield.Redact(ctx, visible, value)            │
  │    ├─ reads privctx.TierFrom(ctx)                          │
  │    ├─ if TierUsers → (value, false) — keep                 │
  │    ├─ if TierPublic && visible == "Public" → (value, false)│
  │    └─ else → ("", true) — omit                             │
  │  Fail-closed: unstamped ctx → TierPublic → redact.         │
  ╰────────────────────────────────────────────────────────────╯
                          │
                          ▼
                   Serialized response
```

### Per-Surface Integration Points

| Surface | File | Current Signature | Integration Strategy |
|---------|------|-------------------|----------------------|
| pdbcompat | `internal/pdbcompat/serializer.go:258-275` `ixLanFromEnt` | `(l *ent.IxLan) peeringdb.IxLan` | **Thread ctx through.** Change signature to `(ctx, l)`. Update all call sites (5 in `depth.go` + 1 in `registry_funcs.go`). Post-build the return, call `privfield.Redact(ctx, l.IxfIxpMemberListURLVisible, l.IxfIxpMemberListURL)`; if omit=true, set `peeringdb.IxLan.IXFIXPMemberListURL = ""`. **BUT** the output is a struct with a `json:"ixf_ixp_member_list_url"` tag — without `omitempty`, an empty string still emits `"ixf_ixp_member_list_url":""`. D-04 says omit the key entirely, so either (a) add `,omitempty` to the json tag (cleanest), or (b) return `map[string]any` and delete the key when redacted. **Recommend (a)**: `json:"ixf_ixp_member_list_url,omitempty"`. Matches upstream exactly. |
| ConnectRPC | `internal/grpcserver/ixlan.go:166-182` `ixLanToProto(il *ent.IxLan)` | Takes only `*ent.IxLan` | Change signature to `ixLanToProto(ctx, il)`. Update 3 call sites (GetIxLan line 92, ListIxLans Convert line 125, StreamIxLans Convert line 160). Call `privfield.Redact(ctx, ...)`; if omit=true, leave `pb.IxLan.IxfIxpMemberListUrl` as nil (proto wrapperspb.StringValue → wire omission via proto3 semantics). |
| GraphQL | `graph/gqlgen.yml` + `graph/schema.resolvers.go` (new) | Direct struct access via generated code | Add `IxLan` to `models:` in `gqlgen.yml` with `fields: {ixfIxpMemberListURL: {resolver: true}}`. Re-run `go generate ./...`. gqlgen emits `IxLanResolver` interface with `IxfIxpMemberListURL(ctx, obj) (*string, error)`. Implement in `graph/custom.resolvers.go` (or a new file). Return `nil` when redacted, `&url` when visible. GraphQL null → omission. |
| entrest / REST | NEW middleware, wrap `ent/rest` handler in `main.go:304` | No per-field hook in entrest | Build `restFieldRedactMiddleware`. Use the same pattern as `restErrorMiddleware` (`main.go:527-569`) — buffer the response body, json.Decode, walk `{"id": …, "ixf_ixp_member_list_url": …}` entries (both single-object GetIxLan and `{"content": [...]}` paginated ListIxLans), `delete` the key when `privfield.Redact` says omit, json.Encode back. Apply only on `/rest/v1/ix-lans*` prefixes. |
| Web UI | `internal/web/templates/*.templ` | UI does not render the URL `[VERIFIED: grep "ixf_ixp_member_list_url" in internal/web found zero matches]` | **No code change needed.** Document the omission in the plan so the plan-checker doesn't flag it. If/when a future UI adds the URL, it must call `privfield.Redact` before rendering. |

### `internal/privfield` Package Shape

**Recommended signature:**

```go
// Package privfield provides serializer-layer field-level privacy redaction.
// It composes with internal/privctx (row-level tier stamping) but operates
// one level lower: on a single field within an already-admitted row.
package privfield

import (
    "context"

    "github.com/dotwaffle/peeringdb-plus/internal/privctx"
)

// Redact returns (value, false) if the caller's tier on ctx admits the
// field, or ("", true) if the serializer should omit the field entirely.
//
// Admission rules:
//   - visible == "Public"                → always admit (any tier)
//   - visible == "Users" && tier Users+  → admit
//   - visible == "Users" && tier Public  → redact (the gated case)
//   - visible == "Private"               → redact in all tiers (upstream parity)
//   - any unrecognized visible value     → redact (fail-closed)
//
// Fail-closed semantics:
// privctx.TierFrom(ctx) already returns TierPublic for un-stamped
// contexts, so an un-plumbed ctx naturally lands in the most
// restrictive branch — no extra check needed here.
func Redact(ctx context.Context, visible, value string) (out string, omit bool) {
    tier := privctx.TierFrom(ctx)

    switch visible {
    case "Public":
        return value, false
    case "Users":
        if tier >= privctx.TierUsers {
            return value, false
        }
        return "", true
    default:
        // "Private" or unknown → always redact.
        return "", true
    }
}
```

**Why non-generic:** D-02 specifies string-typed. Only one known use case (ixlan.ixf_ixp_member_list_url); no other `<field>_visible` companions exist in the schema per Phase 58's empirical audit (`internal/visbaseline/schema_alignment_test.go` locks this assumption). Going generic `Redact[T any]` buys flexibility we don't need and complicates the return signature (zero-value T vs. nil pointer). **Recommendation: stay non-generic.** If a future phase adds a non-string gated field, add a sibling `RedactInt`, `RedactBool`, etc. — YAGNI until the second caller exists.

**Why return `(string, bool)` not `*string`:**
- Matches the existing Go convention in this codebase (`stringVal`, `stringPtrVal` in `internal/grpcserver/*`).
- The second return is self-documenting at call sites (`if omit { … }`).
- Pointer-based would force call sites to allocate even when not redacting.

### Anti-Patterns to Avoid

- **Don't use a MarshalJSON on `ent.IxLan`** — it's generated code (`ent/ixlan.go:1` "DO NOT EDIT") and would be wiped on next `go generate`. Plus it has no ctx access.
- **Don't put the gate in an ent `Policy`** — ent privacy operates at query/mutation level (row admission), not field level. Stretching it to field-level requires interceptors that are significantly more complex than the serializer approach. CONTEXT.md D-01 explicitly rules this out.
- **Don't omit the `_visible` companion field even when redacting the URL** — D-05 locks this. Anon callers see `"ixf_ixp_member_list_url_visible": "Users"` next to a missing URL key. Matches upstream PeeringDB exactly.
- **Don't add the new ent field in the middle of the schema field list.** Proto wire compat depends on positional field numbers. Append to end.
- **Don't try to make one surface's redaction call another's.** Each surface calls `privfield.Redact` directly against its own data shape. Shared helper = `privfield.Redact`. Shared shape = no.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Tier lookup from ctx | Custom ctx key | `privctx.TierFrom(ctx)` | Phase 59 built this. Fail-closed default already baked in. |
| JSON field omission (struct path) | Conditional `map[string]any` construction in pdbcompat | Plain struct + `,omitempty` json tag + empty value | `encoding/json` does this natively. Matches upstream PeeringDB shape (they also omit). Keeps struct-based serialization tidy. |
| Proto wire omission | Custom proto encoder | `*wrapperspb.StringValue` with nil | Proto3 wrapper types are purpose-built for this. Already used for every optional string in this codebase (see `ent/ixlan.go:25` for `ArpSponge`). |
| Response-body rewriting (REST) | Full JSON schema walker | Targeted `restErrorMiddleware`-style wrapper | The existing middleware at `main.go:527-569` is the proven pattern. Only ~40 LOC; scope limited to `/rest/v1/ix-lans*`. |
| GraphQL field override | Custom `graphql.Marshaler` on ent type | gqlgen's `models:` resolver opt-in | gqlgen's native mechanism for "this one field needs custom logic." `[CITED: gqlgen.yml docs]` |
| Field-level gate enumeration across schema | Auto-audit macro | Manual CONTEXT.md checklist | Only one gated field known. Deferred per CONTEXT.md "Deferred Ideas". |

**Key insight:** Each surface has a native mechanism for "omit this field conditionally." Use each surface's native mechanism; don't invent a cross-cutting framework. `privfield.Redact` is the one decision point; wire-level omission is per-surface.

## Runtime State Inventory

> Phase 64 is an **additive schema + serializer change**. Standard migration/rename risks do not apply.

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | **SQLite on LiteFS — 8 machines.** ent auto-migrate adds the `ixf_ixp_member_list_url TEXT DEFAULT ''` column on next startup. Existing rows get `""`. The next scheduled sync cycle populates the 46 rows that have the URL upstream (28 Public + 18 Users). `[VERIFIED: testdata/visibility-baseline/beta/auth/api/ixlan row counts]` | No manual migration. Field populates on next sync (`PDBPLUS_SYNC_INTERVAL` default 1h). |
| Live service config | None. | None. |
| OS-registered state | None. | None. |
| Secrets / env vars | `PDBPLUS_PEERINGDB_API_KEY` already plumbed (`internal/peeringdb/client.go:97,108,131,301`). Required for the 18 `Users`-gated URLs; the 28 `Public` URLs come through anon too. | None — already operational. |
| Build artifacts | `gen/peeringdb/v1/v1.pb.go` + `ent/rest/*.go` + `graph/generated.go` + `graph/schema.graphqls` regenerated by `go generate ./...`. CI's drift check (`.golangci.yml` / CI job per CLAUDE.md §CI) enforces committed files match. | Run `go generate ./...` after schema change; commit all generated output. |

**Nothing found in category:** Live service config, OS-registered state — verified by inspection; this change is code-only + ent auto-migrate.

## Common Pitfalls

### Pitfall 1: Proto field number drift (wire-breaking)
**What goes wrong:** Adding `field.String("ixf_ixp_member_list_url")` adjacent to the `_visible` companion at schema position ~8 shifts every subsequent field number down by one.
**Why it happens:** entproto has NO explicit `entproto.Field(N)` annotations on any ent schema in this codebase `[VERIFIED: grep "entproto" ent/schema → zero matches]`. Field numbers are derived positionally from schema declaration order.
**How to avoid:** Append the new field at the **end** of the `IxLan.Fields()` slice, after `field.String("status")`. That gives it field number 14, with no renumbering.
**Warning signs:** `buf breaking` / any protobuf consumer sees field semantics change for an existing number. The CI drift check catches mismatches between schema + committed proto. Regenerate and diff `proto/peeringdb/v1/v1.proto` — the only new line should be `google.protobuf.StringValue ixf_ixp_member_list_url = 14;` at the bottom of the IxLan message.

### Pitfall 2: Empty string vs. missing key in pdbcompat
**What goes wrong:** Anon response shows `"ixf_ixp_member_list_url": ""` instead of omitting the key. Violates D-04.
**Why it happens:** Default `json:"ixf_ixp_member_list_url"` always emits. Needs `omitempty`.
**How to avoid:** Tag as `json:"ixf_ixp_member_list_url,omitempty"` in `internal/peeringdb/types.go`. Redaction writes empty string → `omitempty` causes key omission.
**Warning signs:** Anon pdbcompat parity test shows extra key. Compare `curl /api/ixlan/<id>` against upstream beta.peeringdb.com with no API key — they must be byte-identical (modulo ordering).

### Pitfall 3: GraphQL exposes the field even when omitted on wire
**What goes wrong:** `Pocs { ixfIxpMemberListURL }` returns an empty string rather than `null` for redacted rows.
**Why it happens:** Default gqlgen resolver reads `obj.IxfIxpMemberListURL` directly — empty string passes through as `""`, not null.
**How to avoid:** Mark the schema field as nullable (`String` not `String!` in gqlgen) AND use a custom resolver that returns `(*string, error)` with nil when redacted. With `field.String("…").Optional()` on the ent schema, entgql emits the GraphQL field as nullable by default. Verify `graph/schema.graphqls` shows `ixfIxpMemberListURL: String` (no bang).
**Warning signs:** E2E test assertions that check for null vs empty string. TierPublic query should show `"ixfIxpMemberListURL": null` in the data, not `"ixfIxpMemberListURL": ""`.

### Pitfall 4: entrest response-rewriting middleware breaks streaming
**What goes wrong:** The `restFieldRedactMiddleware` buffers the response body to parse+rewrite JSON, then writes the modified body. Collides with `http.Flusher` contract required elsewhere.
**Why it happens:** entrest's `/rest/v1/*` endpoints return complete JSON documents (not streamed), so buffering is fine here. But a naive wrapper could drop `http.Flusher`.
**How to avoid:** Mirror the `restErrorWriter` pattern at `main.go:536-569`: wrap `http.ResponseWriter`, buffer only when `Content-Type: application/json`, always implement `Unwrap() http.ResponseWriter` per CLAUDE.md §Middleware. For REST that is never streamed, a simpler `httptest.NewRecorder`-style capture + rewrite-on-flush is acceptable.
**Warning signs:** `go test -race ./cmd/peeringdb-plus` flags missing interfaces; manual `curl -N` against REST endpoints behaves normally (REST is non-streaming so flushing is a non-issue in practice).

### Pitfall 5: Test fixtures miss the URL field
**What goes wrong:** `internal/sync/testdata/refactor_parity.golden.json` and `internal/sync/integration_test.go` seeds don't carry `ixf_ixp_member_list_url`. Golden-file tests go green without exercising the new code path.
**Why it happens:** Seeds/fixtures were written before the field existed. Adding the field to the ent schema doesn't retroactively populate them.
**How to avoid:** Extend `internal/testutil/seed/` Full to add at least one ixlan with `ixf_ixp_member_list_url="https://example/members.json"` + `ixf_ixp_member_list_url_visible="Users"`, plus one with `Public`, plus one with `Private`. Mirror the 3-row pattern in the E2E test fixtures. Update `refactor_parity.golden.json` to include the new field in the expected output.
**Warning signs:** CI drift check passes but `cmd/peeringdb-plus/field_privacy_e2e_test.go` is trivially green (no rows to gate).

### Pitfall 6: `ixLanFromEnt` signature change cascades to 6 call sites
**What goes wrong:** Changing `ixLanFromEnt(l)` to `ixLanFromEnt(ctx, l)` breaks 6 callers, some of which don't have ctx in scope (maybe).
**Why it happens:** The old signature was ctx-less because the function is a pure mapper.
**How to avoid:** All 6 call sites (enumerated in grep above) already have ctx in scope because they are inside HTTP handlers:
- `internal/pdbcompat/depth.go:172,197,211,321,432` — all inside functions that take `ctx context.Context` as first arg.
- `internal/pdbcompat/registry_funcs.go:207` — inside a `func(ctx context.Context, …)` closure.
- `internal/pdbcompat/serializer_test.go:355,370` — tests; use `t.Context()` or `context.Background()`.

Mechanical refactor. No signature archaeology needed.
**Warning signs:** go vet flags unused ctx imports; `go build ./...` will catch any missed site.

## Code Examples

Verified patterns from this repo:

### Phase 59 E2E test — template for Phase 64 E2E test
```go
// Source: cmd/peeringdb-plus/e2e_privacy_test.go:340
func TestE2E_AnonymousCannotSeeUsersPoc(t *testing.T) {
    t.Parallel()
    fix := buildE2EFixture(t, privctx.TierPublic)
    // 5 surfaces asserted in nested t.Run blocks.
}
```

### Proto optional string handling (existing pattern in ixLanToProto)
```go
// Source: internal/grpcserver/ixlan.go:166
func ixLanToProto(il *ent.IxLan) *pb.IxLan {
    return &pb.IxLan{
        // …
        ArpSponge: stringPtrVal(il.ArpSponge), // *string → *wrapperspb.StringValue
    }
}
// Where stringPtrVal returns nil if input is nil or "".
```

New version:
```go
func ixLanToProto(ctx context.Context, il *ent.IxLan) *pb.IxLan {
    urlOut, omitURL := privfield.Redact(ctx, il.IxfIxpMemberListURLVisible, il.IxfIxpMemberListURL)
    var urlProto *wrapperspb.StringValue
    if !omitURL {
        urlProto = stringPtrVal(&urlOut) // or stringVal if we make urlOut non-empty
    }
    return &pb.IxLan{
        // …
        IxfIxpMemberListUrl: urlProto,
    }
}
```

### entrest response middleware (new — mirrors restErrorMiddleware)
```go
// Mirror: cmd/peeringdb-plus/main.go:527-569
func restFieldRedactMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Only intercept ixlan endpoints; pass through others.
        if !strings.HasPrefix(r.URL.Path, "/rest/v1/ix-lans") {
            next.ServeHTTP(w, r)
            return
        }
        rec := &bufferedJSONWriter{ResponseWriter: w}
        next.ServeHTTP(rec, r)
        // Now rewrite rec.body: parse, redact, re-encode.
        // See restErrorWriter for the WriteHeader/Write pattern.
    })
}
```

### gqlgen custom field resolver opt-in
```yaml
# Source: gqlgen docs — https://gqlgen.com/config/
# To add to graph/gqlgen.yml:
models:
  IxLan:
    fields:
      ixfIxpMemberListURL:
        resolver: true   # gqlgen emits IxLanResolver interface with this method
```

Then in `graph/custom.resolvers.go`:
```go
func (r *ixLanResolver) IxfIxpMemberListURL(ctx context.Context, obj *ent.IxLan) (*string, error) {
    v, omit := privfield.Redact(ctx, obj.IxfIxpMemberListURLVisible, obj.IxfIxpMemberListURL)
    if omit {
        return nil, nil
    }
    return &v, nil
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| ent privacy policy filters rows | Adds serializer-layer field filtering one level below | Phase 64 (this phase) | First field-level gate; complements row-level from Phase 59. |
| (None — greenfield) | `internal/privfield` package | Phase 64 | Substrate for v1.16+ OAuth gated fields per CONTEXT.md "Specifics". |

**Deprecated/outdated:** None — Phase 58 correctly identified no new ent fields were needed for row-level privacy; Phase 64 adds the one data field that Phase 58's row-level-scoped audit explicitly deferred.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `peeringdb.IxLan` struct field order does not matter for upstream JSON parity (only struct tag matters) | Pdbcompat integration | `[VERIFIED: encoding/json docs — field order by name, not position]` — not an assumption actually. Low risk. |
| A2 | gqlgen's `models:` `fields:` `resolver: true` pattern is the canonical way to override one field | GraphQL integration | If gqlgen has a different recommended approach, we might write extra boilerplate. Low risk — documented pattern. |
| A3 | Proto wire compat is the operative concern; there are no CBOR/Avro/other binary wire consumers | Schema integration | If any consumer exists that relies on field-number stability and wasn't considered, renumbering would silently break them. **Recommend human-verify the public proto contract before appending.** |
| A4 | The 28 Public URLs + 18 Users URLs in the beta fixture mirror production (`api.peeringdb.com`) within the same order-of-magnitude | Backfill reality check | If production has e.g. 2000 URL rows, sync memory/time is still fine (well under the 400 MB memory guardrail from CLAUDE.md). Low risk. |

## Open Questions

1. **Should `omitempty` on the json tag conflict with the Users-tier case where URL is present but empty?**
   - What we know: `""` as an actual URL is not a valid URL; upstream only emits the key when the URL is non-empty. So `omitempty` dropping on empty string is semantically correct.
   - What's unclear: If a real ixlan row has `_visible=Public` but `url=""` upstream, does upstream emit `"ixf_ixp_member_list_url": ""` or omit the key? Fixtures don't contain this edge case.
   - Recommendation: Go with `omitempty`. If parity diverges on a real production row, a quick follow-up can switch to `map[string]any`.

2. **Is the gqlgen `IxLan` type autobound or a pure ent type?**
   - What we know: `gqlgen.yml:14-18` autobinds `github.com/dotwaffle/peeringdb-plus/ent` — so `*ent.IxLan` is the GraphQL type.
   - What's unclear: Whether gqlgen's autobind + `fields: { ixfIxpMemberListURL: { resolver: true } }` plays nicely, or whether we need to register `IxLan` explicitly in models.
   - Recommendation: Try the simpler config first (just `fields:` block). If gqlgen complains, add explicit `model:` entry pointing to `*ent.IxLan`. Cheap iteration.

3. **Should the REST middleware ever rewrite Content-Length?**
   - What we know: If we omit a field, the body is shorter than what ent-rest's JSON encoder wrote. Naive `http.ResponseWriter` wrapping may leave stale Content-Length.
   - What's unclear: Whether entrest sets `Content-Length` explicitly (usually it doesn't — Go's net/http computes it on flush).
   - Recommendation: Research during implementation — the `bufferedJSONWriter` should reset `Content-Length` before writing the rewritten body. Worst case: set `Transfer-Encoding: chunked` by clearing `Content-Length`.

## Environment Availability

> Skip — no external dependencies beyond what is already in the project (ent, gqlgen, entproto, buf, templ — all Go tool deps already vendored per CLAUDE.md §"Code Generation").

## Validation Architecture

> `workflow.nyquist_validation: true` in `.planning/config.json`.

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing`; ent's `enttest` for in-memory SQLite; `httptest` for surface E2E |
| Config file | `go.mod` (Go 1.26+) — no separate test config |
| Quick run command | `go test -race ./internal/privfield/... ./cmd/peeringdb-plus/... ./internal/pdbcompat/... ./internal/grpcserver/... ./graph/...` |
| Full suite command | `go test -race ./...` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| VIS-08 | `privfield.Redact` returns correct (value, omit) for all (tier, visible) combinations | unit | `go test -race ./internal/privfield/...` | ❌ Wave 0 — new package |
| VIS-08 | Fail-closed when ctx has no tier | unit | `go test -race ./internal/privfield/...` | ❌ Wave 0 |
| VIS-09 | Anon `GET /api/ix-lans/{id}` returns no `ixf_ixp_member_list_url` key when row's visible="Users" | e2e | `go test -race -run TestE2E_FieldLevel_IxlanURL ./cmd/peeringdb-plus/...` | ❌ Wave 0 — new test file |
| VIS-09 | Users-tier `GET /api/ix-lans/{id}` returns the URL | e2e | same as above | ❌ Wave 0 |
| VIS-09 | Anon `GET /rest/v1/ix-lans/{id}` omits URL key (entrest middleware path) | e2e | same as above | ❌ Wave 0 |
| VIS-09 | Anon ConnectRPC `GetIxLan` returns nil for URL field | e2e | same as above | ❌ Wave 0 |
| VIS-09 | Anon GraphQL `{ ixLansList { ixfIxpMemberListURL } }` returns null for gated row | e2e | same as above | ❌ Wave 0 |
| VIS-09 | Public-visible URL ALWAYS exposed regardless of tier (ensures we don't over-redact) | e2e | same as above | ❌ Wave 0 |
| — | Existing pdbcompat parity does not regress | integration | `go test -race -run TestPdbcompatAnonParity ./internal/pdbcompat/...` | ✅ (regression guard) |
| — | Existing Phase 59 privacy E2E still passes | e2e | `go test -race -run TestE2E_AnonymousCannotSeeUsersPoc ./cmd/peeringdb-plus/...` | ✅ (regression guard) |
| — | Schema-alignment regression test in `internal/visbaseline` still passes after adding the field | integration | `go test -race ./internal/visbaseline/...` | ✅ (may need allowlist update — verify D-06 schema addition doesn't trigger the test) |
| — | Sync fixture golden parity | integration | `go test -race -run TestSyncParity ./internal/sync/...` | ✅ (regression — golden file may need update) |

### Sampling Rate
- **Per task commit:** `go test -race ./internal/privfield/... ./cmd/peeringdb-plus/...` — under 10 s.
- **Per wave merge:** `go test -race ./...` + `golangci-lint run` + `go generate ./...` drift-check + `govulncheck ./...`.
- **Phase gate:** Full suite + `fly deploy` dry-run of Dockerfile.prod + manual pdbcompat diff against beta.peeringdb.com.

### Wave 0 Gaps
- [ ] `internal/privfield/privfield.go` — new package implementation.
- [ ] `internal/privfield/privfield_test.go` — unit tests for the 4-case truth table + fail-closed ctx case (D-11).
- [ ] `cmd/peeringdb-plus/field_privacy_e2e_test.go` — NEW sibling file to `e2e_privacy_test.go`. Contains `TestE2E_FieldLevel_IxlanURL_RedactedAnon` + `TestE2E_FieldLevel_IxlanURL_VisibleToUsersTier`. Reuses `buildE2EFixture` with minor extension to seed 3 ixlan rows (Public URL, Users URL, Private URL).
- [ ] Update `internal/testutil/seed/Full` to seed 3 ixlan rows with mixed visibility.
- [ ] Update `internal/sync/testdata/refactor_parity.golden.json` for the new field.
- [ ] Update `internal/visbaseline/schema_alignment_test.go` allowlist — the test already knows `ixf_ixp_member_list_url` is auth-gated (line 81), so verify its assertions still hold after the ent schema gains the data field itself.

## Security Domain

> `security_enforcement` enabled by default.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | indirect | Existing — Api-Key header at peeringdb client. No auth surface changes this phase. |
| V3 Session Management | no | Stateless API; per-request tier stamping only. |
| V4 Access Control | **yes** | `privfield.Redact` is the new access-control enforcement point. Unit tests + 5-surface E2E cover the enforcement. Fail-closed default on un-stamped ctx per D-03. |
| V5 Input Validation | no | No new input; the URL field is populated only from upstream responses. |
| V6 Cryptography | no | No crypto change. |
| V8 Data Protection | **yes** | The URL itself is the data being protected. Correctness of redaction IS the data protection control. |
| V14 Config | indirect | `PDBPLUS_PUBLIC_TIER` (Phase 61) reused unchanged; fails-fast at startup per Phase 61. |

### Known Threat Patterns for field-level privacy

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Information disclosure via missed serializer | Information Disclosure | 5-surface E2E test asserts all surfaces at once (mirrors Phase 59 D-15 pattern). CI blocks regressions. |
| Side-channel: anon vs users byte-length difference confirms row has URL | Information Disclosure | Accepted minor tradeoff per D-05 — anon sees `"ixf_ixp_member_list_url_visible": "Users"` inline, which is already upstream-parity. Matches the attack surface PeeringDB itself has; no novel leak introduced. |
| Fail-open on bug: un-stamped ctx returns URL | Privilege Escalation | `privfield.Redact` relies on `privctx.TierFrom`, which returns `TierPublic` for un-stamped ctx. Unit test D-11 locks this. |
| Developer adds new gated field without redaction | Information Disclosure | `internal/visbaseline/schema_alignment_test.go` regression test (Phase 58) triggers if a new `_visible`-companion field appears. The plan should add a checklist item to the README/CONTRIBUTING that any new `_visible` field MUST add a corresponding `privfield.Redact` call site across all 5 surfaces. |

## Project Constraints (from CLAUDE.md)

- **GO-CS-5 (MUST)**: Input structs for >2 args — `Redact(ctx, visible, value)` is 3 args; ctx is explicitly exempted (§GO-CTX-1). Keep signature as-is. If a sibling `RedactWithLog(ctx, input RedactInput)` grows to 4+ args, wrap in struct.
- **GO-ERR-1 (MUST)**: Wrap errors with `%w`. `privfield.Redact` returns no error — decision is static. Call sites that thread ctx through may need `fmt.Errorf("redact ixlan url: %w", err)` only if reading the field could error (it can't; plain struct access).
- **GO-CTX-1/2 (MUST)**: ctx first, non-nil, honor Done. `privfield.Redact` takes ctx first; does not block; inherits Done trivially.
- **GO-T-1/2 (MUST)**: Table-driven tests with `-race`. Unit test for `privfield.Redact` is textbook table-driven (`(tier, visible) → (value, omit)` matrix).
- **GO-API-1 (MUST)**: Document exported items. Add godoc on `Redact` with full admission rules (see code example above).
- **GO-SEC-1/2**: Validate inputs — n/a (inputs are trusted, already in-DB). Never log secrets — n/a; URL is not a secret but also not logged.
- **CLAUDE.md §ConnectRPC**: "Proto optional fields generate pointer types" — confirmed; ixlan URL will be `*wrapperspb.StringValue`, check `!= nil` pattern on the Go side.
- **CLAUDE.md §Code Generation**: "Always commit generated ent/gen/graph/templ files alongside schema changes." Plan MUST include a commit boundary after `go generate ./...`.
- **CLAUDE.md §Schema & Visibility**: "Field-level visibility field (ixlan.ixf_ixp_member_list_url_visible) gates the sibling url field; the privacy policy nulls the value field when the visibility field is not Public." Phase 64 operationalises this — the CLAUDE.md sentence is aspirational pre-Phase 64. **Plan should update CLAUDE.md** to replace "the privacy policy nulls the value field" with "the `internal/privfield` helper nulls/omits the value field at serializer layer" — matches D-01.
- **CLAUDE.md §NULL handling**: "Ent auto-migrate adds `*_visible` columns with their declared defaults" — same applies to the new URL field. `Default("")` ensures existing rows get `""` not NULL. The privacy policy's NULL-as-default behaviour for the _visible column is unchanged.

## Sources

### Primary (HIGH confidence)
- `testdata/visibility-baseline/beta/auth/api/ixlan/page-{1,2}.json` — verified row counts (500 total, 46 with URL, 28 Public / 18 Users / 454 Private)
- `internal/peeringdb/client.go:97,108,131,301-302` — Api-Key header flow
- `internal/privctx/privctx.go` — fail-closed `TierFrom` semantics
- `ent/ixlan.go:1-50` — ent-generated struct showing json tags + optional string = `*string` pattern
- `proto/peeringdb/v1/v1.proto:258-281` — IxLan proto definition with sequential field numbers 1-13
- `ent/entc.go:79-85` — entproto extension has NO explicit field-number annotations; positional only
- `cmd/peeringdb-plus/e2e_privacy_test.go` — Phase 59 E2E test pattern (template for Phase 64)
- `cmd/peeringdb-plus/main.go:527-569` — `restErrorMiddleware` pattern (template for REST redaction middleware)
- `graph/generated.go:34-37` — current ResolverRoot only declares Query + SyncStatus; no type resolvers

### Secondary (MEDIUM confidence)
- [entrest annotation reference (WebFetch)](https://lrstanley.github.io/entrest/openapi-specs/annotation-reference/) — confirmed no per-request field-omission hook
- [entrest GitHub README (WebFetch)](https://github.com/lrstanley/entrest) — same finding
- gqlgen `models:` `fields:` `resolver: true` pattern — standard per gqlgen documentation convention

### Tertiary (LOW confidence)
- CONTEXT.md D-12 "18 rows get the URL" — **corrected to 46 rows** in findings above. (28 Public + 18 Users. The "18" was the Users-gated subset.)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all libraries already in use; no new deps; verified in repo
- Architecture: HIGH — per-surface integration points all mapped and verified by file inspection
- Pitfalls: HIGH — 6 pitfalls named with warning signs and verification commands
- Backfill reality: HIGH — fixture counts verified by scripting

**Research date:** 2026-04-18
**Valid until:** 2026-05-18 (30 days — stable codegen pipeline, no external dep churn expected)

---

*Phase: 64-field-level-privacy*
*Research: 2026-04-18*
