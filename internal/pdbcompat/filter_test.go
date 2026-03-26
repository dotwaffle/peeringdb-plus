package pdbcompat

import (
	"net/url"
	"strings"
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

// TestBuildExactErrors tests error paths in buildExact for all field types.
func TestBuildExactErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		field   string
		value   string
		ft      FieldType
		wantMsg string
	}{
		{
			name:    "int field non-numeric value",
			field:   "asn",
			value:   "not-a-number",
			ft:      FieldInt,
			wantMsg: "convert",
		},
		{
			name:    "bool field invalid value",
			field:   "info_unicast",
			value:   "maybe",
			ft:      FieldBool,
			wantMsg: "convert",
		},
		{
			name:    "time field invalid value",
			field:   "created",
			value:   "not-a-timestamp",
			ft:      FieldTime,
			wantMsg: "convert",
		},
		{
			name:    "float field non-numeric value",
			field:   "latitude",
			value:   "not-a-float",
			ft:      FieldFloat,
			wantMsg: "convert",
		},
		{
			name:    "unsupported field type",
			field:   "unknown",
			value:   "x",
			ft:      FieldType(99),
			wantMsg: "unsupported field type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := buildExact(tt.field, tt.value, tt.ft)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantMsg)
			}
		})
	}
}

// TestBuildContainsErrors tests error paths for contains on non-string fields.
func TestBuildContainsErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		field   string
		ft      FieldType
		wantMsg string
	}{
		{
			name:    "contains on int field",
			field:   "asn",
			ft:      FieldInt,
			wantMsg: "contains operator not supported on non-string field",
		},
		{
			name:    "contains on bool field",
			field:   "info_unicast",
			ft:      FieldBool,
			wantMsg: "contains operator not supported on non-string field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := buildContains(tt.field, "value", tt.ft)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantMsg)
			}
		})
	}
}

// TestBuildStartsWithErrors tests error paths for startswith on non-string fields.
func TestBuildStartsWithErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		field   string
		ft      FieldType
		wantMsg string
	}{
		{
			name:    "startswith on int field",
			field:   "asn",
			ft:      FieldInt,
			wantMsg: "startswith operator not supported on non-string field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := buildStartsWith(tt.field, "value", tt.ft)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantMsg)
			}
		})
	}
}

// TestBuildInErrors tests error paths for IN operator.
func TestBuildInErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		field   string
		value   string
		ft      FieldType
		wantMsg string
	}{
		{
			name:    "in on int field with non-numeric value",
			field:   "asn",
			value:   "13335,notanumber",
			ft:      FieldInt,
			wantMsg: "convert",
		},
		{
			name:    "in on unsupported field type",
			field:   "info_unicast",
			value:   "true,false",
			ft:      FieldBool,
			wantMsg: "in operator not supported on field type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := buildIn(tt.field, tt.value, tt.ft)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantMsg)
			}
		})
	}
}

// TestConvertValueErrors tests error paths in convertValue.
func TestConvertValueErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		ft      FieldType
		wantMsg string
	}{
		{
			name:    "unsupported field type",
			value:   "x",
			ft:      FieldType(99),
			wantMsg: "unsupported field type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := convertValue(tt.value, tt.ft)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantMsg)
			}
		})
	}
}

// TestParseBoolErrors tests error paths in parseBool.
func TestParseBoolErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantMsg string
	}{
		{name: "invalid string", input: "maybe", wantMsg: "invalid bool value"},
		{name: "empty string", input: "", wantMsg: "invalid bool value"},
		{name: "numeric 2", input: "2", wantMsg: "invalid bool value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseBool(tt.input)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantMsg)
			}
		})
	}
}

// TestParseTimeErrors tests error paths in parseTime.
func TestParseTimeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantMsg string
	}{
		{name: "non-numeric", input: "not-a-timestamp", wantMsg: "invalid unix timestamp"},
		{name: "float value", input: "123.456", wantMsg: "invalid unix timestamp"},
		{name: "empty string", input: "", wantMsg: "invalid unix timestamp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseTime(tt.input)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantMsg)
			}
		})
	}
}

// TestParseFiltersErrorPaths tests error propagation through ParseFilters.
func TestParseFiltersErrorPaths(t *testing.T) {
	t.Parallel()

	fields := map[string]FieldType{
		"asn":          FieldInt,
		"info_unicast": FieldBool,
		"created":      FieldTime,
		"latitude":     FieldFloat,
		"name":         FieldString,
	}

	tests := []struct {
		name    string
		params  url.Values
		wantMsg string
	}{
		{
			name:    "int conversion error propagated",
			params:  url.Values{"asn": {"not-a-number"}},
			wantMsg: "filter asn",
		},
		{
			name:    "bool conversion error propagated",
			params:  url.Values{"info_unicast": {"maybe"}},
			wantMsg: "filter info_unicast",
		},
		{
			name:    "time conversion error propagated",
			params:  url.Values{"created": {"not-a-time"}},
			wantMsg: "filter created",
		},
		{
			name:    "float conversion error propagated",
			params:  url.Values{"latitude": {"not-a-float"}},
			wantMsg: "filter latitude",
		},
		{
			name:    "contains on int field error propagated",
			params:  url.Values{"asn__contains": {"123"}},
			wantMsg: "filter asn__contains",
		},
		{
			name:    "startswith on int field error propagated",
			params:  url.Values{"asn__startswith": {"123"}},
			wantMsg: "filter asn__startswith",
		},
		{
			name:    "in with non-numeric int values error propagated",
			params:  url.Values{"asn__in": {"13335,abc"}},
			wantMsg: "filter asn__in",
		},
		{
			name:    "in on bool field error propagated",
			params:  url.Values{"info_unicast__in": {"true,false"}},
			wantMsg: "filter info_unicast__in",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseFilters(tt.params, fields)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantMsg)
			}
		})
	}
}
