package conformance

import (
	"testing"
)

func TestCompareStructure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		reference map[string]any
		actual    map[string]any
		wantCount int
		wantKinds []string // expected Kind values in order
		wantPaths []string // expected Path values in order
	}{
		{
			name:      "identical flat structures",
			reference: map[string]any{"id": float64(1), "name": "test"},
			actual:    map[string]any{"id": float64(2), "name": "other"},
			wantCount: 0,
		},
		{
			name:      "missing field in actual",
			reference: map[string]any{"id": float64(1), "name": "test"},
			actual:    map[string]any{"id": float64(1)},
			wantCount: 1,
			wantKinds: []string{"missing_field"},
			wantPaths: []string{"name"},
		},
		{
			name:      "extra field in actual",
			reference: map[string]any{"id": float64(1)},
			actual:    map[string]any{"id": float64(1), "extra": "value"},
			wantCount: 1,
			wantKinds: []string{"extra_field"},
			wantPaths: []string{"extra"},
		},
		{
			name:      "type mismatch string vs number",
			reference: map[string]any{"field": "text"},
			actual:    map[string]any{"field": float64(42)},
			wantCount: 1,
			wantKinds: []string{"type_mismatch"},
			wantPaths: []string{"field"},
		},
		{
			name: "nested object with missing inner field",
			reference: map[string]any{
				"outer": map[string]any{"inner": "value", "other": float64(1)},
			},
			actual: map[string]any{
				"outer": map[string]any{"inner": "value"},
			},
			wantCount: 1,
			wantKinds: []string{"missing_field"},
			wantPaths: []string{"outer.other"},
		},
		{
			name: "array with different element structure",
			reference: map[string]any{
				"items": []any{map[string]any{"a": float64(1), "b": "x"}},
			},
			actual: map[string]any{
				"items": []any{map[string]any{"a": float64(2)}},
			},
			wantCount: 1,
			wantKinds: []string{"missing_field"},
			wantPaths: []string{"items[0].b"},
		},
		{
			name:      "null vs object compatible",
			reference: map[string]any{"field": map[string]any{"nested": "val"}},
			actual:    map[string]any{"field": nil},
			wantCount: 0,
		},
		{
			name:      "null vs string compatible",
			reference: map[string]any{"field": nil},
			actual:    map[string]any{"field": "populated"},
			wantCount: 0,
		},
		{
			name:      "empty arrays on both sides",
			reference: map[string]any{"items": []any{}},
			actual:    map[string]any{"items": []any{}},
			wantCount: 0,
		},
		{
			name:      "values ignored same type different value",
			reference: map[string]any{"count": float64(100), "active": true},
			actual:    map[string]any{"count": float64(999), "active": false},
			wantCount: 0,
		},
		{
			// PeeringDB 2.83.0 per-row metadata: keys vary per row.
			name: "per-row meta keys are opaque",
			reference: map[string]any{
				"data": []any{map[string]any{"meta": map[string]any{}}},
			},
			actual: map[string]any{
				"data": []any{map[string]any{"meta": map[string]any{
					"rfc8950": true,
					"planned_status_change": map[string]any{
						"status": "deleted", "date": "2026-10-01",
					},
				}}},
			},
			wantCount: 0,
		},
		{
			name: "nested object meta keys are opaque",
			reference: map[string]any{
				"data": []any{map[string]any{
					"net": map[string]any{"meta": map[string]any{"preferred_ip_mtu": float64(9000)}},
				}},
			},
			actual: map[string]any{
				"data": []any{map[string]any{
					"net": map[string]any{"meta": map[string]any{}},
				}},
			},
			wantCount: 0,
		},
		{
			name: "per-row meta type still compared",
			reference: map[string]any{
				"data": []any{map[string]any{"meta": map[string]any{}}},
			},
			actual: map[string]any{
				"data": []any{map[string]any{"meta": []any{}}},
			},
			wantCount: 1,
			wantKinds: []string{"type_mismatch"},
			wantPaths: []string{"data[0].meta"},
		},
		{
			name: "missing per-row meta still reported",
			reference: map[string]any{
				"data": []any{map[string]any{"id": float64(1), "meta": map[string]any{}}},
			},
			actual: map[string]any{
				"data": []any{map[string]any{"id": float64(1)}},
			},
			wantCount: 1,
			wantKinds: []string{"missing_field"},
			wantPaths: []string{"data[0].meta"},
		},
		{
			name: "envelope meta compared in full",
			reference: map[string]any{
				"meta": map[string]any{"generated": float64(1)},
				"data": []any{},
			},
			actual: map[string]any{
				"meta": map[string]any{},
				"data": []any{},
			},
			wantCount: 1,
			wantKinds: []string{"missing_field"},
			wantPaths: []string{"meta.generated"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			diffs := CompareStructure(tt.reference, tt.actual)

			if len(diffs) != tt.wantCount {
				t.Fatalf("got %d differences, want %d: %+v", len(diffs), tt.wantCount, diffs)
			}

			for i, d := range diffs {
				if i < len(tt.wantKinds) && d.Kind != tt.wantKinds[i] {
					t.Errorf("diff[%d].Kind = %q, want %q", i, d.Kind, tt.wantKinds[i])
				}
				if i < len(tt.wantPaths) && d.Path != tt.wantPaths[i] {
					t.Errorf("diff[%d].Path = %q, want %q", i, d.Path, tt.wantPaths[i])
				}
			}
		})
	}
}

func TestExtractStructure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid PeeringDB envelope",
			input:   `{"meta":{},"data":[{"id":1,"name":"test"}]}`,
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			input:   `{not valid json`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := ExtractStructure([]byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result == nil {
				t.Fatal("expected non-nil result")
			}
		})
	}
}

func TestCompareResponses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		reference string
		actual    string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "matching responses",
			reference: `{"meta":{},"data":[{"id":1,"name":"test"}]}`,
			actual:    `{"meta":{},"data":[{"id":2,"name":"other"}]}`,
			wantCount: 0,
		},
		{
			name:      "per-row meta keys ignored, envelope kept",
			reference: `{"meta":{},"data":[{"id":1,"meta":{}}]}`,
			actual:    `{"meta":{},"data":[{"id":2,"meta":{"rfc8950":true}}]}`,
			wantCount: 0,
		},
		{
			name:      "structurally different responses",
			reference: `{"meta":{},"data":[{"id":1,"name":"test"}]}`,
			actual:    `{"meta":{},"data":[{"id":2}]}`,
			wantCount: 1,
		},
		{
			name:      "invalid reference JSON",
			reference: `{bad`,
			actual:    `{"meta":{},"data":[]}`,
			wantErr:   true,
		},
		{
			name:      "invalid actual JSON",
			reference: `{"meta":{},"data":[]}`,
			actual:    `{bad`,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			diffs, err := CompareResponses([]byte(tt.reference), []byte(tt.actual))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(diffs) != tt.wantCount {
				t.Errorf("got %d differences, want %d: %+v", len(diffs), tt.wantCount, diffs)
			}
		})
	}
}
