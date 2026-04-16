package visbaseline

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// ReportSchemaVersion is the frozen schema version for diff.json. Bump on
// any non-additive change to the emitted structure.
const ReportSchemaVersion = 1

// Report is the per-run diff artifact. It carries aggregate counts and
// field-level deltas for each PeeringDB type, but NEVER actual field values
// (except for the controlled enum `visible`). See threat T-57-02.
type Report struct {
	SchemaVersion int                   `json:"schema_version"`
	GeneratedAt   time.Time             `json:"generated"`
	Targets       []string              `json:"targets"`
	Types         map[string]TypeReport `json:"types"`
}

// TypeReport describes the anon vs auth delta for a single PeeringDB type.
// Row counts + field names + the controlled `visible` enum only — no values,
// no lengths, no hashes.
type TypeReport struct {
	AnonRowCount      int          `json:"anon_row_count"`
	AuthRowCount      int          `json:"auth_row_count"`
	AuthOnlyRowCount  int          `json:"auth_only_row_count"`
	VisibleValuesAnon []string     `json:"visible_values_anon,omitempty"`
	VisibleValuesAuth []string     `json:"visible_values_auth,omitempty"`
	Fields            []FieldDelta `json:"fields"`
}

// FieldDelta describes a single field-level observation. It DOES NOT carry
// field values, lengths, hashes, or any signal that could fingerprint the
// underlying data. See threat T-57-02 and 57-RESEARCH.md Pitfall 4.
type FieldDelta struct {
	Name          string `json:"name"`
	AuthOnly      bool   `json:"auth_only"`
	Placeholder   string `json:"placeholder,omitempty"`     // "<auth-only:TYPE>" sentinel
	RowsAdded     int    `json:"rows_added,omitempty"`      // count of auth rows with this field absent-in-anon
	ValueSetDrift bool   `json:"value_set_drift,omitempty"` // true for "visible" when new enum values appear
	IsPII         bool   `json:"is_pii,omitempty"`          // IsPIIField(Name) at construction time
}

// envelopeForDiff models a PeeringDB response envelope for parsing.
type envelopeForDiff struct {
	Meta json.RawMessage  `json:"meta"`
	Data []map[string]any `json:"data"`
}

// Diff compares an anon/auth envelope pair for a single PeeringDB type and
// returns a TypeReport. The caller is responsible for assembling TypeReports
// into a full Report (covering all 13 types and both targets).
//
// Diff is a pure function: same input bytes yield byte-stable output.
func Diff(typeName string, anonBytes, authBytes []byte) (TypeReport, error) {
	// Disambiguate envelope-with-no-"data"-key from empty-data-array on BOTH
	// sides before unmarshalling into the typed envelope. An envelope missing
	// "data" would otherwise silently produce a zero-row diff.
	if err := requireDataKey(anonBytes, typeName, "anon"); err != nil {
		return TypeReport{}, err
	}
	if err := requireDataKey(authBytes, typeName, "auth"); err != nil {
		return TypeReport{}, err
	}

	var anon envelopeForDiff
	if err := json.Unmarshal(anonBytes, &anon); err != nil {
		return TypeReport{}, fmt.Errorf("unmarshal anon %s: %w", typeName, err)
	}
	var auth envelopeForDiff
	if err := json.Unmarshal(authBytes, &auth); err != nil {
		return TypeReport{}, fmt.Errorf("unmarshal auth %s: %w", typeName, err)
	}

	// Index anon rows by id for fast lookup.
	anonByID := make(map[float64]map[string]any, len(anon.Data))
	for _, row := range anon.Data {
		if id, ok := row["id"].(float64); ok {
			anonByID[id] = row
		}
	}

	rep := TypeReport{
		AnonRowCount: len(anon.Data),
		AuthRowCount: len(auth.Data),
	}

	// Walk auth rows. Accumulate per-field RowsAdded counts.
	fieldCounts := make(map[string]int)
	authOnlyRows := 0
	for _, authRow := range auth.Data {
		var anonRow map[string]any
		if id, ok := authRow["id"].(float64); ok {
			anonRow = anonByID[id]
		}
		if anonRow == nil {
			authOnlyRows++
			for fname := range authRow {
				fieldCounts[fname]++
			}
			continue
		}
		for fname := range authRow {
			if _, present := anonRow[fname]; !present {
				fieldCounts[fname]++
			}
		}
	}
	rep.AuthOnlyRowCount = authOnlyRows

	// Extract the visible enum set for both modes. This is a controlled
	// vocabulary (Public/Users/Private) — NOT PII per 57-RESEARCH.md Pitfall 2.
	rep.VisibleValuesAnon = extractVisibleSet(anon.Data)
	rep.VisibleValuesAuth = extractVisibleSet(auth.Data)
	visibleDrift := !stringSliceEqual(rep.VisibleValuesAnon, rep.VisibleValuesAuth)
	if visibleDrift {
		// Ensure the "visible" field is represented in Fields with
		// ValueSetDrift set. Use a sentinel (-1) to distinguish "synthesised
		// because of drift" from "counted via auth-only rows"; the builder
		// below re-reads fieldCounts and does not use the sentinel value.
		if _, ok := fieldCounts["visible"]; !ok {
			fieldCounts["visible"] = 0
		}
	}

	// Build field deltas — sorted by Name for determinism.
	names := make([]string, 0, len(fieldCounts))
	for n := range fieldCounts {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, n := range names {
		fd := FieldDelta{
			Name:      n,
			AuthOnly:  fieldCounts[n] > 0,
			RowsAdded: fieldCounts[n],
			IsPII:     IsPIIField(n),
		}
		if n == "visible" && visibleDrift {
			// visible is a controlled vocabulary, not an auth-only field —
			// flip AuthOnly off and surface the value-set drift flag.
			fd.AuthOnly = false
			fd.RowsAdded = 0
			fd.ValueSetDrift = true
		}
		if fd.AuthOnly && fd.Placeholder == "" {
			// We cannot know the JSON type of an auth-only field without
			// probing a sample value — and we refuse to probe because doing
			// so risks leaking information. Default to the string placeholder
			// sentinel; the redactor (plan 01) already ensured the committed
			// file uses the correct placeholder in its per-row position.
			fd.Placeholder = PlaceholderString
		}
		rep.Fields = append(rep.Fields, fd)
	}

	return rep, nil
}

// extractVisibleSet returns the sorted-unique set of the "visible" field's
// string values across the rows. Empty or missing "visible" fields are
// skipped (not emitted as "<empty>").
func extractVisibleSet(rows []map[string]any) []string {
	set := make(map[string]struct{})
	for _, r := range rows {
		if v, ok := r["visible"].(string); ok && v != "" {
			set[v] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// stringSliceEqual reports whether two string slices are element-wise equal.
// Assumes both are sorted.
func stringSliceEqual(a, b []string) bool {
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

// requireDataKey returns an error if the envelope bytes do not contain a
// top-level "data" key. side is "anon" or "auth" and is echoed in the error
// for call-site clarity. Empty or invalid JSON also produces an error.
func requireDataKey(raw []byte, typeName, side string) error {
	if len(raw) == 0 {
		return fmt.Errorf("%s %s: empty envelope bytes", side, typeName)
	}
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return fmt.Errorf("%s %s envelope: %w", side, typeName, err)
	}
	if _, ok := probe["data"]; !ok {
		return fmt.Errorf("%s %s: envelope missing \"data\" key", side, typeName)
	}
	return nil
}
