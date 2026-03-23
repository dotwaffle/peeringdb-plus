// Package conformance provides structural comparison of JSON API responses
// for validating PeeringDB compatibility layer output against the real
// PeeringDB API. Comparison is structure-only: field names, value types,
// and nesting depth are checked, but actual values are not.
package conformance

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Difference describes a structural mismatch between two JSON responses.
type Difference struct {
	Path    string // JSON path, e.g., "data[0].net_set[0].asn"
	Kind    string // "missing_field", "extra_field", "type_mismatch"
	Details string // Human-readable description
}

// CompareStructure compares the JSON structure of reference and actual maps.
// It checks field names, value types (string, number, bool, null, array,
// object), and nesting depth. Values are not compared. Differences are
// returned sorted by Path for deterministic output.
func CompareStructure(reference, actual map[string]any) []Difference {
	diffs := compareStructure("", reference, actual)
	sort.Slice(diffs, func(i, j int) bool {
		return diffs[i].Path < diffs[j].Path
	})
	return diffs
}

// compareStructure recursively compares two maps, building a dot-separated
// path prefix for nested fields.
func compareStructure(prefix string, reference, actual map[string]any) []Difference {
	var diffs []Difference

	for key, refVal := range reference {
		path := joinPath(prefix, key)
		actVal, ok := actual[key]
		if !ok {
			diffs = append(diffs, Difference{
				Path:    path,
				Kind:    "missing_field",
				Details: "field present in reference but missing in actual",
			})
			continue
		}
		diffs = append(diffs, compareValues(path, refVal, actVal)...)
	}

	for key := range actual {
		if _, ok := reference[key]; !ok {
			path := joinPath(prefix, key)
			diffs = append(diffs, Difference{
				Path:    path,
				Kind:    "extra_field",
				Details: "field present in actual but missing in reference",
			})
		}
	}

	return diffs
}

// compareValues compares two values structurally, recursing into objects
// and comparing first elements of arrays.
func compareValues(path string, refVal, actVal any) []Difference {
	refType := jsonType(refVal)
	actType := jsonType(actVal)

	if refType != actType {
		return []Difference{{
			Path:    path,
			Kind:    "type_mismatch",
			Details: fmt.Sprintf("reference type %q, actual type %q", refType, actType),
		}}
	}

	// Recurse into nested objects.
	if refMap, ok := refVal.(map[string]any); ok {
		if actMap, ok := actVal.(map[string]any); ok {
			return compareStructure(path, refMap, actMap)
		}
	}

	// Compare array element structure using first element.
	if refArr, ok := refVal.([]any); ok {
		if actArr, ok := actVal.([]any); ok {
			if len(refArr) > 0 && len(actArr) > 0 {
				elemPath := path + "[0]"
				return compareValues(elemPath, refArr[0], actArr[0])
			}
		}
	}

	return nil
}

// jsonType returns the JSON type name for a Go value decoded from JSON.
func jsonType(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "bool"
	case float64, json.Number:
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return fmt.Sprintf("unknown(%T)", v)
	}
}

// joinPath builds a dot-separated JSON path.
func joinPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// ExtractStructure parses a JSON body and returns it as a map for structural
// comparison. The input should be a PeeringDB JSON envelope
// ({"meta":{},"data":[...]}).
func ExtractStructure(jsonBody []byte) (map[string]any, error) {
	var result map[string]any
	if err := json.Unmarshal(jsonBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return result, nil
}

// CompareResponses is a convenience wrapper that extracts structure from
// both JSON bodies and compares them. It returns differences found between
// the reference and actual response structures.
func CompareResponses(reference, actual []byte) ([]Difference, error) {
	refMap, err := ExtractStructure(reference)
	if err != nil {
		return nil, fmt.Errorf("extract reference structure: %w", err)
	}

	actMap, err := ExtractStructure(actual)
	if err != nil {
		return nil, fmt.Errorf("extract actual structure: %w", err)
	}

	return CompareStructure(refMap, actMap), nil
}
