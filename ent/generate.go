package ent

//go:generate go run -mod=mod entc.go
//go:generate sh -c "cd .. && go run ./cmd/pdb-compat-allowlist"
//go:generate sh -c "cd .. && go tool buf generate"
