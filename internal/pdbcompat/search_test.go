package pdbcompat

import (
	"reflect"
	"testing"
)

// testEntity is a sample struct for testing reflect-based field mapping.
type testEntity struct {
	ID       int      `json:"id"`
	Name     string   `json:"name"`
	Value    float64  `json:"value,omitempty"`
	Internal string   `json:"-"`
	Untagged string   // no json tag
	OrgSet   []string `json:"org_set"`
}

func TestItemToMapReflect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     any
		wantOK    bool
		wantKeys  []string
		absentKey string
	}{
		{
			name:     "struct with json tags",
			input:    testEntity{ID: 1, Name: "Test", Value: 3.14, Internal: "secret", Untagged: "skip"},
			wantOK:   true,
			wantKeys: []string{"id", "name", "value", "org_set"},
		},
		{
			name:      "omitempty tag name parsed correctly",
			input:     testEntity{ID: 2, Name: "OmitTest"},
			wantOK:    true,
			wantKeys:  []string{"id", "name", "value"},
			absentKey: "-",
		},
		{
			name:     "pointer to struct",
			input:    &testEntity{ID: 3, Name: "Ptr"},
			wantOK:   true,
			wantKeys: []string{"id", "name"},
		},
		{
			name:   "map passthrough",
			input:  map[string]any{"id": 1, "name": "Map"},
			wantOK: true,
		},
		{
			name:   "nil pointer",
			input:  (*testEntity)(nil),
			wantOK: false,
		},
		{
			name:   "non-struct non-map",
			input:  42,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m, ok := itemToMap(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("itemToMap() ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			for _, key := range tt.wantKeys {
				if _, exists := m[key]; !exists {
					t.Errorf("missing expected key %q in map", key)
				}
			}
			if tt.absentKey != "" {
				if _, exists := m[tt.absentKey]; exists {
					t.Errorf("unexpected key %q should not be in map", tt.absentKey)
				}
			}
			// json:"-" tagged fields should never appear.
			if _, exists := m["-"]; exists {
				t.Error("json:\"-\" tagged field should not appear in map")
			}
		})
	}
}

func TestItemToMapReflect_FieldValues(t *testing.T) {
	t.Parallel()

	e := testEntity{ID: 42, Name: "Cloudflare", Value: 1.5}
	m, ok := itemToMap(e)
	if !ok {
		t.Fatal("itemToMap returned false")
	}

	if got, want := m["id"].(int), 42; got != want {
		t.Errorf("m[\"id\"] = %d, want %d", got, want)
	}
	if got, want := m["name"].(string), "Cloudflare"; got != want {
		t.Errorf("m[\"name\"] = %q, want %q", got, want)
	}
	if got, want := m["value"].(float64), 1.5; got != want {
		t.Errorf("m[\"value\"] = %f, want %f", got, want)
	}
}

func TestApplyFieldProjectionReflect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		data       []any
		fields     []string
		wantFields []string
		wantAbsent []string
	}{
		{
			name: "basic field selection",
			data: []any{
				testEntity{ID: 1, Name: "Test", Value: 3.14},
			},
			fields:     []string{"name"},
			wantFields: []string{"id", "name"},
			wantAbsent: []string{"value"},
		},
		{
			name: "empty fields returns unchanged",
			data: []any{
				testEntity{ID: 1, Name: "Test"},
			},
			fields: []string{},
		},
		{
			name: "_set suffix fields preserved",
			data: []any{
				testEntity{ID: 1, Name: "Test", OrgSet: []string{"a", "b"}},
			},
			fields:     []string{"name"},
			wantFields: []string{"id", "name", "org_set"},
		},
		{
			name: "expanded FK objects preserved",
			data: []any{
				map[string]any{
					"id":   1,
					"name": "Test",
					"org":  map[string]any{"id": 42, "name": "Parent"},
				},
			},
			fields:     []string{"name"},
			wantFields: []string{"id", "name", "org"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := applyFieldProjection(tt.data, tt.fields)
			if len(tt.fields) == 0 {
				// Unchanged -- verify same length.
				if len(result) != len(tt.data) {
					t.Fatalf("expected %d items, got %d", len(tt.data), len(result))
				}
				return
			}
			if len(result) != len(tt.data) {
				t.Fatalf("expected %d items, got %d", len(tt.data), len(result))
			}
			m, ok := result[0].(map[string]any)
			if !ok {
				t.Fatal("projected item is not map[string]any")
			}
			for _, key := range tt.wantFields {
				if _, exists := m[key]; !exists {
					t.Errorf("missing expected key %q in projected result", key)
				}
			}
			for _, key := range tt.wantAbsent {
				if _, exists := m[key]; exists {
					t.Errorf("unexpected key %q should not be in projected result", key)
				}
			}
		})
	}
}

func TestFieldMapCaching(t *testing.T) {
	t.Parallel()

	typ := reflect.TypeOf(testEntity{})

	// Clear any cached entry for this type to start fresh.
	fieldMaps.Delete(typ)

	m1 := getFieldMap(typ)
	m2 := getFieldMap(typ)

	// Both calls should return a non-nil map (cached via sync.Map).
	if m1 == nil || m2 == nil {
		t.Fatal("getFieldMap returned nil")
	}

	// Verify same keys.
	if len(m1) != len(m2) {
		t.Fatalf("cached map length mismatch: %d vs %d", len(m1), len(m2))
	}
	for k := range m1 {
		if _, exists := m2[k]; !exists {
			t.Errorf("key %q missing from second call", k)
		}
	}

	// Verify expected keys from testEntity.
	expectedKeys := []string{"id", "name", "value", "org_set"}
	for _, k := range expectedKeys {
		if _, exists := m1[k]; !exists {
			t.Errorf("expected key %q not in field map", k)
		}
	}

	// Verify excluded keys.
	excludedKeys := []string{"-", "Internal", "Untagged"}
	for _, k := range excludedKeys {
		if _, exists := m1[k]; exists {
			t.Errorf("key %q should not be in field map", k)
		}
	}
}
