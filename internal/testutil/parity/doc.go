// Package parity holds upstream-ported fixture data for the Phase 72
// parity regression test suite.
//
// Fixtures are ported verbatim from peeringdb/peeringdb's
// src/peeringdb_server/management/commands/pdb_api_test.py via
// cmd/pdb-fixture-port. The upstream commit SHA and the sha256 of
// the ported file live in fixtures.go's header; cmd/pdb-fixture-port
// --check recomputes the hash against a live upstream fetch to detect
// drift (advisory only per Phase 72 D-03 — no PR merge gate).
//
// Regeneration:
//
//	go generate ./internal/testutil/parity
//
// Per Phase 72 D-02, this package is DELIBERATELY ISOLATED from
// internal/testutil/seed. Parity tests must not cross-contaminate
// with the seed.Full fixture set — a parity failure should be
// attributable to a behavioural regression against upstream, not to
// an incidental seed.Full row the consumer accidentally picked up.
//
// Consumer pattern (Plans 72-02 through 72-06):
//
//	client := testutil.SetupClient(t)
//	for _, fx := range parity.OrderingFixtures {
//	    // create ent rows from fx.Entity + fx.Fields …
//	}
package parity
