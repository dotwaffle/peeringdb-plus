// Package parity holds regression tests that lock v1.16 pdbcompat
// semantics against future drift.
//
// Each *_test.go file in this directory covers one behavioural
// category, keyed to upstream peeringdb/peeringdb pdb_api_test.py
// citations (or documented `phaseXX-synthesised` markers per
// .planning/phases/72-upstream-parity-regression/CONTEXT.md D-05).
// Fixtures come from internal/testutil/parity/fixtures.go (ported via
// cmd/pdb-fixture-port per Phase 72 plans 72-01..03).
//
// Per Phase 72 D-06, parity tests run via the standard CI tier
// (`go test -race ./...`) — no separate workflow. Any non-test file
// added to this package is a bug; parity is test-only scope.
package parity
