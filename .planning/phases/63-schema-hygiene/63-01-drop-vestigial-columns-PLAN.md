---
phase: 63-schema-hygiene
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - ent/schema/ixprefix.go
  - ent/schema/organization.go
  - internal/peeringdb/types.go
  - internal/pdbcompat/serializer.go
  - internal/pdbcompat/registry.go
  - internal/pdbcompat/anon_parity_test.go
  - internal/pdbcompat/serializer_test.go
  - internal/pdbcompat/testdata/golden/ixpfx/list.json
  - internal/pdbcompat/testdata/golden/ixpfx/detail.json
  - internal/pdbcompat/testdata/golden/ixpfx/depth.json
  - internal/grpcserver/ixprefix.go
  - internal/sync/upsert.go
  - cmd/peeringdb-plus/main.go
  - ent/
  - gen/peeringdb/v1/
  - graph/
  - proto/peeringdb/v1/v1.proto
  - .planning/PROJECT.md
  - docs/ARCHITECTURE.md
  - CLAUDE.md
  - docs/API.md
autonomous: true
requirements:
  - HYGIENE-01
  - HYGIENE-02
  - HYGIENE-03
must_haves:
  truths:
    - "ent schema no longer declares ixprefix.notes, organization.fac_count, or organization.net_count"
    - "pdbcompat /api/ixpfx response omits the notes key entirely (matches upstream PeeringDB shape)"
    - "pdbcompat /api/org response continues to omit fac_count and net_count keys (no regression)"
    - "ent auto-migrate on primary startup drops the three DB columns (via migrate.WithDropColumn(true))"
    - "go generate ./... produces zero drift after schema edits; CI drift check passes"
    - "TestAnonParityFixtures passes with the ixpfx|data[0].notes|extra_field entry removed from knownDivergences"
    - "go test -race ./... and golangci-lint run both clean"
  artifacts:
    - path: "ent/schema/ixprefix.go"
      provides: "IxPrefix schema without notes field"
      contains_not: "field.String(\"notes\")"
    - path: "ent/schema/organization.go"
      provides: "Organization schema without fac_count / net_count fields"
      contains_not: "field.Int(\"fac_count\")"
    - path: "internal/peeringdb/types.go"
      provides: "IxPrefix wire struct without Notes"
      contains_not: "Notes    string    `json:\"notes\"`"
    - path: "internal/pdbcompat/serializer.go"
      provides: "ixPrefixFromEnt without Notes emission"
      contains_not: "Notes:    p.Notes"
    - path: "cmd/peeringdb-plus/main.go"
      provides: "Auto-migrate with WithDropColumn + WithDropIndex enabled"
      contains: "migrate.WithDropColumn(true)"
    - path: "internal/pdbcompat/anon_parity_test.go"
      provides: "knownDivergences map without ixpfx.notes entry"
      contains_not: "ixpfx|data[0].notes|extra_field"
  key_links:
    - from: "cmd/peeringdb-plus/main.go:Schema.Create call"
      to: "ent/migrate package"
      via: "variadic MigrateOption args"
      pattern: "migrate\\.WithDropColumn\\(true\\)"
    - from: "internal/pdbcompat/serializer.go:ixPrefixFromEnt"
      to: "internal/peeringdb/types.go:IxPrefix"
      via: "struct literal without Notes field"
      pattern: "ixPrefixFromEnt"
    - from: "TestAnonParityFixtures"
      to: "internal/pdbcompat/testdata/visibility-baseline/beta/anon/api/ixpfx"
      via: "fixture replay with zero divergences"
      pattern: "TestAnonParityFixtures"
---

<objective>
Drop three confirmed-vestigial ent schema fields (ixprefix.notes, organization.fac_count, organization.net_count), regenerate all downstream codegen layers, update all hand-written references, wire migrate.WithDropColumn(true) + WithDropIndex(true) at the single runtime migrate site, and complete the doc sweep mandated by D-05. This closes the v1.14-deferred ixpfx.notes pdbcompat divergence by removing the field rather than allow-listing it.

Purpose: Eliminate schema debt confirmed vestigial by the post-v1.14 schema-vs-fixture audit. Align our API shape with upstream PeeringDB for /api/ixpfx and keep /api/org unchanged while cleaning internal schema.

Output: Schema without the three fields, clean codegen, live DB columns dropped on next primary startup, TestAnonParityFixtures passing with the ixpfx.notes divergence entry removed, full doc sweep landed.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/STATE.md
@.planning/REQUIREMENTS.md
@.planning/phases/63-schema-hygiene/63-CONTEXT.md
@.planning/phases/63-schema-hygiene/63-RESEARCH.md
@CLAUDE.md

Source files that will be edited (read before touching):
@ent/schema/ixprefix.go
@ent/schema/organization.go
@internal/peeringdb/types.go
@internal/pdbcompat/serializer.go
@internal/pdbcompat/registry.go
@internal/pdbcompat/anon_parity_test.go
@internal/grpcserver/ixprefix.go
@internal/sync/upsert.go
@cmd/peeringdb-plus/main.go

