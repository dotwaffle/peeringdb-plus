package pdbcompat

import (
	"encoding/json"
	"strings"

	"entgo.io/ent/dialect/sql"
)

// buildSearchPredicate creates a sql.Selector predicate that ORs together
// case-insensitive contains matches across the given fields for the search
// term. Returns nil if search is empty, which signals the caller to skip
// applying a search filter.
func buildSearchPredicate(search string, searchFields []string) func(*sql.Selector) {
	search = strings.TrimSpace(search)
	if search == "" {
		return nil
	}
	return func(s *sql.Selector) {
		var ors []*sql.Predicate
		for _, f := range searchFields {
			ors = append(ors, sql.ContainsFold(f, search))
		}
		if len(ors) > 0 {
			s.Where(sql.Or(ors...))
		}
	}
}

// applyFieldProjection filters each object in data to include only the
// requested fields. The "id" field is always included even if not explicitly
// listed. Fields that do not exist on an object are silently ignored.
// If fields is empty, data is returned unchanged.
//
// For depth > 0 responses, _set fields and expanded FK edge objects are
// preserved regardless of the field list.
func applyFieldProjection(data []any, fields []string) []any {
	if len(fields) == 0 {
		return data
	}

	// Build a lookup set with the requested fields plus "id".
	want := make(map[string]bool, len(fields)+1)
	want["id"] = true
	for _, f := range fields {
		want[strings.TrimSpace(f)] = true
	}

	out := make([]any, len(data))
	for i, item := range data {
		m, ok := itemToMap(item)
		if !ok {
			out[i] = item
			continue
		}
		projected := make(map[string]any, len(want))
		for k, v := range m {
			// Always keep _set fields and expanded FK objects (depth > 0 responses).
			if want[k] || strings.HasSuffix(k, "_set") {
				projected[k] = v
				continue
			}
			// Check if this is an expanded FK object (value is a map with an
			// "id" key). These are produced by depth=2 expansion for things
			// like "org", "net", "fac", etc.
			if isExpandedObject(v) {
				projected[k] = v
			}
		}
		out[i] = projected
	}
	return out
}

// itemToMap converts an item (struct or map) to a map[string]any.
// If the item is already a map[string]any it is returned directly.
// Otherwise it is marshaled/unmarshaled through JSON.
func itemToMap(item any) (map[string]any, bool) {
	if m, ok := item.(map[string]any); ok {
		return m, true
	}
	b, err := json.Marshal(item)
	if err != nil {
		return nil, false
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, false
	}
	return m, true
}

// isExpandedObject checks if a value is an expanded FK object (a map with
// an "id" key), as produced by depth=2 expansion.
func isExpandedObject(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	_, hasID := m["id"]
	return hasID
}
