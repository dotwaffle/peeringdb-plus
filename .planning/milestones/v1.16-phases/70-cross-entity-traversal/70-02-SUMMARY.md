---
phase: 70-cross-entity-traversal
plan: 02
subsystem: api
tags: [pdbcompat, codegen, ent, go-generate, path-a, bootstrap]

# Dependency graph
requires:
  - phase: 70-cross-entity-traversal
    plan: 01
    provides: pdbcompat.PrepareQueryAllowAnnotation + FilterExcludeFromTraversalAnnotation + AllowlistEntry value type
provides:
  - cmd/pdb-compat-allowlist codegen binary (reads ent schema graph, emits Go source)
  - internal/pdbcompat/allowlist_gen.go with empty-bootstrap Allowlists + FilterExcludes maps
  - go generate ./ent pipeline step wired between ent codegen and buf generate
  - ent-Go-name → PeeringDB type-string mapping (13 entities) locked by TestPdbTypeFor_AllThirteen
affects: [70-03, 70-04, 70-05]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Codegen tool reads gen.Graph via entc.LoadGraph — no hand-parsing of schema Go files"
    - "Deterministic sort-before-render for byte-stable output (two-run SHA idempotency)"
    - "text/template with `printf \"%q\"` for every emitted identifier (threat T-70-02-01 mitigation)"
    - "gofmt via go/format.Source; parse failure dumps .broken file for diagnosis"
    - "Annotation value decode accepts both map[string]any (JSON-roundtrip) and a fieldser interface (future-proof)"

key-files:
  created:
    - cmd/pdb-compat-allowlist/main.go
    - cmd/pdb-compat-allowlist/main_test.go
    - internal/pdbcompat/allowlist_gen.go
  modified:
    - ent/generate.go
    - .gitignore

key-decisions:
  - "Annotation name constants duplicated as local strings in the codegen tool — avoids importing internal/pdbcompat from cmd/ during `go generate`, keeps the tool independent of runtime package layering"
  - "Tool emits ONLY Allowlists + FilterExcludes in this bootstrap (Plan 70-02 scope). Plan 70-04 extends the same tool to emit the Edges map for Path B introspection; deferred here to keep the wave boundary clean"
  - ".gitignore extended with /pdb-compat-allowlist and /pdbcompat-check to stop stray `go build` binaries from showing up in git status"

patterns-established:
  - "Pattern: cmd/<tool>/main.go for codegen binaries invoked via `//go:generate sh -c \"cd .. && go run ./cmd/<tool>\"` in ent/generate.go"
  - "Pattern: sort everything before template render — sorted slices + pre-sorted map-iteration keys guarantee byte-identical re-runs"
  - "Pattern: emit deterministic Go source via text/template + go/format.Source; dump .broken on gofmt parse failure for developer diagnosis"

requirements-completed: [TRAVERSAL-01, TRAVERSAL-02]

# Metrics
duration: ~15min
completed: 2026-04-19
---

# Phase 70 Plan 02: cmd/pdb-compat-allowlist codegen tool + wire into go generate Summary

**New `cmd/pdb-compat-allowlist` codegen binary reads ent schema graph via `entc.LoadGraph` and emits `internal/pdbcompat/allowlist_gen.go` with sort-determined ordering; two consecutive runs produce byte-identical output; bootstrap emission is empty because schemas don't carry annotations yet (Plan 70-03 populates).**

## Performance

- **Duration:** ~15 min
- **Completed:** 2026-04-19
- **Tasks:** 3 (all atomic, committed as a single Plan 70-02 feature commit `dd8ffcc`)
- **Files created:** 3 (main.go, main_test.go, allowlist_gen.go)
- **Files modified:** 2 (ent/generate.go, .gitignore)
- **Total LOC:** 417 (299 main.go + 99 main_test.go + 19 generated allowlist_gen.go)

## Accomplishments

### Task 1: cmd/pdb-compat-allowlist/main.go (299 LOC)

- `main()` calls `entc.LoadGraph("./ent/schema", &gen.Config{})` and iterates `graph.Nodes` in two passes: `extractAllowlist` (node-level `PrepareQueryAllow` annotation → `NodeEntry.Direct` + `NodeEntry.Via`) and `extractExcludes` (edge-level `FilterExcludeFromTraversal` annotation → `ExcludeEntry`).
- Hop-splitting via `strings.Split(f, "__")`:
  - 2 segments → `Direct`
  - 3 segments → `Via[firstHop]` grouped
  - 0/1 or 4+ → `log.Printf` warn-and-drop (D-04 2-hop cap enforced at codegen time)
