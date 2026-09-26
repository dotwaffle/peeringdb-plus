package httperr

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestWantsProblemJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		accept []string // one entry per Accept header line; nil = no header
		want   bool
	}{
		{"no header", nil, false},
		{"exact type", []string{"application/problem+json"}, true},
		{"q=0", []string{"application/problem+json;q=0"}, false},
		{"q=0.0", []string{"application/problem+json;q=0.0"}, false},
		{"q=0.001", []string{"application/problem+json;q=0.001"}, true},
		{"q=1", []string{"application/problem+json;q=1"}, true},
		{"q=1.5", []string{"application/problem+json;q=1.5"}, false},
		{"q=-1", []string{"application/problem+json;q=-1"}, false},
		{"q=abc", []string{"application/problem+json;q=abc"}, false},
		{"q=NaN", []string{"application/problem+json;q=NaN"}, false},
		{"q=Inf", []string{"application/problem+json;q=Inf"}, false},
		{"any type", []string{"*/*"}, false},
		{"application wildcard", []string{"application/*"}, false},
		{"application/json", []string{"application/json"}, false},
		{"mixed case type and Q", []string{"Application/Problem+JSON; Q=0.5"}, true},
		{"lower q next to json", []string{"application/json, application/problem+json;q=0.1"}, true},
		{"second header line", []string{"application/json", "application/problem+json"}, true},
		{"malformed range next to good one", []string{"a/b;;;=, application/problem+json"}, true},
		{"charset parameter", []string{"application/problem+json;charset=utf-8"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := http.Header{}
			for _, v := range tt.accept {
				h.Add("Accept", v)
			}
			if got := WantsProblemJSON(h); got != tt.want {
				t.Errorf("WantsProblemJSON(%q) = %v, want %v", tt.accept, got, tt.want)
			}
		})
	}
}

func TestWriteMetaError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    MetaErrorInput
		wantKeys []string
		wantMeta map[string]any
		wantData bool
	}{
		{
			name:     "error only",
			input:    MetaErrorInput{Status: http.StatusBadRequest, Error: "bad value"},
			wantKeys: []string{"meta"},
			wantMeta: map[string]any{"error": "bad value"},
		},
		{
			name:     "empty data",
			input:    MetaErrorInput{Status: http.StatusNotFound, Error: "Entity not found", EmptyData: true},
			wantKeys: []string{"data", "meta"},
			wantMeta: map[string]any{"error": "Entity not found"},
			wantData: true,
		},
		{
			name: "extra meta keys cannot replace error",
			input: MetaErrorInput{
				Status: http.StatusRequestEntityTooLarge,
				Error:  "too large",
				Meta:   map[string]any{"error": "replaced", "max_rows": 3, "budget_bytes": 100},
			},
			wantKeys: []string{"meta"},
			wantMeta: map[string]any{"error": "too large", "max_rows": float64(3), "budget_bytes": float64(100)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			WriteMetaError(rec, tt.input)

			if rec.Code != tt.input.Status {
				t.Errorf("status = %d, want %d", rec.Code, tt.input.Status)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			var body map[string]json.RawMessage
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v; body=%s", err, rec.Body.String())
			}
			if keys := slices.Sorted(maps.Keys(body)); !slices.Equal(keys, tt.wantKeys) {
				t.Errorf("top-level keys = %v, want %v", keys, tt.wantKeys)
			}
			var meta map[string]any
			if err := json.Unmarshal(body["meta"], &meta); err != nil {
				t.Fatalf("decode meta: %v", err)
			}
			if !maps.Equal(meta, tt.wantMeta) {
				t.Errorf("meta = %v, want %v", meta, tt.wantMeta)
			}
			if tt.wantData {
				if got := string(body["data"]); got != "[]" {
					t.Errorf("data = %s, want []", got)
				}
			}
		})
	}
}
