package pdbcompat

import (
	"context"
	"net/url"
	"strings"
	"testing"
)

func TestParseFieldOp(t *testing.T) {
	t.Parallel()

	// parseFieldOp returns (relationSegments, finalField, op).
	// Max len(relationSegments) == 2 (caller enforces the 2-hop cap via
	// len>2 rejection — parser returns the raw split so caller sees it).
	tests := []struct {
		name        string
		input       string
		wantRelSegs []string
		wantField   string
		wantOp      string
	}{
		// Non-traversal cases.
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
		// Traversal cases.
		{
			name:        "1-hop no op",
			input:       "org__name",
			wantRelSegs: []string{"org"},
			wantField:   "name",
			wantOp:      "",
		},
		{
			name:        "1-hop with contains",
			input:       "org__name__contains",
			wantRelSegs: []string{"org"},
			wantField:   "name",
			wantOp:      "contains",
		},
		{
			name:        "2-hop no op",
			input:       "ixlan__ix__fac_count",
			wantRelSegs: []string{"ixlan", "ix"},
			wantField:   "fac_count",
			wantOp:      "",
		},
		{
			name:        "2-hop with gt",
			input:       "ixlan__ix__fac_count__gt",
			wantRelSegs: []string{"ixlan", "ix"},
			wantField:   "fac_count",
			wantOp:      "gt",
		},
		{
			name:        "3-hop (caller rejects len>2) no op",
			input:       "a__b__c__d",
			wantRelSegs: []string{"a", "b", "c"},
			wantField:   "d",
			wantOp:      "",
		},
		{
			name:        "4-hop",
			input:       "a__b__c__d__e",
			wantRelSegs: []string{"a", "b", "c", "d"},
			wantField:   "e",
			wantOp:      "",
		},
		{
			name:      "leading sep malformed",
			input:     "__foo",
			wantField: "foo",
			// leading "__" produces [""] before "foo"; relSegs becomes [""]
			// — length 1 so caller will still enter traversal path, but
			// edge lookup on "" fails and key goes to unknown.
			wantRelSegs: []string{""},
			wantOp:      "",
		},
		{
			name:        "trailing sep empty final field",
			input:       "foo__",
			wantRelSegs: []string{"foo"},
			wantField:   "",
			wantOp:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			relSegs, field, op := parseFieldOp(tt.input)
			if field != tt.wantField {
				t.Errorf("parseFieldOp(%q) field = %q, want %q", tt.input, field, tt.wantField)
			}
			if op != tt.wantOp {
				t.Errorf("parseFieldOp(%q) op = %q, want %q", tt.input, op, tt.wantOp)
			}
			if !slicesEqual(relSegs, tt.wantRelSegs) {
				t.Errorf("parseFieldOp(%q) relSegs = %#v, want %#v", tt.input, relSegs, tt.wantRelSegs)
			}
		})
	}
}

