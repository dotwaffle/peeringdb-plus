// Package parity holds regression tests that lock v1.16 pdbcompat
// semantics against future drift.
//
// Each *_test.go file in this directory covers one behavioural
// category, keyed to upstream peeringdb/peeringdb pdb_api_test.py
// citations (or documented `synthesised` markers where no upstream
// test exercises the behaviour).
//
// Each test seeds its own clean rows inline (via the ent client) and
// cites the upstream source line in a comment; the assertions encode
// the expected served response by hand. The earlier ported-fixture
// pipeline (internal/testutil/parity + cmd/pdb-fixture-port) was
// removed once it became clear the ports carried unseedable Python
// source artefacts and were consumed by no behavioural test.
//
// Parity tests run via the standard CI tier (`go test -race ./...`) —
// no separate workflow. Any non-test file added to this package is a
// bug; parity is test-only scope.
package parity
