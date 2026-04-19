// Command pdb-fixture-port ports Django fixture blocks from
// peeringdb/peeringdb's src/peeringdb_server/management/commands/
// pdb_api_test.py into Go struct literals committed to
// internal/testutil/parity/fixtures.go.
//
// Phase 72 D-02 / D-03 rationale: Phase 72's parity regression tests
// consume the same fixture shapes upstream uses, not re-derived
// approximations. Porting the fixtures verbatim at a pinned upstream
// SHA gives behavioural-exact parity on the input side of the
// pdbcompat request pipeline (ordering, status×since, limit, __in,
// traversal). See .planning/phases/72-upstream-parity-regression/
// CONTEXT.md decisions D-02 and D-03 for the selection rationale over
// reusing internal/testutil/seed.Full.
//
// The tool is designed to run offline at milestone boundaries (not
// per-PR). Regeneration:
//
//	go generate ./internal/testutil/parity
//
// Upstream-drift detection:
//
//	go run ./cmd/pdb-fixture-port --check --pinned <sha>
//
// --check is advisory only per D-03: it exits non-zero on drift but
// does not gate PR merges. A scheduled CI job (quarterly) invokes it
// so maintainers are alerted when upstream shifts and a refresh PR is
// warranted.
//
// Usage:
//
//	go run ./cmd/pdb-fixture-port [flags]
//
// Flags:
//
//	--upstream-file   local path to pdb_api_test.py (overrides --upstream-ref)
//	--upstream-ref    git ref to fetch via `gh api` (default "master")
//	--out             output file path (default "internal/testutil/parity/fixtures.go")
//	--category        fixture category ("ordering" for Plan 72-01)
//	--check           advisory drift-check mode; does not write
//	--pinned          expected upstream SHA (hex sha256 of the Python file)
//	--date            ported-on date stamp (default today, UTC); override for
//	                  deterministic test output
//
// The emitted fixtures.go header records the upstream git commit SHA,
// the sha256 of the ported source file, the source path, and the
// porting date. Per T-72-01-01 (threat register), the sha256 lets
// --check detect upstream tampering or drift.
package main
