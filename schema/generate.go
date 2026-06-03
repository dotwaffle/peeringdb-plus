// Package schema contains the intermediate PeeringDB JSON schema and
// generate directives for the schema extraction pipeline.
//
// The full pipeline:
//  1. pdb-schema-extract parses PeeringDB Django Python source -> peeringdb.json
//  2. pdb-schema-generate reads peeringdb.json -> ent/schema/*.go
//  3. entc runs entgo code generation from those schemas
//
// Steps 2 and 3 are both driven from ent/generate.go (in that order) so a
// single `go generate ./...` converges without a second pass: the schema
// producer must run before entc, its consumer, which `go generate ./...`
// could not guarantee if the directive lived here (ent/ is visited first).
//
// NOTE: The extraction step requires a local clone of the PeeringDB repository.
// Set PEERINGDB_REPO_PATH to the path of your local peeringdb/peeringdb checkout.
// If not set, the extraction step is skipped and the existing peeringdb.json is used.
package schema
