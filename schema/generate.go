// Package schema contains the intermediate PeeringDB JSON schema and
// go:generate directives for the schema extraction pipeline.
//
// The full pipeline:
//  1. pdb-schema-extract parses PeeringDB Django Python source -> peeringdb.json
//  2. pdb-schema-generate reads peeringdb.json -> ent/schema/*.go
//  3. go generate ./ent runs entgo code generation
//
// NOTE: The extraction step requires a local clone of the PeeringDB repository.
// Set PEERINGDB_REPO_PATH to the path of your local peeringdb/peeringdb checkout.
// If not set, the extraction step is skipped and the existing peeringdb.json is used.
//
//go:generate go run ../cmd/pdb-schema-generate/main.go peeringdb.json ../ent/schema
package schema
