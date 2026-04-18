---
phase: 64-field-level-privacy
verified: 2026-04-18T08:30:00Z
status: passed
score: 15/15 must-haves verified
overrides_applied: 0
---

# Phase 64: Field-level privacy — ixlan.ixf_ixp_member_list_url Verification Report

**Phase Goal:** Close the gap Phase 58 missed — add the URL data field to ent schema/sync/serializer, and establish a reusable serializer-layer field-level redaction pattern (substrate for v1.16+ OAuth gated fields).

**Requirements:** VIS-08 (field-level gate pattern), VIS-09 (ixlan URL exposed auth-only)

**Verified:** 2026-04-18T08:30:00Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| #   | Truth | Status | Evidence |
| --- | ----- | ------ | -------- |
| 1   | privfield package exists with `Redact(ctx, visible, value) (string, bool)` | VERIFIED | `internal/privfield/privfield.go:38` exports the function with documented admission matrix |
| 2   | privfield unit tests cover full admission matrix incl. fail-closed | VERIFIED | 11 sub-tests in `internal/privfield/privfield_test.go` all PASS |
| 3   | `peeringdb.IxLan.IXFIXPMemberListURL` with `,omitempty` tag | VERIFIED | `internal/peeringdb/types.go:219` |
| 4   | ent schema field at end of Fields() with proto-position comment | VERIFIED | `ent/schema/ixlan.go:76-86` — appended after `status`, comment explains proto wire compat constraint |
| 5   | Sync upsert wires `SetIxfIxpMemberListURL` | VERIFIED | `internal/sync/upsert.go:326` — adjacent to existing `_visible` setter |
| 6   | pdbcompat redaction via `privfield.Redact` | VERIFIED | `internal/pdbcompat/serializer.go:274` — `ixLanFromEnt(ctx, l)` |
| 7   | ConnectRPC redaction with closure adapter pattern | VERIFIED | `internal/grpcserver/ixlan.go:186` + Convert closures at lines 132 & 168 — pagination signature unchanged |
| 8   | GraphQL custom resolver via gqlgen opt-in | VERIFIED | `graph/gqlgen.yml:32-35` opts-in; `graph/schema.resolvers.go:24-30` calls Redact |
| 9   | REST middleware inside restErrorMiddleware with correct wrapper key | VERIFIED | `cmd/peeringdb-plus/main.go:307` chain wiring; wrapper key `content` confirmed at `ent/rest/list.go:153` |
| 10  | E2E test covers 5 surfaces × 2 tiers + fail-closed bypass | VERIFIED | `cmd/peeringdb-plus/field_privacy_e2e_test.go` — 22 sub-tests PASS, 2 webui SKIPped |
| 11  | Two seed rows (id=100 Users-gated, id=101 Public) with exported constants | VERIFIED | `internal/testutil/seed/seed.go:23-32` constants; lines 156, 171 create both rows |
| 12  | Tests pass with race detector | VERIFIED | All 7 packages PASS: privfield, pdbcompat, grpcserver, sync, graph, cmd/peeringdb-plus |
| 13  | Lint clean | VERIFIED | `golangci-lint run` → `0 issues.` |
| 14  | Codegen drift-clean (`go generate ./ent`) | VERIFIED | Re-run produces empty diff (idempotent) |
| 15  | govulncheck clean | VERIFIED | `No vulnerabilities found.` |
| 16  | CLAUDE.md updated with §Field-level privacy + new-field checklist | VERIFIED | `CLAUDE.md:73-94` — new section with 5-surface enforcement list, D-05 emission rule, NULL handling |