<interfaces>
Key contracts extracted from codebase. Executor uses these directly — no exploration needed.

From ent/migrate/migrate.go (re-exports of entgo.io/ent/dialect/sql/schema):

    // Available as package-level vars in github.com/dotwaffle/peeringdb-plus/ent/migrate
    var (
        WithDropColumn = schema.WithDropColumn // enables ALTER TABLE DROP COLUMN
        WithDropIndex  = schema.WithDropIndex  // companion for stale index hygiene
    )

From cmd/peeringdb-plus/main.go (current state, lines 112-118):

    // Auto-migrate schema on primary per D-43.
    if isPrimary {
        if err := entClient.Schema.Create(ctx); err != nil {
            logger.Error("failed to migrate schema", slog.Any("error", err))
            os.Exit(1)
        }
    }

From internal/peeringdb/types.go (current IxPrefix, lines 225-236 — DROP the Notes line):

    type IxPrefix struct {
        ID       int       `json:"id"`
        IXLanID  int       `json:"ixlan_id"`
        Protocol string    `json:"protocol"`
        Prefix   string    `json:"prefix"`
        InDFZ    bool      `json:"in_dfz"`
        Notes    string    `json:"notes"`
        Created  time.Time `json:"created"`
        Updated  time.Time `json:"updated"`
        Status   string    `json:"status"`
    }

From internal/pdbcompat/serializer.go (current ixPrefixFromEnt, around lines 290-302 — DROP the Notes assignment):

    func ixPrefixFromEnt(p *ent.IxPrefix) peeringdb.IxPrefix {
        return peeringdb.IxPrefix{
            ID:       p.ID,
            IXLanID:  derefInt(p.IxlanID),
            Protocol: p.Protocol,
            Prefix:   p.Prefix,
            InDFZ:    p.InDfz,
            Notes:    p.Notes,
            Created:  p.Created,
            Updated:  p.Updated,
            Status:   p.Status,
        }
    }

From internal/grpcserver/ixprefix.go — three hunks to remove at lines 41-42, 59-60, 150:

    // Line 41-42 (in ListIxPrefixes filter block) — REMOVE both lines:
    eqFilter(func(r *pb.ListIxPrefixesRequest) *string { return r.Notes },
        fieldEQString(ixprefix.FieldNotes)),

    // Line 59-60 (in StreamIxPrefixes filter block) — REMOVE both lines:
    eqFilter(func(r *pb.StreamIxPrefixesRequest) *string { return r.Notes },
        fieldEQString(ixprefix.FieldNotes)),

    // Line 150 (in proto conversion struct literal) — REMOVE this single line:
    Notes:    stringVal(ixp.Notes),

From internal/sync/upsert.go line 353 (IxPrefix upsert chain):

    SetNotes(ip.Notes).  // DROP THIS LINE from IxPrefix upsert chain only
    // NOTE: DO NOT touch SetNotes(o.Notes) at line 84 (Organization — keeps its Notes field)
    // NOTE: DO NOT touch SetNotes at lines 125, 171, 217, 274, 422, 519 (other entities)

From internal/pdbcompat/registry.go line 269 (TypeIXPfx block):

    // REMOVE this one line from the TypeIXPfx map entry:
    "notes":    FieldString,
    // NOTE: DO NOT touch "notes" entries at lines 79, 120, 162, 197, 284, 341, 373 (other entities)

From internal/pdbcompat/anon_parity_test.go (lines 55-70 — leave an empty map):

    var knownDivergences = map[string]struct{}{
        // REMOVE entire comment block (lines 56-68) AND the map entry at line 69:
        "ixpfx|data[0].notes|extra_field": {},
    }
    // Result: knownDivergences should be an empty map{} (no entries).

Migrate site AFTER edit (target state for cmd/peeringdb-plus/main.go):

    import (
        // ... existing imports, alphabetically placed:
        "github.com/dotwaffle/peeringdb-plus/ent/migrate"
    )

    // Auto-migrate schema on primary per D-43.
    // WithDropColumn: enables ALTER TABLE DROP COLUMN for v1.15 Phase 63 schema
    // cleanup (ixpfx.notes, organization.fac_count, organization.net_count) and
    // any future hygiene drops. Per D-04.
    // WithDropIndex: symmetric handling of stale indexes per ent docs recommendation.
    if isPrimary {
        if err := entClient.Schema.Create(
            ctx,
            migrate.WithDropColumn(true),
            migrate.WithDropIndex(true),
        ); err != nil {
            logger.Error("failed to migrate schema", slog.Any("error", err))
            os.Exit(1)
        }
    }
</interfaces>
</context>

<tasks>

