package pdbcompat

import (
	"net/url"
	"testing"
)

func TestParseFieldOp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantField string
		wantOp    string
	}{
		{
			name:      "field with contains operator",
			input:     "name__contains",
			wantField: "name",
			wantOp:    "contains",
		},
		{
			name:      "field with no operator",
			input:     "name",
			wantField: "name",
			wantOp:    "",
		},
		{
			name:      "field with startswith",
			input:     "name__startswith",
			wantField: "name",
			wantOp:    "startswith",
		},
		{
			name:      "field with in operator",
			input:     "asn__in",
			wantField: "asn",
			wantOp:    "in",
		},
		{
			name:      "field with lt operator",
			input:     "asn__lt",
			wantField: "asn",
			wantOp:    "lt",
		},
		{
			name:      "field with double underscore in field name",
			input:     "info_prefixes4__gt",
			wantField: "info_prefixes4",
			wantOp:    "gt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			field, op := parseFieldOp(tt.input)
			if field != tt.wantField {
				t.Errorf("parseFieldOp(%q) field = %q, want %q", tt.input, field, tt.wantField)
			}
			if op != tt.wantOp {
				t.Errorf("parseFieldOp(%q) op = %q, want %q", tt.input, op, tt.wantOp)
			}
		})
	}
}

func TestParseFilters(t *testing.T) {
	t.Parallel()

	// Test fields map simulating "net" type.
	fields := map[string]FieldType{
		"id":            FieldInt,
		"name":          FieldString,
		"aka":           FieldString,
		"asn":           FieldInt,
		"info_unicast":  FieldBool,
		"created":       FieldTime,
		"updated":       FieldTime,
		"info_traffic":  FieldString,
		"info_prefixes4": FieldInt,
		"status":        FieldString,
	}

	tests := []struct {
		name       string
		params     url.Values
		wantCount  int
		wantErr    bool
	}{
		{
			name:      "exact match on string field",
			params:    url.Values{"name": {"Cloudflare"}},
			wantCount: 1,
		},
		{
			name:      "contains operator on string field",
			params:    url.Values{"name__contains": {"cloud"}},
			wantCount: 1,
		},
		{
			name:      "startswith operator on string field",
			params:    url.Values{"name__startswith": {"Cloud"}},
			wantCount: 1,
		},
		{
			name:      "in operator on int field",
			params:    url.Values{"asn__in": {"13335,174"}},
			wantCount: 1,
		},
		{
			name:      "lt operator on int field",
			params:    url.Values{"asn__lt": {"1000"}},
			wantCount: 1,
		},
		{
			name:      "gt operator on int field",
			params:    url.Values{"asn__gt": {"1000"}},
			wantCount: 1,
		},
		{
			name:      "lte operator on int field",
			params:    url.Values{"asn__lte": {"1000"}},
			wantCount: 1,
		},
		{
			name:      "gte operator on int field",
			params:    url.Values{"asn__gte": {"1000"}},
			wantCount: 1,
		},
		{
			name:      "unknown field silently ignored per D-20",
			params:    url.Values{"nonexistent_field": {"value"}},
			wantCount: 0,
		},
		{
			name:      "reserved param limit ignored",
			params:    url.Values{"limit": {"10"}},
			wantCount: 0,
		},
		{
			name:      "reserved param skip ignored",
			params:    url.Values{"skip": {"5"}},
			wantCount: 0,
		},
		{
			name:      "reserved param depth ignored",
			params:    url.Values{"depth": {"2"}},
			wantCount: 0,
		},
		{
			name:      "reserved param since ignored",
			params:    url.Values{"since": {"1700000000"}},
			wantCount: 0,
		},
		{
			name:      "reserved param q ignored",
			params:    url.Values{"q": {"cloudflare"}},
			wantCount: 0,
		},
		{
			name:      "reserved param fields ignored",
			params:    url.Values{"fields": {"id,name,asn"}},
			wantCount: 0,
		},
		{
			name:      "unsupported operator returns error",
			params:    url.Values{"name__regex": {".*cloud.*"}},
			wantCount: 0,
			wantErr:   true,
		},
		{
			name:      "multiple filters produce multiple predicates",
			params:    url.Values{"name__contains": {"cloud"}, "asn__gt": {"1000"}},
			wantCount: 2,
		},
		{
			name:      "exact match on int field",
			params:    url.Values{"asn": {"13335"}},
			wantCount: 1,
		},
		{
			name:      "in operator on string field",
			params:    url.Values{"status__in": {"ok,pending"}},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			predicates, err := ParseFilters(tt.params, fields)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseFilters() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("ParseFilters() unexpected error: %v", err)
				return
			}
			if len(predicates) != tt.wantCount {
				t.Errorf("ParseFilters() returned %d predicates, want %d", len(predicates), tt.wantCount)
			}
		})
	}
}
