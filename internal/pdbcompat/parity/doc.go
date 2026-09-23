// Package parity holds regression tests that lock pdbcompat behavior to
// upstream PeeringDB.
//
// Each test cites its upstream source: a test in pdb_api_test.py or
// tests/test_meta_registry.py, or a line of upstream code such as rest.py,
// serializers.py, or models.py. A test with no upstream counterpart carries
// a `synthesised` marker. A DIVERGENCE_ sub-test locks an intentional
// difference that docs/API.md § Known Divergences records.
//
// Each test seeds its own rows through the ent client and states the
// expected response by hand. The package holds test files only. CI runs it
// with the rest of the suite under the race detector (`go test -race ./...`).
package parity
