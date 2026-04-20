package main

import (
	"reflect"
	"testing"
)

// TestBuildAllowlistEntry locks the verbatim-field-list → NodeEntry
// contract. This is the Path A ingestion point since the 260420-esb
// sibling-files refactor moved Path A source-of-truth from ent schema
// annotations into the hand-written schema.PrepareQueryAllows map.
// Fields are split on "__" and routed to Direct / Via (or dropped with
// a warn for 0/1/4+ segment counts per D-04).
func TestBuildAllowlistEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pdbType string
		fields  []string
		want    *NodeEntry
	}{
		{
			name:    "direct_only",
			pdbType: "poc",
			fields:  []string{"net__name", "net__asn"},
			want: &NodeEntry{
				GoName:  "Poc",
				PDBType: "poc",
				Direct:  []string{"net__asn", "net__name"}, // sorted
			},
		},
		{
			name:    "direct_plus_via",
			pdbType: "fac",
			fields:  []string{"org__name", "ixlan__ix__fac_count"},
			want: &NodeEntry{
				GoName:  "Facility",
				PDBType: "fac",
				Direct:  []string{"org__name"},
				Via: []ViaEntry{
					{FirstHop: "ixlan", Tails: []string{"ix__fac_count"}},
				},
			},
		},
		{
			name:    "empty_fields_yields_nil",
			pdbType: "net",
			fields:  nil,
			want:    nil,
		},
		{
			name:    "empty_pdbType_yields_nil",
			pdbType: "",
			fields:  []string{"org__name"},
			want:    nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildAllowlistEntry(tc.pdbType, tc.fields)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("buildAllowlistEntry(%q, %v) =\n  %+v\nwant\n  %+v", tc.pdbType, tc.fields, got, tc.want)
			}
		})
	}
}

// TestBuildAllowlistEntry_DropsInvalidHops verifies the 0/1-segment
// (malformed) and 4+-segment (>D-04 cap) field strings are dropped —
// without affecting the valid entries alongside them. The function
// logs a warn for each but returns the remaining shape intact.
func TestBuildAllowlistEntry_DropsInvalidHops(t *testing.T) {
	t.Parallel()
	got := buildAllowlistEntry("net", []string{
		"org__name",               // valid direct
		"noSeparator",             // 1-segment — dropped
		"a__b__c__d",              // 4-segment — D-04 violation, dropped
		"netfac__fac__name",       // valid via
	})
	want := &NodeEntry{
		GoName:  "Network",
		PDBType: "net",
		Direct:  []string{"org__name"},
		Via: []ViaEntry{
			{FirstHop: "netfac", Tails: []string{"fac__name"}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildAllowlistEntry with invalid hops =\n  %+v\nwant\n  %+v", got, want)
	}
}

// TestPdbTypeFor_AllThirteen locks the ent-Go-name → pdb-type mapping
// that the rest of Phase 70 relies on. If a future schema is added,
// the test will fail and force the author to extend pdbTypeMap.
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
// pdbTypeMap is updated.
func TestPdbTypeFor_UnknownReturnsEmpty(t *testing.T) {
	t.Parallel()
	if got := pdbTypeFor("NotARealEntity"); got != "" {
		t.Errorf("pdbTypeFor(unknown) = %q, want empty string", got)
	}
}

// TestGoNameFor_RoundTrip locks the reverse mapping — every entry in
// pdbTypeMap must round-trip pdbTypeFor ⇌ goNameFor. Guarantees that
// any future extension of the map keeps both directions consistent.
func TestGoNameFor_RoundTrip(t *testing.T) {
	t.Parallel()
	for goName, pdbType := range pdbTypeMap {
		if got := goNameFor(pdbType); got != goName {
			t.Errorf("goNameFor(%q) = %q, want %q (reverse of pdbTypeFor)", pdbType, got, goName)
		}
	}
}

// TestGoNameFor_UnknownReturnsEmpty — parallel of the pdbTypeFor
// fallback contract: unknown inputs yield an empty-string sentinel
// so the caller can skip rather than panic.
func TestGoNameFor_UnknownReturnsEmpty(t *testing.T) {
	t.Parallel()
	if got := goNameFor("not-a-real-pdb-type"); got != "" {
		t.Errorf("goNameFor(unknown) = %q, want empty string", got)
	}
}