// slicesEqual compares two string slices for equality, treating nil and
// zero-length slices as equal (matches how the rest of the test suite
// handles the "no relation segments" case).
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestParseFilters(t *testing.T) {
	t.Parallel()

	// Test TypeConfig simulating "net" type. ParseFilters
	// takes TypeConfig so it can consult FoldedFields for shadow-column
	// routing.
	tc := TypeConfig{
		Name: "net",
		Fields: map[string]FieldType{
			"id":             FieldInt,
			"name":           FieldString,
			"aka":            FieldString,
			"asn":            FieldInt,
			"info_unicast":   FieldBool,
			"created":        FieldTime,
			"updated":        FieldTime,
			"info_traffic":   FieldString,
			"info_prefixes4": FieldInt,
			"status":         FieldString,
		},
	}

	tests := []struct {
		name         string
		params       url.Values
		wantCount    int
		wantErr      bool
		wantEmptyRes bool
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
			name:      "unknown field silently ignored",
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
			// Unknown operator suffix
			// means the last segment ("regex") is treated as the final
			// field and the preceding segment ("name") as a relation
			// traversal. Since "name" is not an edge on this TypeConfig,
			// the key is silently ignored — no error, no predicate.
			// (Previously: parseFieldOp recognised ANY suffix as an op,
			// so buildPredicate rejected "regex" with an error.)
			name:      "unsupported operator silently ignored",
			params:    url.Values{"name__regex": {".*cloud.*"}},
			wantCount: 0,
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
		{
			// Empty __in short-circuits with emptyResult=true
			// and no predicates are emitted (caller returns []).
			name:         "empty __in triggers emptyResult sentinel",
			params:       url.Values{"asn__in": {""}},
			wantCount:    0,
			wantEmptyRes: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			predicates, emptyResult, err := ParseFilters(tt.params, tc)
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
			if emptyResult != tt.wantEmptyRes {
				t.Errorf("ParseFilters() emptyResult = %v, want %v", emptyResult, tt.wantEmptyRes)
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
			_, err := buildExact(tt.field, tt.value, tt.ft, false /*folded*/)
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
			_, err := buildContains(tt.field, "value", tt.ft, false /*folded*/)
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
			_, err := buildStartsWith(tt.field, "value", tt.ft, false /*folded*/)
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
			name:    "in on bool field with non-bool value",
			field:   "info_unicast",
			value:   "true,maybe",
			ft:      FieldBool,
			wantMsg: "convert",
		},
		{
			name:    "in on float field with non-numeric value",
			field:   "latitude",
			value:   "1.5,nope",
			ft:      FieldFloat,
			wantMsg: "convert",
		},
		{
			name:    "in on time field with non-numeric value",
			field:   "created",
			value:   "1700000000,later",
			ft:      FieldTime,
			wantMsg: "convert",
		},
		{
			name:    "in on unknown field type",
			field:   "mystery",
			value:   "a,b",
			ft:      FieldType(99),
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

// TestBuildIn_CoercesBoolFloatTime verifies __in now filters bool/float/
// time fields instead of returning an error (audit PA2 — upstream Django
// coerces these). A valid CSV must produce a non-nil predicate.
func TestBuildIn_CoercesBoolFloatTime(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		field string
		value string
		ft    FieldType
	}{
		{"bool", "info_unicast", "true,false", FieldBool},
		{"float", "latitude", "1.5,2.25", FieldFloat},
		{"time", "created", "1700000000,1700000100", FieldTime},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			pred, err := buildIn(c.field, c.value, c.ft)
			if err != nil {
				t.Fatalf("buildIn(%s) returned error: %v", c.ft, err)
			}
			if pred == nil {
				t.Fatalf("buildIn(%s) returned nil predicate", c.ft)
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

// TestFieldTypeString locks the human-readable rendering of each field
// type used in client-facing filter errors (audit U2).
func TestFieldTypeString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		ft   FieldType
		want string
	}{
		{FieldString, "string"},
		{FieldInt, "int"},
		{FieldBool, "bool"},
		{FieldTime, "time"},
		{FieldFloat, "float"},
		{FieldType(99), "unknown(99)"},
	}
	for _, tt := range tests {
		if got := tt.ft.String(); got != tt.want {
			t.Errorf("FieldType(%d).String() = %q, want %q", int(tt.ft), got, tt.want)
		}
	}
}

// TestFilterErrorsUseTypeNames verifies the three field-type error paths
// render the type name, never the raw enum integer (audit U2).
func TestFilterErrorsUseTypeNames(t *testing.T) {
	t.Parallel()
	_, exactErr := buildExact("f", "v", FieldType(99), false)
	_, convErr := convertValue("v", FieldType(99))
	_, inErr := buildIn("f", "v", FieldType(99))
	checks := []struct {
		name string
		err  error
	}{
		{"exact", exactErr},
		{"convert", convErr},
		{"in", inErr},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if c.err == nil {
				t.Fatal("expected error, got nil")
			}
			msg := c.err.Error()
			if !strings.Contains(msg, "unknown(99)") {
				t.Errorf("error %q does not contain human-readable type name", msg)
			}
			if strings.Contains(msg, "type 99") || strings.Contains(msg, "%!") {
				t.Errorf("error %q leaks raw enum integer or bad format verb", msg)
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

	tc := TypeConfig{
		Name: "test",
		Fields: map[string]FieldType{
			"asn":          FieldInt,
			"info_unicast": FieldBool,
			"created":      FieldTime,
			"latitude":     FieldFloat,
			"name":         FieldString,
		},
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
			// Bool __in now filters (audit PA2); only a malformed value
			// still propagates an error.
			name:    "in on bool field with bad value error propagated",
			params:  url.Values{"info_unicast__in": {"true,maybe"}},
			wantMsg: "filter info_unicast__in",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := ParseFilters(tt.params, tc)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantMsg)
			}
		})
	}
}

// TestParseFilters_AllThirteenEntitiesCoverPathA iterates the Allowlists
// map and asserts that AT LEAST ONE Path A key resolves to a predicate
// for each entity with a non-empty allowlist. Rationale: upstream
// PeeringDB's Django reverse-accessor aliases (e.g. "fac__*" on ix
// meaning "via ix_facilities") don't all map 1:1 to forward ent edges —
// codegen emits the literal allowlist entries from serializers.py but
// the Path A resolver falls through to silent-ignore when LookupEdge
// can't find a matching forward edge. That's the upstream-documented
// behaviour (rest.py:658-662: unknown filter fields are silently
// dropped). This test guards the regression where an entity's ENTIRE
// allowlist becomes unresolvable — i.e. an entity loses all its Path A
// plumbing due to a schema rename or annotation drop.
//
// Dynamically driven by the Allowlists map so adding a 14th entity
// auto-extends coverage. Path A allowlist-resolution lock-in.
func TestParseFilters_AllThirteenEntitiesCoverPathA(t *testing.T) {
	t.Parallel()

	// Seed value is pedestrian so int/string parse paths both resolve
	// without triggering the empty-IN sentinel.
	const probeValue = "1"

	for pdbType, entry := range Allowlists {
		tc, ok := Registry[pdbType]
		if !ok {
			t.Errorf("Allowlists[%q] has no Registry entry — skipping", pdbType)
			continue
		}
		// Gather every allowlist key: Direct entries + Via joined keys.
		keys := make([]string, 0, len(entry.Direct))
		keys = append(keys, entry.Direct...)
		for firstHop, tails := range entry.Via {
			for _, tail := range tails {
				keys = append(keys, firstHop+"__"+tail)
			}
		}
		if len(keys) == 0 {
			// Entity has an empty allowlist — structurally harmless.
			continue
		}
		t.Run(pdbType, func(t *testing.T) {
			t.Parallel()
			var anyResolved bool
			var resolvedKey string
			for _, k := range keys {
				params := url.Values{k: {probeValue}}
				preds, emptyResult, err := ParseFilters(params, tc)
				if err != nil {
					// Unexpected for a probe value like "1". Surface.
					t.Errorf("ParseFilters(%q=%q) on %s: unexpected error: %v",
						k, probeValue, pdbType, err)
					continue
				}
				if emptyResult {
					t.Errorf("ParseFilters(%q=%q) on %s: emptyResult=true, want false",
						k, probeValue, pdbType)
					continue
				}
				if len(preds) == 1 {
					anyResolved = true
					resolvedKey = k
					break
				}
			}
			if !anyResolved {
				t.Errorf("%s: no allowlist key resolved to a predicate (tried %d keys: %v) — entity's Path A plumbing is broken",
					pdbType, len(keys), keys)
			}
			_ = resolvedKey // kept for debugging; could be t.Logged
		})
	}
}

// TestParseFilters_UnknownFieldsAppendToCtx asserts the unknown-field
// diagnostics — unknown filter keys are silently
// dropped from the predicate slice AND appended to the ctx accumulator
// so handlers can emit slog.Debug + OTel span attr. Three distinct
// unknown-field shapes exercised: unknown top-level, 3-hop over-cap,
// unknown edge. ParseFiltersCtx is the diagnostic-aware entrypoint; the
// handler always uses ctx with WithUnknownFields before calling.
func TestParseFilters_UnknownFieldsAppendToCtx(t *testing.T) {
	t.Parallel()

	tc := Registry["net"]
	ctx := WithUnknownFields(context.Background())
	params := url.Values{
		"totally_bogus":    {"x"},
		"a__b__c__d":       {"y"}, // over-cap 3-hop
		"bogus_edge__name": {"z"},
	}
	preds, emptyResult, err := ParseFiltersCtx(ctx, params, tc)
	if err != nil {
		t.Fatalf("ParseFiltersCtx: unexpected error: %v", err)
	}
	if emptyResult {
		t.Errorf("emptyResult = true, want false")
	}
	if len(preds) != 0 {
		t.Errorf("preds count = %d, want 0 (all keys are unknown)", len(preds))
	}
	got := UnknownFieldsFromCtx(ctx)
	if len(got) != 3 {
		t.Fatalf("UnknownFieldsFromCtx len = %d, want 3; got=%v", len(got), got)
	}
	// Accumulator order follows range-iteration order which is
	// randomised for maps — normalise via a seen-set for assertion.
	wantSet := map[string]bool{
		"totally_bogus":    true,
		"a__b__c__d":       true,
		"bogus_edge__name": true,
	}
	for _, k := range got {
		if !wantSet[k] {
			t.Errorf("unexpected unknown field %q; want one of %v", k, wantSet)
		}
		delete(wantSet, k)
	}
	if len(wantSet) > 0 {
		t.Errorf("missing unknown fields in accumulator: %v", wantSet)
	}
}