<task type="auto" tdd="false">
  <name>Task 1: Drop three fields from ent schema and regenerate</name>
  <read_first>
    - ent/schema/ixprefix.go (current IxPrefix schema — line 34 has field.String("notes"))
    - ent/schema/organization.go (current Organization schema — lines 106-113 have net_count and fac_count)
    - CLAUDE.md Code Generation section (confirms go generate ./... pipeline order; do NOT run go generate ./schema)
    - .planning/phases/63-schema-hygiene/63-RESEARCH.md "Codegen pipeline impact" section (authoritative list of auto-regenerated surfaces)
  </read_first>
  <files>
    - ent/schema/ixprefix.go
    - ent/schema/organization.go
    - ent/ (all generated)
    - gen/peeringdb/v1/ (proto regen)
    - graph/ (GraphQL regen)
    - proto/peeringdb/v1/v1.proto (entproto regen)
  </files>
  <action>
    Edit ent schemas to drop the three confirmed-vestigial fields (per D-01, D-02):

    1. ent/schema/ixprefix.go — remove lines 34-37 (the entire field.String("notes").Optional().Default("").Comment("Notes"), block). Leave all other fields untouched.

    2. ent/schema/organization.go — remove lines 105-113 (the `// Computed fields (from serializer, stored per D-40)` comment block AND both field.Int("net_count")... and field.Int("fac_count")... blocks). Leave all other fields untouched.

    3. Run the full codegen pipeline from repo root:

        TMPDIR=/tmp/claude-1000 go generate ./...

       This regenerates (per RESEARCH.md "Codegen pipeline impact"):
       - ent/*.go, ent/organization/*.go, ent/ixprefix/*.go — setter/predicate/field-constant removal
       - ent/migrate/schema.go — Columns slice updates
       - gen/peeringdb/v1/v1.pb.go + proto/peeringdb/v1/v1.proto — entproto field removal (remaining field numbers SHIFT; this is accepted per RESEARCH.md Risks item 4 — read-only mirror has no external proto consumers)
       - graph/schema.graphqls, graph/schema.graphql, graph/generated.go — GraphQL schema + resolvers
       - entrest handlers embedded in ent/ — REST filter predicate removal

    4. Do NOT run `go generate ./schema` (CLAUDE.md warning: strips entproto annotations).

    5. Expect `go build ./...` to fail at this point because hand-written code still references Notes/FacCount/NetCount via the generated packages. That is resolved in Task 2.

    Decision traceability: D-01 (full ixpfx.notes removal), D-02 (full org.fac_count/net_count removal). Per D-05, no annotation or comment in the schema files needs to survive for these fields — they are gone entirely.
  </action>
  <verify>
    <automated>cd /home/dotwaffle/Code/pdb/peeringdb-plus &amp;&amp; ! grep -nE 'field\.String\("notes"\)' ent/schema/ixprefix.go &amp;&amp; ! grep -nE 'field\.Int\("(fac|net)_count"\)' ent/schema/organization.go</automated>
  </verify>
  <acceptance_criteria>
    - grep -nE 'field\.String\("notes"\)' ent/schema/ixprefix.go returns no match (exit 1)
    - grep -nE 'field\.Int\("(fac|net)_count"\)' ent/schema/organization.go returns no matches (exit 1)
    - git status ent/ gen/ graph/ proto/ shows modified files (codegen ran)
    - grep -n 'FieldNotes' ent/ixprefix/ixprefix.go returns no matches (field constant gone)
    - grep -nE 'FieldFacCount|FieldNetCount' ent/organization/organization.go returns no matches
    - Command `TMPDIR=/tmp/claude-1000 go generate ./...` exited 0
  </acceptance_criteria>
  <done>
    Three ent schema fields dropped; go generate ./... ran cleanly; all regenerated files (ent/, gen/, graph/, proto/) are modified on disk and ready to be committed alongside the hand-written edits in Task 2.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Update hand-written consumers and regenerate golden fixtures</name>
  <read_first>
    - internal/peeringdb/types.go (IxPrefix struct — line 232)
    - internal/pdbcompat/serializer.go (ixPrefixFromEnt — lines 286-304)
    - internal/pdbcompat/registry.go (TypeIXPfx block — line 269; CONFIRM surrounding type before editing)
    - internal/pdbcompat/anon_parity_test.go (knownDivergences — lines 55-70)
    - internal/grpcserver/ixprefix.go (filters and proto converter — lines 41, 59, 150)
    - internal/sync/upsert.go (ixpfx upsert chain — line 353; CONFIRM you edit only the IxPrefix chain, not other entities' SetNotes calls at lines 84, 125, 171, 217, 274, 422, 519)
    - internal/pdbcompat/testdata/golden/ixpfx/list.json
    - internal/pdbcompat/testdata/golden/ixpfx/detail.json
    - internal/pdbcompat/testdata/golden/ixpfx/depth.json
    - .planning/phases/63-schema-hygiene/63-RESEARCH.md "Codegen pipeline impact" section (enumerates every hand-written reference)
  </read_first>
  <behavior>
    After this task, the following behaviors are locked by tests:
    - Test: TestAnonParityFixtures/ixpfx — passes with empty knownDivergences (or at least without the ixpfx|data[0].notes|extra_field entry).
    - Test: go build ./... — compiles cleanly (no dangling references to Notes/FieldNotes/ip.Notes/ixp.Notes/r.Notes on IxPrefix types).
    - Test (new, defensive per CONTEXT.md Claude's Discretion): A unit test in internal/pdbcompat/serializer_test.go named TestIxPrefixFromEnt_NoNotesKey that constructs an ent.IxPrefix (using existing test builders or a bare struct if acceptable), calls ixPrefixFromEnt, marshals the result to JSON, unmarshals to map[string]json.RawMessage, and asserts that the key "notes" is absent. Parallel micro-test TestOrganizationJSON_NoCountKeys (may live in the same file) constructs a peeringdb.Organization zero value, marshals it, and asserts "fac_count" and "net_count" are absent from the key set.
    - Golden fixtures for ixpfx (list.json, detail.json, depth.json) contain no "notes": entries.
  </behavior>
  <files>
    - internal/peeringdb/types.go
    - internal/pdbcompat/serializer.go
    - internal/pdbcompat/registry.go
    - internal/pdbcompat/anon_parity_test.go
    - internal/pdbcompat/serializer_test.go (new test or amended existing)
    - internal/grpcserver/ixprefix.go
    - internal/sync/upsert.go
    - internal/pdbcompat/testdata/golden/ixpfx/list.json
    - internal/pdbcompat/testdata/golden/ixpfx/detail.json
    - internal/pdbcompat/testdata/golden/ixpfx/depth.json
  </files>
  <action>
    Hand-edit every non-generated reference to the three dropped fields. Work through them in this order to keep build errors localized:

    1. internal/peeringdb/types.go — delete the single line at 232 (`Notes    string    ` with json tag `notes`) from the IxPrefix struct. Do NOT touch the Notes fields on other types (Organization at line 49, Network at line 90, Facility at 132, InternetExchange at 165, etc. — those are real upstream fields and stay).

    2. internal/pdbcompat/serializer.go — in ixPrefixFromEnt (around line 290), delete the line `Notes:    p.Notes,` at 297. Also update the function's doc comment (lines 287-289) which currently says "but our compat layer includes it (ent serializes all schema fields). Extra fields don't break API consumers — this is a known divergence." Replace that comment with: `// Matches upstream: PeeringDB's live API omits "notes" from ixpfx responses, and as of v1.15 Phase 63 the field is dropped from our ent schema too (per D-01).`

    3. internal/pdbcompat/registry.go — delete the single line `"notes":    FieldString,` inside the TypeIXPfx registry block at line 269. CONFIRM you are editing the ixpfx block (not the other seven "notes" entries at lines 79, 120, 162, 197, 284, 341, 373 — those belong to other entities). Easiest verification: the edited block is the one immediately preceded by a TypeIXPfx constant or comment.

    4. internal/pdbcompat/anon_parity_test.go — per D-03, remove the entire `// ixpfx|data[0].notes|extra_field` comment block (lines 56-68) AND the map entry `"ixpfx|data[0].notes|extra_field": {},` at line 69. The resulting knownDivergences should be a bare empty map: `var knownDivergences = map[string]struct{}{}`.

    5. internal/grpcserver/ixprefix.go — three hunks:
       - Lines 41-42: remove both lines of the eqFilter entry that references r.Notes and ixprefix.FieldNotes for ListIxPrefixesRequest. Delete the trailing comma correctly so the surrounding slice literal stays valid.
       - Lines 59-60: same removal for the StreamIxPrefixesRequest filter.
       - Line 150: remove `Notes:    stringVal(ixp.Notes),` from the proto conversion struct literal.

    6. internal/sync/upsert.go — line 353: remove the single `SetNotes(ip.Notes).` line from the IxPrefix upsert builder chain. CONFIRM the line above/below is on an IxPrefix builder chain (the enclosing function is the ixpfx upsert path). DO NOT touch the other SetNotes lines at 84, 125, 171, 217, 274, 422, 519 — those are real fields on other entities.

    7. Golden fixtures — all three internal/pdbcompat/testdata/golden/ixpfx/{list,detail,depth}.json contain "notes": keys inside their data array objects. Either:
       - (preferred) Run the existing golden-regeneration path if the test supports -update — check internal/pdbcompat/ for an -update flag or similar. If present: `TMPDIR=/tmp/claude-1000 go test ./internal/pdbcompat/... -run TestGolden -update -race` (or whatever name the golden test uses).
       - Otherwise hand-edit: in each of the three files, delete every `"notes": "",` line (they will be inside per-row objects). Verify the resulting JSON is valid (`jq . < list.json > /dev/null`).

    8. Defensive test (per CONTEXT.md Claude's Discretion — recommended): Add to internal/pdbcompat/serializer_test.go (create file if absent) two small tests:

        func TestIxPrefixFromEnt_NoNotesKey(t *testing.T) {
            // Construct a minimal ent.IxPrefix (use existing test helpers or bare struct).
            // Call ixPrefixFromEnt, json.Marshal, assert "notes" is not a key.
        }

        func TestOrganizationJSON_NoCountKeys(t *testing.T) {
            o := peeringdb.Organization{}
            b, err := json.Marshal(o)
            // Assert "fac_count" and "net_count" are absent from the resulting keys.
        }

       Keep each test ≤ ~15 LoC. Use json.Unmarshal into map[string]json.RawMessage to check key presence. If ent.IxPrefix cannot be trivially constructed without a full ent client, marshal peeringdb.IxPrefix{} instead and assert absence of "notes" on the wire struct — that covers the serializer indirectly since the emission path funnels through the peeringdb type.

    9. Run the build and tests:

        TMPDIR=/tmp/claude-1000 go build ./...
        TMPDIR=/tmp/claude-1000 go test ./internal/pdbcompat/... -run TestAnonParityFixtures -race
        TMPDIR=/tmp/claude-1000 go test ./internal/pdbcompat/... -run 'TestIxPrefixFromEnt_NoNotesKey|TestOrganizationJSON_NoCountKeys' -race

    Decision traceability: D-01, D-02, D-03 drive the struct/serializer/registry/test edits. D-05 is partly satisfied here by the serializer doc comment rewrite; remaining doc sweep is Task 4.

    Pitfall reminders (from RESEARCH.md "Common Pitfalls"):
    - Pitfall 3: grep `grep -rn "ixp\.Notes\|ip\.Notes\|FieldNotes" internal/ | grep -vi test | grep -vi fixture` before declaring done — zero hits confirms no strays.
    - Pitfall 4: Goldens must be updated or TestAnonParityFixtures / golden tests will fail.
  </action>
  <verify>
    <automated>cd /home/dotwaffle/Code/pdb/peeringdb-plus &amp;&amp; TMPDIR=/tmp/claude-1000 go build ./... &amp;&amp; TMPDIR=/tmp/claude-1000 go test ./internal/pdbcompat/... -run 'TestAnonParityFixtures|TestIxPrefixFromEnt_NoNotesKey|TestOrganizationJSON_NoCountKeys' -race</automated>
  </verify>
  <acceptance_criteria>
    - `TMPDIR=/tmp/claude-1000 go build ./...` exits 0 (no dangling references)
    - Search for ixp.Notes, ip.Notes, FieldNotes across internal/ (excluding testdata) returns zero lines: `grep -rn "ip\.Notes\|ixp\.Notes\|FieldNotes" internal/ | grep -v testdata` yields nothing
    - `grep -n 'ixpfx|data\[0\]\.notes' internal/pdbcompat/anon_parity_test.go` returns no match
    - `grep -c '"notes"' internal/pdbcompat/testdata/golden/ixpfx/list.json` returns 0 (and same for detail.json and depth.json)
    - `TMPDIR=/tmp/claude-1000 go test ./internal/pdbcompat/... -run TestAnonParityFixtures -race` exits 0
    - The two defensive micro-tests (TestIxPrefixFromEnt_NoNotesKey, TestOrganizationJSON_NoCountKeys) exist and pass
    - `TMPDIR=/tmp/claude-1000 go vet ./...` exits 0
  </acceptance_criteria>
  <done>
    All hand-written consumers of the three dropped fields are cleaned up; golden fixtures regenerated; defensive tests landed; go build ./... and the targeted pdbcompat tests pass. The repo compiles cleanly end-to-end.
  </done>
</task>

<task type="auto" tdd="false">
  <name>Task 3: Wire drop-column migrate options at runtime call site</name>
  <read_first>
    - cmd/peeringdb-plus/main.go (specifically the Schema.Create(ctx) call at line 114 and the surrounding `if isPrimary` block)
    - ent/migrate/migrate.go (confirm WithDropColumn and WithDropIndex are re-exported; they are — RESEARCH.md "Runtime migration site" verifies)
    - .planning/phases/63-schema-hygiene/63-RESEARCH.md "Runtime migration site" section (authoritative diff for this edit)
    - CLAUDE.md LiteFS section (confirms DDL runs on primary only; the existing `if isPrimary` gate is correct)
  </read_first>
  <files>
    - cmd/peeringdb-plus/main.go
  </files>
  <action>
    Wire the two migrate options at the single Schema.Create call site in cmd/peeringdb-plus/main.go (line ~114). This is a runtime change, NOT a codegen change — do NOT touch ent/entc.go.

    1. Add the import (keep the import block in alphabetical order, so this goes with the other `github.com/dotwaffle/peeringdb-plus/ent/*` imports):

        "github.com/dotwaffle/peeringdb-plus/ent/migrate"

    2. Replace the current block (lines 112-118) with the new block that passes WithDropColumn(true) and WithDropIndex(true) as variadic args. See <interfaces> above for the exact target-state code snippet.

    3. Verify the build still compiles and `go vet ./...` is clean.

    Decision traceability: D-04 (rely on ent auto-migrate; flag needed for DROP COLUMN per entgo docs — verified in RESEARCH.md).

    Rationale for landing the flag permanently (not temporary for this phase only): future schema drops will use the same mechanism. Keeping the flag on is the v1.0 pattern + drop-column capability; rollback remains clean because dropped data was always empty anyway (RESEARCH.md Rollback plan item b).

    Pitfall reminder (RESEARCH.md Common Pitfalls 1): forgetting the import produces `undefined: migrate` compile error. The import line is mandatory.
  </action>
  <verify>
    <automated>cd /home/dotwaffle/Code/pdb/peeringdb-plus &amp;&amp; TMPDIR=/tmp/claude-1000 go build ./cmd/peeringdb-plus/... &amp;&amp; grep -q 'migrate.WithDropColumn(true)' cmd/peeringdb-plus/main.go &amp;&amp; grep -q 'migrate.WithDropIndex(true)' cmd/peeringdb-plus/main.go</automated>
  </verify>
  <acceptance_criteria>
    - `TMPDIR=/tmp/claude-1000 go build ./cmd/peeringdb-plus/...` exits 0
    - `grep -c 'migrate.WithDropColumn(true)' cmd/peeringdb-plus/main.go` returns at least 1
    - `grep -c 'migrate.WithDropIndex(true)' cmd/peeringdb-plus/main.go` returns at least 1
    - `grep -c '"github.com/dotwaffle/peeringdb-plus/ent/migrate"' cmd/peeringdb-plus/main.go` returns at least 1 (import landed)
    - `TMPDIR=/tmp/claude-1000 go vet ./cmd/peeringdb-plus/...` exits 0
    - The existing `if isPrimary { ... }` gate is preserved (not removed or inverted) — `grep -c 'if isPrimary' cmd/peeringdb-plus/main.go` returns at least 1 (was already true before)
  </acceptance_criteria>
  <done>
    Runtime call site now passes WithDropColumn(true) and WithDropIndex(true); ent auto-migrate on primary startup will emit ALTER TABLE DROP COLUMN for the three dropped fields (and any future drops). Build clean; vet clean.
  </done>
</task>

<task type="auto" tdd="false">
  <name>Task 4: Doc sweep per D-05 (PROJECT.md, ARCHITECTURE.md, CLAUDE.md, API.md)</name>
  <read_first>
    - .planning/PROJECT.md (Key Decisions table — find the most recent row to match formatting)
    - docs/ARCHITECTURE.md (scan for any enumeration of schema fields that might reference ixpfx.notes, org.fac_count, or org.net_count)
    - CLAUDE.md (Schema & Visibility section — update ent schema notes to reflect drops and call out the new migrate flags)
    - docs/API.md (scan for any per-endpoint response shape reference to /api/ixpfx or /api/org that enumerates fields)
    - .planning/phases/63-schema-hygiene/63-CONTEXT.md D-05 (full doc sweep mandate)
  </read_first>
  <files>
    - .planning/PROJECT.md
    - docs/ARCHITECTURE.md
    - CLAUDE.md
    - docs/API.md
  </files>
  <action>
    Execute the full doc sweep mandated by D-05. Touch each of the four surfaces, but only where the dropped fields are actually referenced — do not fabricate mentions just to have an edit.

    1. .planning/PROJECT.md — add a new row to the Key Decisions table for v1.15 Phase 63. Format must match the existing rows (check the last row for column order). Suggested content:

        | D-XX | v1.15 | Schema hygiene: drop ixpfx.notes + org.{fac,net}_count | Audit-confirmed vestigial; ixpfx matches upstream shape after drop. Runtime wiring: migrate.WithDropColumn(true) + migrate.WithDropIndex(true) at cmd/peeringdb-plus/main.go. Accepted cosmetic wire-compat break: entproto renumbers remaining IxPrefix / Organization proto fields. Read-only mirror — no external proto consumers. |

       Renumber D-XX to match the next available decision ID based on the table's current max.

    2. docs/ARCHITECTURE.md — grep for "notes", "fac_count", "net_count" inside the file. If any explicit schema enumeration references them (e.g. a table of IxPrefix fields, or a bullet list of Organization computed fields), remove the references. If no mention exists, document the search in the task SUMMARY and move on — the grep result IS the evidence.

    3. CLAUDE.md — update the Schema & Visibility section (or the nearest ent/schema section). Add a note capturing:
       - The three fields dropped in v1.15 Phase 63.
       - The migrate.WithDropColumn(true) + WithDropIndex(true) flags are now on permanently in cmd/peeringdb-plus/main.go for future schema hygiene drops.
       - Future hygiene drops follow the same pattern: edit ent/schema/*.go, run go generate ./..., remove hand-written references, regenerate goldens, deploy.

    4. docs/API.md — grep for `/api/ixpfx`, `/api/org`, and for the literal field names. If the file documents pdbcompat response shapes field-by-field, remove `notes` from the ixpfx shape. If org's fac_count/net_count are listed, remove them. If no such enumeration exists, no edit; note the grep result in the SUMMARY.

    5. Verify the sweep:

        grep -rn "ixpfx.notes\|fac_count\|net_count\|IxPrefix\.Notes" docs/ CLAUDE.md .planning/PROJECT.md

       Each hit should now either (a) be in a historical decision row explicitly referring to the Phase 63 drop, or (b) belong to a different context (e.g. net_count as an upstream field on a different type). Review each hit and confirm it's intentional.

    Decision traceability: D-05 full doc sweep (all four surfaces touched).
  </action>
  <verify>
    <automated>cd /home/dotwaffle/Code/pdb/peeringdb-plus &amp;&amp; grep -l "Phase 63" .planning/PROJECT.md CLAUDE.md &amp;&amp; grep -l "WithDropColumn" CLAUDE.md</automated>
  </verify>
  <acceptance_criteria>
    - .planning/PROJECT.md contains a Key Decisions row mentioning "Phase 63" and "ixpfx.notes" (grep succeeds)
    - CLAUDE.md contains a mention of "WithDropColumn" (the new runtime flag) in the ent/schema notes section
    - docs/ARCHITECTURE.md: no remaining references to the three dropped fields as active schema members (historical notes OK); verified by `grep -nE 'ixpfx\.notes|ix_prefixes\.notes|organizations\.(fac|net)_count' docs/ARCHITECTURE.md` returning no matches in any "current schema" section
    - docs/API.md: no remaining references to the three dropped fields as active response keys; verified by `grep -nE 'ixpfx.*notes|fac_count|net_count' docs/API.md` reviewed line-by-line (either no matches, or matches are in historical/changelog context only)
  </acceptance_criteria>
  <done>
    All four doc surfaces mandated by D-05 have been swept. PROJECT.md has a new Key Decisions row. CLAUDE.md ent notes updated. ARCHITECTURE.md and API.md verified clean (either edited or grep-confirmed no edits needed, with the grep evidence captured in SUMMARY.md).
  </done>
</task>

<task type="auto" tdd="false">
  <name>Task 5: Phase gate — run full test suite, lint, drift check</name>
  <read_first>
    - CLAUDE.md CI section (confirms generated-code drift check covers ent/, gen/, graph/, internal/web/templates/)
    - .planning/phases/63-schema-hygiene/63-RESEARCH.md "Pre-deploy checklist for the planner" section (authoritative gate list)
  </read_first>
  <files>
    - (none — this is a verification-only task)
  </files>
  <action>
    Run the full phase-gate validation suite. All four commands must pass clean before the phase is considered done. This is the same gate CI runs on the resulting PR; running locally first catches drift before push.

    1. Generated-code drift check — regenerate and diff:

        TMPDIR=/tmp/claude-1000 go generate ./...
        git diff --exit-code ent/ gen/ graph/ proto/ internal/web/templates/

       Exit 0 means codegen is stable and all generated files committed in Tasks 1-4 are consistent. Non-zero means you forgot to commit something; stage and amend.

    2. Full test suite with race detector:

        TMPDIR=/tmp/claude-1000 go test -race ./...

       This runs TestAnonParityFixtures, the two new defensive tests, the golden tests, and every other test in the repo. Expect 100% pass.

    3. Linter:

        golangci-lint run

       Expect clean. If nolintlint / unused / revive flag anything related to the drops (unused imports, unreferenced vars), those are legitimate cleanup. Do not suppress with nolint directives.

    4. Vulnerability check (per CLAUDE.md — part of CI):

        TMPDIR=/tmp/claude-1000 govulncheck ./...

       Expect clean — this phase doesn't change dependencies.

    If any of these fail, fix the underlying issue and re-run. Do NOT paper over failures with test skips or nolint.

    Leave a brief verification log in the task output noting the commands and their exit codes; this feeds into the SUMMARY.md.
  </action>
  <verify>
    <automated>cd /home/dotwaffle/Code/pdb/peeringdb-plus &amp;&amp; TMPDIR=/tmp/claude-1000 go generate ./... &amp;&amp; git diff --exit-code ent/ gen/ graph/ proto/ internal/web/templates/ &amp;&amp; TMPDIR=/tmp/claude-1000 go test -race ./... &amp;&amp; golangci-lint run</automated>
  </verify>
  <acceptance_criteria>
    - `TMPDIR=/tmp/claude-1000 go generate ./...` exits 0 and `git diff --exit-code ent/ gen/ graph/ proto/ internal/web/templates/` exits 0 (no drift)
    - `TMPDIR=/tmp/claude-1000 go test -race ./...` exits 0 with zero failing tests
    - `golangci-lint run` exits 0 with no findings
    - `TMPDIR=/tmp/claude-1000 govulncheck ./...` exits 0
    - Verification log captured in the task output (or SUMMARY.md) with the four commands and "exit 0" for each
  </acceptance_criteria>
  <done>
    Phase gate passes cleanly: drift-free codegen, all tests green with race detector, lint clean, no vulnerabilities. Repo is ready for commit and deploy. Post-deploy manual check of `sqlite3 /litefs/peeringdb-plus.db '.schema ix_prefixes organizations'` on the LHR primary remains as a post-merge step (tooling was shipped via quick task 260418-1cn).
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Primary DB write (Schema.Create DDL) | Only the LHR primary process (gated by `isPrimary`) executes DDL. LiteFS replicates WAL byte-for-byte to replicas. No external input crosses this boundary — the schema is compiled into the binary. |
| pdbcompat response serialization | Public HTTP surface. The dropped `notes` field on `/api/ixpfx` is a read-only transformation — removing emission can only shrink the response, never expose new data. |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-63-01 | Tampering | `Schema.Create` with `WithDropColumn(true)` could drop unintended columns if a schema edit accidentally removes a live field | mitigate | Task 5 runs `go generate ./...` drift check + full test suite; `TestAnonParityFixtures` catches shape regressions; code review on the schema diff required before merge. Rollback documented in RESEARCH.md §"Rollback plan" — re-adding the column is schema-additive. |
| T-63-02 | Information Disclosure | Dropping `ixpfx.notes` could accidentally expose a previously-hidden field if the drop were misapplied to a gated entity | accept | The three target fields are confirmed public-or-empty per CONTEXT.md audit. `ixpfx.notes` was always empty from upstream; `org.{fac,net}_count` were never written by sync. No information-disclosure surface is created. |
| T-63-03 | Denial of Service | SQLite DROP COLUMN could trigger a full-table rewrite that exceeds the drain timeout | accept | Target tables are ~45K/80K rows on an ~88 MB DB — RESEARCH.md confirms sub-second DDL. PDBPLUS_DRAIN_TIMEOUT=10s absorbs comfortably. Fly health check rolls back the deploy if DDL ever fails. |
| T-63-04 | Repudiation | Proto field renumbering breaks wire-compat for any external gRPC client that pinned field numbers | accept | Read-only mirror; no known external proto consumers per CONTEXT.md. Called out in SUMMARY.md and PROJECT.md Key Decisions row for transparency (D-05 doc sweep). |
</threat_model>

<verification>
Phase gate (Task 5) runs:
- `TMPDIR=/tmp/claude-1000 go generate ./...` + `git diff --exit-code ent/ gen/ graph/ proto/ internal/web/templates/`
- `TMPDIR=/tmp/claude-1000 go test -race ./...`
- `golangci-lint run`
- `TMPDIR=/tmp/claude-1000 govulncheck ./...`

All four must exit 0.

Post-deploy manual check (executor queues this for after merge, NOT part of the phase gate):
- `fly ssh console -a peeringdb-plus --command "sqlite3 /litefs/peeringdb-plus.db '.schema ix_prefixes organizations'"` on LHR primary confirms the three columns are gone.
- Same command on one replica confirms LiteFS WAL replication propagated the schema change.

(sqlite3 is already in the prod image per quick task 260418-1cn.)
</verification>

<success_criteria>
- `ent/schema/ixprefix.go` and `ent/schema/organization.go` no longer declare the three dropped fields.
- `go generate ./...` produces zero drift.
- `internal/peeringdb/types.go` IxPrefix struct no longer has `Notes`.
- `internal/pdbcompat/serializer.go` `ixPrefixFromEnt` no longer emits `Notes`.
- `internal/pdbcompat/registry.go` TypeIXPfx block no longer contains `"notes"`.
- `internal/pdbcompat/anon_parity_test.go` `knownDivergences` is empty (no `ixpfx|data[0].notes|extra_field` entry).
- `internal/grpcserver/ixprefix.go` compiles — three Notes references removed.
- `internal/sync/upsert.go:353` no longer calls `SetNotes(ip.Notes)` on the IxPrefix chain.
- `cmd/peeringdb-plus/main.go` `Schema.Create` now passes `migrate.WithDropColumn(true)` + `migrate.WithDropIndex(true)`.
- Golden fixtures `internal/pdbcompat/testdata/golden/ixpfx/*.json` contain no `notes` keys.
- Two defensive micro-tests exist and pass.
- PROJECT.md, CLAUDE.md updated per D-05; ARCHITECTURE.md and API.md verified (edited or grep-clean).
- Full phase gate (Task 5) passes: drift-free, tests pass with race, lint clean, govulncheck clean.
- SUMMARY.md captures the proto field renumbering as accepted cosmetic wire-compat break per RESEARCH.md §"Risks" item 4.
</success_criteria>

<output>
After completion, create `.planning/phases/63-schema-hygiene/63-01-SUMMARY.md` using the template at $HOME/.claude/get-shit-done/templates/summary.md. Summary MUST include:
- Commit SHA range for the phase
- The proto field renumbering called out as accepted wire-compat break
- The four phase-gate command exit codes (from Task 5)
- The grep evidence for ARCHITECTURE.md / API.md doc sweep (what was searched, whether any edits were required)
- Post-deploy verification checklist (manual `sqlite3` check on primary + one replica)
</output>
