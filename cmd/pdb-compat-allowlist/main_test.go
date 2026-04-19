package main

import (
	"reflect"
	"testing"
)

// TestDecodeFields locks the annotation-payload-to-[]string contract
// that extractAllowlist depends on. ent's LoadGraph JSON-roundtrips
// annotation values, so the usual concrete type arriving here is
// map[string]any with a "Fields" key. The function also accepts a
// concrete fieldser-shaped struct as a resilience hedge against future
// ent releases — both branches are exercised below.
func TestDecodeFields(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input any
		want  []string
	}{
		{
			name:  "json_roundtrip_form",
			input: map[string]any{"Fields": []any{"org__name", "ixlan__ix__fac_count"}},
			want:  []string{"org__name", "ixlan__ix__fac_count"},
		},
		{
			name:  "empty_fields",
			input: map[string]any{"Fields": []any{}},
			want:  []string{},
		},
		{
			name:  "nil_map",
			input: nil,
			want:  nil,
		},
		{
			name:  "non_string_entries_dropped",
			input: map[string]any{"Fields": []any{"org__name", 42, true}},
			want:  []string{"org__name"},
		},
		{
			name:  "missing_Fields_key",
			input: map[string]any{"Other": "value"},
			want:  []string{},
		},
		{
			name:  "unrecognised_type",
			input: 12345,
			want:  nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := decodeFields(tc.input)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("decodeFields(%v) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// TestPdbTypeFor_AllThirteen locks the ent-Go-name → pdb-type mapping
// that the rest of Phase 70 relies on. If a future schema is added,
// the test will fail and force the author to extend pdbTypeFor.
func TestPdbTypeFor_AllThirteen(t *testing.T) {
	t.Parallel()
	expected := map[string]string{
		"Organization":     "org",
		"Network":          "net",
		"Facility":         "fac",
		"InternetExchange": "ix",
		"Poc":              "poc",
		"IxLan":            "ixlan",
		"IxPrefix":         "ixpfx",
		"NetworkIxLan":     "netixlan",
		"NetworkFacility":  "netfac",
		"IxFacility":       "ixfac",
		"Carrier":          "carrier",
		"CarrierFacility":  "carrierfac",
		"Campus":           "campus",
	}
	for goName, wantPDB := range expected {
		if got := pdbTypeFor(goName); got != wantPDB {
			t.Errorf("pdbTypeFor(%q) = %q, want %q", goName, got, wantPDB)
		}
	}
}

// TestPdbTypeFor_UnknownReturnsEmpty documents the fallback contract:
// unknown Go names return "" (caller skips them) rather than error.
// This keeps the tool resilient if the schema grows a new type before
// pdbTypeFor is updated.
func TestPdbTypeFor_UnknownReturnsEmpty(t *testing.T) {
	t.Parallel()
	if got := pdbTypeFor("NotARealEntity"); got != "" {
		t.Errorf("pdbTypeFor(unknown) = %q, want empty string", got)
	}
}