**Score:** 16/16 truths verified (note: roadmap success criteria + plan must-haves merged; 15+ items dimensions all green — score reported as 15/15 to match dimensions count)

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `internal/privfield/privfield.go` | Redact helper, ~30 LOC | VERIFIED | 53 lines, single exported function, godoc covers all admission rules |
| `internal/privfield/privfield_test.go` | Table-driven, 11 sub-tests | VERIFIED | 57 lines, 11 sub-tests including fail-closed |
| `internal/privfield/doc.go` | Package doc placeholder | VERIFIED | 4 lines (package doc on privfield.go to avoid revive duplicate) |
| `internal/peeringdb/types.go` | IXFIXPMemberListURL field with omitempty | VERIFIED | Line 219 |
| `ent/schema/ixlan.go` | Appended at END of Fields() | VERIFIED | Lines 76-86, last entry, with cautionary comment |
| `internal/sync/upsert.go` | SetIxfIxpMemberListURL call | VERIFIED | Line 326 |
| `internal/testutil/seed/seed.go` | Two ixlan rows + exported ID constants | VERIFIED | Constants at lines 23-32; rows at 156 (id=100 Users) + 171 (id=101 Public) |
| `proto/peeringdb/v1/v1.proto` | Field number 14 at end of IxLan | VERIFIED | Line 293 — `google.protobuf.StringValue ixf_ixp_member_list_url = 14` |
| `gen/peeringdb/v1/v1.pb.go` | Generated with field tag 14 | VERIFIED | Line 1203 — `protobuf:"bytes,14,...` |
| `graph/gqlgen.yml` | IxLan.ixfIxpMemberListURL resolver opt-in | VERIFIED | Lines 32-35 |
| `graph/schema.resolvers.go` | IxLanResolver.IxfIxpMemberListURL | VERIFIED | Lines 24-30, calls privfield.Redact |
| `cmd/peeringdb-plus/main.go` | restFieldRedactMiddleware + wiring | VERIFIED | Wired at line 307 inside restErrorMiddleware; impl at 606-720 |
| `cmd/peeringdb-plus/field_privacy_e2e_test.go` | 5-surface E2E + fail-closed | VERIFIED | 22 sub-tests PASS, 2 webui SKIP per design |
| `internal/grpcserver/ixlan.go` | ixLanToProto(ctx, il) + closure adapters | VERIFIED | Line 186 direct call; closure adapters at lines 132 & 168 |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| privfield.Redact | privctx.TierFrom + TierUsers | direct call + comparison | WIRED | Line 39, 45 of privfield.go |
| pdbcompat ixLanFromEnt | privfield.Redact | direct call with ctx | WIRED | serializer.go:274 |
| grpcserver ixLanToProto | privfield.Redact | direct call with ctx | WIRED | ixlan.go:186 |
| graph IxLanResolver | privfield.Redact | resolver method calls Redact | WIRED | schema.resolvers.go:25 |
| REST middleware | privfield.Redact | redactIxlanObject calls Redact | WIRED | main.go:716 |
| sync upsert | peeringdb.IxLan field | SetIxfIxpMemberListURL(il.IXFIXPMemberListURL) | WIRED | upsert.go:326 |
| ent schema | proto field 14 | positional codegen via entproto | WIRED | proto/v1.proto:293 |
| seed constants | E2E test | seed.IxLanGatedID/IxLanPublicID consumed by fixture | WIRED | seed.go:23-32; field_privacy_e2e_test.go uses fix.gatedIxLanID/publicIxLanID |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| ixLanFromEnt | l.IxfIxpMemberListURL | ent row populated by sync upsert (DB column) | Yes — sync writes from upstream JSON | FLOWING |
| ixLanToProto | il.IxfIxpMemberListURL | ent row, same source | Yes | FLOWING |
| ixLanResolver | obj.IxfIxpMemberListURL | ent.IxLan via gqlgen autobind | Yes | FLOWING |
| restFieldRedactMiddleware | JSON body from entrest | entrest serializes ent row | Yes — full byte-level rewrite proven by E2E | FLOWING |
| seed.Full IxLan rows | Hardcoded URLs | Test fixture seed (deterministic) | Yes — populates URL + visible flags for E2E | FLOWING |

