# Phase 64: Field-level privacy — ixlan.ixf_ixp_member_list_url - Context

**Gathered:** 2026-04-17
**Status:** Ready for planning

<domain>
## Phase Boundary

Close the gap Phase 58 missed: the `ixf_ixp_member_list_url` URL data field itself is missing from our schema, struct, sync, and serializers (we have only the `_visible` companion). Upstream exposes this URL auth-only for 18/500 rows. This phase:

1. Adds the URL field all the way down the stack so we capture it on authenticated sync.
2. Establishes a **reusable field-level redaction pattern** (new `internal/privfield` package) that blanks the URL for anonymous callers when the companion `_visible` field is not `"Public"`. ent's privacy package is query-level only; this is the first field-level gate in the codebase.

Phase 59 established row-level privacy for POC. Phase 64 establishes field-level privacy for ixlan.ixf_ixp_member_list_url — and builds the pattern that any future gated field plugs into.

</domain>

<decisions>
## Implementation Decisions

### Redaction architecture
- **D-01: Serializer-layer redaction.** Each surface's serializer (pdbcompat/REST/GraphQL/ConnectRPC/UI) checks the companion `_visible` field + `privctx.TierFrom(ctx)` and omits the URL accordingly. Keeps ent's Query path unchanged.
- **D-02: Reusable pattern via `internal/privfield` package.** Export `Redact(ctx context.Context, visible string, value string) (string, bool)` returning `(value, omit)`. `omit=true` means the serializer should exclude the key entirely from its output.
- **D-03: Fail-closed on missing ctx tier.** If `privctx.TierFrom(ctx)` returns the zero value / unset, treat as `TierPublic` (the most restrictive) and redact. Safer default — prevents accidental leak via unstamped contexts.

### Anonymous output shape
- **D-04: Omit the key entirely** when tier=Public and `_visible != "Public"`. Response has no `ixf_ixp_member_list_url` key at all. Matches upstream's behaviour exactly (they also omit). Pure shape parity.
- **D-05: Leave `_visible` exposed as-is.** Anon callers see `ixf_ixp_member_list_url_visible: "Users"` when the gate is active. Matches upstream emission. Minor info-disclosure tradeoff accepted for parity.

### ixlan.IxfIxpMemberListURL field additions
- **D-06:** Add `IXFIXPMemberListURL string \`json:"ixf_ixp_member_list_url"\`` to `peeringdb.IxLan`. Regular string field; upstream omits it anon, present in auth response for some rows — JSON decoder handles missing gracefully (default zero value = `""`).
- **D-07:** Add `field.String("ixf_ixp_member_list_url").Optional().Default("")` to `ent/schema/ixlan.go`. Not a `_visible`-gated schema field (those are row-level visibility flags; this is a regular data field whose visibility is governed at serializer layer per D-01).
- **D-08:** Wire `SetIxfIxpMemberListURL(il.IXFIXPMemberListURL)` in `internal/sync/upsert.go` ixlan path.
- **D-09:** Emit via the serializer-layer redaction helper in all 5 surfaces.

### Test surface
- **D-10: Mirror Phase 59's D-15 E2E test pattern.** New `TestE2E_FieldLevel_IxlanURL_RedactedAnon` (and companion `_VisibleToUsersTier`) spanning all 5 surfaces. Asserts the URL field is absent from anon responses when `_visible=Users` and present when tier=Users.
- **D-11:** Unit tests for `internal/privfield.Redact`: TierPublic + visible=Public → keep; TierPublic + visible=Users → omit; TierUsers + visible=anything → keep; missing tier → omit (fail-closed per D-03).

### Upsert sync path
- **D-12:** Existing authenticated sync will populate the URL for the 18 rows that have it. Backfill happens automatically on the next scheduled sync cycle (`PDBPLUS_SYNC_INTERVAL`, default 1h). No manual trigger needed — same pattern as v1.14's Users-tier POC rows.

### Claude's Discretion
- Exact name/layout of the E2E test file (likely `cmd/peeringdb-plus/field_privacy_e2e_test.go` alongside existing `e2e_privacy_test.go`)
- Whether to extract a shared redaction helper across all 5 serializers now, or accept some duplication and extract in a follow-up
- The specific method signature: `privfield.Redact(ctx, visible, value) (string, bool)` vs a pointer-based `RedactField(ctx, visible, &value)` that modifies in place — implementation detail

### Folded Todos
None.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Plan-of-record
- `.planning/REQUIREMENTS.md` — VIS-08 (field-level gate pattern), VIS-09 (ixlan URL field exposed to authenticated callers only)
- `.planning/ROADMAP.md` §"Phase 64" — success criteria

