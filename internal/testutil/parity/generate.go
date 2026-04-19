//go:build ignore

// This file carries only the `go generate` directive that regenerates
// fixtures.go. The `ignore` build tag keeps the file out of the
// compiled package; its sole purpose is to localise the directive
// with the package it regenerates (mirror of ent/generate.go).
//
// Run:
//
//	go generate ./internal/testutil/parity
//
// Behaviour:
//   - Fetches peeringdb/peeringdb master via `gh api` (sandbox allow-
//     listed).
//   - Writes the committed fixtures.go in this directory.
//   - Phase 72 D-03: the quarterly drift check is a separate `--check`
//     invocation; this directive only regenerates.

package parity

//
// The directive uses --upstream-ref master to fetch the live upstream
// file at regeneration time. To regenerate against a pinned local
// snapshot (e.g. mirroring an earlier ported SHA), invoke the tool
// directly:
//
//	go run ./cmd/pdb-fixture-port \
//	  --upstream-file /path/to/pdb_api_test.py \
//	  --upstream-commit 99e92c726172ead7d224ce34c344eff0bccb3e63 \
//	  --category all \
//	  --out internal/testutil/parity/fixtures.go \
//	  --date 2026-04-19

//go:generate sh -c "cd ../../.. && go run ./cmd/pdb-fixture-port --category all --out internal/testutil/parity/fixtures.go --date 2026-04-19"
