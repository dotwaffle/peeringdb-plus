package ent

// pdb-schema-generate regenerates ent/schema/{type}.go from peeringdb.json
// and MUST run before entc, which consumes those schemas. It is sequenced
// here as the first ent generate step (rather than under schema/, which
// `go generate ./...` would visit only after ent/) so the whole pipeline
// converges in a single pass.
//go:generate sh -c "cd ../schema && go run ../cmd/pdb-schema-generate/main.go peeringdb.json ../ent/schema"
//go:generate go run -mod=mod entc.go
//go:generate sh -c "cd .. && go run ./cmd/pdb-compat-allowlist"
//go:generate sh -c "cd .. && buf generate"