### Predecessor outputs
- v1.14 Phase 58 — schema audit missed this field; its conclusion ("no new ent fields needed") was scoped to _visible companions only
- v1.14 Phase 59 — row-level privacy pattern (privctx + ent Policy) that this phase composes with at the field level
- Phase 57 fixtures: `testdata/visibility-baseline/beta/auth/api/ixlan/page-1.json` includes `ixf_ixp_member_list_url` on 18 rows as `"<auth-only:string>"`

### ent / entproto / gqlgen
- entproto annotations on ixlan need updating when the field is added — CLAUDE.md notes entproto generates proto types from ent
- entrest annotations on ixlan need the field for REST API shape

### Existing code this phase modifies
- `internal/peeringdb/types.go` — add `IXFIXPMemberListURL` to `IxLan` struct at ~line 146
- `ent/schema/ixlan.go` — add `field.String("ixf_ixp_member_list_url")` next to existing `_visible` companion
- `internal/sync/upsert.go` — wire `SetIxfIxpMemberListURL` in the ixlan upsert path (existing `SetIxfIxpMemberListURLVisible` is at ~line 326)
- `internal/pdbcompat/serializer.go` — `ixLanFromEnt` adds the URL via `privfield.Redact(ctx, l.IxfIxpMemberListURLVisible, l.IxfIxpMemberListURL)`
- `graph/custom.resolvers.go` or generated `graph/*_resolvers.go` — GraphQL field resolver applies the same redaction
- `internal/grpcserver/ixlan.go` — ConnectRPC handler applies the same redaction before wrapping in proto
- `ent/rest/*.go` — entrest-generated REST response path (may need a custom output hook if entrest doesn't support per-field conditional emission)
- `internal/web/templates/*.templ` (if any reference this field on /ui/) — add the redaction-aware rendering
- **NEW:** `internal/privfield/privfield.go` + `_test.go` — the shared helper

### Project conventions
- `CLAUDE.md` §"ConnectRPC" — proto `optional` fields generate pointer types; IXFIXPMemberListURL will be a `*string` on the proto side (omitted when nil)
- `CLAUDE.md` §"Middleware" — privacy tier middleware from Phase 59 stamps the ctx early in the chain; all serializers downstream can read it via `privctx.TierFrom(ctx)`

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/privctx.TierFrom(ctx)` from Phase 59 — the tier lookup helper that `privfield.Redact` will call
- Phase 59's `TestE2E_AnonymousCannotSeeUsersPoc` is the template for the D-10 E2E test
- Phase 60's `seed.Full` has mixed-visibility fixtures; extend with a mixed-visibility ixlan if helpful for tests

### Established Patterns
- Serializer functions (`ixLanFromEnt`, `networkFromEnt`, etc.) already take `ctx` in some paths. Audit whether all 5 surface serializers have ctx access; if not, threading ctx through is a subtask.
- CLAUDE.md §"GO-CS-5" — input struct for >2 args. If `Redact` grows to 4+ args it should be a struct.

### Integration Points
- entrest may not have a native "omit field conditionally" hook. If so, the REST serializer may need a post-process step that mutates the JSON response before writing, or a custom resolver.
- ConnectRPC proto fields: the URL field would be `*string` if marked `optional` in proto; absent = nil pointer. That's the "omit" signal on the proto side.

</code_context>

<specifics>
## Specific Ideas

- **Field-level redaction is a new architectural pattern.** Designing it for reuse (per user decision) means future schema additions that need field-level gating plug in via one line. Don't tightly couple the helper to ixlan specifics.
- **Fail-closed matters.** If Phase 59's privacy middleware ever fails to stamp ctx (bug in the chain, direct handler invocation in a test that forgets), the field must still redact. User explicitly chose fail-closed over fail-open.
- **Shape parity with upstream is the most visible test.** Once the field is added, re-run pdbcompat anon parity on ixlan; it should show the URL absent for anon and present for auth.

</specifics>

<deferred>
## Deferred Ideas

- **Field-level redaction on other fields.** No other field-level gates are known post-audit. If future fields need this, the `internal/privfield` helper is ready.
- **Automated field-level coverage test.** A test that enumerates all `_visible`-companioned fields in the schema and asserts the URL-variant pair is redacted correctly. Over-engineered for one field; revisit if the pattern grows.
- **Emit upstream's 403 on direct lookup.** If someone directly queries `/api/ixlan/{id}` where the URL is Users-only, our response could 404 the whole row (Phase 59 pattern). We're choosing field-level redaction instead — same row visible, one field omitted. Matches upstream behaviour for this specific field.

</deferred>

---

*Phase: 64-field-level-privacy*
*Context gathered: 2026-04-17*
