package main

import "testing"

// TestStringFieldModifiers locks the interlocking NotEmpty / Optional /
// Default("") derivation across the interesting (name, required,
// nullable, references) tuples. The tombstone-scrub cases are
// load-bearing: NotEmpty() on "name"/"role" would reject upstream
// PII-scrubbed tombstones at the upsert builder and abort incremental
// sync (observed live 2026-04-26).
func TestStringFieldModifiers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		desc                             string
		name                             string
		fd                               FieldDef
		notEmpty, optional, defaultEmpty bool
	}{
		{
			desc: "prefix required non-null keeps NotEmpty, no Optional, no default",
			name: "prefix", fd: FieldDef{Type: "string", Required: true},
			notEmpty: true,
		},
		{
			desc: "name required non-null drops NotEmpty (tombstone scrub) but stays required-shaped",
			name: "name", fd: FieldDef{Type: "string", Required: true},
		},
		{
			desc: "role required non-null drops NotEmpty (tombstone scrub)",
			name: "role", fd: FieldDef{Type: "string", Required: true},
		},
		{
			desc: "required non-name field is Optional with empty default",
			name: "notes", fd: FieldDef{Type: "string", Required: true},
			optional: true, defaultEmpty: true,
		},
		{
			desc: "optional non-name field is Optional without default",
			name: "website", fd: FieldDef{Type: "string"},
			optional: true,
		},
		{
			desc: "nullable name field is Optional, never NotEmpty, no default",
			name: "name", fd: FieldDef{Type: "string", Required: true, Nullable: true},
			optional: true,
		},
		{
			desc: "FK-backed name field never gets NotEmpty",
			name: "prefix", fd: FieldDef{Type: "string", Required: true, References: "ixlan"},
			notEmpty: false,
		},
		{
			desc: "nullable non-name field is Optional without empty default",
			name: "logo", fd: FieldDef{Type: "string", Nullable: true},
			optional: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()
			notEmpty, optional, defaultEmpty := stringFieldModifiers(tt.name, tt.fd)
			if notEmpty != tt.notEmpty {
				t.Errorf("notEmpty = %v, want %v", notEmpty, tt.notEmpty)
			}
			if optional != tt.optional {
				t.Errorf("optional = %v, want %v", optional, tt.optional)
			}
			if defaultEmpty != tt.defaultEmpty {
				t.Errorf("defaultEmpty = %v, want %v", defaultEmpty, tt.defaultEmpty)
			}
		})
	}
}
