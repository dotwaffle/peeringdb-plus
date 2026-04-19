# Phase 67 — Deferred Items

Out-of-scope discoveries made during Phase 67 plan execution. Not fixed in-place
per the GSD scope-boundary rule: each plan fixes only issues DIRECTLY caused by
its own task's changes.

## D-67-01 — `cmd/pdb-schema-generate/TestGenerateIndexes` fails since Plan 67-01

**Discovered during:** Plan 67-02 Task 3 (full `go test -race ./...` verification).

**Symptom:**
```
--- FAIL: TestGenerateIndexes (0.00s)
    main_test.go:447: unexpected index "updated"
FAIL	github.com/dotwaffle/peeringdb-plus/cmd/pdb-schema-generate	0.012s
```

**Root cause:** Plan 67-01 Task 1 Part B appended `indexes = append(indexes, "updated")`
in `cmd/pdb-schema-generate/main.go` `generateIndexes()` (see 67-01-SUMMARY.md
§ Task 1 line 575 region). The existing `TestGenerateIndexes` in `main_test.go`
has an allow-list-style assertion that now rejects the new `"updated"` index as
unexpected. Plan 67-01's verification ran `go test -race ./ent/...` — it did not
run `./cmd/pdb-schema-generate/...`, so the regression was not caught.

**Scope:** Test-level only. The shipped behaviour (schemas correctly carry
`index.Fields("updated")`) is correct and matches Plan 67-01's acceptance
criteria. This is purely the test's allow-list needing to admit the new index.

**Blast radius:** `cmd/pdb-schema-generate` test only. Does not affect runtime,
other packages' tests, or CI build — but CI will report this red on `go test ./...`.

**Reproduction:**
```bash
git stash  # to reset to pre-67-02 state
go test -race -run TestGenerateIndexes ./cmd/pdb-schema-generate/  # still fails
git stash pop
```

**Fix path:** Edit `cmd/pdb-schema-generate/main_test.go` around line 447 to
include `"updated"` in the expected-indexes allow-list (alongside the existing
`"status"` entry). 1-line addition + commit message
`fix(67-01): admit "updated" in TestGenerateIndexes allow-list`.

**Deferred to:** Follow-up quick task OR bundled into Plan 67-03 if it also
touches `cmd/pdb-schema-generate/`. Not a Plan 67-02 concern.