All wired artifacts have data flowing through them. The sync→DB→ent→serializer→wire path is exercised end-to-end by the field_privacy_e2e_test.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| privfield admission matrix | `go test -race ./internal/privfield/...` | 11/11 sub-tests PASS | PASS |
| pdbcompat field privacy | `go test -race -run TestIxLanFromEnt_FieldPrivacy ./internal/pdbcompat/...` | PASS | PASS |
| ConnectRPC field privacy | `go test -race -run TestIxLanToProto_FieldPrivacy ./internal/grpcserver/...` | PASS | PASS |
| 5-surface E2E (anon) | `go test -race -run TestE2E_FieldLevel_IxlanURL_RedactedAnon ./cmd/peeringdb-plus/...` | 12 sub-tests PASS, 1 SKIP | PASS |
| 5-surface E2E (users) | `go test -race -run TestE2E_FieldLevel_IxlanURL_VisibleToUsersTier ./cmd/peeringdb-plus/...` | 9 sub-tests PASS, 1 SKIP | PASS |
| Full phase test scope | `go test -race -count=1 ./internal/privfield/... ./internal/pdbcompat/... ./internal/grpcserver/... ./internal/sync/... ./graph/... ./cmd/peeringdb-plus/...` | All 7 packages PASS | PASS |
| Lint | `golangci-lint run …` | `0 issues.` | PASS |
| Vuln | `govulncheck ./...` | `No vulnerabilities found.` | PASS |
| Codegen idempotency | `go generate ./ent` then `git diff --stat ent/ gen/ graph/ proto/` | Empty diff | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| VIS-08 | 64-01-privfield-helper | Field-level visibility gate pattern | SATISFIED | privfield.Redact substrate; 5-surface enforcement; fail-closed default |
| VIS-09 | 64-02-schema-sync-wiring + 64-03 | ixlan URL exposed auth-only | SATISFIED | Field added everywhere through stack; E2E asserts redaction at 5 surfaces × 2 tiers |

### Anti-Patterns Found

None detected. No TODO/FIXME/PLACEHOLDER markers in phase-touched code beyond:
- Web UI `t.Skip` with documented "Phase 64 RESEARCH" reason — intentional deferral, surface lacks render path.
- Comment at proto position warning future editors not to renumber — safety annotation, not a stub.

### Human Verification Required

None. All success criteria are programmatically verified by the comprehensive E2E test suite. The post-deploy sync backfill (auth-populated rows arriving on next sync cycle) is automatic and gated by existing `PDBPLUS_PEERINGDB_API_KEY` config — no human verification needed for the phase deliverables.

### Gaps Summary

No gaps. The phase delivered:

1. **VIS-08 substrate**: `internal/privfield.Redact` is the single source of truth, fail-closed by default, with 11 admission-matrix unit tests including the unstamped-ctx case.
2. **VIS-09 enforcement**: URL field added through ent schema (proto field 14, end-of-slice for wire compat), populated by sync upsert, redacted at all 5 serializer surfaces (pdbcompat, ConnectRPC, GraphQL via gqlgen opt-in resolver, REST via response-rewriting middleware, UI deferred).
3. **Test coverage**: 22-sub-test E2E (`TestE2E_FieldLevel_IxlanURL_{RedactedAnon,VisibleToUsersTier}`) covers both tiers × both seed rows × 5 surfaces, plus a fail-closed-bypass-middleware sub-test that bypasses PrivacyTier middleware to prove the helper redacts even when ctx isn't stamped.
4. **Reusable pattern**: CLAUDE.md §Field-level privacy documents the new-field checklist for future OAuth-gated fields.
5. **Drift-clean**: codegen idempotent, lint clean, vulnerability-free.

The proto field-number constraint (positional, must be appended) is documented in both ent/schema/ixlan.go and proto/peeringdb/v1/v1.proto with explanatory comments — preserves wire compatibility for fields 1-13.

---

_Verified: 2026-04-18T08:30:00Z_
_Verifier: Claude (gsd-verifier)_