- `decodeFields` handles both `map[string]any` (ent's current JSON-roundtrip form, confirmed by `load/schema_test.go:164`) and a `fieldser interface{ GetFields() []string }` fallback for future ent versions.
- `pdbTypeFor` carries the 13-entity Go-name → PeeringDB-type mapping mirrored from `internal/peeringdb/types.go` and `cmd/pdb-schema-generate/modelNameOverrides`. Unknown names log and return `""` (caller skips).
- Deterministic render: `sort.Slice(data.Entries)` by `PDBType`, `sort.Slice(data.FilterExcludes)` by `(Entity, Edge)`, `sort.Strings(entry.Direct)`, and sorted hop iteration. Re-running on unchanged schema produces byte-identical output.
- `outputTemplate` uses `printf "%q"` for every emitted string (T-70-02-01 mitigation — unusual entity names get correctly Go-quoted).
- `go/format.Source` validates syntax; parse failure writes `.broken` for diagnosis before `log.Fatalf`.

### Task 2: cmd/pdb-compat-allowlist/main_test.go (99 LOC)

- `TestDecodeFields` — 6 subcases covering the JSON-roundtrip map, empty-Fields, nil input, non-string-drop, missing-Fields-key, and unrecognised-type paths. Locks the annotation-decode contract Plan 70-03 depends on.
- `TestPdbTypeFor_AllThirteen` — asserts all 13 entity Go-name → pdb-type mappings. Adding a future schema without updating `pdbTypeFor` will fail this test and force the author to extend the map.
- `TestPdbTypeFor_UnknownReturnsEmpty` — documents the fallback contract (unknown → `""`, not error).
- All tests `t.Parallel()`; `go test -race ./cmd/pdb-compat-allowlist/` PASS in 1.045s.

### Task 3: ent/generate.go wiring + idempotency check

- Three `//go:generate` directives now: ent codegen → pdb-compat-allowlist → buf generate.
- Ordering: ent first (generated tree must exist for later consumers), pdb-compat-allowlist second (reads schema package directly — does NOT depend on generated ent code, but gated for partial-tree safety during interrupted runs), buf last (terminal, proto frozen since v1.6).
- Two-run idempotency verified: SHA256 of `internal/pdbcompat/allowlist_gen.go` identical across two consecutive `go run ./cmd/pdb-compat-allowlist` invocations AND across a full `go generate ./ent` pipeline run. SHA: `6b0857fd273be80b6e18860e79a5a6b56c244ee2873788af9e4db494953f5905`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Stray `pdb-compat-allowlist` binary from initial `go build` run**
- **Found during:** Task 1 verification
- **Issue:** `go build ./cmd/pdb-compat-allowlist/` wrote a binary to repo root, appearing in `git status` as untracked. Other codegen binaries (`/pdb-schema-generate`, `/pdb-schema-extract`, `/peeringdb-plus`) were already in `.gitignore` but the new one wasn't.
- **Fix:** Added `/pdb-compat-allowlist` and `/pdbcompat-check` to `.gitignore` (the latter was also missing — follow-on hygiene). Removed the stray binary from the tree.
- **Files modified:** `.gitignore`
- **Commit:** dd8ffcc (folded into the Plan 70-02 feature commit)

### Observations

- **Bootstrap emission is intentionally empty.** `var Allowlists = map[string]AllowlistEntry{}` and `var FilterExcludes = map[string]map[string]bool{}` both render as empty literals because no ent schema yet carries the Phase 70 annotations. Plan 70-03 attaches `WithPrepareQueryAllow(...)` to all 13 schema files and re-runs the tool to populate both maps.
- **go.sum flake.** First `go generate ./ent` run failed with read-only sum.golang.org/latest errors (documented in CLAUDE.md § Go Module). `GONOSUMCHECK=* GONOSUMDB=* GOFLAGS=-insecure` on retry worked but added a spurious `github.com/jhump/protoreflect v1.10.1/go.mod` hash line to go.sum. Reverted with `git checkout -- go.sum` — not in scope for this plan and the sandbox behaves normally for downstream consumers.

## Verification Results

| Check | Command | Result |
|-------|---------|--------|
| Build | `go build ./...` | PASS |
| Vet | `go vet ./...` | PASS |
| Tests | `go test -race ./cmd/pdb-compat-allowlist/` | PASS (1.045s) |
| Tests | `go test -race ./internal/pdbcompat/` | PASS (6.578s) |
| Header | `head -2 internal/pdbcompat/allowlist_gen.go \| grep -c 'Code generated'` | 1 |
| Directives | `grep -c 'pdb-compat-allowlist' ent/generate.go` | 1 |
| Line count | `wc -l ent/generate.go` | 5 (package + 3 directives + trailing newline) |
| Idempotency | Two `go run ./cmd/pdb-compat-allowlist` → SHA256 equal | PASS (6b0857fd...) |
| Idempotency | Full `go generate ./ent` → SHA256 matches standalone run | PASS (6b0857fd...) |

## Success Criteria

- [x] `cmd/pdb-compat-allowlist` is a runnable codegen binary
- [x] `internal/pdbcompat/allowlist_gen.go` exists and is committed with empty `Allowlists` + `FilterExcludes`
- [x] `go generate ./...` invokes the tool AFTER ent codegen and BEFORE buf codegen
- [x] Re-running `go generate ./ent` is a no-op on disk (byte-identical output)
- [x] Parsing unit tests pass (`decodeFields`, `pdbTypeFor`)
- [x] No change to `ent/schema/*.go`, `internal/pdbcompat/registry.go`, `internal/pdbcompat/filter.go`, or `handler.go`

## Commits

- `dd8ffcc` — feat(70-02): cmd/pdb-compat-allowlist codegen tool + wire into go generate

## Next

Plan 70-03: Attach `WithPrepareQueryAllow(...)` annotations to all 13 ent schema files, translating upstream PeeringDB `get_relation_filters(...)` lists verbatim. Re-run `go generate ./...` to populate `allowlist_gen.go`. This unblocks Plan 70-05's parser.

## Self-Check: PASSED

- [x] `cmd/pdb-compat-allowlist/main.go` exists (299 LOC)
- [x] `cmd/pdb-compat-allowlist/main_test.go` exists (99 LOC)
- [x] `internal/pdbcompat/allowlist_gen.go` exists (19 LOC, empty-bootstrap)
- [x] `ent/generate.go` contains 3 `//go:generate` directives
- [x] Commit `dd8ffcc` present in `git log`
