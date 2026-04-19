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

//go:generate sh -c "cd ../../.. && go run ./cmd/pdb-fixture-port --category ordering --out internal/testutil/parity/fixtures.go --date 2026-04-19"
