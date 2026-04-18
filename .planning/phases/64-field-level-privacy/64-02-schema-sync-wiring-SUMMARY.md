---
phase: 64-field-level-privacy
plan: 02
subsystem: schema + sync
tags:
  - schema
  - sync
  - codegen
  - proto
dependency_graph:
  requires:
    - ent/schema/ixlan.go (pre-existing)
    - internal/sync/upsert.go (pre-existing)
    - internal/peeringdb/types.go (pre-existing)
    - internal/testutil/seed/seed.go (pre-existing)
    - testdata/visibility-baseline/beta/auth/api/ixlan/page-1.json (fixture)
  provides:
    - peeringdb.IxLan.IXFIXPMemberListURL (string, json tag ixf_ixp_member_list_url,omitempty)
    - ent.IxLan field ixf_ixp_member_list_url (TEXT NULL default "")
    - SetIxfIxpMemberListURL / IxfIxpMemberListURL getter on generated ent client
    - pb.IxLan.IxfIxpMemberListUrl (*wrapperspb.StringValue, proto field 14)
    - GraphQL field ixfIxpMemberListURL (nullable String)
    - seed.IxLanGatedID=100 (Users-gated row) and seed.IxLanPublicID=101 (Public row)
    - seed.Result.IxLanPublic typed handle
  affects:
    - internal/pdbcompat/serializer.go (Plan 64-03 will consume)
    - internal/grpcserver/ixlan.go (Plan 64-03 will consume)
    - graph/*.resolvers.go (Plan 64-03 will consume)
    - ent/rest (Plan 64-03 will consume via REST middleware)
tech_stack:
  added: []
  patterns:
    - "Positional proto field numbering: new ent fields go at END of Fields() slice to preserve wire compat"
    - "Hand-edited proto/peeringdb/v1/v1.proto for post-v1.6 schema changes (entproto.SkipGenFile + no entproto annotations means proto/ does NOT auto-regenerate)"
    - "seed.Full mixed-visibility pattern: paired Users-gated + Public rows with exported ID constants for downstream E2E targeting"
key_files:
  created:
    - internal/peeringdb/ixlan_fixture_test.go
  modified:
    - internal/peeringdb/types.go
    - internal/peeringdb/types_test.go
    - ent/schema/ixlan.go
    - internal/sync/upsert.go
    - internal/testutil/seed/seed.go
    - internal/testutil/seed/seed_test.go
    - internal/sync/testdata/refactor_parity.golden.json
    - proto/peeringdb/v1/v1.proto (hand-edited)
    - gen/peeringdb/v1/v1.pb.go (regenerated via buf generate)
    - ent/ixlan.go (regenerated)
    - ent/ixlan/ixlan.go (regenerated)
    - ent/ixlan/where.go (regenerated)
    - ent/ixlan_create.go (regenerated)
    - ent/ixlan_update.go (regenerated)
    - ent/migrate/schema.go (regenerated)
    - ent/mutation.go (regenerated)
    - ent/gql_collection.go (regenerated)
    - ent/gql_where_input.go (regenerated)
    - ent/rest/create.go (regenerated)
    - ent/rest/openapi.json (regenerated)
    - ent/rest/update.go (regenerated)
    - ent/runtime/runtime.go (regenerated)
    - graph/schema.graphqls (regenerated)
decisions:
  - "RESEARCH §Pitfall 2 override: json tag carries ,omitempty even though CONTEXT.md D-06 did not specify it — required for anon parity with upstream."
  - "Plan-level deviation (Rule 1): go generate ./ent does NOT regenerate proto/peeringdb/v1/v1.proto. Per CLAUDE.md §Schema hygiene, proto is frozen since v1.6; entproto.SkipGenFile combined with zero entproto.Message annotations means proto is hand-maintained. The plan assumed auto-regen; added field manually at position 14 and ran buf generate for the Go binding."
  - "seed_test.go IxLan count assertion updated 1→2 to match the new two-row seed fixture. TestFull nil/ID checks also gained IxLanPublic entries."
  - "Phase 57 auth fixture round-trip locked as a regression test in internal/peeringdb/ixlan_fixture_test.go."
metrics:
  duration: ~12m
  completed: 2026-04-18
  tasks_completed: 3
  commits: 4
---

# Phase 64 Plan 02: Schema + Sync Wiring for ixlan.ixf_ixp_member_list_url Summary

**One-liner:** Threads the auth-gated URL data field from upstream JSON through the ent schema, sync upsert, and two-row seed fixture — all codegen regenerated, proto field 14 claimed, and a fixture round-trip test locking the JSON decoder contract.

## Overview

Plan 64-02 adds `ixf_ixp_member_list_url` everywhere up to (but not including) the serializer layer. The URL column now exists in SQLite with default `""`, the sync upsert populates it from upstream responses, and `seed.Full` seeds both a Users-gated row (id=100) and a Public row (id=101) so Plan 64-03 can exercise both the redact and always-admit paths against real seed data.

The only deviation from the written plan concerned proto regeneration (see Deviations below) — the plan assumed `go generate ./ent` would emit the new proto field; in reality the proto file is frozen and had to be hand-edited. Everything else executed exactly as written.

## Tasks Executed

| # | Name | Commit | Files |
|---|------|--------|-------|
| 1 | Add IXFIXPMemberListURL to peeringdb.IxLan (TDD) | `0299f92` (RED), `6e87238` (GREEN) | internal/peeringdb/types.go, internal/peeringdb/types_test.go |
| 2 | Append ent field + regenerate ent/graph/proto/entrest | `0443ebc` | ent/schema/ixlan.go, ent/* (regen), gen/peeringdb/v1/v1.pb.go, proto/peeringdb/v1/v1.proto (hand), graph/schema.graphqls, ent/rest/* |
| 3 | Sync upsert + two-row seed + golden regen + fixture round-trip | `7a09c33` | internal/sync/upsert.go, internal/testutil/seed/seed.go, internal/testutil/seed/seed_test.go, internal/sync/testdata/refactor_parity.golden.json, internal/peeringdb/ixlan_fixture_test.go (new) |

## Exact Line Numbers (post-plan)

| File | Line(s) | Change |
|------|---------|--------|
| `internal/peeringdb/types.go` | 219 | New field `IXFIXPMemberListURL string \`json:"ixf_ixp_member_list_url,omitempty"\`` |
| `ent/schema/ixlan.go` | 77-87 | New appended field with cautionary comment explaining proto position constraint |
| `internal/sync/upsert.go` | 326 | `SetIxfIxpMemberListURL(il.IXFIXPMemberListURL)` — adjacent to the `_visible` setter on line 327 |
| `internal/testutil/seed/seed.go` | 20-34 | Exported `IxLanGatedID=100` and `IxLanPublicID=101` constants |
| `internal/testutil/seed/seed.go` | 148-165 | Updated primary IxLan (id=100): Users-gated URL |
| `internal/testutil/seed/seed.go` | 167-179 | NEW second IxLan (id=101): Public URL |
| `proto/peeringdb/v1/v1.proto` | 293 | New `google.protobuf.StringValue ixf_ixp_member_list_url = 14;` (hand-added) |
| `gen/peeringdb/v1/v1.pb.go` | 1203 | New `IxfIxpMemberListUrl *wrapperspb.StringValue` (via `buf generate`) |
| `ent/migrate/schema.go` | (within IxLan columns block) | New `{Name: "ixf_ixp_member_list_url", Type: field.TypeString, Nullable: true, Default: ""}` |

## Proto Field-Number Audit (auditor-facing)

**Pre-change IxLan message fields (1–13):**
```
1  int64 id
2  google.protobuf.Int64Value ix_id
3  google.protobuf.StringValue arp_sponge
4  google.protobuf.StringValue descr
5  bool dot1q_support
6  bool ixf_ixp_import_enabled
7  google.protobuf.StringValue ixf_ixp_member_list_url_visible
8  google.protobuf.Int64Value mtu
9  google.protobuf.StringValue name
10 google.protobuf.Int64Value rs_asn
11 google.protobuf.Timestamp created
12 google.protobuf.Timestamp updated
13 string status
```

**Post-change diff (proto/peeringdb/v1/v1.proto):**
```diff
--- a/proto/peeringdb/v1/v1.proto
+++ b/proto/peeringdb/v1/v1.proto
@@ -281,6 +281,16 @@ message IxLan {
   google.protobuf.Timestamp updated = 12;

   string status = 13;
+
+  // Phase 64 (VIS-09): auth-gated data field. Field-level privacy is
+  // enforced at the serializer layer (internal/privfield) before the
+  // *wrapperspb.StringValue is set — nil on the wire when redacted.
+  // Appended at field number 14 to preserve proto wire compat for
+  // fields 1-13; the proto file is frozen since v1.6 so this edit is
+  // applied by hand (entproto.SkipGenFile + no entproto annotations
+  // on ent schemas means proto/ is not auto-regenerated). Do NOT
+  // renumber.
+  google.protobuf.StringValue ixf_ixp_member_list_url = 14;
 }
```

Fields 1-13 are byte-identical pre/post. Field 14 is new. `ixf_ixp_member_list_url_visible` stayed at its existing number 7 (unchanged).

## Seed Diff — BOTH ixlan rows

```go
// Row 1 — Users-gated (id=100)
r.IxLan, err = client.IxLan.Create().
    SetID(IxLanGatedID).                                              // 100
    SetIxID(r.IX.ID).SetInternetExchange(r.IX).
    SetIxfIxpMemberListURL("https://example.test/ix/100/members.json").
    SetIxfIxpMemberListURLVisible("Users").
    SetCreated(Timestamp).SetUpdated(Timestamp).
    Save(ctx)
if err != nil { tb.Fatalf("seed: create IxLan: %v", err) }

// Row 2 — Public (id=101, NEW)
r.IxLanPublic, err = client.IxLan.Create().
    SetID(IxLanPublicID).                                             // 101
    SetIxID(r.IX.ID).SetInternetExchange(r.IX).
    SetIxfIxpMemberListURL("https://example.test/ix/101/members.json").
    SetIxfIxpMemberListURLVisible("Public").
    SetCreated(Timestamp).SetUpdated(Timestamp).
    Save(ctx)
if err != nil { tb.Fatalf("seed: create IxLanPublic: %v", err) }
```

## Caller Updates for seed.Full Shape Change

Pre-audit ran `grep -rn "IxfIxpMemberListURLVisible\|client.IxLan.Query().Count\|seed.Full" internal/ cmd/ graph/`. Exactly one test asserted the legacy single-row shape:

| File:Line | Before | After |
|-----------|--------|-------|
| `internal/testutil/seed/seed_test.go:108` | `{"IxLan", …Count…, 1}` | `{"IxLan", …Count…, 2}` (Phase 64 comment) |
| `internal/testutil/seed/seed_test.go:28` (TestFull nil checks) | IxLan only | +`{"IxLanPublic", r.IxLanPublic != nil}` |
| `internal/testutil/seed/seed_test.go:56` (TestFull id checks) | IxLan=100 only | +`{"IxLanPublic", r.IxLanPublic.ID, 101}` |

No other caller needed updating:
- `cmd/peeringdb-plus/rest_test.go:102` hand-rolls its own IxLan (id=1) — unrelated.
- `internal/sync/integration_test.go:151` asserts fixture-derived count of 2 for `ix_lans` — unrelated to seed.Full.
- `internal/testutil/seed/seed_mixed_visibility_test.go` exercises POC-only privacy — unaffected.
- `internal/pdbcompat/anon_parity_test.go`, `cmd/peeringdb-plus/privacy_surfaces_test.go` — do not assert IxLan row counts or `_visible` values.

## `git status --short` after `go generate ./ent` (Task 2, pre-proto-edit)

```
 M ent/gql_collection.go
 M ent/gql_where_input.go
 M ent/ixlan.go
 M ent/ixlan/ixlan.go
 M ent/ixlan/where.go
 M ent/ixlan_create.go
 M ent/ixlan_update.go
 M ent/migrate/schema.go
 M ent/mutation.go
 M ent/rest/create.go
 M ent/rest/openapi.json
 M ent/rest/update.go
 M ent/runtime/runtime.go
 M ent/schema/ixlan.go
 M graph/schema.graphqls
```

**Notable absence:** `proto/peeringdb/v1/v1.proto` was NOT touched by `go generate ./ent`. See the Deviations section below. Post hand-edit + `buf generate`, `proto/peeringdb/v1/v1.proto` and `gen/peeringdb/v1/v1.pb.go` were both staged.

## Test Output

### Targeted sync/peeringdb/testutil run

```
TMPDIR=/tmp/claude-1000 go test -race -count=1 ./internal/sync/... ./internal/peeringdb/... ./internal/testutil/...

ok  	github.com/dotwaffle/peeringdb-plus/internal/sync	7.768s
ok  	github.com/dotwaffle/peeringdb-plus/internal/peeringdb	1.358s
?   	github.com/dotwaffle/peeringdb-plus/internal/testutil	[no test files]
ok  	github.com/dotwaffle/peeringdb-plus/internal/testutil/seed	1.609s
```

### ent/ and broader downstream consumers

```
TMPDIR=/tmp/claude-1000 go test -race -count=1 ./ent/... ./internal/pdbcompat/... ./internal/grpcserver/... ./graph/... ./cmd/peeringdb-plus/...

ok  	github.com/dotwaffle/peeringdb-plus/ent/schema	1.423s
ok  	github.com/dotwaffle/peeringdb-plus/internal/pdbcompat	2.313s
ok  	github.com/dotwaffle/peeringdb-plus/internal/grpcserver	4.464s
ok  	github.com/dotwaffle/peeringdb-plus/graph	2.132s
ok  	github.com/dotwaffle/peeringdb-plus/cmd/peeringdb-plus	2.158s
```

### Full-tree sanity

```
TMPDIR=/tmp/claude-1000 go test -race -count=1 ./... | grep FAIL | wc -l
0
```

### Lint

```
TMPDIR=/tmp/claude-1000 golangci-lint run
0 issues.
```

### Idempotency

`go generate ./ent` run twice produces an identical working tree the second time. No codegen drift.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Plan assumed go generate ./ent auto-regenerates proto/peeringdb/v1/v1.proto**

- **Found during:** Task 2 — ran `go generate ./ent` and `git status --short` showed zero changes under `proto/` or `gen/`.
- **Issue:** The plan's `<action>` step 4 said `grep "ixf_ixp_member_list_url = 14" proto/peeringdb/v1/v1.proto` would succeed post-regen. It did not; the proto file was byte-identical after codegen. Root cause: `ent/entc.go` registers the entproto extension with `entproto.SkipGenFile()`, AND no ent schema in `ent/schema/*.go` carries an `entproto.Message()` annotation. entproto emits proto messages ONLY for schemas with that annotation, so the proto file was generated once in v1.6 and has been hand-maintained since (CLAUDE.md §Schema hygiene confirms: "Proto is frozen since v1.6"). The Phase 64 RESEARCH.md correctly identified that proto field numbers are positional and that the new field must go at the end, but it assumed regeneration would happen — it did not.
- **Fix:** Hand-added `google.protobuf.StringValue ixf_ixp_member_list_url = 14;` at the end of the IxLan message in `proto/peeringdb/v1/v1.proto` with an explanatory comment about the frozen-proto constraint and the field-number positional requirement. Then ran `go tool buf generate` to produce the matching `gen/peeringdb/v1/v1.pb.go` binding.
- **Files modified:** `proto/peeringdb/v1/v1.proto` (+10 lines including comment), `gen/peeringdb/v1/v1.pb.go` (regenerated).
- **Commit:** `0443ebc` (combined with the ent schema change for atomic landing — CLAUDE.md §"Code Generation" mandates committing codegen alongside the schema change).

No other deviations. The remaining plan tasks executed exactly as written.

## Known Stubs

None. The URL is fully populated in the DB; only the serializer-layer redaction is outstanding and that is Plan 64-03's explicit scope.

## Threat Flags

None. The threat register in the plan already covered T-64-04 (proto field-number drift — mitigated by end-of-slice append + comment) and T-64-05 (SQLite column default — mitigated by empty-string default triggering omitempty). No new surfaces introduced.

## What Plan 64-03 Can Assume

- `ent.IxLan.IxfIxpMemberListURL` (string) — always populated from DB (may be `""`).
- `pb.IxLan.IxfIxpMemberListUrl` (`*wrapperspb.StringValue`, proto field 14) — set by the ConnectRPC converter in Plan 64-03 to `wrapperspb.String(url)` when admitted, left nil when redacted.
- GraphQL field `IxLan.ixfIxpMemberListURL: String` (nullable).
- REST field `IxfIxpMemberListURL *string` (JSON omitempty).
- Seed helpers: `seed.IxLanGatedID = 100` (Users-gated, URL populated), `seed.IxLanPublicID = 101` (Public, URL populated). Use `seed.Result.IxLanPublic` for the typed handle.
- `peeringdb.IxLan.IXFIXPMemberListURL` is populated by the sync JSON decoder via the Phase 57 auth fixture round-trip; Plan 64-03 does not need to re-verify this.

## Self-Check: PASSED

Created files exist:
- `internal/peeringdb/ixlan_fixture_test.go` — FOUND
- `.planning/phases/64-field-level-privacy/64-02-schema-sync-wiring-SUMMARY.md` — FOUND (this file)

Commits exist in git log:
- `0299f92` test(64-02): add failing IxLan URL JSON round-trip test — FOUND
- `6e87238` feat(64-02): add IXFIXPMemberListURL to peeringdb.IxLan struct — FOUND
- `0443ebc` feat(64-02): add ixf_ixp_member_list_url ent field at end of Fields() — FOUND
- `7a09c33` feat(64-02): wire ixlan URL sync upsert + seed two rows (Users/Public) — FOUND

Key artefacts verified:
- `peeringdb.IxLan.IXFIXPMemberListURL` exists with `,omitempty` tag: FOUND (internal/peeringdb/types.go:219)
- ent field appended at end of Fields(): FOUND (ent/schema/ixlan.go:83)
- `SetIxfIxpMemberListURL` call in sync upsert: FOUND (internal/sync/upsert.go:326)
- Two ixlan rows in seed.Full: FOUND (internal/testutil/seed/seed.go:152,167)
- Proto field 14: FOUND (proto/peeringdb/v1/v1.proto:293)
- Generated `IxfIxpMemberListUrl` on `pb.IxLan` with field number 14: FOUND (gen/peeringdb/v1/v1.pb.go:1203)
- Fixture round-trip test passes: VERIFIED
- `golangci-lint run`: 0 issues
- Full-tree `go test -race ./...`: 0 failures
- `go generate ./ent` idempotent: VERIFIED
